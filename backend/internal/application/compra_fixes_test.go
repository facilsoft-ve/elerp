package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/contabilidad"
)

// TestCrearOrdenCompra_RechazaCombo verifica FIX 1: un combo no se compra (se
// compran sus componentes). Crear una OC con el SKU de un combo debe rechazarse.
func TestCrearOrdenCompra_RechazaCombo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: comboSKU, Cantidad: 1, CostoUnitario: 100}},
	})
	if !errors.Is(err, application.ErrComboEnCompra) {
		t.Fatalf("comprar un combo debe dar ErrComboEnCompra, se obtuvo: %v", err)
	}
}

// TestCrearSolicitud_RechazaCombo verifica FIX 1 en el RFQ: no se pide presupuesto
// de un combo.
func TestCrearSolicitud_RechazaCombo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, err := svc.CrearSolicitud(empDemo, sede1, actorA, origenTst, application.EntradaSolicitud{
		SedeID: sede1,
		Lineas: []application.LineaSolicitudEntrada{{SKU: comboSKU, Cantidad: 2}},
	})
	if !errors.Is(err, application.ErrComboEnCompra) {
		t.Fatalf("solicitar presupuesto de un combo debe dar ErrComboEnCompra, se obtuvo: %v", err)
	}
}

// TestCrearOrdenCompra_FusionaSKUDuplicado verifica FIX 3: el mismo SKU repetido
// con el MISMO costo se fusiona en una sola línea (cantidades sumadas) y la orden
// se puede recibir completa.
func TestCrearOrdenCompra_FusionaSKUDuplicado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{
			{SKU: sku, Cantidad: 3, CostoUnitario: 10},
			{SKU: sku, Cantidad: 7, CostoUnitario: 10},
		},
	})
	if err != nil {
		t.Fatalf("crear OC con SKU duplicado (mismo costo) debía fusionar, se obtuvo: %v", err)
	}
	if len(oc.Lineas) != 1 {
		t.Fatalf("se esperaba 1 línea fusionada, hay %d", len(oc.Lineas))
	}
	if !casi(oc.Lineas[0].Cantidad, 10) {
		t.Fatalf("la cantidad fusionada debía ser 10, fue %v", oc.Lineas[0].Cantidad)
	}
	// La orden fusionada se recibe completa sin colapsar líneas.
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	out, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 10}})
	if err != nil {
		t.Fatalf("recibir la OC fusionada: %v", err)
	}
	if out.Estado != "recibida" {
		t.Fatalf("la OC fusionada debía quedar recibida, quedó %q", out.Estado)
	}
}

// TestCrearOrdenCompra_RechazaSKUDuplicadoConCostoDistinto verifica FIX 3: el mismo
// SKU con costos distintos no se puede fusionar → rechazo.
func TestCrearOrdenCompra_RechazaSKUDuplicadoConCostoDistinto(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	_, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{
			{SKU: sku, Cantidad: 3, CostoUnitario: 10},
			{SKU: sku, Cantidad: 7, CostoUnitario: 12},
		},
	})
	if !errors.Is(err, application.ErrSKUduplicado) {
		t.Fatalf("SKU duplicado con costo distinto debía dar ErrSKUduplicado, se obtuvo: %v", err)
	}
}

// TestRecibirOrdenCompra_BloqueadoTrasFacturar verifica FIX 2: tras registrar la
// factura, la orden no admite más recepciones (la factura cerró la deuda). Además
// comprueba que la CxP proyectada de la orden == el neto de CxP (2101) asentado
// en el diario para esa orden (asiento ⇄ proyección coherentes).
func TestRecibirOrdenCompra_BloqueadoTrasFacturar(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 20}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	// Recepción parcial: 4 de 10.
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 4}}); err != nil {
		t.Fatalf("recibir parcial: %v", err)
	}
	// Se factura (prefill = lo recibido: 4 @ 20).
	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-FIX2", NumeroControl: "00-FIX2", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("registrar factura: %v", err)
	}
	// Recibir MÁS tras facturar debe rechazarse.
	_, err = svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 6}})
	if !errors.Is(err, application.ErrOrdenYaFacturada) {
		t.Fatalf("recibir tras facturar debía dar ErrOrdenYaFacturada, se obtuvo: %v", err)
	}

	// CxP proyectada de la orden.
	deuda := deudaDeOrden(t, svc, oc.ID)
	if !casi(deuda.Recibido, fc.Total) {
		t.Fatalf("CxP proyectada (%v) debía ser el total de la factura (%v)", deuda.Recibido, fc.Total)
	}
	// Neto de CxP (2101) en el diario para los asientos de esta orden: la recepción
	// (RefID=oc.ID) y la factura (RefID=fc.ID). Haber − Debe.
	var netoCxP float64
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefID != oc.ID && a.RefID != fc.ID {
			continue
		}
		for _, l := range a.Lineas {
			if l.Codigo == contabilidad.CtaCuentasPorPagar {
				netoCxP += l.Haber - l.Debe
			}
		}
	}
	if !casi(netoCxP, deuda.Recibido) {
		t.Fatalf("el neto de CxP en el diario (%v) debe igualar la proyección por-pagar (%v)", netoCxP, deuda.Recibido)
	}
}

// TestRegistrarFacturaCompra_IVACapturado verifica FIX 4: si la factura del
// proveedor trae su propio importe de IVA, se usa ESE valor (no el recomputado) en
// la factura, el asiento y la CxP, y todo cuadra.
func TestRegistrarFacturaCompra_IVACapturado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc := ocRecibida(t, svc, sku, 10, 20) // base gravada 200

	const ivaCapturado = 25.75 // distinto del computado (200 * alícuota)
	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-IVA", NumeroControl: "00-IVA", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
		IVA: ivaCapturado,
	})
	if err != nil {
		t.Fatalf("registrar factura con IVA capturado: %v", err)
	}
	if !casi(fc.IVA, ivaCapturado) {
		t.Fatalf("la factura debía registrar el IVA capturado (%v), fue %v", ivaCapturado, fc.IVA)
	}
	if !casi(fc.Total, fc.BaseImponible+fc.BaseExenta+ivaCapturado) {
		t.Fatalf("el total debía usar el IVA capturado: total=%v base=%v exenta=%v", fc.Total, fc.BaseImponible, fc.BaseExenta)
	}

	// El asiento abona a CxP (2101) el IVA capturado (sin diferencia de base) y cuadra.
	var a contabilidad.Asiento
	encontrado := false
	for _, x := range svc.LibroDiario(empDemo) {
		if x.RefTipo == "factura_compra" && x.RefID == fc.ID {
			a, encontrado = x, true
			break
		}
	}
	if !encontrado {
		t.Fatalf("no se encontró el asiento de la factura")
	}
	if !a.Cuadra() {
		t.Fatalf("el asiento con IVA capturado no cuadra: %+v", a)
	}
	var debeIVA, haberCxP float64
	for _, l := range a.Lineas {
		switch l.Codigo {
		case contabilidad.CtaIVACreditoFiscal:
			debeIVA += l.Debe
		case contabilidad.CtaCuentasPorPagar:
			haberCxP += l.Haber
		}
	}
	if !casi(debeIVA, ivaCapturado) || !casi(haberCxP, ivaCapturado) {
		t.Fatalf("el asiento debía usar el IVA capturado (debe 1103=%v haber 2101=%v)", debeIVA, haberCxP)
	}

	// CxP proyectada = total de la factura (con el IVA capturado).
	deuda := deudaDeOrden(t, svc, oc.ID)
	if !casi(deuda.Recibido, fc.Total) {
		t.Fatalf("CxP debía reflejar el total con IVA capturado (%v), fue %v", fc.Total, deuda.Recibido)
	}
	if b := svc.Balance(empDemo); !b.Cuadra {
		t.Fatalf("el libro no cuadra tras facturar con IVA capturado: Debe %v vs Haber %v", b.TotalDebe, b.TotalHaber)
	}
}
