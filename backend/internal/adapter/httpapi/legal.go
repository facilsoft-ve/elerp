package httpapi

import (
	"github.com/gofiber/fiber/v2"
)

// Documentos legales (Términos, Privacidad): versión vigente pública, aceptación
// auditable por usuario y consulta de pendientes/historial. La versión y el hash los fija
// el servidor; el cliente nunca los decide.

// handleLegalVigente (PÚBLICO) devuelve las versiones vigentes con su contenido, para
// mostrarlas sin autenticación (titulares y terceros deben poder consultarlas).
func (s *Server) handleLegalVigente(c *fiber.Ctx) error {
	vs := s.svc.VersionesLegalesVigentes()
	type docOut struct {
		Documento string `json:"documento"`
		Version   string `json:"version"`
		Hash      string `json:"hash"`
		Titulo    string `json:"titulo"`
		Contenido string `json:"contenido"`
	}
	out := make([]docOut, 0, len(vs))
	for _, v := range vs {
		contenido, _ := s.svc.ContenidoLegal(v.Documento)
		out = append(out, docOut{v.Documento, v.Version, v.Hash, v.Titulo, contenido})
	}
	return c.JSON(fiber.Map{"documentos": out})
}

// handleLegalEstado (AUTENTICADO) devuelve las versiones que el usuario AÚN no aceptó.
func (s *Server) handleLegalEstado(c *fiber.Ctx) error {
	p := principalOf(c)
	return c.JSON(fiber.Map{"pendientes": s.svc.LegalPendiente(p.UserID)})
}

type aceptarLegalBody struct {
	Documento string `json:"documento"`
}

// handleLegalAceptar (AUTENTICADO) registra la aceptación de un documento por el usuario.
// Captura la prueba: hash del contenido, IP, user-agent, fecha UTC, en la auditoría.
func (s *Server) handleLegalAceptar(c *fiber.Ctx) error {
	var in aceptarLegalBody
	if err := c.BodyParser(&in); err != nil || in.Documento == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "falta el documento"})
	}
	p := principalOf(c)
	// Contexto opcional: si la petición trae empresa activa, se anota (y el rol).
	empID := c.Get("X-Empresa-ID")
	rol := ""
	if empID != "" {
		if _, r, _, ok := s.tenancy.RolEnEmpresa(p.UserID, empID); ok {
			rol = r
		}
	}
	if _, err := s.svc.AceptarLegal(p.UserID, empID, rol, in.Documento, c.IP(), c.Get("User-Agent"), ""); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"pendientes": s.svc.LegalPendiente(p.UserID)})
}

// handleLegalHistorial (AUTENTICADO) devuelve el historial de aceptaciones del usuario
// (evidencia exportable).
func (s *Server) handleLegalHistorial(c *fiber.Ctx) error {
	p := principalOf(c)
	return c.JSON(fiber.Map{"aceptaciones": s.svc.AceptacionesDe(p.UserID)})
}
