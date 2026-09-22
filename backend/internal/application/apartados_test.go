package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* APARTADOS: mercancía comprometida que todavía no ha salido.
 *
 * La prueba que da sentido a todo el archivo es TestApartado_NoSeVendeDosVeces.
 * Sin apartados, entre que se prepara un pedido y se despacha la mercancía sigue
 * contando como existencia y el mostrador la vende otra vez. Nadie lo nota hasta
 * que el segundo cliente se queda sin su pedido — y para entonces las dos ventas
 * están hechas. */

// servicioConApartados cablea ubicaciones, operaciones y apartados.
func servicioConApartados(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConUbicaciones(st.Ubicaciones)
	svc.ConTiposOperacion(st.TiposOperacion)
	svc.ConApartados(st.Apartados)
	return svc
}

// venderTest emite una factura real. Es el camino que hay que probar: un ajuste
// NO se bloquea por un apartado —una merma es una merma— y usarlo como sucedáneo
// de una venta probaría lo contrario de lo que se quiere.
func venderTest(t *testing.T, svc *application.Service, sku string, cant float64) error {
	t.Helper()
	// Se paga de sobra: lo que esta prueba mira es el inventario, y una factura sin
	// cobro suficiente se rechaza antes de llegar ahí.
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: cant, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: cant * 200, Moneda: "VES"}},
	})
	return err
}

// cuantoHay devuelve solo la cantidad: en este archivo el costo no importa, lo
// que se vigila es cuántas unidades se pueden comprometer y vender.
func cuantoHay(t *testing.T, svc *application.Service, sku string) float64 {
	t.Helper()
	cant, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	return cant
}

// TestApartado_NoSeVendeDosVeces es la razón de ser de este módulo.
func TestApartado_NoSeVendeDosVeces(t *testing.T) {
	svc := servicioConApartados(t)
	alm := almacenPrincipalID(t, svc)
	sku := "APT-1"
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: sku, Nombre: "Producto apartable", Precio: 100,
	}); err != nil {
		t.Fatalf("crear producto: %v", err)
	}
	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, "", sku, "carga", 10, "", "", actorA, origenTst); err != nil {
		t.Fatalf("cargar: %v", err)
	}

	// Se apartan 8 para un cliente.
	if _, err := svc.CrearApartado(empDemo, actorA, origenTst, inventario.Apartado{
		SedeID: sede1, AlmacenID: alm, Motivo: "pedido de María",
		Lineas: []inventario.LineaApartado{{SKU: sku, Cantidad: 8}},
	}); err != nil {
		t.Fatalf("apartar: %v", err)
	}

	// La EXISTENCIA no cambia —la mercancía sigue ahí— pero el DISPONIBLE sí.
	if got := cuantoHay(t, svc, sku); !casi(got, 10) {
		t.Errorf("apartar no mueve unidades: la existencia debía seguir en 10, es %v", got)
	}
	if got := svc.DisponibleDe(empDemo, sede1, sku); !casi(got, 2) {
		t.Errorf("el disponible tenía que bajar a 2, es %v", got)
	}

	// LA PRUEBA: el mostrador intenta vender 5. Solo hay 2 libres.
	abrirTurno(t, svc, actorA)
	if err := venderTest(t, svc, sku, 5); !errors.Is(err, application.ErrApartadoSinDisponible) {
		t.Fatalf("vender 5 con 8 apartadas es vender dos veces lo mismo, debía negarse: %v", err)
	}

	// Lo que sí está libre se vende sin problema.
	if err := venderTest(t, svc, sku, 2); err != nil {
		t.Fatalf("las 2 libres sí se podían vender: %v", err)
	}
	if got := cuantoHay(t, svc, sku); !casi(got, 8) {
		t.Errorf("tras vender 2 quedan 8 (todas apartadas), hay %v", got)
	}
	if got := svc.DisponibleDe(empDemo, sede1, sku); !casi(got, 0) {
		t.Errorf("ya no queda nada libre, el disponible dice %v", got)
	}
}

// TestApartado_NoSeApartaDosVeces: la guarda al crear. Sin ella, dos apartados
// comprometen la misma unidad y nada falla hasta que el segundo va a despachar.
func TestApartado_NoSeApartaDosVeces(t *testing.T) {
	svc := servicioConApartados(t)
	alm := almacenPrincipalID(t, svc)
	sku := "APT-2"
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{SKU: sku, Nombre: "Otro", Precio: 10})
	svc.AjustarEnUbicacion(empDemo, sede1, alm, "", sku, "carga", 10, "", "", actorA, origenTst)

	if _, err := svc.CrearApartado(empDemo, actorA, origenTst, inventario.Apartado{
		SedeID: sede1, AlmacenID: alm, Motivo: "primero",
		Lineas: []inventario.LineaApartado{{SKU: sku, Cantidad: 7}},
	}); err != nil {
		t.Fatalf("primer apartado: %v", err)
	}
	if _, err := svc.CrearApartado(empDemo, actorA, origenTst, inventario.Apartado{
		SedeID: sede1, AlmacenID: alm, Motivo: "segundo",
		Lineas: []inventario.LineaApartado{{SKU: sku, Cantidad: 7}},
	}); !errors.Is(err, application.ErrApartadoSinDisponible) {
		t.Fatalf("el segundo apartado de 7 sobre 10 debía negarse: %v", err)
	}
	// Y lo que cabe, cabe.
	if _, err := svc.CrearApartado(empDemo, actorA, origenTst, inventario.Apartado{
		SedeID: sede1, AlmacenID: alm, Motivo: "segundo, más pequeño",
		Lineas: []inventario.LineaApartado{{SKU: sku, Cantidad: 3}},
	}); err != nil {
		t.Fatalf("3 sobre 3 libres sí cabían: %v", err)
	}
}

// TestApartado_DespacharSacaLoSuyo: el despacho tiene que poder sacar justo lo que
// él mismo comprometió — si su propia reserva le bloqueara, no podría despachar
// nunca y el apartado se volvería una trampa.
func TestApartado_DespacharSacaLoSuyo(t *testing.T) {
	svc := servicioConApartados(t)
	alm := almacenPrincipalID(t, svc)
	prep := nuevaUbicacion(t, svc, alm, "PREPARACION")
	sku := "APT-3"
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{SKU: sku, Nombre: "Para despachar", Precio: 10})
	svc.AjustarEnUbicacion(empDemo, sede1, alm, prep, sku, "carga", 12, "", "", actorA, origenTst)

	apt, err := svc.CrearApartado(empDemo, actorA, origenTst, inventario.Apartado{
		SedeID: sede1, AlmacenID: alm, Motivo: "pedido preparado",
		Lineas: []inventario.LineaApartado{{SKU: sku, Cantidad: 12, UbicacionID: prep}},
	})
	if err != nil {
		t.Fatalf("apartar: %v", err)
	}
	if got := svc.DisponibleDe(empDemo, sede1, sku); !casi(got, 0) {
		t.Fatalf("todo estaba apartado, el disponible debía ser 0: %v", got)
	}

	out, err := svc.DespacharApartado(empDemo, apt.ID, actorA, origenTst)
	if err != nil {
		t.Fatalf("despachar lo propio no podía fallar: %v", err)
	}
	if out.Estado != inventario.ApartadoDespachado {
		t.Errorf("tenía que quedar despachado, quedó %q", out.Estado)
	}
	if got := cuantoHay(t, svc, sku); !casi(got, 0) {
		t.Errorf("tras despachar las 12 no queda nada, hay %v", got)
	}
	// Y no se cuenta dos veces: despachado ya salió por el ledger.
	if got := svc.DisponibleDe(empDemo, sede1, sku); !casi(got, 0) {
		t.Errorf("el disponible no puede irse a negativo restando lo ya despachado: %v", got)
	}
	if got := saldoEn(t, svc, sku, prep); !casi(got, 0) {
		t.Errorf("la mercancía salió de preparación, ahí queda %v", got)
	}
	// Despachar dos veces no puede sacar el doble.
	if _, err := svc.DespacharApartado(empDemo, apt.ID, actorA, origenTst); !errors.Is(err, application.ErrApartadoCerrado) {
		t.Errorf("un apartado ya despachado no admite otro despacho: %v", err)
	}
}

// TestApartado_LiberarDevuelveElDisponible: soltar lo comprometido no mueve
// unidades; solo deja de bloquearlas.
func TestApartado_LiberarDevuelveElDisponible(t *testing.T) {
	svc := servicioConApartados(t)
	alm := almacenPrincipalID(t, svc)
	sku := "APT-4"
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{SKU: sku, Nombre: "Liberable", Precio: 10})
	svc.AjustarEnUbicacion(empDemo, sede1, alm, "", sku, "carga", 6, "", "", actorA, origenTst)

	apt, err := svc.CrearApartado(empDemo, actorA, origenTst, inventario.Apartado{
		SedeID: sede1, AlmacenID: alm, Motivo: "se lo pensó",
		Lineas: []inventario.LineaApartado{{SKU: sku, Cantidad: 6}},
	})
	if err != nil {
		t.Fatalf("apartar: %v", err)
	}
	antes := cuantoHay(t, svc, sku)
	if _, err := svc.LiberarApartado(empDemo, apt.ID, actorA, origenTst); err != nil {
		t.Fatalf("liberar: %v", err)
	}
	if got := cuantoHay(t, svc, sku); !casi(got, antes) {
		t.Errorf("liberar no mueve unidades: %v → %v", antes, got)
	}
	if got := svc.DisponibleDe(empDemo, sede1, sku); !casi(got, 6) {
		t.Errorf("al liberar vuelven las 6 al disponible, hay %v", got)
	}
	// Y ya no se puede volver a liberar ni despachar.
	if _, err := svc.LiberarApartado(empDemo, apt.ID, actorA, origenTst); !errors.Is(err, application.ErrApartadoCerrado) {
		t.Errorf("un apartado liberado ya está cerrado: %v", err)
	}
}

// TestApartado_SinCablearTodoSigueIgual: la regresión. Un servicio sin apartados
// se comporta exactamente como antes — el disponible es la existencia.
func TestApartado_SinCablearTodoSigueIgual(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes) // ...pero SIN ConApartados
	sku := primerSKU(t, svc)

	antes := cuantoHay(t, svc, sku)
	if got := svc.DisponibleDe(empDemo, sede1, sku); !casi(got, antes) {
		t.Errorf("sin apartados el disponible es la existencia: %v vs %v", got, antes)
	}
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "venta", -1, actorA, origenTst); err != nil {
		t.Fatalf("vender tenía que seguir funcionando igual: %v", err)
	}
	if n := len(svc.Apartados(empDemo, sede1)); n != 0 {
		t.Errorf("sin repositorio no hay apartados que listar, hay %d", n)
	}
}

// TestApartado_LoApartadoNoSurteAOtroAlmacen: apartar en un almacén no puede
// bloquear la mercancía que hay en otro. Si lo hiciera, apartar en el depósito
// dejaría a la tienda sin poder vender lo suyo.
func TestApartado_LoApartadoNoSurteAOtroAlmacen(t *testing.T) {
	svc := servicioConApartados(t)
	principal := almacenPrincipalID(t, svc)
	sku := "APT-5"
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{SKU: sku, Nombre: "En dos sitios", Precio: 10})
	svc.AjustarEnUbicacion(empDemo, sede1, principal, "", sku, "carga principal", 5, "", "", actorA, origenTst)

	// Todo lo del principal, apartado.
	if _, err := svc.CrearApartado(empDemo, actorA, origenTst, inventario.Apartado{
		SedeID: sede1, AlmacenID: principal, Motivo: "todo el principal",
		Lineas: []inventario.LineaApartado{{SKU: sku, Cantidad: 5}},
	}); err != nil {
		t.Fatalf("apartar: %v", err)
	}
	// Y una venta del principal ya no encuentra nada libre.
	abrirTurno(t, svc, actorA)
	if err := venderTest(t, svc, sku, 1); !errors.Is(err, application.ErrApartadoSinDisponible) {
		t.Errorf("con las 5 apartadas no debía poder venderse ninguna: %v", err)
	}
	// Pero un AJUSTE sí sale: una caja rota es una caja rota, esté apartada o no.
	// Negar la merma solo dejaría el inventario contando mercancía que ya no existe.
	if _, err := svc.Ajustar(empDemo, sede1, principal, sku, "caja rota", -1, actorA, origenTst); err != nil {
		t.Errorf("un ajuste no se bloquea por un apartado: %v", err)
	}
	// Tras la merma quedan 4 con 5 apartadas: el disponible se va a negativo y eso
	// es correcto y hay que verlo — significa que hay más comprometido que
	// mercancía, y alguien tiene que decidir qué apartado se recorta.
	if got := svc.DisponibleDe(empDemo, sede1, sku); !casi(got, -1) {
		t.Errorf("con 4 en el almacén y 5 apartadas el disponible es -1, dice %v", got)
	}
}
