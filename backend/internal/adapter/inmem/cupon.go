package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/cupon"
)

// --- Cupones de descuento (maestro editable) ---

type CuponRepo struct {
	mu    sync.Mutex
	items []cupon.Cupon
}

func NewCuponRepo() *CuponRepo { return &CuponRepo{} }

func (r *CuponRepo) List(empresaID string) []cupon.Cupon {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []cupon.Cupon{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CuponRepo) ByID(empresaID, id string) (cupon.Cupon, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.ID == id {
			return c, true
		}
	}
	return cupon.Cupon{}, false
}

func (r *CuponRepo) ByCodigo(empresaID, codigo string) (cupon.Cupon, bool) {
	cod := cupon.NormalizarCodigo(codigo)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.Codigo == cod {
			return c, true
		}
	}
	return cupon.Cupon{}, false
}

func (r *CuponRepo) Create(c cupon.Cupon) cupon.Cupon {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cup_")
	}
	r.items = append(r.items, c)
	return c
}

func (r *CuponRepo) Update(c cupon.Cupon) (cupon.Cupon, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == c.EmpresaID && cur.ID == c.ID {
			r.items[i] = c
			return c, true
		}
	}
	return cupon.Cupon{}, false
}
