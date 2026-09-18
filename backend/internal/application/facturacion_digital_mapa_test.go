package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/facturaciondigital"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* MAPEO A LA IMPRENTA DIGITAL.
 *
 * La API NO calcula los montos: los VALIDA y responde 400 cuando no cuadran. Un
 * error acá no se ve en pantalla — se ve como facturas rechazadas en producción,
 * con el correlativo ya quemado. Por eso se prueba la aritmética, no que el JSON
 * tenga las claves. */

var cfgDigital = facturaciondigital.Config{
	SerieStrongID: "serie-uuid", SucursalStrongID: "sucursal-uuid",
}

func mapear(t *testing.T, doc fiscal.Documento) map[string]any {
	t.Helper()
	m, err := application.CuerpoImprenta(doc, cfgDigital, 7, "doc_local_1", "")
	if err != nil {
		t.Fatalf("mapear: %v", err)
	}
	return m
}

func num(t *testing.T, m map[string]any, clave string) float64 {
	t.Helper()
	v, ok := m[clave]
	if !ok {
		t.Fatalf("falta el campo %q", clave)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("%q no es numérico: %T", clave, v)
	}
	return f
}

// Factura simple gravada: lo que sale todos los días por el mostrador.
func TestImprenta_FacturaGravada(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T14:30:00Z", Moneda: "VES",
		ClienteNombre: "Bodega La Esquina", ClienteDocumento: "J-31122334-7",
		BaseImponible: 100, IVA: 16, Subtotal: 100, Total: 116, AlicuotaIVA: 0.16,
		Lineas: []fiscal.Linea{{Nombre: "Harina", Cantidad: 2, PrecioUnitario: 50, Total: 100, Alicuota: 0.16}},
	}
	m := mapear(t, doc)

	if m["DocumentType"] != "FA" {
		t.Fatalf("DocumentType = %v, se esperaba FA", m["DocumentType"])
	}
	if num(t, m, "TaxBase") != 100 || num(t, m, "TaxAmount") != 16 || num(t, m, "GrandTotal") != 116 {
		t.Fatalf("montos mal mapeados: %v", m)
	}
	// El RIF va SIN letra ni guiones —incluido el dígito verificador, como en el
	// ejemplo de su documentación (J-29718274-8 → "297182748")— y la letra aparte.
	if m["FiscalRegistryCode"] != "J" || m["FiscalRegistry"] != "311223347" {
		t.Fatalf("RIF mal partido: %v / %v", m["FiscalRegistryCode"], m["FiscalRegistry"])
	}
	// Nuestro id viaja como SystemReference: es la clave de idempotencia.
	if m["SystemReference"] != "doc_local_1" {
		t.Fatalf("SystemReference = %v", m["SystemReference"])
	}
}

// LA prueba de la hora: el servidor de la imprenta opera en UTC-04:00 y una
// fecha fuera de rango es rechazo seguro.
func TestImprenta_FechaEnHoraDeVenezuela(t *testing.T) {
	doc := fiscal.Documento{Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T14:30:00Z", Moneda: "VES"}
	m := mapear(t, doc)
	f, _ := m["EmissionDateAndTime"].(string)
	// 14:30 UTC son las 10:30 en Venezuela.
	if f != "2026-09-18T10:30:00.000-04:00" {
		t.Fatalf("EmissionDateAndTime = %q, se esperaba la hora de Venezuela con su desfase explícito", f)
	}
}

// Sin cliente identificado la factura es a consumidor final: es un caso
// legítimo del mostrador, no un dato faltante que deba romper la emisión.
func TestImprenta_ConsumidorFinal(t *testing.T) {
	m := mapear(t, fiscal.Documento{Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES"})
	if m["Name"] != "CONSUMIDOR FINAL" {
		t.Fatalf("Name = %v", m["Name"])
	}
	if m["FiscalRegistryCode"] != "V" {
		t.Fatalf("sin documento, la letra debe ser V: %v", m["FiscalRegistryCode"])
	}
}

// Las bases salen del DESGLOSE sellado al emitir, no de recalcular: una factura
// de hace un mes tiene que mandar la base de ese mes.
func TestImprenta_BasesPorAlicuota(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES",
		BaseImponible: 300, BaseExenta: 50, IVA: 40, Subtotal: 350, Total: 390,
		Impuestos: []fiscal.DocumentoImpuesto{
			{Tipo: fiscal.TipoGeneral, Porcentaje: 0.16, Base: 200, Monto: 32},
			{Tipo: fiscal.TipoReducida, Porcentaje: 0.08, Base: 100, Monto: 8},
		},
	}
	m := mapear(t, doc)
	if num(t, m, "TaxBase") != 200 || num(t, m, "TaxAmount") != 32 {
		t.Fatalf("base general mal: %v / %v", m["TaxBase"], m["TaxAmount"])
	}
	if num(t, m, "TaxBaseReduced") != 100 || num(t, m, "TaxAmountReduced") != 8 {
		t.Fatalf("base reducida mal: %v / %v", m["TaxBaseReduced"], m["TaxAmountReduced"])
	}
	if num(t, m, "TaxPercentReduced") != 8 {
		t.Fatalf("porcentaje reducido = %v, se esperaba 8", m["TaxPercentReduced"])
	}
	if num(t, m, "ExemptAmount") != 50 {
		t.Fatalf("base exenta = %v", m["ExemptAmount"])
	}
}

// El recargo suntuario va en su propia base y su propio monto, NO como una tasa
// del 31 %: es como lo declara el SENIAT y como lo separa nuestro libro.
func TestImprenta_RecargoSuntuarioVaAparte(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES",
		BaseImponible: 100, IVA: 31, Subtotal: 100, Total: 131,
		Impuestos: []fiscal.DocumentoImpuesto{
			{Tipo: fiscal.TipoGeneral, Porcentaje: 0.16, Base: 100, Monto: 16},
			{Tipo: fiscal.TipoAdicional, Porcentaje: 0.15, Base: 100, Monto: 15},
		},
	}
	m := mapear(t, doc)
	if num(t, m, "TaxBaseSumptuary") != 100 || num(t, m, "TaxAmountSumptuary") != 15 {
		t.Fatalf("el recargo debe ir en su propia columna: %v / %v", m["TaxBaseSumptuary"], m["TaxAmountSumptuary"])
	}
	if num(t, m, "TaxPercent") != 16 {
		t.Fatalf("la general sigue siendo 16, no 31: %v", m["TaxPercent"])
	}
}

// El IGTF solo se declara si se causó: mandar base cero con 3 % es declarar un
// impuesto que no ocurrió.
func TestImprenta_IGTFSoloSiLoHubo(t *testing.T) {
	sin := mapear(t, fiscal.Documento{Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00",
		Moneda: "VES", BaseImponible: 100, IVA: 16, Subtotal: 100, Total: 116})
	if _, hay := sin["IGTFAmount"]; hay {
		t.Fatal("sin IGTF causado no debe declararse el campo")
	}
	con := mapear(t, fiscal.Documento{Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00",
		Moneda: "VES", BaseImponible: 100, IVA: 16, Subtotal: 100, Total: 119.48,
		IGTF: 3.48, AlicuotaIGTF: 0.03})
	if num(t, con, "IGTFAmount") != 3.48 || num(t, con, "IGTFPercentage") != 3 {
		t.Fatalf("IGTF mal declarado: %v", con)
	}
	// Total va ANTES del IGTF; GrandTotal lo incluye.
	if num(t, con, "Total") != 116 || num(t, con, "GrandTotal") != 119.48 {
		t.Fatalf("Total/GrandTotal mal: %v / %v", con["Total"], con["GrandTotal"])
	}
}

// Una nota de crédito guarda sus montos en NEGATIVO en ElERP (es una reversa),
// pero la imprenta espera el documento en positivo: el que resta es el TIPO.
func TestImprenta_NotaCreditoViajaEnPositivo(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoNotaCredito, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES",
		BaseImponible: -100, IVA: -16, Subtotal: -100, Total: -116, AlicuotaIVA: 0.16,
		Lineas: []fiscal.Linea{{Nombre: "Harina", Cantidad: -1, PrecioUnitario: 100, Total: -100, Alicuota: 0.16}},
	}
	m := mapear(t, doc)
	if m["DocumentType"] != "NC" {
		t.Fatalf("DocumentType = %v, se esperaba NC", m["DocumentType"])
	}
	if num(t, m, "TaxBase") != 100 || num(t, m, "GrandTotal") != 116 {
		t.Fatalf("la NC debe viajar en positivo: %v", m)
	}
	det := m["Details"].([]map[string]any)
	if det[0]["Quantity"].(float64) != 1 {
		t.Fatalf("la cantidad de la NC debe ir en positivo: %v", det[0]["Quantity"])
	}
}

// Cada renglón lleva su código de alícuota: es lo que la imprenta cruza contra
// las bases declaradas arriba.
func TestImprenta_CodigoDeAlicuotaPorRenglon(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES",
		Lineas: []fiscal.Linea{
			{Nombre: "Gravado", Cantidad: 1, PrecioUnitario: 100, Total: 100, Alicuota: 0.16, AlicuotaCodigo: "general"},
			{Nombre: "Reducido", Cantidad: 1, PrecioUnitario: 100, Total: 100, Alicuota: 0.08, AlicuotaCodigo: "reducida"},
			{Nombre: "Lujo", Cantidad: 1, PrecioUnitario: 100, Total: 100, Alicuota: 0.16, AlicuotaCodigo: "suntuario"},
			{Nombre: "Exento", Cantidad: 1, PrecioUnitario: 100, Total: 100, Exento: true},
		},
	}
	m := mapear(t, doc)
	det := m["Details"].([]map[string]any)
	quiero := []string{"G", "R", "A", "E"}
	for i, q := range quiero {
		if det[i]["TaxCode"] != q {
			t.Errorf("renglón %d: TaxCode = %v, se esperaba %s", i, det[i]["TaxCode"], q)
		}
	}
	if det[3]["IsExempt"] != true || det[3]["TaxPercent"].(float64) != 0 {
		t.Fatalf("el renglón exento no debe causar impuesto: %v", det[3])
	}
}

// Factura en divisas: la conversión a bolívares se declara con la tasa
// HISTÓRICA del documento (Art. 177), no con la de hoy.
func TestImprenta_ConversionAVES(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "USD", TasaCambio: 40,
		BaseImponible: 100, IVA: 16, Subtotal: 100, Total: 116, AlicuotaIVA: 0.16,
	}
	m := mapear(t, doc)
	if num(t, m, "ExchangeRate") != 40 || num(t, m, "TaxBaseVES") != 4000 || num(t, m, "GrandTotalVES") != 4640 {
		t.Fatalf("conversión mal: %v", m)
	}
	// En una factura en bolívares los campos VES serían una copia sin información.
	enBs := mapear(t, fiscal.Documento{Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00",
		Moneda: "VES", TasaCambio: 40, BaseImponible: 100, IVA: 16, Total: 116})
	if _, hay := enBs["ExchangeRate"]; hay {
		t.Fatal("una factura en Bs no debe declarar conversión")
	}
}

// La anulación NO es un documento que se envíe: el original se anula por su
// número de control.
func TestImprenta_LaAnulacionNoSeEnviaComoDocumento(t *testing.T) {
	_, err := application.CuerpoImprenta(
		fiscal.Documento{Tipo: fiscal.TipoAnulacion, Fecha: "2026-09-18T10:00:00-04:00"},
		cfgDigital, 1, "x", "")
	if err == nil {
		t.Fatal("la anulación no se emite como documento nuevo")
	}
}
