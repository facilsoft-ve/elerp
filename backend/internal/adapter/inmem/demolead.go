package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/demolead"
)

// DemoLeadRepo es el almacén en memoria de solicitudes de demo (solo-anexado).
// A diferencia de los repos de negocio, no filtra por empresa: los leads son
// previos al login y no pertenecen a ningún tenant.
type DemoLeadRepo struct {
	mu    sync.Mutex
	items []demolead.DemoLead
}

func NewDemoLeadRepo() *DemoLeadRepo { return &DemoLeadRepo{} }

func (r *DemoLeadRepo) Append(l demolead.DemoLead) demolead.DemoLead {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l.ID == "" {
		l.ID = nextID("lead_")
	}
	r.items = append(r.items, l)
	return l
}

func (r *DemoLeadRepo) List() []demolead.DemoLead {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]demolead.DemoLead, len(r.items))
	copy(out, r.items)
	return out
}
