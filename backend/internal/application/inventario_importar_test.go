package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/inventario"
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

/* LA COLUMNA `alicuotaCodigo` DE LA CARGA MASIVA.
 *
 * Con sola la columna `exentoIva` el archivo solo sabía decir «exento» o «lo
 * demás»: un catálogo con bienes suntuarios (16 % + 15 %) o con tasa reducida
 * entraba entero clasificado como general y nadie se enteraba hasta facturar.
 *
 * Las tres pruebas de acá cubren el contrato completo: la columna clasifica, un
 * código inventado es error de FILA (en la vista previa, antes de escribir nada),
 * y —lo que sostiene la compatibilidad— si la columna no viene, nada cambia. */

// servicioImportConAlicuotas es el servicio de la carga masiva con el maestro de
// impuestos cableado (sin él, validarAlicuotaProducto deja pasar cualquier cosa).
func servicioImportConAlicuotas(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConUnidades(st.Unidades)
	svc.ConAlicuotas(st.Alicuotas)
	return svc
}

func productoDeSKU(t *testing.T, svc *application.Service, sku string) inventario.Producto {
	t.Helper()
	for _, p := range svc.Productos(empDemo) {
		if p.SKU == sku {
			return p
		}
	}
	t.Fatalf("no se encontró el producto %q", sku)
	return inventario.Producto{}
}

// TestImportarProductos_AlicuotaClasifica comprueba que la columna entra tanto en
// el alta como en la actualización, y que el booleano queda en sincronía con ella.
func TestImportarProductos_AlicuotaClasifica(t *testing.T) {
	svc := servicioImportConAlicuotas(t)
	filas := []application.FilaImportacionProducto{
		{SKU: "IMP-LUJO", Nombre: "Whisky importado", Unidad: "unidad", Precio: 900, AlicuotaCodigo: "suntuario"},
		{SKU: "IMP-PAN", Nombre: "Harina", Unidad: "unidad", Precio: 30, AlicuotaCodigo: "exento"},
		// La columna manda sobre el booleano: exentoIva=true + código general
		// tiene que quedar GRAVADO, que es lo que el motor cobrará.
		{SKU: "IMP-MIX", Nombre: "Contradictorio", Unidad: "unidad", Precio: 10, ExentoIVA: true, AlicuotaCodigo: "general"},
		{SKU: "REF-2L", Nombre: "Refresco reclasificado", Unidad: "unidad", Precio: 1600, AlicuotaCodigo: "reducida"},
	}
	res, err := svc.ImportarProductos(empDemo, sede1, actorA, origenTst, filas, true)
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if !res.Aplicado || res.Errores != 0 {
		t.Fatalf("esperado aplicado sin errores, se obtuvo %+v", res)
	}
	if p := productoDeSKU(t, svc, "IMP-LUJO"); p.AlicuotaCodigo != "suntuario" || p.ExentoIVA {
		t.Errorf("IMP-LUJO debía quedar suntuario y gravado, se obtuvo (%q, exento=%v)", p.AlicuotaCodigo, p.ExentoIVA)
	}
	// El booleano se mantiene en sincronía: media aplicación todavía lo lee.
	if p := productoDeSKU(t, svc, "IMP-PAN"); p.AlicuotaCodigo != "exento" || !p.ExentoIVA {
		t.Errorf("IMP-PAN debía quedar exento en código y booleano, se obtuvo (%q, exento=%v)", p.AlicuotaCodigo, p.ExentoIVA)
	}
	if p := productoDeSKU(t, svc, "IMP-MIX"); p.AlicuotaCodigo != "general" || p.ExentoIVA {
		t.Errorf("IMP-MIX: manda la columna, debía quedar general y gravado, se obtuvo (%q, exento=%v)", p.AlicuotaCodigo, p.ExentoIVA)
	}
	if p := productoDeSKU(t, svc, "REF-2L"); p.AlicuotaCodigo != "reducida" {
		t.Errorf("REF-2L debía reclasificarse a reducida, se obtuvo %q", p.AlicuotaCodigo)
	}
}

// TestImportarProductos_AlicuotaDesconocidaEsErrorDeFila: un código que el maestro
// no conoce se rechaza en la VISTA PREVIA. Dejarlo pasar crearía un producto
// facturando a la tasa de respaldo sin que nadie lo sepa.
func TestImportarProductos_AlicuotaDesconocidaEsErrorDeFila(t *testing.T) {
	svc := servicioImportConAlicuotas(t)
	filas := []application.FilaImportacionProducto{
		{SKU: "IMP-OK", Nombre: "Válido", Unidad: "unidad", Precio: 10, AlicuotaCodigo: "general"},
		{SKU: "IMP-RARA", Nombre: "Alícuota inventada", Unidad: "unidad", Precio: 10, AlicuotaCodigo: "lujoso"},
	}
	res, err := svc.ImportarProductos(empDemo, sede1, actorA, origenTst, filas, false)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if res.Errores != 1 || res.Nuevos != 1 {
		t.Fatalf("esperado 1 error y 1 nuevo, se obtuvo %+v", res)
	}
	if res.Filas[1].Estado != application.FilaImportError || res.Filas[1].Mensaje == "" {
		t.Errorf("la fila con alícuota inventada debía ser error con mensaje, se obtuvo %+v", res.Filas[1])
	}
	// Y al aplicar rige el todo-o-nada: no se crea ni la buena.
	if _, err := svc.ImportarProductos(empDemo, sede1, actorA, origenTst, filas, true); err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if _, ok := precioDeSKU(svc, "IMP-OK"); ok {
		t.Error("con una fila en error no debía crearse IMP-OK")
	}
}

// TestImportarProductos_SinAlicuotaConservaElComportamiento es LA prueba de
// compatibilidad: un archivo sin la columna (el de siempre) clasifica igual que
// antes por `exentoIva`, y a un producto que YA estaba clasificado a mano no le
// toca la clasificación.
func TestImportarProductos_SinAlicuotaConservaElComportamiento(t *testing.T) {
	svc := servicioImportConAlicuotas(t)
	// Se clasifica un producto existente a mano, como lo haría la ficha.
	suntuario := "suntuario"
	if _, err := svc.ActualizarProducto(empDemo, actorA, origenTst, "REF-2L",
		application.CambiosProducto{Nombre: "Refresco", Precio: 1500, AlicuotaCodigo: &suntuario}); err != nil {
		t.Fatalf("clasificar REF-2L: %v", err)
	}

	filas := []application.FilaImportacionProducto{
		{SKU: "IMP-VIEJO", Nombre: "Sin columna", Unidad: "unidad", Precio: 10},
		{SKU: "IMP-EXENTO", Nombre: "Sin columna, exento", Unidad: "unidad", Precio: 10, ExentoIVA: true},
		{SKU: "REF-2L", Nombre: "Refresco reimportado", Unidad: "unidad", Precio: 1700},
	}
	res, err := svc.ImportarProductos(empDemo, sede1, actorA, origenTst, filas, true)
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if !res.Aplicado || res.Errores != 0 {
		t.Fatalf("esperado aplicado sin errores, se obtuvo %+v", res)
	}
	// Los nuevos entran como antes: sin código, con el booleano del archivo.
	if p := productoDeSKU(t, svc, "IMP-VIEJO"); p.AlicuotaCodigo != "" || p.ExentoIVA {
		t.Errorf("IMP-VIEJO debía quedar sin clasificar y gravado, se obtuvo (%q, exento=%v)", p.AlicuotaCodigo, p.ExentoIVA)
	}
	if p := productoDeSKU(t, svc, "IMP-EXENTO"); p.AlicuotaCodigo != "" || !p.ExentoIVA {
		t.Errorf("IMP-EXENTO debía quedar sin clasificar y exento, se obtuvo (%q, exento=%v)", p.AlicuotaCodigo, p.ExentoIVA)
	}
	// Y el que ya estaba clasificado NO se degrada: sin columna, no se toca.
	if p := productoDeSKU(t, svc, "REF-2L"); p.AlicuotaCodigo != "suntuario" || p.Precio != 1700 {
		t.Errorf("REF-2L debía conservar 'suntuario' y actualizar el precio, se obtuvo (%q, %v)", p.AlicuotaCodigo, p.Precio)
	}
}
