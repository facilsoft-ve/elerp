package contabilidad

import "testing"

// eqDinero compara dos importes de dinero con tolerancia de medio céntimo,
// evitando falsos negativos por la representación binaria de los float64.
func eqDinero(a, b float64) bool {
	d := a - b
	return d < 0.005 && d > -0.005
}

// TestCuadra cubre el corazón del libro diario: un asiento cuadra solo si la
// suma del debe iguala a la del haber dentro de medio céntimo (< 0.005 estricto).
func TestCuadra(t *testing.T) {
	casos := []struct {
		nombre string
		lineas []Linea
		quiere bool
	}{
		{
			nombre: "cuadrado_simple",
			lineas: []Linea{{Debe: 100}, {Haber: 100}},
			quiere: true,
		},
		{
			nombre: "cuadrado_multilinea",
			lineas: []Linea{{Debe: 116}, {Haber: 100}, {Haber: 16}},
			quiere: true,
		},
		{
			nombre: "descuadrado",
			lineas: []Linea{{Debe: 100}, {Haber: 90}},
			quiere: false,
		},
		{
			// Sin líneas, debe y haber valen 0: 0 == 0, así que un asiento vacío
			// "cuadra" trivialmente (la regla de no-vacío vive en application).
			nombre: "vacio",
			lineas: nil,
			quiere: true,
		},
		{
			// Diferencia de 0.004 < 0.005: dentro de tolerancia => cuadra.
			nombre: "dentro_de_tolerancia",
			lineas: []Linea{{Debe: 100.004}, {Haber: 100}},
			quiere: true,
		},
		{
			// Diferencia de exactamente 0.005: el umbral es ESTRICTO (< 0.005),
			// así que NO cuadra.
			nombre: "en_el_umbral_no_cuadra",
			lineas: []Linea{{Debe: 0.005}, {Haber: 0}},
			quiere: false,
		},
		{
			nombre: "claramente_fuera_de_tolerancia",
			lineas: []Linea{{Debe: 100.01}, {Haber: 100}},
			quiere: false,
		},
		{
			// Acumulación de float: 0.1+0.1+0.1 != 0.3 en binario, pero la
			// diferencia es ínfima y debe caer dentro de la tolerancia.
			nombre: "acumulacion_float",
			lineas: []Linea{{Debe: 0.1}, {Debe: 0.1}, {Debe: 0.1}, {Haber: 0.3}},
			quiere: true,
		},
		{
			// Un asiento contrario puede llevar importes negativos en ambos lados;
			// mientras el neto cuadre, es válido.
			nombre: "negativos_cuadrados",
			lineas: []Linea{{Debe: -50}, {Haber: -50}},
			quiere: true,
		},
		{
			// Solo debe, sin contrapartida: descuadrado.
			nombre: "un_solo_lado",
			lineas: []Linea{{Debe: 100}},
			quiere: false,
		},
		{
			nombre: "ceros_en_ambos_lados",
			lineas: []Linea{{Debe: 0, Haber: 0}},
			quiere: true,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			a := Asiento{Lineas: c.lineas}
			if got := a.Cuadra(); got != c.quiere {
				t.Errorf("Cuadra() = %v; se esperaba %v (líneas=%+v)", got, c.quiere, c.lineas)
			}
		})
	}
}

// TestNaturaleza verifica la naturaleza deudora/acreedora por tipo de cuenta:
// activo, costo y gasto son deudoras; pasivo, patrimonio e ingreso, acreedoras.
func TestNaturaleza(t *testing.T) {
	casos := []struct {
		tipo    string
		deudora bool
	}{
		{TipoActivo, true},
		{TipoCosto, true},
		{TipoGasto, true},
		{TipoPasivo, false},
		{TipoPatrimonio, false},
		{TipoIngreso, false},
		{"desconocido", false},
		{"", false},
		{"Activo", false}, // sensible a mayúsculas: solo el valor canónico cuenta
	}
	for _, c := range casos {
		t.Run("tipo_"+c.tipo, func(t *testing.T) {
			if got := Naturaleza(c.tipo); got != c.deudora {
				t.Errorf("Naturaleza(%q) = %v; se esperaba %v", c.tipo, got, c.deudora)
			}
		})
	}
}

// TestTiposCuenta_Valores fija el valor canónico de cada tipo de cuenta. Son
// strings que se persisten y de los que depende Naturaleza y todo el balance:
// un renombre silencioso rompería el signo de los saldos y las proyecciones.
func TestTiposCuenta_Valores(t *testing.T) {
	casos := map[string]string{
		"activo":     TipoActivo,
		"pasivo":     TipoPasivo,
		"patrimonio": TipoPatrimonio,
		"ingreso":    TipoIngreso,
		"costo":      TipoCosto,
		"gasto":      TipoGasto,
	}
	for esperado, got := range casos {
		if got != esperado {
			t.Errorf("tipo de cuenta = %q; se esperaba %q", got, esperado)
		}
	}
}

// TestCodigosPlan_Valores fija los códigos del plan base, incluida la cuenta
// 5202 (Diferencia en compras) que exige la operación venezolana. Estos códigos
// son el contrato con los asientos derivados; cambiarlos descuadra el histórico.
func TestCodigosPlan_Valores(t *testing.T) {
	casos := map[string]string{
		"1101": CtaCajaBancos,
		"1102": CtaCuentasPorCobrar,
		"1103": CtaIVACreditoFiscal,
		"1104": CtaRetencionIVAaFavor,
		"1105": CtaRetencionISLRaFavor,
		"1201": CtaInventario,
		"2101": CtaCuentasPorPagar,
		"2201": CtaIVADebito,
		"2202": CtaIGTFPorPagar,
		"2203": CtaIVARetenidoPorEnterar,
		"2204": CtaISLRRetenidoPorEnterar,
		"3101": CtaCapital,
		"4101": CtaVentas,
		"4102": CtaVentasExentas,
		"5101": CtaCostoDeVentas,
		"5201": CtaGastosOperativos,
		"5202": CtaDiferenciaEnCompras,
	}
	for esperado, got := range casos {
		if got != esperado {
			t.Errorf("código de cuenta = %q; se esperaba %q", got, esperado)
		}
	}
}

// TestCodigosPlan_Distintos garantiza que no haya dos cuentas del plan base con
// el mismo código: un código duplicado colapsaría dos cuentas en el mayor.
func TestCodigosPlan_Distintos(t *testing.T) {
	codigos := []string{
		CtaCajaBancos, CtaCuentasPorCobrar, CtaIVACreditoFiscal, CtaRetencionIVAaFavor,
		CtaRetencionISLRaFavor, CtaInventario, CtaCuentasPorPagar, CtaIVADebito,
		CtaIGTFPorPagar, CtaIVARetenidoPorEnterar, CtaISLRRetenidoPorEnterar,
		CtaCapital, CtaVentas, CtaVentasExentas,
		CtaCostoDeVentas, CtaGastosOperativos, CtaDiferenciaEnCompras,
	}
	visto := map[string]bool{}
	for _, c := range codigos {
		if visto[c] {
			t.Errorf("código de cuenta duplicado en el plan base: %q", c)
		}
		visto[c] = true
	}
}
