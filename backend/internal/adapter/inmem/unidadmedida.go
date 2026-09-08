package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/unidadmedida"
)

// --- Unidades de medida (maestro editable) ---

type UnidadMedidaRepo struct {
	mu    sync.Mutex
	items []unidadmedida.UnidadMedida
}

func NewUnidadMedidaRepo() *UnidadMedidaRepo { return &UnidadMedidaRepo{} }

func (r *UnidadMedidaRepo) List(empresaID string) []unidadmedida.UnidadMedida {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []unidadmedida.UnidadMedida{}
	for _, u := range r.items {
		if u.EmpresaID == empresaID {
			out = append(out, u)
		}
	}
	return out
}

func (r *UnidadMedidaRepo) ByID(empresaID, id string) (unidadmedida.UnidadMedida, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.items {
		if u.EmpresaID == empresaID && u.ID == id {
			return u, true
		}
	}
	return unidadmedida.UnidadMedida{}, false
}

func (r *UnidadMedidaRepo) Create(u unidadmedida.UnidadMedida) unidadmedida.UnidadMedida {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u.ID == "" {
		u.ID = nextID("um_")
	}
	r.items = append(r.items, u)
	return u
}

func (r *UnidadMedidaRepo) Update(u unidadmedida.UnidadMedida) (unidadmedida.UnidadMedida, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == u.EmpresaID && cur.ID == u.ID {
			r.items[i] = u
			return u, true
		}
	}
	return unidadmedida.UnidadMedida{}, false
}
