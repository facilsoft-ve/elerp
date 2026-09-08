package application_test

// Suite de tests de INTEGRACIÓN: ejercen flujos completos que cruzan varios
// módulos (Inventario, Fiscal, Compras, Tesorería, Contabilidad) a través del
// mismo application.Service con adaptadores inmem. El foco no es una unidad
// aislada sino los INVARIANTES que deben mantenerse de punta a punta:
//   - las proyecciones derivadas (existencias, CxC, CxP) siguen a los hechos;
//   - cada asiento contable cuadra (Σ Debe == Σ Haber) y el libro global cuadra;
//   - append-only: nada se edita ni se borra, las correcciones anexan.
//
// Reutiliza los helpers ya definidos en el paquete application_test:
//   nuevoServicio, abrirTurno, primerSKU, casi, aCredito, emitirContado,
//   ocRecibida, ventaCreditoIVA100, asientoRetencion, deudaDeProveedor,
//   saldoProveedor, deudaDeOrden, round2Test, y las constantes empDemo/sede1/…

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

/* --- Helpers de aserción compartidos por la suite de integración ------------ */

// casiEq falla si dos importes no son iguales (con la tolerancia de medio
// céntimo de casi). El mensaje describe qué invariante se rompió.
func casiEq(t *testing.T, got, want float64, msg string) {
	t.Helper()
	if !casi(got, want) {
		t.Fatalf("%s: esperado %v, se obtuvo %v", msg, want, got)
	}
}

// debeCta / haberCta suman el Debe/Haber de una cuenta dentro de un asiento.
func debeCta(a contabilidad.Asiento, codigo string) float64 {
	var s float64
	for _, l := range a.Lineas {
		if l.Codigo == codigo {
			s += l.Debe
		}
	}
	return s
}

func haberCta(a contabilidad.Asiento, codigo string) float64 {
	var s float64
	for _, l := range a.Lineas {
		if l.Codigo == codigo {
			s += l.Haber
		}
	}
	return s
}

// existenciaDe devuelve (cantidad, costoPromedio) de un SKU en una sede: la
// proyección del ledger de inventario, no un contador.
func existenciaDe(t *testing.T, svc *application.Service, empresaID, sedeID, sku string) (float64, float64) {
	t.Helper()
	for _, e := range svc.Existencias(empresaID, sedeID) {
		if e.SKU == sku {
			return e.Cantidad, e.CostoPromedio
		}
	}
	return 0, 0
}

// movsDeRef cuenta, en el Kardex de un SKU/sede, los movimientos de un tipo que
// referencian una entidad concreta y devuelve la cantidad total movida.
func movsDeRef(t *testing.T, svc *application.Service, empresaID, sedeID, sku, tipo, refID string) (int, float64) {
	t.Helper()
	kx, err := svc.Kardex(empresaID, sedeID, sku)
	if err != nil {
		t.Fatalf("kardex %s: %v", sku, err)
	}
	n := 0
	var total float64
	for _, m := range kx.Movimientos {
		if m.Tipo == tipo && m.Ref == refID {
			n++
			total += m.Cantidad
		}
	}
	return n, total
}

// asientoVentaDe localiza el asiento de la FACTURA (no el de costo, ni un
// contrario) derivado de un documento fiscal.
func asientoVentaDe(t *testing.T, svc *application.Service, empresaID string, doc fiscal.Documento) contabilidad.Asiento {
	t.Helper()
	for _, a := range svc.LibroDiario(empresaID) {
		if a.RefID == doc.ID && !a.Contrario && a.Descripcion == "Factura "+doc.NumeroCompleto+" emitida" {
			return a
		}
	}
	t.Fatalf("no se halló el asiento de venta del documento %s", doc.NumeroCompleto)
	return contabilidad.Asiento{}
}

// asientosPorRef devuelve todos los asientos que referencian una entidad.
func asientosPorRef(svc *application.Service, empresaID, refTipo, refID string) []contabilidad.Asiento {
	var out []contabilidad.Asiento
	for _, a := range svc.LibroDiario(empresaID) {
		if a.RefTipo == refTipo && a.RefID == refID {
			out = append(out, a)
		}
	}
	return out
}

// saldoContableCta suma el saldo (Σ Debe − Σ Haber) de una cuenta en TODO el
// libro diario. Sirve para cruzar la contabilidad con las proyecciones de
// Tesorería: deben coincidir (Principio 4, un solo modelo canónico, no dos
// derivaciones que se contradicen).
func saldoContableCta(svc *application.Service, empresaID, codigo string) float64 {
	s := 0.0
	for _, a := range svc.LibroDiario(empresaID) {
		for _, l := range a.Lineas {
			if l.Codigo == codigo {
				s += l.Debe - l.Haber
			}
		}
	}
	return round2Test(s)
}

// assertLibroCuadra es el guardián del principio contable: TODO asiento de la
// empresa cuadra por separado y el balance de comprobación global cuadra
// (Σ Debe == Σ Haber). Es la aserción que se repite tras cada batería de operaciones.
func assertLibroCuadra(t *testing.T, svc *application.Service, empresaID string) {
	t.Helper()
	for _, a := range svc.LibroDiario(empresaID) {
		if !a.Cuadra() {
			t.Fatalf("el asiento %s (%s) no cuadra: %+v", a.Codigo, a.Descripcion, a)
		}
	}
	b := svc.Balance(empresaID)
	if !b.Cuadra {
		t.Fatalf("el libro no cuadra: Σ Debe %v vs Σ Haber %v", b.TotalDebe, b.TotalHaber)
	}
}

/* --- 1. Venta forma libre → inventario + contabilidad + (caja | CxC) -------- */

// TestIntegracion_VentaFormaLibreContado_ImpactaInventarioYContabilidad prueba
// el cruce Inventario⇄Fiscal⇄Contabilidad de una venta de contado por el canal
// FORMA LIBRE (SinCaja): baja de existencia, movimiento `salida` anexado, asiento
// de venta que cuadra (Ventas/IVA al haber, Caja al debe) y su asiento de costo
// (CostoDeVentas/Inventario). No debe quedar por cobrar (fue de contado).
func TestIntegracion_VentaFormaLibreContado_ImpactaInventarioYContabilidad(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc) // producto gravado del seed (moneda VES)

	cantVendida := 2.0
	precio := 100.0
	cant0, costo0 := existenciaDe(t, svc, empDemo, sede1, sku)
	if cant0 < cantVendida {
		t.Fatalf("el seed debería traer stock suficiente de %s (hay %v)", sku, cant0)
	}
	total := precio * cantVendida * (1 + 0.16) // 200 + IVA 16% = 232

	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true, // canal forma libre: no exige caja abierta
		Lineas:  []application.LineaEntrada{{SKU: sku, Cantidad: cantVendida, PrecioUnitario: precio}},
		Pagos:   []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: total, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir factura de contado: %v", err)
	}
	casiEq(t, doc.Total, total, "el total de la factura")

	// (a) Inventario: la existencia baja EXACTAMENTE por lo vendido y quedó un
	// movimiento `salida` (negativo) referenciando el documento.
	cant1, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	casiEq(t, cant0-cant1, cantVendida, "la existencia debe bajar por la cantidad vendida")
	n, movido := movsDeRef(t, svc, empDemo, sede1, sku, inventario.MovSalida, doc.ID)
	if n != 1 {
		t.Fatalf("debía anexarse 1 movimiento de salida por la venta, hubo %d", n)
	}
	casiEq(t, movido, -cantVendida, "el movimiento de salida es negativo por la cantidad")

	// (b) Contabilidad: el asiento de la venta cuadra e impacta las cuentas
	// correctas. De contado → todo al debe de Caja/Bancos, nada por cobrar.
	a := asientoVentaDe(t, svc, empDemo, doc)
	if !a.Cuadra() {
		t.Fatal("el asiento de la venta no cuadra")
	}
	casiEq(t, haberCta(a, contabilidad.CtaVentas), 200, "ventas gravadas al haber")
	casiEq(t, haberCta(a, contabilidad.CtaIVADebito), 32, "IVA débito al haber")
	casiEq(t, debeCta(a, contabilidad.CtaCajaBancos), total, "lo cobrado al debe de Caja/Bancos")
	casiEq(t, debeCta(a, contabilidad.CtaCuentasPorCobrar), 0, "una venta de contado no genera CxC")

	// (c) Costo de ventas: sale del costo promedio del ledger contra Inventario.
	costoEsperado := round2Test(costo0 * cantVendida)
	if costoEsperado > 0.004 {
		costo := asientosPorRef(svc, empDemo, "documento", doc.ID)
		var costoAsiento *contabilidad.Asiento
		for i := range costo {
			if costo[i].ID != a.ID { // el otro asiento del documento es el de costo
				cp := costo[i]
				costoAsiento = &cp
			}
		}
		if costoAsiento == nil {
			t.Fatal("con costo promedio > 0 debía derivarse un asiento de costo de ventas")
		}
		casiEq(t, debeCta(*costoAsiento, contabilidad.CtaCostoDeVentas), costoEsperado, "costo de ventas al debe")
		casiEq(t, haberCta(*costoAsiento, contabilidad.CtaInventario), costoEsperado, "salida de inventario al haber")
	}

	// (d) No debe figurar por cobrar.
	if _, hay := svc.SaldoPorCobrar(empDemo, doc.ID); hay {
		t.Error("una venta de contado no debe aparecer en cuentas por cobrar")
	}
	assertLibroCuadra(t, svc, empDemo)
}

// TestIntegracion_VentaACredito_VaACuentasPorCobrar prueba el cruce Fiscal⇄CxC:
// una venta a crédito no mueve caja; el saldo queda por cobrar con el monto exacto
// y el asiento carga Cuentas por cobrar (no Caja).
func TestIntegracion_VentaACredito_VaACuentasPorCobrar(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 2) // 200 + IVA 32 = 232, a crédito, sin abono

	// La proyección de CxC muestra el saldo completo.
	saldo, hay := svc.SaldoPorCobrar(empDemo, doc.ID)
	if !hay {
		t.Fatal("la factura a crédito debe aparecer por cobrar")
	}
	casiEq(t, saldo, 232, "el saldo por cobrar es el total de la factura a crédito")

	// El asiento va contra Cuentas por cobrar, no contra Caja.
	a := asientoVentaDe(t, svc, empDemo, doc)
	casiEq(t, debeCta(a, contabilidad.CtaCuentasPorCobrar), 232, "el total va al debe de CxC")
	casiEq(t, debeCta(a, contabilidad.CtaCajaBancos), 0, "una venta a crédito no mueve caja")
	if !a.Cuadra() {
		t.Error("el asiento de la venta a crédito no cuadra")
	}
	assertLibroCuadra(t, svc, empDemo)
}

/* --- 2. Nota de crédito parcial → reingreso parcial + CxC baja -------------- */

// TestIntegracion_NotaCreditoParcial prueba el cruce Fiscal⇄Inventario⇄CxC⇄
// Contabilidad de una devolución PARCIAL: reingresa al inventario SOLO lo devuelto
// (movimiento `entrada`), el asiento contrario cuadra, y el saldo por cobrar baja
// por el monto de la nota. Append-only: la factura original queda intacta.
func TestIntegracion_NotaCreditoParcial(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 3) // 3 u × 100 = 300 + IVA 48 = 348, a crédito

	saldo0, _ := svc.SaldoPorCobrar(empDemo, doc.ID)
	casiEq(t, saldo0, 348, "saldo inicial por cobrar")
	cantTrasVenta, _ := existenciaDe(t, svc, empDemo, sede1, doc.Lineas[0].SKU)

	// Se devuelve 1 de las 3 unidades → NC por 100 + IVA 16 = 116 (negativo).
	nc, err := svc.EmitirNotaCredito(empDemo, sede1, actorA, origenTst, doc.ID, "devolución parcial",
		[]application.LineaEntrada{{SKU: doc.Lineas[0].SKU, Cantidad: 1}})
	if err != nil {
		t.Fatalf("emitir nota de crédito: %v", err)
	}
	if nc.Total >= 0 {
		t.Fatalf("la NC debe guardar montos negativos, Total=%v", nc.Total)
	}
	casiEq(t, -nc.Total, 116, "la NC acredita 116 (1 u + IVA)")

	// (a) Inventario: reingreso PARCIAL de 1 unidad (movimiento `entrada`).
	cantTrasNC, _ := existenciaDe(t, svc, empDemo, sede1, doc.Lineas[0].SKU)
	casiEq(t, cantTrasNC-cantTrasVenta, 1, "el stock debe reingresar solo lo devuelto")
	n, reingresado := movsDeRef(t, svc, empDemo, sede1, doc.Lineas[0].SKU, inventario.MovEntrada, nc.ID)
	if n != 1 {
		t.Fatalf("debía anexarse 1 movimiento de entrada por la NC, hubo %d", n)
	}
	casiEq(t, reingresado, 1, "el reingreso es de 1 unidad")

	// (b) CxC: el saldo baja EXACTAMENTE por el monto de la nota.
	saldo1, _ := svc.SaldoPorCobrar(empDemo, doc.ID)
	casiEq(t, saldo0-saldo1, 116, "el saldo por cobrar baja por el monto de la NC")

	// (c) Contabilidad: el asiento contrario de la NC cuadra y el libro global
	// sigue cuadrando; la factura original no se tocó.
	revs := asientosPorRef(svc, empDemo, "documento", nc.ID)
	if len(revs) == 0 {
		t.Fatal("la NC debe derivar su asiento contrario")
	}
	for _, r := range revs {
		if !r.Cuadra() {
			t.Errorf("el asiento contrario de la NC no cuadra: %+v", r)
		}
	}
	if _, viva := svc.Documento(empDemo, doc.ID); !viva {
		t.Error("la factura original debe seguir existiendo (append-only)")
	}
	// (d) Cruce Contabilidad ⇄ Tesorería (Principio 4): una NC sobre venta a
	// CRÉDITO baja Cuentas por Cobrar (1102), NO devuelve por Caja. El saldo de
	// 1102 en el libro debe igualar la proyección de Tesorería, y no debe haberse
	// contabilizado un reembolso de efectivo fantasma. (Regresión del bug que
	// prorrateaba mal la reversa de la NC.)
	casiEq(t, saldoContableCta(svc, empDemo, contabilidad.CtaCuentasPorCobrar), saldo1,
		"el saldo contable de CxC (1102) debe igualar la proyección de Tesorería tras la NC")
	casiEq(t, saldoContableCta(svc, empDemo, contabilidad.CtaCajaBancos), 0,
		"una NC sobre venta a crédito no debe mover Caja/Bancos (no hubo reembolso en efectivo)")
	assertLibroCuadra(t, svc, empDemo)
}

/* --- 2b. Nota de débito → cargo adicional positivo, CxC e ingresos suben ---- */

// TestIntegracion_NotaDebito prueba el cruce Fiscal⇄CxC⇄Contabilidad de un CARGO
// ADICIONAL (espejo positivo de la nota de crédito): la nota de débito referencia
// la factura, guarda montos POSITIVOS, su asiento cuadra y sube Cuentas por Cobrar
// (1102) e ingresos (4101) + IVA débito (2201) por el monto exacto. No toca
// inventario. Append-only: la factura original queda intacta.
func TestIntegracion_NotaDebito(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 3) // factura a crédito: 3 u × 100 + IVA 48 = 348

	sku := doc.Lineas[0].SKU
	cxc0 := saldoContableCta(svc, empDemo, contabilidad.CtaCuentasPorCobrar)
	ventas0 := saldoContableCta(svc, empDemo, contabilidad.CtaVentas)
	ivaDeb0 := saldoContableCta(svc, empDemo, contabilidad.CtaIVADebito)
	stock0, _ := existenciaDe(t, svc, empDemo, sede1, sku)

	// Cargo adicional gravado de 100 (interés/corrección al alza) → base 100, IVA
	// 16, total 116, POSITIVO.
	nd, err := svc.EmitirNotaDebito(empDemo, sede1, actorA, origenTst, doc.ID, application.NotaDebitoEntrada{
		Concepto: "Interés de mora por pago tardío", Monto: 100,
	})
	if err != nil {
		t.Fatalf("emitir nota de débito: %v", err)
	}

	// (a) El documento nota_debito existe, con montos POSITIVOS y su serie propia.
	if nd.Tipo != fiscal.TipoNotaDebito {
		t.Fatalf("el tipo debe ser nota_debito, se obtuvo %q", nd.Tipo)
	}
	if nd.Total <= 0 {
		t.Fatalf("la nota de débito debe guardar montos positivos, Total=%v", nd.Total)
	}
	casiEq(t, nd.Total, 116, "total de la ND (100 + IVA 16)")
	casiEq(t, nd.IVA, 16, "IVA de la ND")
	casiEq(t, nd.BaseImponible, 100, "base imponible de la ND")
	casiEq(t, nd.IGTF, 0, "una ND no causa IGTF")
	if nd.RefDocumentoID != doc.ID {
		t.Errorf("la ND debe referenciar la factura %q, referencia %q", doc.ID, nd.RefDocumentoID)
	}
	if got, ok := svc.Documento(empDemo, nd.ID); !ok || got.Serie == "" || got.Serie[len(got.Serie)-3:] != "-ND" {
		t.Errorf("la ND debe tener serie propia terminada en -ND, se obtuvo %q", nd.Serie)
	}

	// (b) El asiento de la ND cuadra: Debe CxC 116 / Haber Ventas 100 + IVA 16.
	asientos := asientosPorRef(svc, empDemo, "documento", nd.ID)
	if len(asientos) != 1 {
		t.Fatalf("la ND debía derivar 1 asiento, hay %d", len(asientos))
	}
	a := asientos[0]
	if !a.Cuadra() {
		t.Fatalf("el asiento de la ND no cuadra: %+v", a)
	}
	casiEq(t, debeCta(a, contabilidad.CtaCuentasPorCobrar), 116, "el cargo total al debe de CxC")
	casiEq(t, haberCta(a, contabilidad.CtaVentas), 100, "el cargo gravado al haber de Ventas")
	casiEq(t, haberCta(a, contabilidad.CtaIVADebito), 16, "el IVA del cargo al haber de IVA débito")

	// (c) CxC e ingresos suben EXACTAMENTE por el cargo; el IVA débito también.
	// saldoContableCta mide (Σ Debe − Σ Haber): CxC es deudora (sube al debe, +116);
	// Ventas e IVA débito son acreedoras (suben al haber), así que su saldo Debe−Haber
	// BAJA por el crédito → el aumento se mide como ventas0 − después.
	casiEq(t, saldoContableCta(svc, empDemo, contabilidad.CtaCuentasPorCobrar)-cxc0, 116,
		"Cuentas por Cobrar sube por el total de la ND")
	casiEq(t, ventas0-saldoContableCta(svc, empDemo, contabilidad.CtaVentas), 100,
		"los ingresos suben por la base del cargo")
	casiEq(t, ivaDeb0-saldoContableCta(svc, empDemo, contabilidad.CtaIVADebito), 16,
		"el IVA débito sube por el IVA del cargo")

	// (d) No toca inventario (es un ajuste de valor, no mercancía).
	stock1, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	casiEq(t, stock1, stock0, "la ND no debe mover el inventario")

	// (e) Append-only: la factura original sigue intacta.
	if orig, ok := svc.Documento(empDemo, doc.ID); !ok {
		t.Error("la factura original debe seguir existiendo (append-only)")
	} else if !casi(orig.Total, doc.Total) {
		t.Error("el total de la factura original no puede cambiar al emitir una ND")
	}
	assertLibroCuadra(t, svc, empDemo)
}

/* --- 3. Ciclo de compra completo ------------------------------------------- */

// TestIntegracion_CicloDeCompraCompleto recorre Compras⇄Inventario⇄CxP⇄
// Contabilidad de punta a punta: proveedor → OC → confirmar → recibir parcial →
// recibir resto → factura de compra (IVA crédito) → pago → retención emitida.
// Verifica cada saldo de CxP y que cada asiento derivado cuadra.
func TestIntegracion_CicloDeCompraCompleto(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)

	prov, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Suplidora Integración, C.A.", Documento: "J-99999999-8",
	})
	if err != nil {
		t.Fatalf("crear proveedor: %v", err)
	}

	// OC de 10 u a Bs 20 → subtotal 200, IVA 32, total 232.
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: prov.ID, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 20}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar OC: %v", err)
	}

	cant0, _ := existenciaDe(t, svc, empDemo, sede1, sku)

	// (1) Recepción PARCIAL: 6 de 10 → stock +6, deuda neta 120, asiento
	// Debe Inventario 120 / Haber CxP 120.
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 6}}); err != nil {
		t.Fatalf("recepción parcial: %v", err)
	}
	cant1, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	casiEq(t, cant1-cant0, 6, "el stock sube por lo recibido en la parcial")
	casiEq(t, deudaDeOrden(t, svc, oc.ID).Recibido, 120, "la deuda de la orden = costo neto recibido (6×20)")
	casiEq(t, saldoProveedor(t, svc, prov.ID), 120, "el saldo del proveedor tras la parcial")
	recA := asientosPorRef(svc, empDemo, "compra", oc.ID)
	if len(recA) != 1 {
		t.Fatalf("la recepción parcial debía derivar 1 asiento, hay %d", len(recA))
	}
	casiEq(t, debeCta(recA[0], contabilidad.CtaInventario), 120, "inventario al debe (parcial)")
	casiEq(t, haberCta(recA[0], contabilidad.CtaCuentasPorPagar), 120, "CxP al haber (parcial)")
	if !recA[0].Cuadra() {
		t.Error("el asiento de la recepción parcial no cuadra")
	}

	// (2) Recepción del resto: 4 → stock +4 (total 10), deuda neta 200.
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 4}}); err != nil {
		t.Fatalf("recepción del resto: %v", err)
	}
	cant2, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	casiEq(t, cant2-cant0, 10, "el stock sube por el total recibido")
	casiEq(t, deudaDeOrden(t, svc, oc.ID).Recibido, 200, "deuda neta tras recibir todo")

	// (3) Factura de compra → reconoce IVA crédito (Debe 1103 / Haber 2101 = 32)
	// y la deuda de la orden pasa a base + IVA (232).
	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-INT-001", NumeroControl: "00-INT-001", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("registrar factura de compra: %v", err)
	}
	casiEq(t, fc.IVA, 32, "IVA crédito de la factura de compra")
	facA := asientosPorRef(svc, empDemo, "factura_compra", fc.ID)
	if len(facA) != 1 {
		t.Fatalf("la factura de compra debía derivar 1 asiento, hay %d", len(facA))
	}
	casiEq(t, debeCta(facA[0], contabilidad.CtaIVACreditoFiscal), 32, "IVA crédito al debe 1103")
	casiEq(t, haberCta(facA[0], contabilidad.CtaCuentasPorPagar), 32, "IVA al haber 2101")
	if !facA[0].Cuadra() {
		t.Error("el asiento de la factura de compra no cuadra")
	}
	casiEq(t, deudaDeOrden(t, svc, oc.ID).Recibido, fc.Total, "tras facturar la deuda es el total (base+IVA)")
	casiEq(t, saldoProveedor(t, svc, prov.ID), 232, "saldo del proveedor con IVA incluido")

	// (4) Pago a proveedor de 100 imputado a la orden → CxP baja 100 (232→132),
	// asiento Debe 2101 / Haber 1101 = 100.
	pago, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: prov.ID, OrdenCompraID: oc.ID, MontoBs: 100, Metodo: fiscal.PagoTransfer,
	})
	if err != nil {
		t.Fatalf("registrar pago: %v", err)
	}
	casiEq(t, saldoProveedor(t, svc, prov.ID), 132, "el saldo baja por el pago")
	pagA := asientosPorRef(svc, empDemo, "pago_proveedor", pago.ID)
	if len(pagA) != 1 {
		t.Fatalf("el pago debía derivar 1 asiento, hay %d", len(pagA))
	}
	casiEq(t, debeCta(pagA[0], contabilidad.CtaCuentasPorPagar), 100, "CxP al debe 2101")
	casiEq(t, haberCta(pagA[0], contabilidad.CtaCajaBancos), 100, "Caja al haber 1101")

	// (5) Retención de IVA emitida al 75% sobre el IVA (32×0.75 = 24) → CxP baja
	// 24 (132→108), asiento Debe 2101 / Haber 2203 = 24.
	ret, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc.ID, application.EntradaRetencion{
		NumeroComprobante: "COMP-INT-01", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 75,
	})
	if err != nil {
		t.Fatalf("registrar retención emitida: %v", err)
	}
	casiEq(t, ret.MontoRetenido, 24, "monto retenido (32×0.75)")
	casiEq(t, saldoProveedor(t, svc, prov.ID), 108, "el saldo baja por lo retenido")
	retA := asientoRetencion(t, svc, ret.ID)
	casiEq(t, debeCta(retA, contabilidad.CtaCuentasPorPagar), 24, "CxP al debe 2101 (retención)")
	casiEq(t, haberCta(retA, contabilidad.CtaIVARetenidoPorEnterar), 24, "IVA retenido al haber 2203")

	assertLibroCuadra(t, svc, empDemo)
}

/* --- 3b. Notas de crédito/débito de PROVEEDOR → CxP baja/sube --------------- */

// TestIntegracion_NotasDeCompra_AjustanCxP prueba el cruce Compras⇄CxP⇄Contabilidad
// de las notas de crédito y débito de PROVEEDOR sobre una factura de compra:
//   - la NC (devolución/descuento) guarda montos NEGATIVOS, su asiento cuadra
//     (Debe CxP / Haber IVA crédito + Inventario) y BAJA la deuda con el proveedor;
//   - la ND (cargo adicional) guarda montos POSITIVOS, su asiento cuadra (Debe
//     Inventario + IVA crédito / Haber CxP) y SUBE la deuda;
//   - la factura de compra original queda intacta (append-only) y el libro cuadra.
func TestIntegracion_NotasDeCompra_AjustanCxP(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)

	prov, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Suplidora Notas, C.A.", Documento: "J-12121212-8",
	})
	if err != nil {
		t.Fatalf("crear proveedor: %v", err)
	}

	// OC de 10 u a Bs 20 → base 200, IVA 32, total 232. Recibida y facturada.
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: prov.ID, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 20}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar OC: %v", err)
	}
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 10}}); err != nil {
		t.Fatalf("recibir OC: %v", err)
	}
	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-NC-001", NumeroControl: "00-NC-001", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("registrar factura de compra: %v", err)
	}
	casiEq(t, saldoProveedor(t, svc, prov.ID), 232, "saldo inicial del proveedor (base+IVA)")

	// (1) NOTA DE CRÉDITO de proveedor: devolución/descuento de base 50 (IVA 8, total
	// 58) → montos NEGATIVOS, la deuda BAJA 58 (232→174).
	nc, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, application.NotaCompraEntrada{
		Concepto: "Devolución de mercancía dañada", Monto: 50,
		NumeroDocumento: "NC-P-01", NumeroControl: "00-NCP-01", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("emitir NC de compra: %v", err)
	}
	if nc.Tipo != compra.NotaCreditoCompra {
		t.Fatalf("tipo esperado nota_credito, fue %q", nc.Tipo)
	}
	if nc.Total >= 0 {
		t.Fatalf("la NC de compra debe guardar montos negativos, Total=%v", nc.Total)
	}
	casiEq(t, -nc.Total, 58, "la NC de compra acredita 58 (50 + IVA 8)")
	casiEq(t, -nc.IVA, 8, "IVA de la NC de compra")
	if nc.FacturaCompraID != fc.ID {
		t.Errorf("la NC debe referenciar la factura %q, referencia %q", fc.ID, nc.FacturaCompraID)
	}
	if nc.Serie != "NCC" {
		t.Errorf("la NC de compra debe llevar serie NCC, fue %q", nc.Serie)
	}
	casiEq(t, saldoProveedor(t, svc, prov.ID), 174, "la NC baja la deuda con el proveedor (232-58)")

	// El asiento de la NC cuadra: Debe CxP 58 / Haber IVA crédito 8 + Inventario 50.
	ncA := asientosPorRef(svc, empDemo, "nota_compra", nc.ID)
	if len(ncA) != 1 {
		t.Fatalf("la NC de compra debía derivar 1 asiento, hay %d", len(ncA))
	}
	casiEq(t, debeCta(ncA[0], contabilidad.CtaCuentasPorPagar), 58, "NC: total al debe de CxP 2101")
	casiEq(t, haberCta(ncA[0], contabilidad.CtaIVACreditoFiscal), 8, "NC: IVA crédito al haber 1103")
	casiEq(t, haberCta(ncA[0], contabilidad.CtaInventario), 50, "NC: base al haber de Inventario 1201")
	if !ncA[0].Cuadra() {
		t.Errorf("el asiento de la NC de compra no cuadra: %+v", ncA[0])
	}

	// (2) NOTA DE DÉBITO de proveedor: cargo adicional de base 30 (IVA 4.8, total
	// 34.8) → montos POSITIVOS, la deuda SUBE 34.8 (174→208.8).
	nd, err := svc.EmitirNotaDebitoCompra(empDemo, actorA, origenTst, fc.ID, application.NotaCompraEntrada{
		Concepto: "Flete no incluido en la factura", Monto: 30,
		NumeroDocumento: "ND-P-01", NumeroControl: "00-NDP-01", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("emitir ND de compra: %v", err)
	}
	if nd.Tipo != compra.NotaDebitoCompra {
		t.Fatalf("tipo esperado nota_debito, fue %q", nd.Tipo)
	}
	if nd.Total <= 0 {
		t.Fatalf("la ND de compra debe guardar montos positivos, Total=%v", nd.Total)
	}
	casiEq(t, nd.Total, 34.8, "la ND de compra carga 34.8 (30 + IVA 4.8)")
	casiEq(t, nd.IVA, 4.8, "IVA de la ND de compra")
	if nd.Serie != "NDC" {
		t.Errorf("la ND de compra debe llevar serie NDC, fue %q", nd.Serie)
	}
	casiEq(t, saldoProveedor(t, svc, prov.ID), 208.8, "la ND sube la deuda con el proveedor (174+34.8)")

	// El asiento de la ND cuadra: Debe Inventario 30 + IVA crédito 4.8 / Haber CxP 34.8.
	ndA := asientosPorRef(svc, empDemo, "nota_compra", nd.ID)
	if len(ndA) != 1 {
		t.Fatalf("la ND de compra debía derivar 1 asiento, hay %d", len(ndA))
	}
	casiEq(t, debeCta(ndA[0], contabilidad.CtaInventario), 30, "ND: base al debe de Inventario 1201")
	casiEq(t, debeCta(ndA[0], contabilidad.CtaIVACreditoFiscal), 4.8, "ND: IVA crédito al debe 1103")
	casiEq(t, haberCta(ndA[0], contabilidad.CtaCuentasPorPagar), 34.8, "ND: total al haber de CxP 2101")
	if !ndA[0].Cuadra() {
		t.Errorf("el asiento de la ND de compra no cuadra: %+v", ndA[0])
	}

	// (3) Append-only: la factura de compra original quedó intacta.
	if orig, ok := svc.FacturaCompra(empDemo, fc.ID); !ok {
		t.Error("la factura de compra original debe seguir existiendo (append-only)")
	} else if !casi(orig.Total, fc.Total) {
		t.Error("el total de la factura de compra no puede cambiar al emitir notas")
	}

	// (4) Anti-sobre-crédito: no se puede acreditar más de lo facturado (232) con NC.
	// Ya se acreditaron 58; una NC de base 200 (total 232) excede el resto (174).
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, application.NotaCompraEntrada{
		Concepto: "Descuento excesivo", Monto: 200,
		NumeroDocumento: "NC-P-02", NumeroControl: "00-NCP-02", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	}); !errors.Is(err, application.ErrNotaCreditoCompraExcede) {
		t.Fatalf("una NC que excede el total facturado debía dar ErrNotaCreditoCompraExcede, se obtuvo: %v", err)
	}

	assertLibroCuadra(t, svc, empDemo)
}

/* --- 4. Retención recibida sobre venta a crédito → CxC baja ----------------- */

// TestIntegracion_RetencionRecibida_BajaCxC prueba Fiscal⇄CxC⇄Contabilidad: sobre
// una factura de venta a crédito (IVA 100), una retención recibida al 75% baja el
// saldo por cobrar 75 con asiento Debe 1104 / Haber 1102; un cobro del resto deja
// la cuenta en cero. El libro cuadra en cada paso.
func TestIntegracion_RetencionRecibida_BajaCxC(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaCreditoIVA100(t, svc) // total 725, por cobrar 725

	saldo0, _ := svc.SaldoPorCobrar(empDemo, doc.ID)
	casiEq(t, saldo0, 725, "saldo por cobrar inicial")

	ret, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		NumeroComprobante: "20260800099999", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 75,
	})
	if err != nil {
		t.Fatalf("registrar retención recibida: %v", err)
	}
	casiEq(t, ret.MontoRetenido, 75, "retenido 75% del IVA de 100")

	saldo1, _ := svc.SaldoPorCobrar(empDemo, doc.ID)
	casiEq(t, saldo0-saldo1, 75, "la retención baja el saldo por cobrar")

	a := asientoRetencion(t, svc, ret.ID)
	casiEq(t, debeCta(a, contabilidad.CtaRetencionIVAaFavor), 75, "retención IVA a favor al debe 1104")
	casiEq(t, haberCta(a, contabilidad.CtaCuentasPorCobrar), 75, "CxC al haber 1102")
	if !a.Cuadra() {
		t.Error("el asiento de la retención recibida no cuadra")
	}

	// Cobrar el resto (650) deja la factura sin saldo.
	if _, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: doc.ID, Monto: saldo1, Moneda: "VES", Metodo: fiscal.PagoTransfer,
	}); err != nil {
		t.Fatalf("cobrar el resto: %v", err)
	}
	if _, hay := svc.SaldoPorCobrar(empDemo, doc.ID); hay {
		t.Error("tras retención + cobro del resto la factura no debe seguir por cobrar")
	}
	assertLibroCuadra(t, svc, empDemo)
}

/* --- 5. Cierre de período contable ----------------------------------------- */

// TestIntegracion_CierrePeriodo_BloqueaMesCerradoNoElDeHoy prueba la barrera §7.3:
// un mes cerrado impide anexar un asiento con fecha dentro de él, pero jamás
// bloquea la operación corriente (Fecha=hoy).
func TestIntegracion_CierrePeriodo_BloqueaMesCerradoNoElDeHoy(t *testing.T) {
	// --- Parte A: NO bloquea la operación de hoy (cierre de un mes anterior real).
	svc, _ := nuevoServicio(t)
	if _, err := emitirContadoOk(t, svc, sede1, 100); err != nil {
		t.Fatalf("venta previa: %v", err)
	}
	ahora := time.Now().UTC()
	mesAnterior := ahora.AddDate(0, -1, 0)
	if _, err := svc.CerrarPeriodo(empDemo, actorA, origenTst, mesAnterior.Year(), int(mesAnterior.Month())); err != nil {
		t.Fatalf("cerrar el mes anterior debería poder: %v", err)
	}
	// Una venta de HOY se asienta igual: su Fecha (hoy) es posterior al cierre.
	doc2 := emitirContado(t, svc, sede1, 50)
	if len(asientosPorRef(svc, empDemo, "documento", doc2.ID)) == 0 {
		t.Error("una operación de hoy no debe quedar bloqueada por un mes anterior cerrado")
	}
	// Y no se puede cerrar el mes en curso (ni uno futuro).
	if _, err := svc.CerrarPeriodo(empDemo, actorA, origenTst, ahora.Year(), int(ahora.Month())); !errors.Is(err, application.ErrCierreInvalido) {
		t.Errorf("cerrar el mes en curso debía dar ErrCierreInvalido, se obtuvo: %v", err)
	}
	assertLibroCuadra(t, svc, empDemo)

	// --- Parte B: SÍ bloquea un asiento cuya fecha cae en un mes cerrado.
	// La barrera vive en el asentado y solo se activa contra un período cuyo
	// FechaCierre alcanza la fecha del asiento. Como CerrarPeriodo (por diseño)
	// nunca cierra el mes en curso, persistimos DIRECTAMENTE en el mismo store un
	// registro de período cuyo corte cubre HOY, para ejercer la barrera de forma
	// determinista. Reversar un asiento de hoy cae entonces dentro del mes cerrado.
	svc2, st2 := nuevoServicio(t)
	doc := emitirContado(t, svc2, sede1, 100)
	original := asientoVentaDe(t, svc2, empDemo, doc)

	corteFuturo := time.Now().UTC().AddDate(0, 1, 0) // un mes por delante: cubre hoy
	st2.Periodos.Append(contabilidad.Periodo{
		EmpresaID: empDemo, Anio: corteFuturo.Year(), Mes: int(corteFuturo.Month()),
		FechaCierre: corteFuturo.Format(time.RFC3339), CerradoPor: actorA, CerradoEl: ahora.Format(time.RFC3339),
	})
	// El contrario llevaría Fecha=hoy, que cae dentro del mes cerrado → rechazo.
	if _, err := svc2.RevertirAsiento(empDemo, actorA, origenTst, original.ID, "cae en mes cerrado"); !errors.Is(err, application.ErrPeriodoCerrado) {
		t.Fatalf("un asiento con fecha en un mes cerrado debía dar ErrPeriodoCerrado, se obtuvo: %v", err)
	}
	// El asiento original queda intacto (no se editó ni se le anexó contrario).
	for _, a := range svc2.LibroDiario(empDemo) {
		if a.Contrario && a.RefAsientoID == original.ID {
			t.Error("no debió anexarse ningún contrario del asiento en el mes cerrado")
		}
	}
}

// emitirContadoOk es como emitirContado pero devuelve el error en vez de abortar,
// para los flujos que necesitan encadenar un cierre después.
func emitirContadoOk(t *testing.T, svc *application.Service, sede string, precio float64) (fiscal.Documento, error) {
	t.Helper()
	return svc.EmitirFactura(empDemo, sede, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true,
		Lineas:  []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: precio}},
		Pagos:   []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: precio * 2, Moneda: "VES"}},
	})
}

/* --- 6. Cierre Z consolida desde el último corte --------------------------- */

// TestIntegracion_CierreZ_ConsolidaYRechazaVacio prueba Fiscal⇄CierreZ: el Z
// consolida los documentos de la sede desde el último corte; un segundo Z sin
// nada nuevo da ErrNadaQueCerrar; una venta nueva habilita un tercer Z que solo
// toma lo nuevo.
func TestIntegracion_CierreZ_ConsolidaYRechazaVacio(t *testing.T) {
	svc, _ := nuevoServicio(t)
	emitirContado(t, svc, sede1, 100)
	emitirContado(t, svc, sede1, 100)

	prevTot, prevCant, err := svc.PreviewCierreZ(empDemo, sede1)
	if err != nil {
		t.Fatalf("preview Z: %v", err)
	}
	if prevCant < 2 {
		t.Fatalf("el preview debía contar al menos las 2 ventas nuevas, contó %d", prevCant)
	}
	z1, err := svc.EmitirCierreZ(empDemo, sede1, actorA, origenTst)
	if err != nil {
		t.Fatalf("emitir Z: %v", err)
	}
	casiEq(t, z1.Totales.TotalNeto, prevTot.TotalNeto, "el Z consolida lo que el preview anticipó")

	// Segundo Z sin ventas nuevas → nada que cerrar.
	if _, err := svc.EmitirCierreZ(empDemo, sede1, actorA, origenTst); !errors.Is(err, application.ErrNadaQueCerrar) {
		t.Fatalf("un Z sin documentos nuevos debía dar ErrNadaQueCerrar, se obtuvo: %v", err)
	}

	// Una venta nueva habilita el siguiente Z, que solo toma lo nuevo.
	nueva := emitirContado(t, svc, sede1, 50)
	z2, err := svc.EmitirCierreZ(empDemo, sede1, actorA, origenTst)
	if err != nil {
		t.Fatalf("segundo Z real: %v", err)
	}
	if z2.Numero != z1.Numero+1 {
		t.Errorf("la secuencia Z debe avanzar de %d a %d, dio %d", z1.Numero, z1.Numero+1, z2.Numero)
	}
	if z2.Totales.CantidadFacturas != 1 || z2.DocHasta != nueva.NumeroCompleto {
		t.Errorf("el segundo Z solo debe tomar la venta nueva %s, dio %+v", nueva.NumeroCompleto, z2.Totales)
	}
}

/* --- 7. Transferencia despachada y cancelada → devuelve stock -------------- */

// TestIntegracion_TransferenciaCancelada_DevuelveStock prueba la máquina de
// estados de Inventario: despachar saca stock del origen; cancelar en tránsito lo
// devuelve con un movimiento compensatorio (append-only, no borra el despacho);
// y una transferencia ya recibida NO se puede cancelar.
func TestIntegracion_TransferenciaCancelada_DevuelveStock(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	cant0, _ := existenciaDe(t, svc, empDemo, sede1, sku)

	// (a) Crear + despachar: el stock del origen baja.
	tr, err := svc.CrearTransferencia(empDemo, actorA, origenTst, inventario.Transferencia{
		OrigenSedeID: sede1, DestinoSedeID: sede2,
		Lineas: []inventario.LineaTransferencia{{SKU: sku, Cantidad: 3}},
	})
	if err != nil {
		t.Fatalf("crear transferencia: %v", err)
	}
	if _, err := svc.CambiarEstadoTransferencia(empDemo, tr.ID, inventario.TransfDespachada, actorA, origenTst); err != nil {
		t.Fatalf("despachar: %v", err)
	}
	cantDesp, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	casiEq(t, cant0-cantDesp, 3, "el despacho saca stock del origen")

	// (b) Cancelar: el stock del origen se devuelve completo.
	if _, err := svc.CancelarTransferencia(empDemo, tr.ID, actorA, origenTst, "se anuló el traslado"); err != nil {
		t.Fatalf("cancelar transferencia: %v", err)
	}
	cantCanc, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	casiEq(t, cantCanc, cant0, "cancelar devuelve el stock al origen")
	// El despacho original NO se borró: siguen existiendo sus dos patas en el Kardex.
	nSalida, _ := movsDeRef(t, svc, empDemo, sede1, sku, inventario.MovTransferencia, tr.ID)
	if nSalida < 2 {
		t.Errorf("el ledger es append-only: deben quedar el despacho y la compensación (≥2 movs), hay %d", nSalida)
	}

	// (c) Una transferencia YA RECIBIDA no se puede cancelar.
	tr2, err := svc.CrearTransferencia(empDemo, actorA, origenTst, inventario.Transferencia{
		OrigenSedeID: sede1, DestinoSedeID: sede2,
		Lineas: []inventario.LineaTransferencia{{SKU: sku, Cantidad: 2}},
	})
	if err != nil {
		t.Fatalf("crear segunda transferencia: %v", err)
	}
	for _, estado := range []string{inventario.TransfDespachada, inventario.TransfEnTransito, inventario.TransfRecibida} {
		if _, err := svc.CambiarEstadoTransferencia(empDemo, tr2.ID, estado, actorA, origenTst); err != nil {
			t.Fatalf("avanzar a %s: %v", estado, err)
		}
	}
	if _, err := svc.CancelarTransferencia(empDemo, tr2.ID, actorA, origenTst, "tarde"); !errors.Is(err, application.ErrTransferNoCancelable) {
		t.Fatalf("cancelar una transferencia recibida debía dar ErrTransferNoCancelable, se obtuvo: %v", err)
	}
}

/* --- 8. Aislamiento multi-tenant ------------------------------------------- */

// TestIntegracion_AislamientoMultiTenant prueba el Principio 3 (aislamiento de
// tenant) de punta a punta: con DOS empresas activas en el mismo Service, las
// consultas de una jamás devuelven datos de la otra — ni productos, ni
// existencias, ni documentos, ni asientos.
func TestIntegracion_AislamientoMultiTenant(t *testing.T) {
	svc, st := nuevoServicio(t)
	const (
		empDos  = "emp_dos"
		sedeDos = "sede_dos"
	)

	// Segunda empresa con su propio catálogo, stock y una venta.
	st.Empresas.Create(empresa.Empresa{ID: empDos, Nombre: "Otra Empresa, C.A.", MonedaPrincipal: empresa.MonedaVES})
	prod, err := svc.CrearProducto(empDos, actorA, origenTst, inventario.Producto{
		SKU: "DOS-1", Nombre: "Producto de la otra empresa", Precio: 10, UnidadBase: "unidad",
	})
	if err != nil {
		t.Fatalf("crear producto en emp_dos: %v", err)
	}
	// Stock inicial en emp_dos (entrada directa al ledger, como hace el seed).
	st.Movimientos.Append(inventario.Movimiento{
		EmpresaID: empDos, SedeID: sedeDos, ProductoID: prod.ID, SKU: prod.SKU,
		Tipo: inventario.MovEntrada, Cantidad: 100, CostoUnitario: 6, Motivo: "carga inicial", Actor: actorA, Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	docDos, err := svc.EmitirFactura(empDos, sedeDos, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true,
		Lineas:  []application.LineaEntrada{{SKU: "DOS-1", Cantidad: 5, PrecioUnitario: 10}},
		Pagos:   []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 58, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir en emp_dos: %v", err)
	}

	// Productos: cada empresa ve solo los suyos.
	for _, p := range svc.Productos(empDos) {
		if p.EmpresaID != empDos {
			t.Errorf("emp_dos vio un producto ajeno: %+v", p)
		}
	}
	for _, p := range svc.Productos(empDemo) {
		if p.SKU == "DOS-1" {
			t.Error("emp_demo no debe ver el producto de emp_dos")
		}
	}

	// Existencias: la sede de emp_dos no proyecta SKUs de emp_demo.
	skuDemo := primerSKU(t, svc)
	for _, e := range svc.Existencias(empDos, sedeDos) {
		if e.SKU == skuDemo {
			t.Errorf("las existencias de emp_dos no deben incluir el SKU %s de emp_demo", skuDemo)
		}
	}

	// Documentos: los de emp_dos son solo suyos; emp_demo no ve el documento ajeno.
	for _, d := range svc.Documentos(empDos) {
		if d.EmpresaID != empDos {
			t.Errorf("emp_dos vio un documento ajeno: %s", d.NumeroCompleto)
		}
	}
	if _, hay := svc.Documento(empDemo, docDos.ID); hay {
		t.Error("emp_demo no debe poder resolver un documento de emp_dos por id")
	}

	// Asientos: el libro de emp_dos solo contiene asientos de emp_dos.
	if len(svc.LibroDiario(empDos)) == 0 {
		t.Fatal("la venta en emp_dos debió generar asientos")
	}
	for _, a := range svc.LibroDiario(empDos) {
		if a.EmpresaID != empDos {
			t.Errorf("el libro de emp_dos contiene un asiento ajeno: %s", a.Codigo)
		}
	}
	// Ambos libros cuadran de forma independiente.
	assertLibroCuadra(t, svc, empDos)
	assertLibroCuadra(t, svc, empDemo)
}

/* --- 9. Invariante contable global tras una batería de operaciones ---------- */

// TestIntegracion_InvarianteContableGlobal es la prueba paraguas: tras mezclar
// ventas de contado y a crédito, cobros y su reverso, compras con factura y pago,
// retenciones en ambas direcciones, una nota de crédito y una anulación, la suma
// de TODOS los asientos de la empresa cuadra (Σ Debe == Σ Haber) y cada asiento
// cuadra por separado. Es el invariante que ningún flujo puede romper.
func TestIntegracion_InvarianteContableGlobal(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)

	// Tasa para habilitar el tramo en divisas (IGTF).
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 40, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}

	// Venta de contado en Bs.
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("venta contado: %v", err)
	}

	// Venta a crédito con cobro parcial y su reverso.
	cred := aCredito(t, svc, 2)
	cob, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: cred.ID, Monto: 50, Moneda: "VES", Metodo: fiscal.PagoPagoMovil,
	})
	if err != nil {
		t.Fatalf("cobro: %v", err)
	}
	if _, err := svc.ReversarCobro(empDemo, actorA, origenTst, cob.ID, "error de imputación"); err != nil {
		t.Fatalf("reverso de cobro: %v", err)
	}

	// Nota de crédito parcial sobre la venta a crédito.
	if _, err := svc.EmitirNotaCredito(empDemo, sede1, actorA, origenTst, cred.ID, "devolución",
		[]application.LineaEntrada{{SKU: cred.Lineas[0].SKU, Cantidad: 1}}); err != nil {
		t.Fatalf("nota de crédito: %v", err)
	}

	// Compra recibida + factura + pago + retención emitida.
	prov, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{Nombre: "Prov Global", Documento: "J-88888888-7"})
	if err != nil {
		t.Fatalf("crear proveedor: %v", err)
	}
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: prov.ID, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 5, CostoUnitario: 20}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar OC: %v", err)
	}
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst, []application.LineaRecepcion{{SKU: sku, Cantidad: 5}}); err != nil {
		t.Fatalf("recibir OC: %v", err)
	}
	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-GLB-1", NumeroControl: "00-GLB-1", Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("factura de compra: %v", err)
	}
	if _, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: prov.ID, OrdenCompraID: oc.ID, MontoBs: 40, Metodo: fiscal.PagoTransfer,
	}); err != nil {
		t.Fatalf("pago proveedor: %v", err)
	}
	if _, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc.ID, application.EntradaRetencion{
		NumeroComprobante: "COMP-GLB-1", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 75,
	}); err != nil {
		t.Fatalf("retención emitida: %v", err)
	}

	// Una anulación total de otra factura.
	anulable, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir anulable: %v", err)
	}
	if _, err := svc.AnularDocumento(empDemo, sede1, actorA, origenTst, anulable.ID, "prueba"); err != nil {
		t.Fatalf("anular: %v", err)
	}

	// EL invariante: pase lo que pase, el libro cuadra.
	b := svc.Balance(empDemo)
	if b.Asientos == 0 {
		t.Fatal("la batería de operaciones debió generar asientos")
	}
	assertLibroCuadra(t, svc, empDemo)
}
