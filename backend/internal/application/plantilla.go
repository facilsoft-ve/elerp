package application

import (
	"errors"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/plantilla"
)

// Errores de negocio de los formatos de documento.
var (
	ErrPlantillasNoDisponible = errors.New("los formatos de documento no están disponibles")
	ErrPlantillaNoExiste      = errors.New("el formato no existe")
	ErrTipoDocInvalido        = errors.New("tipo de documento inválido")
	ErrPapelInvalido          = errors.New("tamaño de papel inválido")
	ErrPlantillaSinNombre     = errors.New("el formato necesita un nombre")
	ErrDimensionInvalida      = errors.New("un papel a medida necesita ancho y alto mayores que cero (en mm)")
	ErrImagenInsegura         = errors.New("la imagen debe ser PNG, JPEG o WebP y pesar menos de 512 KB")
)

// ConPlantillas cablea el maestro de formatos de documento. Se configura aparte
// de New (como ConUnidades/ConCupones) para no tocar la firma del constructor;
// sin él, el servicio funciona igual y los formatos quedan vacíos.
func (s *Service) ConPlantillas(r plantilla.Repository) *Service {
	s.plantillas = r
	return s
}

// Plantillas lista los formatos de la empresa, ordenados por tipo y luego por
// nombre (para que el front los agrupe naturalmente).
func (s *Service) Plantillas(empresaID string) []plantilla.Plantilla {
	if s.plantillas == nil {
		return []plantilla.Plantilla{}
	}
	out := s.plantillas.List(empresaID)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Tipo != out[j].Tipo {
			return out[i].Tipo < out[j].Tipo
		}
		return strings.ToLower(out[i].Nombre) < strings.ToLower(out[j].Nombre)
	})
	return out
}

// saneaPlantilla normaliza y valida los campos comunes de un formato. Fija las
// dimensiones del papel: para un tamaño preestablecido las toma del dominio; para
// `custom` exige que el usuario haya dado ancho/alto. No toca ID/EmpresaID/
// Sedes/Predeterminada/Creada, que gobiernan los casos de uso concretos.
func (s *Service) saneaPlantilla(p plantilla.Plantilla) (plantilla.Plantilla, error) {
	p.Nombre = strings.TrimSpace(p.Nombre)
	if p.Nombre == "" {
		return plantilla.Plantilla{}, ErrPlantillaSinNombre
	}
	p.Tipo = plantilla.NormalizarTipo(p.Tipo)
	if !plantilla.TipoValido(p.Tipo) {
		return plantilla.Plantilla{}, ErrTipoDocInvalido
	}
	p.Papel = strings.ToLower(strings.TrimSpace(p.Papel))
	if !plantilla.PapelValido(p.Papel) {
		return plantilla.Plantilla{}, ErrPapelInvalido
	}
	if ancho, alto, ok := plantilla.PapelDimensiones(p.Papel); ok {
		p.AnchoMM, p.AltoMM = ancho, alto
	} else if p.AnchoMM <= 0 || p.AltoMM <= 0 {
		return plantilla.Plantilla{}, ErrDimensionInvalida
	}
	// Solo se conservan bloques con un tipo conocido; el resto se descarta en
	// silencio (un cliente viejo o corrupto no debe ensuciar la plantilla). Las
	// imágenes se validan en el SERVIDOR (formato seguro + peso), no solo en el
	// navegador: una imagen fuera de PNG/JPEG/WebP o demasiado pesada se rechaza.
	limpios := make([]plantilla.Bloque, 0, len(p.Bloques))
	for _, b := range p.Bloques {
		b.Tipo = strings.ToLower(strings.TrimSpace(b.Tipo))
		if !plantilla.TipoBloqueValido(b.Tipo) {
			continue
		}
		if b.Imagen != "" && !plantilla.ImagenDataURISegura(b.Imagen) {
			return plantilla.Plantilla{}, ErrImagenInsegura
		}
		limpios = append(limpios, b)
	}
	p.Bloques = limpios
	if p.Sedes == nil {
		p.Sedes = []string{}
	}
	return p, nil
}

// CrearPlantilla da de alta un formato de documento. Arranca activa y sin sedes
// asignadas (la asignación es una operación aparte). Si es el PRIMER formato de su
// tipo en la empresa, se marca predeterminado para que ese tipo siempre resuelva.
func (s *Service) CrearPlantilla(empresaID, actor, origen string, p plantilla.Plantilla) (plantilla.Plantilla, error) {
	if s.plantillas == nil {
		return plantilla.Plantilla{}, ErrPlantillasNoDisponible
	}
	p, err := s.saneaPlantilla(p)
	if err != nil {
		return plantilla.Plantilla{}, err
	}
	// Sin bloques (alta desde el modal "Nuevo formato"): se arranca del PRESET del
	// tipo y el tamaño elegidos, para que el editor abra con una disposición lista
	// en vez de un lienzo en blanco.
	if len(p.Bloques) == 0 {
		p.Bloques = plantilla.PorDefecto(p.Tipo, p.Papel).Bloques
	}
	p.ID = ""
	p.EmpresaID = empresaID
	p.Activa = true
	p.Sedes = []string{}
	p.Creada = ahora()
	p.Actualizada = p.Creada
	// Primer formato de su tipo ⇒ predeterminado (así el tipo siempre resuelve).
	p.Predeterminada = !s.existeTipo(empresaID, p.Tipo)
	out := s.plantillas.Create(p)
	s.audit.Append(evento(empresaID, actor, origen, "config.formato.crear", out.ID, out.Tipo+" · "+out.Nombre))
	return out, nil
}

// existeTipo indica si la empresa ya tiene al menos un formato de ese tipo.
func (s *Service) existeTipo(empresaID, tipo string) bool {
	for _, p := range s.plantillas.List(empresaID) {
		if p.Tipo == tipo {
			return true
		}
	}
	return false
}

// ActualizarPlantilla edita un formato existente (maestro editable). Conserva
// fecha de creación, la asignación de sedes y la marca de predeterminado: esos se
// gobiernan por sus operaciones propias (AsignarPlantillaSede /
// FijarPlantillaPredeterminada), no por un guardado del lienzo.
func (s *Service) ActualizarPlantilla(empresaID, id, actor, origen string, p plantilla.Plantilla) (plantilla.Plantilla, error) {
	if s.plantillas == nil {
		return plantilla.Plantilla{}, ErrPlantillasNoDisponible
	}
	cur, ok := s.plantillas.ByID(empresaID, id)
	if !ok {
		return plantilla.Plantilla{}, ErrPlantillaNoExiste
	}
	p, err := s.saneaPlantilla(p)
	if err != nil {
		return plantilla.Plantilla{}, err
	}
	p.ID = id
	p.EmpresaID = empresaID
	p.Creada = cur.Creada
	p.Sedes = cur.Sedes
	p.Predeterminada = cur.Predeterminada
	p.Activa = cur.Activa
	p.Actualizada = ahora()
	out, ok := s.plantillas.Update(p)
	if !ok {
		return plantilla.Plantilla{}, ErrPlantillaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.formato.actualizar", out.ID, out.Tipo+" · "+out.Nombre))
	return out, nil
}

// EliminarPlantilla borra un formato. Si era el predeterminado de su tipo y quedan
// otros del mismo tipo, el más reciente hereda la marca para que el tipo siga
// resolviendo. Sus sedes asignadas quedan sin asignación explícita: caen al
// predeterminado del tipo.
func (s *Service) EliminarPlantilla(empresaID, id, actor, origen string) error {
	if s.plantillas == nil {
		return ErrPlantillasNoDisponible
	}
	cur, ok := s.plantillas.ByID(empresaID, id)
	if !ok {
		return ErrPlantillaNoExiste
	}
	if !s.plantillas.Delete(empresaID, id) {
		return ErrPlantillaNoExiste
	}
	if cur.Predeterminada {
		s.reasignarPredeterminado(empresaID, cur.Tipo)
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.formato.eliminar", id, cur.Tipo+" · "+cur.Nombre))
	return nil
}

// reasignarPredeterminado marca como predeterminado al formato más reciente que
// quede de un tipo (cuando el anterior predeterminado se borró). No hace nada si
// ya no quedan formatos de ese tipo.
func (s *Service) reasignarPredeterminado(empresaID, tipo string) {
	var elegido *plantilla.Plantilla
	for _, p := range s.plantillas.List(empresaID) {
		if p.Tipo != tipo {
			continue
		}
		if elegido == nil || p.Creada > elegido.Creada {
			cp := p
			elegido = &cp
		}
	}
	if elegido == nil {
		return
	}
	elegido.Predeterminada = true
	elegido.Actualizada = ahora()
	s.plantillas.Update(*elegido)
}

// FijarPlantillaPredeterminada marca un formato como el predeterminado de su tipo
// y quita la marca a los demás del mismo tipo (a lo sumo uno lo es).
func (s *Service) FijarPlantillaPredeterminada(empresaID, id, actor, origen string) error {
	if s.plantillas == nil {
		return ErrPlantillasNoDisponible
	}
	obj, ok := s.plantillas.ByID(empresaID, id)
	if !ok {
		return ErrPlantillaNoExiste
	}
	for _, p := range s.plantillas.List(empresaID) {
		if p.Tipo != obj.Tipo {
			continue
		}
		quiere := p.ID == id
		if p.Predeterminada != quiere {
			p.Predeterminada = quiere
			p.Actualizada = ahora()
			s.plantillas.Update(p)
		}
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.formato.predeterminado", id, obj.Tipo+" · "+obj.Nombre))
	return nil
}

// AsignarPlantillaSede fija qué formato usa una SEDE para un tipo de documento.
// La sede se retira de cualquier otro formato del mismo tipo (usa exactamente uno
// por tipo) y se agrega al indicado. Un plantillaID vacío deja a la sede SIN
// asignación explícita: caerá al formato predeterminado del tipo.
func (s *Service) AsignarPlantillaSede(empresaID, tipo, sedeID, plantillaID, actor, origen string) error {
	if s.plantillas == nil {
		return ErrPlantillasNoDisponible
	}
	tipo = plantilla.NormalizarTipo(tipo)
	if !plantilla.TipoValido(tipo) {
		return ErrTipoDocInvalido
	}
	if strings.TrimSpace(sedeID) == "" {
		return errors.New("falta la sede")
	}
	// Valida el destino (si se indicó) y que sea del tipo correcto.
	if plantillaID != "" {
		destino, ok := s.plantillas.ByID(empresaID, plantillaID)
		if !ok {
			return ErrPlantillaNoExiste
		}
		if destino.Tipo != tipo {
			return errors.New("el formato no es del tipo indicado")
		}
	}
	for _, p := range s.plantillas.List(empresaID) {
		if p.Tipo != tipo {
			continue
		}
		antes := len(p.Sedes)
		p.Sedes = sinSede(p.Sedes, sedeID)
		if p.ID == plantillaID {
			p.Sedes = append(p.Sedes, sedeID)
		}
		if len(p.Sedes) != antes || p.ID == plantillaID {
			p.Actualizada = ahora()
			s.plantillas.Update(p)
		}
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.formato.asignar", plantillaID, tipo+" → sede "+sedeID))
	return nil
}

// sinSede devuelve la lista sin la sede dada (preservando el orden).
func sinSede(sedes []string, sedeID string) []string {
	out := make([]string, 0, len(sedes))
	for _, s := range sedes {
		if s != sedeID {
			out = append(out, s)
		}
	}
	return out
}

// ResolverPlantilla devuelve el formato que una sede debe usar para un tipo de
// documento: primero el que tenga a esa sede asignada; si ninguno, el
// predeterminado del tipo; si tampoco, el primero activo del tipo. El segundo
// valor es false cuando la empresa no tiene ningún formato usable de ese tipo.
func (s *Service) ResolverPlantilla(empresaID, tipo, sedeID string) (plantilla.Plantilla, bool) {
	if s.plantillas == nil {
		return plantilla.Plantilla{}, false
	}
	tipo = plantilla.NormalizarTipo(tipo)
	var predeterminada, primera *plantilla.Plantilla
	for _, p := range s.plantillas.List(empresaID) {
		if p.Tipo != tipo || !p.Activa {
			continue
		}
		for _, sd := range p.Sedes {
			if sd == sedeID {
				return p, true
			}
		}
		if p.Predeterminada {
			cp := p
			predeterminada = &cp
		}
		if primera == nil {
			cp := p
			primera = &cp
		}
	}
	if predeterminada != nil {
		return *predeterminada, true
	}
	if primera != nil {
		return *primera, true
	}
	return plantilla.Plantilla{}, false
}
