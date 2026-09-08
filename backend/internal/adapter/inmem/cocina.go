package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/cocina"
)

// --- Comanderas por sede (módulo Restaurante) ---
// Varias por sede: cocina, barra, postres. Cada una con sus rubros.

type ImpresoraRepo struct {
	mu    sync.Mutex
	items []cocina.Impresora
}

func NewImpresoraRepo() *ImpresoraRepo { return &ImpresoraRepo{} }

func (r *ImpresoraRepo) List(empresaID, sedeID string) []cocina.Impresora {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []cocina.Impresora{}
	for _, i := range r.items {
		if i.EmpresaID == empresaID && (sedeID == "" || i.SedeID == sedeID) {
			out = append(out, i)
		}
	}
	return out
}

func (r *ImpresoraRepo) ByID(empresaID, id string) (cocina.Impresora, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.items {
		if i.EmpresaID == empresaID && i.ID == id {
			return i, true
		}
	}
	return cocina.Impresora{}, false
}

func (r *ImpresoraRepo) Create(imp cocina.Impresora) cocina.Impresora {
	r.mu.Lock()
	defer r.mu.Unlock()
	if imp.ID == "" {
		imp.ID = nextID("imp_")
	}
	r.items = append(r.items, imp)
	return imp
}

func (r *ImpresoraRepo) Update(imp cocina.Impresora) (cocina.Impresora, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx, i := range r.items {
		if i.EmpresaID == imp.EmpresaID && i.ID == imp.ID {
			r.items[idx] = imp
			return imp, true
		}
	}
	return cocina.Impresora{}, false
}

func (r *ImpresoraRepo) Delete(empresaID, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx, i := range r.items {
		if i.EmpresaID == empresaID && i.ID == id {
			r.items = append(r.items[:idx], r.items[idx+1:]...)
			return true
		}
	}
	return false
}
