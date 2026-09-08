package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/listaprecio"
)

// --- Listas de precio (maestro editable) ---

type ListaPrecioRepo struct {
	mu    sync.Mutex
	items []listaprecio.ListaPrecio
}

func NewListaPrecioRepo() *ListaPrecioRepo { return &ListaPrecioRepo{} }

func (r *ListaPrecioRepo) List(empresaID string) []listaprecio.ListaPrecio {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []listaprecio.ListaPrecio{}
	for _, l := range r.items {
		if l.EmpresaID == empresaID {
			out = append(out, l)
		}
	}
	return out
}

func (r *ListaPrecioRepo) ByID(empresaID, id string) (listaprecio.ListaPrecio, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range r.items {
		if l.EmpresaID == empresaID && l.ID == id {
			return l, true
		}
	}
	return listaprecio.ListaPrecio{}, false
}

func (r *ListaPrecioRepo) Create(l listaprecio.ListaPrecio) listaprecio.ListaPrecio {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l.ID == "" {
		l.ID = nextID("lp_")
	}
	r.items = append(r.items, l)
	return l
}

func (r *ListaPrecioRepo) Update(l listaprecio.ListaPrecio) (listaprecio.ListaPrecio, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == l.EmpresaID && cur.ID == l.ID {
			r.items[i] = l
			return l, true
		}
	}
	return listaprecio.ListaPrecio{}, false
}
