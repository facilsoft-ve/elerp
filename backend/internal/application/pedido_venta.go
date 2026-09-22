package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/pedido"
)

/* VENDER CON ENVÍO desde el punto de venta o el módulo de ventas.
 *
 * El caso: el cliente está en el mostrador o al teléfono, se le factura, y hay
 * que llevárselo. Antes eso obligaba a facturar por un lado y cargar el pedido
 * por otro, a mano, con la dirección tecleada dos veces — que es como se manda
 * un pedido a la dirección del cliente anterior.
 *
 * EL ENVÍO ES UN RENGLÓN DE LA FACTURA, no un dato al margen. Cobrar por llevar
 * es un servicio gravado: tiene que salir en la factura, en el libro de ventas y
 * en el IVA. Por eso se resuelve como un producto del catálogo —con su alícuota
 * y su cuenta— y no como un campo suelto del documento.
 *
 * EL COSTO NO SE TECLEA: sale de la ZONA configurada en el módulo de envío. Si
 * quien factura pudiera escribirlo, cada cajero cobraría un flete distinto por la
 * misma dirección, que es exactamente lo que las zonas existen para evitar.
 */

// SKUServicioEnvio es el código del servicio de envío en el catálogo. Fijo y
// reconocible: tiene que poder buscarse en el libro de ventas y en los reportes.
const SKUServicioEnvio = "SRV-ENVIO"

// AsegurarServicioEnvio devuelve el producto con el que se factura el envío,
// creándolo si la empresa todavía no lo tiene.
//
// Se crea solo, igual que el almacén principal, porque es infraestructura del
// módulo y no una decisión del cliente: quien activa el envío no tiene por qué
// saber que además debe dar de alta un producto para poder cobrarlo.
func (s *Service) AsegurarServicioEnvio(empresaID, actor, origen string) (inventario.Producto, error) {
	if p, ok := s.productos.BySKU(empresaID, SKUServicioEnvio); ok {
		return p, nil
	}
	p := inventario.Producto{
		EmpresaID: empresaID, SKU: SKUServicioEnvio, Nombre: "Servicio de envío a domicilio",
		// SIN STOCK: es un servicio. Llevarlo al Kardex lo dejaría en negativo
		// permanente y ensuciaría la valorización del inventario con algo que no
		// es mercancía.
		UnidadBase: inventario.UnidadUnidad, TipoVenta: inventario.TipoVentaUnidad,
		Rubro: "Servicios", Precio: 0, Activo: true,
		// SERVICIO: se factura y no mueve stock. Sin esta marca, cada envío
		// cobrado dejaría el Kardex con una unidad negativa más.
		EsServicio: true,
	}
	out, err := s.CrearProducto(empresaID, actor, origen, p)
	if err != nil {
		return inventario.Producto{}, err
	}
	s.audit.Append(evento(empresaID, actor, origen, "pedido.servicio_envio.crear", SKUServicioEnvio, ""))
	return out, nil
}

// CotizacionEnvio es lo que la pantalla necesita para mostrar el envío ANTES de
// cobrar: a qué zona cae la dirección, cuánto cuesta y en cuánto se promete.
type CotizacionEnvio struct {
	Cubierta   bool    `json:"cubierta"`
	ZonaID     string  `json:"zonaId,omitempty"`
	ZonaNombre string  `json:"zonaNombre,omitempty"`
	Costo      float64 `json:"costo"`
	Minutos    int     `json:"minutos,omitempty"`
	Minimo     float64 `json:"minimo,omitempty"`
	// Motivo explica por qué no se puede llevar, en castellano. «Fuera de zona»
	// dicho a tiempo evita el caso peor: cobrar y después no poder despachar.
	Motivo string `json:"motivo,omitempty"`
	// SinZonas avisa que la empresa todavía no dibujó sus zonas. NO bloquea: un
	// local que recién empieza tiene que poder despachar igual.
	SinZonas bool `json:"sinZonas"`
}

// CotizarEnvio resuelve el envío de una dirección antes de facturar.
func (s *Service) CotizarEnvio(empresaID, sedeID string, lat, lon, totalVenta float64) CotizacionEnvio {
	if s.zonasPedido == nil {
		return CotizacionEnvio{Cubierta: true, SinZonas: true}
	}
	zonas := []pedido.Zona{}
	for _, z := range s.zonasPedido.List(empresaID, sedeID) {
		if z.Activa {
			zonas = append(zonas, z)
		}
	}
	if len(zonas) == 0 {
		return CotizacionEnvio{Cubierta: true, SinZonas: true}
	}
	// Sin coordenadas no se puede medir, y eso NO bloquea: media Venezuela dicta
	// su dirección por referencia y no por punto en el mapa. Se cobra la zona más
	// cercana como estimación y quien despacha decide.
	sede, ok := s.sedeDe(empresaID, sedeID)
	if !ok || (lat == 0 && lon == 0) || (sede.Lat == 0 && sede.Lon == 0) {
		sort.SliceStable(zonas, func(i, j int) bool { return zonas[i].RadioM < zonas[j].RadioM })
		z := zonas[0]
		return CotizacionEnvio{
			Cubierta: true, ZonaID: z.ID, ZonaNombre: z.Nombre, Costo: z.CostoEnvio,
			Minutos: z.MinutosPromesa, Minimo: z.PedidoMinimo,
			Motivo: "sin ubicación en el mapa: se estima con la zona más cercana",
		}
	}
	metros := distanciaM(sede.Lat, sede.Lon, lat, lon)
	sort.SliceStable(zonas, func(i, j int) bool { return zonas[i].RadioM < zonas[j].RadioM })
	for _, z := range zonas {
		if !z.Cubre(metros) {
			continue
		}
		c := CotizacionEnvio{
			Cubierta: true, ZonaID: z.ID, ZonaNombre: z.Nombre, Costo: z.CostoEnvio,
			Minutos: z.MinutosPromesa, Minimo: z.PedidoMinimo,
		}
		if z.PedidoMinimo > 0 && totalVenta > 0 && totalVenta < z.PedidoMinimo {
			c.Cubierta = false
			c.Motivo = fmt.Sprintf("%s exige un mínimo de %.2f y la venta va en %.2f", z.Nombre, z.PedidoMinimo, totalVenta)
		}
		return c
	}
	return CotizacionEnvio{
		Cubierta: false,
		Motivo:   fmt.Sprintf("la dirección está a %.0f m del local, fuera de las zonas de reparto", metros),
	}
}

// EnvioDeVenta son los datos de envío que acompañan a una venta.
type EnvioDeVenta struct {
	Direccion   string
	Referencia  string
	Telefono    string
	Contacto    string
	Instruccion string
	Lat, Lon    float64
	// CobraEnvio en false factura el envío en cero: es la promoción de «envío
	// gratis», que el local decide y tiene que poder aplicar sin falsear la zona.
	CobraEnvio bool
}

// CrearPedidoDeVenta arma el pedido de un documento ya facturado.
//
// Se llama DESPUÉS de emitir y no antes: la factura es el hecho fiscal y no
// puede depender de que el módulo de envío esté sano. Si esto falla, la venta
// igual quedó cobrada y facturada — y el pedido se puede cargar a mano.
//
// El pedido nace CONFIRMADO: lo que se factura en el mostrador ya fue revisado
// por una persona con el cliente delante.
func (s *Service) CrearPedidoDeVenta(empresaID, sedeID, documentoID, actor, origen string,
	env EnvioDeVenta, items []pedido.Item, total float64, pagado bool) (pedido.Pedido, error) {
	if s.pedidos == nil {
		return pedido.Pedido{}, ErrPedidosNoDisponible
	}
	formaPago := pedido.PagoContraEntrega
	if pagado {
		// Ya se cobró en la caja: el repartidor NO cobra nada en la puerta, y
		// decirlo mal es el error que más caro sale de este módulo.
		formaPago = pedido.PagoEnCanal
	}
	p, err := s.CrearPedido(EntradaPedido{
		EmpresaID: empresaID, SedeID: sedeID, Origen: pedido.OrigenManual,
		Destino: pedido.Destino{
			Direccion: env.Direccion, Referencia: env.Referencia, Telefono: env.Telefono,
			Contacto: env.Contacto, Instruccion: env.Instruccion, Lat: env.Lat, Lon: env.Lon,
		},
		Items: items, FormaPago: formaPago, Total: total,
		Actor: actor, OrigenEvento: origen,
	})
	if err != nil {
		return pedido.Pedido{}, err
	}
	// El pedido recuerda SU factura: es lo que permite cerrar el círculo cuando
	// alguien pregunta por qué se cobró un envío, o cuando hay que anular.
	p.DocumentoID = documentoID
	if s.pedidos != nil {
		s.pedidos.Update(p)
	}
	return p, nil
}

// LineaDeEnvio arma el renglón del servicio de envío para la factura.
func LineaDeEnvio(costo float64, zona string) LineaEntrada {
	nota := "Envío a domicilio"
	if strings.TrimSpace(zona) != "" {
		nota += " · " + zona
	}
	return LineaEntrada{SKU: SKUServicioEnvio, Cantidad: 1, PrecioUnitario: costo, Descripcion: nota}
}
