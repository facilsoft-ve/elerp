package httpapi

import (
	"strings"

	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/facturaciondigital"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/usuario"
	"github.com/mornix/elerp/internal/domain/venta"
)

func (s *Server) registerFiscal(r fiber.Router) {
	// Roles con acceso al módulo Fiscal (Contadora en consulta).
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolCajero, usuario.RolContadora)
	// POS: puede facturar todo menos Contadora.
	pos := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolCajero)
	// Anular: acción sensible, solo Dueña/Desarrollador.
	admin := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/fiscal", ver)
	g.Get("/documentos", s.handleDocumentos)
	g.Get("/documentos/:id", s.handleDocumento)
	g.Post("/documentos", pos, s.handleEmitir)
	g.Post("/documentos/:id/anular", admin, s.handleAnular)
	g.Post("/documentos/:id/nota-credito", admin, s.handleNotaCredito)
	// Notas de crédito que NO devuelven mercancía (notas de la contadora, 15:18 y
	// 15:19). Van en rutas propias y no como una variante del cuerpo de arriba
	// porque llevan datos distintos —un monto, o el precio correcto por
	// producto— y mezclarlas haría un cuerpo lleno de campos opcionales donde
	// nadie sabría cuál manda.
	g.Post("/documentos/:id/nota-credito/descuento", admin, s.handleNCDescuento)
	g.Post("/documentos/:id/nota-credito/ajuste-precio", admin, s.handleNCAjustePrecio)
	// Catálogo de motivos: la pantalla no puede llevar la lista escrita a mano o
	// se desincroniza del servidor, que es quien valida.
	g.Get("/motivos-nota", s.handleMotivosNota)
	g.Post("/documentos/:id/nota-debito", admin, s.handleNotaDebito)

	// Libros fiscales (Libro de Ventas / Libro de Compras): reportes DERIVADOS del
	// ledger, de solo lectura, por contribuyente y período mensual. Gate al grupo
	// `ver`, que INCLUYE a la Contadora: leer los libros es el acceso del regulador
	// (Providencia 000121). El Vendedor/Cajero pasan el gate del grupo pero se
	// restringen aquí a Dueña/Desarrollador/Contadora (los libros son consulta de
	// cumplimiento, no del mostrador).
	libros := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)
	g.Get("/libros/ventas", libros, s.handleLibroVentas)
	g.Get("/libros/compras", libros, s.handleLibroCompras)
	// Libro de Inventario: existencia valorizada a la fecha (proyección del
	// ledger append-only), consolidando todas las sedes. Mismo gate de consulta.
	g.Get("/libro-inventario", libros, s.handleLibroInventario)

	// Cierre Z (reporte fiscal diario por sede): la lista la ve cualquiera con
	// acceso al módulo; emitir y previsualizar son de Dueña/Desarrollador (cierre
	// fiscal, acción sensible append-only).
	g.Get("/cierres-z", s.handleCierresZ)
	g.Get("/cierres-z/preview", admin, s.handlePreviewCierreZ)
	g.Post("/cierres-z", admin, s.handleEmitirCierreZ)

	// Retenciones de IVA. Consulta y registro de la recibida (sobre una factura de
	// venta): Dueña/Desarrollador/Contadora (es materia de cumplimiento fiscal, no
	// del mostrador). La emitida (sobre una factura de COMPRA) vive en el módulo
	// Compras. Vendedor/Cajero pasan el gate del grupo pero se quedan fuera aquí.
	ret := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)
	g.Get("/retenciones", ret, s.handleRetenciones)
	g.Post("/documentos/:id/retencion-recibida", ret, s.handleRetencionRecibida)

	// CRM mínimo (clientes) — Cajero puede dar de alta rápido desde el POS.
	// Ventas en espera (flujo 2.6): el carrito apartado del mostrador. Lo usan los
	// roles que cobran; cualquiera de ellos puede retomar una de su sede.
	esp := r.Group("/pos/ventas-en-espera", pos)
	esp.Get("", s.handleVentasEnEspera)
	esp.Post("", s.handleDejarEnEspera)
	esp.Post("/:id/retomar", s.handleRetomarVenta)
	esp.Delete("/:id", s.handleDescartarVenta)

	crm := r.Group("/crm", s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolCajero, usuario.RolContadora))
	crm.Get("/clientes", s.handleClientes)
	// Crear y editar comparten roles: el maestro de clientes es editable (no es
	// un ledger). La Contadora ve la lista pero no da de alta ni edita.
	crmEscribe := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolCajero)
	crm.Post("/clientes", crmEscribe, s.handleCrearCliente)
	crm.Patch("/clientes/:id", crmEscribe, s.handleActualizarCliente)

	// Tesorería (cuentas de cobro) — destinos de pago del POS.
	tes := r.Group("/tesoreria", s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora, usuario.RolVendedor))
	tes.Get("/cuentas-cobro", s.handleCuentasCobro)
	tes.Post("/cuentas-cobro", s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora), s.handleCrearCuentaCobro)

	// Configuración › Métodos de pago. Los ven quienes acceden a Ajustes
	// (Dueña/Desarrollador/Contadora); solo la Dueña/Desarrollador los modifican.
	cfg := r.Group("/config", s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora))
	cfgAdmin := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)
	cfg.Get("/metodos-pago", s.handleMetodosPago)
	cfg.Post("/metodos-pago", cfgAdmin, s.handleCrearMetodoPago)
	cfg.Patch("/metodos-pago/:id", cfgAdmin, s.handleActualizarMetodoPago)
	cfg.Delete("/metodos-pago/:id", cfgAdmin, s.handleEliminarMetodoPago)

	// Configuración › Dispositivos fiscales. Igual gate que métodos de pago: los
	// ve quien accede a Ajustes; solo Dueña/Desarrollador los administran. El
	// enlace real con la impresora lo hace el agente fiscal local (fuera del backend).
	// El CATÁLOGO precargado (marcas/modelos del mercado venezolano) es dato de
	// referencia, no del tenant: lo consume tanto esta pantalla como la de
	// comanderas del módulo Restaurante. Va antes que "/dispositivos/:id" por
	// claridad; no colisiona porque aquel es PATCH.
	// Configuración › Impuestos y alícuotas. El MAESTRO de tasas de IVA con su
	// vigencia. Lo VE quien accede a Ajustes (la Contadora incluida: es su
	// herramienta); solo Dueña/Desarrollador lo modifican.
	cfg.Get("/alicuotas", s.handleAlicuotas)
	cfg.Post("/alicuotas", cfgAdmin, s.handleCrearAlicuota)
	cfg.Patch("/alicuotas/:id", cfgAdmin, s.handleActualizarAlicuota)

	// Maestro de CONCEPTOS ISLR. Solo lectura por ahora: lo consume el selector
	// del modal de retención, que es donde la tarifa dejaba de tecetarse. La
	// edición de la tabla es material de Configuración y va aparte.
	// "/sugerencia" resuelve la tarifa y el monto EN EL SERVIDOR: la pantalla
	// muestra, no calcula.
	cfg.Get("/conceptos-islr", s.handleConceptosISLR)
	cfg.Get("/conceptos-islr/sugerencia", s.handleSugerenciaRetencionISLR)

	cfg.Get("/dispositivos/catalogo", s.handleCatalogoDispositivos)
	cfg.Get("/dispositivos", s.handleDispositivos)
	cfg.Post("/dispositivos", cfgAdmin, s.handleCrearDispositivo)
	cfg.Patch("/dispositivos/:id", cfgAdmin, s.handleActualizarDispositivo)
	cfg.Post("/dispositivos/:id/desactivar", cfgAdmin, s.handleDesactivarDispositivo)

	// Configuración › Series y numeración. Es información de cumplimiento fiscal:
	// tanto VER como FIJAR quedan restringidos a Dueña/Desarrollador (la Contadora
	// pasa el gate del grupo pero la queda fuera cfgAdmin, igual que en métodos).
	cfg.Get("/numeracion", cfgAdmin, s.handleNumeracion)
	cfg.Post("/numeracion/fijar", cfgAdmin, s.handleFijarNumeracion)
	cfg.Post("/numeracion/serie", cfgAdmin, s.handleConfigurarSerie)

	// Configuración › Número de Control (rango autorizado por el SENIAT).
	cfg.Get("/numero-control", cfgAdmin, s.handleNumeroControl)
	cfg.Post("/numero-control", cfgAdmin, s.handleConfigurarNumeroControl)
}

// handleNumeroControl devuelve el rango del Número de Control y el correlativo vivo.
func (s *Server) handleNumeroControl(c *fiber.Ctx) error {
	return c.JSON(s.svc.EstadoNumeroControl(empresaIDOf(c)))
}

// handleConfigurarNumeroControl fija el prefijo y el rango [desde, hasta].
func (s *Server) handleConfigurarNumeroControl(c *fiber.Ctx) error {
	var in struct {
		Prefijo string `json:"prefijo"`
		Desde   int    `json:"desde"`
		Hasta   int    `json:"hasta"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if _, err := s.svc.ConfigurarNumeroControl(empresaIDOf(c), principalOf(c).UserID, origen(c), in.Prefijo, in.Desde, in.Hasta); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(s.svc.EstadoNumeroControl(empresaIDOf(c)))
}

// handleNumeracion devuelve el estado de los correlativos: un bloque por tipo de
// documento —con su prefijo, su rango y el contador de cada sede— más las series
// internas (cotizaciones, órdenes de compra, transferencias, cierres Z).
//
// Los nombres de sede se enriquecen con el maestro del tenant: el Service conoce
// las sedes para la presencia estricta, pero el nombre visible es del tenant y
// una sede borrada tiene que decirlo en vez de mostrar un id suelto.
func (s *Server) handleNumeracion(c *fiber.Ctx) error {
	empID := empresaIDOf(c)
	estado := s.svc.EstadoNumeracion(empID)
	nombres := map[string]string{}
	for _, sd := range s.tenancy.Sedes(empID) {
		nombres[sd.ID] = sd.Nombre
	}
	nombrar := func(id, actual string) string {
		if n := nombres[id]; n != "" {
			return n
		}
		if id != "" {
			return "Sede eliminada"
		}
		return actual
	}
	for i := range estado.Tipos {
		for j := range estado.Tipos[i].Contadores {
			cont := &estado.Tipos[i].Contadores[j]
			cont.SedeNombre = nombrar(cont.SedeID, cont.SedeNombre)
		}
	}
	for i := range estado.Otras {
		estado.Otras[i].SedeNombre = nombrar(estado.Otras[i].SedeID, estado.Otras[i].SedeNombre)
	}
	return c.JSON(estado)
}

// handleConfigurarSerie fija el prefijo y el rango autorizado de un tipo de
// documento. Cambiar el prefijo ABRE UNA SERIE NUEVA: los documentos ya emitidos
// conservan el suyo y el contador del prefijo nuevo arranca donde diga el rango.
func (s *Server) handleConfigurarSerie(c *fiber.Ctx) error {
	var in struct {
		Tipo    string `json:"tipo"`
		Prefijo string `json:"prefijo"`
		Desde   int    `json:"desde"`
		Hasta   int    `json:"hasta"`
	}
	if err := c.BodyParser(&in); err != nil || in.Tipo == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tipo de documento requerido"})
	}
	if err := s.svc.ConfigurarSerie(empresaIDOf(c), principalOf(c).UserID, origen(c),
		in.Tipo, in.Prefijo, in.Desde, in.Hasta); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(s.svc.EstadoNumeracion(empresaIDOf(c)))
}

// handleFijarNumeracion fija el próximo folio de una serie, solo hacia adelante.
// Devuelve 200 con el estado actualizado, o 400 con un mensaje claro si intenta
// retroceder o reusar un folio.
func (s *Server) handleFijarNumeracion(c *fiber.Ctx) error {
	var in struct {
		SedeID  string `json:"sedeId"`
		Serie   string `json:"serie"`
		Proximo int    `json:"proximo"`
	}
	if err := c.BodyParser(&in); err != nil || in.SedeID == "" || in.Serie == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sede, serie y próximo folio requeridos"})
	}
	// La sede tiene que pertenecer al tenant activo: no se fija numeración de otra.
	if !s.tenancy.SedeValida(empresaIDOf(c), in.SedeID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sede inválida"})
	}
	if err := s.svc.FijarNumeracion(empresaIDOf(c), principalOf(c).UserID, origen(c), in.SedeID, in.Serie, in.Proximo); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return s.handleNumeracion(c)
}

// handleCierresZ lista los cierres Z de la sede activa (ordenados por Nº Z).
func (s *Server) handleCierresZ(c *fiber.Ctx) error {
	return c.JSON(s.svc.CierresZ(empresaIDOf(c), sedeIDOf(c)))
}

// handlePreviewCierreZ devuelve lo que se cerraría AHORA para la sede activa: los
// totales del rango pendiente y cuántos documentos incluiría, sin persistir.
func (s *Server) handlePreviewCierreZ(c *fiber.Ctx) error {
	totales, cantidad, err := s.svc.PreviewCierreZ(empresaIDOf(c), sedeIDOf(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"totales": totales, "cantidadDocumentos": cantidad})
}

// handleEmitirCierreZ consolida los documentos pendientes de la sede en un nuevo
// Cierre Z inmutable. 400 con ErrNadaQueCerrar si no hay documentos nuevos.
func (s *Server) handleEmitirCierreZ(c *fiber.Ctx) error {
	out, err := s.svc.EmitirCierreZ(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleRetenciones lista los comprobantes de retención (IVA e ISLR) del tenant.
func (s *Server) handleRetenciones(c *fiber.Ctx) error {
	return c.JSON(s.svc.Retenciones(empresaIDOf(c)))
}

// handleRetencionRecibida registra la retención que un cliente agente de retención
// aplicó sobre una factura de VENTA propia (baja CxC / activo IVA a favor).
func (s *Server) handleRetencionRecibida(c *fiber.Ctx) error {
	var in struct {
		Impuesto          string  `json:"impuesto"`
		NumeroComprobante string  `json:"numeroComprobante"`
		Fecha             string  `json:"fecha"`
		Porcentaje        float64 `json:"porcentaje"`
		Base              float64 `json:"base"`       // solo ISLR
		Concepto          string  `json:"concepto"`   // solo ISLR
		Sustraendo        float64 `json:"sustraendo"` // solo ISLR
		// ConceptoCodigo y Sujeto resuelven la tarifa contra el MAESTRO de
		// conceptos. Cuando vienen, el servicio toma de ahí el porcentaje y el
		// sustraendo y PISA lo que haya llegado en los campos de arriba: saberse
		// la tabla del reglamento de memoria es justo lo que el maestro evita.
		// Vacíos ⇒ comportamiento anterior (se teclea todo).
		ConceptoCodigo string `json:"conceptoCodigo"` // solo ISLR
		Sujeto         string `json:"sujeto"`         // solo ISLR
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.RegistrarRetencionRecibida(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"),
		application.EntradaRetencion{
			Impuesto: in.Impuesto, NumeroComprobante: in.NumeroComprobante, Fecha: in.Fecha,
			Porcentaje: in.Porcentaje, Base: in.Base, Concepto: in.Concepto, Sustraendo: in.Sustraendo,
			ConceptoCodigo: in.ConceptoCodigo, Sujeto: in.Sujeto,
		})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// periodoLibro lee anio/mes de la query con default al mes actual (UTC) si
// faltan o son inválidos. El contribuyente es el tenant activo (empresaIDOf).
func periodoLibro(c *fiber.Ctx) (int, int) {
	now := time.Now().UTC()
	anio := c.QueryInt("anio", now.Year())
	mes := c.QueryInt("mes", int(now.Month()))
	if anio <= 0 {
		anio = now.Year()
	}
	if mes < 1 || mes > 12 {
		mes = int(now.Month())
	}
	return anio, mes
}

// handleLibroVentas devuelve el Libro de Ventas del contribuyente para el mes
// pedido (consolidando todas las sedes). Solo lectura.
func (s *Server) handleLibroVentas(c *fiber.Ctx) error {
	anio, mes := periodoLibro(c)
	return c.JSON(s.svc.LibroVentas(empresaIDOf(c), anio, mes))
}

// handleLibroCompras devuelve el Libro de Compras del contribuyente para el mes
// pedido (best-effort sobre órdenes de compra recibidas). Solo lectura.
func (s *Server) handleLibroCompras(c *fiber.Ctx) error {
	anio, mes := periodoLibro(c)
	return c.JSON(s.svc.LibroCompras(empresaIDOf(c), anio, mes))
}

// handleLibroInventario devuelve el Libro de Inventario del contribuyente: la
// existencia valorizada a la fecha (proyección del ledger), consolidando todas
// las sedes. Solo lectura.
func (s *Server) handleLibroInventario(c *fiber.Ctx) error {
	return c.JSON(s.svc.LibroInventario(empresaIDOf(c)))
}

func (s *Server) handleDocumentos(c *fiber.Ctx) error {
	return c.JSON(s.svc.Documentos(empresaIDOf(c)))
}

func (s *Server) handleDocumento(c *fiber.Ctx) error {
	d, ok := s.svc.Documento(empresaIDOf(c), c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "documento no existe"})
	}
	return c.JSON(d)
}

// vueltoParteReq es una parte del vuelto MIXTO recibida por HTTP (POS y Ventas).
type vueltoParteReq struct {
	Moneda   string  `json:"moneda"`
	Metodo   string  `json:"metodo"`
	Monto    float64 `json:"monto"`
	Banco    string  `json:"banco"`
	Cedula   string  `json:"cedula"`
	Telefono string  `json:"telefono"`
}

// vueltoPartesDe traduce las partes del vuelto del contrato REST a la entrada de
// la capa de aplicación. Vacío ⇒ nil (se usa el vuelto de una sola parte).
func vueltoPartesDe(in []vueltoParteReq) []application.VueltoParteEntrada {
	if len(in) == 0 {
		return nil
	}
	out := make([]application.VueltoParteEntrada, 0, len(in))
	for _, p := range in {
		out = append(out, application.VueltoParteEntrada{
			Moneda: p.Moneda, Metodo: p.Metodo, Monto: p.Monto,
			Banco: p.Banco, Cedula: p.Cedula, Telefono: p.Telefono,
		})
	}
	return out
}

func (s *Server) handleEmitir(c *fiber.Ctx) error {
	var in struct {
		ClienteID string `json:"clienteId"`
		Lineas    []struct {
			SKU            string  `json:"sku"`
			Cantidad       float64 `json:"cantidad"`
			PrecioUnitario float64 `json:"precioUnitario"`
		} `json:"lineas"`
		Pagos []struct {
			Metodo     string  `json:"metodo"`
			CuentaID   string  `json:"cuentaId"`
			Monto      float64 `json:"monto"`
			Moneda     string  `json:"moneda"`
			Referencia string  `json:"referencia"`
		} `json:"pagos"`
		Moneda       string `json:"moneda"`
		Contingencia bool   `json:"contingencia"`
		// Venta a crédito: lo que no se cobró queda por cobrar en Tesorería.
		Credito     bool `json:"credito"`
		DiasCredito int  `json:"diasCredito"`
		// Vuelto DECLARADO por la caja (todos opcionales): en qué moneda y por qué
		// medio se devuelve el excedente. Vacíos ⇒ vuelto derivado y por efectivo.
		VueltoMoneda   string `json:"vueltoMoneda"`
		VueltoMetodo   string `json:"vueltoMetodo"`
		VueltoBanco    string `json:"vueltoBanco"`
		VueltoCedula   string `json:"vueltoCedula"`
		VueltoTelefono string `json:"vueltoTelefono"`
		// Vuelto MIXTO: el excedente repartido en varias partes (moneda + medio +
		// monto). Si viene, manda sobre los campos únicos de arriba.
		VueltoPartes []vueltoParteReq `json:"vueltoPartes"`
		// CuponCodigo: cupón aplicado a la venta. El front ya bajó el precio de las
		// líneas con el descuento; este código CONSUME el cupón al emitir (UsosMax).
		CuponCodigo string `json:"cuponCodigo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	// La tasa NO se lee del cuerpo: la pone el servidor desde la fuente oficial
	// (R9). Si el POS la mandara, el IGTF dependería del navegador.
	ent := application.EmitirEntrada{
		ClienteID: in.ClienteID, Moneda: in.Moneda, Contingencia: in.Contingencia,
		Credito: in.Credito, DiasCredito: in.DiasCredito,
		VueltoMoneda: in.VueltoMoneda, VueltoMetodo: in.VueltoMetodo,
		VueltoBanco: in.VueltoBanco, VueltoCedula: in.VueltoCedula, VueltoTelefono: in.VueltoTelefono,
		VueltoPartes: vueltoPartesDe(in.VueltoPartes),
		CuponCodigo:  in.CuponCodigo,
	}
	for _, l := range in.Lineas {
		ent.Lineas = append(ent.Lineas, application.LineaEntrada{SKU: l.SKU, Cantidad: l.Cantidad, PrecioUnitario: l.PrecioUnitario})
	}
	for _, p := range in.Pagos {
		ent.Pagos = append(ent.Pagos, application.PagoEntrada{
			Metodo: p.Metodo, CuentaID: p.CuentaID, Monto: p.Monto, Moneda: p.Moneda, Referencia: p.Referencia,
		})
	}
	emp, _ := c.Locals("empresa").(empresa.Empresa)
	out, err := s.svc.EmitirFactura(empresaIDOf(c), sedeIDOf(c), emp.Modalidad, principalOf(c).UserID, origen(c), ent)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	/* FACTURACIÓN DIGITAL: se encola DESPUÉS de emitir y nunca antes.
	 *
	 * El cobro no puede depender de que la imprenta conteste. Si está caída, la
	 * venta igual se cobró y la factura sale cuando vuelva — eso es lo que compra
	 * el outbox. Encolar es una escritura local: no hace red, así que no demora
	 * la respuesta al cajero.
	 *
	 * El canal sale de dónde vino la venta: el mostrador y el módulo de ventas se
	 * activan por separado. */
	if emi, encolada := s.svc.EncolarEmision(empresaIDOf(c), canalDeVenta(c), out); encolada {
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"documento": out,
			// La emisión viaja con el documento para que el POS pueda imprimir el
			// ticket con su QR sin una segunda vuelta al servidor.
			"facturaDigital": fiber.Map{
				"token":  emi.Token,
				"estado": emi.Estado,
				"url":    s.urlPublicaFactura(emi.Token),
			},
		})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// canalDeVenta distingue el mostrador del módulo de ventas. El POS lo declara
// con una cabecera; sin ella, la emisión vino del módulo de ventas.
func canalDeVenta(c *fiber.Ctx) string {
	if strings.EqualFold(string(c.Request().Header.Peek("X-Canal")), "pos") {
		return facturaciondigital.CanalPOS
	}
	return facturaciondigital.CanalVentas
}

// urlPublicaFactura arma el enlace que va en el QR del ticket.
func (s *Server) urlPublicaFactura(token string) string {
	base := strings.TrimRight(s.cfg.FrontendURL, "/")
	if base == "" || strings.Contains(base, "*") {
		// Sin URL pública declarada se devuelve la ruta relativa: es mejor que un
		// enlace con el origen equivocado, que llevaría al cliente a ningún lado.
		return "/f/" + token
	}
	return base + "/f/" + token
}

func (s *Server) handleAnular(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := c.BodyParser(&in); err != nil || in.Motivo == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "motivo requerido"})
	}
	out, err := s.svc.AnularDocumento(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.Motivo)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleNotaCredito emite una nota de crédito parcial referenciando una factura:
// acredita solo las cantidades indicadas por línea (devolución/descuento legal).
func (s *Server) handleNotaCredito(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
		Lineas []struct {
			SKU      string  `json:"sku"`
			Cantidad float64 `json:"cantidad"`
		} `json:"lineas"`
	}
	if err := c.BodyParser(&in); err != nil || in.Motivo == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "motivo requerido"})
	}
	lineas := make([]application.LineaEntrada, 0, len(in.Lineas))
	for _, l := range in.Lineas {
		lineas = append(lineas, application.LineaEntrada{SKU: l.SKU, Cantidad: l.Cantidad})
	}
	out, err := s.svc.EmitirNotaCredito(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.Motivo, lineas)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleNotaDebito emite una nota de débito referenciando una factura: carga un
// concepto adicional (con su IVA) que AUMENTA el monto a cobrar del cliente.
func (s *Server) handleNotaDebito(c *fiber.Ctx) error {
	var in struct {
		Concepto string  `json:"concepto"`
		Monto    float64 `json:"monto"`
		Exento   bool    `json:"exento"`
		// Porcentaje sobre el total de la factura, o sobre el renglón `sku`.
		Porcentaje   float64 `json:"porcentaje"`
		SKU          string  `json:"sku"`
		MotivoCodigo string  `json:"motivoCodigo"`
	}
	if err := c.BodyParser(&in); err != nil || in.Concepto == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "concepto requerido"})
	}
	out, err := s.svc.EmitirNotaDebito(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"),
		application.NotaDebitoEntrada{
			Concepto: in.Concepto, Monto: in.Monto, Exento: in.Exento,
			Porcentaje: in.Porcentaje, SKU: in.SKU, MotivoCodigo: in.MotivoCodigo,
		})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleVentasEnEspera lista los carritos apartados de la sede activa.
func (s *Server) handleVentasEnEspera(c *fiber.Ctx) error {
	return c.JSON(s.svc.VentasEnEspera(empresaIDOf(c), sedeIDOf(c)))
}

// handleDejarEnEspera aparta el carrito con una nota para reconocerlo.
func (s *Server) handleDejarEnEspera(c *fiber.Ctx) error {
	var in struct {
		Nota      string `json:"nota"`
		ClienteID string `json:"clienteId"`
		Lineas    []struct {
			SKU            string  `json:"sku"`
			Cantidad       float64 `json:"cantidad"`
			PrecioUnitario float64 `json:"precioUnitario"`
		} `json:"lineas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	lineas := make([]venta.Linea, 0, len(in.Lineas))
	for _, l := range in.Lineas {
		lineas = append(lineas, venta.Linea{SKU: l.SKU, Cantidad: l.Cantidad, PrecioUnitario: l.PrecioUnitario})
	}
	out, err := s.svc.DejarVentaEnEspera(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), in.Nota, in.ClienteID, lineas)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleRetomarVenta devuelve el carrito apartado y lo consume.
func (s *Server) handleRetomarVenta(c *fiber.Ctx) error {
	out, err := s.svc.RetomarVenta(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"))
	if err != nil {
		if errors.Is(err, application.ErrVentaEnEsperaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleDescartarVenta tira un carrito apartado (queda auditado).
func (s *Server) handleDescartarVenta(c *fiber.Ctx) error {
	if err := s.svc.DescartarVenta(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id")); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleClientes(c *fiber.Ctx) error {
	return c.JSON(s.svc.Clientes(empresaIDOf(c)))
}

func (s *Server) handleCrearCliente(c *fiber.Ctx) error {
	var in cliente.Cliente
	if err := c.BodyParser(&in); err != nil || in.Nombre == "" || in.Documento == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nombre y documento requeridos"})
	}
	out, err := s.svc.CrearCliente(empresaIDOf(c), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleActualizarCliente edita un cliente existente (maestro editable, no
// ledger). Parsea el cuerpo sobre cliente.Cliente; los campos de procedencia
// (origen/sistema externo/id externo) los preserva el servicio.
func (s *Server) handleActualizarCliente(c *fiber.Ctx) error {
	var in cliente.Cliente
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarCliente(empresaIDOf(c), principalOf(c).UserID, c.Params("id"), in)
	if err != nil {
		if errors.Is(err, application.ErrClienteNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleCuentasCobro(c *fiber.Ctx) error {
	return c.JSON(s.svc.CuentasCobro(empresaIDOf(c)))
}

func (s *Server) handleCrearCuentaCobro(c *fiber.Ctx) error {
	var in fiscal.CuentaCobro
	if err := c.BodyParser(&in); err != nil || in.Tipo == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tipo requerido"})
	}
	out, err := s.svc.CrearCuentaCobro(empresaIDOf(c), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleMetodosPago(c *fiber.Ctx) error {
	return c.JSON(s.svc.MetodosPago(empresaIDOf(c)))
}

func (s *Server) handleCrearMetodoPago(c *fiber.Ctx) error {
	var in struct {
		Nombre        string `json:"nombre"`
		Tipo          string `json:"tipo"`
		Moneda        string `json:"moneda"`
		CuentaCobroID string `json:"cuentaCobroId"`
		EnCaja        bool   `json:"enCaja"`
		EnVentas      bool   `json:"enVentas"`
		Orden         int    `json:"orden"`
	}
	if err := c.BodyParser(&in); err != nil || in.Nombre == "" || in.Tipo == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nombre y tipo requeridos"})
	}
	out, err := s.svc.CrearMetodoPago(empresaIDOf(c), principalOf(c).UserID, origen(c), fiscal.MetodoPago{
		Nombre: in.Nombre, Tipo: in.Tipo, Moneda: in.Moneda, CuentaCobroID: in.CuentaCobroID,
		EnCaja: in.EnCaja, EnVentas: in.EnVentas, Orden: in.Orden,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarMetodoPago(c *fiber.Ctx) error {
	var in struct {
		Nombre        *string `json:"nombre"`
		Moneda        *string `json:"moneda"`
		CuentaCobroID *string `json:"cuentaCobroId"`
		EnCaja        *bool   `json:"enCaja"`
		EnVentas      *bool   `json:"enVentas"`
		Activo        *bool   `json:"activo"`
		Orden         *int    `json:"orden"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarMetodoPago(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), application.CambiosMetodoPago{
		Nombre: in.Nombre, Moneda: in.Moneda, CuentaCobroID: in.CuentaCobroID,
		EnCaja: in.EnCaja, EnVentas: in.EnVentas, Activo: in.Activo, Orden: in.Orden,
	})
	if err != nil {
		if errors.Is(err, application.ErrMetodoPagoNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleEliminarMetodoPago(c *fiber.Ctx) error {
	if err := s.svc.EliminarMetodoPago(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id")); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleCatalogoDispositivos(c *fiber.Ctx) error {
	return c.JSON(s.svc.CatalogoDispositivos())
}

func (s *Server) handleDispositivos(c *fiber.Ctx) error {
	return c.JSON(s.svc.DispositivosFiscales(empresaIDOf(c)))
}

func (s *Server) handleCrearDispositivo(c *fiber.Ctx) error {
	var in struct {
		Nombre    string `json:"nombre"`
		SedeID    string `json:"sedeId"`
		Tipo      string `json:"tipo"`
		Marca     string `json:"marca"`
		Modelo    string `json:"modelo"`
		Serie     string `json:"serie"`
		Puerto    string `json:"puerto"`
		Protocolo string `json:"protocolo"`
	}
	if err := c.BodyParser(&in); err != nil || in.Nombre == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nombre requerido"})
	}
	out, err := s.svc.CrearDispositivoFiscal(empresaIDOf(c), principalOf(c).UserID, origen(c), fiscal.DispositivoFiscal{
		Nombre: in.Nombre, SedeID: in.SedeID, Tipo: in.Tipo, Marca: in.Marca, Modelo: in.Modelo,
		Serie: in.Serie, Puerto: in.Puerto, Protocolo: in.Protocolo,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarDispositivo(c *fiber.Ctx) error {
	var in struct {
		Nombre    *string `json:"nombre"`
		SedeID    *string `json:"sedeId"`
		Tipo      *string `json:"tipo"`
		Marca     *string `json:"marca"`
		Modelo    *string `json:"modelo"`
		Serie     *string `json:"serie"`
		Puerto    *string `json:"puerto"`
		Protocolo *string `json:"protocolo"`
		Activo    *bool   `json:"activo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarDispositivoFiscal(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), application.CambiosDispositivoFiscal{
		Nombre: in.Nombre, SedeID: in.SedeID, Tipo: in.Tipo, Marca: in.Marca, Modelo: in.Modelo,
		Serie: in.Serie, Puerto: in.Puerto, Protocolo: in.Protocolo, Activo: in.Activo,
	})
	if err != nil {
		if errors.Is(err, application.ErrDispositivoNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleDesactivarDispositivo(c *fiber.Ctx) error {
	if err := s.svc.DesactivarDispositivoFiscal(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id")); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

/* --- Maestro de impuestos -------------------------------------------------- */

// handleConceptosISLR devuelve el maestro de conceptos retenibles de la empresa.
// Se siembra solo la primera vez que se pide (ver application/concepto.go).
func (s *Server) handleConceptosISLR(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"conceptos": s.svc.ConceptosISLR(empresaIDOf(c))})
}

// handleSugerenciaRetencionISLR resuelve «cuánto le retengo a ESTE sujeto por
// ESTE concepto sobre ESTA base». El cálculo vive en el servidor a propósito: si
// lo hiciera la pantalla habría dos implementaciones de la misma fórmula y una
// de las dos se quedaría vieja.
func (s *Server) handleSugerenciaRetencionISLR(c *fiber.Ctx) error {
	out, err := s.svc.SugerirRetencionISLR(empresaIDOf(c),
		c.Query("codigo"), c.Query("sujeto"), c.QueryFloat("base", 0))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleAlicuotas(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"alicuotas": s.svc.AlicuotasVista(empresaIDOf(c))})
}

func (s *Server) handleCrearAlicuota(c *fiber.Ctx) error {
	var in struct {
		Codigo string `json:"codigo"`
		Nombre string `json:"nombre"`
		Tipo   string `json:"tipo"`
		// Porcentaje y Adicional viajan en FRACCIÓN (0.16 = 16 %), igual que se
		// guardan: convertir en la pantalla y no en el transporte evita que el
		// mismo número signifique dos cosas distintas según por dónde entre.
		Porcentaje   float64 `json:"porcentaje"`
		Adicional    float64 `json:"adicional"`
		VigenteDesde string  `json:"vigenteDesde"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearAlicuota(empresaIDOf(c), principalOf(c).UserID, origen(c), fiscal.Alicuota{
		Codigo: in.Codigo, Nombre: in.Nombre, Tipo: in.Tipo,
		Porcentaje: in.Porcentaje, Adicional: in.Adicional, VigenteDesde: in.VigenteDesde,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarAlicuota(c *fiber.Ctx) error {
	// Punteros: lo que no venga no cambia. Y que Porcentaje/Adicional sean
	// punteros es lo que permite distinguir «renombrar» (edita la fila) de
	// «cambiar la tasa» (abre una vigencia nueva) — con valores planos, renombrar
	// mandaría un 0 y partiría el histórico sin que nadie lo pidiera.
	var in struct {
		Nombre       *string  `json:"nombre"`
		Activa       *bool    `json:"activa"`
		Porcentaje   *float64 `json:"porcentaje"`
		Adicional    *float64 `json:"adicional"`
		VigenteDesde string   `json:"vigenteDesde"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarAlicuota(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"),
		application.CambiosAlicuota{
			Nombre: in.Nombre, Activa: in.Activa,
			Porcentaje: in.Porcentaje, Adicional: in.Adicional, VigenteDesde: in.VigenteDesde,
		})
	if err != nil {
		if errors.Is(err, application.ErrAlicuotaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleMotivosNota expone los catálogos de motivos de nota de crédito y débito.
func (s *Server) handleMotivosNota(c *fiber.Ctx) error {
	nc, nd := s.svc.MotivosDeNota()
	return c.JSON(fiber.Map{"credito": nc, "debito": nd})
}

// handleNCDescuento emite una nota de crédito por DESCUENTO: baja el monto sin
// devolver mercancía.
func (s *Server) handleNCDescuento(c *fiber.Ctx) error {
	var in struct {
		Monto  float64 `json:"monto"`
		Exento bool    `json:"exento"`
		Nota   string  `json:"nota"`
		// Porcentaje sobre el total de la factura, o sobre el renglón `sku`.
		Porcentaje float64 `json:"porcentaje"`
		SKU        string  `json:"sku"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.NotaCreditoDescuentoDe(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"),
		application.DescuentoNC{
			Monto: in.Monto, Porcentaje: in.Porcentaje, SKU: in.SKU, Exento: in.Exento, Nota: in.Nota,
		})
	if err != nil {
		return errorDeNota(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleNCAjustePrecio emite una nota de crédito por AJUSTE DE PRECIO: acredita
// la diferencia por producto, sin devolver mercancía.
func (s *Server) handleNCAjustePrecio(c *fiber.Ctx) error {
	var in struct {
		Nota    string `json:"nota"`
		Ajustes []struct {
			SKU            string  `json:"sku"`
			PrecioCorrecto float64 `json:"precioCorrecto"`
		} `json:"ajustes"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	ajustes := make([]application.AjustePrecioLinea, 0, len(in.Ajustes))
	for _, a := range in.Ajustes {
		ajustes = append(ajustes, application.AjustePrecioLinea{SKU: a.SKU, PrecioCorrecto: a.PrecioCorrecto})
	}
	out, err := s.svc.NotaCreditoAjustePrecio(empresaIDOf(c), principalOf(c).UserID, origen(c),
		c.Params("id"), ajustes, in.Nota)
	if err != nil {
		return errorDeNota(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// errorDeNota traduce los fallos de las notas: 404 lo que no existe, 409 lo que
// choca con el estado del documento (ya anulado, tope de crédito alcanzado) y
// 400 lo demás. Distinguirlos permite a la pantalla reaccionar distinto.
func errorDeNota(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, application.ErrDocumentoNoExiste):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, application.ErrYaAnulado), errors.Is(err, application.ErrNCDescuentoExcede):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
}
