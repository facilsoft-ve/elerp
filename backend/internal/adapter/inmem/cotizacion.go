package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/cotizacion"
)

// CotizacionRepo guarda las cotizaciones en memoria, aisladas por empresaID.
type CotizacionRepo struct {
	mu    sync.Mutex
	items []cotizacion.Cotizacion
}

// NewCotizacionRepo construye el repositorio.
func NewCotizacionRepo() *CotizacionRepo { return &CotizacionRepo{} }

func (r *CotizacionRepo) List(empresaID string) []cotizacion.Cotizacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []cotizacion.Cotizacion{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CotizacionRepo) ByID(empresaID, id string) (cotizacion.Cotizacion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.ID == id {
			return c, true
		}
	}
	return cotizacion.Cotizacion{}, false
}

func (r *CotizacionRepo) Create(c cotizacion.Cotizacion) cotizacion.Cotizacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cot_")
	}
	r.items = append(r.items, c)
	return c
}

func (r *CotizacionRepo) Update(c cotizacion.Cotizacion) (cotizacion.Cotizacion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == c.EmpresaID && cur.ID == c.ID {
			r.items[i] = c
			return c, true
		}
	}
	return cotizacion.Cotizacion{}, false
}
