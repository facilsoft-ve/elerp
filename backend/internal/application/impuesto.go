package application

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

// MAESTRO DE IMPUESTOS — casos de uso.
//
// La regla que este archivo hace cumplir, y que es la razón entera de que las
// alícuotas tengan vigencia: CAMBIAR UNA TASA NO PISA LA FILA VIEJA. Se cierra
// la que regía y se abre otra desde la fecha nueva. Si se sobreescribiera, un
// documento de hace seis meses ya no se podría explicar con la tasa de su día, y
// eso es justo lo que exige el Art. 177 (y lo que el SENIAT viene a mirar).
//
// Vive en el servicio y no en la pantalla a propósito: una regla de integridad
// que solo respeta la interfaz no es una regla, es una costumbre.

var (
	ErrAlicuotaNoExiste   = errors.New("la alícuota no existe")
	ErrAlicuotaCodigo     = errors.New("el código de la alícuota es obligatorio")
	ErrAlicuotaTipo       = errors.New("el tipo de alícuota no es válido")
	ErrAlicuotaPorcentaje = errors.New("el porcentaje debe ir entre 0 % y 100 %")
	ErrAlicuotaCodigoEnUso = errors.New("ya existe una alícuota vigente con ese código")
	// ErrVigenciaAnterior protege el histórico: abrir una tasa ANTES de la que ya
	// rige reescribiría el pasado — documentos ya emitidos quedarían explicados
	// por una tasa que no existía cuando se emitieron.
	ErrVigenciaAnterior = errors.New("la nueva vigencia no puede empezar antes de la que ya rige")
)

// AlicuotasDeEmpresa devuelve el maestro, SEMBRÁNDOLO si está vacío.
//
// La siembra es PEREZOSA y vive acá —no en la creación de la empresa— porque
// cuando esto se construyó ya había empresas creadas (las demo y las reales):
// sembrar solo en el alta las habría dejado sin maestro para siempre, y una
// migración para cuatro filas por empresa es más frágil que esto. Es idempotente
// y solo corre en la pantalla de configuración, nunca en el camino caliente de
// facturar (ver alicuotaDeProducto, que es de solo lectura a propósito).
func (s *Service) AlicuotasDeEmpresa(empresaID string) []fiscal.Alicuota {
	if s.alicuotas == nil {
		return []fiscal.Alicuota{}
	}
	if actuales := s.alicuotas.List(empresaID); len(actuales) > 0 {
		return actuales
	}
	// Desde el día en que se siembra: fechar antes sería inventar que estas tasas
	// regían en un pasado que nadie configuró.
	desde := time.Now().UTC().Format("2006-01-02")
	for _, a := range fiscal.AlicuotasPorDefecto(empresaID, desde) {
		s.alicuotas.Create(a)
	}
	return s.alicuotas.List(empresaID)
}

// AlicuotaVista es una fila del maestro como la lee la pantalla: la alícuota más
// si está rigiendo HOY. La vigencia se calcula en el servidor para que la
// interfaz no tenga que repetir la comparación de fechas (y equivocarse).
type AlicuotaVista struct {
	fiscal.Alicuota
	// Vigente indica si esta fila es la que se aplicaría a un documento de hoy.
	Vigente bool `json:"vigente"`
}

// AlicuotasVista devuelve el maestro con la marca de vigencia del día.
func (s *Service) AlicuotasVista(empresaID string) []AlicuotaVista {
	hoy := time.Now().UTC().Format("2006-01-02")
	todas := s.AlicuotasDeEmpresa(empresaID)
	out := make([]AlicuotaVista, 0, len(todas))
	for _, a := range todas {
		// Vigente de verdad = rige hoy Y es la elegida para su código (puede haber
		// solapes por configuración manual; gana la más reciente, igual que en el
		// motor de cálculo).
		elegida, ok := fiscal.VigenteEn(todas, a.Codigo, hoy)
		out = append(out, AlicuotaVista{Alicuota: a, Vigente: ok && elegida.ID == a.ID && a.Activa})
	}
	return out
}

// normalizarAlicuota valida y limpia los campos comunes de alta y edición.
func normalizarAlicuota(a *fiscal.Alicuota) error {
	a.Codigo = strings.ToLower(strings.TrimSpace(a.Codigo))
	if a.Codigo == "" {
		return ErrAlicuotaCodigo
	}
	a.Nombre = strings.TrimSpace(a.Nombre)
	if a.Nombre == "" {
		a.Nombre = a.Codigo
	}
	if !fiscal.TipoAlicuotaValido(a.Tipo) {
		return ErrAlicuotaTipo
	}
	// Las tasas se guardan en FRACCIÓN. Un 16 en vez de 0,16 multiplicaría el
	// impuesto por cien, así que el rango se valida acá y no solo en la pantalla.
	if a.Porcentaje < 0 || a.Porcentaje > 1 || a.Adicional < 0 || a.Adicional > 1 {
		return ErrAlicuotaPorcentaje
	}
	// La exenta no causa impuesto: un porcentaje ahí sería una contradicción que
	// después nadie sabría leer.
	if a.Tipo == fiscal.TipoExento {
		a.Porcentaje, a.Adicional = 0, 0
	}
	if a.VigenteDesde == "" {
		a.VigenteDesde = time.Now().UTC().Format("2006-01-02")
	}
	if !fechaDiaValida(a.VigenteDesde) {
		return errors.New("la fecha de vigencia debe ser AAAA-MM-DD")
	}
	if a.VigenteHasta != "" && !fechaDiaValida(a.VigenteHasta) {
		return errors.New("la fecha de fin de vigencia debe ser AAAA-MM-DD")
	}
	return nil
}

// fechaDiaValida acepta un AAAA-MM-DD real (no un 2026-13-45).
func fechaDiaValida(f string) bool {
	_, err := time.Parse("2006-01-02", strings.TrimSpace(f))
	return err == nil
}

// diaAnterior devuelve el día previo a una fecha AAAA-MM-DD. Es lo que cierra la
// vigencia de la tasa saliente: si la nueva rige desde el 1, la vieja rigió
// hasta el 31 — sin hueco ni solape de un día, que en una declaración mensual se
// nota.
func diaAnterior(f string) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(f))
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}

// CrearAlicuota da de alta una clasificación nueva en el maestro.
func (s *Service) CrearAlicuota(empresaID, actor, origen string, a fiscal.Alicuota) (fiscal.Alicuota, error) {
	if s.alicuotas == nil {
		return fiscal.Alicuota{}, errors.New("el maestro de impuestos no está disponible")
	}
	if err := normalizarAlicuota(&a); err != nil {
		return fiscal.Alicuota{}, err
	}
	// Dos filas vigentes con el mismo código dejarían el motor eligiendo por
	// desempate en vez de por configuración.
	if _, existe := fiscal.VigenteEn(s.AlicuotasDeEmpresa(empresaID), a.Codigo, a.VigenteDesde); existe {
		return fiscal.Alicuota{}, ErrAlicuotaCodigoEnUso
	}
	a.EmpresaID = empresaID
	a.ID = ""
	a.Activa = true
	out := s.alicuotas.Create(a)
	s.audit.Append(evento(empresaID, actor, origen, "config.alicuota.crear", out.Codigo,
		detalleAlicuota(out)))
	return out, nil
}

// CambiosAlicuota describe una edición. Los punteros nil no cambian nada.
type CambiosAlicuota struct {
	Nombre *string
	Activa *bool
	// Porcentaje y Adicional son el CAMBIO DE TASA. Cuando alguno viene, no se
	// edita la fila: se cierra su vigencia y se abre otra desde VigenteDesde.
	Porcentaje *float64
	Adicional  *float64
	// VigenteDesde es desde cuándo rige la tasa nueva. Vacío = hoy.
	VigenteDesde string
}

// ActualizarAlicuota edita el maestro distinguiendo DOS cosas que parecen la
// misma y no lo son:
//
//   - Corregir el NOMBRE o retirar la clasificación (Activa) se hace sobre la
//     fila: no cambia ningún cálculo, así que partir el histórico por un cambio
//     de redacción solo ensuciaría la tabla.
//   - Cambiar la TASA abre una fila nueva y cierra la anterior el día antes. Es
//     una providencia nueva, no una corrección: los documentos de ayer se
//     siguen explicando con la tasa de ayer.
//
// Excepción deliberada: si la tasa nueva rige DESDE EL MISMO DÍA que la que ya
// regía, se corrige en el sitio. Es el caso de quien se equivocó al cargarla
// hace un rato, y es seguro porque cada documento SELLA su alícuota al emitir
// (fiscal.Linea.Alicuota) — lo ya facturado no depende de esta tabla.
func (s *Service) ActualizarAlicuota(empresaID, actor, origen, id string, c CambiosAlicuota) (fiscal.Alicuota, error) {
	if s.alicuotas == nil {
		return fiscal.Alicuota{}, errors.New("el maestro de impuestos no está disponible")
	}
	actual, ok := s.alicuotas.ByID(empresaID, id)
	if !ok {
		return fiscal.Alicuota{}, ErrAlicuotaNoExiste
	}

	cambiaTasa := (c.Porcentaje != nil && *c.Porcentaje != actual.Porcentaje) ||
		(c.Adicional != nil && *c.Adicional != actual.Adicional)

	if !cambiaTasa {
		if c.Nombre != nil {
			actual.Nombre = strings.TrimSpace(*c.Nombre)
		}
		if c.Activa != nil {
			actual.Activa = *c.Activa
		}
		if err := normalizarAlicuota(&actual); err != nil {
			return fiscal.Alicuota{}, err
		}
		out, ok := s.alicuotas.Update(actual)
		if !ok {
			return fiscal.Alicuota{}, ErrAlicuotaNoExiste
		}
		s.audit.Append(evento(empresaID, actor, origen, "config.alicuota.editar", out.Codigo, out.Nombre))
		return out, nil
	}

	desde := strings.TrimSpace(c.VigenteDesde)
	if desde == "" {
		desde = time.Now().UTC().Format("2006-01-02")
	}
	if !fechaDiaValida(desde) {
		return fiscal.Alicuota{}, errors.New("la fecha de vigencia debe ser AAAA-MM-DD")
	}
	if desde < actual.VigenteDesde {
		return fiscal.Alicuota{}, ErrVigenciaAnterior
	}

	nueva := actual
	nueva.ID = ""
	nueva.VigenteDesde = desde
	nueva.VigenteHasta = ""
	nueva.Activa = true
	if c.Porcentaje != nil {
		nueva.Porcentaje = *c.Porcentaje
	}
	if c.Adicional != nil {
		nueva.Adicional = *c.Adicional
	}
	if c.Nombre != nil {
		nueva.Nombre = strings.TrimSpace(*c.Nombre)
	}
	if err := normalizarAlicuota(&nueva); err != nil {
		return fiscal.Alicuota{}, err
	}

	// Mismo día: es una corrección, no una providencia. Se pisa la fila — seguro,
	// porque los documentos ya emitidos llevan su alícuota sellada.
	if desde == actual.VigenteDesde {
		nueva.ID = actual.ID
		out, ok := s.alicuotas.Update(nueva)
		if !ok {
			return fiscal.Alicuota{}, ErrAlicuotaNoExiste
		}
		s.audit.Append(evento(empresaID, actor, origen, "config.alicuota.corregir", out.Codigo,
			detalleAlicuota(out)))
		return out, nil
	}

	// Providencia nueva: se cierra la anterior el día antes y se abre la nueva.
	actual.VigenteHasta = diaAnterior(desde)
	s.alicuotas.Update(actual)
	out := s.alicuotas.Create(nueva)
	s.audit.Append(evento(empresaID, actor, origen, "config.alicuota.nueva_vigencia", out.Codigo,
		detalleAlicuota(out)+" · desde "+desde+" (la anterior rigió hasta "+actual.VigenteHasta+")"))
	return out, nil
}

// detalleAlicuota arma la línea que queda en la bitácora. Se escribe en
// porcentaje y no en fracción: quien lee una auditoría piensa en 16 %, no en
// 0,16.
func detalleAlicuota(a fiscal.Alicuota) string {
	d := a.Nombre + " · " + pct(a.Porcentaje)
	if a.Adicional > 0 {
		d += " + " + pct(a.Adicional) + " adicional"
	}
	return d
}

// pct escribe una fracción como porcentaje legible: 0.16 → "16 %", 0.155 →
// "15,5 %". Sin ceros de relleno, que en una bitácora solo hacen ruido.
func pct(frac float64) string {
	return strconv.FormatFloat(round2(frac*100), 'f', -1, 64) + " %"
}

// ErrAlicuotaDesconocida protege la coherencia entre el catálogo y el motor de
// cálculo: clasificar un producto con un código que el maestro no tiene lo
// dejaría facturando a la tasa de respaldo sin que nadie se entere.
var ErrAlicuotaDesconocida = errors.New("esa alícuota no existe en el maestro de impuestos")

// validarAlicuotaProducto comprueba que el código exista y esté vigente, y de
// paso SIEMBRA el maestro si la empresa todavía no lo tiene.
//
// Sembrar acá y no solo en la pantalla de configuración es deliberado: sin
// esto, un tenant nuevo que clasificara un producto por API o por importación
// —sin haber abierto nunca Ajustes— facturaría a la tasa de respaldo aunque su
// ficha dijera «suntuario». Un impuesto mal calculado en silencio es de los
// errores más caros que puede tener este sistema.
//
// Código vacío es válido: significa «como siempre» (manda el booleano ExentoIVA).
func (s *Service) validarAlicuotaProducto(empresaID, codigo string) (string, error) {
	cod := strings.ToLower(strings.TrimSpace(codigo))
	if cod == "" || s.alicuotas == nil {
		return cod, nil
	}
	hoy := time.Now().UTC().Format("2006-01-02")
	if a, ok := fiscal.VigenteEn(s.AlicuotasDeEmpresa(empresaID), cod, hoy); ok && a.Activa {
		return cod, nil
	}
	return "", ErrAlicuotaDesconocida
}
