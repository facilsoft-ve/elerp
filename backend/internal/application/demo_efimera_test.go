package application_test

import (
	"strings"
	"testing"

	"github.com/mornix/elerp/internal/application"
)

/* DEMO EFÍMERA.
 *
 * Lo que se prueba acá es que la copia NO pueda tocar un tenant real. El resto
 * —que los datos se dupliquen— lo verifica el adaptador contra la base; lo que
 * no puede fallar nunca es el candado. */

// copiaFalsa registra qué se le pidió, sin tocar ninguna base.
type copiaFalsa struct {
	clonados [][2]string
	borrados []string
	seq      int
}

func (c *copiaFalsa) Clonar(origenID, destinoID string) int {
	c.clonados = append(c.clonados, [2]string{origenID, destinoID})
	return 10
}
func (c *copiaFalsa) Borrar(empresaID string) int {
	c.borrados = append(c.borrados, empresaID)
	return 1
}
func (c *copiaFalsa) BarrerVencidas() int { return 0 }
func (c *copiaFalsa) NuevoID() string {
	c.seq++
	return "empdemo_prueba" + string(rune('a'+c.seq))
}

// Sin copia cableada, el modo demo se comporta como antes: entra al tenant
// compartido. Es el respaldo de una instancia sin persistencia, donde cada
// arranque ya empieza limpio.
func TestDemoEfimera_SinCableadoNoAplica(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if svc.HayDemoEfimera() {
		t.Fatal("sin cablear no debería ofrecer copias")
	}
	if _, err := svc.CrearSesionDemo(nil, []string{empDemo}, actorA, origenTst); err == nil {
		t.Fatal("sin copia disponible debería fallar en vez de entregar algo a medias")
	}
}

// Cada visitante recibe un USUARIO propio: dos personas en la demo a la vez no
// pueden compartir identidad, o la bitácora diría que la misma hizo las dos
// cosas.
func TestDemoEfimera_CadaVisitanteTieneSuIdentidad(t *testing.T) {
	svc, st := nuevoServicio(t)
	falsa := &copiaFalsa{}
	svc.ConCopiaDemo(falsa)
	tn := application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)

	a, err := svc.CrearSesionDemo(tn, []string{empDemo}, actorA, origenTst)
	if err != nil {
		t.Fatalf("primera sesión: %v", err)
	}
	b, err := svc.CrearSesionDemo(tn, []string{empDemo}, actorA, origenTst)
	if err != nil {
		t.Fatalf("segunda sesión: %v", err)
	}
	if a.UsuarioID == b.UsuarioID {
		t.Fatal("dos visitantes compartieron identidad")
	}
	if len(a.Empresas) == 0 || a.Empresas[0] == b.Empresas[0] {
		t.Fatalf("dos visitantes compartieron empresa: %v / %v", a.Empresas, b.Empresas)
	}
	// Y la copia se pidió desde la empresa base, no desde otra cosa.
	if len(falsa.clonados) != 2 || falsa.clonados[0][0] != empDemo {
		t.Fatalf("clonados = %v", falsa.clonados)
	}
}

// La empresa copiada lleva el prefijo que la hace reconocible y su fecha de
// expiración: sin eso, la limpieza no sabría a cuál borrar y no podría borrar
// ninguna sin riesgo.
func TestDemoEfimera_LaCopiaSeReconoceYExpira(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConCopiaDemo(&copiaFalsa{})
	tn := application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)

	s, err := svc.CrearSesionDemo(tn, []string{empDemo}, actorA, origenTst)
	if err != nil {
		t.Fatalf("sesión: %v", err)
	}
	emp, ok := st.Empresas.ByID(s.Empresas[0])
	if !ok {
		t.Fatal("la empresa copiada no se creó")
	}
	if !strings.HasPrefix(emp.ID, "empdemo_") {
		t.Fatalf("la copia tiene que ser reconocible por su id: %q", emp.ID)
	}
	if emp.ExpiraEl == "" {
		t.Fatal("una copia sin fecha de expiración no se borra nunca")
	}
	if emp.OrigenSandboxID != empDemo {
		t.Fatalf("la copia debe recordar de dónde salió: %q", emp.OrigenSandboxID)
	}
	// Las SEDES conservan su id: todo el ledger las referencia, y remapearlas
	// obligaría a reescribir cada referencia de cada colección.
	sedesBase := st.Sedes.List(empDemo)
	sedesCopia := st.Sedes.List(emp.ID)
	if len(sedesCopia) != len(sedesBase) {
		t.Fatalf("la copia tiene %d sedes y la base %d", len(sedesCopia), len(sedesBase))
	}
	if len(sedesBase) > 0 && sedesCopia[0].ID != sedesBase[0].ID {
		t.Fatalf("la sede cambió de id: %q → %q", sedesBase[0].ID, sedesCopia[0].ID)
	}
}

// Una base que no existe no puede frenar a las demás: la demostración tiene que
// abrir aunque falte un rubro.
func TestDemoEfimera_UnaBaseRotaNoTumbaLaSesion(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConCopiaDemo(&copiaFalsa{})
	tn := application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)

	s, err := svc.CrearSesionDemo(tn, []string{"emp_que_no_existe", empDemo}, actorA, origenTst)
	if err != nil {
		t.Fatalf("una base inexistente no debería tumbar la sesión: %v", err)
	}
	if len(s.Empresas) != 1 {
		t.Fatalf("se esperaba una empresa copiada, hay %d", len(s.Empresas))
	}
}
