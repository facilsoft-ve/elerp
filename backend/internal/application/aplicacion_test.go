package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
)

func servicioModulos(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConModulos(st.Modulos)
	return svc
}

func contiene(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

// Un módulo comercial (marketing) arranca inactivo; instalar lo activa; desactivar
// lo apaga sin desinstalar; activar lo vuelve a prender.
func TestModulos_InstalarActivarDesactivarMarketing(t *testing.T) {
	svc := servicioModulos(t)
	if svc.ModuloActivo(empDemo, "marketing") {
		t.Fatal("marketing no debería estar activo antes de instalarlo")
	}
	if !contiene(svc.ModulosActivos(empDemo), "inventario") {
		t.Error("los módulos core deben estar siempre activos (inventario)")
	}
	if _, err := svc.Instalar(empDemo, "marketing", actorA, origenTst); err != nil {
		t.Fatalf("instalar marketing: %v", err)
	}
	if !svc.ModuloActivo(empDemo, "marketing") || !contiene(svc.ModulosActivos(empDemo), "marketing") {
		t.Error("tras instalar, marketing debe quedar activo")
	}
	if _, err := svc.Desactivar(empDemo, "marketing", actorA, origenTst); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if svc.ModuloActivo(empDemo, "marketing") {
		t.Error("tras desactivar, marketing no debe estar activo")
	}
	if _, err := svc.Activar(empDemo, "marketing", actorA, origenTst); err != nil {
		t.Fatalf("activar: %v", err)
	}
	if !svc.ModuloActivo(empDemo, "marketing") {
		t.Error("tras activar, marketing debe estar activo de nuevo")
	}
}

// Un módulo core (incluido) no se instala ni se desactiva, y siempre está activo.
func TestModulos_CoreNoSeApaga(t *testing.T) {
	svc := servicioModulos(t)
	if _, err := svc.Instalar(empDemo, "inventario", actorA, origenTst); !errors.Is(err, application.ErrModuloCore) {
		t.Errorf("instalar un core debe dar ErrModuloCore, se obtuvo %v", err)
	}
	if _, err := svc.Desactivar(empDemo, "facturacion", actorA, origenTst); !errors.Is(err, application.ErrModuloCore) {
		t.Errorf("desactivar un core debe dar ErrModuloCore, se obtuvo %v", err)
	}
	if !svc.ModuloActivo(empDemo, "contabilidad") {
		t.Error("un módulo core debe estar siempre activo")
	}
}

// Un id desconocido no se instala; activar sin instalar tampoco. (Hoy el catálogo no
// tiene módulos "próximamente"; la rama ErrModuloProximamente queda cubierta por la
// lógica, no por un módulo real.)
func TestModulos_DesconocidoYSinInstalar(t *testing.T) {
	svc := servicioModulos(t)
	if _, err := svc.Instalar(empDemo, "no-existe", actorA, origenTst); !errors.Is(err, application.ErrModuloNoExiste) {
		t.Errorf("un id desconocido debe dar ErrModuloNoExiste, se obtuvo %v", err)
	}
	// Activar sin instalar.
	if _, err := svc.Activar(empDemo, "marketing", actorA, origenTst); !errors.Is(err, application.ErrModuloNoInstalado) {
		t.Errorf("activar sin instalar debe dar ErrModuloNoInstalado, se obtuvo %v", err)
	}
}
