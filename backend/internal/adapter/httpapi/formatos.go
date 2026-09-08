package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/plantilla"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerFormatos monta los FORMATOS de documento (Configuración › Formatos):
// el maestro editable de plantillas con las que se imprime cada tipo de documento
// (factura, cotización, …), con selección por sede. Mismo gate que el resto de
// Configuración: lo VE quien accede a Ajustes (Dueña/Desarrollador/Contadora);
// solo Dueña/Desarrollador lo MODIFICA.
func (s *Server) registerFormatos(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/config/formatos", ver)
	g.Get("/", s.handleFormatos)
	g.Get("/campos", s.handleFormatoCampos)
	g.Post("/", edit, s.handleCrearFormato)
	g.Post("/asignar", edit, s.handleAsignarFormato)
	g.Patch("/:id", edit, s.handleActualizarFormato)
	g.Post("/:id/predeterminada", edit, s.handleFormatoPredeterminado)
	g.Delete("/:id", edit, s.handleEliminarFormato)
}

func (s *Server) handleFormatos(c *fiber.Ctx) error {
	return c.JSON(s.svc.Plantillas(empresaIDOf(c)))
}

// handleFormatoCampos devuelve el catálogo de campos dinámicos que un bloque de
// campo puede mostrar (dato de referencia estable, para poblar el selector del
// editor).
func (s *Server) handleFormatoCampos(c *fiber.Ctx) error {
	return c.JSON(plantilla.CamposDisponibles())
}

// formatoBody es el cuerpo de creación/edición de un formato. Espeja el modelo de
// dominio salvo los campos que gobiernan casos de uso propios (Sedes,
// Predeterminada, Activa), que NO se leen del cuerpo del guardado del lienzo.
type formatoBody struct {
	Nombre  string             `json:"nombre"`
	Tipo    string             `json:"tipo"`
	Papel   string             `json:"papel"`
	AnchoMM float64            `json:"anchoMm"`
	AltoMM  float64            `json:"altoMm"`
	Bloques []plantilla.Bloque `json:"bloques"`
}

func (b formatoBody) modelo() plantilla.Plantilla {
	return plantilla.Plantilla{
		Nombre: b.Nombre, Tipo: b.Tipo, Papel: b.Papel,
		AnchoMM: b.AnchoMM, AltoMM: b.AltoMM, Bloques: b.Bloques,
	}
}

func (s *Server) handleCrearFormato(c *fiber.Ctx) error {
	var in formatoBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearPlantilla(empresaIDOf(c), principalOf(c).UserID, origen(c), in.modelo())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarFormato(c *fiber.Ctx) error {
	var in formatoBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarPlantilla(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.modelo())
	if err != nil {
		if errors.Is(err, application.ErrPlantillaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleEliminarFormato(c *fiber.Ctx) error {
	if err := s.svc.EliminarPlantilla(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c)); err != nil {
		if errors.Is(err, application.ErrPlantillaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleFormatoPredeterminado(c *fiber.Ctx) error {
	if err := s.svc.FijarPlantillaPredeterminada(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c)); err != nil {
		if errors.Is(err, application.ErrPlantillaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// asignacionBody: qué formato usa una sede para un tipo. plantillaId vacío deja a
// la sede sin asignación explícita (cae al predeterminado del tipo).
type asignacionBody struct {
	Tipo        string `json:"tipo"`
	SedeID      string `json:"sedeId"`
	PlantillaID string `json:"plantillaId"`
}

func (s *Server) handleAsignarFormato(c *fiber.Ctx) error {
	var in asignacionBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if err := s.svc.AsignarPlantillaSede(empresaIDOf(c), in.Tipo, in.SedeID, in.PlantillaID, principalOf(c).UserID, origen(c)); err != nil {
		if errors.Is(err, application.ErrPlantillaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
