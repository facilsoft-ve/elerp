package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/tesoreria"
)

// attachTesoreria cablea los ledgers de Tesorería: cobros (entra) y pagos a
// proveedor (sale).
func (st *Store) attachTesoreria(db *gomongo.Database) {
	st.Cobros = &CobroRepo{coll[tesoreria.Cobro]{db.Collection("cobros")}}
	st.PagosProveedor = &PagoProveedorRepo{coll[tesoreria.PagoProveedor]{db.Collection("pagosproveedor")}}
}

// CobroRepo persiste los cobros. Solo inserta y lee: el ledger no se edita.
type CobroRepo struct{ c coll[tesoreria.Cobro] }

func (r *CobroRepo) Append(c tesoreria.Cobro) tesoreria.Cobro {
	if c.ID == "" {
		c.ID = newID("cob_")
	}
	r.c.insert(c)
	return c
}

func (r *CobroRepo) List(empresaID string) []tesoreria.Cobro {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *CobroRepo) PorDocumento(empresaID, documentoID string) []tesoreria.Cobro {
	return r.c.all(map[string]any{"empresaid": empresaID, "documentoid": documentoID})
}

func (r *CobroRepo) ByID(empresaID, id string) (tesoreria.Cobro, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

// PagoProveedorRepo persiste los pagos a proveedor. Solo inserta y lee: el ledger
// no se edita.
type PagoProveedorRepo struct{ c coll[tesoreria.PagoProveedor] }

func (r *PagoProveedorRepo) Append(p tesoreria.PagoProveedor) tesoreria.PagoProveedor {
	if p.ID == "" {
		p.ID = newID("pgp_")
	}
	r.c.insert(p)
	return p
}

func (r *PagoProveedorRepo) List(empresaID string) []tesoreria.PagoProveedor {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *PagoProveedorRepo) ByID(empresaID, id string) (tesoreria.PagoProveedor, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
