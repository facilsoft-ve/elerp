// Package legal modela la aceptación de los documentos legales (Términos y Condiciones,
// Política de Privacidad). La aceptación se registra además como evento append-only en la
// auditoría (prueba inmutable); esta proyección permite responder en O(1) si un usuario ya
// aceptó la versión vigente, para exigir re-aceptación cuando cambie.
package legal

// Identificadores de documento.
const (
	DocTerminos   = "terminos"
	DocPrivacidad = "privacidad"
)

// Métodos de aceptación (para la traza).
const (
	MetodoOnboarding   = "onboarding"
	MetodoReaceptacion = "reaceptacion"
)

// Version es una versión vigente de un documento legal.
type Version struct {
	Documento    string `json:"documento"`
	Version      string `json:"version"`
	Hash         string `json:"hash"` // SHA-256 del contenido publicado (fija lo aceptado)
	Titulo       string `json:"titulo"`
	VigenteDesde string `json:"vigenteDesde,omitempty"`
}

// Aceptacion es el registro de que un usuario aceptó un documento en una versión.
type Aceptacion struct {
	UserID    string `json:"userId" bson:"userid"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Rol       string `json:"rol" bson:"rol"`
	Documento string `json:"documento" bson:"documento"`
	Version   string `json:"version" bson:"version"`
	Hash      string `json:"hash" bson:"hash"`
	Fecha     string `json:"fecha" bson:"fecha"`   // UTC RFC3339
	Origen    string `json:"origen" bson:"origen"` // IP
	UserAgent string `json:"userAgent" bson:"useragent"`
	Metodo    string `json:"metodo" bson:"metodo"`
}

// Repository es la proyección de última aceptación por usuario+documento. La fuente de
// verdad es la auditoría append-only; esto solo acelera la consulta de "¿ya aceptó?".
type Repository interface {
	Ultima(userID, documento string) (Aceptacion, bool)
	Registrar(a Aceptacion) Aceptacion
	// PorUsuario devuelve todas las aceptaciones de un usuario (para exportar evidencia).
	PorUsuario(userID string) []Aceptacion
}
