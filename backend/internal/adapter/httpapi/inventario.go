package httpapi

import (
	"errors"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"

	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/usuario"
)

func (s *Server) registerInventario(r fiber.Router) {
	g := r.Group("/inventario", s.verInventario)
	g.Get("/productos", s.handleProductos)
	g.Post("/productos", s.escribirInventario, s.handleCrearProducto)
	g.Post("/productos/importar", s.escribirInventario, s.handleImportarProductos)
	g.Patch("/productos/:sku", s.escribirInventario, s.handleActualizarProducto)
	g.Post("/productos/:id/presentaciones", s.escribirInventario, s.handleAgregarPresentacion)
	g.Get("/existencias", s.handleExistencias)
	g.Post("/existencias/:sku/ajustar", s.escribirInventario, s.handleAjustar)

	// TRAZABILIDAD POR LOTE. La existencia por lote es una proyección del mismo
	// ledger; "/por-vencer" alimenta el aviso de caducidad, que es el motivo por el
	// que casi todo el mundo activa los lotes.
	g.Get("/productos/:sku/lotes", s.handleSaldosPorLote)
	g.Get("/lotes/por-vencer", s.handleLotesPorVencer)
	// DÓNDE está un producto dentro de la sede: por almacén y por ubicación. La
	// suma de sus cantidades es la existencia de la sede, siempre.
	g.Get("/productos/:sku/ubicaciones", s.handleExistenciaPorUbicacion)
	g.Post("/productos/:sku/trasladar", s.escribirInventario, s.handleTrasladar)
	g.Get("/pendiente-de-ubicar", s.handlePendienteDeUbicar)

	// VALORACIÓN: dónde está el valor y si coincide con la contabilidad.
	g.Get("/valoracion", s.handleValoracion)

	// DIAGNÓSTICO: qué está torcido y dónde. Es de SOLO LECTURA —no asienta ni
	// ajusta— así que no exige el permiso de escritura: hay que poder mirar antes
	// de decidir si se toca algo, y quien no puede tocar igual necesita saberlo.
	g.Get("/diagnostico", s.handleDiagnosticoInventario)

	// REABASTECIMIENTO: cuándo volver a comprar, y cuánto.
	g.Get("/reabastecimiento/reglas", s.handleReglasReabastecimiento)
	g.Post("/reabastecimiento/reglas", s.escribirInventario, s.handleCrearReglaReabastecimiento)
	g.Patch("/reabastecimiento/reglas/:id", s.escribirInventario, s.handleActualizarReglaReabastecimiento)
	// La revisión NO escribe: es lo que hay que poder mirar antes de generar nada,
	// y por eso no exige el permiso de escritura.
	g.Get("/reabastecimiento/revisar", s.handleRevisarReabastecimiento)
	g.Post("/reabastecimiento/generar", s.escribirInventario, s.handleGenerarReabastecimiento)
	// CORRECCIÓN DE COSTO: revaluar sin mover unidades.
	g.Post("/existencias/:sku/corregir-costo", s.escribirInventario, s.handleCorregirCosto)
	// CONTEO FÍSICO: la hoja entera en una operación. La vista previa NO escribe, y
	// por eso no exige el permiso de escritura: es lo que hay que poder mirar antes
	// de decidir.
	g.Post("/conteo/previsualizar", s.handlePrevisualizarConteo)
	g.Post("/conteo", s.escribirInventario, s.handleAplicarConteo)
	// PLANES DE CONTEO: qué almacén toca, y cada cuánto. Listar y armar la hoja NO
	// escriben: son lo que se consulta antes de salir a contar.
	g.Get("/conteo/planes", s.handlePlanesDeConteo)
	g.Post("/conteo/planes", s.escribirInventario, s.handleCrearPlanDeConteo)
	g.Patch("/conteo/planes/:id", s.escribirInventario, s.handleActualizarPlanDeConteo)
	g.Get("/conteo/planes/:id/hoja", s.handleHojaDeConteo)
	g.Post("/conteo/planes/:id/aplicar", s.escribirInventario, s.handleAplicarConteoDePlan)

	// APARTADOS: mercancía comprometida que todavía no salió. Crear y despachar
	// tocan el inventario; listar, no.
	g.Get("/apartados", s.handleApartados)
	g.Post("/apartados", s.escribirInventario, s.handleCrearApartado)
	g.Post("/apartados/:id/despachar", s.escribirInventario, s.handleDespacharApartado)
	g.Post("/apartados/:id/liberar", s.escribirInventario, s.handleLiberarApartado)
	// EL RASTRO de un lote: «¿a quién le vendí el lote X?». Es la consulta que
	// justifica la trazabilidad; sin ella el dato está guardado pero no sirve.
	g.Get("/productos/:sku/lotes/historico", s.handleLotesHistoricos)
	g.Get("/productos/:sku/lotes/:lote/rastro", s.handleRastroDeLote)
	g.Get("/kardex/:sku", s.handleKardex)
	g.Get("/movimientos", s.handleMovimientos)
	// Imagen del producto: subir (multipart) y quitar. Escribir el catálogo es de
	// Dueña/Desarrollador, igual que el alta de producto.
	g.Post("/productos/:sku/imagen", s.escribirInventario, s.handleSubirImagenProducto)
	g.Delete("/productos/:sku/imagen", s.escribirInventario, s.handleQuitarImagenProducto)
	g.Get("/transferencias", s.handleTransferencias)
	g.Post("/transferencias", s.escribirInventario, s.handleCrearTransferencia)
	g.Patch("/transferencias/:id/estado", s.escribirInventario, s.handleEstadoTransferencia)
	g.Post("/transferencias/:id/cancelar", s.escribirInventario, s.handleCancelarTransferencia)

	// Disponibilidad por ubicación (03 §4.7) — FUERA del guard de inventario a
	// propósito: el Cajero no entra al módulo, pero sí necesita responder «¿en
	// qué otra tienda hay?» desde el botón de información del punto de venta.
	dispo := r.Group("/inventario", s.requireRoles(
		usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor,
		usuario.RolCajero, usuario.RolContadora))
	dispo.Get("/productos/:sku/disponibilidad", s.handleDisponibilidad)
	// Escaneo de código de barras: el Cajero no entra al módulo de inventario,
	// pero su pistola lectora tiene que poder resolver un código.
	dispo.Get("/buscar", s.handleBuscarPorCodigo)
}

// verInventario permite ver el módulo a los roles con acceso (todos menos Cajero).
func (s *Server) verInventario(c *fiber.Ctx) error {
	switch rolOf(c) {
	case usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolContadora, usuario.RolMesonero:
		// El mesonero no entra al MÓDULO de inventario (la UI no se lo muestra), pero
		// necesita LEER el catálogo para el menú de la comandera.
		return c.Next()
	}
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin acceso a inventario"})
}

// escribirInventario restringe las mutaciones a Dueña/Desarrollador (Vendedor y
// Contadora son de solo lectura en este módulo).
func (s *Server) escribirInventario(c *fiber.Ctx) error {
	if rol := rolOf(c); rol == usuario.RolDueno || rol == usuario.RolDesarrollador {
		return c.Next()
	}
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no tiene permiso para modificar inventario"})
}

func (s *Server) sedeParam(c *fiber.Ctx) string {
	if q := c.Query("sede"); q != "" {
		return q
	}
	return sedeIDOf(c)
}

// almacenParam resuelve el almacén de la petición: query `?almacen=` o header
// `X-Almacen-ID`. Vacío ⇒ la consulta es a nivel de sede (suma de sus almacenes) o,
// en escritura, el almacén principal de la sede.
func (s *Server) almacenParam(c *fiber.Ctx) string {
	if q := c.Query("almacen"); q != "" {
		return q
	}
	return c.Get("X-Almacen-ID")
}

func (s *Server) handleProductos(c *fiber.Ctx) error {
	return c.JSON(s.svc.Productos(empresaIDOf(c)))
}

func (s *Server) handleCrearProducto(c *fiber.Ctx) error {
	var in struct {
		SKU        string `json:"sku"`
		Nombre     string `json:"nombre"`
		Rubro      string `json:"rubro"`
		UnidadBase string `json:"unidadBase"`
		// TipoVenta: "unidad" (default) o "peso" (se cobra por kg). Con "peso" el
		// servicio fija UnidadBase="kg".
		TipoVenta string  `json:"tipoVenta"`
		Precio    float64 `json:"precio"`
		// Moneda en la que está expresado el precio (R10). Vacío = la principal
		// de la empresa.
		Moneda string `json:"moneda"`
		// Código de barras propio del producto (R11) y condición de IVA.
		CodigoBarras string `json:"codigoBarras"`
		ExentoIVA    bool   `json:"exentoIva"`
		// AlicuotaCodigo apunta al maestro de impuestos ("general", "reducida",
		// "suntuario", "exento"). Vacío = como siempre: exento si ExentoIVA,
		// general si no. Los catálogos ya cargados no se migran.
		AlicuotaCodigo string `json:"alicuotaCodigo"`
		// ConceptoISLR apunta al maestro de conceptos ("honorarios", "fletes"…) cuando
		// el producto es un SERVICIO sujeto a retención. Vacío = no sujeto.
		ConceptoISLR string `json:"conceptoIslr"`
		// Trazabilidad: lote obligatorio al recibir y, opcionalmente, vencimiento.
		RequiereLote        bool `json:"requiereLote"`
		ControlaVencimiento bool `json:"controlaVencimiento"`
		// Combo (paquete de otros productos): esCombo marca el paquete y componentes
		// lleva su receta (SKU + cantidad). El servicio valida y, si es combo, fuerza
		// unidad/no-stock y sugiere el precio por defecto (suma de componentes).
		EsCombo     bool                         `json:"esCombo"`
		Componentes []inventario.ComboComponente `json:"componentes"`
		// Plato con receta (escandallo, módulo Restaurante): se vende como una línea y
		// descuenta sus insumos del inventario al facturar.
		EsPlato  bool                         `json:"esPlato"`
		EsInsumo bool                         `json:"esInsumo"`
		Receta   []inventario.ComboComponente `json:"receta"`
		// ModoFabricacion decide CUÁNDO se convierten los insumos: al venderlo
		// (bajo pedido) o antes, con una orden (para stock).
		ModoFabricacion string `json:"modoFabricacion"`
		// La fórmula: para qué tanda está escrita, cuánto rinde y cuánta
		// desviación es aceptable.
		LoteBase       float64 `json:"loteBase"`
		RendimientoPct float64 `json:"rendimientoPct"`
		ToleranciaPct  float64 `json:"toleranciaPct"`
		// Comandera por la que sale este producto (módulo Restaurante). Vacío =
		// se rutea por su rubro.
		ComanderaID string `json:"comanderaId"`
	}
	if err := c.BodyParser(&in); err != nil || in.SKU == "" || in.Nombre == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "SKU y nombre requeridos"})
	}
	p := inventario.Producto{
		SKU: in.SKU, Nombre: in.Nombre, Rubro: in.Rubro, UnidadBase: in.UnidadBase,
		TipoVenta: in.TipoVenta, Precio: in.Precio, Moneda: in.Moneda,
		CodigoBarras: in.CodigoBarras, ExentoIVA: in.ExentoIVA, AlicuotaCodigo: in.AlicuotaCodigo,
		ConceptoISLR: in.ConceptoISLR,
		RequiereLote: in.RequiereLote, ControlaVencimiento: in.ControlaVencimiento,
		EsCombo: in.EsCombo, Componentes: in.Componentes,
		EsPlato: in.EsPlato, Receta: in.Receta, EsInsumo: in.EsInsumo,
		ModoFabricacion: in.ModoFabricacion,
		LoteBase:        in.LoteBase, RendimientoPct: in.RendimientoPct, ToleranciaPct: in.ToleranciaPct,
		ComanderaID: in.ComanderaID,
	}
	out, err := s.svc.CrearProducto(empresaIDOf(c), principalOf(c).UserID, origen(c), p)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleImportarProductos ejecuta la carga masiva de productos (CSV parseado por
// el front). confirmar=false ⇒ vista previa (valida y clasifica sin escribir);
// confirmar=true ⇒ aplica (todo-o-nada si hay errores). Ver ImportarProductos.
func (s *Server) handleImportarProductos(c *fiber.Ctx) error {
	var in struct {
		Confirmar bool `json:"confirmar"`
		Filas     []struct {
			SKU               string  `json:"sku"`
			Nombre            string  `json:"nombre"`
			Rubro             string  `json:"rubro"`
			Unidad            string  `json:"unidad"`
			TipoVenta         string  `json:"tipoVenta"`
			Precio            float64 `json:"precio"`
			Moneda            string  `json:"moneda"`
			ExentoIVA         bool    `json:"exentoIva"`
			AlicuotaCodigo    string  `json:"alicuotaCodigo"`
			CodigoBarras      string  `json:"codigoBarras"`
			ExistenciaInicial float64 `json:"existenciaInicial"`
		} `json:"filas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	filas := make([]application.FilaImportacionProducto, 0, len(in.Filas))
	for _, f := range in.Filas {
		filas = append(filas, application.FilaImportacionProducto{
			SKU: f.SKU, Nombre: f.Nombre, Rubro: f.Rubro, Unidad: f.Unidad,
			TipoVenta: f.TipoVenta, Precio: f.Precio, Moneda: f.Moneda,
			ExentoIVA: f.ExentoIVA, AlicuotaCodigo: f.AlicuotaCodigo,
			CodigoBarras: f.CodigoBarras, ExistenciaInicial: f.ExistenciaInicial,
		})
	}
	res, err := s.svc.ImportarProductos(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), filas, in.Confirmar)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

// handleActualizarProducto edita el catálogo (incluido el código de barras).
func (s *Server) handleActualizarProducto(c *fiber.Ctx) error {
	// PATCH parcial: los booleanos y el código de barras se bindean como PUNTEROS
	// para distinguir "no enviado" (nil, no cambiar) de un false explícito. Así un
	// PATCH que solo trae {precio} conserva activo/exentoIva/esCombo en vez de
	// pisarlos a false. La UI puede seguir mandando el objeto completo sin problema.
	var in struct {
		Nombre       string  `json:"nombre"`
		Rubro        string  `json:"rubro"`
		TipoVenta    string  `json:"tipoVenta"`
		UnidadBase   string  `json:"unidadBase"`
		Precio       float64 `json:"precio"`
		Moneda       string  `json:"moneda"`
		CodigoBarras *string `json:"codigoBarras"`
		ExentoIVA    *bool   `json:"exentoIva"`
		// AlicuotaCodigo es *string: nil = no enviado = no se toca; "" devuelve el
		// producto al comportamiento heredado (manda ExentoIVA).
		AlicuotaCodigo *string `json:"alicuotaCodigo"`
		// ConceptoISLR es *string: nil = no enviado = no se toca; "" deja de tratar el
		// producto como servicio sujeto a retención.
		ConceptoISLR *string `json:"conceptoIslr"`
		// *bool: nil = no se toca. Activar la trazabilidad no pierde la existencia
		// anterior; queda visible como «sin lote».
		RequiereLote        *bool `json:"requiereLote"`
		ControlaVencimiento *bool `json:"controlaVencimiento"`
		Activo              *bool `json:"activo"`
		// Combo: esCombo (nil = no cambiar) convierte/mantiene el paquete; componentes
		// (nil = no se toca la receta) lleva la receta cuando se edita.
		EsCombo         *bool                        `json:"esCombo"`
		Componentes     []inventario.ComboComponente `json:"componentes"`
		EsPlato         *bool                        `json:"esPlato"`
		EsInsumo        *bool                        `json:"esInsumo"`
		Receta          []inventario.ComboComponente `json:"receta"`
		ModoFabricacion *string                      `json:"modoFabricacion"`
		LoteBase        *float64                     `json:"loteBase"`
		RendimientoPct  *float64                     `json:"rendimientoPct"`
		ToleranciaPct   *float64                     `json:"toleranciaPct"`
		ComanderaID     *string                      `json:"comanderaId"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarProducto(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("sku"), application.CambiosProducto{
		Nombre: in.Nombre, Rubro: in.Rubro, TipoVenta: in.TipoVenta, UnidadBase: in.UnidadBase,
		ModoFabricacion: in.ModoFabricacion,
		LoteBase:        in.LoteBase, RendimientoPct: in.RendimientoPct, ToleranciaPct: in.ToleranciaPct,
		Precio: in.Precio, Moneda: in.Moneda,
		CodigoBarras: in.CodigoBarras, ExentoIVA: in.ExentoIVA, AlicuotaCodigo: in.AlicuotaCodigo,
		ConceptoISLR: in.ConceptoISLR,
		RequiereLote: in.RequiereLote, ControlaVencimiento: in.ControlaVencimiento,
		Activo:  in.Activo,
		EsCombo: in.EsCombo, Componentes: in.Componentes,
		EsPlato: in.EsPlato, Receta: in.Receta, EsInsumo: in.EsInsumo,
		ComanderaID: in.ComanderaID,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleBuscarPorCodigo resuelve un escaneo de la pistola lectora: devuelve el
// producto cuyo código de barras (propio o de una presentación) coincide.
func (s *Server) handleBuscarPorCodigo(c *fiber.Ctx) error {
	p, ok := s.svc.PorCodigoBarras(empresaIDOf(c), c.Query("codigo"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "ningún producto tiene ese código"})
	}
	return c.JSON(p)
}

func (s *Server) handleAgregarPresentacion(c *fiber.Ctx) error {
	var in inventario.Presentacion
	if err := c.BodyParser(&in); err != nil || in.Nombre == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos de presentación inválidos"})
	}
	out, err := s.svc.AgregarPresentacion(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleExistencias(c *fiber.Ctx) error {
	// Si viene un almacén, la existencia es de ese almacén; si no, de la sede (suma
	// de todos sus almacenes) como siempre.
	if alm := s.almacenParam(c); alm != "" {
		out, err := s.svc.ExistenciasDeAlmacen(empresaIDOf(c), alm)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(out)
	}
	sede := s.sedeParam(c)
	if sede == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "seleccione una sede"})
	}
	return c.JSON(s.svc.Existencias(empresaIDOf(c), sede))
}

// handleSaldosPorLote proyecta la existencia por lote de un producto en la sede.
func (s *Server) handleSaldosPorLote(c *fiber.Ctx) error {
	emp := empresaIDOf(c)
	p, ok := s.svc.ProductoPorSKU(emp, c.Params("sku"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "producto no existe"})
	}
	sede := c.Query("sedeId")
	if sede == "" {
		sede = sedeIDOf(c)
	}
	return c.JSON(fiber.Map{"lotes": s.svc.SaldosPorLote(emp, sede, p.ID)})
}

// handleLotesPorVencer lista lo que caduca pronto (o ya caducó). "dias" acota la
// ventana; sin él, 30.
func (s *Server) handleLotesPorVencer(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"lotes": s.svc.LotesPorVencer(empresaIDOf(c), c.Query("sedeId"), c.QueryInt("dias", 30)),
	})
}

// handleLotesHistoricos lista los lotes que alguna vez existieron del producto,
// con saldo o sin él: tras una alerta, el agotado es justo el que hay que mirar.
func (s *Server) handleLotesHistoricos(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"lotes": s.svc.LotesDeProducto(empresaIDOf(c), c.Params("sku"), c.Query("sedeId")),
	})
}

// handleRastroDeLote devuelve la historia del lote. SIN sede por defecto: en una
// alerta sanitaria el lote no respeta los límites de una sucursal.
func (s *Server) handleRastroDeLote(c *fiber.Ctx) error {
	lote, err := url.PathUnescape(c.Params("lote"))
	if err != nil {
		lote = c.Params("lote")
	}
	return c.JSON(s.svc.RastroDeLote(empresaIDOf(c), c.Params("sku"), lote, c.Query("sedeId")))
}

func (s *Server) handleExistenciaPorUbicacion(c *fiber.Ctx) error {
	sede := c.Query("sedeId")
	if sede == "" {
		sede = sedeIDOf(c)
	}
	return c.JSON(fiber.Map{"ubicaciones": s.svc.ExistenciaPorUbicacion(empresaIDOf(c), sede, c.Params("sku"))})
}

// handleTrasladar mueve mercancía entre ubicaciones del mismo almacén. Es una
// operación NEUTRA —ni cambia la existencia ni asienta—, así que devuelve el mapa
// de ubicaciones para que la pantalla muestre el resultado sin volver a pedirlo.
// handlePendienteDeUbicar lista lo que espera en el muelle el segundo paso de una
// recepción. Vacío cuando no hay recepción en dos pasos configurada, que es lo
// normal y no es un error.
func (s *Server) handlePendienteDeUbicar(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"pendientes": s.svc.PendienteDeUbicar(empresaIDOf(c), s.sedeParam(c))})
}

func (s *Server) handleApartados(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"apartados": s.svc.Apartados(empresaIDOf(c), s.sedeParam(c))})
}

func (s *Server) handleCrearApartado(c *fiber.Ctx) error {
	var in struct {
		Motivo    string `json:"motivo"`
		AlmacenID string `json:"almacenId"`
		RefTipo   string `json:"refTipo"`
		RefID     string `json:"refId"`
		Lineas    []struct {
			SKU         string  `json:"sku"`
			Cantidad    float64 `json:"cantidad"`
			UbicacionID string  `json:"ubicacionId"`
			Lote        string  `json:"lote"`
		} `json:"lineas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	lineas := make([]inventario.LineaApartado, 0, len(in.Lineas))
	for _, l := range in.Lineas {
		lineas = append(lineas, inventario.LineaApartado{
			SKU: l.SKU, Cantidad: l.Cantidad, UbicacionID: l.UbicacionID, Lote: l.Lote,
		})
	}
	out, err := s.svc.CrearApartado(empresaIDOf(c), principalOf(c).UserID, origen(c), inventario.Apartado{
		SedeID: s.sedeParam(c), AlmacenID: in.AlmacenID, Motivo: in.Motivo,
		RefTipo: in.RefTipo, RefID: in.RefID, Lineas: lineas,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleDespacharApartado(c *fiber.Ctx) error {
	out, err := s.svc.DespacharApartado(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	return respuestaApartado(c, out, err)
}

func (s *Server) handleLiberarApartado(c *fiber.Ctx) error {
	out, err := s.svc.LiberarApartado(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	return respuestaApartado(c, out, err)
}

// respuestaApartado distingue «no existe» de «no se puede»: un 404 manda a buscar
// el apartado y un 409 dice que el apartado está, pero ya cerrado.
func respuestaApartado(c *fiber.Ctx, out inventario.Apartado, err error) error {
	if err == nil {
		return c.JSON(out)
	}
	estado := fiber.StatusBadRequest
	switch {
	case errors.Is(err, application.ErrApartadoNoExiste):
		estado = fiber.StatusNotFound
	case errors.Is(err, application.ErrApartadoCerrado):
		estado = fiber.StatusConflict
	}
	return c.Status(estado).JSON(fiber.Map{"error": err.Error()})
}

func (s *Server) handleReglasReabastecimiento(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"reglas": s.svc.ReglasReabastecimiento(empresaIDOf(c), c.Query("sede"))})
}

// reglaReabastecimientoBody es el cuerpo de alta y edición.
type reglaReabastecimientoBody struct {
	SKU         string  `json:"sku"`
	SedeID      string  `json:"sedeId"`
	AlmacenID   string  `json:"almacenId"`
	Minimo      float64 `json:"minimo"`
	Maximo      float64 `json:"maximo"`
	Multiplo    float64 `json:"multiplo"`
	ProveedorID string  `json:"proveedorId"`
	Activa      bool    `json:"activa"`
}

func (in reglaReabastecimientoBody) aDominio(sedePorDefecto string) inventario.ReglaReabastecimiento {
	sede := in.SedeID
	if sede == "" {
		sede = sedePorDefecto
	}
	return inventario.ReglaReabastecimiento{
		SKU: in.SKU, SedeID: sede, AlmacenID: in.AlmacenID,
		Minimo: in.Minimo, Maximo: in.Maximo, Multiplo: in.Multiplo,
		ProveedorID: in.ProveedorID, Activa: in.Activa,
	}
}

func (s *Server) handleCrearReglaReabastecimiento(c *fiber.Ctx) error {
	var in reglaReabastecimientoBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearReglaReabastecimiento(empresaIDOf(c), principalOf(c).UserID, origen(c),
		in.aDominio(s.sedeParam(c)))
	if err != nil {
		estado := fiber.StatusBadRequest
		// Una regla repetida es un conflicto de estado, no un dato mal escrito: la
		// petición era correcta y ya existe algo que la cubre.
		if errors.Is(err, application.ErrReglaDuplicada) {
			estado = fiber.StatusConflict
		}
		return c.Status(estado).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarReglaReabastecimiento(c *fiber.Ctx) error {
	var in reglaReabastecimientoBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarReglaReabastecimiento(empresaIDOf(c), c.Params("id"),
		principalOf(c).UserID, origen(c), in.aDominio(s.sedeParam(c)))
	if err != nil {
		estado := fiber.StatusBadRequest
		if errors.Is(err, application.ErrReglaNoExiste) {
			estado = fiber.StatusNotFound
		}
		return c.Status(estado).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleRevisarReabastecimiento(c *fiber.Ctx) error {
	return c.JSON(s.svc.RevisarReabastecimiento(empresaIDOf(c), s.sedeParam(c)))
}

func (s *Server) handleGenerarReabastecimiento(c *fiber.Ctx) error {
	out, err := s.svc.GenerarSolicitudesDeReabastecimiento(empresaIDOf(c), s.sedeParam(c),
		principalOf(c).UserID, origen(c))
	if err != nil {
		estado := fiber.StatusBadRequest
		// «Nada bajo mínimos» no es un error de la petición: es que no hace falta.
		if errors.Is(err, application.ErrSinNadaQuePedir) {
			estado = fiber.StatusConflict
		}
		return c.Status(estado).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"solicitudes": out})
}

func (s *Server) handlePlanesDeConteo(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"planes": s.svc.PlanesDeConteo(empresaIDOf(c), s.sedeParam(c))})
}

// planConteoBody es el cuerpo de alta y edición. UltimoConteo NO viaja: es un
// hecho, no una preferencia — poder retocarlo permitiría aplazar un conteo vencido
// cambiando una fecha, que es justo lo que el plan existe para evitar.
type planConteoBody struct {
	Nombre      string `json:"nombre"`
	SedeID      string `json:"sedeId"`
	AlmacenID   string `json:"almacenId"`
	UbicacionID string `json:"ubicacionId"`
	Rubro       string `json:"rubro"`
	CadaDias    int    `json:"cadaDias"`
	Activo      bool   `json:"activo"`
}

func (in planConteoBody) aDominio(sedePorDefecto string) inventario.PlanConteo {
	sede := in.SedeID
	if sede == "" {
		sede = sedePorDefecto
	}
	return inventario.PlanConteo{
		Nombre: in.Nombre, SedeID: sede, AlmacenID: in.AlmacenID,
		UbicacionID: in.UbicacionID, Rubro: in.Rubro,
		CadaDias: in.CadaDias, Activo: in.Activo,
	}
}

func (s *Server) handleCrearPlanDeConteo(c *fiber.Ctx) error {
	var in planConteoBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearPlanDeConteo(empresaIDOf(c), principalOf(c).UserID, origen(c), in.aDominio(s.sedeParam(c)))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarPlanDeConteo(c *fiber.Ctx) error {
	var in planConteoBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarPlanDeConteo(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c),
		in.aDominio(s.sedeParam(c)))
	if err != nil {
		estado := fiber.StatusBadRequest
		if errors.Is(err, application.ErrPlanNoExiste) {
			estado = fiber.StatusNotFound
		}
		return c.Status(estado).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleHojaDeConteo(c *fiber.Ctx) error {
	out, err := s.svc.HojaDeConteoDe(empresaIDOf(c), c.Params("id"))
	if err != nil {
		estado := fiber.StatusBadRequest
		if errors.Is(err, application.ErrPlanNoExiste) {
			estado = fiber.StatusNotFound
		}
		return c.Status(estado).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleAplicarConteoDePlan(c *fiber.Ctx) error {
	var in conteoBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.AplicarConteoDePlan(empresaIDOf(c), c.Params("id"), in.Motivo, in.aDominio(),
		principalOf(c).UserID, origen(c))
	if err != nil {
		estado := fiber.StatusBadRequest
		if errors.Is(err, application.ErrPlanNoExiste) {
			estado = fiber.StatusNotFound
		}
		return c.Status(estado).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleValoracion(c *fiber.Ctx) error {
	// Sin ?sede= mira la empresa entera, que es lo único que se puede comparar con
	// el diario: el saldo de una cuenta contable no es de una sede.
	return c.JSON(s.svc.Valoracion(empresaIDOf(c), c.Query("sede")))
}

func (s *Server) handleDiagnosticoInventario(c *fiber.Ctx) error {
	return c.JSON(s.svc.DiagnosticarInventario(empresaIDOf(c)))
}

func (s *Server) handleCorregirCosto(c *fiber.Ctx) error {
	var in struct {
		Costo  float64 `json:"costo"`
		Motivo string  `json:"motivo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CorregirCosto(empresaIDOf(c), s.sedeParam(c), c.Params("sku"), in.Costo,
		in.Motivo, principalOf(c).UserID, origen(c))
	if err != nil {
		estado := fiber.StatusBadRequest
		// «No hay existencia que revaluar» es un conflicto de estado, no un dato mal
		// escrito: la petición era correcta, el almacén es el que está vacío.
		if errors.Is(err, application.ErrCorreccionSinExistencia) || errors.Is(err, application.ErrCorreccionSinCambio) {
			estado = fiber.StatusConflict
		}
		return c.Status(estado).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// conteoBody es la hoja de conteo.
type conteoBody struct {
	AlmacenID string `json:"almacenId"`
	Motivo    string `json:"motivo"`
	Lineas    []struct {
		SKU         string  `json:"sku"`
		UbicacionID string  `json:"ubicacionId"`
		Contado     float64 `json:"contado"`
		Lote        string  `json:"lote"`
	} `json:"lineas"`
}

func (in conteoBody) aDominio() []application.LineaConteo {
	out := make([]application.LineaConteo, 0, len(in.Lineas))
	for _, l := range in.Lineas {
		out = append(out, application.LineaConteo{
			SKU: l.SKU, UbicacionID: l.UbicacionID, Contado: l.Contado, Lote: l.Lote,
		})
	}
	return out
}

func (s *Server) handlePrevisualizarConteo(c *fiber.Ctx) error {
	var in conteoBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.PrevisualizarConteo(empresaIDOf(c), s.sedeParam(c), in.AlmacenID, in.aDominio())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleAplicarConteo(c *fiber.Ctx) error {
	var in conteoBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.AplicarConteo(empresaIDOf(c), s.sedeParam(c), in.AlmacenID, in.Motivo,
		in.aDominio(), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleTrasladar(c *fiber.Ctx) error {
	var in struct {
		Origen    string  `json:"origen"`
		Destino   string  `json:"destino"`
		Lote      string  `json:"lote"`
		Cantidad  float64 `json:"cantidad"`
		Motivo    string  `json:"motivo"`
		AlmacenID string  `json:"almacenId"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	alm := in.AlmacenID
	if alm == "" {
		alm = s.almacenParam(c)
	}
	out, err := s.svc.TrasladarEntreUbicaciones(empresaIDOf(c), principalOf(c).UserID, origen(c), application.TrasladoPeticion{
		SedeID: s.sedeParam(c), AlmacenID: alm,
		Origen: in.Origen, Destino: in.Destino, SKU: c.Params("sku"),
		Lote: in.Lote, Cantidad: in.Cantidad, Motivo: in.Motivo,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ubicaciones": out})
}

func (s *Server) handleAjustar(c *fiber.Ctx) error {
	var in struct {
		Motivo   string  `json:"motivo"`
		Cantidad float64 `json:"cantidad"`
		// DÓNDE, y de qué lote. El ajuste ya sabía ponerlos —AjustarEnUbicacion
		// existe desde la tanda de ubicaciones— pero la API no los pedía, así que
		// todo ajuste caía en «sin ubicar» y sin lote por mucho que el almacén
		// tuviera estantes. Un sobrante encontrado en el pasillo B entraba al
		// sistema sin decir que estaba en el pasillo B.
		UbicacionID string `json:"ubicacionId"`
		Lote        string `json:"lote"`
		Vencimiento string `json:"vencimiento"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	sede := s.sedeParam(c)
	// El ajuste va al almacén indicado (X-Almacen-ID/?almacen=) o, si no viene, al
	// principal de la sede. La ubicación y el lote son opcionales: sin ellos el
	// comportamiento es exactamente el de antes.
	out, err := s.svc.AjustarEnUbicacion(empresaIDOf(c), sede, s.almacenParam(c), in.UbicacionID,
		c.Params("sku"), in.Motivo, in.Cantidad, in.Lote, in.Vencimiento,
		principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleKardex(c *fiber.Ctx) error {
	// Kardex por almacén si viene el almacén; si no, por sede (comportamiento actual).
	if alm := s.almacenParam(c); alm != "" {
		out, err := s.svc.KardexDeAlmacen(empresaIDOf(c), alm, c.Params("sku"))
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(out)
	}
	out, err := s.svc.Kardex(empresaIDOf(c), s.sedeParam(c), c.Params("sku"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleMovimientos es el registro de entradas y salidas: lista los movimientos del
// ledger filtrables por sede, almacén, sku, tipo y rango de fechas (query params).
func (s *Server) handleMovimientos(c *fiber.Ctx) error {
	f := inventario.FiltroMovimiento{
		SedeID:    c.Query("sede"),
		AlmacenID: c.Query("almacen"),
		SKU:       c.Query("sku"),
	}
	return c.JSON(s.svc.Movimientos(empresaIDOf(c), f, c.Query("desde"), c.Query("hasta"), c.Query("tipo")))
}

func (s *Server) handleTransferencias(c *fiber.Ctx) error {
	return c.JSON(s.svc.Transferencias(empresaIDOf(c)))
}

func (s *Server) handleCrearTransferencia(c *fiber.Ctx) error {
	var in struct {
		OrigenSedeID     string `json:"origenSedeId"`
		DestinoSedeID    string `json:"destinoSedeId"`
		OrigenAlmacenID  string `json:"origenAlmacenId"`
		DestinoAlmacenID string `json:"destinoAlmacenId"`
		Lineas           []struct {
			SKU      string  `json:"sku"`
			Cantidad float64 `json:"cantidad"`
		} `json:"lineas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	t := inventario.Transferencia{
		OrigenSedeID: in.OrigenSedeID, DestinoSedeID: in.DestinoSedeID,
		OrigenAlmacenID: in.OrigenAlmacenID, DestinoAlmacenID: in.DestinoAlmacenID,
	}
	for _, l := range in.Lineas {
		t.Lineas = append(t.Lineas, inventario.LineaTransferencia{SKU: l.SKU, Cantidad: l.Cantidad})
	}
	out, err := s.svc.CrearTransferencia(empresaIDOf(c), principalOf(c).UserID, origen(c), t)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleEstadoTransferencia(c *fiber.Ctx) error {
	var in struct {
		Estado string `json:"estado"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CambiarEstadoTransferencia(empresaIDOf(c), c.Params("id"), in.Estado, principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleCancelarTransferencia(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if in.Motivo == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "el motivo es requerido"})
	}
	out, err := s.svc.CancelarTransferencia(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.Motivo)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleDisponibilidad responde «¿cuántas unidades hay y en qué tienda?» para el
// botón de información de cada tarjeta de producto en el punto de venta.
func (s *Server) handleDisponibilidad(c *fiber.Ctx) error {
	empID := empresaIDOf(c)
	// Las sedes salen de TenancyService, ya acotadas al tenant.
	refs := []application.SedeRef{}
	for _, sd := range s.tenancy.SedesDeEmpresa(empID) {
		if !sd.Activa {
			continue
		}
		refs = append(refs, application.SedeRef{ID: sd.ID, Nombre: sd.Nombre})
	}
	out, err := s.svc.Disponibilidad(empID, sedeIDOf(c), c.Params("sku"), refs)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleSubirImagenProducto recibe un multipart con el campo "archivo", lo guarda
// en el bucket de la empresa y asocia la URL al producto. Si el producto ya tenía
// imagen, la anterior se borra para no dejar archivos huérfanos ocupando disco.
func (s *Server) handleSubirImagenProducto(c *fiber.Ctx) error {
	if s.archivos == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "el almacenamiento de archivos no está configurado"})
	}
	fh, err := c.FormFile("archivo")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "falta el archivo"})
	}
	f, err := fh.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no se pudo leer el archivo"})
	}
	defer f.Close()

	empID := empresaIDOf(c)
	url, err := s.archivos.Guardar(empID, "productos", f)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	out, anterior, err := s.svc.AsignarImagenProducto(empID, principalOf(c).UserID, origen(c), c.Params("sku"), url)
	if err != nil {
		// El producto no existe: no se deja el archivo suelto en el bucket.
		_ = s.archivos.Borrar(empID, url)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	if anterior != "" && anterior != url {
		_ = s.archivos.Borrar(empID, anterior)
	}
	return c.JSON(out)
}

func (s *Server) handleQuitarImagenProducto(c *fiber.Ctx) error {
	empID := empresaIDOf(c)
	out, anterior, err := s.svc.AsignarImagenProducto(empID, principalOf(c).UserID, origen(c), c.Params("sku"), "")
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	if anterior != "" && s.archivos != nil {
		_ = s.archivos.Borrar(empID, anterior)
	}
	return c.JSON(out)
}
