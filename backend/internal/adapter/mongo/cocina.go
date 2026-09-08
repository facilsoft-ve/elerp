package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/cocina"
)

func (st *Store) attachImpresoras(db *gomongo.Database) {
	st.Impresoras = &ImpresoraRepo{coll[cocina.Impresora]{db.Collection("impresoras_cocina")}}
}

// ImpresoraRepo persiste la impresora de comandas por (empresa, sede). Clave
// lógica empresaid+sedeid (no hay campo id): Upsert reemplaza por ese par.
type ImpresoraRepo struct {
	c coll[cocina.Impresora]
}

func (r *ImpresoraRepo) Get(empresaID, sedeID string) (cocina.Impresora, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "sedeid": sedeID})
}

func (r *ImpresoraRepo) Upsert(i cocina.Impresora) cocina.Impresora {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx,
		map[string]any{"empresaid": i.EmpresaID, "sedeid": i.SedeID},
		i, options.Replace().SetUpsert(true))
	return i
}
