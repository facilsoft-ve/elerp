package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

func (st *Store) attachSeries(db *gomongo.Database) {
	st.Series = &SerieRepo{coll[fiscal.SerieDocumento]{db.Collection("series_documento")}}
}

// SerieRepo persiste la configuración de correlativos con el filtro empresaid
// obligatorio en toda consulta (Mongo no tiene RLS: el aislamiento se hace acá).
type SerieRepo struct {
	c coll[fiscal.SerieDocumento]
}

func (r *SerieRepo) List(empresaID string) []fiscal.SerieDocumento {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *SerieRepo) Get(empresaID, tipo string) (fiscal.SerieDocumento, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "tipo": tipo})
}

// Upsert reemplaza la configuración de ese tipo, o la crea. Una sola por
// (empresa, tipo): configurar no acumula historial, lo que se audita es el
// cambio.
func (r *SerieRepo) Upsert(s fiscal.SerieDocumento) fiscal.SerieDocumento {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx,
		map[string]any{"empresaid": s.EmpresaID, "tipo": s.Tipo},
		s, options.Replace().SetUpsert(true))
	return s
}
