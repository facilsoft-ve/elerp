package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* VALORACIÓN, CORRECCIÓN DE COSTO Y CONTEO FÍSICO.
 *
 * El hilo que une los tres: el valor del inventario tiene que poder explicarse.
 * Dónde está, por qué cambió y si coincide con lo que dice la contabilidad. */

// servicioCompleto cablea todo lo que la Ola C necesita.
func servicioCompleto(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConUbicaciones(st.Ubicaciones)
	return svc
}

// TestValoracion_ElDesgloseSumaElTotal: la comprobación básica de cualquier
// informe. Si las partes no suman el todo, una de las dos cifras miente y el
// informe deja de servir para cuadrar nada.
func TestValoracion_ElDesgloseSumaElTotal(t *testing.T) {
	svc := servicioCompleto(t)
	alm := almacenPrincipalID(t, svc)
	a01 := nuevaUbicacion(t, svc, alm, "A-01")
	sku := primerSKU(t, svc)

	if _, err := svc.AjustarEnUbicacion(empDemo, sede1, alm, a01, sku, "carga", 10, "", "", actorA, origenTst); err != nil {
		t.Fatalf("cargar: %v", err)
	}

	v := svc.Valoracion(empDemo, "")
	if len(v.Filas) == 0 {
		t.Fatal("el informe salió vacío")
	}
	suma := 0.0
	for _, f := range v.Filas {
		suma += f.Valor
	}
	if !casi(round2Test(suma), v.ValorTotal) {
		t.Errorf("las filas suman %v y el total dice %v", round2Test(suma), v.ValorTotal)
	}
	sumaAlm := 0.0
	for _, t := range v.PorAlmacen {
		sumaAlm += t.Valor
	}
	if !casi(round2Test(sumaAlm), v.ValorTotal) {
		t.Errorf("los totales por almacén suman %v y el total dice %v", round2Test(sumaAlm), v.ValorTotal)
	}
	sumaCta := 0.0
	for _, t := range v.PorCuenta {
		sumaCta += t.Valor
	}
	if !casi(round2Test(sumaCta), v.ValorTotal) {
		t.Errorf("los totales por cuenta suman %v y el total dice %v", round2Test(sumaCta), v.ValorTotal)
	}
	// Y la ubicación tiene que aparecer: es la razón de ser del informe.
	vistaA01 := false
	for _, f := range v.Filas {
		if f.Ubicacion == "A-01" {
			vistaA01 = true
		}
	}
	if !vistaA01 {
		t.Error("A-01 tenía que aparecer en el desglose")
	}
}

// TestValoracion_ComparaConLaContabilidad es lo que cierra la cuenta por rubro:
// separar el inventario en varias cuentas no sirve de nada si nadie puede
// comprobar que cada una corresponde a mercancía que existe.
//
// Se mide el MOVIMIENTO de las dos cifras, no su valor absoluto: el seed carga
// existencias sin emitir asientos —su 1201 arranca en negativo con inventario
// positivo—, así que exigir que cuadre probaría el seed y no el informe. Lo que
// tiene que cumplirse es que una compra mueva el inventario y el diario EN LA
// MISMA CUENTA Y POR EL MISMO IMPORTE; si eso se cumple, un saldo de partida
// correcto cuadra.
func TestValoracion_ComparaConLaContabilidad(t *testing.T) {
	svc := servicioCompleto(t)
	crearCuentaAlterna(t, svc)
	rubroConCuenta(t, svc, "Electrónica", ctaInvAlterna)
	sku := "ELE-VAL"
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: sku, Nombre: "Parlante", Precio: 100, Rubro: "Electrónica",
	})

	deCuenta := func(v application.ValoracionResult, codigo string) application.TotalValoracion {
		for _, tot := range v.PorCuenta {
			if tot.Clave == codigo {
				return tot
			}
		}
		return application.TotalValoracion{}
	}
	antes := deCuenta(svc.Valoracion(empDemo, ""), ctaInvAlterna)

	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 8, CostoUnitario: 25}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst)
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 8}}); err != nil {
		t.Fatalf("recibir: %v", err)
	}
	despues := deCuenta(svc.Valoracion(empDemo, ""), ctaInvAlterna)

	if got := round2Test(despues.Valor - antes.Valor); !casi(got, 200) {
		t.Errorf("el inventario de esa cuenta tenía que subir 200: %v", got)
	}
	if got := round2Test(despues.Contable - antes.Contable); !casi(got, 200) {
		t.Errorf("y el diario de esa cuenta, lo mismo: %v", got)
	}
	// La diferencia no puede haberse movido: si el inventario y el diario suben
	// distinto, el informe deja de servir para cuadrar nada.
	if got := round2Test(despues.Diferencia - antes.Diferencia); !casi(got, 0) {
		t.Errorf("la diferencia entre inventario y diario no podía cambiar: %v", got)
	}
	// Y la reclasificación existe: al asignar la cuenta, el inventario que el rubro
	// ya tenía se movió también en el diario. Sin eso, `antes.Diferencia` sería el
	// inventario previo entero y la función nacería descuadrada.
	if antes.Valor <= 200 {
		t.Fatalf("el seed trae inventario de este rubro: la cuenta tenía que arrastrarlo (%v)", antes.Valor)
	}
	if !casi(antes.Valor, antes.Contable) {
		t.Errorf("tras reclasificar, el inventario previo del rubro tenía que estar también en el diario: %v vs %v",
			antes.Valor, antes.Contable)
	}
}

// TestValoracion_PorSedeNoComparaConElDiario: el saldo de una cuenta contable es de
// la empresa entera. Contrastarlo con el inventario de UNA sede daría una
// diferencia que no significa nada — y que alguien intentaría cuadrar.
func TestValoracion_PorSedeNoComparaConElDiario(t *testing.T) {
	svc := servicioCompleto(t)
	sku := primerSKU(t, svc)
	svc.Ajustar(empDemo, sede1, "", sku, "carga", 5, actorA, origenTst)

	v := svc.Valoracion(empDemo, sede1)
	for _, tot := range v.PorCuenta {
		if tot.Contable != 0 || tot.Diferencia != 0 {
			t.Errorf("filtrando por sede no se compara con el diario: %+v", tot)
		}
	}
}

// TestValoracion_LoQueNoTieneAlmacenCuentaEnElPrincipal fija la convención que el
// resto del sistema ya seguía: un movimiento sin almacén es anterior a que hubiera
// almacenes, y su mercancía está en el principal de la sede.
//
// La valoración agrupaba por el AlmacenID literal, así que ese stock aparecía bajo
// «Sin almacén» mientras Existencias —que pasa por movsDeAlmacen— lo mostraba en el
// Almacén Principal. Dos pantallas, el mismo stock, dos respuestas; y como el total
// seguía cuadrando contra la contabilidad, nada fallaba.
func TestValoracion_LoQueNoTieneAlmacenCuentaEnElPrincipal(t *testing.T) {
	svc := servicioCompleto(t)
	sku := primerSKU(t, svc)
	// Ajuste SIN almacén: es como nace el histórico y como siembra el seed.
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "carga", 7, actorA, origenTst); err != nil {
		t.Fatalf("ajustar: %v", err)
	}
	prin := almacenPrincipalID(t, svc)

	// Se mira SOLO la sede que tiene almacén. El seed trae una segunda sede sin
	// almacenes, y ahí «Sin almacén» es la respuesta honesta: no hay dónde
	// atribuirlo. Lo que se prueba es la atribución, no que el seed esté completo.
	v := svc.Valoracion(empDemo, sede1)
	if len(v.Filas) == 0 {
		t.Fatal("el informe de la sede salió vacío")
	}
	for _, f := range v.Filas {
		if f.AlmacenID != prin {
			t.Errorf("%s quedó en almacén %q; tenía que atribuirse al principal (%s)", f.SKU, f.AlmacenID, prin)
		}
	}
	for _, tot := range v.PorAlmacen {
		if tot.Nombre == "Sin almacén" {
			t.Errorf("el desglose por almacén no debe traer «Sin almacén» habiendo principal: %+v", tot)
		}
	}
}

// TestValoracion_DosAlmacenesHomonimosNoSeFunden: el desglose agrupaba por NOMBRE,
// y como cada sede nace con su «Almacén Principal», dos sedes daban una sola fila
// que sumaba el stock de ambas — un almacén que no existe, con un valor que no es
// de nadie. Se agrupa por id, y el nombre lleva la sede cuando se repite.
func TestValoracion_DosAlmacenesHomonimosNoSeFunden(t *testing.T) {
	svc := servicioCompleto(t)
	sku := primerSKU(t, svc)
	// Stock en las dos sedes; cada una resuelve a su propio almacén principal.
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "carga", 4, actorA, origenTst); err != nil {
		t.Fatalf("ajustar sede1: %v", err)
	}
	if _, err := svc.Ajustar(empDemo, sede2, "", sku, "carga", 6, actorA, origenTst); err != nil {
		t.Fatalf("ajustar sede2: %v", err)
	}
	a1 := almacenPrincipalID(t, svc)
	a2, err := svc.AsegurarAlmacenPrincipal(empDemo, sede2, "sistema", origenTst)
	if err != nil {
		t.Fatalf("almacén de sede2: %v", err)
	}
	if a1 == a2.ID {
		t.Fatal("las dos sedes comparten almacén; el caso no se está probando")
	}

	v := svc.Valoracion(empDemo, "")
	claves := map[string]bool{}
	for _, tot := range v.PorAlmacen {
		if claves[tot.Clave] {
			t.Errorf("el almacén %q aparece dos veces en el desglose", tot.Clave)
		}
		claves[tot.Clave] = true
	}
	if !claves[a1] || !claves[a2.ID] {
		t.Errorf("faltan almacenes en el desglose: %v (esperaba %s y %s)", claves, a1, a2.ID)
	}
	// Y con el nombre repetido, la etiqueta tiene que distinguirlos.
	nombres := map[string]int{}
	for _, tot := range v.PorAlmacen {
		nombres[tot.Nombre]++
	}
	for n, veces := range nombres {
		if veces > 1 {
			t.Errorf("%d almacenes distintos se muestran como %q", veces, n)
		}
	}
}

// TestCorregirCosto_CambiaElValorNoLasUnidades es la prueba central de la
// corrección: mueve lo que vale, no cuánto hay.
func TestCorregirCosto_CambiaElValorNoLasUnidades(t *testing.T) {
	svc := servicioCompleto(t)
	sku := "COST-1"
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{SKU: sku, Nombre: "Cable", Precio: 50})

	oc, _ := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 10}},
	})
	svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst)
	svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst, []application.LineaRecepcion{{SKU: sku, Cantidad: 10}})

	cantAntes, avgAntes := existenciaDe(t, svc, empDemo, sede1, sku)
	if !casi(avgAntes, 10) {
		t.Fatalf("el costo de partida tenía que ser 10: %v", avgAntes)
	}

	out, err := svc.CorregirCosto(empDemo, sede1, sku, 14, "la factura venía en dólares", actorA, origenTst)
	if err != nil {
		t.Fatalf("corregir: %v", err)
	}
	if !casi(out.ValorAjustado, 40) {
		t.Errorf("subir de 10 a 14 sobre 10 unidades son 40: %v", out.ValorAjustado)
	}

	cantDespues, avgDespues := existenciaDe(t, svc, empDemo, sede1, sku)
	if !casi(cantDespues, cantAntes) {
		t.Errorf("corregir el costo NO mueve unidades: %v → %v", cantAntes, cantDespues)
	}
	if !casi(avgDespues, 14) {
		t.Errorf("el costo tenía que quedar en 14: %v", avgDespues)
	}
	// Y deja rastro propio: una corrección hecha por una persona no puede
	// confundirse con la revaluación automática de un costo en destino.
	hayAsiento := false
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefTipo == "correccion_costo" {
			hayAsiento = true
			debe, haber := 0.0, 0.0
			for _, l := range a.Lineas {
				debe += l.Debe
				haber += l.Haber
			}
			if !casi(debe, haber) || !casi(debe, 40) {
				t.Errorf("el asiento de la corrección tenía que ser de 40 y cuadrar: debe %v, haber %v", debe, haber)
			}
		}
	}
	if !hayAsiento {
		t.Error("la corrección tenía que derivar su asiento")
	}
}

// TestCorregirCosto_SinExistenciaSeNiega es la guarda que evita el fallo
// silencioso: el plegado ignora una revaluación cuando no hay saldo entre el que
// repartir el valor, así que aceptarla dejaría un movimiento que no hace nada —
// la pantalla diría «corregido» y el costo seguiría igual.
func TestCorregirCosto_SinExistenciaSeNiega(t *testing.T) {
	svc := servicioCompleto(t)
	sku := "COST-2"
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{SKU: sku, Nombre: "Sin stock", Precio: 10})

	if _, err := svc.CorregirCosto(empDemo, sede1, sku, 20, "prueba", actorA, origenTst); !errors.Is(err, application.ErrCorreccionSinExistencia) {
		t.Errorf("sin existencia la corrección debía negarse: %v", err)
	}
	if _, err := svc.CorregirCosto(empDemo, sede1, sku, 20, "", actorA, origenTst); !errors.Is(err, application.ErrMotivoRequerido) {
		t.Errorf("una corrección de costo sin motivo no se puede auditar: %v", err)
	}
	// Y con existencia, pedir el costo que ya tiene tampoco emite nada.
	svc.Ajustar(empDemo, sede1, "", sku, "carga", 5, actorA, origenTst)
	_, avg := existenciaDe(t, svc, empDemo, sede1, sku)
	if _, err := svc.CorregirCosto(empDemo, sede1, sku, avg, "sin cambio", actorA, origenTst); !errors.Is(err, application.ErrCorreccionSinCambio) {
		t.Errorf("corregir al mismo costo no es una corrección: %v", err)
	}
}

// TestConteo_SoloTocaLoQueSeDeclara: una hoja parcial es el caso normal. Suponer
// cero en lo no declarado convertiría cada conteo de un pasillo en una merma
// masiva del almacén entero.
func TestConteo_SoloTocaLoQueSeDeclara(t *testing.T) {
	svc := servicioCompleto(t)
	alm := almacenPrincipalID(t, svc)
	a01 := nuevaUbicacion(t, svc, alm, "A-01")
	b02 := nuevaUbicacion(t, svc, alm, "B-02")
	sku := primerSKU(t, svc)

	svc.AjustarEnUbicacion(empDemo, sede1, alm, a01, sku, "carga A", 10, "", "", actorA, origenTst)
	svc.AjustarEnUbicacion(empDemo, sede1, alm, b02, sku, "carga B", 7, "", "", actorA, origenTst)

	// Se cuenta SOLO A-01, y hay 8 en vez de 10.
	res, err := svc.AplicarConteo(empDemo, sede1, alm, "conteo del pasillo A",
		[]application.LineaConteo{{SKU: sku, UbicacionID: a01, Contado: 8}}, actorA, origenTst)
	if err != nil {
		t.Fatalf("conteo: %v", err)
	}
	if res.ConAjuste != 1 || res.ConError != 0 {
		t.Fatalf("tenía que ajustarse una línea sin errores: %+v", res)
	}
	if got := saldoEn(t, svc, sku, a01); !casi(got, 8) {
		t.Errorf("A-01 tenía que quedar en 8: %v", got)
	}
	// LO QUE IMPORTA: B-02 no se tocó.
	if got := saldoEn(t, svc, sku, b02); !casi(got, 7) {
		t.Errorf("B-02 no se contó, no podía cambiar: %v", got)
	}
}

// TestConteo_PrevisualizarNoTocaNada: un conteo mal tecleado es una merma masiva
// que ya no se puede deshacer. La vista previa es la única oportunidad de verlo.
func TestConteo_PrevisualizarNoTocaNada(t *testing.T) {
	svc := servicioCompleto(t)
	alm := almacenPrincipalID(t, svc)
	a01 := nuevaUbicacion(t, svc, alm, "A-01")
	sku := primerSKU(t, svc)
	svc.AjustarEnUbicacion(empDemo, sede1, alm, a01, sku, "carga", 500, "", "", actorA, origenTst)

	antes := saldoEn(t, svc, sku, a01)
	// Alguien teclea 5 donde hay 500.
	res, err := svc.PrevisualizarConteo(empDemo, sede1, alm, []application.LineaConteo{{SKU: sku, UbicacionID: a01, Contado: 5}})
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	if len(res.Lineas) != 1 || !casi(res.Lineas[0].Diferencia, -495) {
		t.Fatalf("la vista previa tenía que anunciar la merma de 495: %+v", res.Lineas)
	}
	if res.Lineas[0].Ajustado {
		t.Error("una previsualización no ajusta nada")
	}
	if got := saldoEn(t, svc, sku, a01); !casi(got, antes) {
		t.Errorf("la existencia no podía moverse: %v → %v", antes, got)
	}
}

// TestConteo_SeValidaTodoAntesDeEmitirNada: aplicar media hoja y fallar dejaría un
// almacén medio cuadrado, que es peor que uno descuadrado — porque parece correcto.
func TestConteo_SeValidaTodoAntesDeEmitirNada(t *testing.T) {
	svc := servicioCompleto(t)
	alm := almacenPrincipalID(t, svc)
	sku := primerSKU(t, svc)
	antes := existenciaDeSoloCantidad(t, svc, sku)

	_, err := svc.AplicarConteo(empDemo, sede1, alm, "hoja con un error", []application.LineaConteo{
		{SKU: sku, Contado: 3},
		{SKU: "NO-EXISTE", Contado: 1},
	}, actorA, origenTst)
	if !errors.Is(err, application.ErrProductoNoExiste) {
		t.Fatalf("un SKU inventado tenía que abortar la hoja entera: %v", err)
	}
	if got := existenciaDeSoloCantidad(t, svc, sku); !casi(got, antes) {
		t.Errorf("la línea buena tampoco podía aplicarse: %v → %v", antes, got)
	}
	// Y una cantidad negativa también aborta.
	if _, err := svc.AplicarConteo(empDemo, sede1, alm, "negativa",
		[]application.LineaConteo{{SKU: sku, Contado: -1}}, actorA, origenTst); !errors.Is(err, application.ErrConteoNegativo) {
		t.Errorf("contar en negativo debía rechazarse: %v", err)
	}
	if _, err := svc.AplicarConteo(empDemo, sede1, alm, "", nil, actorA, origenTst); err == nil {
		t.Error("un conteo sin motivo ni líneas debía rechazarse")
	}
}

// existenciaDeSoloCantidad devuelve solo la cantidad de un sku en la sede.
func existenciaDeSoloCantidad(t *testing.T, svc *application.Service, sku string) float64 {
	t.Helper()
	c, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	return c
}

// TestCuentasDeInventario_LasListaTodas: sin esta lista, quien mire el balance
// tendría que recordar de memoria en qué cuentas quedó repartido el inventario.
func TestCuentasDeInventario_LasListaTodas(t *testing.T) {
	svc := servicioCompleto(t)
	if got := svc.CuentasDeInventario(empDemo); len(got) != 1 || got[0] != contabilidad.CtaInventario {
		t.Fatalf("sin rubros configurados solo está la general: %v", got)
	}
	crearCuentaAlterna(t, svc)
	rubroConCuenta(t, svc, "Electrónica", ctaInvAlterna)
	got := svc.CuentasDeInventario(empDemo)
	if len(got) != 2 || got[0] != contabilidad.CtaInventario || got[1] != ctaInvAlterna {
		t.Errorf("tenían que salir las dos, ordenadas: %v", got)
	}
}
