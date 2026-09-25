package application_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/almacen"
)

/* UBICACIONES DENTRO DEL ALMACÉN.
 *
 * La regla que sostiene todo, y la que estas pruebas cuidan por encima de las
 * demás: LA SUMA POR UBICACIÓN ES LA EXISTENCIA DE LA SEDE. En cuanto se separan,
 * la ubicación miente sin que nada falle — alguien va al estante que dice el
 * sistema, no encuentra la mercancía, y a partir de ahí deja de mirar el sistema.
 *
 * El modo de fallo que más se vigila es sutil: si las ENTRADAS se ubican y las
 * SALIDAS no, la suma sigue cuadrando (el saldo «sin ubicar» se va a negativo) y
 * sin embargo el sitio donde está la mercancía deja de ser cierto. */

// servicioConUbicaciones cablea el maestro de ubicaciones.
func servicioConUbicaciones(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes) // las ubicaciones cuelgan de un almacén
	svc.ConUbicaciones(st.Ubicaciones)
	return svc
}

// almacenPrincipalID devuelve el almacén principal de la sede de prueba,
// creándolo si el seed no lo trae (es lo que hace el arranque real).
func almacenPrincipalID(t *testing.T, svc *application.Service) string {
	t.Helper()
	a, err := svc.AsegurarAlmacenPrincipal(empDemo, sede1, "sistema", origenTst)
	if err != nil {
		t.Fatalf("asegurar almacén principal: %v", err)
	}
	return a.ID
}

// nuevaUbicacion da de alta una ubicación y devuelve su id.
//
// Si el almacén YA tiene una con ese código, devuelve la que hay en vez de fallar.
// El seed siembra estantes y un muelle en el almacén principal —para que la
// demostración enseñe el reparto por ubicación y no una columna de «SIN UBICAR»—, y
// una prueba no tiene por qué romperse porque la demo traiga datos de más: lo que
// necesita es UNA ubicación con ese código, no haberla creado ella.
func nuevaUbicacion(t *testing.T, svc *application.Service, almacenID, codigo string) string {
	t.Helper()
	for _, u := range svc.UbicacionesDe(empDemo, almacenID) {
		if strings.EqualFold(u.Codigo, codigo) {
			return u.ID
		}
	}
	u, err := svc.CrearUbicacion(empDemo, actorA, origenTst, almacen.Ubicacion{
		AlmacenID: almacenID, Codigo: codigo, Nombre: "Ubicación " + codigo,
	})
	if err != nil {
		t.Fatalf("crear ubicación %s: %v", codigo, err)
	}
	return u.ID
}

// sumaYTotal devuelve la suma por ubicación y la existencia de la sede.
func sumaYTotal(t *testing.T, svc *application.Service, sku string) (float64, float64) {
	t.Helper()
	suma := 0.0
	for _, u := range svc.ExistenciaPorUbicacion(empDemo, sede1, sku) {
		suma += u.Cantidad
	}
	total := 0.0
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			total = e.Cantidad
		}
	}
	return suma, total
}

// TestUbicacion_ElMaestroSeDefiende: cada guarda evita una ubicación que después
// nadie sabría interpretar.
func TestUbicacion_ElMaestroSeDefiende(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)

	if _, err := svc.CrearUbicacion(empDemo, actorA, origenTst, almacen.Ubicacion{
		AlmacenID: alm, Codigo: "  ",
	}); !errors.Is(err, application.ErrUbicacionSinCodigo) {
		t.Errorf("sin código debía rechazarse: %v", err)
	}
	if _, err := svc.CrearUbicacion(empDemo, actorA, origenTst, almacen.Ubicacion{
		Codigo: "T-01",
	}); !errors.Is(err, application.ErrUbicacionSinAlmacen) {
		t.Errorf("sin almacén debía rechazarse: %v", err)
	}
	if _, err := svc.CrearUbicacion(empDemo, actorA, origenTst, almacen.Ubicacion{
		AlmacenID: alm, Codigo: "T-01", Tipo: "sótano",
	}); !errors.Is(err, application.ErrUbicacionTipoInvalido) {
		t.Errorf("un tipo inventado debía rechazarse: %v", err)
	}

	nuevaUbicacion(t, svc, alm, "T-01")
	// El código se normaliza, así que "a-01" es la misma ubicación y choca.
	if _, err := svc.CrearUbicacion(empDemo, actorA, origenTst, almacen.Ubicacion{
		AlmacenID: alm, Codigo: "a-01",
	}); !errors.Is(err, application.ErrUbicacionCodigoEnUso) {
		t.Errorf("un código repetido debía rechazarse aunque cambie la caja: %v", err)
	}
}

// TestUbicacion_LaRecepcionUbicaYLaSalidaConsume es el caso que sostiene todo el
// nivel: si las entradas se ubican y las salidas no, la suma sigue cuadrando pero
// el sitio deja de ser cierto.
func TestUbicacion_LaRecepcionUbicaYLaSalidaConsume(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)
	pasillo := nuevaUbicacion(t, svc, alm, "T-01")
	sku := primerSKU(t, svc)

	// Se recibe ubicando en A-01.
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 20, CostoUnitario: 10}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 20, UbicacionID: pasillo}}); err != nil {
		t.Fatalf("recibir: %v", err)
	}

	enA01 := func() float64 {
		for _, u := range svc.ExistenciaPorUbicacion(empDemo, sede1, sku) {
			if u.UbicacionID == pasillo {
				return u.Cantidad
			}
		}
		return 0
	}
	if !casi(enA01(), 20) {
		t.Fatalf("las 20 unidades tenían que quedar en A-01, hay %v", enA01())
	}

	// Una salida: tiene que consumir DE la ubicación, no dejarla intacta.
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "salida de prueba", -8, actorA, origenTst); err != nil {
		t.Fatalf("ajuste: %v", err)
	}

	// El seed trae existencia previa sin ubicar, que es la que se gasta primero.
	// Lo que NO puede pasar es que A-01 quede intacta mientras el total baja Y
	// tampoco que nadie consuma: la invariante lo cubre en los dos sentidos.
	suma, total := sumaYTotal(t, svc, sku)
	if !casi(suma, total) {
		t.Fatalf("la suma por ubicación se apartó de la existencia: %v vs %v", suma, total)
	}
	for _, u := range svc.ExistenciaPorUbicacion(empDemo, sede1, sku) {
		if u.Cantidad < -0.0001 {
			t.Errorf("ninguna ubicación puede quedar en negativo: %+v", u)
		}
	}
}

// TestUbicacion_SeGastaPrimeroLoQueNoEstaUbicado: el saldo que el sistema no sabe
// dónde está se consume antes, para que el inventario ubicado sea cada vez más
// fiel en vez de dejar un resto indefinido creciendo.
func TestUbicacion_SeGastaPrimeroLoQueNoEstaUbicado(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)
	pasillo := nuevaUbicacion(t, svc, alm, "B-02")
	sku := primerSKU(t, svc)

	// Las dos cargas van al MISMO almacén a propósito: una salida se surte de donde
	// está la mercancía, así que comparar casillas de almacenes distintos no
	// probaría el orden sino el filtro por almacén.
	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, "", sku, "carga sin ubicar", 6, "", "", actorA, origenTst); err != nil {
		t.Fatalf("cargar sin ubicar: %v", err)
	}
	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, pasillo, sku, "carga en B-02", 10, "", "", actorA, origenTst); err != nil {
		t.Fatalf("cargar en B-02: %v", err)
	}

	// Una salida de 4. Lo que esta prueba defiende es el ORDEN: sale de casillas
	// SIN UBICAR, nunca de B-02.
	//
	// No se puede fijar de qué casilla sin ubicar sale exactamente, y es correcto que
	// no se pueda: el almacén principal absorbe también el saldo histórico que no
	// tiene almacén —igual que lo hace la pantalla de existencias por almacén—, y ese
	// saldo se consume antes por ser el menos localizado. Fijar una casilla concreta
	// ataría la prueba al seed en vez de a la regla.
	sinUbicarAntes := 0.0
	for _, u := range svc.ExistenciaPorUbicacion(empDemo, sede1, sku) {
		if u.UbicacionID == "" {
			sinUbicarAntes += u.Cantidad
		}
	}
	if _, err := svc.Ajustar(empDemo, sede1, alm, sku, "salida", -4, actorA, origenTst); err != nil {
		t.Fatalf("ajuste: %v", err)
	}
	enB02, sinUbicarDespues := 0.0, 0.0
	for _, u := range svc.ExistenciaPorUbicacion(empDemo, sede1, sku) {
		if u.UbicacionID == pasillo && u.AlmacenID == alm {
			enB02 = u.Cantidad
		} else if u.UbicacionID == "" {
			sinUbicarDespues += u.Cantidad
		}
	}
	if !casi(enB02, 10) {
		t.Errorf("B-02 no tenía que tocarse todavía, quedó en %v", enB02)
	}
	if !casi(sinUbicarAntes-sinUbicarDespues, 4) {
		t.Errorf("las 4 tenían que salir de lo sin ubicar: bajó %v", sinUbicarAntes-sinUbicarDespues)
	}

	suma, total := sumaYTotal(t, svc, sku)
	if !casi(suma, total) {
		t.Fatalf("la suma por ubicación se apartó: %v vs %v", suma, total)
	}
}

// TestUbicacion_LoSinAlmacenSeGastaAntesQueLoDelAlmacen cierra el orden completo.
//
// Nació de un fallo intermitente: dos casillas «sin ubicar» —una histórica, sin
// almacén, y otra dentro de un almacén— empataban en todos los criterios, y el
// orden entre ellas lo decidía el recorrido de un mapa. La misma salida consumía
// una u otra según la corrida. No fallaba nada: solo cambiaba solo el sitio del
// que salía la mercancía, que es la clase de error que nadie persigue porque el
// total siempre cuadra.
func TestUbicacion_LoSinAlmacenSeGastaAntesQueLoDelAlmacen(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)
	sku := primerSKU(t, svc)

	// Lo que el seed dejó sin almacén, que es el histórico anterior a los almacenes.
	previo := 0.0
	for _, u := range svc.ExistenciaPorUbicacion(empDemo, sede1, sku) {
		if u.AlmacenID == "" {
			previo = u.Cantidad
		}
	}
	if previo <= 0 {
		t.Skip("el seed no dejó saldo sin almacén: nada que ordenar")
	}
	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, "", sku, "carga en el almacén", 10, "", "", actorA, origenTst); err != nil {
		t.Fatalf("cargar: %v", err)
	}

	// Una salida de toda la sede, más pequeña que el saldo histórico: no puede
	// tocar el almacén mientras quede saldo sin localizar.
	salida := previo / 2
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "salida", -salida, actorA, origenTst); err != nil {
		t.Fatalf("ajuste: %v", err)
	}
	enAlmacen, sinAlmacen := 0.0, 0.0
	for _, u := range svc.ExistenciaPorUbicacion(empDemo, sede1, sku) {
		if u.AlmacenID == "" {
			sinAlmacen = u.Cantidad
		} else if u.AlmacenID == alm && u.UbicacionID == "" {
			enAlmacen = u.Cantidad
		}
	}
	if !casi(enAlmacen, 10) {
		t.Errorf("el almacén no tenía que tocarse todavía, quedó en %v", enAlmacen)
	}
	if !casi(sinAlmacen, previo-salida) {
		t.Errorf("lo sin almacén tenía que bajar a %v, quedó en %v", previo-salida, sinAlmacen)
	}

	suma, total := sumaYTotal(t, svc, sku)
	if !casi(suma, total) {
		t.Fatalf("la suma por ubicación se apartó: %v vs %v", suma, total)
	}
}

// TestUbicacion_UnaUbicacionAjenaNoSeUsaEnSilencio: pedir ubicar en una ubicación
// de OTRO almacén no puede caer callando a otra — el llamante creería que ubicó
// donde dijo. Cae a «sin ubicar», que se ve.
func TestUbicacion_UnaUbicacionAjenaNoSeUsaEnSilencio(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)
	propia := nuevaUbicacion(t, svc, alm, "C-03")
	sku := primerSKU(t, svc)

	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, "ubi_inventada", sku, "carga", 5, "", "", actorA, origenTst); err != nil {
		t.Fatalf("ajuste con ubicación inventada: %v", err)
	}
	for _, u := range svc.ExistenciaPorUbicacion(empDemo, sede1, sku) {
		if u.UbicacionID == propia {
			t.Fatal("una ubicación inventada no puede acabar en otra real")
		}
	}
	suma, total := sumaYTotal(t, svc, sku)
	if !casi(suma, total) {
		t.Fatalf("la suma por ubicación se apartó: %v vs %v", suma, total)
	}
}

// TestUbicacion_SinCablearTodoSigueIgual: la regresión que importa. Un servicio
// sin el maestro de ubicaciones se comporta exactamente como antes.
func TestUbicacion_SinCablearTodoSigueIgual(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes) // ...pero SIN ConUbicaciones
	sku := primerSKU(t, svc)
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "salida", -3, actorA, origenTst); err != nil {
		t.Fatalf("ajuste: %v", err)
	}
	filas := svc.ExistenciaPorUbicacion(empDemo, sede1, sku)
	if len(filas) != 1 || filas[0].Codigo != "SIN UBICAR" {
		t.Fatalf("sin ubicaciones todo cae en una sola casilla sin ubicar: %+v", filas)
	}
	suma, total := sumaYTotal(t, svc, sku)
	if !casi(suma, total) {
		t.Fatalf("la suma se apartó: %v vs %v", suma, total)
	}
}
