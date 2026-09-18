package application

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/adapter/unidigital"
	fd "github.com/mornix/elerp/internal/domain/facturaciondigital"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* FACTURACIÓN DIGITAL — casos de uso.
 *
 * El módulo se enciende por empresa y por canal (mostrador / módulo de ventas).
 * Apagado, nada de esto corre y ElERP factura exactamente como antes: eso es lo
 * que lo hace un módulo y no una bifurcación del motor fiscal.
 *
 * La pieza crítica es EncolarEmision, que se llama DESPUÉS de emitir el
 * documento local y nunca antes: el cobro no puede depender de que la imprenta
 * conteste. Si la imprenta está caída, la venta igual se cobra y la factura se
 * envía cuando vuelva.
 */

var (
	ErrDigitalNoDisponible = errors.New("la facturación digital no está disponible en esta instancia")
	ErrDigitalIncompleta   = errors.New("faltan datos para activar: usuario, contraseña, serie y sucursal")
	ErrDigitalCredenciales = errors.New("la imprenta digital rechazó las credenciales")
)

// ConFacturacionDigital cablea el módulo. Opcional: sin él, ElERP factura como
// siempre y las pantallas del módulo responden que no está disponible.
func (s *Service) ConFacturacionDigital(cfg fd.ConfigRepository, emi fd.EmisionRepository) *Service {
	s.configDigital = cfg
	s.emisionesDigitales = emi
	return s
}

// ConfigDigital devuelve la configuración de la empresa (vacía si nunca se tocó).
func (s *Service) ConfigDigital(empresaID string) fd.Config {
	if s.configDigital == nil {
		return fd.Config{}
	}
	c, _ := s.configDigital.Get(empresaID)
	c.EmpresaID = empresaID
	if c.Ambiente == "" {
		c.Ambiente = fd.AmbienteQA
	}
	if c.TicketPOS == "" {
		c.TicketPOS = fd.TicketTermica
	}
	return c
}

// EntradaConfigDigital son los datos que manda la pantalla. La contraseña llega
// en claro UNA vez y se guarda hasheada; vacía significa «no la cambies», para
// poder editar el resto sin volver a teclearla.
type EntradaConfigDigital struct {
	Activa           bool
	PorPOS           bool
	PorVentas        bool
	Ambiente         string
	Usuario          string
	Password         string
	SerieStrongID    string
	SerieNombre      string
	SucursalStrongID string
	SucursalNombre   string
	TicketPOS        string
}

// GuardarConfigDigital valida y persiste. NO deja activar sin serie y sucursal:
// sin ellas la imprenta rechaza todo, y es mejor no dejar encender el módulo que
// fallar en la primera venta del día.
func (s *Service) GuardarConfigDigital(empresaID, actor, origen string, in EntradaConfigDigital) (fd.Config, error) {
	if s.configDigital == nil {
		return fd.Config{}, ErrDigitalNoDisponible
	}
	c := s.ConfigDigital(empresaID)
	c.Activa = in.Activa
	c.PorPOS = in.PorPOS
	c.PorVentas = in.PorVentas
	if in.Ambiente == fd.AmbienteProduccion {
		c.Ambiente = fd.AmbienteProduccion
	} else {
		c.Ambiente = fd.AmbienteQA
	}
	c.Usuario = strings.TrimSpace(in.Usuario)
	if p := strings.TrimSpace(in.Password); p != "" {
		// Se guarda el digest, no la contraseña: es lo que viaja a la API, así que
		// guardar el texto plano no aporta nada y sí expone.
		c.PasswordSHA512 = unidigital.HashPassword(p)
	}
	c.SerieStrongID = strings.TrimSpace(in.SerieStrongID)
	c.SerieNombre = strings.TrimSpace(in.SerieNombre)
	c.SucursalStrongID = strings.TrimSpace(in.SucursalStrongID)
	c.SucursalNombre = strings.TrimSpace(in.SucursalNombre)
	switch in.TicketPOS {
	case fd.TicketFiscalNoFiscal, fd.TicketNinguno:
		c.TicketPOS = in.TicketPOS
	default:
		c.TicketPOS = fd.TicketTermica
	}
	c.Actualizada = ahora()

	if c.Activa && !c.Lista() {
		return fd.Config{}, ErrDigitalIncompleta
	}
	out := s.configDigital.Upsert(c)
	s.audit.Append(evento(empresaID, actor, origen, "config.facturaciondigital", empresaID,
		fmt.Sprintf("activa=%v pos=%v ventas=%v ambiente=%s", c.Activa, c.PorPOS, c.PorVentas, c.Ambiente)))
	return out, nil
}

// clienteDigital arma el cliente de la imprenta para una empresa.
func (s *Service) clienteDigital(c fd.Config) (*unidigital.Cliente, error) {
	if strings.TrimSpace(c.Usuario) == "" || strings.TrimSpace(c.PasswordSHA512) == "" {
		return nil, ErrDigitalIncompleta
	}
	return unidigital.Nuevo(c.BaseURL(), c.Usuario, c.PasswordSHA512), nil
}

// DiagnosticoDigital es lo que devuelve «Probar conexión». Lista lo que la
// imprenta reconoce de la cuenta, para poder ELEGIR serie y sucursal en vez de
// teclear un UUID, y para ver de una si la cuenta está habilitada.
type DiagnosticoDigital struct {
	OK bool `json:"ok"`
	// Avisos son los `information[]` de la imprenta: ahí dice, por ejemplo, que
	// una serie no tiene plantilla de correo.
	Avisos []string `json:"avisos"`
	// Series son las que EXISTEN; Configuradas, las habilitadas para emitir. La
	// distinción importa: una cuenta puede tener la serie listada y no poder
	// facturar con ella, que es exactamente cómo nos entregaron el sandbox.
	Series       []unidigital.Serie    `json:"series"`
	Configuradas []unidigital.Serie    `json:"configuradas"`
	Sucursales   []unidigital.Sucursal `json:"sucursales"`
	// PuedeEmitir resume el diagnóstico: hay al menos una serie habilitada y una
	// sucursal. Es lo que decide si tiene sentido activar el módulo.
	PuedeEmitir bool   `json:"puedeEmitir"`
	Problema    string `json:"problema,omitempty"`
}

// ProbarDigital autentica contra la imprenta y trae series y sucursales.
//
// `password` en claro permite probar ANTES de guardar; vacía usa la guardada.
func (s *Service) ProbarDigital(ctx context.Context, empresaID, ambiente, usuario, password string) (DiagnosticoDigital, error) {
	c := s.ConfigDigital(empresaID)
	if ambiente != "" {
		c.Ambiente = ambiente
	}
	if u := strings.TrimSpace(usuario); u != "" {
		c.Usuario = u
	}
	if p := strings.TrimSpace(password); p != "" {
		c.PasswordSHA512 = unidigital.HashPassword(p)
	}
	cli, err := s.clienteDigital(c)
	if err != nil {
		return DiagnosticoDigital{}, err
	}
	avisos, err := cli.Login(ctx)
	if err != nil {
		return DiagnosticoDigital{Avisos: avisos, Problema: err.Error()}, ErrDigitalCredenciales
	}
	d := DiagnosticoDigital{OK: true, Avisos: avisos}
	// Las series se piden por su propio endpoint y NO se leen del login: el login
	// devolvió `series: []` en una cuenta que sí tenía una (ver
	// docs/unidigital-api-verificada.md).
	d.Series, _ = cli.Series(ctx)
	d.Configuradas, _ = cli.SeriesConfiguradas(ctx)
	d.Sucursales, _ = cli.Sucursales(ctx)
	d.PuedeEmitir = len(d.Configuradas) > 0 && len(d.Sucursales) > 0
	if !d.PuedeEmitir {
		d.Problema = problemaDeProvision(d)
	}
	return d, nil
}

// problemaDeProvision explica en una línea qué le falta a la cuenta. Sin esto,
// el error que llega al emitir es «La serie no es válida», que manda a buscar el
// problema en el lugar equivocado.
func problemaDeProvision(d DiagnosticoDigital) string {
	falta := []string{}
	if len(d.Configuradas) == 0 {
		if len(d.Series) > 0 {
			falta = append(falta, "la serie existe pero no está habilitada para emitir")
		} else {
			falta = append(falta, "la cuenta no tiene series")
		}
	}
	if len(d.Sucursales) == 0 {
		falta = append(falta, "no hay sucursales registradas")
	}
	if len(falta) == 0 {
		return ""
	}
	return "La imprenta todavía no puede emitir con esta cuenta: " + strings.Join(falta, " y ") +
		". Pídelo a soporte de la imprenta antes de activar el módulo."
}

/* --- Emisión ------------------------------------------------------------- */

// tokenPublico genera el identificador de la página pública. Es aleatorio y no
// derivado del documento a propósito: la página no tiene clave, así que un
// identificador adivinable dejaría recorrer las facturas de la empresa.
func tokenPublico() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		// Sin azar del sistema es preferible fallar el token que emitir uno
		// predecible: la página quedaría abierta a enumeración.
		return ""
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

// EncolarEmision registra la intención de facturar digitalmente un documento.
//
// ES UN OUTBOX: se persiste ANTES de tocar la red y el envío lo hace el lazo de
// fondo. Si el proceso se cae entre el cobro y el envío, la factura no se pierde.
// Y si la imprenta está caída, la venta igual se cobró.
//
// Devuelve la emisión creada, o `false` si el canal no aplica (módulo apagado,
// canal apagado, o tipo de documento que no se emite por imprenta).
func (s *Service) EncolarEmision(empresaID, canal string, doc fiscal.Documento) (fd.Emision, bool) {
	if s.emisionesDigitales == nil || s.configDigital == nil {
		return fd.Emision{}, false
	}
	cfg := s.ConfigDigital(empresaID)
	if !cfg.AplicaA(canal) {
		return fd.Emision{}, false
	}
	tipo, ok := tipoDocumentoDigital(doc.Tipo)
	if !ok {
		return fd.Emision{}, false
	}
	// Idempotencia local: un documento no se encola dos veces. El reintento vive
	// en la emisión que ya existe.
	if ya, existe := s.emisionesDigitales.ByDocumento(empresaID, doc.ID); existe {
		return ya, true
	}
	// El correlativo de la imprenta es PROPIO y ascendente por serie y tipo: no se
	// reusa el número del documento local, que lleva su propia secuencia.
	numero := 0
	if s.numerador != nil {
		numero = s.numerador.Siguiente(empresaID, "", serieImprenta(cfg, tipo))
	}
	e := fd.Emision{
		EmpresaID: empresaID, SedeID: doc.SedeID, DocumentoID: doc.ID,
		Tipo: tipo, Numero: numero, Serie: cfg.SerieNombre,
		Estado: fd.EstadoPendiente, Token: tokenPublico(),
		Creada: ahora(), Actualizada: ahora(),
	}
	out := s.emisionesDigitales.Append(e)
	s.audit.Append(evento(empresaID, doc.Actor, canal, "facturaciondigital.encolada",
		doc.NumeroCompleto, fmt.Sprintf("%s nº %d", tipo, numero)))
	return out, true
}

// serieImprenta es la clave del contador del correlativo de la imprenta. Lleva
// el tipo adentro porque la imprenta numera por serie Y tipo, con secuencias
// independientes.
func serieImprenta(cfg fd.Config, tipo string) string {
	return "UD-" + cfg.SerieStrongID + "-" + tipo
}

/* EL NÚMERO DE CONTROL NO SE ESCRIBE EN EL DOCUMENTO, y no es un olvido.
 *
 * El documento es APPEND-ONLY y va sellado: su hash cubre el número de control y
 * encadena con el documento anterior de la empresa (ver fiscal.contenidoSellado).
 * Con imprenta digital ese número llega DESPUÉS de sellar, así que escribirlo
 * encima rompería la cadena de integridad de todo lo emitido después — que es
 * justamente lo que hay que poder demostrarle al SENIAT.
 *
 * Así que el número vive en la EMISIÓN, y quien lo necesite lo resuelve por acá.
 * Hay precedente: en máquina fiscal el documento también queda sin número de
 * control, porque lo asigna la impresora.
 */

// NumeroControlDigital devuelve el número de control que la imprenta asignó a un
// documento, o "" si todavía no llegó (o si no se factura digitalmente).
func (s *Service) NumeroControlDigital(empresaID, documentoID string) string {
	e, ok := s.EmisionDeDocumento(empresaID, documentoID)
	if !ok {
		return ""
	}
	return e.NumeroControl
}

// EmisionDeDocumento devuelve el estado de la emisión de un documento.
func (s *Service) EmisionDeDocumento(empresaID, documentoID string) (fd.Emision, bool) {
	if s.emisionesDigitales == nil {
		return fd.Emision{}, false
	}
	return s.emisionesDigitales.ByDocumento(empresaID, documentoID)
}

// EmisionPorToken resuelve la página pública. No filtra por empresa: el
// visitante solo trae el token.
func (s *Service) EmisionPorToken(token string) (fd.Emision, bool) {
	if s.emisionesDigitales == nil {
		return fd.Emision{}, false
	}
	return s.emisionesDigitales.ByToken(token)
}

// EmisionesDigitales lista el outbox de la empresa, para la pantalla de
// seguimiento: es la tabla de conciliación documento ↔ número ↔ control.
func (s *Service) EmisionesDigitales(empresaID string) []fd.Emision {
	if s.emisionesDigitales == nil {
		return []fd.Emision{}
	}
	return s.emisionesDigitales.List(empresaID)
}

/* --- Lazo de fondo -------------------------------------------------------- */

// esperaTrasFallo es la espera creciente entre reintentos. Crece rápido para no
// martillar a la imprenta cuando está caída, y se detiene en una hora: más allá
// de eso conviene que alguien mire.
func esperaTrasFallo(intentos int) time.Duration {
	switch {
	case intentos <= 1:
		return 30 * time.Second
	case intentos == 2:
		return 2 * time.Minute
	case intentos == 3:
		return 10 * time.Minute
	default:
		return time.Hour
	}
}

// ProcesarEmisiones atiende una tanda del outbox: envía las pendientes y
// consulta el número de control de las enviadas.
//
// Corre de a UNA: la imprenta responde 429 cuando la petición anterior de la
// cuenta sigue procesándose, así que la concurrencia acá no acelera nada — hace
// fallar envíos.
func (s *Service) ProcesarEmisiones(ctx context.Context, limite int) int {
	if s.emisionesDigitales == nil {
		return 0
	}
	hechas := 0
	for _, e := range s.emisionesDigitales.Pendientes(limite) {
		if e.ProximoIntento != "" {
			if t, err := time.Parse(time.RFC3339, e.ProximoIntento); err == nil && time.Now().Before(t) {
				continue // todavía no le toca
			}
		}
		cfg := s.ConfigDigital(e.EmpresaID)
		if !cfg.Lista() {
			continue // el módulo se apagó o quedó incompleto: no se fuerza nada
		}
		cli, err := s.clienteDigital(cfg)
		if err != nil {
			continue
		}
		switch e.Estado {
		case fd.EstadoPendiente, fd.EstadoErrorTemporal:
			s.enviarEmision(ctx, cli, cfg, e)
		case fd.EstadoEnviado:
			s.consultarEmision(ctx, cli, e)
		}
		hechas++
	}
	return hechas
}

// enviarEmision manda el documento a la imprenta.
func (s *Service) enviarEmision(ctx context.Context, cli *unidigital.Cliente, cfg fd.Config, e fd.Emision) {
	doc, ok := s.documentos.ByID(e.EmpresaID, e.DocumentoID)
	if !ok {
		s.marcarRechazada(e, unidigital.ErrorAPI{Code: "LOCAL", Message: "el documento ya no existe"})
		return
	}
	correo := ""
	if cl, ok := s.clientes.ByID(e.EmpresaID, doc.ClienteID); ok {
		correo = cl.Email
	}
	cuerpo, err := CuerpoImprenta(doc, cfg, e.Numero, e.DocumentoID, correo)
	if err != nil {
		s.marcarRechazada(e, unidigital.ErrorAPI{Code: "MAPEO", Message: err.Error()})
		return
	}
	strongID, err := cli.CrearYAprobar(ctx, cuerpo)
	if err != nil {
		s.registrarFallo(e, err)
		return
	}
	e.StrongID = strongID
	e.Estado = fd.EstadoEnviado
	e.ProximoIntento = time.Now().Add(90 * time.Second).Format(time.RFC3339)
	e.Actualizada = ahora()
	e.Intentos = append(e.Intentos, fd.Intento{Cuando: ahora(), HTTP: 200})
	s.emisionesDigitales.Update(e)
}

// consultarEmision busca el número de control, que llega entre uno y cinco
// minutos después de aceptado el documento.
func (s *Service) consultarEmision(ctx context.Context, cli *unidigital.Cliente, e fd.Emision) {
	doc, err := cli.PorStrongID(ctx, e.StrongID)
	if err != nil {
		s.registrarFallo(e, err)
		return
	}
	if doc.ControlNumber == 0 {
		// Todavía no se lo asignan: se vuelve a preguntar más tarde. No es un
		// error, es el ciclo normal.
		e.ProximoIntento = time.Now().Add(60 * time.Second).Format(time.RFC3339)
		e.Actualizada = ahora()
		s.emisionesDigitales.Update(e)
		return
	}
	e.NumeroControl = fmt.Sprintf("%d", doc.ControlNumber)
	e.Estado = fd.EstadoFiscal
	e.ProximoIntento = ""
	e.Actualizada = ahora()
	// El código corto y la URL son para la página pública; si no vienen, la
	// página igual funciona con lo que ya tiene.
	if c, err := cli.CodigoCorto(ctx, e.StrongID); err == nil {
		e.CodigoCorto = c
	}
	if u, err := cli.URLDocumento(ctx, e.StrongID); err == nil {
		e.URLDocumento = u
	}
	s.emisionesDigitales.Update(e)
	s.audit.Append(evento(e.EmpresaID, "sistema", "imprenta_digital", "facturaciondigital.control",
		e.DocumentoID, e.NumeroControl))
}

// registrarFallo anota el intento y decide si se reintenta. Un 400 NO se
// reintenta solo: es una regla de negocio incumplida, y repetirlo a ciegas
// quema otro correlativo y vuelve a fallar igual.
func (s *Service) registrarFallo(e fd.Emision, err error) {
	var f *unidigital.Fallo
	if errors.As(err, &f) {
		e.Intentos = append(e.Intentos, fd.Intento{
			Cuando: ahora(), HTTP: f.HTTP,
			Codigo: f.Primero().Code, Mensaje: f.Primero().Message, Extra: f.Primero().Extra,
		})
		if !f.Temporal() {
			e.Estado = fd.EstadoRechazado
			e.ProximoIntento = ""
			e.Actualizada = ahora()
			s.emisionesDigitales.Update(e)
			return
		}
	} else {
		e.Intentos = append(e.Intentos, fd.Intento{Cuando: ahora(), Mensaje: err.Error()})
	}
	e.Estado = fd.EstadoErrorTemporal
	e.ProximoIntento = time.Now().Add(esperaTrasFallo(len(e.Intentos))).Format(time.RFC3339)
	e.Actualizada = ahora()
	s.emisionesDigitales.Update(e)
}

func (s *Service) marcarRechazada(e fd.Emision, er unidigital.ErrorAPI) {
	e.Estado = fd.EstadoRechazado
	e.ProximoIntento = ""
	e.Actualizada = ahora()
	e.Intentos = append(e.Intentos, fd.Intento{Cuando: ahora(), Codigo: er.Code, Mensaje: er.Message})
	s.emisionesDigitales.Update(e)
}
