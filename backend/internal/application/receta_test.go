package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// DESCUENTO DE INSUMOS POR RECETA (escandallo).
//
// Un plato no se stockea: al facturarlo el inventario descuenta sus INSUMOS. Y la línea
// del documento guarda un SNAPSHOT de la receta, para que una anulación reingrese
// exactamente lo que salió — aunque la receta del catálogo haya cambiado en el medio.
// Es la pieza que sostiene el cuadre del Kardex y no tenía ninguna prueba.

const (
	skuMasa  = "TEST-MASA"
	skuQueso = "TEST-QUESO"
	skuPlato = "TEST-PIZZA"
)

// salonConPlato deja un plato con receta (0,2 de masa + 0,1 de queso), sus insumos con
// stock y una caja abierta para poder facturar.
func salonConPlato(t *testing.T) (*application.Service, *inventarioTest) {
	t.Helper()
	svc, _ := nuevoServicio(t)

	for _, sku := range []string{skuMasa, skuQueso} {
		if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
			SKU: sku, Nombre: sku, Rubro: "Insumos", UnidadBase: "kg",
			TipoVenta: inventario.TipoVentaPeso, EsInsumo: true, Activo: true,
		}); err != nil {
			t.Fatalf("crear insumo %s: %v", sku, err)
		}
		// 10 kg de entrada a Bs 100/kg.
		if _, err := svc.Ajustar(empDemo, sede1, "", sku, "carga inicial", 10, actorA, origenTst); err != nil {
			t.Fatalf("cargar %s: %v", sku, err)
		}
	}
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: skuPlato, Nombre: "Pizza de prueba", Rubro: "Cocina", UnidadBase: "unidad",
		Precio: 50000, EsPlato: true, Activo: true,
		Receta: []inventario.ComboComponente{
			{SKU: skuMasa, Cantidad: 0.2},
			{SKU: skuQueso, Cantidad: 0.1},
		},
	}); err != nil {
		t.Fatalf("crear plato: %v", err)
	}
	return svc, &inventarioTest{svc: svc}
}

// inventarioTest lee existencias por SKU sin repetir el bucle en cada prueba.
type inventarioTest struct{ svc *application.Service }

func (i *inventarioTest) stock(t *testing.T, sku string) float64 {
	t.Helper()
	for _, e := range i.svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			return e.Cantidad
		}
	}
	return 0 // un plato no aparece en existencias: no se stockea
}

// Vender un plato descuenta sus INSUMOS —no el plato— y la línea del documento se lleva
// el snapshot de la receta.
func TestReceta_VenderUnPlatoDescuentaSusInsumos(t *testing.T) {
	svc, inv := salonConPlato(t)

	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true,
		Lineas:  []application.LineaEntrada{{SKU: skuPlato, Cantidad: 3, PrecioUnitario: 50000}},
		Pagos:   []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 174000, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}

	// 3 pizzas → 0,6 de masa y 0,3 de queso.
	if got := inv.stock(t, skuMasa); !casiRec(got, 10-0.6) {
		t.Errorf("masa: se esperaba %.3f, hay %.3f", 10-0.6, got)
	}
	if got := inv.stock(t, skuQueso); !casiRec(got, 10-0.3) {
		t.Errorf("queso: se esperaba %.3f, hay %.3f", 10-0.3, got)
	}
	// El plato NO se stockea: no puede tener existencia ni movimientos.
	if got := inv.stock(t, skuPlato); got != 0 {
		t.Errorf("el plato no se stockea, su existencia debería ser 0 y es %.3f", got)
	}

	// El snapshot viaja en la línea: es lo que permite deshacer sin depender del catálogo.
	if len(doc.Lineas) != 1 {
		t.Fatalf("se esperaba 1 línea, hay %d", len(doc.Lineas))
	}
	ins := doc.Lineas[0].Insumos
	if len(ins) != 2 {
		t.Fatalf("la línea debe guardar los 2 insumos de la receta, guardó %d", len(ins))
	}
	porSKU := map[string]float64{}
	for _, x := range ins {
		porSKU[x.SKU] = x.CantidadUnitaria
	}
	// Cantidad POR UNIDAD del plato (no multiplicada): así la reversa puede recalcular
	// para cualquier cantidad devuelta.
	if !casiRec(porSKU[skuMasa], 0.2) || !casiRec(porSKU[skuQueso], 0.1) {
		t.Errorf("el snapshot debe guardar la cantidad por unidad: %v", porSKU)
	}
}

// LA RAZÓN DE SER DEL SNAPSHOT: si la receta del catálogo cambia después de vender,
// anular tiene que reingresar lo que SALIÓ, no lo que la receta dice hoy. Sin esto el
// Kardex queda descuadrado para siempre.
func TestReceta_AnularReingresaLoQueSalio_AunqueLaRecetaCambie(t *testing.T) {
	svc, inv := salonConPlato(t)

	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true,
		Lineas:  []application.LineaEntrada{{SKU: skuPlato, Cantidad: 2, PrecioUnitario: 50000}},
		Pagos:   []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 116000, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	trasVenta := inv.stock(t, skuMasa) // 10 - 0,4 = 9,6

	// El cocinero cambia la receta: ahora la pizza lleva el TRIPLE de masa.
	nueva := []inventario.ComboComponente{{SKU: skuMasa, Cantidad: 0.6}, {SKU: skuQueso, Cantidad: 0.1}}
	if _, err := svc.ActualizarProducto(empDemo, actorA, origenTst, skuPlato,
		application.CambiosProducto{Receta: nueva}); err != nil {
		t.Fatalf("cambiar la receta: %v", err)
	}

	if _, err := svc.AnularDocumento(empDemo, sede1, actorA, origenTst, doc.ID, "prueba"); err != nil {
		t.Fatalf("anular: %v", err)
	}

	// Debe volver a 10 exacto: se reingresa 0,4 (lo que salió), no 1,2 (la receta nueva).
	if got := inv.stock(t, skuMasa); !casiRec(got, 10) {
		t.Errorf("la anulación debe reingresar lo que SALIÓ (volver a 10,000); hay %.3f "+
			"(tras la venta había %.3f). Si dio 10,800 se usó la receta NUEVA y el Kardex quedó inflado",
			got, trasVenta)
	}
	if got := inv.stock(t, skuQueso); !casiRec(got, 10) {
		t.Errorf("queso: se esperaba 10,000, hay %.3f", got)
	}
}

// Una nota de crédito PARCIAL reingresa solo la parte devuelta.
func TestReceta_NotaCreditoParcialReingresaLaParte(t *testing.T) {
	svc, inv := salonConPlato(t)

	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true,
		Lineas:  []application.LineaEntrada{{SKU: skuPlato, Cantidad: 4, PrecioUnitario: 50000}},
		Pagos:   []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 232000, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	// 4 pizzas → 0,8 de masa. Quedan 9,2.
	if got := inv.stock(t, skuMasa); !casiRec(got, 9.2) {
		t.Fatalf("masa tras vender 4: se esperaba 9,200, hay %.3f", got)
	}

	// Devuelve 1 de las 4.
	if _, err := svc.EmitirNotaCredito(empDemo, sede1, actorA, origenTst, doc.ID, "una salió mal",
		[]application.LineaEntrada{{SKU: skuPlato, Cantidad: 1, PrecioUnitario: 50000}}); err != nil {
		t.Fatalf("nota de crédito: %v", err)
	}

	// Reingresa 0,2 de masa (una pizza), no las cuatro.
	if got := inv.stock(t, skuMasa); !casiRec(got, 9.4) {
		t.Errorf("la NC de 1 pizza debe reingresar 0,200 (quedar en 9,400); hay %.3f", got)
	}
	if got := inv.stock(t, skuQueso); !casiRec(got, 9.7) {
		t.Errorf("queso: se esperaba 9,700, hay %.3f", got)
	}
}

// Un producto NORMAL (sin receta) sigue moviendo su propio SKU: el camino de los insumos
// no debe cambiarle nada al resto del catálogo.
func TestReceta_UnProductoNormalMueveSuPropioSKU(t *testing.T) {
	svc, inv := salonConPlato(t)
	antes := inv.stock(t, skuMasa)

	// Se vende el insumo directamente (un caso administrativo: no pasa por el POS porque
	// `vendibles()` lo filtra, pero el motor fiscal debe manejarlo igual).
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true,
		Lineas:  []application.LineaEntrada{{SKU: skuMasa, Cantidad: 1, PrecioUnitario: 200}},
		Pagos:   []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 232, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if got := inv.stock(t, skuMasa); !casiRec(got, antes-1) {
		t.Errorf("un producto sin receta descuenta su propio SKU: se esperaba %.3f, hay %.3f", antes-1, got)
	}
}

// casiRec compara cantidades con tolerancia de gramos (las recetas llevan 3 decimales).
func casiRec(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.0005
}
