package application

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/authn"
	"github.com/mornix/elerp/internal/domain/credencial"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/organizacion"
	"github.com/mornix/elerp/internal/domain/sede"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// Errores de tenancy / auth nativa.
var (
	ErrCredencialInvalida = errors.New("correo o contraseña inválidos")
	ErrPasswordDebil      = errors.New("la contraseña debe tener al menos 8 caracteres")
	ErrEmailEnUso         = errors.New("ese correo ya tiene una cuenta")
	ErrInvitacionInvalida = errors.New("invitación inválida o expirada")
	ErrEmpresaNoExiste    = errors.New("empresa no existe")
	// ErrEmailInvalido ya se declara en crm.go (mismo paquete): se reutiliza aquí.
	ErrMiembroExistente     = errors.New("ya hay un miembro activo con ese correo en la empresa")
	ErrRolInvalido          = errors.New("rol inválido")
	ErrSedeInvalida         = errors.New("sede inválida")
	ErrInvitacionYaAceptada = errors.New("esa invitación ya fue aceptada")
)

// emailRe valida el formato del correo con la misma forma que la interfaz.
var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// nuevoToken genera un token de invitación aleatorio (URL-safe, hex).
func nuevoToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// TenancyService gestiona la jerarquía Organización→Empresa→Sede, los usuarios
// y membresías, y la autenticación local (sin Hubmy).
type TenancyService struct {
	orgs    organizacion.Repository
	emps    empresa.Repository
	sedes   sede.Repository
	users   usuario.UsuarioRepo
	members usuario.MembresiaRepo
	creds   credencial.Repository
	audit   auditoria.Repository
}

// NewTenancy construye el servicio (orden de puertos = cmd/api).
func NewTenancy(
	orgs organizacion.Repository,
	emps empresa.Repository,
	sedes sede.Repository,
	users usuario.UsuarioRepo,
	members usuario.MembresiaRepo,
	creds credencial.Repository,
	audit auditoria.Repository,
) *TenancyService {
	return &TenancyService{orgs: orgs, emps: emps, sedes: sedes, users: users, members: members, creds: creds, audit: audit}
}

// --- Vistas ---

type SedeView = sede.Sede

// EmpresaView es una empresa con el rol del usuario y sus sedes.
type EmpresaView struct {
	ID      string      `json:"id"`
	Nombre  string      `json:"nombre"`
	RIF     string      `json:"rif"`
	Rol     string      `json:"rol"`
	Sandbox bool        `json:"sandbox"` // empresa de prueba (QA): se rotula en el selector
	Sedes   []sede.Sede `json:"sedes"`
}

// OrgView es una organización con sus empresas accesibles.
type OrgView struct {
	ID       string        `json:"id"`
	Nombre   string        `json:"nombre"`
	Empresas []EmpresaView `json:"empresas"`
}

// MeView es la respuesta de GET /api/me.
type MeView struct {
	User            authn.Principal `json:"user"`
	Organizaciones  []OrgView       `json:"organizaciones"`
	NeedsOnboarding bool            `json:"needsOnboarding"`
}

// Me arma el contexto del usuario: sus organizaciones→empresas→sedes y su rol.
func (t *TenancyService) Me(p authn.Principal) MeView {
	mems := t.members.ByUsuario(p.UserID)
	byOrg := map[string]*OrgView{}
	order := []string{}
	for _, m := range mems {
		if m.Estado != usuario.EstadoActiva {
			continue
		}
		emp, ok := t.emps.ByID(m.EmpresaID)
		if !ok || !emp.Activa {
			continue // una empresa desactivada (o un sandbox eliminado) no es operable: no va al selector
		}
		org, ok := t.orgs.ByID(emp.OrganizacionID)
		if !ok {
			continue
		}
		ev := EmpresaView{ID: emp.ID, Nombre: emp.Nombre, RIF: emp.RIF, Rol: m.Rol, Sandbox: emp.Sandbox, Sedes: t.sedes.List(emp.ID)}
		if o, seen := byOrg[org.ID]; seen {
			o.Empresas = append(o.Empresas, ev)
		} else {
			byOrg[org.ID] = &OrgView{ID: org.ID, Nombre: org.Nombre, Empresas: []EmpresaView{ev}}
			order = append(order, org.ID)
		}
	}
	orgs := make([]OrgView, 0, len(order))
	for _, id := range order {
		orgs = append(orgs, *byOrg[id])
	}
	return MeView{User: p, Organizaciones: orgs, NeedsOnboarding: len(orgs) == 0}
}

// RolEnEmpresa valida la membresía de un usuario en una empresa y devuelve
// (empresa, rol, sedeFija, ok). Es la base del aislamiento de tenant en el
// middleware de contexto.
func (t *TenancyService) RolEnEmpresa(usuarioID, empresaID string) (empresa.Empresa, string, string, bool) {
	for _, m := range t.members.ByUsuario(usuarioID) {
		if m.EmpresaID == empresaID && m.Estado == usuario.EstadoActiva {
			emp, ok := t.emps.ByID(empresaID)
			if !ok || !emp.Activa {
				return empresa.Empresa{}, "", "", false
			}
			return emp, m.Rol, m.SedeID, true
		}
	}
	return empresa.Empresa{}, "", "", false
}

// SedeValida indica si la sede pertenece a la empresa.
func (t *TenancyService) SedeValida(empresaID, sedeID string) bool {
	if sedeID == "" {
		return true
	}
	_, ok := t.sedes.ByID(empresaID, sedeID)
	return ok
}

// Empresa expone una empresa por id (para el contexto y bootstrap).
func (t *TenancyService) Empresa(id string) (empresa.Empresa, bool) { return t.emps.ByID(id) }

// Sedes lista las sedes de una empresa.
func (t *TenancyService) Sedes(empresaID string) []sede.Sede { return t.sedes.List(empresaID) }

// --- Onboarding / alta ---

// CrearEmpresa da de alta una empresa. Si el usuario no tiene organización, se
// crea una automáticamente y se le asigna la membresía de Dueña.
func (t *TenancyService) CrearEmpresa(p authn.Principal, origen, nombre, rif string) (empresa.Empresa, error) {
	// El RIF, si viene, se fija aquí y debe validar (formato + DV para J/G). Un
	// RIF vacío en el alta se completa luego en la configuración del tenant.
	if strings.TrimSpace(rif) != "" {
		if err := fiscal.ValidarDocumentoStr(strings.TrimSpace(rif)); err != nil {
			return empresa.Empresa{}, err
		}
	}
	orgID := t.primeraOrg(p.UserID)
	if orgID == "" {
		org := t.orgs.Create(organizacion.Organizacion{Nombre: nombre, Estado: "activa", Plan: "base", Creada: ahora()})
		orgID = org.ID
	}
	emp := t.emps.Create(empresa.Empresa{
		OrganizacionID: orgID, Nombre: nombre, RIF: rif, Activa: true, Creada: ahora(),
	})
	t.asegurarUsuario(p)
	t.members.Create(usuario.Membresia{
		UsuarioID: p.UserID, Email: p.Email, Nombre: p.Nombre, EmpresaID: emp.ID,
		Rol: usuario.RolDueno, Estado: usuario.EstadoActiva,
	})
	t.audit.Append(evento(emp.ID, p.UserID, origen, "tenancy.empresa.crear", emp.ID, emp.Nombre))
	return emp, nil
}

// Onboarding fija el giro y la modalidad de facturación de una empresa.
func (t *TenancyService) Onboarding(empresaID, actor, origen, giro, modalidad string, moneda ConfigMoneda) (empresa.Empresa, error) {
	emp, ok := t.emps.ByID(empresaID)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	emp.Giro = giro
	emp.Modalidad = modalidad
	// Configuración de moneda del paso «Monedas y tu primer almacén» (R9 + R10).
	// Con valores por defecto seguros: bolívar y tasa oficial del BCV, que es lo
	// que el prototipo trae seleccionado.
	emp.MonedaPrincipal = empresa.MonedaVES
	if empresa.MonedaValida(moneda.MonedaPrincipal) {
		emp.MonedaPrincipal = moneda.MonedaPrincipal
	}
	emp.FuenteTasa = empresa.FuenteTasaBCV
	if empresa.FuenteTasaValida(moneda.FuenteTasa) {
		emp.FuenteTasa = moneda.FuenteTasa
	}
	// Si la empresa piensa en dólares, capturar precios en dólares va implícito.
	emp.PreciosEnUsd = moneda.PreciosEnUsd || emp.MonedaPrincipal == empresa.MonedaUSD
	emp.OnboardingOK = true
	out, _ := t.emps.Update(emp)
	t.audit.Append(evento(empresaID, actor, origen, "tenancy.empresa.onboarding", empresaID, giro+"/"+modalidad))
	return out, nil
}

// CrearSede añade una sede a una empresa.
func (t *TenancyService) CrearSede(empresaID, actor, origen, nombre, direccion string) (sede.Sede, error) {
	if _, ok := t.emps.ByID(empresaID); !ok {
		return sede.Sede{}, ErrEmpresaNoExiste
	}
	if lim, _, sucursales, ok := t.limiteDeEmpresa(empresaID); ok && lim.Sucursales > 0 && sucursales >= lim.Sucursales {
		return sede.Sede{}, ErrLimiteSucursales
	}
	sd := t.sedes.Create(sede.Sede{EmpresaID: empresaID, Nombre: nombre, Direccion: direccion, Activa: true})
	t.audit.Append(evento(empresaID, actor, origen, "tenancy.sede.crear", sd.ID, sd.Nombre))
	return sd, nil
}

// --- Datos de empresa y gestión de sedes (Configuración) ---

// ErrRIFRequerido se devuelve cuando se intenta guardar la empresa sin RIF.
var ErrRIFRequerido = errors.New("el RIF es obligatorio")

// ErrSedeNoExiste se devuelve cuando la sede no existe o no pertenece a la empresa.
var ErrSedeNoExiste = errors.New("la sede no existe en esta empresa")

// ErrUltimaSedeActiva impide desactivar la única sede activa: una empresa
// necesita al menos una para poder operar (abrir caja, tener existencias).
var ErrUltimaSedeActiva = errors.New("no se puede desactivar la última sede activa: la empresa necesita al menos una")

// EmpresaEdit son los campos editables de la cabecera fiscal de una empresa.
// No incluye OrganizacionID (el tenant no cambia de organización desde aquí) ni
// el estado de onboarding/moneda (esos tienen su propio flujo).
type EmpresaEdit struct {
	Nombre      string
	RIF         string
	RazonSocial string
	Direccion   string
	Telefono    string
	Email       string
	Logo        string
	// ColorMarca (COMPAT) y LogoAlterno (COMPAT) son campos previos del branding de
	// dos niveles; el modelo nuevo los reemplaza por TemaPantalla.ColorEnfasis y
	// LogoBlanco/LogoNegro. Se conservan para no romper datos ya guardados.
	ColorMarca  string
	LogoAlterno string
	// LogoBlanco y LogoNegro son versiones del logo para contraste (URL/data URI):
	// la blanca para fondos oscuros, la negra para claros. El logo de color (Logo)
	// sigue en Datos de empresa (también va en comprobantes).
	LogoBlanco string
	LogoNegro  string
	// TemaPantalla es la apariencia de la pantalla del cliente (fondos por espacio,
	// texto, énfasis, versión de logo). Se normaliza (hex válidos / versión acotada).
	TemaPantalla empresa.TemaPantallaCliente
	// ExigirCedulaCliente obliga a identificar al cliente antes de cobrar en el POS.
	ExigirCedulaCliente bool
	// BannerSuperior y BannerLateral son imágenes publicitarias del comercio para
	// la pantalla auxiliar del cliente en caja. Mismo tipo que Logo (URL/data URI).
	BannerSuperior string
	BannerLateral  string
	// PantallaClienteModo elige qué muestra la pantalla del cliente del POS
	// (solo_productos | mixta | publicidad). Se normaliza a un modo válido.
	PantallaClienteModo string
	// Publicidad es la lista de slides (texto o imagen) del carrusel de la pantalla
	// del cliente. Ver empresa.AnuncioSlide.
	Publicidad []empresa.AnuncioSlide
}

// normalizarPublicidad limpia la lista de slides: recorta espacios y descarta los
// vacíos según el tipo (texto sin Texto, imagen sin Imagen), fijando un tipo
// coherente. Conserva el orden. Nunca devuelve nil sobre entrada no nil (para que
// el JSON serialice `[]` y no `null`).
func normalizarPublicidad(in []empresa.AnuncioSlide) []empresa.AnuncioSlide {
	out := make([]empresa.AnuncioSlide, 0, len(in))
	for _, s := range in {
		texto := strings.TrimSpace(s.Texto)
		subtexto := strings.TrimSpace(s.Subtexto)
		imagen := strings.TrimSpace(s.Imagen)
		switch strings.TrimSpace(s.Tipo) {
		case empresa.SlideImagen:
			if imagen != "" {
				out = append(out, empresa.AnuncioSlide{Tipo: empresa.SlideImagen, Imagen: imagen})
			}
		case empresa.SlideTexto:
			if texto != "" {
				out = append(out, empresa.AnuncioSlide{Tipo: empresa.SlideTexto, Texto: texto, Subtexto: subtexto})
			}
		default:
			// Tipo vacío/desconocido (p. ej. de datos viejos): se infiere por el
			// contenido presente; si no hay ninguno, se descarta.
			if imagen != "" {
				out = append(out, empresa.AnuncioSlide{Tipo: empresa.SlideImagen, Imagen: imagen})
			} else if texto != "" {
				out = append(out, empresa.AnuncioSlide{Tipo: empresa.SlideTexto, Texto: texto, Subtexto: subtexto})
			}
		}
	}
	return out
}

// ActualizarEmpresa edita los datos fiscales de cabecera del tenant. Valida que
// el RIF no quede vacío y nunca toca OrganizacionID. Audita el cambio.
func (t *TenancyService) ActualizarEmpresa(empresaID, actor, origen string, in EmpresaEdit) (empresa.Empresa, error) {
	emp, ok := t.emps.ByID(empresaID)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	if strings.TrimSpace(in.RIF) == "" {
		return empresa.Empresa{}, ErrRIFRequerido
	}
	// El RIF de la empresa viene CON prefijo de tipo ("J-…"): se valida su formato
	// y, para J/G, el dígito verificador módulo 11 (SENIAT).
	if err := fiscal.ValidarDocumentoStr(strings.TrimSpace(in.RIF)); err != nil {
		return empresa.Empresa{}, err
	}
	emp.Nombre = strings.TrimSpace(in.Nombre)
	emp.RIF = strings.TrimSpace(in.RIF)
	emp.RazonSocial = strings.TrimSpace(in.RazonSocial)
	emp.Direccion = strings.TrimSpace(in.Direccion)
	emp.Telefono = strings.TrimSpace(in.Telefono)
	emp.Email = strings.TrimSpace(in.Email)
	emp.Logo = strings.TrimSpace(in.Logo)
	emp.ColorMarca = empresa.NormalizarColorHex(in.ColorMarca)
	emp.LogoAlterno = strings.TrimSpace(in.LogoAlterno)
	emp.LogoBlanco = strings.TrimSpace(in.LogoBlanco)
	emp.LogoNegro = strings.TrimSpace(in.LogoNegro)
	emp.TemaPantalla = in.TemaPantalla.Normalizar()
	emp.ExigirCedulaCliente = in.ExigirCedulaCliente
	emp.BannerSuperior = strings.TrimSpace(in.BannerSuperior)
	emp.BannerLateral = strings.TrimSpace(in.BannerLateral)
	emp.PantallaClienteModo = empresa.NormalizarPantallaClienteModo(in.PantallaClienteModo)
	emp.Publicidad = normalizarPublicidad(in.Publicidad)
	out, ok := t.emps.Update(emp)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	t.audit.Append(evento(empresaID, actor, origen, "config.empresa.actualizar", empresaID, out.Nombre))
	return out, nil
}

// ErrAlicuotaInvalida se devuelve cuando una tasa cae fuera del rango [0,1].
var ErrAlicuotaInvalida = errors.New("las alícuotas se expresan como fracción entre 0 y 1 (p. ej. 0.16 = 16%); 0 usa el valor del sistema")

// ActualizarImpuestos configura las alícuotas de IVA e IGTF de la empresa
// (fracciones: 0.16 = 16%). Materializa «compliance as configuration»: una nueva
// providencia se aplica sin recompilar. Valida el rango [0,1] (0 = usar el default
// del sistema; negativos o > 1 se rechazan). No toca documentos ya emitidos: cada
// uno conserva la tasa que grabó al emitir (ADR tasa histórica), así que el cambio
// solo rige para documentos NUEVOS. Audita el cambio.
func (t *TenancyService) ActualizarImpuestos(empresaID, actor, origen string, iva, igtf float64, agenteIVA, agenteISLR bool, retIVAPct float64) (empresa.Empresa, error) {
	emp, ok := t.emps.ByID(empresaID)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	if iva < 0 || iva > 1 || igtf < 0 || igtf > 1 {
		return empresa.Empresa{}, ErrAlicuotaInvalida
	}
	if retIVAPct < 0 || retIVAPct > 100 {
		return empresa.Empresa{}, ErrAlicuotaInvalida
	}
	emp.AlicuotaIVA = iva
	emp.AlicuotaIGTF = igtf
	emp.AgenteRetencionIVA = agenteIVA
	emp.AgenteRetencionISLR = agenteISLR
	emp.RetencionIVAPorcentaje = retIVAPct
	out, ok := t.emps.Update(emp)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	t.audit.Append(evento(empresaID, actor, origen, "config.impuestos.actualizar", empresaID,
		fmt.Sprintf("IVA %.4f / IGTF %.4f / agIVA %t / agISLR %t / retIVA%% %.0f", iva, igtf, agenteIVA, agenteISLR, retIVAPct)))
	return out, nil
}

// ActualizarSede cambia el nombre y la dirección de una sede del tenant.
func (t *TenancyService) ActualizarSede(empresaID, sedeID, actor, origen, nombre, direccion string) (sede.Sede, error) {
	sd, ok := t.sedes.ByID(empresaID, sedeID)
	if !ok {
		return sede.Sede{}, ErrSedeNoExiste
	}
	if strings.TrimSpace(nombre) == "" {
		return sede.Sede{}, errors.New("el nombre de la sede es obligatorio")
	}
	sd.Nombre = strings.TrimSpace(nombre)
	sd.Direccion = strings.TrimSpace(direccion)
	out, ok := t.sedes.Update(sd)
	if !ok {
		return sede.Sede{}, ErrSedeNoExiste
	}
	t.audit.Append(evento(empresaID, actor, origen, "config.sede.actualizar", out.ID, out.Nombre))
	return out, nil
}

// DesactivarSede da de baja (reversible) una sede. Rechaza desactivar la última
// sede activa de la empresa.
func (t *TenancyService) DesactivarSede(empresaID, sedeID, actor, origen string) error {
	sd, ok := t.sedes.ByID(empresaID, sedeID)
	if !ok {
		return ErrSedeNoExiste
	}
	if !sd.Activa {
		return nil // ya está inactiva: idempotente.
	}
	activas := 0
	for _, s := range t.sedes.List(empresaID) {
		if s.Activa {
			activas++
		}
	}
	if activas <= 1 {
		return ErrUltimaSedeActiva
	}
	sd.Activa = false
	if _, ok := t.sedes.Update(sd); !ok {
		return ErrSedeNoExiste
	}
	t.audit.Append(evento(empresaID, actor, origen, "config.sede.desactivar", sd.ID, sd.Nombre))
	return nil
}

// ReactivarSede vuelve a poner una sede como activa.
func (t *TenancyService) ReactivarSede(empresaID, sedeID, actor, origen string) error {
	sd, ok := t.sedes.ByID(empresaID, sedeID)
	if !ok {
		return ErrSedeNoExiste
	}
	if sd.Activa {
		return nil // ya está activa: idempotente.
	}
	sd.Activa = true
	if _, ok := t.sedes.Update(sd); !ok {
		return ErrSedeNoExiste
	}
	t.audit.Append(evento(empresaID, actor, origen, "config.sede.reactivar", sd.ID, sd.Nombre))
	return nil
}

// --- Autenticación local (bcrypt) ---

// LoginNativo valida email+contraseña. Corre un compare ficticio si el usuario
// no existe para no filtrar por tiempo la existencia de la cuenta.
func (t *TenancyService) LoginNativo(email, password string) (authn.Principal, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	c, ok := t.creds.ByEmail(email)
	if !ok {
		bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinv"), []byte(password))
		return authn.Principal{}, ErrCredencialInvalida
	}
	if bcrypt.CompareHashAndPassword([]byte(c.Hash), []byte(password)) != nil {
		return authn.Principal{}, ErrCredencialInvalida
	}
	return authn.Principal{UserID: c.UsuarioID, Nombre: c.Nombre, Email: c.Email}, nil
}

// PreviewInvitacion devuelve datos de una invitación para la pantalla de aceptar.
func (t *TenancyService) PreviewInvitacion(token string) (map[string]string, error) {
	m, ok := t.members.ByToken(token)
	if !ok || m.Estado != usuario.EstadoInvitada {
		return nil, ErrInvitacionInvalida
	}
	emp, _ := t.emps.ByID(m.EmpresaID)
	return map[string]string{"email": m.Email, "empresa": emp.Nombre, "rol": m.Rol}, nil
}

// AceptarInvitacion crea la credencial local y activa las membresías del email.
func (t *TenancyService) AceptarInvitacion(token, nombre, password, origen string) (authn.Principal, error) {
	m, ok := t.members.ByToken(token)
	if !ok || m.Estado != usuario.EstadoInvitada {
		return authn.Principal{}, ErrInvitacionInvalida
	}
	if len(password) < 8 {
		return authn.Principal{}, ErrPasswordDebil
	}
	email := strings.ToLower(m.Email)
	if _, exists := t.creds.ByEmail(email); exists {
		return authn.Principal{}, ErrEmailEnUso
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return authn.Principal{}, err
	}
	u := t.users.Create(usuario.Usuario{Nombre: nombre, Email: email})
	t.creds.Create(credencial.Credencial{Email: email, Hash: string(hash), UsuarioID: u.ID, Nombre: nombre})
	t.ClaimInvitations(email, u.ID, nombre)
	t.audit.Append(evento(m.EmpresaID, u.ID, origen, "tenancy.invitacion.aceptar", m.ID, email))
	return authn.Principal{UserID: u.ID, Nombre: nombre, Email: email}, nil
}

// ClaimInvitations vincula las invitaciones pendientes de un email a un usuario
// ya identificado (tras login por Hubmy o alta local) y las activa.
func (t *TenancyService) ClaimInvitations(email, usuarioID, nombre string) {
	email = strings.ToLower(email)
	for _, m := range t.members.ByEmail(email) {
		if m.Estado == usuario.EstadoInvitada {
			m.UsuarioID = usuarioID
			m.Nombre = nombre
			m.Estado = usuario.EstadoActiva
			t.members.Update(m)
		}
	}
}

// asegurarUsuario crea el registro de usuario si aún no existe (p. ej. login por Hubmy).
func (t *TenancyService) asegurarUsuario(p authn.Principal) {
	if _, ok := t.users.ByID(p.UserID); ok {
		return
	}
	t.users.Create(usuario.Usuario{ID: p.UserID, Nombre: p.Nombre, Email: strings.ToLower(p.Email)})
}

func (t *TenancyService) primeraOrg(usuarioID string) string {
	for _, m := range t.members.ByUsuario(usuarioID) {
		if emp, ok := t.emps.ByID(m.EmpresaID); ok {
			return emp.OrganizacionID
		}
	}
	return ""
}

// SedesDeEmpresa devuelve las sedes del tenant. Lo usa el adaptador HTTP para
// alimentar consultas que necesitan recorrer toda la red de tiendas.
func (t *TenancyService) SedesDeEmpresa(empresaID string) []sede.Sede {
	return t.sedes.List(empresaID)
}

// TodasLasEmpresas devuelve todas las empresas de todas las organizaciones. La
// usa el arranque para tareas de mantenimiento (p. ej. recontabilizar documentos
// anteriores al libro diario); no la expone ninguna ruta HTTP.
func (t *TenancyService) TodasLasEmpresas() []empresa.Empresa {
	out := []empresa.Empresa{}
	for _, o := range t.orgs.List() {
		out = append(out, t.emps.List(o.ID)...)
	}
	return out
}

// --- Vista de PLATAFORMA (super-admin / Mornix) ---
//
// Estas vistas y mutaciones cruzan a propósito el aislamiento por tenant: son la
// base del contrato interno /internal que consume la consola de plataforma (F0
// del plan de la consola). NUNCA se exponen a una cuenta cliente; el adaptador
// HTTP las cuelga bajo /internal, detrás de la clave M2M (requirePlatform), sin
// pasar por empresaContext.

// ErrOrgNoExiste / ErrEstadoOrgInvalido son errores del contrato de plataforma.
var (
	ErrOrgNoExiste       = errors.New("la organización no existe")
	ErrEstadoOrgInvalido = errors.New("estado de organización inválido (activa|suspendida)")
	// Límites de CAPACIDAD del plan (corte duro). NO se aplican a la emisión de
	// facturas: facturar es una obligación legal y nunca se bloquea por plan (SAD);
	// el impago se gestiona suspendiendo la organización, no cortando una factura.
	ErrLimiteUsuarios   = errors.New("se alcanzó el límite de usuarios del plan")
	ErrLimiteSucursales = errors.New("se alcanzó el límite de sucursales del plan")
)

// limiteDeEmpresa resuelve los límites del plan y el uso actual (usuarios,
// sucursales) de la organización de una empresa. Un límite 0 = ilimitado. Los
// sandboxes no cuentan (ConteosDeOrg los excluye).
func (t *TenancyService) limiteDeEmpresa(empresaID string) (lim organizacion.Limites, usuarios, sucursales int, ok bool) {
	o, existe := t.OrgDeEmpresa(empresaID)
	if !existe {
		return organizacion.Limites{}, 0, 0, false
	}
	usuarios, sucursales, _, _ = t.ConteosDeOrg(o.ID)
	return o.Limites, usuarios, sucursales, true
}

// PlatformSedeView, PlatformEmpresaView y PlatformOrgView son la foto de un
// tenant para la consola de plataforma (org → empresas → sedes + conteos).
type PlatformSedeView struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Activa bool   `json:"activa"`
}

type PlatformEmpresaView struct {
	ID        string             `json:"id"`
	Nombre    string             `json:"nombre"`
	RIF       string             `json:"rif"`
	Giro      string             `json:"giro"`
	Modalidad string             `json:"modalidad"`
	Activa    bool               `json:"activa"`
	Usuarios  int                `json:"usuarios"`
	Sedes     []PlatformSedeView `json:"sedes"`
	// Sandbox marca las empresas de QA (clon de una empresa origen, TTL). La consola
	// las distingue para ofrecer la acción correcta (archivar) y no confundirlas con
	// tenants reales. ExpiraEl es su fecha de caducidad (si aplica).
	Sandbox  bool   `json:"sandbox"`
	ExpiraEl string `json:"expiraEl,omitempty"`
}

type PlatformOrgView struct {
	ID       string                `json:"id"`
	Nombre   string                `json:"nombre"`
	Estado   string                `json:"estado"`
	Plan     string                `json:"plan"`
	Limites  organizacion.Limites  `json:"limites"`
	Creada   string                `json:"creada"`
	Empresas []PlatformEmpresaView `json:"empresas"`
}

func (t *TenancyService) empresaPlataforma(e empresa.Empresa) PlatformEmpresaView {
	ev := PlatformEmpresaView{
		ID: e.ID, Nombre: e.Nombre, RIF: e.RIF, Giro: e.Giro, Modalidad: e.Modalidad,
		Activa: e.Activa, Usuarios: len(t.members.ByEmpresa(e.ID)), Sedes: []PlatformSedeView{},
		Sandbox: e.Sandbox, ExpiraEl: e.ExpiraEl,
	}
	for _, s := range t.sedes.List(e.ID) {
		ev.Sedes = append(ev.Sedes, PlatformSedeView{ID: s.ID, Nombre: s.Nombre, Activa: s.Activa})
	}
	return ev
}

// TenantsPlataforma lista TODAS las organizaciones con sus empresas y sedes.
func (t *TenancyService) TenantsPlataforma() []PlatformOrgView {
	orgs := t.orgs.List()
	out := make([]PlatformOrgView, 0, len(orgs))
	for _, o := range orgs {
		ov := PlatformOrgView{ID: o.ID, Nombre: o.Nombre, Estado: o.Estado, Plan: o.Plan, Limites: o.Limites, Creada: o.Creada, Empresas: []PlatformEmpresaView{}}
		for _, e := range t.emps.List(o.ID) {
			ov.Empresas = append(ov.Empresas, t.empresaPlataforma(e))
		}
		out = append(out, ov)
	}
	return out
}

// TenantPlataforma devuelve el detalle de un tenant (empresa + su organización).
func (t *TenancyService) TenantPlataforma(empresaID string) (PlatformOrgView, bool) {
	emp, ok := t.emps.ByID(empresaID)
	if !ok {
		return PlatformOrgView{}, false
	}
	o, ok := t.orgs.ByID(emp.OrganizacionID)
	if !ok {
		return PlatformOrgView{}, false
	}
	return PlatformOrgView{
		ID: o.ID, Nombre: o.Nombre, Estado: o.Estado, Plan: o.Plan, Limites: o.Limites, Creada: o.Creada,
		Empresas: []PlatformEmpresaView{t.empresaPlataforma(emp)},
	}, true
}

// Org devuelve una organización por id (para la consola de plataforma).
func (t *TenancyService) Org(orgID string) (organizacion.Organizacion, bool) {
	return t.orgs.ByID(orgID)
}

// OrgDeEmpresa devuelve la organización a la que pertenece una empresa.
func (t *TenancyService) OrgDeEmpresa(empresaID string) (organizacion.Organizacion, bool) {
	emp, ok := t.emps.ByID(empresaID)
	if !ok {
		return organizacion.Organizacion{}, false
	}
	return t.orgs.ByID(emp.OrganizacionID)
}

// OrgActivaPorID indica si una organización (por id) NO está suspendida. Lo usa
// empresaContext con el OrganizacionID que ya trae la empresa resuelta, para que
// el chequeo cueste una sola consulta extra (la org), no volver a leer la empresa.
func (t *TenancyService) OrgActivaPorID(orgID string) bool {
	o, ok := t.orgs.ByID(orgID)
	return ok && o.Estado != "suspendida"
}

// OrgActivaDeEmpresa es la variante por empresa (resuelve la org primero). La usan
// las pruebas y llamadores que solo tienen el id de empresa.
func (t *TenancyService) OrgActivaDeEmpresa(empresaID string) bool {
	o, ok := t.OrgDeEmpresa(empresaID)
	return ok && o.Estado != "suspendida"
}

// FijarEstadoOrg activa o suspende una organización (acción de plataforma).
func (t *TenancyService) FijarEstadoOrg(orgID, estado, actor, origen string) (organizacion.Organizacion, error) {
	if estado != "activa" && estado != "suspendida" {
		return organizacion.Organizacion{}, ErrEstadoOrgInvalido
	}
	o, ok := t.orgs.ByID(orgID)
	if !ok {
		return organizacion.Organizacion{}, ErrOrgNoExiste
	}
	o.Estado = estado
	updated, ok := t.orgs.Update(o)
	if !ok {
		return organizacion.Organizacion{}, ErrOrgNoExiste
	}
	t.audit.Append(evento(orgID, actor, origen, "plataforma.org.estado", orgID, estado))
	return updated, nil
}

// FijarPlan asigna el plan comercial y sus límites de capacidad a una
// organización (acción de plataforma). Los límites en 0 quedan ilimitados.
func (t *TenancyService) FijarPlan(orgID, plan string, limites organizacion.Limites, actor, origen string) (organizacion.Organizacion, error) {
	o, ok := t.orgs.ByID(orgID)
	if !ok {
		return organizacion.Organizacion{}, ErrOrgNoExiste
	}
	o.Plan = plan
	o.Limites = limites
	updated, ok := t.orgs.Update(o)
	if !ok {
		return organizacion.Organizacion{}, ErrOrgNoExiste
	}
	t.audit.Append(evento(orgID, actor, origen, "plataforma.org.plan", orgID, plan))
	return updated, nil
}

// ConteosDeOrg cuenta el uso de capacidad de una organización: usuarios
// DISTINTOS y sucursales (sedes) a lo largo de todas sus empresas, y devuelve
// además los ids de empresa (para que el llamador cuente las facturas del mes).
func (t *TenancyService) ConteosDeOrg(orgID string) (usuarios, sucursales int, empresaIDs []string, ok bool) {
	if _, existe := t.orgs.ByID(orgID); !existe {
		return 0, 0, nil, false
	}
	usuariosSet := map[string]bool{}
	for _, e := range t.emps.List(orgID) {
		if e.Sandbox {
			continue // las empresas de prueba (QA) no cuentan para límites ni facturación
		}
		empresaIDs = append(empresaIDs, e.ID)
		sucursales += len(t.sedes.List(e.ID))
		for _, m := range t.members.ByEmpresa(e.ID) {
			if m.Estado == usuario.EstadoActiva {
				usuariosSet[m.UsuarioID] = true
			}
		}
	}
	return len(usuariosSet), sucursales, empresaIDs, true
}

// ErrSandboxDeSandbox / ErrNoEsSandbox acotan las operaciones de sandbox.
var (
	ErrSandboxDeSandbox = errors.New("no se puede crear un sandbox a partir de otro sandbox")
	ErrNoEsSandbox      = errors.New("la empresa no es un sandbox")
)

// CrearSandboxBase crea la empresa SANDBOX (QA) de una empresa origen, en su
// misma organización, clonando su CONFIGURACIÓN, sus SEDES y sus MEMBRESÍAS (para
// que los mismos usuarios del cliente la vean en el selector). Los LEDGERS quedan
// vacíos; los MAESTROS de negocio los clona Service.ClonarMaestros aparte, con el
// mapa sede-vieja→sede-nueva que devuelve este método (para remapear referencias).
func (t *TenancyService) CrearSandboxBase(origenID string, ttlDias int, actor, origen string) (empresa.Empresa, map[string]string, error) {
	src, ok := t.emps.ByID(origenID)
	if !ok {
		return empresa.Empresa{}, nil, ErrEmpresaNoExiste
	}
	if src.Sandbox {
		return empresa.Empresa{}, nil, ErrSandboxDeSandbox
	}
	if ttlDias <= 0 {
		ttlDias = 30
	}
	nueva := src // copia todos los campos de configuración de cabecera/branding
	nueva.ID = ""
	nueva.Nombre = src.Nombre + " (Sandbox)"
	nueva.Sandbox = true
	nueva.OrigenSandboxID = origenID
	nueva.ExpiraEl = enDias(ttlDias)
	nueva.Activa = true
	nueva.Creada = ahora()
	sb := t.emps.Create(nueva)

	sedeMap := map[string]string{}
	for _, s := range t.sedes.List(origenID) {
		ns := t.sedes.Create(sede.Sede{EmpresaID: sb.ID, Nombre: s.Nombre, Direccion: s.Direccion, Activa: s.Activa})
		sedeMap[s.ID] = ns.ID
	}
	for _, m := range t.members.ByEmpresa(origenID) {
		if m.Estado != usuario.EstadoActiva {
			continue
		}
		t.members.Create(usuario.Membresia{
			UsuarioID: m.UsuarioID, Email: m.Email, Nombre: m.Nombre, EmpresaID: sb.ID,
			Rol: m.Rol, SedeID: sedeMap[m.SedeID], Estado: usuario.EstadoActiva,
		})
	}
	t.audit.Append(evento(sb.ID, actor, origen, "plataforma.sandbox.crear", sb.ID, origenID))
	return sb, sedeMap, nil
}

// CrearEmpresaRestaurada crea una empresa NUEVA (id fresco) en la organización destino a
// partir de un respaldo: copia la configuración de cabecera, recrea las SEDES conservando
// sus IDs originales (para que las referencias del ledger sigan válidas) y las MEMBRESÍAS
// activas (para que los usuarios recuperen acceso). Los DATOS de negocio los importa
// Service.RestaurarDatos aparte. Nunca sobrescribe un tenant vivo: siempre es una empresa
// nueva. Marca la empresa como restaurada en el nombre.
func (t *TenancyService) CrearEmpresaRestaurada(orgID string, src empresa.Empresa, sedes []sede.Sede, miembros []MiembroView, actor, origen string) (empresa.Empresa, error) {
	if _, ok := t.orgs.ByID(orgID); !ok {
		return empresa.Empresa{}, ErrOrgNoExiste
	}
	nueva := src
	nueva.ID = ""
	nueva.OrganizacionID = orgID
	nueva.Nombre = src.Nombre + " (restaurada " + ahora()[:10] + ")"
	nueva.Sandbox = false
	nueva.OrigenSandboxID = ""
	nueva.ExpiraEl = ""
	nueva.Activa = true
	nueva.Creada = ahora()
	emp := t.emps.Create(nueva)

	for _, s := range sedes {
		// Conserva el ID original de la sede (referencias del ledger dependen de él).
		t.sedes.Create(sede.Sede{ID: s.ID, EmpresaID: emp.ID, Nombre: s.Nombre, Direccion: s.Direccion, Activa: s.Activa})
	}
	for _, m := range miembros {
		if m.Estado != usuario.EstadoActiva {
			continue
		}
		t.members.Create(usuario.Membresia{
			UsuarioID: m.UsuarioID, Email: m.Email, Nombre: m.Nombre, EmpresaID: emp.ID,
			Rol: m.Rol, SedeID: m.SedeID, Estado: usuario.EstadoActiva,
		})
	}
	t.audit.Append(evento(emp.ID, actor, origen, "plataforma.restore", emp.ID, src.ID))
	return emp, nil
}

// EliminarSandbox archiva (soft-delete) una empresa sandbox: la desactiva para que
// desaparezca del selector del cliente. Rechaza empresas que no sean sandbox, para
// no borrar jamás un tenant real.
func (t *TenancyService) EliminarSandbox(empresaID, actor, origen string) error {
	emp, ok := t.emps.ByID(empresaID)
	if !ok {
		return ErrEmpresaNoExiste
	}
	if !emp.Sandbox {
		return ErrNoEsSandbox
	}
	emp.Activa = false
	if _, ok := t.emps.Update(emp); !ok {
		return ErrEmpresaNoExiste
	}
	t.audit.Append(evento(empresaID, actor, origen, "plataforma.sandbox.eliminar", empresaID, ""))
	return nil
}

// FijarEmpresaActiva activa o desactiva una empresa (acción de plataforma).
func (t *TenancyService) FijarEmpresaActiva(empresaID string, activa bool, actor, origen string) (empresa.Empresa, error) {
	emp, ok := t.emps.ByID(empresaID)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	emp.Activa = activa
	updated, ok := t.emps.Update(emp)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	estado := "desactivada"
	if activa {
		estado = "activada"
	}
	t.audit.Append(evento(empresaID, actor, origen, "plataforma.empresa.activa", empresaID, estado))
	return updated, nil
}

// --- Miembros de la empresa (Configuración › Usuarios y roles) ---

// ErrSedeRequerida se devuelve cuando un rol de una sola sede se guarda sin sede.
var ErrSedeRequerida = errors.New("los roles de mostrador (cajero y vendedor) trabajan en una sola sede: hay que asignársela")

// RolesDeUnaSolaSede son los roles que operan en UNA sede y no pueden elegir otra.
// El cajero es el caso duro: su turno, su caja y sus documentos son de esa sede,
// y el servidor le fija la sede en cada petición (ver empresaContext).
func RolesDeUnaSolaSede() []string {
	return []string{usuario.RolCajero, usuario.RolVendedor, usuario.RolMesonero}
}

func rolDeUnaSolaSede(rol string) bool {
	for _, r := range RolesDeUnaSolaSede() {
		if r == rol {
			return true
		}
	}
	return false
}

// MiembroView es un usuario de la empresa con su rol y su sede, para la vista de
// Usuarios y roles.
type MiembroView struct {
	ID        string `json:"id"`
	UsuarioID string `json:"usuarioId"`
	Nombre    string `json:"nombre"`
	Email     string `json:"email"`
	Rol       string `json:"rol"`
	SedeID    string `json:"sedeId"`
	// SedeNombre resuelto, o «Todas las sedes» cuando el rol no está atado a una.
	SedeNombre string `json:"sedeNombre"`
	Estado     string `json:"estado"`
	// SedeObligatoria le dice a la interfaz que este rol exige sede, para que el
	// formulario lo pida en vez de dejar guardar algo inválido.
	SedeObligatoria bool `json:"sedeObligatoria"`
}

// Miembros lista los usuarios de la empresa con su rol y su sede.
func (t *TenancyService) Miembros(empresaID string) []MiembroView {
	sedes := map[string]string{}
	for _, sd := range t.sedes.List(empresaID) {
		sedes[sd.ID] = sd.Nombre
	}
	out := []MiembroView{}
	for _, m := range t.members.ByEmpresa(empresaID) {
		v := MiembroView{
			ID: m.ID, UsuarioID: m.UsuarioID, Nombre: m.Nombre, Email: m.Email,
			Rol: m.Rol, SedeID: m.SedeID, Estado: m.Estado,
			SedeObligatoria: rolDeUnaSolaSede(m.Rol),
		}
		if m.SedeID != "" {
			v.SedeNombre = sedes[m.SedeID]
			if v.SedeNombre == "" {
				v.SedeNombre = "Sede eliminada"
			}
		} else if m.Rol == usuario.RolContadora {
			v.SedeNombre = "Todas (solo lectura)"
		} else {
			v.SedeNombre = "Todas las sedes"
		}
		out = append(out, v)
	}
	return out
}

// GuardarMiembro cambia el rol y la sede de un miembro de la empresa.
//
// Regla de negocio: cajero y vendedor trabajan en UNA sede. Guardar un cajero sin
// sede se rechaza — si no, entraría sin contexto y podría ver el mostrador de
// otra tienda. La sede la impone después el servidor en cada petición.
func (t *TenancyService) GuardarMiembro(empresaID, miembroID, rol, sedeID string) (usuario.Membresia, error) {
	var actual usuario.Membresia
	encontrada := false
	for _, m := range t.members.ByEmpresa(empresaID) {
		if m.ID == miembroID {
			actual, encontrada = m, true
			break
		}
	}
	if !encontrada {
		return usuario.Membresia{}, errors.New("ese usuario no pertenece a la empresa")
	}
	if !usuario.RolValido(rol) {
		return usuario.Membresia{}, errors.New("rol inválido")
	}
	if rolDeUnaSolaSede(rol) {
		if sedeID == "" {
			return usuario.Membresia{}, ErrSedeRequerida
		}
		if !t.SedeValida(empresaID, sedeID) {
			return usuario.Membresia{}, errors.New("sede inválida")
		}
	} else {
		// Un rol que ve toda la empresa no queda atado a una sede.
		sedeID = ""
	}
	actual.Rol = rol
	actual.SedeID = sedeID
	out, ok := t.members.Update(actual)
	if !ok {
		return usuario.Membresia{}, errors.New("no se pudo guardar el usuario")
	}
	return out, nil
}

// --- Invitación de miembros (Configuración › Usuarios y roles) ---

// InviteInput son los datos para invitar a alguien a la empresa.
type InviteInput struct {
	Email  string
	Nombre string
	Rol    string
	SedeID string
}

// miembroDe busca una membresía por id dentro del scope del tenant.
func (t *TenancyService) miembroDe(empresaID, miembroID string) (usuario.Membresia, bool) {
	for _, m := range t.members.ByEmpresa(empresaID) {
		if m.ID == miembroID {
			return m, true
		}
	}
	return usuario.Membresia{}, false
}

// validarInvite normaliza y valida los datos de una invitación: correo, rol y la
// sede obligatoria de los roles de mostrador. Devuelve email y sede ya limpios.
func (t *TenancyService) validarInvite(empresaID string, in InviteInput) (email, sedeID string, err error) {
	email = strings.ToLower(strings.TrimSpace(in.Email))
	if !emailRe.MatchString(email) {
		return "", "", ErrEmailInvalido
	}
	if !usuario.RolValido(in.Rol) {
		return "", "", ErrRolInvalido
	}
	sedeID = strings.TrimSpace(in.SedeID)
	if rolDeUnaSolaSede(in.Rol) {
		if sedeID == "" {
			return "", "", ErrSedeRequerida
		}
		if !t.SedeValida(empresaID, sedeID) {
			return "", "", ErrSedeInvalida
		}
	} else {
		// Un rol que ve toda la empresa no queda atado a una sede.
		sedeID = ""
	}
	return email, sedeID, nil
}

// InvitarMiembro crea (o reactiva) una membresía en estado «invitada» con un token
// único. La invitación queda lista para el flujo de aceptación existente
// (AceptarInvitacion / ClaimInvitations): al aceptar, se crea la credencial local
// o se vincula el login por Hubmy y la membresía pasa a activa.
//
// Reglas: correo válido, rol válido, sede obligatoria para cajero/vendedor, y no
// puede existir ya un miembro ACTIVO con ese correo en la empresa. Si hay una
// invitación PENDIENTE con ese correo, se reutiliza (se actualizan rol/sede/nombre
// y se regenera el token) en vez de duplicar. Audita `usuario.invitar`.
//
// NOTA (punto de integración): el envío del correo de invitación aún NO está
// cableado —no hay servicio de email real en el backend—. Mientras tanto la
// invitación se comparte con el ENLACE que devuelve este método
// (`/?invite=<token>`). Cuando se conecte el envío, usar cfg.HubmyInviteTemplateID
// con la plantilla de Hubmy (hubmy_send_email) para notificar al invitado.
func (t *TenancyService) InvitarMiembro(empresaID, actor, origen string, in InviteInput) (usuario.Membresia, error) {
	if _, ok := t.emps.ByID(empresaID); !ok {
		return usuario.Membresia{}, ErrEmpresaNoExiste
	}
	email, sedeID, err := t.validarInvite(empresaID, in)
	if err != nil {
		return usuario.Membresia{}, err
	}
	token, err := nuevoToken()
	if err != nil {
		return usuario.Membresia{}, err
	}
	nombre := strings.TrimSpace(in.Nombre)

	// ¿Ya hay una membresía con ese correo en la empresa? Activa: se rechaza.
	// Pendiente: se reutiliza (reinvitar con datos frescos y token nuevo).
	for _, m := range t.members.ByEmpresa(empresaID) {
		if strings.ToLower(m.Email) != email {
			continue
		}
		if m.Estado == usuario.EstadoActiva {
			return usuario.Membresia{}, ErrMiembroExistente
		}
		m.Nombre = nombre
		m.Rol = in.Rol
		m.SedeID = sedeID
		m.Estado = usuario.EstadoInvitada
		m.Token = token
		out, ok := t.members.Update(m)
		if !ok {
			return usuario.Membresia{}, errors.New("no se pudo guardar la invitación")
		}
		t.audit.Append(evento(empresaID, actor, origen, "usuario.invitar", out.ID, email))
		return out, nil
	}

	// Alta de un usuario NUEVO en la organización: corte duro por límite del plan.
	// Va aquí (tras la rama de reinvitación) para no bloquear re-invitar a alguien
	// que ya estaba pendiente. Cuenta usuarios activos distintos de la organización.
	if lim, usuarios, _, ok := t.limiteDeEmpresa(empresaID); ok && lim.Usuarios > 0 && usuarios >= lim.Usuarios {
		return usuario.Membresia{}, ErrLimiteUsuarios
	}
	out := t.members.Create(usuario.Membresia{
		Email: email, Nombre: nombre, EmpresaID: empresaID,
		Rol: in.Rol, SedeID: sedeID, Estado: usuario.EstadoInvitada, Token: token,
	})
	t.audit.Append(evento(empresaID, actor, origen, "usuario.invitar", out.ID, email))
	return out, nil
}

// ReenviarInvitacion regenera el token de una invitación pendiente (invalida el
// enlace anterior y devuelve la membresía con el token nuevo). Audita el reenvío.
func (t *TenancyService) ReenviarInvitacion(empresaID, miembroID, actor, origen string) (usuario.Membresia, error) {
	m, ok := t.miembroDe(empresaID, miembroID)
	if !ok {
		return usuario.Membresia{}, errors.New("esa invitación no pertenece a la empresa")
	}
	if m.Estado != usuario.EstadoInvitada {
		return usuario.Membresia{}, ErrInvitacionYaAceptada
	}
	token, err := nuevoToken()
	if err != nil {
		return usuario.Membresia{}, err
	}
	m.Token = token
	out, ok := t.members.Update(m)
	if !ok {
		return usuario.Membresia{}, errors.New("no se pudo reenviar la invitación")
	}
	t.audit.Append(evento(empresaID, actor, origen, "usuario.invitar.reenviar", out.ID, out.Email))
	return out, nil
}

// CancelarInvitacion elimina una invitación pendiente (no toca miembros activos).
func (t *TenancyService) CancelarInvitacion(empresaID, miembroID, actor, origen string) error {
	m, ok := t.miembroDe(empresaID, miembroID)
	if !ok {
		return errors.New("esa invitación no pertenece a la empresa")
	}
	if m.Estado != usuario.EstadoInvitada {
		return errors.New("solo se pueden cancelar invitaciones pendientes")
	}
	if !t.members.Delete(m.ID) {
		return errors.New("no se pudo cancelar la invitación")
	}
	t.audit.Append(evento(empresaID, actor, origen, "usuario.invitar.cancelar", m.ID, m.Email))
	return nil
}
