package mesonero

import (
	"fmt"
	"strings"
	"time"
)

// HORARIOS del salón: a qué hora se espera que trabaje cada quien.
//
// QUÉ RESUELVE Y QUÉ NO: el horario NO es un candado. Quien autoriza cada turno
// sigue siendo el supervisor con su PIN. Si el horario bloqueara duro, la
// primera noche que alguien cubra a un compañero enfermo el sistema trancaría el
// servicio — y ahí el supervisor aprende a saltárselo, que es peor que no
// tenerlo. El horario sirve para tres cosas concretas:
//
//   1. calcular a qué hora DEBERÍA terminar el turno (FinPrevisto);
//   2. dejar marcado en la bitácora quien entró fuera de su horario;
//   3. disparar el CIERRE SUAVE al vencer, cuando la sede lo configuró así.
//
// La sede elige entre AVISAR o CERRAR SOLO (mesa.ConfigSalon.HorarioModo).

// Modos de vencimiento del horario, configurables por sede.
const (
	// HorarioAvisa: al pasar la hora no pasa nada automático; la pantalla lo
	// muestra y el supervisor decide. Es el valor por defecto: menos intrusivo.
	HorarioAvisa = "aviso"
	// HorarioCierraSolo: al pasar la hora el turno entra SOLO en cierre suave —
	// deja de tomar mesas nuevas y termina de atender las suyas. No lo apaga: eso
	// dejaría mesas sin quién las atienda a mitad de servicio.
	HorarioCierraSolo = "cierre_automatico"
)

// ModoHorarioValido acota el modo. Vacío ⇒ HorarioAvisa (así las sedes ya
// creadas no necesitan migración).
func ModoHorarioValido(m string) bool { return m == HorarioAvisa || m == HorarioCierraSolo }

// NormalizarModoHorario deja el modo en uno de los dos admitidos.
func NormalizarModoHorario(m string) string {
	m = strings.TrimSpace(strings.ToLower(m))
	if m == HorarioCierraSolo {
		return HorarioCierraSolo
	}
	return HorarioAvisa
}

// Motivos por los que un turno entró en cierre suave. Importa distinguirlos: un
// cierre que disparó el HORARIO se puede revertir aprobando tiempo extra; uno
// que ordenó una persona, no — o el tiempo extra estaría deshaciendo en silencio
// una decisión del supervisor.
const (
	CerrandoPorHorario    = "horario"
	CerrandoPorSupervisor = "supervisor"
)

// Franja es un tramo de trabajo semanal: los días y la hora de entrada y salida.
// Varias franjas por persona permiten lo real de un restaurante ("de martes a
// viernes en la noche, y sábado y domingo también al mediodía").
type Franja struct {
	// Dias son días de la semana, 0=domingo … 6=sábado (igual que time.Weekday).
	Dias []int `json:"dias" bson:"dias"`
	// Desde y Hasta son hora LOCAL de pared, "HH:MM". No llevan fecha: se
	// resuelven contra el día concreto al abrir el turno.
	Desde string `json:"desde" bson:"desde"`
	Hasta string `json:"hasta" bson:"hasta"`
}

// Horario es el patrón semanal de un mesonero. Uno por persona; es EDITABLE
// (no es un ledger): se reemplaza cuando cambia su turno de trabajo.
type Horario struct {
	EmpresaID  string `json:"empresaId" bson:"empresaid"`
	SedeID     string `json:"sedeId" bson:"sedeid"`
	MesoneroID string `json:"mesoneroId" bson:"mesoneroid"`
	// Franjas vacío = sin horario declarado. No impide trabajar: solo significa
	// que no hay una hora de fin que calcular ni que vencer.
	Franjas     []Franja `json:"franjas" bson:"franjas"`
	Actualizado string   `json:"actualizado" bson:"actualizado"` // RFC3339
}

// SinHorario indica que esta persona no tiene tramos declarados.
func (h Horario) SinHorario() bool { return len(h.Franjas) == 0 }

// HoraValida acepta "HH:MM" en 24 horas. Se valida acá y no en la interfaz
// porque un "25:00" guardado haría que el vencimiento nunca dispare.
func HoraValida(s string) bool {
	var hh, mm int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &hh, &mm); err != nil {
		return false
	}
	if len(strings.TrimSpace(s)) != 5 || strings.Index(s, ":") != 2 {
		return false
	}
	return hh >= 0 && hh <= 23 && mm >= 0 && mm <= 59
}

// Minutos convierte "HH:MM" a minutos desde medianoche. Devuelve -1 si no es una
// hora válida, para que quien la use no pueda confundirla con las 00:00.
func Minutos(hhmm string) int {
	if !HoraValida(hhmm) {
		return -1
	}
	var hh, mm int
	fmt.Sscanf(strings.TrimSpace(hhmm), "%d:%d", &hh, &mm)
	return hh*60 + mm
}

// CruzaMedianoche indica si el tramo termina al día SIGUIENTE. Es lo normal en
// un restaurante —se entra a las 18:00 y se cierra a la 1:00— y si no se
// contempla, el turno nacería vencido.
func (f Franja) CruzaMedianoche() bool {
	d, h := Minutos(f.Desde), Minutos(f.Hasta)
	return d >= 0 && h >= 0 && h <= d
}

// CubreDia indica si la franja incluye ese día de la semana.
func (f Franja) CubreDia(dia int) bool {
	for _, d := range f.Dias {
		if d == dia {
			return true
		}
	}
	return false
}

// HorarioRepository persiste los horarios. Uno por mesonero: Upsert reemplaza.
// Filtro por empresa obligatorio, como todo el resto.
type HorarioRepository interface {
	List(empresaID, sedeID string) []Horario
	ByMesonero(empresaID, mesoneroID string) (Horario, bool)
	Upsert(h Horario) Horario
	Delete(empresaID, mesoneroID string) bool
}

// FinDeTurno resuelve, para un instante de apertura dado (hora LOCAL), a qué
// hora local termina el turno según este horario, y si esa apertura cae DENTRO
// de algún tramo declarado.
//
// Devuelve (fin, dentro). `fin` es cero cuando no hay tramo aplicable: sin
// horario no hay nada que vencer, y el turno simplemente no tiene fin previsto.
//
// LOS DOS CASOS QUE IMPORTAN:
//
//   - Un tramo que CRUZA MEDIANOCHE (18:00→01:00) es lo normal en un
//     restaurante. Su fin cae al día siguiente; si se resolviera en el mismo día
//     el turno nacería vencido y el cierre automático lo cerraría al instante.
//   - Abrir ANTES de la hora de entrada (llegó temprano) igual toma el tramo de
//     hoy: el fin es el de ese tramo, no el de mañana.
func (h Horario) FinDeTurno(apertura time.Time) (time.Time, bool) {
	dia := int(apertura.Weekday())
	mediaNoche := time.Date(apertura.Year(), apertura.Month(), apertura.Day(), 0, 0, 0, 0, apertura.Location())

	var mejor time.Time
	dentro := false
	for _, f := range h.Franjas {
		d, hasta := Minutos(f.Desde), Minutos(f.Hasta)
		if d < 0 || hasta < 0 {
			continue
		}
		// El tramo de HOY (empieza hoy). Y también el de AYER, si cruzaba
		// medianoche y todavía sigue corriendo: quien entra a la 00:30 pertenece
		// al tramo que arrancó ayer a las 18:00, no al de esta noche.
		for _, cand := range []struct {
			diaTramo int
			offset   time.Duration
		}{{dia, 0}, {(dia + 6) % 7, -24 * time.Hour}} {
			if !f.CubreDia(cand.diaTramo) {
				continue
			}
			ini := mediaNoche.Add(cand.offset).Add(time.Duration(d) * time.Minute)
			fin := ini.Add(time.Duration(hasta-d) * time.Minute)
			if f.CruzaMedianoche() {
				fin = ini.Add(time.Duration(hasta+24*60-d) * time.Minute)
			}
			if !apertura.Before(ini) && apertura.Before(fin) {
				// Cae dentro del tramo: es el candidato correcto y manda.
				return fin, true
			}
			// Llegó temprano al tramo de hoy: se le asigna igual, pero queda
			// marcado como fuera de horario (todavía no era su hora).
			if cand.offset == 0 && apertura.Before(ini) && (mejor.IsZero() || fin.Before(mejor)) {
				mejor = fin
			}
		}
	}
	return mejor, dentro
}
