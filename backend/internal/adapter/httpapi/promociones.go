package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerPromociones monta el maestro de Promociones (submódulo de Ventas). Es
// un maestro editable (no ledger): se crean y actualizan, nunca es append-only.
// Las promociones activas y vigentes alimentan el carrusel de la pantalla del
// cliente del POS, junto a los slides manuales de Ajustes.
func (s *Server) registerPromociones(r fiber.Router) {
	// Ver: Dueña/Desarrollador + Vendedor (quienes trabajan Ventas).
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor)
	// Mutaciones: solo Dueña/Desarrollador (configuración comercial de la empresa).
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	// Todo el submódulo exige el módulo Marketing activo (gating de servidor).
	g := r.Group("/ventas/promociones", ver, s.requireModulo(aplicacion.ModMarketing))
	g.Get("/", s.handlePromociones)
	g.Post("/", edit, s.handleCrearPromocion)
	g.Patch("/:id", edit, s.handleActualizarPromocion)
}

func (s *Server) handlePromociones(c *fiber.Ctx) error {
	return c.JSON(s.svc.Promociones(empresaIDOf(c)))
}

// promocionBody es el cuerpo de creación/edición de una promoción.
type promocionBody struct {
	Nombre   string `json:"nombre"`
	Tipo     string `json:"tipo"`
	Titulo   string `json:"titulo"`
	Subtexto string `json:"subtexto"`
	Imagen   string `json:"imagen"`
	Desde    string `json:"desde"`
	Hasta    string `json:"hasta"`
	Activa   bool   `json:"activa"`
	Orden    int    `json:"orden"`
}

func (b promocionBody) entrada() application.EntradaPromocion {
	return application.EntradaPromocion{
		Nombre: b.Nombre, Tipo: b.Tipo, Titulo: b.Titulo, Subtexto: b.Subtexto,
		Imagen: b.Imagen, Desde: b.Desde, Hasta: b.Hasta, Activa: b.Activa, Orden: b.Orden,
	}
}

func (s *Server) handleCrearPromocion(c *fiber.Ctx) error {
	var in promocionBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearPromocion(empresaIDOf(c), principalOf(c).UserID, origen(c), in.entrada())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarPromocion(c *fiber.Ctx) error {
	var in promocionBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarPromocion(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.entrada())
	if err != nil {
		if errors.Is(err, application.ErrPromocionNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}
