package mesa

import "strings"

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
	// HoraApertura y HoraCierre son el horario de atención del salón ("18:00",
	// "01:00"), en hora local. Sirven para AVISAR cuando se toma una reserva
	// fuera de servicio. Vacíos = sin declarar, y entonces no se avisa nada: un
	// local que no lo configuró no puede recibir advertencias todo el día.
	HoraApertura string `json:"horaApertura" bson:"horaapertura"`
	HoraCierre   string `json:"horaCierre" bson:"horacierre"`
	Actualizada  string `json:"actualizada" bson:"actualizada"` // RFC3339
}

// ConfigSalonRepository persiste la configuración del módulo por sede. Una sola por
// sede: Upsert reemplaza. Get devuelve false si nunca se guardó (valen los defaults).
type ConfigSalonRepository interface {
	Get(empresaID, sedeID string) (ConfigSalon, bool)
	Upsert(c ConfigSalon) ConfigSalon
}

// Horario de servicio del salón, para las reservas.
//
// Nota de la contadora (15:06): «trabajar validaciones en reservas: AFORO,
// SECTOR DEL RESTAURANTE, HORA». La hora hacía falta poder compararla contra
// algo, y ese algo es cuándo abre el local.
//
// Vacío = sin horario declarado, y entonces NO se avisa nada por la hora: un
// local que no configuró su horario no puede recibir advertencias todo el día.

// HorarioServicio indica si la sede declaró horario de atención.
func (c ConfigSalon) HorarioServicio() bool {
	return c.HoraApertura != "" && c.HoraCierre != ""
}

// DentroDelServicio indica si una hora "HH:MM" cae dentro del horario de
// atención. Contempla el cierre DESPUÉS de medianoche (18:00→01:00), que es lo
// normal en un restaurante: sin eso, toda reserva de la noche saldría marcada
// como fuera de horario.
func (c ConfigSalon) DentroDelServicio(hhmm string) bool {
	if !c.HorarioServicio() {
		return true // sin horario declarado no hay nada contra qué comparar
	}
	m, a, cierre := minutosDelDia(hhmm), minutosDelDia(c.HoraApertura), minutosDelDia(c.HoraCierre)
	if m < 0 || a < 0 || cierre < 0 {
		return true
	}
	if cierre > a {
		return m >= a && m <= cierre
	}
	// Cruza medianoche: vale desde la apertura hasta el fin del día, o desde el
	// inicio del día hasta el cierre.
	return m >= a || m <= cierre
}

// minutosDelDia convierte "HH:MM" a minutos desde medianoche; -1 si no es válida.
func minutosDelDia(hhmm string) int {
	s := strings.TrimSpace(hhmm)
	if len(s) != 5 || s[2] != ':' {
		return -1
	}
	hh := int(s[0]-'0')*10 + int(s[1]-'0')
	mm := int(s[3]-'0')*10 + int(s[4]-'0')
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return -1
	}
	return hh*60 + mm
}
