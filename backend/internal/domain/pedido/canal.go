package pedido

import "strings"

/* CANALES CONECTADOS Y ZONAS DE REPARTO.
 *
 * Un canal es por dónde entran pedidos: la tienda web, una app de comercio, o el
 * mostrador. Cada uno se configura por separado porque las decisiones son
 * distintas: la tienda propia puede confirmarse sola, y una app de terceros con
 * comisión conviene mirarla antes de aceptar.
 *
 * LA REGLA QUE ORDENA EL ENVÍO: el canal manda sobre el modo. Si el canal trae
 * su propio reparto, el local no despacha y no hay decisión que tomar. Los demás
 * dejan la decisión a la zona y la carga del momento. Sin esto, despacho
 * ofrecería asignar repartidor a un pedido que ya tiene courier en camino.
 */

// Canal es una fuente de pedidos conectada.
type Canal struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId,omitempty" bson:"sedeid,omitempty"`
	Nombre    string `json:"nombre" bson:"nombre"`
	// Origen dice de qué tipo es (ecommerce, app_commerce…).
	Origen string `json:"origen" bson:"origen"`
	Activo bool   `json:"activo" bson:"activo"`

	// ConfirmacionAutomatica deja pasar solos los pedidos que cumplen stock, zona
	// y pago. Apagada por defecto: confirmar reserva inventario y factura, así que
	// el default seguro es que alguien mire.
	ConfirmacionAutomatica bool `json:"confirmacionAutomatica" bson:"confirmacionautomatica"`
	// MinutosAceptacion es la ventana pactada para responder. 0 = sin ventana.
	// Vencida, el canal da el pedido por perdido y avisa al cliente, así que hay
	// que verla en la bandeja con un reloj y no descubrirla después.
	MinutosAceptacion int `json:"minutosAceptacion" bson:"minutosaceptacion"`

	// EnvioPropioDelCanal marca los canales que reparten con su propia flota. En
	// esos, el local solo prepara y entrega al courier: no se asigna repartidor ni
	// se ofrece elección de envío.
	EnvioPropioDelCanal bool `json:"envioPropioDelCanal" bson:"enviopropiodelcanal"`

	/* TOKEN DEL CANAL: con qué se autentica una tienda web al publicar pedidos.
	 *
	 * Se guarda HASHEADO y se muestra UNA sola vez, al generarlo. Si se pudiera
	 * volver a leer, cualquiera con acceso a la pantalla de configuración podría
	 * llevárselo — y un token de integración no se rota tan fácil como una
	 * contraseña, porque hay que ir a tocar el sistema del cliente.
	 *
	 * El hash es SHA-256 y no bcrypt a propósito: un token es alto en entropía y
	 * se verifica en CADA pedido que entra, así que el costo deliberado de bcrypt
	 * acá no compra seguridad, solo latencia en la puerta por donde llega el
	 * trabajo.
	 */
	TokenHash string `json:"-" bson:"tokenhash,omitempty"`
	// TokenPista son los últimos caracteres del token, para reconocerlo en la
	// pantalla sin poder reconstruirlo («…f3a9»).
	TokenPista string `json:"tokenPista,omitempty" bson:"tokenpista,omitempty"`
	// WebhookSecreto firma los avisos que el canal nos manda (estados del envío).
	WebhookSecreto string `json:"-" bson:"webhooksecreto,omitempty"`

	Creado      string `json:"creado" bson:"creado"`
	Actualizado string `json:"actualizado" bson:"actualizado"`
}

// ModoEnvioDe resuelve quién lleva un pedido de este canal. El canal manda: si
// reparte con su flota, no hay decisión. Si no, decide la zona (y en su defecto,
// la flota propia del local).
func (c Canal) ModoEnvioDe(z Zona) string {
	if c.EnvioPropioDelCanal {
		return EnvioCanal
	}
	if z.ID != "" && z.ModoEnvio != "" {
		return z.ModoEnvio
	}
	return EnvioPropio
}

/* ZONA DE REPARTO.
 *
 * Una zona es un área con su costo de envío, su promesa de tiempo y quién la
 * cubre. Existe porque «fuera de zona» es una de las razones más comunes de
 * rechazo, y detectarla ANTES de confirmar evita el caso peor: un pedido
 * aceptado, cobrado y cocinado que después nadie puede llevar.
 *
 * El área se define por RADIO desde el local y no por polígono. Un polígono se
 * dibuja mejor pero nadie lo mantiene; el radio se entiende, se explica al
 * cliente por teléfono («llegamos hasta Los Ruices») y alcanza para el 90 % de
 * los locales. Si hace falta el polígono, se agrega sin cambiar el resto.
 */
type Zona struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	Nombre    string `json:"nombre" bson:"nombre"`
	// RadioM es hasta dónde llega, en metros desde la sede. Las zonas se evalúan
	// de la más chica a la más grande, así una zona cercana y barata gana sobre
	// una lejana que también la contiene.
	RadioM int `json:"radioM" bson:"radiom"`
	// CostoEnvio es lo que se le cobra al cliente.
	CostoEnvio float64 `json:"costoEnvio" bson:"costoenvio"`
	// MinutosPromesa es cuánto se promete tardar. Fija la promesa de entrega al
	// confirmar, que es contra lo que se marca un pedido demorado.
	MinutosPromesa int `json:"minutosPromesa" bson:"minutospromesa"`
	// PedidoMinimo es el monto mínimo para repartir ahí. 0 = sin mínimo.
	PedidoMinimo float64 `json:"pedidoMinimo" bson:"pedidominimo"`
	// ModoEnvio fuerza quién cubre esta zona; vacío = flota propia.
	ModoEnvio string `json:"modoEnvio,omitempty" bson:"modoenvio,omitempty"`
	Activa    bool   `json:"activa" bson:"activa"`
}

// Cubre indica si una distancia en metros cae dentro de la zona.
func (z Zona) Cubre(metros float64) bool {
	return z.Activa && z.RadioM > 0 && metros <= float64(z.RadioM)
}

// Repartidor es quien lleva los pedidos de la flota propia.
//
// Es su propia entidad y no un usuario más porque su ciclo es distinto: entra y
// sale por turno, se le asignan pedidos, y al volver liquida lo que cobró. Un
// repartidor puede además tener usuario para la vista móvil, igual que el
// mesonero.
type Repartidor struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	Codigo    string `json:"codigo" bson:"codigo"`
	Nombre    string `json:"nombre" bson:"nombre"`
	Telefono  string `json:"telefono,omitempty" bson:"telefono,omitempty"`
	// UsuarioID enlaza con su cuenta para la vista móvil. Vacío = solo se le
	// asignan pedidos desde despacho, sin app propia.
	UsuarioID string `json:"usuarioId,omitempty" bson:"usuarioid,omitempty"`
	Vehiculo  string `json:"vehiculo,omitempty" bson:"vehiculo,omitempty"`
	Activo    bool   `json:"activo" bson:"activo"`
	// Disponible lo marca despacho o el propio repartidor: es lo que decide a
	// quién se le puede asignar ahora.
	Disponible bool   `json:"disponible" bson:"disponible"`
	Creado     string `json:"creado" bson:"creado"`
}

// Vehículos de reparto.
const (
	VehiculoMoto      = "moto"
	VehiculoBicicleta = "bicicleta"
	VehiculoCarro     = "carro"
	VehiculoAPie      = "a_pie"
)

// NormalizarCanal limpia y acota los campos del canal.
func (c *Canal) Normalizar() {
	c.Nombre = strings.TrimSpace(c.Nombre)
	if !OrigenValido(c.Origen) {
		c.Origen = OrigenEcommerce
	}
	if c.MinutosAceptacion < 0 {
		c.MinutosAceptacion = 0
	}
}

// Repositorios.
type CanalRepository interface {
	List(empresaID string) []Canal
	ByID(empresaID, id string) (Canal, bool)
	// ByTokenHash resuelve el canal que publica un pedido. NO lleva empresaID:
	// quien llama es la tienda del cliente, que solo trae su token — el token ES
	// la identidad, y de él sale la empresa.
	ByTokenHash(hash string) (Canal, bool)
	Upsert(c Canal) Canal
}

type ZonaRepository interface {
	List(empresaID, sedeID string) []Zona
	ByID(empresaID, id string) (Zona, bool)
	Upsert(z Zona) Zona
}

type RepartidorRepository interface {
	List(empresaID, sedeID string) []Repartidor
	ByID(empresaID, id string) (Repartidor, bool)
	ByUsuario(empresaID, usuarioID string) (Repartidor, bool)
	Upsert(r Repartidor) Repartidor
}

// LiquidacionRepository guarda las actas de cierre del repartidor. Solo Append y
// lectura: un acta de plata contada no se edita — si algo cambió, se levanta
// otra.
type LiquidacionRepository interface {
	Append(l Liquidacion) Liquidacion
	List(empresaID, sedeID string) []Liquidacion
	ByID(empresaID, id string) (Liquidacion, bool)
}
