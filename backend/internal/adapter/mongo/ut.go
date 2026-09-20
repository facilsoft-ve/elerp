package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

func (st *Store) attachUnidadesTributarias(db *gomongo.Database) {
	st.UnidadesTributarias = &UnidadTributariaRepo{coll[fiscal.UnidadTributaria]{db.Collection("unidades_tributarias")}}
}

// UnidadTributariaRepo persiste el histórico de la UT con filtro empresaid. Sin
// Update ni Delete: es de solo anexado (ver domain/fiscal/ut.go).
type UnidadTributariaRepo struct {
	c coll[fiscal.UnidadTributaria]
}

func (r *UnidadTributariaRepo) List(empresaID string) []fiscal.UnidadTributaria {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *UnidadTributariaRepo) Create(u fiscal.UnidadTributaria) fiscal.UnidadTributaria {
	if u.ID == "" {
		u.ID = newID("ut_")
	}
	r.c.insert(u)
	return u
}
