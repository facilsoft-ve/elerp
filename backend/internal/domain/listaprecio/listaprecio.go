// Package listaprecio es el maestro de listas/tarifas de precio de la empresa.
//
// Una lista de precio fija un precio EXPLÍCITO por producto (por SKU) que
// reemplaza al precio base del catálogo para ese producto; los productos que la
// lista no menciona conservan su precio base. Hay listas de VENTA (tarifas por
// cliente, canal o volumen) y de COMPRA (costos negociados por proveedor).
//
// A diferencia de los documentos fiscales (append-only), este es un maestro
// EDITABLE (Update), como el maestro de clientes: corregir un precio de una
// lista no reescribe historia contable; los documentos ya emitidos conservan el
// precio con el que se emitieron.
package listaprecio

// Tipos de lista de precio.
const (
	TipoVenta  = "venta"  // tarifa de venta (precio al cliente)
	TipoCompra = "compra" // tarifa de compra (costo del proveedor)
)

// TipoValido indica si el tipo de lista es uno de los admitidos.
func TipoValido(t string) bool { return t == TipoVenta || t == TipoCompra }

// ItemLista es el precio explícito de un producto dentro de una lista. El precio
// está expresado en la moneda de la lista (ListaPrecio.Moneda).
type ItemLista struct {
	SKU    string  `json:"sku" bson:"sku"`
	Precio float64 `json:"precio" bson:"precio"`
}

// ListaPrecio es una tarifa de la empresa: un conjunto de precios por SKU con un
// nombre, un tipo (venta/compra), una moneda y un interruptor de actividad.
type ListaPrecio struct {
	ID        string      `json:"id" bson:"id"`
	EmpresaID string      `json:"empresaId" bson:"empresaid"`
	Nombre    string      `json:"nombre" bson:"nombre"`
	Tipo      string      `json:"tipo" bson:"tipo"`     // TipoVenta | TipoCompra
	Activa    bool        `json:"activa" bson:"activa"` // desactivar sin borrar
	Moneda    string      `json:"moneda" bson:"moneda"` // moneda de los precios (VES, USD, …)
	Items     []ItemLista `json:"items" bson:"items"`   // precio por SKU
}

// Repository persiste listas de precio, aislado por empresaID.
type Repository interface {
	List(empresaID string) []ListaPrecio
	ByID(empresaID, id string) (ListaPrecio, bool)
	Create(l ListaPrecio) ListaPrecio
	// Update reemplaza una lista existente del tenant (maestro editable, no
	// ledger). Devuelve false si el id no pertenece a la empresa.
	Update(l ListaPrecio) (ListaPrecio, bool)
}
