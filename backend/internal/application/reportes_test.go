package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// TestReporteVentas_PliegaFacturaDelMes comprueba que una factura recién emitida
// aparece en el reporte de ventas del mes: suma a las ventas netas y al IVA, se
// cuenta como factura, y sus líneas caen en el desglose por producto.
func TestReporteVentas_PliegaFacturaDelMes(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)

	antes := svc.ReporteVentas(empDemo, "", "")

	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 2, PrecioUnitario: 50}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir factura: %v", err)
	}

	rep := svc.ReporteVentas(empDemo, "", "")
	if rep.CantidadFacturas != antes.CantidadFacturas+1 {
		t.Fatalf("la cantidad de facturas debería subir en 1: antes=%d ahora=%d", antes.CantidadFacturas, rep.CantidadFacturas)
	}
	if !casi(rep.VentasNetas, antes.VentasNetas+doc.Total) {
		t.Fatalf("las ventas netas no reflejan la factura: antes=%v ahora=%v total=%v", antes.VentasNetas, rep.VentasNetas, doc.Total)
	}
	if rep.TicketPromedio <= 0 {
		t.Fatalf("el ticket promedio debería ser positivo con facturas presentes: %v", rep.TicketPromedio)
	}
	// La factura del POS trae caja: canal Punto de venta.
	var pos bool
	for _, ch := range rep.PorCanal {
		if ch.Canal == "Punto de venta" && ch.Monto > 0 {
			pos = true
		}
	}
	if !pos {
		t.Fatalf("la venta del mostrador debería contarse en el canal Punto de venta: %+v", rep.PorCanal)
	}
	// El SKU vendido aparece en el desglose por producto.
	var hallado bool
	for _, p := range rep.PorProducto {
		if p.SKU == sku {
			hallado = true
		}
	}
	if !hallado {
		t.Fatalf("el SKU %s vendido no aparece en el desglose por producto", sku)
	}
}

// TestReporteVentas_NotaCreditoResta verifica que una nota de crédito baja las
// ventas netas del período (las reversas restan).
func TestReporteVentas_NotaCreditoResta(t *testing.T) {
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
	antes := svc.ReporteVentas(empDemo, "", "").VentasNetas

	if _, err := svc.EmitirNotaCredito(empDemo, sede1, actorA, origenTst, doc.ID, "devolución parcial",
		[]application.LineaEntrada{{SKU: sku, Cantidad: 1}}); err != nil {
		t.Fatalf("emitir nota de crédito: %v", err)
	}

	despues := svc.ReporteVentas(empDemo, "", "").VentasNetas
	if despues >= antes {
		t.Fatalf("las ventas netas deberían bajar tras la NC: antes=%v después=%v", antes, despues)
	}
}

// TestReporteInventario_ValorizaYCuentaFaltantes comprueba que el reporte de
// inventario valoriza el stock y cuenta productos, sin números negativos raros.
func TestReporteInventario_ValorizaYCuentaFaltantes(t *testing.T) {
	svc, _ := nuevoServicio(t)
	rep := svc.ReporteInventario(empDemo)
	if rep.NumeroProductos == 0 {
		t.Fatalf("el seed demo debería traer productos")
	}
	if rep.ValorizacionTotal < 0 {
		t.Fatalf("la valorización no debería ser negativa: %v", rep.ValorizacionTotal)
	}
	if rep.BajoMinimo+rep.Agotados > rep.NumeroProductos {
		t.Fatalf("bajo mínimo + agotados no puede exceder el total de productos: %d+%d > %d",
			rep.BajoMinimo, rep.Agotados, rep.NumeroProductos)
	}
}

// TestReporteCobranza_AgingSumaAlTotal verifica que la suma de los tramos del
// aging cuadra con el total por cobrar del resumen.
func TestReporteCobranza_AgingSumaAlTotal(t *testing.T) {
	svc, _ := nuevoServicio(t)
	rep := svc.ReporteCobranza(empDemo)
	var suma float64
	for _, tr := range rep.Aging {
		suma += tr.Monto
	}
	if !casi(suma, rep.TotalPorCobrar) {
		t.Fatalf("la suma del aging debería igualar el total por cobrar: %v vs %v", suma, rep.TotalPorCobrar)
	}
	if len(rep.Aging) != 4 {
		t.Fatalf("el aging debería tener 4 tramos, tiene %d", len(rep.Aging))
	}
}

// TestPanelEjecutivo_Consolida comprueba que el panel arma un objeto coherente
// reutilizando los demás reportes.
func TestPanelEjecutivo_Consolida(t *testing.T) {
	svc, _ := nuevoServicio(t)
	panel := svc.PanelEjecutivo(empDemo)
	if panel.Inventario.NumeroProductos == 0 {
		t.Fatalf("el panel debería incluir el inventario del seed")
	}
	if panel.Ventas.Desde == "" || panel.Ventas.Hasta == "" {
		t.Fatalf("el panel debería fijar el rango de ventas del mes: %+v", panel.Ventas)
	}
}
