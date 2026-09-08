package fiscal

// MetodoPago es un método de pago configurable por empresa. Cada empresa decide
// qué formas de cobro ofrece y dónde: en el Punto de venta (caja) y/o en el
// módulo Ventas. Enlaza opcionalmente a una CuentaCobro como destino del cobro.
type MetodoPago struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Nombre    string `json:"nombre" bson:"nombre"` // "Efectivo Bs", "Pago Móvil", "Zelle"...
	// Tipo reutiliza las constantes de método de pago del documento fiscal:
	// efectivo_bs | efectivo_usd | pago_movil | zelle | tarjeta | transferencia.
	Tipo   string `json:"tipo" bson:"tipo"`
	Moneda string `json:"moneda" bson:"moneda"` // VES | USD
	// CuentaCobroID es el destino asociado (id de CuentaCobro), opcional.
	CuentaCobroID string `json:"cuentaCobroId" bson:"cuentacobroid"`
	EnCaja        bool   `json:"enCaja" bson:"encaja"`     // disponible en el Punto de venta
	EnVentas      bool   `json:"enVentas" bson:"enventas"` // disponible en el módulo Ventas
	Activo        bool   `json:"activo" bson:"activo"`
	Orden         int    `json:"orden" bson:"orden"`
}

// MetodoPagoRepo persiste métodos de pago, aislado por empresaID.
type MetodoPagoRepo interface {
	List(empresaID string) []MetodoPago
	ByID(empresaID, id string) (MetodoPago, bool)
	Create(m MetodoPago) MetodoPago
	Update(m MetodoPago) (MetodoPago, bool)
	Delete(empresaID, id string) bool
}
