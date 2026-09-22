package mesa

import "strings"

/* PISOS DEL LOCAL.
 *
 * Un restaurante de dos plantas no es un plano más grande: es DOS planos. La
 * terraza del segundo piso no queda «a la derecha» del salón de abajo, y
 * dibujarlos en una sola grilla obliga a inventar una separación que no existe
 * y a leer el mapa como no se camina el local.
 *
 * Cada piso tiene SU forma —una planta alta suele ser más chica que la baja—,
 * sus áreas, sus muebles y sus mesas. Lo único que comparten es la sede.
 *
 * RETROCOMPATIBILIDAD, con el mismo patrón que Area.Celdas: un plano guardado
 * antes de esto no tiene pisos, y sus campos de nivel superior SON la planta
 * baja. `PisosEfectivos` devuelve eso sintetizado, así ningún salón ya dibujado
 * necesita migración ni deja de abrir.
 */

// Piso es una planta del local con su propia grilla y su contenido.
type Piso struct {
	ID     string `json:"id" bson:"id"`
	Nombre string `json:"nombre" bson:"nombre"`
	// Orden es cómo se apilan de abajo hacia arriba. Se usa para ordenar las
	// pestañas: el personal piensa «planta baja, primero, segundo», no por id.
	Orden    int `json:"orden" bson:"orden"`
	Filas    int `json:"filas" bson:"filas"`
	Columnas int `json:"columnas" bson:"columnas"`

	Bloqueadas  []Celda     `json:"bloqueadas" bson:"bloqueadas"`
	Areas       []Area      `json:"areas" bson:"areas"`
	Mostradores []Mostrador `json:"mostradores" bson:"mostradores"`
}

// PisoPrincipal es el id de la planta baja: el piso al que pertenece todo lo que
// se dibujó antes de que existieran los pisos, y donde nace un salón nuevo.
const PisoPrincipal = "piso_1"

// NombrePisoPrincipal es como se llama por defecto. «Planta baja» y no «Piso 1»
// porque es el que existe siempre, incluso en un local de un solo nivel donde
// numerarlo sugeriría que hay otro.
const NombrePisoPrincipal = "Planta baja"

// PisosMax acota cuántas plantas admite un local. Diez es más de lo que un
// restaurante tiene y suficiente para que el límite nunca moleste.
const PisosMax = 10

/* PisosEfectivos son los pisos del plano.
 *
 * Con la lista vacía —todo plano dibujado antes de los pisos— sintetiza la
 * planta baja con lo que hay en el nivel superior. Es la misma verdad leída de
 * dos formas, no dos verdades: por eso el resto del código llama siempre a esto
 * y nunca a `p.Areas` directamente.
 */
func (p Plano) PisosEfectivos() []Piso {
	if len(p.Pisos) > 0 {
		return p.Pisos
	}
	return []Piso{{
		ID: PisoPrincipal, Nombre: NombrePisoPrincipal, Orden: 0,
		Filas: p.Filas, Columnas: p.Columnas,
		Bloqueadas: p.Bloqueadas, Areas: p.Areas, Mostradores: p.Mostradores,
	}}
}

// PisoDe devuelve el piso por su id. Un id vacío o desconocido cae en la planta
// baja: una mesa sin piso es de antes de los pisos, y una mesa cuyo piso alguien
// borró tiene que seguir apareciendo en algún lado en vez de desaparecer del
// mapa sin aviso.
func (p Plano) PisoDe(id string) Piso {
	pisos := p.PisosEfectivos()
	for _, x := range pisos {
		if x.ID == id {
			return x
		}
	}
	return pisos[0]
}

// TienePiso indica si el id corresponde a un piso existente.
func (p Plano) TienePiso(id string) bool {
	for _, x := range p.PisosEfectivos() {
		if x.ID == id {
			return true
		}
	}
	return false
}

// PisoDeMesa resuelve a qué piso pertenece una mesa, resolviendo el vacío.
func (p Plano) PisoDeMesa(m Mesa) Piso { return p.PisoDe(m.PisoID) }

// NormalizarPiso acota el nombre y la grilla de un piso.
func (pi *Piso) Normalizar() {
	pi.Nombre = strings.TrimSpace(pi.Nombre)
	if pi.Nombre == "" {
		pi.Nombre = NombrePisoPrincipal
	}
	if pi.Filas < FilasMin {
		pi.Filas = FilasDefault
	}
	if pi.Filas > FilasMax {
		pi.Filas = FilasMax
	}
	if pi.Columnas < ColumnasMin {
		pi.Columnas = ColumnasDefault
	}
	if pi.Columnas > ColumnasMax {
		pi.Columnas = ColumnasMax
	}
}

// ZonaDe devuelve el nombre del área del piso que contiene la celda.
func (pi Piso) ZonaDe(c, r int) string {
	for _, a := range pi.Areas {
		if a.Cubre(c, r) {
			return a.Nombre
		}
	}
	return ""
}

// MostradorEn devuelve el mueble del piso que cubre la celda, si hay alguno.
func (pi Piso) MostradorEn(c, r int) (Mostrador, bool) {
	for _, m := range pi.Mostradores {
		if m.Cubre(c, r) {
			return m, true
		}
	}
	return Mostrador{}, false
}

// EsDelPiso indica si la mesa vive en este piso. Resuelve el id vacío contra la
// planta baja, que es donde está todo lo dibujado antes de los pisos.
func (pi Piso) EsDelPiso(m Mesa) bool {
	if m.PisoID == "" {
		return pi.ID == PisoPrincipal || pi.Orden == 0
	}
	return m.PisoID == pi.ID
}

// MesasDe filtra las mesas que están en este piso.
func (pi Piso) MesasDe(ms []Mesa) []Mesa {
	out := make([]Mesa, 0, len(ms))
	for _, m := range ms {
		if pi.EsDelPiso(m) {
			out = append(out, m)
		}
	}
	return out
}
