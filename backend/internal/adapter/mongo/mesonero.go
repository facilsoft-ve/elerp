package mongo

import (
	"sort"

	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/mesonero"
)

func (st *Store) attachMesoneros(db *gomongo.Database) {
	st.Mesoneros = &MesoneroRepo{coll[mesonero.Mesonero]{db.Collection("mesoneros")}}
	st.Turnos = &TurnoRepo{coll[mesonero.Turno]{db.Collection("turnos_salon")}}
	st.Horarios = &HorarioRepo{coll[mesonero.Horario]{db.Collection("horarios_salon")}}
}

// MesoneroRepo persiste las credenciales de mesonero con filtro empresaid
// obligatorio (Mongo no tiene Row-Level Security).
type MesoneroRepo struct {
	c coll[mesonero.Mesonero]
}

// porCodigoMsn deja la grilla del salón estable: MS-001, MS-002, …
func porCodigoMsn(xs []mesonero.Mesonero) []mesonero.Mesonero {
	sort.SliceStable(xs, func(i, j int) bool { return xs[i].Codigo < xs[j].Codigo })
	return xs
}

func (r *MesoneroRepo) List(empresaID string) []mesonero.Mesonero {
	return porCodigoMsn(r.c.all(map[string]any{"empresaid": empresaID}))
}

func (r *MesoneroRepo) ByID(empresaID, id string) (mesonero.Mesonero, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *MesoneroRepo) ByCodigo(empresaID, codigo string) (mesonero.Mesonero, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "codigo": codigo})
}

func (r *MesoneroRepo) ByUsuario(empresaID, usuarioID string) (mesonero.Mesonero, bool) {
	// Un usuarioID vacío casaría con toda credencial sin usuario: eso entregaría
	// una credencial ajena a quien no tiene ninguna.
	if usuarioID == "" {
		return mesonero.Mesonero{}, false
	}
	return r.c.one(map[string]any{"empresaid": empresaID, "usuarioid": usuarioID})
}

func (r *MesoneroRepo) Create(m mesonero.Mesonero) mesonero.Mesonero {
	if m.ID == "" {
		m.ID = newID("msn_")
	}
	r.c.insert(m)
	return m
}

func (r *MesoneroRepo) Update(m mesonero.Mesonero) (mesonero.Mesonero, bool) {
	if _, ok := r.ByID(m.EmpresaID, m.ID); !ok {
		return mesonero.Mesonero{}, false
	}
	r.c.replace(m.ID, m)
	return m, true
}

// TurnoRepo persiste los turnos del salón, con el mismo aislamiento por empresa.
type TurnoRepo struct {
	c coll[mesonero.Turno]
}

// vivos es el filtro «está trabajando ahora»: abierto o cerrando.
var vivos = map[string]any{"$in": []string{mesonero.EstadoAbierto, mesonero.EstadoCerrando}}

func (r *TurnoRepo) Vivos(empresaID, sedeID string) []mesonero.Turno {
	f := map[string]any{"empresaid": empresaID, "estado": vivos}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	out := r.c.all(f)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Apertura < out[j].Apertura })
	return out
}

func (r *TurnoRepo) VivoDeMesonero(empresaID, mesoneroID string) (mesonero.Turno, bool) {
	if mesoneroID == "" {
		return mesonero.Turno{}, false
	}
	return r.c.one(map[string]any{"empresaid": empresaID, "mesoneroid": mesoneroID, "estado": vivos})
}

func (r *TurnoRepo) ByID(empresaID, id string) (mesonero.Turno, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

// Historico va del más reciente al más viejo.
func (r *TurnoRepo) Historico(empresaID, sedeID string) []mesonero.Turno {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	out := r.c.all(f)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Apertura > out[j].Apertura })
	return out
}

func (r *TurnoRepo) Create(t mesonero.Turno) mesonero.Turno {
	if t.ID == "" {
		t.ID = newID("trn_")
	}
	r.c.insert(t)
	return t
}

func (r *TurnoRepo) Update(t mesonero.Turno) (mesonero.Turno, bool) {
	if _, ok := r.ByID(t.EmpresaID, t.ID); !ok {
		return mesonero.Turno{}, false
	}
	r.c.replace(t.ID, t)
	return t, true
}

// HorarioRepo persiste los horarios del salón. Uno por mesonero.
type HorarioRepo struct {
	c coll[mesonero.Horario]
}

func (r *HorarioRepo) List(empresaID, sedeID string) []mesonero.Horario {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}

func (r *HorarioRepo) ByMesonero(empresaID, mesoneroID string) (mesonero.Horario, bool) {
	if mesoneroID == "" {
		return mesonero.Horario{}, false
	}
	return r.c.one(map[string]any{"empresaid": empresaID, "mesoneroid": mesoneroID})
}

// Upsert reemplaza el horario de ese mesonero; si no existe, lo inserta. La
// clave es empresa+mesonero: uno por persona. Mismo patrón que AsignacionRepo.
func (r *HorarioRepo) Upsert(h mesonero.Horario) mesonero.Horario {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx,
		map[string]any{"empresaid": h.EmpresaID, "mesoneroid": h.MesoneroID},
		h, options.Replace().SetUpsert(true))
	return h
}

func (r *HorarioRepo) Delete(empresaID, mesoneroID string) bool {
	return r.c.del(map[string]any{"empresaid": empresaID, "mesoneroid": mesoneroID})
}
