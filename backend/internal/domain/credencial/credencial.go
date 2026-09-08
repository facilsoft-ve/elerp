// Package credencial guarda la identidad local (email + contraseña) para el
// login nativo, sin depender de Hubmy. El hash nunca se serializa.
package credencial

// Credencial asocia un email con un hash bcrypt y el usuario local resultante.
type Credencial struct {
	Email     string `json:"email" bson:"email"`
	Hash      string `json:"-" bson:"hash"` // bcrypt; jamás se envía al cliente
	UsuarioID string `json:"usuarioId" bson:"usuarioid"`
	Nombre    string `json:"nombre" bson:"nombre"`
}

// Repository es el puerto de persistencia de credenciales locales.
type Repository interface {
	ByEmail(email string) (Credencial, bool)
	Create(c Credencial) Credencial
}
