package inventario

// APARTADO: mercancía comprometida que todavía no ha salido.
//
// Se llama apartado y no «reserva» porque reserva.Reserva ya existe y es de mesas
// de restaurante. Confundirlas sería fácil y caro.
//
// POR QUÉ EXISTE. Sin esto, una entrega en dos pasos permite VENDER DOS VECES LA
// MISMA UNIDAD: entre que se prepara un pedido y se despacha, la mercancía sigue
// contando como existencia y el mostrador la vende otra vez. Nadie lo nota hasta
// que el segundo cliente se queda sin su pedido, y para entonces las dos ventas
// están hechas.
//
// NO ES UN MOVIMIENTO. Apartar no mueve unidades: la mercancía sigue en el almacén
// y sigue siendo del negocio. Por eso vive en su propio documento y no en el ledger
// —que es solo para lo que ya ocurrió— y por eso no genera asiento: no ha habido
// ni venta ni merma.
//
// NO ES UN CONTADOR. Lo apartado se PROYECTA sumando las líneas de los apartados
// abiertos, igual que la existencia se pliega del ledger. Un contador que se suma
// y se resta a mano se desincroniza en la primera operación que falle a medias, y
// la diferencia no falla: solo hace que el disponible mienta.
type Apartado struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	AlmacenID string `json:"almacenId" bson:"almacenid"`
	// Motivo es para quién se aparta, en palabras de quien lo apartó. Es lo que
	// alguien necesita leer para decidir si puede liberarlo.
	Motivo string          `json:"motivo" bson:"motivo"`
	Estado string          `json:"estado" bson:"estado"`
	Lineas []LineaApartado `json:"lineas" bson:"lineas"`
	// RefTipo y RefID enlazan con lo que lo originó (un pedido, una entrega). Vacíos
	// en un apartado hecho a mano, que es un caso legítimo: apartar para un cliente
	// que va a pasar a buscarlo.
	RefTipo string `json:"refTipo,omitempty" bson:"reftipo,omitempty"`
	RefID   string `json:"refId,omitempty" bson:"refid,omitempty"`
	Actor   string `json:"actor" bson:"actor"`
	Creado  string `json:"creado" bson:"creado"`
	Cerrado string `json:"cerrado,omitempty" bson:"cerrado,omitempty"`
}

// LineaApartado es un producto comprometido.
type LineaApartado struct {
	ProductoID string  `json:"productoId" bson:"productoid"`
	SKU        string  `json:"sku" bson:"sku"`
	Cantidad   float64 `json:"cantidad" bson:"cantidad"`
	// UbicacionID dice de qué casilla se apartó. Con entrega en dos pasos es la
	// ubicación de preparación, donde la mercancía espera físicamente.
	UbicacionID string `json:"ubicacionId,omitempty" bson:"ubicacionid,omitempty"`
	Lote        string `json:"lote,omitempty" bson:"lote,omitempty"`
}

// Estados de un apartado. Solo el ABIERTO resta del disponible: un apartado
// despachado ya salió por el ledger —restarlo otra vez lo contaría dos veces— y
// uno liberado dejó de comprometer nada.
const (
	// ApartadoAbierto compromete la mercancía. Es el único estado que resta.
	ApartadoAbierto = "abierto"
	// ApartadoDespachado: la mercancía salió. El ledger ya lo refleja.
	ApartadoDespachado = "despachado"
	// ApartadoLiberado: alguien decidió que ya no se aparta. No es un borrado: el
	// documento se conserva para poder explicar por qué estuvo comprometido.
	ApartadoLiberado = "liberado"
)

// EstadoApartadoValido acota el estado.
func EstadoApartadoValido(e string) bool {
	switch e {
	case ApartadoAbierto, ApartadoDespachado, ApartadoLiberado:
		return true
	}
	return false
}

// ApartadoRepo persiste apartados, aislado por empresaID.
type ApartadoRepo interface {
	List(empresaID string) []Apartado
	ByID(empresaID, id string) (Apartado, bool)
	Create(a Apartado) Apartado
	Update(a Apartado) (Apartado, bool)
}
