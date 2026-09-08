package tasafuente_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mornix/elerp/internal/adapter/tasafuente"
)

// portadaBCV reproduce la estructura real de la portada del BCV: el bloque del
// dólar identificado por id="dolar", con la cifra en <strong> y coma decimal, y
// la fecha valor en el atributo content de la fecha publicada. Es el fixture
// contra el que se verifica el raspado sin salir a internet.
const portadaBCV = `<!DOCTYPE html><html><head><title>BCV</title></head><body>
<div class="view-content">
  <span class="date-display-single" property="dc:date" content="2026-07-31T00:00:00-04:00">Jueves, 31 Julio 2026</span>
</div>
<div id="euro" class="col-sm-6 col-xs-6 recuadrotsmc">
  <div class="col-sm-6 col-xs-6 centrado"><span><sup>EUR</sup></span></div>
  <div class="col-sm-6 col-xs-6 centrado"><strong> 812,45000000</strong></div>
</div>
<div id="dolar" class="col-sm-6 col-xs-6 recuadrotsmc">
  <div class="col-sm-6 col-xs-6 centrado"><span><sup>USD</sup></span></div>
  <div class="col-sm-6 col-xs-6 centrado"><strong> 745,63000000</strong></div>
</div>
</body></html>`

func TestExtraerBCV_LeeLaTasaYLaFechaValor(t *testing.T) {
	valor, fecha, err := tasafuente.ExtraerBCV([]byte(portadaBCV))
	if err != nil {
		t.Fatalf("extraer: %v", err)
	}
	if valor != 745.63 {
		t.Errorf("valor esperado 745,63, se obtuvo %v", valor)
	}
	if fecha != "2026-07-31" {
		t.Errorf("fecha valor esperada 2026-07-31, se obtuvo %q", fecha)
	}
}

func TestExtraerBCV_NoConfundeElDolarConElEuro(t *testing.T) {
	// El euro va ANTES en el HTML y su cifra es mayor. Si el extractor tomara el
	// primer <strong> del documento, publicaríamos la tasa del euro como dólar.
	valor, _, err := tasafuente.ExtraerBCV([]byte(portadaBCV))
	if err != nil {
		t.Fatalf("extraer: %v", err)
	}
	if valor == 812.45 {
		t.Fatal("se tomó la cifra del euro en lugar de la del dólar")
	}
}

func TestExtraerBCV_SiElHTMLCambiaFallaEnVezDeInventar(t *testing.T) {
	// Es el riesgo aceptado del raspado: si el BCV rediseña su portada, esto
	// tiene que FALLAR limpio para que la aplicación conserve la última tasa
	// buena. Lo que no puede hacer nunca es devolver un número cualquiera.
	sinBloque := strings.Replace(portadaBCV, `id="dolar"`, `id="dolares-hoy"`, 1)
	if _, _, err := tasafuente.ExtraerBCV([]byte(sinBloque)); !errors.Is(err, tasafuente.ErrSinDato) {
		t.Fatalf("se esperaba ErrSinDato al no encontrar el bloque, se obtuvo: %v", err)
	}
}

func TestExtraerBCV_RechazaCifraAlterada(t *testing.T) {
	// Un HTML manipulado por un intermediario: donde iba la cifra hay otra cosa.
	// El extractor no acepta nada que no sea un decimal con formato venezolano.
	casos := map[string]string{
		"script inyectado":     `<strong>7<script>alert(1)</script>45,63</strong>`,
		"notación exponencial": `<strong>7e10</strong>`,
		"cifra negativa":       `<strong>-745,63</strong>`,
		"texto":                `<strong>consultar</strong>`,
		"separadores absurdos": `<strong>7.4.5,6.3</strong>`,
	}
	for nombre, cifra := range casos {
		html := strings.Replace(portadaBCV, `<strong> 745,63000000</strong>`, cifra, 1)
		if v, _, err := tasafuente.ExtraerBCV([]byte(html)); err == nil {
			t.Errorf("%s: debía rechazarse, se obtuvo el valor %v", nombre, v)
		}
	}
}

func TestParsearDecimalVE(t *testing.T) {
	ok := map[string]float64{
		"745,63":       745.63,
		"1.234,56":     1234.56,
		"36,62150000":  36.6215,
		"745":          745,
		" 745,63 ":     745.63,
		"1.000.000,00": 1000000,
	}
	for entrada, esperado := range ok {
		v, err := tasafuente.ParsearDecimalVE(entrada)
		if err != nil {
			t.Errorf("%q: %v", entrada, err)
			continue
		}
		if v != esperado {
			t.Errorf("%q: esperado %v, se obtuvo %v", entrada, esperado, v)
		}
	}
	// El formato anglosajón se rechaza a propósito: si el BCV empezara a
	// publicar "745.63", la ambigüedad con el punto de miles podría convertir
	// 1.234 (mil doscientos treinta y cuatro) en 1,234. Mejor fallar.
	for _, malo := range []string{"745.63", "", "abc", "NaN", "1,2,3"} {
		if v, err := tasafuente.ParsearDecimalVE(malo); err == nil {
			t.Errorf("%q debía rechazarse, se obtuvo %v", malo, v)
		}
	}
}
