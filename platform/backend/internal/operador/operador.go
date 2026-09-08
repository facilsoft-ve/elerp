// Package operador es el dominio de la consola de plataforma: los operadores de Mornix
// (super usuarios) y sus sesiones. Es intencionalmente independiente del core ERP: la
// consola tiene sus PROPIAS credenciales y su propia base de datos.
package operador

import "time"

// Operador es un super usuario de Mornix que opera la consola. Autentica con
// email+contraseña (bcrypt) y un segundo factor TOTP obligatorio.
type Operador struct {
	ID     string `json:"id" bson:"id"`
	Email  string `json:"email" bson:"email"`
	Nombre string `json:"nombre" bson:"nombre"`
	// Hash es el bcrypt de la contraseña; nunca se serializa a JSON.
	Hash string `json:"-" bson:"hash"`
	// TOTPSecret es el secreto base32 del segundo factor; nunca se serializa a JSON.
	TOTPSecret string `json:"-" bson:"totpsecret"`
	// MFAConfigurada indica si ya enroló su autenticador. Mientras sea false, el primer
	// login exige enrolarlo antes de emitir sesión.
	MFAConfigurada bool   `json:"mfaConfigurada" bson:"mfaconfigurada"`
	Creado         string `json:"creado" bson:"creado"`
}

// Repository persiste operadores. La consola no tiene multi-tenancy: es el staff Mornix.
type Repository interface {
	ByEmail(email string) (Operador, bool)
	ByID(id string) (Operador, bool)
	Create(o Operador) Operador
	Update(o Operador) (Operador, bool)
	Count() int
}

// Session es una sesión autenticada de un operador (cookie opaca).
type Session struct {
	ID         string    `json:"id" bson:"id"`
	OperadorID string    `json:"operadorId" bson:"operadorid"`
	Email      string    `json:"email" bson:"email"`
	Nombre     string    `json:"nombre" bson:"nombre"`
	CreatedAt  time.Time `json:"createdAt" bson:"createdat"`
	ExpiresAt  time.Time `json:"expiresAt" bson:"expiresat"`
}

// Expired indica si la sesión ya venció.
func (s Session) Expired(now time.Time) bool { return now.After(s.ExpiresAt) }

// SessionStore persiste sesiones de operador.
type SessionStore interface {
	Create(o Operador, ttl time.Duration) Session
	Get(id string) (Session, bool)
	Delete(id string)
}
