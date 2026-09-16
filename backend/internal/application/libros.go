package application

import (
	"fmt"
	"sort"

	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// Libros fiscales (Libro de Ventas y Libro de Compras) — Providencia SENIAT.
//
// Son REPORTES DERIVADOS del ledger, de solo lectura: no persisten nada ni
// requieren auditoría (son consulta, no un hecho económico). Se calculan por
// contribuyente (empresa/RIF, consolidando TODAS las sedes) y por período
// mensual, plegando los documentos fiscales / órdenes de compra cuyo prefijo
// de fecha "YYYY-MM" cae en el período pedido.
//
// El layout TXT/XML EXACTO que exige el SENIAT depende de la providencia
// vigente y es configuración versionada (Principio 1): esta capa entrega las
// filas y totales; el formato oficial de exportación queda como siguiente paso.

// LibroVentasFila es un renglón del Libro de Ventas: un documento fiscal
// (factura, nota de crédito o anulación) tal como se declara ante el SENIAT.
// Las notas de crédito y anulaciones conservan su signo NEGATIVO (en el libro
// son sustracciones del débito fiscal).
type LibroVentasFila struct {
	Fecha string `json:"fecha"` // emisión, UTC RFC3339
	Tipo  string `json:"tipo"`  // factura | nota_credito | anulacion
	// NumeroCompleto es el correlativo de la factura; NumeroControl el número de
	// control fiscal, que es OBLIGATORIO en el libro de ventas y se asigna al
	// emitir. Antes no se copiaba acá y la columna del archivo salía siempre
	// vacía aunque el documento sí lo llevara.
	NumeroCompleto   string  `json:"numeroCompleto"`
	NumeroControl    string  `json:"numeroControl"`
	ClienteDocumento string  `json:"clienteDocumento"` // RIF / cédula
	ClienteNombre    string  `json:"clienteNombre"`
	Total            float64 `json:"total"` // con IVA
	BaseExenta       float64 `json:"baseExenta"`
	// BaseImponible, Alicuota e IVADebito son el AGREGADO (suma de todas las
	// alícuotas). Se conservan porque el histórico y las pantallas viejas los
	// leen, pero el libro se declara con el desglose de abajo.
	BaseImponible float64 `json:"baseImponible"`
	Alicuota      float64 `json:"alicuota"`  // fracción (0.16 = 16%)
	IVADebito     float64 `json:"ivaDebito"` // débito fiscal
	// DESGLOSE POR ALÍCUOTA. El SENIAT exige declarar cada tasa en su propia
	// columna: no se puede sumar el 16% con el 8% ni con el recargo suntuario.
	// El recargo va aparte de la general aunque comparta base — un artículo de
	// lujo paga 16%+15% y las dos porciones se informan por separado.
	BaseGeneral       float64 `json:"baseGeneral"`
	AlicuotaGeneral   float64 `json:"alicuotaGeneral"`
	IVAGeneral        float64 `json:"ivaGeneral"`
	BaseReducida      float64 `json:"baseReducida"`
	AlicuotaReducida  float64 `json:"alicuotaReducida"`
	IVAReducida       float64 `json:"ivaReducida"`
	BaseAdicional     float64 `json:"baseAdicional"`
	AlicuotaAdicional float64 `json:"alicuotaAdicional"`
	IVAAdicional      float64 `json:"ivaAdicional"`
	IGTF              float64 `json:"igtf"`
	// IVARetenido es lo que el CLIENTE (agente de retención) retuvo sobre esta
	// factura: una retención RECIBIDA. Va en el libro de ventas porque es parte
	// de la declaración del período, y sin ella la contadora tenía que cruzarla a
	// mano contra los comprobantes.
	IVARetenido float64 `json:"ivaRetenido"`
}

// LibroVentasTotales es la fila de totales al pie del libro.
type LibroVentasTotales struct {
	Total         float64 `json:"total"`
	BaseExenta    float64 `json:"baseExenta"`
	BaseImponible float64 `json:"baseImponible"`
	IVADebito     float64 `json:"ivaDebito"`
	// Totales del desglose: son los que se vuelcan en la declaración.
	BaseGeneral   float64 `json:"baseGeneral"`
	IVAGeneral    float64 `json:"ivaGeneral"`
	BaseReducida  float64 `json:"baseReducida"`
	IVAReducida   float64 `json:"ivaReducida"`
	BaseAdicional float64 `json:"baseAdicional"`
	IVAAdicional  float64 `json:"ivaAdicional"`
	IGTF          float64 `json:"igtf"`
	IVARetenido   float64 `json:"ivaRetenido"`
}

// LibroVentasResult es el Libro de Ventas de un período: filas + totales.
type LibroVentasResult struct {
	Anio    int                `json:"anio"`
	Mes     int                `json:"mes"`
	Periodo string             `json:"periodo"` // "YYYY-MM"
	Filas   []LibroVentasFila  `json:"filas"`
	Totales LibroVentasTotales `json:"totales"`
}

// LibroVentas construye el Libro de Ventas del contribuyente (empresaID) para el
// mes anio-mes, consolidando TODAS las sedes. Solo lectura, sin auditoría.
func (s *Service) LibroVentas(empresaID string, anio, mes int) LibroVentasResult {
	periodo := fmt.Sprintf("%04d-%02d", anio, mes)
	res := LibroVentasResult{Anio: anio, Mes: mes, Periodo: periodo, Filas: []LibroVentasFila{}}
	if s.documentos == nil {
		return res
	}
	// Retenciones de IVA que los clientes agentes de retención hicieron sobre
	// estas facturas. Se indexan UNA vez por documento en vez de consultarlas por
	// fila: el libro de un mes movido tiene cientos de filas.
	retenidoPorDoc := s.retencionesIVAPorDocumento(empresaID, fiscal.RetencionRecibida)
	for _, d := range s.documentos.List(empresaID) {
		switch d.Tipo {
		case fiscal.TipoFactura, fiscal.TipoNotaCredito, fiscal.TipoNotaDebito, fiscal.TipoAnulacion:
			// La nota de débito se declara como un débito POSITIVO adicional (aumenta
			// la base imponible y el IVA débito del período), como una factura.
		default:
			continue
		}
		// Filtro por mes: el prefijo "YYYY-MM" de la Fecha RFC3339 UTC ordena
		// cronológicamente como string.
		if len(d.Fecha) < 7 || d.Fecha[:7] != periodo {
			continue
		}
		alic := d.AlicuotaIVA
		if alic == 0 {
			alic = fiscal.AlicuotaIVA
		}
		fila := LibroVentasFila{
			Fecha:            d.Fecha,
			Tipo:             d.Tipo,
			NumeroCompleto:   d.NumeroCompleto,
			NumeroControl:    d.NumeroControl,
			ClienteDocumento: d.ClienteDocumento,
			ClienteNombre:    d.ClienteNombre,
			Total:            d.Total,         // NC/anulación: negativo
			BaseExenta:       d.BaseExenta,    // negativo en reversas
			BaseImponible:    d.BaseImponible, // negativo en reversas
			Alicuota:         alic,
			IVADebito:        d.IVA,  // negativo en reversas
			IGTF:             d.IGTF, // negativo en reversas
			IVARetenido:      retenidoPorDoc[d.ID],
		}
		desglosarVenta(&fila, d, alic)
		res.Filas = append(res.Filas, fila)
	}
	// Orden por Fecha ascendente y, a igualdad, por NumeroCompleto (insertion
	// sort, como el resto del código). Ambos son strings ordenables.
	f := res.Filas
	for i := 1; i < len(f); i++ {
		for j := i; j > 0 && menorVenta(f[j], f[j-1]); j-- {
			f[j], f[j-1] = f[j-1], f[j]
		}
	}
	var t LibroVentasTotales
	for _, r := range res.Filas {
		t.Total += r.Total
		t.BaseExenta += r.BaseExenta
		t.BaseImponible += r.BaseImponible
		t.IVADebito += r.IVADebito
		t.BaseGeneral += r.BaseGeneral
		t.IVAGeneral += r.IVAGeneral
		t.BaseReducida += r.BaseReducida
		t.IVAReducida += r.IVAReducida
		t.BaseAdicional += r.BaseAdicional
		t.IVAAdicional += r.IVAAdicional
		t.IGTF += r.IGTF
		t.IVARetenido += r.IVARetenido
	}
	t.Total = round2(t.Total)
	t.BaseExenta = round2(t.BaseExenta)
	t.BaseImponible = round2(t.BaseImponible)
	t.IVADebito = round2(t.IVADebito)
	t.BaseGeneral = round2(t.BaseGeneral)
	t.IVAGeneral = round2(t.IVAGeneral)
	t.BaseReducida = round2(t.BaseReducida)
	t.IVAReducida = round2(t.IVAReducida)
	t.BaseAdicional = round2(t.BaseAdicional)
	t.IVAAdicional = round2(t.IVAAdicional)
	t.IGTF = round2(t.IGTF)
	t.IVARetenido = round2(t.IVARetenido)
	res.Totales = t
	return res
}

// desglosarVenta reparte la base y el impuesto del documento en las columnas por
// alícuota que exige el libro.
//
// La fuente es `Documento.Impuestos`, el desglose que el motor sella al emitir.
// Un documento ANTERIOR al maestro de impuestos lo trae vacío: para esos toda la
// base imponible se declara como general con la alícuota del documento, que es
// exactamente lo que era antes de que existieran varias tasas. Sin ese respaldo
// el histórico saldría en cero y el libro dejaría de cuadrar.
func desglosarVenta(fila *LibroVentasFila, d fiscal.Documento, alic float64) {
	if len(d.Impuestos) == 0 {
		fila.BaseGeneral = d.BaseImponible
		fila.AlicuotaGeneral = alic
		fila.IVAGeneral = d.IVA
		return
	}
	for _, imp := range d.Impuestos {
		switch imp.Tipo {
		case fiscal.TipoGeneral:
			fila.BaseGeneral = round2(fila.BaseGeneral + imp.Base)
			fila.IVAGeneral = round2(fila.IVAGeneral + imp.Monto)
			fila.AlicuotaGeneral = imp.Porcentaje
		case fiscal.TipoReducida:
			fila.BaseReducida = round2(fila.BaseReducida + imp.Base)
			fila.IVAReducida = round2(fila.IVAReducida + imp.Monto)
			fila.AlicuotaReducida = imp.Porcentaje
		case fiscal.TipoAdicional:
			// El recargo suntuario comparte base con la general pero se declara
			// aparte: sumarlo a la general daría un 31% que el libro no admite.
			fila.BaseAdicional = round2(fila.BaseAdicional + imp.Base)
			fila.IVAAdicional = round2(fila.IVAAdicional + imp.Monto)
			fila.AlicuotaAdicional = imp.Porcentaje
		case fiscal.TipoExento:
			// La base exenta ya viene del documento; contarla aquí la duplicaría.
		}
	}
}

// retencionesIVAPorDocumento indexa el IVA retenido por documento, en una
// dirección (recibida = el cliente me retuvo; emitida = yo le retuve al
// proveedor). Devuelve un mapa vacío si no hay repositorio de retenciones: el
// libro tiene que poder armarse igual, solo que sin esa columna.
func (s *Service) retencionesIVAPorDocumento(empresaID, direccion string) map[string]float64 {
	out := map[string]float64{}
	if s.retenciones == nil {
		return out
	}
	for _, r := range s.retenciones.List(empresaID) {
		if r.Tipo != direccion || r.Impuesto != fiscal.ImpuestoIVA || r.DocumentoID == "" {
			continue
		}
		out[r.DocumentoID] = round2(out[r.DocumentoID] + r.MontoRetenido)
	}
	return out
}

// menorVenta ordena por Fecha y, a igualdad, por NumeroCompleto.
func menorVenta(a, b LibroVentasFila) bool {
	if a.Fecha != b.Fecha {
		return a.Fecha < b.Fecha
	}
	return a.NumeroCompleto < b.NumeroCompleto
}

// LibroComprasFila es un renglón del Libro de Compras: una factura FISCAL de un
// proveedor tal como se declara ante el SENIAT, con su número de control.
type LibroComprasFila struct {
	Fecha            string  `json:"fecha"` // fecha del documento del proveedor
	ProveedorRIF     string  `json:"proveedorRif"`
	ProveedorNombre  string  `json:"proveedorNombre"`
	NumeroFactura    string  `json:"numeroFactura"` // Nº de la factura del proveedor
	NumeroControl    string  `json:"numeroControl"` // Nº de control fiscal (SENIAT)
	Total            float64 `json:"total"`
	BaseExenta       float64 `json:"baseExenta"`
	BaseImponible    float64 `json:"baseImponible"`
	Alicuota         float64 `json:"alicuota"`
	IVACreditoFiscal float64 `json:"ivaCreditoFiscal"`
	// Desglose por alícuota. La factura de un proveedor trae UNA tasa (la que él
	// aplicó), así que cada fila cae entera en general o en reducida según a cuál
	// se parezca su tasa efectiva. No hay columna adicional: el recargo suntuario
	// lo declara quien VENDE, no quien compra.
	BaseGeneral      float64 `json:"baseGeneral"`
	AlicuotaGeneral  float64 `json:"alicuotaGeneral"`
	IVAGeneral       float64 `json:"ivaGeneral"`
	BaseReducida     float64 `json:"baseReducida"`
	AlicuotaReducida float64 `json:"alicuotaReducida"`
	IVAReducida      float64 `json:"ivaReducida"`
	// IVARetenido es lo que TÚ le retuviste al proveedor sobre esta factura (una
	// retención EMITIDA). Es columna obligatoria del libro de compras del
	// contribuyente especial y antes salía siempre vacía, aunque el comprobante
	// existiera.
	IVARetenido float64 `json:"ivaRetenido"`
}

// LibroComprasTotales es la fila de totales del Libro de Compras.
type LibroComprasTotales struct {
	Total            float64 `json:"total"`
	BaseExenta       float64 `json:"baseExenta"`
	BaseImponible    float64 `json:"baseImponible"`
	IVACreditoFiscal float64 `json:"ivaCreditoFiscal"`
	BaseGeneral      float64 `json:"baseGeneral"`
	IVAGeneral       float64 `json:"ivaGeneral"`
	BaseReducida     float64 `json:"baseReducida"`
	IVAReducida      float64 `json:"ivaReducida"`
	IVARetenido      float64 `json:"ivaRetenido"`
}

// LibroComprasResult es el Libro de Compras de un período.
type LibroComprasResult struct {
	Anio    int                 `json:"anio"`
	Mes     int                 `json:"mes"`
	Periodo string              `json:"periodo"`
	Filas   []LibroComprasFila  `json:"filas"`
	Totales LibroComprasTotales `json:"totales"`
}

// LibroCompras construye el Libro de Compras del contribuyente (empresaID) para
// el mes anio-mes, consolidando TODAS las sedes. La fuente son las FACTURAS de
// compra de proveedores registradas cuya fecha de documento cae en el mes: las
// órdenes recibidas sin factura NO son compras fiscales y no se declaran. Solo
// lectura, sin auditoría.
func (s *Service) LibroCompras(empresaID string, anio, mes int) LibroComprasResult {
	periodo := fmt.Sprintf("%04d-%02d", anio, mes)
	res := LibroComprasResult{Anio: anio, Mes: mes, Periodo: periodo, Filas: []LibroComprasFila{}}
	if s.facturasCompra == nil {
		return res
	}
	alicEmpresa := s.alicuotaIVA(empresaID)
	// Retenciones de IVA que la empresa le hizo a sus proveedores sobre estas
	// facturas. Indexadas una sola vez, como en el libro de ventas.
	retenidoPorDoc := s.retencionesIVAPorDocumento(empresaID, fiscal.RetencionEmitida)
	for _, f := range s.facturasCompra.List(empresaID) {
		// Filtro por mes: prefijo "YYYY-MM" de la fecha del documento del proveedor.
		if len(f.Fecha) < 7 || f.Fecha[:7] != periodo {
			continue
		}
		// La alícuota se deriva de la PROPIA factura (IVA / base), no de la
		// configurada por la empresa: el proveedor pudo facturar al 8% y declarar
		// el 16% de la empresa sería declarar algo que no ocurrió.
		alic := alicEmpresa
		if f.BaseImponible > 0.005 || f.BaseImponible < -0.005 {
			alic = redondearAlicuota(f.IVA / f.BaseImponible)
		}
		fila := LibroComprasFila{
			Fecha:            f.Fecha,
			ProveedorRIF:     f.ProveedorRIF,
			ProveedorNombre:  f.ProveedorNombre,
			NumeroFactura:    f.NumeroFactura,
			NumeroControl:    f.NumeroControl,
			Total:            f.Total,
			BaseExenta:       f.BaseExenta,
			BaseImponible:    f.BaseImponible,
			Alicuota:         alic,
			IVACreditoFiscal: f.IVA,
			IVARetenido:      retenidoPorDoc[f.ID],
		}
		// Una tasa por factura: cae entera en la columna a la que corresponde.
		if s.esAlicuotaReducida(empresaID, alic, f.Fecha) {
			fila.BaseReducida, fila.AlicuotaReducida, fila.IVAReducida = f.BaseImponible, alic, f.IVA
		} else {
			fila.BaseGeneral, fila.AlicuotaGeneral, fila.IVAGeneral = f.BaseImponible, alic, f.IVA
		}
		res.Filas = append(res.Filas, fila)
	}
	// Orden por Fecha ascendente y, a igualdad, por NumeroCompleto.
	f := res.Filas
	for i := 1; i < len(f); i++ {
		for j := i; j > 0 && menorCompra(f[j], f[j-1]); j-- {
			f[j], f[j-1] = f[j-1], f[j]
		}
	}
	var t LibroComprasTotales
	for _, r := range res.Filas {
		t.Total += r.Total
		t.BaseExenta += r.BaseExenta
		t.BaseImponible += r.BaseImponible
		t.IVACreditoFiscal += r.IVACreditoFiscal
		t.BaseGeneral += r.BaseGeneral
		t.IVAGeneral += r.IVAGeneral
		t.BaseReducida += r.BaseReducida
		t.IVAReducida += r.IVAReducida
		t.IVARetenido += r.IVARetenido
	}
	t.Total = round2(t.Total)
	t.BaseExenta = round2(t.BaseExenta)
	t.BaseImponible = round2(t.BaseImponible)
	t.IVACreditoFiscal = round2(t.IVACreditoFiscal)
	t.BaseGeneral = round2(t.BaseGeneral)
	t.IVAGeneral = round2(t.IVAGeneral)
	t.BaseReducida = round2(t.BaseReducida)
	t.IVAReducida = round2(t.IVAReducida)
	t.IVARetenido = round2(t.IVARetenido)
	res.Totales = t
	return res
}

// redondearAlicuota deja la tasa efectiva en 4 decimales (0.1600, 0.0800). El
// cociente IVA/base arrastra el redondeo a céntimos de los dos montos y sin esto
// una factura al 16% saldría como 0.15997 y el libro mostraría «15,997%».
func redondearAlicuota(v float64) float64 {
	return float64(int64(v*10000+0.5)) / 10000
}

// esAlicuotaReducida decide si una tasa corresponde a la alícuota REDUCIDA del
// maestro, para saber en qué columna del libro declarar la factura. Se compara
// contra el maestro vigente a la fecha del documento —no contra el de hoy—
// porque una providencia puede haber cambiado la tasa después.
//
// Sin maestro cargado no hay con qué comparar y todo va a la general, que es el
// comportamiento de siempre.
func (s *Service) esAlicuotaReducida(empresaID string, alic float64, fecha string) bool {
	if s.alicuotas == nil || alic <= 0 {
		return false
	}
	red, ok := fiscal.VigenteEn(s.alicuotas.List(empresaID), fiscal.CodReducida, fecha)
	if !ok {
		return false
	}
	dif := alic - red.Porcentaje
	return dif < 0.0005 && dif > -0.0005
}

// menorCompra ordena por Fecha y, a igualdad, por número de control fiscal.
func menorCompra(a, b LibroComprasFila) bool {
	if a.Fecha != b.Fecha {
		return a.Fecha < b.Fecha
	}
	return a.NumeroControl < b.NumeroControl
}

// LibroInventarioFila es un renglón del Libro de Inventario: la existencia
// valorizada de un producto A LA FECHA, consolidando TODAS las sedes del
// contribuyente. Cantidad, costo promedio y valor se PLIEGAN del ledger
// append-only (misma proyección que Existencias / ReporteInventario, vía fold):
// nunca es un contador editable.
type LibroInventarioFila struct {
	SKU           string  `json:"sku"`
	Nombre        string  `json:"nombre"`
	UnidadBase    string  `json:"unidadBase"`
	Cantidad      float64 `json:"cantidad"`
	CostoPromedio float64 `json:"costoPromedio"`
	Valor         float64 `json:"valor"`
}

// LibroInventarioTotales resume el libro: número de productos con existencia y
// el valor total del inventario. La cantidad NO se suma (mezclaría unidades
// distintas): el único total con sentido contable es el valor.
type LibroInventarioTotales struct {
	NumeroProductos int     `json:"numeroProductos"`
	Valor           float64 `json:"valor"`
}

// LibroInventarioResult es el Libro de Inventario a una fecha de corte.
type LibroInventarioResult struct {
	Fecha   string                 `json:"fecha"` // corte (generación) UTC RFC3339
	Filas   []LibroInventarioFila  `json:"filas"`
	Totales LibroInventarioTotales `json:"totales"`
}

// LibroInventario construye el Libro de Inventario del contribuyente (empresaID)
// a la fecha actual, consolidando TODAS las sedes. Es un REPORTE DERIVADO del
// ledger, de solo lectura y sin auditoría (consulta, no hecho económico): por
// cada producto con existencia distinta de cero pliega cantidad y costo promedio
// del ledger (fold, sin filtrar por sede — sigue aislado por empresaID) y calcula
// su valor. Los productos sin existencia no se declaran. Ordenado por SKU.
//
// El layout TXT/XML oficial del Libro de Inventarios y Balances depende de la
// providencia vigente (configuración versionada, Principio 1): esta capa entrega
// filas y totales; el formato oficial de exportación queda como siguiente paso.
func (s *Service) LibroInventario(empresaID string) LibroInventarioResult {
	res := LibroInventarioResult{Fecha: ahora(), Filas: []LibroInventarioFila{}}
	if s.productos == nil || s.movimientos == nil {
		return res
	}
	for _, p := range s.productos.List(empresaID) {
		movs := s.movimientos.List(empresaID, inventario.FiltroMovimiento{ProductoID: p.ID})
		cant, avg := fold(movs)
		// Solo se declara lo que hay en existencia: un producto en cero no aporta al
		// inventario a la fecha.
		if cant > -1e-9 && cant < 1e-9 {
			continue
		}
		valor := round2(cant * avg)
		res.Filas = append(res.Filas, LibroInventarioFila{
			SKU: p.SKU, Nombre: p.Nombre, UnidadBase: p.UnidadBase,
			Cantidad: round2(cant), CostoPromedio: round2(avg), Valor: valor,
		})
		res.Totales.NumeroProductos++
		res.Totales.Valor += valor
	}
	res.Totales.Valor = round2(res.Totales.Valor)
	sort.Slice(res.Filas, func(i, j int) bool { return res.Filas[i].SKU < res.Filas[j].SKU })
	return res
}
