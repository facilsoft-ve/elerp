package usuario

import "testing"

// La autorización efectiva (RBAC/ABAC: qué puede hacer cada rol, el read-only de
// la contadora fuera de Contabilidad/Tesorería, la sede fija de vendedor/cajero)
// se evalúa en la capa de aplicación/httpapi por petición, no aquí. La única
// lógica PURA de este paquete es el validador de rol de empresa, cuyo detalle de
// negocio relevante es que super_admin (rol de PLATAFORMA) NO se asigna por acá.

func TestRolValido(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		want   bool
	}{
		{"dueño", RolDueno, true},
		{"vendedor", RolVendedor, true},
		{"cajero", RolCajero, true},
		{"contadora", RolContadora, true},
		{"desarrollador", RolDesarrollador, true},
		{"super_admin NO es rol de empresa", "super_admin", false},
		{"vacío", "", false},
		{"desconocido", "gerente", false},
		{"mayúsculas no coinciden (exacto)", "Dueno", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := RolValido(c.in); got != c.want {
				t.Errorf("RolValido(%q) = %v; quería %v", c.in, got, c.want)
			}
		})
	}
}
