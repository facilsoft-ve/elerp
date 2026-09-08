package cupon

import "testing"

// El CÁLCULO del descuento (porcentaje vs monto fijo, tope al subtotal, monto
// mínimo, vigencia por fecha y control de usos) se aplica en
// internal/application/cupon.go (ValidarCupon) y ya está cubierto por sus tests.
// Aquí solo se prueba la lógica PURA del dominio: el validador de tipo y la
// normalización del código.

func TestTipoValido(t *testing.T) {
	casos := []struct {
		in   string
		want bool
	}{
		{TipoPorcentaje, true},
		{TipoMonto, true},
		{"", false},
		{"fijo", false},
		{"Porcentaje", false}, // comparación exacta, sin normalizar
	}
	for _, c := range casos {
		if got := TipoValido(c.in); got != c.want {
			t.Errorf("TipoValido(%q) = %v; quería %v", c.in, got, c.want)
		}
	}
}

func TestNormalizarCodigo(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   string
	}{
		{"minúsculas a mayúsculas", "verano10", "VERANO10"},
		{"recorta espacios a los lados", "  black  ", "BLACK"},
		{"mixto con espacios", " Verano10 ", "VERANO10"},
		{"vacío queda vacío", "", ""},
		{"solo espacios → vacío", "   ", ""},
		{"ya canónico se conserva", "PROMO", "PROMO"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := NormalizarCodigo(c.in); got != c.want {
				t.Errorf("NormalizarCodigo(%q) = %q; quería %q", c.in, got, c.want)
			}
		})
	}
}
