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
