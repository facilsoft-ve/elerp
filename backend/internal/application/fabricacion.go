package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/almacen"
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
	return s.CerrarOrden(empresaID, id, actor, origen, CierreOrden{Producida: producida})
}

/* CierreOrden es el REPORTE DEL RESULTADO: cuánto salió bien y qué pasó con lo
 * demás. No es una cifra sino un reparto — de una tanda de 15 pueden salir 10
 * buenos, 3 perdidos y 2 para reprocesar.
 */
type CierreOrden struct {
	Producida  float64
	Resultados []fabricacion.Resultado
}

/* CerrarOrden reporta el resultado de la fabricación.
 *
 * TRES DECISIONES QUE PARECEN CONTABLES Y SON DE NEGOCIO:
 *
 *  1. Solo lo BUENO entra al inventario a su costo. Lo demás no es producto.
 *  2. Lo que sigue EXISTIENDO —descarte y reproceso— entra al almacén de
 *     descarte a COSTO CERO. Su valor ya se reconoció como pérdida; lo que hace
 *     falta es poder contarlo, porque mientras esté ahí el conteo físico tiene
 *     que cuadrar. Valorar lo que no se sabe si servirá sería inventar un activo.
 *  3. La merma DENTRO de la tolerancia la cargan los buenos —el pan bueno carga
 *     con el quemado, y eso es lo que de verdad costó—. La que se PASA, no:
 *     inflaría el costo del producto y escondería el problema en el margen.
 */
func (s *Service) CerrarOrden(empresaID, id, actor, origen string, in CierreOrden) (fabricacion.Orden, error) {
	o, err := s.ordenParaCambio(empresaID, id, fabricacion.EstadoTerminada)
	if err != nil {
		return fabricacion.Orden{}, err
	}
	producida := in.Producida
	if producida < 0 {
		producida = 0
	}
	/* CERO ES UNA RESPUESTA VÁLIDA: la tanda se perdió entera. Antes cero se leía
	 * como «no declaró nada» y se sustituía por lo planificado, así que una pérdida
	 * total quedaba registrada como producción completa — al revés de lo que pasó. */
	for i, r := range in.Resultados {
		if r.Cantidad <= 0 {
			continue
		}
		if !fabricacion.DestinoValido(r.Destino) {
			return fabricacion.Orden{}, fmt.Errorf("destino desconocido para lo que no salió bien: %q", r.Destino)
		}
		if strings.TrimSpace(r.Motivo) == "" {
			return fabricacion.Orden{}, errors.New("hace falta decir qué pasó: un desperdicio sin explicación no sirve para decidir nada")
		}
		in.Resultados[i].Cantidad = round2(r.Cantidad)
		in.Resultados[i].Motivo = strings.TrimSpace(r.Motivo)
		o.Resultados = append(o.Resultados, in.Resultados[i])
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
	/* EL COSTO UNITARIO, y dónde cae lo que los buenos no deben cargar. */
	absorbible := o.CantidadProducida
	if p, ok := s.productos.ByID(empresaID, o.ProductoID); ok && o.FueraDeTolerancia && p.ToleranciaPct > 0 {
		if normal := round2(o.Cantidad * (1 - p.ToleranciaPct/100)); normal > absorbible {
			absorbible = normal
		}
	}
	o.CostoUnitario, o.PerdidaAnormal = 0, 0
	if absorbible > 0 {
		o.CostoUnitario = round2(o.CostoTotal / absorbible)
		o.PerdidaAnormal = round2(o.CostoTotal - o.CostoUnitario*o.CantidadProducida)
	} else {
		// Nada salió bien: el costo entero es pérdida. No hay producto que lo cargue.
		o.PerdidaAnormal = o.CostoTotal
	}
	if o.PerdidaAnormal < 0.005 {
		o.PerdidaAnormal = 0
	}
	// Solo entra al inventario lo que salió BIEN. Una tanda perdida entera no
	// ingresa nada: su costo ya quedó como pérdida.
	if o.CantidadProducida > 0 {
		s.movimientos.Append(inventario.Movimiento{
			EmpresaID: empresaID, SedeID: o.SedeID, AlmacenID: o.AlmacenID,
			ProductoID: o.ProductoID, SKU: o.SKU,
			Tipo: inventario.MovEntrada, Cantidad: o.CantidadProducida, CostoUnitario: o.CostoUnitario,
			Lote: o.Lote, Vencimiento: o.Vencimiento,
			Motivo:  "fabricación " + o.NumeroCompleto,
			RefTipo: RefFabricacion, RefID: o.ID,
			Actor: actor, Fecha: ahora(),
		})
	}
	/* LO QUE SIGUE EXISTIENDO va al almacén de descarte, a costo cero. Sin un
	 * almacén de descarte no se mueve nada: queda el registro en la orden, que es
	 * mejor que meter mercancía inservible en el almacén bueno. */
	if alm := s.almacenDeDescarte(empresaID, o.SedeID); alm != "" {
		for _, r := range o.Resultados {
			if r.Destino == fabricacion.DestinoPerdida || r.Cantidad <= 0 {
				continue // no queda nada que guardar
			}
			s.movimientos.Append(inventario.Movimiento{
				EmpresaID: empresaID, SedeID: o.SedeID, AlmacenID: alm,
				ProductoID: o.ProductoID, SKU: o.SKU,
				Tipo: inventario.MovEntrada, Cantidad: r.Cantidad, CostoUnitario: 0,
				Lote: o.Lote, Vencimiento: o.Vencimiento,
				Motivo:  r.Destino + " de " + o.NumeroCompleto + " — " + r.Motivo,
				RefTipo: RefFabricacion, RefID: o.ID,
				Actor: actor, Fecha: ahora(),
			})
		}
	}
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
	/* LA PÉRDIDA ANORMAL SALE DEL INVENTARIO Y VA A RESULTADOS.
	 *
	 * Es valor que se consumió y no quedó en ningún producto. Dejarlo dentro del
	 * inventario lo dejaría contando mercancía que no existe; cargárselo a los
	 * buenos inflaría su costo. Va a costo del período, que es donde se ve. */
	if o.PerdidaAnormal > 0.004 {
		origenPerdida := destino
		for _, c := range o.Consumos {
			if cta := s.cuentaInventarioDe(empresaID, c.ProductoID); cta != "" {
				origenPerdida = cta
				break
			}
		}
		s.asentar(empresaID, actor, o.Terminada,
			"Merma anormal de fabricación "+o.NumeroCompleto+" — "+o.Nombre, RefFabricacion, o.ID,
			[]contabilidad.Linea{
				{Codigo: contabilidad.CtaCostoDeVentas, Debe: o.PerdidaAnormal},
				{Codigo: origenPerdida, Haber: o.PerdidaAnormal},
			})
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

// almacenDeDescarte busca el almacén donde va lo que ya no se puede vender.
// Vacío si la empresa no tiene ninguno: entonces solo queda el registro en la
// orden, que es preferible a meter mercancía inservible en el almacén bueno.
func (s *Service) almacenDeDescarte(empresaID, sedeID string) string {
	if s.almacenes == nil {
		return ""
	}
	for _, a := range s.almacenes.List(empresaID) {
		if a.Tipo == almacen.TipoDescarte && a.Activo && (a.SedeID == sedeID || a.SedeID == "") {
			return a.ID
		}
	}
	return ""
}

/* ResumenFabricacion es lo que pasó en el taller durante un período.
 *
 * Existe porque orden por orden no se ve lo que importa: una tanda que pierde
 * tres unidades es mala suerte, veinte tandas perdiendo tres cada una es un
 * problema del proceso. Y el número que nadie mira hasta que duele es cuánto
 * costó lo que NO se vendió — la pérdida anormal del período, que orden por
 * orden aparece como cifras chicas y juntas es una línea del estado de
 * resultados.
 */
type ResumenFabricacion struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`

	Ordenes    int `json:"ordenes"`
	Terminadas int `json:"terminadas"`
	// EnCurso son las que todavía tienen insumos afuera: mercancía comprometida
	// que ya no está en el almacén y todavía no es producto.
	EnCurso    int `json:"enCurso"`
	Canceladas int `json:"canceladas"`

	Producido   float64 `json:"producido"`
	Perdido     float64 `json:"perdido"`
	Descartado  float64 `json:"descartado"`
	Reprocesado float64 `json:"reprocesado"`
	// Planificado es lo que se esperaba producir EN LAS TANDAS TERMINADAS; la
	// diferencia contra Producido es el rendimiento real del taller. Las que
	// siguen en curso no cuentan acá: todavía no tuvieron su oportunidad.
	Planificado float64 `json:"planificado"`
	// EnProceso es lo planificado que sigue en el taller, con su costo ya salido
	// del almacén: mercancía comprometida que todavía no es producto ni pérdida.
	EnProceso      float64 `json:"enProceso"`
	CostoEnProceso float64 `json:"costoEnProceso"`

	CostoInsumos   float64 `json:"costoInsumos"`
	ValorProducido float64 `json:"valorProducido"`
	PerdidaAnormal float64 `json:"perdidaAnormal"`
	// FueraDeTolerancia son las tandas que se salieron de lo que su fórmula
	// declara normal. Es la lista corta que alguien tiene que mirar.
	FueraDeTolerancia int `json:"fueraDeTolerancia"`

	PorProducto []ResumenProducto `json:"porProducto"`
	// Motivos agrupa POR QUÉ se perdió, que es lo único que permite arreglarlo.
	Motivos []MotivoResumen `json:"motivos"`
}

// ResumenProducto es la fila de un producto dentro del resumen.
type ResumenProducto struct {
	SKU         string  `json:"sku"`
	Nombre      string  `json:"nombre"`
	Ordenes     int     `json:"ordenes"`
	Planificado float64 `json:"planificado"`
	Producido   float64 `json:"producido"`
	NoLogrado   float64 `json:"noLogrado"`
	// RendimientoPct es lo producido sobre lo planificado: el rendimiento REAL,
	// contra el que la fórmula declara.
	RendimientoPct float64 `json:"rendimientoPct"`
	CostoInsumos   float64 `json:"costoInsumos"`
	PerdidaAnormal float64 `json:"perdidaAnormal"`
}

// MotivoResumen agrupa lo no logrado por su explicación.
type MotivoResumen struct {
	Motivo   string  `json:"motivo"`
	Destino  string  `json:"destino"`
	Cantidad float64 `json:"cantidad"`
	Veces    int     `json:"veces"`
}

// ResumenDeFabricacion agrega las órdenes de una sede en un rango de fechas.
// Rango vacío = todo lo que haya.
func (s *Service) ResumenDeFabricacion(empresaID, sedeID, desde, hasta string) ResumenFabricacion {
	res := ResumenFabricacion{
		Desde: desde, Hasta: hasta,
		PorProducto: []ResumenProducto{}, Motivos: []MotivoResumen{},
	}
	if s.ordenesFabricacion == nil {
		return res
	}
	porProd := map[string]*ResumenProducto{}
	orden := []string{}
	porMotivo := map[string]*MotivoResumen{}
	ordenMotivo := []string{}

	for _, o := range s.ordenesFabricacion.List(empresaID, sedeID) {
		// El rango se mide por cuándo se CREÓ: es cuando se decidió producir, y es
		// la fecha con la que quien planifica piensa el período.
		if desde != "" && o.Creada < desde {
			continue
		}
		if hasta != "" && o.Creada > hasta+"T23:59:59Z" {
			continue
		}
		res.Ordenes++
		switch o.Estado {
		case fabricacion.EstadoTerminada:
			res.Terminadas++
		case fabricacion.EstadoCancelada:
			res.Canceladas++
		default:
			res.EnCurso++
		}
		// Una cancelada devolvió sus insumos: no produjo ni perdió nada.
		if o.Estado == fabricacion.EstadoCancelada {
			continue
		}
		// Una tanda en curso todavía no produjo: si su cantidad planificada entrara
		// en el rendimiento, el resumen diría que se perdió lo que aún se está
		// cocinando. Va aparte, como trabajo en proceso.
		if o.Estado != fabricacion.EstadoTerminada {
			res.EnProceso = round2(res.EnProceso + o.Cantidad)
			res.CostoEnProceso = round2(res.CostoEnProceso + o.CostoTotal)
			continue
		}

		res.Planificado = round2(res.Planificado + o.Cantidad)
		res.Producido = round2(res.Producido + o.CantidadProducida)
		res.CostoInsumos = round2(res.CostoInsumos + o.CostoTotal)
		res.ValorProducido = round2(res.ValorProducido + o.CantidadProducida*o.CostoUnitario)
		res.PerdidaAnormal = round2(res.PerdidaAnormal + o.PerdidaAnormal)
		if o.FueraDeTolerancia {
			res.FueraDeTolerancia++
		}

		f, ok := porProd[o.SKU]
		if !ok {
			f = &ResumenProducto{SKU: o.SKU, Nombre: o.Nombre}
			porProd[o.SKU] = f
			orden = append(orden, o.SKU)
		}
		f.Ordenes++
		f.Planificado = round2(f.Planificado + o.Cantidad)
		f.Producido = round2(f.Producido + o.CantidadProducida)
		f.CostoInsumos = round2(f.CostoInsumos + o.CostoTotal)
		f.PerdidaAnormal = round2(f.PerdidaAnormal + o.PerdidaAnormal)

		for _, r := range o.Resultados {
			f.NoLogrado = round2(f.NoLogrado + r.Cantidad)
			switch r.Destino {
			case fabricacion.DestinoPerdida:
				res.Perdido = round2(res.Perdido + r.Cantidad)
			case fabricacion.DestinoDescarte:
				res.Descartado = round2(res.Descartado + r.Cantidad)
			case fabricacion.DestinoReproceso:
				res.Reprocesado = round2(res.Reprocesado + r.Cantidad)
			}
			k := r.Destino + "|" + strings.ToLower(strings.TrimSpace(r.Motivo))
			m, ok := porMotivo[k]
			if !ok {
				m = &MotivoResumen{Motivo: r.Motivo, Destino: r.Destino}
				porMotivo[k] = m
				ordenMotivo = append(ordenMotivo, k)
			}
			m.Cantidad = round2(m.Cantidad + r.Cantidad)
			m.Veces++
		}
	}

	for _, sku := range orden {
		f := porProd[sku]
		if f.Planificado > 0 {
			f.RendimientoPct = round2(f.Producido / f.Planificado * 100)
		}
		res.PorProducto = append(res.PorProducto, *f)
	}
	// Lo que más se pierde, primero: es por donde hay que empezar a mirar.
	sort.SliceStable(ordenMotivo, func(i, j int) bool {
		return porMotivo[ordenMotivo[i]].Cantidad > porMotivo[ordenMotivo[j]].Cantidad
	})
	for _, k := range ordenMotivo {
		res.Motivos = append(res.Motivos, *porMotivo[k])
	}
	return res
}
