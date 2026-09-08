package fiscal

// Cierre Z: el reporte fiscal DIARIO por sede.
//
// Integridad (misma regla que el Documento): un Cierre Z es INMUTABLE y de
// solo-anexado. Consolida los documentos fiscales emitidos desde el último Z de
// la sede; su numeración Z es secuencial por sede (vía Numerador, serie "Z"). No
// se edita ni se borra: si hubo un error, se corrige con un Z POSTERIOR que
// vuelve a consolidar el rango pendiente. Todos sus montos son DERIVADOS del
// ledger de documentos, no se capturan a mano.

// TotalesZ es el consolidado de un rango de documentos, tal como lo exige un
// cierre fiscal: las ventas por su base, los impuestos separados, y las
// devoluciones/anulaciones (montos POSITIVOS, sumados aparte del neto).
type TotalesZ struct {
	// De las facturas.
	VentasGravadas float64 `json:"ventasGravadas" bson:"ventasgravadas"` // base imponible
	VentasExentas  float64 `json:"ventasExentas" bson:"ventasexentas"`   // base exenta
	IVADebito      float64 `json:"ivaDebito" bson:"ivadebito"`
	IGTF           float64 `json:"igtf" bson:"igtf"`
	TotalVentas    float64 `json:"totalVentas" bson:"totalventas"`
	// De las notas de crédito y anulaciones: sus montos en POSITIVO (los
	// documentos los guardan negativos), restados aparte del total neto.
	NotasCredito float64 `json:"notasCredito" bson:"notascredito"`
	Anulaciones  float64 `json:"anulaciones" bson:"anulaciones"`
	// TotalNeto = TotalVentas − NotasCredito − Anulaciones.
	TotalNeto float64 `json:"totalNeto" bson:"totalneto"`

	CantidadFacturas    int `json:"cantidadFacturas" bson:"cantidadfacturas"`
	CantidadNotas       int `json:"cantidadNotas" bson:"cantidadnotas"`
	CantidadAnulaciones int `json:"cantidadAnulaciones" bson:"cantidadanulaciones"`
}

// CierreZ es un reporte de cierre diario por sede. Inmutable, solo-anexado.
type CierreZ struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID    string `json:"sedeId" bson:"sedeid"`

	Numero         int    `json:"numero" bson:"numero"`                 // secuencia Z por sede (vía Numerador serie "Z")
	NumeroCompleto string `json:"numeroCompleto" bson:"numerocompleto"` // "Z-00000001"

	Fecha string `json:"fecha" bson:"fecha"` // emisión (ahora), UTC RFC3339

	// Rango CUBIERTO por el cierre: Fecha del primer y último documento incluidos.
	DesdeFecha string `json:"desdeFecha" bson:"desdefecha"`
	HastaFecha string `json:"hastaFecha" bson:"hastafecha"`
	// Folios extremos del rango: NumeroCompleto del primer y último documento.
	DocDesde string `json:"docDesde" bson:"docdesde"`
	DocHasta string `json:"docHasta" bson:"dochasta"`

	Totales TotalesZ `json:"totales" bson:"totales"`

	Actor string `json:"actor" bson:"actor"`
}

// CierreZRepo es el puerto de los cierres Z: solo-anexado (Append) + lectura.
// No expone Update ni Delete: un Z no se edita ni se borra —se corrige con otro Z.
type CierreZRepo interface {
	List(empresaID string) []CierreZ
	Append(CierreZ) CierreZ
}
