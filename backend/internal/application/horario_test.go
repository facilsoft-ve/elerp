package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/mesonero"
)

/* Horarios, vencimiento y tiempo extra del salón.
 *
 * El horario NO es un candado: quien autoriza cada turno sigue siendo el
 * supervisor. Lo que estas pruebas cuidan es que haga bien las tres cosas para
 * las que existe —calcular la hora de salida, marcar a quien entró fuera de su
 * turno y disparar el cierre suave donde la sede lo pidió— y que el tiempo extra
 * no se convierta en una puerta trasera para deshacer la decisión de alguien. */

// zonaVET es la hora legal de Venezuela. El horario se declara en hora de PARED
// ("18:00"), así que los tramos hay que armarlos contra el reloj local o la
// prueba pasaría o fallaría según la hora a la que se corra.
var zonaVET = time.FixedZone("VET", -4*60*60)

// todosLosDias cubre la semana entera: una prueba que solo falla los martes es
// peor que no tenerla.
var todosLosDias = []int{0, 1, 2, 3, 4, 5, 6}

// servicioHorarios monta el salón con credenciales, turnos y horarios.
func servicioHorarios(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := servicioTurnos(t)
	svc.ConHorarios(st.Horarios)
	return svc, st
}

// cierraSolo deja la sede en modo «cierre automático al vencer el horario».
func cierraSolo(t *testing.T, svc *application.Service) {
	t.Helper()
	modo := mesonero.HorarioCierraSolo
	// La asignación estricta va en nil: se toca SOLO el modo de horario.
	if _, err := svc.GuardarConfigSalonCompleta(empSalon, sedeSalon, actorA, origenTst, nil, &modo); err != nil {
		t.Fatalf("configurar el modo de horario: %v", err)
	}
}

/* franjaQueCubreAhora arma un tramo que contiene el instante actual y termina
 * `horas` más adelante, sea la hora que sea. Cubre todos los días para no
 * depender del día en que se corra; con la semana entera, un tramo que cruza
 * medianoche también encuentra su día de arranque (a las 00:30 el tramo vigente
 * es el que abrió ayer). */
func franjaQueCubreAhora(horas int) mesonero.Franja {
	ahora := time.Now().In(zonaVET)
	return mesonero.Franja{
		Dias:  todosLosDias,
		Desde: ahora.Add(-time.Hour).Format("15:04"),
		Hasta: ahora.Add(time.Duration(horas) * time.Hour).Format("15:04"),
	}
}

// franjaDeUnDiaQueNoTrabaja arma un tramo en un día que no es ni hoy ni ayer
// —los dos que el cálculo mira— para poder probar la entrada fuera de horario.
func franjaDeUnDiaQueNoTrabaja() mesonero.Franja {
	otro := int(time.Now().In(zonaVET).AddDate(0, 0, 3).Weekday())
	return mesonero.Franja{Dias: []int{otro}, Desde: "10:00", Hasta: "14:00"}
}

// conHorario le fija a un mesonero un tramo que lo cubre ahora mismo.
func conHorario(t *testing.T, svc *application.Service, m mesonero.Mesonero, horas int) {
	t.Helper()
	if _, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID,
		[]mesonero.Franja{franjaQueCubreAhora(horas)}); err != nil {
		t.Fatalf("guardar horario de %s: %v", m.Nombre, err)
	}
}

// vencerTurno mueve la hora de salida al pasado: es la forma de probar el
// vencimiento sin adelantar el reloj ni esperar de verdad.
func vencerTurno(t *testing.T, st *inmem.Store, turnoID string, haceCuanto time.Duration) {
	t.Helper()
	tu, ok := st.Turnos.ByID(empSalon, turnoID)
	if !ok {
		t.Fatalf("no se encontró el turno %s para vencerlo", turnoID)
	}
	tu.FinPrevisto = time.Now().UTC().Add(-haceCuanto).Format(time.RFC3339)
	if _, ok := st.Turnos.Update(tu); !ok {
		t.Fatalf("no se pudo vencer el turno %s", turnoID)
	}
}

// turnoDe relee el turno del repo: casi todo lo que se prueba acá son cambios
// que el servicio aplica por su cuenta, no cosas que devuelva.
func turnoDe(t *testing.T, st *inmem.Store, turnoID string) mesonero.Turno {
	t.Helper()
	tu, ok := st.Turnos.ByID(empSalon, turnoID)
	if !ok {
		t.Fatalf("no se encontró el turno %s", turnoID)
	}
	return tu
}

/* --- Validación del horario ------------------------------------------------ */

// Un horario mal guardado no se nota hasta que el vencimiento no dispara nunca o
// dispara siempre. Por eso se valida al guardar y no al usar.
func TestGuardarHorario_RechazaTramosInvalidos(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	hoy := int(time.Now().In(zonaVET).Weekday())

	casos := []struct {
		nombre string
		franja mesonero.Franja
	}{
		{"hora que no existe", mesonero.Franja{Dias: []int{hoy}, Desde: "25:00", Hasta: "26:00"}},
		{"minutos fuera de rango", mesonero.Franja{Dias: []int{hoy}, Desde: "18:60", Hasta: "20:00"}},
		{"hora sin cero a la izquierda", mesonero.Franja{Dias: []int{hoy}, Desde: "8:30", Hasta: "12:00"}},
		{"hora vacía", mesonero.Franja{Dias: []int{hoy}, Desde: "", Hasta: "20:00"}},
		{"día fuera de 0..6", mesonero.Franja{Dias: []int{7}, Desde: "18:00", Hasta: "22:00"}},
		{"día negativo", mesonero.Franja{Dias: []int{-1}, Desde: "18:00", Hasta: "22:00"}},
		// Un tramo sin días no se aplica nunca y, peor, se VE configurado: quien lo
		// guardó creería tener horario.
		{"sin días", mesonero.Franja{Dias: []int{}, Desde: "18:00", Hasta: "22:00"}},
		// Desde == Hasta no es «24 horas», es duración cero: dejaría un turno que
		// vence en el mismo instante en que se abre.
		{"duración cero", mesonero.Franja{Dias: []int{hoy}, Desde: "18:00", Hasta: "18:00"}},
	}
	for _, c := range casos {
		_, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID, []mesonero.Franja{c.franja})
		if !errors.Is(err, application.ErrHorarioInvalido) {
			t.Errorf("%s: se esperaba ErrHorarioInvalido, se obtuvo: %v", c.nombre, err)
		}
	}
	// Y ninguno de los rechazos dejó nada guardado a medias.
	if h := svc.HorarioDe(empSalon, m.ID); !h.SinHorario() {
		t.Errorf("un horario rechazado no debe quedar guardado, quedó %+v", h.Franjas)
	}
}

// Los días llegan como los toque quien configura: repetidos y en cualquier
// orden. Guardarlos así haría que dos horarios idénticos se vean distintos, y
// que mostrarlos dependa del orden en que se hizo clic.
func TestGuardarHorario_DeduplicaYOrdenaLosDias(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")

	out, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID, []mesonero.Franja{
		{Dias: []int{5, 1, 5, 3, 1}, Desde: "18:00", Hasta: "23:00"},
	})
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if len(out.Franjas) != 1 {
		t.Fatalf("se esperaba 1 tramo, hay %d", len(out.Franjas))
	}
	quiero := []int{1, 3, 5}
	got := out.Franjas[0].Dias
	if len(got) != len(quiero) {
		t.Fatalf("días = %v, se esperaban %v (sin repetir)", got, quiero)
	}
	for i := range quiero {
		if got[i] != quiero[i] {
			t.Fatalf("días = %v, se esperaban %v (sin repetir y ordenados)", got, quiero)
		}
	}
}

// Varios tramos el mismo día (almuerzo y cena) es lo normal en un restaurante:
// guardar uno solo dejaría media jornada sin horario.
func TestGuardarHorario_GuardaVariosTramos(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")

	out, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID, []mesonero.Franja{
		{Dias: []int{1, 2, 3}, Desde: "11:00", Hasta: "15:00"},
		{Dias: []int{1, 2, 3}, Desde: "18:00", Hasta: "23:00"},
	})
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if len(out.Franjas) != 2 {
		t.Fatalf("se esperaban 2 tramos, hay %d", len(out.Franjas))
	}
	if svc.HorarioDe(empSalon, m.ID).SinHorario() {
		t.Error("el horario guardado tiene que poder releerse")
	}
}

// «Sin horario» es un estado válido y tiene que poder volverse a él: si no, una
// persona que pasa a turnos libres arrastraría para siempre el horario viejo.
func TestGuardarHorario_SinFranjasBorraElHorario(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	conHorario(t, svc, m, 3)
	if svc.HorarioDe(empSalon, m.ID).SinHorario() {
		t.Fatal("el horario debería haber quedado guardado")
	}

	out, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID, nil)
	if err != nil {
		t.Fatalf("borrar: %v", err)
	}
	if len(out.Franjas) != 0 {
		t.Errorf("al borrar no puede quedar ninguna franja, quedaron %v", out.Franjas)
	}
	if h := svc.HorarioDe(empSalon, m.ID); !h.SinHorario() {
		t.Errorf("el horario debía quedar borrado, quedó %+v", h.Franjas)
	}
	if len(svc.Horarios(empSalon, sedeSalon)) != 0 {
		t.Error("el horario borrado no debe seguir apareciendo en la lista de la sede")
	}
}

func TestGuardarHorario_MesoneroInexistente(t *testing.T) {
	svc, _ := servicioHorarios(t)
	if _, err := svc.GuardarHorario(empSalon, actorA, origenTst, "msn_no_existe",
		[]mesonero.Franja{franjaQueCubreAhora(3)}); !errors.Is(err, application.ErrMesoneroNoExiste) {
		t.Fatalf("se esperaba ErrMesoneroNoExiste, se obtuvo: %v", err)
	}
}

/* --- El fin previsto al abrir el turno ------------------------------------- */

func TestIniciarTurno_DentroDelHorarioCalculaElFinYNoMarca(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	conHorario(t, svc, m, 3)

	tu := enTurno(t, svc, m, "4455")
	if tu.FueraDeHorario {
		t.Error("entró dentro de su tramo: no debería quedar marcado fuera de horario")
	}
	fin, err := time.Parse(time.RFC3339, tu.FinPrevisto)
	if err != nil {
		t.Fatalf("FinPrevisto no es una fecha válida (%q): %v", tu.FinPrevisto, err)
	}
	// El tramo se armó para terminar 3 horas después, con precisión de minuto.
	esperado := time.Now().UTC().Add(3 * time.Hour)
	if d := fin.Sub(esperado); d > 2*time.Minute || d < -2*time.Minute {
		t.Errorf("fin previsto %v, se esperaba cerca de %v (desfase %v)", fin, esperado, d)
	}
}

// Entrar un día que no le toca NO se bloquea —el supervisor ya autorizó— pero
// tiene que quedar registrado: esa marca es de lo único que sirve tener horarios
// cuando el modo es «aviso».
func TestIniciarTurno_MarcaLaEntradaFueraDeHorario(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	if _, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID,
		[]mesonero.Franja{franjaDeUnDiaQueNoTrabaja()}); err != nil {
		t.Fatalf("guardar horario: %v", err)
	}

	tu := enTurno(t, svc, m, "4455")
	if !tu.FueraDeHorario {
		t.Error("entró un día que no trabaja: debía quedar marcado fuera de horario")
	}
	if tu.FinPrevisto != "" {
		t.Errorf("sin tramo aplicable no hay hora de salida que calcular, dio %q", tu.FinPrevisto)
	}
}

// Sin horario declarado no hay nada que vencer, y tampoco se puede estar FUERA
// de un horario que no existe: marcarlo sería acusar a alguien de nada.
func TestIniciarTurno_SinHorarioNoHayFinNiMarca(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")

	tu := enTurno(t, svc, m, "4455")
	if tu.FinPrevisto != "" {
		t.Errorf("sin horario no debe haber fin previsto, dio %q", tu.FinPrevisto)
	}
	if tu.FueraDeHorario {
		t.Error("sin horario declarado nadie puede estar «fuera de horario»")
	}
}

/* --- Vencimiento ----------------------------------------------------------- */

// LA prueba del lote. El vencimiento se evalúa de forma perezosa; si solo
// corriera al pintar la pantalla de turnos, un mesonero vencido seguiría tomando
// mesas toda la noche mientras nadie abriera esa pantalla. Acá se llega al
// candado SIN pasar antes por TurnosVivos ni ListarMesoneros.
func TestVencimiento_ElVencidoNoTomaMesasAunqueNadieMireLaPantalla(t *testing.T) {
	svc, st := servicioHorarios(t)
	cierraSolo(t, svc)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	conHorario(t, svc, m, 3)
	tu := enTurno(t, svc, m, "4455")
	vencerTurno(t, st, tu.ID, 30*time.Minute)

	if _, err := abrirMesa(t, svc, st, m, "2", 2); !errors.Is(err, application.ErrTurnoCerrandoSinMesas) {
		t.Fatalf("con el horario vencido no debería tomar mesas nuevas, se obtuvo: %v", err)
	}
	got := turnoDe(t, st, tu.ID)
	if got.Estado != mesonero.EstadoCerrando {
		t.Fatalf("debería haber entrado en cierre suave, está %q", got.Estado)
	}
	// El motivo no es decorativo: es lo que decide si el tiempo extra puede
	// revertir este cierre.
	if got.CerrandoMotivo != mesonero.CerrandoPorHorario {
		t.Errorf("el motivo debería ser %q, es %q", mesonero.CerrandoPorHorario, got.CerrandoMotivo)
	}
	if got.CerrandoDesde == "" {
		t.Error("un turno en cierre suave tiene que decir desde cuándo lo está")
	}
}

// En modo AVISO (el de por defecto) no pasa nada automático: pasar la hora no
// puede trancar a nadie en medio del servicio.
func TestVencimiento_EnModoAvisoNoCierraNada(t *testing.T) {
	svc, st := servicioHorarios(t) // el modo por defecto es «aviso»
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	conHorario(t, svc, m, 3)
	tu := enTurno(t, svc, m, "4455")
	vencerTurno(t, st, tu.ID, time.Hour)

	svc.TurnosVivos(empSalon, sedeSalon)
	if _, err := abrirMesa(t, svc, st, m, "2", 2); err != nil {
		t.Fatalf("en modo aviso debe poder seguir trabajando: %v", err)
	}
	if got := turnoDe(t, st, tu.ID); got.Estado != mesonero.EstadoAbierto {
		t.Errorf("en modo aviso el turno sigue abierto, quedó %q", got.Estado)
	}
}

// Un turno VIGENTE no se toca ni en cierre automático. Sin esta prueba, un error
// de signo en la comparación cerraría a todo el mundo al abrir la pantalla — y
// las otras pruebas, que parten de un turno ya vencido, pasarían sin notarlo.
func TestVencimiento_NoTocaAlTurnoVigente(t *testing.T) {
	svc, st := servicioHorarios(t)
	cierraSolo(t, svc)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	conHorario(t, svc, m, 3)
	tu := enTurno(t, svc, m, "4455")
	// Sin esto la prueba pasaría en vacío: un turno SIN hora de salida tampoco
	// vence, así que no estaría comprobando la comparación sino su ausencia.
	if tu.FinPrevisto == "" {
		t.Fatal("preparación: el turno tenía que salir con hora de salida para que haya algo que comparar")
	}

	svc.TurnosVivos(empSalon, sedeSalon)
	svc.ListarMesoneros(empSalon, sedeSalon)
	if got := turnoDe(t, st, tu.ID); got.Estado != mesonero.EstadoAbierto {
		t.Fatalf("un turno vigente debe seguir abierto, quedó %q", got.Estado)
	}
	if _, err := abrirMesa(t, svc, st, m, "2", 2); err != nil {
		t.Errorf("un turno vigente debe poder tomar mesas: %v", err)
	}
}

// Sin horario declarado no hay hora de salida, así que no hay nada que vencer
// por más que la sede esté en cierre automático.
func TestVencimiento_SinHorarioNoVenceNunca(t *testing.T) {
	svc, st := servicioHorarios(t)
	cierraSolo(t, svc)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")

	svc.TurnosVivos(empSalon, sedeSalon)
	if got := turnoDe(t, st, tu.ID); got.Estado != mesonero.EstadoAbierto {
		t.Errorf("sin horario el turno no puede vencer, quedó %q", got.Estado)
	}
}

/* --- Tiempo extra ---------------------------------------------------------- */

// El tiempo extra se paga: no lo puede dar cualquiera. Y una extensión rechazada
// no puede dejar rastro, o se termina pagando algo que nadie autorizó.
func TestExtenderTurno_ExigeSupervisorYElRechazoNoDejaRastro(t *testing.T) {
	svc, st := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")

	if _, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, 30, "0000"); !errors.Is(err, application.ErrPinSupervisorInvalido) {
		t.Fatalf("se esperaba ErrPinSupervisorInvalido, se obtuvo: %v", err)
	}
	got := turnoDe(t, st, tu.ID)
	if got.ExtensionMinutos != 0 || got.ExtensionPor != "" {
		t.Errorf("una extensión rechazada no puede quedar registrada: %+v", got)
	}
}

// Un dedo de más al teclear («600» en vez de «60») dejaría un turno abierto diez
// horas y el vencimiento no lo rescataría.
func TestExtenderTurno_AcotaLosMinutos(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")

	for _, min := range []int{0, -30, 481, 10000} {
		if _, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, min, pinSup); !errors.Is(err, application.ErrExtensionInvalida) {
			t.Errorf("%d minutos deberían rechazarse, se obtuvo: %v", min, err)
		}
	}
	// El tope exacto sí se admite: 480 son las ocho horas de una jornada.
	if _, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, 480, pinSup); err != nil {
		t.Errorf("480 minutos es el máximo admitido, no debería fallar: %v", err)
	}
}

// Las extensiones se ACUMULAN: dos medias horas son una hora de tiempo extra, y
// eso es lo que hay que poder responder cuando se pague. Guardar solo la última
// haría desaparecer el resto.
func TestExtenderTurno_AcumulaLosMinutosYRegistraQuienAutorizo(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")

	if _, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, 30, pinSup); err != nil {
		t.Fatalf("extender: %v", err)
	}
	out, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, 45, pinSup)
	if err != nil {
		t.Fatalf("extender de nuevo: %v", err)
	}
	if out.ExtensionMinutos != 75 {
		t.Errorf("las extensiones se acumulan: se esperaban 75 minutos, hay %d", out.ExtensionMinutos)
	}
	if out.ExtensionPor == "" {
		t.Error("tiene que quedar registrado quién autorizó el tiempo extra")
	}
	// Y la segunda extensión parte de la hora YA extendida, no de cero.
	fin, err := time.Parse(time.RFC3339, out.FinPrevisto)
	if err != nil {
		t.Fatalf("FinPrevisto inválido (%q): %v", out.FinPrevisto, err)
	}
	if !fin.After(time.Now().UTC().Add(70 * time.Minute)) {
		t.Errorf("tras +30 y +45 el fin debería pasar de 70 min desde ahora, es %v", fin)
	}
}

// Aprobar tiempo extra a quien el HORARIO mandó a cerrar es justamente lo que
// significa el tiempo extra: vuelve a trabajar con normalidad.
func TestExtenderTurno_ReabreElQueVencioPorHorario(t *testing.T) {
	svc, st := servicioHorarios(t)
	cierraSolo(t, svc)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	conHorario(t, svc, m, 3)
	tu := enTurno(t, svc, m, "4455")
	vencerTurno(t, st, tu.ID, 30*time.Minute)
	svc.TurnosVivos(empSalon, sedeSalon) // dispara el vencimiento
	if got := turnoDe(t, st, tu.ID); got.Estado != mesonero.EstadoCerrando {
		t.Fatalf("preparación: el turno debía estar cerrando, está %q", got.Estado)
	}

	out, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, 60, pinSup)
	if err != nil {
		t.Fatalf("extender: %v", err)
	}
	if out.Estado != mesonero.EstadoAbierto {
		t.Fatalf("el tiempo extra debe devolverlo a abierto, quedó %q", out.Estado)
	}
	if out.CerrandoDesde != "" || out.CerrandoMotivo != "" {
		t.Errorf("al reabrir no pueden quedar rastros del cierre: desde=%q motivo=%q", out.CerrandoDesde, out.CerrandoMotivo)
	}
	// Y vuelve a poder tomar mesas, que es el punto de todo esto.
	if _, err := abrirMesa(t, svc, st, m, "2", 2); err != nil {
		t.Errorf("tras el tiempo extra debería poder tomar una mesa: %v", err)
	}
}

// Pero extender NO puede deshacer en silencio la decisión de una PERSONA. Si el
// supervisor dijo «termina», darle tiempo extra no lo reabre: sería una puerta
// trasera para ignorar una orden.
func TestExtenderTurno_NoRevierteElCierreQueOrdenoUnaPersona(t *testing.T) {
	svc, st := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")
	// Con una mesa abierta, terminar el turno lo deja en cierre SUAVE (si no, se
	// cerraría de golpe y ya no habría nada que extender).
	if _, err := abrirMesa(t, svc, st, m, "2", 2); err != nil {
		t.Fatalf("abrir mesa: %v", err)
	}
	if _, err := svc.FinalizarTurno(empSalon, actorA, origenTst, tu.ID); err != nil {
		t.Fatalf("finalizar: %v", err)
	}
	if got := turnoDe(t, st, tu.ID); got.CerrandoMotivo != mesonero.CerrandoPorSupervisor {
		t.Fatalf("preparación: el motivo debía ser %q, es %q", mesonero.CerrandoPorSupervisor, got.CerrandoMotivo)
	}

	out, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, 60, pinSup)
	if err != nil {
		t.Fatalf("extender: %v", err)
	}
	if out.Estado != mesonero.EstadoCerrando {
		t.Errorf("un cierre ordenado por una persona no se revierte con tiempo extra, quedó %q", out.Estado)
	}
	if out.CerrandoMotivo != mesonero.CerrandoPorSupervisor {
		t.Errorf("el motivo del cierre no puede cambiar, es %q", out.CerrandoMotivo)
	}
	// Y sigue sin poder tomar mesas nuevas.
	if _, err := abrirMesa(t, svc, st, m, "3", 2); !errors.Is(err, application.ErrTurnoCerrandoSinMesas) {
		t.Errorf("en cerrando no toma mesas nuevas: se obtuvo %v", err)
	}
}

// Extender 30 minutos un turno que venció hace dos horas no puede dejarlo
// vencido igual: la base es AHORA, no una hora de salida que ya pasó. Si esto
// falla, el tiempo extra «se da» y el turno se vuelve a cerrar en el acto.
func TestExtenderTurno_AlVencidoHaceRatoLoDejaVigente(t *testing.T) {
	svc, st := servicioHorarios(t)
	cierraSolo(t, svc)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	conHorario(t, svc, m, 3)
	tu := enTurno(t, svc, m, "4455")
	vencerTurno(t, st, tu.ID, 2*time.Hour)

	out, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, 30, pinSup)
	if err != nil {
		t.Fatalf("extender: %v", err)
	}
	fin, err := time.Parse(time.RFC3339, out.FinPrevisto)
	if err != nil {
		t.Fatalf("FinPrevisto inválido (%q): %v", out.FinPrevisto, err)
	}
	if !fin.After(time.Now().UTC()) {
		t.Fatalf("tras el tiempo extra el turno tiene que quedar vigente, vence en %v", fin)
	}
	// Y no se vuelve a cerrar en cuanto alguien mire la pantalla.
	svc.TurnosVivos(empSalon, sedeSalon)
	if got := turnoDe(t, st, tu.ID); got.Estado != mesonero.EstadoAbierto {
		t.Errorf("el turno extendido debe seguir abierto, quedó %q", got.Estado)
	}
	if _, err := abrirMesa(t, svc, st, m, "2", 2); err != nil {
		t.Errorf("tras el tiempo extra debería poder tomar una mesa: %v", err)
	}
}

// Un turno ya cerrado es historia: extenderlo lo resucitaría y su resumen
// congelado dejaría de cuadrar con lo que pasó después.
func TestExtenderTurno_NoSeExtiendeUnTurnoCerrado(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")
	if _, err := svc.FinalizarTurno(empSalon, actorA, origenTst, tu.ID); err != nil {
		t.Fatalf("finalizar: %v", err)
	}

	if _, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, 30, pinSup); !errors.Is(err, application.ErrTurnoNoVivo) {
		t.Fatalf("se esperaba ErrTurnoNoVivo, se obtuvo: %v", err)
	}
}

func TestExtenderTurno_TurnoInexistente(t *testing.T) {
	svc, _ := servicioHorarios(t)
	if _, err := svc.ExtenderTurno(empSalon, actorA, origenTst, "trn_no_existe", 30, pinSup); !errors.Is(err, application.ErrTurnoNoExiste) {
		t.Fatalf("se esperaba ErrTurnoNoExiste, se obtuvo: %v", err)
	}
}

/* --- Configuración de la sede ---------------------------------------------- */

// Los dos ajustes viven en la MISMA ficha y el guardado la reemplaza entera: sin
// cuidado, tocar uno borra el otro en silencio.
func TestConfigSalon_CambiarElModoNoApagaLaAsignacionEstricta(t *testing.T) {
	svc, _ := servicioHorarios(t)
	si := true
	if _, err := svc.GuardarConfigSalonCompleta(empSalon, sedeSalon, actorA, origenTst, &si, nil); err != nil {
		t.Fatalf("activar la asignación estricta: %v", err)
	}
	cierraSolo(t, svc)

	cfg := svc.ConfigSalon(empSalon, sedeSalon)
	if !cfg.AsignacionEstricta {
		t.Error("cambiar el modo de horario no puede apagar la asignación estricta")
	}
	if svc.ModoHorario(empSalon, sedeSalon) != mesonero.HorarioCierraSolo {
		t.Errorf("el modo debía quedar en cierre automático, es %q", svc.ModoHorario(empSalon, sedeSalon))
	}
}

func TestConfigSalon_GuardarLaAsignacionNoPisaElModoDeHorario(t *testing.T) {
	svc, _ := servicioHorarios(t)
	cierraSolo(t, svc)
	si := true
	if _, err := svc.GuardarConfigSalonCompleta(empSalon, sedeSalon, actorA, origenTst, &si, nil); err != nil {
		t.Fatalf("activar la asignación estricta: %v", err)
	}

	if got := svc.ModoHorario(empSalon, sedeSalon); got != mesonero.HorarioCierraSolo {
		t.Errorf("guardar la asignación no puede pisar el modo de horario, quedó %q", got)
	}
}

/* --- Aislamiento por empresa ----------------------------------------------- */

// Última línea de defensa: Mongo no tiene Row-Level Security, así que el filtro
// por empresa tiene que sostenerse solo en CADA operación, no en la de al lado.
func TestHorariosYTiempoExtra_AisladosPorEmpresa(t *testing.T) {
	svc, st := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	conHorario(t, svc, m, 3)
	tu := enTurno(t, svc, m, "4455")

	// Otra empresa no puede fijarle el horario a un mesonero ajeno…
	if _, err := svc.GuardarHorario(empDemo, actorA, origenTst, m.ID,
		[]mesonero.Franja{franjaQueCubreAhora(3)}); !errors.Is(err, application.ErrMesoneroNoExiste) {
		t.Errorf("se esperaba ErrMesoneroNoExiste, se obtuvo: %v", err)
	}
	// …ni verlo…
	if got := svc.Horarios(empDemo, ""); len(got) != 0 {
		t.Errorf("otra empresa no debería ver horarios ajenos, ve %d", len(got))
	}
	if h := svc.HorarioDe(empDemo, m.ID); !h.SinHorario() {
		t.Errorf("otra empresa no debería resolver el horario ajeno, obtuvo %+v", h.Franjas)
	}
	// …ni darle tiempo extra a su turno.
	if _, err := svc.ExtenderTurno(empDemo, actorA, origenTst, tu.ID, 30, pinSup); !errors.Is(err, application.ErrTurnoNoExiste) {
		t.Errorf("se esperaba ErrTurnoNoExiste, se obtuvo: %v", err)
	}
	if got := turnoDe(t, st, tu.ID); got.ExtensionMinutos != 0 {
		t.Errorf("el turno ajeno no puede haber quedado extendido: %+v", got)
	}
	// Y el horario propio sigue intacto tras todos los intentos ajenos.
	if svc.HorarioDe(empSalon, m.ID).SinHorario() {
		t.Error("el horario de la empresa dueña no debió verse afectado")
	}
}
