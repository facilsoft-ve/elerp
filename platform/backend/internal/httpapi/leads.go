package httpapi

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp-platform/internal/lead"
	"github.com/mornix/elerp-platform/internal/notify"
)

// --- Entrada pública (formulario de la web) --------------------------------

// leadEntradaPublica es lo que manda el formulario de elerp.tech. Es un DTO aparte a
// propósito: el visitante NO puede fijar estado, notas ni IDs.
type leadEntradaPublica struct {
	Nombre   string `json:"nombre"`
	Empresa  string `json:"empresa"`
	Email    string `json:"email"`
	Telefono string `json:"telefono"`
	Mensaje  string `json:"mensaje"`
	Giro     string `json:"giro"`
	// SitioWeb es un HONEYPOT: el formulario lo mantiene oculto y vacío; los bots que
	// rellenan todo lo llenan y se descartan sin decírselo.
	SitioWeb string `json:"sitioWeb"`
}

// handleCrearLeadPublico recibe una solicitud de demo SIN sesión. Es la única ruta
// pública de escritura del BFF, así que va con rate limit estricto, honeypot y un DTO
// acotado.
func (s *Server) handleCrearLeadPublico(c *fiber.Ctx) error {
	if !s.cfg.LeadsPublicoHabilitado {
		return fiber.NewError(fiber.StatusNotFound, "no disponible")
	}
	var in leadEntradaPublica
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo inválido")
	}
	// Honeypot lleno ⇒ bot. Se responde OK y no se guarda: contarle que lo detectamos
	// solo le enseña a evadirlo.
	if strings.TrimSpace(in.SitioWeb) != "" {
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	}

	l := lead.Lead{
		Nombre: in.Nombre, Empresa: in.Empresa, Email: in.Email,
		Telefono: in.Telefono, Mensaje: in.Mensaje, Giro: in.Giro,
		Origen: lead.OrigenWeb, Estado: lead.EstadoNuevo,
		IP: ipCliente(c), UserAgent: truncar(c.Get("User-Agent"), 300),
	}
	l.Normalizar()
	if err := l.Validar(); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	creado := s.leads.Create(l)
	s.avisarLeadNuevo(creado)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "id": creado.ID})
}

// --- Bandeja del equipo (requiere sesión de operador) ----------------------

// handleListarLeads devuelve las solicitudes, más nuevas primero. Filtro opcional por
// estado (?estado=nuevo).
func (s *Server) handleListarLeads(c *fiber.Ctx) error {
	items := s.leads.List()
	if e := c.Query("estado"); e != "" {
		filtrados := make([]lead.Lead, 0, len(items))
		for _, l := range items {
			if l.Estado == e {
				filtrados = append(filtrados, l)
			}
		}
		items = filtrados
	}
	return c.JSON(fiber.Map{"leads": items, "estados": lead.Estados, "giros": lead.Giros})
}

// leadParche son los campos que el equipo puede cambiar. Punteros para distinguir
// "no vino" de "vino vacío" (limpiar una nota es una operación legítima).
type leadParche struct {
	Estado        *string `json:"estado"`
	Notas         *string `json:"notas"`
	EmpresaDemoID *string `json:"empresaDemoId"`
}

func (s *Server) handleActualizarLead(c *fiber.Ctx) error {
	l, ok := s.leads.ByID(c.Params("id"))
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "solicitud no encontrada")
	}
	var p leadParche
	if err := c.BodyParser(&p); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo inválido")
	}
	if p.Estado != nil {
		if !lead.EstadoValido(*p.Estado) {
			return fiber.NewError(fiber.StatusBadRequest, lead.ErrEstadoInvalido.Error())
		}
		l.Estado = *p.Estado
	}
	if p.Notas != nil {
		l.Notas = *p.Notas
	}
	if p.EmpresaDemoID != nil {
		l.EmpresaDemoID = *p.EmpresaDemoID
	}
	l.Normalizar()
	if err := l.Validar(); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	actualizado, ok := s.leads.Update(l)
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "solicitud no encontrada")
	}
	return c.JSON(actualizado)
}

// --- Aviso al equipo -------------------------------------------------------

// avisarLeadNuevo manda el email en OTRA GOROUTINE: un SMTP lento no debe hacer esperar
// (ni fallar) al visitante que ya dejó sus datos. El lead ya está guardado; el email es
// una comodidad, no la fuente de verdad.
func (s *Server) avisarLeadNuevo(l lead.Lead) {
	if s.avisos == nil || !s.avisos.Configurado() {
		return
	}
	a := notify.Aviso{
		Asunto:  "Nueva solicitud de demo: " + primero(l.Empresa, l.Nombre),
		ReplyTo: l.Email,
		Cuerpo:  cuerpoAvisoLead(l),
	}
	go func() {
		if err := s.avisos.Enviar(a); err != nil {
			// Se registra y se sigue: el lead no se pierde por un fallo de correo.
			log.Printf("aviso de solicitud de demo %s: %v", l.ID, err)
		}
	}()
}

func cuerpoAvisoLead(l lead.Lead) string {
	campos := [][2]string{
		{"Nombre", l.Nombre},
		{"Empresa", l.Empresa},
		{"Email", l.Email},
		{"Teléfono", l.Telefono},
		{"Rubro", l.Giro},
		{"Origen", l.Origen},
		{"Recibida", l.Creado},
	}
	var b strings.Builder
	b.WriteString("Llegó una solicitud de demo desde la web.\n\n")
	for _, c := range campos {
		if c[1] != "" {
			b.WriteString(c[0] + ": " + c[1] + "\n")
		}
	}
	if l.Mensaje != "" {
		b.WriteString("\nQué le gustaría ver:\n" + l.Mensaje + "\n")
	}
	b.WriteString("\nGestionala en la consola: Solicitudes de demo.\n")
	return b.String()
}

// --- Utilidades ------------------------------------------------------------

// ipCliente resuelve la IP real detrás del proxy. Nginx setea X-Forwarded-For; el primer
// elemento es el cliente original.
func ipCliente(c *fiber.Ctx) string {
	if xff := c.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	return c.IP()
}

func truncar(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}

func primero(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return "sin nombre"
}
