package inmem

import (
	"sort"
	"sync"

	"github.com/mornix/elerp/internal/domain/mesonero"
)

// --- Credenciales de mesonero (módulo Restaurante) ---
//
// Mismo contrato que el de Mongo: filtro por empresa en TODAS las consultas.

type MesoneroRepo struct {
	mu    sync.RWMutex
	items []mesonero.Mesonero
}

func NewMesoneroRepo() *MesoneroRepo { return &MesoneroRepo{} }

// porCodigo deja la grilla del salón en orden estable: MS-001, MS-002, …
func porCodigo(xs []mesonero.Mesonero) []mesonero.Mesonero {
	sort.SliceStable(xs, func(i, j int) bool { return xs[i].Codigo < xs[j].Codigo })
	return xs
}

func (r *MesoneroRepo) List(empresaID string) []mesonero.Mesonero {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []mesonero.Mesonero{}
	for _, m := range r.items {
		if m.EmpresaID == empresaID {
			out = append(out, m)
		}
	}
	return porCodigo(out)
}

func (r *MesoneroRepo) ByID(empresaID, id string) (mesonero.Mesonero, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.items {
		if m.EmpresaID == empresaID && m.ID == id {
			return m, true
		}
	}
	return mesonero.Mesonero{}, false
}

func (r *MesoneroRepo) ByCodigo(empresaID, codigo string) (mesonero.Mesonero, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.items {
		if m.EmpresaID == empresaID && m.Codigo == codigo {
			return m, true
		}
	}
	return mesonero.Mesonero{}, false
}

func (r *MesoneroRepo) ByUsuario(empresaID, usuarioID string) (mesonero.Mesonero, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if usuarioID == "" {
		return mesonero.Mesonero{}, false
	}
	for _, m := range r.items {
		if m.EmpresaID == empresaID && m.UsuarioID == usuarioID {
			return m, true
		}
	}
	return mesonero.Mesonero{}, false
}

func (r *MesoneroRepo) Create(m mesonero.Mesonero) mesonero.Mesonero {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ID == "" {
		m.ID = nextID("msn_")
	}
	r.items = append(r.items, m)
	return m
}

func (r *MesoneroRepo) Update(m mesonero.Mesonero) (mesonero.Mesonero, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == m.EmpresaID && x.ID == m.ID {
			r.items[i] = m
			return m, true
		}
	}
	return mesonero.Mesonero{}, false
}

// --- Turnos del salón ---

type TurnoRepo struct {
	mu    sync.RWMutex
	items []mesonero.Turno
}

func NewTurnoRepo() *TurnoRepo { return &TurnoRepo{} }

func (r *TurnoRepo) Vivos(empresaID, sedeID string) []mesonero.Turno {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []mesonero.Turno{}
	for _, t := range r.items {
		if t.EmpresaID == empresaID && (sedeID == "" || t.SedeID == sedeID) && t.Vivo() {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Apertura < out[j].Apertura })
	return out
}

func (r *TurnoRepo) VivoDeMesonero(empresaID, mesoneroID string) (mesonero.Turno, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if mesoneroID == "" {
		return mesonero.Turno{}, false
	}
	for _, t := range r.items {
		if t.EmpresaID == empresaID && t.MesoneroID == mesoneroID && t.Vivo() {
			return t, true
		}
	}
	return mesonero.Turno{}, false
}

func (r *TurnoRepo) ByID(empresaID, id string) (mesonero.Turno, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.items {
		if t.EmpresaID == empresaID && t.ID == id {
			return t, true
		}
	}
	return mesonero.Turno{}, false
}

// Historico va del más reciente al más viejo: un turno se consulta para ver lo
// de anoche, no lo del año pasado.
func (r *TurnoRepo) Historico(empresaID, sedeID string) []mesonero.Turno {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []mesonero.Turno{}
	for _, t := range r.items {
		if t.EmpresaID == empresaID && (sedeID == "" || t.SedeID == sedeID) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Apertura > out[j].Apertura })
	return out
}

func (r *TurnoRepo) Create(t mesonero.Turno) mesonero.Turno {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = nextID("trn_")
	}
	r.items = append(r.items, t)
	return t
}

func (r *TurnoRepo) Update(t mesonero.Turno) (mesonero.Turno, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == t.EmpresaID && x.ID == t.ID {
			r.items[i] = t
			return t, true
		}
	}
	return mesonero.Turno{}, false
}

// --- Horarios del salón ---

type HorarioRepo struct {
	mu    sync.RWMutex
	items []mesonero.Horario
}

func NewHorarioRepo() *HorarioRepo { return &HorarioRepo{} }

func (r *HorarioRepo) List(empresaID, sedeID string) []mesonero.Horario {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []mesonero.Horario{}
	for _, h := range r.items {
		if h.EmpresaID == empresaID && (sedeID == "" || h.SedeID == sedeID) {
			out = append(out, h)
		}
	}
	return out
}

func (r *HorarioRepo) ByMesonero(empresaID, mesoneroID string) (mesonero.Horario, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if mesoneroID == "" {
		return mesonero.Horario{}, false
	}
	for _, h := range r.items {
		if h.EmpresaID == empresaID && h.MesoneroID == mesoneroID {
			return h, true
		}
	}
	return mesonero.Horario{}, false
}

// Upsert: uno por mesonero. Reemplaza el existente en vez de acumular.
func (r *HorarioRepo) Upsert(h mesonero.Horario) mesonero.Horario {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == h.EmpresaID && x.MesoneroID == h.MesoneroID {
			r.items[i] = h
			return h
		}
	}
	r.items = append(r.items, h)
	return h
}

func (r *HorarioRepo) Delete(empresaID, mesoneroID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.EmpresaID == empresaID && x.MesoneroID == mesoneroID {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return true
		}
	}
	return false
}
