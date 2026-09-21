package compra

// COSTO EN DESTINO (landed cost): lo que la mercancía costó ADEMÁS de lo que
// facturó el proveedor.
//
// POR QUÉ EXISTE. Un flete internacional, un impuesto de importación, un seguro o
// una gestión aduanal se pagan aparte y, sin esto, no entran al costo del
// producto. El inventario queda valorado por debajo de lo que de verdad costó y
// CADA VENTA muestra un margen que no existe. No falla nada: simplemente todos los
// números de rentabilidad están mal, y hacia arriba, que es la dirección en la que
// nadie los cuestiona.
//
// CÓMO SE APLICA. El ledger de inventario es de solo anexado y la recepción ya
// ocurrió, así que el costo NO reescribe la entrada: se anexa una REVALUACIÓN
// (inventario.MovRevaluacion) por producto, que sube el promedio ponderado sin
// mover unidades. El documento guarda el reparto para poder explicarlo después.
type CostoEnDestino struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// OrdenCompraID es la recepción sobre la que se reparte. El costo en destino
	// siempre pertenece a una compra concreta: repartirlo «en general» sería
	// imposible de auditar.
	OrdenCompraID string `json:"ordenCompraId" bson:"ordencompraid"`
	OrdenNumero   string `json:"ordenNumero" bson:"ordennumero"`
	// Descripcion dice QUÉ costó ("Flete internacional", "Impuesto de importación").
	// Es lo que hace legible el Kardex seis meses después.
	Descripcion string `json:"descripcion" bson:"descripcion"`
	// Monto es el costo a repartir. Puede ser NEGATIVO: es la única forma de
	// corregir un costo mal cargado sin romper el solo-anexado — se aplica el
	// contrario, igual que un asiento se corrige con su reverso.
	Monto float64 `json:"monto" bson:"monto"`
	// Criterio de reparto entre las líneas recibidas.
	Criterio string `json:"criterio" bson:"criterio"`

	Lineas []LineaCostoEnDestino `json:"lineas" bson:"lineas"`

	// Absorbido es la parte que entró al inventario; AlGasto la que no pudo,
	// porque esa mercancía ya se vendió. Suman exactamente Monto.
	//
	// La distinción es contable, no un detalle: el costo de lo que ya salió no se
	// puede capitalizar —no hay existencia que valorar—, así que es gasto del
	// período. Meterlo igual al inventario inflaría la valorización con un costo
	// que no corresponde a ninguna unidad en el almacén.
	Absorbido float64 `json:"absorbido" bson:"absorbido"`
	AlGasto   float64 `json:"alGasto" bson:"algasto"`

	Actor    string `json:"actor" bson:"actor"`
	Aplicado string `json:"aplicado" bson:"aplicado"` // UTC RFC3339
}

// LineaCostoEnDestino es el reparto a un producto, guardado para poder rehacerlo
// a mano.
type LineaCostoEnDestino struct {
	ProductoID string `json:"productoId" bson:"productoid"`
	SKU        string `json:"sku" bson:"sku"`
	Nombre     string `json:"nombre" bson:"nombre"`
	// Recibido es lo que entró por esta orden; EnStock, lo que queda hoy. De su
	// proporción sale cuánto del reparto puede capitalizarse.
	Recibido float64 `json:"recibido" bson:"recibido"`
	EnStock  float64 `json:"enStock" bson:"enstock"`
	// Reparto es lo que le tocó del monto; Absorbido, lo que de eso entró al
	// inventario. La diferencia es la parte que corresponde a lo ya vendido.
	Reparto   float64 `json:"reparto" bson:"reparto"`
	Absorbido float64 `json:"absorbido" bson:"absorbido"`
}

// Criterios de reparto.
const (
	// CriterioValor reparte en proporción al VALOR recibido de cada producto. Es el
	// habitual: un flete se contrata por lo que vale la carga.
	CriterioValor = "valor"
	// CriterioCantidad reparte en proporción a las UNIDADES recibidas. Sirve cuando
	// lo que encarece es manipular bultos, no su precio.
	CriterioCantidad = "cantidad"
)

// CriterioValido acota el criterio de reparto.
func CriterioValido(c string) bool {
	return c == CriterioValor || c == CriterioCantidad
}

// CostoEnDestinoRepo persiste los costos en destino, aislado por empresaID.
//
// Es de SOLO ANEXADO, como el ledger que alimenta: un costo mal cargado no se
// edita ni se borra, se corrige aplicando otro con el monto contrario. Editarlo
// dejaría el documento diciendo una cosa y las revaluaciones del Kardex otra.
type CostoEnDestinoRepo interface {
	List(empresaID string) []CostoEnDestino
	PorOrden(empresaID, ordenID string) []CostoEnDestino
	Append(c CostoEnDestino) CostoEnDestino
}
