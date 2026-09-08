package inventario

import (
	"math"
	"testing"
)

// eqDinero compara importes con tolerancia de medio céntimo.
func eqDinero(a, b float64) bool { return math.Abs(a-b) < 0.005 }

// eqPeso compara kilogramos con tolerancia de media milésima (los productos por
// peso llevan hasta 3 decimales).
func eqPeso(a, b float64) bool { return math.Abs(a-b) < 0.0005 }

// foldKardex replica la CONVENCIÓN del ledger de inventario (ADR-03): la
// existencia y el costo promedio ponderado son PROYECCIONES plegadas desde los
// movimientos, con la convención de signo del dominio (cantidad > 0 suma,
// cantidad < 0 resta). La proyección real vive en application.Kardex; aquí se
// caracteriza la regla directamente sobre los tipos y constantes del dominio:
//
//   - entrada (cantidad >= 0): promedio = (saldo*avg + cantidad*costo) / saldoNuevo
//   - salida  (cantidad <  0): el promedio NO cambia (el costo de salida es el
//     promedio vigente = COGS); solo baja el saldo.
//
// Devuelve el saldo corriente y el costo promedio ponderado finales.
func foldKardex(movs []Movimiento) (saldo, avg float64) {
	for _, m := range movs {
		if m.Cantidad >= 0 {
			nuevo := saldo + m.Cantidad
			valor := saldo*avg + m.Cantidad*m.CostoUnitario
			if nuevo > 0 {
				avg = valor / nuevo
			}
			saldo = nuevo
		} else {
			saldo += m.Cantidad
		}
	}
	return saldo, avg
}

// TestFoldKardex_PromedioPonderado cubre el cálculo de saldo + costo promedio.
func TestFoldKardex_PromedioPonderado(t *testing.T) {
	casos := []struct {
		nombre    string
		movs      []Movimiento
		wantSaldo float64
		wantAvg   float64
		saldoPeso bool // comparar el saldo con tolerancia de peso (3 decimales)
	}{
		{
			nombre:    "ledger_vacio",
			movs:      nil,
			wantSaldo: 0,
			wantAvg:   0,
		},
		{
			nombre: "una_entrada",
			movs: []Movimiento{
				{Tipo: MovEntrada, Cantidad: 10, CostoUnitario: 2.00},
			},
			wantSaldo: 10,
			wantAvg:   2.00,
		},
		{
			// Dos entradas a distinto costo: (10*2 + 10*3) / 20 = 2.50.
			nombre: "dos_entradas_distinto_costo",
			movs: []Movimiento{
				{Tipo: MovEntrada, Cantidad: 10, CostoUnitario: 2.00},
				{Tipo: MovEntrada, Cantidad: 10, CostoUnitario: 3.00},
			},
			wantSaldo: 20,
			wantAvg:   2.50,
		},
		{
			// La salida no altera el promedio; solo baja el saldo.
			nombre: "salida_no_cambia_promedio",
			movs: []Movimiento{
				{Tipo: MovEntrada, Cantidad: 10, CostoUnitario: 2.00},
				{Tipo: MovEntrada, Cantidad: 10, CostoUnitario: 4.00}, // avg = 3.00
				{Tipo: MovSalida, Cantidad: -5, CostoUnitario: 0},
			},
			wantSaldo: 15,
			wantAvg:   3.00,
		},
		{
			// Promedio ponderado no trivial: (10*1.50 + 30*2.50) / 40 = 2.25.
			nombre: "promedio_ponderado_desigual",
			movs: []Movimiento{
				{Tipo: MovEntrada, Cantidad: 10, CostoUnitario: 1.50},
				{Tipo: MovEntrada, Cantidad: 30, CostoUnitario: 2.50},
			},
			wantSaldo: 40,
			wantAvg:   2.25,
		},
		{
			// Saldo cero tras vaciar el stock; el promedio calculado se conserva
			// pero el saldo es 0 (no hay reingreso que lo recalcule).
			nombre: "saldo_cero",
			movs: []Movimiento{
				{Tipo: MovEntrada, Cantidad: 10, CostoUnitario: 2.00},
				{Tipo: MovSalida, Cantidad: -10, CostoUnitario: 0},
			},
			wantSaldo: 0,
			wantAvg:   2.00,
		},
		{
			// Ajuste negativo (merma): cuenta como salida por la convención de signo.
			nombre: "ajuste_merma_negativo",
			movs: []Movimiento{
				{Tipo: MovEntrada, Cantidad: 10, CostoUnitario: 2.00},
				{Tipo: MovAjuste, Cantidad: -3, CostoUnitario: 0},
			},
			wantSaldo: 7,
			wantAvg:   2.00,
		},
		{
			// Venta fraccionaria (producto por peso): 5.5kg @ 1.80, se despachan
			// 1.250kg => saldo 4.250kg, promedio 1.80 intacto.
			nombre: "venta_fraccionaria_peso",
			movs: []Movimiento{
				{Tipo: MovEntrada, Cantidad: 5.5, CostoUnitario: 1.80},
				{Tipo: MovSalida, Cantidad: -1.250, CostoUnitario: 0},
			},
			wantSaldo: 4.250,
			wantAvg:   1.80,
			saldoPeso: true,
		},
		{
			// Entradas por peso a distinto costo: (2.000*3.00 + 3.000*4.00) / 5.000
			// = 18/5 = 3.60.
			nombre: "peso_promedio_ponderado",
			movs: []Movimiento{
				{Tipo: MovEntrada, Cantidad: 2.000, CostoUnitario: 3.00},
				{Tipo: MovEntrada, Cantidad: 3.000, CostoUnitario: 4.00},
			},
			wantSaldo: 5.000,
			wantAvg:   3.60,
			saldoPeso: true,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			saldo, avg := foldKardex(c.movs)
			saldoOK := eqDinero(saldo, c.wantSaldo)
			if c.saldoPeso {
				saldoOK = eqPeso(saldo, c.wantSaldo)
			}
			if !saldoOK {
				t.Errorf("saldo = %v; se esperaba %v", saldo, c.wantSaldo)
			}
			if !eqDinero(avg, c.wantAvg) {
				t.Errorf("costo promedio = %v; se esperaba %v", avg, c.wantAvg)
			}
		})
	}
}

// TestUnidades_Valores fija las unidades base soportadas.
func TestUnidades_Valores(t *testing.T) {
	casos := map[string]string{
		"unidad": UnidadUnidad,
		"kg":     UnidadKg,
		"litro":  UnidadLitro,
	}
	for esperado, got := range casos {
		if got != esperado {
			t.Errorf("unidad = %q; se esperaba %q", got, esperado)
		}
	}
}

// TestTipoVenta_Valores fija las formas de venta: por unidad (defecto) y por
// peso (kg). El vacío se lee como "unidad" en application (retrocompat).
func TestTipoVenta_Valores(t *testing.T) {
	if TipoVentaUnidad != "unidad" {
		t.Errorf("TipoVentaUnidad = %q; se esperaba %q", TipoVentaUnidad, "unidad")
	}
	if TipoVentaPeso != "peso" {
		t.Errorf("TipoVentaPeso = %q; se esperaba %q", TipoVentaPeso, "peso")
	}
}

// TestTiposMovimiento_Valores fija los tipos del ledger; se persisten.
func TestTiposMovimiento_Valores(t *testing.T) {
	casos := map[string]string{
		"entrada":       MovEntrada,
		"salida":        MovSalida,
		"ajuste":        MovAjuste,
		"transferencia": MovTransferencia,
	}
	for esperado, got := range casos {
		if got != esperado {
			t.Errorf("tipo de movimiento = %q; se esperaba %q", got, esperado)
		}
	}
}

// TestTiposMovimiento_Distintos garantiza que los tipos de movimiento sean
// distintos entre sí.
func TestTiposMovimiento_Distintos(t *testing.T) {
	tipos := []string{MovEntrada, MovSalida, MovAjuste, MovTransferencia}
	visto := map[string]bool{}
	for _, tp := range tipos {
		if visto[tp] {
			t.Errorf("tipo de movimiento duplicado: %q", tp)
		}
		visto[tp] = true
	}
}

// TestEstadosTransferencia_Valores fija los estados de la máquina de estados de
// transferencias, incluida "cancelada" (estado terminal fuera del avance lineal).
func TestEstadosTransferencia_Valores(t *testing.T) {
	casos := map[string]string{
		"borrador":    TransfBorrador,
		"despachada":  TransfDespachada,
		"en_transito": TransfEnTransito,
		"recibida":    TransfRecibida,
		"cerrada":     TransfCerrada,
		"cancelada":   TransfCancelada,
	}
	for esperado, got := range casos {
		if got != esperado {
			t.Errorf("estado de transferencia = %q; se esperaba %q", got, esperado)
		}
	}
}
