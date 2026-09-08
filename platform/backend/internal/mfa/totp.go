// Package mfa implementa TOTP (RFC 6238) sin dependencias externas: HMAC-SHA1 sobre el
// contador de tiempo (períodos de 30 s), truncado dinámico a 6 dígitos. Es el segundo
// factor obligatorio de los operadores de la consola.
package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	digitos = 6
	periodo = 30 // segundos
	emisor  = "ElERP Plataforma"
)

// base32 sin relleno, en mayúsculas (alfabeto estándar RFC 4648): es el formato que
// entienden Google Authenticator, Authy, 1Password, etc.
var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerarSecreto crea un secreto TOTP nuevo (160 bits) en base32.
func GenerarSecreto() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return enc.EncodeToString(buf), nil
}

// OtpauthURL arma el URI otpauth:// que se codifica como QR para enrolar el autenticador.
func OtpauthURL(secreto, email string) string {
	label := url.PathEscape(emisor + ":" + email)
	q := url.Values{}
	q.Set("secret", secreto)
	q.Set("issuer", emisor)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", digitos))
	q.Set("period", fmt.Sprintf("%d", periodo))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// Codigo calcula el código TOTP de un secreto para un instante dado (útil en tests).
func Codigo(secreto string, t time.Time) (string, error) {
	clave, err := enc.DecodeString(strings.ToUpper(strings.TrimSpace(secreto)))
	if err != nil {
		return "", err
	}
	return hotp(clave, uint64(t.Unix())/periodo), nil
}

// Validar comprueba un código contra el secreto, tolerando ±1 ventana de tiempo (para
// el desfase de reloj del teléfono). Devuelve true si coincide en algún paso.
func Validar(secreto, codigo string, ahora time.Time) bool {
	codigo = strings.TrimSpace(codigo)
	if len(codigo) != digitos {
		return false
	}
	clave, err := enc.DecodeString(strings.ToUpper(strings.TrimSpace(secreto)))
	if err != nil {
		return false
	}
	contador := uint64(ahora.Unix()) / periodo
	for _, delta := range []int64{0, -1, 1} {
		if hmac.Equal([]byte(hotp(clave, uint64(int64(contador)+delta))), []byte(codigo)) {
			return true
		}
	}
	return false
}

// hotp es HOTP (RFC 4226): HMAC-SHA1(clave, contador) + truncado dinámico → N dígitos.
func hotp(clave []byte, contador uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], contador)
	h := hmac.New(sha1.New, clave)
	h.Write(buf[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])
	return fmt.Sprintf("%0*d", digitos, code%1_000_000)
}
