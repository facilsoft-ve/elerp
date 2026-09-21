package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/almacen"
)

// --- Tipos de operación (cómo entra y sale la mercancía) ---

type TipoOperacionRepo struct {
	mu    sync.RWMutex
	items []almacen.TipoOperacion
}

func NewTipoOperacionRepo() *TipoOperacionRepo { return &TipoOperacionRepo{} }

func (r *TipoOperacionRepo) List(empresaID string) []almacen.TipoOperacion {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []almacen.TipoOperacion{}
	for _, t := range r.items {
		if t.EmpresaID == empresaID {
			out = append(out, t)
		}
	}
	return out
}

func (r *TipoOperacionRepo) ByID(empresaID, id string) (almacen.TipoOperacion, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.items {
		if t.EmpresaID == empresaID && t.ID == id {
			return t, true
		}
	}
	return almacen.TipoOperacion{}, false
}

func (r *TipoOperacionRepo) Create(t almacen.TipoOperacion) almacen.TipoOperacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = nextID("ope_")
	}
	r.items = append(r.items, t)
	return t
}

func (r *TipoOperacionRepo) Update(t almacen.TipoOperacion) (almacen.TipoOperacion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == t.EmpresaID && x.ID == t.ID {
			r.items[i] = t
			return t, true
		}
	}
	return almacen.TipoOperacion{}, false
}
