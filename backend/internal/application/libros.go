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
	Fecha            string  `json:"fecha"` // emisión, UTC RFC3339
	Tipo             string  `json:"tipo"`  // factura | nota_credito | anulacion
	NumeroCompleto   string  `json:"numeroCompleto"`
	ClienteDocumento string  `json:"clienteDocumento"` // RIF / cédula
	ClienteNombre    string  `json:"clienteNombre"`
	Total            float64 `json:"total"` // con IVA
	BaseExenta       float64 `json:"baseExenta"`
	BaseImponible    float64 `json:"baseImponible"`
	Alicuota         float64 `json:"alicuota"`  // fracción (0.16 = 16%)
	IVADebito        float64 `json:"ivaDebito"` // débito fiscal
	IGTF             float64 `json:"igtf"`
}

// LibroVentasTotales es la fila de totales al pie del libro.
type LibroVentasTotales struct {
	Total         float64 `json:"total"`
	BaseExenta    float64 `json:"baseExenta"`
	BaseImponible float64 `json:"baseImponible"`
	IVADebito     float64 `json:"ivaDebito"`
	IGTF          float64 `json:"igtf"`
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
		res.Filas = append(res.Filas, LibroVentasFila{
			Fecha:            d.Fecha,
			Tipo:             d.Tipo,
			NumeroCompleto:   d.NumeroCompleto,
			ClienteDocumento: d.ClienteDocumento,
			ClienteNombre:    d.ClienteNombre,
			Total:            d.Total,         // NC/anulación: negativo
			BaseExenta:       d.BaseExenta,    // negativo en reversas
			BaseImponible:    d.BaseImponible, // negativo en reversas
			Alicuota:         alic,
			IVADebito:        d.IVA,  // negativo en reversas
			IGTF:             d.IGTF, // negativo en reversas
		})
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
		t.IGTF += r.IGTF
	}
	t.Total = round2(t.Total)
	t.BaseExenta = round2(t.BaseExenta)
	t.BaseImponible = round2(t.BaseImponible)
	t.IVADebito = round2(t.IVADebito)
	t.IGTF = round2(t.IGTF)
	res.Totales = t
	return res
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
}

// LibroComprasTotales es la fila de totales del Libro de Compras.
type LibroComprasTotales struct {
	Total            float64 `json:"total"`
	BaseExenta       float64 `json:"baseExenta"`
	BaseImponible    float64 `json:"baseImponible"`
	IVACreditoFiscal float64 `json:"ivaCreditoFiscal"`
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
	alic := s.alicuotaIVA(empresaID)
	for _, f := range s.facturasCompra.List(empresaID) {
		// Filtro por mes: prefijo "YYYY-MM" de la fecha del documento del proveedor.
		if len(f.Fecha) < 7 || f.Fecha[:7] != periodo {
			continue
		}
		res.Filas = append(res.Filas, LibroComprasFila{
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
		})
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
	}
	t.Total = round2(t.Total)
	t.BaseExenta = round2(t.BaseExenta)
	t.BaseImponible = round2(t.BaseImponible)
	t.IVACreditoFiscal = round2(t.IVACreditoFiscal)
	res.Totales = t
	return res
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
