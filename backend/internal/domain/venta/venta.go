// Package venta modela la VENTA EN ESPERA: el carrito que el cajero aparta para
// atender al siguiente cliente (flujo 2.6).
//
// Es lo contrario de un documento fiscal: NO es inmutable, no tiene numeración y
// se borra al retomarla o al descartarla. Existe porque en el mostrador pasa todo
// el tiempo —«voy al cajero automático y vuelvo»— y sin esto el cajero pierde el
// carrito o bloquea la cola.
//
// Vive en el servidor, no en el navegador, por tres razones: sobrevive a que se
// recargue la página o se caiga la tablet, la puede retomar OTRO cajero del turno
// siguiente (es lo que dice el prototipo: «cualquier cajero puede retomarlas»), y
// queda acotada a su sede.
package venta

// Linea es un renglón del carrito apartado. Se guarda con el precio y la
// condición de IVA del momento: si el precio del catálogo cambia mientras la
// venta espera, se retoma tal como se armó y el cajero decide.
type Linea struct {
	SKU            string  `json:"sku" bson:"sku"`
	Nombre         string  `json:"nombre" bson:"nombre"`
	Cantidad       float64 `json:"cantidad" bson:"cantidad"`
	PrecioUnitario float64 `json:"precioUnitario" bson:"preciounitario"`
	Exento         bool    `json:"exento" bson:"exento"`
}

// EnEspera es un carrito apartado.
type EnEspera struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// Nota libre para reconocerla («el señor de la camisa azul»). Opcional.
	Nota string `json:"nota" bson:"nota"`
	// ClienteID si ya se había elegido cliente.
	ClienteID string `json:"clienteId" bson:"clienteid"`
	// CajeroNombre y CajaCodigo son de quien la apartó: la lista dice de quién es
	// aunque la retome otro.
	CajeroNombre string  `json:"cajeroNombre" bson:"cajeronombre"`
	CajaCodigo   string  `json:"cajaCodigo" bson:"cajacodigo"`
	ActorID      string  `json:"actorId" bson:"actorid"`
	Lineas       []Linea `json:"lineas" bson:"lineas"`
	// Total es el del momento en que se apartó, solo para pintar la lista. El
	// cálculo fiscal real lo hace el servidor al emitir, nunca este número.
	Total  float64 `json:"total" bson:"total"`
	Creada string  `json:"creada" bson:"creada"` // UTC RFC3339
}

// Repository es el puerto de persistencia. Toda consulta se aísla por empresaID.
//
// A diferencia de los ledgers, acá SÍ hay Delete: una venta en espera es un
// borrador de mostrador, no un registro contable. Retomarla la consume.
type Repository interface {
	// List devuelve las ventas apartadas de una empresa, opcionalmente de una
	// sede (las de otra tienda no le sirven a este mostrador).
	List(empresaID, sedeID string) []EnEspera
	ByID(empresaID, id string) (EnEspera, bool)
	Create(v EnEspera) EnEspera
	Delete(empresaID, id string) bool
}
