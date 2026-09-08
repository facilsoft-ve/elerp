package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// ASIGNACIÓN DE MESAS A MESONEROS
//
// La asignación organiza el turno: qué mesas o zonas atiende cada mesonero. Es una guía,
// no un candado — por defecto un mesonero puede tomar una mesa de otro, se le advierte y
// queda en la bitácora. Con `AsignacionEstricta` encendida en la configuración de la sede
// pasa a ser un candado y el servidor rechaza el intento.
//
// Un mesonero SIN asignación atiende cualquier mesa (el caso normal en un local chico).
// Una mesa que no está asignada a nadie la atiende cualquiera, sin advertencia.

var (
	ErrAsignacionNoDisponible = errors.New("la asignación de mesas no está disponible")
	ErrMesoneroRequerido      = errors.New("hay que indicar el mesonero")
	// ErrMesaDeOtroMesonero solo se devuelve con la asignación ESTRICTA encendida.
	ErrMesaDeOtroMesonero = errors.New("esta mesa está asignada a otro mesonero")
)

// ConAsignacionMesas cablea la asignación de mesas y la configuración del salón. Se
// configura aparte de New (como ConMesas) para no romper las firmas de los constructores.
func (s *Service) ConAsignacionMesas(a mesa.AsignacionRepository, cfg mesa.ConfigSalonRepository) *Service {
	s.asignaciones = a
	s.configSalon = cfg
	return s
}

// ConfigSalon devuelve la configuración del módulo en la sede (con sus defaults).
func (s *Service) ConfigSalon(empresaID, sedeID string) mesa.ConfigSalon {
	if s.configSalon != nil {
		if c, ok := s.configSalon.Get(empresaID, sedeID); ok {
			return c
		}
	}
	return mesa.ConfigSalon{EmpresaID: empresaID, SedeID: sedeID}
}

// GuardarConfigSalon fija la configuración del módulo en la sede.
func (s *Service) GuardarConfigSalon(empresaID, sedeID, actor, origen string, estricta bool) (mesa.ConfigSalon, error) {
	if s.configSalon == nil {
		return mesa.ConfigSalon{}, ErrAsignacionNoDisponible
	}
	c := mesa.ConfigSalon{
		EmpresaID: empresaID, SedeID: sedeID,
		AsignacionEstricta: estricta,
		Actualizada:        time.Now().UTC().Format(time.RFC3339),
	}
	out := s.configSalon.Upsert(c)
	modo := "flexible (se advierte y se registra)"
	if estricta {
		modo = "estricta (se rechaza)"
	}
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.config", "asignacion", modo))
	return out, nil
}

// Asignaciones devuelve las asignaciones de la sede, ordenadas por nombre.
func (s *Service) Asignaciones(empresaID, sedeID string) []mesa.Asignacion {
	if s.asignaciones == nil {
		return []mesa.Asignacion{}
	}
	out := s.asignaciones.List(empresaID, sedeID)
	for i := range out {
		if out[i].Mesas == nil {
			out[i].Mesas = []string{}
		}
		if out[i].Zonas == nil {
			out[i].Zonas = []string{}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nombre < out[j].Nombre })
	return out
}

// GuardarAsignacion fija las mesas y zonas de UN mesonero. Sin mesas ni zonas la
// asignación se borra: «sin asignar» y «asignado a nada» son lo mismo (atiende
// cualquier mesa) y guardar un registro vacío solo confundiría la pantalla.
func (s *Service) GuardarAsignacion(empresaID, sedeID, actor, origen string, a mesa.Asignacion) (mesa.Asignacion, error) {
	if s.asignaciones == nil {
		return mesa.Asignacion{}, ErrAsignacionNoDisponible
	}
	if strings.TrimSpace(a.UsuarioID) == "" {
		return mesa.Asignacion{}, ErrMesoneroRequerido
	}
	a.EmpresaID, a.SedeID = empresaID, sedeID
	a.Normalizar()

	// Las mesas tienen que existir en ESTA sede (si no, la asignación apunta a nada).
	if len(a.Mesas) > 0 && s.mesas != nil {
		validas := map[string]bool{}
		for _, m := range s.mesas.List(empresaID, sedeID) {
			validas[m.ID] = true
		}
		filtradas := make([]string, 0, len(a.Mesas))
		for _, id := range a.Mesas {
			if validas[id] {
				filtradas = append(filtradas, id)
			}
		}
		a.Mesas = filtradas
	}

	if a.SinAsignar() {
		s.asignaciones.Delete(empresaID, sedeID, a.UsuarioID)
		s.audit.Append(evento(empresaID, actor, origen, "restaurante.asignacion", a.UsuarioID, "sin asignación (atiende cualquier mesa)"))
		return mesa.Asignacion{EmpresaID: empresaID, SedeID: sedeID, UsuarioID: a.UsuarioID, Nombre: a.Nombre, Mesas: []string{}, Zonas: []string{}}, nil
	}

	a.Actualizada = time.Now().UTC().Format(time.RFC3339)
	out := s.asignaciones.Upsert(a)
	s.audit.Append(evento(empresaID, actor, origen, "restaurante.asignacion", a.UsuarioID,
		detalleAsignacion(a)))
	return out, nil
}

func detalleAsignacion(a mesa.Asignacion) string {
	partes := []string{}
	if len(a.Zonas) > 0 {
		partes = append(partes, "zonas: "+strings.Join(a.Zonas, ", "))
	}
	if len(a.Mesas) > 0 {
		partes = append(partes, fmt.Sprintf("%d mesa(s)", len(a.Mesas)))
	}
	return strings.Join(partes, " · ")
}

// mesonerosDeMesa devuelve los mesoneros que tienen asignada esta mesa (por id o por su
// zona). Vacío significa que la mesa no es de nadie en particular.
func (s *Service) mesonerosDeMesa(empresaID, sedeID string, m mesa.Mesa) []mesa.Asignacion {
	if s.asignaciones == nil {
		return nil
	}
	out := []mesa.Asignacion{}
	for _, a := range s.asignaciones.List(empresaID, sedeID) {
		if a.Cubre(m) {
			out = append(out, a)
		}
	}
	return out
}

// verificarMesaDelMesonero decide si `actorID` puede tomar la mesa `m`.
//
// Devuelve (ajena, dueños, error):
//   - ajena=false: la mesa no es de nadie o es suya → adelante, sin ruido.
//   - ajena=true con error nil: es de otro pero la config es flexible → adelante, y el
//     llamador debe dejar constancia en la bitácora.
//   - error ErrMesaDeOtroMesonero: la config es estricta → se rechaza.
//
// Solo aplica al rol MESONERO: la caja y la dueña atienden cualquier mesa por definición
// (son quienes cubren y cobran), así que la asignación no las limita.
func (s *Service) verificarMesaDelMesonero(empresaID, sedeID, actorID, rolActor string, m mesa.Mesa) (bool, []mesa.Asignacion, error) {
	if rolActor != usuario.RolMesonero {
		return false, nil, nil
	}
	duenos := s.mesonerosDeMesa(empresaID, sedeID, m)
	if len(duenos) == 0 {
		return false, nil, nil // mesa libre de asignación: la atiende cualquiera
	}
	for _, d := range duenos {
		if d.UsuarioID == actorID {
			return false, duenos, nil // es suya
		}
	}
	if s.ConfigSalon(empresaID, sedeID).AsignacionEstricta {
		return true, duenos, ErrMesaDeOtroMesonero
	}
	return true, duenos, nil
}

// nombresDe une los nombres de los mesoneros para la bitácora.
func nombresDe(as []mesa.Asignacion) string {
	ns := make([]string, 0, len(as))
	for _, a := range as {
		if a.Nombre != "" {
			ns = append(ns, a.Nombre)
		}
	}
	return strings.Join(ns, ", ")
}
