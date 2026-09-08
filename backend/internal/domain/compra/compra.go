// Package compra modela la Orden de compra: el flujo de aprovisionamiento
// borrador → confirmada → recepción (parcial/total). Es análogo a cotizacion
// pero del lado de la COMPRA: maneja costos (no precios de venta) y su recepción
// ALIMENTA el ledger de inventario (append-only) con movimientos de entrada al
// costo de la orden, y deriva el asiento de compra (Inventario / Cuentas por
// pagar). La orden en sí es un documento con estado; el hecho económico —la
// entrada de stock— vive en el ledger inmutable, nunca en un contador editable.
package compra

// Estados de la máquina de una orden de compra.
const (
	OCBorrador        = "borrador"
	OCConfirmada      = "confirmada"
	OCRecibidaParcial = "recibida_parcial"
	OCRecibida        = "recibida"
	OCCancelada       = "cancelada"
)

// Linea es un renglón de la orden. CostoUnitario es el costo NETO (sin IVA): es
// lo que entra al Kardex cuando se recibe. CantidadRecibida es la proyección de
// lo ya recepcionado por este renglón; nunca puede exceder Cantidad.
type Linea struct {
	ProductoID       string  `json:"productoId" bson:"productoid"`
	SKU              string  `json:"sku" bson:"sku"`
	Nombre           string  `json:"nombre" bson:"nombre"`
	Cantidad         float64 `json:"cantidad" bson:"cantidad"`
	CostoUnitario    float64 `json:"costoUnitario" bson:"costounitario"`
	CantidadRecibida float64 `json:"cantidadRecibida" bson:"cantidadrecibida"`
	// Total es el NETO de la línea = cantidad * costoUnitario (sin IVA).
	Total float64 `json:"total" bson:"total"`
	// Exento marca la línea como no gravada con IVA crédito fiscal.
	Exento bool `json:"exento" bson:"exento"`
}

// OrdenCompra es un pedido de aprovisionamiento a un proveedor.
type OrdenCompra struct {
	ID              string `json:"id" bson:"id"`
	EmpresaID       string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID          string `json:"sedeId" bson:"sedeid"`       // sede que recibe
	ProveedorID     string `json:"proveedorId" bson:"proveedorid"`
	ProveedorNombre string `json:"proveedorNombre" bson:"proveedornombre"`

	Numero         int    `json:"numero" bson:"numero"`
	NumeroCompleto string `json:"numeroCompleto" bson:"numerocompleto"` // serie "OC" vía Numerador
	Serie          string `json:"serie" bson:"serie"`
	Estado         string `json:"estado" bson:"estado"`

	Lineas   []Linea `json:"lineas" bson:"lineas"`
	Subtotal float64 `json:"subtotal" bson:"subtotal"`
	IVA      float64 `json:"iva" bson:"iva"`
	Total    float64 `json:"total" bson:"total"`

	Moneda     string  `json:"moneda" bson:"moneda"`
	TasaCambio float64 `json:"tasaCambio" bson:"tasacambio"`
	TasaFuente string  `json:"tasaFuente" bson:"tasafuente"`

	CondicionesPago string `json:"condicionesPago" bson:"condicionespago"`
	Notas           string `json:"notas" bson:"notas"`

	Actor       string `json:"actor" bson:"actor"`
	Creada      string `json:"creada" bson:"creada"`           // UTC RFC3339
	Actualizada string `json:"actualizada" bson:"actualizada"` // UTC RFC3339
}

// Repository persiste órdenes de compra, aislado por empresaID. Igual que
// cotizacion: la orden cambia de estado (Update), pero los movimientos que su
// recepción emite van al ledger inmutable.
type Repository interface {
	List(empresaID string) []OrdenCompra
	ByID(empresaID, id string) (OrdenCompra, bool)
	Create(o OrdenCompra) OrdenCompra
	Update(o OrdenCompra) (OrdenCompra, bool)
}
