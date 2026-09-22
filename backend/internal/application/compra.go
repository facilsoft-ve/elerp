package application

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/almacen"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/proveedor"
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

// RetencionesOCEntrada permite AJUSTAR, para esta orden concreta, el perfil de
// retenciones que trae el proveedor. Nil ⇒ se usa el perfil del proveedor tal cual.
// Existe porque el concepto o el porcentaje de una compra puntual pueden no ser los
// habituales del proveedor (un mismo proveedor factura honorarios un mes y un flete
// al siguiente), y quien arma la orden es quien lo sabe.
type RetencionesOCEntrada struct {
	RetieneIVA    bool
	IVAPorcentaje float64
	RetieneISLR   bool
	// ISLRConceptoCodigo y ISLRSujeto resuelven la tarifa contra el MAESTRO de
	// conceptos. No se teclea porcentaje: el maestro es la única fuente de tarifas,
	// y el comprobante las copia al emitirse.
	ISLRConceptoCodigo string
	ISLRSujeto         string
}

// EntradaOC son los datos para crear una orden de compra.
type EntradaOC struct {
	ProveedorID     string
	SedeID          string
	CondicionesPago string
	Notas           string
	Lineas          []LineaOCEntrada
	// Retenciones ajusta el perfil del proveedor solo para esta orden. Nil ⇒ perfil
	// del proveedor.
	Retenciones *RetencionesOCEntrada
}

// porcentajeRetencionIVA resuelve el % a retener de IVA: el del proveedor, si no el
// por defecto de la empresa, si no el 75 % de la providencia. Misma escalera que
// alicuotaIVA (config del caso > config de la empresa > default del sistema).
const porcentajeRetencionIVADefault = 75.0

func (s *Service) porcentajeRetencionIVA(empresaID string, delProveedor float64) float64 {
	if delProveedor > 0 {
		return delProveedor
	}
	if e, ok := s.empresas.ByID(empresaID); ok && e.RetencionIVAPorcentaje > 0 {
		return e.RetencionIVAPorcentaje
	}
	return porcentajeRetencionIVADefault
}

// basesISLRPorConcepto agrupa el NETO de las líneas por concepto de retención,
// devolviendo los códigos ordenados y su base. El orden es alfabético y no el de
// las líneas a propósito: el desglose de dos órdenes con los mismos conceptos
// tiene que salir igual, y un mapa de Go no garantiza ningún orden.
//
// Las líneas sin concepto (toda mercancía) no entran en ninguna base: sobre la
// compra de un producto no se retiene ISLR.
func basesISLRPorConcepto(lineas []compra.Linea) ([]string, map[string]float64) {
	bases := map[string]float64{}
	codigos := []string{}
	for _, l := range lineas {
		cod := strings.ToLower(strings.TrimSpace(l.ConceptoISLR))
		if cod == "" {
			continue
		}
		if _, visto := bases[cod]; !visto {
			codigos = append(codigos, cod)
		}
		bases[cod] = round2(bases[cod] + l.Total)
	}
	sort.Strings(codigos)
	return codigos, bases
}

// proyectarRetencionesOC calcula lo que se le retendrá al proveedor cuando llegue su
// factura, y por tanto el NETO que se le va a pagar. Es una proyección que se graba
// en la orden; el comprobante que vale ante el SENIAT se emite después, sobre la
// factura de compra.
//
// Dos condiciones tienen que darse para que se retenga: la EMPRESA debe ser agente
// de retención de ese impuesto (Configuración › Impuestos) y el PROVEEDOR tenerlo
// activado. Si falta cualquiera, el monto es 0 — no se inventa una retención que
// después nadie podría emitir.
//
// DE DÓNDE SALE EL CONCEPTO. El ISLR se retiene por el CONCEPTO DEL PAGO —qué se
// está pagando—, así que lo declara el PRODUCTO (inventario.Producto.ConceptoISLR)
// y se retiene línea por línea, agrupando por concepto. El concepto del perfil del
// proveedor es el RESPALDO para cuando ninguna línea lo declara: servicios que no
// están en el catálogo, o catálogos todavía sin clasificar.
//
// El perfil del proveedor (y su ajuste por orden) sigue mandando en lo suyo: si se
// retiene o no, y el TIPO DE SUJETO, que es de quien cobra y no de lo que se compra.
// Por eso un ajuste por orden no puede reclasificar un servicio que el catálogo ya
// clasificó: para eso se corrige la ficha del producto, que es donde vive.
//
// Bases: el IVA se retiene sobre el IVA de la orden; el ISLR, sobre el neto de las
// líneas de cada concepto, que es el ingreso del proveedor. El cálculo lo hace el
// propio concepto (fiscal.ConceptoISLR.Retener), que ya sabe de base mínima y de
// sustraendo: si la proyección lo repitiera por su cuenta, tarde o temprano diría un
// número distinto al del comprobante que se emite después.
func (s *Service) proyectarRetencionesOC(empresaID string, prov proveedor.Proveedor, in *RetencionesOCEntrada,
	lineas []compra.Linea, subtotal, iva float64) (o compra.OrdenCompra) {
	perfil := RetencionesOCEntrada{
		RetieneIVA: prov.RetieneIVA, IVAPorcentaje: prov.RetencionIVAPorcentaje,
		RetieneISLR:        prov.RetieneISLR,
		ISLRConceptoCodigo: prov.ConceptoISLRCodigo, ISLRSujeto: prov.SujetoISLR,
	}
	if in != nil {
		perfil = *in
	}
	emp, _ := s.empresas.ByID(empresaID)

	if emp.AgenteRetencionIVA && perfil.RetieneIVA && iva > 0.004 {
		o.RetencionIVAPorcentaje = s.porcentajeRetencionIVA(empresaID, perfil.IVAPorcentaje)
		o.RetencionIVAMonto = round2(iva * o.RetencionIVAPorcentaje / 100)
	}
	if !emp.AgenteRetencionISLR || !perfil.RetieneISLR {
		return o
	}

	maestro := s.ConceptosISLR(empresaID)
	// La UT del día de la orden: de ella salen el sustraendo y el mínimo en
	// bolívares. Se resuelve UNA vez para toda la orden, para que dos conceptos de
	// la misma no puedan quedar valorados con UT distintas.
	valorUT := s.ValorUTEn(empresaID, "")
	hoy := ahora()[:10]
	codigos, bases := basesISLRPorConcepto(lineas)
	if len(codigos) == 0 {
		// Respaldo: ninguna línea declara concepto ⇒ el del proveedor sobre el neto
		// completo, que es como funcionaba antes de que el producto lo clasificara.
		cod := strings.ToLower(strings.TrimSpace(perfil.ISLRConceptoCodigo))
		if cod == "" || subtotal <= 0.004 {
			return o
		}
		codigos, bases = []string{cod}, map[string]float64{cod: subtotal}
	}

	for _, cod := range codigos {
		base := bases[cod]
		if base <= 0.004 {
			continue
		}
		// El TRAMO de la escala lo decide cuánto se le lleva pagado al proveedor en el
		// ejercicio por este concepto, con esta orden dentro. Se resuelve en dos pasos
		// porque la porción gravable depende del concepto y el concepto del acumulado.
		primero, _ := fiscal.ConceptoPara(maestro, cod, perfil.ISLRSujeto, 0)
		acumulado := s.AcumuladoISLRUT(empresaID, prov.ID, cod, hoy, "") +
			baseEnUT(primero.BaseGravable(base), valorUT)

		c, ok := fiscal.ConceptoPara(maestro, cod, perfil.ISLRSujeto, acumulado)
		if !ok {
			// El maestro conoce el concepto pero no tiene tarifa para ESTE tipo de
			// sujeto (un flete comprado a una persona natural cuando la tabla solo trae
			// la de jurídica). Se deja constancia con monto 0 en vez de omitir la fila:
			// callarlo haría que una tabla incompleta se viera exactamente igual que
			// «a este proveedor no se le retiene», y nadie iría a buscarla.
			nombre := fiscal.NombreDeConcepto(maestro, cod)
			if nombre == "" {
				nombre = cod
			}
			o.RetencionISLRDetalle = append(o.RetencionISLRDetalle, compra.RetencionISLRProyectada{
				Codigo: cod, Concepto: nombre, Base: base,
				Impedimento: compra.ImpedimentoSinTarifa,
			})
			continue
		}
		if c.RequiereUT() && valorUT <= 0 {
			// El concepto tiene sustraendo o mínimo en unidades tributarias y no hay UT
			// cargada. Seguir con la UT en cero anularía el sustraendo y retendría DE
			// MÁS, sin que fallara nada: es justo el error que hay que hacer visible.
			o.RetencionISLRDetalle = append(o.RetencionISLRDetalle, compra.RetencionISLRProyectada{
				Codigo: c.Codigo, Concepto: c.Nombre, Base: base, Porcentaje: c.Porcentaje,
				Impedimento: compra.ImpedimentoSinUT,
			})
			continue
		}
		monto := round2(c.Retener(base, valorUT))
		o.RetencionISLRDetalle = append(o.RetencionISLRDetalle, compra.RetencionISLRProyectada{
			Codigo: c.Codigo, Concepto: c.Nombre,
			// La base que se informa es la GRAVABLE: en los conceptos que no retienen
			// sobre todo el pago, el neto de las líneas no explica el monto.
			Base:       round2(c.BaseGravable(base)),
			Porcentaje: c.Porcentaje, Sustraendo: round2(c.SustraendoEn(valorUT)), Monto: monto,
		})
		o.RetencionISLRMonto = round2(o.RetencionISLRMonto + monto)
	}

	// Escalares: describen el caso de UN concepto, que es el habitual. Con varios,
	// solo el monto (la suma) y la lista de nombres significan algo — un porcentaje
	// único de una mezcla de tarifas sería un número que nadie podría declarar.
	switch len(o.RetencionISLRDetalle) {
	case 0:
	case 1:
		d := o.RetencionISLRDetalle[0]
		o.RetencionISLRConcepto = d.Concepto
		o.RetencionISLRPorcentaje, o.RetencionISLRSustraendo = d.Porcentaje, d.Sustraendo
	default:
		nombres := make([]string, 0, len(o.RetencionISLRDetalle))
		for _, d := range o.RetencionISLRDetalle {
			nombres = append(nombres, d.Concepto)
		}
		o.RetencionISLRConcepto = strings.Join(nombres, " · ")
	}
	return o
}

// LineaRecepcion indica cuánto se recibe de un SKU en una recepción concreta.
type LineaRecepcion struct {
	SKU      string
	Cantidad float64
	// Lote y Vencimiento solo los exigen los productos que llevan trazabilidad. Se
	// piden al RECIBIR porque es el único momento en que alguien tiene la caja
	// delante con la etiqueta: preguntarlo después es pedir que lo inventen.
	Lote        string
	Vencimiento string
	// UbicacionID ubica lo recibido dentro del almacén. Vacío = el almacén sin más
	// detalle, que es lo correcto mientras nadie lo haya dividido.
	UbicacionID string
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
			// El concepto de ISLR se copia del catálogo: es lo que hace que la
			// retención salga sola en vez de teclearse. Vacío en toda mercancía.
			ConceptoISLR: p.ConceptoISLR,
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
	// Condiciones de pago: las de la orden, y si no vienen, las pactadas con el
	// proveedor. Se COPIAN a la orden: cambiar el maestro mañana no reescribe lo
	// que ya se pactó en una orden vieja.
	condiciones := strings.TrimSpace(in.CondicionesPago)
	if condiciones == "" {
		condiciones = strings.TrimSpace(prov.CondicionPago)
	}

	tasaActual, _ := s.TasaVigente(empresaID)
	o := compra.OrdenCompra{
		EmpresaID: empresaID, SedeID: sede,
		ProveedorID: prov.ID, ProveedorNombre: prov.Nombre,
		Serie: serieOrdenCompra, Estado: compra.OCBorrador,
		Lineas: lineas, Subtotal: subtotal, IVA: iva, Total: total,
		Moneda: empresa.MonedaVES, TasaCambio: tasaActual.Valor, TasaFuente: tasaActual.Fuente,
		CondicionesPago: condiciones, Notas: in.Notas,
		Actor: actor, Creada: ahora(), Actualizada: ahora(),
	}
	// Retenciones proyectadas: cuánto se le va a pagar de verdad al proveedor.
	ret := s.proyectarRetencionesOC(empresaID, prov, in.Retenciones, lineas, subtotal, iva)
	o.RetencionIVAPorcentaje, o.RetencionIVAMonto = ret.RetencionIVAPorcentaje, ret.RetencionIVAMonto
	o.RetencionISLRConcepto, o.RetencionISLRPorcentaje = ret.RetencionISLRConcepto, ret.RetencionISLRPorcentaje
	o.RetencionISLRSustraendo, o.RetencionISLRMonto = ret.RetencionISLRSustraendo, ret.RetencionISLRMonto
	o.RetencionISLRDetalle = ret.RetencionISLRDetalle
	// Los tres importes ya vienen redondeados a dos decimales: restarlos no necesita
	// otro round2, que además truncaría hacia cero si el neto diera negativo.
	o.NetoAPagar = total - o.RetencionIVAMonto - o.RetencionISLRMonto
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
	// Se agrega por (SKU, LOTE): un mismo producto puede llegar repartido en varios
	// lotes en la MISMA entrega, que es lo normal cuando el proveedor completa un
	// pedido con lo que tiene. Agregar solo por SKU obligaría a partir la recepción
	// en dos, o —peor— haría que un lote pisara al otro sin decirlo.
	type claveRecepcion struct{ SKU, Lote string }
	recibir := map[string]float64{}         // total por SKU, para validar contra lo pendiente
	porLote := map[claveRecepcion]float64{} // lo que entra a cada lote
	vencs := map[claveRecepcion]string{}
	ubis := map[claveRecepcion]string{}
	orden := []claveRecepcion{}
	for _, l := range lineas {
		if l.Cantidad <= 0 {
			continue
		}
		k := claveRecepcion{SKU: l.SKU, Lote: strings.TrimSpace(l.Lote)}
		if _, visto := porLote[k]; !visto {
			orden = append(orden, k)
		}
		recibir[l.SKU] += l.Cantidad
		porLote[k] += l.Cantidad
		if v := strings.TrimSpace(l.Vencimiento); v != "" {
			vencs[k] = v
		}
		if u := strings.TrimSpace(l.UbicacionID); u != "" {
			ubis[k] = u
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

	// Ya validado: se anexan los movimientos de entrada y se suma lo recibido.
	//
	// A DÓNDE ENTRA lo decide el tipo de operación de recepción, si hay uno
	// configurado. Sin él —que es el caso de toda empresa que no haya tocado esa
	// pantalla— entra al almacén principal de la sede, exactamente como antes.
	//
	// Con DOS PASOS, todo lo que llega aterriza en la ubicación intermedia (el
	// muelle) sin importar la que pidiera la línea: el sentido de los dos pasos es
	// precisamente que nadie decide dónde va la mercancía hasta haberla revisado.
	// Colocarla ya en su sitio y llamarlo «dos pasos» sería un paso con un rodeo.
	almacenID := s.almacenParaEscritura(empresaID, o.SedeID, "")
	// Desglose por producto de ESTA recepción, para el asiento. Se arma aquí y no
	// se deduce después del ledger porque una orden admite varias recepciones: leer
	// los movimientos de la orden daría el acumulado de todas, y este asiento es
	// solo de la que se está registrando.
	costoPorProducto := montoPorProducto{}
	enDosPasos := false
	muelle := ""
	if op, hay := s.OperacionPara(empresaID, o.SedeID, almacen.ClaseRecepcion); hay {
		if op.AlmacenID != "" {
			almacenID = s.almacenParaEscritura(empresaID, o.SedeID, op.AlmacenID)
		}
		if op.Pasos == 2 {
			enDosPasos, muelle = true, op.UbicacionIntermediaID
		}
	}
	costoRecepcion := 0.0
	// Se valida TODO antes de anexar el primer movimiento: el ledger es de solo
	// anexado, y fallar a medias dejaría media recepción registrada sin vuelta atrás.
	for _, k := range orden {
		prod, _ := s.productos.BySKU(empresaID, k.SKU)
		if _, _, err := validarLoteDeEntrada(prod, k.Lote, vencs[k]); err != nil {
			return compra.OrdenCompra{}, err
		}
	}
	for _, k := range orden {
		i := idx[k.SKU]
		l := o.Lineas[i]
		prod, _ := s.productos.BySKU(empresaID, k.SKU)
		lote, venc, _ := validarLoteDeEntrada(prod, k.Lote, vencs[k])
		cant := porLote[k]
		s.movimientos.Append(inventario.Movimiento{
			EmpresaID: empresaID, SedeID: o.SedeID, AlmacenID: almacenID, ProductoID: l.ProductoID, SKU: l.SKU,
			Tipo: inventario.MovEntrada, Cantidad: cant, CostoUnitario: l.CostoUnitario,
			Lote: lote, Vencimiento: venc,
			UbicacionID: s.ubicacionDeRecepcion(empresaID, almacenID, ubis[k], enDosPasos, muelle),
			Motivo:      "recepción OC " + o.NumeroCompleto,
			RefTipo:     "compra", RefID: o.ID, Actor: actor, Fecha: ahora(),
		})
		o.Lineas[i].CantidadRecibida = round2(o.Lineas[i].CantidadRecibida + cant)
		costoRecepcion += cant * l.CostoUnitario
		costoPorProducto[l.ProductoID] += cant * l.CostoUnitario
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
	s.asentarCompra(empresaID, actor, out, costoRecepcion, costoPorProducto)

	s.audit.Append(evento(empresaID, actor, origen, "compras.orden.recibir", out.NumeroCompleto, out.Estado))
	return out, nil
}

// asentarCompra registra el asiento derivado de una recepción de compra: el
// inventario sube y la cuenta por pagar al proveedor sube por el mismo monto.
func (s *Service) asentarCompra(empresaID, actor string, o compra.OrdenCompra, costo float64, porProducto montoPorProducto) {
	if s.asientos == nil || costo <= 0.004 {
		return
	}
	// La mercancía entra a la cuenta del RUBRO de cada producto (ver cuenta_rubro.go).
	// Sin rubros con cuenta propia sale una sola línea, idéntica a la de antes.
	inv := s.lineasDeInventario(empresaID, porProducto, true)
	if len(inv) == 0 {
		inv = []contabilidad.Linea{{Codigo: contabilidad.CtaInventario, Debe: round2(costo)}}
	}
	// La recepción ocurre AHORA (se registra al recibir la mercancía); "" ⇒ hoy.
	s.asentar(empresaID, actor, "", "Recepción de compra "+o.NumeroCompleto, "compra", o.ID, append(
		inv,
		contabilidad.Linea{Codigo: contabilidad.CtaCuentasPorPagar, Haber: round2(costo)},
	))
}
