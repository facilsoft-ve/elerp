package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerCupones monta el maestro de Cupones de descuento (submódulo de Ventas).
// Es un maestro editable (no ledger): se crean y actualizan, nunca es append-only.
// El endpoint /validar resuelve un código contra un subtotal y devuelve el
// descuento en Bs (quien cobra/cotiza baja el precioUnitario de las líneas con él).
func (s *Server) registerCupones(r fiber.Router) {
	// Ver / validar: Dueña/Desarrollador + Vendedor (los que cobran/cotizan).
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor)
	// Mutaciones: solo Dueña/Desarrollador (config de descuentos de la empresa).
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/ventas/cupones", ver)
	g.Get("/", s.handleCupones)
	g.Post("/validar", s.handleValidarCupon)
	g.Post("/", edit, s.handleCrearCupon)
	g.Patch("/:id", edit, s.handleActualizarCupon)
}

func (s *Server) handleCupones(c *fiber.Ctx) error {
	return c.JSON(s.svc.Cupones(empresaIDOf(c)))
}

// cuponBody es el cuerpo de creación/edición de un cupón.
type cuponBody struct {
	Codigo      string  `json:"codigo"`
	Descripcion string  `json:"descripcion"`
	Tipo        string  `json:"tipo"`
	Valor       float64 `json:"valor"`
	MontoMinimo float64 `json:"montoMinimo"`
	Desde       string  `json:"desde"`
	Hasta       string  `json:"hasta"`
	Activo      bool    `json:"activo"`
	UsosMax     int     `json:"usosMax"`
}

func (b cuponBody) entrada() application.EntradaCupon {
	return application.EntradaCupon{
		Codigo: b.Codigo, Descripcion: b.Descripcion, Tipo: b.Tipo,
		Valor: b.Valor, MontoMinimo: b.MontoMinimo, Desde: b.Desde, Hasta: b.Hasta,
		Activo: b.Activo, UsosMax: b.UsosMax,
	}
}

func (s *Server) handleCrearCupon(c *fiber.Ctx) error {
	var in cuponBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearCupon(empresaIDOf(c), principalOf(c).UserID, origen(c), in.entrada())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarCupon(c *fiber.Ctx) error {
	var in cuponBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarCupon(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.entrada())
	if err != nil {
		if errors.Is(err, application.ErrCuponNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleValidarCupon(c *fiber.Ctx) error {
	var in struct {
		Codigo   string  `json:"codigo"`
		Subtotal float64 `json:"subtotal"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	res, err := s.svc.ValidarCupon(empresaIDOf(c), in.Codigo, in.Subtotal)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}
