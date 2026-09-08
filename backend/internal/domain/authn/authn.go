// Package authn define el principal autenticado, la sesión y el puerto de
// almacenamiento de sesiones. No depende de ningún adaptador.
package authn

import "time"

// Principal es la identidad autenticada (venga de Hubmy o de credencial local).
type Principal struct {
	UserID string `json:"userId"`
	Nombre string `json:"nombre"`
	Email  string `json:"email"`
	// SuperAdmin marca al personal de plataforma (Mornix). Nunca opera datos de
	// negocio de un cliente; accede solo al panel /admin.
	SuperAdmin bool `json:"superAdmin"`
}

// Session es una sesión emitida tras autenticar.
type Session struct {
	ID         string    `json:"id"`
	Principal  Principal `json:"principal"`
	HubmyToken string    `json:"-"` // token opaco de Hubmy, si el login fue por SSO
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// Expired indica si la sesión ya venció.
func (s Session) Expired(now time.Time) bool { return now.After(s.ExpiresAt) }

// Store es el puerto de persistencia de sesiones (in-memory o Mongo).
type Store interface {
	Create(p Principal, hubmyToken string, ttl time.Duration) Session
	Get(id string) (Session, bool)
	Delete(id string)
	// DeleteByUser invalida todas las sesiones de un usuario (p. ej. al cambiar
	// contraseña o detectar actividad sospechosa — §13.1 del doc de seguridad).
	DeleteByUser(userID string)
}
