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
	// DocumentoID es la factura emitida al cerrar (fase fiscal); vacío mientras
	// tanto.
	DocumentoID string `json:"documentoId" bson:"documentoid"`
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
	Create(c Cuenta) Cuenta
	Update(c Cuenta) (Cuenta, bool)
}
