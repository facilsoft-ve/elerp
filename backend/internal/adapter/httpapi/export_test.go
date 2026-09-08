package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// El cifrado del export es reversible con la misma clave y opaco frente al claro.
func TestCifrarDescifrarGCM_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	k := base64.StdEncoding.EncodeToString(key)
	msg := []byte("respaldo de prueba ElERP — datos del tenant")

	enc, err := cifrarGCM(msg, k)
	if err != nil {
		t.Fatalf("cifrar: %v", err)
	}
	if bytes.Contains(enc, msg) {
		t.Error("el cifrado no debe contener el texto claro")
	}
	dec, err := descifrarGCM(enc, k)
	if err != nil {
		t.Fatalf("descifrar: %v", err)
	}
	if !bytes.Equal(dec, msg) {
		t.Errorf("round-trip falló: %q", dec)
	}
}

// Una clave que no es base64 de 32 bytes se rechaza (no cifra en claro por error).
func TestCifrarGCM_ClaveInvalida(t *testing.T) {
	if _, err := cifrarGCM([]byte("x"), "clave-corta"); err == nil {
		t.Error("una clave inválida debe fallar")
	}
	// 16 bytes (AES-128) tampoco: exigimos 32 (AES-256).
	k16 := base64.StdEncoding.EncodeToString(make([]byte, 16))
	if _, err := cifrarGCM([]byte("x"), k16); err == nil {
		t.Error("una clave de 16 bytes debe rechazarse (se exige AES-256)")
	}
}
