package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/caja"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/venta"
)

// Identificadores y credenciales del seed demo (ver adapter/inmem/seed.go).
const (
	empDemo   = "emp_demo"
	sede1     = "sede_demo_1" // Sede Principal
	sede2     = "sede_demo_2" // Sede Este
	caja1     = "caja_demo_1" // C-001, Sede Principal
	cajaEste  = "caja_demo_3" // C-003, Sede Este
	actorA    = "usr_a"
	actorB    = "usr_b"
	origenTst = "test"
)

// nuevoServicio arma el servicio sobre el store en memoria ya sembrado.
func nuevoServicio(t *testing.T) (*application.Service, *inmem.Store) {
	t.Helper()
	st := inmem.New()
	svc := application.New(
		st.Productos, st.Movimientos, st.Transferencias, st.Rubros, st.Audit,
		st.Documentos, st.Numerador, st.CuentasCobro, st.MetodosPago, st.Clientes, st.Cotizaciones,
		st.Cajas, st.Cajeros, st.SesionesCaja,
		st.Tasas, st.Empresas, st.VentasEnEspera, st.Cobros,
		st.CuentasContables, st.Asientos, st.Periodos,
		st.Proveedores, st.OrdenesCompra, st.CierresZ,
		st.FacturasCompra, st.PagosProveedor, st.Retenciones,
		st.Dispositivos,
	)
	// Notas de crédito/débito de proveedor: el Store in-memory aún no expone el repo
	// (vive en seed.go), así que se cablea con uno propio para los tests.
	svc.ConNotasCompra(inmem.NewNotaCompraRepo())
	svc.ConSolicitudesCompra(st.Solicitudes)
	return svc, st
}

func TestAbrirCaja_PinIncorrecto(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-001", "0000")
	if !errors.Is(err, application.ErrCajeroInvalido) {
		t.Fatalf("con PIN incorrecto se esperaba ErrCajeroInvalido, se obtuvo: %v", err)
	}
}

func TestAbrirCaja_CodigoInexistente(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// El error debe ser el MISMO que con PIN incorrecto: no revelar si el código
	// existe evita enumerar cajeros válidos.
	_, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-999", inmem.PinDemo)
	if !errors.Is(err, application.ErrCajeroInvalido) {
		t.Fatalf("con código inexistente se esperaba ErrCajeroInvalido, se obtuvo: %v", err)
	}
}

func TestAbrirCaja_CajeroDeOtraSede(t *testing.T) {
	svc, _ := nuevoServicio(t)
	// OP-002 (Ana Gómez) es de Sede Este; caja1 es de Sede Principal.
	_, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-002", inmem.PinDemo)
	if !errors.Is(err, application.ErrCajeroOtraSede) {
		t.Fatalf("se esperaba ErrCajeroOtraSede, se obtuvo: %v", err)
	}
}

func TestAbrirCaja_Deshabilitada(t *testing.T) {
	svc, st := nuevoServicio(t)
	c, _ := st.Cajas.ByID(empDemo, caja1)
	c.Estado = caja.EstadoDeshabilitada
	st.Cajas.Update(c)

	_, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo)
	if !errors.Is(err, application.ErrCajaDeshabilitada) {
		t.Fatalf("se esperaba ErrCajaDeshabilitada, se obtuvo: %v", err)
	}
}

func TestAbrirCaja_Exitosa(t *testing.T) {
	svc, _ := nuevoServicio(t)
	ses, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo)
	if err != nil {
		t.Fatalf("la apertura debía funcionar: %v", err)
	}
	if !ses.Abierta() {
		t.Error("la sesión recién abierta debe estar abierta")
	}
	if ses.CajaCodigo != "C-001" || ses.CajeroNombre != "Luis Marcano" {
		t.Errorf("la sesión debe copiar caja y cajero para el arqueo, se obtuvo %+v", ses)
	}
	if ses.SedeID != sede1 {
		t.Errorf("la sesión debe heredar la sede de la caja, se obtuvo %q", ses.SedeID)
	}
}

func TestAbrirCaja_TomadaPorOtroCajero(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo); err != nil {
		t.Fatalf("preparación: %v", err)
	}
	// OP-003 (Pedro Salas) también es de Sede Principal, así que pasa la
	// validación de sede pero debe chocar con el turno ya abierto.
	_, err := svc.AbrirCaja(empDemo, actorB, origenTst, caja1, "OP-003", inmem.PinDemo)
	if !errors.Is(err, application.ErrCajaTomada) {
		t.Fatalf("se esperaba ErrCajaTomada, se obtuvo: %v", err)
	}
}

func TestAbrirCaja_RetomarTurnoPropioNoDuplica(t *testing.T) {
	svc, st := nuevoServicio(t)
	primera, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo)
	if err != nil {
		t.Fatalf("preparación: %v", err)
	}
	// El mismo cajero vuelve, quizá desde otro dispositivo.
	segunda, err := svc.AbrirCaja(empDemo, actorB, origenTst, caja1, "OP-001", inmem.PinDemo)
	if err != nil {
		t.Fatalf("retomar el turno propio no debe fallar: %v", err)
	}
	if segunda.ID != primera.ID {
		t.Errorf("retomar debe reutilizar la sesión %q, creó %q", primera.ID, segunda.ID)
	}
	if segunda.Apertura != primera.Apertura {
		t.Error("retomar no debe reescribir la hora de apertura: el arqueo mentiría")
	}
	if segunda.ActorID != actorB {
		t.Errorf("retomar debe apuntar al dispositivo actual, quedó %q", segunda.ActorID)
	}
	if n := len(st.SesionesCaja.List(empDemo)); n != 1 {
		t.Errorf("debe haber una sola sesión tras retomar, hay %d", n)
	}
}

func TestCerrarCaja_LiberaLaCaja(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo); err != nil {
		t.Fatalf("preparación: %v", err)
	}
	if _, err := svc.CerrarCaja(empDemo, actorA, origenTst, caja1, false); err != nil {
		t.Fatalf("el cierre debía funcionar: %v", err)
	}
	if _, abierta := svc.SesionDeActor(empDemo, actorA); abierta {
		t.Error("tras cerrar no debe quedar sesión abierta para el actor")
	}
	// La caja queda libre para el siguiente turno.
	if _, err := svc.AbrirCaja(empDemo, actorB, origenTst, caja1, "OP-003", inmem.PinDemo); err != nil {
		t.Errorf("otro cajero debía poder abrir la caja liberada: %v", err)
	}
}

// --- Arqueo de caja ---

// armarTurnoConArqueo abre un turno con fondo inicial y emite tres facturas en
// él (efectivo Bs con vuelto, efectivo USD y tarjeta), para ejercitar el arqueo.
// Devuelve la sesión abierta. Tasa fijada en 100 Bs/US$ para que las cuentas en
// divisas sean deterministas.
func armarTurnoConArqueo(t *testing.T, svc *application.Service) caja.Sesion {
	t.Helper()
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 100, ""); err != nil {
		t.Fatalf("tasa: %v", err)
	}
	ses, err := svc.AbrirCajaConFondo(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo, 50)
	if err != nil {
		t.Fatalf("abrir con fondo: %v", err)
	}
	if !casi(ses.FondoInicial, 50) {
		t.Fatalf("la apertura debe guardar el fondo inicial 50, guardó %v", ses.FondoInicial)
	}
	// Doc 1: efectivo Bs. Total 116; se paga 200 ⇒ vuelto 84 Bs.
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 200, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("emitir efectivo Bs: %v", err)
	}
	// Doc 2: efectivo USD. Total 116; se paga 5 US$ (=500 Bs) ⇒ vuelto en US$
	// (aparte del Bs, no toca la gaveta de bolívares).
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoUSD, Monto: 5, Moneda: "USD"}},
	}); err != nil {
		t.Fatalf("emitir efectivo USD: %v", err)
	}
	// Doc 3: tarjeta. Total 116; se paga justo, sin vuelto.
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoTarjeta, Monto: 116, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("emitir tarjeta: %v", err)
	}
	return ses
}

func montoDeMetodo(a caja.Arqueo, metodo string) (caja.ArqueoMetodo, bool) {
	for _, m := range a.Metodos {
		if m.Metodo == metodo {
			return m, true
		}
	}
	return caja.ArqueoMetodo{}, false
}

func TestArqueoDeSesion_PliegaPorMetodoYEfectivoEsperado(t *testing.T) {
	svc, _ := nuevoServicio(t)
	ses := armarTurnoConArqueo(t, svc)

	arqueo, err := svc.ArqueoDeSesion(empDemo, ses.ID)
	if err != nil {
		t.Fatalf("arqueo: %v", err)
	}
	if arqueo.Documentos != 3 {
		t.Errorf("el arqueo debía plegar 3 documentos del turno, plegó %d", arqueo.Documentos)
	}
	// Esperado por método.
	if m, ok := montoDeMetodo(arqueo, fiscal.PagoEfectivoBs); !ok || !casi(m.Monto, 200) || !casi(m.EquivalenteBs, 200) {
		t.Errorf("efectivo Bs esperado: 200 en gaveta, se obtuvo %+v", m)
	}
	// Doc 2 pagó 5 US$ (=500 Bs); el total con IGTF es 119,48 Bs, así que el vuelto
	// en USD es 380,52 Bs = 3,81 US$. El bucket de la divisa reporta el NETO en
	// gaveta (recibido − vuelto), no el bruto recibido.
	if m, ok := montoDeMetodo(arqueo, fiscal.PagoEfectivoUSD); !ok || !casi(m.Monto, 1.19) || !casi(m.EquivalenteBs, 119.48) || !m.EnDivisa {
		t.Errorf("efectivo USD esperado NETO: 1,19 US$ = 119,48 Bs (5 − 3,81 de vuelto), se obtuvo %+v", m)
	}
	if m, ok := montoDeMetodo(arqueo, fiscal.PagoTarjeta); !ok || !casi(m.Monto, 116) || !casi(m.EquivalenteBs, 116) {
		t.Errorf("tarjeta esperada: 116, se obtuvo %+v", m)
	}
	// Efectivo Bs esperado = fondo(50) + cobros efectivo Bs(200) − vuelto Bs(84) = 166.
	if !casi(arqueo.CobrosEfectivoBs, 200) || !casi(arqueo.VueltoEfectivoBs, 84) {
		t.Errorf("cobros/vuelto efectivo Bs esperados 200/84, se obtuvo %v/%v", arqueo.CobrosEfectivoBs, arqueo.VueltoEfectivoBs)
	}
	if !casi(arqueo.EfectivoEsperadoBs, 166) {
		t.Errorf("efectivo esperado = 50 + 200 − 84 = 166, se obtuvo %v", arqueo.EfectivoEsperadoBs)
	}
}

func TestArqueoDeSesion_VueltoMixtoSoloBajaLaGavetaLaParteBsEfectivo(t *testing.T) {
	// Vuelto MIXTO: parte en Bs efectivo + parte en US$ efectivo + parte por pago
	// móvil. Solo la parte en Bs efectivo sale de la GAVETA de bolívares; el
	// efectivo en divisas se cuenta aparte y el pago móvil es una transferencia.
	svc, _ := nuevoServicio(t)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 100, ""); err != nil {
		t.Fatalf("tasa: %v", err)
	}
	ses, err := svc.AbrirCajaConFondo(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo, 50)
	if err != nil {
		t.Fatalf("abrir con fondo: %v", err)
	}
	// Total 116 (100 + IVA 16). Paga 500 Bs efectivo ⇒ excedente 384 Bs, repartido
	// en 100 Bs efectivo + 1 US$ efectivo (100 Bs) + 184 Bs por pago móvil.
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
		VueltoPartes: []application.VueltoParteEntrada{
			{Moneda: "VES", Metodo: fiscal.VueltoEfectivo, Monto: 100},
			{Moneda: "USD", Metodo: fiscal.VueltoEfectivo, Monto: 1},
			{Moneda: "VES", Metodo: fiscal.VueltoPagoMovil, Monto: 184, Banco: "0102", Cedula: "V-12345678", Telefono: "0414-1234567"},
		},
	}); err != nil {
		t.Fatalf("emitir con vuelto mixto: %v", err)
	}
	arqueo, err := svc.ArqueoDeSesion(empDemo, ses.ID)
	if err != nil {
		t.Fatalf("arqueo: %v", err)
	}
	// Solo la parte Bs efectivo (100) baja la gaveta; ni el US$ ni el pago móvil.
	if !casi(arqueo.VueltoEfectivoBs, 100) {
		t.Errorf("vuelto en efectivo Bs esperado 100 (solo la parte Bs), se obtuvo %v", arqueo.VueltoEfectivoBs)
	}
	// Efectivo esperado = fondo(50) + cobros efectivo Bs(500) − vuelto Bs efectivo(100) = 450.
	if !casi(arqueo.CobrosEfectivoBs, 500) || !casi(arqueo.EfectivoEsperadoBs, 450) {
		t.Errorf("cobros 500 / esperado 450, se obtuvo %v / %v", arqueo.CobrosEfectivoBs, arqueo.EfectivoEsperadoBs)
	}
}

func TestArqueoDeSesion_VueltoEnDivisaRebajaElBucketDeEsaDivisa(t *testing.T) {
	// FIX arqueo: cuando el cliente paga en efectivo en una DIVISA y el vuelto se
	// entrega en ESA divisa, el bucket de la divisa debe reportar el NETO en gaveta
	// (recibido − vuelto), no el bruto recibido. Antes conservaba el bruto y
	// sobrestimaba el efectivo esperado en esa moneda.
	svc, _ := nuevoServicio(t)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 100, ""); err != nil {
		t.Fatalf("tasa: %v", err)
	}
	ses, err := svc.AbrirCajaConFondo(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo, 0)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	// REF-2L gravado, precio 100 Bs. Paga 3 US$ (=300 Bs) y el vuelto sale en US$.
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoUSD, Monto: 3, Moneda: "USD"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	arqueo, err := svc.ArqueoDeSesion(empDemo, ses.ID)
	if err != nil {
		t.Fatalf("arqueo: %v", err)
	}
	m, ok := montoDeMetodo(arqueo, fiscal.PagoEfectivoUSD)
	if !ok {
		t.Fatal("debe haber un bucket de efectivo en US$")
	}
	// Todo se pagó en US$ y el resto se devolvió en US$: el neto en US$ retenido
	// cubre exactamente el total, así que su equivalente en Bs ≈ el total.
	if !casi(m.EquivalenteBs, doc.Total) {
		t.Errorf("el bucket US$ debe reportar el neto (≈ total %v Bs), se obtuvo %v", doc.Total, m.EquivalenteBs)
	}
	// Y por lo tanto NO el bruto de 300 Bs recibidos.
	if m.EquivalenteBs >= 300 {
		t.Errorf("el bucket US$ no debe conservar el bruto recibido (300 Bs), se obtuvo %v", m.EquivalenteBs)
	}
}

func TestArqueoDeSesion_NoMezclaOtrasSesiones(t *testing.T) {
	// El arqueo pliega SOLO los documentos de su SesionCajaID: la venta forma
	// libre sin caja (SinCaja) no entra en el arqueo del turno.
	svc, _ := nuevoServicio(t)
	ses := armarTurnoConArqueo(t, svc)
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorB, origenTst, application.EmitirEntrada{
		Lineas:  []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:   []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
		SinCaja: true,
	}); err != nil {
		t.Fatalf("emitir sin caja: %v", err)
	}
	arqueo, err := svc.ArqueoDeSesion(empDemo, ses.ID)
	if err != nil {
		t.Fatalf("arqueo: %v", err)
	}
	if arqueo.Documentos != 3 {
		t.Errorf("la venta sin caja no debe entrar al arqueo del turno; se plegaron %d (esperados 3)", arqueo.Documentos)
	}
}

func TestCerrarCajaConArqueo_Sobrante(t *testing.T) {
	svc, _ := nuevoServicio(t)
	armarTurnoConArqueo(t, svc)
	// Esperado 166; el cajero declara 170 ⇒ sobrante +4.
	contado := 170.0
	out, err := svc.CerrarCajaConArqueo(empDemo, actorA, origenTst, caja1, false, application.CierreArqueo{
		EfectivoContadoBs: &contado,
	})
	if err != nil {
		t.Fatalf("cerrar con arqueo: %v", err)
	}
	if out.Arqueo == nil {
		t.Fatal("la sesión cerrada debe congelar su arqueo")
	}
	if !casi(out.Arqueo.EfectivoEsperadoBs, 166) || !casi(out.Arqueo.EfectivoContadoBs, 170) {
		t.Errorf("esperado 166 / contado 170, se obtuvo %v / %v", out.Arqueo.EfectivoEsperadoBs, out.Arqueo.EfectivoContadoBs)
	}
	if !casi(out.Arqueo.DiferenciaBs, 4) {
		t.Errorf("un sobrante de 4 (170 − 166) esperado, se obtuvo %v", out.Arqueo.DiferenciaBs)
	}
	if out.Abierta() {
		t.Error("tras el arqueo el turno debe quedar cerrado")
	}
}

func TestCerrarCajaConArqueo_Faltante(t *testing.T) {
	svc, _ := nuevoServicio(t)
	armarTurnoConArqueo(t, svc)
	// Esperado 166; el cajero declara 160 ⇒ faltante −6.
	contado := 160.0
	out, err := svc.CerrarCajaConArqueo(empDemo, actorA, origenTst, caja1, false, application.CierreArqueo{
		EfectivoContadoBs: &contado,
	})
	if err != nil {
		t.Fatalf("cerrar con arqueo: %v", err)
	}
	if out.Arqueo == nil || !casi(out.Arqueo.DiferenciaBs, -6) {
		t.Errorf("un faltante de −6 (160 − 166) esperado, se obtuvo %+v", out.Arqueo)
	}
	// El arqueo congelado NO se recalcula al consultarlo de nuevo: es la foto del cierre.
	arqueo, err := svc.ArqueoDeSesion(empDemo, out.ID)
	if err != nil {
		t.Fatalf("arqueo tras cierre: %v", err)
	}
	if !casi(arqueo.DiferenciaBs, -6) || !arqueo.Declarado {
		t.Errorf("el arqueo consultado debe ser la foto congelada del cierre, se obtuvo %+v", arqueo)
	}
}

func TestCerrarCaja_SinConteoCongelaEsperadoSinDiferencia(t *testing.T) {
	// Un cierre sin conteo (p. ej. forzado) congela lo esperado pero no declara la
	// gaveta: la diferencia no se inventa.
	svc, _ := nuevoServicio(t)
	armarTurnoConArqueo(t, svc)
	out, err := svc.CerrarCaja(empDemo, actorA, origenTst, caja1, false)
	if err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	if out.Arqueo == nil || !casi(out.Arqueo.EfectivoEsperadoBs, 166) {
		t.Fatalf("debe congelar lo esperado (166), se obtuvo %+v", out.Arqueo)
	}
	if out.Arqueo.Declarado || !casi(out.Arqueo.DiferenciaBs, 0) {
		t.Errorf("sin conteo no hay diferencia ni declaración, se obtuvo %+v", out.Arqueo)
	}
}

func TestCrearCaja_NaceDeshabilitadaYNumerada(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCaja(empDemo, sede1, actorA, origenTst, "Caja nueva", "")
	if err != nil {
		t.Fatalf("crear caja: %v", err)
	}
	if c.Estado != caja.EstadoDeshabilitada {
		t.Errorf("una caja nueva debe nacer deshabilitada, nació %q", c.Estado)
	}
	if c.Codigo == "" {
		t.Error("el código lo genera el servidor, no puede quedar vacío")
	}
	// El seed ya trae C-001..C-003, así que la nueva no puede repetirlos.
	for _, usado := range []string{"C-001", "C-002", "C-003"} {
		if c.Codigo == usado {
			t.Errorf("el código %q ya estaba en uso", c.Codigo)
		}
	}
}

func TestCrearCajero_RechazaPinDebil(t *testing.T) {
	svc, _ := nuevoServicio(t)
	for _, pin := range []string{"", "12", "12345", "abcd"} {
		if _, err := svc.CrearCajero(empDemo, actorA, origenTst, sede1, "Prueba", "", pin, "", false); !errors.Is(err, application.ErrPinDebil) {
			t.Errorf("el PIN %q debía rechazarse con ErrPinDebil, se obtuvo: %v", pin, err)
		}
	}
}

func TestCrearCajero_NoExponeElPin(t *testing.T) {
	svc, _ := nuevoServicio(t)
	cj, err := svc.CrearCajero(empDemo, actorA, origenTst, sede1, "Prueba", "", "1357", "", false)
	if err != nil {
		t.Fatalf("crear cajero: %v", err)
	}
	if cj.PinHash == "1357" {
		t.Fatal("el PIN se guardó en claro")
	}
	if cj.PinHash == "" {
		t.Fatal("falta el hash del PIN")
	}
	// Y la credencial recién creada debe servir para abrir una caja de su sede.
	c, err := svc.CrearCaja(empDemo, sede1, actorA, origenTst, "Caja de prueba", "")
	if err != nil {
		t.Fatalf("crear caja: %v", err)
	}
	if _, err := svc.CambiarEstadoCaja(empDemo, actorA, origenTst, c.ID, caja.EstadoHabilitada); err != nil {
		t.Fatalf("habilitar caja: %v", err)
	}
	if _, err := svc.AbrirCaja(empDemo, actorA, origenTst, c.ID, cj.Codigo, "1357"); err != nil {
		t.Errorf("el cajero creado debía poder abrir su caja: %v", err)
	}
}

func TestListarCajas_MarcaOcupadaYPropia(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo); err != nil {
		t.Fatalf("preparación: %v", err)
	}
	// Quien abrió ve su caja como propia…
	for _, v := range svc.ListarCajas(empDemo, sede1, actorA) {
		if v.ID != caja1 {
			continue
		}
		if !v.Ocupada || !v.Propia || v.OcupadaPor != "Luis Marcano" {
			t.Errorf("para el propio actor se esperaba ocupada+propia por Luis Marcano, se obtuvo %+v", v)
		}
	}
	// …y otro usuario la ve ocupada pero ajena.
	for _, v := range svc.ListarCajas(empDemo, sede1, actorB) {
		if v.ID != caja1 {
			continue
		}
		if !v.Ocupada || v.Propia {
			t.Errorf("para otro actor se esperaba ocupada y NO propia, se obtuvo %+v", v)
		}
	}
}

func TestCambiarEstado_NoDeshabilitaConTurnoAbierto(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.AbrirCaja(empDemo, actorA, origenTst, caja1, "OP-001", inmem.PinDemo); err != nil {
		t.Fatalf("preparación: %v", err)
	}
	// Deshabilitar con turno abierto perdería el arqueo del turno en curso.
	if _, err := svc.CambiarEstadoCaja(empDemo, actorA, origenTst, caja1, caja.EstadoDeshabilitada); err == nil {
		t.Error("deshabilitar una caja con turno abierto debía fallar")
	}
}

func TestAislamientoDeTenant_NoVeCajasDeOtraEmpresa(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if cajas := svc.ListarCajas("emp_ajena", "", actorA); len(cajas) != 0 {
		t.Errorf("una empresa ajena no debe ver ninguna caja, vio %d", len(cajas))
	}
	// Y no puede abrir una caja que no es suya ni con el ID correcto.
	if _, err := svc.AbrirCaja("emp_ajena", actorA, origenTst, caja1, "OP-001", inmem.PinDemo); !errors.Is(err, application.ErrCajaNoExiste) {
		t.Errorf("se esperaba ErrCajaNoExiste desde otro tenant, se obtuvo: %v", err)
	}
}

// pinesDistintos monta una empresa aparte con UN cajero raso y UN supervisor con PIN
// DISTINTOS. AutorizarSupervisor resuelve solo por PIN (no recibe código), así que la
// regla «un cajero raso no se autoriza a sí mismo» únicamente se puede probar con PINes
// diferentes. No usa el seed a propósito: sus PIN de demostración son todos iguales por
// comodidad del recorrido, y un test de seguridad no debe depender de ese dato.
func pinesDistintos(t *testing.T) (*application.Service, string, string) {
	t.Helper()
	svc, _ := nuevoServicio(t)
	const pinRaso, pinSuper = "1111", "9999"
	if _, err := svc.CrearCajero("emp_pins", actorA, origenTst, sede1, "Cajero Raso", "", pinRaso, "", false); err != nil {
		t.Fatalf("crear cajero raso: %v", err)
	}
	if _, err := svc.CrearCajero("emp_pins", actorA, origenTst, sede1, "Supervisora", "", pinSuper, "", true); err != nil {
		t.Fatalf("crear supervisor: %v", err)
	}
	return svc, pinRaso, pinSuper
}

func TestAutorizarSupervisor_SoloConElPinDeUnSupervisor(t *testing.T) {
	// El PIN de un cajero raso NO autoriza: si autorizara, el propio cajero podría
	// borrar líneas del carrito y la regla del flujo 2.4 no protegería nada.
	svc, pinRaso, pinSuper := pinesDistintos(t)
	if _, err := svc.AutorizarSupervisor("emp_pins", actorA, origenTst, "quitar línea", pinRaso); !errors.Is(err, application.ErrPinSupervisorInvalido) {
		t.Errorf("el PIN de un cajero no supervisor no debe autorizar, se obtuvo: %v", err)
	}
	nombre, err := svc.AutorizarSupervisor("emp_pins", actorA, origenTst, "quitar línea", pinSuper)
	if err != nil {
		t.Fatalf("el PIN del supervisor debía autorizar: %v", err)
	}
	if nombre == "" {
		t.Error("la autorización debe devolver quién autorizó, para poder mostrarlo y auditarlo")
	}
}

func TestAutorizarSupervisor_QuedaEnLaAuditoria(t *testing.T) {
	svc, _, pinSuper := pinesDistintos(t)
	if _, err := svc.AutorizarSupervisor("emp_pins", actorA, origenTst, "vaciar el carrito", pinSuper); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	encontrado := false
	for _, e := range svc.Auditoria("emp_pins") {
		if e.Accion == "caja.autorizacion" && e.Entidad == "vaciar el carrito" {
			encontrado = true
		}
	}
	if !encontrado {
		t.Error("cada autorización de supervisor tiene que quedar registrada con su acción")
	}
}

func TestGuardarMiembro_ElCajeroExigeSede(t *testing.T) {
	// Regla pedida por el cliente: un cajero pertenece a UNA sede. Guardarlo sin
	// sede lo dejaría sin contexto de mostrador.
	_, st := nuevoServicio(t)
	tn := application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)
	var cajero string
	for _, m := range tn.Miembros(empDemo) {
		if m.Rol == "cajero" {
			cajero = m.ID
		}
	}
	if cajero == "" {
		t.Fatal("el seed debería traer un usuario con rol cajero")
	}
	if _, err := tn.GuardarMiembro(empDemo, cajero, "cajero", ""); !errors.Is(err, application.ErrSedeRequerida) {
		t.Fatalf("un cajero sin sede debe rechazarse, se obtuvo: %v", err)
	}
	if _, err := tn.GuardarMiembro(empDemo, cajero, "cajero", sede2); err != nil {
		t.Fatalf("con sede válida debe guardar: %v", err)
	}
	// Un rol que ve toda la empresa no queda atado a una sede.
	m, err := tn.GuardarMiembro(empDemo, cajero, "dueno", sede2)
	if err != nil {
		t.Fatalf("guardar dueño: %v", err)
	}
	if m.SedeID != "" {
		t.Errorf("un rol de toda la empresa no debe quedar atado a una sede, quedó %q", m.SedeID)
	}
}

func TestVentaEnEspera_SeRetomaUnaSolaVez(t *testing.T) {
	// La regla que evita cobrar dos veces la misma venta: retomarla la CONSUME, así
	// que un segundo cajero no puede retomar el mismo carrito.
	svc, _ := nuevoServicio(t)
	v, err := svc.DejarVentaEnEspera(empDemo, sede1, actorA, origenTst, "el señor de la gorra", "",
		[]venta.Linea{{SKU: "REF-2L", Cantidad: 2}})
	if err != nil {
		t.Fatalf("dejar en espera: %v", err)
	}
	if len(svc.VentasEnEspera(empDemo, sede1)) != 1 {
		t.Fatal("la venta apartada debía aparecer en la lista de su sede")
	}
	// La retoma OTRO cajero: es lo que el prototipo declara («cualquier cajero
	// puede retomarlas»).
	retomada, err := svc.RetomarVenta(empDemo, sede1, actorB, origenTst, v.ID)
	if err != nil {
		t.Fatalf("retomar: %v", err)
	}
	if len(retomada.Lineas) != 1 || retomada.Lineas[0].SKU != "REF-2L" {
		t.Errorf("la venta retomada debía traer sus líneas: %+v", retomada.Lineas)
	}
	if _, err := svc.RetomarVenta(empDemo, sede1, actorA, origenTst, v.ID); !errors.Is(err, application.ErrVentaEnEsperaNoExiste) {
		t.Errorf("retomarla de nuevo debe fallar, se obtuvo: %v", err)
	}
	if len(svc.VentasEnEspera(empDemo, sede1)) != 0 {
		t.Error("tras retomarla no debe quedar en la lista")
	}
}

func TestVentaEnEspera_NoCruzaSedesNiTenants(t *testing.T) {
	svc, _ := nuevoServicio(t)
	v, err := svc.DejarVentaEnEspera(empDemo, sede1, actorA, origenTst, "", "",
		[]venta.Linea{{SKU: "REF-2L", Cantidad: 1}})
	if err != nil {
		t.Fatalf("dejar en espera: %v", err)
	}
	// Otra sede no la ve ni la puede retomar: sus precios y su stock son de la
	// tienda donde se armó.
	if len(svc.VentasEnEspera(empDemo, sede2)) != 0 {
		t.Error("la sede Este no debería ver una venta apartada en Sede Principal")
	}
	if _, err := svc.RetomarVenta(empDemo, sede2, actorA, origenTst, v.ID); err == nil {
		t.Error("retomarla desde otra sede debe rechazarse")
	}
	// Otro tenant tampoco.
	if len(svc.VentasEnEspera("emp_otra", "")) != 0 {
		t.Error("otra empresa no puede ver ventas apartadas de esta")
	}
	if _, err := svc.RetomarVenta("emp_otra", "", actorA, origenTst, v.ID); err == nil {
		t.Error("otro tenant no puede retomar esta venta")
	}
}

func TestVentaEnEspera_TomaLosPreciosDelCatalogo(t *testing.T) {
	// El POS no decide cuánto cuesta algo: si el cliente manda un precio de 1 Bs,
	// se guarda el del catálogo (misma regla que al emitir).
	svc, _ := nuevoServicio(t)
	v, err := svc.DejarVentaEnEspera(empDemo, sede1, actorA, origenTst, "", "",
		[]venta.Linea{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 0}})
	if err != nil {
		t.Fatalf("dejar en espera: %v", err)
	}
	prod, _ := svc.PorCodigoBarras(empDemo, "REF-2L")
	if !casi(v.Lineas[0].PrecioUnitario, prod.Precio) {
		t.Errorf("precio esperado %v (catálogo), se obtuvo %v", prod.Precio, v.Lineas[0].PrecioUnitario)
	}
	if v.Lineas[0].Exento != prod.ExentoIVA {
		t.Error("la condición de IVA se toma del catálogo, no del cliente")
	}
}

func TestVentaEnEspera_DescartarQuedaAuditado(t *testing.T) {
	// Descartar es la vía por la que una venta armada desaparece sin dejar
	// documento fiscal: tiene que quedar registro de quién la tiró.
	svc, _ := nuevoServicio(t)
	v, err := svc.DejarVentaEnEspera(empDemo, sede1, actorA, origenTst, "cliente se fue", "",
		[]venta.Linea{{SKU: "REF-2L", Cantidad: 3}})
	if err != nil {
		t.Fatalf("dejar en espera: %v", err)
	}
	if err := svc.DescartarVenta(empDemo, actorA, origenTst, v.ID); err != nil {
		t.Fatalf("descartar: %v", err)
	}
	encontrado := false
	for _, e := range svc.Auditoria(empDemo) {
		if e.Accion == "pos.venta_descartada" && e.Entidad == v.ID {
			encontrado = true
		}
	}
	if !encontrado {
		t.Error("descartar una venta en espera tiene que quedar auditado")
	}
}

// --- Tesorería ---

// aCredito emite una factura a crédito para el cliente demo y devuelve el doc.
func aCredito(t *testing.T, svc *application.Service, cant float64) fiscal.Documento {
	t.Helper()
	abrirTurno(t, svc, actorA)
	cl := svc.Clientes(empDemo)
	if len(cl) == 0 {
		t.Fatal("el seed debería traer clientes")
	}
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		ClienteID: cl[0].ID,
		Lineas:    []application.LineaEntrada{{SKU: "REF-2L", Cantidad: cant, PrecioUnitario: 100}},
		Credito:   true, DiasCredito: 30,
	})
	if err != nil {
		t.Fatalf("emitir a crédito: %v", err)
	}
	return doc
}

func TestVentaACredito_NoSeLeFiaAlConsumidorFinal(t *testing.T) {
	// Sin cliente identificado no hay a quién cobrarle después.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	_, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas:  []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Credito: true,
	})
	if !errors.Is(err, application.ErrCreditoSinCliente) {
		t.Fatalf("se esperaba ErrCreditoSinCliente, se obtuvo: %v", err)
	}
}

func TestCuentasPorCobrar_SaldoEsProyeccionDeLosCobros(t *testing.T) {
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 1) // 100 + IVA 16 = 116
	res := svc.CuentasPorCobrar(empDemo)
	var cta *application.CuentaPorCobrar
	for i := range res.Cuentas {
		if res.Cuentas[i].DocumentoID == doc.ID {
			cta = &res.Cuentas[i]
		}
	}
	if cta == nil {
		t.Fatal("la factura a crédito debía aparecer por cobrar")
	}
	if !casi(cta.Saldo, 116) {
		t.Fatalf("saldo esperado 116, se obtuvo %v", cta.Saldo)
	}
	// Un abono de 50 baja el saldo a 66 — sin editar ningún campo de saldo.
	if _, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: doc.ID, Monto: 50, Moneda: "VES", Metodo: fiscal.PagoPagoMovil,
	}); err != nil {
		t.Fatalf("registrar cobro: %v", err)
	}
	saldo, _ := svc.SaldoPorCobrar(empDemo, doc.ID)
	if !casi(saldo, 66) {
		t.Errorf("saldo esperado 66 tras el abono, se obtuvo %v", saldo)
	}
	// Cobrar más que el saldo se rechaza: eso sería un anticipo, otro documento.
	if _, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: doc.ID, Monto: 100, Moneda: "VES", Metodo: fiscal.PagoEfectivoBs,
	}); !errors.Is(err, application.ErrCobroExcedido) {
		t.Errorf("se esperaba ErrCobroExcedido, se obtuvo: %v", err)
	}
	// Al cubrir el saldo, la factura sale de por cobrar.
	if _, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: doc.ID, Monto: 66, Moneda: "VES", Metodo: fiscal.PagoEfectivoBs,
	}); err != nil {
		t.Fatalf("cobro final: %v", err)
	}
	for _, c := range svc.CuentasPorCobrar(empDemo).Cuentas {
		if c.DocumentoID == doc.ID {
			t.Error("una factura cobrada por completo no debe seguir por cobrar")
		}
	}
}

func TestReversarCobro_NoBorraNadaYRestituyeElSaldo(t *testing.T) {
	// Un cobro mal registrado se corrige con su reverso, como un documento fiscal.
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 1)
	cob, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: doc.ID, Monto: 116, Moneda: "VES", Metodo: fiscal.PagoEfectivoBs,
	})
	if err != nil {
		t.Fatalf("registrar: %v", err)
	}
	if _, err := svc.ReversarCobro(empDemo, actorA, origenTst, cob.ID, "se registró en la factura equivocada"); err != nil {
		t.Fatalf("reversar: %v", err)
	}
	saldo, hay := svc.SaldoPorCobrar(empDemo, doc.ID)
	if !hay || !casi(saldo, 116) {
		t.Errorf("tras el reverso el saldo vuelve a 116, se obtuvo %v (%v)", saldo, hay)
	}
	// El cobro original sigue en el histórico: no se borró.
	vistos := 0
	for _, c := range svc.Cobros(empDemo) {
		if c.ID == cob.ID || c.RefCobroID == cob.ID {
			vistos++
		}
	}
	if vistos != 2 {
		t.Errorf("el histórico debe conservar el cobro y su reverso, se contaron %d", vistos)
	}
	if _, err := svc.ReversarCobro(empDemo, actorA, origenTst, cob.ID, "otra vez"); !errors.Is(err, application.ErrCobroYaReversado) {
		t.Errorf("no se puede reversar dos veces: %v", err)
	}
}

func TestSaldosDeTesoreria_RestaElVueltoEntregado(t *testing.T) {
	// El efectivo declarado no puede ser mayor que el que hay en la gaveta: lo que
	// se entregó de vuelto salió.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	antes := svc.SaldosDeTesoreria(empDemo)
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 200, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("emitir: %v", err)
	}
	despues := svc.SaldosDeTesoreria(empDemo)
	// Entró 200 y se devolvieron 84 (200 − 116): la gaveta sube 116.
	if !casi(despues.EfectivoBs-antes.EfectivoBs, 116) {
		t.Errorf("el efectivo debía subir 116 (200 cobrados − 84 de vuelto), subió %v",
			despues.EfectivoBs-antes.EfectivoBs)
	}
	if !casi(despues.VueltoEntregadoBs-antes.VueltoEntregadoBs, 84) {
		t.Errorf("el vuelto entregado debía sumar 84, sumó %v", despues.VueltoEntregadoBs-antes.VueltoEntregadoBs)
	}
}

func TestReporteIGTF_SaleDeLosDocumentos(t *testing.T) {
	// El reporte no es un registro paralelo: se deriva de los documentos, así que
	// no puede desincronizarse de lo facturado.
	svc, _ := nuevoServicio(t)
	_, total := svc.ReporteIGTF(empDemo)
	if total <= 0 {
		t.Fatal("el seed trae ventas en divisas: el total de IGTF no puede ser cero")
	}
	ops, _ := svc.ReporteIGTF(empDemo)
	for _, o := range ops {
		if o.IGTF <= 0 {
			t.Error("el reporte solo lista operaciones que causaron IGTF")
		}
		// La base declarada tiene que ser coherente con el impuesto.
		if !casi(o.BaseDivisas*0.03, o.IGTF) {
			t.Errorf("base %v × 3%% debería dar %v", o.BaseDivisas, o.IGTF)
		}
	}
}

// --- Contabilidad ---

func TestAsientoDeVenta_CuadraYSeparaLasBases(t *testing.T) {
	// El asiento sale de la venta, no de una captura manual, y tiene que cuadrar
	// por construcción. Además separa venta gravada de exenta y el IVA que la
	// empresa solo retiene.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{
			{SKU: "HAR-001", Cantidad: 1, PrecioUnitario: 100}, // exento
			{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100},  // gravado
		},
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 216, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	var venta *contabilidad.Asiento
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefID == doc.ID && !a.Contrario && a.Descripcion == "Factura "+doc.NumeroCompleto+" emitida" {
			cp := a
			venta = &cp
		}
	}
	if venta == nil {
		t.Fatal("emitir una factura tiene que generar su asiento")
	}
	if !venta.Cuadra() {
		t.Error("el asiento de la venta no cuadra")
	}
	haberDe := func(codigo string) float64 {
		for _, l := range venta.Lineas {
			if l.Codigo == codigo {
				return l.Haber
			}
		}
		return 0
	}
	if !casi(haberDe(contabilidad.CtaVentas), 100) {
		t.Errorf("ventas gravadas esperadas 100, se obtuvo %v", haberDe(contabilidad.CtaVentas))
	}
	if !casi(haberDe(contabilidad.CtaVentasExentas), 100) {
		t.Errorf("ventas exentas esperadas 100, se obtuvo %v", haberDe(contabilidad.CtaVentasExentas))
	}
	if !casi(haberDe(contabilidad.CtaIVADebito), 16) {
		t.Errorf("IVA débito esperado 16, se obtuvo %v", haberDe(contabilidad.CtaIVADebito))
	}
}

func TestAsientoDeVentaACredito_VaACuentasPorCobrar(t *testing.T) {
	// Lo que no se cobró no entra a caja: entra a cuentas por cobrar.
	svc, _ := nuevoServicio(t)
	doc := aCredito(t, svc, 1) // 116, sin abono
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefID != doc.ID || a.Contrario {
			continue
		}
		for _, l := range a.Lineas {
			if l.Codigo == contabilidad.CtaCajaBancos && l.Debe > 0.004 {
				t.Error("una venta a crédito sin abono no debe mover caja")
			}
		}
	}
	balance := svc.Balance(empDemo)
	for _, c := range balance.Cuentas {
		if c.Codigo == contabilidad.CtaCuentasPorCobrar && !casi(c.Saldo, 116) {
			t.Errorf("cuentas por cobrar esperadas 116, se obtuvo %v", c.Saldo)
		}
	}
}

func TestElLibroSiempreCuadra(t *testing.T) {
	// La prueba que protege el principio: pase lo que pase con las operaciones, el
	// balance de comprobación tiene que cuadrar.
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	// Venta de contado con vuelto, venta en divisas (IGTF), venta a crédito con
	// cobro y su reverso, y una anulación.
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 100, ""); err != nil {
		t.Fatalf("tasa: %v", err)
	}
	svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 500, Moneda: "VES"}},
	})
	svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoUSD, Monto: 5, Moneda: "USD"}},
	})
	doc := aCredito(t, svc, 2)
	cob, err := svc.RegistrarCobro(empDemo, sede1, actorA, origenTst, application.CobroEntrada{
		DocumentoID: doc.ID, Monto: 50, Moneda: "VES", Metodo: fiscal.PagoPagoMovil,
	})
	if err != nil {
		t.Fatalf("cobro: %v", err)
	}
	if _, err := svc.ReversarCobro(empDemo, actorA, origenTst, cob.ID, "error de tipeo"); err != nil {
		t.Fatalf("reverso: %v", err)
	}
	anulable, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir anulable: %v", err)
	}
	if _, err := svc.AnularDocumento(empDemo, sede1, actorA, origenTst, anulable.ID, "prueba"); err != nil {
		t.Fatalf("anular: %v", err)
	}

	b := svc.Balance(empDemo)
	if !b.Cuadra {
		t.Fatalf("el libro tiene que cuadrar siempre: debe %v vs haber %v", b.TotalDebe, b.TotalHaber)
	}
	if b.Asientos == 0 {
		t.Fatal("las operaciones debían generar asientos")
	}
	// Y cada asiento por separado también cuadra.
	for _, a := range svc.LibroDiario(empDemo) {
		if !a.Cuadra() {
			t.Errorf("el asiento %s no cuadra", a.Codigo)
		}
	}
}

func TestRevertirAsiento_AnexaElContrarioSinTocarElOriginal(t *testing.T) {
	svc, _ := nuevoServicio(t)
	abrirTurno(t, svc, actorA)
	doc, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	var original contabilidad.Asiento
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefID == doc.ID {
			original = a
			break
		}
	}
	rev, err := svc.RevertirAsiento(empDemo, actorA, origenTst, original.ID, "cuenta equivocada")
	if err != nil {
		t.Fatalf("revertir: %v", err)
	}
	if !rev.Contrario || rev.RefAsientoID != original.ID {
		t.Error("el contrario debe quedar marcado y apuntar al original")
	}
	// El original sigue igual en el libro.
	sigue := false
	for _, a := range svc.LibroDiario(empDemo) {
		if a.ID == original.ID && a.Total == original.Total && !a.Contrario {
			sigue = true
		}
	}
	if !sigue {
		t.Error("el asiento original no puede alterarse al revertirlo")
	}
	if _, err := svc.RevertirAsiento(empDemo, actorA, origenTst, original.ID, "otra vez"); !errors.Is(err, application.ErrAsientoYaRevertido) {
		t.Errorf("no se revierte dos veces: %v", err)
	}
	// Y el libro sigue cuadrando después de la corrección.
	if !svc.Balance(empDemo).Cuadra {
		t.Error("tras revertir, el libro tiene que seguir cuadrando")
	}
}

func TestSeedDemo_LosCodigosDeBarrasSonUnicos(t *testing.T) {
	// Dos productos con el mismo código hacen que un escaneo cobre el producto
	// equivocado. La validación existe al crear y al editar, pero el seed inserta
	// directo en el repositorio, así que se verifica acá.
	svc, _ := nuevoServicio(t)
	vistos := map[string]string{}
	for _, p := range svc.Productos(empDemo) {
		codigos := []string{p.CodigoBarras}
		for _, pr := range p.Presentaciones {
			codigos = append(codigos, pr.CodigoBarras)
		}
		for _, c := range codigos {
			if c == "" {
				continue
			}
			if otro, repetido := vistos[c]; repetido {
				t.Errorf("el código %s está en %s y en %s", c, otro, p.SKU)
			}
			vistos[c] = p.SKU
		}
	}
}
