// Package unidadmedida es el maestro de unidades de medida de la empresa.
//
// Una unidad de medida es un SÍMBOLO ("kg", "L", "unidad") con su nombre legible
// ("Kilogramo") y una categoría (conteo, peso, volumen o longitud). El catálogo
// de producto elige su UnidadBase de este maestro; antes UnidadBase era texto
// libre. El símbolo es lo que se guarda en el producto, así que este maestro solo
// ALIMENTA el select del front: no se rechaza un producto cuyo símbolo no esté en
// el catálogo (para no romper datos ya cargados).
//
// Como el maestro de cupones o el de listas de precio, es EDITABLE (Update): no es
// un ledger append-only. No hay borrado duro; una unidad se DESACTIVA (Activa=false)
// para conservar el histórico y las referencias de productos que ya la usan.
package unidadmedida

import "strings"

// Categorías admitidas de una unidad de medida.
const (
	CategoriaConteo   = "conteo"   // unidad, docena, par, caja…
	CategoriaPeso     = "peso"     // kg, g…
	CategoriaVolumen  = "volumen"  // L, mL…
	CategoriaLongitud = "longitud" // m, cm…
)

// CategoriaValida indica si la categoría es una de las admitidas.
func CategoriaValida(c string) bool {
	switch c {
	case CategoriaConteo, CategoriaPeso, CategoriaVolumen, CategoriaLongitud:
		return true
	}
	return false
}

// NormalizarSimbolo recorta espacios a los lados. NO cambia mayúsculas/minúsculas:
// los símbolos del SI son sensibles a la caja ("kg" ≠ "Kg", "mL" ≠ "ml"), así que
// se conservan tal cual los escribe la empresa.
func NormalizarSimbolo(s string) string { return strings.TrimSpace(s) }

// NormalizarCategoria deja la categoría en minúsculas y sin espacios (así "Peso"
// llega como "peso" y pasa CategoriaValida).
func NormalizarCategoria(c string) string { return strings.ToLower(strings.TrimSpace(c)) }

// UnidadMedida es una unidad del maestro de la empresa.
type UnidadMedida struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Simbolo   string `json:"simbolo" bson:"simbolo"`     // "kg", "L", "unidad" — único por empresa
	Nombre    string `json:"nombre" bson:"nombre"`       // "Kilogramo"
	Categoria string `json:"categoria" bson:"categoria"` // conteo | peso | volumen | longitud
	Activa    bool   `json:"activa" bson:"activa"`       // desactivar sin borrar
	// Factor es CUÁNTAS UNIDADES DE REFERENCIA de su categoría vale una de ésta.
	// El kilogramo es la referencia del peso (1); el gramo vale 0,001; un saco de
	// 50 kg vale 50. Convertir entre dos unidades es cantidad × FactorOrigen ÷
	// FactorDestino, y por eso basta un número por unidad en vez de una tabla.
	//
	// CERO SIGNIFICA «SIN DECLARAR», y es el caso de todo lo que existía antes: esa
	// unidad no se convierte, y quien la use trabaja como hasta ahora. No se pone 1
	// por defecto a propósito — un factor inventado convertiría en silencio y con el
	// número equivocado, que es mucho peor que no convertir.
	Factor float64 `json:"factor,omitempty" bson:"factor,omitempty"`
	Creada string  `json:"creada" bson:"creada"` // RFC3339
}

// Repository persiste unidades de medida, aislado por empresaID. Sin borrado duro:
// desactivar es un Update con Activa=false.
type Repository interface {
	List(empresaID string) []UnidadMedida
	ByID(empresaID, id string) (UnidadMedida, bool)
	Create(u UnidadMedida) UnidadMedida
	// Update reemplaza una unidad existente del tenant (maestro editable, no ledger).
	// Devuelve false si el id no pertenece a la empresa.
	Update(u UnidadMedida) (UnidadMedida, bool)
}

// PorDefecto devuelve el juego de unidades comunes en Venezuela que se siembra
// para una empresa nueva: conteo (unidad, docena, par, caja), peso (kg, g),
// volumen (L, mL) y longitud (m, cm). Todas quedan activas; el ID y la fecha los
// asigna el adaptador al crearlas.
// Los factores del juego por defecto. La referencia de cada categoría vale 1.
//
// LA CAJA Y LA DOCENA NO SON IGUALES, y ahí está la diferencia que importa: una
// docena SIEMPRE son 12, así que su factor es universal. Una «caja» son las que
// quepan, y cambia por producto — por eso se queda SIN factor y se resuelve con las
// presentaciones del producto, que es donde ese dato pertenece.
func PorDefecto(empresaID string) []UnidadMedida {
	defs := []struct {
		simbolo, nombre, categoria string
		factor                     float64
	}{
		{"unidad", "Unidad", CategoriaConteo, 1},
		{"docena", "Docena", CategoriaConteo, 12},
		{"par", "Par", CategoriaConteo, 2},
		{"caja", "Caja", CategoriaConteo, 0}, // depende del producto: sin factor
		{"kg", "Kilogramo", CategoriaPeso, 1},
		{"g", "Gramo", CategoriaPeso, 0.001},
		{"L", "Litro", CategoriaVolumen, 1},
		{"mL", "Mililitro", CategoriaVolumen, 0.001},
		{"m", "Metro", CategoriaLongitud, 1},
		{"cm", "Centímetro", CategoriaLongitud, 0.01},
	}
	out := make([]UnidadMedida, 0, len(defs))
	for _, d := range defs {
		out = append(out, UnidadMedida{
			EmpresaID: empresaID, Simbolo: d.simbolo, Nombre: d.nombre,
			Categoria: d.categoria, Factor: d.factor, Activa: true,
		})
	}
	return out
}
