package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
)

/* COSTO EN DESTINO (landed cost).
 *
 * Un flete o un impuesto de importación se pagan aparte y, sin esto, no entran al
 * costo del producto: el inventario queda valorado por debajo de lo que costó y
 * CADA VENTA muestra un margen que no existe. No falla nada — todos los números
 * de rentabilidad quedan mal, y hacia arriba, que es la dirección en la que nadie
 * los cuestiona.
 *
 * Lo que estas pruebas cuidan: que el reparto cuadre al céntimo, que el asiento
 * cuadre (uno descuadrado NO se guarda, solo se loguea: el costo desaparecería en
 * silencio) y que la parte de lo ya vendido no se capitalice. */

// servicioConCostos devuelve un servicio con los costos en destino cableados.
func servicioConCostos(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConCostosEnDestino(st.CostosEnDestino)
	return svc
}

// ordenRecibidaDe recibe `cant` unidades de un SKU a `costo` cada una.
func ordenRecibidaDe(t *testing.T, svc *application.Service, sku string, cant, costo float64) compra.OrdenCompra {
	t.Helper()
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: cant, CostoUnitario: costo}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	out, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: cant}})
	if err != nil {
		t.Fatalf("recibir: %v", err)
	}
	return out
}

// costoPromedioDe proyecta el costo promedio vigente de un SKU en la sede.
func costoPromedioDe(t *testing.T, svc *application.Service, sku string) float64 {
	t.Helper()
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			return e.CostoPromedio
		}
	}
	t.Fatalf("el SKU %s no aparece en existencias", sku)
	return 0
}

// dosSKUs devuelve dos productos gravados distintos del catálogo sembrado.
func dosSKUs(t *testing.T) [2]string {
	t.Helper()
	svc, _ := nuevoServicio(t)
	out := [2]string{}
	n := 0
	for _, p := range svc.Productos(empDemo) {
		if p.ExentoIVA || p.EsCombo || p.EsPlato {
			continue
		}
		out[n] = p.SKU
		if n++; n == 2 {
			return out
		}
	}
	t.Fatal("el seed debería traer al menos dos productos gravados")
	return out
}

// ponerExistenciaEnCero deja el SKU sin stock, para que las proporciones del
// reparto se puedan seguir a mano sin cargar con lo que trae el seed.
func ponerExistenciaEnCero(t *testing.T, svc *application.Service, sku string) {
	t.Helper()
	actual := 0.0
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			actual = e.Cantidad
		}
	}
	if actual <= 0.0001 {
		return
	}
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "puesta a cero para la prueba", -actual, actorA, origenTst); err != nil {
		t.Fatalf("poner en cero: %v", err)
	}
}

// venderUnidades saca stock del almacén. Se usa un ajuste y no una venta a
// propósito: lo que la prueba necesita es que las unidades ya no estén, no
// arrastrar el flujo fiscal entero.
func venderUnidades(t *testing.T, svc *application.Service, sku string, cant float64) {
	t.Helper()
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "salida para la prueba", -cant, actorA, origenTst); err != nil {
		t.Fatalf("sacar %v unidades: %v", cant, err)
	}
}

// TestCostoDestino_ElFleteEntraAlCostoDelProducto es el caso que da sentido a
// todo: 10 unidades a Bs 100 con Bs 200 de flete cuestan Bs 120, no Bs 100.
func TestCostoDestino_ElFleteEntraAlCostoDelProducto(t *testing.T) {
	svc := servicioConCostos(t)
	sku := primerSKU(t, svc)
	ponerExistenciaEnCero(t, svc, sku)
	oc := ordenRecibidaDe(t, svc, sku, 10, 100)
	// El promedio se mide DESPUÉS de recibir: comparando contra el de antes se
	// mezclaría el efecto de la recepción con el del flete, y con stock previo del
	// seed la suma puede incluso bajar.
	trasRecepcion := costoPromedioDe(t, svc, sku)
	casiEq(t, trasRecepcion, 100, "recibidas 10 u a Bs 100, el promedio es 100")

	cd, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, application.EntradaCostoEnDestino{
		OrdenCompraID: oc.ID, Descripcion: "Flete internacional", Monto: 200,
	})
	if err != nil {
		t.Fatalf("aplicar costo en destino: %v", err)
	}
	casiEq(t, cd.Absorbido, 200, "con todo el stock en almacén, el inventario absorbe el costo entero")
	casiEq(t, cd.AlGasto, 0, "nada al gasto: no se vendió nada")

	// 200 repartidos entre 10 unidades: Bs 20 más por unidad. ESTE es el número
	// que hace que el margen deje de mentir.
	casiEq(t, costoPromedioDe(t, svc, sku), 120, "cada unidad costó 100 de compra + 20 de flete")
}

// TestCostoDestino_RepartoPorValorYPorCantidad: dos productos de precios muy
// distintos reparten el flete de forma distinta según el criterio, y las dos
// sumas dan el monto exacto.
func TestCostoDestino_RepartoPorValorYPorCantidad(t *testing.T) {
	skus := dosSKUs(t)
	caros, baratos := skus[0], skus[1]

	casos := []struct {
		criterio string
		aCaro    float64
		por      string
	}{
		// Caro: 10 u × 100 = 1.000. Barato: 10 u × 10 = 100. Total 1.100.
		{compra.CriterioValor, 100, "por valor, el caro carga 1.000/1.100 del flete"},
		// Mismas unidades ⇒ mitad y mitad.
		{compra.CriterioCantidad, 55, "por cantidad, mismas unidades reparten igual"},
	}
	for _, c := range casos {
		svc := servicioConCostos(t)
		oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
			ProveedorID: provDemo1, SedeID: sede1,
			Lineas: []application.LineaOCEntrada{
				{SKU: caros, Cantidad: 10, CostoUnitario: 100},
				{SKU: baratos, Cantidad: 10, CostoUnitario: 10},
			},
		})
		if err != nil {
			t.Fatalf("%s: crear OC: %v", c.por, err)
		}
		if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
			t.Fatalf("confirmar: %v", err)
		}
		if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst, []application.LineaRecepcion{
			{SKU: caros, Cantidad: 10}, {SKU: baratos, Cantidad: 10},
		}); err != nil {
			t.Fatalf("recibir: %v", err)
		}
		cd, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, application.EntradaCostoEnDestino{
			OrdenCompraID: oc.ID, Descripcion: "Flete", Monto: 110, Criterio: c.criterio,
		})
		if err != nil {
			t.Fatalf("%s: aplicar: %v", c.por, err)
		}
		var alCaro, suma float64
		for _, l := range cd.Lineas {
			suma += l.Reparto
			if l.SKU == caros {
				alCaro = l.Reparto
			}
		}
		casiEq(t, alCaro, c.aCaro, c.por)
		// LO QUE MÁS IMPORTA: el reparto suma el monto EXACTO. Un céntimo perdido
		// descuadra el asiento, y un asiento descuadrado no se guarda.
		casiEq(t, suma, 110, c.criterio+": el reparto suma el monto exacto")
	}
}

// TestCostoDestino_LoYaVendidoNoSeCapitaliza: si de 10 recibidas quedan 4, solo
// el 40 % del reparto corresponde a existencias que se pueden valorar. El resto
// es gasto del período — capitalizarlo inflaría el inventario con un costo que no
// respalda ninguna unidad en el almacén.
func TestCostoDestino_LoYaVendidoNoSeCapitaliza(t *testing.T) {
	svc := servicioConCostos(t)
	sku := primerSKU(t, svc)
	// Se parte de existencia CERO para que la proporción sea exactamente 4 de 10.
	ponerExistenciaEnCero(t, svc, sku)
	oc := ordenRecibidaDe(t, svc, sku, 10, 100)
	venderUnidades(t, svc, sku, 6)

	cd, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, application.EntradaCostoEnDestino{
		OrdenCompraID: oc.ID, Descripcion: "Impuesto de importación", Monto: 200,
	})
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	casiEq(t, cd.Absorbido, 80, "quedan 4 de 10: el inventario absorbe el 40 %")
	casiEq(t, cd.AlGasto, 120, "el 60 % de lo ya vendido es gasto del período")
	casiEq(t, cd.Absorbido+cd.AlGasto, cd.Monto, "absorbido + gasto = monto, siempre")
}

// TestCostoDestino_ElAsientoCuadraYLlegaALasCuentas. Un asiento descuadrado NO se
// guarda —solo se registra en el log—, así que el fallo se vería como la ausencia
// del asiento, no como un error.
func TestCostoDestino_ElAsientoCuadraYLlegaALasCuentas(t *testing.T) {
	svc := servicioConCostos(t)
	sku := primerSKU(t, svc)
	ponerExistenciaEnCero(t, svc, sku)
	oc := ordenRecibidaDe(t, svc, sku, 10, 100)
	venderUnidades(t, svc, sku, 6)

	antes := len(svc.LibroDiario(empDemo))
	cd, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, application.EntradaCostoEnDestino{
		OrdenCompraID: oc.ID, Descripcion: "Flete", Monto: 200,
	})
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	asientos := svc.LibroDiario(empDemo)
	if len(asientos) != antes+1 {
		t.Fatalf("no se registró el asiento: un descuadre se ve exactamente así (antes %d, ahora %d)", antes, len(asientos))
	}
	// Por REFERENCIA y no por posición: el libro sale del más nuevo al más viejo y
	// depender de ese orden haría la prueba frágil por el motivo equivocado.
	var a contabilidad.Asiento
	for _, x := range asientos {
		if x.RefTipo == "costo_destino" && x.RefID == cd.ID {
			a = x
		}
	}
	if a.RefID == "" {
		t.Fatal("el asiento no quedó amarrado al costo en destino que lo originó")
	}
	casiEq(t, debeCta(a, contabilidad.CtaInventario), cd.Absorbido, "1201 sube lo que el inventario absorbió")
	casiEq(t, debeCta(a, contabilidad.CtaDiferenciaEnCompras), cd.AlGasto, "5202 se lleva lo de la mercancía ya vendida")
	casiEq(t, haberCta(a, contabilidad.CtaCuentasPorPagar), cd.Monto, "2101 debe el costo entero a quien lo prestó")
}

// TestCostoDestino_SeCorrigeConElContrario: el ledger es de solo anexado, así que
// un costo mal cargado no se borra — se aplica el contrario y el promedio vuelve.
func TestCostoDestino_SeCorrigeConElContrario(t *testing.T) {
	svc := servicioConCostos(t)
	sku := primerSKU(t, svc)
	oc := ordenRecibidaDe(t, svc, sku, 10, 100)
	original := costoPromedioDe(t, svc, sku)

	if _, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, application.EntradaCostoEnDestino{
		OrdenCompraID: oc.ID, Descripcion: "Flete mal cargado", Monto: 500,
	}); err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if costoPromedioDe(t, svc, sku) <= original {
		t.Fatal("el costo tenía que haber subido")
	}
	if _, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, application.EntradaCostoEnDestino{
		OrdenCompraID: oc.ID, Descripcion: "Reverso del flete mal cargado", Monto: -500,
	}); err != nil {
		t.Fatalf("aplicar el contrario: %v", err)
	}
	casiEq(t, costoPromedioDe(t, svc, sku), original, "aplicado el contrario, el promedio vuelve a donde estaba")
}

// TestCostoDestino_RechazaLoQueNoSePuedeRepartir. Cada guarda evita un documento
// que parece que hizo algo y no hizo nada.
func TestCostoDestino_RechazaLoQueNoSePuedeRepartir(t *testing.T) {
	svc := servicioConCostos(t)
	sku := primerSKU(t, svc)
	oc := ordenRecibidaDe(t, svc, sku, 10, 100)
	base := application.EntradaCostoEnDestino{OrdenCompraID: oc.ID, Descripcion: "Flete", Monto: 100}

	sinDesc := base
	sinDesc.Descripcion = "  "
	if _, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, sinDesc); !errors.Is(err, application.ErrCostoDestinoSinDescripcion) {
		t.Errorf("sin descripción debía rechazarse: %v", err)
	}
	cero := base
	cero.Monto = 0
	if _, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, cero); !errors.Is(err, application.ErrCostoDestinoMontoCero) {
		t.Errorf("monto cero debía rechazarse: %v", err)
	}
	malCriterio := base
	malCriterio.Criterio = "por_peso"
	if _, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, malCriterio); !errors.Is(err, application.ErrCostoDestinoCriterio) {
		t.Errorf("un criterio inventado debía rechazarse: %v", err)
	}

	// Una orden sin recibir nada: no hay existencia que encarecer.
	sinRecibir, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 5, CostoUnitario: 100}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.AplicarCostoEnDestino(empDemo, actorA, origenTst, application.EntradaCostoEnDestino{
		OrdenCompraID: sinRecibir.ID, Descripcion: "Flete", Monto: 100,
	}); !errors.Is(err, application.ErrCostoDestinoSinRecibir) {
		t.Errorf("sobre una orden sin recibir debía rechazarse: %v", err)
	}
}
