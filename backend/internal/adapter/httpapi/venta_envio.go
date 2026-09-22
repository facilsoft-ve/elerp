package httpapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/pedido"
)

/* VENDER CON ENVÍO — la parte HTTP, compartida por las DOS rutas de facturación.
 *
 * Se factura por el mostrador (POS) y por el módulo de ventas (cotización), y el
 * envío tiene que comportarse igual en las dos: mismo renglón, mismo costo de
 * zona, mismo pedido. Tenerlo escrito dos veces es como termina el flete
 * cobrándose distinto según por dónde entró la venta.
 */

// envioReq son los datos de envío tal como los manda la pantalla de cobro.
type envioReq struct {
	Direccion   string  `json:"direccion"`
	Referencia  string  `json:"referencia"`
	Telefono    string  `json:"telefono"`
	Contacto    string  `json:"contacto"`
	Instruccion string  `json:"instruccion"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	// CobraEnvio en false factura el flete en cero: es «envío gratis», que el
	// local decide y tiene que poder aplicar sin falsear la zona.
	CobraEnvio bool `json:"cobraEnvio"`
}

// pide informa si esta venta se lleva. Una dirección vacía es «retira en el
// local», no un envío a ninguna parte.
func (e *envioReq) pide() bool { return e != nil && strings.TrimSpace(e.Direccion) != "" }

// lineaEnvio cotiza el flete y arma su renglón para la factura.
//
// El costo NO lo teclea quien factura: sale de la zona configurada. Si se
// pudiera escribir, cada cajero cobraría un flete distinto por la misma
// dirección, que es exactamente lo que las zonas existen para evitar.
//
// Devuelve un error en castellano si la dirección está fuera de zona: eso se
// rechaza ANTES de emitir, porque aceptar y después no poder llevar deja una
// factura hecha y un cliente esperando.
func (s *Server) lineaEnvio(c *fiber.Ctx, env *envioReq, baseVenta float64) (application.LineaEntrada, error) {
	if _, err := s.svc.AsegurarServicioEnvio(empresaIDOf(c), principalOf(c).UserID, origen(c)); err != nil {
		return application.LineaEntrada{}, fiber.NewError(fiber.StatusBadRequest,
			"no se pudo preparar el servicio de envío: "+err.Error())
	}
	cot := s.svc.CotizarEnvio(empresaIDOf(c), sedeIDOf(c), env.Lat, env.Lon, baseVenta)
	if !cot.Cubierta {
		return application.LineaEntrada{}, fiber.NewError(fiber.StatusBadRequest, cot.Motivo)
	}
	costo := cot.Costo
	if !env.CobraEnvio {
		costo = 0 // envío gratis: lo decide el local, y tiene que poder hacerlo
	}
	return application.LineaDeEnvio(costo, cot.ZonaNombre), nil
}

// pedidoDeVenta crea el pedido de un documento YA emitido y arma lo que la
// pantalla necesita para seguirlo.
//
// Se llama después de emitir y nunca antes: la factura es el hecho fiscal y no
// puede depender de que el módulo de envío esté sano. Si esto falla, la venta
// igual quedó cobrada — y el pedido se carga a mano; por eso el fallo se
// devuelve en el cuerpo en vez de tumbar la respuesta.
func (s *Server) pedidoDeVenta(c *fiber.Ctx, env *envioReq, doc fiscal.Documento) fiber.Map {
	items := make([]pedido.Item, 0, len(doc.Lineas))
	for _, l := range doc.Lineas {
		if l.SKU == application.SKUServicioEnvio {
			continue // el flete no es algo que el repartidor lleve: es lo que cobra por llevar
		}
		items = append(items, pedido.Item{
			SKU: l.SKU, Nombre: l.Nombre, Cantidad: l.Cantidad, PrecioUnitario: l.PrecioUnitario,
		})
	}
	// Pagado = se cobró completo. Una venta a crédito deja saldo, y ahí el
	// repartidor sí cobra en la puerta.
	p, err := s.svc.CrearPedidoDeVenta(empresaIDOf(c), sedeIDOf(c), doc.ID,
		principalOf(c).UserID, origen(c),
		application.EnvioDeVenta{
			Direccion: env.Direccion, Referencia: env.Referencia,
			Telefono: env.Telefono, Contacto: env.Contacto,
			Instruccion: env.Instruccion, Lat: env.Lat, Lon: env.Lon,
			CobraEnvio: env.CobraEnvio,
		}, items, doc.Total, !doc.Credito)
	if err != nil {
		return fiber.Map{"error": err.Error()}
	}
	return fiber.Map{"id": p.ID, "numero": p.Numero, "estado": p.Estado,
		"seguimiento": s.urlSeguimiento(p.TokenPublico)}
}

// baseDeLineas suma lo que vale la mercancía, sin impuestos. Es contra esto que
// la zona evalúa su pedido mínimo.
func baseDeLineas(ls []application.LineaEntrada) float64 {
	t := 0.0
	for _, l := range ls {
		t += l.PrecioUnitario * l.Cantidad
	}
	return t
}
