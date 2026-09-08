package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/cotizacion"
)

func (st *Store) attachCotizacion(db *gomongo.Database) {
	st.Cotizaciones = &CotizacionRepo{coll[cotizacion.Cotizacion]{db.Collection("cotizaciones")}}
}

// CotizacionRepo persiste cotizaciones con filtro empresaid obligatorio.
type CotizacionRepo struct{ c coll[cotizacion.Cotizacion] }

func (r *CotizacionRepo) List(empresaID string) []cotizacion.Cotizacion {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *CotizacionRepo) ByID(empresaID, id string) (cotizacion.Cotizacion, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *CotizacionRepo) Create(c cotizacion.Cotizacion) cotizacion.Cotizacion {
	if c.ID == "" {
		c.ID = newID("cot_")
	}
	r.c.insert(c)
	return c
}
func (r *CotizacionRepo) Update(c cotizacion.Cotizacion) (cotizacion.Cotizacion, bool) {
	if _, ok := r.ByID(c.EmpresaID, c.ID); !ok {
		return cotizacion.Cotizacion{}, false
	}
	r.c.replace(c.ID, c)
	return c, true
}
