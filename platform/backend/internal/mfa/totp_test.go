package mfa

import (
	"testing"
	"time"
)

func TestTOTP_CodigoValida(t *testing.T) {
	secreto, err := GenerarSecreto()
	if err != nil {
		t.Fatalf("generar secreto: %v", err)
	}
	ahora := time.Unix(1_700_000_000, 0)
	cod, err := Codigo(secreto, ahora)
	if err != nil {
		t.Fatalf("codigo: %v", err)
	}
	if len(cod) != 6 {
		t.Fatalf("el código TOTP debe tener 6 dígitos, se obtuvo %q", cod)
	}
	if !Validar(secreto, cod, ahora) {
		t.Fatal("el código recién calculado debe validar")
	}
	if Validar(secreto, "000000", ahora) && cod != "000000" {
		t.Fatal("un código arbitrario no debe validar")
	}
}

func TestTOTP_ToleraDesfase(t *testing.T) {
	secreto, _ := GenerarSecreto()
	base := time.Unix(1_700_000_000, 0)
	// Código del período anterior debe seguir siendo válido (ventana ±1).
	cod, _ := Codigo(secreto, base.Add(-30*time.Second))
	if !Validar(secreto, cod, base) {
		t.Fatal("el código del período anterior debe validar (tolerancia de reloj)")
	}
	// Dos períodos atrás ya NO debe validar.
	viejo, _ := Codigo(secreto, base.Add(-90*time.Second))
	if Validar(secreto, viejo, base) && viejo == cod {
		t.Skip("colisión improbable de códigos")
	}
	if Validar(secreto, viejo, base) {
		t.Fatal("un código de dos períodos atrás no debe validar")
	}
}
