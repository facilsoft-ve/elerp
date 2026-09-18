package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/inventario"
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
	// ErrNotaDebitoConLineas: una ND no devuelve mercancía. La entrada de stock la
	// hace la recepción de la orden, no un cargo del proveedor.
	ErrNotaDebitoConLineas = errors.New("una nota de débito no mueve mercancía: el stock entra por la recepción de la orden")
	// ErrNotaCompraMontoYLineas: con líneas devueltas el monto se DERIVA de ellas
	// (cantidad × costo con que entró); tecleado aparte, uno de los dos mentiría.
	ErrNotaCompraMontoYLineas = errors.New("una devolución deriva su monto de las líneas devueltas: no se indica aparte")
	// ErrDevolucionVacia: se pidió devolver, pero ninguna línea trae cantidad.
	ErrDevolucionVacia = errors.New("la devolución no indica cantidades a devolver")
	// ErrDevolucionSKUAjeno: el producto no está en la orden que originó la factura.
	ErrDevolucionSKUAjeno = errors.New("ese producto no está en la orden de compra de la factura")
	// ErrDevolucionExcede: devolver más de lo recibido (descontando lo ya devuelto).
	ErrDevolucionExcede = errors.New("no se puede devolver más de lo recibido en la orden")
	// ErrDevolucionSinStock: la mercancía ya no está en el almacén que la recibió.
	ErrDevolucionSinStock = errors.New("no hay stock suficiente en el almacén para devolver")
)

// refNotaCompra etiqueta los movimientos de inventario que emite una devolución a
// proveedor. Sirve para trazarlos hasta su nota y para que la ROTACIÓN del reporte
// de inventario no los cuente como ventas (salen, pero no se vendieron).
const refNotaCompra = "nota_compra"

// ConNotasCompra cablea el maestro de notas de crédito/débito de compra. Se
// configura aparte de New (como ConListasPrecio / ConAlmacenes) para no romper las
// firmas de los constructores ya cableados en cmd/api; sin él, el servicio funciona
// igual y las notas de compra quedan deshabilitadas.
func (s *Service) ConNotasCompra(r compra.NotaCompraRepo) *Service {
	s.notasCompra = r
	return s
}

// LineaNotaCompraEntrada es un renglón a DEVOLVER al proveedor: qué SKU y cuánto.
// El costo no se indica — lo pone el servidor desde la línea de la orden de compra,
// que es el costo con que la mercancía entró al Kardex.
type LineaNotaCompraEntrada struct {
	SKU      string
	Cantidad float64
}

// NotaCompraEntrada son los datos de una nota de crédito/débito de proveedor.
//
// La nota admite DOS formas, excluyentes entre sí:
//
//   - Con Lineas ⇒ DEVOLUCIÓN de mercancía (solo nota de crédito). La base se
//     deriva de las líneas y el ledger de inventario recibe una salida por cada una.
//   - Con Monto ⇒ AJUSTE de monto (descuento, rebaja, flete, interés). No mueve
//     stock, y por eso su asiento no toca la cuenta de inventario.
//
// El IVA se DERIVA en ambos casos de la alícuota histórica de la factura.
type NotaCompraEntrada struct {
	// Concepto explica el ajuste (queda como Concepto de la nota y motivo del
	// asiento). Obligatorio.
	Concepto string
	// Monto es la BASE del ajuste (positivo) cuando la nota NO devuelve mercancía.
	// El IVA se calcula encima con la alícuota histórica de la factura de compra,
	// salvo que el ajuste sea exento.
	Monto float64
	// Exento marca el AJUSTE como NO gravado con IVA crédito fiscal. En una
	// devolución no se usa: la exención se hereda de cada línea de la orden.
	Exento bool
	// Lineas son los renglones devueltos. Vacío ⇒ la nota es un ajuste de monto.
	Lineas []LineaNotaCompraEntrada
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

	// La orden que originó la factura: de ella salen la sede (numeración y almacén) y,
	// si la nota devuelve mercancía, lo recibido contra lo que se valida.
	var orden compra.OrdenCompra
	if s.ordenesCompra != nil {
		if o, found := s.ordenesCompra.ByID(empresaID, fc.OrdenCompraID); found {
			orden = o
		}
	}

	// Devolución (con líneas) vs ajuste de monto (sin ellas). Todo se valida ANTES de
	// anexar nada: la nota es inmutable y los movimientos también, así que una
	// devolución a medias no se puede deshacer.
	base := round2(in.Monto)
	var lineas []compra.LineaNotaCompra
	almacenID := ""
	if len(in.Lineas) > 0 {
		if tipo != compra.NotaCreditoCompra {
			return compra.NotaCompra{}, ErrNotaDebitoConLineas
		}
		if base > 0.004 {
			return compra.NotaCompra{}, ErrNotaCompraMontoYLineas
		}
		var err error
		lineas, almacenID, err = s.armarLineasDevolucion(empresaID, fc, orden, in.Lineas)
		if err != nil {
			return compra.NotaCompra{}, err
		}
	} else if base <= 0.004 {
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

	// Bases: en una devolución salen de las líneas (con la exención heredada de cada
	// una); en un ajuste, del monto indicado.
	var baseImp, baseEx float64
	if len(lineas) > 0 {
		for _, l := range lineas {
			if l.Exento {
				baseEx += l.Total
			} else {
				baseImp += l.Total
			}
		}
		baseImp, baseEx = round2(baseImp), round2(baseEx)
	} else if in.Exento {
		baseEx = base
	} else {
		baseImp = base
	}
	iva := round2(baseImp * tasaIVA)
	totalPos := round2(baseImp + baseEx + iva)

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
	sede := orden.SedeID

	n := compra.NotaCompra{
		EmpresaID: empresaID, FacturaCompraID: fc.ID, OrdenCompraID: fc.OrdenCompraID,
		ProveedorID: fc.ProveedorID, ProveedorNombre: fc.ProveedorNombre, ProveedorRIF: fc.ProveedorRIF,
		Tipo: tipo, Serie: serie,
		NumeroDocumento: strings.TrimSpace(in.NumeroDocumento), NumeroControl: strings.TrimSpace(in.NumeroControl), Fecha: in.Fecha,
		Concepto: strings.TrimSpace(in.Concepto),
		Lineas:   lineas,
		// Los montos se redondean en POSITIVO y se niegan al final, misma convención
		// que la NC de cliente en fiscal.go: así el total y sus partes se redondean
		// sobre las mismas magnitudes y no puede aparecer un céntimo de diferencia
		// entre ellos.
		BaseImponible: signo * baseImp, BaseExenta: signo * baseEx,
		IVA: signo * iva, Total: signo * totalPos,
		Moneda: empresa.MonedaVES, Registrada: ahora(), Actor: actor,
	}
	n.Numero = s.numerador.Siguiente(empresaID, sede, serie)
	n.NumeroCompleto = fmt.Sprintf("%s-%06d", serie, n.Numero)
	out := s.notasCompra.Append(n)

	// La mercancía devuelta SALE del ledger, al mismo costo con que entró y por el
	// mismo almacén que la recibió. Es lo que hace que el Kardex diga la verdad sin
	// que nadie tenga que ajustar a mano: sin esto, la contabilidad rebaja el
	// inventario y el almacén sigue contando las unidades.
	for _, l := range out.Lineas {
		s.movimientos.Append(inventario.Movimiento{
			EmpresaID: empresaID, SedeID: sede, AlmacenID: almacenID,
			ProductoID: l.ProductoID, SKU: l.SKU,
			Tipo: inventario.MovSalida, Cantidad: -l.Cantidad, CostoUnitario: l.CostoSalida,
			Motivo:  "devolución al proveedor " + out.NumeroCompleto,
			RefTipo: refNotaCompra, RefID: out.ID, Actor: actor, Fecha: ahora(),
		})
	}

	// Asiento DERIVADO: cuadra por construcción (total = base + IVA). Ajusta la CxP
	// del proveedor en el mismo sentido que la deuda.
	s.asentarNotaCompra(empresaID, actor, out)
	s.audit.Append(evento(empresaID, actor, origen, "compras.nota."+notaVerbo(tipo), out.NumeroCompleto, out.ProveedorNombre))
	return out, nil
}

// armarLineasDevolucion valida lo que se pretende devolver contra la ORDEN que
// originó la factura y lo completa con el costo de entrada. Devuelve además el
// almacén del que va a salir la mercancía.
//
// Es TODO O NADA, igual que la recepción: si una sola línea no pasa, no se devuelve
// nada. Se valida contra dos topes distintos y ambos importan:
//
//   - lo RECIBIDO de esa línea menos lo ya devuelto en notas previas — no se le puede
//     devolver al proveedor más de lo que entregó;
//   - lo DISPONIBLE hoy en el almacén — no se puede sacar lo que ya no está, o el
//     ledger quedaría en negativo (misma política que el despacho de una
//     transferencia).
func (s *Service) armarLineasDevolucion(empresaID string, fc compra.FacturaCompra, orden compra.OrdenCompra,
	in []LineaNotaCompraEntrada) ([]compra.LineaNotaCompra, string, error) {
	if orden.ID == "" {
		return nil, "", ErrOCNoExiste
	}
	// Agrega por SKU conservando el orden de llegada: dos renglones del mismo
	// producto son una sola devolución.
	pedido := map[string]float64{}
	skus := []string{}
	for _, l := range in {
		sku := strings.TrimSpace(l.SKU)
		if sku == "" || l.Cantidad <= 0 {
			continue
		}
		if _, visto := pedido[sku]; !visto {
			skus = append(skus, sku)
		}
		pedido[sku] += l.Cantidad
	}
	if len(skus) == 0 {
		return nil, "", ErrDevolucionVacia
	}

	// Lo ya devuelto en notas previas de esta factura (que es única por orden).
	yaDevuelto := map[string]float64{}
	for _, n := range s.notasCompra.ByFactura(empresaID, fc.ID) {
		for _, l := range n.Lineas {
			yaDevuelto[l.SKU] += l.Cantidad
		}
	}

	// La mercancía sale por donde entró: el almacén al que la recepción la metió.
	almacenID := s.almacenParaEscritura(empresaID, orden.SedeID, "")

	idx := map[string]int{}
	for i, l := range orden.Lineas {
		idx[l.SKU] = i
	}
	out := make([]compra.LineaNotaCompra, 0, len(skus))
	for _, sku := range skus {
		cant := round2(pedido[sku])
		i, existe := idx[sku]
		if !existe {
			return nil, "", fmt.Errorf("%w: %s", ErrDevolucionSKUAjeno, sku)
		}
		l := orden.Lineas[i]
		devolvible := round2(l.CantidadRecibida - yaDevuelto[sku])
		if cant > devolvible+0.0001 {
			return nil, "", fmt.Errorf("%w: %s (devolvible %.2f, solicitado %.2f)",
				ErrDevolucionExcede, sku, devolvible, cant)
		}
		if disp := s.disponibleEnAlmacen(empresaID, almacenID, orden.SedeID, sku); disp < cant-0.0001 {
			return nil, "", fmt.Errorf("%w: %s (disponible %.2f, requiere %.2f)",
				ErrDevolucionSinStock, l.Nombre, disp, cant)
		}
		// Costo con que la mercancía SALE del Kardex: el promedio vigente de la sede,
		// como en una venta. No es el costo de la orden —el ledger no costea por lote—
		// y es el importe por el que tiene que bajar la cuenta de inventario. Se guarda
		// SIN redondear, igual que el costo del movimiento de una venta: redondearlo
		// por unidad desviaría el asiento de la valorización hasta medio céntimo por
		// unidad devuelta. El redondeo va al final, sobre el total.
		_, avg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: orden.SedeID, SKU: sku}))
		out = append(out, compra.LineaNotaCompra{
			ProductoID: l.ProductoID, SKU: l.SKU, Nombre: l.Nombre,
			Cantidad: cant, CostoUnitario: l.CostoUnitario, CostoSalida: avg,
			Total: round2(cant * l.CostoUnitario), Exento: l.Exento,
		})
	}
	return out, almacenID, nil
}

// notaVerbo mapea el tipo de nota a un verbo corto para el evento de auditoría.
func notaVerbo(tipo string) string {
	if tipo == compra.NotaCreditoCompra {
		return "credito"
	}
	return "debito"
}

// asentarNotaCompra registra el asiento derivado de una nota de crédito/débito de
// compra. Espeja el asiento de la factura de compra (IVA crédito fiscal, deuda en
// CxP) y la contrapartida de la base depende de si la mercancía SE MOVIÓ:
//
//   - NC con líneas (DEVOLUCIÓN, baja la deuda): Debe CxP 2101 / Haber IVA crédito
//     1103 + Inventario 1201 POR EL VALOR QUE SALE DEL KARDEX (cantidad × costo
//     promedio vigente, igual que una venta) + la diferencia contra lo que el
//     proveedor acredita, en 5202. Acreditar 1201 por el precio de compra en vez de
//     por el valor que sale separaría el balance de la valorización — el mismo
//     descuadre, en la otra dirección.
//   - NC sin líneas (descuento o rebaja) y toda ND (flete, interés, corrección al
//     alza): la base va a 5202 Diferencia en compras, NO a Inventario. El Kardex no
//     se re-valúa —el costo del inventario es el que la recepción metió—, y tocar
//     1201 sin un movimiento detrás separaría el balance de la valorización en
//     silencio. Es el mismo criterio con el que la factura de compra trata su
//     diferencia contra lo recibido.
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

	// Devolución: el inventario baja por lo que SALE del Kardex, no por lo que el
	// proveedor acredita. Lo que sobra o falta entre ambos es resultado del período.
	if n.EsDevolucion() {
		valor := 0.0
		for _, l := range n.Lineas {
			valor += l.Cantidad * l.CostoSalida
		}
		valor = round2(valor)
		lineas := []contabilidad.Linea{
			{Codigo: contabilidad.CtaCuentasPorPagar, Debe: round2(total)},
			{Codigo: contabilidad.CtaIVACreditoFiscal, Haber: round2(iva)},
			{Codigo: contabilidad.CtaInventario, Haber: valor},
		}
		// base > valor ⇒ te acreditan más de lo que valía: ganancia (al haber).
		// base < valor ⇒ sale del inventario más valor del que te devuelven: pérdida.
		// Se compara antes de restar y cada rama redondea una magnitud positiva: deja
		// explícito de qué lado cae el ajuste, en vez de deducirlo del signo.
		if base > valor+0.004 {
			lineas = append(lineas, contabilidad.Linea{Codigo: contabilidad.CtaDiferenciaEnCompras, Haber: round2(base - valor)})
		} else if valor > base+0.004 {
			lineas = append(lineas, contabilidad.Linea{Codigo: contabilidad.CtaDiferenciaEnCompras, Debe: round2(valor - base)})
		}
		s.asentar(empresaID, actor, n.Fecha, "Nota de crédito de compra "+n.NumeroCompleto+" (devolución)", "nota_compra", n.ID, lineas)
		return
	}

	ctaBase := contabilidad.CtaDiferenciaEnCompras
	if n.Tipo == compra.NotaCreditoCompra {
		s.asentar(empresaID, actor, n.Fecha, "Nota de crédito de compra "+n.NumeroCompleto, "nota_compra", n.ID, []contabilidad.Linea{
			{Codigo: contabilidad.CtaCuentasPorPagar, Debe: round2(total)},
			{Codigo: contabilidad.CtaIVACreditoFiscal, Haber: round2(iva)},
			{Codigo: ctaBase, Haber: round2(base)},
		})
		return
	}
	s.asentar(empresaID, actor, n.Fecha, "Nota de débito de compra "+n.NumeroCompleto, "nota_compra", n.ID, []contabilidad.Linea{
		{Codigo: ctaBase, Debe: round2(base)},
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
