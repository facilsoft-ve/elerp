// Package cuenta modela la CUENTA de una mesa en el módulo Restaurante: el pedido
// que crece mientras la mesa está ocupada, hasta que se cierra (y, en la fase
// fiscal, se convierte en factura).
//
// Es un documento OPERATIVO editable (como las ventas en espera del POS), no un
// ledger: se agregan y quitan renglones, y cada envío a cocina agrupa los
// pendientes en una RONDA (la comanda que se imprime). Cada renglón lleva su
// estado de cocina (pendiente → en_cocina → listo → servido, o cancelado).
package cuenta

import "strings"

// Estados de la cuenta.
const (
	EstadoAbierta = "abierta"
	EstadoCerrada = "cerrada"
	EstadoAnulada = "anulada"
)

// Estados de cocina de un renglón.
const (
	ItemPendiente = "pendiente" // agregado, aún no enviado a cocina
	ItemEnCocina  = "en_cocina" // enviado (en una comanda/ronda)
	ItemListo     = "listo"     // cocina lo marcó listo
	ItemServido   = "servido"   // el mesonero lo llevó a la mesa
	ItemCancelado = "cancelado" // anulado (no cuenta para el total)
)

// EstadoItemValido indica si el estado de cocina es uno de los admitidos.
func EstadoItemValido(e string) bool {
	switch e {
	case ItemPendiente, ItemEnCocina, ItemListo, ItemServido, ItemCancelado:
		return true
	}
	return false
}

// Item es un renglón de la cuenta.
type Item struct {
	ID             string  `json:"id" bson:"id"`
	SKU            string  `json:"sku" bson:"sku"`
	Nombre         string  `json:"nombre" bson:"nombre"`
	Cantidad       float64 `json:"cantidad" bson:"cantidad"`
	PrecioUnitario float64 `json:"precioUnitario" bson:"preciounitario"` // en Bs (autoritativo, como el POS)
	Exento         bool    `json:"exento" bson:"exento"`
	Nota           string  `json:"nota" bson:"nota"` // "sin cebolla", "término medio"…
	Estado         string  `json:"estado" bson:"estado"`
	// Ronda es el número de envío a cocina que agrupó este renglón (0 = aún no
	// enviado). Cada envío incrementa la ronda de la cuenta.
	Ronda    int    `json:"ronda" bson:"ronda"`
	Agregada string `json:"agregada" bson:"agregada"` // RFC3339
	// EnviadoEn es el instante en que el renglón se mandó a cocina (RFC3339); vacío
	// mientras está pendiente. La pantalla de cocina lo usa para «hace X min».
	EnviadoEn string `json:"enviadoEn,omitempty" bson:"enviadoen,omitempty"`
	// PrefacturaID es la SOLICITUD DE FACTURACIÓN que ya se llevó este renglón
	// (id de la cotización confirmada). Vacío = todavía está sin pedir, así que
	// entra en la próxima solicitud. Es lo que permite que una misma mesa se
	// facture por partes: dos comensales que pagan lo suyo son dos solicitudes
	// sobre renglones distintos de la MISMA cuenta.
	PrefacturaID string `json:"prefacturaId,omitempty" bson:"prefacturaid,omitempty"`
}

// Importe del renglón (Bs). Un renglón cancelado no aporta.
func (i Item) Importe() float64 {
	if i.Estado == ItemCancelado {
		return 0
	}
	return i.PrecioUnitario * i.Cantidad
}

// Cuenta es el pedido abierto de una mesa.
type Cuenta struct {
	ID         string `json:"id" bson:"id"`
	EmpresaID  string `json:"empresaId" bson:"empresaid"`
	SedeID     string `json:"sedeId" bson:"sedeid"`
	MesaID     string `json:"mesaId" bson:"mesaid"`
	MesaNombre string `json:"mesaNombre" bson:"mesanombre"`
	Estado     string `json:"estado" bson:"estado"`
	// Quién la abrió/atiende (mesonero o caja).
	MesoneroID     string `json:"mesoneroId" bson:"mesoneroid"`
	MesoneroNombre string `json:"mesoneroNombre" bson:"mesoneronombre"`
	Comensales     int    `json:"comensales" bson:"comensales"`
	Items          []Item `json:"items" bson:"items"`
	UltimaRonda    int    `json:"ultimaRonda" bson:"ultimaronda"`
	Abierta        string `json:"abierta" bson:"abierta"` // RFC3339
	Cerrada        string `json:"cerrada" bson:"cerrada"`
	// Prefacturas son las cotizaciones CONFIRMADAS (prefacturas) generadas al pedir la
	// cuenta. Una sola cuando se paga junto; varias cuando se dividió por productos
	// (cada comensal con su propia prefactura y, después, su propia factura). La
	// división en partes IGUALES no genera varias: eso es un reparto del COBRO, no del
	// documento, y se resuelve con varios pagos sobre una única factura.
	Prefacturas []string `json:"prefacturas" bson:"prefacturas"`
	// PrefacturadaEn es cuándo se pidió la cuenta (RFC3339). Vacío mientras no se pidió.
	PrefacturadaEn string `json:"prefacturadaEn" bson:"prefacturadaen"`
	// DocumentoID es la factura emitida al cerrar (fase fiscal); vacío mientras
	// tanto.
	DocumentoID string `json:"documentoId" bson:"documentoid"`
	// TurnoID es el turno del mesonero que abrió la cuenta (mesonero.Turno).
	// Se estampa al abrir y no cambia — igual que la sesión de caja se estampa en
	// el documento fiscal. Es lo que permite PLEGAR el resumen del turno al
	// cerrarlo (mesas, personas, órdenes, ticket) en vez de llevar contadores.
	// Vacío cuando la cuenta la abrió alguien sin turno (la dueña, el cajero).
	TurnoID string `json:"turnoId,omitempty" bson:"turnoid,omitempty"`
}

// Total de la cuenta en Bs (suma de renglones no cancelados). Es una PROYECCIÓN,
// nunca un contador editable.
func (c Cuenta) Total() float64 {
	var t float64
	for _, it := range c.Items {
		t += it.Importe()
	}
	return t
}

// TienePendientes indica si hay renglones sin enviar a cocina.
func (c Cuenta) TienePendientes() bool {
	for _, it := range c.Items {
		if it.Estado == ItemPendiente {
			return true
		}
	}
	return false
}

// ItemsSinFacturar devuelve los renglones vivos que todavía no entraron en ninguna
// solicitud de facturación: son los que se pueden pedir en la próxima.
func (c Cuenta) ItemsSinFacturar() []Item {
	out := make([]Item, 0, len(c.Items))
	for _, it := range c.Items {
		if it.Estado != ItemCancelado && it.PrefacturaID == "" {
			out = append(out, it)
		}
	}
	return out
}

// TieneSinFacturar indica si queda consumo sin pedir. Mientras sea cierto la mesa NO
// se cierra aunque ya se hayan cobrado todas las solicitudes emitidas: alguien sigue
// comiendo en esa mesa.
func (c Cuenta) TieneSinFacturar() bool { return len(c.ItemsSinFacturar()) > 0 }

// NormalizarNota recorta la nota de un renglón.
func NormalizarNota(s string) string { return strings.TrimSpace(s) }

// Repository persiste cuentas, aislado por empresaID. Operativo (editable), no
// ledger.
type Repository interface {
	// Abiertas devuelve las cuentas ABIERTAS de una sede (para el tablero de mesas).
	Abiertas(empresaID, sedeID string) []Cuenta
	ByID(empresaID, id string) (Cuenta, bool)
	// AbiertaDeMesa devuelve la cuenta abierta de una mesa, si existe.
	AbiertaDeMesa(empresaID, mesaID string) (Cuenta, bool)
	// DeTurno devuelve TODAS las cuentas de un turno (abiertas y cerradas). Es la
	// fuente del resumen que se congela al cerrar el turno, y la que responde
	// «¿le queda alguna mesa por cerrar?» durante el cierre suave.
	DeTurno(empresaID, turnoID string) []Cuenta
	Create(c Cuenta) Cuenta
	Update(c Cuenta) (Cuenta, bool)
}
