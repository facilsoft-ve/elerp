package application_test

import (
	"strings"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fabricacion"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* FABRICACIÓN.
 *
 * Lo que se prueba acá es lo que se paga con plata: que el costo del producto
 * terminado SALGA de lo que consumió y no de un número tecleado, y que el
 * inventario cierre en los tres momentos —arrancar, terminar y cancelar—.
 *
 * Y la regla que más fácil se rompe en un refactor: un plato fabricado PARA
 * STOCK no vuelve a consumir sus insumos al venderse. Si eso se pierde, cada
 * venta descuenta los insumos por segunda vez y el inventario se va a negativo
 * mientras el producto terminado queda intacto.
 */

func servicioFabricacion(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConFabricacion(st.OrdenesFabricacion)
	return svc, st
}

// cargar mete stock CON COSTO. El ajuste normal usa el promedio vigente, que en
// un producto nuevo es cero, y una prueba de costeo sobre costos nulos no prueba
// nada. Recibe el store explícito: un global se quedaría apuntando al de otra
// prueba en cuanto alguna arme su propio servicio.
func cargar(t *testing.T, st *inmem.Store, productoID, sku string, cant, costo float64) {
	t.Helper()
	st.Movimientos.Append(inventario.Movimiento{
		EmpresaID: empDemo, SedeID: sede1, ProductoID: productoID, SKU: sku,
		Tipo: inventario.MovEntrada, Cantidad: cant, CostoUnitario: costo,
		Motivo: "carga de prueba", Actor: actorA, Fecha: "2026-01-01T00:00:00Z",
	})
}

// recetaDePrueba crea un producto con receta sobre dos insumos del catálogo demo
// y deja stock suficiente de ambos.
// El store va explícito: el stock de prueba entra con costo, y eso se escribe
// directo en el ledger porque un ajuste usaría el promedio vigente —cero en un
// producto nuevo— y la prueba de costeo correría sobre nada.
func recetaDePrueba(t *testing.T, svc *application.Service, st *inmem.Store, modo string) (inventario.Producto, string, string) {
	t.Helper()
	insA, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		EmpresaID: empDemo, SKU: "INS-A", Nombre: "Insumo A", UnidadBase: inventario.UnidadKg,
		EsInsumo: true, Activo: true,
	})
	if err != nil {
		t.Fatalf("insumo A: %v", err)
	}
	insB, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		EmpresaID: empDemo, SKU: "INS-B", Nombre: "Insumo B", UnidadBase: inventario.UnidadUnidad,
		EsInsumo: true, Activo: true,
	})
	if err != nil {
		t.Fatalf("insumo B: %v", err)
	}
	/* El stock entra CON COSTO. Un ajuste usa el promedio vigente, que en un
	 * producto nuevo es cero, y entonces toda la prueba correría sobre costos nulos
	 * — que es como una prueba de costeo no prueba nada. */
	cargar(t, st, insA.ID, insA.SKU, 100, 10)
	cargar(t, st, insB.ID, insB.SKU, 50, 4)
	plato, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		EmpresaID: empDemo, SKU: "TORTA", Nombre: "Torta de chocolate", UnidadBase: inventario.UnidadUnidad,
		EsPlato: true, ModoFabricacion: modo, Precio: 500, Activo: true,
		Receta: []inventario.ComboComponente{
			{SKU: insA.SKU, Cantidad: 2}, // 2 kg por torta
			{SKU: insB.SKU, Cantidad: 3}, // 3 unidades por torta
		},
	})
	if err != nil {
		t.Fatalf("plato: %v", err)
	}
	return plato, insA.SKU, insB.SKU
}

// El costo del producto terminado SALE de lo que consumió. Es la razón de ser
// del módulo: un costo tecleado vuelve inútiles el margen y la valorización.
func TestFabricacion_ElCostoSaleDeLosInsumos(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)

	// El ajuste de carga entra al costo promedio vigente, que para un producto
	// nuevo es 0; se le pone costo comprando de verdad no hace falta acá: lo que
	// importa es que el costo del plato sea la SUMA de sus consumos, sea cual sea.
	plan, err := svc.PlanearOrden(empDemo, sede1, plato.SKU, 10)
	if err != nil {
		t.Fatalf("planear: %v", err)
	}
	if len(plan.Consumos) != 2 {
		t.Fatalf("la receta tiene 2 insumos, el plan trae %d", len(plan.Consumos))
	}
	// 10 tortas × 2 kg = 20 de A; × 3 = 30 de B.
	if plan.Consumos[0].Cantidad != 20 || plan.Consumos[1].Cantidad != 30 {
		t.Fatalf("las cantidades del plan salieron mal: %+v", plan.Consumos)
	}
	if !plan.Alcanza {
		t.Fatalf("hay stock de sobra y dice que no alcanza: %v", plan.Faltantes)
	}

	o, err := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: plato.SKU, Cantidad: 10, Actor: actorA, Origen: origenTst,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if o.NumeroCompleto == "" {
		t.Fatal("la orden necesita su número visible: en el taller se dice en voz alta")
	}
	o, err = svc.IniciarOrden(empDemo, o.ID, actorA, origenTst)
	if err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if o.CostoTotal != plan.CostoTotal {
		t.Fatalf("el costo congelado (%v) no es el que se planificó (%v)", o.CostoTotal, plan.CostoTotal)
	}
	o, err = svc.TerminarOrden(empDemo, o.ID, 10, actorA, origenTst)
	if err != nil {
		t.Fatalf("terminar: %v", err)
	}
	if o.CostoUnitario != round2Fab(o.CostoTotal/10) {
		t.Fatalf("el costo unitario tiene que ser el total entre lo producido: %v", o.CostoUnitario)
	}
}

// Arrancar SACA los insumos del almacén. Si esperara al final, dos órdenes
// podrían planificarse sobre el mismo kilo y las dos «cabrían».
func TestFabricacion_ArrancarConsumeYTerminarProduce(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, insA, insB := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)

	o, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: plato.SKU, Cantidad: 10, Actor: actorA, Origen: origenTst,
	})
	if stockDe(svc, insA) != 100 {
		t.Fatal("planificar no puede tocar el inventario")
	}
	if _, err := svc.IniciarOrden(empDemo, o.ID, actorA, origenTst); err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if got := stockDe(svc, insA); got != 80 {
		t.Fatalf("A debería quedar en 80 tras consumir 20, quedó en %v", got)
	}
	if got := stockDe(svc, insB); got != 20 {
		t.Fatalf("B debería quedar en 20 tras consumir 30, quedó en %v", got)
	}
	if got := stockDe(svc, plato.SKU); got != 0 {
		t.Fatalf("todavía no se produjo nada y el plato tiene %v", got)
	}
	if _, err := svc.TerminarOrden(empDemo, o.ID, 10, actorA, origenTst); err != nil {
		t.Fatalf("terminar: %v", err)
	}
	if got := stockDe(svc, plato.SKU); got != 10 {
		t.Fatalf("el plato fabricado debería estar en 10, está en %v", got)
	}
}

// De una masa para 20 panes salen 18: el costo se reparte entre los que
// salieron, y la merma queda a la vista en vez de disolverse.
func TestFabricacion_LoQueNoSalioEncareceLoQueSi(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)
	o, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: plato.SKU, Cantidad: 10, Actor: actorA, Origen: origenTst,
	})
	o, _ = svc.IniciarOrden(empDemo, o.ID, actorA, origenTst)
	o, err := svc.TerminarOrden(empDemo, o.ID, 8, actorA, origenTst)
	if err != nil {
		t.Fatalf("terminar: %v", err)
	}
	if o.Merma() != 2 {
		t.Fatalf("la merma tiene que quedar explícita: %v", o.Merma())
	}
	if o.CantidadProducida != 8 {
		t.Fatalf("produjo 8, dice %v", o.CantidadProducida)
	}
	// El costo total no cambia; el unitario sube porque se reparte entre menos.
	if o.CostoTotal > 0 && o.CostoUnitario != round2Fab(o.CostoTotal/8) {
		t.Fatalf("el costo de los 10 tiene que repartirse entre los 8: %v", o.CostoUnitario)
	}
}

// Cancelar DEVUELVE los insumos: lo que no se fabricó sigue en el almacén, y
// darlo por gastado haría que el conteo físico no cuadre con el sistema.
func TestFabricacion_CancelarDevuelveLosInsumos(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, insA, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)
	o, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: plato.SKU, Cantidad: 10, Actor: actorA, Origen: origenTst,
	})
	o, _ = svc.IniciarOrden(empDemo, o.ID, actorA, origenTst)
	if stockDe(svc, insA) != 80 {
		t.Fatal("no consumió al arrancar")
	}
	if _, err := svc.CancelarOrden(empDemo, o.ID, "se dañó el horno", actorA, origenTst); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if got := stockDe(svc, insA); got != 100 {
		t.Fatalf("al cancelar, los 20 kg vuelven al almacén; quedó en %v", got)
	}
}

// No se arranca lo que no se puede terminar: dejar salir insumos para una orden
// que se va a trabar es perderlos de vista sin producir nada.
func TestFabricacion_NoArrancaSinInsumos(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)
	o, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		// 100 tortas necesitan 200 kg de A y solo hay 100.
		SedeID: sede1, SKU: plato.SKU, Cantidad: 100, Actor: actorA, Origen: origenTst,
	})
	if _, err := svc.IniciarOrden(empDemo, o.ID, actorA, origenTst); err == nil {
		t.Fatal("no hay insumos para 100 tortas y dejó arrancar")
	}
	if o2, _ := svc.OrdenFabricacion(empDemo, o.ID); o2.Estado != fabricacion.EstadoBorrador {
		t.Fatalf("la orden que no arrancó sigue en borrador, no en %q", o2.Estado)
	}
}

/* LA REGLA QUE UN REFACTOR ROMPE SIN DARSE CUENTA.
 *
 * Un plato fabricado PARA STOCK ya consumió sus insumos al fabricarse. Venderlo
 * tiene que descontarlo A ÉL. Si volviera a traer su receta, cada venta
 * descontaría los insumos por segunda vez: el inventario se iría a negativo y el
 * producto terminado quedaría intacto, al revés de lo que pasó en el mostrador.
 */
func TestFabricacion_VenderLoFabricadoNoConsumeInsumosOtraVez(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, insA, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)
	o, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: plato.SKU, Cantidad: 10, Actor: actorA, Origen: origenTst,
	})
	o, _ = svc.IniciarOrden(empDemo, o.ID, actorA, origenTst)
	if _, err := svc.TerminarOrden(empDemo, o.ID, 10, actorA, origenTst); err != nil {
		t.Fatalf("terminar: %v", err)
	}
	insAntes := stockDe(svc, insA)

	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas:  []application.LineaEntrada{{SKU: plato.SKU, Cantidad: 2, PrecioUnitario: 500}},
		Pagos:   []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 1160, Moneda: "VES"}},
		SinCaja: true,
	}); err != nil {
		t.Fatalf("vender: %v", err)
	}
	if got := stockDe(svc, plato.SKU); got != 8 {
		t.Fatalf("vender 2 tortas deja 8; quedaron %v", got)
	}
	if got := stockDe(svc, insA); got != insAntes {
		t.Fatalf("los insumos NO se tocan al vender lo ya fabricado: %v → %v", insAntes, got)
	}
}

// Y el plato BAJO PEDIDO sigue comportándose como siempre: no se stockea y
// venderlo consume sus insumos.
func TestFabricacion_ElPlatoBajoPedidoSigueIgual(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, insA, _ := recetaDePrueba(t, svc, st, inventario.FabricaBajoPedido)
	if stockDe(svc, plato.SKU) != 0 {
		t.Fatal("un plato bajo pedido no se stockea: no debería aparecer en existencias")
	}
	antes := stockDe(svc, insA)
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas:  []application.LineaEntrada{{SKU: plato.SKU, Cantidad: 2, PrecioUnitario: 500}},
		Pagos:   []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 1160, Moneda: "VES"}},
		SinCaja: true,
	}); err != nil {
		t.Fatalf("vender: %v", err)
	}
	if got := stockDe(svc, insA); got != antes-4 {
		t.Fatalf("vender 2 platos bajo pedido consume 4 kg de A: %v → %v", antes, got)
	}
}

func round2Fab(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

/* NO SE FABRICA CON ORDEN LO QUE NO SE GUARDA.
 *
 * Un producto bajo pedido se prepara al venderlo. Producirlo con una orden
 * metería unidades al ledger que ninguna pantalla muestra —inventario
 * invisible— y al venderlo consumiría los insumos otra vez. Pasó en producción
 * con el postre del demo: la orden corrió, el movimiento entró y el stock no
 * aparecía en ningún lado.
 */
func TestFabricacion_NoSeFabricaConOrdenLoQueNoSeGuarda(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaBajoPedido)
	_, err := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: plato.SKU, Cantidad: 5, Actor: actorA, Origen: origenTst,
	})
	if err == nil {
		t.Fatal("un producto bajo pedido no se produce con una orden: quedaría stock invisible")
	}
	// Y el mensaje tiene que decir QUÉ cambiar, no solo que no se puede.
	if !strings.Contains(err.Error(), "para stock") {
		t.Fatalf("el mensaje debe nombrar el interruptor a cambiar: %q", err)
	}
}

/* LA FÓRMULA, EN SERIO.
 *
 * Una receta de tres campos alcanza para una torta y no para producir. Lo que se
 * prueba acá son las tres pérdidas que una receta plana se salta, y que son
 * justamente las que hacen que la cuenta «cierre poco» — la forma en que un
 * error de costeo se queda años sin que nadie lo vea.
 */

// formulaEstricta arma un plato cuya receta está escrita para una TANDA de 10,
// pierde 20% en el proceso y descarta 10% al preparar el insumo A.
func formulaEstricta(t *testing.T, svc *application.Service, st *inmem.Store) inventario.Producto {
	t.Helper()
	plato, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)
	out, err := svc.ActualizarProducto(empDemo, actorA, origenTst, plato.SKU, application.CambiosProducto{
		Receta: []inventario.ComboComponente{
			{SKU: "INS-A", Cantidad: 2, MermaPct: 10}, // se descarta 10% al pelarlo
			{SKU: "INS-B", Cantidad: 3},
		},
		LoteBase:       ptrF(10), // la receta es para 10 unidades
		RendimientoPct: ptrF(80), // el proceso rinde 80%
		ToleranciaPct:  ptrF(5),
	})
	if err != nil {
		t.Fatalf("fórmula: %v", err)
	}
	return out
}

func ptrF(v float64) *float64 { return &v }

// La receta está escrita para una TANDA. Multiplicar por la cantidad pedida sin
// dividir entre el lote base pide diez veces de más.
func TestFormula_LaRecetaEsPorTandaNoPorUnidad(t *testing.T) {
	svc, st := servicioFabricacion(t)
	p := formulaEstricta(t, svc, st)
	plan, err := svc.PlanearOrden(empDemo, sede1, p.SKU, 8)
	if err != nil {
		t.Fatalf("planear: %v", err)
	}
	// Para 8 unidades, con lote de 10 y rendimiento 80%: factor = 8/(10×0,8) = 1.
	if plan.Factor != 1 {
		t.Fatalf("el factor debería ser 1 (8 / (10 × 0,8)), es %v", plan.Factor)
	}
	// INS-B no tiene merma: 3 × 1 = 3.
	var b float64
	for _, c := range plan.Consumos {
		if c.SKU == "INS-B" {
			b = c.Cantidad
		}
	}
	if b != 3 {
		t.Fatalf("INS-B debería consumir 3, consume %v", b)
	}
}

// El rendimiento se aplica AL REVÉS de lo intuitivo: para obtener menos hay que
// meter más. Un proceso que rinde 80% necesita partir de 1,25× el insumo.
func TestFormula_ElRendimientoPideMasInsumoNoMenos(t *testing.T) {
	svc, st := servicioFabricacion(t)
	p := formulaEstricta(t, svc, st)
	// 10 unidades con lote 10 y rendimiento 80% ⇒ factor 1,25.
	plan, _ := svc.PlanearOrden(empDemo, sede1, p.SKU, 10)
	if plan.Factor != 1.25 {
		t.Fatalf("factor = %v, debería ser 1,25: para sacar 10 con 80%% de rendimiento hay que partir de más", plan.Factor)
	}
}

// La merma del insumo es SUYA, no del proceso: de un tomate se descarta el 10%
// entre en la receta que entre. Se saca más del almacén de lo que entra a la olla.
func TestFormula_LaMermaDelInsumoSacaMasDelAlmacen(t *testing.T) {
	svc, st := servicioFabricacion(t)
	p := formulaEstricta(t, svc, st)
	plan, _ := svc.PlanearOrden(empDemo, sede1, p.SKU, 8) // factor 1
	var a float64
	for _, c := range plan.Consumos {
		if c.SKU == "INS-A" {
			a = c.Cantidad
		}
	}
	// 2 × factor 1, más 10% que se descarta al pelarlo = 2,2.
	if a != 2.2 {
		t.Fatalf("INS-A debería sacar 2,2 del almacén (2 a la olla + 10%% de merma), saca %v", a)
	}
}

// Una tanda que se desvía más de lo declarado queda MARCADA. No se bloquea —ya
// salió— pero alguien tiene que mirarla.
func TestFormula_LaDesviacionFueraDeToleranciaQuedaMarcada(t *testing.T) {
	svc, st := servicioFabricacion(t)
	p := formulaEstricta(t, svc, st)

	dentro, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: p.SKU, Cantidad: 8, Actor: actorA, Origen: origenTst,
	})
	dentro, _ = svc.IniciarOrden(empDemo, dentro.ID, actorA, origenTst)
	dentro, _ = svc.TerminarOrden(empDemo, dentro.ID, 7.8, actorA, origenTst) // −2,5%, dentro del 5%
	if dentro.FueraDeTolerancia {
		t.Fatal("una desviación del 2,5% con tolerancia del 5% no debería marcarse")
	}

	fuera, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: p.SKU, Cantidad: 8, Actor: actorA, Origen: origenTst,
	})
	fuera, _ = svc.IniciarOrden(empDemo, fuera.ID, actorA, origenTst)
	fuera, _ = svc.TerminarOrden(empDemo, fuera.ID, 6, actorA, origenTst) // −25%
	if !fuera.FueraDeTolerancia {
		t.Fatal("una tanda que rindió 25% menos tiene que quedar señalada")
	}
}

// Y sin fórmula declarada todo se comporta como antes: es lo que permite que
// ninguna receta ya cargada cambie de conducta.
func TestFormula_SinDeclararNadaSeComportaComoAntes(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)
	plan, err := svc.PlanearOrden(empDemo, sede1, plato.SKU, 5)
	if err != nil {
		t.Fatalf("planear: %v", err)
	}
	if plan.Factor != 1*5 {
		t.Fatalf("sin lote ni rendimiento el factor es la cantidad pedida: %v", plan.Factor)
	}
	for _, c := range plan.Consumos {
		if c.SKU == "INS-A" && c.Cantidad != 10 { // 2 × 5, sin merma
			t.Fatalf("INS-A debería ser 10 sin fórmula declarada, es %v", c.Cantidad)
		}
	}
}

/* RESTAURANTE Y FABRICACIÓN, LOS DOS ACTIVOS.
 *
 * Conviven, y conviven POR PRODUCTO: el mismo local prepara la pasta al pedirla
 * y tiene los postres hechos en la vitrina. Lo que no puede pasar es que un
 * postre ya fabricado se mande a cocina — el cliente esperaría por algo que está
 * a tres metros, y la comanda quedaría abierta hasta que alguien marque listo un
 * plato que nadie va a preparar.
 */
func TestFabricacion_LoFabricadoParaStockNoVaACocina(t *testing.T) {
	svc, st := servicioFabricacion(t)
	svc.ConPedidos(st.Pedidos, st.CanalesPedido, st.ZonasPedido, st.Repartidores)
	svc.ConCuentas(st.Cuentas)

	bajoPedido, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaBajoPedido)
	if !bajoPedido.SePreparaAlPedirlo() {
		t.Fatal("un plato bajo pedido SÍ se prepara al pedirlo: tiene que ir a cocina")
	}

	paraStock, err := svc.ActualizarProducto(empDemo, actorA, origenTst, bajoPedido.SKU,
		application.CambiosProducto{ModoFabricacion: ptrS(inventario.FabricaParaStock)})
	if err != nil {
		t.Fatalf("cambiar modo: %v", err)
	}
	if paraStock.SePreparaAlPedirlo() {
		t.Fatal("un plato fabricado para stock ya está hecho: no va a cocina")
	}
	// Y sigue siendo un plato con receta: lo que cambió es CUÁNDO se produce.
	if !paraStock.EsPlato || len(paraStock.Receta) == 0 {
		t.Fatal("cambiar el modo no puede dejar de ser un plato con receta")
	}
}

func ptrS(v string) *string { return &v }

/* EL PLATO FABRICADO QUE SE VENDE POR PESO.
 *
 * Una torta se produce «una torta» y se vende POR KILO. Hasta ahora era
 * imposible de representar: marcar un producto como plato le forzaba la unidad,
 * así que lo fabricado y lo vendido eran unidades distintas y el inventario no
 * podía cerrar.
 *
 * La restricción tenía sentido cuando todo plato era bajo pedido —sin existencia,
 * la unidad es nominal— y dejó de tenerlo cuando un plato puede ser mercancía.
 */
func TestFormula_UnPlatoFabricadoSePuedeVenderPorPeso(t *testing.T) {
	svc, st := servicioFabricacion(t)
	plato, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)

	porPeso, err := svc.ActualizarProducto(empDemo, actorA, origenTst, plato.SKU,
		application.CambiosProducto{
			TipoVenta:  inventario.TipoVentaPeso,
			UnidadBase: inventario.UnidadKg,
		})
	if err != nil {
		t.Fatalf("pasar a peso: %v", err)
	}
	if porPeso.TipoVenta != inventario.TipoVentaPeso || porPeso.UnidadBase != inventario.UnidadKg {
		t.Fatalf("un plato fabricado para stock tiene que poder venderse por kilo: %q / %q",
			porPeso.TipoVenta, porPeso.UnidadBase)
	}

	// Y el bajo pedido sigue forzado a unidad: sin existencia, su unidad es nominal.
	bajo, _, _ := recetaDePrueba2(t, svc)
	if bajo.TipoVenta != inventario.TipoVentaUnidad {
		t.Fatalf("un plato bajo pedido se sigue vendiendo por unidad: %q", bajo.TipoVenta)
	}
}

/* UNA RECETA PUEDE USAR OTRO PREPARADO, si ese se fabrica para stock.
 *
 * La salsa se produce con su propia orden, entra al inventario, y el sándwich la
 * consume como cualquier insumo. Lo que sigue prohibido es anidar uno BAJO
 * PEDIDO: ese no tiene existencia, así que la orden pediría algo que no está en
 * ningún estante.
 */
func TestFormula_UnPreparadoParaStockPuedeSerInsumoDeOtro(t *testing.T) {
	svc, st := servicioFabricacion(t)
	salsa, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)

	_, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		EmpresaID: empDemo, SKU: "SANDWICH", Nombre: "Sándwich", EsPlato: true, Activo: true,
		Precio: 300, Receta: []inventario.ComboComponente{{SKU: salsa.SKU, Cantidad: 0.05}},
	})
	if err != nil {
		t.Fatalf("un preparado para stock sí puede ser insumo: %v", err)
	}

	// El bajo pedido, no.
	bajo, _, _ := recetaDePrueba2(t, svc)
	_, err = svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		EmpresaID: empDemo, SKU: "OTRO", Nombre: "Otro", EsPlato: true, Activo: true,
		Precio: 100, Receta: []inventario.ComboComponente{{SKU: bajo.SKU, Cantidad: 1}},
	})
	if err == nil {
		t.Fatal("anidar un plato bajo pedido no puede permitirse: no hay existencia que consumir")
	}
}

// recetaDePrueba2 crea un segundo plato bajo pedido con SKUs propios, para poder
// tener los dos modos vivos en la misma prueba.
func recetaDePrueba2(t *testing.T, svc *application.Service) (inventario.Producto, string, string) {
	t.Helper()
	p, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		EmpresaID: empDemo, SKU: "PASTA", Nombre: "Pasta al momento", EsPlato: true,
		Precio: 200, Activo: true,
		Receta: []inventario.ComboComponente{{SKU: "INS-A", Cantidad: 1}},
	})
	if err != nil {
		t.Fatalf("plato bajo pedido: %v", err)
	}
	return p, "INS-A", ""
}

/* LOS DOS MÓDULOS FUNCIONAN POR SEPARADO.
 *
 * Que puedan convivir no puede significar que se necesiten. Un taller activa
 * Fabricación y nunca Restaurante; una cocina que prepara todo al momento activa
 * Restaurante y nunca Fabricación. La prueba recorre las dos soledades.
 */
func TestFabricacion_FuncionaSinElModuloRestaurante(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConFabricacion(st.OrdenesFabricacion)
	// Deliberadamente NO se cablea ConMesas ni ConCuentas: no hay restaurante.

	plato, _, _ := recetaDePrueba(t, svc, st, inventario.FabricaParaStock)
	o, err := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: plato.SKU, Cantidad: 5, Actor: actorA, Origen: origenTst,
	})
	if err != nil {
		t.Fatalf("fabricar sin restaurante tiene que poder: %v", err)
	}
	if o, err = svc.IniciarOrden(empDemo, o.ID, actorA, origenTst); err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if _, err := svc.TerminarOrden(empDemo, o.ID, 5, actorA, origenTst); err != nil {
		t.Fatalf("terminar: %v", err)
	}
	if got := stockDe(svc, plato.SKU); got != 5 {
		t.Fatalf("lo producido tiene que estar en existencia: %v", got)
	}
}

// Y al revés: un plato con receta se vende consumiendo sus insumos aunque el
// módulo de fabricación no exista.
func TestFabricacion_ElRestauranteFuncionaSinFabricacion(t *testing.T) {
	svc, st := nuevoServicio(t) // sin ConFabricacion
	plato, insA, _ := recetaDePrueba(t, svc, st, inventario.FabricaBajoPedido)
	antes := stockDe(svc, insA)

	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas:  []application.LineaEntrada{{SKU: plato.SKU, Cantidad: 2, PrecioUnitario: 500}},
		Pagos:   []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 1160, Moneda: "VES"}},
		SinCaja: true,
	}); err != nil {
		t.Fatalf("vender un plato sin el módulo de fabricación: %v", err)
	}
	if got := stockDe(svc, insA); got != antes-4 {
		t.Fatalf("sigue consumiendo sus insumos al venderse: %v → %v", antes, got)
	}
	// Y pedir una orden sin el módulo se niega con claridad, no con un pánico.
	if _, err := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: plato.SKU, Cantidad: 1, Actor: actorA, Origen: origenTst,
	}); err == nil {
		t.Fatal("sin el módulo cableado, crear una orden tiene que negarse")
	}
}

/* EL MODO NUNCA SE GUARDA VACÍO.
 *
 * «Vacío significa bajo pedido» era una convención que cada lector tenía que
 * recordar, y la pantalla la olvidó: abría una fórmula bajo pedido, veía el
 * campo vacío, caía a su propio default «para stock», y al guardar convertía el
 * producto en silencio — dejaba de consumir insumos al venderse sin que nadie lo
 * hubiera pedido.
 */
func TestFormula_ElModoSeGuardaExplicito(t *testing.T) {
	svc, st := servicioFabricacion(t)
	recetaDePrueba(t, svc, st, inventario.FabricaParaStock) // deja INS-A en el catálogo
	// Se crea SIN declarar modo, que es como lo mandaría un cliente viejo.
	p, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		EmpresaID: empDemo, SKU: "SIN-MODO", Nombre: "Preparado sin modo", EsPlato: true,
		Precio: 100, Activo: true,
		Receta: []inventario.ComboComponente{{SKU: "INS-A", Cantidad: 1}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if p.ModoFabricacion != inventario.FabricaBajoPedido {
		t.Fatalf("el modo tiene que quedar explícito, quedó %q", p.ModoFabricacion)
	}
	if p.SeFabricaParaStock() {
		t.Fatal("sin declarar nada, un producto con receta se prepara al venderlo")
	}
}

/* LO QUE SALIÓ MAL: descarte y merma anormal.
 *
 * Dos reglas que parecen contables y son de negocio: lo descartado no entra al
 * inventario —existió, pero nunca fue producto vendible— y la merma que se pasa
 * de la tolerancia NO la cargan los buenos, porque inflaría su costo y escondería
 * el problema dentro del margen.
 */
func TestFabricacion_LaTandaPerdidaEnteraSePuedeRegistrar(t *testing.T) {
	svc, st := servicioFabricacion(t)
	p := formulaEstricta(t, svc, st) // tolerancia 5%
	o, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: p.SKU, Cantidad: 8, Actor: actorA, Origen: origenTst,
	})
	o, _ = svc.IniciarOrden(empDemo, o.ID, actorA, origenTst)

	o, err := svc.CerrarOrden(empDemo, o.ID, actorA, origenTst, application.CierreOrden{
		Producida: 0,
		Resultados: []fabricacion.Resultado{
			{Cantidad: 8, Destino: fabricacion.DestinoPerdida, Motivo: "se cortó la masa"},
		},
	})
	if err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	// ANTES cero se leía como «no declaró nada» y se sustituía por lo planificado:
	// una pérdida total quedaba registrada como producción completa.
	if o.CantidadProducida != 0 {
		t.Fatalf("la tanda se perdió entera y quedó como %v producidas", o.CantidadProducida)
	}
	if got := stockDe(svc, p.SKU); got != 0 {
		t.Fatalf("nada salió bien: no puede haber entrado nada al inventario (%v)", got)
	}
	if o.PerdidaAnormal != o.CostoTotal {
		t.Fatalf("sin producto que lo cargue, el costo entero es pérdida: %v de %v", o.PerdidaAnormal, o.CostoTotal)
	}
	if o.NoLogrado() != 8 || o.EnDestino(fabricacion.DestinoPerdida) != 8 {
		t.Fatalf("lo no logrado tiene que quedar registrado con su destino: %+v", o.Resultados)
	}
}

// Un descarte sin explicación no sirve para decidir nada.
func TestFabricacion_LoNoLogradoExigeMotivo(t *testing.T) {
	svc, st := servicioFabricacion(t)
	p := formulaEstricta(t, svc, st)
	o, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: p.SKU, Cantidad: 8, Actor: actorA, Origen: origenTst,
	})
	o, _ = svc.IniciarOrden(empDemo, o.ID, actorA, origenTst)
	if _, err := svc.CerrarOrden(empDemo, o.ID, actorA, origenTst,
		application.CierreOrden{Producida: 6, Resultados: []fabricacion.Resultado{
			{Cantidad: 2, Destino: fabricacion.DestinoDescarte},
		}}); err == nil {
		t.Fatal("descartar sin motivo tenía que rechazarse")
	}
}

// Dentro de la tolerancia, los buenos cargan con todo —el pan bueno carga con el
// quemado—. Fuera, solo hasta lo que la fórmula declara normal.
func TestFabricacion_LosBuenosNoCarganLaMermaAnormal(t *testing.T) {
	svc, st := servicioFabricacion(t)
	p := formulaEstricta(t, svc, st) // tolerancia 5%

	dentro, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: p.SKU, Cantidad: 8, Actor: actorA, Origen: origenTst,
	})
	dentro, _ = svc.IniciarOrden(empDemo, dentro.ID, actorA, origenTst)
	dentro, _ = svc.CerrarOrden(empDemo, dentro.ID, actorA, origenTst,
		application.CierreOrden{Producida: 7.8})
	if dentro.PerdidaAnormal != 0 {
		t.Fatalf("dentro de la tolerancia no hay pérdida anormal: %v", dentro.PerdidaAnormal)
	}
	if dentro.CostoUnitario != round2Fab(dentro.CostoTotal/7.8) {
		t.Fatal("dentro de la tolerancia, los buenos cargan el costo entero")
	}

	fuera, _ := svc.CrearOrdenFabricacion(empDemo, application.EntradaOrden{
		SedeID: sede1, SKU: p.SKU, Cantidad: 8, Actor: actorA, Origen: origenTst,
	})
	fuera, _ = svc.IniciarOrden(empDemo, fuera.ID, actorA, origenTst)
	fuera, _ = svc.CerrarOrden(empDemo, fuera.ID, actorA, origenTst,
		application.CierreOrden{Producida: 5, Resultados: []fabricacion.Resultado{
			{Cantidad: 2, Destino: fabricacion.DestinoDescarte, Motivo: "se pasaron de horno"},
			{Cantidad: 1, Destino: fabricacion.DestinoReproceso, Motivo: "se puede rehacer"},
		}})
	if fuera.PerdidaAnormal <= 0 {
		t.Fatal("una tanda 37% por debajo tiene pérdida anormal: no la cargan los buenos")
	}
	// El costo unitario se calcula sobre lo NORMAL (8 × 0,95 = 7,6), no sobre 5.
	if esperado := round2Fab(fuera.CostoTotal / 7.6); fuera.CostoUnitario != esperado {
		t.Fatalf("el costo unitario debería salir de lo normalmente esperable (%v), es %v",
			esperado, fuera.CostoUnitario)
	}
}
