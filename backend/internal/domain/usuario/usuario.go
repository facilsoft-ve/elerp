// Package usuario modela usuarios y sus membresías. Una Membresia vincula un
// usuario con una empresa (tenant) asignándole un rol y, opcionalmente, una
// sede fija (para Vendedor/Cajero).
package usuario

// Roles dentro de una empresa (ver doc de flujos y permisos).
const (
	RolDueno         = "dueno"         // Dueña / Admin de empresa
	RolVendedor      = "vendedor"      // una sede fija
	RolCajero        = "cajero"        // una caja, una sede
	RolContadora     = "contadora"     // total en Contabilidad/Tesorería, resto consulta
	RolDesarrollador = "desarrollador" // todo lo de Dueña + modo desarrollador
	RolMesonero      = "mesonero"      // módulo Restaurante: comandera en tablet, una sede
)

// RolValido valida un rol de empresa (el rol de plataforma super_admin no se
// asigna por acá).
func RolValido(r string) bool {
	switch r {
	case RolDueno, RolVendedor, RolCajero, RolContadora, RolDesarrollador, RolMesonero:
		return true
	}
	return false
}

// Estados de una membresía.
const (
	EstadoActiva  = "active"
	EstadoInvitada = "invited"
)

// Usuario es una persona con acceso a la plataforma.
type Usuario struct {
	ID     string `json:"id" bson:"id"`
	Nombre string `json:"nombre" bson:"nombre"`
	Email  string `json:"email" bson:"email"`
}

// Membresia es la relación usuario↔empresa con su rol y sede.
type Membresia struct {
	ID        string `json:"id" bson:"id"`
	UsuarioID string `json:"usuarioId" bson:"usuarioid"` // vacío mientras la invitación está pendiente
	Email     string `json:"email" bson:"email"`
	Nombre    string `json:"nombre" bson:"nombre"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Rol       string `json:"rol" bson:"rol"`
	SedeID    string `json:"sedeId" bson:"sedeid"` // opcional: fija la sede para roles de una sola sede
	Estado    string `json:"estado" bson:"estado"`
	Token     string `json:"-" bson:"token"` // token de invitación (nunca se serializa al cliente)
}

// UsuarioRepo es el puerto de persistencia de usuarios.
type UsuarioRepo interface {
	ByID(id string) (Usuario, bool)
	ByEmail(email string) (Usuario, bool)
	Create(u Usuario) Usuario
	Update(u Usuario) (Usuario, bool)
}

// MembresiaRepo es el puerto de persistencia de membresías.
type MembresiaRepo interface {
	// ByUsuario devuelve las membresías activas de un usuario.
	ByUsuario(usuarioID string) []Membresia
	// ByEmpresa devuelve las membresías de una empresa (tenant scope).
	ByEmpresa(empresaID string) []Membresia
	// ByEmail devuelve membresías (incluidas invitaciones pendientes) por email.
	ByEmail(email string) []Membresia
	ByToken(token string) (Membresia, bool)
	Create(m Membresia) Membresia
	Update(m Membresia) (Membresia, bool)
	Delete(id string) bool
}
