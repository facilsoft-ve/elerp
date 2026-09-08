package httpapi

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/authn"
)

func (s *Server) handleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"ok": true, "hubmy": s.cfg.HubmyConfigured(), "devLogin": s.cfg.DevLogin})
}

func (s *Server) sameSite() string {
	if s.cfg.CookieSameSite != "" {
		return s.cfg.CookieSameSite
	}
	if s.cfg.CookieSecure {
		return "None"
	}
	return "Lax"
}

// stateCookieName es la cookie de un solo uso que guarda el nonce anti-CSRF del
// flujo OAuth de Hubmy (se emite en /login y se compara en el callback).
func (s *Server) stateCookieName() string { return s.cfg.CookieName + "_oauth_state" }

func (s *Server) issueSession(c *fiber.Ctx, p authn.Principal, hubmyToken string) {
	sess := s.sessions.Create(p, hubmyToken, s.cfg.SessionTTL)
	c.Cookie(&fiber.Cookie{
		Name: s.cfg.CookieName, Value: sess.ID, Path: "/", Domain: s.cfg.CookieDomain,
		HTTPOnly: true, Secure: s.cfg.CookieSecure, SameSite: s.sameSite(), Expires: sess.ExpiresAt,
	})
}

func randState() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// handleLogin redirige al SSO de Hubmy.
func (s *Server) handleLogin(c *fiber.Ctx) error {
	if !s.cfg.HubmyConfigured() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Hubmy no está configurado"})
	}
	// Nonce anti-CSRF (Providencia de seguridad / buenas prácticas SDK Auth):
	// se guarda en una cookie HTTPOnly propia y se verifica en el callback.
	state := randState()
	c.Cookie(&fiber.Cookie{
		Name: s.stateCookieName(), Value: state, Path: "/", Domain: s.cfg.CookieDomain,
		HTTPOnly: true, Secure: s.cfg.CookieSecure, SameSite: s.sameSite(), MaxAge: 600,
	})
	return c.Redirect(s.hubmy.AuthorizeURL(s.cfg.HubmyAppID, state), fiber.StatusFound)
}

// handleDevLogin crea una sesión demo sin Hubmy (solo si DEV_LOGIN=true).
func (s *Server) handleDevLogin(c *fiber.Ctx) error {
	if !s.cfg.DevLogin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "modo demo deshabilitado"})
	}
	p := authn.Principal{UserID: application.DemoUserID, Nombre: application.DemoNombre, Email: application.DemoEmail}
	s.issueSession(c, p, "")
	return c.Redirect("/app/", fiber.StatusFound)
}

// handleRegistrarLeadDemo captura los datos de un prospecto ANTES de entrar a la
// demo. Es una ruta PÚBLICA (pre-login, sin tenant): registra el lead de verdad
// en el almacén append-only. Un fallo aquí no debe impedir que el front continúe
// al demo; devuelve el error para que la interfaz lo sepa, pero es no bloqueante.
func (s *Server) handleRegistrarLeadDemo(c *fiber.Ctx) error {
	var in struct {
		Nombre   string `json:"nombre"`
		Empresa  string `json:"empresa"`
		Email    string `json:"email"`
		Telefono string `json:"telefono"`
		Mensaje  string `json:"mensaje"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	lead, err := s.svc.RegistrarLeadDemo(application.DemoLeadInput{
		Nombre: in.Nombre, Empresa: in.Empresa, Email: in.Email,
		Telefono: in.Telefono, Mensaje: in.Mensaje,
	}, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "id": lead.ID})
}

// handleCallback valida el token de Hubmy, crea la sesión y redirige al front.
func (s *Server) handleCallback(c *fiber.Ctx) error {
	// Anti-CSRF: el `state` devuelto por Hubmy (callback_method=GET) debe coincidir
	// con el nonce que emitimos en /login. La cookie es de un solo uso: se limpia
	// siempre, haya match o no.
	want := c.Cookies(s.stateCookieName())
	got := c.Query("state")
	c.Cookie(&fiber.Cookie{
		Name: s.stateCookieName(), Value: "", Path: "/", Domain: s.cfg.CookieDomain,
		HTTPOnly: true, Secure: s.cfg.CookieSecure, SameSite: s.sameSite(), MaxAge: -1,
	})
	if want == "" || got != want {
		return c.Status(fiber.StatusBadRequest).SendString("state inválido")
	}

	// Hubmy entrega el JWT en el parámetro `session` (callback_method=GET):
	//   /auth/hubmy/callback?session=<JWT>&user_id=<id>&state=<nonce>
	token := c.Query("session")
	if token == "" {
		token = c.Query("token") // compatibilidad
	}
	if token == "" {
		return c.Status(fiber.StatusBadRequest).SendString("falta la sesión de Hubmy")
	}
	user, _, err := s.hubmy.Validate(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("token inválido")
	}
	nombre := user.Name
	if nombre == "" {
		nombre = user.DisplayName
	}
	p := authn.Principal{UserID: user.ID, Nombre: nombre, Email: user.Email}
	s.tenancy.ClaimInvitations(user.Email, user.ID, nombre)
	s.issueSession(c, p, token)
	return c.Redirect("/app/", fiber.StatusFound)
}

// handleNativeLogin autentica con email + contraseña locales.
func (s *Server) handleNativeLogin(c *fiber.Ctx) error {
	var in struct{ Email, Password string }
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	p, err := s.tenancy.LoginNativo(in.Email, in.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}
	s.issueSession(c, p, "")
	return c.JSON(fiber.Map{"ok": true})
}

// handleLogout revoca la sesión (y la de Hubmy si aplica).
func (s *Server) handleLogout(c *fiber.Ctx) error {
	id := c.Cookies(s.cfg.CookieName)
	if sess, ok := s.sessions.Get(id); ok {
		if sess.HubmyToken != "" {
			_ = s.hubmy.Logout(c.Context(), sess.HubmyToken)
		}
		s.sessions.Delete(id)
	}
	c.Cookie(&fiber.Cookie{Name: s.cfg.CookieName, Value: "", Path: "/", Domain: s.cfg.CookieDomain, HTTPOnly: true, Secure: s.cfg.CookieSecure, SameSite: s.sameSite(), MaxAge: -1})
	return c.JSON(fiber.Map{"ok": true})
}

// handleMe devuelve el contexto del usuario (organizaciones→empresas→sedes).
func (s *Server) handleMe(c *fiber.Ctx) error {
	return c.JSON(s.tenancy.Me(principalOf(c)))
}

// handleSessionContext valida que el usuario pueda operar la empresa+sede dada.
func (s *Server) handleSessionContext(c *fiber.Ctx) error {
	var in struct {
		EmpresaID string `json:"empresaId"`
		SedeID    string `json:"sedeId"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	_, _, _, ok := s.tenancy.RolEnEmpresa(principalOf(c).UserID, in.EmpresaID)
	if !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin acceso a esa empresa"})
	}
	if !s.tenancy.SedeValida(in.EmpresaID, in.SedeID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sede inválida"})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// handleInvitePreview muestra datos de una invitación (pantalla de aceptar).
func (s *Server) handleInvitePreview(c *fiber.Ctx) error {
	data, err := s.tenancy.PreviewInvitacion(c.Params("token"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(data)
}

// handleAcceptInvite crea la credencial local y activa la membresía.
func (s *Server) handleAcceptInvite(c *fiber.Ctx) error {
	var in struct{ Token, Nombre, Password string }
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	p, err := s.tenancy.AceptarInvitacion(in.Token, in.Nombre, in.Password, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	s.issueSession(c, p, "")
	return c.JSON(fiber.Map{"ok": true})
}
