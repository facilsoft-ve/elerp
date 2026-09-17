package application

import (
	"errors"
	"math"
	"strings"

	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/sede"
)

// PRESENCIA ESTRICTA: ciertos roles solo pueden empezar a trabajar estando
// FÍSICAMENTE en su sede. Se compara la ubicación que reporta el navegador
// contra las coordenadas de la sede.
//
// ES CAPACIDAD DE PLATAFORMA, NO DEL MÓDULO RESTAURANTE. Las coordenadas viven
// en sede.Sede y los roles en la configuración de empresa; el salón la CONSUME
// al abrir un turno. Se diseñó así porque el cajero es tan candidato como el
// mesonero, y el cajero no es del salón: meter esto dentro de Restaurante
// obligaría a duplicarlo o a romper el límite del módulo (principio 5).
//
// HASTA DÓNDE LLEGA — y conviene no engañarse:
//
//	La geolocalización del navegador SE PUEDE FALSEAR. Una app de GPS falso en
//	Android, o las herramientas de desarrollo del propio navegador, reportan la
//	coordenada que se quiera. Esto sube el listón para una persona normal —que es
//	el caso real que preocupa: el mesonero que entra desde su casa— pero NO
//	detiene a alguien decidido. Lo que cierra eso de verdad es atar la sesión a un
//	EQUIPO INSCRITO, que es una capa posterior y todavía no está construida.
//
// Y una regla que no se puede cumplir no protege: si el GPS falla o la persona
// niega el permiso, no se tranca el servicio. El supervisor puede autorizar la
// excepción con su PIN, declarando el motivo, y eso queda auditado. La presencia
// física convence en el momento pero no deja rastro; el registro sí.

var (
	// ErrPresenciaSinUbicacion: el rol exige presencia pero no llegó ninguna
	// posición (permiso negado, GPS apagado, navegador sin soporte). Es un error
	// DISTINGUIBLE a propósito: la interfaz necesita saber que este caso se
	// resuelve pidiendo la excepción del supervisor, no reintentando.
	ErrPresenciaSinUbicacion = errors.New("no se pudo obtener tu ubicación: pídele a un supervisor que autorice la excepción")
	// ErrFueraDeSede: llegó una posición y está fuera del radio de la sede.
	ErrFueraDeSede = errors.New("estás fuera de la sede: hay que iniciar el turno en el local")
	// ErrCoordenadaInvalida protege de guardar una sede en una latitud imposible.
	ErrCoordenadaInvalida = errors.New("las coordenadas de la sede no son válidas")
	ErrRadioInvalido      = errors.New("el radio de la sede debe ir entre 20 y 5000 metros")
)

// Límites del radio configurable. El mínimo existe porque bajo 20 m ni el mejor
// GPS de celular distingue nada y la regla se volvería un bloqueo permanente; el
// máximo, porque un radio de medio pueblo no verifica presencia, solo la simula.
const (
	radioMinimoM = 20
	radioMaximoM = 5000
)

// precisionMaximaConsiderada acota cuánto margen se le concede a la precisión
// que reporta el navegador.
//
// POR QUÉ UN TOPE: la precisión la declara el CLIENTE. Sin tope, reportar una
// precisión de 100 km haría que cualquier punto del país cayera «dentro» y la
// verificación no serviría para nada. 200 m cubre con holgura el GPS malo de
// interiores, que es el caso honesto que queremos perdonar.
const precisionMaximaConsiderada = 200.0

// Posicion es la ubicación que reporta el navegador. PrecisionM es el radio de
// incertidumbre que el propio navegador declara (`coords.accuracy`).
type Posicion struct {
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	PrecisionM float64 `json:"precisionM"`
}

// ConSedes cablea el repositorio de sedes al Service. Hace falta para leer las
// coordenadas al verificar presencia. Opcional: sin cablear, la verificación no
// aplica y todo se comporta como antes de existir esta capacidad.
func (s *Service) ConSedes(r sede.Repository) *Service {
	s.sedes = r
	return s
}

// radioTierraM es el radio medio de la Tierra. Con la fórmula del haversine y
// distancias de pocos cientos de metros, el error del modelo esférico es
// despreciable frente a la precisión del propio GPS.
const radioTierraM = 6371000.0

// distanciaM devuelve los metros entre dos coordenadas por la fórmula del
// haversine (distancia sobre la superficie de la esfera).
//
// Se usa haversine y no una resta de grados porque un grado de longitud NO mide
// lo mismo en todas partes: en el ecuador son ~111 km y cerca de los polos casi
// nada. Restar grados daría un radio deformado según la latitud del local.
func distanciaM(lat1, lon1, lat2, lon2 float64) float64 {
	const gradoARadian = math.Pi / 180
	dLat := (lat2 - lat1) * gradoARadian
	dLon := (lon2 - lon1) * gradoARadian
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*gradoARadian)*math.Cos(lat2*gradoARadian)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	// Math.Min(1, …) evita que un error de redondeo saque a Asin de su dominio.
	return 2 * radioTierraM * math.Asin(math.Min(1, math.Sqrt(a)))
}

// VerificarPresencia comprueba que quien va a empezar a trabajar esté en su
// sede. Devuelve nil cuando la verificación NO APLICA, que son varios casos
// legítimos y no excepciones:
//
//   - el repositorio de sedes no está cableado;
//   - ese rol no está en la lista de presencia estricta de la empresa;
//   - la sede no tiene coordenadas configuradas (no hay contra qué comparar).
//
// Cuando sí aplica: sin posición devuelve ErrPresenciaSinUbicacion (la interfaz
// ofrece la excepción del supervisor) y fuera del radio, ErrFueraDeSede.
func (s *Service) VerificarPresencia(empresaID, sedeID, rol string, pos *Posicion) error {
	if s.sedes == nil || s.empresas == nil {
		return nil
	}
	emp, ok := s.empresas.ByID(empresaID)
	if !ok || !emp.ExigePresencia(rol) {
		return nil
	}
	sd, ok := s.sedes.ByID(empresaID, sedeID)
	if !ok || !sd.TieneUbicacion() {
		// Exigir presencia contra una sede sin coordenadas dejaría al local sin
		// poder trabajar por una configuración a medias. Se prefiere no verificar
		// —y que se note en la pantalla de configuración— antes que trancar.
		return nil
	}
	if pos == nil {
		return ErrPresenciaSinUbicacion
	}
	if !sede.CoordenadaValida(pos.Lat, pos.Lon) {
		// Una coordenada imposible no es «lejos»: es un dato roto. Se trata como
		// «sin ubicación» para que salga por la vía de la excepción y no como un
		// rechazo que la persona no puede resolver moviéndose.
		return ErrPresenciaSinUbicacion
	}
	dist := distanciaM(sd.Lat, sd.Lon, pos.Lat, pos.Lon)
	// El margen de precisión juega A FAVOR de quien consulta: si el navegador dice
	// «estoy acá ±80 m», se le conceden esos 80 m. Es la diferencia entre una
	// regla usable y uno que llama al supervisor cada noche porque el GPS de su
	// teléfono es malo bajo techo.
	margen := math.Max(0, math.Min(pos.PrecisionM, precisionMaximaConsiderada))
	if dist-margen > float64(sd.RadioEfectivo()) {
		return ErrFueraDeSede
	}
	return nil
}

// FijarUbicacionSede guarda las coordenadas y el radio de una sede. Radio 0
// deja el valor por defecto (sede.RadioPorDefecto).
//
// Lat y Lon en cero BORRAN la ubicación: es la forma de apagar la verificación
// para una sede sin tener que vaciar la lista de roles de toda la empresa.
func (s *Service) FijarUbicacionSede(empresaID, sedeID, actor, origen string, lat, lon float64, radioM int) (sede.Sede, error) {
	if s.sedes == nil {
		return sede.Sede{}, errors.New("las sedes no están disponibles")
	}
	sd, ok := s.sedes.ByID(empresaID, sedeID)
	if !ok {
		return sede.Sede{}, ErrSedeNoExiste
	}
	if !sede.CoordenadaValida(lat, lon) {
		return sede.Sede{}, ErrCoordenadaInvalida
	}
	if radioM != 0 && (radioM < radioMinimoM || radioM > radioMaximoM) {
		return sede.Sede{}, ErrRadioInvalido
	}
	sd.Lat, sd.Lon, sd.RadioM = lat, lon, radioM
	out, ok := s.sedes.Update(sd)
	if !ok {
		return sede.Sede{}, ErrSedeNoExiste
	}
	detalle := "ubicación borrada"
	if out.TieneUbicacion() {
		detalle = "ubicación fijada · radio " + itoa(out.RadioEfectivo()) + " m"
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.sede.ubicacion", out.ID, out.Nombre+" · "+detalle))
	return out, nil
}

// FijarRolesPresencia define qué roles exigen estar en la sede para empezar a
// trabajar. Lista vacía = ninguno (el comportamiento de siempre).
func (s *Service) FijarRolesPresencia(empresaID, actor, origen string, roles []string) (empresa.Empresa, error) {
	if s.empresas == nil {
		return empresa.Empresa{}, errors.New("la empresa no está disponible")
	}
	e, ok := s.empresas.ByID(empresaID)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	e.RolesPresenciaEstricta = empresa.NormalizarRolesPresencia(roles)
	out, ok := s.empresas.Update(e)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	detalle := "ninguno"
	if len(out.RolesPresenciaEstricta) > 0 {
		detalle = strings.Join(out.RolesPresenciaEstricta, ", ")
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.presencia.roles", empresaID, detalle))
	return out, nil
}

// itoa evita arrastrar strconv solo para armar un texto de auditoría.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
