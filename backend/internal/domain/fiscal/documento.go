package fiscal

import (
	"errors"
	"strings"
)

// Validación de documento de identidad fiscal venezolano (formato SENIAT).
//
// Este es el MISMO algoritmo que el frontend (frontend/src/lib/format.js):
// dígito verificador del RIF por módulo 11. El backend debe coincidir EXACTO,
// porque es la última línea de defensa: la UI solo avisa, el dominio valida.
//
// Tipos:
//   · V / E — cédula de persona natural (venezolano / extranjero): SOLO dígitos,
//     la cédula tal cual, SIN dígito verificador (6–9 dígitos).
//   · J / G — RIF jurídico / gubernamental: 9 dígitos = 8 de cuerpo + 1 dígito
//     verificador (p. ej. J-12345678-4).
//   · P — pasaporte (extranjero sin cédula): alfanumérico, 4–20 caracteres.

// Errores de validación de documento (sentinels, mensajes en español).
var (
	ErrTipoDocumento        = errors.New("tipo de documento inválido: debe ser V, E, J, G o P")
	ErrCedulaInvalida       = errors.New("la cédula debe tener entre 6 y 9 dígitos")
	ErrRIFLongitud          = errors.New("el RIF debe tener 9 dígitos (8 de cuerpo + 1 dígito verificador)")
	ErrRIFDigitoVerificador = errors.New("el dígito verificador del RIF no es correcto")
	ErrPasaporteInvalido    = errors.New("el pasaporte debe tener entre 4 y 20 caracteres (letras o números)")
)

// valorLetraRIF asigna a cada tipo su valor numérico en el cálculo del DV.
// Incluye C (contribuyente especial) por completitud del algoritmo SENIAT,
// aunque ElERP no lo use como tipo de documento de cliente.
var valorLetraRIF = map[byte]int{'V': 1, 'E': 2, 'J': 3, 'P': 4, 'G': 5, 'C': 6}

// pesosRIF son los pesos del módulo 11. Índice 0 = letra, 1..8 = los 8 dígitos
// del cuerpo.
var pesosRIF = [9]int{4, 3, 2, 7, 6, 5, 4, 3, 2}

// DigitoVerificadorRIF calcula el dígito verificador (0–9) del RIF por el
// algoritmo módulo 11 del SENIAT. `letra` es el tipo (V/E/J/G/P/C) y `cuerpo`
// los 8 dígitos del cuerpo. El segundo valor es false si el cuerpo no son
// exactamente 8 dígitos o la letra no es conocida.
func DigitoVerificadorRIF(letra byte, cuerpo string) (int, bool) {
	if letra >= 'a' && letra <= 'z' {
		letra -= 'a' - 'A'
	}
	val, ok := valorLetraRIF[letra]
	if !ok {
		return 0, false
	}
	if len(cuerpo) != 8 {
		return 0, false
	}
	suma := val * pesosRIF[0]
	for i := 0; i < 8; i++ {
		d := cuerpo[i]
		if d < '0' || d > '9' {
			return 0, false
		}
		suma += int(d-'0') * pesosRIF[i+1]
	}
	dv := 11 - (suma % 11)
	if dv >= 10 {
		dv = 0
	}
	return dv, true
}

// limpiarNumero quita guiones y espacios del cuerpo del documento.
func limpiarNumero(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' || c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func soloDigitos(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func alfanumMayus(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// ValidarDocumento valida un documento fiscal según su tipo. `tipo` debe ser uno
// de V/E/J/G/P; a `numero` se le quitan guiones y espacios antes de validar.
// Para J/G además verifica que el 9º dígito coincida con el DV módulo 11 de los
// primeros 8. V/E NO llevan dígito verificador. Devuelve nil si es válido.
func ValidarDocumento(tipo, numero string) error {
	tipo = strings.ToUpper(strings.TrimSpace(tipo))
	num := strings.ToUpper(limpiarNumero(numero))

	switch tipo {
	case "V", "E":
		if !soloDigitos(num) || len(num) < 6 || len(num) > 9 {
			return ErrCedulaInvalida
		}
		return nil
	case "J", "G":
		if len(num) != 9 || !soloDigitos(num) {
			return ErrRIFLongitud
		}
		dv, ok := DigitoVerificadorRIF(tipo[0], num[:8])
		if !ok || byte('0'+dv) != num[8] {
			return ErrRIFDigitoVerificador
		}
		return nil
	case "P":
		if len(num) < 4 || len(num) > 20 || !alfanumMayus(num) {
			return ErrPasaporteInvalido
		}
		return nil
	default:
		return ErrTipoDocumento
	}
}

// ValidarDocumentoStr valida un documento con prefijo de tipo ("J-12345678-4",
// "V-12345678"): toma la 1ª letra como tipo y el resto como número, y delega en
// ValidarDocumento.
func ValidarDocumentoStr(full string) error {
	s := strings.TrimSpace(full)
	if s == "" {
		return ErrTipoDocumento
	}
	tipo := s[:1]
	numero := s[1:]
	return ValidarDocumento(tipo, numero)
}
