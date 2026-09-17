package application

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* NOTAS DE CRÉDITO POR DESCUENTO Y POR AJUSTE DE PRECIO.
 *
 * Notas de la contadora (15:18 y 15:19): «se debe poder generar una nota de
 * crédito por DESCUENTO» y «por AJUSTE DE PRECIO INDIVIDUAL DE PRODUCTO».
 *
 * Hasta acá la NC solo sabía hacer una cosa: devolver mercancía por cantidad. Y
 * como reingresaba stock siempre, usarla para un descuento habría INFLADO EL
 * INVENTARIO en silencio — nadie devolvió nada, solo se cobró de más. Por eso el
 * motivo no es decorativo: es lo que decide si la nota toca el inventario
 * (ver fiscal.NCDevuelveMercancia).
 */

var (
	ErrMotivoNCInvalido  = errors.New("el motivo de la nota de crédito no es válido")
	ErrNCDescuentoMonto  = errors.New("el descuento debe ser mayor que cero")
	ErrNCDescuentoExcede = errors.New("el descuento no puede superar el monto de la factura")
	ErrNCAjusteVacio     = errors.New("hay que indicar al menos un producto y su precio correcto")
	ErrNCAjustePrecio    = errors.New("el precio correcto debe ser menor que el facturado: para cobrar de más va una nota de débito")
	ErrNCPorcentaje      = errors.New("el porcentaje debe ir entre 0 y 100")
)

// AjustePrecioLinea es la corrección del precio de UN renglón de la factura: el
// precio que debió haberse facturado. La diferencia contra el precio facturado,
// por la cantidad del renglón, es lo que se acredita.
type AjustePrecioLinea struct {
	SKU string
	// PrecioCorrecto es el unitario que correspondía. Tiene que ser MENOR que el
	// facturado: acreditar hacia arriba no existe — eso es una nota de débito.
	PrecioCorrecto float64
}

// DescuentoNC describe un descuento: por MONTO fijo o por PORCENTAJE, y sobre
// toda la factura o sobre UN producto.
//
// El porcentaje existe porque así se pacta un descuento en la vida real («10 %
// por pronto pago»), y calcularlo a mano para escribir el monto es justo donde
// se cuela un error de céntimos que después no cuadra con la factura.
type DescuentoNC struct {
	// Monto fijo, o Porcentaje (0–100). Se usa UNO de los dos.
	Monto      float64
	Porcentaje float64
	// SKU vacío = sobre toda la factura. Con SKU = sobre ese renglón.
	SKU string
	// Exento solo aplica al descuento por MONTO sobre toda la factura, donde no
	// hay de dónde deducir la condición. En los demás casos se hereda de la
	// factura o del renglón, que es el dato correcto.
	Exento bool
	Nota   string
}

// baseDelDescuento resuelve sobre qué monto se aplica el porcentaje y cómo se
// reparte entre gravado y exento.
//
// EL PORCENTAJE VA SOBRE LA BASE, no sobre el total con IVA. Da lo mismo en
// plata —el IVA es proporcional, así que 10 % de la base baja el total un 10 %—
// pero aplicarlo sobre el total y volver a calcular IVA encima descontaría dos
// veces el impuesto.
//
// Sobre toda la factura con bases mixtas, el descuento se reparte EN PROPORCIÓN
// entre lo gravado y lo exento: cargarlo todo a una sola base cambiaría el IVA
// de la nota y descuadraría el libro.
func (s *Service) baseDelDescuento(orig fiscal.Documento, d DescuentoNC) (gravado, exento float64, err error) {
	g, e, tope, err := repartirBase(orig, d.SKU, d.Monto, d.Porcentaje, d.Exento)
	if err != nil {
		return 0, 0, err
	}
	// Tope SOLO en la nota de crédito: no se puede acreditar más de lo que se
	// cobró. La nota de débito no lo tiene, porque un cargo posterior (mora,
	// diferencial cambiario) sí puede superar el renglón que lo originó.
	if tope > 0 && round2(g+e) > round2(tope)+0.005 {
		return 0, 0, fmt.Errorf("%w: %s (máximo %.2f)", ErrNCDescuentoExcede, d.SKU, tope)
	}
	return g, e, nil
}

// repartirBase resuelve sobre qué monto se aplica un ajuste y cómo se reparte
// entre base gravada y exenta. Lo comparten la nota de crédito y la de débito:
// la aritmética del «% sobre el total» o del «% sobre este producto» es la misma
// en las dos; lo único que cambia es el signo y si hay tope.
//
// `tope` devuelve el total del renglón cuando el ajuste va sobre un producto (0
// cuando va sobre toda la factura), para que quien llama decida si acota.
func repartirBase(orig fiscal.Documento, sku string, monto, porcentaje float64, exentoDeclarado bool) (gravado, exento, tope float64, err error) {
	sku = strings.TrimSpace(sku)
	if sku != "" {
		var ol *fiscal.Linea
		for i := range orig.Lineas {
			if orig.Lineas[i].SKU == sku {
				ol = &orig.Lineas[i]
				break
			}
		}
		if ol == nil {
			return 0, 0, 0, fmt.Errorf("%w: %s", ErrNotaCreditoSKU, sku)
		}
		// El renglón de una NC guarda montos negativos; el porcentaje se aplica
		// sobre la MAGNITUD para no invertir el signo del ajuste.
		base := math.Abs(ol.Total)
		if porcentaje > 0 {
			monto = base * porcentaje / 100
		}
		monto = round2(monto)
		if ol.Exento {
			return 0, monto, base, nil
		}
		return monto, 0, base, nil
	}
	// Sobre toda la factura: el porcentaje se reparte EN PROPORCIÓN entre lo
	// gravado y lo exento. Cargarlo todo a una sola base cambiaría el IVA del
	// ajuste y descuadraría el libro.
	if porcentaje > 0 {
		return round2(orig.BaseImponible * porcentaje / 100), round2(orig.BaseExenta * porcentaje / 100), 0, nil
	}
	// Monto fijo sin producto: la condición la declara quien emite, porque no hay
	// de dónde deducirla.
	if exentoDeclarado {
		return 0, round2(monto), 0, nil
	}
	return round2(monto), 0, 0, nil
}

// NotaCreditoDescuento emite una NC que baja el monto de la factura sin devolver
// mercancía. El descuento entra como renglones sin SKU: no hay producto que
// devolver, y ponerle uno haría que el guard anti-sobre-crédito de las
// devoluciones contara una cantidad que nadie devolvió.
func (s *Service) NotaCreditoDescuento(empresaID, actor, origen, refID string, monto float64, exento bool, nota string) (fiscal.Documento, error) {
	return s.NotaCreditoDescuentoDe(empresaID, actor, origen, refID,
		DescuentoNC{Monto: monto, Exento: exento, Nota: nota})
}

// NotaCreditoDescuentoDe es la variante completa: monto o porcentaje, sobre la
// factura o sobre un producto.
func (s *Service) NotaCreditoDescuentoDe(empresaID, actor, origen, refID string, d DescuentoNC) (fiscal.Documento, error) {
	orig, err := s.facturaAcreditable(empresaID, refID)
	if err != nil {
		return fiscal.Documento{}, err
	}
	if d.Porcentaje < 0 || d.Porcentaje > 100 {
		return fiscal.Documento{}, ErrNCPorcentaje
	}
	if (d.Monto > 0) == (d.Porcentaje > 0) {
		// Los dos o ninguno: no se puede adivinar cuál quiso decir, y elegir por
		// él daría un descuento distinto del que pactó.
		return fiscal.Documento{}, ErrNCDescuentoMonto
	}
	gravado, exentoMonto, err := s.baseDelDescuento(orig, d)
	if err != nil {
		return fiscal.Documento{}, err
	}
	monto := round2(gravado + exentoMonto)
	if monto <= 0 {
		return fiscal.Documento{}, ErrNCDescuentoMonto
	}
	// No se puede acreditar más de lo facturado, contando lo ya acreditado por
	// otras notas: si no, la factura terminaría en negativo.
	if disponible := s.montoAcreditableRestante(empresaID, orig); monto > disponible+0.005 {
		return fiscal.Documento{}, fmt.Errorf("%w (disponible %.2f)", ErrNCDescuentoExcede, disponible)
	}
	texto := strings.TrimSpace(d.Nota)
	if texto == "" {
		texto = fiscal.NombreMotivo(fiscal.MotivosNotaCredito(), fiscal.MotivoNCDescuento)
	}
	// El rótulo dice CÓMO se calculó: quien lee la nota dentro de un año tiene
	// que poder reconstruir el descuento sin adivinar.
	sobre := "sobre " + orig.NumeroCompleto
	if d.SKU != "" {
		sobre = "sobre " + d.SKU
	}
	etiqueta := "Descuento " + sobre
	if d.Porcentaje > 0 {
		etiqueta = fmt.Sprintf("Descuento %.2f%% %s", d.Porcentaje, sobre)
	}
	lineas := []fiscal.Linea{}
	if gravado > 0 {
		lineas = append(lineas, fiscal.Linea{Nombre: etiqueta, Cantidad: 1, PrecioUnitario: gravado, Total: gravado})
	}
	if exentoMonto > 0 {
		lineas = append(lineas, fiscal.Linea{
			Nombre: etiqueta + " (exento)", Cantidad: 1,
			PrecioUnitario: exentoMonto, Total: exentoMonto, Exento: true,
		})
	}
	return s.emitirNotaCreditoSinMercancia(empresaID, actor, origen, orig,
		fiscal.MotivoNCDescuento, texto, lineas)
}

// NotaCreditoAjustePrecio emite una NC por haber facturado a un precio MAYOR que
// el correcto. Acredita, por producto, la diferencia multiplicada por la
// cantidad facturada. No devuelve mercancía: lo que estuvo mal fue el precio.
func (s *Service) NotaCreditoAjustePrecio(empresaID, actor, origen, refID string, ajustes []AjustePrecioLinea, nota string) (fiscal.Documento, error) {
	orig, err := s.facturaAcreditable(empresaID, refID)
	if err != nil {
		return fiscal.Documento{}, err
	}
	porSKU := map[string]fiscal.Linea{}
	for _, l := range orig.Lineas {
		porSKU[l.SKU] = l
	}
	lineas := []fiscal.Linea{}
	for _, a := range ajustes {
		ol, ok := porSKU[strings.TrimSpace(a.SKU)]
		if !ok {
			return fiscal.Documento{}, fmt.Errorf("%w: %s", ErrNotaCreditoSKU, a.SKU)
		}
		if a.PrecioCorrecto < 0 || a.PrecioCorrecto >= ol.PrecioUnitario {
			return fiscal.Documento{}, fmt.Errorf("%w (%s: facturado %.2f)", ErrNCAjustePrecio, ol.SKU, ol.PrecioUnitario)
		}
		diferencia := round2((ol.PrecioUnitario - a.PrecioCorrecto) * ol.Cantidad)
		if diferencia <= 0 {
			continue
		}
		// SIN SKU a propósito: el renglón representa una diferencia de precio, no
		// una devolución. Con SKU, el guard anti-sobre-crédito de las devoluciones
		// contaría esta cantidad y bloquearía una devolución real posterior.
		lineas = append(lineas, fiscal.Linea{
			Nombre:   fmt.Sprintf("Ajuste de precio · %s (%.2f → %.2f)", ol.Nombre, ol.PrecioUnitario, a.PrecioCorrecto),
			Cantidad: 1, PrecioUnitario: diferencia, Total: diferencia,
			// La exención la HEREDA del renglón original: la diferencia de precio de
			// un producto exento tampoco causa IVA.
			Exento: ol.Exento,
		})
	}
	if len(lineas) == 0 {
		return fiscal.Documento{}, ErrNCAjusteVacio
	}
	texto := strings.TrimSpace(nota)
	if texto == "" {
		texto = fiscal.NombreMotivo(fiscal.MotivosNotaCredito(), fiscal.MotivoNCAjustePrecio)
	}
	return s.emitirNotaCreditoSinMercancia(empresaID, actor, origen, orig,
		fiscal.MotivoNCAjustePrecio, texto, lineas)
}

// MotivosDeNota expone los catálogos a la interfaz, para que la lista no se
// escriba a mano en la pantalla y se desincronice del servidor.
func (s *Service) MotivosDeNota() (credito, debito []fiscal.MotivoNota) {
	return fiscal.MotivosNotaCredito(), fiscal.MotivosNotaDebito()
}

/* --- Piezas compartidas ---------------------------------------------------- */

// facturaAcreditable valida que el documento exista, sea una factura y no esté
// anulada. Misma derivación de «anulado» que usa el resto (existe su reversa).
func (s *Service) facturaAcreditable(empresaID, refID string) (fiscal.Documento, error) {
	orig, ok := s.documentos.ByID(empresaID, refID)
	if !ok || orig.Tipo != fiscal.TipoFactura {
		return fiscal.Documento{}, ErrDocumentoNoExiste
	}
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID == refID {
			return fiscal.Documento{}, ErrYaAnulado
		}
	}
	return orig, nil
}

// montoAcreditableRestante es cuánto queda por acreditar de una factura: su
// total menos lo que ya acreditaron otras notas de crédito. Sin este tope, dos
// descuentos sucesivos podrían dejar la factura en negativo.
func (s *Service) montoAcreditableRestante(empresaID string, orig fiscal.Documento) float64 {
	acreditado := 0.0
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoNotaCredito && d.RefDocumentoID == orig.ID {
			acreditado += -d.Total // las notas se guardan en negativo
		}
	}
	return round2(orig.Total - acreditado)
}

// emitirNotaCreditoSinMercancia arma, numera y asienta una NC que NO devuelve
// stock. Comparte todo con la devolución salvo lo único que importa acá: no
// emite movimientos de inventario y su costo reingresado es cero.
func (s *Service) emitirNotaCreditoSinMercancia(empresaID, actor, origen string, orig fiscal.Documento,
	motivoCodigo, motivoTexto string, lineas []fiscal.Linea) (fiscal.Documento, error) {
	if !fiscal.MotivoNCValido(motivoCodigo) {
		return fiscal.Documento{}, ErrMotivoNCInvalido
	}
	sedeID := orig.SedeID
	nc := fiscal.Documento{
		EmpresaID: empresaID, SedeID: sedeID, Tipo: fiscal.TipoNotaCredito, Modalidad: orig.Modalidad,
		ClienteID: orig.ClienteID, ClienteNombre: orig.ClienteNombre, ClienteDocumento: orig.ClienteDocumento,
		ClienteDireccion: orig.ClienteDireccion,
		// Tasa y alícuotas HEREDADAS del original: la nota acredita la misma
		// operación y tiene que cuadrar con ella. Usar las de hoy descuadraría el
		// asiento (misma regla que la devolución).
		Moneda: orig.Moneda, TasaCambio: orig.TasaCambio, TasaFuente: orig.TasaFuente,
		AlicuotaIVA: orig.AlicuotaIVA, AlicuotaIGTF: orig.AlicuotaIGTF,
		RefDocumentoID: orig.ID, Motivo: motivoTexto, MotivoCodigo: motivoCodigo,
		Lineas: lineas, Actor: actor, Fecha: ahora(),
	}
	var subtotal, baseImponible, baseExenta float64
	for _, l := range lineas {
		subtotal += l.Total
		if l.Exento {
			baseExenta += l.Total
		} else {
			baseImponible += l.Total
		}
	}
	tasaIVA := orig.AlicuotaIVA
	if tasaIVA <= 0 {
		tasaIVA = fiscal.AlicuotaIVA
	}
	// Montos NEGATIVOS (la nota resta). Se redondea en positivo y se niega al
	// final: round2 trunca hacia cero en negativo y dejaría un céntimo de
	// descuadre entre total y subtotal. Misma convención que la devolución.
	ivaPos := round2(baseImponible * tasaIVA)
	nc.Subtotal = -round2(subtotal)
	nc.BaseImponible = -round2(baseImponible)
	nc.BaseExenta = -round2(baseExenta)
	nc.IVA = -ivaPos
	nc.Total = -round2(subtotal + ivaPos)

	nc.Serie = serieDe(orig.Modalidad) + "-NC"
	nc.Numero = s.numerador.Siguiente(empresaID, sedeID, nc.Serie)
	nc.NumeroCompleto = fmt.Sprintf("%s-%08d", nc.Serie, nc.Numero)
	nc.NumeroControl = s.numeroControl(empresaID, orig.Modalidad)
	out := s.documentos.Append(nc)

	// SIN movimientos de inventario y con costo reingresado CERO: acá no volvió
	// ninguna mercancía. Este es el punto entero de distinguir el motivo.
	creditado := -out.Total
	cobradoProrrateado := creditado
	if orig.Total > 0 {
		cobradoOrig := orig.Cobrado
		if cobradoOrig > orig.Total {
			cobradoOrig = orig.Total
		}
		cobradoProrrateado = round2(creditado * cobradoOrig / orig.Total)
	}
	base := fiscal.Documento{
		NumeroCompleto: orig.NumeroCompleto,
		BaseImponible:  -out.BaseImponible, BaseExenta: -out.BaseExenta,
		IVA: -out.IVA, IGTF: -out.IGTF, Total: creditado, Cobrado: cobradoProrrateado,
	}
	s.asentarReversaFiscal(empresaID, actor, out, base, 0)
	s.audit.Append(evento(empresaID, actor, origen, "fiscal.documento.nota_credito",
		orig.NumeroCompleto, motivoCodigo+" · "+motivoTexto))
	return out, nil
}
