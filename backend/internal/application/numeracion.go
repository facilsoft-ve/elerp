package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* CORRELATIVOS FISCALES (Configuración › Series y numeración).
 *
 * El contador vivo lo lleva el Numerador: «último folio entregado» por clave
 * empresa|sede|serie, atómico y forward-only. Esta capa expone tres cosas:
 *
 *   1. VER el estado de cada tipo de documento, exista o no su contador. Antes
 *      la fila aparecía recién DESPUÉS de emitir el primer documento, que es
 *      justo cuando ya no sirve: una empresa que migra necesita declarar «mi
 *      factura arranca en 1500» ANTES de vender.
 *   2. CONFIGURAR el prefijo y el rango autorizado de cada tipo.
 *   3. FIJAR el próximo folio, SOLO hacia adelante.
 *
 * Nunca se retrocede ni se reasigna un folio ya usado (Principio 2, integridad
 * fiscal): un número emitido no vuelve, aunque el documento se anule.
 */

var (
	// ErrProximoInvalido: el próximo folio pedido no es un entero positivo.
	ErrProximoInvalido = errors.New("el próximo folio debe ser un número mayor o igual a 1")
	// ErrTipoSerie: el tipo de documento no está en el catálogo.
	ErrTipoSerie = errors.New("ese tipo de documento no tiene correlativo configurable")
	// ErrPrefijoSerie: el prefijo no cumple el formato.
	ErrPrefijoSerie = errors.New("el prefijo debe tener de 1 a 12 caracteres: letras, números o guion")
	// ErrPrefijoFijo: hay tipos cuyo formato lo fija el SENIAT y no se elige.
	ErrPrefijoFijo = errors.New("el formato de ese comprobante lo fija el SENIAT: no lleva prefijo configurable")
	// ErrRangoSerie: el rango autorizado es incoherente.
	ErrRangoSerie = errors.New("el rango es inválido: 'hasta' debe ser mayor o igual que 'desde'")
	// ErrSeriesNoDisponible: no se cableó el repositorio de configuración.
	ErrSeriesNoDisponible = errors.New("la configuración de correlativos no está disponible")
)

// ContadorSerie es el estado del contador de una serie concreta. Los documentos
// de venta se numeran POR SEDE, así que un mismo tipo tiene un contador por cada
// una: mostrarlos juntos es lo que deja ver que la sede nueva arranca en 1.
type ContadorSerie struct {
	SedeID     string `json:"sedeId"`
	SedeNombre string `json:"sedeNombre"`
	Serie      string `json:"serie"`
	Actual     int    `json:"actual"`  // último folio entregado (0 = sin emitir)
	Proximo    int    `json:"proximo"` // el que recibirá el próximo documento
	// Restantes del rango autorizado; -1 cuando no hay tope declarado.
	Restantes int `json:"restantes"`
	// Ejemplo es cómo se verá el próximo documento, con su formato real.
	Ejemplo string `json:"ejemplo"`
}

// TipoNumeracion agrupa la configuración de un tipo con sus contadores.
type TipoNumeracion struct {
	Tipo            string          `json:"tipo"`
	Nombre          string          `json:"nombre"`
	Ayuda           string          `json:"ayuda"`
	Prefijo         string          `json:"prefijo"`
	PrefijoEditable bool            `json:"prefijoEditable"`
	ConRango        bool            `json:"conRango"`
	PorSede         bool            `json:"porSede"`
	Mensual         bool            `json:"mensual"`
	Desde           int             `json:"desde"`
	Hasta           int             `json:"hasta"`
	Periodo         string          `json:"periodo,omitempty"` // "2026-09" en los mensuales
	Contadores      []ContadorSerie `json:"contadores"`
}

// EstadoNumeracionView es lo que consume la pantalla: los tipos fiscales
// configurables y, aparte, las series internas que también llevan folio
// (cotizaciones, órdenes de compra, transferencias, cierres Z). Están separadas
// porque no son documentos fiscales y confundirlas invitaría a «configurar» algo
// que el SENIAT no mira.
type EstadoNumeracionView struct {
	Tipos []TipoNumeracion  `json:"tipos"`
	Otras []SerieNumeracion `json:"otras"`
}

// SerieNumeracion es una serie interna descubierta del contador.
type SerieNumeracion struct {
	SedeID     string `json:"sedeId"`
	SedeNombre string `json:"sedeNombre"`
	Serie      string `json:"serie"`
	Actual     int    `json:"actual"`
	Proximo    int    `json:"proximo"`
}

// modalidadDe es la modalidad de facturación declarada por la empresa; es la que
// decide el prefijo por defecto de sus documentos de venta.
func (s *Service) modalidadDe(empresaID string) string {
	if emp, ok := s.empresas.ByID(empresaID); ok && emp.Modalidad != "" {
		return emp.Modalidad
	}
	return "forma_libre"
}

// configuracionSerie devuelve la configuración guardada de un tipo (vacía si no
// se configuró nunca).
func (s *Service) configuracionSerie(empresaID, tipo string) fiscal.SerieDocumento {
	if s.series == nil {
		return fiscal.SerieDocumento{}
	}
	cfg, _ := s.series.Get(empresaID, tipo)
	return cfg
}

// serieRetencion arma la serie del contador de retenciones del período en curso.
// Lleva el período dentro porque el correlativo REINICIA cada mes por
// providencia: son contadores distintos, no el mismo avanzando.
func serieRetencion(tipo, aaaamm string) string {
	impuesto := "IVA"
	if tipo == fiscal.SerieRetencionISLR {
		impuesto = "ISLR"
	}
	return "RET" + impuesto + aaaamm
}

// periodoActual es el "YYYYMM" de hoy, y su forma legible "YYYY-MM".
func periodoActual() (aaaamm, legible string) {
	hoy := ahora()
	return hoy[0:4] + hoy[5:7], hoy[0:7]
}

// EstadoNumeracion arma el estado completo de los correlativos de la empresa.
func (s *Service) EstadoNumeracion(empresaID string) EstadoNumeracionView {
	out := EstadoNumeracionView{Tipos: []TipoNumeracion{}, Otras: []SerieNumeracion{}}
	if s.numerador == nil {
		return out
	}
	modalidad := s.modalidadDe(empresaID)
	aaaamm, periodoLegible := periodoActual()

	// Sedes de la empresa: los documentos de venta numeran por sede, y hay que
	// listarlas TODAS —no solo las que ya emitieron— o la sede recién abierta no
	// aparecería hasta su primera venta, que es cuando ya no se puede preparar.
	sedes := s.sedesDeEmpresa(empresaID)
	usadas := map[string]bool{} // claves sede|serie ya mostradas en un tipo

	for _, t := range fiscal.TiposSerie() {
		cfg := s.configuracionSerie(empresaID, t.Tipo)
		fila := TipoNumeracion{
			Tipo: t.Tipo, Nombre: t.Nombre, Ayuda: t.Ayuda,
			PrefijoEditable: t.PrefijoEditable, ConRango: t.ConRango,
			PorSede: t.PorSede, Mensual: t.Mensual,
			Desde: cfg.Desde, Hasta: cfg.Hasta,
			Contadores: []ContadorSerie{},
		}

		switch {
		case t.Tipo == fiscal.SerieNumeroControl:
			// El número de control vive en la empresa desde antes de esta pantalla:
			// se lee de ahí para no tener dos fuentes de verdad del mismo dato.
			emp, _ := s.empresas.ByID(empresaID)
			fila.Prefijo = s.prefijoNumeroControl(empresaID)
			fila.Desde, fila.Hasta = emp.NumeroControlDesde, emp.NumeroControlHasta
			fila.Contadores = append(fila.Contadores,
				s.contadorDe(empresaID, "", "", serieControl, fila.Hasta, formatoControl(fila.Prefijo)))
			usadas[""+"|"+serieControl] = true

		case t.Mensual:
			fila.Prefijo = "AAAAMM"
			fila.Periodo = periodoLegible
			serie := serieRetencion(t.Tipo, aaaamm)
			fila.Contadores = append(fila.Contadores,
				s.contadorDe(empresaID, "", "", serie, 0, formatoRetencion(aaaamm)))
			usadas[""+"|"+serie] = true

		default:
			fila.Prefijo = s.serieDoc(empresaID, t.Tipo, modalidad)
			for _, sd := range sedes {
				fila.Contadores = append(fila.Contadores,
					s.contadorDe(empresaID, sd.id, sd.nombre, fila.Prefijo, fila.Hasta, formatoFolio(fila.Prefijo)))
				usadas[sd.id+"|"+fila.Prefijo] = true
			}
		}
		out.Tipos = append(out.Tipos, fila)
	}

	// Series internas descubiertas del contador (COT, OC, TRF, Z, SOL…) y las
	// series fiscales ABANDONADAS por un cambio de prefijo: se muestran para que
	// nadie crea que los folios emitidos bajo el prefijo anterior se perdieron.
	prefijo := empresaID + "|"
	for key, ultimo := range s.numerador.Series(empresaID) {
		resto := strings.TrimPrefix(key, prefijo)
		sedeID, serie, ok := strings.Cut(resto, "|")
		if !ok || usadas[sedeID+"|"+serie] {
			continue
		}
		out.Otras = append(out.Otras, SerieNumeracion{
			SedeID: sedeID, SedeNombre: s.nombreSede(sedes, sedeID), Serie: serie,
			Actual: ultimo, Proximo: ultimo + 1,
		})
	}
	sort.Slice(out.Otras, func(i, j int) bool {
		if out.Otras[i].SedeID != out.Otras[j].SedeID {
			return out.Otras[i].SedeID < out.Otras[j].SedeID
		}
		return out.Otras[i].Serie < out.Otras[j].Serie
	})
	return out
}

// contadorDe arma el estado de un contador concreto.
func (s *Service) contadorDe(empresaID, sedeID, sedeNombre, serie string, hasta int, formato func(int) string) ContadorSerie {
	actual := s.numerador.Actual(empresaID, sedeID, serie)
	c := ContadorSerie{
		SedeID: sedeID, SedeNombre: sedeNombre, Serie: serie,
		Actual: actual, Proximo: actual + 1, Restantes: -1,
		Ejemplo: formato(actual + 1),
	}
	if hasta > 0 {
		if c.Restantes = hasta - actual; c.Restantes < 0 {
			c.Restantes = 0
		}
	}
	return c
}

func formatoFolio(serie string) func(int) string {
	return func(n int) string { return fmt.Sprintf("%s-%08d", serie, n) }
}

func formatoControl(prefijo string) func(int) string {
	return func(n int) string { return fmt.Sprintf("%s-%08d", prefijo, n) }
}

func formatoRetencion(aaaamm string) func(int) string {
	return func(n int) string { return fmt.Sprintf("%s%08d", aaaamm, n) }
}

type sedeRef struct{ id, nombre string }

// sedesDeEmpresa lista las sedes activas. Sin el repositorio cableado devuelve
// una sede vacía, que es como se comportaba antes: un solo contador por serie.
func (s *Service) sedesDeEmpresa(empresaID string) []sedeRef {
	if s.sedes == nil {
		return []sedeRef{{id: "", nombre: ""}}
	}
	out := []sedeRef{}
	for _, sd := range s.sedes.List(empresaID) {
		if sd.Activa {
			out = append(out, sedeRef{id: sd.ID, nombre: sd.Nombre})
		}
	}
	if len(out) == 0 {
		out = append(out, sedeRef{id: "", nombre: ""})
	}
	return out
}

func (s *Service) nombreSede(sedes []sedeRef, id string) string {
	for _, sd := range sedes {
		if sd.id == id {
			return sd.nombre
		}
	}
	return id
}

// ConfigurarSerie fija el prefijo y el rango autorizado de un tipo de documento.
//
// CAMBIAR EL PREFIJO ABRE UNA SERIE NUEVA. Los documentos ya emitidos conservan
// el suyo —son inmutables, y su número completo es parte del documento—, así que
// el contador del prefijo nuevo arranca donde diga `desde` (o en 1). Los folios
// de la serie anterior quedan como estaban: ni se reutilizan ni se pierden.
func (s *Service) ConfigurarSerie(empresaID, actor, origen, tipo, prefijo string, desde, hasta int) error {
	t, ok := fiscal.TipoSerieDe(tipo)
	if !ok {
		return ErrTipoSerie
	}
	if desde < 0 || hasta < 0 || (hasta > 0 && desde > 0 && hasta < desde) {
		return ErrRangoSerie
	}
	if !t.ConRango && (desde > 0 || hasta > 0) {
		// Un correlativo que reinicia cada mes no tiene rango que declarar.
		return ErrRangoSerie
	}
	prefijo = strings.TrimSpace(prefijo)
	if prefijo != "" {
		if !t.PrefijoEditable {
			return ErrPrefijoFijo
		}
		if !fiscal.PrefijoValido(prefijo) {
			return ErrPrefijoSerie
		}
	}

	// El número de control conserva su almacenamiento propio (campos de la
	// empresa) y su validación de prefijo de 2 dígitos.
	if tipo == fiscal.SerieNumeroControl {
		_, err := s.ConfigurarNumeroControl(empresaID, actor, origen, prefijo, desde, hasta)
		return err
	}
	if s.series == nil {
		return ErrSeriesNoDisponible
	}

	serie := prefijo
	if serie == "" {
		serie = s.serieDoc(empresaID, tipo, s.modalidadDe(empresaID))
	}
	// Posicionar el contador al inicio del rango, en cada sede y solo hacia
	// adelante: si una sede ya pasó ese número, se la deja donde está en vez de
	// fallar. Bloquear el guardado por una sede adelantada obligaría a configurar
	// sede por sede un dato que es de la empresa.
	if desde > 0 && s.numerador != nil && t.PorSede {
		for _, sd := range s.sedesDeEmpresa(empresaID) {
			if s.numerador.Actual(empresaID, sd.id, serie) < desde-1 {
				_ = s.numerador.Fijar(empresaID, sd.id, serie, desde-1)
			}
		}
	}

	out := fiscal.SerieDocumento{
		EmpresaID: empresaID, Tipo: tipo, Prefijo: prefijo,
		Desde: desde, Hasta: hasta, Actualizada: ahora(),
	}
	s.series.Upsert(out)
	s.audit.Append(evento(empresaID, actor, origen, "config.numeracion.serie", tipo,
		fmt.Sprintf("prefijo %q · rango %d–%d", prefijo, desde, hasta)))
	return nil
}

// FijarNumeracion fija el próximo folio de una serie SOLO hacia adelante: exige
// que `proximo` sea >= 1 y estrictamente mayor que el último folio ya entregado
// (si no, ErrFolioRetrocede). Guarda `proximo-1` como último entregado, de modo
// que la próxima emisión reciba exactamente `proximo`. Audita el cambio.
func (s *Service) FijarNumeracion(empresaID, actor, origen, sedeID, serie string, proximo int) error {
	if proximo < 1 {
		return ErrProximoInvalido
	}
	// Forward-only en la capa de aplicación: el próximo folio tiene que superar al
	// último entregado. El adaptador vuelve a validarlo (defensa en profundidad).
	if proximo <= s.numerador.Actual(empresaID, sedeID, serie) {
		return fiscal.ErrFolioRetrocede
	}
	if err := s.numerador.Fijar(empresaID, sedeID, serie, proximo-1); err != nil {
		return err
	}
	s.audit.Append(evento(empresaID, actor, origen, "config.numeracion.fijar",
		serie, fmt.Sprintf("sede %s · serie %s · próximo folio %d", sedeID, serie, proximo)))
	return nil
}
