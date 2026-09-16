package mesa

import "testing"

/* La regla del constructor de planos: CADA CUADRO ADMITE 4 PERSONAS. Manda el
 * TAMAÑO y el aforo se acomoda — no al revés. Si el aforo mandara, teclear «20»
 * en una mesa la haría crecer sola y pisar a las vecinas. */

func TestDimensionSugerida_UnCuadroHastaCuatro(t *testing.T) {
	for _, cap := range []int{0, 1, 2, 3, 4} {
		if c, f := DimensionSugerida(cap); c != 1 || f != 1 {
			t.Errorf("capacidad %d debería caber en 1×1, sugiere %d×%d", cap, c, f)
		}
	}
}

// El tamaño sugerido tiene que ALCANZAR para la gente pedida: si sugiriera de
// menos, el alta fallaría contra su propio valor por defecto.
func TestDimensionSugerida_SiempreAlcanza(t *testing.T) {
	for cap := 0; cap <= 24; cap++ {
		c, f := DimensionSugerida(cap)
		if c*f*PersonasPorCelda < cap {
			t.Errorf("capacidad %d: sugiere %d×%d, que solo admite %d", cap, c, f, c*f*PersonasPorCelda)
		}
	}
}

// El tamaño GUARDADO manda sobre el sugerido: una mesa de 4 que alguien amplió a
// dos cuadros se queda de dos cuadros.
func TestDimension_ElTamanoGuardadoManda(t *testing.T) {
	m := Mesa{Capacidad: 4, AnchoCeldas: 3, AltoCeldas: 1}
	if c, f := m.Dimension(); c != 3 || f != 1 {
		t.Errorf("debería respetar el tamaño guardado 3×1, dio %d×%d", c, f)
	}
	// Y ese tamaño habilita más aforo, que es el punto de ampliar.
	if m.CapacidadMaxima() != 12 {
		t.Errorf("3 cuadros admiten 12 personas, dice %d", m.CapacidadMaxima())
	}
}

// Sin tamaño guardado (mesas anteriores al redimensionado) se deriva de la
// capacidad: los planos ya dibujados no necesitan migración.
func TestDimension_SinTamanoGuardadoSeDeriva(t *testing.T) {
	if c, f := (Mesa{Capacidad: 8}).Dimension(); c != 2 || f != 1 {
		t.Errorf("una mesa vieja de 8 debería derivar 2×1, dio %d×%d", c, f)
	}
	if c, f := (Mesa{Capacidad: 2}).Dimension(); c != 1 || f != 1 {
		t.Errorf("una mesa vieja de 2 debería derivar 1×1, dio %d×%d", c, f)
	}
}

func TestCapacidadMaxima_CuatroPorCuadro(t *testing.T) {
	casos := []struct{ ancho, alto, max int }{
		{1, 1, 4}, {2, 1, 8}, {3, 1, 12}, {2, 2, 16},
	}
	for _, c := range casos {
		m := Mesa{AnchoCeldas: c.ancho, AltoCeldas: c.alto}
		if got := m.CapacidadMaxima(); got != c.max {
			t.Errorf("%d×%d debería admitir %d, admite %d", c.ancho, c.alto, c.max, got)
		}
	}
}

// Ocupa tiene que cubrir TODA la superficie: si solo mirara la esquina, se
// podría colocar otra mesa sobre la mitad de un mesón y quedarían encimadas.
func TestOcupa_CubreTodaLaSuperficie(t *testing.T) {
	meson := Mesa{Columna: 2, Fila: 3, AnchoCeldas: 2, AltoCeldas: 2}
	for _, celda := range [][2]int{{2, 3}, {3, 3}, {2, 4}, {3, 4}} {
		if !meson.Ocupa(celda[0], celda[1]) {
			t.Errorf("el mesón debería ocupar (%d,%d)", celda[0], celda[1])
		}
	}
	for _, celda := range [][2]int{{1, 3}, {4, 3}, {2, 2}, {2, 5}} {
		if meson.Ocupa(celda[0], celda[1]) {
			t.Errorf("el mesón NO debería ocupar (%d,%d)", celda[0], celda[1])
		}
	}
}

func TestSeSolapaCon(t *testing.T) {
	meson := Mesa{Columna: 2, Fila: 3, AnchoCeldas: 2, AltoCeldas: 2} // (2,3)…(3,4)
	if !meson.SeSolapaCon(Mesa{Columna: 3, Fila: 4, AnchoCeldas: 1, AltoCeldas: 1}) {
		t.Error("una mesa dentro de la superficie del mesón se solapa")
	}
	if meson.SeSolapaCon(Mesa{Columna: 4, Fila: 3, AnchoCeldas: 1, AltoCeldas: 1}) {
		t.Error("una mesa adyacente no se solapa")
	}
	a := Mesa{Columna: 0, Fila: 0, AnchoCeldas: 2, AltoCeldas: 1}
	if a.SeSolapaCon(Mesa{Columna: 2, Fila: 0, AnchoCeldas: 2, AltoCeldas: 1}) {
		t.Error("dos mesas largas contiguas no se solapan")
	}
	if !a.SeSolapaCon(Mesa{Columna: 1, Fila: 0, AnchoCeldas: 2, AltoCeldas: 1}) {
		t.Error("una mesa larga a una celda de otra sí se solapa")
	}
}
