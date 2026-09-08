package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"sort"
	"strings"

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

// aceptarState decide si el callback puede seguir, y de qué flujo viene.
//
//	want = el nonce de NUESTRA cookie ("" si no hay ninguna)
//	got  = el `state` que devuelve Hubmy
//
// Devuelve (iniciadoPorPlataforma, aceptar). Está aparte del handler para poder probar
// la matriz completa sin montar el servidor: es la regla de seguridad del login y
// conviene que esté fijada por escrito.
func aceptarState(want, got string) (bool, bool) {
	if want == "" {
		// Sin cookie propia no hay nada que comparar: es el launcher de Hubmy entrando
		// directo. Se acepta y la prueba de identidad pasa a ser el JWT.
		return true, true
	}
	// Nosotros iniciamos el flujo: se exige que el nonce vuelva idéntico.
	return false, got == want
}

// nombresDeQuery lista los NOMBRES de los parámetros de la URL, nunca sus valores: uno
// de ellos es el JWT de Hubmy y no debe terminar en un log.
func nombresDeQuery(c *fiber.Ctx) string {
	nombres := []string{}
	c.Context().QueryArgs().VisitAll(func(k, _ []byte) {
		nombres = append(nombres, string(k))
	})
	if len(nombres) == 0 {
		return "(ninguno)"
	}
	sort.Strings(nombres)
	return strings.Join(nombres, ", ")
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
	// Hay DOS flujos legítimos y se exige distinto en cada uno:
	//
	//   · Iniciado por LA APP (el usuario apretó «Entrar con Hubmy»): nosotros emitimos
	//     el nonce en /api/auth/login, así que la cookie EXISTE y se exige que coincida.
	//     Protección anti-CSRF completa.
	//   · Iniciado por LA PLATAFORMA (el launcher de Hubmy abre la app y salta directo
	//     acá con session+state+user_id): nunca pasó por nuestro /login, así que no hay
	//     cookie ni nonce propio con el que comparar. Exigir uno rechazaría todo login
	//     desde el launcher — que es exactamente lo que pasaba (400 en cada intento).
	//     Acá la prueba de identidad es el JWT, que se valida contra Hubmy más abajo.
	//
	// RIESGO ASUMIDO a conciencia: sin nonce queda abierto el login-CSRF — alguien puede
	// hacer que una víctima abra un enlace con el JWT DEL ATACANTE y quede con sesión en
	// la cuenta ajena. NO expone los datos de la víctima (la sesión es del atacante),
	// pero sirve para engaño o para plantar datos. Se acepta para poder entrar desde el
	// launcher, y cada caso queda registrado para poder auditarlo.
	iniciadoPorPlataforma, ok := aceptarState(want, got)
	if !ok {
		log.Printf("auth/callback rechazado: el state no coincide con el emitido · params recibidos: %s", nombresDeQuery(c))
		return c.Status(fiber.StatusBadRequest).SendString("state inválido")
	}

	// Hubmy entrega el JWT en el parámetro `session` (callback_method=GET):
	//   /auth/hubmy/callback?session=<JWT>&user_id=<id>&state=<nonce>
	token := c.Query("session")
	if token == "" {
		token = c.Query("token") // compatibilidad
	}
	if token == "" {
		log.Printf("auth/callback rechazado: sin `session` ni `token` en la URL · params recibidos: %s", nombresDeQuery(c))
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
	if iniciadoPorPlataforma {
		// Queda en el log para poder auditar el riesgo que se asumió: quién entró sin
		// nonce. El token ya está validado contra Hubmy en este punto.
		log.Printf("auth/callback: login iniciado por la PLATAFORMA (sin nonce propio) · usuario %s", user.ID)
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
