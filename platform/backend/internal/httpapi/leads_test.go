package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp-platform/internal/billing"
	"github.com/mornix/elerp-platform/internal/config"
	"github.com/mornix/elerp-platform/internal/core"
	"github.com/mornix/elerp-platform/internal/httpapi"
	"github.com/mornix/elerp-platform/internal/lead"
	"github.com/mornix/elerp-platform/internal/mfa"
	"github.com/mornix/elerp-platform/internal/store"
)

// serverConLeads levanta el BFF con la ruta pública de solicitudes habilitada y devuelve
// también el store para inspeccionar qué se guardó de verdad.
func serverConLeads() (*fiber.App, *store.MemLeads, *store.MemOperadores) {
	ops := store.NewMemOperadores()
	sess := store.NewMemSesiones()
	cli := core.New("http://core.invalid", "test-key")
	bill := billing.New(store.NewMemPlanes(), store.NewMemSuscripciones(), cli, nil)
	leads := store.NewMemLeads()
	cfg := config.Config{
		CookieName: cookieName, SessionTTL: time.Hour,
		FrontendURL: "http://localhost:5174", WebDir: "/no-existe",
		LeadsPublicoHabilitado: true,
	}
	return httpapi.NewServer(cfg, ops, sess, cli, bill, leads, nil), leads, ops
}

func TestLeadPublico_GuardaLaSolicitud(t *testing.T) {
	app, leads, _ := serverConLeads()

	body := `{"nombre":"  José  ","empresa":"Bodega La Esquina","email":"JOSE@Ejemplo.COM",
	          "telefono":"04141234567","giro":"bodega","mensaje":"Quiero ver el POS"}`
	resp := postJSON(app, "/papi/public/leads", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("una solicitud válida debe dar 201, dio %d", resp.StatusCode)
	}

	items := leads.List()
	if len(items) != 1 {
		t.Fatalf("se esperaba 1 solicitud guardada, hay %d", len(items))
	}
	l := items[0]
	if l.Nombre != "José" {
		t.Errorf("el nombre debe quedar recortado: %q", l.Nombre)
	}
	if l.Email != "jose@ejemplo.com" {
		t.Errorf("el email debe normalizarse a minúsculas: %q", l.Email)
	}
	if l.Estado != lead.EstadoNuevo {
		t.Errorf("una solicitud nueva debe quedar en estado %q, quedó %q", lead.EstadoNuevo, l.Estado)
	}
	if l.Origen != lead.OrigenWeb {
		t.Errorf("el origen debe ser %q, fue %q", lead.OrigenWeb, l.Origen)
	}
}

// El honeypot no debe delatarse: responde OK pero no guarda nada.
func TestLeadPublico_HoneypotDescartaSinAvisar(t *testing.T) {
	app, leads, _ := serverConLeads()

	resp := postJSON(app, "/papi/public/leads",
		`{"nombre":"Bot","email":"bot@spam.com","sitioWeb":"http://spam.example"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("el honeypot debe responder 201 (sin delatarse), dio %d", resp.StatusCode)
	}
	if n := len(leads.List()); n != 0 {
		t.Fatalf("el honeypot no debe guardar nada, guardó %d", n)
	}
}

func TestLeadPublico_RechazaDatosInvalidos(t *testing.T) {
	casos := []struct{ nombre, body string }{
		{"sin nombre", `{"email":"a@b.com"}`},
		{"sin email", `{"nombre":"Ana"}`},
		{"email mal formado", `{"nombre":"Ana","email":"ana@sinpunto"}`},
		{"rubro inexistente", `{"nombre":"Ana","email":"a@b.com","giro":"panaderia"}`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			app, leads, _ := serverConLeads()
			resp := postJSON(app, "/papi/public/leads", c.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s debe dar 400, dio %d", c.nombre, resp.StatusCode)
			}
			if n := len(leads.List()); n != 0 {
				t.Fatalf("no debe guardar nada inválido, guardó %d", n)
			}
		})
	}
}

// El visitante no puede fijar campos internos: el DTO público los ignora.
func TestLeadPublico_IgnoraCamposInternos(t *testing.T) {
	app, leads, _ := serverConLeads()

	postJSON(app, "/papi/public/leads",
		`{"nombre":"Ana","email":"a@b.com","estado":"ganado","notas":"soy admin","id":"forzado"}`)

	l := leads.List()[0]
	if l.Estado != lead.EstadoNuevo {
		t.Errorf("el visitante no debe poder fijar el estado: quedó %q", l.Estado)
	}
	if l.Notas != "" {
		t.Errorf("el visitante no debe poder fijar notas: quedó %q", l.Notas)
	}
	if l.ID == "forzado" {
		t.Error("el visitante no debe poder fijar el ID")
	}
}

// La bandeja del equipo exige sesión de operador.
func TestLeads_BandejaExigeSesion(t *testing.T) {
	app, _, _ := serverConLeads()

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/papi/leads", nil), -1)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sin sesión debe dar 401, dio %d", resp.StatusCode)
	}
}

func TestLeads_OperadorListaYCambiaEstado(t *testing.T) {
	app, leads, ops := serverConLeads()
	secreto := seedOperador(ops, "op@mornix.tech", "clave12345", true)
	codigo, _ := mfa.Codigo(secreto, time.Now())
	login := postJSON(app, "/papi/auth/login",
		`{"email":"op@mornix.tech","password":"clave12345","codigo":"`+codigo+`"}`)
	sesion := cookieDe(login, cookieName)
	if sesion == "" {
		t.Fatal("no se obtuvo sesión de operador")
	}
	creado := leads.Create(lead.Lead{Nombre: "Ana", Email: "a@b.com", Estado: lead.EstadoNuevo, Origen: lead.OrigenWeb})

	// Listar
	req := httptest.NewRequest(http.MethodGet, "/papi/leads", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: sesion})
	resp, _ := app.Test(req, -1)
	if resp.StatusCode != 200 {
		t.Fatalf("listar con sesión debe dar 200, dio %d", resp.StatusCode)
	}
	var payload struct {
		Leads []lead.Lead `json:"leads"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if len(payload.Leads) != 1 {
		t.Fatalf("se esperaba 1 solicitud en la bandeja, vinieron %d", len(payload.Leads))
	}

	// Cambiar estado
	preq := httptest.NewRequest(http.MethodPatch, "/papi/leads/"+creado.ID,
		strings.NewReader(`{"estado":"contactado","notas":"llamada hecha"}`))
	preq.Header.Set("Content-Type", "application/json")
	preq.AddCookie(&http.Cookie{Name: cookieName, Value: sesion})
	presp, _ := app.Test(preq, -1)
	if presp.StatusCode != 200 {
		t.Fatalf("el PATCH debe dar 200, dio %d", presp.StatusCode)
	}
	actualizado, _ := leads.ByID(creado.ID)
	if actualizado.Estado != lead.EstadoContactado || actualizado.Notas != "llamada hecha" {
		t.Fatalf("el parche no se aplicó: %+v", actualizado)
	}
}

func TestLeads_RechazaEstadoInventado(t *testing.T) {
	app, leads, ops := serverConLeads()
	secreto := seedOperador(ops, "op@mornix.tech", "clave12345", true)
	codigo, _ := mfa.Codigo(secreto, time.Now())
	login := postJSON(app, "/papi/auth/login",
		`{"email":"op@mornix.tech","password":"clave12345","codigo":"`+codigo+`"}`)
	sesion := cookieDe(login, cookieName)
	creado := leads.Create(lead.Lead{Nombre: "Ana", Email: "a@b.com", Estado: lead.EstadoNuevo})

	req := httptest.NewRequest(http.MethodPatch, "/papi/leads/"+creado.ID,
		strings.NewReader(`{"estado":"inventado"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: cookieName, Value: sesion})
	resp, _ := app.Test(req, -1)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("un estado inexistente debe dar 400, dio %d", resp.StatusCode)
	}
}
