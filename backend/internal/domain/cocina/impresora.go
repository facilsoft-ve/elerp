// Package cocina agrupa la configuración del flujo de cocina del módulo Restaurante:
// las COMANDERAS, es decir las impresoras por donde salen las comandas.
//
// Un local tiene VARIAS: la de cocina, la de la barra y la de postres son puestos de
// preparación distintos y cada uno necesita su propio ticket con SUS renglones. Por eso
// cada comandera declara de qué RUBROS imprime, y una queda como predeterminada para lo
// que no encaje en ninguna — así un producto nuevo nunca se pierde en el camino.
//
// Cada comandera puede ser LOCAL (la del propio equipo / por el agente local, como las
// máquinas fiscales) o de RED (térmica con IP fija en la LAN, típicamente RAW/JetDirect
// en el puerto 9100). Es configuración POR SEDE. Editable, no ledger.
package cocina

import "strings"

// Modos de conexión de una comandera.
const (
	// ConexionLocal: conectada al equipo del cajero/estación (se imprime a través del
	// agente local, igual que la máquina fiscal). Es lo más común.
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

// Impresora es una COMANDERA: un puesto de impresión de comandas de una sede.
type Impresora struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	Nombre    string `json:"nombre" bson:"nombre"`     // "Cocina", "Barra", "Postres"
	Conexion  string `json:"conexion" bson:"conexion"` // local | red
	Host      string `json:"host" bson:"host"`         // IP/host (solo red)
	Puerto    int    `json:"puerto" bson:"puerto"`     // 9100 típico (solo red)
	AnchoMM   int    `json:"anchoMm" bson:"anchomm"`   // 58 | 80
	// Rubros son los rubros del catálogo cuyos productos salen por acá (p. ej.
	// "Bebidas" por la barra). Vacío ⇒ no atrae nada por rubro; solo recibe si es la
	// predeterminada.
	Rubros []string `json:"rubros" bson:"rubros"`
	// Predeterminada recibe lo que no encaja en ningún rubro configurado. Debe haber
	// exactamente una por sede: sin ella, un producto de un rubro nuevo no se imprimiría
	// en ninguna parte y el pedido se perdería.
	Predeterminada bool `json:"predeterminada" bson:"predeterminada"`
	// Activa: apagada, sus comandas no se mandan a imprimir (se pueden ver en pantalla).
	// Permite operar sin impresora mientras se configura.
	Activa      bool   `json:"activa" bson:"activa"`
	Actualizada string `json:"actualizada" bson:"actualizada"` // RFC3339
}

// ImprimeRubro indica si esta comandera atrae los productos de ese rubro.
func (i Impresora) ImprimeRubro(rubro string) bool {
	r := strings.TrimSpace(strings.ToLower(rubro))
	if r == "" {
		return false
	}
	for _, x := range i.Rubros {
		if strings.TrimSpace(strings.ToLower(x)) == r {
			return true
		}
	}
	return false
}

// Repository persiste las comanderas de una sede. Filtro por empresa obligatorio.
type Repository interface {
	List(empresaID, sedeID string) []Impresora
	ByID(empresaID, id string) (Impresora, bool)
	Create(i Impresora) Impresora
	Update(i Impresora) (Impresora, bool)
	Delete(empresaID, id string) bool
}
