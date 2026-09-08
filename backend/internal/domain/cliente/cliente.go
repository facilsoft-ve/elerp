// Package cliente es el mínimo de CRM que el POS necesita: alta e
// identificación fiscal del cliente (documento único por empresa).
package cliente

// Tipos de documento de identidad fiscal venezolano.
const (
	DocV = "V" // cédula venezolana
	DocE = "E" // cédula extranjera
	DocJ = "J" // RIF jurídico
	DocG = "G" // RIF gubernamental
	DocP = "P" // pasaporte
)

// Orígenes de un cliente. Trazan de dónde vino el registro para poder
// importar/sincronizar desde otro software sin perder la procedencia.
const (
	OrigenManual    = "manual"    // cargado a mano en ElERP
	OrigenOdoo      = "odoo"      // importado/sincronizado desde Odoo
	OrigenImportado = "importado" // importado desde otro CRM/ERP genérico
)

// Cliente es un tercero comprador de la empresa.
//
// A diferencia de los documentos fiscales (append-only), el maestro de clientes
// SÍ se edita (Update): es un maestro, no un ledger. Corregir el teléfono o el
// correo de un cliente no reescribe historia contable.
type Cliente struct {
	ID            string `json:"id" bson:"id"`
	EmpresaID     string `json:"empresaId" bson:"empresaid"`
	Nombre        string `json:"nombre" bson:"nombre"`
	TipoDocumento string `json:"tipoDocumento" bson:"tipodocumento"`
	Documento     string `json:"documento" bson:"documento"`
	Telefono      string `json:"telefono" bson:"telefono"`
	Direccion     string `json:"direccion" bson:"direccion"`
	Email         string `json:"email" bson:"email"`

	// Información ampliada (CRM). Todos opcionales.
	NombreComercial string `json:"nombreComercial" bson:"nombrecomercial"`
	Contacto        string `json:"contacto" bson:"contacto"` // persona de contacto
	Notas           string `json:"notas" bson:"notas"`
	Activo          bool   `json:"activo" bson:"activo"` // desactivar/reactivar sin borrar

	// Interoperabilidad con otros ERP/CRM (Odoo u otro). Estos campos dejan el
	// modelo LISTO para importar/sincronizar terceros desde un sistema externo
	// manteniendo la trazabilidad y permitiendo el round-trip:
	//   - Origen: cómo entró el registro (OrigenManual/OrigenOdoo/OrigenImportado).
	//   - SistemaExterno: nombre/instancia del sistema de origen (p. ej. "odoo").
	//   - IdExterno: identidad del registro en ese sistema (para reconciliar y
	//     evitar duplicados en una futura sincronización bidireccional).
	Origen         string `json:"origen" bson:"origen"`
	SistemaExterno string `json:"sistemaExterno" bson:"sistemaexterno"`
	IdExterno      string `json:"idExterno" bson:"idexterno"`
}

// Repository persiste clientes, aislado por empresaID.
type Repository interface {
	List(empresaID string) []Cliente
	ByID(empresaID, id string) (Cliente, bool)
	ByDocumento(empresaID, tipoDoc, documento string) (Cliente, bool)
	Create(c Cliente) Cliente
	// Update reemplaza un cliente existente del tenant (maestro editable, no
	// ledger). Devuelve false si el id no pertenece a la empresa.
	Update(c Cliente) (Cliente, bool)
}
