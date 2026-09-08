package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/unidadmedida"
)

// servicioConUnidades arma el servicio del seed demo con el maestro de unidades
// cableado (nuevoServicio no lo cablea, igual que cmd/api lo hace aparte).
func servicioConUnidades(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConUnidades(st.Unidades)
	return svc, st
}

func TestUnidadesPorDefecto_Sembradas(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	us := svc.Unidades(empDemo)
	if len(us) != 10 {
		t.Fatalf("la empresa demo debía traer 10 unidades por defecto, trajo %d", len(us))
	}
	presentes := map[string]bool{}
	for _, u := range us {
		presentes[u.Simbolo] = true
		if u.EmpresaID != empDemo {
			t.Errorf("unidad %q de empresa %q; quería %q", u.Simbolo, u.EmpresaID, empDemo)
		}
	}
	for _, esp := range []string{"unidad", "docena", "par", "caja", "kg", "g", "L", "mL", "m", "cm"} {
		if !presentes[esp] {
			t.Errorf("faltó la unidad por defecto %q en el seed", esp)
		}
	}
}

func TestCrearUnidad_NormalizaYPersiste(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	out, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "  gal  ", Nombre: "  Galón ", Categoria: "Volumen",
	})
	if err != nil {
		t.Fatalf("crear unidad válida: %v", err)
	}
	if out.Simbolo != "gal" {
		t.Errorf("símbolo debía recortarse a %q, se obtuvo %q", "gal", out.Simbolo)
	}
	if out.Categoria != unidadmedida.CategoriaVolumen {
		t.Errorf("categoría debía normalizarse a %q, se obtuvo %q", unidadmedida.CategoriaVolumen, out.Categoria)
	}
	if !out.Activa {
		t.Errorf("una unidad nueva debía arrancar activa")
	}
	if out.ID == "" || out.Creada == "" {
		t.Errorf("la unidad debía recibir ID y fecha de creación (id=%q creada=%q)", out.ID, out.Creada)
	}
}

func TestCrearUnidad_NombreOpcionalPorDefectoSimbolo(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	out, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "saco", Categoria: unidadmedida.CategoriaConteo,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if out.Nombre != "saco" {
		t.Errorf("sin nombre, debía tomar el símbolo %q, se obtuvo %q", "saco", out.Nombre)
	}
}

func TestCrearUnidad_SimboloRequerido(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	if _, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "   ", Categoria: unidadmedida.CategoriaConteo,
	}); err == nil {
		t.Errorf("un símbolo vacío debía fallar")
	}
}

func TestCrearUnidad_CategoriaInvalida(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	if _, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "xx", Categoria: "masa",
	}); !errors.Is(err, application.ErrCategoriaInvalida) {
		t.Errorf("una categoría inválida debía dar ErrCategoriaInvalida, se obtuvo %v", err)
	}
}

func TestCrearUnidad_SimboloDuplicadoRechazado(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	// "kg" ya está sembrada; un alta con la misma caja o distinta debe colisionar
	// (la unicidad es case-insensitive).
	if _, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "kg", Categoria: unidadmedida.CategoriaPeso,
	}); !errors.Is(err, application.ErrUnidadDuplicada) {
		t.Errorf("un símbolo repetido debía dar ErrUnidadDuplicada, se obtuvo %v", err)
	}
	if _, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "KG", Categoria: unidadmedida.CategoriaPeso,
	}); !errors.Is(err, application.ErrUnidadDuplicada) {
		t.Errorf("un símbolo repetido (otra caja) debía dar ErrUnidadDuplicada, se obtuvo %v", err)
	}
}

func TestActualizarUnidad_OKYNoExiste(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	nueva, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "rollo", Categoria: unidadmedida.CategoriaConteo,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	out, err := svc.ActualizarUnidad(empDemo, nueva.ID, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "rollo", Nombre: "Rollo grande", Categoria: unidadmedida.CategoriaConteo,
	}, bptr(true))
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.Nombre != "Rollo grande" {
		t.Errorf("el nombre no se actualizó, se obtuvo %q", out.Nombre)
	}
	if out.Creada != nueva.Creada {
		t.Errorf("la fecha de creación debía conservarse (%q → %q)", nueva.Creada, out.Creada)
	}
	if _, err := svc.ActualizarUnidad(empDemo, "um_inexistente", actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "z", Categoria: unidadmedida.CategoriaConteo,
	}, bptr(true)); !errors.Is(err, application.ErrUnidadNoExiste) {
		t.Errorf("editar un id ajeno debía dar ErrUnidadNoExiste, se obtuvo %v", err)
	}
}

// TestActualizarUnidad_PATCHParcialConservaActiva verifica la semántica de PATCH
// parcial: sin `activa` (nil) se conserva el estado actual; con activa=false sí se
// desactiva. Antes, el bind de un bool ponía Activa=false por omisión y un PATCH
// que solo editaba el nombre desactivaba la unidad.
func TestActualizarUnidad_PATCHParcialConservaActiva(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	// "saco" no está en el seed por defecto, así que no colisiona.
	nueva, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "saco", Categoria: unidadmedida.CategoriaConteo,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if !nueva.Activa {
		t.Fatalf("una unidad nueva debía arrancar activa")
	}
	// PATCH sin `activa` (nil): conserva la actual (activa).
	out, err := svc.ActualizarUnidad(empDemo, nueva.ID, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "saco", Nombre: "Saco 50kg", Categoria: unidadmedida.CategoriaConteo,
	}, nil)
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if !out.Activa {
		t.Errorf("un PATCH sin `activa` no debía desactivar la unidad")
	}
	if out.Nombre != "Saco 50kg" {
		t.Errorf("el nombre no se actualizó, se obtuvo %q", out.Nombre)
	}
	// activa=false explícito sí la desactiva.
	out, err = svc.ActualizarUnidad(empDemo, nueva.ID, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "saco", Categoria: unidadmedida.CategoriaConteo,
	}, bptr(false))
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.Activa {
		t.Errorf("activa=false explícito debía desactivar la unidad")
	}
}

func TestDesactivarUnidad_SoftDisable(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	nueva, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "fardo", Categoria: unidadmedida.CategoriaConteo,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if err := svc.DesactivarUnidad(empDemo, nueva.ID, actorA, origenTst); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	// Sigue existiendo en el listado completo, pero fuera de las activas.
	for _, u := range svc.Unidades(empDemo) {
		if u.ID == nueva.ID && u.Activa {
			t.Errorf("la unidad desactivada seguía activa")
		}
	}
	for _, u := range svc.UnidadesActivas(empDemo) {
		if u.ID == nueva.ID {
			t.Errorf("una unidad desactivada no debía aparecer en UnidadesActivas")
		}
	}
	if err := svc.DesactivarUnidad(empDemo, "um_inexistente", actorA, origenTst); !errors.Is(err, application.ErrUnidadNoExiste) {
		t.Errorf("desactivar un id ajeno debía dar ErrUnidadNoExiste, se obtuvo %v", err)
	}
}

func TestUnidades_AislamientoPorEmpresa(t *testing.T) {
	svc, _ := servicioConUnidades(t)
	const otra = "emp_otra"
	if _, err := svc.CrearUnidad(otra, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "barril", Categoria: unidadmedida.CategoriaVolumen,
	}); err != nil {
		t.Fatalf("crear en otra empresa: %v", err)
	}
	// El mismo símbolo NO colisiona entre empresas distintas (unicidad por tenant).
	if _, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "barril", Categoria: unidadmedida.CategoriaVolumen,
	}); err != nil {
		t.Fatalf("el mismo símbolo en otra empresa debía permitirse: %v", err)
	}
	for _, u := range svc.Unidades(otra) {
		if u.EmpresaID != otra {
			t.Errorf("Unidades(%q) devolvió una de %q", otra, u.EmpresaID)
		}
	}
	// La empresa demo no ve la única unidad exclusiva de la otra... salvo el símbolo
	// que ambas comparten; se cuenta por tenant.
	if got := len(svc.Unidades(otra)); got != 1 {
		t.Errorf("la otra empresa debía tener 1 unidad propia, tiene %d", got)
	}
}

func TestUnidades_SinCablearDevuelveVacio(t *testing.T) {
	// nuevoServicio no cablea el maestro de unidades: la lista debe salir vacía y
	// las escrituras dar ErrUnidadesNoDisponible (no panic por repo nil).
	svc, _ := nuevoServicio(t)
	if got := svc.Unidades(empDemo); len(got) != 0 {
		t.Errorf("sin repo, Unidades debía salir vacío, trajo %d", len(got))
	}
	if _, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: "x", Categoria: unidadmedida.CategoriaConteo,
	}); !errors.Is(err, application.ErrUnidadesNoDisponible) {
		t.Errorf("sin repo, crear debía dar ErrUnidadesNoDisponible, se obtuvo %v", err)
	}
}
