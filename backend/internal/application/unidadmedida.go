package application

import (
	"errors"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/unidadmedida"
)

var (
	ErrUnidadesNoDisponible = errors.New("el maestro de unidades no está disponible")
	ErrUnidadNoExiste       = errors.New("unidad de medida no existe")
	ErrUnidadDuplicada      = errors.New("ya existe una unidad con ese símbolo")
	ErrCategoriaInvalida    = errors.New("categoría de unidad inválida (conteo, peso, volumen o longitud)")
)

// ConUnidades cablea el maestro de unidades de medida. Se configura aparte de New
// (como ConCupones/ConListasPrecio) para no tocar la firma del constructor ya
// cableado en cmd/api; sin él, el servicio funciona igual y las unidades quedan
// vacías.
func (s *Service) ConUnidades(r unidadmedida.Repository) *Service {
	s.unidades = r
	return s
}

// Unidades lista las unidades de medida de la empresa, ordenadas por categoría y
// luego por símbolo (para que el select del front las agrupe naturalmente).
func (s *Service) Unidades(empresaID string) []unidadmedida.UnidadMedida {
	if s.unidades == nil {
		return []unidadmedida.UnidadMedida{}
	}
	out := s.unidades.List(empresaID)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Categoria != out[j].Categoria {
			return out[i].Categoria < out[j].Categoria
		}
		return out[i].Simbolo < out[j].Simbolo
	})
	return out
}

// UnidadesActivas devuelve solo las unidades activas (las que alimentan el select
// del editor de producto). Es lo que viaja en /api/bootstrap.
func (s *Service) UnidadesActivas(empresaID string) []unidadmedida.UnidadMedida {
	out := []unidadmedida.UnidadMedida{}
	for _, u := range s.Unidades(empresaID) {
		if u.Activa {
			out = append(out, u)
		}
	}
	return out
}

// simboloUnidadDuplicado indica si otra unidad de la empresa (distinta de
// excluirID) ya usa ese símbolo. La comparación es case-insensitive para evitar
// cuasi-duplicados ("kg"/"KG"); un símbolo vacío nunca colisiona.
func (s *Service) simboloUnidadDuplicado(empresaID, simbolo, excluirID string) bool {
	simbolo = strings.TrimSpace(simbolo)
	if simbolo == "" {
		return false
	}
	for _, u := range s.unidades.List(empresaID) {
		if u.ID != excluirID && strings.EqualFold(strings.TrimSpace(u.Simbolo), simbolo) {
			return true
		}
	}
	return false
}

// saneaUnidad normaliza y valida los campos de una unidad (símbolo requerido,
// categoría válida, símbolo único por empresa excluyendo `id`). Devuelve la unidad
// lista para persistir salvo ID/EmpresaID/Activa/Creada, que fija cada caso de uso.
func (s *Service) saneaUnidad(empresaID, id string, u unidadmedida.UnidadMedida) (unidadmedida.UnidadMedida, error) {
	simbolo := unidadmedida.NormalizarSimbolo(u.Simbolo)
	if simbolo == "" {
		return unidadmedida.UnidadMedida{}, errors.New("símbolo requerido")
	}
	cat := unidadmedida.NormalizarCategoria(u.Categoria)
	if !unidadmedida.CategoriaValida(cat) {
		return unidadmedida.UnidadMedida{}, ErrCategoriaInvalida
	}
	if s.simboloUnidadDuplicado(empresaID, simbolo, id) {
		return unidadmedida.UnidadMedida{}, ErrUnidadDuplicada
	}
	nombre := strings.TrimSpace(u.Nombre)
	if nombre == "" {
		nombre = simbolo // el nombre legible es opcional; por defecto, el propio símbolo
	}
	u.Simbolo = simbolo
	u.Nombre = nombre
	u.Categoria = cat
	return u, nil
}

// CrearUnidad da de alta una unidad de medida de la empresa. Arranca activa.
func (s *Service) CrearUnidad(empresaID, actor, origen string, u unidadmedida.UnidadMedida) (unidadmedida.UnidadMedida, error) {
	if s.unidades == nil {
		return unidadmedida.UnidadMedida{}, ErrUnidadesNoDisponible
	}
	u, err := s.saneaUnidad(empresaID, "", u)
	if err != nil {
		return unidadmedida.UnidadMedida{}, err
	}
	u.ID = ""
	u.EmpresaID = empresaID
	u.Activa = true
	u.Creada = ahora()
	out := s.unidades.Create(u)
	s.audit.Append(evento(empresaID, actor, origen, "config.unidad.crear", out.ID, out.Simbolo+" "+out.Nombre))
	return out, nil
}

// ActualizarUnidad edita una unidad existente del tenant (maestro editable, no
// ledger). Conserva la fecha de creación. Es un PATCH parcial: `activa` es un
// puntero — nil = no enviado = conserva el estado actual, para que un PATCH que no
// toca ese campo no desactive la unidad por omisión; un puntero no-nil aplica el
// valor (incluido false, que la desactiva).
func (s *Service) ActualizarUnidad(empresaID, id, actor, origen string, u unidadmedida.UnidadMedida, activa *bool) (unidadmedida.UnidadMedida, error) {
	if s.unidades == nil {
		return unidadmedida.UnidadMedida{}, ErrUnidadesNoDisponible
	}
	cur, ok := s.unidades.ByID(empresaID, id)
	if !ok {
		return unidadmedida.UnidadMedida{}, ErrUnidadNoExiste
	}
	u, err := s.saneaUnidad(empresaID, id, u)
	if err != nil {
		return unidadmedida.UnidadMedida{}, err
	}
	u.ID = id
	u.EmpresaID = empresaID
	u.Creada = cur.Creada
	// Estado activo: puntero nil = campo ausente = se conserva el actual.
	if activa != nil {
		u.Activa = *activa
	} else {
		u.Activa = cur.Activa
	}
	out, ok := s.unidades.Update(u)
	if !ok {
		return unidadmedida.UnidadMedida{}, ErrUnidadNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.unidad.actualizar", out.ID, out.Simbolo+" "+out.Nombre))
	return out, nil
}

// DesactivarUnidad desactiva (soft-disable) una unidad de la empresa. No borra:
// pone Activa=false para conservar el histórico y los productos que la usan; se
// reactiva editándola con Activa=true.
func (s *Service) DesactivarUnidad(empresaID, id, actor, origen string) error {
	if s.unidades == nil {
		return ErrUnidadesNoDisponible
	}
	u, ok := s.unidades.ByID(empresaID, id)
	if !ok {
		return ErrUnidadNoExiste
	}
	u.Activa = false
	if _, ok := s.unidades.Update(u); !ok {
		return ErrUnidadNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.unidad.desactivar", id, u.Simbolo))
	return nil
}
