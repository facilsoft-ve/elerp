package store

import (
	"sync"

	"github.com/mornix/elerp-platform/internal/facturacion"
)

// --- Planes en memoria -----------------------------------------------------

// MemPlanes es el catálogo de planes en memoria.
type MemPlanes struct {
	mu    sync.Mutex
	items map[string]facturacion.Plan
}

func NewMemPlanes() *MemPlanes { return &MemPlanes{items: map[string]facturacion.Plan{}} }

func (m *MemPlanes) List() []facturacion.Plan {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]facturacion.Plan, 0, len(m.items))
	for _, p := range m.items {
		out = append(out, p)
	}
	return out
}

func (m *MemPlanes) ByID(id string) (facturacion.Plan, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.items[id]
	return p, ok
}

func (m *MemPlanes) Create(p facturacion.Plan) facturacion.Plan {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.ID == "" {
		p.ID = randID()
	}
	if p.Creado == "" {
		p.Creado = ahoraRFC()
	}
	m.items[p.ID] = p
	return p
}

func (m *MemPlanes) Update(p facturacion.Plan) (facturacion.Plan, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[p.ID]; !ok {
		return facturacion.Plan{}, false
	}
	m.items[p.ID] = p
	return p, true
}

// --- Suscripciones en memoria ----------------------------------------------

// MemSuscripciones almacena una suscripción por organización.
type MemSuscripciones struct {
	mu    sync.Mutex
	items map[string]facturacion.Suscripcion // por orgID
}

func NewMemSuscripciones() *MemSuscripciones {
	return &MemSuscripciones{items: map[string]facturacion.Suscripcion{}}
}

func (m *MemSuscripciones) List() []facturacion.Suscripcion {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]facturacion.Suscripcion, 0, len(m.items))
	for _, s := range m.items {
		out = append(out, s)
	}
	return out
}

func (m *MemSuscripciones) ByOrg(orgID string) (facturacion.Suscripcion, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.items[orgID]
	return s, ok
}

func (m *MemSuscripciones) Upsert(s facturacion.Suscripcion) facturacion.Suscripcion {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.Actualizada == "" {
		s.Actualizada = ahoraRFC()
	}
	m.items[s.OrgID] = s
	return s
}
