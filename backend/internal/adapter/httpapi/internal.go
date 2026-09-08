package httpapi

import (
	"crypto/subtle"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/organizacion"
)

// Superficie INTERNA de plataforma (/internal). La consume, máquina-a-máquina, la
// consola de super-admin (Mornix) — un servicio aparte, no una cuenta cliente.
// Va detrás de requirePlatform (clave M2M) y NO pasa por requireAuth ni
// empresaContext: a propósito cruza el aislamiento por tenant para listar y
// gestionar a TODOS los clientes. Es F0 del plan de la consola de plataforma.

// requirePlatform autentica la clave M2M. Sin PLATFORM_API_KEY configurada, la
// superficie queda CERRADA (503): seguro por defecto, nunca abierta sin clave.
func (s *Server) requirePlatform(c *fiber.Ctx) error {
	key := s.cfg.PlatformAPIKey
	if key == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "API de plataforma no configurada"})
	}
	// Comparación de tiempo constante para no filtrar la clave por temporización.
	got := c.Get("X-Platform-Key")
	if subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no autorizado"})
	}
	return c.Next()
}

// platformActor identifica al operador Mornix detrás de la llamada (para la
// auditoría). El servicio de plataforma lo manda en X-Platform-Actor; si falta,
// se registra como "plataforma".
func platformActor(c *fiber.Ctx) string {
	if a := c.Get("X-Platform-Actor"); a != "" {
		return a
	}
	return "plataforma"
}

// GET /internal/tenants — lista todas las organizaciones con sus empresas y sedes.
func (s *Server) handlePlatformTenants(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"tenants": s.tenancy.TenantsPlataforma()})
}

// GET /internal/tenants/:empresaId — detalle de un tenant (empresa + su org).
func (s *Server) handlePlatformTenant(c *fiber.Ctx) error {
	view, ok := s.tenancy.TenantPlataforma(c.Params("empresaId"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tenant no encontrado"})
	}
	return c.JSON(view)
}

// PATCH /internal/orgs/:id/estado — activa | suspendida.
func (s *Server) handlePlatformOrgEstado(c *fiber.Ctx) error {
	var in struct {
		Estado string `json:"estado"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	org, err := s.tenancy.FijarEstadoOrg(c.Params("id"), in.Estado, platformActor(c), origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(org)
}

// POST /internal/tenants/:empresaId/sandbox — crea un sandbox (QA) del tenant:
// clona config+sedes+miembros y los maestros de negocio, con ledgers vacíos.
func (s *Server) handlePlatformSandbox(c *fiber.Ctx) error {
	var in struct {
		TtlDias int `json:"ttlDias"`
	}
	_ = c.BodyParser(&in) // body opcional (ttl por defecto en el servicio)
	base, sedeMap, err := s.tenancy.CrearSandboxBase(c.Params("empresaId"), in.TtlDias, platformActor(c), origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	clon := s.svc.ClonarMaestros(c.Params("empresaId"), base.ID, sedeMap)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"sandbox": base, "clon": clon})
}

// DELETE /internal/empresas/:id — archiva (soft-delete) un sandbox.
func (s *Server) handlePlatformSandboxDelete(c *fiber.Ctx) error {
	if err := s.tenancy.EliminarSandbox(c.Params("id"), platformActor(c), origen(c)); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// GET /internal/tenants/:empresaId/auditoria?desde&hasta&limite — log de cambios
// del tenant, filtrado por rango de fechas y acotado (más nuevo primero).
func (s *Server) handlePlatformAuditoria(c *fiber.Ctx) error {
	ev := s.svc.AuditoriaEntre(c.Params("empresaId"), c.Query("desde"), c.Query("hasta"), c.QueryInt("limite", 500))
	return c.JSON(fiber.Map{"eventos": ev})
}

// PATCH /internal/orgs/:id/plan — { "plan": "...", "limites": {...} }.
func (s *Server) handlePlatformOrgPlan(c *fiber.Ctx) error {
	var in struct {
		Plan    string               `json:"plan"`
		Limites organizacion.Limites `json:"limites"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	org, err := s.tenancy.FijarPlan(c.Params("id"), in.Plan, in.Limites, platformActor(c), origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(org)
}

// GET /internal/orgs/:id/uso — uso de capacidad vs. límites del plan (cuota).
func (s *Server) handlePlatformOrgUso(c *fiber.Ctx) error {
	orgID := c.Params("id")
	org, ok := s.tenancy.Org(orgID)
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "organización no encontrada"})
	}
	usuarios, sucursales, empresaIDs, _ := s.tenancy.ConteosDeOrg(orgID)
	facturas := 0
	for _, empID := range empresaIDs {
		facturas += s.svc.FacturasDelMes(empID, "")
	}
	return c.JSON(fiber.Map{
		"orgId":       orgID,
		"plan":        org.Plan,
		"facturasMes": application.Cuota(facturas, org.Limites.FacturasMes),
		"usuarios":    application.Cuota(usuarios, org.Limites.Usuarios),
		"sucursales":  application.Cuota(sucursales, org.Limites.Sucursales),
	})
}

// PATCH /internal/empresas/:id/activa — { "activa": bool }.
func (s *Server) handlePlatformEmpresaActiva(c *fiber.Ctx) error {
	var in struct {
		Activa bool `json:"activa"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	emp, err := s.tenancy.FijarEmpresaActiva(c.Params("id"), in.Activa, platformActor(c), origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(emp)
}
