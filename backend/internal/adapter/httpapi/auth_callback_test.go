package httpapi

import "testing"

// La regla de seguridad del login SSO, fijada por escrito.
//
// Hubmy entra de dos maneras y hay que tratarlas distinto:
//   - Iniciado por LA APP: nosotros emitimos el nonce en /api/auth/login, así que la
//     cookie existe y el `state` que vuelve tiene que ser idéntico. Es la protección
//     anti-CSRF y no se negocia.
//   - Iniciado por LA PLATAFORMA: el launcher abre la app y salta directo al callback,
//     sin haber pasado por nuestro /login. No hay nonce propio con el que comparar;
//     exigir uno rechazaba TODO login desde el launcher (pasó: 400 en cada intento).
func TestAceptarState(t *testing.T) {
	casos := []struct {
		nombre           string
		want, got        string
		esperaPlataforma bool
		esperaAceptar    bool
	}{
		{
			nombre: "iniciado por la app y el nonce coincide → se acepta",
			want:   "abc123", got: "abc123",
			esperaPlataforma: false, esperaAceptar: true,
		},
		{
			nombre: "iniciado por la app y el nonce NO coincide → se rechaza",
			want:   "abc123", got: "otro",
			esperaPlataforma: false, esperaAceptar: false,
		},
		{
			nombre: "iniciado por la app y no vuelve nonce → se rechaza",
			want:   "abc123", got: "",
			esperaPlataforma: false, esperaAceptar: false,
		},
		{
			nombre: "sin cookie propia → login de la plataforma, se acepta",
			want:   "", got: "state-de-hubmy",
			esperaPlataforma: true, esperaAceptar: true,
		},
		{
			nombre: "sin cookie y sin state → igual es login de la plataforma (decide el JWT)",
			want:   "", got: "",
			esperaPlataforma: true, esperaAceptar: true,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			plataforma, aceptar := aceptarState(c.want, c.got)
			if aceptar != c.esperaAceptar {
				t.Errorf("aceptar: se esperaba %v, se obtuvo %v", c.esperaAceptar, aceptar)
			}
			if plataforma != c.esperaPlataforma {
				t.Errorf("iniciadoPorPlataforma: se esperaba %v, se obtuvo %v", c.esperaPlataforma, plataforma)
			}
		})
	}
}

// Un nonce emitido por nosotros NUNCA se puede saltar mandando el state vacío: si eso
// pasara, cualquiera podría degradar un login protegido a uno sin protección.
func TestAceptarState_NoSePuedeDegradarUnLoginProtegido(t *testing.T) {
	for _, got := range []string{"", " ", "null", "undefined", "0"} {
		if _, aceptar := aceptarState("nonce-real", got); aceptar {
			t.Errorf("con nonce emitido, un state %q no debe aceptarse", got)
		}
	}
}
