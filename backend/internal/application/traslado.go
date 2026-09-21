package application

import (
	"errors"
	"fmt"

	"github.com/mornix/elerp/internal/domain/inventario"
)

// TRASLADO ENTRE UBICACIONES DEL MISMO ALMACÉN.
//
// POR QUÉ EXISTE. Con ubicaciones, la mercancía se mueve dentro del depósito todos
// los días: llega al muelle y sube al estante, se baja a preparación, se aparta a
// cuarentena. Sin una operación propia, la única forma de reflejarlo es un ajuste
// en menos y otro en más — y eso MIENTE dos veces: el Kardex lo lee como una merma
// seguida de un sobrante, y la contabilidad recibe dos asientos por algo que no
// cambió ni una unidad ni un bolívar.
//
// ES NEUTRO Y POR ESO NO ASIENTA. La cantidad total no cambia, el costo promedio
// tampoco: la mercancía sigue siendo de la misma empresa, la misma sede y el mismo
// almacén. Un asiento aquí sería ruido en el diario. Igual que las transferencias
// entre sedes, que tampoco asientan.
//
// ES LA BASE DE LOS PROCESOS EN PASOS. Una recepción en dos pasos es una entrada al
// muelle y un traslado a la ubicación final; una entrega en dos pasos es un traslado
// a preparación y una salida desde ahí. Por eso esta operación va primero.
var (
	// ErrTrasladoMismaUbicacion: origen y destino iguales no mueve nada, y aceptarlo
	// dejaría un par de movimientos que no explican ningún cambio.
	ErrTrasladoMismaUbicacion = errors.New("el origen y el destino son la misma ubicación")
	// ErrTrasladoCantidad: trasladar cero o menos no es una operación.
	ErrTrasladoCantidad = errors.New("la cantidad a trasladar debe ser mayor que cero")
	// ErrTrasladoSinSaldo: no hay tanto en la ubicación de origen. Mover lo que no
	// está es inventar existencia en el destino y hundir el origen en negativo.
	ErrTrasladoSinSaldo = errors.New("la ubicación de origen no tiene esa cantidad")
	// ErrTrasladoDestinoInvalido: el destino no existe, no es de este almacén o está
	// inactivo. NO se cae en silencio a «sin ubicar»: quien traslada eligió un sitio
	// y creería que la mercancía está ahí.
	ErrTrasladoDestinoInvalido = errors.New("la ubicación de destino no existe en este almacén")
)

// TrasladoPeticion describe un movimiento interno de mercancía.
type TrasladoPeticion struct {
	SedeID    string
	AlmacenID string
	// Origen y Destino son ids de ubicación. Origen vacío es legítimo y frecuente:
	// es la mercancía que está en el almacén sin más detalle, que es justo la que
	// interesa ubicar. Destino vacío también: desubicar es una decisión válida.
	Origen  string
	Destino string
	SKU     string
	// Lote acota el traslado a uno concreto. Vacío reparte por FEFO dentro de la
	// ubicación de origen: si alguien mueve «10 del estante A», mueve lo que caduca
	// antes, que es lo que conviene tener a mano.
	Lote     string
	Cantidad float64
	Motivo   string
}

// TrasladarEntreUbicaciones mueve mercancía de una ubicación a otra del MISMO
// almacén, emitiendo un par de movimientos por cada casilla tocada.
//
// El vencido SÍ se traslada, al revés que en una venta. Es lo contrario de un
// descuido: apartar a cuarentena lo que caducó es exactamente lo que hay que hacer
// con ello, y negar el traslado dejaría la mercancía vencida en el anaquel de venta
// sin forma de sacarla salvo dándola de baja.
func (s *Service) TrasladarEntreUbicaciones(empresaID, actor, origen string, pet TrasladoPeticion) ([]SaldoUbicacion, error) {
	if pet.Cantidad <= 0.0001 {
		return nil, ErrTrasladoCantidad
	}
	p, ok := s.productos.BySKU(empresaID, pet.SKU)
	if !ok {
		return nil, ErrProductoNoExiste
	}
	if p.EsCombo {
		return nil, ErrComboNoStockeable
	}
	almacenID := s.almacenParaEscritura(empresaID, pet.SedeID, pet.AlmacenID)
	if pet.Origen == pet.Destino {
		return nil, ErrTrasladoMismaUbicacion
	}
	// El destino se valida CONTRA EL ALMACÉN y a la vista: a diferencia de una
	// recepción —donde una ubicación inválida cae a «sin ubicar» y se ve—, aquí el
	// traslado entero perdería su sentido. Quien lo pidió quiere la mercancía en un
	// sitio concreto; dejarla en otro es peor que no moverla.
	if pet.Destino != "" && s.ubicacionParaEscritura(empresaID, almacenID, pet.Destino) == "" {
		return nil, ErrTrasladoDestinoInvalido
	}

	// De qué casillas sale. Se reparte por FEFO dentro de la ubicación de origen,
	// con el mismo criterio que una salida: lo que vence antes se mueve primero.
	candidatas := []bucket{}
	for _, b := range s.bucketsDe(empresaID, pet.SedeID, almacenID, p.ID) {
		if b.UbicacionID != pet.Origen {
			continue
		}
		if pet.Lote != "" && b.Lote != pet.Lote {
			continue
		}
		candidatas = append(candidatas, b)
	}

	type tramo struct {
		Lote, Vencimiento string
		Cantidad          float64
	}
	tramos := []tramo{}
	restante := pet.Cantidad
	for _, b := range candidatas {
		if restante <= 0.0001 {
			break
		}
		toma := b.Cantidad
		if toma > restante {
			toma = restante
		}
		tramos = append(tramos, tramo{Lote: b.Lote, Vencimiento: b.Vencimiento, Cantidad: round2(toma)})
		restante = round2(restante - toma)
	}
	// Sin tolerancia al descubierto, al revés que una venta: una venta ya ocurrió y
	// negarla no devuelve la mercancía al anaquel, pero un traslado que no puede
	// surtirse simplemente no se hace. Anexarlo crearía existencia en el destino.
	if restante > 0.0001 {
		return nil, fmt.Errorf("%w: faltan %.2f de %s", ErrTrasladoSinSaldo, restante, p.SKU)
	}

	motivo := pet.Motivo
	if motivo == "" {
		motivo = "traslado interno"
	}
	_, avg := fold(s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: pet.SedeID, ProductoID: p.ID}))
	for _, t := range tramos {
		base := inventario.Movimiento{
			EmpresaID: empresaID, SedeID: pet.SedeID, AlmacenID: almacenID,
			ProductoID: p.ID, SKU: p.SKU, Tipo: inventario.MovTransferencia,
			CostoUnitario: avg, Lote: t.Lote, Vencimiento: t.Vencimiento,
			Motivo: motivo, RefTipo: "traslado", Actor: actor, Fecha: ahora(),
		}
		salida := base
		salida.Cantidad, salida.UbicacionID = -t.Cantidad, pet.Origen
		guardada := s.movimientos.Append(salida)

		// La entrada apunta a SU salida. Un traslado interno no es un documento —no
		// hay nada que aprobar ni recibir—, así que el enlace no es un id aparte sino
		// el de la pata que lo origina: leyendo la entrada se sabe exactamente de qué
		// casilla vino, que es la pregunta que alguien se hace al mirarlo.
		entrada := base
		entrada.Cantidad, entrada.UbicacionID = t.Cantidad, pet.Destino
		entrada.RefID = guardada.ID
		s.movimientos.Append(entrada)
	}

	s.audit.Append(evento(empresaID, actor, origen, "inventario.traslado", p.SKU, motivo))
	return s.ExistenciaPorUbicacion(empresaID, pet.SedeID, p.SKU), nil
}
