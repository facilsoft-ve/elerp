package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

func stockEnSede(svc *application.Service, sede, sku string) float64 {
	for _, e := range svc.Existencias(empDemo, sede) {
		if e.SKU == sku {
			return e.Cantidad
		}
	}
	return 0
}

// Fiscal A1: la anulación reingresa el inventario y registra la reversa en la sede
// del documento ORIGINAL, no en la del header de quien anula.
func TestAnular_ReingresaEnLaSedeDelOriginalNoDelHeader(t *testing.T) {
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
	s1, s2 := stockEnSede(svc, sede1, sku), stockEnSede(svc, sede2, sku)

	// Se anula CON EL HEADER de la sede 2 (otro operador / otra sede activa).
	rev, err := svc.AnularDocumento(empDemo, sede2, actorA, origenTst, doc.ID, "prueba")
	if err != nil {
		t.Fatalf("anular: %v", err)
	}
	if rev.SedeID != sede1 {
		t.Errorf("la reversa debe registrarse en la sede del original (%s), quedó en %s", sede1, rev.SedeID)
	}
	if d := stockEnSede(svc, sede1, sku) - s1; !casi(d, 2) {
		t.Errorf("el stock debe reingresar en la sede original (+2), cambió %v", d)
	}
	if d := stockEnSede(svc, sede2, sku) - s2; !casi(d, 0) {
		t.Errorf("la sede del header NO debe recibir stock, cambió %v", d)
	}
}

// Contabilidad A1: RecontabilizarPendientes no debe re-asentar el movimiento de
// entrada de una compra (ya asentado en vivo por asentarCompra) en cada corrida.
func TestRecontabilizar_NoDuplicaElAsientoDeCompra(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	prov, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Suplidora Recontab, C.A.", Documento: "J-99999999-8",
	})
	if err != nil {
		t.Fatalf("crear proveedor: %v", err)
	}
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: prov.ID, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 20}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar OC: %v", err)
	}
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 10}}); err != nil {
		t.Fatalf("recibir OC: %v", err)
	}

	debe1201 := func() float64 {
		for _, c := range svc.Balance(empDemo).Cuentas {
			if c.Codigo == "1201" {
				return c.Debe
			}
		}
		return -1
	}
	// Primera corrida: backfillea el stock inicial del seed y asienta lo pendiente.
	svc.RecontabilizarPendientes(empDemo, actorA)
	antes := debe1201()
	// Segunda corrida (simula un reinicio): debe ser IDEMPOTENTE. Con el bug, la
	// entrada de la compra (RefTipo="compra", ya asentada en vivo) se re-asentaba como
	// Debe 1201 / Haber 3101 en cada corrida, inflando el inventario contable.
	svc.RecontabilizarPendientes(empDemo, actorA)
	if despues := debe1201(); !casi(despues, antes) {
		t.Errorf("recontabilizar no es idempotente: el Debe de Inventario (1201) cambió %v → %v", antes, despues)
	}
}

// Ventas A1/M2: la cotización NO reconvierte el precio que ya viene en bolívares
// (>=0 es autoritativo, 0 = línea gratis); solo convertiría con <0 (catálogo).
func TestCrearCotizacion_NoReconvierteElPrecioEnBs(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.GuardarConfigMoneda(empDemo, actorA, origenTst, application.ConfigMoneda{
		MonedaPrincipal: "USD", FuenteTasa: "manual", PreciosEnUsd: true,
	}); err != nil {
		t.Fatalf("config moneda: %v", err)
	}
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 40, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "USD-100", Nombre: "Producto en dólares", Precio: 100, Moneda: "USD",
	}); err != nil {
		t.Fatalf("crear producto USD: %v", err)
	}
	// El front manda el precio YA en bolívares (100 US$ × 40 = 4000 Bs) y una línea
	// gratis en 0. El servidor debe respetar ambos, no multiplicar de nuevo por 40.
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{
			{SKU: "USD-100", Cantidad: 1, PrecioUnitario: 4000},
			{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 0},
		},
	})
	if err != nil {
		t.Fatalf("crear cotización: %v", err)
	}
	porSKU := map[string]float64{}
	for _, l := range c.Lineas {
		porSKU[l.SKU] = l.PrecioUnitario
	}
	if !casi(porSKU["USD-100"], 4000) {
		t.Errorf("el precio en Bs no debe reconvertirse: esperaba 4000, quedó %v (¿×40 otra vez?)", porSKU["USD-100"])
	}
	if !casi(porSKU["REF-2L"], 0) {
		t.Errorf("una línea gratis (0) debe respetarse, quedó %v", porSKU["REF-2L"])
	}
}
