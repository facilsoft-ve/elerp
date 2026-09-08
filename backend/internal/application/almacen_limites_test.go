package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/almacen"
)

// Un almacén con RubrosAdmitidos rechaza ingresar un producto de otro rubro; sin
// restricción admite cualquiera.
func TestAlmacen_RestriccionPorRubro(t *testing.T) {
	svc := servicioAlmacenes(t)
	if _, err := svc.AsegurarAlmacenPrincipal(empDemo, sede1, "sistema", "sistema"); err != nil {
		t.Fatalf("principal: %v", err)
	}
	// Almacén que SOLO admite un rubro inexistente para REF-2L.
	restringido, err := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{
		SedeID: sede1, Nombre: "Solo lácteos", RubrosAdmitidos: []string{"rubro-inexistente"},
	})
	if err != nil {
		t.Fatalf("crear restringido: %v", err)
	}
	if _, err := svc.Ajustar(empDemo, sede1, restringido.ID, "REF-2L", "prueba", 5, actorA, origenTst); !errors.Is(err, application.ErrRubroNoAdmitido) {
		t.Errorf("ajustar un rubro no admitido debe dar ErrRubroNoAdmitido, se obtuvo %v", err)
	}
	// Un almacén sin restricción admite el mismo producto.
	libre, _ := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "Libre"})
	if _, err := svc.Ajustar(empDemo, sede1, libre.ID, "REF-2L", "prueba", 5, actorA, origenTst); err != nil {
		t.Errorf("un almacén sin restricción debe admitir el producto, se obtuvo %v", err)
	}
}

// La capacidad exige unidad; la ocupación suma la existencia en esa unidad.
func TestAlmacen_CapacidadYOcupacion(t *testing.T) {
	svc := servicioAlmacenes(t)
	if _, err := svc.AsegurarAlmacenPrincipal(empDemo, sede1, "sistema", "sistema"); err != nil {
		t.Fatalf("principal: %v", err)
	}
	// Capacidad sin unidad se rechaza.
	if _, err := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "Cap sin unidad", Capacidad: 100}); !errors.Is(err, application.ErrCapacidadSinUnidad) {
		t.Errorf("capacidad sin unidad debe dar ErrCapacidadSinUnidad, se obtuvo %v", err)
	}
	// Capacidad 100 "unidad": tras ajustar +10 de un producto medido en unidad, la
	// ocupación es 10.
	dep, err := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "Depósito cap", Capacidad: 100, CapacidadUnidad: "unidad"})
	if err != nil {
		t.Fatalf("crear con capacidad: %v", err)
	}
	if _, err := svc.Ajustar(empDemo, sede1, dep.ID, "REF-2L", "carga", 10, actorA, origenTst); err != nil {
		t.Fatalf("ajustar: %v", err)
	}
	if occ := svc.OcupacionDeAlmacen(empDemo, dep); !casi(occ, 10) {
		t.Errorf("la ocupación debía ser 10, se obtuvo %v", occ)
	}
}
