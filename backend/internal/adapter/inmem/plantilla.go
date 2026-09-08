package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/plantilla"
)

// --- Formatos de documento (maestro editable) ---

type PlantillaRepo struct {
	mu    sync.Mutex
	items []plantilla.Plantilla
}

func NewPlantillaRepo() *PlantillaRepo { return &PlantillaRepo{} }

func (r *PlantillaRepo) List(empresaID string) []plantilla.Plantilla {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []plantilla.Plantilla{}
	for _, p := range r.items {
		if p.EmpresaID == empresaID {
			out = append(out, p)
		}
	}
	return out
}

func (r *PlantillaRepo) ByID(empresaID, id string) (plantilla.Plantilla, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.ID == id {
			return p, true
		}
	}
	return plantilla.Plantilla{}, false
}

func (r *PlantillaRepo) Create(p plantilla.Plantilla) plantilla.Plantilla {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = nextID("fmt_")
	}
	r.items = append(r.items, p)
	return p
}

func (r *PlantillaRepo) Update(p plantilla.Plantilla) (plantilla.Plantilla, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == p.EmpresaID && cur.ID == p.ID {
			r.items[i] = p
			return p, true
		}
	}
	return plantilla.Plantilla{}, false
}

func (r *PlantillaRepo) Delete(empresaID, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == empresaID && cur.ID == id {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return true
		}
	}
	return false
}
