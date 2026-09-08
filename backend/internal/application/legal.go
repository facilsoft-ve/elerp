package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/legal"
	"github.com/mornix/elerp/internal/legaldocs"
)

// Versiones vigentes de los documentos legales. Al publicar una versión nueva se cambia
// aquí (y se reemplaza el contenido en internal/legaldocs): los usuarios que aceptaron la
// anterior verán el muro de re-aceptación. La fecha de vigencia queda pendiente hasta la
// validación legal (los documentos son borradores).
const (
	VersionTerminos   = "1.0"
	VersionPrivacidad = "1.0"
)

// ErrLegalDocNoExiste se devuelve al aceptar un documento desconocido.
var ErrLegalDocNoExiste = errors.New("documento legal desconocido")

// ConLegal cablea la proyección de aceptaciones legales (opcional, como los demás Con*).
func (s *Service) ConLegal(r legal.Repository) *Service {
	s.legales = r
	return s
}

// hashLegal computa el SHA-256 del contenido publicado (fija exactamente lo aceptado).
func hashLegal(contenido string) string {
	sum := sha256.Sum256([]byte(contenido))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// VersionesLegalesVigentes devuelve las versiones vigentes con su hash real (computado del
// contenido incrustado). El frontend nunca decide la versión: la recibe del servidor.
func (s *Service) VersionesLegalesVigentes() []legal.Version {
	return []legal.Version{
		{Documento: legal.DocTerminos, Version: VersionTerminos, Hash: hashLegal(legaldocs.Terminos), Titulo: "Términos y Condiciones de Uso"},
		{Documento: legal.DocPrivacidad, Version: VersionPrivacidad, Hash: hashLegal(legaldocs.Privacidad), Titulo: "Política de Privacidad y Tratamiento de Datos"},
	}
}

// ContenidoLegal devuelve el texto (markdown) de un documento legal para renderizarlo.
func (s *Service) ContenidoLegal(doc string) (string, bool) {
	switch doc {
	case legal.DocTerminos:
		return legaldocs.Terminos, true
	case legal.DocPrivacidad:
		return legaldocs.Privacidad, true
	}
	return "", false
}

func (s *Service) versionVigente(doc string) (legal.Version, bool) {
	for _, v := range s.VersionesLegalesVigentes() {
		if v.Documento == doc {
			return v, true
		}
	}
	return legal.Version{}, false
}

// LegalPendiente devuelve las versiones vigentes que el usuario AÚN no ha aceptado. Si no
// hay proyección cableada, se consideran todas pendientes (secure-by-default).
func (s *Service) LegalPendiente(userID string) []legal.Version {
	out := []legal.Version{}
	for _, v := range s.VersionesLegalesVigentes() {
		if s.legales == nil {
			out = append(out, v)
			continue
		}
		a, ok := s.legales.Ultima(userID, v.Documento)
		if !ok || a.Version != v.Version {
			out = append(out, v)
		}
	}
	return out
}

// AceptarLegal registra la aceptación de un documento por un usuario: emite el evento
// append-only de auditoría (prueba inmutable, con hash + IP + user-agent) y actualiza la
// proyección. La versión y el hash los fija el servidor (no el cliente).
func (s *Service) AceptarLegal(userID, empresaID, rol, doc, origen, userAgent, metodo string) (legal.Aceptacion, error) {
	vig, ok := s.versionVigente(doc)
	if !ok {
		return legal.Aceptacion{}, ErrLegalDocNoExiste
	}
	if metodo == "" {
		metodo = legal.MetodoOnboarding
	}
	a := legal.Aceptacion{
		UserID: userID, EmpresaID: empresaID, Rol: rol,
		Documento: doc, Version: vig.Version, Hash: vig.Hash,
		Fecha: ahora(), Origen: origen, UserAgent: userAgent, Metodo: metodo,
	}
	if s.legales != nil {
		s.legales.Registrar(a)
	}
	if s.audit != nil {
		// Evento con Rol para la prueba (el helper evento() no lo lleva).
		s.audit.Append(auditoria.Evento{
			EmpresaID: empresaID, Actor: userID, Rol: rol,
			Accion:  "legal.aceptacion",
			Entidad: doc + "@" + vig.Version,
			Detalle: fmt.Sprintf("%s | metodo=%s | ua=%s", vig.Hash, metodo, userAgent),
			Origen:  origen, Fecha: ahora(),
		})
	}
	return a, nil
}

// AceptacionesDe devuelve el historial de aceptaciones de un usuario (evidencia).
func (s *Service) AceptacionesDe(userID string) []legal.Aceptacion {
	if s.legales == nil {
		return []legal.Aceptacion{}
	}
	return s.legales.PorUsuario(userID)
}
