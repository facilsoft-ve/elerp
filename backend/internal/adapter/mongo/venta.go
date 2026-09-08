package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/venta"
)

// attachVenta cablea las ventas en espera.
func (st *Store) attachVenta(db *gomongo.Database) {
	st.VentasEnEspera = &VentaEnEsperaRepo{coll[venta.EnEspera]{db.Collection("ventas_en_espera")}}
}

// VentaEnEsperaRepo persiste los carritos apartados. Toda consulta filtra por
// `empresaid`, incluido el borrado.
type VentaEnEsperaRepo struct{ c coll[venta.EnEspera] }

func (r *VentaEnEsperaRepo) List(empresaID, sedeID string) []venta.EnEspera {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}

func (r *VentaEnEsperaRepo) ByID(empresaID, id string) (venta.EnEspera, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *VentaEnEsperaRepo) Create(v venta.EnEspera) venta.EnEspera {
	if v.ID == "" {
		v.ID = newID("esp_")
	}
	r.c.insert(v)
	return v
}

func (r *VentaEnEsperaRepo) Delete(empresaID, id string) bool {
	return r.c.del(map[string]any{"empresaid": empresaID, "id": id})
}
