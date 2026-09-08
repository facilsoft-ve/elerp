// Package promocion es el maestro de PROMOCIONES de la empresa: una biblioteca
// de anuncios (de IMAGEN o de TEXTO, con vigencia) que alimentan el espacio
// promocional (carrusel) de la pantalla del cliente del POS.
//
// A diferencia del ledger de inventario o de los documentos fiscales, es un
// maestro EDITABLE (Update): no es append-only. Una promoción se muestra en el
// carrusel cuando está ACTIVA y dentro de su periodo de vigencia; junto a ella
// conviven los slides manuales que la empresa carga en Ajustes › Pantalla del
// cliente (empresa.Publicidad). Ambos comparten la misma forma de slide
// (tipo/texto/subtexto/imagen), así que el carrusel los unifica en una sola lista.
package promocion

import "strings"

// Tipos de promoción (coinciden con los tipos de slide de la pantalla del cliente).
const (
	// TipoImagen: la promoción es una imagen (ruta servida o URL), a 16:9.
	TipoImagen = "imagen"
	// TipoTexto: un mensaje de marca (título grande + subtexto opcional), sin imagen.
	TipoTexto = "texto"
)

// TipoValido indica si el tipo de promoción es uno de los admitidos.
func TipoValido(t string) bool { return t == TipoImagen || t == TipoTexto }

// NormalizarTipo devuelve un tipo válido; por defecto TipoTexto.
func NormalizarTipo(t string) string {
	t = strings.TrimSpace(strings.ToLower(t))
	if TipoValido(t) {
		return t
	}
	return TipoTexto
}

// Promocion es un anuncio de la biblioteca de promociones de la empresa.
type Promocion struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	// Nombre interno para gestionarla en la lista (no se muestra al cliente).
	Nombre string `json:"nombre" bson:"nombre"`
	Tipo   string `json:"tipo" bson:"tipo"` // TipoImagen | TipoTexto
	// Titulo es el texto grande del slide (para el tipo TEXTO). Se mapea al
	// `texto` del slide de la pantalla del cliente.
	Titulo string `json:"titulo" bson:"titulo"`
	// Subtexto acompaña al título en un slide de texto (opcional).
	Subtexto string `json:"subtexto" bson:"subtexto"`
	// Imagen es la ruta relativa servida (/api/archivos/…) o una URL/data URI
	// para el tipo IMAGEN.
	Imagen string `json:"imagen" bson:"imagen"`
	Desde  string `json:"desde" bson:"desde"` // YYYY-MM-DD, vacío = sin límite inferior
	Hasta  string `json:"hasta" bson:"hasta"` // YYYY-MM-DD, vacío = sin límite superior
	Activa bool   `json:"activa" bson:"activa"`
	// Orden controla la posición en el carrusel (menor primero); empate = por nombre.
	Orden int `json:"orden" bson:"orden"`
}

// Repository persiste promociones, aislado por empresaID.
type Repository interface {
	List(empresaID string) []Promocion
	ByID(empresaID, id string) (Promocion, bool)
	Create(p Promocion) Promocion
	// Update reemplaza una promoción existente del tenant (maestro editable, no
	// ledger). Devuelve false si el id no pertenece a la empresa.
	Update(p Promocion) (Promocion, bool)
}
