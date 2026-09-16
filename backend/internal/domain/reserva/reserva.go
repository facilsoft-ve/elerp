// Package reserva modela las RESERVACIONES del salón (módulo Restaurante): alguien
// aparta una mesa para un día, una hora y una cantidad de personas.
//
// Para qué sirve de verdad: que a las 8 de la noche, cuando llegue quien reservó,
// haya una mesa libre esperándolo. Por eso una reserva puede apuntar a una MESA
// concreta o solo a una ZONA ("terraza"): en un local chico el anfitrión sabe qué
// mesa dar, y en uno grande basta con reservar el área y decidir al llegar.
//
// En la puerta, quien recibe verifica al que llega por NOMBRE y CÉDULA: es el dato
// que el cliente dio al reservar y el único que hace comprobable que la reserva es
// suya. Por eso el documento se guarda normalizado (sin puntos ni guiones) — nadie
// lo teclea igual dos veces.
//
// Es un documento OPERATIVO editable, no un ledger: se cambia la hora, se agrega
// gente, se cancela. Lo que no se pierde es el rastro de quién hizo qué (auditoría).
package reserva

import "strings"

// Estados de una reserva.
const (
	// EstadoPendiente: registrada y esperando. Es la que "aparta" la mesa.
	EstadoPendiente = "pendiente"
	// EstadoSentada: el cliente llegó y se le abrió su cuenta.
	EstadoSentada = "sentada"
	// EstadoNoLlego: pasó la hora y nunca apareció. Se distingue de la cancelada
	// porque no es lo mismo para el local: el "no llegó" es una mesa que se dejó
	// vacía esperando, y saber cuántas hay dice si conviene sobrevender.
	EstadoNoLlego = "no_llego"
	// EstadoCancelada: avisó que no viene (o la canceló el local).
	EstadoCancelada = "cancelada"
)

// EstadoValido indica si el estado es uno de los admitidos.
func EstadoValido(e string) bool {
	switch e {
	case EstadoPendiente, EstadoSentada, EstadoNoLlego, EstadoCancelada:
		return true
	}
	return false
}

// Activa indica si la reserva todavía ocupa un lugar en el salón. Una cancelada o
// una que no llegó ya no reservan nada.
func (r Reserva) Activa() bool { return r.Estado == EstadoPendiente }

// Reserva es un apartado del salón.
type Reserva struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID    string `json:"sedeId" bson:"sedeid"`       // el salón es por sede
	// Fecha (YYYY-MM-DD) y Hora (HH:MM) LOCALES del local. Se guardan separadas y en
	// hora local a propósito: una reserva es "el jueves a las 8", no un instante UTC
	// que hay que convertir para leerlo, y el local no cambia de huso.
	Fecha string `json:"fecha" bson:"fecha"`
	Hora  string `json:"hora" bson:"hora"`
	// Personas es para cuántos comensales es la mesa.
	Personas int `json:"personas" bson:"personas"`
	// Quién reserva. Documento es la cédula/RIF con la que se verifica en la puerta.
	Nombre    string `json:"nombre" bson:"nombre"`
	Documento string `json:"documento" bson:"documento"`
	Telefono  string `json:"telefono" bson:"telefono"`
	// MesaID aparta una mesa CONCRETA; Zona aparta solo el área y deja la mesa para
	// decidirla al llegar. Pueden venir las dos (la mesa manda) o ninguna.
	MesaID     string `json:"mesaId" bson:"mesaid"`
	MesaNombre string `json:"mesaNombre" bson:"mesanombre"`
	Zona       string `json:"zona" bson:"zona"`
	Estado     string `json:"estado" bson:"estado"`
	Nota       string `json:"nota" bson:"nota"` // "cumpleaños", "silla para bebé"
	// CuentaID es la cuenta de mesa que se abrió al sentarla: enlaza la reserva con
	// lo que esa gente consumió.
	CuentaID string `json:"cuentaId" bson:"cuentaid"`
	// Trazabilidad mínima.
	CreadaPor   string `json:"creadaPor" bson:"creadapor"`
	Creada      string `json:"creada" bson:"creada"`           // RFC3339
	Actualizada string `json:"actualizada" bson:"actualizada"` // RFC3339
}

// NormalizarDocumento deja la cédula/RIF comparable: sin espacios, puntos ni guiones
// y en mayúsculas. "V-12.345.678" y "v12345678" son la misma persona, y en la puerta
// nadie escribe el documento dos veces igual.
func NormalizarDocumento(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Coincide indica si la reserva corresponde a lo que se teclea en la puerta: parte
// del nombre o el documento (normalizado). Es la búsqueda que hace quien recibe.
func (r Reserva) Coincide(termino string) bool {
	t := strings.TrimSpace(strings.ToLower(termino))
	if t == "" {
		return true
	}
	if strings.Contains(strings.ToLower(r.Nombre), t) {
		return true
	}
	doc := NormalizarDocumento(termino)
	return doc != "" && strings.Contains(r.Documento, doc)
}

// Repository persiste las reservas, aislado por empresaID. Operativo (editable).
type Repository interface {
	// DelDia devuelve las reservas de una sede en una fecha (YYYY-MM-DD),
	// ordenadas por hora.
	DelDia(empresaID, sedeID, fecha string) []Reserva
	// Desde devuelve las reservas de una sede a partir de una fecha (inclusive),
	// para la agenda de los próximos días.
	Desde(empresaID, sedeID, fecha string) []Reserva
	ByID(empresaID, id string) (Reserva, bool)
	Create(r Reserva) Reserva
	Update(r Reserva) (Reserva, bool)
}
