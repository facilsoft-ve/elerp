// Package cotizacion modela el flujo de VENTA FORMA LIBRE previo al documento
// fiscal: cotización → confirmar (pedido/prefactura) → facturar.
//
// A diferencia del documento fiscal (inmutable, append-only), una cotización SÍ
// se edita mientras está en borrador: es un presupuesto que se negocia con el
// cliente. Solo cuando se factura se emite un documento fiscal inmutable (una
// factura forma libre) y la cotización queda enlazada a él por DocumentoID; a
// partir de ahí ya no se toca. La numeración usa la serie "COT" del Numerador,
// distinta de la fiscal, porque una cotización no es un comprobante fiscal.
package cotizacion

// Estados de la máquina de una cotización.
const (
	EstadoBorrador   = "borrador"
	EstadoConfirmada = "confirmada"
	EstadoFacturada  = "facturada"
	EstadoCancelada  = "cancelada"
)

// Linea es un renglón de la cotización. Se guarda con el precio y la condición
// de IVA del momento en que se armó; la conversión y el cálculo fiscal final los
// rehace el servidor al facturar.
type Linea struct {
	ProductoID  string  `json:"productoId" bson:"productoid"`
	SKU         string  `json:"sku" bson:"sku"`
	Nombre      string  `json:"nombre" bson:"nombre"`
	Descripcion string  `json:"descripcion" bson:"descripcion"` // texto libre opcional por línea
	Cantidad    float64 `json:"cantidad" bson:"cantidad"`
	// PrecioUnitario es el precio ORIGINAL de lista (sin descuento).
	PrecioUnitario float64 `json:"precioUnitario" bson:"preciounitario"`
	// Descuento es el porcentaje 0..100 aplicado a la línea (0 = sin descuento).
	Descuento float64 `json:"descuento" bson:"descuento"`
	// Total es el NETO de la línea = cantidad * precioUnitario * (1 - descuento/100).
	Total  float64 `json:"total" bson:"total"`
	Exento bool    `json:"exento" bson:"exento"`
}

// Cotizacion es un presupuesto de venta forma libre.
type Cotizacion struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID    string `json:"sedeId" bson:"sedeid"`

	Numero         int    `json:"numero" bson:"numero"`
	NumeroCompleto string `json:"numeroCompleto" bson:"numerocompleto"` // serie "COT" vía Numerador
	Estado         string `json:"estado" bson:"estado"`

	ClienteID        string `json:"clienteId" bson:"clienteid"`
	ClienteNombre    string `json:"clienteNombre" bson:"clientenombre"`
	ClienteDocumento string `json:"clienteDocumento" bson:"clientedocumento"`

	Lineas   []Linea `json:"lineas" bson:"lineas"`
	Subtotal float64 `json:"subtotal" bson:"subtotal"`
	IVA      float64 `json:"iva" bson:"iva"`
	// IGTF es 0 en la cotización: el impuesto sobre pagos en divisas depende de
	// CÓMO se cobre, y eso solo se sabe al facturar. Se deja el campo para no
	// mentir sobre la forma del total, pero no se calcula aquí.
	IGTF  float64 `json:"igtf" bson:"igtf"`
	Total float64 `json:"total" bson:"total"`

	Moneda     string  `json:"moneda" bson:"moneda"`
	TasaCambio float64 `json:"tasaCambio" bson:"tasacambio"`

	Validez         string `json:"validez" bson:"validez"`                 // texto libre ("15 días")
	CondicionesPago string `json:"condicionesPago" bson:"condicionespago"` // "Contado", "15 días", "30 días"
	Terminos        string `json:"terminos" bson:"terminos"`               // términos y condiciones / notas al pie
	Notas           string `json:"notas" bson:"notas"`
	// CuponCodigo: cupón aplicado a la cotización (el descuento ya está bajado en el
	// precio de las líneas). Se guarda para CONSUMIR el uso del cupón al facturar.
	CuponCodigo string `json:"cuponCodigo" bson:"cuponcodigo"`

	// ListaPrecio identifica la lista de precio aplicada. Hoy solo existe la lista
	// base ("" o "base"); el campo queda listo para cuando el catálogo de listas
	// sea real (no altera los precios todavía).
	ListaPrecio string `json:"listaPrecio" bson:"listaprecio"`
	// DireccionEntrega es el destino de despacho de la mercancía (texto libre; por
	// defecto la dirección del cliente, editable en el formulario).
	DireccionEntrega string `json:"direccionEntrega" bson:"direccionentrega"`
	// SedeDespacho es el almacén/sede DE DONDE SALE la mercancía. Al facturar, la
	// salida de inventario se descuenta de esta sede en vez de la del contexto.
	// Vacío ⇒ se descuenta de la sede de la cotización (comportamiento previo).
	SedeDespacho string `json:"sedeDespacho" bson:"sededespacho"`

	// CuentaMesaID enlaza la prefactura con la cuenta de mesa que la originó (módulo
	// Restaurante). Sirve para cerrar la cuenta y liberar la mesa cuando todas sus
	// prefacturas quedaron facturadas. Vacío en una cotización normal de Ventas.
	CuentaMesaID string `json:"cuentaMesaId" bson:"cuentamesaid"`
	// MesaNombre rotula la prefactura con la mesa ("Mesa 5"), que es como la busca el
	// cajero: no conoce números de cotización, conoce mesas.
	MesaNombre string `json:"mesaNombre" bson:"mesanombre"`
	// DocumentoID enlaza a la factura emitida al facturar. Vacío hasta entonces.
	DocumentoID string `json:"documentoId" bson:"documentoid"`

	Actor       string `json:"actor" bson:"actor"`
	Fecha       string `json:"fecha" bson:"fecha"`             // UTC RFC3339
	Actualizada string `json:"actualizada" bson:"actualizada"` // UTC RFC3339
}

// Repository persiste cotizaciones, aislado por empresaID. A diferencia de los
// ledgers, acá SÍ hay Update: una cotización en borrador se negocia y cambia.
type Repository interface {
	List(empresaID string) []Cotizacion
	ByID(empresaID, id string) (Cotizacion, bool)
	Create(c Cotizacion) Cotizacion
	Update(c Cotizacion) (Cotizacion, bool)
}
