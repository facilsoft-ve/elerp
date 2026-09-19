package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/listaprecio"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

/* Modalidad de compra: adjudicación directa, licitación o lista de precios.
 *
 * Lo que se prueba acá no es que el campo se guarde —eso sería una etiqueta— sino
 * que CAMBIE lo que la solicitud exige. Una compra tiene que poder auditarse: quien
 * la revise debe ver si hubo concurso, y el documento no puede decir lo contrario
 * de lo que hizo. */

// solicitudCon arma una solicitud en borrador con una modalidad y unos proveedores.
func solicitudCon(t *testing.T, svc *application.Service, sku, modalidad string, provs ...string) (compra.SolicitudCompra, error) {
	t.Helper()
	return svc.CrearSolicitud(empDemo, sede1, actorA, origenTst, application.EntradaSolicitud{
		SedeID: sede1, Modalidad: modalidad,
		Lineas:      []application.LineaSolicitudEntrada{{SKU: sku, Cantidad: 5}},
		Proveedores: provs,
	})
}

// dosProveedores da de alta dos proveedores nuevos para comparar ofertas.
func dosProveedores(t *testing.T, svc *application.Service) (string, string) {
	t.Helper()
	a, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{Nombre: "Oferente A, C.A."})
	if err != nil {
		t.Fatalf("crear proveedor A: %v", err)
	}
	b, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{Nombre: "Oferente B, C.A."})
	if err != nil {
		t.Fatalf("crear proveedor B: %v", err)
	}
	return a.ID, b.ID
}

// TestModalidad_LicitacionExigeVariosProveedores: una licitación de uno solo no es
// una licitación.
func TestModalidad_LicitacionExigeVariosProveedores(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	a, b := dosProveedores(t, svc)

	if _, err := solicitudCon(t, svc, sku, compra.ModalidadLicitacion, a); !errors.Is(err, application.ErrLicitacionUnProveedor) {
		t.Fatalf("una licitación con un solo proveedor debía rechazarse, se obtuvo: %v", err)
	}
	if _, err := solicitudCon(t, svc, sku, compra.ModalidadLicitacion, a, b); err != nil {
		t.Fatalf("una licitación con dos proveedores es válida: %v", err)
	}
}

// TestModalidad_DirectaEsAUnSoloProveedor: si se le pide a varios hubo concurso.
func TestModalidad_DirectaEsAUnSoloProveedor(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	a, b := dosProveedores(t, svc)

	if _, err := solicitudCon(t, svc, sku, compra.ModalidadDirecta, a, b); !errors.Is(err, application.ErrDirectaVariosProveedores) {
		t.Fatalf("una adjudicación directa a dos proveedores debía rechazarse, se obtuvo: %v", err)
	}
	if _, err := solicitudCon(t, svc, sku, compra.ModalidadDirecta, a); err != nil {
		t.Fatalf("una adjudicación directa a uno es válida: %v", err)
	}
}

// TestModalidad_VaciaSeLeeComoDirecta: las solicitudes anteriores a esta
// funcionalidad no necesitan migración.
func TestModalidad_VaciaSeLeeComoDirecta(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	a, _ := dosProveedores(t, svc)

	sol, err := solicitudCon(t, svc, sku, "", a)
	if err != nil {
		t.Fatalf("sin modalidad debe seguir funcionando: %v", err)
	}
	if sol.Modalidad != compra.ModalidadDirecta {
		t.Errorf("una solicitud sin modalidad se lee como directa, quedó %q", sol.Modalidad)
	}
}

// TestModalidad_ElControlDuroEsAlEnviar: en borrador se está armando, así que los
// proveedores son opcionales; al enviar ya no hay excusa.
func TestModalidad_ElControlDuroEsAlEnviar(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	a, b := dosProveedores(t, svc)

	// Licitación sin proveedores todavía: se admite en borrador.
	sol, err := solicitudCon(t, svc, sku, compra.ModalidadLicitacion)
	if err != nil {
		t.Fatalf("una licitación en borrador sin proveedores debe admitirse: %v", err)
	}
	// Se le agrega UNO y se intenta enviar: ahí sí muerde.
	if _, err := svc.ActualizarSolicitud(empDemo, sol.ID, actorA, origenTst, application.EntradaSolicitud{
		SedeID: sede1, Modalidad: compra.ModalidadLicitacion,
		Lineas:      []application.LineaSolicitudEntrada{{SKU: sku, Cantidad: 5}},
		Proveedores: []string{a},
	}); !errors.Is(err, application.ErrLicitacionUnProveedor) {
		t.Fatalf("editar a un solo proveedor en licitación debía rechazarse, se obtuvo: %v", err)
	}
	// Con dos, se envía.
	if _, err := svc.ActualizarSolicitud(empDemo, sol.ID, actorA, origenTst, application.EntradaSolicitud{
		SedeID: sede1, Modalidad: compra.ModalidadLicitacion,
		Lineas:      []application.LineaSolicitudEntrada{{SKU: sku, Cantidad: 5}},
		Proveedores: []string{a, b},
	}); err != nil {
		t.Fatalf("editar a dos proveedores: %v", err)
	}
	if _, err := svc.EnviarSolicitud(empDemo, sol.ID, actorA, origenTst); err != nil {
		t.Fatalf("enviar la licitación con dos proveedores: %v", err)
	}
}

/* --- Modalidad lista de precios ------------------------------------------- */

// conTarifa crea un proveedor con su lista de precios de compra activa.
func conTarifa(t *testing.T, svc *application.Service, sku string, precio float64) string {
	t.Helper()
	p, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{Nombre: "Con tarifa, C.A."})
	if err != nil {
		t.Fatalf("crear proveedor: %v", err)
	}
	if _, err := svc.CrearListaPrecio(empDemo, actorA, origenTst, application.EntradaListaPrecio{
		Nombre: "Tarifa 2026", Tipo: listaprecio.TipoCompra, ProveedorID: p.ID, Activa: true,
		Items: []listaprecio.ItemLista{{SKU: sku, Precio: precio}},
	}); err != nil {
		t.Fatalf("crear lista de compra: %v", err)
	}
	return p.ID
}

// TestModalidad_ListaDePreciosConvierteSinRespuesta es el punto de la modalidad: no
// se le pidió presupuesto a nadie, así que no hay respuesta que esperar — el costo
// sale de la tarifa ya pactada.
func TestModalidad_ListaDePreciosConvierteSinRespuesta(t *testing.T) {
	svc := servicioConListas(t)
	sku := primerSKU(t, svc)
	prov := conTarifa(t, svc, sku, 33)

	sol, err := solicitudCon(t, svc, sku, compra.ModalidadListaPrecios, prov)
	if err != nil {
		t.Fatalf("crear solicitud por lista de precios: %v", err)
	}
	if _, err := svc.EnviarSolicitud(empDemo, sol.ID, actorA, origenTst); err != nil {
		t.Fatalf("enviar: %v", err)
	}
	// Sin registrar ninguna respuesta, se convierte.
	_, oc, err := svc.ConvertirEnOrden(empDemo, sol.ID, actorA, origenTst, prov)
	if err != nil {
		t.Fatalf("convertir sin respuesta debe funcionar en lista de precios: %v", err)
	}
	if len(oc.Lineas) != 1 {
		t.Fatalf("la orden debía traer la línea de la solicitud, trae %d", len(oc.Lineas))
	}
	casiEq(t, oc.Lineas[0].CostoUnitario, 33, "el costo sale de la tarifa del proveedor")
	casiEq(t, oc.Subtotal, 165, "5 u × Bs 33")
}

// TestModalidad_ListaDePreciosExigeTarifaActiva: sin tarifa no hay de dónde sacar
// los costos, y se avisa al enviar en vez de fallar al convertir.
func TestModalidad_ListaDePreciosExigeTarifaActiva(t *testing.T) {
	svc := servicioConListas(t)
	sku := primerSKU(t, svc)
	sinTarifa, _ := dosProveedores(t, svc)

	sol, err := solicitudCon(t, svc, sku, compra.ModalidadListaPrecios, sinTarifa)
	if err != nil {
		t.Fatalf("crear solicitud: %v", err)
	}
	if _, err := svc.EnviarSolicitud(empDemo, sol.ID, actorA, origenTst); !errors.Is(err, application.ErrSinListaDeProveedor) {
		t.Fatalf("enviar sin tarifa debía dar ErrSinListaDeProveedor, se obtuvo: %v", err)
	}
}

// TestModalidad_ListaDePreciosAvisaQuePRODUCTOFalta: si la tarifa no cubre un
// producto, se dice CUÁL, en vez de dejar un costo en cero.
func TestModalidad_ListaDePreciosAvisaQueProductoFalta(t *testing.T) {
	svc := servicioConListas(t)
	sku := primerSKU(t, svc)
	prov := conTarifa(t, svc, sku, 33)

	// Un segundo producto que la tarifa NO cubre.
	otro := ""
	for _, p := range svc.Productos(empDemo) {
		if p.SKU != sku && !p.EsCombo {
			otro = p.SKU
			break
		}
	}
	if otro == "" {
		t.Skip("el seed no trae un segundo producto")
	}

	sol, err := svc.CrearSolicitud(empDemo, sede1, actorA, origenTst, application.EntradaSolicitud{
		SedeID: sede1, Modalidad: compra.ModalidadListaPrecios,
		Lineas: []application.LineaSolicitudEntrada{
			{SKU: sku, Cantidad: 5}, {SKU: otro, Cantidad: 2},
		},
		Proveedores: []string{prov},
	})
	if err != nil {
		t.Fatalf("crear solicitud: %v", err)
	}
	if _, err := svc.EnviarSolicitud(empDemo, sol.ID, actorA, origenTst); err != nil {
		t.Fatalf("enviar: %v", err)
	}
	_, _, err = svc.ConvertirEnOrden(empDemo, sol.ID, actorA, origenTst, prov)
	if !errors.Is(err, application.ErrSKUSinTarifa) {
		t.Fatalf("un producto fuera de la tarifa debía dar ErrSKUSinTarifa, se obtuvo: %v", err)
	}
	if err != nil && !contieneTexto(err.Error(), otro) {
		t.Errorf("el error debe decir QUÉ producto falta (%s), dijo: %v", otro, err)
	}
}

func contieneTexto(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexDe(s, sub) >= 0)
}

func indexDe(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

/* --- Lista de compra atada al proveedor ------------------------------------ */

// TestListaDeCompra_SoloLasDeCompraTienenProveedor: el precio al cliente no depende
// de a quién se le compró.
func TestListaDeCompra_SoloLasDeCompraTienenProveedor(t *testing.T) {
	svc := servicioConListas(t)
	a, _ := dosProveedores(t, svc)

	if _, err := svc.CrearListaPrecio(empDemo, actorA, origenTst, application.EntradaListaPrecio{
		Nombre: "Tarifa mal puesta", Tipo: listaprecio.TipoVenta, ProveedorID: a, Activa: true,
	}); !errors.Is(err, application.ErrListaProveedorSoloCompra) {
		t.Fatalf("una lista de VENTA con proveedor debía rechazarse, se obtuvo: %v", err)
	}
}

// TestListaDeCompra_ElProveedorDebeExistir.
func TestListaDeCompra_ElProveedorDebeExistir(t *testing.T) {
	svc := servicioConListas(t)
	if _, err := svc.CrearListaPrecio(empDemo, actorA, origenTst, application.EntradaListaPrecio{
		Nombre: "Tarifa fantasma", Tipo: listaprecio.TipoCompra, ProveedorID: "prov_inexistente", Activa: true,
	}); !errors.Is(err, application.ErrProveedorNoExiste) {
		t.Fatalf("una tarifa de un proveedor inexistente debía rechazarse, se obtuvo: %v", err)
	}
}

// TestCostoPactado_DistingueSinTarifaDePrecioCero: un obsequio cuesta 0 y eso no es
// lo mismo que «no hay tarifa».
func TestCostoPactado_DistingueSinTarifaDePrecioCero(t *testing.T) {
	svc := servicioConListas(t)
	sku := primerSKU(t, svc)
	prov := conTarifa(t, svc, sku, 0)

	precio, ok := svc.CostoPactadoCon(empDemo, prov, sku)
	if !ok {
		t.Fatal("la tarifa cubre el SKU aunque su precio sea 0")
	}
	casiEq(t, precio, 0, "el precio pactado es 0 (obsequio/muestra)")

	if _, ok := svc.CostoPactadoCon(empDemo, prov, "SKU-QUE-NO-ESTA"); ok {
		t.Error("un SKU fuera de la tarifa debe reportarse como ausente")
	}
}

// TestModalidad_SeDeduceCuandoNoSeDeclara: el API no obliga a mandar el campo
// nuevo. Si no se declara, se deduce de los hechos —a varios es concurso, a uno es
// directa— y así ni los clientes existentes ni las solicitudes viejas se rompen.
func TestModalidad_SeDeduceCuandoNoSeDeclara(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	a, b := dosProveedores(t, svc)

	unSolo, err := solicitudCon(t, svc, sku, "", a)
	if err != nil {
		t.Fatalf("sin modalidad, un proveedor: %v", err)
	}
	if unSolo.Modalidad != compra.ModalidadDirecta {
		t.Errorf("a un solo proveedor se deduce adjudicación directa, quedó %q", unSolo.Modalidad)
	}

	varios, err := solicitudCon(t, svc, sku, "", a, b)
	if err != nil {
		t.Fatalf("sin modalidad, dos proveedores: %v", err)
	}
	if varios.Modalidad != compra.ModalidadLicitacion {
		t.Errorf("a varios proveedores se deduce licitación, quedó %q", varios.Modalidad)
	}
}
