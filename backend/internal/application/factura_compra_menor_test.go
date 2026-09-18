package application_test

import (
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/contabilidad"
)

/* Regresión del defecto: una factura de proveedor por MENOS de lo recibido no
 * derivaba ningún asiento.
 *
 * La diferencia (facturado − recibido) es negativa, y round2 truncaba hacia cero en
 * negativo: la diferencia y el ajuste de CxP perdían un céntimo cada uno, pero no el
 * mismo, así que el asiento quedaba descuadrado por 0,01. Un asiento descuadrado NO
 * SE GUARDA —solo deja una línea de log—, de modo que el IVA crédito fiscal y la
 * diferencia desaparecían en silencio. No era un importe raro: era una operación
 * entera sin contabilizar. */

// TestFacturaCompraMenorQueLoRecibido_DerivaSuAsiento es el test que fallaba.
func TestFacturaCompraMenorQueLoRecibido_DerivaSuAsiento(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	// Recibido: 10 u × Bs 20 ⇒ base recibida 200, ya asentada en Inventario / CxP.
	oc := ocRecibida(t, svc, sku, 10, 20)

	// El proveedor factura solo 1 de las 10 unidades: base 20, IVA 3,20.
	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-MENOR-1", NumeroControl: "00-00010001",
		Fecha:  time.Now().UTC().Format(time.RFC3339Nano),
		Lineas: []application.LineaFacturaCompraEntrada{{SKU: sku, Cantidad: 1, CostoUnitario: 20}},
	})
	if err != nil {
		t.Fatalf("registrar factura: %v", err)
	}

	// (a) La diferencia guardada es exacta: −180,00, no −179,99.
	casiEq(t, fc.BaseRecibida, 200, "base recibida")
	casiEq(t, fc.BaseImponible, 20, "base facturada")
	casiEq(t, fc.DiferenciaBase, -180, "la diferencia contra lo recibido es exacta")

	// (b) EL PUNTO: la factura deriva su asiento.
	as := asientosPorRef(svc, empDemo, "factura_compra", fc.ID)
	if len(as) != 1 {
		t.Fatalf("la factura debía derivar 1 asiento, hay %d — el IVA crédito y la diferencia se perdieron", len(as))
	}
	a := as[0]
	if !a.Cuadra() {
		t.Fatalf("el asiento de la factura no cuadra: %+v", a)
	}

	// (c) Y dice lo correcto: se reconoce el IVA crédito, la diferencia a favor va al
	// haber de 5202 (recibimos más de lo que nos facturan) y la CxP BAJA.
	casiEq(t, debeCta(a, contabilidad.CtaIVACreditoFiscal), 3.2, "IVA crédito fiscal al debe")
	casiEq(t, haberCta(a, contabilidad.CtaDiferenciaEnCompras), 180, "la diferencia a favor va al haber de 5202")
	casiEq(t, debeCta(a, contabilidad.CtaCuentasPorPagar), 176.8, "CxP baja por el neto (180 − 3,20)")

	assertLibroCuadra(t, svc, empDemo)
}

// TestFacturaCompraMenorQueLoRecibido_CxPQuedaEnElTotalDeLaFactura: la proyección de
// Cuentas por pagar tiene que coincidir con lo que dice el libro.
func TestFacturaCompraMenorQueLoRecibido_CxPQuedaEnElTotalDeLaFactura(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc := ocRecibida(t, svc, sku, 10, 20)

	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-MENOR-2", NumeroControl: "00-00010002",
		Fecha:  time.Now().UTC().Format(time.RFC3339Nano),
		Lineas: []application.LineaFacturaCompraEntrada{{SKU: sku, Cantidad: 1, CostoUnitario: 20}},
	})
	if err != nil {
		t.Fatalf("registrar factura: %v", err)
	}
	// Facturado: base 20 + IVA 3,20 = 23,20. Esa es la deuda real con el proveedor.
	casiEq(t, fc.Total, 23.2, "total de la factura del proveedor")
	casiEq(t, deudaDeOrden(t, svc, oc.ID).Saldo, 23.2, "la deuda de la orden pasa a ser el total facturado")

	// El saldo contable de 2101 tiene que decir lo mismo que la proyección: la
	// recepción asentó 200 y la factura lo bajó a 23,20.
	casiEq(t, saldoContableCta(svc, empDemo, contabilidad.CtaCuentasPorPagar), -23.2,
		"la cuenta por pagar coincide con la deuda proyectada (pasivo: saldo acreedor)")
}

// TestRound2_RedondeaIgualEnAmbosSignos fija la regla del redondeo, que es de donde
// venía el defecto. Se prueba a través de un cálculo real —una factura de compra
// menor a lo recibido— porque round2 no es exportada.
func TestRound2_RedondeaIgualEnAmbosSignos(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)

	// Recibido 3 u × Bs 3,33 ⇒ base 9,99. Facturado 1 u × 3,33 ⇒ 3,33.
	// Diferencia exacta: −6,66. Con el truncado viejo habría salido −6,65.
	oc := ocRecibida(t, svc, sku, 3, 3.33)
	fc, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-REDONDEO", NumeroControl: "00-00010003",
		Fecha:  time.Now().UTC().Format(time.RFC3339Nano),
		Lineas: []application.LineaFacturaCompraEntrada{{SKU: sku, Cantidad: 1, CostoUnitario: 3.33}},
	})
	if err != nil {
		t.Fatalf("registrar factura: %v", err)
	}
	casiEq(t, fc.DiferenciaBase, -6.66, "la diferencia negativa conserva su magnitud")

	as := asientosPorRef(svc, empDemo, "factura_compra", fc.ID)
	if len(as) != 1 || !as[0].Cuadra() {
		t.Fatalf("la factura debía derivar 1 asiento cuadrado, hay %d", len(as))
	}
	assertLibroCuadra(t, svc, empDemo)
}
