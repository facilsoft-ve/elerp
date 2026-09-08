// Package notify manda avisos por email al equipo de Mornix (hoy: una solicitud de demo
// nueva). Es un puerto con dos implementaciones: SMTP y una nula.
//
// Por qué SMTP y no Hubmy: la API de emails de Hubmy solo acepta como destino un USER de
// la app (user/group/broadcast) que ya haya hecho SDK auth — devuelve ERR_USER_NOT_ELIGIBLE
// para una dirección arbitraria. El buzón del equipo (contacto@mornix.tech) no es un user
// de la app, así que Hubmy no puede entregarle. SMTP sí.
package notify

import (
	"fmt"
	"mime"
	"net/smtp"
	"strings"
	"time"
)

// Aviso es un email saliente ya redactado.
type Aviso struct {
	Asunto string
	Cuerpo string // texto plano UTF-8
	// ReplyTo deja que el equipo responda al prospecto con "Responder" (opcional).
	ReplyTo string
}

// Notificador entrega avisos. Enviar debe ser seguro de llamar aunque no haya
// configuración: en ese caso no hace nada y no es un error.
type Notificador interface {
	Enviar(a Aviso) error
	Configurado() bool
}

// Nulo descarta los avisos (sin SMTP configurado). Mantiene el código de llamada simple:
// nunca hay que chequear nil.
type Nulo struct{}

func (Nulo) Enviar(Aviso) error { return nil }
func (Nulo) Configurado() bool  { return false }

// SMTP entrega por un servidor SMTP con STARTTLS (puerto 587, el caso normal).
//
// Limitación conocida: usa smtp.SendMail, que negocia STARTTLS. NO sirve para el puerto
// 465 (TLS implícito desde el saludo). Configurá 587.
type SMTP struct {
	host    string
	puerto  string
	usuario string
	clave   string
	desde   string
	para    []string
}

// NuevoSMTP arma el notificador. Si falta host, remitente o destinatarios devuelve nil,
// y el llamador debe usar Nulo.
func NuevoSMTP(host, puerto, usuario, clave, desde string, para []string) *SMTP {
	limpios := make([]string, 0, len(para))
	for _, p := range para {
		if p = strings.TrimSpace(p); p != "" {
			limpios = append(limpios, p)
		}
	}
	if host == "" || desde == "" || len(limpios) == 0 {
		return nil
	}
	if puerto == "" {
		puerto = "587"
	}
	return &SMTP{host: host, puerto: puerto, usuario: usuario, clave: clave, desde: desde, para: limpios}
}

func (s *SMTP) Configurado() bool { return s != nil && s.host != "" }

// Destinatarios expone a quién se avisa (para el log de arranque).
func (s *SMTP) Destinatarios() []string {
	if s == nil {
		return nil
	}
	return s.para
}

func (s *SMTP) Enviar(a Aviso) error {
	if !s.Configurado() {
		return nil
	}
	var auth smtp.Auth
	if s.usuario != "" {
		auth = smtp.PlainAuth("", s.usuario, s.clave, s.host)
	}
	addr := s.host + ":" + s.puerto
	return smtp.SendMail(addr, auth, s.desde, s.para, s.mensaje(a))
}

// mensaje arma el RFC 5322. El asunto va como encoded-word porque lleva acentos.
func (s *SMTP) mensaje(a Aviso) []byte {
	h := []string{
		"From: " + s.desde,
		"To: " + strings.Join(s.para, ", "),
		"Subject: " + mime.QEncoding.Encode("utf-8", a.Asunto),
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	if a.ReplyTo != "" {
		h = append(h, "Reply-To: "+a.ReplyTo)
	}
	// CRLF es obligatorio en SMTP, y los saltos del cuerpo también deben normalizarse.
	cuerpo := strings.ReplaceAll(strings.ReplaceAll(a.Cuerpo, "\r\n", "\n"), "\n", "\r\n")
	return []byte(fmt.Sprintf("%s\r\n\r\n%s", strings.Join(h, "\r\n"), cuerpo))
}
