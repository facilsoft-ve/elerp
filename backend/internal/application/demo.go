package application

import (
	"errors"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/demolead"
)

// Identidad demo usada por el modo DEV_LOGIN y por el seed de datos. Mantener
// en sync: el seed crea el usuario y la membresía de este principal para que
// "Entrar en modo demo" tenga una empresa con datos al iniciar sesión.
const (
	DemoUserID = "usr_demo"
	DemoNombre = "María Fernández"
	DemoEmail  = "maria@bodegalacima.com"
)

// ErrLeadsNoDisponible se devuelve si el almacén de leads no fue cableado.
var ErrLeadsNoDisponible = errors.New("el registro de solicitudes de demo no está disponible")

// DemoLeadInput son los datos que el prospecto envía antes de entrar a la demo.
type DemoLeadInput struct {
	Nombre   string
	Empresa  string
	Email    string
	Telefono string
	Mensaje  string
}

// ConLeadsDemo cablea el almacén de leads de demo. Se configura aparte de New
// (como ConFuentesDeTasa) porque es infraestructura opcional de captación,
// previa al login, no un agregado de negocio con tenant.
func (s *Service) ConLeadsDemo(r demolead.Repository) *Service {
	s.demoLeads = r
	return s
}

// RegistrarLeadDemo persiste la solicitud de un prospecto que quiere probar la
// demostración. Valida el correo (requerido) y anexa el lead con su fecha UTC.
// No envía correo ni notifica a nadie: la notificación al equipo queda como un
// punto de integración claro (webhook/email de Hubmy) sobre este mismo registro.
func (s *Service) RegistrarLeadDemo(in DemoLeadInput, origen string) (demolead.DemoLead, error) {
	email := strings.TrimSpace(in.Email)
	if email == "" || !emailValido(email) {
		return demolead.DemoLead{}, ErrEmailInvalido
	}
	if s.demoLeads == nil {
		return demolead.DemoLead{}, ErrLeadsNoDisponible
	}
	l := demolead.DemoLead{
		Nombre:   strings.TrimSpace(in.Nombre),
		Empresa:  strings.TrimSpace(in.Empresa),
		Email:    email,
		Telefono: strings.TrimSpace(in.Telefono),
		Mensaje:  strings.TrimSpace(in.Mensaje),
		CreadoEn: time.Now().UTC().Format(time.RFC3339),
		Origen:   origen,
	}
	return s.demoLeads.Append(l), nil
}
