package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// ventaCreditoIVA100 emite una factura de venta a crédito con base 625 → IVA 100,
// para que las pruebas de retención tengan un IVA redondo sobre el cual retener.
func ventaCreditoIVA100(t *testing.T, svc *application.Service) fiscal.Documento {
	t.Helper()
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc) // producto gravado del seed
	cl := svc.Clientes(empDemo)
	if len(cl) == 0 {
		t.Fatal("el seed debería traer clientes")
	}
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		ClienteID: cl[0].ID,
		Lineas:    []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 625}},
		Credito:   true, DiasCredito: 30,
	})
	if err != nil {
		t.Fatalf("emitir a crédito: %v", err)
	}
	if !casi(doc.IVA, 100) {
		t.Fatalf("el escenario esperaba IVA 100, fue %v", doc.IVA)
	}
	return doc
}

// asientoRetencion localiza el asiento de una retención por su id.
func asientoRetencion(t *testing.T, svc *application.Service, retID string) contabilidad.Asiento {
	t.Helper()
	var found []contabilidad.Asiento
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefTipo == "retencion_iva" && a.RefID == retID {
			found = append(found, a)
		}
	}
	if len(found) != 1 {
		t.Fatalf("se esperaba 1 asiento de la retención, hay %d", len(found))
	}
	return found[0]
}

// TestRetencionRecibida_AsientaYBajaCxC comprueba una retención recibida al 75%
// sobre una factura con IVA 100: monto retenido 75, asiento Debe 1104 / Haber 1102
// cuadrado, y el saldo por cobrar del documento baja 75.
func TestRetencionRecibida_AsientaYBajaCxC(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaCreditoIVA100(t, svc) // total 725, por cobrar 725

	saldoAntes, _ := svc.SaldoPorCobrar(empDemo, doc.ID)

	ret, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		NumeroComprobante: "20260800012345", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 75,
	})
	if err != nil {
		t.Fatalf("registrar retención recibida: %v", err)
	}
	if !casi(ret.MontoRetenido, 75) {
		t.Fatalf("monto retenido esperado 75, fue %v", ret.MontoRetenido)
	}
	if ret.Tipo != fiscal.RetencionRecibida || ret.DocumentoNumero != doc.NumeroCompleto {
		t.Fatalf("la retención no quedó autocontenida: %+v", ret)
	}

	a := asientoRetencion(t, svc, ret.ID)
	var debe1104, haber1102 float64
	for _, l := range a.Lineas {
		switch l.Codigo {
		case contabilidad.CtaRetencionIVAaFavor:
			debe1104 += l.Debe
		case contabilidad.CtaCuentasPorCobrar:
			haber1102 += l.Haber
		}
	}
	if !casi(debe1104, 75) || !casi(haber1102, 75) {
		t.Fatalf("asiento esperado Debe 1104=75 / Haber 1102=75, fue debe=%v haber=%v", debe1104, haber1102)
	}
	if !a.Cuadra() {
		t.Fatalf("el asiento de la retención no cuadra: %+v", a)
	}

	saldoDespues, _ := svc.SaldoPorCobrar(empDemo, doc.ID)
	if !casi(saldoAntes-saldoDespues, 75) {
		t.Fatalf("el saldo por cobrar debía bajar 75 (de %v a %v)", saldoAntes, saldoDespues)
	}
}

// TestRetencionEmitida_AsientaYBajaCxP comprueba una retención emitida al 75%
// sobre una factura de compra: asiento Debe 2101 / Haber 2203 cuadrado y la deuda
// por pagar del proveedor baja por lo retenido.
func TestRetencionEmitida_AsientaYBajaCxP(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	// Factura de compra: 10 u a Bs 20 → base 200, IVA 32.
	fc := ocFacturada(t, svc, sku, "F-777", "00-777", time.Now().UTC().Format(time.RFC3339Nano))

	saldoAntes := deudaDeProveedor(t, svc, fc.ProveedorID)

	ret, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc.ID, application.EntradaRetencion{
		NumeroComprobante: "COMP-EMI-01", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 75,
	})
	if err != nil {
		t.Fatalf("registrar retención emitida: %v", err)
	}
	esperado := round2Test(fc.IVA * 0.75)
	if !casi(ret.MontoRetenido, esperado) {
		t.Fatalf("monto retenido esperado %v, fue %v", esperado, ret.MontoRetenido)
	}

	a := asientoRetencion(t, svc, ret.ID)
	var debe2101, haber2203 float64
	for _, l := range a.Lineas {
		switch l.Codigo {
		case contabilidad.CtaCuentasPorPagar:
			debe2101 += l.Debe
		case contabilidad.CtaIVARetenidoPorEnterar:
			haber2203 += l.Haber
		}
	}
	if !casi(debe2101, esperado) || !casi(haber2203, esperado) {
		t.Fatalf("asiento esperado Debe 2101=%v / Haber 2203=%v, fue debe=%v haber=%v", esperado, esperado, debe2101, haber2203)
	}
	if !a.Cuadra() {
		t.Fatalf("el asiento de la retención emitida no cuadra: %+v", a)
	}

	saldoDespues := deudaDeProveedor(t, svc, fc.ProveedorID)
	if !casi(saldoAntes-saldoDespues, esperado) {
		t.Fatalf("la deuda por pagar debía bajar %v (de %v a %v)", esperado, saldoAntes, saldoDespues)
	}
}

// TestRetencion_Duplicada verifica que una factura no recibe dos comprobantes de
// la misma dirección.
func TestRetencion_Duplicada(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaCreditoIVA100(t, svc)
	if _, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		NumeroComprobante: "R-1", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 75,
	}); err != nil {
		t.Fatalf("primera retención: %v", err)
	}
	_, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		NumeroComprobante: "R-2", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 100,
	})
	if !errors.Is(err, application.ErrRetencionDuplicada) {
		t.Fatalf("se esperaba ErrRetencionDuplicada, se obtuvo: %v", err)
	}
}

// TestRetencion_PorcentajeInvalido rechaza porcentajes fuera de (0, 100].
func TestRetencion_PorcentajeInvalido(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaCreditoIVA100(t, svc)
	for _, pct := range []float64{0, -5, 150} {
		_, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
			NumeroComprobante: "R-X", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: pct,
		})
		if !errors.Is(err, application.ErrRetencionPorcentaje) {
			t.Fatalf("con porcentaje %v se esperaba ErrRetencionPorcentaje, se obtuvo: %v", pct, err)
		}
	}
}

// TestRetencionEmitida_FacturaInexistente rechaza retener sobre una factura de
// compra que no existe.
func TestRetencionEmitida_FacturaInexistente(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, "fc_inexistente", application.EntradaRetencion{
		NumeroComprobante: "C-1", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 75,
	})
	if !errors.Is(err, application.ErrFacturaCompraNoExiste) {
		t.Fatalf("se esperaba ErrFacturaCompraNoExiste, se obtuvo: %v", err)
	}
}

// ventaContadoIVA100 emite una factura de venta de CONTADO (pagada completa en el
// mostrador) con base 625 → IVA 100 → total 725, para probar que la retención
// recibida se rechaza sobre contado.
func ventaContadoIVA100(t *testing.T, svc *application.Service) fiscal.Documento {
	t.Helper()
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 625}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 725, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir de contado: %v", err)
	}
	if doc.Credito {
		t.Fatalf("el escenario esperaba una factura de contado, quedó a crédito")
	}
	if !casi(doc.IVA, 100) {
		t.Fatalf("el escenario esperaba IVA 100, fue %v", doc.IVA)
	}
	return doc
}

// TestRetencionRecibida_RechazaContado comprueba la corrección del descuadre: una
// retención de IVA recibida NO puede registrarse sobre una factura de contado
// (rechazada con ErrRetencionSoloCredito). El asiento acredita CxC (1102) y la
// proyección de CxC solo pliega documentos a crédito; permitirla sobre contado
// dejaría un saldo 1102 colgado que la proyección nunca reflejaría. Se verifica
// además que no se creó ningún asiento y que el libro global sigue cuadrando.
func TestRetencionRecibida_RechazaContado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaContadoIVA100(t, svc)

	asientosAntes := len(svc.LibroDiario(empDemo))
	retsAntes := len(svc.Retenciones(empDemo)) // el seed trae un comprobante de ISLR

	_, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		NumeroComprobante: "20260800099999", Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 75,
	})
	if !errors.Is(err, application.ErrRetencionSoloCredito) {
		t.Fatalf("se esperaba ErrRetencionSoloCredito sobre contado, se obtuvo: %v", err)
	}

	// Ni comprobante ni asiento NUEVO: el rechazo es total, no deja un 1102 colgado.
	if rets := svc.Retenciones(empDemo); len(rets) != retsAntes {
		t.Fatalf("no debía crearse ningún comprobante de retención (antes %d, después %d)", retsAntes, len(rets))
	}
	if asientosDespues := len(svc.LibroDiario(empDemo)); asientosDespues != asientosAntes {
		t.Fatalf("no debía crearse ningún asiento (antes %d, después %d)", asientosAntes, asientosDespues)
	}
	assertLibroCuadra(t, svc, empDemo)
}

// deudaDeProveedor devuelve el saldo neto por pagar de un proveedor.
func deudaDeProveedor(t *testing.T, svc *application.Service, provID string) float64 {
	t.Helper()
	for _, p := range svc.CuentasPorPagar(empDemo).Proveedores {
		if p.ProveedorID == provID {
			return p.Saldo
		}
	}
	return 0
}

// round2Test redondea a 2 decimales igual que el dominio (para calcular esperados).
func round2Test(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

// TestRetencionRecibidaISLR_AsientaYBajaCxC: una retención de ISLR recibida al 3%
// sobre una base de 1000 (monto 30) baja el saldo por cobrar 30, con asiento Debe
// 1105 (Retención ISLR a favor) / Haber 1102 (Cuentas por cobrar), cuadrado.
func TestRetencionRecibidaISLR_AsientaYBajaCxC(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaCreditoIVA100(t, svc) // total 725, por cobrar 725

	saldoAntes, _ := svc.SaldoPorCobrar(empDemo, doc.ID)

	ret, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoISLR, NumeroComprobante: "20260800077777",
		Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 3,
		Base: 1000, Concepto: "Honorarios profesionales",
	})
	if err != nil {
		t.Fatalf("registrar retención ISLR recibida: %v", err)
	}
	if ret.Impuesto != fiscal.ImpuestoISLR || ret.Concepto != "Honorarios profesionales" {
		t.Fatalf("la retención ISLR no quedó autocontenida: %+v", ret)
	}
	if !casi(ret.MontoRetenido, 30) {
		t.Fatalf("monto ISLR esperado 30 (1000×3%%), fue %v", ret.MontoRetenido)
	}

	a := asientoRetencion(t, svc, ret.ID)
	if !casi(debeCta(a, contabilidad.CtaRetencionISLRaFavor), 30) || !casi(haberCta(a, contabilidad.CtaCuentasPorCobrar), 30) {
		t.Fatalf("asiento esperado Debe 1105=30 / Haber 1102=30, fue %+v", a.Lineas)
	}
	if !a.Cuadra() {
		t.Fatalf("el asiento de la retención ISLR no cuadra: %+v", a)
	}
	saldoDespues, _ := svc.SaldoPorCobrar(empDemo, doc.ID)
	if !casi(saldoAntes-saldoDespues, 30) {
		t.Fatalf("el saldo por cobrar debía bajar 30 (de %v a %v)", saldoAntes, saldoDespues)
	}
}

// TestRetencionEmitidaISLR_AsientaYBajaCxP: una retención de ISLR emitida sobre
// una factura de compra (base 1000, 3%, sustraendo 10 → monto 20) baja la deuda
// por pagar 20, con asiento Debe 2101 (CxP) / Haber 2204 (ISLR por enterar).
func TestRetencionEmitidaISLR_AsientaYBajaCxP(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-ISLR", "00-ISLR", time.Now().UTC().Format(time.RFC3339Nano))

	saldoAntes := deudaDeProveedor(t, svc, fc.ProveedorID)

	ret, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoISLR, NumeroComprobante: "COMP-ISLR-01",
		Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 3,
		Base: 1000, Concepto: "Arrendamiento de inmuebles", Sustraendo: 10,
	})
	if err != nil {
		t.Fatalf("registrar retención ISLR emitida: %v", err)
	}
	if !casi(ret.MontoRetenido, 20) { // max(0, 1000×3% − 10) = 20
		t.Fatalf("monto ISLR esperado 20, fue %v", ret.MontoRetenido)
	}
	a := asientoRetencion(t, svc, ret.ID)
	if !casi(debeCta(a, contabilidad.CtaCuentasPorPagar), 20) || !casi(haberCta(a, contabilidad.CtaISLRRetenidoPorEnterar), 20) {
		t.Fatalf("asiento esperado Debe 2101=20 / Haber 2204=20, fue %+v", a.Lineas)
	}
	if !a.Cuadra() {
		t.Fatalf("el asiento de la retención ISLR emitida no cuadra: %+v", a)
	}
	saldoDespues := deudaDeProveedor(t, svc, fc.ProveedorID)
	if !casi(saldoAntes-saldoDespues, 20) {
		t.Fatalf("la deuda por pagar debía bajar 20 (de %v a %v)", saldoAntes, saldoDespues)
	}
}

// TestRetencionISLR_SustraendoAgota: si el sustraendo iguala o supera la retención
// bruta, no hay monto que retener y se rechaza con ErrRetencionSinMonto (nunca un
// monto negativo).
func TestRetencionISLR_SustraendoAgota(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaCreditoIVA100(t, svc)
	_, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoISLR, NumeroComprobante: "R-ISLR-0",
		Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 3,
		Base: 100, Sustraendo: 50, // 100×3% = 3 < 50 → monto 0
	})
	if !errors.Is(err, application.ErrRetencionSinMonto) {
		t.Fatalf("se esperaba ErrRetencionSinMonto, se obtuvo: %v", err)
	}
}

// TestRetencionISLR_SinBase: la retención de ISLR exige una base gravable positiva.
func TestRetencionISLR_SinBase(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaCreditoIVA100(t, svc)
	_, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoISLR, NumeroComprobante: "R-ISLR-NB",
		Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 3, Base: 0,
	})
	if !errors.Is(err, application.ErrRetencionSinBase) {
		t.Fatalf("se esperaba ErrRetencionSinBase, se obtuvo: %v", err)
	}
}

// TestRetencion_IVAyISLRMismoDocumento: un mismo documento admite a la vez una
// retención de IVA y una de ISLR de la misma dirección (la unicidad es por tipo +
// impuesto + documento). Ambas bajan la CxC y el libro cuadra.
func TestRetencion_IVAyISLRMismoDocumento(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaCreditoIVA100(t, svc) // IVA 100, por cobrar 725
	saldo0, _ := svc.SaldoPorCobrar(empDemo, doc.ID)

	// IVA al 75% del IVA (100) → 75.
	if _, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoIVA, NumeroComprobante: "R-IVA",
		Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 75,
	}); err != nil {
		t.Fatalf("retención IVA: %v", err)
	}
	// ISLR sobre base 1000 al 3% → 30.
	if _, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoISLR, NumeroComprobante: "R-ISLR",
		Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 3, Base: 1000,
	}); err != nil {
		t.Fatalf("retención ISLR sobre el mismo documento: %v", err)
	}

	// Dos comprobantes sobre el mismo documento.
	n := 0
	for _, r := range svc.Retenciones(empDemo) {
		if r.DocumentoID == doc.ID {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("el documento debía tener 2 retenciones (IVA + ISLR), tiene %d", n)
	}

	// Reintentar el mismo par (dirección, impuesto) sí se rechaza como duplicado.
	if _, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoISLR, NumeroComprobante: "R-ISLR-2",
		Fecha: time.Now().UTC().Format(time.RFC3339Nano), Porcentaje: 3, Base: 1000,
	}); !errors.Is(err, application.ErrRetencionDuplicada) {
		t.Fatalf("se esperaba ErrRetencionDuplicada al repetir ISLR, se obtuvo: %v", err)
	}

	saldo1, _ := svc.SaldoPorCobrar(empDemo, doc.ID)
	if !casi(saldo0-saldo1, 105) { // 75 (IVA) + 30 (ISLR)
		t.Fatalf("el saldo por cobrar debía bajar 105 (de %v a %v)", saldo0, saldo1)
	}
	assertLibroCuadra(t, svc, empDemo)
}

// Un agente de retención EMITE con número autogenerado (AAAAMM+8) y % IVA por
// defecto; si la empresa NO es agente, no puede emitir.
func TestRetencionEmitida_NumeroAutogeneradoYGating(t *testing.T) {
	svc, st := nuevoServicio(t)
	tn := application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-901", "00-901", time.Now().UTC().Format(time.RFC3339Nano))

	// La demo es agente (seed): el sistema genera el número; sin % usa 75 por defecto.
	ret, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc.ID, application.EntradaRetencion{Fecha: "2026-08-15"})
	if err != nil {
		t.Fatalf("emitida: %v", err)
	}
	if len(ret.NumeroComprobante) != 14 || ret.NumeroComprobante[:6] != "202608" {
		t.Errorf("el número debe ser AAAAMM+8 (202608xxxxxxxx), fue %q", ret.NumeroComprobante)
	}
	if ret.Porcentaje != 75 {
		t.Errorf("IVA debe usar 75%% por defecto, fue %v", ret.Porcentaje)
	}

	// Deja de ser agente → no puede emitir sobre otra factura.
	if _, err := tn.ActualizarImpuestos(empDemo, actorA, origenTst, 0.16, 0.03, false, false, 0); err != nil {
		t.Fatalf("actualizar impuestos: %v", err)
	}
	fc2 := ocFacturada(t, svc, sku, "F-902", "00-902", time.Now().UTC().Format(time.RFC3339Nano))
	if _, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc2.ID, application.EntradaRetencion{Porcentaje: 75}); !errors.Is(err, application.ErrNoEsAgenteRetencion) {
		t.Errorf("sin ser agente debe dar ErrNoEsAgenteRetencion, fue %v", err)
	}
}
