// Package pedido modela el ciclo de un PEDIDO PARA LLEVAR A DOMICILIO, desde que
// entra hasta que se entrega, venga de donde venga y lo lleve quien lo lleve.
//
// QUÉ NO ESTÁ ACÁ, Y POR QUÉ. El pedido no reimplementa el cobro: sale por el
// motor fiscal de siempre.
//
// UN SOLO FLUJO PARA CUALQUIER NEGOCIO. El pedido pide UNIDADES VENDIBLES y no
// sabe —ni tiene por qué saber— cómo se producen. Cómo se produce una unidad es
// asunto del PRODUCTO: un plato declara su receta y descuenta insumos del
// inventario; un artículo de tienda se toma del estante; una pieza formulada
// descontaría su fórmula igual que el plato. Esa es la misma segmentación, y es
// lo que más adelante permite un módulo de fabricación sin tocar nada de acá.
//
// Por eso NO hay «modo restaurante» y «modo retail»: serían dos flujos paralelos
// para el mismo ciclo, y cada regla nueva habría que escribirla dos veces. Lo
// único que cambia entre un local y otro es QUÉ DISPARA el paso a «listo»:
//
//   - Si hay renglones que se producen (receta) y la empresa tiene cocina
//     configurada, se emite la comanda y el pedido queda listo cuando sale el
//     último renglón. Sin duplicar tableros: el cocinero usa el de siempre.
//   - Si no hay nada que producir, no se emite comanda y quien arma el pedido lo
//     marca listo desde la bandeja.
//
// Las dos cosas terminan en el mismo estado por el mismo camino. La segunda no
// es un modo: es el caso en que no había nada que mandar a producir.
//
// Lo demás que es nuevo: de dónde entró el pedido, a dónde va, quién lo lleva y
// en qué punto del camino está.
package pedido

import "strings"

/* ORIGEN — de dónde entró el pedido.
 *
 * Los tres orígenes producen la misma estructura y caen en la misma bandeja; lo
 * que cambia es en qué estado NACEN y quién responde por ellos. Un pedido que
 * entra por API nace `nuevo` y espera revisión, porque confirmar es el punto sin
 * retorno: reserva inventario y factura. Uno que arma el cajero ya fue revisado
 * por una persona al armarlo, así que nace confirmado.
 */
const (
	// OrigenManual: lo arma la caja, de mostrador o por teléfono.
	OrigenManual = "manual"
	// OrigenEcommerce: lo publica la tienda web por API.
	OrigenEcommerce = "ecommerce"
	// OrigenAppCommerce: viene de una app de pedidos, con ventana de aceptación.
	// Si nadie responde a tiempo, el canal vence el pedido y avisa al cliente.
	OrigenAppCommerce = "app_commerce"
)

/* ESTADOS. Uno solo por pedido y siempre visible: un pedido que está «en dos
 * estados a la vez» es un pedido que nadie sabe quién tiene.
 *
 * El recorrido normal es nuevo → confirmado → en_preparacion → listo → (quien lo
 * lleve) → entregado. Los tres finales de excepción (rechazado, cancelado,
 * entrega_fallida) pueden alcanzarse desde distintos puntos, y `devuelto` es el
 * cierre de una entrega fallida cuya mercancía volvió al local.
 */
const (
	EstadoNuevo          = "nuevo"
	EstadoConfirmado     = "confirmado"
	EstadoEnPreparacion  = "en_preparacion"
	EstadoListo          = "listo"
	EstadoAsignado       = "asignado"
	EstadoRetiradoCanal  = "retirado_canal"
	EstadoEnRuta         = "en_ruta"
	EstadoEntregado      = "entregado"
	EstadoRechazado      = "rechazado"
	EstadoCancelado      = "cancelado"
	EstadoEntregaFallida = "entrega_fallida"
	EstadoDevuelto       = "devuelto"
)

// Finales son los estados donde el pedido deja de moverse.
func Final(estado string) bool {
	switch estado {
	case EstadoEntregado, EstadoRechazado, EstadoCancelado, EstadoDevuelto:
		return true
	}
	return false
}

/* MODO DE ENVÍO — quién lo lleva.
 *
 * La diferencia que manda: en `canal` el local NO despacha. Prepara, empaca y se
 * lo entrega al courier que manda la app; el seguimiento y el cobro quedan del
 * lado de ella. Tratarlo como los otros dos haría que despacho ofrezca asignar
 * un repartidor que no existe, y que el sistema prometa un seguimiento que no
 * controla.
 */
const (
	// EnvioPropio: flota del local. Despacho asigna repartidor y vehículo.
	EnvioPropio = "propio"
	// EnvioApp: app de envío contratada por el local. Se le solicita el envío y
	// sus estados entran por webhook.
	EnvioApp = "app"
	// EnvioCanal: la misma app que trajo el pedido lo reparte. El local solo
	// prepara y entrega al courier.
	EnvioCanal = "canal"
)

// Item es un renglón del pedido. Es el mismo dato que un renglón de cuenta —se
// copia al abrir la cuenta de cocina— pero se guarda acá también porque un
// pedido rechazado nunca llega a tener cuenta y aun así hay que saber qué pedía.
type Item struct {
	SKU            string  `json:"sku" bson:"sku"`
	Nombre         string  `json:"nombre" bson:"nombre"`
	Cantidad       float64 `json:"cantidad" bson:"cantidad"`
	PrecioUnitario float64 `json:"precioUnitario" bson:"preciounitario"`
	Nota           string  `json:"nota,omitempty" bson:"nota,omitempty"`
}

// Destino es a dónde va el pedido. La dirección se guarda TEXTUAL además de sus
// coordenadas: el repartidor navega con el punto, pero quien atiende el teléfono
// necesita leer «al lado de la panadería» — y esa es la mitad de las direcciones
// reales en Venezuela.
type Destino struct {
	Direccion   string  `json:"direccion" bson:"direccion"`
	Referencia  string  `json:"referencia,omitempty" bson:"referencia,omitempty"`
	Lat         float64 `json:"lat,omitempty" bson:"lat,omitempty"`
	Lon         float64 `json:"lon,omitempty" bson:"lon,omitempty"`
	ZonaID      string  `json:"zonaId,omitempty" bson:"zonaid,omitempty"`
	ZonaNombre  string  `json:"zonaNombre,omitempty" bson:"zonanombre,omitempty"`
	Telefono    string  `json:"telefono,omitempty" bson:"telefono,omitempty"`
	Contacto    string  `json:"contacto,omitempty" bson:"contacto,omitempty"`
	Instruccion string  `json:"instruccion,omitempty" bson:"instruccion,omitempty"`
}

// Evento es una entrada de la bitácora. El estado del pedido es uno solo, pero
// CÓMO llegó ahí es lo que se necesita cuando alguien reclama: quién, cuándo y
// desde dónde. Append-only: los eventos no se editan ni se borran.
type Evento struct {
	Cuando string `json:"cuando" bson:"cuando"` // RFC3339
	Estado string `json:"estado" bson:"estado"`
	// Actor es el usuario, el repartidor o el canal; Origen dice si fue una
	// persona, un webhook o una regla automática.
	Actor  string `json:"actor,omitempty" bson:"actor,omitempty"`
	Origen string `json:"origen,omitempty" bson:"origen,omitempty"`
	Motivo string `json:"motivo,omitempty" bson:"motivo,omitempty"`
}

// Pedido es el agregado.
type Pedido struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// Numero es el correlativo visible del pedido en el local ("#124"). No es
	// fiscal: la factura lleva su propia numeración.
	Numero int    `json:"numero" bson:"numero"`
	Estado string `json:"estado" bson:"estado"`

	Origen string `json:"origen" bson:"origen"`
	// CanalID es el canal conectado que lo trajo (vacío en los manuales), y
	// ReferenciaExterna el id del pedido EN ESE canal. La referencia es la clave
	// de deduplicación: un canal que reintenta no puede crear dos pedidos.
	CanalID           string `json:"canalId,omitempty" bson:"canalid,omitempty"`
	CanalNombre       string `json:"canalNombre,omitempty" bson:"canalnombre,omitempty"`
	ReferenciaExterna string `json:"referenciaExterna,omitempty" bson:"referenciaexterna,omitempty"`

	ClienteID     string  `json:"clienteId,omitempty" bson:"clienteid,omitempty"`
	ClienteNombre string  `json:"clienteNombre" bson:"clientenombre"`
	Destino       Destino `json:"destino" bson:"destino"`
	Items         []Item  `json:"items" bson:"items"`

	// CuentaID es la cuenta SIN MESA por la que los renglones llegaron a
	// producción, cuando hubo algo que producir. Vacío significa que este pedido
	// no necesitó comanda —ni porque el local no produce, ni porque lo pedido sale
	// del estante— y entonces lo marca listo quien lo arma. No es un modo: es el
	// rastro de si hubo producción.
	CuentaID string `json:"cuentaId,omitempty" bson:"cuentaid,omitempty"`
	// DocumentoID es la factura, cuando se emitió.
	DocumentoID string `json:"documentoId,omitempty" bson:"documentoid,omitempty"`

	// Tracking es la llave del envío: identifica la etiqueta impresa, el enlace
	// público del cliente y la referencia frente a la app. Se emite al marcar
	// LISTO, no al confirmar — ver TrackingAlConfirmar.
	Tracking string `json:"tracking,omitempty" bson:"tracking,omitempty"`
	// TokenPublico es el identificador de la página de seguimiento. Va aparte del
	// tracking porque el tracking se imprime y se dicta por teléfono: si fuera la
	// llave de la página, cualquiera que oiga un número entra a ver la dirección
	// de otro.
	TokenPublico string `json:"-" bson:"tokenpublico,omitempty"`

	ModoEnvio string `json:"modoEnvio,omitempty" bson:"modoenvio,omitempty"`
	// RepartidorID/Nombre solo aplican en flota propia.
	RepartidorID     string `json:"repartidorId,omitempty" bson:"repartidorid,omitempty"`
	RepartidorNombre string `json:"repartidorNombre,omitempty" bson:"repartidornombre,omitempty"`
	// EnvioExternoID es el identificador del envío en la app (propia o del canal),
	// y es lo que permite conciliar y liquidar comisiones después.
	EnvioExternoID string `json:"envioExternoId,omitempty" bson:"envioexternoid,omitempty"`

	// CostoEnvio es lo que se le cobra al cliente por el envío; CostoCourier lo
	// que cuesta el courier. Se guardan los dos: su diferencia es el margen (o la
	// pérdida) del delivery, y sin separarlos no hay forma de saberlo.
	CostoEnvio   float64 `json:"costoEnvio" bson:"costoenvio"`
	CostoCourier float64 `json:"costoCourier,omitempty" bson:"costocourier,omitempty"`

	// FormaPago dice si el pedido ya viene pagado por el canal o se cobra contra
	// entrega. Decide si el repartidor tiene que cobrar y cerrar caja al volver.
	FormaPago string  `json:"formaPago" bson:"formapago"`
	Total     float64 `json:"total" bson:"total"`

	/* LO QUE EL REPARTIDOR TRAE.
	 *
	 * CobradoBs es lo que declaró haber recibido en la puerta, y se guarda apenas
	 * cierra la entrega, no al final del turno: preguntarle al volver cuánto cobró
	 * en cada una de once puertas es preguntarle que se acuerde.
	 *
	 * LiquidacionID es el acta donde esa plata se entregó. Es lo que impide
	 * contarla dos veces: un pedido ya liquidado no vuelve a aparecer pendiente,
	 * por más veces que se abra la pantalla. */
	CobradoBs     float64 `json:"cobradoBs,omitempty" bson:"cobradobs,omitempty"`
	LiquidacionID string  `json:"liquidacionId,omitempty" bson:"liquidacionid,omitempty"`

	// PromesaEntrega (RFC3339) es a qué hora se prometió. Vencida, el pedido se
	// marca demorado igual que una comanda demorada en cocina.
	PromesaEntrega string `json:"promesaEntrega,omitempty" bson:"promesaentrega,omitempty"`
	// ProgramadoPara (RFC3339) es un pedido pactado para más tarde: no entra a
	// cocina al confirmarse, sino a su hora.
	ProgramadoPara string `json:"programadoPara,omitempty" bson:"programadopara,omitempty"`
	// VenceAceptacion (RFC3339) es hasta cuándo el canal espera respuesta.
	VenceAceptacion string `json:"venceAceptacion,omitempty" bson:"venceaceptacion,omitempty"`

	/* PRODUCCIÓN LISTA ≠ PEDIDO LISTO, y confundirlas manda al repartidor a
	 * buscar algo que todavía no está en el mostrador.
	 *
	 * Cocina avisa que terminó el último plato; eso no significa que el pedido se
	 * pueda retirar — falta empacarlo, meter la bebida, revisar que esté completo.
	 * Quien gestiona el delivery es el que confirma «listo para retirar», y esa
	 * confirmación es la que emite el número de envío y le dice a la app (o al
	 * repartidor propio) que venga.
	 *
	 * ProduccionLista guarda CUÁNDO avisó cocina, para que la bandeja lo destaque
	 * y quien despacha sepa que ya puede revisar y confirmar.
	 */
	ProduccionLista string `json:"produccionLista,omitempty" bson:"produccionlista,omitempty"`

	// PruebaEntrega es cómo se comprobó la entrega (firma, foto, nombre de quien
	// recibió). Es lo que responde un reclamo de «nunca me llegó».
	PruebaEntrega string `json:"pruebaEntrega,omitempty" bson:"pruebaentrega,omitempty"`
	MotivoCierre  string `json:"motivoCierre,omitempty" bson:"motivocierre,omitempty"`

	Bitacora []Evento `json:"bitacora" bson:"bitacora"`
	Creado   string   `json:"creado" bson:"creado"`
	Cerrado  string   `json:"cerrado,omitempty" bson:"cerrado,omitempty"`
}

// Formas de pago de un pedido.
const (
	// PagoEnCanal: el cliente ya pagó en la tienda web o en la app. El repartidor
	// no cobra nada.
	PagoEnCanal = "en_canal"
	// PagoContraEntrega: se cobra al entregar (efectivo o punto móvil), y el
	// repartidor cierra caja al volver.
	PagoContraEntrega = "contra_entrega"
)

/* TRANSICIONES PERMITIDAS.
 *
 * Se declaran en vez de comprobarse a mano en cada operación porque el ciclo
 * tiene once estados y tres caminos de envío: escrito a mano, tarde o temprano
 * alguien deja pasar un «entregado» sobre un pedido que nunca salió, y entonces
 * el historial deja de servir para responder un reclamo.
 */
var transiciones = map[string][]string{
	EstadoNuevo:          {EstadoConfirmado, EstadoRechazado, EstadoCancelado},
	EstadoConfirmado:     {EstadoEnPreparacion, EstadoCancelado, EstadoListo},
	EstadoEnPreparacion:  {EstadoListo, EstadoCancelado},
	EstadoListo:          {EstadoAsignado, EstadoRetiradoCanal, EstadoEnRuta, EstadoCancelado},
	EstadoAsignado:       {EstadoEnRuta, EstadoListo, EstadoCancelado},
	EstadoRetiradoCanal:  {EstadoEntregado, EstadoEntregaFallida},
	EstadoEnRuta:         {EstadoEntregado, EstadoEntregaFallida},
	EstadoEntregaFallida: {EstadoEnRuta, EstadoDevuelto, EstadoEntregado},
}

// PuedePasarA indica si la transición es válida.
func PuedePasarA(desde, hasta string) bool {
	if desde == hasta {
		return false
	}
	for _, s := range transiciones[desde] {
		if s == hasta {
			return true
		}
	}
	return false
}

// Demorado indica si venció la promesa de entrega sin haber entregado. Se marca
// igual que una comanda demorada en cocina: en rojo, para quien despacha.
func (p Pedido) Demorado(ahora string) bool {
	if p.PromesaEntrega == "" || Final(p.Estado) {
		return false
	}
	return p.PromesaEntrega < ahora
}

// EsperaAceptacion indica si el pedido está en la ventana en que el canal espera
// respuesta.
func (p Pedido) EsperaAceptacion() bool {
	return p.Estado == EstadoNuevo && p.VenceAceptacion != ""
}

// VencioAceptacion indica si se pasó la ventana sin responder.
func (p Pedido) VencioAceptacion(ahora string) bool {
	return p.EsperaAceptacion() && p.VenceAceptacion < ahora
}

// EsperaConfirmacionDeListo indica que cocina ya terminó y falta que alguien
// confirme que el pedido está armado y se puede retirar. Es el estado que la
// bandeja tiene que destacar: es trabajo esperando a una persona.
func (p Pedido) EsperaConfirmacionDeListo() bool {
	return p.Estado == EstadoEnPreparacion && p.ProduccionLista != ""
}

// EnProduccion indica si este pedido mandó algo a producir. Es informativo —para
// saber si esperar a cocina o marcarlo listo a mano—, no una bifurcación del
// ciclo: el estado siguiente es «listo» en los dos casos.
func (p Pedido) EnProduccion() bool { return p.CuentaID != "" }

// DespachaElLocal indica si el local tiene que resolver el envío. Con envío del
// canal NO: prepara, empaca y entrega al courier que la app manda, y ofrecer
// asignar repartidor ahí sería ofrecer algo que no existe.
func (p Pedido) DespachaElLocal() bool { return p.ModoEnvio != EnvioCanal }

// CobraElRepartidor indica si hay que cobrarle al cliente en la puerta.
func (p Pedido) CobraElRepartidor() bool {
	return p.FormaPago == PagoContraEntrega && p.ModoEnvio == EnvioPropio
}

/* LIQUIDACIÓN DEL REPARTIDOR — el turno se cierra con plata contada.
 *
 * El repartidor sale con pedidos que se cobran en la puerta y vuelve con
 * efectivo ajeno en el bolsillo. Eso no es un detalle operativo: hasta que no
 * entrega y alguien cuenta, la empresa tiene plata afuera y no sabe cuánta.
 *
 * El acta es INMUTABLE, como el resto de lo que toca dinero: si al día siguiente
 * aparece un billete, se hace otra liquidación, no se edita esta. Lo que se
 * corrige a mano no se puede auditar.
 */
type Liquidacion struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`

	RepartidorID     string `json:"repartidorId" bson:"repartidorid"`
	RepartidorNombre string `json:"repartidorNombre" bson:"repartidornombre"`

	// PedidoIDs son las entregas que esta acta salda. Van explícitas y no por
	// rango de fechas: el acta tiene que poder releerse dentro de un año y decir
	// exactamente qué cubrió.
	PedidoIDs []string `json:"pedidoIds" bson:"pedidoids"`

	// Esperado es lo que el repartidor debía traer según las entregas; Declarado,
	// lo que puso sobre el mostrador. La diferencia se guarda calculada porque es
	// el número por el que se conversa, y no se deja para que cada pantalla lo
	// vuelva a sacar.
	EsperadoBs   float64 `json:"esperadoBs" bson:"esperadobs"`
	DeclaradoBs  float64 `json:"declaradoBs" bson:"declaradobs"`
	DiferenciaBs float64 `json:"diferenciaBs" bson:"diferenciabs"`

	// Nota explica la diferencia cuando la hay. Un faltante sin explicación no
	// sirve para decidir nada.
	Nota string `json:"nota,omitempty" bson:"nota,omitempty"`

	Actor  string `json:"actor" bson:"actor"`
	Creada string `json:"creada" bson:"creada"`
}

// Cuadra indica si lo declarado coincide con lo esperado, con la tolerancia del
// centavo que impone la aritmética de punto flotante.
func (l Liquidacion) Cuadra() bool { return l.DiferenciaBs > -0.005 && l.DiferenciaBs < 0.005 }

// OrigenValido acota el origen.
func OrigenValido(o string) bool {
	switch o {
	case OrigenManual, OrigenEcommerce, OrigenAppCommerce:
		return true
	}
	return false
}

// ModoEnvioValido acota el modo de envío.
func ModoEnvioValido(m string) bool {
	switch m {
	case EnvioPropio, EnvioApp, EnvioCanal:
		return true
	}
	return false
}

// Normalizar limpia los campos de texto del pedido.
func (p *Pedido) Normalizar() {
	p.ClienteNombre = strings.TrimSpace(p.ClienteNombre)
	p.Destino.Direccion = strings.TrimSpace(p.Destino.Direccion)
	p.Destino.Referencia = strings.TrimSpace(p.Destino.Referencia)
	p.Destino.Telefono = strings.TrimSpace(p.Destino.Telefono)
	p.ReferenciaExterna = strings.TrimSpace(p.ReferenciaExterna)
}

// Repository persiste los pedidos, aislado por empresa.
type Repository interface {
	Append(p Pedido) Pedido
	Update(p Pedido) (Pedido, bool)
	ByID(empresaID, id string) (Pedido, bool)
	// ByReferencia resuelve la deduplicación: un canal que reintenta no puede
	// crear dos pedidos.
	ByReferencia(empresaID, canalID, referencia string) (Pedido, bool)
	// ByTracking y ByToken son las búsquedas del seguimiento.
	ByTracking(empresaID, tracking string) (Pedido, bool)
	ByToken(token string) (Pedido, bool)
	List(empresaID, sedeID string) []Pedido
}
