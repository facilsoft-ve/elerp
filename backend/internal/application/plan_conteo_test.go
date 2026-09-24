package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* PLANES DE CONTEO.
 *
 * Lo que defienden estas pruebas: que el plan diga la verdad sobre cuándo se contó.
 * Un plan que se marca como hecho sin que nadie haya contado deja el almacén sin
 * revisar Y sin aviso — peor que no tener plan, porque además da confianza. */

// servicioConPlanes cablea lo que los planes necesitan.
func servicioConPlanes(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConUbicaciones(st.Ubicaciones)
	svc.ConPlanesDeConteo(st.PlanesConteo)
	return svc
}

// planDe da de alta un plan de conteo.
func planDe(t *testing.T, svc *application.Service, nombre string, cadaDias int, almacenID, ubicacionID, rubro string) inventario.PlanConteo {
	t.Helper()
	p, err := svc.CrearPlanDeConteo(empDemo, actorA, origenTst, inventario.PlanConteo{
		SedeID: sede1, Nombre: nombre, CadaDias: cadaDias,
		AlmacenID: almacenID, UbicacionID: ubicacionID, Rubro: rubro,
	})
	if err != nil {
		t.Fatalf("crear plan %s: %v", nombre, err)
	}
	return p
}

// vistaDe busca un plan en la lista.
func vistaDe(t *testing.T, svc *application.Service, id string) application.PlanConteoView {
	t.Helper()
	for _, v := range svc.PlanesDeConteo(empDemo, sede1) {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("el plan %s no está en la lista", id)
	return application.PlanConteoView{}
}

// TestPlanConteo_UnPlanNuevoTocaYa: un almacén que nadie ha revisado nunca es
// precisamente el caso urgente. Darle un mes de gracia al plan recién creado
// retrasaría justo el conteo que más falta hace.
func TestPlanConteo_UnPlanNuevoTocaYa(t *testing.T) {
	svc := servicioConPlanes(t)
	p := planDe(t, svc, "Almacén principal, mensual", 30, "", "", "")

	v := vistaDe(t, svc, p.ID)
	if !v.Vencido {
		t.Error("un plan que nunca se contó toca ya")
	}
	if v.DiasDesde != -1 {
		t.Errorf("nunca contado se representa con -1: %d", v.DiasDesde)
	}
	if v.Productos == 0 {
		t.Error("el plan tenía que alcanzar los productos con existencia de la sede")
	}
}

// TestPlanConteo_SinPeriodicidadNoVenceNunca: un plan «a demanda» existe para tener
// la hoja a mano. Marcarlo como pendiente lo dejaría siempre en rojo y enseñaría a
// ignorar la lista entera — que es lo que mata a los avisos.
func TestPlanConteo_SinPeriodicidadNoVenceNunca(t *testing.T) {
	svc := servicioConPlanes(t)
	p := planDe(t, svc, "Conteo suelto", 0, "", "", "")
	if vistaDe(t, svc, p.ID).Vencido {
		t.Error("sin periodicidad no vence: existe para tener la hoja preparada")
	}
}

// TestPlanConteo_LaHojaTraeLoQueDeberiaEstar: la hoja lleva lo que el sistema cree
// que hay en ese ámbito, no el catálogo entero. Una hoja de ochocientos renglones
// no se usa.
func TestPlanConteo_LaHojaTraeLoQueDeberiaEstar(t *testing.T) {
	svc := servicioConPlanes(t)
	alm := almacenPrincipalID(t, svc)
	pasillo := nuevaUbicacion(t, svc, alm, "B-02")
	sku := primerSKU(t, svc)

	// Solo este producto está en B-02.
	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, pasillo, sku, "carga", 12, "", "", actorA, origenTst); err != nil {
		t.Fatalf("cargar: %v", err)
	}
	p := planDe(t, svc, "Pasillo B, semanal", 7, alm, pasillo, "")

	hoja, err := svc.HojaDeConteoDe(empDemo, p.ID)
	if err != nil {
		t.Fatalf("hoja: %v", err)
	}
	if hoja.Ubicacion != "B-02" {
		t.Errorf("la hoja tenía que decir qué casilla se cuenta: %q", hoja.Ubicacion)
	}
	if len(hoja.Lineas) != 1 || hoja.Lineas[0].SKU != sku {
		t.Fatalf("en B-02 solo está ese producto: %+v", hoja.Lineas)
	}
	if !casi(hoja.Lineas[0].Sistema, 12) {
		t.Errorf("la hoja trae lo que el sistema cree que hay: %v", hoja.Lineas[0].Sistema)
	}
}

// TestPlanConteo_ContarSellaElPlan es la prueba central: aplicar el conteo pone la
// fecha, y a partir de ahí el plan deja de estar vencido.
func TestPlanConteo_ContarSellaElPlan(t *testing.T) {
	svc := servicioConPlanes(t)
	alm := almacenPrincipalID(t, svc)
	pasillo := nuevaUbicacion(t, svc, alm, "A-01")
	sku := primerSKU(t, svc)
	svc.AjustarEnUbicacion(empDemo, sede1, alm, pasillo, sku, "carga", 20, "", "", actorA, origenTst)

	p := planDe(t, svc, "Pasillo A, mensual", 30, alm, pasillo, "")
	if !vistaDe(t, svc, p.ID).Vencido {
		t.Fatal("de partida toca")
	}

	// Se cuenta y hay 18, no 20. Ojo: la línea NO declara ubicación — la hereda del
	// plan, que es lo que evita repetir «pasillo A» en cada renglón.
	res, err := svc.AplicarConteoDePlan(empDemo, p.ID, "", []application.LineaConteo{{SKU: sku, Contado: 18}}, actorA, origenTst)
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if res.ConAjuste != 1 {
		t.Fatalf("había una diferencia de 2: %+v", res)
	}
	if got := saldoEn(t, svc, sku, pasillo); !casi(got, 18) {
		t.Errorf("el ajuste tenía que caer en A-01: %v", got)
	}

	v := vistaDe(t, svc, p.ID)
	if v.Vencido {
		t.Error("recién contado no vence")
	}
	if v.UltimoConteo != time.Now().UTC().Format("2006-01-02") {
		t.Errorf("tenía que sellarse con la fecha de hoy: %q", v.UltimoConteo)
	}
	if v.DiasDesde != 0 {
		t.Errorf("contado hoy son 0 días: %d", v.DiasDesde)
	}
}

// TestPlanConteo_UnaHojaQueFallaNoSellaNada es la guarda que da sentido al sello.
//
// Marcar el plan como contado cuando la hoja se cayó entera dejaría el almacén sin
// revisar y sin aviso hasta el siguiente vencimiento: un mes creyendo que está
// cuadrado.
func TestPlanConteo_UnaHojaQueFallaNoSellaNada(t *testing.T) {
	svc := servicioConPlanes(t)
	p := planDe(t, svc, "Mensual", 30, "", "", "")

	if _, err := svc.AplicarConteoDePlan(empDemo, p.ID, "", []application.LineaConteo{
		{SKU: "NO-EXISTE", Contado: 5},
	}, actorA, origenTst); err == nil {
		t.Fatal("un SKU inventado tenía que abortar la hoja")
	}
	v := vistaDe(t, svc, p.ID)
	if v.UltimoConteo != "" {
		t.Errorf("la hoja falló: el plan no puede quedar sellado (%q)", v.UltimoConteo)
	}
	if !v.Vencido {
		t.Error("y sigue tocando")
	}
}

// TestPlanConteo_ElMaestroSeDefiende: las guardas del alta.
func TestPlanConteo_ElMaestroSeDefiende(t *testing.T) {
	svc := servicioConPlanes(t)

	if _, err := svc.CrearPlanDeConteo(empDemo, actorA, origenTst, inventario.PlanConteo{
		SedeID: sede1, Nombre: "  ", CadaDias: 30,
	}); !errors.Is(err, application.ErrPlanSinNombre) {
		t.Errorf("sin nombre debía rechazarse: %v", err)
	}
	if _, err := svc.CrearPlanDeConteo(empDemo, actorA, origenTst, inventario.PlanConteo{
		SedeID: sede1, Nombre: "Raro", CadaDias: -5,
	}); !errors.Is(err, application.ErrPlanPeriodicidad) {
		t.Errorf("una periodicidad negativa debía rechazarse: %v", err)
	}
	if _, err := svc.HojaDeConteoDe(empDemo, "pcn_inventado"); !errors.Is(err, application.ErrPlanNoExiste) {
		t.Errorf("un plan inventado debía rechazarse: %v", err)
	}
}

// TestPlanConteo_SinCablearNoPasaNada: la regresión.
func TestPlanConteo_SinCablearNoPasaNada(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes) // ...pero SIN ConPlanesDeConteo
	if n := len(svc.PlanesDeConteo(empDemo, sede1)); n != 0 {
		t.Errorf("sin repositorio no hay planes: %d", n)
	}
	if _, err := svc.HojaDeConteoDe(empDemo, "x"); err == nil {
		t.Error("sin planes no hay hoja que armar")
	}
}
