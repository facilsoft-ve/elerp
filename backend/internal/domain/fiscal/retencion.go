package fiscal

// Dirección del comprobante de retención.
const (
	// RetencionRecibida: un cliente agente de retención te retuvo impuesto sobre una
	// FACTURA DE VENTA propia. Baja tu CxC y registra un activo (impuesto a favor).
	RetencionRecibida = "recibida"
	// RetencionEmitida: TÚ retienes el impuesto al proveedor sobre una FACTURA DE
	// COMPRA. Baja tu CxP y crea el pasivo (impuesto retenido por enterar al SENIAT).
	RetencionEmitida = "emitida"
)

// Impuesto sobre el que recae la retención. Un mismo documento puede tener a la
// vez una retención de IVA y una de ISLR de la misma dirección: la unicidad es
// por (tipo, impuesto, documento), no solo por documento.
const (
	// ImpuestoIVA: retención de IVA. La base es el IVA del documento; el monto es
	// Base * %/100, sin sustraendo.
	ImpuestoIVA = "iva"
	// ImpuestoISLR: retención de Impuesto Sobre La Renta. La base es el monto
	// gravable de la operación (no el IVA), el porcentaje es la tarifa del concepto
	// y se aplica un sustraendo (tabla del reglamento): monto = max(0, Base*%/100 −
	// sustraendo).
	ImpuestoISLR = "islr"
)

// Retencion es un comprobante de retención (IVA o ISLR) en cualquiera de las dos
// direcciones. Es APPEND-ONLY (como el documento fiscal y la factura de compra):
// una vez registrado no se edita ni se borra. El monto retenido se deriva de la
// base y del porcentaje (menos el sustraendo en ISLR) y se graba autocontenido
// para que el comprobante no dependa de que la factura o el tercero sigan igual
// mañana.
type Retencion struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	// Tipo: "recibida" (sobre factura de venta) | "emitida" (sobre factura de compra).
	Tipo string `json:"tipo" bson:"tipo"`
	// Impuesto: "iva" | "islr".
	Impuesto string `json:"impuesto" bson:"impuesto"`
	// DocumentoID referencia la factura sobre la que se retiene: la factura de
	// VENTA (documento fiscal) si es recibida, o la factura de COMPRA si es emitida.
	DocumentoID     string `json:"documentoId" bson:"documentoid"`
	DocumentoNumero string `json:"documentoNumero" bson:"documentonumero"` // para mostrar
	// NumeroComprobante es el del comprobante de retención (lo emite el agente de
	// retención: el cliente si recibida, tú si emitida).
	NumeroComprobante string `json:"numeroComprobante" bson:"numerocomprobante"`
	Fecha             string `json:"fecha" bson:"fecha"` // fecha del comprobante
	// Tercero es el cliente (recibida) o el proveedor (emitida).
	TerceroNombre string `json:"terceroNombre" bson:"terceronombre"`
	TerceroRIF    string `json:"terceroRif" bson:"tercerorif"`
	// Base es el monto gravable sobre el que se calcula la retención: para IVA es el
	// IVA del documento; para ISLR es la base entrada (el monto sujeto a retención).
	Base float64 `json:"base" bson:"base"`
	// Concepto es la etiqueta del concepto ISLR (honorarios, arrendamientos, …).
	// Vacío en IVA.
	Concepto string `json:"concepto" bson:"concepto"`
	// Sustraendo se resta al calcular la retención de ISLR (tabla del reglamento).
	// 0 en IVA.
	Sustraendo    float64 `json:"sustraendo" bson:"sustraendo"`
	Porcentaje    float64 `json:"porcentaje" bson:"porcentaje"`       // 75 | 100 típicamente
	MontoRetenido float64 `json:"montoRetenido" bson:"montoretenido"` // derivado de Base, % y sustraendo
	Actor         string  `json:"actor" bson:"actor"`
	Registrada    string  `json:"registrada" bson:"registrada"` // UTC RFC3339
}

// RetencionRepo persiste los comprobantes de retención, aislado por empresaID. Es
// APPEND-ONLY: no expone Update ni Delete a propósito. ByDocumento permite exigir
// una sola retención por dirección E impuesto sobre un mismo documento (una
// factura puede tener retención de IVA y de ISLR de la misma dirección).
type RetencionRepo interface {
	List(empresaID string) []Retencion
	ByID(empresaID, id string) (Retencion, bool)
	// ByDocumento devuelve la retención de un tipo e impuesto dados sobre un documento.
	ByDocumento(empresaID, tipo, impuesto, docID string) (Retencion, bool)
	Append(r Retencion) Retencion
}
