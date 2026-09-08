package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/almacen"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerAlmacenes monta el maestro de Almacenes (Configuración). Un almacén
// pertenece a una sede; una sede tiene ≥1 almacén y exactamente uno principal (del
// que despacha el POS). Maestro editable con soft-disable, mismo gate que el resto
// de Configuración: lo ve quien accede a Ajustes (Dueña/Desarrollador/Contadora),
// solo Dueña/Desarrollador lo modifica.
func (s *Server) registerAlmacenes(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/config/almacenes", ver)
	g.Get("/", s.handleAlmacenes)
	g.Post("/", edit, s.handleCrearAlmacen)
	g.Patch("/:id", edit, s.handleActualizarAlmacen)
	g.Post("/:id/desactivar", edit, s.handleDesactivarAlmacen)
}

func (s *Server) handleAlmacenes(c *fiber.Ctx) error {
	// Devuelve los almacenes CON su ocupación (para la barra de llenado). Opcional
	// ?sede= filtra a una sede.
	todos := s.svc.AlmacenesConOcupacion(empresaIDOf(c))
	if sedeID := c.Query("sede"); sedeID != "" {
		filtrados := make([]application.AlmacenView, 0, len(todos))
		for _, a := range todos {
			if a.SedeID == sedeID {
				filtrados = append(filtrados, a)
			}
		}
		return c.JSON(filtrados)
	}
	return c.JSON(todos)
}

// almacenBody es el cuerpo de creación/edición. `activo` va como puntero para
// soportar un PATCH parcial: nil = no enviado = no cambiar el estado.
type almacenBody struct {
	SedeID          string   `json:"sedeId"`
	Nombre          string   `json:"nombre"`
	Tipo            string   `json:"tipo"`
	Principal       bool     `json:"principal"`
	Activo          *bool    `json:"activo"`
	Capacidad       float64  `json:"capacidad"`
	CapacidadUnidad string   `json:"capacidadUnidad"`
	RubrosAdmitidos []string `json:"rubrosAdmitidos"`
}

func (b almacenBody) modelo() almacen.Almacen {
	return almacen.Almacen{
		SedeID: b.SedeID, Nombre: b.Nombre, Tipo: b.Tipo, Principal: b.Principal,
		Capacidad: b.Capacidad, CapacidadUnidad: b.CapacidadUnidad, RubrosAdmitidos: b.RubrosAdmitidos,
	}
}

func (s *Server) handleCrearAlmacen(c *fiber.Ctx) error {
	var in almacenBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	// La sede debe pertenecer al tenant (defensa cross-tenant además de la del repo).
	if !s.tenancy.SedeValida(empresaIDOf(c), in.SedeID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "la sede no existe en esta empresa"})
	}
	out, err := s.svc.CrearAlmacen(empresaIDOf(c), principalOf(c).UserID, origen(c), in.modelo())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarAlmacen(c *fiber.Ctx) error {
	var in almacenBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarAlmacen(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.modelo(), in.Activo)
	if err != nil {
		if errors.Is(err, application.ErrAlmacenNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleDesactivarAlmacen(c *fiber.Ctx) error {
	if err := s.svc.DesactivarAlmacen(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c)); err != nil {
		if errors.Is(err, application.ErrAlmacenNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
