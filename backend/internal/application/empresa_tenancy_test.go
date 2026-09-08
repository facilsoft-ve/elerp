package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

func TestActualizarEmpresa_GuardaCamposNuevos(t *testing.T) {
	tn, _ := nuevaTenancy(t)
	actual, ok := tn.Empresa(empDemo)
	if !ok {
		t.Fatal("el seed debe traer la empresa demo")
	}
	out, err := tn.ActualizarEmpresa(empDemo, actorA, origenTst, application.EmpresaEdit{
		Nombre:              actual.Nombre,
		RIF:                 actual.RIF,
		ExigirCedulaCliente: true,
		TemaPantalla: empresa.TemaPantallaCliente{
			ColorEnfasis: "#09B69B",
			ColorTexto:   "#FFFFFF",
		},
	})
	if err != nil {
		t.Fatalf("actualizar empresa: %v", err)
	}
	if !out.ExigirCedulaCliente {
		t.Error("ExigirCedulaCliente debe quedar en true tras guardarlo")
	}
	if out.TemaPantalla.ColorEnfasis == "" {
		t.Error("el tema de la pantalla del cliente debe persistir el color de énfasis")
	}
	// La edición no debe tocar la organización dueña del tenant.
	if out.OrganizacionID != actual.OrganizacionID {
		t.Errorf("ActualizarEmpresa nunca debe cambiar la organización: %q → %q", actual.OrganizacionID, out.OrganizacionID)
	}
}

func TestActualizarEmpresa_RIFObligatorio(t *testing.T) {
	tn, _ := nuevaTenancy(t)
	if _, err := tn.ActualizarEmpresa(empDemo, actorA, origenTst, application.EmpresaEdit{
		Nombre: "Sin RIF", RIF: "   ",
	}); !errors.Is(err, application.ErrRIFRequerido) {
		t.Fatalf("guardar una empresa sin RIF debe dar ErrRIFRequerido, se obtuvo: %v", err)
	}
}

func TestActualizarEmpresa_NoExiste(t *testing.T) {
	tn, _ := nuevaTenancy(t)
	if _, err := tn.ActualizarEmpresa("emp_fantasma", actorA, origenTst, application.EmpresaEdit{RIF: "J-1"}); !errors.Is(err, application.ErrEmpresaNoExiste) {
		t.Fatalf("editar una empresa inexistente debe dar ErrEmpresaNoExiste, se obtuvo: %v", err)
	}
}

// TestAislamientoPorEmpresa comprueba el Principio 3: los maestros están aislados
// por empresaID, y una empresa jamás ve los datos de otra.
func TestAislamientoPorEmpresa(t *testing.T) {
	svc, _ := nuevoServicio(t)
	const otraEmp = "emp_otra"

	// Un proveedor y un cliente en OTRA empresa no deben verse desde empDemo.
	if _, err := svc.CrearProveedor(otraEmp, actorA, origenTst, proveedor.Proveedor{Nombre: "Ajeno"}); err != nil {
		t.Fatalf("crear proveedor en otra empresa: %v", err)
	}
	if _, err := svc.CrearCliente(otraEmp, actorA, origenTst, cliente.Cliente{
		Nombre: "Cliente Ajeno", TipoDocumento: cliente.DocV, Documento: "77777777",
	}); err != nil {
		t.Fatalf("crear cliente en otra empresa: %v", err)
	}

	for _, p := range svc.Proveedores(empDemo) {
		if p.Nombre == "Ajeno" {
			t.Fatal("empDemo no debe ver un proveedor creado en otra empresa")
		}
	}
	for _, c := range svc.Clientes(empDemo) {
		if c.Documento == "77777777" {
			t.Fatal("empDemo no debe ver un cliente creado en otra empresa")
		}
	}

	// Y la empresa recién poblada solo ve lo suyo: exactamente un proveedor.
	if provs := svc.Proveedores(otraEmp); len(provs) != 1 || provs[0].Nombre != "Ajeno" {
		t.Fatalf("otraEmp solo debe ver su propio proveedor, se obtuvo %+v", provs)
	}
	// Un cobro/consulta de cuentas por cobrar de una empresa vacía no arrastra
	// documentos del seed de empDemo.
	if res := svc.CuentasPorCobrar(otraEmp); len(res.Cuentas) != 0 {
		t.Fatalf("una empresa sin ventas no debe tener cuentas por cobrar, se obtuvo %d", len(res.Cuentas))
	}
}
