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
	// AnchoCeldas y AltoCeldas son el TAMAÑO de la mesa en cuadros del plano.
	// Quien dibuja el salón amplía la mesa y eso habilita más aforo — no al
	// revés: el plano es la realidad física del local y el aforo se acomoda a
	// ella, no la mesa al número que alguien tecleó.
	//
	// 0 significa «mesa anterior al redimensionado»: se le calcula un tamaño a
	// partir de su capacidad (ver DimensionSugerida) para que los planos ya
	// dibujados sigan viéndose bien sin migrar nada.
	AnchoCeldas int `json:"anchoCeldas,omitempty" bson:"anchoceldas,omitempty"`
	AltoCeldas  int `json:"altoCeldas,omitempty" bson:"altoceldas,omitempty"`
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

/* --- Tamaño de la mesa y aforo ---------------------------------------------
 *
 * Regla del constructor de planos: CADA CUADRO ADMITE HASTA 4 PERSONAS. Una
 * mesa de 2, 3 o 4 ocupa un cuadro; para sentar a más hay que AMPLIAR la mesa,
 * y recién entonces se puede elegir un aforo mayor.
 *
 * El sentido de la dependencia importa: manda el TAMAÑO y el aforo se acomoda.
 * El plano es la realidad física del local — si el aforo mandara sobre el
 * tamaño, teclear «20» en una mesa la haría crecer sola y pisar a las vecinas.
 */

// PersonasPorCelda es cuánta gente cabe en un cuadro del plano.
const PersonasPorCelda = 4

// DimensionSugerida es el tamaño MÍNIMO que hace falta para sentar a esa
// cantidad. Se usa en dos sitios: al dar de alta una mesa y para las mesas
// anteriores al redimensionado, que no tienen tamaño guardado.
//
// Crece primero a lo ancho y después en bloque, que es como se junta una mesa
// larga en un salón real.
func DimensionSugerida(capacidad int) (columnas, filas int) {
	// Cuadros que hacen falta, redondeando hacia arriba: 5 personas no caben en
	// un cuadro de 4, necesitan dos.
	celdas := (capacidad + PersonasPorCelda - 1) / PersonasPorCelda
	if celdas < 1 {
		celdas = 1
	}
	// Hasta tres cuadros se arma a lo LARGO, que es como se junta una mesa en un
	// salón real (tres mesas en fila, no un bloque).
	if celdas <= 3 {
		return celdas, 1
	}
	// De ahí en adelante, el bloque más compacto que alcance. Tiene que
	// ALCANZAR siempre: si sugiriera de menos, el alta fallaría contra su propio
	// valor por defecto (lo destapó una prueba con aforo 17).
	columnas = 1
	for columnas*columnas < celdas {
		columnas++
	}
	filas = (celdas + columnas - 1) / columnas
	return columnas, filas
}

// Dimension es el tamaño EFECTIVO de la mesa en cuadros. Sin tamaño guardado
// (mesas anteriores) se deriva de la capacidad, así los planos ya dibujados no
// necesitan migración.
func (m Mesa) Dimension() (columnas, filas int) {
	if m.AnchoCeldas > 0 && m.AltoCeldas > 0 {
		return m.AnchoCeldas, m.AltoCeldas
	}
	return DimensionSugerida(m.Capacidad)
}

// CapacidadMaxima es cuánta gente admite la mesa por su tamaño: 4 por cuadro.
// Es el tope que la interfaz ofrece y que el servidor hace cumplir.
func (m Mesa) CapacidadMaxima() int {
	c, f := m.Dimension()
	return c * f * PersonasPorCelda
}

// Ocupa indica si la mesa cubre la celda (c, r) contando toda su superficie, no
// solo su esquina. Es lo que impide poner otra mesa «encima» de la mitad de un
// mesón, que a simple vista parecería una celda libre.
func (m Mesa) Ocupa(c, r int) bool {
	anchoC, altoF := m.Dimension()
	return c >= m.Columna && c < m.Columna+anchoC && r >= m.Fila && r < m.Fila+altoF
}

// SeSolapaCon indica si dos mesas comparten alguna celda.
func (m Mesa) SeSolapaCon(o Mesa) bool {
	anchoM, altoM := m.Dimension()
	for c := m.Columna; c < m.Columna+anchoM; c++ {
		for r := m.Fila; r < m.Fila+altoM; r++ {
			if o.Ocupa(c, r) {
				return true
			}
		}
	}
	return false
}
