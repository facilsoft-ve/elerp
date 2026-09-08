package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mornix/elerp/internal/domain/aplicacion"
)

var (
	ErrModulosNoDisponible = errors.New("el marco de aplicaciones no está disponible")
	ErrModuloNoExiste      = errors.New("el módulo no existe en el catálogo")
	ErrModuloProximamente  = errors.New("ese módulo todavía no está disponible")
	ErrModuloCore          = errors.New("ese módulo viene incluido: no se instala ni se desactiva")
	ErrModuloNoInstalado   = errors.New("el módulo no está instalado")
	ErrDependenciaFaltante = errors.New("faltan módulos requeridos")
	ErrDependientesActivos = errors.New("otros módulos activos dependen de este")
)

// ConModulos cablea el marco de aplicaciones (se configura aparte de New, como
// ConAlmacenes/ConUnidades). Sin él, todo módulo core queda activo y los
// comercializables inactivos.
func (s *Service) ConModulos(r aplicacion.Repository) *Service {
	s.modulos = r
	return s
}

// ModuloView es una entrada del catálogo con el estado de la empresa resuelto, para
// la vitrina de Aplicaciones.
type ModuloView struct {
	aplicacion.Modulo
	Instalado bool `json:"instalado"`
	Activo    bool `json:"activo"`
	// RequeridoPor lista los NOMBRES de los módulos del catálogo que dependen de este
	// (para mostrar "lo requieren: …" y advertir antes de desinstalar).
	RequeridoPor []string `json:"requeridoPor,omitempty"`
}

func (s *Service) nombreModulo(id string) string {
	if m, ok := aplicacion.EnCatalogo(id); ok {
		return m.Nombre
	}
	return id
}

// dependenciasFaltantes devuelve los NOMBRES de los módulos requeridos por `m` que
// NO están activos en la empresa.
func (s *Service) dependenciasFaltantes(empresaID string, m aplicacion.Modulo) []string {
	faltan := []string{}
	for _, dep := range m.Requiere {
		if !s.ModuloActivo(empresaID, dep) {
			faltan = append(faltan, s.nombreModulo(dep))
		}
	}
	return faltan
}

// dependientesActivos devuelve los NOMBRES de los módulos ACTIVOS que requieren a
// `moduloID` (los que se romperían si se desactiva/desinstala).
func (s *Service) dependientesActivos(empresaID, moduloID string) []string {
	out := []string{}
	for _, m := range aplicacion.Catalogo() {
		if m.ID == moduloID {
			continue
		}
		for _, dep := range m.Requiere {
			if dep != moduloID {
				continue
			}
			if _, act := s.estadoModulo(empresaID, m); act {
				out = append(out, m.Nombre)
			}
		}
	}
	return out
}

// moduloReferidoPor devuelve los nombres de TODOS los módulos del catálogo que
// declaran a `moduloID` como dependencia (activos o no), para la vista.
func (s *Service) moduloReferidoPor(moduloID string) []string {
	out := []string{}
	for _, m := range aplicacion.Catalogo() {
		for _, dep := range m.Requiere {
			if dep == moduloID {
				out = append(out, m.Nombre)
			}
		}
	}
	return out
}

// estadoModulo resuelve (instalado, activo) de un módulo para una empresa. Los core
// están siempre instalados y activos; los demás salen del repo.
func (s *Service) estadoModulo(empresaID string, m aplicacion.Modulo) (bool, bool) {
	if m.Core {
		return true, true
	}
	if s.modulos == nil {
		return false, false
	}
	if in, ok := s.modulos.ByID(empresaID, m.ID); ok {
		return in.Instalado, in.Instalado && in.Activo
	}
	return false, false
}

// Modulos devuelve el catálogo con el estado de la empresa (para la vitrina).
func (s *Service) Modulos(empresaID string) []ModuloView {
	cat := aplicacion.Catalogo()
	out := make([]ModuloView, 0, len(cat))
	for _, m := range cat {
		inst, act := s.estadoModulo(empresaID, m)
		out = append(out, ModuloView{Modulo: m, Instalado: inst, Activo: act, RequeridoPor: s.moduloReferidoPor(m.ID)})
	}
	return out
}

// ModulosActivos devuelve los IDs de los módulos ACTIVOS de la empresa (core
// siempre + los comercializables instalados y activos). Viaja en el bootstrap para
// que la UI oculte lo apagado.
func (s *Service) ModulosActivos(empresaID string) []string {
	out := []string{}
	for _, m := range aplicacion.Catalogo() {
		if _, act := s.estadoModulo(empresaID, m); act {
			out = append(out, m.ID)
		}
	}
	return out
}

// ModuloActivo indica si un módulo está activo para la empresa. Es el helper de
// GATING que consultan los casos de uso de cada módulo (p. ej. Promociones). Un id
// desconocido se considera activo (no se gatea lo que no está en el catálogo).
func (s *Service) ModuloActivo(empresaID, moduloID string) bool {
	m, ok := aplicacion.EnCatalogo(moduloID)
	if !ok {
		return true
	}
	_, act := s.estadoModulo(empresaID, m)
	return act
}

// guarda valida el id del catálogo y devuelve el módulo, o el error de negocio.
func (s *Service) moduloComercial(moduloID string) (aplicacion.Modulo, error) {
	if s.modulos == nil {
		return aplicacion.Modulo{}, ErrModulosNoDisponible
	}
	m, ok := aplicacion.EnCatalogo(moduloID)
	if !ok {
		return aplicacion.Modulo{}, ErrModuloNoExiste
	}
	if m.Core {
		return aplicacion.Modulo{}, ErrModuloCore
	}
	if m.Proximamente {
		return aplicacion.Modulo{}, ErrModuloProximamente
	}
	return m, nil
}

func (s *Service) guardarInstalacion(empresaID string, m aplicacion.Modulo, instalado, activo bool, actor, origen, evento_ string) (aplicacion.Instalacion, error) {
	out := s.modulos.Upsert(aplicacion.Instalacion{
		EmpresaID: empresaID, ModuloID: m.ID, Instalado: instalado, Activo: activo, Actualizada: ahora(),
	})
	s.audit.Append(evento(empresaID, actor, origen, evento_, m.ID, m.Nombre))
	return out, nil
}

// requerirDependencias falla con un mensaje específico si al activar/instalar `m`
// faltan módulos requeridos activos.
func (s *Service) requerirDependencias(empresaID string, m aplicacion.Modulo) error {
	if faltan := s.dependenciasFaltantes(empresaID, m); len(faltan) > 0 {
		return fmt.Errorf("%w: activa primero %s", ErrDependenciaFaltante, strings.Join(faltan, ", "))
	}
	return nil
}

// impedirSiTieneDependientes falla si otros módulos ACTIVOS requieren a `moduloID`
// (no se puede desactivar/desinstalar hasta resolver esa dependencia).
func (s *Service) impedirSiTieneDependientes(empresaID, moduloID string) error {
	if dep := s.dependientesActivos(empresaID, moduloID); len(dep) > 0 {
		return fmt.Errorf("%w: %s lo requiere(n). Desactívalo(s) primero", ErrDependientesActivos, strings.Join(dep, ", "))
	}
	return nil
}

// Instalar da de alta el módulo comercial para la empresa (queda activo).
func (s *Service) Instalar(empresaID, moduloID, actor, origen string) (aplicacion.Instalacion, error) {
	m, err := s.moduloComercial(moduloID)
	if err != nil {
		return aplicacion.Instalacion{}, err
	}
	if err := s.requerirDependencias(empresaID, m); err != nil {
		return aplicacion.Instalacion{}, err
	}
	return s.guardarInstalacion(empresaID, m, true, true, actor, origen, "config.modulo.instalar")
}

// Desinstalar quita el módulo (queda no instalado e inactivo).
func (s *Service) Desinstalar(empresaID, moduloID, actor, origen string) (aplicacion.Instalacion, error) {
	m, err := s.moduloComercial(moduloID)
	if err != nil {
		return aplicacion.Instalacion{}, err
	}
	if err := s.impedirSiTieneDependientes(empresaID, m.ID); err != nil {
		return aplicacion.Instalacion{}, err
	}
	return s.guardarInstalacion(empresaID, m, false, false, actor, origen, "config.modulo.desinstalar")
}

// Activar prende un módulo ya instalado.
func (s *Service) Activar(empresaID, moduloID, actor, origen string) (aplicacion.Instalacion, error) {
	m, err := s.moduloComercial(moduloID)
	if err != nil {
		return aplicacion.Instalacion{}, err
	}
	if in, ok := s.modulos.ByID(empresaID, m.ID); !ok || !in.Instalado {
		return aplicacion.Instalacion{}, ErrModuloNoInstalado
	}
	if err := s.requerirDependencias(empresaID, m); err != nil {
		return aplicacion.Instalacion{}, err
	}
	return s.guardarInstalacion(empresaID, m, true, true, actor, origen, "config.modulo.activar")
}

// Desactivar apaga un módulo sin desinstalarlo (conserva su configuración).
func (s *Service) Desactivar(empresaID, moduloID, actor, origen string) (aplicacion.Instalacion, error) {
	m, err := s.moduloComercial(moduloID)
	if err != nil {
		return aplicacion.Instalacion{}, err
	}
	if in, ok := s.modulos.ByID(empresaID, m.ID); !ok || !in.Instalado {
		return aplicacion.Instalacion{}, ErrModuloNoInstalado
	}
	if err := s.impedirSiTieneDependientes(empresaID, m.ID); err != nil {
		return aplicacion.Instalacion{}, err
	}
	return s.guardarInstalacion(empresaID, m, true, false, actor, origen, "config.modulo.desactivar")
}
