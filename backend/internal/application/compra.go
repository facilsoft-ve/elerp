package application

import (
	"errors"
	"fmt"
	"math"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// Errores de negocio del módulo de Compras.
var (
	ErrOCNoExiste      = errors.New("orden de compra no existe")
	ErrOCVacia         = errors.New("la orden de compra no tiene líneas")
	ErrOCEstado        = errors.New("la orden de compra no está en un estado que permita esta acción")
	ErrRecepcionExcede = errors.New("la recepción excede lo pendiente de esa línea")
	ErrRecepcionVacia  = errors.New("la recepción no indica líneas")
	// ErrComboEnCompra: un combo (paquete de productos) no se compra ni se recibe;
	// se compran sus componentes. Anexar un movimiento de inventario para el SKU del
	// combo crearía stock fantasma y doble conteo.
	ErrComboEnCompra = errors.New("un combo no se compra ni se recibe: se compran sus componentes")
	// ErrOrdenYaFacturada: la orden ya tiene su factura de compra registrada, que
	// cerró la deuda; no se admite recibir más sobre ella (mantiene asiento ⇄ CxP).
	ErrOrdenYaFacturada = errors.New("la orden ya tiene una factura registrada: no admite más recepciones")
	// ErrSKUduplicado: la orden trae el mismo SKU repetido con costos distintos; no
	// se puede fusionar (se fusiona solo cuando el costo coincide).
	ErrSKUduplicado = errors.New("el mismo SKU aparece con costos distintos en la orden")
)

// serieOrdenCompra es la correlativa de las órdenes de compra (no es fiscal).
const serieOrdenCompra = "OC"

// LineaOCEntrada es un renglón al crear una orden: el costo NETO (sin IVA) al que
// se pacta la compra con el proveedor.
type LineaOCEntrada struct {
	SKU           string
	Cantidad      float64
	CostoUnitario float64
	Exento        bool
}

// EntradaOC son los datos para crear una orden de compra.
type EntradaOC struct {
	ProveedorID     string
	SedeID          string
	CondicionesPago string
	Notas           string
	Lineas          []LineaOCEntrada
}

// LineaRecepcion indica cuánto se recibe de un SKU en una recepción concreta.
type LineaRecepcion struct {
	SKU      string
	Cantidad float64
}

// OrdenesCompra lista las órdenes de compra de la empresa.
func (s *Service) OrdenesCompra(empresaID string) []compra.OrdenCompra {
	if s.ordenesCompra == nil {
		return []compra.OrdenCompra{}
	}
	return s.ordenesCompra.List(empresaID)
}

// OrdenCompra devuelve una orden de compra por id.
func (s *Service) OrdenCompra(empresaID, id string) (compra.OrdenCompra, bool) {
	if s.ordenesCompra == nil {
		return compra.OrdenCompra{}, false
	}
	return s.ordenesCompra.ByID(empresaID, id)
}

// armarLineasOC construye las líneas desde el catálogo (enriqueciendo
// nombre/productoID) y calcula los totales. El costo es NETO: el IVA se calcula
// solo sobre las líneas no exentas (crédito fiscal), separado del costo que
// entrará al inventario.
func (s *Service) armarLineasOC(empresaID string, in EntradaOC) ([]compra.Linea, float64, float64, float64, error) {
	lineas := make([]compra.Linea, 0, len(in.Lineas))
	idxPorSKU := map[string]int{}
	for _, l := range in.Lineas {
		p, ok := s.productos.BySKU(empresaID, l.SKU)
		if !ok {
			return nil, 0, 0, 0, fmt.Errorf("%w: %s", ErrProductoNoExiste, l.SKU)
		}
		// Un combo no se compra: se compran sus componentes. Rechazar aquí evita que
		// la recepción anexe un movimiento de stock para el SKU del combo.
		if p.EsCombo {
			return nil, 0, 0, 0, fmt.Errorf("%w: %s", ErrComboEnCompra, p.SKU)
		}
		// Guardas de backend (el front ya valida, pero el API es el contrato): una
		// cantidad ≤0 mete líneas basura y un costo negativo arrastra el costo promedio
		// ponderado a la baja de forma permanente al recibir.
		if l.Cantidad <= 0 {
			return nil, 0, 0, 0, fmt.Errorf("la cantidad de %s debe ser mayor a 0", p.SKU)
		}
		if l.CostoUnitario < 0 {
			return nil, 0, 0, 0, fmt.Errorf("el costo de %s no puede ser negativo", p.SKU)
		}
		// La exención hereda del producto salvo que la entrada la marque explícita.
		exento := l.Exento || p.ExentoIVA
		// Fusiona renglones del mismo SKU: si el costo coincide se suman las
		// cantidades (una sola línea recibible); si difiere, se rechaza (una OC no
		// puede tener el mismo SKU a dos precios).
		if i, dup := idxPorSKU[p.SKU]; dup {
			if math.Abs(lineas[i].CostoUnitario-l.CostoUnitario) > 0.0001 {
				return nil, 0, 0, 0, fmt.Errorf("%w: %s", ErrSKUduplicado, p.SKU)
			}
			lineas[i].Cantidad = round2(lineas[i].Cantidad + l.Cantidad)
			lineas[i].Total = round2(lineas[i].CostoUnitario * lineas[i].Cantidad)
			continue
		}
		idxPorSKU[p.SKU] = len(lineas)
		lineas = append(lineas, compra.Linea{
			ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre,
			Cantidad: l.Cantidad, CostoUnitario: l.CostoUnitario, CantidadRecibida: 0,
			Total: round2(l.CostoUnitario * l.Cantidad), Exento: exento,
		})
	}
	var subtotal, baseImponible float64
	for _, l := range lineas {
		subtotal += l.Total
		if !l.Exento {
			baseImponible += l.Total
		}
	}
	subtotal = round2(subtotal)
	iva := round2(round2(baseImponible) * s.alicuotaIVA(empresaID))
	total := round2(subtotal + iva)
	return lineas, subtotal, iva, total, nil
}

// CrearOrdenCompra arma una orden de compra en borrador con folio de la serie
// "OC". Valida que el proveedor exista y que haya al menos una línea.
func (s *Service) CrearOrdenCompra(empresaID, sedeID, actor, origen string, in EntradaOC) (compra.OrdenCompra, error) {
	if len(in.Lineas) == 0 {
		return compra.OrdenCompra{}, ErrOCVacia
	}
	prov, ok := s.provs.ByID(empresaID, in.ProveedorID)
	if !ok {
		return compra.OrdenCompra{}, ErrProveedorNoExiste
	}
	lineas, subtotal, iva, total, err := s.armarLineasOC(empresaID, in)
	if err != nil {
		return compra.OrdenCompra{}, err
	}
	// La orden se recibe en la sede indicada; si no viene, la del contexto.
	sede := in.SedeID
	if sede == "" {
		sede = sedeID
	}
	tasaActual, _ := s.TasaVigente(empresaID)
	o := compra.OrdenCompra{
		EmpresaID: empresaID, SedeID: sede,
		ProveedorID: prov.ID, ProveedorNombre: prov.Nombre,
		Serie: serieOrdenCompra, Estado: compra.OCBorrador,
		Lineas: lineas, Subtotal: subtotal, IVA: iva, Total: total,
		Moneda: empresa.MonedaVES, TasaCambio: tasaActual.Valor, TasaFuente: tasaActual.Fuente,
		CondicionesPago: in.CondicionesPago, Notas: in.Notas,
		Actor: actor, Creada: ahora(), Actualizada: ahora(),
	}
	o.Numero = s.numerador.Siguiente(empresaID, sede, serieOrdenCompra)
	o.NumeroCompleto = fmt.Sprintf("%s-%06d", serieOrdenCompra, o.Numero)

	out := s.ordenesCompra.Create(o)
	s.audit.Append(evento(empresaID, actor, origen, "compras.orden.crear", out.NumeroCompleto, out.ProveedorNombre))
	return out, nil
}

// ConfirmarOrdenCompra pasa un borrador a confirmada (queda lista para recibir).
func (s *Service) ConfirmarOrdenCompra(empresaID, id, actor, origen string) (compra.OrdenCompra, error) {
	o, ok := s.ordenesCompra.ByID(empresaID, id)
	if !ok {
		return compra.OrdenCompra{}, ErrOCNoExiste
	}
	if o.Estado != compra.OCBorrador {
		return compra.OrdenCompra{}, ErrOCEstado
	}
	o.Estado = compra.OCConfirmada
	o.Actualizada = ahora()
	out, ok := s.ordenesCompra.Update(o)
	if !ok {
		return compra.OrdenCompra{}, ErrOCNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.orden.confirmar", out.NumeroCompleto, out.Estado))
	return out, nil
}

// CancelarOrdenCompra marca una orden como cancelada. Se permite en borrador,
// confirmada o recibida_parcial. Si ya hubo recepción parcial, el stock ya
// recibido NO se revierte (esos movimientos quedan en el ledger): solo se
// cancela lo pendiente. Una orden ya recibida por completo no se cancela.
func (s *Service) CancelarOrdenCompra(empresaID, id, actor, origen, motivo string) (compra.OrdenCompra, error) {
	o, ok := s.ordenesCompra.ByID(empresaID, id)
	if !ok {
		return compra.OrdenCompra{}, ErrOCNoExiste
	}
	switch o.Estado {
	case compra.OCBorrador, compra.OCConfirmada, compra.OCRecibidaParcial:
		// permitido
	default:
		return compra.OrdenCompra{}, ErrOCEstado
	}
	o.Estado = compra.OCCancelada
	if motivo != "" {
		o.Notas = motivo
	}
	o.Actualizada = ahora()
	out, ok := s.ordenesCompra.Update(o)
	if !ok {
		return compra.OrdenCompra{}, ErrOCNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "compras.orden.cancelar", out.NumeroCompleto, motivo))
	return out, nil
}

// RecibirOrdenCompra recepciona (total o parcialmente) una orden confirmada:
// por cada {sku, cantidad} valida que no exceda lo pendiente de esa línea,
// anexa un movimiento de ENTRADA al ledger de inventario en la sede de la orden
// (al costo de la orden), y suma a CantidadRecibida. Deriva un asiento de compra
// por el costo recibido en ESTA recepción (Debe Inventario / Haber Cuentas por
// pagar); el IVA crédito fiscal se reconoce con la factura del proveedor, fuera
// de este alcance. Si todas las líneas quedan completas → recibida; si no →
// recibida_parcial. Solo se recibe desde confirmada o recibida_parcial.
func (s *Service) RecibirOrdenCompra(empresaID, id, actor, origen string, lineas []LineaRecepcion) (compra.OrdenCompra, error) {
	o, ok := s.ordenesCompra.ByID(empresaID, id)
	if !ok {
		return compra.OrdenCompra{}, ErrOCNoExiste
	}
	if o.Estado != compra.OCConfirmada && o.Estado != compra.OCRecibidaParcial {
		return compra.OrdenCompra{}, ErrOCEstado
	}
	// Una vez la orden tiene factura de compra registrada, esa factura fijó la deuda
	// del proveedor (CxP pasa a su Total): admitir recepciones posteriores haría que
	// el diario acumule más CxP que la proyección. La factura cierra la orden.
	if s.facturasCompra != nil {
		if _, facturada := s.facturasCompra.ByOrden(empresaID, id); facturada {
			return compra.OrdenCompra{}, ErrOrdenYaFacturada
		}
	}
	// Se piden cantidades > 0; una recepción sin nada que recibir no avanza nada.
	recibir := map[string]float64{}
	for _, l := range lineas {
		if l.Cantidad > 0 {
			recibir[l.SKU] += l.Cantidad
		}
	}
	if len(recibir) == 0 {
		return compra.OrdenCompra{}, ErrRecepcionVacia
	}
	// Índice de líneas por SKU para validar contra lo pendiente antes de tocar
	// el ledger: si una sola línea excede, no se recibe nada (todo o nada).
	idx := map[string]int{}
	for i, l := range o.Lineas {
		idx[l.SKU] = i
	}
	for sku, cant := range recibir {
		i, existe := idx[sku]
		if !existe {
			return compra.OrdenCompra{}, fmt.Errorf("%w: %s", ErrRecepcionExcede, sku)
		}
		// Guarda contra datos viejos: si la línea quedó apuntando a un combo, recibirla
		// anexaría stock fantasma para el SKU del paquete. Un combo no se recibe.
		if p, ok := s.productos.BySKU(empresaID, sku); ok && p.EsCombo {
			return compra.OrdenCompra{}, fmt.Errorf("%w: %s", ErrComboEnCompra, sku)
		}
		pendiente := o.Lineas[i].Cantidad - o.Lineas[i].CantidadRecibida
		if cant > pendiente+0.0001 {
			return compra.OrdenCompra{}, fmt.Errorf("%w: %s", ErrRecepcionExcede, sku)
		}
	}

	// Ya validado: se anexan los movimientos de entrada y se suma lo recibido. La
	// mercancía entra al almacén principal de la sede de la orden.
	almacenID := s.almacenParaEscritura(empresaID, o.SedeID, "")
	costoRecepcion := 0.0
	for sku, cant := range recibir {
		i := idx[sku]
		l := o.Lineas[i]
		s.movimientos.Append(inventario.Movimiento{
			EmpresaID: empresaID, SedeID: o.SedeID, AlmacenID: almacenID, ProductoID: l.ProductoID, SKU: l.SKU,
			Tipo: inventario.MovEntrada, Cantidad: cant, CostoUnitario: l.CostoUnitario,
			Motivo:  "recepción OC " + o.NumeroCompleto,
			RefTipo: "compra", RefID: o.ID, Actor: actor, Fecha: ahora(),
		})
		o.Lineas[i].CantidadRecibida = round2(l.CantidadRecibida + cant)
		costoRecepcion += cant * l.CostoUnitario
	}
	costoRecepcion = round2(costoRecepcion)

	// ¿Quedó todo recibido? Cada línea completa cuando lo recibido cubre lo pedido.
	completa := true
	for _, l := range o.Lineas {
		if l.CantidadRecibida+0.0001 < l.Cantidad {
			completa = false
			break
		}
	}
	if completa {
		o.Estado = compra.OCRecibida
	} else {
		o.Estado = compra.OCRecibidaParcial
	}
	o.Actualizada = ahora()
	out, ok := s.ordenesCompra.Update(o)
	if !ok {
		return compra.OrdenCompra{}, ErrOCNoExiste
	}

	// Asiento de compra por el costo recibido en esta recepción: la mercancía
	// entra al inventario contra la deuda con el proveedor. Sin IVA: el crédito
	// fiscal se reconoce con la factura del proveedor.
	s.asentarCompra(empresaID, actor, out, costoRecepcion)

	s.audit.Append(evento(empresaID, actor, origen, "compras.orden.recibir", out.NumeroCompleto, out.Estado))
	return out, nil
}

// asentarCompra registra el asiento derivado de una recepción de compra: el
// inventario sube y la cuenta por pagar al proveedor sube por el mismo monto.
func (s *Service) asentarCompra(empresaID, actor string, o compra.OrdenCompra, costo float64) {
	if s.asientos == nil || costo <= 0.004 {
		return
	}
	// La recepción ocurre AHORA (se registra al recibir la mercancía); "" ⇒ hoy.
	s.asentar(empresaID, actor, "", "Recepción de compra "+o.NumeroCompleto, "compra", o.ID, []contabilidad.Linea{
		{Codigo: contabilidad.CtaInventario, Debe: round2(costo)},
		{Codigo: contabilidad.CtaCuentasPorPagar, Haber: round2(costo)},
	})
}
