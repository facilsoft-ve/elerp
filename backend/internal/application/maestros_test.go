package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

/* --- Proveedores ----------------------------------------------------------- */

func TestCrearProveedor_ExigeNombre(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{Nombre: "  "}); !errors.Is(err, application.ErrProveedorSinNombre) {
		t.Fatalf("un proveedor sin nombre debe dar ErrProveedorSinNombre, se obtuvo: %v", err)
	}
}

// TestCrearCliente_ValidaDocumento verifica que el maestro rechace un RIF J con
// dígito verificador incorrecto y acepte el DV correcto (SENIAT módulo 11).
func TestCrearCliente_ValidaDocumento(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// DV incorrecto (J-12345678 debía terminar en 4, no en 5).
	if _, err := svc.CrearCliente(empDemo, actorA, origenTst, cliente.Cliente{
		Nombre: "RIF Malo", TipoDocumento: cliente.DocJ, Documento: "12345678-5",
	}); !errors.Is(err, fiscal.ErrRIFDigitoVerificador) {
		t.Fatalf("un RIF J con DV inválido debe dar ErrRIFDigitoVerificador, se obtuvo: %v", err)
	}
	// DV correcto → se acepta.
	if _, err := svc.CrearCliente(empDemo, actorA, origenTst, cliente.Cliente{
		Nombre: "RIF Bueno", TipoDocumento: cliente.DocJ, Documento: "12345678-4",
	}); err != nil {
		t.Fatalf("un RIF J con DV válido debe aceptarse, se obtuvo: %v", err)
	}
}

// TestCrearProveedor_ValidaDocumento verifica lo mismo para proveedores, cuyo
// documento viene CON prefijo de tipo.
func TestCrearProveedor_ValidaDocumento(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Prov Malo", Documento: "J-12345678-5",
	}); !errors.Is(err, fiscal.ErrRIFDigitoVerificador) {
		t.Fatalf("un RIF de proveedor con DV inválido debe rechazarse, se obtuvo: %v", err)
	}
	if _, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Prov Bueno", Documento: "J-12345678-4",
	}); err != nil {
		t.Fatalf("un RIF de proveedor con DV válido debe aceptarse, se obtuvo: %v", err)
	}
}

func TestProveedor_CicloDeVida(t *testing.T) {
	svc, _ := nuevoServicio(t)
	p, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "  Insumos del Sur  ", Documento: "J-40000000-9", Email: "ventas@sur.ve",
	})
	if err != nil {
		t.Fatalf("crear proveedor: %v", err)
	}
	if p.Nombre != "Insumos del Sur" {
		t.Errorf("el nombre debe normalizarse (trim), se obtuvo %q", p.Nombre)
	}
	if !p.Activo {
		t.Error("un proveedor nace activo")
	}
	if got, ok := svc.Proveedor(empDemo, p.ID); !ok || got.ID != p.ID {
		t.Fatalf("el proveedor debe recuperarse por id, se obtuvo %+v (%v)", got, ok)
	}

	// Edición de contacto.
	upd, err := svc.ActualizarProveedor(empDemo, p.ID, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Insumos del Sur C.A.", Telefono: "0212-5550000",
	})
	if err != nil {
		t.Fatalf("actualizar proveedor: %v", err)
	}
	if upd.Nombre != "Insumos del Sur C.A." || upd.Telefono != "0212-5550000" {
		t.Errorf("la edición no se aplicó: %+v", upd)
	}

	// Baja reversible: no se borra, queda inactivo.
	des, err := svc.DesactivarProveedor(empDemo, p.ID, actorA, origenTst)
	if err != nil {
		t.Fatalf("desactivar proveedor: %v", err)
	}
	if des.Activo {
		t.Error("tras desactivar, el proveedor debe quedar inactivo")
	}
	if _, ok := svc.Proveedor(empDemo, p.ID); !ok {
		t.Error("desactivar no debe borrar el proveedor")
	}
}

func TestActualizarProveedor_NoExiste(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.ActualizarProveedor(empDemo, "prov_fantasma", actorA, origenTst, proveedor.Proveedor{Nombre: "x"}); !errors.Is(err, application.ErrProveedorNoExiste) {
		t.Fatalf("editar un proveedor inexistente debe dar ErrProveedorNoExiste, se obtuvo: %v", err)
	}
	if _, err := svc.DesactivarProveedor(empDemo, "prov_fantasma", actorA, origenTst); !errors.Is(err, application.ErrProveedorNoExiste) {
		t.Fatalf("desactivar un proveedor inexistente debe dar ErrProveedorNoExiste, se obtuvo: %v", err)
	}
}

/* --- Clientes (CRM) -------------------------------------------------------- */

func TestCrearCliente_RechazaEmailInvalido(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CrearCliente(empDemo, actorA, origenTst, cliente.Cliente{
		Nombre: "Correo Malo", TipoDocumento: cliente.DocV, Documento: "12345678", Email: "no-es-un-correo",
	}); !errors.Is(err, application.ErrEmailInvalido) {
		t.Fatalf("un correo sin formato debe dar ErrEmailInvalido, se obtuvo: %v", err)
	}
}

func TestCrearCliente_DocumentoDuplicado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	base := cliente.Cliente{Nombre: "Primero", TipoDocumento: cliente.DocV, Documento: "99887766"}
	if _, err := svc.CrearCliente(empDemo, actorA, origenTst, base); err != nil {
		t.Fatalf("crear el primero: %v", err)
	}
	base.Nombre = "Segundo con mismo documento"
	if _, err := svc.CrearCliente(empDemo, actorA, origenTst, base); !errors.Is(err, application.ErrDocumentoDuplicado) {
		t.Fatalf("un documento repetido por empresa debe dar ErrDocumentoDuplicado, se obtuvo: %v", err)
	}
}

func TestCrearCliente_DefaultsActivoYManual(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCliente(empDemo, actorA, origenTst, cliente.Cliente{
		Nombre: "Cliente Nuevo", Documento: "11223344", // sin TipoDocumento
	})
	if err != nil {
		t.Fatalf("crear cliente: %v", err)
	}
	if c.TipoDocumento != cliente.DocV {
		t.Errorf("sin tipo de documento debe asumirse V, se obtuvo %q", c.TipoDocumento)
	}
	if !c.Activo || c.Origen != cliente.OrigenManual {
		t.Errorf("un cliente nace activo y de origen manual, se obtuvo activo=%v origen=%q", c.Activo, c.Origen)
	}
}

func TestActualizarCliente_PreservaOrigenYValida(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCliente(empDemo, actorA, origenTst, cliente.Cliente{
		Nombre: "Importado", TipoDocumento: cliente.DocJ, Documento: "50000000-6",
		Origen: cliente.OrigenOdoo, SistemaExterno: "odoo", IdExterno: "res.partner-42",
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	// Edición manual: cambia contacto pero NO debe reescribir la procedencia.
	upd, err := svc.ActualizarCliente(empDemo, actorA, c.ID, cliente.Cliente{
		Nombre: "Importado C.A.", TipoDocumento: cliente.DocJ, Documento: "50000000-6",
		Telefono: "0212-1234567", Activo: true, Origen: cliente.OrigenManual, // se intenta pisar el origen
	})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if upd.Origen != cliente.OrigenOdoo || upd.SistemaExterno != "odoo" || upd.IdExterno != "res.partner-42" {
		t.Errorf("la edición manual no debe reescribir la procedencia importada, se obtuvo %+v",
			[]any{upd.Origen, upd.SistemaExterno, upd.IdExterno})
	}
	if upd.Nombre != "Importado C.A." || upd.Telefono != "0212-1234567" {
		t.Errorf("la edición de contacto no se aplicó: %+v", upd)
	}
	// Documento vacío se rechaza.
	if _, err := svc.ActualizarCliente(empDemo, actorA, c.ID, cliente.Cliente{Nombre: "X", Documento: "  "}); err == nil {
		t.Error("editar un cliente sin documento debe rechazarse")
	}
}

func TestActualizarCliente_NoExiste(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.ActualizarCliente(empDemo, actorA, "cli_fantasma", cliente.Cliente{Nombre: "X", Documento: "1"}); !errors.Is(err, application.ErrClienteNoExiste) {
		t.Fatalf("editar un cliente inexistente debe dar ErrClienteNoExiste, se obtuvo: %v", err)
	}
}

/* --- Numeración fiscal (series) -------------------------------------------- */

func TestEstadoNumeracion_ReflejaLoEmitido(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("emitir para tener una serie con folios: %v", err)
	}
	estado := svc.EstadoNumeracion(empDemo)
	if len(estado) == 0 {
		t.Fatal("tras emitir debe haber al menos una serie con estado")
	}
	for _, s := range estado {
		if s.Proximo != s.Actual+1 {
			t.Errorf("el próximo folio debe ser actual+1: serie %q actual=%d proximo=%d", s.Serie, s.Actual, s.Proximo)
		}
	}
}

func TestFijarNumeracion_ForwardOnly(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// Firma: FijarNumeracion(empresaID, actor, origen, sedeID, serie, proximo).
	// Fijar el próximo folio de una serie nueva hacia adelante funciona.
	if err := svc.FijarNumeracion(empDemo, actorA, origenTst, sede1, "PRUEBA", 500); err != nil {
		t.Fatalf("fijar hacia adelante no debería fallar: %v", err)
	}
	// El estado refleja el salto: próximo = 500, actual = 499.
	visto := false
	for _, s := range svc.EstadoNumeracion(empDemo) {
		if s.Serie == "PRUEBA" && s.SedeID == sede1 {
			visto = true
			if s.Proximo != 500 || s.Actual != 499 {
				t.Errorf("tras fijar 500: actual esperado 499 y próximo 500, se obtuvo actual=%d proximo=%d", s.Actual, s.Proximo)
			}
		}
	}
	if !visto {
		t.Fatal("la serie fijada debe aparecer en el estado de numeración")
	}
	// Próximo < 1 se rechaza.
	if err := svc.FijarNumeracion(empDemo, actorA, origenTst, sede1, "PRUEBA", 0); !errors.Is(err, application.ErrProximoInvalido) {
		t.Fatalf("un próximo folio 0 debe dar ErrProximoInvalido, se obtuvo: %v", err)
	}
	// Retroceder (o repetir) el folio se rechaza: nunca se reasigna un folio usado.
	if err := svc.FijarNumeracion(empDemo, actorA, origenTst, sede1, "PRUEBA", 400); !errors.Is(err, fiscal.ErrFolioRetrocede) {
		t.Fatalf("retroceder el folio debe dar ErrFolioRetrocede, se obtuvo: %v", err)
	}
}
