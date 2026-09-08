package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
)

// provDemo2 es el segundo proveedor sembrado (Alimentos Mary).
const provDemo2 = "prov_demo_2"

// solEnviada crea una solicitud con dos líneas y dos proveedores y la envía.
func solEnviada(t *testing.T, svc *application.Service) compra.SolicitudCompra {
	t.Helper()
	sku := primerSKU(t, svc)
	sol, err := svc.CrearSolicitud(empDemo, sede1, actorA, origenTst, application.EntradaSolicitud{
		SedeID:      sede1,
		Lineas:      []application.LineaSolicitudEntrada{{SKU: sku, Cantidad: 10}},
		Proveedores: []string{provDemo1, provDemo2},
	})
	if err != nil {
		t.Fatalf("crear solicitud: %v", err)
	}
	if sol.Estado != compra.SolBorrador {
		t.Fatalf("una solicitud nace en borrador, fue %q", sol.Estado)
	}
	out, err := svc.EnviarSolicitud(empDemo, sol.ID, actorA, origenTst)
	if err != nil {
		t.Fatalf("enviar solicitud: %v", err)
	}
	if out.Estado != compra.SolEnviada {
		t.Fatalf("tras enviar la solicitud debería quedar enviada, fue %q", out.Estado)
	}
	return out
}

// TestCrearSolicitud_Vacia rechaza una solicitud sin líneas.
func TestCrearSolicitud_Vacia(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, err := svc.CrearSolicitud(empDemo, sede1, actorA, origenTst, application.EntradaSolicitud{})
	if !errors.Is(err, application.ErrSolicitudVacia) {
		t.Fatalf("se esperaba ErrSolicitudVacia, se obtuvo: %v", err)
	}
}

// TestEnviarSolicitud_SinProveedores exige al menos un proveedor para enviar.
func TestEnviarSolicitud_SinProveedores(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	sol, err := svc.CrearSolicitud(empDemo, sede1, actorA, origenTst, application.EntradaSolicitud{
		Lineas: []application.LineaSolicitudEntrada{{SKU: sku, Cantidad: 3}},
	})
	if err != nil {
		t.Fatalf("crear solicitud: %v", err)
	}
	_, err = svc.EnviarSolicitud(empDemo, sol.ID, actorA, origenTst)
	if !errors.Is(err, application.ErrSolicitudSinProv) {
		t.Fatalf("se esperaba ErrSolicitudSinProv, se obtuvo: %v", err)
	}
}

// TestRegistrarRespuesta_ProveedorNoInvitado rechaza cargar precios de un proveedor
// que no fue incluido en la solicitud.
func TestRegistrarRespuesta_ProveedorNoInvitado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sol := solEnviada(t, svc)
	sku := sol.Lineas[0].SKU
	_, err := svc.RegistrarRespuesta(empDemo, sol.ID, actorA, origenTst, "prov_demo_3",
		[]application.RespuestaLineaEntrada{{SKU: sku, PrecioUnitario: 10}})
	if !errors.Is(err, application.ErrProvNoInvitado) {
		t.Fatalf("se esperaba ErrProvNoInvitado, se obtuvo: %v", err)
	}
}

// TestRegistrarRespuesta_CalculaTotalYEstado comprueba que registrar la respuesta
// de un proveedor valoriza el total (precio × cantidad) y mueve la solicitud a
// respondida.
func TestRegistrarRespuesta_CalculaTotalYEstado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sol := solEnviada(t, svc) // 10 unidades del primer SKU
	sku := sol.Lineas[0].SKU

	out, err := svc.RegistrarRespuesta(empDemo, sol.ID, actorA, origenTst, provDemo1,
		[]application.RespuestaLineaEntrada{{SKU: sku, PrecioUnitario: 15}})
	if err != nil {
		t.Fatalf("registrar respuesta: %v", err)
	}
	if out.Estado != compra.SolRespondida {
		t.Fatalf("tras la primera respuesta la solicitud debería estar respondida, fue %q", out.Estado)
	}
	var pc *compra.ProveedorCotiza
	for i := range out.Proveedores {
		if out.Proveedores[i].ProveedorID == provDemo1 {
			pc = &out.Proveedores[i]
		}
	}
	if pc == nil || !pc.Respondida || pc.Estado != compra.CotizaRespondida {
		t.Fatalf("el proveedor debería quedar respondido: %+v", pc)
	}
	if !casi(pc.Total, 150) { // 10 × 15
		t.Fatalf("total esperado 150, fue %v", pc.Total)
	}
}

// TestConvertirEnOrden_ReusaOC verifica que convertir crea una OC en borrador con
// las líneas y los precios del proveedor elegido, y cierra la solicitud enlazada.
func TestConvertirEnOrden_ReusaOC(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sol := solEnviada(t, svc)
	sku := sol.Lineas[0].SKU
	if _, err := svc.RegistrarRespuesta(empDemo, sol.ID, actorA, origenTst, provDemo1,
		[]application.RespuestaLineaEntrada{{SKU: sku, PrecioUnitario: 12}}); err != nil {
		t.Fatalf("registrar respuesta: %v", err)
	}

	solOut, oc, err := svc.ConvertirEnOrden(empDemo, sol.ID, actorA, origenTst, provDemo1)
	if err != nil {
		t.Fatalf("convertir en orden: %v", err)
	}
	if oc.Estado != compra.OCBorrador {
		t.Fatalf("la OC debería nacer en borrador, fue %q", oc.Estado)
	}
	if oc.ProveedorID != provDemo1 {
		t.Fatalf("la OC debería ser del proveedor elegido, fue %q", oc.ProveedorID)
	}
	if len(oc.Lineas) != 1 || oc.Lineas[0].SKU != sku || !casi(oc.Lineas[0].CostoUnitario, 12) {
		t.Fatalf("la OC debería llevar la línea con el precio cotizado: %+v", oc.Lineas)
	}
	if solOut.Estado != compra.SolCerrada || solOut.OrdenGeneradaID != oc.ID || solOut.ProveedorElegidoID != provDemo1 {
		t.Fatalf("la solicitud debería quedar cerrada y enlazada a la orden: %+v", solOut)
	}
	// La OC creada debe ser recuperable por el módulo de compras (reusa el repo real).
	if _, ok := svc.OrdenCompra(empDemo, oc.ID); !ok {
		t.Fatalf("la OC convertida no aparece en el módulo de órdenes de compra")
	}
}

// TestConvertirEnOrden_SinRespuesta rechaza convertir con un proveedor que no cotizó.
func TestConvertirEnOrden_SinRespuesta(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sol := solEnviada(t, svc)
	_, _, err := svc.ConvertirEnOrden(empDemo, sol.ID, actorA, origenTst, provDemo2)
	if !errors.Is(err, application.ErrSolicitudSinPrecios) {
		t.Fatalf("se esperaba ErrSolicitudSinPrecios, se obtuvo: %v", err)
	}
}
