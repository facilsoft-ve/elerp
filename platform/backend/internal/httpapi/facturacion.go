package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp-platform/internal/facturacion"
)

// registerBilling monta el catálogo de planes y las suscripciones por organización.
func (s *Server) registerBilling(g fiber.Router) {
	g.Get("/planes", s.handlePlanes)
	g.Post("/planes", s.handleCrearPlan)
	g.Patch("/planes/:id", s.handleEditarPlan)
	g.Delete("/planes/:id", s.handleArchivarPlan)

	g.Get("/facturacion", s.handleResumen)

	g.Get("/orgs/:id/suscripcion", s.handleSuscripcion)
	g.Post("/orgs/:id/suscripcion", s.handleAsignarPlan)
	g.Post("/orgs/:id/suscripcion/marcar-pagada", s.handleMarcarPagada)
	g.Post("/orgs/:id/suscripcion/cancelar", s.handleCancelar)
	g.Post("/orgs/:id/suscripcion/sincronizar", s.handleSincronizar)
}

func (s *Server) handlePlanes(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"planes": s.bill.Planes(), "hubmyDisponible": s.bill.HubmyDisponible()})
}

// planBody es el cuerpo de creación/edición de un plan.
type planBody struct {
	Nombre         string `json:"nombre"`
	Descripcion    string `json:"descripcion"`
	PrecioCents    int    `json:"precioCents"`
	Moneda         string `json:"moneda"`
	Intervalo      string `json:"intervalo"`
	Limites        facturacion.Limites `json:"limites"`
	HubmyPackageID string `json:"hubmyPackageId"`
}

func (b planBody) plan() facturacion.Plan {
	moneda := b.Moneda
	if moneda == "" {
		moneda = "USD"
	}
	intervalo := b.Intervalo
	if intervalo == "" {
		intervalo = facturacion.IntervaloMensual
	}
	return facturacion.Plan{
		Nombre: b.Nombre, Descripcion: b.Descripcion, PrecioCents: b.PrecioCents,
		Moneda: moneda, Intervalo: intervalo, Limites: b.Limites, HubmyPackageID: b.HubmyPackageID,
	}
}

func (s *Server) handleCrearPlan(c *fiber.Ctx) error {
	var in planBody
	if err := c.BodyParser(&in); err != nil || in.Nombre == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "el plan necesita un nombre"})
	}
	return c.Status(fiber.StatusCreated).JSON(s.bill.CrearPlan(in.plan()))
}

func (s *Server) handleEditarPlan(c *fiber.Ctx) error {
	var in planBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	p := in.plan()
	p.Activo = true
	out, ok := s.bill.EditarPlan(c.Params("id"), p)
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "el plan no existe"})
	}
	return c.JSON(out)
}

func (s *Server) handleArchivarPlan(c *fiber.Ctx) error {
	out, ok := s.bill.ArchivarPlan(c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "el plan no existe"})
	}
	return c.JSON(out)
}

func (s *Server) handleResumen(c *fiber.Ctx) error {
	return c.JSON(s.bill.Resumen())
}

func (s *Server) handleSuscripcion(c *fiber.Ctx) error {
	return c.JSON(s.bill.Suscripcion(c.Params("id")))
}

type asignarBody struct {
	PlanID      string `json:"planId"`
	HubmyUserID string `json:"hubmyUserId"`
}

func (s *Server) handleAsignarPlan(c *fiber.Ctx) error {
	var in asignarBody
	if err := c.BodyParser(&in); err != nil || in.PlanID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "falta el plan"})
	}
	view, err := s.bill.AsignarPlan(c.Context(), c.Params("id"), in.PlanID, in.HubmyUserID, sesionDe(c).Email)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(view)
}

func (s *Server) handleMarcarPagada(c *fiber.Ctx) error {
	view, err := s.bill.MarcarPagada(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(view)
}

func (s *Server) handleCancelar(c *fiber.Ctx) error {
	view, err := s.bill.Cancelar(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(view)
}

func (s *Server) handleSincronizar(c *fiber.Ctx) error {
	view, err := s.bill.Sincronizar(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(view)
}
