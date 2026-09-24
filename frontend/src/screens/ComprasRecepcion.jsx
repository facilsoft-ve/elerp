import { useState, useMemo, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Modal, Empty, TableSkeleton, useToast, Field, Select } from '../components/primitives.jsx'
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
  // Lote y vencimiento por línea. Solo los piden los productos con trazabilidad,
  // y se piden ACÁ porque es el único momento en que alguien tiene la caja
  // delante con la etiqueta: preguntarlo después es pedir que lo inventen.
  const [lote, setLote] = useState(() => lineasPend.map(() => ''))
  const [venc, setVenc] = useState(() => lineasPend.map(() => ''))
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)
  // El catálogo vive en `db`, no en la raíz de useData(): destructurar PRODUCTOS
  // directamente daba undefined y la columna se pintaba vacía SIN fallar — el
  // producto parecía no llevar lotes.
  const { db } = useData()
  // Ubicaciones del almacén al que entra la mercancía. Si el almacén no está
  // dividido, la lista viene vacía y la columna no aparece: no se pide un dato que
  // no existe.
  const [ubis, setUbis] = useState([])
  const [ubiSel, setUbiSel] = useState('')
  // UNIDAD EN LA QUE SE CUENTA lo que llegó, por línea. Vacío = la del producto,
  // que es el caso de siempre. Se ofrecen solo las de su misma naturaleza con
  // equivalencia declarada: un desplegable que deja elegir litros para algo que se
  // lleva en kilos solo sirve para que alguien lo elija.
  const [unidadSel, setUnidadSel] = useState(() => lineasPend.map(() => ''))
  const [compat, setCompat] = useState({}) // unidad base → [{simbolo, nombre, factor}]
  useEffect(() => {
    let vivo = true
    const alm = (db?.ALMACENES || []).find((a) => a.sedeId === orden.sedeId && a.principal && a.activo)
    if (!alm) { setUbis([]); return undefined }
    api.ubicaciones(alm.id)
      .then((r) => { if (vivo) setUbis((r?.ubicaciones || []).filter((u) => u.activa)) })
      .catch(() => { if (vivo) setUbis([]) })
    return () => { vivo = false }
  }, [db, orden.sedeId])

  // Se pide una vez por unidad base distinta de la orden, no por línea: una orden
  // de veinte renglones de la misma bodega suele tener una o dos.
  useEffect(() => {
    let vivo = true
    const bases = [...new Set(lineasPend.map((l) => unidadBaseDe(l.sku)).filter(Boolean))]
    Promise.all(bases.map((b) =>
      api.unidadesCompatibles(b).then((r) => [b, r?.unidades || []]).catch(() => [b, []])
    )).then((pares) => { if (vivo) setCompat(Object.fromEntries(pares)) })
    return () => { vivo = false }
  }, [db, orden.id]) // eslint-disable-line react-hooks/exhaustive-deps

  const unidadBaseDe = (sku) => (db?.PRODUCTOS || []).find((x) => x.sku === sku)?.unidadBase || ''

  // Cuánto entra al almacén con lo tecleado. El servidor hace la conversión de
  // verdad; esto solo la ANTICIPA, para que nadie teclee a ciegas y para validar
  // contra lo pendiente en la unidad correcta.
  const equivalenteDe = (l, i) => {
    const n = Number(cant[i])
    if (!Number.isFinite(n) || n <= 0) return null
    const u = unidadSel[i]
    const base = unidadBaseDe(l.sku)
    if (!u || u === base) return null
    const lista = compat[base] || []
    const fu = lista.find((x) => x.simbolo === u)?.factor
    const fb = lista.find((x) => x.simbolo === base)?.factor
    if (!fu || !fb) return null
    return Math.round((n * fu / fb) * 10000) / 10000
  }

  const trazaDe = (sku) => {
    const p = (db?.PRODUCTOS || []).find((x) => x.sku === sku)
    return { lote: !!p?.requiereLote, venc: !!p?.controlaVencimiento }
  }

  const errorDe = (l, i) => {
    const pend = pendienteDe(l)
    const v = cant[i]
    if (v === '' || v == null) return ''
    const n = Number(v)
    if (isNaN(n) || n < 0) return 'Cantidad inválida.'
    // Se compara EN LA UNIDAD DEL PRODUCTO: si se teclean sacos y lo pendiente son
    // kilos, comparar los números crudos dejaría pasar una recepción de más que el
    // servidor rechazaría después, sin decir por qué en esta pantalla.
    const enBase = equivalenteDe(l, i)
    const aComparar = enBase == null ? n : enBase
    if (aComparar > pend) {
      return enBase == null
        ? `Máximo ${fmtNum(pend)} (lo pendiente).`
        : `Son ${fmtNum(enBase)} ${unidadBaseDe(l.sku)}, y quedan ${fmtNum(pend)}.`
    }
    return ''
  }

  // El lote se valida aparte de la cantidad: son dos errores distintos y
  // mezclarlos dejaría al usuario buscando cuál de los dos campos está mal.
  const errorLoteDe = (l, i) => {
    if (!(Number(cant[i]) > 0)) return ''
    const t = trazaDe(l.sku)
    if (t.lote && !String(lote[i] || '').trim()) return 'Este producto se lleva por lotes.'
    if (t.venc && !venc[i]) return 'Indica la fecha de vencimiento.'
    return ''
  }

  // Al enviar se AGREGA por SKU (el backend imputa la recepción por SKU): dos
  // líneas del mismo producto suman su cantidad a recibir en una sola entrada.
  const lineasEnvio = useMemo(() => {
    // Una línea por (SKU, lote): el servidor agrega así, y es lo que permite que
    // una misma entrega llegue partida en dos lotes —lo normal cuando el proveedor
    // completa el pedido con lo que tiene—. Fusionar por SKU obligaría a hacer dos
    // recepciones, o haría que un lote pisara al otro sin decirlo.
    const porClave = {}
    lineasPend.forEach((l, i) => {
      const n = Number(cant[i]) || 0
      if (n <= 0) return
      const lt = String(lote[i] || '').trim()
      const k = l.sku + '\u0000' + lt
      const prev = porClave[k] || {
        sku: l.sku, cantidad: 0, lote: lt, vencimiento: venc[i] || '',
        ubicacionId: ubiSel, unidad: unidadSel[i] || '',
      }
      prev.cantidad += n
      porClave[k] = prev
    })
    return Object.values(porClave)
  }, [lineasPend, cant, lote, venc, ubiSel, unidadSel])

  const hayError = lineasPend.some((l, i) => errorDe(l, i))
  const hayErrorLote = lineasPend.some((l, i) => errorLoteDe(l, i))
  const errGlobal = hayError ? 'Corrige las cantidades marcadas.'
    : hayErrorLote ? 'Falta el lote o el vencimiento de algún producto.'
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

        {/* La ubicación es de TODA la entrega, no por línea: quien descarga un
            camión lo pone en un sitio, y pedirlo renglón a renglón haría que nadie
            lo rellenara. Repartir una entrega entre dos ubicaciones se hace en dos
            recepciones, igual que con los lotes. */}
        {ubis.length ? (
          <Field label="¿Dónde se ubica?" hint="opcional · si no lo indicas queda «sin ubicar» en el almacén">
            <Select value={ubiSel} onChange={(e) => setUbiSel(e.target.value)}>
              <option value="">Sin ubicar (el almacén, sin más detalle)</option>
              {ubis.map((u) => <option key={u.id} value={u.id}>{u.codigo} · {u.nombre}</option>)}
            </Select>
          </Field>
        ) : null}

        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <table className="w-full text-sm min-w-[520px]">
            <thead>
              <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                <th className="py-2 px-3 font-medium">Producto</th>
                <th className="py-2 pr-3 font-medium text-right w-24">Pendiente</th>
                <th className="py-2 pr-3 font-medium text-right w-40">A recibir</th>
                <th className="py-2 pr-3 font-medium w-56">Lote / vencimiento</th>
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
                      <div className="flex items-center justify-end gap-1.5">
                        <input type="number" min="0" step="0.01" value={cant[i] ?? ''}
                          onChange={(e) => setLinea(i, e.target.value)} onBlur={() => setTouched(true)}
                          className={`w-28 h-8 text-right px-2 rounded-lg border bg-white dark:bg-slate-900 text-sm num ring-focus
                            ${touched && err ? 'border-red-400 focus:ring-red-300' : 'border-slate-200 dark:border-slate-700'}`} />
                        {/* El selector SOLO sale si el producto tiene a qué convertir.
                            Ofrecerlo vacío haría pensar que falta configurar algo. */}
                        {(compat[unidadBaseDe(l.sku)] || []).length > 1 ? (
                          <select value={unidadSel[i] ?? ''} title="¿En qué unidad viene?"
                            onChange={(e) => setUnidadSel((c) => c.map((x, j) => (j === i ? e.target.value : x)))}
                            className="h-8 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-1.5 text-[12px] max-w-[7.5rem]">
                            <option value="">{unidadBaseDe(l.sku) || 'unidad'}</option>
                            {(compat[unidadBaseDe(l.sku)] || [])
                              .filter((u) => u.simbolo !== unidadBaseDe(l.sku))
                              .map((u) => <option key={u.simbolo} value={u.simbolo}>{u.simbolo}</option>)}
                          </select>
                        ) : null}
                      </div>
                      {equivalenteDe(l, i) != null ? (
                        <div className="mt-1 text-right text-[11px] text-slate-500 dark:text-slate-400">
                          = {fmtNum(equivalenteDe(l, i))} {unidadBaseDe(l.sku)} al almacén
                        </div>
                      ) : null}
                      {touched && err ? <div className="mt-1 text-right text-[11px] text-red-600 dark:text-red-400">{err}</div> : null}
                    </td>
                    {/* Solo aparece para los productos que lo exigen: un catálogo de
                        mercancía corriente no tiene por qué ver estos campos. */}
                    <td className="py-2 pr-3 align-top">
                      {(() => {
                        const t = trazaDe(l.sku)
                        if (!t.lote) return <span className="text-[11.5px] text-slate-400">—</span>
                        const errL = errorLoteDe(l, i)
                        return (
                          <div className="space-y-1">
                            <input type="text" value={lote[i] ?? ''} placeholder="Lote"
                              onChange={(e) => setLote((c) => c.map((x, j) => (j === i ? e.target.value : x)))}
                              onBlur={() => setTouched(true)}
                              className={`w-full h-8 px-2 rounded-lg border bg-white dark:bg-slate-900 text-sm num ring-focus
                                ${touched && errL ? 'border-red-400 focus:ring-red-300' : 'border-slate-200 dark:border-slate-700'}`} />
                            {t.venc ? (
                              <input type="date" value={venc[i] ?? ''}
                                onChange={(e) => setVenc((c) => c.map((x, j) => (j === i ? e.target.value : x)))}
                                onBlur={() => setTouched(true)}
                                className={`w-full h-8 px-2 rounded-lg border bg-white dark:bg-slate-900 text-sm num ring-focus
                                  ${touched && errL ? 'border-red-400 focus:ring-red-300' : 'border-slate-200 dark:border-slate-700'}`} />
                            ) : null}
                            {touched && errL ? <div className="text-[11px] text-red-600 dark:text-red-400">{errL}</div> : null}
                          </div>
                        )
                      })()}
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
