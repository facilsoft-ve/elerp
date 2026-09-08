package listaprecio

import "testing"

// La resolución de precio por SKU (recorrer Items para encontrar el precio de un
// producto en la lista) la hace la capa de venta/aplicación, no el dominio: la
// ListaPrecio solo expone los Items, sin método de búsqueda. La única lógica
// PURA de este paquete es el validador de tipo (venta/compra).

func TestTipoValido(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   bool
	}{
		{"venta", TipoVenta, true},
		{"compra", TipoCompra, true},
		{"vacío", "", false},
		{"desconocido", "traslado", false},
		{"mayúsculas no coinciden (exacto)", "Venta", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := TipoValido(c.in); got != c.want {
				t.Errorf("TipoValido(%q) = %v; quería %v", c.in, got, c.want)
			}
		})
	}
}
