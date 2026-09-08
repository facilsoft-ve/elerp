package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/almacen"
)

// --- Almacenes (maestro editable por sede) ---

type AlmacenRepo struct {
	mu    sync.Mutex
	items []almacen.Almacen
}

func NewAlmacenRepo() *AlmacenRepo { return &AlmacenRepo{} }

func (r *AlmacenRepo) List(empresaID string) []almacen.Almacen {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []almacen.Almacen{}
	for _, a := range r.items {
		if a.EmpresaID == empresaID {
			out = append(out, a)
		}
	}
	return out
}

func (r *AlmacenRepo) ByID(empresaID, id string) (almacen.Almacen, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.items {
		if a.EmpresaID == empresaID && a.ID == id {
			return a, true
		}
	}
	return almacen.Almacen{}, false
}

func (r *AlmacenRepo) Create(a almacen.Almacen) almacen.Almacen {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.ID == "" {
		a.ID = nextID("alm_")
	}
	r.items = append(r.items, a)
	return a
}

func (r *AlmacenRepo) Update(a almacen.Almacen) (almacen.Almacen, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == a.EmpresaID && cur.ID == a.ID {
			r.items[i] = a
			return a, true
		}
	}
	return almacen.Almacen{}, false
}
