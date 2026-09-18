// Package facturaciondigital modela la emisión de documentos a través de una
// IMPRENTA DIGITAL autorizada por el SENIAT (hoy: UniDigital / DigitalInvoice).
//
// POR QUÉ ES UN MÓDULO APARTE Y NO UN CAMPO MÁS DEL DOCUMENTO.
//
// La imprenta digital rompe el supuesto sobre el que está construido el resto
// del motor fiscal: que al terminar de emitir, el documento YA es fiscal. Acá no.
// `createandapprove` devuelve un identificador; el NÚMERO DE CONTROL —lo que
// convierte el documento en fiscal— lo asigna la imprenta entre uno y cinco
// minutos después, y hay que ir a buscarlo.
//
// Eso obliga a tres cosas que no existían:
//
//  1. Un documento que nace SIN número de control y en estado pendiente. Ya hay
//     precedente: en máquina fiscal lo asigna la impresora y queda vacío.
//  2. Una COLA con reintentos. La API responde 429 cuando la petición anterior
//     de la misma cuenta sigue procesándose, así que los envíos de una serie van
//     de a uno, nunca en paralelo.
//  3. Un OUTBOX: la intención de emitir se persiste ANTES de llamar a la API. Si
//     el proceso se cae entre el cobro y el envío, la factura no se pierde — se
//     reintenta. Sin esto, un corte de luz en el momento exacto deja una venta
//     cobrada y sin facturar, y el cliente ya se fue.
//
// El correlativo lo seguimos llevando nosotros (ver domain/fiscal/serie.go): la
// imprenta exige `Number` ascendente estricto por serie y tipo, y rechaza huecos.
package facturaciondigital

import "strings"

// Estados de una emisión. Avanzan en un solo sentido salvo el reintento, que
// devuelve de `error_temporal` a `pendiente`.
const (
	// EstadoPendiente: encolada, todavía no se envió. Es el estado en el que nace
	// toda emisión, antes de tocar la red.
	EstadoPendiente = "pendiente"
	// EstadoEnviado: la imprenta la aceptó y devolvió su identificador. Todavía NO
	// es fiscal: falta el número de control.
	EstadoEnviado = "enviado"
	// EstadoFiscal: tiene número de control. Recién acá el documento es fiscal.
	EstadoFiscal = "fiscal"
	// EstadoErrorTemporal: falló por algo que puede resolverse solo (red, 429,
	// 500, token vencido). Se reintenta con espera creciente.
	EstadoErrorTemporal = "error_temporal"
	// EstadoRechazado: la imprenta lo rechazó por una regla de negocio (montos que
	// no cuadran, RIF inválido, correlativo fuera de orden). NO se reintenta solo:
	// reintentar a ciegas quema otro correlativo y repite el mismo error.
	EstadoRechazado = "rechazado"
	// EstadoAnulado: se anuló en la imprenta.
	EstadoAnulado = "anulado"
)

// Emision es el registro de outbox de un documento enviado a la imprenta.
// APPEND-ONLY en su historia: cada intento agrega una entrada a Intentos, y el
// estado avanza. No se borra nunca — es la tabla de conciliación que hace falta
// ante cualquier reclamo (nuestro documento ↔ su número ↔ su número de control).
type Emision struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// DocumentoID es el documento fiscal de ElERP que origina la emisión. Viaja a
	// la imprenta como `SystemReference`: es la clave de idempotencia, y con ella
	// se puede preguntar «¿esto ya se envió?» antes de reintentar y quemar otro
	// correlativo.
	DocumentoID string `json:"documentoId" bson:"documentoid"`
	Tipo        string `json:"tipo" bson:"tipo"` // FA | NC | ND
	// Numero es el correlativo que le asignamos nosotros (ascendente por serie y
	// tipo). Se reserva ANTES de enviar y no se reutiliza.
	Numero int    `json:"numero" bson:"numero"`
	Serie  string `json:"serie" bson:"serie"`
	Estado string `json:"estado" bson:"estado"`
	// StrongID es el identificador que devuelve la imprenta al aceptar.
	StrongID string `json:"strongId,omitempty" bson:"strongid,omitempty"`
	// NumeroControl llega después, por consulta. Mientras esté vacío, el documento
	// no es fiscal.
	NumeroControl string `json:"numeroControl,omitempty" bson:"numerocontrol,omitempty"`
	// CodigoCorto es el código que la imprenta genera para compartir el documento
	// (pensado para WhatsApp).
	CodigoCorto string `json:"codigoCorto,omitempty" bson:"codigocorto,omitempty"`
	// URLDocumento es la URL de visualización que da la imprenta.
	URLDocumento string `json:"urlDocumento,omitempty" bson:"urldocumento,omitempty"`
	// Token es NUESTRO identificador público, el de la página que ve el cliente.
	// No es el id del documento a propósito: la página es pública y sin clave, y
	// un id secuencial o adivinable dejaría enumerar las facturas de la empresa.
	Token string `json:"token" bson:"token"`
	// Intentos es la bitácora: cada envío con su resultado. Lo que soporte pide.
	Intentos []Intento `json:"intentos" bson:"intentos"`
	Creada   string    `json:"creada" bson:"creada"`
	// ProximoIntento (RFC3339) es cuándo vuelve a tocarle a esta emisión. Es lo
	// que implementa la espera creciente sin bloquear a nadie.
	ProximoIntento string `json:"proximoIntento,omitempty" bson:"proximointento,omitempty"`
	Actualizada    string `json:"actualizada" bson:"actualizada"`
}

// Intento registra un envío con su resultado. Los tres campos del error se
// guardan enteros porque son literalmente lo que soporte pide para diagnosticar.
type Intento struct {
	Cuando  string `json:"cuando" bson:"cuando"`
	HTTP    int    `json:"http" bson:"http"`
	Codigo  string `json:"codigo,omitempty" bson:"codigo,omitempty"`
	Mensaje string `json:"mensaje,omitempty" bson:"mensaje,omitempty"`
	Extra   string `json:"extra,omitempty" bson:"extra,omitempty"`
}

// EsFiscal indica si la emisión ya recibió su número de control.
func (e Emision) EsFiscal() bool { return strings.TrimSpace(e.NumeroControl) != "" }

// Terminada indica si ya no hay nada más que hacer con esta emisión.
func (e Emision) Terminada() bool {
	return e.Estado == EstadoFiscal || e.Estado == EstadoRechazado || e.Estado == EstadoAnulado
}

/* CONFIGURACIÓN POR EMPRESA.
 *
 * El módulo se enciende y se apaga, y se enciende POR CANAL: una empresa puede
 * facturar digitalmente lo que vende por el mostrador y no lo que factura desde
 * el módulo de ventas, o al revés. Son decisiones operativas distintas —el POS
 * necesita entregarle algo al cliente en el acto; el módulo de ventas no— así
 * que se configuran por separado en vez de con un solo interruptor.
 */
type Config struct {
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// Activa es el interruptor maestro. Apagada, nada de este módulo corre.
	Activa bool `json:"activa" bson:"activa"`
	// PorPOS y PorVentas encienden el canal. Con Activa en false, ninguno aplica.
	PorPOS    bool `json:"porPOS" bson:"porpos"`
	PorVentas bool `json:"porVentas" bson:"porventas"`
	// Ambiente: "qa" o "produccion". Se guarda explícito y NO se deduce de la URL:
	// emitir contra producción creyendo que es la prueba genera documentos
	// fiscales de verdad, que no se borran (solo se anulan).
	Ambiente string `json:"ambiente" bson:"ambiente"`
	Usuario  string `json:"usuario" bson:"usuario"`
	// PasswordSHA512 es el digest hexadecimal, que es lo que la API espera. Se
	// guarda el digest y no la contraseña: es lo que viaja, así que guardar el
	// texto plano no aporta nada y sí expone. Nunca sale por JSON.
	PasswordSHA512 string `json:"-" bson:"passwordsha512"`
	// SerieStrongID y SucursalStrongID los da la imprenta. Se eligen de la lista
	// que ella devuelve, nunca se teclean.
	SerieStrongID    string `json:"serieStrongId" bson:"seriestrongid"`
	SerieNombre      string `json:"serieNombre" bson:"serienombre"`
	SucursalStrongID string `json:"sucursalStrongId" bson:"sucursalstrongid"`
	SucursalNombre   string `json:"sucursalNombre" bson:"sucursalnombre"`
	// TicketPOS decide por dónde sale el comprobante del mostrador.
	TicketPOS   string `json:"ticketPOS" bson:"ticketpos"`
	Actualizada string `json:"actualizada" bson:"actualizada"`
}

// Ambientes y URLs.
const (
	AmbienteQA         = "qa"
	AmbienteProduccion = "produccion"
	BaseURLQA          = "https://qa.unidigital.global/digitalinvoice-core"
	BaseURLProduccion  = "https://www.unidigital.global/digitalinvoice-core"
)

/* POR DÓNDE SALE EL TICKET DEL MOSTRADOR.
 *
 * REGLA QUE NO ES CONFIGURABLE: si la imprenta digital emite la factura fiscal,
 * la impresora fiscal de la caja NO puede emitir otra. Serían dos documentos
 * fiscales por una sola venta. Lo que sale por la caja es un COMPROBANTE NO
 * FISCAL —una nota de entrega con el QR— y eso vale para las tres opciones.
 *
 * Lo que sí se elige es el aparato, porque el parque instalado varía: hay locales
 * con impresora fiscal que pasan a digital y quieren seguir usándola en modo no
 * fiscal, y locales que van a digital justamente porque no tienen ninguna.
 */
const (
	// TicketTermica: una impresora térmica común (la de comandas). Sin ambigüedad:
	// esa máquina nunca emitió documentos fiscales.
	TicketTermica = "termica"
	// TicketFiscalNoFiscal: la impresora fiscal, emitiendo un comprobante NO
	// fiscal. Requiere que el modelo lo soporte.
	TicketFiscalNoFiscal = "fiscal_no_fiscal"
	// TicketNinguno: no se imprime nada; el cliente recibe el enlace por otra vía.
	TicketNinguno = "ninguno"
)

// BaseURL devuelve la URL de la API según el ambiente configurado.
func (c Config) BaseURL() string {
	if c.Ambiente == AmbienteProduccion {
		return BaseURLProduccion
	}
	return BaseURLQA
}

// Lista indica si la configuración alcanza para emitir. Sin serie o sin sucursal
// la imprenta rechaza el documento, así que es mejor no dejar encender el módulo
// que fallar en la primera venta.
func (c Config) Lista() bool {
	return c.Activa && strings.TrimSpace(c.Usuario) != "" && strings.TrimSpace(c.PasswordSHA512) != "" &&
		strings.TrimSpace(c.SerieStrongID) != "" && strings.TrimSpace(c.SucursalStrongID) != ""
}

// Canales de emisión.
const (
	CanalPOS    = "pos"
	CanalVentas = "ventas"
)

// AplicaA indica si un canal debe facturarse digitalmente.
func (c Config) AplicaA(canal string) bool {
	if !c.Lista() {
		return false
	}
	switch canal {
	case CanalPOS:
		return c.PorPOS
	case CanalVentas:
		return c.PorVentas
	}
	return false
}

// Repositorios.
type ConfigRepository interface {
	Get(empresaID string) (Config, bool)
	Upsert(c Config) Config
}

// EmisionRepository persiste el outbox. Append de la emisión y Update de su
// avance; nunca Delete: es la tabla de conciliación.
type EmisionRepository interface {
	Append(e Emision) Emision
	Update(e Emision) (Emision, bool)
	ByID(empresaID, id string) (Emision, bool)
	ByDocumento(empresaID, documentoID string) (Emision, bool)
	ByToken(token string) (Emision, bool)
	List(empresaID string) []Emision
	// Pendientes son las emisiones que el lazo de fondo tiene que atender, de
	// TODAS las empresas: el trabajador corre por instancia, no por tenant.
	Pendientes(limite int) []Emision
}
