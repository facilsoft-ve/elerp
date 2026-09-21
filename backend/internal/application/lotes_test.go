package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* TRAZABILIDAD POR LOTE Y VENCIMIENTO.
 *
 * Sin lote, una alerta sanitaria se atiende sacando TODO el producto del anaquel,
 * y a «¿a quién le vendí el lote X?» no hay respuesta.
 *
 * La propiedad que lo sostiene todo y que estas pruebas cuidan por encima de
 * cualquier otra: EL SALDO POR LOTE NUNCA SE APARTA DEL SALDO DEL PRODUCTO. En
 * cuanto se separan, la trazabilidad miente sin que nada falle. */

const (
	skuLote  = "MED-LOTE"
	loteA    = "L-2024-A"
	loteB    = "L-2024-B"
	sinFecha = ""
)

func fechaEnDias(n int) string {
	return time.Now().UTC().AddDate(0, 0, n).Format("2006-01-02")
}

// productoConLote da de alta un producto con trazabilidad y devuelve su id.
func productoConLote(t *testing.T, svc *application.Service, sku string, conVencimiento bool) string {
	t.Helper()
	p, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: sku, Nombre: "Medicamento " + sku, Precio: 100,
		RequiereLote: true, ControlaVencimiento: conVencimiento,
	})
	if err != nil {
		t.Fatalf("crear producto con lote: %v", err)
	}
	return p.ID
}

// recibirLote recibe una cantidad de un SKU declarando lote y vencimiento.
func recibirLote(t *testing.T, svc *application.Service, sku string, cant float64, lote, venc string) error {
	t.Helper()
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: sku, Cantidad: cant, CostoUnitario: 10}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	_, err = svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: sku, Cantidad: cant, Lote: lote, Vencimiento: venc}})
	return err
}

// saldoTotalYPorLote devuelve el saldo del PRODUCTO y la suma de sus lotes. Que
// coincidan es la invariante de todo el módulo.
func saldoTotalYPorLote(t *testing.T, svc *application.Service, sku, productoID string) (float64, float64) {
	t.Helper()
	total := 0.0
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku {
			total = e.Cantidad
		}
	}
	porLote := 0.0
	for _, l := range svc.SaldosPorLote(empDemo, sede1, productoID) {
		porLote += l.Cantidad
	}
	return total, porLote
}

// TestLotes_LaRecepcionExigeLoteYVencimiento. Se pide al RECIBIR porque es el
// único momento en que alguien tiene la caja delante con la etiqueta.
func TestLotes_LaRecepcionExigeLoteYVencimiento(t *testing.T) {
	svc, _ := nuevoServicio(t)
	productoConLote(t, svc, skuLote, true)

	if err := recibirLote(t, svc, skuLote, 10, "", fechaEnDias(90)); !errors.Is(err, application.ErrLoteRequerido) {
		t.Errorf("sin lote debía rechazarse: %v", err)
	}
	if err := recibirLote(t, svc, skuLote, 10, loteA, ""); !errors.Is(err, application.ErrVencimientoRequerido) {
		t.Errorf("sin vencimiento debía rechazarse: %v", err)
	}
	if err := recibirLote(t, svc, skuLote, 10, loteA, "30-06-2026"); !errors.Is(err, application.ErrVencimientoInvalido) {
		t.Errorf("una fecha mal formada debía rechazarse: %v", err)
	}
	if err := recibirLote(t, svc, skuLote, 10, loteA, fechaEnDias(90)); err != nil {
		t.Errorf("con lote y fecha válidos debía entrar: %v", err)
	}
}

// TestLotes_SinTrazabilidadTodoSigueIgual: la regresión que más importa. El
// catálogo existente no lleva lotes y no debe notar nada.
func TestLotes_SinTrazabilidadTodoSigueIgual(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	if err := recibirLote(t, svc, sku, 10, "", ""); err != nil {
		t.Fatalf("un producto sin trazabilidad no debe exigir nada: %v", err)
	}
	// Y su existencia se proyecta como siempre, sin lotes de por medio.
	for _, e := range svc.Existencias(empDemo, sede1) {
		if e.SKU == sku && e.Cantidad <= 0 {
			t.Fatal("la recepción tenía que sumar existencia")
		}
	}
}

// TestLotes_FEFO_SaleAntesLoQueVenceAntes es la razón de ser del consumo
// automático: sin él, lo que caduca primero se queda en el anaquel.
func TestLotes_FEFO_SaleAntesLoQueVenceAntes(t *testing.T) {
	svc, _ := nuevoServicio(t)
	prodID := productoConLote(t, svc, skuLote, true)

	// Se recibe PRIMERO el que vence más TARDE, para que el orden de llegada no
	// pueda confundirse con el de vencimiento.
	if err := recibirLote(t, svc, skuLote, 10, loteB, fechaEnDias(180)); err != nil {
		t.Fatalf("recibir B: %v", err)
	}
	if err := recibirLote(t, svc, skuLote, 10, loteA, fechaEnDias(30)); err != nil {
		t.Fatalf("recibir A: %v", err)
	}

	tramos, err := svc.RepartirSalidaFEFO(empDemo, sede1, prodID, 15)
	if err != nil {
		t.Fatalf("repartir: %v", err)
	}
	if len(tramos) != 2 {
		t.Fatalf("15 unidades salen de dos lotes, dio %d tramos: %+v", len(tramos), tramos)
	}
	if tramos[0].Lote != loteA || !casi(tramos[0].Cantidad, 10) {
		t.Errorf("primero tiene que salir el que vence antes (A, 10): %+v", tramos[0])
	}
	if tramos[1].Lote != loteB || !casi(tramos[1].Cantidad, 5) {
		t.Errorf("el resto sale del siguiente (B, 5): %+v", tramos[1])
	}
}

// TestLotes_UnaVentaConsumeLosLotes: el saldo por lote se mantiene solo, sin que
// el mostrador tenga que elegir nada.
func TestLotes_UnaVentaConsumeLosLotes(t *testing.T) {
	svc, _ := nuevoServicio(t)
	prodID := productoConLote(t, svc, skuLote, true)
	if err := recibirLote(t, svc, skuLote, 10, loteA, fechaEnDias(30)); err != nil {
		t.Fatalf("recibir A: %v", err)
	}
	if err := recibirLote(t, svc, skuLote, 10, loteB, fechaEnDias(180)); err != nil {
		t.Fatalf("recibir B: %v", err)
	}

	// Una salida de 12 por el camino normal del inventario (una merma sirve: lo que
	// se prueba es el reparto, no el flujo fiscal).
	if _, err := svc.Ajustar(empDemo, sede1, "", skuLote, "salida de prueba", -12, actorA, origenTst); err != nil {
		t.Fatalf("ajuste: %v", err)
	}

	saldos := svc.SaldosPorLote(empDemo, sede1, prodID)
	porLote := map[string]float64{}
	for _, l := range saldos {
		porLote[l.Lote] = l.Cantidad
	}
	if _, quedaA := porLote[loteA]; quedaA {
		t.Errorf("el lote que vencía antes tenía que agotarse: %+v", saldos)
	}
	if !casi(porLote[loteB], 8) {
		t.Errorf("del lote B tenían que quedar 8, quedan %v", porLote[loteB])
	}

	// LA INVARIANTE.
	total, suma := saldoTotalYPorLote(t, svc, skuLote, prodID)
	if !casi(total, suma) {
		t.Fatalf("el saldo por lote se apartó del saldo del producto: %v vs %v", total, suma)
	}
}

// TestLotes_ElLoteViajaEnLaTransferencia: cruzar de sede no puede borrar el
// rastro — es justo cuando más falta hace saber qué lote está dónde.
func TestLotes_ElLoteViajaEnLaTransferencia(t *testing.T) {
	svc, _ := nuevoServicio(t)
	prodID := productoConLote(t, svc, skuLote, true)
	if err := recibirLote(t, svc, skuLote, 10, loteA, fechaEnDias(60)); err != nil {
		t.Fatalf("recibir: %v", err)
	}

	tr, err := svc.CrearTransferencia(empDemo, actorA, origenTst, inventario.Transferencia{
		OrigenSedeID: sede1, DestinoSedeID: sede2,
		Lineas: []inventario.LineaTransferencia{{SKU: skuLote, Cantidad: 4}},
	})
	if err != nil {
		t.Fatalf("crear transferencia: %v", err)
	}
	for _, estado := range []string{inventario.TransfDespachada, inventario.TransfEnTransito, inventario.TransfRecibida} {
		if _, err := svc.CambiarEstadoTransferencia(empDemo, tr.ID, estado, actorA, origenTst); err != nil {
			t.Fatalf("pasar a %s: %v", estado, err)
		}
	}

	destino := svc.SaldosPorLote(empDemo, sede2, prodID)
	if len(destino) != 1 || destino[0].Lote != loteA {
		t.Fatalf("el lote tenía que llegar al destino con su nombre: %+v", destino)
	}
	if !casi(destino[0].Cantidad, 4) {
		t.Errorf("tenían que llegar 4 unidades, llegaron %v", destino[0].Cantidad)
	}
	if destino[0].Vencimiento == "" {
		t.Error("el vencimiento tenía que viajar con el lote")
	}
}

// TestLotes_PorVencerYVencidos alimenta el aviso. Un lote vencido con existencia
// es mercancía que no se puede vender y que nadie va a notar mirando el total.
func TestLotes_PorVencerYVencidos(t *testing.T) {
	svc, _ := nuevoServicio(t)
	productoConLote(t, svc, skuLote, true)
	if err := recibirLote(t, svc, skuLote, 5, "L-VENCIDO", fechaEnDias(-10)); err != nil {
		t.Fatalf("recibir vencido: %v", err)
	}
	if err := recibirLote(t, svc, skuLote, 5, "L-PRONTO", fechaEnDias(15)); err != nil {
		t.Fatalf("recibir por vencer: %v", err)
	}
	if err := recibirLote(t, svc, skuLote, 5, "L-LEJOS", fechaEnDias(300)); err != nil {
		t.Fatalf("recibir lejano: %v", err)
	}

	avisos := svc.LotesPorVencer(empDemo, sede1, 30)
	if len(avisos) != 2 {
		t.Fatalf("a 30 días avisan el vencido y el próximo, no el lejano: %+v", avisos)
	}
	// Primero el vencido: es el que hay que sacar del anaquel hoy.
	if avisos[0].Lote != "L-VENCIDO" || !avisos[0].Vencido {
		t.Errorf("el vencido va primero y marcado: %+v", avisos[0])
	}
	if avisos[0].DiasParaVencer >= 0 {
		t.Errorf("un lote vencido tiene días NEGATIVOS, dio %d", avisos[0].DiasParaVencer)
	}
	if avisos[1].Lote != "L-PRONTO" || avisos[1].Vencido {
		t.Errorf("el segundo es el que está por vencer, sin marcar: %+v", avisos[1])
	}
}

// TestLotes_LaExistenciaAnteriorNoSePierde: a un producto con stock se le activa
// la trazabilidad. Esas unidades no tienen lote y no se pueden inventar, pero
// tampoco pueden desaparecer: quedan visibles como «sin lote».
func TestLotes_LaExistenciaAnteriorNoSePierde(t *testing.T) {
	svc, _ := nuevoServicio(t)
	sku := primerSKU(t, svc)
	if err := recibirLote(t, svc, sku, 10, "", ""); err != nil {
		t.Fatalf("recibir sin lote: %v", err)
	}
	prodID := ""
	for _, p := range svc.Productos(empDemo) {
		if p.SKU == sku {
			prodID = p.ID
		}
	}
	activo := true
	if _, err := svc.ActualizarProducto(empDemo, actorA, origenTst, sku, application.CambiosProducto{
		RequiereLote: &activo,
	}); err != nil {
		t.Fatalf("activar trazabilidad: %v", err)
	}

	total, suma := saldoTotalYPorLote(t, svc, sku, prodID)
	if !casi(total, suma) {
		t.Fatalf("al activar lotes la existencia anterior se perdió: total %v, por lote %v", total, suma)
	}
	if total <= 0 {
		t.Fatal("tenía que quedar existencia")
	}
}

/* --- Los huecos que se cerraron antes de desplegar ----------------------- */

// facturarACliente emite una venta de contado del SKU, para que el rastro tenga
// documentos y terceros de verdad detrás. Se usa el camino real de emisión: es
// justo lo que el rastro tiene que saber reconstruir.
func facturarACliente(t *testing.T, svc *application.Service, sku string, cant float64) {
	t.Helper()
	abrirTurno(t, svc, actorA)
	cl := svc.Clientes(empDemo)
	if len(cl) == 0 {
		t.Fatal("el seed debería traer clientes")
	}
	if _, err := svc.EmitirFactura(empDemo, sede1, "forma_libre", actorA, origenTst, application.EmitirEntrada{
		ClienteID: cl[0].ID,
		Lineas:    []application.LineaEntrada{{SKU: sku, Cantidad: cant, PrecioUnitario: 100}},
		Credito:   true, DiasCredito: 30,
	}); err != nil {
		t.Fatalf("emitir factura de %v %s: %v", cant, sku, err)
	}
}

// TestLotes_UnVencidoNoSeVende: FEFO lo pondría PRIMERO —es el que caduca antes—
// así que sin guarda el consumo automático despacharía justo lo que no se puede
// vender, en silencio y en cada venta.
func TestLotes_UnVencidoNoSeVende(t *testing.T) {
	svc, _ := nuevoServicio(t)
	prodID := productoConLote(t, svc, skuLote, true)
	if err := recibirLote(t, svc, skuLote, 5, "L-VENCIDO", fechaEnDias(-3)); err != nil {
		t.Fatalf("recibir vencido: %v", err)
	}
	if err := recibirLote(t, svc, skuLote, 10, "L-BUENO", fechaEnDias(120)); err != nil {
		t.Fatalf("recibir bueno: %v", err)
	}

	// Pese a vencer antes, el vencido se salta: salen las 4 del lote bueno.
	tramos, err := svc.RepartirSalidaFEFO(empDemo, sede1, prodID, 4)
	if err != nil {
		t.Fatalf("repartir: %v", err)
	}
	if len(tramos) != 1 || tramos[0].Lote != "L-BUENO" {
		t.Fatalf("el vencido no puede salir: %+v", tramos)
	}

	// Y si lo único que alcanza está vencido, se dice POR QUÉ: «no hay» mandaría a
	// comprar más cuando lo que hay que hacer es dar de baja el lote.
	if _, err := svc.RepartirSalidaFEFO(empDemo, sede1, prodID, 12); !errors.Is(err, application.ErrSoloQuedaVencido) {
		t.Fatalf("debía distinguir que lo que queda está vencido: %v", err)
	}
}

// TestLotes_VariosLotesEnUnaSolaRecepcion: el proveedor completa un pedido con lo
// que tiene, y eso llega en dos lotes el mismo día. Antes había que partir la
// recepción en dos.
func TestLotes_VariosLotesEnUnaSolaRecepcion(t *testing.T) {
	svc, _ := nuevoServicio(t)
	prodID := productoConLote(t, svc, skuLote, true)

	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: skuLote, Cantidad: 30, CostoUnitario: 10}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst, []application.LineaRecepcion{
		{SKU: skuLote, Cantidad: 20, Lote: loteA, Vencimiento: fechaEnDias(60)},
		{SKU: skuLote, Cantidad: 10, Lote: loteB, Vencimiento: fechaEnDias(200)},
	}); err != nil {
		t.Fatalf("recibir en dos lotes: %v", err)
	}

	saldos := svc.SaldosPorLote(empDemo, sede1, prodID)
	if len(saldos) != 2 {
		t.Fatalf("tenían que quedar dos lotes: %+v", saldos)
	}
	porLote := map[string]float64{}
	for _, l := range saldos {
		porLote[l.Lote] = l.Cantidad
	}
	if !casi(porLote[loteA], 20) || !casi(porLote[loteB], 10) {
		t.Errorf("cada lote con lo suyo: %+v", porLote)
	}
	// Y lo recibido de la LÍNEA suma las dos entregas, o la orden quedaría
	// eternamente pendiente de 10 unidades que sí llegaron.
	oc2, _ := svc.OrdenCompra(empDemo, oc.ID)
	if !casi(oc2.Lineas[0].CantidadRecibida, 30) {
		t.Errorf("la línea tenía que quedar recibida por 30, quedó %v", oc2.Lineas[0].CantidadRecibida)
	}
	total, suma := saldoTotalYPorLote(t, svc, skuLote, prodID)
	if !casi(total, suma) {
		t.Fatalf("el saldo por lote se apartó del producto: %v vs %v", total, suma)
	}
}

// TestLotes_ElRastroDiceAQuienSeLeVendio es la consulta que justifica todo el
// módulo: guardar el lote y no poder preguntarlo es tener el dato y no la función.
func TestLotes_ElRastroDiceAQuienSeLeVendio(t *testing.T) {
	svc, _ := nuevoServicio(t)
	productoConLote(t, svc, skuLote, true)
	if err := recibirLote(t, svc, skuLote, 20, loteA, fechaEnDias(120)); err != nil {
		t.Fatalf("recibir: %v", err)
	}
	// Dos ventas a clientes distintos del MISMO lote.
	facturarACliente(t, svc, skuLote, 3)
	facturarACliente(t, svc, skuLote, 2)

	r := svc.RastroDeLote(empDemo, skuLote, loteA, "")
	if !casi(r.Recibido, 20) {
		t.Errorf("recibido = %v, se esperaban 20", r.Recibido)
	}
	if !casi(r.Salido, 5) {
		t.Errorf("salido = %v, se esperaban 5", r.Salido)
	}
	if !casi(r.EnStock, 15) {
		t.Errorf("en stock = %v, se esperaban 15", r.EnStock)
	}
	if len(r.Pasos) < 3 {
		t.Fatalf("el rastro tenía que traer la entrada y las dos salidas: %+v", r.Pasos)
	}
	// El primer paso es la compra, con su orden y su proveedor: de ahí sale a quién
	// RECLAMARLE si el lote viene malo.
	if r.Pasos[0].Documento == "" || r.Pasos[0].Tercero == "" {
		t.Errorf("la entrada tenía que nombrar la orden y el proveedor: %+v", r.Pasos[0])
	}
	// Y la respuesta corta: a quién avisar.
	if len(r.Clientes) == 0 {
		t.Fatal("el rastro tiene que decir a quién se le vendió: es para lo que existe")
	}
	// Un lote agotado o inexistente no revienta: devuelve un rastro vacío.
	vacio := svc.RastroDeLote(empDemo, skuLote, "L-QUE-NO-EXISTE", "")
	if len(vacio.Pasos) != 0 || vacio.EnStock != 0 {
		t.Errorf("un lote inexistente devuelve rastro vacío: %+v", vacio)
	}
}
