package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/inventario"
)

func (st *Store) attachPlanesConteo(db *gomongo.Database) {
	st.PlanesConteo = &PlanConteoRepo{coll[inventario.PlanConteo]{db.Collection("planes_conteo")}}
}

// PlanConteoRepo persiste los planes con filtro empresaid.
type PlanConteoRepo struct {
	c coll[inventario.PlanConteo]
}

func (r *PlanConteoRepo) List(empresaID string) []inventario.PlanConteo {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *PlanConteoRepo) ByID(empresaID, id string) (inventario.PlanConteo, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *PlanConteoRepo) Create(x inventario.PlanConteo) inventario.PlanConteo {
	if x.ID == "" {
		x.ID = newID("pcn_")
	}
	r.c.insert(x)
	return x
}

func (r *PlanConteoRepo) Update(x inventario.PlanConteo) (inventario.PlanConteo, bool) {
	if _, ok := r.ByID(x.EmpresaID, x.ID); !ok {
		return inventario.PlanConteo{}, false
	}
	r.c.replace(x.ID, x)
	return x, true
}
