package application

import (
	"testing"

	"github.com/mornix/elerp/internal/domain/inventario"
)

// Con stock negativo, un ingreso que cruza a positivo debe tomar su propio costo, no
// promediar contra el saldo <0 (que inflaba el costo promedio). Regresión del Alto de
// auditoría de inventario.
func TestFold_NoCorrompeElPromedioConStockNegativo(t *testing.T) {
	movs := []inventario.Movimiento{
		{Fecha: "2026-01-01T00:00:00Z", Cantidad: -5, CostoUnitario: 0},   // salida sin stock → saldo -5
		{Fecha: "2026-01-02T00:00:00Z", Cantidad: 10, CostoUnitario: 100}, // entrada 10 @ 100
	}
	cant, avg := fold(movs)
	if cant != 5 {
		t.Errorf("cantidad esperada 5, se obtuvo %v", cant)
	}
	if avg != 100 {
		t.Errorf("costo promedio esperado 100 (antes se corrompía a 200), se obtuvo %v", avg)
	}
}

// Camino normal (saldo siempre ≥0): el promedio ponderado sigue igual.
func TestFold_PromedioPonderadoNormal(t *testing.T) {
	movs := []inventario.Movimiento{
		{Fecha: "2026-01-01T00:00:00Z", Cantidad: 10, CostoUnitario: 100},
		{Fecha: "2026-01-02T00:00:00Z", Cantidad: 10, CostoUnitario: 200},
	}
	cant, avg := fold(movs)
	if cant != 20 || avg != 150 {
		t.Errorf("esperado (20, 150), se obtuvo (%v, %v)", cant, avg)
	}
}
