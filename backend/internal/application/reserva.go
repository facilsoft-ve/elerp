package application

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/cuenta"
	"github.com/mornix/elerp/internal/domain/reserva"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// RESERVACIONES DEL SALÓN
//
// Una reserva aparta el salón para una hora. El valor está en que, cuando el cliente
// llegue, la mesa esté libre; por eso la mesa reservada se MARCA en el tablero dentro
// de una ventana antes de la hora (VentanaReserva) y no desde que se toma la reserva:
// apartar una mesa desde la mañana para las 9 de la noche le quita al local un turno
// entero que sí podía vender.
//
// Quien recibe verifica al que llega por nombre o cédula (Buscar) y lo SIENTA: eso
// abre su cuenta de mesa, que es donde sigue el flujo normal del módulo.

var (
	ErrReservasNoDisponible = errors.New("las reservaciones no están disponibles")
	ErrReservaNoExiste      = errors.New("esa reserva no existe")
	ErrReservaFecha         = errors.New("la reserva necesita una fecha válida (YYYY-MM-DD)")
	ErrReservaHora          = errors.New("la reserva necesita una hora válida (HH:MM)")
	ErrReservaPersonas      = errors.New("la reserva necesita al menos una persona")
	ErrReservaNombre        = errors.New("la reserva necesita el nombre de quien reserva")
	ErrReservaMesaNoExiste  = errors.New("esa mesa no existe en esta sede")
	ErrReservaMesaChica     = errors.New("esa mesa no tiene capacidad para tantas personas")
	ErrReservaMesaOcupada   = errors.New("esa mesa ya está reservada a esa hora")
	ErrReservaEstado        = errors.New("la reserva no está en un estado que permita esta acción")
	ErrReservaZonaNoExiste  = errors.New("esa zona no existe en esta sede")
	ErrReservaPasado        = errors.New("no se puede reservar para una fecha y hora que ya pasaron")
	// ErrHorarioServicioInvalido: el horario de atención viene a medias o mal
	// formado. A medias no se puede comparar contra nada.
	ErrHorarioServicioInvalido = errors.New("el horario de atención necesita apertura y cierre en formato HH:MM")
)

// VentanaReserva es cuánto ANTES de la hora la mesa reservada se aparta en el tablero.
// Dos horas: lo que dura una comida. Antes de eso la mesa se sigue vendiendo.
const VentanaReserva = 2 * time.Hour

// RolesReservas son los que pueden ver y manejar la agenda. El MESONERO entra: en la
// práctica quien recibe en la puerta es del salón, y obligarlo a buscar a un gerente
// para confirmar una cédula es justo lo que hace que la reserva no se use.
// La contadora no: es consulta fuera de Contabilidad/Tesorería.
var RolesReservas = []string{
	usuario.RolDueno, usuario.RolDesarrollador, usuario.RolCajero,
	usuario.RolVendedor, usuario.RolMesonero,
}

// ConReservas cablea el repositorio de reservas.
func (s *Service) ConReservas(r reserva.Repository) *Service {
	s.reservas = r
	return s
}

// EntradaReserva son los datos con los que se toma o edita una reserva.
type EntradaReserva struct {
	EmpresaID string
	SedeID    string
	Fecha     string // YYYY-MM-DD
	Hora      string // HH:MM
	Personas  int
	Nombre    string
	Documento string
	Telefono  string
	MesaID    string
	Zona      string
	Nota      string
	Actor     string
	Origen    string
}

// ReservasDelDia devuelve la agenda de una fecha, ordenada por hora.
func (s *Service) ReservasDelDia(empresaID, sedeID, fecha string) []reserva.Reserva {
	if s.reservas == nil {
		return []reserva.Reserva{}
	}
	if strings.TrimSpace(fecha) == "" {
		fecha = hoyLocal()
	}
	return s.reservas.DelDia(empresaID, sedeID, fecha)
}

// ReservasProximas devuelve las reservas de hoy en adelante: es la agenda con la que
// trabaja el salón, sin arrastrar el histórico.
func (s *Service) ReservasProximas(empresaID, sedeID string) []reserva.Reserva {
	if s.reservas == nil {
		return []reserva.Reserva{}
	}
	return s.reservas.Desde(empresaID, sedeID, hoyLocal())
}

// BuscarReservas filtra la agenda de un día por nombre o cédula. Es la búsqueda de la
// puerta: alguien llega, dice su nombre, y hay que encontrarlo sin leer toda la lista.
func (s *Service) BuscarReservas(empresaID, sedeID, fecha, termino string) []reserva.Reserva {
	todas := s.ReservasDelDia(empresaID, sedeID, fecha)
	if strings.TrimSpace(termino) == "" {
		return todas
	}
	out := make([]reserva.Reserva, 0, len(todas))
	for _, r := range todas {
		if r.Coincide(termino) {
			out = append(out, r)
		}
	}
	return out
}

// CrearReserva registra una reserva nueva.
func (s *Service) CrearReserva(in EntradaReserva) (reserva.Reserva, error) {
	if s.reservas == nil {
		return reserva.Reserva{}, ErrReservasNoDisponible
	}
	r := reserva.Reserva{
		EmpresaID: in.EmpresaID, SedeID: in.SedeID,
		Fecha: strings.TrimSpace(in.Fecha), Hora: strings.TrimSpace(in.Hora),
		Personas: in.Personas, Nombre: strings.TrimSpace(in.Nombre),
		Documento: reserva.NormalizarDocumento(in.Documento),
		Telefono:  strings.TrimSpace(in.Telefono),
		MesaID:    strings.TrimSpace(in.MesaID), Zona: strings.TrimSpace(in.Zona),
		Nota: strings.TrimSpace(in.Nota), Estado: reserva.EstadoPendiente,
		CreadaPor: in.Actor, Creada: ahora(), Actualizada: ahora(),
	}
	if err := s.validarReserva(&r, ""); err != nil {
		return reserva.Reserva{}, err
	}
	// Solo al CREAR: editar una reserva vieja (corregir el teléfono, dejar una
	// nota) tiene que seguir siendo posible.
	if esPasado(r.Fecha, r.Hora) {
		return reserva.Reserva{}, ErrReservaPasado
	}
	out := s.reservas.Create(r)
	s.audit.Append(evento(in.EmpresaID, in.Actor, in.Origen, "restaurante.reserva.crear", out.ID,
		fmt.Sprintf("%s · %s %s · %d persona(s)", out.Nombre, out.Fecha, out.Hora, out.Personas)))
	return out, nil
}

// ActualizarReserva edita una reserva pendiente (hora, personas, mesa, datos).
func (s *Service) ActualizarReserva(id string, in EntradaReserva) (reserva.Reserva, error) {
	if s.reservas == nil {
		return reserva.Reserva{}, ErrReservasNoDisponible
	}
	r, ok := s.reservas.ByID(in.EmpresaID, id)
	if !ok {
		return reserva.Reserva{}, ErrReservaNoExiste
	}
	if !r.Activa() {
		return reserva.Reserva{}, ErrReservaEstado
	}
	if v := strings.TrimSpace(in.Fecha); v != "" {
		r.Fecha = v
	}
	if v := strings.TrimSpace(in.Hora); v != "" {
		r.Hora = v
	}
	if in.Personas > 0 {
		r.Personas = in.Personas
	}
	if v := strings.TrimSpace(in.Nombre); v != "" {
		r.Nombre = v
	}
	if v := strings.TrimSpace(in.Documento); v != "" {
		r.Documento = reserva.NormalizarDocumento(v)
	}
	if v := strings.TrimSpace(in.Telefono); v != "" {
		r.Telefono = v
	}
	// La mesa y la zona SÍ se pueden vaciar: soltar la mesa concreta y dejar solo el
	// área es una edición normal ("mejor ponlos donde haya sitio").
	r.MesaID = strings.TrimSpace(in.MesaID)
	r.Zona = strings.TrimSpace(in.Zona)
	r.Nota = strings.TrimSpace(in.Nota)
	r.Actualizada = ahora()
	if err := s.validarReserva(&r, r.ID); err != nil {
		return reserva.Reserva{}, err
	}
	out, ok := s.reservas.Update(r)
	if !ok {
		return reserva.Reserva{}, ErrReservaNoExiste
	}
	s.audit.Append(evento(in.EmpresaID, in.Actor, in.Origen, "restaurante.reserva.editar", out.ID, out.Nombre))
	return out, nil
}

// CambiarEstadoReserva marca la reserva como cancelada o como "no llegó".
func (s *Service) CambiarEstadoReserva(empresaID, id, estado, actor, origen string) (reserva.Reserva, error) {
	if s.reservas == nil {
		return reserva.Reserva{}, ErrReservasNoDisponible
	}
	if estado != reserva.EstadoCancelada && estado != reserva.EstadoNoLlego && estado != reserva.EstadoPendiente {
		return reserva.Reserva{}, ErrReservaEstado
	}
	r, ok := s.reservas.ByID(empresaID, id)
	if !ok {
		return reserva.Reserva{}, ErrReservaNoExiste
	}
	// Una reserva YA SENTADA no se cancela: esa gente está comiendo. Lo que se cierra
	// es su cuenta, por la vía normal.
	if r.Estado == reserva.EstadoSentada {
		return reserva.Reserva{}, ErrReservaEstado
	}
	r.Estado = estado
	r.Actualizada = ahora()
	out, ok := s.reservas.Update(r)
	if !ok {
		return reserva.Reserva{}, ErrReservaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.reserva."+estado, out.ID, out.Nombre))
	return out, nil
}

// SentarReserva recibe al cliente que llegó: abre la cuenta de su mesa y deja la
// reserva enlazada con ella. `mesaID` permite elegir la mesa en el momento cuando la
// reserva solo apartó una zona.
func (s *Service) SentarReserva(empresaID, id, mesaID, actor, rolActor, origen string) (reserva.Reserva, cuenta.Cuenta, error) {
	if s.reservas == nil {
		return reserva.Reserva{}, cuenta.Cuenta{}, ErrReservasNoDisponible
	}
	r, ok := s.reservas.ByID(empresaID, id)
	if !ok {
		return reserva.Reserva{}, cuenta.Cuenta{}, ErrReservaNoExiste
	}
	if !r.Activa() {
		return reserva.Reserva{}, cuenta.Cuenta{}, ErrReservaEstado
	}
	destino := strings.TrimSpace(mesaID)
	if destino == "" {
		destino = r.MesaID
	}
	if destino == "" {
		return reserva.Reserva{}, cuenta.Cuenta{}, ErrReservaMesaNoExiste
	}
	m, ok := s.mesas.ByID(empresaID, destino)
	if !ok || m.SedeID != r.SedeID {
		return reserva.Reserva{}, cuenta.Cuenta{}, ErrReservaMesaNoExiste
	}
	// Abrir la cuenta es el mismo camino que usa la comandera: no hay una vía
	// "de reservas" con reglas propias.
	c, err := s.AbrirCuenta(AperturaCuenta{
		EmpresaID: empresaID, SedeID: r.SedeID, MesaID: m.ID,
		MesoneroID: actor, MesoneroNombre: r.Nombre, RolActor: rolActor,
		Actor: actor, Origen: origen, Comensales: r.Personas,
	})
	if err != nil {
		return reserva.Reserva{}, cuenta.Cuenta{}, err
	}
	r.Estado = reserva.EstadoSentada
	r.MesaID = m.ID
	r.MesaNombre = m.Nombre
	r.CuentaID = c.ID
	r.Actualizada = ahora()
	out, ok := s.reservas.Update(r)
	if !ok {
		return reserva.Reserva{}, cuenta.Cuenta{}, ErrReservaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.reserva.sentar", out.ID,
		fmt.Sprintf("%s en la mesa %s", out.Nombre, m.Nombre)))
	return out, c, nil
}

// MesasReservadasAhora devuelve, por id de mesa, la reserva que la está apartando en
// este momento: las pendientes con mesa fija cuya hora cae dentro de la ventana. Es lo
// que el tablero pinta como «reservada» para que nadie siente a otro ahí.
func (s *Service) MesasReservadasAhora(empresaID, sedeID string) map[string]reserva.Reserva {
	out := map[string]reserva.Reserva{}
	if s.reservas == nil {
		return out
	}
	ahoraT := time.Now()
	for _, r := range s.reservas.DelDia(empresaID, sedeID, hoyLocal()) {
		if !r.Activa() || r.MesaID == "" {
			continue
		}
		t, err := horaDeReserva(r.Fecha, r.Hora)
		if err != nil {
			continue
		}
		// Desde VentanaReserva antes y hasta VentanaReserva después: una reserva a la
		// que todavía no llegan sigue apartando la mesa un rato, no se suelta al minuto.
		if ahoraT.After(t.Add(-VentanaReserva)) && ahoraT.Before(t.Add(VentanaReserva)) {
			out[r.MesaID] = r
		}
	}
	return out
}

// --- validación ---

func (s *Service) validarReserva(r *reserva.Reserva, excluirID string) error {
	if _, err := time.Parse("2006-01-02", r.Fecha); err != nil {
		return ErrReservaFecha
	}
	if _, err := time.Parse("15:04", r.Hora); err != nil {
		return ErrReservaHora
	}
	if r.Personas <= 0 {
		return ErrReservaPersonas
	}
	if r.Nombre == "" {
		return ErrReservaNombre
	}
	// Zona: una zona inventada no es una decisión del negocio, es un dato mal
	// escrito. La reserva apuntaría a un sector que no existe y nadie la
	// encontraría en la puerta.
	if s.mesas != nil && !zonaExiste(s.mesas.List(r.EmpresaID, r.SedeID), r.Zona) {
		return ErrReservaZonaNoExiste
	}
	if r.MesaID == "" {
		r.MesaNombre = ""
		return nil
	}
	if s.mesas == nil {
		return ErrReservaMesaNoExiste
	}
	m, ok := s.mesas.ByID(r.EmpresaID, r.MesaID)
	if !ok || m.SedeID != r.SedeID {
		return ErrReservaMesaNoExiste
	}
	// Sentar a 6 personas en una mesa de 2 no es un detalle: se descubre con la
	// gente parada en la puerta.
	if m.Capacidad > 0 && r.Personas > m.Capacidad {
		return ErrReservaMesaChica
	}
	r.MesaNombre = m.Nombre
	if r.Zona == "" {
		r.Zona = m.Zona
	}
	// Choque con otra reserva de la MISMA mesa en la misma franja.
	nueva, err := horaDeReserva(r.Fecha, r.Hora)
	if err != nil {
		return ErrReservaHora
	}
	for _, otra := range s.reservas.DelDia(r.EmpresaID, r.SedeID, r.Fecha) {
		if otra.ID == excluirID || !otra.Activa() || otra.MesaID != r.MesaID {
			continue
		}
		t, err := horaDeReserva(otra.Fecha, otra.Hora)
		if err != nil {
			continue
		}
		if diferenciaMenorA(nueva, t, VentanaReserva) {
			return ErrReservaMesaOcupada
		}
	}
	return nil
}

// horaDeReserva arma el instante local de una reserva a partir de su fecha y hora.
func horaDeReserva(fecha, hora string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04", fecha+" "+hora, time.Local)
}

func diferenciaMenorA(a, b time.Time, d time.Duration) bool {
	dif := a.Sub(b)
	if dif < 0 {
		dif = -dif
	}
	return dif < d
}

// hoyLocal es la fecha de hoy en el huso del local (YYYY-MM-DD).
func hoyLocal() string { return time.Now().Format("2006-01-02") }
