package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// El guard de CSRF existe porque la cookie de sesión es SameSite=None: Hubmy embebe la
// app en un iframe cross-site y con `Lax` la cookie no viaja (el login SSO fallaba con
// «state inválido»). El costo de `None` es que la cookie acompaña CUALQUIER petición
// entre sitios, así que un formulario en un sitio ajeno podría disparar una escritura con
// la sesión de la víctima. CORS no lo impide: impide LEER la respuesta, no que la
// petición llegue y tenga efecto.
//
// Se monta SOLO el middleware sobre una app mínima: lo que se prueba es su decisión, no
// el resto del servidor.
func appConGuard() *fiber.App {
	app := fiber.New()
	app.Use(guardCSRF("https://elerp.tech"))
	app.All("/*", func(c *fiber.Ctx) error { return c.SendString("ok") })
	return app
}

func pedir(t *testing.T, metodo string, cabeceras map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(metodo, "/api/algo", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cabeceras {
		req.Header.Set(k, v)
	}
	resp, err := appConGuard().Test(req, -1)
	if err != nil {
		t.Fatalf("petición: %v", err)
	}
	return resp.StatusCode
}

func TestCSRF_LecturaSiemprePasa(t *testing.T) {
	// GET no cambia estado: el guard no debe estorbar ni desde un sitio ajeno.
	if got := pedir(t, http.MethodGet, map[string]string{
		"Sec-Fetch-Site": "cross-site", "Origin": "https://sitio-ajeno.example",
	}); got == http.StatusForbidden {
		t.Fatalf("una lectura no debe bloquearse, dio %d", got)
	}
}

func TestCSRF_EscrituraCrossSiteAjenaSeRechaza(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if got := pedir(t, m, map[string]string{
			"Sec-Fetch-Site": "cross-site", "Origin": "https://sitio-ajeno.example",
		}); got != http.StatusForbidden {
			t.Errorf("%s desde un sitio ajeno debe dar 403, dio %d", m, got)
		}
	}
}

func TestCSRF_EscrituraDelPropioOrigenPasa(t *testing.T) {
	// Dentro del iframe el documento es NUESTRO, así que sus fetch son same-origin.
	for _, sitio := range []string{"same-origin", "same-site"} {
		if got := pedir(t, http.MethodPost, map[string]string{"Sec-Fetch-Site": sitio}); got == http.StatusForbidden {
			t.Errorf("Sec-Fetch-Site %q no debe bloquearse, dio %d", sitio, got)
		}
	}
}

func TestCSRF_EscrituraDesdeElEmbebedorAutorizadoPasa(t *testing.T) {
	// Mismo criterio que el frame-ancestors del nginx: si uno permite y el otro no, el
	// síntoma es una app que carga embebida pero no puede guardar nada.
	for _, origen := range []string{"https://app.hubmy.app", "https://dev.hubmy.app", "https://hubmy.app"} {
		if got := pedir(t, http.MethodPost, map[string]string{
			"Sec-Fetch-Site": "cross-site", "Origin": origen,
		}); got == http.StatusForbidden {
			t.Errorf("%s está autorizado a embebernos y debe poder escribir, dio %d", origen, got)
		}
	}
}

func TestCSRF_NoConfundeDominiosParecidos(t *testing.T) {
	// El sufijo tiene que ser de ETIQUETA, no de cadena: si no, hubmy.app.malicioso.example
	// pasaría por ser «algo que termina en hubmy.app» y el guard no protegería nada.
	for _, origen := range []string{
		"https://hubmy.app.malicioso.example",
		"https://nohubmy.app",
		"https://evilhubmy.app",
		"https://hubmy.app.co",
	} {
		if got := pedir(t, http.MethodPost, map[string]string{
			"Sec-Fetch-Site": "cross-site", "Origin": origen,
		}); got != http.StatusForbidden {
			t.Errorf("%s NO debe pasar como hubmy.app, dio %d", origen, got)
		}
	}
}

func TestCSRF_ClienteSinCabecerasDeNavegadorPasa(t *testing.T) {
	// El agente fiscal local, curl e integraciones no envían Sec-Fetch-* ni Origin: ahí no
	// hay CSRF que prevenir (no hay cookie que el navegador adjunte solo) y bloquearlos
	// rompería integraciones legítimas.
	if got := pedir(t, http.MethodPost, nil); got == http.StatusForbidden {
		t.Fatalf("un cliente que no es navegador no debe bloquearse, dio %d", got)
	}
}

func TestCSRF_SinSecFetchPeroConOrigenAjenoSeRechaza(t *testing.T) {
	// Un navegador viejo que no manda Sec-Fetch-Site pero sí Origin: se decide por Origin.
	if got := pedir(t, http.MethodPost, map[string]string{"Origin": "https://sitio-ajeno.example"}); got != http.StatusForbidden {
		t.Fatalf("con Origin ajeno debe dar 403, dio %d", got)
	}
	if got := pedir(t, http.MethodPost, map[string]string{"Origin": "https://elerp.tech"}); got == http.StatusForbidden {
		t.Fatalf("con nuestro propio Origin no debe bloquearse, dio %d", got)
	}
}

func TestCSRF_NavegacionDirectaPasa(t *testing.T) {
	// «none» es una navegación escrita/elegida por el usuario, no un sitio ajeno.
	if got := pedir(t, http.MethodPost, map[string]string{"Sec-Fetch-Site": "none"}); got == http.StatusForbidden {
		t.Fatalf("una navegación directa no debe bloquearse, dio %d", got)
	}
}
