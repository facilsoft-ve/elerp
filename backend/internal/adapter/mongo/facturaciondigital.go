package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	fd "github.com/mornix/elerp/internal/domain/facturaciondigital"
)

func (st *Store) attachFacturacionDigital(db *gomongo.Database) {
	st.ConfigDigital = &ConfigDigitalRepo{coll[fd.Config]{db.Collection("configdigital")}}
	st.EmisionesDigitales = &EmisionDigitalRepo{coll[fd.Emision]{db.Collection("emisionesdigitales")}}
}

// ConfigDigitalRepo persiste la configuración por empresa. Filtro empresaid
// obligatorio: Mongo no tiene RLS y el aislamiento se hace acá.
type ConfigDigitalRepo struct{ c coll[fd.Config] }

func (r *ConfigDigitalRepo) Get(empresaID string) (fd.Config, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID})
}

func (r *ConfigDigitalRepo) Upsert(c fd.Config) fd.Config {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx, map[string]any{"empresaid": c.EmpresaID}, c,
		options.Replace().SetUpsert(true))
	return c
}

// EmisionDigitalRepo persiste el outbox. Sin Delete: es la tabla de
// conciliación que hace falta ante cualquier reclamo.
type EmisionDigitalRepo struct{ c coll[fd.Emision] }

func (r *EmisionDigitalRepo) Append(e fd.Emision) fd.Emision {
	if e.ID == "" {
		e.ID = newID("emi_")
	}
	r.c.insert(e)
	return e
}

func (r *EmisionDigitalRepo) Update(e fd.Emision) (fd.Emision, bool) {
	ctx, cancel := opctx()
	defer cancel()
	res, err := r.c.c.ReplaceOne(ctx, map[string]any{"empresaid": e.EmpresaID, "id": e.ID}, e)
	if err != nil || res.MatchedCount == 0 {
		return fd.Emision{}, false
	}
	return e, true
}

func (r *EmisionDigitalRepo) ByID(empresaID, id string) (fd.Emision, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *EmisionDigitalRepo) ByDocumento(empresaID, documentoID string) (fd.Emision, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "documentoid": documentoID})
}

// ByToken NO filtra por empresa a propósito: es la búsqueda de la página
// pública, donde el visitante solo trae el token. Por eso el token es
// impredecible (ver el dominio) — es lo único que protege esa consulta.
func (r *EmisionDigitalRepo) ByToken(token string) (fd.Emision, bool) {
	if token == "" {
		return fd.Emision{}, false
	}
	return r.c.one(map[string]any{"token": token})
}

func (r *EmisionDigitalRepo) List(empresaID string) []fd.Emision {
	return r.c.all(map[string]any{"empresaid": empresaID})
}

// Pendientes recorre TODAS las empresas: el trabajador corre por instancia.
func (r *EmisionDigitalRepo) Pendientes(limite int) []fd.Emision {
	todas := r.c.all(map[string]any{"estado": map[string]any{
		"$in": []string{fd.EstadoPendiente, fd.EstadoEnviado, fd.EstadoErrorTemporal},
	}})
	if len(todas) > limite {
		todas = todas[:limite]
	}
	return todas
}
