package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerReportes monta el módulo Reportes y BI: agregaciones DERIVADAS del
// ledger, de solo lectura. Gate a Dueña / Desarrollador / Contadora — la
// Contadora entra como acceso del regulador (Providencia 000121), igual que en
// los libros fiscales. El tenant sale de empresaIDOf (empresaContext ya lo
// validó); `desde`/`hasta` son YYYY-MM-DD y el Service aplica el default (mes en
// curso) cuando faltan o son inválidas.
func (s *Server) registerReportes(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)

	g := r.Group("/reportes", ver)
	g.Get("/panel", s.handlePanelEjecutivo)
	g.Get("/ventas", s.handleReporteVentas)
	g.Get("/inventario", s.handleReporteInventario)
	g.Get("/compras", s.handleReporteCompras)
	g.Get("/cobranza", s.handleReporteCobranza)
}

// handlePanelEjecutivo consolida ventas del mes, inventario, cobranza y compras.
func (s *Server) handlePanelEjecutivo(c *fiber.Ctx) error {
	return c.JSON(s.svc.PanelEjecutivo(empresaIDOf(c)))
}

// handleReporteVentas devuelve la vista de ventas del rango (defaults al mes).
func (s *Server) handleReporteVentas(c *fiber.Ctx) error {
	desde := c.Query("desde")
	hasta := c.Query("hasta")
	return c.JSON(s.svc.ReporteVentas(empresaIDOf(c), desde, hasta))
}

// handleReporteInventario devuelve la valorización y rotación del inventario.
func (s *Server) handleReporteInventario(c *fiber.Ctx) error {
	return c.JSON(s.svc.ReporteInventario(empresaIDOf(c)))
}

// handleReporteCompras devuelve el resumen de compras del rango y el pipeline.
func (s *Server) handleReporteCompras(c *fiber.Ctx) error {
	desde := c.Query("desde")
	hasta := c.Query("hasta")
	return c.JSON(s.svc.ReporteCompras(empresaIDOf(c), desde, hasta))
}

// handleReporteCobranza devuelve el aging de la cartera por cobrar.
func (s *Server) handleReporteCobranza(c *fiber.Ctx) error {
	return c.JSON(s.svc.ReporteCobranza(empresaIDOf(c)))
}
