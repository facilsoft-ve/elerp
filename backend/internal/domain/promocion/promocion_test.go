package promocion

import "testing"

// La vigencia/actividad de una promoción en el carrusel (Activa + rango
// Desde/Hasta) se resuelve en la capa de aplicación al armar la pantalla del
// cliente. Aquí solo se prueba la lógica PURA del dominio: el validador y el
// normalizador de tipo.

func TestTipoValido(t *testing.T) {
	casos := []struct {
		in   string
		want bool
	}{
		{TipoImagen, true},
		{TipoTexto, true},
		{"", false},
		{"video", false},
		{"IMAGEN", false}, // comparación exacta, sin normalizar
	}
	for _, c := range casos {
		if got := TipoValido(c.in); got != c.want {
			t.Errorf("TipoValido(%q) = %v; quería %v", c.in, got, c.want)
		}
	}
}

func TestNormalizarTipo(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   string
	}{
		{"imagen se conserva", TipoImagen, TipoImagen},
		{"texto se conserva", TipoTexto, TipoTexto},
		{"mayúsculas se bajan", "IMAGEN", TipoImagen},
		{"espacios se recortan", "  texto  ", TipoTexto},
		{"mixto normaliza", " ImAgEn ", TipoImagen},
		{"vacío cae al default texto", "", TipoTexto},
		{"desconocido cae al default texto", "gif", TipoTexto},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := NormalizarTipo(c.in); got != c.want {
				t.Errorf("NormalizarTipo(%q) = %q; quería %q", c.in, got, c.want)
			}
		})
	}
}
