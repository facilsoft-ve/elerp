package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/mesa"
)

func (st *Store) attachMesas(db *gomongo.Database) {
	st.Mesas = &MesaRepo{coll[mesa.Mesa]{db.Collection("mesas")}}
}

// MesaRepo persiste mesas con filtro empresaid obligatorio.
type MesaRepo struct {
	c coll[mesa.Mesa]
}

func (r *MesaRepo) List(empresaID, sedeID string) []mesa.Mesa {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}
func (r *MesaRepo) ByID(empresaID, id string) (mesa.Mesa, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *MesaRepo) Create(m mesa.Mesa) mesa.Mesa {
	if m.ID == "" {
		m.ID = newID("mesa_")
	}
	r.c.insert(m)
	return m
}
func (r *MesaRepo) Update(m mesa.Mesa) (mesa.Mesa, bool) {
	if _, ok := r.ByID(m.EmpresaID, m.ID); !ok {
		return mesa.Mesa{}, false
	}
	r.c.replace(m.ID, m)
	return m, true
}
func (r *MesaRepo) Delete(empresaID, id string) bool {
	return r.c.del(map[string]any{"empresaid": empresaID, "id": id})
}

func (st *Store) attachPlanos(db *gomongo.Database) {
	st.Planos = &PlanoRepo{coll[mesa.Plano]{db.Collection("planos_salon")}}
}

// PlanoRepo persiste el plano (grilla) por (empresa, sede). Upsert por ese par.
type PlanoRepo struct {
	c coll[mesa.Plano]
}

func (r *PlanoRepo) Get(empresaID, sedeID string) (mesa.Plano, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "sedeid": sedeID})
}
func (r *PlanoRepo) Upsert(p mesa.Plano) mesa.Plano {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx,
		map[string]any{"empresaid": p.EmpresaID, "sedeid": p.SedeID},
		p, options.Replace().SetUpsert(true))
	return p
}

func (st *Store) attachAsignaciones(db *gomongo.Database) {
	st.Asignaciones = &AsignacionRepo{coll[mesa.Asignacion]{db.Collection("asignaciones_mesas")}}
	st.ConfigSalon = &ConfigSalonRepo{coll[mesa.ConfigSalon]{db.Collection("config_salon")}}
}

// AsignacionRepo persiste la asignación de mesas/zonas a cada mesonero. Una por
// (empresa, sede, usuario): Upsert por ese trío. Filtro empresaid obligatorio.
type AsignacionRepo struct {
	c coll[mesa.Asignacion]
}

func (r *AsignacionRepo) List(empresaID, sedeID string) []mesa.Asignacion {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}

func (r *AsignacionRepo) Upsert(a mesa.Asignacion) mesa.Asignacion {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx,
		map[string]any{"empresaid": a.EmpresaID, "sedeid": a.SedeID, "usuarioid": a.UsuarioID},
		a, options.Replace().SetUpsert(true))
	return a
}

func (r *AsignacionRepo) Delete(empresaID, sedeID, usuarioID string) bool {
	return r.c.del(map[string]any{"empresaid": empresaID, "sedeid": sedeID, "usuarioid": usuarioID})
}

// ConfigSalonRepo persiste la configuración del módulo por (empresa, sede).
type ConfigSalonRepo struct {
	c coll[mesa.ConfigSalon]
}

func (r *ConfigSalonRepo) Get(empresaID, sedeID string) (mesa.ConfigSalon, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "sedeid": sedeID})
}

func (r *ConfigSalonRepo) Upsert(c mesa.ConfigSalon) mesa.ConfigSalon {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx,
		map[string]any{"empresaid": c.EmpresaID, "sedeid": c.SedeID},
		c, options.Replace().SetUpsert(true))
	return c
}
