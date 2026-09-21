package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/almacen"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerAlmacenes monta el maestro de Almacenes (Configuración). Un almacén
// pertenece a una sede; una sede tiene ≥1 almacén y exactamente uno principal (del
// que despacha el POS). Maestro editable con soft-disable, mismo gate que el resto
// de Configuración: lo ve quien accede a Ajustes (Dueña/Desarrollador/Contadora),
// solo Dueña/Desarrollador lo modifica.
func (s *Server) registerAlmacenes(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolContadora)
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/config/almacenes", ver)
	g.Get("/", s.handleAlmacenes)
	g.Post("/", edit, s.handleCrearAlmacen)
	g.Patch("/:id", edit, s.handleActualizarAlmacen)
	g.Post("/:id/desactivar", edit, s.handleDesactivarAlmacen)

	// UBICACIONES dentro del almacén (pasillo, estante, muelle). Editables como el
	// almacén: sin borrado duro, se desactivan.
	g.Get("/:id/ubicaciones", s.handleUbicaciones)
	g.Post("/:id/ubicaciones", edit, s.handleCrearUbicacion)
	g.Patch("/:id/ubicaciones/:ubicacionId", edit, s.handleActualizarUbicacion)

	// TIPOS DE OPERACIÓN: en cuántos pasos entra y sale la mercancía. Cuelgan de
	// Configuración y no de un almacén, porque un tipo vale para toda la sede.
	o := r.Group("/config/operaciones", ver)
	o.Get("/", s.handleTiposOperacion)
	o.Post("/", edit, s.handleCrearTipoOperacion)
	o.Post("/sembrar", edit, s.handleSembrarTiposOperacion)
	o.Patch("/:id", edit, s.handleActualizarTipoOperacion)
}

func (s *Server) handleTiposOperacion(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"operaciones": s.svc.TiposOperacion(empresaIDOf(c))})
}

// tipoOperacionBody es el cuerpo de alta y edición. `pasos` llega como número: 0
// significa «no lo toques» al editar, y al crear se lee como uno.
type tipoOperacionBody struct {
	Codigo                string `json:"codigo"`
	Nombre                string `json:"nombre"`
	Clase                 string `json:"clase"`
	SedeID                string `json:"sedeId"`
	Pasos                 int    `json:"pasos"`
	AlmacenID             string `json:"almacenId"`
	UbicacionIntermediaID string `json:"ubicacionIntermediaId"`
	PorDefecto            bool   `json:"porDefecto"`
	Activo                bool   `json:"activo"`
}

func (in tipoOperacionBody) aDominio() almacen.TipoOperacion {
	return almacen.TipoOperacion{
		Codigo: in.Codigo, Nombre: in.Nombre, Clase: in.Clase, SedeID: in.SedeID,
		Pasos: in.Pasos, AlmacenID: in.AlmacenID,
		UbicacionIntermediaID: in.UbicacionIntermediaID,
		PorDefecto:            in.PorDefecto, Activo: in.Activo,
	}
}

func (s *Server) handleCrearTipoOperacion(c *fiber.Ctx) error {
	var in tipoOperacionBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearTipoOperacion(empresaIDOf(c), principalOf(c).UserID, origen(c), in.aDominio())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarTipoOperacion(c *fiber.Ctx) error {
	var in tipoOperacionBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarTipoOperacion(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.aDominio())
	if err != nil {
		estado := fiber.StatusBadRequest
		if errors.Is(err, application.ErrOperacionNoExiste) {
			estado = fiber.StatusNotFound
		}
		return c.Status(estado).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleSembrarTiposOperacion crea los cuatro por defecto, todos de un paso. Es
// idempotente: si ya hay tipos, no toca nada y devuelve 0.
func (s *Server) handleSembrarTiposOperacion(c *fiber.Ctx) error {
	n := s.svc.SembrarTiposOperacion(empresaIDOf(c), principalOf(c).UserID, origen(c))
	return c.JSON(fiber.Map{"creados": n, "operaciones": s.svc.TiposOperacion(empresaIDOf(c))})
}

func (s *Server) handleUbicaciones(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"ubicaciones": s.svc.UbicacionesDe(empresaIDOf(c), c.Params("id"))})
}

func (s *Server) handleCrearUbicacion(c *fiber.Ctx) error {
	var in struct {
		Codigo string `json:"codigo"`
		Nombre string `json:"nombre"`
		Tipo   string `json:"tipo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearUbicacion(empresaIDOf(c), principalOf(c).UserID, origen(c), almacen.Ubicacion{
		AlmacenID: c.Params("id"), Codigo: in.Codigo, Nombre: in.Nombre, Tipo: in.Tipo,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarUbicacion(c *fiber.Ctx) error {
	var in struct {
		Nombre string `json:"nombre"`
		Tipo   string `json:"tipo"`
		Activa bool   `json:"activa"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarUbicacion(empresaIDOf(c), c.Params("ubicacionId"), principalOf(c).UserID, origen(c),
		almacen.Ubicacion{Nombre: in.Nombre, Tipo: in.Tipo, Activa: in.Activa})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleAlmacenes(c *fiber.Ctx) error {
	// Devuelve los almacenes CON su ocupación (para la barra de llenado). Opcional
	// ?sede= filtra a una sede.
	todos := s.svc.AlmacenesConOcupacion(empresaIDOf(c))
	if sedeID := c.Query("sede"); sedeID != "" {
		filtrados := make([]application.AlmacenView, 0, len(todos))
		for _, a := range todos {
			if a.SedeID == sedeID {
				filtrados = append(filtrados, a)
			}
		}
		return c.JSON(filtrados)
	}
	return c.JSON(todos)
}

// almacenBody es el cuerpo de creación/edición. `activo` va como puntero para
// soportar un PATCH parcial: nil = no enviado = no cambiar el estado.
type almacenBody struct {
	SedeID          string   `json:"sedeId"`
	Nombre          string   `json:"nombre"`
	Tipo            string   `json:"tipo"`
	Principal       bool     `json:"principal"`
	Activo          *bool    `json:"activo"`
	Capacidad       float64  `json:"capacidad"`
	CapacidadUnidad string   `json:"capacidadUnidad"`
	RubrosAdmitidos []string `json:"rubrosAdmitidos"`
}

func (b almacenBody) modelo() almacen.Almacen {
	return almacen.Almacen{
		SedeID: b.SedeID, Nombre: b.Nombre, Tipo: b.Tipo, Principal: b.Principal,
		Capacidad: b.Capacidad, CapacidadUnidad: b.CapacidadUnidad, RubrosAdmitidos: b.RubrosAdmitidos,
	}
}

func (s *Server) handleCrearAlmacen(c *fiber.Ctx) error {
	var in almacenBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	// La sede debe pertenecer al tenant (defensa cross-tenant además de la del repo).
	if !s.tenancy.SedeValida(empresaIDOf(c), in.SedeID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "la sede no existe en esta empresa"})
	}
	out, err := s.svc.CrearAlmacen(empresaIDOf(c), principalOf(c).UserID, origen(c), in.modelo())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarAlmacen(c *fiber.Ctx) error {
	var in almacenBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarAlmacen(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.modelo(), in.Activo)
	if err != nil {
		if errors.Is(err, application.ErrAlmacenNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleDesactivarAlmacen(c *fiber.Ctx) error {
	if err := s.svc.DesactivarAlmacen(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c)); err != nil {
		if errors.Is(err, application.ErrAlmacenNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
