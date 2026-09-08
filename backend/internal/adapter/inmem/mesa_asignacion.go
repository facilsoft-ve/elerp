package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/mesa"
)

// --- Asignaciones de mesas a mesoneros ---

type AsignacionRepo struct {
	mu    sync.Mutex
	items []mesa.Asignacion
}

func NewAsignacionRepo() *AsignacionRepo { return &AsignacionRepo{} }

func (r *AsignacionRepo) List(empresaID, sedeID string) []mesa.Asignacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []mesa.Asignacion{}
	for _, a := range r.items {
		if a.EmpresaID == empresaID && (sedeID == "" || a.SedeID == sedeID) {
			out = append(out, a)
		}
	}
	return out
}

func (r *AsignacionRepo) Upsert(a mesa.Asignacion) mesa.Asignacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == a.EmpresaID && cur.SedeID == a.SedeID && cur.UsuarioID == a.UsuarioID {
			r.items[i] = a
			return a
		}
	}
	r.items = append(r.items, a)
	return a
}

func (r *AsignacionRepo) Delete(empresaID, sedeID, usuarioID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == empresaID && cur.SedeID == sedeID && cur.UsuarioID == usuarioID {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return true
		}
	}
	return false
}

// --- Configuración del salón (por sede) ---

type ConfigSalonRepo struct {
	mu    sync.Mutex
	items map[string]mesa.ConfigSalon // clave empresa|sede
}

func NewConfigSalonRepo() *ConfigSalonRepo {
	return &ConfigSalonRepo{items: map[string]mesa.ConfigSalon{}}
}

func (r *ConfigSalonRepo) Get(empresaID, sedeID string) (mesa.ConfigSalon, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.items[empresaID+"|"+sedeID]
	return c, ok
}

func (r *ConfigSalonRepo) Upsert(c mesa.ConfigSalon) mesa.ConfigSalon {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[c.EmpresaID+"|"+c.SedeID] = c
	return c
}
