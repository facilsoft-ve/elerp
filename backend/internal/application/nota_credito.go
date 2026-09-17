package application

import (
	"errors"
	"fmt"
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

// NotaCreditoDescuento emite una NC que baja el monto de la factura sin devolver
// mercancía. El descuento entra como un único renglón sin SKU: no hay producto
// que devolver, y ponerle uno haría que el guard anti-sobre-crédito de las
// devoluciones contara una cantidad que nadie devolvió.
func (s *Service) NotaCreditoDescuento(empresaID, actor, origen, refID string, monto float64, exento bool, nota string) (fiscal.Documento, error) {
	orig, err := s.facturaAcreditable(empresaID, refID)
	if err != nil {
		return fiscal.Documento{}, err
	}
	monto = round2(monto)
	if monto <= 0 {
		return fiscal.Documento{}, ErrNCDescuentoMonto
	}
	// No se puede acreditar más de lo facturado, contando lo ya acreditado por
	// otras notas: si no, la factura terminaría en negativo.
	if disponible := s.montoAcreditableRestante(empresaID, orig); monto > disponible+0.005 {
		return fiscal.Documento{}, fmt.Errorf("%w (disponible %.2f)", ErrNCDescuentoExcede, disponible)
	}
	texto := strings.TrimSpace(nota)
	if texto == "" {
		texto = fiscal.NombreMotivo(fiscal.MotivosNotaCredito(), fiscal.MotivoNCDescuento)
	}
	linea := fiscal.Linea{
		Nombre: "Descuento sobre " + orig.NumeroCompleto, Cantidad: 1,
		PrecioUnitario: monto, Total: monto, Exento: exento,
	}
	return s.emitirNotaCreditoSinMercancia(empresaID, actor, origen, orig,
		fiscal.MotivoNCDescuento, texto, []fiscal.Linea{linea})
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
			Nombre: fmt.Sprintf("Ajuste de precio · %s (%.2f → %.2f)", ol.Nombre, ol.PrecioUnitario, a.PrecioCorrecto),
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
