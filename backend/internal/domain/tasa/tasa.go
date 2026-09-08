// Package tasa modela la tasa de cambio Bs/US$ con la que ElERP convierte
// entre monedas.
//
// Decisión de diseño (R9 del backlog): la tasa NO SE TECLEA. Entra sola desde
// una fuente oficial, y la interfaz siempre declara de dónde salió y de qué día
// es — nunca se rotula «BCV» una cifra que escribió una persona.
//
// El histórico es de SOLO-ANEXADO, igual que los ledgers fiscal y de inventario:
// una tasa registrada no se edita ni se borra. Cambiar la tasa viva jamás puede
// reescribir la tasa que un documento ya emitido copió al emitirse (Art. 177);
// esa vive en el documento, no aquí.
package tasa

import (
	"context"
	"strings"
)

// MonedaUSD es la divisa por defecto de una tasa. Todo el histórico anterior al
// soporte multimoneda se registró SIN campo de moneda: por retrocompatibilidad,
// un `Moneda` vacío se lee siempre como "USD" (ver NormalizarMoneda).
const MonedaUSD = "USD"

// NormalizarMoneda pasa un código de moneda a su forma canónica (mayúsculas, sin
// espacios) y trata el vacío como "USD": así los registros viejos —que no tienen
// el campo— siguen siendo tasas del dólar sin migración.
func NormalizarMoneda(m string) string {
	m = strings.ToUpper(strings.TrimSpace(m))
	if m == "" {
		return MonedaUSD
	}
	return m
}

// Fuentes posibles de una tasa, de más a menos autoritativa.
const (
	// FuenteBCV: raspado del sitio del Banco Central de Venezuela. Es la fuente
	// primaria: el BCV no publica API, su tasa vive en una página HTML.
	FuenteBCV = "bcv"
	// FuenteRespaldo: API de terceros que republica la tasa oficial del BCV.
	// Se usa solo cuando el sitio del BCV no responde o cambió su HTML.
	FuenteRespaldo = "respaldo"
	// FuenteMercado: promedio del mercado. NO es la tasa oficial; solo se usa
	// si la empresa la eligió explícitamente, y la interfaz lo dice.
	FuenteMercado = "mercado"
	// FuenteManual: carga de un administrador. Último recurso, siempre auditada
	// con el usuario que la hizo.
	FuenteManual = "manual"
	// FuenteSemilla: dato de demostración del modo demo. Existe para que la
	// interfaz no mienta llamando «BCV» a una cifra sembrada.
	FuenteSemilla = "semilla"
)

// Estados de un registro de tasa.
const (
	// EstadoVigente: la tasa pasó las validaciones y se puede usar para calcular.
	EstadoVigente = "vigente"
	// EstadoRechazada: CUARENTENA. La lectura llegó fuera del rango plausible o
	// con una variación anómala contra la última conocida, así que no se aplica.
	// No se descarta en silencio: queda registrada para que una administradora
	// la apruebe a mano si la devaluación fue real.
	EstadoRechazada = "rechazada"
)

// Límites de plausibilidad. Son la defensa de la condición 2 de seguridad (R9):
// el dato que llega de una fuente externa no se confía nunca.
const (
	// MinPlausible / MaxPlausible: rango absoluto de Bs por US$. Amplio a
	// propósito — Venezuela ha reconvertido su moneda tres veces — pero cierra
	// la puerta a un 0, un negativo o una cifra absurda inyectada.
	MinPlausible float64 = 0.5
	MaxPlausible float64 = 10_000_000
	// VariacionMax: variación relativa máxima aceptada contra la última tasa
	// vigente conocida (20%). Por encima, la lectura va a cuarentena.
	VariacionMax = 0.20
)

// Tasa es un registro inmutable del histórico de tasas.
type Tasa struct {
	ID string `json:"id" bson:"id"`
	// EmpresaID vacío significa TASA DE PLATAFORMA: la del BCV es del país, no
	// de un tenant, y se obtiene una vez por instancia (condición 7: una
	// consulta al día, no una por empresa). Las cargas manuales sí llevan
	// empresaID porque son una decisión de esa empresa.
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// Moneda es el código ISO de la divisa a la que corresponde esta tasa
	// ("USD", "EUR", …). VACÍO se lee como "USD" (retrocompat: el histórico
	// anterior al multimoneda es todo del dólar). `Valor` es entonces bolívares
	// por 1 unidad de ESTA moneda.
	Moneda string `json:"moneda" bson:"moneda"`
	// Valor en bolívares por una unidad de la moneda (por defecto, por un dólar).
	Valor  float64 `json:"valor" bson:"valor"`
	Fuente string  `json:"fuente" bson:"fuente"`
	// FechaValor es el día al que corresponde la tasa (YYYY-MM-DD), no el día en
	// que se consultó: el BCV publica hoy la tasa que rige hoy.
	FechaValor string `json:"fechaValor" bson:"fechavalor"`
	// ObtenidaEn es el instante de obtención (UTC RFC3339), para saber qué tan
	// vieja es la cifra que la caja está usando.
	ObtenidaEn string `json:"obtenidaEn" bson:"obtenidaen"`
	// Actor es quién la trajo: "sistema" cuando la trajo la sincronización, o el
	// id del usuario que la cargó a mano (condición 8: auditable).
	Actor string `json:"actor" bson:"actor"`
	// Detalle describe el origen concreto (el host consultado, p. ej.).
	Detalle string `json:"detalle" bson:"detalle"`
	Estado  string `json:"estado" bson:"estado"`
	// Motivo explica el rechazo cuando el estado es cuarentena.
	Motivo string `json:"motivo" bson:"motivo"`
}

// Vigente indica si la tasa se puede usar para calcular.
func (t Tasa) Vigente() bool { return t.Estado == EstadoVigente }

// Oficial indica si la cifra proviene del BCV (directo o republicado). Sostiene
// el rótulo de la interfaz: solo esto se puede llamar «BCV».
func (t Tasa) Oficial() bool { return t.Fuente == FuenteBCV || t.Fuente == FuenteRespaldo }

// Repository es el puerto del histórico de tasas. SOLO-ANEXADO: no expone
// Update ni Delete a propósito.
//
// Aislamiento de tenant: `empresaID` acota el ámbito. La cadena vacía es el
// ámbito de plataforma (tasa oficial), legible por cualquier tenant y escribible
// solo por la sincronización del servidor.
type Repository interface {
	Append(t Tasa) Tasa
	// UltimaVigente devuelve la tasa vigente más reciente del ámbito.
	UltimaVigente(empresaID string) (Tasa, bool)
	// UltimaVigenteDeFuentes devuelve la tasa vigente más reciente del ámbito
	// limitada a ciertas fuentes (p. ej. solo las oficiales). Implícitamente es
	// del dólar: filtra la moneda "USD" (tratando el vacío como "USD") para no
	// confundir la tasa del USD con la de otra divisa cargada en el mismo ámbito.
	UltimaVigenteDeFuentes(empresaID string, fuentes ...string) (Tasa, bool)
	// UltimaVigenteDeMonedaFuentes es la variante multimoneda: la tasa vigente más
	// reciente del ámbito para una MONEDA concreta (Moneda=="" se trata como
	// "USD"), opcionalmente acotada a ciertas fuentes.
	UltimaVigenteDeMonedaFuentes(empresaID, moneda string, fuentes ...string) (Tasa, bool)
	// Historial devuelve el histórico del ámbito, de más reciente a más
	// antiguo, incluyendo las rechazadas (la cuarentena tiene que ser visible).
	Historial(empresaID string, limite int) []Tasa
	// ByID busca un registro del ámbito, para aprobar una cuarentena.
	ByID(empresaID, id string) (Tasa, bool)
}

// Lectura es lo que devuelve una fuente externa antes de validarse. Es
// deliberadamente pobre: un número, un día y de dónde salió. Nada de la fuente
// externa entra al dominio sin pasar por la validación de la aplicación.
type Lectura struct {
	Valor float64
	// FechaValor en YYYY-MM-DD; vacío si la fuente no la declara.
	FechaValor string
	Fuente     string
	Detalle    string
}

// Proveedor es el puerto de obtención externa de la tasa. Lo implementan los
// adaptadores (raspado del BCV, API de respaldo). Contrato de seguridad:
// solo tráfico saliente, petición anónima (sin credenciales ni datos del
// tenant), TLS verificado, cuerpo acotado y nada de lo que llega se ejecuta.
type Proveedor interface {
	// Nombre identifica la fuente para el registro y la auditoría.
	Nombre() string
	// Obtener consulta la fuente. Debe respetar el contexto (timeout corto): la
	// caja nunca se bloquea esperando una tasa (condición 5).
	Obtener(ctx context.Context) (Lectura, error)
}
