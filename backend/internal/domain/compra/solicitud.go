package compra

// SolicitudCompra (RFQ, «solicitud de presupuesto») es el paso PREVIO a la orden
// de compra: se le pide presupuesto a uno o varios proveedores, se comparan sus
// respuestas y se convierte a orden de compra la del proveedor elegido. A
// diferencia de la orden (que alimenta el ledger de inventario al recibir), la
// solicitud es un documento de GESTIÓN, editable en borrador; al enviarla queda
// registrada. No toca stock ni contabilidad: solo negocia precios.

// Estados de la máquina de una solicitud de presupuesto.
const (
	SolBorrador   = "borrador"   // recién armada, editable
	SolEnviada    = "enviada"    // pedida a los proveedores, a la espera de respuestas
	SolRespondida = "respondida" // al menos un proveedor respondió
	SolCerrada    = "cerrada"    // negociación terminada (convertida a orden o cerrada a mano)
	SolCancelada  = "cancelada"  // descartada
)

// Estados de la cotización de un proveedor concreto dentro de la solicitud.
const (
	CotizaPendiente  = "pendiente"  // se le pidió, aún no responde
	CotizaRespondida = "respondida" // cargó sus precios
)

// LineaSolicitud es un renglón de producto que se pide cotizar. No lleva precio:
// el precio lo pone cada proveedor en su respuesta (RespuestaLinea).
type LineaSolicitud struct {
	ProductoID string  `json:"productoId" bson:"productoid"`
	SKU        string  `json:"sku" bson:"sku"`
	Nombre     string  `json:"nombre" bson:"nombre"`
	Cantidad   float64 `json:"cantidad" bson:"cantidad"`
}

// RespuestaLinea es el precio unitario que un proveedor cotizó para un SKU.
type RespuestaLinea struct {
	SKU            string  `json:"sku" bson:"sku"`
	PrecioUnitario float64 `json:"precioUnitario" bson:"preciounitario"`
}

// ProveedorCotiza es un proveedor al que se le pide presupuesto, con su respuesta.
// Total es la suma de precioUnitario × cantidad de cada línea que cotizó (derivado
// al registrar la respuesta), lo que permite comparar entre proveedores.
type ProveedorCotiza struct {
	ProveedorID     string           `json:"proveedorId" bson:"proveedorid"`
	ProveedorNombre string           `json:"proveedorNombre" bson:"proveedornombre"`
	Estado          string           `json:"estado" bson:"estado"` // CotizaPendiente | CotizaRespondida
	Respondida      bool             `json:"respondida" bson:"respondida"`
	Lineas          []RespuestaLinea `json:"lineas" bson:"lineas"`
	Total           float64          `json:"total" bson:"total"`
	RespondidaEn    string           `json:"respondidaEn" bson:"respondidaen"` // UTC RFC3339, vacío si no ha respondido
}

// SolicitudCompra es un pedido de presupuesto a uno o varios proveedores.
type SolicitudCompra struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID    string `json:"sedeId" bson:"sedeid"`

	Numero         int    `json:"numero" bson:"numero"`
	NumeroCompleto string `json:"numeroCompleto" bson:"numerocompleto"` // serie "SOL" vía Numerador
	Serie          string `json:"serie" bson:"serie"`
	Estado         string `json:"estado" bson:"estado"`

	Fecha string `json:"fecha" bson:"fecha"` // fecha del documento (UTC RFC3339)
	Notas string `json:"notas" bson:"notas"`

	Lineas      []LineaSolicitud  `json:"lineas" bson:"lineas"`
	Proveedores []ProveedorCotiza `json:"proveedores" bson:"proveedores"`

	// Trazabilidad de la conversión: qué proveedor se eligió y qué orden se generó.
	ProveedorElegidoID string `json:"proveedorElegidoId" bson:"proveedorelegidoid"`
	OrdenGeneradaID    string `json:"ordenGeneradaId" bson:"ordengeneradaid"`

	Actor       string `json:"actor" bson:"actor"`
	Creada      string `json:"creada" bson:"creada"`           // UTC RFC3339
	Actualizada string `json:"actualizada" bson:"actualizada"` // UTC RFC3339
}

// SolicitudCompraRepo persiste solicitudes de presupuesto, aislado por empresaID.
// Es un documento de gestión editable (Update), no un ledger append-only.
type SolicitudCompraRepo interface {
	List(empresaID string) []SolicitudCompra
	ByID(empresaID, id string) (SolicitudCompra, bool)
	Create(s SolicitudCompra) SolicitudCompra
	Update(s SolicitudCompra) (SolicitudCompra, bool)
}
