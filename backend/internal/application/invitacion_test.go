package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
)

// nuevaTenancy arma el servicio de tenancy sobre el store en memoria ya sembrado.
func nuevaTenancy(t *testing.T) (*application.TenancyService, func() int) {
	t.Helper()
	_, st := nuevoServicio(t)
	tn := application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)
	contarPendientes := func() int {
		n := 0
		for _, m := range tn.Miembros(empDemo) {
			if m.Estado != "active" {
				n++
			}
		}
		return n
	}
	return tn, contarPendientes
}

func TestInvitarMiembro_CreaPendienteConTokenYActivaAlAceptar(t *testing.T) {
	tn, _ := nuevaTenancy(t)

	m, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{
		Email: "Nuevo.Vendedor@Empresa.com", Nombre: "Nuevo Vendedor", Rol: "vendedor", SedeID: sede2,
	})
	if err != nil {
		t.Fatalf("invitar debería funcionar: %v", err)
	}
	if m.Estado != "invited" {
		t.Fatalf("la membresía nueva debe quedar invitada, se obtuvo: %q", m.Estado)
	}
	if m.Token == "" {
		t.Fatal("la invitación debe traer un token para el flujo de aceptación")
	}
	if m.Email != "nuevo.vendedor@empresa.com" {
		t.Fatalf("el correo debe normalizarse a minúsculas, se obtuvo: %q", m.Email)
	}
	if m.UsuarioID != "" {
		t.Fatal("una invitación pendiente no tiene usuario todavía")
	}

	// El preview del token existente debe verla.
	if _, err := tn.PreviewInvitacion(m.Token); err != nil {
		t.Fatalf("el token recién emitido debe ser válido en el preview: %v", err)
	}

	// Aceptar la invitación crea la credencial y activa la membresía → login nativo.
	if _, err := tn.AceptarInvitacion(m.Token, "Nuevo Vendedor", "clave-segura-8", origenTst); err != nil {
		t.Fatalf("aceptar la invitación emitida debe funcionar: %v", err)
	}
	if _, err := tn.LoginNativo("nuevo.vendedor@empresa.com", "clave-segura-8"); err != nil {
		t.Fatalf("tras aceptar, el login nativo del invitado debe funcionar: %v", err)
	}
}

func TestInvitarMiembro_SedeObligatoriaParaCajero(t *testing.T) {
	tn, _ := nuevaTenancy(t)

	if _, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{
		Email: "cajero.nuevo@empresa.com", Rol: "cajero",
	}); !errors.Is(err, application.ErrSedeRequerida) {
		t.Fatalf("un cajero sin sede debe rechazarse, se obtuvo: %v", err)
	}
	if _, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{
		Email: "cajero.nuevo@empresa.com", Rol: "cajero", SedeID: sede2,
	}); err != nil {
		t.Fatalf("con sede válida debe crear la invitación: %v", err)
	}
}

func TestInvitarMiembro_RechazaEmailInvalidoYRolInvalido(t *testing.T) {
	tn, _ := nuevaTenancy(t)

	if _, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{
		Email: "no-es-correo", Rol: "vendedor",
	}); !errors.Is(err, application.ErrEmailInvalido) {
		t.Fatalf("un correo inválido debe rechazarse, se obtuvo: %v", err)
	}
	if _, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{
		Email: "ok@empresa.com", Rol: "super_admin",
	}); !errors.Is(err, application.ErrRolInvalido) {
		t.Fatalf("un rol fuera de la empresa debe rechazarse, se obtuvo: %v", err)
	}
}

func TestInvitarMiembro_ReutilizaPendienteYRegeneraToken(t *testing.T) {
	tn, contarPendientes := nuevaTenancy(t)
	antes := contarPendientes()

	m1, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{
		Email: "repetido@empresa.com", Rol: "vendedor", SedeID: sede2,
	})
	if err != nil {
		t.Fatalf("primera invitación: %v", err)
	}
	m2, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{
		Email: "repetido@empresa.com", Rol: "contadora",
	})
	if err != nil {
		t.Fatalf("reinvitar el mismo correo debe reutilizar la pendiente: %v", err)
	}
	if m1.ID != m2.ID {
		t.Fatal("reinvitar debe reutilizar la misma membresía, no duplicar")
	}
	if m1.Token == m2.Token {
		t.Fatal("reinvitar debe regenerar el token (invalida el enlace anterior)")
	}
	if m2.Rol != "contadora" {
		t.Fatalf("reinvitar debe actualizar el rol, se obtuvo: %q", m2.Rol)
	}
	if got := contarPendientes() - antes; got != 1 {
		t.Fatalf("solo debe existir 1 invitación pendiente nueva, se contaron %d", got)
	}
}

func TestInvitarMiembro_RechazaMiembroActivoDuplicado(t *testing.T) {
	tn, _ := nuevaTenancy(t)
	// La dueña del seed ya es un miembro ACTIVO; invitarla otra vez debe fallar.
	var emailDueno string
	for _, m := range tn.Miembros(empDemo) {
		if m.Rol == "dueno" {
			emailDueno = m.Email
		}
	}
	if emailDueno == "" {
		t.Fatal("el seed debería traer una dueña activa")
	}
	if _, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{
		Email: emailDueno, Rol: "contadora",
	}); !errors.Is(err, application.ErrMiembroExistente) {
		t.Fatalf("invitar a un miembro ya activo debe rechazarse, se obtuvo: %v", err)
	}
}

func TestCancelarInvitacion_EliminaPendienteYNoTocaActivos(t *testing.T) {
	tn, _ := nuevaTenancy(t)

	m, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{
		Email: "cancelame@empresa.com", Rol: "contadora",
	})
	if err != nil {
		t.Fatalf("invitar: %v", err)
	}
	if err := tn.CancelarInvitacion(empDemo, m.ID, actorA, origenTst); err != nil {
		t.Fatalf("cancelar una pendiente debe funcionar: %v", err)
	}
	if _, err := tn.PreviewInvitacion(m.Token); err == nil {
		t.Fatal("tras cancelar, el token ya no debe ser válido")
	}
	// No se puede cancelar un miembro activo.
	var activo string
	for _, mm := range tn.Miembros(empDemo) {
		if mm.Estado == "active" {
			activo = mm.ID
		}
	}
	if err := tn.CancelarInvitacion(empDemo, activo, actorA, origenTst); err == nil {
		t.Fatal("cancelar un miembro activo debe rechazarse")
	}
}
