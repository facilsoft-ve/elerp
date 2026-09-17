package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/sede"
	"github.com/mornix/elerp/internal/domain/usuario"
)

/* PRESENCIA ESTRICTA.
 *
 * Lo que se prueba acá no es la trigonometría: es que la regla NO APLIQUE
 * cuando no debe. Un candado que se activa por una configuración a medias deja
 * al local sin poder trabajar, y eso es peor que no tener candado. */

// Coordenadas de referencia: la Plaza Bolívar de Caracas. A esta latitud un
// grado de longitud ≈ 111 km, así que 0.001° ≈ 111 m — suficiente para poner a
// alguien justo afuera de un radio de 100 m.
const (
	latSede = 10.5061
	lonSede = -66.9146
)

// servicioPresencia arma el servicio con las sedes cableadas.
func servicioPresencia(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConSedes(st.Sedes)
	return svc, st
}

// sedeUbicada le pone coordenadas a la sede demo y marca el rol como estricto.
func sedeUbicada(t *testing.T, svc *application.Service, radioM int, roles ...string) {
	t.Helper()
	if _, err := svc.FijarUbicacionSede(empDemo, sede1, actorA, origenTst, latSede, lonSede, radioM); err != nil {
		t.Fatalf("fijar ubicación: %v", err)
	}
	if _, err := svc.FijarRolesPresencia(empDemo, actorA, origenTst, roles); err != nil {
		t.Fatalf("fijar roles: %v", err)
	}
}

/* --- Cuándo NO aplica (lo que más importa) -------------------------------- */

// Sin roles marcados, la verificación no existe: es el comportamiento de
// siempre y ninguna empresa ya creada puede quedar trancada por esto.
func TestPresencia_SinRolesEstrictosNoAplica(t *testing.T) {
	svc, _ := servicioPresencia(t)
	if _, err := svc.FijarUbicacionSede(empDemo, sede1, actorA, origenTst, latSede, lonSede, 100); err != nil {
		t.Fatalf("fijar ubicación: %v", err)
	}
	// Ni siquiera manda posición: da igual, el rol no es estricto.
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, nil); err != nil {
		t.Fatalf("sin roles estrictos no debe verificar nada: %v", err)
	}
}

// Un rol que no está en la lista pasa aunque otro sí lo esté.
func TestPresencia_OtroRolNoSeVeAfectado(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 100, usuario.RolMesonero)
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolDueno, nil); err != nil {
		t.Fatalf("la dueña no tiene presencia estricta: %v", err)
	}
}

// Exigir presencia contra una sede SIN coordenadas dejaría el local sin poder
// trabajar por una configuración a medias. Se prefiere no verificar.
func TestPresencia_SedeSinCoordenadasNoAplica(t *testing.T) {
	svc, _ := servicioPresencia(t)
	if _, err := svc.FijarRolesPresencia(empDemo, actorA, origenTst, []string{usuario.RolMesonero}); err != nil {
		t.Fatalf("fijar roles: %v", err)
	}
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, nil); err != nil {
		t.Fatalf("sin coordenadas configuradas no debe verificar: %v", err)
	}
}

// Sin el repositorio cableado tampoco aplica: una instancia que no usa esto
// sigue funcionando exactamente igual.
func TestPresencia_SinSedesCableadasNoAplica(t *testing.T) {
	svc, _ := nuevoServicio(t) // sin ConSedes
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, nil); err != nil {
		t.Fatalf("sin sedes cableadas no debe verificar: %v", err)
	}
}

/* --- Cuándo sí aplica ------------------------------------------------------ */

func TestPresencia_DentroDelRadioPasa(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 150, usuario.RolMesonero)
	// A unos 11 m de la sede: dentro de cualquier radio razonable.
	pos := &application.Posicion{Lat: latSede + 0.0001, Lon: lonSede, PrecisionM: 10}
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, pos); err != nil {
		t.Fatalf("estando en la sede debe pasar: %v", err)
	}
}

// El caso que motiva todo esto: el mesonero en su casa, a kilómetros del local.
func TestPresencia_FueraDelRadioSeRechaza(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 150, usuario.RolMesonero)
	// ~5,5 km al norte.
	pos := &application.Posicion{Lat: latSede + 0.05, Lon: lonSede, PrecisionM: 10}
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, pos); !errors.Is(err, application.ErrFueraDeSede) {
		t.Fatalf("se esperaba ErrFueraDeSede, se obtuvo: %v", err)
	}
}

// Sin ubicación NO es lo mismo que estar lejos: es el error que la pantalla
// traduce en «pídele la excepción al supervisor».
func TestPresencia_SinUbicacionEsErrorDistinguible(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 150, usuario.RolMesonero)
	err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, nil)
	if !errors.Is(err, application.ErrPresenciaSinUbicacion) {
		t.Fatalf("se esperaba ErrPresenciaSinUbicacion, se obtuvo: %v", err)
	}
	if errors.Is(err, application.ErrFueraDeSede) {
		t.Error("«sin ubicación» y «fuera de la sede» no pueden confundirse: se resuelven distinto")
	}
}

// Una coordenada imposible es un dato roto, no «lejos»: sale por la vía de la
// excepción, porque la persona no lo puede resolver moviéndose.
func TestPresencia_CoordenadaImposibleSeTrataComoSinUbicacion(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 150, usuario.RolMesonero)
	pos := &application.Posicion{Lat: 999, Lon: 999, PrecisionM: 5}
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, pos); !errors.Is(err, application.ErrPresenciaSinUbicacion) {
		t.Fatalf("se esperaba ErrPresenciaSinUbicacion, se obtuvo: %v", err)
	}
}

/* --- La precisión juega a favor de quien consulta -------------------------- */

// Quien está en el borde con un GPS malo (bajo techo, en la cocina) NO puede
// quedar afuera: si no, llamaría al supervisor todas las noches.
func TestPresencia_LaPrecisionSalvaAQuienEstaEnElBorde(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 100, usuario.RolMesonero)
	// ~111 m: fuera de un radio de 100 m si se mira la cifra pelada.
	lejos := application.Posicion{Lat: latSede + 0.001, Lon: lonSede, PrecisionM: 5}
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, &lejos); !errors.Is(err, application.ErrFueraDeSede) {
		t.Fatalf("con buena precisión, 111 m queda fuera de 100 m: %v", err)
	}
	// Mismo punto, pero el navegador declara ±80 m de incertidumbre: entra.
	conRuido := lejos
	conRuido.PrecisionM = 80
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, &conRuido); err != nil {
		t.Fatalf("la incertidumbre del GPS debe jugar a favor: %v", err)
	}
}

// El margen tiene TOPE: sin él, declarar una precisión absurda haría que
// cualquier punto del país cayera «dentro» y la verificación no serviría.
func TestPresencia_UnaPrecisionAbsurdaNoAbreLaPuerta(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 150, usuario.RolMesonero)
	// A ~5,5 km, diciendo «±100 km de precisión».
	pos := &application.Posicion{Lat: latSede + 0.05, Lon: lonSede, PrecisionM: 100000}
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, pos); !errors.Is(err, application.ErrFueraDeSede) {
		t.Fatalf("una precisión declarada absurda no puede saltarse el radio: %v", err)
	}
}

/* --- Configuración --------------------------------------------------------- */

func TestFijarUbicacionSede_RechazaCoordenadasYRadiosImposibles(t *testing.T) {
	svc, _ := servicioPresencia(t)
	if _, err := svc.FijarUbicacionSede(empDemo, sede1, actorA, origenTst, 91, 0, 0); !errors.Is(err, application.ErrCoordenadaInvalida) {
		t.Errorf("latitud 91 debe rechazarse: %v", err)
	}
	if _, err := svc.FijarUbicacionSede(empDemo, sede1, actorA, origenTst, latSede, lonSede, 5); !errors.Is(err, application.ErrRadioInvalido) {
		t.Errorf("un radio de 5 m debe rechazarse: ni el mejor GPS lo distingue (%v)", err)
	}
	if _, err := svc.FijarUbicacionSede(empDemo, sede1, actorA, origenTst, latSede, lonSede, 99999); !errors.Is(err, application.ErrRadioInvalido) {
		t.Errorf("un radio de medio país debe rechazarse: %v", err)
	}
}

// Radio 0 = usar el default. 150 m y no 20 porque el GPS bajo techo es malo.
func TestFijarUbicacionSede_RadioCeroUsaElDefault(t *testing.T) {
	svc, st := servicioPresencia(t)
	out, err := svc.FijarUbicacionSede(empDemo, sede1, actorA, origenTst, latSede, lonSede, 0)
	if err != nil {
		t.Fatalf("fijar: %v", err)
	}
	if out.RadioEfectivo() != sede.RadioPorDefecto {
		t.Errorf("radio efectivo = %d, se esperaba el default %d", out.RadioEfectivo(), sede.RadioPorDefecto)
	}
	guardada, _ := st.Sedes.ByID(empDemo, sede1)
	if !guardada.TieneUbicacion() {
		t.Error("la ubicación no quedó guardada")
	}
}

// Poner lat/lon en cero BORRA la ubicación: es la forma de apagar la
// verificación de una sede sin vaciar los roles de toda la empresa.
func TestFijarUbicacionSede_CeroBorraLaUbicacion(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 100, usuario.RolMesonero)
	if _, err := svc.FijarUbicacionSede(empDemo, sede1, actorA, origenTst, 0, 0, 0); err != nil {
		t.Fatalf("borrar: %v", err)
	}
	// Ya no verifica, aunque el rol siga siendo estricto.
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, nil); err != nil {
		t.Fatalf("sin ubicación no debe verificar: %v", err)
	}
}

func TestFijarRolesPresencia_NormalizaYDeduplica(t *testing.T) {
	svc, _ := servicioPresencia(t)
	out, err := svc.FijarRolesPresencia(empDemo, actorA, origenTst,
		[]string{"  MESONERO ", "mesonero", "", "cajero"})
	if err != nil {
		t.Fatalf("fijar roles: %v", err)
	}
	if len(out.RolesPresenciaEstricta) != 2 {
		t.Fatalf("se esperaban 2 roles tras normalizar, hay %d: %v", len(out.RolesPresenciaEstricta), out.RolesPresenciaEstricta)
	}
	if out.RolesPresenciaEstricta[0] != "mesonero" || out.RolesPresenciaEstricta[1] != "cajero" {
		t.Errorf("normalización incorrecta: %v", out.RolesPresenciaEstricta)
	}
}

// La lista vacía apaga la regla: tiene que poder deshacerse.
func TestFijarRolesPresencia_VaciaApagaLaRegla(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 100, usuario.RolMesonero)
	if _, err := svc.FijarRolesPresencia(empDemo, actorA, origenTst, nil); err != nil {
		t.Fatalf("vaciar roles: %v", err)
	}
	if err := svc.VerificarPresencia(empDemo, sede1, usuario.RolMesonero, nil); err != nil {
		t.Fatalf("sin roles la regla se apaga: %v", err)
	}
}

/* --- Aislamiento por empresa ----------------------------------------------- */

// La configuración de una empresa no puede regir en otra, ni su sede ser
// alcanzable desde afuera.
func TestPresencia_AisladaPorEmpresa(t *testing.T) {
	svc, _ := servicioPresencia(t)
	sedeUbicada(t, svc, 100, usuario.RolMesonero)

	// Otra empresa: ni ve la sede ni hereda los roles estrictos.
	if err := svc.VerificarPresencia("emp_demo_rest", sede1, usuario.RolMesonero, nil); err != nil {
		t.Errorf("la config de una empresa no rige en otra: %v", err)
	}
	if _, err := svc.FijarUbicacionSede("emp_demo_rest", sede1, actorA, origenTst, latSede, lonSede, 100); !errors.Is(err, application.ErrSedeNoExiste) {
		t.Errorf("no se puede ubicar la sede de otra empresa: %v", err)
	}
}
