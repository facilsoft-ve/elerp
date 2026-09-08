package caja

import "testing"

// La matemática del ARQUEO (efectivo esperado = fondo + cobros − vuelto,
// diferencia declarado vs esperado, redondeo a 2 decimales) NO vive en este
// paquete de dominio: se pliega en internal/application/caja.go y ya está
// cubierta por internal/application/caja_test.go. Aquí solo se prueba la lógica
// PURA que sí reside en el dominio: los predicados de estado y el normalizador
// de versión de logo.

func TestNormalizarLogoVersion(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   string
	}{
		{"principal", LogoVersionPrincipal, LogoVersionPrincipal},
		{"alterno", LogoVersionAlterno, LogoVersionAlterno},
		{"vacío queda vacío (usar default)", "", ""},
		{"desconocido cae a vacío", "otro", ""},
		{"mayúsculas no coinciden (switch exacto)", "PRINCIPAL", ""},
		{"con espacios no coincide (sin trim)", "principal ", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := NormalizarLogoVersion(c.in); got != c.want {
				t.Errorf("NormalizarLogoVersion(%q) = %q; quería %q", c.in, got, c.want)
			}
		})
	}
}

func TestCajaHabilitada(t *testing.T) {
	casos := []struct {
		nombre string
		estado string
		want   bool
	}{
		{"habilitada admite apertura", EstadoHabilitada, true},
		{"deshabilitada no admite", EstadoDeshabilitada, false},
		{"estado vacío no admite", "", false},
		{"estado desconocido no admite", "pausada", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := (Caja{Estado: c.estado}).Habilitada(); got != c.want {
				t.Errorf("Caja{Estado:%q}.Habilitada() = %v; quería %v", c.estado, got, c.want)
			}
		})
	}
}

func TestSesionAbierta(t *testing.T) {
	casos := []struct {
		nombre string
		cierre string
		want   bool
	}{
		{"sin cierre está abierta", "", true},
		{"con cierre está cerrada", "2026-08-07T12:00:00Z", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := (Sesion{Cierre: c.cierre}).Abierta(); got != c.want {
				t.Errorf("Sesion{Cierre:%q}.Abierta() = %v; quería %v", c.cierre, got, c.want)
			}
		})
	}
}
