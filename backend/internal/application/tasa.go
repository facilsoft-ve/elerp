package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/tasa"
)

// Errores de negocio de la tasa de cambio.
var (
	ErrSinTasa               = errors.New("no hay tasa de cambio cargada: sin ella no se pueden convertir montos en divisas")
	ErrTasaInvalida          = errors.New("la tasa debe ser un número mayor que cero")
	ErrTasaFueraDeRango      = errors.New("la tasa está fuera del rango plausible")
	ErrTasaNoEnCuarentena    = errors.New("ese registro de tasa no está en cuarentena")
	ErrSincroDemasiadoPronto = errors.New("la tasa se consultó hace poco; se consulta una vez al día")
	ErrMonedaNoActiva        = errors.New("esa divisa no está activa en la empresa")
)

// intervaloMinimoSincro limita los intentos de obtención (condición 7 de R9:
// una consulta al día por instancia, no una por tenant ni una por petición).
// Un intento forzado desde la interfaz respeta este mínimo para que no nos
// convirtamos en tráfico abusivo contra el BCV.
const intervaloMinimoSincro = time.Hour

// fallosParaAbrirCircuito es el corte del disyuntor: tras tres fallos seguidos
// se espera el descanso largo antes de volver a intentar (condición 5).
const fallosParaAbrirCircuito = 3

// descansoCircuitoAbierto es lo que se espera con el circuito abierto.
const descansoCircuitoAbierto = 6 * time.Hour

// estadoSincro es el estado en memoria del sincronizador. No se persiste: si el
// proceso reinicia, lo peor que pasa es un intento extra.
type estadoSincro struct {
	mu             sync.Mutex
	ultimoIntento  time.Time
	fallosSeguidos int
	ultimoError    string
}

// TasaView es la tasa que consume la interfaz, ya resuelta y rotulada. La
// interfaz NUNCA inventa el rótulo: el origen y la fecha vienen de aquí.
type TasaView struct {
	// Hay indica si existe una tasa usable. En false, la interfaz muestra el
	// estado vacío que dice qué falta, nunca un cero ni una cifra inventada.
	Hay         bool    `json:"hay"`
	Valor       float64 `json:"valor"`
	Fuente      string  `json:"fuente"`
	FuenteLabel string  `json:"fuenteLabel"`
	FechaValor  string  `json:"fechaValor"`
	ObtenidaEn  string  `json:"obtenidaEn"`
	Detalle     string  `json:"detalle"`
	// EsDeHoy distingue «Bs 745,63 · BCV · hoy» de una tasa vieja que se sigue
	// usando porque la fuente no responde. La caja opera igual (offline-first),
	// pero la interfaz dice de cuándo es.
	EsDeHoy bool `json:"esDeHoy"`
	// Oficial es true solo si viene del BCV (directo o republicado): es lo único
	// que la interfaz puede rotular «BCV».
	Oficial bool `json:"oficial"`
	// EnCuarentena avisa que la última lectura de la fuente se rechazó por
	// variación anómala y espera aprobación manual.
	EnCuarentena      bool    `json:"enCuarentena"`
	ValorEnCuarentena float64 `json:"valorEnCuarentena"`
	CuarentenaID      string  `json:"cuarentenaId"`
	MotivoCuarentena  string  `json:"motivoCuarentena"`
	// UltimoError es el fallo más reciente de la sincronización, para poder
	// explicar por qué la tasa no es de hoy.
	UltimoError string `json:"ultimoError"`
}

// etiquetaFuente traduce la fuente al rótulo del prototipo. «BCV» solo aparece
// cuando la cifra realmente vino del BCV.
func etiquetaFuente(f string) string {
	switch f {
	case tasa.FuenteBCV:
		return "BCV"
	case tasa.FuenteRespaldo:
		return "BCV (respaldo)"
	case tasa.FuenteMercado:
		return "Promedio del mercado"
	case tasa.FuenteManual:
		return "Tasa manual"
	case tasa.FuenteSemilla:
		return "Tasa de demostración"
	}
	return "Origen desconocido"
}

// TasaVigente resuelve la tasa que rige para una empresa, respetando la fuente
// que la empresa eligió en el onboarding:
//
//   - bcv / mercado → la tasa de plataforma que trajo la sincronización, salvo
//     que la empresa haya cargado una manual MÁS RECIENTE (el último recurso
//     acordado: si la fuente automática falla, la administradora la carga).
//   - manual → solo la de la empresa.
//
// Devuelve false si no hay ninguna: sin tasa no se inventa un 1.
//
// Es un alias del dólar: TasaVigenteDeMoneda(empresaID, "USD"). El flujo USD
// histórico (POS, facturas, /tasa) llama aquí y se comporta idéntico.
func (s *Service) TasaVigente(empresaID string) (tasa.Tasa, bool) {
	return s.TasaVigenteDeMoneda(empresaID, empresa.MonedaUSD)
}

// TasaVigenteDeMoneda resuelve la tasa vigente de UNA moneda para la empresa:
//
//   - USD → la lógica de siempre: la manual del tenant y/o la de plataforma
//     (BCV/mercado), según la fuente que la empresa eligió, ganando la más
//     reciente. El vacío se trata como USD.
//   - otras divisas (EUR, …) → SOLO fuente MANUAL del tenant: nunca hay BCV para
//     algo que no sea el dólar. La más reciente vigente de esa moneda.
//   - VES → no lleva tasa (es la base, vale 1); devuelve false.
//
// Devuelve false si no hay ninguna: sin tasa no se inventa un 1.
func (s *Service) TasaVigenteDeMoneda(empresaID, moneda string) (tasa.Tasa, bool) {
	if s.tasas == nil {
		return tasa.Tasa{}, false
	}
	moneda = tasa.NormalizarMoneda(moneda)
	if moneda == empresa.MonedaVES {
		return tasa.Tasa{}, false
	}
	if moneda != empresa.MonedaUSD {
		// Divisas distintas del dólar: solo la manual del tenant para esa moneda.
		return s.tasas.UltimaVigenteDeMonedaFuentes(empresaID, moneda, tasa.FuenteManual)
	}
	manual, hayManual := s.tasas.UltimaVigenteDeMonedaFuentes(empresaID, empresa.MonedaUSD)
	if s.fuenteDe(empresaID) == empresa.FuenteTasaManual {
		return manual, hayManual
	}
	auto, hayAuto := s.tasas.UltimaVigenteDeMonedaFuentes("", empresa.MonedaUSD)
	switch {
	case hayManual && hayAuto:
		// Gana la más reciente por instante de obtención: si el BCV volvió a
		// responder después de una carga manual, manda el BCV.
		if manual.ObtenidaEn > auto.ObtenidaEn {
			return manual, true
		}
		return auto, true
	case hayAuto:
		return auto, true
	default:
		return manual, hayManual
	}
}

// fuenteDe devuelve la fuente de tasa configurada por la empresa (bcv por
// defecto: es la que el prototipo trae seleccionada).
func (s *Service) fuenteDe(empresaID string) string {
	if s.empresas == nil {
		return empresa.FuenteTasaBCV
	}
	e, ok := s.empresas.ByID(empresaID)
	if !ok || e.FuenteTasa == "" {
		return empresa.FuenteTasaBCV
	}
	return e.FuenteTasa
}

// TasaDeEmpresa arma la vista de la tasa para una empresa, con su rótulo, su
// antigüedad y la cuarentena pendiente si la hay.
func (s *Service) TasaDeEmpresa(empresaID string) TasaView {
	v := TasaView{}
	if t, ok := s.TasaVigente(empresaID); ok {
		v = TasaView{
			Hay: true, Valor: t.Valor, Fuente: t.Fuente, FuenteLabel: etiquetaFuente(t.Fuente),
			FechaValor: t.FechaValor, ObtenidaEn: t.ObtenidaEn, Detalle: t.Detalle,
			EsDeHoy: t.FechaValor == hoyVE(), Oficial: t.Oficial(),
		}
	}
	// Cuarentena pendiente: solo si el registro de plataforma MÁS RECIENTE es un
	// rechazo. Si después llegó una lectura buena, ya no hay nada que decidir.
	if s.tasas != nil {
		if h := s.tasas.Historial("", 1); len(h) == 1 && h[0].Estado == tasa.EstadoRechazada {
			v.EnCuarentena = true
			v.ValorEnCuarentena = h[0].Valor
			v.CuarentenaID = h[0].ID
			v.MotivoCuarentena = h[0].Motivo
		}
	}
	if s.sincro != nil {
		s.sincro.mu.Lock()
		v.UltimoError = s.sincro.ultimoError
		s.sincro.mu.Unlock()
	}
	return v
}

// HistorialTasa devuelve el histórico visible para la empresa: sus cargas
// manuales y las tasas de plataforma, de más reciente a más antigua.
func (s *Service) HistorialTasa(empresaID string, limite int) []tasa.Tasa {
	if s.tasas == nil {
		return []tasa.Tasa{}
	}
	if limite <= 0 {
		limite = 30
	}
	out := append(s.tasas.Historial(empresaID, limite), s.tasas.Historial("", limite)...)
	// Orden descendente por instante de obtención (cadenas RFC3339 UTC: el
	// orden lexicográfico es el cronológico).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].ObtenidaEn > out[j-1].ObtenidaEn; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > limite {
		out = out[:limite]
	}
	return out
}

// CargarTasaManual registra una tasa del DÓLAR escrita por una administradora.
// Es el ÚLTIMO RECURSO acordado (R9, opción 3): queda con el usuario que la hizo
// y se rotula «Tasa manual», nunca «BCV». Es la firma histórica (retrocompat):
// delega en CargarTasaManualEnMoneda con moneda "USD".
func (s *Service) CargarTasaManual(empresaID, actor, origen string, valor float64, fechaValor string) (tasa.Tasa, error) {
	return s.CargarTasaManualEnMoneda(empresaID, actor, origen, empresa.MonedaUSD, valor, fechaValor)
}

// CargarTasaManualEnMoneda registra una tasa manual de UNA divisa (USD, EUR, …).
// Para divisas distintas del dólar valida que la moneda esté activa en la empresa
// (no se carga la tasa de algo que no se habilitó). El dólar se acepta siempre:
// es la divisa por defecto y no requiere configuración previa.
func (s *Service) CargarTasaManualEnMoneda(empresaID, actor, origen, moneda string, valor float64, fechaValor string) (tasa.Tasa, error) {
	if s.tasas == nil {
		return tasa.Tasa{}, ErrSinTasa
	}
	moneda = tasa.NormalizarMoneda(moneda)
	if moneda != empresa.MonedaUSD && !s.monedaActiva(empresaID, moneda) {
		return tasa.Tasa{}, fmt.Errorf("%w: %s", ErrMonedaNoActiva, moneda)
	}
	if valor <= 0 || math.IsNaN(valor) || math.IsInf(valor, 0) {
		return tasa.Tasa{}, ErrTasaInvalida
	}
	if valor < tasa.MinPlausible || valor > tasa.MaxPlausible {
		return tasa.Tasa{}, fmt.Errorf("%w (entre %g y %g Bs por unidad)", ErrTasaFueraDeRango, tasa.MinPlausible, tasa.MaxPlausible)
	}
	fechaValor = strings.TrimSpace(fechaValor)
	if !fechaISOValida(fechaValor) {
		fechaValor = hoyVE()
	}
	t := s.tasas.Append(tasa.Tasa{
		EmpresaID: empresaID, Moneda: moneda, Valor: valor, Fuente: tasa.FuenteManual,
		FechaValor: fechaValor, ObtenidaEn: ahora(), Actor: actor,
		Detalle: "carga manual", Estado: tasa.EstadoVigente,
	})
	// Condición 8: cada cambio de tasa registra origen, valor, fecha y quién.
	s.audit.Append(evento(empresaID, actor, origen, "tasa.manual",
		fmt.Sprintf("%s %.2f", moneda, valor), fechaValor))
	return t, nil
}

// AprobarTasaEnCuarentena aplica una lectura que la validación había rechazado
// por variación anómala. Existe porque en Venezuela una devaluación real puede
// superar el umbral: la decisión es de la Dueña, y queda auditada con su nombre.
//
// No edita el registro rechazado (el histórico es de solo-anexado): anexa uno
// nuevo, vigente, que referencia el motivo de la aprobación.
func (s *Service) AprobarTasaEnCuarentena(empresaID, actor, origen, id string) (tasa.Tasa, error) {
	if s.tasas == nil {
		return tasa.Tasa{}, ErrSinTasa
	}
	// La cuarentena vive en el ámbito de plataforma: es una lectura de la fuente
	// externa, no de un tenant.
	rechazada, ok := s.tasas.ByID("", id)
	if !ok || rechazada.Estado != tasa.EstadoRechazada {
		return tasa.Tasa{}, ErrTasaNoEnCuarentena
	}
	t := s.tasas.Append(tasa.Tasa{
		Moneda: rechazada.Moneda, Valor: rechazada.Valor, Fuente: rechazada.Fuente,
		FechaValor: rechazada.FechaValor, ObtenidaEn: ahora(), Actor: actor,
		Estado:  tasa.EstadoVigente,
		Detalle: rechazada.Detalle + " · aprobada manualmente",
	})
	s.audit.Append(evento(empresaID, actor, origen, "tasa.cuarentena.aprobar",
		fmt.Sprintf("%.2f", rechazada.Valor), rechazada.Motivo))
	return t, nil
}

// SincronizarTasa intenta traer la tasa de las fuentes externas, en orden: la
// configurada como primaria y luego el respaldo. Nunca devuelve error hacia la
// caja: si todo falla, se conserva la última tasa buena y se registra el fallo
// (condición 5). `forzado` lo usa la administradora desde la interfaz y aun así
// respeta el intervalo mínimo entre consultas (condición 7).
func (s *Service) SincronizarTasa(ctx context.Context, actor, origen string, forzado bool) (TasaView, error) {
	if s.tasas == nil || len(s.proveedores) == 0 {
		return s.TasaDeEmpresa(""), errors.New("la obtención automática de la tasa no está configurada")
	}
	st := s.sincro
	st.mu.Lock()
	espera := intervaloMinimoSincro
	if st.fallosSeguidos >= fallosParaAbrirCircuito {
		espera = descansoCircuitoAbierto // circuito abierto: descanso largo
	}
	if !st.ultimoIntento.IsZero() && time.Since(st.ultimoIntento) < espera {
		st.mu.Unlock()
		if forzado {
			return s.TasaDeEmpresa(""), ErrSincroDemasiadoPronto
		}
		return s.TasaDeEmpresa(""), nil
	}
	st.ultimoIntento = time.Now()
	st.mu.Unlock()

	var errs []string
	for _, prov := range s.proveedores {
		lect, err := prov.Obtener(ctx)
		if err != nil {
			errs = append(errs, prov.Nombre()+": "+err.Error())
			continue
		}
		if _, err := s.registrarLectura(lect, actor, origen); err != nil {
			// La lectura llegó pero no pasó la validación: queda en cuarentena.
			errs = append(errs, prov.Nombre()+": "+err.Error())
			continue
		}
		st.mu.Lock()
		st.fallosSeguidos = 0
		st.ultimoError = ""
		st.mu.Unlock()
		return s.TasaDeEmpresa(""), nil
	}

	st.mu.Lock()
	st.fallosSeguidos++
	st.ultimoError = strings.Join(errs, " · ")
	st.mu.Unlock()
	// El fallo se audita en el ámbito de plataforma para poder explicar después
	// por qué la tasa de un día no se actualizó.
	s.audit.Append(evento("", actor, origen, "tasa.sincronizar.fallo", "", strings.Join(errs, " · ")))
	return s.TasaDeEmpresa(""), nil
}

// horaSincro es la hora local venezolana desde la que se busca la tasa del día.
// El prototipo lo declara en Integraciones: «Se actualiza todos los días
// hábiles a las 9am».
const horaSincro = 9

// SincronizarTasaDiaria es el lazo que cablea cmd/api: se llama cada cierto
// rato y decide si hoy toca consultar. Consulta cuando:
//
//   - no hay ninguna tasa todavía (arranque en limpio), o
//   - la tasa vigente no es del día de hoy en Venezuela y ya pasaron las 9 am.
//
// Así son, como máximo, una o dos consultas al día por instancia — nunca una
// por tenant ni una por petición (condición 7).
func (s *Service) SincronizarTasaDiaria(ctx context.Context) {
	if s.tasas == nil || len(s.proveedores) == 0 {
		return
	}
	ult, hay := s.tasas.UltimaVigente("")
	if hay && ult.FechaValor == hoyVE() {
		return // ya tenemos la de hoy
	}
	if hay && horaVE() < horaSincro {
		return // antes de las 9 am el BCV aún no publicó la del día
	}
	s.SincronizarTasa(ctx, "sistema", "scheduler", false)
}

// registrarLectura valida una lectura externa y la anexa al histórico de
// plataforma. Es la condición 2 de R9: el dato que llega no se confía nunca.
func (s *Service) registrarLectura(l tasa.Lectura, actor, origen string) (tasa.Tasa, error) {
	if l.Valor <= 0 || math.IsNaN(l.Valor) || math.IsInf(l.Valor, 0) {
		return tasa.Tasa{}, ErrTasaInvalida
	}
	fecha := l.FechaValor
	if !fechaISOValida(fecha) {
		fecha = hoyVE()
	}

	motivo := ""
	if l.Valor < tasa.MinPlausible || l.Valor > tasa.MaxPlausible {
		motivo = fmt.Sprintf("fuera del rango plausible (%g–%g Bs/US$)", tasa.MinPlausible, tasa.MaxPlausible)
	} else if ult, ok := s.tasas.UltimaVigenteDeFuentes("", tasa.FuenteBCV, tasa.FuenteRespaldo, tasa.FuenteMercado, tasa.FuenteSemilla); ok && ult.Valor > 0 {
		if variacion := math.Abs(l.Valor-ult.Valor) / ult.Valor; variacion > tasa.VariacionMax {
			motivo = fmt.Sprintf("variación de %.1f%% contra la última conocida (Bs %.2f); el umbral es %.0f%%",
				variacion*100, ult.Valor, tasa.VariacionMax*100)
		}
	}

	if motivo != "" {
		// CUARENTENA: no se aplica sola, pero tampoco se descarta en silencio.
		// Un HTML alterado no puede distorsionar los precios de nadie; y si la
		// devaluación fue real, la Dueña la aprueba.
		rechazada := s.tasas.Append(tasa.Tasa{
			Moneda: tasa.MonedaUSD, Valor: l.Valor, Fuente: l.Fuente, FechaValor: fecha,
			ObtenidaEn: ahora(), Actor: actor, Detalle: l.Detalle,
			Estado: tasa.EstadoRechazada, Motivo: motivo,
		})
		s.audit.Append(evento("", actor, origen, "tasa.rechazada", fmt.Sprintf("%.2f", l.Valor), motivo))
		return rechazada, errors.New("lectura en cuarentena: " + motivo)
	}

	// Si ya tenemos exactamente esta tasa para este día y fuente, no se duplica
	// el histórico: el registro es para los cambios, no para los latidos.
	if ult, ok := s.tasas.UltimaVigente(""); ok && ult.Valor == l.Valor && ult.FechaValor == fecha {
		return ult, nil
	}

	t := s.tasas.Append(tasa.Tasa{
		Moneda: tasa.MonedaUSD, Valor: l.Valor, Fuente: l.Fuente, FechaValor: fecha,
		ObtenidaEn: ahora(), Actor: actor, Detalle: l.Detalle, Estado: tasa.EstadoVigente,
	})
	s.audit.Append(evento("", actor, origen, "tasa.sincronizar", fmt.Sprintf("%.2f", l.Valor), l.Fuente+" "+fecha))
	return t, nil
}

// --- Configuración de moneda de la empresa (R10) ---

// ConfigMoneda son los ajustes de moneda que se eligen en el onboarding y se
// pueden cambiar después en Configuración.
type ConfigMoneda struct {
	MonedaPrincipal string `json:"monedaPrincipal"`
	FuenteTasa      string `json:"fuenteTasa"`
	PreciosEnUsd    bool   `json:"preciosEnUsd"`
	// MonedasActivas es OPCIONAL (multimoneda): las divisas extra habilitadas. Un
	// nil deja la configuración de divisas como estaba (retrocompat: quien no
	// manda el campo no borra nada). Un slice vacío no envuelto en nil sí la vacía.
	MonedasActivas *[]empresa.MonedaActiva `json:"monedasActivas"`
}

// GuardarConfigMoneda fija la moneda principal, la fuente de la tasa, si la
// empresa maneja precios en dólares y —opcionalmente— sus divisas activas.
func (s *Service) GuardarConfigMoneda(empresaID, actor, origen string, in ConfigMoneda) (empresa.Empresa, error) {
	if s.empresas == nil {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	e, ok := s.empresas.ByID(empresaID)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	if !empresa.MonedaValida(in.MonedaPrincipal) {
		return empresa.Empresa{}, errors.New("moneda principal inválida (VES o USD)")
	}
	if !empresa.FuenteTasaValida(in.FuenteTasa) {
		return empresa.Empresa{}, errors.New("fuente de tasa inválida (bcv, mercado o manual)")
	}
	if in.MonedasActivas != nil {
		limpias, err := empresa.NormalizarMonedasActivas(*in.MonedasActivas)
		if err != nil {
			return empresa.Empresa{}, err
		}
		e.MonedasActivas = limpias
	}
	e.MonedaPrincipal = in.MonedaPrincipal
	e.FuenteTasa = in.FuenteTasa
	e.PreciosEnUsd = in.PreciosEnUsd
	out, _ := s.empresas.Update(e)
	s.audit.Append(evento(empresaID, actor, origen, "empresa.config.moneda", in.MonedaPrincipal, in.FuenteTasa))
	if in.MonedasActivas != nil {
		s.audit.Append(evento(empresaID, actor, origen, "config.monedas.actualizar", codigosDe(out.MonedasActivas), ""))
	}
	return out, nil
}

// ActualizarMonedasActivas fija SOLO las divisas activas de la empresa (valida la
// lista blanca y que la fuente no sea bcv salvo para USD). Audita
// `config.monedas.actualizar`.
func (s *Service) ActualizarMonedasActivas(empresaID, actor, origen string, monedas []empresa.MonedaActiva) error {
	if s.empresas == nil {
		return ErrEmpresaNoExiste
	}
	e, ok := s.empresas.ByID(empresaID)
	if !ok {
		return ErrEmpresaNoExiste
	}
	limpias, err := empresa.NormalizarMonedasActivas(monedas)
	if err != nil {
		return err
	}
	e.MonedasActivas = limpias
	out, _ := s.empresas.Update(e)
	s.audit.Append(evento(empresaID, actor, origen, "config.monedas.actualizar", codigosDe(out.MonedasActivas), ""))
	return nil
}

// MonedasActivas devuelve las divisas activas de la empresa. Si no configuró
// ninguna, se comporta como hoy: solo el dólar, con la fuente de tasa de la
// empresa (bcv por defecto).
func (s *Service) MonedasActivas(empresaID string) []empresa.MonedaActiva {
	if s.empresas != nil {
		if e, ok := s.empresas.ByID(empresaID); ok && len(e.MonedasActivas) > 0 {
			return e.MonedasActivas
		}
	}
	return []empresa.MonedaActiva{{Codigo: empresa.MonedaUSD, Fuente: s.fuenteDe(empresaID)}}
}

// monedaActiva indica si una divisa está habilitada en la empresa.
func (s *Service) monedaActiva(empresaID, moneda string) bool {
	moneda = tasa.NormalizarMoneda(moneda)
	for _, m := range s.MonedasActivas(empresaID) {
		if tasa.NormalizarMoneda(m.Codigo) == moneda {
			return true
		}
	}
	return false
}

// TasaMonedaView es la tasa vigente de UNA divisa activa, rotulada, para pintar
// el selector de monedas. `Hay` en false = la divisa está activa pero aún no
// tiene tasa cargada (p. ej. EUR recién habilitado sin tasa manual).
type TasaMonedaView struct {
	Moneda      string  `json:"moneda"`
	Fuente      string  `json:"fuente"`
	FuenteLabel string  `json:"fuenteLabel"`
	Hay         bool    `json:"hay"`
	Valor       float64 `json:"valor"`
	FechaValor  string  `json:"fechaValor"`
	ObtenidaEn  string  `json:"obtenidaEn"`
	Oficial     bool    `json:"oficial"`
	EsDeHoy     bool    `json:"esDeHoy"`
}

// TasasVigentes devuelve la tasa vigente de cada divisa activa de la empresa,
// rotulada, para el selector multimoneda.
func (s *Service) TasasVigentes(empresaID string) []TasaMonedaView {
	activas := s.MonedasActivas(empresaID)
	out := make([]TasaMonedaView, 0, len(activas))
	for _, m := range activas {
		cod := tasa.NormalizarMoneda(m.Codigo)
		v := TasaMonedaView{Moneda: cod, Fuente: m.Fuente, FuenteLabel: etiquetaFuente(m.Fuente)}
		if t, ok := s.TasaVigenteDeMoneda(empresaID, cod); ok {
			v.Hay = true
			v.Valor = t.Valor
			v.Fuente = t.Fuente
			v.FuenteLabel = etiquetaFuente(t.Fuente)
			v.FechaValor = t.FechaValor
			v.ObtenidaEn = t.ObtenidaEn
			v.Oficial = t.Oficial()
			v.EsDeHoy = t.FechaValor == hoyVE()
		}
		out = append(out, v)
	}
	return out
}

// codigosDe arma la lista de códigos separados por coma para la auditoría.
func codigosDe(ms []empresa.MonedaActiva) string {
	cods := make([]string, 0, len(ms))
	for _, m := range ms {
		cods = append(cods, m.Codigo)
	}
	return strings.Join(cods, ",")
}

// GuardarConfigSeguridad activa o apaga el PIN de supervisor de caja (flujo 2.4).
// Queda auditado: es un interruptor que cambia qué puede hacer el cajero solo.
func (s *Service) GuardarConfigSeguridad(empresaID, actor, origen string, requiere bool) (empresa.Empresa, error) {
	if s.empresas == nil {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	e, ok := s.empresas.ByID(empresaID)
	if !ok {
		return empresa.Empresa{}, ErrEmpresaNoExiste
	}
	e.RequiereSupervisorPin = requiere
	out, _ := s.empresas.Update(e)
	estado := "off"
	if requiere {
		estado = "on"
	}
	s.audit.Append(evento(empresaID, actor, origen, "empresa.config.seguridad_caja", estado, ""))
	return out, nil
}

// AuditarCambioDeRol registra que a alguien se le cambió el rol o la sede. Vive
// acá porque el registro de auditoría es del Service, no del TenancyService.
func (s *Service) AuditarCambioDeRol(empresaID, actor, origen, nombre, rol, sedeID string) {
	s.audit.Append(evento(empresaID, actor, origen, "usuario.rol", nombre, rol+" "+sedeID))
}

// MonedaPrincipalDe devuelve la moneda en la que la empresa piensa sus precios.
func (s *Service) MonedaPrincipalDe(empresaID string) string {
	if s.empresas == nil {
		return empresa.MonedaVES
	}
	e, ok := s.empresas.ByID(empresaID)
	if !ok || e.MonedaPrincipal == "" {
		return empresa.MonedaVES
	}
	return e.MonedaPrincipal
}

// --- Helpers de fecha ---

// zonaVE es la hora legal de Venezuela (UTC-4). Se usa para decidir a qué DÍA
// pertenece una tasa: el BCV publica por día calendario venezolano, y una
// consulta a las 21:00 de UTC-4 no debe contarse como del día siguiente.
var zonaVE = time.FixedZone("VET", -4*60*60)

// hoyVE devuelve la fecha de hoy en Venezuela, YYYY-MM-DD.
func hoyVE() string { return time.Now().In(zonaVE).Format("2006-01-02") }

// horaVE devuelve la hora local venezolana (0–23).
func horaVE() int { return time.Now().In(zonaVE).Hour() }

// esDelMesActual indica si una marca de tiempo cae en el mes en curso, en hora de
// Venezuela: agrupar por mes con UTC movería los cobros de fin de mes de la noche.
func esDelMesActual(fecha string) bool {
	if len(fecha) < 7 {
		return false
	}
	t, err := time.Parse(time.RFC3339, fecha)
	if err != nil {
		return false
	}
	local := t.In(zonaVE)
	ahoraVE := time.Now().In(zonaVE)
	return local.Year() == ahoraVE.Year() && local.Month() == ahoraVE.Month()
}

// diasEntre cuenta los días completos entre dos fechas YYYY-MM-DD (b − a).
func diasEntre(a, b string) int {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

// enDias devuelve la fecha (YYYY-MM-DD, hora de Venezuela) dentro de n días. Se
// usa para el vencimiento de una venta a crédito.
func enDias(n int) string {
	return time.Now().In(zonaVE).AddDate(0, 0, n).Format("2006-01-02")
}

// fechaISOValida valida el formato YYYY-MM-DD sin aceptar nada más.
func fechaISOValida(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
