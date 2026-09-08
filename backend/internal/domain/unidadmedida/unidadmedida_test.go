package unidadmedida

import "testing"

func TestCategoriaValida(t *testing.T) {
	casos := []struct {
		in   string
		want bool
	}{
		{CategoriaConteo, true},
		{CategoriaPeso, true},
		{CategoriaVolumen, true},
		{CategoriaLongitud, true},
		{"", false},
		{"masa", false},
		{"Peso", false}, // comparación exacta, sin normalizar
	}
	for _, c := range casos {
		if got := CategoriaValida(c.in); got != c.want {
			t.Errorf("CategoriaValida(%q) = %v; quería %v", c.in, got, c.want)
		}
	}
}

func TestNormalizarSimbolo(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   string
	}{
		{"recorta espacios", "  kg  ", "kg"},
		{"conserva la caja", "mL", "mL"},
		{"no toca L mayúscula", "L", "L"},
		{"vacío queda vacío", "   ", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := NormalizarSimbolo(c.in); got != c.want {
				t.Errorf("NormalizarSimbolo(%q) = %q; quería %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizarCategoria(t *testing.T) {
	casos := []struct {
		in   string
		want string
	}{
		{"Peso", "peso"},
		{"  Conteo ", "conteo"},
		{"VOLUMEN", "volumen"},
		{"", ""},
	}
	for _, c := range casos {
		if got := NormalizarCategoria(c.in); got != c.want {
			t.Errorf("NormalizarCategoria(%q) = %q; quería %q", c.in, got, c.want)
		}
	}
}

func TestPorDefecto(t *testing.T) {
	us := PorDefecto("emp_x")
	if len(us) != 10 {
		t.Fatalf("PorDefecto debía traer 10 unidades, trajo %d", len(us))
	}
	// Todas del tenant pedido, activas y con categoría válida.
	porSimbolo := map[string]UnidadMedida{}
	porCategoria := map[string]int{}
	for _, u := range us {
		if u.EmpresaID != "emp_x" {
			t.Errorf("unidad %q con empresa %q; quería emp_x", u.Simbolo, u.EmpresaID)
		}
		if !u.Activa {
			t.Errorf("la unidad por defecto %q debía venir activa", u.Simbolo)
		}
		if !CategoriaValida(u.Categoria) {
			t.Errorf("la unidad %q trae categoría inválida %q", u.Simbolo, u.Categoria)
		}
		porSimbolo[u.Simbolo] = u
		porCategoria[u.Categoria]++
	}
	// Un símbolo esperado por categoría (muestra representativa).
	for _, esperado := range []string{"unidad", "docena", "par", "caja", "kg", "g", "L", "mL", "m", "cm"} {
		if _, ok := porSimbolo[esperado]; !ok {
			t.Errorf("faltó la unidad por defecto %q", esperado)
		}
	}
	if porCategoria[CategoriaConteo] != 4 {
		t.Errorf("conteo debía traer 4 unidades, trajo %d", porCategoria[CategoriaConteo])
	}
	if porCategoria[CategoriaPeso] != 2 || porCategoria[CategoriaVolumen] != 2 || porCategoria[CategoriaLongitud] != 2 {
		t.Errorf("peso/volumen/longitud debían traer 2 c/u; se obtuvo %d/%d/%d",
			porCategoria[CategoriaPeso], porCategoria[CategoriaVolumen], porCategoria[CategoriaLongitud])
	}
}
