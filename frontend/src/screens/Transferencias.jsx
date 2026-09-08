import { useState, useMemo, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Select, Input, Modal, Empty, TableSkeleton, useToast, Field } from '../components/primitives.jsx'
import { fmtNum, fmtDate } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'

// Máquina de estados de transferencia entre sedes. Cada avance es un evento
// auditable (crear → despacho → tránsito → recepción → cierre).
const ESTADOS = ['borrador', 'despachada', 'en_transito', 'recibida', 'cerrada']
const ESTADO_META = {
  borrador: { label: 'Borrador', color: 'slate' },
  despachada: { label: 'Despachada', color: 'sky' },
  en_transito: { label: 'En tránsito', color: 'amber' },
  recibida: { label: 'Recibida', color: 'violet' },
  cerrada: { label: 'Cerrada', color: 'emerald' },
  cancelada: { label: 'Cancelada', color: 'rose' },
}
// Fase gruesa (3 estatus visibles) sobre la máquina fina: en espera / en proceso /
// realizada (+ cancelada). Es lo que el usuario ve de un vistazo; el estado fino
// vive en el stepper del detalle.
const FASE_META = {
  borrador: { label: 'En espera', color: 'slate' },
  despachada: { label: 'En proceso', color: 'amber' },
  en_transito: { label: 'En proceso', color: 'amber' },
  recibida: { label: 'Realizada', color: 'emerald' },
  cerrada: { label: 'Realizada', color: 'emerald' },
  cancelada: { label: 'Cancelada', color: 'rose' },
}
const faseDe = (estado) => FASE_META[estado] || { label: estado, color: 'slate' }
const ACCION = {
  borrador: 'Despachar', despachada: 'Marcar en tránsito', en_transito: 'Recibir', recibida: 'Cerrar',
}
// Estados en los que aún se puede cancelar: la salida en origen todavía no se ha
// recibido en destino, así que la reversa es una compensación al origen. Una vez
// recibida/cerrada la reversa correcta es una transferencia de vuelta.
const CANCELABLES = ['borrador', 'despachada', 'en_transito']
const puedeGestionar = (rol) => ['dueno', 'desarrollador'].includes(rol)

export function Transferencias() {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const transferencias = db.TRANSFERENCIAS
  const sedes = db.SEDES || []

  const [nueva, setNueva] = useState(false)
  const [detalle, setDetalle] = useState(null)

  const editable = puedeGestionar(ui.rol)
  const sedeName = (id) => sedes.find((s) => s.id === id)?.nombre || id || '—'

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las transferencias"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <div className="text-[13px] text-slate-500">Movimientos de stock entre sedes de la empresa.</div>
        {editable ? <Button className="ml-auto" icon={<Icon.Plus size={16} />} onClick={() => setNueva(true)}>Nueva transferencia</Button> : null}
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || transferencias === undefined ? (
          <div className="p-4"><TableSkeleton rows={5} cols={5} /></div>
        ) : (transferencias || []).length === 0 ? (
          <Empty icon={<Icon.ArrowLeftRight size={22} />} title="Sin transferencias"
            body="Crea una transferencia para mover stock entre dos sedes con trazabilidad completa."
            cta={editable ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNueva(true)}>Nueva transferencia</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 pr-3 font-medium">Documento</th>
                  <th className="py-2.5 pr-3 font-medium">Creada</th>
                  <th className="py-2.5 pr-3 font-medium">Ruta</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Ítems</th>
                  <th className="py-2.5 pr-3 font-medium">Estatus</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {(transferencias || []).map((t) => {
                  const fase = faseDe(t.estado)
                  const det = ESTADO_META[t.estado] || { label: t.estado }
                  return (
                    <tr key={t.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover cursor-pointer" onClick={() => setDetalle(t)}>
                      <td className="py-2.5 pr-3 whitespace-nowrap num text-[12.5px] font-medium">{t.numeroCompleto || '—'}</td>
                      <td className="py-2.5 pr-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(t.creada)}</td>
                      <td className="py-2.5 pr-3">
                        <div className="inline-flex items-center gap-1.5 text-[13px] font-medium">
                          {sedeName(t.origenSedeId)} <Icon.ArrowRight size={14} className="text-slate-400" /> {sedeName(t.destinoSedeId)}
                        </div>
                      </td>
                      <td className="py-2.5 pr-3 text-right num">{(t.lineas || []).length}</td>
                      <td className="py-2.5 pr-3">
                        <Badge size="sm" color={fase.color} dot>{fase.label}</Badge>
                        <div className="text-[10.5px] text-slate-400 mt-0.5">{det.label}</div>
                      </td>
                      <td className="py-2.5 pr-3 text-right">
                        <Icon.ChevRight size={15} className="inline text-slate-300" />
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {nueva ? <NuevaTransferencia sedes={sedes} almacenes={(db.ALMACENES || []).filter((a) => a.activo)} productos={db.PRODUCTOS || []} onClose={() => setNueva(false)} onSaved={reload} toast={toast} /> : null}
      {detalle ? <DetalleTransferencia t={detalle} editable={editable} sedeName={sedeName} onClose={() => setDetalle(null)} onSaved={reload} toast={toast} /> : null}
    </div>
  )
}

// Stepper visible de la máquina de estados.
function Stepper({ estado }) {
  const idx = ESTADOS.indexOf(estado)
  return (
    <div className="flex items-center gap-1">
      {ESTADOS.map((e, i) => (
        <div key={e} className="flex items-center gap-1 flex-1">
          <div className={`h-6 w-6 shrink-0 rounded-full inline-flex items-center justify-center text-[10px] font-semibold ${i < idx ? 'bg-teal-500 text-white' : i === idx ? 'bg-elerp-500 text-white' : 'bg-slate-100 dark:bg-slate-800 text-slate-400'}`}>
            {i < idx ? <Icon.Check size={12} /> : i + 1}
          </div>
          <span className={`text-[10px] font-medium hidden md:block ${i === idx ? 'text-slate-900 dark:text-slate-100' : 'text-slate-400'}`}>{ESTADO_META[e].label}</span>
          {i < ESTADOS.length - 1 ? <div className={`flex-1 h-px ${i < idx ? 'bg-teal-400' : 'bg-slate-200 dark:bg-slate-700'}`} /> : null}
        </div>
      ))}
    </div>
  )
}

function NuevaTransferencia({ sedes, almacenes, productos, onClose, onSaved, toast }) {
  const [origen, setOrigen] = useState(sedes[0]?.id || '')
  const [destino, setDestino] = useState(sedes[1]?.id || '')
  const [origenAlm, setOrigenAlm] = useState('')
  const [destinoAlm, setDestinoAlm] = useState('')
  const [lineas, setLineas] = useState([{ sku: '', cantidad: '' }])
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  // Almacenes disponibles por sede (vacío ⇒ el principal de la sede, que el backend
  // resuelve). Al cambiar de sede se limpia el almacén elegido.
  const almOrigen = (almacenes || []).filter((a) => a.sedeId === origen)
  const almDestino = (almacenes || []).filter((a) => a.sedeId === destino)
  useEffect(() => { setOrigenAlm('') }, [origen])
  useEffect(() => { setDestinoAlm('') }, [destino])

  const lineasValidas = lineas.filter((l) => l.sku && Number(l.cantidad) > 0)
  // La ubicación (almacén si se eligió, si no la sede) de origen y destino debe diferir:
  // así se permite una transferencia entre almacenes de la MISMA sede.
  const ubicOrigen = origenAlm || origen
  const ubicDestino = destinoAlm || destino
  const errRuta = !origen || !destino ? 'Elige origen y destino.'
    : ubicOrigen === ubicDestino ? 'El origen y el destino deben ser distintos (sede o almacén).' : ''
  const errLineas = lineasValidas.length === 0 ? 'Agrega al menos una línea con SKU y cantidad.' : ''
  const valid = !errRuta && !errLineas

  const setLinea = (i, k, v) => setLineas((ls) => ls.map((l, j) => (j === i ? { ...l, [k]: v } : l)))
  const addLinea = () => setLineas((ls) => [...ls, { sku: '', cantidad: '' }])
  const rmLinea = (i) => setLineas((ls) => ls.filter((_, j) => j !== i))

  const save = async () => {
    setTouched(true)
    if (!valid) return
    setBusy(true)
    try {
      await api.createTransferencia({
        origenSedeId: origen, destinoSedeId: destino,
        origenAlmacenId: origenAlm || undefined, destinoAlmacenId: destinoAlm || undefined,
        lineas: lineasValidas.map((l) => ({ sku: l.sku, cantidad: Number(l.cantidad) })),
      })
      toast({ title: 'Transferencia creada', body: `${lineasValidas.length} ítem(s) en borrador.` })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo crear', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} icon={<Icon.ArrowLeftRight size={18} />} title="Nueva transferencia" sub="Se crea en estado Borrador."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={save} loading={busy} icon={<Icon.Check size={16} />}>Crear en borrador</Button>
      </>}>
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Sede origen" required error={touched ? errRuta : ''}>
            <Select value={origen} onChange={(e) => setOrigen(e.target.value)}>
              <option value="">—</option>
              {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
            </Select>
          </Field>
          <Field label="Sede destino" required>
            <Select value={destino} onChange={(e) => setDestino(e.target.value)}>
              <option value="">—</option>
              {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
            </Select>
          </Field>
          {almOrigen.length ? (
            <Field label="Almacén origen" hint="vacío ⇒ principal">
              <Select value={origenAlm} onChange={(e) => setOrigenAlm(e.target.value)}>
                <option value="">Almacén principal</option>
                {almOrigen.map((a) => <option key={a.id} value={a.id}>{a.nombre}{a.principal ? ' ★' : ''}</option>)}
              </Select>
            </Field>
          ) : null}
          {almDestino.length ? (
            <Field label="Almacén destino" hint="vacío ⇒ principal">
              <Select value={destinoAlm} onChange={(e) => setDestinoAlm(e.target.value)}>
                <option value="">Almacén principal</option>
                {almDestino.map((a) => <option key={a.id} value={a.id}>{a.nombre}{a.principal ? ' ★' : ''}</option>)}
              </Select>
            </Field>
          ) : null}
        </div>

        <div>
          <div className="flex items-center justify-between mb-1.5">
            <label className="text-[12px] font-medium text-slate-500">Líneas</label>
            <Button size="sm" variant="ghost" icon={<Icon.Plus size={14} />} onClick={addLinea}>Agregar línea</Button>
          </div>
          <div className="space-y-2">
            {lineas.map((l, i) => (
              <div key={i} className="flex items-center gap-2">
                <Select className="flex-1" value={l.sku} onChange={(e) => setLinea(i, 'sku', e.target.value)}>
                  <option value="">Elige producto…</option>
                  {productos.map((p) => <option key={p.id} value={p.sku}>{p.sku} · {p.nombre}</option>)}
                </Select>
                <Input className="!w-28" type="number" min="0" step="0.01" placeholder="Cant." value={l.cantidad} onChange={(e) => setLinea(i, 'cantidad', e.target.value)} />
                <button onClick={() => rmLinea(i)} disabled={lineas.length === 1}
                  className="h-9 w-9 shrink-0 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-40">
                  <Icon.Trash size={15} />
                </button>
              </div>
            ))}
          </div>
          {touched && errLineas ? (
            <div className="mt-1.5 flex items-start gap-1 text-[11.5px] text-red-600 dark:text-red-400"><Icon.CircleAlert size={13} className="mt-0.5 shrink-0" /><span>{errLineas}</span></div>
          ) : null}
        </div>
      </div>
    </Modal>
  )
}

function DetalleTransferencia({ t, editable, sedeName, onClose, onSaved, toast }) {
  const [busy, setBusy] = useState(false)
  const [cancelando, setCancelando] = useState(false)
  const [motivo, setMotivo] = useState('')
  const [touched, setTouched] = useState(false)
  const idx = ESTADOS.indexOf(t.estado)
  const siguiente = idx >= 0 && idx < ESTADOS.length - 1 ? ESTADOS[idx + 1] : null
  const accionLabel = ACCION[t.estado]
  const puedeCancelar = editable && CANCELABLES.includes(t.estado)
  const yaEnDestino = t.estado === 'recibida' || t.estado === 'cerrada'

  const avanzar = async () => {
    if (!siguiente) return
    setBusy(true)
    try {
      await api.setTransferenciaEstado(t.id, siguiente)
      toast({ title: 'Estado actualizado', body: `Transferencia ahora ${ESTADO_META[siguiente].label.toLowerCase()}.` })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo avanzar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  const cancelar = async () => {
    setTouched(true)
    if (!motivo.trim()) return
    setBusy(true)
    try {
      await api.cancelarTransferencia(t.id, { motivo: motivo.trim() })
      toast({ title: 'Transferencia cancelada', body: t.estado === 'borrador' ? 'No había movimientos que revertir.' : 'El stock en tránsito se devolvió a la sede origen.' })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo cancelar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  const footer = cancelando ? (
    <>
      <Button variant="ghost" onClick={() => { setCancelando(false); setTouched(false) }} disabled={busy}>Volver</Button>
      <Button variant="destructive" onClick={cancelar} loading={busy} icon={<Icon.X size={16} />}>Confirmar cancelación</Button>
    </>
  ) : (
    <>
      <Button variant="ghost" onClick={onClose}>Cerrar</Button>
      {puedeCancelar ? <Button variant="destructive" onClick={() => setCancelando(true)} icon={<Icon.X size={16} />}>Cancelar transferencia</Button> : null}
      {editable && siguiente ? <Button onClick={avanzar} loading={busy} icon={<Icon.ArrowRight size={16} />}>{accionLabel}</Button> : null}
    </>
  )

  return (
    <Modal open onClose={onClose} icon={<Icon.ArrowLeftRight size={18} />}
      title={`${t.numeroCompleto ? t.numeroCompleto + ' · ' : ''}${sedeName(t.origenSedeId)} → ${sedeName(t.destinoSedeId)}`}
      sub={`${faseDe(t.estado).label} · Creada ${fmtDate(t.creada)} · salida en origen / entrada en destino (Kardex)`}
      footer={footer}>
      <div className="space-y-5">
        {t.estado === 'cancelada' ? (
          <div className="flex items-center gap-2"><Badge size="sm" color="rose" dot>Cancelada</Badge></div>
        ) : <Stepper estado={t.estado} />}

        {cancelando ? (
          <Field label="Motivo de la cancelación" required error={touched && !motivo.trim() ? 'El motivo es requerido.' : ''}>
            <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} placeholder="Ej. error de destino, pedido anulado…" autoFocus />
          </Field>
        ) : null}

        <div>
          <div className="text-[12px] font-medium text-slate-500 mb-2">Ítems ({(t.lineas || []).length})</div>
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
            {(t.lineas || []).map((l, i) => (
              <div key={i} className="flex items-center gap-3 px-3 py-2.5">
                <Icon.Package size={16} className="text-slate-400" />
                <div className="flex-1 min-w-0">
                  <div className="text-[13px] font-medium truncate">{l.nombre || l.sku}</div>
                  <div className="text-[11px] text-slate-400 num">{l.sku}</div>
                </div>
                <div className="num text-[13px] font-medium">{fmtNum(l.cantidad)}</div>
              </div>
            ))}
          </div>
        </div>

        {t.estado === 'cerrada' ? (
          <div className="flex items-center gap-2 text-[12.5px] text-emerald-700 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-900/20 rounded-lg px-3 py-2">
            <Icon.CircleCheck size={15} /> Transferencia cerrada. El stock ya se reflejó en ambas sedes.
          </div>
        ) : null}

        {yaEnDestino && editable && !cancelando ? (
          <div className="flex items-start gap-2 text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/40 rounded-lg px-3 py-2">
            <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" />
            <span>La mercancía ya está en el destino, así que no puede cancelarse. Para revertirla, crea una <strong>transferencia de vuelta</strong> {sedeName(t.destinoSedeId)} → {sedeName(t.origenSedeId)}; así el ledger sigue siendo append-only.</span>
          </div>
        ) : null}

        {t.estado === 'cancelada' ? (
          <div className="flex items-center gap-2 text-[12.5px] text-rose-700 dark:text-rose-400 bg-rose-50 dark:bg-rose-900/20 rounded-lg px-3 py-2">
            <Icon.CircleAlert size={15} /> Transferencia cancelada. El stock en tránsito se devolvió a la sede origen.
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
