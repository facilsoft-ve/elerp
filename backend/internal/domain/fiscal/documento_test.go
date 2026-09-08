package fiscal

import (
	"errors"
	"testing"
)

// TestDigitoVerificadorRIF cubre los 7 valores de auto-verificación del algoritmo
// SENIAT (módulo 11) más los casos de cuerpo/letra inválidos. Si estos valores
// cambian, el algoritmo está mal y el backend dejaría de coincidir con el
// frontend (frontend/src/lib/format.js).
func TestDigitoVerificadorRIF(t *testing.T) {
	casos := []struct {
		letra  byte
		cuerpo string
		dv     int
	}{
		{'J', "12345678", 4},
		{'G', "20000123", 2},
		{'J', "00012345", 4},
		{'J', "40123456", 9},
		{'J', "30099887", 8},
		{'J', "31200011", 2},
		{'J', "31122334", 7},
	}
	for _, c := range casos {
		got, ok := DigitoVerificadorRIF(c.letra, c.cuerpo)
		if !ok {
			t.Fatalf("%c-%s: ok=false, se esperaba dv=%d", c.letra, c.cuerpo, c.dv)
		}
		if got != c.dv {
			t.Errorf("%c-%s: dv=%d, se esperaba %d", c.letra, c.cuerpo, got, c.dv)
		}
	}

	// Cuerpo inválido: distinto de 8 dígitos o con no-dígitos → ok=false.
	for _, cuerpo := range []string{"", "1234567", "123456789", "1234567a"} {
		if _, ok := DigitoVerificadorRIF('J', cuerpo); ok {
			t.Errorf("cuerpo %q debería ser inválido (ok=false)", cuerpo)
		}
	}
	// Letra desconocida → ok=false.
	if _, ok := DigitoVerificadorRIF('X', "12345678"); ok {
		t.Error("letra 'X' debería ser inválida (ok=false)")
	}
	// Minúsculas se normalizan a mayúsculas.
	if dv, ok := DigitoVerificadorRIF('j', "12345678"); !ok || dv != 4 {
		t.Errorf("'j' minúscula: dv=%d ok=%v, se esperaba 4/true", dv, ok)
	}
}

func TestValidarDocumento(t *testing.T) {
	// Válidos.
	ok := []struct{ tipo, numero string }{
		{"V", "123456"},       // cédula 6 dígitos
		{"V", "12345678"},     // cédula 8 dígitos
		{"E", "123456789"},    // cédula extranjera 9 dígitos
		{"J", "12345678-4"},   // RIF con guion y DV correcto
		{"J", "123456784"},    // RIF sin guion, DV correcto
		{"G", "200001232"},    // RIF gubernamental, DV correcto
		{"J", " 40123456-9 "}, // con espacios
		{"P", "ABC123"},       // pasaporte alfanumérico
		{"P", "abc123"},       // pasaporte en minúsculas (se normaliza)
		{"j", "12345678-4"},   // tipo en minúscula
	}
	for _, c := range ok {
		if err := ValidarDocumento(c.tipo, c.numero); err != nil {
			t.Errorf("ValidarDocumento(%q,%q) debería ser válido, dio: %v", c.tipo, c.numero, err)
		}
	}

	// Inválidos, verificando el sentinel esperado.
	bad := []struct {
		tipo, numero string
		want         error
	}{
		{"V", "12345", ErrCedulaInvalida},            // muy corta (5)
		{"V", "1234567890", ErrCedulaInvalida},       // muy larga (10)
		{"V", "1234abcd", ErrCedulaInvalida},         // no dígitos
		{"J", "12345678", ErrRIFLongitud},            // solo 8 (falta DV)
		{"J", "1234567890", ErrRIFLongitud},          // 10 dígitos
		{"J", "12345678-5", ErrRIFDigitoVerificador}, // DV incorrecto (debía 4)
		{"G", "200001239", ErrRIFDigitoVerificador},  // DV incorrecto (debía 2)
		{"P", "AB", ErrPasaporteInvalido},            // muy corto
		{"P", "ABC$123", ErrPasaporteInvalido},       // símbolo no alfanumérico
		{"Z", "12345678", ErrTipoDocumento},          // tipo desconocido
		{"", "12345678", ErrTipoDocumento},           // tipo vacío
	}
	for _, c := range bad {
		err := ValidarDocumento(c.tipo, c.numero)
		if !errors.Is(err, c.want) {
			t.Errorf("ValidarDocumento(%q,%q): err=%v, se esperaba %v", c.tipo, c.numero, err, c.want)
		}
	}
}

func TestValidarDocumentoStr(t *testing.T) {
	ok := []string{"J-12345678-4", "V-12345678", "G-200001232", "P-ABC123", "E-123456789"}
	for _, s := range ok {
		if err := ValidarDocumentoStr(s); err != nil {
			t.Errorf("ValidarDocumentoStr(%q) debería ser válido, dio: %v", s, err)
		}
	}
	bad := []struct {
		full string
		want error
	}{
		{"J-12345678-5", ErrRIFDigitoVerificador},
		{"J-1234567", ErrRIFLongitud},
		{"Z-12345678", ErrTipoDocumento},
		{"", ErrTipoDocumento},
	}
	for _, c := range bad {
		if err := ValidarDocumentoStr(c.full); !errors.Is(err, c.want) {
			t.Errorf("ValidarDocumentoStr(%q): err=%v, se esperaba %v", c.full, err, c.want)
		}
	}
}
