package mesonero

import (
	"testing"
	"time"
)

// Los horarios de un restaurante cruzan medianoche casi siempre. Si eso se
// resuelve mal, el turno nace vencido y el cierre automático lo manda a cerrar
// en el mismo instante en que se abrió. Estas pruebas son esa reja.

// vet arma un instante local. El día 2026-09-16 es MIÉRCOLES (3).
func vet(dia, hora, min int) time.Time {
	return time.Date(2026, 9, dia, hora, min, 0, 0, time.UTC)
}

const (
	mar = 2 // 2026-09-15
	mie = 3 // 2026-09-16
	jue = 4 // 2026-09-17
)

func nocturno() Horario {
	// Miércoles y jueves, de 18:00 a 01:00 del día siguiente.
	return Horario{Franjas: []Franja{{Dias: []int{mie, jue}, Desde: "18:00", Hasta: "01:00"}}}
}

func TestFinDeTurno_TramoQueCruzaMedianoche(t *testing.T) {
	fin, dentro := nocturno().FinDeTurno(vet(16, 19, 0)) // miércoles 19:00
	if !dentro {
		t.Fatal("las 19:00 del miércoles están dentro del tramo 18:00–01:00")
	}
	quiero := vet(17, 1, 0) // jueves 01:00
	if !fin.Equal(quiero) {
		t.Errorf("fin = %v, se esperaba %v (al día SIGUIENTE)", fin, quiero)
	}
}

// Quien entra a las 00:30 pertenece al tramo que arrancó AYER a las 18:00, no al
// de esta noche. Si se le asigna el de hoy, su turno termina 24 horas tarde.
func TestFinDeTurno_MadrugadaPerteneceAlTramoDeAyer(t *testing.T) {
	fin, dentro := nocturno().FinDeTurno(vet(17, 0, 30)) // jueves 00:30
	if !dentro {
		t.Fatal("las 00:30 del jueves siguen dentro del tramo que abrió el miércoles")
	}
	quiero := vet(17, 1, 0) // jueves 01:00 — el cierre de la noche del miércoles
	if !fin.Equal(quiero) {
		t.Errorf("fin = %v, se esperaba %v (cierre del tramo de ayer)", fin, quiero)
	}
}

// Llegar temprano no es estar dentro del horario, pero sí toma el tramo de hoy:
// si no, el turno quedaría sin fin previsto y nunca vencería.
func TestFinDeTurno_LlegaTempranoTomaElTramoDeHoyPeroQuedaFuera(t *testing.T) {
	fin, dentro := nocturno().FinDeTurno(vet(16, 17, 0)) // miércoles 17:00
	if dentro {
		t.Error("las 17:00 son antes de su entrada: no está dentro del horario")
	}
	quiero := vet(17, 1, 0)
	if !fin.Equal(quiero) {
		t.Errorf("fin = %v, se esperaba %v (el tramo de esta noche)", fin, quiero)
	}
}

// Un día que no trabaja no tiene fin que calcular. Sin esto, el cierre
// automático se dispararía sobre alguien que vino a cubrir un día libre.
func TestFinDeTurno_DiaQueNoTrabajaNoTieneFin(t *testing.T) {
	fin, dentro := nocturno().FinDeTurno(vet(15, 19, 0)) // martes
	if dentro {
		t.Error("el martes no está en su horario")
	}
	if !fin.IsZero() {
		t.Errorf("sin tramo aplicable el fin debe quedar en cero, dio %v", fin)
	}
}

func TestFinDeTurno_SinHorarioNoHayFin(t *testing.T) {
	fin, dentro := Horario{}.FinDeTurno(vet(16, 19, 0))
	if dentro || !fin.IsZero() {
		t.Errorf("sin franjas no hay fin ni «dentro»: dio %v / %v", fin, dentro)
	}
}

// Un tramo diurno normal (no cruza medianoche) tiene que seguir funcionando.
func TestFinDeTurno_TramoDiurno(t *testing.T) {
	h := Horario{Franjas: []Franja{{Dias: []int{mie}, Desde: "11:00", Hasta: "16:00"}}}
	fin, dentro := h.FinDeTurno(vet(16, 12, 0))
	if !dentro {
		t.Fatal("las 12:00 están dentro de 11:00–16:00")
	}
	if !fin.Equal(vet(16, 16, 0)) {
		t.Errorf("fin = %v, se esperaba el mismo día a las 16:00", fin)
	}
}

// Con dos tramos el mismo día (almuerzo y cena), cada apertura toma el suyo.
func TestFinDeTurno_DosTramosElMismoDia(t *testing.T) {
	h := Horario{Franjas: []Franja{
		{Dias: []int{mie}, Desde: "11:00", Hasta: "15:00"},
		{Dias: []int{mie}, Desde: "18:00", Hasta: "23:00"},
	}}
	if fin, dentro := h.FinDeTurno(vet(16, 12, 0)); !dentro || !fin.Equal(vet(16, 15, 0)) {
		t.Errorf("a las 12:00 le toca cerrar 15:00, dio %v (dentro=%v)", fin, dentro)
	}
	if fin, dentro := h.FinDeTurno(vet(16, 20, 0)); !dentro || !fin.Equal(vet(16, 23, 0)) {
		t.Errorf("a las 20:00 le toca cerrar 23:00, dio %v (dentro=%v)", fin, dentro)
	}
	// En el hueco entre tramos (16:00) no está dentro, pero el siguiente tramo de
	// hoy es el que le corresponde.
	if fin, dentro := h.FinDeTurno(vet(16, 16, 30)); dentro || !fin.Equal(vet(16, 23, 0)) {
		t.Errorf("en el hueco debe quedar fuera y tomar el tramo siguiente: %v (dentro=%v)", fin, dentro)
	}
}

func TestHoraValida(t *testing.T) {
	for _, s := range []string{"00:00", "18:30", "23:59"} {
		if !HoraValida(s) {
			t.Errorf("%q debería ser válida", s)
		}
	}
	// Un "25:00" guardado haría que el vencimiento nunca dispare.
	for _, s := range []string{"", "24:00", "18:60", "8:30", "18h30", "1830", "-1:00"} {
		if HoraValida(s) {
			t.Errorf("%q NO debería ser válida", s)
		}
	}
}

func TestMinutos(t *testing.T) {
	if got := Minutos("18:30"); got != 1110 {
		t.Errorf("18:30 = %d minutos, se esperaban 1110", got)
	}
	// -1 y no 0: una hora inválida no puede confundirse con medianoche.
	if got := Minutos("nope"); got != -1 {
		t.Errorf("una hora inválida debe dar -1, dio %d", got)
	}
}

func TestCruzaMedianoche(t *testing.T) {
	if !(Franja{Desde: "18:00", Hasta: "01:00"}).CruzaMedianoche() {
		t.Error("18:00→01:00 cruza medianoche")
	}
	if (Franja{Desde: "11:00", Hasta: "16:00"}).CruzaMedianoche() {
		t.Error("11:00→16:00 no cruza medianoche")
	}
}

func TestNormalizarModoHorario(t *testing.T) {
	if NormalizarModoHorario("") != HorarioAvisa {
		t.Error("vacío debe caer en «aviso»: las sedes ya creadas no migran")
	}
	if NormalizarModoHorario("CIERRE_AUTOMATICO") != HorarioCierraSolo {
		t.Error("el modo debe reconocerse sin distinguir mayúsculas")
	}
	if NormalizarModoHorario("cualquier cosa") != HorarioAvisa {
		t.Error("un modo desconocido cae en el menos intrusivo")
	}
}
