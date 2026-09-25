package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
)

/* TRASLADO ENTRE UBICACIONES.
 *
 * Lo que estas pruebas cuidan: un traslado NO cambia nada salvo el sitio. Ni la
 * existencia de la sede, ni el costo promedio, ni el diario. En cuanto una de esas
 * tres se mueve, el traslado dejó de ser un traslado y nadie lo va a notar mirando
 * la pantalla de existencias — que seguirá cuadrando. */

// saldoEn devuelve lo que hay de un sku en una ubicación concreta de la sede.
func saldoEn(t *testing.T, svc *application.Service, sku, ubicacionID string) float64 {
	t.Helper()
	for _, u := range svc.ExistenciaPorUbicacion(empDemo, sede1, sku) {
		if u.UbicacionID == ubicacionID {
			return u.Cantidad
		}
	}
	return 0
}

// TestTraslado_MueveElSitioYNadaMas es la prueba central: cambia la ubicación y
// deja igual la existencia, el costo promedio y el número de asientos.
func TestTraslado_MueveElSitioYNadaMas(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)
	muelle := nuevaUbicacion(t, svc, alm, "T-MUELLE")
	estante := nuevaUbicacion(t, svc, alm, "E-01")
	sku := primerSKU(t, svc)

	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, muelle, sku, "llegada", 30, "", "", actorA, origenTst); err != nil {
		t.Fatalf("cargar en muelle: %v", err)
	}

	antesCant, antesCosto := existenciaDe(t, svc, empDemo, sede1, sku)
	antesAsientos := len(svc.LibroDiario(empDemo))

	if _, err := svc.TrasladarEntreUbicaciones(empDemo, actorA, origenTst, application.TrasladoPeticion{
		SedeID: sede1, AlmacenID: alm, Origen: muelle, Destino: estante, SKU: sku, Cantidad: 12,
	}); err != nil {
		t.Fatalf("trasladar: %v", err)
	}

	if got := saldoEn(t, svc, sku, muelle); !casi(got, 18) {
		t.Errorf("el muelle tenía que bajar a 18, quedó en %v", got)
	}
	if got := saldoEn(t, svc, sku, estante); !casi(got, 12) {
		t.Errorf("el estante tenía que subir a 12, quedó en %v", got)
	}

	despuesCant, despuesCosto := existenciaDe(t, svc, empDemo, sede1, sku)
	if !casi(antesCant, despuesCant) {
		t.Errorf("un traslado no cambia la existencia: %v → %v", antesCant, despuesCant)
	}
	if !casi(antesCosto, despuesCosto) {
		t.Errorf("un traslado no cambia el costo promedio: %v → %v", antesCosto, despuesCosto)
	}
	// La que más importa: un traslado NO es una merma seguida de un sobrante.
	if n := len(svc.LibroDiario(empDemo)); n != antesAsientos {
		t.Errorf("un traslado no asienta: el diario pasó de %d a %d", antesAsientos, n)
	}

	suma, total := sumaYTotal(t, svc, sku)
	if !casi(suma, total) {
		t.Fatalf("la suma por ubicación se apartó: %v vs %v", suma, total)
	}
}

// TestTraslado_NoMueveLoQueNoEsta: sin saldo en el origen no se traslada NADA.
// Un traslado a medias inventaría existencia en el destino.
func TestTraslado_NoMueveLoQueNoEsta(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)
	a := nuevaUbicacion(t, svc, alm, "T-01")
	b := nuevaUbicacion(t, svc, alm, "T-B01")
	sku := primerSKU(t, svc)

	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, a, sku, "carga", 5, "", "", actorA, origenTst); err != nil {
		t.Fatalf("cargar: %v", err)
	}
	if _, err := svc.TrasladarEntreUbicaciones(empDemo, actorA, origenTst, application.TrasladoPeticion{
		SedeID: sede1, AlmacenID: alm, Origen: a, Destino: b, SKU: sku, Cantidad: 9,
	}); !errors.Is(err, application.ErrTrasladoSinSaldo) {
		t.Fatalf("trasladar más de lo que hay debía fallar: %v", err)
	}
	if got := saldoEn(t, svc, sku, a); !casi(got, 5) {
		t.Errorf("el origen no podía tocarse: %v", got)
	}
	if got := saldoEn(t, svc, sku, b); !casi(got, 0) {
		t.Errorf("el destino no podía recibir nada: %v", got)
	}
}

// TestTraslado_ElDestinoInvalidoNoCaeEnSilencio: a diferencia de una recepción,
// aquí una ubicación que no vale aborta la operación. Dejar la mercancía en otro
// sitio del que el operador pidió es peor que no moverla.
func TestTraslado_ElDestinoInvalidoNoCaeEnSilencio(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)
	a := nuevaUbicacion(t, svc, alm, "T-01")
	sku := primerSKU(t, svc)

	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, a, sku, "carga", 5, "", "", actorA, origenTst); err != nil {
		t.Fatalf("cargar: %v", err)
	}
	if _, err := svc.TrasladarEntreUbicaciones(empDemo, actorA, origenTst, application.TrasladoPeticion{
		SedeID: sede1, AlmacenID: alm, Origen: a, Destino: "ubi_inventada", SKU: sku, Cantidad: 2,
	}); !errors.Is(err, application.ErrTrasladoDestinoInvalido) {
		t.Fatalf("un destino inventado debía abortar el traslado: %v", err)
	}
	if got := saldoEn(t, svc, sku, a); !casi(got, 5) {
		t.Errorf("nada tenía que moverse: %v", got)
	}
	if _, err := svc.TrasladarEntreUbicaciones(empDemo, actorA, origenTst, application.TrasladoPeticion{
		SedeID: sede1, AlmacenID: alm, Origen: a, Destino: a, SKU: sku, Cantidad: 2,
	}); !errors.Is(err, application.ErrTrasladoMismaUbicacion) {
		t.Errorf("origen igual a destino debía rechazarse: %v", err)
	}
}

// TestTraslado_ElVencidoSiSeMueve: apartar a cuarentena lo que caducó es justo lo
// que hay que hacer con ello. Negarlo —como se niega una venta— dejaría la
// mercancía vencida en el anaquel sin forma de sacarla.
func TestTraslado_ElVencidoSiSeMueve(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)
	anaquel := nuevaUbicacion(t, svc, alm, "T-01")
	cuarentena := nuevaUbicacion(t, svc, alm, "CUARENTENA")
	sku := "TRASL-LOTE-1"
	prodID := productoConLote(t, svc, sku, true)

	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, anaquel, sku, "carga",
		10, "L-VIEJO", "2020-01-01", actorA, origenTst); err != nil {
		t.Fatalf("cargar lote vencido: %v", err)
	}
	// Se comprueba primero que ese lote NO se puede VENDER: es lo que hace del
	// traslado la única salida. Se mira el repartidor, que es por donde pasan la
	// venta y la facturación — un AJUSTE sí puede sacarlo, y debe poder: una merma
	// de mercancía caducada es justamente una de las formas de deshacerse de ella.
	if _, err := svc.RepartirSalidaFEFO(empDemo, sede1, alm, prodID, 1); !errors.Is(err, application.ErrSoloQuedaVencido) {
		t.Fatalf("una venta de lote vencido debía negarse diciendo que está vencido: %v", err)
	}
	if _, err := svc.TrasladarEntreUbicaciones(empDemo, actorA, origenTst, application.TrasladoPeticion{
		SedeID: sede1, AlmacenID: alm, Origen: anaquel, Destino: cuarentena, SKU: sku, Cantidad: 10,
	}); err != nil {
		t.Fatalf("el vencido tenía que poder apartarse: %v", err)
	}
	if got := saldoEn(t, svc, sku, cuarentena); !casi(got, 10) {
		t.Errorf("las 10 tenían que quedar en cuarentena: %v", got)
	}
	if got := saldoEn(t, svc, sku, anaquel); !casi(got, 0) {
		t.Errorf("el anaquel tenía que quedar vacío: %v", got)
	}
}

// TestTraslado_ElLoteViajaConLaMercancia: el lote no se pierde al cambiar de
// estante. Si se perdiera, la trazabilidad se rompería por mover una caja.
func TestTraslado_ElLoteViajaConLaMercancia(t *testing.T) {
	svc := servicioConUbicaciones(t)
	alm := almacenPrincipalID(t, svc)
	a := nuevaUbicacion(t, svc, alm, "T-01")
	b := nuevaUbicacion(t, svc, alm, "T-B01")
	sku := "TRASL-LOTE-2"
	prodID := productoConLote(t, svc, sku, true)

	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, a, sku, "carga",
		8, "L-2027", "2027-12-31", actorA, origenTst); err != nil {
		t.Fatalf("cargar: %v", err)
	}
	if _, err := svc.TrasladarEntreUbicaciones(empDemo, actorA, origenTst, application.TrasladoPeticion{
		SedeID: sede1, AlmacenID: alm, Origen: a, Destino: b, SKU: sku, Cantidad: 3,
	}); err != nil {
		t.Fatalf("trasladar: %v", err)
	}
	// El saldo POR LOTE no se movió: sigue habiendo 8 del lote, en dos sitios.
	total := 0.0
	for _, sl := range svc.SaldosPorLote(empDemo, sede1, prodID) {
		if sl.Lote == "L-2027" {
			total = sl.Cantidad
		}
	}
	if !casi(total, 8) {
		t.Errorf("el lote tenía que seguir con 8 unidades: %v", total)
	}
	suma, tot := sumaYTotal(t, svc, sku)
	if !casi(suma, tot) {
		t.Fatalf("la suma por ubicación se apartó: %v vs %v", suma, tot)
	}
}
