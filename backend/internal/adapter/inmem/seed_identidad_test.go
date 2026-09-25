package inmem_test

import (
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* LA IDENTIDAD DE LO SEMBRADO.
 *
 * El seed es la única parte del sistema que escribe en el ledger sin pasar por la
 * capa de aplicación, y sus filas terminan en una base PERSISTENTE. Que dos de sus
 * movimientos compartan id no rompe ninguna suma, pero envenena todo lo que indexa
 * por id —los asientos, la recontabilización, el rastro de lotes— y hace que los
 * guards de «esto ya está sembrado» salten filas legítimas.
 *
 * Llegó a producción: seis ids compartidos en la demo del restaurante.
 */

// TestSeed_NingunIDDeMovimientoSeRepite: dentro de una siembra, ni entre empresas.
func TestSeed_NingunIDDeMovimientoSeRepite(t *testing.T) {
	st := inmem.New()
	vistos := map[string]inventario.Movimiento{}
	total := 0
	for _, n := range inmem.NichosDemo() {
		for _, m := range st.Movimientos.List(n.EmpresaID, inventario.FiltroMovimiento{}) {
			total++
			if otro, repe := vistos[m.ID]; repe {
				t.Errorf("el id %s lo usan dos movimientos: %s (%s) y %s (%s)",
					m.ID, otro.SKU, otro.EmpresaID, m.SKU, m.EmpresaID)
			}
			vistos[m.ID] = m
		}
	}
	if total == 0 {
		t.Fatal("el seed no sembró ningún movimiento; el test no probó nada")
	}
}

// TestSeed_LosIDsNoDependenDelArranque es la propiedad que faltaba, y la que
// explica cómo se coló la colisión: el contador de `nextID` es del PROCESO y
// arranca en cero cada vez, así que dos arranques que siembran cosas distintas
// recorren la misma secuencia y emiten los mismos ids. Como el seed se inserta en
// una base persistente a lo largo de muchos arranques, ahí se encuentran.
//
// Dos siembras independientes tienen que producir los MISMOS ids para los mismos
// movimientos: es lo que hace que «¿ya está sembrado este id?» signifique algo.
func TestSeed_LosIDsNoDependenDelArranque(t *testing.T) {
	ids := func() []string {
		st := inmem.New()
		out := []string{}
		for _, n := range inmem.NichosDemo() {
			for _, m := range st.Movimientos.List(n.EmpresaID, inventario.FiltroMovimiento{}) {
				out = append(out, m.ID)
			}
		}
		return out
	}
	a, b := ids(), ids()
	if len(a) != len(b) {
		t.Fatalf("dos siembras dieron distinta cantidad de movimientos: %d y %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("el movimiento %d cambió de id entre siembras: %s vs %s", i, a[i], b[i])
		}
	}
}

// TestSeed_NadaNaceEnNegativo: una demostración que arranca con existencia negativa
// enseña el producto roto. La vaselina de la farmacia salía en −726,73 porque el
// inventario inicial (6.000 g) no cubría lo que consumían sus cuatro órdenes de
// fabricación (6.726,73): nadie sumó las dos cifras al escribir los datos, y como
// el ledger es append-only y admite negativos sin quejarse, nada lo dijo.
func TestSeed_NadaNaceEnNegativo(t *testing.T) {
	st := inmem.New()
	for _, n := range inmem.NichosDemo() {
		saldo := map[string]float64{}
		sku := map[string]string{}
		for _, m := range st.Movimientos.List(n.EmpresaID, inventario.FiltroMovimiento{}) {
			saldo[m.ProductoID] += m.Cantidad
			sku[m.ProductoID] = m.SKU
		}
		for prod, cant := range saldo {
			if cant < -0.005 {
				t.Errorf("%s: %s nace con existencia %.2f", n.Giro, sku[prod], cant)
			}
		}
	}
}
