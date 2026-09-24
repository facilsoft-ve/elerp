package application

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/facturaciondigital"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* MAPEO DE UN DOCUMENTO DE ElERP AL CUERPO QUE ESPERA LA IMPRENTA DIGITAL.
 *
 * Esta es la pieza que decide si la imprenta acepta o rechaza. La API NO calcula
 * los montos: los VALIDA contra sus reglas y responde 400 cuando no cuadran. Así
 * que acá no se inventa nada — se traduce lo que el documento ya selló al
 * emitirse, incluida su alícuota histórica y su tasa de cambio.
 *
 * TRES DECISIONES QUE EVITAN RECHAZOS:
 *
 *  1. LAS BASES SALEN DEL DESGLOSE, no de recalcular. `Documento.Impuestos` trae
 *     una fila por alícuota efectivamente aplicada. Recalcular con la tasa de hoy
 *     daría otro número para una factura de hace un mes, y la imprenta lo
 *     rechazaría con razón.
 *  2. LA HORA VA EN UTC-04:00 EXPLÍCITO. El servidor de la imprenta opera en hora
 *     de Venezuela y una fecha fuera de rango es rechazo seguro. No se depende de
 *     la zona del servidor donde corra ElERP.
 *  3. EL REDONDEO ES A DOS DECIMALES Y CONSISTENTE entre renglones y totales: si
 *     la suma de los renglones no da el total, rebota.
 *
 * El RECARGO SUNTUARIO (16 % + 15 % adicional) se declara como su propia base y
 * su propio monto, no como una tasa del 31 %: es como lo declara el SENIAT y como
 * lo separa nuestro propio libro de ventas.
 */

// La zona horaria de Venezuela ya está declarada en tasa.go (`zonaVE`): el
// servidor de la imprenta opera en UTC-04:00 y una fecha fuera de rango es
// rechazo seguro, así que se usa la misma y no una copia que pueda divergir.

// fechaEmisionVE formatea una fecha RFC3339 en la zona de Venezuela.
func fechaEmisionVE(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t = time.Now()
	}
	// El desfase va con "-07:00", el marcador de zona del layout de referencia de
	// Go. Escribirlo como "-04:00" parece más claro y está MAL: el "04" de la
	// referencia son los MINUTOS, así que producía "-30:00" y la imprenta habría
	// rechazado toda factura por fecha inválida.
	return t.In(zonaVE).Format("2006-01-02T15:04:05.000-07:00")
}

// r2 redondea a dos decimales, que es la precisión de un documento fiscal.
func r2(v float64) float64 { return math.Round(v*100) / 100 }

// tipoDocumentoDigital traduce el tipo de ElERP al `codeName` de la imprenta.
// Verificado contra POST /documenttypes/list.
func tipoDocumentoDigital(t string) (string, bool) {
	switch t {
	case fiscal.TipoFactura:
		return "FA", true
	case fiscal.TipoNotaCredito:
		return "NC", true
	case fiscal.TipoNotaDebito:
		return "ND", true
	}
	// La anulación NO se envía como documento: se anula el original por su número
	// de control (POST /documents/anulled). Ver anularEnImprenta.
	return "", false
}

// codigoAlicuota traduce nuestra clasificación al `TaxCode` de la imprenta.
// Verificado contra GET /companies/tax: G general, R reducida, E exento; el
// recargo de lujo tiene su propio código.
func codigoAlicuota(l fiscal.Linea) string {
	if l.Exento {
		return "E"
	}
	switch strings.ToLower(l.AlicuotaCodigo) {
	case fiscal.CodReducida:
		return "R"
	case fiscal.CodSuntuario:
		return "A"
	case fiscal.CodExento:
		return "E"
	}
	return "G"
}

// baseDe busca la base de un tipo de alícuota en el desglose del documento.
func baseDe(doc fiscal.Documento, tipo string) (base, monto, porcentaje float64) {
	for _, im := range doc.Impuestos {
		if im.Tipo == tipo {
			return im.Base, im.Monto, im.Porcentaje * 100
		}
	}
	return 0, 0, 0
}

// CuerpoImprenta arma el JSON del documento para `POST /documents/createandapprove`.
//
// `numero` es NUESTRO correlativo (la imprenta lo exige ascendente estricto por
// serie y tipo) y `referencia` es nuestro id de documento, que viaja como
// SystemReference y es la clave de idempotencia. `correo` es a dónde la imprenta
// manda el documento; vacío = no se declara, porque un campo en blanco no ayuda
// a nadie y puede hacer fallar la validación.
func CuerpoImprenta(doc fiscal.Documento, cfg facturaciondigital.Config, numero int, referencia, correo string) (map[string]any, error) {
	tipo, ok := tipoDocumentoDigital(doc.Tipo)
	if !ok {
		return nil, fmt.Errorf("el tipo de documento %q no se emite por imprenta digital", doc.Tipo)
	}

	// Las bases salen del desglose sellado al emitir. Para documentos anteriores
	// al maestro de impuestos el desglose viene vacío: ahí manda BaseImponible con
	// la alícuota del documento, que es la única verdad que tienen.
	baseGeneral, ivaGeneral, pctGeneral := baseDe(doc, fiscal.TipoGeneral)
	baseReducida, ivaReducida, pctReducida := baseDe(doc, fiscal.TipoReducida)
	baseAdicional, ivaAdicional, pctAdicional := baseDe(doc, fiscal.TipoAdicional)
	if len(doc.Impuestos) == 0 {
		baseGeneral, ivaGeneral = doc.BaseImponible, doc.IVA
		pctGeneral = doc.AlicuotaIVA * 100
	}
	if pctGeneral == 0 {
		pctGeneral = doc.AlicuotaIVA * 100
	}

	// El signo: una nota de crédito guarda sus montos en NEGATIVO en ElERP (es una
	// reversa), pero la imprenta espera el documento en positivo — el que resta es
	// el tipo, no el signo.
	abs := func(v float64) float64 { return math.Abs(r2(v)) }

	subtotal := abs(doc.Subtotal)
	total := abs(doc.Total) - abs(doc.IGTF)
	cuerpo := map[string]any{
		"SerieStrongId":       cfg.SerieStrongID,
		"SucursalStrongId":    cfg.SucursalStrongID,
		"DocumentType":        tipo,
		"Number":              numero,
		"EmissionDateAndTime": fechaEmisionVE(doc.Fecha),
		"SystemReference":     referencia,

		"Name":               nombreReceptor(doc),
		"FiscalRegistryCode": letraRIF(doc.ClienteDocumento),
		"FiscalRegistry":     numeroRIF(doc.ClienteDocumento),
		"Address":            strings.TrimSpace(doc.ClienteDireccion),
		"PaymentType":        condicionDePago(doc),

		"Currency":             monedaDe(doc),
		"ExemptAmount":         abs(doc.BaseExenta),
		"TaxBase":              abs(baseGeneral),
		"TaxBaseReduced":       abs(baseReducida),
		"TaxBaseSumptuary":     abs(baseAdicional),
		"Subtotal":             subtotal,
		"Discount":             0,
		"SubtotalPlusDiscount": subtotal,
		"TaxPercent":           r2(pctGeneral),
		// LAS ALÍCUOTAS VAN SIEMPRE CON SU VALOR DE LEY, aunque la base sea cero.
		// La imprenta valida el porcentaje contra la providencia y no contra la
		// base: mandar 0 % en la reducida porque no hubo renglones reducidos hace
		// que rechace la factura entera («no puede ser diferente de 8%»).
		"TaxPercentReduced":   pctReducidaDeLey(pctReducida),
		"TaxPercentSumptuary": pctAdicionalDeLey(pctAdicional),
		"TaxAmount":           abs(ivaGeneral),
		"TaxAmountReduced":    abs(ivaReducida),
		"TaxAmountSumptuary":  abs(ivaAdicional),
		"Taxes":               abs(doc.IVA),
		"Total":               r2(total),
		"GrandTotal":          abs(doc.Total),
	}

	// El IGTF solo se declara si lo hubo: mandar una base en cero con un 3 % es
	// declarar un impuesto que no se causó.
	// El PORCENTAJE del IGTF va siempre —la imprenta lo valida contra la ley igual
	// que las alícuotas—, pero la BASE y el MONTO solo si se causó: declarar una
	// base en cero con su 3 % es declarar un impuesto que no ocurrió.
	cuerpo["IGTFPercentage"] = pctIGTFDeLey(doc.AlicuotaIGTF * 100)
	hayIGTF := abs(doc.IGTF) > 0
	// LA BASE DEL IGTF ES LO PAGADO EN DIVISAS, NO EL TOTAL DE LA FACTURA.
	//
	// Antes acá iba el total, y la imprenta rechaza la factura entera: valida
	// base × 3 % == monto, y quien paga 20 US$ de una compra de 22.719 Bs causa
	// 510 y no 681,57. Solo se ve cuando el pago es MIXTO —si todo se paga en
	// divisas la base coincide con el total y el error queda tapado—, que es
	// justo el caso normal del mostrador.
	baseIGTF := abs(doc.BaseDelIGTF())
	if hayIGTF {
		cuerpo["IGTFBaseAmount"] = r2(baseIGTF)
		cuerpo["IGTFAmount"] = abs(doc.IGTF)
	}

	/* CONVERSIÓN A BOLÍVARES — obligatoria SIEMPRE, incluso en una factura que ya
	 * está en bolívares.
	 *
	 * Parece redundante y no lo es: la imprenta valida que los montos en moneda
	 * local coincidan con su conversión, y una factura en Bs sin los campos VES se
	 * rechaza entera («se requiere que TaxBase y TaxBaseVES tengan el mismo
	 * valor»). En ese caso la tasa es 1 y los valores se copian. */
	t := doc.TasaCambio
	if doc.Moneda == "" || doc.Moneda == "VES" || t <= 0 {
		t = 1
	}
	{
		cuerpo["ConversionCurrency"] = "VES"
		cuerpo["ExchangeRate"] = t
		cuerpo["ExemptAmountVES"] = r2(abs(doc.BaseExenta) * t)
		cuerpo["TaxBaseVES"] = r2(abs(baseGeneral) * t)
		cuerpo["TaxBaseReducedVES"] = r2(abs(baseReducida) * t)
		cuerpo["SubtotalVES"] = r2(subtotal * t)
		cuerpo["SubtotalPlusDiscountVES"] = r2(subtotal * t)
		cuerpo["TaxAmountVES"] = r2(abs(ivaGeneral) * t)
		cuerpo["TaxAmountReducedVES"] = r2(abs(ivaReducida) * t)
		cuerpo["TotalVES"] = r2(total * t)
		cuerpo["GrandTotalVES"] = r2(abs(doc.Total) * t)
		/* EL RECARGO SUNTUARIO Y EL IGTF TAMBIÉN SE ESPEJAN.
		 *
		 * La regla de la imprenta compara CADA campo con su conversión, no solo los
		 * totales: una factura con IGTF y sin IGTFAmountVES se rechaza entera. Se
		 * descubrió en producción, con una venta cobrada en divisas — la prueba que
		 * validó el espejo no tenía IGTF, así que el hueco no se veía.
		 *
		 * Van todos los que existen, no los que hoy hacen falta: el que se olvide
		 * vuelve a aparecer como un rechazo con la factura ya cobrada. */
		cuerpo["TaxBaseSumptuaryVES"] = r2(abs(baseAdicional) * t)
		cuerpo["TaxAmountSumptuaryVES"] = r2(abs(ivaAdicional) * t)
		cuerpo["DiscountVES"] = 0
		if hayIGTF {
			cuerpo["IGTFBaseAmountVES"] = r2(baseIGTF * t)
			cuerpo["IGTFAmountVES"] = r2(abs(doc.IGTF) * t)
		}
	}

	// Los renglones. `Amount` y `TotalAmount` se derivan del propio renglón para
	// que la suma cuadre con las bases declaradas arriba.
	detalles := make([]map[string]any, 0, len(doc.Lineas))
	for _, l := range doc.Lineas {
		monto := abs(l.Total)
		pct := l.Alicuota * 100
		if l.Exento {
			pct = 0
		} else if pct == 0 {
			pct = r2(pctGeneral)
		}
		ivaLinea := r2(monto * pct / 100)
		detalles = append(detalles, map[string]any{
			"Description":        nombreRenglon(l),
			"Quantity":           math.Abs(l.Cantidad),
			"UnitPrice":          abs(l.PrecioUnitario),
			"Amount":             monto,
			"AmountPlusDiscount": monto,
			"TaxAmount":          ivaLinea,
			"TaxPercent":         r2(pct),
			"TaxCode":            codigoAlicuota(l),
			"IsExempt":           l.Exento,
			"TotalAmount":        r2(monto + ivaLinea),
			"ProductType":        1,
			// OperationCode es obligatorio por renglón: sin él la imprenta rechaza
			// con «debe indicar el código de operación del producto facturado».
			// C001 es venta de bienes y servicios, que es lo que factura ElERP.
			"OperationCode": codigoOperacion,
		})
	}
	cuerpo["Details"] = detalles
	/* EL CORREO ES OBLIGATORIO para la imprenta, y la mayoría de las ventas de
	 * mostrador son a consumidor final sin correo. Por eso hay un respaldo: la
	 * dirección de la empresa, donde cae la copia cuando el cliente no dio la
	 * suya. Sin respaldo no se puede emitir, y eso es preferible a emitir a una
	 * dirección inventada. */
	c := strings.TrimSpace(correo)
	if c == "" {
		c = strings.TrimSpace(cfg.CorreoRespaldo)
	}
	if c == "" {
		return nil, errors.New("la imprenta exige un correo y el documento no trae uno: configura el correo de respaldo del módulo")
	}
	cuerpo["EmailTo"] = c

	/* EL RECEPTOR TIENE QUE ESTAR IDENTIFICADO, y esto es lo que más cambia la
	 * operación del mostrador: la imprenta exige cédula/RIF y dirección en TODA
	 * factura, también en la venta a consumidor final. Una bodega que hoy factura
	 * sin preguntar nada tiene que empezar a pedir la cédula.
	 *
	 * Se detecta acá, con nombre y apellido, en vez de dejar que la imprenta
	 * conteste «'Fiscal Registry' no debería estar vacío» — ese mensaje manda a
	 * buscar el problema en la integración y el problema está en el mostrador. */
	if falta := ReceptorIncompleto(doc); len(falta) > 0 {
		return nil, fmt.Errorf("la imprenta exige identificar al cliente: falta %s", strings.Join(falta, " y "))
	}

	return cuerpo, nil
}

/* ReceptorIncompleto enumera qué le falta al cliente del documento para que la
 * imprenta lo acepte. Vacío = está completo.
 *
 * Vive acá y no dentro del mapeo porque se consulta DOS veces: al mapear (por si
 * algo se coló) y ANTES de emitir, que es donde sirve de verdad — una vez
 * emitida la factura ya no se puede pedir la cédula. */
func ReceptorIncompleto(doc fiscal.Documento) []string {
	falta := []string{}
	if numeroRIF(doc.ClienteDocumento) == "" {
		falta = append(falta, "la cédula o el RIF")
	}
	if strings.TrimSpace(doc.ClienteDireccion) == "" {
		falta = append(falta, "la dirección")
	}
	return falta
}

/* LAS ALÍCUOTAS DE LEY.
 *
 * La imprenta valida cada porcentaje contra la providencia, no contra la base
 * del documento: una factura sin renglones reducidos igual tiene que declarar
 * que la alícuota reducida es 8 %. Si el documento trae su propio porcentaje se
 * respeta —una providencia futura puede cambiarlo y el histórico se emite con el
 * que estaba vigente—; si viene en cero, se usa el de ley.
 */
const (
	pctReducidaLey  = 8.0
	pctAdicionalLey = 31.0
	pctIGTFLey      = 3.0
	// codigoOperacion C001: venta de bienes y servicios. Es lo que factura ElERP.
	codigoOperacion = "C001"
)

func pctDeLey(pct, ley float64) float64 {
	if pct > 0 {
		return r2(pct)
	}
	return ley
}

func pctReducidaDeLey(pct float64) float64  { return pctDeLey(pct, pctReducidaLey) }
func pctAdicionalDeLey(pct float64) float64 { return pctDeLey(pct, pctAdicionalLey) }
func pctIGTFDeLey(pct float64) float64      { return pctDeLey(pct, pctIGTFLey) }

// nombreReceptor devuelve a quién se factura. Sin cliente identificado, la
// factura es a consumidor final, que es un caso legítimo y no un dato faltante.
func nombreReceptor(doc fiscal.Documento) string {
	if n := strings.TrimSpace(doc.ClienteNombre); n != "" {
		return n
	}
	return "CONSUMIDOR FINAL"
}

// letraRIF extrae la letra del documento (V, E, J, G, P). Sin documento se usa
// V, que es lo que corresponde a una persona natural no identificada.
func letraRIF(doc string) string {
	d := strings.ToUpper(strings.TrimSpace(doc))
	if d == "" {
		return "V"
	}
	switch d[0] {
	case 'V', 'E', 'J', 'G', 'P':
		return string(d[0])
	}
	return "V"
}

// numeroRIF deja solo los dígitos: la API los espera sin letra ni guiones.
func numeroRIF(doc string) string {
	var b strings.Builder
	for _, r := range doc {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func monedaDe(doc fiscal.Documento) string {
	if doc.Moneda != "" {
		return doc.Moneda
	}
	return "VES"
}

func nombreRenglon(l fiscal.Linea) string {
	n := strings.TrimSpace(l.Nombre)
	if n == "" {
		n = l.SKU
	}
	/* EL PESO VA EN LA DESCRIPCIÓN PORQUE LA IMPRENTA IMPRIME LA CANTIDAD SIN
	 * DECIMALES.
	 *
	 * Verificado contra el sandbox: se envió `Quantity: 0.35` y la imprenta lo
	 * guardó bien (`OriginalQuantity: 0.35`) pero lo IMPRIME como «0». En la
	 * factura de una charcutería eso deja un renglón que cobra 1.435 Bs por una
	 * cantidad de cero, que es justo lo que un cliente reclama.
	 *
	 * Su API tiene `UnitMeasureCode` por renglón y un catálogo en
	 * `GET /companies/unitMeasure`, pero el de esta cuenta trae una sola unidad
	 * («kg/m²»): hay que pedirle a UniDigital que cargue la tabla real antes de
	 * poder usarlo. Mientras tanto el dato viaja donde sí se lee entero.
	 */
	if l.CantidadFraccionada() {
		cant, precio := numeroVE(math.Abs(l.Cantidad), 3), numeroVE(math.Abs(l.PrecioUnitario), 2)
		if u := l.UnidadNombre(); u != "" {
			return fmt.Sprintf("%s — %s %s a %s por %s", n, cant, u, precio, u)
		}
		// Documento anterior al sellado de la unidad: se dice la cantidad, que es
		// el dato que faltaba, sin inventar en qué se mide.
		return fmt.Sprintf("%s — %s a %s c/u", n, cant, precio)
	}
	return n
}

// numeroVE formatea un número con la convención venezolana (punto de miles,
// coma decimal), que es como lo lee quien recibe la factura.
func numeroVE(v float64, decimales int) string {
	s := strconv.FormatFloat(v, 'f', decimales, 64)
	entero, resto := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		entero, resto = s[:i], s[i+1:]
	}
	var b strings.Builder
	for i, c := range entero {
		if i > 0 && (len(entero)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if resto != "" {
		b.WriteByte(',')
		b.WriteString(resto)
	}
	return b.String()
}

// condicionDePago es el texto libre que la imprenta imprime como condición.
func condicionDePago(doc fiscal.Documento) string {
	if doc.Credito {
		return "CRÉDITO"
	}
	return "CONTADO"
}
