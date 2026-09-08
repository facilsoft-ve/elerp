import { useState, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Modal, Empty, TableSkeleton, useToast } from '../components/primitives.jsx'
import { fmtNum, fmtDate } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'

// Sólo dueño/desarrollador reciben mercancía (mueve stock); contadora lee.
const puedeRecibir = (rol) => ['dueno', 'desarrollador'].includes(rol)
// Estados de una OC con pendiente por recibir.
const PENDIENTES = ['confirmada', 'recibida_parcial']

// Pendiente por línea = lo ordenado menos lo ya recibido.
const pendienteDe = (l) => Math.max(0, (Number(l.cantidad) || 0) - (Number(l.cantidadRecibida) || 0))
export const tienePendiente = (oc) => PENDIENTES.includes(oc.estado) && (oc.lineas || []).some((l) => pendienteDe(l) > 0)

/* Recepción — worklist. Sólo las OC confirmadas o parcialmente recibidas (las que
 * tienen algo pendiente). Por cada una se listan sus líneas con el pendiente, y un
 * botón "Recibir" que abre el modal de recepción (el mismo que usa Órdenes). */
export function ComprasRecepcion() {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const ordenes = db.ORDENES_COMPRA
  const [recibir, setRecibir] = useState(null)

  const gestiona = puedeRecibir(ui.rol)

  const pendientes = useMemo(
    () => (ordenes || []).filter(tienePendiente),
    [ordenes],
  )

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar la recepción"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  if (loading || ordenes === undefined) {
    return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={4} cols={4} /></div>
  }

  if (pendientes.length === 0) {
    return (
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        <Empty icon={<Icon.Inbox size={22} />} title="Nada por recibir"
          body="Cuando confirmes una orden de compra, aparecerá aquí para recibir la mercancía y sumarla al inventario." />
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="text-[13px] text-slate-500">
        {fmtNum(pendientes.length, 0)} orden(es) con mercancía pendiente por recibir.
      </div>
      {pendientes.map((oc) => (
        <div key={oc.id} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <div className="flex items-center gap-3 flex-wrap px-4 py-3 border-b border-slate-100 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="num text-[13px] font-semibold">{oc.numeroCompleto || '—'}</span>
                <Badge size="sm" color={oc.estado === 'recibida_parcial' ? 'amber' : 'blue'} dot>
                  {oc.estado === 'recibida_parcial' ? 'Recibida parcial' : 'Confirmada'}
                </Badge>
              </div>
              <div className="text-[12px] text-slate-500 truncate">{oc.proveedorNombre || 'Proveedor'} · {fmtDate(oc.creada)}</div>
            </div>
            {gestiona ? (
              <Button className="ml-auto" size="sm" variant="dinero" icon={<Icon.Inbox size={15} />} onClick={() => setRecibir(oc)}>Recibir</Button>
            ) : <span className="ml-auto text-[11.5px] text-slate-400">Sólo lectura</span>}
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800">
                  <th className="py-2 px-4 font-medium">Producto</th>
                  <th className="py-2 pr-3 font-medium text-right">Ordenado</th>
                  <th className="py-2 pr-3 font-medium text-right">Recibido</th>
                  <th className="py-2 pr-4 font-medium text-right">Pendiente</th>
                </tr>
              </thead>
              <tbody>
                {(oc.lineas || []).map((l, i) => {
                  const pend = pendienteDe(l)
                  return (
                    <tr key={l.sku || i} className={`border-b border-slate-100 dark:border-slate-800/70 ${pend === 0 ? 'opacity-55' : ''}`}>
                      <td className="py-2 px-4">
                        <div className="text-[13px] font-medium truncate">{l.nombre || l.sku}</div>
                        <div className="text-[11px] text-slate-400 num">{l.sku}</div>
                      </td>
                      <td className="py-2 pr-3 text-right num text-slate-500">{fmtNum(l.cantidad)}</td>
                      <td className="py-2 pr-3 text-right num text-slate-500">{fmtNum(l.cantidadRecibida || 0)}</td>
                      <td className="py-2 pr-4 text-right num font-medium">
                        {pend > 0 ? <span className="text-amber-700 dark:text-amber-400">{fmtNum(pend)}</span>
                          : <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400"><Icon.Check size={13} /> Completo</span>}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      ))}

      {recibir ? <ModalRecepcion orden={recibir} onClose={() => setRecibir(null)} onSaved={reload} toast={toast} /> : null}
    </div>
  )
}

/* Modal de recepción — reutilizado desde Recepción y desde la acción "Recibir" de
 * Órdenes. Por cada línea con pendiente, un input "cantidad a recibir" (default =
 * pendiente, máx = pendiente) con validación inline. Al confirmar anexa entradas
 * al inventario vía api.recibirOrdenCompra. */
export function ModalRecepcion({ orden, onClose, onSaved, toast }) {
  const lineasPend = useMemo(
    () => (orden.lineas || []).filter((l) => pendienteDe(l) > 0),
    [orden],
  )
  // Estado del formulario POR ÍNDICE de línea (no por SKU): una OC puede traer el
  // mismo producto en dos líneas y cada una se edita por separado (antes compartían
  // estado al indexar por SKU: editar una cambiaba la otra y la recepción chocaba).
  const [cant, setCant] = useState(() => lineasPend.map((l) => String(pendienteDe(l))))
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  const errorDe = (l, i) => {
    const pend = pendienteDe(l)
    const v = cant[i]
    if (v === '' || v == null) return ''
    const n = Number(v)
    if (isNaN(n) || n < 0) return 'Cantidad inválida.'
    if (n > pend) return `Máximo ${fmtNum(pend)} (lo pendiente).`
    return ''
  }

  // Al enviar se AGREGA por SKU (el backend imputa la recepción por SKU): dos
  // líneas del mismo producto suman su cantidad a recibir en una sola entrada.
  const lineasEnvio = useMemo(() => {
    const porSku = {}
    lineasPend.forEach((l, i) => {
      const n = Number(cant[i]) || 0
      if (n > 0) porSku[l.sku] = (porSku[l.sku] || 0) + n
    })
    return Object.entries(porSku).map(([sku, cantidad]) => ({ sku, cantidad }))
  }, [lineasPend, cant])

  const hayError = lineasPend.some((l, i) => errorDe(l, i))
  const errGlobal = hayError ? 'Corrige las cantidades marcadas.'
    : lineasEnvio.length === 0 ? 'Indica al menos una cantidad a recibir.' : ''

  const setLinea = (i, v) => setCant((c) => c.map((x, j) => (j === i ? v : x)))

  const recibir = async () => {
    setTouched(true)
    if (errGlobal) return
    setBusy(true)
    try {
      await api.recibirOrdenCompra(orden.id, { lineas: lineasEnvio })
      const totalItems = lineasEnvio.reduce((a, l) => a + l.cantidad, 0)
      toast({ title: 'Mercancía recibida', body: `${orden.numeroCompleto || 'OC'}: ${fmtNum(totalItems)} unidad(es) sumadas al inventario.` })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo recibir', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Inbox size={18} />}
      title="Recibir mercancía" sub={`${orden.numeroCompleto || 'Orden de compra'} · ${orden.proveedorNombre || 'Proveedor'}`}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant="dinero" onClick={recibir} loading={busy} disabled={!!errGlobal} title={errGlobal || 'Sumar al inventario'} icon={<Icon.Check size={16} />}>Confirmar recepción</Button>
      </>}>
      <div className="space-y-4">
        <div className="flex items-start gap-2 text-[12.5px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>Al confirmar, las cantidades recibidas se suman como <strong>entradas</strong> al inventario de la sede de la orden. Puedes recibir en varias entregas: lo que no recibas ahora queda pendiente.</span>
        </div>

        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <table className="w-full text-sm min-w-[520px]">
            <thead>
              <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                <th className="py-2 px-3 font-medium">Producto</th>
                <th className="py-2 pr-3 font-medium text-right w-24">Pendiente</th>
                <th className="py-2 pr-3 font-medium text-right w-40">A recibir</th>
              </tr>
            </thead>
            <tbody>
              {lineasPend.map((l, i) => {
                const pend = pendienteDe(l)
                const err = errorDe(l, i)
                return (
                  <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2 px-3 align-top">
                      <div className="text-[13px] font-medium">{l.nombre || l.sku}</div>
                      <div className="text-[11px] text-slate-400 num">{l.sku}</div>
                    </td>
                    <td className="py-2 pr-3 text-right num text-slate-500 align-top">{fmtNum(pend)}</td>
                    <td className="py-2 pr-3 align-top">
                      <input type="number" min="0" max={pend} step="0.01" value={cant[i] ?? ''}
                        onChange={(e) => setLinea(i, e.target.value)} onBlur={() => setTouched(true)}
                        className={`w-32 h-8 text-right px-2 rounded-lg border bg-white dark:bg-slate-900 text-sm num ring-focus ml-auto block
                          ${touched && err ? 'border-red-400 focus:ring-red-300' : 'border-slate-200 dark:border-slate-700'}`} />
                      {touched && err ? <div className="mt-1 text-right text-[11px] text-red-600 dark:text-red-400">{err}</div> : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>

        {touched && errGlobal ? (
          <div className="flex items-start gap-1.5 text-[12px] text-amber-700 dark:text-amber-400">
            <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{errGlobal}</span>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
