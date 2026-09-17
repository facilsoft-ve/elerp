package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* Notas de crédito por DESCUENTO y por AJUSTE DE PRECIO (notas de la contadora,
 * 15:18 y 15:19).
 *
 * Lo que de verdad se prueba acá: que NO muevan inventario. La NC solo sabía
 * devolver mercancía y reingresaba stock siempre; usarla para un descuento
 * habría inflado el inventario en silencio — nadie devolvió nada, solo se cobró
 * de más. */

// facturaParaNC emite una factura de contado con un producto gravado conocido.
func facturaParaNC(t *testing.T, svc *application.Service, sku string, cant, precio float64) fiscal.Documento {
	t.Helper()
	abrirTurno(t, svc, actorA)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: cant, PrecioUnitario: precio}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 100000, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	return doc
}

// stockDe es la existencia del SKU en la sede, para comprobar que NO se mueva.
// Reusa el helper que ya existe en inventario_importar_test.go.
func stockDe(svc *application.Service, sku string) float64 {
	n, _ := existenciaDeSKU(svc, sku)
	return n
}

// LA prueba del lote: un descuento NO devuelve mercancía.
func TestNotaCreditoDescuento_NoTocaElInventario(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 2, 100)
	antes := stockDe(svc, sku)

	nc, err := svc.NotaCreditoDescuento(empDemo, actorA, origenTst, doc.ID, 50, false, "pronto pago")
	if err != nil {
		t.Fatalf("descuento: %v", err)
	}
	if despues := stockDe(svc, sku); !casi(despues, antes) {
		t.Errorf("el descuento NO puede devolver mercancía: existencia %v → %v", antes, despues)
	}
	if nc.MotivoCodigo != fiscal.MotivoNCDescuento {
		t.Errorf("motivo mal grabado: %q", nc.MotivoCodigo)
	}
	// 50 de base + 16% = 58 acreditados, en negativo.
	if !casi(nc.Total, -58) {
		t.Errorf("total = %v, se esperaban -58 (50 + IVA)", nc.Total)
	}
}

// El ajuste de precio acredita la DIFERENCIA por la cantidad facturada, y
// tampoco devuelve mercancía.
func TestNotaCreditoAjustePrecio_AcreditaLaDiferencia(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 3, 100) // se facturó a 100; correspondía 80
	antes := stockDe(svc, sku)

	nc, err := svc.NotaCreditoAjustePrecio(empDemo, actorA, origenTst, doc.ID,
		[]application.AjustePrecioLinea{{SKU: sku, PrecioCorrecto: 80}}, "")
	if err != nil {
		t.Fatalf("ajuste: %v", err)
	}
	if despues := stockDe(svc, sku); !casi(despues, antes) {
		t.Errorf("el ajuste de precio NO devuelve mercancía: %v → %v", antes, despues)
	}
	// (100 − 80) × 3 = 60 de base, +16% = 69,60.
	if !casi(nc.BaseImponible, -60) || !casi(nc.Total, -69.6) {
		t.Errorf("base %v / total %v, se esperaban -60 y -69,60", nc.BaseImponible, nc.Total)
	}
}

// Acreditar HACIA ARRIBA no existe: cobrar de más se corrige con una nota de
// DÉBITO. Si esto se permitiera, una NC aumentaría la deuda del cliente.
func TestNotaCreditoAjustePrecio_RechazaPrecioMayor(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 1, 100)

	_, err := svc.NotaCreditoAjustePrecio(empDemo, actorA, origenTst, doc.ID,
		[]application.AjustePrecioLinea{{SKU: sku, PrecioCorrecto: 120}}, "")
	if !errors.Is(err, application.ErrNCAjustePrecio) {
		t.Fatalf("se esperaba ErrNCAjustePrecio, se obtuvo: %v", err)
	}
}

// Dos descuentos sucesivos no pueden dejar la factura en negativo.
func TestNotaCreditoDescuento_NoSuperaLoFacturado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 1, 100) // total 116

	if _, err := svc.NotaCreditoDescuento(empDemo, actorA, origenTst, doc.ID, 80, false, ""); err != nil {
		t.Fatalf("primer descuento: %v", err)
	}
	// Ya se acreditaron 92,80 de 116: quedan 23,20.
	if _, err := svc.NotaCreditoDescuento(empDemo, actorA, origenTst, doc.ID, 80, false, ""); !errors.Is(err, application.ErrNCDescuentoExcede) {
		t.Fatalf("se esperaba ErrNCDescuentoExcede, se obtuvo: %v", err)
	}
}

func TestNotaCreditoDescuento_RechazaMontoCero(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := facturaParaNC(t, svc, primerSKU(t, svc), 1, 100)
	for _, m := range []float64{0, -10} {
		if _, err := svc.NotaCreditoDescuento(empDemo, actorA, origenTst, doc.ID, m, false, ""); !errors.Is(err, application.ErrNCDescuentoMonto) {
			t.Errorf("monto %v debería rechazarse, dio: %v", m, err)
		}
	}
}

// Un descuento NO puede consumir el cupo de una devolución real posterior: son
// cosas distintas y el guard anti-sobre-crédito cuenta CANTIDADES devueltas.
func TestNotaCreditoDescuento_NoBloqueaUnaDevolucionPosterior(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 2, 100)

	if _, err := svc.NotaCreditoDescuento(empDemo, actorA, origenTst, doc.ID, 20, false, ""); err != nil {
		t.Fatalf("descuento: %v", err)
	}
	// Las 2 unidades siguen disponibles para devolver.
	if _, err := svc.EmitirNotaCredito(empDemo, sede1, actorA, origenTst, doc.ID, "devolución",
		[]application.LineaEntrada{{SKU: sku, Cantidad: 2}}); err != nil {
		t.Fatalf("la devolución posterior debe poder: %v", err)
	}
}

// Y al revés: la devolución SÍ reingresa stock. Es el contraste que le da
// sentido a todo lo anterior.
func TestNotaCreditoDevolucion_SiReingresaStock(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 2, 100)
	antes := stockDe(svc, sku)

	if _, err := svc.EmitirNotaCredito(empDemo, sede1, actorA, origenTst, doc.ID, "devolución",
		[]application.LineaEntrada{{SKU: sku, Cantidad: 2}}); err != nil {
		t.Fatalf("devolución: %v", err)
	}
	if despues := stockDe(svc, sku); !casi(despues, antes+2) {
		t.Errorf("la devolución sí reingresa: %v → %v (se esperaba %v)", antes, despues, antes+2)
	}
}

func TestMotivosDeNota_CatalogosCoherentes(t *testing.T) {
	svc, _ := nuevoServicio(t)
	nc, nd := svc.MotivosDeNota()
	if len(nc) == 0 || len(nd) == 0 {
		t.Fatal("los catálogos no pueden venir vacíos")
	}
	// SOLO la devolución mueve inventario: si otro motivo lo hiciera, se repetiría
	// el bug que esto vino a cerrar.
	for _, m := range nc {
		if m.MueveInventario && m.Codigo != fiscal.MotivoNCDevolucion {
			t.Errorf("%q no debería mover inventario", m.Codigo)
		}
		if m.Nombre == "" || m.Ayuda == "" {
			t.Errorf("%q: sin nombre o sin ayuda, la pantalla no puede explicarlo", m.Codigo)
		}
	}
	// Los dos usos que declaró la contadora tienen que estar.
	tiene := func(cat []fiscal.MotivoNota, cod string) bool {
		for _, m := range cat {
			if m.Codigo == cod {
				return true
			}
		}
		return false
	}
	if !tiene(nd, fiscal.MotivoNDDiferencialCambiario) || !tiene(nd, fiscal.MotivoNDCobroDeMenos) {
		t.Error("faltan los motivos de ND que declaró la contadora")
	}
}
