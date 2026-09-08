import { useState, useEffect, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Segmented, Select, Empty, TableSkeleton } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useUI } from '../context/UIContext.jsx'
import { useData } from '../context/DataContext.jsx'
import { api } from '../lib/api.js'

// Libros fiscales (Libro de Ventas / Libro de Compras): reportes DERIVADOS del
// ledger, de SOLO LECTURA. Por contribuyente (empresa/RIF, consolidando TODAS
// las sedes) y por período mensual. La interfaz nunca inventa un total: pinta lo
// que el servidor plegó del ledger de documentos y de las órdenes de compra.
//
// Solo entran los roles con acceso de consulta fiscal: la Contadora DEBE poder
// leerlos (acceso del regulador, Providencia 000121). El backend es la autoridad
// real; aquí solo evitamos una llamada que devolvería 403.
const ROLES_LIBROS = ['dueno', 'desarrollador', 'contadora']

const MESES = [
  'Enero', 'Febrero', 'Marzo', 'Abril', 'Mayo', 'Junio',
  'Julio', 'Agosto', 'Septiembre', 'Octubre', 'Noviembre', 'Diciembre',
]

// Etiqueta legible del tipo de documento del Libro de Ventas.
const TIPO_LABEL = { factura: 'Factura', nota_credito: 'Nota de crédito', anulacion: 'Anulación' }
const esNegativo = (tipo) => tipo === 'nota_credito' || tipo === 'anulacion'

export function LibrosFiscales({ libroInicial = 'ventas' }) {
  const { ui } = useUI()
  const { db } = useData()
  const ccy = ui.ccy
  const puedeVer = ROLES_LIBROS.includes(ui.rol)

  const hoy = new Date()
  const [libro, setLibro] = useState(libroInicial) // 'ventas' | 'compras'
  const [anio, setAnio] = useState(hoy.getUTCFullYear())
  const [mes, setMes] = useState(hoy.getUTCMonth() + 1) // 1..12
  const [data, setData] = useState(undefined) // undefined = cargando
  const [error, setError] = useState(null)

  const cargar = useCallback(async () => {
    if (!puedeVer) return
    setError(null)
    setData(undefined)
    try {
      const res = libro === 'ventas' ? await api.libroVentas(anio, mes) : await api.libroCompras(anio, mes)
      setData(res)
    } catch (e) {
      setData(null)
      setError(e)
    }
  }, [libro, anio, mes, puedeVer])

  useEffect(() => { cargar() }, [cargar])

  // Años disponibles en el selector: del actual hacia atrás unos cuantos.
  const anios = useMemo(() => {
    const y = hoy.getUTCFullYear()
    return [y, y - 1, y - 2, y - 3, y - 4]
  }, [hoy])

  if (!puedeVer) {
    return <Empty icon={<Icon.Lock size={22} />} title="Sin acceso a los libros fiscales"
      body="Los libros de ventas y compras son de consulta de la Dueña, el Desarrollador y la Contadora." />
  }

  const filas = data?.filas || []
  const totales = data?.totales || {}

  const exportar = () => exportarCSV(libro, anio, mes, filas, totales, TIPO_LABEL)
  const exportarTxt = () => exportarSeniat(libro, anio, mes, filas, totales, db.EMPRESA)

  return (
    <div>
      {/* Controles: tipo de libro + período + exportar */}
      <div className="flex items-center gap-3 flex-wrap mb-3">
        <Segmented
          options={[{ value: 'ventas', label: 'Ventas' }, { value: 'compras', label: 'Compras' }]}
          value={libro} onChange={setLibro} />
        <div className="flex items-center gap-2">
          <Select value={mes} onChange={(e) => setMes(Number(e.target.value))} className="w-36">
            {MESES.map((m, i) => <option key={i} value={i + 1}>{m}</option>)}
          </Select>
          <Select value={anio} onChange={(e) => setAnio(Number(e.target.value))} className="w-24">
            {anios.map((y) => <option key={y} value={y}>{y}</option>)}
          </Select>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Button variant="ghost" icon={<Icon.Download size={15} />} disabled={!filas.length} onClick={exportar}
            title={filas.length ? 'Exportar a CSV' : 'No hay filas que exportar'}>
            Exportar CSV
          </Button>
          <Button variant="secondary" icon={<Icon.Download size={15} />} disabled={!filas.length} onClick={exportarTxt}
            title={filas.length ? 'Exportar en formato de libro SENIAT (TXT)' : 'No hay filas que exportar'}>
            Exportar SENIAT (TXT)
          </Button>
        </div>
      </div>

      <div className="mb-2 text-[12.5px] text-slate-500">
        {libro === 'ventas' ? 'Libro de Ventas' : 'Libro de Compras'} ·
        <span className="font-medium text-slate-700 dark:text-slate-200"> {MESES[mes - 1]} {anio}</span> ·
        <span className="text-slate-400"> todas las sedes del contribuyente</span>
      </div>

      {/* Nota del Libro de Compras: base best-effort en órdenes recibidas. */}
      {libro === 'compras' ? (
        <div className="mb-3 flex items-start gap-2 text-[11.5px] text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/20 rounded-lg px-3 py-2">
          <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" />
          <span>Basado en órdenes de compra recibidas; la factura fiscal del proveedor (nº de control, retención) llega con el módulo de facturas de compra.</span>
        </div>
      ) : null}

      {error ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar el libro"
          body={String(error.message || error)}
          cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          {data === undefined ? (
            <div className="p-4"><TableSkeleton rows={6} cols={libro === 'ventas' ? 8 : 6} /></div>
          ) : filas.length === 0 ? (
            <Empty framed={false} icon={<Icon.Book size={22} />}
              title="Sin documentos en este período"
              body={`No hay ${libro === 'ventas' ? 'documentos fiscales' : 'órdenes de compra recibidas'} en ${MESES[mes - 1]} ${anio}.`} />
          ) : libro === 'ventas' ? (
            <TablaVentas filas={filas} totales={totales} ccy={ccy} />
          ) : (
            <TablaCompras filas={filas} totales={totales} ccy={ccy} />
          )}
        </div>
      )}

      {filas.length ? (
        <div className="mt-2 text-[11.5px] text-slate-400">
          {fmtNum(filas.length, 0)} documento(s) · El TXT usa el formato estándar de columnas del libro {libro === 'ventas' ? 'de ventas' : 'de compras'} del SENIAT (delimitado por «|», con RIF del contribuyente y totales). El layout exacto del TXT/XML oficial es configuración versionada por providencia; el CSV sigue disponible para hoja de cálculo.
        </div>
      ) : null}
    </div>
  )
}

// --- Libro de Ventas ---
function TablaVentas({ filas, totales, ccy }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm tbl-sticky">
        <thead>
          <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
            <th className="py-2.5 px-3 font-medium">Fecha</th>
            <th className="py-2.5 px-3 font-medium">Tipo</th>
            <th className="py-2.5 px-3 font-medium">Documento</th>
            <th className="py-2.5 px-3 font-medium">RIF / Cédula</th>
            <th className="py-2.5 px-3 font-medium">Cliente</th>
            <th className="py-2.5 px-3 font-medium text-right">Base exenta</th>
            <th className="py-2.5 px-3 font-medium text-right">Base imponible</th>
            <th className="py-2.5 px-3 font-medium text-center">Alíc.</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA débito</th>
            <th className="py-2.5 px-3 font-medium text-right">IGTF</th>
            <th className="py-2.5 px-3 font-medium text-right">Total</th>
          </tr>
        </thead>
        <tbody>
          {filas.map((f, i) => {
            const neg = esNegativo(f.tipo)
            return (
              <tr key={i} className={`border-b border-slate-100 dark:border-slate-800/70 ${neg ? 'bg-amber-50/40 dark:bg-amber-900/10' : ''}`}>
                <td className="py-2 px-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(f.fecha)}</td>
                <td className="py-2 px-3 text-[12.5px]">
                  {neg ? <Badge color="amber" size="sm">{TIPO_LABEL[f.tipo] || f.tipo}</Badge>
                    : <span className="text-slate-500">{TIPO_LABEL[f.tipo] || f.tipo}</span>}
                </td>
                <td className="py-2 px-3 num text-[12.5px] font-medium whitespace-nowrap">{f.numeroCompleto}</td>
                <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.clienteDocumento || '—'}</td>
                <td className="py-2 px-3 text-[12.5px] text-slate-600 dark:text-slate-300 max-w-[180px] truncate">{f.clienteNombre || '—'}</td>
                <td className={`py-2 px-3 text-right num text-[12.5px] ${neg ? 'text-amber-700 dark:text-amber-300' : 'text-slate-500'}`}>{fmtCurrency(f.baseExenta, ccy)}</td>
                <td className={`py-2 px-3 text-right num text-[12.5px] ${neg ? 'text-amber-700 dark:text-amber-300' : 'text-slate-500'}`}>{fmtCurrency(f.baseImponible, ccy)}</td>
                <td className="py-2 px-3 text-center num text-[12px] text-slate-400">{fmtNum((f.alicuota || 0) * 100, 0)}%</td>
                <td className={`py-2 px-3 text-right num text-[12.5px] ${neg ? 'text-amber-700 dark:text-amber-300' : 'text-slate-500'}`}>{fmtCurrency(f.ivaDebito, ccy)}</td>
                <td className={`py-2 px-3 text-right num text-[12.5px] ${neg ? 'text-amber-700 dark:text-amber-300' : 'text-slate-500'}`}>{fmtCurrency(f.igtf, ccy)}</td>
                <td className={`py-2 px-3 text-right num text-[12.5px] font-medium ${neg ? 'text-amber-700 dark:text-amber-300' : ''}`}>{fmtCurrency(f.total, ccy)}</td>
              </tr>
            )
          })}
        </tbody>
        <tfoot>
          <tr className="sticky bottom-0 bg-slate-100 dark:bg-slate-800 border-t-2 border-slate-300 dark:border-slate-700 font-semibold text-[12.5px]">
            <td className="py-2.5 px-3" colSpan={5}>Totales del período</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseExenta, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseImponible, ccy)}</td>
            <td className="py-2.5 px-3"></td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaDebito, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.igtf, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.total, ccy)}</td>
          </tr>
        </tfoot>
      </table>
    </div>
  )
}

// --- Libro de Compras ---
function TablaCompras({ filas, totales, ccy }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm tbl-sticky">
        <thead>
          <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
            <th className="py-2.5 px-3 font-medium">Fecha</th>
            <th className="py-2.5 px-3 font-medium">RIF proveedor</th>
            <th className="py-2.5 px-3 font-medium">Proveedor</th>
            <th className="py-2.5 px-3 font-medium">Orden</th>
            <th className="py-2.5 px-3 font-medium text-right">Base exenta</th>
            <th className="py-2.5 px-3 font-medium text-right">Base imponible</th>
            <th className="py-2.5 px-3 font-medium text-center">Alíc.</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA crédito</th>
            <th className="py-2.5 px-3 font-medium text-right">Total</th>
          </tr>
        </thead>
        <tbody>
          {filas.map((f, i) => (
            <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70">
              <td className="py-2 px-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(f.fecha)}</td>
              <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.proveedorRif || '—'}</td>
              <td className="py-2 px-3 text-[12.5px] text-slate-600 dark:text-slate-300 max-w-[200px] truncate">{f.proveedorNombre || '—'}</td>
              <td className="py-2 px-3 num text-[12.5px] font-medium whitespace-nowrap">{f.numeroCompleto}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtCurrency(f.baseExenta, ccy)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtCurrency(f.baseImponible, ccy)}</td>
              <td className="py-2 px-3 text-center num text-[12px] text-slate-400">{fmtNum((f.alicuota || 0) * 100, 0)}%</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtCurrency(f.ivaCreditoFiscal, ccy)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium">{fmtCurrency(f.total, ccy)}</td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          <tr className="sticky bottom-0 bg-slate-100 dark:bg-slate-800 border-t-2 border-slate-300 dark:border-slate-700 font-semibold text-[12.5px]">
            <td className="py-2.5 px-3" colSpan={4}>Totales del período</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseExenta, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseImponible, ccy)}</td>
            <td className="py-2.5 px-3"></td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaCreditoFiscal, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.total, ccy)}</td>
          </tr>
        </tfoot>
      </table>
    </div>
  )
}

// exportarCSV genera un CSV client-side a partir de las filas ya cargadas (con
// encabezados y la fila de totales) y dispara la descarga con un Blob. El layout
// TXT/XML oficial del SENIAT depende de la providencia vigente (configuración
// versionada) y queda como siguiente paso; el CSV con todas las columnas es la
// base sobre la que se construirá.
function exportarCSV(libro, anio, mes, filas, totales, tipoLabel) {
  const per = `${anio}-${String(mes).padStart(2, '0')}`
  let headers, rows, totalRow
  const n = (v) => (v == null ? '0' : String(v)) // numérico crudo, sin formato de moneda
  if (libro === 'ventas') {
    headers = ['Fecha', 'Tipo', 'Documento', 'RIF/Cedula', 'Cliente', 'Base exenta', 'Base imponible', 'Alicuota', 'IVA debito', 'IGTF', 'Total']
    rows = filas.map((f) => [
      f.fecha, tipoLabel[f.tipo] || f.tipo, f.numeroCompleto, f.clienteDocumento, f.clienteNombre,
      n(f.baseExenta), n(f.baseImponible), n(f.alicuota), n(f.ivaDebito), n(f.igtf), n(f.total),
    ])
    totalRow = ['TOTALES', '', '', '', '', n(totales.baseExenta), n(totales.baseImponible), '', n(totales.ivaDebito), n(totales.igtf), n(totales.total)]
  } else {
    headers = ['Fecha', 'RIF proveedor', 'Proveedor', 'Orden', 'Base exenta', 'Base imponible', 'Alicuota', 'IVA credito fiscal', 'Total']
    rows = filas.map((f) => [
      f.fecha, f.proveedorRif, f.proveedorNombre, f.numeroCompleto,
      n(f.baseExenta), n(f.baseImponible), n(f.alicuota), n(f.ivaCreditoFiscal), n(f.total),
    ])
    totalRow = ['TOTALES', '', '', '', n(totales.baseExenta), n(totales.baseImponible), '', n(totales.ivaCreditoFiscal), n(totales.total)]
  }
  const all = [headers, ...rows, totalRow]
  const csv = all.map((r) => r.map(csvCell).join(',')).join('\r\n')
  // BOM para que Excel abra los acentos correctamente.
  const blob = new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `libro-${libro}-${per}.csv`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

// --- Export SENIAT (libro de ventas / compras en TXT) ---
//
// Genera un archivo de texto delimitado por pipe «|» con el orden de columnas
// del formato estándar del Libro de Ventas o del Libro de Compras del SENIAT, a
// partir de las MISMAS filas ya plegadas del período. Es un formato de archivo
// (no de pantalla): montos con punto decimal y 2 decimales, SIN separador de
// miles; fechas DD/MM/AAAA. El layout exacto del TXT/XML oficial depende de la
// providencia vigente (configuración versionada) — esto es la disposición de
// columnas estándar del libro sobre la que se construirá.
//
// Tipo de documento (Libro de Ventas): 01 = factura, 02 = nota de débito,
// 03 = nota de crédito. La anulación es una reversión (comportamiento de nota de
// crédito) → 03. Los campos aún no capturados (p. ej. nº de control) van vacíos
// pero conservan su posición/columna.
const TIPO_SENIAT = { factura: '01', nota_debito: '02', nota_credito: '03', anulacion: '03' }

// seniatFecha: ISO → DD/MM/AAAA en UTC (evita corrimientos por zona horaria).
function seniatFecha(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d)) return String(iso)
  const dd = String(d.getUTCDate()).padStart(2, '0')
  const mm = String(d.getUTCMonth() + 1).padStart(2, '0')
  return `${dd}/${mm}/${d.getUTCFullYear()}`
}
// seniatMonto: número crudo → "0.00" (punto decimal, sin separador de miles).
const seniatMonto = (v) => (Number(v) || 0).toFixed(2)
// seniatAlic: fracción (0.16) → "16.00" (%).
const seniatAlic = (v) => ((Number(v) || 0) * 100).toFixed(2)
// seniatTexto: sanea un campo de texto para el delimitador «|».
const seniatTexto = (v) => (v == null ? '' : String(v).replace(/[|\r\n]+/g, ' ').trim())

function exportarSeniat(libro, anio, mes, filas, totales, empresa) {
  const per = `${anio}-${String(mes).padStart(2, '0')}`
  const perLabel = `${String(mes).padStart(2, '0')}/${anio}`
  const rif = seniatTexto(empresa?.rif) || 'SIN-RIF'
  const razon = seniatTexto(empresa?.razonSocial || empresa?.nombre) || 'CONTRIBUYENTE'

  const lines = []
  let cabecera, columnas, cuerpo, totalRow
  if (libro === 'ventas') {
    cabecera = ['LIBRO DE VENTAS', rif, razon, perLabel]
    columnas = ['Fecha', 'TipoDoc', 'NumeroDocumento', 'NumeroControl', 'RIF_Cedula', 'RazonSocial',
      'TotalConIVA', 'VentasExentas', 'BaseImponible', 'Alicuota', 'IVADebito', 'IGTF']
    cuerpo = filas.map((f) => [
      seniatFecha(f.fecha),
      TIPO_SENIAT[f.tipo] || '',
      seniatTexto(f.numeroCompleto),
      seniatTexto(f.numeroControl), // aún no se captura → vacío, columna conservada
      seniatTexto(f.clienteDocumento),
      seniatTexto(f.clienteNombre),
      seniatMonto(f.total),
      seniatMonto(f.baseExenta),
      seniatMonto(f.baseImponible),
      seniatAlic(f.alicuota),
      seniatMonto(f.ivaDebito),
      seniatMonto(f.igtf),
    ])
    totalRow = ['TOTALES', '', '', '', '', '',
      seniatMonto(totales.total), seniatMonto(totales.baseExenta), seniatMonto(totales.baseImponible),
      '', seniatMonto(totales.ivaDebito), seniatMonto(totales.igtf)]
  } else {
    cabecera = ['LIBRO DE COMPRAS', rif, razon, perLabel]
    columnas = ['Fecha', 'RIF_Proveedor', 'RazonSocial', 'NumeroFactura', 'NumeroControl',
      'TotalCompras', 'ComprasExentas', 'BaseImponible', 'Alicuota', 'IVACreditoFiscal', 'IVARetenido']
    cuerpo = filas.map((f) => [
      seniatFecha(f.fecha),
      seniatTexto(f.proveedorRif),
      seniatTexto(f.proveedorNombre),
      seniatTexto(f.numeroCompleto),
      seniatTexto(f.numeroControl), // aún no se captura → vacío, columna conservada
      seniatMonto(f.total),
      seniatMonto(f.baseExenta),
      seniatMonto(f.baseImponible),
      seniatAlic(f.alicuota),
      seniatMonto(f.ivaCreditoFiscal),
      f.ivaRetenido == null ? '' : seniatMonto(f.ivaRetenido), // retención → vacío si no hay
    ])
    totalRow = ['TOTALES', '', '', '', '',
      seniatMonto(totales.total), seniatMonto(totales.baseExenta), seniatMonto(totales.baseImponible),
      '', seniatMonto(totales.ivaCreditoFiscal), '']
  }

  lines.push(cabecera.join('|'))
  lines.push(columnas.join('|'))
  for (const r of cuerpo) lines.push(r.join('|'))
  lines.push(totalRow.join('|'))
  const txt = lines.join('\r\n') + '\r\n'

  // BOM UTF-8 para que los acentos (razón social) se lean bien.
  const blob = new Blob(['﻿' + txt], { type: 'text/plain;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `libro-${libro}-${per}.txt`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

// csvCell escapa un valor para CSV: entrecomilla si contiene coma, comilla o
// salto de línea, y duplica las comillas internas.
function csvCell(v) {
  const s = v == null ? '' : String(v)
  if (/[",\r\n]/.test(s)) return '"' + s.replace(/"/g, '""') + '"'
  return s
}
