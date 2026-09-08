package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerAplicaciones monta la sección "Aplicaciones": el catálogo de módulos con
// su estado por empresa y las acciones instalar/activar/desactivar. Lo ve quien
// accede a Ajustes; solo Dueña/Desarrollador lo administra.
func (s *Server) registerAplicaciones(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/config/aplicaciones", ver)
	g.Get("/", s.handleAplicaciones)
	g.Post("/:id/instalar", edit, s.handleModuloAccion(s.svc.Instalar))
	g.Post("/:id/desinstalar", edit, s.handleModuloAccion(s.svc.Desinstalar))
	g.Post("/:id/activar", edit, s.handleModuloAccion(s.svc.Activar))
	g.Post("/:id/desactivar", edit, s.handleModuloAccion(s.svc.Desactivar))
}

func (s *Server) handleAplicaciones(c *fiber.Ctx) error {
	return c.JSON(s.svc.Modulos(empresaIDOf(c)))
}

// requireModulo es el gating de servidor por módulo: 403 si el módulo no está
// activo para la empresa. Lo usan las rutas de los módulos comercializables (p. ej.
// Promociones exige el módulo Marketing). Es la contraparte servidor del ocultado
// que hace la UI con db.MODULOS — "la UI solo oculta, el servidor protege".
func (s *Server) requireModulo(moduloID string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !s.svc.ModuloActivo(empresaIDOf(c), moduloID) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "el módulo requerido no está activo para esta empresa"})
		}
		return c.Next()
	}
}

// handleModuloAccion adapta las cuatro acciones (misma firma) a un handler HTTP.
func (s *Server) handleModuloAccion(accion func(empresaID, moduloID, actor, origen string) (aplicacion.Instalacion, error)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		_, err := accion(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
		if err != nil {
			if errors.Is(err, application.ErrModuloNoExiste) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		// Se devuelve el catálogo con estado actualizado, para que el front repinte.
		return c.JSON(s.svc.Modulos(empresaIDOf(c)))
	}
}
