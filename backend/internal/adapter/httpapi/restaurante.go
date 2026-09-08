package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerRestaurante monta el módulo Restaurante: el maestro de MESAS del salón
// (con su mapa) y la configuración de la IMPRESORA de comandas. El mapa lo VE
// cualquier rol que opere el salón (para seleccionar mesa); lo EDITA la Dueña/
// Desarrollador. La impresora la configura la Dueña/Desarrollador.
func (s *Server) registerRestaurante(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolCajero, usuario.RolContadora, usuario.RolMesonero)
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/restaurante", ver)
	g.Get("/mesas", s.handleMesas)
	g.Post("/mesas", edit, s.handleCrearMesa)
	g.Patch("/mesas/:id", edit, s.handleActualizarMesa)
	g.Delete("/mesas/:id", edit, s.handleEliminarMesa)
	g.Post("/mapa", edit, s.handleGuardarMapa)
	g.Get("/plano", s.handlePlano)
	g.Put("/plano", edit, s.handleGuardarPlano)

	g.Get("/impresora", s.handleImpresora)
	g.Put("/impresora", edit, s.handleGuardarImpresora)

	// Cuentas de mesa (comandera): abrir, agregar renglones, enviar a cocina,
	// cancelar/marcar renglón, cerrar. Las opera cualquier rol del salón (ver).
	g.Get("/cuentas", s.handleCuentas)
	g.Post("/cuentas", s.handleAbrirCuenta)
	g.Get("/cuentas/:id", s.handleCuenta)
	g.Post("/cuentas/:id/items", s.handleAgregarItems)
	g.Post("/cuentas/:id/enviar", s.handleEnviarCocina)
	g.Delete("/cuentas/:id/items/:itemId", s.handleCancelarItem)
	g.Post("/cuentas/:id/items/:itemId/estado", s.handleMarcarItem)
	g.Post("/cuentas/:id/cerrar", s.handleCerrarCuenta)
	// Cobro → factura desde la cuenta. Emitir factura es de roles con caja/venta
	// (no la contadora, que es consulta fuera de Contabilidad/Tesorería).
	emitir := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolCajero, usuario.RolVendedor)
	g.Get("/cuentas/:id/preview-cobro", s.handlePreviewCobro)
	g.Post("/cuentas/:id/cobrar", emitir, s.handleCobrarCuenta)
}

func (s *Server) handlePreviewCobro(c *fiber.Ctx) error {
	sub, iva, total, err := s.svc.PreviewCobroCuenta(empresaIDOf(c), c.Params("id"))
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(fiber.Map{"subtotal": sub, "iva": iva, "total": total})
}

func (s *Server) handleCobrarCuenta(c *fiber.Ctx) error {
	var in application.CobroCuentaEntrada
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	cta, doc, err := s.svc.CobrarCuenta(empresaIDOf(c), sedeIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"cuenta": cta, "documento": doc})
}

func (s *Server) handleCuentas(c *fiber.Ctx) error {
	return c.JSON(s.svc.CuentasAbiertas(empresaIDOf(c), sedeIDOf(c)))
}

func (s *Server) handleCuenta(c *fiber.Ctx) error {
	out, ok := s.svc.Cuenta(empresaIDOf(c), c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "la cuenta no existe"})
	}
	return c.JSON(out)
}

func (s *Server) handleAbrirCuenta(c *fiber.Ctx) error {
	var in struct {
		MesaID     string `json:"mesaId"`
		Comensales int    `json:"comensales"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	p := principalOf(c)
	out, err := s.svc.AbrirCuenta(empresaIDOf(c), sedeIDOf(c), in.MesaID, p.UserID, p.Nombre, p.UserID, origen(c), in.Comensales)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleAgregarItems(c *fiber.Ctx) error {
	var in struct {
		Items []application.ItemInput `json:"items"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.AgregarItems(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.Items)
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleEnviarCocina(c *fiber.Ctx) error {
	cta, comanda, err := s.svc.EnviarACocina(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(fiber.Map{"cuenta": cta, "comanda": fiber.Map{"ronda": cta.UltimaRonda, "items": comanda}})
}

func (s *Server) handleCancelarItem(c *fiber.Ctx) error {
	out, err := s.svc.CancelarItem(empresaIDOf(c), c.Params("id"), c.Params("itemId"), principalOf(c).UserID, origen(c))
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleMarcarItem(c *fiber.Ctx) error {
	var in struct {
		Estado string `json:"estado"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.MarcarItem(empresaIDOf(c), c.Params("id"), c.Params("itemId"), in.Estado, principalOf(c).UserID, origen(c))
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleCerrarCuenta(c *fiber.Ctx) error {
	out, err := s.svc.CerrarCuenta(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(out)
}

// cuentaErr mapea los errores de cuenta a su código HTTP.
func cuentaErr(c *fiber.Ctx, err error) error {
	if errors.Is(err, application.ErrCuentaMesaNoExiste) || errors.Is(err, application.ErrItemCuentaNoExiste) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
}

func (s *Server) handleMesas(c *fiber.Ctx) error {
	return c.JSON(s.svc.Mesas(empresaIDOf(c), sedeIDOf(c)))
}

// mesaBody es el cuerpo de creación/edición de una mesa. La posición es la celda
// (columna, fila) de la grilla del salón.
type mesaBody struct {
	Nombre    string `json:"nombre"`
	Zona      string `json:"zona"`
	Capacidad int    `json:"capacidad"`
	Forma     string `json:"forma"`
	Columna   int    `json:"columna"`
	Fila      int    `json:"fila"`
	Activa    *bool  `json:"activa"`
}

func (b mesaBody) modelo() mesa.Mesa {
	return mesa.Mesa{
		Nombre: b.Nombre, Zona: b.Zona, Capacidad: b.Capacidad, Forma: b.Forma,
		Columna: b.Columna, Fila: b.Fila,
	}
}

func (s *Server) handleCrearMesa(c *fiber.Ctx) error {
	var in mesaBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearMesa(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), in.modelo())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarMesa(c *fiber.Ctx) error {
	var in mesaBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarMesa(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.modelo(), in.Activa)
	if err != nil {
		if errors.Is(err, application.ErrMesaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleEliminarMesa(c *fiber.Ctx) error {
	if err := s.svc.EliminarMesa(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c)); err != nil {
		if errors.Is(err, application.ErrMesaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handlePlano(c *fiber.Ctx) error {
	return c.JSON(s.svc.PlanoSalon(empresaIDOf(c), sedeIDOf(c)))
}

func (s *Server) handleGuardarPlano(c *fiber.Ctx) error {
	var in struct {
		Filas      int          `json:"filas"`
		Columnas   int          `json:"columnas"`
		Bloqueadas []mesa.Celda `json:"bloqueadas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.GuardarPlanoSalon(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), in.Filas, in.Columnas, in.Bloqueadas)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleGuardarMapa(c *fiber.Ctx) error {
	var in struct {
		Posiciones []application.PosicionMesa `json:"posiciones"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if err := s.svc.GuardarMapa(empresaIDOf(c), principalOf(c).UserID, origen(c), in.Posiciones); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleImpresora(c *fiber.Ctx) error {
	return c.JSON(s.svc.ImpresoraComandas(empresaIDOf(c), sedeIDOf(c)))
}

func (s *Server) handleGuardarImpresora(c *fiber.Ctx) error {
	var in application.ImpresoraBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.GuardarImpresoraComandas(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}
