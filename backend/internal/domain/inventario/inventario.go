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
	// MovRevaluacion sube (o baja) el VALOR del stock sin mover una sola unidad.
	// Es lo que permite que un flete o un impuesto de importación entre al costo
	// del producto después de haberlo recibido: el costo en destino.
	//
	// Lleva Cantidad 0 y su valor en ValorAgregado. Sin un tipo propio no se podía
	// expresar: un movimiento de cantidad 0 atraviesa el pliegue sin cambiar nada,
	// y meterlo como una entrada de cantidad simbólica habría inventado unidades.
	MovRevaluacion = "revaluacion"
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
	/* MermaPct es lo que se PIERDE al preparar ESTE insumo, en porcentaje: el
	 * pelado del tomate, la limpieza de la carne, el recorte del pan.
	 *
	 * Va por insumo y no por fórmula porque es una propiedad del ingrediente: de
	 * un tomate se descarta el 10% siempre, entre en la receta que entre. Sin
	 * esto, la fórmula pide 2 kg de tomate, se sacan 2 kg del almacén y a la olla
	 * entran 1,8 — y la cuenta nunca cierra, pero cierra "poco", que es la forma
	 * en que un error se queda años.
	 *
	 * Cero ⇒ sin merma de preparación, que es el comportamiento anterior. Solo
	 * aplica a recetas de fabricación; en un combo se ignora. */
	MermaPct float64 `json:"mermaPct,omitempty" bson:"mermapct,omitempty"`
}

// CantidadBruta es cuánto hay que SACAR del almacén para que lleguen `Cantidad`
// al proceso, contando la merma de preparación.
func (c ComboComponente) CantidadBruta(factor float64) float64 {
	bruta := c.Cantidad * factor
	if c.MermaPct > 0 {
		bruta *= 1 + c.MermaPct/100
	}
	return bruta
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
	// AlicuotaCodigo apunta al MAESTRO DE IMPUESTOS (fiscal.Alicuota): "general",
	// "reducida", "suntuario", "exento"… Se guarda el CÓDIGO y no el porcentaje,
	// para que una providencia que cambie la tasa no obligue a tocar el catálogo
	// entero.
	//
	// VACÍO es válido y significa «como siempre»: exento si ExentoIVA, general si
	// no. Así los catálogos ya cargados siguen facturando igual sin migración.
	AlicuotaCodigo string `json:"alicuotaCodigo,omitempty" bson:"alicuotacodigo,omitempty"`
	// ConceptoISLR apunta al MAESTRO DE CONCEPTOS (fiscal.ConceptoISLR) cuando el
	// producto es un SERVICIO sujeto a retención de ISLR: honorarios,
	// arrendamiento, fletes… Vacío = no sujeto (el caso de toda mercancía).
	//
	// Nota de la contadora (15:53): la tabla de conceptos y sus porcentajes tiene
	// que ser configurable Y quedar asociada a los productos tipo servicio, para
	// que al facturarlos la retención salga sola en vez de teclearse.
	ConceptoISLR string `json:"conceptoIslr,omitempty" bson:"conceptoislr,omitempty"`
	Activo       bool   `json:"activo" bson:"activo"`
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
	// (el modo de fabricación está debajo, junto a la receta)
	// Receta son los insumos que consume UNA unidad del plato: SKU del insumo y
	// cantidad por plato (p. ej. 0.12 kg de pasta, 0.05 kg de queso). Cada insumo es
	// un producto normal del catálogo. Solo tiene sentido cuando EsPlato.
	Receta []ComboComponente `json:"receta" bson:"receta"`
	/* MODO DE FABRICACIÓN: cuándo se convierten los insumos en el producto.
	 *
	 *   · BAJO PEDIDO (por defecto, y lo que hacía siempre): el producto no se
	 *     stockea. Al venderlo, el inventario descuenta sus INSUMOS. Es la pasta
	 *     que se prepara cuando la piden.
	 *   · PARA STOCK: se fabrica antes con una orden, y lo producido entra al
	 *     inventario como cualquier mercancía. Al venderlo se descuenta ÉL, no sus
	 *     insumos — ya se consumieron al fabricarlo. Es la bandeja de postres que
	 *     está en la vitrina, el lote de pan, la pieza armada en el taller.
	 *
	 * La diferencia vive acá y no en el módulo porque es una propiedad del
	 * producto: un mismo local tiene los dos a la vez. Y vacío se lee como bajo
	 * pedido, así que ningún plato ya cargado cambia de comportamiento. */
	ModoFabricacion string `json:"modoFabricacion,omitempty" bson:"modofabricacion,omitempty"`

	/* LA FÓRMULA, EN SERIO.
	 *
	 * Una receta de tres campos alcanza para una torta. No alcanza para producir:
	 * una fórmula real dice para qué TANDA está escrita, cuánto RINDE y cuánta
	 * desviación es aceptable. Sin eso no se puede distinguir «se produjo menos de
	 * lo esperado» de «se planificó mal», que es justamente lo que un control de
	 * producción existe para responder.
	 *
	 * Los tres tienen default neutro, así que toda receta ya cargada se comporta
	 * exactamente igual que antes y no hay nada que migrar.
	 */

	// LoteBase es para cuántas unidades está escrita la fórmula. Una receta dice
	// «para 10 kg de masa», no «para 1». Cero o uno ⇒ la receta es por unidad.
	LoteBase float64 `json:"loteBase,omitempty" bson:"lotebase,omitempty"`
	/* RendimientoPct es cuánto del lote SALE como producto terminado, en
	 * porcentaje. 10 kg de pollo crudo rinden 6,5 kg de pollo cocido: el agua se
	 * fue, y esa pérdida es del PROCESO, no de ningún insumo en particular.
	 *
	 * Se usa al revés de lo que parece: para obtener 6,5 kg hay que partir de más
	 * insumo, no de menos. Cero o cien ⇒ el proceso no pierde. */
	RendimientoPct float64 `json:"rendimientoPct,omitempty" bson:"rendimientopct,omitempty"`
	/* ToleranciaPct es cuánta desviación entre lo esperado y lo producido se
	 * considera normal. Fuera de ella, la orden queda marcada para que alguien
	 * mire: una tanda que rinde 20% menos no es mala suerte dos veces seguidas.
	 *
	 * Cero ⇒ no se controla. Es el default a propósito: avisar de desviaciones a
	 * quien no declaró cuál le importa es enseñarle a ignorar el aviso. */
	ToleranciaPct float64 `json:"toleranciaPct,omitempty" bson:"toleranciapct,omitempty"`
	/* EsServicio marca algo que SE VENDE PERO NO SE STOCKEA: un envío a
	 * domicilio, una instalación, una hora de mano de obra.
	 *
	 * Es distinto de un plato —que tampoco se stockea pero sí consume insumos— y
	 * de un insumo, que se stockea y no se vende. Un servicio no mueve el Kardex
	 * en absoluto: facturarlo sin esta marca lo dejaría con existencia negativa
	 * creciendo para siempre y ensuciaría la valorización del inventario con algo
	 * que no es mercancía.
	 */
	EsServicio bool `json:"esServicio" bson:"esservicio"`
	// EsInsumo marca una MATERIA PRIMA: se compra y se stockea (participa del Kardex y
	// del costo promedio) pero NO se vende directamente — se consume por la receta de
	// un plato. Un restaurante vende platos, bebidas y productos de reventa; no vende
	// el kilo de pasta cruda. Por eso los insumos quedan fuera de las pantallas de
	// venta (POS, comandera, ventas y cotizaciones) y no necesitan precio de venta.
	EsInsumo bool `json:"esInsumo" bson:"esinsumo"`

	// --- Trazabilidad por lote ------------------------------------------------
	//
	// RequiereLote obliga a declarar el LOTE al recibir y hace que el stock se lleve
	// lote por lote. Es lo que permite responder «¿a quién le vendí el lote X?»
	// cuando el fabricante manda a retirarlo, y sin eso una alerta sanitaria se
	// atiende sacando TODO el producto del anaquel.
	//
	// ControlaVencimiento añade la fecha de caducidad al lote. Se separa de
	// RequiereLote porque hay lotes sin vencimiento (un lote de fabricación de
	// tornillos) y no tiene sentido pedir una fecha que nadie va a mirar.
	// ControlaVencimiento sin RequiereLote no significa nada: la fecha vive en el
	// lote, así que activarlo implica el otro.
	//
	// Las dos son FALSAS por defecto: los catálogos ya cargados siguen funcionando
	// exactamente igual, sin lote y sin migración.
	RequiereLote        bool `json:"requiereLote" bson:"requierelote"`
	ControlaVencimiento bool `json:"controlaVencimiento" bson:"controlavencimiento"`
	// ComanderaID fija POR QUÉ COMANDERA sale este producto cuando la comanda se
	// manda a preparación, sin depender de su rubro. Un postre con receta y un plato
	// de cocina son los dos "platos", pero se preparan en puestos distintos; y dos
	// postres de la misma carta pueden ir uno a la barra de postres y otro a cocina.
	// Vacío ⇒ se rutea por el RUBRO (comportamiento de siempre) y, si no encaja en
	// ninguno, por la comandera predeterminada: así un producto nuevo nunca se pierde.
	ComanderaID string `json:"comanderaId,omitempty" bson:"comanderaid,omitempty"`
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
	AlmacenID string `json:"almacenId" bson:"almacenid"`
	// UbicacionID ubica el movimiento DENTRO del almacén (pasillo, estante, muelle).
	// Vacío = «el almacén, sin más detalle», que es todo el histórico y todo almacén
	// que no se haya dividido: sin migración, igual que se hizo con el lote.
	//
	// La valoración NO lo usa: el costo promedio sigue siendo por producto y sede.
	// La ubicación dice dónde está la unidad, no cuánto vale.
	UbicacionID   string  `json:"ubicacionId,omitempty" bson:"ubicacionid,omitempty"`
	ProductoID    string  `json:"productoId" bson:"productoid"`
	SKU           string  `json:"sku" bson:"sku"`
	Tipo          string  `json:"tipo" bson:"tipo"`
	Cantidad      float64 `json:"cantidad" bson:"cantidad"`
	CostoUnitario float64 `json:"costoUnitario" bson:"costounitario"`
	// ValorAgregado es el valor TOTAL que este movimiento añade al stock sin mover
	// unidades. Solo lo usa MovRevaluacion; en los demás tipos es 0 y se ignora.
	//
	// Va como valor total y no como costo unitario porque el reparto ya se hizo
	// aguas arriba: lo que se reparte es un flete entre varios productos, y volver
	// a dividirlo por la cantidad acá daría un número distinto si el stock cambió
	// entre el reparto y el pliegue.
	ValorAgregado float64 `json:"valorAgregado,omitempty" bson:"valoragregado,omitempty"`
	// Lote identifica el lote de fabricación al que pertenece esta cantidad, y
	// Vencimiento su caducidad (AAAA-MM-DD). Vacíos en todo producto que no la
	// exige, que son casi todos.
	//
	// Van en el MOVIMIENTO y no en una tabla de lotes aparte por la misma razón que
	// el resto del inventario: el saldo de un lote es una proyección del ledger, no
	// un contador que alguien mantiene. Un contador se desincroniza del Kardex y
	// entonces hay dos verdades.
	//
	// El COSTO no se lleva por lote: la valoración sigue siendo el promedio
	// ponderado del producto. Separarla por lote sería otro sistema de valoración
	// (FIFO por capas), no un añadido — y mezclarlos daría dos cifras distintas
	// para el mismo inventario.
	Lote        string `json:"lote,omitempty" bson:"lote,omitempty"`
	Vencimiento string `json:"vencimiento,omitempty" bson:"vencimiento,omitempty"`
	Motivo      string `json:"motivo" bson:"motivo"`
	RefTipo     string `json:"refTipo" bson:"reftipo"` // p. ej. "transferencia"
	RefID       string `json:"refId" bson:"refid"`
	Actor       string `json:"actor" bson:"actor"`
	Fecha       string `json:"fecha" bson:"fecha"` // UTC RFC3339, ordenable
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
	// CuentaInventario permite que este rubro se acumule en su propia cuenta de
	// activo en vez de en la única de inventario. Vacío = la de siempre, que es lo
	// que tiene toda empresa que no toque esto.
	//
	// POR QUÉ. Una ferretería que vende tornillos y también maquinaria quiere verlos
	// separados en el balance; con una sola cuenta, el contador tiene que estimarlo
	// a mano cada cierre.
	//
	// LA REGLA QUE NO SE PUEDE ROMPER: si un rubro tiene cuenta propia, TODOS sus
	// asientos —entrada, venta, merma, sobrante, devolución, costo en destino— van a
	// ella. En cuanto uno solo se quede en la cuenta general, esa cuenta crece para
	// siempre y la general se va a negativo, con el balance cuadrando en los dos
	// casos. Por eso la resuelve un único sitio (cuentaInventarioDe) y no cada
	// asiento por su cuenta.
	CuentaInventario string `json:"cuentaInventario,omitempty" bson:"cuentainventario,omitempty"`
}

// FiltroMovimiento acota una consulta al ledger. EmpresaID es obligatorio en
// el adaptador de persistencia (aislamiento de tenant).
type FiltroMovimiento struct {
	SedeID      string
	AlmacenID   string
	UbicacionID string
	ProductoID  string
	SKU         string
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
	// Update existe para poder asignarle su cuenta contable. Un rubro no se borra:
	// los productos que lo referencian dejarían de tener categoría.
	Update(r Rubro) (Rubro, bool)
}

// Modos de fabricación de un producto con receta.
const (
	// FabricaBajoPedido: no se stockea; al venderlo se descuentan sus insumos.
	FabricaBajoPedido = "bajo_pedido"
	// FabricaParaStock: se fabrica con una orden y entra al inventario.
	FabricaParaStock = "para_stock"
)

// SeFabricaParaStock indica si el producto se produce ANTES y se guarda. Solo
// tiene sentido en un producto con receta: sin receta no hay nada que fabricar.
func (p Producto) SeFabricaParaStock() bool {
	return p.ModoFabricacion == FabricaParaStock && len(p.Receta) > 0
}

/* SePreparaAlPedirlo indica si este producto hay que PRODUCIRLO cuando lo piden.
 *
 * No es lo mismo que «tiene receta»: un postre fabricado para stock tiene receta
 * y ya está hecho, en la vitrina. Mandarlo a cocina haría esperar al cliente por
 * algo que está a tres metros, y dejaría la comanda abierta hasta que alguien
 * marque como listo un plato que nadie va a preparar.
 */
func (p Producto) SePreparaAlPedirlo() bool {
	return p.EsPlato && len(p.Receta) > 0 && !p.SeFabricaParaStock()
}

/* SeStockea indica si el producto tiene existencia propia en el Kardex.
 *
 * Es la pregunta que antes estaba repetida en cada sitio como «EsCombo ||
 * EsPlato || EsServicio», y que la fabricación para stock vuelve más sutil: un
 * plato que se fabrica antes SÍ se stockea, porque está en la vitrina. Tenerla
 * en un solo lugar es lo que evita que una pantalla lo cuente y otra no.
 */
func (p Producto) SeStockea() bool {
	if p.EsCombo || p.EsServicio {
		return false
	}
	if p.EsPlato {
		return p.SeFabricaParaStock()
	}
	return true
}

/* LoteDeFormula es el tamaño de tanda para el que está escrita la receta.
 * Normaliza el caso «no declarado» a 1, que es como se comportaba antes.
 */
func (p Producto) LoteDeFormula() float64 {
	if p.LoteBase > 0 {
		return p.LoteBase
	}
	return 1
}

// RendimientoDeFormula es la fracción del lote que sale como producto terminado
// (1 = no se pierde nada). Se acota a (0, 1]: un rendimiento de cero o negativo
// haría falta infinito insumo, y uno mayor que 100% sería crear materia.
func (p Producto) RendimientoDeFormula() float64 {
	if p.RendimientoPct <= 0 || p.RendimientoPct > 100 {
		return 1
	}
	return p.RendimientoPct / 100
}

/* FactorDeFormula es por cuánto hay que multiplicar cada renglón de la receta
 * para obtener `objetivo` unidades de producto terminado.
 *
 * Contempla las dos pérdidas, que son distintas y se aplican en sitios
 * distintos: el RENDIMIENTO es del proceso y entra acá (para sacar 6,5 hay que
 * meter para 10); la MERMA es de cada insumo y entra en CantidadBruta.
 */
func (p Producto) FactorDeFormula(objetivo float64) float64 {
	return objetivo / (p.LoteDeFormula() * p.RendimientoDeFormula())
}

// DesviacionAceptable indica si producir `real` contra `esperado` entra dentro de
// la tolerancia declarada. Sin tolerancia declarada no se controla nada.
func (p Producto) DesviacionAceptable(esperado, real float64) bool {
	if p.ToleranciaPct <= 0 || esperado <= 0 {
		return true
	}
	desvio := (real - esperado) / esperado * 100
	if desvio < 0 {
		desvio = -desvio
	}
	return desvio <= p.ToleranciaPct
}
