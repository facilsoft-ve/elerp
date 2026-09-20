package application

import (
	"errors"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// MAESTRO DE LA UNIDAD TRIBUTARIA. Ver domain/fiscal/ut.go para el porqué.
//
// Lo que resuelve acá: que los sustraendos y los mínimos de ISLR se actualicen
// cambiando UN número cuando el SENIAT publica una UT nueva, en vez de editar
// una por una las filas de la tabla de conceptos —y descubrir seis meses después
// que quedaron tres sin tocar.

var (
	// ErrUTInvalida: el valor tiene que ser positivo y traer desde cuándo rige.
	ErrUTInvalida = errors.New("la unidad tributaria necesita un valor mayor que cero y una fecha de vigencia")
	// ErrUTNoCargada: no hay UT vigente para esa fecha. NO se resuelve con un cero:
	// un sustraendo anulado retiene de más y no falla en ningún lado.
	ErrUTNoCargada = errors.New("no hay valor de la unidad tributaria cargado para esa fecha")
	// ErrUTRepetida: ya hay una UT que arranca ese mismo día.
	ErrUTRepetida = errors.New("ya hay una unidad tributaria vigente desde esa fecha")
	// ErrUTNoCargadas: el maestro no está cableado en este servicio.
	ErrUTNoCargadas = errors.New("el histórico de la unidad tributaria no está disponible")
)

// ConUnidadesTributarias cablea el histórico de la UT.
func (s *Service) ConUnidadesTributarias(r fiscal.UnidadTributariaRepo) *Service {
	s.uts = r
	return s
}

// UnidadesTributarias devuelve el histórico de la empresa, de la más reciente a
// la más vieja. A diferencia de los conceptos, NO se siembra: el valor de la UT
// es un dato oficial con fecha y no se puede inventar un punto de partida.
func (s *Service) UnidadesTributarias(empresaID string) []fiscal.UnidadTributaria {
	if s.uts == nil {
		return []fiscal.UnidadTributaria{}
	}
	out := s.uts.List(empresaID)
	// Orden estable de más nueva a más vieja, que es como se lee la pantalla.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].VigenteDesde > out[i].VigenteDesde {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// UTVigenteEn devuelve el valor de la UT que regía en una fecha (AAAA-MM-DD).
// Fecha vacía = hoy. El bool es false cuando no hay ninguna cargada para ese día.
func (s *Service) UTVigenteEn(empresaID, fecha string) (fiscal.UnidadTributaria, bool) {
	f := strings.TrimSpace(fecha)
	if f == "" {
		f = time.Now().UTC().Format("2006-01-02")
	}
	return fiscal.UTVigenteEn(s.UnidadesTributarias(empresaID), f)
}

// ValorUTEn devuelve solo el valor, o 0 si no hay UT para esa fecha. Pensado para
// quien ya comprobó que el concepto no la requiere; si la requiere, hay que usar
// UTVigenteEn y atender el false.
func (s *Service) ValorUTEn(empresaID, fecha string) float64 {
	u, ok := s.UTVigenteEn(empresaID, fecha)
	if !ok {
		return 0
	}
	return u.Valor
}

// CargarUT anexa un valor de la UT al histórico. Es de SOLO ANEXADO: una UT
// pasada no se corrige, se carga la siguiente. Corregirla haría que un
// comprobante ya emitido dejara de poder explicarse.
func (s *Service) CargarUT(empresaID, actor, origen string, u fiscal.UnidadTributaria) (fiscal.UnidadTributaria, error) {
	if s.uts == nil {
		return fiscal.UnidadTributaria{}, ErrUTNoCargadas
	}
	u.VigenteDesde = strings.TrimSpace(u.VigenteDesde)
	if u.Valor <= 0 || len(u.VigenteDesde) != 10 {
		return fiscal.UnidadTributaria{}, ErrUTInvalida
	}
	// Dos filas que arrancan el mismo día dejarían el valor decidido por el orden
	// de lectura. Se rechaza acá, que es el único momento en que se puede.
	for _, otra := range s.UnidadesTributarias(empresaID) {
		if otra.VigenteDesde == u.VigenteDesde {
			return fiscal.UnidadTributaria{}, ErrUTRepetida
		}
	}
	u.EmpresaID = empresaID
	u.Fuente = strings.TrimSpace(u.Fuente)
	u.Actor = actor
	u.Creada = ahora()
	out := s.uts.Create(u)
	s.audit.Append(evento(empresaID, actor, origen, "config.unidad_tributaria.cargar", out.VigenteDesde, out.Fuente))
	return out, nil
}
