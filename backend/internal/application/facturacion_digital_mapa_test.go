package application_test

import (
	"math"
	"strings"
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
	// La imprenta EXIGE destinatario: sin respaldo, ninguna venta a consumidor
	// final se podría emitir (verificado contra el sandbox el 22/09/2026).
	CorreoRespaldo: "facturas@mornix.tech",
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
		ClienteNombre: "Bodega La Esquina", ClienteDocumento: "J-31122334-7", ClienteDireccion: "Av. Bolívar, Caracas",
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
	doc := fiscal.Documento{Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T14:30:00Z", Moneda: "VES", ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas"}
	m := mapear(t, doc)
	f, _ := m["EmissionDateAndTime"].(string)
	// 14:30 UTC son las 10:30 en Venezuela.
	if f != "2026-09-18T10:30:00.000-04:00" {
		t.Fatalf("EmissionDateAndTime = %q, se esperaba la hora de Venezuela con su desfase explícito", f)
	}
}

// Sin NOMBRE la factura es a consumidor final: es un caso legítimo del
// mostrador. La cédula y la dirección, en cambio, sí hacen falta (ver
// TestImprenta_ElReceptorTieneQueEstarIdentificado).
func TestImprenta_ConsumidorFinal(t *testing.T) {
	m := mapear(t, fiscal.Documento{Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES", ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas"})
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
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES", ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas",
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
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES", ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas",
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
		Moneda: "VES", ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas", BaseImponible: 100, IVA: 16, Subtotal: 100, Total: 116})
	if _, hay := sin["IGTFAmount"]; hay {
		t.Fatal("sin IGTF causado no debe declararse el campo")
	}
	con := mapear(t, fiscal.Documento{Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00",
		Moneda: "VES", ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas", BaseImponible: 100, IVA: 16, Subtotal: 100, Total: 119.48,
		IGTF: 3.48, AlicuotaIGTF: 0.03})
	if num(t, con, "IGTFAmount") != 3.48 || num(t, con, "IGTFPercentage") != 3 {
		t.Fatalf("IGTF mal declarado: %v", con)
	}
	// Total va ANTES del IGTF; GrandTotal lo incluye.
	if num(t, con, "Total") != 116 || num(t, con, "GrandTotal") != 119.48 {
		t.Fatalf("Total/GrandTotal mal: %v / %v", con["Total"], con["GrandTotal"])
	}
}

/* LA BASE DEL IGTF ES LO PAGADO EN DIVISAS, NO EL TOTAL DE LA FACTURA.
 *
 * Acá iba el total del documento y el sandbox rechazó la factura entera:
 * «IGTFAmount = 510 no coincide con IGTFBaseAmount * 3% = 681,57». Solo se ve
 * con pago MIXTO —si todo se paga en divisas la base coincide con el total y el
 * error queda tapado—, que es justamente el caso normal del mostrador.
 */
func TestImprenta_LaBaseDelIGTFEsLoPagadoEnDivisas(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES",
		ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas",
		BaseImponible: 19585.5, IVA: 3133.68, Subtotal: 19585.5, Total: 23229.18,
		// 20 US$ a 850 de una compra de 22.719,18: el impuesto grava los 17.000.
		BaseIGTF: 17000, IGTF: 510, AlicuotaIGTF: 0.03,
	}
	m := mapear(t, doc)
	if got := num(t, m, "IGTFBaseAmount"); got != 17000 {
		t.Fatalf("la base del IGTF es lo pagado en divisas (17000), no el total: %v", got)
	}
	if got := num(t, m, "IGTFBaseAmountVES"); got != 17000 {
		t.Fatalf("el espejo en bolívares tiene que seguir a la base: %v", got)
	}
	// LA REGLA QUE VALIDA LA IMPRENTA, verificada acá antes de que la rechace ella.
	calculado := math.Round(num(t, m, "IGTFBaseAmount")*num(t, m, "IGTFPercentage")/100*100) / 100
	if calculado != num(t, m, "IGTFAmount") {
		t.Fatalf("base × %v%% tiene que dar el monto declarado: %v × %v ≠ %v",
			m["IGTFPercentage"], m["IGTFBaseAmount"], m["IGTFPercentage"], m["IGTFAmount"])
	}
}

// Los documentos anteriores al campo no traen la base grabada: se deriva del
// impuesto y su alícuota, que es exacto salvo el centavo del redondeo original.
func TestImprenta_LaBaseDelIGTFSeDerivaEnDocumentosViejos(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES",
		ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas",
		BaseImponible: 100, IVA: 16, Subtotal: 100, Total: 119.48,
		IGTF: 3.48, AlicuotaIGTF: 0.03, // sin BaseIGTF: documento viejo
	}
	if got := num(t, mapear(t, doc), "IGTFBaseAmount"); got != 116 {
		t.Fatalf("sin base grabada se deriva 3,48 / 3%% = 116, no %v", got)
	}
}

// Una nota de crédito guarda sus montos en NEGATIVO en ElERP (es una reversa),
// pero la imprenta espera el documento en positivo: el que resta es el TIPO.
func TestImprenta_NotaCreditoViajaEnPositivo(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoNotaCredito, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES", ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas",
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
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES", ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas",
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
		ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas",
		BaseImponible: 100, IVA: 16, Subtotal: 100, Total: 116, AlicuotaIVA: 0.16,
	}
	m := mapear(t, doc)
	if num(t, m, "ExchangeRate") != 40 || num(t, m, "TaxBaseVES") != 4000 || num(t, m, "GrandTotalVES") != 4640 {
		t.Fatalf("conversión mal: %v", m)
	}
	// Una factura YA en bolívares declara su conversión igual, con tasa 1. La tasa
	// del documento se ignora a propósito: convertir bolívares a bolívares por 40
	// multiplicaría la factura por cuarenta.
	enBs := mapear(t, fiscal.Documento{Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00",
		Moneda: "VES", ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas", TasaCambio: 40, BaseImponible: 100, IVA: 16, Total: 116})
	if num(t, enBs, "ExchangeRate") != 1 || num(t, enBs, "TaxBaseVES") != 100 || num(t, enBs, "GrandTotalVES") != 116 {
		t.Fatalf("la factura en Bs debe convertirse a sí misma: %v", enBs)
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

/* LO QUE EL SANDBOX DE UNIDIGITAL DESMINTIÓ (verificado el 22/09/2026).
 *
 * Estas cuatro reglas no se dedujeron de la guía: se descubrieron porque la
 * imprenta rechazó la factura con ellas en el cuerpo. Van con prueba porque son
 * exactamente el tipo de detalle que un refactor «limpia» por parecer redundante
 * —un porcentaje sobre una base en cero, una conversión de bolívares a
 * bolívares— y el precio de limpiarlo son facturas rechazadas en producción con
 * el correlativo ya quemado.
 */

// Las alícuotas se validan contra la ley y no contra la base del documento: una
// factura sin renglones reducidos igual declara que la reducida es 8 %.
func docSimple() fiscal.Documento {
	return fiscal.Documento{
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T14:30:00Z", Moneda: "VES",
		ClienteNombre: "Bodega La Esquina", ClienteDocumento: "J-31122334-7", ClienteDireccion: "Av. Bolívar, Caracas",
		BaseImponible: 100, IVA: 16, Subtotal: 100, Total: 116, AlicuotaIVA: 0.16,
		Lineas: []fiscal.Linea{{Nombre: "Harina", Cantidad: 2, PrecioUnitario: 50, Total: 100, Alicuota: 0.16}},
	}
}

func TestImprenta_LasAlicuotasVanConSuValorDeLey(t *testing.T) {
	m := mapear(t, docSimple())
	for clave, ley := range map[string]float64{
		"TaxPercentReduced":   8,
		"TaxPercentSumptuary": 31,
		"IGTFPercentage":      3,
	} {
		if v := num(t, m, clave); v != ley {
			t.Fatalf("%s = %v, la imprenta exige %v aunque la base sea cero", clave, v, ley)
		}
	}
}

// Una factura en bolívares TAMBIÉN lleva su conversión a bolívares. Parece
// redundante; sin ella la imprenta rechaza el documento entero.
func TestImprenta_LaFacturaEnBolivaresLlevaSuConversion(t *testing.T) {
	m := mapear(t, docSimple())
	if m["ConversionCurrency"] != "VES" {
		t.Fatalf("ConversionCurrency = %v", m["ConversionCurrency"])
	}
	if num(t, m, "ExchangeRate") != 1 {
		t.Fatalf("en una factura en Bs la tasa es 1, salió %v", num(t, m, "ExchangeRate"))
	}
	for base, espejo := range map[string]string{
		"TaxBase": "TaxBaseVES", "TaxAmount": "TaxAmountVES",
		"Total": "TotalVES", "GrandTotal": "GrandTotalVES",
	} {
		if num(t, m, base) != num(t, m, espejo) {
			t.Fatalf("%s (%v) y %s (%v) tienen que coincidir", base, num(t, m, base), espejo, num(t, m, espejo))
		}
	}
}

// Sin código de operación por renglón la imprenta rechaza: «debe indicar el
// código de operación del producto facturado».
func TestImprenta_CadaRenglonLlevaCodigoDeOperacion(t *testing.T) {
	m := mapear(t, docSimple())
	ds, _ := m["Details"].([]map[string]any)
	if len(ds) == 0 {
		t.Fatal("el documento salió sin renglones")
	}
	for i, d := range ds {
		if d["OperationCode"] != "C001" {
			t.Fatalf("renglón %d sin código de operación: %v", i, d["OperationCode"])
		}
	}
}

// El correo es obligatorio. Sin el del cliente se usa el de respaldo; sin
// ninguno de los dos NO se emite, que es preferible a emitir a una dirección
// inventada.
func TestImprenta_ElCorreoEsObligatorio(t *testing.T) {
	m := mapear(t, docSimple())
	if m["EmailTo"] != "facturas@mornix.tech" {
		t.Fatalf("sin correo del cliente debe usarse el de respaldo, salió %v", m["EmailTo"])
	}
	conCliente, err := application.CuerpoImprenta(docSimple(), cfgDigital, 7, "doc_local_1", "ana@ejemplo.com")
	if err != nil {
		t.Fatalf("mapear: %v", err)
	}
	if conCliente["EmailTo"] != "ana@ejemplo.com" {
		t.Fatalf("el correo del cliente manda sobre el respaldo, salió %v", conCliente["EmailTo"])
	}
	sinNada := cfgDigital
	sinNada.CorreoRespaldo = ""
	if _, err := application.CuerpoImprenta(docSimple(), sinNada, 7, "doc_local_1", ""); err == nil {
		t.Fatal("sin correo ni respaldo tenía que negarse a emitir")
	}
}

/* EL RECEPTOR TIENE QUE ESTAR IDENTIFICADO — la regla que más cambia la
 * operación del mostrador, descubierta emitiendo (22/09/2026).
 *
 * Una bodega que hoy factura sin preguntar nada tiene que empezar a pedir la
 * cédula. Se falla ACÁ, con nombre y apellido, en vez de dejar que la imprenta
 * conteste «'Fiscal Registry' no debería estar vacío»: ese mensaje manda a
 * buscar el problema en la integración, y el problema está en el mostrador.
 */
func TestImprenta_ElReceptorTieneQueEstarIdentificado(t *testing.T) {
	base := docSimple()

	sinNada := base
	sinNada.ClienteDocumento, sinNada.ClienteDireccion = "", ""
	if _, err := application.CuerpoImprenta(sinNada, cfgDigital, 7, "x", ""); err == nil {
		t.Fatal("sin cédula ni dirección la imprenta rechaza: hay que negarse antes de emitir")
	}

	sinCedula := base
	sinCedula.ClienteDocumento = ""
	if _, err := application.CuerpoImprenta(sinCedula, cfgDigital, 7, "x", ""); err == nil {
		t.Fatal("sin cédula ni RIF no se puede emitir")
	}

	sinDireccion := base
	sinDireccion.ClienteDireccion = ""
	if _, err := application.CuerpoImprenta(sinDireccion, cfgDigital, 7, "x", ""); err == nil {
		t.Fatal("sin dirección no se puede emitir")
	}

	// Y el mensaje tiene que decir QUÉ falta: «datos incompletos» obliga a
	// adivinar cuál, con el cliente esperando en el mostrador.
	if falta := application.ReceptorIncompleto(sinCedula); len(falta) != 1 {
		t.Fatalf("debería faltar exactamente una cosa: %v", falta)
	}
	if falta := application.ReceptorIncompleto(base); len(falta) != 0 {
		t.Fatalf("el documento completo no debería tener faltantes: %v", falta)
	}
}

/* CADA CAMPO TIENE SU ESPEJO EN BOLÍVARES, no solo los totales.
 *
 * La imprenta compara uno por uno. Este caso —una venta con IGTF— se rechazó en
 * producción con la factura ya cobrada, porque la prueba que validó el espejo no
 * tenía IGTF y el hueco no se veía. Va con recargo suntuario además, que es el
 * otro par que faltaba.
 */
func TestImprenta_ElEspejoEnBolivaresNoSeSaltaNingunCampo(t *testing.T) {
	doc := docSimple()
	doc.IGTF = 30
	doc.AlicuotaIGTF = 0.03
	doc.Total = 146
	doc.Impuestos = []fiscal.DocumentoImpuesto{
		{Tipo: fiscal.TipoGeneral, Porcentaje: 0.16, Base: 100, Monto: 16},
		{Tipo: fiscal.TipoAdicional, Porcentaje: 0.15, Base: 100, Monto: 15},
	}
	m := mapear(t, doc)

	// Todo campo X que la imprenta compara tiene que traer su XVES con el mismo
	// valor: la factura está en bolívares, así que la conversión es la identidad.
	for _, campo := range []string{
		"ExemptAmount", "TaxBase", "TaxBaseReduced", "TaxBaseSumptuary",
		"Subtotal", "SubtotalPlusDiscount", "TaxAmount", "TaxAmountReduced",
		"TaxAmountSumptuary", "Total", "GrandTotal", "IGTFBaseAmount", "IGTFAmount",
	} {
		v, hay := m[campo]
		if !hay {
			continue // el campo no aplica a este documento
		}
		espejo, hayEspejo := m[campo+"VES"]
		if !hayEspejo {
			t.Fatalf("%s viaja sin su espejo %sVES: la imprenta rechaza el documento entero", campo, campo)
		}
		if v != espejo {
			t.Fatalf("%s (%v) y %sVES (%v) tienen que coincidir en una factura en Bs", campo, v, campo, espejo)
		}
	}
}

/* EL GRANEL TIENE QUE DECIR SU PESO EN EL RENGLÓN.
 *
 * La imprenta recibe la cantidad bien (guardó `OriginalQuantity: 0.35`) pero la
 * IMPRIME sin decimales: el renglón salía cobrando 1.435,00 Bs por una cantidad
 * de «0». Hasta que su catálogo de unidades esté cargado y se pueda usar
 * `UnitMeasureCode`, el peso viaja en la descripción, que sí se imprime entera.
 */
func TestImprenta_ElGranelDiceSuPesoEnElRenglon(t *testing.T) {
	doc := fiscal.Documento{
		Tipo: fiscal.TipoFactura, Fecha: "2026-09-18T10:00:00-04:00", Moneda: "VES",
		ClienteDocumento: "V-12345678", ClienteDireccion: "Caracas",
		BaseImponible: 1435, IVA: 229.6, Subtotal: 1435, Total: 1664.6, AlicuotaIVA: 0.16,
		Lineas: []fiscal.Linea{
			{Nombre: "Queso blanco duro (granel)", Cantidad: 0.35, Unidad: "kg", PrecioUnitario: 4100, Total: 1435, Alicuota: 0.16},
			{Nombre: "Café molido premium 500g", Cantidad: 1, Unidad: "unidad", PrecioUnitario: 3350, Total: 3350, Alicuota: 0.16},
		},
	}
	det := mapear(t, doc)["Details"].([]map[string]any)
	granel, _ := det[0]["Description"].(string)
	if !strings.Contains(granel, "0,350") || !strings.Contains(granel, "kg") {
		t.Fatalf("el renglón a granel tiene que decir cuánto pesó: %q", granel)
	}
	if !strings.Contains(granel, "4.100,00") {
		t.Fatalf("y a cuánto el kilo, que es lo que el cliente verifica: %q", granel)
	}
	// EL RENGLÓN NORMAL NO SE TOCA: agregarle «1,000 unidad» a cada línea de una
	// factura de bodega la vuelve ilegible sin resolver nada.
	if entero, _ := det[1]["Description"].(string); entero != "Café molido premium 500g" {
		t.Fatalf("una cantidad entera no necesita explicación: %q", entero)
	}
}

// Un documento anterior al sellado de la unidad dice la cantidad pero no
// inventa la medida: «1,250 unidad» de jamón es peor que «1,250».
func TestImprenta_SinUnidadSelladaNoSeInventaLaMedida(t *testing.T) {
	doc := docSimple()
	doc.Lineas = []fiscal.Linea{{Nombre: "Jamón de pierna", Cantidad: 1.25, PrecioUnitario: 6710, Total: 8387.5, Alicuota: 0.16}}
	d, _ := mapear(t, doc)["Details"].([]map[string]any)[0]["Description"].(string)
	if !strings.Contains(d, "1,250") {
		t.Fatalf("la cantidad fraccionada tiene que verse: %q", d)
	}
	if strings.Contains(d, "unidad") {
		t.Fatalf("sin unidad sellada no se inventa una: %q", d)
	}
}
