package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/tasa"
)

// proveedorFalso simula una fuente externa sin salir a internet.
type proveedorFalso struct {
	nombre string
	lect   tasa.Lectura
	err    error
	// llamadas cuenta cuántas veces se consultó: sostiene la prueba de que no
	// martillamos la fuente.
	llamadas int
}

func (p *proveedorFalso) Nombre() string { return p.nombre }
func (p *proveedorFalso) Obtener(context.Context) (tasa.Lectura, error) {
	p.llamadas++
	return p.lect, p.err
}

// servicioSinTasa arma un servicio con el histórico de tasas VACÍO, para probar
// qué hace la caja el día en que no hay ninguna tasa cargada. El seed demo
// siempre siembra una, así que aquí se cablea un histórico limpio en su lugar.
func servicioSinTasa(t *testing.T) *application.Service {
	t.Helper()
	st := inmem.New()
	return application.New(
		st.Productos, st.Movimientos, st.Transferencias, st.Rubros, st.Audit,
		st.Documentos, st.Numerador, st.CuentasCobro, st.MetodosPago, st.Clientes, st.Cotizaciones,
		st.Cajas, st.Cajeros, st.SesionesCaja,
		inmem.NewTasaRepo(), st.Empresas, st.VentasEnEspera, st.Cobros,
		st.CuentasContables, st.Asientos, st.Periodos,
		st.Proveedores, st.OrdenesCompra, st.CierresZ,
		st.FacturasCompra, st.PagosProveedor, st.Retenciones,
		st.Dispositivos,
	)
}

// conFuente arma un servicio con una fuente externa controlada.
func conFuente(t *testing.T, provs ...tasa.Proveedor) *application.Service {
	t.Helper()
	svc, _ := nuevoServicio(t)
	return svc.ConFuentesDeTasa(provs...)
}

func TestTasaVigente_LaSemillaNoSeRotulaBCV(t *testing.T) {
	svc, _ := nuevoServicio(t)
	v := svc.TasaDeEmpresa(empDemo)
	if !v.Hay {
		t.Fatal("el modo demo debe traer una tasa sembrada")
	}
	// Regla del cliente: nunca se rotula «BCV» una cifra que nadie trajo del
	// Banco Central.
	if v.Oficial || v.FuenteLabel == "BCV" {
		t.Errorf("la tasa sembrada no puede presentarse como oficial: %+v", v)
	}
}

func TestSincronizarTasa_AceptaLecturaPlausible(t *testing.T) {
	// Una variación pequeña contra la semilla (745,63 → 760,00) se acepta.
	prov := &proveedorFalso{nombre: tasa.FuenteBCV, lect: tasa.Lectura{
		Valor: 760, FechaValor: "2026-07-31", Fuente: tasa.FuenteBCV, Detalle: "www.bcv.org.ve",
	}}
	svc := conFuente(t, prov)
	if _, err := svc.SincronizarTasa(context.Background(), "sistema", origenTst, false); err != nil {
		t.Fatalf("sincronizar: %v", err)
	}
	v := svc.TasaDeEmpresa(empDemo)
	if v.Valor != 760 || !v.Oficial {
		t.Fatalf("la tasa del BCV debía quedar vigente y rotulada oficial: %+v", v)
	}
}

func TestSincronizarTasa_VariacionAnomalaQuedaEnCuarentena(t *testing.T) {
	// El caso que protege a todos los clientes: un HTML alterado (o un
	// intermediario comprometido) devuelve una tasa absurda. No se aplica sola…
	prov := &proveedorFalso{nombre: tasa.FuenteBCV, lect: tasa.Lectura{
		Valor: 99999, FechaValor: "2026-07-31", Fuente: tasa.FuenteBCV,
	}}
	svc := conFuente(t, prov)
	svc.SincronizarTasa(context.Background(), "sistema", origenTst, false)

	v := svc.TasaDeEmpresa(empDemo)
	if v.Valor == 99999 {
		t.Fatal("una variación anómala no puede aplicarse sola")
	}
	// …pero tampoco se descarta en silencio: queda visible para decidir.
	if !v.EnCuarentena || v.ValorEnCuarentena != 99999 || v.CuarentenaID == "" {
		t.Fatalf("la lectura rechazada debía quedar en cuarentena: %+v", v)
	}
	if v.MotivoCuarentena == "" {
		t.Error("la cuarentena debe explicar por qué se rechazó")
	}
}

func TestAprobarTasaEnCuarentena_LaAplicaSinEditarElRechazo(t *testing.T) {
	prov := &proveedorFalso{nombre: tasa.FuenteBCV, lect: tasa.Lectura{Valor: 1200, Fuente: tasa.FuenteBCV}}
	svc := conFuente(t, prov)
	svc.SincronizarTasa(context.Background(), "sistema", origenTst, false)
	v := svc.TasaDeEmpresa(empDemo)
	if !v.EnCuarentena {
		t.Fatal("precondición: la lectura debía quedar en cuarentena")
	}

	// Una devaluación real por encima del umbral: la Dueña la aprueba.
	if _, err := svc.AprobarTasaEnCuarentena(empDemo, actorA, origenTst, v.CuarentenaID); err != nil {
		t.Fatalf("aprobar: %v", err)
	}
	after := svc.TasaDeEmpresa(empDemo)
	if after.Valor != 1200 {
		t.Errorf("tras aprobar, la tasa vigente debía ser 1200, se obtuvo %v", after.Valor)
	}
	if after.EnCuarentena {
		t.Error("aprobada la lectura, no debe quedar cuarentena pendiente")
	}
	// El histórico es de solo-anexado: el rechazo original sigue ahí.
	rechazos := 0
	for _, h := range svc.HistorialTasa(empDemo, 50) {
		if h.Estado == tasa.EstadoRechazada {
			rechazos++
		}
	}
	if rechazos != 1 {
		t.Errorf("el registro rechazado debe conservarse en el histórico (se contaron %d)", rechazos)
	}
}

func TestSincronizarTasa_SiLaFuenteFallaConservaLaUltimaBuena(t *testing.T) {
	// Condición 5: la caja nunca se queda sin tasa porque el BCV no responda.
	prov := &proveedorFalso{nombre: tasa.FuenteBCV, err: errors.New("TLS handshake timeout")}
	svc := conFuente(t, prov)
	antes := svc.TasaDeEmpresa(empDemo)
	svc.SincronizarTasa(context.Background(), "sistema", origenTst, false)
	despues := svc.TasaDeEmpresa(empDemo)

	if despues.Valor != antes.Valor {
		t.Errorf("con la fuente caída la tasa no debe cambiar: %v → %v", antes.Valor, despues.Valor)
	}
	if despues.UltimoError == "" {
		t.Error("el fallo debe quedar declarado para poder explicar por qué no es de hoy")
	}
}

func TestSincronizarTasa_UsaElRespaldoCuandoElBCVFalla(t *testing.T) {
	bcv := &proveedorFalso{nombre: tasa.FuenteBCV, err: errors.New("certificado inválido")}
	resp := &proveedorFalso{nombre: tasa.FuenteRespaldo, lect: tasa.Lectura{
		Valor: 750, Fuente: tasa.FuenteRespaldo, Detalle: "pydolarve.org",
	}}
	svc := conFuente(t, bcv, resp)
	svc.SincronizarTasa(context.Background(), "sistema", origenTst, false)

	v := svc.TasaDeEmpresa(empDemo)
	if v.Valor != 750 {
		t.Fatalf("debía tomarse el respaldo, se obtuvo %v", v.Valor)
	}
	// El rótulo declara que fue el respaldo, no el BCV directo.
	if v.FuenteLabel != "BCV (respaldo)" {
		t.Errorf("el origen debe declararse tal cual: %q", v.FuenteLabel)
	}
}

func TestSincronizarTasa_NoConsultaDosVecesSeguidas(t *testing.T) {
	// Condición 7: una consulta al día por instancia. Un segundo intento
	// inmediato no vuelve a salir a la red.
	prov := &proveedorFalso{nombre: tasa.FuenteBCV, lect: tasa.Lectura{Valor: 750, Fuente: tasa.FuenteBCV}}
	svc := conFuente(t, prov)
	svc.SincronizarTasa(context.Background(), "sistema", origenTst, false)
	_, err := svc.SincronizarTasa(context.Background(), actorA, origenTst, true)

	if prov.llamadas != 1 {
		t.Errorf("la fuente debía consultarse una sola vez, se consultó %d", prov.llamadas)
	}
	if !errors.Is(err, application.ErrSincroDemasiadoPronto) {
		t.Errorf("un intento forzado inmediato debe avisar de la espera, se obtuvo: %v", err)
	}
}

func TestCargarTasaManual_RechazaValoresImposibles(t *testing.T) {
	svc, _ := nuevoServicio(t)
	for _, v := range []float64{0, -5} {
		if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, v, ""); !errors.Is(err, application.ErrTasaInvalida) {
			t.Errorf("valor %v debía rechazarse con ErrTasaInvalida, se obtuvo: %v", v, err)
		}
	}
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 50_000_000, ""); !errors.Is(err, application.ErrTasaFueraDeRango) {
		t.Error("una cifra fuera del rango plausible debe rechazarse")
	}
}

func TestCargarTasaManual_SeRotulaManualYNoOficial(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 800, "2026-07-31"); err != nil {
		t.Fatalf("cargar: %v", err)
	}
	v := svc.TasaDeEmpresa(empDemo)
	if v.Valor != 800 {
		t.Fatalf("la carga manual debía quedar vigente, se obtuvo %v", v.Valor)
	}
	if v.Oficial || v.FuenteLabel != "Tasa manual" {
		t.Errorf("una cifra tecleada no se rotula BCV: %+v", v)
	}
}

func TestTasaManual_NoSeFiltraEntreEmpresas(t *testing.T) {
	// Aislamiento de tenant: la carga manual es de la empresa que la hizo.
	svc, _ := nuevoServicio(t)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 900, ""); err != nil {
		t.Fatalf("cargar: %v", err)
	}
	for _, h := range svc.HistorialTasa("emp_otra", 50) {
		if h.Valor == 900 && h.Fuente == tasa.FuenteManual {
			t.Fatal("otra empresa no puede ver la carga manual de esta")
		}
	}
}

func TestEmitirFactura_SinTasaNoAceptaCobroEnDivisas(t *testing.T) {
	// El POS ya no manda la tasa: si no hay ninguna en el servidor, un cobro en
	// divisas no se puede convertir y la emisión se rechaza con una explicación,
	// en lugar de calcular el IGTF con una tasa inventada.
	svc := servicioSinTasa(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoUSD, Monto: 1, Moneda: "USD"}},
	})
	if !errors.Is(err, application.ErrSinTasa) {
		t.Fatalf("se esperaba ErrSinTasa, se obtuvo: %v", err)
	}
}

func TestEmitirFactura_SinTasaSigueFacturandoEnBolivares(t *testing.T) {
	// Offline-first: que el BCV no haya respondido hoy no puede cerrar la caja
	// de quien cobra íntegramente en bolívares.
	svc := servicioSinTasa(t)
	abrirTurno(t, svc, actorA)
	sku := primerSKU(t, svc)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("una venta solo en bolívares debe emitirse sin tasa: %v", err)
	}
	if doc.IGTF != 0 {
		t.Errorf("sin divisas no hay IGTF, se obtuvo %v", doc.IGTF)
	}
}

func TestEmitirFactura_ConvierteElPrecioEnDolaresConLaTasaDelServidor(t *testing.T) {
	// R10: el precio del catálogo puede estar en US$; la factura legal va en Bs
	// con la tasa del día, y el documento guarda esa tasa y su origen.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 800, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		// ELE-TV está sembrado con precio en dólares (150,00 US$). PrecioUnitario
		// negativo ⇒ "usar precio de catálogo" (única ruta que convierte con la tasa).
		Lineas: []application.LineaEntrada{{SKU: "ELE-TV", Cantidad: 1, PrecioUnitario: -1}},
		// 150 US$ × 800 = 120.000 Bs + IVA 16% = 139.200 Bs.
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 139_200, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	// 150 US$ × 800 Bs = 120.000 Bs de subtotal.
	if !casi(doc.Subtotal, 120_000) {
		t.Errorf("subtotal esperado 120.000 Bs (150 US$ × 800), se obtuvo %v", doc.Subtotal)
	}
	if doc.TasaCambio != 800 || doc.TasaFuente != tasa.FuenteManual {
		t.Errorf("el documento debe guardar la tasa usada y su origen: %v / %q", doc.TasaCambio, doc.TasaFuente)
	}
}

func TestGuardarConfigMoneda_ValidaLasOpciones(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.GuardarConfigMoneda(empDemo, actorA, origenTst, application.ConfigMoneda{
		MonedaPrincipal: "EUR", FuenteTasa: "bcv",
	}); err == nil {
		t.Error("una moneda principal fuera de VES/USD debe rechazarse")
	}
	if _, err := svc.GuardarConfigMoneda(empDemo, actorA, origenTst, application.ConfigMoneda{
		MonedaPrincipal: "VES", FuenteTasa: "paralelo_telegram",
	}); err == nil {
		t.Error("una fuente de tasa desconocida debe rechazarse")
	}
	emp, err := svc.GuardarConfigMoneda(empDemo, actorA, origenTst, application.ConfigMoneda{
		MonedaPrincipal: "USD", FuenteTasa: "manual", PreciosEnUsd: true,
	})
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if emp.MonedaPrincipal != "USD" || emp.FuenteTasa != "manual" || !emp.PreciosEnUsd {
		t.Errorf("la configuración no se guardó: %+v", emp)
	}
}

func TestTasaManualEUR_AisladaDelDolar(t *testing.T) {
	// Multimoneda: activar EUR y cargarle una tasa manual no toca la tasa del
	// dólar, y cada una se lee por su moneda.
	svc, _ := nuevoServicio(t)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 800, "2026-08-01"); err != nil {
		t.Fatalf("cargar USD: %v", err)
	}
	// EUR aún no está activo: cargarlo debe rechazarse.
	if _, err := svc.CargarTasaManualEnMoneda(empDemo, actorA, origenTst, "EUR", 870, ""); !errors.Is(err, application.ErrMonedaNoActiva) {
		t.Fatalf("sin activar EUR la carga debía rechazarse con ErrMonedaNoActiva, se obtuvo: %v", err)
	}
	// Se activa EUR (fuente manual) y se le carga su tasa.
	if err := svc.ActualizarMonedasActivas(empDemo, actorA, origenTst, []empresa.MonedaActiva{
		{Codigo: "USD", Fuente: "bcv"}, {Codigo: "EUR", Fuente: "manual"},
	}); err != nil {
		t.Fatalf("activar EUR: %v", err)
	}
	if _, err := svc.CargarTasaManualEnMoneda(empDemo, actorA, origenTst, "EUR", 870, ""); err != nil {
		t.Fatalf("cargar EUR: %v", err)
	}
	// Cada moneda devuelve la suya; el dólar no se contamina con el euro.
	usd, ok := svc.TasaVigenteDeMoneda(empDemo, "USD")
	if !ok || usd.Valor != 800 {
		t.Fatalf("USD debía seguir en 800, se obtuvo %v (ok=%v)", usd.Valor, ok)
	}
	eur, ok := svc.TasaVigenteDeMoneda(empDemo, "EUR")
	if !ok || eur.Valor != 870 || eur.Moneda != "EUR" {
		t.Fatalf("EUR debía leerse en 870, se obtuvo %v / %q (ok=%v)", eur.Valor, eur.Moneda, ok)
	}
	// Retrocompat: TasaVigente sigue siendo el dólar.
	if v, _ := svc.TasaVigente(empDemo); v.Valor != 800 {
		t.Errorf("TasaVigente debe seguir dando el dólar (800), se obtuvo %v", v.Valor)
	}
	// FuenteActivaValida: bcv no aplica a una divisa que no sea USD.
	if err := svc.ActualizarMonedasActivas(empDemo, actorA, origenTst, []empresa.MonedaActiva{
		{Codigo: "EUR", Fuente: "bcv"},
	}); err == nil {
		t.Error("bcv como fuente de EUR debía rechazarse (la fuente BCV es solo del USD)")
	}
}

func TestSincronizarBCV_NoTocaLasOtrasDivisas(t *testing.T) {
	// La sincronización del BCV es SOLO del dólar: mover la tasa oficial no puede
	// alterar la tasa manual de otra divisa.
	prov := &proveedorFalso{nombre: tasa.FuenteBCV, lect: tasa.Lectura{
		Valor: 760, FechaValor: "2026-08-01", Fuente: tasa.FuenteBCV,
	}}
	svc := conFuente(t, prov)
	if err := svc.ActualizarMonedasActivas(empDemo, actorA, origenTst, []empresa.MonedaActiva{
		{Codigo: "USD", Fuente: "bcv"}, {Codigo: "EUR", Fuente: "manual"},
	}); err != nil {
		t.Fatalf("activar EUR: %v", err)
	}
	if _, err := svc.CargarTasaManualEnMoneda(empDemo, actorA, origenTst, "EUR", 870, ""); err != nil {
		t.Fatalf("cargar EUR: %v", err)
	}
	svc.SincronizarTasa(context.Background(), "sistema", origenTst, false)

	// El dólar tomó la del BCV…
	if usd := svc.TasaDeEmpresa(empDemo); usd.Valor != 760 || !usd.Oficial {
		t.Fatalf("el dólar debía tomar la del BCV (760, oficial), se obtuvo %+v", usd)
	}
	// …y el euro siguió intacto en su tasa manual.
	eur, ok := svc.TasaVigenteDeMoneda(empDemo, "EUR")
	if !ok || eur.Valor != 870 {
		t.Errorf("sincronizar el BCV no debe tocar el euro (870), se obtuvo %v (ok=%v)", eur.Valor, ok)
	}
}

func TestFuenteManual_IgnoraLaTasaDePlataforma(t *testing.T) {
	// Si la empresa eligió «La escribo yo cada día», la sincronización del BCV
	// no le cambia los precios por debajo.
	prov := &proveedorFalso{nombre: tasa.FuenteBCV, lect: tasa.Lectura{Valor: 760, Fuente: tasa.FuenteBCV}}
	svc := conFuente(t, prov)
	if _, err := svc.GuardarConfigMoneda(empDemo, actorA, origenTst, application.ConfigMoneda{
		MonedaPrincipal: "VES", FuenteTasa: "manual",
	}); err != nil {
		t.Fatalf("configurar: %v", err)
	}
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 900, ""); err != nil {
		t.Fatalf("cargar: %v", err)
	}
	svc.SincronizarTasa(context.Background(), "sistema", origenTst, false)

	if v := svc.TasaDeEmpresa(empDemo); v.Valor != 900 {
		t.Errorf("con fuente manual la tasa debe seguir siendo la cargada (900), se obtuvo %v", v.Valor)
	}
}
