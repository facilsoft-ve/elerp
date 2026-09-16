package mongo

import (
	"sort"

	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

func (st *Store) attachConceptosISLR(db *gomongo.Database) {
	st.ConceptosISLR = &ConceptoISLRRepo{coll[fiscal.ConceptoISLR]{db.Collection("conceptos_islr")}}
}

// ConceptoISLRRepo persiste el maestro de conceptos con filtro empresaid.
type ConceptoISLRRepo struct {
	c coll[fiscal.ConceptoISLR]
}

func (r *ConceptoISLRRepo) List(empresaID string) []fiscal.ConceptoISLR {
	out := r.c.all(map[string]any{"empresaid": empresaID})
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Codigo != out[j].Codigo {
			return out[i].Codigo < out[j].Codigo
		}
		return out[i].Sujeto < out[j].Sujeto
	})
	return out
}

func (r *ConceptoISLRRepo) ByID(empresaID, id string) (fiscal.ConceptoISLR, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *ConceptoISLRRepo) Create(c fiscal.ConceptoISLR) fiscal.ConceptoISLR {
	if c.ID == "" {
		c.ID = newID("cis_")
	}
	r.c.insert(c)
	return c
}

func (r *ConceptoISLRRepo) Update(c fiscal.ConceptoISLR) (fiscal.ConceptoISLR, bool) {
	if _, ok := r.ByID(c.EmpresaID, c.ID); !ok {
		return fiscal.ConceptoISLR{}, false
	}
	r.c.replace(c.ID, c)
	return c, true
}
