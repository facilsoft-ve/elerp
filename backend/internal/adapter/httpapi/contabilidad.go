package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/usuario"
)

/* registerContabilidad monta el módulo de Contabilidad.
 *
 * Es el módulo de la Contadora: ella tiene acceso total acá (junto con Dueña y
 * Desarrollador). Nadie más entra — el vendedor no ve costos ni contabilidad, y
 * eso lo aplica el servidor, no la interfaz.
 *
 * Todo es de lectura salvo revertir un asiento, porque los asientos se DERIVAN de
 * las operaciones: no hay captura manual que pueda contradecir al fiscal.
 */
func (s *Server) registerContabilidad(r fiber.Router) {
	acceso := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)

	g := r.Group("/contabilidad", acceso)
	g.Get("/plan", s.handlePlanDeCuentas)
	// Edición del plan de cuentas (maestro editable; mismo gate del grupo). No hay
	// borrado: solo desactivar. El código es inmutable.
	g.Post("/plan", s.handleCrearCuenta)
	g.Patch("/plan/:codigo", s.handleRenombrarCuenta)
	g.Post("/plan/:codigo/activar", s.handleReactivarCuenta)
	g.Post("/plan/:codigo/desactivar", s.handleDesactivarCuenta)
	g.Get("/diario", s.handleLibroDiario)
	// Asiento MANUAL: único punto de captura a mano del libro. Reservado a quien
	// gestiona la contabilidad (Dueña y Contadora), como el cierre de período.
	g.Post("/diario", s.requireRoles(usuario.RolDueno, usuario.RolContadora), s.handleCrearAsientoManual)
	g.Get("/balance", s.handleBalance)
	g.Get("/resultados", s.handleResultados)
	// Sello de integridad: recalcula el hash-encadenado de asientos y documentos.
	g.Get("/integridad", s.handleIntegridad)
	g.Post("/diario/:id/revertir", s.handleRevertirAsiento)

	// Cierre de período (§7.3). Listar es lectura (mismo gate del grupo). Cerrar es
	// una escritura reservada a quien gestiona la contabilidad: Dueña y Contadora.
	g.Get("/periodos", s.handlePeriodosCerrados)
	g.Post("/periodos/cerrar", s.requireRoles(usuario.RolDueno, usuario.RolContadora), s.handleCerrarPeriodo)
}

// cuentaView es una cuenta del plan con el flag `base` derivado (las cuentas base
// del sistema no se pueden desactivar; la UI las marca y bloquea esa acción).
type cuentaView struct {
	contabilidad.Cuenta
	Base bool `json:"base"`
}

func (s *Server) handlePlanDeCuentas(c *fiber.Ctx) error {
	plan := s.svc.PlanDeCuentas(empresaIDOf(c))
	out := make([]cuentaView, 0, len(plan))
	for _, ct := range plan {
		out = append(out, cuentaView{Cuenta: ct, Base: application.EsCuentaBase(ct.Codigo)})
	}
	return c.JSON(out)
}

func (s *Server) handleCrearCuenta(c *fiber.Ctx) error {
	var in struct{ Codigo, Nombre, Tipo, CodigoPadre string }
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	ct, err := s.svc.CrearCuenta(empresaIDOf(c), in.Codigo, in.Nombre, in.Tipo, in.CodigoPadre, principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(cuentaView{Cuenta: ct, Base: application.EsCuentaBase(ct.Codigo)})
}

func (s *Server) handleRenombrarCuenta(c *fiber.Ctx) error {
	var in struct{ Nombre string }
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	ct, err := s.svc.RenombrarCuenta(empresaIDOf(c), c.Params("codigo"), in.Nombre, principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(cuentaView{Cuenta: ct, Base: application.EsCuentaBase(ct.Codigo)})
}

func (s *Server) handleReactivarCuenta(c *fiber.Ctx) error { return s.fijarCuentaActiva(c, true) }
func (s *Server) handleDesactivarCuenta(c *fiber.Ctx) error { return s.fijarCuentaActiva(c, false) }

func (s *Server) fijarCuentaActiva(c *fiber.Ctx, activa bool) error {
	ct, err := s.svc.FijarCuentaActiva(empresaIDOf(c), c.Params("codigo"), activa, principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(cuentaView{Cuenta: ct, Base: application.EsCuentaBase(ct.Codigo)})
}

func (s *Server) handleLibroDiario(c *fiber.Ctx) error {
	return c.JSON(s.svc.LibroDiario(empresaIDOf(c)))
}

func (s *Server) handleBalance(c *fiber.Ctx) error {
	return c.JSON(s.svc.Balance(empresaIDOf(c)))
}

func (s *Server) handleResultados(c *fiber.Ctx) error {
	// P&G del período: ?desde=&hasta= (YYYY-MM-DD). Sin ellos, acumulado desde el inicio.
	return c.JSON(s.svc.Resultados(empresaIDOf(c), c.Query("desde"), c.Query("hasta")))
}

// handlePeriodosCerrados lista los cierres de mes de la empresa.
func (s *Server) handlePeriodosCerrados(c *fiber.Ctx) error {
	return c.JSON(s.svc.PeriodosCerrados(empresaIDOf(c)))
}

// handleCerrarPeriodo cierra un mes contable. El cierre es DEFINITIVO (§7.3): no
// hay ruta de reapertura.
func (s *Server) handleCerrarPeriodo(c *fiber.Ctx) error {
	var in struct {
		Anio int `json:"anio"`
		Mes  int `json:"mes"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "año y mes requeridos"})
	}
	out, err := s.svc.CerrarPeriodo(empresaIDOf(c), principalOf(c).UserID, origen(c), in.Anio, in.Mes)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleIntegridad recalcula el sello de integridad (hash-encadenado) del libro
// diario y del libro de documentos, y devuelve el veredicto de cada uno.
func (s *Server) handleIntegridad(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"libros": s.svc.VerificarIntegridad(empresaIDOf(c))})
}

// handleCrearAsientoManual anexa un asiento tecleado a mano (tiene que cuadrar).
func (s *Server) handleCrearAsientoManual(c *fiber.Ctx) error {
	var in struct {
		Fecha       string `json:"fecha"`
		Descripcion string `json:"descripcion"`
		Lineas      []struct {
			Codigo string  `json:"codigo"`
			Debe   float64 `json:"debe"`
			Haber  float64 `json:"haber"`
		} `json:"lineas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	lineas := make([]application.LineaAsientoManual, 0, len(in.Lineas))
	for _, l := range in.Lineas {
		lineas = append(lineas, application.LineaAsientoManual{Codigo: l.Codigo, Debe: l.Debe, Haber: l.Haber})
	}
	a, err := s.svc.RegistrarAsientoManual(empresaIDOf(c), principalOf(c).UserID, origen(c), in.Fecha, in.Descripcion, lineas)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(a)
}

// handleRevertirAsiento anexa el asiento contrario. El original queda intacto.
func (s *Server) handleRevertirAsiento(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "motivo requerido"})
	}
	out, err := s.svc.RevertirAsiento(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.Motivo)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}
