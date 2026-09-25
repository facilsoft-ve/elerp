package application

import (
	"github.com/mornix/elerp/internal/domain/fabricacion"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* COMPRAS RECIBIDAS QUE NUNCA ASENTARON.
 *
 * Una recepción de compra sube el inventario y la cuenta por pagar por el mismo
 * monto; lo hace RecibirOrdenCompra al recibir. Pero una orden puede llegar a la
 * base ya recibida sin pasar por ahí —el seed las siembra así, y un arreglo contra
 * la base también— y entonces la mercancía está en el estante y la contabilidad no
 * se enteró.
 *
 * POR QUÉ NO LO ARREGLABA LA RECONTABILIZACIÓN QUE YA EXISTÍA, que es la parte
 * interesante: esa excluye a propósito los movimientos con documento, porque su
 * asiento genérico de entrada acredita CAPITAL —correcto para un inventario inicial
 * y falso para una compra, que se le debe al proveedor—. Asentarlas por esa vía
 * habría cuadrado el inventario metiendo la deuda en el patrimonio. La exclusión
 * estaba bien; lo que faltaba era esto, que sabe la contrapartida.
 *
 * Se encontró en la demostración de la ferretería: dos órdenes recibidas sin asiento
 * por 300 y 150, que explicaban al céntimo los 450 de diferencia que la pantalla de
 * Valoración venía acusando sin que nadie pudiera rastrearlos.
 */

// RecontabilizarComprasPendientes emite el asiento de las órdenes de compra que
// movieron inventario y no tienen ninguno. Devuelve cuántas asentó.
//
// Es idempotente y CONSERVADORA: solo toca la orden que no tiene asiento alguno. Una
// con asiento parcial o con el importe equivocado no se corrige acá —habría que
// decidir qué hacer con el que ya está, y el libro es inmutable—: el diagnóstico la
// seguirá acusando, que es lo correcto mientras la decisión sea de una persona.
func (s *Service) RecontabilizarComprasPendientes(empresaID, actor string) int {
	if s.asientos == nil || s.ordenesCompra == nil || s.movimientos == nil {
		return 0
	}
	conAsiento := map[string]bool{}
	for _, a := range s.LibroDiario(empresaID) {
		if a.RefID != "" {
			conAsiento[a.RefID] = true
		}
	}

	// Lo que REALMENTE entró al ledger por cada orden, no lo que dice la orden: si
	// se recibió parcial, el asiento tiene que valer la mercancía que hay.
	costo := map[string]float64{}
	porProducto := map[string]montoPorProducto{}
	for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{}) {
		if m.RefTipo != "compra" || m.RefID == "" || conAsiento[m.RefID] {
			continue
		}
		monto := m.Cantidad * m.CostoUnitario
		if monto <= 0 {
			continue
		}
		costo[m.RefID] += monto
		if porProducto[m.RefID] == nil {
			porProducto[m.RefID] = montoPorProducto{}
		}
		porProducto[m.RefID][m.ProductoID] += monto
	}
	if len(costo) == 0 {
		return 0
	}

	n := 0
	for _, o := range s.ordenesCompra.List(empresaID) {
		total, hay := costo[o.ID]
		if !hay || total <= 0.004 {
			continue
		}
		s.asentarCompra(empresaID, actor, o, round2(total), porProducto[o.ID])
		n++
	}
	return n
}

/* ÓRDENES DE FABRICACIÓN QUE ARRANCARON SIN PASAR POR CAJA.
 *
 * Mismo caso que las compras y misma razón: el seed las siembra ya en proceso, con
 * sus consumos anexados, sin pasar por IniciarOrden —que es quien lleva el valor a
 * Producción en proceso—. El resultado es el descuadre que esta tanda persigue: los
 * insumos salieron del almacén y su valor no está en ninguna cuenta.
 *
 * También cubre a las órdenes que ya estaban en curso cuando se desplegó la cuenta
 * 1202: arrancaron cuando no existía, así que nadie les cargó nada.
 */
func (s *Service) RecontabilizarFabricacionesEnCurso(empresaID, actor string) int {
	if s.asientos == nil || s.ordenesFabricacion == nil {
		return 0
	}
	n := 0
	for _, o := range s.OrdenesFabricacion(empresaID, "") {
		// Solo las ABIERTAS: una terminada ya pasó por su asiento de cierre, y
		// cargarle ahora el de arranque dejaría 1202 con saldo de una orden que no
		// tiene nada en curso.
		if o.Estado != fabricacion.EstadoEnProceso || o.CostoTotal <= 0.004 {
			continue
		}
		if s.enProduccionDe(empresaID, o.ID) > 0.004 {
			continue // ya está cargada: idempotente
		}
		s.asentarArranqueDeFabricacion(empresaID, actor, o)
		n++
	}
	return n
}
