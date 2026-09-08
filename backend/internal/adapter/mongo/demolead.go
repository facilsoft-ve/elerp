package mongo

import (
	"github.com/mornix/elerp/internal/domain/demolead"
)

// DemoLeadRepo persiste las solicitudes de demo en la colección `demoleads`.
// Es un almacén global de solo-anexado: los leads son previos al login y no
// llevan filtro por empresa (a diferencia del resto de agregados de negocio).
type DemoLeadRepo struct{ c coll[demolead.DemoLead] }

func (r *DemoLeadRepo) Append(l demolead.DemoLead) demolead.DemoLead {
	if l.ID == "" {
		l.ID = newID("lead_")
	}
	r.c.insert(l)
	return l
}

func (r *DemoLeadRepo) List() []demolead.DemoLead {
	return r.c.all(map[string]any{})
}
