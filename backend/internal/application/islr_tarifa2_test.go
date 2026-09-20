package application_test

import (
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

/* TARIFA 2: LA ESCALA DE LOS NO DOMICILIADOS.
 *
 * A un proveedor del exterior no se le retiene un porcentaje fijo sino una escala
 * —15 %, 22 %, 34 %— y el tramo lo decide cuánto se le lleva pagado EN EL AÑO por
 * ese concepto, no lo que dice la factura de hoy. Además, no todo el pago forma la
 * base: en honorarios no mercantiles es el 90 %.
 *
 * Sin acumular, cada factura se mira aislada, todas caen en el primer tramo y se
 * retiene de menos todo el ejercicio. El error crece con el volumen y no da
 * ninguna señal: es exactamente el tipo de fallo que este módulo persigue.
 *
 * Los valores son los de la escala de la ley, los mismos que trae nuestra
 * localización de Odoo. La UT de estas pruebas vale 1 para poder seguir las
 * cuentas a mano: así «1.000 UT acumuladas» son Bs 1.000. */

const (
	codExterior = "honorarios_exterior"
	pjnd        = fiscal.SujetoJuridicaNoDomiciliada
)

// escalaExterior carga los tres tramos de la Tarifa 2 sobre el 90 % del pago.
func escalaExterior(t *testing.T, svc *application.Service) {
	t.Helper()
	tramos := []struct {
		desde float64
		pct   float64
	}{{0, 15}, {2000.01, 22}, {3000.01, 34}}
	for _, tr := range tramos {
		if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, fiscal.ConceptoISLR{
			Codigo: codExterior, Nombre: "Honorarios profesionales no mercantiles",
			Sujeto: pjnd, Porcentaje: tr.pct,
			PorcentajeBase: 90, DesdeAcumuladoUT: tr.desde, Activo: true,
		}); err != nil {
			t.Fatalf("cargar tramo desde %v UT: %v", tr.desde, err)
		}
	}
}

// ahoraFechaTest es el día de hoy en AAAA-MM-DD, que es el ejercicio en el que
// caen las retenciones que emiten estas pruebas.
func ahoraFechaTest() string { return time.Now().UTC().Format("2006-01-02") }

// acumular emite una retención REAL de ISLR al proveedor por una base dada,
// pasando por todo el camino —orden, recepción, factura, comprobante—.
//
// No inserta el comprobante a mano a propósito: si la emisión dejara de guardar
// la base en UT, el tercero o el código del concepto, el acumulado quedaría en
// cero y un atajo en el test no lo vería. El camino real es la prueba.
func acumular(t *testing.T, svc *application.Service, provID string, base float64) {
	t.Helper()
	sku := primerSKU(t, svc)
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provID, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 1, CostoUnitario: base}},
	})
	if err != nil {
		t.Fatalf("crear OC para acumular: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 1}}); err != nil {
		t.Fatalf("recibir: %v", err)
	}
	// Número de factura y de control únicos por llamada: el correlativo de la orden
	// sirve de sufijo y no se repite.
	suf := oc.NumeroCompleto
	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-" + suf, NumeroControl: "00-" + suf, Fecha: ahoraFechaTest(),
	})
	if err != nil {
		t.Fatalf("facturar: %v", err)
	}
	// La base se manda SIN el 90 %: la porción gravable la aplica el servidor, que
	// es justo una de las cosas que estas pruebas comprueban.
	if _, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc.ID, application.EntradaRetencion{
		Impuesto: "islr", Fecha: ahoraFechaTest(), Base: base / 0.90,
		ConceptoCodigo: codExterior, Sujeto: pjnd,
	}); err != nil {
		t.Fatalf("emitir retención: %v", err)
	}
}

// TestTarifa2_ElTramoLoDecideElAcumuladoNoLaFactura.
func TestTarifa2_ElTramoLoDecideElAcumuladoNoLaFactura(t *testing.T) {
	svc := servicioConConceptos(t) // UT = 1
	escalaExterior(t, svc)

	casos := []struct {
		pago      float64
		acumulado float64
		pct       float64
		por       string
	}{
		{1000, 0, 15, "primer pago pequeño: primer tramo"},
		{1000, 1500, 22, "con 1.500 UT ya acumuladas, este pago cruza a 22 %"},
		{1000, 2500, 34, "con 2.500 acumuladas, cruza al último tramo"},
		{5000, 0, 34, "un pago grande solo también llega al último tramo"},
	}
	for _, c := range casos {
		// La base gravable es el 90 % del pago; el acumulado se expresa en UT (=Bs).
		got, ok := fiscal.ConceptoPara(svc.ConceptosISLR(empDemo), codExterior, pjnd,
			c.acumulado+c.pago*0.90)
		if !ok {
			t.Fatalf("%s: no resolvió tramo", c.por)
		}
		if got.Porcentaje != c.pct {
			t.Errorf("%s — se esperaba %v %%, dio %v %%", c.por, c.pct, got.Porcentaje)
		}
	}
}

// TestTarifa2_NoTodoElPagoEsBaseGravable: en honorarios al exterior se retiene
// sobre el 90 %. Aplicar la tarifa al total retiene de más.
func TestTarifa2_NoTodoElPagoEsBaseGravable(t *testing.T) {
	svc := servicioConConceptos(t)
	escalaExterior(t, svc)

	c, ok := fiscal.ConceptoPara(svc.ConceptosISLR(empDemo), codExterior, pjnd, 0)
	if !ok {
		t.Fatal("no resolvió el concepto")
	}
	casiEq(t, c.BaseGravable(1000), 900, "la base gravable es el 90 % del pago")
	// 900 × 15 % = 135. Sobre el total serían 150: 15 de más en cada factura.
	casiEq(t, c.Retener(1000, 1), 135, "se retiene sobre la base gravable, no sobre el pago")
}

// TestTarifa2_LoRetenidoAntesEmpujaElTramo es la prueba de que el acumulado es
// REAL: se emiten dos retenciones y la tercera orden ya proyecta el tramo alto.
func TestTarifa2_LoRetenidoAntesEmpujaElTramo(t *testing.T) {
	svc := servicioConConceptos(t)
	escalaExterior(t, svc)
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "SRV-EXT", Nombre: "Consultoría del exterior", Precio: 100, ConceptoISLR: codExterior,
	}); err != nil {
		t.Fatalf("crear servicio: %v", err)
	}
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Consultora del Exterior LLC", RetieneISLR: true,
		ConceptoISLRCodigo: codExterior, SujetoISLR: pjnd,
	})

	// Antes de retener nada: base 1.000 → gravable 900 → primer tramo, 15 %.
	oc := ocConLineas(t, svc, prov.ID, "SRV-EXT")
	if len(oc.RetencionISLRDetalle) != 1 {
		t.Fatalf("desglose inesperado: %+v", oc.RetencionISLRDetalle)
	}
	casiEq(t, oc.RetencionISLRDetalle[0].Porcentaje, 15, "sin acumulado, primer tramo")
	casiEq(t, oc.RetencionISLRDetalle[0].Base, 900, "la base informada es la gravable")
	casiEq(t, oc.RetencionISLRMonto, 135, "900 × 15 %")

	// Se le retienen 2.500 UT en el ejercicio (dos comprobantes).
	acumular(t, svc, prov.ID, 1500)
	acumular(t, svc, prov.ID, 1000)
	if got := svc.AcumuladoISLRUT(empDemo, prov.ID, codExterior, ahoraFechaTest(), ""); !casi(got, 2500) {
		t.Fatalf("acumulado del ejercicio = %v, se esperaban 2.500 UT", got)
	}

	// La MISMA orden, ahora: 2.500 + 900 = 3.400 ⇒ último tramo, 34 %.
	oc2 := ocConLineas(t, svc, prov.ID, "SRV-EXT")
	casiEq(t, oc2.RetencionISLRDetalle[0].Porcentaje, 34, "con 2.500 UT acumuladas, último tramo")
	casiEq(t, oc2.RetencionISLRMonto, 306, "900 × 34 %")
}

// TestTarifa2_ElAcumuladoNoCruzaProveedoresNiConceptos: acumular de más empuja al
// tramo alto a quien no le toca, y eso retiene de más sin que nadie lo note.
func TestTarifa2_ElAcumuladoNoCruzaProveedoresNiConceptos(t *testing.T) {
	svc := servicioConConceptos(t)
	escalaExterior(t, svc)
	uno := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Exterior Uno LLC", RetieneISLR: true,
		ConceptoISLRCodigo: codExterior, SujetoISLR: pjnd,
	})
	otro := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Exterior Dos LLC", RetieneISLR: true,
		ConceptoISLRCodigo: codExterior, SujetoISLR: pjnd,
	})
	acumular(t, svc, uno.ID, 2500)

	hoy := ahoraFechaTest()
	if got := svc.AcumuladoISLRUT(empDemo, otro.ID, codExterior, hoy, ""); got != 0 {
		t.Errorf("lo retenido a un proveedor no acumula en otro: dio %v", got)
	}
	if got := svc.AcumuladoISLRUT(empDemo, uno.ID, "honorarios", hoy, ""); got != 0 {
		t.Errorf("lo retenido por un concepto no acumula en otro: dio %v", got)
	}
	// Y el ejercicio acota: lo del año pasado no empuja el tramo de este.
	if got := svc.AcumuladoISLRUT(empDemo, uno.ID, codExterior, "2019-06-01", ""); got != 0 {
		t.Errorf("lo de otro ejercicio no debería acumular: dio %v", got)
	}
}

// TestTarifa2_FechaVaciaEsHoy: el endpoint que consulta el acumulado recibe la
// fecha por query y la pantalla la manda vacía.
//
// Lo cazó una prueba en el navegador: la orden proyectaba el tramo alto y la
// consulta del acumulado devolvía 0. Ninguna de las dos fallaba — simplemente se
// contradecían, que es la forma más cara de equivocarse acá.
func TestTarifa2_FechaVaciaEsHoy(t *testing.T) {
	svc := servicioConConceptos(t)
	escalaExterior(t, svc)
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Exterior LLC", RetieneISLR: true,
		ConceptoISLRCodigo: codExterior, SujetoISLR: pjnd,
	})
	acumular(t, svc, prov.ID, 1500)

	conFecha := svc.AcumuladoISLRUT(empDemo, prov.ID, codExterior, ahoraFechaTest(), "")
	sinFecha := svc.AcumuladoISLRUT(empDemo, prov.ID, codExterior, "", "")
	if !casi(conFecha, 1500) {
		t.Fatalf("con fecha explícita: %v, se esperaban 1.500 UT", conFecha)
	}
	if !casi(sinFecha, conFecha) {
		t.Errorf("sin fecha tiene que dar lo mismo que hoy: %v vs %v", sinFecha, conFecha)
	}
	// Y el mapa por concepto, que es lo que consume la pantalla.
	if got := svc.AcumuladosISLRUTDe(empDemo, prov.ID, "")[codExterior]; !casi(got, 1500) {
		t.Errorf("el mapa por concepto con fecha vacía dio %v", got)
	}
}
