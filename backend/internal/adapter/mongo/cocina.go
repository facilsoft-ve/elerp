package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/cocina"
)

func (st *Store) attachImpresoras(db *gomongo.Database) {
	st.Impresoras = &ImpresoraRepo{coll[cocina.Impresora]{db.Collection("impresoras_cocina")}}
}

// ImpresoraRepo persiste las COMANDERAS con filtro empresaid obligatorio. Varias por
// sede (cocina, barra, postres), cada una con su id.
type ImpresoraRepo struct {
	c coll[cocina.Impresora]
}

func (r *ImpresoraRepo) List(empresaID, sedeID string) []cocina.Impresora {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}

func (r *ImpresoraRepo) ByID(empresaID, id string) (cocina.Impresora, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *ImpresoraRepo) Create(i cocina.Impresora) cocina.Impresora {
	if i.ID == "" {
		i.ID = newID("imp_")
	}
	r.c.insert(i)
	return i
}

func (r *ImpresoraRepo) Update(i cocina.Impresora) (cocina.Impresora, bool) {
	if _, ok := r.ByID(i.EmpresaID, i.ID); !ok {
		return cocina.Impresora{}, false
	}
	r.c.replace(i.ID, i)
	return i, true
}

func (r *ImpresoraRepo) Delete(empresaID, id string) bool {
	return r.c.del(map[string]any{"empresaid": empresaID, "id": id})
}
