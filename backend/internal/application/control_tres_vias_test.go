package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
)

/* CONTROL EN TRES VÍAS: pedido · recepción · factura del proveedor.
 *
 * La decisión de diseño que estos tests fijan: la factura SIEMPRE se registra —hay
 * obligación de llevarla al Libro de Compras—, así que el control no frena el
 * registro sino el PAGO. No pagar lo que no llegó es el objetivo; negarse a anotar
 * una factura que el proveedor ya emitió sería otro problema, y fiscal. */

// conPolitica configura el control de la empresa demo. El tenancy se construye
// aparte del servicio, igual que en el resto de la suite.
func conPolitica(t *testing.T, st *inmem.Store, politica string, tolMonto, tolPct float64) {
	t.Helper()
	tn := application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)
	if _, err := tn.ActualizarControlCompras(empDemo, actorA, origenTst, politica, tolMonto, tolPct); err != nil {
		t.Fatalf("configurar control de compras: %v", err)
	}
}

// facturaDeOrden localiza la factura de compra de una orden.
func facturaDeOrden(t *testing.T, svc *application.Service, ordenID string) compra.FacturaCompra {
	t.Helper()
	for _, f := range svc.FacturasCompra(empDemo) {
		if f.OrdenCompraID == ordenID {
			return f
		}
	}
	t.Fatalf("la orden %s no tiene factura de compra", ordenID)
	return compra.FacturaCompra{}
}

// facturaDesajustada recibe 10 u a Bs 20 (base 200) y factura solo `facturado`.
func facturaDesajustada(t *testing.T, svc *application.Service, sku string, cantFacturada float64, num string) (ordenID string) {
	t.Helper()
	oc := ocRecibida(t, svc, sku, 10, 20)
	if _, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: num, NumeroControl: "00-" + num,
		Fecha:  time.Now().UTC().Format(time.RFC3339Nano),
		Lineas: []application.LineaFacturaCompraEntrada{{SKU: sku, Cantidad: cantFacturada, CostoUnitario: 20}},
	}); err != nil {
		t.Fatalf("registrar factura: %v", err)
	}
	return oc.ID
}

// TestControl_LaFacturaSiempreSeRegistra: ni siquiera con política de bloqueo se
// impide anotarla. Lo que queda es su veredicto.
func TestControl_LaFacturaSiempreSeRegistra(t *testing.T) {
	svc, st := nuevoServicio(t)
	sku := primerSKU(t, svc)
	conPolitica(t, st, "bloquear", 0, 0)

	ordenID := facturaDesajustada(t, svc, sku, 4, "F-CTRL-1") // facturado 80 contra recibido 200
	// facturaDeOrden falla el test si no existe: registrarla es obligación fiscal.
	fc := facturaDeOrden(t, svc, ordenID)
	if !fc.ControlEvaluado {
		t.Error("la factura debe quedar evaluada por el control")
	}
	if fc.ControlConforme {
		t.Error("facturar 80 contra 200 recibidos no es conforme")
	}
	casiEq(t, fc.DiferenciaBase, -120, "la diferencia se guarda tal cual")
}

// TestControl_LaToleranciaDecide: dentro de tolerancia es conforme, fuera no.
func TestControl_LaToleranciaDecide(t *testing.T) {
	svc, st := nuevoServicio(t)
	sku := primerSKU(t, svc)
	// Tolerancia de Bs 25: una diferencia de 20 cabe, una de 40 no.
	conPolitica(t, st, "avisar", 25, 0)

	dentro := facturaDesajustada(t, svc, sku, 9, "F-CTRL-2") // 180 vs 200 ⇒ −20
	fc := facturaDeOrden(t, svc, dentro)
	if !fc.ControlConforme {
		t.Errorf("una diferencia de 20 cabe en una tolerancia de 25")
	}
	casiEq(t, fc.ControlTolerancia, 25, "la tolerancia aplicada se graba en el documento")

	fuera := facturaDesajustada(t, svc, sku, 8, "F-CTRL-3") // 160 vs 200 ⇒ −40
	fc2 := facturaDeOrden(t, svc, fuera)
	if fc2.ControlConforme {
		t.Error("una diferencia de 40 no cabe en una tolerancia de 25")
	}
}

// TestControl_ToleranciaPorPorcentaje: el porcentaje escala con el importe, y manda
// el mayor de los dos topes.
func TestControl_ToleranciaPorPorcentaje(t *testing.T) {
	svc, st := nuevoServicio(t)
	sku := primerSKU(t, svc)
	// 25% de 200 = 50, que gana al tope en monto de 10.
	conPolitica(t, st, "avisar", 10, 25)

	ordenID := facturaDesajustada(t, svc, sku, 8, "F-CTRL-4") // −40
	fc := facturaDeOrden(t, svc, ordenID)
	casiEq(t, fc.ControlTolerancia, 50, "manda el mayor de los dos topes")
	if !fc.ControlConforme {
		t.Error("40 cabe en una tolerancia de 50")
	}
}

// TestControl_BloquearFrenaElPagoYPideMotivo es el corazón de la funcionalidad.
func TestControl_BloquearFrenaElPagoYPideMotivo(t *testing.T) {
	svc, st := nuevoServicio(t)
	sku := primerSKU(t, svc)
	conPolitica(t, st, "bloquear", 0, 0)
	ordenID := facturaDesajustada(t, svc, sku, 4, "F-CTRL-5")

	// Sin motivo: frenado.
	_, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: provDemo1, OrdenCompraID: ordenID, MontoBs: 10, Metodo: "transferencia",
	})
	if !errors.Is(err, application.ErrPagoBloqueadoPorControl) {
		t.Fatalf("pagar una factura en excepción debía frenarse, se obtuvo: %v", err)
	}

	// Con motivo declarado: pasa, y el motivo queda guardado en el pago.
	pago, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: provDemo1, OrdenCompraID: ordenID, MontoBs: 10, Metodo: "transferencia",
		ExcepcionMotivo: "Faltante acordado con el proveedor, envía el resto la semana próxima",
	})
	if err != nil {
		t.Fatalf("con motivo declarado el pago procede: %v", err)
	}
	if pago.ExcepcionMotivo == "" {
		t.Error("el motivo tiene que quedar guardado en el pago: es lo que se audita")
	}
}

// TestControl_PagarACuentaNoEsquivaElBloqueo: si solo se mirara la orden imputada,
// bastaría pagar «a cuenta» para saltarse el control.
func TestControl_PagarACuentaNoEsquivaElBloqueo(t *testing.T) {
	svc, st := nuevoServicio(t)
	sku := primerSKU(t, svc)
	conPolitica(t, st, "bloquear", 0, 0)
	facturaDesajustada(t, svc, sku, 4, "F-CTRL-6")

	_, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: provDemo1, MontoBs: 10, Metodo: "transferencia", // sin orden: a cuenta
	})
	if !errors.Is(err, application.ErrPagoBloqueadoPorControl) {
		t.Fatalf("un pago a cuenta con facturas en excepción debía frenarse, se obtuvo: %v", err)
	}
}

// TestControl_AvisarNoFrenaNada: la política por defecto marca la excepción para
// que se vea, pero no impide operar.
func TestControl_AvisarNoFrenaNada(t *testing.T) {
	svc, st := nuevoServicio(t)
	sku := primerSKU(t, svc)
	conPolitica(t, st, "avisar", 0, 0)
	ordenID := facturaDesajustada(t, svc, sku, 4, "F-CTRL-7")

	fc := facturaDeOrden(t, svc, ordenID)
	if fc.ControlConforme {
		t.Error("la excepción igual se marca en avisar")
	}
	if _, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: provDemo1, OrdenCompraID: ordenID, MontoBs: 10, Metodo: "transferencia",
	}); err != nil {
		t.Fatalf("en política avisar el pago no se frena: %v", err)
	}
}

// TestControl_UnaFacturaConformeNoEstorba: el control no puede volverse un peaje
// para la operación normal.
func TestControl_UnaFacturaConformeNoEstorba(t *testing.T) {
	svc, st := nuevoServicio(t)
	sku := primerSKU(t, svc)
	conPolitica(t, st, "bloquear", 0, 0)

	// Factura que coincide exactamente con lo recibido.
	oc := ocRecibida(t, svc, sku, 10, 20)
	if _, err := svc.RegistrarFacturaCompra(empDemo, actorA, origenTst, oc.ID, application.EntradaFacturaCompra{
		NumeroFactura: "F-CTRL-8", NumeroControl: "00-F-CTRL-8",
		Fecha: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("registrar factura: %v", err)
	}
	fc := facturaDeOrden(t, svc, oc.ID)
	if !fc.ControlConforme {
		t.Fatal("una factura igual a lo recibido es conforme")
	}
	if _, err := svc.RegistrarPagoProveedor(empDemo, actorA, origenTst, application.EntradaPagoProveedor{
		ProveedorID: provDemo1, OrdenCompraID: oc.ID, MontoBs: 10, Metodo: "transferencia",
	}); err != nil {
		t.Fatalf("una factura conforme se paga sin fricción: %v", err)
	}
}

// TestControl_PoliticaYToleranciaSeValidan.
func TestControl_PoliticaYToleranciaSeValidan(t *testing.T) {
	svc, st := nuevoServicio(t)
	_ = svc
	tn := application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)
	if _, err := tn.ActualizarControlCompras(empDemo, actorA, origenTst, "frenar", 0, 0); !errors.Is(err, application.ErrPoliticaControlInvalida) {
		t.Fatalf("una política desconocida debía rechazarse, se obtuvo: %v", err)
	}
	if _, err := tn.ActualizarControlCompras(empDemo, actorA, origenTst, "avisar", -1, 0); !errors.Is(err, application.ErrToleranciaControlInvalida) {
		t.Fatalf("una tolerancia negativa debía rechazarse, se obtuvo: %v", err)
	}
	if _, err := tn.ActualizarControlCompras(empDemo, actorA, origenTst, "avisar", 0, 150); !errors.Is(err, application.ErrToleranciaControlInvalida) {
		t.Fatalf("un porcentaje mayor a 100 debía rechazarse, se obtuvo: %v", err)
	}
}
