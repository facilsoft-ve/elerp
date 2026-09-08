package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// TestCrearProductoPorPeso verifica que un producto con TipoVenta "peso" se
// persiste con la forma de venta correcta y con la unidad base fijada en "kg",
// sin importar qué unidad base se haya enviado (la fija el servicio).
func TestCrearProductoPorPeso(t *testing.T) {
	svc, _ := nuevoServicio(t)
	out, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "PESO-QUESO", Nombre: "Queso de prueba (por peso)", Rubro: "Charcutería",
		// Se envía a propósito una unidad base distinta: el servicio debe forzarla a "kg".
		UnidadBase: inventario.UnidadUnidad, TipoVenta: inventario.TipoVentaPeso, Precio: 5000,
	})
	if err != nil {
		t.Fatalf("crear producto por peso: %v", err)
	}
	if out.TipoVenta != inventario.TipoVentaPeso {
		t.Errorf("TipoVenta esperado %q, se obtuvo %q", inventario.TipoVentaPeso, out.TipoVenta)
	}
	if out.UnidadBase != inventario.UnidadKg {
		t.Errorf("un producto por peso debe quedar en kg, se obtuvo %q", out.UnidadBase)
	}
	// La lectura del catálogo también debe conservar la forma de venta.
	p, ok := svc.PorCodigoBarras(empDemo, "PESO-QUESO") // resuelve por SKU
	if !ok {
		t.Fatal("el producto por peso debería encontrarse por su SKU")
	}
	if p.TipoVenta != inventario.TipoVentaPeso || p.UnidadBase != inventario.UnidadKg {
		t.Errorf("el producto persistido debe seguir siendo peso/kg, se obtuvo %q/%q", p.TipoVenta, p.UnidadBase)
	}
}

// TestCrearProductoTipoVentaInvalido rechaza una forma de venta desconocida.
func TestCrearProductoTipoVentaInvalido(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "MALA-001", Nombre: "Producto raro", TipoVenta: "litros", Precio: 10,
	})
	if err != application.ErrTipoVentaInvalido {
		t.Fatalf("se esperaba ErrTipoVentaInvalido, se obtuvo: %v", err)
	}
}

// TestEmitirFacturaProductoPorPeso emite una venta de un producto por peso con
// CANTIDAD FRACCIONARIA (1,25 kg) y verifica que el total = 1,25 × precio/kg más
// el IVA (el producto es gravado), que el inventario decrementa exactamente 1,25
// kg (salida por peso) y que el libro diario sigue cuadrando.
func TestEmitirFacturaProductoPorPeso(t *testing.T) {
	svc, _ := nuevoServicio(t)

	// QUE-KG es un producto del seed que se vende por peso (Bs/kg), gravado con IVA.
	const sku = "QUE-KG"
	prod, ok := svc.PorCodigoBarras(empDemo, sku)
	if !ok {
		t.Fatalf("el seed debería traer el producto por peso %s", sku)
	}
	if prod.TipoVenta != inventario.TipoVentaPeso || prod.UnidadBase != inventario.UnidadKg {
		t.Fatalf("%s debería estar sembrado como peso/kg, se obtuvo %q/%q", sku, prod.TipoVenta, prod.UnidadBase)
	}
	if prod.ExentoIVA {
		t.Fatalf("la prueba asume %s gravado para poder verificar el IVA", sku)
	}
	precioKg := prod.Precio

	const kg = 1.25
	cant0, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	if cant0 < kg {
		t.Fatalf("el seed debería traer stock suficiente de %s (hay %v kg)", sku, cant0)
	}

	base := kg * precioKg          // 1,25 kg × precio/kg
	iva := round2Test(base * 0.16) // gravado al 16%
	total := round2Test(base + iva)

	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true, // canal forma libre: no exige caja abierta
		// PrecioUnitario negativo ⇒ usar el precio/kg del catálogo.
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: kg, PrecioUnitario: -1}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: total, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir factura por peso: %v", err)
	}

	// La línea conserva la cantidad fraccionaria y su total = kg × precio/kg.
	if len(doc.Lineas) != 1 || !casi(doc.Lineas[0].Cantidad, kg) {
		t.Fatalf("la factura debe tener una línea de %v kg, se obtuvo %+v", kg, doc.Lineas)
	}
	if !casi(doc.Lineas[0].Total, round2Test(base)) {
		t.Errorf("total de línea esperado %v, se obtuvo %v", round2Test(base), doc.Lineas[0].Total)
	}
	if !casi(doc.Subtotal, round2Test(base)) {
		t.Errorf("subtotal esperado %v, se obtuvo %v", round2Test(base), doc.Subtotal)
	}
	if !casi(doc.BaseImponible, round2Test(base)) || !casi(doc.BaseExenta, 0) {
		t.Errorf("un producto gravado va todo a base imponible, se obtuvo imp=%v exenta=%v", doc.BaseImponible, doc.BaseExenta)
	}
	if !casi(doc.IVA, iva) {
		t.Errorf("IVA esperado %v, se obtuvo %v", iva, doc.IVA)
	}
	if !casi(doc.Total, total) {
		t.Errorf("total esperado %v, se obtuvo %v", total, doc.Total)
	}

	// El inventario decrementa EXACTAMENTE por el peso vendido (salida de 1,25 kg).
	cant1, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	if !casi(cant0-cant1, kg) {
		t.Errorf("el inventario debía bajar %v kg (de %v a %v), bajó %v", kg, cant0, cant1, cant0-cant1)
	}

	// El asiento derivado de la venta cuadra y el balance global también.
	assertLibroCuadra(t, svc, empDemo)
}

// TestEmitirFacturaExentoPorPeso comprueba que un producto por peso EXENTO no
// causa IVA: la base va a exenta y el total es solo kg × precio/kg.
func TestEmitirFacturaExentoPorPeso(t *testing.T) {
	svc, _ := nuevoServicio(t)

	// Producto por peso exento creado para la prueba (p. ej. un vegetal de la
	// cesta básica vendido a granel).
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "VEG-KG", Nombre: "Yuca (por peso)", Rubro: "Víveres",
		TipoVenta: inventario.TipoVentaPeso, Precio: 800, ExentoIVA: true,
	}); err != nil {
		t.Fatalf("crear producto exento por peso: %v", err)
	}
	// Entrada inicial para que haya stock que descontar.
	if _, err := svc.Ajustar(empDemo, sede1, "", "VEG-KG", "inventario inicial", 10, actorA, origenTst); err != nil {
		t.Fatalf("ajuste inicial: %v", err)
	}

	const kg = 2.5
	base := kg * 800.0
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true,
		Lineas:  []application.LineaEntrada{{SKU: "VEG-KG", Cantidad: kg, PrecioUnitario: -1}}, // -1 = precio de catálogo
		Pagos:   []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: round2Test(base), Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir factura exenta por peso: %v", err)
	}
	if !casi(doc.IVA, 0) {
		t.Errorf("un producto exento no causa IVA, se obtuvo %v", doc.IVA)
	}
	if !casi(doc.BaseExenta, round2Test(base)) || !casi(doc.BaseImponible, 0) {
		t.Errorf("la base debe ir toda a exenta, se obtuvo exenta=%v imp=%v", doc.BaseExenta, doc.BaseImponible)
	}
	if !casi(doc.Total, round2Test(base)) {
		t.Errorf("total esperado %v (sin IVA), se obtuvo %v", round2Test(base), doc.Total)
	}
	assertLibroCuadra(t, svc, empDemo)
}

// TestBalanzaDispositivo comprueba que la balanza es un tipo de dispositivo
// válido y que se administra por el flujo existente de dispositivos, guardando
// puerto y protocolo.
func TestBalanzaDispositivo(t *testing.T) {
	svc, _ := nuevoServicio(t)

	out, err := svc.CrearDispositivoFiscal(empDemo, actorA, origenTst, fiscal.DispositivoFiscal{
		Nombre: "Balanza mostrador", SedeID: sede1, Tipo: fiscal.DispositivoBalanza,
		Marca: "Aclas", Modelo: "OS2X", Puerto: "COM3", Protocolo: "prt1",
	})
	if err != nil {
		t.Fatalf("crear balanza: %v", err)
	}
	if out.Tipo != fiscal.DispositivoBalanza || out.Puerto != "COM3" || out.Protocolo != "prt1" {
		t.Errorf("la balanza debe conservar tipo/puerto/protocolo, se obtuvo %+v", out)
	}

	// Actualización parcial de puerto/protocolo.
	nuevoPuerto := "/dev/ttyUSB1"
	upd, err := svc.ActualizarDispositivoFiscal(empDemo, actorA, origenTst, out.ID, application.CambiosDispositivoFiscal{
		Puerto: &nuevoPuerto,
	})
	if err != nil {
		t.Fatalf("actualizar balanza: %v", err)
	}
	if upd.Puerto != nuevoPuerto || upd.Protocolo != "prt1" {
		t.Errorf("el patch debe cambiar solo el puerto, se obtuvo %+v", upd)
	}

	// El seed también trae una balanza demo en la sede principal.
	var balanzas int
	for _, d := range svc.DispositivosFiscales(empDemo) {
		if d.Tipo == fiscal.DispositivoBalanza {
			balanzas++
		}
	}
	if balanzas < 2 {
		t.Errorf("se esperaban al menos 2 balanzas (seed + creada), hay %d", balanzas)
	}
}
