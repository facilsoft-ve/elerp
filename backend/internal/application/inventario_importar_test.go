package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
)

// servicioImport arma el servicio del seed con el maestro de unidades cableado
// (la carga masiva valida la unidad contra el maestro).
func servicioImport(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConUnidades(st.Unidades)
	return svc
}

func existenciaDeSKU(svc *application.Service, sku string) (float64, bool) {
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			return e.Cantidad, true
		}
	}
	return 0, false
}

func precioDeSKU(svc *application.Service, sku string) (float64, bool) {
	for _, p := range svc.Productos(empDemo) {
		if p.SKU == sku {
			return p.Precio, true
		}
	}
	return 0, false
}

// TestImportarProductos_PreviewClasificaFilas comprueba la vista previa: clasifica
// nuevo/actualiza/error sin escribir, exige confirmación si hay SKUs existentes y
// rechaza una unidad que no está en el maestro.
func TestImportarProductos_PreviewClasificaFilas(t *testing.T) {
	svc := servicioImport(t)
	filas := []application.FilaImportacionProducto{
		{SKU: "IMP-001", Nombre: "Producto Nuevo", Unidad: "unidad", Precio: 10},
		{SKU: "REF-2L", Nombre: "Refresco (update)", Unidad: "unidad", Precio: 20}, // ya existe
		{SKU: "IMP-003", Nombre: "Unidad rara", Unidad: "xyz", Precio: 5},          // unidad fuera del maestro
		{SKU: "", Nombre: "Sin SKU"}, // error: SKU vacío
	}
	res, err := svc.ImportarProductos(empDemo, sede1, actorA, origenTst, filas, false)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if res.Nuevos != 1 || res.Actualizados != 1 || res.Errores != 2 {
		t.Fatalf("clasificación esperada nuevos=1 actualiza=1 errores=2, se obtuvo %+v", res)
	}
	if !res.RequiereConfirmacion {
		t.Error("con un SKU existente el preview debe requerir confirmación")
	}
	if res.Aplicado {
		t.Error("un dry run (confirmar=false) no debe aplicar cambios")
	}
	if _, ok := precioDeSKU(svc, "IMP-001"); ok {
		t.Error("el preview no debía crear IMP-001")
	}
	// La fila de unidad desconocida debe explicar el porqué.
	if res.Filas[2].Estado != application.FilaImportError || res.Filas[2].Mensaje == "" {
		t.Errorf("la fila con unidad 'xyz' debía ser error con mensaje, se obtuvo %+v", res.Filas[2])
	}
}

// TestImportarProductos_ConErroresNoAplicaNada comprueba el todo-o-nada: si alguna
// fila falla, ni las válidas se escriben.
func TestImportarProductos_ConErroresNoAplicaNada(t *testing.T) {
	svc := servicioImport(t)
	filas := []application.FilaImportacionProducto{
		{SKU: "IMP-020", Nombre: "Válido", Unidad: "unidad", Precio: 10},
		{SKU: "IMP-021", Nombre: "Unidad mala", Unidad: "nope", Precio: 10},
	}
	res, err := svc.ImportarProductos(empDemo, sede1, actorA, origenTst, filas, true)
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if res.Aplicado {
		t.Error("con una fila en error no debe aplicar ninguna (todo o nada)")
	}
	if _, ok := precioDeSKU(svc, "IMP-020"); ok {
		t.Error("la fila válida no debía crearse porque otra falló")
	}
}

// TestImportarProductos_AplicaUpsertYExistenciaInicial comprueba el UPSERT y la
// carga de existencia inicial como ajuste auditado en el producto nuevo.
func TestImportarProductos_AplicaUpsertYExistenciaInicial(t *testing.T) {
	svc := servicioImport(t)
	filas := []application.FilaImportacionProducto{
		{SKU: "IMP-010", Nombre: "Nuevo con stock", Unidad: "unidad", Precio: 50, ExistenciaInicial: 7},
		{SKU: "REF-2L", Nombre: "Refresco actualizado", Unidad: "unidad", Precio: 1600},
	}
	res, err := svc.ImportarProductos(empDemo, sede1, actorA, origenTst, filas, true)
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if !res.Aplicado || res.Nuevos != 1 || res.Actualizados != 1 {
		t.Fatalf("esperado aplicado nuevos=1 actualiza=1, se obtuvo %+v", res)
	}
	// El nuevo se creó con su precio.
	if pr, ok := precioDeSKU(svc, "IMP-010"); !ok || pr != 50 {
		t.Errorf("IMP-010 debía crearse con precio 50, se obtuvo (%v, %v)", pr, ok)
	}
	// La existencia inicial entró como movimiento (ajuste auditado).
	if cant, ok := existenciaDeSKU(svc, "IMP-010"); !ok || cant != 7 {
		t.Errorf("IMP-010 debía quedar con existencia 7, se obtuvo (%v, %v)", cant, ok)
	}
	// El existente se actualizó (precio nuevo).
	if pr, ok := precioDeSKU(svc, "REF-2L"); !ok || pr != 1600 {
		t.Errorf("REF-2L debía actualizarse a 1600, se obtuvo (%v, %v)", pr, ok)
	}
}
