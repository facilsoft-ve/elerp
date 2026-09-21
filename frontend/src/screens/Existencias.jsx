import { useState, useMemo, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, Modal, Empty, TableSkeleton, useToast, Field, Segmented } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { api } from '../lib/api.js'

/* AVISO DE VENCIMIENTOS.
 *
 * Es la razón por la que casi todo el mundo activa los lotes: la mercancía caduca
 * en el almacén sin que nadie se entere hasta que un cliente lo ve en la etiqueta.
 * El total de existencia no lo delata —las unidades están ahí—, así que tiene que
 * avisarlo alguien.
 *
 * No aparece si no hay nada por vencer, para no ocupar sitio en un catálogo que no
 * lleva vencimientos.
 */
function AvisoVencimientos() {
  const [lotes, setLotes] = useState([])
  useEffect(() => {
    let vivo = true
    api.lotesPorVencer({ dias: 30 })
      .then((r) => { if (vivo) setLotes(r?.lotes || []) })
      .catch(() => { if (vivo) setLotes([]) })
    return () => { vivo = false }
  }, [])
  if (lotes.length === 0) return null

  const vencidos = lotes.filter((l) => l.vencido)
  return (
    <div className="mb-3 rounded-xl bg-amber-50 dark:bg-amber-900/25 border border-amber-200 dark:border-amber-900/40 px-3 py-2.5">
      <div className="flex items-start gap-2.5">
        <Icon.CircleAlert size={15} className="mt-0.5 shrink-0 text-amber-700 dark:text-amber-400" />
        <div className="text-[12.5px] text-amber-900 dark:text-amber-200 min-w-0">
          <strong>
            {vencidos.length > 0
              ? `${vencidos.length} lote(s) VENCIDO(S) con existencia`
              : `${lotes.length} lote(s) vencen en los próximos 30 días`}
          </strong>
          {vencidos.length > 0 && lotes.length > vencidos.length
            ? ` y ${lotes.length - vencidos.length} más por vencer` : null}
          <div className="mt-1 space-y-0.5">
            {lotes.slice(0, 5).map((l) => (
              <div key={l.sku + l.lote} className="flex items-baseline gap-2 flex-wrap">
                <span className="num text-[11.5px]">{l.lote}</span>
                <span className="text-[11.5px]">{l.nombre}</span>
                <span className="num text-[11.5px]">{fmtNum(l.cantidad)} u.</span>
                <span className={`text-[11.5px] ${l.vencido ? 'font-semibold' : ''}`}>
                  {l.vencido ? `venció hace ${Math.abs(l.diasParaVencer)} día(s)` : `vence en ${l.diasParaVencer} día(s)`}
                </span>
              </div>
            ))}
            {lotes.length > 5 ? <div className="text-[11.5px] opacity-80">…y {lotes.length - 5} más.</div> : null}
          </div>
        </div>
      </div>
    </div>
  )
}

/* DÓNDE ESTÁ un producto dentro de la sede.
 *
 * La suma de las cantidades es la existencia de la sede, siempre — y se muestra,
 * porque es la comprobación que convierte esta pantalla en algo fiable: si el
 * desglose no cuadra con el total, la ubicación está mintiendo.
 */
function FilaUbicaciones({ sku, sedeId, total }) {
  const [rows, setRows] = useState(null)
  useEffect(() => {
    let vivo = true
    api.existenciaPorUbicacion(sku, sedeId)
      .then((r) => { if (vivo) setRows(r?.ubicaciones || []) })
      .catch(() => { if (vivo) setRows([]) })
    return () => { vivo = false }
  }, [sku, sedeId])

  const suma = (rows || []).reduce((a, u) => a + (Number(u.cantidad) || 0), 0)
  const cuadra = Math.abs(suma - (Number(total) || 0)) < 0.005

  return (
    <tr className="bg-slate-50/70 dark:bg-slate-800/40">
      <td colSpan={7} className="px-4 py-3">
        {rows === null ? (
          <div className="text-[12px] text-slate-400">Cargando…</div>
        ) : (
          <div className="space-y-1">
            {rows.map((u) => (
              <div key={u.almacenId + '/' + u.ubicacionId} className="flex items-baseline gap-2 text-[12.5px]">
                <span className="num text-slate-500 w-32 shrink-0">{u.codigo}</span>
                <span className="text-slate-600 dark:text-slate-300 flex-1 min-w-0 truncate">
                  {u.almacenNombre}{u.nombre && u.codigo !== u.nombre ? ` · ${u.nombre}` : ''}
                </span>
                <span className="num font-medium">{fmtNum(u.cantidad)}</span>
              </div>
            ))}
            <div className={`flex items-baseline gap-2 text-[12px] pt-1 mt-1 border-t border-slate-200 dark:border-slate-700 ${cuadra ? 'text-slate-400' : 'text-red-600 dark:text-red-400 font-medium'}`}>
              <span className="flex-1">
                {cuadra
                  ? 'La suma por ubicación cuadra con la existencia de la sede.'
                  : 'El desglose NO cuadra con la existencia: avisa al equipo.'}
              </span>
              <span className="num">{fmtNum(suma)}</span>
            </div>
          </div>
        )}
      </td>
    </tr>
  )
}

const LOW_STOCK = 5
const puedeAjustar = (rol) => ['dueno', 'desarrollador'].includes(rol)

export function Existencias({ onKardex }) {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const { activeSede, activeSedeId } = useAuth()
  const toast = useToast()
  const almacenes = (db.ALMACENES || []).filter((a) => a.activo && a.sedeId === activeSede?.id)

  const [q, setQ] = useState('')
  const [soloBajo, setSoloBajo] = useState('todos')
  const [ajuste, setAjuste] = useState(null)
  // Qué SKU tiene desplegado su desglose por ubicación. Uno a la vez: la pregunta
  // «¿dónde está esto?» es de un producto concreto, no de la lista entera.
  const [dondeEsta, setDondeEsta] = useState('')
  // Almacén seleccionado: '' = TODOS (existencia por sede, suma de almacenes). Con un
  // almacén concreto, la existencia es de ESE almacén (se pide al backend aparte).
  const [almacenSel, setAlmacenSel] = useState('')
  const [rowsAlm, setRowsAlm] = useState(null) // null ⇒ usar db.EXISTENCIAS (sede)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    if (!almacenSel) { setRowsAlm(null); return }
    let vivo = true
    api.existencias(activeSede?.id, almacenSel)
      .then((r) => { if (vivo) setRowsAlm(r || []) })
      .catch(() => { if (vivo) setRowsAlm([]) })
    return () => { vivo = false }
  }, [almacenSel, activeSede, tick])

  // Al cambiar de sede, el almacén elegido deja de ser válido: volver a "Todos".
  useEffect(() => { setAlmacenSel('') }, [activeSede])

  const existencias = almacenSel ? rowsAlm : db.EXISTENCIAS
  const recargar = async () => { await reload(); setTick((t) => t + 1) }

  const editable = puedeAjustar(ui.rol)
  const pad = 'py-2.5'

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return (existencias || []).filter((e) => {
      const okQ = !term || e.nombre.toLowerCase().includes(term) || (e.sku || '').toLowerCase().includes(term)
      const okB = soloBajo === 'todos' || (Number(e.cantidad) || 0) <= LOW_STOCK
      return okQ && okB
    })
  }, [existencias, q, soloBajo])

  const valorTotal = (existencias || []).reduce((a, e) => a + (Number(e.valor) || 0), 0)

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las existencias"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  return (
    <div>
      <AvisoVencimientos />
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por nombre o SKU…"
          value={q} onChange={(e) => setQ(e.target.value)} />
        <Segmented size="sm" value={soloBajo} onChange={setSoloBajo}
          options={[{ value: 'todos', label: 'Todos' }, { value: 'bajo', label: 'Stock bajo' }]} />
        <Badge color="huberp" className="ml-1"><Icon.Home size={12} /> {activeSede ? activeSede.nombre : 'Sede activa'}</Badge>
        {almacenes.length ? (
          <Select className="!w-52" value={almacenSel} onChange={(e) => setAlmacenSel(e.target.value)}
            title="Ver existencia por almacén (o el total de la sede)">
            <option value="">Todos los almacenes (sede)</option>
            {almacenes.map((a) => <option key={a.id} value={a.id}>{a.nombre}{a.principal ? ' ★' : ''}</option>)}
          </Select>
        ) : null}
        <div className="ml-auto flex items-center gap-2">
          <div className="text-[12px] text-slate-500 hidden sm:block">Valor total: <span className="num font-medium text-slate-700 dark:text-slate-200 private-mask">{fmtCurrency(valorTotal, ui.ccy, { max: 0 })}</span></div>
        </div>
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || existencias === undefined ? (
          <div className="p-4"><TableSkeleton rows={7} cols={5} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.Boxes size={22} />}
            title={q || soloBajo === 'bajo' ? 'Sin resultados' : 'Sin existencias en esta sede'}
            body={q || soloBajo === 'bajo' ? 'Prueba con otro término o filtro.' : 'Las existencias se generan a partir de compras, ventas y ajustes.'} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className={`${pad} pr-3 font-medium`}>SKU</th>
                  <th className={`${pad} pr-3 font-medium`}>Producto</th>
                  <th className={`${pad} pr-3 font-medium text-right`}>Cantidad</th>
                  <th className={`${pad} pr-3 font-medium text-right`}>Costo prom.</th>
                  <th className={`${pad} pr-3 font-medium text-right`}>Valor</th>
                  <th className={`${pad} pr-3 font-medium text-right`}>Acciones</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((e) => {
                  const bajo = (Number(e.cantidad) || 0) <= LOW_STOCK
                  return (
                    <tr key={e.productoId || e.sku} className={`border-b border-slate-100 dark:border-slate-800/70 row-hover ${bajo ? 'bg-amber-50/40 dark:bg-amber-900/10' : ''}`}>
                      <td className={`${pad} pr-3 num text-[12.5px] text-slate-500`}>{e.sku}</td>
                      <td className={`${pad} pr-3 font-medium text-[13px]`}>{e.nombre}</td>
                      <td className={`${pad} pr-3 text-right num font-medium ${bajo ? 'text-amber-700 dark:text-amber-400' : ''}`}>
                        {fmtNum(e.cantidad)} {bajo ? <Icon.CircleAlert size={13} className="inline ml-1 -mt-0.5" /> : null}
                      </td>
                      <td className={`${pad} pr-3 text-right num text-slate-500 private-mask`}>{fmtCurrency(e.costoPromedio, ui.ccy)}</td>
                      <td className={`${pad} pr-3 text-right num font-medium private-mask`}>{fmtCurrency(e.valor, ui.ccy)}</td>
                      <td className={`${pad} pr-3 text-right whitespace-nowrap`}>
                        <button onClick={() => setDondeEsta(dondeEsta === e.sku ? '' : e.sku)} title="¿Dónde está?"
                          className={`h-7 w-7 inline-flex items-center justify-center rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 ${dondeEsta === e.sku ? 'text-[#D6246E]' : 'text-slate-400'}`}>
                          <Icon.Home size={15} />
                        </button>
                        <button onClick={() => onKardex && onKardex(e.sku)} title="Ver Kardex"
                          className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800">
                          <Icon.History size={15} />
                        </button>
                        {editable ? (
                          <button onClick={() => setAjuste(e)} title="Ajustar existencia"
                            className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800">
                            <Icon.Pencil size={15} />
                          </button>
                        ) : null}
                      </td>
                    </tr>
                  )
                }).flatMap((fila, i) => {
                  const e = rows[i]
                  if (!e || dondeEsta !== e.sku) return [fila]
                  return [fila, <FilaUbicaciones key={e.sku + '-ubi'} sku={e.sku} sedeId={activeSedeId} total={e.cantidad} />]
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} producto(s){soloBajo === 'bajo' ? ' con stock bajo' : ''}</div> : null}

      {ajuste ? <AjustarModal existencia={ajuste} sedeNombre={activeSede?.nombre}
        almacenId={almacenSel} almacenNombre={almacenes.find((a) => a.id === almacenSel)?.nombre}
        onClose={() => setAjuste(null)} onSaved={recargar} toast={toast} /> : null}
    </div>
  )
}

// Ajuste manual auditado: exige motivo + cantidad (+/-). Nunca sobrescribe el
// saldo sin traza; el backend registra el ajuste en el ledger (Kardex).
function AjustarModal({ existencia, sedeNombre, almacenId, almacenNombre, onClose, onSaved, toast }) {
  const [motivo, setMotivo] = useState('')
  const [cantidad, setCantidad] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  const nCant = Number(cantidad)
  const errMotivo = !motivo.trim() ? 'El motivo es obligatorio (queda en el historial).' : ''
  const errCant = cantidad === '' || isNaN(nCant) ? 'Ingresa una cantidad.' : nCant === 0 ? 'La cantidad no puede ser cero.' : ''
  const valid = !errMotivo && !errCant
  const nuevoSaldo = (Number(existencia.cantidad) || 0) + (isNaN(nCant) ? 0 : nCant)

  const save = async () => {
    setTouched(true)
    if (!valid) return
    setBusy(true)
    try {
      await api.ajustarExistencia(existencia.sku, { motivo: motivo.trim(), cantidad: nCant }, almacenId)
      toast({ title: 'Existencia ajustada', body: `${existencia.nombre}: ${nCant > 0 ? '+' : ''}${nCant}${almacenNombre ? ' en ' + almacenNombre : ''}. Registrado en el Kardex.` })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo ajustar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Pencil size={18} />}
      title="Ajustar existencia" sub={`${existencia.nombre} · ${almacenNombre || sedeNombre || ''}${almacenNombre ? '' : ' · almacén principal'}`}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={save} loading={busy} icon={<Icon.Check size={16} />}>Registrar ajuste</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="flex items-center justify-between text-[13px] p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60">
          <span className="text-slate-500">Saldo actual</span>
          <span className="num font-medium">{fmtNum(existencia.cantidad)}</span>
        </div>
        <Field label="Cantidad" required hint="usa negativo para restar" error={touched ? errCant : ''}>
          <Input type="number" step="0.01" value={cantidad} onChange={(e) => setCantidad(e.target.value)} onBlur={() => setTouched(true)}
            invalid={touched && !!errCant} placeholder="Ej: +10 o -3" autoFocus />
        </Field>
        <Field label="Motivo" required error={touched ? errMotivo : ''}>
          <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} onBlur={() => setTouched(true)}
            invalid={touched && !!errMotivo} placeholder="Ej: Merma por vencimiento, conteo físico…" />
        </Field>
        {!errCant && cantidad !== '' ? (
          <div className="flex items-center justify-between text-[13px] p-3 rounded-lg bg-elerp-50 dark:bg-elerp-900/30">
            <span className="text-elerp-700 dark:text-elerp-200">Nuevo saldo</span>
            <span className="num font-semibold text-elerp-700 dark:text-elerp-100">{fmtNum(nuevoSaldo)}</span>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
