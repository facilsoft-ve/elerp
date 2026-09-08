// Package httpapi expone la API REST de ElERP sobre Fiber. Aquí vive la
// cadena de middleware, la autenticación por cookie de sesión y la resolución
// del contexto de tenant (empresa + sede) en cada petición.
package httpapi

import (
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/mornix/elerp/internal/adapter/almacen"
	"github.com/mornix/elerp/internal/adapter/hubmy"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/config"
	"github.com/mornix/elerp/internal/domain/authn"
)

// Server agrupa dependencias y monta las rutas.
type Server struct {
	cfg      config.Config
	svc      *application.Service
	tenancy  *application.TenancyService
	hubmy    *hubmy.Client
	sessions authn.Store
	archivos *almacen.Almacen
}

// NewServer construye la app Fiber con middleware y rutas.
func NewServer(cfg config.Config, svc *application.Service, tenancy *application.TenancyService, hb *hubmy.Client, sessions authn.Store, archivos *almacen.Almacen) *fiber.App {
	s := &Server{cfg: cfg, svc: svc, tenancy: tenancy, hubmy: hb, sessions: sessions, archivos: archivos}

	app := fiber.New(fiber.Config{
		AppName:     "ElERP API",
		BodyLimit:   4 * 1024 * 1024,
		ProxyHeader: fiber.HeaderXForwardedFor,
		// Immutable: los strings de c.Get/Params/Query/Cookies se copian, no
		// apuntan al buffer reutilizable de fasthttp. Imprescindible porque
		// persistimos empresaID/sedeID (cabeceras) en el store; sin esto, el
		// buffer se reutiliza en la siguiente petición y corrompe los datos.
		Immutable: true,
	})
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{Format: "${time} ${status} ${method} ${path} (${latency})\n"}))
	app.Use(helmet.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.FrontendURL,
		AllowCredentials: true,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Empresa-ID, X-Sede-ID, X-Almacen-ID",
		AllowMethods:     "GET, POST, PUT, PATCH, DELETE, OPTIONS",
	}))
	app.Use(limiter.New(limiter.Config{
		Max:          300,
		Expiration:   time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string { return c.IP() },
		Next: func(c *fiber.Ctx) bool {
			return c.Path() == "/health" || c.Path() == "/api/health"
		},
	}))

	// Límite estricto para los endpoints de autenticación: frena la fuerza bruta
	// de contraseñas (native-login) y el abuso de los flujos de login/aceptación,
	// sin tocar el límite general (300/min) del resto de la API.
	authLimiter := limiter.New(limiter.Config{
		Max:          20,
		Expiration:   time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string { return c.IP() },
	})

	// Públicas
	app.Get("/health", s.handleHealth)
	app.Get("/api/health", s.handleHealth)
	app.Get("/api/auth/login", authLimiter, s.handleLogin)
	app.Get("/api/auth/dev-login", authLimiter, s.handleDevLogin)
	app.Get("/auth/hubmy/callback", s.handleCallback)
	app.Post("/api/auth/native-login", authLimiter, s.handleNativeLogin)
	app.Post("/api/auth/logout", s.handleLogout)
	app.Get("/api/auth/invite/:token", s.handleInvitePreview)
	app.Post("/api/auth/accept-invite", authLimiter, s.handleAcceptInvite)
	// Captación del prospecto antes de la demo (pública, sin tenant).
	app.Post("/api/demo/lead", s.handleRegistrarLeadDemo)
	// Documentos legales vigentes (PÚBLICO: titulares y terceros deben poder consultarlos).
	app.Get("/api/legal/vigente", s.handleLegalVigente)

	// Autenticadas (sin exigir empresa activa: onboarding y /me)
	api := app.Group("/api", s.requireAuth)
	api.Get("/me", s.handleMe)
	api.Post("/session/context", s.handleSessionContext)
	// Aceptación de documentos legales (por usuario; sin exigir empresa activa).
	api.Get("/legal/estado", s.handleLegalEstado)
	api.Post("/legal/aceptar", s.handleLegalAceptar)
	api.Get("/legal/historial", s.handleLegalHistorial)
	api.Post("/empresas", s.handleCrearEmpresa)
	api.Post("/empresas/:id/onboarding", s.handleOnboarding)
	api.Patch("/empresas/:id", s.handleActualizarEmpresa)
	api.Patch("/empresas/:id/impuestos", s.handleActualizarImpuestos)
	api.Post("/empresas/:id/sedes", s.handleCrearSede)
	api.Patch("/empresas/:id/sedes/:sedeId", s.handleActualizarSede)
	api.Post("/empresas/:id/sedes/:sedeId/desactivar", s.handleDesactivarSede)
	api.Post("/empresas/:id/sedes/:sedeId/reactivar", s.handleReactivarSede)
	// Invitación de miembros (Configuración › Usuarios y roles). Va en el grupo
	// /empresas/:id (sin empresaContext, como las demás rutas de administración del
	// tenant): el permiso lo valida puedeAdministrar contra el :id de la ruta.
	api.Post("/empresas/:id/invitaciones", s.handleInvitarMiembro)
	api.Post("/empresas/:id/invitaciones/:miembroId/reenviar", s.handleReenviarInvitacion)
	api.Post("/empresas/:id/invitaciones/:miembroId/cancelar", s.handleCancelarInvitacion)

	// Archivos del bucket del tenant. Va ANTES del grupo `data` a propósito: en
	// Fiber, api.Group("", empresaContext) monta ese middleware en el prefijo /api
	// para TODA ruta registrada después, así que registrar esto luego lo obligaría
	// a exigir X-Empresa-ID —header que un <img src="/api/archivos/…"> no puede
	// mandar (solo va la cookie)—. Acá solo exige sesión; handleArchivo valida que
	// el usuario sea miembro de la empresa de la ruta. Imagen de producto = dato de
	// negocio, no asset público.
	api.Get("/archivos/:empresa/:carpeta/:nombre", s.handleArchivo)

	// Superficie interna de plataforma (super-admin / Mornix), M2M. NO pasa por
	// requireAuth ni empresaContext: cruza el aislamiento por tenant a propósito.
	// Cerrada por defecto si no hay PLATFORM_API_KEY (requirePlatform → 503).
	internal := app.Group("/internal", s.requirePlatform)
	internal.Get("/tenants", s.handlePlatformTenants)
	internal.Get("/tenants/:empresaId", s.handlePlatformTenant)
	internal.Get("/tenants/:empresaId/auditoria", s.handlePlatformAuditoria)
	internal.Post("/tenants/:empresaId/export", s.handlePlatformExport)
	internal.Post("/tenants/:empresaId/sandbox", s.handlePlatformSandbox)
	internal.Post("/restore", s.handlePlatformRestore)
	internal.Delete("/empresas/:id", s.handlePlatformSandboxDelete)
	internal.Patch("/orgs/:id/estado", s.handlePlatformOrgEstado)
	internal.Patch("/orgs/:id/plan", s.handlePlatformOrgPlan)
	internal.Get("/orgs/:id/uso", s.handlePlatformOrgUso)
	internal.Patch("/empresas/:id/activa", s.handlePlatformEmpresaActiva)

	// Con contexto de tenant (exigen X-Empresa-ID válido)
	data := api.Group("", s.empresaContext)
	data.Get("/bootstrap", s.handleBootstrap)
	s.registerInventario(data)
	s.registerFiscal(data)
	s.registerVentas(data)
	s.registerCupones(data)
	s.registerPromociones(data)
	s.registerArchivos(data)
	s.registerCajas(data)
	s.registerTasa(data)
	s.registerTesoreria(data)
	s.registerContabilidad(data)
	s.registerCompras(data)
	s.registerListasPrecio(data)
	s.registerUnidades(data)
	s.registerFormatos(data)
	s.registerRestaurante(data)
	s.registerAlmacenes(data)
	s.registerAplicaciones(data)
	s.registerReportes(data)
	s.registerAuditoria(data)
	s.registerAsistente(data)

	return app
}

// requireRoles crea un middleware que solo deja pasar los roles indicados.
func (s *Server) requireRoles(allowed ...string) fiber.Handler {
	set := make(map[string]bool, len(allowed))
	for _, r := range allowed {
		set[r] = true
	}
	return func(c *fiber.Ctx) error {
		if set[rolOf(c)] {
			return c.Next()
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin permiso para esta acción"})
	}
}

// --- Middleware ---

func (s *Server) requireAuth(c *fiber.Ctx) error {
	id := c.Cookies(s.cfg.CookieName)
	sess, ok := s.sessions.Get(id)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no autenticado"})
	}
	c.Locals("principal", sess.Principal)
	c.Locals("session", sess)
	return c.Next()
}

// empresaContext resuelve y valida el tenant activo desde X-Empresa-ID, y la
// sede desde X-Sede-ID. Es el punto donde se aplica el aislamiento por empresa.
func (s *Server) empresaContext(c *fiber.Ctx) error {
	p := principalOf(c)
	empID := c.Get("X-Empresa-ID")
	if empID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "seleccione una empresa"})
	}
	emp, rol, sedeFija, ok := s.tenancy.RolEnEmpresa(p.UserID, empID)
	if !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sin acceso a esa empresa"})
	}
	// Tenant cuya ORGANIZACIÓN suspendió la plataforma (impago, etc.): se bloquea
	// toda operación con datos. La membresía sigue siendo válida; lo que corta es
	// el estado comercial de la organización (lo fija la consola de super-admin).
	if !s.tenancy.OrgActivaPorID(emp.OrganizacionID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "organización suspendida — contacta a soporte"})
	}
	sedeID := c.Get("X-Sede-ID")
	if sedeFija != "" {
		sedeID = sedeFija // roles de una sola sede quedan fijados a la suya
	}
	if !s.tenancy.SedeValida(empID, sedeID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sede inválida"})
	}
	c.Locals("empresaID", empID)
	c.Locals("empresa", emp)
	c.Locals("rol", rol)
	c.Locals("sedeID", sedeID)
	return c.Next()
}

// --- Helpers de contexto ---

func principalOf(c *fiber.Ctx) authn.Principal {
	if p, ok := c.Locals("principal").(authn.Principal); ok {
		return p
	}
	return authn.Principal{}
}

func empresaIDOf(c *fiber.Ctx) string {
	if v, ok := c.Locals("empresaID").(string); ok {
		return v
	}
	return ""
}

func sedeIDOf(c *fiber.Ctx) string {
	if v, ok := c.Locals("sedeID").(string); ok {
		return v
	}
	return ""
}

func rolOf(c *fiber.Ctx) string {
	if v, ok := c.Locals("rol").(string); ok {
		return v
	}
	return ""
}

// origen arma una etiqueta de origen (IP) para auditoría.
func origen(c *fiber.Ctx) string { return c.IP() }

// handleArchivo sirve un archivo del bucket de una empresa. Solo exige sesión
// (para que un <img> lo pueda cargar con la sola cookie), y rechaza la lectura
// cruzada entre tenants validando que el usuario de la sesión sea MIEMBRO de la
// empresa de la ruta (no compara contra un header que el <img> no puede mandar).
func (s *Server) handleArchivo(c *fiber.Ctx) error {
	if s.archivos == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	emp := c.Params("empresa")
	if emp == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "ese archivo no pertenece a tu empresa"})
	}
	if _, _, _, ok := s.tenancy.RolEnEmpresa(principalOf(c).UserID, emp); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "ese archivo no pertenece a tu empresa"})
	}
	carpeta := filepath.Base(filepath.Clean("/" + c.Params("carpeta")))
	nombre := filepath.Base(filepath.Clean("/" + c.Params("nombre")))
	ruta := filepath.Join(s.archivos.Raiz(), emp, carpeta, nombre)
	// Las imágenes son inmutables (nombre aleatorio por subida): se pueden
	// cachear con agresividad, pero solo en el navegador del usuario.
	c.Set("Cache-Control", "private, max-age=31536000, immutable")
	return c.SendFile(ruta, false)
}
