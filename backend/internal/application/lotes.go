package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/fiscal"
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
	// ErrSoloQuedaVencido: hay existencia, pero está vencida. Es un motivo distinto
	// de «no hay»: se arregla dando de baja el lote, no comprando más.
	ErrSoloQuedaVencido = errors.New("la existencia disponible está VENCIDA: dale de baja antes de vender")
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

// TramoSalida es un pedazo de una salida asignado a un lote y una ubicación
// concretos. Las dos dimensiones viajan juntas porque el ledger las guarda juntas:
// repartir primero por lote y después por ubicación daría tramos que no existen.
type TramoSalida struct {
	Lote        string
	Vencimiento string
	// AlmacenID y UbicacionID dicen DE DÓNDE sale. El almacén va aquí y no lo pone
	// el llamante porque la mercancía sale de donde está: escribir la salida en un
	// almacén distinto del que la tenía deja ese almacén en negativo y al otro con
	// stock que ya no existe, con el total cuadrando en ambos casos.
	AlmacenID   string
	UbicacionID string
	Cantidad    float64
	// Descubierto marca el tramo que NO salió de una casilla real: es lo que los
	// saldos no cubrían. Se distingue porque su almacén no es «donde estaba la
	// mercancía» sino «donde el operador creía tenerla», y es ahí donde hay que ir
	// a cuadrarlo.
	Descubierto bool
}

// bucket es una casilla real del ledger: lo que hay de un producto en una
// ubicación concreta y de un lote concreto.
type bucket struct {
	Lote        string
	AlmacenID   string
	UbicacionID string
	Vencimiento string
	Cantidad    float64
	Vencido     bool
	Codigo      string // de la ubicación, para ordenar de forma estable
}

// bucketsDe proyecta las casillas con existencia de un producto en una sede,
// ordenadas por el orden en que se deben consumir:
//
//  1. Lo que vence antes (FEFO). Un lote sin fecha va al final: no se puede
//     afirmar que caduque antes que uno con fecha.
//  2. A igualdad, lo que está SIN UBICAR primero. Es el saldo que el sistema no
//     sabe dónde está; gastarlo primero hace que el inventario ubicado sea cada
//     vez más fiel, en vez de dejar un resto indefinido creciendo para siempre.
//
// `almacenID` acota la búsqueda a un almacén; vacío mira toda la sede.
//
// LO APARTADO NO SURTE. Se descuenta aquí, y aquí es donde tenía que ir: este es
// el único camino por el que sale mercancía, así que descontándolo una vez lo
// respetan todas las salidas —el punto de venta, la facturación, las
// transferencias— sin que ninguna sepa que los apartados existen. Restarlo en cada
// sitio que vende habría bastado con que uno se olvidara para vender lo apartado,
// y eso no falla: solo deja sin mercancía al cliente que la esperaba.
// Un apartado deja de contar en cuanto se marca despachado, y por eso el despacho
// lo marca ANTES de emitir sus salidas: si no, cada línea competiría con la propia
// reserva que la respalda y no podría surtirse.
func (s *Service) bucketsDe(empresaID, sedeID, almacenID, productoID string) []bucket {
	// EL PRINCIPAL ABSORBE LO QUE NO TIENE ALMACÉN, igual que movsDeAlmacen. No es
	// un detalle: todo el histórico anterior a los almacenes se guardó sin uno, y
	// las pantallas que muestran «la existencia del almacén principal» ya lo cuentan
	// dentro. Si el reparto filtrara exacto, la pantalla y el ledger dirían cosas
	// distintas sobre el mismo almacén — y lo cazó un conteo físico que anunciaba
	// sumar 22 unidades donde había que restar 3, porque comparaba la existencia que
	// ve la pantalla contra las casillas que ve el reparto.
	movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{
		SedeID: sedeID, ProductoID: productoID,
	})
	if almacenID != "" {
		incluirSinAlmacen := false
		if s.almacenes != nil {
			if a, ok := s.almacenes.ByID(empresaID, almacenID); ok {
				incluirSinAlmacen = s.esAlmacenPrincipal(empresaID, a)
			}
		}
		acotados := make([]inventario.Movimiento, 0, len(movs))
		for _, m := range movs {
			if m.AlmacenID == almacenID || (incluirSinAlmacen && m.AlmacenID == "") {
				acotados = append(acotados, m)
			}
		}
		movs = acotados
	}
	hoy := time.Now().UTC().Format("2006-01-02")

	type clave = claveCasilla
	suma := map[clave]float64{}
	for _, m := range movs {
		if m.Tipo == inventario.MovRevaluacion {
			continue // no mueve unidades: no ocupa casilla
		}
		suma[clave{Lote: m.Lote, Almacen: m.AlmacenID, Ubicacion: m.UbicacionID}] += m.Cantidad
	}
	for k, comprometido := range s.apartadoPorCasilla(empresaID, sedeID, productoID, "") {
		if _, existe := suma[k]; existe {
			suma[k] -= comprometido
		}
		// Un apartado sobre una casilla que ya no tiene movimientos no resta de
		// ninguna otra: restarlo del total repartiéndolo por ahí dejaría sin surtir
		// una casilla que sí tiene mercancía libre.
	}

	out := []bucket{}
	for k, cant := range suma {
		if cant <= 0.0001 {
			continue // agotada, comprometida o en negativo: no puede surtir nada
		}
		b := bucket{Lote: k.Lote, AlmacenID: k.Almacen, UbicacionID: k.Ubicacion, Cantidad: round2(cant)}
		b.Vencimiento = vencimientoDeLote(movs, k.Lote)
		if b.Vencimiento != "" {
			b.Vencido = b.Vencimiento < hoy
		}
		if k.Ubicacion != "" && s.ubicaciones != nil {
			if u, ok := s.ubicaciones.ByID(empresaID, k.Ubicacion); ok {
				b.Codigo = u.Codigo
			}
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, c := out[i].Vencimiento, out[j].Vencimiento
		if (a == "") != (c == "") {
			return c == ""
		}
		if a != c {
			return a < c
		}
		if out[i].Lote != out[j].Lote {
			return out[i].Lote < out[j].Lote
		}
		// Sin ubicar primero, después por código.
		if (out[i].UbicacionID == "") != (out[j].UbicacionID == "") {
			return out[i].UbicacionID == ""
		}
		if out[i].Codigo != out[j].Codigo {
			return out[i].Codigo < out[j].Codigo
		}
		// Último desempate, por ALMACÉN, y no es cosmético: dos casillas «sin ubicar»
		// —una histórica sin almacén y otra dentro de un almacén— empataban en todo lo
		// anterior, y el orden entre ellas lo decidía el recorrido de un mapa. El mismo
		// ajuste consumía una u otra según la corrida: nada fallaba, pero el sitio del
		// que salía la mercancía cambiaba solo.
		//
		// Se gasta primero lo que NO tiene almacén, por el mismo motivo por el que se
		// gasta antes lo que no está ubicado: es el saldo menos localizado que hay —el
		// histórico anterior a los almacenes— y consumirlo hace que lo que queda sea
		// cada vez más fiel.
		if (out[i].AlmacenID == "") != (out[j].AlmacenID == "") {
			return out[i].AlmacenID == ""
		}
		return out[i].AlmacenID < out[j].AlmacenID
	})
	return out
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
func (s *Service) RepartirSalidaFEFO(empresaID, sedeID, almacenID, productoID string, cantidad float64) ([]TramoSalida, error) {
	p, ok := s.productos.ByID(empresaID, productoID)
	if !ok {
		return []TramoSalida{{Cantidad: cantidad}}, nil
	}
	// OJO: esto NO se salta para los productos sin lote. También ellos ocupan una
	// ubicación, y si sus salidas no consumieran de una, el saldo «sin ubicar» se
	// iría a negativo mientras las ubicaciones quedan en positivo: la suma cuadra
	// pero el sitio donde está la mercancía deja de ser cierto.
	//
	// Para un producto sin lotes y sin ubicaciones, bucketsDe devuelve una sola
	// casilla vacía y el reparto sale igual que siempre: un tramo sin nada.
	if cantidad <= 0.0001 {
		return []TramoSalida{}, nil
	}
	disponibles := s.bucketsDe(empresaID, sedeID, almacenID, productoID)

	tramos := []TramoSalida{}
	restante := cantidad
	vencidoDisponible := false
	for _, l := range disponibles {
		if restante <= 0.0001 {
			break
		}
		// UN LOTE VENCIDO NO SALE. FEFO lo pondría el primero —es el que caduca
		// antes— y sin esta guarda el consumo automático despacharía justo lo que no
		// se puede vender, de forma silenciosa y en cada venta.
		//
		// La mercancía vencida se saca del ledger con una merma, que es una decisión
		// de alguien y deja rastro; no desapareciendo por la puerta del mostrador.
		if l.Vencido {
			vencidoDisponible = true
			continue
		}
		toma := l.Cantidad
		if toma > restante {
			toma = restante
		}
		tramos = append(tramos, TramoSalida{
			Lote: l.Lote, Vencimiento: l.Vencimiento,
			AlmacenID: l.AlmacenID, UbicacionID: l.UbicacionID, Cantidad: round2(toma),
		})
		restante = round2(restante - toma)
	}
	if restante > 0.0001 {
		// Se distingue el motivo: no es lo mismo «no hay» que «lo que hay está
		// vencido». El segundo se arregla dando de baja el lote, y decir solo que
		// falta stock mandaría a buscar donde no está el problema.
		if vencidoDisponible {
			return nil, fmt.Errorf("%w: %s (faltan %.2f)", ErrSoloQuedaVencido, p.SKU, restante)
		}
		// Sacar el resto de un lote vacío —o sin lote— dejaría el saldo del producto
		// cuadrado y el de los lotes roto, que es la forma de fallar que este módulo
		// existe para evitar.
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
/* repartirSalidaEntreCasillas reparte una salida entre las casillas reales del
 * producto, acotando a UN LOTE si se declaró.
 *
 * Es el caso de quien cuenta leyendo la etiqueta de la caja: sabe de qué lote es y
 * no mira el número del anaquel. Sin acotar se repartiría por FEFO entre todos los
 * lotes y descontaría del equivocado; sin repartir saldría «sin ubicar» y dejaría
 * esa casilla en negativo con los estantes intactos.
 */
func (s *Service) repartirSalidaEntreCasillas(empresaID, sedeID, almacenID, productoID, lote string, cantidad float64) []TramoSalida {
	if lote == "" {
		return s.repartirSalidaFEFOTolerante(empresaID, sedeID, almacenID, productoID, cantidad)
	}
	cubierto := 0.0
	tramos := []TramoSalida{}
	for _, b := range s.bucketsDe(empresaID, sedeID, almacenID, productoID) {
		if b.Lote != lote || cubierto >= cantidad-0.0001 {
			continue
		}
		toma := b.Cantidad
		if toma > cantidad-cubierto {
			toma = cantidad - cubierto
		}
		tramos = append(tramos, TramoSalida{
			Lote: b.Lote, Vencimiento: b.Vencimiento,
			AlmacenID: b.AlmacenID, UbicacionID: b.UbicacionID, Cantidad: round2(toma),
		})
		cubierto = round2(cubierto + toma)
	}
	// Lo que no cubre ninguna casilla se anota igual: el ajuste no se puede negar a
	// registrar una merma porque el sitio no cuadre, pero queda marcado como
	// descubierto para que se vea dónde hay que ir a mirar.
	if falta := round2(cantidad - cubierto); falta > 0.0001 {
		tramos = append(tramos, TramoSalida{Lote: lote, AlmacenID: almacenID, Cantidad: falta, Descubierto: true})
	}
	return tramos
}

func (s *Service) repartirSalidaFEFOTolerante(empresaID, sedeID, almacenID, productoID string, cantidad float64) []TramoSalida {
	tramos, err := s.RepartirSalidaFEFO(empresaID, sedeID, almacenID, productoID, cantidad)
	if err == nil {
		return tramos
	}
	// Se rehace cubriendo lo que haya y dejando el descubierto sin lote.
	cubierto := 0.0
	tramos = []TramoSalida{}
	for _, l := range s.bucketsDe(empresaID, sedeID, almacenID, productoID) {
		if cubierto >= cantidad-0.0001 {
			continue
		}
		toma := l.Cantidad
		if toma > cantidad-cubierto {
			toma = cantidad - cubierto
		}
		tramos = append(tramos, TramoSalida{
			Lote: l.Lote, Vencimiento: l.Vencimiento,
			AlmacenID: l.AlmacenID, UbicacionID: l.UbicacionID, Cantidad: round2(toma),
		})
		cubierto = round2(cubierto + toma)
	}
	if falta := round2(cantidad - cubierto); falta > 0.0001 {
		// El descubierto se anota en el almacén que pidió la salida: es donde el
		// operador creía tener la mercancía, y es donde hay que ir a cuadrarlo.
		tramos = append(tramos, TramoSalida{AlmacenID: almacenID, Cantidad: falta, Descubierto: true})
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
	for _, t := range s.repartirSalidaFEFOTolerante(base.EmpresaID, base.SedeID, base.AlmacenID, base.ProductoID, -base.Cantidad) {
		m := base
		m.Cantidad = -t.Cantidad
		m.Lote, m.Vencimiento = t.Lote, t.Vencimiento
		m.UbicacionID = t.UbicacionID
		// La salida se escribe DONDE ESTABA la mercancía, incluso si ese almacén es
		// el vacío de los movimientos antiguos. Solo el descubierto se queda en el
		// almacén que pidió la salida.
		if !t.Descubierto {
			m.AlmacenID = t.AlmacenID
		}
		s.movimientos.Append(m)
	}
}

// validarLotesVendibles comprueba que una venta se pueda surtir SIN tocar lotes
// vencidos, antes de emitir nada.
//
// Solo mira los productos que controlan vencimiento: para el resto no hay nada
// que comprobar y el mostrador se comporta igual que siempre.
//
// Se llama ANTES de numerar y anexar el documento a propósito. La factura es de
// solo anexado: fallar después dejaría el folio quemado y media venta registrada,
// que es peor que no dejar vender.
func (s *Service) validarLotesVendibles(empresaID, sedeID string, lineas []fiscal.Linea) error {
	// Se agregan los consumos por producto antes de comprobar: dos líneas del mismo
	// SKU —o dos platos que comparten insumo— tienen que mirarse juntas, o cada una
	// pasaría por su cuenta y entre las dos no habría existencia.
	porProducto := map[string]float64{}
	for _, l := range lineas {
		for _, cs := range consumosDeLinea(l, l.Cantidad) {
			porProducto[cs.ProductoID] += cs.Cantidad
		}
	}
	for prodID, cant := range porProducto {
		p, ok := s.productos.ByID(empresaID, prodID)
		if !ok || !p.RequiereLote || !p.ControlaVencimiento {
			continue
		}
		if _, err := s.RepartirSalidaFEFO(empresaID, sedeID, "", prodID, cant); err != nil {
			return err
		}
	}
	return nil
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
		out = append(out, TramoSalida{
			Lote: m.Lote, Vencimiento: m.Vencimiento, UbicacionID: m.UbicacionID, Cantidad: -m.Cantidad,
		})
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
