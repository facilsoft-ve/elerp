package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerVentas monta el módulo Ventas (forma libre): cotización → confirmar
// (pedido/prefactura) → facturar.
func (s *Server) registerVentas(r fiber.Router) {
	// Ver: Dueña/Desarrollador/Vendedor + Contadora (consulta).
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolContadora)
	// Mutaciones: Dueña/Desarrollador/Vendedor. La Contadora es de solo lectura.
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor)

	g := r.Group("/ventas", ver)
	g.Get("/cotizaciones", s.handleCotizaciones)
	g.Get("/cotizaciones/:id", s.handleCotizacion)
	g.Post("/cotizaciones", edit, s.handleCrearCotizacion)
	g.Patch("/cotizaciones/:id", edit, s.handleActualizarCotizacion)
	g.Post("/cotizaciones/:id/confirmar", edit, s.handleConfirmarCotizacion)
	g.Post("/cotizaciones/:id/facturar", edit, s.handleFacturarCotizacion)
	g.Post("/cotizaciones/:id/cancelar", edit, s.handleCancelarCotizacion)
}

func (s *Server) handleCotizaciones(c *fiber.Ctx) error {
	return c.JSON(s.svc.Cotizaciones(empresaIDOf(c)))
}

func (s *Server) handleCotizacion(c *fiber.Ctx) error {
	ct, ok := s.svc.Cotizacion(empresaIDOf(c), c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "cotización no existe"})
	}
	return c.JSON(ct)
}

// cotizacionBody es el cuerpo de creación/edición de una cotización. La tasa NO
// se lee del cliente: la pone el servidor (R9).
type cotizacionBody struct {
	ClienteID string `json:"clienteId"`
	Lineas    []struct {
		SKU            string  `json:"sku"`
		Cantidad       float64 `json:"cantidad"`
		PrecioUnitario float64 `json:"precioUnitario"`
		Descripcion    string  `json:"descripcion"`
		Descuento      float64 `json:"descuento"`
	} `json:"lineas"`
	Moneda           string `json:"moneda"`
	Validez          string `json:"validez"`
	CondicionesPago  string `json:"condicionesPago"`
	Terminos         string `json:"terminos"`
	Notas            string `json:"notas"`
	ListaPrecio      string `json:"listaPrecio"`
	DireccionEntrega string `json:"direccionEntrega"`
	SedeDespacho     string `json:"sedeDespacho"`
	CuponCodigo      string `json:"cuponCodigo"`
}

func (b cotizacionBody) entrada() application.EntradaCotizacion {
	ent := application.EntradaCotizacion{
		ClienteID: b.ClienteID, Moneda: b.Moneda, Validez: b.Validez,
		CondicionesPago: b.CondicionesPago, Terminos: b.Terminos, Notas: b.Notas,
		CuponCodigo: b.CuponCodigo,
		ListaPrecio: b.ListaPrecio, DireccionEntrega: b.DireccionEntrega, SedeDespacho: b.SedeDespacho,
	}
	for _, l := range b.Lineas {
		ent.Lineas = append(ent.Lineas, application.LineaEntrada{
			SKU: l.SKU, Cantidad: l.Cantidad, PrecioUnitario: l.PrecioUnitario,
			Descripcion: l.Descripcion, Descuento: l.Descuento,
		})
	}
	return ent
}

func (s *Server) handleCrearCotizacion(c *fiber.Ctx) error {
	var in cotizacionBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearCotizacion(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), in.entrada())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarCotizacion(c *fiber.Ctx) error {
	var in cotizacionBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarCotizacion(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.entrada())
	if err != nil {
		if errors.Is(err, application.ErrCotizacionNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleConfirmarCotizacion(c *fiber.Ctx) error {
	out, err := s.svc.ConfirmarCotizacion(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		if errors.Is(err, application.ErrCotizacionNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleFacturarCotizacion(c *fiber.Ctx) error {
	var in struct {
		Pagos []struct {
			Metodo     string  `json:"metodo"`
			CuentaID   string  `json:"cuentaId"`
			Monto      float64 `json:"monto"`
			Moneda     string  `json:"moneda"`
			Referencia string  `json:"referencia"`
		} `json:"pagos"`
		// Venta a crédito: lo que no se cobró queda por cobrar en Tesorería.
		Credito     bool `json:"credito"`
		DiasCredito int  `json:"diasCredito"`
		// Vuelto DECLARADO por quien factura (todos opcionales), igual que el POS: en
		// qué moneda y por qué medio se devuelve el excedente. Vacíos ⇒ vuelto derivado.
		VueltoMoneda   string `json:"vueltoMoneda"`
		VueltoMetodo   string `json:"vueltoMetodo"`
		VueltoBanco    string `json:"vueltoBanco"`
		VueltoCedula   string `json:"vueltoCedula"`
		VueltoTelefono string `json:"vueltoTelefono"`
		// Vuelto MIXTO: el excedente repartido en varias partes (misma vía que el POS).
		VueltoPartes []vueltoParteReq `json:"vueltoPartes"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	ent := application.EntradaFacturacion{
		Credito: in.Credito, DiasCredito: in.DiasCredito,
		VueltoMoneda: in.VueltoMoneda, VueltoMetodo: in.VueltoMetodo,
		VueltoBanco: in.VueltoBanco, VueltoCedula: in.VueltoCedula, VueltoTelefono: in.VueltoTelefono,
		VueltoPartes: vueltoPartesDe(in.VueltoPartes),
	}
	for _, p := range in.Pagos {
		ent.Pagos = append(ent.Pagos, application.PagoEntrada{
			Metodo: p.Metodo, CuentaID: p.CuentaID, Monto: p.Monto, Moneda: p.Moneda, Referencia: p.Referencia,
		})
	}
	ct, doc, err := s.svc.FacturarCotizacion(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), ent)
	if err != nil {
		if errors.Is(err, application.ErrCotizacionNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"cotizacion": ct, "documento": doc})
}

func (s *Server) handleCancelarCotizacion(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CancelarCotizacion(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.Motivo)
	if err != nil {
		if errors.Is(err, application.ErrCotizacionNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}
