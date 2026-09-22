package application_test

import (
	"strings"
	"testing"

	"github.com/mornix/elerp/internal/adapter/unidigital"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cliente"
	fd "github.com/mornix/elerp/internal/domain/facturaciondigital"
)

/* MÓDULO DE FACTURACIÓN DIGITAL.
 *
 * Lo que se prueba acá es que el módulo NO se pueda encender a medias y que,
 * apagado, no exista: una empresa que no lo usa tiene que facturar exactamente
 * como antes. */

func TestDigital_ApagadoNoHaceNada(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConFacturacionDigital(st.ConfigDigital, st.EmisionesDigitales)
	cfg := svc.ConfigDigital(empDemo)
	if cfg.Activa {
		t.Fatal("el módulo tiene que nacer apagado")
	}
	// Con el módulo apagado, ningún canal aplica.
	for _, canal := range []string{fd.CanalPOS, fd.CanalVentas} {
		if cfg.AplicaA(canal) {
			t.Fatalf("apagado, el canal %q no debe aplicar", canal)
		}
	}
}

// No se deja activar sin serie y sucursal: sin ellas la imprenta rechaza todo, y
// es mejor no dejar encender el módulo que fallar en la primera venta del día.
func TestDigital_NoSeActivaAMedias(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConFacturacionDigital(st.ConfigDigital, st.EmisionesDigitales)
	_, err := svc.GuardarConfigDigital(empDemo, actorA, origenTst, application.EntradaConfigDigital{
		Activa: true, PorPOS: true, Usuario: "u@x.com", Password: "clave",
	})
	if err == nil {
		t.Fatal("activar sin serie ni sucursal debería rechazarse")
	}
}

// Los canales son independientes: el mostrador y el módulo de ventas son
// decisiones operativas distintas.
func TestDigital_CanalesIndependientes(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConFacturacionDigital(st.ConfigDigital, st.EmisionesDigitales)
	cfg, err := svc.GuardarConfigDigital(empDemo, actorA, origenTst, application.EntradaConfigDigital{
		Activa: true, PorPOS: true, PorVentas: false,
		Usuario: "u@x.com", Password: "clave",
		SerieStrongID: "serie-1", CorreoRespaldo: "facturas@mornix.tech",
	})
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if !cfg.AplicaA(fd.CanalPOS) {
		t.Fatal("el mostrador quedó encendido y no aplica")
	}
	if cfg.AplicaA(fd.CanalVentas) {
		t.Fatal("el módulo de ventas quedó apagado y sí aplica")
	}
}

// La contraseña se guarda HASHEADA y no vuelve por JSON: es lo que viaja a la
// API, así que guardar el texto plano no aporta nada y sí expone.
func TestDigital_LaClaveSeGuardaHasheada(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConFacturacionDigital(st.ConfigDigital, st.EmisionesDigitales)
	if _, err := svc.GuardarConfigDigital(empDemo, actorA, origenTst, application.EntradaConfigDigital{
		Activa: true, PorPOS: true, Usuario: "u@x.com", Password: "secreta",
		SerieStrongID: "serie-1", CorreoRespaldo: "facturas@mornix.tech",
	}); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	cfg := svc.ConfigDigital(empDemo)
	if cfg.PasswordSHA512 == "secreta" {
		t.Fatal("la contraseña quedó en texto plano")
	}
	if cfg.PasswordSHA512 != unidigital.HashPassword("secreta") {
		t.Fatal("el digest guardado no es el que espera la API")
	}
	// Y una contraseña vacía al editar no la borra: se puede cambiar el resto sin
	// volver a teclearla.
	if _, err := svc.GuardarConfigDigital(empDemo, actorA, origenTst, application.EntradaConfigDigital{
		Activa: true, PorPOS: true, PorVentas: true, Usuario: "u@x.com", Password: "",
		SerieStrongID: "serie-1", CorreoRespaldo: "facturas@mornix.tech",
	}); err != nil {
		t.Fatalf("editar sin clave: %v", err)
	}
	if svc.ConfigDigital(empDemo).PasswordSHA512 == "" {
		t.Fatal("editar sin contraseña la borró")
	}
}

// El token de la página pública es aleatorio y no derivado del documento: esa
// página no tiene clave, y uno adivinable dejaría recorrer las facturas.
func TestDigital_ElTokenPublicoNoEsPredecible(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConFacturacionDigital(st.ConfigDigital, st.EmisionesDigitales)
	if _, err := svc.GuardarConfigDigital(empDemo, actorA, origenTst, application.EntradaConfigDigital{
		Activa: true, PorPOS: true, Usuario: "u@x.com", Password: "clave",
		SerieStrongID: "serie-1", CorreoRespaldo: "facturas@mornix.tech",
	}); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	vistos := map[string]bool{}
	for i := 0; i < 3; i++ {
		doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
			Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 10}},
			Pagos:  []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 500, Moneda: "VES"}},
		})
		if err != nil {
			t.Fatalf("emitir: %v", err)
		}
		e, ok := svc.EncolarEmision(empDemo, fd.CanalPOS, doc)
		if !ok {
			t.Fatal("con el canal encendido, la emisión debe encolarse")
		}
		if len(e.Token) < 12 {
			t.Fatalf("token demasiado corto para ser impredecible: %q", e.Token)
		}
		if vistos[e.Token] {
			t.Fatal("dos emisiones con el mismo token público")
		}
		vistos[e.Token] = true
		// El correlativo de la imprenta es PROPIO: no se reusa el del documento.
		if e.Numero != i+1 {
			t.Fatalf("correlativo de la imprenta = %d, se esperaba %d", e.Numero, i+1)
		}
		if e.Estado != fd.EstadoPendiente {
			t.Fatalf("la emisión nace en %q, debería nacer pendiente", e.Estado)
		}
	}
}

// Un documento no se encola dos veces: el reintento vive en la emisión que ya
// existe, no en una nueva que quemaría otro correlativo.
func TestDigital_NoSeEncolaDosVeces(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConFacturacionDigital(st.ConfigDigital, st.EmisionesDigitales)
	if _, err := svc.GuardarConfigDigital(empDemo, actorA, origenTst, application.EntradaConfigDigital{
		Activa: true, PorVentas: true, Usuario: "u@x.com", Password: "clave",
		SerieStrongID: "serie-1", CorreoRespaldo: "facturas@mornix.tech",
	}); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 10}},
		Pagos:  []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 500, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	a, _ := svc.EncolarEmision(empDemo, fd.CanalVentas, doc)
	b, _ := svc.EncolarEmision(empDemo, fd.CanalVentas, doc)
	if a.ID != b.ID {
		t.Fatalf("el mismo documento se encoló dos veces: %s y %s", a.ID, b.ID)
	}
}

/* EXIGIR EL CLIENTE ANTES DE COBRAR.
 *
 * Con la imprenta encendida la factura necesita cédula/RIF y dirección. Se
 * pregunta ANTES de emitir porque después el cliente ya se fue: quedaría una
 * venta cobrada que nunca va a ser fiscal, y nadie a quien pedirle la cédula.
 */
func TestDigital_ExigeClienteAntesDeCobrar(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConFacturacionDigital(st.ConfigDigital, st.EmisionesDigitales)
	if _, err := svc.GuardarConfigDigital(empDemo, actorA, origenTst, application.EntradaConfigDigital{
		Activa: true, PorPOS: true, Usuario: "u@x.com", Password: "clave",
		SerieStrongID: "serie-1", CorreoRespaldo: "facturas@mornix.tech",
	}); err != nil {
		t.Fatalf("activar: %v", err)
	}

	// Sin cliente: se niega, y el motivo es para leérselo al cajero.
	motivo := svc.FaltaClienteParaImprenta(empDemo, fd.CanalPOS, "")
	if motivo == "" {
		t.Fatal("sin cliente identificado la venta tenía que negarse antes de emitir")
	}
	if !strings.Contains(motivo, "cliente") {
		t.Fatalf("el motivo tiene que decir qué falta, en castellano: %q", motivo)
	}

	// Un cliente sin dirección tampoco sirve, y el aviso lo NOMBRA: «faltan
	// datos» obliga a adivinar cuál, con el cliente esperando en el mostrador.
	sinDir := st.Clientes.Create(cliente.Cliente{
		EmpresaID: empDemo, Nombre: "Pedro Sin Casa", TipoDocumento: "V", Documento: "V-12345678",
	})
	m2 := svc.FaltaClienteParaImprenta(empDemo, fd.CanalPOS, sinDir.ID)
	if m2 == "" || !strings.Contains(m2, "dirección") {
		t.Fatalf("debería reclamar la dirección y nombrar al cliente: %q", m2)
	}

	// Completo: pasa.
	ok := st.Clientes.Create(cliente.Cliente{
		EmpresaID: empDemo, Nombre: "Ana Completa", TipoDocumento: "V", Documento: "V-87654321",
		Direccion: "Av. Bolívar, Caracas",
	})
	if m3 := svc.FaltaClienteParaImprenta(empDemo, fd.CanalPOS, ok.ID); m3 != "" {
		t.Fatalf("un cliente completo no debería frenar la venta: %q", m3)
	}

	// Y con el módulo apagado no se le pide nada a nadie: ElERP factura como antes.
	if m4 := svc.FaltaClienteParaImprenta(empDemo, fd.CanalVentas, ""); m4 != "" {
		t.Fatalf("el canal apagado no puede exigir cliente: %q", m4)
	}
}
