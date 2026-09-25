package application

import (
	"fmt"
	"sort"

	"github.com/mornix/elerp/internal/domain/inventario"
)

/* CHEQUEO DE SALUD DEL INVENTARIO.
 *
 * El inventario se proyecta de varias formas a partir del mismo ledger: por sede,
 * por almacén, por ubicación, por lote, y como saldo de una cuenta contable. Cada
 * proyección se cuidó en su tanda; NINGUNA se había comparado contra las otras.
 *
 * Ese es justo el modo de fallo que este sistema produce: dos proyecciones del
 * mismo dato que discrepan SIN QUE NADA FALLE. Ya ocurrió tres veces —el conteo que
 * decía «+22» donde iba «−3», la valoración que ponía en «Sin almacén» lo que
 * Existencias mostraba en el principal, los dos almacenes homónimos fundidos en una
 * fila—. Las tres se encontraron mirando dos pantallas a la vez, a ojo.
 *
 * Esto lo hace a propósito y de una vez. No arregla nada: dice qué está torcido y
 * dónde, que es lo que hoy no se puede saber sin salir a buscarlo.
 */

// Gravedades de un hallazgo. ALTA es «el dato que se muestra está mal»; MEDIA es
// «esto va a dar problemas» (un costo en cero que envenenará el promedio).
const (
	GravedadAlta  = "alta"
	GravedadMedia = "media"
)

// Clases de hallazgo. Son estables: la pantalla agrupa por ellas.
const (
	ClaseNoCuadraContabilidad = "no-cuadra-con-contabilidad"
	ClaseDocumentoSinAsiento  = "documento-sin-asiento"
	ClaseMovimientoSinAsiento = "movimiento-sin-asiento"
	ClaseAlmacenesNoSuman     = "almacenes-no-suman"
	ClaseUbicacionesNoSuman   = "ubicaciones-no-suman"
	ClaseLotesNoSuman         = "lotes-no-suman"
	ClaseApartadoExcedido     = "apartado-excede-existencia"
	ClaseExistenciaNegativa   = "existencia-negativa"
	ClaseCostoInvalido        = "costo-invalido"
	ClaseIDDuplicado          = "id-de-movimiento-duplicado"
)

// En qué está expresado un hallazgo. La pantalla NO puede adivinarlo: una
// existencia negativa de −726,73 son unidades, y pintarlas como «−Bs 726,73»
// convierte un faltante de vaselina en una pérdida contable que nadie tiene.
const (
	MedidaMonto    = "monto"
	MedidaCantidad = "cantidad"
)

// HallazgoInventario es una discrepancia concreta, con las dos cifras que no
// coinciden. Se guardan LAS DOS a propósito: «no cuadra» no sirve para nada, y la
// diferencia sola tampoco dice cuál de los dos lados hay que mirar.
type HallazgoInventario struct {
	Clase      string  `json:"clase"`
	Gravedad   string  `json:"gravedad"`
	Medida     string  `json:"medida"`
	Detalle    string  `json:"detalle"`
	SKU        string  `json:"sku,omitempty"`
	Ref        string  `json:"ref,omitempty"`
	Esperado   float64 `json:"esperado"`
	Encontrado float64 `json:"encontrado"`
}

// DiagnosticoInventario es el resultado del chequeo completo.
type DiagnosticoInventario struct {
	Fecha     string               `json:"fecha"`
	Sano      bool                 `json:"sano"`
	Revisados int                  `json:"revisados"`
	Hallazgos []HallazgoInventario `json:"hallazgos"`
}

// casiIgual absorbe el ruido de coma flotante. El umbral es de céntimo: por debajo
// no hay diferencia que un humano pueda perseguir, y por encima sí la hay.
func casiIgual(a, b float64) bool {
	d := a - b
	return d > -0.005 && d < 0.005
}

// DiagnosticarInventario corre todas las comprobaciones y devuelve lo que no cuadra.
// Es de SOLO LECTURA: no asienta, no ajusta, no toca el ledger.
func (s *Service) DiagnosticarInventario(empresaID string) DiagnosticoInventario {
	res := DiagnosticoInventario{Fecha: ahora(), Hallazgos: []HallazgoInventario{}}
	if s.productos == nil || s.movimientos == nil {
		return res
	}
	res.Revisados = len(s.movimientos.List(empresaID, inventario.FiltroMovimiento{}))

	add := func(h HallazgoInventario) { res.Hallazgos = append(res.Hallazgos, h) }

	s.revisarIdentidad(empresaID, add)
	s.revisarAsientos(empresaID, add)
	s.revisarProyecciones(empresaID, add)
	s.revisarSaldos(empresaID, add)

	// Orden estable: primero lo grave, y dentro de eso por clase y referencia. Sin
	// esto el informe cambia de orden entre dos llamadas iguales y no se puede
	// comparar con el de ayer.
	//
	// El orden de clases es EXPLÍCITO y no alfabético: arriba el descuadre contra la
	// contabilidad, que es el titular y engloba a los demás; después sus causas
	// conocidas; al final los saldos raros. Alfabéticamente, «existencia-negativa»
	// salía antes que «no-cuadra-con-contabilidad» y el titular quedaba sepultado.
	prioridad := map[string]int{
		ClaseIDDuplicado:          -1,
		ClaseNoCuadraContabilidad: 0,
		ClaseDocumentoSinAsiento:  1,
		ClaseMovimientoSinAsiento: 2,
		ClaseAlmacenesNoSuman:     3,
		ClaseUbicacionesNoSuman:   4,
		ClaseLotesNoSuman:         5,
		ClaseExistenciaNegativa:   6,
		ClaseApartadoExcedido:     7,
		ClaseCostoInvalido:        8,
	}
	sort.SliceStable(res.Hallazgos, func(i, j int) bool {
		a, b := res.Hallazgos[i], res.Hallazgos[j]
		if a.Gravedad != b.Gravedad {
			return a.Gravedad == GravedadAlta
		}
		if a.Clase != b.Clase {
			return prioridad[a.Clase] < prioridad[b.Clase]
		}
		if a.SKU != b.SKU {
			return a.SKU < b.SKU
		}
		return a.Ref < b.Ref
	})
	res.Sano = len(res.Hallazgos) == 0
	return res
}

/* --- Identidad --- */

// revisarIdentidad busca ids de movimiento repetidos.
//
// El ledger es de solo-anexado y TODO lo demás se apoya en que cada movimiento tiene
// una identidad propia: el asiento lo referencia por id, la recontabilización marca
// por id lo que ya asentó, el rastro de lotes enlaza por id. Dos movimientos
// distintos con el mismo id no rompen la suma —el fold recorre la lista, no el
// mapa— pero envenenan todo lo que indexa: asentar uno da por asentado al otro.
//
// Se encontró en la demostración del restaurante: seis ids compartidos entre el
// juego de insumos de cocina y el de repostería, sembrados por caminos distintos que
// reiniciaban la misma secuencia. Se buscaba otra cosa —una proyección que contaba
// el doble— y la causa estaba acá, dos capas más abajo.
func (s *Service) revisarIdentidad(empresaID string, add func(HallazgoInventario)) {
	vistos := map[string]inventario.Movimiento{}
	for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{}) {
		if m.ID == "" {
			continue
		}
		otro, repetido := vistos[m.ID]
		if !repetido {
			vistos[m.ID] = m
			continue
		}
		add(HallazgoInventario{
			Clase: ClaseIDDuplicado, Gravedad: GravedadAlta, Medida: MedidaCantidad,
			Detalle: "el id " + m.ID + " lo usan dos movimientos distintos (" + otro.SKU + " y " + m.SKU + ")",
			SKU:     m.SKU, Ref: m.ID,
			Esperado: round2(otro.Cantidad), Encontrado: round2(m.Cantidad),
		})
	}
}

/* --- Respaldo contable --- */

// revisarAsientos busca inventario que se movió sin que la contabilidad se enterara.
//
// EL PUNTO CIEGO QUE ESTO CIERRA: la recontabilización excluye los movimientos que
// vienen de un documento, y hace bien —su asiento lo emite el documento con el
// importe agregado, asentarlos uno a uno los contaría dos veces—. Pero de ahí no se
// sigue que ese asiento EXISTA: un asiento descuadrado no se guarda, solo se
// registra en el log. Cuando eso pasa, el movimiento queda huérfano para siempre:
// la recontabilización no lo repesca («tiene documento») y nada lo señala.
//
// Por eso acá la pregunta es distinta: no «¿este movimiento tiene su asiento?» sino
// «¿el documento que lo originó asentó algo?». Se agrupa por documento porque es
// como se asienta: un asiento por documento, no uno por línea.
func (s *Service) revisarAsientos(empresaID string, add func(HallazgoInventario)) {
	if s.asientos == nil {
		return
	}
	/* PRIMERO LA PREGUNTA GRANDE: ¿el inventario vale lo que dice la contabilidad?
	 *
	 * Va antes que el detalle porque es la que engloba a todas las demás, y porque
	 * su ausencia dejaba a este chequeo diciendo «sano» sobre una empresa cuya
	 * pantalla de Valoración decía «no cuadra» a la vez —exactamente la clase de
	 * contradicción entre pantallas que esto existe para encontrar—.
	 *
	 * Las causas de abajo (movimiento huérfano, documento que no asentó) explican
	 * ALGUNOS descuadres, no todos: un asiento con el importe equivocado cuadra el
	 * recuento de asientos y descuadra el saldo igual. Por eso se comprueba el saldo
	 * directamente y no solo sus causas conocidas. */
	for _, c := range s.Valoracion(empresaID, "").PorCuenta {
		if c.Cuadra {
			continue
		}
		add(HallazgoInventario{
			Clase: ClaseNoCuadraContabilidad, Gravedad: GravedadAlta, Medida: MedidaMonto,
			Detalle: "el inventario valorado no coincide con el saldo de la cuenta " + c.Clave + " (" + c.Nombre + ")",
			Ref:     c.Clave, Esperado: c.Valor, Encontrado: c.Contable,
		})
	}

	// Referencias que SÍ tienen asiento, por las dos vías: el asiento propio de un
	// movimiento y el asiento del documento.
	conAsiento := map[string]bool{}
	for _, a := range s.LibroDiario(empresaID) {
		if a.RefID != "" {
			conAsiento[a.RefID] = true
		}
	}

	type doc struct {
		refTipo string
		movs    int
		valor   float64
		sku     string
	}
	docs := map[string]*doc{}

	for _, m := range s.movimientosSinAsiento(empresaID) {
		valor := m.Cantidad * m.CostoUnitario
		if valor < 0 {
			valor = -valor
		}
		add(HallazgoInventario{
			Clase: ClaseMovimientoSinAsiento, Gravedad: GravedadAlta, Medida: MedidaMonto,
			Detalle: fmt.Sprintf("movimiento de %s sin asiento (%s)", m.Tipo, m.Motivo),
			SKU:     m.SKU, Ref: m.ID, Esperado: round2(valor), Encontrado: 0,
		})
	}

	for _, m := range s.movimientos.List(empresaID, inventario.FiltroMovimiento{}) {
		if m.RefTipo == "" || m.RefID == "" || conAsiento[m.RefID] {
			continue
		}
		// Las transferencias y traslados no se asientan nunca: la mercancía sigue
		// siendo de la misma empresa y el patrimonio no cambia.
		if m.Tipo == inventario.MovTransferencia {
			continue
		}
		d := docs[m.RefID]
		if d == nil {
			d = &doc{refTipo: m.RefTipo, sku: m.SKU}
			docs[m.RefID] = d
		}
		d.movs++
		v := m.Cantidad * m.CostoUnitario
		if v < 0 {
			v = -v
		}
		d.valor += v
	}
	for refID, d := range docs {
		add(HallazgoInventario{
			Clase: ClaseDocumentoSinAsiento, Gravedad: GravedadAlta, Medida: MedidaMonto,
			Detalle: fmt.Sprintf("%s movió inventario (%d movimiento(s)) y no tiene asiento", d.refTipo, d.movs),
			SKU:     d.sku, Ref: refID, Esperado: round2(d.valor), Encontrado: 0,
		})
	}
}

/* --- Las proyecciones dicen lo mismo --- */

// revisarProyecciones comprueba que sede, almacén, ubicación y lote cuenten la misma
// existencia. Son cuatro lecturas del MISMO ledger: si se separan, alguna pantalla
// está mintiendo y ninguna falla.
func (s *Service) revisarProyecciones(empresaID string, add func(HallazgoInventario)) {
	if s.almacenes == nil {
		return
	}
	for _, sd := range s.sedesDeEmpresa(empresaID) {
		if sd.id == "" {
			continue
		}
		porSede := map[string]float64{}
		for _, e := range s.Existencias(empresaID, sd.id) {
			porSede[e.ProductoID] = e.Cantidad
		}

		// Suma de los almacenes de la sede == existencia de la sede.
		sumaAlmacenes := map[string]float64{}
		for _, a := range s.AlmacenesDeSede(empresaID, sd.id) {
			filas, err := s.ExistenciasDeAlmacen(empresaID, a.ID)
			if err != nil {
				continue
			}
			for _, f := range filas {
				if _, cuenta := porSede[f.ProductoID]; !cuenta {
					// Un producto que la proyección por sede no lista (no se
					// stockea) tampoco debe sumar acá; si suma, las dos
					// proyecciones no están usando el mismo criterio.
					if !casiIgual(f.Cantidad, 0) {
						add(HallazgoInventario{
							Clase: ClaseAlmacenesNoSuman, Gravedad: GravedadAlta, Medida: MedidaCantidad,
							Detalle: "el almacén " + a.Nombre + " tiene existencia de un producto que la proyección por sede no lista",
							SKU:     f.SKU, Ref: a.ID, Esperado: 0, Encontrado: round2(f.Cantidad),
						})
					}
					continue
				}
				sumaAlmacenes[f.ProductoID] += f.Cantidad
			}

			// Suma de las ubicaciones del almacén == existencia del almacén.
			s.revisarUbicacionesDe(empresaID, sd.id, a.ID, a.Nombre, filas, add)
		}
		for prodID, cant := range porSede {
			if !casiIgual(cant, sumaAlmacenes[prodID]) {
				add(HallazgoInventario{
					Clase: ClaseAlmacenesNoSuman, Gravedad: GravedadAlta, Medida: MedidaCantidad,
					Detalle: "los almacenes de la sede no suman la existencia de la sede",
					SKU:     s.skuDe(empresaID, prodID), Ref: sd.id,
					Esperado: round2(cant), Encontrado: round2(sumaAlmacenes[prodID]),
				})
			}
		}

		// Suma por lote == existencia del producto en la sede. Solo se comprueba
		// donde hay lotes: un producto sin trazabilidad no tiene nada que sumar.
		for prodID, cant := range porSede {
			saldos := s.SaldosPorLote(empresaID, sd.id, prodID)
			if len(saldos) == 0 {
				continue
			}
			suma := 0.0
			for _, sl := range saldos {
				suma += sl.Cantidad
			}
			if !casiIgual(cant, suma) {
				add(HallazgoInventario{
					Clase: ClaseLotesNoSuman, Gravedad: GravedadAlta, Medida: MedidaCantidad,
					Detalle: "los lotes no suman la existencia del producto en la sede",
					SKU:     s.skuDe(empresaID, prodID), Ref: sd.id,
					Esperado: round2(cant), Encontrado: round2(suma),
				})
			}
		}
	}
}

func (s *Service) revisarUbicacionesDe(empresaID, sedeID, almacenID, almacenNombre string, filas []ExistenciaView, add func(HallazgoInventario)) {
	if s.ubicaciones == nil {
		return
	}
	ubis := s.UbicacionesDe(empresaID, almacenID)
	if len(ubis) == 0 {
		return // sin ubicaciones declaradas no hay reparto que comprobar
	}
	for _, f := range filas {
		if casiIgual(f.Cantidad, 0) {
			continue
		}
		suma := 0.0
		for _, b := range s.bucketsDe(empresaID, sedeID, almacenID, f.ProductoID) {
			suma += b.Cantidad
		}
		if !casiIgual(f.Cantidad, suma) {
			add(HallazgoInventario{
				Clase: ClaseUbicacionesNoSuman, Gravedad: GravedadAlta, Medida: MedidaCantidad,
				Detalle: "las ubicaciones no suman la existencia del almacén " + almacenNombre,
				SKU:     f.SKU, Ref: almacenID,
				Esperado: round2(f.Cantidad), Encontrado: round2(suma),
			})
		}
	}
}

/* --- Saldos que no pueden ser --- */

// revisarSaldos busca estados imposibles o envenenados: existencia negativa,
// apartado que promete más de lo que hay, y costo cero sobre mercancía que existe.
func (s *Service) revisarSaldos(empresaID string, add func(HallazgoInventario)) {
	for _, sd := range s.sedesDeEmpresa(empresaID) {
		if sd.id == "" {
			continue
		}
		for _, e := range s.Existencias(empresaID, sd.id) {
			if e.Cantidad < -0.005 {
				add(HallazgoInventario{
					Clase: ClaseExistenciaNegativa, Gravedad: GravedadAlta, Medida: MedidaCantidad,
					Detalle: "existencia negativa: salió más de lo que había",
					SKU:     e.SKU, Ref: sd.id, Esperado: 0, Encontrado: round2(e.Cantidad),
				})
			}
			// Un costo promedio en cero con mercancía en la mano envenena todo lo que
			// venga después: la salida se valora a cero y el costo de ventas se hunde
			// sin que ninguna cifra parezca rota.
			if e.Cantidad > 0.005 && e.CostoPromedio <= 0 {
				add(HallazgoInventario{
					Clase: ClaseCostoInvalido, Gravedad: GravedadMedia, Medida: MedidaMonto,
					Detalle: "hay existencia pero el costo promedio es cero o negativo",
					SKU:     e.SKU, Ref: sd.id, Esperado: 0, Encontrado: round2(e.CostoPromedio),
				})
			}
			// El apartado solo se compara si hay algo apartado. Sin este filtro, una
			// existencia NEGATIVA disparaba «hay más apartado que existencia» con
			// cero apartado —cierto y del todo inútil—: duplicaba el hallazgo de la
			// existencia negativa y mandaba a revisar las entregas, que no tienen
			// nada que ver.
			if s.apartados != nil {
				apartado := s.Apartado(empresaID, sd.id, e.ProductoID)
				if apartado > 0.005 && apartado > e.Cantidad+0.005 {
					add(HallazgoInventario{
						Clase: ClaseApartadoExcedido, Gravedad: GravedadAlta, Medida: MedidaCantidad,
						Detalle: "hay más apartado que existencia: alguna entrega no se va a poder cumplir",
						SKU:     e.SKU, Ref: sd.id,
						Esperado: round2(e.Cantidad), Encontrado: round2(apartado),
					})
				}
			}
		}
	}
}

func (s *Service) skuDe(empresaID, productoID string) string {
	if p, ok := s.productos.ByID(empresaID, productoID); ok {
		return p.SKU
	}
	return productoID
}
