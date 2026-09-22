package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/inventario"
)

func (st *Store) attachApartados(db *gomongo.Database) {
	st.Apartados = &ApartadoRepo{coll[inventario.Apartado]{db.Collection("apartados")}}
}

// ApartadoRepo persiste los apartados con filtro empresaid.
type ApartadoRepo struct {
	c coll[inventario.Apartado]
}

func (r *ApartadoRepo) List(empresaID string) []inventario.Apartado {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *ApartadoRepo) ByID(empresaID, id string) (inventario.Apartado, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *ApartadoRepo) Create(a inventario.Apartado) inventario.Apartado {
	if a.ID == "" {
		a.ID = newID("apt_")
	}
	r.c.insert(a)
	return a
}

func (r *ApartadoRepo) Update(a inventario.Apartado) (inventario.Apartado, bool) {
	if _, ok := r.ByID(a.EmpresaID, a.ID); !ok {
		return inventario.Apartado{}, false
	}
	r.c.replace(a.ID, a)
	return a, true
}
