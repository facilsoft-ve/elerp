package mesa

import "testing"

// El corte en 4 es deliberado: la mesa estándar de hasta cuatro sigue ocupando
// una celda. Si creciera, todos los planos ya dibujados se desarmarían.
func TestDimension_LaMesaEstandarNoCrece(t *testing.T) {
	for _, cap := range []int{0, 1, 2, 3, 4} {
		if c, f := Dimension(cap); c != 1 || f != 1 {
			t.Errorf("capacidad %d debería ocupar 1×1, ocupa %d×%d", cap, c, f)
		}
	}
}

func TestDimension_LasGrandesCrecen(t *testing.T) {
	for _, cap := range []int{5, 6, 8} {
		if c, f := Dimension(cap); c != 2 || f != 1 {
			t.Errorf("capacidad %d debería ocupar 2×1, ocupa %d×%d", cap, c, f)
		}
	}
	for _, cap := range []int{9, 12, 20} {
		if c, f := Dimension(cap); c != 2 || f != 2 {
			t.Errorf("capacidad %d debería ocupar 2×2, ocupa %d×%d", cap, c, f)
		}
	}
}

// Ocupa tiene que cubrir TODA la superficie: si solo mirara la esquina, se
// podría colocar otra mesa sobre la mitad de un mesón y el mapa quedaría con dos
// mesas encimadas.
func TestOcupa_CubreTodaLaSuperficie(t *testing.T) {
	meson := Mesa{Columna: 2, Fila: 3, Capacidad: 10} // 2×2
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
	meson := Mesa{Columna: 2, Fila: 3, Capacidad: 10} // 2×2 → (2,3)…(3,4)
	// Una mesa chica en la esquina inferior derecha del mesón: se pisa.
	if !meson.SeSolapaCon(Mesa{Columna: 3, Fila: 4, Capacidad: 2}) {
		t.Error("una mesa dentro de la superficie del mesón se solapa")
	}
	// Pegada pero afuera: no se pisa.
	if meson.SeSolapaCon(Mesa{Columna: 4, Fila: 3, Capacidad: 2}) {
		t.Error("una mesa adyacente no se solapa")
	}
	// Dos mesas largas contiguas (2×1 cada una) caben lado a lado.
	a := Mesa{Columna: 0, Fila: 0, Capacidad: 6}
	b := Mesa{Columna: 2, Fila: 0, Capacidad: 6}
	if a.SeSolapaCon(b) {
		t.Error("dos mesas largas contiguas no se solapan")
	}
	// Pero una a una sola celda de distancia sí.
	if !a.SeSolapaCon(Mesa{Columna: 1, Fila: 0, Capacidad: 6}) {
		t.Error("una mesa larga a una celda de otra sí se solapa")
	}
}
