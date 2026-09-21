package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/almacen"
)

func (st *Store) attachTiposOperacion(db *gomongo.Database) {
	st.TiposOperacion = &TipoOperacionRepo{coll[almacen.TipoOperacion]{db.Collection("tipos_operacion")}}
}

// TipoOperacionRepo persiste los tipos de operación con filtro empresaid.
type TipoOperacionRepo struct {
	c coll[almacen.TipoOperacion]
}

func (r *TipoOperacionRepo) List(empresaID string) []almacen.TipoOperacion {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *TipoOperacionRepo) ByID(empresaID, id string) (almacen.TipoOperacion, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *TipoOperacionRepo) Create(t almacen.TipoOperacion) almacen.TipoOperacion {
	if t.ID == "" {
		t.ID = newID("ope_")
	}
	r.c.insert(t)
	return t
}

func (r *TipoOperacionRepo) Update(t almacen.TipoOperacion) (almacen.TipoOperacion, bool) {
	if _, ok := r.ByID(t.EmpresaID, t.ID); !ok {
		return almacen.TipoOperacion{}, false
	}
	r.c.replace(t.ID, t)
	return t, true
}
