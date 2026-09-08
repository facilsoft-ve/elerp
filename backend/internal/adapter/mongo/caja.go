package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/caja"
)

// attachCaja cablea los repos de caja a sus colecciones. Toda consulta lleva el
// filtro obligatorio por empresaid: es el aislamiento de tenant, que en Mongo
// vive en esta capa porque no hay Row-Level Security.
func (st *Store) attachCaja(db *gomongo.Database) {
	st.Cajas = &CajaRepo{coll[caja.Caja]{db.Collection("cajas")}}
	st.Cajeros = &CajeroRepo{coll[caja.Cajero]{db.Collection("cajeros")}}
	st.SesionesCaja = &SesionCajaRepo{coll[caja.Sesion]{db.Collection("sesionescaja")}}
}

// --- Cajas ---

type CajaRepo struct{ c coll[caja.Caja] }

func (r *CajaRepo) List(empresaID string) []caja.Caja {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *CajaRepo) ByID(empresaID, id string) (caja.Caja, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *CajaRepo) Create(c caja.Caja) caja.Caja {
	if c.ID == "" {
		c.ID = newID("caja_")
	}
	r.c.insert(c)
	return c
}
func (r *CajaRepo) Update(c caja.Caja) (caja.Caja, bool) {
	r.c.replace(c.ID, c)
	return c, true
}

// --- Cajeros ---

type CajeroRepo struct{ c coll[caja.Cajero] }

func (r *CajeroRepo) List(empresaID string) []caja.Cajero {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *CajeroRepo) ByCodigo(empresaID, codigo string) (caja.Cajero, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "codigo": codigo})
}
func (r *CajeroRepo) Create(c caja.Cajero) caja.Cajero {
	if c.ID == "" {
		c.ID = newID("cjr_")
	}
	r.c.insert(c)
	return c
}
func (r *CajeroRepo) Update(c caja.Cajero) (caja.Cajero, bool) {
	r.c.replace(c.ID, c)
	return c, true
}

// --- Sesiones de caja ---

type SesionCajaRepo struct{ c coll[caja.Sesion] }

// Una sesión abierta es la que no tiene cierre: cierre == "".
func (r *SesionCajaRepo) Abiertas(empresaID, sedeID string) []caja.Sesion {
	f := map[string]any{"empresaid": empresaID, "cierre": ""}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}
func (r *SesionCajaRepo) AbiertaDeCaja(empresaID, cajaID string) (caja.Sesion, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "cajaid": cajaID, "cierre": ""})
}
func (r *SesionCajaRepo) AbiertaDeActor(empresaID, actorID string) (caja.Sesion, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "actorid": actorID, "cierre": ""})
}
func (r *SesionCajaRepo) List(empresaID string) []caja.Sesion {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *SesionCajaRepo) Create(s caja.Sesion) caja.Sesion {
	if s.ID == "" {
		s.ID = newID("ses_")
	}
	r.c.insert(s)
	return s
}
func (r *SesionCajaRepo) Update(s caja.Sesion) (caja.Sesion, bool) {
	r.c.replace(s.ID, s)
	return s, true
}
