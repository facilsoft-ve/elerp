import { useState, useMemo } from 'react'
import { Icon } from './Icon.jsx'
import { Button, TableSkeleton } from './primitives.jsx'
import { fmtNum } from '../lib/format.js'

/* TablaDatos — patrón reutilizable de tabla densa pero MANIPULABLE (regla UX
 * vinculante del Manual: «tablas de alta densidad… sort/filter/… elegir densidad,
 * export»). Reúne en un solo componente las tres capacidades que hoy solo tenía
 * el Catálogo por separado:
 *
 *   · Orden por columna  — clic en el encabezado (asc/desc), con la flecha y
 *     `aria-sort` para el lector de pantalla. Mismo criterio que Catalogo.jsx
 *     (numérico cuando ambos valores son números; localeCompare 'es' si no).
 *   · Densidad           — Cómodo / Compacto (Segmented de la marca), igual que
 *     el Catálogo.
 *   · Export CSV         — Blob + BOM UTF-8, idéntico al de LibrosFiscales /
 *     Reportes, para que Excel abra los acentos bien. Exporta el ORDEN visible.
 *
 * La tabla NO conoce los filtros/segmentos de cada pantalla: recibe ya filtradas
 * las `rows`. Cada columna declara cómo se ordena (`sortValue`), cómo se pinta
 * (`cell`) y qué exporta (`csv`), así el markup rico (badges, steppers, botones de
 * fila) se conserva tal cual. Las filas clicables quedan operables por teclado
 * (role=button, tabIndex, Enter/Espacio) de una sola vez para todas las pantallas.
 *
 * columnas: [{
 *   key,                       // id único de la columna
 *   header,                    // rótulo del encabezado (y del CSV)
 *   align = 'left',            // 'left' | 'right' | 'center'
 *   sortable = false,          // ¿se puede ordenar por esta columna?
 *   sortValue = (row)=>row[key], // valor comparable para ordenar
 *   cell = (row)=>…,           // JSX de la celda (por defecto el valor de csv)
 *   csv,                       // valor crudo para el CSV; `false` = no exportar
 *   stop = false,              // detiene la propagación del clic (celda de acciones)
 *   tdClassName, thClassName,  // clases extra
 * }]
 */
const ALIGN = { left: 'text-left', right: 'text-right', center: 'text-center' }
const ALIGN_FLEX = { left: '', right: 'justify-end', center: 'justify-center' }

function csvCell(v) {
  const s = v == null ? '' : String(v)
  if (/[",\r\n]/.test(s)) return '"' + s.replace(/"/g, '""') + '"'
  return s
}

function descargarCSV(nombre, headers, filas) {
  const all = [headers, ...filas]
  const csv = all.map((r) => r.map(csvCell).join(',')).join('\r\n')
  // BOM UTF-8 para que Excel abra los acentos correctamente (igual que Reportes).
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

const valorDe = (col) => col.sortValue || ((row) => row[col.key])

export function TablaDatos({
  columns,
  rows,
  rowKey = (r) => r.id,
  onRowClick,
  rowClassName,
  csvName = 'export',
  loading = false,
  empty = null,
  initialSort = null,      // { key, dir } | null
  minWidth = '',
  footerLabel,             // (n)=>string · pie con el conteo (n = filas visibles)
}) {
  const [sort, setSort] = useState(initialSort)
  const pad = 'py-2.5 pr-3'
  const clickable = !!onRowClick

  const sorted = useMemo(() => {
    if (!sort) return rows
    const col = columns.find((c) => c.key === sort.key)
    if (!col) return rows
    const val = valorDe(col)
    const dir = sort.dir
    return [...rows].sort((a, b) => {
      const av = val(a), bv = val(b)
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir
      return String(av ?? '').localeCompare(String(bv ?? ''), 'es') * dir
    })
  }, [rows, sort, columns])

  const toggleSort = (key) => setSort((s) => (s && s.key === key ? { key, dir: -s.dir } : { key, dir: 1 }))

  const exportar = () => {
    const cols = columns.filter((c) => c.csv !== false)
    const headers = cols.map((c) => c.header || '')
    const filas = sorted.map((r) => cols.map((c) => {
      const fn = typeof c.csv === 'function' ? c.csv : valorDe(c)
      return fn(r)
    }))
    descargarCSV(csvName, headers, filas)
  }

  const hayFilas = !loading && rows.length > 0

  return (
    <div>
      {hayFilas ? (
        <div className="flex items-center justify-end gap-2 mb-2">
          <Button variant="secondary" size="sm" icon={<Icon.Download size={15} />} onClick={exportar}
            title="Exportar la tabla a CSV (respeta el orden visible)">Exportar CSV</Button>
        </div>
      ) : null}

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading ? (
          <div className="p-4"><TableSkeleton rows={7} cols={columns.length} /></div>
        ) : rows.length === 0 ? (
          empty
        ) : (
          <div className="overflow-x-auto">
            <table className={`w-full text-sm tbl-sticky ${minWidth}`}>
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-500 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  {columns.map((col) => {
                    const align = col.align || 'left'
                    const activo = sort?.key === col.key
                    return (
                      <th key={col.key}
                        className={`${pad} font-medium ${ALIGN[align]} ${col.sortable ? 'cursor-pointer select-none' : ''} ${col.thClassName || ''}`}
                        onClick={col.sortable ? () => toggleSort(col.key) : undefined}
                        aria-sort={col.sortable ? (activo ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none') : undefined}>
                        <span className={`inline-flex items-center gap-1 ${ALIGN_FLEX[align]}`}>
                          {col.header}
                          {col.sortable && activo ? (sort.dir === 1 ? <Icon.ChevUp size={12} /> : <Icon.ChevDown size={12} />) : null}
                        </span>
                      </th>
                    )
                  })}
                </tr>
              </thead>
              <tbody>
                {sorted.map((row) => (
                  <tr key={rowKey(row)}
                    className={`border-b border-slate-100 dark:border-slate-800/70 ${clickable ? 'row-hover cursor-pointer ring-focus' : ''} ${rowClassName ? rowClassName(row) : ''}`}
                    onClick={clickable ? () => onRowClick(row) : undefined}
                    role={clickable ? 'button' : undefined}
                    tabIndex={clickable ? 0 : undefined}
                    onKeyDown={clickable ? (e) => {
                      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onRowClick(row) }
                    } : undefined}>
                    {columns.map((col) => {
                      const align = col.align || 'left'
                      const contenido = col.cell ? col.cell(row) : String(valorDe(col)(row) ?? '')
                      return (
                        <td key={col.key} className={`${pad} ${ALIGN[align]} ${col.tdClassName || ''}`}
                          onClick={col.stop ? (e) => e.stopPropagation() : undefined}>
                          {contenido}
                        </td>
                      )
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {hayFilas && footerLabel ? (
        <div className="mt-2 text-[11.5px] text-slate-500">{footerLabel(rows.length)}</div>
      ) : null}
    </div>
  )
}
