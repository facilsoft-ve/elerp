package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/aplicacion"
)

func (st *Store) attachModulos(db *gomongo.Database) {
	st.Modulos = &ModuloRepo{coll[aplicacion.Instalacion]{db.Collection("modulos")}}
}

// ModuloRepo persiste el estado de módulos por empresa. La clave lógica es
// empresaid+moduloid (no hay campo id): Upsert reemplaza por ese par.
type ModuloRepo struct {
	c coll[aplicacion.Instalacion]
}

func (r *ModuloRepo) List(empresaID string) []aplicacion.Instalacion {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *ModuloRepo) ByID(empresaID, moduloID string) (aplicacion.Instalacion, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "moduloid": moduloID})
}
func (r *ModuloRepo) Upsert(i aplicacion.Instalacion) aplicacion.Instalacion {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx,
		map[string]any{"empresaid": i.EmpresaID, "moduloid": i.ModuloID},
		i, options.Replace().SetUpsert(true))
	return i
}
