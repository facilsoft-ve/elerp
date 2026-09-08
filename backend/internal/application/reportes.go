package application

import (
	"sort"
	"time"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// Reportes y BI — agregaciones DERIVADAS del ledger, de solo lectura.
//
// Igual que los libros fiscales (libros.go) y el Cierre Z (cierrez.go), este
// módulo no persiste nada ni requiere auditoría: son consultas, no hechos
// económicos. Cada número sale de plegar los documentos fiscales, los
// movimientos de inventario, las órdenes de compra y las cuentas por cobrar que
// ya viven en el Service — nunca de un contador paralelo que pueda
// desincronizarse. La interfaz del módulo pinta lo que aquí se calcula; no
// recalcula a su manera.
//
// Convención de rango: `desde`/`hasta` son fechas YYYY-MM-DD inclusivas. Se
// comparan contra el prefijo de 10 caracteres de la Fecha RFC3339 (que ordena
// cronológicamente como string). Si faltan o son inválidas se usa el mes en
// curso (ver normalizarRango). Los reportes sin rango (inventario, cobranza)
// son fotos del estado actual.

/* --- Reporte de Ventas ----------------------------------------------------- */

// SerieDia es un punto de la tendencia diaria: fecha (YYYY-MM-DD) → monto neto.
type SerieDia struct {
	Fecha string  `json:"fecha"`
	Monto float64 `json:"monto"`
}

// VentaPorProducto agrega ventas netas por SKU (de las líneas de los documentos).
type VentaPorProducto struct {
	SKU      string  `json:"sku"`
	Nombre   string  `json:"nombre"`
	Cantidad float64 `json:"cantidad"`
	Monto    float64 `json:"monto"`
}

// VentaPorCliente agrega ventas netas por cliente.
type VentaPorCliente struct {
	ClienteNombre string  `json:"clienteNombre"`
	Monto         float64 `json:"monto"`
	Documentos    int     `json:"documentos"`
}

// VentaPorCanal separa el mostrador (Punto de venta) del canal Ventas.
type VentaPorCanal struct {
	Canal      string  `json:"canal"`
	Monto      float64 `json:"monto"`
	Documentos int     `json:"documentos"`
}

// VentaPorSede agrega ventas netas por sede (el documento guarda SedeID).
type VentaPorSede struct {
	SedeID     string  `json:"sedeId"`
	Monto      float64 `json:"monto"`
	Documentos int     `json:"documentos"`
}

// ReporteVentasResult es la vista de ventas de un rango: totales, tendencia y
// desgloses. Las notas de crédito y anulaciones RESTAN (montos netos).
type ReporteVentasResult struct {
	Desde            string             `json:"desde"`
	Hasta            string             `json:"hasta"`
	VentasNetas      float64            `json:"ventasNetas"`
	IVADebito        float64            `json:"ivaDebito"`
	IGTF             float64            `json:"igtf"`
	CantidadFacturas int                `json:"cantidadFacturas"`
	TicketPromedio   float64            `json:"ticketPromedio"`
	PorDia           []SerieDia         `json:"porDia"`
	PorProducto      []VentaPorProducto `json:"porProducto"`
	PorCliente       []VentaPorCliente  `json:"porCliente"`
	PorCanal         []VentaPorCanal    `json:"porCanal"`
	PorSede          []VentaPorSede     `json:"porSede"`
}

// ReporteVentas pliega los documentos fiscales del rango en la vista de ventas.
//
// El monto es NETO: las facturas suman y las notas de crédito / anulaciones
// restan. Los documentos ya guardan Total/IVA/IGTF con signo (negativo en las
// reversas), así que a nivel de cabecera se suman tal cual. Las LÍNEAS de una
// reversa, en cambio, se copian del original en POSITIVO (ver EmitirNotaCredito
// / AnularDocumento), por eso los desgloses por producto aplican el signo del
// tipo de documento a la línea.
func (s *Service) ReporteVentas(empresaID, desde, hasta string) ReporteVentasResult {
	d, h := normalizarRango(desde, hasta)
	res := ReporteVentasResult{
		Desde: d, Hasta: h,
		PorDia: []SerieDia{}, PorProducto: []VentaPorProducto{},
		PorCliente: []VentaPorCliente{}, PorCanal: []VentaPorCanal{}, PorSede: []VentaPorSede{},
	}
	if s.documentos == nil {
		return res
	}

	porDia := map[string]float64{}
	porProd := map[string]*VentaPorProducto{}
	porCli := map[string]*VentaPorCliente{}
	porCanal := map[string]*VentaPorCanal{}
	porSede := map[string]*VentaPorSede{}

	for _, doc := range s.documentos.List(empresaID) {
		switch doc.Tipo {
		case fiscal.TipoFactura, fiscal.TipoNotaCredito, fiscal.TipoNotaDebito, fiscal.TipoAnulacion:
		default:
			continue
		}
		if !enRango(doc.Fecha, d, h) {
			continue
		}
		// La factura y la nota de débito suman en POSITIVO (la ND es un cargo
		// adicional); la nota de crédito y la anulación restan (guardan montos
		// negativos, pero el signo de las líneas se aplica aparte).
		sign := 1.0
		if doc.Tipo == fiscal.TipoNotaCredito || doc.Tipo == fiscal.TipoAnulacion {
			sign = -1
		}

		res.VentasNetas += doc.Total // ya viene con signo en las reversas
		res.IVADebito += doc.IVA
		res.IGTF += doc.IGTF
		if doc.Tipo == fiscal.TipoFactura {
			res.CantidadFacturas++
		}

		if len(doc.Fecha) >= 10 {
			porDia[doc.Fecha[:10]] += doc.Total
		}

		// Por producto: la línea es positiva incluso en reversas, se le aplica el
		// signo del documento para que una NC descuente unidades y monto.
		for _, l := range doc.Lineas {
			p := porProd[l.SKU]
			if p == nil {
				p = &VentaPorProducto{SKU: l.SKU, Nombre: l.Nombre}
				porProd[l.SKU] = p
			}
			p.Cantidad += sign * l.Cantidad
			p.Monto += sign * l.Total
		}

		nombre := doc.ClienteNombre
		if nombre == "" {
			nombre = "(sin cliente)"
		}
		c := porCli[nombre]
		if c == nil {
			c = &VentaPorCliente{ClienteNombre: nombre}
			porCli[nombre] = c
		}
		c.Monto += doc.Total
		c.Documentos++

		canal := canalDoc(doc)
		cn := porCanal[canal]
		if cn == nil {
			cn = &VentaPorCanal{Canal: canal}
			porCanal[canal] = cn
		}
		cn.Monto += doc.Total
		cn.Documentos++

		sd := porSede[doc.SedeID]
		if sd == nil {
			sd = &VentaPorSede{SedeID: doc.SedeID}
			porSede[doc.SedeID] = sd
		}
		sd.Monto += doc.Total
		sd.Documentos++
	}

	res.VentasNetas = round2(res.VentasNetas)
	res.IVADebito = round2(res.IVADebito)
	res.IGTF = round2(res.IGTF)
	if res.CantidadFacturas > 0 {
		res.TicketPromedio = round2(res.VentasNetas / float64(res.CantidadFacturas))
	}

	// Serie diaria en orden cronológico.
	for f, m := range porDia {
		res.PorDia = append(res.PorDia, SerieDia{Fecha: f, Monto: round2(m)})
	}
	sort.Slice(res.PorDia, func(i, j int) bool { return res.PorDia[i].Fecha < res.PorDia[j].Fecha })

	for _, p := range porProd {
		p.Cantidad = round2(p.Cantidad)
		p.Monto = round2(p.Monto)
		res.PorProducto = append(res.PorProducto, *p)
	}
	sort.Slice(res.PorProducto, func(i, j int) bool { return res.PorProducto[i].Monto > res.PorProducto[j].Monto })

	for _, c := range porCli {
		c.Monto = round2(c.Monto)
		res.PorCliente = append(res.PorCliente, *c)
	}
	sort.Slice(res.PorCliente, func(i, j int) bool { return res.PorCliente[i].Monto > res.PorCliente[j].Monto })

	for _, cn := range porCanal {
		cn.Monto = round2(cn.Monto)
		res.PorCanal = append(res.PorCanal, *cn)
	}
	sort.Slice(res.PorCanal, func(i, j int) bool { return res.PorCanal[i].Monto > res.PorCanal[j].Monto })

	for _, sd := range porSede {
		sd.Monto = round2(sd.Monto)
		res.PorSede = append(res.PorSede, *sd)
	}
	sort.Slice(res.PorSede, func(i, j int) bool { return res.PorSede[i].Monto > res.PorSede[j].Monto })

	return res
}

/* --- Reporte de Inventario ------------------------------------------------- */

// InventarioValorItem es la valorización de un producto (existencia × costo).
type InventarioValorItem struct {
	SKU           string  `json:"sku"`
	Nombre        string  `json:"nombre"`
	Cantidad      float64 `json:"cantidad"`
	CostoPromedio float64 `json:"costoPromedio"`
	Valor         float64 `json:"valor"`
}

// RotacionItem son las unidades que salieron de un SKU en la ventana reciente.
type RotacionItem struct {
	SKU              string  `json:"sku"`
	Nombre           string  `json:"nombre"`
	UnidadesVendidas float64 `json:"unidadesVendidas"`
}

// ReporteInventarioResult resume el estado del inventario de la empresa
// (consolidando todas las sedes) y la rotación de los últimos 30 días.
type ReporteInventarioResult struct {
	ValorizacionTotal float64               `json:"valorizacionTotal"`
	NumeroProductos   int                   `json:"numeroProductos"`
	BajoMinimo        int                   `json:"bajoMinimo"`
	Agotados          int                   `json:"agotados"`
	TopPorValor       []InventarioValorItem `json:"topPorValor"`
	Rotacion          []RotacionItem        `json:"rotacion"`
}

// DiasRotacion es la ventana (en días) sobre la que se mide la rotación.
const DiasRotacion = 30

// ReporteInventario valoriza el inventario y calcula la rotación reciente.
//
// La existencia se pliega del ledger SIN filtrar por sede: es la posición de
// toda la empresa (sigue aislada por empresaID, la frontera de tenant). "Bajo
// mínimo" son los productos con stock positivo pero ≤ UmbralStockBajo;
// "agotados" los que están en cero o menos (cubetas disjuntas). La rotación
// suma las SALIDAS del ledger de los últimos DiasRotacion días.
func (s *Service) ReporteInventario(empresaID string) ReporteInventarioResult {
	res := ReporteInventarioResult{TopPorValor: []InventarioValorItem{}, Rotacion: []RotacionItem{}}
	if s.productos == nil || s.movimientos == nil {
		return res
	}
	corte := time.Now().UTC().AddDate(0, 0, -DiasRotacion).Format(time.RFC3339)

	for _, p := range s.productos.List(empresaID) {
		res.NumeroProductos++
		movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{ProductoID: p.ID})
		cant, avg := fold(movs)
		valor := round2(cant * avg)
		res.ValorizacionTotal += valor
		switch {
		case cant <= 0:
			res.Agotados++
		case cant <= UmbralStockBajo:
			res.BajoMinimo++
		}
		res.TopPorValor = append(res.TopPorValor, InventarioValorItem{
			SKU: p.SKU, Nombre: p.Nombre, Cantidad: round2(cant),
			CostoPromedio: round2(avg), Valor: valor,
		})

		// Rotación: unidades que salieron (MovSalida) en la ventana reciente. La
		// salida guarda cantidad negativa; se acumula su valor absoluto.
		var salidas float64
		for _, m := range movs {
			if m.Tipo == inventario.MovSalida && m.Fecha >= corte {
				salidas += -m.Cantidad
			}
		}
		if salidas > 0 {
			res.Rotacion = append(res.Rotacion, RotacionItem{
				SKU: p.SKU, Nombre: p.Nombre, UnidadesVendidas: round2(salidas),
			})
		}
	}
	res.ValorizacionTotal = round2(res.ValorizacionTotal)

	sort.Slice(res.TopPorValor, func(i, j int) bool { return res.TopPorValor[i].Valor > res.TopPorValor[j].Valor })
	sort.Slice(res.Rotacion, func(i, j int) bool { return res.Rotacion[i].UnidadesVendidas > res.Rotacion[j].UnidadesVendidas })
	return res
}

/* --- Reporte de Compras ---------------------------------------------------- */

// CompraPorProveedor agrega las OCs del rango por proveedor.
type CompraPorProveedor struct {
	ProveedorID string  `json:"proveedorId"`
	Nombre      string  `json:"nombre"`
	RIF         string  `json:"rif"`
	Total       float64 `json:"total"`
	Ordenes     int     `json:"ordenes"`
}

// OrdenesPorEstado cuenta las OCs del rango en cada estado.
type OrdenesPorEstado struct {
	Estado   string `json:"estado"`
	Cantidad int    `json:"cantidad"`
}

// ReporteComprasResult resume las órdenes de compra del rango y el pipeline
// pendiente de recibir (que es una foto del estado actual, no del rango).
type ReporteComprasResult struct {
	Desde                    string               `json:"desde"`
	Hasta                    string               `json:"hasta"`
	PorProveedor             []CompraPorProveedor `json:"porProveedor"`
	PorEstado                []OrdenesPorEstado   `json:"porEstado"`
	OrdenesAbiertas          int                  `json:"ordenesAbiertas"`
	PendienteRecibirUnidades float64              `json:"pendienteRecibirUnidades"`
	PendienteRecibirMonto    float64              `json:"pendienteRecibirMonto"`
}

// ReporteCompras agrega las OCs del rango por proveedor y estado, y calcula lo
// pendiente de recibir.
//
// Por proveedor y por estado se filtran por rango (Creada). El pendiente de
// recibir y las órdenes abiertas son una FOTO del pipeline actual: se toman de
// las órdenes confirmada / recibida_parcial sin filtrar por fecha, porque una
// orden abierta hace tres meses sigue pendiente hoy.
func (s *Service) ReporteCompras(empresaID, desde, hasta string) ReporteComprasResult {
	d, h := normalizarRango(desde, hasta)
	res := ReporteComprasResult{
		Desde: d, Hasta: h,
		PorProveedor: []CompraPorProveedor{}, PorEstado: []OrdenesPorEstado{},
	}
	if s.ordenesCompra == nil {
		return res
	}

	porProv := map[string]*CompraPorProveedor{}
	porEstado := map[string]int{}
	rifCache := map[string]string{}

	for _, o := range s.ordenesCompra.List(empresaID) {
		// Pipeline abierto (independiente del rango).
		if o.Estado == compra.OCConfirmada || o.Estado == compra.OCRecibidaParcial {
			res.OrdenesAbiertas++
			for _, l := range o.Lineas {
				pend := l.Cantidad - l.CantidadRecibida
				if pend > 0 {
					res.PendienteRecibirUnidades += pend
					res.PendienteRecibirMonto += pend * l.CostoUnitario
				}
			}
		}

		if len(o.Creada) < 10 || !enRango(o.Creada, d, h) {
			continue
		}
		porEstado[o.Estado]++

		p := porProv[o.ProveedorID]
		if p == nil {
			rif, ok := rifCache[o.ProveedorID]
			if !ok {
				if s.provs != nil {
					if pr, found := s.provs.ByID(empresaID, o.ProveedorID); found {
						rif = pr.Documento
					}
				}
				rifCache[o.ProveedorID] = rif
			}
			p = &CompraPorProveedor{ProveedorID: o.ProveedorID, Nombre: o.ProveedorNombre, RIF: rif}
			porProv[o.ProveedorID] = p
		}
		p.Total += o.Total
		p.Ordenes++
	}

	res.PendienteRecibirUnidades = round2(res.PendienteRecibirUnidades)
	res.PendienteRecibirMonto = round2(res.PendienteRecibirMonto)

	for _, p := range porProv {
		p.Total = round2(p.Total)
		res.PorProveedor = append(res.PorProveedor, *p)
	}
	sort.Slice(res.PorProveedor, func(i, j int) bool { return res.PorProveedor[i].Total > res.PorProveedor[j].Total })

	for e, n := range porEstado {
		res.PorEstado = append(res.PorEstado, OrdenesPorEstado{Estado: e, Cantidad: n})
	}
	sort.Slice(res.PorEstado, func(i, j int) bool { return res.PorEstado[i].Estado < res.PorEstado[j].Estado })
	return res
}

/* --- Reporte de Cobranza --------------------------------------------------- */

// AgingTramo es un tramo de antigüedad de la cartera por cobrar.
type AgingTramo struct {
	Tramo      string  `json:"tramo"` // 0-30 | 31-60 | 61-90 | 90+
	Monto      float64 `json:"monto"`
	Documentos int     `json:"documentos"`
}

// ReporteCobranzaResult resume las cuentas por cobrar: total, vencido, aging e
// IGTF del período.
type ReporteCobranzaResult struct {
	TotalPorCobrar float64      `json:"totalPorCobrar"`
	TotalVencido   float64      `json:"totalVencido"`
	Aging          []AgingTramo `json:"aging"`
	IGTF           float64      `json:"igtf"`
}

// ReporteCobranza construye el aging de la cartera reutilizando CuentasPorCobrar.
//
// La antigüedad se mide en días desde la FECHA DEL DOCUMENTO (no la de
// vencimiento) hasta hoy, en tramos 0-30, 31-60, 61-90 y 90+. El total vencido
// sale del mismo resumen (deriva del vencimiento acordado). El IGTF del período
// se toma de ReporteIGTF.
func (s *Service) ReporteCobranza(empresaID string) ReporteCobranzaResult {
	resumen := s.CuentasPorCobrar(empresaID)
	tramos := []*AgingTramo{
		{Tramo: "0-30"}, {Tramo: "31-60"}, {Tramo: "61-90"}, {Tramo: "90+"},
	}
	hoy := hoyVE()
	for _, c := range resumen.Cuentas {
		fecha := c.Fecha
		if len(fecha) >= 10 {
			fecha = fecha[:10]
		}
		dias := diasEntre(fecha, hoy)
		var idx int
		switch {
		case dias <= 30:
			idx = 0
		case dias <= 60:
			idx = 1
		case dias <= 90:
			idx = 2
		default:
			idx = 3
		}
		tramos[idx].Monto += c.Saldo
		tramos[idx].Documentos++
	}
	aging := make([]AgingTramo, 0, len(tramos))
	for _, t := range tramos {
		t.Monto = round2(t.Monto)
		aging = append(aging, *t)
	}
	_, igtf := s.ReporteIGTF(empresaID)
	return ReporteCobranzaResult{
		TotalPorCobrar: resumen.Total,
		TotalVencido:   resumen.TotalVencido,
		Aging:          aging,
		IGTF:           igtf,
	}
}

/* --- Panel ejecutivo ------------------------------------------------------- */

// PanelResult consolida lo esencial para el dashboard del módulo en un solo
// objeto: ventas del mes en curso, estado del inventario, cartera por cobrar y
// pipeline de compras.
type PanelResult struct {
	Ventas                ReporteVentasResult     `json:"ventas"`
	Inventario            ReporteInventarioResult `json:"inventario"`
	TotalPorCobrar        float64                 `json:"totalPorCobrar"`
	TotalVencido          float64                 `json:"totalVencido"`
	OrdenesAbiertas       int                     `json:"ordenesAbiertas"`
	PendienteRecibirMonto float64                 `json:"pendienteRecibirMonto"`
}

// PanelEjecutivo arma la vista general del módulo reutilizando los demás
// reportes: ventas del mes en curso (con su serie diaria, canales, sedes y top
// de productos), valorización y faltantes de inventario, total por cobrar y
// órdenes de compra abiertas.
func (s *Service) PanelEjecutivo(empresaID string) PanelResult {
	desde, hasta := rangoMesActual()
	ventas := s.ReporteVentas(empresaID, desde, hasta)
	inv := s.ReporteInventario(empresaID)
	cob := s.ReporteCobranza(empresaID)
	compras := s.ReporteCompras(empresaID, desde, hasta)
	return PanelResult{
		Ventas:                ventas,
		Inventario:            inv,
		TotalPorCobrar:        cob.TotalPorCobrar,
		TotalVencido:          cob.TotalVencido,
		OrdenesAbiertas:       compras.OrdenesAbiertas,
		PendienteRecibirMonto: compras.PendienteRecibirMonto,
	}
}

/* --- Helpers --------------------------------------------------------------- */

// canalDoc deriva el canal del documento: si trae caja (CajaID/CajaCodigo) fue
// emitido en el mostrador (Punto de venta); si no, por el canal Ventas.
func canalDoc(d fiscal.Documento) string {
	if d.CajaID != "" || d.CajaCodigo != "" {
		return "Punto de venta"
	}
	return "Ventas"
}

// enRango indica si una Fecha (RFC3339) cae dentro de [desde, hasta] (YYYY-MM-DD
// inclusivos), comparando el prefijo de 10 caracteres.
func enRango(fecha, desde, hasta string) bool {
	if len(fecha) < 10 {
		return false
	}
	f := fecha[:10]
	return f >= desde && f <= hasta
}

// rangoMesActual devuelve [primer día del mes, hoy] en UTC, YYYY-MM-DD.
func rangoMesActual() (string, string) {
	now := time.Now().UTC()
	desde := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	return desde, now.Format("2006-01-02")
}

// normalizarRango aplica los defaults (mes en curso) a un rango con fechas
// faltantes o mal formadas, aceptando solo YYYY-MM-DD válidas.
func normalizarRango(desde, hasta string) (string, string) {
	d, h := rangoMesActual()
	if fechaISOValida(desde) {
		d = desde
	}
	if fechaISOValida(hasta) {
		h = hasta
	}
	return d, h
}
