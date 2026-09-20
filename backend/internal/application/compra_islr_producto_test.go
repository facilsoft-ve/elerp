package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

/* EL CONCEPTO DE ISLR LO DECLARA EL PRODUCTO.
 *
 * El ISLR se retiene por el CONCEPTO DEL PAGO —qué se está pagando—, no por a
 * quién. Por eso lo clasifica la ficha del producto y la orden retiene línea por
 * línea, agrupando por concepto; el del proveedor queda como respaldo para lo que
 * no está en el catálogo.
 *
 * El tipo de SUJETO sí es del proveedor: una empresa es jurídica siempre.
 *
 * Las órdenes de estos tests se arman a Bs 100 la unidad, así que 10 u = base
 * 1.000 por línea. */

// servicioConCatalogoISLR devuelve un servicio con el maestro cableado y un
// helper para dar de alta servicios clasificados.
func servicioConCatalogoISLR(t *testing.T) *application.Service {
	t.Helper()
	svc := servicioConConceptos(t)
	svc.ConceptosISLR(empDemo) // siembra el maestro antes de clasificar nada
	return svc
}

// servicioClasificado da de alta un producto de servicio con su concepto de ISLR.
func servicioClasificado(t *testing.T, svc *application.Service, sku, concepto string) string {
	t.Helper()
	p, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: sku, Nombre: "Servicio " + sku, Precio: 100, ConceptoISLR: concepto,
	})
	if err != nil {
		t.Fatalf("crear servicio %s: %v", sku, err)
	}
	return p.SKU
}

// ocConLineas arma una orden de 10 u a Bs 100 por cada SKU.
func ocConLineas(t *testing.T, svc *application.Service, provID string, skus ...string) compra.OrdenCompra {
	t.Helper()
	lineas := make([]application.LineaOCEntrada, 0, len(skus))
	for _, sku := range skus {
		lineas = append(lineas, application.LineaOCEntrada{SKU: sku, Cantidad: 10, CostoUnitario: 100})
	}
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provID, SedeID: sede1, Lineas: lineas,
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	return oc
}

// TestOC_ElConceptoLoPoneElProducto: lo que decide la tarifa es QUÉ se compra. El
// concepto del perfil del proveedor no se usa cuando la línea trae el suyo.
func TestOC_ElConceptoLoPoneElProducto(t *testing.T) {
	svc := servicioConCatalogoISLR(t)
	hon := servicioClasificado(t, svc, "SRV-HON", "honorarios")
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Consultores, C.A.", RetieneISLR: true,
		// El perfil dice «fletes», pero se está comprando una consultoría.
		ConceptoISLRCodigo: "fletes", SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	})

	oc := ocConLineas(t, svc, prov.ID, hon)
	if len(oc.RetencionISLRDetalle) != 1 {
		t.Fatalf("se esperaba un concepto en el desglose, hay %d", len(oc.RetencionISLRDetalle))
	}
	d := oc.RetencionISLRDetalle[0]
	if d.Codigo != "honorarios" {
		t.Fatalf("el concepto debía salir del producto (honorarios), salió %q", d.Codigo)
	}
	// Honorarios a jurídica domiciliada: 5 % en el maestro. Fletes serían 3 %.
	casiEq(t, d.Porcentaje, 5, "tarifa de honorarios a jurídica")
	casiEq(t, d.Base, 1000, "base del concepto")
	casiEq(t, oc.RetencionISLRMonto, 50, "retención proyectada (1.000 × 5%)")
}

// TestOC_LaMercanciaNoEntraEnLaBaseDeISLR: sobre la compra de un producto no se
// retiene ISLR. En una orden mixta, la base es solo la del servicio.
func TestOC_LaMercanciaNoEntraEnLaBaseDeISLR(t *testing.T) {
	svc := servicioConCatalogoISLR(t)
	hon := servicioClasificado(t, svc, "SRV-MIX", "honorarios")
	mercancia := primerSKU(t, svc) // del seed: sin concepto
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Suple y asesora, C.A.", RetieneISLR: true,
		ConceptoISLRCodigo: "honorarios", SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	})

	oc := ocConLineas(t, svc, prov.ID, hon, mercancia)
	casiEq(t, oc.Subtotal, 2000, "subtotal de las dos líneas")
	if len(oc.RetencionISLRDetalle) != 1 {
		t.Fatalf("la mercancía no debía crear un concepto: %+v", oc.RetencionISLRDetalle)
	}
	casiEq(t, oc.RetencionISLRDetalle[0].Base, 1000, "la base es la del servicio, no el subtotal")
	casiEq(t, oc.RetencionISLRMonto, 50, "retención sobre 1.000, no sobre 2.000")
}

// TestOC_VariosConceptosSeDesglosan: una orden puede mezclar conceptos y cada uno
// se retiene a su tarifa. El escalar de porcentaje queda en cero a propósito: no
// existe una tarifa única que describa la mezcla.
func TestOC_VariosConceptosSeDesglosan(t *testing.T) {
	svc := servicioConCatalogoISLR(t)
	hon := servicioClasificado(t, svc, "SRV-H2", "honorarios")
	flete := servicioClasificado(t, svc, "SRV-F2", "fletes")
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Todo en uno, C.A.", RetieneISLR: true,
		ConceptoISLRCodigo: "honorarios", SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	})

	oc := ocConLineas(t, svc, prov.ID, hon, flete)
	if len(oc.RetencionISLRDetalle) != 2 {
		t.Fatalf("se esperaban dos conceptos en el desglose, hay %d", len(oc.RetencionISLRDetalle))
	}
	// El desglose sale ordenado por código: fletes antes que honorarios.
	if oc.RetencionISLRDetalle[0].Codigo != "fletes" || oc.RetencionISLRDetalle[1].Codigo != "honorarios" {
		t.Fatalf("el desglose debía salir ordenado por código: %+v", oc.RetencionISLRDetalle)
	}
	// Fletes a jurídica 3 % sobre 1.000 = 30; honorarios 5 % sobre 1.000 = 50.
	casiEq(t, oc.RetencionISLRMonto, 80, "la retención total es la suma de los conceptos")
	casiEq(t, oc.RetencionISLRPorcentaje, 0, "con varios conceptos no hay un porcentaje único")
	if oc.RetencionISLRConcepto == "" {
		t.Fatal("con varios conceptos el escalar debería listar los nombres")
	}
	// IVA 16 % sobre 2.000 = 320; total 2.320; neto = total − 80.
	casiEq(t, oc.NetoAPagar, 2240, "neto a pagar")
}

// TestOC_ConceptoSinTarifaParaEseSujetoQuedaVisible: si el maestro no tiene la
// tarifa de ese concepto para el tipo de sujeto del proveedor, la fila se guarda
// con monto 0 y su marca. Omitirla haría que una tabla incompleta se viera igual
// que «a este proveedor no se le retiene».
func TestOC_ConceptoSinTarifaParaEseSujetoQuedaVisible(t *testing.T) {
	svc := servicioConCatalogoISLR(t)
	// El maestro por defecto solo trae fletes para jurídica domiciliada.
	flete := servicioClasificado(t, svc, "SRV-F3", "fletes")
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Fletero natural", RetieneISLR: true,
		ConceptoISLRCodigo: "servicios", SujetoISLR: fiscal.SujetoNaturalResidente,
	})

	oc := ocConLineas(t, svc, prov.ID, flete)
	if len(oc.RetencionISLRDetalle) != 1 {
		t.Fatalf("el concepto sin tarifa debía dejar constancia: %+v", oc.RetencionISLRDetalle)
	}
	d := oc.RetencionISLRDetalle[0]
	if !d.SinTarifa {
		t.Fatal("la fila debía venir marcada como sin tarifa para ese sujeto")
	}
	if d.Concepto == "" || d.Concepto == d.Codigo {
		t.Fatalf("la fila debía nombrar el concepto del maestro, trae %q", d.Concepto)
	}
	casiEq(t, d.Base, 1000, "la base se informa aunque no haya tarifa")
	casiEq(t, oc.RetencionISLRMonto, 0, "sin tarifa no se retiene nada")
}

// TestOC_SinConceptoEnLasLineasMandaElDelProveedor: el respaldo sigue vivo para
// servicios que no están en el catálogo y catálogos aún sin clasificar.
func TestOC_SinConceptoEnLasLineasMandaElDelProveedor(t *testing.T) {
	svc := servicioConCatalogoISLR(t)
	mercancia := primerSKU(t, svc)
	prov := proveedorConPerfil(t, svc, proveedor.Proveedor{
		Nombre: "Sin clasificar, C.A.", RetieneISLR: true,
		ConceptoISLRCodigo: "servicios", SujetoISLR: fiscal.SujetoJuridicaDomiciliada,
	})

	oc := ocConLineas(t, svc, prov.ID, mercancia)
	if len(oc.RetencionISLRDetalle) != 1 || oc.RetencionISLRDetalle[0].Codigo != "servicios" {
		t.Fatalf("sin concepto en las líneas debía mandar el del proveedor: %+v", oc.RetencionISLRDetalle)
	}
	// Servicios a jurídica domiciliada: 2 % en el maestro.
	casiEq(t, oc.RetencionISLRMonto, 20, "retención del concepto de respaldo")
}

// TestProducto_ConceptoFueraDelMaestroSeRechaza: un producto clasificado con un
// concepto inexistente no retendría nada, y esa ausencia se ve igual que «no es un
// servicio». Se rechaza al guardar la ficha.
func TestProducto_ConceptoFueraDelMaestroSeRechaza(t *testing.T) {
	svc := servicioConCatalogoISLR(t)
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "SRV-MAL", Nombre: "Servicio raro", Precio: 100, ConceptoISLR: "no_existe_xyz",
	}); !errors.Is(err, application.ErrConceptoISLRDesconocido) {
		t.Fatalf("un concepto fuera del maestro debía rechazarse, se obtuvo: %v", err)
	}
	// Vacío es válido: es el caso de toda mercancía.
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: "MRC-OK", Nombre: "Mercancía", Precio: 100,
	}); err != nil {
		t.Fatalf("un producto sin concepto no debe exigir nada: %v", err)
	}
}

// TestConcepto_NoSeDuplicaElParCodigoSujeto: el par (código, sujeto) es la clave
// con la que se resuelve la tarifa. Dos filas vivas del mismo par no dan error en
// ningún lado: solo hacen que gane una de las dos sin decir cuál.
func TestConcepto_NoSeDuplicaElParCodigoSujeto(t *testing.T) {
	svc := servicioConCatalogoISLR(t)

	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, fiscal.ConceptoISLR{
		Codigo: "honorarios", Nombre: "Honorarios (otra tarifa)",
		Sujeto: fiscal.SujetoJuridicaDomiciliada, Porcentaje: 9, Activo: true,
	}); !errors.Is(err, application.ErrConceptoDuplicado) {
		t.Fatalf("duplicar el par debía dar ErrConceptoDuplicado, se obtuvo: %v", err)
	}

	// El MISMO código con otro sujeto sí es válido: es justo para lo que existe la
	// tabla (una tarifa por tipo de sujeto).
	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, fiscal.ConceptoISLR{
		Codigo: "fletes", Nombre: "Fletes y transporte",
		Sujeto: fiscal.SujetoNaturalResidente, Porcentaje: 3, Activo: true,
	}); err != nil {
		t.Fatalf("el mismo código con otro sujeto es válido: %v", err)
	}

	// Editar una fila existente no choca consigo misma.
	honorarios, ok := fiscal.ConceptoPara(svc.ConceptosISLR(empDemo), "honorarios", fiscal.SujetoJuridicaDomiciliada)
	if !ok {
		t.Fatal("el maestro sembrado debería traer honorarios para jurídica")
	}
	honorarios.Porcentaje = 6
	if _, err := svc.GuardarConceptoISLR(empDemo, actorA, origenTst, honorarios); err != nil {
		t.Fatalf("editar la propia fila no debe chocar consigo misma: %v", err)
	}
}
