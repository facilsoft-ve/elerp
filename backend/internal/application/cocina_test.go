package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// servicioComanderas monta el salón con sus comanderas.
func servicioComanderas(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	svc, st := servicioSalon(t)
	svc.ConImpresoras(st.Impresoras)
	return svc, st
}

// La demo del restaurante trae tres puestos precargados y coherentes: uno solo
// predeterminado y los rubros del catálogo repartidos.
func TestComanderas_DemoPrecargada(t *testing.T) {
	svc, _ := servicioComanderas(t)
	imps := svc.Impresoras(empSalon, sedeSalon)
	if len(imps) != 3 {
		t.Fatalf("la demo debe traer 3 comanderas (cocina, barra, postres), trae %d", len(imps))
	}
	predeterminadas := 0
	nombres := map[string]bool{}
	for _, i := range imps {
		nombres[i.Nombre] = true
		if i.Predeterminada {
			predeterminadas++
		}
		if i.AnchoMM != 80 {
			t.Errorf("%s: ancho esperado 80 mm, es %d", i.Nombre, i.AnchoMM)
		}
	}
	if predeterminadas != 1 {
		t.Errorf("debe haber EXACTAMENTE una predeterminada, hay %d", predeterminadas)
	}
	for _, n := range []string{"Cocina", "Barra", "Postres"} {
		if !nombres[n] {
			t.Errorf("falta la comandera %q", n)
		}
	}
}

// El ruteo: cada renglón sale por la comandera de SU rubro. Es lo que evita que la barra
// lea comandas de cocina y viceversa.
func TestComanderas_CadaProductoPorSuComandera(t *testing.T) {
	svc, st := servicioComanderas(t)
	m := mesaPorNombre(t, st, "3")
	c, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
		MesoneroID: "usr_meso", MesoneroNombre: "Meso", RolActor: usuario.RolMesonero,
		Actor: "usr_meso", Origen: origenTst, Comensales: 2,
	})
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	// Un plato (rubro Cocina), una bebida (Bebidas) y un postre (Postres).
	if _, err := svc.AgregarItems(empSalon, c.ID, "usr_meso", usuario.RolMesonero, origenTst, []application.ItemInput{
		{SKU: "PLA-MILANESA", Nombre: "Milanesa", Cantidad: 1, PrecioUnitario: 34000},
		{SKU: "BEB-CERVEZA", Nombre: "Cerveza", Cantidad: 2, PrecioUnitario: 3200},
		{SKU: "POS-TORTA", Nombre: "Torta", Cantidad: 1, PrecioUnitario: 8500},
	}); err != nil {
		t.Fatalf("agregar: %v", err)
	}

	_, tickets, err := svc.EnviarACocina(empSalon, c.ID, "usr_meso", origenTst)
	if err != nil {
		t.Fatalf("enviar a cocina: %v", err)
	}
	if len(tickets) != 3 {
		t.Fatalf("se esperaban 3 tickets (uno por puesto), hay %d", len(tickets))
	}
	porPuesto := map[string][]string{}
	for _, tk := range tickets {
		for _, it := range tk.Items {
			porPuesto[tk.Impresora.Nombre] = append(porPuesto[tk.Impresora.Nombre], it.SKU)
		}
	}
	esperado := map[string]string{"Cocina": "PLA-MILANESA", "Barra": "BEB-CERVEZA", "Postres": "POS-TORTA"}
	for puesto, sku := range esperado {
		skus := porPuesto[puesto]
		if len(skus) != 1 || skus[0] != sku {
			t.Errorf("la comandera %q debía recibir solo %s, recibió %v", puesto, sku, skus)
		}
	}
}

// Un producto de un rubro que NINGUNA comandera reclama va a la predeterminada: si no,
// el pedido no se imprimiría en ninguna parte y se perdería en silencio.
func TestComanderas_LoNoReclamadoVaALaPredeterminada(t *testing.T) {
	svc, st := servicioComanderas(t)
	// Producto de un rubro que NINGUNA comandera reclama.
	if _, err := svc.CrearProducto(empSalon, actorA, origenTst, inventario.Producto{
		SKU: "CAF-001", Nombre: "Café negro", Rubro: "Cafetería",
		UnidadBase: "unidad", TipoVenta: inventario.TipoVentaUnidad,
		Precio: 5000, Activo: true,
	}); err != nil {
		t.Fatalf("crear producto: %v", err)
	}
	m := mesaPorNombre(t, st, "3")
	c, _ := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
		MesoneroID: "usr_meso", MesoneroNombre: "Meso", RolActor: usuario.RolMesonero,
		Actor: "usr_meso", Origen: origenTst,
	})
	if _, err := svc.AgregarItems(empSalon, c.ID, "usr_meso", usuario.RolMesonero, origenTst,
		[]application.ItemInput{{SKU: "CAF-001", Nombre: "Café", Cantidad: 1, PrecioUnitario: 5000}}); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	_, tickets, err := svc.EnviarACocina(empSalon, c.ID, "usr_meso", origenTst)
	if err != nil {
		t.Fatalf("enviar: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("se esperaba 1 ticket, hay %d", len(tickets))
	}
	if !tickets[0].Impresora.Predeterminada {
		t.Errorf("un rubro sin comandera debe caer en la PREDETERMINADA, cayó en %q", tickets[0].Impresora.Nombre)
	}
}

// Sin comanderas configuradas la comanda igual se genera (se ve en pantalla): es como se
// opera mientras se configura el local.
func TestComanderas_SinConfigurarSigueSaliendoLaComanda(t *testing.T) {
	svc, st := servicioComanderas(t)
	for _, imp := range svc.Impresoras(empSalon, sedeSalon) {
		if err := svc.EliminarImpresora(empSalon, sedeSalon, imp.ID, actorA, origenTst); err != nil {
			t.Fatalf("eliminar comandera: %v", err)
		}
	}
	m := mesaPorNombre(t, st, "3")
	c, _ := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
		MesoneroID: "usr_meso", MesoneroNombre: "Meso", RolActor: usuario.RolMesonero,
		Actor: "usr_meso", Origen: origenTst,
	})
	if _, err := svc.AgregarItems(empSalon, c.ID, "usr_meso", usuario.RolMesonero, origenTst,
		[]application.ItemInput{{SKU: "PLA-MILANESA", Nombre: "Milanesa", Cantidad: 1, PrecioUnitario: 34000}}); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	_, tickets, err := svc.EnviarACocina(empSalon, c.ID, "usr_meso", origenTst)
	if err != nil {
		t.Fatalf("enviar sin comanderas debe funcionar: %v", err)
	}
	if len(tickets) != 1 || len(tickets[0].Items) != 1 {
		t.Fatalf("se esperaba un ticket con el renglón, se obtuvo %+v", tickets)
	}
}

// ============ COMANDERA FIJADA EN EL PRODUCTO (gana sobre el rubro) ============
//
// Un postre con receta y un plato de cocina son los dos "platos", pero se preparan en
// puestos distintos. Y dos productos del mismo rubro pueden ir a comanderas distintas.
// Por eso el producto puede fijar SU comandera, y esa manda sobre el rubro.

// comanderaPorNombre resuelve el id de un puesto sembrado del restaurante demo.
func comanderaPorNombre(t *testing.T, svc *application.Service, nombre string) string {
	t.Helper()
	for _, i := range svc.Impresoras(empSalon, sedeSalon) {
		if i.Nombre == nombre {
			return i.ID
		}
	}
	t.Fatalf("no se encontró la comandera %q", nombre)
	return ""
}

func cuentaConItems(t *testing.T, svc *application.Service, st *inmem.Store, mesaNombre string, items []application.ItemInput) string {
	t.Helper()
	m := mesaPorNombre(t, st, mesaNombre)
	c, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
		MesoneroID: "usr_meso", MesoneroNombre: "Meso", RolActor: usuario.RolMesonero,
		Actor: "usr_meso", Origen: origenTst, Comensales: 2,
	})
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	if _, err := svc.AgregarItems(empSalon, c.ID, "usr_meso", usuario.RolMesonero, origenTst, items); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	return c.ID
}

// El plato sale por la comandera que tiene fijada, aunque su rubro apunte a otra.
func TestComanderas_LaDelProductoGanaSobreElRubro(t *testing.T) {
	svc, st := servicioComanderas(t)
	postres := comanderaPorNombre(t, svc, "Postres")
	// La milanesa es de rubro Cocina, pero se le fija la barra de postres.
	if _, err := svc.ActualizarProducto(empSalon, "usr_admin", origenTst, "PLA-MILANESA",
		application.CambiosProducto{ComanderaID: &postres}); err != nil {
		t.Fatalf("fijar comandera: %v", err)
	}
	id := cuentaConItems(t, svc, st, "3", []application.ItemInput{
		{SKU: "PLA-MILANESA", Nombre: "Milanesa", Cantidad: 1, PrecioUnitario: 34000},
	})
	_, tickets, err := svc.EnviarACocina(empSalon, id, "usr_meso", origenTst)
	if err != nil {
		t.Fatalf("enviar: %v", err)
	}
	if len(tickets) != 1 || tickets[0].Impresora.Nombre != "Postres" {
		t.Fatalf("debía salir por Postres, salió por %+v", tickets)
	}
}

// Vaciar la elección devuelve el producto al ruteo por rubro.
func TestComanderas_VaciarLaEleccionVuelveAlRubro(t *testing.T) {
	svc, st := servicioComanderas(t)
	postres := comanderaPorNombre(t, svc, "Postres")
	vacio := ""
	if _, err := svc.ActualizarProducto(empSalon, "usr_admin", origenTst, "PLA-MILANESA",
		application.CambiosProducto{ComanderaID: &postres}); err != nil {
		t.Fatalf("fijar: %v", err)
	}
	if _, err := svc.ActualizarProducto(empSalon, "usr_admin", origenTst, "PLA-MILANESA",
		application.CambiosProducto{ComanderaID: &vacio}); err != nil {
		t.Fatalf("vaciar: %v", err)
	}
	id := cuentaConItems(t, svc, st, "3", []application.ItemInput{
		{SKU: "PLA-MILANESA", Nombre: "Milanesa", Cantidad: 1, PrecioUnitario: 34000},
	})
	_, tickets, err := svc.EnviarACocina(empSalon, id, "usr_meso", origenTst)
	if err != nil {
		t.Fatalf("enviar: %v", err)
	}
	if len(tickets) != 1 || tickets[0].Impresora.Nombre != "Cocina" {
		t.Fatalf("sin comandera fijada debía volver a su rubro (Cocina), salió por %+v", tickets)
	}
}

// Una comandera fijada que ya no existe (se borró el puesto) NO puede tragarse el
// renglón en silencio: el producto vuelve a rutearse como cualquier otro, por su rubro,
// y solo si tampoco encaja ahí cae a la predeterminada.
func TestComanderas_FijadaQueYaNoExisteVuelveAlRubro(t *testing.T) {
	svc, st := servicioComanderas(t)
	fantasma := "imp_borrada"
	if _, err := svc.ActualizarProducto(empSalon, "usr_admin", origenTst, "BEB-CERVEZA",
		application.CambiosProducto{ComanderaID: &fantasma}); err != nil {
		t.Fatalf("fijar: %v", err)
	}
	id := cuentaConItems(t, svc, st, "3", []application.ItemInput{
		{SKU: "BEB-CERVEZA", Nombre: "Cerveza", Cantidad: 1, PrecioUnitario: 3200},
	})
	_, tickets, err := svc.EnviarACocina(empSalon, id, "usr_meso", origenTst)
	if err != nil {
		t.Fatalf("enviar: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("se esperaba 1 ticket, hay %d", len(tickets))
	}
	if tickets[0].Impresora.Nombre != "Barra" {
		t.Errorf("debía volver al ruteo por rubro (la cerveza es de Bebidas ⇒ Barra), cayó en %q", tickets[0].Impresora.Nombre)
	}
}

// La demo tiene que enseñar que un POSTRE también lleva receta: cargarlos todos como
// producto de reventa hacía creer que "plato con receta" es solo comida de cocina.
func TestDemoRestaurante_LosPostresDeLaCasaSonRecetas(t *testing.T) {
	svc, _ := servicioComanderas(t)
	porSKU := map[string]inventario.Producto{}
	for _, p := range svc.Productos(empSalon) {
		porSKU[p.SKU] = p
	}
	torta, ok := porSKU["POS-TORTA"]
	if !ok {
		t.Fatal("la demo debería traer la torta de chocolate")
	}
	if !torta.EsPlato || len(torta.Receta) == 0 {
		t.Errorf("la torta debe ser un plato con receta, es EsPlato=%v receta=%d", torta.EsPlato, len(torta.Receta))
	}
	if torta.Rubro != "Postres" {
		t.Errorf("el postre debe quedar en el rubro Postres, quedó en %q", torta.Rubro)
	}
	if torta.ComanderaID == "" {
		t.Error("el postre debe salir por la comandera de postres, no por la de su rubro por casualidad")
	}
	// Y sigue existiendo un postre de REVENTA: no todo postre se prepara.
	helado, ok := porSKU["POS-HELADO"]
	if !ok {
		t.Fatal("la demo debería traer un postre comprado hecho")
	}
	if helado.EsPlato {
		t.Error("un postre comprado hecho no lleva receta")
	}
}

/* --- Catálogo de comanderas ----------------------------------------------- */

// Elegir un modelo del catálogo tiene que dejar la ficha resuelta: la grafía
// canónica (para que el maestro no se llene de variantes de lo mismo) y el ancho
// de papel del equipo, que es lo que nadie en la cocina sabe de memoria.
func TestComanderas_ModeloDelCatalogoPrecargaElAncho(t *testing.T) {
	svc, _ := servicioComanderas(t)
	imp, err := svc.GuardarImpresora(empSalon, sedeSalon, actorA, origenTst, application.ImpresoraBody{
		Nombre: "Barra", Marca: "xprinter", Modelo: "xp-58iih",
		Conexion: "local", Activa: true,
	})
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if imp.Marca != "Xprinter" || imp.Modelo != "XP-58IIH" {
		t.Errorf("no normalizó a la grafía del catálogo: %q / %q", imp.Marca, imp.Modelo)
	}
	if imp.AnchoMM != 58 {
		t.Errorf("la XP-58IIH es de 58 mm, quedó en %d", imp.AnchoMM)
	}
}

// El ancho elegido a mano gana sobre el del catálogo: hay quien le mete rollo de
// 58 a un equipo de 80 y la comanda tiene que salir como el local la imprime.
func TestComanderas_AnchoExplicitoGanaAlCatalogo(t *testing.T) {
	svc, _ := servicioComanderas(t)
	imp, err := svc.GuardarImpresora(empSalon, sedeSalon, actorA, origenTst, application.ImpresoraBody{
		Nombre: "Postres", Marca: "Epson", Modelo: "TM-T20III",
		AnchoMM: 58, Conexion: "local", Activa: true,
	})
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if imp.AnchoMM != 58 {
		t.Errorf("el catálogo pisó el ancho elegido: %d", imp.AnchoMM)
	}
}

// Una comandera sin marca sigue siendo válida: el catálogo ayuda, no obliga.
func TestComanderas_SinMarcaSigueSiendoValida(t *testing.T) {
	svc, _ := servicioComanderas(t)
	imp, err := svc.GuardarImpresora(empSalon, sedeSalon, actorA, origenTst, application.ImpresoraBody{
		Nombre: "Plancha", Conexion: "local", Activa: true,
	})
	if err != nil {
		t.Fatalf("una comandera sin marca debe aceptarse: %v", err)
	}
	if imp.AnchoMM != 80 {
		t.Errorf("sin modelo, el ancho por defecto debe seguir siendo 80 mm, fue %d", imp.AnchoMM)
	}
}
