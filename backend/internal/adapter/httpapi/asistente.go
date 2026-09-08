package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerAsistente monta el asistente (módulo "asistente-ia"). Todo el grupo exige el
// módulo activo (gating de servidor): sin instalarlo, /ai/* da 403. La capa MECÁNICA
// responde local; la capa IA es opt-in por empresa (ver application/asistente.go).
func (s *Server) registerAsistente(r fiber.Router) {
	g := r.Group("/ai", s.requireModulo(aplicacion.ModAsistenteIA))
	// Preguntar: cualquier rol con la app (el servicio acota la respuesta al rol).
	g.Post("/ask", s.handleAIAsk)
	// Configuración de la capa IA (opt-in + modelo).
	g.Get("/config", s.handleAsistenteConfig)
	g.Patch("/config", s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador), s.handleConfigurarAsistente)
}

type aiAskBody struct {
	Message string `json:"message"`
}

// handleAIAsk responde una pregunta en dos capas: primero MECÁNICA (local, exacta,
// acotada al rol); si ningún intent aplica y la empresa habilitó la IA, escala a la
// capa IA; si tampoco, devuelve "sin_respuesta" con una guía honesta.
func (s *Server) handleAIAsk(c *fiber.Ctx) error {
	var in aiAskBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	empresaID, rol := empresaIDOf(c), rolOf(c)

	// 1) Capa mecánica (siempre, 100% local).
	if resp, ok := s.svc.ResponderMecanica(empresaID, rol, in.Message); ok {
		return c.JSON(resp)
	}

	// 2) Capa IA (solo si la empresa la habilitó y hay proxy cableado).
	resp, err := s.svc.ResponderIA(c.Context(), empresaID, rol, in.Message)
	if err == nil {
		return c.JSON(resp)
	}
	if !errors.Is(err, application.ErrIANoDisponible) {
		// La IA está habilitada pero el proveedor falló: honesto, no inventamos.
		return c.JSON(application.RespuestaAsistente{
			Tipo:      application.RespSinRespuesta,
			Respuesta: "No pude consultar la IA en este momento. Reintenta en un momento, o reformula tu pregunta como una consulta de datos (ventas, cartera, stock, por pagar).",
		})
	}

	// 3) Sin IA habilitada: guía al usuario.
	return c.JSON(application.RespuestaAsistente{
		Tipo:      application.RespSinRespuesta,
		Respuesta: "No puedo responder eso automáticamente. Puedo con tus datos —ventas del mes, cartera vencida, stock de un producto, cuentas por pagar, si el libro cuadra— y dudas de uso (IGTF, transferir stock, retenciones). Para preguntas abiertas, activa la IA en Configuración › Asistente.",
	})
}

func (s *Server) handleAsistenteConfig(c *fiber.Ctx) error {
	return c.JSON(s.svc.AsistenteConfigDe(empresaIDOf(c)))
}

type asistenteConfigBody struct {
	Habilitada bool   `json:"habilitada"`
	Modelo     string `json:"modelo"`
}

func (s *Server) handleConfigurarAsistente(c *fiber.Ctx) error {
	var in asistenteConfigBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo inválido"})
	}
	cfg, err := s.svc.ConfigurarAsistente(empresaIDOf(c), principalOf(c).UserID, origen(c), in.Habilitada, in.Modelo)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(cfg)
}
