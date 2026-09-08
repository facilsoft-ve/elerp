package inmem

import (
	"strings"
	"sync"

	"github.com/mornix/elerp/internal/domain/credencial"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/organizacion"
	"github.com/mornix/elerp/internal/domain/sede"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// --- Organizaciones ---

type OrganizacionRepo struct {
	mu    sync.Mutex
	items []organizacion.Organizacion
}

func NewOrganizacionRepo() *OrganizacionRepo { return &OrganizacionRepo{} }

func (r *OrganizacionRepo) List() []organizacion.Organizacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]organizacion.Organizacion(nil), r.items...)
}

func (r *OrganizacionRepo) ByID(id string) (organizacion.Organizacion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, o := range r.items {
		if o.ID == id {
			return o, true
		}
	}
	return organizacion.Organizacion{}, false
}

func (r *OrganizacionRepo) Create(o organizacion.Organizacion) organizacion.Organizacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if o.ID == "" {
		o.ID = nextID("org_")
	}
	r.items = append(r.items, o)
	return o
}

func (r *OrganizacionRepo) Update(o organizacion.Organizacion) (organizacion.Organizacion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.ID == o.ID {
			r.items[i] = o
			return o, true
		}
	}
	return organizacion.Organizacion{}, false
}

// --- Empresas ---

type EmpresaRepo struct {
	mu    sync.Mutex
	items []empresa.Empresa
}

func NewEmpresaRepo() *EmpresaRepo { return &EmpresaRepo{} }

func (r *EmpresaRepo) List(organizacionID string) []empresa.Empresa {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []empresa.Empresa{}
	for _, e := range r.items {
		if e.OrganizacionID == organizacionID {
			out = append(out, e)
		}
	}
	return out
}

func (r *EmpresaRepo) ByID(id string) (empresa.Empresa, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.items {
		if e.ID == id {
			return e, true
		}
	}
	return empresa.Empresa{}, false
}

func (r *EmpresaRepo) Create(e empresa.Empresa) empresa.Empresa {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.ID == "" {
		e.ID = nextID("emp_")
	}
	r.items = append(r.items, e)
	return e
}

func (r *EmpresaRepo) Update(e empresa.Empresa) (empresa.Empresa, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.ID == e.ID {
			r.items[i] = e
			return e, true
		}
	}
	return empresa.Empresa{}, false
}

// --- Sedes ---

type SedeRepo struct {
	mu    sync.Mutex
	items []sede.Sede
}

func NewSedeRepo() *SedeRepo { return &SedeRepo{} }

func (r *SedeRepo) List(empresaID string) []sede.Sede {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []sede.Sede{}
	for _, s := range r.items {
		if s.EmpresaID == empresaID {
			out = append(out, s)
		}
	}
	return out
}

func (r *SedeRepo) ByID(empresaID, id string) (sede.Sede, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.items {
		if s.EmpresaID == empresaID && s.ID == id {
			return s, true
		}
	}
	return sede.Sede{}, false
}

func (r *SedeRepo) Create(s sede.Sede) sede.Sede {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		s.ID = nextID("sede_")
	}
	r.items = append(r.items, s)
	return s
}

func (r *SedeRepo) Update(s sede.Sede) (sede.Sede, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == s.EmpresaID && cur.ID == s.ID {
			r.items[i] = s
			return s, true
		}
	}
	return sede.Sede{}, false
}

// --- Usuarios ---

type UsuarioRepo struct {
	mu    sync.Mutex
	items []usuario.Usuario
}

func NewUsuarioRepo() *UsuarioRepo { return &UsuarioRepo{} }

// Todos vuelca los usuarios del store. No es parte del puerto de dominio: lo usa
// el Snapshot para que el adaptador Mongo siembre desde la misma fuente.
func (r *UsuarioRepo) Todos() []usuario.Usuario {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]usuario.Usuario, len(r.items))
	copy(out, r.items)
	return out
}

func (r *UsuarioRepo) ByID(id string) (usuario.Usuario, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.items {
		if u.ID == id {
			return u, true
		}
	}
	return usuario.Usuario{}, false
}

func (r *UsuarioRepo) ByEmail(email string) (usuario.Usuario, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	email = strings.ToLower(email)
	for _, u := range r.items {
		if strings.ToLower(u.Email) == email {
			return u, true
		}
	}
	return usuario.Usuario{}, false
}

func (r *UsuarioRepo) Create(u usuario.Usuario) usuario.Usuario {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u.ID == "" {
		u.ID = nextID("usr_")
	}
	r.items = append(r.items, u)
	return u
}

func (r *UsuarioRepo) Update(u usuario.Usuario) (usuario.Usuario, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.ID == u.ID {
			r.items[i] = u
			return u, true
		}
	}
	return usuario.Usuario{}, false
}

// --- Membresías ---

type MembresiaRepo struct {
	mu    sync.Mutex
	items []usuario.Membresia
}

func NewMembresiaRepo() *MembresiaRepo { return &MembresiaRepo{} }

func (r *MembresiaRepo) ByUsuario(usuarioID string) []usuario.Membresia {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []usuario.Membresia{}
	for _, m := range r.items {
		if m.UsuarioID == usuarioID {
			out = append(out, m)
		}
	}
	return out
}

func (r *MembresiaRepo) ByEmpresa(empresaID string) []usuario.Membresia {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []usuario.Membresia{}
	for _, m := range r.items {
		if m.EmpresaID == empresaID {
			out = append(out, m)
		}
	}
	return out
}

func (r *MembresiaRepo) ByEmail(email string) []usuario.Membresia {
	r.mu.Lock()
	defer r.mu.Unlock()
	email = strings.ToLower(email)
	out := []usuario.Membresia{}
	for _, m := range r.items {
		if strings.ToLower(m.Email) == email {
			out = append(out, m)
		}
	}
	return out
}

func (r *MembresiaRepo) ByToken(token string) (usuario.Membresia, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.items {
		if m.Token != "" && m.Token == token {
			return m, true
		}
	}
	return usuario.Membresia{}, false
}

func (r *MembresiaRepo) Create(m usuario.Membresia) usuario.Membresia {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ID == "" {
		m.ID = nextID("mem_")
	}
	r.items = append(r.items, m)
	return m
}

func (r *MembresiaRepo) Update(m usuario.Membresia) (usuario.Membresia, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.ID == m.ID {
			r.items[i] = m
			return m, true
		}
	}
	return usuario.Membresia{}, false
}

func (r *MembresiaRepo) Delete(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.ID == id {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return true
		}
	}
	return false
}

// --- Credenciales ---

type CredencialRepo struct {
	mu    sync.Mutex
	items []credencial.Credencial
}

func NewCredencialRepo() *CredencialRepo { return &CredencialRepo{} }

func (r *CredencialRepo) ByEmail(email string) (credencial.Credencial, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	email = strings.ToLower(email)
	for _, c := range r.items {
		if strings.ToLower(c.Email) == email {
			return c, true
		}
	}
	return credencial.Credencial{}, false
}

func (r *CredencialRepo) Create(c credencial.Credencial) credencial.Credencial {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, c)
	return c
}

// Todas devuelve todas las credenciales locales. La usa el seed de Mongo para plantar
// de forma aditiva las credenciales de DEMOSTRACIÓN en una base ya sembrada.
func (r *CredencialRepo) Todas() []credencial.Credencial {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]credencial.Credencial, 0, len(r.items))
	for _, c := range r.items {
		out = append(out, c)
	}
	return out
}
