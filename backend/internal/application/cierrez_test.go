package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// emitirContado emite una factura de contado en la sede indicada y la devuelve.
func emitirContado(t *testing.T, svc *application.Service, sede string, precio float64) fiscal.Documento {
	t.Helper()
	doc, err := svc.EmitirFactura(empDemo, sede, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		SinCaja: true, // canal forma libre: no exige caja abierta
		Lineas:  []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: precio}},
		Pagos:   []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: precio * 2, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir contado: %v", err)
	}
	return doc
}

func TestCierreZ_SinDocumentosNoSeEmite(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// Sede sin ninguna venta propia: nada que cerrar.
	if _, err := svc.EmitirCierreZ(empDemo, sede2, actorA, origenTst); !errors.Is(err, application.ErrNadaQueCerrar) {
		t.Fatalf("se esperaba ErrNadaQueCerrar, se obtuvo: %v", err)
	}
	_, cant, err := svc.PreviewCierreZ(empDemo, sede2)
	if err != nil {
		t.Fatalf("preview no debe fallar: %v", err)
	}
	if cant != 0 {
		t.Errorf("una sede sin ventas nuevas debe previsualizar 0 documentos, dio %d", cant)
	}
}

func TestCierreZ_ConsolidaFoldeaYNumera(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// El seed ya trae documentos en sede1; se agregan dos ventas nuevas de 100.
	emitirContado(t, svc, sede1, 100)
	emitirContado(t, svc, sede1, 100)

	prevTot, prevCant, err := svc.PreviewCierreZ(empDemo, sede1)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if prevCant == 0 {
		t.Fatal("con ventas nuevas el preview no puede dar 0 documentos")
	}

	z, err := svc.EmitirCierreZ(empDemo, sede1, actorA, origenTst)
	if err != nil {
		t.Fatalf("emitir Z: %v", err)
	}
	if z.NumeroCompleto != "Z-00000001" {
		t.Errorf("el primer Z de la sede debe ser Z-00000001, dio %q", z.NumeroCompleto)
	}
	// El cierre consolida exactamente lo que el preview anticipó.
	if !casi(z.Totales.TotalNeto, prevTot.TotalNeto) {
		t.Errorf("el Z emitido (%v) debe cuadrar con el preview (%v)", z.Totales.TotalNeto, prevTot.TotalNeto)
	}
	// Las dos ventas de 100 gravadas aportan al menos 200 de base imponible.
	if z.Totales.VentasGravadas < 200-0.005 {
		t.Errorf("ventas gravadas esperadas ≥ 200, dio %v", z.Totales.VentasGravadas)
	}
	if z.Totales.CantidadFacturas < 2 {
		t.Errorf("el Z debe contar al menos las 2 facturas nuevas, contó %d", z.Totales.CantidadFacturas)
	}
	if z.DocDesde == "" || z.DocHasta == "" || z.DesdeFecha == "" || z.HastaFecha == "" {
		t.Errorf("el Z debe registrar el rango de folios y fechas: %+v", z)
	}
}

func TestCierreZ_SegundoZSoloTomaLoNuevo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	emitirContado(t, svc, sede1, 100)
	primero, err := svc.EmitirCierreZ(empDemo, sede1, actorA, origenTst)
	if err != nil {
		t.Fatalf("primer Z: %v", err)
	}
	// Cerrar de nuevo sin ventas nuevas no debe emitir nada.
	if _, err := svc.EmitirCierreZ(empDemo, sede1, actorA, origenTst); !errors.Is(err, application.ErrNadaQueCerrar) {
		t.Fatalf("un segundo Z sin ventas nuevas debe fallar, se obtuvo: %v", err)
	}
	// Una venta nueva y un segundo Z: numera Z-00000002 y solo consolida lo nuevo.
	nueva := emitirContado(t, svc, sede1, 50)
	segundo, err := svc.EmitirCierreZ(empDemo, sede1, actorA, origenTst)
	if err != nil {
		t.Fatalf("segundo Z: %v", err)
	}
	if segundo.Numero != primero.Numero+1 {
		t.Errorf("la secuencia Z debe avanzar de %d a %d, dio %d", primero.Numero, primero.Numero+1, segundo.Numero)
	}
	// El segundo Z arranca DESPUÉS del primero: no re-consolida lo ya cerrado.
	if segundo.DesdeFecha <= primero.HastaFecha {
		t.Errorf("el segundo Z debe arrancar tras el corte del primero (%s), arrancó en %s", primero.HastaFecha, segundo.DesdeFecha)
	}
	if segundo.Totales.CantidadFacturas != 1 || segundo.DocHasta != nueva.NumeroCompleto {
		t.Errorf("el segundo Z solo debe tomar la venta nueva %s, dio %+v", nueva.NumeroCompleto, segundo.Totales)
	}
}

func TestCierreZ_NoCruzaSedesNiTenants(t *testing.T) {
	svc, _ := nuevoServicio(t)
	emitirContado(t, svc, sede1, 100)
	if _, err := svc.EmitirCierreZ(empDemo, sede1, actorA, origenTst); err != nil {
		t.Fatalf("Z sede1: %v", err)
	}
	// La sede2 no vio esas ventas: su preview sigue vacío.
	if _, cant, _ := svc.PreviewCierreZ(empDemo, sede2); cant != 0 {
		t.Errorf("la sede2 no debe ver los documentos de sede1, vio %d", cant)
	}
	// Los Z de sede1 no aparecen para sede2 ni para otro tenant.
	if len(svc.CierresZ(empDemo, sede2)) != 0 {
		t.Error("la sede2 no debe listar cierres Z de sede1")
	}
	if len(svc.CierresZ("emp_ajena", sede1)) != 0 {
		t.Error("otro tenant no debe ver los cierres Z de esta empresa")
	}
}
