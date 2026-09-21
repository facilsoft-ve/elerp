package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/almacen"
)

/* TIPOS DE OPERACIÓN Y RECEPCIÓN EN DOS PASOS.
 *
 * La prueba que más importa de este archivo es la primera, y no es la más vistosa:
 * SIN CONFIGURAR, TODO SIGUE IGUAL. Es lo que permite desplegar esto sobre datos
 * vivos sin migrar nada. Si se rompe, no lo notará quien active los dos pasos —lo
 * notará quien no los active, que son todos los demás. */

// servicioConOperaciones cablea ubicaciones y tipos de operación.
func servicioConOperaciones(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConUbicaciones(st.Ubicaciones)
	svc.ConTiposOperacion(st.TiposOperacion)
	return svc
}

// recibir crea, confirma y recibe una orden de compra. Devuelve lo recibido.
func recibirSimple(t *testing.T, svc *application.Service, sku string, cant float64, ubic string) {
	t.Helper()
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: cant, CostoUnitario: 10}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: cant, UbicacionID: ubic}}); err != nil {
		t.Fatalf("recibir: %v", err)
	}
}

// TestOperacion_SinConfigurarTodoSigueIgual es la regresión que sostiene la tanda.
func TestOperacion_SinConfigurarTodoSigueIgual(t *testing.T) {
	svc := servicioConOperaciones(t) // ...con el maestro cableado pero VACÍO
	alm := almacenPrincipalID(t, svc)
	estante := nuevaUbicacion(t, svc, alm, "A-01")
	sku := primerSKU(t, svc)

	if _, hay := svc.OperacionPara(empDemo, sede1, almacen.ClaseRecepcion); hay {
		t.Fatal("sin sembrar no puede haber tipo por defecto")
	}
	recibirSimple(t, svc, sku, 12, estante)

	// La mercancía va donde pidió la línea, como siempre.
	if got := saldoEn(t, svc, sku, estante); !casi(got, 12) {
		t.Errorf("las 12 tenían que quedar en A-01, hay %v", got)
	}
	if n := len(svc.PendienteDeUbicar(empDemo, sede1)); n != 0 {
		t.Errorf("sin dos pasos no hay nada pendiente de ubicar, hay %d", n)
	}
	suma, total := sumaYTotal(t, svc, sku)
	if !casi(suma, total) {
		t.Fatalf("la suma por ubicación se apartó: %v vs %v", suma, total)
	}
}

// TestOperacion_SembrarEsIdempotenteYDeUnPaso: los cuatro tipos por defecto no
// cambian el comportamiento de nadie, y sembrar dos veces no duplica.
func TestOperacion_SembrarEsIdempotenteYDeUnPaso(t *testing.T) {
	svc := servicioConOperaciones(t)

	if n := svc.SembrarTiposOperacion(empDemo, actorA, origenTst); n != 4 {
		t.Fatalf("tenían que sembrarse 4 tipos, se sembraron %d", n)
	}
	if n := svc.SembrarTiposOperacion(empDemo, actorA, origenTst); n != 0 {
		t.Errorf("sembrar dos veces no puede duplicar, creó %d", n)
	}
	for _, op := range svc.TiposOperacion(empDemo) {
		if op.Pasos != 1 {
			t.Errorf("%s se sembró con %d pasos: los por defecto son de uno, para que nadie note el cambio", op.Codigo, op.Pasos)
		}
	}
	op, hay := svc.OperacionPara(empDemo, sede1, almacen.ClaseRecepcion)
	if !hay || op.Codigo != "REC" {
		t.Fatalf("la recepción por defecto tenía que resolverse a REC: %+v (%v)", op, hay)
	}
}

// TestOperacion_ElMaestroSeDefiende: cada guarda evita un tipo que después haría
// algo distinto de lo que dice.
func TestOperacion_ElMaestroSeDefiende(t *testing.T) {
	svc := servicioConOperaciones(t)

	if _, err := svc.CrearTipoOperacion(empDemo, actorA, origenTst, almacen.TipoOperacion{
		Clase: almacen.ClaseRecepcion,
	}); !errors.Is(err, application.ErrOperacionSinCodigo) {
		t.Errorf("sin código debía rechazarse: %v", err)
	}
	if _, err := svc.CrearTipoOperacion(empDemo, actorA, origenTst, almacen.TipoOperacion{
		Codigo: "X", Clase: "inventada",
	}); !errors.Is(err, application.ErrOperacionClaseInvalida) {
		t.Errorf("una clase que nadie lee debía rechazarse: %v", err)
	}
	if _, err := svc.CrearTipoOperacion(empDemo, actorA, origenTst, almacen.TipoOperacion{
		Codigo: "X", Clase: almacen.ClaseRecepcion, Pasos: 3,
	}); !errors.Is(err, application.ErrOperacionPasos) {
		t.Errorf("tres pasos debía rechazarse: %v", err)
	}
	// La importante: dos pasos SIN ubicación intermedia no es un proceso en dos
	// pasos — es una recepción normal que además pierde el rastro del muelle.
	if _, err := svc.CrearTipoOperacion(empDemo, actorA, origenTst, almacen.TipoOperacion{
		Codigo: "X", Clase: almacen.ClaseRecepcion, Pasos: 2,
	}); !errors.Is(err, application.ErrOperacionSinIntermedia) {
		t.Errorf("dos pasos sin intermedia debía rechazarse: %v", err)
	}
}

// TestOperacion_SoloUnPorDefectoPorClase: con dos, la operación dependería de cuál
// se leyera primero, que es un orden de mapa — o sea, ninguno.
func TestOperacion_SoloUnPorDefectoPorClase(t *testing.T) {
	svc := servicioConOperaciones(t)
	svc.SembrarTiposOperacion(empDemo, actorA, origenTst)

	otro, err := svc.CrearTipoOperacion(empDemo, actorA, origenTst, almacen.TipoOperacion{
		Codigo: "REC2", Nombre: "Otra recepción", Clase: almacen.ClaseRecepcion, PorDefecto: true,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	porDefecto := []string{}
	for _, op := range svc.TiposOperacion(empDemo) {
		if op.Clase == almacen.ClaseRecepcion && op.PorDefecto {
			porDefecto = append(porDefecto, op.Codigo)
		}
	}
	if len(porDefecto) != 1 || porDefecto[0] != otro.Codigo {
		t.Fatalf("tenía que quedar uno solo por defecto y ser el nuevo: %v", porDefecto)
	}
}

// TestOperacion_DosPasosDejaLaMercanciaEnElMuelle es el comportamiento nuevo, y la
// parte que más se puede malinterpretar: la ubicación que pide la línea se IGNORA.
func TestOperacion_DosPasosDejaLaMercanciaEnElMuelle(t *testing.T) {
	svc := servicioConOperaciones(t)
	alm := almacenPrincipalID(t, svc)
	muelle := nuevaUbicacion(t, svc, alm, "MUELLE")
	estante := nuevaUbicacion(t, svc, alm, "A-01")
	sku := primerSKU(t, svc)

	if _, err := svc.CrearTipoOperacion(empDemo, actorA, origenTst, almacen.TipoOperacion{
		Codigo: "REC2P", Nombre: "Recepción en dos pasos", Clase: almacen.ClaseRecepcion,
		Pasos: 2, UbicacionIntermediaID: muelle, PorDefecto: true,
	}); err != nil {
		t.Fatalf("crear tipo: %v", err)
	}

	// Se recibe pidiendo el ESTANTE: con dos pasos, se ignora.
	recibirSimple(t, svc, sku, 20, estante)

	if got := saldoEn(t, svc, sku, muelle); !casi(got, 20) {
		t.Errorf("con dos pasos todo aterriza en el muelle, hay %v", got)
	}
	if got := saldoEn(t, svc, sku, estante); !casi(got, 0) {
		t.Errorf("el estante no puede recibir nada hasta el segundo paso, tiene %v", got)
	}

	// Y se ve como pendiente: si no se viera, la mercancía se quedaría en el muelle
	// sin que nadie se entere — el total de existencia no lo delata.
	pend := svc.PendienteDeUbicar(empDemo, sede1)
	encontrado := false
	for _, p := range pend {
		if p.SKU == sku && casi(p.Cantidad, 20) {
			encontrado = true
		}
	}
	if !encontrado {
		t.Fatalf("las 20 del muelle tenían que salir como pendientes de ubicar: %+v", pend)
	}

	// Segundo paso: se coloca con el traslado, que es la operación que existe para eso.
	if _, err := svc.TrasladarEntreUbicaciones(empDemo, actorA, origenTst, application.TrasladoPeticion{
		SedeID: sede1, AlmacenID: alm, Origen: muelle, Destino: estante, SKU: sku, Cantidad: 20,
		Motivo: "ubicación tras revisión",
	}); err != nil {
		t.Fatalf("segundo paso: %v", err)
	}
	if got := saldoEn(t, svc, sku, estante); !casi(got, 20) {
		t.Errorf("tras el segundo paso las 20 están en A-01, hay %v", got)
	}
	if n := len(svc.PendienteDeUbicar(empDemo, sede1)); n != 0 {
		t.Errorf("ya no queda nada pendiente, quedan %d", n)
	}
	suma, total := sumaYTotal(t, svc, sku)
	if !casi(suma, total) {
		t.Fatalf("la suma por ubicación se apartó: %v vs %v", suma, total)
	}
}

// TestOperacion_ElDeLaSedeGanaAlGeneral: una sede se configura aparte justamente
// porque allí el proceso es distinto.
func TestOperacion_ElDeLaSedeGanaAlGeneral(t *testing.T) {
	svc := servicioConOperaciones(t)
	alm := almacenPrincipalID(t, svc)
	muelle := nuevaUbicacion(t, svc, alm, "MUELLE")
	svc.SembrarTiposOperacion(empDemo, actorA, origenTst) // REC general, un paso

	if _, err := svc.CrearTipoOperacion(empDemo, actorA, origenTst, almacen.TipoOperacion{
		Codigo: "RECS1", Nombre: "Recepción de esta sede", Clase: almacen.ClaseRecepcion,
		SedeID: sede1, Pasos: 2, UbicacionIntermediaID: muelle, PorDefecto: true,
	}); err != nil {
		t.Fatalf("crear: %v", err)
	}
	op, hay := svc.OperacionPara(empDemo, sede1, almacen.ClaseRecepcion)
	if !hay || op.Codigo != "RECS1" {
		t.Fatalf("el de la sede tenía que ganar: %+v", op)
	}
	// Y el general sigue valiendo donde no hay uno propio.
	otra, hay2 := svc.OperacionPara(empDemo, "sede_que_no_existe", almacen.ClaseRecepcion)
	if !hay2 || otra.Codigo != "REC" {
		t.Fatalf("sin tipo propio tenía que caer al general: %+v", otra)
	}
}
