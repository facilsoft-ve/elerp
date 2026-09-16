package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/mesonero"
)

/* Horarios, vencimiento y tiempo extra — los bordes.
 *
 * El núcleo (validación de tramos, vencimiento en sus dos modos, reapertura por
 * tiempo extra) vive en mesonero_test.go. Acá quedan los casos que se le escapan
 * y que son justamente donde este tipo de código se rompe: la normalización de
 * lo que teclea quien configura, la marca de quien entró fuera de su turno, la
 * acumulación del tiempo extra —que es lo que después se paga— y el aislamiento
 * entre empresas.
 *
 * Reusa servicioHorarios, cierraSolo y todosLosDias de mesonero_test.go. */

// zonaVET es la hora legal de Venezuela. El horario se declara en hora de PARED
// ("18:00"), así que los tramos hay que armarlos contra el reloj local o la
// prueba pasaría o fallaría según la hora a la que se corra.
var zonaVET = time.FixedZone("VET", -4*60*60)

/* franjaQueCubreAhora arma un tramo que contiene el instante actual y termina
 * `horas` más adelante, sea la hora que sea. Cubre TODOS los días por el mismo
 * motivo que todosLosDias: una prueba que solo falla los martes es peor que no
 * tenerla. Con la semana entera, un tramo que cruza medianoche también encuentra
 * su día de arranque (a las 00:30 el tramo vigente es el que abrió ayer). */
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

/* --- Normalización de lo que se teclea ------------------------------------- */

// Los días llegan como los toque quien configura: repetidos y en cualquier
// orden. Guardarlos así haría que dos horarios idénticos se vean distintos, y
// que comparar o mostrar un horario dependa del orden en que se hizo clic.
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

/* --- La marca de quien entró fuera de su turno ----------------------------- */

// Entrar un día que no le toca NO se bloquea —el supervisor ya autorizó— pero
// tiene que quedar registrado: esa marca es de lo único que sirve tener
// horarios cuando el modo es «aviso».
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

// Y el caso contrario: dentro de su tramo, ni marca ni sorpresas — y con la hora
// de salida bien calculada, que es la base de todo el vencimiento.
func TestIniciarTurno_DentroDelHorarioNoMarcaYCalculaElFin(t *testing.T) {
	svc, _ := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	if _, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID,
		[]mesonero.Franja{franjaQueCubreAhora(3)}); err != nil {
		t.Fatalf("guardar horario: %v", err)
	}

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

/* --- Vencimiento: el que todavía NO venció --------------------------------- */

// Un turno VIGENTE no se toca ni en cierre automático. Sin esta prueba, un error
// de signo en la comparación cerraría a todo el mundo al abrir la pantalla — y
// las otras pruebas de vencimiento, que parten de un turno ya vencido, pasarían
// igual sin notarlo.
func TestVencimiento_NoTocaAlTurnoVigente(t *testing.T) {
	svc, st := servicioHorarios(t)
	cierraSolo(t, svc)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	if _, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID,
		[]mesonero.Franja{franjaQueCubreAhora(3)}); err != nil {
		t.Fatalf("guardar horario: %v", err)
	}
	tu := enTurno(t, svc, m, "4455")

	svc.TurnosVivos(empSalon, sedeSalon)
	svc.ListarMesoneros(empSalon, sedeSalon)
	if got := turnoDe(t, st, tu.ID); got.Estado != mesonero.EstadoAbierto {
		t.Fatalf("un turno vigente debe seguir abierto, quedó %q", got.Estado)
	}
	if _, err := abrirMesa(t, svc, st, m, "2", 2); err != nil {
		t.Errorf("un turno vigente debe poder tomar mesas: %v", err)
	}
}

/* --- Tiempo extra: lo que después se paga ---------------------------------- */

// Las extensiones se ACUMULAN: dos medias horas son una hora de tiempo extra, y
// eso es exactamente lo que hay que poder responder cuando se pague. Guardar
// solo la última haría desaparecer el resto.
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

// Una extensión rechazada no puede dejar rastro: si el PIN falla pero los
// minutos ya se sumaron, se termina pagando un tiempo extra que nadie autorizó.
func TestExtenderTurno_LaExtensionRechazadaNoDejaRastro(t *testing.T) {
	svc, st := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")

	if _, err := svc.ExtenderTurno(empSalon, actorA, origenTst, tu.ID, 30, "0000"); err == nil {
		t.Fatal("con PIN de supervisor incorrecto no debería extender")
	}
	got := turnoDe(t, st, tu.ID)
	if got.ExtensionMinutos != 0 || got.ExtensionPor != "" {
		t.Errorf("una extensión rechazada no puede quedar registrada: %+v", got)
	}
}

// Extender 30 minutos un turno que venció hace dos horas no puede dejarlo
// vencido igual: la base es AHORA, no una hora de salida que ya pasó. Si esto
// falla, el tiempo extra «se da» y el turno se vuelve a cerrar en el acto.
func TestExtenderTurno_AlVencidoHaceRatoLoDejaVigente(t *testing.T) {
	svc, st := servicioHorarios(t)
	cierraSolo(t, svc)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	if _, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID,
		[]mesonero.Franja{franjaQueCubreAhora(3)}); err != nil {
		t.Fatalf("guardar horario: %v", err)
	}
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

func TestExtenderTurno_TurnoInexistente(t *testing.T) {
	svc, _ := servicioHorarios(t)
	if _, err := svc.ExtenderTurno(empSalon, actorA, origenTst, "trn_no_existe", 30, pinSup); !errors.Is(err, application.ErrTurnoNoExiste) {
		t.Fatalf("se esperaba ErrTurnoNoExiste, se obtuvo: %v", err)
	}
}

/* --- Aislamiento por empresa ----------------------------------------------- */

// Última línea de defensa: Mongo no tiene Row-Level Security, así que el filtro
// por empresa tiene que sostenerse solo en CADA operación, no en la de al lado.
func TestHorariosYTiempoExtra_AisladosPorEmpresa(t *testing.T) {
	svc, st := servicioHorarios(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	if _, err := svc.GuardarHorario(empSalon, actorA, origenTst, m.ID,
		[]mesonero.Franja{franjaQueCubreAhora(3)}); err != nil {
		t.Fatalf("guardar horario: %v", err)
	}
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
