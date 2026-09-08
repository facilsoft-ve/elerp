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
	Creada    string `json:"creada" bson:"creada"`       // RFC3339
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
func PorDefecto(empresaID string) []UnidadMedida {
	defs := []struct{ simbolo, nombre, categoria string }{
		{"unidad", "Unidad", CategoriaConteo},
		{"docena", "Docena", CategoriaConteo},
		{"par", "Par", CategoriaConteo},
		{"caja", "Caja", CategoriaConteo},
		{"kg", "Kilogramo", CategoriaPeso},
		{"g", "Gramo", CategoriaPeso},
		{"L", "Litro", CategoriaVolumen},
		{"mL", "Mililitro", CategoriaVolumen},
		{"m", "Metro", CategoriaLongitud},
		{"cm", "Centímetro", CategoriaLongitud},
	}
	out := make([]UnidadMedida, 0, len(defs))
	for _, d := range defs {
		out = append(out, UnidadMedida{
			EmpresaID: empresaID, Simbolo: d.simbolo, Nombre: d.nombre,
			Categoria: d.categoria, Activa: true,
		})
	}
	return out
}
