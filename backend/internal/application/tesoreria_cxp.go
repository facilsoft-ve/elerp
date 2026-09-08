package application

import "github.com/mornix/elerp/internal/domain/compra"

/* --- Cuentas por pagar (proyección) ----------------------------------------
 *
 * El espejo de Cuentas por cobrar, pero del lado del PASIVO con proveedores. No
 * se guarda un saldo por pagar: se DERIVA de las órdenes de compra con mercancía
 * recibida. Cuando se recibe una orden, la recepción asienta la deuda en 2101
 * (Cuentas por pagar) por el costo NETO que entró al inventario; acá se reconstruye
 * ese pasivo leyendo las órdenes, sin un contador editable que desincronizar.
 *
 * Los PAGOS a proveedor (ledger de solo-anexado, ver tesoreria_pagos.go) rebajan lo
 * adeudado: el saldo NETO es «adeudado − pagado». Un pago puede imputarse a una
 * orden (baja el saldo de esa orden) o ir a cuenta del proveedor (baja su deuda
 * global sin señalar una orden). TotalPorPagar es el saldo neto total.
 */

// CuentaPorPagar es una orden de compra con mercancía recibida y su deuda derivada.
type CuentaPorPagar struct {
	OrdenID         string `json:"ordenId"`
	NumeroCompleto  string `json:"numeroCompleto"`
	Fecha           string `json:"fecha"`
	ProveedorID     string `json:"proveedorId"`
	ProveedorNombre string `json:"proveedorNombre"`
	RIF             string `json:"rif"`
	Estado          string `json:"estado"`
	// Recibido es lo adeudado por esta orden: si ya tiene factura de compra del
	// proveedor, el Total de la factura (base + IVA); si aún no, el costo NETO
	// recibido = Σ por línea (cantidadRecibida * costoUnitario).
	Recibido float64 `json:"recibido"`
	// Pagado es lo abonado a ESTA orden (pagos que la referencian, neto de reversos).
	// Los pagos a cuenta del proveedor no se imputan a ninguna orden.
	Pagado float64 `json:"pagado"`
	// Saldo es lo que falta por pagar de la orden: Recibido − Pagado.
	Saldo float64 `json:"saldo"`
	// Facturada indica si la orden ya tiene su factura fiscal registrada (con
	// número de control). Sin factura, la deuda es neta y falta el IVA por declarar.
	Facturada bool `json:"facturada"`
}

// DeudaProveedor agrupa lo adeudado a un proveedor por todas sus órdenes recibidas.
type DeudaProveedor struct {
	ProveedorID     string  `json:"proveedorId"`
	ProveedorNombre string  `json:"proveedorNombre"`
	RIF             string  `json:"rif"`
	Ordenes         int     `json:"ordenes"`
	Recibido        float64 `json:"recibido"`
	// Pagado es todo lo pagado al proveedor (imputado a órdenes o a cuenta), neto
	// de reversos. Saldo es la deuda neta: Recibido − Pagado.
	Pagado float64 `json:"pagado"`
	Saldo  float64 `json:"saldo"`
}

// ResumenPorPagar son los totales que alimentan los KPIs de Cuentas por pagar,
// para que la interfaz no los recalcule a su manera.
type ResumenPorPagar struct {
	Proveedores         []DeudaProveedor `json:"proveedores"`
	Ordenes             []CuentaPorPagar `json:"ordenes"`
	TotalPorPagar       float64          `json:"totalPorPagar"`
	ProveedoresConSaldo int              `json:"proveedoresConSaldo"`
	OrdenesConSaldo     int              `json:"ordenesConSaldo"`
}

// CuentasPorPagar proyecta las órdenes de compra con mercancía recibida como el
// pasivo con proveedores. Espejo de CuentasPorCobrar: agrupa por proveedor y
// ordena de mayor a menor deuda (a quién se le debe más, primero).
func (s *Service) CuentasPorPagar(empresaID string) ResumenPorPagar {
	res := ResumenPorPagar{Proveedores: []DeudaProveedor{}, Ordenes: []CuentaPorPagar{}}
	if s.ordenesCompra == nil {
		return res
	}

	// El RIF del proveedor se cachea: varias órdenes comparten proveedor.
	rifCache := map[string]string{}
	rifDe := func(provID string) string {
		if r, ok := rifCache[provID]; ok {
			return r
		}
		r := ""
		if s.provs != nil {
			if pr, found := s.provs.ByID(empresaID, provID); found {
				r = pr.Documento
			}
		}
		rifCache[provID] = r
		return r
	}

	// Pagos ya hechos, netos de reversos: por proveedor (todos) y por orden (solo
	// los imputados a una orden concreta). Rebajan lo adeudado.
	pagadoProv, pagadoOrden := s.pagosProveedorNetos(empresaID)

	// Retenciones EMITIDAS (IVA o ISLR) sobre la factura de compra de una orden: ese
	// impuesto no se le paga al proveedor (se entera al SENIAT), así que baja la deuda.
	// Se cuenta como aplicado, igual que un pago. Se acumula por proveedor sobre la
	// marcha, porque la retención cuelga de la factura y esta de la orden.
	retPorFactura := s.retencionesEmitidasPorFactura(empresaID)
	retProv := map[string]float64{}

	// Notas de crédito/débito de PROVEEDOR sobre la factura de compra de una orden:
	// ajustan la DEUDA (no un pago). La NC (negativa) la baja, la ND (positiva) la
	// sube. Se suman al costo facturado de la orden, imputadas por su factura.
	notasPorFactura := s.notasCompraNetasPorFactura(empresaID)

	porProv := map[string]*DeudaProveedor{}
	for _, o := range s.ordenesCompra.List(empresaID) {
		// Solo las órdenes con mercancía recibida generan pasivo: es la recepción
		// la que asienta la deuda, no la orden confirmada ni el borrador.
		if o.Estado != compra.OCRecibidaParcial && o.Estado != compra.OCRecibida {
			continue
		}
		// Lo adeudado = costo NETO de lo efectivamente recibido (lo que entró al
		// Kardex y asentó 2101). Si la orden ya tiene factura del proveedor, la deuda
		// pasa a ser el Total de la factura (base + IVA): el IVA crédito ya se asentó
		// contra CxP y ahora forma parte del pasivo.
		recibido := 0.0
		for _, l := range o.Lineas {
			recibido += l.CantidadRecibida * l.CostoUnitario
		}
		recibido = round2(recibido)
		if recibido <= 0.004 {
			continue
		}
		facturada := false
		retEmitida := 0.0
		if s.facturasCompra != nil {
			if fc, ok := s.facturasCompra.ByOrden(empresaID, o.ID); ok {
				facturada = true
				// La deuda facturada = total de la factura MÁS el ajuste neto de sus
				// notas de crédito/débito (NC baja, ND sube).
				recibido = round2(fc.Total + notasPorFactura[fc.ID])
				retEmitida = retPorFactura[fc.ID]
			}
		}
		retProv[o.ProveedorID] += retEmitida
		rif := rifDe(o.ProveedorID)
		// Lo retenido cuenta como aplicado a la orden, junto con los pagos.
		pagadoDeOrden := round2(pagadoOrden[o.ID] + retEmitida)
		res.Ordenes = append(res.Ordenes, CuentaPorPagar{
			OrdenID: o.ID, NumeroCompleto: o.NumeroCompleto, Fecha: o.Creada,
			ProveedorID: o.ProveedorID, ProveedorNombre: o.ProveedorNombre, RIF: rif,
			Estado: o.Estado, Recibido: recibido, Pagado: pagadoDeOrden,
			Saldo: round2(recibido - pagadoDeOrden), Facturada: facturada,
		})

		p := porProv[o.ProveedorID]
		if p == nil {
			p = &DeudaProveedor{ProveedorID: o.ProveedorID, ProveedorNombre: o.ProveedorNombre, RIF: rif}
			porProv[o.ProveedorID] = p
		}
		p.Ordenes++
		p.Recibido = round2(p.Recibido + recibido)
	}

	// El saldo neto por proveedor = adeudado − pagado (incluye los pagos a cuenta,
	// que no cuelgan de ninguna orden). TotalPorPagar es la suma de esos saldos.
	for _, p := range porProv {
		p.Pagado = round2(pagadoProv[p.ProveedorID] + retProv[p.ProveedorID])
		p.Saldo = round2(p.Recibido - p.Pagado)
		res.Proveedores = append(res.Proveedores, *p)
		res.TotalPorPagar += p.Saldo
		if p.Saldo > 0.004 {
			res.ProveedoresConSaldo++
		}
	}
	res.TotalPorPagar = round2(res.TotalPorPagar)
	for _, o := range res.Ordenes {
		if o.Saldo > 0.004 {
			res.OrdenesConSaldo++
		}
	}

	// A quién se le debe más (neto), primero: es a quién hay que pagarle o negociar antes.
	for i := 1; i < len(res.Proveedores); i++ {
		for j := i; j > 0 && res.Proveedores[j].Saldo > res.Proveedores[j-1].Saldo; j-- {
			res.Proveedores[j], res.Proveedores[j-1] = res.Proveedores[j-1], res.Proveedores[j]
		}
	}
	for i := 1; i < len(res.Ordenes); i++ {
		for j := i; j > 0 && res.Ordenes[j].Saldo > res.Ordenes[j-1].Saldo; j-- {
			res.Ordenes[j], res.Ordenes[j-1] = res.Ordenes[j-1], res.Ordenes[j]
		}
	}
	return res
}

// pagosProveedorNetos suma los pagos ya hechos, netos de reversos: por proveedor
// (todos) y por orden (solo los imputados a una orden). Es la base para derivar el
// saldo NETO de Cuentas por pagar.
func (s *Service) pagosProveedorNetos(empresaID string) (porProv, porOrden map[string]float64) {
	porProv = map[string]float64{}
	porOrden = map[string]float64{}
	if s.pagosProveedor == nil {
		return
	}
	for _, p := range s.pagosProveedor.List(empresaID) {
		signo := 1.0
		if p.Reverso {
			signo = -1
		}
		porProv[p.ProveedorID] += signo * p.MontoBs
		if p.OrdenCompraID != "" {
			porOrden[p.OrdenCompraID] += signo * p.MontoBs
		}
	}
	return
}

// saldoPorPagarProveedor devuelve el saldo NETO pendiente de un proveedor
// (adeudado − pagado). Es el tope de un nuevo pago.
func (s *Service) saldoPorPagarProveedor(empresaID, proveedorID string) float64 {
	for _, p := range s.CuentasPorPagar(empresaID).Proveedores {
		if p.ProveedorID == proveedorID {
			return p.Saldo
		}
	}
	return 0
}
