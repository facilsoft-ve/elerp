package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/inventario"
)

// --- Apartados (mercancía comprometida que todavía no salió) ---

type ApartadoRepo struct {
	mu    sync.RWMutex
	items []inventario.Apartado
}

func NewApartadoRepo() *ApartadoRepo { return &ApartadoRepo{} }

func (r *ApartadoRepo) List(empresaID string) []inventario.Apartado {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []inventario.Apartado{}
	for _, a := range r.items {
		if a.EmpresaID == empresaID {
			out = append(out, a)
		}
	}
	return out
}

func (r *ApartadoRepo) ByID(empresaID, id string) (inventario.Apartado, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, a := range r.items {
		if a.EmpresaID == empresaID && a.ID == id {
			return a, true
		}
	}
	return inventario.Apartado{}, false
}

func (r *ApartadoRepo) Create(a inventario.Apartado) inventario.Apartado {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.ID == "" {
		a.ID = nextID("apt_")
	}
	r.items = append(r.items, a)
	return a
}

func (r *ApartadoRepo) Update(a inventario.Apartado) (inventario.Apartado, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == a.EmpresaID && x.ID == a.ID {
			r.items[i] = a
			return a, true
		}
	}
	return inventario.Apartado{}, false
}
