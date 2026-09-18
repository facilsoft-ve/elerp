package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/inventario"
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

/* --- Devolución de mercancía al proveedor (NC con líneas) -------------------- */

// devolucionEntrada arma una nota de crédito que DEVUELVE mercancía: líneas con SKU
// y cantidad, sin monto (el servidor lo deriva del costo con que entró).
func devolucionEntrada(sku string, cantidad float64) application.NotaCompraEntrada {
	in := notaEntradaValida(0)
	in.Concepto = "devolución de mercancía"
	in.Lineas = []application.LineaNotaCompraEntrada{{SKU: sku, Cantidad: cantidad}}
	return in
}

// TestNotaCreditoCompra_ConLineasDevuelveMercanciaAlLedger es el test del defecto que
// motivó este cambio: una NC de devolución tiene que SACAR la mercancía del ledger,
// no solo rebajar la contabilidad. Antes, el almacén seguía contando las unidades y
// la cuenta 1201 ya no las tenía — descuadre silencioso hasta un ajuste manual.
func TestNotaCreditoCompra_ConLineasDevuelveMercanciaAlLedger(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	// La OC de ocFacturada recibe 10 u a Bs 20 (base 200, IVA 32 ⇒ alícuota 16%).
	fc := ocFacturada(t, svc, sku, "F-DEV-1", "00-00033333", time.Now().UTC().Format(time.RFC3339Nano))
	cant0, avg0 := existenciaDe(t, svc, empDemo, sede1, sku)
	cta0 := saldoContableCta(svc, empDemo, contabilidad.CtaInventario)

	nc, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 3))
	if err != nil {
		t.Fatalf("emitir NC con devolución: %v", err)
	}

	// (a) La base se DERIVA de las líneas: 3 u × Bs 20 = 60, IVA 9,60, total 69,60.
	if len(nc.Lineas) != 1 || !casi(nc.Lineas[0].Cantidad, 3) || !casi(nc.Lineas[0].CostoUnitario, 20) {
		t.Fatalf("la nota debe guardar la línea devuelta con su costo de entrada, se obtuvo %+v", nc.Lineas)
	}
	if !nc.EsDevolucion() {
		t.Error("una nota con líneas es una devolución")
	}
	casiEq(t, -nc.BaseImponible, 60, "base derivada de las líneas (3 × 20)")
	casiEq(t, -nc.IVA, 9.6, "IVA con la alícuota histórica de la factura (16%)")
	casiEq(t, -nc.Total, 69.6, "total de la devolución")

	// (b) EL PUNTO: la existencia baja por lo devuelto.
	cant1, avg1 := existenciaDe(t, svc, empDemo, sede1, sku)
	casiEq(t, cant0-cant1, 3, "la existencia baja por la mercancía devuelta")

	// (c) Y baja por un movimiento trazable hasta la nota, no por un ajuste anónimo.
	n, movido := movsDeRef(t, svc, empDemo, sede1, sku, inventario.MovSalida, nc.ID)
	if n != 1 {
		t.Fatalf("la devolución debía anexar 1 salida al ledger, hubo %d", n)
	}
	casiEq(t, movido, -3, "el movimiento de la devolución es negativo por lo devuelto")

	// (d) EL INVARIANTE que motivó todo esto: la cuenta de inventario y la
	// valorización del Kardex se mueven JUNTAS y por el mismo importe. La salida se
	// valora al promedio vigente (como una venta), no al precio de compra.
	a := asientosPorRef(svc, empDemo, "nota_compra", nc.ID)
	if len(a) != 1 {
		t.Fatalf("la NC debía derivar 1 asiento, hay %d", len(a))
	}
	valorQueSalio := round2Test(cant0*avg0 - cant1*avg1)
	casiEq(t, haberCta(a[0], contabilidad.CtaInventario), valorQueSalio,
		"la cuenta 1201 baja exactamente lo que bajó la valorización del inventario")
	// El saldo de la cuenta se compara aparte y con holgura: saldoContableCta redondea
	// con round2Test, que TRUNCA hacia cero, y el 1201 del fixture es negativo (el seed
	// carga existencias sin asentarlas), así que arrastra hasta un céntimo que no es
	// del asiento. El asiento, que es la fuente, sí cuadra al céntimo.
	cta1 := saldoContableCta(svc, empDemo, contabilidad.CtaInventario)
	if d := (cta0 - cta1) - valorQueSalio; d > 0.011 || d < -0.011 {
		t.Errorf("el saldo de 1201 se apartó de la valorización: delta cuenta %v vs valor que salió %v", cta0-cta1, valorQueSalio)
	}
	casiEq(t, nc.Lineas[0].CostoSalida, avg0, "la línea guarda el costo promedio con que salió")

	// La deuda baja por lo que el proveedor acredita; la diferencia contra el valor
	// que salió es resultado del período (5202), y el asiento cuadra igual.
	casiEq(t, debeCta(a[0], contabilidad.CtaCuentasPorPagar), 69.6, "devolución: total al debe de CxP")
	// La resta se compara tal cual: ambos lados salen ya redondeados del asiento, y
	// casi() absorbe el ruido de coma flotante.
	diferencia := haberCta(a[0], contabilidad.CtaDiferenciaEnCompras) - debeCta(a[0], contabilidad.CtaDiferenciaEnCompras)
	casiEq(t, valorQueSalio+diferencia, 60, "1201 + 5202 suman la base que acredita el proveedor")
	if !a[0].Cuadra() {
		t.Errorf("el asiento de la devolución no cuadra: %+v", a[0])
	}
	assertLibroCuadra(t, svc, empDemo)
}

// TestNotaCreditoCompra_SinLineasNoTocaElInventario: un descuento puro no mueve
// stock, y por eso su asiento va a 5202 y no a 1201. Si fuera a 1201, el balance se
// separaría de la valorización del inventario sin que nada avisara.
func TestNotaCreditoCompra_SinLineasNoTocaElInventario(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-DESC-1", "00-00055555", time.Now().UTC().Format(time.RFC3339Nano))
	cant0, _ := existenciaDe(t, svc, empDemo, sede1, sku)

	nc, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, notaEntradaValida(50))
	if err != nil {
		t.Fatalf("emitir NC de descuento: %v", err)
	}
	if nc.EsDevolucion() {
		t.Error("una nota sin líneas no es una devolución")
	}
	cant1, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	casiEq(t, cant1, cant0, "un descuento no mueve existencias")

	a := asientosPorRef(svc, empDemo, "nota_compra", nc.ID)
	if len(a) != 1 {
		t.Fatalf("la NC debía derivar 1 asiento, hay %d", len(a))
	}
	casiEq(t, haberCta(a[0], contabilidad.CtaDiferenciaEnCompras), 50, "descuento: base a Diferencia en compras 5202")
	casiEq(t, haberCta(a[0], contabilidad.CtaInventario), 0, "descuento: no toca Inventario 1201")
	assertLibroCuadra(t, svc, empDemo)
}

// TestNotaCreditoCompra_NoSeDevuelveMasDeLoRecibido tapa el agujero obvio: acreditar
// mercancía que el proveedor nunca entregó.
func TestNotaCreditoCompra_NoSeDevuelveMasDeLoRecibido(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-DEV-2", "00-00066666", time.Now().UTC().Format(time.RFC3339Nano))

	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 11)); !errors.Is(err, application.ErrDevolucionExcede) {
		t.Fatalf("devolver 11 de 10 recibidas debía dar ErrDevolucionExcede, se obtuvo: %v", err)
	}
}

// TestNotaCreditoCompra_DevolucionesSucesivasAcumulan: el tope es lo recibido MENOS
// lo ya devuelto en notas previas, no lo recibido a secas.
func TestNotaCreditoCompra_DevolucionesSucesivasAcumulan(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-DEV-3", "00-00077777", time.Now().UTC().Format(time.RFC3339Nano))

	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 6)); err != nil {
		t.Fatalf("primera devolución (6 de 10): %v", err)
	}
	// Quedan 4 devolvibles: pedir 5 se pasa.
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 5)); !errors.Is(err, application.ErrDevolucionExcede) {
		t.Fatalf("la segunda devolución debía respetar lo ya devuelto, se obtuvo: %v", err)
	}
	// Y 4 exactos sí entran.
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 4)); err != nil {
		t.Fatalf("devolver las 4 restantes: %v", err)
	}
}

// TestNotaCreditoCompra_NoSeDevuelveLoQueYaNoEstaEnElAlmacen: si la mercancía ya
// salió (se vendió, se transfirió, se ajustó), devolverla dejaría el ledger en
// negativo. Misma política que el despacho de una transferencia.
func TestNotaCreditoCompra_NoSeDevuelveLoQueYaNoEstaEnElAlmacen(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-DEV-4", "00-00088888", time.Now().UTC().Format(time.RFC3339Nano))

	// Vaciar la existencia de la sede con un ajuste (conteo físico que no encuentra nada).
	cant, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "conteo físico: no está", -cant, actorA, origenTst); err != nil {
		t.Fatalf("vaciar existencia: %v", err)
	}
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 1)); !errors.Is(err, application.ErrDevolucionSinStock) {
		t.Fatalf("devolver sin stock debía dar ErrDevolucionSinStock, se obtuvo: %v", err)
	}
}

// TestNotaCompra_DevolucionRechazaCasosImposibles agrupa las entradas que no tienen
// sentido de negocio y deben morir en la validación, antes de tocar nada.
func TestNotaCompra_DevolucionRechazaCasosImposibles(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-DEV-5", "00-00099998", time.Now().UTC().Format(time.RFC3339Nano))

	// (a) Una ND no devuelve mercancía: el stock entra por la recepción, no por un cargo.
	if _, err := svc.EmitirNotaDebitoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 1)); !errors.Is(err, application.ErrNotaDebitoConLineas) {
		t.Fatalf("una ND con líneas debía dar ErrNotaDebitoConLineas, se obtuvo: %v", err)
	}

	// (b) Monto y líneas a la vez: uno de los dos mentiría sobre el importe.
	conMonto := devolucionEntrada(sku, 1)
	conMonto.Monto = 999
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, conMonto); !errors.Is(err, application.ErrNotaCompraMontoYLineas) {
		t.Fatalf("monto + líneas debía dar ErrNotaCompraMontoYLineas, se obtuvo: %v", err)
	}

	// (c) Un producto que no está en la orden de esa factura.
	ajeno := ""
	for _, p := range svc.Productos(empDemo) {
		if p.SKU != sku && !p.EsCombo {
			ajeno = p.SKU
			break
		}
	}
	if ajeno == "" {
		t.Skip("el seed no trae un segundo producto stockeable")
	}
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(ajeno, 1)); !errors.Is(err, application.ErrDevolucionSKUAjeno) {
		t.Fatalf("un SKU fuera de la orden debía dar ErrDevolucionSKUAjeno, se obtuvo: %v", err)
	}

	// (d) Líneas en cero: se pidió devolver, pero no hay nada que devolver. El error
	// es el específico de la devolución, no el genérico de nota vacía.
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 0)); !errors.Is(err, application.ErrDevolucionVacia) {
		t.Fatalf("una devolución de cantidad 0 debía dar ErrDevolucionVacia, se obtuvo: %v", err)
	}
}

// TestReporteInventario_LaDevolucionNoCuentaComoRotacion: la devolución sale del
// ledger como cualquier salida, pero no se vendió. Contarla inflaría la rotación con
// mercancía que volvió atrás.
func TestReporteInventario_LaDevolucionNoCuentaComoRotacion(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-DEV-6", "00-00099997", time.Now().UTC().Format(time.RFC3339Nano))

	rotacionDe := func() float64 {
		for _, r := range svc.ReporteInventario(empDemo).Rotacion {
			if r.SKU == sku {
				return r.UnidadesVendidas
			}
		}
		return 0
	}
	antes := rotacionDe()
	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 4)); err != nil {
		t.Fatalf("emitir devolución: %v", err)
	}
	casiEq(t, rotacionDe(), antes, "la devolución al proveedor no es rotación de venta")
}
