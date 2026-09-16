package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
)

/* Validaciones y avisos de reserva. Lo que estas pruebas cuidan es la
 * DISTINCIÓN entre rechazar y avisar: un anfitrión que no puede tomar una
 * reserva porque el sistema cree saber más que él la anota en un papel, y ahí se
 * pierde entera. */

func tieneAviso(avisos []application.AvisoReserva, codigo string) bool {
	for _, a := range avisos {
		if a.Codigo == codigo {
			return true
		}
	}
	return false
}

func entradaReserva(fecha, hora string, personas int, zona string) application.EntradaReserva {
	return application.EntradaReserva{
		EmpresaID: empSalon, SedeID: sedeSalon, Fecha: fecha, Hora: hora,
		Personas: personas, Nombre: "Prueba", Zona: zona,
		Actor: actorA, Origen: origenTst,
	}
}

// EL aviso que pidió la contadora: no hay una sola mesa para el grupo, hay que
// sentarlos en mesas distintas. Es ADVERTENCIA, no rechazo.
func TestAvisosReserva_SinMesaParaTantos(t *testing.T) {
	svc, _ := servicioReservas(t)
	f, h := fechaDeManana(), "20:00"
	// El restaurante demo tiene mesas de hasta 8.
	avisos := svc.AvisosDeReserva(entradaReserva(f, h, 20, ""), "")
	if !tieneAviso(avisos, application.AvisoSinMesaUnica) {
		t.Fatalf("20 personas no caben en una sola mesa: debería avisar. Avisos: %+v", avisos)
	}
	// Y sin embargo la reserva SE PUEDE CREAR: es aviso, no candado.
	if _, err := svc.CrearReserva(entradaReserva(f, h, 20, "")); err != nil {
		t.Errorf("el aviso no puede impedir la reserva: %v", err)
	}
}

func TestAvisosReserva_GrupoQueSiCabeNoAvisa(t *testing.T) {
	svc, _ := servicioReservas(t)
	f, h := fechaDeManana(), "20:00"
	if avisos := svc.AvisosDeReserva(entradaReserva(f, h, 4, ""), ""); tieneAviso(avisos, application.AvisoSinMesaUnica) {
		t.Errorf("4 personas caben en una mesa del demo: no debería avisar. %+v", avisos)
	}
}

// El aforo se evalúa por FRANJA, no por día: dos reservas a horas distintas no
// compiten por las mismas mesas.
func TestAvisosReserva_AforoDeLaFranja(t *testing.T) {
	svc, _ := servicioReservas(t)
	f := fechaDeManana()
	// El salón demo tiene 34 puestos (4+4+2+2+6+4+4+8). Se llena la franja.
	for i := 0; i < 5; i++ {
		if _, err := svc.CrearReserva(entradaReserva(f, "20:00", 8, "")); err != nil {
			t.Fatalf("crear reserva %d: %v", i, err)
		}
	}
	avisos := svc.AvisosDeReserva(entradaReserva(f, "20:00", 8, ""), "")
	if !tieneAviso(avisos, application.AvisoAforoSuperado) {
		t.Fatalf("la franja ya está por encima del aforo: debería avisar. %+v", avisos)
	}
	// A otra hora, fuera de la ventana, no hay conflicto.
	if avisos := svc.AvisosDeReserva(entradaReserva(f, "13:00", 8, ""), ""); tieneAviso(avisos, application.AvisoAforoSuperado) {
		t.Errorf("a otra hora no compite por las mismas mesas: %+v", avisos)
	}
}

// Al EDITAR, la propia reserva no puede contarse dos veces contra el aforo: si
// no, mover la hora de un grupo grande dispararía el aviso siempre.
func TestAvisosReserva_AlEditarNoSeCuentaASiMisma(t *testing.T) {
	svc, _ := servicioReservas(t)
	f := fechaDeManana()
	for i := 0; i < 4; i++ {
		svc.CrearReserva(entradaReserva(f, "20:00", 8, ""))
	}
	r, err := svc.CrearReserva(entradaReserva(f, "20:00", 2, ""))
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	conElla := svc.AvisosDeReserva(entradaReserva(f, "20:00", 2, ""), "")
	sinElla := svc.AvisosDeReserva(entradaReserva(f, "20:00", 2, ""), r.ID)
	if len(sinElla) > len(conElla) {
		t.Errorf("excluirse no puede EMPEORAR el aforo: con %d avisos, sin %d", len(conElla), len(sinElla))
	}
}

// Fuera del horario de atención se AVISA, no se rechaza: un local que acepta una
// reserva fuera de hora por excepción no puede quedar trancado.
func TestAvisosReserva_FueraDelHorarioDeAtencion(t *testing.T) {
	svc, _ := servicioReservas(t)
	modo := "aviso"
	if _, err := svc.GuardarConfigSalonCompleta(empSalon, sedeSalon, actorA, origenTst, nil, &modo); err != nil {
		t.Fatalf("config: %v", err)
	}
	// El horario se fija directo sobre el repo: la pantalla de configuración lo
	// hará por su ruta, pero acá interesa el cálculo.
	svc.GuardarHorarioServicioSalon(empSalon, sedeSalon, actorA, origenTst, "18:00", "01:00")

	f := fechaDeManana()
	if avisos := svc.AvisosDeReserva(entradaReserva(f, "15:00", 2, ""), ""); !tieneAviso(avisos, application.AvisoFueraDeHorario) {
		t.Errorf("las 15:00 están fuera de 18:00–01:00: debería avisar. %+v", avisos)
	}
	// Y la medianoche SÍ está dentro: un cierre a la 01:00 cruza el día, y sin
	// contemplarlo toda reserva nocturna saldría marcada.
	if avisos := svc.AvisosDeReserva(entradaReserva(f, "23:30", 2, ""), ""); tieneAviso(avisos, application.AvisoFueraDeHorario) {
		t.Errorf("las 23:30 están dentro de 18:00–01:00: %+v", avisos)
	}
}

func TestAvisosReserva_SinHorarioDeclaradoNoAvisa(t *testing.T) {
	svc, _ := servicioReservas(t)
	f := fechaDeManana()
	if avisos := svc.AvisosDeReserva(entradaReserva(f, "04:00", 2, ""), ""); tieneAviso(avisos, application.AvisoFueraDeHorario) {
		t.Errorf("sin horario configurado no hay nada contra qué comparar: %+v", avisos)
	}
}

/* --- Lo que SÍ se rechaza -------------------------------------------------- */

// Una zona inventada no es una decisión del negocio: es un dato mal escrito, y
// la reserva quedaría apuntando a un sector que no existe.
func TestCrearReserva_RechazaZonaInexistente(t *testing.T) {
	svc, _ := servicioReservas(t)
	f, h := fechaDeManana(), "20:00"
	if _, err := svc.CrearReserva(entradaReserva(f, h, 2, "Azotea")); !errors.Is(err, application.ErrReservaZonaNoExiste) {
		t.Fatalf("se esperaba ErrReservaZonaNoExiste, se obtuvo: %v", err)
	}
	// La zona real del demo sí pasa.
	if _, err := svc.CrearReserva(entradaReserva(f, h, 2, "Terraza")); err != nil {
		t.Errorf("una zona real debe aceptarse: %v", err)
	}
}

// Reservar para ayer no es política: es imposible, y casi siempre un mes mal
// tecleado.
func TestCrearReserva_RechazaElPasado(t *testing.T) {
	svc, _ := servicioReservas(t)
	ayer := time.Now().Add(-48 * time.Hour).Format("2006-01-02")
	if _, err := svc.CrearReserva(entradaReserva(ayer, "20:00", 2, "")); !errors.Is(err, application.ErrReservaPasado) {
		t.Fatalf("se esperaba ErrReservaPasado, se obtuvo: %v", err)
	}
}

// Editar una reserva vieja (corregir el teléfono, dejar una nota) tiene que
// seguir siendo posible: el rechazo del pasado es solo al CREAR.
func TestActualizarReserva_ElPasadoNoBloqueaLaEdicion(t *testing.T) {
	svc, st := servicioReservas(t)
	f, h := fechaDeManana(), "20:00"
	r, err := svc.CrearReserva(entradaReserva(f, h, 2, ""))
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	// Se la empuja al pasado por el repo, simulando el día siguiente.
	r.Fecha = time.Now().Add(-48 * time.Hour).Format("2006-01-02")
	st.Reservas.Update(r)

	in := entradaReserva(r.Fecha, r.Hora, 2, "")
	in.Telefono = "0412-0000000"
	if _, err := svc.ActualizarReserva(r.ID, in); err != nil {
		t.Errorf("editar una reserva pasada debe poder: %v", err)
	}
}

func TestAforoYMayorCapacidad_IgnoranMesasInactivas(t *testing.T) {
	svc, st := servicioReservas(t)
	// Se desactiva la mesa más grande del demo (capacidad 8).
	for _, m := range st.Mesas.List(empSalon, sedeSalon) {
		if m.Capacidad == 8 {
			m.Activa = false
			st.Mesas.Update(m)
		}
	}
	f, h := fechaDeManana(), "20:00"
	avisos := svc.AvisosDeReserva(entradaReserva(f, h, 7, ""), "")
	if !tieneAviso(avisos, application.AvisoSinMesaUnica) {
		t.Errorf("con la mesa de 8 dada de baja, 7 personas ya no caben en una sola: %+v", avisos)
	}
}
