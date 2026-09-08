package contabilidad

// Periodo es el cierre de un mes contable. Marca (año, mes) como cerrado
// DEFINITIVAMENTE: un período cerrado no se reabre (§7.3). Es de solo-anexado,
// igual que el libro diario: no se edita ni se borra, y no existe la operación
// inversa de "reabrir".
//
// FechaCierre guarda el último instante del mes cerrado en RFC3339 UTC (p. ej.
// "2026-07-31T23:59:59Z"). Se usa como barrera por comparación de strings: un
// asiento cuya Fecha sea <= a algún FechaCierre cae dentro (o antes) de un mes
// cerrado y no puede anexarse. Como los asientos derivados de hoy siempre llevan
// Fecha=ahora() y solo se cierran meses estrictamente anteriores al actual, esta
// barrera nunca bloquea la operación corriente.
type Periodo struct {
	ID          string `json:"id" bson:"id"`
	EmpresaID   string `json:"empresaId" bson:"empresaid"` // tenant
	Anio        int    `json:"anio" bson:"anio"`
	Mes         int    `json:"mes" bson:"mes"` // 1..12
	FechaCierre string `json:"fechaCierre" bson:"fechacierre"`
	CerradoPor  string `json:"cerradoPor" bson:"cerradopor"`
	CerradoEl   string `json:"cerradoEl" bson:"cerradoel"`
}

// PeriodoRepo es el puerto de los cierres de período. SOLO-ANEXADO: no expone
// Update ni Delete a propósito — un cierre no se reabre.
type PeriodoRepo interface {
	List(empresaID string) []Periodo
	Append(p Periodo) Periodo
}
