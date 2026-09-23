package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/usuario"
)

/* FABRICACIÓN — superficie HTTP.
 *
 * Planificar lo ve cualquiera de la operación: saber qué se está produciendo y
 * si alcanzan los insumos es información de todos los días. MOVER la orden
 * —arrancar, terminar, cancelar— toca el inventario, así que va con el mismo
 * gate que el resto de la escritura de inventario.
 */
func (s *Server) registrarFabricacion(api fiber.Router) {
	g := api.Group("/fabricacion")
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor,
		usuario.RolCajero, usuario.RolContadora)

	g.Get("/ordenes", ver, s.handleOrdenesFabricacion)
	g.Get("/ordenes/:id", ver, s.handleOrdenFabricacion)
	// Planear NO escribe: es lo que deja ver si alcanza y cuánto va a costar
	// ANTES de sacar los insumos del almacén.
	g.Post("/planear", ver, s.handlePlanearOrden)

	g.Post("/ordenes", s.escribirInventario, s.handleCrearOrdenFabricacion)
	g.Post("/ordenes/:id/iniciar", s.escribirInventario, s.handleIniciarOrden)
	g.Post("/ordenes/:id/terminar", s.escribirInventario, s.handleTerminarOrden)
	g.Post("/ordenes/:id/cancelar", s.escribirInventario, s.handleCancelarOrden)
}

func (s *Server) handleOrdenesFabricacion(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"ordenes": s.svc.OrdenesFabricacion(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handleOrdenFabricacion(c *fiber.Ctx) error {
	o, ok := s.svc.OrdenFabricacion(empresaIDOf(c), c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "la orden no existe"})
	}
	return c.JSON(o)
}

func (s *Server) handlePlanearOrden(c *fiber.Ctx) error {
	var in struct {
		SKU      string  `json:"sku"`
		Cantidad float64 `json:"cantidad"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	plan, err := s.svc.PlanearOrden(empresaIDOf(c), sedeIDOf(c), in.SKU, in.Cantidad)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(plan)
}

func (s *Server) handleCrearOrdenFabricacion(c *fiber.Ctx) error {
	var in struct {
		SKU          string  `json:"sku"`
		Cantidad     float64 `json:"cantidad"`
		AlmacenID    string  `json:"almacenId"`
		Lote         string  `json:"lote"`
		Vencimiento  string  `json:"vencimiento"`
		PesoUnitario float64 `json:"pesoUnitario"`
		OrigenTipo   string  `json:"origenTipo"`
		OrigenID     string  `json:"origenId"`
		Nota         string  `json:"nota"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearOrdenFabricacion(empresaIDOf(c), application.EntradaOrden{
		SedeID: sedeIDOf(c), AlmacenID: in.AlmacenID, SKU: in.SKU, Cantidad: in.Cantidad,
		Lote: in.Lote, Vencimiento: in.Vencimiento, PesoUnitario: in.PesoUnitario,
		OrigenTipo: in.OrigenTipo, OrigenID: in.OrigenID, Nota: in.Nota,
		Actor: principalOf(c).UserID, Origen: origen(c),
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleIniciarOrden(c *fiber.Ctx) error {
	out, err := s.svc.IniciarOrden(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleTerminarOrden(c *fiber.Ctx) error {
	var in struct {
		Producida float64 `json:"producida"`
	}
	_ = c.BodyParser(&in)
	out, err := s.svc.TerminarOrden(empresaIDOf(c), c.Params("id"), in.Producida, principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleCancelarOrden(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
	}
	_ = c.BodyParser(&in)
	out, err := s.svc.CancelarOrden(empresaIDOf(c), c.Params("id"), in.Motivo, principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}
