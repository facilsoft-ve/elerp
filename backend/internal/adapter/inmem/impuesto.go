package inmem

import (
	"sort"
	"sync"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// --- Maestro de impuestos (alícuotas de IVA con vigencia) ---

type AlicuotaRepo struct {
	mu    sync.RWMutex
	items []fiscal.Alicuota
}

func NewAlicuotaRepo() *AlicuotaRepo { return &AlicuotaRepo{} }

func (r *AlicuotaRepo) List(empresaID string) []fiscal.Alicuota {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []fiscal.Alicuota{}
	for _, a := range r.items {
		if a.EmpresaID == empresaID {
			out = append(out, a)
		}
	}
	// Orden estable para la pantalla: por código y, dentro del mismo código, de
	// la vigencia más reciente a la más vieja (arriba la que rige hoy).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Codigo != out[j].Codigo {
			return out[i].Codigo < out[j].Codigo
		}
		return out[i].VigenteDesde > out[j].VigenteDesde
	})
	return out
}

func (r *AlicuotaRepo) ByID(empresaID, id string) (fiscal.Alicuota, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, a := range r.items {
		if a.EmpresaID == empresaID && a.ID == id {
			return a, true
		}
	}
	return fiscal.Alicuota{}, false
}

func (r *AlicuotaRepo) Create(a fiscal.Alicuota) fiscal.Alicuota {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.ID == "" {
		a.ID = nextID("alq_")
	}
	r.items = append(r.items, a)
	return a
}

func (r *AlicuotaRepo) Update(a fiscal.Alicuota) (fiscal.Alicuota, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == a.EmpresaID && x.ID == a.ID {
			r.items[i] = a
			return a, true
		}
	}
	return fiscal.Alicuota{}, false
}
