package httpapi

import (
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
		// Combo (paquete de otros productos): esCombo marca el paquete y componentes
		// lleva su receta (SKU + cantidad). El servicio valida y, si es combo, fuerza
		// unidad/no-stock y sugiere el precio por defecto (suma de componentes).
		EsCombo     bool                         `json:"esCombo"`
		Componentes []inventario.ComboComponente `json:"componentes"`
		// Plato con receta (escandallo, módulo Restaurante): se vende como una línea y
		// descuenta sus insumos del inventario al facturar.
		EsPlato bool                         `json:"esPlato"`
		Receta  []inventario.ComboComponente `json:"receta"`
	}
	if err := c.BodyParser(&in); err != nil || in.SKU == "" || in.Nombre == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "SKU y nombre requeridos"})
	}
	p := inventario.Producto{
		SKU: in.SKU, Nombre: in.Nombre, Rubro: in.Rubro, UnidadBase: in.UnidadBase,
		TipoVenta: in.TipoVenta, Precio: in.Precio, Moneda: in.Moneda,
		CodigoBarras: in.CodigoBarras, ExentoIVA: in.ExentoIVA,
		EsCombo: in.EsCombo, Componentes: in.Componentes,
		EsPlato: in.EsPlato, Receta: in.Receta,
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
			ExentoIVA: f.ExentoIVA, CodigoBarras: f.CodigoBarras, ExistenciaInicial: f.ExistenciaInicial,
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
		Activo       *bool   `json:"activo"`
		// Combo: esCombo (nil = no cambiar) convierte/mantiene el paquete; componentes
		// (nil = no se toca la receta) lleva la receta cuando se edita.
		EsCombo     *bool                        `json:"esCombo"`
		Componentes []inventario.ComboComponente `json:"componentes"`
		EsPlato     *bool                        `json:"esPlato"`
		Receta      []inventario.ComboComponente `json:"receta"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarProducto(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("sku"), application.CambiosProducto{
		Nombre: in.Nombre, Rubro: in.Rubro, TipoVenta: in.TipoVenta, UnidadBase: in.UnidadBase,
		Precio: in.Precio, Moneda: in.Moneda,
		CodigoBarras: in.CodigoBarras, ExentoIVA: in.ExentoIVA, Activo: in.Activo,
		EsCombo: in.EsCombo, Componentes: in.Componentes,
		EsPlato: in.EsPlato, Receta: in.Receta,
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

func (s *Server) handleAjustar(c *fiber.Ctx) error {
	var in struct {
		Motivo   string  `json:"motivo"`
		Cantidad float64 `json:"cantidad"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	sede := s.sedeParam(c)
	// El ajuste va al almacén indicado (X-Almacen-ID/?almacen=) o, si no viene, al
	// principal de la sede.
	out, err := s.svc.Ajustar(empresaIDOf(c), sede, s.almacenParam(c), c.Params("sku"), in.Motivo, in.Cantidad, principalOf(c).UserID, origen(c))
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
