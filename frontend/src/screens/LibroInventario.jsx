import { useState, useEffect, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Input, Empty, TableSkeleton } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'

// Libro de Inventario: existencia VALORIZADA a la fecha (cantidad, costo promedio
// y valor por producto, con el total general), consolidando TODAS las sedes del
// contribuyente. Es un reporte DERIVADO del ledger append-only (misma proyección
// de existencias/costo promedio que Inventario), de SOLO LECTURA: la interfaz no
// inventa ningún total; pinta lo que el servidor plegó del ledger.
//
// Acceso de consulta fiscal: Dueña / Desarrollador / Contadora (como los libros).
const ROLES_VER = ['dueno', 'desarrollador', 'contadora']

export function LibroInventario() {
  const { ui } = useUI()
  const ccy = ui.ccy
  const puedeVer = ROLES_VER.includes(ui.rol)

  const [q, setQ] = useState('')
  const [data, setData] = useState(undefined) // undefined = cargando
  const [error, setError] = useState(null)

  const cargar = useCallback(async () => {
    if (!puedeVer) return
    setError(null)
    setData(undefined)
    try {
      setData(await api.libroInventario())
    } catch (e) {
      setData(null)
      setError(e)
    }
  }, [puedeVer])

  useEffect(() => { cargar() }, [cargar])

  const filas = data?.filas || []
  const totales = data?.totales || {}

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    if (!term) return filas
    return filas.filter((f) =>
      (f.sku || '').toLowerCase().includes(term) || (f.nombre || '').toLowerCase().includes(term))
  }, [filas, q])

  if (!puedeVer) {
    return <Empty icon={<Icon.Lock size={22} />} title="Sin acceso al libro de inventario"
      body="El libro de inventario es de consulta de la Dueña, el Desarrollador y la Contadora." />
  }

  const exportar = () => exportarCSV(data?.fecha, filas, totales)

  return (
    <div>
      <div className="flex items-center gap-3 flex-wrap mb-3">
        <Input className="w-64" icon={<Icon.Search size={15} />}
          placeholder="Buscar por SKU o nombre…" value={q} onChange={(e) => setQ(e.target.value)} />
        <div className="ml-auto flex items-center gap-2">
          <Button variant="ghost" icon={<Icon.Download size={15} />} disabled={!filas.length} onClick={exportar}
            title={filas.length ? 'Exportar a CSV' : 'No hay filas que exportar'}>
            Exportar CSV
          </Button>
        </div>
      </div>

      <div className="mb-2 text-[12.5px] text-slate-500">
        Libro de Inventario ·
        <span className="font-medium text-slate-700 dark:text-slate-200"> existencia valorizada al {fmtDate(data?.fecha) || 'día'}</span> ·
        <span className="text-slate-400"> todas las sedes del contribuyente</span>
      </div>

      {error ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar el libro de inventario"
          body={String(error.message || error)}
          cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          {data === undefined ? (
            <div className="p-4"><TableSkeleton rows={6} cols={5} /></div>
          ) : rows.length === 0 ? (
            <Empty framed={false} icon={<Icon.Boxes size={22} />}
              title={q ? 'Sin resultados' : 'Sin existencias que declarar'}
              body={q ? 'Prueba con otro término.'
                : 'No hay productos con existencia. El inventario se pliega del ledger de movimientos (entradas de compra, ventas, ajustes y transferencias).'} />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm tbl-sticky">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                    <th className="py-2.5 px-3 font-medium">SKU</th>
                    <th className="py-2.5 px-3 font-medium">Producto</th>
                    <th className="py-2.5 px-3 font-medium text-right">Existencia</th>
                    <th className="py-2.5 px-3 font-medium text-right">Costo promedio</th>
                    <th className="py-2.5 px-3 font-medium text-right">Valor</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((f, i) => (
                    <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70">
                      <td className="py-2 px-3 num text-[12.5px] font-medium whitespace-nowrap">{f.sku}</td>
                      <td className="py-2 px-3 text-[12.5px] text-slate-600 dark:text-slate-300 max-w-[280px] truncate">
                        {f.nombre}
                        {f.unidadBase ? <span className="text-slate-400"> · {f.unidadBase}</span> : null}
                      </td>
                      <td className="py-2 px-3 text-right num text-[12.5px]">{fmtNum(f.cantidad)}</td>
                      <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500 private-mask">{fmtCurrency(f.costoPromedio, ccy)}</td>
                      <td className="py-2 px-3 text-right num text-[12.5px] font-medium private-mask">{fmtCurrency(f.valor, ccy)}</td>
                    </tr>
                  ))}
                </tbody>
                <tfoot>
                  <tr className="sticky bottom-0 bg-slate-100 dark:bg-slate-800 border-t-2 border-slate-300 dark:border-slate-700 font-semibold text-[12.5px]">
                    <td className="py-2.5 px-3" colSpan={4}>Valor total del inventario ({fmtNum(totales.numeroProductos || 0, 0)} productos)</td>
                    <td className="py-2.5 px-3 text-right num private-mask">{fmtCurrency(totales.valor, ccy)}</td>
                  </tr>
                </tfoot>
              </table>
            </div>
          )}
        </div>
      )}

      {filas.length ? (
        <div className="mt-2 text-[11.5px] text-slate-400">
          {fmtNum(filas.length, 0)} producto(s) con existencia · Valorización a costo promedio, derivada del ledger append-only. El layout TXT/XML oficial del Libro de Inventarios y Balances es configuración versionada por providencia; el CSV con todas las columnas queda disponible para hoja de cálculo.
        </div>
      ) : null}
    </div>
  )
}

// exportarCSV genera un CSV client-side a partir de las filas ya cargadas (con
// encabezados y la fila de total) y dispara la descarga con un Blob. El layout
// oficial del SENIAT (Libro de Inventarios y Balances) depende de la providencia
// vigente (configuración versionada) y queda como siguiente paso; el CSV es la
// base sobre la que se construirá.
function exportarCSV(fecha, filas, totales) {
  const dia = (fecha ? String(fecha).slice(0, 10) : new Date().toISOString().slice(0, 10))
  const n = (v) => (v == null ? '0' : String(v)) // numérico crudo, sin formato de moneda
  const headers = ['SKU', 'Producto', 'Unidad', 'Existencia', 'Costo promedio', 'Valor']
  const rows = filas.map((f) => [f.sku, f.nombre, f.unidadBase, n(f.cantidad), n(f.costoPromedio), n(f.valor)])
  const totalRow = ['TOTAL', '', '', '', '', n(totales.valor)]
  const all = [headers, ...rows, totalRow]
  const csv = all.map((r) => r.map(csvCell).join(',')).join('\r\n')
  // BOM para que Excel abra los acentos correctamente.
  const blob = new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `libro-inventario-${dia}.csv`
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
