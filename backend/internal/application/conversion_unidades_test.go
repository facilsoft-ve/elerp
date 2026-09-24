package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/unidadmedida"
)

/* CONVERSIÓN ENTRE UNIDADES.
 *
 * Lo que estas pruebas cuidan: que al ledger entre SIEMPRE la unidad del producto.
 * Un ledger con unidades mezcladas suma sacos con kilos y el saldo deja de
 * significar nada — y no falla: da un número. */

// servicioParaConvertir reutiliza el montaje de unidades que ya existe y le añade
// los almacenes, que la recepción necesita.
func servicioParaConvertir(t *testing.T) *application.Service {
	t.Helper()
	svc, st := servicioConUnidades(t)
	svc.ConAlmacenes(st.Almacenes)
	return svc
}

// crearUnidadTest da de alta una unidad con su factor.
func crearUnidadTest(t *testing.T, svc *application.Service, simbolo, nombre, categoria string, factor float64) unidadmedida.UnidadMedida {
	t.Helper()
	u, err := svc.CrearUnidad(empDemo, actorA, origenTst, unidadmedida.UnidadMedida{
		Simbolo: simbolo, Nombre: nombre, Categoria: categoria, Factor: factor, Activa: true,
	})
	if err != nil {
		t.Fatalf("crear unidad %s: %v", simbolo, err)
	}
	return u
}

// TestConversion_LaCuentaSale: lo básico, en las dos direcciones y con el caso que
// más se usa — recibir en una unidad mayor que la del anaquel.
func TestConversion_LaCuentaSale(t *testing.T) {
	svc := servicioParaConvertir(t)
	casos := []struct {
		desde, hacia string
		cantidad     float64
		espera       float64
	}{
		{"kg", "g", 2, 2000},
		{"g", "kg", 500, 0.5},
		{"L", "mL", 1.5, 1500},
		{"docena", "unidad", 3, 36},
		{"unidad", "docena", 24, 2},
		{"m", "cm", 2, 200},
		// La misma unidad, o sin declarar: pasa tal cual. Es lo que hace que todo lo
		// anterior a esta función siga funcionando.
		{"kg", "kg", 7, 7},
		{"", "kg", 7, 7},
		{"kg", "", 7, 7},
	}
	for _, c := range casos {
		got, err := svc.ConvertirCantidad(empDemo, c.desde, c.hacia, c.cantidad)
		if err != nil {
			t.Errorf("%v %s → %s: %v", c.cantidad, c.desde, c.hacia, err)
			continue
		}
		if !casi(got, c.espera) {
			t.Errorf("%v %s → %s: esperaba %v, dio %v", c.cantidad, c.desde, c.hacia, c.espera, got)
		}
	}
}

// TestConversion_NoSeConvierteLoQueNoSePuede es la guarda que importa.
//
// Pasar kilos a litros exige la densidad del producto, que no está en ninguna
// parte. Aceptarlo con cualquier factor daría una existencia PLAUSIBLE Y FALSA —
// el peor resultado posible, porque nadie la mira dos veces.
func TestConversion_NoSeConvierteLoQueNoSePuede(t *testing.T) {
	svc := servicioParaConvertir(t)

	if _, err := svc.ConvertirCantidad(empDemo, "kg", "L", 5); !errors.Is(err, application.ErrUnidadesIncompatibles) {
		t.Errorf("peso a volumen tenía que negarse: %v", err)
	}
	if _, err := svc.ConvertirCantidad(empDemo, "unidad", "kg", 5); !errors.Is(err, application.ErrUnidadesIncompatibles) {
		t.Errorf("conteo a peso tenía que negarse: %v", err)
	}
	if _, err := svc.ConvertirCantidad(empDemo, "kg", "quintal", 5); !errors.Is(err, application.ErrUnidadNoExiste) {
		t.Errorf("una unidad inventada tenía que negarse: %v", err)
	}
	// «caja» existe y es de conteo, pero NO declara factor: son las que quepan, y
	// eso depende del producto. Convertir con ella daría un número inventado.
	if _, err := svc.ConvertirCantidad(empDemo, "caja", "unidad", 3); !errors.Is(err, application.ErrUnidadSinFactor) {
		t.Errorf("una caja no tiene equivalencia universal: %v", err)
	}
}

// TestConversion_AlLedgerEntraLaUnidadDelProducto es la prueba del ciclo real: se
// reciben 2 sacos y el almacén cuenta 100 kg.
func TestConversion_AlLedgerEntraLaUnidadDelProducto(t *testing.T) {
	svc := servicioParaConvertir(t)

	// Un saco de 50 kg, declarado en el maestro.
	crearUnidadTest(t, svc, "saco50", "Saco de 50 kg", "peso", 50)

	sku := "HARINA-SACO"
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: sku, Nombre: "Harina a granel", Precio: 40, UnidadBase: "kg",
	}); err != nil {
		t.Fatalf("crear producto: %v", err)
	}

	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 100, CostoUnitario: 20}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst)

	// Se reciben DOS SACOS. Al ledger tienen que entrar 100 kg.
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 2, Unidad: "saco50"}}); err != nil {
		t.Fatalf("recibir en sacos: %v", err)
	}
	cant, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	if !casi(cant, 100) {
		t.Fatalf("dos sacos de 50 son 100 kg en el anaquel, hay %v", cant)
	}
	// Y el movimiento quedó en kg, no en sacos: si guardara sacos, el pliegue
	// sumaría sacos con kilos en la próxima entrada a granel.
	for _, m := range svc.Movimientos(empDemo, inventario.FiltroMovimiento{SedeID: sede1}, "", "", "") {
		if m.SKU == sku && !casi(m.Cantidad, 100) {
			t.Errorf("el ledger tenía que guardar 100 (kg), guardó %v", m.Cantidad)
		}
	}
}

// TestConversion_UnaUnidadImposibleNoRecibeNada: si la conversión falla, NO se
// recibe. Media recepción convertida sería una existencia falsa que nadie descubre
// hasta el conteo físico.
func TestConversion_UnaUnidadImposibleNoRecibeNada(t *testing.T) {
	svc := servicioParaConvertir(t)
	sku := "HARINA-2"
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: sku, Nombre: "Harina", Precio: 40, UnidadBase: "kg",
	})
	oc, _ := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 100, CostoUnitario: 20}},
	})
	svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst)

	antes, _ := existenciaDe(t, svc, empDemo, sede1, sku)
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 5, Unidad: "L"}}); err == nil {
		t.Fatal("recibir litros de un producto que se lleva en kilos debía negarse")
	}
	if got, _ := existenciaDe(t, svc, empDemo, sede1, sku); !casi(got, antes) {
		t.Errorf("una recepción negada no puede dejar existencia: %v → %v", antes, got)
	}
}

// TestConversion_ElBackfillRellenaSoloLoQueFalta cubre el fallo que solo apareció
// en producción: el campo nació después que los datos, así que las empresas que ya
// existían tenían sus unidades sin factor y la conversión quedaba viva y VACÍA —
// el selector no aparecía nunca y nada fallaba.
func TestConversion_ElBackfillRellenaSoloLoQueFalta(t *testing.T) {
	svc, st := servicioConUnidades(t)

	// Se simula el estado de una empresa anterior al campo: todas sin equivalencia.
	for _, u := range svc.Unidades(empDemo) {
		u.Factor = 0
		st.Unidades.Update(u)
	}
	if n := len(svc.UnidadesCompatiblesCon(empDemo, "kg")); n != 0 {
		t.Fatalf("de partida no se puede convertir nada: %d", n)
	}

	if n := svc.AsegurarFactoresDeUnidades(empDemo, "sistema", "prueba"); n == 0 {
		t.Fatal("tenía que rellenar las equivalencias del juego por defecto")
	}
	compat := map[string]bool{}
	for _, u := range svc.UnidadesCompatiblesCon(empDemo, "kg") {
		compat[u.Simbolo] = true
	}
	if !compat["kg"] || !compat["g"] {
		t.Errorf("kg y g tenían que quedar convertibles: %v", compat)
	}
	// «caja» sigue sin factor: son las que quepan, y eso depende del producto.
	if _, err := svc.ConvertirCantidad(empDemo, "caja", "unidad", 1); !errors.Is(err, application.ErrUnidadSinFactor) {
		t.Errorf("«caja» no tiene equivalencia universal, el backfill no se la inventa: %v", err)
	}

	// SEGUNDA PASADA: no toca nada. Y si alguien pone un factor a cero a propósito
	// —«esta unidad no se convierte»— no se lo deshace en el siguiente arranque.
	if n := svc.AsegurarFactoresDeUnidades(empDemo, "sistema", "prueba"); n != 0 {
		t.Errorf("con equivalencias ya declaradas no vuelve a intervenir: %d", n)
	}
	for _, u := range svc.Unidades(empDemo) {
		if u.Simbolo == "g" {
			u.Factor = 0
			st.Unidades.Update(u)
		}
	}
	if n := svc.AsegurarFactoresDeUnidades(empDemo, "sistema", "prueba"); n != 0 {
		t.Errorf("un cero deliberado no se deshace solo: %d", n)
	}
}

// TestConversion_SoloSeOfreceLoQueSePuede: la pantalla no debe ofrecer una
// conversión que después se va a rechazar.
func TestConversion_SoloSeOfreceLoQueSePuede(t *testing.T) {
	svc := servicioParaConvertir(t)
	crearUnidadTest(t, svc, "saco50", "Saco de 50 kg", "peso", 50)

	compatibles := svc.UnidadesCompatiblesCon(empDemo, "kg")
	simbolos := map[string]bool{}
	for _, u := range compatibles {
		simbolos[u.Simbolo] = true
	}
	for _, esperado := range []string{"kg", "g", "saco50"} {
		if !simbolos[esperado] {
			t.Errorf("%s es de peso y tiene factor: tenía que ofrecerse", esperado)
		}
	}
	for _, prohibido := range []string{"L", "unidad", "caja"} {
		if simbolos[prohibido] {
			t.Errorf("%s no se puede convertir a kg: no debía ofrecerse", prohibido)
		}
	}
	// Un producto cuya unidad no tiene factor no admite ninguna conversión.
	if n := len(svc.UnidadesCompatiblesCon(empDemo, "caja")); n != 0 {
		t.Errorf("«caja» no declara equivalencia: no admite conversiones, se ofrecieron %d", n)
	}
}
