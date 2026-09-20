// Package compra modela la Orden de compra: el flujo de aprovisionamiento
// borrador → confirmada → recepción (parcial/total). Es análogo a cotizacion
// pero del lado de la COMPRA: maneja costos (no precios de venta) y su recepción
// ALIMENTA el ledger de inventario (append-only) con movimientos de entrada al
// costo de la orden, y deriva el asiento de compra (Inventario / Cuentas por
// pagar). La orden en sí es un documento con estado; el hecho económico —la
// entrada de stock— vive en el ledger inmutable, nunca en un contador editable.
package compra

// Estados de la máquina de una orden de compra.
const (
	OCBorrador        = "borrador"
	OCConfirmada      = "confirmada"
	OCRecibidaParcial = "recibida_parcial"
	OCRecibida        = "recibida"
	OCCancelada       = "cancelada"
)

// Linea es un renglón de la orden. CostoUnitario es el costo NETO (sin IVA): es
// lo que entra al Kardex cuando se recibe. CantidadRecibida es la proyección de
// lo ya recepcionado por este renglón; nunca puede exceder Cantidad.
type Linea struct {
	ProductoID       string  `json:"productoId" bson:"productoid"`
	SKU              string  `json:"sku" bson:"sku"`
	Nombre           string  `json:"nombre" bson:"nombre"`
	Cantidad         float64 `json:"cantidad" bson:"cantidad"`
	CostoUnitario    float64 `json:"costoUnitario" bson:"costounitario"`
	CantidadRecibida float64 `json:"cantidadRecibida" bson:"cantidadrecibida"`
	// Total es el NETO de la línea = cantidad * costoUnitario (sin IVA).
	Total float64 `json:"total" bson:"total"`
	// Exento marca la línea como no gravada con IVA crédito fiscal.
	Exento bool `json:"exento" bson:"exento"`
	// ConceptoISLR es el código del concepto de retención que trae el PRODUCTO
	// (inventario.Producto.ConceptoISLR), copiado al armar la orden. Vacío = no
	// sujeto, que es el caso de toda mercancía.
	//
	// Se copia en vez de leerse del catálogo al proyectar porque la orden tiene que
	// seguir explicando su número cuando alguien reclasifique el producto mañana —
	// la misma razón por la que se copia la tasa de cambio.
	ConceptoISLR string `json:"conceptoIslr,omitempty" bson:"conceptoislr,omitempty"`
}

// RetencionISLRProyectada es una fila del desglose de ISLR de la orden: un
// concepto, la base que le corresponde y lo que se retendría por él.
//
// El desglose existe porque el ISLR se retiene POR CONCEPTO DEL PAGO, y una sola
// orden puede mezclarlos: honorarios de un técnico y el flete de lo que trajo se
// retienen a tarifas distintas y se declaran por separado. Un solo par
// concepto/porcentaje en la orden no puede expresar eso.
type RetencionISLRProyectada struct {
	Codigo     string  `json:"codigo" bson:"codigo"`
	Concepto   string  `json:"concepto" bson:"concepto"`
	Base       float64 `json:"base" bson:"base"`
	Porcentaje float64 `json:"porcentaje" bson:"porcentaje"`
	Sustraendo float64 `json:"sustraendo" bson:"sustraendo"`
	Monto      float64 `json:"monto" bson:"monto"`
	// SinTarifa marca el concepto que el maestro NO tiene cargado para el tipo de
	// sujeto de este proveedor (p. ej. un flete comprado a una persona natural
	// cuando la tabla solo trae la tarifa de jurídica). La fila se guarda con monto
	// 0 y esta marca a propósito: si se omitiera, una configuración incompleta se
	// vería igual que «a este proveedor no se le retiene», y nadie la buscaría.
	SinTarifa bool `json:"sinTarifa,omitempty" bson:"sintarifa,omitempty"`
}

// OrdenCompra es un pedido de aprovisionamiento a un proveedor.
type OrdenCompra struct {
	ID              string `json:"id" bson:"id"`
	EmpresaID       string `json:"empresaId" bson:"empresaid"` // tenant
	SedeID          string `json:"sedeId" bson:"sedeid"`       // sede que recibe
	ProveedorID     string `json:"proveedorId" bson:"proveedorid"`
	ProveedorNombre string `json:"proveedorNombre" bson:"proveedornombre"`

	Numero         int    `json:"numero" bson:"numero"`
	NumeroCompleto string `json:"numeroCompleto" bson:"numerocompleto"` // serie "OC" vía Numerador
	Serie          string `json:"serie" bson:"serie"`
	Estado         string `json:"estado" bson:"estado"`

	Lineas   []Linea `json:"lineas" bson:"lineas"`
	Subtotal float64 `json:"subtotal" bson:"subtotal"`
	IVA      float64 `json:"iva" bson:"iva"`
	Total    float64 `json:"total" bson:"total"`

	Moneda     string  `json:"moneda" bson:"moneda"`
	TasaCambio float64 `json:"tasaCambio" bson:"tasacambio"`
	TasaFuente string  `json:"tasaFuente" bson:"tasafuente"`

	CondicionesPago string `json:"condicionesPago" bson:"condicionespago"`
	Notas           string `json:"notas" bson:"notas"`

	// --- Retenciones PROYECTADAS ---------------------------------------------
	//
	// Instantánea del perfil fiscal del proveedor al crear la orden, para responder
	// desde el pedido «¿cuánto le vamos a pagar de verdad?». Es una PROYECCIÓN, no
	// un comprobante: el comprobante de retención se emite sobre la FACTURA de
	// compra —nunca sobre la orden— porque es el documento que lo sustenta ante el
	// SENIAT. Se guarda en la orden (y no se recalcula al vuelo) por la misma razón
	// que la tasa de cambio: el perfil del proveedor puede cambiar mañana y la orden
	// tiene que seguir explicando el número con el que se pactó.
	//
	// Los montos valen 0 cuando la empresa no es agente de retención de ese
	// impuesto o el proveedor no lo tiene activado.
	RetencionIVAPorcentaje float64 `json:"retencionIvaPorcentaje" bson:"retencionivaporcentaje"`
	RetencionIVAMonto      float64 `json:"retencionIvaMonto" bson:"retencionivamonto"`

	// RetencionISLRDetalle es el desglose POR CONCEPTO y la fuente de verdad del
	// ISLR de la orden. Los cuatro escalares de abajo se conservan por
	// compatibilidad y porque el caso de un solo concepto es el habitual:
	//
	//   - 0 conceptos ⇒ todo en cero.
	//   - 1 concepto  ⇒ los escalares lo describen (como siempre).
	//   - N conceptos ⇒ Monto es la SUMA y Concepto lista los nombres, pero
	//     Porcentaje y Sustraendo quedan en 0: no existe una tarifa única que
	//     describa la mezcla, e inventar un promedio sería un número que nadie
	//     podría declarar. Quien muestre un porcentaje tiene que mirar el detalle.
	RetencionISLRDetalle []RetencionISLRProyectada `json:"retencionIslrDetalle,omitempty" bson:"retencionislrdetalle,omitempty"`

	RetencionISLRConcepto   string  `json:"retencionIslrConcepto" bson:"retencionislrconcepto"`
	RetencionISLRPorcentaje float64 `json:"retencionIslrPorcentaje" bson:"retencionislrporcentaje"`
	RetencionISLRSustraendo float64 `json:"retencionIslrSustraendo" bson:"retencionislrsustraendo"`
	RetencionISLRMonto      float64 `json:"retencionIslrMonto" bson:"retencionislrmonto"`

	// NetoAPagar es Total − retenciones: lo que efectivamente recibe el proveedor.
	// Lo retenido no se le paga a él, se entera al SENIAT.
	NetoAPagar float64 `json:"netoAPagar" bson:"netoapagar"`

	Actor       string `json:"actor" bson:"actor"`
	Creada      string `json:"creada" bson:"creada"`           // UTC RFC3339
	Actualizada string `json:"actualizada" bson:"actualizada"` // UTC RFC3339
}

// Repository persiste órdenes de compra, aislado por empresaID. Igual que
// cotizacion: la orden cambia de estado (Update), pero los movimientos que su
// recepción emite van al ledger inmutable.
type Repository interface {
	List(empresaID string) []OrdenCompra
	ByID(empresaID, id string) (OrdenCompra, bool)
	Create(o OrdenCompra) OrdenCompra
	Update(o OrdenCompra) (OrdenCompra, bool)
}
