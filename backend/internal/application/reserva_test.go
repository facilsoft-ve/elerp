package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/reserva"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// RESERVACIONES DEL SALÓN
//
// Lo que tiene que quedar fijo: que una reserva aparte una mesa de verdad (que no se
// pueda reservar dos veces la misma a la misma hora), que la gente se encuentre en la
// puerta por nombre o cédula aunque la escriban distinto, y que sentarla abra su
// cuenta — porque si sentar no abre cuenta, el mesonero tiene que crearla aparte y la
// reserva no sirve de nada.

func servicioReservas(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := servicioSalon(t)
	svc.ConReservas(st.Reservas)
	return svc, st
}

// fechaDeManana: las reservas de prueba se toman para MAÑANA, no para hoy.
// Con "hoy a las 20:00" la suite pasaba por la mañana y fallaba de noche —una
// reserva para una hora ya pasada se rechaza—, y eso hacía que el resultado
// dependiera de a qué hora se corriera. Mañana siempre está en el futuro.
func fechaDeManana() string { return time.Now().Add(24 * time.Hour).Format("2006-01-02") }

// reservaBase arma una entrada válida para la sede del restaurante demo.
func reservaBase(st *inmem.Store, t *testing.T, over func(*application.EntradaReserva)) application.EntradaReserva {
	t.Helper()
	in := application.EntradaReserva{
		EmpresaID: empSalon, SedeID: sedeSalon,
		Fecha: fechaDeManana(), Hora: "20:00", Personas: 2,
		Nombre: "Ana Pérez", Documento: "V-12.345.678", Telefono: "04141234567",
		Actor: "usr_meso", Origen: origenTst,
	}
	if over != nil {
		over(&in)
	}
	return in
}

func TestReserva_SeCreaYQuedaPendiente(t *testing.T) {
	svc, st := servicioReservas(t)
	r, err := svc.CrearReserva(reservaBase(st, t, nil))
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if r.Estado != reserva.EstadoPendiente {
		t.Errorf("una reserva nace pendiente, nació %q", r.Estado)
	}
	// El documento se guarda NORMALIZADO: en la puerta nadie lo escribe igual dos veces.
	if r.Documento != "V12345678" {
		t.Errorf("el documento debe quedar normalizado, quedó %q", r.Documento)
	}
	if len(svc.ReservasDelDia(empSalon, sedeSalon, r.Fecha)) != 1 {
		t.Error("la reserva debe aparecer en la agenda del día")
	}
}

func TestReserva_ExigeLoMinimoParaServir(t *testing.T) {
	svc, st := servicioReservas(t)
	casos := []struct {
		nombre string
		ajuste func(*application.EntradaReserva)
		err    error
	}{
		{"sin fecha", func(i *application.EntradaReserva) { i.Fecha = "" }, application.ErrReservaFecha},
		{"fecha inventada", func(i *application.EntradaReserva) { i.Fecha = "31/12/2026" }, application.ErrReservaFecha},
		{"sin hora", func(i *application.EntradaReserva) { i.Hora = "" }, application.ErrReservaHora},
		{"hora imposible", func(i *application.EntradaReserva) { i.Hora = "25:00" }, application.ErrReservaHora},
		{"sin personas", func(i *application.EntradaReserva) { i.Personas = 0 }, application.ErrReservaPersonas},
		{"sin nombre", func(i *application.EntradaReserva) { i.Nombre = "  " }, application.ErrReservaNombre},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := svc.CrearReserva(reservaBase(st, t, c.ajuste))
			if !errors.Is(err, c.err) {
				t.Fatalf("se esperaba %v, se obtuvo %v", c.err, err)
			}
		})
	}
}

// Reservar una mesa concreta es apartarla: otra reserva a la misma hora no entra.
func TestReserva_NoSeReservaDosVecesLaMismaMesa(t *testing.T) {
	svc, st := servicioReservas(t)
	m := mesaPorNombre(t, st, "6") // Terraza, capacidad 4
	if _, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.MesaID = m.ID
	})); err != nil {
		t.Fatalf("primera reserva: %v", err)
	}
	_, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.MesaID = m.ID
		i.Hora = "20:30" // dentro de la ventana
		i.Nombre = "Luis Gómez"
	}))
	if !errors.Is(err, application.ErrReservaMesaOcupada) {
		t.Fatalf("la misma mesa a la misma hora debe rechazarse, dio: %v", err)
	}
	// Fuera de la ventana sí entra: el turno anterior ya se fue.
	if _, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.MesaID = m.ID
		i.Hora = "23:00"
		i.Nombre = "Luis Gómez"
	})); err != nil {
		t.Errorf("tres horas después la mesa está libre: %v", err)
	}
}

// Sentar a 6 en una mesa de 2 se descubre con la gente parada en la puerta.
func TestReserva_RespetaLaCapacidadDeLaMesa(t *testing.T) {
	svc, st := servicioReservas(t)
	m := mesaPorNombre(t, st, "3") // capacidad 2
	_, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.MesaID = m.ID
		i.Personas = 6
	}))
	if !errors.Is(err, application.ErrReservaMesaChica) {
		t.Fatalf("se esperaba ErrReservaMesaChica, se obtuvo: %v", err)
	}
}

// Sin mesa concreta (solo zona) la reserva vale igual: se decide la mesa al llegar.
func TestReserva_PuedeApartarSoloLaZona(t *testing.T) {
	svc, st := servicioReservas(t)
	r, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.Zona = "Terraza"
		i.Personas = 8
	}))
	if err != nil {
		t.Fatalf("reservar por zona: %v", err)
	}
	if r.MesaID != "" {
		t.Errorf("no debía fijar mesa, fijó %q", r.MesaID)
	}
	if r.Zona != "Terraza" {
		t.Errorf("debía conservar la zona, quedó %q", r.Zona)
	}
}

// La búsqueda de la puerta: por nombre parcial o por cédula escrita de cualquier forma.
func TestReserva_SeEncuentraPorNombreOCedula(t *testing.T) {
	svc, st := servicioReservas(t)
	if _, err := svc.CrearReserva(reservaBase(st, t, nil)); err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.Nombre = "Carlos Díaz"
		i.Documento = "E-98765432"
		i.Hora = "21:00"
	})); err != nil {
		t.Fatalf("crear 2: %v", err)
	}
	hoy := fechaDeManana()
	casos := map[string]string{
		"pérez":        "Ana Pérez",   // parte del nombre
		"PEREZ":        "",            // sin acento NO coincide: es el dato tal como se tomó
		"12345678":     "Ana Pérez",   // cédula sin prefijo
		"V-12.345.678": "Ana Pérez",   // cédula con puntos y guion
		"e98765432":    "Carlos Díaz", // minúsculas
	}
	for termino, esperado := range casos {
		out := svc.BuscarReservas(empSalon, sedeSalon, hoy, termino)
		if esperado == "" {
			if len(out) != 0 {
				t.Errorf("%q no debía encontrar nada, encontró %d", termino, len(out))
			}
			continue
		}
		if len(out) != 1 || out[0].Nombre != esperado {
			t.Errorf("%q debía encontrar a %s, encontró %+v", termino, esperado, out)
		}
	}
}

// Sentar a quien llegó ABRE su cuenta: si no, el mesonero la tendría que crear aparte
// y la reserva no habría servido de nada.
func TestReserva_SentarAbreLaCuentaDeLaMesa(t *testing.T) {
	svc, st := servicioReservas(t)
	m := mesaPorNombre(t, st, "7")
	r, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.MesaID = m.ID
		i.Personas = 4
	}))
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	out, c, err := svc.SentarReserva(empSalon, r.ID, "", "usr_meso", usuario.RolMesonero, origenTst)
	if err != nil {
		t.Fatalf("sentar: %v", err)
	}
	if out.Estado != reserva.EstadoSentada {
		t.Errorf("la reserva debe quedar sentada, quedó %q", out.Estado)
	}
	if c.ID == "" || out.CuentaID != c.ID {
		t.Errorf("la reserva debe quedar enlazada con la cuenta abierta: cuenta=%q enlace=%q", c.ID, out.CuentaID)
	}
	if c.Comensales != 4 {
		t.Errorf("la cuenta debe abrir con los comensales de la reserva, abrió con %d", c.Comensales)
	}
	// Y ya no se puede cancelar: esa gente está comiendo.
	if _, err := svc.CambiarEstadoReserva(empSalon, r.ID, reserva.EstadoCancelada, "usr_meso", origenTst); !errors.Is(err, application.ErrReservaEstado) {
		t.Errorf("una reserva sentada no se cancela, dio: %v", err)
	}
}

// Una reserva por ZONA se sienta eligiendo la mesa en el momento.
func TestReserva_PorZonaSeSientaEligiendoMesa(t *testing.T) {
	svc, st := servicioReservas(t)
	r, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) { i.Zona = "Terraza" }))
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	// Sin mesa ni en la reserva ni en la llamada no hay dónde sentarla.
	if _, _, err := svc.SentarReserva(empSalon, r.ID, "", "usr_meso", usuario.RolMesonero, origenTst); !errors.Is(err, application.ErrReservaMesaNoExiste) {
		t.Fatalf("sin mesa debe pedirla, dio: %v", err)
	}
	m := mesaPorNombre(t, st, "8")
	out, _, err := svc.SentarReserva(empSalon, r.ID, m.ID, "usr_meso", usuario.RolMesonero, origenTst)
	if err != nil {
		t.Fatalf("sentar eligiendo mesa: %v", err)
	}
	if out.MesaID != m.ID || out.MesaNombre != m.Nombre {
		t.Errorf("la reserva debe quedar con la mesa donde se sentó: %+v", out)
	}
}

// El tablero tiene que saber qué mesa está apartada AHORA, y solo dentro de la ventana:
// apartar una mesa desde la mañana para la noche le quita al local un turno vendible.
func TestReserva_LaMesaSeApartaSoloCercaDeLaHora(t *testing.T) {
	svc, st := servicioReservas(t)
	m := mesaPorNombre(t, st, "6")
	ahora := time.Now()

	// Una reserva dentro de media hora: aparta.
	if _, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.MesaID = m.ID
		i.Hora = ahora.Add(30 * time.Minute).Format("15:04")
		i.Fecha = ahora.Add(30 * time.Minute).Format("2006-01-02")
	})); err != nil {
		t.Fatalf("crear cercana: %v", err)
	}
	apartadas := svc.MesasReservadasAhora(empSalon, sedeSalon)
	if _, ok := apartadas[m.ID]; !ok {
		t.Error("una reserva a media hora debe apartar la mesa en el tablero")
	}

	// Otra mesa, reservada para dentro de 6 horas: NO aparta todavía.
	lejos := mesaPorNombre(t, st, "7")
	futuro := ahora.Add(6 * time.Hour)
	if _, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.MesaID = lejos.ID
		i.Fecha = futuro.Format("2006-01-02")
		i.Hora = futuro.Format("15:04")
		i.Nombre = "Reserva lejana"
	})); err != nil {
		t.Fatalf("crear lejana: %v", err)
	}
	if _, ok := svc.MesasReservadasAhora(empSalon, sedeSalon)[lejos.ID]; ok {
		t.Error("una reserva de dentro de 6 horas no debe apartar la mesa todavía")
	}
}

// Cancelar libera la mesa para otra reserva a la misma hora.
func TestReserva_CancelarLiberaLaMesa(t *testing.T) {
	svc, st := servicioReservas(t)
	m := mesaPorNombre(t, st, "6")
	r, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) { i.MesaID = m.ID }))
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := svc.CambiarEstadoReserva(empSalon, r.ID, reserva.EstadoCancelada, "usr_meso", origenTst); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if _, err := svc.CrearReserva(reservaBase(st, t, func(i *application.EntradaReserva) {
		i.MesaID = m.ID
		i.Nombre = "Otro cliente"
	})); err != nil {
		t.Errorf("con la anterior cancelada la mesa está libre: %v", err)
	}
}

// Sin el repositorio cableado (módulo apagado) las reservas no revientan: responden
// vacío y el resto del sistema sigue igual.
func TestReserva_SinModuloNoRompe(t *testing.T) {
	svc, _ := servicioSalon(t) // sin ConReservas
	if out := svc.ReservasDelDia(empSalon, sedeSalon, fechaDeManana()); len(out) != 0 {
		t.Errorf("sin reservas cableadas la agenda es vacía, dio %d", len(out))
	}
	if _, err := svc.CrearReserva(application.EntradaReserva{EmpresaID: empSalon}); !errors.Is(err, application.ErrReservasNoDisponible) {
		t.Errorf("se esperaba ErrReservasNoDisponible, se obtuvo: %v", err)
	}
}
