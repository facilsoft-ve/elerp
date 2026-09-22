package httpapi

import (
	"fmt"
	"html"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	fd "github.com/mornix/elerp/internal/domain/facturaciondigital"
	"github.com/mornix/elerp/internal/domain/usuario"
)

/* FACTURACIÓN DIGITAL — superficie HTTP.
 *
 * Dos zonas con reglas opuestas:
 *
 *   - La CONFIGURACIÓN y el seguimiento son de la administración de la empresa.
 *   - La PÁGINA DEL CLIENTE es pública y sin clave: la abre quien escanea el QR
 *     del ticket, en la calle, con el dato móvil justo. Por eso se sirve desde el
 *     backend como HTML plano y no desde la SPA: cargar la aplicación entera para
 *     mostrar tres líneas sería cobrarle al cliente el diseño de nuestra app.
 */
func (s *Server) rutasFacturacionDigital(api fiber.Router) {
	admin := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)
	g := api.Group("/facturacion-digital")
	g.Get("/config", admin, s.handleConfigDigital)
	g.Put("/config", admin, s.handleGuardarConfigDigital)
	// Probar conexión ANTES de activar: trae series y sucursales para elegirlas,
	// y dice si la cuenta está habilitada para emitir.
	g.Post("/probar", admin, s.handleProbarDigital)
	g.Get("/emisiones", admin, s.handleEmisionesDigitales)
	// Reintentar lo rechazado, cuando la causa ya se corrigió. No es automático a
	// propósito: repetir un 400 a ciegas repite el mismo error.
	g.Post("/emisiones/:id/reintentar", admin, s.handleReintentarEmision)
	g.Post("/emisiones/reintentar-rechazadas", admin, s.handleReintentarRechazadas)
}

func (s *Server) handleConfigDigital(c *fiber.Ctx) error {
	cfg := s.svc.ConfigDigital(empresaIDOf(c))
	return c.JSON(fiber.Map{
		"config": cfg,
		// tieneClave dice si hay contraseña guardada SIN devolverla: la pantalla
		// necesita saber si puede dejar el campo vacío al editar.
		"tieneClave": cfg.PasswordSHA512 != "",
		"lista":      cfg.Lista(),
	})
}

func (s *Server) handleGuardarConfigDigital(c *fiber.Ctx) error {
	var in struct {
		Activa           bool   `json:"activa"`
		PorPOS           bool   `json:"porPOS"`
		PorVentas        bool   `json:"porVentas"`
		Ambiente         string `json:"ambiente"`
		Usuario          string `json:"usuario"`
		Password         string `json:"password"`
		SerieStrongID    string `json:"serieStrongId"`
		SerieNombre      string `json:"serieNombre"`
		SucursalStrongID string `json:"sucursalStrongId"`
		SucursalNombre   string `json:"sucursalNombre"`
		CorreoRespaldo   string `json:"correoRespaldo"`
		TicketPOS        string `json:"ticketPOS"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "datos inválidos"})
	}
	out, err := s.svc.GuardarConfigDigital(empresaIDOf(c), principalOf(c).UserID, origen(c),
		application.EntradaConfigDigital{
			Activa: in.Activa, PorPOS: in.PorPOS, PorVentas: in.PorVentas,
			Ambiente: in.Ambiente, Usuario: in.Usuario, Password: in.Password,
			SerieStrongID: in.SerieStrongID, SerieNombre: in.SerieNombre,
			SucursalStrongID: in.SucursalStrongID, SucursalNombre: in.SucursalNombre,
			CorreoRespaldo: in.CorreoRespaldo, TicketPOS: in.TicketPOS,
		})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"config": out, "lista": out.Lista()})
}

func (s *Server) handleProbarDigital(c *fiber.Ctx) error {
	var in struct {
		Ambiente string `json:"ambiente"`
		Usuario  string `json:"usuario"`
		Password string `json:"password"`
	}
	_ = c.BodyParser(&in)
	d, err := s.svc.ProbarDigital(c.Context(), empresaIDOf(c), in.Ambiente, in.Usuario, in.Password)
	if err != nil {
		// 200 con el diagnóstico adentro: «no pude entrar» es un RESULTADO de la
		// prueba, no un error de la petición, y la pantalla tiene que poder
		// mostrar el motivo en vez de un cartel rojo genérico.
		return c.JSON(fiber.Map{"ok": false, "problema": d.Problema, "avisos": d.Avisos, "error": err.Error()})
	}
	return c.JSON(d)
}

func (s *Server) handleEmisionesDigitales(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"emisiones": s.svc.EmisionesDigitales(empresaIDOf(c))})
}

/* --- Página pública del cliente ------------------------------------------ */

// handleFacturaPublica muestra el estado de la factura de un cliente. Sin clave,
// porque se llega por el QR del ticket; el token es lo único que la protege, y
// por eso es aleatorio y no derivado del documento.
//
// Se responde HTML plano, servido por el backend: quien escanea está en la calle
// con el dato justo, y cargar la SPA entera para mostrar tres líneas sería
// cobrarle al cliente el peso de nuestra aplicación.
func (s *Server) handleFacturaPublica(c *fiber.Ctx) error {
	e, ok := s.svc.EmisionPorToken(c.Params("token"))
	c.Set("Content-Type", "text/html; charset=utf-8")
	// Nunca en caché: el estado cambia de «en proceso» a «lista» en minutos, y una
	// copia vieja haría que el cliente vuelva a mirar lo mismo.
	c.Set("Cache-Control", "no-store")
	if !ok {
		return c.Status(fiber.StatusNotFound).SendString(paginaFactura(estadoPagina{
			Titulo: "No encontramos esa factura",
			Cuerpo: "El enlace no corresponde a ninguna factura. Revisa el código del ticket o pídele ayuda al comercio.",
			Tono:   "aviso",
		}))
	}
	return c.SendString(paginaFactura(paginaDeEmision(e)))
}

type estadoPagina struct {
	Titulo   string
	Cuerpo   string
	Tono     string // ok | espera | aviso
	Detalle  []string
	Enlace   string
	Etiqueta string
}

// paginaDeEmision traduce el estado interno a lo que el cliente necesita saber.
// Nunca se le muestra un estado técnico: «error_temporal» no significa nada para
// quien compró un kilo de harina.
func paginaDeEmision(e fd.Emision) estadoPagina {
	detalle := []string{}
	if e.NumeroControl != "" {
		detalle = append(detalle, "Número de control: "+e.NumeroControl)
	}
	switch e.Estado {
	case fd.EstadoFiscal:
		p := estadoPagina{
			Titulo: "Tu factura está lista", Tono: "ok", Detalle: detalle,
			Cuerpo: "Ya fue emitida por la imprenta digital autorizada.",
		}
		if e.URLDocumento != "" {
			p.Enlace, p.Etiqueta = e.URLDocumento, "Ver y descargar la factura"
		}
		return p
	case fd.EstadoRechazado:
		return estadoPagina{
			Titulo: "Tu factura necesita revisión", Tono: "aviso",
			Cuerpo: "Hubo un inconveniente al emitirla y el comercio ya fue notificado. Guarda tu ticket: tu compra está registrada.",
		}
	case fd.EstadoAnulado:
		return estadoPagina{Titulo: "Esta factura fue anulada", Tono: "aviso",
			Cuerpo: "Si no esperabas esto, consulta con el comercio."}
	default:
		// Pendiente, enviada o reintentando: para el cliente es lo mismo — todavía
		// no está, y va a estar.
		return estadoPagina{
			Titulo: "Tu factura se está generando", Tono: "espera",
			Cuerpo: "La imprenta digital la está procesando. Suele tardar unos minutos. " +
				"Puedes volver a abrir este enlace más tarde: se actualiza solo.",
		}
	}
}

// paginaFactura arma el HTML. Es una sola página, sin dependencias externas ni
// JavaScript: tiene que abrir en un teléfono viejo, con mala señal, y no puede
// quedarse en blanco porque no cargó un script.
func paginaFactura(p estadoPagina) string {
	colores := map[string][2]string{
		"ok":     {"#166B41", "#EAF5EF"},
		"espera": {"#1D3477", "#EDF2F9"},
		"aviso":  {"#92600A", "#FDF6E7"},
	}
	col, ok := colores[p.Tono]
	if !ok {
		col = colores["espera"]
	}
	var det strings.Builder
	for _, d := range p.Detalle {
		fmt.Fprintf(&det, `<div class="d">%s</div>`, html.EscapeString(d))
	}
	enlace := ""
	if p.Enlace != "" {
		enlace = fmt.Sprintf(`<a class="btn" href="%s" rel="noopener">%s</a>`,
			html.EscapeString(p.Enlace), html.EscapeString(p.Etiqueta))
	}
	return fmt.Sprintf(`<!doctype html><html lang="es"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>%s</title>
<style>
:root{color-scheme:light dark}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
 padding:24px;font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;
 background:#FAFBFC;color:#1F2430}
.c{width:100%%;max-width:420px;background:#fff;border:1px solid #E8EAEF;border-radius:16px;
 padding:28px 24px;box-shadow:0 1px 3px rgba(0,0,0,.05)}
.p{display:inline-block;padding:5px 12px;border-radius:999px;font-size:13px;font-weight:600;
 color:%s;background:%s;margin-bottom:16px}
h1{font-size:21px;line-height:1.25;margin:0 0 10px}
p{margin:0;color:#5C6470;font-size:14.5px}
.d{margin-top:14px;font-size:13.5px;color:#5C6470;font-variant-numeric:tabular-nums}
.btn{display:block;margin-top:20px;padding:13px;border-radius:10px;background:#1D3477;color:#fff;
 text-align:center;text-decoration:none;font-weight:600;font-size:15px}
.f{margin-top:22px;padding-top:16px;border-top:1px solid #E8EAEF;font-size:12px;color:#8B94A3}
@media(prefers-color-scheme:dark){body{background:#0f1117;color:#e8eaef}
 .c{background:#171a21;border-color:#262b36}p,.d{color:#9aa3b2}.f{border-color:#262b36}}
</style></head><body><div class="c">
<span class="p">Factura digital</span>
<h1>%s</h1><p>%s</p>%s%s
<div class="f">Documento emitido por una imprenta digital autorizada por el SENIAT.</div>
</div></body></html>`,
		html.EscapeString(p.Titulo), col[0], col[1],
		html.EscapeString(p.Titulo), html.EscapeString(p.Cuerpo), det.String(), enlace)
}

/* --- QR del ticket -------------------------------------------------------- */

// handleQRFactura devuelve el QR del enlace público como PNG.
//
// Se genera en el SERVIDOR y no en la pantalla por dos razones: el frontend
// depende solo de react y react-dom —meter un codificador de QR rompería esa
// disciplina— y el ticket tiene que poder imprimirlo también el agente fiscal
// local, que no es un navegador.
//
// No lleva autenticación porque es la misma información que ya está impresa en
// el papel: el enlace a una página pública. Pedir sesión para dibujar un QR que
// el cliente ya tiene en la mano no protegería nada.
func (s *Server) handleQRFactura(c *fiber.Ctx) error {
	token := c.Params("token")
	if _, ok := s.svc.EmisionPorToken(token); !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}
	png, err := qrDe(s.urlPublicaFactura(token))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "no se pudo generar el QR"})
	}
	c.Set("Content-Type", "image/png")
	// El QR de un token no cambia nunca: se puede cachear con tranquilidad, y así
	// reimprimir un ticket no vuelve a generarlo.
	c.Set("Cache-Control", "public, max-age=604800, immutable")
	return c.Send(png)
}

func (s *Server) handleReintentarEmision(c *fiber.Ctx) error {
	out, err := s.svc.ReintentarEmision(empresaIDOf(c), c.Params("id"), principalOf(c).UserID, origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}

func (s *Server) handleReintentarRechazadas(c *fiber.Ctx) error {
	n := s.svc.ReintentarRechazadas(empresaIDOf(c), principalOf(c).UserID, origen(c))
	return c.JSON(fiber.Map{"reencoladas": n})
}
