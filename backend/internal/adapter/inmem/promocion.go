package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/promocion"
)

// --- Promociones (maestro editable) ---

type PromocionRepo struct {
	mu    sync.Mutex
	items []promocion.Promocion
}

func NewPromocionRepo() *PromocionRepo { return &PromocionRepo{} }

func (r *PromocionRepo) List(empresaID string) []promocion.Promocion {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []promocion.Promocion{}
	for _, p := range r.items {
		if p.EmpresaID == empresaID {
			out = append(out, p)
		}
	}
	return out
}

func (r *PromocionRepo) ByID(empresaID, id string) (promocion.Promocion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.ID == id {
			return p, true
		}
	}
	return promocion.Promocion{}, false
}

func (r *PromocionRepo) Create(p promocion.Promocion) promocion.Promocion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = nextID("promo_")
	}
	r.items = append(r.items, p)
	return p
}

func (r *PromocionRepo) Update(p promocion.Promocion) (promocion.Promocion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == p.EmpresaID && cur.ID == p.ID {
			r.items[i] = p
			return p, true
		}
	}
	return promocion.Promocion{}, false
}
