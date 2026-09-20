package fiscal

import "testing"

/* El maestro de conceptos existe para que la tarifa no se teclee. Lo que estas
 * pruebas cuidan es el CÁLCULO, que es donde un error se convierte en un
 * impuesto mal enterado al SENIAT. */

func conceptos() []ConceptoISLR {
	return []ConceptoISLR{
		{Codigo: "honorarios", Sujeto: SujetoNaturalResidente, Porcentaje: 3, SustraendoUT: 10, BaseMinimaUT: 200, Activo: true},
		{Codigo: "honorarios", Sujeto: SujetoJuridicaDomiciliada, Porcentaje: 5, Activo: true},
		{Codigo: "viejo", Sujeto: SujetoJuridicaDomiciliada, Porcentaje: 9, Activo: false},
	}
}

// La misma actividad tiene tarifa distinta según a quién se le retiene: esa es
// la razón de que el maestro lleve el tipo de sujeto y no solo el concepto.
func TestConceptoPara_LaTarifaDependeDelSujeto(t *testing.T) {
	nat, ok := ConceptoPara(conceptos(), "honorarios", SujetoNaturalResidente, 0)
	if !ok || nat.Porcentaje != 3 {
		t.Errorf("a una persona natural le corresponde 3%%, dio %+v", nat)
	}
	jur, ok := ConceptoPara(conceptos(), "honorarios", SujetoJuridicaDomiciliada, 0)
	if !ok || jur.Porcentaje != 5 {
		t.Errorf("a una jurídica le corresponde 5%%, dio %+v", jur)
	}
}

// Un concepto dado de baja no puede seguir usándose: si se pudiera, seguiría
// reteniendo con una tarifa que ya no rige.
func TestConceptoPara_IgnoraLosInactivos(t *testing.T) {
	if _, ok := ConceptoPara(conceptos(), "viejo", SujetoJuridicaDomiciliada, 0); ok {
		t.Error("un concepto inactivo no debería resolver")
	}
	if _, ok := ConceptoPara(conceptos(), "no-existe", SujetoNaturalResidente, 0); ok {
		t.Error("un concepto inexistente no debería resolver")
	}
}

func TestRetener_AplicaTarifaYSustraendo(t *testing.T) {
	// UT = 5: mínimo 200 UT × 5 = 1.000; sustraendo 10 UT × 5 × 3% = 1,50.
	c := ConceptoISLR{Porcentaje: 3, SustraendoUT: 10, BaseMinimaUT: 200}
	// 10 000 × 3% = 300, menos 1,50 de sustraendo = 298,50.
	if got := c.Retener(10000, 5); got != 298.5 {
		t.Errorf("retención = %v, se esperaban 298,50 (300 − 1,50)", got)
	}
}

// EL SUSTRAENDO SE MUEVE CON LA UT, que es la razón de guardarlo en unidades
// tributarias y no en bolívares. Duplicar la UT duplica el sustraendo sin tocar
// una sola fila del maestro.
func TestRetener_ElSustraendoSigueALaUT(t *testing.T) {
	c := ConceptoISLR{Porcentaje: 3, SustraendoUT: 10}
	// UT 5 ⇒ sustraendo 1,50. UT 10 ⇒ sustraendo 3,00.
	if got := c.Retener(10000, 5); got != 298.5 {
		t.Errorf("con UT 5: %v, se esperaban 298,50", got)
	}
	if got := c.Retener(10000, 10); got != 297 {
		t.Errorf("con UT 10: %v, se esperaban 297", got)
	}
}

// RequiereUT distingue los conceptos que no se pueden calcular sin saber cuánto
// vale la UT de los que sí (las jurídicas, sin sustraendo ni mínimo).
func TestRequiereUT(t *testing.T) {
	if (ConceptoISLR{Porcentaje: 5}).RequiereUT() {
		t.Error("una tarifa pelada no necesita la UT")
	}
	if !(ConceptoISLR{Porcentaje: 3, SustraendoUT: 83.33}).RequiereUT() {
		t.Error("un concepto con sustraendo en UT sí la necesita")
	}
	if !(ConceptoISLR{Porcentaje: 3, BaseMinimaUT: 83.33}).RequiereUT() {
		t.Error("un concepto con mínimo en UT sí la necesita")
	}
}

// Por debajo del mínimo NO se retiene: si no, se emitirían comprobantes por
// montos irrisorios que después hay que anular a mano.
func TestRetener_RespetaLaBaseMinima(t *testing.T) {
	// Sin sustraendo a propósito: con él, en el mínimo exacto el resultado sería
	// cero por OTRO motivo y la prueba no diría nada sobre la base mínima.
	c := ConceptoISLR{Porcentaje: 3, BaseMinimaUT: 200} // × UT 5 = 1.000
	if got := c.Retener(999, 5); got != 0 {
		t.Errorf("por debajo del mínimo no se retiene, dio %v", got)
	}
	if got := c.Retener(1000, 5); got != 30 {
		t.Errorf("justo en el mínimo sí se retiene: dio %v, se esperaban 30", got)
	}
}

// El sustraendo puede superar al impuesto calculado. Una retención NEGATIVA no
// existe —sería devolverle dinero al proveedor— y por eso queda en cero.
func TestRetener_NuncaNegativa(t *testing.T) {
	c := ConceptoISLR{Porcentaje: 3, SustraendoUT: 20000}
	if got := c.Retener(1000, 5); got != 0 { // 30 − 3.000 < 0
		t.Errorf("una retención negativa no existe: dio %v", got)
	}
}

// Una jurídica no lleva sustraendo: el cálculo es la tarifa pelada.
func TestRetener_SinSustraendo(t *testing.T) {
	c := ConceptoISLR{Porcentaje: 5}
	if got := c.Retener(2000, 5); got != 100 {
		t.Errorf("retención = %v, se esperaban 100", got)
	}
}

func TestCodigosDeConcepto_SinRepetirYSoloActivos(t *testing.T) {
	cods := CodigosDeConcepto(conceptos())
	if len(cods) != 1 || cods[0] != "honorarios" {
		t.Errorf("debería dar solo «honorarios» (sin repetir, sin inactivos): %v", cods)
	}
}

func TestSujetoValido(t *testing.T) {
	for _, s := range []string{SujetoNaturalResidente, SujetoNaturalNoResidente,
		SujetoJuridicaDomiciliada, SujetoJuridicaNoDomiciliada} {
		if !SujetoValido(s) {
			t.Errorf("%q debería ser válido", s)
		}
	}
	for _, s := range []string{"", "natural", "empresa"} {
		if SujetoValido(s) {
			t.Errorf("%q no debería ser válido", s)
		}
	}
}

// La tabla sembrada tiene que ser usable tal cual: cada concepto con sus dos
// sujetos donde corresponda, y ninguna tarifa en cero (una tarifa en cero no
// retiene nada y se ve configurada).
func TestConceptosPorDefecto_UsablesTalCual(t *testing.T) {
	def := ConceptosPorDefecto("emp_x")
	if len(def) == 0 {
		t.Fatal("la tabla sembrada no puede venir vacía")
	}
	for _, c := range def {
		if c.EmpresaID != "emp_x" || !c.Activo {
			t.Errorf("%s/%s: mal sembrado", c.Codigo, c.Sujeto)
		}
		if !SujetoValido(c.Sujeto) {
			t.Errorf("%s: sujeto inválido %q", c.Codigo, c.Sujeto)
		}
		if c.Porcentaje <= 0 {
			t.Errorf("%s/%s: una tarifa en cero no retiene nada y se ve configurada", c.Codigo, c.Sujeto)
		}
		// A las personas naturales residentes se les retiene con sustraendo. Sembrarlo
		// en cero —como estaba— les retiene DE MÁS en cada factura, sin fallar nada.
		if c.Sujeto == SujetoNaturalResidente && c.SustraendoUT <= 0 {
			t.Errorf("%s: una persona natural residente sin sustraendo retiene de más", c.Codigo)
		}
	}
	if _, ok := ConceptoPara(def, "honorarios", SujetoNaturalResidente, 0); !ok {
		t.Error("honorarios para persona natural es el caso más común y debe venir sembrado")
	}
}
