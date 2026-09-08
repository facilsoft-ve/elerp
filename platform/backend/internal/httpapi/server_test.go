package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/mornix/elerp-platform/internal/billing"
	"github.com/mornix/elerp-platform/internal/config"
	"github.com/mornix/elerp-platform/internal/core"
	"github.com/mornix/elerp-platform/internal/httpapi"
	"github.com/mornix/elerp-platform/internal/mfa"
	"github.com/mornix/elerp-platform/internal/operador"
	"github.com/mornix/elerp-platform/internal/store"
)

const cookieName = "pc_session"

func nuevoServer(coreURL string) (*fiber.App, *store.MemOperadores) {
	ops := store.NewMemOperadores()
	sess := store.NewMemSesiones()
	cli := core.New(coreURL, "test-key")
	bill := billing.New(store.NewMemPlanes(), store.NewMemSuscripciones(), cli, nil)
	cfg := config.Config{
		CookieName:  cookieName,
		SessionTTL:  time.Hour,
		FrontendURL: "http://localhost:5174",
		WebDir:      "/no-existe", // sin SPA estático en test
	}
	return httpapi.NewServer(cfg, ops, sess, cli, bill, store.NewMemLeads(), nil), ops
}

func seedOperador(ops *store.MemOperadores, email, password string, mfaConfig bool) string {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	secreto, _ := mfa.GenerarSecreto()
	ops.Create(operador.Operador{Email: email, Nombre: "Op", Hash: string(hash), TOTPSecret: secreto, MFAConfigurada: mfaConfig})
	return secreto
}

func postJSON(app *fiber.App, path, body string) *http.Response {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, -1)
	return resp
}

func TestLogin_ExigeCodigoMFA(t *testing.T) {
	app, ops := nuevoServer("http://core.invalid")
	seedOperador(ops, "a@b.com", "clave12345", true)

	// Sin código: 401 con paso "mfa".
	resp := postJSON(app, "/papi/auth/login", `{"email":"a@b.com","password":"clave12345"}`)
	if resp.StatusCode != 401 {
		t.Fatalf("login sin código debe dar 401, dio %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["paso"] != "mfa" {
		t.Fatalf("debe indicar paso=mfa, dio %v", body)
	}

	// Contraseña mala: 401 credenciales.
	resp = postJSON(app, "/papi/auth/login", `{"email":"a@b.com","password":"mala"}`)
	if resp.StatusCode != 401 {
		t.Fatalf("contraseña mala debe dar 401, dio %d", resp.StatusCode)
	}
}

func TestLogin_ConCodigoEmiteSesion(t *testing.T) {
	app, ops := nuevoServer("http://core.invalid")
	secreto := seedOperador(ops, "a@b.com", "clave12345", true)
	codigo, _ := mfa.Codigo(secreto, time.Now())

	resp := postJSON(app, "/papi/auth/login", `{"email":"a@b.com","password":"clave12345","codigo":"`+codigo+`"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("login con código válido debe dar 200, dio %d", resp.StatusCode)
	}
	if cookieDe(resp, cookieName) == "" {
		t.Fatal("el login debe emitir la cookie de sesión")
	}
}

func TestEnrolarMFA_Flujo(t *testing.T) {
	app, ops := nuevoServer("http://core.invalid")
	seedOperador(ops, "nuevo@b.com", "clave12345", false) // aún sin MFA

	// login → pide enrolar.
	resp := postJSON(app, "/papi/auth/login", `{"email":"nuevo@b.com","password":"clave12345"}`)
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["paso"] != "enrolar-mfa" {
		t.Fatalf("un operador sin MFA debe recibir paso=enrolar-mfa, dio %v", body)
	}

	// setup → devuelve otpauthUrl + secreto.
	resp = postJSON(app, "/papi/auth/mfa/setup", `{"email":"nuevo@b.com","password":"clave12345"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("setup debe dar 200, dio %d", resp.StatusCode)
	}
	var setup map[string]any
	json.NewDecoder(resp.Body).Decode(&setup)
	secreto, _ := setup["secreto"].(string)
	if secreto == "" || setup["otpauthUrl"] == "" {
		t.Fatalf("setup debe devolver secreto + otpauthUrl, dio %v", setup)
	}

	// verify con el código correcto → emite sesión.
	codigo, _ := mfa.Codigo(secreto, time.Now())
	resp = postJSON(app, "/papi/auth/mfa/verify", `{"email":"nuevo@b.com","password":"clave12345","codigo":"`+codigo+`"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("verify con código válido debe dar 200, dio %d", resp.StatusCode)
	}
	if cookieDe(resp, cookieName) == "" {
		t.Fatal("verify debe emitir la cookie de sesión")
	}
	// El operador quedó con MFA configurada.
	o, _ := ops.ByEmail("nuevo@b.com")
	if !o.MFAConfigurada {
		t.Fatal("tras verify el operador debe quedar con MFAConfigurada=true")
	}
}

func TestProxy_InyectaClaveYActor(t *testing.T) {
	// Core falso que verifica las cabeceras M2M y responde.
	var gotKey, gotActor string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Platform-Key")
		gotActor = r.Header.Get("X-Platform-Actor")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"tenants":[]}`)
	}))
	defer fake.Close()

	app, ops := nuevoServer(fake.URL)
	secreto := seedOperador(ops, "op@mornix.tech", "clave12345", true)
	codigo, _ := mfa.Codigo(secreto, time.Now())

	// login para obtener la cookie.
	resp := postJSON(app, "/papi/auth/login", `{"email":"op@mornix.tech","password":"clave12345","codigo":"`+codigo+`"}`)
	cookie := cookieDe(resp, cookieName)
	if cookie == "" {
		t.Fatal("no se obtuvo cookie de sesión")
	}

	// Sin cookie: 401.
	req := httptest.NewRequest(http.MethodGet, "/papi/tenants", nil)
	resp, _ = app.Test(req, -1)
	if resp.StatusCode != 401 {
		t.Fatalf("sin sesión /papi/tenants debe dar 401, dio %d", resp.StatusCode)
	}

	// Con cookie: relaya al core, que ve la clave y el actor.
	req = httptest.NewRequest(http.MethodGet, "/papi/tenants", nil)
	req.Header.Set("Cookie", cookieName+"="+cookie)
	resp, _ = app.Test(req, -1)
	if resp.StatusCode != 200 {
		t.Fatalf("con sesión debe dar 200, dio %d", resp.StatusCode)
	}
	if gotKey != "test-key" {
		t.Fatalf("el core debe recibir X-Platform-Key=test-key, recibió %q", gotKey)
	}
	if gotActor != "op@mornix.tech" {
		t.Fatalf("el core debe recibir X-Platform-Actor con el email del operador, recibió %q", gotActor)
	}
}

// cookieDe extrae el valor de una cookie de la respuesta.
func cookieDe(resp *http.Response, nombre string) string {
	for _, c := range resp.Cookies() {
		if c.Name == nombre && c.Value != "" {
			return c.Value
		}
	}
	return ""
}
