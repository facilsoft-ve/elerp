package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
)

/* GUARDAR EL PLANO: posición Y TAMAÑO.
 *
 * La fila 0 del salón demo está libre entre la caja (0,0) y el paso de postres
 * (7,0): es donde se prueba sin chocar con la distribución sembrada.
 *
 * En el editor la mesa se mueve arrastrándola y se agranda arrastrando sus
 * bordes: para quien dibuja el salón es el mismo gesto. Si el tamaño viajara por
 * otra ruta, un plano se guardaría a medias cuando una de las dos llamadas
 * falla, y quedaría una mesa movida con el tamaño viejo encima de su vecina. */

func TestGuardarMapa_GuardaElTamano(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")

	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: m.ID, Columna: 3, Fila: 0, AnchoCeldas: 2, AltoCeldas: 2},
	}); err != nil {
		t.Fatalf("guardar mapa: %v", err)
	}
	got := mesaPorNombre(t, st, "2")
	if got.AnchoCeldas != 2 || got.AltoCeldas != 2 {
		t.Fatalf("tamaño = %d×%d, se esperaba 2×2", got.AnchoCeldas, got.AltoCeldas)
	}
}

// Un cliente que solo manda posiciones (o el guardado de un plano sin
// redimensionar) NO debe achicar las mesas.
func TestGuardarMapa_SinTamanoNoLoToca(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")
	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: m.ID, Columna: 1, Fila: 0, AnchoCeldas: 3, AltoCeldas: 1},
	}); err != nil {
		t.Fatalf("guardar tamaño: %v", err)
	}
	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: m.ID, Columna: 3, Fila: 0},
	}); err != nil {
		t.Fatalf("guardar solo posición: %v", err)
	}
	got := mesaPorNombre(t, st, "2")
	if got.AnchoCeldas != 3 || got.AltoCeldas != 1 {
		t.Fatalf("el tamaño se perdió al guardar solo la posición: %d×%d", got.AnchoCeldas, got.AltoCeldas)
	}
}

// Al ACHICAR, el aforo se recorta al nuevo tope. Rechazar el guardado sería
// peor: el plano ya se dibujó y quien lo hizo tendría que adivinar cuál de
// veinte mesas quedó con un número que ya no entra.
func TestGuardarMapa_AlAchicarRecortaElAforo(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")
	grande := m
	grande.Capacidad, grande.AnchoCeldas, grande.AltoCeldas = 12, 3, 1
	if _, err := svc.ActualizarMesa(empSalon, m.ID, actorA, origenTst, grande, nil); err != nil {
		t.Fatalf("preparar mesa de 12: %v", err)
	}

	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: m.ID, Columna: 1, Fila: 0, AnchoCeldas: 1, AltoCeldas: 1},
	}); err != nil {
		t.Fatalf("achicar: %v", err)
	}
	got := mesaPorNombre(t, st, "2")
	if got.Capacidad != 4 {
		t.Fatalf("aforo = %d, un cuadro admite 4", got.Capacidad)
	}
}

// El tope de tamaño se sigue haciendo cumplir en el servidor: la pantalla puede
// mentir, el plano no.
func TestGuardarMapa_RechazaTamanoImposible(t *testing.T) {
	svc, st := servicioSalon(t)
	m := mesaPorNombre(t, st, "2")
	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: m.ID, Columna: 1, Fila: 0, AnchoCeldas: 99, AltoCeldas: 1},
	}); err == nil {
		t.Fatal("una mesa de 99 cuadros de lado debería rechazarse")
	}
}

// Y el solape se sigue mirando sobre TODA la superficie: agrandar una mesa
// encima de su vecina tiene que fallar, no pisarla.
func TestGuardarMapa_AgrandarSobreOtraFalla(t *testing.T) {
	svc, st := servicioSalon(t)
	a := mesaPorNombre(t, st, "1")
	b := mesaPorNombre(t, st, "2")

	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: a.ID, Columna: 1, Fila: 0, AnchoCeldas: 1, AltoCeldas: 1},
		{ID: b.ID, Columna: 2, Fila: 0, AnchoCeldas: 1, AltoCeldas: 1},
	}); err != nil {
		t.Fatalf("colocar las dos: %v", err)
	}
	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: a.ID, Columna: 1, Fila: 0, AnchoCeldas: 3, AltoCeldas: 1},
	}); err == nil {
		t.Fatal("agrandar una mesa sobre su vecina debería rechazarse")
	}
}
