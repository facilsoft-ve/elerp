// Package auditoria registra, de forma solo-anexada, toda acción sensible
// (quién, qué, cuándo, desde dónde) — §13.3 del doc de seguridad. El historial
// es de solo lectura incluso para el rol Desarrollador.
package auditoria

// Evento es una entrada inmutable del registro de auditoría.
type Evento struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Actor     string `json:"actor" bson:"actor"`   // userID que ejecutó
	Rol       string `json:"rol" bson:"rol"`
	Accion    string `json:"accion" bson:"accion"` // p.ej. inventario.ajuste, transferencia.estado
	Entidad   string `json:"entidad" bson:"entidad"`
	Detalle   string `json:"detalle" bson:"detalle"`
	Origen    string `json:"origen" bson:"origen"` // IP / dispositivo
	Fecha     string `json:"fecha" bson:"fecha"`   // UTC RFC3339
}

// Repository es el puerto de auditoría: solo append y lectura, nunca update/delete.
type Repository interface {
	Append(e Evento) Evento
	List(empresaID string) []Evento
}
