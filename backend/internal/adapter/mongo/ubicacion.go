package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/almacen"
)

func (st *Store) attachUbicaciones(db *gomongo.Database) {
	st.Ubicaciones = &UbicacionRepo{coll[almacen.Ubicacion]{db.Collection("ubicaciones")}}
}

// UbicacionRepo persiste las ubicaciones con filtro empresaid.
type UbicacionRepo struct {
	c coll[almacen.Ubicacion]
}

func (r *UbicacionRepo) List(empresaID string) []almacen.Ubicacion {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *UbicacionRepo) ByID(empresaID, id string) (almacen.Ubicacion, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *UbicacionRepo) Create(u almacen.Ubicacion) almacen.Ubicacion {
	if u.ID == "" {
		u.ID = newID("ubi_")
	}
	r.c.insert(u)
	return u
}

func (r *UbicacionRepo) Update(u almacen.Ubicacion) (almacen.Ubicacion, bool) {
	if _, ok := r.ByID(u.EmpresaID, u.ID); !ok {
		return almacen.Ubicacion{}, false
	}
	r.c.replace(u.ID, u)
	return u, true
}
