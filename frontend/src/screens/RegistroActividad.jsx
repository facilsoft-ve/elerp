import { useState, useEffect, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Badge, Input, Select, Button, Empty, TableSkeleton } from '../components/primitives.jsx'
import { fmtDate } from '../lib/format.js'
import { api } from '../lib/api.js'

/* Registro de actividad (bitácora de auditoría del tenant): la trazabilidad
 * transversal de quién hizo qué, cuándo y desde dónde. Solo lectura. Buscador
 * avanzado: rango de fechas + prefijo de acción + búsqueda libre. */

// Prefijos de acción para el filtro (las acciones se nombran "area.entidad.verbo").
const AREAS = [
  { v: '', label: 'Todas las áreas' },
  { v: 'fiscal', label: 'Fiscal (documentos, retenciones)' },
  { v: 'inventario', label: 'Inventario' },
  { v: 'contabilidad', label: 'Contabilidad' },
  { v: 'compras', label: 'Compras' },
  { v: 'ventas', label: 'Ventas' },
  { v: 'caja', label: 'Caja' },
  { v: 'config', label: 'Configuración' },
  { v: 'usuario', label: 'Usuarios' },
  { v: 'tenancy', label: 'Empresa / sedes' },
]

// Color del badge por área (prefijo de la acción).
const COLOR_AREA = {
  fiscal: 'blue', inventario: 'emerald', contabilidad: 'amber',
  compras: 'slate', ventas: 'teal', caja: 'teal', config: 'slate', usuario: 'blue', tenancy: 'slate',
}
const areaDe = (accion) => (accion || '').split('.')[0]

// Fecha de hoy y de hace 30 días (YYYY-MM-DD) como rango por defecto.
function isoHace(dias) {
  const d = new Date(); d.setDate(d.getDate() - dias)
  return d.toISOString().slice(0, 10)
}

export function RegistroActividad() {
  const [f, setF] = useState({ desde: isoHace(30), hasta: new Date().toISOString().slice(0, 10), accion: '', q: '' })
  const [data, setData] = useState(undefined) // undefined = cargando; null = error
  const [error, setError] = useState('')

  // Debounce: se re-consulta 350 ms después del último cambio de filtro.
  const key = useMemo(() => JSON.stringify(f), [f])
  useEffect(() => {
    let vivo = true
    const t = setTimeout(async () => {
      setData(undefined); setError('')
      try {
        const r = await api.auditoria({ desde: f.desde, hasta: f.hasta, accion: f.accion, q: f.q.trim(), limite: 500 })
        if (vivo) setData(r || [])
      } catch (e) {
        if (vivo) { setError(e?.message || 'No se pudo cargar el registro'); setData(null) }
      }
    }, 350)
    return () => { vivo = false; clearTimeout(t) }
  }, [key]) // eslint-disable-line react-hooks/exhaustive-deps

  const set = (campo, valor) => setF((s) => ({ ...s, [campo]: valor }))
  const limpiar = () => setF({ desde: isoHace(30), hasta: new Date().toISOString().slice(0, 10), accion: '', q: '' })

  return (
    <div>
      {/* Buscador avanzado */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-3.5 mb-3">
        <div className="grid grid-cols-1 sm:grid-cols-4 gap-2.5">
          <div>
            <label className="block text-[11px] font-medium uppercase tracking-wide text-slate-400 mb-1">Desde</label>
            <Input type="date" value={f.desde} max={f.hasta || undefined} onChange={(e) => set('desde', e.target.value)} />
          </div>
          <div>
            <label className="block text-[11px] font-medium uppercase tracking-wide text-slate-400 mb-1">Hasta</label>
            <Input type="date" value={f.hasta} min={f.desde || undefined} onChange={(e) => set('hasta', e.target.value)} />
          </div>
          <div>
            <label className="block text-[11px] font-medium uppercase tracking-wide text-slate-400 mb-1">Área</label>
            <Select value={f.accion} onChange={(e) => set('accion', e.target.value)}>
              {AREAS.map((a) => <option key={a.v} value={a.v}>{a.label}</option>)}
            </Select>
          </div>
          <div>
            <label className="block text-[11px] font-medium uppercase tracking-wide text-slate-400 mb-1">Buscar</label>
            <Input icon={<Icon.Search size={15} />} placeholder="Actor, entidad, detalle…" value={f.q} onChange={(e) => set('q', e.target.value)} />
          </div>
        </div>
        <div className="flex items-center justify-between mt-2.5">
          <div className="text-[11.5px] text-slate-400">
            {data === undefined ? 'Buscando…' : data && data.length ? `${data.length} evento(s)` : ''}
          </div>
          <Button size="sm" variant="ghost" icon={<Icon.X size={14} />} onClick={limpiar}>Limpiar filtros</Button>
        </div>
      </div>

      {/* Resultados */}
      {data === undefined ? (
        <TableSkeleton cols={5} rows={8} />
      ) : error || data === null ? (
        <Empty icon={<Icon.Shield size={22} />} title="No se pudo cargar el registro" body={error} />
      ) : data.length === 0 ? (
        <Empty icon={<Icon.Shield size={22} />} title="Sin actividad en el rango"
          body="No hay eventos que coincidan con los filtros. Amplía el rango de fechas o quita la búsqueda." />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium whitespace-nowrap">Fecha y hora</th>
                  <th className="py-2.5 px-3 font-medium">Actor</th>
                  <th className="py-2.5 px-3 font-medium">Acción</th>
                  <th className="py-2.5 px-3 font-medium">Entidad / detalle</th>
                  <th className="py-2.5 px-3 font-medium">Origen</th>
                </tr>
              </thead>
              <tbody>
                {data.map((e) => (
                  <tr key={e.id} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2.5 px-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(e.fecha)}</td>
                    <td className="py-2.5 px-3">
                      <div className="text-[13px] font-medium truncate max-w-[180px]">{e.actorNombre}</div>
                      {e.rol ? <div className="text-[11px] text-slate-400">{e.rol}</div> : null}
                    </td>
                    <td className="py-2.5 px-3">
                      <Badge size="sm" color={COLOR_AREA[areaDe(e.accion)] || 'slate'}>{areaDe(e.accion)}</Badge>
                      <div className="text-[11.5px] text-slate-400 mono mt-0.5">{e.accion}</div>
                    </td>
                    <td className="py-2.5 px-3">
                      <div className="text-[12.5px] truncate max-w-[280px]">{e.detalle || e.entidad || '—'}</div>
                      {e.entidad && e.detalle && e.entidad !== e.detalle ? <div className="text-[11px] text-slate-400 mono truncate max-w-[280px]">{e.entidad}</div> : null}
                    </td>
                    <td className="py-2.5 px-3 text-[12px] text-slate-400 mono whitespace-nowrap">{e.origen || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <div className="mt-3 text-[11.5px] text-slate-400">
        El registro es <b>append-only</b> (no se edita ni se borra): cada cambio de estado del sistema deja su huella
        con actor, acción, entidad, origen y hora UTC. Es la base de la trazabilidad ante una auditoría.
      </div>
    </div>
  )
}
