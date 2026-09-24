package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/inventario"
)

// --- Reglas de reabastecimiento (cuándo volver a comprar) ---

type ReglaReabastecimientoRepo struct {
	mu    sync.RWMutex
	items []inventario.ReglaReabastecimiento
}

func NewReglaReabastecimientoRepo() *ReglaReabastecimientoRepo {
	return &ReglaReabastecimientoRepo{}
}

func (r *ReglaReabastecimientoRepo) List(empresaID string) []inventario.ReglaReabastecimiento {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []inventario.ReglaReabastecimiento{}
	for _, x := range r.items {
		if x.EmpresaID == empresaID {
			out = append(out, x)
		}
	}
	return out
}

func (r *ReglaReabastecimientoRepo) ByID(empresaID, id string) (inventario.ReglaReabastecimiento, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, x := range r.items {
		if x.EmpresaID == empresaID && x.ID == id {
			return x, true
		}
	}
	return inventario.ReglaReabastecimiento{}, false
}

func (r *ReglaReabastecimientoRepo) Create(x inventario.ReglaReabastecimiento) inventario.ReglaReabastecimiento {
	r.mu.Lock()
	defer r.mu.Unlock()
	if x.ID == "" {
		x.ID = nextID("rea_")
	}
	r.items = append(r.items, x)
	return x
}

func (r *ReglaReabastecimientoRepo) Update(x inventario.ReglaReabastecimiento) (inventario.ReglaReabastecimiento, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, y := range r.items {
		if y.EmpresaID == x.EmpresaID && y.ID == x.ID {
			r.items[i] = x
			return x, true
		}
	}
	return inventario.ReglaReabastecimiento{}, false
}
