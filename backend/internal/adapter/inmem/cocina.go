package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/cocina"
)

// --- Impresora de comandas por sede (módulo Restaurante) ---

type ImpresoraRepo struct {
	mu    sync.Mutex
	items []cocina.Impresora
}

func NewImpresoraRepo() *ImpresoraRepo { return &ImpresoraRepo{} }

func (r *ImpresoraRepo) Get(empresaID, sedeID string) (cocina.Impresora, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.items {
		if i.EmpresaID == empresaID && i.SedeID == sedeID {
			return i, true
		}
	}
	return cocina.Impresora{}, false
}

func (r *ImpresoraRepo) Upsert(imp cocina.Impresora) cocina.Impresora {
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx, i := range r.items {
		if i.EmpresaID == imp.EmpresaID && i.SedeID == imp.SedeID {
			r.items[idx] = imp
			return imp
		}
	}
	r.items = append(r.items, imp)
	return imp
}
