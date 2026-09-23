package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/fabricacion"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* FABRICACIÓN: convertir insumos en producto terminado.
 *
 * LA REGLA QUE SOSTIENE TODO: el costo del producto terminado se DERIVA de lo
 * que consumió, nunca se teclea. Un costo inventado vuelve inútiles el margen,
 * la valorización y el costo de ventas — que es justamente para lo que sirve
 * fabricar con un sistema y no con un cuaderno.
 *
 * DOS MOMENTOS, NO UNO:
 *
 *   · ARRANCAR consume los insumos. Salen del almacén de verdad, porque están en
 *     la mesa de trabajo y ya no los puede tomar otra orden. Si esperáramos al
 *     final, dos órdenes simultáneas podrían planificarse sobre el mismo kilo de
 *     harina y las dos "cabrían".
 *   · TERMINAR ingresa lo producido, con el costo de lo que salió repartido
 *     entre las unidades que REALMENTE salieron.
 *
 * Y LA CONTABILIDAD NO SE TOCA EN EL MEDIO. Fabricar no gasta ni gana: el valor
 * se mueve de una forma del inventario a otra. Por eso los movimientos de la
 * orden NO asientan costo de ventas —eso sería declarar un gasto que no
 * ocurrió— y solo se asienta cuando insumos y producto terminado viven en
 * cuentas de inventario DISTINTAS, que es cuando el valor sí cambió de cuenta.
 */

var (
	ErrFabricacionNoDisponible = errors.New("el módulo de fabricación no está disponible")
	ErrOrdenNoExiste           = errors.New("la orden de fabricación no existe")
	ErrTransicionOrden         = errors.New("esa orden no puede pasar a ese estado")
	ErrSinReceta               = errors.New("ese producto no tiene receta: no hay nada que fabricar")
	ErrCantidadInvalida        = errors.New("la cantidad a fabricar tiene que ser mayor que cero")
	ErrInsumoSinStock          = errors.New("no hay suficiente insumo para fabricar")
)

// RefFabricacion es el tipo de referencia con que la orden marca SUS movimientos
// del ledger. Es lo que permite que el backfill contable los reconozca y no los
// vuelva a asentar como una salida cualquiera.
const RefFabricacion = "fabricacion"

// ConFabricacion cablea el módulo. Opcional: sin él, ElERP se comporta como
// antes y el producto con receta solo se consume al venderse.
func (s *Service) ConFabricacion(r fabricacion.Repository) *Service {
	s.ordenesFabricacion = r
	return s
}

// EntradaOrden son los datos para planificar una orden.
type EntradaOrden struct {
	SedeID       string
	AlmacenID    string
	SKU          string
	Cantidad     float64
	Lote         string
	Vencimiento  string
	PesoUnitario float64
	OrigenTipo   string
	OrigenID     string
	Nota         string
	Actor        string
	Origen       string
}

// CrearOrdenFabricacion planifica una orden. No toca el inventario todavía.
func (s *Service) CrearOrdenFabricacion(empresaID string, in EntradaOrden) (fabricacion.Orden, error) {
	if s.ordenesFabricacion == nil {
		return fabricacion.Orden{}, ErrFabricacionNoDisponible
	}
	if in.Cantidad <= 0 {
		return fabricacion.Orden{}, ErrCantidadInvalida
	}
	p, ok := s.productos.BySKU(empresaID, strings.TrimSpace(in.SKU))
	if !ok {
		return fabricacion.Orden{}, ErrProductoNoExiste
	}
	if len(p.Receta) == 0 {
		return fabricacion.Orden{}, fmt.Errorf("%w: %s", ErrSinReceta, p.SKU)
	}
	/* SOLO SE FABRICA CON ORDEN LO QUE SE GUARDA.
	 *
	 * Un producto BAJO PEDIDO no se stockea: se prepara al venderlo y descuenta
	 * sus insumos ahí. Producirlo con una orden metería unidades al ledger que
	 * ninguna pantalla muestra —inventario invisible— y al venderlo se
	 * consumirían los insumos OTRA VEZ.
	 *
	 * Se niega con el nombre del interruptor, no con un «no se puede»: lo que hay
	 * que hacer es cambiarle el modo al producto, y decirlo ahorra el viaje. */
	if !p.SeFabricaParaStock() {
		return fabricacion.Orden{}, fmt.Errorf(
			"«%s» está configurado para fabricarse BAJO PEDIDO: se prepara al venderlo y no se guarda. "+
				"Para producirlo con una orden, cámbialo a «fabricar para stock» en el catálogo", p.Nombre)
	}
	o := fabricacion.Orden{
		EmpresaID: empresaID, SedeID: in.SedeID,
		AlmacenID:  s.almacenParaEscritura(empresaID, in.SedeID, in.AlmacenID),
		ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre, Cantidad: in.Cantidad,
		Estado: fabricacion.EstadoBorrador,
		Lote:   in.Lote, Vencimiento: in.Vencimiento, PesoUnitario: in.PesoUnitario,
		OrigenTipo: in.OrigenTipo, OrigenID: in.OrigenID, Nota: in.Nota,
		Creada: ahora(),
	}
	o.Normalizar()
	o.Numero = s.numerador.Siguiente(empresaID, in.SedeID, "OF")
	o.NumeroCompleto = fmt.Sprintf("OF-%06d", o.Numero)
	o.Bitacora = append(o.Bitacora, fabricacion.Evento{
		Cuando: o.Creada, Estado: o.Estado, Actor: in.Actor,
	})
	out := s.ordenesFabricacion.Append(o)
	s.audit.Append(evento(empresaID, in.Actor, in.Origen, "fabricacion.orden.crear", out.NumeroCompleto, out.SKU))
	return out, nil
}

/* PlanDeOrden es lo que la orden VA a consumir, con el costo de hoy.
 *
 * Se calcula para mostrarlo antes de arrancar: quien planifica tiene que poder
 * ver si le alcanza y cuánto va a costar, y descubrirlo al arrancar —cuando los
 * insumos ya salieron— es descubrirlo tarde.
 */
type PlanDeOrden struct {
	Consumos []fabricacion.Consumo `json:"consumos"`
	// Disponible es cuánto hay de cada insumo, en el mismo orden que Consumos.
	Disponible []float64 `json:"disponible"`
	// Faltantes nombra lo que no alcanza, en castellano y listo para mostrar.
	Faltantes     []string `json:"faltantes"`
	CostoTotal    float64  `json:"costoTotal"`
	CostoUnitario float64  `json:"costoUnitario"`
	Alcanza       bool     `json:"alcanza"`
	// Factor es por cuánto se multiplicó la receta: sale del tamaño de tanda y del
	// rendimiento. Se devuelve para que la pantalla pueda explicar de dónde salen
	// las cantidades en vez de mostrar números que no cuadran con la receta.
	Factor float64 `json:"factor"`
	// Esperado es cuánto producto terminado debería salir.
	Esperado float64 `json:"esperado"`
}

// PlanearOrden calcula los consumos y el costo de fabricar `cantidad` unidades.
func (s *Service) PlanearOrden(empresaID, sedeID, sku string, cantidad float64) (PlanDeOrden, error) {
	plan := PlanDeOrden{Consumos: []fabricacion.Consumo{}, Disponible: []float64{}, Faltantes: []string{}, Alcanza: true}
	p, ok := s.productos.BySKU(empresaID, strings.TrimSpace(sku))
	if !ok {
		return plan, ErrProductoNoExiste
	}
	if len(p.Receta) == 0 {
		return plan, fmt.Errorf("%w: %s", ErrSinReceta, p.SKU)
	}
	if cantidad <= 0 {
		return plan, ErrCantidadInvalida
	}
	/* EL FACTOR DE LA FÓRMULA, no una multiplicación simple.
	 *
	 * La receta está escrita para una TANDA (10 kg de masa, no 1), y el proceso
	 * RINDE menos de lo que entra (10 kg de pollo crudo dan 6,5 cocidos). Para
	 * obtener lo pedido hay que partir de más insumo, no de menos — y eso es
	 * justamente lo que un `cantidad × receta` se salta. */
	factor := p.FactorDeFormula(cantidad)
	plan.Factor = round2(factor)
	plan.Esperado = round2(cantidad)
	for _, comp := range p.Receta {
		ins, ok := s.productos.BySKU(empresaID, comp.SKU)
		if !ok {
			plan.Alcanza = false
			plan.Faltantes = append(plan.Faltantes, fmt.Sprintf("el insumo %s no está en el catálogo", comp.SKU))
			continue
		}
		hay, costo := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: ins.ID}))
		// Bruto: lo que hay que SACAR del almacén, contando lo que se pierde al
		// preparar ese insumo (pelado, limpieza, recorte).
		nec := round2(comp.CantidadBruta(factor))
		c := fabricacion.Consumo{
			SKU: ins.SKU, ProductoID: ins.ID, Nombre: ins.Nombre,
			Cantidad: nec, CostoUnitario: round2(costo),
		}
		plan.Consumos = append(plan.Consumos, c)
		plan.Disponible = append(plan.Disponible, round2(hay))
		plan.CostoTotal = round2(plan.CostoTotal + c.Total())
		if hay+0.0001 < nec {
			plan.Alcanza = false
			plan.Faltantes = append(plan.Faltantes,
				fmt.Sprintf("%s: hacen falta %.2f y hay %.2f", ins.Nombre, nec, hay))
		}
	}
	if cantidad > 0 {
		plan.CostoUnitario = round2(plan.CostoTotal / cantidad)
	}
	return plan, nil
}

/* IniciarOrden saca los insumos del almacén.
 *
 * Es el momento en que la orden toca el inventario por primera vez, y por eso
 * también el momento en que se congelan los consumos: la receta del catálogo
 * puede cambiar mañana y la orden tiene que poder explicar con qué se hizo lo
 * que se hizo.
 */
func (s *Service) IniciarOrden(empresaID, id, actor, origen string) (fabricacion.Orden, error) {
	o, err := s.ordenParaCambio(empresaID, id, fabricacion.EstadoEnProceso)
	if err != nil {
		return fabricacion.Orden{}, err
	}
	plan, err := s.PlanearOrden(empresaID, o.SedeID, o.SKU, o.Cantidad)
	if err != nil {
		return fabricacion.Orden{}, err
	}
	if !plan.Alcanza {
		// No se arranca lo que no se puede terminar: dejar salir insumos para una
		// orden que se va a trabar es perderlos de vista sin producir nada.
		return fabricacion.Orden{}, fmt.Errorf("%w: %s", ErrInsumoSinStock, strings.Join(plan.Faltantes, "; "))
	}
	for _, c := range plan.Consumos {
		s.movimientos.Append(inventario.Movimiento{
			EmpresaID: empresaID, SedeID: o.SedeID, AlmacenID: o.AlmacenID,
			ProductoID: c.ProductoID, SKU: c.SKU,
			Tipo: inventario.MovSalida, Cantidad: -c.Cantidad, CostoUnitario: c.CostoUnitario,
			Motivo:  "fabricación " + o.NumeroCompleto,
			RefTipo: RefFabricacion, RefID: o.ID,
			Actor: actor, Fecha: ahora(),
		})
	}
	o.Consumos = plan.Consumos
	o.CostoTotal = plan.CostoTotal
	o.Iniciada = ahora()
	o = s.marcarOrden(o, fabricacion.EstadoEnProceso, actor, "")
	out, _ := s.ordenesFabricacion.Update(o)
	s.audit.Append(evento(empresaID, actor, origen, "fabricacion.orden.iniciar", out.NumeroCompleto,
		fmt.Sprintf("%d insumo(s) por %.2f", len(out.Consumos), out.CostoTotal)))
	return out, nil
}

/* TerminarOrden ingresa lo producido al inventario.
 *
 * `producida` puede ser MENOR que lo planificado —de una masa para 20 panes
 * salen 18— y entonces el costo de los insumos se reparte entre las que
 * salieron: el pan bueno carga con el quemado, que es lo que de verdad costó.
 * Producir de más también se acepta: la merma sale negativa y queda a la vista.
 */
func (s *Service) TerminarOrden(empresaID, id string, producida float64, actor, origen string) (fabricacion.Orden, error) {
	o, err := s.ordenParaCambio(empresaID, id, fabricacion.EstadoTerminada)
	if err != nil {
		return fabricacion.Orden{}, err
	}
	if producida <= 0 {
		producida = o.Cantidad
	}
	o.CantidadProducida = round2(producida)
	/* ¿SE DESVIÓ DE LO ESPERADO? Hasta ahora la orden no podía saberlo: consumía
	 * exactamente lo que decía la fórmula, así que lo real ERA lo teórico por
	 * construcción y no había nada que comparar. Con el rendimiento declarado sí
	 * hay un esperado, y la desviación fuera de tolerancia queda marcada para que
	 * alguien mire — una tanda que rinde 20% menos no es mala suerte dos veces. */
	if p, ok := s.productos.ByID(empresaID, o.ProductoID); ok {
		o.FueraDeTolerancia = !p.DesviacionAceptable(o.Cantidad, o.CantidadProducida)
	}
	o.CostoUnitario = 0
	if o.CantidadProducida > 0 {
		o.CostoUnitario = round2(o.CostoTotal / o.CantidadProducida)
	}
	s.movimientos.Append(inventario.Movimiento{
		EmpresaID: empresaID, SedeID: o.SedeID, AlmacenID: o.AlmacenID,
		ProductoID: o.ProductoID, SKU: o.SKU,
		Tipo: inventario.MovEntrada, Cantidad: o.CantidadProducida, CostoUnitario: o.CostoUnitario,
		Lote: o.Lote, Vencimiento: o.Vencimiento,
		Motivo:  "fabricación " + o.NumeroCompleto,
		RefTipo: RefFabricacion, RefID: o.ID,
		Actor: actor, Fecha: ahora(),
	})
	o.Terminada = ahora()
	o = s.marcarOrden(o, fabricacion.EstadoTerminada, actor, "")
	out, _ := s.ordenesFabricacion.Update(o)
	s.asentarFabricacion(empresaID, actor, out)
	s.audit.Append(evento(empresaID, actor, origen, "fabricacion.orden.terminar", out.NumeroCompleto,
		fmt.Sprintf("%.2f × %s a %.2f c/u", out.CantidadProducida, out.SKU, out.CostoUnitario)))
	return out, nil
}

/* CancelarOrden la cierra. Si ya había consumido, DEVUELVE los insumos.
 *
 * Devolverlos y no dejarlos consumidos: lo que no se fabricó sigue en el
 * almacén, y darlo por gastado haría que el conteo físico no cuadre con el
 * sistema — que es la forma más rápida de que nadie confíe en el inventario.
 */
func (s *Service) CancelarOrden(empresaID, id, motivo, actor, origen string) (fabricacion.Orden, error) {
	o, err := s.ordenParaCambio(empresaID, id, fabricacion.EstadoCancelada)
	if err != nil {
		return fabricacion.Orden{}, err
	}
	if o.Estado == fabricacion.EstadoEnProceso {
		for _, c := range o.Consumos {
			s.movimientos.Append(inventario.Movimiento{
				EmpresaID: empresaID, SedeID: o.SedeID, AlmacenID: o.AlmacenID,
				ProductoID: c.ProductoID, SKU: c.SKU,
				Tipo: inventario.MovEntrada, Cantidad: c.Cantidad, CostoUnitario: c.CostoUnitario,
				Motivo:  "devolución por cancelar " + o.NumeroCompleto,
				RefTipo: RefFabricacion, RefID: o.ID,
				Actor: actor, Fecha: ahora(),
			})
		}
	}
	o = s.marcarOrden(o, fabricacion.EstadoCancelada, actor, motivo)
	out, _ := s.ordenesFabricacion.Update(o)
	s.audit.Append(evento(empresaID, actor, origen, "fabricacion.orden.cancelar", out.NumeroCompleto, motivo))
	return out, nil
}

// OrdenesFabricacion lista las órdenes de una sede, de la más nueva a la más vieja.
func (s *Service) OrdenesFabricacion(empresaID, sedeID string) []fabricacion.Orden {
	if s.ordenesFabricacion == nil {
		return []fabricacion.Orden{}
	}
	out := s.ordenesFabricacion.List(empresaID, sedeID)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// OrdenFabricacion devuelve una orden por su id.
func (s *Service) OrdenFabricacion(empresaID, id string) (fabricacion.Orden, bool) {
	if s.ordenesFabricacion == nil {
		return fabricacion.Orden{}, false
	}
	return s.ordenesFabricacion.ByID(empresaID, id)
}

func (s *Service) ordenParaCambio(empresaID, id, destino string) (fabricacion.Orden, error) {
	if s.ordenesFabricacion == nil {
		return fabricacion.Orden{}, ErrFabricacionNoDisponible
	}
	o, ok := s.ordenesFabricacion.ByID(empresaID, id)
	if !ok {
		return fabricacion.Orden{}, ErrOrdenNoExiste
	}
	if !fabricacion.PuedePasarA(o.Estado, destino) {
		return fabricacion.Orden{}, fmt.Errorf("%w: de %q a %q", ErrTransicionOrden, o.Estado, destino)
	}
	return o, nil
}

func (s *Service) marcarOrden(o fabricacion.Orden, estado, actor, nota string) fabricacion.Orden {
	o.Estado = estado
	o.Bitacora = append(o.Bitacora, fabricacion.Evento{
		Cuando: ahora(), Estado: estado, Actor: actor, Nota: nota,
	})
	return o
}

/* asentarFabricacion mueve el valor entre cuentas de inventario.
 *
 * FABRICAR NO ES UN GASTO NI UNA GANANCIA: el valor sale de los insumos y entra
 * al producto terminado, y el patrimonio no cambia. Por eso acá NO se asienta
 * costo de ventas —eso declararía un gasto que no ocurrió— y solo se asienta
 * cuando insumos y producto terminado viven en cuentas DISTINTAS, que es cuando
 * el valor sí cambió de sitio.
 *
 * Con la misma cuenta para todo (el caso normal) el asiento sería debe y haber
 * sobre la misma línea: cero información y un libro más largo.
 */
func (s *Service) asentarFabricacion(empresaID, actor string, o fabricacion.Orden) {
	if s.asientos == nil || o.CostoTotal <= 0.004 {
		return
	}
	destino := s.cuentaInventarioDe(empresaID, o.ProductoID)
	salidas := map[string]float64{}
	for _, c := range o.Consumos {
		cta := s.cuentaInventarioDe(empresaID, c.ProductoID)
		if cta == destino {
			continue // mismo bolsillo: no hay nada que mover
		}
		salidas[cta] = round2(salidas[cta] + c.Total())
	}
	if len(salidas) == 0 {
		return
	}
	lineas := []contabilidad.Linea{}
	total := 0.0
	for cta, monto := range salidas {
		lineas = append(lineas, contabilidad.Linea{Codigo: cta, Haber: monto})
		total = round2(total + monto)
	}
	lineas = append([]contabilidad.Linea{{Codigo: destino, Debe: total}}, lineas...)
	s.asentar(empresaID, actor, o.Terminada,
		"Fabricación "+o.NumeroCompleto+" — "+o.Nombre, RefFabricacion, o.ID, lineas)
}
