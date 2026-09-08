package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/cupon"
)

func (st *Store) attachCupones(db *gomongo.Database) {
	st.Cupones = &CuponRepo{coll[cupon.Cupon]{db.Collection("cupones")}}
}

// CuponRepo persiste cupones con filtro empresaid obligatorio.
type CuponRepo struct{ c coll[cupon.Cupon] }

func (r *CuponRepo) List(empresaID string) []cupon.Cupon {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *CuponRepo) ByID(empresaID, id string) (cupon.Cupon, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *CuponRepo) ByCodigo(empresaID, codigo string) (cupon.Cupon, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "codigo": cupon.NormalizarCodigo(codigo)})
}
func (r *CuponRepo) Create(c cupon.Cupon) cupon.Cupon {
	if c.ID == "" {
		c.ID = newID("cup_")
	}
	r.c.insert(c)
	return c
}
func (r *CuponRepo) Update(c cupon.Cupon) (cupon.Cupon, bool) {
	if _, ok := r.ByID(c.EmpresaID, c.ID); !ok {
		return cupon.Cupon{}, false
	}
	r.c.replace(c.ID, c)
	return c, true
}
