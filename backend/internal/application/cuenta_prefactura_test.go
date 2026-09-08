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

// Con la cuenta ya pedida no se agregan renglones: la prefactura quedaría desactualizada
// y el cliente pagaría menos de lo que consumió. Anularla devuelve la mesa a servicio.
func TestPrefactura_BloqueaAgregarYSeAnula(t *testing.T) {
	svc, st := servicioSalon(t)
	c := cuentaConPedido(t, svc, st, "2")
	if _, _, err := svc.PrefacturarCuenta(empSalon, c.ID, "usr_meso", origenTst,
		application.DivisionCuenta{Modo: application.DivisionUnica}); err != nil {
		t.Fatalf("prefacturar: %v", err)
	}

	_, err := svc.AgregarItems(empSalon, c.ID, "usr_meso", usuario.RolMesonero, origenTst,
		[]application.ItemInput{{SKU: "POS-TORTA", Nombre: "Torta", Cantidad: 1, PrecioUnitario: 8500}})
	if !errors.Is(err, application.ErrCuentaYaPrefacturada) {
		t.Fatalf("no debe poder agregar sobre una cuenta pedida, se obtuvo: %v", err)
	}

	// Anular la prefactura devuelve la mesa a servicio y permite seguir agregando.
	if _, err := svc.CancelarPrefacturasCuenta(empSalon, c.ID, "usr_meso", origenTst); err != nil {
		t.Fatalf("anular prefactura: %v", err)
	}
	if m, ok := st.Mesas.ByID(empSalon, c.MesaID); ok && m.Estado != mesa.EstadoOcupada {
		t.Errorf("al volver a servicio la mesa debe quedar ocupada, quedó %q", m.Estado)
	}
	if _, err := svc.AgregarItems(empSalon, c.ID, "usr_meso", usuario.RolMesonero, origenTst,
		[]application.ItemInput{{SKU: "POS-TORTA", Nombre: "Torta", Cantidad: 1, PrecioUnitario: 8500}}); err != nil {
		t.Errorf("tras anular la prefactura debe poder agregar: %v", err)
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
