package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* Maestro de conceptos ISLR. Lo que se prueba acá es que la tarifa DEJE de
 * teclearse: el usuario elige el concepto y el sistema resuelve cuánto retener.
 * Un error en este cálculo es un impuesto mal enterado al SENIAT. */

func servicioConConceptos(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConConceptosISLR(st.ConceptosISLR)
	return svc
}

// El maestro se siembra SOLO la primera vez que se pide: sin esto, una empresa
// ya creada (todas, cuando esto se construyó) se quedaría con la tabla vacía.
func TestConceptosISLR_SeSiembranAlPrimerUso(t *testing.T) {
	svc := servicioConConceptos(t)
	primero := svc.ConceptosISLR(empDemo)
	if len(primero) == 0 {
		t.Fatal("el maestro debería sembrarse al primer uso")
	}
	segundo := svc.ConceptosISLR(empDemo)
	if len(segundo) != len(primero) {
		t.Errorf("la siembra no puede repetirse: %d → %d", len(primero), len(segundo))
	}
}

// LA función del maestro: elegir el concepto y que la tarifa venga con él.
func TestSugerirRetencionISLR_ResuelveLaTarifa(t *testing.T) {
	svc := servicioConConceptos(t)
	sug, err := svc.SugerirRetencionISLR(empDemo, "honorarios", fiscal.SujetoNaturalResidente, 10000)
	if err != nil {
		t.Fatalf("sugerir: %v", err)
	}
	if sug.Porcentaje != 3 {
		t.Errorf("honorarios a persona natural = 3%%, dio %v", sug.Porcentaje)
	}
	if !casi(sug.Monto, 300) {
		t.Errorf("retención = %v, se esperaban 300 (10.000 × 3%%)", sug.Monto)
	}
	if !sug.Retiene {
		t.Error("con monto > 0 tiene que marcar que sí retiene")
	}
}

// La misma actividad tiene tarifa distinta según a quién se le retiene. Es la
// razón de que el maestro lleve el tipo de sujeto.
func TestSugerirRetencionISLR_DistingueElSujeto(t *testing.T) {
	svc := servicioConConceptos(t)
	nat, _ := svc.SugerirRetencionISLR(empDemo, "honorarios", fiscal.SujetoNaturalResidente, 10000)
	jur, _ := svc.SugerirRetencionISLR(empDemo, "honorarios", fiscal.SujetoJuridicaDomiciliada, 10000)
	if nat.Porcentaje == jur.Porcentaje {
		t.Errorf("natural y jurídica no pueden dar la misma tarifa: %v", nat.Porcentaje)
	}
}

func TestSugerirRetencionISLR_ConceptoDesconocido(t *testing.T) {
	svc := servicioConConceptos(t)
	if _, err := svc.SugerirRetencionISLR(empDemo, "no-existe", fiscal.SujetoNaturalResidente, 100); !errors.Is(err, application.ErrConceptoNoExiste) {
		t.Fatalf("se esperaba ErrConceptoNoExiste, se obtuvo: %v", err)
	}
}

// Una tarifa fuera de (0, 100] no es una tarifa: en 0 no retiene nada y se ve
// configurada; por encima de 100 se quedaría con más de lo facturado.
func TestGuardarConceptoISLR_ValidaLaTarifa(t *testing.T) {
	svc := servicioConConceptos(t)
	base := fiscal.ConceptoISLR{Codigo: "x", Nombre: "X", Sujeto: fiscal.SujetoNaturalResidente, Porcentaje: 5}
	for _, pct := range []float64{0, -3, 101} {
		c := base
		c.Porcentaje = pct
		if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, c); !errors.Is(err, application.ErrConceptoInvalido) {
			t.Errorf("la tarifa %v debería rechazarse, dio: %v", pct, err)
		}
	}
	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, base); err != nil {
		t.Errorf("una tarifa válida debería aceptarse: %v", err)
	}
}

func TestGuardarConceptoISLR_ExigeSujetoValido(t *testing.T) {
	svc := servicioConConceptos(t)
	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, fiscal.ConceptoISLR{
		Codigo: "x", Nombre: "X", Sujeto: "cualquiera", Porcentaje: 5,
	}); !errors.Is(err, application.ErrConceptoInvalido) {
		t.Fatalf("un sujeto desconocido debe rechazarse, dio: %v", err)
	}
}

// Editar la tabla NO puede alterar lo ya retenido: el comprobante copió su
// porcentaje y su sustraendo, y es append-only. Se comprueba que el maestro
// cambie y que la tabla siga resolviendo lo nuevo.
func TestGuardarConceptoISLR_EditarCambiaLoNuevo(t *testing.T) {
	svc := servicioConConceptos(t)
	var honorarios fiscal.ConceptoISLR
	for _, c := range svc.ConceptosISLR(empDemo) {
		if c.Codigo == "honorarios" && c.Sujeto == fiscal.SujetoNaturalResidente {
			honorarios = c
		}
	}
	if honorarios.ID == "" {
		t.Fatal("no se encontró el concepto sembrado")
	}
	honorarios.Porcentaje = 6
	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, honorarios); err != nil {
		t.Fatalf("editar: %v", err)
	}
	sug, _ := svc.SugerirRetencionISLR(empDemo, "honorarios", fiscal.SujetoNaturalResidente, 10000)
	if !casi(sug.Monto, 600) {
		t.Errorf("tras subir la tarifa a 6%%, la retención debería ser 600, dio %v", sug.Monto)
	}
}

func TestConceptosISLR_AisladosPorEmpresa(t *testing.T) {
	svc := servicioConConceptos(t)
	svc.ConceptosISLR(empDemo)
	if _, err := svc.SugerirRetencionISLR("emp_otra", "honorarios", fiscal.SujetoNaturalResidente, 100); err == nil {
		// Otra empresa siembra su PROPIA tabla, no ve la ajena: que resuelva es
		// correcto, pero tiene que ser con sus filas.
		propios := svc.ConceptosISLR("emp_otra")
		for _, c := range propios {
			if c.EmpresaID != "emp_otra" {
				t.Fatalf("se filtró un concepto de otra empresa: %+v", c)
			}
		}
	}
}

/* --- El maestro enganchado al registro de retenciones -------------------- */

// LO QUE PEDÍA LA CONTADORA: elegir el concepto y que la tarifa venga con él.
// Se manda un porcentaje ABSURDO a propósito: el maestro tiene que pisarlo.
func TestRegistrarRetencion_LaTarifaSaleDelMaestroNoDeLoTecleado(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConConceptosISLR(st.ConceptosISLR)
	doc := ventaCreditoIVA100(t, svc)

	r, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: "islr", NumeroComprobante: "2026090000001", Fecha: "2026-09-16",
		Base: 10000, Porcentaje: 99, // disparate: el maestro manda
		ConceptoCodigo: "honorarios", Sujeto: fiscal.SujetoNaturalResidente,
	})
	if err != nil {
		t.Fatalf("registrar: %v", err)
	}
	if !casi(r.Porcentaje, 3) {
		t.Errorf("la tarifa debe salir del maestro (3%%), quedó %v", r.Porcentaje)
	}
	if !casi(r.MontoRetenido, 300) {
		t.Errorf("retenido = %v, se esperaban 300", r.MontoRetenido)
	}
	if r.Concepto == "" {
		t.Error("el nombre del concepto debería copiarse del maestro")
	}
}

// Un concepto inexistente FALLA en vez de retener con una tarifa inventada:
// enterar de más o de menos al SENIAT es peor que no poder registrar.
func TestRegistrarRetencion_ConceptoDesconocidoFalla(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConConceptosISLR(st.ConceptosISLR)
	doc := ventaCreditoIVA100(t, svc)

	_, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: "islr", NumeroComprobante: "2026090000002", Fecha: "2026-09-16",
		Base: 10000, Porcentaje: 3,
		ConceptoCodigo: "no-existe", Sujeto: fiscal.SujetoNaturalResidente,
	})
	if !errors.Is(err, application.ErrConceptoNoExiste) {
		t.Fatalf("se esperaba ErrConceptoNoExiste, se obtuvo: %v", err)
	}
}

// Sin concepto elegido sigue valiendo lo tecleado: no se rompe a quien ya
// registraba retenciones a mano.
func TestRegistrarRetencion_SinConceptoSigueComoAntes(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConConceptosISLR(st.ConceptosISLR)
	doc := ventaCreditoIVA100(t, svc)

	r, err := svc.RegistrarRetencionRecibida(empDemo, actorA, origenTst, doc.ID, application.EntradaRetencion{
		Impuesto: "islr", NumeroComprobante: "2026090000003", Fecha: "2026-09-16",
		Base: 10000, Porcentaje: 7, Concepto: "a mano", Sustraendo: 0,
	})
	if err != nil {
		t.Fatalf("registrar: %v", err)
	}
	if !casi(r.Porcentaje, 7) || !casi(r.MontoRetenido, 700) {
		t.Errorf("lo tecleado debe seguir valiendo: %v%% → %v", r.Porcentaje, r.MontoRetenido)
	}
}

// El comprobante que se EMITE al proveedor es justo donde la tarifa de ISLR se
// erraba a mano (y donde equivocarse significa enterarle de más o de menos al
// SENIAT). Igual que en la recibida, se manda un porcentaje absurdo a propósito:
// el maestro tiene que pisarlo.
func TestRetencionEmitida_LaTarifaSaleDelMaestro(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConConceptosISLR(st.ConceptosISLR)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-MAESTRO", "00-MAESTRO", time.Now().UTC().Format(time.RFC3339Nano))

	r, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoISLR, Fecha: "2026-09-16",
		Base: 10000, Porcentaje: 99, // disparate: el maestro manda
		ConceptoCodigo: "honorarios", Sujeto: fiscal.SujetoJuridicaDomiciliada,
	})
	if err != nil {
		t.Fatalf("registrar emitida: %v", err)
	}
	if !casi(r.Porcentaje, 5) { // jurídica domiciliada: 5%, no 3%
		t.Errorf("la tarifa debe salir del maestro (5%%), quedó %v", r.Porcentaje)
	}
	if !casi(r.MontoRetenido, 500) {
		t.Errorf("retenido = %v, se esperaban 500", r.MontoRetenido)
	}
	if r.Concepto == "" {
		t.Error("el nombre del concepto debería copiarse del maestro")
	}
}

// Sin concepto elegido, la emitida sigue comportándose como antes.
func TestRetencionEmitida_SinConceptoSigueComoAntes(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConConceptosISLR(st.ConceptosISLR)
	sku := primerSKU(t, svc)
	fc := ocFacturada(t, svc, sku, "F-AMANO", "00-AMANO", time.Now().UTC().Format(time.RFC3339Nano))

	r, err := svc.RegistrarRetencionEmitida(empDemo, actorA, origenTst, fc.ID, application.EntradaRetencion{
		Impuesto: fiscal.ImpuestoISLR, Fecha: "2026-09-16",
		Base: 10000, Porcentaje: 7, Concepto: "a mano", Sustraendo: 0,
	})
	if err != nil {
		t.Fatalf("registrar emitida: %v", err)
	}
	if !casi(r.Porcentaje, 7) || !casi(r.MontoRetenido, 700) {
		t.Errorf("lo tecleado debe seguir valiendo: %v%% → %v", r.Porcentaje, r.MontoRetenido)
	}
}
