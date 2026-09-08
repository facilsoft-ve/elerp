// Package tesoreria modela el dinero que entra DESPUÉS de la factura: los cobros
// de una venta a crédito.
//
// Decisión de diseño, la misma que el resto del sistema: es un LEDGER DE
// SOLO-ANEXADO. El saldo de un cliente no es un campo que se edita, es la
// proyección de «total facturado − todo lo cobrado». Así el histórico explica
// siempre cómo se llegó al saldo, y corregir un cobro mal registrado es anexar su
// reverso, no borrarlo.
//
// Por qué existe: hasta ahora toda venta se cobraba completa en el mostrador. En
// el comercio venezolano la venta a crédito al abasto o a la bodega vecina es
// corriente, y sin ella el KPI «Por cobrar vencido» no puede existir.
package tesoreria

// Cobro es un pago recibido contra un documento fiscal ya emitido.
type Cobro struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// DocumentoID es la factura que se está cobrando. Un cobro sin documento no
	// existe: no hay dinero que entre «en general».
	DocumentoID   string `json:"documentoId" bson:"documentoid"`
	DocumentoNum  string `json:"documentoNumero" bson:"documentonumero"`
	ClienteID     string `json:"clienteId" bson:"clienteid"`
	ClienteNombre string `json:"clienteNombre" bson:"clientenombre"`
	// Monto en la moneda del cobro; MontoBs es su equivalente con la tasa del día
	// del cobro, que también se guarda (Art. 177: la tasa del hecho, no la de hoy).
	Monto      float64 `json:"monto" bson:"monto"`
	Moneda     string  `json:"moneda" bson:"moneda"`
	MontoBs    float64 `json:"montoBs" bson:"montobs"`
	TasaCambio float64 `json:"tasaCambio" bson:"tasacambio"`
	TasaFuente string  `json:"tasaFuente" bson:"tasafuente"`
	// IGTF (en Bs) sobre un abono pagado en DIVISA: el cliente entrega el abono más
	// este impuesto. 0 en cobros en bolívares. Se declara al SENIAT (ReporteIGTF).
	IGTF   float64 `json:"igtf" bson:"igtf"`
	Metodo string  `json:"metodo" bson:"metodo"`
	// CuentaID es la cuenta de cobro donde entró el dinero (banco, pago móvil,
	// Zelle). Vacío en efectivo: ese entra a la caja física.
	CuentaID   string `json:"cuentaId" bson:"cuentaid"`
	Referencia string `json:"referencia" bson:"referencia"`
	// Reverso marca un cobro que anula otro anterior (RefCobroID). Un cobro mal
	// registrado no se borra: se reversa, igual que un documento fiscal.
	Reverso    bool   `json:"reverso" bson:"reverso"`
	RefCobroID string `json:"refCobroId" bson:"refcobroid"`
	Motivo     string `json:"motivo" bson:"motivo"`
	Actor      string `json:"actor" bson:"actor"`
	Fecha      string `json:"fecha" bson:"fecha"` // UTC RFC3339
}

// Repository es el puerto del ledger de cobros. SOLO-ANEXADO a propósito: no hay
// Update ni Delete. Toda consulta se aísla por empresaID.
type Repository interface {
	Append(c Cobro) Cobro
	List(empresaID string) []Cobro
	// PorDocumento devuelve los cobros de una factura, para calcular su saldo.
	PorDocumento(empresaID, documentoID string) []Cobro
	ByID(empresaID, id string) (Cobro, bool)
}
