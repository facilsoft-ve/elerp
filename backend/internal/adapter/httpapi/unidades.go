package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/unidadmedida"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerUnidades monta el maestro de Unidades de medida (Configuración). Es un
// maestro editable (no ledger): se crean, editan y desactivan (soft-disable), del
// que el catálogo de producto elige su UnidadBase. Mismo gate que el resto de
// Configuración: lo ve quien accede a Ajustes (Dueña/Desarrollador/Contadora); solo
// Dueña/Desarrollador lo modifica. Las unidades ACTIVAS también viajan en el
// bootstrap para que el select del editor de producto no haga una llamada extra.
func (s *Server) registerUnidades(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/config/unidades", ver)
	g.Get("/", s.handleUnidades)
	g.Post("/", edit, s.handleCrearUnidad)
	g.Patch("/:id", edit, s.handleActualizarUnidad)
	g.Post("/:id/desactivar", edit, s.handleDesactivarUnidad)
}

func (s *Server) handleUnidades(c *fiber.Ctx) error {
	return c.JSON(s.svc.Unidades(empresaIDOf(c)))
}

// unidadBody es el cuerpo de creación/edición de una unidad de medida. `activa` va
// como puntero para soportar un PATCH parcial: nil = no enviado = no cambiar el
// estado (en el alta se ignora, toda unidad nueva arranca activa).
type unidadBody struct {
	Simbolo   string `json:"simbolo"`
	Nombre    string `json:"nombre"`
	Categoria string `json:"categoria"`
	Activa    *bool  `json:"activa"`
}

func (b unidadBody) modelo() unidadmedida.UnidadMedida {
	return unidadmedida.UnidadMedida{
		Simbolo: b.Simbolo, Nombre: b.Nombre, Categoria: b.Categoria,
	}
}

func (s *Server) handleCrearUnidad(c *fiber.Ctx) error {
	var in unidadBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearUnidad(empresaIDOf(c), principalOf(c).UserID, origen(c), in.modelo())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarUnidad(c *fiber.Ctx) error {
	var in unidadBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarUnidad(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.modelo(), in.Activa)
	if err != nil {
		if errors.Is(err, application.ErrUnidadNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleDesactivarUnidad(c *fiber.Ctx) error {
	if err := s.svc.DesactivarUnidad(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c)); err != nil {
		if errors.Is(err, application.ErrUnidadNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
