package application_test

import (
	"errors"
	"regexp"
	"testing"

	"github.com/mornix/elerp/internal/application"
)

var reControl = regexp.MustCompile(`^00-\d{8}$`)

// El SENIAT exige un Número de Control propio (forma libre): correlativo y con el
// formato NN-NNNNNNNN, distinto del número de factura.
func TestNumeroDeControl_FormaLibreCorrelativo(t *testing.T) {
	svc, _ := nuevoServicio(t)

	d1 := emitirContado(t, svc, sede1, 100)
	d2 := emitirContado(t, svc, sede1, 100)

	if !reControl.MatchString(d1.NumeroControl) {
		t.Errorf("número de control mal formado: %q", d1.NumeroControl)
	}
	if d1.NumeroControl == d2.NumeroControl {
		t.Errorf("el número de control debe ser correlativo (distinto): %q == %q", d1.NumeroControl, d2.NumeroControl)
	}
	if d1.NumeroControl == d1.NumeroCompleto {
		t.Error("el número de control debe ser DISTINTO del número de factura")
	}
}

// Configurar el rango aplica prefijo y arranca el correlativo en 'desde'; es
// forward-only (no se reinicia por debajo de lo ya emitido) y valida el prefijo.
func TestConfigurarNumeroControl_PrefijoRangoYForwardOnly(t *testing.T) {
	svc, _ := nuevoServicio(t)

	if _, err := svc.ConfigurarNumeroControl(empDemo, actorA, origenTst, "01", 500, 1000); err != nil {
		t.Fatalf("configurar: %v", err)
	}
	d := emitirContado(t, svc, sede1, 100)
	if d.NumeroControl != "01-00000500" {
		t.Errorf("el próximo control debía ser 01-00000500, fue %q", d.NumeroControl)
	}
	v := svc.EstadoNumeroControl(empDemo)
	if v.Prefijo != "01" || v.Actual != 500 || v.Restantes != 500 {
		t.Errorf("estado inesperado: %+v", v)
	}
	// No se puede reiniciar antes de lo ya emitido.
	if _, err := svc.ConfigurarNumeroControl(empDemo, actorA, origenTst, "01", 100, 1000); !errors.Is(err, application.ErrNumeroControlUsado) {
		t.Errorf("reiniciar antes debe dar ErrNumeroControlUsado, fue %v", err)
	}
	// Prefijo inválido.
	if _, err := svc.ConfigurarNumeroControl(empDemo, actorA, origenTst, "ABC", 0, 0); !errors.Is(err, application.ErrNumeroControlPrefijo) {
		t.Errorf("prefijo inválido debe dar ErrNumeroControlPrefijo, fue %v", err)
	}
}
