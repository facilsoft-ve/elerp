package httpapi

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
)

/* CÓDIGOS DE ESTADO DE LA CONFIGURACIÓN DE ISLR.
 *
 * La convención del API (ver errorDeNota) es 404 lo que no existe, 409 lo que
 * choca con el estado del sistema, 400 lo demás. Los handlers de ISLR la
 * ignoraban: la sugerencia devolvía 404 para TODO.
 *
 * Importa porque los dos errores se arreglan en sitios distintos. «Ese concepto
 * no existe» se resuelve eligiendo otro; «falta cargar la unidad tributaria» se
 * resuelve en Configuración, y hasta que alguien lo haga ningún concepto con
 * sustraendo va a resolver. Un 404 para el segundo manda a revisar el catálogo de
 * conceptos, que es justo donde NO está el problema. */

func TestEstadoDeConfigISLR_DistingueLoQueNoExisteDeLoQueFalta(t *testing.T) {
	casos := []struct {
		err      error
		esperado int
		por      string
	}{
		{application.ErrConceptoNoExiste, fiber.StatusNotFound,
			"un concepto que no está en el maestro NO existe: 404"},
		{application.ErrUTNoCargada, fiber.StatusConflict,
			"falta cargar la UT: el concepto existe, lo que choca es el estado del sistema"},
		{application.ErrConceptoDuplicado, fiber.StatusConflict,
			"el par (código, sujeto) ya está tomado: choca con lo que hay"},
		{application.ErrUTRepetida, fiber.StatusConflict,
			"ya hay una UT desde esa fecha: choca con lo que hay"},
		{application.ErrUTInvalida, fiber.StatusBadRequest,
			"un valor que no es una UT es culpa de la petición: 400"},
		{application.ErrConceptoInvalido, fiber.StatusBadRequest,
			"un concepto mal formado es culpa de la petición: 400"},
		{errors.New("cualquier otra cosa"), fiber.StatusBadRequest,
			"lo no clasificado cae en 400, como hacían estos handlers antes"},
	}
	for _, c := range casos {
		if got := estadoDeConfigISLR(c.err); got != c.esperado {
			t.Errorf("%s — se esperaba %d, dio %d (error: %v)", c.por, c.esperado, got, c.err)
		}
	}
}

// El error envuelto tiene que traducirse igual: los servicios devuelven el error
// con contexto (fmt.Errorf con %w) en varios caminos, y una comparación por
// igualdad en vez de errors.Is dejaría esos casos cayendo al 400 por defecto.
func TestEstadoDeConfigISLR_AtraviesaElEnvoltorio(t *testing.T) {
	envuelto := fmt.Errorf("al resolver la tarifa: %w", application.ErrUTNoCargada)
	if got := estadoDeConfigISLR(envuelto); got != fiber.StatusConflict {
		t.Errorf("un error envuelto debía traducirse igual: se esperaba 409, dio %d", got)
	}
}
