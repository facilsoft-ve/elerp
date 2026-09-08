// Package mesa modela las MESAS de un restaurante y su disposición en el mapa
// del salón (módulo Restaurante).
//
// Una mesa es un puesto de atención con un nombre/número, una zona (salón,
// terraza, barra…), una capacidad de comensales y una FORMA. Además guarda su
// POSICIÓN y TAMAÑO en el mapa del salón (coordenadas del lienzo), para que el
// editor de arrastrar-y-soltar dibuje el plano y la caja/mesonero seleccione la
// mesa visualmente.
//
// Es CONFIGURACIÓN EDITABLE (como el maestro de unidades o el de formatos), no un
// ledger: una mesa se crea, mueve, edita y desactiva. El ESTADO operativo (libre/
// ocupada/…) lo gobierna la cuenta abierta de la mesa (fase siguiente); aquí es
// solo el valor por defecto para pintar el plano.
package mesa

import "strings"

// Estados operativos de una mesa (los pinta el plano del salón).
const (
	EstadoLibre     = "libre"
	EstadoOcupada   = "ocupada"
	EstadoPorCobrar = "por_cobrar"
	EstadoReservada = "reservada"
)

// Formas de mesa admitidas (para dibujarla en el mapa).
const (
	FormaRedonda     = "redonda"
	FormaCuadrada    = "cuadrada"
	FormaRectangular = "rectangular"
)

// FormaValida indica si la forma es una de las admitidas.
func FormaValida(f string) bool {
	switch f {
	case FormaRedonda, FormaCuadrada, FormaRectangular:
		return true
	}
	return false
}

// NormalizarForma deja la forma en minúsculas y sin espacios; vacío ⇒ cuadrada.
func NormalizarForma(f string) string {
	f = strings.ToLower(strings.TrimSpace(f))
	if f == "" {
		return FormaCuadrada
	}
	return f
}

// Mesa es una mesa del salón de una sede.
type Mesa struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID    string `json:"sedeId" bson:"sedeid"`       // el salón es por sede
	Nombre    string `json:"nombre" bson:"nombre"`       // "1", "Terraza 3", "Barra 2"
	Zona      string `json:"zona" bson:"zona"`           // salón/zona (opcional)
	Capacidad int    `json:"capacidad" bson:"capacidad"` // comensales
	Forma     string `json:"forma" bson:"forma"`         // redonda | cuadrada | rectangular
	// Posición en la GRILLA del salón (0-indexada): la mesa ocupa la celda
	// (Columna, Fila). El plano define cuántas filas y columnas hay (ver Plano).
	Columna int `json:"columna" bson:"columna"`
	Fila    int `json:"fila" bson:"fila"`
	// Posición/tamaño libres en px (modelo anterior, previo a la grilla). Se
	// conservan por retrocompatibilidad pero el editor de grilla usa Columna/Fila.
	X     float64 `json:"x" bson:"x"`
	Y     float64 `json:"y" bson:"y"`
	Ancho float64 `json:"ancho" bson:"ancho"`
	Alto  float64 `json:"alto" bson:"alto"`
	// Estado operativo por defecto (lo sobreescribe la cuenta abierta en la fase de
	// comandas). Vacío se lee como "libre".
	Estado string `json:"estado" bson:"estado"`
	Activa bool   `json:"activa" bson:"activa"`
	Creada string `json:"creada" bson:"creada"` // RFC3339
}

// Repository persiste mesas, aislado por empresaID. El salón es por sede, así que
// List acota por empresa + sede. Sin borrado lógico obligatorio: Delete es duro
// (una mesa es config, no un registro contable), pero la aplicación puede preferir
// desactivar (Activa=false) si la mesa tuvo cuentas.
type Repository interface {
	List(empresaID, sedeID string) []Mesa
	ByID(empresaID, id string) (Mesa, bool)
	Create(m Mesa) Mesa
	Update(m Mesa) (Mesa, bool)
	Delete(empresaID, id string) bool
}
