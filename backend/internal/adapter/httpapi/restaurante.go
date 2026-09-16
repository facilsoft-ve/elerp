package httpapi

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/mesonero"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// registerRestaurante monta el módulo Restaurante: el maestro de MESAS del salón
// (con su mapa) y la configuración de la IMPRESORA de comandas. El mapa lo VE
// cualquier rol que opere el salón (para seleccionar mesa); lo EDITA la Dueña/
// Desarrollador. La impresora la configura la Dueña/Desarrollador.
func (s *Server) registerRestaurante(r fiber.Router) {
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor, usuario.RolCajero, usuario.RolContadora, usuario.RolMesonero)
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/restaurante", ver)
	g.Get("/mesas", s.handleMesas)
	g.Post("/mesas", edit, s.handleCrearMesa)
	g.Patch("/mesas/:id", edit, s.handleActualizarMesa)
	g.Delete("/mesas/:id", edit, s.handleEliminarMesa)
	g.Post("/mapa", edit, s.handleGuardarMapa)
	g.Get("/plano", s.handlePlano)
	g.Put("/plano", edit, s.handleGuardarPlano)

	// Asignación de mesas/zonas a mesoneros y configuración del módulo. Ver es para todo
	// el salón (el mesonero necesita saber qué mesas son suyas); editar, solo la
	// administración de la sede.
	g.Get("/asignaciones", s.handleAsignaciones)
	g.Put("/asignaciones", edit, s.handleGuardarAsignacion)
	g.Get("/config", s.handleConfigSalon)
	g.Put("/config", edit, s.handleGuardarConfigSalon)

	// Comanderas: varias por sede (cocina, barra, postres), cada una con sus rubros.
	g.Get("/impresoras", s.handleImpresoras)
	g.Put("/impresoras", edit, s.handleGuardarImpresora)
	g.Delete("/impresoras/:id", edit, s.handleEliminarImpresora)

	// Cuentas de mesa (comandera): abrir, agregar renglones, enviar a cocina,
	// cancelar/marcar renglón, cerrar. Las opera cualquier rol del salón (ver).
	g.Get("/cuentas", s.handleCuentas)
	g.Post("/cuentas", s.handleAbrirCuenta)
	g.Get("/cuentas/:id", s.handleCuenta)
	g.Post("/cuentas/:id/items", s.handleAgregarItems)
	g.Post("/cuentas/:id/enviar", s.handleEnviarCocina)
	g.Delete("/cuentas/:id/items/:itemId", s.handleCancelarItem)
	g.Post("/cuentas/:id/items/:itemId/estado", s.handleMarcarItem)
	g.Post("/cuentas/:id/cerrar", s.handleCerrarCuenta)
	// Prefactura: el mesonero pide la cuenta y la convierte en cotización(es)
	// confirmadas rotuladas con la mesa. El cajero las cobra desde Ventas.
	g.Post("/cuentas/:id/prefacturar", s.handlePrefacturarCuenta)
	g.Post("/cuentas/:id/prefacturar/cancelar", s.handleCancelarPrefactura)
	// Cobro → factura desde la cuenta. Emitir factura es de roles con caja/venta
	// (no la contadora, que es consulta fuera de Contabilidad/Tesorería).
	emitir := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolCajero, usuario.RolVendedor)
	g.Get("/cuentas/:id/preview-cobro", s.handlePreviewCobro)
	g.Post("/cuentas/:id/cobrar", emitir, s.handleCobrarCuenta)

	// Reservaciones del salón. Las maneja el salón entero —quien recibe en la puerta
	// suele ser un mesonero— salvo la contadora, que es consulta.
	reservas := s.requireRoles(application.RolesReservas...)
	g.Get("/reservas", reservas, s.handleReservas)
	g.Post("/reservas", reservas, s.handleCrearReserva)
	g.Patch("/reservas/:id", reservas, s.handleActualizarReserva)
	g.Post("/reservas/:id/estado", reservas, s.handleEstadoReserva)
	g.Post("/reservas/:id/sentar", reservas, s.handleSentarReserva)
	// Mesas apartadas AHORA: lo que el tablero pinta como «reservada».
	g.Get("/reservas/mesas", reservas, s.handleMesasReservadas)
}

// handleReservas devuelve la agenda. `fecha` (YYYY-MM-DD) elige el día —vacío = hoy—,
// `q` filtra por nombre o cédula (la búsqueda de la puerta) y `proximas=1` trae de hoy
// en adelante, que es la agenda con la que trabaja el salón.
func (s *Server) handleReservas(c *fiber.Ctx) error {
	emp, sede := empresaIDOf(c), sedeIDOf(c)
	if c.Query("proximas") == "1" {
		return c.JSON(s.svc.ReservasProximas(emp, sede))
	}
	return c.JSON(s.svc.BuscarReservas(emp, sede, c.Query("fecha"), c.Query("q")))
}

func (s *Server) handleMesasReservadas(c *fiber.Ctx) error {
	return c.JSON(s.svc.MesasReservadasAhora(empresaIDOf(c), sedeIDOf(c)))
}

// reservaBody es el cuerpo de creación/edición de una reserva.
type reservaBody struct {
	Fecha     string `json:"fecha"`
	Hora      string `json:"hora"`
	Personas  int    `json:"personas"`
	Nombre    string `json:"nombre"`
	Documento string `json:"documento"`
	Telefono  string `json:"telefono"`
	MesaID    string `json:"mesaId"`
	Zona      string `json:"zona"`
	Nota      string `json:"nota"`
}

func (b reservaBody) entrada(c *fiber.Ctx) application.EntradaReserva {
	return application.EntradaReserva{
		EmpresaID: empresaIDOf(c), SedeID: sedeIDOf(c),
		Fecha: b.Fecha, Hora: b.Hora, Personas: b.Personas,
		Nombre: b.Nombre, Documento: b.Documento, Telefono: b.Telefono,
		MesaID: b.MesaID, Zona: b.Zona, Nota: b.Nota,
		Actor: principalOf(c).UserID, Origen: origen(c),
	}
}

func (s *Server) handleCrearReserva(c *fiber.Ctx) error {
	var in reservaBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearReserva(in.entrada(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarReserva(c *fiber.Ctx) error {
	var in reservaBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarReserva(c.Params("id"), in.entrada(c))
	if err != nil {
		return reservaErr(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleEstadoReserva(c *fiber.Ctx) error {
	var in struct {
		Estado string `json:"estado"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CambiarEstadoReserva(empresaIDOf(c), c.Params("id"), in.Estado,
		principalOf(c).UserID, origen(c))
	if err != nil {
		return reservaErr(c, err)
	}
	return c.JSON(out)
}

// handleSentarReserva recibe al cliente: abre su cuenta de mesa y devuelve las dos
// cosas juntas, para que la pantalla pueda saltar directo a tomar el pedido.
func (s *Server) handleSentarReserva(c *fiber.Ctx) error {
	var in struct {
		MesaID string `json:"mesaId"`
	}
	_ = c.BodyParser(&in) // cuerpo opcional: la reserva puede traer ya su mesa
	r, cta, err := s.svc.SentarReserva(empresaIDOf(c), c.Params("id"), in.MesaID,
		principalOf(c).UserID, rolOf(c), origen(c))
	if err != nil {
		return reservaErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"reserva": r, "cuenta": cta})
}

// reservaErr traduce los errores de reservas: 404 lo que no existe, 400 lo demás.
func reservaErr(c *fiber.Ctx, err error) error {
	if errors.Is(err, application.ErrReservaNoExiste) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
}

func (s *Server) handlePreviewCobro(c *fiber.Ctx) error {
	sub, iva, total, err := s.svc.PreviewCobroCuenta(empresaIDOf(c), c.Params("id"))
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(fiber.Map{"subtotal": sub, "iva": iva, "total": total})
}

func (s *Server) handleCobrarCuenta(c *fiber.Ctx) error {
	var in application.CobroCuentaEntrada
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	cta, doc, err := s.svc.CobrarCuenta(empresaIDOf(c), sedeIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"cuenta": cta, "documento": doc})
}

func (s *Server) handleCuentas(c *fiber.Ctx) error {
	return c.JSON(s.svc.CuentasAbiertas(empresaIDOf(c), sedeIDOf(c)))
}

func (s *Server) handleCuenta(c *fiber.Ctx) error {
	out, ok := s.svc.Cuenta(empresaIDOf(c), c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "la cuenta no existe"})
	}
	return c.JSON(out)
}

func (s *Server) handleAbrirCuenta(c *fiber.Ctx) error {
	var in struct {
		MesaID     string `json:"mesaId"`
		Comensales int    `json:"comensales"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	p := principalOf(c)
	out, err := s.svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empresaIDOf(c), SedeID: sedeIDOf(c), MesaID: in.MesaID,
		MesoneroID: p.UserID, MesoneroNombre: p.Nombre,
		RolActor: rolOf(c), Actor: p.UserID, Origen: origen(c), Comensales: in.Comensales,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleAgregarItems(c *fiber.Ctx) error {
	var in struct {
		Items []application.ItemInput `json:"items"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.AgregarItems(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, rolOf(c), origen(c), in.Items)
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleEnviarCocina(c *fiber.Ctx) error {
	cta, tickets, err := s.svc.EnviarACocina(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return cuentaErr(c, err)
	}
	// `tickets` es la comanda ya REPARTIDA por comandera (cocina, barra, postres): la
	// interfaz imprime/muestra un ticket por cada una.
	return c.JSON(fiber.Map{"cuenta": cta, "comanda": fiber.Map{"ronda": cta.UltimaRonda, "tickets": tickets}})
}

func (s *Server) handleCancelarItem(c *fiber.Ctx) error {
	out, err := s.svc.CancelarItem(empresaIDOf(c), c.Params("id"), c.Params("itemId"), principalOf(c).UserID, rolOf(c), origen(c))
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleMarcarItem(c *fiber.Ctx) error {
	var in struct {
		Estado string `json:"estado"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.MarcarItem(empresaIDOf(c), c.Params("id"), c.Params("itemId"), in.Estado, principalOf(c).UserID, origen(c))
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(out)
}

func (s *Server) handleCerrarCuenta(c *fiber.Ctx) error {
	out, err := s.svc.CerrarCuenta(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return cuentaErr(c, err)
	}
	return c.JSON(out)
}

// cuentaErr mapea los errores de cuenta a su código HTTP.
func cuentaErr(c *fiber.Ctx, err error) error {
	if errors.Is(err, application.ErrCuentaMesaNoExiste) || errors.Is(err, application.ErrItemCuentaNoExiste) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
}

func (s *Server) handleMesas(c *fiber.Ctx) error {
	return c.JSON(s.svc.Mesas(empresaIDOf(c), sedeIDOf(c)))
}

// mesaBody es el cuerpo de creación/edición de una mesa. La posición es la celda
// (columna, fila) de la grilla del salón.
type mesaBody struct {
	Nombre    string `json:"nombre"`
	Zona      string `json:"zona"`
	Capacidad int    `json:"capacidad"`
	Forma     string `json:"forma"`
	Columna   int    `json:"columna"`
	Fila      int    `json:"fila"`
	Activa    *bool  `json:"activa"`
}

func (b mesaBody) modelo() mesa.Mesa {
	return mesa.Mesa{
		Nombre: b.Nombre, Zona: b.Zona, Capacidad: b.Capacidad, Forma: b.Forma,
		Columna: b.Columna, Fila: b.Fila,
	}
}

func (s *Server) handleCrearMesa(c *fiber.Ctx) error {
	var in mesaBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CrearMesa(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), in.modelo())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarMesa(c *fiber.Ctx) error {
	var in mesaBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarMesa(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), in.modelo(), in.Activa)
	if err != nil {
		if errors.Is(err, application.ErrMesaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleEliminarMesa(c *fiber.Ctx) error {
	if err := s.svc.EliminarMesa(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c)); err != nil {
		if errors.Is(err, application.ErrMesaNoExiste) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handlePlano(c *fiber.Ctx) error {
	return c.JSON(s.svc.PlanoSalon(empresaIDOf(c), sedeIDOf(c)))
}

func (s *Server) handleGuardarPlano(c *fiber.Ctx) error {
	var in struct {
		Filas      int          `json:"filas"`
		Columnas   int          `json:"columnas"`
		Bloqueadas []mesa.Celda `json:"bloqueadas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.GuardarPlanoSalon(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), in.Filas, in.Columnas, in.Bloqueadas)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleGuardarMapa(c *fiber.Ctx) error {
	var in struct {
		Posiciones []application.PosicionMesa `json:"posiciones"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if err := s.svc.GuardarMapa(empresaIDOf(c), principalOf(c).UserID, origen(c), in.Posiciones); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleImpresoras lista las COMANDERAS de la sede (cocina, barra, postres…).
func (s *Server) handleImpresoras(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"impresoras": s.svc.Impresoras(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handleGuardarImpresora(c *fiber.Ctx) error {
	var in application.ImpresoraBody
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.GuardarImpresora(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleEliminarImpresora(c *fiber.Ctx) error {
	if err := s.svc.EliminarImpresora(empresaIDOf(c), sedeIDOf(c), c.Params("id"),
		principalOf(c).UserID, origen(c)); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// --- Asignación de mesas a mesoneros ---

func (s *Server) handleAsignaciones(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"asignaciones": s.svc.Asignaciones(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handleGuardarAsignacion(c *fiber.Ctx) error {
	var in struct {
		UsuarioID string   `json:"usuarioId"`
		Nombre    string   `json:"nombre"`
		Mesas     []string `json:"mesas"`
		Zonas     []string `json:"zonas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.GuardarAsignacion(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c),
		mesa.Asignacion{UsuarioID: in.UsuarioID, Nombre: in.Nombre, Mesas: in.Mesas, Zonas: in.Zonas})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleConfigSalon(c *fiber.Ctx) error {
	return c.JSON(s.svc.ConfigSalon(empresaIDOf(c), sedeIDOf(c)))
}

func (s *Server) handleGuardarConfigSalon(c *fiber.Ctx) error {
	// Los DOS ajustes son punteros: lo que no venga en el cuerpo no se toca. La
	// ficha se guarda entera, así que con un bool normal mandar solo el modo de
	// horario apagaría la asignación estricta sin que nadie lo pidiera.
	var in struct {
		AsignacionEstricta *bool   `json:"asignacionEstricta"`
		HorarioModo        *string `json:"horarioModo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.GuardarConfigSalonCompleta(empresaIDOf(c), sedeIDOf(c), principalOf(c).UserID, origen(c),
		in.AsignacionEstricta, in.HorarioModo)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// --- Prefactura de la mesa ---

func (s *Server) handlePrefacturarCuenta(c *fiber.Ctx) error {
	var in struct {
		Modo       string            `json:"modo"`
		Comensales int               `json:"comensales"`
		Items      map[string]int    `json:"items"`
		Nombres    map[string]string `json:"nombres"`
		// Seleccion: los renglones que entran en ESTA solicitud (segmentar la mesa).
		// Vacía = todo lo que quede sin pedir.
		Seleccion []string `json:"seleccion"`
		// ClienteID: el mesonero ya tomó los datos de quien paga esta parte.
		ClienteID string `json:"clienteId"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	// Los nombres llegan con clave string (JSON no admite claves numéricas).
	nombres := map[int]string{}
	for k, v := range in.Nombres {
		if n, err := strconv.Atoi(k); err == nil {
			nombres[n] = v
		}
	}
	cta, prefacturas, err := s.svc.PrefacturarCuenta(empresaIDOf(c), c.Params("id"),
		principalOf(c).UserID, origen(c), application.DivisionCuenta{
			Modo: in.Modo, Comensales: in.Comensales, Items: in.Items, Nombres: nombres,
			Seleccion: in.Seleccion, ClienteID: in.ClienteID,
		})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"cuenta": cta, "prefacturas": prefacturas})
}

func (s *Server) handleCancelarPrefactura(c *fiber.Ctx) error {
	out, err := s.svc.CancelarPrefacturasCuenta(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

/* --- Turnos del salón ------------------------------------------------------
 *
 * Credenciales de mesonero (MS-) y sus turnos. Reglas que impone el servidor y
 * que la pantalla solo refleja: un turno lo valida un supervisor con su PIN, y
 * sin turno abierto un mesonero no toma mesas (ver application/mesonero.go).
 *
 * Gate de MÓDULO además del de rol: los turnos son del módulo Restaurante, que
 * se comercializa aparte. Sin él activo, 403.
 */

// registerTurnosSalon monta los turnos. `verTurnos` incluye al mesonero porque
// la grilla de entrada es SU pantalla; administrar credenciales y forzar cierres
// es de la Dueña/Desarrollador.
func (s *Server) registerTurnosSalon(r fiber.Router) {
	modulo := s.requireModulo(aplicacion.ModRestaurante)
	ver := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador, usuario.RolVendedor,
		usuario.RolCajero, usuario.RolContadora, usuario.RolMesonero)
	admin := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)

	g := r.Group("/restaurante/salon", modulo, ver)
	g.Get("/mesoneros", s.handleMesoneros)
	g.Post("/mesoneros", admin, s.handleCrearMesonero)
	g.Patch("/mesoneros/:id", admin, s.handleActualizarMesonero)
	// Fijar el PIN NO es de administración: lo hace la propia persona en la
	// tablet, con un supervisor autorizando con el suyo. Por eso pasa con `ver`.
	g.Post("/mesoneros/:id/pin", s.handleFijarPinMesonero)

	g.Get("/turnos", s.handleTurnosVivos)
	g.Get("/turnos/historial", admin, s.handleHistorialTurnos)
	g.Post("/turnos", s.handleIniciarTurno)
	g.Post("/turnos/:id/finalizar", s.handleFinalizarTurno)
	g.Get("/turnos/:id/relevos", admin, s.handleCandidatosRelevo)
	// Tiempo extra: lo aprueba un supervisor con su PIN. Pasa con `ver` porque se
	// pide desde la tablet del salón, no desde una pantalla de administración.
	g.Post("/turnos/:id/extender", s.handleExtenderTurno)
	// Horarios: los fija quien administra.
	g.Get("/horarios", admin, s.handleHorarios)
	g.Put("/mesoneros/:id/horario", admin, s.handleGuardarHorario)
	g.Post("/turnos/:id/forzar-cierre", admin, s.handleForzarCierreTurno)
}

// estadoTurnoError traduce los errores del turno al código HTTP correcto: un PIN
// equivocado es 403, un relevo mal elegido es 409 (conflicto de estado) y lo
// demás 400. Que el cliente pueda distinguirlos es lo que permite a la pantalla
// reaccionar distinto ante «PIN incorrecto» y ante «elige a quién le pasas las
// mesas».
//
// 403 y NO 401 para el PIN, por dos razones: acá la sesión SÍ es válida (lo que
// falló es una autorización puntual), y el cliente trata el 401 como sesión
// caída —ni siquiera lee el cuerpo—, así que un 401 se tragaría el mensaje. Es
// el mismo criterio que ya usa /api/cajas/autorizar.
func estadoTurnoError(err error) int {
	switch {
	case errors.Is(err, application.ErrPinMesoneroInvalido),
		errors.Is(err, application.ErrPinSupervisorInvalido):
		return fiber.StatusForbidden
	case errors.Is(err, application.ErrSinTurnoAbierto),
		errors.Is(err, application.ErrTurnoCerrandoSinMesas):
		return fiber.StatusForbidden
	case errors.Is(err, application.ErrRelevoRequerido),
		errors.Is(err, application.ErrRelevoInvalido),
		errors.Is(err, application.ErrTurnoYaAbierto),
		errors.Is(err, application.ErrTurnoNoVivo):
		return fiber.StatusConflict
	case errors.Is(err, application.ErrMesoneroNoExiste),
		errors.Is(err, application.ErrTurnoNoExiste):
		return fiber.StatusNotFound
	}
	return fiber.StatusBadRequest
}

func (s *Server) handleMesoneros(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"mesoneros": s.svc.ListarMesoneros(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handleCrearMesonero(c *fiber.Ctx) error {
	var in struct {
		Nombre    string `json:"nombre"`
		SedeID    string `json:"sedeId"`
		UsuarioID string `json:"usuarioId"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	sede := in.SedeID
	if sede == "" {
		sede = sedeIDOf(c)
	}
	out, err := s.svc.CrearMesonero(empresaIDOf(c), principalOf(c).UserID, origen(c), sede, in.Nombre, in.UsuarioID)
	if err != nil {
		return c.Status(estadoTurnoError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleActualizarMesonero(c *fiber.Ctx) error {
	var in struct {
		Nombre *string `json:"nombre"`
		SedeID *string `json:"sedeId"`
		Activo *bool   `json:"activo"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ActualizarMesonero(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"),
		application.CambiosMesonero{Nombre: in.Nombre, SedeID: in.SedeID, Activo: in.Activo})
	if err != nil {
		return c.Status(estadoTurnoError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleFijarPinMesonero(c *fiber.Ctx) error {
	var in struct {
		Pin           string `json:"pin"`
		PinSupervisor string `json:"pinSupervisor"`
		Reiniciar     bool   `json:"reiniciar"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if err := s.svc.FijarPinMesonero(empresaIDOf(c), principalOf(c).UserID, origen(c),
		c.Params("id"), in.Pin, in.PinSupervisor, in.Reiniciar); err != nil {
		return c.Status(estadoTurnoError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleTurnosVivos(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"turnos": s.svc.TurnosVivos(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handleHistorialTurnos(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"turnos": s.svc.HistorialTurnos(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handleIniciarTurno(c *fiber.Ctx) error {
	var in struct {
		MesoneroID    string `json:"mesoneroId"`
		Pin           string `json:"pin"`
		PinSupervisor string `json:"pinSupervisor"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.IniciarTurno(empresaIDOf(c), principalOf(c).UserID, origen(c),
		in.MesoneroID, in.Pin, in.PinSupervisor)
	if err != nil {
		return c.Status(estadoTurnoError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (s *Server) handleFinalizarTurno(c *fiber.Ctx) error {
	out, err := s.svc.FinalizarTurno(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"))
	if err != nil {
		return c.Status(estadoTurnoError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleCandidatosRelevo(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"relevos": s.svc.CandidatosRelevo(empresaIDOf(c), c.Params("id"))})
}

func (s *Server) handleForzarCierreTurno(c *fiber.Ctx) error {
	var in struct {
		RelevoMesoneroID string `json:"relevoMesoneroId"`
		PinSupervisor    string `json:"pinSupervisor"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.CerrarTurnoForzado(empresaIDOf(c), principalOf(c).UserID, origen(c),
		c.Params("id"), in.RelevoMesoneroID, in.PinSupervisor)
	if err != nil {
		return c.Status(estadoTurnoError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleHorarios(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"horarios": s.svc.Horarios(empresaIDOf(c), sedeIDOf(c)),
		// El modo viaja con la lista: la pantalla necesita decir si al vencer se
		// avisa o se cierra solo, y pedirlo aparte sería una llamada de más.
		"modo": s.svc.ModoHorario(empresaIDOf(c), sedeIDOf(c)),
	})
}

func (s *Server) handleGuardarHorario(c *fiber.Ctx) error {
	var in struct {
		Franjas []mesonero.Franja `json:"franjas"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.GuardarHorario(empresaIDOf(c), principalOf(c).UserID, origen(c), c.Params("id"), in.Franjas)
	if err != nil {
		return c.Status(estadoTurnoError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleExtenderTurno(c *fiber.Ctx) error {
	var in struct {
		Minutos       int    `json:"minutos"`
		PinSupervisor string `json:"pinSupervisor"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.ExtenderTurno(empresaIDOf(c), principalOf(c).UserID, origen(c),
		c.Params("id"), in.Minutos, in.PinSupervisor)
	if err != nil {
		return c.Status(estadoTurnoError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}
