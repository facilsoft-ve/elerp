package tasa

import "testing"

// La SELECCIÓN de la tasa vigente por moneda/fecha (UltimaVigente…) es contrato
// del Repository, y la VALIDACIÓN de plausibilidad (MinPlausible/MaxPlausible/
// VariacionMax → cuarentena) se aplica en internal/application/tasa.go, ya
// cubierta por sus tests. Aquí solo se prueba la lógica PURA del dominio: el
// normalizador de moneda y los predicados de la Tasa.

func TestNormalizarMoneda(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   string
	}{
		{"vacío se lee como USD (retrocompat)", "", MonedaUSD},
		{"solo espacios → USD", "   ", MonedaUSD},
		{"minúsculas → mayúsculas", "usd", "USD"},
		{"con espacios se recorta", "  eur ", "EUR"},
		{"ya canónica se conserva", "USDT", "USDT"},
		{"mixto", "CoP", "COP"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := NormalizarMoneda(c.in); got != c.want {
				t.Errorf("NormalizarMoneda(%q) = %q; quería %q", c.in, got, c.want)
			}
		})
	}
}

func TestTasaVigente(t *testing.T) {
	casos := []struct {
		nombre string
		estado string
		want   bool
	}{
		{"vigente se puede usar", EstadoVigente, true},
		{"rechazada (cuarentena) no", EstadoRechazada, false},
		{"estado vacío no es vigente", "", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := (Tasa{Estado: c.estado}).Vigente(); got != c.want {
				t.Errorf("Tasa{Estado:%q}.Vigente() = %v; quería %v", c.estado, got, c.want)
			}
		})
	}
}

func TestTasaOficial(t *testing.T) {
	casos := []struct {
		nombre string
		fuente string
		want   bool
	}{
		{"BCV directo es oficial", FuenteBCV, true},
		{"respaldo (republica BCV) es oficial", FuenteRespaldo, true},
		{"mercado NO es oficial", FuenteMercado, false},
		{"manual NO es oficial", FuenteManual, false},
		{"semilla NO es oficial", FuenteSemilla, false},
		{"vacío NO es oficial", "", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := (Tasa{Fuente: c.fuente}).Oficial(); got != c.want {
				t.Errorf("Tasa{Fuente:%q}.Oficial() = %v; quería %v", c.fuente, got, c.want)
			}
		})
	}
}
