package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
)

func cuentaPorCodigo(svc *application.Service, empresaID, codigo string) (string, bool, bool) {
	for _, c := range svc.PlanDeCuentas(empresaID) {
		if c.Codigo == codigo {
			return c.Nombre, c.Desactivada, true
		}
	}
	return "", false, false
}

// El plan se amplía con la plantilla del giro; esas cuentas NO son base (se pueden
// desactivar), a diferencia de las que usa el motor de asientos.
func TestPlanPorGiro_ExtiendeYEsEditable(t *testing.T) {
	svc, _ := nuevoServicio(t)
	plan := svc.PlanDeCuentas(empDemo)
	tiene := func(cod string) bool {
		for _, c := range plan {
			if c.Codigo == cod {
				return true
			}
		}
		return false
	}
	// Comunes a todo comercio (aparecen sea cual sea el giro).
	if !tiene("5203") || !tiene("5205") {
		t.Error("el plan debería incluir las cuentas comunes de plantilla (5203, 5205)")
	}
	if application.EsCuentaBase("5203") {
		t.Error("una cuenta de plantilla no debe considerarse base")
	}
	if _, err := svc.FijarCuentaActiva(empDemo, "5203", false, actorA, origenTst); err != nil {
		t.Errorf("una cuenta de plantilla debe poder desactivarse: %v", err)
	}
	// Plantilla específica por giro.
	hayMed := false
	for _, c := range application.PlanPorGiro("farmacia") {
		if c.Nombre == "Ventas de medicamentos" {
			hayMed = true
		}
	}
	if !hayMed {
		t.Error("el giro farmacia debería sugerir 'Ventas de medicamentos'")
	}
}

// Una subcuenta hereda el tipo de su padre; solo las HOJAS reciben asientos (un
// padre con subcuentas no se puede asentar).
func TestSubcuentas_HerenciaYSoloHojasAsientan(t *testing.T) {
	svc, _ := nuevoServicio(t)
	svc.PlanDeCuentas(empDemo)

	if _, err := svc.CrearCuenta(empDemo, "6100", "Gastos generales", "gasto", "", actorA, origenTst); err != nil {
		t.Fatalf("padre: %v", err)
	}
	// El tipo pasado se ignora: se hereda del padre (gasto).
	sub, err := svc.CrearCuenta(empDemo, "6100.01", "Papelería", "activo", "6100", actorA, origenTst)
	if err != nil {
		t.Fatalf("subcuenta: %v", err)
	}
	if sub.CodigoPadre != "6100" || sub.Tipo != "gasto" {
		t.Errorf("la subcuenta debe heredar el tipo del padre, se obtuvo %+v", sub)
	}
	if _, err := svc.CrearCuenta(empDemo, "6100.02", "X", "gasto", "9999", actorA, origenTst); !errors.Is(err, application.ErrCuentaPadreNoExiste) {
		t.Errorf("padre inexistente debe dar ErrCuentaPadreNoExiste, se obtuvo %v", err)
	}
	// Asentar en el PADRE (tiene subcuentas) se rechaza; en la HOJA funciona.
	alPadre := []application.LineaAsientoManual{{Codigo: "6100", Debe: 30}, {Codigo: "1101", Haber: 30}}
	if _, err := svc.RegistrarAsientoManual(empDemo, actorA, origenTst, "2999-02-01", "al padre", alPadre); !errors.Is(err, application.ErrCuentaConHijas) {
		t.Errorf("asentar en cuenta con subcuentas debe dar ErrCuentaConHijas, se obtuvo %v", err)
	}
	aHoja := []application.LineaAsientoManual{{Codigo: "6100.01", Debe: 30}, {Codigo: "1101", Haber: 30}}
	if _, err := svc.RegistrarAsientoManual(empDemo, actorA, origenTst, "2999-02-01", "papelería", aHoja); err != nil {
		t.Errorf("asentar en la subcuenta hoja debe funcionar: %v", err)
	}
}

// Crear una cuenta exige código único y tipo válido; la naturaleza se deriva.
func TestCrearCuenta_UnicaYTipoValido(t *testing.T) {
	svc, _ := nuevoServicio(t)

	c, err := svc.CrearCuenta(empDemo, "6101", "Gastos de mercadeo", "gasto", "", actorA, origenTst)
	if err != nil {
		t.Fatalf("crear cuenta: %v", err)
	}
	if c.Deudora != true { // un gasto es deudora
		t.Error("una cuenta de gasto debe ser deudora")
	}
	if _, _, ok := cuentaPorCodigo(svc, empDemo, "6101"); !ok {
		t.Error("la cuenta creada debe aparecer en el plan")
	}
	// Código duplicado (uno base).
	if _, err := svc.CrearCuenta(empDemo, "4101", "Otra", "ingreso", "", actorA, origenTst); !errors.Is(err, application.ErrCuentaExiste) {
		t.Errorf("código duplicado debe dar ErrCuentaExiste, se obtuvo %v", err)
	}
	// Tipo inválido y datos vacíos.
	if _, err := svc.CrearCuenta(empDemo, "6102", "X", "raro", "", actorA, origenTst); !errors.Is(err, application.ErrTipoCuentaInvalido) {
		t.Errorf("tipo inválido debe dar ErrTipoCuentaInvalido, se obtuvo %v", err)
	}
	if _, err := svc.CrearCuenta(empDemo, "", "Sin código", "activo", "", actorA, origenTst); !errors.Is(err, application.ErrDatosCuenta) {
		t.Errorf("código vacío debe dar ErrDatosCuenta, se obtuvo %v", err)
	}
}

// Renombrar cambia el nombre (código inmutable); es seguro sobre cuentas base.
func TestRenombrarCuenta_CambiaNombre(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.RenombrarCuenta(empDemo, "4101", "Ingresos por ventas", actorA, origenTst); err != nil {
		t.Fatalf("renombrar: %v", err)
	}
	if nombre, _, _ := cuentaPorCodigo(svc, empDemo, "4101"); nombre != "Ingresos por ventas" {
		t.Errorf("el nombre no se actualizó, se obtuvo %q", nombre)
	}
	if _, err := svc.RenombrarCuenta(empDemo, "0000", "No existe", actorA, origenTst); !errors.Is(err, application.ErrCuentaNoExiste) {
		t.Errorf("cuenta inexistente debe dar ErrCuentaNoExiste, se obtuvo %v", err)
	}
}

// Las cuentas BASE no se pueden desactivar; una cuenta propia sí (y se reactiva).
func TestFijarCuentaActiva_BaseProtegidaYPropiaAlterna(t *testing.T) {
	svc, _ := nuevoServicio(t)

	if _, err := svc.FijarCuentaActiva(empDemo, "4101", false, actorA, origenTst); !errors.Is(err, application.ErrCuentaBase) {
		t.Errorf("desactivar una cuenta base debe dar ErrCuentaBase, se obtuvo %v", err)
	}

	if _, err := svc.CrearCuenta(empDemo, "6201", "Cuenta de prueba", "gasto", "", actorA, origenTst); err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := svc.FijarCuentaActiva(empDemo, "6201", false, actorA, origenTst); err != nil {
		t.Fatalf("desactivar propia: %v", err)
	}
	if _, desactivada, _ := cuentaPorCodigo(svc, empDemo, "6201"); !desactivada {
		t.Error("la cuenta propia debería quedar desactivada")
	}
	if _, err := svc.FijarCuentaActiva(empDemo, "6201", true, actorA, origenTst); err != nil {
		t.Fatalf("reactivar: %v", err)
	}
	if _, desactivada, _ := cuentaPorCodigo(svc, empDemo, "6201"); desactivada {
		t.Error("la cuenta propia debería quedar activa de nuevo")
	}
}
