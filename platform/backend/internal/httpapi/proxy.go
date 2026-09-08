package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp-platform/internal/core"
)

// registerProxy monta las rutas /papi/* que delegan en el core /internal/*. Cada una
// viaja con el actor = email del operador (para la auditoría del core) y la clave M2M
// (que el navegador nunca ve). El BFF no re-modela los datos: relaya la respuesta.
func (s *Server) registerProxy(g fiber.Router) {
	g.Get("/tenants", func(c *fiber.Ctx) error {
		return s.forward(c, fiber.MethodGet, "/internal/tenants", nil)
	})
	g.Get("/tenants/:empresaId", func(c *fiber.Ctx) error {
		return s.forward(c, fiber.MethodGet, "/internal/tenants/"+c.Params("empresaId"), nil)
	})
	g.Get("/tenants/:empresaId/auditoria", func(c *fiber.Ctx) error {
		path := "/internal/tenants/" + c.Params("empresaId") + "/auditoria"
		if q := string(c.Request().URI().QueryString()); q != "" {
			path += "?" + q
		}
		return s.forward(c, fiber.MethodGet, path, nil)
	})
	g.Post("/tenants/:empresaId/export", s.handleExport)
	g.Post("/restore", s.handleRestore)
	g.Post("/tenants/:empresaId/sandbox", func(c *fiber.Ctx) error {
		return s.forward(c, fiber.MethodPost, "/internal/tenants/"+c.Params("empresaId")+"/sandbox", c.Body())
	})
	g.Delete("/empresas/:id", func(c *fiber.Ctx) error {
		return s.forward(c, fiber.MethodDelete, "/internal/empresas/"+c.Params("id"), nil)
	})
	g.Patch("/orgs/:id/estado", func(c *fiber.Ctx) error {
		return s.forward(c, fiber.MethodPatch, "/internal/orgs/"+c.Params("id")+"/estado", c.Body())
	})
	g.Patch("/orgs/:id/plan", func(c *fiber.Ctx) error {
		return s.forward(c, fiber.MethodPatch, "/internal/orgs/"+c.Params("id")+"/plan", c.Body())
	})
	g.Get("/orgs/:id/uso", func(c *fiber.Ctx) error {
		return s.forward(c, fiber.MethodGet, "/internal/orgs/"+c.Params("id")+"/uso", nil)
	})
	g.Patch("/empresas/:id/activa", func(c *fiber.Ctx) error {
		return s.forward(c, fiber.MethodPatch, "/internal/empresas/"+c.Params("id")+"/activa", c.Body())
	})
}

// forward hace la petición al core y relaya la respuesta. body nil ⇒ sin cuerpo; si hay
// cuerpo se envía como JSON (es lo que manda el navegador).
func (s *Server) forward(c *fiber.Ctx, method, path string, body []byte) error {
	ct := ""
	if body != nil && len(body) > 0 {
		ct = "application/json"
	} else {
		body = nil
	}
	resp, err := s.core.Forward(c.Context(), method, path, sesionDe(c).Email, body, ct)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "no se pudo contactar el core"})
	}
	return relay(c, resp)
}

// relay copia estado, Content-Type y cuerpo de la respuesta del core al navegador.
func relay(c *fiber.Ctx, resp *core.Resp) error {
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Set(fiber.HeaderContentType, ct)
	}
	return c.Status(resp.Status).Send(resp.Body)
}

// handleRestore reenvía el archivo de respaldo (binario) al core, que crea una empresa
// nueva. Preserva la query (orgId) y manda el cuerpo como octet-stream.
func (s *Server) handleRestore(c *fiber.Ctx) error {
	path := "/internal/restore"
	if q := string(c.Request().URI().QueryString()); q != "" {
		path += "?" + q
	}
	resp, err := s.core.Forward(c.Context(), fiber.MethodPost, path, sesionDe(c).Email, c.Body(), "application/octet-stream")
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "no se pudo contactar el core"})
	}
	return relay(c, resp)
}

// handleExport relaya el respaldo (octet-stream) preservando las cabeceras de descarga.
func (s *Server) handleExport(c *fiber.Ctx) error {
	resp, err := s.core.Forward(c.Context(), fiber.MethodPost, "/internal/tenants/"+c.Params("empresaId")+"/export", sesionDe(c).Email, nil, "")
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "no se pudo contactar el core"})
	}
	for _, h := range []string{fiber.HeaderContentType, fiber.HeaderContentDisposition, "X-Export-Encrypted"} {
		if v := resp.Header.Get(h); v != "" {
			c.Set(h, v)
		}
	}
	return c.Status(resp.Status).Send(resp.Body)
}
