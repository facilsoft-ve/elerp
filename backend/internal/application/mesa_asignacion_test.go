package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// servicioSalon monta el módulo Restaurante con mesas, cuentas y asignaciones.
func servicioSalon(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConMesas(st.Mesas, st.Planos)
	svc.ConCuentas(st.Cuentas)
	svc.ConAsignacionMesas(st.Asignaciones, st.ConfigSalon)
	return svc, st
}

const (
	empSalon  = "emp_demo_rest"
	sedeSalon = "sede_demo_rest"
)

// mesaPorNombre busca una mesa sembrada del restaurante demo.
func mesaPorNombre(t *testing.T, st *inmem.Store, nombre string) mesa.Mesa {
	t.Helper()
	for _, m := range st.Mesas.List(empSalon, sedeSalon) {
		if m.Nombre == nombre {
			return m
		}
	}
	t.Fatalf("no se encontró la mesa %q en el restaurante demo", nombre)
	return mesa.Mesa{}
}

// Sin asignación, cualquier mesonero atiende cualquier mesa: es el caso normal de un
// local chico y no debe pedir configurar nada.
func TestAsignacion_SinAsignarAtiendeCualquierMesa(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")

	_, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
		MesoneroID: "usr_x", MesoneroNombre: "Mesonero X",
		RolActor: usuario.RolMesonero, Actor: "usr_x", Origen: origenTst, Comensales: 2,
	})
	if err != nil {
		t.Fatalf("sin asignación debe poder abrir cualquier mesa: %v", err)
	}
}

// Asignar una ZONA cubre sus mesas actuales y las que se agreguen después.
func TestAsignacion_LaZonaCubreSusMesas(t *testing.T) {
	svc, st := servicioSalon(t)
	if _, err := svc.GuardarAsignacion(empSalon, sedeSalon, actorA, origenTst, mesa.Asignacion{
		UsuarioID: "usr_keiber", Nombre: "Keiber", Zonas: []string{"Terraza"},
	}); err != nil {
		t.Fatalf("guardar asignación: %v", err)
	}

	// Una mesa de la Terraza es suya…
	terraza := mesaPorNombre(t, st, "6")
	if terraza.Zona != "Terraza" {
		t.Fatalf("la mesa 6 debería ser de la Terraza, es de %q", terraza.Zona)
	}
	if _, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: terraza.ID,
		MesoneroID: "usr_keiber", MesoneroNombre: "Keiber",
		RolActor: usuario.RolMesonero, Actor: "usr_keiber", Origen: origenTst,
	}); err != nil {
		t.Errorf("su propia zona no debe dar problema: %v", err)
	}

	// …y una mesa nueva que se agregue a la Terraza también, sin reasignar nada.
	nueva, err := svc.CrearMesa(empSalon, sedeSalon, actorA, origenTst, mesa.Mesa{
		SedeID: sedeSalon, Nombre: "T-nueva", Zona: "Terraza", Capacidad: 2,
		Forma: mesa.FormaRedonda, Columna: 2, Fila: 6,
	})
	if err != nil {
		t.Fatalf("crear mesa: %v", err)
	}
	if _, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: nueva.ID,
		MesoneroID: "usr_keiber", MesoneroNombre: "Keiber",
		RolActor: usuario.RolMesonero, Actor: "usr_keiber", Origen: origenTst,
	}); err != nil {
		t.Errorf("una mesa nueva de su zona debe quedar cubierta sin reasignar: %v", err)
	}
}

// Modo FLEXIBLE (el de por defecto): tomar la mesa de otro se permite y queda en la
// bitácora. Es la decisión de producto: un bloqueo duro hace que en un turno movido el
// mesero abandone el sistema.
func TestAsignacion_FlexiblePermiteYRegistra(t *testing.T) {
	svc, st := servicioSalon(t)
	if _, err := svc.GuardarAsignacion(empSalon, sedeSalon, actorA, origenTst, mesa.Asignacion{
		UsuarioID: "usr_keiber", Nombre: "Keiber", Zonas: []string{"Salón"},
	}); err != nil {
		t.Fatalf("guardar asignación: %v", err)
	}
	m := mesaPorNombre(t, st, "2") // libre: el seed ocupa la 1, la 4 y la 5

	// Otro mesonero toma una mesa del Salón (de Keiber).
	if _, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
		MesoneroID: "usr_otro", MesoneroNombre: "Otro",
		RolActor: usuario.RolMesonero, Actor: "usr_otro", Origen: origenTst,
	}); err != nil {
		t.Fatalf("en modo flexible debe poder tomarla: %v", err)
	}

	var registrado bool
	for _, e := range svc.Auditoria(empSalon) {
		if e.Accion == "restaurante.mesa.ajena" {
			registrado = true
			if e.Detalle == "" {
				t.Error("el registro debe decir de quién era la mesa")
			}
		}
	}
	if !registrado {
		t.Error("tomar la mesa de otro tiene que quedar en la bitácora: es lo que hace aceptable no bloquearlo")
	}
}

// Modo ESTRICTO: el servidor rechaza. La UI solo oculta; el candado tiene que estar acá.
func TestAsignacion_EstrictaRechaza(t *testing.T) {
	svc, st := servicioSalon(t)
	if _, err := svc.GuardarConfigSalon(empSalon, sedeSalon, actorA, origenTst, true); err != nil {
		t.Fatalf("activar estricta: %v", err)
	}
	if _, err := svc.GuardarAsignacion(empSalon, sedeSalon, actorA, origenTst, mesa.Asignacion{
		UsuarioID: "usr_keiber", Nombre: "Keiber", Zonas: []string{"Salón"},
	}); err != nil {
		t.Fatalf("guardar asignación: %v", err)
	}
	m := mesaPorNombre(t, st, "2") // libre: el seed ocupa la 1, la 4 y la 5

	_, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
		MesoneroID: "usr_otro", MesoneroNombre: "Otro",
		RolActor: usuario.RolMesonero, Actor: "usr_otro", Origen: origenTst,
	})
	if !errors.Is(err, application.ErrMesaDeOtroMesonero) {
		t.Fatalf("con asignación estricta debe rechazar, se obtuvo: %v", err)
	}

	// Su dueño sí puede.
	if _, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
		MesoneroID: "usr_keiber", MesoneroNombre: "Keiber",
		RolActor: usuario.RolMesonero, Actor: "usr_keiber", Origen: origenTst,
	}); err != nil {
		t.Errorf("el mesonero asignado debe poder abrirla: %v", err)
	}
}

// La asignación limita al MESONERO, no a la caja ni a la dueña: son quienes cubren y
// cobran, así que un candado ahí paralizaría el servicio.
func TestAsignacion_NoLimitaALaCaja(t *testing.T) {
	svc, st := servicioSalon(t)
	if _, err := svc.GuardarConfigSalon(empSalon, sedeSalon, actorA, origenTst, true); err != nil {
		t.Fatalf("activar estricta: %v", err)
	}
	if _, err := svc.GuardarAsignacion(empSalon, sedeSalon, actorA, origenTst, mesa.Asignacion{
		UsuarioID: "usr_keiber", Nombre: "Keiber", Zonas: []string{"Salón"},
	}); err != nil {
		t.Fatalf("guardar asignación: %v", err)
	}
	m := mesaPorNombre(t, st, "2") // libre: el seed ocupa la 1, la 4 y la 5

	for _, rol := range []string{usuario.RolCajero, usuario.RolDueno} {
		if _, err := svc.AbrirCuenta(application.AperturaCuenta{
			EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
			MesoneroID: "usr_caja", MesoneroNombre: "Caja",
			RolActor: rol, Actor: "usr_caja", Origen: origenTst,
		}); err != nil {
			t.Errorf("el rol %s debe poder abrir cualquier mesa: %v", rol, err)
		}
		// Cerrar para dejar la mesa libre al siguiente rol del bucle.
		if c, ok := st.Cuentas.AbiertaDeMesa(empSalon, m.ID); ok {
			if _, err := svc.CerrarCuenta(empSalon, c.ID, actorA, origenTst); err != nil {
				t.Fatalf("cerrar: %v", err)
			}
		}
	}
}

// Guardar una asignación vacía equivale a quitarla: «sin asignar» y «asignado a nada»
// son lo mismo (atiende cualquier mesa) y un registro vacío solo confundiría.
func TestAsignacion_VaciaEquivaleAQuitarla(t *testing.T) {
	svc, _ := servicioSalon(t)
	if _, err := svc.GuardarAsignacion(empSalon, sedeSalon, actorA, origenTst, mesa.Asignacion{
		UsuarioID: "usr_keiber", Nombre: "Keiber", Zonas: []string{"Salón"},
	}); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	// Se cuenta SOLO la de este mesonero: la demo siembra otras y el total no dice nada.
	tiene := func() bool {
		for _, a := range svc.Asignaciones(empSalon, sedeSalon) {
			if a.UsuarioID == "usr_keiber" {
				return true
			}
		}
		return false
	}
	if !tiene() {
		t.Fatal("la asignación recién guardada debería estar")
	}
	if _, err := svc.GuardarAsignacion(empSalon, sedeSalon, actorA, origenTst, mesa.Asignacion{
		UsuarioID: "usr_keiber", Nombre: "Keiber",
	}); err != nil {
		t.Fatalf("guardar vacía: %v", err)
	}
	if tiene() {
		t.Error("una asignación vacía debe quitarse")
	}
}
