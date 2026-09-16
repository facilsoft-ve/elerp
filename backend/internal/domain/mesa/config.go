package mesa

// ConfigSalon es la configuración del módulo Restaurante en una sede.
type ConfigSalon struct {
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// AsignacionEstricta convierte la asignación de mesas en un CANDADO: un mesonero no
	// puede abrir ni agregar a una mesa que está asignada a otro. Apagada (el valor por
	// defecto) la asignación es solo organización: puede tomarla, se le advierte y queda
	// registrado en la bitácora.
	//
	// Está apagada por defecto a propósito: en un turno movido los mesoneros se cubren
	// entre sí, y un bloqueo duro hace que dejen el sistema y anoten en papel — peor que
	// un dato de organización imperfecto. Se enciende donde la operación lo justifique.
	AsignacionEstricta bool `json:"asignacionEstricta" bson:"asignacionestricta"`
	// HorarioModo decide qué pasa cuando se cumple la hora de salida de un
	// mesonero: «aviso» (por defecto, no pasa nada automático) o
	// «cierre_automatico» (el turno entra solo en cierre suave). Ver
	// mesonero.NormalizarModoHorario — vacío equivale a «aviso», así que las
	// sedes ya creadas no necesitan migración.
	HorarioModo string `json:"horarioModo" bson:"horariomodo"`
	Actualizada string `json:"actualizada" bson:"actualizada"` // RFC3339
}

// ConfigSalonRepository persiste la configuración del módulo por sede. Una sola por
// sede: Upsert reemplaza. Get devuelve false si nunca se guardó (valen los defaults).
type ConfigSalonRepository interface {
	Get(empresaID, sedeID string) (ConfigSalon, bool)
	Upsert(c ConfigSalon) ConfigSalon
}
