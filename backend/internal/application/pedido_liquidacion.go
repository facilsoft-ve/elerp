package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/pedido"
)

/* LIQUIDACIÓN DEL REPARTIDOR — cerrar el turno con plata contada.
 *
 * El repartidor sale con pedidos que se cobran en la puerta y vuelve con
 * efectivo ajeno en el bolsillo. Mientras no entregue y alguien cuente, la
 * empresa tiene plata afuera y no sabe cuánta: eso no es un detalle operativo,
 * es un faltante esperando a que nadie se acuerde.
 *
 * Tres reglas sostienen esto:
 *
 *   1. LO QUE SE ESPERA NO SE TECLEA. Sale de las entregas que él cerró. Si el
 *      número esperado lo pusiera quien recibe, la liquidación no probaría nada.
 *   2. UN PEDIDO SE LIQUIDA UNA SOLA VEZ. El acta estampa su id en cada pedido,
 *      y el pedido estampado ya no vuelve a aparecer pendiente — por más veces
 *      que se abra la pantalla, y aunque dos personas la abran a la vez.
 *   3. EL ACTA NO SE EDITA. Si al día siguiente aparece un billete, se levanta
 *      otra. Lo que se corrige a mano no se puede auditar.
 *
 * Y la plata no se queda en el acta: cada entrega cobrada se registra como cobro
 * contra SU factura, que es donde vive la cuenta por cobrar. Sin eso, el cliente
 * quedaría debiendo para siempre algo que ya pagó en la puerta.
 */

var (
	ErrLiquidacionesNoDisponible = errors.New("la liquidación de repartidores no está disponible")
	ErrNadaQueLiquidar           = errors.New("ese repartidor no tiene entregas cobradas pendientes de liquidar")
	ErrDeclaradoInvalido         = errors.New("lo declarado no puede ser negativo")
)

// ConLiquidaciones cablea el registro de actas de cierre del repartidor.
func (s *Service) ConLiquidaciones(r pedido.LiquidacionRepository) *Service {
	s.liquidaciones = r
	return s
}

// EntregaPorLiquidar es una entrega cobrada que todavía no se ha rendido.
type EntregaPorLiquidar struct {
	PedidoID    string  `json:"pedidoId"`
	Numero      int     `json:"numero"`
	Tracking    string  `json:"tracking,omitempty"`
	Direccion   string  `json:"direccion,omitempty"`
	DocumentoID string  `json:"documentoId,omitempty"`
	Total       float64 `json:"total"`
	// CobradoBs es lo que el repartidor declaró haber recibido en esa puerta.
	// Puede diferir del total: un cliente que no tenía todo el efectivo deja una
	// diferencia, y esconderla acá la volvería un faltante del repartidor.
	CobradoBs float64 `json:"cobradoBs"`
	Cerrado   string  `json:"cerrado,omitempty"`
}

// ResumenLiquidacion es lo que la pantalla necesita para recibirle el turno.
type ResumenLiquidacion struct {
	RepartidorID     string               `json:"repartidorId"`
	RepartidorNombre string               `json:"repartidorNombre"`
	Entregas         []EntregaPorLiquidar `json:"entregas"`
	// EsperadoBs es lo que debe traer. Se deriva de las entregas: nunca se teclea.
	EsperadoBs float64 `json:"esperadoBs"`
}

// esperadoDe es cuánto debe rendir una entrega.
//
// Lo declarado en la puerta manda sobre el total del pedido: si el cliente pagó
// de menos, el faltante es del pedido y no del repartidor, y cargárselo a él es
// como se pierde un repartidor.
func esperadoDe(p pedido.Pedido) float64 {
	if p.CobradoBs > 0 {
		return p.CobradoBs
	}
	return p.Total
}

// PendienteDeLiquidar lista lo que un repartidor tiene que rendir.
func (s *Service) PendienteDeLiquidar(empresaID, sedeID, repartidorID string) (ResumenLiquidacion, error) {
	if s.pedidos == nil || s.repartidores == nil {
		return ResumenLiquidacion{}, ErrPedidosNoDisponible
	}
	rep, ok := s.repartidores.ByID(empresaID, repartidorID)
	if !ok {
		return ResumenLiquidacion{}, ErrRepartidorNoExiste
	}
	out := ResumenLiquidacion{RepartidorID: rep.ID, RepartidorNombre: rep.Nombre, Entregas: []EntregaPorLiquidar{}}
	for _, p := range s.pedidos.List(empresaID, sedeID) {
		if p.RepartidorID != repartidorID || p.LiquidacionID != "" {
			continue
		}
		// Solo lo ENTREGADO y cobrado en la puerta. Lo que venía pagado por el
		// canal no pasó por sus manos, y lo que no se entregó todavía no se cobró.
		if p.Estado != pedido.EstadoEntregado || !p.CobraElRepartidor() {
			continue
		}
		e := EntregaPorLiquidar{
			PedidoID: p.ID, Numero: p.Numero, Tracking: p.Tracking,
			Direccion: p.Destino.Direccion, DocumentoID: p.DocumentoID,
			Total: p.Total, CobradoBs: p.CobradoBs, Cerrado: p.Cerrado,
		}
		out.Entregas = append(out.Entregas, e)
		out.EsperadoBs = round2(out.EsperadoBs + esperadoDe(p))
	}
	return out, nil
}

// LiquidarRepartidor cierra el turno: cuenta lo que trajo, levanta el acta y
// salda las facturas de lo que cobró en la puerta.
//
// `declarado` es lo que el repartidor puso sobre el mostrador. No se valida
// contra lo esperado —una liquidación que solo acepta el número correcto no
// sirve para encontrar faltantes—: se registra la diferencia y se exige una nota
// cuando no cuadra.
func (s *Service) LiquidarRepartidor(empresaID, sedeID, repartidorID, actor, origen string,
	declarado float64, nota string) (pedido.Liquidacion, error) {
	if s.liquidaciones == nil {
		return pedido.Liquidacion{}, ErrLiquidacionesNoDisponible
	}
	if declarado < 0 {
		return pedido.Liquidacion{}, ErrDeclaradoInvalido
	}
	resumen, err := s.PendienteDeLiquidar(empresaID, sedeID, repartidorID)
	if err != nil {
		return pedido.Liquidacion{}, err
	}
	if len(resumen.Entregas) == 0 {
		return pedido.Liquidacion{}, ErrNadaQueLiquidar
	}
	diferencia := round2(declarado - resumen.EsperadoBs)
	if diferencia != 0 && strings.TrimSpace(nota) == "" {
		// Un faltante (o un sobrante) sin explicación no sirve para decidir nada:
		// ni para descontarlo, ni para perdonarlo, ni para buscar el error.
		return pedido.Liquidacion{}, fmt.Errorf("la cuenta no cuadra por %.2f: hace falta una nota que lo explique", diferencia)
	}

	l := pedido.Liquidacion{
		EmpresaID: empresaID, SedeID: sedeID,
		RepartidorID: resumen.RepartidorID, RepartidorNombre: resumen.RepartidorNombre,
		EsperadoBs: resumen.EsperadoBs, DeclaradoBs: round2(declarado), DiferenciaBs: diferencia,
		Nota: strings.TrimSpace(nota), Actor: actor, Creada: ahora(),
	}
	for _, e := range resumen.Entregas {
		l.PedidoIDs = append(l.PedidoIDs, e.PedidoID)
	}
	l = s.liquidaciones.Append(l)

	// El estampado va DESPUÉS de tener el acta con id: si se marcara antes y el
	// acta fallara, los pedidos quedarían liquidados contra nada.
	for _, e := range resumen.Entregas {
		p, ok := s.pedidoDe(empresaID, e.PedidoID)
		if !ok {
			continue
		}
		p.LiquidacionID = l.ID
		s.pedidos.Update(p)
		s.saldarEntregaCobrada(empresaID, sedeID, actor, origen, p)
	}
	s.audit.Append(evento(empresaID, actor, origen, "pedido.repartidor.liquidar",
		l.RepartidorNombre, fmt.Sprintf("%d entregas · esperado %.2f · declarado %.2f", len(l.PedidoIDs), l.EsperadoBs, l.DeclaradoBs)))
	return l, nil
}

// saldarEntregaCobrada registra contra la factura lo que el repartidor cobró en
// la puerta.
//
// Es a mejor esfuerzo y no tumba la liquidación: el acta de la plata contada ya
// es un hecho, y si el saldo de la factura no se puede tocar —porque la venta no
// fue a crédito, o ya se saldó por otra vía— eso se revisa en Tesorería, no
// impidiendo que el repartidor entregue lo que trajo.
func (s *Service) saldarEntregaCobrada(empresaID, sedeID, actor, origen string, p pedido.Pedido) {
	if p.DocumentoID == "" {
		return
	}
	saldo, hay := s.SaldoPorCobrar(empresaID, p.DocumentoID)
	if !hay || saldo <= 0.004 {
		return
	}
	monto := esperadoDe(p)
	if monto > saldo {
		// Nunca por encima del saldo: cobrar de más no es un cobro, es un anticipo.
		monto = saldo
	}
	if monto <= 0.004 {
		return
	}
	if _, err := s.RegistrarCobro(empresaID, sedeID, actor, origen, CobroEntrada{
		DocumentoID: p.DocumentoID, Monto: monto, Moneda: "VES", Metodo: "efectivo_bs",
		Referencia: "Entrega " + p.Tracking,
	}); err != nil {
		s.audit.Append(evento(empresaID, actor, origen, "pedido.liquidacion.cobro_fallido", p.Tracking, err.Error()))
	}
}

// LiquidacionesDe lista las actas de cierre de una sede.
func (s *Service) LiquidacionesDe(empresaID, sedeID string) []pedido.Liquidacion {
	if s.liquidaciones == nil {
		return []pedido.Liquidacion{}
	}
	return s.liquidaciones.List(empresaID, sedeID)
}
