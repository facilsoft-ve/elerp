package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/caja"
)

// --- Cajas ---

type CajaRepo struct {
	mu    sync.Mutex
	items []caja.Caja
}

func NewCajaRepo() *CajaRepo { return &CajaRepo{} }

func (r *CajaRepo) List(empresaID string) []caja.Caja {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []caja.Caja{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CajaRepo) ByID(empresaID, id string) (caja.Caja, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.ID == id {
			return c, true
		}
	}
	return caja.Caja{}, false
}

func (r *CajaRepo) Create(c caja.Caja) caja.Caja {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("caja_")
	}
	r.items = append(r.items, c)
	return c
}

func (r *CajaRepo) Update(c caja.Caja) (caja.Caja, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == c.EmpresaID && x.ID == c.ID {
			r.items[i] = c
			return c, true
		}
	}
	return caja.Caja{}, false
}

// --- Cajeros (credencial de puesto) ---

type CajeroRepo struct {
	mu    sync.Mutex
	items []caja.Cajero
}

func NewCajeroRepo() *CajeroRepo { return &CajeroRepo{} }

func (r *CajeroRepo) List(empresaID string) []caja.Cajero {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []caja.Cajero{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CajeroRepo) ByCodigo(empresaID, codigo string) (caja.Cajero, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.Codigo == codigo {
			return c, true
		}
	}
	return caja.Cajero{}, false
}

func (r *CajeroRepo) Create(c caja.Cajero) caja.Cajero {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cjr_")
	}
	r.items = append(r.items, c)
	return c
}

func (r *CajeroRepo) Update(c caja.Cajero) (caja.Cajero, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == c.EmpresaID && x.ID == c.ID {
			r.items[i] = c
			return c, true
		}
	}
	return caja.Cajero{}, false
}

// --- Sesiones de caja (turnos) ---

type SesionCajaRepo struct {
	mu    sync.Mutex
	items []caja.Sesion
}

func NewSesionCajaRepo() *SesionCajaRepo { return &SesionCajaRepo{} }

func (r *SesionCajaRepo) Abiertas(empresaID, sedeID string) []caja.Sesion {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []caja.Sesion{}
	for _, s := range r.items {
		if s.EmpresaID == empresaID && s.Abierta() && (sedeID == "" || s.SedeID == sedeID) {
			out = append(out, s)
		}
	}
	return out
}

func (r *SesionCajaRepo) AbiertaDeCaja(empresaID, cajaID string) (caja.Sesion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.items {
		if s.EmpresaID == empresaID && s.CajaID == cajaID && s.Abierta() {
			return s, true
		}
	}
	return caja.Sesion{}, false
}

func (r *SesionCajaRepo) AbiertaDeActor(empresaID, actorID string) (caja.Sesion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.items {
		if s.EmpresaID == empresaID && s.ActorID == actorID && s.Abierta() {
			return s, true
		}
	}
	return caja.Sesion{}, false
}

func (r *SesionCajaRepo) List(empresaID string) []caja.Sesion {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []caja.Sesion{}
	for _, s := range r.items {
		if s.EmpresaID == empresaID {
			out = append(out, s)
		}
	}
	return out
}

func (r *SesionCajaRepo) Create(s caja.Sesion) caja.Sesion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		s.ID = nextID("ses_")
	}
	r.items = append(r.items, s)
	return s
}

func (r *SesionCajaRepo) Update(s caja.Sesion) (caja.Sesion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == s.EmpresaID && x.ID == s.ID {
			r.items[i] = s
			return s, true
		}
	}
	return caja.Sesion{}, false
}
