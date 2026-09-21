package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/almacen"
)

// --- Ubicaciones dentro del almacén ---

type UbicacionRepo struct {
	mu    sync.RWMutex
	items []almacen.Ubicacion
}

func NewUbicacionRepo() *UbicacionRepo { return &UbicacionRepo{} }

func (r *UbicacionRepo) List(empresaID string) []almacen.Ubicacion {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []almacen.Ubicacion{}
	for _, u := range r.items {
		if u.EmpresaID == empresaID {
			out = append(out, u)
		}
	}
	return out
}

func (r *UbicacionRepo) ByID(empresaID, id string) (almacen.Ubicacion, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.items {
		if u.EmpresaID == empresaID && u.ID == id {
			return u, true
		}
	}
	return almacen.Ubicacion{}, false
}

func (r *UbicacionRepo) Create(u almacen.Ubicacion) almacen.Ubicacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u.ID == "" {
		u.ID = nextID("ubi_")
	}
	r.items = append(r.items, u)
	return u
}

func (r *UbicacionRepo) Update(u almacen.Ubicacion) (almacen.Ubicacion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == u.EmpresaID && x.ID == u.ID {
			r.items[i] = u
			return u, true
		}
	}
	return almacen.Ubicacion{}, false
}
