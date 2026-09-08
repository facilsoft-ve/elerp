package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// comboSKU es el combo demo sembrado (Harina + Café + Leche).
const comboSKU = "COMBO-DESAYUNO"

// TestCrearComboOK crea un combo con componentes válidos y verifica que queda
// marcado como combo, con su receta, forzado a venta por unidad y sin heredar
// forma de venta peso.
func TestCrearComboOK(t *testing.T) {
	svc, _ := nuevoServicio(t)
	out, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "COMBO-TEST", Nombre: "Combo de prueba", Rubro: "Víveres",
		// Se envía peso a propósito: un combo debe forzarse a unidad.
		TipoVenta: inventario.TipoVentaPeso, Precio: 5000, EsCombo: true,
		Componentes: []inventario.ComboComponente{
			{SKU: "HAR-001", Cantidad: 1},
			{SKU: "CAF-500", Cantidad: 2},
		},
	})
	if err != nil {
		t.Fatalf("crear combo válido: %v", err)
	}
	if !out.EsCombo {
		t.Error("el producto debería quedar marcado como combo")
	}
	if len(out.Componentes) != 2 {
		t.Fatalf("se esperaban 2 componentes, se obtuvieron %d", len(out.Componentes))
	}
	if out.TipoVenta != inventario.TipoVentaUnidad || out.UnidadBase != inventario.UnidadUnidad {
		t.Errorf("un combo se vende por unidad, se obtuvo %q/%q", out.TipoVenta, out.UnidadBase)
	}
	if !casi(out.Precio, 5000) {
		t.Errorf("el precio propio del combo debe respetarse, se obtuvo %v", out.Precio)
	}
}

// TestCrearComboPrecioPorDefecto comprueba que un combo sin precio propio adopta
// como default la suma de sus componentes (editable por el usuario).
func TestCrearComboPrecioPorDefecto(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// HAR-001 = 890, CAF-500 = 3350 → suma = 4240.
	out, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "COMBO-SUM", Nombre: "Combo suma", EsCombo: true,
		Componentes: []inventario.ComboComponente{
			{SKU: "HAR-001", Cantidad: 1},
			{SKU: "CAF-500", Cantidad: 1},
		},
	})
	if err != nil {
		t.Fatalf("crear combo sin precio: %v", err)
	}
	if !casi(out.Precio, 4240) {
		t.Errorf("el precio por defecto debe ser la suma de componentes (4240), se obtuvo %v", out.Precio)
	}
}

// TestCrearComboRechazos cubre las validaciones: sin componentes, componente
// inexistente, combo anidado y cantidad no positiva.
func TestCrearComboRechazos(t *testing.T) {
	svc, _ := nuevoServicio(t)

	casos := []struct {
		nombre string
		p      inventario.Producto
		quiere error
	}{
		{"sin componentes", inventario.Producto{
			SKU: "COMBO-A", Nombre: "sin componentes", EsCombo: true,
		}, application.ErrComboSinComponentes},
		{"componente inexistente", inventario.Producto{
			SKU: "COMBO-B", Nombre: "inexistente", EsCombo: true,
			Componentes: []inventario.ComboComponente{{SKU: "NO-EXISTE", Cantidad: 1}},
		}, application.ErrComboComponenteInvalido},
		{"cantidad cero", inventario.Producto{
			SKU: "COMBO-C", Nombre: "cantidad cero", EsCombo: true,
			Componentes: []inventario.ComboComponente{{SKU: "HAR-001", Cantidad: 0}},
		}, application.ErrComboComponenteInvalido},
		{"combo anidado", inventario.Producto{
			SKU: "COMBO-D", Nombre: "anidado", EsCombo: true,
			Componentes: []inventario.ComboComponente{{SKU: comboSKU, Cantidad: 1}},
		}, application.ErrComboAnidado},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := svc.CrearProducto(empDemo, actorA, origenTst, c.p)
			if !errors.Is(err, c.quiere) {
				t.Fatalf("se esperaba %v, se obtuvo %v", c.quiere, err)
			}
		})
	}
}

// TestComboComponenteInactivo rechaza un combo cuyo componente está dado de baja:
// un componente inactivo no se puede vender.
func TestComboComponenteInactivo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// GAL-OLD está sembrado como inactivo (descontinuado).
	_, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "COMBO-INACT", Nombre: "con inactivo", EsCombo: true,
		Componentes: []inventario.ComboComponente{{SKU: "GAL-OLD", Cantidad: 1}},
	})
	if !errors.Is(err, application.ErrComboComponenteInvalido) {
		t.Fatalf("un componente inactivo debe rechazarse, se obtuvo %v", err)
	}
}

// TestComboExcluidoDeExistencias verifica que el combo demo NO aparece en la
// proyección de existencias (no se stockea) pero sus componentes sí.
func TestComboExcluidoDeExistencias(t *testing.T) {
	svc, _ := nuevoServicio(t)
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == comboSKU {
			t.Fatalf("un combo no debe listarse con existencia, apareció %+v", e)
		}
	}
	// Un componente normal sí está en existencias.
	cant, _ := existenciaDe(t, svc, empDemo, sede1, "HAR-001")
	if cant <= 0 {
		t.Error("el componente HAR-001 debería tener existencia en la sede principal")
	}
}

// TestComboNoAjustable comprueba que no se puede ajustar la existencia de un combo.
func TestComboNoAjustable(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, err := svc.Ajustar(empDemo, sede1, "", comboSKU, "conteo", 5, actorA, origenTst)
	if !errors.Is(err, application.ErrComboNoStockeable) {
		t.Fatalf("ajustar un combo debe fallar con ErrComboNoStockeable, se obtuvo %v", err)
	}
}

// TestComboNoTransferible comprueba que una transferencia rechaza una línea combo.
func TestComboNoTransferible(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, err := svc.CrearTransferencia(empDemo, actorA, origenTst, inventario.Transferencia{
		OrigenSedeID: sede1, DestinoSedeID: sede2,
		Lineas: []inventario.LineaTransferencia{{SKU: comboSKU, Cantidad: 1}},
	})
	if !errors.Is(err, application.ErrComboNoStockeable) {
		t.Fatalf("transferir un combo debe fallar con ErrComboNoStockeable, se obtuvo %v", err)
	}
}

// TestEmitirFacturaComboRechazado verifica que EmitirFactura rechaza una línea
// que referencia un combo (guarda de servidor: el frontend explota el combo en
// sus componentes antes de enviar) pero acepta normalmente las líneas de sus
// componentes.
func TestEmitirFacturaComboRechazado(t *testing.T) {
	svc, _ := nuevoServicio(t)

	// Una línea que es un combo: rechazada.
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true,
		Lineas:  []application.LineaEntrada{{SKU: comboSKU, Cantidad: 1}},
		Pagos:   []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 8500, Moneda: "VES"}},
	})
	if !errors.Is(err, application.ErrComboNoFacturable) {
		t.Fatalf("facturar un combo como línea debe fallar con ErrComboNoFacturable, se obtuvo %v", err)
	}

	// Las líneas de sus componentes (lo que envía el frontend explotado) sí se
	// facturan normalmente. HAR-001 es exento: total = 2 × 890 = 1780, sin IVA.
	const total = 1780.0
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true,
		Lineas:  []application.LineaEntrada{{SKU: "HAR-001", Cantidad: 2, PrecioUnitario: -1}}, // -1 = precio de catálogo
		Pagos:   []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: total, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("facturar la línea de un componente debe funcionar: %v", err)
	}
	if len(doc.Lineas) != 1 || doc.Lineas[0].SKU != "HAR-001" {
		t.Fatalf("se esperaba una línea de HAR-001, se obtuvo %+v", doc.Lineas)
	}
	if !casi(doc.Total, total) {
		t.Errorf("total esperado %v, se obtuvo %v", total, doc.Total)
	}
}
