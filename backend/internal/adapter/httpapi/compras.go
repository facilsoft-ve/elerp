package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/proveedor"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerCompras monta el módulo Compras: proveedores, órdenes de compra y su
// recepción. Lectura para Dueña/Desarrollador/Contadora; escritura solo para
// Dueña/Desarrollador (la Contadora es consulta fuera de Contabilidad/Tesorería).
func (s *Server) registerCompras(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)
	escribir := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/compras", ver)

	// Proveedores (maestro).
	g.Get("/proveedores", s.handleProveedores)
	g.Post("/proveedores", escribir, s.handleCrearProveedor)
	g.Patch("/proveedores/:id", escribir, s.handleActualizarProveedor)
	g.Post("/proveedores/:id/desactivar", escribir, s.handleDesactivarProveedor)

	// Solicitudes de presupuesto (RFQ): el paso previo a la orden de compra. Pedir
	// cotización a varios proveedores, comparar y convertir en orden.
	g.Get("/solicitudes", s.handleSolicitudes)
	g.Get("/solicitudes/:id", s.handleSolicitud)
	g.Post("/solicitudes", escribir, s.handleCrearSolicitud)
	g.Patch("/solicitudes/:id", escribir, s.handleActualizarSolicitud)
	g.Post("/solicitudes/:id/enviar", escribir, s.handleEnviarSolicitud)
	g.Post("/solicitudes/:id/respuesta", escribir, s.handleRegistrarRespuestaSolicitud)
	g.Post("/solicitudes/:id/cerrar", escribir, s.handleCerrarSolicitud)
	g.Post("/solicitudes/:id/convertir", escribir, s.handleConvertirSolicitud)

	// Órdenes de compra.
	g.Get("/ordenes", s.handleOrdenesCompra)
	g.Get("/ordenes/:id", s.handleOrdenCompra)
	g.Post("/ordenes", escribir, s.handleCrearOrdenCompra)
	g.Post("/ordenes/:id/confirmar", escribir, s.handleConfirmarOrdenCompra)
	g.Post("/ordenes/:id/recibir", escribir, s.handleRecibirOrdenCompra)
	g.Post("/ordenes/:id/cancelar", escribir, s.handleCancelarOrdenCompra)
	g.Post("/ordenes/:id/factura", escribir, s.handleRegistrarFacturaCompra)

	// Facturas de compra (documento fiscal del proveedor).
	g.Get("/facturas", s.handleFacturasCompra)
	// Retención de IVA EMITIDA: tú retienes el IVA al proveedor sobre su factura de
	// compra (baja CxP / pasivo IVA por enterar). Acción sensible: Dueña/Desarrollador.
	g.Post("/facturas/:id/retencion-emitida", escribir, s.handleRetencionEmitida)

	// Notas de crédito/débito de PROVEEDOR: ajustan una factura de compra (la NC baja
	// la CxP, la ND la sube). Consulta para Dueña/Desarrollador/Contadora; emitirlas
	// —al ser materia fiscal/contable (crédito fiscal, Libro de compras)— también las
	// habilita a las tres, no solo a la escritura operativa de Compras.
	notas := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)
	g.Get("/notas", s.handleNotasCompra)
	g.Post("/facturas/:id/nota-credito", notas, s.handleEmitirNotaCreditoCompra)
	g.Post("/facturas/:id/nota-debito", notas, s.handleEmitirNotaDebitoCompra)
}

// notaCompraEntrada es el cuerpo compartido de emitir NC/ND de proveedor.
type notaCompraEntrada struct {
	Concepto        string  `json:"concepto"`
	Monto           float64 `json:"monto"`
	Exento          bool    `json:"exento"`
	NumeroDocumento string  `json:"numeroDocumento"`
	NumeroControl   string  `json:"numeroControl"`
	Fecha           string  `json:"fecha"`
}

func (in notaCompraEntrada) toApp() application.NotaCompraEntrada {
	return application.NotaCompraEntrada{
		Concepto: in.Concepto, Monto: in.Monto, Exento: in.Exento,
		NumeroDocumento: in.NumeroDocumento, NumeroControl: in.NumeroControl, Fecha: in.Fecha,
	}
}

func (s *Server) handleNotasCompra(c *fiber.Ctx) error {
	return c.JSON(s.svc.NotasCompra(empresaIDOf(c)))
}

func (s *Server) handleEmitirNotaCreditoCompra(c *fiber.Ctx) error {
	var in notaCompraEntrada
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.EmitirNotaCreditoCompra(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.toApp())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleEmitirNotaDebitoCompra(c *fiber.Ctx) error {
	var in notaCompraEntrada
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.EmitirNotaDebitoCompra(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.toApp())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleRetencionEmitida(c *fiber.Ctx) error {
	var in struct {
		Impuesto          string  `json:"impuesto"`
		NumeroComprobante string  `json:"numeroComprobante"`
		Fecha             string  `json:"fecha"`
		Porcentaje        float64 `json:"porcentaje"`
		Base              float64 `json:"base"`       // solo ISLR
		Concepto          string  `json:"concepto"`   // solo ISLR
		Sustraendo        float64 `json:"sustraendo"` // solo ISLR
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.RegistrarRetencionEmitida(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"),
		application.EntradaRetencion{
			Impuesto: in.Impuesto, NumeroComprobante: in.NumeroComprobante, Fecha: in.Fecha,
			Porcentaje: in.Porcentaje, Base: in.Base, Concepto: in.Concepto, Sustraendo: in.Sustraendo,
		})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// --- Proveedores ---

func (s *Server) handleProveedores(c *fiber.Ctx) error {
	return c.JSON(s.svc.Proveedores(empresaIDOf(c)))
}

func (s *Server) handleCrearProveedor(c *fiber.Ctx) error {
	var in proveedor.Proveedor
	if err := c.BodyParser(&in); err != nil || in.Nombre == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nombre requerido"})
	}
	out, err := s.svc.CrearProveedor(empresaIDOf(c), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarProveedor(c *fiber.Ctx) error {
	var in proveedor.Proveedor
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarProveedor(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleDesactivarProveedor(c *fiber.Ctx) error {
	out, err := s.svc.DesactivarProveedor(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// --- Órdenes de compra ---

func (s *Server) handleOrdenesCompra(c *fiber.Ctx) error {
	return c.JSON(s.svc.OrdenesCompra(empresaIDOf(c)))
}

func (s *Server) handleOrdenCompra(c *fiber.Ctx) error {
	o, ok := s.svc.OrdenCompra(empresaIDOf(c), c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "orden de compra no existe"})
	}
	return c.JSON(o)
}

func (s *Server) handleCrearOrdenCompra(c *fiber.Ctx) error {
	var in struct {
		ProveedorID     string `json:"proveedorId"`
		SedeID          string `json:"sedeId"`
		CondicionesPago string `json:"condicionesPago"`
		Notas           string `json:"notas"`
		Lineas          []struct {
			SKU           string  `json:"sku"`
			Cantidad      float64 `json:"cantidad"`
			CostoUnitario float64 `json:"costoUnitario"`
			Exento        bool    `json:"exento"`
		} `json:"lineas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	ent := application.EntradaOC{
		ProveedorID: in.ProveedorID, SedeID: in.SedeID,
		CondicionesPago: in.CondicionesPago, Notas: in.Notas,
	}
	for _, l := range in.Lineas {
		ent.Lineas = append(ent.Lineas, application.LineaOCEntrada{
			SKU: l.SKU, Cantidad: l.Cantidad, CostoUnitario: l.CostoUnitario, Exento: l.Exento,
		})
	}
	out, err := s.svc.CrearOrdenCompra(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), ent)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleConfirmarOrdenCompra(c *fiber.Ctx) error {
	out, err := s.svc.ConfirmarOrdenCompra(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleRecibirOrdenCompra(c *fiber.Ctx) error {
	var in struct {
		Lineas []struct {
			SKU      string  `json:"sku"`
			Cantidad float64 `json:"cantidad"`
		} `json:"lineas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	lineas := make([]application.LineaRecepcion, 0, len(in.Lineas))
	for _, l := range in.Lineas {
		lineas = append(lineas, application.LineaRecepcion{SKU: l.SKU, Cantidad: l.Cantidad})
	}
	out, err := s.svc.RecibirOrdenCompra(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), lineas)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleRegistrarFacturaCompra(c *fiber.Ctx) error {
	var in struct {
		NumeroFactura string  `json:"numeroFactura"`
		NumeroControl string  `json:"numeroControl"`
		Fecha         string  `json:"fecha"`
		IVA           float64 `json:"iva"` // opcional: IVA capturado del documento del proveedor
		Lineas        []struct {
			SKU           string  `json:"sku"`
			Cantidad      float64 `json:"cantidad"`
			CostoUnitario float64 `json:"costoUnitario"`
			Exento        bool    `json:"exento"`
		} `json:"lineas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	lineas := make([]application.LineaFacturaCompraEntrada, 0, len(in.Lineas))
	for _, l := range in.Lineas {
		lineas = append(lineas, application.LineaFacturaCompraEntrada{
			SKU: l.SKU, Cantidad: l.Cantidad, CostoUnitario: l.CostoUnitario, Exento: l.Exento,
		})
	}
	out, err := s.svc.RegistrarFacturaCompra(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), application.EntradaFacturaCompra{
		NumeroFactura: in.NumeroFactura, NumeroControl: in.NumeroControl, Fecha: in.Fecha, IVA: in.IVA, Lineas: lineas,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleFacturasCompra(c *fiber.Ctx) error {
	return c.JSON(s.svc.FacturasCompra(empresaIDOf(c)))
}

func (s *Server) handleCancelarOrdenCompra(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CancelarOrdenCompra(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.Motivo)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// --- Solicitudes de presupuesto (RFQ) ---

// solicitudEntrada es el cuerpo de crear/actualizar una solicitud.
type solicitudEntrada struct {
	SedeID string `json:"sedeId"`
	Notas  string `json:"notas"`
	Lineas []struct {
		SKU      string  `json:"sku"`
		Cantidad float64 `json:"cantidad"`
	} `json:"lineas"`
	Proveedores []string `json:"proveedores"`
}

func (in solicitudEntrada) toApp() application.EntradaSolicitud {
	ent := application.EntradaSolicitud{SedeID: in.SedeID, Notas: in.Notas, Proveedores: in.Proveedores}
	for _, l := range in.Lineas {
		ent.Lineas = append(ent.Lineas, application.LineaSolicitudEntrada{SKU: l.SKU, Cantidad: l.Cantidad})
	}
	return ent
}

func (s *Server) handleSolicitudes(c *fiber.Ctx) error {
	return c.JSON(s.svc.SolicitudesCompra(empresaIDOf(c)))
}

func (s *Server) handleSolicitud(c *fiber.Ctx) error {
	sol, ok := s.svc.SolicitudCompra(empresaIDOf(c), c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "solicitud de presupuesto no existe"})
	}
	return c.JSON(sol)
}

func (s *Server) handleCrearSolicitud(c *fiber.Ctx) error {
	var in solicitudEntrada
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearSolicitud(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), in.toApp())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarSolicitud(c *fiber.Ctx) error {
	var in solicitudEntrada
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarSolicitud(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.toApp())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleEnviarSolicitud(c *fiber.Ctx) error {
	out, err := s.svc.EnviarSolicitud(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleRegistrarRespuestaSolicitud(c *fiber.Ctx) error {
	var in struct {
		ProveedorID string `json:"proveedorId"`
		Lineas      []struct {
			SKU            string  `json:"sku"`
			PrecioUnitario float64 `json:"precioUnitario"`
		} `json:"lineas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	lineas := make([]application.RespuestaLineaEntrada, 0, len(in.Lineas))
	for _, l := range in.Lineas {
		lineas = append(lineas, application.RespuestaLineaEntrada{SKU: l.SKU, PrecioUnitario: l.PrecioUnitario})
	}
	out, err := s.svc.RegistrarRespuesta(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.ProveedorID, lineas)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleCerrarSolicitud(c *fiber.Ctx) error {
	out, err := s.svc.CerrarSolicitud(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleConvertirSolicitud(c *fiber.Ctx) error {
	var in struct {
		ProveedorID string `json:"proveedorId"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	sol, oc, err := s.svc.ConvertirEnOrden(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.ProveedorID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	// Devuelve la solicitud cerrada y la orden creada, para que el frontend pueda
	// navegar directo a la OC recién generada.
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"solicitud": sol, "orden": oc})
}
