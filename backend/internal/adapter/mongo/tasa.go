package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/tasa"
)

// attachTasa cablea el histórico de tasas.
func (st *Store) attachTasa(db *gomongo.Database) {
	st.Tasas = &TasaRepo{coll[tasa.Tasa]{db.Collection("tasas")}}
}

// TasaRepo es el histórico de tasas sobre Mongo. SOLO-ANEXADO: solo inserta.
// El aislamiento se mantiene con el filtro obligatorio por `empresaid`, donde la
// cadena vacía es el ámbito de plataforma (la tasa oficial, común a todos).
type TasaRepo struct{ c coll[tasa.Tasa] }

// Append inserta un registro nuevo.
func (r *TasaRepo) Append(t tasa.Tasa) tasa.Tasa {
	if t.ID == "" {
		t.ID = newID("tasa_")
	}
	if t.Estado == "" {
		t.Estado = tasa.EstadoVigente
	}
	r.c.insert(t)
	return t
}

// UltimaVigente devuelve la tasa vigente más reciente del ámbito.
func (r *TasaRepo) UltimaVigente(empresaID string) (tasa.Tasa, bool) {
	return r.UltimaVigenteDeFuentes(empresaID)
}

// UltimaVigenteDeFuentes filtra además por fuente (sin fuentes = cualquiera). Es
// del dólar: acota a la moneda "USD" (retrocompat: el vacío/ausente cuenta como
// "USD").
func (r *TasaRepo) UltimaVigenteDeFuentes(empresaID string, fuentes ...string) (tasa.Tasa, bool) {
	return r.UltimaVigenteDeMonedaFuentes(empresaID, tasa.MonedaUSD, fuentes...)
}

// UltimaVigenteDeMonedaFuentes filtra por moneda (Moneda=="" se trata como
// "USD") y opcionalmente por fuente (sin fuentes = cualquiera).
func (r *TasaRepo) UltimaVigenteDeMonedaFuentes(empresaID, moneda string, fuentes ...string) (tasa.Tasa, bool) {
	filtro := map[string]any{"empresaid": empresaID, "estado": tasa.EstadoVigente}
	if quiere := tasa.NormalizarMoneda(moneda); quiere == tasa.MonedaUSD {
		// Los registros del dólar viejos guardan la moneda vacía o ausente; los
		// nuevos, "USD". Los tres cuentan como dólar.
		filtro["moneda"] = map[string]any{"$in": []any{"", tasa.MonedaUSD, nil}}
	} else {
		filtro["moneda"] = quiere
	}
	if len(fuentes) > 0 {
		filtro["fuente"] = map[string]any{"$in": fuentes}
	}
	items := r.buscar(filtro, 1)
	if len(items) == 0 {
		return tasa.Tasa{}, false
	}
	return items[0], true
}

// Historial devuelve el histórico del ámbito, más reciente primero, con las
// rechazadas incluidas: la cuarentena tiene que ser visible.
func (r *TasaRepo) Historial(empresaID string, limite int) []tasa.Tasa {
	if limite <= 0 {
		limite = 30
	}
	return r.buscar(map[string]any{"empresaid": empresaID}, int64(limite))
}

// ByID busca un registro dentro del ámbito.
func (r *TasaRepo) ByID(empresaID, id string) (tasa.Tasa, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

// buscar ordena por instante de obtención descendente. `obtenidaen` es RFC3339
// UTC, así que el orden lexicográfico es el cronológico.
func (r *TasaRepo) buscar(filtro map[string]any, limite int64) []tasa.Tasa {
	ctx, cancel := opctx()
	defer cancel()
	opts := options.Find().SetSort(map[string]any{"obtenidaen": -1})
	if limite > 0 {
		opts = opts.SetLimit(limite)
	}
	out := []tasa.Tasa{}
	cur, err := r.c.c.Find(ctx, filtro, opts)
	if err != nil {
		return out
	}
	defer cur.Close(ctx)
	_ = cur.All(ctx, &out)
	if out == nil {
		out = []tasa.Tasa{}
	}
	return out
}
