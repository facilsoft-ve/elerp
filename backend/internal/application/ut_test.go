package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

/* LA UNIDAD TRIBUTARIA.
 *
 * El reglamento expresa los sustraendos y los mínimos de ISLR en UT, no en
 * bolívares, y el SENIAT ajusta el valor de la UT. Guardar bolívares en la tabla
 * de conceptos hace que el número envejezca con cada providencia SIN QUE FALLE
 * NADA: a partir de ese día se retiene mal en cada factura.
 *
 * Lo que estas pruebas cuidan es justamente eso: que el error sea imposible de
 * cometer en silencio. */

// servicioSinUT devuelve un servicio con el maestro de conceptos cableado pero
// SIN ninguna UT cargada, que es como queda una empresa recién creada.
func servicioSinUT(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConConceptosISLR(st.ConceptosISLR)
	svc.ConUnidadesTributarias(st.UnidadesTributarias)
	return svc
}

// TestUT_MandaLaQueRegiaEseDia: registrar hoy una factura de agosto tiene que
// usar la UT de agosto. Sin esto, capturar facturas atrasadas después de una
// providencia da un número distinto al del comprobante que ya se emitió.
func TestUT_MandaLaQueRegiaEseDia(t *testing.T) {
	svc := servicioSinUT(t)
	cargar := func(valor float64, desde string) {
		t.Helper()
		if _, err := svc.CargarUT(empDemo, actorA, origenTst, fiscal.UnidadTributaria{
			Valor: valor, VigenteDesde: desde, Fuente: "Gaceta de prueba",
		}); err != nil {
			t.Fatalf("cargar UT %v: %v", valor, err)
		}
	}
	cargar(9, "2026-01-01")
	cargar(40, "2026-09-01")

	casos := []struct {
		fecha string
		valor float64
		por   string
	}{
		{"2026-08-31", 9, "el día anterior al cambio sigue rigiendo la vieja"},
		{"2026-09-01", 40, "el día del cambio rige la nueva"},
		{"2026-12-31", 40, "después del cambio sigue rigiendo la nueva"},
	}
	for _, c := range casos {
		u, ok := svc.UTVigenteEn(empDemo, c.fecha)
		if !ok || u.Valor != c.valor {
			t.Errorf("%s (%s): se esperaba %v, dio %v (hay=%v)", c.por, c.fecha, c.valor, u.Valor, ok)
		}
	}

	// Antes de la primera cargada no hay ninguna: eso NO es «la UT vale cero».
	if _, ok := svc.UTVigenteEn(empDemo, "2025-06-01"); ok {
		t.Error("antes de la primera UT cargada no debería resolver ninguna")
	}
}

// TestUT_NoSeDuplicaLaFecha: dos filas que arrancan el mismo día dejarían el
// valor decidido por el orden de lectura.
func TestUT_NoSeDuplicaLaFecha(t *testing.T) {
	svc := servicioSinUT(t)
	u := fiscal.UnidadTributaria{Valor: 40, VigenteDesde: "2026-09-01"}
	if _, err := svc.CargarUT(empDemo, actorA, origenTst, u); err != nil {
		t.Fatalf("cargar: %v", err)
	}
	u.Valor = 50
	if _, err := svc.CargarUT(empDemo, actorA, origenTst, u); !errors.Is(err, application.ErrUTRepetida) {
		t.Fatalf("repetir la fecha debía dar ErrUTRepetida, se obtuvo: %v", err)
	}
}

// TestUT_RechazaValoresQueNoSonUnaUT: un valor en cero anularía todos los
// sustraendos, que es exactamente el fallo que este maestro viene a evitar.
func TestUT_RechazaValoresQueNoSonUnaUT(t *testing.T) {
	svc := servicioSinUT(t)
	malos := []fiscal.UnidadTributaria{
		{Valor: 0, VigenteDesde: "2026-09-01"},
		{Valor: -40, VigenteDesde: "2026-09-01"},
		{Valor: 40, VigenteDesde: ""},
		{Valor: 40, VigenteDesde: "2026-09"},
	}
	for _, u := range malos {
		if _, err := svc.CargarUT(empDemo, actorA, origenTst, u); !errors.Is(err, application.ErrUTInvalida) {
			t.Errorf("%+v debía dar ErrUTInvalida, se obtuvo: %v", u, err)
		}
	}
}

// TestConcepto_ConSustraendoExigeLaUTAlGuardar: se avisa cuando quien configura
// tiene el contexto, no al primer documento que lo use semanas después.
func TestConcepto_ConSustraendoExigeLaUTAlGuardar(t *testing.T) {
	svc := servicioSinUT(t)
	conSustraendo := fiscal.ConceptoISLR{
		Codigo: "consultoria_pn", Nombre: "Consultoría a persona natural",
		Sujeto: fiscal.SujetoNaturalResidente, Porcentaje: 3, SustraendoUT: 83.33, Activo: true,
	}
	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, conSustraendo); !errors.Is(err, application.ErrUTNoCargada) {
		t.Fatalf("sin UT cargada debía dar ErrUTNoCargada, se obtuvo: %v", err)
	}

	// Un concepto SIN sustraendo ni mínimo no depende de la UT y se guarda igual.
	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, fiscal.ConceptoISLR{
		Codigo: "consultoria_pj", Nombre: "Consultoría a jurídica",
		Sujeto: fiscal.SujetoJuridicaDomiciliada, Porcentaje: 5, Activo: true,
	}); err != nil {
		t.Fatalf("una tarifa pelada no necesita la UT: %v", err)
	}

	// Con la UT cargada, el mismo concepto entra.
	if _, err := svc.CargarUT(empDemo, actorA, origenTst, fiscal.UnidadTributaria{
		Valor: 40, VigenteDesde: "2026-01-01",
	}); err != nil {
		t.Fatalf("cargar UT: %v", err)
	}
	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, conSustraendo); err != nil {
		t.Fatalf("con la UT cargada debía entrar: %v", err)
	}
}

// TestOC_SinUTLoDiceEnVezDeRetenerDeMas es la prueba que da sentido a todo esto.
//
// La tabla sembrada trae el sustraendo de las personas naturales. Sin UT, ese
// sustraendo vale cero y la retención sale DE MÁS — sin error, sin aviso, en cada
// factura. La orden tiene que decir que falta la UT, no inventar un número.
func TestOC_SinUTLoDiceEnVezDeRetenerDeMas(t *testing.T) {
	svc := servicioSinUT(t)
	sku := primerSKU(t, svc)
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Profesional independiente", RetieneISLR: true,
		ConceptoISLRCodigo: "honorarios", SujetoISLR: fiscal.SujetoNaturalResidente,
	})

	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: prov.ID, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 100}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if len(oc.RetencionISLRDetalle) != 1 {
		t.Fatalf("la falta de UT tenía que dejar constancia: %+v", oc.RetencionISLRDetalle)
	}
	if oc.RetencionISLRDetalle[0].Impedimento != compra.ImpedimentoSinUT {
		t.Fatalf("el impedimento debía ser %q, es %q", compra.ImpedimentoSinUT, oc.RetencionISLRDetalle[0].Impedimento)
	}
	casiEq(t, oc.RetencionISLRMonto, 0, "sin UT no se retiene: se avisa")

	// A una JURÍDICA, cuyo concepto no lleva sustraendo, no le afecta: la falta de
	// UT solo frena a quien la necesita.
	pj := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Consultora, C.A.", RetieneISLR: true,
		ConceptoISLRCodigo: "honorarios", SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	})
	o2 := ocDe(t, svc, pj.ID, sku, nil)
	casiEq(t, o2.RetISLR, 50, "una tarifa sin sustraendo no depende de la UT")
}

// TestOC_ElSustraendoSeAplicaConLaUTDelDia: el camino feliz completo, para que el
// número quede escrito y cualquiera pueda rehacerlo a mano.
func TestOC_ElSustraendoSeAplicaConLaUTDelDia(t *testing.T) {
	svc := servicioSinUT(t)
	if _, err := svc.CargarUT(empDemo, actorA, origenTst, fiscal.UnidadTributaria{
		Valor: 10, VigenteDesde: "2020-01-01", Fuente: "Gaceta de prueba",
	}); err != nil {
		t.Fatalf("cargar UT: %v", err)
	}
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "SRV-HON-UT", Nombre: "Honorarios", Precio: 100, ConceptoISLR: "honorarios",
	}); err != nil {
		t.Fatalf("crear servicio: %v", err)
	}
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Profesional independiente", RetieneISLR: true,
		ConceptoISLRCodigo: "honorarios", SujetoISLR: fiscal.SujetoNaturalResidente,
	})

	oc := ocConLineas(t, svc, prov.ID, "SRV-HON-UT") // base 1.000
	if len(oc.RetencionISLRDetalle) != 1 {
		t.Fatalf("desglose inesperado: %+v", oc.RetencionISLRDetalle)
	}
	d := oc.RetencionISLRDetalle[0]
	if d.Impedimento != "" {
		t.Fatalf("con UT cargada no debería haber impedimento: %q", d.Impedimento)
	}
	// Mínimo: 83,33 UT × 10 = 833,30 ≤ 1.000, así que se retiene.
	// Sustraendo: 83,33 × 10 × 3 % = 25,00. Retención: 30 − 25 = 5,00.
	casiEq(t, d.Sustraendo, 25, "sustraendo en bolívares (83,33 UT × 10 × 3%)")
	casiEq(t, oc.RetencionISLRMonto, 5, "retención (1.000 × 3% − 25)")
}
