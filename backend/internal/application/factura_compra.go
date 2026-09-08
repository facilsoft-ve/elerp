package application

import (
	"errors"
	"fmt"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/empresa"
)

// Errores de negocio de la factura de compra del proveedor.
var (
	// ErrFacturaCompraDuplicada: la orden ya tiene una factura de compra registrada.
	ErrFacturaCompraDuplicada = errors.New("la orden de compra ya tiene una factura registrada")
	// ErrDatosFacturaCompra: faltan datos obligatorios del documento del proveedor.
	ErrDatosFacturaCompra = errors.New("la factura de compra requiere número de factura y número de control")
)

// LineaFacturaCompraEntrada es un renglón del documento del proveedor tal como lo
// captura la interfaz: puede diferir de la OC (precio, cantidad, ítem extra o
// faltante). El SKU debe existir en el catálogo (para amarrar productoID/nombre).
type LineaFacturaCompraEntrada struct {
	SKU           string
	Cantidad      float64
	CostoUnitario float64 // NETO (sin IVA)
	Exento        bool
}

// EntradaFacturaCompra son los datos del documento fiscal del proveedor. Lineas
// son los renglones TAL COMO vienen en la factura del proveedor y pueden diferir
// de lo ordenado/recibido: la OC es prefill y referencia, no camisa de fuerza. Si
// Lineas viene vacío, se toma lo efectivamente recibido de la orden (comportamiento
// clásico: la factura coincide con la recepción).
type EntradaFacturaCompra struct {
	NumeroFactura string // correlativo de la factura del proveedor
	NumeroControl string // número de control fiscal (SENIAT)
	Fecha         string // fecha del documento del proveedor
	Lineas        []LineaFacturaCompraEntrada
	// IVA es el importe de IVA TAL COMO aparece en el documento del proveedor. Una
	// factura de tercero trae su propio IVA (redondeos, alícuotas mixtas): si viene
	// > 0 se registra ESE importe; si es 0/omitido se computa sobre la base gravada a
	// la alícuota vigente (retrocompatibilidad).
	IVA float64
}

// FacturasCompra lista las facturas de compra de la empresa.
func (s *Service) FacturasCompra(empresaID string) []compra.FacturaCompra {
	if s.facturasCompra == nil {
		return []compra.FacturaCompra{}
	}
	return s.facturasCompra.List(empresaID)
}

// FacturaCompra devuelve una factura de compra por id.
func (s *Service) FacturaCompra(empresaID, id string) (compra.FacturaCompra, bool) {
	if s.facturasCompra == nil {
		return compra.FacturaCompra{}, false
	}
	return s.facturasCompra.ByID(empresaID, id)
}

// RegistrarFacturaCompra registra la factura FISCAL del proveedor contra una
// orden de compra recibida (total o parcial). La recepción ya asentó Inventario /
// Cuentas por pagar por el costo NETO recibido; esta factura registra el documento
// del proveedor con SUS PROPIAS líneas e importes (que pueden diferir de la OC) y
// asienta lo que falta para llevar la deuda con el proveedor a su total real:
//
//   - Debe IVA crédito fiscal (crédito de la factura), y
//   - Diferencia en compras (5202) por la base facturada − base recibida: al Debe
//     si la factura supera lo recibido, al Haber si es menor, y
//   - Cuentas por pagar por el neto de ambos (lleva CxP al total de la factura).
//
// El inventario NO se re-valúa: el Kardex conserva el costo efectivamente recibido
// y la diferencia de precio/cantidad de la factura queda explícita en 5202. Así se
// respeta el patrón append-only (la recepción es el único movimiento de stock) y la
// contabilidad cuadra por construcción. Es APPEND-ONLY y única por orden.
//
// Si in.Lineas viene vacío, las líneas se prefijan desde lo efectivamente recibido
// (la factura coincide con la recepción): la diferencia es cero y solo se reconoce
// el IVA, como antes.
func (s *Service) RegistrarFacturaCompra(empresaID, actor, origen, ordenCompraID string, in EntradaFacturaCompra) (compra.FacturaCompra, error) {
	if s.ordenesCompra == nil || s.facturasCompra == nil {
		return compra.FacturaCompra{}, ErrOCNoExiste
	}
	o, ok := s.ordenesCompra.ByID(empresaID, ordenCompraID)
	if !ok {
		return compra.FacturaCompra{}, ErrOCNoExiste
	}
	// Solo se factura lo recibido: una orden en borrador/confirmada no tiene aún
	// mercancía (ni deuda) contra la cual reconocer crédito fiscal.
	if o.Estado != compra.OCRecibida && o.Estado != compra.OCRecibidaParcial {
		return compra.FacturaCompra{}, ErrOCEstado
	}
	if _, existe := s.facturasCompra.ByOrden(empresaID, ordenCompraID); existe {
		return compra.FacturaCompra{}, ErrFacturaCompraDuplicada
	}
	if in.NumeroControl == "" || in.NumeroFactura == "" {
		return compra.FacturaCompra{}, ErrDatosFacturaCompra
	}

	// Base RECIBIDA: costo neto que la recepción asentó en Inventario / CxP. Es la
	// referencia contra la que se mide la diferencia de la factura.
	var baseRecibida float64
	for _, l := range o.Lineas {
		baseRecibida += l.CantidadRecibida * l.CostoUnitario
	}
	baseRecibida = round2(baseRecibida)

	// Líneas de la factura: las del proveedor si vienen, o lo recibido como prefill.
	lineas, err := s.armarLineasFacturaCompra(empresaID, o, in.Lineas)
	if err != nil {
		return compra.FacturaCompra{}, err
	}

	// Bases de la FACTURA (separando gravada de exenta) y su IVA crédito fiscal a la
	// alícuota vigente. El total es lo que realmente se le adeuda al proveedor.
	var baseGravada, baseExenta float64
	for _, l := range lineas {
		if l.Exento {
			baseExenta += l.Total
		} else {
			baseGravada += l.Total
		}
	}
	baseGravada = round2(baseGravada)
	baseExenta = round2(baseExenta)
	// IVA efectivo: el capturado del documento del proveedor si viene, o el computado
	// sobre la base gravada a la alícuota vigente. El asiento y la CxP usan este valor.
	iva := round2(baseGravada * s.alicuotaIVA(empresaID))
	if in.IVA > 0 {
		iva = round2(in.IVA)
	}
	total := round2(baseGravada + baseExenta + iva)
	// Diferencia de la factura contra lo recibido (puede ser + o −).
	diferencia := round2(baseGravada + baseExenta - baseRecibida)

	// RIF del proveedor desde el maestro (autocontenido en la factura, para que el
	// Libro de Compras no dependa de que el proveedor siga existiendo).
	rif := ""
	if s.provs != nil {
		if pr, found := s.provs.ByID(empresaID, o.ProveedorID); found {
			rif = pr.Documento
		}
	}

	f := compra.FacturaCompra{
		EmpresaID: empresaID, OrdenCompraID: o.ID,
		ProveedorID: o.ProveedorID, ProveedorNombre: o.ProveedorNombre, ProveedorRIF: rif,
		NumeroFactura: in.NumeroFactura, NumeroControl: in.NumeroControl, Fecha: in.Fecha,
		Lineas:        lineas,
		BaseImponible: baseGravada, BaseExenta: baseExenta, IVA: iva, Total: total,
		BaseRecibida: baseRecibida, DiferenciaBase: diferencia,
		Moneda: empresa.MonedaVES, Registrada: ahora(), Actor: actor,
	}
	out := s.facturasCompra.Append(f)

	// Asiento: la base recibida ya está en CxP desde la recepción. La factura suma
	// el IVA crédito fiscal (activo) y la diferencia de base contra lo recibido; la
	// contrapartida (CxP) es el neto, que lleva la deuda al total real de la factura.
	//   delta = iva + diferencia  (lo que hay que ajustar en CxP)
	// Si delta > 0 aumenta CxP (Haber); si < 0 la reduce (Debe). La diferencia: al
	// Debe si es positiva (mayor costo), al Haber si negativa (recuperación). El
	// asiento cuadra por construcción y las líneas en cero las descarta `asentar`.
	delta := round2(iva + diferencia)
	lineasAsiento := []contabilidad.Linea{
		{Codigo: contabilidad.CtaIVACreditoFiscal, Debe: iva},
	}
	if diferencia > 0.004 {
		lineasAsiento = append(lineasAsiento, contabilidad.Linea{Codigo: contabilidad.CtaDiferenciaEnCompras, Debe: diferencia})
	} else if diferencia < -0.004 {
		lineasAsiento = append(lineasAsiento, contabilidad.Linea{Codigo: contabilidad.CtaDiferenciaEnCompras, Haber: -diferencia})
	}
	if delta > 0.004 {
		lineasAsiento = append(lineasAsiento, contabilidad.Linea{Codigo: contabilidad.CtaCuentasPorPagar, Haber: delta})
	} else if delta < -0.004 {
		lineasAsiento = append(lineasAsiento, contabilidad.Linea{Codigo: contabilidad.CtaCuentasPorPagar, Debe: -delta})
	}
	// Solo se asienta si hay algo que reconocer (IVA o diferencia): una factura que
	// coincide con lo recibido y es toda exenta no mueve nada.
	if iva > 0.004 || diferencia > 0.004 || diferencia < -0.004 {
		s.asentar(empresaID, actor, out.Fecha, "Factura de compra "+out.NumeroFactura, "factura_compra", out.ID, lineasAsiento)
	}

	s.audit.Append(evento(empresaID, actor, origen, "compras.factura.registrar", out.NumeroFactura, out.NumeroControl))
	return out, nil
}

// armarLineasFacturaCompra construye las líneas de la factura del proveedor. Si
// `entrada` trae renglones, se usan tal cual (con productoID/nombre resueltos del
// catálogo y la exención heredada del producto salvo marca explícita); si viene
// vacía, se prefijan desde lo efectivamente RECIBIDO de la orden (cantidadRecibida
// a su costo), replicando el comportamiento clásico de «la factura = la recepción».
func (s *Service) armarLineasFacturaCompra(empresaID string, o compra.OrdenCompra, entrada []LineaFacturaCompraEntrada) ([]compra.LineaFacturaCompra, error) {
	if len(entrada) == 0 {
		out := make([]compra.LineaFacturaCompra, 0, len(o.Lineas))
		for _, l := range o.Lineas {
			if l.CantidadRecibida <= 0 {
				continue
			}
			out = append(out, compra.LineaFacturaCompra{
				ProductoID: l.ProductoID, SKU: l.SKU, Nombre: l.Nombre,
				Cantidad: l.CantidadRecibida, CostoUnitario: l.CostoUnitario,
				Total: round2(l.CantidadRecibida * l.CostoUnitario), Exento: l.Exento,
			})
		}
		return out, nil
	}
	out := make([]compra.LineaFacturaCompra, 0, len(entrada))
	for _, l := range entrada {
		if l.Cantidad <= 0 {
			continue // una línea sin cantidad no factura nada
		}
		p, ok := s.productos.BySKU(empresaID, l.SKU)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrProductoNoExiste, l.SKU)
		}
		exento := l.Exento || p.ExentoIVA
		out = append(out, compra.LineaFacturaCompra{
			ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre,
			Cantidad: l.Cantidad, CostoUnitario: l.CostoUnitario,
			Total: round2(l.Cantidad * l.CostoUnitario), Exento: exento,
		})
	}
	if len(out) == 0 {
		return nil, ErrRecepcionVacia
	}
	return out, nil
}
