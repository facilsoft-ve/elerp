package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* EL CHEQUEO TIENE QUE ENCONTRAR LO QUE SE ROMPE A PROPÓSITO.
 *
 * Un diagnóstico que nunca acusa nada es indistinguible de uno que no funciona, y
 * eso es peor que no tenerlo: da por sano lo que no miró. Cada invariante se prueba
 * rompiéndola, y además se comprueba que el caso sano NO dispara —un chequeo que
 * grita siempre se termina ignorando—.
 */

// hallazgosDe filtra por clase.
func hallazgosDe(d application.DiagnosticoInventario, clase string) []application.HallazgoInventario {
	out := []application.HallazgoInventario{}
	for _, h := range d.Hallazgos {
		if h.Clase == clase {
			out = append(out, h)
		}
	}
	return out
}

// TestDiagnostico_ElCasoSanoNoAcusaNada: sobre un inventario recién cargado por la
// vía normal, el chequeo tiene que callarse. Si acusa acá, acusará siempre.
func TestDiagnostico_ElCasoSanoNoAcusaNada(t *testing.T) {
	svc := servicioCompleto(t)
	sku := primerSKU(t, svc)
	almacenPrincipalID(t, svc)
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "carga", 10, actorA, origenTst); err != nil {
		t.Fatalf("ajustar: %v", err)
	}

	d := svc.DiagnosticarInventario(empDemo)
	for _, clase := range []string{
		application.ClaseAlmacenesNoSuman,
		application.ClaseUbicacionesNoSuman,
		application.ClaseLotesNoSuman,
		application.ClaseExistenciaNegativa,
		application.ClaseApartadoExcedido,
	} {
		if hs := hallazgosDe(d, clase); len(hs) > 0 {
			t.Errorf("%s no debía acusar nada en el caso sano: %+v", clase, hs[0])
		}
	}
}

// TestDiagnostico_VeLaExistenciaNegativa: salir más de lo que hay deja el ledger en
// negativo. No es un error de cálculo —el ledger es append-only y la suma es esa—,
// es mercancía que se despachó sin tenerla.
func TestDiagnostico_VeLaExistenciaNegativa(t *testing.T) {
	svc := servicioCompleto(t)
	sku := primerSKU(t, svc)
	almacenPrincipalID(t, svc)
	// Se fuerza el negativo por la vía del ajuste, que es como llega en la práctica.
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "descarga", -100000, actorA, origenTst); err != nil {
		t.Fatalf("ajustar: %v", err)
	}

	hs := hallazgosDe(svc.DiagnosticarInventario(empDemo), application.ClaseExistenciaNegativa)
	if len(hs) == 0 {
		t.Fatal("no vio la existencia negativa")
	}
	if hs[0].SKU != sku {
		t.Errorf("acusó el SKU equivocado: %s (esperaba %s)", hs[0].SKU, sku)
	}
	if hs[0].Encontrado >= 0 {
		t.Errorf("el hallazgo debe traer la cantidad negativa, trae %v", hs[0].Encontrado)
	}
}

// TestDiagnostico_VeElMovimientoQueNadieAsento: un movimiento anexado directo al
// repositorio —como hace el seed, o un arreglo contra la base— mueve la existencia
// sin tocar la contabilidad. El balance sigue cuadrando porque no falta media pata
// sino el asiento entero: nada lo delata salvo esto.
func TestDiagnostico_VeElMovimientoQueNadieAsento(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConSedes(st.Sedes)
	sku := primerSKU(t, svc)
	p, _ := svc.ProductoPorSKU(empDemo, sku)

	antes := len(hallazgosDe(svc.DiagnosticarInventario(empDemo), application.ClaseMovimientoSinAsiento))
	st.Movimientos.Append(inventario.Movimiento{
		EmpresaID: empDemo, SedeID: sede1, ProductoID: p.ID, SKU: sku,
		Tipo: inventario.MovEntrada, Cantidad: 5, CostoUnitario: 100,
		Motivo: "anexado a mano", Actor: actorA, Fecha: "2026-01-01T00:00:00Z",
	})
	hs := hallazgosDe(svc.DiagnosticarInventario(empDemo), application.ClaseMovimientoSinAsiento)
	if len(hs) != antes+1 {
		t.Fatalf("esperaba un hallazgo más (%d), hay %d", antes+1, len(hs))
	}
}

// TestDiagnostico_VeElDocumentoQueNoAsento es EL PUNTO CIEGO, y es la razón de ser
// de este chequeo.
//
// La recontabilización excluye los movimientos que vienen de un documento, y con
// razón: su asiento lo emite el documento con el importe agregado, asentarlos uno a
// uno los contaría dos veces. Pero de ahí no se sigue que ese asiento exista — un
// asiento descuadrado no se guarda, solo se registra en el log. Cuando eso pasa, el
// movimiento queda huérfano PARA SIEMPRE: la recontabilización no lo repesca
// («tiene documento») y hasta ahora nada lo señalaba.
func TestDiagnostico_VeElDocumentoQueNoAsento(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConSedes(st.Sedes)
	sku := primerSKU(t, svc)
	p, _ := svc.ProductoPorSKU(empDemo, sku)

	// Un movimiento CON documento cuyo asiento no existe: exactamente lo que queda
	// cuando el asiento de la factura sale descuadrado y no se guarda.
	st.Movimientos.Append(inventario.Movimiento{
		EmpresaID: empDemo, SedeID: sede1, ProductoID: p.ID, SKU: sku,
		Tipo: inventario.MovSalida, Cantidad: -3, CostoUnitario: 100,
		RefTipo: "documento", RefID: "doc_que_no_asento",
		Motivo: "venta", Actor: actorA, Fecha: "2026-01-01T00:00:00Z",
	})

	// La recontabilización NO lo ve: ese es el punto ciego, y se deja probado para
	// que quede claro que el diagnóstico no duplica lo que ya existía.
	for _, m := range svc.RevisarContabilidadDeInventario(empDemo).SinAsiento {
		if m.ID != "" && m.SKU == sku && m.Tipo == string(inventario.MovSalida) {
			t.Error("la recontabilización no debería ver los movimientos con documento; si ahora los ve, este chequeo sobra")
		}
	}

	hs := hallazgosDe(svc.DiagnosticarInventario(empDemo), application.ClaseDocumentoSinAsiento)
	visto := false
	for _, h := range hs {
		if h.Ref == "doc_que_no_asento" {
			visto = true
			if h.Esperado != 300 {
				t.Errorf("el importe huérfano debía ser 300, es %v", h.Esperado)
			}
		}
	}
	if !visto {
		t.Fatalf("no vio el documento sin asiento; hallazgos: %+v", hs)
	}
}

// TestDiagnostico_NoSeContradiceConLaValoracion: si la pantalla de Valoración dice
// que una cuenta no cuadra, el diagnóstico NO puede decir que el inventario está
// sano. Parece obvio y no lo era: el chequeo miraba las causas conocidas del
// descuadre —movimiento huérfano, documento sin asiento— y no el descuadre mismo,
// así que daba por sanas dos empresas demo cuya valoración acusaba diferencia en la
// misma pantalla.
func TestDiagnostico_NoSeContradiceConLaValoracion(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConSedes(st.Sedes)
	sku := primerSKU(t, svc)
	p, _ := svc.ProductoPorSKU(empDemo, sku)

	// Mercancía que entra sin que nadie la asiente: el inventario sube y la cuenta
	// no. Es como llega cualquier dato cargado contra la base.
	st.Movimientos.Append(inventario.Movimiento{
		EmpresaID: empDemo, SedeID: sede1, ProductoID: p.ID, SKU: sku,
		Tipo: inventario.MovEntrada, Cantidad: 7, CostoUnitario: 1000,
		RefTipo: "documento", RefID: "doc_sin_asiento_ninguno",
		Motivo: "carga directa", Actor: actorA, Fecha: "2026-01-01T00:00:00Z",
	})

	v := svc.Valoracion(empDemo, "")
	d := svc.DiagnosticarInventario(empDemo)
	if v.TodoCuadrado {
		t.Skip("la valoración cuadra; este test necesita un descuadre para probar algo")
	}
	if d.Sano {
		t.Error("la valoración acusa descuadre y el diagnóstico dice que está sano")
	}
	if len(hallazgosDe(d, application.ClaseNoCuadraContabilidad)) == 0 {
		t.Errorf("faltó el hallazgo del descuadre; hallazgos: %+v", d.Hallazgos)
	}
}

// TestDiagnostico_NoAcusaLaTransferencia: una transferencia mueve mercancía entre
// sedes de la misma empresa y NO se asienta nunca —el patrimonio no cambia—. Si el
// chequeo la acusara, el informe se llenaría de falsos positivos y dejaría de
// leerse, que es la forma habitual en que muere una alarma.
func TestDiagnostico_NoAcusaLaTransferencia(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConSedes(st.Sedes)
	sku := primerSKU(t, svc)
	p, _ := svc.ProductoPorSKU(empDemo, sku)

	st.Movimientos.Append(inventario.Movimiento{
		EmpresaID: empDemo, SedeID: sede1, ProductoID: p.ID, SKU: sku,
		Tipo: inventario.MovTransferencia, Cantidad: -2, CostoUnitario: 100,
		RefTipo: "transferencia", RefID: "tr_sin_asiento",
		Motivo: "envío a Sede Este", Actor: actorA, Fecha: "2026-01-01T00:00:00Z",
	})
	for _, h := range svc.DiagnosticarInventario(empDemo).Hallazgos {
		if h.Ref == "tr_sin_asiento" {
			t.Errorf("la transferencia no se asienta y no debe acusarse: %+v", h)
		}
	}
}

// TestDiagnostico_VeLosIdsRepetidos: dos movimientos con el mismo id no rompen
// ninguna suma —el fold recorre la lista— pero envenenan todo lo que INDEXA por id:
// asentar uno da por asentado al otro, y el rastro de lotes enlaza al equivocado.
// Apareció en la demo del restaurante buscando otra cosa.
func TestDiagnostico_VeLosIdsRepetidos(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConSedes(st.Sedes)
	sku := primerSKU(t, svc)
	p, _ := svc.ProductoPorSKU(empDemo, sku)

	base := inventario.Movimiento{
		ID: "mov_colision", EmpresaID: empDemo, SedeID: sede1, ProductoID: p.ID, SKU: sku,
		Tipo: inventario.MovEntrada, Cantidad: 3, CostoUnitario: 100,
		Motivo: "primero", Actor: actorA, Fecha: "2026-01-01T00:00:00Z",
	}
	st.Movimientos.Append(base)
	base.Cantidad, base.Motivo = 9, "segundo"
	st.Movimientos.Append(base)

	hs := hallazgosDe(svc.DiagnosticarInventario(empDemo), application.ClaseIDDuplicado)
	if len(hs) != 1 {
		t.Fatalf("esperaba un hallazgo de id repetido, hay %d", len(hs))
	}
	if hs[0].Ref != "mov_colision" {
		t.Errorf("el hallazgo debe señalar el id colisionado, señala %q", hs[0].Ref)
	}
}
