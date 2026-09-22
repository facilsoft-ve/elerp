package httpapi

import (
	"fmt"
	"html"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/pedido"
	"github.com/mornix/elerp/internal/domain/usuario"
)

/* PEDIDOS PARA LLEVAR — superficie HTTP.
 *
 * Tres zonas con reglas distintas:
 *
 *   - ATENDER pedidos (la bandeja) lo hace quien gestiona el mostrador: es una
 *     pantalla de trabajo, como el tablero de comandas.
 *   - CONFIGURAR canales, zonas y repartidores es de la administración.
 *   - El SEGUIMIENTO del cliente es público y sin clave, como la página de la
 *     factura digital: lo abre quien recibió el enlace, en la calle.
 */
func (s *Server) registerPedidos(api fiber.Router) {
	admin := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)
	g := api.Group("/pedidos")

	// La bandeja y su atención: mismo gate que el resto de la operación del
	// mostrador. Quien cobra también atiende pedidos.
	g.Get("/", s.handlePedidos)
	g.Post("/", s.handleCrearPedido)
	g.Get("/:id", s.handlePedido)
	g.Post("/:id/confirmar", s.handleConfirmarPedido)
	g.Post("/:id/rechazar", s.handleRechazarPedido)
	g.Post("/:id/listo", s.handleListoPedido)
	g.Post("/:id/asignar", s.handleAsignarPedido)
	g.Post("/:id/en-ruta", s.handleEnRutaPedido)
	g.Post("/:id/entregado", s.handleEntregadoPedido)
	g.Post("/:id/fallida", s.handleFallidaPedido)
	g.Post("/:id/cancelar", s.handleCancelarPedido)

	// VISTA DEL REPARTIDOR. Sin gate de rol: lo que la acota es el usuario de la
	// sesión, que solo devuelve lo suyo. Un repartidor puede tener cualquier rol
	// —o ninguno de los de oficina— y aun así tiene que poder trabajar.
	g.Get("/mis-entregas", s.handleMisEntregas)
	g.Post("/disponibilidad", s.handleDisponibilidadRepartidor)

	// COTIZAR EL ENVÍO antes de facturar: lo usan el punto de venta y el módulo
	// de ventas para mostrar el costo y avisar si la dirección queda fuera. Lo
	// llama quien cobra, así que va con el gate de la operación.
	g.Post("/cotizar-envio", s.handleCotizarEnvio)

	// Maestros del módulo.
	g.Get("/config/canales", admin, s.handleCanalesPedido)
	// El token se genera y se muestra UNA vez: si se pudiera volver a leer,
	// cualquiera con acceso a esta pantalla podría llevárselo.
	g.Post("/config/canales/:id/token", admin, s.handleTokenCanal)
	g.Put("/config/canales", admin, s.handleGuardarCanalPedido)
	g.Get("/config/zonas", admin, s.handleZonasPedido)
	g.Put("/config/zonas", admin, s.handleGuardarZonaPedido)
	g.Get("/config/repartidores", s.handleRepartidores)
	g.Put("/config/repartidores", admin, s.handleGuardarRepartidor)
}

func (s *Server) handlePedidos(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"pedidos": s.svc.Pedidos(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handlePedido(c *fiber.Ctx) error {
	p, ok := s.svc.PedidoDe(empresaIDOf(c), c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "el pedido no existe"})
	}
	return c.JSON(p)
}

type itemPedidoIn struct {
	SKU            string  `json:"sku"`
	Nombre         string  `json:"nombre"`
	Cantidad       float64 `json:"cantidad"`
	PrecioUnitario float64 `json:"precioUnitario"`
	Nota           string  `json:"nota"`
}

func (s *Server) handleCrearPedido(c *fiber.Ctx) error {
	var in struct {
		Origen            string `json:"origen"`
		CanalID           string `json:"canalId"`
		ReferenciaExterna string `json:"referenciaExterna"`
		ClienteID         string `json:"clienteId"`
		ClienteNombre     string `json:"clienteNombre"`
		Destino           struct {
			Direccion   string  `json:"direccion"`
			Referencia  string  `json:"referencia"`
			Lat         float64 `json:"lat"`
			Lon         float64 `json:"lon"`
			Telefono    string  `json:"telefono"`
			Contacto    string  `json:"contacto"`
			Instruccion string  `json:"instruccion"`
		} `json:"destino"`
		Items          []itemPedidoIn `json:"items"`
		FormaPago      string         `json:"formaPago"`
		Total          float64        `json:"total"`
		ProgramadoPara string         `json:"programadoPara"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	items := make([]pedido.Item, 0, len(in.Items))
	for _, it := range in.Items {
		items = append(items, pedido.Item{
			SKU: it.SKU, Nombre: it.Nombre, Cantidad: it.Cantidad,
			PrecioUnitario: it.PrecioUnitario, Nota: it.Nota,
		})
	}
	out, err := s.svc.CrearPedido(application.EntradaPedido{
		EmpresaID: empresaIDOf(c), SedeID: sedeIDOf(c),
		Origen: in.Origen, CanalID: in.CanalID, ReferenciaExterna: in.ReferenciaExterna,
		ClienteID: in.ClienteID, ClienteNombre: in.ClienteNombre,
		Destino: pedido.Destino{
			Direccion: in.Destino.Direccion, Referencia: in.Destino.Referencia,
			Lat: in.Destino.Lat, Lon: in.Destino.Lon,
			Telefono: in.Destino.Telefono, Contacto: in.Destino.Contacto,
			Instruccion: in.Destino.Instruccion,
		},
		Items: items, FormaPago: in.FormaPago, Total: in.Total,
		ProgramadoPara: in.ProgramadoPara,
		Actor:          principalOf(c).UserID, OrigenEvento: origen(c),
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

// conMotivo comparte la lectura del motivo, que en tres operaciones distintas es
// obligatorio por la misma razón: un pedido que se cae sin explicación no deja
// aprender nada ni responderle al cliente.
func conMotivo(c *fiber.Ctx) string {
	var in struct {
		Motivo string `json:"motivo"`
	}
	_ = c.BodyParser(&in)
	return in.Motivo
}

func (s *Server) responder(c *fiber.Ctx, p pedido.Pedido, err error) error {
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(p)
}

func (s *Server) handleConfirmarPedido(c *fiber.Ctx) error {
	p, err := s.svc.ConfirmarPedido(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	return s.responder(c, p, err)
}

func (s *Server) handleRechazarPedido(c *fiber.Ctx) error {
	p, err := s.svc.RechazarPedido(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), conMotivo(c))
	return s.responder(c, p, err)
}

func (s *Server) handleListoPedido(c *fiber.Ctx) error {
	p, err := s.svc.MarcarListo(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	return s.responder(c, p, err)
}

func (s *Server) handleAsignarPedido(c *fiber.Ctx) error {
	var in struct {
		RepartidorID string `json:"repartidorId"`
	}
	if err := c.BodyParser(&in); err != nil || in.RepartidorID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "elige un repartidor"})
	}
	p, err := s.svc.AsignarRepartidor(empresaIDOf(c), c.Params("id"), in.RepartidorID, principalOf(c).UserID, origen(c))
	return s.responder(c, p, err)
}

func (s *Server) handleEnRutaPedido(c *fiber.Ctx) error {
	p, err := s.svc.MarcarEnRuta(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	return s.responder(c, p, err)
}

func (s *Server) handleEntregadoPedido(c *fiber.Ctx) error {
	var in struct {
		Prueba    string  `json:"prueba"`
		CobradoBs float64 `json:"cobradoBs"`
	}
	_ = c.BodyParser(&in)
	p, err := s.svc.MarcarEntregado(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c),
		application.CierreEntrega{Prueba: in.Prueba, CobradoBs: in.CobradoBs})
	return s.responder(c, p, err)
}

func (s *Server) handleFallidaPedido(c *fiber.Ctx) error {
	p, err := s.svc.EntregaFallida(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), conMotivo(c))
	return s.responder(c, p, err)
}

func (s *Server) handleCancelarPedido(c *fiber.Ctx) error {
	p, err := s.svc.CancelarPedido(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c), conMotivo(c))
	return s.responder(c, p, err)
}

// handleCotizarEnvio resuelve a qué zona cae una dirección y cuánto cuesta
// llevarla, ANTES de cobrar.
//
// Antes de cobrar y no después: «fuera de zona» descubierto tarde es el caso
// peor — la venta ya se cobró, la comida ya se hizo y nadie puede llevarla.
func (s *Server) handleCotizarEnvio(c *fiber.Ctx) error {
	var in struct {
		Lat   float64 `json:"lat"`
		Lon   float64 `json:"lon"`
		Total float64 `json:"total"`
	}
	_ = c.BodyParser(&in)
	return c.JSON(s.svc.CotizarEnvio(empresaIDOf(c), sedeIDOf(c), in.Lat, in.Lon, in.Total))
}

/* --- Maestros ------------------------------------------------------------- */

func (s *Server) handleCanalesPedido(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"canales": s.svc.CanalesPedido(empresaIDOf(c))})
}

func (s *Server) handleGuardarCanalPedido(c *fiber.Ctx) error {
	var in pedido.Canal
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.GuardarCanalPedido(empresaIDOf(c), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

// handleTokenCanal genera el token de un canal y lo devuelve en claro, una vez.
func (s *Server) handleTokenCanal(c *fiber.Ctx) error {
	token, err := s.svc.GenerarTokenCanal(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"token": token,
		"url":   s.urlEntradaPedidos(),
		"aviso": "Guarda este token ahora: por seguridad no se puede volver a ver. Si lo pierdes, genera uno nuevo (el anterior deja de servir).",
	})
}

// urlEntradaPedidos es la dirección a la que la tienda tiene que publicar.
func (s *Server) urlEntradaPedidos() string {
	base := strings.TrimRight(s.cfg.FrontendURL, "/")
	if base == "" || strings.Contains(base, "*") {
		return "/pedidos/entrada"
	}
	return base + "/pedidos/entrada"
}

func (s *Server) handleZonasPedido(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"zonas": s.svc.ZonasPedido(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handleGuardarZonaPedido(c *fiber.Ctx) error {
	var in pedido.Zona
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if in.SedeID == "" {
		in.SedeID = sedeIDOf(c)
	}
	out, err := s.svc.GuardarZonaPedido(empresaIDOf(c), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleRepartidores(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"repartidores": s.svc.Repartidores(empresaIDOf(c), sedeIDOf(c))})
}

func (s *Server) handleGuardarRepartidor(c *fiber.Ctx) error {
	var in pedido.Repartidor
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	if in.SedeID == "" {
		in.SedeID = sedeIDOf(c)
	}
	out, err := s.svc.GuardarRepartidor(empresaIDOf(c), principalOf(c).UserID, origen(c), in)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

/* --- Seguimiento público -------------------------------------------------- */

// handleSeguimientoPublico muestra al cliente dónde va su pedido. Sin clave: se
// llega por el enlace que recibió. Lo protege el token, que es aleatorio y NO el
// número de tracking — ese se imprime en la etiqueta y se dicta por teléfono, y
// si fuera la llave cualquiera que oiga un número vería la dirección de otro.
//
// HTML plano servido por el backend, como la página de la factura: quien lo abre
// está esperando en su casa con el dato justo, no queriendo cargar una
// aplicación entera.
func (s *Server) handleSeguimientoPublico(c *fiber.Ctx) error {
	p, ok := s.svc.PedidoPorToken(c.Params("token"))
	c.Set("Content-Type", "text/html; charset=utf-8")
	c.Set("Cache-Control", "no-store")
	if !ok {
		return c.Status(fiber.StatusNotFound).SendString(paginaSeguimiento(
			"No encontramos ese pedido",
			"El enlace no corresponde a ningún pedido. Revisa el mensaje que recibiste o consulta con el comercio.",
			"aviso", "", nil))
	}
	titulo, cuerpo, tono := textoSeguimiento(p)
	return c.SendString(paginaSeguimiento(titulo, cuerpo, tono, p.Tracking, pasosSeguimiento(p)))
}

// textoSeguimiento traduce el estado interno a lo que el cliente necesita saber.
// Nunca se le muestra el estado técnico: «retirado_canal» no significa nada para
// quien está esperando su comida.
func textoSeguimiento(p pedido.Pedido) (titulo, cuerpo, tono string) {
	switch p.Estado {
	case pedido.EstadoNuevo:
		return "Recibimos tu pedido", "El comercio lo está revisando. En un momento te confirmamos.", "espera"
	case pedido.EstadoConfirmado:
		return "Tu pedido fue confirmado", "Ya está en la cola de preparación.", "espera"
	case pedido.EstadoEnPreparacion:
		return "Estamos preparando tu pedido", "Te avisamos apenas salga hacia tu dirección.", "espera"
	case pedido.EstadoListo, pedido.EstadoAsignado:
		return "Tu pedido está listo", "Está por salir hacia tu dirección.", "espera"
	case pedido.EstadoRetiradoCanal:
		return "Tu pedido va en camino", "Lo retiró el repartidor de la aplicación por la que pediste.", "ok"
	case pedido.EstadoEnRuta:
		return "Tu pedido va en camino", nombreRepartidor(p), "ok"
	case pedido.EstadoEntregado:
		return "Tu pedido fue entregado", "¡Gracias por tu compra!", "ok"
	case pedido.EstadoEntregaFallida:
		return "No pudimos entregarlo", "El comercio se va a comunicar contigo para resolverlo.", "aviso"
	case pedido.EstadoRechazado, pedido.EstadoCancelado:
		return "Tu pedido no se pudo procesar", "Si ya pagaste, el comercio te va a contactar.", "aviso"
	}
	return "Tu pedido", "Estamos procesándolo.", "espera"
}

func nombreRepartidor(p pedido.Pedido) string {
	if p.RepartidorNombre != "" {
		return p.RepartidorNombre + " lo lleva a tu dirección."
	}
	return "Va hacia tu dirección."
}

// pasosSeguimiento arma la línea de tiempo. Se muestran los pasos ALCANZADOS y
// el actual, no los futuros: una lista de pasos por venir con horas en blanco se
// lee como una promesa que nadie hizo.
func pasosSeguimiento(p pedido.Pedido) []string {
	vistos := map[string]bool{}
	out := []string{}
	for _, e := range p.Bitacora {
		etiqueta := etiquetaEstado(e.Estado)
		if etiqueta == "" || vistos[e.Estado] {
			continue
		}
		vistos[e.Estado] = true
		hora := ""
		if len(e.Cuando) >= 16 {
			hora = e.Cuando[11:16]
		}
		out = append(out, strings.TrimSpace(hora+" · "+etiqueta))
	}
	return out
}

func etiquetaEstado(e string) string {
	switch e {
	case pedido.EstadoNuevo:
		return "Pedido recibido"
	case pedido.EstadoConfirmado:
		return "Confirmado"
	case pedido.EstadoEnPreparacion:
		return "En preparación"
	case pedido.EstadoListo:
		return "Listo"
	case pedido.EstadoEnRuta, pedido.EstadoRetiradoCanal:
		return "En camino"
	case pedido.EstadoEntregado:
		return "Entregado"
	}
	return ""
}

// paginaSeguimiento arma el HTML. Una sola página, sin JavaScript ni recursos
// externos: tiene que abrir en un teléfono viejo con mala señal y no puede
// quedarse en blanco porque no cargó un script.
func paginaSeguimiento(titulo, cuerpo, tono, tracking string, pasos []string) string {
	colores := map[string][2]string{
		"ok":     {"#166B41", "#EAF5EF"},
		"espera": {"#1D3477", "#EDF2F9"},
		"aviso":  {"#92600A", "#FDF6E7"},
	}
	col, ok := colores[tono]
	if !ok {
		col = colores["espera"]
	}
	var linea strings.Builder
	for _, p := range pasos {
		fmt.Fprintf(&linea, `<li>%s</li>`, html.EscapeString(p))
	}
	ref := ""
	if tracking != "" {
		ref = fmt.Sprintf(`<div class="t">Nº de envío <b>%s</b></div>`, html.EscapeString(tracking))
	}
	return fmt.Sprintf(`<!doctype html><html lang="es"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>%s</title>
<style>
:root{color-scheme:light dark}*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;
 font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;background:#FAFBFC;color:#1F2430}
.c{width:100%%;max-width:420px;background:#fff;border:1px solid #E8EAEF;border-radius:16px;padding:28px 24px;
 box-shadow:0 1px 3px rgba(0,0,0,.05)}
.p{display:inline-block;padding:5px 12px;border-radius:999px;font-size:13px;font-weight:600;color:%s;background:%s;margin-bottom:16px}
h1{font-size:21px;line-height:1.25;margin:0 0 10px}
p{margin:0;color:#5C6470;font-size:14.5px}
.t{margin-top:14px;font-size:13.5px;color:#5C6470;font-variant-numeric:tabular-nums}
ul{margin:18px 0 0;padding:0;list-style:none;font-size:13.5px;color:#5C6470}
li{padding:7px 0 7px 18px;border-left:2px solid #E8EAEF;position:relative}
li:last-child{border-left-color:%s;color:#1F2430;font-weight:600}
.f{margin-top:22px;padding-top:16px;border-top:1px solid #E8EAEF;font-size:12px;color:#8B94A3}
@media(prefers-color-scheme:dark){body{background:#0f1117;color:#e8eaef}.c{background:#171a21;border-color:#262b36}
 p,.t,ul{color:#9aa3b2}li{border-left-color:#262b36}.f{border-color:#262b36}}
</style></head><body><div class="c">
<span class="p">Seguimiento del pedido</span>
<h1>%s</h1><p>%s</p>%s<ul>%s</ul>
<div class="f">Seguimiento en vivo. Vuelve a abrir este enlace para ver el avance.</div>
</div></body></html>`,
		html.EscapeString(titulo), col[0], col[1], col[0],
		html.EscapeString(titulo), html.EscapeString(cuerpo), ref, linea.String())
}

/* --- Vista del repartidor ------------------------------------------------- */

// handleMisEntregas devuelve los pedidos del repartidor que consulta.
//
// Se resuelve por el USUARIO de la sesión y no por un id en la URL: un
// repartidor solo ve lo suyo, y dejar que pida el id de otro sería dejar que vea
// las direcciones y los teléfonos de los clientes de todo el local.
func (s *Server) handleMisEntregas(c *fiber.Ctx) error {
	r, ok := s.svc.RepartidorDeUsuario(empresaIDOf(c), principalOf(c).UserID)
	if !ok {
		return c.JSON(fiber.Map{"repartidor": nil, "entregas": []any{}})
	}
	return c.JSON(fiber.Map{
		"repartidor": r,
		"entregas":   s.svc.EntregasDe(empresaIDOf(c), r.ID),
	})
}

// handleDisponibilidad marca al repartidor disponible o no. Lo decide él desde
// su teléfono: despacho no puede saber si ya volvió del almuerzo.
func (s *Server) handleDisponibilidadRepartidor(c *fiber.Ctx) error {
	var in struct {
		Disponible bool `json:"disponible"`
	}
	_ = c.BodyParser(&in)
	r, err := s.svc.MarcarDisponibilidad(empresaIDOf(c), principalOf(c).UserID, in.Disponible, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(r)
}

/* --- Entrada pública: la tienda web publica sus pedidos ------------------- */

/* POR QUÉ ESTA RUTA VIVE FUERA DE /api.
 *
 * Quien llama es el sistema del cliente —su tienda web—, que no tiene sesión de
 * usuario ni la va a tener: no es una persona, es un servidor. Se autentica con
 * el TOKEN DEL CANAL, que además dice de qué empresa y qué sede es el pedido.
 *
 * Sin esto el módulo declaraba tres orígenes y solo funcionaban dos: el
 * ecommerce no tenía por dónde entrar.
 */
func (s *Server) registerPedidosPublico(app *fiber.App) {
	app.Post("/pedidos/entrada", s.handleEntradaPedido)
}

// handleEntradaPedido recibe un pedido de un canal conectado.
func (s *Server) handleEntradaPedido(c *fiber.Ctx) error {
	// El token va en cabecera y no en el cuerpo: así no queda escrito en los
	// registros de quien reenvía la petición ni en el historial de una prueba.
	token := strings.TrimSpace(string(c.Request().Header.Peek("X-Canal-Token")))
	if token == "" {
		token = strings.TrimPrefix(strings.TrimSpace(string(c.Request().Header.Peek("Authorization"))), "Bearer ")
	}
	canal, ok := s.svc.CanalPorToken(token)
	if !ok {
		// 401 y no 404: la ruta existe, lo que no vale es la credencial. Y no se
		// distingue «token inválido» de «canal apagado» — decirlo ayudaría a
		// quien esté probando tokens ajenos.
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "token de canal inválido o canal inactivo"})
	}

	var in struct {
		Referencia    string  `json:"referencia"`
		ClienteNombre string  `json:"cliente"`
		Telefono      string  `json:"telefono"`
		Direccion     string  `json:"direccion"`
		Referencia2   string  `json:"puntoDeReferencia"`
		Lat           float64 `json:"lat"`
		Lon           float64 `json:"lon"`
		Pagado        bool    `json:"pagado"`
		Total         float64 `json:"total"`
		Programado    string  `json:"programadoPara"`
		Items         []struct {
			SKU      string  `json:"sku"`
			Nombre   string  `json:"nombre"`
			Cantidad float64 `json:"cantidad"`
			Precio   float64 `json:"precioUnitario"`
			Nota     string  `json:"nota"`
		} `json:"items"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	items := make([]pedido.Item, 0, len(in.Items))
	for _, it := range in.Items {
		items = append(items, pedido.Item{
			SKU: it.SKU, Nombre: it.Nombre, Cantidad: it.Cantidad,
			PrecioUnitario: it.Precio, Nota: it.Nota,
		})
	}
	// La forma de pago la declara la tienda: en un ecommerce lo normal es que ya
	// se haya cobrado, pero hay tiendas que ofrecen pagar al recibir. Quien lo
	// sabe es ella, no nosotros.
	formaPago := pedido.PagoEnCanal
	if !in.Pagado {
		formaPago = pedido.PagoContraEntrega
	}
	out, err := s.svc.CrearPedido(application.EntradaPedido{
		EmpresaID: canal.EmpresaID, SedeID: canal.SedeID,
		Origen: canal.Origen, CanalID: canal.ID, ReferenciaExterna: in.Referencia,
		ClienteNombre: in.ClienteNombre,
		Destino: pedido.Destino{
			Direccion: in.Direccion, Referencia: in.Referencia2,
			Lat: in.Lat, Lon: in.Lon, Telefono: in.Telefono,
		},
		Items: items, FormaPago: formaPago, Total: in.Total,
		ProgramadoPara: in.Programado,
		Actor:          "canal:" + canal.Nombre, OrigenEvento: "api",
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	// Se devuelve lo que la tienda necesita para su propio seguimiento: nuestro
	// número y el enlace que le puede mostrar a su cliente. Nada más — el resto
	// del pedido es del local, no de la tienda.
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":          out.ID,
		"numero":      out.Numero,
		"estado":      out.Estado,
		"seguimiento": s.urlSeguimiento(out.TokenPublico),
	})
}

// urlSeguimiento arma el enlace público del pedido.
func (s *Server) urlSeguimiento(token string) string {
	base := strings.TrimRight(s.cfg.FrontendURL, "/")
	if base == "" || strings.Contains(base, "*") {
		return "/t/" + token
	}
	return base + "/t/" + token
}
