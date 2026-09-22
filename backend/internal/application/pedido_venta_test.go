package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/pedido"
)

/* VENDER CON ENVÍO desde el mostrador o desde el módulo de ventas.
 *
 * Lo que se prueba acá es la parte que se paga con plata real: que el flete se
 * FACTURE (no es un dato al margen: es un servicio gravado), que su costo salga
 * de la zona y no del teclado del cajero, y que la venta que no se puede llevar
 * se rechace ANTES de emitir —una factura hecha no se deshace, y un cliente
 * esperando algo que nadie puede despachar es el caso peor de este módulo.
 */

func servicioEnvioVenta(t *testing.T) (*application.Service, string) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConPedidos(st.Pedidos, st.CanalesPedido, st.ZonasPedido, st.Repartidores)
	svc.ConCuentas(st.Cuentas)
	// La semilla trae sus propias zonas. Se apagan para que cada prueba mida la
	// zona que ella dibujó y no la del demo: si no, lo que se estaría probando es
	// la semilla.
	for _, z := range svc.ZonasPedido(empDemo, sede1) {
		z.Activa = false
		if _, err := svc.GuardarZonaPedido(empDemo, actorA, origenTst, z); err != nil {
			t.Fatalf("apagar zona sembrada: %v", err)
		}
	}
	return svc, empDemo
}

// El servicio de envío se da de alta solo: quien activa el módulo no tiene por
// qué saber que además debe crear un producto para poder cobrar el flete.
func TestEnvioVenta_ElServicioSeCreaSolo(t *testing.T) {
	svc, emp := servicioEnvioVenta(t)
	p, err := svc.AsegurarServicioEnvio(emp, actorA, origenTst)
	if err != nil {
		t.Fatalf("asegurar: %v", err)
	}
	if p.SKU != application.SKUServicioEnvio {
		t.Fatalf("SKU = %q, se esperaba %q", p.SKU, application.SKUServicioEnvio)
	}
	if !p.EsServicio {
		t.Fatal("el flete NO es mercancía: sin la marca de servicio cada envío cobrado deja el Kardex con una unidad negativa más")
	}
	// Llamar dos veces no puede duplicar el producto: se factura por SKU.
	otra, err := svc.AsegurarServicioEnvio(emp, actorA, origenTst)
	if err != nil {
		t.Fatalf("asegurar de nuevo: %v", err)
	}
	if otra.ID != p.ID {
		t.Fatal("el servicio de envío se duplicó: la segunda venta facturaría otro producto")
	}
}

// Sin zonas dibujadas el envío NO se bloquea: un local que recién empieza tiene
// que poder despachar igual, y ya pondrá sus zonas después.
func TestEnvioVenta_SinZonasNoBloquea(t *testing.T) {
	svc, emp := servicioEnvioVenta(t)
	cot := svc.CotizarEnvio(emp, sede1, 10.5, -66.9, 500)
	if !cot.Cubierta || !cot.SinZonas {
		t.Fatalf("sin zonas configuradas el envío debería pasar, y avisar: %+v", cot)
	}
	if cot.Costo != 0 {
		t.Fatalf("sin zonas no hay costo que cobrar, y salió %v", cot.Costo)
	}
}

// El costo sale de LA ZONA. Es la regla que sostiene todo lo demás: si cada
// cajero pudiera teclearlo, la misma dirección costaría distinto según quién
// cobre.
func TestEnvioVenta_ElCostoSaleDeLaZona(t *testing.T) {
	svc, emp := servicioEnvioVenta(t)
	if _, err := svc.FijarUbicacionSede(emp, sede1, actorA, origenTst, 10.5, -66.9, 0); err != nil {
		t.Fatalf("ubicar sede: %v", err)
	}
	if _, err := svc.GuardarZonaPedido(emp, actorA, origenTst, pedido.Zona{
		EmpresaID: emp, SedeID: sede1, Nombre: "Centro", RadioM: 3000,
		CostoEnvio: 40, MinutosPromesa: 25, Activa: true,
	}); err != nil {
		t.Fatalf("zona: %v", err)
	}
	// Mismo punto que el local: cae dentro de los 3 km.
	cot := svc.CotizarEnvio(emp, sede1, 10.5, -66.9, 500)
	if !cot.Cubierta || cot.ZonaNombre != "Centro" || cot.Costo != 40 {
		t.Fatalf("debería caer en Centro a 40: %+v", cot)
	}

	linea := application.LineaDeEnvio(cot.Costo, cot.ZonaNombre)
	if linea.SKU != application.SKUServicioEnvio || linea.PrecioUnitario != 40 || linea.Cantidad != 1 {
		t.Fatalf("el renglón del flete salió mal: %+v", linea)
	}
}

// Fuera de zona se dice ANTES de cobrar. Descubrirlo después deja la venta
// cobrada y a nadie que pueda llevarla.
func TestEnvioVenta_FueraDeZonaSeAvisaAntes(t *testing.T) {
	svc, emp := servicioEnvioVenta(t)
	if _, err := svc.FijarUbicacionSede(emp, sede1, actorA, origenTst, 10.5, -66.9, 0); err != nil {
		t.Fatalf("ubicar sede: %v", err)
	}
	if _, err := svc.GuardarZonaPedido(emp, actorA, origenTst, pedido.Zona{
		EmpresaID: emp, SedeID: sede1, Nombre: "Centro", RadioM: 1000,
		CostoEnvio: 40, Activa: true,
	}); err != nil {
		t.Fatalf("zona: %v", err)
	}
	// ~1 grado de latitud son ~111 km: muy lejos de un radio de 1 km.
	cot := svc.CotizarEnvio(emp, sede1, 11.5, -66.9, 500)
	if cot.Cubierta {
		t.Fatal("una dirección a 111 km de un radio de 1 km no puede darse por cubierta")
	}
	if cot.Motivo == "" {
		t.Fatal("el rechazo tiene que decir POR QUÉ, en castellano: es lo que el cajero le repite al cliente")
	}
}

// El pedido mínimo de la zona se evalúa contra la venta, no contra cero.
func TestEnvioVenta_ElMinimoDeLaZonaSeRespeta(t *testing.T) {
	svc, emp := servicioEnvioVenta(t)
	if _, err := svc.FijarUbicacionSede(emp, sede1, actorA, origenTst, 10.5, -66.9, 0); err != nil {
		t.Fatalf("ubicar sede: %v", err)
	}
	if _, err := svc.GuardarZonaPedido(emp, actorA, origenTst, pedido.Zona{
		EmpresaID: emp, SedeID: sede1, Nombre: "Centro", RadioM: 3000,
		CostoEnvio: 40, PedidoMinimo: 300, Activa: true,
	}); err != nil {
		t.Fatalf("zona: %v", err)
	}
	if cot := svc.CotizarEnvio(emp, sede1, 10.5, -66.9, 100); cot.Cubierta {
		t.Fatal("100 no alcanza el mínimo de 300 de la zona y se dio por bueno")
	}
	if cot := svc.CotizarEnvio(emp, sede1, 10.5, -66.9, 500); !cot.Cubierta {
		t.Fatal("500 supera el mínimo de 300 y se rechazó")
	}
}

// La venta ya cobrada NO se cobra otra vez en la puerta. Decirlo al revés es el
// error que más caro sale de este módulo.
func TestEnvioVenta_LoCobradoEnCajaNoSeCobraEnLaPuerta(t *testing.T) {
	svc, emp := servicioEnvioVenta(t)
	sku := primerSKU(t, svc)
	items := []pedido.Item{{SKU: sku, Nombre: "Algo", Cantidad: 1, PrecioUnitario: 100}}
	env := application.EnvioDeVenta{Direccion: "Av. Principal, casa 4", Telefono: "04141234567", CobraEnvio: true}

	pagado, err := svc.CrearPedidoDeVenta(emp, sede1, "doc_1", actorA, origenTst, env, items, 140, true)
	if err != nil {
		t.Fatalf("crear pedido de venta: %v", err)
	}
	if pagado.FormaPago != pedido.PagoEnCanal {
		t.Fatalf("la venta se cobró en la caja y el pedido quedó como %q: el repartidor cobraría dos veces", pagado.FormaPago)
	}
	if pagado.DocumentoID != "doc_1" {
		t.Fatal("el pedido tiene que recordar SU factura: es lo que cierra el círculo al anular o al reclamar el flete")
	}
	if pagado.Estado == pedido.EstadoNuevo {
		t.Fatal("lo facturado con el cliente delante ya fue revisado: no vuelve a la cola de confirmación")
	}

	credito, err := svc.CrearPedidoDeVenta(emp, sede1, "doc_2", actorA, origenTst, env, items, 140, false)
	if err != nil {
		t.Fatalf("crear pedido a crédito: %v", err)
	}
	if credito.FormaPago != pedido.PagoContraEntrega {
		t.Fatalf("la venta a crédito deja saldo y el repartidor sí cobra: quedó %q", credito.FormaPago)
	}
}
