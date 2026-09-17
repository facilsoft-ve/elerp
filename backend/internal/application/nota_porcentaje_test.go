package application_test

import (
	"math"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* PORCENTAJE EN LAS NOTAS DE CRÉDITO Y DÉBITO.
 *
 * Un descuento o un recargo se pacta en porcentaje —«10 % por pronto pago»,
 * «5 % de mora»—, no en bolívares. Antes solo se aceptaba el monto ya calculado,
 * así que alguien sacaba la regla de tres en una calculadora: ahí se cuela el
 * céntimo que no cuadra contra la factura, y nadie puede reconstruir después de
 * qué porcentaje salió ese número.
 *
 * Lo que se prueba acá es la aritmética que hace que el porcentaje sea
 * CONFIABLE: sobre qué base cae, cómo se reparte entre gravado y exento, y que
 * no se pueda acreditar más de lo cobrado. */

// dosSKU toma un producto GRAVADO y uno EXENTO del seed. Se necesitan los dos
// porque el reparto proporcional del porcentaje solo se puede comprobar sobre
// una factura mixta: con una sola base el reparto es trivial y no prueba nada.
func dosSKU(t *testing.T, svc *application.Service) [2]string {
	t.Helper()
	var gravado, exento string
	for _, p := range svc.Productos(empDemo) {
		if p.ExentoIVA && exento == "" {
			exento = p.SKU
		}
		if !p.ExentoIVA && gravado == "" {
			gravado = p.SKU
		}
	}
	if gravado == "" || exento == "" {
		t.Fatal("el seed debería traer un producto gravado y uno exento")
	}
	return [2]string{gravado, exento}
}

// facturaMixtaParaNota emite una factura con la base gravada y la exenta
// indicadas, cada una en su renglón.
func facturaMixtaParaNota(t *testing.T, svc *application.Service, baseGravada, baseExenta float64) fiscal.Documento {
	t.Helper()
	abrirTurno(t, svc, actorA)
	skus := dosSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{
			{SKU: skus[0], Cantidad: 1, PrecioUnitario: baseGravada},
			{SKU: skus[1], Cantidad: 1, PrecioUnitario: baseExenta},
		},
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 100000, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir factura mixta: %v", err)
	}
	return doc
}

// LA prueba de fondo: el % va sobre la BASE y el IVA se recalcula encima, lo que
// da exactamente el mismo % sobre el TOTAL. Si alguien lo aplicara sobre el
// total y volviera a sumar IVA, se descontaría el impuesto dos veces.
func TestNCDescuentoPorcentaje_EsElMismoPorcentajeDelTotal(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 2, 100) // base 200 + IVA

	nc, err := svc.NotaCreditoDescuentoDe(empDemo, actorA, origenTst, doc.ID,
		application.DescuentoNC{Porcentaje: 10, Nota: "pronto pago"})
	if err != nil {
		t.Fatalf("descuento 10%%: %v", err)
	}
	if !casi(nc.BaseImponible, -20) {
		t.Fatalf("base acreditada = %.4f, se esperaba 10%% de 200 = 20", nc.BaseImponible)
	}
	// El total de la nota tiene que ser el 10 % del total de la factura.
	esperado := doc.Total * 0.10
	if !casi(math.Abs(nc.Total), esperado) {
		t.Fatalf("total NC = %.4f, se esperaba el 10%% del total facturado = %.4f", math.Abs(nc.Total), esperado)
	}
}

// El porcentaje sobre un PRODUCTO cae sobre ese renglón, no sobre la factura.
func TestNCDescuentoPorcentaje_SobreUnProducto(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	skus := dosSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{
			{SKU: skus[0], Cantidad: 1, PrecioUnitario: 100},
			{SKU: skus[1], Cantidad: 1, PrecioUnitario: 900},
		},
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 100000, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}

	nc, err := svc.NotaCreditoDescuentoDe(empDemo, actorA, origenTst, doc.ID,
		application.DescuentoNC{Porcentaje: 50, SKU: skus[0], Nota: "acuerdo"})
	if err != nil {
		t.Fatalf("descuento por producto: %v", err)
	}
	// 50 % de 100, no 50 % de 1000: acotarlo al renglón es todo el punto.
	if !casi(math.Abs(nc.Subtotal), 50) {
		t.Fatalf("subtotal = %.4f, se esperaba 50 (50%% del renglón de 100)", math.Abs(nc.Subtotal))
	}
}

// Sobre una factura MIXTA el porcentaje se reparte en proporción. Cargarlo todo a
// una sola base cambiaría el IVA de la nota y descuadraría el libro.
func TestNCDescuentoPorcentaje_RepartePorBaseGravadaYExenta(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := facturaMixtaParaNota(t, svc, 200, 100) // 200 gravado + 100 exento

	nc, err := svc.NotaCreditoDescuentoDe(empDemo, actorA, origenTst, doc.ID,
		application.DescuentoNC{Porcentaje: 10, Nota: "descuento global"})
	if err != nil {
		t.Fatalf("descuento: %v", err)
	}
	if !casi(math.Abs(nc.BaseImponible), 20) || !casi(math.Abs(nc.BaseExenta), 10) {
		t.Fatalf("reparto = gravado %.4f / exento %.4f, se esperaba 20 / 10",
			math.Abs(nc.BaseImponible), math.Abs(nc.BaseExenta))
	}
	// Y el IVA cae SOLO sobre la parte gravada.
	if !casi(math.Abs(nc.IVA), 20*doc.AlicuotaIVA) {
		t.Fatalf("IVA = %.4f, se esperaba el de 20 gravados", math.Abs(nc.IVA))
	}
}

// Un porcentaje sobre un producto no puede exceder ese producto: el tope de la NC
// («no se acredita más de lo que se cobró») tiene que seguir vigente.
func TestNCDescuentoPorcentaje_NoSuperaElRenglon(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 1, 100)

	// 100 % está justo en el tope: tiene que pasar.
	if _, err := svc.NotaCreditoDescuentoDe(empDemo, actorA, origenTst, doc.ID,
		application.DescuentoNC{Porcentaje: 100, SKU: sku, Nota: "todo"}); err != nil {
		t.Fatalf("100%% del renglón debería poder acreditarse: %v", err)
	}
	// Y un monto por encima del renglón, no.
	if _, err := svc.NotaCreditoDescuentoDe(empDemo, actorA, origenTst, doc.ID,
		application.DescuentoNC{Monto: 150, SKU: sku, Nota: "de más"}); err == nil {
		t.Fatal("acreditar 150 sobre un renglón de 100 debería fallar")
	}
}

// Monto Y porcentaje a la vez es ambiguo: elegir por el usuario daría un
// descuento distinto del pactado, así que se rechaza.
func TestNCDescuento_RechazaMontoYPorcentajeJuntos(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 1, 100)

	if _, err := svc.NotaCreditoDescuentoDe(empDemo, actorA, origenTst, doc.ID,
		application.DescuentoNC{Monto: 10, Porcentaje: 10, Nota: "ambiguo"}); err == nil {
		t.Fatal("monto y porcentaje juntos deberían rechazarse")
	}
	if _, err := svc.NotaCreditoDescuentoDe(empDemo, actorA, origenTst, doc.ID,
		application.DescuentoNC{Porcentaje: 120, Nota: "imposible"}); err == nil {
		t.Fatal("un porcentaje mayor que 100 debería rechazarse")
	}
}

// El descuento porcentual TAMPOCO mueve inventario: nadie devolvió nada.
func TestNCDescuentoPorcentaje_NoTocaElInventario(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 2, 100)
	antes := stockDe(svc, sku)

	if _, err := svc.NotaCreditoDescuentoDe(empDemo, actorA, origenTst, doc.ID,
		application.DescuentoNC{Porcentaje: 25, Nota: "acuerdo"}); err != nil {
		t.Fatalf("descuento: %v", err)
	}
	if despues := stockDe(svc, sku); !casi(antes, despues) {
		t.Fatalf("el descuento movió el stock: %.4f → %.4f", antes, despues)
	}
}

// --- NOTA DE DÉBITO -------------------------------------------------------

// El % de la ND cae sobre la misma base y suma, en vez de restar.
func TestNDPorcentaje_CargaSobreElTotal(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 2, 100) // base 200

	nd, err := svc.EmitirNotaDebito(empDemo, sede1, actorA, origenTst, doc.ID,
		application.NotaDebitoEntrada{Concepto: "Diferencial cambiario", Porcentaje: 8,
			MotivoCodigo: fiscal.MotivoNDDiferencialCambiario})
	if err != nil {
		t.Fatalf("nota de débito 8%%: %v", err)
	}
	if !casi(nd.BaseImponible, 16) {
		t.Fatalf("base = %.4f, se esperaba 8%% de 200 = 16", nd.BaseImponible)
	}
	// POSITIVO: la nota de débito suma. El signo es lo que la distingue de la NC.
	if nd.Total <= 0 {
		t.Fatalf("total = %.4f, la nota de débito tiene que sumar", nd.Total)
	}
	if nd.MotivoCodigo != fiscal.MotivoNDDiferencialCambiario {
		t.Fatalf("motivo = %q, se esperaba el del catálogo", nd.MotivoCodigo)
	}
}

// La ND sobre un producto usa ese renglón, y SIN tope: un cargo posterior sí
// puede superar lo que lo originó (una mora del 100 % es legítima).
func TestNDPorcentaje_SobreUnProductoSinTope(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 1, 100)

	nd, err := svc.EmitirNotaDebito(empDemo, sede1, actorA, origenTst, doc.ID,
		application.NotaDebitoEntrada{Concepto: "Mora", Porcentaje: 100, SKU: sku,
			MotivoCodigo: fiscal.MotivoNDInteresesMora})
	if err != nil {
		t.Fatalf("nota de débito sobre producto: %v", err)
	}
	if !casi(nd.Subtotal, 100) {
		t.Fatalf("subtotal = %.4f, se esperaba 100", nd.Subtotal)
	}
}

// Un motivo que no existe se rechaza en vez de guardarse: un código inventado
// rompe cualquier consulta por motivo sin que nadie lo note.
func TestNDMotivoInventado_SeRechaza(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	doc := facturaParaNC(t, svc, sku, 1, 100)

	if _, err := svc.EmitirNotaDebito(empDemo, sede1, actorA, origenTst, doc.ID,
		application.NotaDebitoEntrada{Concepto: "Cargo", Monto: 10, MotivoCodigo: "porque_si"}); err == nil {
		t.Fatal("un motivo fuera del catálogo debería rechazarse")
	}
}

// La ND sobre una factura mixta reparte igual que la NC, y el IVA solo grava la
// parte gravada.
func TestNDPorcentaje_RepartePorBaseGravadaYExenta(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := facturaMixtaParaNota(t, svc, 200, 100)

	nd, err := svc.EmitirNotaDebito(empDemo, sede1, actorA, origenTst, doc.ID,
		application.NotaDebitoEntrada{Concepto: "Ajuste", Porcentaje: 10})
	if err != nil {
		t.Fatalf("nota de débito: %v", err)
	}
	if !casi(nd.BaseImponible, 20) || !casi(nd.BaseExenta, 10) {
		t.Fatalf("reparto = gravado %.4f / exento %.4f, se esperaba 20 / 10", nd.BaseImponible, nd.BaseExenta)
	}
	if !casi(nd.IVA, 20*doc.AlicuotaIVA) {
		t.Fatalf("IVA = %.4f, se esperaba el de 20 gravados", nd.IVA)
	}
}
