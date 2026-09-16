package inmem

import (
	"fmt"
	"sort"
	"sync"

	"github.com/mornix/elerp/internal/domain/reserva"
)

// ReservaRepo guarda las reservaciones del salón en memoria. Mismo contrato que el
// de Mongo: filtro por empresa en TODAS las consultas (aislamiento de tenant).
type ReservaRepo struct {
	mu   sync.RWMutex
	seq  int
	data map[string]reserva.Reserva
}

func NewReservaRepo() *ReservaRepo {
	return &ReservaRepo{data: map[string]reserva.Reserva{}}
}

// ordenar deja la agenda por hora: es como se lee un día de salón.
func ordenar(xs []reserva.Reserva) []reserva.Reserva {
	sort.SliceStable(xs, func(i, j int) bool {
		if xs[i].Fecha != xs[j].Fecha {
			return xs[i].Fecha < xs[j].Fecha
		}
		return xs[i].Hora < xs[j].Hora
	})
	return xs
}

func (r *ReservaRepo) DelDia(empresaID, sedeID, fecha string) []reserva.Reserva {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []reserva.Reserva{}
	for _, x := range r.data {
		if x.EmpresaID == empresaID && x.SedeID == sedeID && x.Fecha == fecha {
			out = append(out, x)
		}
	}
	return ordenar(out)
}

func (r *ReservaRepo) Desde(empresaID, sedeID, fecha string) []reserva.Reserva {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []reserva.Reserva{}
	for _, x := range r.data {
		if x.EmpresaID == empresaID && x.SedeID == sedeID && x.Fecha >= fecha {
			out = append(out, x)
		}
	}
	return ordenar(out)
}

func (r *ReservaRepo) ByID(empresaID, id string) (reserva.Reserva, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	x, ok := r.data[id]
	if !ok || x.EmpresaID != empresaID {
		return reserva.Reserva{}, false
	}
	return x, true
}

func (r *ReservaRepo) Create(x reserva.Reserva) reserva.Reserva {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	x.ID = fmt.Sprintf("res_%d", r.seq)
	r.data[x.ID] = x
	return x
}

func (r *ReservaRepo) Update(x reserva.Reserva) (reserva.Reserva, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	viejo, ok := r.data[x.ID]
	if !ok || viejo.EmpresaID != x.EmpresaID {
		return reserva.Reserva{}, false
	}
	r.data[x.ID] = x
	return x, true
}
