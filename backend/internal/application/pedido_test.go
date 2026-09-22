package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/pedido"
)

/* PEDIDOS PARA LLEVAR.
 *
 * Lo que se prueba acá es que el ciclo sea UNO SOLO. La tentación al construir
 * esto es hacer dos flujos —uno de restaurante y otro de tienda— y entonces cada
 * regla nueva hay que escribirla dos veces y la segunda se olvida. Así que las
 * pruebas recorren el mismo camino en los dos negocios y verifican que termine
 * igual. */

func servicioPedidos(t *testing.T) (*application.Service, string) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConPedidos(st.Pedidos, st.CanalesPedido, st.ZonasPedido, st.Repartidores)
	svc.ConCuentas(st.Cuentas)
	return svc, empDemo
}

func entradaBase(sku string) application.EntradaPedido {
	return application.EntradaPedido{
		EmpresaID: empDemo, SedeID: sede1,
		Origen: pedido.OrigenManual, ClienteNombre: "Ana Pérez",
		Destino: pedido.Destino{Direccion: "Av. Principal, casa 4", Telefono: "04141234567"},
		Items:   []pedido.Item{{SKU: sku, Nombre: "Algo", Cantidad: 1, PrecioUnitario: 100}},
		Total:   100, Actor: actorA, OrigenEvento: origenTst,
	}
}

// El pedido de mostrador nace CONFIRMADO: una persona ya lo revisó al armarlo.
func TestPedido_ElManualNaceConfirmado(t *testing.T) {
	svc, emp := servicioPedidos(t)
	p, err := svc.CrearPedido(entradaBase(primerSKU(t, svc)))
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if p.Estado == pedido.EstadoNuevo {
		t.Fatal("el pedido de mostrador no debería quedarse esperando revisión")
	}
	if p.Numero == 0 {
		t.Fatal("el pedido necesita su número visible del local")
	}
	_ = emp
}

// El que entra por API nace NUEVO y espera: confirmar reserva inventario y
// factura, así que el default seguro es que alguien mire.
func TestPedido_ElDeAPINaceNuevo(t *testing.T) {
	svc, emp := servicioPedidos(t)
	in := entradaBase(primerSKU(t, svc))
	in.Origen = pedido.OrigenEcommerce
	p, err := svc.CrearPedido(in)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if p.Estado != pedido.EstadoNuevo {
		t.Fatalf("estado = %q, debía quedar en espera de revisión", p.Estado)
	}
	// Y se puede confirmar a mano.
	c, err := svc.ConfirmarPedido(emp, p.ID, actorA, origenTst)
	if err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if c.Estado == pedido.EstadoNuevo {
		t.Fatal("tras confirmar no debería seguir nuevo")
	}
}

// LA prueba del flujo único: el mismo recorrido en un negocio SIN producción
// termina igual que en uno con cocina. No hay dos ciclos.
func TestPedido_MismoCicloSinProduccion(t *testing.T) {
	svc, emp := servicioPedidos(t)
	// nuevoServicio no activa el módulo Restaurante: esto es una tienda.
	p, err := svc.CrearPedido(entradaBase(primerSKU(t, svc)))
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if p.Estado != pedido.EstadoEnPreparacion {
		t.Fatalf("estado = %q, se esperaba en preparación", p.Estado)
	}
	// Sin nada que producir no se abre cuenta: una cuenta vacía en un local sin
	// salón es un fantasma en una pantalla que nadie mira.
	if p.CuentaID != "" {
		t.Fatalf("se abrió una cuenta de cocina en un negocio sin producción: %q", p.CuentaID)
	}
	// Y quien arma el pedido lo marca listo, que es el mismo paso.
	listo, err := svc.MarcarListo(emp, p.ID, actorA, origenTst)
	if err != nil {
		t.Fatalf("marcar listo: %v", err)
	}
	if listo.Estado != pedido.EstadoListo {
		t.Fatalf("estado = %q", listo.Estado)
	}
	if listo.Tracking == "" {
		t.Fatal("al quedar listo tiene que emitirse el número de envío")
	}
}

// El tracking se emite al marcar LISTO, no al confirmar: es la llave del envío,
// y emitirlo antes obligaría a anularlo en cada pedido cancelado en preparación.
func TestPedido_ElTrackingSeEmiteAlEstarListo(t *testing.T) {
	svc, emp := servicioPedidos(t)
	p, _ := svc.CrearPedido(entradaBase(primerSKU(t, svc)))
	if p.Tracking != "" {
		t.Fatal("al confirmar todavía no hay envío que identificar")
	}
	listo, _ := svc.MarcarListo(emp, p.ID, actorA, origenTst)
	if listo.Tracking == "" {
		t.Fatal("falta el tracking")
	}
}

// El token de la página pública NO es el tracking: el tracking se imprime y se
// dicta por teléfono, así que si fuera la llave, cualquiera que oiga un número
// vería la dirección de otro cliente.
func TestPedido_ElTokenPublicoNoEsElTracking(t *testing.T) {
	svc, emp := servicioPedidos(t)
	p, _ := svc.CrearPedido(entradaBase(primerSKU(t, svc)))
	listo, _ := svc.MarcarListo(emp, p.ID, actorA, origenTst)
	if listo.TokenPublico == "" || listo.TokenPublico == listo.Tracking {
		t.Fatalf("token público = %q, tracking = %q", listo.TokenPublico, listo.Tracking)
	}
	if _, ok := svc.PedidoPorToken(listo.TokenPublico); !ok {
		t.Fatal("el token tiene que resolver la página pública")
	}
	if _, ok := svc.PedidoPorToken(listo.Tracking); ok {
		t.Fatal("el número de envío NO puede abrir la página pública")
	}
}

// Las transiciones imposibles se rechazan: con once estados y tres caminos de
// envío, comprobarlas a mano dejaría pasar un «entregado» sobre algo que nunca
// salió, y entonces el historial deja de servir para responder un reclamo.
func TestPedido_TransicionesImposibles(t *testing.T) {
	svc, emp := servicioPedidos(t)
	p, _ := svc.CrearPedido(entradaBase(primerSKU(t, svc)))
	// Recién confirmado/en preparación: no puede estar en ruta ni entregado.
	if _, err := svc.MarcarEnRuta(emp, p.ID, actorA, origenTst); err == nil {
		t.Fatal("un pedido en preparación no puede salir a ruta")
	}
	if _, err := svc.MarcarEntregado(emp, p.ID, actorA, origenTst, application.CierreEntrega{}); err == nil {
		t.Fatal("un pedido en preparación no puede estar entregado")
	}
}

// Rechazar y cancelar exigen motivo: un pedido que se cae sin explicación no
// deja aprender nada ni responderle al cliente.
func TestPedido_RechazarExigeMotivo(t *testing.T) {
	svc, emp := servicioPedidos(t)
	in := entradaBase(primerSKU(t, svc))
	in.Origen = pedido.OrigenEcommerce
	p, _ := svc.CrearPedido(in)
	if _, err := svc.RechazarPedido(emp, p.ID, actorA, origenTst, "  "); err == nil {
		t.Fatal("rechazar sin motivo debería fallar")
	}
	r, err := svc.RechazarPedido(emp, p.ID, actorA, origenTst, "fuera de horario")
	if err != nil {
		t.Fatalf("rechazar: %v", err)
	}
	if r.Estado != pedido.EstadoRechazado || r.MotivoCierre == "" {
		t.Fatalf("el rechazo tiene que quedar con su motivo: %+v", r.Estado)
	}
}

// Un canal que reintenta no puede crear dos pedidos: para eso existe la
// referencia externa.
func TestPedido_DeduplicaPorReferencia(t *testing.T) {
	svc, emp := servicioPedidos(t)
	canal, err := svc.GuardarCanalPedido(emp, actorA, origenTst, pedido.Canal{
		Nombre: "Tienda web", Origen: pedido.OrigenEcommerce, Activo: true,
	})
	if err != nil {
		t.Fatalf("canal: %v", err)
	}
	in := entradaBase(primerSKU(t, svc))
	in.Origen = pedido.OrigenEcommerce
	in.CanalID = canal.ID
	in.ReferenciaExterna = "WEB-1001"

	a, _ := svc.CrearPedido(in)
	b, _ := svc.CrearPedido(in)
	if a.ID != b.ID {
		t.Fatalf("el mismo pedido del canal entró dos veces: %s y %s", a.ID, b.ID)
	}
}

// La bitácora guarda CÓMO llegó el pedido a donde está. El estado es uno solo,
// pero el recorrido es lo que se necesita cuando alguien reclama.
func TestPedido_BitacoraGuardaElRecorrido(t *testing.T) {
	svc, emp := servicioPedidos(t)
	p, _ := svc.CrearPedido(entradaBase(primerSKU(t, svc)))
	listo, _ := svc.MarcarListo(emp, p.ID, actorA, origenTst)
	estados := []string{}
	for _, e := range listo.Bitacora {
		estados = append(estados, e.Estado)
	}
	quiero := []string{pedido.EstadoNuevo, pedido.EstadoConfirmado, pedido.EstadoEnPreparacion, pedido.EstadoListo}
	if len(estados) != len(quiero) {
		t.Fatalf("bitácora = %v, se esperaba %v", estados, quiero)
	}
	for i := range quiero {
		if estados[i] != quiero[i] {
			t.Fatalf("bitácora = %v, se esperaba %v", estados, quiero)
		}
	}
}

/* VARIAS APLICACIONES A LA VEZ.
 *
 * Una empresa puede estar publicada en Yummy, PedidosYa y su propia tienda al
 * mismo tiempo. Los tres son canales distintos con su propia configuración, y
 * sus numeraciones no se cruzan. */

func TestPedido_VariasAppsALaVez(t *testing.T) {
	svc, emp := servicioPedidos(t)
	sku := primerSKU(t, svc)

	yummy, _ := svc.GuardarCanalPedido(emp, actorA, origenTst, pedido.Canal{
		Nombre: "Yummy", Origen: pedido.OrigenAppCommerce, Activo: true,
		EnvioPropioDelCanal: true, MinutosAceptacion: 5,
	})
	pedidosya, _ := svc.GuardarCanalPedido(emp, actorA, origenTst, pedido.Canal{
		Nombre: "PedidosYa", Origen: pedido.OrigenAppCommerce, Activo: true,
	})
	tienda, _ := svc.GuardarCanalPedido(emp, actorA, origenTst, pedido.Canal{
		Nombre: "Tienda web", Origen: pedido.OrigenEcommerce, Activo: true,
		ConfirmacionAutomatica: true,
	})
	if len(svc.CanalesPedido(emp)) != 3 {
		t.Fatalf("se esperaban 3 canales configurados, hay %d", len(svc.CanalesPedido(emp)))
	}

	// DOS APPS PUEDEN USAR EL MISMO NÚMERO DE PEDIDO sin pisarse: la referencia
	// externa solo identifica dentro de su canal.
	a := entradaBase(sku)
	a.Origen, a.CanalID, a.ReferenciaExterna = pedido.OrigenAppCommerce, yummy.ID, "1001"
	b := entradaBase(sku)
	b.Origen, b.CanalID, b.ReferenciaExterna = pedido.OrigenAppCommerce, pedidosya.ID, "1001"
	pa, _ := svc.CrearPedido(a)
	pb, _ := svc.CrearPedido(b)
	if pa.ID == pb.ID {
		t.Fatal("dos apps distintas con el mismo número de pedido se fundieron en uno")
	}
	// Y cada pedido sabe de QUÉ app vino, no solo que vino de una app.
	if pa.CanalNombre != "Yummy" || pb.CanalNombre != "PedidosYa" {
		t.Fatalf("el pedido tiene que decir de qué app vino: %q / %q", pa.CanalNombre, pb.CanalNombre)
	}
	// Los dos números conviven: el interno y el de la app.
	if pa.Numero == 0 || pa.ReferenciaExterna != "1001" {
		t.Fatalf("faltan los dos números: interno=%d externo=%q", pa.Numero, pa.ReferenciaExterna)
	}
	if pa.Numero == pb.Numero {
		t.Fatal("el correlativo interno tiene que ser propio de ElERP, no el de la app")
	}

	// La ventana de aceptación es POR CANAL: Yummy la declaró, PedidosYa no.
	if pa.VenceAceptacion == "" {
		t.Fatal("el canal con ventana de aceptación debe fijar su reloj")
	}
	if pb.VenceAceptacion != "" {
		t.Fatal("un canal sin ventana no debería inventar una")
	}

	// La confirmación automática también es por canal: la tienda propia pasa sola.
	w := entradaBase(sku)
	w.Origen, w.CanalID, w.ReferenciaExterna = pedido.OrigenEcommerce, tienda.ID, "WEB-7"
	pw, _ := svc.CrearPedido(w)
	if pw.Estado == pedido.EstadoNuevo {
		t.Fatal("el canal con confirmación automática no debería quedarse esperando")
	}
	// Y el envío del canal manda: Yummy reparte con su flota.
	pa2, _ := svc.ConfirmarPedido(emp, pa.ID, actorA, origenTst)
	if pa2.ModoEnvio != pedido.EnvioCanal {
		t.Fatalf("modo de envío = %q, Yummy reparte con su propia flota", pa2.ModoEnvio)
	}
}
