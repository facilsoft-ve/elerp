package compra

// LineaFacturaCompra es un renglón de la factura del proveedor TAL COMO viene en
// su documento: puede diferir de lo ordenado/recibido (precio distinto, cantidad
// ajustada, ítem extra o faltante). Es la línea del proveedor, no la de la OC.
// CostoUnitario es NETO (sin IVA); Total = Cantidad * CostoUnitario.
type LineaFacturaCompra struct {
	ProductoID    string  `json:"productoId" bson:"productoid"`
	SKU           string  `json:"sku" bson:"sku"`
	Nombre        string  `json:"nombre" bson:"nombre"`
	Cantidad      float64 `json:"cantidad" bson:"cantidad"`
	CostoUnitario float64 `json:"costoUnitario" bson:"costounitario"`
	Total         float64 `json:"total" bson:"total"`
	Exento        bool    `json:"exento" bson:"exento"`
}

// FacturaCompra es la factura FISCAL del proveedor asociada a una orden de compra
// recibida: registra el número de control del SENIAT y el IVA crédito fiscal del
// documento. Es APPEND-ONLY (como el documento fiscal de venta): una vez
// registrada no se edita ni se borra; una corrección sería otro hecho (nota de
// crédito/débito de proveedor). La recepción de la orden ya asentó Inventario /
// Cuentas por pagar por el costo NETO recibido; la factura registra el documento
// del proveedor con SUS PROPIAS líneas e importes, que pueden diferir de lo
// recibido. La factura AGREGA a la contabilidad el IVA crédito fiscal y la
// diferencia de base (facturado − recibido) contra la deuda con el proveedor, sin
// re-mover el inventario (el costo del Kardex es el efectivamente recibido).
type FacturaCompra struct {
	ID              string `json:"id" bson:"id"`
	EmpresaID       string `json:"empresaId" bson:"empresaid"` // tenant
	OrdenCompraID   string `json:"ordenCompraId" bson:"ordencompraid"`
	ProveedorID     string `json:"proveedorId" bson:"proveedorid"`
	ProveedorNombre string `json:"proveedorNombre" bson:"proveedornombre"`
	ProveedorRIF    string `json:"proveedorRif" bson:"proveedorrif"`

	// NumeroFactura es el correlativo de la factura tal como la emitió el
	// proveedor; NumeroControl es el número de control fiscal (SENIAT). Ambos son
	// del documento del proveedor, no los genera ElERP.
	NumeroFactura string `json:"numeroFactura" bson:"numerofactura"`
	NumeroControl string `json:"numeroControl" bson:"numerocontrol"`
	Fecha         string `json:"fecha" bson:"fecha"` // fecha del documento (del proveedor)

	// Lineas son los renglones del documento del proveedor (sus cantidades y
	// precios), que pueden diferir de la OC. De aquí se derivan las bases.
	Lineas []LineaFacturaCompra `json:"lineas" bson:"lineas"`

	BaseImponible float64 `json:"baseImponible" bson:"baseimponible"` // base gravada (de la factura)
	BaseExenta    float64 `json:"baseExenta" bson:"baseexenta"`
	IVA           float64 `json:"iva" bson:"iva"` // crédito fiscal (de la factura)
	Total         float64 `json:"total" bson:"total"`

	// BaseRecibida es el costo NETO que la recepción asentó en Inventario / CxP
	// (Σ cantidadRecibida * costoUnitario de la OC). DiferenciaBase es
	// (BaseImponible + BaseExenta) − BaseRecibida: lo que la factura del proveedor
	// difiere de lo efectivamente recibido. Se guardan para que el documento sea
	// autocontenido y la diferencia quede trazable sin recomputarla desde la OC.
	BaseRecibida   float64 `json:"baseRecibida" bson:"baserecibida"`
	DiferenciaBase float64 `json:"diferenciaBase" bson:"diferenciabase"`

	Moneda     string `json:"moneda" bson:"moneda"`
	Registrada string `json:"registrada" bson:"registrada"` // UTC RFC3339 (cuándo se registró)
	Actor      string `json:"actor" bson:"actor"`
}

// FacturaCompraRepo persiste las facturas de compra, aislado por empresaID. Es
// APPEND-ONLY: no expone Update ni Delete a propósito. ByOrden permite exigir una
// sola factura por orden de compra.
type FacturaCompraRepo interface {
	List(empresaID string) []FacturaCompra
	ByID(empresaID, id string) (FacturaCompra, bool)
	ByOrden(empresaID, ordenID string) (FacturaCompra, bool)
	Append(f FacturaCompra) FacturaCompra
}
