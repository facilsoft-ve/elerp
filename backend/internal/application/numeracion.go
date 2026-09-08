package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// Configuración de numeración fiscal (Ajustes › Series y numeración).
//
// El contador de folios es "último folio entregado" por clave empresa|sede|serie.
// Esta capa expone dos cosas: VER el estado de todas las series de la empresa, y
// FIJAR el próximo folio de una serie SOLO hacia adelante — para continuar la
// numeración de un sistema previo sin reusar folios (requisito legal). Nunca se
// retrocede ni se reasigna un folio ya usado (Principio 2, integridad fiscal).

// ErrProximoInvalido se devuelve cuando el próximo folio pedido no es un entero
// positivo.
var ErrProximoInvalido = errors.New("el próximo folio debe ser un número mayor o igual a 1")

// SerieNumeracion es el estado de una serie: su último folio entregado y el
// próximo que se asignará. La sede se resuelve a su nombre cuando se conoce.
type SerieNumeracion struct {
	SedeID     string `json:"sedeId"`
	SedeNombre string `json:"sedeNombre"`
	Serie      string `json:"serie"`
	Actual     int    `json:"actual"`  // último folio entregado (0 si nunca se emitió)
	Proximo    int    `json:"proximo"` // Actual + 1: el que recibirá el próximo documento
}

// EstadoNumeracion lista las series de la empresa con su folio actual y próximo,
// ordenadas por sede y serie. Puede venir vacía si aún no se ha emitido nada.
//
// El nombre de la sede se deja igual al id: el Service no tiene acceso directo al
// repositorio de sedes (vive en TenancyService). El adaptador HTTP lo enriquece
// con el nombre real antes de responder.
func (s *Service) EstadoNumeracion(empresaID string) []SerieNumeracion {
	series := s.numerador.Series(empresaID)
	out := make([]SerieNumeracion, 0, len(series))
	prefijo := empresaID + "|"
	for key, ultimo := range series {
		// key = "empresaID|sedeID|serie". El empresaID puede contener '|'? No: los
		// ids son controlados y sin separador. Se recorta el prefijo y se parte la
		// primera '|' para separar sede de serie (la serie tampoco lleva '|').
		resto := strings.TrimPrefix(key, prefijo)
		sedeID, serie, ok := strings.Cut(resto, "|")
		if !ok {
			// Clave con forma inesperada: se ignora en vez de mostrar basura.
			continue
		}
		out = append(out, SerieNumeracion{
			SedeID: sedeID, SedeNombre: sedeID, Serie: serie,
			Actual: ultimo, Proximo: ultimo + 1,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SedeID != out[j].SedeID {
			return out[i].SedeID < out[j].SedeID
		}
		return out[i].Serie < out[j].Serie
	})
	return out
}

// FijarNumeracion fija el próximo folio de una serie SOLO hacia adelante: exige
// que `proximo` sea >= 1 y estrictamente mayor que el último folio ya entregado
// (si no, ErrFolioRetrocede). Guarda `proximo-1` como último entregado, de modo
// que la próxima emisión reciba exactamente `proximo`. Audita el cambio.
func (s *Service) FijarNumeracion(empresaID, actor, origen, sedeID, serie string, proximo int) error {
	if proximo < 1 {
		return ErrProximoInvalido
	}
	// Forward-only en la capa de aplicación: el próximo folio tiene que superar al
	// último entregado. El adaptador vuelve a validarlo (defensa en profundidad).
	if proximo <= s.numerador.Actual(empresaID, sedeID, serie) {
		return fiscal.ErrFolioRetrocede
	}
	if err := s.numerador.Fijar(empresaID, sedeID, serie, proximo-1); err != nil {
		return err
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.numeracion.fijar",
		serie, fmt.Sprintf("sede %s · serie %s · próximo folio %d", sedeID, serie, proximo)))
	return nil
}
