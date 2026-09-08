package application_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/organizacion"
)

// Corte DURO de capacidad: con el uso en el tope, crear una sucursal o invitar un
// usuario nuevo se rechaza; con límite 0 (ilimitado) no. Facturar NUNCA se toca.
func TestEnforcementLimites_SucursalesYUsuarios(t *testing.T) {
	tn, _ := nuevaTenancy(t)
	org, _ := tn.OrgDeEmpresa(empDemo)
	usuarios, sucursales, _, _ := tn.ConteosDeOrg(org.ID)

	// Tope exactamente en el uso actual ⇒ crear de más se bloquea.
	if _, err := tn.FijarPlan(org.ID, "pyme", organizacion.Limites{Sucursales: sucursales, Usuarios: usuarios}, actorA, origenTst); err != nil {
		t.Fatalf("fijar plan: %v", err)
	}
	if _, err := tn.CrearSede(empDemo, actorA, origenTst, "Sucursal Extra", ""); !errors.Is(err, application.ErrLimiteSucursales) {
		t.Errorf("crear sede sobre el tope debe dar ErrLimiteSucursales, se obtuvo %v", err)
	}
	if _, err := tn.InvitarMiembro(empDemo, actorA, origenTst, application.InviteInput{Email: "extra@x.com", Nombre: "Extra", Rol: "vendedor", SedeID: sede2}); !errors.Is(err, application.ErrLimiteUsuarios) {
		t.Errorf("invitar sobre el tope debe dar ErrLimiteUsuarios, se obtuvo %v", err)
	}

	// Límite 0 = ilimitado ⇒ vuelve a permitir.
	if _, err := tn.FijarPlan(org.ID, "ilimitado", organizacion.Limites{}, actorA, origenTst); err != nil {
		t.Fatalf("fijar plan ilimitado: %v", err)
	}
	if _, err := tn.CrearSede(empDemo, actorA, origenTst, "Sucursal OK", ""); err != nil {
		t.Errorf("con límite 0 crear sede debe funcionar, se obtuvo %v", err)
	}
}

// Un sandbox clona config+sedes+maestros de la empresa origen pero nace con los
// LEDGERS vacíos, no cuenta para la facturación de la org, y no se puede clonar
// a partir de otro sandbox. Eliminarlo lo desactiva.
func TestSandbox_ClonaMaestrosLedgerVacioYNoFactura(t *testing.T) {
	svc, st := nuevoServicio(t)
	tn := application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)

	prodOrigen := len(svc.Productos(empDemo))
	sedesOrigen := len(tn.Sedes(empDemo))

	sb, sedeMap, err := tn.CrearSandboxBase(empDemo, 7, actorA, origenTst)
	if err != nil {
		t.Fatalf("crear sandbox: %v", err)
	}
	if !sb.Sandbox || sb.OrigenSandboxID != empDemo || sb.ExpiraEl == "" {
		t.Fatalf("empresa sandbox mal marcada: %+v", sb)
	}
	if len(sedeMap) != sedesOrigen {
		t.Errorf("sedes clonadas %d, esperaba %d", len(sedeMap), sedesOrigen)
	}

	clon := svc.ClonarMaestros(empDemo, sb.ID, sedeMap)
	if clon["productos"] != prodOrigen {
		t.Errorf("productos clonados %d, esperaba %d", clon["productos"], prodOrigen)
	}
	if got := len(svc.Productos(sb.ID)); got != prodOrigen {
		t.Errorf("productos en el sandbox %d, esperaba %d", got, prodOrigen)
	}
	// Ledger en blanco.
	if n := len(svc.Documentos(sb.ID)); n != 0 {
		t.Errorf("el sandbox debe nacer sin documentos, hay %d", n)
	}
	// Los almacenes clonados apuntan a las sedes NUEVAS del sandbox.
	nuevas := map[string]bool{}
	for _, id := range sedeMap {
		nuevas[id] = true
	}
	for _, a := range svc.Almacenes(sb.ID) {
		if a.SedeID != "" && !nuevas[a.SedeID] {
			t.Errorf("almacén %s con sede fuera del sandbox: %s", a.ID, a.SedeID)
		}
	}
	// No cuenta para la facturación/límites de la organización.
	org, _ := tn.OrgDeEmpresa(empDemo)
	_, _, empresaIDs, _ := tn.ConteosDeOrg(org.ID)
	for _, id := range empresaIDs {
		if id == sb.ID {
			t.Error("el sandbox no debe contar en ConteosDeOrg")
		}
	}
	// No se puede clonar un sandbox.
	if _, _, err := tn.CrearSandboxBase(sb.ID, 7, actorA, origenTst); !errors.Is(err, application.ErrSandboxDeSandbox) {
		t.Errorf("clonar un sandbox debe dar ErrSandboxDeSandbox, se obtuvo %v", err)
	}
	// Eliminar (soft) lo desactiva; y no se puede eliminar una empresa real.
	if err := tn.EliminarSandbox(sb.ID, actorA, origenTst); err != nil {
		t.Fatalf("eliminar sandbox: %v", err)
	}
	if emp, _ := tn.Empresa(sb.ID); emp.Activa {
		t.Error("el sandbox eliminado debe quedar inactivo")
	}
	if err := tn.EliminarSandbox(empDemo, actorA, origenTst); !errors.Is(err, application.ErrNoEsSandbox) {
		t.Errorf("eliminar una empresa real debe dar ErrNoEsSandbox, se obtuvo %v", err)
	}
}

// El export por-tenant trae los datos del demo y NO trae nada de un tenant
// inexistente (el volcado se apoya en métodos ya aislados por empresaID).
func TestExportarTenant_TieneDatosYAisla(t *testing.T) {
	svc, _ := nuevoServicio(t)
	exp := svc.ExportarTenant(empDemo)
	if reflect.ValueOf(exp["productos"]).Len() == 0 {
		t.Error("el export del demo debería traer productos")
	}
	if reflect.ValueOf(exp["documentos"]).Len() == 0 {
		t.Error("el export del demo debería traer documentos")
	}
	vacio := svc.ExportarTenant("emp_fantasma")
	if reflect.ValueOf(vacio["productos"]).Len() != 0 || reflect.ValueOf(vacio["documentos"]).Len() != 0 {
		t.Error("un tenant inexistente no debe traer datos (aislamiento)")
	}
}

// El log de cambios se filtra por rango [desde,hasta], viene más nuevo primero y
// se acota por límite. Se usa un empresaID propio para aislar del ruido del seed.
func TestAuditoriaEntre_FiltraOrdenaYAcota(t *testing.T) {
	svc, st := nuevoServicio(t)
	const emp = "emp_test_audit"
	st.Audit.Append(auditoria.Evento{EmpresaID: emp, Accion: "ene", Fecha: "2026-01-10T08:00:00Z"})
	st.Audit.Append(auditoria.Evento{EmpresaID: emp, Accion: "feb", Fecha: "2026-02-15T09:00:00Z"})
	st.Audit.Append(auditoria.Evento{EmpresaID: emp, Accion: "mar", Fecha: "2026-03-20T10:00:00Z"})

	feb := svc.AuditoriaEntre(emp, "2026-02-01", "2026-02-28", 0)
	if len(feb) != 1 || feb[0].Accion != "feb" {
		t.Fatalf("febrero debería traer solo 'feb', se obtuvo %+v", feb)
	}

	todo := svc.AuditoriaEntre(emp, "", "", 0)
	if len(todo) != 3 {
		t.Fatalf("sin rango debería traer 3, se obtuvo %d", len(todo))
	}
	if todo[0].Accion != "mar" {
		t.Errorf("debe venir el más nuevo primero, se obtuvo %s", todo[0].Accion)
	}

	uno := svc.AuditoriaEntre(emp, "", "", 1)
	if len(uno) != 1 || uno[0].Accion != "mar" {
		t.Errorf("límite 1 + orden desc debería dar solo 'mar', se obtuvo %+v", uno)
	}

	// `hasta` como fecha sin hora debe incluir TODO el día 20 (evento de las 10:00).
	marzo := svc.AuditoriaEntre(emp, "2026-03-01", "2026-03-20", 0)
	if len(marzo) != 1 {
		t.Errorf("hasta=YYYY-MM-DD debe incluir el día completo, se obtuvo %d", len(marzo))
	}
}

// La vista de plataforma cruza el aislamiento por tenant: lista la organización
// del seed con su empresa demo, sus sedes y el conteo de usuarios.
func TestTenantsPlataforma_ListaOrgYEmpresaDemo(t *testing.T) {
	tn, _ := nuevaTenancy(t)

	tenants := tn.TenantsPlataforma()
	if len(tenants) == 0 {
		t.Fatal("la plataforma debería ver al menos la organización del seed")
	}
	var demo *application.PlatformEmpresaView
	for i := range tenants {
		if tenants[i].Estado == "" {
			t.Errorf("org %s sin estado", tenants[i].ID)
		}
		for j := range tenants[i].Empresas {
			if tenants[i].Empresas[j].ID == empDemo {
				demo = &tenants[i].Empresas[j]
			}
		}
	}
	if demo == nil {
		t.Fatalf("la empresa demo %q no aparece en la vista de plataforma", empDemo)
	}
	if len(demo.Sedes) == 0 {
		t.Error("la empresa demo debería tener al menos una sede")
	}
	if demo.Usuarios == 0 {
		t.Error("la empresa demo debería tener al menos un usuario (la dueña)")
	}
}

// Fijar plan guarda plan + límites en la organización y se refleja en la vista.
func TestFijarPlan_GuardaLimites(t *testing.T) {
	tn, _ := nuevaTenancy(t)
	org, ok := tn.OrgDeEmpresa(empDemo)
	if !ok {
		t.Fatalf("sin org para %q", empDemo)
	}
	lim := organizacion.Limites{FacturasMes: 200, Usuarios: 5, Sucursales: 3}
	if _, err := tn.FijarPlan(org.ID, "pyme", lim, actorA, origenTst); err != nil {
		t.Fatalf("fijar plan: %v", err)
	}
	got, _ := tn.Org(org.ID)
	if got.Plan != "pyme" || got.Limites.FacturasMes != 200 || got.Limites.Usuarios != 5 || got.Limites.Sucursales != 3 {
		t.Errorf("plan/límites no persistieron: %+v", got)
	}
	if _, err := tn.FijarPlan("org_x", "pyme", lim, actorA, origenTst); !errors.Is(err, application.ErrOrgNoExiste) {
		t.Errorf("org inexistente debe dar ErrOrgNoExiste, se obtuvo %v", err)
	}
}

// ConteosDeOrg cuenta usuarios distintos y sucursales de la organización del seed.
func TestConteosDeOrg_UsuariosYSucursales(t *testing.T) {
	tn, _ := nuevaTenancy(t)
	org, _ := tn.OrgDeEmpresa(empDemo)
	usuarios, sucursales, empresaIDs, ok := tn.ConteosDeOrg(org.ID)
	if !ok {
		t.Fatal("ConteosDeOrg debería encontrar la org del seed")
	}
	if usuarios < 1 || sucursales < 1 || len(empresaIDs) < 1 {
		t.Errorf("conteos inválidos: usuarios=%d sucursales=%d empresas=%d", usuarios, sucursales, len(empresaIDs))
	}
}

// Cuota marca 'excede' solo cuando hay tope (>0) superado; 0 = ilimitado.
func TestCuota_ExcedeSoloConTope(t *testing.T) {
	if application.Cuota(300, 200).Excede != true {
		t.Error("300 sobre tope 200 debe exceder")
	}
	if application.Cuota(300, 0).Excede != false {
		t.Error("tope 0 (ilimitado) nunca excede")
	}
	if application.Cuota(50, 200).Excede != false {
		t.Error("50 bajo tope 200 no excede")
	}
}

// FacturasDelMes cuenta solo facturas del mes indicado para la empresa.
func TestFacturasDelMes_CuentaPorMes(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// El seed tiene documentos; el acumulado del mes en curso es ≥ 0 y un mes
	// lejano en el pasado sin datos debe dar 0.
	if n := svc.FacturasDelMes(empDemo, "1999-01"); n != 0 {
		t.Errorf("un mes sin facturas debe dar 0, se obtuvo %d", n)
	}
}

// Suspender la organización deja de considerarla activa (lo que bloquea
// empresaContext); reactivarla la vuelve a habilitar. Valida además los errores.
func TestFijarEstadoOrg_SuspendeYReactiva(t *testing.T) {
	tn, _ := nuevaTenancy(t)

	org, ok := tn.OrgDeEmpresa(empDemo)
	if !ok {
		t.Fatalf("no se encontró la organización de %q", empDemo)
	}
	if !tn.OrgActivaDeEmpresa(empDemo) {
		t.Fatal("la organización del seed debería empezar activa")
	}

	if _, err := tn.FijarEstadoOrg(org.ID, "suspendida", actorA, origenTst); err != nil {
		t.Fatalf("suspender: %v", err)
	}
	if tn.OrgActivaDeEmpresa(empDemo) {
		t.Error("tras suspender, la organización NO debe considerarse activa")
	}

	if _, err := tn.FijarEstadoOrg(org.ID, "activa", actorA, origenTst); err != nil {
		t.Fatalf("reactivar: %v", err)
	}
	if !tn.OrgActivaDeEmpresa(empDemo) {
		t.Error("tras reactivar, la organización debe volver a estar activa")
	}

	if _, err := tn.FijarEstadoOrg(org.ID, "vacaciones", actorA, origenTst); !errors.Is(err, application.ErrEstadoOrgInvalido) {
		t.Errorf("un estado inválido debe dar ErrEstadoOrgInvalido, se obtuvo %v", err)
	}
	if _, err := tn.FijarEstadoOrg("org_inexistente", "activa", actorA, origenTst); !errors.Is(err, application.ErrOrgNoExiste) {
		t.Errorf("una org inexistente debe dar ErrOrgNoExiste, se obtuvo %v", err)
	}
}

// Activar/desactivar una empresa se refleja en el detalle de plataforma.
func TestFijarEmpresaActiva_SeReflejaEnElDetalle(t *testing.T) {
	tn, _ := nuevaTenancy(t)

	if _, err := tn.FijarEmpresaActiva(empDemo, false, actorA, origenTst); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	view, ok := tn.TenantPlataforma(empDemo)
	if !ok || len(view.Empresas) == 0 {
		t.Fatal("no se obtuvo el detalle del tenant")
	}
	if view.Empresas[0].Activa {
		t.Error("la empresa debería quedar como no activa")
	}

	if _, err := tn.FijarEmpresaActiva(empDemo, true, actorA, origenTst); err != nil {
		t.Fatalf("reactivar: %v", err)
	}
	view, _ = tn.TenantPlataforma(empDemo)
	if !view.Empresas[0].Activa {
		t.Error("la empresa debería quedar activa de nuevo")
	}
}
