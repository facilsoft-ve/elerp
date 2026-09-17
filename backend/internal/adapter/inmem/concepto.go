package inmem

import (
	"sort"
	"sync"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// --- Maestro de conceptos ISLR ---

type ConceptoISLRRepo struct {
	mu    sync.RWMutex
	items []fiscal.ConceptoISLR
}

func NewConceptoISLRRepo() *ConceptoISLRRepo { return &ConceptoISLRRepo{} }

func (r *ConceptoISLRRepo) List(empresaID string) []fiscal.ConceptoISLR {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []fiscal.ConceptoISLR{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	// Por concepto y, dentro de él, por sujeto: es como se lee la tabla del
	// reglamento (misma actividad, las dos tarifas juntas).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Codigo != out[j].Codigo {
			return out[i].Codigo < out[j].Codigo
		}
		return out[i].Sujeto < out[j].Sujeto
	})
	return out
}

func (r *ConceptoISLRRepo) ByID(empresaID, id string) (fiscal.ConceptoISLR, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.ID == id {
			return c, true
		}
	}
	return fiscal.ConceptoISLR{}, false
}

func (r *ConceptoISLRRepo) Create(c fiscal.ConceptoISLR) fiscal.ConceptoISLR {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cis_")
	}
	r.items = append(r.items, c)
	return c
}

func (r *ConceptoISLRRepo) Update(c fiscal.ConceptoISLR) (fiscal.ConceptoISLR, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == c.EmpresaID && x.ID == c.ID {
			r.items[i] = c
			return c, true
		}
	}
	return fiscal.ConceptoISLR{}, false
}
