package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/almacen"
	"github.com/mornix/elerp/internal/domain/inventario"
)

func qtyAlmacen(t *testing.T, svc *application.Service, almacenID, sku string) float64 {
	t.Helper()
	ex, err := svc.ExistenciasDeAlmacen(empDemo, almacenID)
	if err != nil {
		t.Fatalf("existencias de almacén: %v", err)
	}
	for _, e := range ex {
		if e.SKU == sku {
			return e.Cantidad
		}
	}
	return 0
}

func qtySede(svc *application.Service, sedeID, sku string) float64 {
	for _, e := range svc.Existencias(empDemo, sedeID) {
		if e.SKU == sku {
			return e.Cantidad
		}
	}
	return 0
}

// Un ajuste en un almacén no afecta a otro; sin almacén va al principal; la
// existencia por sede es la suma de sus almacenes (retrocompat con lo del seed).
func TestInventarioPorAlmacen_AjusteAislado(t *testing.T) {
	svc := servicioAlmacenes(t)
	// El servicio de test no corre el backfill de arranque: aseguramos el principal
	// para que el "Depósito" sea un SEGUNDO almacén, no el primero (que sería principal).
	if _, err := svc.AsegurarAlmacenPrincipal(empDemo, sede1, "sistema", "sistema"); err != nil {
		t.Fatalf("asegurar principal: %v", err)
	}
	dep, err := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "Depósito"})
	if err != nil {
		t.Fatalf("crear depósito: %v", err)
	}
	prin, ok := svc.AlmacenPrincipalDe(empDemo, sede1)
	if !ok {
		t.Fatal("la sede debe tener almacén principal")
	}
	const sku = "REF-2L"
	p0 := qtyAlmacen(t, svc, prin.ID, sku) // incluye lo del seed (AlmacenID vacío → principal)
	d0 := qtyAlmacen(t, svc, dep.ID, sku)
	if d0 != 0 {
		t.Fatalf("el depósito nuevo debe arrancar en 0, tiene %v", d0)
	}
	sedeTotal0 := qtySede(svc, sede1, sku)

	// Ajuste al DEPÓSITO (almacén explícito): +7.
	if _, err := svc.Ajustar(empDemo, sede1, dep.ID, sku, "carga depósito", 7, actorA, origenTst); err != nil {
		t.Fatalf("ajustar depósito: %v", err)
	}
	if got := qtyAlmacen(t, svc, dep.ID, sku); !casi(got, 7) {
		t.Errorf("el depósito debe tener 7, tiene %v", got)
	}
	if got := qtyAlmacen(t, svc, prin.ID, sku); !casi(got, p0) {
		t.Errorf("el principal NO debe cambiar por un ajuste al depósito: %v → %v", p0, got)
	}
	if got := qtySede(svc, sede1, sku); !casi(got, sedeTotal0+7) {
		t.Errorf("la existencia por sede debe sumar los almacenes (+7): %v → %v", sedeTotal0, got)
	}

	// Ajuste SIN almacén: va al principal.
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "carga principal", 3, actorA, origenTst); err != nil {
		t.Fatalf("ajustar principal: %v", err)
	}
	if got := qtyAlmacen(t, svc, prin.ID, sku); !casi(got, p0+3) {
		t.Errorf("un ajuste sin almacén va al principal (+3): %v → %v", p0, got)
	}
	if got := qtyAlmacen(t, svc, dep.ID, sku); !casi(got, 7) {
		t.Errorf("el depósito no debe cambiar, tiene %v", got)
	}
}

// No se puede despachar una transferencia por encima de lo disponible en el origen
// (política de stock: el origen no queda en negativo).
func TestTransferencia_BloqueaDespachoSinStock(t *testing.T) {
	svc := servicioAlmacenes(t)
	tr, err := svc.CrearTransferencia(empDemo, actorA, origenTst, inventario.Transferencia{
		OrigenSedeID: sede1, DestinoSedeID: sede2,
		Lineas: []inventario.LineaTransferencia{{SKU: "REF-2L", Cantidad: 999999}},
	})
	if err != nil {
		t.Fatalf("crear transferencia: %v", err)
	}
	// El despacho debe fallar: no hay 999999 en el origen.
	if _, err := svc.CambiarEstadoTransferencia(empDemo, tr.ID, inventario.TransfDespachada, actorA, origenTst); !errors.Is(err, application.ErrStockInsuficiente) {
		t.Errorf("despachar sin stock debe dar ErrStockInsuficiente, se obtuvo %v", err)
	}
}

// Una transferencia entre dos almacenes de la MISMA sede mueve el stock del origen
// al destino sin cambiar el total de la sede.
func TestInventarioPorAlmacen_TransferenciaIntraSede(t *testing.T) {
	svc := servicioAlmacenes(t)
	if _, err := svc.AsegurarAlmacenPrincipal(empDemo, sede1, "sistema", "sistema"); err != nil {
		t.Fatalf("asegurar principal: %v", err)
	}
	dep, _ := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "Depósito"})
	prin, _ := svc.AlmacenPrincipalDe(empDemo, sede1)
	const sku = "REF-2L"
	// Asegurar stock en el principal.
	if _, err := svc.Ajustar(empDemo, sede1, prin.ID, sku, "stock", 20, actorA, origenTst); err != nil {
		t.Fatalf("ajuste inicial: %v", err)
	}
	p0 := qtyAlmacen(t, svc, prin.ID, sku)
	sedeTotal0 := qtySede(svc, sede1, sku)

	tr, err := svc.CrearTransferencia(empDemo, actorA, origenTst, inventario.Transferencia{
		OrigenSedeID: sede1, DestinoSedeID: sede1,
		OrigenAlmacenID: prin.ID, DestinoAlmacenID: dep.ID,
		Lineas: []inventario.LineaTransferencia{{SKU: sku, Cantidad: 6}},
	})
	if err != nil {
		t.Fatalf("crear transferencia intra-sede: %v", err)
	}
	// Avanzar la máquina de estados hasta recibida (emite salida en origen y entrada en destino).
	for _, st := range []string{inventario.TransfDespachada, inventario.TransfEnTransito, inventario.TransfRecibida} {
		if _, err := svc.CambiarEstadoTransferencia(empDemo, tr.ID, st, actorA, origenTst); err != nil {
			t.Fatalf("estado %s: %v", st, err)
		}
	}
	if got := qtyAlmacen(t, svc, prin.ID, sku); !casi(got, p0-6) {
		t.Errorf("el principal debe bajar 6: %v → %v", p0, got)
	}
	if got := qtyAlmacen(t, svc, dep.ID, sku); !casi(got, 6) {
		t.Errorf("el depósito debe subir a 6, tiene %v", got)
	}
	if got := qtySede(svc, sede1, sku); !casi(got, sedeTotal0) {
		t.Errorf("el total de la sede no debe cambiar por una transferencia interna: %v → %v", sedeTotal0, got)
	}
}
