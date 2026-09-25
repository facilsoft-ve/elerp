package application

import (
	"errors"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/almacen"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// UBICACIONES DENTRO DEL ALMACÉN. Ver domain/almacen/ubicacion.go para el porqué.
//
// La regla que sostiene todo: LA SUMA POR UBICACIÓN ES LA EXISTENCIA DEL ALMACÉN.
// En cuanto se separan, la ubicación miente sin que nada falle — alguien va al
// estante que dice el sistema y no encuentra la mercancía, y a partir de ahí deja
// de mirar el sistema. Por eso toda escritura pasa por ubicacionParaEscritura y
// nadie elige por su cuenta, igual que con el almacén y con el lote.

var (
	// ErrUbicacionNoExiste: la ubicación no está o no es de este almacén.
	ErrUbicacionNoExiste = errors.New("la ubicación no existe en ese almacén")
	// ErrUbicacionSinCodigo: el código es lo que se lee en el anaquel.
	ErrUbicacionSinCodigo = errors.New("la ubicación necesita un código")
	// ErrUbicacionCodigoEnUso: dos ubicaciones con el mismo código en un almacén
	// hacen que quien la busca no sepa a cuál ir.
	ErrUbicacionCodigoEnUso = errors.New("ya hay una ubicación con ese código en este almacén")
	// ErrUbicacionTipoInvalido: tipo fuera de los que los procesos saben usar.
	ErrUbicacionTipoInvalido = errors.New("tipo de ubicación inválido (almacenamiento, muelle o preparación)")
	// ErrUbicacionSinAlmacen: una ubicación siempre cuelga de un almacén.
	ErrUbicacionSinAlmacen = errors.New("la ubicación necesita un almacén")
)

// ConUbicaciones cablea el maestro de ubicaciones. Sin él, todo se comporta como
// antes: los movimientos se anexan sin ubicación y las proyecciones la ignoran.
func (s *Service) ConUbicaciones(r almacen.UbicacionRepo) *Service {
	s.ubicaciones = r
	return s
}

// UbicacionesDe lista las ubicaciones de un almacén, ordenadas por código.
func (s *Service) UbicacionesDe(empresaID, almacenID string) []almacen.Ubicacion {
	if s.ubicaciones == nil {
		return []almacen.Ubicacion{}
	}
	out := []almacen.Ubicacion{}
	for _, u := range s.ubicaciones.List(empresaID) {
		if u.AlmacenID == almacenID {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Codigo < out[j].Codigo })
	return out
}

// CrearUbicacion da de alta una ubicación en un almacén.
func (s *Service) CrearUbicacion(empresaID, actor, origen string, u almacen.Ubicacion) (almacen.Ubicacion, error) {
	if s.ubicaciones == nil {
		return almacen.Ubicacion{}, errors.New("las ubicaciones no están disponibles")
	}
	if strings.TrimSpace(u.AlmacenID) == "" {
		return almacen.Ubicacion{}, ErrUbicacionSinAlmacen
	}
	alm, ok := s.almacenes.ByID(empresaID, u.AlmacenID)
	if !ok {
		return almacen.Ubicacion{}, ErrAlmacenNoExiste
	}
	u.Codigo = almacen.NormalizarCodigo(u.Codigo)
	if u.Codigo == "" {
		return almacen.Ubicacion{}, ErrUbicacionSinCodigo
	}
	u.Tipo = strings.ToLower(strings.TrimSpace(u.Tipo))
	if !almacen.TipoUbicacionValido(u.Tipo) {
		return almacen.Ubicacion{}, ErrUbicacionTipoInvalido
	}
	for _, otra := range s.UbicacionesDe(empresaID, alm.ID) {
		if otra.Activa && otra.Codigo == u.Codigo {
			return almacen.Ubicacion{}, ErrUbicacionCodigoEnUso
		}
	}
	u.EmpresaID = empresaID
	u.Nombre = strings.TrimSpace(u.Nombre)
	if u.Nombre == "" {
		u.Nombre = u.Codigo
	}
	u.Activa = true
	u.Creada = ahora()
	out := s.ubicaciones.Create(u)
	s.audit.Append(evento(empresaID, actor, origen, "inventario.ubicacion.crear", out.Codigo, alm.Nombre))
	return out, nil
}

// ActualizarUbicacion edita el nombre, el tipo o el estado. El CÓDIGO y el ALMACÉN
// no se tocan: los movimientos ya anexados la referencian por id, y cambiarle el
// almacén movería stock sin emitir un movimiento — el ledger dejaría de explicar
// dónde está la mercancía.
func (s *Service) ActualizarUbicacion(empresaID, id, actor, origen string, cambios almacen.Ubicacion) (almacen.Ubicacion, error) {
	if s.ubicaciones == nil {
		return almacen.Ubicacion{}, errors.New("las ubicaciones no están disponibles")
	}
	u, ok := s.ubicaciones.ByID(empresaID, id)
	if !ok {
		return almacen.Ubicacion{}, ErrUbicacionNoExiste
	}
	if n := strings.TrimSpace(cambios.Nombre); n != "" {
		u.Nombre = n
	}
	if t := strings.ToLower(strings.TrimSpace(cambios.Tipo)); t != "" {
		if !almacen.TipoUbicacionValido(t) {
			return almacen.Ubicacion{}, ErrUbicacionTipoInvalido
		}
		u.Tipo = t
	}
	u.Activa = cambios.Activa
	out, ok := s.ubicaciones.Update(u)
	if !ok {
		return almacen.Ubicacion{}, ErrUbicacionNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "inventario.ubicacion.editar", out.Codigo, out.Nombre))
	return out, nil
}

// ubicacionParaEscritura resuelve a qué ubicación se anexa un movimiento.
//
// ES EL ÚNICO CAMINO. Espeja almacenParaEscritura y por el mismo motivo: si cada
// sitio que escribe eligiera por su cuenta, el primero que se olvidara dejaría
// movimientos sin ubicar y la suma por ubicación se apartaría de la existencia del
// almacén — sin que nada fallara.
//
// Devuelve VACÍO cuando no hay ubicaciones configuradas, y eso es correcto: un
// almacén sin dividir es un almacén de una sola zona. La existencia por ubicación
// lo muestra como «sin ubicar», que es la verdad.
func (s *Service) ubicacionParaEscritura(empresaID, almacenID, deseada string) string {
	if s.ubicaciones == nil || almacenID == "" {
		return ""
	}
	if deseada != "" {
		if u, ok := s.ubicaciones.ByID(empresaID, deseada); ok && u.AlmacenID == almacenID && u.Activa {
			return u.ID
		}
		// Una ubicación pedida que no vale NO cae en silencio a otra: el llamante
		// creería que ubicó donde dijo. Cae a vacío, que se ve como «sin ubicar».
		return ""
	}
	return ""
}

// SaldoUbicacion es la existencia de un producto en una ubicación concreta.
type SaldoUbicacion struct {
	AlmacenID     string  `json:"almacenId"`
	AlmacenNombre string  `json:"almacenNombre"`
	UbicacionID   string  `json:"ubicacionId"`
	Codigo        string  `json:"codigo"`
	Nombre        string  `json:"nombre"`
	Cantidad      float64 `json:"cantidad"`
	// SKU y NombreProducto solo los llena quien lista VARIOS productos (lo pendiente
	// de ubicar). En el desglose de un producto concreto sobran: ya se sabe cuál es.
	SKU            string `json:"sku,omitempty"`
	NombreProducto string `json:"nombreProducto,omitempty"`
}

// ExistenciaPorUbicacion proyecta dónde está un producto dentro de una sede.
//
// La suma de sus cantidades es la existencia de la sede, siempre. Los movimientos
// sin ubicación se agrupan bajo el código «SIN UBICAR» en vez de omitirse: si se
// omitieran, la suma no cuadraría y nadie sabría por qué.
func (s *Service) ExistenciaPorUbicacion(empresaID, sedeID, sku string) []SaldoUbicacion {
	p, ok := s.productos.BySKU(empresaID, sku)
	if !ok {
		return []SaldoUbicacion{}
	}
	movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID})

	type clave struct{ Almacen, Ubicacion string }
	saldos := map[clave]float64{}
	for _, m := range movs {
		// La revaluación no mueve unidades: no pertenece a ninguna ubicación.
		if m.Tipo == inventario.MovRevaluacion {
			continue
		}
		saldos[clave{m.AlmacenID, m.UbicacionID}] += m.Cantidad
	}

	out := []SaldoUbicacion{}
	for k, cant := range saldos {
		if cant > -0.0001 && cant < 0.0001 {
			continue
		}
		fila := SaldoUbicacion{AlmacenID: k.Almacen, UbicacionID: k.Ubicacion, Cantidad: round2(cant)}
		if a, ok := s.almacenes.ByID(empresaID, k.Almacen); ok {
			fila.AlmacenNombre = a.Nombre
		}
		switch {
		case k.Ubicacion == "":
			fila.Codigo, fila.Nombre = "SIN UBICAR", "El almacén, sin más detalle"
		case s.ubicaciones == nil:
			/* SIN EL MAESTRO CABLEADO, LA UBICACIÓN NO SE MUESTRA: se lee como el
			 * almacén a secas.
			 *
			 * El movimiento puede traer una ubicación aunque esta instancia no tenga
			 * el maestro —los datos sembrados la traen—, y el código quedaba VACÍO:
			 * una fila sin nombre en la columna, que no es «sin ubicar» ni un sitio.
			 * Decir «SIN UBICAR» es lo honesto cuando el sistema no puede saber qué
			 * sitio es. */
			fila.Codigo, fila.Nombre = "SIN UBICAR", "El almacén, sin más detalle"
		default:
			if u, ok := s.ubicaciones.ByID(empresaID, k.Ubicacion); ok {
				fila.Codigo, fila.Nombre = u.Codigo, u.Nombre
			} else {
				// Una ubicación borrada del maestro no puede borrar el stock que la
				// referencia: se muestra con su id para que alguien pueda recolocarlo.
				fila.Codigo, fila.Nombre = k.Ubicacion, "Ubicación desconocida"
			}
		}
		out = append(out, fila)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AlmacenNombre != out[j].AlmacenNombre {
			return out[i].AlmacenNombre < out[j].AlmacenNombre
		}
		return out[i].Codigo < out[j].Codigo
	})
	return out
}
