package httpapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerAuditoria monta el Registro de actividad (bitácora) del TENANT: la
// trazabilidad transversal de quién hizo qué, cuándo y desde dónde. Es de solo
// lectura y la ven quienes gobiernan la empresa (Dueña, Contadora, Desarrollador):
// el SAD la llama "actividad del propio tenant, transparencia hacia el cliente".
func (s *Server) registerAuditoria(r fiber.Router) {
	r.Get("/auditoria", s.requireRoles(usuario.RolDueno, usuario.RolContadora, usuario.RolDesarrollador), s.handleAuditoria)
}

// handleAuditoria devuelve el registro de actividad de la empresa, filtrable por
// rango de fechas (?desde&hasta, RFC3339 o YYYY-MM-DD) y por prefijo de acción
// (?accion, p. ej. "fiscal" o "contabilidad.cuenta"), acotado por ?limite. Enriquece
// cada evento con el NOMBRE del actor (resuelto contra los miembros de la empresa).
func (s *Server) handleAuditoria(c *fiber.Ctx) error {
	empresaID := empresaIDOf(c)
	eventos := s.svc.AuditoriaEntre(empresaID, c.Query("desde"), c.Query("hasta"), 0)
	accion := c.Query("accion")
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	limite := c.QueryInt("limite", 500)

	nombres := map[string]string{}
	for _, m := range s.tenancy.Miembros(empresaID) {
		if m.UsuarioID != "" {
			nombres[m.UsuarioID] = m.Nombre
		}
	}

	type eventoView struct {
		auditoria.Evento
		ActorNombre string `json:"actorNombre"`
	}
	out := make([]eventoView, 0, len(eventos))
	for _, e := range eventos {
		if accion != "" && !strings.HasPrefix(e.Accion, accion) {
			continue
		}
		actorNombre := nombreActor(nombres, e.Actor)
		// Búsqueda libre: coincide en actor (id o nombre), acción, entidad, detalle u origen.
		if q != "" && !strings.Contains(strings.ToLower(actorNombre+" "+e.Actor+" "+e.Accion+" "+e.Entidad+" "+e.Detalle+" "+e.Origen), q) {
			continue
		}
		out = append(out, eventoView{Evento: e, ActorNombre: actorNombre})
		if len(out) >= limite {
			break
		}
	}
	return c.JSON(out)
}

// nombreActor resuelve el actor a un nombre mostrable: el del miembro si existe, o
// una etiqueta legible para los actores del sistema.
func nombreActor(nombres map[string]string, actor string) string {
	if n, ok := nombres[actor]; ok && n != "" {
		return n
	}
	switch actor {
	case "sistema":
		return "Sistema"
	case "":
		return "—"
	}
	return actor
}
