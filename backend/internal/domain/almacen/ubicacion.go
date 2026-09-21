package almacen

import "strings"

// UBICACIÓN: el nivel que falta entre el almacén y la unidad.
//
// POR QUÉ EXISTE. El almacén dice en qué depósito está la mercancía; la ubicación,
// en qué parte de ese depósito. En un almacén pequeño da igual, pero en uno grande
// es la diferencia entre saber que hay 40 unidades y saber dónde están — y quien
// las busca pierde el tiempo igual que si no estuvieran.
//
// Y ES LA BASE DE LOS PROCESOS EN PASOS: un muelle de recepción es una ubicación, y
// una zona de preparación también. Sin este nivel, «recibido pero todavía no
// ubicado» no se puede expresar.
//
// PLANA, NO JERÁRQUICA. Una ubicación pertenece a un almacén y punto: no hay
// ubicaciones dentro de ubicaciones ni ubicaciones virtuales (proveedor, cliente,
// producción) como en Odoo. Esas convertirían cada movimiento en un par
// origen→destino, que es otro modelo de datos y no un nivel más.
//
// Editable como el almacén, no un ledger: no hay borrado duro, se desactiva para
// conservar los movimientos que ya la referencian.
type Ubicacion struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	AlmacenID string `json:"almacenId" bson:"almacenid"`
	// Codigo es la referencia corta que se lee en el anaquel y se teclea al ubicar
	// («A-03-B»). Único dentro de su almacén: dos ubicaciones con el mismo código
	// hacen que quien la busca no sepa a cuál ir.
	Codigo string `json:"codigo" bson:"codigo"`
	Nombre string `json:"nombre" bson:"nombre"` // "Pasillo 3, estante B"
	Tipo   string `json:"tipo" bson:"tipo"`     // vacío ⇒ almacenamiento
	Activa bool   `json:"activa" bson:"activa"`
	Creada string `json:"creada" bson:"creada"` // RFC3339
}

// Tipos de ubicación. El tipo no restringe nada por sí solo: dice PARA QUÉ sirve,
// y de ahí lo toman los procesos en pasos (el muelle es donde entra lo que aún no
// se ha revisado, la preparación donde espera lo ya vendido).
const (
	// UbicAlmacenamiento es el caso normal: un sitio donde la mercancía reposa.
	UbicAlmacenamiento = "almacenamiento"
	// UbicMuelle recibe lo que acaba de llegar y todavía no se ha ubicado.
	UbicMuelle = "muelle"
	// UbicPreparacion guarda lo que ya está comprometido y espera despacho.
	UbicPreparacion = "preparacion"
)

// TipoUbicacionValido acota el tipo. Vacío es válido y se lee como almacenamiento,
// así que una ubicación cargada sin tipo no obliga a migrar nada.
func TipoUbicacionValido(t string) bool {
	switch t {
	case "", UbicAlmacenamiento, UbicMuelle, UbicPreparacion:
		return true
	}
	return false
}

// NormalizarCodigo deja el código en mayúsculas y sin espacios alrededor: se teclea
// a mano desde un anaquel y "a-03-b" tiene que encontrar la misma ubicación que
// "A-03-B".
func NormalizarCodigo(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// UbicacionRepo persiste ubicaciones, aislado por empresaID. Sin borrado duro:
// desactivar es un Update con Activa=false.
type UbicacionRepo interface {
	List(empresaID string) []Ubicacion
	ByID(empresaID, id string) (Ubicacion, bool)
	Create(u Ubicacion) Ubicacion
	Update(u Ubicacion) (Ubicacion, bool)
}
