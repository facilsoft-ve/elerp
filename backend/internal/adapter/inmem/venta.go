package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/venta"
)

// VentaEnEsperaRepo guarda los carritos apartados en memoria.
type VentaEnEsperaRepo struct {
	mu    sync.Mutex
	items []venta.EnEspera
}

// NewVentaEnEsperaRepo construye el repositorio.
func NewVentaEnEsperaRepo() *VentaEnEsperaRepo { return &VentaEnEsperaRepo{} }

// List devuelve las ventas apartadas del tenant, filtradas por sede si se indica.
func (r *VentaEnEsperaRepo) List(empresaID, sedeID string) []venta.EnEspera {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []venta.EnEspera{}
	for _, v := range r.items {
		if v.EmpresaID != empresaID {
			continue
		}
		if sedeID != "" && v.SedeID != sedeID {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (r *VentaEnEsperaRepo) ByID(empresaID, id string) (venta.EnEspera, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.items {
		if v.EmpresaID == empresaID && v.ID == id {
			return v, true
		}
	}
	return venta.EnEspera{}, false
}

func (r *VentaEnEsperaRepo) Create(v venta.EnEspera) venta.EnEspera {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v.ID == "" {
		v.ID = nextID("esp_")
	}
	r.items = append(r.items, v)
	return v
}

// Delete borra dentro del tenant: el id solo no alcanza para borrar.
func (r *VentaEnEsperaRepo) Delete(empresaID, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, v := range r.items {
		if v.EmpresaID == empresaID && v.ID == id {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return true
		}
	}
	return false
}
