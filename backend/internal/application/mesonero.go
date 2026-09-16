package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mornix/elerp/internal/domain/cuenta"
	"github.com/mornix/elerp/internal/domain/mesonero"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// Turnos del salón (módulo Restaurante). Ver domain/mesonero para el porqué.
//
// Las dos reglas que este archivo hace cumplir, y que NO son de la interfaz:
//
//  1. Un turno lo abre un SUPERVISOR con su PIN. Sin turno vivo, el PIN del
//     mesonero no habilita nada: por eso no se puede entrar a trabajar de
//     madrugada ni desde la casa «porque me sé mi clave».
//  2. El turno se apaga en dos tiempos (cerrando → cerrado). En cerrando ya no
//     toma mesas nuevas pero atiende las suyas, y se cierra SOLO al cerrarse la
//     última. Un turno no termina porque alguien toque un botón: termina cuando
//     se va el último cliente.

var (
	ErrMesonerosNoDisponible = errors.New("los turnos del salón no están disponibles")
	ErrMesoneroNoExiste      = errors.New("el mesonero no existe")
	ErrMesoneroInactivo      = errors.New("el mesonero está inactivo")
	ErrMesoneroSinPin        = errors.New("este mesonero todavía no tiene PIN: hay que fijarlo con autorización del supervisor")
	ErrMesoneroYaTienePin    = errors.New("este mesonero ya fijó su PIN: para cambiarlo hay que reiniciarlo")
	ErrPinMesoneroInvalido   = errors.New("PIN incorrecto")
	ErrUsuarioYaEsMesonero   = errors.New("ese usuario ya tiene una credencial de mesonero")
	ErrTurnoYaAbierto        = errors.New("este mesonero ya tiene un turno en curso")
	ErrTurnoNoExiste         = errors.New("el turno no existe")
	ErrTurnoNoVivo           = errors.New("el turno ya está cerrado")
	ErrRelevoRequerido       = errors.New("el turno tiene mesas abiertas: hay que pasárselas a otro mesonero")
	ErrRelevoInvalido        = errors.New("el mesonero de relevo debe tener un turno abierto en la misma sede")
	ErrSinTurnoAbierto       = errors.New("no tienes un turno abierto: pídele a un supervisor que te lo valide")
	ErrTurnoCerrandoSinMesas = errors.New("tu turno está cerrando: ya no puedes tomar mesas nuevas")
)

// ConMesoneros cablea las credenciales de mesonero y sus turnos. Opcional, como
// el resto del módulo Restaurante: sin cablear, el salón sigue funcionando
// exactamente como antes (sin turnos y sin el candado que exige tenerlos).
func (s *Service) ConMesoneros(m mesonero.Repository, t mesonero.TurnoRepository) *Service {
	s.mesoneros = m
	s.turnos = t
	return s
}

/* --- Credenciales --------------------------------------------------------- */

// MesoneroView es la ficha como la pinta la grilla del salón: la credencial más
// lo que la pantalla necesita para decidir qué ofrecer — si ya tiene PIN, si
// está en turno y cuántas mesas lleva abiertas.
type MesoneroView struct {
	mesonero.Mesonero
	// TienePin en claro para la interfaz: el hash nunca se serializa.
	TienePin bool `json:"tienePin"`
	// Turno es el turno vivo, si lo tiene. Nil = no está trabajando.
	Turno *mesonero.Turno `json:"turno,omitempty"`
	// MesasAbiertas son las cuentas que atiende ahora mismo.
	MesasAbiertas int `json:"mesasAbiertas"`
}

// ListarMesoneros devuelve las credenciales de una sede con su estado de turno.
// Sede vacía = toda la empresa.
func (s *Service) ListarMesoneros(empresaID, sedeID string) []MesoneroView {
	if s.mesoneros == nil {
		return []MesoneroView{}
	}
	abiertas := s.cuentasAbiertasPorUsuario(empresaID, sedeID)
	out := []MesoneroView{}
	for _, m := range s.mesoneros.List(empresaID) {
		if sedeID != "" && m.SedeID != sedeID {
			continue
		}
		v := MesoneroView{Mesonero: m, TienePin: m.TienePin(), MesasAbiertas: len(abiertas[m.UsuarioID])}
		if s.turnos != nil {
			if t, ok := s.turnos.VivoDeMesonero(empresaID, m.ID); ok {
				v.Turno = &t
			}
		}
		out = append(out, v)
	}
	return out
}

// cuentasAbiertasPorUsuario agrupa las cuentas abiertas de la sede por el
// usuario que las atiende AHORA (cuenta.MesoneroID). Es «quién tiene mesa
// encima», la pregunta que manda tanto en el cierre suave como en el relevo.
func (s *Service) cuentasAbiertasPorUsuario(empresaID, sedeID string) map[string][]cuenta.Cuenta {
	out := map[string][]cuenta.Cuenta{}
	if s.cuentasMesa == nil {
		return out
	}
	for _, c := range s.cuentasMesa.Abiertas(empresaID, sedeID) {
		if c.MesoneroID != "" {
			out[c.MesoneroID] = append(out[c.MesoneroID], c)
		}
	}
	return out
}

// siguienteCodigoMesonero numera la serie MS-. Prefijo propio, distinto del OP-
// de los cajeros y del C- de las cajas: en la auditoría y en el reparto de mesas
// los tres aparecen juntos y confundirlos sale caro.
func (s *Service) siguienteCodigoMesonero(empresaID string) string {
	max := 0
	for _, m := range s.mesoneros.List(empresaID) {
		var n int
		if _, err := fmt.Sscanf(m.Codigo, "MS-%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("MS-%03d", max+1)
}

// CrearMesonero da de alta la credencial. NO fija el PIN: eso lo hace la propia
// persona después (FijarPinMesonero), con un supervisor autorizando. Así nadie
// más conoce ese PIN — importa porque el turno respalda la comisión.
func (s *Service) CrearMesonero(empresaID, actor, origen, sedeID, nombre, usuarioID string) (mesonero.Mesonero, error) {
	if s.mesoneros == nil {
		return mesonero.Mesonero{}, ErrMesonerosNoDisponible
	}
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return mesonero.Mesonero{}, errors.New("el nombre del mesonero es obligatorio")
	}
	sedeID = strings.TrimSpace(sedeID)
	if sedeID == "" {
		return mesonero.Mesonero{}, ErrSedeRequerida
	}
	usuarioID = strings.TrimSpace(usuarioID)
	if usuarioID == "" {
		return mesonero.Mesonero{}, errors.New("hay que indicar a qué usuario pertenece la credencial")
	}
	// Una persona, una credencial: dos credenciales sobre el mismo usuario
	// abrirían dos turnos a la vez y partirían sus mesas entre ambos.
	if _, existe := s.mesoneros.ByUsuario(empresaID, usuarioID); existe {
		return mesonero.Mesonero{}, ErrUsuarioYaEsMesonero
	}
	out := s.mesoneros.Create(mesonero.Mesonero{
		EmpresaID: empresaID, SedeID: sedeID, Codigo: s.siguienteCodigoMesonero(empresaID),
		Nombre: nombre, UsuarioID: usuarioID, Activo: true, Creado: ahora(),
	})
	s.audit.Append(evento(empresaID, actor, origen, "salon.mesonero.crear", out.Codigo, nombre))
	return out, nil
}

// CambiosMesonero describe una edición parcial. Punteros nil = no cambia.
type CambiosMesonero struct {
	Nombre *string
	SedeID *string
	Activo *bool
}

// ActualizarMesonero edita nombre, sede y activación. NO cambia el código (que
// la persona ya memorizó) ni el PIN (que se reinicia aparte, con autorización).
func (s *Service) ActualizarMesonero(empresaID, actor, origen, id string, c CambiosMesonero) (mesonero.Mesonero, error) {
	if s.mesoneros == nil {
		return mesonero.Mesonero{}, ErrMesonerosNoDisponible
	}
	m, ok := s.mesoneros.ByID(empresaID, id)
	if !ok {
		return mesonero.Mesonero{}, ErrMesoneroNoExiste
	}
	if c.Nombre != nil {
		n := strings.TrimSpace(*c.Nombre)
		if n == "" {
			return mesonero.Mesonero{}, errors.New("el nombre del mesonero es obligatorio")
		}
		m.Nombre = n
	}
	if c.SedeID != nil {
		sede := strings.TrimSpace(*c.SedeID)
		if sede == "" {
			return mesonero.Mesonero{}, ErrSedeRequerida
		}
		m.SedeID = sede
	}
	if c.Activo != nil {
		m.Activo = *c.Activo
	}
	out, ok := s.mesoneros.Update(m)
	if !ok {
		return mesonero.Mesonero{}, ErrMesoneroNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "salon.mesonero.actualizar", out.Codigo, out.Nombre))
	return out, nil
}

// FijarPinMesonero deja que la persona elija su PIN, con un supervisor
// autorizando con el suyo. Es el reemplazo del enlace por correo: acá la prueba
// de que quien fija el PIN es la persona correcta NO es un canal privado, es que
// hay un supervisor parado al lado. Por eso la autorización es obligatoria y
// queda auditada — la presencia física convence en el momento pero no deja
// rastro; el registro sí.
//
// `reiniciar` distingue fijarlo por primera vez de reemplazar uno olvidado: sin
// esa marca, quien agarre la tablet podría pisarle el PIN a otro.
func (s *Service) FijarPinMesonero(empresaID, actor, origen, mesoneroID, pin, pinSupervisor string, reiniciar bool) error {
	if s.mesoneros == nil {
		return ErrMesonerosNoDisponible
	}
	m, ok := s.mesoneros.ByID(empresaID, mesoneroID)
	if !ok {
		return ErrMesoneroNoExiste
	}
	if !m.Activo {
		return ErrMesoneroInactivo
	}
	if m.TienePin() && !reiniciar {
		return ErrMesoneroYaTienePin
	}
	if !pinValido(pin) {
		return ErrPinDebil
	}
	accion := "salon.pin.fijar"
	if reiniciar {
		accion = "salon.pin.reiniciar"
	}
	if _, err := s.AutorizarSupervisor(empresaID, actor, origen, accion+":"+m.Codigo, pinSupervisor); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	m.PinHash = string(hash)
	if _, ok := s.mesoneros.Update(m); !ok {
		return ErrMesoneroNoExiste
	}
	// Nunca se audita el PIN ni su hash: solo que se fijó y para quién.
	s.audit.Append(evento(empresaID, actor, origen, accion, m.Codigo, m.Nombre))
	return nil
}

/* --- Turnos --------------------------------------------------------------- */

// IniciarTurno abre el turno de un mesonero. Exige las dos cosas a la vez: el
// PIN de la persona (quién es) y el de un supervisor (que está autorizada a
// trabajar ahora). El PIN solo nunca alcanza — esa es toda la idea.
func (s *Service) IniciarTurno(empresaID, actor, origen, mesoneroID, pin, pinSupervisor string) (mesonero.Turno, error) {
	if s.mesoneros == nil || s.turnos == nil {
		return mesonero.Turno{}, ErrMesonerosNoDisponible
	}
	m, ok := s.mesoneros.ByID(empresaID, mesoneroID)
	if !ok {
		return mesonero.Turno{}, ErrMesoneroNoExiste
	}
	if !m.Activo {
		return mesonero.Turno{}, ErrMesoneroInactivo
	}
	if !m.TienePin() {
		return mesonero.Turno{}, ErrMesoneroSinPin
	}
	// Un turno vivo por persona: dos turnos a la vez partirían sus mesas y su
	// resumen en dos, y ninguno de los dos sería cierto.
	if _, ya := s.turnos.VivoDeMesonero(empresaID, m.ID); ya {
		return mesonero.Turno{}, ErrTurnoYaAbierto
	}
	if bcrypt.CompareHashAndPassword([]byte(m.PinHash), []byte(pin)) != nil {
		s.audit.Append(evento(empresaID, actor, origen, "salon.turno.pin.fallido", m.Codigo, m.Nombre))
		return mesonero.Turno{}, ErrPinMesoneroInvalido
	}
	// La autorización va DESPUÉS del PIN del mesonero a propósito: si fuera al
	// revés, alguien podría probar PINes de supervisor sin siquiera tener una
	// credencial válida de salón.
	supervisor, err := s.AutorizarSupervisor(empresaID, actor, origen, "salon.turno.iniciar:"+m.Codigo, pinSupervisor)
	if err != nil {
		return mesonero.Turno{}, err
	}
	out := s.turnos.Create(mesonero.Turno{
		EmpresaID: empresaID, SedeID: m.SedeID,
		MesoneroID: m.ID, MesoneroCodigo: m.Codigo, MesoneroNombre: m.Nombre, UsuarioID: m.UsuarioID,
		Estado: mesonero.EstadoAbierto, Apertura: ahora(), ValidadoPor: supervisor,
	})
	s.audit.Append(evento(empresaID, actor, origen, "salon.turno.iniciar", out.ID, m.Codigo+" "+m.Nombre+" · validó "+supervisor))
	return out, nil
}

// FinalizarTurno ordena el fin del turno. NO lo apaga de golpe: si al mesonero
// le quedan mesas abiertas, el turno pasa a CERRANDO — ya no toma mesas nuevas,
// pero atiende las suyas hasta cerrarlas, y ahí se cierra solo. Si no le queda
// ninguna, se cierra en el acto.
func (s *Service) FinalizarTurno(empresaID, actor, origen, turnoID string) (mesonero.Turno, error) {
	if s.turnos == nil {
		return mesonero.Turno{}, ErrMesonerosNoDisponible
	}
	t, ok := s.turnos.ByID(empresaID, turnoID)
	if !ok {
		return mesonero.Turno{}, ErrTurnoNoExiste
	}
	if !t.Vivo() {
		return mesonero.Turno{}, ErrTurnoNoVivo
	}
	pendientes := s.cuentasAbiertasPorUsuario(empresaID, t.SedeID)[t.UsuarioID]
	if len(pendientes) == 0 {
		return s.cerrarTurno(empresaID, actor, origen, t, "")
	}
	if t.Estado == mesonero.EstadoAbierto {
		t.Estado = mesonero.EstadoCerrando
		t.CerrandoDesde = ahora()
		out, ok := s.turnos.Update(t)
		if !ok {
			return mesonero.Turno{}, ErrTurnoNoExiste
		}
		s.audit.Append(evento(empresaID, actor, origen, "salon.turno.cerrando", out.ID,
			fmt.Sprintf("%s · le quedan %d mesa(s)", t.MesoneroNombre, len(pendientes))))
		return out, nil
	}
	// Ya venía en cerrando y todavía tiene mesas: no hay nada que cambiar.
	return t, nil
}

// CerrarTurnoForzado cierra un turno YA, sin esperar a que se vacíen sus mesas.
// Es el caso real del mesonero que se enferma y se va con mesas encima: alguien
// tiene que heredarlas o esas cuentas quedan sin quién las cobre. Por eso, si
// quedan mesas abiertas, el relevo es OBLIGATORIO.
//
// Las mesas cambian de responsable (MesoneroID), pero conservan su TurnoID: el
// sello de quién las abrió no se mueve, igual que la sesión de caja sellada en un
// documento fiscal. Así el resumen de cada turno sigue contando lo que esa
// persona realmente atendió.
func (s *Service) CerrarTurnoForzado(empresaID, actor, origen, turnoID, relevoMesoneroID, pinSupervisor string) (mesonero.Turno, error) {
	if s.turnos == nil || s.mesoneros == nil {
		return mesonero.Turno{}, ErrMesonerosNoDisponible
	}
	t, ok := s.turnos.ByID(empresaID, turnoID)
	if !ok {
		return mesonero.Turno{}, ErrTurnoNoExiste
	}
	if !t.Vivo() {
		return mesonero.Turno{}, ErrTurnoNoVivo
	}
	pendientes := s.cuentasAbiertasPorUsuario(empresaID, t.SedeID)[t.UsuarioID]
	var relevo mesonero.Mesonero
	if len(pendientes) > 0 {
		if strings.TrimSpace(relevoMesoneroID) == "" {
			return mesonero.Turno{}, ErrRelevoRequerido
		}
		r, ok := s.mesoneros.ByID(empresaID, relevoMesoneroID)
		if !ok || r.ID == t.MesoneroID || r.SedeID != t.SedeID {
			return mesonero.Turno{}, ErrRelevoInvalido
		}
		// El relevo tiene que poder TOMAR mesas: pasárselas a alguien que también
		// se está yendo solo mueve el problema de lugar.
		rt, ok := s.turnos.VivoDeMesonero(empresaID, r.ID)
		if !ok || !rt.PuedeTomarMesas() {
			return mesonero.Turno{}, ErrRelevoInvalido
		}
		relevo = r
	}
	supervisor, err := s.AutorizarSupervisor(empresaID, actor, origen, "salon.turno.forzar:"+t.MesoneroCodigo, pinSupervisor)
	if err != nil {
		return mesonero.Turno{}, err
	}
	for _, c := range pendientes {
		c.MesoneroID, c.MesoneroNombre = relevo.UsuarioID, relevo.Nombre
		if _, ok := s.cuentasMesa.Update(c); ok {
			s.audit.Append(evento(empresaID, actor, origen, "salon.mesa.relevo", c.ID,
				fmt.Sprintf("mesa %s: %s → %s", c.MesaNombre, t.MesoneroNombre, relevo.Nombre)))
		}
	}
	return s.cerrarTurno(empresaID, actor, origen, t, supervisor)
}

// cerrarTurno pasa el turno a cerrado y CONGELA su resumen. Una vez cerrado, esa
// foto no se recalcula: si mañana una mesa se reasigna, lo de anoche no cambia
// (misma regla que el arqueo de caja).
func (s *Service) cerrarTurno(empresaID, actor, origen string, t mesonero.Turno, cerradoPor string) (mesonero.Turno, error) {
	cierre := ahora()
	r := s.resumenDeTurno(empresaID, t, cierre)
	t.Estado = mesonero.EstadoCerrado
	t.Cierre = cierre
	t.CerradoPor = cerradoPor
	t.Resumen = &r
	out, ok := s.turnos.Update(t)
	if !ok {
		return mesonero.Turno{}, ErrTurnoNoExiste
	}
	detalle := fmt.Sprintf("%s · %d mesa(s), %d persona(s), %d orden(es)", t.MesoneroNombre, r.Mesas, r.Personas, r.Ordenes)
	if cerradoPor != "" {
		detalle += " · forzado por " + cerradoPor
	}
	s.audit.Append(evento(empresaID, actor, origen, "salon.turno.cerrar", out.ID, detalle))
	return out, nil
}

// resumenDeTurno pliega las cuentas DEL TURNO (por su sello TurnoID). Las cifras
// se derivan del pedido, nunca de contadores: es el mismo criterio del arqueo,
// del Kardex y del balance.
func (s *Service) resumenDeTurno(empresaID string, t mesonero.Turno, cierre string) mesonero.Resumen {
	r := mesonero.Resumen{MinutosTrabajados: minutosEntre(t.Apertura, cierre)}
	if s.cuentasMesa == nil {
		return r
	}
	for _, c := range s.cuentasMesa.DeTurno(empresaID, t.ID) {
		// Una mesa anulada no se atendió: contarla inflaría las mesas y hundiría
		// el ticket promedio.
		if c.Estado == cuenta.EstadoAnulada {
			continue
		}
		r.Mesas++
		r.Personas += c.Comensales
		// UltimaRonda es cuántas veces mandó comanda a cocina por esa mesa: eso
		// son las órdenes que atendió.
		r.Ordenes += c.UltimaRonda
		r.TotalFacturado += c.Total()
	}
	r.TotalFacturado = round2(r.TotalFacturado)
	if r.Mesas > 0 {
		r.TicketPromedio = round2(r.TotalFacturado / float64(r.Mesas))
	}
	return r
}

// minutosEntre mide el turno. Ante una fecha ilegible devuelve 0 en vez de una
// cifra inventada: un resumen con un número falso es peor que uno en blanco.
func minutosEntre(desde, hasta string) int {
	a, err1 := time.Parse(time.RFC3339, desde)
	b, err2 := time.Parse(time.RFC3339, hasta)
	if err1 != nil || err2 != nil || b.Before(a) {
		return 0
	}
	return int(b.Sub(a).Minutes())
}

// Relevo es un candidato a heredar las mesas de un turno que se cierra.
type Relevo struct {
	MesoneroID string `json:"mesoneroId"`
	Codigo     string `json:"codigo"`
	Nombre     string `json:"nombre"`
	// MesasAbiertas es por lo que se ordena: la sugerencia es siempre quien
	// menos carga tiene encima.
	MesasAbiertas int `json:"mesasAbiertas"`
	// Sugerido marca al primero, para que la pantalla lo destaque. Es una
	// sugerencia: quien decide es el supervisor, que sabe cosas que el sistema no
	// (quién está por irse, quién conoce esa zona).
	Sugerido bool `json:"sugerido"`
}

// CandidatosRelevo lista a quién se le pueden pasar las mesas de un turno, del
// menos cargado al más cargado. Solo entran turnos ABIERTOS: alguien en cerrando
// tampoco puede tomar mesas nuevas.
func (s *Service) CandidatosRelevo(empresaID, turnoID string) []Relevo {
	out := []Relevo{}
	if s.turnos == nil || s.mesoneros == nil {
		return out
	}
	t, ok := s.turnos.ByID(empresaID, turnoID)
	if !ok {
		return out
	}
	abiertas := s.cuentasAbiertasPorUsuario(empresaID, t.SedeID)
	for _, otro := range s.turnos.Vivos(empresaID, t.SedeID) {
		if otro.ID == t.ID || !otro.PuedeTomarMesas() {
			continue
		}
		out = append(out, Relevo{
			MesoneroID: otro.MesoneroID, Codigo: otro.MesoneroCodigo, Nombre: otro.MesoneroNombre,
			MesasAbiertas: len(abiertas[otro.UsuarioID]),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MesasAbiertas != out[j].MesasAbiertas {
			return out[i].MesasAbiertas < out[j].MesasAbiertas
		}
		return out[i].Nombre < out[j].Nombre
	})
	if len(out) > 0 {
		out[0].Sugerido = true
	}
	return out
}

// TurnosVivos son los turnos en pie de una sede: quién está trabajando ahora.
func (s *Service) TurnosVivos(empresaID, sedeID string) []mesonero.Turno {
	if s.turnos == nil {
		return []mesonero.Turno{}
	}
	return s.turnos.Vivos(empresaID, sedeID)
}

// HistorialTurnos son los turnos de una sede, del más reciente al más viejo, con
// su resumen congelado. Es la base de «quién trabajó anoche» y de la comisión.
func (s *Service) HistorialTurnos(empresaID, sedeID string) []mesonero.Turno {
	if s.turnos == nil {
		return []mesonero.Turno{}
	}
	return s.turnos.Historico(empresaID, sedeID)
}

/* --- Enganches con el salón ------------------------------------------------ */

// turnoVivoDeUsuario resuelve el turno en pie de un usuario de la app. Devuelve
// (turno, true) solo si la persona tiene credencial de mesonero Y turno vivo.
func (s *Service) turnoVivoDeUsuario(empresaID, usuarioID string) (mesonero.Turno, bool) {
	if s.mesoneros == nil || s.turnos == nil || usuarioID == "" {
		return mesonero.Turno{}, false
	}
	m, ok := s.mesoneros.ByUsuario(empresaID, usuarioID)
	if !ok {
		return mesonero.Turno{}, false
	}
	return s.turnos.VivoDeMesonero(empresaID, m.ID)
}

// exigirTurnoParaAtender es el candado del salón: un MESONERO solo atiende si
// tiene turno vivo. Es lo que hace que el PIN por sí solo no sirva de madrugada
// ni desde la casa.
//
// `propia` distingue las dos mitades del cierre suave: seguir atendiendo una
// mesa QUE YA ES SUYA vale también en cerrando —es justamente lo que cerrando
// significa—; tomar una mesa NUEVA, no.
//
// Solo aplica al rol mesonero y solo cuando los turnos están cableados: la dueña
// y el cajero atienden sin turno, y una empresa que no usa turnos sigue
// funcionando igual que antes.
func (s *Service) exigirTurnoParaAtender(empresaID, usuarioID, rolActor string, propia bool) error {
	if s.turnos == nil || rolActor != usuario.RolMesonero {
		return nil
	}
	t, ok := s.turnoVivoDeUsuario(empresaID, usuarioID)
	if !ok {
		return ErrSinTurnoAbierto
	}
	if propia {
		return nil
	}
	if !t.PuedeTomarMesas() {
		return ErrTurnoCerrandoSinMesas
	}
	return nil
}

// cerrarTurnoSiSeVacio cierra SOLO el turno que estaba en cerrando y acaba de
// quedarse sin mesas. Es la segunda mitad del cierre suave: el turno termina
// cuando se va el último cliente, no cuando alguien toca un botón.
//
// Se llama al cerrar una cuenta. Silencioso por diseño: que no haya turno, o que
// no estuviera cerrando, es lo normal y no es un error.
func (s *Service) cerrarTurnoSiSeVacio(empresaID, usuarioID, actor, origen string) {
	t, ok := s.turnoVivoDeUsuario(empresaID, usuarioID)
	if !ok || t.Estado != mesonero.EstadoCerrando {
		return
	}
	if len(s.cuentasAbiertasPorUsuario(empresaID, t.SedeID)[usuarioID]) > 0 {
		return
	}
	s.cerrarTurno(empresaID, actor, origen, t, "")
}
