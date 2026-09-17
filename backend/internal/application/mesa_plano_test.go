package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/mesa"
)

/* ÁREAS Y MOSTRADORES DEL PLANO.
 *
 * La diferencia que manda: un MOSTRADOR ocupa piso (es un mueble real, ahí no
 * entra una mesa) y un ÁREA no (es una etiqueta de superficie, y las mesas viven
 * adentro). Confundirlas haría que dibujar la terraza expulsara sus mesas. */

// planoBase usa una grilla más grande que la del salón demo: así la esquina
// (9,7) está libre de mesas y sirve para probar el mobiliario sin chocar con la
// distribución sembrada.
func planoBase(areas []mesa.Area, mostradores []mesa.Mostrador) application.PlanoEntrada {
	return application.PlanoEntrada{Filas: 8, Columnas: 10, Areas: areas, Mostradores: mostradores}
}

// libre es una esquina del plano sin mesas del demo.
const (
	libreCol = 9
	libreFil = 7
)

func TestPlano_GuardaAreasYMostradores(t *testing.T) {
	svc, _ := servicioSalon(t)
	out, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{{Nombre: "Terraza", Columna: 0, Fila: 0, Ancho: 3, Alto: 2}},
		[]mesa.Mostrador{{Nombre: "Barra", Tipo: mesa.MostradorBarra, Columna: libreCol, Fila: libreFil, Ancho: 1, Alto: 1}},
	))
	if err != nil {
		t.Fatalf("guardar plano: %v", err)
	}
	if len(out.Areas) != 1 || len(out.Mostradores) != 1 {
		t.Fatalf("se guardaron %d áreas y %d mostradores", len(out.Areas), len(out.Mostradores))
	}
	// El id se deriva de la esquina cuando el cliente no manda uno: dos áreas no
	// pueden encimarse, así que la esquina las identifica sin ambigüedad.
	if out.Areas[0].ID == "" || out.Mostradores[0].ID == "" {
		t.Fatal("un área o un mostrador sin id no se puede seleccionar ni editar")
	}
}

// Dos áreas encimadas harían ambigua la zona de las mesas de esa franja.
func TestPlano_RechazaAreasEncimadas(t *testing.T) {
	svc, _ := servicioSalon(t)
	_, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{
			{Nombre: "Terraza", Columna: 0, Fila: 0, Ancho: 3, Alto: 2},
			{Nombre: "Pórtico", Columna: 2, Fila: 1, Ancho: 3, Alto: 2},
		}, nil))
	if err == nil {
		t.Fatal("dos áreas encimadas deberían rechazarse")
	}
}

// Un área SÍ se superpone a las mesas: para eso está, las contiene.
func TestPlano_AreaSobreMesasEsValida(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")
	_, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{{Nombre: "Salón principal", Columna: m.Columna, Fila: m.Fila, Ancho: 2, Alto: 2}}, nil))
	if err != nil {
		t.Fatalf("un área sobre una mesa es lo normal: %v", err)
	}
}

// El mostrador es un mueble: ahí no cabe una mesa.
func TestPlano_MostradorNoPuedePisarUnaMesa(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")
	_, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(nil,
		[]mesa.Mostrador{{Nombre: "Barra", Tipo: mesa.MostradorBarra, Columna: m.Columna, Fila: m.Fila, Ancho: 1, Alto: 1}}))
	if err == nil {
		t.Fatal("un mostrador encima de una mesa debería rechazarse")
	}
}

// Y al revés: una vez puesta la barra, no se puede arrastrar una mesa encima.
func TestGuardarMapa_MesaNoPuedePisarUnMostrador(t *testing.T) {
	svc, st := servicioSalon(t)
	if _, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(nil,
		[]mesa.Mostrador{{Nombre: "Barra", Tipo: mesa.MostradorBarra, Columna: libreCol, Fila: libreFil, Ancho: 1, Alto: 1}})); err != nil {
		t.Fatalf("poner la barra: %v", err)
	}
	m := mesaPorNombre(t, st, "2")
	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: m.ID, Columna: libreCol, Fila: libreFil, AnchoCeldas: 1, AltoCeldas: 1},
	}); err == nil {
		t.Fatal("arrastrar una mesa sobre la barra debería rechazarse")
	}
}

func TestPlano_RechazaTipoDeMostradorInventado(t *testing.T) {
	svc, _ := servicioSalon(t)
	if _, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(nil,
		[]mesa.Mostrador{{Nombre: "Algo", Tipo: "mueble_raro", Columna: libreCol, Fila: libreFil, Ancho: 1, Alto: 1}})); err == nil {
		t.Fatal("un tipo fuera del catálogo debería rechazarse")
	}
	if _, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{{Nombre: "  ", Columna: 0, Fila: 0, Ancho: 2, Alto: 2}}, nil)); err == nil {
		t.Fatal("un área sin nombre debería rechazarse: su único propósito es nombrar una parte del salón")
	}
}

// LA prueba que justifica las áreas: la zona de la mesa se DEDUCE de dónde está.
// Escrita a mano en cada ficha terminaba con «Terraza», «terraza» y «Terrraza»
// conviviendo, y cualquier agrupación por zona salía mal.
func TestPlano_LaMesaTomaLaZonaDeSuArea(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")
	if _, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{{Nombre: "Terraza", Columna: m.Columna, Fila: m.Fila, Ancho: 1, Alto: 1}}, nil)); err != nil {
		t.Fatalf("guardar plano: %v", err)
	}
	if got := mesaPorNombre(t, st, "2"); got.Zona != "Terraza" {
		t.Fatalf("zona = %q, se esperaba «Terraza»", got.Zona)
	}
}

// Fuera de toda área, la zona escrita a mano se respeta: dibujar la terraza no
// puede borrar la zona de las mesas del resto del salón.
func TestPlano_FueraDeAreaLaZonaNoSeToca(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")
	antes := m.Zona
	if _, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{{Nombre: "Terraza", Columna: libreCol, Fila: libreFil, Ancho: 1, Alto: 1}}, nil)); err != nil {
		t.Fatalf("guardar plano: %v", err)
	}
	if got := mesaPorNombre(t, st, "2"); got.Zona != antes {
		t.Fatalf("la zona cambió sin que la mesa esté en el área: %q → %q", antes, got.Zona)
	}
}

// La ruta vieja solo sabe de grilla: no puede borrar la barra y las zonas del
// local con solo cambiar el número de filas.
func TestPlano_LaRutaViejaConservaAreasYMostradores(t *testing.T) {
	svc, _ := servicioSalon(t)
	if _, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{{Nombre: "Terraza", Columna: 0, Fila: 0, Ancho: 2, Alto: 2}},
		[]mesa.Mostrador{{Nombre: "Barra", Tipo: mesa.MostradorBarra, Columna: libreCol, Fila: libreFil, Ancho: 1, Alto: 1}},
	)); err != nil {
		t.Fatalf("guardar plano: %v", err)
	}
	out, err := svc.GuardarPlanoSalon(empSalon, sedeSalon, actorA, origenTst, 8, 10, nil)
	if err != nil {
		t.Fatalf("guardar solo grilla: %v", err)
	}
	if len(out.Areas) != 1 || len(out.Mostradores) != 1 {
		t.Fatalf("la ruta vieja borró el mobiliario: %d áreas, %d mostradores", len(out.Areas), len(out.Mostradores))
	}
}

// Achicar la grilla no debería impedir guardar: el área se recorta a lo que
// quedó, o se descarta si quedó fuera del todo.
func TestPlano_AchicarLaGrillaRecortaLasAreas(t *testing.T) {
	svc, _ := servicioSalon(t)
	out, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, application.PlanoEntrada{
		Filas: 3, Columnas: 3,
		Areas: []mesa.Area{
			{Nombre: "Terraza", Columna: 0, Fila: 0, Ancho: 8, Alto: 8},
			{Nombre: "Afuera", Columna: 7, Fila: 7, Ancho: 2, Alto: 2},
		},
	})
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if len(out.Areas) != 1 {
		t.Fatalf("áreas = %d, la que quedó fuera debía descartarse", len(out.Areas))
	}
	if out.Areas[0].Ancho != 3 || out.Areas[0].Alto != 3 {
		t.Fatalf("el área no se recortó a la grilla: %d×%d", out.Areas[0].Ancho, out.Areas[0].Alto)
	}
}

/* ÁREAS DE FORMA LIBRE.
 *
 * Un local real tiene terrazas en L y salones con un recorte donde está la
 * escalera. Obligar a que cada zona fuera un rectángulo forzaba a partir la
 * terraza en «Terraza 1» y «Terraza 2», y con eso se pierde justo lo que el área
 * existe para dar: UN nombre de zona para las mesas que están ahí. */

// celdasL es una zona en L: tres celdas en fila y una colgando de la primera.
func celdasL(c0, f0 int) []mesa.Celda {
	return []mesa.Celda{
		{Columna: c0, Fila: f0}, {Columna: c0 + 1, Fila: f0}, {Columna: c0 + 2, Fila: f0},
		{Columna: c0, Fila: f0 + 1},
	}
}

func TestArea_GuardaUnaFormaEnL(t *testing.T) {
	svc, _ := servicioSalon(t)
	out, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{{Nombre: "Terraza", Celdas: celdasL(0, 6)}}, nil))
	if err != nil {
		t.Fatalf("guardar área en L: %v", err)
	}
	a := out.Areas[0]
	if len(a.Celdas) != 4 {
		t.Fatalf("celdas = %d, se esperaban 4", len(a.Celdas))
	}
	// La caja se DERIVA y es solo dónde va el rótulo, no la forma.
	if a.Columna != 0 || a.Fila != 6 || a.Ancho != 3 || a.Alto != 2 {
		t.Fatalf("caja = (%d,%d) %dx%d, se esperaba (0,6) 3x2", a.Columna, a.Fila, a.Ancho, a.Alto)
	}
	// Y el hueco de la L NO pertenece al área.
	if !a.Cubre(0, 7) {
		t.Fatal("el brazo de la L debería pertenecer al área")
	}
	if a.Cubre(2, 7) {
		t.Fatal("el hueco de la L no pertenece al área, aunque esté dentro de su caja")
	}
}

// Dos zonas en L encajan una en el hueco de la otra: compararlas por su CAJA las
// rechazaría sin motivo.
func TestArea_DosFormasEncajanSinSolaparse(t *testing.T) {
	svc, _ := servicioSalon(t)
	_, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{
			{Nombre: "Terraza", Celdas: celdasL(0, 6)},
			{Nombre: "Pórtico", Celdas: []mesa.Celda{{Columna: 1, Fila: 7}, {Columna: 2, Fila: 7}}},
		}, nil))
	if err != nil {
		t.Fatalf("dos zonas que encajan no deberían rechazarse: %v", err)
	}
}

// Pero compartir UNA celda sigue siendo solape.
func TestArea_UnaCeldaCompartidaSigueSiendoSolape(t *testing.T) {
	svc, _ := servicioSalon(t)
	_, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{
			{Nombre: "Terraza", Celdas: celdasL(0, 6)},
			{Nombre: "Pórtico", Celdas: []mesa.Celda{{Columna: 0, Fila: 7}}},
		}, nil))
	if err == nil {
		t.Fatal("dos zonas que comparten una celda deberían rechazarse")
	}
}

// Un área guardada antes del dibujo libre (solo rectángulo) se sigue leyendo, y
// al guardarla queda expandida en celdas: no hay nada que migrar.
func TestArea_LaFormaViejaSeLeeYSeExpande(t *testing.T) {
	svc, _ := servicioSalon(t)
	out, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{{Nombre: "Salón", Columna: 0, Fila: 6, Ancho: 2, Alto: 2}}, nil))
	if err != nil {
		t.Fatalf("guardar área rectangular: %v", err)
	}
	if len(out.Areas[0].Celdas) != 4 {
		t.Fatalf("el rectángulo debía expandirse a 4 celdas, son %d", len(out.Areas[0].Celdas))
	}
}

// La mesa toma la zona por CELDA: estar en la caja de la L no alcanza.
func TestArea_LaZonaSeResuelvePorCelda(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")
	antes := m.Zona
	// El hueco de una L colocada sobre la mesa: no debería adoptarla.
	hueco := []mesa.Celda{
		{Columna: m.Columna, Fila: m.Fila + 1},
		{Columna: m.Columna + 1, Fila: m.Fila + 1},
	}
	if _, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst, planoBase(
		[]mesa.Area{{Nombre: "Terraza", Celdas: hueco}}, nil)); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if got := mesaPorNombre(t, st, "2"); got.Zona != antes {
		t.Fatalf("la mesa tomó una zona que no la contiene: %q → %q", antes, got.Zona)
	}
}
