package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cotizacion"
	"github.com/mornix/elerp/internal/domain/cuenta"
	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// cuentaConPedido abre una mesa del restaurante demo y le carga dos renglones.
func cuentaConPedido(t *testing.T, svc *application.Service, st *inmem.Store, mesaNombre string) cuenta.Cuenta {
	t.Helper()
	m := mesaPorNombre(t, st, mesaNombre)
	c, err := svc.AbrirCuenta(application.AperturaCuenta{
		EmpresaID: empSalon, SedeID: sedeSalon, MesaID: m.ID,
		MesoneroID: "usr_meso", MesoneroNombre: "Meso", RolActor: usuario.RolMesonero,
		Actor: "usr_meso", Origen: origenTst, Comensales: 2,
	})
	if err != nil {
		t.Fatalf("abrir cuenta: %v", err)
	}
	c, err = svc.AgregarItems(empSalon, c.ID, "usr_meso", usuario.RolMesonero, origenTst, []application.ItemInput{
		{SKU: "PLA-BOLONESA", Nombre: "Spaghetti a la boloñesa", Cantidad: 2, PrecioUnitario: 32000},
		{SKU: "BEB-REFRESCO", Nombre: "Refresco", Cantidad: 2, PrecioUnitario: 2200},
	})
	if err != nil {
		t.Fatalf("agregar items: %v", err)
	}
	return c
}

// Pedir la cuenta genera UNA prefactura confirmada, rotulada con la mesa, y la mesa pasa
// a «por cobrar». El cajero la busca por mesa, no por número de cotización.
func TestPrefactura_UnicaRotuladaConLaMesa(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")

	cta, prefs, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Modo: application.DivisionUnica, Comensales: 2})
	if err != nil {
		t.Fatalf("prefacturar: %v", err)
	}
	if len(prefs) != 1 {
		t.Fatalf("partes iguales NO debe partir el documento: se esperaba 1 prefactura, hay %d", len(prefs))
	}
	p := prefs[0]
	if p.Estado != cotizacion.EstadoConfirmada {
		t.Errorf("la prefactura debe quedar confirmada (cobrable), está %q", p.Estado)
	}
	if p.MesaNombre != "2" || p.CuentaMesaID != c.ID {
		t.Errorf("la prefactura debe enlazar la mesa y su cuenta: mesa=%q cuenta=%q", p.MesaNombre, p.CuentaMesaID)
	}
	if p.Notas == "" {
		t.Error("la nota debe nombrar la mesa: es como la encuentra el cajero")
	}
	// Los renglones van ENTEROS: la factura no debe mostrar cantidades fraccionadas.
	for _, l := range p.Lineas {
		if l.Cantidad != 2 {
			t.Errorf("el renglón %s debe ir entero (2), fue %v", l.SKU, l.Cantidad)
		}
	}
	if len(cta.Prefacturas) != 1 {
		t.Errorf("la cuenta debe guardar su prefactura, guardó %d", len(cta.Prefacturas))
	}
	if m, ok := st.Mesas.ByID(empSalon, c.MesaID); ok && m.Estado != mesa.EstadoPorCobrar {
		t.Errorf("la mesa debe quedar «por cobrar», quedó %q", m.Estado)
	}
}

// Dividir POR PRODUCTOS sí parte el documento: cada comensal con sus renglones, para que
// pueda pedir su propia factura.
func TestPrefactura_DivisionPorProductos(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")
	asignacion := map[string]int{}
	for i, it := range c.Items {
		asignacion[it.ID] = i + 1 // un renglón a cada parte
	}

	_, prefs, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{
			Modo: application.DivisionPorItems, Items: asignacion,
			Nombres: map[int]string{1: "Ana", 2: "Luis"},
		})
	if err != nil {
		t.Fatalf("prefacturar dividida: %v", err)
	}
	if len(prefs) != 2 {
		t.Fatalf("se esperaban 2 prefacturas, hay %d", len(prefs))
	}
	for _, p := range prefs {
		if len(p.Lineas) != 1 {
			t.Errorf("cada parte debe llevar su renglón: la prefactura %s tiene %d", p.NumeroCompleto, len(p.Lineas))
		}
		if p.Total <= 0 {
			t.Errorf("la prefactura %s quedó en cero", p.NumeroCompleto)
		}
	}
	// Y la suma de las partes es el consumo completo (nada se cobra dos veces ni se pierde).
	var suma float64
	for _, p := range prefs {
		suma += p.Subtotal
	}
	if suma != 2*32000+2*2200 {
		t.Errorf("la suma de las partes (%v) no cuadra con el consumo", suma)
	}
}

// Un renglón sin asignar dejaría plata sin cobrar: es un error, no algo a resolver por
// omisión.
func TestPrefactura_RenglonSinAsignarEsError(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")
	solounon := map[string]int{c.Items[0].ID: 1}

	_, _, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Modo: application.DivisionPorItems, Items: solounon})
	if !errors.Is(err, application.ErrDivisionInvalida) {
		t.Fatalf("un renglón sin asignar debe dar ErrDivisionInvalida, se obtuvo: %v", err)
	}
}

// Con una solicitud de facturación en curso la mesa SIGUE VIVA: lo que se agregue
// después queda sin pedir y entra en la próxima solicitud. Antes esto se bloqueaba —
// el modelo era una prefactura única por cuenta— y obligaba a anular para seguir.
func TestSolicitud_SeSigueConsumiendoDespues(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")
	if _, _, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Modo: application.DivisionUnica}); err != nil {
		t.Fatalf("prefacturar: %v", err)
	}

	out, err := svc.AgregarItems(empSalon, c.ID, "usr_meso", usuario.RolMesonero, origenTst,
		[]application.ItemInput{{SKU: "POS-TORTA", Nombre: "Torta", Cantidad: 1, PrecioUnitario: 8500}})
	if err != nil {
		t.Fatalf("con la cuenta pedida se debe poder seguir agregando: %v", err)
	}
	sueltos := out.ItemsSinFacturar()
	if len(sueltos) != 1 || sueltos[0].SKU != "POS-TORTA" {
		t.Fatalf("el renglón nuevo debe quedar sin pedir, quedaron %d: %+v", len(sueltos), sueltos)
	}

	// Y anular lo pendiente devuelve la mesa a servicio, liberando sus renglones.
	if _, err := svc.CancelarPrefacturasCuenta(empSalon, c.ID, "usr_meso", origenTst); err != nil {
		t.Fatalf("anular solicitud: %v", err)
	}
	if m, ok := st.Mesas.ByID(empSalon, c.MesaID); ok && m.Estado != mesa.EstadoOcupada {
		t.Errorf("al volver a servicio la mesa debe quedar ocupada, quedó %q", m.Estado)
	}
	tras, _ := svc.Cuenta(empSalon, c.ID)
	if len(tras.Prefacturas) != 0 {
		t.Errorf("no debe quedar ninguna solicitud viva, quedaron %d", len(tras.Prefacturas))
	}
	for _, it := range tras.Items {
		if it.Estado != cuenta.ItemCancelado && it.PrefacturaID != "" {
			t.Errorf("el renglón %s debe quedar libre tras anular, sigue en %s", it.SKU, it.PrefacturaID)
		}
	}
}

// Los renglones CANCELADOS no se cobran.
func TestPrefactura_IgnoraRenglonesCancelados(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")
	// Enviar a cocina y luego cancelar uno (ya enviado ⇒ queda marcado cancelado).
	if _, _, err := svc.EnviarACocina(empSalon, c.ID, "usr_meso", origenTst); err != nil {
		t.Fatalf("enviar a cocina: %v", err)
	}
	if _, err := svc.CancelarItem(empSalon, c.ID, c.Items[1].ID, "usr_caja", usuario.RolCajero, origenTst); err != nil {
		t.Fatalf("cancelar renglón: %v", err)
	}
	_, prefs, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Modo: application.DivisionUnica})
	if err != nil {
		t.Fatalf("prefacturar: %v", err)
	}
	if len(prefs[0].Lineas) != 1 {
		t.Errorf("el renglón cancelado no debe cobrarse: la prefactura tiene %d renglones", len(prefs[0].Lineas))
	}
}

// El circuito completo: el mesonero prefactura, la CAJA factura y la cuenta se cierra
// sola liberando la mesa. Es lo que hace que nadie tenga que cerrar la mesa a mano.
func TestPrefactura_AlFacturarCierraLaCuentaYLiberaLaMesa(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")
	_, prefs, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Modo: application.DivisionUnica, Comensales: 2})
	if err != nil {
		t.Fatalf("prefacturar: %v", err)
	}
	total := prefs[0].Total

	// La caja cobra: dos pagos (el reparto entre comensales es del COBRO, no del
	// documento) sobre una única factura.
	mitad := round2Test(total / 2)
	_, doc, err := svc.FacturarCotizacion(empSalon, prefs[0].ID, "usr_caja", origenTst,
		application.EntradaFacturacion{Pagos: []application.PagoEntrada{
			{Metodo: "efectivo_bs", Monto: mitad, Moneda: "VES"},
			{Metodo: "punto_venta", Monto: round2Test(total - mitad), Moneda: "VES"},
		}})
	if err != nil {
		t.Fatalf("facturar la prefactura: %v", err)
	}
	if doc.Total != total {
		t.Errorf("la factura debe cobrar el total de la prefactura (%v), cobró %v", total, doc.Total)
	}
	if len(doc.Pagos) != 2 {
		t.Errorf("se esperaban 2 pagos (uno por comensal), hay %d", len(doc.Pagos))
	}

	cta, ok := st.Cuentas.ByID(empSalon, c.ID)
	if !ok {
		t.Fatal("la cuenta desapareció")
	}
	if cta.Estado != cuenta.EstadoCerrada {
		t.Errorf("la cuenta debe cerrarse al cobrar, quedó %q", cta.Estado)
	}
	if cta.DocumentoID != doc.ID {
		t.Errorf("la cuenta debe enlazar la factura emitida")
	}
	if m, ok := st.Mesas.ByID(empSalon, c.MesaID); ok && m.Estado != mesa.EstadoLibre {
		t.Errorf("la mesa debe quedar libre tras cobrar, quedó %q", m.Estado)
	}
}

// Con la cuenta dividida, la mesa se libera solo cuando se cobraron TODAS las partes:
// liberarla con una parte sin pagar perdería la deuda de vista.
func TestPrefactura_DivididaLiberaLaMesaAlCobrarTodas(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")
	asignacion := map[string]int{}
	for i, it := range c.Items {
		asignacion[it.ID] = i + 1
	}
	_, prefs, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Modo: application.DivisionPorItems, Items: asignacion})
	if err != nil {
		t.Fatalf("prefacturar: %v", err)
	}

	// Se cobra SOLO la primera parte.
	if _, _, err := svc.FacturarCotizacion(empSalon, prefs[0].ID, "usr_caja", origenTst,
		application.EntradaFacturacion{Pagos: []application.PagoEntrada{
			{Metodo: "efectivo_bs", Monto: prefs[0].Total, Moneda: "VES"},
		}}); err != nil {
		t.Fatalf("facturar parte 1: %v", err)
	}
	if cta, _ := st.Cuentas.ByID(empSalon, c.ID); cta.Estado != cuenta.EstadoAbierta {
		t.Errorf("con una parte sin cobrar la cuenta NO debe cerrarse, quedó %q", cta.Estado)
	}

	// Y la segunda.
	if _, _, err := svc.FacturarCotizacion(empSalon, prefs[1].ID, "usr_caja", origenTst,
		application.EntradaFacturacion{Pagos: []application.PagoEntrada{
			{Metodo: "efectivo_bs", Monto: prefs[1].Total, Moneda: "VES"},
		}}); err != nil {
		t.Fatalf("facturar parte 2: %v", err)
	}
	if cta, _ := st.Cuentas.ByID(empSalon, c.ID); cta.Estado != cuenta.EstadoCerrada {
		t.Errorf("cobradas todas las partes, la cuenta debe cerrarse, quedó %q", cta.Estado)
	}
	if m, ok := st.Mesas.ByID(empSalon, c.MesaID); ok && m.Estado != mesa.EstadoLibre {
		t.Errorf("la mesa debe quedar libre, quedó %q", m.Estado)
	}
}

// Antes de enviar a cocina el mesonero corrige el pedido libremente; después NO. Anular
// algo que la cocina ya está preparando cuesta comida, así que lo autoriza la caja.
func TestCancelarItem_SoloAntesDeEnviarParaElMesonero(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")

	// PENDIENTE: el mesonero lo elimina, y desaparece de la cuenta (no queda rastro:
	// corregir lo que se acaba de tocar es parte de tomar el pedido).
	out, err := svc.CancelarItem(empSalon, c.ID, c.Items[0].ID, "usr_meso", usuario.RolMesonero, origenTst)
	if err != nil {
		t.Fatalf("un renglón pendiente debe poder eliminarse: %v", err)
	}
	if len(out.Items) != len(c.Items)-1 {
		t.Errorf("el renglón pendiente debe ELIMINARSE, quedaron %d de %d", len(out.Items), len(c.Items))
	}

	// Enviado a cocina: el mesonero ya no puede.
	if _, _, err := svc.EnviarACocina(empSalon, c.ID, "usr_meso", origenTst); err != nil {
		t.Fatalf("enviar a cocina: %v", err)
	}
	enviado, _ := st.Cuentas.ByID(empSalon, c.ID)
	_, err = svc.CancelarItem(empSalon, c.ID, enviado.Items[0].ID, "usr_meso", usuario.RolMesonero, origenTst)
	if !errors.Is(err, application.ErrItemYaEnviado) {
		t.Fatalf("el mesonero no debe anular un renglón ya en cocina, se obtuvo: %v", err)
	}

	// La CAJA sí, y el renglón queda marcado «cancelado» (no se borra): el arqueo tiene
	// que poder explicar el faltante.
	out, err = svc.CancelarItem(empSalon, c.ID, enviado.Items[0].ID, "usr_caja", usuario.RolCajero, origenTst)
	if err != nil {
		t.Fatalf("la caja debe poder anularlo: %v", err)
	}
	if len(out.Items) != len(enviado.Items) {
		t.Error("un renglón ya enviado no se borra: se marca cancelado para que quede rastro")
	}
	// Y queda en «cancelado», no en cualquier otro estado: sin esta comprobación, un
	// renglón que quedara «servido» pasaría el conteo de arriba y se cobraría igual.
	for _, it := range out.Items {
		if it.ID == enviado.Items[0].ID && it.Estado != cuenta.ItemCancelado {
			t.Errorf("el renglón anulado por la caja debe quedar %q, quedó %q", cuenta.ItemCancelado, it.Estado)
		}
	}
	if out.Items[0].Estado != cuenta.ItemCancelado {
		t.Errorf("el renglón debe quedar «cancelado», quedó %q", out.Items[0].Estado)
	}
}

// Antes de enviar a cocina el mesonero corrige libremente; después NO. Anular algo que
// ya se está preparando cuesta comida, así que esa decisión es de la caja o la dueña —
// y queda registrada, no se borra.
func TestCancelarItem_MesoneroSoloAntesDeEnviar(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")

	// PENDIENTE: el mesonero lo elimina y desaparece de la cuenta.
	antes := len(c.Items)
	out, err := svc.CancelarItem(empSalon, c.ID, c.Items[1].ID, "usr_meso", usuario.RolMesonero, origenTst)
	if err != nil {
		t.Fatalf("un renglón pendiente lo debe poder quitar: %v", err)
	}
	if len(out.Items) != antes-1 {
		t.Errorf("el renglón pendiente se elimina de verdad: quedaron %d de %d", len(out.Items), antes)
	}

	// Ya en cocina: el mesonero NO puede.
	if _, _, err := svc.EnviarACocina(empSalon, c.ID, "usr_meso", origenTst); err != nil {
		t.Fatalf("enviar a cocina: %v", err)
	}
	enviado := out.Items[0].ID
	if _, err := svc.CancelarItem(empSalon, c.ID, enviado, "usr_meso", usuario.RolMesonero, origenTst); !errors.Is(err, application.ErrItemYaEnviado) {
		t.Fatalf("el mesonero no debe anular lo ya enviado, se obtuvo: %v", err)
	}

	// La caja sí, y el renglón queda CANCELADO (no se borra): el arqueo tiene que poder
	// explicar el faltante.
	trasCaja, err := svc.CancelarItem(empSalon, c.ID, enviado, "usr_caja", usuario.RolCajero, origenTst)
	if err != nil {
		t.Fatalf("la caja debe poder anularlo: %v", err)
	}
	var visto bool
	for _, it := range trasCaja.Items {
		if it.ID == enviado {
			visto = true
			if it.Estado != cuenta.ItemCancelado {
				t.Errorf("debe quedar cancelado, quedó %q", it.Estado)
			}
		}
	}
	if !visto {
		t.Error("un renglón ya enviado no se borra: queda en la cuenta marcado como cancelado")
	}
}

// ===================== SOLICITUDES DE FACTURACIÓN SEGMENTADAS =====================
//
// Dos personas en la misma mesa, cada una paga lo suyo: el mesonero manda una solicitud
// con los renglones de una y, cuando la otra termine, manda la suya. La mesa solo se
// cierra cuando ya no queda consumo sin pedir Y todo lo pedido está cobrado.

// El mesonero segmenta: pide factura SOLO de unos renglones y el resto sigue en la mesa.
func TestSolicitud_SegmentaLaMesa(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2") // [0] spaghetti, [1] refresco

	cta, prefs, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Seleccion: []string{c.Items[0].ID}})
	if err != nil {
		t.Fatalf("solicitar por segmento: %v", err)
	}
	if len(prefs) != 1 || len(prefs[0].Lineas) != 1 || prefs[0].Lineas[0].SKU != "PLA-BOLONESA" {
		t.Fatalf("la solicitud debe llevar SOLO el renglón elegido, llevó %+v", prefs)
	}
	sueltos := cta.ItemsSinFacturar()
	if len(sueltos) != 1 || sueltos[0].SKU != "BEB-REFRESCO" {
		t.Fatalf("el resto de la mesa debe quedar disponible, quedó %+v", sueltos)
	}

	// La segunda solicitud se lleva lo que quedaba, sin repetir nada.
	cta2, prefs2, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{})
	if err != nil {
		t.Fatalf("segunda solicitud: %v", err)
	}
	if len(prefs2) != 1 || len(prefs2[0].Lineas) != 1 || prefs2[0].Lineas[0].SKU != "BEB-REFRESCO" {
		t.Fatalf("la segunda solicitud debe llevar solo lo que faltaba, llevó %+v", prefs2)
	}
	if len(cta2.Prefacturas) != 2 {
		t.Errorf("la cuenta debe acumular las 2 solicitudes, tiene %d", len(cta2.Prefacturas))
	}
	if cta2.TieneSinFacturar() {
		t.Error("ya no debe quedar consumo sin pedir")
	}

	// Y una tercera no tiene nada que pedir: es un error explícito, no una solicitud vacía.
	if _, _, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{}); !errors.Is(err, application.ErrNadaPorFacturar) {
		t.Errorf("sin consumo suelto debe decir que no queda nada, dio: %v", err)
	}
}

// Un renglón que ya se llevó otra solicitud no se puede volver a pedir.
func TestSolicitud_NoRepiteRenglon(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")
	if _, _, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Seleccion: []string{c.Items[0].ID}}); err != nil {
		t.Fatalf("primera solicitud: %v", err)
	}
	_, _, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Seleccion: []string{c.Items[0].ID}})
	if !errors.Is(err, application.ErrItemYaFacturado) {
		t.Fatalf("pedir dos veces el mismo renglón debe fallar, dio: %v", err)
	}
}

// La mesa se cierra SOLA cuando se cobró todo lo pedido y no queda consumo suelto.
// Mientras falte cobrar una parte —o alguien siga comiendo— la mesa sigue viva.
func TestSolicitud_MesaSeCierraCuandoTodoEstaPago(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")

	_, prefs1, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Seleccion: []string{c.Items[0].ID}})
	if err != nil {
		t.Fatalf("solicitud 1: %v", err)
	}
	// Se cobra la primera parte: la mesa NO se cierra, queda consumo sin pedir.
	if _, _, err := svc.FacturarCotizacion(empSalon, prefs1[0].ID, "usr_caja", origenTst,
		application.EntradaFacturacion{Pagos: []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 100000, Moneda: "VES"}}}); err != nil {
		t.Fatalf("facturar parte 1: %v", err)
	}
	viva, _ := svc.Cuenta(empSalon, c.ID)
	if viva.Estado != cuenta.EstadoAbierta {
		t.Fatalf("con consumo sin pedir la mesa debe seguir abierta, quedó %q", viva.Estado)
	}
	if m, ok := st.Mesas.ByID(empSalon, c.MesaID); ok && m.Estado == mesa.EstadoLibre {
		t.Error("la mesa no debe liberarse: el otro comensal sigue en ella")
	}

	// Se pide y se cobra el resto: ahí sí cierra sola.
	_, prefs2, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst, application.DivisionCuenta{})
	if err != nil {
		t.Fatalf("solicitud 2: %v", err)
	}
	if _, _, err := svc.FacturarCotizacion(empSalon, prefs2[0].ID, "usr_caja", origenTst,
		application.EntradaFacturacion{Pagos: []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 10000, Moneda: "VES"}}}); err != nil {
		t.Fatalf("facturar parte 2: %v", err)
	}
	cerrada, _ := svc.Cuenta(empSalon, c.ID)
	if cerrada.Estado != cuenta.EstadoCerrada {
		t.Errorf("con todo cobrado la mesa debe cerrarse sola, quedó %q", cerrada.Estado)
	}
	if m, ok := st.Mesas.ByID(empSalon, c.MesaID); ok && m.Estado != mesa.EstadoLibre {
		t.Errorf("la mesa debe quedar libre, quedó %q", m.Estado)
	}
}

// El mesonero puede mandar la solicitud con el cliente ya identificado; y si la manda
// sin datos, el cajero los pone al cobrar y la factura sale a ese cliente.
func TestSolicitud_ClienteDelMesoneroODelCajero(t *testing.T) {
	svc, st := servicioSalon(t)
	clienteSalon := primerClienteSalon(t, st)

	// a) con cliente desde la mesa
	c := cuentaConPedido(t, svc, st, "2")
	_, prefs, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Seleccion: []string{c.Items[0].ID}, ClienteID: clienteSalon})
	if err != nil {
		t.Fatalf("solicitud con cliente: %v", err)
	}
	if prefs[0].ClienteID != clienteSalon {
		t.Errorf("la solicitud debe llevar el cliente que tomó el mesonero, lleva %q", prefs[0].ClienteID)
	}

	// b) sin cliente: lo pone el cajero al facturar
	_, prefs2, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst, application.DivisionCuenta{})
	if err != nil {
		t.Fatalf("solicitud sin cliente: %v", err)
	}
	if prefs2[0].ClienteID != "" {
		t.Fatalf("esta solicitud debe venir sin cliente, trae %q", prefs2[0].ClienteID)
	}
	_, doc, err := svc.FacturarCotizacion(empSalon, prefs2[0].ID, "usr_caja", origenTst,
		application.EntradaFacturacion{
			ClienteID: clienteSalon,
			Pagos:     []application.PagoEntrada{{Metodo: "efectivo_bs", Monto: 10000, Moneda: "VES"}},
		})
	if err != nil {
		t.Fatalf("facturar poniendo el cliente en caja: %v", err)
	}
	if doc.ClienteID != clienteSalon {
		t.Errorf("la factura debe salir al cliente que puso el cajero, salió a %q", doc.ClienteID)
	}
}

// primerClienteSalon devuelve un cliente sembrado del restaurante demo: los ids se
// generan al sembrar, así que se resuelve por catálogo y no se teclea.
func primerClienteSalon(t *testing.T, st *inmem.Store) string {
	t.Helper()
	cs := st.Clientes.List(empSalon)
	if len(cs) == 0 {
		t.Fatal("el restaurante demo debería tener clientes sembrados")
	}
	return cs[0].ID
}
