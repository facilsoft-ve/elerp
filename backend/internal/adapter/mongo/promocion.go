package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/promocion"
)

func (st *Store) attachPromociones(db *gomongo.Database) {
	st.Promociones = &PromocionRepo{coll[promocion.Promocion]{db.Collection("promociones")}}
}

// PromocionRepo persiste promociones con filtro empresaid obligatorio.
type PromocionRepo struct{ c coll[promocion.Promocion] }

func (r *PromocionRepo) List(empresaID string) []promocion.Promocion {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *PromocionRepo) ByID(empresaID, id string) (promocion.Promocion, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *PromocionRepo) Create(p promocion.Promocion) promocion.Promocion {
	if p.ID == "" {
		p.ID = newID("promo_")
	}
	r.c.insert(p)
	return p
}
func (r *PromocionRepo) Update(p promocion.Promocion) (promocion.Promocion, bool) {
	if _, ok := r.ByID(p.EmpresaID, p.ID); !ok {
		return promocion.Promocion{}, false
	}
	r.c.replace(p.ID, p)
	return p, true
}
