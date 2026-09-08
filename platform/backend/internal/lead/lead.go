// Package lead modela las SOLICITUDES DE DEMO que llegan del formulario de la web
// pública (y las que carga el equipo a mano). Son prospectos de Mornix, no datos de un
// tenant: por eso viven en la base de la consola de plataforma y no en el core ERP.
package lead

import (
	"errors"
	"regexp"
	"strings"
)

// Estados del embudo. El equipo los mueve a mano desde la consola.
const (
	EstadoNuevo      = "nuevo"       // recién llegado del formulario
	EstadoContactado = "contactado"  // alguien del equipo ya escribió
	EstadoDemoCreada = "demo_creada" // se le abrió una demo/sandbox
	EstadoGanado     = "ganado"      // se convirtió en cliente
	EstadoDescartado = "descartado"  // no califica o no respondió
)

// Estados es el orden en que la consola los muestra.
var Estados = []string{EstadoNuevo, EstadoContactado, EstadoDemoCreada, EstadoGanado, EstadoDescartado}

// Giros con demo precargada. Coincide con empresa.Giro del core y con el selector del
// formulario de la web: es lo que decide QUÉ demo se le abre al prospecto.
const (
	GiroBodega      = "bodega"
	GiroFerreteria  = "ferreteria"
	GiroFarmacia    = "farmacia"
	GiroRestaurante = "restaurante"
	GiroOtro        = "otro"
)

// Giros son los valores aceptados (vacío también se acepta: el visitante puede no elegir).
var Giros = []string{GiroBodega, GiroFerreteria, GiroFarmacia, GiroRestaurante, GiroOtro}

// Origen de la solicitud.
const (
	OrigenWeb     = "web"     // formulario de elerp.tech
	OrigenConsola = "consola" // cargado a mano por el equipo
)

// Límites de longitud. El endpoint es PÚBLICO: acotar el tamaño es parte de la defensa,
// no una preferencia de UI.
const (
	MaxNombre   = 120
	MaxEmpresa  = 160
	MaxEmail    = 254 // RFC 5321
	MaxTelefono = 40
	MaxMensaje  = 2000
	MaxNotas    = 4000
)

// Lead es una solicitud de demo.
type Lead struct {
	ID       string `json:"id" bson:"id"`
	Nombre   string `json:"nombre" bson:"nombre"`
	Empresa  string `json:"empresa" bson:"empresa"`
	Email    string `json:"email" bson:"email"`
	Telefono string `json:"telefono" bson:"telefono"`
	Mensaje  string `json:"mensaje" bson:"mensaje"`
	Giro     string `json:"giro" bson:"giro"`

	Origen    string `json:"origen" bson:"origen"`
	IP        string `json:"ip" bson:"ip"`
	UserAgent string `json:"userAgent" bson:"useragent"`

	Estado string `json:"estado" bson:"estado"`
	Notas  string `json:"notas" bson:"notas"`
	// EmpresaDemoID apunta a la empresa/sandbox que se le abrió (si ya se hizo).
	EmpresaDemoID string `json:"empresaDemoId" bson:"empresademoid"`

	Creado      string `json:"creado" bson:"creado"`           // RFC3339 UTC
	Actualizado string `json:"actualizado" bson:"actualizado"` // RFC3339 UTC
}

// Repository es el puerto de persistencia.
type Repository interface {
	List() []Lead
	ByID(id string) (Lead, bool)
	Create(l Lead) Lead
	Update(l Lead) (Lead, bool)
}

// Errores de validación.
var (
	ErrNombreRequerido = errors.New("el nombre es obligatorio")
	ErrEmailRequerido  = errors.New("el email es obligatorio")
	ErrEmailInvalido   = errors.New("el email no tiene un formato válido")
	ErrGiroInvalido    = errors.New("el rubro indicado no es válido")
	ErrEstadoInvalido  = errors.New("el estado indicado no es válido")
)

// Deliberadamente laxo: valida la FORMA, no la existencia del buzón. Rechazar direcciones
// raras pero legítimas pierde clientes; el email real se confirma al contactar.
var reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)

// EstadoValido indica si e es un estado del embudo.
func EstadoValido(e string) bool { return contiene(Estados, e) }

// GiroValido indica si g es un rubro conocido (el vacío es válido: "no lo dijo").
func GiroValido(g string) bool { return g == "" || contiene(Giros, g) }

func contiene(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Normalizar recorta espacios, baja el email a minúsculas y TRUNCA cada campo a su
// máximo. Truncar (en vez de rechazar) es a propósito: un mensaje largo no debe costar
// el lead.
func (l *Lead) Normalizar() {
	l.Nombre = recortar(l.Nombre, MaxNombre)
	l.Empresa = recortar(l.Empresa, MaxEmpresa)
	l.Email = strings.ToLower(recortar(l.Email, MaxEmail))
	l.Telefono = recortar(l.Telefono, MaxTelefono)
	l.Mensaje = recortar(l.Mensaje, MaxMensaje)
	l.Notas = recortar(l.Notas, MaxNotas)
	l.Giro = strings.ToLower(strings.TrimSpace(l.Giro))
	if l.Estado == "" {
		l.Estado = EstadoNuevo
	}
}

// Validar exige lo mínimo para poder responderle al prospecto: nombre y email.
func (l Lead) Validar() error {
	if l.Nombre == "" {
		return ErrNombreRequerido
	}
	if l.Email == "" {
		return ErrEmailRequerido
	}
	if !reEmail.MatchString(l.Email) {
		return ErrEmailInvalido
	}
	if !GiroValido(l.Giro) {
		return ErrGiroInvalido
	}
	if !EstadoValido(l.Estado) {
		return ErrEstadoInvalido
	}
	return nil
}

// recortar trima y limita a n RUNAS (no bytes: cortar bytes parte los acentos a la mitad).
func recortar(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
