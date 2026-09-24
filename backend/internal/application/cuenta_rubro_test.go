package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* CUENTA DE INVENTARIO POR RUBRO.
 *
 * La prueba que sostiene el módulo es TestCuentaRubro_LoQueEntraSalePorLaMisma.
 * Si un rubro tiene cuenta propia y alguno de sus asientos se queda en la cuenta
 * general, esa cuenta crece para siempre y la general se va a negativo — con el
 * BALANCE CUADRANDO en los dos casos, porque el total no cambia. Es un descuadre
 * invisible dentro de un balance correcto, que es la peor forma de estar mal.
 *
 * La segunda que importa es TestCuentaRubro_SinConfigurarNadaCambia: mientras
 * nadie declare una cuenta, los asientos tienen que salir exactamente como antes. */

// cuentaInventarioAlterna da de alta una cuenta de activo para pruebas.
const ctaInvAlterna = "1202"

func crearCuentaAlterna(t *testing.T, svc *application.Service) {
	t.Helper()
	if _, err := svc.CrearCuenta(empDemo, ctaInvAlterna, "Inventario de electrónica",
		contabilidad.TipoActivo, "", actorA, origenTst); err != nil {
		t.Fatalf("crear cuenta: %v", err)
	}
}

// rubroPorNombre busca un rubro del seed por su nombre.
func rubroPorNombre(t *testing.T, svc *application.Service, nombre string) inventario.Rubro {
	t.Helper()
	for _, r := range svc.Rubros(empDemo) {
		if r.Nombre == nombre {
			return r
		}
	}
	t.Fatalf("el seed no trae el rubro %q; los que hay: %+v", nombre, svc.Rubros(empDemo))
	return inventario.Rubro{}
}

// rubroConCuenta le asigna a un rubro existente su cuenta de inventario.
func rubroConCuenta(t *testing.T, svc *application.Service, nombre, cuenta string) inventario.Rubro {
	t.Helper()
	r := rubroPorNombre(t, svc, nombre)
	out, err := svc.ActualizarCuentaRubro(empDemo, r.ID, cuenta, true, actorA, origenTst)
	if err != nil {
		t.Fatalf("asignar cuenta al rubro: %v", err)
	}
	return out
}

// saldoDeCuenta suma debe − haber de una cuenta en todo el diario.
func saldoDeCuenta(t *testing.T, svc *application.Service, codigo string) float64 {
	t.Helper()
	saldo := 0.0
	for _, a := range svc.LibroDiario(empDemo) {
		for _, l := range a.Lineas {
			if l.Codigo == codigo {
				saldo += l.Debe - l.Haber
			}
		}
	}
	return round2Test(saldo)
}

// TestCuentaRubro_ElMaestroSeDefiende: una cuenta que no existe, o que no es de
// activo, se rechaza AL DECLARARLA. Después sería tarde: un asiento contra una
// cuenta inexistente no se guarda, y el fallo aparecería como una ausencia de
// asientos que nadie relacionaría con haber tocado un rubro.
func TestCuentaRubro_ElMaestroSeDefiende(t *testing.T) {
	svc, _ := nuevoServicio(t)
	r := rubroPorNombre(t, svc, "Electrónica")

	if _, err := svc.ActualizarCuentaRubro(empDemo, r.ID, "9999", true, actorA, origenTst); !errors.Is(err, application.ErrCuentaNoExiste) {
		t.Errorf("una cuenta que no está en el plan debía rechazarse: %v", err)
	}
	if _, err := svc.ActualizarCuentaRubro(empDemo, r.ID, contabilidad.CtaVentas, true, actorA, origenTst); !errors.Is(err, application.ErrCuentaNoEsActivo) {
		t.Errorf("el inventario es un activo: una cuenta de ingreso debía rechazarse: %v", err)
	}
	if _, err := svc.ActualizarCuentaRubro(empDemo, "rub_inventado", contabilidad.CtaInventario, true, actorA, origenTst); !errors.Is(err, application.ErrRubroNoExiste) {
		t.Errorf("un rubro inventado debía rechazarse: %v", err)
	}
}

// TestCuentaRubro_NoReclasificaSinQueAlguienLoPida es la guarda de la decisión.
//
// Cambiarle la cuenta a un rubro CON existencia emite un asiento en un libro de
// solo-anexado: no se borra, se contra-asienta. Que salga solo, por elegir una
// opción de un desplegable, es pedir que alguien tenga que explicar un asiento que
// no recuerda haber hecho.
//
// La guarda vive en la APLICACIÓN y no en la pantalla a propósito: puesta en la
// interfaz, cualquier otra llamada al API movería el libro igual.
func TestCuentaRubro_NoReclasificaSinQueAlguienLoPida(t *testing.T) {
	svc := servicioCompleto(t)
	crearCuentaAlterna(t, svc)
	r := rubroPorNombre(t, svc, "Electrónica") // el seed le trae existencia

	// Primero se puede MIRAR lo que pasaría, sin tocar nada.
	prev, err := svc.PrevisualizarCuentaRubro(empDemo, r.ID, ctaInvAlterna)
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	if !prev.Asiento || prev.Valor <= 0 {
		t.Fatalf("este rubro tiene existencia: el cambio emite asiento (%+v)", prev)
	}
	if prev.CuentaActual != contabilidad.CtaInventario || prev.CuentaNueva != ctaInvAlterna {
		t.Errorf("la vista previa tiene que decir de dónde a dónde: %+v", prev)
	}
	if prev.Productos == 0 {
		t.Error("y cuántos productos arrastra")
	}
	asientosAntes := len(svc.LibroDiario(empDemo))

	// Sin confirmar, no se hace nada. NI el cambio NI el asiento.
	if _, err := svc.ActualizarCuentaRubro(empDemo, r.ID, ctaInvAlterna, false, actorA, origenTst); !errors.Is(err, application.ErrReclasificacionNoConfirmada) {
		t.Fatalf("sin confirmar tenía que negarse: %v", err)
	}
	if got := len(svc.LibroDiario(empDemo)); got != asientosAntes {
		t.Errorf("una negativa no puede dejar asientos: %d → %d", asientosAntes, got)
	}
	if rr := rubroPorNombre(t, svc, "Electrónica"); rr.CuentaInventario != "" {
		t.Errorf("tampoco puede haber cambiado la configuración: %q", rr.CuentaInventario)
	}

	// Confirmado sí.
	if _, err := svc.ActualizarCuentaRubro(empDemo, r.ID, ctaInvAlterna, true, actorA, origenTst); err != nil {
		t.Fatalf("confirmado tenía que aplicarse: %v", err)
	}
	if got := len(svc.LibroDiario(empDemo)); got != asientosAntes+1 {
		t.Errorf("tenía que emitirse exactamente un asiento: %d → %d", asientosAntes, got)
	}
}

// TestCuentaRubro_SinExistenciaNoPideConfirmacion: si el rubro no tiene mercancía,
// el cambio es solo configuración y no mueve el libro. Pedir confirmación ahí sería
// un trámite sin contenido, y los trámites sin contenido enseñan a aceptarlos sin
// leerlos — que es justo lo que no se quiere el día que sí importa.
func TestCuentaRubro_SinExistenciaNoPideConfirmacion(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	crearCuentaAlterna(t, svc)
	// Un rubro nuevo, sin productos: nada que reclasificar.
	vacio := st.Rubros.Create(inventario.Rubro{EmpresaID: empDemo, Nombre: "Rubro sin mercancía"})

	prev, err := svc.PrevisualizarCuentaRubro(empDemo, vacio.ID, ctaInvAlterna)
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	if prev.Asiento {
		t.Errorf("sin existencia no hay asiento que emitir: %+v", prev)
	}
	asientosAntes := len(svc.LibroDiario(empDemo))
	if _, err := svc.ActualizarCuentaRubro(empDemo, vacio.ID, ctaInvAlterna, false, actorA, origenTst); err != nil {
		t.Fatalf("sin asiento de por medio no hace falta confirmar: %v", err)
	}
	if got := len(svc.LibroDiario(empDemo)); got != asientosAntes {
		t.Errorf("no podía emitirse ningún asiento: %d → %d", asientosAntes, got)
	}
}

// TestCuentaRubro_SinConfigurarNadaCambia es la regresión que permite desplegar
// esto sobre datos vivos: mientras ningún rubro declare cuenta, TODO va a la
// general, exactamente como antes.
func TestCuentaRubro_SinConfigurarNadaCambia(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)

	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "sobrante de conteo", 5, actorA, origenTst); err != nil {
		t.Fatalf("ajuste: %v", err)
	}
	for _, a := range svc.LibroDiario(empDemo) {
		for _, l := range a.Lineas {
			if l.Codigo == ctaInvAlterna {
				t.Fatalf("sin configurar nada, ninguna línea puede ir a otra cuenta: %+v", a)
			}
		}
	}
	if saldoDeCuenta(t, svc, contabilidad.CtaInventario) == 0 {
		t.Error("el inventario tenía que moverse en la cuenta general")
	}
}

// TestCuentaRubro_LoQueEntraSalePorLaMisma es LA prueba del módulo.
//
// Se recorre el ciclo entero de un producto de un rubro con cuenta propia —compra,
// venta, merma— y al final la cuenta general no puede haberse movido ni un céntimo
// por ese producto. Si se moviera, las dos cuentas dejarían de significar lo que
// dicen y el balance seguiría cuadrando igual.
func TestCuentaRubro_LoQueEntraSalePorLaMisma(t *testing.T) {
	svc, _ := nuevoServicio(t)
	crearCuentaAlterna(t, svc)
	rubroConCuenta(t, svc, "Electrónica", ctaInvAlterna)

	sku := "ELE-1"
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: sku, Nombre: "Televisor", Precio: 200, Rubro: "Electrónica",
	}); err != nil {
		t.Fatalf("crear producto: %v", err)
	}

	// Se miden DELTAS y no saldos absolutos: el seed carga existencias sin emitir
	// asientos, así que el saldo de partida de estas cuentas no corresponde a nada
	// y compararlo con un número fijo probaría el seed, no el código.
	generalAntes := saldoDeCuenta(t, svc, contabilidad.CtaInventario)
	rubroAntes := saldoDeCuenta(t, svc, ctaInvAlterna)

	// 1 · Entra por compra.
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: 10, CostoUnitario: 50}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: 10}}); err != nil {
		t.Fatalf("recibir: %v", err)
	}
	if got := saldoDeCuenta(t, svc, ctaInvAlterna) - rubroAntes; !casi(got, 500) {
		t.Fatalf("la compra tenía que sumar 500 a la cuenta del rubro: %v", got)
	}

	// 2 · Sale por venta.
	abrirTurno(t, svc, actorA)
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		Lineas: []application.LineaEntrada{{SKU: sku, Cantidad: 4, PrecioUnitario: 200}},
		Pagos:  []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 2000, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("vender: %v", err)
	}

	// 3 · Sale por merma.
	if _, err := svc.Ajustar(empDemo, sede1, "", sku, "caja rota", -2, actorA, origenTst); err != nil {
		t.Fatalf("merma: %v", err)
	}

	// Entraron 10×50=500 y salieron 6×50=300: la cuenta del rubro sube 200 neto.
	if got := saldoDeCuenta(t, svc, ctaInvAlterna) - rubroAntes; !casi(got, 200) {
		t.Errorf("la cuenta del rubro tenía que subir 200 netos: %v", got)
	}
	// Y LA GENERAL NO SE MOVIÓ. Es lo que se vigila: un solo asiento que se hubiera
	// quedado ahí dejaría esta cuenta con un saldo que no corresponde a nada.
	if got := saldoDeCuenta(t, svc, contabilidad.CtaInventario); !casi(got, generalAntes) {
		t.Errorf("la cuenta general no podía moverse por un producto con cuenta propia: %v → %v", generalAntes, got)
	}
}

// TestCuentaRubro_DosRubrosEnUnaMismaCompra: el asiento se parte por cuenta y la
// suma no cambia. Es lo que hace seguro el reparto — si la suma cambiara, el
// asiento no cuadraría y NO SE GUARDARÍA: el fallo sería una ausencia, no un error.
func TestCuentaRubro_DosRubrosEnUnaMismaCompra(t *testing.T) {
	svc, _ := nuevoServicio(t)
	crearCuentaAlterna(t, svc)
	rubroConCuenta(t, svc, "Electrónica", ctaInvAlterna)

	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{SKU: "ELE-2", Nombre: "Radio", Precio: 10, Rubro: "Electrónica"})
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{SKU: "VIV-2", Nombre: "Harina", Precio: 10, Rubro: "Víveres"})

	generalAntes := saldoDeCuenta(t, svc, contabilidad.CtaInventario)
	rubroAntes := saldoDeCuenta(t, svc, ctaInvAlterna)

	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{
			{SKU: "ELE-2", Cantidad: 10, CostoUnitario: 30}, // 300 a la del rubro
			{SKU: "VIV-2", Cantidad: 10, CostoUnitario: 20}, // 200 a la general
		},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst)
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst, []application.LineaRecepcion{
		{SKU: "ELE-2", Cantidad: 10}, {SKU: "VIV-2", Cantidad: 10},
	}); err != nil {
		t.Fatalf("recibir: %v", err)
	}

	if got := saldoDeCuenta(t, svc, ctaInvAlterna) - rubroAntes; !casi(got, 300) {
		t.Errorf("a la cuenta del rubro tenían que ir 300: %v", got)
	}
	if got := saldoDeCuenta(t, svc, contabilidad.CtaInventario) - generalAntes; !casi(got, 200) {
		t.Errorf("a la general tenían que ir 200: %v", got)
	}
	// Y el asiento de la recepción tiene que existir y cuadrar: si el reparto
	// hubiera cambiado la suma, no se habría guardado y esto no lo encontraría.
	encontrado := false
	for _, a := range svc.LibroDiario(empDemo) {
		if a.RefTipo != "compra" || a.RefID != oc.ID {
			continue
		}
		encontrado = true
		debe, haber := 0.0, 0.0
		for _, l := range a.Lineas {
			debe += l.Debe
			haber += l.Haber
		}
		if !casi(debe, haber) {
			t.Errorf("el asiento partido en dos cuentas no cuadra: debe %v, haber %v", debe, haber)
		}
		if !casi(haber, 500) {
			t.Errorf("la contrapartida tenía que ser 500: %v", haber)
		}
	}
	if !encontrado {
		t.Fatal("no hay asiento de la recepción: si no cuadró, no se guardó")
	}
}

// TestCuentaRubro_LaDevolucionVuelveAlaMisma: devolver al proveedor tiene que
// descargar la cuenta del rubro, no la general. Si descargara la general, la del
// rubro quedaría con más de lo que hay en el anaquel, para siempre.
//
// Es el sexto y último sitio que mueve inventario. Que esté cubierto es lo que
// permite afirmar que el ciclo cierra: sin él, una devolución bastaría para que
// las dos cuentas dejaran de significar lo que dicen.
func TestCuentaRubro_LaDevolucionVuelveAlaMisma(t *testing.T) {
	svc, _ := nuevoServicio(t)
	crearCuentaAlterna(t, svc)
	rubroConCuenta(t, svc, "Electrónica", ctaInvAlterna)

	sku := "ELE-3"
	svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: sku, Nombre: "Audífonos", Precio: 60, Rubro: "Electrónica",
	})

	rubroAntes := saldoDeCuenta(t, svc, ctaInvAlterna)
	// ocFacturada recibe 10 u a Bs 20 y registra su factura: 200 a la cuenta del rubro.
	fc := ocFacturada(t, svc, sku, "F-RUB-1", "00-00044444", time.Now().UTC().Format(time.RFC3339Nano))
	if got := saldoDeCuenta(t, svc, ctaInvAlterna) - rubroAntes; !casi(got, 200) {
		t.Fatalf("la compra tenía que sumar 200 a la cuenta del rubro: %v", got)
	}
	generalAntes := saldoDeCuenta(t, svc, contabilidad.CtaInventario)

	if _, err := svc.EmitirNotaCreditoCompra(empDemo, actorA, origenTst, fc.ID, devolucionEntrada(sku, 3)); err != nil {
		t.Fatalf("devolver al proveedor: %v", err)
	}

	// Salen 3 × 20 = 60 de la cuenta del rubro: el neto de la compra baja a 140.
	if got := saldoDeCuenta(t, svc, ctaInvAlterna) - rubroAntes; !casi(got, 140) {
		t.Errorf("la devolución tenía que dejar el neto del rubro en 140: %v", got)
	}
	if got := saldoDeCuenta(t, svc, contabilidad.CtaInventario); !casi(got, generalAntes) {
		t.Errorf("la general no podía moverse: %v → %v", generalAntes, got)
	}
}
