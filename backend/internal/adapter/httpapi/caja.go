package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/caja"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerCajas monta el módulo de cajas.
//
// Administración (crear caja, habilitar, dar de alta cajeros): solo Dueña y
// Desarrollador. Operación (listar cajas de la sede, abrir y cerrar turno):
// también Vendedor y Cajero, que son quienes están en el mostrador.
func (s *Server) registerCajas(r fiber.Router) {
	admin := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)
	opera := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolCajero)
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolCajero, usuario.RolContadora)

	g := r.Group("/cajas")
	g.Get("", ver, s.handleCajas)
	g.Post("", admin, s.handleCrearCaja)
	g.Patch("/:id", admin, s.handleActualizarCaja)
	g.Patch("/:id/estado", admin, s.handleEstadoCaja)
	g.Post("/:id/abrir", opera, s.handleAbrirCaja)
	g.Post("/:id/cerrar", opera, s.handleCerrarCaja)

	// Sesión de caja del usuario actual: lo consulta el POS para decidir entre
	// el punto de venta y la vista «Sin caja abierta».
	g.Get("/mi-sesion", ver, s.handleMiSesionCaja)

	// Historial de turnos: es la auditoría del arqueo que muestra Configuración.
	g.Get("/sesiones", s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora), s.handleSesionesCaja)

	// Preview del arqueo de un turno: lo esperado por método (derivado de los
	// documentos del turno) para que el cajero compare antes de declarar el conteo.
	g.Get("/sesiones/:id/arqueo", ver, s.handleArqueoSesion)

	// Autorización de supervisor (flujo 2.4): la pide el modo caja para quitar
	// una línea o salir. La valida el servidor, no la interfaz.
	g.Post("/autorizar", opera, s.handleAutorizarSupervisor)

	cj := r.Group("/cajeros")
	cj.Get("", ver, s.handleCajeros)
	cj.Post("", admin, s.handleCrearCajero)
	cj.Patch("/:id", admin, s.handleActualizarCajero)
}

// errorDeCaja traduce los errores de negocio a códigos HTTP con significado.
// 409 en «caja tomada» es lo que el contrato del paquete de entrega especifica.
func errorDeCaja(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, application.ErrCajaTomada):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, application.ErrCajeroInvalido),
		errors.Is(err, application.ErrCajeroOtraSede):
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, application.ErrCajaNoExiste),
		errors.Is(err, application.ErrCajeroNoExiste),
		errors.Is(err, application.ErrSesionNoExiste):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, application.ErrCodigoCajeroEnUso):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
}

func (s *Server) handleCajas(c *fiber.Ctx) error {
	// `sede` acota; sin él devuelve las cajas de toda la empresa (vista de
	// administración, que agrupa por sede).
	sedeID := string(append([]byte(nil), c.Query("sede")...))
	if sedeID == "" {
		sedeID = sedeIDOf(c)
	}
	return c.JSON(s.svc.ListarCajas(empresaIDOf(c), sedeID, principalOf(c).UserID))
}

func (s *Server) handleCrearCaja(c *fiber.Ctx) error {
	var in struct {
		Nombre              string `json:"nombre"`
		SedeID              string `json:"sedeId"`
		DispositivoFiscalID string `json:"dispositivoFiscalId"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	sedeID := in.SedeID
	if sedeID == "" {
		sedeID = sedeIDOf(c)
	}
	// La sede tiene que pertenecer al tenant: nunca se confía en el cliente.
	if !s.tenancy.SedeValida(empresaIDOf(c), sedeID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sede inválida"})
	}
	out, err := s.svc.CrearCaja(empresaIDOf(c), sedeID, principalOf(c).UserID, origen(c), in.Nombre, in.DispositivoFiscalID)
	if err != nil {
		return errorDeCaja(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleActualizarCaja edita una caja (nombre, sede, dispositivo fiscal) y su
// activación. Los campos ausentes en el cuerpo no se tocan.
func (s *Server) handleActualizarCaja(c *fiber.Ctx) error {
	var in struct {
		Nombre              *string `json:"nombre"`
		SedeID              *string `json:"sedeId"`
		DispositivoFiscalID *string `json:"dispositivoFiscalId"`
		Activa              *bool   `json:"activa"`
		ColorFondo          *string `json:"colorFondo"`
		LogoVersion         *string `json:"logoVersion"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if in.SedeID != nil && *in.SedeID != "" && !s.tenancy.SedeValida(empresaIDOf(c), *in.SedeID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sede inválida"})
	}
	out, err := s.svc.ActualizarCaja(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), application.CambiosCaja{
		Nombre: in.Nombre, SedeID: in.SedeID, DispositivoFiscalID: in.DispositivoFiscalID, Activa: in.Activa,
		ColorFondo: in.ColorFondo, LogoVersion: in.LogoVersion,
	})
	if err != nil {
		return errorDeCaja(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleEstadoCaja(c *fiber.Ctx) error {
	var in struct{ Estado string }
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CambiarEstadoCaja(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.Estado)
	if err != nil {
		return errorDeCaja(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleAbrirCaja(c *fiber.Ctx) error {
	var in struct {
		CodigoCajero string  `json:"codigoCajero"`
		Pin          string  `json:"pin"`
		FondoInicial float64 `json:"fondoInicial"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.AbrirCajaConFondo(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.CodigoCajero, in.Pin, in.FondoInicial)
	if err != nil {
		return errorDeCaja(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleCerrarCaja(c *fiber.Ctx) error {
	var in struct {
		Forzado bool `json:"forzado"`
		// EfectivoContadoBs es puntero: nil = no se declaró conteo (cierre sin
		// arqueo, p. ej. forzado). Presente = el cajero contó la gaveta.
		EfectivoContadoBs *float64            `json:"efectivoContadoBs"`
		ContadoPorMetodo  []caja.ArqueoConteo `json:"contadoPorMetodo"`
	}
	_ = c.BodyParser(&in)
	// Forzar el cierre de la caja de otro cajero es acción de administración.
	rol := rolOf(c)
	if in.Forzado && rol != usuario.RolDueno && rol != usuario.RolDesarrollador {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "solo la Dueña o el Desarrollador pueden forzar el cierre"})
	}
	out, err := s.svc.CerrarCajaConArqueo(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.Forzado, application.CierreArqueo{
		EfectivoContadoBs: in.EfectivoContadoBs,
		ContadoPorMetodo:  in.ContadoPorMetodo,
	})
	if err != nil {
		return errorDeCaja(c, err)
	}
	return c.JSON(out)
}

// handleArqueoSesion devuelve el arqueo (esperado, o congelado si ya cerró) de
// un turno concreto.
func (s *Server) handleArqueoSesion(c *fiber.Ctx) error {
	arqueo, err := s.svc.ArqueoDeSesion(empresaIDOf(c), c.Params("id"))
	if err != nil {
		return errorDeCaja(c, err)
	}
	return c.JSON(arqueo)
}

func (s *Server) handleMiSesionCaja(c *fiber.Ctx) error {
	ses, ok := s.svc.SesionDeActor(empresaIDOf(c), principalOf(c).UserID)
	if !ok {
		// No es un error: es el estado normal antes de abrir turno.
		return c.JSON(fiber.Map{"abierta": false})
	}
	return c.JSON(fiber.Map{"abierta": true, "sesion": ses})
}

// handleSesionesCaja devuelve el historial de turnos de la empresa.
func (s *Server) handleSesionesCaja(c *fiber.Ctx) error {
	return c.JSON(s.svc.SesionesDeCaja(empresaIDOf(c)))
}

// handleAutorizarSupervisor valida el PIN de un supervisor para una acción
// sensible del modo caja. Devuelve quién autorizó, para poder mostrarlo.
func (s *Server) handleAutorizarSupervisor(c *fiber.Ctx) error {
	var in struct {
		Accion string `json:"accion"`
		Pin    string `json:"pin"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	nombre, err := s.svc.AutorizarSupervisor(empresaIDOf(c), principalOf(c).UserID, origen(c), in.Accion, in.Pin)
	if err != nil {
		if errors.Is(err, application.ErrPinSupervisorInvalido) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"autorizadoPor": nombre})
}

func (s *Server) handleCajeros(c *fiber.Ctx) error {
	return c.JSON(s.svc.ListarCajeros(empresaIDOf(c)))
}

func (s *Server) handleCrearCajero(c *fiber.Ctx) error {
	var in struct {
		Nombre, Codigo, Pin, SedeID, UsuarioID string
		Supervisor                             bool
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	sedeID := in.SedeID
	if sedeID == "" {
		sedeID = sedeIDOf(c)
	}
	if sedeID != "" && !s.tenancy.SedeValida(empresaIDOf(c), sedeID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sede inválida"})
	}
	out, err := s.svc.CrearCajero(empresaIDOf(c), principalOf(c).UserID, origen(c), sedeID, in.Nombre, in.Codigo, in.Pin, in.UsuarioID, in.Supervisor)
	if err != nil {
		return errorDeCaja(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleActualizarCajero edita una credencial de puesto: nombre, sede, rol
// supervisor, activación y (opcionalmente) reinicio del PIN. El código no se
// cambia. Los campos ausentes no se tocan.
func (s *Server) handleActualizarCajero(c *fiber.Ctx) error {
	var in struct {
		Nombre     *string `json:"nombre"`
		SedeID     *string `json:"sedeId"`
		Supervisor *bool   `json:"supervisor"`
		Activo     *bool   `json:"activo"`
		Pin        *string `json:"pin"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if in.SedeID != nil && *in.SedeID != "" && !s.tenancy.SedeValida(empresaIDOf(c), *in.SedeID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sede inválida"})
	}
	out, err := s.svc.ActualizarCajero(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), application.CambiosCajero{
		Nombre: in.Nombre, SedeID: in.SedeID, Supervisor: in.Supervisor, Activo: in.Activo, Pin: in.Pin,
	})
	if err != nil {
		return errorDeCaja(c, err)
	}
	return c.JSON(out)
}

// Referencia al dominio para que el import no quede sin uso si se recortan
// handlers: los estados de caja son parte del contrato de este adaptador.
var _ = caja.EstadoHabilitada
