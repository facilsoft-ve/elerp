package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/mesa"
)

func (st *Store) attachMesas(db *gomongo.Database) {
	st.Mesas = &MesaRepo{coll[mesa.Mesa]{db.Collection("mesas")}}
}

// MesaRepo persiste mesas con filtro empresaid obligatorio.
type MesaRepo struct {
	c coll[mesa.Mesa]
}

func (r *MesaRepo) List(empresaID, sedeID string) []mesa.Mesa {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}
func (r *MesaRepo) ByID(empresaID, id string) (mesa.Mesa, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *MesaRepo) Create(m mesa.Mesa) mesa.Mesa {
	if m.ID == "" {
		m.ID = newID("mesa_")
	}
	r.c.insert(m)
	return m
}
func (r *MesaRepo) Update(m mesa.Mesa) (mesa.Mesa, bool) {
	if _, ok := r.ByID(m.EmpresaID, m.ID); !ok {
		return mesa.Mesa{}, false
	}
	r.c.replace(m.ID, m)
	return m, true
}
func (r *MesaRepo) Delete(empresaID, id string) bool {
	return r.c.del(map[string]any{"empresaid": empresaID, "id": id})
}

func (st *Store) attachPlanos(db *gomongo.Database) {
	st.Planos = &PlanoRepo{coll[mesa.Plano]{db.Collection("planos_salon")}}
}

// PlanoRepo persiste el plano (grilla) por (empresa, sede). Upsert por ese par.
type PlanoRepo struct {
	c coll[mesa.Plano]
}

func (r *PlanoRepo) Get(empresaID, sedeID string) (mesa.Plano, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "sedeid": sedeID})
}
func (r *PlanoRepo) Upsert(p mesa.Plano) mesa.Plano {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx,
		map[string]any{"empresaid": p.EmpresaID, "sedeid": p.SedeID},
		p, options.Replace().SetUpsert(true))
	return p
}
