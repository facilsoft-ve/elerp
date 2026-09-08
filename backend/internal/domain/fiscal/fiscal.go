// Package fiscal modela los documentos fiscales de ElERP.
//
// Integridad (§13.2 / Providencia 000121): los documentos son INMUTABLES y de
// solo-anexado. Una factura nunca se edita ni se borra; las correcciones son
// documentos nuevos (nota de crédito / anulación) que referencian al original.
// El estado "anulado" es DERIVADO: existe una reversa que apunta al documento.
package fiscal

import (
	"errors"
	"fmt"
)

// ErrFolioRetrocede se devuelve cuando se intenta fijar la numeración de una
// serie por DEBAJO del último folio ya entregado. Es una regla legal dura: un
// folio ya usado no se puede reasignar, así que la numeración solo avanza
// (forward-only). Continuar desde un sistema previo adelanta el contador; nunca
// lo devuelve atrás.
var ErrFolioRetrocede = errors.New("la numeración solo avanza: el próximo folio no puede ser menor o igual al último ya entregado (los folios usados no se reasignan)")

// Tipos de documento.
const (
	TipoFactura     = "factura"
	TipoNotaCredito = "nota_credito"
	TipoNotaDebito  = "nota_debito" // cargo adicional que AUMENTA sobre una factura
	TipoAnulacion   = "anulacion"   // reversa total de una factura
)

// Medios por los que la caja entrega el VUELTO. Por defecto es efectivo (la
// gaveta); pago_movil registra la INSTRUCCIÓN de transferirle el vuelto al
// cliente a su banco/cédula/teléfono (la ejecución real contra el banco es una
// integración futura: aquí solo se deja constancia de la intención).
const (
	VueltoEfectivo  = "efectivo"
	VueltoPagoMovil = "pago_movil"
)

// Métodos de pago.
const (
	PagoEfectivoBs  = "efectivo_bs"
	PagoEfectivoUSD = "efectivo_usd"
	PagoPagoMovil   = "pago_movil"
	PagoZelle       = "zelle"
	PagoTarjeta     = "tarjeta"
	PagoTransfer    = "transferencia"
)

// Alícuotas base (parametrizables en un motor de reglas en el futuro).
const (
	AlicuotaIVA  = 0.16 // IVA general Venezuela
	AlicuotaIGTF = 0.03 // IGTF sobre pagos en divisas
)

// Linea es un renglón del documento.
type Linea struct {
	ProductoID     string  `json:"productoId" bson:"productoid"`
	SKU            string  `json:"sku" bson:"sku"`
	Nombre         string  `json:"nombre" bson:"nombre"`
	Cantidad       float64 `json:"cantidad" bson:"cantidad"`
	PrecioUnitario float64 `json:"precioUnitario" bson:"preciounitario"`
	Total          float64 `json:"total" bson:"total"`
	// Exento marca el renglón como NO gravado con IVA. En Venezuela buena parte
	// de la cesta básica lo está (harina de maíz, arroz), y la factura tiene que
	// separar base imponible de base exenta: el IVA se calcula solo sobre la
	// primera. Se copia del producto al emitir, para que el documento no dependa
	// de que el catálogo siga clasificado igual mañana.
	Exento bool `json:"exento" bson:"exento"`
	// Insumos es el SNAPSHOT de la receta (escandallo) del plato AL MOMENTO de
	// facturar: qué insumos consume UNA unidad. Vacío ⇒ producto normal (el stock
	// que se descuenta es el propio SKU). Cuando trae insumos, el inventario
	// descuenta ESTOS (cantidad × cantidad del renglón), no el SKU del plato; y la
	// anulación/NC reingresa exactamente lo mismo, sin depender de que la receta del
	// catálogo siga igual mañana.
	Insumos []InsumoLinea `json:"insumos,omitempty" bson:"insumos,omitempty"`
}

// InsumoLinea es un insumo consumido por UNA unidad de un plato (snapshot en la
// línea fiscal). CantidadUnitaria es por unidad del plato; el consumo total del
// renglón es CantidadUnitaria × Linea.Cantidad.
type InsumoLinea struct {
	SKU              string  `json:"sku" bson:"sku"`
	ProductoID       string  `json:"productoId" bson:"productoid"`
	Nombre           string  `json:"nombre" bson:"nombre"`
	CantidadUnitaria float64 `json:"cantidadUnitaria" bson:"cantidadunitaria"`
}

// Pago es una línea de cobro (cobro mixto multi-cuenta).
type Pago struct {
	Metodo   string  `json:"metodo" bson:"metodo"`
	CuentaID string  `json:"cuentaId" bson:"cuentaid"`
	Monto    float64 `json:"monto" bson:"monto"`
	Moneda   string  `json:"moneda" bson:"moneda"` // VES | USD | EUR | …
	EnDivisa bool    `json:"enDivisa" bson:"endivisa"`
	// TasaCambio es la tasa (Bs por 1 unidad de la MONEDA DE ESTE PAGO) con la que
	// se convirtió a bolívares al emitir. Multimoneda: cada pago lleva la tasa de
	// SU divisa (USD, EUR, …), no una tasa única del documento. VES ⇒ 1 (no
	// convierte). Se graba porque la conversión de cada cobro es memoria histórica
	// (Art. 177) y el arqueo/IGTF de la caja dependen de ella.
	TasaCambio float64 `json:"tasaCambio" bson:"tasacambio"`
	// Referencia del pago electrónico (últimos dígitos del pago móvil, de la
	// transferencia o del envío por Zelle). Es lo que permite conciliar contra el
	// banco más adelante; sin ella un cobro electrónico no se puede rastrear.
	Referencia string `json:"referencia" bson:"referencia"`
}

// VueltoParte es una porción del vuelto entregado al cliente. El vuelto puede
// repartirse en VARIAS partes, cada una con su moneda y su medio: p. ej. se
// deben US$23 → US$20 en efectivo + el resto en Bs por pago móvil. Monto va en
// la moneda de la parte; MontoBs es su equivalente en bolívares (con la tasa de
// esa moneda al emitir). Banco/Cédula/Teléfono solo aplican al pago móvil.
type VueltoParte struct {
	Moneda   string  `json:"moneda" bson:"moneda"`
	Metodo   string  `json:"metodo" bson:"metodo"` // efectivo | pago_movil
	Monto    float64 `json:"monto" bson:"monto"`   // en la moneda de la parte
	MontoBs  float64 `json:"montoBs" bson:"montobs"`
	Banco    string  `json:"banco" bson:"banco"`
	Cedula   string  `json:"cedula" bson:"cedula"`
	Telefono string  `json:"telefono" bson:"telefono"`
}

// Documento es un documento fiscal inmutable.
type Documento struct {
	ID             string `json:"id" bson:"id"`
	EmpresaID      string `json:"empresaId" bson:"empresaid"`
	SedeID         string `json:"sedeId" bson:"sedeid"`
	Tipo           string `json:"tipo" bson:"tipo"`
	Serie          string `json:"serie" bson:"serie"`
	Numero         int    `json:"numero" bson:"numero"`
	NumeroCompleto string `json:"numeroCompleto" bson:"numerocompleto"`
	// NumeroControl es el "Número de Control" que exige el SENIAT para forma libre e
	// imprenta digital: un correlativo PROPIO, distinto del número de factura,
	// autorizado por rango. En máquina fiscal lo asigna la impresora (queda vacío).
	NumeroControl string `json:"numeroControl" bson:"numerocontrol"`
	Modalidad      string `json:"modalidad" bson:"modalidad"`
	Contingencia   bool   `json:"contingencia" bson:"contingencia"`

	ClienteID        string `json:"clienteId" bson:"clienteid"`
	ClienteNombre    string `json:"clienteNombre" bson:"clientenombre"`
	ClienteDocumento string `json:"clienteDocumento" bson:"clientedocumento"`

	Lineas   []Linea `json:"lineas" bson:"lineas"`
	Subtotal float64 `json:"subtotal" bson:"subtotal"`
	// BaseImponible y BaseExenta descomponen el subtotal. El IVA sale solo de la
	// base imponible; la exenta se declara aparte en los libros fiscales.
	BaseImponible float64 `json:"baseImponible" bson:"baseimponible"`
	BaseExenta    float64 `json:"baseExenta" bson:"baseexenta"`
	IVA           float64 `json:"iva" bson:"iva"`
	IGTF          float64 `json:"igtf" bson:"igtf"`
	Total         float64 `json:"total" bson:"total"`
	// AlicuotaIVA y AlicuotaIGTF son las tasas EFECTIVAMENTE aplicadas al emitir
	// este documento (fracciones: 0.16 = 16%). Se graban para cumplir el ADR de
	// tasa histórica: una nota de crédito o la base de IGTF derivan de la tasa DEL
	// DOCUMENTO, nunca de la config actual de la empresa —así el histórico cuadra
	// aunque la alícuota cambie después (Providencia SENIAT). 0 = tasa del sistema
	// vigente al emitir (documentos previos a esta configuración).
	AlicuotaIVA  float64 `json:"alicuotaIVA" bson:"alicuotaiva"`
	AlicuotaIGTF float64 `json:"alicuotaIGTF" bson:"alicuotaigtf"`

	Moneda     string  `json:"moneda" bson:"moneda"`
	TasaCambio float64 `json:"tasaCambio" bson:"tasacambio"` // memoria histórica (Art. 177)
	// TasaFuente registra DE DÓNDE salió esa tasa (bcv, respaldo, mercado,
	// manual). Sin esto, ante el SENIAT la tasa histórica no es demostrable: hay
	// que poder decir que la cifra vino del BCV y no del teclado de alguien.
	// 0 en TasaCambio con TasaFuente vacía significa que no hubo conversión: la
	// venta fue íntegramente en bolívares.
	TasaFuente string `json:"tasaFuente" bson:"tasafuente"`

	Pagos []Pago `json:"pagos" bson:"pagos"`
	// Cobrado es lo que efectivamente entregó el cliente, expresado en bolívares
	// (los pagos en divisas se convierten con TasaCambio). Vuelto es el excedente
	// que hay que devolverle, en la moneda VueltoMoneda: el efectivo en dólares se
	// devuelve en dólares. Se guardan porque son parte del arqueo de la caja —
	// sin ellos no se puede cuadrar el turno contra lo que hay en la gaveta.
	Cobrado float64 `json:"cobrado" bson:"cobrado"`
	// Credito marca una venta que el cliente NO pagó completa en el mostrador: lo
	// que falta queda por cobrar (Tesorería). VenceEl es la fecha de pago
	// acordada, que es lo que hace que un saldo pueda estar «vencido».
	Credito      bool    `json:"credito" bson:"credito"`
	VenceEl      string  `json:"venceEl" bson:"venceel"` // YYYY-MM-DD
	Vuelto       float64 `json:"vuelto" bson:"vuelto"`
	VueltoMoneda string  `json:"vueltoMoneda" bson:"vueltomoneda"`
	// VueltoMetodo es el medio por el que se entrega el vuelto: "efectivo" (por
	// defecto, la gaveta) o "pago_movil". La moneda del vuelto (VueltoMoneda) la
	// DECLARA la caja: puede dar el vuelto en Bs aunque haya cobrado en divisas
	// (quizá no tiene billetes de esa divisa). Sin vuelto, estos campos van vacíos.
	VueltoMetodo string `json:"vueltoMetodo" bson:"vueltometodo"`
	// Datos del pago móvil del vuelto: solo aplican cuando VueltoMetodo ==
	// "pago_movil". Registran a quién se le transfiere el vuelto (banco, cédula y
	// teléfono del cliente); la transferencia real es una integración futura.
	VueltoBanco    string `json:"vueltoBanco" bson:"vueltobanco"`
	VueltoCedula   string `json:"vueltoCedula" bson:"vueltocedula"`
	VueltoTelefono string `json:"vueltoTelefono" bson:"vueltotelefono"`
	// VueltoPartes reparte el vuelto en varias partes, cada una con su moneda y su
	// medio. Es la forma RICA del vuelto; los campos únicos de arriba se conservan
	// como RESUMEN retrocompatible: una sola parte ⇒ se comportan igual que antes
	// (Vuelto en VueltoMoneda); vuelto MIXTO ⇒ Vuelto es el total en Bs y
	// VueltoMoneda/VueltoMetodo quedan vacíos. El arqueo y la tesorería pliegan
	// estas partes (con fallback a los campos únicos para documentos antiguos).
	VueltoPartes []VueltoParte `json:"vueltoPartes" bson:"vueltopartes"`

	RefDocumentoID string `json:"refDocumentoId" bson:"refdocumentoid"` // original referenciado (nota/anulación)
	Motivo         string `json:"motivo" bson:"motivo"`

	Actor string `json:"actor" bson:"actor"`
	Fecha string `json:"fecha" bson:"fecha"` // UTC RFC3339

	// Trazabilidad de arqueo: en qué caja y por quién se cobró. Se copian al
	// emitir para que el histórico no dependa de que la caja siga existiendo.
	CajaID       string `json:"cajaId" bson:"cajaid"`
	CajaCodigo   string `json:"cajaCodigo" bson:"cajacodigo"`
	CajeroNombre string `json:"cajeroNombre" bson:"cajeronombre"`
	SesionCajaID string `json:"sesionCajaId" bson:"sesioncajaid"`

	// Sello de integridad (encadenamiento por hash), asignado por el repositorio al
	// anexar. PrevHash = Hash del documento anterior de la empresa; Hash sella este.
	// Vacío = anterior a la activación del sello. No entran en Contenido().
	PrevHash string `json:"prevHash" bson:"prevhash"`
	Hash     string `json:"hash" bson:"hash"`
}

// Contenido devuelve la representación canónica e inmutable del documento para el
// sello de integridad (excluye PrevHash/Hash). Cubre los campos materiales:
// identidad, numeración, tipo/fecha, cliente, bases e impuestos, total, moneda/
// tasa (memoria histórica Art. 177), crédito, referencia de reversa y líneas.
func (d Documento) Contenido() string {
	s := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%s|%s|%s|%.2f|%.2f|%.2f|%.2f|%.2f|%.2f|%s|%.6f|%t|%s|%s",
		d.ID, d.NumeroCompleto, d.NumeroControl, d.Tipo, d.Serie, d.Numero, d.Fecha,
		d.ClienteID, d.ClienteDocumento, d.Subtotal, d.BaseImponible, d.BaseExenta,
		d.IVA, d.IGTF, d.Total, d.Moneda, d.TasaCambio, d.Credito, d.RefDocumentoID, d.Actor)
	for _, l := range d.Lineas {
		s += fmt.Sprintf("|%s:%.3f:%.2f:%.2f", l.SKU, l.Cantidad, l.PrecioUnitario, l.Total)
	}
	return s
}

// Repository es el puerto de documentos: solo-anexado (Append) + lectura.
// No expone Update ni Delete: la integridad fiscal lo prohíbe.
type Repository interface {
	Append(d Documento) Documento
	List(empresaID string) []Documento
	ByID(empresaID, id string) (Documento, bool)
}

// Numerador entrega folios seriados por empresa+sede+serie con incremento
// atómico (evita folios duplicados o saltados — §13.2).
type Numerador interface {
	// Siguiente entrega y consume el próximo folio (incrementa el contador).
	Siguiente(empresaID, sedeID, serie string) int
	// Actual devuelve el último folio ENTREGADO de la clave (0 si nunca se emitió).
	// A diferencia de Siguiente, NO incrementa: es una consulta pura.
	Actual(empresaID, sedeID, serie string) int
	// Fijar establece el último folio entregado de la clave, SOLO hacia adelante:
	// falla con ErrFolioRetrocede si `ultimo` es menor que el actual (los folios ya
	// usados no se reasignan). Sirve para continuar la numeración de un sistema
	// previo sin nunca reutilizar un folio.
	Fijar(empresaID, sedeID, serie string, ultimo int) error
	// Series devuelve el mapa "empresaID|sedeID|serie" → último folio entregado,
	// filtrado a la empresa indicada. Vacío si aún no se ha emitido nada.
	Series(empresaID string) map[string]int
}

// CuentaCobro es una cuenta de la empresa donde recibe pagos (destino del cobro).
type CuentaCobro struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Tipo      string `json:"tipo" bson:"tipo"` // pago_movil | zelle | banco | punto_venta
	Moneda    string `json:"moneda" bson:"moneda"`
	Titular   string `json:"titular" bson:"titular"`
	Datos     string `json:"datos" bson:"datos"` // nº de cuenta / teléfono / correo
}

// CuentaCobroRepo persiste cuentas de cobro, aislado por empresaID.
type CuentaCobroRepo interface {
	List(empresaID string) []CuentaCobro
	ByID(empresaID, id string) (CuentaCobro, bool)
	Create(c CuentaCobro) CuentaCobro
}
