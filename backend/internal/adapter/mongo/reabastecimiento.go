package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/inventario"
)

func (st *Store) attachReglasReabastecimiento(db *gomongo.Database) {
	st.ReglasReabastecimiento = &ReglaReabastecimientoRepo{
		coll[inventario.ReglaReabastecimiento]{db.Collection("reglas_reabastecimiento")},
	}
}

// ReglaReabastecimientoRepo persiste las reglas con filtro empresaid.
type ReglaReabastecimientoRepo struct {
	c coll[inventario.ReglaReabastecimiento]
}

func (r *ReglaReabastecimientoRepo) List(empresaID string) []inventario.ReglaReabastecimiento {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *ReglaReabastecimientoRepo) ByID(empresaID, id string) (inventario.ReglaReabastecimiento, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *ReglaReabastecimientoRepo) Create(x inventario.ReglaReabastecimiento) inventario.ReglaReabastecimiento {
	if x.ID == "" {
		x.ID = newID("rea_")
	}
	r.c.insert(x)
	return x
}

func (r *ReglaReabastecimientoRepo) Update(x inventario.ReglaReabastecimiento) (inventario.ReglaReabastecimiento, bool) {
	if _, ok := r.ByID(x.EmpresaID, x.ID); !ok {
		return inventario.ReglaReabastecimiento{}, false
	}
	r.c.replace(x.ID, x)
	return x, true
}
