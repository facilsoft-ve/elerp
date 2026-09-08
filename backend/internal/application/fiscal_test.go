package application_test

import (
	"errors"
	"math"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// casi compara importes con tolerancia de medio céntimo: los totales fiscales se
// calculan en float64 y la igualdad exacta sería una prueba frágil, no estricta.
func casi(a, b float64) bool { return math.Abs(a-b) < 0.005 }

// bptr/sptr construyen punteros a bool/string para poblar los PATCH parciales
// (CambiosProducto), donde nil = "campo no enviado".
func bptr(b bool) *bool     { return &b }
func sptr(s string) *string { return &s }

// abrirTurno deja una caja abierta para el actor, que es precondición de emitir.
func abrirTurno(t *testing.T, svc *application.Service, actor string) {
	t.Helper()
	if _, err := svc.AbrirCaja(empDemo, actor, origenTst, caja1, "OP-001", "1234"); err != nil {
		t.Fatalf("preparación del turno: %v", err)
	}
}

// primerSKU toma un producto GRAVADO real del seed en lugar de inventar un
// código. Gravado a propósito: el seed tiene productos exentos de IVA (la cesta
// básica venezolana) y las pruebas de IVA/IGTF necesitan uno que sí lo cause.
func primerSKU(t *testing.T, svc *application.Service) string {
	t.Helper()
	for _, p := range svc.Productos(empDemo) {
		if !p.ExentoIVA {
			return p.SKU
		}
	}
	t.Fatal("el seed debería traer al menos un producto gravado")
	return ""
}

func TestEmitirFactura_RechazaSinCajaAbierta(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 10}},
	})
	if !errors.Is(err, application.ErrSinCajaAbierta) {
		t.Fatalf("sin turno abierto debía rechazar con ErrSinCajaAbierta, se obtuvo: %v", err)
	}
}

func TestEmitirFactura_RechazaTurnoDeOtraSede(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA) // turno en Sede Principal
	sku := primerSKU(t, svc)
	// Se intenta facturar en Sede Este con un turno de Sede Principal.
	_, err := svc.EmitirFactura(empDemo, sede2, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 10}},
	})
	if !errors.Is(err, application.ErrCajaOtraSede) {
		t.Fatalf("se esperaba ErrCajaOtraSede, se obtuvo: %v", err)
	}
}

func TestEmitirFactura_GrabaElArqueo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 2, PrecioUnitario: 50}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if doc.CajaCodigo != "C-001" || doc.CajeroNombre != "Luis Marcano" || doc.SesionCajaID == "" {
		t.Errorf("el documento debe guardar quién cobró y desde qué caja, se obtuvo %+v",
			[]any{doc.CajaCodigo, doc.CajeroNombre, doc.SesionCajaID})
	}
	if doc.NumeroCompleto == "" {
		t.Error("el documento debe salir numerado")
	}
}

func TestEmitirFactura_CalculaIVA(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 2, PrecioUnitario: 50}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if !casi(doc.Subtotal, 100) {
		t.Errorf("subtotal esperado 100, se obtuvo %v", doc.Subtotal)
	}
	if !casi(doc.IVA, 16) {
		t.Errorf("IVA al 16%% esperado 16, se obtuvo %v", doc.IVA)
	}
	if !casi(doc.Total, 116) {
		t.Errorf("total esperado 116, se obtuvo %v", doc.Total)
	}
}

func TestEmitirFactura_PrecioEnBsPorPOSNoSeReconvierte(t *testing.T) {
	// FIX doble conversión (R10): el POS/Ventas ya convierten el precio a Bs y lo
	// mandan como AUTORITATIVO (>= 0). El servidor NO debe volver a multiplicar por
	// la tasa aunque el producto tenga su precio de catálogo en US$ —antes lo hacía
	// y disparaba el total (un producto USD no se podía vender por POS).
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 800, ""); err != nil {
		t.Fatalf("tasa: %v", err)
	}
	// ELE-TV tiene precio de catálogo en US$; el front manda 120.000 Bs (=150 US$
	// × 800) como precio de línea ya convertido.
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "ELE-TV", Cantidad: 1, PrecioUnitario: 120_000}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 139_200, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	// Si se re-multiplicara por 800, el subtotal sería 96.000.000; debe quedar en
	// los 120.000 Bs que mandó el POS.
	if !casi(doc.Subtotal, 120_000) {
		t.Fatalf("el precio en Bs no debe re-multiplicarse por la tasa: esperado 120.000, se obtuvo %v", doc.Subtotal)
	}
}

func TestEmitirFactura_LineaGratisSeRespeta(t *testing.T) {
	// Contrato: PrecioUnitario 0 es una línea GRATIS legítima (p. ej. la que un
	// cupón deja en cero), NO "usar catálogo". El total la respeta en 0.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 0}},
	})
	if err != nil {
		t.Fatalf("emitir línea gratis: %v", err)
	}
	if !casi(doc.Subtotal, 0) || !casi(doc.Total, 0) {
		t.Errorf("una línea a precio 0 debe respetarse (total 0), se obtuvo subtotal=%v total=%v", doc.Subtotal, doc.Total)
	}
	if len(doc.Lineas) != 1 || !casi(doc.Lineas[0].PrecioUnitario, 0) {
		t.Errorf("la línea debe conservar el precio 0, se obtuvo %+v", doc.Lineas)
	}
}

func TestEmitirFactura_ContadoSinPagosSeRechaza(t *testing.T) {
	// FIX CxC colgada: una factura NO-crédito con cero pagos deja Cobrado 0 y
	// cargaría el total a CxC (1102) sin que la proyección de CxC lo vea. Se rechaza
	// con ErrCobroInsuficiente; el crédito es la única vía de emitir con saldo.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
	}); !errors.Is(err, application.ErrCobroInsuficiente) {
		t.Fatalf("una factura de contado sin pagos debe dar ErrCobroInsuficiente, se obtuvo: %v", err)
	}
	// Pagada completa: OK.
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("una factura de contado pagada completa debe emitirse: %v", err)
	}
	// Crédito sin pagos: OK (el saldo queda por cobrar; es la vía legítima).
	valido := ""
	for _, cl := range svc.Clientes(empDemo) {
		if cl.TipoDocumento == cliente.DocJ {
			valido = cl.ID
			break
		}
	}
	if valido == "" {
		t.Fatal("el seed debería traer un cliente J para la venta a crédito")
	}
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		ClienteID: valido, Credito: true,
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
	}); err != nil {
		t.Fatalf("una venta a crédito sin pagos debe emitirse (queda por cobrar): %v", err)
	}
}

func TestEmitirFactura_IGTFSoloSobreLaPorcionEnDivisas(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	// La tasa ya no la manda el POS: la pone el servidor (R9). Aquí se carga a
	// mano, que es el último recurso previsto cuando la fuente automática falla.
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 40, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	// Cobro mixto: parte en Bs, parte en USD. El IGTF (3%) grava solo la parte en
	// divisas, y se cobra en bolívares (condicionante fiscal de §8.2).
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		// El cobro tiene que cubrir el total CON el IGTF: 100 + IVA 16 + IGTF 1,20
		// = 117,20 Bs, de los cuales 40 entran como 1 US$.
		Pagos: []application.PagoEntrada{
			{Metodo: fiscal.PagoEfectivoBs, Monto: 77.20, Moneda: "VES"},
			{Metodo: fiscal.PagoEfectivoUSD, Monto: 1, Moneda: "USD"}, // 1 USD = 40 Bs
		},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if doc.IGTF <= 0 {
		t.Fatalf("un pago en divisas debe generar IGTF, se obtuvo %v", doc.IGTF)
	}
	// 40 Bs en divisas × 3% = 1,20 Bs.
	if !casi(doc.IGTF, 1.2) {
		t.Errorf("IGTF esperado 1,20 (3%% de 40 Bs), se obtuvo %v", doc.IGTF)
	}
	// El IGTF nunca se mezcla con el precio: el subtotal no lo incluye.
	if !casi(doc.Subtotal, 100) {
		t.Errorf("el IGTF no debe alterar el subtotal, se obtuvo %v", doc.Subtotal)
	}
}

func TestEmitirFactura_SinPagoEnDivisasNoHayIGTF(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if doc.IGTF != 0 {
		t.Errorf("un cobro solo en bolívares no genera IGTF, se obtuvo %v", doc.IGTF)
	}
}

func TestEmitirFactura_RechazaDocumentoVacio(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{}); err == nil {
		t.Error("una factura sin líneas debía rechazarse")
	}
}

func TestEmitirFactura_NumeracionCorrelativaYSinSaltos(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	vistos := map[string]bool{}
	for i := 0; i < 5; i++ {
		doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
			Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 10}},
			Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 11.6, Moneda: "VES"}},
		})
		if err != nil {
			t.Fatalf("emisión %d: %v", i, err)
		}
		if vistos[doc.NumeroCompleto] {
			t.Fatalf("número fiscal duplicado: %s", doc.NumeroCompleto)
		}
		vistos[doc.NumeroCompleto] = true
	}
}

func TestEmitirFactura_DescuentaInventario(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)

	antes := 0.0
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			antes = e.Cantidad
		}
	}
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 3, PrecioUnitario: 10}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 34.8, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("emitir: %v", err)
	}
	despues := 0.0
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			despues = e.Cantidad
		}
	}
	if !casi(antes-despues, 3) {
		t.Errorf("vender 3 unidades debe bajar el stock en 3: antes %v, después %v", antes, despues)
	}
}

func TestAnular_NoEditaElOriginalYReponeStock(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 2, PrecioUnitario: 10}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 23.2, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	stockTrasVenta := 0.0
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			stockTrasVenta = e.Cantidad
		}
	}

	rev, err := svc.AnularDocumento(empDemo, sede1, actorA, origenTst, doc.ID, "prueba")
	if err != nil {
		t.Fatalf("anular: %v", err)
	}
	// Integridad fiscal: la anulación es un documento NUEVO que referencia al
	// original; el original no se edita ni se borra.
	if rev.ID == doc.ID {
		t.Error("la anulación debe ser un documento nuevo, no el mismo")
	}
	if rev.RefDocumentoID != doc.ID {
		t.Errorf("la reversa debe referenciar al original %q, referencia %q", doc.ID, rev.RefDocumentoID)
	}
	if orig, ok := svc.Documento(empDemo, doc.ID); !ok {
		t.Error("el documento original debe seguir existiendo")
	} else if !casi(orig.Total, doc.Total) {
		t.Error("el total del original no puede cambiar al anularlo")
	}
	// Y el stock vuelve.
	stockTrasAnular := 0.0
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			stockTrasAnular = e.Cantidad
		}
	}
	if !casi(stockTrasAnular-stockTrasVenta, 2) {
		t.Errorf("anular debe reponer las 2 unidades: %v → %v", stockTrasVenta, stockTrasAnular)
	}
}

func TestDisponibilidad_RecorreLasSedesYSemaforiza(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	refs := []application.SedeRef{{ID: sede1, Nombre: "Sede Principal"}, {ID: sede2, Nombre: "Sede Este"}}
	v, err := svc.Disponibilidad(empDemo, sede1, sku, refs)
	if err != nil {
		t.Fatalf("disponibilidad: %v", err)
	}
	if len(v.Sedes) != 2 {
		t.Fatalf("se esperaban 2 sedes, llegaron %d", len(v.Sedes))
	}
	total := 0.0
	estaTienda := 0
	for _, s := range v.Sedes {
		total += s.Cantidad
		if s.EstaTienda {
			estaTienda++
			if s.SedeID != sede1 {
				t.Errorf("«esta tienda» debe ser la sede consultada, marcó %q", s.SedeID)
			}
		}
		switch {
		case s.Cantidad <= 0 && s.Nivel != "agotado":
			t.Errorf("sin stock el nivel debe ser agotado, es %q", s.Nivel)
		case s.Cantidad > application.UmbralStockBajo && s.Nivel != "disponible":
			t.Errorf("con stock holgado el nivel debe ser disponible, es %q", s.Nivel)
		}
	}
	if estaTienda != 1 {
		t.Errorf("debe marcarse exactamente una sede como «esta tienda», se marcaron %d", estaTienda)
	}
	if !casi(v.TotalRed, total) {
		t.Errorf("el total de la red (%v) debe ser la suma de las sedes (%v)", v.TotalRed, total)
	}
}

func TestDisponibilidad_ProductoInexistente(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.Disponibilidad(empDemo, sede1, "NO-EXISTE", []application.SedeRef{{ID: sede1}}); err == nil {
		t.Error("un SKU inexistente debía dar error")
	}
}

func TestEmitirFactura_NoCobraIVASobreProductosExentos(t *testing.T) {
	// La harina de maíz está exenta en Venezuela: facturarle 16% es un error
	// fiscal. El documento debe separar base imponible de base exenta.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "HAR-001", Cantidad: 2, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 200, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if doc.IVA != 0 {
		t.Errorf("un producto exento no genera IVA, se obtuvo %v", doc.IVA)
	}
	if !casi(doc.BaseExenta, 200) || doc.BaseImponible != 0 {
		t.Errorf("base exenta 200 / imponible 0, se obtuvo %v / %v", doc.BaseExenta, doc.BaseImponible)
	}
	if !casi(doc.Total, 200) {
		t.Errorf("total esperado 200 sin IVA, se obtuvo %v", doc.Total)
	}
	if !doc.Lineas[0].Exento {
		t.Error("la línea debe quedar marcada como exenta en el documento")
	}
}

func TestEmitirFactura_MezclaGravadoYExento(t *testing.T) {
	// Una compra real mezcla las dos cosas: el IVA sale SOLO de la parte gravada.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{
			{SKU: "HAR-001", Cantidad: 1, PrecioUnitario: 100}, // exento
			{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100},  // gravado
		},
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 216, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if !casi(doc.IVA, 16) {
		t.Errorf("IVA esperado 16 (16%% de los 100 gravados), se obtuvo %v", doc.IVA)
	}
	if !casi(doc.Total, 216) {
		t.Errorf("total esperado 216, se obtuvo %v", doc.Total)
	}
}

func TestEmitirFactura_CalculaElVueltoEnBolivares(t *testing.T) {
	// El cliente paga 500 por un total de 232: el vuelto lo calcula el servidor y
	// queda guardado, porque es dinero que sale de la gaveta.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 2, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if !casi(doc.Total, 232) {
		t.Fatalf("total esperado 232 (200 + IVA), se obtuvo %v", doc.Total)
	}
	if !casi(doc.Cobrado, 500) {
		t.Errorf("cobrado esperado 500, se obtuvo %v", doc.Cobrado)
	}
	if !casi(doc.Vuelto, 268) || doc.VueltoMoneda != "VES" {
		t.Errorf("vuelto esperado 268 Bs, se obtuvo %v %s", doc.Vuelto, doc.VueltoMoneda)
	}
}

func TestEmitirFactura_ElVueltoDeEfectivoEnDivisasVaEnDivisas(t *testing.T) {
	// Si el cliente pagó con un billete de 20 US$, el vuelto que la caja tiene
	// que entregar se expresa en dólares, no en bolívares.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 100, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoUSD, Monto: 20, Moneda: "USD"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	// 100 + IVA 16 = 116. El IGTF grava la porción de la FACTURA pagada en
	// divisas (116 Bs), no los 2.000 Bs del billete: 3% de 116 = 3,48 → 119,48.
	if !casi(doc.Total, 119.48) {
		t.Fatalf("total esperado 119,48, se obtuvo %v", doc.Total)
	}
	if doc.VueltoMoneda != "USD" {
		t.Fatalf("el vuelto de un pago en efectivo en divisas va en USD, se obtuvo %q", doc.VueltoMoneda)
	}
	// Cobró 2.000 Bs, debía 119,48 → sobran 1.880,52 Bs = 18,81 US$.
	if !casi(doc.Vuelto, 18.81) {
		t.Errorf("vuelto esperado 18,81 US$, se obtuvo %v", doc.Vuelto)
	}
}

func TestEmitirFactura_VueltoDeclaradoEnBolivaresAunqueCobreEnDivisas(t *testing.T) {
	// La caja cobró con un billete de 20 US$ pero NO tiene divisas para dar el
	// vuelto: lo declara en Bs. El excedente (1.880,52 Bs) se entrega tal cual en
	// bolívares, no convertido a dólares, por el método efectivo por defecto.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 100, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas:       []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:        []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoUSD, Monto: 20, Moneda: "USD"}},
		VueltoMoneda: "VES",
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	// Mismo total que en divisas: 119,48 Bs. Cobró 2.000 → sobran 1.880,52 Bs.
	if doc.VueltoMoneda != "VES" {
		t.Fatalf("el vuelto declarado en VES debe salir en VES, se obtuvo %q", doc.VueltoMoneda)
	}
	if !casi(doc.Vuelto, 1880.52) {
		t.Errorf("vuelto esperado 1.880,52 Bs, se obtuvo %v", doc.Vuelto)
	}
	if doc.VueltoMetodo != fiscal.VueltoEfectivo {
		t.Errorf("método de vuelto esperado efectivo, se obtuvo %q", doc.VueltoMetodo)
	}
}

func TestEmitirFactura_VueltoMonedaInvalidaEsRechazada(t *testing.T) {
	// Declarar el vuelto en una divisa que no está activa ni tiene tasa cargada
	// no se puede convertir: se rechaza con un error claro.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas:       []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 2, PrecioUnitario: 100}},
		Pagos:        []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
		VueltoMoneda: empresa.MonedaEUR,
	})
	if !errors.Is(err, application.ErrVueltoMonedaInvalida) {
		t.Fatalf("se esperaba ErrVueltoMonedaInvalida, se obtuvo: %v", err)
	}
}

func TestEmitirFactura_VueltoPorPagoMovilGuardaLosDatos(t *testing.T) {
	// El vuelto por pago móvil registra la instrucción de transferencia: banco,
	// cédula y teléfono del cliente. La ejecución real es una integración futura.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas:         []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 2, PrecioUnitario: 100}},
		Pagos:          []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
		VueltoMetodo:   fiscal.VueltoPagoMovil,
		VueltoBanco:    "0102",
		VueltoCedula:   "V-12345678",
		VueltoTelefono: "0414-1234567",
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if !casi(doc.Vuelto, 268) || doc.VueltoMoneda != "VES" {
		t.Errorf("vuelto esperado 268 Bs, se obtuvo %v %s", doc.Vuelto, doc.VueltoMoneda)
	}
	if doc.VueltoMetodo != fiscal.VueltoPagoMovil {
		t.Errorf("método esperado pago_movil, se obtuvo %q", doc.VueltoMetodo)
	}
	if doc.VueltoBanco != "0102" || doc.VueltoCedula != "V-12345678" || doc.VueltoTelefono != "0414-1234567" {
		t.Errorf("datos de pago móvil no guardados: %q %q %q", doc.VueltoBanco, doc.VueltoCedula, doc.VueltoTelefono)
	}
}

func TestEmitirFactura_VueltoPorPagoMovilSinDatosEsRechazado(t *testing.T) {
	// Sin banco/cédula/teléfono no se puede instruir la transferencia del vuelto.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas:       []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 2, PrecioUnitario: 100}},
		Pagos:        []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
		VueltoMetodo: fiscal.VueltoPagoMovil,
		VueltoBanco:  "0102",
		// falta cédula y teléfono
	})
	if !errors.Is(err, application.ErrVueltoPagoMovilDatos) {
		t.Fatalf("se esperaba ErrVueltoPagoMovilDatos, se obtuvo: %v", err)
	}
}

func TestEmitirFactura_SinDeclararVueltoSeComportaComoHoy(t *testing.T) {
	// Retrocompat: sin declarar nada, el vuelto de un efectivo en divisas sigue
	// saliendo en esa divisa y por método efectivo (idéntico al comportamiento
	// previo a la flexibilización).
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 100, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoUSD, Monto: 20, Moneda: "USD"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if doc.VueltoMoneda != "USD" || !casi(doc.Vuelto, 18.81) {
		t.Errorf("vuelto esperado 18,81 US$, se obtuvo %v %s", doc.Vuelto, doc.VueltoMoneda)
	}
	if doc.VueltoMetodo != fiscal.VueltoEfectivo {
		t.Errorf("método por defecto esperado efectivo, se obtuvo %q", doc.VueltoMetodo)
	}
}

func TestEmitirFactura_VueltoMixtoQueCuadra(t *testing.T) {
	// Vuelto MIXTO: se deben 268 Bs de vuelto y se reparten en US$1 en efectivo
	// (100 Bs a tasa 100) + 168 Bs por pago móvil. La suma en Bs cuadra con el
	// excedente, así que se acepta y las partes quedan guardadas.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 100, ""); err != nil {
		t.Fatalf("tasa: %v", err)
	}
	// 2×100 + IVA 16% (32) = 232. Paga 500 Bs ⇒ excedente 268 Bs.
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 2, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
		VueltoPartes: []application.VueltoParteEntrada{
			{Moneda: "USD", Metodo: fiscal.VueltoEfectivo, Monto: 1},
			{Moneda: "VES", Metodo: fiscal.VueltoPagoMovil, Monto: 168, Banco: "0102", Cedula: "V-12345678", Telefono: "0414-1234567"},
		},
	})
	if err != nil {
		t.Fatalf("emitir con vuelto mixto: %v", err)
	}
	if len(doc.VueltoPartes) != 2 {
		t.Fatalf("se esperaban 2 partes de vuelto, se obtuvieron %d (%+v)", len(doc.VueltoPartes), doc.VueltoPartes)
	}
	// Con vuelto mixto, Vuelto es el TOTAL en Bs y la moneda de resumen queda vacía.
	if !casi(doc.Vuelto, 268) || doc.VueltoMoneda != "" {
		t.Errorf("resumen mixto esperado 268 Bs sin moneda única, se obtuvo %v %q", doc.Vuelto, doc.VueltoMoneda)
	}
	p0, p1 := doc.VueltoPartes[0], doc.VueltoPartes[1]
	if p0.Moneda != "USD" || !casi(p0.Monto, 1) || !casi(p0.MontoBs, 100) {
		t.Errorf("parte USD esperada 1 US$ = 100 Bs, se obtuvo %+v", p0)
	}
	if p1.Metodo != fiscal.VueltoPagoMovil || !casi(p1.MontoBs, 168) || p1.Banco != "0102" {
		t.Errorf("parte pago móvil esperada 168 Bs con banco 0102, se obtuvo %+v", p1)
	}
}

func TestEmitirFactura_VueltoMixtoQueNoCuadraEsRechazado(t *testing.T) {
	// Las partes suman 200 Bs pero el excedente es 268: no cuadra, se rechaza
	// (misma severidad que un cobro que no cubre el total).
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 2, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
		VueltoPartes: []application.VueltoParteEntrada{
			{Moneda: "VES", Metodo: fiscal.VueltoEfectivo, Monto: 100},
			{Moneda: "VES", Metodo: fiscal.VueltoEfectivo, Monto: 100},
		},
	})
	if !errors.Is(err, application.ErrVueltoNoCuadra) {
		t.Fatalf("se esperaba ErrVueltoNoCuadra, se obtuvo: %v", err)
	}
}

func TestEmitirFactura_VueltoMixtoPagoMovilSinDatosEsRechazado(t *testing.T) {
	// Una parte por pago móvil sin banco/cédula/teléfono no se puede instruir.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 2, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
		VueltoPartes: []application.VueltoParteEntrada{
			{Moneda: "VES", Metodo: fiscal.VueltoEfectivo, Monto: 100},
			{Moneda: "VES", Metodo: fiscal.VueltoPagoMovil, Monto: 168, Banco: "0102"},
		},
	})
	if !errors.Is(err, application.ErrVueltoPagoMovilDatos) {
		t.Fatalf("se esperaba ErrVueltoPagoMovilDatos, se obtuvo: %v", err)
	}
}

func TestEmitirFactura_RechazaCobroQueNoCubreElTotal(t *testing.T) {
	// Registrar una factura cobrada de menos deja un descuadre que nadie puede
	// explicar después. El backend la rechaza aunque la interfaz lo permitiera.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 50, Moneda: "VES"}},
	})
	if !errors.Is(err, application.ErrCobroInsuficiente) {
		t.Fatalf("se esperaba ErrCobroInsuficiente, se obtuvo: %v", err)
	}
}

func TestEmitirFactura_ElIGTFNoGravaElVuelto(t *testing.T) {
	// El caso que estaba mal: un billete de 20 US$ para una compra chica hacía
	// que el IGTF se calculara sobre los 20 US$ completos en vez de sobre la
	// parte de la factura pagada en divisas. El vuelto no es un pago en divisas.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 100, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: primerSKU(t, svc), Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoUSD, Monto: 20, Moneda: "USD"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	// 3% de 116 (la factura), no 3% de 2.000 (el billete).
	if !casi(doc.IGTF, 3.48) {
		t.Errorf("IGTF esperado 3,48 (3%% de la factura), se obtuvo %v", doc.IGTF)
	}
}

func TestEmitirFactura_PagoEnEuroConvierteConLaTasaDelEuro(t *testing.T) {
	// Multimoneda: un cobro en euros se convierte con la tasa del EURO (no la del
	// dólar), el IGTF grava esa porción igual que con dólares, y el vuelto de un
	// efectivo en euros se devuelve en euros.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	// El euro no es la divisa por defecto: hay que habilitarlo y cargarle su tasa
	// manual (nunca hay BCV para algo que no sea el dólar).
	if err := svc.ActualizarMonedasActivas(empDemo, actorA, origenTst,
		[]empresa.MonedaActiva{{Codigo: empresa.MonedaEUR, Fuente: empresa.FuenteTasaManual}}); err != nil {
		t.Fatalf("activar EUR: %v", err)
	}
	// Se cargan tasas DISTINTAS para USD y EUR: si el backend usara la única del
	// dólar, la conversión saldría con 50 en vez de 120 y el test lo detectaría.
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 50, ""); err != nil {
		t.Fatalf("cargar tasa USD: %v", err)
	}
	if _, err := svc.CargarTasaManualEnMoneda(empDemo, actorA, origenTst, empresa.MonedaEUR, 120, ""); err != nil {
		t.Fatalf("cargar tasa EUR: %v", err)
	}
	// Compra de 100 + IVA 16 = 116 Bs. El cliente paga con un billete de 2 €.
	// 2 € × 120 = 240 Bs entregados.
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoUSD, Monto: 2, Moneda: empresa.MonedaEUR}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	// La tasa del EURO quedó grabada en el pago (no la del dólar).
	if len(doc.Pagos) != 1 || !casi(doc.Pagos[0].TasaCambio, 120) {
		t.Fatalf("el pago debe grabar la tasa del euro (120), se obtuvo %+v", doc.Pagos)
	}
	// IGTF: 3% de la porción en divisas topada a la factura (116 Bs) = 3,48.
	if !casi(doc.IGTF, 3.48) {
		t.Errorf("IGTF esperado 3,48 (3%% de 116 Bs pagados en euros), se obtuvo %v", doc.IGTF)
	}
	if !casi(doc.Total, 119.48) {
		t.Errorf("total esperado 119,48, se obtuvo %v", doc.Total)
	}
	// Cobró 240 Bs, debía 119,48 → sobran 120,52 Bs; en euros a 120 = 1,00 €.
	if doc.VueltoMoneda != empresa.MonedaEUR {
		t.Errorf("el vuelto de un efectivo en euros va en EUR, se obtuvo %q", doc.VueltoMoneda)
	}
	if !casi(doc.Vuelto, 1.00) {
		t.Errorf("vuelto esperado 1,00 €, se obtuvo %v", doc.Vuelto)
	}
	if !casi(doc.Cobrado, 240) {
		t.Errorf("cobrado esperado 240 Bs (2 € × 120), se obtuvo %v", doc.Cobrado)
	}
}

func TestEmitirFactura_PagoMixtoBsUsdEurCuadraConCadaTasa(t *testing.T) {
	// Cobro mixto en tres monedas: cada porción en divisa se convierte con SU
	// tasa. Cobrado (en Bs) = Σ de cada pago por su tasa, y el asiento cuadra.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	if err := svc.ActualizarMonedasActivas(empDemo, actorA, origenTst,
		[]empresa.MonedaActiva{{Codigo: empresa.MonedaEUR, Fuente: empresa.FuenteTasaManual}}); err != nil {
		t.Fatalf("activar EUR: %v", err)
	}
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 40, ""); err != nil {
		t.Fatalf("cargar tasa USD: %v", err)
	}
	if _, err := svc.CargarTasaManualEnMoneda(empDemo, actorA, origenTst, empresa.MonedaEUR, 50, ""); err != nil {
		t.Fatalf("cargar tasa EUR: %v", err)
	}
	// Factura: 100 + IVA 16 = 116 base para IGTF.
	// Divisas entregadas: 1 US$×40 + 1 €×50 = 90 Bs. IGTF = 3% de 90 = 2,70.
	// Total = 116 + 2,70 = 118,70. Se cobra el resto en Bs: 118,70 − 90 = 28,70.
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos: []application.PagoEntrada{
			{Metodo: fiscal.PagoEfectivoBs, Monto: 28.70, Moneda: "VES"},
			{Metodo: fiscal.PagoPagoMovil, Monto: 1, Moneda: empresa.MonedaUSD},   // 1 US$ = 40 Bs
			{Metodo: fiscal.PagoEfectivoUSD, Monto: 1, Moneda: empresa.MonedaEUR}, // 1 € = 50 Bs
		},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	// IGTF sobre 90 Bs en divisas (USD + EUR), no sobre una sola divisa.
	if !casi(doc.IGTF, 2.70) {
		t.Errorf("IGTF esperado 2,70 (3%% de 90 Bs en divisas), se obtuvo %v", doc.IGTF)
	}
	if !casi(doc.Total, 118.70) {
		t.Errorf("total esperado 118,70, se obtuvo %v", doc.Total)
	}
	// Cobrado = 28,70 (Bs) + 40 (US$) + 50 (€) = 118,70 Bs, exactamente el total.
	if !casi(doc.Cobrado, 118.70) {
		t.Errorf("cobrado esperado 118,70 (Σ por tasa de cada divisa), se obtuvo %v", doc.Cobrado)
	}
	if !casi(doc.Vuelto, 0) {
		t.Errorf("el cobro cuadra justo, no debe haber vuelto, se obtuvo %v %s", doc.Vuelto, doc.VueltoMoneda)
	}
	// Cada pago conservó la tasa de su moneda.
	for _, pg := range doc.Pagos {
		switch pg.Moneda {
		case empresa.MonedaUSD:
			if !casi(pg.TasaCambio, 40) {
				t.Errorf("el pago en USD debe grabar tasa 40, se obtuvo %v", pg.TasaCambio)
			}
		case empresa.MonedaEUR:
			if !casi(pg.TasaCambio, 50) {
				t.Errorf("el pago en EUR debe grabar tasa 50, se obtuvo %v", pg.TasaCambio)
			}
		}
	}
	// El asiento derivado de la venta cuadra (Σ Debe == Σ Haber).
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefID == doc.ID && !a.Cuadra() {
			t.Fatalf("el asiento de la venta multimoneda no cuadra: %+v", a)
		}
	}
}

func TestPorCodigoBarras_ResuelveElEscaneo(t *testing.T) {
	// Lo que hace que la pistola lectora sirva: el código escaneado tiene que
	// resolver a un producto, sea el código del producto o el de su bulto.
	svc, _ := nuevoServicio(t)
	p, ok := svc.PorCodigoBarras(empDemo, "7591234000301")
	if !ok || p.SKU != "HAR-001" {
		t.Fatalf("el código propio debía resolver a HAR-001, se obtuvo %q (%v)", p.SKU, ok)
	}
	// Código de la presentación «Bulto x24» del mismo producto.
	p, ok = svc.PorCodigoBarras(empDemo, "7591234000241")
	if !ok || p.SKU != "HAR-001" {
		t.Fatalf("el código de la presentación debía resolver al producto, se obtuvo %q (%v)", p.SKU, ok)
	}
	if _, ok := svc.PorCodigoBarras(empDemo, "0000000000000"); ok {
		t.Error("un código inexistente no debe resolver a nada")
	}
	// Aislamiento de tenant: el código de otra empresa no se ve.
	if _, ok := svc.PorCodigoBarras("emp_otra", "7591234000301"); ok {
		t.Error("un código de otra empresa no debe resolver acá")
	}
}

func TestActualizarProducto_CodigoDeBarrasUnicoPorEmpresa(t *testing.T) {
	// Si dos productos comparten código, un escaneo es ambiguo y el cajero cobra
	// el producto equivocado.
	svc, _ := nuevoServicio(t)
	_, err := svc.ActualizarProducto(empDemo, actorA, origenTst, "REF-2L", application.CambiosProducto{
		Nombre: "Refresco 2L", Precio: 63.20, CodigoBarras: sptr("7591234000301"), Activo: bptr(true),
	})
	if !errors.Is(err, application.ErrCodigoBarrasEnUso) {
		t.Fatalf("se esperaba ErrCodigoBarrasEnUso, se obtuvo: %v", err)
	}
	// Un código libre sí se guarda, y el propio código del producto no choca.
	out, err := svc.ActualizarProducto(empDemo, actorA, origenTst, "REF-2L", application.CambiosProducto{
		Nombre: "Refresco 2L", Precio: 63.20, CodigoBarras: sptr("7591234099999"), Activo: bptr(true),
	})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.CodigoBarras != "7591234099999" {
		t.Errorf("el código no se guardó: %q", out.CodigoBarras)
	}
}

func TestActualizarProducto_CambiaLaCondicionDeIVA(t *testing.T) {
	// Cambiar un producto a exento tiene efecto fiscal inmediato en la próxima
	// factura, así que se verifica de punta a punta.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.ActualizarProducto(empDemo, actorA, origenTst, "REF-2L", application.CambiosProducto{
		Nombre: "Refresco 2L", Precio: 100, ExentoIVA: bptr(true), Activo: bptr(true),
	}); err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 100, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if doc.IVA != 0 || !casi(doc.BaseExenta, 100) {
		t.Errorf("tras marcarlo exento no debe cobrar IVA: iva %v, exenta %v", doc.IVA, doc.BaseExenta)
	}
}

// TestActualizarProducto_PATCHParcialConservaBooleanos verifica la semántica de
// PATCH parcial: un cuerpo que NO envía los booleanos (activo/exentoIva) conserva
// su valor actual en vez de pisarlo a false. Antes, el bind de un bool los ponía en
// false por omisión, así que un {"precio":999} dejaba el producto inactivo y sin
// exención (un exento por ley perdía su exención y desaparecía del POS).
func TestActualizarProducto_PATCHParcialConservaBooleanos(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// Estado de partida conocido: activo y exento de IVA.
	if _, err := svc.ActualizarProducto(empDemo, actorA, origenTst, "REF-2L", application.CambiosProducto{
		ExentoIVA: bptr(true), Activo: bptr(true),
	}); err != nil {
		t.Fatalf("preparar estado: %v", err)
	}
	// PATCH parcial: solo el precio. Los booleanos no viajan (nil = no cambiar).
	out, err := svc.ActualizarProducto(empDemo, actorA, origenTst, "REF-2L", application.CambiosProducto{
		Precio: 999,
	})
	if err != nil {
		t.Fatalf("patch parcial: %v", err)
	}
	if !out.Activo {
		t.Error("un PATCH sin `activo` no debía desactivar el producto")
	}
	if !out.ExentoIVA {
		t.Error("un PATCH sin `exentoIva` no debía quitar la exención")
	}
	if !casi(out.Precio, 999) {
		t.Errorf("el precio debía actualizarse a 999, se obtuvo %v", out.Precio)
	}
	// Enviar exentoIva=false explícito SÍ cambia la condición (y `activo` ausente se
	// conserva).
	out, err = svc.ActualizarProducto(empDemo, actorA, origenTst, "REF-2L", application.CambiosProducto{
		ExentoIVA: bptr(false),
	})
	if err != nil {
		t.Fatalf("patch exentoIva=false: %v", err)
	}
	if out.ExentoIVA {
		t.Error("exentoIva=false explícito debía quitar la exención")
	}
	if !out.Activo {
		t.Error("no se envió `activo`: debía conservarse activo")
	}
}

// TestActualizarProducto_PATCHParcialConservaCombo verifica que un PATCH que solo
// cambia el precio de un combo conserva su condición de combo y su receta: esCombo
// nil no lo pisa a false y componentes nil no vacía la receta.
func TestActualizarProducto_PATCHParcialConservaCombo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	out, err := svc.ActualizarProducto(empDemo, actorA, origenTst, comboSKU, application.CambiosProducto{
		Precio: 12345,
	})
	if err != nil {
		t.Fatalf("patch parcial de combo: %v", err)
	}
	if !out.EsCombo {
		t.Error("un PATCH sin `esCombo` no debía dejar de ser combo")
	}
	if len(out.Componentes) == 0 {
		t.Error("un PATCH sin `componentes` no debía vaciar la receta")
	}
	if !casi(out.Precio, 12345) {
		t.Errorf("el precio debía actualizarse, se obtuvo %v", out.Precio)
	}
}

// La emisión exige un RIF/cédula VÁLIDO del receptor cuando la venta identifica un
// cliente. El consumidor final (sin cliente) queda permitido sin RIF.
func TestEmitirFactura_ExigeReceptorValido(t *testing.T) {
	svc, st := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	linea := []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 10}}

	// Consumidor final (sin cliente): permitido.
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: linea,
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 11.6, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("el consumidor final debe poder facturar sin RIF: %v", err)
	}

	// Cliente con documento válido (del seed, ya normalizado): permitido.
	valido := ""
	for _, c := range svc.Clientes(empDemo) {
		if c.TipoDocumento == cliente.DocJ {
			valido = c.ID
			break
		}
	}
	if valido == "" {
		t.Fatal("el seed debería traer un cliente J")
	}
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		ClienteID: valido, Lineas: linea,
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 11.6, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("un cliente con RIF válido debe poder facturar: %v", err)
	}

	// Cliente con RIF de dígito verificador INVÁLIDO, inyectado directo saltando
	// CrearCliente (que ya lo rechazaría): la emisión debe negarse.
	st.Clientes.Create(cliente.Cliente{
		ID: "cli_rif_malo", EmpresaID: empDemo, Nombre: "RIF Malo, C.A.",
		TipoDocumento: cliente.DocJ, Documento: "12345678-9", // el DV correcto es 4
	})
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		ClienteID: "cli_rif_malo", Lineas: linea,
	}); !errors.Is(err, application.ErrReceptorDocumentoInvalido) {
		t.Fatalf("facturar a un cliente con DV inválido debe dar ErrReceptorDocumentoInvalido, se obtuvo: %v", err)
	}

	// ClienteID inexistente: error explícito (antes caía silenciosamente a consumidor final).
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		ClienteID: "cli_fantasma", Lineas: linea,
	}); !errors.Is(err, application.ErrClienteNoExiste) {
		t.Fatalf("facturar con un clienteId inexistente debe dar ErrClienteNoExiste, se obtuvo: %v", err)
	}
}
