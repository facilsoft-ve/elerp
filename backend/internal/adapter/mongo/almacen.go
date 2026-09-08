package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/almacen"
)

func (st *Store) attachAlmacenes(db *gomongo.Database) {
	st.Almacenes = &AlmacenRepo{coll[almacen.Almacen]{db.Collection("almacenes")}}
}

// AlmacenRepo persiste almacenes con filtro empresaid obligatorio.
type AlmacenRepo struct {
	c coll[almacen.Almacen]
}

func (r *AlmacenRepo) List(empresaID string) []almacen.Almacen {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *AlmacenRepo) ByID(empresaID, id string) (almacen.Almacen, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *AlmacenRepo) Create(a almacen.Almacen) almacen.Almacen {
	if a.ID == "" {
		a.ID = newID("alm_")
	}
	r.c.insert(a)
	return a
}
func (r *AlmacenRepo) Update(a almacen.Almacen) (almacen.Almacen, bool) {
	if _, ok := r.ByID(a.EmpresaID, a.ID); !ok {
		return almacen.Almacen{}, false
	}
	r.c.replace(a.ID, a)
	return a, true
}
