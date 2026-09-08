package httpapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// GUARD DE CSRF PARA PETICIONES ENTRE SITIOS
//
// La cookie de sesión es `SameSite=None` porque Hubmy embebe la app en un iframe
// cross-site y con `Lax` no viaja (el login SSO fallaba con «state inválido»). El costo
// de `None` es que la cookie ahora acompaña CUALQUIER petición entre sitios, así que un
// formulario en un sitio ajeno podría disparar una escritura con la sesión de la víctima.
//
// CORS no alcanza para esto: impide LEER la respuesta, no impide que la petición llegue y
// tenga efecto. Un `<form method="POST">` cross-site no necesita permiso de CORS.
//
// Criterio: en los métodos que ESCRIBEN, se exige que el navegador declare que la
// petición es del mismo sitio, o que el `Origin` esté en la lista permitida.
//
//   - `Sec-Fetch-Site` lo envían todos los navegadores actuales y no es falsificable por
//     JavaScript, así que es la señal fuerte. `same-origin` y `same-site` pasan; `none`
//     también (es una navegación escrita por el usuario en la barra de direcciones).
//   - `cross-site` pasa SOLO si el Origin es nuestro propio dominio o un subdominio de
//     hubmy.app, que son los únicos que el CSP autoriza a embebernos.
//   - Si NO viene ninguna de las dos cabeceras se permite: son clientes que no son
//     navegadores (el agente fiscal local, curl, integraciones), donde CSRF no aplica
//     porque no hay cookie de sesión que el navegador adjunte sola.
//
// Devuelve 403 con un mensaje explícito en vez de fallar silenciosamente, para que un
// integrador entienda qué pasó.

// origenesEmbebedores son los sitios que pueden operar la app embebida. Debe coincidir
// con el `frame-ancestors` del nginx: si uno permite y el otro no, el síntoma es una app
// que carga pero no puede guardar nada.
func origenPermitido(origin, propio string) bool {
	if origin == "" {
		return false
	}
	if propio != "" && strings.EqualFold(origin, propio) {
		return true
	}
	host := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	host = strings.ToLower(host)
	return host == "hubmy.app" || strings.HasSuffix(host, ".hubmy.app")
}

// esEscritura indica si el método puede cambiar estado.
func esEscritura(metodo string) bool {
	switch metodo {
	case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		return true
	}
	return false
}

// guardCSRF construye el middleware. `propio` es la URL pública de la app.
func guardCSRF(propio string) fiber.Handler {
	propio = strings.TrimRight(propio, "/")
	return func(c *fiber.Ctx) error {
		if !esEscritura(c.Method()) {
			return c.Next()
		}
		sitio := c.Get("Sec-Fetch-Site")
		origin := c.Get("Origin")
		switch sitio {
		case "same-origin", "same-site", "none":
			return c.Next()
		case "cross-site":
			if origenPermitido(origin, propio) {
				return c.Next()
			}
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "petición entre sitios no permitida",
			})
		}
		// Sin Sec-Fetch-Site: si declara un Origin, tiene que ser uno permitido; si no
		// declara ninguno, no es un navegador y se deja pasar.
		if origin != "" && !origenPermitido(origin, propio) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "origen no permitido",
			})
		}
		return c.Next()
	}
}
