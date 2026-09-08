package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/empresa"
)

// Errores de negocio de las notas de crédito/débito de compra.
var (
	// ErrNotasCompraNoConfig: el maestro de notas de compra no está cableado.
	ErrNotasCompraNoConfig = errors.New("las notas de compra no están configuradas")
	// ErrNotaCompraConcepto: la nota necesita un concepto que explique el ajuste.
	ErrNotaCompraConcepto = errors.New("la nota de compra necesita un concepto que explique el ajuste")
	// ErrNotaCompraVacia: la nota no ajusta ningún monto.
	ErrNotaCompraVacia = errors.New("la nota de compra no ajusta ningún monto")
	// ErrDatosNotaCompra: falta el número del documento del proveedor o su nº de control.
	ErrDatosNotaCompra = errors.New("la nota de compra requiere el número del documento del proveedor y su número de control")
	// ErrNotaCreditoCompraExcede: acreditar más de lo facturado (descontando NC previas).
	ErrNotaCreditoCompraExcede = errors.New("la nota de crédito excede el total de la factura de compra (descontando notas de crédito previas)")
)

// ConNotasCompra cablea el maestro de notas de crédito/débito de compra. Se
// configura aparte de New (como ConListasPrecio / ConLeadsDemo) para no romper las
// firmas de los constructores ya cableados en cmd/api; sin él, el servicio funciona
// igual y las notas de compra quedan deshabilitadas.
func (s *Service) ConNotasCompra(r compra.NotaCompraRepo) *Service {
	s.notasCompra = r
	return s
}

// NotaCompraEntrada son los datos de una nota de crédito/débito de proveedor. El
// monto es la BASE del ajuste (en Bs), en la misma línea que la ND de cliente: no
// está atado a los SKU de la orden (la factura de compra no guarda líneas), sino a
// un concepto con su base. El IVA se DERIVA de la alícuota histórica de la factura.
type NotaCompraEntrada struct {
	// Concepto explica el ajuste (queda como Concepto de la nota y motivo del
	// asiento). Obligatorio.
	Concepto string
	// Monto es la BASE del ajuste (positivo). El IVA se calcula encima con la
	// alícuota histórica de la factura de compra, salvo que el ajuste sea exento.
	Monto float64
	// Exento marca el ajuste como NO gravado con IVA crédito fiscal.
	Exento bool
	// Datos del documento tal como lo emitió el proveedor (para el Libro de compras).
	NumeroDocumento string
	NumeroControl   string
	Fecha           string
}

// NotasCompra lista las notas de crédito/débito de compra de la empresa (más
// reciente primero).
func (s *Service) NotasCompra(empresaID string) []compra.NotaCompra {
	if s.notasCompra == nil {
		return []compra.NotaCompra{}
	}
	out := s.notasCompra.List(empresaID)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Registrada > out[j-1].Registrada; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// NotaCompra devuelve una nota de compra por id.
func (s *Service) NotaCompra(empresaID, id string) (compra.NotaCompra, bool) {
	if s.notasCompra == nil {
		return compra.NotaCompra{}, false
	}
	return s.notasCompra.ByID(empresaID, id)
}

// EmitirNotaCreditoCompra emite una nota de crédito de PROVEEDOR (devolución o
// descuento) que referencia una factura de compra y DISMINUYE lo que se le debe.
// Es el ESPEJO en compras de la nota de crédito de cliente: guarda montos NEGATIVOS
// y asienta el contrario de (parte de) la compra. Append-only e inmutable.
func (s *Service) EmitirNotaCreditoCompra(empresaID, actor, origen, facturaCompraID string, in NotaCompraEntrada) (compra.NotaCompra, error) {
	return s.emitirNotaCompra(empresaID, actor, origen, facturaCompraID, compra.NotaCreditoCompra, in)
}

// EmitirNotaDebitoCompra emite una nota de débito de PROVEEDOR (cargo adicional:
// flete, interés, corrección al alza) que referencia una factura de compra y
// AUMENTA lo que se le debe. Es el ESPEJO POSITIVO de la nota de crédito de compra.
// Append-only e inmutable.
func (s *Service) EmitirNotaDebitoCompra(empresaID, actor, origen, facturaCompraID string, in NotaCompraEntrada) (compra.NotaCompra, error) {
	return s.emitirNotaCompra(empresaID, actor, origen, facturaCompraID, compra.NotaDebitoCompra, in)
}

// emitirNotaCompra es el cuerpo común de las notas de crédito/débito de compra: la
// única diferencia es el SIGNO (crédito resta, débito suma) y la dirección del
// asiento. Deriva el IVA de la alícuota histórica de la factura de compra.
func (s *Service) emitirNotaCompra(empresaID, actor, origen, facturaCompraID, tipo string, in NotaCompraEntrada) (compra.NotaCompra, error) {
	if s.notasCompra == nil || s.facturasCompra == nil {
		return compra.NotaCompra{}, ErrNotasCompraNoConfig
	}
	fc, ok := s.facturasCompra.ByID(empresaID, facturaCompraID)
	if !ok {
		return compra.NotaCompra{}, ErrFacturaCompraNoExiste
	}
	if strings.TrimSpace(in.Concepto) == "" {
		return compra.NotaCompra{}, ErrNotaCompraConcepto
	}
	if strings.TrimSpace(in.NumeroDocumento) == "" || strings.TrimSpace(in.NumeroControl) == "" {
		return compra.NotaCompra{}, ErrDatosNotaCompra
	}
	base := round2(in.Monto)
	if base <= 0.004 {
		return compra.NotaCompra{}, ErrNotaCompraVacia
	}

	// IVA con la ALÍCUOTA HISTÓRICA de la factura de compra: la que estuvo vigente al
	// registrarla, derivada de sus propios montos (IVA/base). Si la factura fue
	// íntegramente exenta (base gravada 0) se cae al default vigente. Es la misma
	// regla de tasa histórica de las notas de cliente.
	tasaIVA := s.alicuotaIVA(empresaID)
	if fc.BaseImponible > 0.004 {
		tasaIVA = fc.IVA / fc.BaseImponible
	}

	var baseImp, baseEx, iva float64
	if in.Exento {
		baseEx = base
	} else {
		baseImp = base
		iva = round2(base * tasaIVA)
	}
	totalPos := round2(base + iva)

	// Guard anti-sobre-crédito: la suma de notas de CRÉDITO no puede exceder el total
	// de la factura de compra. La ND (cargo) no tiene tope (es dinero adicional).
	if tipo == compra.NotaCreditoCompra {
		yaAcreditado := 0.0
		for _, n := range s.notasCompra.ByFactura(empresaID, fc.ID) {
			if n.Tipo == compra.NotaCreditoCompra {
				yaAcreditado += -n.Total // los montos de la NC se guardan negativos
			}
		}
		if round2(yaAcreditado+totalPos) > round2(fc.Total)+0.004 {
			return compra.NotaCompra{}, fmt.Errorf("%w: máximo %.2f", ErrNotaCreditoCompraExcede, round2(fc.Total-yaAcreditado))
		}
	}

	// Signo: la nota de crédito resta (negativo), la de débito suma (positivo).
	signo := 1.0
	serie := "NDC"
	if tipo == compra.NotaCreditoCompra {
		signo = -1
		serie = "NCC"
	}

	// La numeración se serializa por la sede de la ORDEN de la factura (el almacén que
	// recibió), para que el correlativo viva donde vive la operación de compra.
	sede := ""
	if s.ordenesCompra != nil {
		if o, found := s.ordenesCompra.ByID(empresaID, fc.OrdenCompraID); found {
			sede = o.SedeID
		}
	}

	n := compra.NotaCompra{
		EmpresaID: empresaID, FacturaCompraID: fc.ID, OrdenCompraID: fc.OrdenCompraID,
		ProveedorID: fc.ProveedorID, ProveedorNombre: fc.ProveedorNombre, ProveedorRIF: fc.ProveedorRIF,
		Tipo: tipo, Serie: serie,
		NumeroDocumento: strings.TrimSpace(in.NumeroDocumento), NumeroControl: strings.TrimSpace(in.NumeroControl), Fecha: in.Fecha,
		Concepto: strings.TrimSpace(in.Concepto),
		// Los montos se redondean en POSITIVO y se niegan al final: round2 trunca
		// hacia cero en negativo (misma convención que la NC de cliente en fiscal.go),
		// así que negar los positivos ya redondeados evita un descuadre de 1 céntimo.
		BaseImponible: signo * baseImp, BaseExenta: signo * baseEx,
		IVA: signo * iva, Total: signo * totalPos,
		Moneda: empresa.MonedaVES, Registrada: ahora(), Actor: actor,
	}
	n.Numero = s.numerador.Siguiente(empresaID, sede, serie)
	n.NumeroCompleto = fmt.Sprintf("%s-%06d", serie, n.Numero)
	out := s.notasCompra.Append(n)

	// Asiento DERIVADO: cuadra por construcción (total = base + IVA). Ajusta la CxP
	// del proveedor en el mismo sentido que la deuda.
	s.asentarNotaCompra(empresaID, actor, out)
	s.audit.Append(evento(empresaID, actor, origen, "compras.nota."+notaVerbo(tipo), out.NumeroCompleto, out.ProveedorNombre))
	return out, nil
}

// notaVerbo mapea el tipo de nota a un verbo corto para el evento de auditoría.
func notaVerbo(tipo string) string {
	if tipo == compra.NotaCreditoCompra {
		return "credito"
	}
	return "debito"
}

// asentarNotaCompra registra el asiento derivado de una nota de crédito/débito de
// compra. Espeja el asiento de la factura de compra (base ya en Inventario, IVA
// crédito fiscal, deuda en CxP):
//
//   - NC de compra (devolución/descuento, baja la deuda): Debe CxP 2101 / Haber IVA
//     crédito 1103 + Inventario 1201. Revierte (parte de) la compra.
//   - ND de compra (cargo adicional, sube la deuda): Debe Inventario 1201 + IVA
//     crédito 1103 / Haber CxP 2101.
//
// Se asienta con las MAGNITUDES (la nota guarda los montos con signo). Cuadra
// siempre: total = base + IVA.
func (s *Service) asentarNotaCompra(empresaID, actor string, n compra.NotaCompra) {
	if s.asientos == nil {
		return
	}
	base := absF(n.BaseImponible) + absF(n.BaseExenta)
	iva := absF(n.IVA)
	total := absF(n.Total)
	if n.Tipo == compra.NotaCreditoCompra {
		s.asentar(empresaID, actor, n.Fecha, "Nota de crédito de compra "+n.NumeroCompleto, "nota_compra", n.ID, []contabilidad.Linea{
			{Codigo: contabilidad.CtaCuentasPorPagar, Debe: round2(total)},
			{Codigo: contabilidad.CtaIVACreditoFiscal, Haber: round2(iva)},
			{Codigo: contabilidad.CtaInventario, Haber: round2(base)},
		})
		return
	}
	s.asentar(empresaID, actor, n.Fecha, "Nota de débito de compra "+n.NumeroCompleto, "nota_compra", n.ID, []contabilidad.Linea{
		{Codigo: contabilidad.CtaInventario, Debe: round2(base)},
		{Codigo: contabilidad.CtaIVACreditoFiscal, Debe: round2(iva)},
		{Codigo: contabilidad.CtaCuentasPorPagar, Haber: round2(total)},
	})
}

// notasCompraNetasPorFactura suma el AJUSTE neto de las notas de compra por factura
// (NC negativas, ND positivas). Alimenta Cuentas por pagar: la NC baja la deuda de
// la orden facturada y la ND la sube, imputadas a la factura y por ella a su orden.
// absF (para las magnitudes de los asientos) se reutiliza de cierrez.go.
func (s *Service) notasCompraNetasPorFactura(empresaID string) map[string]float64 {
	out := map[string]float64{}
	if s.notasCompra == nil {
		return out
	}
	for _, n := range s.notasCompra.List(empresaID) {
		out[n.FacturaCompraID] += n.Total
	}
	return out
}
