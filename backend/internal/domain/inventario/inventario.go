// Package inventario define el modelo de inventario de ElERP.
//
// Decisión de diseño clave (ADR-03 / §13.2): el inventario es un LEDGER de
// movimientos de solo-anexado. La existencia y el Kardex son PROYECCIONES
// derivadas de esos movimientos, nunca contadores editables. Un ajuste o una
// transferencia NO sobrescriben un saldo: emiten nuevos movimientos auditables.
package inventario

// Unidades base soportadas.
const (
	UnidadUnidad = "unidad"
	UnidadKg     = "kg"
	UnidadLitro  = "litro"
)

// Formas de venta de un producto (cómo se cobra la cantidad).
const (
	// TipoVentaUnidad: el producto se vende por unidades enteras o según su
	// unidad base habitual. Es el comportamiento por defecto y el que asume un
	// producto que no declara TipoVenta (retrocompat, sin migración).
	TipoVentaUnidad = "unidad"
	// TipoVentaPeso: el producto se vende por KILOGRAMOS. El precio es por kg y
	// las cantidades/existencias van en kg con decimales (charcutería, granel).
	// Un producto peso siempre tiene UnidadBase == "kg".
	TipoVentaPeso = "peso"
)

// Tipos de movimiento del ledger.
const (
	MovEntrada       = "entrada"       // compra/recepción (cantidad > 0, con costoUnitario)
	MovSalida        = "salida"        // venta/despacho (cantidad < 0)
	MovAjuste        = "ajuste"        // conteo físico/merma (cantidad +/-, requiere motivo)
	MovTransferencia = "transferencia" // pata de una transferencia entre sedes
)

// Estados de una transferencia (máquina de estados).
const (
	TransfBorrador   = "borrador"
	TransfDespachada = "despachada"
	TransfEnTransito = "en_transito"
	TransfRecibida   = "recibida"
	TransfCerrada    = "cerrada"
	// TransfCancelada es un estado terminal fuera del avance lineal: se llega a él
	// solo vía CancelarTransferencia, nunca por CambiarEstadoTransferencia.
	TransfCancelada = "cancelada"
)

// ComboComponente es un renglón de la receta de un producto combo: qué SKU
// entra y en qué cantidad. El componente es un producto normal del catálogo
// (nunca otro combo), y el combo se explota en las líneas de sus componentes al
// facturar.
type ComboComponente struct {
	SKU      string  `json:"sku" bson:"sku"`
	Cantidad float64 `json:"cantidad" bson:"cantidad"`
}

// Presentacion es una forma de venta de un producto (p. ej. "Bulto x24") que
// descuenta del MISMO stock base mediante un factor de conversión.
type Presentacion struct {
	ID               string  `json:"id" bson:"id"`
	Nombre           string  `json:"nombre" bson:"nombre"`
	FactorConversion float64 `json:"factorConversion" bson:"factorconversion"`
	Precio           float64 `json:"precio" bson:"precio"`
	CodigoBarras     string  `json:"codigoBarras" bson:"codigobarras"`
}

// Producto es un artículo del catálogo. El stock vive en el ledger, no aquí.
type Producto struct {
	ID         string `json:"id" bson:"id"`
	EmpresaID  string `json:"empresaId" bson:"empresaid"`
	SKU        string `json:"sku" bson:"sku"`
	Nombre     string `json:"nombre" bson:"nombre"`
	Rubro      string `json:"rubro" bson:"rubro"`
	UnidadBase string `json:"unidadBase" bson:"unidadbase"`
	// TipoVenta indica cómo se cobra la cantidad: "unidad" (por defecto) o
	// "peso". Cuando es "peso" el producto se vende por KILOGRAMOS —Precio es el
	// precio POR KG y las cantidades/existencias van en kg con decimales (hasta
	// 3)— y UnidadBase queda fijado en "kg". Vacío/ausente se lee como "unidad",
	// así los productos ya existentes no necesitan migración.
	TipoVenta string  `json:"tipoVenta" bson:"tipoventa"`
	Precio    float64 `json:"precio" bson:"precio"`
	// Moneda declara EN QUÉ MONEDA está expresado Precio ("VES" o "USD", R10).
	// El precio homólogo en la otra moneda se CALCULA con la tasa del día y
	// nunca se guarda: dos precios guardados se desincronizan en cuanto la tasa
	// cambia. Vacío se lee como la moneda principal de la empresa, así que los
	// productos ya existentes no necesitan migración.
	Moneda string `json:"moneda" bson:"moneda"`
	// CodigoBarras es el código PROPIO del producto (R11), además del de cada
	// presentación. Es de uso intensivo en Venezuela: la pistola lectora escribe
	// en el buscador y el producto se agrega solo. Único por empresa cuando está
	// presente — un escaneo tiene que resolver a un solo producto.
	CodigoBarras string `json:"codigoBarras" bson:"codigobarras"`
	// ExentoIVA marca el producto como no gravado. En Venezuela la harina de
	// maíz, el arroz y buena parte de la cesta básica están exentos; facturar
	// IVA sobre ellos es un error fiscal, no un redondeo.
	ExentoIVA bool `json:"exentoIva" bson:"exentoiva"`
	Activo    bool `json:"activo" bson:"activo"`
	// ImagenURL apunta al archivo en el bucket de la empresa. Vacío significa
	// «sin imagen asignada» y la interfaz muestra su marcador explícito, nunca
	// una foto ajena ni un hueco silencioso.
	ImagenURL      string         `json:"imagenUrl" bson:"imagenurl"`
	Presentaciones []Presentacion `json:"presentaciones" bson:"presentaciones"`
	// EsCombo marca el producto como un PAQUETE: un grupo de otros productos
	// vendido junto, con su propio Precio (predeterminado = suma de sus
	// componentes, pero editable). Un combo se vende por unidad (TipoVenta
	// "unidad") y NO se stockea: no tiene existencia propia ni participa del
	// Kardex. Al facturar no entra como línea; el frontend lo EXPLOTA en las
	// líneas de sus componentes (precio prorrateado e IVA por ítem).
	EsCombo bool `json:"esCombo" bson:"escombo"`
	// Componentes es la receta del combo: los SKU que lo integran y sus
	// cantidades. Solo tiene sentido cuando EsCombo. Cada componente es un
	// producto normal y activo del catálogo, nunca otro combo (sin anidar).
	Componentes []ComboComponente `json:"componentes" bson:"componentes"`
	// EsPlato marca un PLATO con RECETA (escandallo), del módulo Restaurante: se
	// vende como UNA línea a su precio (a diferencia del combo, que se explota en
	// líneas), pero al facturarlo el inventario descuenta sus INSUMOS (Receta), no
	// el plato en sí (que no lleva stock: se prepara al momento).
	EsPlato bool `json:"esPlato" bson:"esplato"`
	// Receta son los insumos que consume UNA unidad del plato: SKU del insumo y
	// cantidad por plato (p. ej. 0.12 kg de pasta, 0.05 kg de queso). Cada insumo es
	// un producto normal del catálogo. Solo tiene sentido cuando EsPlato.
	Receta []ComboComponente `json:"receta" bson:"receta"`
}

// Movimiento es una entrada inmutable del ledger de inventario.
// Convención de signo: cantidad > 0 suma stock, cantidad < 0 lo resta.
type Movimiento struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// AlmacenID ubica el movimiento en un almacén DENTRO de la sede. Retrocompat:
	// un movimiento con AlmacenID vacío (los previos a los almacenes) se interpreta
	// como del almacén PRINCIPAL de su sede en la capa de proyección. La existencia
	// por sede sigue siendo la suma de todos los almacenes de esa sede.
	AlmacenID     string  `json:"almacenId" bson:"almacenid"`
	ProductoID    string  `json:"productoId" bson:"productoid"`
	SKU           string  `json:"sku" bson:"sku"`
	Tipo          string  `json:"tipo" bson:"tipo"`
	Cantidad      float64 `json:"cantidad" bson:"cantidad"`
	CostoUnitario float64 `json:"costoUnitario" bson:"costounitario"`
	Motivo        string  `json:"motivo" bson:"motivo"`
	RefTipo       string  `json:"refTipo" bson:"reftipo"` // p. ej. "transferencia"
	RefID         string  `json:"refId" bson:"refid"`
	Actor         string  `json:"actor" bson:"actor"`
	Fecha         string  `json:"fecha" bson:"fecha"` // UTC RFC3339, ordenable
}

// LineaTransferencia es un renglón de una transferencia.
type LineaTransferencia struct {
	ProductoID string  `json:"productoId" bson:"productoid"`
	SKU        string  `json:"sku" bson:"sku"`
	Nombre     string  `json:"nombre" bson:"nombre"`
	Cantidad   float64 `json:"cantidad" bson:"cantidad"`
}

// Transferencia mueve stock entre dos ubicaciones mediante una máquina de estados.
// Las sedes origen/destino identifican el documento; los almacenes origen/destino
// (dentro de esas sedes) son la ubicación real del stock. Almacén vacío ⇒ el
// principal de la sede correspondiente (retrocompat con transferencias previas).
type Transferencia struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// Folio del documento de transferencia (serie "TRF", por empresa+sede origen).
	Numero           int                  `json:"numero" bson:"numero"`
	NumeroCompleto   string               `json:"numeroCompleto" bson:"numerocompleto"`
	OrigenSedeID     string               `json:"origenSedeId" bson:"origensedeid"`
	DestinoSedeID    string               `json:"destinoSedeId" bson:"destinosedeid"`
	OrigenAlmacenID  string               `json:"origenAlmacenId" bson:"origenalmacenid"`
	DestinoAlmacenID string               `json:"destinoAlmacenId" bson:"destinoalmacenid"`
	Estado           string               `json:"estado" bson:"estado"`
	Lineas           []LineaTransferencia `json:"lineas" bson:"lineas"`
	Creada           string               `json:"creada" bson:"creada"`
	Actualizada      string               `json:"actualizada" bson:"actualizada"`
}

// Rubro clasifica productos (categoría del catálogo).
type Rubro struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Nombre    string `json:"nombre" bson:"nombre"`
}

// FiltroMovimiento acota una consulta al ledger. EmpresaID es obligatorio en
// el adaptador de persistencia (aislamiento de tenant).
type FiltroMovimiento struct {
	SedeID     string
	AlmacenID  string
	ProductoID string
	SKU        string
}

// --- Puertos de persistencia ---

// ProductoRepo persiste productos. Toda consulta se aísla por empresaID.
type ProductoRepo interface {
	List(empresaID string) []Producto
	ByID(empresaID, id string) (Producto, bool)
	BySKU(empresaID, sku string) (Producto, bool)
	Create(p Producto) Producto
	Update(p Producto) (Producto, bool)
}

// MovimientoRepo es el ledger de solo-anexado: Append + lectura, nunca update/delete.
type MovimientoRepo interface {
	Append(m Movimiento) Movimiento
	List(empresaID string, f FiltroMovimiento) []Movimiento
}

// TransferenciaRepo persiste transferencias (el documento cambia de estado; los
// movimientos que emite van al ledger inmutable).
type TransferenciaRepo interface {
	List(empresaID string) []Transferencia
	ByID(empresaID, id string) (Transferencia, bool)
	Create(t Transferencia) Transferencia
	Update(t Transferencia) (Transferencia, bool)
}

// RubroRepo persiste rubros (categorías) por empresa.
type RubroRepo interface {
	List(empresaID string) []Rubro
	Create(r Rubro) Rubro
}
