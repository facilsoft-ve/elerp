package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

/* Perfil fiscal y comercial del proveedor, y su proyección en la orden de compra.
 *
 * La empresa del seed ES agente de retención de IVA (75 %) e ISLR, así que lo que
 * decide si se retiene es el perfil del proveedor. Las órdenes se arman de 10 u a
 * Bs 100: subtotal 1.000, IVA 160, total 1.160. */

// proveedorConPerfil da de alta un proveedor con su perfil ya cargado.
func proveedorConPerfil(t *testing.T, svc *application.Service, p proveedor.Proveedor) proveedor.Proveedor {
	t.Helper()
	if p.Nombre == "" {
		p.Nombre = "Suplidora de prueba"
	}
	out, err := svc.CrearProveedor(empDemo, actorA, origenTst, p)
	if err != nil {
		t.Fatalf("crear proveedor con perfil: %v", err)
	}
	return out
}

// ocDe arma una orden de compra de 10 u a Bs 100 para un proveedor dado.
func ocDe(t *testing.T, svc *application.Service, provID, sku string, ret *application.RetencionesOCEntrada) (o struct {
	Total, RetIVA, RetISLR, Neto float64
	Condiciones, ConceptoISLR    string
	PctIVA, PctISLR, Sustraendo  float64
}) {
	t.Helper()
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provID, SedeID: sede1,
		Lineas:      []application.LineaOCEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 100}},
		Retenciones: ret,
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	o.Total, o.RetIVA, o.RetISLR, o.Neto = oc.Total, oc.RetencionIVAMonto, oc.RetencionISLRMonto, oc.NetoAPagar
	o.Condiciones, o.ConceptoISLR = oc.CondicionesPago, oc.RetencionISLRConcepto
	o.PctIVA, o.PctISLR, o.Sustraendo = oc.RetencionIVAPorcentaje, oc.RetencionISLRPorcentaje, oc.RetencionISLRSustraendo
	return o
}

// TestOC_ProyectaRetencionIVADelProveedor: el perfil del proveedor decide, y la
// orden responde «cuánto le vamos a pagar de verdad» antes de recibir nada.
func TestOC_ProyectaRetencionIVADelProveedor(t *testing.T) {
	svc := servicioConConceptos(t)
	sku := primerSKU(t, svc)
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Retenida IVA, C.A.", RetieneIVA: true, // sin % propio ⇒ el 75 de la empresa
	})

	o := ocDe(t, svc, prov.ID, sku, nil)
	casiEq(t, o.Total, 1160, "total de la orden (1.000 + IVA 160)")
	casiEq(t, o.PctIVA, 75, "el % sale de la empresa cuando el proveedor no trae el suyo")
	casiEq(t, o.RetIVA, 120, "retención de IVA proyectada (160 × 75%)")
	casiEq(t, o.RetISLR, 0, "sin ISLR: el proveedor no lo tiene activado")
	casiEq(t, o.Neto, 1040, "neto a pagar al proveedor (1.160 − 120)")
}

// TestOC_LaTarifaDeISLRSaleDelMaestro: el ISLR se retiene sobre el NETO (el ingreso
// del proveedor, sin IVA) y su tarifa NO se teclea — sale del maestro de conceptos
// según el código y el tipo de sujeto.
func TestOC_LaTarifaDeISLRSaleDelMaestro(t *testing.T) {
	svc := servicioConConceptos(t)
	sku := primerSKU(t, svc)
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre:     "Servicios Profesionales, C.A.",
		RetieneIVA: true, RetencionIVAPorcentaje: 100,
		RetieneISLR: true, ConceptoISLRCodigo: "honorarios", SujetoISLR: fiscal.SujetoNaturalResidente,
	})

	o := ocDe(t, svc, prov.ID, sku, nil)
	casiEq(t, o.RetIVA, 160, "el % propio del proveedor (100%) gana al de la empresa")
	// Honorarios a persona NATURAL residente: 3 % en el maestro.
	casiEq(t, o.PctISLR, 3, "la tarifa la pone el maestro, no la ficha del proveedor")
	casiEq(t, o.RetISLR, 30, "retención de ISLR (1.000 × 3%)")
	if o.ConceptoISLR != "Honorarios profesionales" {
		t.Errorf("la orden guarda el nombre del concepto del maestro, se obtuvo %q", o.ConceptoISLR)
	}
	casiEq(t, o.Neto, 970, "neto a pagar (1.160 − 160 − 30)")
}

// TestOC_ElSujetoDecideLaTarifa es la razón de ser del maestro: el MISMO concepto
// tiene dos tarifas según a quién se le retiene, y eso el texto libre no podía
// expresarlo.
func TestOC_ElSujetoDecideLaTarifa(t *testing.T) {
	svc := servicioConConceptos(t)
	sku := primerSKU(t, svc)

	natural := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Profesional independiente", RetieneISLR: true,
		ConceptoISLRCodigo: "honorarios", SujetoISLR: fiscal.SujetoNaturalResidente,
	})
	juridica := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Consultora, C.A.", RetieneISLR: true,
		ConceptoISLRCodigo: "honorarios", SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	})

	casiEq(t, ocDe(t, svc, natural.ID, sku, nil).RetISLR, 30, "honorarios a natural residente: 3%")
	casiEq(t, ocDe(t, svc, juridica.ID, sku, nil).RetISLR, 50, "el MISMO concepto a jurídica: 5%")
}

// TestOC_SustraendoNoDejaLaRetencionNegativa: si el sustraendo supera el cálculo,
// no se retiene nada — nunca se le "devuelve" ISLR al proveedor.
func TestOC_SustraendoNoDejaLaRetencionNegativa(t *testing.T) {
	svc := servicioConConceptos(t)
	sku := primerSKU(t, svc)

	// Un concepto con base mínima alta: por debajo de ella NO se retiene. Es regla
	// del maestro, y la proyección la respeta porque el cálculo lo hace el concepto.
	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, fiscal.ConceptoISLR{
		Codigo: "consultoria_menor", Nombre: "Consultoría de monto menor",
		Sujeto: fiscal.SujetoJuridicaDomiciliada, Porcentaje: 5, BaseMinima: 5000, Activo: true,
	}); err != nil {
		t.Fatalf("crear concepto con base mínima: %v", err)
	}
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre:      "Bajo el mínimo, C.A.",
		RetieneISLR: true, ConceptoISLRCodigo: "consultoria_menor", SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	})

	o := ocDe(t, svc, prov.ID, sku, nil) // base 1.000, por debajo del mínimo de 5.000
	casiEq(t, o.RetISLR, 0, "por debajo de la base mínima del concepto no se retiene")
	casiEq(t, o.Neto, 1160, "sin retención, el neto es el total")
}

// TestOC_SinPerfilNoRetieneNada: un proveedor sin perfil (los que ya existían antes
// de esta funcionalidad) no cambia de comportamiento.
func TestOC_SinPerfilNoRetieneNada(t *testing.T) {
	svc := servicioConConceptos(t)
	sku := primerSKU(t, svc)

	o := ocDe(t, svc, provDemo1, sku, nil)
	casiEq(t, o.RetIVA, 0, "sin perfil no se retiene IVA")
	casiEq(t, o.RetISLR, 0, "sin perfil no se retiene ISLR")
	casiEq(t, o.Neto, o.Total, "el neto a pagar es el total de la orden")
}

// TestOC_RetencionesSePuedenAjustarEnLaOrden: un mismo proveedor factura honorarios
// un mes y un flete al siguiente; quien arma la orden puede ajustar el concepto.
func TestOC_RetencionesSePuedenAjustarEnLaOrden(t *testing.T) {
	svc := servicioConConceptos(t)
	sku := primerSKU(t, svc)
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre:     "Mixta, C.A.",
		RetieneIVA: true, RetieneISLR: true,
		ConceptoISLRCodigo: "honorarios", SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	})

	// Esta compra es un flete: el maestro dice 3 % para jurídica, y sin IVA retenido.
	o := ocDe(t, svc, prov.ID, sku, &application.RetencionesOCEntrada{
		RetieneIVA:  false,
		RetieneISLR: true, ISLRConceptoCodigo: "fletes", ISLRSujeto: fiscal.SujetoJuridicaDomiciliada,
	})
	casiEq(t, o.RetIVA, 0, "el ajuste de la orden apaga la retención de IVA del perfil")
	casiEq(t, o.RetISLR, 30, "ISLR del flete, con la tarifa del maestro (1.000 × 3%)")
	if o.ConceptoISLR != "Fletes y transporte" {
		t.Errorf("la orden guarda el NOMBRE del concepto del maestro, se obtuvo %q", o.ConceptoISLR)
	}
	casiEq(t, o.Neto, 1130, "neto a pagar (1.160 − 30)")

	// La ficha no se toca: el perfil del proveedor sigue siendo el habitual.
	p, _ := svc.Proveedor(empDemo, prov.ID)
	if p.ConceptoISLRCodigo != "honorarios" {
		t.Errorf("ajustar una orden no puede reescribir el perfil del proveedor, quedó %q", p.ConceptoISLRCodigo)
	}
}

// TestOC_HeredaLaCondicionDePagoDelProveedor: se pacta una vez en la ficha y la
// orden la propone, pero guarda la suya (el maestro puede cambiar mañana).
func TestOC_HeredaLaCondicionDePagoDelProveedor(t *testing.T) {
	svc := servicioConConceptos(t)
	sku := primerSKU(t, svc)
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{Nombre: "A crédito, C.A.", CondicionPago: "30 días"})

	o := ocDe(t, svc, prov.ID, sku, nil)
	if o.Condiciones != "30 días" {
		t.Errorf("la orden debía heredar la condición del proveedor, se obtuvo %q", o.Condiciones)
	}

	// Lo indicado en la orden manda sobre el maestro.
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: prov.ID, SedeID: sede1, CondicionesPago: "Contado",
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 1, CostoUnitario: 10}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if oc.CondicionesPago != "Contado" {
		t.Errorf("la condición indicada en la orden debe ganar, se obtuvo %q", oc.CondicionesPago)
	}
}

// TestProveedor_PerfilDeRetencionSeValida: los datos que después sustentan un
// comprobante ante el SENIAT no se guardan a medias.
func TestProveedor_PerfilDeRetencionSeValida(t *testing.T) {
	svc := servicioConConceptos(t)

	// ISLR activo sin concepto: no hay forma de justificar el porcentaje.
	if _, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Sin concepto", RetieneISLR: true, SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	}); !errors.Is(err, application.ErrRetencionISLRSinConcepto) {
		t.Fatalf("ISLR sin concepto debía dar ErrRetencionISLRSinConcepto, se obtuvo: %v", err)
	}

	// Porcentaje fuera de rango.
	if _, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Porcentaje absurdo", RetieneIVA: true, RetencionIVAPorcentaje: 150,
	}); !errors.Is(err, application.ErrRetencionPorcentajeProveedor) {
		t.Fatalf("un porcentaje mayor a 100 debía dar ErrRetencionPorcentajeProveedor, se obtuvo: %v", err)
	}

	// Sin declarar qué es el proveedor no se sabe cuál de las dos tarifas aplica.
	if _, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Sin sujeto", RetieneISLR: true, ConceptoISLRCodigo: "servicios",
	}); !errors.Is(err, application.ErrSujetoISLRInvalido) {
		t.Fatalf("sin tipo de sujeto debía dar ErrSujetoISLRInvalido, se obtuvo: %v", err)
	}

	// Un concepto que no está en el maestro se descubre al guardar la ficha.
	if _, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Concepto inventado", RetieneISLR: true, ConceptoISLRCodigo: "no_existe_xyz",
		SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	}); !errors.Is(err, application.ErrConceptoISLRDelProveedorNoExiste) {
		t.Fatalf("un concepto fuera del maestro debía rechazarse, se obtuvo: %v", err)
	}

	// Apagar el impuesto deja de exigir el resto de los datos.
	if _, err := svc.CrearProveedor(empDemo, actorA, origenTst, proveedor.Proveedor{
		Nombre: "Sin retenciones", RetieneISLR: false,
	}); err != nil {
		t.Fatalf("un proveedor sin retenciones no debe exigir nada: %v", err)
	}
}
