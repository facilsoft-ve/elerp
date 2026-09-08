package application

import (
	"errors"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/almacen"
)

var (
	ErrAlmacenesNoDisponible = errors.New("el maestro de almacenes no está disponible")
	ErrAlmacenNoExiste       = errors.New("almacén no existe")
	ErrAlmacenDuplicado      = errors.New("ya existe un almacén con ese nombre en la sede")
	ErrAlmacenTipoInvalido   = errors.New("tipo de almacén inválido")
	ErrAlmacenSedeRequerida  = errors.New("el almacén debe pertenecer a una sede")
	ErrUltimoAlmacenActivo   = errors.New("no se puede desactivar el único almacén activo de la sede")
	ErrAlmacenPrincipal      = errors.New("no se puede desactivar el almacén principal: marca otro como principal primero")
	ErrCapacidadSinUnidad    = errors.New("indica la unidad de la capacidad (p. ej. kg o L)")
	ErrRubroNoAdmitido       = errors.New("el almacén no admite productos de ese rubro")
)

// ConAlmacenes cablea el maestro de almacenes (se configura aparte de New, como
// ConUnidades/ConCupones). Sin él, el servicio funciona igual y los almacenes
// quedan vacíos.
func (s *Service) ConAlmacenes(r almacen.Repository) *Service {
	s.almacenes = r
	return s
}

// Almacenes lista los almacenes de la empresa (todas las sedes), ordenados por sede
// y, dentro de cada sede, con el principal primero y luego por nombre.
func (s *Service) Almacenes(empresaID string) []almacen.Almacen {
	if s.almacenes == nil {
		return []almacen.Almacen{}
	}
	out := s.almacenes.List(empresaID)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SedeID != out[j].SedeID {
			return out[i].SedeID < out[j].SedeID
		}
		if out[i].Principal != out[j].Principal {
			return out[i].Principal
		}
		return out[i].Nombre < out[j].Nombre
	})
	return out
}

// AlmacenesDeSede devuelve los almacenes (activos e inactivos) de una sede.
func (s *Service) AlmacenesDeSede(empresaID, sedeID string) []almacen.Almacen {
	out := []almacen.Almacen{}
	for _, a := range s.Almacenes(empresaID) {
		if a.SedeID == sedeID {
			out = append(out, a)
		}
	}
	return out
}

// AlmacenPrincipalDe devuelve el almacén principal (activo) de una sede. Si ninguno
// está marcado como principal, cae al primer activo. La Fase 2 lo usará para ubicar
// el stock por defecto del POS y la retrocompat del ledger.
func (s *Service) AlmacenPrincipalDe(empresaID, sedeID string) (almacen.Almacen, bool) {
	var primero almacen.Almacen
	tiene := false
	for _, a := range s.AlmacenesDeSede(empresaID, sedeID) {
		if !a.Activo {
			continue
		}
		if a.Principal {
			return a, true
		}
		if !tiene {
			primero, tiene = a, true
		}
	}
	return primero, tiene
}

// almacenParaEscritura resuelve el almacén donde se ubica un movimiento de stock:
// el `deseado` si viene, pertenece a la sede y está activo; si no, el PRINCIPAL de
// la sede. Devuelve "" solo si el maestro no está cableado o la sede no tiene
// almacenes (no debería tras el backfill) — en ese caso el movimiento queda con
// AlmacenID vacío y la proyección por sede lo sigue contando igual (retrocompat).
func (s *Service) almacenParaEscritura(empresaID, sedeID, deseado string) string {
	if s.almacenes == nil {
		return ""
	}
	if deseado != "" {
		if a, ok := s.almacenes.ByID(empresaID, deseado); ok && a.SedeID == sedeID && a.Activo {
			return a.ID
		}
	}
	if a, ok := s.AlmacenPrincipalDe(empresaID, sedeID); ok {
		return a.ID
	}
	return ""
}

// esAlmacenPrincipal indica si el almacén dado es el principal de su sede (usado
// por las proyecciones por almacén para la retrocompat de movimientos sin almacén).
func (s *Service) esAlmacenPrincipal(empresaID string, a almacen.Almacen) bool {
	prin, ok := s.AlmacenPrincipalDe(empresaID, a.SedeID)
	return ok && prin.ID == a.ID
}

func (s *Service) nombreAlmacenDuplicado(empresaID, sedeID, nombre, excluirID string) bool {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return false
	}
	for _, a := range s.AlmacenesDeSede(empresaID, sedeID) {
		if a.ID != excluirID && strings.EqualFold(strings.TrimSpace(a.Nombre), nombre) {
			return true
		}
	}
	return false
}

// saneaAlmacen normaliza y valida (nombre requerido y único por sede, sede requerida,
// tipo válido con default "general"). Deja ID/EmpresaID/Activo/Creada/Principal a
// cada caso de uso.
func (s *Service) saneaAlmacen(empresaID, id, sedeID string, a almacen.Almacen) (almacen.Almacen, error) {
	nombre := strings.TrimSpace(a.Nombre)
	if nombre == "" {
		return almacen.Almacen{}, errors.New("nombre requerido")
	}
	if strings.TrimSpace(sedeID) == "" {
		return almacen.Almacen{}, ErrAlmacenSedeRequerida
	}
	tipo := almacen.NormalizarTipo(a.Tipo)
	if tipo == "" {
		tipo = almacen.TipoGeneral
	}
	if !almacen.TipoValido(tipo) {
		return almacen.Almacen{}, ErrAlmacenTipoInvalido
	}
	if s.nombreAlmacenDuplicado(empresaID, sedeID, nombre, id) {
		return almacen.Almacen{}, ErrAlmacenDuplicado
	}
	// Capacidad opcional: si es >0 exige una unidad (símbolo del maestro). Negativa
	// se trata como sin límite.
	if a.Capacidad < 0 {
		a.Capacidad = 0
	}
	a.CapacidadUnidad = strings.TrimSpace(a.CapacidadUnidad)
	if a.Capacidad > 0 && a.CapacidadUnidad == "" {
		return almacen.Almacen{}, ErrCapacidadSinUnidad
	}
	if a.Capacidad == 0 {
		a.CapacidadUnidad = ""
	}
	// Rubros admitidos: limpiar vacíos y duplicados (lista vacía = admite todos).
	a.RubrosAdmitidos = limpiarLista(a.RubrosAdmitidos)
	a.Nombre = nombre
	a.Tipo = tipo
	a.SedeID = sedeID
	return a, nil
}

// limpiarLista quita espacios, vacíos y duplicados conservando el orden.
func limpiarLista(xs []string) []string {
	out := []string{}
	visto := map[string]bool{}
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" || visto[x] {
			continue
		}
		visto[x] = true
		out = append(out, x)
	}
	return out
}

// --- Ocupación (barra de llenado) y admisión por rubro ---

// factorABase mapea las unidades comunes a su categoría y factor hacia la unidad
// base de esa categoría (kg para peso, L para volumen). Permite sumar la existencia
// de un almacén en la unidad de su capacidad (kg↔g, L↔mL); para otras unidades se
// exige coincidencia exacta de símbolo.
var factorABase = map[string]struct {
	cat string
	f   float64
}{
	"kg": {"peso", 1}, "g": {"peso", 0.001},
	"L": {"volumen", 1}, "mL": {"volumen", 0.001},
}

// aporteACapacidad convierte `cant` de la unidad del producto a la unidad de la
// capacidad. Devuelve 0 si no es convertible (unidad de otra categoría / no medible
// en esa capacidad).
func aporteACapacidad(cant float64, uProducto, uCapacidad string) float64 {
	if uProducto == uCapacidad {
		return cant
	}
	fp, okp := factorABase[uProducto]
	fc, okc := factorABase[uCapacidad]
	if okp && okc && fp.cat == fc.cat && fc.f != 0 {
		return cant * fp.f / fc.f
	}
	return 0
}

// OcupacionDeAlmacen suma la existencia del almacén expresada en la unidad de su
// capacidad. 0 si el almacén no tiene capacidad configurada.
func (s *Service) OcupacionDeAlmacen(empresaID string, a almacen.Almacen) float64 {
	if a.Capacidad <= 0 || a.CapacidadUnidad == "" {
		return 0
	}
	total := 0.0
	ex, err := s.ExistenciasDeAlmacen(empresaID, a.ID)
	if err != nil {
		return 0
	}
	for _, e := range ex {
		if e.Cantidad <= 0 {
			continue
		}
		p, ok := s.productos.BySKU(empresaID, e.SKU)
		if !ok {
			continue
		}
		total += aporteACapacidad(e.Cantidad, p.UnidadBase, a.CapacidadUnidad)
	}
	return round2(total)
}

// AlmacenView es un almacén con su ocupación resuelta (para la vitrina/lista).
type AlmacenView struct {
	almacen.Almacen
	Ocupado float64 `json:"ocupado"` // en CapacidadUnidad; 0 si sin capacidad
}

// AlmacenesConOcupacion devuelve los almacenes de la empresa con su ocupación. Es lo
// que consume la pantalla de Configuración (para pintar la barra de llenado).
func (s *Service) AlmacenesConOcupacion(empresaID string) []AlmacenView {
	base := s.Almacenes(empresaID)
	out := make([]AlmacenView, 0, len(base))
	for _, a := range base {
		out = append(out, AlmacenView{Almacen: a, Ocupado: s.OcupacionDeAlmacen(empresaID, a)})
	}
	return out
}

// verificarAdmisionAlmacen rechaza ingresar un producto cuyo rubro no admite el
// almacén (restricción dura de RubrosAdmitidos). almacenID vacío = sin restricción.
func (s *Service) verificarAdmisionAlmacen(empresaID, almacenID, rubroID string) error {
	if s.almacenes == nil || almacenID == "" {
		return nil
	}
	a, ok := s.almacenes.ByID(empresaID, almacenID)
	if !ok {
		return nil
	}
	if !a.AdmiteRubro(rubroID) {
		return ErrRubroNoAdmitido
	}
	return nil
}

// demoteOtrosPrincipales garantiza que solo UN almacén de la sede sea principal.
func (s *Service) demoteOtrosPrincipales(empresaID, sedeID, exceptoID string) {
	for _, a := range s.AlmacenesDeSede(empresaID, sedeID) {
		if a.ID != exceptoID && a.Principal {
			a.Principal = false
			s.almacenes.Update(a)
		}
	}
}

// CrearAlmacen da de alta un almacén en una sede. Arranca activo. El PRIMER almacén
// de una sede queda como principal automáticamente.
func (s *Service) CrearAlmacen(empresaID, actor, origen string, a almacen.Almacen) (almacen.Almacen, error) {
	if s.almacenes == nil {
		return almacen.Almacen{}, ErrAlmacenesNoDisponible
	}
	sedeID := strings.TrimSpace(a.SedeID)
	esPrimero := len(s.AlmacenesDeSede(empresaID, sedeID)) == 0
	a, err := s.saneaAlmacen(empresaID, "", sedeID, a)
	if err != nil {
		return almacen.Almacen{}, err
	}
	a.ID = ""
	a.EmpresaID = empresaID
	a.Activo = true
	a.Creada = ahora()
	if esPrimero {
		a.Principal = true // una sede siempre tiene su primer almacén como principal
	}
	out := s.almacenes.Create(a)
	if out.Principal {
		s.demoteOtrosPrincipales(empresaID, sedeID, out.ID)
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.almacen.crear", out.ID, out.Nombre))
	return out, nil
}

// ActualizarAlmacen edita un almacén (maestro editable). La SEDE es inmutable. El
// principal no se puede quitar directamente (se cambia marcando OTRO como principal),
// así que siempre queda al menos un principal por sede.
func (s *Service) ActualizarAlmacen(empresaID, id, actor, origen string, a almacen.Almacen, activo *bool) (almacen.Almacen, error) {
	if s.almacenes == nil {
		return almacen.Almacen{}, ErrAlmacenesNoDisponible
	}
	cur, ok := s.almacenes.ByID(empresaID, id)
	if !ok {
		return almacen.Almacen{}, ErrAlmacenNoExiste
	}
	a, err := s.saneaAlmacen(empresaID, id, cur.SedeID, a)
	if err != nil {
		return almacen.Almacen{}, err
	}
	a.ID = id
	a.EmpresaID = empresaID
	a.SedeID = cur.SedeID
	a.Creada = cur.Creada
	if activo != nil {
		a.Activo = *activo
	} else {
		a.Activo = cur.Activo
	}
	if cur.Principal && !a.Principal {
		a.Principal = true // no se desmarca el principal directamente; se marca otro
	}
	out, ok := s.almacenes.Update(a)
	if !ok {
		return almacen.Almacen{}, ErrAlmacenNoExiste
	}
	if out.Principal {
		s.demoteOtrosPrincipales(empresaID, cur.SedeID, out.ID)
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.almacen.actualizar", out.ID, out.Nombre))
	return out, nil
}

// DesactivarAlmacen hace soft-disable. No se puede desactivar el principal (hay que
// marcar otro primero) ni el último almacén activo de la sede.
func (s *Service) DesactivarAlmacen(empresaID, id, actor, origen string) error {
	if s.almacenes == nil {
		return ErrAlmacenesNoDisponible
	}
	a, ok := s.almacenes.ByID(empresaID, id)
	if !ok {
		return ErrAlmacenNoExiste
	}
	if a.Principal {
		return ErrAlmacenPrincipal
	}
	activos := 0
	for _, x := range s.AlmacenesDeSede(empresaID, a.SedeID) {
		if x.Activo {
			activos++
		}
	}
	if activos <= 1 {
		return ErrUltimoAlmacenActivo
	}
	a.Activo = false
	if _, ok := s.almacenes.Update(a); !ok {
		return ErrAlmacenNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.almacen.desactivar", id, a.Nombre))
	return nil
}

// AsegurarAlmacenPrincipal garantiza que la sede tenga un almacén principal activo.
// Idempotente: si ya lo tiene, no hace nada; si tiene almacenes pero ninguno
// principal, promueve el primero; si no tiene ninguno, crea "Almacén Principal".
// Lo usan la creación de sede y el backfill de arranque para el invariante "≥1
// almacén por sede".
func (s *Service) AsegurarAlmacenPrincipal(empresaID, sedeID, actor, origen string) (almacen.Almacen, error) {
	if s.almacenes == nil {
		return almacen.Almacen{}, ErrAlmacenesNoDisponible
	}
	if strings.TrimSpace(sedeID) == "" {
		return almacen.Almacen{}, ErrAlmacenSedeRequerida
	}
	if a, ok := s.AlmacenPrincipalDe(empresaID, sedeID); ok {
		return a, nil
	}
	if existentes := s.AlmacenesDeSede(empresaID, sedeID); len(existentes) > 0 {
		a := existentes[0]
		a.Principal, a.Activo = true, true
		out, _ := s.almacenes.Update(a)
		s.demoteOtrosPrincipales(empresaID, sedeID, out.ID)
		return out, nil
	}
	out := s.almacenes.Create(almacen.Almacen{
		EmpresaID: empresaID, SedeID: sedeID, Nombre: "Almacén Principal",
		Tipo: almacen.TipoPrincipal, Principal: true, Activo: true, Creada: ahora(),
	})
	s.audit.Append(evento(empresaID, actor, origen, "config.almacen.principal_auto", out.ID, sedeID))
	return out, nil
}
