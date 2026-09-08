package empresa

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestMonedaValida(t *testing.T) {
	casos := []struct {
		in   string
		want bool
	}{
		{MonedaVES, true},
		{MonedaUSD, true},
		{"EUR", false}, // divisa activable, no moneda principal
		{"", false},
		{"ves", false}, // sin normalizar: comparación exacta
	}
	for _, c := range casos {
		if got := MonedaValida(c.in); got != c.want {
			t.Errorf("MonedaValida(%q) = %v; quería %v", c.in, got, c.want)
		}
	}
}

func TestFuenteTasaValida(t *testing.T) {
	casos := []struct {
		in   string
		want bool
	}{
		{FuenteTasaBCV, true},
		{FuenteTasaMercado, true},
		{FuenteTasaManual, true},
		{"", false},
		{"otra", false},
	}
	for _, c := range casos {
		if got := FuenteTasaValida(c.in); got != c.want {
			t.Errorf("FuenteTasaValida(%q) = %v; quería %v", c.in, got, c.want)
		}
	}
}

func TestDivisaConocida(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   bool
	}{
		{"USD", MonedaUSD, true},
		{"EUR", MonedaEUR, true},
		{"COP", MonedaCOP, true},
		{"USDT", MonedaUSDT, true},
		{"minúsculas se normalizan", "eur", true},
		{"con espacios se recortan", "  usd ", true},
		{"VES no es activable (base legal)", MonedaVES, false},
		{"desconocida", "GBP", false},
		{"vacío", "", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := DivisaConocida(c.in); got != c.want {
				t.Errorf("DivisaConocida(%q) = %v; quería %v", c.in, got, c.want)
			}
		})
	}
}

func TestFuenteActivaValida(t *testing.T) {
	casos := []struct {
		nombre string
		codigo string
		fuente string
		want   bool
	}{
		{"bcv solo aplica a USD", MonedaUSD, FuenteTasaBCV, true},
		{"bcv en EUR se rechaza", MonedaEUR, FuenteTasaBCV, false},
		{"bcv en eur minúsculas también se rechaza", "eur", FuenteTasaBCV, false},
		{"mercado válido para EUR", MonedaEUR, FuenteTasaMercado, true},
		{"manual válido para COP", MonedaCOP, FuenteTasaManual, true},
		{"fuente desconocida se rechaza", MonedaUSD, "otra", false},
		{"fuente vacía se rechaza", MonedaUSD, "", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := FuenteActivaValida(c.codigo, c.fuente); got != c.want {
				t.Errorf("FuenteActivaValida(%q,%q) = %v; quería %v", c.codigo, c.fuente, got, c.want)
			}
		})
	}
}

func TestFuenteEfectivaDivisa(t *testing.T) {
	casos := []struct {
		nombre string
		codigo string
		fuente string
		want   string
	}{
		{"USD respeta bcv", MonedaUSD, FuenteTasaBCV, FuenteTasaBCV},
		{"USD respeta mercado", MonedaUSD, FuenteTasaMercado, FuenteTasaMercado},
		{"USD respeta manual", MonedaUSD, FuenteTasaManual, FuenteTasaManual},
		{"USD con fuente inválida cae a bcv", MonedaUSD, "basura", FuenteTasaBCV},
		{"USD con fuente vacía cae a bcv", MonedaUSD, "", FuenteTasaBCV},
		{"EUR nunca es bcv: pidiendo bcv cae a manual", MonedaEUR, FuenteTasaBCV, FuenteTasaManual},
		{"EUR con mercado se respeta", MonedaEUR, FuenteTasaMercado, FuenteTasaMercado},
		{"EUR con vacío cae a manual", MonedaEUR, "", FuenteTasaManual},
		{"minúsculas/espacios se normalizan (usd)", "  usd ", FuenteTasaMercado, FuenteTasaMercado},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := FuenteEfectivaDivisa(c.codigo, c.fuente); got != c.want {
				t.Errorf("FuenteEfectivaDivisa(%q,%q) = %q; quería %q", c.codigo, c.fuente, got, c.want)
			}
		})
	}
}

func TestNormalizarMonedasActivas_OK(t *testing.T) {
	in := []MonedaActiva{
		{Codigo: "usd", Fuente: FuenteTasaBCV},        // minúsculas → USD
		{Codigo: MonedaVES, Fuente: FuenteTasaManual}, // VES se descarta (base legal)
		{Codigo: "", Fuente: FuenteTasaManual},        // vacío se descarta
		{Codigo: MonedaEUR, Fuente: ""},               // fuente vacía → efectiva manual
		{Codigo: "USD", Fuente: FuenteTasaBCV},        // duplicado USD → se deduplica
		{Codigo: MonedaCOP, Fuente: FuenteTasaMercado},
	}
	out, err := NormalizarMonedasActivas(in)
	if err != nil {
		t.Fatalf("no se esperaba error, se obtuvo: %v", err)
	}
	want := []MonedaActiva{
		{Codigo: MonedaUSD, Fuente: FuenteTasaBCV},
		{Codigo: MonedaEUR, Fuente: FuenteTasaManual},
		{Codigo: MonedaCOP, Fuente: FuenteTasaMercado},
	}
	if len(out) != len(want) {
		t.Fatalf("largo = %d (%v); quería %d (%v)", len(out), out, len(want), want)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("out[%d] = %+v; quería %+v", i, out[i], want[i])
		}
	}
}

func TestNormalizarMonedasActivas_Errores(t *testing.T) {
	casos := []struct {
		nombre string
		in     []MonedaActiva
	}{
		{"divisa desconocida", []MonedaActiva{{Codigo: "GBP", Fuente: FuenteTasaManual}}},
		{"bcv en divisa no-USD", []MonedaActiva{{Codigo: MonedaEUR, Fuente: FuenteTasaBCV}}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := NormalizarMonedasActivas(c.in); err == nil {
				t.Errorf("se esperaba error para %+v", c.in)
			}
		})
	}
}

func TestNormalizarMonedasActivas_Vacia(t *testing.T) {
	out, err := NormalizarMonedasActivas(nil)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("entrada vacía debe dar salida vacía; se obtuvo %v", out)
	}
}

func TestNormalizarColorHex(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   string
	}{
		{"vacío se queda vacío", "", ""},
		{"solo espacios → vacío", "   ", ""},
		{"forma corta #abc → #aabbcc", "#abc", "#aabbcc"},
		{"forma corta sin # → #aabbcc", "abc", "#aabbcc"},
		{"forma larga con # en minúsculas", "#a1b2c3", "#a1b2c3"},
		{"mayúsculas se pasan a minúsculas", "#ABCDEF", "#abcdef"},
		{"sin # se acepta", "ABCDEF", "#abcdef"},
		{"espacios a los lados se recortan", "  #FfF ", "#ffffff"},
		{"forma corta mayúsculas #FFF", "#FFF", "#ffffff"},
		{"longitud 4 inválida", "#abcd", ""},
		{"longitud 5 inválida", "12345", ""},
		{"carácter no-hex en larga", "gggggg", ""},
		{"carácter no-hex tras expandir corta", "#12g", ""},
		{"símbolo no-hex", "#12345z", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := NormalizarColorHex(c.in); got != c.want {
				t.Errorf("NormalizarColorHex(%q) = %q; quería %q", c.in, got, c.want)
			}
		})
	}
}

func TestPantallaClienteModoValido(t *testing.T) {
	casos := []struct {
		in   string
		want bool
	}{
		{PantallaModoSoloProductos, true},
		{PantallaModoMixta, true},
		{PantallaModoPublicidad, true},
		{"  mixta  ", true}, // se recorta
		{"", false},
		{"otro", false},
	}
	for _, c := range casos {
		if got := PantallaClienteModoValido(c.in); got != c.want {
			t.Errorf("PantallaClienteModoValido(%q) = %v; quería %v", c.in, got, c.want)
		}
	}
}

func TestNormalizarPantallaClienteModo(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   string
	}{
		{"válido se conserva", PantallaModoPublicidad, PantallaModoPublicidad},
		{"con espacios se recorta", "  mixta  ", PantallaModoMixta},
		{"vacío cae al default seguro", "", PantallaModoSoloProductos},
		{"desconocido cae al default seguro", "zzz", PantallaModoSoloProductos},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := NormalizarPantallaClienteModo(c.in); got != c.want {
				t.Errorf("NormalizarPantallaClienteModo(%q) = %q; quería %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizarTemaLogoVersion(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   string
	}{
		{"color", TemaLogoColor, TemaLogoColor},
		{"blanco", TemaLogoBlanco, TemaLogoBlanco},
		{"negro", TemaLogoNegro, TemaLogoNegro},
		{"con espacios se recorta y conserva", "  blanco  ", TemaLogoBlanco},
		{"vacío queda vacío (default color)", "", ""},
		{"desconocido queda vacío", "gris", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := NormalizarTemaLogoVersion(c.in); got != c.want {
				t.Errorf("NormalizarTemaLogoVersion(%q) = %q; quería %q", c.in, got, c.want)
			}
		})
	}
}

func TestTemaPantallaClienteNormalizar(t *testing.T) {
	in := TemaPantallaCliente{
		Fondo:           "#ABC",     // expande + minúsculas
		FondoCabecera:   "roto",     // inválido → vacío
		FondoProductos:  "  #fff ",  // recorta + expande
		FondoTotales:    "",         // vacío se queda vacío
		FondoPublicidad: "112233",   // sin # válido
		ColorTexto:      "#000000",  // válido
		ColorEnfasis:    "#GGGGGG",  // no-hex → vacío
		LogoVersion:     " blanco ", // acota + recorta
	}
	got := in.Normalizar()
	want := TemaPantallaCliente{
		Fondo:           "#aabbcc",
		FondoCabecera:   "",
		FondoProductos:  "#ffffff",
		FondoTotales:    "",
		FondoPublicidad: "#112233",
		ColorTexto:      "#000000",
		ColorEnfasis:    "",
		LogoVersion:     TemaLogoBlanco,
	}
	if got != want {
		t.Errorf("Normalizar() = %+v; quería %+v", got, want)
	}
}

func TestAnuncioSlideUnmarshalBSONValue(t *testing.T) {
	// Un documento embebido (la forma NUEVA) se decodifica normalmente.
	tp, data, err := bson.MarshalValue(map[string]string{
		"tipo": SlideImagen, "imagen": "promo.png",
	})
	if err != nil {
		t.Fatalf("marshal documento: %v", err)
	}
	var doc AnuncioSlide
	if err := doc.UnmarshalBSONValue(tp, data); err != nil {
		t.Fatalf("UnmarshalBSONValue documento: %v", err)
	}
	if doc.Tipo != SlideImagen || doc.Imagen != "promo.png" {
		t.Errorf("documento embebido mal decodificado: %+v", doc)
	}

	// La forma ANTIGUA (un string suelto de URL) no debe romper: queda vacío.
	tp2, data2, err := bson.MarshalValue("http://viejo/foto.png")
	if err != nil {
		t.Fatalf("marshal string: %v", err)
	}
	viejo := AnuncioSlide{Tipo: "sucio"} // debe quedar reseteado a vacío
	if err := viejo.UnmarshalBSONValue(tp2, data2); err != nil {
		t.Fatalf("UnmarshalBSONValue string: %v", err)
	}
	if (viejo != AnuncioSlide{}) {
		t.Errorf("string suelto (forma antigua) debía dar slide vacío; se obtuvo %+v", viejo)
	}
}
