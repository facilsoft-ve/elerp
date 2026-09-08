package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/listaprecio"
)

// servicioConListas arma el servicio del seed demo con el maestro de listas de
// precio cableado (nuevoServicio no lo cablea, igual que cmd/api lo hace aparte).
func servicioConListas(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConListasPrecio(st.ListasPrecio)
	return svc
}

func TestListasPrecio_SeedYFiltroPorTipo(t *testing.T) {
	svc := servicioConListas(t)

	venta := svc.ListasPrecio(empDemo, listaprecio.TipoVenta)
	if len(venta) == 0 {
		t.Fatalf("el seed demo debía traer al menos una lista de venta")
	}
	for _, l := range venta {
		if l.Tipo != listaprecio.TipoVenta {
			t.Errorf("filtro por venta trajo una lista de tipo %q", l.Tipo)
		}
	}
	compra := svc.ListasPrecio(empDemo, listaprecio.TipoCompra)
	for _, l := range compra {
		if l.Tipo != listaprecio.TipoCompra {
			t.Errorf("filtro por compra trajo una lista de tipo %q", l.Tipo)
		}
	}
	// Sin filtro devuelve ambas.
	if todas := svc.ListasPrecio(empDemo, ""); len(todas) != len(venta)+len(compra) {
		t.Errorf("sin filtro esperaba %d listas, se obtuvo %d", len(venta)+len(compra), len(todas))
	}
}

func TestCrearListaPrecio_ValidaNombreYTipo(t *testing.T) {
	svc := servicioConListas(t)

	if _, err := svc.CrearListaPrecio(empDemo, actorA, origenTst, application.EntradaListaPrecio{Nombre: " ", Tipo: listaprecio.TipoVenta}); err == nil {
		t.Errorf("un nombre vacío debía fallar")
	}
	if _, err := svc.CrearListaPrecio(empDemo, actorA, origenTst, application.EntradaListaPrecio{Nombre: "X", Tipo: "otro"}); !errors.Is(err, application.ErrTipoListaInvalido) {
		t.Errorf("un tipo inválido debía dar ErrTipoListaInvalido, se obtuvo %v", err)
	}
	if _, err := svc.CrearListaPrecio(empDemo, actorA, origenTst, application.EntradaListaPrecio{
		Nombre: "Neg", Tipo: listaprecio.TipoVenta, Items: []listaprecio.ItemLista{{SKU: "ARR-001", Precio: -1}},
	}); err == nil {
		t.Errorf("un precio negativo debía fallar")
	}
}

func TestCrearListaPrecio_NormalizaItemsYDefaultMoneda(t *testing.T) {
	svc := servicioConListas(t)

	out, err := svc.CrearListaPrecio(empDemo, actorA, origenTst, application.EntradaListaPrecio{
		Nombre: "Especial", Tipo: listaprecio.TipoVenta, Activa: true,
		Items: []listaprecio.ItemLista{
			{SKU: "  ARR-001 ", Precio: 40},
			{SKU: "", Precio: 99},        // se descarta (sin SKU)
			{SKU: "ARR-001", Precio: 42}, // dedup: gana el último
		},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if out.Moneda != "VES" {
		t.Errorf("sin moneda debía quedar en VES, quedó %q", out.Moneda)
	}
	if len(out.Items) != 1 {
		t.Fatalf("los ítems debían quedar en 1 (descarte + dedup), quedaron %d", len(out.Items))
	}
	if out.Items[0].SKU != "ARR-001" || out.Items[0].Precio != 42 {
		t.Errorf("el ítem normalizado esperaba ARR-001=42, se obtuvo %s=%v", out.Items[0].SKU, out.Items[0].Precio)
	}
}

func TestActualizarListaPrecio_EditaYConservaTipo(t *testing.T) {
	svc := servicioConListas(t)
	creada, err := svc.CrearListaPrecio(empDemo, actorA, origenTst, application.EntradaListaPrecio{
		Nombre: "Base", Tipo: listaprecio.TipoVenta, Activa: true,
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}

	out, err := svc.ActualizarListaPrecio(empDemo, creada.ID, actorA, origenTst, application.EntradaListaPrecio{
		Nombre: "Renombrada", Tipo: listaprecio.TipoCompra, Activa: false, Moneda: "USD",
		Items: []listaprecio.ItemLista{{SKU: "AZU-001", Precio: 3.5}},
	})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.Nombre != "Renombrada" || out.Activa != false || out.Moneda != "USD" {
		t.Errorf("edición no aplicada: %+v", out)
	}
	// El tipo NO se muta desde la edición: sigue siendo de venta.
	if out.Tipo != listaprecio.TipoVenta {
		t.Errorf("el tipo debía conservarse en venta, quedó %q", out.Tipo)
	}
	if len(out.Items) != 1 || out.Items[0].SKU != "AZU-001" {
		t.Errorf("los ítems debían actualizarse a [AZU-001], se obtuvo %+v", out.Items)
	}

	// Actualizar una lista inexistente da ErrListaPrecioNoExiste.
	if _, err := svc.ActualizarListaPrecio(empDemo, "lp_inexistente", actorA, origenTst, application.EntradaListaPrecio{Nombre: "x"}); !errors.Is(err, application.ErrListaPrecioNoExiste) {
		t.Errorf("una lista inexistente debía dar ErrListaPrecioNoExiste, se obtuvo %v", err)
	}
}
