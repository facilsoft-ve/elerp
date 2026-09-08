package mongo

import (
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/legal"
)

// LegalRepo persiste las aceptaciones legales (append-only). Ultima devuelve la más
// reciente por usuario+documento (para "¿ya aceptó la versión vigente?").
type LegalRepo struct{ c coll[legal.Aceptacion] }

func (r *LegalRepo) Registrar(a legal.Aceptacion) legal.Aceptacion {
	r.c.insert(a)
	return a
}

func (r *LegalRepo) Ultima(userID, documento string) (legal.Aceptacion, bool) {
	ctx, cancel := opctx()
	defer cancel()
	var out legal.Aceptacion
	opts := options.FindOne().SetSort(map[string]any{"fecha": -1})
	if err := r.c.c.FindOne(ctx, map[string]any{"userid": userID, "documento": documento}, opts).Decode(&out); err != nil {
		return legal.Aceptacion{}, false
	}
	return out, true
}

func (r *LegalRepo) PorUsuario(userID string) []legal.Aceptacion {
	return r.c.all(map[string]any{"userid": userID})
}
