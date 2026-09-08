// Package almacen es el maestro de almacenes (depósitos) de la empresa. Un almacén
// pertenece a una SEDE: una sede puede tener varios almacenes y DEBE tener al menos
// uno. El inventario se ubica por almacén (el ledger de movimientos llevará su ID);
// esta entidad es el catálogo editable de esos almacenes.
//
// Como el maestro de unidades de medida, es EDITABLE (Update), no un ledger
// append-only: no hay borrado duro; un almacén se DESACTIVA (Activo=false) para
// conservar el histórico y los movimientos que ya lo referencian.
package almacen

import "strings"

// Tipos de almacén admitidos. El tipo es OPCIONAL: vacío ⇒ "general".
const (
	TipoPrincipal    = "principal"
	TipoGeneral      = "general"
	TipoTransito     = "transito"
	TipoDevoluciones = "devoluciones"
	TipoMateriaPrima = "materia_prima"
	TipoCuarentena   = "cuarentena"
	TipoRefrigerado  = "refrigerado"
)

// TipoValido indica si el tipo es uno de los admitidos.
func TipoValido(t string) bool {
	switch t {
	case TipoPrincipal, TipoGeneral, TipoTransito, TipoDevoluciones,
		TipoMateriaPrima, TipoCuarentena, TipoRefrigerado:
		return true
	}
	return false
}

// NormalizarTipo deja el tipo en minúsculas y sin espacios (así "Refrigerado" pasa).
func NormalizarTipo(t string) string { return strings.ToLower(strings.TrimSpace(t)) }

// Almacen es un depósito físico dentro de una sede.
type Almacen struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	Nombre    string `json:"nombre" bson:"nombre"`
	Tipo      string `json:"tipo" bson:"tipo"`           // principal|general|transito|... (vacío ⇒ general)
	Principal bool   `json:"principal" bson:"principal"` // exactamente uno por sede: de él despacha el POS
	Activo    bool   `json:"activo" bson:"activo"`       // desactivar sin borrar
	// Capacidad opcional: límite físico del almacén, expresado en CapacidadUnidad
	// (un símbolo del maestro de unidades, típicamente de peso o volumen). 0 = sin
	// límite. Es un límite BLANDO: se muestra como barra de llenado, no bloquea.
	Capacidad       float64 `json:"capacidad" bson:"capacidad"`
	CapacidadUnidad string  `json:"capacidadUnidad" bson:"capacidadunidad"`
	// RubrosAdmitidos restringe qué productos ACEPTA el almacén (por rubro). Vacío =
	// admite todos. Restricción DURA: no se puede ingresar stock de un rubro ajeno.
	RubrosAdmitidos []string `json:"rubrosAdmitidos" bson:"rubrosadmitidos"`
	Creada          string   `json:"creada" bson:"creada"` // RFC3339
}

// AdmiteRubro indica si el almacén acepta productos de ese rubro. Sin restricción
// (lista vacía) admite todo.
func (a Almacen) AdmiteRubro(rubroID string) bool {
	if len(a.RubrosAdmitidos) == 0 {
		return true
	}
	for _, r := range a.RubrosAdmitidos {
		if r == rubroID {
			return true
		}
	}
	return false
}

// Repository persiste almacenes, aislado por empresaID (el tenant). Sin borrado
// duro: desactivar es un Update con Activo=false.
type Repository interface {
	List(empresaID string) []Almacen
	ByID(empresaID, id string) (Almacen, bool)
	Create(a Almacen) Almacen
	Update(a Almacen) (Almacen, bool)
}
