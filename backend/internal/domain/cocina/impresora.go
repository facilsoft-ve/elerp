// Package cocina agrupa la configuración del flujo de cocina del módulo
// Restaurante. Por ahora: la IMPRESORA donde se imprimen las comandas.
//
// La impresora puede ser LOCAL (la del propio equipo / a través del agente local,
// como las máquinas fiscales) o de RED (una impresora térmica con IP fija en la
// LAN del local, típicamente RAW/JetDirect en el puerto 9100). Es configuración
// POR SEDE (cada local tiene su cocina). Editable, no ledger.
package cocina

import "strings"

// Modos de conexión de la impresora de comandas.
const (
	// ConexionLocal: impresora conectada al equipo del cajero/estación (se imprime
	// a través del agente local, igual que la máquina fiscal). Es lo más común.
	ConexionLocal = "local"
	// ConexionRed: impresora térmica con IP en la red del local (RAW 9100).
	ConexionRed = "red"
)

// ConexionValida indica si el modo de conexión es uno de los admitidos.
func ConexionValida(c string) bool { return c == ConexionLocal || c == ConexionRed }

// NormalizarConexion deja el modo en minúsculas; vacío ⇒ local.
func NormalizarConexion(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	if c == "" {
		return ConexionLocal
	}
	return c
}

// Impresora es la configuración de la impresora de comandas de una sede.
type Impresora struct {
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	Nombre    string `json:"nombre" bson:"nombre"`     // "Cocina", "Barra", …
	Conexion  string `json:"conexion" bson:"conexion"` // local | red
	Host      string `json:"host" bson:"host"`         // IP/host (solo red)
	Puerto    int    `json:"puerto" bson:"puerto"`     // 9100 típico (solo red)
	AnchoMM   int    `json:"anchoMm" bson:"anchomm"`   // 58 | 80
	// Activa: si está apagada, las comandas no se envían a imprimir (se pueden ver
	// en pantalla). Permite operar sin impresora mientras se configura.
	Activa      bool   `json:"activa" bson:"activa"`
	Actualizada string `json:"actualizada" bson:"actualizada"` // RFC3339
}

// Repository persiste la impresora de comandas por (empresa, sede). Una sola por
// sede: Upsert reemplaza. Get devuelve false si aún no se configuró.
type Repository interface {
	Get(empresaID, sedeID string) (Impresora, bool)
	Upsert(i Impresora) Impresora
}
