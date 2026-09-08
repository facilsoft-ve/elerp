package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/mornix/elerp/internal/domain/caja"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// Errores de negocio del módulo de cajas. Se traducen a HTTP en el adaptador.
var (
	ErrCajaNoExiste          = errors.New("la caja no existe")
	ErrCajaDeshabilitada     = errors.New("la caja está deshabilitada")
	ErrCajaTomada            = errors.New("otro cajero ya tiene esta caja abierta")
	ErrCajeroInvalido        = errors.New("código de cajero o PIN incorrecto")
	ErrCajeroOtraSede        = errors.New("el cajero no pertenece a la sede de esta caja")
	ErrSinCajaAbierta        = errors.New("no hay una caja abierta a tu nombre")
	ErrCajaOtraSede          = errors.New("tu caja abierta pertenece a otra sede")
	ErrSesionNoExiste        = errors.New("la sesión de caja no existe")
	ErrPinDebil              = errors.New("el PIN debe tener 4 dígitos")
	ErrCodigoCajeroEnUso     = errors.New("ya existe un cajero con ese código en la empresa")
	ErrCajeroNoExiste        = errors.New("el cajero no existe")
	ErrPinSupervisorInvalido = errors.New("PIN de supervisor incorrecto")
	ErrSinSupervisores       = errors.New("la empresa no tiene ningún cajero marcado como supervisor")
	// ErrDispositivoNoEsImpresora: una caja solo puede tener asignada una IMPRESORA
	// fiscal como su dispositivo. Una balanza (u otro tipo) mide/pesa, no emite el
	// documento fiscal de la caja; asignarla como impresora sería un error de
	// configuración que rompería la emisión en el mostrador.
	ErrDispositivoNoEsImpresora = errors.New("el dispositivo asignado a la caja debe ser una impresora fiscal")
)

// CajaView es una caja con el estado de su turno, para pintar el modal de
// apertura sin que el cliente tenga que cruzar dos listas.
type CajaView struct {
	caja.Caja
	// Ocupada y OcupadaPor describen el turno vigente, si lo hay.
	Ocupada    bool   `json:"ocupada"`
	OcupadaPor string `json:"ocupadaPor"`
	// Propia marca la caja cuyo turno abierto pertenece a quien consulta: la
	// interfaz la ofrece como «tu turno abierto · retomar».
	Propia bool `json:"propia"`
}

// ListarCajas devuelve las cajas de la empresa (opcionalmente de una sede) con
// el estado de ocupación resuelto.
func (s *Service) ListarCajas(empresaID, sedeID, actorID string) []CajaView {
	abiertas := s.sesiones.Abiertas(empresaID, sedeID)
	porCaja := map[string]caja.Sesion{}
	for _, ses := range abiertas {
		porCaja[ses.CajaID] = ses
	}
	out := []CajaView{}
	for _, c := range s.cajas.List(empresaID) {
		if sedeID != "" && c.SedeID != sedeID {
			continue
		}
		v := CajaView{Caja: c}
		if ses, ok := porCaja[c.ID]; ok {
			v.Ocupada = true
			v.OcupadaPor = ses.CajeroNombre
			v.Propia = ses.ActorID == actorID
		}
		out = append(out, v)
	}
	return out
}

// CrearCaja registra una caja nueva. Nace DESHABILITADA a propósito: el
// administrador la habilita cuando el puesto está realmente operativo, y el
// código lo genera el servidor (C-001, C-002…) por empresa.
//
// dispositivoFiscalID es opcional ("" = sin dispositivo): si viene, debe ser un
// dispositivo fiscal de la MISMA empresa — nunca se confía en el cliente.
func (s *Service) CrearCaja(empresaID, sedeID, actor, origen, nombre, dispositivoFiscalID string) (caja.Caja, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return caja.Caja{}, errors.New("el nombre de la caja es obligatorio")
	}
	if sedeID == "" {
		return caja.Caja{}, errors.New("la caja debe pertenecer a una sede")
	}
	dispositivoFiscalID = strings.TrimSpace(dispositivoFiscalID)
	if err := s.validarDispositivoDeCaja(empresaID, dispositivoFiscalID); err != nil {
		return caja.Caja{}, err
	}
	c := caja.Caja{
		EmpresaID:           empresaID,
		SedeID:              sedeID,
		Nombre:              nombre,
		Codigo:              s.siguienteCodigoCaja(empresaID),
		Estado:              caja.EstadoDeshabilitada,
		DispositivoFiscalID: dispositivoFiscalID,
		Creada:              ahora(),
	}
	out := s.cajas.Create(c)
	s.audit.Append(evento(empresaID, actor, origen, "caja.crear", out.Codigo, nombre))
	return out, nil
}

// validarDispositivoDeCaja comprueba que un dispositivo fiscal asignado a una
// caja exista dentro del tenant Y sea una IMPRESORA fiscal: la caja imprime sus
// documentos, así que una balanza (u otro tipo) no es asignable como su
// dispositivo. La cadena vacía es válida (sin dispositivo).
func (s *Service) validarDispositivoDeCaja(empresaID, dispositivoFiscalID string) error {
	if dispositivoFiscalID == "" {
		return nil
	}
	d, ok := s.dispositivos.ByID(empresaID, dispositivoFiscalID)
	if !ok {
		return ErrDispositivoNoExiste
	}
	if d.Tipo != fiscal.DispositivoImpresoraFiscal {
		return ErrDispositivoNoEsImpresora
	}
	return nil
}

// CambiosCaja describe una edición parcial de una caja. Los punteros nil son
// campos que no cambian: así se distingue «no tocar» de «poner en vacío».
type CambiosCaja struct {
	Nombre              *string
	SedeID              *string
	DispositivoFiscalID *string
	// Activa habilita (true) o deshabilita (false) la caja sin borrarla.
	Activa *bool
	// ColorFondo y LogoVersion son el OVERRIDE de branding por caja de la pantalla
	// del cliente: color de fondo (hex; se normaliza) y qué versión del logo usar
	// («principal»|«alterno»). Cadena vacía = volver al default de la empresa.
	ColorFondo  *string
	LogoVersion *string
}

// ActualizarCaja edita una caja existente (nombre, sede, dispositivo fiscal) y
// permite habilitarla/deshabilitarla sin borrarla. Deshabilitar con un turno
// abierto se rechaza: primero hay que cerrar el turno para no perder el arqueo.
func (s *Service) ActualizarCaja(empresaID, actor, origen, id string, cambios CambiosCaja) (caja.Caja, error) {
	c, ok := s.cajas.ByID(empresaID, id)
	if !ok {
		return caja.Caja{}, ErrCajaNoExiste
	}
	if cambios.Nombre != nil {
		nombre := strings.TrimSpace(*cambios.Nombre)
		if nombre == "" {
			return caja.Caja{}, errors.New("el nombre de la caja es obligatorio")
		}
		c.Nombre = nombre
	}
	if cambios.SedeID != nil {
		if *cambios.SedeID == "" {
			return caja.Caja{}, errors.New("la caja debe pertenecer a una sede")
		}
		// Mover una caja de sede con un turno abierto dejaría el arqueo en la
		// sede equivocada: se exige cerrar primero.
		if *cambios.SedeID != c.SedeID {
			if _, abierta := s.sesiones.AbiertaDeCaja(empresaID, id); abierta {
				return caja.Caja{}, errors.New("cierra el turno abierto antes de cambiar la caja de sede")
			}
		}
		c.SedeID = *cambios.SedeID
	}
	if cambios.DispositivoFiscalID != nil {
		disp := strings.TrimSpace(*cambios.DispositivoFiscalID)
		if err := s.validarDispositivoDeCaja(empresaID, disp); err != nil {
			return caja.Caja{}, err
		}
		c.DispositivoFiscalID = disp
	}
	if cambios.Activa != nil {
		nuevoEstado := caja.EstadoDeshabilitada
		if *cambios.Activa {
			nuevoEstado = caja.EstadoHabilitada
		}
		if nuevoEstado == caja.EstadoDeshabilitada && c.Estado != caja.EstadoDeshabilitada {
			if _, abierta := s.sesiones.AbiertaDeCaja(empresaID, id); abierta {
				return caja.Caja{}, errors.New("cierra el turno abierto antes de deshabilitar la caja")
			}
		}
		c.Estado = nuevoEstado
	}
	if cambios.ColorFondo != nil {
		c.ColorFondo = empresa.NormalizarColorHex(*cambios.ColorFondo)
	}
	if cambios.LogoVersion != nil {
		c.LogoVersion = caja.NormalizarLogoVersion(*cambios.LogoVersion)
	}
	out, ok := s.cajas.Update(c)
	if !ok {
		return caja.Caja{}, ErrCajaNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "caja.actualizar", out.Codigo, out.Nombre))
	return out, nil
}

// siguienteCodigoCaja numera las cajas por empresa: C-001, C-002…
func (s *Service) siguienteCodigoCaja(empresaID string) string {
	max := 0
	for _, c := range s.cajas.List(empresaID) {
		var n int
		if _, err := fmt.Sscanf(c.Codigo, "C-%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("C-%03d", max+1)
}

// CambiarEstadoCaja habilita o deshabilita una caja. Deshabilitar no cierra el
// turno en curso: primero se fuerza el cierre, para no perder el arqueo.
func (s *Service) CambiarEstadoCaja(empresaID, actor, origen, cajaID, estado string) (caja.Caja, error) {
	if estado != caja.EstadoHabilitada && estado != caja.EstadoDeshabilitada {
		return caja.Caja{}, errors.New("estado de caja inválido")
	}
	c, ok := s.cajas.ByID(empresaID, cajaID)
	if !ok {
		return caja.Caja{}, ErrCajaNoExiste
	}
	if estado == caja.EstadoDeshabilitada {
		if _, abierta := s.sesiones.AbiertaDeCaja(empresaID, cajaID); abierta {
			return caja.Caja{}, errors.New("cierra el turno abierto antes de deshabilitar la caja")
		}
	}
	c.Estado = estado
	out, _ := s.cajas.Update(c)
	s.audit.Append(evento(empresaID, actor, origen, "caja.estado", c.Codigo, estado))
	return out, nil
}

// CrearCajero registra una credencial de puesto. El PIN se guarda con bcrypt.
//
// codigo es opcional: si viene vacío lo genera el servidor (C-001, C-002…); si
// el administrador prefiere uno memorable, se normaliza en mayúsculas y debe ser
// único por empresa (es lo que el cajero teclea para abrir su turno).
func (s *Service) CrearCajero(empresaID, actor, origen, sedeID, nombre, codigo, pin, usuarioID string, supervisor bool) (caja.Cajero, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return caja.Cajero{}, errors.New("el nombre del cajero es obligatorio")
	}
	if !pinValido(pin) {
		return caja.Cajero{}, ErrPinDebil
	}
	codigo = strings.ToUpper(strings.TrimSpace(codigo))
	if codigo == "" {
		codigo = s.siguienteCodigoCajero(empresaID)
	}
	if _, existe := s.cajeros.ByCodigo(empresaID, codigo); existe {
		return caja.Cajero{}, ErrCodigoCajeroEnUso
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return caja.Cajero{}, err
	}
	out := s.cajeros.Create(caja.Cajero{
		EmpresaID: empresaID, SedeID: sedeID, Codigo: codigo, Nombre: nombre,
		PinHash: string(hash), UsuarioID: usuarioID, Supervisor: supervisor, Activo: true,
	})
	// Nunca se audita el PIN, ni su hash.
	s.audit.Append(evento(empresaID, actor, origen, "cajero.crear", out.Codigo, nombre))
	return out, nil
}

// CambiosCajero describe una edición parcial de un cajero. Los punteros nil son
// campos que no cambian. Pin, cuando viene, es un RESET: se re-hashea; el PIN en
// claro nunca se guarda ni se audita.
type CambiosCajero struct {
	Nombre     *string
	SedeID     *string
	Supervisor *bool
	Activo     *bool
	Pin        *string
}

// ActualizarCajero edita una credencial de puesto: nombre, sede, rol supervisor,
// activación y (opcionalmente) reinicio del PIN. No cambia el código, que es lo
// que el cajero ya memorizó para abrir su turno.
func (s *Service) ActualizarCajero(empresaID, actor, origen, id string, cambios CambiosCajero) (caja.Cajero, error) {
	cj, ok := s.cajeroPorID(empresaID, id)
	if !ok {
		return caja.Cajero{}, ErrCajeroNoExiste
	}
	if cambios.Nombre != nil {
		nombre := strings.TrimSpace(*cambios.Nombre)
		if nombre == "" {
			return caja.Cajero{}, errors.New("el nombre del cajero es obligatorio")
		}
		cj.Nombre = nombre
	}
	if cambios.SedeID != nil {
		cj.SedeID = *cambios.SedeID
	}
	if cambios.Supervisor != nil {
		cj.Supervisor = *cambios.Supervisor
	}
	if cambios.Activo != nil {
		cj.Activo = *cambios.Activo
	}
	pinReseteado := false
	if cambios.Pin != nil {
		if !pinValido(*cambios.Pin) {
			return caja.Cajero{}, ErrPinDebil
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*cambios.Pin), bcrypt.DefaultCost)
		if err != nil {
			return caja.Cajero{}, err
		}
		cj.PinHash = string(hash)
		pinReseteado = true
	}
	out, ok := s.cajeros.Update(cj)
	if !ok {
		return caja.Cajero{}, ErrCajeroNoExiste
	}
	s.audit.Append(evento(empresaID, actor, origen, "cajero.actualizar", out.Codigo, out.Nombre))
	if pinReseteado {
		// Se audita el hecho del reinicio, jamás el PIN ni su hash.
		s.audit.Append(evento(empresaID, actor, origen, "cajero.pin_reset", out.Codigo, out.Nombre))
	}
	return out, nil
}

// cajeroPorID busca un cajero del tenant por su ID. El puerto expone ByCodigo
// pero no ByID, así que se resuelve sobre la lista ya aislada por empresa (sin
// tocar la interfaz ni el adaptador de Mongo).
func (s *Service) cajeroPorID(empresaID, id string) (caja.Cajero, bool) {
	for _, cj := range s.cajeros.List(empresaID) {
		if cj.ID == id {
			return cj, true
		}
	}
	return caja.Cajero{}, false
}

func pinValido(pin string) bool {
	if len(pin) != 4 {
		return false
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *Service) siguienteCodigoCajero(empresaID string) string {
	// Los cajeros usan el prefijo OP- (operador), DISTINTO del C- de las cajas:
	// así "OP-001" (cajero) y "C-001" (caja) no se confunden en abrir turno,
	// arqueo ni auditoría, donde ambos aparecen juntos. Se cuenta solo la serie
	// OP-; un cajero antiguo con prefijo C- conserva su código y no altera la
	// numeración nueva (prefijos separados no colisionan).
	max := 0
	for _, c := range s.cajeros.List(empresaID) {
		var n int
		if _, err := fmt.Sscanf(c.Codigo, "OP-%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("OP-%03d", max+1)
}

// ListarCajeros devuelve las credenciales de puesto (sin PIN ni hash: el
// dominio ya marca PinHash con json:"-").
func (s *Service) ListarCajeros(empresaID string) []caja.Cajero {
	return s.cajeros.List(empresaID)
}

// AutorizarSupervisor valida el PIN de un cajero SUPERVISOR para autorizar una
// acción sensible del modo caja (quitar una línea, vaciar el carrito, salir de la
// caja). Deja rastro en la auditoría con la acción autorizada: es el registro que
// pide el flujo 2.4, para poder responder después quién autorizó qué.
//
// No dice qué código de supervisor existe ni cuál acertó: solo si el PIN sirvió.
func (s *Service) AutorizarSupervisor(empresaID, actor, origen, accion, pin string) (string, error) {
	supervisores := 0
	for _, cj := range s.cajeros.List(empresaID) {
		if !cj.Activo || !cj.Supervisor {
			continue
		}
		supervisores++
		if bcrypt.CompareHashAndPassword([]byte(cj.PinHash), []byte(pin)) == nil {
			s.audit.Append(evento(empresaID, actor, origen, "caja.autorizacion", accion, cj.Codigo+" "+cj.Nombre))
			return cj.Nombre, nil
		}
	}
	if supervisores == 0 {
		return "", ErrSinSupervisores
	}
	// Comparación señuelo: el tiempo de respuesta no delata cuántos supervisores hay.
	bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinv"), []byte(pin))
	s.audit.Append(evento(empresaID, actor, origen, "caja.autorizacion.fallida", accion, ""))
	return "", ErrPinSupervisorInvalido
}

// SesionesDeCaja devuelve el historial de turnos de la empresa (auditoría del
// arqueo): quién abrió qué caja y cuándo, más reciente primero.
func (s *Service) SesionesDeCaja(empresaID string) []caja.Sesion {
	out := s.sesiones.List(empresaID)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Apertura > out[j-1].Apertura; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// AbrirCaja es el login de cajero. Valida, en este orden:
//  1. que la caja exista en el tenant y esté habilitada;
//  2. que el código de cajero y el PIN sean correctos;
//  3. que el cajero pertenezca a la sede de la caja;
//  4. que la caja no esté tomada por OTRO cajero.
//
// Si el turno abierto es del mismo cajero, lo RETOMA sin crear un segundo
// registro de apertura — así el log de auditoría no miente sobre cuántas veces
// se abrió la caja (03 §4.3).
func (s *Service) AbrirCaja(empresaID, actor, origen, cajaID, codigoCajero, pin string) (caja.Sesion, error) {
	return s.AbrirCajaConFondo(empresaID, actor, origen, cajaID, codigoCajero, pin, 0)
}

// AbrirCajaConFondo es AbrirCaja con FONDO INICIAL: el efectivo en bolívares con
// el que se abre la gaveta (0 = sin fondo declarado). El fondo se guarda en la
// sesión y es la base del efectivo esperado en el arqueo del cierre. Al RETOMAR
// un turno propio NO se toca el fondo ya fijado: el turno abrió una sola vez.
func (s *Service) AbrirCajaConFondo(empresaID, actor, origen, cajaID, codigoCajero, pin string, fondoInicial float64) (caja.Sesion, error) {
	c, ok := s.cajas.ByID(empresaID, cajaID)
	if !ok {
		return caja.Sesion{}, ErrCajaNoExiste
	}
	if !c.Habilitada() {
		return caja.Sesion{}, ErrCajaDeshabilitada
	}
	if fondoInicial < 0 {
		fondoInicial = 0
	}
	fondoInicial = round2(fondoInicial)

	cj, ok := s.cajeros.ByCodigo(empresaID, strings.ToUpper(strings.TrimSpace(codigoCajero)))
	if !ok || !cj.Activo {
		// Comparación señuelo: el tiempo de respuesta no revela si el código existe.
		bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinv"), []byte(pin))
		return caja.Sesion{}, ErrCajeroInvalido
	}
	if bcrypt.CompareHashAndPassword([]byte(cj.PinHash), []byte(pin)) != nil {
		return caja.Sesion{}, ErrCajeroInvalido
	}
	if cj.SedeID != "" && cj.SedeID != c.SedeID {
		return caja.Sesion{}, ErrCajeroOtraSede
	}

	if ses, abierta := s.sesiones.AbiertaDeCaja(empresaID, cajaID); abierta {
		if ses.CajeroID != cj.ID {
			return caja.Sesion{}, ErrCajaTomada
		}
		// Retomar el turno propio: se actualiza el actor (puede haber cambiado
		// de dispositivo) pero NO se registra una apertura nueva.
		ses.ActorID = actor
		out, _ := s.sesiones.Update(ses)
		s.audit.Append(evento(empresaID, actor, origen, "caja.retomar", c.Codigo, cj.Codigo+" "+cj.Nombre))
		return out, nil
	}

	out := s.sesiones.Create(caja.Sesion{
		EmpresaID: empresaID, SedeID: c.SedeID, CajaID: c.ID, CajeroID: cj.ID,
		CajeroNombre: cj.Nombre, CajaCodigo: c.Codigo, ActorID: actor, Apertura: ahora(),
		FondoInicial: fondoInicial,
	})
	s.audit.Append(evento(empresaID, actor, origen, "caja.abrir", c.Codigo, cj.Codigo+" "+cj.Nombre))
	return out, nil
}

// CierreArqueo es el conteo DECLARADO por el cajero al cerrar el turno. Todos
// los campos son opcionales: un cierre sin conteo (p. ej. forzado por
// administración) congela igual lo esperado, pero sin declarar la gaveta.
type CierreArqueo struct {
	// EfectivoContadoBs es el efectivo en bolívares contado en la gaveta. Nil =
	// no se declaró (no se calcula diferencia). Es el conteo mínimo del arqueo.
	EfectivoContadoBs *float64
	// ContadoPorMetodo es el conteo declarado por método (opcional, para el acta).
	ContadoPorMetodo []caja.ArqueoConteo
}

// CerrarCaja termina el turno sin conteo declarado (congela lo esperado). Es la
// firma retrocompatible; el POS cierra con arqueo vía CerrarCajaConArqueo.
// `forzado` lo usa la Dueña/Admin para liberar una caja cuyo cajero se fue sin
// cerrar; queda diferenciado en la auditoría.
func (s *Service) CerrarCaja(empresaID, actor, origen, cajaID string, forzado bool) (caja.Sesion, error) {
	return s.CerrarCajaConArqueo(empresaID, actor, origen, cajaID, forzado, CierreArqueo{})
}

// CerrarCajaConArqueo termina el turno CONGELANDO el arqueo en la sesión: lo
// esperado por método (plegado de los documentos del turno), el efectivo Bs
// esperado (fondo + cobros − vuelto) y —si el cajero declaró un conteo— lo
// contado y la diferencia (sobrante/faltante). La foto queda inmutable en la
// sesión cerrada (append-only): no se reescribe después.
func (s *Service) CerrarCajaConArqueo(empresaID, actor, origen, cajaID string, forzado bool, in CierreArqueo) (caja.Sesion, error) {
	ses, abierta := s.sesiones.AbiertaDeCaja(empresaID, cajaID)
	if !abierta {
		return caja.Sesion{}, ErrSesionNoExiste
	}
	arqueo := s.arqueoDe(empresaID, ses)
	if in.EfectivoContadoBs != nil {
		arqueo.Declarado = true
		arqueo.EfectivoContadoBs = round2(*in.EfectivoContadoBs)
		// La diferencia puede ser negativa (faltante); round2 trunca en negativo,
		// así que se redondea el valor absoluto y se le devuelve el signo.
		arqueo.DiferenciaBs = round2signed(arqueo.EfectivoContadoBs - arqueo.EfectivoEsperadoBs)
	}
	if len(in.ContadoPorMetodo) > 0 {
		conteo := make([]caja.ArqueoConteo, 0, len(in.ContadoPorMetodo))
		for _, c := range in.ContadoPorMetodo {
			conteo = append(conteo, caja.ArqueoConteo{
				Metodo: c.Metodo, Moneda: c.Moneda, Monto: round2(c.Monto),
			})
		}
		arqueo.ContadoPorMetodo = conteo
	}
	ses.Cierre = ahora()
	ses.Arqueo = &arqueo
	out, _ := s.sesiones.Update(ses)
	accion := "caja.cerrar"
	if forzado {
		accion = "caja.cerrar_forzado"
	}
	detalle := ses.CajeroNombre
	if arqueo.Declarado {
		detalle = fmt.Sprintf("%s · diferencia Bs %.2f", ses.CajeroNombre, arqueo.DiferenciaBs)
	}
	s.audit.Append(evento(empresaID, actor, origen, accion, ses.CajaCodigo, detalle))
	return out, nil
}

// round2signed redondea a dos decimales conservando el signo. round2 trunca los
// negativos hacia cero (quirk documentado), lo que en una diferencia de arqueo
// convertiría un faltante de −6,00 en −5,99. Se redondea el valor absoluto.
func round2signed(v float64) float64 {
	if v < 0 {
		return -round2(-v)
	}
	return round2(v)
}

// SesionPorID resuelve un turno del tenant por su ID. El puerto expone List (ya
// aislado por empresa) pero no ByID, así que se busca sobre esa lista.
func (s *Service) SesionPorID(empresaID, sesionID string) (caja.Sesion, bool) {
	for _, ses := range s.sesiones.List(empresaID) {
		if ses.ID == sesionID {
			return ses, true
		}
	}
	return caja.Sesion{}, false
}

// ArqueoDeSesion calcula (sin persistir) el arqueo ESPERADO de un turno: pliega
// todos los documentos con ese SesionCajaID y devuelve el desglose por método de
// pago más el efectivo Bs esperado en la gaveta. Es el preview que el POS muestra
// antes de que el cajero declare el conteo. Si la sesión ya está cerrada devuelve
// su arqueo congelado (la foto real del cierre), no un recálculo.
func (s *Service) ArqueoDeSesion(empresaID, sesionID string) (caja.Arqueo, error) {
	ses, ok := s.SesionPorID(empresaID, sesionID)
	if !ok {
		return caja.Arqueo{}, ErrSesionNoExiste
	}
	if ses.Arqueo != nil {
		return *ses.Arqueo, nil
	}
	return s.arqueoDe(empresaID, ses), nil
}

// arqueoDe es el motor del arqueo: pliega los documentos cobrados en la sesión.
//
// Reglas de dinero (no se reimplementa el motor de conversión: se lee la tasa
// histórica grabada en cada pago):
//   - Solo cuentan las FACTURAS del turno; una factura ANULADA no cuenta (su
//     dinero se devolvió), igual que en SaldosDeTesoreria.
//   - Cada pago se agrupa por (método, moneda). El monto va en la moneda del
//     método; el equivalente en Bs sale de la tasa del pago (VES ⇒ 1).
//   - Efectivo Bs esperado = fondo inicial + cobros en efectivo Bs − vuelto
//     entregado en efectivo Bs. El vuelto por pago móvil no sale de la gaveta.
func (s *Service) arqueoDe(empresaID string, ses caja.Sesion) caja.Arqueo {
	arqueo := caja.Arqueo{FondoInicial: ses.FondoInicial}
	if s.documentos == nil {
		arqueo.EfectivoEsperadoBs = round2(ses.FondoInicial)
		return arqueo
	}
	// Facturas anuladas del tenant: su reversa deshace la venta, no cuentan.
	anulados := map[string]bool{}
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoAnulacion && d.RefDocumentoID != "" {
			anulados[d.RefDocumentoID] = true
		}
	}

	type clave struct{ metodo, moneda string }
	buckets := map[clave]*caja.ArqueoMetodo{}
	orden := []clave{}
	bucket := func(k clave, enDivisa, efectivo bool) *caja.ArqueoMetodo {
		if b, ok := buckets[k]; ok {
			return b
		}
		b := &caja.ArqueoMetodo{Metodo: k.metodo, Moneda: k.moneda, EnDivisa: enDivisa, Efectivo: efectivo}
		buckets[k] = b
		orden = append(orden, k)
		return b
	}

	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo != fiscal.TipoFactura || d.SesionCajaID != ses.ID || anulados[d.ID] {
			continue
		}
		arqueo.Documentos++
		for _, pg := range d.Pagos {
			moneda := pg.Moneda
			if moneda == "" {
				moneda = empresa.MonedaVES
			}
			montoBs := pg.Monto
			if pg.EnDivisa && pg.TasaCambio > 0 {
				montoBs = round2(pg.Monto * pg.TasaCambio)
			}
			efectivo := pg.Metodo == fiscal.PagoEfectivoBs || pg.Metodo == fiscal.PagoEfectivoUSD
			b := bucket(clave{pg.Metodo, moneda}, pg.EnDivisa, efectivo)
			b.Monto = round2(b.Monto + pg.Monto)
			b.EquivalenteBs = round2(b.EquivalenteBs + montoBs)
			arqueo.TotalCobradoBs = round2(arqueo.TotalCobradoBs + montoBs)
			if pg.Metodo == fiscal.PagoEfectivoBs {
				arqueo.CobrosEfectivoBs = round2(arqueo.CobrosEfectivoBs + pg.Monto)
			}
		}
		// El vuelto se pliega parte a parte (puede ser MIXTO): solo el efectivo en
		// bolívares sale de la GAVETA; el efectivo en divisas se cuenta aparte (no
		// mezcla monedas) y el pago móvil es una transferencia, no toca la gaveta.
		// Cada parte que sale (efectivo, en cualquier moneda) resta del total neto.
		for _, vp := range partesDeVuelto(d) {
			esEfectivo := vp.Metodo == "" || vp.Metodo == fiscal.VueltoEfectivo
			if !esEfectivo {
				continue // pago móvil: no sale de la gaveta ni del efectivo contado.
			}
			if vp.Moneda == "" || vp.Moneda == empresa.MonedaVES {
				arqueo.VueltoEfectivoBs = round2(arqueo.VueltoEfectivoBs + vp.MontoBs)
			} else {
				// Vuelto en efectivo en una DIVISA: la caja lo entrega de los billetes
				// de ESA divisa que recibió, así que rebaja el bucket de esa divisa
				// (esperado = recibido − vuelto). Sin esto el bucket conserva el bruto
				// recibido y sobrestima el efectivo esperado en esa moneda.
				for k, b := range buckets {
					if b.Efectivo && b.EnDivisa && k.moneda == vp.Moneda {
						b.Monto = round2(b.Monto - vp.Monto)
						b.EquivalenteBs = round2(b.EquivalenteBs - vp.MontoBs)
						break
					}
				}
			}
			arqueo.TotalCobradoBs = round2(arqueo.TotalCobradoBs - vp.MontoBs)
		}
	}

	// Orden determinista del desglose: efectivo Bs primero, luego las divisas en
	// efectivo, luego los medios electrónicos; a igualdad, por método/moneda.
	sort.SliceStable(orden, func(i, j int) bool {
		a, b := buckets[orden[i]], buckets[orden[j]]
		rank := func(m *caja.ArqueoMetodo) int {
			switch {
			case m.Efectivo && !m.EnDivisa:
				return 0
			case m.Efectivo && m.EnDivisa:
				return 1
			default:
				return 2
			}
		}
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if a.Metodo != b.Metodo {
			return a.Metodo < b.Metodo
		}
		return a.Moneda < b.Moneda
	})
	arqueo.Metodos = make([]caja.ArqueoMetodo, 0, len(orden))
	for _, k := range orden {
		arqueo.Metodos = append(arqueo.Metodos, *buckets[k])
	}

	arqueo.EfectivoEsperadoBs = round2(ses.FondoInicial + arqueo.CobrosEfectivoBs - arqueo.VueltoEfectivoBs)
	return arqueo
}

// SesionDeActor devuelve el turno abierto del usuario, si tiene uno. Es lo que
// consulta el frontend para decidir entre el punto de venta y la vista «Sin caja
// abierta», y lo que aplica la regla al emitir.
func (s *Service) SesionDeActor(empresaID, actorID string) (caja.Sesion, bool) {
	return s.sesiones.AbiertaDeActor(empresaID, actorID)
}
