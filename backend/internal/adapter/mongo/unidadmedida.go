package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/unidadmedida"
)

func (st *Store) attachUnidades(db *gomongo.Database) {
	st.Unidades = &UnidadMedidaRepo{coll[unidadmedida.UnidadMedida]{db.Collection("unidades")}}
}

// UnidadMedidaRepo persiste unidades de medida con filtro empresaid obligatorio.
type UnidadMedidaRepo struct {
	c coll[unidadmedida.UnidadMedida]
}

func (r *UnidadMedidaRepo) List(empresaID string) []unidadmedida.UnidadMedida {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *UnidadMedidaRepo) ByID(empresaID, id string) (unidadmedida.UnidadMedida, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *UnidadMedidaRepo) Create(u unidadmedida.UnidadMedida) unidadmedida.UnidadMedida {
	if u.ID == "" {
		u.ID = newID("um_")
	}
	r.c.insert(u)
	return u
}
func (r *UnidadMedidaRepo) Update(u unidadmedida.UnidadMedida) (unidadmedida.UnidadMedida, bool) {
	if _, ok := r.ByID(u.EmpresaID, u.ID); !ok {
		return unidadmedida.UnidadMedida{}, false
	}
	r.c.replace(u.ID, u)
	return u, true
}
