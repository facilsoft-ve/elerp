package compra

// NotaCompra es una nota de crédito o débito de PROVEEDOR: el documento con el que
// un proveedor AJUSTA una factura de compra ya registrada.
//
//   - Nota de CRÉDITO de compra (devolución/descuento): el proveedor te acredita —
//     DISMINUYE lo que le debes. Se guarda con montos NEGATIVOS (misma convención de
//     reversa que la nota de crédito de cliente en fiscal.Documento).
//   - Nota de DÉBITO de compra (cargo adicional: flete, interés, corrección al alza):
//     el proveedor te carga más — AUMENTA lo que le debes. Montos POSITIVOS.
//
// Es APPEND-ONLY e INMUTABLE (como FacturaCompra y el documento fiscal de venta):
// una vez registrada no se edita ni se borra. Referencia SIEMPRE la factura de
// compra original (FacturaCompraID) y hereda su orden y proveedor, para que las
// proyecciones (Cuentas por pagar, Libro de compras) puedan imputarle el ajuste.
// El IVA se calcula con la ALÍCUOTA HISTÓRICA de la factura de compra (no la de hoy).
//
// Una nota de crédito puede ser de DOS naturalezas, y la diferencia la marcan sus
// Lineas (ver LineaNotaCompra):
//
//   - CON líneas ⇒ DEVOLUCIÓN FÍSICA: la mercancía vuelve al proveedor. Anexa
//     movimientos de salida al ledger de inventario y su base se DERIVA de las
//     líneas (cantidad × costo con que entró), nunca se teclea.
//   - SIN líneas ⇒ AJUSTE DE MONTO (descuento, rebaja, interés): el stock no se
//     mueve, así que el asiento tampoco toca la cuenta de inventario.
type NotaCompra struct {
	ID              string `json:"id" bson:"id"`
	EmpresaID       string `json:"empresaId" bson:"empresaid"` // tenant
	FacturaCompraID string `json:"facturaCompraId" bson:"facturacompraid"`
	OrdenCompraID   string `json:"ordenCompraId" bson:"ordencompraid"`
	ProveedorID     string `json:"proveedorId" bson:"proveedorid"`
	ProveedorNombre string `json:"proveedorNombre" bson:"proveedornombre"`
	ProveedorRIF    string `json:"proveedorRif" bson:"proveedorrif"`

	// Tipo: NotaCreditoCompra | NotaDebitoCompra.
	Tipo string `json:"tipo" bson:"tipo"`

	// Numeración/serie propia de ElERP (correlativa interna, atómica por
	// empresa+sede+serie vía el Numerador): "NCC" (crédito) / "NDC" (débito).
	Serie          string `json:"serie" bson:"serie"`
	Numero         int    `json:"numero" bson:"numero"`
	NumeroCompleto string `json:"numeroCompleto" bson:"numerocompleto"`

	// Datos del documento tal como lo emitió el PROVEEDOR (no los genera ElERP):
	// su correlativo, su número de control fiscal (SENIAT) y su fecha.
	NumeroDocumento string `json:"numeroDocumento" bson:"numerodocumento"`
	NumeroControl   string `json:"numeroControl" bson:"numerocontrol"`
	Fecha           string `json:"fecha" bson:"fecha"`

	// Concepto explica el ajuste (motivo de la devolución/descuento o del cargo).
	Concepto string `json:"concepto" bson:"concepto"`

	// Lineas son los renglones DEVUELTOS al proveedor. Vacío ⇒ la nota es un ajuste
	// de monto y no movió inventario. Con líneas, cada una tiene su movimiento de
	// salida en el ledger, referenciado por RefTipo "nota_compra" + RefID = esta nota.
	Lineas []LineaNotaCompra `json:"lineas" bson:"lineas"`

	// Montos: NEGATIVOS para la nota de crédito (resta), POSITIVOS para la de
	// débito (suma). Total = BaseImponible + BaseExenta + IVA (con el mismo signo).
	BaseImponible float64 `json:"baseImponible" bson:"baseimponible"`
	BaseExenta    float64 `json:"baseExenta" bson:"baseexenta"`
	IVA           float64 `json:"iva" bson:"iva"`
	Total         float64 `json:"total" bson:"total"`

	Moneda     string `json:"moneda" bson:"moneda"`
	Registrada string `json:"registrada" bson:"registrada"` // UTC RFC3339 (cuándo se registró)
	Actor      string `json:"actor" bson:"actor"`
}

// Tipos de nota de compra.
const (
	NotaCreditoCompra = "nota_credito"
	NotaDebitoCompra  = "nota_debito"
)

// LineaNotaCompra es un renglón de mercancía DEVUELTA al proveedor en una nota de
// crédito. Las cantidades se guardan en POSITIVO (lo devuelto); el signo de la
// nota vive en sus montos, y el signo de la salida, en el movimiento del ledger.
//
// CostoUnitario es el costo con que la mercancía ENTRÓ (el de la línea de la orden
// de compra), no el promedio vigente: devolver tiene que sacar del inventario
// exactamente el valor que metió la recepción, o el asiento y el Kardex se separan.
type LineaNotaCompra struct {
	ProductoID    string  `json:"productoId" bson:"productoid"`
	SKU           string  `json:"sku" bson:"sku"`
	Nombre        string  `json:"nombre" bson:"nombre"`
	Cantidad      float64 `json:"cantidad" bson:"cantidad"`
	CostoUnitario float64 `json:"costoUnitario" bson:"costounitario"`
	// CostoSalida es el costo PROMEDIO vigente con el que la mercancía sale del
	// Kardex, que no tiene por qué coincidir con CostoUnitario: el ledger valora por
	// promedio ponderado, no por lote, igual que en una venta. De él sale el importe
	// que baja de la cuenta de inventario; la diferencia contra lo que el proveedor
	// acredita es un resultado del período.
	CostoSalida float64 `json:"costoSalida" bson:"costosalida"`
	// Total es el NETO devuelto en esta línea = cantidad * costoUnitario (positivo).
	Total float64 `json:"total" bson:"total"`
	// Exento se hereda de la línea de la orden: una devolución de mercancía exenta
	// no acredita IVA.
	Exento bool `json:"exento" bson:"exento"`
}

// EsDevolucion indica si la nota movió inventario (trae líneas devueltas).
func (n NotaCompra) EsDevolucion() bool { return len(n.Lineas) > 0 }

// NotaCompraRepo persiste las notas de crédito/débito de compra, aislado por
// empresaID. Es APPEND-ONLY: no expone Update ni Delete a propósito. ByFactura
// permite imputar el ajuste a la factura de compra (y a su orden) en Cuentas por
// pagar y en el Libro de compras.
type NotaCompraRepo interface {
	List(empresaID string) []NotaCompra
	ByID(empresaID, id string) (NotaCompra, bool)
	ByFactura(empresaID, facturaID string) []NotaCompra
	Append(n NotaCompra) NotaCompra
}
