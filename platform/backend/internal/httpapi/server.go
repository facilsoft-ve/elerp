// Package httpapi es la capa HTTP del BFF de la consola: autenticación propia de
// operadores (contraseña + TOTP), y un proxy gateado por sesión hacia el core /internal/*.
package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/mornix/elerp-platform/internal/billing"
	"github.com/mornix/elerp-platform/internal/config"
	"github.com/mornix/elerp-platform/internal/core"
	"github.com/mornix/elerp-platform/internal/lead"
	"github.com/mornix/elerp-platform/internal/notify"
	"github.com/mornix/elerp-platform/internal/operador"
)

// Server agrupa las dependencias de la capa HTTP.
type Server struct {
	cfg  config.Config
	ops  operador.Repository
	sess operador.SessionStore
	core *core.Client
	bill *billing.Service
	// leads son las solicitudes de demo de la web; avisos manda el email al equipo
	// (puede ser el notificador nulo: el lead se guarda igual).
	leads  lead.Repository
	avisos notify.Notificador
}

// NewServer construye la app Fiber del BFF.
func NewServer(cfg config.Config, ops operador.Repository, sess operador.SessionStore, cli *core.Client, bill *billing.Service, leads lead.Repository, avisos notify.Notificador) *fiber.App {
	if avisos == nil {
		avisos = notify.Nulo{}
	}
	s := &Server{cfg: cfg, ops: ops, sess: sess, core: cli, bill: bill, leads: leads, avisos: avisos}

	app := fiber.New(fiber.Config{
		AppName:               "huberp-platform",
		DisableStartupMessage: true,
		// El core corre con Immutable; aquí no persistimos strings del contexto, pero lo
		// dejamos por prudencia (mismo criterio que el ERP).
		Immutable: true,
	})
	app.Use(helmet.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.FrontendURL,
		AllowCredentials: true,
		AllowHeaders:     "Content-Type",
	}))

	// Límite general anti-abuso.
	app.Use(limiter.New(limiter.Config{Max: 300, Expiration: time.Minute}))

	// Límite estricto para las rutas de autenticación (fuerza bruta de contraseña/OTP).
	authLimiter := limiter.New(limiter.Config{
		Max: 20, Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string { return c.IP() },
	})

	papi := app.Group("/papi")

	// Salud (sin auth): útil para el healthcheck del contenedor.
	papi.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true, "devLogin": s.cfg.DevLogin})
	})

	// Autenticación (sin sesión previa).
	auth := papi.Group("/auth")
	auth.Post("/login", authLimiter, s.handleLogin)
	auth.Post("/mfa/setup", authLimiter, s.handleMFASetup)
	auth.Post("/mfa/verify", authLimiter, s.handleMFAVerify)
	auth.Post("/logout", s.handleLogout)
	auth.Get("/me", s.handleMe)

	// Solicitudes de demo del formulario público de la web. Límite MUY estricto: es
	// escritura sin autenticación. El honeypot y la validación están en el handler.
	if cfg.LeadsPublicoHabilitado && leads != nil {
		leadLimiter := limiter.New(limiter.Config{
			Max: 5, Expiration: 10 * time.Minute,
			KeyGenerator: func(c *fiber.Ctx) string { return ipCliente(c) },
		})
		papi.Post("/public/leads", leadLimiter, s.handleCrearLeadPublico)
	}

	// Proxy gateado por sesión de operador hacia el core /internal/*.
	g := papi.Group("", s.requireOperador)
	s.registerProxy(g)
	s.registerBilling(g)
	if leads != nil {
		g.Get("/leads", s.handleListarLeads)
		g.Patch("/leads/:id", s.handleActualizarLead)
	}

	// SPA estático de la consola (si hay build). En dev se usa el server de Vite.
	s.serveSPA(app)

	return app
}

// serveSPA sirve el frontend construido desde WebDir, con fallback a index.html para el
// ruteo del lado del cliente. Solo se activa si el directorio existe.
func (s *Server) serveSPA(app *fiber.App) {
	dir := s.cfg.WebDir
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return
	}
	base := s.cfg.BasePath // "" (raíz) o "/consola"
	app.Static(base+"/", dir, fiber.Static{Index: "index.html"})
	// Fallback SPA: rutas GET bajo el base que no sean archivos reales devuelven el shell.
	index := filepath.Join(dir, "index.html")
	app.Use(func(c *fiber.Ctx) error {
		if c.Method() != fiber.MethodGet {
			return c.Next()
		}
		if base != "" && !strings.HasPrefix(c.Path(), base) {
			return c.Next()
		}
		return c.SendFile(index)
	})
}

// --- Sesión ---------------------------------------------------------------

func (s *Server) sameSite() string {
	if s.cfg.CookieSameSite != "" {
		return s.cfg.CookieSameSite
	}
	if s.cfg.CookieSecure {
		return "None"
	}
	return "Lax"
}

// emitirSesion crea la sesión del operador y setea la cookie opaca.
func (s *Server) emitirSesion(c *fiber.Ctx, o operador.Operador) {
	sesion := s.sess.Create(o, s.cfg.SessionTTL)
	c.Cookie(&fiber.Cookie{
		Name:     s.cfg.CookieName,
		Value:    sesion.ID,
		HTTPOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: s.sameSite(),
		Domain:   s.cfg.CookieDomain,
		Expires:  sesion.ExpiresAt,
		Path:     "/",
	})
}

func (s *Server) limpiarCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name: s.cfg.CookieName, Value: "", HTTPOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: s.sameSite(), Domain: s.cfg.CookieDomain,
		Expires: time.Now().Add(-time.Hour), Path: "/",
	})
}

// requireOperador exige una sesión de operador válida. Guarda la sesión en Locals.
func (s *Server) requireOperador(c *fiber.Ctx) error {
	id := c.Cookies(s.cfg.CookieName)
	sesion, ok := s.sess.Get(id)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no autenticado"})
	}
	c.Locals("sesion", sesion)
	return c.Next()
}

func sesionDe(c *fiber.Ctx) operador.Session {
	if s, ok := c.Locals("sesion").(operador.Session); ok {
		return s
	}
	return operador.Session{}
}
