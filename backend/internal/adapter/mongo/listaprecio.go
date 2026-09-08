package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/listaprecio"
)

func (st *Store) attachListasPrecio(db *gomongo.Database) {
	st.ListasPrecio = &ListaPrecioRepo{coll[listaprecio.ListaPrecio]{db.Collection("listasprecio")}}
}

// ListaPrecioRepo persiste listas de precio con filtro empresaid obligatorio.
type ListaPrecioRepo struct{ c coll[listaprecio.ListaPrecio] }

func (r *ListaPrecioRepo) List(empresaID string) []listaprecio.ListaPrecio {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *ListaPrecioRepo) ByID(empresaID, id string) (listaprecio.ListaPrecio, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *ListaPrecioRepo) Create(l listaprecio.ListaPrecio) listaprecio.ListaPrecio {
	if l.ID == "" {
		l.ID = newID("lp_")
	}
	r.c.insert(l)
	return l
}
func (r *ListaPrecioRepo) Update(l listaprecio.ListaPrecio) (listaprecio.ListaPrecio, bool) {
	if _, ok := r.ByID(l.EmpresaID, l.ID); !ok {
		return listaprecio.ListaPrecio{}, false
	}
	r.c.replace(l.ID, l)
	return l, true
}
