package billing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mornix/elerp-platform/internal/billing"
	"github.com/mornix/elerp-platform/internal/core"
	"github.com/mornix/elerp-platform/internal/facturacion"
	"github.com/mornix/elerp-platform/internal/hubmy"
	"github.com/mornix/elerp-platform/internal/store"
)

// coreFalso responde 200 al PATCH del plan y registra el cuerpo recibido.
func coreFalso(t *testing.T, capturado *string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/internal/orgs/") || r.Method != http.MethodPatch {
			w.WriteHeader(404)
			return
		}
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		*capturado = string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"org1","plan":"Pro","estado":"activa"}`))
	}))
}

func nuevoSvc(coreURL string, hub *hubmy.Client) (*billing.Service, *store.MemPlanes) {
	planes := store.NewMemPlanes()
	subs := store.NewMemSuscripciones()
	return billing.New(planes, subs, core.New(coreURL, "k"), hub), planes
}

func TestAsignarPlan_Local_AplicaLimitesYActiva(t *testing.T) {
	var body string
	fc := coreFalso(t, &body)
	defer fc.Close()
	svc, planes := nuevoSvc(fc.URL, nil) // sin Hubmy

	plan := planes.Create(facturacion.Plan{Nombre: "Pro", Intervalo: facturacion.IntervaloMensual, PrecioCents: 999, Moneda: "USD",
		Limites: facturacion.Limites{FacturasMes: 500, Usuarios: 10, Sucursales: 3}})

	view, err := svc.AsignarPlan(context.Background(), "org1", plan.ID, "", "op@mornix.tech")
	if err != nil {
		t.Fatalf("AsignarPlan: %v", err)
	}
	if view.Suscripcion.Estado != facturacion.EstadoActiva {
		t.Fatalf("sin Hubmy la suscripción debe quedar activa, dio %q", view.Suscripcion.Estado)
	}
	// El core recibió los límites del plan.
	if !strings.Contains(body, `"facturasMes":500`) || !strings.Contains(body, `"usuarios":10`) {
		t.Fatalf("el core debió recibir los límites del plan; cuerpo=%s", body)
	}
	if view.Suscripcion.ProximoCobro == "" {
		t.Fatal("un plan mensual debe fijar próximo cobro")
	}
}

func TestAsignarPlan_Hubmy_GeneraCheckout(t *testing.T) {
	var body string
	fc := coreFalso(t, &body)
	defer fc.Close()

	fh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/api/sales":
			w.Write([]byte(`{"data":{"id":"sal_1","status":"pending","checkout_url":"https://checkout.hubmy.app/c/xyz"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/api/sales/sal_1":
			w.Write([]byte(`{"data":{"id":"sal_1","status":"paid"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer fh.Close()

	svc, planes := nuevoSvc(fc.URL, hubmy.New(fh.URL, "hk"))
	plan := planes.Create(facturacion.Plan{Nombre: "Pro", Intervalo: facturacion.IntervaloMensual, PrecioCents: 999, Moneda: "USD", HubmyPackageID: "pkg_1"})

	view, err := svc.AsignarPlan(context.Background(), "org1", plan.ID, "usr_1", "op@mornix.tech")
	if err != nil {
		t.Fatalf("AsignarPlan: %v", err)
	}
	if view.Suscripcion.Estado != facturacion.EstadoPendientePago {
		t.Fatalf("con Hubmy debe quedar pendiente de pago, dio %q", view.Suscripcion.Estado)
	}
	if view.Suscripcion.CheckoutURL == "" || view.Suscripcion.HubmySaleID != "sal_1" {
		t.Fatalf("debe traer checkout y saleId; sub=%+v", view.Suscripcion)
	}

	// Sincronizar: la venta ya está pagada → activa.
	view, err = svc.Sincronizar(context.Background(), "org1")
	if err != nil {
		t.Fatalf("Sincronizar: %v", err)
	}
	if view.Suscripcion.Estado != facturacion.EstadoActiva {
		t.Fatalf("tras el pago la suscripción debe quedar activa, dio %q", view.Suscripcion.Estado)
	}
}

func TestResumen_MRR(t *testing.T) {
	fc := coreFalso(t, new(string))
	defer fc.Close()
	svc, planes := nuevoSvc(fc.URL, nil)
	mensual := planes.Create(facturacion.Plan{Nombre: "Mensual", Intervalo: facturacion.IntervaloMensual, PrecioCents: 1000, Moneda: "USD"})
	anual := planes.Create(facturacion.Plan{Nombre: "Anual", Intervalo: facturacion.IntervaloAnual, PrecioCents: 12000, Moneda: "USD"})

	svc.AsignarPlan(context.Background(), "orgA", mensual.ID, "", "op") // activa, +1000/mes
	svc.AsignarPlan(context.Background(), "orgB", anual.ID, "", "op")   // activa, +1000/mes (12000/12)
	svc.Cancelar("orgB")                                                // cancelada → no cuenta

	res := svc.Resumen()
	if res.Suscripciones != 2 {
		t.Fatalf("deben verse 2 suscripciones, dio %d", res.Suscripciones)
	}
	if res.Activas != 1 || res.MRRCents != 1000 {
		t.Fatalf("solo orgA activa debe aportar MRR=1000, dio activas=%d mrr=%d", res.Activas, res.MRRCents)
	}
}
