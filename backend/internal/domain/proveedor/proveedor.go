// Package proveedor es el maestro de terceros vendedores de la empresa: el
// espejo de cliente pero del lado de las Compras. Un proveedor identifica a
// quien surte mercancía (RIF, contacto) y se DESACTIVA en vez de borrarse, para
// no perder el rastro de las órdenes de compra que lo referencian.
package proveedor

// Proveedor es un tercero que surte mercancía o servicios a la empresa.
type Proveedor struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"` // tenant
	Nombre    string `json:"nombre" bson:"nombre"`
	// Documento es el RIF (o cédula) del proveedor. No se exige único: un mismo
	// RIF puede reaparecer si se dio de baja y se vuelve a registrar.
	Documento string `json:"documento" bson:"documento"`
	Email     string `json:"email" bson:"email"`
	Telefono  string `json:"telefono" bson:"telefono"`
	Direccion string `json:"direccion" bson:"direccion"`
	// Activo permite la baja REVERSIBLE (nunca se borra): un proveedor inactivo
	// no aparece para nuevas órdenes pero sigue enlazado a su histórico.
	Activo bool   `json:"activo" bson:"activo"`
	Creado string `json:"creado" bson:"creado"` // UTC RFC3339

	// --- Perfil comercial -----------------------------------------------------

	// CondicionPago es la condición pactada con este proveedor ("Contado",
	// "30 días", …). Es el VALOR POR DEFECTO que propone la orden de compra; la
	// orden guarda el suyo, así que cambiarlo acá no reescribe órdenes pasadas.
	// Vacío ⇒ la orden arranca en "Contado".
	CondicionPago string `json:"condicionPago" bson:"condicionpago"`

	// --- Perfil fiscal: retenciones -------------------------------------------
	//
	// Estos campos NO deciden por sí solos que se retenga: la empresa tiene que ser
	// agente de retención del impuesto (empresa.AgenteRetencionIVA / ISLR). Son el
	// perfil de ESTE proveedor, que se proyecta en la orden de compra para saber
	// cuánto se le va a pagar de verdad, y que prellena el comprobante de retención
	// cuando llega su factura. El comprobante se emite SIEMPRE sobre la factura —
	// nunca sobre la orden—, así que en la orden esto es una proyección.

	// ContribuyenteEspecial marca al proveedor como contribuyente especial. Es
	// informativo para quien decide el trato fiscal; no cambia ningún cálculo.
	ContribuyenteEspecial bool `json:"contribuyenteEspecial" bson:"contribuyenteespecial"`

	// RetieneIVA indica si a este proveedor se le retiene IVA.
	// RetencionIVAPorcentaje es el % a retener sobre el IVA de la factura
	// (típicamente 75 o 100). 0 ⇒ se usa el porcentaje por defecto de la empresa.
	RetieneIVA             bool    `json:"retieneIva" bson:"retieneiva"`
	RetencionIVAPorcentaje float64 `json:"retencionIvaPorcentaje" bson:"retencionivaporcentaje"`

	// RetieneISLR indica si a este proveedor se le retiene ISLR.
	//
	// La tarifa NO se guarda acá: se resuelve contra el MAESTRO de conceptos
	// (fiscal.ConceptoISLR) con el código y el tipo de sujeto. Guardar el
	// porcentaje en la ficha del proveedor era reintroducir el problema que el
	// maestro vino a eliminar —el mismo concepto escrito de cinco maneras, tarifas
	// tecleadas de memoria, y después nada auditable por concepto— y además ignoraba
	// que un mismo concepto tiene DOS tarifas según a quién se le retiene.
	RetieneISLR bool `json:"retieneIslr" bson:"retieneislr"`
	// ConceptoISLRCodigo referencia el maestro ("honorarios", "fletes"…). La tarifa
	// y el sustraendo salen de ahí, y el comprobante los COPIA al emitirse: cambiar
	// la tabla mañana no altera lo ya retenido.
	ConceptoISLRCodigo string `json:"conceptoIslrCodigo" bson:"conceptoislrcodigo"`
	// SujetoISLR es qué es este proveedor ante el reglamento: persona natural
	// residente o jurídica domiciliada (fiscal.Sujeto*). Es atributo del PROVEEDOR,
	// no de cada compra: una empresa es jurídica siempre. De él depende cuál de las
	// dos tarifas del concepto aplica.
	SujetoISLR string `json:"sujetoIslr" bson:"sujetoislr"`
}

// Repository persiste proveedores, aislado por empresaID. Igual que
// cliente.Repository, con Update para la edición y la baja reversible.
type Repository interface {
	List(empresaID string) []Proveedor
	ByID(empresaID, id string) (Proveedor, bool)
	Create(p Proveedor) Proveedor
	Update(p Proveedor) (Proveedor, bool)
}
