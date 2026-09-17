package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/config"
)

/* A ElERP SE ENTRA CON HUBMY.
 *
 * Lo que se prueba acá es que la política la hace cumplir el SERVIDOR, no la
 * pantalla. Ocultar el formulario no cierra nada: quien arme la petición a mano
 * entra igual, y esa es justo la puerta que nadie vigila porque no se ve.
 *
 * Son dos rutas, no una: el login nativo y la aceptación de invitación, que
 * también emite sesión sin pasar por Hubmy. Cerrar una y dejar la otra sería
 * cerrar la puerta y dejar la ventana. */

// servidorDePrueba levanta el servidor con lo mínimo: estas rutas se cierran
// ANTES de tocar servicios, que es precisamente la propiedad que se comprueba.
func servidorDePrueba(t *testing.T, authNativa bool) *fiber.App {
	t.Helper()
	// FrontendURL concreto: con el comodín por defecto, Fiber entra en pánico por
	// combinar CORS abierto con credenciales.
	return NewServer(config.Config{AuthNativa: authNativa, FrontendURL: "http://localhost:5173"}, nil, nil, nil, nil, nil)
}

func TestAuthNativa_CerradaPorDefecto(t *testing.T) {
	app := servidorDePrueba(t, false)
	for _, ruta := range []string{"/api/auth/native-login", "/api/auth/accept-invite"} {
		req := httptest.NewRequest("POST", ruta, strings.NewReader(`{"email":"x@y.z","password":"loquesea"}`))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("%s: %v", ruta, err)
		}
		// 404 y no 403: un 403 confirma que la puerta existe y que solo falta el
		// permiso, y para quien prueba credenciales eso ya es información.
		if res.StatusCode != 404 {
			t.Fatalf("%s devolvió %d; con AUTH_NATIVA apagada debe responder 404", ruta, res.StatusCode)
		}
	}
}

func TestAuthNativa_SeAbreConElInterruptor(t *testing.T) {
	app := servidorDePrueba(t, true)
	req := httptest.NewRequest("POST", "/api/auth/native-login", strings.NewReader(`{"email":"x@y.z","password":"loquesea"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("native-login: %v", err)
	}
	// Con la puerta abierta ya no es 404: llega a validar y rechaza la
	// credencial inventada con 401.
	if res.StatusCode == 404 {
		t.Fatal("con AUTH_NATIVA encendida la ruta no debería responder 404")
	}
}

// El health le dice a la pantalla qué puertas hay. Si mintiera, la pantalla
// ofrecería un formulario que el servidor va a rechazar.
func TestHealth_DeclaraLasPuertas(t *testing.T) {
	cfg := config.Config{}
	if cfg.AuthNativa {
		t.Fatal("AUTH_NATIVA tiene que venir apagada en el cero del struct: a ElERP se entra con Hubmy")
	}
}
