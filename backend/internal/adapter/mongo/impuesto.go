package mongo

import (
	"sort"

	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

func (st *Store) attachAlicuotas(db *gomongo.Database) {
	st.Alicuotas = &AlicuotaRepo{coll[fiscal.Alicuota]{db.Collection("alicuotas")}}
}

// AlicuotaRepo persiste el maestro de impuestos con filtro empresaid obligatorio.
type AlicuotaRepo struct {
	c coll[fiscal.Alicuota]
}

func (r *AlicuotaRepo) List(empresaID string) []fiscal.Alicuota {
	out := r.c.all(map[string]any{"empresaid": empresaID})
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Codigo != out[j].Codigo {
			return out[i].Codigo < out[j].Codigo
		}
		return out[i].VigenteDesde > out[j].VigenteDesde
	})
	return out
}

func (r *AlicuotaRepo) ByID(empresaID, id string) (fiscal.Alicuota, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *AlicuotaRepo) Create(a fiscal.Alicuota) fiscal.Alicuota {
	if a.ID == "" {
		a.ID = newID("alq_")
	}
	r.c.insert(a)
	return a
}

func (r *AlicuotaRepo) Update(a fiscal.Alicuota) (fiscal.Alicuota, bool) {
	if _, ok := r.ByID(a.EmpresaID, a.ID); !ok {
		return fiscal.Alicuota{}, false
	}
	r.c.replace(a.ID, a)
	return a, true
}
