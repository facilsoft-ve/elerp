package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/cuenta"
)

func (st *Store) attachCuentas(db *gomongo.Database) {
	st.Cuentas = &CuentaRepo{coll[cuenta.Cuenta]{db.Collection("cuentas_mesa")}}
}

// CuentaRepo persiste cuentas de mesa con filtro empresaid obligatorio.
type CuentaRepo struct {
	c coll[cuenta.Cuenta]
}

func (r *CuentaRepo) Abiertas(empresaID, sedeID string) []cuenta.Cuenta {
	f := map[string]any{"empresaid": empresaID, "estado": cuenta.EstadoAbierta}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}
func (r *CuentaRepo) ByID(empresaID, id string) (cuenta.Cuenta, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *CuentaRepo) AbiertaDeMesa(empresaID, mesaID string) (cuenta.Cuenta, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "mesaid": mesaID, "estado": cuenta.EstadoAbierta})
}
func (r *CuentaRepo) Create(c cuenta.Cuenta) cuenta.Cuenta {
	if c.ID == "" {
		c.ID = newID("cta_")
	}
	r.c.insert(c)
	return c
}
func (r *CuentaRepo) Update(c cuenta.Cuenta) (cuenta.Cuenta, bool) {
	if _, ok := r.ByID(c.EmpresaID, c.ID); !ok {
		return cuenta.Cuenta{}, false
	}
	r.c.replace(c.ID, c)
	return c, true
}
