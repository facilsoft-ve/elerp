package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// --- Histórico de la Unidad Tributaria ---
//
// Solo anexado: no hay Update ni Delete a propósito (ver domain/fiscal/ut.go).

type UnidadTributariaRepo struct {
	mu    sync.RWMutex
	items []fiscal.UnidadTributaria
}

func NewUnidadTributariaRepo() *UnidadTributariaRepo { return &UnidadTributariaRepo{} }

func (r *UnidadTributariaRepo) List(empresaID string) []fiscal.UnidadTributaria {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []fiscal.UnidadTributaria{}
	for _, u := range r.items {
		if u.EmpresaID == empresaID {
			out = append(out, u)
		}
	}
	return out
}

func (r *UnidadTributariaRepo) Create(u fiscal.UnidadTributaria) fiscal.UnidadTributaria {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u.ID == "" {
		u.ID = nextID("ut_")
	}
	r.items = append(r.items, u)
	return u
}
