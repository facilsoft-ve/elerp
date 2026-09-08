package tesoreria

// PagoProveedor es el dinero que SALE hacia un proveedor: el espejo del Cobro,
// pero del lado del pasivo. Se anexa a un ledger de SOLO-ANEXADO; el saldo por
// pagar no es un campo que se edita, es la proyección de «lo adeudado − lo
// pagado». Así corregir un pago mal registrado es anexar su reverso, no borrarlo.
type PagoProveedor struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	// ProveedorID es a quién se le paga. Un pago sin proveedor no existe.
	ProveedorID     string `json:"proveedorId" bson:"proveedorid"`
	ProveedorNombre string `json:"proveedorNombre" bson:"proveedornombre"`
	// OrdenCompraID imputa el pago a una orden concreta; vacío cuando es un pago a
	// cuenta del proveedor (abona su deuda global, no una orden en particular).
	OrdenCompraID string `json:"ordenCompraId" bson:"ordencompraid"`
	// Monto en la moneda del pago; MontoBs es su equivalente en bolívares. Hoy el
	// pago se registra directamente en Bs, así que ambos coinciden, pero se guardan
	// separados para admitir pagos en divisa sin cambiar el esquema (Art. 177).
	Monto      float64 `json:"monto" bson:"monto"`
	MontoBs    float64 `json:"montoBs" bson:"montobs"`
	Moneda     string  `json:"moneda" bson:"moneda"`
	Metodo     string  `json:"metodo" bson:"metodo"`
	Referencia string  `json:"referencia" bson:"referencia"`
	Fecha      string  `json:"fecha" bson:"fecha"` // UTC RFC3339
	Actor      string  `json:"actor" bson:"actor"`
	// Reverso marca un pago que anula otro anterior (RefPagoID). Un pago mal
	// registrado no se borra: se reversa, igual que un cobro o un documento fiscal.
	Reverso   bool   `json:"reverso" bson:"reverso"`
	RefPagoID string `json:"refPagoId" bson:"refpagoid"`
	Motivo    string `json:"motivo" bson:"motivo"`
}

// PagoProveedorRepo es el puerto del ledger de pagos a proveedor. SOLO-ANEXADO a
// propósito: no hay Update ni Delete. Toda consulta se aísla por empresaID.
type PagoProveedorRepo interface {
	List(empresaID string) []PagoProveedor
	ByID(empresaID, id string) (PagoProveedor, bool)
	Append(p PagoProveedor) PagoProveedor
}
