package httpapi

import (
	"errors"
	"net/url"

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
	// El FACTOR va por su propia ruta y no dentro del cuerpo general: cambiarlo
	// altera cómo se convierte toda recepción futura, así que merece una acción
	// explícita en vez de viajar de polizón en una edición de nombre.
	g.Patch("/:id/factor", edit, s.handleFactorUnidad)
	// Con qué unidades se puede expresar una cantidad de la dada. La pantalla la usa
	// para no ofrecer una conversión que después se va a rechazar.
	g.Get("/compatibles/:simbolo", s.handleUnidadesCompatibles)
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

func (s *Server) handleFactorUnidad(c *fiber.Ctx) error {
	var in struct {
		// Cero es válido y significa «sin declarar»: así se vuelve atrás sin borrar
		// la unidad, que rompería los productos que ya la usan.
		Factor float64 `json:"factor"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarFactorUnidad(empresaIDOf(c), c.Params("id"), in.Factor,
		principalOf(c).UserID, origen(c))
	if err != nil {
		if errors.Is(err, application.ErrUnidadNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleUnidadesCompatibles(c *fiber.Ctx) error {
	simbolo, err := url.PathUnescape(c.Params("simbolo"))
	if err != nil {
		simbolo = c.Params("simbolo")
	}
	return c.JSON(fiber.Map{"unidades": s.svc.UnidadesCompatiblesCon(empresaIDOf(c), simbolo)})
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
