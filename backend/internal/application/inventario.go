package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/almacen"
	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// Errores de negocio del módulo de inventario.
var (
	ErrProductoNoExiste        = errors.New("producto no existe")
	ErrSKUDuplicado            = errors.New("ya existe un producto con ese SKU")
	ErrMotivoRequerido         = errors.New("el ajuste requiere un motivo")
	ErrTransicionInvalida      = errors.New("transición de estado no permitida")
	ErrSedesIguales            = errors.New("origen y destino deben ser sedes distintas")
	ErrTransferVacia           = errors.New("la transferencia no tiene líneas")
	ErrStockInsuficiente       = errors.New("no hay stock suficiente en el almacén origen para despachar")
	ErrTransferNoCancelable    = errors.New("la transferencia no se puede cancelar en su estado actual: revierta con una transferencia de vuelta destino→origen")
	ErrPreciosUSDNoHabilitados = errors.New("la empresa no maneja precios en dólares: actívalo en la configuración de moneda")
	ErrCodigoBarrasEnUso       = errors.New("ya hay otro producto con ese código de barras")
	ErrTipoVentaInvalido       = errors.New("tipo de venta inválido (unidad o peso)")
	// Errores de producto COMBO (paquete de otros productos).
	ErrComboSinComponentes     = errors.New("un combo necesita al menos un componente")
	ErrComboComponenteInvalido = errors.New("componente de combo inválido: el SKU debe existir, estar activo y llevar cantidad mayor que cero")
	ErrComboAnidado            = errors.New("un combo no puede contener otro combo (sin anidar)")
	ErrComboNoStockeable       = errors.New("un combo no se stockea: no tiene existencia propia (ajúste/transfiera sus componentes)")
	ErrPlatoSinInsumos         = errors.New("un plato necesita al menos un insumo en su receta")
	ErrPlatoInsumoInvalido     = errors.New("insumo de plato inválido: el SKU debe existir, estar activo y llevar cantidad mayor que cero")
	ErrPlatoAnidado            = errors.New("un insumo de plato no puede ser otro plato ni un combo (sin anidar)")
)

// validarCombo valida y normaliza la receta de un producto combo. Un combo:
//   - necesita al menos un componente;
//   - cada componente debe existir, estar activo, no ser a su vez un combo (sin
//     anidar) ni el propio combo, y llevar cantidad > 0;
//   - se vende por unidad y no se stockea, así que su forma de venta se fuerza a
//     "unidad"/UnidadBase "unidad".
//
// Además, si el combo no trae un precio propio (<= 0), se sugiere el default:
// la suma del precio de sus componentes por su cantidad (editable por el
// usuario; solo es un punto de partida cuando no se declara).
func (s *Service) validarCombo(empresaID string, p *inventario.Producto) error {
	if !p.EsCombo {
		p.Componentes = nil
		return nil
	}
	if len(p.Componentes) == 0 {
		return ErrComboSinComponentes
	}
	suma := 0.0
	for i, c := range p.Componentes {
		sku := strings.TrimSpace(c.SKU)
		if sku == "" || sku == p.SKU || c.Cantidad <= 0 {
			return fmt.Errorf("%w: %s", ErrComboComponenteInvalido, sku)
		}
		comp, ok := s.productos.BySKU(empresaID, sku)
		if !ok || !comp.Activo {
			return fmt.Errorf("%w: %s", ErrComboComponenteInvalido, sku)
		}
		if comp.EsCombo {
			return fmt.Errorf("%w: %s", ErrComboAnidado, sku)
		}
		p.Componentes[i].SKU = sku
		suma += comp.Precio * c.Cantidad
	}
	// Un combo se vende por unidad y no participa del stock.
	p.TipoVenta = inventario.TipoVentaUnidad
	p.UnidadBase = inventario.UnidadUnidad
	if p.Precio <= 0 {
		p.Precio = round2(suma)
	}
	return nil
}

// validarPlato valida y normaliza la RECETA (escandallo) de un plato del módulo
// Restaurante. Un plato: se vende por unidad y NO se stockea (se prepara al
// momento) — su forma de venta se fuerza a "unidad"; su PRECIO lo fija el usuario
// (precio de menú, no el costo). Sus insumos deben existir, estar activos y no ser
// otro plato ni un combo (sin anidar). Un plato no puede ser a la vez combo.
func (s *Service) validarPlato(empresaID string, p *inventario.Producto) error {
	if !p.EsPlato {
		p.Receta = nil
		return nil
	}
	// Plato y combo son excluyentes: un plato se factura como una línea y descuenta
	// insumos; un combo se explota en líneas. Prevalece plato.
	p.EsCombo = false
	p.Componentes = nil
	if len(p.Receta) == 0 {
		return ErrPlatoSinInsumos
	}
	for i, c := range p.Receta {
		sku := strings.TrimSpace(c.SKU)
		if sku == "" || sku == p.SKU || c.Cantidad <= 0 {
			return fmt.Errorf("%w: %s", ErrPlatoInsumoInvalido, sku)
		}
		ins, ok := s.productos.BySKU(empresaID, sku)
		if !ok || !ins.Activo {
			return fmt.Errorf("%w: %s", ErrPlatoInsumoInvalido, sku)
		}
		if ins.EsCombo || ins.EsPlato {
			return fmt.Errorf("%w: %s", ErrPlatoAnidado, sku)
		}
		p.Receta[i].SKU = sku
	}
	p.TipoVenta = inventario.TipoVentaUnidad
	p.UnidadBase = inventario.UnidadUnidad
	return nil
}

// normalizarTipoVenta valida y normaliza la forma de venta de un producto. Vacío
// se lee como "unidad" (retrocompat). Para "peso" el producto se cobra por kilos,
// así que su unidad base queda fijada en "kg". Devuelve (tipoVenta, unidadBase).
func normalizarTipoVenta(tipoVenta, unidadBase string) (string, string, error) {
	switch tipoVenta {
	case "", inventario.TipoVentaUnidad:
		return inventario.TipoVentaUnidad, unidadBase, nil
	case inventario.TipoVentaPeso:
		// Un producto por peso se lleva en kilogramos: el precio es Bs/kg y las
		// existencias van en kg. La unidad base se fuerza para que el Kardex y la
		// interfaz no puedan contradecir la forma de venta.
		return inventario.TipoVentaPeso, inventario.UnidadKg, nil
	default:
		return "", unidadBase, ErrTipoVentaInvalido
	}
}

// admitePreciosUSD indica si la empresa puede expresar precios en dólares: lo
// puede porque su moneda principal es el dólar, o porque lo activó (R10).
func (s *Service) admitePreciosUSD(empresaID string) bool {
	if s.empresas == nil {
		return false
	}
	e, ok := s.empresas.ByID(empresaID)
	if !ok {
		return false
	}
	return e.PreciosEnUsd || e.MonedaPrincipal == empresa.MonedaUSD
}

// --- Vistas (proyecciones de lectura) ---

// ExistenciaView es la existencia de un producto en una sede, derivada del ledger.
type ExistenciaView struct {
	ProductoID    string  `json:"productoId"`
	SKU           string  `json:"sku"`
	Nombre        string  `json:"nombre"`
	SedeID        string  `json:"sedeId"`
	AlmacenID     string  `json:"almacenId,omitempty"`
	Cantidad      float64 `json:"cantidad"`
	CostoPromedio float64 `json:"costoPromedio"`
	Valor         float64 `json:"valor"`
}

// KardexLinea es un renglón del Kardex con saldo y costo corridos.
type KardexLinea struct {
	ID            string  `json:"id"`
	Fecha         string  `json:"fecha"`
	Tipo          string  `json:"tipo"`
	Cantidad      float64 `json:"cantidad"`
	CostoUnitario float64 `json:"costoUnitario"`
	Saldo         float64 `json:"saldo"`
	SaldoCosto    float64 `json:"saldoCosto"`
	Motivo        string  `json:"motivo"`
	Ref           string  `json:"ref"`
}

// KardexView es el Kardex completo de un SKU.
type KardexView struct {
	SKU         string        `json:"sku"`
	Nombre      string        `json:"nombre"`
	UnidadBase  string        `json:"unidadBase"`
	Movimientos []KardexLinea `json:"movimientos"`
}

// --- Catálogo ---

// Productos devuelve el catálogo de la empresa.
func (s *Service) Productos(empresaID string) []inventario.Producto {
	return s.productos.List(empresaID)
}

// CrearProducto da de alta un producto validando SKU único por empresa.
func (s *Service) CrearProducto(empresaID, actor, origen string, p inventario.Producto) (inventario.Producto, error) {
	if _, ok := s.productos.BySKU(empresaID, p.SKU); ok {
		return inventario.Producto{}, ErrSKUDuplicado
	}
	// Forma de venta (unidad|peso): un producto por peso se cobra por kilos, así
	// que su unidad base queda fijada en "kg".
	tv, ub, err := normalizarTipoVenta(p.TipoVenta, p.UnidadBase)
	if err != nil {
		return inventario.Producto{}, err
	}
	p.TipoVenta, p.UnidadBase = tv, ub
	// UnidadBase se guarda como el SÍMBOLO del maestro de unidades (Configuración).
	// Validación SUAVE: si viene vacío, por defecto "unidad"; NO se rechaza un
	// símbolo ausente del catálogo (el maestro solo alimenta el select del front,
	// no bloquea el alta ni rompe datos ya cargados).
	if strings.TrimSpace(p.UnidadBase) == "" {
		p.UnidadBase = inventario.UnidadUnidad
	}
	p.EmpresaID = empresaID
	p.Activo = true
	// Moneda del precio (R10): si no se declara, es la principal de la empresa.
	// En dólares solo se admite si la empresa lo habilitó en su configuración —
	// si no, un precio en US$ sería un número sin significado para su caja.
	if p.Moneda == "" {
		p.Moneda = s.MonedaPrincipalDe(empresaID)
	}
	if !empresa.MonedaValida(p.Moneda) {
		return inventario.Producto{}, errors.New("moneda del precio inválida (VES o USD)")
	}
	if p.Moneda == empresa.MonedaUSD && !s.admitePreciosUSD(empresaID) {
		return inventario.Producto{}, ErrPreciosUSDNoHabilitados
	}
	p.CodigoBarras = strings.TrimSpace(p.CodigoBarras)
	if p.CodigoBarras != "" {
		if otro, ok := s.PorCodigoBarras(empresaID, p.CodigoBarras); ok && otro.SKU != p.SKU {
			return inventario.Producto{}, fmt.Errorf("%w: %s", ErrCodigoBarrasEnUso, otro.Nombre)
		}
	}
	// Combo (paquete de otros productos): valida la receta y, si aplica, fuerza
	// unidad/no-stock y sugiere el precio por defecto (suma de componentes).
	if err := s.validarCombo(empresaID, &p); err != nil {
		return inventario.Producto{}, err
	}
	// Plato (receta/escandallo, módulo Restaurante): valida la receta y fuerza
	// unidad/no-stock (descuenta insumos al facturar, no el plato).
	if err := s.validarPlato(empresaID, &p); err != nil {
		return inventario.Producto{}, err
	}
	if p.Receta == nil {
		p.Receta = []inventario.ComboComponente{}
	}
	if p.Presentaciones == nil {
		p.Presentaciones = []inventario.Presentacion{}
	}
	if p.Componentes == nil {
		p.Componentes = []inventario.ComboComponente{}
	}
	out := s.productos.Create(p)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.producto.crear", out.ID, out.SKU+" "+out.Nombre))
	return out, nil
}

/* PorCodigoBarras resuelve un escaneo: busca el código en el producto y también
 * en sus presentaciones (el bulto tiene su propio código). Es lo que permite que
 * la pistola lectora agregue el ítem sola (R11).
 *
 * Devuelve el producto y, si el código era de una presentación, cuál. */
func (s *Service) PorCodigoBarras(empresaID, codigo string) (inventario.Producto, bool) {
	codigo = strings.TrimSpace(codigo)
	if codigo == "" {
		return inventario.Producto{}, false
	}
	productos := s.productos.List(empresaID)
	// Primero los códigos de barras (propios y de presentaciones): son lo que lee
	// la pistola y tienen prioridad sobre el SKU para que un SKU que coincida con
	// el código de otro producto no gane.
	for _, p := range productos {
		if p.CodigoBarras != "" && strings.EqualFold(p.CodigoBarras, codigo) {
			return p, true
		}
		for _, pr := range p.Presentaciones {
			if pr.CodigoBarras != "" && strings.EqualFold(pr.CodigoBarras, codigo) {
				return p, true
			}
		}
	}
	// Después el SKU: hay comercios que imprimen su propia etiqueta con el SKU.
	for _, p := range productos {
		if strings.EqualFold(p.SKU, codigo) {
			return p, true
		}
	}
	return inventario.Producto{}, false
}

// CambiosProducto describe un PATCH (parcial) del catálogo. Sigue la semántica de
// PATCH: un campo AUSENTE conserva su valor actual, nunca lo pisa.
//   - Los booleanos (y el código de barras) son PUNTEROS: nil = no enviado = no
//     cambiar. Esto evita que un PATCH que no los toca (p. ej. {"precio":999}) los
//     caiga a su cero — que dejaría un producto inactivo o a un exento cobrando IVA.
//     Un puntero no-nil sí aplica, incluso a false (desactivar / quitar exención).
//   - Los strings/números mantienen la convención previa: vacío/0 = no cambiar.
//     El código de barras es la excepción (vacío = vaciar a propósito), por eso va
//     como *string: nil no lo toca, "" lo limpia.
type CambiosProducto struct {
	Nombre       string
	Rubro        string
	TipoVenta    string
	UnidadBase   string
	Precio       float64
	Moneda       string
	CodigoBarras *string
	ExentoIVA    *bool
	Activo       *bool
	EsCombo      *bool
	Componentes  []inventario.ComboComponente
	EsPlato      *bool
	Receta       []inventario.ComboComponente
}

// ActualizarProducto edita los datos del catálogo (nombre, precio, moneda, IVA,
// código de barras, rubro y si sigue activo). No toca existencias: eso solo se
// mueve por el ledger. Es un PATCH parcial: los campos ausentes en `cambios`
// conservan su valor actual (ver CambiosProducto).
func (s *Service) ActualizarProducto(empresaID, actor, origen, sku string, cambios CambiosProducto) (inventario.Producto, error) {
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return inventario.Producto{}, ErrProductoNoExiste
	}
	if n := strings.TrimSpace(cambios.Nombre); n != "" {
		p.Nombre = n
	}
	if cambios.Precio > 0 {
		p.Precio = cambios.Precio
	}
	if cambios.Moneda != "" {
		if !empresa.MonedaValida(cambios.Moneda) {
			return inventario.Producto{}, errors.New("moneda del precio inválida (VES o USD)")
		}
		if cambios.Moneda == empresa.MonedaUSD && !s.admitePreciosUSD(empresaID) {
			return inventario.Producto{}, ErrPreciosUSDNoHabilitados
		}
		p.Moneda = cambios.Moneda
	}
	if cambios.Rubro != "" {
		p.Rubro = cambios.Rubro
	}
	// Forma de venta: vacío = no se toca; si viene se valida y, cuando es "peso",
	// se fuerza la unidad base a "kg".
	if cambios.TipoVenta != "" {
		tv, ub, err := normalizarTipoVenta(cambios.TipoVenta, p.UnidadBase)
		if err != nil {
			return inventario.Producto{}, err
		}
		p.TipoVenta, p.UnidadBase = tv, ub
	}
	// UnidadBase (símbolo del maestro de unidades): se puede cambiar mientras el
	// producto no sea por peso —en ese caso queda fijado en "kg"—. Validación SUAVE:
	// se acepta cualquier símbolo (el catálogo solo alimenta el select del front).
	if ub := strings.TrimSpace(cambios.UnidadBase); ub != "" && p.TipoVenta != inventario.TipoVentaPeso {
		p.UnidadBase = ub
	}
	// El código de barras es *string: nil = no enviado = no se toca. Cuando SÍ viene
	// se puede vaciar a propósito ("" limpia), así que se compara contra el valor
	// recibido y no contra «no vacío».
	if cambios.CodigoBarras != nil {
		codigo := strings.TrimSpace(*cambios.CodigoBarras)
		if codigo != p.CodigoBarras {
			if codigo != "" {
				if otro, ok := s.PorCodigoBarras(empresaID, codigo); ok && otro.SKU != p.SKU {
					return inventario.Producto{}, fmt.Errorf("%w: %s", ErrCodigoBarrasEnUso, otro.Nombre)
				}
			}
			p.CodigoBarras = codigo
		}
	}
	// Combo: la edición puede convertir el producto en combo o cambiar su receta.
	// EsCombo es *bool: nil = no enviado = conserva su condición actual. Si viene en
	// true se valida (y se fuerza unidad/no-stock); dejar de ser combo limpia la
	// receta. Componentes nil = no se toca la receta actual.
	if cambios.EsCombo != nil {
		p.EsCombo = *cambios.EsCombo
	}
	if cambios.Componentes != nil {
		p.Componentes = cambios.Componentes
	}
	if err := s.validarCombo(empresaID, &p); err != nil {
		return inventario.Producto{}, err
	}
	if p.Componentes == nil {
		p.Componentes = []inventario.ComboComponente{}
	}
	// Plato (receta): mismo patrón que combo. EsPlato *bool: nil = conserva.
	if cambios.EsPlato != nil {
		p.EsPlato = *cambios.EsPlato
	}
	if cambios.Receta != nil {
		p.Receta = cambios.Receta
	}
	if err := s.validarPlato(empresaID, &p); err != nil {
		return inventario.Producto{}, err
	}
	if p.Receta == nil {
		p.Receta = []inventario.ComboComponente{}
	}
	// IVA y estado activo son *bool: nil = no enviado = conserva el valor actual (no
	// se pisan a false en un PATCH parcial); no-nil aplica el valor, incluido false.
	if cambios.ExentoIVA != nil {
		p.ExentoIVA = *cambios.ExentoIVA
	}
	if cambios.Activo != nil {
		p.Activo = *cambios.Activo
	}
	out, ok := s.productos.Update(p)
	if !ok {
		return inventario.Producto{}, ErrProductoNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "inventario.producto.editar", out.SKU, out.Nombre))
	return out, nil
}

// AgregarPresentacion añade una presentación de venta a un producto.
func (s *Service) AgregarPresentacion(empresaID, productoID, actor, origen string, pres inventario.Presentacion) (inventario.Producto, error) {
	p, ok := s.productos.ByID(empresaID, productoID)
	if !ok {
		return inventario.Producto{}, ErrProductoNoExiste
	}
	p.Presentaciones = append(p.Presentaciones, pres)
	out, _ := s.productos.Update(p)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.presentacion.agregar", p.ID, pres.Nombre))
	return out, nil
}

// --- Existencias y Kardex (proyecciones del ledger) ---

// Existencias proyecta la existencia de cada producto en una sede.
func (s *Service) Existencias(empresaID, sedeID string) []ExistenciaView {
	prods := s.productos.List(empresaID)
	out := make([]ExistenciaView, 0, len(prods))
	for _, p := range prods {
		// Un combo no se stockea: no tiene existencia propia (su stock es el de
		// sus componentes). Se excluye de la proyección para no listarlo con saldo.
		if p.EsCombo {
			continue
		}
		movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID})
		cant, avg := fold(movs)
		out = append(out, ExistenciaView{
			ProductoID:    p.ID,
			SKU:           p.SKU,
			Nombre:        p.Nombre,
			SedeID:        sedeID,
			Cantidad:      cant,
			CostoPromedio: avg,
			Valor:         cant * avg,
		})
	}
	return out
}

// Kardex devuelve el detalle de movimientos de un SKU con saldos corridos.
func (s *Service) Kardex(empresaID, sedeID, sku string) (KardexView, error) {
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return KardexView{}, ErrProductoNoExiste
	}
	movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, SKU: sku})
	return construirKardex(p, movs), nil
}

// construirKardex arma la vista de Kardex (saldo y costo promedio corridos) a
// partir de un conjunto de movimientos ya acotados (por sede o por almacén).
func construirKardex(p inventario.Producto, movs []inventario.Movimiento) KardexView {
	ordenar(movs)
	saldo, avg := 0.0, 0.0
	lineas := make([]KardexLinea, 0, len(movs))
	for _, m := range movs {
		costoLinea := m.CostoUnitario
		if m.Cantidad >= 0 {
			nuevo := saldo + m.Cantidad
			if saldo < 0 {
				// Ingreso que cruza un saldo negativo: toma su propio costo (no mezcla
				// contra el saldo <0, que corrompería el promedio ponderado).
				if nuevo > 0 {
					avg = m.CostoUnitario
				}
			} else {
				valor := saldo*avg + m.Cantidad*m.CostoUnitario
				if nuevo > 0 {
					avg = valor / nuevo
				}
			}
			saldo = nuevo
		} else {
			// Salida: costo de la línea = promedio vigente (COGS); el promedio no cambia.
			costoLinea = avg
			saldo += m.Cantidad
		}
		lineas = append(lineas, KardexLinea{
			ID: m.ID, Fecha: m.Fecha, Tipo: m.Tipo, Cantidad: m.Cantidad,
			CostoUnitario: costoLinea, Saldo: saldo, SaldoCosto: saldo * avg,
			Motivo: m.Motivo, Ref: m.RefID,
		})
	}
	return KardexView{SKU: p.SKU, Nombre: p.Nombre, UnidadBase: p.UnidadBase, Movimientos: lineas}
}

// movsDeAlmacen devuelve los movimientos de un almacén. Retrocompat: si el almacén
// es el PRINCIPAL de su sede, incluye además los movimientos de esa sede SIN almacén
// asignado (los previos a los almacenes).
func (s *Service) movsDeAlmacen(empresaID string, alm almacen.Almacen, f inventario.FiltroMovimiento) []inventario.Movimiento {
	f.SedeID = alm.SedeID
	f.AlmacenID = "" // se filtra en Go para poder incluir los vacíos del principal
	principal := s.esAlmacenPrincipal(empresaID, alm)
	out := []inventario.Movimiento{}
	for _, m := range s.movimientos.List(empresaID, f) {
		if m.AlmacenID == alm.ID || (principal && m.AlmacenID == "") {
			out = append(out, m)
		}
	}
	return out
}

// disponibleEnAlmacen devuelve la existencia de un SKU en un almacén concreto (con
// la retrocompat del principal); si no hay almacén, cae a la existencia por sede.
func (s *Service) disponibleEnAlmacen(empresaID, almacenID, sedeID, sku string) float64 {
	if s.almacenes != nil && almacenID != "" {
		if a, ok := s.almacenes.ByID(empresaID, almacenID); ok {
			cant, _ := fold(s.movsDeAlmacen(empresaID, a, inventario.FiltroMovimiento{SKU: sku}))
			return cant
		}
	}
	cant, _ := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, SKU: sku}))
	return cant
}

// ExistenciasDeAlmacen proyecta la existencia de cada producto en UN almacén.
func (s *Service) ExistenciasDeAlmacen(empresaID, almacenID string) ([]ExistenciaView, error) {
	if s.almacenes == nil {
		return nil, ErrAlmacenesNoDisponible
	}
	alm, ok := s.almacenes.ByID(empresaID, almacenID)
	if !ok {
		return nil, ErrAlmacenNoExiste
	}
	prods := s.productos.List(empresaID)
	out := make([]ExistenciaView, 0, len(prods))
	for _, p := range prods {
		if p.EsCombo {
			continue
		}
		cant, avg := fold(s.movsDeAlmacen(empresaID, alm, inventario.FiltroMovimiento{ProductoID: p.ID}))
		out = append(out, ExistenciaView{
			ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre,
			SedeID: alm.SedeID, AlmacenID: alm.ID,
			Cantidad: cant, CostoPromedio: avg, Valor: cant * avg,
		})
	}
	return out, nil
}

// KardexDeAlmacen devuelve el Kardex de un SKU en UN almacén.
func (s *Service) KardexDeAlmacen(empresaID, almacenID, sku string) (KardexView, error) {
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return KardexView{}, ErrProductoNoExiste
	}
	if s.almacenes == nil {
		return KardexView{}, ErrAlmacenesNoDisponible
	}
	alm, ok := s.almacenes.ByID(empresaID, almacenID)
	if !ok {
		return KardexView{}, ErrAlmacenNoExiste
	}
	return construirKardex(p, s.movsDeAlmacen(empresaID, alm, inventario.FiltroMovimiento{SKU: sku})), nil
}

// MovimientoView es un movimiento del ledger enriquecido con el nombre del producto,
// para el registro de entradas y salidas.
type MovimientoView struct {
	inventario.Movimiento
	Nombre string `json:"nombre"`
}

// Movimientos devuelve el REGISTRO de entradas y salidas: los movimientos del ledger
// (todas las entradas, salidas, ajustes y transferencias) filtrables por sede,
// almacén, SKU, tipo y rango de fechas (YYYY-MM-DD), ordenados del más reciente al
// más antiguo. Es la vista cronológica global que complementa al Kardex (por producto).
func (s *Service) Movimientos(empresaID string, f inventario.FiltroMovimiento, desde, hasta, tipo string) []MovimientoView {
	movs := s.movimientos.List(empresaID, f)
	out := make([]MovimientoView, 0, len(movs))
	for _, m := range movs {
		if tipo != "" && m.Tipo != tipo {
			continue
		}
		fecha := m.Fecha
		if len(fecha) >= 10 {
			fecha = fecha[:10]
		}
		if desde != "" && fecha < desde {
			continue
		}
		if hasta != "" && fecha > hasta {
			continue
		}
		nombre := m.SKU
		if p, ok := s.productos.BySKU(empresaID, m.SKU); ok {
			nombre = p.Nombre
		}
		out = append(out, MovimientoView{Movimiento: m, Nombre: nombre})
	}
	// Más reciente primero (el ledger no garantiza orden de inserción).
	sort.SliceStable(out, func(i, j int) bool { return out[i].Fecha > out[j].Fecha })
	return out
}

// Ajustar registra un ajuste de existencias (merma/conteo) como un movimiento
// nuevo — nunca sobrescribe el saldo. Requiere motivo (auditado).
func (s *Service) Ajustar(empresaID, sedeID, almacenID, sku, motivo string, cantidad float64, actor, origen string) (ExistenciaView, error) {
	if motivo == "" {
		return ExistenciaView{}, ErrMotivoRequerido
	}
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return ExistenciaView{}, ErrProductoNoExiste
	}
	// Un combo no se stockea: no se ajusta su existencia (se ajustan sus componentes).
	if p.EsCombo {
		return ExistenciaView{}, ErrComboNoStockeable
	}
	// Almacén de destino del ajuste: el indicado o el principal de la sede.
	almacenID = s.almacenParaEscritura(empresaID, sedeID, almacenID)
	// Restricción por rubro: solo al INGRESAR stock (cantidad>0); retirar siempre se
	// permite (para poder vaciar un producto que ya no admite el almacén).
	if cantidad > 0 {
		if err := s.verificarAdmisionAlmacen(empresaID, almacenID, p.Rubro); err != nil {
			return ExistenciaView{}, err
		}
	}
	// Para ajustes positivos usamos el costo promedio vigente como costo del ingreso.
	_, avg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID}))
	m := inventario.Movimiento{
		EmpresaID: empresaID, SedeID: sedeID, AlmacenID: almacenID, ProductoID: p.ID, SKU: p.SKU,
		Tipo: inventario.MovAjuste, Cantidad: cantidad, CostoUnitario: avg,
		Motivo: motivo, Actor: actor, Fecha: ahora(),
	}
	guardado := s.movimientos.Append(m)
	// Asiento derivado del ajuste: una merma es costo del período; un sobrante
	// entra al inventario. Sin esto el inventario contable se separaría del real.
	s.asentarMovimientoInventario(empresaID, actor, guardado)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.ajuste", p.SKU, motivo))
	cant, navg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID}))
	return ExistenciaView{ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre, SedeID: sedeID, Cantidad: cant, CostoPromedio: navg, Valor: cant * navg}, nil
}

// --- Transferencias (máquina de estados que emite movimientos) ---

// Transferencias lista las transferencias de la empresa.
func (s *Service) Transferencias(empresaID string) []inventario.Transferencia {
	return s.transferencias.List(empresaID)
}

// CrearTransferencia crea una transferencia en estado borrador.
func (s *Service) CrearTransferencia(empresaID, actor, origen string, t inventario.Transferencia) (inventario.Transferencia, error) {
	if len(t.Lineas) == 0 {
		return inventario.Transferencia{}, ErrTransferVacia
	}
	// Ubicación real: almacén origen/destino (el indicado o el principal de cada
	// sede). Una transferencia puede ser entre sedes o entre almacenes de la misma
	// sede; lo que no se permite es origen == destino (nada que transferir).
	t.OrigenAlmacenID = s.almacenParaEscritura(empresaID, t.OrigenSedeID, t.OrigenAlmacenID)
	t.DestinoAlmacenID = s.almacenParaEscritura(empresaID, t.DestinoSedeID, t.DestinoAlmacenID)
	mismaUbicacion := (t.OrigenAlmacenID != "" && t.OrigenAlmacenID == t.DestinoAlmacenID) ||
		(t.OrigenAlmacenID == "" && t.DestinoAlmacenID == "" && t.OrigenSedeID == t.DestinoSedeID)
	if mismaUbicacion {
		return inventario.Transferencia{}, ErrSedesIguales
	}
	// Enriquecer líneas con nombre/productoID desde el catálogo. Un combo no se
	// stockea: no se transfiere (se transfieren sus componentes).
	for i, l := range t.Lineas {
		if p, ok := s.productos.BySKU(empresaID, l.SKU); ok {
			if p.EsCombo {
				return inventario.Transferencia{}, ErrComboNoStockeable
			}
			// El almacén destino debe admitir el rubro del producto (restricción dura).
			if err := s.verificarAdmisionAlmacen(empresaID, t.DestinoAlmacenID, p.Rubro); err != nil {
				return inventario.Transferencia{}, fmt.Errorf("%s: %w", p.Nombre, err)
			}
			t.Lineas[i].ProductoID = p.ID
			t.Lineas[i].Nombre = p.Nombre
		}
	}
	t.EmpresaID = empresaID
	t.Estado = inventario.TransfBorrador
	t.Creada = ahora()
	t.Actualizada = t.Creada
	// Folio del documento de transferencia: serie "TRF" serializada por empresa+sede
	// origen (mismo Numerador atómico que los documentos fiscales).
	t.Numero = s.numerador.Siguiente(empresaID, t.OrigenSedeID, "TRF")
	t.NumeroCompleto = fmt.Sprintf("TRF-%06d", t.Numero)
	out := s.transferencias.Create(t)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.transferencia.crear", out.NumeroCompleto, ""))
	return out, nil
}

var ordenTransfer = map[string]int{
	inventario.TransfBorrador: 0, inventario.TransfDespachada: 1, inventario.TransfEnTransito: 2,
	inventario.TransfRecibida: 3, inventario.TransfCerrada: 4,
}

// CambiarEstadoTransferencia avanza la máquina de estados (solo al estado
// inmediatamente siguiente) y emite movimientos de ledger en los pasos que
// afectan stock: despacho (salida en origen) y recepción (entrada en destino).
func (s *Service) CambiarEstadoTransferencia(empresaID, id, nuevo, actor, origen string) (inventario.Transferencia, error) {
	t, ok := s.transferencias.ByID(empresaID, id)
	if !ok {
		return inventario.Transferencia{}, ErrTransicionInvalida
	}
	cur, okc := ordenTransfer[t.Estado]
	nxt, okn := ordenTransfer[nuevo]
	if !okc || !okn || nxt != cur+1 {
		return inventario.Transferencia{}, ErrTransicionInvalida
	}
	switch nuevo {
	case inventario.TransfDespachada:
		// No se puede despachar más de lo disponible en el almacén origen: una
		// transferencia no debe dejar el origen en negativo (política de stock).
		for _, l := range t.Lineas {
			disp := s.disponibleEnAlmacen(empresaID, t.OrigenAlmacenID, t.OrigenSedeID, l.SKU)
			if disp < l.Cantidad-0.0001 {
				return inventario.Transferencia{}, fmt.Errorf("%w: %s (disponible %.2f, requiere %.2f)",
					ErrStockInsuficiente, l.Nombre, disp, l.Cantidad)
			}
		}
		for _, l := range t.Lineas {
			_, avg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: t.OrigenSedeID, SKU: l.SKU}))
			s.movimientos.Append(inventario.Movimiento{
				EmpresaID: empresaID, SedeID: t.OrigenSedeID, AlmacenID: t.OrigenAlmacenID, ProductoID: l.ProductoID, SKU: l.SKU,
				Tipo: inventario.MovTransferencia, Cantidad: -l.Cantidad, CostoUnitario: avg,
				Motivo: "despacho transferencia", RefTipo: "transferencia", RefID: t.ID, Actor: actor, Fecha: ahora(),
			})
		}
	case inventario.TransfRecibida:
		for _, l := range t.Lineas {
			// El costo entra al destino al mismo costo con que salió del origen.
			costo := costoSalidaTransfer(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SKU: l.SKU}), t.ID)
			s.movimientos.Append(inventario.Movimiento{
				EmpresaID: empresaID, SedeID: t.DestinoSedeID, AlmacenID: t.DestinoAlmacenID, ProductoID: l.ProductoID, SKU: l.SKU,
				Tipo: inventario.MovTransferencia, Cantidad: l.Cantidad, CostoUnitario: costo,
				Motivo: "recepción transferencia", RefTipo: "transferencia", RefID: t.ID, Actor: actor, Fecha: ahora(),
			})
		}
	}
	t.Estado = nuevo
	t.Actualizada = ahora()
	out, _ := s.transferencias.Update(t)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.transferencia.estado", t.ID, nuevo))
	return out, nil
}

// CancelarTransferencia cancela una transferencia. La cancelación es un estado
// terminal fuera del avance lineal. Mantiene el ledger append-only: nunca borra
// movimientos; si el stock ya salió del origen (despachada/en_transito) lo
// devuelve anexando movimientos positivos de compensación en la sede origen.
// Una vez recibida en destino ya no se puede cancelar: la reversa correcta es
// una nueva transferencia de vuelta destino→origen.
func (s *Service) CancelarTransferencia(empresaID, id, actor, origen, motivo string) (inventario.Transferencia, error) {
	t, ok := s.transferencias.ByID(empresaID, id)
	if !ok {
		return inventario.Transferencia{}, ErrTransicionInvalida
	}
	switch t.Estado {
	case inventario.TransfBorrador:
		// Aún no emitió movimientos: no hay nada que compensar.
	case inventario.TransfDespachada, inventario.TransfEnTransito:
		// La salida en origen ya ocurrió pero aún no se recibió en destino:
		// devolvemos el stock en tránsito al origen con movimientos positivos.
		for _, l := range t.Lineas {
			costo := costoSalidaTransfer(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SKU: l.SKU}), t.ID)
			s.movimientos.Append(inventario.Movimiento{
				EmpresaID: empresaID, SedeID: t.OrigenSedeID, AlmacenID: t.OrigenAlmacenID, ProductoID: l.ProductoID, SKU: l.SKU,
				Tipo: inventario.MovTransferencia, Cantidad: l.Cantidad, CostoUnitario: costo,
				Motivo: "cancelación transferencia", RefTipo: "transferencia", RefID: t.ID, Actor: actor, Fecha: ahora(),
			})
		}
	default:
		// recibida / cerrada / cancelada.
		return inventario.Transferencia{}, ErrTransferNoCancelable
	}
	t.Estado = inventario.TransfCancelada
	t.Actualizada = ahora()
	out, _ := s.transferencias.Update(t)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.transferencia.cancelar", t.ID, motivo))
	return out, nil
}

// --- Helpers ---

// fold reduce un conjunto de movimientos a (cantidad, costoPromedio) vigentes.
func fold(movs []inventario.Movimiento) (float64, float64) {
	ordenar(movs)
	saldo, avg := 0.0, 0.0
	for _, m := range movs {
		if m.Cantidad >= 0 {
			nuevo := saldo + m.Cantidad
			if saldo < 0 {
				// No hay inventario válido previo (saldo negativo): mezclar el promedio
				// contra un saldo <0 lo corrompe. El ingreso que cruza a positivo define
				// el costo; mientras siga ≤0 no hay valuación.
				if nuevo > 0 {
					avg = m.CostoUnitario
				}
			} else {
				valor := saldo*avg + m.Cantidad*m.CostoUnitario
				if nuevo > 0 {
					avg = valor / nuevo
				}
			}
			saldo = nuevo
		} else {
			saldo += m.Cantidad
		}
	}
	return saldo, avg
}

// costoSalidaTransfer recupera el costo unitario con que una línea salió del
// origen en una transferencia, para ingresarla al destino al mismo costo.
func costoSalidaTransfer(movs []inventario.Movimiento, transfID string) float64 {
	for _, m := range movs {
		if m.RefID == transfID && m.Cantidad < 0 {
			return m.CostoUnitario
		}
	}
	return 0
}

func ordenar(movs []inventario.Movimiento) {
	sort.SliceStable(movs, func(i, j int) bool { return movs[i].Fecha < movs[j].Fecha })
}

func ahora() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func evento(empresaID, actor, origen, accion, entidad, detalle string) auditoria.Evento {
	return auditoria.Evento{
		EmpresaID: empresaID, Actor: actor, Accion: accion, Entidad: entidad,
		Detalle: detalle, Origen: origen, Fecha: ahora(),
	}
}

// --- Disponibilidad por ubicación (03 §4.7) ---

// DisponibilidadSede es el stock de un producto en una sede concreta.
type DisponibilidadSede struct {
	SedeID     string  `json:"sedeId"`
	SedeNombre string  `json:"sedeNombre"`
	Cantidad   float64 `json:"cantidad"`
	// EstaTienda marca la sede desde la que se consulta, para que la interfaz
	// pueda destacarla («esta tienda»).
	EstaTienda bool `json:"estaTienda"`
	// Nivel resume el semáforo: agotado | bajo | disponible. Lo calcula el
	// servidor para que todas las interfaces usen el mismo umbral.
	Nivel string `json:"nivel"`
}

// DisponibilidadView responde «¿cuántas unidades hay y en qué tienda?».
type DisponibilidadView struct {
	SKU      string               `json:"sku"`
	Nombre   string               `json:"nombre"`
	Precio   float64              `json:"precio"`
	Sedes    []DisponibilidadSede `json:"sedes"`
	TotalRed float64              `json:"totalRed"`
}

// UmbralStockBajo es el límite por debajo del cual el semáforo pasa a "bajo".
// Debe volverse configurable por empresa cuando exista Configuración.
const UmbralStockBajo = 5

// Disponibilidad recorre TODAS las sedes de la empresa y devuelve el stock del
// SKU en cada una, más el total de la red.
//
// Cruza el aislamiento por SEDE a propósito — es el caso de uso pedido: el
// cajero necesita saber en qué otra tienda de la cadena hay existencias. NO
// cruza el aislamiento por TENANT: sigue filtrando por empresaID en cada
// consulta, que es la frontera que de verdad protege datos ajenos.
func (s *Service) Disponibilidad(empresaID, sedeActual, sku string, sedes []SedeRef) (DisponibilidadView, error) {
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return DisponibilidadView{}, ErrProductoNoExiste
	}
	out := DisponibilidadView{SKU: p.SKU, Nombre: p.Nombre, Precio: p.Precio}
	for _, sd := range sedes {
		movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sd.ID, ProductoID: p.ID})
		cant, _ := fold(movs)
		nivel := "disponible"
		switch {
		case cant <= 0:
			nivel = "agotado"
		case cant <= UmbralStockBajo:
			nivel = "bajo"
		}
		out.Sedes = append(out.Sedes, DisponibilidadSede{
			SedeID: sd.ID, SedeNombre: sd.Nombre, Cantidad: cant,
			EstaTienda: sd.ID == sedeActual, Nivel: nivel,
		})
		out.TotalRed += cant
	}
	return out, nil
}

// SedeRef es el mínimo que el servicio necesita saber de una sede. El adaptador
// HTTP se lo pasa desde TenancyService, para que Inventario no dependa del
// puerto de sedes.
type SedeRef struct {
	ID     string
	Nombre string
}

// AsignarImagenProducto guarda la URL de la imagen de un producto. El archivo ya
// fue escrito en el bucket del tenant por el adaptador de almacenamiento; acá
// solo se persiste la referencia y se audita. Devuelve la URL anterior para que
// el adaptador pueda borrar el archivo que queda huérfano.
func (s *Service) AsignarImagenProducto(empresaID, actor, origen, sku, url string) (inventario.Producto, string, error) {
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return inventario.Producto{}, "", ErrProductoNoExiste
	}
	anterior := p.ImagenURL
	p.ImagenURL = url
	out, _ := s.productos.Update(p)
	accion := "inventario.producto.imagen"
	if url == "" {
		accion = "inventario.producto.imagen.quitar"
	}
	s.audit.Append(evento(empresaID, actor, origen, accion, out.SKU, url))
	return out, anterior, nil
}
