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
