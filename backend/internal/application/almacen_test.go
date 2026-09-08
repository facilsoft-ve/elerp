package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/almacen"
)

func servicioAlmacenes(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	return svc
}

// El primer almacén de una sede nace como principal; marcar otro principal degrada
// al anterior (exactamente uno principal por sede).
func TestAlmacen_PrimeroEsPrincipalYSoloUno(t *testing.T) {
	svc := servicioAlmacenes(t)
	a1, err := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "Principal"})
	if err != nil {
		t.Fatalf("crear a1: %v", err)
	}
	if !a1.Principal {
		t.Error("el primer almacén de la sede debe quedar como principal")
	}
	a2, err := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "Depósito", Tipo: "Refrigerado"})
	if err != nil {
		t.Fatalf("crear a2 (tipo Refrigerado): %v", err)
	}
	if a2.Principal {
		t.Error("el segundo almacén no debe ser principal por defecto")
	}
	if a2.Tipo != almacen.TipoRefrigerado {
		t.Errorf("el tipo 'Refrigerado' debe normalizarse a %q, quedó %q", almacen.TipoRefrigerado, a2.Tipo)
	}
	// Marcar a2 como principal degrada a a1.
	if _, err := svc.ActualizarAlmacen(empDemo, a2.ID, actorA, origenTst, almacen.Almacen{Nombre: "Depósito", Tipo: "refrigerado", Principal: true}, nil); err != nil {
		t.Fatalf("marcar a2 principal: %v", err)
	}
	prin, ok := svc.AlmacenPrincipalDe(empDemo, sede1)
	if !ok || prin.ID != a2.ID {
		t.Errorf("el principal de la sede debe ser a2 (%s), es %+v", a2.ID, prin)
	}
}

func TestAlmacen_NoDesactivarPrincipalNiUltimo(t *testing.T) {
	svc := servicioAlmacenes(t)
	a1, _ := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "Principal"})
	// Único y principal: no se puede desactivar (ambas reglas aplican).
	if err := svc.DesactivarAlmacen(empDemo, a1.ID, actorA, origenTst); !errors.Is(err, application.ErrAlmacenPrincipal) {
		t.Errorf("desactivar el principal debe dar ErrAlmacenPrincipal, se obtuvo %v", err)
	}
	// Con un segundo no-principal, ese sí se puede desactivar; el principal no.
	a2, _ := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "Depósito"})
	if err := svc.DesactivarAlmacen(empDemo, a2.ID, actorA, origenTst); err != nil {
		t.Errorf("desactivar un almacén no principal debía funcionar, se obtuvo %v", err)
	}
}

func TestAlmacen_TipoInvalido(t *testing.T) {
	svc := servicioAlmacenes(t)
	if _, err := svc.CrearAlmacen(empDemo, actorA, origenTst, almacen.Almacen{SedeID: sede1, Nombre: "X", Tipo: "sotano"}); !errors.Is(err, application.ErrAlmacenTipoInvalido) {
		t.Errorf("un tipo fuera de la lista debe dar ErrAlmacenTipoInvalido, se obtuvo %v", err)
	}
}

// El backfill garantiza ≥1 almacén principal por sede y es idempotente.
func TestAsegurarAlmacenPrincipal_Idempotente(t *testing.T) {
	svc := servicioAlmacenes(t)
	a, err := svc.AsegurarAlmacenPrincipal(empDemo, sede2, "sistema", "sistema")
	if err != nil || !a.Principal {
		t.Fatalf("debía crear el Almacén Principal de la sede, se obtuvo %+v (%v)", a, err)
	}
	b, _ := svc.AsegurarAlmacenPrincipal(empDemo, sede2, "sistema", "sistema")
	if b.ID != a.ID {
		t.Errorf("asegurar dos veces no debe crear otro almacén: %s vs %s", a.ID, b.ID)
	}
	if n := len(svc.AlmacenesDeSede(empDemo, sede2)); n != 1 {
		t.Errorf("la sede debe tener exactamente 1 almacén tras dos asegurados, tiene %d", n)
	}
}
