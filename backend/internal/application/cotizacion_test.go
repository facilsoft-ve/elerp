package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cotizacion"
	"github.com/mornix/elerp/internal/domain/fiscal"
)

// clienteDemo devuelve el primer cliente del seed (para las cotizaciones que
// exigen un tercero identificado, p. ej. las de crédito).
func clienteDemo(t *testing.T, svc *application.Service) string {
	t.Helper()
	cl := svc.Clientes(empDemo)
	if len(cl) == 0 {
		t.Fatal("el seed debería traer al menos un cliente")
	}
	return cl[0].ID
}

func TestCrearCotizacion_RechazaVacia(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{}); !errors.Is(err, application.ErrCotizacionVacia) {
		t.Fatalf("una cotización sin líneas debe dar ErrCotizacionVacia, se obtuvo: %v", err)
	}
}

func TestCrearCotizacion_ProductoInexistente(t *testing.T) {
	svc, _ := nuevoServicio(t)
	_, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "NO-EXISTE", Cantidad: 1, PrecioUnitario: 10}},
	})
	if !errors.Is(err, application.ErrProductoNoExiste) {
		t.Fatalf("una línea con SKU inexistente debe dar ErrProductoNoExiste, se obtuvo: %v", err)
	}
}

// TestCrearCotizacion_CalculaComoElPOS comprueba que la cotización nace en
// borrador, numerada con la serie COT, y con los MISMOS totales fiscales que
// calcularía una factura (subtotal, IVA 16%, IGTF 0 hasta que se cobre).
func TestCrearCotizacion_CalculaComoElPOS(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 2, PrecioUnitario: 100}}, // gravado
	})
	if err != nil {
		t.Fatalf("crear cotización: %v", err)
	}
	if c.Estado != cotizacion.EstadoBorrador {
		t.Errorf("una cotización nueva nace en borrador, quedó %q", c.Estado)
	}
	if c.NumeroCompleto == "" || c.Numero == 0 {
		t.Errorf("la cotización debe salir folada con la serie COT, se obtuvo %q", c.NumeroCompleto)
	}
	if !casi(c.Subtotal, 200) {
		t.Errorf("subtotal esperado 200, se obtuvo %v", c.Subtotal)
	}
	if !casi(c.IVA, 32) {
		t.Errorf("IVA 16%% esperado 32, se obtuvo %v", c.IVA)
	}
	if !casi(c.Total, 232) {
		t.Errorf("total esperado 232, se obtuvo %v", c.Total)
	}
	if c.IGTF != 0 {
		t.Errorf("el IGTF se calcula al cobrar, en la cotización debe ser 0, se obtuvo %v", c.IGTF)
	}
	// Debe listarse y recuperarse por id.
	if _, ok := svc.Cotizacion(empDemo, c.ID); !ok {
		t.Error("la cotización recién creada debe recuperarse por id")
	}
	if len(svc.Cotizaciones(empDemo)) == 0 {
		t.Error("la cotización debe aparecer en el listado de la empresa")
	}
}

func TestCrearCotizacion_AplicaDescuentoDeLinea(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100, Descuento: 10}},
	})
	if err != nil {
		t.Fatalf("crear cotización: %v", err)
	}
	// 100 − 10% = 90 neto; IVA 16% de 90 = 14,40; total 104,40.
	if !casi(c.Subtotal, 90) {
		t.Errorf("subtotal con 10%% de descuento esperado 90, se obtuvo %v", c.Subtotal)
	}
	if !casi(c.IVA, 14.4) {
		t.Errorf("IVA esperado 14,40, se obtuvo %v", c.IVA)
	}
	if l := c.Lineas[0]; !casi(l.PrecioUnitario, 100) || l.Descuento != 10 {
		t.Errorf("la línea debe conservar el precio de lista 100 y el descuento 10, se obtuvo %+v", l)
	}
}

func TestActualizarCotizacion_SoloEnBorrador(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	// Editar el borrador recalcula.
	upd, err := svc.ActualizarCotizacion(empDemo, c.ID, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 3, PrecioUnitario: 100}},
	})
	if err != nil {
		t.Fatalf("actualizar borrador: %v", err)
	}
	if !casi(upd.Subtotal, 300) {
		t.Errorf("tras editar a 3 unidades el subtotal debe ser 300, se obtuvo %v", upd.Subtotal)
	}
	// Confirmada ya no se edita.
	if _, err := svc.ConfirmarCotizacion(empDemo, c.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if _, err := svc.ActualizarCotizacion(empDemo, c.ID, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
	}); !errors.Is(err, application.ErrTransicionCotizacion) {
		t.Errorf("editar una cotización confirmada debe dar ErrTransicionCotizacion, se obtuvo: %v", err)
	}
}

func TestActualizarCotizacion_NoExiste(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.ActualizarCotizacion(empDemo, "cot_fantasma", actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
	}); !errors.Is(err, application.ErrCotizacionNoExiste) {
		t.Fatalf("se esperaba ErrCotizacionNoExiste, se obtuvo: %v", err)
	}
}

func TestConfirmarCotizacion_TransicionValida(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	conf, err := svc.ConfirmarCotizacion(empDemo, c.ID, actorA, origenTst)
	if err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if conf.Estado != cotizacion.EstadoConfirmada {
		t.Errorf("tras confirmar el estado debe ser confirmada, quedó %q", conf.Estado)
	}
	// Confirmar dos veces ya no es una transición válida.
	if _, err := svc.ConfirmarCotizacion(empDemo, c.ID, actorA, origenTst); !errors.Is(err, application.ErrTransicionCotizacion) {
		t.Errorf("confirmar una cotización ya confirmada debe dar ErrTransicionCotizacion, se obtuvo: %v", err)
	}
}

// TestFacturarCotizacion_ReusaMotorFiscal comprueba que facturar una cotización
// pasa por la MISMA ruta fiscal que el POS: el documento emitido conserva el
// subtotal/IVA de la cotización y calcula el IGTF a partir de los pagos (parte en
// divisas), no de la cotización (que lo dejó en 0).
func TestFacturarCotizacion_ReusaMotorFiscal(t *testing.T) {
	svc, _ := nuevoServicio(t)
	if _, err := svc.CargarTasaManual(empDemo, actorA, origenTst, 40, ""); err != nil {
		t.Fatalf("cargar tasa: %v", err)
	}
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}}, // gravado
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := svc.ConfirmarCotizacion(empDemo, c.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	// Total de la cotización = 116 (100 + IVA 16). Al cobrar 1 US$ (=40 Bs) más el
	// resto en Bs, el IGTF (3% de 40 = 1,20) lo agrega el motor fiscal.
	out, doc, err := svc.FacturarCotizacion(empDemo, c.ID, actorA, origenTst, application.EntradaFacturacion{
		Pagos: []application.PagoEntrada{
			{Metodo: fiscal.PagoEfectivoBs, Monto: 77.20, Moneda: "VES"},
			{Metodo: fiscal.PagoEfectivoUSD, Monto: 1, Moneda: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("facturar cotización: %v", err)
	}
	if out.Estado != cotizacion.EstadoFacturada {
		t.Errorf("tras facturar el estado debe ser facturada, quedó %q", out.Estado)
	}
	if out.DocumentoID != doc.ID {
		t.Errorf("la cotización debe enlazar el documento emitido %q, quedó %q", doc.ID, out.DocumentoID)
	}
	if doc.Tipo != fiscal.TipoFactura || doc.NumeroCompleto == "" {
		t.Errorf("debe emitirse una factura fiscal numerada, se obtuvo %+v", []any{doc.Tipo, doc.NumeroCompleto})
	}
	if !casi(doc.Subtotal, c.Subtotal) || !casi(doc.IVA, c.IVA) {
		t.Errorf("el subtotal/IVA de la factura debe salir de la cotización: cot(%v,%v) doc(%v,%v)",
			c.Subtotal, c.IVA, doc.Subtotal, doc.IVA)
	}
	if !casi(doc.IGTF, 1.2) {
		t.Errorf("el IGTF debe calcularse de los pagos (3%% de 40 Bs = 1,20), se obtuvo %v", doc.IGTF)
	}
}

func TestFacturarCotizacion_SoloConfirmada(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	// En borrador (sin confirmar) no se puede facturar.
	if _, _, err := svc.FacturarCotizacion(empDemo, c.ID, actorA, origenTst, application.EntradaFacturacion{
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	}); !errors.Is(err, application.ErrTransicionCotizacion) {
		t.Fatalf("facturar un borrador debe dar ErrTransicionCotizacion, se obtuvo: %v", err)
	}
}

func TestFacturarCotizacion_ACreditoQuedaPorCobrar(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		ClienteID: clienteDemo(t, svc),
		Lineas:    []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := svc.ConfirmarCotizacion(empDemo, c.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	// A crédito sin pagos: el total (116) queda por cobrar en Tesorería.
	_, doc, err := svc.FacturarCotizacion(empDemo, c.ID, actorA, origenTst, application.EntradaFacturacion{
		Credito: true, DiasCredito: 30,
	})
	if err != nil {
		t.Fatalf("facturar a crédito: %v", err)
	}
	saldo, hay := svc.SaldoPorCobrar(empDemo, doc.ID)
	if !hay || !casi(saldo, 116) {
		t.Errorf("la factura a crédito de la cotización debe quedar por cobrar por 116, se obtuvo %v (%v)", saldo, hay)
	}
}

func TestCancelarCotizacion(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	out, err := svc.CancelarCotizacion(empDemo, c.ID, actorA, origenTst, "el cliente desistió")
	if err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if out.Estado != cotizacion.EstadoCancelada {
		t.Errorf("tras cancelar el estado debe ser cancelada, quedó %q", out.Estado)
	}
	// Cancelar de nuevo (ya cancelada) no es una transición válida.
	if _, err := svc.CancelarCotizacion(empDemo, c.ID, actorA, origenTst, "otra vez"); !errors.Is(err, application.ErrTransicionCotizacion) {
		t.Errorf("cancelar una cotización ya cancelada debe dar ErrTransicionCotizacion, se obtuvo: %v", err)
	}
}

func TestCancelarCotizacion_FacturadaNoSePuede(t *testing.T) {
	svc, _ := nuevoServicio(t)
	c, err := svc.CrearCotizacion(empDemo, sede1, actorA, origenTst, application.EntradaCotizacion{
		Lineas: []application.LineaEntrada{{SKU: "REF-2L", Cantidad: 1, PrecioUnitario: 100}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := svc.ConfirmarCotizacion(empDemo, c.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if _, _, err := svc.FacturarCotizacion(empDemo, c.ID, actorA, origenTst, application.EntradaFacturacion{
		Pagos: []application.PagoEntrada{{Metodo: fiscal.PagoEfectivoBs, Monto: 116, Moneda: "VES"}},
	}); err != nil {
		t.Fatalf("facturar: %v", err)
	}
	// Una cotización ya facturada solo se corrige anulando su factura, no aquí.
	if _, err := svc.CancelarCotizacion(empDemo, c.ID, actorA, origenTst, "tarde"); !errors.Is(err, application.ErrTransicionCotizacion) {
		t.Fatalf("cancelar una cotización facturada debe dar ErrTransicionCotizacion, se obtuvo: %v", err)
	}
}
