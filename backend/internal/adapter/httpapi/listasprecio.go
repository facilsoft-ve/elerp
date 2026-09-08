package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/listaprecio"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerListasPrecio monta el maestro de Listas de precio (venta y compra).
// Es un maestro editable (no ledger): se crean y se actualizan, nunca es
// append-only. El tipo (venta|compra) va en el query `?tipo=` de la lista.
func (s *Server) registerListasPrecio(r fiber.Router) {
	// Ver: Dueña/Desarrollador + Vendedor (listas de venta) + Contadora (compras).
	// El filtrado por tipo lo resuelve el query; el frontend consulta el tipo que
	// corresponde a cada módulo (Ventas → venta, Compras → compra).
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolContadora)
	// Mutaciones: solo Dueña/Desarrollador (config de precios de la empresa).
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/listas-precio", ver)
	g.Get("/", s.handleListasPrecio)
	g.Post("/", edit, s.handleCrearListaPrecio)
	g.Patch("/:id", edit, s.handleActualizarListaPrecio)
}

func (s *Server) handleListasPrecio(c *fiber.Ctx) error {
	return c.JSON(s.svc.ListasPrecio(empresaIDOf(c), c.Query("tipo")))
}

// listaPrecioBody es el cuerpo de creación/edición de una lista de precio.
type listaPrecioBody struct {
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"`
	Activa bool   `json:"activa"`
	Moneda string `json:"moneda"`
	Items  []struct {
		SKU    string  `json:"sku"`
		Precio float64 `json:"precio"`
	} `json:"items"`
}

func (b listaPrecioBody) entrada() application.EntradaListaPrecio {
	ent := application.EntradaListaPrecio{
		Nombre: b.Nombre, Tipo: b.Tipo, Activa: b.Activa, Moneda: b.Moneda,
	}
	for _, it := range b.Items {
		ent.Items = append(ent.Items, listaprecio.ItemLista{SKU: it.SKU, Precio: it.Precio})
	}
	return ent
}

func (s *Server) handleCrearListaPrecio(c *fiber.Ctx) error {
	var in listaPrecioBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearListaPrecio(empresaIDOf(c), principalOf(c).UserID, origen(c), in.entrada())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarListaPrecio(c *fiber.Ctx) error {
	var in listaPrecioBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarListaPrecio(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.entrada())
	if err != nil {
		if errors.Is(err, application.ErrListaPrecioNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}
