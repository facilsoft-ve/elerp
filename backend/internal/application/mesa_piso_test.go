package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/mesa"
)

/* PISOS DEL LOCAL.
 *
 * Un restaurante de dos plantas no es un plano más grande: es dos planos. Lo que
 * se prueba acá es lo que se rompería si se tratara como uno solo —la terraza de
 * arriba pisando el salón de abajo— y, sobre todo, que un salón dibujado ANTES
 * de que existieran los pisos siga abriendo sin migrar nada.
 */

func dosPisos() []mesa.Piso {
	return []mesa.Piso{
		{ID: "piso_1", Nombre: "Planta baja", Filas: 8, Columnas: 10},
		{ID: "piso_2", Nombre: "Mezzanina", Filas: 6, Columnas: 6},
	}
}

// Un plano guardado antes de los pisos ES la planta baja. Sin esto, todo salón
// ya dibujado se quedaría sin mapa de un día para otro.
func TestPiso_ElPlanoViejoEsLaPlantaBaja(t *testing.T) {
	p := mesa.Plano{
		Filas: 6, Columnas: 8,
		Areas:       []mesa.Area{{ID: "a1", Nombre: "Terraza", Columna: 0, Fila: 0, Ancho: 2, Alto: 2}},
		Mostradores: []mesa.Mostrador{{ID: "m1", Nombre: "Barra", Tipo: mesa.MostradorBarra, Columna: 5, Fila: 5, Ancho: 1, Alto: 1}},
	}
	pisos := p.PisosEfectivos()
	if len(pisos) != 1 {
		t.Fatalf("un plano sin pisos tiene exactamente uno: %d", len(pisos))
	}
	pi := pisos[0]
	if pi.ID != mesa.PisoPrincipal || pi.Filas != 6 || pi.Columnas != 8 {
		t.Fatalf("la planta baja debe heredar la grilla del plano viejo: %+v", pi)
	}
	if len(pi.Areas) != 1 || len(pi.Mostradores) != 1 {
		t.Fatal("la planta baja se queda con las áreas y muebles que ya estaban dibujados")
	}
	// Y una mesa sin piso pertenece a esa planta: es donde estaba.
	if !pi.EsDelPiso(mesa.Mesa{Nombre: "1"}) {
		t.Fatal("una mesa anterior a los pisos tiene que seguir apareciendo en la planta baja")
	}
}

// Dos áreas en la misma coordenada de plantas distintas NO se estorban: una está
// encima de la otra. Rechazarlas obligaría a dibujar cada piso corrido a un
// lado, que es el mapa ilegible que los pisos vienen a evitar.
func TestPiso_LasAreasDePisosDistintosNoSeEstorban(t *testing.T) {
	svc, _ := servicioSalon(t)
	pisos := dosPisos()
	misma := []mesa.Celda{{Columna: 0, Fila: 0}, {Columna: 1, Fila: 0}}
	pisos[0].Areas = []mesa.Area{{ID: "a1", Nombre: "Salón", Celdas: misma}}
	pisos[1].Areas = []mesa.Area{{ID: "a2", Nombre: "Mezzanina", Celdas: misma}}

	out, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst,
		application.PlanoEntrada{Filas: 8, Columnas: 10, Pisos: pisos})
	if err != nil {
		t.Fatalf("dos pisos pueden usar las mismas coordenadas: %v", err)
	}
	if len(out.Pisos) != 2 {
		t.Fatalf("se guardaron %d pisos", len(out.Pisos))
	}
}

// Dentro de UN piso la regla sigue en pie: dos áreas encimadas hacen ambigua la
// zona de las mesas de esa franja.
func TestPiso_DentroDeUnPisoLasAreasSiguenSinPoderEncimarse(t *testing.T) {
	svc, _ := servicioSalon(t)
	pisos := dosPisos()
	pisos[1].Areas = []mesa.Area{
		{ID: "a1", Nombre: "Mezzanina", Celdas: []mesa.Celda{{Columna: 0, Fila: 0}}},
		{ID: "a2", Nombre: "Lounge", Celdas: []mesa.Celda{{Columna: 0, Fila: 0}}},
	}
	_, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst,
		application.PlanoEntrada{Filas: 8, Columnas: 10, Pisos: pisos})
	if err == nil {
		t.Fatal("dos áreas encimadas en la misma planta tenían que rechazarse")
	}
}

// Cada planta se recorta a SU grilla. Una mezzanina de 6×6 no puede tener un
// área en la columna 9 porque la planta baja sí llega hasta allá.
func TestPiso_CadaPlantaSeRecortaASuPropiaGrilla(t *testing.T) {
	svc, _ := servicioSalon(t)
	pisos := dosPisos()
	pisos[1].Areas = []mesa.Area{{ID: "a1", Nombre: "Mezzanina", Celdas: []mesa.Celda{
		{Columna: 0, Fila: 0}, {Columna: 9, Fila: 0}, // la 9 no existe en 6 columnas
	}}}
	out, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst,
		application.PlanoEntrada{Filas: 8, Columnas: 10, Pisos: pisos})
	if err != nil {
		t.Fatalf("recortar no debería impedir guardar: %v", err)
	}
	celdas := out.Pisos[1].Areas[0].Celdas
	if len(celdas) != 1 || celdas[0].Columna != 0 {
		t.Fatalf("la celda fuera de la mezzanina tenía que recortarse: %+v", celdas)
	}
}

// La zona de una mesa sale del área de SU planta. Buscarla en todo el plano le
// pondría a una mesa de arriba el nombre de la terraza de abajo, que ocupa las
// mismas coordenadas y no es el mismo sitio.
func TestPiso_LaZonaSaleDelAreaDeSuPropiaPlanta(t *testing.T) {
	svc, st := servicioSalon(t)
	pisos := dosPisos()
	celda := []mesa.Celda{{Columna: 7, Fila: 6}}
	pisos[0].Areas = []mesa.Area{{ID: "a1", Nombre: "Terraza", Celdas: celda}}
	pisos[1].Areas = []mesa.Area{{ID: "a2", Nombre: "Mezzanina", Celdas: []mesa.Celda{{Columna: 2, Fila: 2}}}}

	abajo, err := svc.CrearMesa(empSalon, sedeSalon, actorA, origenTst,
		mesa.Mesa{Nombre: "T1", Capacidad: 4, Forma: mesa.FormaCuadrada, Columna: 7, Fila: 6})
	if err != nil {
		t.Fatalf("crear abajo: %v", err)
	}
	arriba, err := svc.CrearMesa(empSalon, sedeSalon, actorA, origenTst,
		mesa.Mesa{Nombre: "M1", Capacidad: 4, Forma: mesa.FormaCuadrada, Columna: 2, Fila: 2, PisoID: "piso_2"})
	if err != nil {
		t.Fatalf("crear arriba: %v", err)
	}
	if _, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst,
		application.PlanoEntrada{Filas: 8, Columnas: 10, Pisos: pisos}); err != nil {
		t.Fatalf("guardar: %v", err)
	}

	a, _ := st.Mesas.ByID(empSalon, abajo.ID)
	b, _ := st.Mesas.ByID(empSalon, arriba.ID)
	if a.Zona != "Terraza" {
		t.Fatalf("la mesa de abajo debería quedar en Terraza, quedó en %q", a.Zona)
	}
	if b.Zona != "Mezzanina" {
		t.Fatalf("la mesa de arriba debería quedar en Mezzanina, quedó en %q", b.Zona)
	}
}

// Dos mesas en la misma celda de plantas distintas no se estorban. Es la misma
// regla que las áreas, y la que permite que cada planta se dibuje desde su
// esquina en vez de correrse para no chocar con la de abajo.
func TestPiso_DosMesasEnPlantasDistintasPuedenCompartirCelda(t *testing.T) {
	svc, _ := servicioSalon(t)
	abajo, err := svc.CrearMesa(empSalon, sedeSalon, actorA, origenTst,
		mesa.Mesa{Nombre: "P1", Capacidad: 4, Forma: mesa.FormaCuadrada, Columna: 9, Fila: 7})
	if err != nil {
		t.Fatalf("crear abajo: %v", err)
	}
	arriba, err := svc.CrearMesa(empSalon, sedeSalon, actorA, origenTst,
		mesa.Mesa{Nombre: "P2", Capacidad: 4, Forma: mesa.FormaCuadrada, Columna: 9, Fila: 7, PisoID: "piso_2"})
	if err != nil {
		t.Fatalf("crear arriba: %v", err)
	}
	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: abajo.ID, Columna: 9, Fila: 7}, {ID: arriba.ID, Columna: 9, Fila: 7},
	}); err != nil {
		t.Fatalf("dos plantas pueden usar la misma celda: %v", err)
	}
}

// Y dentro de la misma planta siguen sin poder encimarse.
func TestPiso_DentroDeUnaPlantaLasMesasSiguenSinPoderEncimarse(t *testing.T) {
	svc, _ := servicioSalon(t)
	a, err := svc.CrearMesa(empSalon, sedeSalon, actorA, origenTst,
		mesa.Mesa{Nombre: "Q1", Capacidad: 4, Forma: mesa.FormaCuadrada, Columna: 9, Fila: 7, PisoID: "piso_2"})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	b, err := svc.CrearMesa(empSalon, sedeSalon, actorA, origenTst,
		mesa.Mesa{Nombre: "Q2", Capacidad: 4, Forma: mesa.FormaCuadrada, Columna: 8, Fila: 7, PisoID: "piso_2"})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: a.ID, Columna: 9, Fila: 7}, {ID: b.ID, Columna: 9, Fila: 7},
	}); err == nil {
		t.Fatal("dos mesas de la misma planta en la misma celda tenían que rechazarse")
	}
}

// Mover una mesa de planta le cambia la zona. Sin esto se queda con el nombre de
// donde estaba, y la zona es justo lo que el personal usa para nombrarla en voz
// alta: «la cuatro de la terraza» apuntando a una mesa que ya no está ahí.
func TestPiso_MoverUnaMesaDePlantaLeCambiaLaZona(t *testing.T) {
	svc, st := servicioSalon(t)
	pisos := dosPisos()
	pisos[0].Areas = []mesa.Area{{ID: "a1", Nombre: "Terraza", Celdas: []mesa.Celda{{Columna: 9, Fila: 7}}}}
	pisos[1].Areas = []mesa.Area{{ID: "a2", Nombre: "Mezzanina", Celdas: []mesa.Celda{{Columna: 3, Fila: 3}}}}
	if _, err := svc.GuardarPlanoCompleto(empSalon, sedeSalon, actorA, origenTst,
		application.PlanoEntrada{Filas: 8, Columnas: 10, Pisos: pisos}); err != nil {
		t.Fatalf("guardar plano: %v", err)
	}
	m, err := svc.CrearMesa(empSalon, sedeSalon, actorA, origenTst,
		mesa.Mesa{Nombre: "Z1", Capacidad: 4, Forma: mesa.FormaCuadrada, Columna: 9, Fila: 7})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: m.ID, Columna: 9, Fila: 7},
	}); err != nil {
		t.Fatalf("mapa: %v", err)
	}
	if x, _ := st.Mesas.ByID(empSalon, m.ID); x.Zona != "Terraza" {
		t.Fatalf("abajo debería estar en Terraza, está en %q", x.Zona)
	}
	// Y ahora sube a la mezzanina, a una celda que allá sí tiene zona.
	if err := svc.GuardarMapa(empSalon, actorA, origenTst, []application.PosicionMesa{
		{ID: m.ID, Columna: 3, Fila: 3, PisoID: "piso_2"},
	}); err != nil {
		t.Fatalf("subir: %v", err)
	}
	x, _ := st.Mesas.ByID(empSalon, m.ID)
	if x.PisoID != "piso_2" {
		t.Fatalf("la mesa no subió: piso %q", x.PisoID)
	}
	if x.Zona != "Mezzanina" {
		t.Fatalf("al subir, la zona tenía que pasar a Mezzanina; quedó en %q", x.Zona)
	}
}
