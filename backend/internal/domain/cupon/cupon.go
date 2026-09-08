// Package cupon es el maestro de cupones de descuento de la empresa.
//
// Un cupón es un CÓDIGO con un descuento (porcentual o de monto fijo) que se
// aplica al cobrar o cotizar, si está activo, dentro de su vigencia, alcanza el
// monto mínimo de la compra y no agotó sus usos.
//
// Como el maestro de listas de precio o el de clientes, es EDITABLE (Update): no
// es un ledger append-only. El cupón NO recalcula impuestos: al aplicarlo, la
// capa de venta baja el precioUnitario de las líneas y el motor fiscal del
// servidor calcula el IVA sobre esa base ya descontada (regla de fiscal.go).
package cupon

import "strings"

// Tipos de descuento de un cupón.
const (
	TipoPorcentaje = "porcentaje" // Valor es un % (0..100) que baja cada precio
	TipoMonto      = "monto"      // Valor es un monto fijo en Bs repartido entre las líneas
)

// TipoValido indica si el tipo de cupón es uno de los admitidos.
func TipoValido(t string) bool { return t == TipoPorcentaje || t == TipoMonto }

// NormalizarCodigo deja el código en MAYÚSCULAS y sin espacios a los lados: el
// código es único por empresa y se compara así (case-insensitive por diseño).
func NormalizarCodigo(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// Cupon es un código de descuento de la empresa.
type Cupon struct {
	ID           string  `json:"id" bson:"id"`
	EmpresaID    string  `json:"empresaId" bson:"empresaid"`
	Codigo       string  `json:"codigo" bson:"codigo"` // único por empresa, en MAYÚSCULAS
	Descripcion  string  `json:"descripcion" bson:"descripcion"`
	Tipo         string  `json:"tipo" bson:"tipo"`               // TipoPorcentaje | TipoMonto
	Valor        float64 `json:"valor" bson:"valor"`             // % (0..100) o monto fijo en Bs
	MontoMinimo  float64 `json:"montoMinimo" bson:"montominimo"` // subtotal mínimo para aplicar (0 = sin mínimo)
	Desde        string  `json:"desde" bson:"desde"`             // YYYY-MM-DD, vacío = sin límite inferior
	Hasta        string  `json:"hasta" bson:"hasta"`             // YYYY-MM-DD, vacío = sin límite superior
	Activo       bool    `json:"activo" bson:"activo"`           // desactivar sin borrar
	UsosMax      int     `json:"usosMax" bson:"usosmax"`         // 0 = ilimitado
	UsosActuales int     `json:"usosActuales" bson:"usosactuales"`
}

// Repository persiste cupones, aislado por empresaID.
type Repository interface {
	List(empresaID string) []Cupon
	ByID(empresaID, id string) (Cupon, bool)
	// ByCodigo busca por código normalizado (MAYÚSCULAS); es la consulta que usa
	// la validación al aplicar y el control de unicidad al crear/editar.
	ByCodigo(empresaID, codigo string) (Cupon, bool)
	Create(c Cupon) Cupon
	// Update reemplaza un cupón existente del tenant (maestro editable, no ledger).
	// Devuelve false si el id no pertenece a la empresa.
	Update(c Cupon) (Cupon, bool)
}
