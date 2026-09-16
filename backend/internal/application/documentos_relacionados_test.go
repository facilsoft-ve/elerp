package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
)

/* La traza de correcciones de un documento, en los DOS sentidos.
 *
 * Notas de la contadora: la factura tiene que mostrar lo que salió de ella, y la
 * nota tiene que mostrar de qué factura viene. El vínculo ya existía en el dato
 * (`RefDocumentoID`); lo que faltaba era poder recorrerlo. Sin esto, saber si una
 * factura tenía nota de crédito obligaba a buscarla a mano en la lista — que es
 * justo lo que hace que nadie lo revise. */

// LA prueba del sentido «hacia adelante»: desde la factura se ve lo que salió.
func TestDocumentosRelacionados_LaFacturaMuestraSusNotas(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 3)

	nc, err := svc.EmitirNotaCredito(empDemo, sede1, actorA, origenTst, doc.ID, "devolución parcial",
		[]application.LineaEntrada{{SKU: doc.Lineas[0].SKU, Cantidad: 1}})
	if err != nil {
		t.Fatalf("nota de crédito: %v", err)
	}

	rel, ok := svc.DocumentosRelacionados(empDemo, doc.ID)
	if !ok {
		t.Fatal("la factura debería existir")
	}
	if rel.Origen != nil {
		t.Errorf("una factura no viene de ningún documento, dice venir de %+v", rel.Origen)
	}
	if len(rel.Derivados) != 1 || rel.Derivados[0].ID != nc.ID {
		t.Fatalf("la factura debería listar su nota de crédito, listó %+v", rel.Derivados)
	}
	if rel.Derivados[0].Tipo != "nota_credito" || rel.Derivados[0].Motivo != "devolución parcial" {
		t.Errorf("el vínculo debe traer tipo y motivo para leerse sin abrirlo: %+v", rel.Derivados[0])
	}
}

// Y el sentido «hacia atrás»: desde la nota se llega a su factura. Es el que
// pidió la contadora aparte, y el que permite navegar de ida y vuelta.
func TestDocumentosRelacionados_LaNotaApuntaASuFactura(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 3)
	nc, err := svc.EmitirNotaCredito(empDemo, sede1, actorA, origenTst, doc.ID, "devolución",
		[]application.LineaEntrada{{SKU: doc.Lineas[0].SKU, Cantidad: 1}})
	if err != nil {
		t.Fatalf("nota de crédito: %v", err)
	}

	rel, ok := svc.DocumentosRelacionados(empDemo, nc.ID)
	if !ok {
		t.Fatal("la nota debería existir")
	}
	if rel.Origen == nil || rel.Origen.ID != doc.ID {
		t.Fatalf("la nota debería apuntar a su factura, apunta a %+v", rel.Origen)
	}
	if rel.Origen.NumeroCompleto != doc.NumeroCompleto {
		t.Errorf("el vínculo debe traer el número para poder mostrarlo: %q", rel.Origen.NumeroCompleto)
	}
	if len(rel.Derivados) != 0 {
		t.Errorf("de una nota de crédito no sale nada más: %+v", rel.Derivados)
	}
}

// Una anulación también es un derivado: es la corrección más importante de
// mostrar en la factura, porque la deja sin efecto.
func TestDocumentosRelacionados_LaAnulacionApareceComoDerivada(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 1)
	anu, err := svc.AnularDocumento(empDemo, sede1, actorA, origenTst, doc.ID, "error de carga")
	if err != nil {
		t.Fatalf("anular: %v", err)
	}

	rel, _ := svc.DocumentosRelacionados(empDemo, doc.ID)
	if len(rel.Derivados) != 1 || rel.Derivados[0].ID != anu.ID || rel.Derivados[0].Tipo != "anulacion" {
		t.Fatalf("la anulación debería figurar como derivada: %+v", rel.Derivados)
	}
	// Y desde la anulación se vuelve a la factura.
	relAnu, _ := svc.DocumentosRelacionados(empDemo, anu.ID)
	if relAnu.Origen == nil || relAnu.Origen.ID != doc.ID {
		t.Errorf("la anulación debería apuntar a la factura anulada: %+v", relAnu.Origen)
	}
}

// Las retenciones van APARTE de los derivados: no son documentos fiscales de
// venta (no llevan número de control ni entran al libro como una nota), y
// mezclarlas confundiría la lectura.
func TestDocumentosRelacionados_LasRetencionesVanAparte(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 3)

	if _, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: "iva", NumeroComprobante: "20260900001234", Fecha: "2026-09-16", Porcentaje: 75,
	}); err != nil {
		t.Fatalf("registrar retención: %v", err)
	}

	rel, _ := svc.DocumentosRelacionados(empDemo, doc.ID)
	if len(rel.Derivados) != 0 {
		t.Errorf("una retención NO es un documento derivado: %+v", rel.Derivados)
	}
	if len(rel.Retenciones) != 1 {
		t.Fatalf("la factura debería listar su retención, listó %+v", rel.Retenciones)
	}
	r := rel.Retenciones[0]
	if r.NumeroComprobante != "20260900001234" || r.Impuesto != "iva" || r.MontoRetenido <= 0 {
		t.Errorf("el vínculo de retención llegó incompleto: %+v", r)
	}
}

// Un documento sin correcciones responde con listas vacías, no con error: la
// vista tiene que poder pintar «sin documentos relacionados» sin tratarlo como
// una falla.
func TestDocumentosRelacionados_SinCorreccionesDevuelveVacio(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 1)

	rel, ok := svc.DocumentosRelacionados(empDemo, doc.ID)
	if !ok {
		t.Fatal("el documento existe")
	}
	if rel.Origen != nil || len(rel.Derivados) != 0 || len(rel.Retenciones) != 0 {
		t.Errorf("sin correcciones todo debería venir vacío: %+v", rel)
	}
}

// El aislamiento por tenant es la última línea: otra empresa no puede leer la
// traza de un documento ajeno.
func TestDocumentosRelacionados_AisladoPorEmpresa(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 1)

	if _, ok := svc.DocumentosRelacionados("emp_otra", doc.ID); ok {
		t.Error("otra empresa no debería poder leer la traza de este documento")
	}
	if _, ok := svc.DocumentosRelacionados(empDemo, "no-existe"); ok {
		t.Error("un documento inexistente no puede resolver")
	}
}
