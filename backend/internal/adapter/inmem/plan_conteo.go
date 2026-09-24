package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/inventario"
)

// --- Planes de conteo (qué toca contar, y cada cuánto) ---

type PlanConteoRepo struct {
	mu    sync.RWMutex
	items []inventario.PlanConteo
}

func NewPlanConteoRepo() *PlanConteoRepo { return &PlanConteoRepo{} }

func (r *PlanConteoRepo) List(empresaID string) []inventario.PlanConteo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []inventario.PlanConteo{}
	for _, x := range r.items {
		if x.EmpresaID == empresaID {
			out = append(out, x)
		}
	}
	return out
}

func (r *PlanConteoRepo) ByID(empresaID, id string) (inventario.PlanConteo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, x := range r.items {
		if x.EmpresaID == empresaID && x.ID == id {
			return x, true
		}
	}
	return inventario.PlanConteo{}, false
}

func (r *PlanConteoRepo) Create(x inventario.PlanConteo) inventario.PlanConteo {
	r.mu.Lock()
	defer r.mu.Unlock()
	if x.ID == "" {
		x.ID = nextID("pcn_")
	}
	r.items = append(r.items, x)
	return x
}

func (r *PlanConteoRepo) Update(x inventario.PlanConteo) (inventario.PlanConteo, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, y := range r.items {
		if y.EmpresaID == x.EmpresaID && y.ID == x.ID {
			r.items[i] = x
			return x, true
		}
	}
	return inventario.PlanConteo{}, false
}
