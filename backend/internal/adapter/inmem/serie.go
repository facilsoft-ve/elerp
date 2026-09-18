package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// --- Configuración de correlativos por tipo de documento ---

type SerieRepo struct {
	mu    sync.RWMutex
	items []fiscal.SerieDocumento
}

func NewSerieRepo() *SerieRepo { return &SerieRepo{} }

func (r *SerieRepo) List(empresaID string) []fiscal.SerieDocumento {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []fiscal.SerieDocumento{}
	for _, s := range r.items {
		if s.EmpresaID == empresaID {
			out = append(out, s)
		}
	}
	return out
}

func (r *SerieRepo) Get(empresaID, tipo string) (fiscal.SerieDocumento, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.items {
		if s.EmpresaID == empresaID && s.Tipo == tipo {
			return s, true
		}
	}
	return fiscal.SerieDocumento{}, false
}

// Upsert reemplaza la configuración de ese tipo, o la crea. Una sola por
// (empresa, tipo): configurar no acumula historial, lo que se audita es el
// cambio.
func (r *SerieRepo) Upsert(s fiscal.SerieDocumento) fiscal.SerieDocumento {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == s.EmpresaID && cur.Tipo == s.Tipo {
			r.items[i] = s
			return s
		}
	}
	r.items = append(r.items, s)
	return s
}
