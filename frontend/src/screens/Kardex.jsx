import { useState, useEffect, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Badge, Select, Empty, TableSkeleton, Card } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { api } from '../lib/api.js'

const TIPO_META = {
  entrada: { color: 'emerald', label: 'Entrada', icon: Icon.ArrowDown },
  salida: { color: 'rose', label: 'Salida', icon: Icon.ArrowUp },
  ajuste: { color: 'amber', label: 'Ajuste', icon: Icon.Pencil },
  transferencia: { color: 'sky', label: 'Transferencia', icon: Icon.ArrowLeftRight },
}

export function Kardex({ sku, setSku }) {
  const { db } = useData()
  const { ui } = useUI()
  const { activeSedeId, activeSede } = useAuth()
  const productos = db.PRODUCTOS || []
  const almacenes = (db.ALMACENES || []).filter((a) => a.activo && a.sedeId === activeSedeId)

  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  // '' = Kardex de la sede (todos sus almacenes); un almacén ⇒ Kardex de ese almacén.
  const [almacenSel, setAlmacenSel] = useState('')
  useEffect(() => { setAlmacenSel('') }, [activeSedeId])

  const load = useCallback(async () => {
    if (!sku) { setData(null); return }
    setLoading(true)
    setError(null)
    try {
      const res = await api.kardex(sku, activeSedeId, almacenSel)
      setData(res)
    } catch (e) {
      setError(e)
    } finally {
      setLoading(false)
    }
  }, [sku, activeSedeId, almacenSel])

  useEffect(() => { load() }, [load])

  const movs = data?.movimientos || []

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Select className="!w-72" value={sku || ''} onChange={(e) => setSku(e.target.value)}>
          <option value="">Elige un producto…</option>
          {productos.map((p) => <option key={p.id} value={p.sku}>{p.sku} · {p.nombre}</option>)}
        </Select>
        <Badge color="huberp" className="ml-1"><Icon.Home size={12} /> {activeSede ? activeSede.nombre : 'Sede activa'}</Badge>
        {almacenes.length ? (
          <Select className="!w-52" value={almacenSel} onChange={(e) => setAlmacenSel(e.target.value)}
            title="Kardex por almacén (o el total de la sede)">
            <option value="">Todos los almacenes (sede)</option>
            {almacenes.map((a) => <option key={a.id} value={a.id}>{a.nombre}{a.principal ? ' ★' : ''}</option>)}
          </Select>
        ) : null}
      </div>

      {/* Nota de diseño: el saldo NO es un contador editable. */}
      <div className="flex items-start gap-2 text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/50 border border-slate-200 dark:border-slate-800 rounded-lg px-3 py-2 mb-3">
        <Icon.Shield size={14} className="mt-0.5 shrink-0 text-teal-500" />
        <span>El saldo y el costo promedio se <b>derivan de los movimientos</b> (ledger append-only): no son contadores editables. Las correcciones se hacen con nuevos movimientos, nunca borrando.</span>
      </div>

      {!sku ? (
        <Empty icon={<Icon.History size={22} />} title="Elige un producto"
          body="Selecciona un SKU para ver su Kardex: cada movimiento con su saldo y costo promedio resultante." />
      ) : error ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar el Kardex" body={String(error.message || error)} />
      ) : (
        <>
          <PanelLotes sku={sku} producto={productos.find((p) => p.sku === sku)} />
          {data?.producto ? (
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-4">
              <Card className="!p-4">
                <div className="text-[11px] uppercase tracking-wide text-slate-400">Producto</div>
                <div className="text-sm font-semibold mt-1">{data.producto.nombre}</div>
                <div className="text-[11.5px] text-slate-400 num">{data.producto.sku} · {data.producto.unidadBase}</div>
              </Card>
              <Card className="!p-4">
                <div className="text-[11px] uppercase tracking-wide text-slate-400">Saldo actual</div>
                <div className="text-lg font-semibold num mt-1">{fmtNum(movs.length ? movs[movs.length - 1].saldo : 0)}</div>
              </Card>
              <Card className="!p-4">
                <div className="text-[11px] uppercase tracking-wide text-slate-400">Costo promedio</div>
                <div className="text-lg font-semibold num private-mask mt-1">{fmtCurrency(movs.length ? movs[movs.length - 1].saldoCosto : 0, ui.ccy)}</div>
              </Card>
            </div>
          ) : null}

          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
            {loading ? (
              <div className="p-4"><TableSkeleton rows={8} cols={6} /></div>
            ) : movs.length === 0 ? (
              <Empty icon={<Icon.History size={22} />} title="Sin movimientos" body="Este producto todavía no registra entradas, salidas ni ajustes en esta sede." />
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm tbl-sticky">
                  <thead>
                    <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                      <th className="py-2.5 pr-3 font-medium">Fecha</th>
                      <th className="py-2.5 pr-3 font-medium">Tipo</th>
                      <th className="py-2.5 pr-3 font-medium">Motivo / Ref.</th>
                      <th className="py-2.5 pr-3 font-medium text-right">Cantidad</th>
                      <th className="py-2.5 pr-3 font-medium text-right">Costo unit.</th>
                      <th className="py-2.5 pr-3 font-medium text-right">Saldo</th>
                      <th className="py-2.5 pr-3 font-medium text-right">Costo prom.</th>
                    </tr>
                  </thead>
                  <tbody>
                    {movs.map((m) => {
                      const meta = TIPO_META[m.tipo] || { color: 'slate', label: m.tipo, icon: Icon.Activity }
                      const IconC = meta.icon
                      const entra = m.tipo === 'entrada' || (m.tipo !== 'salida' && Number(m.cantidad) > 0)
                      return (
                        <tr key={m.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
                          <td className="py-2.5 pr-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(m.fecha)}</td>
                          <td className="py-2.5 pr-3"><Badge size="sm" color={meta.color} dot>{meta.label}</Badge></td>
                          <td className="py-2.5 pr-3">
                            <div className="text-[12.5px] truncate max-w-[220px]">{m.motivo || '—'}</div>
                            {m.ref ? <div className="text-[11px] text-slate-400 num">{m.ref}</div> : null}
                          </td>
                          <td className={`py-2.5 pr-3 text-right num font-medium ${entra ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'}`}>
                            {Number(m.cantidad) > 0 ? '+' : ''}{fmtNum(m.cantidad)}
                          </td>
                          <td className="py-2.5 pr-3 text-right num text-slate-500 private-mask">{fmtCurrency(m.costoUnitario, ui.ccy)}</td>
                          <td className="py-2.5 pr-3 text-right num font-medium">{fmtNum(m.saldo)}</td>
                          <td className="py-2.5 pr-3 text-right num text-slate-500 private-mask">{fmtCurrency(m.saldoCosto, ui.ccy)}</td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}

/* TRAZABILIDAD POR LOTE — «¿a quién le vendí el lote X?».
 *
 * Es la consulta que justifica llevar lotes: guardar el dato y no poder
 * preguntarlo es tener el archivo y no la función. En una alerta sanitaria hay
 * horas para avisar a quien compró, y revisar el ledger a mano no es una
 * respuesta.
 *
 * Vive dentro del Kardex porque es donde alguien va a buscar la historia de un
 * producto; no aparece si el producto no lleva lotes.
 */
function PanelLotes({ sku, producto }) {
  const [lotes, setLotes] = useState([])
  const [sel, setSel] = useState('')
  const [rastro, setRastro] = useState(null)
  const [cargando, setCargando] = useState(false)

  useEffect(() => {
    let vivo = true
    setSel(''); setRastro(null)
    if (!sku) return undefined
    // SIN sede: un lote no respeta los límites de una sucursal, y en una alerta
    // hay que verlo entero.
    api.lotesHistoricos(sku)
      .then((r) => { if (vivo) setLotes(r?.lotes || []) })
      .catch(() => { if (vivo) setLotes([]) })
    return () => { vivo = false }
  }, [sku])

  const verRastro = (lote) => {
    setSel(lote)
    setRastro(null)
    if (!lote) return
    setCargando(true)
    api.rastroDeLote(sku, lote)
      .then(setRastro)
      .catch(() => setRastro(null))
      .finally(() => setCargando(false))
  }

  if (!producto?.requiereLote) return null

  return (
    <Card className="mb-4">
      <div className="flex items-start justify-between gap-3 flex-wrap mb-2">
        <div>
          <div className="text-[13px] font-semibold">Trazabilidad por lote</div>
          <div className="text-[12.5px] text-slate-500 dark:text-slate-400 mt-0.5">
            Elige un lote para ver su historia completa: de quién se compró, a quién se vendió y qué queda.
            Se listan también los <strong>agotados</strong>: tras una alerta, ese es justo el que hay que mirar.
          </div>
        </div>
        <Select className="!w-64" value={sel} onChange={(e) => verRastro(e.target.value)}>
          <option value="">Elige un lote…</option>
          {lotes.map((l) => <option key={l} value={l}>{l}</option>)}
        </Select>
      </div>

      {lotes.length === 0 ? (
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
          Este producto lleva lotes pero todavía no se ha recibido ninguno.
        </div>
      ) : cargando ? (
        <TableSkeleton rows={3} cols={4} />
      ) : rastro ? (
        <div className="space-y-3">
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-[12.5px]">
            <div><span className="text-slate-500">Recibido</span><div className="num font-medium">{fmtNum(rastro.recibido)}</div></div>
            <div><span className="text-slate-500">Salido</span><div className="num font-medium">{fmtNum(rastro.salido)}</div></div>
            <div><span className="text-slate-500">En stock</span><div className="num font-medium">{fmtNum(rastro.enStock)}</div></div>
            <div>
              <span className="text-slate-500">Vence</span>
              <div className={`num font-medium ${rastro.vencido ? 'text-amber-700 dark:text-amber-400' : ''}`}>
                {rastro.vencimiento || '—'}{rastro.vencido ? ' (vencido)' : ''}
              </div>
            </div>
          </div>

          {/* LA RESPUESTA CORTA: a quién avisar. Va primero porque es lo que se
              necesita en los primeros cinco minutos de una alerta. */}
          {rastro.clientes?.length ? (
            <div className="rounded-lg bg-amber-50 dark:bg-amber-900/25 border border-amber-200 dark:border-amber-900/40 px-3 py-2 text-[12.5px] text-amber-900 dark:text-amber-200">
              <strong>Salió a {rastro.clientes.length} destinatario(s):</strong> {rastro.clientes.join(' · ')}
            </div>
          ) : (
            <div className="text-[12px] text-slate-500 dark:text-slate-400">
              De este lote no ha salido nada todavía.
            </div>
          )}

          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800">
                  <th className="py-2 pr-3 font-medium">Fecha</th>
                  <th className="py-2 pr-3 font-medium">Movimiento</th>
                  <th className="py-2 pr-3 font-medium">Documento</th>
                  <th className="py-2 pr-3 font-medium">Tercero</th>
                  <th className="py-2 pr-3 font-medium text-right">Cantidad</th>
                  <th className="py-2 pr-3 font-medium text-right">Saldo</th>
                </tr>
              </thead>
              <tbody>
                {rastro.pasos.map((p, i) => (
                  <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2 pr-3 text-[12px] text-slate-500 whitespace-nowrap">{fmtDate(p.fecha)}</td>
                    <td className="py-2 pr-3 text-[12.5px]">{p.tipo}</td>
                    <td className="py-2 pr-3 num text-[12px]">{p.documento || <span className="text-slate-400">—</span>}</td>
                    <td className="py-2 pr-3 text-[12.5px]">{p.tercero || <span className="text-slate-400">—</span>}</td>
                    <td className={`py-2 pr-3 text-right num text-[12.5px] ${p.cantidad < 0 ? 'text-slate-600 dark:text-slate-300' : ''}`}>
                      {fmtNum(p.cantidad)}
                    </td>
                    <td className="py-2 pr-3 text-right num text-[12.5px]">{fmtNum(p.saldo)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
          {lotes.length} lote(s) registrados. Elige uno para ver su rastro.
        </div>
      )}
    </Card>
  )
}
