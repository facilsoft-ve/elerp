package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// saldoProveedor localiza el saldo NETO de un proveedor en Cuentas por pagar.
func saldoProveedor(t *testing.T, svc *application.Service, provID string) float64 {
	t.Helper()
	for _, p := range svc.CuentasPorPagar(empDemo).Proveedores {
		if p.ProveedorID == provID {
			return p.Saldo
		}
	}
	return 0
}

// TestRegistrarPagoProveedor_BajaElSaldoYAsienta comprueba el flujo espejo del
// cobro: un pago baja el saldo neto de CxP y asienta Debe 2101 / Haber 1101.
func TestRegistrarPagoProveedor_BajaElSaldoYAsienta(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	oc := ocRecibida(t, svc, sku, 10, 20) // adeudado neto 200

	if !casi(saldoProveedor(t, svc, provDemo1), 200) {
		t.Fatalf("saldo inicial esperado 200, se obtuvo %v", saldoProveedor(t, svc, provDemo1))
	}
	totalAntes := svc.CuentasPorPagar(empDemo).TotalPorPagar

	pago, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: provDemo1, OrdenCompraID: oc.ID, MontoBs: 120, Metodo: fiscal.PagoPagoMovil,
	})
	if err != nil {
		t.Fatalf("registrar pago: %v", err)
	}

	// El saldo del proveedor baja 120 → 80, y el de la orden también (el pago la referencia).
	if !casi(saldoProveedor(t, svc, provDemo1), 80) {
		t.Errorf("saldo tras pagar 120 esperado 80, se obtuvo %v", saldoProveedor(t, svc, provDemo1))
	}
	res := svc.CuentasPorPagar(empDemo)
	if !casi(res.TotalPorPagar, totalAntes-120) {
		t.Errorf("el total por pagar debía bajar 120 (de %v a %v), quedó %v", totalAntes, totalAntes-120, res.TotalPorPagar)
	}
	for _, o := range res.Ordenes {
		if o.OrdenID == oc.ID {
			if !casi(o.Pagado, 120) || !casi(o.Saldo, 80) {
				t.Errorf("la orden debía quedar pagada 120 / saldo 80, se obtuvo pagado=%v saldo=%v", o.Pagado, o.Saldo)
			}
		}
	}

	// El asiento del pago: Debe 2101 / Haber 1101 = 120, y cuadra.
	var asiento *contabilidad.Asiento
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefTipo == "pago_proveedor" && a.RefID == pago.ID {
			cp := a
			asiento = &cp
		}
	}
	if asiento == nil {
		t.Fatal("registrar un pago tiene que generar su asiento")
	}
	if !asiento.Cuadra() {
		t.Error("el asiento del pago no cuadra")
	}
	debe := func(codigo string) float64 {
		total := 0.0
		for _, l := range asiento.Lineas {
			if l.Codigo == codigo {
				total += l.Debe
			}
		}
		return total
	}
	haber := func(codigo string) float64 {
		total := 0.0
		for _, l := range asiento.Lineas {
			if l.Codigo == codigo {
				total += l.Haber
			}
		}
		return total
	}
	if !casi(debe(contabilidad.CtaCuentasPorPagar), 120) {
		t.Errorf("Debe 2101 esperado 120, se obtuvo %v", debe(contabilidad.CtaCuentasPorPagar))
	}
	if !casi(haber(contabilidad.CtaCajaBancos), 120) {
		t.Errorf("Haber 1101 esperado 120, se obtuvo %v", haber(contabilidad.CtaCajaBancos))
	}
}

// TestRegistrarPagoProveedor_NoPagaMasQueElSaldo verifica que pagar de más se
// rechaza con ErrPagoExcede (eso sería un anticipo, otro concepto).
func TestRegistrarPagoProveedor_NoPagaMasQueElSaldo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	ocRecibida(t, svc, sku, 10, 20) // adeudado 200

	if _, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: provDemo1, MontoBs: 250, Metodo: fiscal.PagoTransfer,
	}); !errors.Is(err, application.ErrPagoExcede) {
		t.Fatalf("pagar más que el saldo debía dar ErrPagoExcede, se obtuvo: %v", err)
	}
	// Monto no positivo.
	if _, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: provDemo1, MontoBs: 0, Metodo: fiscal.PagoTransfer,
	}); !errors.Is(err, application.ErrPagoInvalido) {
		t.Errorf("un monto de 0 debía dar ErrPagoInvalido, se obtuvo: %v", err)
	}
	// Proveedor inexistente.
	if _, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: "prov_fantasma", MontoBs: 10, Metodo: fiscal.PagoTransfer,
	}); !errors.Is(err, application.ErrProveedorNoPago) {
		t.Errorf("un proveedor inexistente debía dar ErrProveedorNoPago, se obtuvo: %v", err)
	}
}

// TestReversarPagoProveedor_RestituyeElSaldo comprueba que el reverso no borra nada
// y devuelve el saldo, asentando el contrario (Debe 1101 / Haber 2101).
func TestReversarPagoProveedor_RestituyeElSaldo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	ocRecibida(t, svc, sku, 10, 20) // adeudado 200

	pago, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: provDemo1, MontoBs: 200, Metodo: fiscal.PagoTransfer,
	})
	if err != nil {
		t.Fatalf("registrar pago: %v", err)
	}
	if !casi(saldoProveedor(t, svc, provDemo1), 0) {
		t.Fatalf("tras pagar el total el saldo debía ser 0, se obtuvo %v", saldoProveedor(t, svc, provDemo1))
	}

	if _, err := svc.ReversarPagoProveedor(empDemo, actorA, origenTst, pago.ID, "se pagó a la orden equivocada"); err != nil {
		t.Fatalf("reversar: %v", err)
	}
	if !casi(saldoProveedor(t, svc, provDemo1), 200) {
		t.Errorf("tras el reverso el saldo vuelve a 200, se obtuvo %v", saldoProveedor(t, svc, provDemo1))
	}

	// El pago original sigue en el histórico junto a su reverso: no se borró.
	vistos := 0
	for _, p := range svc.PagosProveedor(empDemo) {
		if p.ID == pago.ID || p.RefPagoID == pago.ID {
			vistos++
		}
	}
	if vistos != 2 {
		t.Errorf("el histórico debe conservar el pago y su reverso, se contaron %d", vistos)
	}

	// No se reversa dos veces.
	if _, err := svc.ReversarPagoProveedor(empDemo, actorA, origenTst, pago.ID, "otra vez"); !errors.Is(err, application.ErrPagoYaReversado) {
		t.Errorf("no se puede reversar dos veces: %v", err)
	}

	// El libro sigue cuadrando tras pago + reverso.
	if !svc.Balance(empDemo).Cuadra {
		t.Error("tras el pago y su reverso, el libro tiene que seguir cuadrando")
	}
}
