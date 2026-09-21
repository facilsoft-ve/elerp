package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/compra"
)

// --- Costos en destino (solo anexado) ---

type CostoEnDestinoRepo struct {
	mu    sync.RWMutex
	items []compra.CostoEnDestino
}

func NewCostoEnDestinoRepo() *CostoEnDestinoRepo { return &CostoEnDestinoRepo{} }

func (r *CostoEnDestinoRepo) List(empresaID string) []compra.CostoEnDestino {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []compra.CostoEnDestino{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CostoEnDestinoRepo) PorOrden(empresaID, ordenID string) []compra.CostoEnDestino {
	out := []compra.CostoEnDestino{}
	for _, c := range r.List(empresaID) {
		if c.OrdenCompraID == ordenID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CostoEnDestinoRepo) Append(c compra.CostoEnDestino) compra.CostoEnDestino {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cd_")
	}
	r.items = append(r.items, c)
	return c
}
