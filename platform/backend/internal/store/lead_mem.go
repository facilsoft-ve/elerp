package store

import (
	"sort"
	"sync"

	"github.com/mornix/elerp-platform/internal/lead"
)

// MemLeads guarda las solicitudes de demo en memoria (dev/preview).
type MemLeads struct {
	mu    sync.Mutex
	items map[string]lead.Lead
}

func NewMemLeads() *MemLeads { return &MemLeads{items: map[string]lead.Lead{}} }

// List devuelve las solicitudes MÁS NUEVAS PRIMERO (es una bandeja de entrada).
func (m *MemLeads) List() []lead.Lead {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]lead.Lead, 0, len(m.items))
	for _, l := range m.items {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Creado > out[j].Creado })
	return out
}

func (m *MemLeads) ByID(id string) (lead.Lead, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.items[id]
	return l, ok
}

func (m *MemLeads) Create(l lead.Lead) lead.Lead {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l.ID == "" {
		l.ID = randID()
	}
	if l.Creado == "" {
		l.Creado = ahoraRFC()
	}
	l.Actualizado = l.Creado
	m.items[l.ID] = l
	return l
}

func (m *MemLeads) Update(l lead.Lead) (lead.Lead, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[l.ID]; !ok {
		return lead.Lead{}, false
	}
	l.Actualizado = ahoraRFC()
	m.items[l.ID] = l
	return l, true
}
