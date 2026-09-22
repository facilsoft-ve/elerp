package application_test

import (
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/pedido"
)

/* CERRAR EL TURNO DEL REPARTIDOR.
 *
 * Lo que se prueba acá es lo que evita un faltante: que lo esperado se derive de
 * las entregas y no se teclee, que un pedido no se pueda liquidar dos veces, y
 * que una cuenta que no cuadra no pase en silencio.
 */

func servicioLiquidacion(t *testing.T) (*application.Service, string) {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConPedidos(st.Pedidos, st.CanalesPedido, st.ZonasPedido, st.Repartidores)
	svc.ConCuentas(st.Cuentas)
	svc.ConLiquidaciones(st.LiquidacionesPedido)
	// La semilla deja entregas cobradas sin liquidar a propósito (la demo tiene que
	// mostrar un turno por recibir). Se cierran acá para que cada prueba mida lo
	// que ella misma sembró: si no, lo que se estaría probando es la demo.
	for _, r := range svc.Repartidores(empDemo, sede1) {
		res, err := svc.PendienteDeLiquidar(empDemo, sede1, r.ID)
		if err != nil || len(res.Entregas) == 0 {
			continue
		}
		if _, err := svc.LiquidarRepartidor(empDemo, sede1, r.ID, actorA, origenTst, res.EsperadoBs, ""); err != nil {
			t.Fatalf("cerrar lo sembrado: %v", err)
		}
	}
	return svc, empDemo
}

// entregaCobrada lleva un pedido hasta «entregado» cobrando en la puerta, que es
// el único caso que genera plata que liquidar.
func entregaCobrada(t *testing.T, svc *application.Service, repartidorID string, total, cobrado float64) pedido.Pedido {
	t.Helper()
	in := entradaBase(primerSKU(t, svc))
	in.FormaPago = pedido.PagoContraEntrega
	in.Total = total
	p, err := svc.CrearPedido(in)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if p.Estado != pedido.EstadoListo {
		if _, err := svc.MarcarListo(empDemo, p.ID, actorA, origenTst); err != nil {
			t.Fatalf("listo: %v", err)
		}
	}
	if _, err := svc.AsignarRepartidor(empDemo, p.ID, repartidorID, actorA, origenTst); err != nil {
		t.Fatalf("asignar: %v", err)
	}
	if _, err := svc.MarcarEnRuta(empDemo, p.ID, actorA, origenTst); err != nil {
		t.Fatalf("en ruta: %v", err)
	}
	out, err := svc.MarcarEntregado(empDemo, p.ID, actorA, origenTst,
		application.CierreEntrega{Prueba: "entregado en mano", CobradoBs: cobrado})
	if err != nil {
		t.Fatalf("entregado: %v", err)
	}
	return out
}

func primerRepartidor(t *testing.T, svc *application.Service) string {
	t.Helper()
	reps := svc.Repartidores(empDemo, sede1)
	if len(reps) == 0 {
		t.Fatal("la semilla no trae repartidores")
	}
	return reps[0].ID
}

// Lo que el repartidor debe traer sale de SUS entregas. Si lo tecleara quien
// recibe, la liquidación no probaría nada.
func TestLiquidacion_LoEsperadoSaleDeLasEntregas(t *testing.T) {
	svc, emp := servicioLiquidacion(t)
	rep := primerRepartidor(t, svc)
	entregaCobrada(t, svc, rep, 100, 100)
	entregaCobrada(t, svc, rep, 250, 250)

	res, err := svc.PendienteDeLiquidar(emp, sede1, rep)
	if err != nil {
		t.Fatalf("pendiente: %v", err)
	}
	if len(res.Entregas) != 2 {
		t.Fatalf("debería haber 2 entregas por rendir, hay %d", len(res.Entregas))
	}
	if res.EsperadoBs != 350 {
		t.Fatalf("esperado = %v, debería ser 350", res.EsperadoBs)
	}
}

// Lo declarado en la puerta manda sobre el total: si el cliente pagó de menos,
// el faltante es del pedido y cargárselo al repartidor es como se pierde uno.
func TestLiquidacion_LoQueSeCobroEnLaPuertaMandaSobreElTotal(t *testing.T) {
	svc, emp := servicioLiquidacion(t)
	rep := primerRepartidor(t, svc)
	entregaCobrada(t, svc, rep, 200, 180) // el cliente no tenía los 200

	res, err := svc.PendienteDeLiquidar(emp, sede1, rep)
	if err != nil {
		t.Fatalf("pendiente: %v", err)
	}
	if res.EsperadoBs != 180 {
		t.Fatalf("esperado = %v: debe pedirse lo que cobró (180), no lo que valía (200)", res.EsperadoBs)
	}
}

// Un pedido se liquida UNA vez. Es la regla que impide cobrarle dos veces la
// misma plata al mismo repartidor.
func TestLiquidacion_UnPedidoNoSeLiquidaDosVeces(t *testing.T) {
	svc, emp := servicioLiquidacion(t)
	rep := primerRepartidor(t, svc)
	entregaCobrada(t, svc, rep, 100, 100)

	l, err := svc.LiquidarRepartidor(emp, sede1, rep, actorA, origenTst, 100, "")
	if err != nil {
		t.Fatalf("liquidar: %v", err)
	}
	if !l.Cuadra() || l.DiferenciaBs != 0 {
		t.Fatalf("100 contra 100 tiene que cuadrar: %+v", l)
	}
	if len(l.PedidoIDs) != 1 {
		t.Fatalf("el acta tiene que decir QUÉ cubrió: %+v", l.PedidoIDs)
	}

	res, err := svc.PendienteDeLiquidar(emp, sede1, rep)
	if err != nil {
		t.Fatalf("pendiente: %v", err)
	}
	if len(res.Entregas) != 0 {
		t.Fatal("la entrega ya liquidada volvió a aparecer pendiente: se le cobraría dos veces")
	}
	if _, err := svc.LiquidarRepartidor(emp, sede1, rep, actorA, origenTst, 100, ""); err == nil {
		t.Fatal("liquidar de nuevo sin nada pendiente tenía que fallar")
	}
}

// Un faltante sin explicación no sirve para decidir nada: ni para descontarlo,
// ni para perdonarlo, ni para buscar el error.
func TestLiquidacion_ElFaltanteExigeExplicacion(t *testing.T) {
	svc, emp := servicioLiquidacion(t)
	rep := primerRepartidor(t, svc)
	entregaCobrada(t, svc, rep, 300, 300)

	if _, err := svc.LiquidarRepartidor(emp, sede1, rep, actorA, origenTst, 250, ""); err == nil {
		t.Fatal("faltan 50 y se aceptó sin nota")
	}
	l, err := svc.LiquidarRepartidor(emp, sede1, rep, actorA, origenTst, 250,
		"se le quedó un billete al cliente, lo trae mañana")
	if err != nil {
		t.Fatalf("con nota debería pasar: %v", err)
	}
	if l.DiferenciaBs != -50 {
		t.Fatalf("la diferencia = %v, debería ser -50", l.DiferenciaBs)
	}
	if l.Cuadra() {
		t.Fatal("un acta con 50 de faltante no cuadra")
	}
}

// Lo que ya venía pagado por el canal NO pasó por las manos del repartidor: no
// se le puede pedir al volver.
func TestLiquidacion_LoPagadoEnElCanalNoSeLePide(t *testing.T) {
	svc, emp := servicioLiquidacion(t)
	rep := primerRepartidor(t, svc)
	in := entradaBase(primerSKU(t, svc))
	in.FormaPago = pedido.PagoEnCanal
	in.Total = 400
	p, err := svc.CrearPedido(in)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if p.Estado != pedido.EstadoListo {
		if _, err := svc.MarcarListo(emp, p.ID, actorA, origenTst); err != nil {
			t.Fatalf("listo: %v", err)
		}
	}
	if _, err := svc.AsignarRepartidor(emp, p.ID, rep, actorA, origenTst); err != nil {
		t.Fatalf("asignar: %v", err)
	}
	if _, err := svc.MarcarEnRuta(emp, p.ID, actorA, origenTst); err != nil {
		t.Fatalf("en ruta: %v", err)
	}
	if _, err := svc.MarcarEntregado(emp, p.ID, actorA, origenTst, application.CierreEntrega{Prueba: "ok"}); err != nil {
		t.Fatalf("entregado: %v", err)
	}
	res, err := svc.PendienteDeLiquidar(emp, sede1, rep)
	if err != nil {
		t.Fatalf("pendiente: %v", err)
	}
	for _, e := range res.Entregas {
		if e.PedidoID == p.ID {
			t.Fatal("un pedido pagado en el canal se le está pidiendo al repartidor")
		}
	}
}
