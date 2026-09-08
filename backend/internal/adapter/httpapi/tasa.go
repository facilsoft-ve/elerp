package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerTasa monta las rutas de la tasa de cambio.
//
// La lectura es abierta a todos los roles del tenant (el cajero necesita saber a
// qué tasa está cobrando). Escribir —cargar a mano, forzar una consulta, aprobar
// una cuarentena— es de Dueña/Desarrollador: es una decisión que mueve todos los
// precios de la empresa.
func (s *Server) registerTasa(r fiber.Router) {
	admin := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	r.Get("/tasa", s.handleTasa)
	r.Get("/tasas", s.handleTasas)
	r.Get("/tasa/historial", s.requireRoles(
		usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora,
	), s.handleTasaHistorial)
	r.Post("/tasa/manual", admin, s.handleTasaManual)
	r.Post("/tasa/sincronizar", admin, s.handleTasaSincronizar)
	r.Post("/tasa/cuarentena/:id/aprobar", admin, s.handleTasaAprobar)

	// Configuración de la empresa: moneda (R10) y seguridad de caja (flujo 2.4).
	r.Put("/empresa/config/moneda", admin, s.handleConfigMoneda)
	r.Put("/empresa/config/seguridad", admin, s.handleConfigSeguridad)

	// Usuarios y roles (Configuración). Ver es de Dueña/Desarrollador/Contadora;
	// cambiar rol o sede, solo administración.
	r.Get("/usuarios", s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora), s.handleUsuarios)
	r.Patch("/usuarios/:id", admin, s.handleGuardarUsuario)
}

// handleTasa devuelve la tasa vigente del DÓLAR ya rotulada (valor, origen y
// fecha). Sin cambios: es el flujo USD histórico.
func (s *Server) handleTasa(c *fiber.Ctx) error {
	return c.JSON(s.svc.TasaDeEmpresa(empresaIDOf(c)))
}

// handleTasas devuelve la tasa vigente de cada divisa activa (selector
// multimoneda) junto con la lista de divisas activas de la empresa.
func (s *Server) handleTasas(c *fiber.Ctx) error {
	emp := empresaIDOf(c)
	return c.JSON(fiber.Map{
		"tasas":   s.svc.TasasVigentes(emp),
		"activas": s.svc.MonedasActivas(emp),
	})
}

// handleTasaHistorial devuelve el histórico, incluidas las lecturas rechazadas.
func (s *Server) handleTasaHistorial(c *fiber.Ctx) error {
	return c.JSON(s.svc.HistorialTasa(empresaIDOf(c), c.QueryInt("limite", 30)))
}

// handleTasaManual registra una tasa cargada a mano (último recurso de R9).
func (s *Server) handleTasaManual(c *fiber.Ctx) error {
	var in struct {
		Moneda     string  `json:"moneda"`
		Valor      float64 `json:"valor"`
		FechaValor string  `json:"fechaValor"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	// Moneda por defecto "USD" (retrocompat: el POS/cliente actual no la manda).
	t, err := s.svc.CargarTasaManualEnMoneda(empresaIDOf(c), principalOf(c).UserID, origen(c), in.Moneda, in.Valor, in.FechaValor)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"tasa": t, "vigente": s.svc.TasaDeEmpresa(empresaIDOf(c)),
	})
}

// handleTasaSincronizar fuerza un intento de obtención. Respeta el intervalo
// mínimo entre consultas: la condición 7 vale también para el botón.
func (s *Server) handleTasaSincronizar(c *fiber.Ctx) error {
	vista, err := s.svc.SincronizarTasa(c.Context(), principalOf(c).UserID, origen(c), true)
	if err != nil {
		// 429: no es un error del usuario, es una espera.
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"error": err.Error(), "vigente": vista,
		})
	}
	return c.JSON(vista)
}

// handleTasaAprobar aplica una lectura que había quedado en cuarentena.
func (s *Server) handleTasaAprobar(c *fiber.Ctx) error {
	// Immutable está activo en Fiber, pero copiar el parámetro es explícito:
	// este id no se guarda, se consulta.
	id := c.Params("id")
	if _, err := s.svc.AprobarTasaEnCuarentena(empresaIDOf(c), principalOf(c).UserID, origen(c), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(s.svc.TasaDeEmpresa(empresaIDOf(c)))
}

// handleConfigSeguridad activa o desactiva el PIN de supervisor de caja.
func (s *Server) handleConfigSeguridad(c *fiber.Ctx) error {
	var in struct {
		RequiereSupervisorPin bool `json:"requiereSupervisorPin"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	emp, err := s.svc.GuardarConfigSeguridad(empresaIDOf(c), principalOf(c).UserID, origen(c), in.RequiereSupervisorPin)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(emp)
}

// handleUsuarios lista los miembros de la empresa con su rol y su sede.
func (s *Server) handleUsuarios(c *fiber.Ctx) error {
	return c.JSON(s.tenancy.Miembros(empresaIDOf(c)))
}

// handleGuardarUsuario cambia el rol y la sede de un miembro.
func (s *Server) handleGuardarUsuario(c *fiber.Ctx) error {
	var in struct {
		Rol    string `json:"rol"`
		SedeID string `json:"sedeId"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	// c.Params con Immutable: no se guarda tal cual, se usa para buscar.
	out, err := s.tenancy.GuardarMiembro(empresaIDOf(c), c.Params("id"), in.Rol, in.SedeID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	s.svc.AuditarCambioDeRol(empresaIDOf(c), principalOf(c).UserID, origen(c), out.Nombre, out.Rol, out.SedeID)
	return c.JSON(out)
}

// handleConfigMoneda guarda moneda principal, fuente de tasa y precios en US$.
func (s *Server) handleConfigMoneda(c *fiber.Ctx) error {
	var in application.ConfigMoneda
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	// Copias de los strings del cuerpo no hacen falta (BodyParser ya copia),
	// pero la empresa sí se toma del contexto validado, nunca del cuerpo.
	emp, err := s.svc.GuardarConfigMoneda(empresaIDOf(c), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"empresa": emp, "tasa": s.svc.TasaDeEmpresa(empresaIDOf(c))})
}
