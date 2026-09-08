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
