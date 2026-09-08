package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
)

// notaEntradaValida arma una entrada de nota de compra con los datos mínimos que
// no fallan la validación (concepto + nº documento + nº control).
func notaEntradaValida(monto float64) application.NotaCompraEntrada {
	return application.NotaCompraEntrada{
		Concepto:        "ajuste de compra",
		Monto:           monto,
		NumeroDocumento: "NC-000001",
		NumeroControl:   "00-00099999",
		Fecha:           time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func TestNotaCreditoCompra_GuardaNegativoYEsInmutable(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-NC-1", "00-00011111", time.Now().UTC().Format(time.RFC3339Nano))

	nc, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, notaEntradaValida(50))
	if err != nil {
		t.Fatalf("emitir NC de compra: %v", err)
	}
	if nc.Tipo != compra.NotaCreditoCompra {
		t.Errorf("el tipo debe ser nota de crédito de compra, se obtuvo %q", nc.Tipo)
	}
	// La NC guarda los montos en NEGATIVO (baja la deuda), espejo de la NC de cliente.
	if nc.Total >= 0 || nc.BaseImponible >= 0 || nc.IVA >= 0 {
		t.Errorf("la NC de compra debe guardar montos negativos, se obtuvo total=%v base=%v iva=%v",
			nc.Total, nc.BaseImponible, nc.IVA)
	}
	// El total cuadra: |total| = |base| + |iva|.
	if !casi(-nc.Total, -nc.BaseImponible-nc.IVA) {
		t.Errorf("|total| debe ser |base|+|IVA|: total=%v base=%v iva=%v", nc.Total, nc.BaseImponible, nc.IVA)
	}
	// Append-only: recuperarla por id devuelve exactamente lo emitido.
	got, ok := svc.NotaCompra(empDemo, nc.ID)
	if !ok || got.Total != nc.Total || got.NumeroCompleto != nc.NumeroCompleto {
		t.Errorf("la nota debe recuperarse íntegra por id, se obtuvo %+v", got)
	}
	if len(svc.NotasCompra(empDemo)) == 0 {
		t.Error("la nota debe aparecer en el listado de la empresa")
	}
}

func TestNotaDebitoCompra_GuardaPositivoYNoTieneTope(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-ND-1", "00-00022222", time.Now().UTC().Format(time.RFC3339Nano))

	// La ND (cargo adicional: flete, interés) SUBE la deuda y no tiene tope: puede
	// exceder el total de la factura sin ser rechazada.
	nd, err := svc.EmitirNotaDebitoCompra(empDemo, actorA, origenTst, fc.ID, notaEntradaValida(fc.Total+500))
	if err != nil {
		t.Fatalf("emitir ND de compra: %v", err)
	}
	if nd.Tipo != compra.NotaDebitoCompra {
		t.Errorf("el tipo debe ser nota de débito de compra, se obtuvo %q", nd.Tipo)
	}
	if nd.Total <= 0 {
		t.Errorf("la ND de compra debe guardar montos positivos, se obtuvo total=%v", nd.Total)
	}
}

func TestNotaCompra_ConceptoObligatorio(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-NC-2", "00-00033333", time.Now().UTC().Format(time.RFC3339Nano))
	in := notaEntradaValida(10)
	in.Concepto = "   "
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, in); !errors.Is(err, application.ErrNotaCompraConcepto) {
		t.Fatalf("sin concepto debe dar ErrNotaCompraConcepto, se obtuvo: %v", err)
	}
}

func TestNotaCompra_DatosDelDocumentoObligatorios(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-NC-3", "00-00044444", time.Now().UTC().Format(time.RFC3339Nano))
	in := notaEntradaValida(10)
	in.NumeroControl = ""
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, in); !errors.Is(err, application.ErrDatosNotaCompra) {
		t.Fatalf("sin número de control debe dar ErrDatosNotaCompra, se obtuvo: %v", err)
	}
}

func TestNotaCompra_MontoCeroEsVacia(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-NC-4", "00-00055555", time.Now().UTC().Format(time.RFC3339Nano))
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, notaEntradaValida(0)); !errors.Is(err, application.ErrNotaCompraVacia) {
		t.Fatalf("monto 0 debe dar ErrNotaCompraVacia, se obtuvo: %v", err)
	}
}

func TestNotaCompra_FacturaInexistente(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, "fc_fantasma", notaEntradaValida(10)); !errors.Is(err, application.ErrFacturaCompraNoExiste) {
		t.Fatalf("una factura de compra inexistente debe dar ErrFacturaCompraNoExiste, se obtuvo: %v", err)
	}
}
