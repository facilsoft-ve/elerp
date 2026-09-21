package almacen

import "strings"

// TIPO DE OPERACIÓN: la configuración de CÓMO entra y sale la mercancía.
//
// POR QUÉ EXISTE. Hoy una recepción siempre hace lo mismo: entra al almacén
// principal, de una vez, sin revisión. Eso le sirve a una bodega y no le sirve a un
// depósito donde lo que llega se revisa antes de subirlo al anaquel, ni a uno donde
// lo vendido se prepara y después se despacha. La diferencia no es de código: es de
// cuántos pasos tiene el proceso y a qué ubicación va cada uno.
//
// QUÉ NO ES. No es un motor de rutas. En Odoo, los tipos de operación cuelgan de
// reglas push/pull que encadenan operaciones automáticamente por producto y por
// ruta; es la parte donde los clientes se pierden y donde más soporte se consume.
// Aquí un tipo de operación solo responde tres preguntas: en cuántos pasos, a qué
// ubicación entra, y desde cuál sale.
//
// COMPATIBILIDAD. Una empresa sin tipos configurados se comporta EXACTAMENTE como
// antes: un paso, almacén principal. Los cuatro tipos por defecto se siembran con
// Pasos=1 para que nadie note el cambio hasta que decida usarlo.
type TipoOperacion struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// SedeID acota el tipo a una sede. Vacío = vale para toda la empresa, que es lo
	// normal: el proceso suele ser el mismo en todas.
	SedeID string `json:"sedeId" bson:"sedeid"`
	Codigo string `json:"codigo" bson:"codigo"` // "REC", "ENT"
	Nombre string `json:"nombre" bson:"nombre"` // "Recepción de compras"
	// Clase dice QUÉ operación configura. No es libre: cada clase tiene un sitio
	// concreto en el código que la consulta, y una clase inventada no la leería nadie.
	Clase string `json:"clase" bson:"clase"`
	// Pasos: 1 o 2. Con 2, la mercancía pasa por una ubicación intermedia y hace
	// falta una segunda acción de una persona para completarla. Es lo único que
	// cambia el número de movimientos que se emiten.
	Pasos int `json:"pasos" bson:"pasos"`
	// AlmacenID fija el almacén de la operación. Vacío = el principal de la sede,
	// que es el comportamiento de siempre.
	AlmacenID string `json:"almacenId" bson:"almacenid"`
	// UbicacionIntermediaID es el muelle (en una recepción) o la zona de preparación
	// (en una entrega). Solo se usa con Pasos=2; con un paso no significa nada.
	UbicacionIntermediaID string `json:"ubicacionIntermediaId" bson:"ubicacionintermediaid"`
	Activo                bool   `json:"activo" bson:"activo"`
	// PorDefecto marca cuál se aplica cuando nadie elige. Uno por clase y sede: con
	// dos, la operación dependería de cuál se leyera primero.
	PorDefecto bool   `json:"porDefecto" bson:"pordefecto"`
	Creado     string `json:"creado" bson:"creado"`
}

// Clases de operación. Son cerradas a propósito: cada una la consulta un sitio
// concreto del código, y una clase que nadie lee es configuración que no hace nada
// — que es peor que no poder configurarla.
const (
	// ClaseRecepcion configura la entrada de mercancía de una compra.
	ClaseRecepcion = "recepcion"
	// ClaseEntrega configura la salida hacia el cliente.
	ClaseEntrega = "entrega"
	// ClaseAjuste configura el conteo y la corrección de existencia.
	ClaseAjuste = "ajuste"
	// ClaseTransferencia configura el movimiento entre sedes o almacenes.
	ClaseTransferencia = "transferencia"
)

// ClaseOperacionValida acota la clase.
func ClaseOperacionValida(c string) bool {
	switch c {
	case ClaseRecepcion, ClaseEntrega, ClaseAjuste, ClaseTransferencia:
		return true
	}
	return false
}

// PasosValidos: uno o dos. Tres pasos existen en Odoo (recepción → control de
// calidad → almacenamiento) y se dejan fuera a propósito: el tercero solo aporta
// si hay un proceso de calidad que registrar, y eso es otro módulo. Cero es un
// error de datos que dejaría la operación sin emitir movimiento.
func PasosValidos(p int) bool { return p == 1 || p == 2 }

// NormalizarCodigoOperacion deja el código en mayúsculas y sin espacios.
func NormalizarCodigoOperacion(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// TipoOperacionRepo persiste tipos de operación, aislado por empresaID. Editable,
// sin borrado duro: desactivar es un Update con Activo=false.
type TipoOperacionRepo interface {
	List(empresaID string) []TipoOperacion
	ByID(empresaID, id string) (TipoOperacion, bool)
	Create(t TipoOperacion) TipoOperacion
	Update(t TipoOperacion) (TipoOperacion, bool)
}
