package application

import "github.com/mornix/elerp/internal/domain/inventario"

// QUÉ MOVIMIENTOS SE QUEDARON SIN ASIENTO: el diagnóstico.
//
// Un movimiento anexado directamente al repositorio —sin pasar por la capa de
// aplicación— mueve la existencia pero no deriva su asiento. Pasa con los datos
// sembrados y con cualquier arreglo hecho contra la base. El resultado es el peor
// descuadre posible: el inventario y la cuenta contable cuentan historias
// distintas, y el BALANCE CUADRA IGUAL —porque no falta media pata sino el asiento
// entero—, así que nada lo delata.
//
// ARREGLARLO YA ESTABA RESUELTO: lo hace RecontabilizarPendientes en cada arranque,
// con este mismo criterio. Lo que faltaba era poder MIRAR qué hay pendiente sin
// aplicar nada — que es lo que se necesita cuando el informe de valoración acusa
// una diferencia y hay que averiguar de dónde sale antes de tocar el libro.

// MovimientoSinAsiento describe un movimiento que quedó sin respaldo contable.
type MovimientoSinAsiento struct {
	ID     string  `json:"id"`
	SKU    string  `json:"sku"`
	Tipo   string  `json:"tipo"`
	Fecha  string  `json:"fecha"`
	Motivo string  `json:"motivo"`
	Valor  float64 `json:"valor"`
}

// ResultadoRecontabilizacion resume lo que se hizo (o se haría).
type ResultadoRecontabilizacion struct {
	Revisados  int                    `json:"revisados"`
	SinAsiento []MovimientoSinAsiento `json:"sinAsiento"`
	Asentados  int                    `json:"asentados"`
	Aplicado   bool                   `json:"aplicado"`
}

// movimientosSinAsiento encuentra los movimientos de inventario que deberían tener
// asiento propio y no lo tienen. Aplica el MISMO criterio que la recontabilización
// del arranque; si los dos se separaran, el diagnóstico diría que falta algo que ya
// está asentado, o al revés.
//
// QUÉ SE EXCLUYE Y POR QUÉ:
//
//   - Los que llevan RefTipo (documento, compra, transferencia, costo en destino).
//     Su asiento lo emite el documento que los originó, con el importe agregado de
//     todas sus líneas; asentarlos uno a uno los contaría DOS VECES.
//   - Las transferencias, que no se asientan nunca: la mercancía sigue siendo de la
//     misma empresa y el patrimonio no cambia.
//   - Las revaluaciones sin referencia, porque no hay forma de saber contra qué
//     cuenta de resultado iban. Se listan pero no se asientan solas.
func (s *Service) movimientosSinAsiento(empresaID string) []inventario.Movimiento {
	if s.asientos == nil || s.movimientos == nil {
		return nil
	}
	conAsiento := map[string]bool{}
	for _, a := range s.LibroDiario(empresaID) {
		if a.RefTipo == "movimiento" && a.RefID != "" {
			conAsiento[a.RefID] = true
		}
	}
	out := []inventario.Movimiento{}
	for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{}) {
		if m.RefTipo != "" || conAsiento[m.ID] {
			continue
		}
		if m.Tipo == inventario.MovTransferencia || m.Tipo == inventario.MovRevaluacion {
			continue
		}
		if m.CostoUnitario <= 0 {
			continue // sin costo no hay importe que asentar
		}
		out = append(out, m)
	}
	return out
}

// RevisarContabilidadDeInventario lista lo que falta SIN tocar nada.
func (s *Service) RevisarContabilidadDeInventario(empresaID string) ResultadoRecontabilizacion {
	res := ResultadoRecontabilizacion{SinAsiento: []MovimientoSinAsiento{}}
	if s.movimientos != nil {
		res.Revisados = len(s.movimientos.List(empresaID, inventario.FiltroMovimiento{}))
	}
	for _, m := range s.movimientosSinAsiento(empresaID) {
		valor := m.Cantidad * m.CostoUnitario
		if valor < 0 {
			valor = -valor
		}
		res.SinAsiento = append(res.SinAsiento, MovimientoSinAsiento{
			ID: m.ID, SKU: m.SKU, Tipo: m.Tipo, Fecha: m.Fecha,
			Motivo: m.Motivo, Valor: round2(valor),
		})
	}
	return res
}
