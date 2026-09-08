package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/usuario"
)

/* registerTesoreria monta el módulo de Tesorería.
 *
 * Permisos (matriz de 02 §1): la Contadora tiene acceso TOTAL acá —es su módulo—,
 * la Dueña y el Desarrollador también. El Vendedor solo consulta lo que le sirve
 * para vender (qué le debe un cliente); el Cajero no entra: su mundo es la caja.
 */
func (s *Server) registerTesoreria(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora, usuario.RolVendedor)
	cobra := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)

	g := r.Group("/tesoreria", ver)
	g.Get("/por-cobrar", s.handlePorCobrar)
	// Cuentas por pagar es lectura del pasivo con proveedores: mismo gate de LECTURA
	// que por-cobrar (Dueña, Desarrollador, Contadora; el Vendedor también consulta).
	g.Get("/por-pagar", s.handlePorPagar)
	g.Get("/saldos", s.handleSaldosTesoreria)
	g.Get("/igtf", s.handleReporteIGTF)
	g.Get("/cobros", s.handleCobros)
	g.Post("/cobros", cobra, s.handleRegistrarCobro)
	g.Post("/cobros/:id/reversar", cobra, s.handleReversarCobro)
	// Pagos a proveedor: espejo de los cobros del lado del pasivo. Misma matriz de
	// permisos (la escritura de Tesorería la tienen Dueña, Desarrollador, Contadora).
	g.Get("/pagos-proveedor", s.handlePagosProveedor)
	g.Post("/pagos-proveedor", cobra, s.handleRegistrarPagoProveedor)
	g.Post("/pagos-proveedor/:id/reversar", cobra, s.handleReversarPagoProveedor)
}

// handlePorCobrar devuelve las facturas a crédito con saldo, con sus totales.
func (s *Server) handlePorCobrar(c *fiber.Ctx) error {
	return c.JSON(s.svc.CuentasPorCobrar(empresaIDOf(c)))
}

// handlePorPagar devuelve las órdenes de compra recibidas como pasivo con
// proveedores, agrupadas por proveedor, con sus totales.
func (s *Server) handlePorPagar(c *fiber.Ctx) error {
	return c.JSON(s.svc.CuentasPorPagar(empresaIDOf(c)))
}

// handleSaldosTesoreria devuelve lo cobrado por cuenta y el efectivo en caja.
func (s *Server) handleSaldosTesoreria(c *fiber.Ctx) error {
	return c.JSON(s.svc.SaldosDeTesoreria(empresaIDOf(c)))
}

// handleReporteIGTF devuelve las operaciones con IGTF y su total a declarar.
func (s *Server) handleReporteIGTF(c *fiber.Ctx) error {
	lineas, total := s.svc.ReporteIGTF(empresaIDOf(c))
	return c.JSON(fiber.Map{"operaciones": lineas, "total": total})
}

// handleCobros devuelve el histórico de cobros.
func (s *Server) handleCobros(c *fiber.Ctx) error {
	return c.JSON(s.svc.Cobros(empresaIDOf(c)))
}

// handleRegistrarCobro anexa un cobro contra una factura a crédito.
func (s *Server) handleRegistrarCobro(c *fiber.Ctx) error {
	var in struct {
		DocumentoID string  `json:"documentoId"`
		Monto       float64 `json:"monto"`
		Moneda      string  `json:"moneda"`
		Metodo      string  `json:"metodo"`
		CuentaID    string  `json:"cuentaId"`
		Referencia  string  `json:"referencia"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.RegistrarCobro(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), application.CobroEntrada{
		DocumentoID: in.DocumentoID, Monto: in.Monto, Moneda: in.Moneda,
		Metodo: in.Metodo, CuentaID: in.CuentaID, Referencia: in.Referencia,
	})
	if err != nil {
		if errors.Is(err, application.ErrDocumentoNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleReversarCobro anula un cobro mal registrado anexando su reverso.
func (s *Server) handleReversarCobro(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "motivo requerido"})
	}
	out, err := s.svc.ReversarCobro(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.Motivo)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handlePagosProveedor devuelve el histórico de pagos a proveedor.
func (s *Server) handlePagosProveedor(c *fiber.Ctx) error {
	return c.JSON(s.svc.PagosProveedor(empresaIDOf(c)))
}

// handleRegistrarPagoProveedor anexa un pago contra la deuda de un proveedor.
func (s *Server) handleRegistrarPagoProveedor(c *fiber.Ctx) error {
	var in struct {
		ProveedorID   string  `json:"proveedorId"`
		OrdenCompraID string  `json:"ordenCompraId"`
		MontoBs       float64 `json:"montoBs"`
		Metodo        string  `json:"metodo"`
		Referencia    string  `json:"referencia"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.RegistrarPagoProveedor(empresaIDOf(c), principalOf(c).UserID, origen(c), application.EntradaPagoProveedor{
		ProveedorID: in.ProveedorID, OrdenCompraID: in.OrdenCompraID, MontoBs: in.MontoBs,
		Metodo: in.Metodo, Referencia: in.Referencia,
	})
	if err != nil {
		if errors.Is(err, application.ErrProveedorNoPago) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleReversarPagoProveedor anula un pago mal registrado anexando su reverso.
func (s *Server) handleReversarPagoProveedor(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "motivo requerido"})
	}
	out, err := s.svc.ReversarPagoProveedor(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.Motivo)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}
