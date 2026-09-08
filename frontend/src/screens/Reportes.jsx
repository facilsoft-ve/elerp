import { useState, useEffect, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Card, Badge, Stat, Segmented, Select, Empty, PageHeader, TableSkeleton } from '../components/primitives.jsx'
import { BarChart, DonutChart } from '../components/charts.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { RegistroActividad } from './RegistroActividad.jsx'

/* Reportes y BI — la cara de lectura de los reportes que el backend ya pliega del
 * ledger (documentos fiscales, movimientos de inventario, órdenes de compra y
 * cuentas por cobrar). No captura ni recalcula nada: pinta lo que reportes.go
 * devuelve. Cinco vistas, navegadas por el menú lateral (sin pestañas en el
 * contenedor): el submódulo activo se deriva de la ruta "reportes:<sub>".
 *
 * Los gráficos SÍ aportan aquí: es el módulo donde una tendencia o un donut dicen
 * más que una fila de tabla. Se usan con criterio, siempre acompañados del número
 * exacto en un KPI o en la tabla de al lado. El verde-dinero NO se usa como color
 * de datos (regla del sistema visual): azul de marca primero, luego teal. */

const SUBS = [
  { id: 'panel', label: 'Panel ejecutivo' },
  { id: 'ventas', label: 'Ventas' },
  { id: 'inventario', label: 'Inventario' },
  { id: 'compras', label: 'Compras' },
  { id: 'cobranza', label: 'Cobranza' },
  { id: 'actividad', label: 'Registro de actividad' },
]

// Paleta de series: azul de marca, acento teal, navies/ámbar/gris/verde-tabla.
const SERIE = ['#1D3477', '#09B69B', '#2A4A8F', '#92600A', '#5C6470', '#166B41']

export function Reportes({ route }) {
  const [tab, setTab] = useState((route || '').split(':')[1] || 'panel')
  useEffect(() => { setTab((route || '').split(':')[1] || 'panel') }, [route])
  const label = SUBS.find((s) => s.id === tab)?.label

  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader
        breadcrumb={['Reportes y BI', label]}
        title="Reportes y BI"
        sub="Indicadores derivados del ledger: ventas, inventario, compras y cobranza. Consolidan todas las sedes; son de solo lectura." />
      {tab === 'panel' ? <PanelEjecutivo /> : null}
      {tab === 'ventas' ? <ReporteVentas /> : null}
      {tab === 'inventario' ? <ReporteInventario /> : null}
      {tab === 'compras' ? <ReporteCompras /> : null}
      {tab === 'cobranza' ? <ReporteCobranza /> : null}
      {tab === 'actividad' ? <RegistroActividad /> : null}
    </div>
  )
}

/* ===================== Infraestructura compartida ===================== */

// useReporte centraliza el fetch de cada vista con sus tres estados:
// data === undefined → cargando · data === null → error · si no, datos.
function useReporte(fetcher, deps) {
  const [data, setData] = useState(undefined)
  const [error, setError] = useState(null)
  const cargar = useCallback(() => {
    setError(null)
    setData(undefined)
    fetcher().then(setData).catch((e) => { setData(null); setError(e) })
  }, deps) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { cargar() }, [cargar])
  return { data, error, loading: data === undefined, reload: cargar }
}

// EstadoReporte pinta carga/error y delega los datos al render hijo.
function EstadoReporte({ loading, error, onRetry, cols = 4, children }) {
  if (loading) {
    return (
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
        <TableSkeleton rows={6} cols={cols} />
      </div>
    )
  }
  if (error) {
    return (
      <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar el reporte"
        body={String(error.message || error)}
        cta={<Button onClick={onRetry} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
    )
  }
  return children
}

// Selector de período mes/año — mismo patrón que los libros fiscales. Devuelve el
// rango [primer día, último día] del mes elegido en YYYY-MM-DD.
const MESES = ['Enero', 'Febrero', 'Marzo', 'Abril', 'Mayo', 'Junio', 'Julio', 'Agosto', 'Septiembre', 'Octubre', 'Noviembre', 'Diciembre']

function rangoDeMes(anio, mes) {
  const desde = `${anio}-${String(mes).padStart(2, '0')}-01`
  const ultimo = new Date(Date.UTC(anio, mes, 0)).getUTCDate()
  const hasta = `${anio}-${String(mes).padStart(2, '0')}-${String(ultimo).padStart(2, '0')}`
  return { desde, hasta }
}

function PeriodoSelector({ anio, mes, setAnio, setMes }) {
  const y = new Date().getUTCFullYear()
  const anios = [y, y - 1, y - 2, y - 3, y - 4]
  return (
    <div className="flex items-center gap-2">
      <Select value={mes} onChange={(e) => setMes(Number(e.target.value))} className="w-36">
        {MESES.map((m, i) => <option key={i} value={i + 1}>{m}</option>)}
      </Select>
      <Select value={anio} onChange={(e) => setAnio(Number(e.target.value))} className="w-24">
        {anios.map((a) => <option key={a} value={a}>{a}</option>)}
      </Select>
    </div>
  )
}

// Botón de exportación a CSV (client-side). Recibe encabezados y filas de valores
// crudos (sin formato de moneda) y dispara la descarga con un Blob + BOM.
function BotonExportar({ nombre, headers, rows, disabled }) {
  const exportar = () => exportarCSV(nombre, headers, rows)
  return (
    <Button variant="secondary" size="sm" icon={<Icon.Download size={15} />} disabled={disabled || !rows.length}
      onClick={exportar} title={rows.length ? 'Exportar la tabla a CSV' : 'No hay filas que exportar'}>
      Exportar CSV
    </Button>
  )
}

function exportarCSV(nombre, headers, rows) {
  const all = [headers, ...rows]
  const csv = all.map((r) => r.map(csvCell).join(',')).join('\r\n')
  const blob = new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${nombre}.csv`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

function csvCell(v) {
  const s = v == null ? '' : String(v)
  if (/[",\r\n]/.test(s)) return '"' + s.replace(/"/g, '""') + '"'
  return s
}

// Clases de tabla compartidas (thead uppercase 11px + fila con row-hover).
const TCard = ({ children }) => (
  <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
    <div className="overflow-x-auto">{children}</div>
  </div>
)
const THead = ({ cols }) => (
  <thead>
    <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
      {cols.map((c, i) => (
        <th key={i} className={`py-2.5 px-3 font-medium ${c.right ? 'text-right' : ''}`}>{c.label}</th>
      ))}
    </tr>
  </thead>
)

// Rótulo de rango bajo los controles ("Marzo 2026 · todas las sedes").
function EtiquetaRango({ anio, mes }) {
  return (
    <div className="mb-3 text-[12.5px] text-slate-500">
      <span className="font-medium text-slate-700 dark:text-slate-200">{MESES[mes - 1]} {anio}</span>
      <span className="text-slate-400"> · todas las sedes de la empresa</span>
    </div>
  )
}

/* Tendencia diaria (barras). porDia = [{fecha, monto}]. Etiquetas de eje: día del
 * mes al inicio/medio y "hoy" al final (las pone el propio BarChart). */
function TendenciaVentas({ porDia, enMoneda, height = 230 }) {
  const puntos = (porDia || []).map((p) => ({ d: (p.fecha || '').slice(8, 10), v: p.monto }))
  const hayDatos = puntos.some((p) => p.v > 0)
  if (!hayDatos) {
    return (
      <Empty framed={false} icon={<Icon.Chart size={22} />}
        title="Sin ventas en el período"
        body="La tendencia diaria aparece en cuanto haya facturas emitidas en este rango." />
    )
  }
  return <BarChart data={puntos} height={height} fmt={(v) => enMoneda(v)} />
}

/* Donut de composición con leyenda. items = [{label, monto}]. Filtra los montos
 * no positivos (una serie neta puede tener negativos que el donut no representa).
 * center muestra el total. */
function DonutComposicion({ items, enMoneda, size = 168 }) {
  const positivos = (items || []).filter((i) => i.monto > 0)
  if (!positivos.length) {
    return (
      <Empty framed={false} icon={<Icon.Chart size={22} />}
        title="Sin datos para el gráfico" body="Aún no hay montos que componer." />
    )
  }
  const total = positivos.reduce((a, b) => a + b.monto, 0)
  const data = positivos.map((i, idx) => ({ value: i.monto, color: SERIE[idx % SERIE.length] }))
  return (
    <div className="flex items-center gap-5 flex-wrap">
      <DonutChart data={data} size={size} center={
        <div className="text-center">
          <div className="text-[10.5px] uppercase tracking-wide text-slate-400">Total</div>
          <div className="num text-[15px] font-semibold private-mask">{enMoneda(total)}</div>
        </div>
      } />
      <div className="space-y-2 min-w-0">
        {positivos.map((i, idx) => (
          <div key={idx} className="flex items-center gap-2 text-[13px] min-w-0">
            <span className="w-2.5 h-2.5 rounded-sm shrink-0" style={{ background: SERIE[idx % SERIE.length] }} />
            <span className="text-slate-600 dark:text-slate-300 truncate">{i.label}</span>
            <span className="num text-slate-500 ml-auto pl-3 private-mask">{enMoneda(i.monto)}</span>
            <span className="num text-[11.5px] text-slate-400 w-10 text-right">{Math.round((i.monto / total) * 100)}%</span>
          </div>
        ))}
      </div>
    </div>
  )
}

/* Ranking horizontal (top productos): barra proporcional + monto. */
function RankingBarras({ items, enMoneda, max }) {
  if (!items.length) {
    return <div className="text-[12.5px] text-slate-500 py-6 text-center">Sin productos en el período.</div>
  }
  const tope = max || Math.max(...items.map((i) => Math.abs(i.monto)), 1)
  return (
    <div className="space-y-2.5">
      {items.map((i, idx) => (
        <div key={idx} className="min-w-0">
          <div className="flex items-baseline gap-2 mb-1">
            <span className="text-[13px] font-medium truncate">{i.nombre || i.sku}</span>
            <span className="num text-[12.5px] text-slate-500 ml-auto shrink-0 private-mask">{enMoneda(i.monto)}</span>
          </div>
          <div className="h-1.5 rounded-full bg-slate-100 dark:bg-slate-800 overflow-hidden">
            <div className="h-full rounded-full" style={{ width: `${Math.max(2, (Math.abs(i.monto) / tope) * 100)}%`, background: SERIE[0] }} />
          </div>
        </div>
      ))}
    </div>
  )
}

/* ===================== 1 · Panel ejecutivo ===================== */

function PanelEjecutivo() {
  const { ui } = useUI()
  const ccy = ui.ccy
  const enMoneda = (v) => fmtCurrency(v, ccy)
  const { data, error, loading, reload } = useReporte(() => api.repPanel(), [])

  return (
    <EstadoReporte loading={loading} error={error} onRetry={reload}>
      {data ? <PanelContenido data={data} enMoneda={enMoneda} /> : null}
    </EstadoReporte>
  )
}

function PanelContenido({ data, enMoneda }) {
  const v = data.ventas || {}
  const inv = data.inventario || {}
  const topProd = (v.porProducto || []).filter((p) => p.monto > 0).slice(0, 6)

  return (
    <div className="space-y-5">
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-3">
        <Stat label="Ventas del mes" value={enMoneda(v.ventasNetas)} icon={<Icon.Receipt size={14} />}
          sub={`${fmtNum(v.cantidadFacturas, 0)} factura(s)`} accent />
        <Stat label="Ticket promedio" value={enMoneda(v.ticketPromedio)} icon={<Icon.Tag size={14} />} />
        <Stat label="Valorización inventario" value={enMoneda(inv.valorizacionTotal)} icon={<Icon.Boxes size={14} />}
          sub={`${fmtNum(inv.numeroProductos, 0)} producto(s)`} />
        <Stat label="Por cobrar" value={enMoneda(data.totalPorCobrar)} icon={<Icon.Wallet size={14} />}
          sub={data.totalVencido ? `${enMoneda(data.totalVencido)} vencido` : 'sin vencidos'} />
        <Stat label="Órdenes abiertas" value={fmtNum(data.ordenesAbiertas, 0)} icon={<Icon.Cart size={14} />}
          sub={data.pendienteRecibirMonto ? `${enMoneda(data.pendienteRecibirMonto)} por recibir` : 'nada por recibir'} />
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-3 gap-4">
        <Card className="xl:col-span-2">
          <div className="font-display font-semibold text-[16px] mb-1">Tendencia de ventas</div>
          <div className="text-[12.5px] text-slate-500 mb-4">Ventas netas por día del mes en curso.</div>
          <TendenciaVentas porDia={v.porDia} enMoneda={enMoneda} />
        </Card>
        <Card>
          <div className="font-display font-semibold text-[16px] mb-1">Ventas por canal</div>
          <div className="text-[12.5px] text-slate-500 mb-4">Punto de venta vs. Ventas.</div>
          <DonutComposicion items={(v.porCanal || []).map((c) => ({ label: c.canal, monto: c.monto }))} enMoneda={enMoneda} />
        </Card>
      </div>

      <Card>
        <div className="font-display font-semibold text-[16px] mb-1">Top productos del mes</div>
        <div className="text-[12.5px] text-slate-500 mb-4">Los seis productos con mayor venta neta.</div>
        <RankingBarras items={topProd} enMoneda={enMoneda} />
      </Card>
    </div>
  )
}

/* ===================== 2 · Ventas ===================== */

function ReporteVentas() {
  const { ui } = useUI()
  const ccy = ui.ccy
  const enMoneda = (v) => fmtCurrency(v, ccy)
  const hoy = new Date()
  const [anio, setAnio] = useState(hoy.getUTCFullYear())
  const [mes, setMes] = useState(hoy.getUTCMonth() + 1)
  const [tabla, setTabla] = useState('productos')
  const { desde, hasta } = rangoDeMes(anio, mes)
  const { data, error, loading, reload } = useReporte(() => api.repVentas(desde, hasta), [desde, hasta])

  const v = data || {}
  const exportInfo = ventasExport(tabla, v, ccy, anio, mes)

  return (
    <div>
      <div className="flex items-center gap-3 flex-wrap mb-3">
        <PeriodoSelector anio={anio} mes={mes} setAnio={setAnio} setMes={setMes} />
        <div className="ml-auto">
          <BotonExportar nombre={exportInfo.nombre} headers={exportInfo.headers} rows={exportInfo.rows} disabled={loading || error} />
        </div>
      </div>
      <EtiquetaRango anio={anio} mes={mes} />

      <EstadoReporte loading={loading} error={error} onRetry={reload}>
        {data ? (
          <div className="space-y-5">
            <div className="grid grid-cols-2 lg:grid-cols-5 gap-3">
              <Stat label="Ventas netas" value={enMoneda(v.ventasNetas)} icon={<Icon.Receipt size={14} />} accent />
              <Stat label="IVA débito" value={enMoneda(v.ivaDebito)} icon={<Icon.Banknote size={14} />} />
              <Stat label="IGTF" value={enMoneda(v.igtf)} icon={<Icon.Banknote size={14} />} />
              <Stat label="Facturas" value={fmtNum(v.cantidadFacturas, 0)} icon={<Icon.Book size={14} />} />
              <Stat label="Ticket promedio" value={enMoneda(v.ticketPromedio)} icon={<Icon.Tag size={14} />} />
            </div>

            <div className="grid grid-cols-1 xl:grid-cols-3 gap-4">
              <Card className="xl:col-span-2">
                <div className="font-display font-semibold text-[16px] mb-1">Tendencia diaria</div>
                <div className="text-[12.5px] text-slate-500 mb-4">Ventas netas por día del período.</div>
                <TendenciaVentas porDia={v.porDia} enMoneda={enMoneda} />
              </Card>
              <Card>
                <div className="font-display font-semibold text-[16px] mb-1">Por canal</div>
                <div className="text-[12.5px] text-slate-500 mb-4">Punto de venta vs. Ventas.</div>
                <DonutComposicion items={(v.porCanal || []).map((c) => ({ label: c.canal, monto: c.monto }))} enMoneda={enMoneda} />
              </Card>
            </div>

            <div>
              <div className="flex items-center justify-between gap-3 mb-3 flex-wrap">
                <Segmented value={tabla} onChange={setTabla} options={[
                  { value: 'productos', label: 'Top productos' },
                  { value: 'clientes', label: 'Por cliente' },
                  { value: 'sedes', label: 'Por sede' },
                ]} />
              </div>
              {tabla === 'productos' ? <TablaProductos filas={v.porProducto || []} enMoneda={enMoneda} /> : null}
              {tabla === 'clientes' ? <TablaClientes filas={v.porCliente || []} enMoneda={enMoneda} /> : null}
              {tabla === 'sedes' ? <TablaSedes filas={v.porSede || []} enMoneda={enMoneda} /> : null}
            </div>
          </div>
        ) : null}
      </EstadoReporte>
    </div>
  )
}

function TablaProductos({ filas, enMoneda }) {
  if (!filas.length) return <Empty icon={<Icon.Package size={22} />} title="Sin ventas por producto" body="No hubo líneas facturadas en el período." />
  return (
    <TCard>
      <table className="w-full text-sm">
        <THead cols={[{ label: 'SKU' }, { label: 'Producto' }, { label: 'Cantidad', right: true }, { label: 'Monto', right: true }]} />
        <tbody>
          {filas.map((f, i) => (
            <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
              <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.sku}</td>
              <td className="py-2 px-3 text-[13px] max-w-[280px] truncate">{f.nombre || '—'}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtNum(f.cantidad, 2)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium private-mask">{enMoneda(f.monto)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TCard>
  )
}

function TablaClientes({ filas, enMoneda }) {
  if (!filas.length) return <Empty icon={<Icon.Users size={22} />} title="Sin ventas por cliente" body="No hubo documentos en el período." />
  return (
    <TCard>
      <table className="w-full text-sm">
        <THead cols={[{ label: 'Cliente' }, { label: 'Documentos', right: true }, { label: 'Monto', right: true }]} />
        <tbody>
          {filas.map((f, i) => (
            <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
              <td className="py-2 px-3 text-[13px] max-w-[360px] truncate">{f.clienteNombre || '—'}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtNum(f.documentos, 0)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium private-mask">{enMoneda(f.monto)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TCard>
  )
}

function TablaSedes({ filas, enMoneda }) {
  if (!filas.length) return <Empty icon={<Icon.Boxes size={22} />} title="Sin ventas por sede" body="No hubo documentos en el período." />
  return (
    <TCard>
      <table className="w-full text-sm">
        <THead cols={[{ label: 'Sede' }, { label: 'Documentos', right: true }, { label: 'Monto', right: true }]} />
        <tbody>
          {filas.map((f, i) => (
            <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
              <td className="py-2 px-3 num text-[12.5px] text-slate-600 dark:text-slate-300">{f.sedeId || '—'}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtNum(f.documentos, 0)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium private-mask">{enMoneda(f.monto)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TCard>
  )
}

function ventasExport(tabla, v, ccy, anio, mes) {
  const per = `${anio}-${String(mes).padStart(2, '0')}`
  if (tabla === 'clientes') {
    return {
      nombre: `ventas-por-cliente-${per}`,
      headers: ['Cliente', 'Documentos', 'Monto'],
      rows: (v.porCliente || []).map((f) => [f.clienteNombre, f.documentos, f.monto]),
    }
  }
  if (tabla === 'sedes') {
    return {
      nombre: `ventas-por-sede-${per}`,
      headers: ['Sede', 'Documentos', 'Monto'],
      rows: (v.porSede || []).map((f) => [f.sedeId, f.documentos, f.monto]),
    }
  }
  return {
    nombre: `ventas-por-producto-${per}`,
    headers: ['SKU', 'Producto', 'Cantidad', 'Monto'],
    rows: (v.porProducto || []).map((f) => [f.sku, f.nombre, f.cantidad, f.monto]),
  }
}

/* ===================== 3 · Inventario ===================== */

function ReporteInventario() {
  const { ui } = useUI()
  const ccy = ui.ccy
  const enMoneda = (val) => fmtCurrency(val, ccy)
  const [tabla, setTabla] = useState('valor')
  const { data, error, loading, reload } = useReporte(() => api.repInventario(), [])

  const inv = data || {}
  const exportInfo = tabla === 'valor'
    ? { nombre: 'inventario-valorizacion', headers: ['SKU', 'Producto', 'Cantidad', 'Costo promedio', 'Valor'], rows: (inv.topPorValor || []).map((f) => [f.sku, f.nombre, f.cantidad, f.costoPromedio, f.valor]) }
    : { nombre: 'inventario-rotacion', headers: ['SKU', 'Producto', 'Unidades vendidas (30d)'], rows: (inv.rotacion || []).map((f) => [f.sku, f.nombre, f.unidadesVendidas]) }

  return (
    <div>
      <EstadoReporte loading={loading} error={error} onRetry={reload}>
        {data ? (
          <div className="space-y-5">
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
              <Stat label="Valorización total" value={enMoneda(inv.valorizacionTotal)} icon={<Icon.Boxes size={14} />} accent />
              <Stat label="Productos" value={fmtNum(inv.numeroProductos, 0)} icon={<Icon.Package size={14} />} />
              <Stat label="Bajo mínimo" value={fmtNum(inv.bajoMinimo, 0)} icon={<Icon.CircleAlert size={14} />} sub="stock por reponer" />
              <Stat label="Agotados" value={fmtNum(inv.agotados, 0)} icon={<Icon.CircleAlert size={14} />} sub="en cero o menos" />
            </div>

            <div>
              <div className="flex items-center justify-between gap-3 mb-3 flex-wrap">
                <Segmented value={tabla} onChange={setTabla} options={[
                  { value: 'valor', label: 'Top por valor' },
                  { value: 'rotacion', label: 'Rotación (30d)' },
                ]} />
                <BotonExportar nombre={exportInfo.nombre} headers={exportInfo.headers} rows={exportInfo.rows} />
              </div>
              {tabla === 'valor' ? <TablaValor filas={inv.topPorValor || []} enMoneda={enMoneda} /> : <TablaRotacion filas={inv.rotacion || []} />}
            </div>
          </div>
        ) : null}
      </EstadoReporte>
    </div>
  )
}

function TablaValor({ filas, enMoneda }) {
  if (!filas.length) return <Empty icon={<Icon.Boxes size={22} />} title="Sin inventario valorizado" body="Aún no hay productos con existencia y costo." />
  return (
    <TCard>
      <table className="w-full text-sm">
        <THead cols={[{ label: 'SKU' }, { label: 'Producto' }, { label: 'Cantidad', right: true }, { label: 'Costo promedio', right: true }, { label: 'Valor', right: true }]} />
        <tbody>
          {filas.map((f, i) => (
            <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
              <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.sku}</td>
              <td className="py-2 px-3 text-[13px] max-w-[280px] truncate">{f.nombre || '—'}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtNum(f.cantidad, 2)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500 private-mask">{enMoneda(f.costoPromedio)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium private-mask">{enMoneda(f.valor)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TCard>
  )
}

function TablaRotacion({ filas }) {
  if (!filas.length) return <Empty icon={<Icon.Repeat size={22} />} title="Sin rotación reciente" body="No hubo salidas de inventario en los últimos 30 días." />
  const max = Math.max(...filas.map((f) => f.unidadesVendidas), 1)
  return (
    <TCard>
      <table className="w-full text-sm">
        <THead cols={[{ label: 'SKU' }, { label: 'Producto' }, { label: 'Unidades vendidas (30d)', right: true }]} />
        <tbody>
          {filas.map((f, i) => (
            <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
              <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.sku}</td>
              <td className="py-2 px-3 text-[13px]">
                <div className="max-w-[280px] truncate">{f.nombre || '—'}</div>
                <div className="h-1.5 mt-1 rounded-full bg-slate-100 dark:bg-slate-800 overflow-hidden max-w-[280px]">
                  <div className="h-full rounded-full" style={{ width: `${Math.max(2, (f.unidadesVendidas / max) * 100)}%`, background: SERIE[1] }} />
                </div>
              </td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium align-top">{fmtNum(f.unidadesVendidas, 2)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TCard>
  )
}

/* ===================== 4 · Compras ===================== */

function ReporteCompras() {
  const { ui } = useUI()
  const ccy = ui.ccy
  const enMoneda = (v) => fmtCurrency(v, ccy)
  const hoy = new Date()
  const [anio, setAnio] = useState(hoy.getUTCFullYear())
  const [mes, setMes] = useState(hoy.getUTCMonth() + 1)
  const [tabla, setTabla] = useState('proveedor')
  const { desde, hasta } = rangoDeMes(anio, mes)
  const { data, error, loading, reload } = useReporte(() => api.repCompras(desde, hasta), [desde, hasta])

  const c = data || {}
  const per = `${anio}-${String(mes).padStart(2, '0')}`
  const exportInfo = tabla === 'proveedor'
    ? { nombre: `compras-por-proveedor-${per}`, headers: ['Proveedor', 'RIF', 'Órdenes', 'Total'], rows: (c.porProveedor || []).map((f) => [f.nombre, f.rif, f.ordenes, f.total]) }
    : { nombre: `compras-por-estado-${per}`, headers: ['Estado', 'Cantidad'], rows: (c.porEstado || []).map((f) => [f.estado, f.cantidad]) }

  return (
    <div>
      <div className="flex items-center gap-3 flex-wrap mb-3">
        <PeriodoSelector anio={anio} mes={mes} setAnio={setAnio} setMes={setMes} />
        <div className="ml-auto">
          <BotonExportar nombre={exportInfo.nombre} headers={exportInfo.headers} rows={exportInfo.rows} disabled={loading || error} />
        </div>
      </div>
      <EtiquetaRango anio={anio} mes={mes} />

      <EstadoReporte loading={loading} error={error} onRetry={reload}>
        {data ? (
          <div className="space-y-5">
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-3">
              <Stat label="Órdenes abiertas" value={fmtNum(c.ordenesAbiertas, 0)} icon={<Icon.Cart size={14} />} sub="confirmadas o parciales" accent />
              <Stat label="Pendiente por recibir" value={fmtNum(c.pendienteRecibirUnidades, 2)} icon={<Icon.Package size={14} />} sub="unidades" />
              <Stat label="Monto por recibir" value={enMoneda(c.pendienteRecibirMonto)} icon={<Icon.Wallet size={14} />} />
            </div>

            <div>
              <div className="flex items-center justify-between gap-3 mb-3 flex-wrap">
                <Segmented value={tabla} onChange={setTabla} options={[
                  { value: 'proveedor', label: 'Por proveedor' },
                  { value: 'estado', label: 'Por estado' },
                ]} />
              </div>
              {tabla === 'proveedor' ? <TablaProveedores filas={c.porProveedor || []} enMoneda={enMoneda} /> : <TablaEstados filas={c.porEstado || []} />}
            </div>
          </div>
        ) : null}
      </EstadoReporte>
    </div>
  )
}

function TablaProveedores({ filas, enMoneda }) {
  if (!filas.length) return <Empty icon={<Icon.Truck size={22} />} title="Sin órdenes en el período" body="No hubo órdenes de compra creadas en este rango." />
  return (
    <TCard>
      <table className="w-full text-sm">
        <THead cols={[{ label: 'Proveedor' }, { label: 'RIF' }, { label: 'Órdenes', right: true }, { label: 'Total', right: true }]} />
        <tbody>
          {filas.map((f, i) => (
            <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
              <td className="py-2 px-3 text-[13px] max-w-[300px] truncate">{f.nombre || '—'}</td>
              <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.rif || '—'}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtNum(f.ordenes, 0)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium private-mask">{enMoneda(f.total)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TCard>
  )
}

const ESTADO_OC_LABEL = {
  borrador: 'Borrador', confirmada: 'Confirmada', recibida_parcial: 'Recibida parcial',
  recibida: 'Recibida', cancelada: 'Cancelada',
}
const ESTADO_OC_COLOR = {
  borrador: 'slate', confirmada: 'blue', recibida_parcial: 'amber', recibida: 'emerald', cancelada: 'red',
}

function TablaEstados({ filas }) {
  if (!filas.length) return <Empty icon={<Icon.ClipboardList size={22} />} title="Sin órdenes en el período" body="No hubo órdenes de compra creadas en este rango." />
  return (
    <TCard>
      <table className="w-full text-sm">
        <THead cols={[{ label: 'Estado' }, { label: 'Cantidad', right: true }]} />
        <tbody>
          {filas.map((f, i) => (
            <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
              <td className="py-2 px-3"><Badge size="sm" color={ESTADO_OC_COLOR[f.estado] || 'slate'}>{ESTADO_OC_LABEL[f.estado] || f.estado}</Badge></td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium">{fmtNum(f.cantidad, 0)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TCard>
  )
}

/* ===================== 5 · Cobranza ===================== */

// Color de cada tramo de antigüedad: más viejo = más severo.
const AGING_COLOR = { '0-30': '#1D3477', '31-60': '#2A4A8F', '61-90': '#92600A', '90+': '#B3362C' }

function ReporteCobranza() {
  const { ui } = useUI()
  const ccy = ui.ccy
  const enMoneda = (v) => fmtCurrency(v, ccy)
  const { data, error, loading, reload } = useReporte(() => api.repCobranza(), [])

  const c = data || {}
  const aging = c.aging || []
  const exportInfo = {
    nombre: 'cobranza-antiguedad',
    headers: ['Tramo (días)', 'Documentos', 'Monto'],
    rows: aging.map((t) => [t.tramo, t.documentos, t.monto]),
  }

  return (
    <div>
      <div className="flex items-center justify-end mb-3">
        <BotonExportar nombre={exportInfo.nombre} headers={exportInfo.headers} rows={exportInfo.rows} disabled={loading || error} />
      </div>

      <EstadoReporte loading={loading} error={error} onRetry={reload}>
        {data ? (
          <div className="space-y-5">
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-3">
              <Stat label="Total por cobrar" value={enMoneda(c.totalPorCobrar)} icon={<Icon.Wallet size={14} />} accent />
              <Stat label="Vencido" value={enMoneda(c.totalVencido)} icon={<Icon.CircleAlert size={14} />}
                sub={c.totalPorCobrar ? `${Math.round((c.totalVencido / c.totalPorCobrar) * 100)}% de la cartera` : undefined} />
              <Stat label="IGTF del período" value={enMoneda(c.igtf)} icon={<Icon.Banknote size={14} />} />
            </div>

            <Card>
              <div className="font-display font-semibold text-[16px] mb-1">Antigüedad de la cartera</div>
              <div className="text-[12.5px] text-slate-500 mb-4">Saldo por cobrar según los días transcurridos desde la fecha del documento.</div>
              <AgingChart aging={aging} enMoneda={enMoneda} />
            </Card>

            <TablaAging aging={aging} enMoneda={enMoneda} />
          </div>
        ) : null}
      </EstadoReporte>
    </div>
  )
}

// Gráfico de antigüedad: una barra vertical etiquetada por tramo (el BarChart
// genérico rotula el eje X con "hoy", que no aplica a tramos, así que aquí va uno
// propio con la etiqueta de cada tramo bajo su barra).
function AgingChart({ aging, enMoneda }) {
  const hayDatos = aging.some((t) => t.monto > 0)
  if (!hayDatos) {
    return (
      <Empty framed={false} icon={<Icon.CircleCheck size={22} />} title="Cartera al día"
        body="No hay saldos por cobrar que envejezcan." />
    )
  }
  const max = Math.max(...aging.map((t) => t.monto), 1)
  return (
    <div>
      <div className="flex items-end gap-4" style={{ height: 200 }}>
        {aging.map((t) => {
          const h = t.monto > 0 ? Math.max(2, (t.monto / max) * 100) : 0
          return (
            <div key={t.tramo} className="flex-1 min-w-0 h-full flex flex-col justify-end items-center" title={`${t.tramo} días · ${enMoneda(t.monto)}`}>
              <div className="num text-[11.5px] text-slate-500 mb-1 private-mask">{t.monto > 0 ? enMoneda(t.monto) : ''}</div>
              <div className="w-full max-w-[72px] rounded-t-[4px] transition-[height] duration-300"
                style={{ height: `${h}%`, minHeight: t.monto > 0 ? 2 : 0, background: AGING_COLOR[t.tramo] || SERIE[0] }} />
            </div>
          )
        })}
      </div>
      <div className="flex gap-4 mt-2">
        {aging.map((t) => (
          <div key={t.tramo} className="flex-1 min-w-0 text-center">
            <div className="text-[12px] font-medium text-slate-600 dark:text-slate-300">{t.tramo}</div>
            <div className="text-[11px] text-slate-400">días</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function TablaAging({ aging, enMoneda }) {
  const total = aging.reduce((a, b) => a + b.monto, 0)
  const totalDocs = aging.reduce((a, b) => a + (b.documentos || 0), 0)
  return (
    <TCard>
      <table className="w-full text-sm">
        <THead cols={[{ label: 'Tramo (días)' }, { label: 'Documentos', right: true }, { label: 'Monto', right: true }]} />
        <tbody>
          {aging.map((t) => (
            <tr key={t.tramo} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
              <td className="py-2 px-3">
                <span className="inline-flex items-center gap-2 text-[13px]">
                  <span className="w-2.5 h-2.5 rounded-sm" style={{ background: AGING_COLOR[t.tramo] || SERIE[0] }} />
                  {t.tramo}
                </span>
              </td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtNum(t.documentos, 0)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium private-mask">{enMoneda(t.monto)}</td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          <tr className="bg-slate-100 dark:bg-slate-800 border-t-2 border-slate-300 dark:border-slate-700 font-semibold text-[12.5px]">
            <td className="py-2.5 px-3">Total cartera</td>
            <td className="py-2.5 px-3 text-right num">{fmtNum(totalDocs, 0)}</td>
            <td className="py-2.5 px-3 text-right num private-mask">{enMoneda(total)}</td>
          </tr>
        </tfoot>
      </table>
    </TCard>
  )
}
