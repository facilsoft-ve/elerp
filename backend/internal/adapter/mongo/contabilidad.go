package mongo

import (
	"github.com/mornix/elerp/internal/domain/sello"
	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/contabilidad"
)

// attachContabilidad cablea el plan de cuentas, el libro diario y los cierres.
func (st *Store) attachContabilidad(db *gomongo.Database) {
	st.CuentasContables = &CuentaContableRepo{coll[contabilidad.Cuenta]{db.Collection("plan_cuentas")}}
	st.Asientos = &AsientoRepo{coll[contabilidad.Asiento]{db.Collection("asientos")}}
	st.Periodos = &PeriodoRepo{coll[contabilidad.Periodo]{db.Collection("periodos_contables")}}
}

// CuentaContableRepo persiste el plan de cuentas.
type CuentaContableRepo struct{ c coll[contabilidad.Cuenta] }

func (r *CuentaContableRepo) List(empresaID string) []contabilidad.Cuenta {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *CuentaContableRepo) ByCodigo(empresaID, codigo string) (contabilidad.Cuenta, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "codigo": codigo})
}

func (r *CuentaContableRepo) Create(c contabilidad.Cuenta) contabilidad.Cuenta {
	if c.ID == "" {
		c.ID = newID("cta_")
	}
	r.c.insert(c)
	return c
}

func (r *CuentaContableRepo) Update(c contabilidad.Cuenta) (contabilidad.Cuenta, bool) {
	if _, ok := r.ByCodigo(c.EmpresaID, c.Codigo); !ok {
		return contabilidad.Cuenta{}, false
	}
	r.c.replace(c.ID, c)
	return c, true
}

// AsientoRepo persiste el libro diario. Solo inserta y lee.
type AsientoRepo struct{ c coll[contabilidad.Asiento] }

func (r *AsientoRepo) Append(a contabilidad.Asiento) contabilidad.Asiento {
	if a.ID == "" {
		a.ID = newID("as_")
	}
	prev := ""
	if todos := r.c.all(map[string]any{"empresaid": a.EmpresaID}); len(todos) > 0 {
		prev = todos[len(todos)-1].Hash
	}
	a.PrevHash = prev
	a.Hash = sello.Encadenar(prev, a.Contenido())
	r.c.insert(a)
	return a
}

func (r *AsientoRepo) List(empresaID string) []contabilidad.Asiento {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *AsientoRepo) ByID(empresaID, id string) (contabilidad.Asiento, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *AsientoRepo) PorRef(empresaID, refTipo, refID string) []contabilidad.Asiento {
	return r.c.all(map[string]any{"empresaid": empresaID, "reftipo": refTipo, "refid": refID})
}

// SiguienteNumero lee el mayor número del libro de la empresa y suma uno.
//
// Ojo: no es atómico como el numerador fiscal (que usa $inc). Acá es aceptable
// porque un asiento duplicado en el número no rompe la integridad del libro —el
// id es único y el asiento sigue cuadrando—, mientras que en la numeración fiscal
// un salto o un repetido sí es un problema ante el SENIAT.
func (r *AsientoRepo) SiguienteNumero(empresaID string) int {
	ctx, cancel := opctx()
	defer cancel()
	opts := options.FindOne().SetSort(map[string]any{"numero": -1})
	var ultimo contabilidad.Asiento
	if err := r.c.c.FindOne(ctx, map[string]any{"empresaid": empresaID}, opts).Decode(&ultimo); err != nil {
		return 1
	}
	return ultimo.Numero + 1
}

// PeriodoRepo persiste los cierres de período. Solo inserta y lee: un cierre no
// se reabre (§7.3).
type PeriodoRepo struct{ c coll[contabilidad.Periodo] }

func (r *PeriodoRepo) List(empresaID string) []contabilidad.Periodo {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

func (r *PeriodoRepo) Append(p contabilidad.Periodo) contabilidad.Periodo {
	if p.ID == "" {
		p.ID = newID("per_")
	}
	r.c.insert(p)
	return p
}
