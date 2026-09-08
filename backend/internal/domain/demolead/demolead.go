// Package demolead guarda los datos de un posible cliente (lead) que pide
// probar la demostración de ElERP antes de entrar. Es un registro previo al
// login: NO pertenece a ningún tenant (empresa), así que su repositorio es
// global, sin filtro por empresaID —a diferencia de los agregados de negocio—.
//
// El almacén es de solo-anexado: cada solicitud de demo deja su propia fila con
// la fecha UTC en que se registró. No se edita ni se borra.
package demolead

// DemoLead es la solicitud de un prospecto para probar la demostración.
type DemoLead struct {
	ID       string `json:"id" bson:"id"`
	Nombre   string `json:"nombre" bson:"nombre"`
	Empresa  string `json:"empresa" bson:"empresa"`   // empresa/negocio del prospecto
	Email    string `json:"email" bson:"email"`       // requerido, validado
	Telefono string `json:"telefono" bson:"telefono"` // opcional
	Mensaje  string `json:"mensaje" bson:"mensaje"`   // «¿qué te gustaría resolver?» (opcional)
	// CreadoEn es la fecha UTC (RFC3339) en que llegó la solicitud.
	CreadoEn string `json:"creadoEn" bson:"creadoen"`
	// Origen es una etiqueta de procedencia (IP) para trazabilidad básica.
	Origen string `json:"origen" bson:"origen"`
}

// Repository persiste los leads de demo. Es global (sin tenant): estos registros
// nacen antes de que exista una empresa activa.
type Repository interface {
	// Append anexa un lead nuevo (solo-anexado) y lo devuelve con su ID.
	Append(l DemoLead) DemoLead
	// List devuelve todos los leads registrados (para consulta del equipo).
	List() []DemoLead
}
