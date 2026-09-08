// Package caja modela la Caja y su sesión de turno: el cuarto nivel de la
// jerarquía (Organización → Empresa → Sede → Caja).
//
// Regla de negocio central, deliberada por diseño (03 §4.3/§4.4 del paquete de
// entrega): NO SE PUEDE FACTURAR SIN UNA CAJA ABIERTA a nombre de un cajero
// identificado con código y PIN. Existe para que cada documento fiscal tenga un
// responsable trazable en el arqueo: sin esto no se sabe quién cobró.
// La validación vive en el backend, nunca solo en la interfaz.
package caja

// Estados de una caja (la configura la Dueña/Admin).
const (
	// EstadoDeshabilitada: recién creada o inhabilitada por el administrador.
	// No admite apertura de turno.
	EstadoDeshabilitada = "deshabilitada"
	// EstadoHabilitada: disponible para que un cajero abra turno.
	EstadoHabilitada = "habilitada"
)

// Caja es un puesto de cobro físico dentro de una sede.
type Caja struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID    string `json:"sedeId" bson:"sedeid"`
	Nombre    string `json:"nombre" bson:"nombre"`
	// Codigo lo genera el servidor (C-001, C-002…), nunca el cliente: es el
	// identificador que el cajero ve en el modal de apertura.
	Codigo string `json:"codigo" bson:"codigo"`
	Estado string `json:"estado" bson:"estado"`
	// DispositivoFiscalID vincula la caja con una impresora fiscal registrada
	// (domain/fiscal.DispositivoFiscal). Opcional: "" significa que la caja no
	// tiene un dispositivo fiscal asignado. La conexión real con el hardware la
	// hace el agente fiscal local; aquí solo se guarda la asociación.
	DispositivoFiscalID string `json:"dispositivoFiscalId" bson:"dispositivofiscalid"`
	// ColorFondo es un OVERRIDE por caja del color de fondo de la pantalla del
	// cliente (hex). Existe para ajustar el contraste puesto a puesto sin tocar la
	// marca de la empresa. Vacío = usar el degradado navy por defecto. Se normaliza
	// a un hex válido o a vacío.
	ColorFondo string `json:"colorFondo" bson:"colorfondo"`
	// LogoVersion elige qué versión del logo de la empresa muestra la pantalla del
	// cliente de esta caja: «principal» (empresa.Logo) o «alterno» (empresa.LogoAlterno).
	// Vacío = usar la principal (el default de empresa). Ver LogoVersionValido.
	LogoVersion string `json:"logoVersion" bson:"logoversion"`
	Creada      string `json:"creada" bson:"creada"`
}

// Versiones del logo que una caja puede elegir para su pantalla del cliente.
const (
	// LogoVersionPrincipal usa empresa.Logo (el default).
	LogoVersionPrincipal = "principal"
	// LogoVersionAlterno usa empresa.LogoAlterno (para el contraste opuesto).
	LogoVersionAlterno = "alterno"
)

// NormalizarLogoVersion acota la versión de logo a «principal» o «alterno». Un
// valor vacío o desconocido queda en «» = usar el default (la principal); así las
// cajas ya creadas no necesitan migración.
func NormalizarLogoVersion(v string) string {
	switch v {
	case LogoVersionPrincipal, LogoVersionAlterno:
		return v
	}
	return ""
}

// Habilitada indica si la caja admite apertura de turno.
func (c Caja) Habilitada() bool { return c.Estado == EstadoHabilitada }

// Cajero es la credencial de puesto: código + PIN con el que una persona abre
// una caja. Es independiente del usuario de la aplicación porque en una tienda
// el turno lo abre quien está en el mostrador, con una credencial corta que se
// teclea rápido y de pie.
type Cajero struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// SedeID acota dónde puede abrir caja: la apertura valida que el cajero
	// pertenezca a la sede de la caja.
	SedeID string `json:"sedeId" bson:"sedeid"`
	Codigo string `json:"codigo" bson:"codigo"` // OP-001
	Nombre string `json:"nombre" bson:"nombre"`
	// PinHash es bcrypt. El PIN en claro no se guarda ni se serializa jamás.
	PinHash string `json:"-" bson:"pinhash"`
	// UsuarioID vincula la credencial de caja con un usuario de la app, cuando
	// esa persona además tiene sesión propia. Opcional.
	UsuarioID string `json:"usuarioId" bson:"usuarioid"`
	// Supervisor habilita a esta credencial para AUTORIZAR lo que el cajero no
	// puede hacer solo: quitar una línea del carrito o salir del modo caja
	// cuando la empresa exige PIN de supervisor (flujo 2.4). Un cajero raso no
	// puede autorizarse a sí mismo.
	Supervisor bool `json:"supervisor" bson:"supervisor"`
	Activo     bool `json:"activo" bson:"activo"`
}

// Sesion es un turno de caja. Append-only en la práctica: se abre y se cierra,
// y el histórico queda para el arqueo. Una sesión sin Cierre está abierta.
type Sesion struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	CajaID    string `json:"cajaId" bson:"cajaid"`
	CajeroID  string `json:"cajeroId" bson:"cajeroid"`
	// CajeroNombre y CajaCodigo se copian al abrir para que el arqueo histórico
	// no dependa de que la caja o el cajero sigan existiendo igual hoy.
	CajeroNombre string `json:"cajeroNombre" bson:"cajeronombre"`
	CajaCodigo   string `json:"cajaCodigo" bson:"cajacodigo"`
	// ActorID es el usuario de la app que tenía la sesión web al abrir. Es el
	// que EmitirFactura compara para saber si puede facturar.
	ActorID  string `json:"actorId" bson:"actorid"`
	Apertura string `json:"apertura" bson:"apertura"` // UTC RFC3339
	Cierre   string `json:"cierre" bson:"cierre"`     // vacío mientras esté abierta
	// FondoInicial es el efectivo en bolívares con el que el cajero abrió la
	// gaveta (base de cambio para dar vuelto). Se fija al abrir y NO cambia al
	// retomar el turno. El efectivo Bs esperado al cierre parte de aquí.
	FondoInicial float64 `json:"fondoInicial" bson:"fondoinicial"`
	// Arqueo es el cuadre del turno, congelado al CERRAR. Nil mientras el turno
	// está abierto. Una vez cerrado es inmutable (append-only): las correcciones
	// se hacen con movimientos posteriores, nunca reescribiendo esta foto.
	Arqueo *Arqueo `json:"arqueo,omitempty" bson:"arqueo,omitempty"`
}

// Abierta indica si el turno sigue vigente.
func (s Sesion) Abierta() bool { return s.Cierre == "" }

// ArqueoMetodo es el desglose ESPERADO de un método de pago en el arqueo del
// turno: el monto en la moneda del método y su equivalente en bolívares. El
// efectivo en divisas se reporta aparte del bolívar (no se mezclan monedas).
type ArqueoMetodo struct {
	Metodo string `json:"metodo" bson:"metodo"`
	Moneda string `json:"moneda" bson:"moneda"`
	// EnDivisa distingue el efectivo/cobro en moneda extranjera del bolívar.
	EnDivisa bool `json:"enDivisa" bson:"endivisa"`
	// Efectivo marca el dinero que vive en la gaveta (se cuenta físicamente),
	// frente a los medios electrónicos (tarjeta, pago móvil, transferencia).
	Efectivo bool `json:"efectivo" bson:"efectivo"`
	// Monto es lo cobrado por este método en SU moneda; EquivalenteBs es su
	// conversión a bolívares con la tasa histórica grabada en cada pago.
	Monto         float64 `json:"monto" bson:"monto"`
	EquivalenteBs float64 `json:"equivalenteBs" bson:"equivalentebs"`
}

// ArqueoConteo es lo que el cajero DECLARÓ contado por un método al cerrar. Es
// opcional (además del efectivo Bs, que es el conteo mínimo).
type ArqueoConteo struct {
	Metodo string  `json:"metodo" bson:"metodo"`
	Moneda string  `json:"moneda" bson:"moneda"`
	Monto  float64 `json:"monto" bson:"monto"`
}

// Arqueo es el cuadre de un turno de caja. Lo ESPERADO se deriva plegando todos
// los documentos cobrados en la sesión (por su SesionCajaID); lo CONTADO lo
// declara el cajero al cerrar; la DIFERENCIA del efectivo Bs es contado −
// esperado (>0 sobrante, <0 faltante). Se congela al cerrar y no se edita.
type Arqueo struct {
	// FondoInicial se copia de la sesión para que la foto sea autocontenida.
	FondoInicial float64 `json:"fondoInicial" bson:"fondoinicial"`
	// Metodos es el desglose de lo cobrado (bruto) por método de pago.
	Metodos []ArqueoMetodo `json:"metodos" bson:"metodos"`
	// Efectivo Bs esperado en la gaveta = FondoInicial + CobrosEfectivoBs −
	// VueltoEfectivoBs (lo que salió como vuelto en efectivo Bs).
	CobrosEfectivoBs   float64 `json:"cobrosEfectivoBs" bson:"cobrosefectivobs"`
	VueltoEfectivoBs   float64 `json:"vueltoEfectivoBs" bson:"vueltoefectivobs"`
	EfectivoEsperadoBs float64 `json:"efectivoEsperadoBs" bson:"efectivoesperadobs"`
	// TotalCobradoBs es todo lo cobrado del turno en bolívares (neto de vuelto);
	// Documentos es cuántas facturas se plegaron.
	TotalCobradoBs float64 `json:"totalCobradoBs" bson:"totalcobradobs"`
	Documentos     int     `json:"documentos" bson:"documentos"`
	// Declarado indica si el cajero realmente contó al cerrar (vs. un cierre
	// forzado por administración que no cuenta). Sin declarar, la diferencia no
	// tiene sentido y queda en cero.
	Declarado bool `json:"declarado" bson:"declarado"`
	// EfectivoContadoBs es el efectivo Bs que el cajero declaró en la gaveta, y
	// DiferenciaBs la resta contra lo esperado (sobrante/faltante).
	EfectivoContadoBs float64 `json:"efectivoContadoBs" bson:"efectivocontadobs"`
	DiferenciaBs      float64 `json:"diferenciaBs" bson:"diferenciabs"`
	// ContadoPorMetodo es el conteo declarado por método (opcional, para el acta).
	ContadoPorMetodo []ArqueoConteo `json:"contadoPorMetodo,omitempty" bson:"contadopormetodo,omitempty"`
}

// CajaRepo es el puerto de persistencia de cajas. Toda consulta se aísla por
// empresaID (el tenant): Mongo no tiene Row-Level Security.
type CajaRepo interface {
	List(empresaID string) []Caja
	ByID(empresaID, id string) (Caja, bool)
	Create(c Caja) Caja
	Update(c Caja) (Caja, bool)
}

// CajeroRepo es el puerto de credenciales de caja.
type CajeroRepo interface {
	List(empresaID string) []Cajero
	ByCodigo(empresaID, codigo string) (Cajero, bool)
	Create(c Cajero) Cajero
	Update(c Cajero) (Cajero, bool)
}

// SesionRepo es el puerto de turnos de caja.
type SesionRepo interface {
	// Abiertas devuelve los turnos vigentes de una empresa (opcionalmente de
	// una sede). Sostiene el modal de apertura: qué cajas están tomadas.
	Abiertas(empresaID, sedeID string) []Sesion
	// AbiertaDeCaja devuelve el turno vigente de una caja concreta, si existe.
	AbiertaDeCaja(empresaID, cajaID string) (Sesion, bool)
	// AbiertaDeActor devuelve el turno vigente asociado a un usuario de la app.
	// Es la consulta que usa la regla "sin caja abierta no se factura".
	AbiertaDeActor(empresaID, actorID string) (Sesion, bool)
	List(empresaID string) []Sesion
	Create(s Sesion) Sesion
	Update(s Sesion) (Sesion, bool)
}
