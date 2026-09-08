package httpapi

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/mornix/elerp-platform/internal/mfa"
	"github.com/mornix/elerp-platform/internal/operador"
)

// Flujo de autenticación en dos factores:
//   1) login {email, password[, codigo]}:
//        · password inválida            → 401
//        · MFA no enrolada              → 200 {paso:"enrolar-mfa"}  (el cliente enrola)
//        · MFA enrolada y sin código    → 401 {paso:"mfa"}         (pide el código)
//        · MFA enrolada y código válido → emite sesión, 200 {ok}
//   2) mfa/setup {email, password}: valida la contraseña, genera secreto TOTP pendiente
//      y devuelve el otpauthUrl (para el QR).
//   3) mfa/verify {email, password, codigo}: valida contraseña + código contra el secreto
//      pendiente, marca la MFA como configurada y emite sesión.
//
// setup/verify piden la contraseña de nuevo porque aún no hay sesión. Todo va detrás del
// authLimiter (fuerza bruta).

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Codigo   string `json:"codigo"`
}

// validarPassword busca el operador y compara la contraseña con bcrypt en tiempo
// constante-ish (siempre corre una comparación, incluso si el operador no existe).
func (s *Server) validarPassword(email, password string) (operador.Operador, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	o, ok := s.ops.ByEmail(email)
	if !ok {
		// Comparación señuelo para no filtrar la existencia del operador por tiempo.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinva"), []byte(password))
		return operador.Operador{}, false
	}
	if bcrypt.CompareHashAndPassword([]byte(o.Hash), []byte(password)) != nil {
		return operador.Operador{}, false
	}
	return o, true
}

func (s *Server) handleLogin(c *fiber.Ctx) error {
	var in loginBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	o, ok := s.validarPassword(in.Email, in.Password)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "credenciales inválidas"})
	}
	// Bypass TEMPORAL del segundo factor (entornos con reloj no confiable): con la
	// contraseña válida se emite sesión directo, sin TOTP.
	if s.cfg.MFADeshabilitada {
		s.emitirSesion(c, o)
		return c.JSON(fiber.Map{"ok": true, "operador": fiber.Map{"email": o.Email, "nombre": o.Nombre}})
	}
	if !o.MFAConfigurada {
		// Falta enrolar el segundo factor: el cliente debe ir a /mfa/setup.
		return c.JSON(fiber.Map{"paso": "enrolar-mfa"})
	}
	if strings.TrimSpace(in.Codigo) == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"paso": "mfa", "error": "código requerido"})
	}
	if !mfa.Validar(o.TOTPSecret, in.Codigo, time.Now()) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"paso": "mfa", "error": "código inválido"})
	}
	s.emitirSesion(c, o)
	return c.JSON(fiber.Map{"ok": true, "operador": fiber.Map{"email": o.Email, "nombre": o.Nombre}})
}

type setupBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleMFASetup(c *fiber.Ctx) error {
	var in setupBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	o, ok := s.validarPassword(in.Email, in.Password)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "credenciales inválidas"})
	}
	if o.MFAConfigurada {
		// Ya enrolada: no se re-enrola desde acá (evita que un código robado la reinicie).
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "el segundo factor ya está configurado"})
	}
	secreto, err := mfa.GenerarSecreto()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "no se pudo generar el secreto"})
	}
	o.TOTPSecret = secreto // pendiente: aún MFAConfigurada=false hasta verificar
	if _, ok := s.ops.Update(o); !ok {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "no se pudo guardar"})
	}
	return c.JSON(fiber.Map{"otpauthUrl": mfa.OtpauthURL(secreto, o.Email), "secreto": secreto})
}

type verifyBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Codigo   string `json:"codigo"`
}

func (s *Server) handleMFAVerify(c *fiber.Ctx) error {
	var in verifyBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	o, ok := s.validarPassword(in.Email, in.Password)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "credenciales inválidas"})
	}
	if o.TOTPSecret == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "primero genera el código QR (setup)"})
	}
	if !mfa.Validar(o.TOTPSecret, in.Codigo, time.Now()) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "código inválido"})
	}
	o.MFAConfigurada = true
	if _, ok := s.ops.Update(o); !ok {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "no se pudo guardar"})
	}
	s.emitirSesion(c, o)
	return c.JSON(fiber.Map{"ok": true, "operador": fiber.Map{"email": o.Email, "nombre": o.Nombre}})
}

func (s *Server) handleLogout(c *fiber.Ctx) error {
	if id := c.Cookies(s.cfg.CookieName); id != "" {
		s.sess.Delete(id)
	}
	s.limpiarCookie(c)
	return c.JSON(fiber.Map{"ok": true})
}

func (s *Server) handleMe(c *fiber.Ctx) error {
	id := c.Cookies(s.cfg.CookieName)
	sesion, ok := s.sess.Get(id)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no autenticado"})
	}
	return c.JSON(fiber.Map{"email": sesion.Email, "nombre": sesion.Nombre})
}
