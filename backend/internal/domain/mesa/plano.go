package mesa

// Plano es la configuración de la GRILLA del salón de una sede: cuántas filas y
// columnas tiene el mapa (para dar la forma del local — cuadrado, alargado…) y qué
// celdas están BLOQUEADAS (una pared, la cocina, una columna: ahí no puede ir una
// mesa). Es configuración por sede, editable, no ledger.
type Plano struct {
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	Filas     int    `json:"filas" bson:"filas"`
	Columnas  int    `json:"columnas" bson:"columnas"`
	// Bloqueadas son las celdas donde NO puede colocarse una mesa.
	Bloqueadas []Celda `json:"bloqueadas" bson:"bloqueadas"`
	// Areas delimitan zonas del local (Terraza, Salón principal, Pórtico). NO
	// bloquean: son etiquetas de superficie, y las mesas viven DENTRO de ellas.
	Areas []Area `json:"areas" bson:"areas"`
	// Mostradores son los muebles de servicio (barra, caja, barra de postres).
	// Sí ocupan superficie: son muebles reales y ahí no entra una mesa.
	Mostradores []Mostrador `json:"mostradores" bson:"mostradores"`
	Actualizada string      `json:"actualizada" bson:"actualizada"` // RFC3339
}

/* ÁREAS Y MOSTRADORES.
 *
 * Un salón no es solo mesas: tiene una barra, una caja, una barra de postres, y
 * está dividido en zonas que el personal nombra en voz alta («la cuatro de la
 * terraza»). Dibujar eso en el plano no es decoración — es lo que hace que el
 * mapa se parezca al local y que alguien reconozca su mesa de un vistazo.
 *
 * LA DIFERENCIA QUE MANDA: un mostrador OCUPA (es un mueble; ahí no se puede
 * poner una mesa), un área NO (es una etiqueta de superficie; las mesas van
 * adentro). Confundirlas haría que dibujar la terraza expulsara sus mesas.
 *
 * El área además RESUELVE LA ZONA de las mesas que contiene. Hasta ahora la zona
 * se escribía a mano en cada mesa, así que «Terraza», «terraza» y «Terrraza»
 * convivían y cualquier agrupación por zona salía mal. Dibujada una vez, la zona
 * se deduce de dónde está la mesa, que es como funciona en el local.
 */

// Area es una zona del salón: un rectángulo con nombre.
type Area struct {
	ID      string `json:"id" bson:"id"`
	Nombre  string `json:"nombre" bson:"nombre"`
	Columna int    `json:"columna" bson:"columna"`
	Fila    int    `json:"fila" bson:"fila"`
	Ancho   int    `json:"ancho" bson:"ancho"`
	Alto    int    `json:"alto" bson:"alto"`
	// Color es el tinte del área en el plano (índice del catálogo, ver
	// ColoresArea). Es identidad visual, no dato de negocio.
	Color string `json:"color,omitempty" bson:"color,omitempty"`
}

// Mostrador es un mueble de servicio del salón.
type Mostrador struct {
	ID     string `json:"id" bson:"id"`
	Nombre string `json:"nombre" bson:"nombre"`
	// Tipo es del catálogo (TiposMostrador). Decide el icono y, más adelante,
	// a qué comandera se asocia.
	Tipo    string `json:"tipo" bson:"tipo"`
	Columna int    `json:"columna" bson:"columna"`
	Fila    int    `json:"fila" bson:"fila"`
	Ancho   int    `json:"ancho" bson:"ancho"`
	Alto    int    `json:"alto" bson:"alto"`
}

// Tipos de mostrador.
const (
	MostradorBarra    = "barra"
	MostradorCaja     = "caja"
	MostradorPostres  = "postres"
	MostradorEstacion = "estacion"
	MostradorCocina   = "cocina"
)

// TipoMostrador describe una entrada del catálogo, para que la interfaz no lleve
// la lista escrita a mano y se desincronice de la que valida.
type TipoMostrador struct {
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
}

// TiposMostrador es el catálogo de muebles de servicio.
func TiposMostrador() []TipoMostrador {
	return []TipoMostrador{
		{Codigo: MostradorBarra, Nombre: "Barra"},
		{Codigo: MostradorCaja, Nombre: "Caja"},
		{Codigo: MostradorPostres, Nombre: "Barra de postres"},
		{Codigo: MostradorEstacion, Nombre: "Estación de servicio"},
		{Codigo: MostradorCocina, Nombre: "Paso de cocina"},
	}
}

// TipoMostradorValido acota el tipo al catálogo.
func TipoMostradorValido(c string) bool {
	for _, t := range TiposMostrador() {
		if t.Codigo == c {
			return true
		}
	}
	return false
}

// cubre responde si un rectángulo (col,fil,ancho,alto) cubre la celda (c,r).
func cubre(col, fil, ancho, alto, c, r int) bool {
	return c >= col && c < col+ancho && r >= fil && r < fil+alto
}

// seSolapan responde si dos rectángulos comparten alguna celda.
func seSolapan(c1, f1, a1, al1, c2, f2, a2, al2 int) bool {
	return c1 < c2+a2 && c2 < c1+a1 && f1 < f2+al2 && f2 < f1+al1
}

// Cubre indica si el área cubre la celda.
func (a Area) Cubre(c, r int) bool { return cubre(a.Columna, a.Fila, a.Ancho, a.Alto, c, r) }

// Cubre indica si el mostrador cubre la celda. Es lo que impide poner una mesa
// encima de la barra.
func (m Mostrador) Cubre(c, r int) bool { return cubre(m.Columna, m.Fila, m.Ancho, m.Alto, c, r) }

// SeSolapaCon indica si dos áreas comparten superficie. Dos áreas encimadas
// harían ambigua la zona de las mesas de esa franja, así que no se permiten.
func (a Area) SeSolapaCon(o Area) bool {
	return seSolapan(a.Columna, a.Fila, a.Ancho, a.Alto, o.Columna, o.Fila, o.Ancho, o.Alto)
}

// SeSolapaCon indica si dos mostradores comparten superficie.
func (m Mostrador) SeSolapaCon(o Mostrador) bool {
	return seSolapan(m.Columna, m.Fila, m.Ancho, m.Alto, o.Columna, o.Fila, o.Ancho, o.Alto)
}

// PisaMesa indica si el mostrador se encima con una mesa. Se comprueba contra
// TODA la superficie de la mesa, no solo su esquina.
func (m Mostrador) PisaMesa(x Mesa) bool {
	dc, df := x.Dimension()
	return seSolapan(m.Columna, m.Fila, m.Ancho, m.Alto, x.Columna, x.Fila, dc, df)
}

// ZonaDe devuelve el nombre del área que contiene la celda, o "" si ninguna.
// Es lo que deduce la zona de una mesa por dónde está, en vez de que alguien la
// escriba distinto en cada mesa.
func (p Plano) ZonaDe(c, r int) string {
	for _, a := range p.Areas {
		if a.Cubre(c, r) {
			return a.Nombre
		}
	}
	return ""
}

// MostradorEn devuelve el mostrador que cubre la celda, si hay alguno.
func (p Plano) MostradorEn(c, r int) (Mostrador, bool) {
	for _, m := range p.Mostradores {
		if m.Cubre(c, r) {
			return m, true
		}
	}
	return Mostrador{}, false
}

// ColoresArea son los tintes disponibles para dibujar un área. Cinco alcanzan:
// un salón con más de cinco zonas nombradas no se lee en un plano de todos
// modos, y una paleta corta evita que cada sede invente su propio código.
func ColoresArea() []string { return []string{"violeta", "teal", "ambar", "rosa", "pizarra"} }

// Celda es una posición de la grilla (0-indexada).
type Celda struct {
	Columna int `json:"columna" bson:"columna"`
	Fila    int `json:"fila" bson:"fila"`
}

// Límites de la grilla del salón.
const (
	FilasMin        = 1
	FilasMax        = 20
	ColumnasMin     = 1
	ColumnasMax     = 20
	FilasDefault    = 6
	ColumnasDefault = 8
)

// PlanoRepository persiste el plano (grilla) de una sede. Una sola por sede:
// Upsert reemplaza. Get devuelve false si aún no se configuró.
type PlanoRepository interface {
	Get(empresaID, sedeID string) (Plano, bool)
	Upsert(p Plano) Plano
}
