// Package fabricacion modela la PRODUCCIÓN: convertir insumos en un producto
// terminado que entra al inventario.
//
// POR QUÉ EXISTE, Y QUÉ CAMBIA RESPECTO DE LO QUE YA HABÍA.
//
// El módulo Restaurante ya sabía fabricar: al facturar un plato, el inventario
// descuenta sus INSUMOS en vez del plato. Eso funciona para lo que se prepara en
// el momento —una pasta se hace cuando la piden— pero no para lo que se produce
// ANTES y se guarda: una bandeja de postres, un lote de pan, una pieza armada en
// un taller. Ahí el producto terminado existe en el estante, se cuenta, se
// vence, se puede agotar, y venderlo NO debe volver a consumir insumos porque ya
// se consumieron cuando se fabricó.
//
// Por eso la diferencia no vive en el módulo sino en el PRODUCTO: un producto
// con receta se fabrica BAJO PEDIDO (el comportamiento de siempre) o PARA STOCK
// (se fabrica con una orden y después se vende como cualquier mercancía). Un
// mismo local puede tener los dos: la pasta bajo pedido y el postre para stock.
//
// LO QUE NO SE NEGOCIA: el producto terminado entra al inventario por el LEDGER,
// con el costo REAL de lo que consumió. No se teclea un costo de fabricación —se
// deriva de los insumos que salieron— porque un costo inventado vuelve inútil el
// margen y la valorización, que es justamente para lo que sirve fabricar con un
// sistema y no con un cuaderno.
package fabricacion

import "strings"

/* ESTADOS DE UNA ORDEN.
 *
 * Tres, y ninguno de adorno:
 *
 *   · BORRADOR se planifica y se puede corregir. No ha tocado el inventario.
 *   · EN PROCESO ya CONSUMIÓ los insumos: salieron del almacén y están en la
 *     mesa de trabajo. Esto es lo que evita que otra orden los tome.
 *   · TERMINADA ingresó el producto al inventario con su costo real.
 *
 * Y CANCELADA, que devuelve los insumos si ya habían salido. Una orden no se
 * borra: lo que consumió y lo que produjo son hechos del ledger.
 */
const (
	EstadoBorrador  = "borrador"
	EstadoEnProceso = "en_proceso"
	EstadoTerminada = "terminada"
	EstadoCancelada = "cancelada"
)

// Transiciones permitidas. Se declara el grafo en vez de repartir ifs: una orden
// terminada que pudiera volver a consumir insumos duplicaría el costo.
var transiciones = map[string][]string{
	EstadoBorrador:  {EstadoEnProceso, EstadoCancelada},
	EstadoEnProceso: {EstadoTerminada, EstadoCancelada},
	EstadoTerminada: {},
	EstadoCancelada: {},
}

// PuedePasarA indica si el cambio de estado es válido.
func PuedePasarA(desde, hasta string) bool {
	for _, x := range transiciones[desde] {
		if x == hasta {
			return true
		}
	}
	return false
}

// Final indica si la orden ya no admite cambios.
func Final(estado string) bool { return estado == EstadoTerminada || estado == EstadoCancelada }

/* Orden es una orden de fabricación.
 *
 * Los CONSUMOS se congelan al arrancar (no al planificar): la receta del
 * catálogo puede cambiar mañana, y una orden tiene que poder explicar con qué se
 * hizo lo que se hizo. Es el mismo criterio del snapshot de receta en la línea
 * fiscal.
 */
type Orden struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// AlmacenID es de dónde salen los insumos y a dónde entra lo producido. Vacío
	// ⇒ el almacén principal de la sede.
	AlmacenID string `json:"almacenId,omitempty" bson:"almacenid,omitempty"`
	// Numero es el correlativo visible (OF-000001): quien trabaja en el taller lo
	// dice en voz alta, no lee un identificador.
	Numero         int    `json:"numero" bson:"numero"`
	NumeroCompleto string `json:"numeroCompleto" bson:"numerocompleto"`

	ProductoID string  `json:"productoId" bson:"productoid"`
	SKU        string  `json:"sku" bson:"sku"`
	Nombre     string  `json:"nombre" bson:"nombre"`
	Cantidad   float64 `json:"cantidad" bson:"cantidad"`

	/* CantidadProducida es lo que REALMENTE salió, y puede ser menor que lo
	 * planificado: de una masa para 20 panes salen 18 porque dos se quemaron. Se
	 * declara al terminar y es la que entra al inventario.
	 *
	 * Que difiera no es un error que haya que impedir: es información. Lo que no
	 * puede pasar es que el costo de los 20 se reparta entre los 18 sin que nadie
	 * lo vea — por eso la merma queda explícita. */
	CantidadProducida float64 `json:"cantidadProducida" bson:"cantidadproducida"`

	Estado string `json:"estado" bson:"estado"`

	// Consumos es lo que se sacó del almacén, congelado al arrancar.
	Consumos []Consumo `json:"consumos" bson:"consumos"`
	// CostoTotal es lo que costaron los insumos consumidos, y CostoUnitario el
	// costo con que el producto terminado entra al Kardex. Los dos se DERIVAN: no
	// se teclean.
	CostoTotal    float64 `json:"costoTotal" bson:"costototal"`
	CostoUnitario float64 `json:"costoUnitario" bson:"costounitario"`

	/* PesoUnitario es el peso final de UNA unidad producida, en kilogramos.
	 *
	 * Existe porque hay producto terminado que se vende por peso: una torta se
	 * fabrica «una torta» y se vende por kilo. Sin el peso, lo fabricado y lo
	 * vendido son unidades distintas y el inventario no cierra. Cero ⇒ el
	 * producto se vende por unidad, que es el caso normal. */
	PesoUnitario float64 `json:"pesoUnitario,omitempty" bson:"pesounitario,omitempty"`

	// Lote y Vencimiento del producto terminado, cuando lo exige. Lo fabricado
	// HOY es un lote nuevo: por eso se declaran acá y no se heredan del insumo.
	Lote        string `json:"lote,omitempty" bson:"lote,omitempty"`
	Vencimiento string `json:"vencimiento,omitempty" bson:"vencimiento,omitempty"`

	// Origen enlaza con el pedido o la comanda que la disparó, cuando se fabricó
	// contra una demanda concreta. Vacío ⇒ se fabricó para tener.
	OrigenTipo string `json:"origenTipo,omitempty" bson:"origentipo,omitempty"`
	OrigenID   string `json:"origenId,omitempty" bson:"origenid,omitempty"`

	Nota      string   `json:"nota,omitempty" bson:"nota,omitempty"`
	Bitacora  []Evento `json:"bitacora" bson:"bitacora"`
	Creada    string   `json:"creada" bson:"creada"`
	Iniciada  string   `json:"iniciada,omitempty" bson:"iniciada,omitempty"`
	Terminada string   `json:"terminada,omitempty" bson:"terminada,omitempty"`
	_         struct{} `bson:"-"`
}

// Consumo es un insumo que la orden sacó del almacén, con su costo del momento.
type Consumo struct {
	SKU        string  `json:"sku" bson:"sku"`
	ProductoID string  `json:"productoId" bson:"productoid"`
	Nombre     string  `json:"nombre" bson:"nombre"`
	Cantidad   float64 `json:"cantidad" bson:"cantidad"`
	// CostoUnitario es el promedio del insumo al momento de consumirlo. Se
	// congela porque el promedio se mueve con cada compra, y el costo de lo ya
	// fabricado no puede cambiar retroactivamente.
	CostoUnitario float64 `json:"costoUnitario" bson:"costounitario"`
}

// Total es lo que costó este consumo.
func (c Consumo) Total() float64 { return c.Cantidad * c.CostoUnitario }

// Evento es una entrada de la bitácora de la orden.
type Evento struct {
	Cuando string `json:"cuando" bson:"cuando"`
	Estado string `json:"estado" bson:"estado"`
	Actor  string `json:"actor" bson:"actor"`
	Nota   string `json:"nota,omitempty" bson:"nota,omitempty"`
}

// Merma es lo que se planificó y no salió. Positiva cuando se produjo de menos.
func (o Orden) Merma() float64 {
	if o.Estado != EstadoTerminada {
		return 0
	}
	return o.Cantidad - o.CantidadProducida
}

// Normalizar acota los campos de texto y los mínimos.
func (o *Orden) Normalizar() {
	o.Nota = strings.TrimSpace(o.Nota)
	o.Lote = strings.TrimSpace(o.Lote)
	o.Vencimiento = strings.TrimSpace(o.Vencimiento)
	if o.Cantidad < 0 {
		o.Cantidad = 0
	}
	if o.PesoUnitario < 0 {
		o.PesoUnitario = 0
	}
}

// Repository persiste las órdenes, aislado por empresaID. Las órdenes se editan
// mientras son borrador, así que hay Update; lo que NUNCA se edita son los
// movimientos de inventario que generan.
type Repository interface {
	Append(o Orden) Orden
	Update(o Orden) (Orden, bool)
	ByID(empresaID, id string) (Orden, bool)
	List(empresaID, sedeID string) []Orden
}
