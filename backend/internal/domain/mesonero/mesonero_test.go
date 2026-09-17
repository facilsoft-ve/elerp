package mesonero

import "testing"

// El cierre suave es la regla de negocio de este paquete, y vive entera en estos
// dos predicados: en «cerrando» el mesonero SIGUE trabajando pero YA NO toma
// mesas nuevas. Si alguien los colapsa en uno solo, el turno vuelve a apagarse
// de golpe y las mesas abiertas quedan sin quién las atienda.
func TestTurno_CerrandoSigueVivoPeroNoTomaMesas(t *testing.T) {
	casos := []struct {
		estado    string
		vivo      bool
		tomaMesas bool
		porQue    string
	}{
		{EstadoAbierto, true, true, "abierto trabaja con normalidad"},
		{EstadoCerrando, true, false, "cerrando atiende lo suyo pero no toma más"},
		{EstadoCerrado, false, false, "cerrado no hace nada"},
	}
	for _, c := range casos {
		tu := Turno{Estado: c.estado}
		if tu.Vivo() != c.vivo {
			t.Errorf("%s: Vivo() = %v, se esperaba %v (%s)", c.estado, tu.Vivo(), c.vivo, c.porQue)
		}
		if tu.PuedeTomarMesas() != c.tomaMesas {
			t.Errorf("%s: PuedeTomarMesas() = %v, se esperaba %v (%s)", c.estado, tu.PuedeTomarMesas(), c.tomaMesas, c.porQue)
		}
	}
}

func TestEstadoValido(t *testing.T) {
	for _, e := range []string{EstadoAbierto, EstadoCerrando, EstadoCerrado} {
		if !EstadoValido(e) {
			t.Errorf("%q debería ser un estado válido", e)
		}
	}
	for _, e := range []string{"", "pendiente", "pausado", "ABIERTO"} {
		if EstadoValido(e) {
			t.Errorf("%q no debería ser un estado válido", e)
		}
	}
}

// Sin PIN fijado no se puede iniciar turno: la credencial existe pero todavía no
// identifica a nadie. Un PIN en blanco NUNCA puede pasar por «tiene PIN», o una
// credencial recién creada abriría turno sin que su dueño la haya tocado.
func TestMesonero_TienePin(t *testing.T) {
	if (Mesonero{}).TienePin() {
		t.Error("una credencial recién creada no tiene PIN")
	}
	if (Mesonero{PinHash: "   "}).TienePin() {
		t.Error("un hash en blanco no es un PIN")
	}
	if !(Mesonero{PinHash: "$2a$10$algoquepareceunhash"}).TienePin() {
		t.Error("con hash sí tiene PIN")
	}
}
