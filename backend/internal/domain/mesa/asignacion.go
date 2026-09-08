package mesa

import "strings"

// Asignacion vincula un MESONERO con las mesas que atiende en una sede: mesas sueltas
// y/o ZONAS completas (Salón, Terraza, Barra). Asignar una zona cubre sus mesas actuales
// y las que se agreguen después, que es como se organiza un turno de verdad; las mesas
// sueltas cubren la excepción.
//
// Regla de fondo: la asignación es una GUÍA de organización, no un candado. Si un
// mesonero no tiene nada asignado atiende cualquier mesa, y si toma una mesa de otro
// puede hacerlo — el sistema lo advierte y lo deja registrado. Un bloqueo duro haría que
// en un turno movido el mesero abandone el sistema y anote en papel, que es peor que un
// dato de organización imperfecto.
//
// Es configuración por sede (maestro editable), no un ledger.
type Asignacion struct {
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// UsuarioID es el mesonero (usuario.Membresia.UsuarioID).
	UsuarioID string `json:"usuarioId" bson:"usuarioid"`
	// Nombre se guarda desnormalizado para poder mostrar «esta mesa es de X» sin
	// resolver el usuario en cada pantalla.
	Nombre string `json:"nombre" bson:"nombre"`
	// Mesas son ids de mesa asignados explícitamente.
	Mesas []string `json:"mesas" bson:"mesas"`
	// Zonas son nombres de zona (mesa.Zona) asignados completos.
	Zonas       []string `json:"zonas" bson:"zonas"`
	Actualizada string   `json:"actualizada" bson:"actualizada"` // RFC3339
}

// SinAsignar indica que este mesonero no tiene nada asignado y, por lo tanto, puede
// atender cualquier mesa.
func (a Asignacion) SinAsignar() bool { return len(a.Mesas) == 0 && len(a.Zonas) == 0 }

// Cubre indica si esta asignación incluye la mesa dada, por id o por su zona.
func (a Asignacion) Cubre(m Mesa) bool {
	for _, id := range a.Mesas {
		if id == m.ID {
			return true
		}
	}
	zona := strings.TrimSpace(strings.ToLower(m.Zona))
	if zona == "" {
		return false
	}
	for _, z := range a.Zonas {
		if strings.TrimSpace(strings.ToLower(z)) == zona {
			return true
		}
	}
	return false
}

// Normalizar limpia duplicados y valores vacíos de mesas y zonas.
func (a *Asignacion) Normalizar() {
	a.Mesas = unicos(a.Mesas)
	a.Zonas = unicos(a.Zonas)
	a.Nombre = strings.TrimSpace(a.Nombre)
}

func unicos(xs []string) []string {
	visto := map[string]bool{}
	out := []string{}
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		clave := strings.ToLower(x)
		if visto[clave] {
			continue
		}
		visto[clave] = true
		out = append(out, x)
	}
	return out
}

// AsignacionRepository persiste las asignaciones de una sede. Una por mesonero:
// Upsert reemplaza la del par (empresa, sede, usuario).
type AsignacionRepository interface {
	List(empresaID, sedeID string) []Asignacion
	Upsert(a Asignacion) Asignacion
	Delete(empresaID, sedeID, usuarioID string) bool
}
