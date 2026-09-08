package mesa

// Plano es la configuración de la GRILLA del salón de una sede: cuántas filas y
// columnas tiene el mapa (para dar la forma del local — cuadrado, alargado…) y qué
// celdas están BLOQUEADAS (una pared, la cocina, una columna: ahí no puede ir una
// mesa). Es configuración por sede, editable, no ledger.
type Plano struct {
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	Filas     int    `json:"filas" bson:"filas"`
	Columnas  int    `json:"columnas" bson:"columnas"`
	// Bloqueadas son las celdas donde NO puede colocarse una mesa.
	Bloqueadas  []Celda `json:"bloqueadas" bson:"bloqueadas"`
	Actualizada string  `json:"actualizada" bson:"actualizada"` // RFC3339
}

// Celda es una posición de la grilla (0-indexada).
type Celda struct {
	Columna int `json:"columna" bson:"columna"`
	Fila    int `json:"fila" bson:"fila"`
}

// Límites de la grilla del salón.
const (
	FilasMin    = 1
	FilasMax    = 20
	ColumnasMin = 1
	ColumnasMax = 20
	FilasDefault    = 6
	ColumnasDefault = 8
)

// PlanoRepository persiste el plano (grilla) de una sede. Una sola por sede:
// Upsert reemplaza. Get devuelve false si aún no se configuró.
type PlanoRepository interface {
	Get(empresaID, sedeID string) (Plano, bool)
	Upsert(p Plano) Plano
}
