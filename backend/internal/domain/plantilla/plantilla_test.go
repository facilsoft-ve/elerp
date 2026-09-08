package plantilla

import (
	"strings"
	"testing"
)

func TestTipoValido(t *testing.T) {
	casos := []struct {
		in   string
		want bool
	}{
		{TipoFactura, true},
		{TipoCotizacion, true},
		{TipoOrden, true},
		{"", false},
		{"factura_x", false},
	}
	for _, c := range casos {
		if got := TipoValido(c.in); got != c.want {
			t.Errorf("TipoValido(%q) = %v; quería %v", c.in, got, c.want)
		}
	}
}

func TestNormalizarTipo(t *testing.T) {
	if got := NormalizarTipo("  Factura "); got != "factura" {
		t.Errorf("NormalizarTipo = %q; quería %q", got, "factura")
	}
}

func TestPapelDimensiones(t *testing.T) {
	if a, h, ok := PapelDimensiones(PapelCarta); !ok || a != 215.9 || h != 279.4 {
		t.Errorf("carta = %v×%v ok=%v; quería 215.9×279.4 ok=true", a, h, ok)
	}
	if a, h, ok := PapelDimensiones(PapelTicket58); !ok || a != 58 {
		t.Errorf("ticket_58 ancho = %v (alto %v ok %v); quería 58", a, h, ok)
	}
	if _, _, ok := PapelDimensiones(PapelPersonalizado); ok {
		t.Errorf("custom no debe tener dimensiones de dominio (ok=true)")
	}
}

func TestPapelValido(t *testing.T) {
	for _, p := range []string{PapelCarta, PapelMediaCarta, PapelA4, PapelTicket80, PapelTicket58, PapelPersonalizado} {
		if !PapelValido(p) {
			t.Errorf("PapelValido(%q) = false; quería true", p)
		}
	}
	if PapelValido("oficio") {
		t.Errorf("PapelValido(oficio) = true; quería false")
	}
}

func TestTipoBloqueValido(t *testing.T) {
	if !TipoBloqueValido(BloqueTexto) || !TipoBloqueValido(BloqueTablaItems) || !TipoBloqueValido(BloqueImagen) {
		t.Errorf("bloques válidos rechazados")
	}
	if TipoBloqueValido("video") {
		t.Errorf("bloque 'video' no debería ser válido")
	}
}

func TestImagenDataURISegura(t *testing.T) {
	if !ImagenDataURISegura("") {
		t.Errorf("cadena vacía (sin imagen) debía ser válida")
	}
	if !ImagenDataURISegura("data:image/png;base64,AAAA") {
		t.Errorf("PNG debía aceptarse")
	}
	if !ImagenDataURISegura("data:image/webp;base64,AAAA") {
		t.Errorf("WebP debía aceptarse")
	}
	if ImagenDataURISegura("data:image/svg+xml;base64,AAAA") {
		t.Errorf("SVG NUNCA debe aceptarse (riesgo XSS)")
	}
	if ImagenDataURISegura("data:text/html;base64,AAAA") {
		t.Errorf("un data URI que no es imagen debía rechazarse")
	}
	grande := "data:image/png;base64," + strings.Repeat("A", MaxImagenBase64)
	if ImagenDataURISegura(grande) {
		t.Errorf("una imagen por encima del peso máximo debía rechazarse")
	}
}
