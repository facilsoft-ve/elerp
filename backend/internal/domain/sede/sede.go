// Package sede modela la Sede/tienda: el tercer nivel de la jerarquía. Las
// existencias de inventario y las cajas viven a nivel de sede.
package sede

// Sede es un local físico de una empresa.
type Sede struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Nombre    string `json:"nombre" bson:"nombre"`
	Direccion string `json:"direccion" bson:"direccion"`
	Activa    bool   `json:"activa" bson:"activa"`
}

// Repository es el puerto de persistencia de sedes. Toda consulta se aísla por
// empresaID (el tenant).
type Repository interface {
	List(empresaID string) []Sede
	ByID(empresaID, id string) (Sede, bool)
	Create(s Sede) Sede
	Update(s Sede) (Sede, bool)
}
