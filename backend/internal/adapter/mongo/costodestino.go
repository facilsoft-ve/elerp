package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/compra"
)

func (st *Store) attachCostosEnDestino(db *gomongo.Database) {
	st.CostosEnDestino = &CostoEnDestinoRepo{coll[compra.CostoEnDestino]{db.Collection("costos_destino")}}
}

// CostoEnDestinoRepo persiste los costos en destino con filtro empresaid. Sin
// Update ni Delete: es de solo anexado (ver domain/compra/costodestino.go).
type CostoEnDestinoRepo struct {
	c coll[compra.CostoEnDestino]
}

func (r *CostoEnDestinoRepo) List(empresaID string) []compra.CostoEnDestino {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *CostoEnDestinoRepo) PorOrden(empresaID, ordenID string) []compra.CostoEnDestino {
	return r.c.all(map[string]any{"empresaid": empresaID, "ordencompraid": ordenID})
}

func (r *CostoEnDestinoRepo) Append(c compra.CostoEnDestino) compra.CostoEnDestino {
	if c.ID == "" {
		c.ID = newID("cd_")
	}
	r.c.insert(c)
	return c
}
