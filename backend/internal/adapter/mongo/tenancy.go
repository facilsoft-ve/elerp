package mongo

import (
	"strings"

	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/credencial"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/organizacion"
	"github.com/mornix/elerp/internal/domain/sede"
	"github.com/mornix/elerp/internal/domain/usuario"
)

func (st *Store) attachTenancy(db *gomongo.Database) {
	st.Organizaciones = &OrganizacionRepo{coll[organizacion.Organizacion]{db.Collection("organizaciones")}}
	st.Empresas = &EmpresaRepo{coll[empresa.Empresa]{db.Collection("empresas")}}
	st.Sedes = &SedeRepo{coll[sede.Sede]{db.Collection("sedes")}}
	st.Usuarios = &UsuarioRepo{coll[usuario.Usuario]{db.Collection("usuarios")}}
	st.Membresias = &MembresiaRepo{coll[usuario.Membresia]{db.Collection("membresias")}}
	st.Credenciales = &CredencialRepo{coll[credencial.Credencial]{db.Collection("credenciales")}}
}

// --- Organizaciones ---

type OrganizacionRepo struct{ c coll[organizacion.Organizacion] }

func (r *OrganizacionRepo) List() []organizacion.Organizacion { return r.c.all(map[string]any{}) }
func (r *OrganizacionRepo) ByID(id string) (organizacion.Organizacion, bool) {
	return r.c.one(map[string]any{"id": id})
}
func (r *OrganizacionRepo) Create(o organizacion.Organizacion) organizacion.Organizacion {
	if o.ID == "" {
		o.ID = newID("org_")
	}
	r.c.insert(o)
	return o
}
func (r *OrganizacionRepo) Update(o organizacion.Organizacion) (organizacion.Organizacion, bool) {
	r.c.replace(o.ID, o)
	return o, true
}

// --- Empresas ---

type EmpresaRepo struct{ c coll[empresa.Empresa] }

func (r *EmpresaRepo) List(organizacionID string) []empresa.Empresa {
	return r.c.all(map[string]any{"organizacionid": organizacionID})
}
func (r *EmpresaRepo) ByID(id string) (empresa.Empresa, bool) {
	return r.c.one(map[string]any{"id": id})
}
func (r *EmpresaRepo) Create(e empresa.Empresa) empresa.Empresa {
	if e.ID == "" {
		e.ID = newID("emp_")
	}
	r.c.insert(e)
	return e
}
func (r *EmpresaRepo) Update(e empresa.Empresa) (empresa.Empresa, bool) {
	r.c.replace(e.ID, e)
	return e, true
}

// --- Sedes ---

type SedeRepo struct{ c coll[sede.Sede] }

func (r *SedeRepo) List(empresaID string) []sede.Sede {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *SedeRepo) ByID(empresaID, id string) (sede.Sede, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *SedeRepo) Create(s sede.Sede) sede.Sede {
	if s.ID == "" {
		s.ID = newID("sede_")
	}
	r.c.insert(s)
	return s
}
func (r *SedeRepo) Update(s sede.Sede) (sede.Sede, bool) {
	r.c.replace(s.ID, s)
	return s, true
}

// --- Usuarios ---

type UsuarioRepo struct{ c coll[usuario.Usuario] }

func (r *UsuarioRepo) ByID(id string) (usuario.Usuario, bool) {
	return r.c.one(map[string]any{"id": id})
}
func (r *UsuarioRepo) ByEmail(email string) (usuario.Usuario, bool) {
	return r.c.one(map[string]any{"email": strings.ToLower(email)})
}
func (r *UsuarioRepo) Create(u usuario.Usuario) usuario.Usuario {
	if u.ID == "" {
		u.ID = newID("usr_")
	}
	u.Email = strings.ToLower(u.Email)
	r.c.insert(u)
	return u
}
func (r *UsuarioRepo) Update(u usuario.Usuario) (usuario.Usuario, bool) {
	r.c.replace(u.ID, u)
	return u, true
}

// --- Membresías ---

type MembresiaRepo struct{ c coll[usuario.Membresia] }

func (r *MembresiaRepo) ByUsuario(usuarioID string) []usuario.Membresia {
	return r.c.all(map[string]any{"usuarioid": usuarioID})
}
func (r *MembresiaRepo) ByEmpresa(empresaID string) []usuario.Membresia {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *MembresiaRepo) ByEmail(email string) []usuario.Membresia {
	return r.c.all(map[string]any{"email": strings.ToLower(email)})
}
func (r *MembresiaRepo) ByToken(token string) (usuario.Membresia, bool) {
	if token == "" {
		return usuario.Membresia{}, false
	}
	return r.c.one(map[string]any{"token": token})
}
func (r *MembresiaRepo) Create(m usuario.Membresia) usuario.Membresia {
	if m.ID == "" {
		m.ID = newID("mem_")
	}
	m.Email = strings.ToLower(m.Email)
	r.c.insert(m)
	return m
}
func (r *MembresiaRepo) Update(m usuario.Membresia) (usuario.Membresia, bool) {
	r.c.replace(m.ID, m)
	return m, true
}
func (r *MembresiaRepo) Delete(id string) bool {
	return r.c.del(map[string]any{"id": id})
}

// --- Credenciales ---

type CredencialRepo struct{ c coll[credencial.Credencial] }

func (r *CredencialRepo) ByEmail(email string) (credencial.Credencial, bool) {
	return r.c.one(map[string]any{"email": strings.ToLower(email)})
}
func (r *CredencialRepo) Create(c credencial.Credencial) credencial.Credencial {
	c.Email = strings.ToLower(c.Email)
	r.c.insert(c)
	return c
}
