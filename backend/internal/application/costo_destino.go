package application

import (
	"errors"
	"strings"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// COSTO EN DESTINO. Ver domain/compra/costodestino.go para el porqué.
//
// Lo que resuelve acá: repartir un costo entre lo recibido y anexarlo al ledger
// como revaluación, sin reescribir la recepción.

var (
	// ErrCostoDestinoSinOrden: se reparte sobre una recepción concreta.
	ErrCostoDestinoSinOrden = errors.New("el costo en destino necesita una orden de compra")
	// ErrCostoDestinoSinRecibir: no se puede encarecer lo que todavía no llegó. El
	// costo se aplica sobre lo RECIBIDO, que es lo único que tiene existencia.
	ErrCostoDestinoSinRecibir = errors.New("la orden no tiene nada recibido sobre lo que repartir el costo")
	// ErrCostoDestinoMontoCero: un reparto de cero no hace nada y deja un documento
	// que parece que sí.
	ErrCostoDestinoMontoCero = errors.New("el costo en destino necesita un monto distinto de cero")
	// ErrCostoDestinoSinDescripcion: sin decir QUÉ costó, el Kardex queda con una
	// revaluación que nadie sabrá explicar en seis meses.
	ErrCostoDestinoSinDescripcion = errors.New("el costo en destino necesita una descripción (flete, impuesto, seguro…)")
	// ErrCostoDestinoCriterio: criterio de reparto desconocido.
	ErrCostoDestinoCriterio = errors.New("criterio de reparto inválido (valor o cantidad)")
	// ErrCostoDestinoSinBase: con el criterio elegido no hay nada sobre lo que
	// prorratear (todo el valor recibido es cero, por ejemplo). Se avisa en vez de
	// repartir a partes iguales, que sería inventarse un criterio.
	ErrCostoDestinoSinBase = errors.New("con ese criterio no hay base para repartir el costo")
)

// ConCostosEnDestino cablea el repositorio de costos en destino.
func (s *Service) ConCostosEnDestino(r compra.CostoEnDestinoRepo) *Service {
	s.costosDestino = r
	return s
}

// CostosEnDestinoDe devuelve los costos ya aplicados a una orden.
func (s *Service) CostosEnDestinoDe(empresaID, ordenID string) []compra.CostoEnDestino {
	if s.costosDestino == nil {
		return []compra.CostoEnDestino{}
	}
	return s.costosDestino.PorOrden(empresaID, ordenID)
}

// EntradaCostoEnDestino son los datos para aplicar un costo en destino.
type EntradaCostoEnDestino struct {
	OrdenCompraID string
	Descripcion   string
	Monto         float64
	Criterio      string // vacío ⇒ por valor
}

// pesoLinea devuelve el peso de una línea recibida según el criterio.
func pesoLinea(l compra.Linea, criterio string) float64 {
	if criterio == compra.CriterioCantidad {
		return l.CantidadRecibida
	}
	return l.CantidadRecibida * l.CostoUnitario
}

// AplicarCostoEnDestino reparte un costo entre lo recibido de una orden y lo
// anexa al inventario como revaluación.
//
// EL REPARTO tiene dos pasos que conviene no confundir:
//
//  1. Cuánto le toca a cada producto, por valor o por cantidad recibida.
//  2. Cuánto de eso puede ABSORBER el inventario. Si de 10 unidades recibidas
//     quedan 4, solo el 40 % de su parte corresponde a existencias que todavía se
//     pueden valorar; el otro 60 % es de mercancía ya vendida y es gasto del
//     período. Capitalizarlo igual inflaría la valorización con un costo que no
//     respalda ninguna unidad en el almacén.
//
// El asiento cuadra por construcción: Absorbido (1201) + AlGasto (5202) = Monto
// (2101). El redondeo se cierra contra la última línea, porque un asiento
// descuadrado NO se guarda —solo se registra en el log— y el costo desaparecería
// sin que nada fallara.
func (s *Service) AplicarCostoEnDestino(empresaID, actor, origen string, in EntradaCostoEnDestino) (compra.CostoEnDestino, error) {
	if s.costosDestino == nil {
		return compra.CostoEnDestino{}, errors.New("los costos en destino no están disponibles")
	}
	if strings.TrimSpace(in.OrdenCompraID) == "" {
		return compra.CostoEnDestino{}, ErrCostoDestinoSinOrden
	}
	desc := strings.TrimSpace(in.Descripcion)
	if desc == "" {
		return compra.CostoEnDestino{}, ErrCostoDestinoSinDescripcion
	}
	monto := round2(in.Monto)
	if monto > -0.005 && monto < 0.005 {
		return compra.CostoEnDestino{}, ErrCostoDestinoMontoCero
	}
	criterio := strings.TrimSpace(in.Criterio)
	if criterio == "" {
		criterio = compra.CriterioValor
	}
	if !compra.CriterioValido(criterio) {
		return compra.CostoEnDestino{}, ErrCostoDestinoCriterio
	}
	o, ok := s.ordenesCompra.ByID(empresaID, in.OrdenCompraID)
	if !ok {
		return compra.CostoEnDestino{}, ErrOCNoExiste
	}

	// Solo las líneas RECIBIDAS participan: lo pedido y no llegado no tiene
	// existencia que valorar.
	recibidas := make([]compra.Linea, 0, len(o.Lineas))
	base := 0.0
	for _, l := range o.Lineas {
		if l.CantidadRecibida > 0.0001 {
			recibidas = append(recibidas, l)
			base += pesoLinea(l, criterio)
		}
	}
	if len(recibidas) == 0 {
		return compra.CostoEnDestino{}, ErrCostoDestinoSinRecibir
	}
	if base <= 0.0001 {
		return compra.CostoEnDestino{}, ErrCostoDestinoSinBase
	}

	// --- Reparto y absorción -------------------------------------------------
	lineas := make([]compra.LineaCostoEnDestino, 0, len(recibidas))
	repartidoAcum, absorbidoAcum := 0.0, 0.0
	for i, l := range recibidas {
		reparto := round2(monto * pesoLinea(l, criterio) / base)
		// La última línea se lleva el resto del redondeo: la suma de los repartos
		// tiene que dar el monto EXACTO o el asiento no cuadra.
		if i == len(recibidas)-1 {
			reparto = round2(monto - repartidoAcum)
		}
		repartidoAcum = round2(repartidoAcum + reparto)

		enStock := s.existenciaDeProductoEnSede(empresaID, o.SedeID, l.ProductoID)
		absorbido := reparto
		switch {
		case enStock <= 0.0001:
			absorbido = 0 // no queda nada que valorar: todo es gasto del período
		case enStock < l.CantidadRecibida:
			absorbido = round2(reparto * enStock / l.CantidadRecibida)
		}
		absorbidoAcum = round2(absorbidoAcum + absorbido)

		lineas = append(lineas, compra.LineaCostoEnDestino{
			ProductoID: l.ProductoID, SKU: l.SKU, Nombre: l.Nombre,
			Recibido: l.CantidadRecibida, EnStock: enStock,
			Reparto: reparto, Absorbido: absorbido,
		})
	}
	alGasto := round2(monto - absorbidoAcum)

	// --- Ledger: una revaluación por producto que absorbe algo ----------------
	almacenID := s.almacenParaEscritura(empresaID, o.SedeID, "")
	for _, l := range lineas {
		if l.Absorbido > -0.005 && l.Absorbido < 0.005 {
			continue
		}
		s.movimientos.Append(inventario.Movimiento{
			EmpresaID: empresaID, SedeID: o.SedeID, AlmacenID: almacenID,
			ProductoID: l.ProductoID, SKU: l.SKU,
			Tipo: inventario.MovRevaluacion, Cantidad: 0, ValorAgregado: l.Absorbido,
			Motivo:  desc + " · OC " + o.NumeroCompleto,
			RefTipo: refCostoDestino, RefID: o.ID, Actor: actor, Fecha: ahora(),
		})
	}

	out := s.costosDestino.Append(compra.CostoEnDestino{
		EmpresaID: empresaID, OrdenCompraID: o.ID, OrdenNumero: o.NumeroCompleto,
		Descripcion: desc, Monto: monto, Criterio: criterio, Lineas: lineas,
		Absorbido: absorbidoAcum, AlGasto: alGasto,
		Actor: actor, Aplicado: ahora(),
	})

	s.asentarCostoEnDestino(empresaID, actor, out)
	s.audit.Append(evento(empresaID, actor, origen, "compras.costo_destino.aplicar", o.NumeroCompleto, desc))
	return out, nil
}

// refCostoDestino etiqueta en el ledger los movimientos de un costo en destino,
// para poder distinguir una revaluación de un ajuste de inventario al auditar.
const refCostoDestino = "costo_destino"

// existenciaDeProductoEnSede proyecta cuánto queda HOY de un producto en la sede.
func (s *Service) existenciaDeProductoEnSede(empresaID, sedeID, productoID string) float64 {
	movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: productoID})
	cant, _ := fold(movs)
	return cant
}

// asentarCostoEnDestino deriva el asiento: lo que el inventario absorbió sube
// 1201, lo que no cabía (mercancía ya vendida) va al gasto, y el total se debe a
// quien prestó el servicio.
//
//	Debe  1201 Inventario            (absorbido)
//	Debe  5202 Diferencia en compras (al gasto)
//	Haber 2101 Cuentas por pagar     (monto)
//
// Con monto negativo —la corrección de un costo mal cargado— las tres patas se
// invierten solas: asentar() reparte por signo.
func (s *Service) asentarCostoEnDestino(empresaID, actor string, c compra.CostoEnDestino) {
	if s.asientos == nil {
		return
	}
	lineas := []contabilidad.Linea{}
	agregar := func(codigo string, monto float64) {
		if monto > -0.005 && monto < 0.005 {
			return
		}
		if monto > 0 {
			lineas = append(lineas, contabilidad.Linea{Codigo: codigo, Debe: monto})
		} else {
			lineas = append(lineas, contabilidad.Linea{Codigo: codigo, Haber: -monto})
		}
	}
	agregar(contabilidad.CtaInventario, c.Absorbido)
	agregar(contabilidad.CtaDiferenciaEnCompras, c.AlGasto)
	agregar(contabilidad.CtaCuentasPorPagar, -c.Monto)
	if len(lineas) == 0 {
		return
	}
	s.asentar(empresaID, actor, "", c.Descripcion+" — costo en destino OC "+c.OrdenNumero,
		refCostoDestino, c.ID, lineas)
}
