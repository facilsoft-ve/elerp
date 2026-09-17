package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/mesa"
)

// maxCeldasMesa acota el lado de una mesa en cuadros.
const maxCeldasMesa = 6

// Errores de negocio de las mesas (módulo Restaurante).
var (
	// ErrMesasSolapadas: dos mesas comparten celdas. Una mesa de más capacidad
	// ocupa varias (ver mesa.Dimension), así que el choque puede no ser evidente
	// mirando solo las esquinas.
	ErrMesasSolapadas = errors.New("hay mesas encimadas en el plano")
	// ErrAreasSolapadas: dos zonas comparten superficie. La zona de las mesas de
	// esa franja quedaría ambigua, y la zona es lo que el personal usa para
	// nombrar las mesas en voz alta.
	ErrAreasSolapadas = errors.New("hay áreas encimadas en el plano")
	// ErrAreaSinNombre: un área sin nombre no sirve para nada — su único
	// propósito es nombrar una parte del salón.
	ErrAreaSinNombre = errors.New("el área necesita un nombre (Terraza, Salón principal…)")
	// ErrMostradoresSolapados / ErrMostradorPisaMesa: un mostrador es un mueble
	// real; ahí no cabe otra cosa.
	ErrMostradoresSolapados = errors.New("hay mostradores encimados en el plano")
	ErrMostradorPisaMesa    = errors.New("un mostrador no puede ocupar el lugar de una mesa")
	ErrMostradorSinNombre   = errors.New("el mostrador necesita un nombre (Barra, Caja…)")
	ErrMostradorTipo        = errors.New("el tipo de mostrador no está en el catálogo")
	// ErrAforoExcedeTamano: se pidió más gente de la que caben en los cuadros
	// que ocupa la mesa (4 por cuadro).
	ErrAforoExcedeTamano = errors.New("el aforo no cabe en el tamaño de la mesa")
	// ErrTamanoMesaInvalido acota el tamaño: una mesa más grande que esto no es
	// una mesa, es un error de tecleo que desarma el plano.
	ErrTamanoMesaInvalido = errors.New("el tamaño de la mesa es inválido")
	ErrMesasNoDisponible  = errors.New("el módulo de mesas no está disponible")
	ErrMesaNoExiste       = errors.New("la mesa no existe")
	ErrMesaSinNombre      = errors.New("la mesa necesita un nombre o número")
	ErrFormaInvalida      = errors.New("forma de mesa inválida (redonda, cuadrada o rectangular)")
)

// ConMesas cablea el maestro de mesas y el repo del plano del salón. Se configura
// aparte de New (como ConUnidades/ConPlantillas); sin él, el servicio funciona
// igual y las mesas quedan vacías.
func (s *Service) ConMesas(r mesa.Repository, planos mesa.PlanoRepository) *Service {
	s.mesas = r
	s.planos = planos
	return s
}

// PlanoSalon devuelve la grilla del salón de una sede (o una por defecto si aún no
// se configuró, para que el editor siempre tenga con qué dibujar).
func (s *Service) PlanoSalon(empresaID, sedeID string) mesa.Plano {
	if s.planos != nil {
		if p, ok := s.planos.Get(empresaID, sedeID); ok {
			if p.Filas <= 0 {
				p.Filas = mesa.FilasDefault
			}
			if p.Columnas <= 0 {
				p.Columnas = mesa.ColumnasDefault
			}
			if p.Bloqueadas == nil {
				p.Bloqueadas = []mesa.Celda{}
			}
			return p
		}
	}
	return mesa.Plano{EmpresaID: empresaID, SedeID: sedeID, Filas: mesa.FilasDefault, Columnas: mesa.ColumnasDefault, Bloqueadas: []mesa.Celda{}}
}

// GuardarPlanoSalon fija la grilla (filas/columnas) y las celdas bloqueadas del
// salón de una sede. Acota filas/columnas al rango válido y descarta celdas fuera
// de la grilla.
func (s *Service) GuardarPlanoSalon(empresaID, sedeID, actor, origen string, filas, columnas int, bloqueadas []mesa.Celda) (mesa.Plano, error) {
	return s.GuardarPlanoCompleto(empresaID, sedeID, actor, origen, PlanoEntrada{
		Filas: filas, Columnas: columnas, Bloqueadas: bloqueadas, conservar: true,
	})
}

// PlanoEntrada es el plano completo que deja el editor: la grilla, las celdas
// bloqueadas, las ÁREAS (zonas nombradas) y los MOSTRADORES (barra, caja, barra
// de postres).
type PlanoEntrada struct {
	Filas       int
	Columnas    int
	Bloqueadas  []mesa.Celda
	Areas       []mesa.Area
	Mostradores []mesa.Mostrador
	// conservar deja áreas y mostradores como están. Lo usa la ruta vieja, que
	// solo sabe de grilla: sin esto, un cliente anterior borraría la barra y las
	// zonas del local con solo cambiar el número de filas.
	conservar bool
}

// GuardarPlanoCompleto guarda la grilla con sus áreas y mostradores.
//
// Dos reglas que no se pueden relajar:
//   - Un MOSTRADOR no puede pisar una mesa ni a otro mostrador: es un mueble
//     real y ahí no cabe otra cosa.
//   - Dos ÁREAS no pueden encimarse: la zona de las mesas de esa franja quedaría
//     ambigua, y la zona es lo que el personal usa para nombrar las mesas.
//
// Un área SÍ se superpone a las mesas: para eso está, las contiene. Y al
// guardar, cada mesa toma la zona del área donde cayó — dibujarla una vez vale
// más que escribir «Terraza» en veinte fichas y que tres queden «terraza».
func (s *Service) GuardarPlanoCompleto(empresaID, sedeID, actor, origen string, in PlanoEntrada) (mesa.Plano, error) {
	if s.planos == nil {
		return mesa.Plano{}, ErrMesasNoDisponible
	}
	filas, columnas := in.Filas, in.Columnas
	if filas < mesa.FilasMin {
		filas = mesa.FilasMin
	}
	if filas > mesa.FilasMax {
		filas = mesa.FilasMax
	}
	if columnas < mesa.ColumnasMin {
		columnas = mesa.ColumnasMin
	}
	if columnas > mesa.ColumnasMax {
		columnas = mesa.ColumnasMax
	}
	limpias := make([]mesa.Celda, 0, len(in.Bloqueadas))
	vistas := map[[2]int]bool{}
	for _, c := range in.Bloqueadas {
		if c.Columna < 0 || c.Columna >= columnas || c.Fila < 0 || c.Fila >= filas {
			continue
		}
		k := [2]int{c.Columna, c.Fila}
		if vistas[k] {
			continue
		}
		vistas[k] = true
		limpias = append(limpias, c)
	}
	areas, mostradores := in.Areas, in.Mostradores
	if in.conservar {
		// La ruta vieja solo manda grilla: se conserva lo que ya había.
		if ant, ok := s.planos.Get(empresaID, sedeID); ok {
			areas, mostradores = ant.Areas, ant.Mostradores
		}
	} else {
		var err error
		if areas, err = saneaAreas(areas, columnas, filas); err != nil {
			return mesa.Plano{}, err
		}
		if mostradores, err = s.saneaMostradores(empresaID, sedeID, mostradores, columnas, filas); err != nil {
			return mesa.Plano{}, err
		}
	}

	p := mesa.Plano{
		EmpresaID: empresaID, SedeID: sedeID, Filas: filas, Columnas: columnas,
		Bloqueadas: limpias, Areas: areas, Mostradores: mostradores, Actualizada: ahora(),
	}
	out := s.planos.Upsert(p)
	// La zona de cada mesa se deduce del área donde está. Se hace DESPUÉS de
	// guardar el plano para que use las áreas nuevas, y solo cuando la mesa cae
	// dentro de alguna: fuera de toda área la zona escrita a mano se respeta.
	s.aplicarZonasDeAreas(empresaID, sedeID, out)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.plano.guardar", sedeID, ""))
	return out, nil
}

/* El id de un área o un mostrador lo manda el cliente (UUIDv7 del lado del
 * navegador, como el resto del sistema offline-first). Si viene vacío se deriva
 * de su esquina: dos áreas no pueden encimarse y dos mostradores tampoco, así
 * que la esquina los identifica sin ambigüedad y el id queda estable entre
 * guardados en vez de cambiar en cada uno. */

// saneaAreas valida y recorta las áreas al plano.
func saneaAreas(areas []mesa.Area, columnas, filas int) ([]mesa.Area, error) {
	out := make([]mesa.Area, 0, len(areas))
	for _, a := range areas {
		a.Nombre = strings.TrimSpace(a.Nombre)
		if a.Nombre == "" {
			return nil, ErrAreaSinNombre
		}
		if a.ID == "" {
			a.ID = fmt.Sprintf("area_%d_%d", a.Columna, a.Fila)
		}
		a.Columna, a.Fila = maxInt(0, a.Columna), maxInt(0, a.Fila)
		// Se RECORTA al plano en vez de rechazar: achicar la grilla no debería
		// impedir guardar, solo dejar el área dentro de lo que quedó.
		a.Ancho = clampInt(a.Ancho, 1, columnas-a.Columna)
		a.Alto = clampInt(a.Alto, 1, filas-a.Fila)
		if a.Columna >= columnas || a.Fila >= filas {
			continue // quedó fuera de la grilla: se descarta
		}
		for _, otra := range out {
			if a.SeSolapaCon(otra) {
				return nil, fmt.Errorf("%w: «%s» y «%s»", ErrAreasSolapadas, a.Nombre, otra.Nombre)
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// saneaMostradores valida los muebles de servicio contra el plano y las mesas.
func (s *Service) saneaMostradores(empresaID, sedeID string, ms []mesa.Mostrador, columnas, filas int) ([]mesa.Mostrador, error) {
	var mesas []mesa.Mesa
	if s.mesas != nil {
		for _, m := range s.mesas.List(empresaID, sedeID) {
			if m.Activa {
				mesas = append(mesas, m)
			}
		}
	}
	out := make([]mesa.Mostrador, 0, len(ms))
	for _, m := range ms {
		m.Nombre = strings.TrimSpace(m.Nombre)
		if m.Nombre == "" {
			return nil, ErrMostradorSinNombre
		}
		if !mesa.TipoMostradorValido(m.Tipo) {
			return nil, fmt.Errorf("%w: «%s»", ErrMostradorTipo, m.Nombre)
		}
		if m.ID == "" {
			m.ID = fmt.Sprintf("most_%d_%d", m.Columna, m.Fila)
		}
		m.Columna, m.Fila = maxInt(0, m.Columna), maxInt(0, m.Fila)
		m.Ancho = clampInt(m.Ancho, 1, columnas-m.Columna)
		m.Alto = clampInt(m.Alto, 1, filas-m.Fila)
		if m.Columna >= columnas || m.Fila >= filas {
			continue
		}
		for _, otro := range out {
			if m.SeSolapaCon(otro) {
				return nil, fmt.Errorf("%w: «%s» y «%s»", ErrMostradoresSolapados, m.Nombre, otro.Nombre)
			}
		}
		for _, x := range mesas {
			if m.PisaMesa(x) {
				return nil, fmt.Errorf("%w: «%s» y la mesa «%s»", ErrMostradorPisaMesa, m.Nombre, x.Nombre)
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// aplicarZonasDeAreas pone a cada mesa la zona del área que la contiene. Sin
// áreas dibujadas no toca nada: la zona escrita a mano sigue valiendo.
func (s *Service) aplicarZonasDeAreas(empresaID, sedeID string, p mesa.Plano) {
	if s.mesas == nil || len(p.Areas) == 0 {
		return
	}
	for _, m := range s.mesas.List(empresaID, sedeID) {
		zona := p.ZonaDe(m.Columna, m.Fila)
		if zona == "" || zona == m.Zona {
			continue
		}
		m.Zona = zona
		s.mesas.Update(m)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Mesas lista las mesas del salón de una sede, ordenadas por zona y luego por
// nombre (para agrupar el listado).
func (s *Service) Mesas(empresaID, sedeID string) []mesa.Mesa {
	if s.mesas == nil {
		return []mesa.Mesa{}
	}
	out := s.mesas.List(empresaID, sedeID)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Zona != out[j].Zona {
			return out[i].Zona < out[j].Zona
		}
		return out[i].Nombre < out[j].Nombre
	})
	return out
}

// dimsPorForma da el tamaño por defecto (unidades del lienzo) de una forma.
func dimsPorForma(forma string) (ancho, alto float64) {
	if forma == mesa.FormaRectangular {
		return 110, 70
	}
	return 70, 70
}

// saneaMesa normaliza y valida los campos comunes de una mesa. No toca ID/
// EmpresaID/SedeID/Creada.
func (s *Service) saneaMesa(m mesa.Mesa) (mesa.Mesa, error) {
	m.Nombre = strings.TrimSpace(m.Nombre)
	if m.Nombre == "" {
		return mesa.Mesa{}, ErrMesaSinNombre
	}
	m.Zona = strings.TrimSpace(m.Zona)
	m.Forma = mesa.NormalizarForma(m.Forma)
	if !mesa.FormaValida(m.Forma) {
		return mesa.Mesa{}, ErrFormaInvalida
	}
	if m.Capacidad < 0 {
		m.Capacidad = 0
	}
	// Tamaño en cuadros. Sin tamaño explícito se usa el mínimo que hace falta
	// para el aforo pedido: dar de alta «mesa de 8» no debería obligar a
	// dimensionarla a mano antes de poder guardarla.
	if m.AnchoCeldas <= 0 || m.AltoCeldas <= 0 {
		m.AnchoCeldas, m.AltoCeldas = mesa.DimensionSugerida(m.Capacidad)
	}
	if m.AnchoCeldas > maxCeldasMesa || m.AltoCeldas > maxCeldasMesa {
		return mesa.Mesa{}, ErrTamanoMesaInvalido
	}
	// EL TOPE: cada cuadro admite 4 personas. Para sentar a más hay que ampliar
	// la mesa. Se valida acá y no solo en la pantalla porque es la regla que
	// mantiene coherente el plano con el aforo declarado.
	if m.Capacidad > m.CapacidadMaxima() {
		return mesa.Mesa{}, fmt.Errorf("%w: una mesa de %d×%d cuadros admite hasta %d personas",
			ErrAforoExcedeTamano, m.AnchoCeldas, m.AltoCeldas, m.CapacidadMaxima())
	}
	if m.Ancho <= 0 || m.Alto <= 0 {
		m.Ancho, m.Alto = dimsPorForma(m.Forma)
	}
	if strings.TrimSpace(m.Estado) == "" {
		m.Estado = mesa.EstadoLibre
	}
	return m, nil
}

// CrearMesa da de alta una mesa en el salón de una sede. Arranca activa y libre.
func (s *Service) CrearMesa(empresaID, sedeID, actor, origen string, m mesa.Mesa) (mesa.Mesa, error) {
	if s.mesas == nil {
		return mesa.Mesa{}, ErrMesasNoDisponible
	}
	m, err := s.saneaMesa(m)
	if err != nil {
		return mesa.Mesa{}, err
	}
	m.ID = ""
	m.EmpresaID = empresaID
	m.SedeID = sedeID
	m.Activa = true
	m.Estado = mesa.EstadoLibre
	m.Creada = ahora()
	out := s.mesas.Create(m)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.mesa.crear", out.ID, out.Nombre))
	return out, nil
}

// ActualizarMesa edita una mesa existente (nombre, zona, capacidad, forma,
// posición y tamaño). Conserva fecha de creación y sede.
func (s *Service) ActualizarMesa(empresaID, id, actor, origen string, m mesa.Mesa, activa *bool) (mesa.Mesa, error) {
	if s.mesas == nil {
		return mesa.Mesa{}, ErrMesasNoDisponible
	}
	cur, ok := s.mesas.ByID(empresaID, id)
	if !ok {
		return mesa.Mesa{}, ErrMesaNoExiste
	}
	m, err := s.saneaMesa(m)
	if err != nil {
		return mesa.Mesa{}, err
	}
	m.ID = id
	m.EmpresaID = empresaID
	m.SedeID = cur.SedeID
	m.Creada = cur.Creada
	// El estado operativo lo gobierna la cuenta (fase de comandas): un guardado del
	// mapa no lo cambia; se conserva el actual.
	m.Estado = cur.Estado
	if activa != nil {
		m.Activa = *activa
	} else {
		m.Activa = cur.Activa
	}
	out, ok := s.mesas.Update(m)
	if !ok {
		return mesa.Mesa{}, ErrMesaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.mesa.actualizar", out.ID, out.Nombre))
	return out, nil
}

// EliminarMesa borra una mesa del salón.
func (s *Service) EliminarMesa(empresaID, id, actor, origen string) error {
	if s.mesas == nil {
		return ErrMesasNoDisponible
	}
	cur, ok := s.mesas.ByID(empresaID, id)
	if !ok {
		return ErrMesaNoExiste
	}
	if !s.mesas.Delete(empresaID, id) {
		return ErrMesaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.mesa.eliminar", id, cur.Nombre))
	return nil
}

// PosicionMesa es una reubicación de una mesa en la grilla (del guardado del
// editor). Lleva también el TAMAÑO porque en el plano se redimensiona
// arrastrando los bordes: mover y agrandar son el mismo gesto para quien dibuja
// el salón, y mandarlos por rutas distintas haría que un plano se guardara a
// medias si una de las dos llamadas falla.
//
// AnchoCeldas/AltoCeldas en CERO significan «no lo toques», para que un cliente
// viejo que solo manda posiciones no achique todas las mesas.
type PosicionMesa struct {
	ID          string `json:"id"`
	Columna     int    `json:"columna"`
	Fila        int    `json:"fila"`
	AnchoCeldas int    `json:"anchoCeldas,omitempty"`
	AltoCeldas  int    `json:"altoCeldas,omitempty"`
}

// GuardarMapa aplica en lote las posiciones (celda de grilla) de varias mesas (lo
// que deja el editor al guardar el plano). Ignora ids que no sean de la empresa. No
// cambia nombre/estado/actividad.
func (s *Service) GuardarMapa(empresaID, actor, origen string, pos []PosicionMesa) error {
	if s.mesas == nil {
		return ErrMesasNoDisponible
	}
	// Se arma el mapa COMPLETO antes de tocar nada: una mesa grande ocupa varias
	// celdas, así que dos mesas pueden encimarse aunque sus esquinas sean
	// distintas. Validar de a una dejaría pasar justo ese caso.
	nuevas := map[string]mesa.Mesa{}
	for _, p := range pos {
		m, ok := s.mesas.ByID(empresaID, p.ID)
		if !ok {
			continue
		}
		m.Columna, m.Fila = p.Columna, p.Fila
		if p.AnchoCeldas > 0 && p.AltoCeldas > 0 {
			if p.AnchoCeldas > maxCeldasMesa || p.AltoCeldas > maxCeldasMesa {
				return fmt.Errorf("%w: «%s»", ErrTamanoMesaInvalido, m.Nombre)
			}
			m.AnchoCeldas, m.AltoCeldas = p.AnchoCeldas, p.AltoCeldas
			// Al achicar, el aforo se recorta al nuevo tope. Rechazar el guardado
			// sería peor: el plano ya se dibujó y quien lo hizo tendría que adivinar
			// cuál de veinte mesas quedó con un número que ya no entra.
			if max := m.CapacidadMaxima(); m.Capacidad > max {
				m.Capacidad = max
			}
		}
		nuevas[m.ID] = m
	}
	// El plano final es: las mesas movidas, más las que no se tocaron.
	var sedeID string
	for _, m := range nuevas {
		sedeID = m.SedeID
		break
	}
	final := []mesa.Mesa{}
	for _, m := range s.mesas.List(empresaID, sedeID) {
		if n, movida := nuevas[m.ID]; movida {
			final = append(final, n)
		} else if m.Activa {
			final = append(final, m)
		}
	}
	for i := range final {
		for j := i + 1; j < len(final); j++ {
			if final[i].SeSolapaCon(final[j]) {
				return fmt.Errorf("%w: «%s» y «%s»", ErrMesasSolapadas, final[i].Nombre, final[j].Nombre)
			}
		}
	}
	// Y contra los MUEBLES: la barra y la caja ocupan piso. Sin esto se podría
	// arrastrar una mesa encima de la barra y el plano diría una cosa que el
	// local no permite.
	if s.planos != nil && sedeID != "" {
		if p, ok := s.planos.Get(empresaID, sedeID); ok {
			for _, most := range p.Mostradores {
				for _, m := range final {
					if most.PisaMesa(m) {
						return fmt.Errorf("%w: «%s» y «%s»", ErrMostradorPisaMesa, most.Nombre, m.Nombre)
					}
				}
			}
		}
	}
	for _, m := range nuevas {
		s.mesas.Update(m)
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.mapa.guardar", "", ""))
	return nil
}
