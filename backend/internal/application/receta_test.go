package application

import (
	"testing"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// consumosDeLinea expande una línea a lo que mueve el inventario: el propio SKU
// (producto normal) o los insumos × cantidad (plato con receta snapshot).
func TestConsumosDeLinea(t *testing.T) {
	// Producto normal: descuenta su propio SKU por la cantidad del renglón.
	normal := fiscal.Linea{SKU: "ARR-001", ProductoID: "p1", Cantidad: 3}
	cs := consumosDeLinea(normal, normal.Cantidad)
	if len(cs) != 1 || cs[0].SKU != "ARR-001" || cs[0].Cantidad != 3 {
		t.Fatalf("producto normal: esperaba 1 consumo ARR-001×3, obtuvo %+v", cs)
	}

	// Plato con receta: descuenta los insumos (cantidadUnitaria × cantidad), no el
	// plato.
	plato := fiscal.Linea{SKU: "PABELLON", ProductoID: "pp", Cantidad: 2, Insumos: []fiscal.InsumoLinea{
		{SKU: "ARR-001", ProductoID: "p1", CantidadUnitaria: 2},
		{SKU: "AZU-001", ProductoID: "p2", CantidadUnitaria: 1},
	}}
	cs = consumosDeLinea(plato, plato.Cantidad)
	if len(cs) != 2 {
		t.Fatalf("plato: esperaba 2 consumos (insumos), obtuvo %d", len(cs))
	}
	if cs[0].SKU != "ARR-001" || cs[0].Cantidad != 4 { // 2 por plato × 2 platos
		t.Errorf("insumo 1: esperaba ARR-001×4, obtuvo %s×%v", cs[0].SKU, cs[0].Cantidad)
	}
	if cs[1].SKU != "AZU-001" || cs[1].Cantidad != 2 { // 1 × 2
		t.Errorf("insumo 2: esperaba AZU-001×2, obtuvo %s×%v", cs[1].SKU, cs[1].Cantidad)
	}
	// El SKU del plato NUNCA aparece en el consumo (no se stockea).
	for _, c := range cs {
		if c.SKU == "PABELLON" {
			t.Errorf("el plato no debe descontar su propio SKU")
		}
	}
}
