package application_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

/* CORRELATIVOS CONFIGURABLES.
 *
 * Lo que se prueba acá no es que se puedan guardar tres campos: es que la
 * configuración NO pueda romper la integridad del correlativo. Un folio emitido
 * no vuelve, aunque el documento se anule, y ningún camino de esta pantalla
 * puede reasignarlo. */

func tipoDe(estado application.EstadoNumeracionView, tipo string) (application.TipoNumeracion, bool) {
	for _, t := range estado.Tipos {
		if t.Tipo == tipo {
			return t, true
		}
	}
	return application.TipoNumeracion{}, false
}

// La pantalla tiene que servir ANTES de emitir nada: una empresa que migra
// necesita declarar «mi factura arranca en 1500» antes de vender, que es
// justamente cuando antes no existía la fila.
func TestNumeracion_ListaLosTiposSinHaberEmitidoNada(t *testing.T) {
	svc, _ := nuevoServicio(t)
	estado := svc.EstadoNumeracion(empDemo)
	for _, quiero := range []string{
		fiscal.SerieFactura, fiscal.SerieNotaCredito, fiscal.SerieNotaDebito,
		fiscal.SerieAnulacion, fiscal.SerieNumeroControl,
		fiscal.SerieRetencionIVA, fiscal.SerieRetencionISLR,
	} {
		f, ok := tipoDe(estado, quiero)
		if !ok {
			t.Fatalf("falta el tipo %q en el estado de numeración", quiero)
		}
		if len(f.Contadores) == 0 {
			t.Fatalf("el tipo %q debe traer al menos un contador", quiero)
		}
	}
}

// Sin configurar nada, se numera EXACTAMENTE como antes. Un correlativo que
// cambia solo al actualizar el sistema es un problema fiscal, no un detalle.
func TestNumeracion_SinConfigurarNumeraComoAntes(t *testing.T) {
	svc, _ := nuevoServicio(t)
	estado := svc.EstadoNumeracion(empDemo)
	f, _ := tipoDe(estado, fiscal.SerieFactura)
	if f.Prefijo != "FL" {
		t.Fatalf("prefijo por defecto = %q, se esperaba FL (forma libre)", f.Prefijo)
	}
	nc, _ := tipoDe(estado, fiscal.SerieNotaCredito)
	if nc.Prefijo != "FL-NC" {
		t.Fatalf("prefijo por defecto de la NC = %q, se esperaba FL-NC", nc.Prefijo)
	}
}

// El prefijo configurado manda al emitir: es el punto de toda la pantalla.
func TestNumeracion_ElPrefijoConfiguradoSeUsaAlEmitir(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieFactura, "A-01", 1500, 0); err != nil {
		t.Fatalf("configurar la serie: %v", err)
	}
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if doc.Serie != "A-01" {
		t.Fatalf("serie = %q, se esperaba la configurada A-01", doc.Serie)
	}
	// Y arranca donde dice el rango, no en 1.
	if doc.Numero != 1500 {
		t.Fatalf("número = %d, el rango empezaba en 1500", doc.Numero)
	}
	if !strings.HasPrefix(doc.NumeroCompleto, "A-01-") {
		t.Fatalf("número completo = %q, debía llevar la serie configurada", doc.NumeroCompleto)
	}
}

// LA prueba de integridad: cambiar el prefijo no puede reasignar folios ya
// usados, y los de la serie anterior quedan donde estaban.
func TestNumeracion_CambiarElPrefijoAbreUnaSerieNueva(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	emitir := func() fiscal.Documento {
		t.Helper()
		d, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
			Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
			Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
		})
		if err != nil {
			t.Fatalf("emitir: %v", err)
		}
		return d
	}
	vieja := emitir()
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieFactura, "B", 0, 0); err != nil {
		t.Fatalf("cambiar el prefijo: %v", err)
	}
	nueva := emitir()

	if nueva.Serie != "B" {
		t.Fatalf("serie nueva = %q, se esperaba B", nueva.Serie)
	}
	// La serie nueva arranca en 1: es OTRA serie, no la misma renombrada.
	if nueva.Numero != 1 {
		t.Fatalf("la serie nueva debía arrancar en 1, arrancó en %d", nueva.Numero)
	}
	// Y el documento viejo conserva el suyo: es inmutable.
	if vieja.Serie != "FL" || !strings.HasPrefix(vieja.NumeroCompleto, "FL-") {
		t.Fatalf("el documento anterior cambió de serie: %q", vieja.NumeroCompleto)
	}
	// El contador de la serie abandonada sigue visible, para que nadie crea que
	// esos folios se perdieron.
	visto := false
	for _, o := range svc.EstadoNumeracion(empDemo).Otras {
		if o.Serie == "FL" {
			visto = true
		}
	}
	if !visto {
		t.Fatal("la serie anterior debe seguir apareciendo con sus folios")
	}
}

// El rango no puede retroceder el contador: posicionar en `desde` solo avanza.
func TestNumeracion_ElRangoNoRetrocedeElContador(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieFactura, "", 900, 1000); err != nil {
		t.Fatalf("configurar rango: %v", err)
	}
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if doc.Numero != 900 {
		t.Fatalf("número = %d, el rango arrancaba en 900", doc.Numero)
	}
	// Volver a declarar un rango MÁS BAJO no puede devolver el contador: ese folio
	// ya se usó.
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieFactura, "", 100, 1000); err != nil {
		t.Fatalf("reconfigurar: %v", err)
	}
	f, _ := tipoDe(svc.EstadoNumeracion(empDemo), fiscal.SerieFactura)
	if f.Contadores[0].Proximo <= 900 {
		t.Fatalf("el contador retrocedió a %d: un folio usado no se reasigna", f.Contadores[0].Proximo)
	}
}

// «Restantes» es lo que evita quedarse sin folios autorizados en plena venta.
func TestNumeracion_RestantesDelRango(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// El rango se declara POR ENCIMA del contador vivo: la empresa demo ya tiene
	// folios emitidos y un rango por debajo no puede —ni debe— retroceder nada.
	antes, _ := tipoDe(svc.EstadoNumeracion(empDemo), fiscal.SerieFactura)
	desde := antes.Contadores[0].Proximo + 10
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieFactura, "", desde, desde+2); err != nil {
		t.Fatalf("configurar: %v", err)
	}
	f, _ := tipoDe(svc.EstadoNumeracion(empDemo), fiscal.SerieFactura)
	if f.Contadores[0].Proximo != desde {
		t.Fatalf("próximo = %d, el rango arrancaba en %d", f.Contadores[0].Proximo, desde)
	}
	// Contador en desde-1, tope desde+2 ⇒ quedan 3 folios.
	if f.Contadores[0].Restantes != 3 {
		t.Fatalf("restantes = %d, se esperaban 3", f.Contadores[0].Restantes)
	}
}

// Y un rango declarado POR DEBAJO de lo emitido no deja el contador en un número
// ya usado: se queda donde está y «restantes» avisa que el rango está agotado.
func TestNumeracion_RangoYaAgotado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	antes, _ := tipoDe(svc.EstadoNumeracion(empDemo), fiscal.SerieFactura)
	if antes.Contadores[0].Actual == 0 {
		t.Skip("la empresa demo no tiene folios emitidos en esta serie")
	}
	tope := antes.Contadores[0].Actual - 1
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieFactura, "", 1, tope); err != nil {
		t.Fatalf("configurar: %v", err)
	}
	f, _ := tipoDe(svc.EstadoNumeracion(empDemo), fiscal.SerieFactura)
	if f.Contadores[0].Proximo != antes.Contadores[0].Proximo {
		t.Fatalf("el contador se movió a %d: un folio usado no se reasigna", f.Contadores[0].Proximo)
	}
	if f.Contadores[0].Restantes != 0 {
		t.Fatalf("restantes = %d, el rango ya está agotado", f.Contadores[0].Restantes)
	}
}

// Lo que la ley fija no se ofrece como configurable: quien vea un campo va a
// creer que sirve.
func TestNumeracion_LoQueFijaElSeniatNoSeConfigura(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// Las retenciones no llevan prefijo elegible (el formato es AAAAMM+8).
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieRetencionIVA, "MIRET", 0, 0); !errors.Is(err, application.ErrPrefijoFijo) {
		t.Fatalf("un prefijo en retenciones debe dar ErrPrefijoFijo, se obtuvo: %v", err)
	}
	// Ni rango: reinician cada mes, no hay tope que declarar.
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieRetencionISLR, "", 1, 100); !errors.Is(err, application.ErrRangoSerie) {
		t.Fatalf("un rango en retenciones debe rechazarse, se obtuvo: %v", err)
	}
	// Un tipo inventado tampoco entra.
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, "recibo_de_cumpleanos", "X", 0, 0); !errors.Is(err, application.ErrTipoSerie) {
		t.Fatalf("un tipo fuera del catálogo debe dar ErrTipoSerie, se obtuvo: %v", err)
	}
}

// El prefijo se acota: la barra vertical es el separador de la clave del
// contador, y dejarla pasar partiría la clave y el correlativo terminaría en
// otra serie sin que nadie lo note.
func TestNumeracion_PrefijoAcotado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	for _, malo := range []string{"A|B", "serie con espacios", "DEMASIADO-LARGO-12", "A/B"} {
		if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieFactura, malo, 0, 0); !errors.Is(err, application.ErrPrefijoSerie) {
			t.Fatalf("el prefijo %q debía rechazarse, se obtuvo: %v", malo, err)
		}
	}
	for _, bueno := range []string{"A", "FL", "A-01", "FACT2026"} {
		if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieFactura, bueno, 0, 0); err != nil {
			t.Fatalf("el prefijo %q debía aceptarse: %v", bueno, err)
		}
	}
}

// Las retenciones se muestran por PERÍODO: su correlativo reinicia cada mes, así
// que lo que se configura es el secuencial del mes en curso.
func TestNumeracion_RetencionesPorPeriodo(t *testing.T) {
	svc, _ := nuevoServicio(t)
	f, ok := tipoDe(svc.EstadoNumeracion(empDemo), fiscal.SerieRetencionIVA)
	if !ok {
		t.Fatal("falta el bloque de retención de IVA")
	}
	if !f.Mensual || f.Periodo == "" {
		t.Fatalf("la retención debe declararse mensual y con período: mensual=%v periodo=%q", f.Mensual, f.Periodo)
	}
	if len(f.Contadores) != 1 || !strings.Contains(f.Contadores[0].Serie, "RETIVA") {
		t.Fatalf("el contador debe ser el del período en curso, es %+v", f.Contadores)
	}
	// El ejemplo lleva el formato del SENIAT: AAAAMM + 8 dígitos = 14 posiciones.
	if len(f.Contadores[0].Ejemplo) != 14 {
		t.Fatalf("ejemplo = %q, el comprobante SENIAT tiene 14 posiciones", f.Contadores[0].Ejemplo)
	}
}

// El número de control conserva su validación propia: dos dígitos, ni una letra.
// Es formato del SENIAT, no una preferencia de la empresa.
func TestNumeracion_NumeroControlSoloDosDigitos(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieNumeroControl, "AB", 0, 0); err == nil {
		t.Fatal("un prefijo con letras debería rechazarse en el número de control")
	}
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieNumeroControl, "123", 0, 0); err == nil {
		t.Fatal("un prefijo de tres dígitos debería rechazarse")
	}
	if err := svc.ConfigurarSerie(empDemo, actorA, origenTst, fiscal.SerieNumeroControl, "01", 0, 0); err != nil {
		t.Fatalf("«01» es un prefijo válido: %v", err)
	}
	f, _ := tipoDe(svc.EstadoNumeracion(empDemo), fiscal.SerieNumeroControl)
	if f.Prefijo != "01" {
		t.Fatalf("prefijo = %q, se esperaba 01", f.Prefijo)
	}
	// Y su formato es NN-NNNNNNNN: 11 posiciones.
	if len(f.Contadores[0].Ejemplo) != 11 {
		t.Fatalf("ejemplo = %q, el número de control es NN-NNNNNNNN", f.Contadores[0].Ejemplo)
	}
}
