// Package proveedor es el maestro de terceros vendedores de la empresa: el
// espejo de cliente pero del lado de las Compras. Un proveedor identifica a
// quien surte mercancía (RIF, contacto) y se DESACTIVA en vez de borrarse, para
// no perder el rastro de las órdenes de compra que lo referencian.
package proveedor

// Proveedor es un tercero que surte mercancía o servicios a la empresa.
type Proveedor struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	Nombre    string `json:"nombre" bson:"nombre"`
	// Documento es el RIF (o cédula) del proveedor. No se exige único: un mismo
	// RIF puede reaparecer si se dio de baja y se vuelve a registrar.
	Documento string `json:"documento" bson:"documento"`
	Email     string `json:"email" bson:"email"`
	Telefono  string `json:"telefono" bson:"telefono"`
	Direccion string `json:"direccion" bson:"direccion"`
	// Activo permite la baja REVERSIBLE (nunca se borra): un proveedor inactivo
	// no aparece para nuevas órdenes pero sigue enlazado a su histórico.
	Activo bool   `json:"activo" bson:"activo"`
	Creado string `json:"creado" bson:"creado"` // UTC RFC3339
}

// Repository persiste proveedores, aislado por empresaID. Igual que
// cliente.Repository, con Update para la edición y la baja reversible.
type Repository interface {
	List(empresaID string) []Proveedor
	ByID(empresaID, id string) (Proveedor, bool)
	Create(p Proveedor) Proveedor
	Update(p Proveedor) (Proveedor, bool)
}
