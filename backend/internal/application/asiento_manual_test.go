package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
)

func lineasCuadradas() []application.LineaAsientoManual {
	// Ajuste de caja contra otros ingresos: 100 al debe (Caja 1101) y 100 al haber
	// (una cuenta de plantilla, 4103). Cuadra.
	return []application.LineaAsientoManual{
		{Codigo: "1101", Debe: 100},
		{Codigo: "4103", Haber: 100},
	}
}

// Un asiento manual válido se anexa, cuadrado y con RefTipo "manual".
func TestAsientoManual_SeAnexaYCuadra(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.PlanDeCuentas(empDemo) // materializa el plan (incluye 4103 de plantilla)

	a, err := svc.RegistrarAsientoManual(empDemo, actorA, origenTst, "2999-01-15", "Ajuste de caja", lineasCuadradas())
	if err != nil {
		t.Fatalf("registrar: %v", err)
	}
	if a.RefTipo != "manual" || !a.Cuadra() || a.Codigo == "" {
		t.Fatalf("asiento manual inválido: %+v", a)
	}
	// Aparece en el libro.
	visto := false
	for _, x := range svc.LibroDiario(empDemo) {
		if x.ID == a.ID {
			visto = true
		}
	}
	if !visto {
		t.Error("el asiento manual debería aparecer en el libro diario")
	}
}

// Se rechazan: descuadre, menos de dos líneas, línea inválida, cuenta inexistente,
// descripción vacía.
func TestAsientoManual_Validaciones(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.PlanDeCuentas(empDemo)

	casos := []struct {
		nombre string
		desc   string
		lineas []application.LineaAsientoManual
		err    error
	}{
		{"descuadre", "x", []application.LineaAsientoManual{{Codigo: "1101", Debe: 100}, {Codigo: "4103", Haber: 90}}, application.ErrAsientoDescuadrado},
		{"una línea", "x", []application.LineaAsientoManual{{Codigo: "1101", Debe: 100}}, application.ErrAsientoMinLineas},
		{"ambos lados", "x", []application.LineaAsientoManual{{Codigo: "1101", Debe: 100, Haber: 100}, {Codigo: "4103", Haber: 100}}, application.ErrLineaAsientoInvalida},
		{"cuenta inexistente", "x", []application.LineaAsientoManual{{Codigo: "9999", Debe: 100}, {Codigo: "4103", Haber: 100}}, application.ErrCuentaNoExiste},
		{"sin descripción", "", lineasCuadradas(), application.ErrDescripcionAsiento},
	}
	for _, c := range casos {
		if _, err := svc.RegistrarAsientoManual(empDemo, actorA, origenTst, "2999-01-15", c.desc, c.lineas); !errors.Is(err, c.err) {
			t.Errorf("%s: esperaba %v, se obtuvo %v", c.nombre, c.err, err)
		}
	}
}

// No se puede asentar en una cuenta desactivada.
func TestAsientoManual_RechazaCuentaDesactivada(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.PlanDeCuentas(empDemo)
	if _, err := svc.CrearCuenta(empDemo, "6301", "Cuenta apagada", "gasto", "", actorA, origenTst); err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := svc.FijarCuentaActiva(empDemo, "6301", false, actorA, origenTst); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	lin := []application.LineaAsientoManual{{Codigo: "6301", Debe: 50}, {Codigo: "1101", Haber: 50}}
	if _, err := svc.RegistrarAsientoManual(empDemo, actorA, origenTst, "2999-01-15", "usa apagada", lin); !errors.Is(err, application.ErrCuentaDesactivada) {
		t.Errorf("asentar en cuenta desactivada debe dar ErrCuentaDesactivada, se obtuvo %v", err)
	}
}
