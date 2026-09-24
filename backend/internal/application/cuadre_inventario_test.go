package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* EL INVENTARIO Y LA CONTABILIDAD TIENEN QUE CONTAR LA MISMA HISTORIA.
 *
 * Este archivo defiende una sola cosa, y es la que nadie ve venir: cuando falta el
 * asiento ENTERO de un movimiento —no una de sus patas—, el balance sigue cuadrando.
 * El descuadre no está entre el debe y el haber; está entre lo que hay en el almacén
 * y lo que dice la cuenta. Ninguna validación contable lo detecta. */

// recontabilizarTodoTest hace lo mismo que el arranque real, y en el mismo orden:
// primero los documentos —cuyos movimientos de inventario ya están cubiertos por el
// asiento del documento— y después los movimientos sueltos. Llamar solo a una de
// las dos deja la mitad de los hechos sin respaldo y el informe acusa un descuadre
// que no existe.
func recontabilizarTodoTest(t *testing.T, svc *application.Service) int {
	t.Helper()
	return svc.RecontabilizarPendientes(empDemo, "sistema")
}

// TestCuadre_LaDemoNaceCuadrada es la prueba del arreglo del seed.
//
// El seed anexa movimientos directamente al repositorio, saltándose la capa que
// deriva los asientos. Eso dejaba la demo arrancando descuadrada, y era lo primero
// que veía cualquiera que abriera el informe de valoración.
func TestCuadre_LaDemoNaceCuadrada(t *testing.T) {
	svc := servicioCompleto(t)
	// Lo que hace el arranque real, y por el mismo motivo.
	recontabilizarTodoTest(t, svc)

	v := svc.Valoracion(empDemo, "")
	for _, c := range v.PorCuenta {
		if !c.Cuadra {
			t.Errorf("la cuenta %s (%s) no cuadra: inventario %v, contabilidad %v, diferencia %v",
				c.Clave, c.Nombre, c.Valor, c.Contable, c.Diferencia)
		}
	}
	if !v.TodoCuadrado {
		t.Error("tras recontabilizar, el informe tenía que declararse cuadrado")
	}
}

// TestCuadre_RecontabilizarEsIdempotente: ejecutarla dos veces no puede duplicar
// asientos. Si los duplicara, el arreglo sería peor que el problema — y en un libro
// de solo-anexado no habría vuelta atrás.
func TestCuadre_RecontabilizarEsIdempotente(t *testing.T) {
	svc := servicioCompleto(t)

	primera := svc.RecontabilizarPendientes(empDemo, "sistema")
	saldoTras1 := saldoDeCuenta(t, svc, contabilidad.CtaInventario)
	asientosTras1 := len(svc.LibroDiario(empDemo))

	if segunda := svc.RecontabilizarPendientes(empDemo, "sistema"); segunda != 0 {
		t.Errorf("la segunda pasada no podía asentar nada, asentó %d", segunda)
	}
	if got := saldoDeCuenta(t, svc, contabilidad.CtaInventario); !casi(got, saldoTras1) {
		t.Errorf("el saldo no podía moverse: %v → %v", saldoTras1, got)
	}
	if got := len(svc.LibroDiario(empDemo)); got != asientosTras1 {
		t.Errorf("no podían aparecer asientos nuevos: %d → %d", asientosTras1, got)
	}
	if primera == 0 {
		t.Error("la primera pasada tenía que asentar algo: el seed anexa movimientos sin asiento")
	}
}

// TestCuadre_NoAsientaDosVecesLoQueYaTieneDocumento es la guarda que evita inflar
// la contabilidad: los movimientos de una venta o una compra ya están asentados por
// su documento, con el importe agregado de todas sus líneas. Asentarlos otra vez uno
// a uno duplicaría el costo de ventas de la empresa entera.
func TestCuadre_NoAsientaDosVecesLoQueYaTieneDocumento(t *testing.T) {
	svc := servicioCompleto(t)
	recontabilizarTodoTest(t, svc) // parte cuadrada

	sku := primerSKU(t, svc)
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 30}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst)
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 10}}); err != nil {
		t.Fatalf("recibir: %v", err)
	}

	saldoAntes := saldoDeCuenta(t, svc, contabilidad.CtaInventario)
	if n := recontabilizarTodoTest(t, svc); n != 0 {
		t.Errorf("la recepción ya tenía su asiento: no podía recibir otro (asentó %d)", n)
	}
	if got := saldoDeCuenta(t, svc, contabilidad.CtaInventario); !casi(got, saldoAntes) {
		t.Errorf("el saldo no podía moverse: %v → %v", saldoAntes, got)
	}
	// Y sigue cuadrando.
	for _, c := range svc.Valoracion(empDemo, "").PorCuenta {
		if !c.Cuadra {
			t.Errorf("tras la compra la cuenta %s dejó de cuadrar: %+v", c.Clave, c)
		}
	}
}

// TestCuadre_UnMovimientoDirectoSeDetectaYSeArregla recorre el caso completo: así
// es como se rompe en la vida real (un arreglo hecho contra la base) y así es como
// se detecta y se corrige.
func TestCuadre_UnMovimientoDirectoSeDetectaYSeArregla(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConUbicaciones(st.Ubicaciones)
	recontabilizarTodoTest(t, svc)

	sku := primerSKU(t, svc)
	p, _ := svc.ProductoPorSKU(empDemo, sku)
	// Alguien anexa stock directamente al repositorio, sin pasar por la aplicación.
	st.Movimientos.Append(inventario.Movimiento{
		EmpresaID: empDemo, SedeID: sede1, ProductoID: p.ID, SKU: p.SKU,
		Tipo: inventario.MovEntrada, Cantidad: 100, CostoUnitario: 25,
		Motivo: "carga directa contra la base", Fecha: ahoraFechaTest(),
	})

	// El informe lo delata: 100 × 25 = 2.500 de inventario sin respaldo contable.
	roto := svc.Valoracion(empDemo, "")
	if roto.TodoCuadrado {
		t.Fatal("un movimiento sin asiento tenía que descuadrar el informe")
	}
	// La revisión lo nombra SIN tocar nada.
	rev := svc.RevisarContabilidadDeInventario(empDemo)
	if len(rev.SinAsiento) != 1 || rev.SinAsiento[0].SKU != sku {
		t.Fatalf("la revisión tenía que encontrar exactamente ese movimiento: %+v", rev.SinAsiento)
	}
	if rev.Aplicado {
		t.Error("revisar no aplica nada")
	}
	if !casi(rev.SinAsiento[0].Valor, 2500) {
		t.Errorf("el valor sin respaldo eran 2.500: %v", rev.SinAsiento[0].Valor)
	}

	// Y la recontabilización lo arregla.
	if n := recontabilizarTodoTest(t, svc); n != 1 {
		t.Fatalf("tenía que asentar ese movimiento y solo ese: %d", n)
	}
	for _, c := range svc.Valoracion(empDemo, "").PorCuenta {
		if !c.Cuadra {
			t.Errorf("tras arreglarlo, %s tenía que cuadrar: %+v", c.Clave, c)
		}
	}
}
