package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/inventario"
)

// TRAZABILIDAD POR LOTE.
//
// POR QUÉ EXISTE. Sin lote, una alerta sanitaria se atiende sacando TODO el
// producto del anaquel, y a la pregunta «¿a quién le vendí el lote X?» no hay
// respuesta. Con vencimiento, además, la mercancía caduca en el almacén sin que
// nadie se entere hasta que un cliente lo ve en la etiqueta.
//
// CÓMO. El saldo por lote es una PROYECCIÓN del mismo ledger —no un contador
// aparte— y las salidas lo consumen solas por FEFO: sale antes lo que vence
// antes. Que sea automático es lo que permite que el punto de venta y la
// facturación no cambien: el cajero no elige lote, y aun así el saldo por lote
// nunca se aparta del saldo del producto.
//
// LA VALORACIÓN NO CAMBIA: sigue siendo el promedio ponderado del producto. Un
// costo por lote sería otro sistema de valoración (FIFO por capas), no un añadido.

var (
	// ErrLoteRequerido: el producto exige lote y la entrada no lo trae.
	ErrLoteRequerido = errors.New("este producto se lleva por lotes: indica el lote")
	// ErrVencimientoRequerido: el producto controla vencimiento y falta la fecha.
	ErrVencimientoRequerido = errors.New("este producto controla vencimiento: indica la fecha de caducidad")
	// ErrVencimientoInvalido: la fecha no tiene forma de fecha.
	ErrVencimientoInvalido = errors.New("la fecha de vencimiento debe ser AAAA-MM-DD")
	// ErrStockPorLoteInsuficiente: hay saldo del producto pero no repartido en
	// lotes que lo cubran. Se avisa en vez de sacar de un lote inventado.
	ErrStockPorLoteInsuficiente = errors.New("no hay lotes con existencia suficiente para esa salida")
)

// SaldoLote es la existencia de un lote concreto de un producto en una sede.
type SaldoLote struct {
	ProductoID  string  `json:"productoId"`
	SKU         string  `json:"sku"`
	Nombre      string  `json:"nombre"`
	SedeID      string  `json:"sedeId"`
	Lote        string  `json:"lote"`
	Vencimiento string  `json:"vencimiento,omitempty"`
	Cantidad    float64 `json:"cantidad"`
	// DiasParaVencer es negativo si ya venció. Solo tiene sentido con vencimiento.
	DiasParaVencer int  `json:"diasParaVencer,omitempty"`
	Vencido        bool `json:"vencido,omitempty"`
}

// foldLotes reduce los movimientos de un producto a su saldo POR LOTE.
//
// Los movimientos sin lote —todo el histórico anterior a esto, y todo producto
// que no lo exige— se agrupan bajo el lote vacío. Eso es deliberado: así un
// producto al que se le activa el control de lotes hoy no pierde su existencia
// anterior, queda visible como «sin lote» y alguien decide qué hacer con ella.
func foldLotes(movs []inventario.Movimiento) map[string]float64 {
	saldos := map[string]float64{}
	for _, m := range movs {
		// La revaluación no mueve unidades: no pertenece a ningún lote.
		if m.Tipo == inventario.MovRevaluacion {
			continue
		}
		saldos[m.Lote] += m.Cantidad
	}
	// Los lotes agotados no se listan, pero sí los negativos: un saldo por lote en
	// negativo es un dato roto y esconderlo lo dejaría sin arreglar.
	for lote, cant := range saldos {
		if cant > -0.0001 && cant < 0.0001 {
			delete(saldos, lote)
		}
	}
	return saldos
}

// vencimientoDeLote recupera la fecha de caducidad con la que un lote entró.
// Se toma de su primera ENTRADA: el lote es del fabricante y su fecha no cambia
// porque el mismo lote vuelva a comprarse.
func vencimientoDeLote(movs []inventario.Movimiento, lote string) string {
	for _, m := range movs {
		if m.Lote == lote && m.Vencimiento != "" {
			return m.Vencimiento
		}
	}
	return ""
}

// SaldosPorLote proyecta la existencia por lote de un producto en una sede.
// Ordenados por vencimiento (los que vencen antes primero) y, a igualdad, por
// lote: es el orden en que se consumen y en que interesa leerlos.
func (s *Service) SaldosPorLote(empresaID, sedeID, productoID string) []SaldoLote {
	p, ok := s.productos.ByID(empresaID, productoID)
	if !ok {
		return []SaldoLote{}
	}
	movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: productoID})
	hoy := time.Now().UTC().Format("2006-01-02")
	out := []SaldoLote{}
	for lote, cant := range foldLotes(movs) {
		venc := vencimientoDeLote(movs, lote)
		sl := SaldoLote{
			ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre, SedeID: sedeID,
			Lote: lote, Vencimiento: venc, Cantidad: round2(cant),
		}
		if venc != "" {
			sl.DiasParaVencer = diasEntre(hoy, venc)
			sl.Vencido = venc < hoy
		}
		out = append(out, sl)
	}
	ordenarPorVencimiento(out)
	return out
}

// ordenarPorVencimiento deja primero lo que caduca antes. Un lote SIN fecha va al
// final: no se puede afirmar que venza antes que uno con fecha, y adelantarlo
// haría que el consumo automático se comiera primero justo lo que no urge.
func ordenarPorVencimiento(ls []SaldoLote) {
	sort.SliceStable(ls, func(i, j int) bool {
		a, b := ls[i].Vencimiento, ls[j].Vencimiento
		if (a == "") != (b == "") {
			return b == ""
		}
		if a != b {
			return a < b
		}
		return ls[i].Lote < ls[j].Lote
	})
}

// TramoSalida es un pedazo de una salida asignado a un lote concreto.
type TramoSalida struct {
	Lote        string
	Vencimiento string
	Cantidad    float64
}

// RepartirSalidaFEFO decide de qué lotes sale una cantidad: primero lo que vence
// antes (First Expired, First Out).
//
// ES EL ÚNICO CAMINO para que una salida toque lotes. Que sea automático y
// centralizado es lo que permite que el punto de venta, la facturación y las
// transferencias sigan sin saber de lotes: si cada uno eligiera por su cuenta, el
// primero que se olvidara dejaría el saldo por lote apartado del saldo real, y
// esa diferencia no falla — solo hace que la trazabilidad mienta.
//
// Un producto SIN control de lotes devuelve un único tramo sin lote, que es el
// comportamiento de siempre.
func (s *Service) RepartirSalidaFEFO(empresaID, sedeID, productoID string, cantidad float64) ([]TramoSalida, error) {
	p, ok := s.productos.ByID(empresaID, productoID)
	if !ok || !p.RequiereLote {
		return []TramoSalida{{Cantidad: cantidad}}, nil
	}
	if cantidad <= 0.0001 {
		return []TramoSalida{}, nil
	}
	disponibles := s.SaldosPorLote(empresaID, sedeID, productoID)

	tramos := []TramoSalida{}
	restante := cantidad
	for _, l := range disponibles {
		if restante <= 0.0001 {
			break
		}
		if l.Cantidad <= 0.0001 {
			continue // un lote en negativo no puede surtir nada
		}
		toma := l.Cantidad
		if toma > restante {
			toma = restante
		}
		tramos = append(tramos, TramoSalida{Lote: l.Lote, Vencimiento: l.Vencimiento, Cantidad: round2(toma)})
		restante = round2(restante - toma)
	}
	if restante > 0.0001 {
		// Hay que decirlo. Sacar el resto de un lote vacío —o sin lote— dejaría el
		// saldo del producto cuadrado y el de los lotes roto, que es la forma de
		// fallar que este módulo existe para evitar.
		return nil, fmt.Errorf("%w: faltan %.2f de %s", ErrStockPorLoteInsuficiente, restante, p.SKU)
	}
	return tramos, nil
}

// repartirSalidaFEFOTolerante es la variante para el MOSTRADOR: reparte lo que
// los lotes cubran y manda el resto a un tramo SIN lote.
//
// Por qué no falla como la estricta: la emisión de una factura no exige stock —el
// punto de venta tiene que poder seguir vendiendo—, así que bloquearla acá
// convertiría una discrepancia que YA existía en una caja parada. El faltante no
// se esconde: sale como saldo NEGATIVO del lote vacío, visible en la existencia
// por lote, que es donde alguien puede arreglarlo.
func (s *Service) repartirSalidaFEFOTolerante(empresaID, sedeID, productoID string, cantidad float64) []TramoSalida {
	tramos, err := s.RepartirSalidaFEFO(empresaID, sedeID, productoID, cantidad)
	if err == nil {
		return tramos
	}
	// Se rehace cubriendo lo que haya y dejando el descubierto sin lote.
	cubierto := 0.0
	tramos = []TramoSalida{}
	for _, l := range s.SaldosPorLote(empresaID, sedeID, productoID) {
		if cubierto >= cantidad-0.0001 || l.Cantidad <= 0.0001 {
			continue
		}
		toma := l.Cantidad
		if toma > cantidad-cubierto {
			toma = cantidad - cubierto
		}
		tramos = append(tramos, TramoSalida{Lote: l.Lote, Vencimiento: l.Vencimiento, Cantidad: round2(toma)})
		cubierto = round2(cubierto + toma)
	}
	if falta := round2(cantidad - cubierto); falta > 0.0001 {
		tramos = append(tramos, TramoSalida{Cantidad: falta})
	}
	return tramos
}

// anexarSalidaPorLotes emite una salida repartida en lotes por FEFO.
//
// `base` es el movimiento tal como se emitiría sin lotes, con la cantidad ya en
// NEGATIVO; esta función lo clona una vez por lote. Los productos sin
// trazabilidad salen en un único movimiento idéntico al de siempre.
//
// Existe para que ningún sitio de salida tenga que acordarse de los lotes: el que
// se olvidara dejaría el saldo por lote apartado del real, y esa diferencia no
// falla — solo hace que la trazabilidad mienta.
func (s *Service) anexarSalidaPorLotes(base inventario.Movimiento) {
	for _, t := range s.repartirSalidaFEFOTolerante(base.EmpresaID, base.SedeID, base.ProductoID, -base.Cantidad) {
		m := base
		m.Cantidad = -t.Cantidad
		m.Lote, m.Vencimiento = t.Lote, t.Vencimiento
		s.movimientos.Append(m)
	}
}

// lotesQueSalieron recupera, de una transferencia, con qué lotes y en qué
// cantidad salió un SKU del origen. Es lo que permite que el lote VIAJE: el
// destino reingresa exactamente lo mismo que se despachó.
//
// Sin lotes de por medio devuelve un único tramo con la cantidad total, así que
// quien lo use se comporta igual que antes para el resto del catálogo.
func (s *Service) lotesQueSalieron(empresaID, sku, transfID string) []TramoSalida {
	out := []TramoSalida{}
	for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{SKU: sku}) {
		if m.RefID != transfID || m.Cantidad >= 0 {
			continue
		}
		out = append(out, TramoSalida{Lote: m.Lote, Vencimiento: m.Vencimiento, Cantidad: -m.Cantidad})
	}
	return out
}

// validarLoteDeEntrada comprueba y normaliza el lote y el vencimiento de una
// entrada según lo que el producto exige.
//
// Se valida al RECIBIR y no al vender porque es el único momento en que alguien
// tiene la caja delante con la etiqueta. Pedirlo después es pedir que lo
// inventen.
func validarLoteDeEntrada(p inventario.Producto, lote, vencimiento string) (string, string, error) {
	lote = strings.TrimSpace(lote)
	vencimiento = strings.TrimSpace(vencimiento)
	if !p.RequiereLote {
		// El producto no los lleva: se descartan en vez de guardarse a medias, para
		// que no aparezcan lotes fantasma en un catálogo que no los usa.
		return "", "", nil
	}
	if lote == "" {
		return "", "", fmt.Errorf("%w: %s", ErrLoteRequerido, p.SKU)
	}
	if p.ControlaVencimiento {
		if vencimiento == "" {
			return "", "", fmt.Errorf("%w: %s", ErrVencimientoRequerido, p.SKU)
		}
		if _, err := time.Parse("2006-01-02", vencimiento); err != nil {
			return "", "", fmt.Errorf("%w: %s", ErrVencimientoInvalido, vencimiento)
		}
	}
	return lote, vencimiento, nil
}

// LotesPorVencer lista los lotes con existencia que vencen dentro de `dias`
// (incluidos los YA vencidos, con días negativos), en toda la empresa o en una
// sede.
//
// `dias` 0 se lee como 30: un aviso por defecto es más útil que una lista vacía
// que parece decir «no hay nada por vencer».
func (s *Service) LotesPorVencer(empresaID, sedeID string, dias int) []SaldoLote {
	if dias <= 0 {
		dias = 30
	}
	sedes := []string{sedeID}
	if sedeID == "" {
		// Sin sede se barre la empresa entera. Sin el repo de sedes cableado no hay
		// forma de saber cuáles son, y devolver vacío diría «no hay nada por vencer»,
		// que es una mentira peligrosa: se deja fuera y quien llama pasa la sede.
		if s.sedes == nil {
			return []SaldoLote{}
		}
		sedes = sedes[:0]
		for _, sd := range s.sedes.List(empresaID) {
			sedes = append(sedes, sd.ID)
		}
	}
	out := []SaldoLote{}
	for _, sede := range sedes {
		for _, p := range s.productos.List(empresaID) {
			if !p.RequiereLote || !p.ControlaVencimiento {
				continue
			}
			for _, l := range s.SaldosPorLote(empresaID, sede, p.ID) {
				if l.Vencimiento == "" || l.Cantidad <= 0.0001 {
					continue
				}
				if l.DiasParaVencer <= dias {
					out = append(out, l)
				}
			}
		}
	}
	ordenarPorVencimiento(out)
	return out
}
