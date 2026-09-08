package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/mesa"
)

// --- Mesas del salón (maestro editable, módulo Restaurante) ---

type MesaRepo struct {
	mu    sync.Mutex
	items []mesa.Mesa
}

func NewMesaRepo() *MesaRepo { return &MesaRepo{} }

func (r *MesaRepo) List(empresaID, sedeID string) []mesa.Mesa {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []mesa.Mesa{}
	for _, m := range r.items {
		if m.EmpresaID == empresaID && (sedeID == "" || m.SedeID == sedeID) {
			out = append(out, m)
		}
	}
	return out
}

func (r *MesaRepo) ByID(empresaID, id string) (mesa.Mesa, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.items {
		if m.EmpresaID == empresaID && m.ID == id {
			return m, true
		}
	}
	return mesa.Mesa{}, false
}

func (r *MesaRepo) Create(m mesa.Mesa) mesa.Mesa {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ID == "" {
		m.ID = nextID("mesa_")
	}
	r.items = append(r.items, m)
	return m
}

func (r *MesaRepo) Update(m mesa.Mesa) (mesa.Mesa, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == m.EmpresaID && cur.ID == m.ID {
			r.items[i] = m
			return m, true
		}
	}
	return mesa.Mesa{}, false
}

func (r *MesaRepo) Delete(empresaID, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == empresaID && cur.ID == id {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return true
		}
	}
	return false
}

// --- Plano del salón (grilla por sede) ---

type PlanoRepo struct {
	mu    sync.Mutex
	items []mesa.Plano
}

func NewPlanoRepo() *PlanoRepo { return &PlanoRepo{} }

func (r *PlanoRepo) Get(empresaID, sedeID string) (mesa.Plano, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.SedeID == sedeID {
			return p, true
		}
	}
	return mesa.Plano{}, false
}

func (r *PlanoRepo) Upsert(p mesa.Plano) mesa.Plano {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == p.EmpresaID && cur.SedeID == p.SedeID {
			r.items[i] = p
			return p
		}
	}
	r.items = append(r.items, p)
	return p
}
