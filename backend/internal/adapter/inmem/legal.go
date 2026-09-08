package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/legal"
)

// LegalRepo es la proyección en memoria de aceptaciones legales: la última por
// usuario+documento (para "¿ya aceptó?") y el historial por usuario (evidencia).
type LegalRepo struct {
	mu      sync.Mutex
	ultima  map[string]legal.Aceptacion   // clave userID+"|"+documento
	porUser map[string][]legal.Aceptacion // userID → historial
}

func NewLegalRepo() *LegalRepo {
	return &LegalRepo{ultima: map[string]legal.Aceptacion{}, porUser: map[string][]legal.Aceptacion{}}
}

func (r *LegalRepo) Ultima(userID, documento string) (legal.Aceptacion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.ultima[userID+"|"+documento]
	return a, ok
}

func (r *LegalRepo) Registrar(a legal.Aceptacion) legal.Aceptacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ultima[a.UserID+"|"+a.Documento] = a
	r.porUser[a.UserID] = append(r.porUser[a.UserID], a)
	return a
}

func (r *LegalRepo) PorUsuario(userID string) []legal.Aceptacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]legal.Aceptacion, len(r.porUser[userID]))
	copy(out, r.porUser[userID])
	return out
}
