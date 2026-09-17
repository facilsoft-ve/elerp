package application_test

import (
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// mesActual devuelve el anio/mes UTC de hoy, que es cuando los servicios sellan
// la Fecha (ahora()) de los documentos que emiten en la prueba.
func mesActual() (int, int) {
	n := time.Now().UTC()
	return n.Year(), int(n.Month())
}

// TestLibroVentas_PliegaFacturasDelMes verifica que el Libro de Ventas recoge
// las facturas del período y suma sus bases e impuestos en la fila de totales.
func TestLibroVentas_PliegaFacturasDelMes(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc) // producto gravado
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 2, PrecioUnitario: 50}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir factura: %v", err)
	}

	anio, mes := mesActual()
	lv := svc.LibroVentas(empDemo, anio, mes)
	if len(lv.Filas) == 0 {
		t.Fatalf("el libro de ventas debería contener la factura recién emitida")
	}
	var hallada *application.LibroVentasFila
	for i := range lv.Filas {
		if lv.Filas[i].NumeroCompleto == doc.NumeroCompleto {
			hallada = &lv.Filas[i]
		}
	}
	if hallada == nil {
		t.Fatalf("no se encontró la factura %s en el libro", doc.NumeroCompleto)
	}
	if !casi(hallada.BaseImponible, doc.BaseImponible) || !casi(hallada.IVADebito, doc.IVA) {
		t.Fatalf("la fila no refleja las bases del documento: fila=%+v doc IVA=%v base=%v", hallada, doc.IVA, doc.BaseImponible)
	}
	if hallada.Alicuota <= 0 {
		t.Fatalf("la alícuota debería tener un default fiscal, se obtuvo %v", hallada.Alicuota)
	}
	// La fila de totales suma al menos esta factura.
	if !casi(lv.Totales.IVADebito, sumaIVA(lv.Filas)) {
		t.Fatalf("el total de IVA débito no cuadra con la suma de filas: %v vs %v", lv.Totales.IVADebito, sumaIVA(lv.Filas))
	}
}

// TestLibroVentas_NotaCreditoResta comprueba que una nota de crédito entra en el
// libro con signo negativo y reduce los totales (es una sustracción del débito).
func TestLibroVentas_NotaCreditoResta(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 2, PrecioUnitario: 50}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir factura: %v", err)
	}
	// Débito antes de la NC (el seed demo ya trae documentos del mes; medimos el
	// DELTA que introduce la nota de crédito, no un absoluto).
	anio, mes := mesActual()
	antes := svc.LibroVentas(empDemo, anio, mes).Totales.IVADebito

	nc, err := svc.EmitirNotaCredito(empDemo, sede1, actorA, origenTst, doc.ID, "devolución parcial",
		[]application.LineaEntrada{{SKU: sku, Cantidad: 1}})
	if err != nil {
		t.Fatalf("emitir nota de crédito: %v", err)
	}
	if nc.Total >= 0 {
		t.Fatalf("la NC debería guardar montos negativos, Total=%v", nc.Total)
	}

	lv := svc.LibroVentas(empDemo, anio, mes)
	var ncFila *application.LibroVentasFila
	for i := range lv.Filas {
		if lv.Filas[i].NumeroCompleto == nc.NumeroCompleto {
			ncFila = &lv.Filas[i]
		}
	}
	if ncFila == nil {
		t.Fatalf("la nota de crédito %s no aparece en el libro", nc.NumeroCompleto)
	}
	if ncFila.Tipo != fiscal.TipoNotaCredito {
		t.Fatalf("la fila de la NC tiene tipo inesperado: %q", ncFila.Tipo)
	}
	if ncFila.IVADebito >= 0 || ncFila.Total >= 0 {
		t.Fatalf("la NC debe conservar su signo negativo en el libro: %+v", ncFila)
	}
	// El débito neto del mes baja exactamente en el IVA (negativo) de la NC: la
	// nota de crédito es una sustracción del débito fiscal.
	if !casi(lv.Totales.IVADebito, antes+ncFila.IVADebito) {
		t.Fatalf("el débito neto no bajó por la NC: antes=%v después=%v ncIVA=%v", antes, lv.Totales.IVADebito, ncFila.IVADebito)
	}
	if lv.Totales.IVADebito >= antes {
		t.Fatalf("el débito neto debería bajar tras la NC: antes=%v después=%v", antes, lv.Totales.IVADebito)
	}
}

// TestLibroVentas_FiltraPorPeriodo comprueba que un mes sin documentos devuelve
// el estado vacío honesto (sin filas, totales en cero).
func TestLibroVentas_FiltraPorPeriodo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 10}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 11.6, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("emitir factura: %v", err)
	}
	// Un año lejano al pasado: no debería haber nada.
	lv := svc.LibroVentas(empDemo, 2000, 1)
	if len(lv.Filas) != 0 {
		t.Fatalf("un período sin documentos debería venir vacío, trajo %d filas", len(lv.Filas))
	}
	if lv.Totales.IVADebito != 0 || lv.Totales.Total != 0 {
		t.Fatalf("los totales de un período vacío deberían ser cero: %+v", lv.Totales)
	}
}

// TestLibroCompras_PliegaFacturaRegistrada verifica que el Libro de Compras
// recoge una FACTURA de compra registrada (con nº de control) y su crédito
// fiscal, y que una orden recibida SIN factura no aparece (no es compra fiscal).
func TestLibroCompras_PliegaFacturaRegistrada(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)

	// Orden 1: recibida y facturada → sí aparece en el libro.
	fc := ocFacturada(t, svc, sku, "F-000123", "00-00012345", time.Now().UTC().Format(time.RFC3339Nano))

	// Orden 2: recibida pero SIN factura → no es compra fiscal, no aparece.
	ocSinFactura := ocRecibida(t, svc, sku, 5, 20)

	anio, mes := mesActual()
	lc := svc.LibroCompras(empDemo, anio, mes)
	var fila *application.LibroComprasFila
	for i := range lc.Filas {
		if lc.Filas[i].NumeroControl == fc.NumeroControl {
			fila = &lc.Filas[i]
		}
		if lc.Filas[i].NumeroFactura == "" {
			t.Fatalf("una fila del libro de compras sin nº de factura no debería existir: %+v", lc.Filas[i])
		}
	}
	if fila == nil {
		t.Fatalf("la factura de compra %s no aparece en el libro", fc.NumeroControl)
	}
	if fila.ProveedorRIF != "J-00012345-4" {
		t.Fatalf("el RIF del proveedor no se resolvió: %q", fila.ProveedorRIF)
	}
	if !casi(fila.IVACreditoFiscal, fc.IVA) || !casi(fila.BaseImponible, fc.BaseImponible) {
		t.Fatalf("la fila no refleja la factura: fila=%+v factura IVA=%v base=%v", fila, fc.IVA, fc.BaseImponible)
	}
	if !casi(lc.Totales.IVACreditoFiscal, fila.IVACreditoFiscal) {
		t.Fatalf("el total de crédito fiscal no cuadra: %v vs %v", lc.Totales.IVACreditoFiscal, fila.IVACreditoFiscal)
	}
	// La orden recibida sin factura no debe estar en el libro.
	if len(lc.Filas) != 1 {
		t.Fatalf("solo la orden facturada debería declararse, hay %d filas (oc sin factura=%s)", len(lc.Filas), ocSinFactura.NumeroCompleto)
	}
}

func sumaIVA(filas []application.LibroVentasFila) float64 {
	var s float64
	for _, f := range filas {
		s += f.IVADebito
	}
	// Redondeo a 2 decimales, igual que el servicio.
	return float64(int64(s*100+0.5)) / 100
}

/* --- Lo que la contadora encontró en la primera revisión -------------------
 *
 * Tres columnas del libro salían SIEMPRE vacías aunque el dato existiera, y el
 * libro declaraba una sola alícuota cuando en Venezuela conviven tres. */

// El número de control es OBLIGATORIO en el libro de ventas. El documento
// siempre lo llevó (se asigna al emitir), pero la fila del libro no lo copiaba y
// la columna del archivo salía en blanco.
func TestLibroVentas_LlevaElNumeroDeControl(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if doc.NumeroControl == "" {
		t.Fatal("el escenario necesita un documento CON número de control")
	}
	anio, mes := mesActual()
	fila := filaVenta(t, svc.LibroVentas(empDemo, anio, mes), doc.NumeroCompleto)
	if fila.NumeroControl != doc.NumeroControl {
		t.Errorf("el libro debe declarar el nº de control %q, trae %q", doc.NumeroControl, fila.NumeroControl)
	}
}

// filaVenta localiza una fila del libro de ventas por su número de documento.
func filaVenta(t *testing.T, lv application.LibroVentasResult, numero string) application.LibroVentasFila {
	t.Helper()
	for _, f := range lv.Filas {
		if f.NumeroCompleto == numero {
			return f
		}
	}
	t.Fatalf("no se encontró %s en el libro de ventas", numero)
	return application.LibroVentasFila{}
}

// El SENIAT declara cada tasa en SU columna: el 16 %, el 8 % y el recargo
// suntuario no se pueden sumar. Antes el libro traía una sola alícuota y era
// imposible llenar la declaración de una empresa con productos de lujo.
func TestLibroVentas_DesglosaPorAlicuota(t *testing.T) {
	svc, _ := servicioConImpuestos(t)
	productoConAlicuota(t, svc, "LIB-GEN", fiscal.CodGeneral, 100)
	productoConAlicuota(t, svc, "LIB-RED", fiscal.CodReducida, 200)
	productoConAlicuota(t, svc, "LIB-LUJO", fiscal.CodSuntuario, 300)

	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{
			{SKU: "LIB-GEN", Cantidad: 1, PrecioUnitario: 100},
			{SKU: "LIB-RED", Cantidad: 1, PrecioUnitario: 200},
			{SKU: "LIB-LUJO", Cantidad: 1, PrecioUnitario: 300},
		},
		// Base 600 + IVA (64 general + 16 reducida + 45 recargo) = 725.
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 725, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	anio, mes := mesActual()
	f := filaVenta(t, svc.LibroVentas(empDemo, anio, mes), doc.NumeroCompleto)

	// General: 100 (el suntuario también tributa general) + 300 = 400 al 16 %.
	if !casi(f.BaseGeneral, 400) || !casi(f.IVAGeneral, 64) {
		t.Errorf("general: base %v / IVA %v; se esperaban 400 y 64", f.BaseGeneral, f.IVAGeneral)
	}
	if !casi(f.BaseReducida, 200) || !casi(f.IVAReducida, 16) {
		t.Errorf("reducida: base %v / IVA %v; se esperaban 200 y 16", f.BaseReducida, f.IVAReducida)
	}
	// El recargo suntuario va APARTE, sobre la base del artículo de lujo.
	if !casi(f.BaseAdicional, 300) || !casi(f.IVAAdicional, 45) {
		t.Errorf("adicional: base %v / IVA %v; se esperaban 300 y 45", f.BaseAdicional, f.IVAAdicional)
	}
	// Y el agregado sigue cuadrando con la suma de las porciones.
	if !casi(f.IVADebito, f.IVAGeneral+f.IVAReducida+f.IVAAdicional) {
		t.Errorf("el IVA agregado (%v) debe ser la suma del desglose", f.IVADebito)
	}
}

// Un documento anterior al maestro de impuestos NO trae desglose. Si el libro no
// lo respaldara, todo el histórico saldría en cero y dejaría de cuadrar.
func TestLibroVentas_DocumentoSinDesgloseCaeAGeneral(t *testing.T) {
	svc, _ := nuevoServicio(t) // sin maestro: el motor no sella Impuestos
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	anio, mes := mesActual()
	f := filaVenta(t, svc.LibroVentas(empDemo, anio, mes), doc.NumeroCompleto)
	if !casi(f.BaseGeneral, doc.BaseImponible) || !casi(f.IVAGeneral, doc.IVA) {
		t.Errorf("sin desglose, todo va a la general: base %v / IVA %v (doc: %v / %v)",
			f.BaseGeneral, f.IVAGeneral, doc.BaseImponible, doc.IVA)
	}
	if !casi(f.BaseReducida, 0) || !casi(f.BaseAdicional, 0) {
		t.Errorf("no puede inventar bases reducida (%v) ni adicional (%v)", f.BaseReducida, f.BaseAdicional)
	}
}

// Lo que retuvo el cliente agente de retención es parte de la declaración del
// período. Sin esta columna la contadora tenía que cruzarlo a mano contra los
// comprobantes.
func TestLibroVentas_DeclaraElIVAQueRetuvoElCliente(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := ventaCreditoIVA100(t, svc) // IVA de 100
	if _, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoIVA, NumeroComprobante: "20260900012345", Porcentaje: 75,
	}); err != nil {
		t.Fatalf("registrar retención recibida: %v", err)
	}
	anio, mes := mesActual()
	lv := svc.LibroVentas(empDemo, anio, mes)
	f := filaVenta(t, lv, doc.NumeroCompleto)
	if !casi(f.IVARetenido, 75) {
		t.Errorf("el libro debe declarar 75 de IVA retenido, trae %v", f.IVARetenido)
	}
	if !casi(lv.Totales.IVARetenido, 75) {
		t.Errorf("el total de retenido debe sumar 75, es %v", lv.Totales.IVARetenido)
	}
}

// La columna IVARetenido del libro de COMPRAS salía siempre vacía aunque el
// comprobante existiera: es obligatoria para el contribuyente especial.
func TestLibroCompras_DeclaraElIVARetenidoAlProveedor(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	hoy := time.Now().UTC().Format("2006-01-02")
	fc := ocFacturada(t, svc, sku, "F-00123", "00-0099887", hoy)
	if _, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoIVA, Porcentaje: 75,
	}); err != nil {
		t.Fatalf("registrar retención emitida: %v", err)
	}
	anio, mes := mesActual()
	lc := svc.LibroCompras(empDemo, anio, mes)
	var f application.LibroComprasFila
	for _, x := range lc.Filas {
		if x.NumeroFactura == "F-00123" {
			f = x
		}
	}
	if f.NumeroFactura == "" {
		t.Fatal("la factura de compra no apareció en el libro")
	}
	esperado := round2Test(fc.IVA * 0.75)
	if !casi(f.IVARetenido, esperado) {
		t.Errorf("el libro debe declarar %v de IVA retenido, trae %v", esperado, f.IVARetenido)
	}
	if !casi(lc.Totales.IVARetenido, esperado) {
		t.Errorf("el total de retenido debe sumar %v, es %v", esperado, lc.Totales.IVARetenido)
	}
}

// La alícuota del libro de compras sale de la PROPIA factura del proveedor, no
// de la configurada por la empresa: si el proveedor facturó al 8 %, declarar el
// 16 % sería declarar algo que no ocurrió.
func TestLibroCompras_AlicuotaSaleDeLaFacturaNoDeLaEmpresa(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	hoy := time.Now().UTC().Format("2006-01-02")
	fc := ocFacturada(t, svc, sku, "F-00777", "00-0077777", hoy)
	anio, mes := mesActual()
	lc := svc.LibroCompras(empDemo, anio, mes)
	for _, f := range lc.Filas {
		if f.NumeroFactura != "F-00777" {
			continue
		}
		if fc.BaseImponible <= 0 {
			t.Skip("la factura del escenario no tiene base gravada")
		}
		efectiva := fc.IVA / fc.BaseImponible
		if diff := f.Alicuota - efectiva; diff > 0.001 || diff < -0.001 {
			t.Errorf("la alícuota debe derivarse de la factura (%.4f), trae %.4f", efectiva, f.Alicuota)
		}
		// Y la fila cae entera en la columna general con esa tasa.
		if !casi(f.BaseGeneral, fc.BaseImponible) || !casi(f.IVAGeneral, fc.IVA) {
			t.Errorf("la base debe declararse en la columna general: %v / %v", f.BaseGeneral, f.IVAGeneral)
		}
		return
	}
	t.Fatal("no se encontró la factura en el libro")
}
