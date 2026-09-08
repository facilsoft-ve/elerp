package fiscal

// Tipos de dispositivo fiscal soportados en la configuración.
const (
	DispositivoImpresoraFiscal = "impresora_fiscal"
	// DispositivoBalanza es una balanza electrónica de mostrador (charcutería,
	// granel): pesa el producto y devuelve los kilogramos que el POS multiplica
	// por el precio/kg. Como la impresora fiscal, la conexión real con el
	// hardware la hace el agente fiscal local; aquí solo se administra su ficha.
	DispositivoBalanza = "balanza"
	DispositivoOtro    = "otro"
)

// DispositivoFiscal es un dispositivo (típicamente una impresora fiscal
// homologada) registrado por la empresa. La conexión real con el hardware la
// hace el agente fiscal local (un puente aparte, fuera del backend): aquí solo
// se administra el inventario de dispositivos y sus datos de identificación.
type DispositivoFiscal struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// SedeID opcional: "" significa que el dispositivo aplica a toda la empresa.
	SedeID string `json:"sedeId" bson:"sedeid"`
	Nombre string `json:"nombre" bson:"nombre"` // "Impresora caja 1", "The Factory HKA"...
	// Tipo: impresora_fiscal | otro.
	Tipo   string `json:"tipo" bson:"tipo"`
	Marca  string `json:"marca" bson:"marca"`
	Modelo string `json:"modelo" bson:"modelo"`
	// Serie es el serial fiscal declarado; único por empresa cuando viene.
	Serie string `json:"serie" bson:"serie"`
	// Puerto y Protocolo son opcionales y los usa la balanza para que el agente
	// fiscal local sepa dónde y cómo hablarle (p. ej. Puerto "COM3"/"/dev/ttyUSB0"
	// y Protocolo "prt1"/"dialog06"). Son inocuos para las impresoras fiscales,
	// que ya se identifican con marca/modelo/serie.
	Puerto    string `json:"puerto" bson:"puerto"`
	Protocolo string `json:"protocolo" bson:"protocolo"`
	Activo    bool   `json:"activo" bson:"activo"`
	Creado    string `json:"creado" bson:"creado"` // UTC RFC3339
}

// TipoDispositivoValido valida el tipo del dispositivo.
func TipoDispositivoValido(t string) bool {
	return t == DispositivoImpresoraFiscal || t == DispositivoBalanza || t == DispositivoOtro
}

// DispositivoFiscalRepo persiste dispositivos fiscales, aislado por empresaID.
// Sin borrado duro: se desactiva con Activo=false (reversible).
type DispositivoFiscalRepo interface {
	List(empresaID string) []DispositivoFiscal
	ByID(empresaID, id string) (DispositivoFiscal, bool)
	Create(d DispositivoFiscal) DispositivoFiscal
	Update(d DispositivoFiscal) (DispositivoFiscal, bool)
}
