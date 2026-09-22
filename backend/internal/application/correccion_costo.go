package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// CORRECCIÓN DE COSTO: poner el promedio donde debe estar, dejando rastro.
//
// POR QUÉ EXISTE. El movimiento de revaluación ya estaba en el ledger, pero solo lo
// emitía el costo en destino. Cuando una compra se cargó con el costo equivocado
// —un cero por descuido, un precio en la moneda que no era—, el promedio queda mal
// para siempre: afecta al costo de cada venta posterior y al valor del inventario
// en el balance, y no había forma de arreglarlo salvo inventar entradas y salidas
// que falsean el Kardex.
//
// NO CAMBIA UNIDADES. Corrige lo que vale lo que hay, no cuánto hay. Si además
// falta o sobra mercancía, eso es un ajuste y es otra operación: mezclarlas dejaría
// un solo movimiento que nadie sabría leer.
var (
	// ErrCorreccionSinExistencia: no se puede revaluar lo que no está. Y es LA
	// guarda de este archivo — el plegado ignora una revaluación cuando el saldo es
	// cero o negativo (no hay entre qué unidades repartir el valor), así que
	// aceptarla dejaría un movimiento en el ledger que no cambia nada: la pantalla
	// diría «corregido», el costo seguiría igual y nadie sabría por qué.
	ErrCorreccionSinExistencia = errors.New("no hay existencia que revaluar: el costo se corrige sobre la mercancía que está en el almacén")
	// ErrCorreccionCostoInvalido: un costo negativo no existe.
	ErrCorreccionCostoInvalido = errors.New("el costo corregido no puede ser negativo")
	// ErrCorreccionSinCambio: el costo pedido ya es el vigente.
	ErrCorreccionSinCambio = errors.New("el costo ya es ese: no hay nada que corregir")
)

// CorreccionCosto describe el resultado de una corrección, para poder enseñarlo.
type CorreccionCosto struct {
	SKU           string  `json:"sku"`
	Nombre        string  `json:"nombre"`
	SedeID        string  `json:"sedeId"`
	Cantidad      float64 `json:"cantidad"`
	CostoAnterior float64 `json:"costoAnterior"`
	CostoNuevo    float64 `json:"costoNuevo"`
	// ValorAjustado es lo que cambia el inventario: (nuevo − anterior) × cantidad.
	// Es el importe que va al diario, y el que hay que poder explicar.
	ValorAjustado float64 `json:"valorAjustado"`
	Motivo        string  `json:"motivo"`
}

// CorregirCosto lleva el costo promedio de un producto al valor indicado, emitiendo
// una revaluación por la diferencia.
//
// El asiento sigue el mismo criterio que un ajuste de existencia: si el inventario
// vale más, la contrapartida rebaja el costo del período (como un sobrante); si
// vale menos, lo aumenta (como una merma). No es un caprichoso: la diferencia de
// valoración es resultado del ejercicio, y llevarla a patrimonio saltándose el
// estado de resultados escondería el error que se está corrigiendo.
func (s *Service) CorregirCosto(empresaID, sedeID, sku string, costoNuevo float64, motivo, actor, origen string) (CorreccionCosto, error) {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return CorreccionCosto{}, ErrMotivoRequerido
	}
	if costoNuevo < 0 {
		return CorreccionCosto{}, ErrCorreccionCostoInvalido
	}
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return CorreccionCosto{}, ErrProductoNoExiste
	}
	if p.EsCombo {
		return CorreccionCosto{}, ErrComboNoStockeable
	}

	movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID})
	cant, avg := fold(movs)
	if cant <= 0.0001 {
		return CorreccionCosto{}, fmt.Errorf("%w: %s", ErrCorreccionSinExistencia, p.SKU)
	}
	diferencia := round2((costoNuevo - avg) * cant)
	if diferencia > -0.005 && diferencia < 0.005 {
		return CorreccionCosto{}, ErrCorreccionSinCambio
	}

	almacenID := s.almacenParaEscritura(empresaID, sedeID, "")
	guardado := s.movimientos.Append(inventario.Movimiento{
		EmpresaID: empresaID, SedeID: sedeID, AlmacenID: almacenID,
		ProductoID: p.ID, SKU: p.SKU,
		Tipo: inventario.MovRevaluacion, Cantidad: 0, ValorAgregado: diferencia,
		Motivo: "corrección de costo · " + motivo,
		// RefTipo propio: una corrección hecha por una persona no puede confundirse
		// con la revaluación automática de un costo en destino. Quien audite el
		// Kardex tiene que poder distinguir quién movió el valor y por qué.
		RefTipo: "correccion_costo", Actor: actor, Fecha: ahora(),
	})
	s.asentarCorreccionCosto(empresaID, actor, guardado, diferencia)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.costo.corregir", p.SKU, motivo))

	_, nuevoAvg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID}))
	return CorreccionCosto{
		SKU: p.SKU, Nombre: p.Nombre, SedeID: sedeID,
		Cantidad: round2(cant), CostoAnterior: round2(avg), CostoNuevo: round2(nuevoAvg),
		ValorAjustado: diferencia, Motivo: motivo,
	}, nil
}

// asentarCorreccionCosto deriva el asiento de la revaluación manual. La cuenta de
// inventario es la del rubro del producto, como en cualquier otro movimiento suyo.
func (s *Service) asentarCorreccionCosto(empresaID, actor string, m inventario.Movimiento, diferencia float64) {
	if s.asientos == nil || (diferencia > -0.005 && diferencia < 0.005) {
		return
	}
	montos := montoPorProducto{m.ProductoID: absFloat(diferencia)}
	if diferencia > 0 {
		s.asentar(empresaID, actor, m.Fecha, "Corrección de costo (mayor valor) — "+m.Motivo, "correccion_costo", m.ID, append(
			s.lineasDeInventario(empresaID, montos, true),
			contabilidad.Linea{Codigo: contabilidad.CtaCostoDeVentas, Haber: round2(diferencia)},
		))
		return
	}
	s.asentar(empresaID, actor, m.Fecha, "Corrección de costo (menor valor) — "+m.Motivo, "correccion_costo", m.ID, append(
		[]contabilidad.Linea{{Codigo: contabilidad.CtaCostoDeVentas, Debe: round2(-diferencia)}},
		s.lineasDeInventario(empresaID, montos, false)...,
	))
}

// absFloat es el valor absoluto. Existe porque montoPorProducto solo admite
// importes positivos: el lado del asiento lo decide quien llama, no el signo.
func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
