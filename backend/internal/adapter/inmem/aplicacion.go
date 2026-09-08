package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/aplicacion"
)

// --- Aplicaciones: estado de módulos por empresa (editable) ---

type ModuloRepo struct {
	mu    sync.Mutex
	items []aplicacion.Instalacion
}

func NewModuloRepo() *ModuloRepo { return &ModuloRepo{} }

func (r *ModuloRepo) List(empresaID string) []aplicacion.Instalacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []aplicacion.Instalacion{}
	for _, i := range r.items {
		if i.EmpresaID == empresaID {
			out = append(out, i)
		}
	}
	return out
}

func (r *ModuloRepo) ByID(empresaID, moduloID string) (aplicacion.Instalacion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.items {
		if i.EmpresaID == empresaID && i.ModuloID == moduloID {
			return i, true
		}
	}
	return aplicacion.Instalacion{}, false
}

func (r *ModuloRepo) Upsert(i aplicacion.Instalacion) aplicacion.Instalacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, cur := range r.items {
		if cur.EmpresaID == i.EmpresaID && cur.ModuloID == i.ModuloID {
			r.items[k] = i
			return i
		}
	}
	r.items = append(r.items, i)
	return i
}
