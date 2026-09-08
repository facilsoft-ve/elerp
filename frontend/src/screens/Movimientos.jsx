import { useState, useEffect, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Badge, Select, Input, Empty } from '../components/primitives.jsx'
import { TablaDatos } from '../components/TablaDatos.jsx'
import { fmtNum, fmtDate } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { api } from '../lib/api.js'

// Registro de entradas y salidas: vista cronológica global de los movimientos del
// ledger (entradas, salidas, ajustes y transferencias), filtrable. Complementa al
// Kardex, que es por producto.
const TIPO_META = {
  entrada: { label: 'Entrada', color: 'emerald' },
  salida: { label: 'Salida', color: 'rose' },
  ajuste: { label: 'Ajuste', color: 'amber' },
  transferencia: { label: 'Transferencia', color: 'sky' },
}
const tipoMeta = (t) => TIPO_META[t] || { label: t || '—', color: 'slate' }

export function Movimientos({ onKardex }) {
  const { db } = useData()
  const sedes = db.SEDES || []
  const almacenes = db.ALMACENES || []
  const productos = db.PRODUCTOS || []
  const sedeNombre = (id) => sedes.find((s) => s.id === id)?.nombre || (id ? '—' : 'Todas')
  const almacenNombre = (id) => almacenes.find((a) => a.id === id)?.nombre || (id ? '' : '')

  const [f, setF] = useState({ sede: '', almacen: '', sku: '', tipo: '', desde: '', hasta: '' })
  const [rows, setRows] = useState(null)
  const [err, setErr] = useState(null)

  // Almacenes disponibles para el filtro, acotados a la sede elegida.
  const almacenesFiltro = useMemo(
    () => almacenes.filter((a) => !f.sede || a.sedeId === f.sede),
    [almacenes, f.sede],
  )

  useEffect(() => {
    let vivo = true
    setErr(null)
    setRows(null)
    api.movimientos(f)
      .then((r) => { if (vivo) setRows(r || []) })
      .catch((e) => { if (vivo) { setErr(e); setRows([]) } })
    return () => { vivo = false }
  }, [f])

  const set = (k, v) => setF((s) => ({ ...s, [k]: v, ...(k === 'sede' ? { almacen: '' } : {}) }))

  const columns = [
    {
      key: 'fecha', header: 'Fecha', sortable: true,
      sortValue: (m) => m.fecha, csv: (m) => m.fecha,
      cell: (m) => <span className="num text-[12.5px] text-slate-500 whitespace-nowrap">{fmtDate(m.fecha)}</span>,
    },
    {
      key: 'producto', header: 'Producto', sortable: true,
      sortValue: (m) => m.nombre || m.sku, csv: (m) => `${m.sku} ${m.nombre || ''}`.trim(),
      cell: (m) => (
        <div>
          <div className="text-[13px] font-medium truncate max-w-[220px]">{m.nombre || m.sku}</div>
          <div className="text-[11px] text-slate-400 num">{m.sku}</div>
        </div>
      ),
    },
    {
      key: 'tipo', header: 'Tipo', sortable: true,
      sortValue: (m) => m.tipo, csv: (m) => tipoMeta(m.tipo).label,
      cell: (m) => <Badge size="sm" color={tipoMeta(m.tipo).color} dot>{tipoMeta(m.tipo).label}</Badge>,
    },
    {
      key: 'cantidad', header: 'Cantidad', align: 'right', sortable: true,
      sortValue: (m) => Number(m.cantidad) || 0, csv: (m) => Number(m.cantidad) || 0,
      cell: (m) => {
        const n = Number(m.cantidad) || 0
        return <span className={`num font-medium ${n >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'}`}>{n > 0 ? '+' : ''}{fmtNum(n)}</span>
      },
    },
    {
      key: 'almacen', header: 'Almacén', sortable: true,
      sortValue: (m) => almacenNombre(m.almacenId), csv: (m) => almacenNombre(m.almacenId),
      cell: (m) => <span className="text-[12.5px] text-slate-500">{almacenNombre(m.almacenId) || <span className="text-slate-300">principal</span>}</span>,
    },
    {
      key: 'sede', header: 'Sede', sortable: true,
      sortValue: (m) => sedeNombre(m.sedeId), csv: (m) => sedeNombre(m.sedeId),
      cell: (m) => <span className="text-[12.5px] text-slate-500">{sedeNombre(m.sedeId)}</span>,
    },
    {
      key: 'motivo', header: 'Motivo / Ref.', sortable: false,
      csv: (m) => m.motivo || '',
      cell: (m) => <span className="text-[12px] text-slate-500 truncate max-w-[240px] inline-block align-middle">{m.motivo || '—'}</span>,
    },
  ]

  return (
    <div>
      <div className="flex items-end gap-2 flex-wrap mb-3">
        <div>
          <label className="block text-[11px] text-slate-400 mb-0.5">Producto</label>
          <Select className="!w-52" value={f.sku} onChange={(e) => set('sku', e.target.value)}>
            <option value="">Todos</option>
            {productos.filter((p) => !p.esCombo).map((p) => <option key={p.id} value={p.sku}>{p.sku} · {p.nombre}</option>)}
          </Select>
        </div>
        <div>
          <label className="block text-[11px] text-slate-400 mb-0.5">Sede</label>
          <Select className="!w-40" value={f.sede} onChange={(e) => set('sede', e.target.value)}>
            <option value="">Todas</option>
            {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
          </Select>
        </div>
        {almacenesFiltro.length ? (
          <div>
            <label className="block text-[11px] text-slate-400 mb-0.5">Almacén</label>
            <Select className="!w-44" value={f.almacen} onChange={(e) => set('almacen', e.target.value)}>
              <option value="">Todos</option>
              {almacenesFiltro.map((a) => <option key={a.id} value={a.id}>{a.nombre}</option>)}
            </Select>
          </div>
        ) : null}
        <div>
          <label className="block text-[11px] text-slate-400 mb-0.5">Tipo</label>
          <Select className="!w-40" value={f.tipo} onChange={(e) => set('tipo', e.target.value)}>
            <option value="">Todos</option>
            <option value="entrada">Entrada</option>
            <option value="salida">Salida</option>
            <option value="ajuste">Ajuste</option>
            <option value="transferencia">Transferencia</option>
          </Select>
        </div>
        <div>
          <label className="block text-[11px] text-slate-400 mb-0.5">Desde</label>
          <Input type="date" value={f.desde} onChange={(e) => set('desde', e.target.value)} />
        </div>
        <div>
          <label className="block text-[11px] text-slate-400 mb-0.5">Hasta</label>
          <Input type="date" value={f.hasta} onChange={(e) => set('hasta', e.target.value)} />
        </div>
      </div>

      {err ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar los movimientos" body={String(err?.message || err)} />
      ) : (
        <TablaDatos
          columns={columns}
          rows={rows || []}
          rowKey={(m) => m.id}
          onRowClick={onKardex ? (m) => onKardex(m.sku) : undefined}
          csvName="movimientos"
          loading={rows === null}
          initialSort={{ key: 'fecha', dir: -1 }}
          minWidth="min-w-[820px]"
          empty={<Empty icon={<Icon.ArrowLeftRight size={22} />} title="Sin movimientos"
            body="No hay entradas ni salidas para los filtros elegidos." />}
          footerLabel={(n) => `${fmtNum(n, 0)} movimiento(s)`}
        />
      )}
    </div>
  )
}
