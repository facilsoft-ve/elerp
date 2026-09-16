package application_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/mesonero"
	"github.com/mornix/elerp/internal/domain/usuario"
)

/* Turnos del salón.
 *
 * Lo que de verdad se prueba acá no es el CRUD: es que el PIN por sí solo NO
 * sirva. Un mesonero que se sabe su PIN, a las 3 de la mañana, desde su casa, no
 * puede ponerse a trabajar — y eso tiene que fallar en el servidor, no en una
 * pantalla que oculta un botón. */

// pinSup es el PIN del supervisor sembrado (OP-002) del restaurante demo.
const pinSup = inmem.PinDemo

// servicioTurnos monta el salón con credenciales de mesonero y turnos.
func servicioTurnos(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := servicioComanderas(t)
	svc.ConMesoneros(st.Mesoneros, st.Turnos)
	return svc, st
}

// mesoneroListo da de alta una credencial y le fija su PIN, que es el estado
// desde el que arranca casi toda prueba.
func mesoneroListo(t *testing.T, svc *application.Service, nombre, usuarioID, pin string) mesonero.Mesonero {
	t.Helper()
	m, err := svc.CrearMesonero(empSalon, actorA, origenTst, sedeSalon, nombre, usuarioID)
	if err != nil {
		t.Fatalf("crear mesonero %s: %v", nombre, err)
	}
	if err := svc.FijarPinMesonero(empSalon, actorA, origenTst, m.ID, pin, pinSup, false); err != nil {
		t.Fatalf("fijar PIN de %s: %v", nombre, err)
	}
	return m
}

// enTurno deja al mesonero trabajando.
func enTurno(t *testing.T, svc *application.Service, m mesonero.Mesonero, pin string) mesonero.Turno {
	t.Helper()
	tu, err := svc.IniciarTurno(empSalon, m.UsuarioID, origenTst, m.ID, pin, pinSup)
	if err != nil {
		t.Fatalf("iniciar turno de %s: %v", m.Nombre, err)
	}
	return tu
}

// abrirMesa abre la cuenta de una mesa del restaurante demo a nombre del mesonero.
func abrirMesa(t *testing.T, svc *application.Service, st *inmem.Store, m mesonero.Mesonero, nombreMesa string, comensales int) (string, error) {
	t.Helper()
	mesa := mesaPorNombre(t, st, nombreMesa)
	c, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: mesa.ID,
		MesoneroID: m.UsuarioID, MesoneroNombre: m.Nombre, RolActor: usuario.RolMesonero,
		Actor: m.UsuarioID, Origen: origenTst, Comensales: comensales,
	})
	// AbrirCuenta devuelve la cuenta YA abierta si la mesa está ocupada. La demo
	// del restaurante deja ocupadas las mesas 1, 4 y 5: si una prueba eligiera una
	// de esas, pasaría por el camino equivocado sin decir nada.
	if err == nil && c.MesoneroID != m.UsuarioID {
		t.Fatalf("la mesa %s ya estaba ocupada por otro: la prueba necesita una libre", nombreMesa)
	}
	return c.ID, err
}

/* --- El candado: sin turno no se trabaja ---------------------------------- */

// LA prueba de todo esto. La credencial es válida y el PIN es correcto, pero
// nadie le validó el turno: no puede tomar una mesa.
func TestTurno_SinTurnoElMesoneroNoTomaMesas(t *testing.T) {
	svc, st := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")

	_, err := abrirMesa(t, svc, st, m, "2", 2)
	if !errors.Is(err, application.ErrSinTurnoAbierto) {
		t.Fatalf("sin turno se esperaba ErrSinTurnoAbierto, se obtuvo: %v", err)
	}
}

func TestTurno_ConTurnoAbiertoSiTomaMesas(t *testing.T) {
	svc, st := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	enTurno(t, svc, m, "4455")

	if _, err := abrirMesa(t, svc, st, m, "2", 2); err != nil {
		t.Fatalf("con turno abierto debería poder: %v", err)
	}
}

// El candado es SOLO para el mesonero: la dueña y el cajero abren mesas sin
// turno. Si esto se rompe, el restaurante no puede operar cuando el encargado
// atiende una mesa él mismo.
func TestTurno_LaDuenaAbreMesaSinTurno(t *testing.T) {
	svc, st := servicioTurnos(t)
	mesa := mesaPorNombre(t, st, "2")
	_, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: mesa.ID,
		MesoneroID: actorA, MesoneroNombre: "Dueña", RolActor: usuario.RolDueno,
		Actor: actorA, Origen: origenTst, Comensales: 2,
	})
	if err != nil {
		t.Fatalf("la dueña no necesita turno: %v", err)
	}
}

/* --- Iniciar turno -------------------------------------------------------- */

func TestIniciarTurno_PinDelMesoneroIncorrecto(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")

	_, err := svc.IniciarTurno(empSalon, actorA, origenTst, m.ID, "0000", pinSup)
	if !errors.Is(err, application.ErrPinMesoneroInvalido) {
		t.Fatalf("se esperaba ErrPinMesoneroInvalido, se obtuvo: %v", err)
	}
}

// El PIN propio no basta: hace falta que un supervisor lo valide. Esta es la
// mitad del candado que impide auto-asignarse un turno.
func TestIniciarTurno_SinSupervisorNoAbre(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")

	_, err := svc.IniciarTurno(empSalon, actorA, origenTst, m.ID, "4455", "0000")
	if !errors.Is(err, application.ErrPinSupervisorInvalido) {
		t.Fatalf("sin PIN de supervisor válido se esperaba ErrPinSupervisorInvalido, se obtuvo: %v", err)
	}
}

func TestIniciarTurno_SinPinFijadoNoAbre(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m, err := svc.CrearMesonero(empSalon, actorA, origenTst, sedeSalon, "Luis", "usr_luis")
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := svc.IniciarTurno(empSalon, actorA, origenTst, m.ID, "4455", pinSup); !errors.Is(err, application.ErrMesoneroSinPin) {
		t.Fatalf("se esperaba ErrMesoneroSinPin, se obtuvo: %v", err)
	}
}

// Dos turnos a la vez partirían sus mesas y su resumen en dos, y ninguno de los
// dos sería cierto.
func TestIniciarTurno_NoSePuedeAbrirDosVeces(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	enTurno(t, svc, m, "4455")

	if _, err := svc.IniciarTurno(empSalon, actorA, origenTst, m.ID, "4455", pinSup); !errors.Is(err, application.ErrTurnoYaAbierto) {
		t.Fatalf("se esperaba ErrTurnoYaAbierto, se obtuvo: %v", err)
	}
}

func TestIniciarTurno_MesoneroInactivoNoAbre(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	no := false
	if _, err := svc.ActualizarMesonero(empSalon, actorA, origenTst, m.ID, application.CambiosMesonero{Activo: &no}); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if _, err := svc.IniciarTurno(empSalon, actorA, origenTst, m.ID, "4455", pinSup); !errors.Is(err, application.ErrMesoneroInactivo) {
		t.Fatalf("se esperaba ErrMesoneroInactivo, se obtuvo: %v", err)
	}
}

/* --- El PIN lo elige su dueño --------------------------------------------- */

// Fijar el PIN reemplaza al enlace por correo: la prueba de que es la persona
// correcta NO es un canal privado, es que hay un supervisor al lado. Por eso sin
// su autorización no se fija nada.
func TestFijarPin_ExigeSupervisor(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m, _ := svc.CrearMesonero(empSalon, actorA, origenTst, sedeSalon, "Luis", "usr_luis")

	if err := svc.FijarPinMesonero(empSalon, actorA, origenTst, m.ID, "4455", "0000", false); !errors.Is(err, application.ErrPinSupervisorInvalido) {
		t.Fatalf("se esperaba ErrPinSupervisorInvalido, se obtuvo: %v", err)
	}
	// Y el PIN no quedó fijado a medias.
	if _, err := svc.IniciarTurno(empSalon, actorA, origenTst, m.ID, "4455", pinSup); !errors.Is(err, application.ErrMesoneroSinPin) {
		t.Fatalf("el PIN no debió quedar fijado: %v", err)
	}
}

// Sin esta regla, quien agarre la tablet le pisa el PIN a otro y trabaja como él.
func TestFijarPin_NoSePisaUnPinExistente(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")

	if err := svc.FijarPinMesonero(empSalon, actorA, origenTst, m.ID, "9999", pinSup, false); !errors.Is(err, application.ErrMesoneroYaTienePin) {
		t.Fatalf("se esperaba ErrMesoneroYaTienePin, se obtuvo: %v", err)
	}
	// Con reinicio explícito sí: es el PIN olvidado.
	if err := svc.FijarPinMesonero(empSalon, actorA, origenTst, m.ID, "9999", pinSup, true); err != nil {
		t.Fatalf("reiniciar debería poder: %v", err)
	}
	if _, err := svc.IniciarTurno(empSalon, actorA, origenTst, m.ID, "9999", pinSup); err != nil {
		t.Fatalf("el PIN nuevo debería servir: %v", err)
	}
}

func TestFijarPin_RechazaPinDebil(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m, _ := svc.CrearMesonero(empSalon, actorA, origenTst, sedeSalon, "Luis", "usr_luis")
	for _, pin := range []string{"", "12", "12345", "abcd"} {
		if err := svc.FijarPinMesonero(empSalon, actorA, origenTst, m.ID, pin, pinSup, false); err == nil {
			t.Errorf("el PIN %q debería rechazarse", pin)
		}
	}
}

/* --- Cierre suave: la parte que hace esto usable en un salón -------------- */

// Sin mesas encima, terminar el turno lo cierra en el acto: no tiene sentido
// dejarlo «cerrando» esperando algo que no va a pasar.
func TestFinalizarTurno_SinMesasCierraDirecto(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")

	out, err := svc.FinalizarTurno(empSalon, actorA, origenTst, tu.ID)
	if err != nil {
		t.Fatalf("finalizar: %v", err)
	}
	if out.Estado != mesonero.EstadoCerrado {
		t.Fatalf("sin mesas debería quedar cerrado, quedó %q", out.Estado)
	}
	if out.Resumen == nil {
		t.Fatal("un turno cerrado tiene que llevar su resumen congelado")
	}
}

// EL comportamiento que pidió el cliente: con mesas encima el turno NO se apaga.
// Pasa a cerrando, deja de tomar mesas nuevas y sigue atendiendo las suyas.
func TestFinalizarTurno_ConMesasPasaACerrandoYNoTomaMasMesas(t *testing.T) {
	svc, st := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")
	if _, err := abrirMesa(t, svc, st, m, "2", 2); err != nil {
		t.Fatalf("abrir mesa: %v", err)
	}

	out, err := svc.FinalizarTurno(empSalon, actorA, origenTst, tu.ID)
	if err != nil {
		t.Fatalf("finalizar: %v", err)
	}
	if out.Estado != mesonero.EstadoCerrando {
		t.Fatalf("con mesas abiertas debería quedar cerrando, quedó %q", out.Estado)
	}
	if out.Resumen != nil {
		t.Error("un turno que sigue vivo no puede tener el resumen congelado todavía")
	}
	// Ya no toma mesas NUEVAS…
	if _, err := abrirMesa(t, svc, st, m, "3", 2); !errors.Is(err, application.ErrTurnoCerrandoSinMesas) {
		t.Fatalf("en cerrando no debería tomar mesas nuevas, se obtuvo: %v", err)
	}
	// …pero SIGUE atendiendo la que tiene (volver a tocarla se la devuelve).
	if _, err := abrirMesa(t, svc, st, m, "2", 2); err != nil {
		t.Fatalf("en cerrando debe seguir atendiendo su mesa: %v", err)
	}
}

// La segunda mitad del cierre suave: el turno termina cuando se va el último
// cliente, no cuando alguien toca un botón.
func TestFinalizarTurno_SeCierraSoloAlCerrarLaUltimaMesa(t *testing.T) {
	svc, st := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")
	c1, _ := abrirMesa(t, svc, st, m, "2", 2)
	c2, _ := abrirMesa(t, svc, st, m, "3", 3)
	if _, err := svc.FinalizarTurno(empSalon, actorA, origenTst, tu.ID); err != nil {
		t.Fatalf("finalizar: %v", err)
	}

	// Con una mesa todavía abierta el turno sigue vivo.
	if _, err := svc.CerrarCuenta(empSalon, c1, m.UsuarioID, origenTst); err != nil {
		t.Fatalf("cerrar cuenta 1: %v", err)
	}
	if got, _ := svc.HistorialTurnos(empSalon, sedeSalon)[0], 0; got.Estado != mesonero.EstadoCerrando {
		t.Fatalf("con una mesa abierta el turno debe seguir cerrando, está %q", got.Estado)
	}

	// Al cerrar la última, el turno se cierra SOLO.
	if _, err := svc.CerrarCuenta(empSalon, c2, m.UsuarioID, origenTst); err != nil {
		t.Fatalf("cerrar cuenta 2: %v", err)
	}
	fin := svc.HistorialTurnos(empSalon, sedeSalon)[0]
	if fin.Estado != mesonero.EstadoCerrado {
		t.Fatalf("al cerrar la última mesa el turno debe cerrarse solo, está %q", fin.Estado)
	}
	if fin.CerradoPor != "" {
		t.Errorf("se cerró solo: no debería figurar quién lo cerró, dice %q", fin.CerradoPor)
	}
	if fin.Resumen == nil || fin.Resumen.Mesas != 2 {
		t.Fatalf("el resumen debería contar 2 mesas, es %+v", fin.Resumen)
	}
}

// Un turno abierto sin mesas NO se cierra solo porque otro cierre una cuenta:
// solo el que está en cerrando termina por esta vía.
func TestCierreAutomatico_NoTocaUnTurnoAbierto(t *testing.T) {
	svc, st := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	enTurno(t, svc, m, "4455")
	c, _ := abrirMesa(t, svc, st, m, "2", 2)

	if _, err := svc.CerrarCuenta(empSalon, c, m.UsuarioID, origenTst); err != nil {
		t.Fatalf("cerrar cuenta: %v", err)
	}
	if got := svc.HistorialTurnos(empSalon, sedeSalon)[0]; got.Estado != mesonero.EstadoAbierto {
		t.Fatalf("un turno abierto no se cierra por quedarse sin mesas, quedó %q", got.Estado)
	}
}

/* --- Cierre forzado y relevo ---------------------------------------------- */

// El mesonero se enfermó y se fue con mesas encima. Alguien TIENE que heredarlas
// o esas cuentas quedan sin quién las cobre.
func TestForzarCierre_ConMesasExigeRelevo(t *testing.T) {
	svc, st := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")
	abrirMesa(t, svc, st, m, "2", 2)

	if _, err := svc.CerrarTurnoForzado(empSalon, actorA, origenTst, tu.ID, "", pinSup); !errors.Is(err, application.ErrRelevoRequerido) {
		t.Fatalf("se esperaba ErrRelevoRequerido, se obtuvo: %v", err)
	}
}

func TestForzarCierre_TraspasaLasMesasYCierra(t *testing.T) {
	svc, st := servicioTurnos(t)
	luis := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	ana := mesoneroListo(t, svc, "Ana", "usr_ana", "6677")
	tuLuis := enTurno(t, svc, luis, "4455")
	enTurno(t, svc, ana, "6677")
	cid, _ := abrirMesa(t, svc, st, luis, "2", 4)

	out, err := svc.CerrarTurnoForzado(empSalon, actorA, origenTst, tuLuis.ID, ana.ID, pinSup)
	if err != nil {
		t.Fatalf("forzar cierre: %v", err)
	}
	if out.Estado != mesonero.EstadoCerrado {
		t.Fatalf("debería quedar cerrado, quedó %q", out.Estado)
	}
	if out.CerradoPor == "" {
		t.Error("un cierre forzado tiene que decir quién lo forzó")
	}
	// La mesa cambió de responsable…
	c, ok := st.Cuentas.ByID(empSalon, cid)
	if !ok {
		t.Fatal("la cuenta desapareció")
	}
	if c.MesoneroID != ana.UsuarioID {
		t.Errorf("la mesa debería haber pasado a Ana, está en %q", c.MesoneroID)
	}
	// …pero el SELLO del turno no se mueve: el resumen de Luis sigue contando la
	// mesa que él atendió.
	if c.TurnoID != tuLuis.ID {
		t.Errorf("el sello del turno no debe moverse con el relevo: %q", c.TurnoID)
	}
	if out.Resumen == nil || out.Resumen.Mesas != 1 {
		t.Fatalf("el resumen de Luis debería contar su mesa, es %+v", out.Resumen)
	}
}

// Pasarle las mesas a alguien que también se está yendo solo mueve el problema.
func TestForzarCierre_ElRelevoDebePoderTomarMesas(t *testing.T) {
	svc, st := servicioTurnos(t)
	luis := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	ana := mesoneroListo(t, svc, "Ana", "usr_ana", "6677")
	tuLuis := enTurno(t, svc, luis, "4455")
	tuAna := enTurno(t, svc, ana, "6677")
	abrirMesa(t, svc, st, luis, "2", 2)
	abrirMesa(t, svc, st, ana, "3", 2)
	// Ana entra en cerrando: ya no puede recibir nada.
	if _, err := svc.FinalizarTurno(empSalon, actorA, origenTst, tuAna.ID); err != nil {
		t.Fatalf("finalizar el de Ana: %v", err)
	}

	if _, err := svc.CerrarTurnoForzado(empSalon, actorA, origenTst, tuLuis.ID, ana.ID, pinSup); !errors.Is(err, application.ErrRelevoInvalido) {
		t.Fatalf("se esperaba ErrRelevoInvalido, se obtuvo: %v", err)
	}
}

func TestForzarCierre_SinMesasNoPideRelevo(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")

	if _, err := svc.CerrarTurnoForzado(empSalon, actorA, origenTst, tu.ID, "", pinSup); err != nil {
		t.Fatalf("sin mesas no hace falta relevo: %v", err)
	}
}

// La sugerencia es siempre quien menos carga tiene encima; el supervisor decide.
func TestCandidatosRelevo_SugiereAlQueMenosMesasTiene(t *testing.T) {
	svc, st := servicioTurnos(t)
	luis := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	ana := mesoneroListo(t, svc, "Ana", "usr_ana", "6677")
	beto := mesoneroListo(t, svc, "Beto", "usr_beto", "8899")
	tuLuis := enTurno(t, svc, luis, "4455")
	enTurno(t, svc, ana, "6677")
	enTurno(t, svc, beto, "8899")
	abrirMesa(t, svc, st, luis, "2", 2)
	abrirMesa(t, svc, st, ana, "3", 2)
	abrirMesa(t, svc, st, ana, "6", 2)
	abrirMesa(t, svc, st, beto, "7", 2)

	rel := svc.CandidatosRelevo(empSalon, tuLuis.ID)
	if len(rel) != 2 {
		t.Fatalf("debería haber 2 candidatos (Ana y Beto), hay %d", len(rel))
	}
	if rel[0].Nombre != "Beto" || rel[0].MesasAbiertas != 1 {
		t.Errorf("el primero debería ser Beto con 1 mesa, es %+v", rel[0])
	}
	if !rel[0].Sugerido {
		t.Error("el primero tiene que venir marcado como sugerido")
	}
	if rel[1].Sugerido {
		t.Error("solo el primero se sugiere")
	}
	// El propio turno nunca es candidato de sí mismo.
	for _, r := range rel {
		if r.MesoneroID == luis.ID {
			t.Error("el turno que se cierra no puede ser su propio relevo")
		}
	}
}

/* --- El resumen de la jornada --------------------------------------------- */

// Lo que pidió el cliente: tiempo, mesas, personas, órdenes y ticket. Las cifras
// se DERIVAN de las cuentas del turno, nunca de contadores.
func TestResumenTurno_CuentaMesasPersonasOrdenesYTicket(t *testing.T) {
	svc, st := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")

	c1, _ := abrirMesa(t, svc, st, m, "2", 2)
	if _, err := svc.AgregarItems(empSalon, c1, m.UsuarioID, usuario.RolMesonero, origenTst,
		[]application.ItemInput{{SKU: "CAF-001", Nombre: "Café", Cantidad: 2, PrecioUnitario: 100}}); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	if _, _, err := svc.EnviarACocina(empSalon, c1, m.UsuarioID, origenTst); err != nil {
		t.Fatalf("enviar a cocina: %v", err)
	}
	c2, _ := abrirMesa(t, svc, st, m, "3", 3)
	if _, err := svc.AgregarItems(empSalon, c2, m.UsuarioID, usuario.RolMesonero, origenTst,
		[]application.ItemInput{{SKU: "CAF-001", Nombre: "Café", Cantidad: 4, PrecioUnitario: 100}}); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	if _, _, err := svc.EnviarACocina(empSalon, c2, m.UsuarioID, origenTst); err != nil {
		t.Fatalf("enviar a cocina: %v", err)
	}

	svc.CerrarCuenta(empSalon, c1, m.UsuarioID, origenTst)
	svc.CerrarCuenta(empSalon, c2, m.UsuarioID, origenTst)
	out, err := svc.FinalizarTurno(empSalon, actorA, origenTst, tu.ID)
	if err != nil {
		t.Fatalf("finalizar: %v", err)
	}
	r := out.Resumen
	if r == nil {
		t.Fatal("sin resumen")
	}
	if r.Mesas != 2 {
		t.Errorf("mesas = %d, se esperaban 2", r.Mesas)
	}
	if r.Personas != 5 {
		t.Errorf("personas = %d, se esperaban 5 (2+3)", r.Personas)
	}
	if r.Ordenes != 2 {
		t.Errorf("órdenes = %d, se esperaban 2 (una ronda por mesa)", r.Ordenes)
	}
	if r.TotalFacturado != 600 {
		t.Errorf("total = %v, se esperaban 600 (200 + 400)", r.TotalFacturado)
	}
	if r.TicketPromedio != 300 {
		t.Errorf("ticket promedio = %v, se esperaban 300", r.TicketPromedio)
	}
}

// Una vez cerrado, el turno es una FOTO: reasignar una mesa después no puede
// cambiar lo que dice el resumen de anoche (misma regla que el arqueo de caja).
func TestResumenTurno_QuedaCongelado(t *testing.T) {
	svc, st := servicioTurnos(t)
	luis := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	ana := mesoneroListo(t, svc, "Ana", "usr_ana", "6677")
	tu := enTurno(t, svc, luis, "4455")
	enTurno(t, svc, ana, "6677")
	cid, _ := abrirMesa(t, svc, st, luis, "2", 4)

	out, err := svc.CerrarTurnoForzado(empSalon, actorA, origenTst, tu.ID, ana.ID, pinSup)
	if err != nil {
		t.Fatalf("forzar: %v", err)
	}
	antes := *out.Resumen

	// La mesa la sigue trabajando Ana y cambia de monto: el resumen de Luis no.
	if _, err := svc.AgregarItems(empSalon, cid, ana.UsuarioID, usuario.RolMesonero, origenTst,
		[]application.ItemInput{{SKU: "CAF-001", Nombre: "Café", Cantidad: 9, PrecioUnitario: 1000}}); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	fin, _ := st.Turnos.ByID(empSalon, tu.ID)
	if fin.Resumen == nil || *fin.Resumen != antes {
		t.Errorf("el resumen congelado cambió: antes %+v, ahora %+v", antes, fin.Resumen)
	}
}

/* --- Maestro y aislamiento ------------------------------------------------ */

// La serie MS- CONTINÚA: no se reinicia ni repite un código ya entregado. Se
// comprueba la continuidad y no valores absolutos, porque la demo del
// restaurante ya siembra credenciales y atarse a "MS-001" haría frágil la prueba
// ante cualquier cambio del seed.
func TestCrearMesonero_NumeraLaSerieMS(t *testing.T) {
	svc, _ := servicioTurnos(t)
	previos := len(svc.ListarMesoneros(empSalon, sedeSalon))
	a, err := svc.CrearMesonero(empSalon, actorA, origenTst, sedeSalon, "Luis", "usr_luis")
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	b, err := svc.CrearMesonero(empSalon, actorA, origenTst, sedeSalon, "Ana", "usr_ana")
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	quiero := func(n int) string { return fmt.Sprintf("MS-%03d", n) }
	if a.Codigo != quiero(previos+1) || b.Codigo != quiero(previos+2) {
		t.Errorf("la serie debería continuar en %s y %s: dio %q y %q",
			quiero(previos+1), quiero(previos+2), a.Codigo, b.Codigo)
	}
}

// Una persona, una credencial: dos abrirían dos turnos y partirían sus mesas.
func TestCrearMesonero_UnaCredencialPorUsuario(t *testing.T) {
	svc, _ := servicioTurnos(t)
	if _, err := svc.CrearMesonero(empSalon, actorA, origenTst, sedeSalon, "Luis", "usr_luis"); err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := svc.CrearMesonero(empSalon, actorA, origenTst, sedeSalon, "Luis (bis)", "usr_luis"); !errors.Is(err, application.ErrUsuarioYaEsMesonero) {
		t.Fatalf("se esperaba ErrUsuarioYaEsMesonero, se obtuvo: %v", err)
	}
}

func TestCrearMesonero_ExigeSedeYUsuario(t *testing.T) {
	svc, _ := servicioTurnos(t)
	if _, err := svc.CrearMesonero(empSalon, actorA, origenTst, "", "Luis", "usr_luis"); err == nil {
		t.Error("sin sede debería rechazarse: el mesonero trabaja en un salón, no en la empresa entera")
	}
	if _, err := svc.CrearMesonero(empSalon, actorA, origenTst, sedeSalon, "Luis", ""); err == nil {
		t.Error("sin usuario debería rechazarse: las mesas se llevan por usuario")
	}
}

// El aislamiento por tenant es la última línea: otra empresa no ve ni toca esto.
func TestTurnos_AisladosPorEmpresa(t *testing.T) {
	svc, _ := servicioTurnos(t)
	m := mesoneroListo(t, svc, "Luis", "usr_luis", "4455")
	tu := enTurno(t, svc, m, "4455")

	if got := svc.ListarMesoneros(empDemo, ""); len(got) != 0 {
		t.Errorf("otra empresa no debería ver mesoneros ajenos, ve %d", len(got))
	}
	if got := svc.TurnosVivos(empDemo, ""); len(got) != 0 {
		t.Errorf("otra empresa no debería ver turnos ajenos, ve %d", len(got))
	}
	if _, err := svc.FinalizarTurno(empDemo, actorA, origenTst, tu.ID); !errors.Is(err, application.ErrTurnoNoExiste) {
		t.Errorf("no se puede cerrar el turno de otra empresa: %v", err)
	}
}

// Sin los turnos cableados el salón funciona como siempre: una empresa que no
// los usa no puede quedar trancada.
func TestTurnos_SinCablearElSalonFuncionaIgual(t *testing.T) {
	svc, st := servicioComanderas(t) // sin ConMesoneros
	mesa := mesaPorNombre(t, st, "2")
	_, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: mesa.ID,
		MesoneroID: "usr_meso", MesoneroNombre: "Meso", RolActor: usuario.RolMesonero,
		Actor: "usr_meso", Origen: origenTst, Comensales: 2,
	})
	if err != nil {
		t.Fatalf("sin turnos cableados el mesonero debe poder trabajar: %v", err)
	}
}
