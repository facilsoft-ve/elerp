// Package mesonero modela la CREDENCIAL DE TURNO del mesonero y su TURNO (la
// jornada de trabajo) dentro del módulo Restaurante.
//
// EL PROBLEMA QUE RESUELVE: hasta ahora el mesonero entraba como cualquier
// usuario de oficina — correo y contraseña. En un salón eso no funciona: se
// teclea de pie, en una tablet compartida, al empezar el servicio. Lo que pasa
// de verdad es que nadie cierra sesión y todos trabajan con la del primero, y
// ahí se pierde de quién es cada mesa (y con ella la comisión).
//
// Es el mismo problema que ya resolvió la caja, y por eso este paquete es el
// espejo de `domain/caja`: allá `Cajero` es la «credencial de puesto: código +
// PIN, independiente del usuario de la app, porque el turno lo abre quien está
// en el mostrador», y `Sesion` es el turno con su arqueo congelado al cerrar.
// Acá `Mesonero` y `Turno` son lo mismo para el salón. Códigos MS- (mesoneros),
// para no confundirlos con los OP- de los cajeros ni los C- de las cajas.
//
// DOS REGLAS QUE VIVEN EN EL SERVIDOR, NO EN LA PANTALLA:
//
//  1. Un turno SIEMPRE lo valida un supervisor. Sin turno abierto el PIN del
//     mesonero no abre nada: es una credencial válida sin un contexto donde
//     servir. Eso es lo que impide entrar de madrugada, desde casa, «porque me
//     sé mi PIN».
//
//  2. El turno no se apaga de golpe: pasa por CERRANDO. Un turno no termina
//     cuando alguien toca un botón, termina cuando se va el último cliente. En
//     cerrando el mesonero ya NO toma mesas nuevas pero sigue atendiendo las
//     suyas, y al cerrar la última el turno se cierra solo.
package mesonero

import "strings"

// Estados del turno. No hay «pendiente»: un turno existe porque un supervisor lo
// validó, así que nace abierto. Lo que no fue validado sencillamente no es un
// turno.
const (
	// EstadoAbierto: el mesonero trabaja con normalidad y puede tomar mesas.
	EstadoAbierto = "abierto"
	// EstadoCerrando es el CIERRE SUAVE: el turno ya terminó para efectos de
	// tomar mesas nuevas, pero las que tiene abiertas las sigue atendiendo hasta
	// cerrarlas. Se entra acá por decisión del supervisor o porque se cumplió el
	// horario; se sale solo, al cerrar la última cuenta.
	EstadoCerrando = "cerrando"
	// EstadoCerrado: terminado. Lleva su resumen congelado.
	EstadoCerrado = "cerrado"
)

// EstadoValido indica si el estado es uno de los tres admitidos.
func EstadoValido(e string) bool {
	return e == EstadoAbierto || e == EstadoCerrando || e == EstadoCerrado
}

// Mesonero es la CREDENCIAL DE TURNO: código corto + PIN con el que una persona
// del salón entra a trabajar. No reemplaza al usuario de la aplicación: lo
// acompaña. El `UsuarioID` es obligatorio (a diferencia del cajero, donde es
// opcional) porque las cuentas de mesa y las asignaciones del salón ya se
// llevan por usuario, y partir esa identidad en dos dejaría mesas sin dueño.
type Mesonero struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	// SedeID acota dónde trabaja: un mesonero pertenece a un salón, no a la
	// empresa entera (igual que el cajero y su sede).
	SedeID string `json:"sedeId" bson:"sedeid"`
	Codigo string `json:"codigo" bson:"codigo"` // MS-001
	Nombre string `json:"nombre" bson:"nombre"`
	// UsuarioID es el usuario de la app al que pertenece esta credencial. Es el
	// mismo id que llevan cuenta.Cuenta.MesoneroID y mesa.Asignacion.UsuarioID.
	UsuarioID string `json:"usuarioId" bson:"usuarioid"`
	// PinHash es bcrypt. VACÍO significa que la persona todavía no fijó su PIN:
	// aparece en la grilla marcada «sin PIN» y lo elige ella misma la primera
	// vez, con un supervisor al lado autorizando. Nadie más conoce ese PIN — eso
	// importa porque el turno respalda la comisión.
	PinHash string `json:"-" bson:"pinhash"`
	Activo  bool   `json:"activo" bson:"activo"`
	Creado  string `json:"creado" bson:"creado"` // UTC RFC3339
}

// TienePin indica si la persona ya fijó su PIN. Mientras sea falso no puede
// iniciar turno: primero lo fija (con autorización del supervisor).
func (m Mesonero) TienePin() bool { return strings.TrimSpace(m.PinHash) != "" }

// Resumen es la foto del turno, CONGELADA al cerrar. Las cifras se derivan de
// las cuentas del turno —nunca son contadores que alguien incrementa—, pero una
// vez cerrado el turno no se recalculan: si mañana una mesa se reasigna, la foto
// de anoche no puede cambiar. Misma regla que el arqueo de caja.
type Resumen struct {
	// MinutosTrabajados va de la apertura al cierre real (incluye el rato en
	// cerrando: atender las mesas que quedaban también es trabajo).
	MinutosTrabajados int `json:"minutosTrabajados" bson:"minutostrabajados"`
	// Mesas son las cuentas que atendió; Personas la suma de comensales de esas
	// cuentas; Ordenes las rondas que mandó a cocina.
	Mesas    int `json:"mesas" bson:"mesas"`
	Personas int `json:"personas" bson:"personas"`
	Ordenes  int `json:"ordenes" bson:"ordenes"`
	// TotalFacturado es la suma de las cuentas del turno (Bs) y TicketPromedio
	// ese total entre las mesas atendidas. Se guardan los dos porque el promedio
	// solo no deja reconstruir el total, y el total solo no se compara entre
	// turnos de distinta duración.
	TotalFacturado float64 `json:"totalFacturado" bson:"totalfacturado"`
	TicketPromedio float64 `json:"ticketPromedio" bson:"ticketpromedio"`
}

// Turno es la jornada de un mesonero. Append-only en la práctica, como la sesión
// de caja: se abre, pasa por cerrando y se cierra; el histórico queda.
type Turno struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// MesoneroID es la credencial; Codigo, Nombre y UsuarioID se COPIAN al abrir
	// para que el histórico no dependa de que la credencial siga existiendo igual
	// hoy (mismo criterio que CajeroNombre/CajaCodigo en la sesión de caja).
	MesoneroID     string `json:"mesoneroId" bson:"mesoneroid"`
	MesoneroCodigo string `json:"mesoneroCodigo" bson:"mesonerocodigo"`
	MesoneroNombre string `json:"mesoneroNombre" bson:"mesoneronombre"`
	UsuarioID      string `json:"usuarioId" bson:"usuarioid"`
	Estado         string `json:"estado" bson:"estado"`
	Apertura       string `json:"apertura" bson:"apertura"` // UTC RFC3339
	// ValidadoPor es el supervisor que autorizó la apertura ("OP-001 Ana"). Nunca
	// va vacío: un turno sin autorización no se crea.
	ValidadoPor string `json:"validadoPor" bson:"validadopor"`
	// FinPrevisto es la hora a la que debería terminar según su horario (RFC3339).
	// Vacío cuando no hay horario configurado. Hoy es informativo y la base sobre
	// la que se apoyará el cierre automático.
	FinPrevisto string `json:"finPrevisto,omitempty" bson:"finprevisto,omitempty"`
	// CerrandoDesde marca cuándo entró en cierre suave; vacío si nunca pasó por
	// ahí (un turno se puede cerrar de golpe si no tenía mesas abiertas).
	CerrandoDesde string `json:"cerrandoDesde,omitempty" bson:"cerrandodesde,omitempty"`
	// CerrandoMotivo dice QUIÉN lo mandó a cerrar: el horario o una persona
	// (CerrandoPorHorario | CerrandoPorSupervisor). Importa porque solo el
	// vencimiento del horario se puede revertir aprobando tiempo extra: revertir
	// una orden del supervisor sería deshacerla en silencio.
	CerrandoMotivo string `json:"cerrandoMotivo,omitempty" bson:"cerrandomotivo,omitempty"`
	// FueraDeHorario queda marcado cuando el turno se abrió fuera de los tramos
	// declarados de esa persona. No bloquea nada —el supervisor ya autorizó— pero
	// deja el hecho en el registro, que es de lo que sirve tener horarios.
	FueraDeHorario bool `json:"fueraDeHorario,omitempty" bson:"fueradehorario,omitempty"`
	// ExtensionPor es el supervisor que aprobó tiempo extra (vacío si no hubo).
	// El nuevo fin queda en FinPrevisto; acá se guarda quién lo autorizó y cuánto,
	// porque el tiempo extra se paga y tiene que poder responderse quién lo dio.
	ExtensionPor     string `json:"extensionPor,omitempty" bson:"extensionpor,omitempty"`
	ExtensionMinutos int    `json:"extensionMinutos,omitempty" bson:"extensionminutos,omitempty"`
	Cierre           string `json:"cierre,omitempty" bson:"cierre,omitempty"`
	// CerradoPor queda cuando lo cerró una persona (el supervisor que lo forzó).
	// Vacío cuando el turno se cerró SOLO, al cerrarse su última cuenta: esa
	// distinción es la que después explica un cierre a destiempo.
	CerradoPor string `json:"cerradoPor,omitempty" bson:"cerradopor,omitempty"`
	// Resumen es la foto congelada al cerrar. Nil mientras el turno sigue vivo.
	Resumen *Resumen `json:"resumen,omitempty" bson:"resumen,omitempty"`
}

// Vivo indica si el turno sigue en pie: abierto o cerrando. Es la pregunta que
// responde «¿esta persona está trabajando ahora?» y la que habilita atender.
func (t Turno) Vivo() bool { return t.Estado == EstadoAbierto || t.Estado == EstadoCerrando }

// PuedeTomarMesas indica si el mesonero puede abrir mesas NUEVAS. En cerrando la
// respuesta es no: sigue atendiendo lo suyo, pero ya no se le carga más trabajo.
// Esta es la regla que hace real el cierre suave.
func (t Turno) PuedeTomarMesas() bool { return t.Estado == EstadoAbierto }

// Repository persiste las credenciales de mesonero. Filtro por empresa
// obligatorio en toda consulta: Mongo no tiene Row-Level Security. Sin borrado
// duro — se desactiva con Activo=false, igual que el resto de los maestros.
type Repository interface {
	List(empresaID string) []Mesonero
	ByID(empresaID, id string) (Mesonero, bool)
	ByCodigo(empresaID, codigo string) (Mesonero, bool)
	// ByUsuario resuelve la credencial desde el usuario de la app. Es la consulta
	// que responde «¿el que está pidiendo esto tiene turno?».
	ByUsuario(empresaID, usuarioID string) (Mesonero, bool)
	Create(m Mesonero) Mesonero
	Update(m Mesonero) (Mesonero, bool)
}

// TurnoRepository persiste los turnos. Mismo aislamiento por empresa.
type TurnoRepository interface {
	// Vivos devuelve los turnos abiertos o cerrando de una sede: es el «quién
	// está trabajando ahora» que sostiene la grilla y el reparto de mesas.
	Vivos(empresaID, sedeID string) []Turno
	// VivoDeMesonero devuelve el turno en pie de un mesonero, si lo tiene. Es lo
	// que impide abrir dos turnos a la misma persona.
	VivoDeMesonero(empresaID, mesoneroID string) (Turno, bool)
	ByID(empresaID, id string) (Turno, bool)
	// Historico devuelve los turnos de una sede, del más reciente al más viejo.
	Historico(empresaID, sedeID string) []Turno
	Create(t Turno) Turno
	Update(t Turno) (Turno, bool)
}
