package inmem

import (
	"fmt"
	"math"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/caja"
	"github.com/mornix/elerp/internal/domain/credencial"
	"github.com/mornix/elerp/internal/domain/cuenta"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/proveedor"
	"github.com/mornix/elerp/internal/domain/tasa"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// DATOS DE OPERACIÓN de las demos por rubro: caja, credenciales, cuentas de cobro,
// métodos de pago, proveedores y un mes de facturación ya emitida. Sin esto el rubro se
// ve vacío y el que prueba no puede juzgar nada.
//
// La contabilidad NO se siembra: los asientos los deriva `RecontabilizarPendientes` al
// arrancar, a partir de estos documentos. Es el principio de la app (la contabilidad es
// una proyección de la operación), y respetarlo evita sembrar un libro que no cuadre.

// PinDemo es el PIN y la contraseña de TODA credencial de DEMOSTRACIÓN: cajeros, usuarios
// por rol y la dueña. Un recorrido de demo no debe frenarse porque alguien no recuerda
// cuál de tres PINs iba en qué puesto.
//
// Es deliberadamente débil y SOLO se siembra en los tenants de demostración. No lo use
// ningún dato real: la demo pública ya deja entrar con un clic (DEV_LOGIN), así que esto
// no agrega exposición, pero en una instalación de cliente no debe existir.
const PinDemo = "1234"

// hashDemo cifra el PIN/contraseña de demostración. MinCost a propósito: el seed crea
// muchas credenciales y ninguna protege nada real.
func hashDemo() string {
	h, _ := bcrypt.GenerateFromPassword([]byte(PinDemo), bcrypt.MinCost)
	return string(h)
}

// round2Nicho redondea a céntimos (los importes fiscales no llevan más decimales).
func round2Nicho(v float64) float64 { return math.Round(v*100) / 100 }

// seedOperacionNicho siembra todo lo que hace que el rubro se vea "en uso".
func (s *Store) seedOperacionNicho(e especNicho) {
	fecha := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)

	// --- Cajas y credenciales de puesto ---
	// Sin caja habilitada no se puede facturar (regla de EmitirFactura), así que cada
	// rubro necesita la suya para que el prospecto pueda vender de verdad.
	s.Cajas.Create(caja.Caja{
		ID: "caja_" + e.empID + "_1", EmpresaID: e.empID, SedeID: e.sedeID,
		Nombre: "Caja 1", Codigo: "C-001", Estado: caja.EstadoHabilitada, Creada: fecha,
	})
	s.Cajas.Create(caja.Caja{
		ID: "caja_" + e.empID + "_2", EmpresaID: e.empID, SedeID: e.sedeID,
		Nombre: "Caja 2", Codigo: "C-002", Estado: caja.EstadoHabilitada, Creada: fecha,
	})
	// El segundo es SUPERVISOR: es quien autoriza quitar una línea del carrito o salir
	// del modo caja cuando la empresa lo exige. Un cajero raso no se autoriza solo.
	for _, cj := range []struct {
		codigo, nombre string
		supervisor     bool
	}{
		{"OP-001", e.cajero, false},
		{"OP-002", e.supervisor, true},
	} {
		s.Cajeros.Create(caja.Cajero{
			ID: "cjr_" + e.empID + "_" + cj.codigo, EmpresaID: e.empID, SedeID: e.sedeID,
			Codigo: cj.codigo, Nombre: cj.nombre, PinHash: hashDemo(),
			Supervisor: cj.supervisor, Activo: true,
		})
	}

	// --- Usuarios por rol, con contraseña ---
	// Para que el que prueba pueda entrar como vendedor, cajero o contadora y ver la app
	// desde cada rol (la interfaz cambia por completo). Todas con la misma contraseña.
	slug := e.slug
	for _, u := range []struct{ sufijo, nombre, rol, sede string }{
		{"vend", e.vendedor, usuario.RolVendedor, e.sedeID},
		{"cajero", e.cajero, usuario.RolCajero, e.sedeID},
		{"conta", e.contadora, usuario.RolContadora, ""},
	} {
		id := "usr_" + slug + "_" + u.sufijo
		email := u.sufijo + "@" + slug + ".test"
		s.Usuarios.Create(usuario.Usuario{ID: id, Nombre: u.nombre, Email: email})
		s.Membresias.Create(usuario.Membresia{
			UsuarioID: id, Email: email, Nombre: u.nombre,
			EmpresaID: e.empID, Rol: u.rol, SedeID: u.sede, Estado: usuario.EstadoActiva,
		})
		s.Credenciales.Create(credencial.Credencial{
			Email: email, Hash: hashDemo(), UsuarioID: id, Nombre: u.nombre,
		})
	}
	// El mesonero solo tiene sentido con el módulo Restaurante activo.
	if len(e.mesas) > 0 {
		id, email := "usr_"+slug+"_mesonero", "mesonero@"+slug+".test"
		s.Usuarios.Create(usuario.Usuario{ID: id, Nombre: e.mesonero, Email: email})
		s.Membresias.Create(usuario.Membresia{
			UsuarioID: id, Email: email, Nombre: e.mesonero,
			EmpresaID: e.empID, Rol: usuario.RolMesonero, SedeID: e.sedeID, Estado: usuario.EstadoActiva,
		})
		s.Credenciales.Create(credencial.Credencial{
			Email: email, Hash: hashDemo(), UsuarioID: id, Nombre: e.mesonero,
		})
	}

	// --- Cuentas de cobro y métodos de pago ---
	// Todos los tipos y ambas monedas: son los destinos del cobro mixto, y el IGTF
	// depende de que existan cuentas en divisas.
	cuentaPorTipo := map[string]string{}
	for _, cc := range []fiscal.CuentaCobro{
		{Tipo: "pago_movil", Moneda: "VES", Titular: e.nombre, Datos: "0102-04-" + e.telefonoBanco},
		{Tipo: "banco", Moneda: "VES", Titular: e.nombre, Datos: "Banesco 0134-0" + e.telefonoBanco},
		{Tipo: "punto_venta", Moneda: "VES", Titular: "Punto Sede Principal", Datos: "Terminal " + e.telefonoBanco},
		{Tipo: "efectivo", Moneda: "VES", Titular: "Caja chica Bs", Datos: "Efectivo en bolívares"},
		{Tipo: "zelle", Moneda: "USD", Titular: e.org, Datos: "pagos@" + slug + ".com"},
		{Tipo: "efectivo", Moneda: "USD", Titular: "Caja chica US$", Datos: "Efectivo en divisas"},
	} {
		cc.EmpresaID = e.empID
		out := s.CuentasCobro.Create(cc)
		if _, visto := cuentaPorTipo[out.Tipo]; !visto {
			cuentaPorTipo[out.Tipo] = out.ID
		}
	}
	for _, mp := range []fiscal.MetodoPago{
		{Nombre: "Efectivo Bs", Tipo: fiscal.PagoEfectivoBs, Moneda: "VES", EnCaja: true, EnVentas: true, Orden: 1},
		{Nombre: "Efectivo USD", Tipo: fiscal.PagoEfectivoUSD, Moneda: "USD", EnCaja: true, EnVentas: true, Orden: 2},
		{Nombre: "Pago Móvil", Tipo: fiscal.PagoPagoMovil, Moneda: "VES", CuentaCobroID: cuentaPorTipo["pago_movil"], EnCaja: true, EnVentas: true, Orden: 3},
		{Nombre: "Zelle", Tipo: fiscal.PagoZelle, Moneda: "USD", CuentaCobroID: cuentaPorTipo["zelle"], EnCaja: false, EnVentas: true, Orden: 4},
		{Nombre: "Tarjeta", Tipo: fiscal.PagoTarjeta, Moneda: "VES", EnCaja: true, EnVentas: false, Orden: 5},
		{Nombre: "Transferencia", Tipo: fiscal.PagoTransfer, Moneda: "VES", EnCaja: false, EnVentas: true, Orden: 6},
	} {
		mp.EmpresaID = e.empID
		mp.Activo = true
		s.MetodosPago.Create(mp)
	}

	// --- Proveedores (para que Compras no salga vacío) ---
	for _, p := range e.proveedores {
		s.Proveedores.Create(proveedor.Proveedor{
			EmpresaID: e.empID, Nombre: p.nombre, Documento: p.doc,
			Telefono: p.telefono, Activo: true, Creado: fecha,
		})
	}

	// --- Un mes de facturación ---
	s.seedFacturacionNicho(e)

	// --- Restaurante: mesas ya ocupadas ---
	if len(e.mesas) > 0 {
		s.seedCuentasAbiertas(e)
	}
}

// emisionNicho son los parámetros de un documento sembrado.
type emisionNicho struct {
	diasAtras int
	tipo      string
	serie     string
	cliente   string
	doc       string
	sku       string
	cant      float64
	// contingencia marca la factura emitida SIN conexión (serie reservada).
	contingencia bool
	// divisas cobra la mitad en dólares: genera IGTF sobre esa porción.
	divisas bool
	ref     string
	motivo  string
	// plazoDias > 0 emite A CRÉDITO; abono es lo que entró en el mostrador.
	plazoDias int
	abono     float64
}

// emitirNicho arma un documento fiscal COHERENTE y su pata de inventario: mismo criterio
// que la emisión viva (numerador propio, IVA que respeta la exención del producto, IGTF
// solo sobre la porción en divisas) y misma numeración, así el contador queda adelantado
// y la primera factura real del prospecto no colisiona con los folios sembrados.
func (s *Store) emitirNicho(e especNicho, em emisionNicho) fiscal.Documento {
	prod, ok := s.Productos.BySKU(e.empID, em.sku)
	if !ok {
		return fiscal.Documento{}
	}
	num := s.Numerador.Siguiente(e.empID, e.sedeID, em.serie)
	f := hoyDemo.AddDate(0, 0, -em.diasAtras)

	// El precio sale del catálogo: documentos y catálogo nunca cuentan precios
	// distintos del mismo producto.
	precio := prod.Precio
	base := round2Nicho(em.cant * precio)
	gravada, exenta := base, 0.0
	if prod.ExentoIVA {
		gravada, exenta = 0, base
	}
	iva := round2Nicho(gravada * fiscal.AlicuotaIVA)
	total := round2Nicho(base + iva)
	igtf := 0.0

	pagos := []fiscal.Pago{{Metodo: fiscal.PagoEfectivoBs, Monto: total, Moneda: "VES"}}
	switch {
	case em.plazoDias > 0:
		pagos = []fiscal.Pago{}
		if em.abono > 0 {
			pagos = []fiscal.Pago{{Metodo: fiscal.PagoPagoMovil, Monto: em.abono, Moneda: "VES"}}
		}
	case em.divisas:
		mitad := round2Nicho(total / 2)
		igtf = round2Nicho(mitad * fiscal.AlicuotaIGTF)
		total = round2Nicho(total + igtf)
		pagos = []fiscal.Pago{
			{Metodo: fiscal.PagoEfectivoBs, Monto: round2Nicho(mitad + igtf), Moneda: "VES"},
			{Metodo: fiscal.PagoEfectivoUSD, Monto: round2Nicho(mitad / tasaDemo), Moneda: "USD", EnDivisa: true},
		}
	}

	cobrado := total
	vence := ""
	if em.plazoDias > 0 {
		cobrado = em.abono
		vence = f.AddDate(0, 0, em.plazoDias).Format("2006-01-02")
	}

	doc := s.Documentos.Append(fiscal.Documento{
		EmpresaID: e.empID, SedeID: e.sedeID, Tipo: em.tipo,
		Serie: em.serie, Numero: num, NumeroCompleto: fmt.Sprintf("%s-%08d", em.serie, num),
		NumeroControl: fmt.Sprintf("00-%08d", s.Numerador.Siguiente(e.empID, "", "CTRL")),
		Modalidad:     empresa.ModalidadFormaLibre, Contingencia: em.contingencia,
		ClienteNombre: em.cliente, ClienteDocumento: em.doc,
		Lineas: []fiscal.Linea{{
			ProductoID: prod.ID, SKU: prod.SKU, Nombre: prod.Nombre,
			Cantidad: em.cant, PrecioUnitario: precio, Total: base, Exento: prod.ExentoIVA,
			// Un PLATO viaja con el snapshot de su receta: es lo que se reingresa si
			// después se anula, sin depender de que la receta del catálogo siga igual.
			Insumos: insumosSnapshot(prod),
		}},
		Subtotal: base, BaseImponible: gravada, BaseExenta: exenta,
		IVA: iva, IGTF: igtf, Total: total,
		Cobrado: cobrado, Credito: em.plazoDias > 0, VenceEl: vence,
		Moneda: "VES", TasaCambio: tasaDemo, TasaFuente: tasa.FuenteSemilla, Pagos: pagos,
		RefDocumentoID: em.ref, Motivo: em.motivo,
		CajaID: "caja_" + e.empID + "_1", CajaCodigo: "C-001", CajeroNombre: e.cajero,
		Actor: application.DemoUserID, Fecha: f.Format(time.RFC3339Nano),
	})

	// Pata de inventario: la factura descuenta, la reversa repone. Un plato mueve sus
	// INSUMOS, no el plato (que no se stockea).
	salida := em.tipo == fiscal.TipoFactura
	for _, c := range consumosNicho(prod, em.cant) {
		p2, ok := s.Productos.BySKU(e.empID, c.sku)
		if !ok {
			continue
		}
		signo := 1.0
		tipoMov := inventario.MovEntrada
		if salida {
			signo, tipoMov = -1.0, inventario.MovSalida
		}
		s.Movimientos.Append(inventario.Movimiento{
			EmpresaID: e.empID, SedeID: e.sedeID, ProductoID: p2.ID, SKU: p2.SKU,
			Tipo: tipoMov, Cantidad: signo * c.cantidad, CostoUnitario: e.costoDe(c.sku),
			Motivo: "venta " + doc.NumeroCompleto, RefTipo: "documento", RefID: doc.ID,
			Actor: application.DemoUserID, Fecha: f.Format(time.RFC3339Nano),
		})
	}
	return doc
}

// consumoNicho es lo que una línea mueve del stock.
type consumoNicho struct {
	sku      string
	cantidad float64
}

// consumosNicho expande una línea a lo que realmente mueve el inventario: los insumos de
// la receta si es un plato, o el propio SKU si es un producto normal.
func consumosNicho(p inventario.Producto, cant float64) []consumoNicho {
	if !p.EsPlato || len(p.Receta) == 0 {
		return []consumoNicho{{sku: p.SKU, cantidad: cant}}
	}
	out := make([]consumoNicho, 0, len(p.Receta))
	for _, ins := range p.Receta {
		out = append(out, consumoNicho{sku: ins.SKU, cantidad: ins.Cantidad * cant})
	}
	return out
}

// insumosSnapshot congela la receta del plato en la línea del documento.
func insumosSnapshot(p inventario.Producto) []fiscal.InsumoLinea {
	if !p.EsPlato || len(p.Receta) == 0 {
		return nil
	}
	out := make([]fiscal.InsumoLinea, 0, len(p.Receta))
	for _, ins := range p.Receta {
		out = append(out, fiscal.InsumoLinea{SKU: ins.SKU, CantidadUnitaria: ins.Cantidad})
	}
	return out
}

// seedCuentasAbiertas deja el salón del restaurante EN SERVICIO: mesas ocupadas, una
// comanda ya en cocina y otra recién tomada. Sin esto la comandera y la pantalla de
// Cocina se ven vacías y no se entiende el flujo.
func (s *Store) seedCuentasAbiertas(e especNicho) {
	mesas := s.Mesas.List(e.empID, e.sedeID)
	if len(mesas) < 3 {
		return
	}
	ahora := hoyDemo
	precioDe := func(sku string) (string, string, float64) {
		p, ok := s.Productos.BySKU(e.empID, sku)
		if !ok {
			return "", "", 0
		}
		return p.ID, p.Nombre, p.Precio
	}

	abrir := func(m mesa.Mesa, minutosAtras int, items []struct {
		sku      string
		cant     float64
		nota     string
		estado   string
		enCocina bool
	}) {
		abierta := ahora.Add(-time.Duration(minutosAtras) * time.Minute)
		c := cuenta.Cuenta{
			EmpresaID: e.empID, SedeID: e.sedeID, MesaID: m.ID, MesaNombre: m.Nombre,
			Estado: cuenta.EstadoAbierta, Abierta: abierta.Format(time.RFC3339Nano),
			MesoneroNombre: e.mesonero, Comensales: m.Capacidad, UltimaRonda: 1,
		}
		for i, it := range items {
			pid, nombre, precio := precioDe(it.sku)
			if pid == "" {
				continue
			}
			item := cuenta.Item{
				ID:       fmt.Sprintf("it_%s_%s_%d", e.empID, m.ID, i+1),
				SKU:      it.sku,
				Nombre:   nombre,
				Cantidad: it.cant, PrecioUnitario: precio,
				Nota: it.nota, Estado: it.estado, Ronda: 1,
				Agregada: abierta.Format(time.RFC3339Nano),
			}
			if it.enCocina {
				item.EnviadoEn = abierta.Add(2 * time.Minute).Format(time.RFC3339Nano)
			}
			c.Items = append(c.Items, item)
		}
		if len(c.Items) == 0 {
			return
		}
		s.Cuentas.Create(c)
		// La mesa refleja el estado de su cuenta.
		m.Estado = mesa.EstadoOcupada
		s.Mesas.Update(m)
	}

	type it = struct {
		sku      string
		cant     float64
		nota     string
		estado   string
		enCocina bool
	}
	// Mesa con la comanda YA en cocina (hace 12 min: la pantalla de Cocina la muestra
	// en ámbar por espera).
	abrir(mesas[0], 12, []it{
		{sku: "PLA-BOLONESA", cant: 2, nota: "una sin queso", estado: cuenta.ItemEnCocina, enCocina: true},
		{sku: "BEB-REFRESCO", cant: 2, estado: cuenta.ItemServido},
	})
	// Mesa con un plato ya listo y otro en preparación.
	abrir(mesas[3], 6, []it{
		{sku: "PLA-MILANESA", cant: 1, estado: cuenta.ItemListo, enCocina: true},
		{sku: "PLA-ENSALADA", cant: 1, nota: "sin aderezo", estado: cuenta.ItemEnCocina, enCocina: true},
		{sku: "BEB-CERVEZA", cant: 3, estado: cuenta.ItemServido},
	})
	// Mesa recién tomada: todavía sin enviar a cocina (el mesonero está tomando el resto).
	abrir(mesas[4], 2, []it{
		{sku: "PLA-POLLO-ARROZ", cant: 4, estado: cuenta.ItemPendiente},
		{sku: "BEB-AGUA", cant: 4, estado: cuenta.ItemPendiente},
	})
}

// seedFacturacionNicho emite la facturación sembrada del rubro. La lista viene en la
// especificación (seed_nichos_datos.go) y cubre a propósito toda la gama: contado,
// cobro en divisas con IGTF, contingencia y ventas a crédito (vencida y vigente), para
// que Facturación, Ventas, Tesorería y Contabilidad tengan de qué hablar.
//
// La ANULACIÓN y la NOTA DE CRÉDITO se emiten al final, referenciando facturas ya
// sembradas: el estado «anulado» lo DERIVA el backend de que exista la reversa, así que
// no se marca nada a mano.
func (s *Store) seedFacturacionNicho(e especNicho) {
	emitidas := make([]fiscal.Documento, 0, len(e.facturas))
	for _, em := range e.facturas {
		if em.tipo == "" {
			em.tipo = fiscal.TipoFactura
		}
		if em.serie == "" {
			em.serie = "FL"
		}
		d := s.emitirNicho(e, em)
		if d.ID != "" {
			emitidas = append(emitidas, d)
		}
	}
	if len(emitidas) < 4 {
		return
	}

	// ANULADA: reversa TOTAL de la primera factura de contado (repone su stock).
	orig := emitidas[0]
	s.emitirNicho(e, emisionNicho{
		diasAtras: em0Dias(orig), tipo: fiscal.TipoAnulacion, serie: orig.Serie,
		cliente: orig.ClienteNombre, doc: orig.ClienteDocumento,
		sku: orig.Lineas[0].SKU, cant: orig.Lineas[0].Cantidad,
		ref: orig.ID, motivo: "el cliente devolvió el pedido completo",
	})

	// NOTA DE CRÉDITO parcial sobre otra factura (devolución de una parte).
	parcial := emitidas[2]
	cant := parcial.Lineas[0].Cantidad / 2
	if cant <= 0 {
		return
	}
	s.emitirNicho(e, emisionNicho{
		diasAtras: em0Dias(parcial) - 1, tipo: fiscal.TipoNotaCredito, serie: parcial.Serie,
		cliente: parcial.ClienteNombre, doc: parcial.ClienteDocumento,
		sku: parcial.Lineas[0].SKU, cant: cant,
		ref: parcial.ID, motivo: "parte de la mercancía llegó dañada",
	})
}

// em0Dias devuelve cuántos días atrás se emitió un documento sembrado, para que su
// reversa quede DESPUÉS de él en la línea de tiempo (una anulación no puede anteceder a
// la factura que anula).
func em0Dias(d fiscal.Documento) int {
	t, err := time.Parse(time.RFC3339Nano, d.Fecha)
	if err != nil {
		return 1
	}
	dias := int(hoyDemo.Sub(t).Hours() / 24)
	if dias <= 1 {
		return 1
	}
	return dias - 1
}
