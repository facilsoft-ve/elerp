package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
)

// provDemo1 es el proveedor sembrado por el seed (Distribuidora Polar).
const provDemo1 = "prov_demo_1"

// ocRecibida crea una orden de compra de una línea, la confirma y la recibe por
// completo. Devuelve la orden ya en estado recibida.
func ocRecibida(t *testing.T, svc *application.Service, sku string, cant, costo float64) compra.OrdenCompra {
	t.Helper()
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: cant, CostoUnitario: costo}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar OC: %v", err)
	}
	out, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: cant}})
	if err != nil {
		t.Fatalf("recibir OC: %v", err)
	}
	return out
}

// ocFacturada recibe una orden (10 u a Bs 20) y registra su factura de compra.
func ocFacturada(t *testing.T, svc *application.Service, sku, numFactura, numControl, fecha string) compra.FacturaCompra {
	t.Helper()
	oc := ocRecibida(t, svc, sku, 10, 20)
	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: numFactura, NumeroControl: numControl, Fecha: fecha,
	})
	if err != nil {
		t.Fatalf("registrar factura de compra: %v", err)
	}
	return fc
}

// TestRegistrarFacturaCompra_AsientaIVACredito comprueba que registrar la factura
// del proveedor crea la factura con sus montos derivados y asienta SOLO el IVA
// crédito fiscal (Debe 1103 / Haber 2101), sin volver a asentar la base (que ya
// entró en CxP con la recepción).
func TestRegistrarFacturaCompra_AsientaIVACredito(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc := ocRecibida(t, svc, sku, 10, 20) // base gravada 200

	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-000123", NumeroControl: "00-00012345", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("registrar factura: %v", err)
	}
	// La base debe coincidir con lo recibido y el IVA con base*alícuota.
	if !casi(fc.BaseImponible, 200) {
		t.Fatalf("base imponible esperada 200, fue %v", fc.BaseImponible)
	}
	if fc.IVA <= 0 {
		t.Fatalf("el IVA crédito debería ser > 0, fue %v", fc.IVA)
	}
	if !casi(fc.Total, fc.BaseImponible+fc.BaseExenta+fc.IVA) {
		t.Fatalf("el total no cuadra: total=%v base=%v exenta=%v iva=%v", fc.Total, fc.BaseImponible, fc.BaseExenta, fc.IVA)
	}
	if fc.ProveedorRIF != "J-00012345-4" {
		t.Fatalf("el RIF del proveedor no se copió a la factura: %q", fc.ProveedorRIF)
	}

	// Debe existir un asiento con RefTipo factura_compra que carga 1103 y abona 2101
	// por el IVA (y nada más: la base ya se asentó en la recepción).
	var asientos []contabilidad.Asiento
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefTipo == "factura_compra" && a.RefID == fc.ID {
			asientos = append(asientos, a)
		}
	}
	if len(asientos) != 1 {
		t.Fatalf("se esperaba 1 asiento de la factura, hay %d", len(asientos))
	}
	a := asientos[0]
	var debeIVACredito, haberCxP float64
	for _, l := range a.Lineas {
		switch l.Codigo {
		case contabilidad.CtaIVACreditoFiscal:
			debeIVACredito += l.Debe
		case contabilidad.CtaCuentasPorPagar:
			haberCxP += l.Haber
		}
	}
	if !casi(debeIVACredito, fc.IVA) || !casi(haberCxP, fc.IVA) {
		t.Fatalf("el asiento del IVA crédito no cuadra: debe 1103=%v haber 2101=%v iva=%v", debeIVACredito, haberCxP, fc.IVA)
	}
	if !a.Cuadra() {
		t.Fatalf("el asiento de la factura de compra no cuadra: %+v", a)
	}
}

// TestRegistrarFacturaCompra_DifiereDeRecepcion comprueba el hallazgo cerrado: la
// factura del proveedor puede traer SUS PROPIAS líneas/importes, distintos de lo
// recibido. Se recibe 10 u a Bs 20 (neto 200) y el proveedor factura 10 u a Bs 22
// (base 220). Se verifica que: la factura queda con su base propia (220) y su
// diferencia (+20); CxP refleja el TOTAL de la factura (220 + IVA); el asiento de
// la factura cuadra y contiene el IVA crédito y la diferencia en compras (5202); y
// el inventario NO se re-valúa (el Kardex conserva el costo recibido de 20).
func TestRegistrarFacturaCompra_DifiereDeRecepcion(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc := ocRecibida(t, svc, sku, 10, 20) // recibido neto 200 al costo 20
	// Costo del Kardex ANTES de facturar: la factura no debe re-valuarlo.
	_, costoAntes := existenciaDe(t, svc, empDemo, sede1, sku)

	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-DIF-1", NumeroControl: "00-DIF-1", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
		Lineas: []application.LineaFacturaCompraEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 22}}, // base 220
	})
	if err != nil {
		t.Fatalf("registrar factura que difiere: %v", err)
	}

	// La factura lleva su base propia (220), la referencia recibida (200) y la
	// diferencia (+20), no lo recibido.
	if !casi(fc.BaseImponible, 220) {
		t.Fatalf("la base de la factura debía ser 220 (línea propia), fue %v", fc.BaseImponible)
	}
	if !casi(fc.BaseRecibida, 200) || !casi(fc.DiferenciaBase, 20) {
		t.Fatalf("referencia recibida 200 y diferencia +20 esperadas, fueron recibida=%v dif=%v", fc.BaseRecibida, fc.DiferenciaBase)
	}
	if !casi(fc.Total, fc.BaseImponible+fc.BaseExenta+fc.IVA) {
		t.Fatalf("el total no cuadra: total=%v base=%v exenta=%v iva=%v", fc.Total, fc.BaseImponible, fc.BaseExenta, fc.IVA)
	}

	// CxP refleja el TOTAL de la factura (base 220 + IVA), no el neto recibido.
	deuda := deudaDeOrden(t, svc, oc.ID)
	if !casi(deuda.Recibido, fc.Total) {
		t.Fatalf("CxP debía asentar el total de la factura (%v), fue %v", fc.Total, deuda.Recibido)
	}

	// El asiento de la factura cuadra y contiene IVA crédito, diferencia (5202) y CxP.
	var a contabilidad.Asiento
	encontrado := false
	for _, x := range svc.LibroDiario(empDemo) {
		if x.RefTipo == "factura_compra" && x.RefID == fc.ID {
			a, encontrado = x, true
			break
		}
	}
	if !encontrado {
		t.Fatalf("no se encontró el asiento de la factura de compra")
	}
	if !a.Cuadra() {
		t.Fatalf("el asiento de la factura que difiere no cuadra: %+v", a)
	}
	var debeIVA, debeDif, haberCxP float64
	for _, l := range a.Lineas {
		switch l.Codigo {
		case contabilidad.CtaIVACreditoFiscal:
			debeIVA += l.Debe
		case contabilidad.CtaDiferenciaEnCompras:
			debeDif += l.Debe
		case contabilidad.CtaCuentasPorPagar:
			haberCxP += l.Haber
		}
	}
	if !casi(debeIVA, fc.IVA) {
		t.Fatalf("el IVA crédito del asiento (%v) no es el de la factura (%v)", debeIVA, fc.IVA)
	}
	if !casi(debeDif, 20) {
		t.Fatalf("la diferencia en compras (5202) debía ser 20 al Debe, fue %v", debeDif)
	}
	if !casi(haberCxP, fc.IVA+20) {
		t.Fatalf("CxP del asiento debía ser IVA + diferencia (%v), fue %v", fc.IVA+20, haberCxP)
	}

	// El inventario NO se re-valúa: el costo del Kardex es idéntico al de antes de
	// facturar (la diferencia de precio de la factura no toca el ledger de stock).
	_, costoDespues := existenciaDe(t, svc, empDemo, sede1, sku)
	if !casi(costoDespues, costoAntes) {
		t.Fatalf("el costo del Kardex no debía cambiar al facturar (%v → %v)", costoAntes, costoDespues)
	}

	// El libro entero cuadra tras la operación.
	if b := svc.Balance(empDemo); !b.Cuadra {
		t.Fatalf("el libro no cuadra tras facturar con diferencia: Debe %v vs Haber %v", b.TotalDebe, b.TotalHaber)
	}
}

// TestRegistrarFacturaCompra_Duplicada verifica que una orden no se factura dos veces.
func TestRegistrarFacturaCompra_Duplicada(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc := ocRecibida(t, svc, sku, 10, 20)
	if _, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-1", NumeroControl: "00-1", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("primera factura: %v", err)
	}
	_, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-2", NumeroControl: "00-2", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if !errors.Is(err, application.ErrFacturaCompraDuplicada) {
		t.Fatalf("se esperaba ErrFacturaCompraDuplicada, se obtuvo: %v", err)
	}
}

// TestRegistrarFacturaCompra_DatosFaltantes exige nº de factura y nº de control.
func TestRegistrarFacturaCompra_DatosFaltantes(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc := ocRecibida(t, svc, sku, 10, 20)
	_, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-1", NumeroControl: "", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if !errors.Is(err, application.ErrDatosFacturaCompra) {
		t.Fatalf("sin nº de control se esperaba ErrDatosFacturaCompra, se obtuvo: %v", err)
	}
}

// TestRegistrarFacturaCompra_OCNoRecibida rechaza facturar una orden confirmada
// (sin mercancía recibida todavía).
func TestRegistrarFacturaCompra_OCNoRecibida(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 4, CostoUnitario: 10}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar OC: %v", err)
	}
	_, err = svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-1", NumeroControl: "00-1", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if !errors.Is(err, application.ErrOCEstado) {
		t.Fatalf("se esperaba ErrOCEstado, se obtuvo: %v", err)
	}
}

// TestCuentasPorPagar_ReflejaBaseMasIVA comprueba que, tras facturar, la deuda de
// la orden en CxP pasa a base + IVA (Total de la factura) y queda marcada como facturada.
func TestCuentasPorPagar_ReflejaBaseMasIVA(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc := ocRecibida(t, svc, sku, 10, 20) // neto recibido 200

	// Antes de facturar: la orden adeuda el neto (200).
	antes := deudaDeOrden(t, svc, oc.ID)
	if !casi(antes.Recibido, 200) || antes.Facturada {
		t.Fatalf("antes de facturar debía adeudar el neto sin marca de facturada: %+v", antes)
	}

	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-9", NumeroControl: "00-9", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("registrar factura: %v", err)
	}

	despues := deudaDeOrden(t, svc, oc.ID)
	if !despues.Facturada {
		t.Fatalf("tras facturar la orden debería marcarse como facturada: %+v", despues)
	}
	if !casi(despues.Recibido, fc.Total) {
		t.Fatalf("tras facturar la deuda debía ser el total de la factura (%v), fue %v", fc.Total, despues.Recibido)
	}
	if despues.Recibido <= antes.Recibido {
		t.Fatalf("la deuda facturada (%v) debería superar la neta (%v) por el IVA", despues.Recibido, antes.Recibido)
	}
}

// deudaDeOrden localiza la fila de CxP de una orden concreta.
func deudaDeOrden(t *testing.T, svc *application.Service, ordenID string) application.CuentaPorPagar {
	t.Helper()
	res := svc.CuentasPorPagar(empDemo)
	for _, o := range res.Ordenes {
		if o.OrdenID == ordenID {
			return o
		}
	}
	t.Fatalf("la orden %s no aparece en cuentas por pagar", ordenID)
	return application.CuentaPorPagar{}
}
