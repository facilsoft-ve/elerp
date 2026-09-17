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
