package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fabricacion"
	"github.com/mornix/elerp/internal/domain/usuario"
)

/* FABRICACIÓN — superficie HTTP.
 *
 * Planificar lo ve cualquiera de la operación: saber qué se está produciendo y
 * si alcanzan los insumos es información de todos los días. MOVER la orden
 * —arrancar, terminar, cancelar— toca el inventario, así que va con el mismo
 * gate que el resto de la escritura de inventario.
 */
func (s *Server) registrarFabricacion(api fiber.Router) {
	g := api.Group("/fabricacion")
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor,
		usuario.RolCajero, usuario.RolContadora)

	g.Get("/ordenes", ver, s.handleOrdenesFabricacion)
	// El resumen del período: orden por orden no se ve que veinte tandas pierdan
	// tres cada una, y eso es lo que hay que ver.
	g.Get("/resumen", ver, s.handleResumenFabricacion)
	g.Get("/ordenes/:id", ver, s.handleOrdenFabricacion)
	// Planear NO escribe: es lo que deja ver si alcanza y cuánto va a costar
	// ANTES de sacar los insumos del almacén.
	g.Post("/planear", ver, s.handlePlanearOrden)
	// El catálogo de destinos lo da el servidor, que es quien valida: tenerlo
	// escrito en la pantalla se desincroniza.
	g.Get("/destinos", ver, s.handleDestinosFabricacion)

	g.Post("/ordenes", s.escribirInventario, s.handleCrearOrdenFabricacion)
	g.Post("/ordenes/:id/iniciar", s.escribirInventario, s.handleIniciarOrden)
	g.Post("/ordenes/:id/terminar", s.escribirInventario, s.handleTerminarOrden)
	g.Post("/ordenes/:id/cancelar", s.escribirInventario, s.handleCancelarOrden)
}

func (s *Server) handleOrdenesFabricacion(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"ordenes": s.svc.OrdenesFabricacion(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handleOrdenFabricacion(c *fiber.Ctx) error {
	o, ok := s.svc.OrdenFabricacion(empresaIDOf(c), c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "la orden no existe"})
	}
	return c.JSON(o)
}

func (s *Server) handlePlanearOrden(c *fiber.Ctx) error {
	var in struct {
		SKU      string  `json:"sku"`
		Cantidad float64 `json:"cantidad"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	plan, err := s.svc.PlanearOrden(empresaIDOf(c), sedeIDOf(c), in.SKU, in.Cantidad)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(plan)
}

func (s *Server) handleCrearOrdenFabricacion(c *fiber.Ctx) error {
	var in struct {
		SKU          string  `json:"sku"`
		Cantidad     float64 `json:"cantidad"`
		AlmacenID    string  `json:"almacenId"`
		Lote         string  `json:"lote"`
		Vencimiento  string  `json:"vencimiento"`
		PesoUnitario float64 `json:"pesoUnitario"`
		OrigenTipo   string  `json:"origenTipo"`
		OrigenID     string  `json:"origenId"`
		Nota         string  `json:"nota"`
		/* Iniciar arranca la orden en el mismo paso. Lo normal en un taller es
		 * planificar y empezar de inmediato: separarlo en dos clics es hacerle
		 * teclear al operario una transición que ya decidió. Queda separable porque
		 * a veces sí se planifica para después. */
		Iniciar bool `json:"iniciar"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearOrdenFabricacion(empresaIDOf(c), application.EntradaOrden{
		SedeID: sedeIDOf(c), AlmacenID: in.AlmacenID, SKU: in.SKU, Cantidad: in.Cantidad,
		Lote: in.Lote, Vencimiento: in.Vencimiento, PesoUnitario: in.PesoUnitario,
		OrigenTipo: in.OrigenTipo, OrigenID: in.OrigenID, Nota: in.Nota,
		Actor: principalOf(c).UserID, Origen: origen(c),
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if in.Iniciar {
		/* Si arrancar falla —no alcanzan los insumos— la orden YA quedó creada y se
		 * devuelve con el motivo. No se deshace: planificarla igual es útil, y
		 * borrarla obligaría a rehacerla cuando llegue la materia prima. */
		iniciada, errIni := s.svc.IniciarOrden(empresaIDOf(c), out.ID, principalOf(c).UserID, origen(c))
		if errIni != nil {
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{
				"orden": out, "avisoInicio": errIni.Error(),
			})
		}
		out = iniciada
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleResumenFabricacion(c *fiber.Ctx) error {
	return c.JSON(s.svc.ResumenDeFabricacion(empresaIDOf(c), sedeIDOf(c),
		c.Query("desde"), c.Query("hasta")))
}

func (s *Server) handleIniciarOrden(c *fiber.Ctx) error {
	out, err := s.svc.IniciarOrden(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleTerminarOrden(c *fiber.Ctx) error {
	var in struct {
		// Producida es lo que salió BIEN. Cero es una respuesta válida: la tanda se
		// perdió entera.
		Producida float64 `json:"producida"`
		// Resultados reparte lo que NO salió bien entre sus destinos: de una tanda
		// de 15 pueden salir 10 buenos, 3 perdidos y 2 para reprocesar.
		Resultados []fabricacion.Resultado `json:"resultados"`
	}
	_ = c.BodyParser(&in)
	out, err := s.svc.CerrarOrden(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c),
		application.CierreOrden{Producida: in.Producida, Resultados: in.Resultados})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleCancelarOrden(c *fiber.Ctx) error {
	var in struct {
		Motivo string `json:"motivo"`
	}
	_ = c.BodyParser(&in)
	out, err := s.svc.CancelarOrden(empresaIDOf(c), c.Params("id"), in.Motivo, principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleDestinosFabricacion lista a dónde puede ir lo que no salió bien. Qué
// destinos tienen sentido depende del negocio —una panadería recicla, una
// farmacia destruye— pero el catálogo es uno solo y lo valida el servidor.
func (s *Server) handleDestinosFabricacion(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"destinos": []fiber.Map{
		{"codigo": fabricacion.DestinoPerdida, "nombre": "Pérdida",
			"ayuda": "No queda nada: se derramó, se quemó, se evaporó."},
		{"codigo": fabricacion.DestinoDescarte, "nombre": "Descarte",
			"ayuda": "Existe y no se puede vender. Va al almacén de descarte hasta que se destruya."},
		{"codigo": fabricacion.DestinoReproceso, "nombre": "Para reprocesar",
			"ayuda": "Sirve para volver a entrar a producción."},
	}})
}
