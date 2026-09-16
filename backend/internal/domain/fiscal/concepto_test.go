package fiscal

import "testing"

/* El maestro de conceptos existe para que la tarifa no se teclee. Lo que estas
 * pruebas cuidan es el CÁLCULO, que es donde un error se convierte en un
 * impuesto mal enterado al SENIAT. */

func conceptos() []ConceptoISLR {
	return []ConceptoISLR{
		{Codigo: "honorarios", Sujeto: SujetoNaturalResidente, Porcentaje: 3, Sustraendo: 50, BaseMinima: 1000, Activo: true},
		{Codigo: "honorarios", Sujeto: SujetoJuridicaDomiciliada, Porcentaje: 5, Activo: true},
		{Codigo: "viejo", Sujeto: SujetoJuridicaDomiciliada, Porcentaje: 9, Activo: false},
	}
}

// La misma actividad tiene tarifa distinta según a quién se le retiene: esa es
// la razón de que el maestro lleve el tipo de sujeto y no solo el concepto.
func TestConceptoPara_LaTarifaDependeDelSujeto(t *testing.T) {
	nat, ok := ConceptoPara(conceptos(), "honorarios", SujetoNaturalResidente)
	if !ok || nat.Porcentaje != 3 {
		t.Errorf("a una persona natural le corresponde 3%%, dio %+v", nat)
	}
	jur, ok := ConceptoPara(conceptos(), "honorarios", SujetoJuridicaDomiciliada)
	if !ok || jur.Porcentaje != 5 {
		t.Errorf("a una jurídica le corresponde 5%%, dio %+v", jur)
	}
}

// Un concepto dado de baja no puede seguir usándose: si se pudiera, seguiría
// reteniendo con una tarifa que ya no rige.
func TestConceptoPara_IgnoraLosInactivos(t *testing.T) {
	if _, ok := ConceptoPara(conceptos(), "viejo", SujetoJuridicaDomiciliada); ok {
		t.Error("un concepto inactivo no debería resolver")
	}
	if _, ok := ConceptoPara(conceptos(), "no-existe", SujetoNaturalResidente); ok {
		t.Error("un concepto inexistente no debería resolver")
	}
}

func TestRetener_AplicaTarifaYSustraendo(t *testing.T) {
	c := ConceptoISLR{Porcentaje: 3, Sustraendo: 50, BaseMinima: 1000}
	// 10 000 × 3% = 300, menos 50 de sustraendo = 250.
	if got := c.Retener(10000); got != 250 {
		t.Errorf("retención = %v, se esperaban 250 (300 − 50)", got)
	}
}

// Por debajo del mínimo NO se retiene: si no, se emitirían comprobantes por
// montos irrisorios que después hay que anular a mano.
func TestRetener_RespetaLaBaseMinima(t *testing.T) {
	// Sin sustraendo a propósito: con él, en el mínimo exacto el resultado sería
	// cero por OTRO motivo y la prueba no diría nada sobre la base mínima.
	c := ConceptoISLR{Porcentaje: 3, BaseMinima: 1000}
	if got := c.Retener(999); got != 0 {
		t.Errorf("por debajo del mínimo no se retiene, dio %v", got)
	}
	if got := c.Retener(1000); got != 30 {
		t.Errorf("justo en el mínimo sí se retiene: dio %v, se esperaban 30", got)
	}
}

// El sustraendo puede superar al impuesto calculado. Una retención NEGATIVA no
// existe —sería devolverle dinero al proveedor— y por eso queda en cero.
func TestRetener_NuncaNegativa(t *testing.T) {
	c := ConceptoISLR{Porcentaje: 3, Sustraendo: 500}
	if got := c.Retener(1000); got != 0 { // 30 − 500 < 0
		t.Errorf("una retención negativa no existe: dio %v", got)
	}
}

// Una jurídica no lleva sustraendo: el cálculo es la tarifa pelada.
func TestRetener_SinSustraendo(t *testing.T) {
	c := ConceptoISLR{Porcentaje: 5}
	if got := c.Retener(2000); got != 100 {
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
	for _, s := range []string{SujetoNaturalResidente, SujetoJuridicaDomiciliada} {
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
	}
	if _, ok := ConceptoPara(def, "honorarios", SujetoNaturalResidente); !ok {
		t.Error("honorarios para persona natural es el caso más común y debe venir sembrado")
	}
}
