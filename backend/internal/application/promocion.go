package application

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/promocion"
)

var (
	ErrPromocionNoDisponible = errors.New("el maestro de promociones no está disponible")
	ErrPromocionNoExiste     = errors.New("promoción no existe")
	ErrTipoPromocionInvalido = errors.New("tipo de promoción inválido (imagen o texto)")
)

// ConPromociones cablea el maestro de promociones. Se configura aparte de New
// (como ConCupones/ConListasPrecio) para no romper las firmas de los
// constructores ya cableados en cmd/api; sin él, el servicio funciona igual y
// las promociones quedan vacías.
func (s *Service) ConPromociones(r promocion.Repository) *Service {
	s.promociones = r
	return s
}

// EntradaPromocion son los datos para crear/editar una promoción.
type EntradaPromocion struct {
	Nombre   string
	Tipo     string
	Titulo   string
	Subtexto string
	Imagen   string
	Desde    string
	Hasta    string
	Activa   bool
	Orden    int
}

// Promociones lista las promociones de la empresa, ordenadas por Orden y nombre.
func (s *Service) Promociones(empresaID string) []promocion.Promocion {
	if s.promociones == nil {
		return []promocion.Promocion{}
	}
	out := s.promociones.List(empresaID)
	ordenarPromociones(out)
	return out
}

// ordenarPromociones deja la lista por Orden ascendente y, a igual orden, por
// nombre — el mismo orden en que rotarán en el carrusel de la pantalla del cliente.
func ordenarPromociones(ps []promocion.Promocion) {
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].Orden != ps[j].Orden {
			return ps[i].Orden < ps[j].Orden
		}
		return strings.ToLower(ps[i].Nombre) < strings.ToLower(ps[j].Nombre)
	})
}

// validarEntradaPromocion arma una Promocion normalizada desde la entrada y valida
// las reglas de negocio: nombre requerido, tipo válido y el contenido propio del
// tipo (una imagen necesita imagen; un texto, título).
func (s *Service) validarEntradaPromocion(id string, in EntradaPromocion) (promocion.Promocion, error) {
	nombre := strings.TrimSpace(in.Nombre)
	if nombre == "" {
		return promocion.Promocion{}, errors.New("nombre requerido")
	}
	if !promocion.TipoValido(in.Tipo) {
		return promocion.Promocion{}, ErrTipoPromocionInvalido
	}
	titulo := strings.TrimSpace(in.Titulo)
	subtexto := strings.TrimSpace(in.Subtexto)
	imagen := strings.TrimSpace(in.Imagen)
	if in.Tipo == promocion.TipoImagen && imagen == "" {
		return promocion.Promocion{}, errors.New("una promoción de imagen necesita una imagen")
	}
	if in.Tipo == promocion.TipoTexto && titulo == "" {
		return promocion.Promocion{}, errors.New("una promoción de texto necesita un título")
	}
	desde := normalizarFechaCupon(in.Desde)
	hasta := normalizarFechaCupon(in.Hasta)
	if desde != "" && hasta != "" && hasta < desde {
		return promocion.Promocion{}, errors.New("la fecha «hasta» no puede ser anterior a «desde»")
	}
	return promocion.Promocion{
		ID: id, Nombre: nombre, Tipo: in.Tipo,
		Titulo: titulo, Subtexto: subtexto, Imagen: imagen,
		Desde: desde, Hasta: hasta, Activa: in.Activa, Orden: in.Orden,
	}, nil
}

// CrearPromocion da de alta una promoción.
func (s *Service) CrearPromocion(empresaID, actor, origen string, in EntradaPromocion) (promocion.Promocion, error) {
	if s.promociones == nil {
		return promocion.Promocion{}, ErrPromocionNoDisponible
	}
	p, err := s.validarEntradaPromocion("", in)
	if err != nil {
		return promocion.Promocion{}, err
	}
	p.EmpresaID = empresaID
	out := s.promociones.Create(p)
	s.audit.Append(evento(empresaID, actor, origen, "ventas.promocion.crear", out.ID, out.Nombre))
	return out, nil
}

// ActualizarPromocion edita una promoción existente del tenant (maestro editable,
// no ledger).
func (s *Service) ActualizarPromocion(empresaID, id, actor, origen string, in EntradaPromocion) (promocion.Promocion, error) {
	if s.promociones == nil {
		return promocion.Promocion{}, ErrPromocionNoDisponible
	}
	if _, ok := s.promociones.ByID(empresaID, id); !ok {
		return promocion.Promocion{}, ErrPromocionNoExiste
	}
	p, err := s.validarEntradaPromocion(id, in)
	if err != nil {
		return promocion.Promocion{}, err
	}
	p.EmpresaID = empresaID
	out, ok := s.promociones.Update(p)
	if !ok {
		return promocion.Promocion{}, ErrPromocionNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "ventas.promocion.actualizar", out.ID, out.Nombre))
	return out, nil
}

// PromocionesActivas devuelve las promociones que deben mostrarse AHORA en el
// carrusel de la pantalla del cliente: las activas y dentro de su vigencia,
// ordenadas por Orden. Es la fuente que el carrusel combina con los slides
// manuales de la empresa (empresa.Publicidad).
func (s *Service) PromocionesActivas(empresaID string) []promocion.Promocion {
	if s.promociones == nil {
		return []promocion.Promocion{}
	}
	now := time.Now().UTC()
	out := []promocion.Promocion{}
	for _, p := range s.promociones.List(empresaID) {
		if !p.Activa {
			continue
		}
		if !promocionVigente(p, now) {
			continue
		}
		out = append(out, p)
	}
	ordenarPromociones(out)
	return out
}

// promocionVigente indica si `now` cae dentro del periodo [Desde, Hasta]. Compara
// por día en YYYY-MM-DD (lexicográfico, correcto para ese formato); extremos
// vacíos significan «sin límite» de ese lado (misma regla que los cupones).
func promocionVigente(p promocion.Promocion, now time.Time) bool {
	hoy := now.Format("2006-01-02")
	if p.Desde != "" && hoy < p.Desde {
		return false
	}
	if p.Hasta != "" && hoy > p.Hasta {
		return false
	}
	return true
}
