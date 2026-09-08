import { useState, useMemo, useRef, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, VistaDetalle, Modal, Empty, TableSkeleton, useToast, Field } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { porCodigo } from '../lib/precio.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { puedeGestionar } from './ComprasOrdenes.jsx'

/* Solicitudes de presupuesto (RFQ). El paso PREVIO a la orden de compra: se pide
 * cotización a uno o varios proveedores, se comparan sus respuestas y se convierte
 * la elegida en orden. Máquina de estados:
 *   borrador → enviada → respondida → cerrada   (o cancelada)
 * No toca stock ni contabilidad: solo negocia precios. La conversión reusa la OC. */

const ESTADO_META = {
  borrador: { label: 'Borrador', color: 'slate' },
  enviada: { label: 'Enviada', color: 'blue' },
  respondida: { label: 'Respondida', color: 'teal' },
  cerrada: { label: 'Cerrada', color: 'emerald' },
  cancelada: { label: 'Cancelada', color: 'rose' },
}

const FILTROS = [
  { value: 'todos', label: 'Todos los estados' },
  { value: 'borrador', label: 'Borradores' },
  { value: 'enviada', label: 'Enviadas' },
  { value: 'respondida', label: 'Respondidas' },
  { value: 'cerrada', label: 'Cerradas' },
]

export function ComprasSolicitudes({ navigate }) {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const solicitudes = db.SOLICITUDES_COMPRA

  const [q, setQ] = useState('')
  const [filtro, setFiltro] = useState('todos')
  const [nueva, setNueva] = useState(false)
  const [editar, setEditar] = useState(null)
  const [detalleId, setDetalleId] = useState(null)

  const gestiona = puedeGestionar(ui.rol)

  // El detalle se resuelve por id desde la lista viva, para que tras registrar una
  // respuesta o convertir, la vista refleje el estado nuevo sin cerrarse.
  const detalle = useMemo(
    () => (detalleId ? (solicitudes || []).find((s) => s.id === detalleId) || null : null),
    [detalleId, solicitudes],
  )

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return (solicitudes || []).filter((s) => {
      const okQ = !term
        || (s.numeroCompleto || '').toLowerCase().includes(term)
        || (s.notas || '').toLowerCase().includes(term)
        || (s.proveedores || []).some((p) => (p.proveedorNombre || '').toLowerCase().includes(term))
      const okF = filtro === 'todos' || s.estado === filtro
      return okQ && okF
    })
  }, [solicitudes, q, filtro])

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las solicitudes de presupuesto"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  if (nueva || editar) {
    return <FormSolicitud solicitud={editar} onVolver={() => { setNueva(false); setEditar(null) }} onSaved={reload} toast={toast} />
  }

  if (detalle) {
    return <DetalleSolicitud sol={detalle} gestiona={gestiona} onVolver={() => setDetalleId(null)}
      onEditar={() => { setEditar(detalle); setDetalleId(null) }} onSaved={reload} toast={toast} navigate={navigate} />
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por número, proveedor o nota…" value={q} onChange={(e) => setQ(e.target.value)} />
        <Select className="!w-52" value={filtro} onChange={(e) => setFiltro(e.target.value)}>
          {FILTROS.map((f) => <option key={f.value} value={f.value}>{f.label}</option>)}
        </Select>
        {gestiona ? <Button className="ml-auto" icon={<Icon.Plus size={16} />} onClick={() => setNueva(true)}>Nueva solicitud</Button> : null}
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || solicitudes === undefined ? (
          <div className="p-4"><TableSkeleton rows={6} cols={5} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.ClipboardList size={22} />}
            title={q || filtro !== 'todos' ? 'Sin resultados' : 'Aún no hay solicitudes de presupuesto'}
            body={q || filtro !== 'todos' ? 'Prueba con otro término o filtro.' : 'Pide presupuesto a varios proveedores, compara sus precios y convierte la mejor oferta en una orden de compra.'}
            cta={gestiona && !q && filtro === 'todos' ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNueva(true)}>Nueva solicitud</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 pr-3 font-medium">Número</th>
                  <th className="py-2.5 pr-3 font-medium text-center">Estado</th>
                  <th className="py-2.5 pr-3 font-medium text-center">Proveedores</th>
                  <th className="py-2.5 pr-3 font-medium">Notas</th>
                  <th className="py-2.5 pr-3 font-medium">Fecha</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((s) => {
                  const em = ESTADO_META[s.estado] || { label: s.estado, color: 'slate' }
                  const provs = s.proveedores || []
                  const respondidos = provs.filter((p) => p.respondida).length
                  return (
                    <tr key={s.id} className={`border-b border-slate-100 dark:border-slate-800/70 row-hover cursor-pointer ${s.estado === 'cancelada' ? 'opacity-60' : ''}`} onClick={() => setDetalleId(s.id)}>
                      <td className="py-2.5 pr-3 num text-[12.5px] font-medium">{s.numeroCompleto || '—'}</td>
                      <td className="py-2.5 pr-3 text-center"><Badge size="sm" color={em.color} dot>{em.label}</Badge></td>
                      <td className="py-2.5 pr-3 text-center text-[12.5px] text-slate-500 num">
                        {provs.length ? <>{respondidos}/{provs.length} <span className="text-slate-400">resp.</span></> : '—'}
                      </td>
                      <td className="py-2.5 pr-3"><div className="text-[13px] truncate max-w-[320px] text-slate-500">{s.notas || '—'}</div></td>
                      <td className="py-2.5 pr-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(s.fecha || s.creada)}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} solicitud(es)</div> : null}
    </div>
  )
}

/* Alta/edición de una solicitud — formulario a PANTALLA COMPLETA. A diferencia de
 * la orden, aquí NO se captura precio: solo qué productos y cuánto se pide cotizar,
 * y a qué proveedores. El precio lo pone cada proveedor al responder. */
function FormSolicitud({ solicitud, onVolver, onSaved, toast }) {
  const { db } = useData()
  // Los combos son un empaquetado de venta; no se compran (el backend rechaza una
  // OC/solicitud con SKU combo). Se excluyen del selector para no ofrecerlos.
  const productos = (db.PRODUCTOS || []).filter((p) => !p.esCombo)
  const proveedores = (db.PROVEEDORES || []).filter((p) => p.activo)
  const sedes = db.SEDES || []
  const esEdicion = !!solicitud

  const [sedeId, setSedeId] = useState(solicitud?.sedeId || db.SEDE_ACTIVA?.id || sedes[0]?.id || '')
  const [notas, setNotas] = useState(solicitud?.notas || '')
  // Líneas: { sku, nombre, cantidad }
  const [lineas, setLineas] = useState(() => (solicitud?.lineas || []).map((l) => ({ sku: l.sku, nombre: l.nombre, cantidad: l.cantidad })))
  const [provSel, setProvSel] = useState(() => new Set((solicitud?.proveedores || []).map((p) => p.proveedorId)))
  const [q, setQ] = useState('')
  const [sel, setSel] = useState(0)
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)
  const searchRef = useRef(null)

  const resultados = useMemo(() => {
    const term = q.trim().toLowerCase()
    if (!term) return []
    return productos
      .filter((p) => p.nombre.toLowerCase().includes(term)
        || (p.sku || '').toLowerCase().includes(term)
        || (p.codigoBarras || '').includes(term))
      .slice(0, 8)
  }, [q, productos])

  useEffect(() => { setSel(0) }, [q])

  const addProducto = (p) => {
    if (!p) return
    setLineas((c) => {
      const i = c.findIndex((l) => l.sku === p.sku)
      if (i >= 0) return c.map((l, j) => (j === i ? { ...l, cantidad: l.cantidad + 1 } : l))
      return [...c, { sku: p.sku, nombre: p.nombre, cantidad: 1 }]
    })
    setQ('')
    searchRef.current?.focus()
  }

  const onSearchKey = (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setSel((s) => Math.min(s + 1, resultados.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)) }
    else if (e.key === 'Enter') {
      e.preventDefault()
      const exacto = porCodigo(productos, q)
      addProducto(exacto || resultados[sel])
    }
  }

  const setLinea = (sku, cantidad) => setLineas((c) => c.map((l) => (l.sku === sku ? { ...l, cantidad } : l)))
  const rmLinea = (sku) => setLineas((c) => c.filter((l) => l.sku !== sku))
  const toggleProv = (id) => setProvSel((c) => {
    const n = new Set(c)
    n.has(id) ? n.delete(id) : n.add(id)
    return n
  })

  const errSede = !sedeId ? 'Elige la sede que recibiría la mercancía.' : ''
  const errLineas = lineas.length === 0 ? 'Agrega al menos un producto a cotizar.'
    : lineas.some((l) => !(Number(l.cantidad) > 0)) ? 'Todas las cantidades deben ser mayores a 0.'
    : ''
  const err = errSede || errLineas

  const guardar = async () => {
    setTouched(true)
    if (err) return
    setBusy(true)
    try {
      const body = {
        sedeId, notas: notas.trim() || undefined,
        lineas: lineas.map((l) => ({ sku: l.sku, cantidad: Number(l.cantidad) })),
        proveedores: [...provSel],
      }
      if (esEdicion) {
        await api.actualizarSolicitudCompra(solicitud.id, body)
        toast({ title: 'Solicitud actualizada', body: solicitud.numeroCompleto || '' })
      } else {
        const s = await api.crearSolicitudCompra(body)
        toast({ title: 'Solicitud creada', body: s?.numeroCompleto ? `${s.numeroCompleto} en borrador.` : 'Guardada en borrador.' })
      }
      await onSaved()
      onVolver()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <button onClick={onVolver}
          className="h-9 px-2.5 inline-flex items-center gap-1.5 rounded-lg text-[13px] font-medium text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus">
          <Icon.ChevLeft size={16} /> Volver
        </button>
        <div className="flex-1 min-w-0">
          <h2 className="font-display text-[20px] font-bold tracking-tight truncate">{esEdicion ? `Editar ${solicitud.numeroCompleto || 'solicitud'}` : 'Nueva solicitud de presupuesto'}</h2>
          <div className="text-[12px] text-slate-500">Se guarda en borrador; luego la envías a los proveedores para pedir sus precios.</div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" onClick={onVolver}>Cancelar</Button>
          <Button onClick={guardar} loading={busy} disabled={!!err} title={err || 'Guardar en borrador'} icon={<Icon.Check size={16} />}>Guardar solicitud</Button>
        </div>
      </div>

      {/* Cabecera */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 mb-4">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Field label="Sede que recibiría" required error={touched ? errSede : ''}>
            <Select value={sedeId} onChange={(e) => setSedeId(e.target.value)}>
              <option value="">Elige sede…</option>
              {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
            </Select>
          </Field>
        </div>
      </div>

      {/* Buscador + líneas */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 mb-4">
        <label className="block text-[12px] font-medium text-slate-500 mb-1">Productos a cotizar</label>
        <div className="relative mb-3">
          <Input ref={searchRef} icon={<Icon.Search size={15} />} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onSearchKey}
            placeholder="Buscar por nombre, SKU o código… (Enter para agregar)" />
          {q && resultados.length ? (
            <div className="absolute z-20 mt-1 w-full max-w-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-modal overflow-hidden">
              {resultados.map((p, i) => (
                <button key={p.id || p.sku} onClick={() => addProducto(p)} onMouseEnter={() => setSel(i)}
                  className={`w-full flex items-center gap-3 px-3 h-11 text-left ${i === sel ? 'bg-elerp-50 dark:bg-elerp-900/40' : ''}`}>
                  <Icon.Package size={16} className="text-slate-400" />
                  <div className="flex-1 min-w-0">
                    <div className="text-[13px] font-medium truncate">{p.nombre}</div>
                    <div className="text-[11px] text-slate-400 num">{p.sku}</div>
                  </div>
                </button>
              ))}
            </div>
          ) : q && !resultados.length ? (
            <div className="absolute z-20 mt-1 w-full max-w-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-modal px-4 py-3 text-center text-[13px] text-slate-500">Sin resultados para “{q}”.</div>
          ) : null}
        </div>

        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          {lineas.length === 0 ? (
            <div className="px-4 py-10 text-center text-[13px] text-slate-500">Sin líneas todavía. Busca un producto arriba y presiona Enter para agregarlo.</div>
          ) : (
            <table className="w-full text-sm min-w-[520px]">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2 px-3 font-medium">Producto</th>
                  <th className="py-2 pr-3 font-medium text-center w-24">Cantidad</th>
                  <th className="py-2 pr-3 w-9"></th>
                </tr>
              </thead>
              <tbody>
                {lineas.map((l) => (
                  <tr key={l.sku} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2 px-3">
                      <div className="font-medium text-[13px]">{l.nombre}</div>
                      <div className="text-[11px] text-slate-400 num">{l.sku}</div>
                    </td>
                    <td className="py-2 pr-3">
                      <input type="number" min="0" step="1" value={l.cantidad} onChange={(e) => setLinea(l.sku, Number(e.target.value))}
                        className="w-20 h-8 text-center rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num ring-focus mx-auto block" />
                    </td>
                    <td className="py-2 pr-3 text-right">
                      <button onClick={() => rmLinea(l.sku)} className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800"><Icon.Trash size={15} /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* Proveedores a los que se pide + notas */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
          <div className="text-[12px] font-medium text-slate-500 mb-2">Proveedores a los que pedir presupuesto <span className="text-slate-400 font-normal">· {provSel.size} seleccionado(s)</span></div>
          {proveedores.length === 0 ? (
            <div className="text-[12.5px] text-amber-700 dark:text-amber-400">No hay proveedores activos. Crea uno en la pestaña Proveedores.</div>
          ) : (
            <div className="space-y-1 max-h-64 overflow-y-auto">
              {proveedores.map((p) => (
                <label key={p.id} className="flex items-center gap-2.5 px-2.5 py-2 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800/60 cursor-pointer">
                  <input type="checkbox" checked={provSel.has(p.id)} onChange={() => toggleProv(p.id)}
                    className="h-4 w-4 rounded border-slate-300 dark:border-slate-600 text-elerp-500 ring-focus" />
                  <div className="min-w-0">
                    <div className="text-[13px] font-medium truncate">{p.nombre}</div>
                    {p.documento ? <div className="text-[11px] text-slate-400 num">{p.documento}</div> : null}
                  </div>
                </label>
              ))}
            </div>
          )}
          <div className="mt-2 text-[11px] text-slate-400 leading-snug">Puedes guardar sin proveedores y agregarlos después; para enviar la solicitud hace falta al menos uno.</div>
        </div>

        <Field label="Notas" hint="opcional · qué necesitas cotizar, plazos, condiciones…">
          <textarea value={notas} onChange={(e) => setNotas(e.target.value)} rows={4} placeholder="Ej: Reposición mensual. Indicar disponibilidad y tiempo de entrega."
            className="w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm ring-focus resize-y" />
        </Field>
      </div>

      {touched && err ? (
        <div className="mt-3 flex items-start gap-1.5 text-[12px] text-amber-700 dark:text-amber-400">
          <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{err}</span>
        </div>
      ) : null}
    </div>
  )
}

// Detalle de una solicitud: líneas, proveedores invitados, tabla comparativa de
// respuestas (con el más barato resaltado) y las acciones según el estado.
function DetalleSolicitud({ sol, gestiona, onVolver, onEditar, onSaved, toast, navigate }) {
  const [respuesta, setRespuesta] = useState(null) // proveedor al que se le cargan precios
  const [busy, setBusy] = useState('')
  const em = ESTADO_META[sol.estado] || { label: sol.estado, color: 'slate' }
  const provs = sol.proveedores || []
  const puedeEditar = gestiona && sol.estado === 'borrador'
  const puedeResponder = gestiona && (sol.estado === 'enviada' || sol.estado === 'respondida')
  const abierta = sol.estado !== 'cerrada' && sol.estado !== 'cancelada'

  const accion = async (fn, ok, kind) => {
    setBusy(kind)
    try {
      await fn()
      toast({ title: ok, body: sol.numeroCompleto || '' })
      await onSaved()
    } catch (e) {
      toast({ title: 'No se pudo completar', body: e?.message || 'Error', kind: 'error' })
    } finally {
      setBusy('')
    }
  }

  const convertir = async (proveedorId) => {
    setBusy('conv:' + proveedorId)
    try {
      const res = await api.convertirSolicitudEnOrden(sol.id, proveedorId)
      const oc = res?.orden
      toast({ title: 'Orden de compra creada', body: oc?.numeroCompleto ? `${oc.numeroCompleto} en borrador, lista para confirmar.` : 'Generada desde la solicitud.' })
      await onSaved()
      // Navega a Órdenes de compra: la nueva OC en borrador aparece en la lista.
      if (navigate) navigate('compras:ordenes')
    } catch (e) {
      toast({ title: 'No se pudo convertir', body: e?.message || 'Error', kind: 'error' })
      setBusy('')
    }
  }

  const acciones = (() => {
    if (!gestiona) return null
    const botones = []
    if (sol.estado === 'borrador') {
      botones.push(<Button key="ed" variant="secondary" icon={<Icon.Pencil size={15} />} onClick={onEditar}>Editar</Button>)
      botones.push(<Button key="env" icon={<Icon.Send size={16} />} loading={busy === 'enviar'} onClick={() => accion(() => api.enviarSolicitudCompra(sol.id), 'Solicitud enviada', 'enviar')}>Enviar a proveedores</Button>)
    }
    if (abierta && sol.estado !== 'borrador') {
      botones.push(<Button key="cer" variant="ghost" icon={<Icon.CircleX size={16} />} loading={busy === 'cerrar'} onClick={() => accion(() => api.cerrarSolicitudCompra(sol.id), 'Solicitud cerrada', 'cerrar')}>Cerrar</Button>)
    }
    return botones.length ? <>{botones}</> : null
  })()

  return (
    <VistaDetalle onVolver={onVolver} icon={<Icon.ClipboardList size={18} />}
      titulo={sol.numeroCompleto || 'Solicitud de presupuesto'} sub={`${em.label} · ${fmtDate(sol.fecha || sol.creada)}`} acciones={acciones} maxWidth="max-w-6xl">
      <div className="space-y-4">
        {/* Estado + notas */}
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 flex flex-wrap items-start gap-x-6 gap-y-3">
          <div>
            <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Estado</div>
            <Badge color={em.color} dot>{em.label}</Badge>
          </div>
          <div className="flex-1 min-w-[220px]">
            <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Notas</div>
            <div className="text-[12.5px] text-slate-600 dark:text-slate-300 whitespace-pre-wrap">{sol.notas || '—'}</div>
          </div>
          {sol.estado === 'cerrada' && sol.ordenGeneradaId ? (
            <div className="flex items-center gap-2 text-[12.5px] text-emerald-700 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-900/20 rounded-lg px-3 py-2">
              <Icon.CircleCheck size={15} /> Convertida en orden de compra.
            </div>
          ) : null}
        </div>

        {/* Tabla comparativa: productos × proveedores */}
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
          <div className="flex items-center gap-2 mb-3">
            <Icon.ArrowLeftRight size={15} className="text-slate-400" />
            <div className="text-[13px] font-semibold">Comparativa de presupuestos</div>
            <div className="text-[11.5px] text-slate-400">· el más barato por línea va resaltado</div>
          </div>
          <TablaComparativa sol={sol} gestiona={gestiona} abierta={abierta} busy={busy}
            onResponder={puedeResponder ? (p) => setRespuesta(p) : null}
            onConvertir={(id) => convertir(id)} />
        </div>

        {/* Proveedores invitados sin respuesta aún — acción de cargar respuesta */}
        {puedeResponder ? (
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
            <div className="text-[12px] font-medium text-slate-500 mb-2">Cargar la respuesta de un proveedor</div>
            <div className="flex flex-wrap gap-2">
              {provs.map((p) => (
                <button key={p.proveedorId} onClick={() => setRespuesta(p)}
                  className="inline-flex items-center gap-1.5 h-8 px-3 rounded-lg border border-slate-200 dark:border-slate-700 text-[12.5px] hover:bg-slate-50 dark:hover:bg-slate-800">
                  {p.respondida ? <Icon.Pencil size={13} className="text-teal-600" /> : <Icon.Plus size={13} className="text-slate-400" />}
                  {p.proveedorNombre}
                  {p.respondida ? <span className="text-teal-600 dark:text-teal-400">· respondió</span> : <span className="text-slate-400">· pendiente</span>}
                </button>
              ))}
            </div>
          </div>
        ) : null}
      </div>

      {respuesta ? <ModalRespuesta sol={sol} proveedor={respuesta} onClose={() => setRespuesta(null)} onSaved={onSaved} toast={toast} /> : null}
    </VistaDetalle>
  )
}

// Tabla comparativa: filas = productos solicitados; columnas = proveedores. Cada
// celda muestra el precio unitario cotizado; el más barato de la fila se resalta.
// La última fila es el total por proveedor, con el mejor total resaltado y (si la
// solicitud sigue abierta) el botón para convertir esa oferta en orden de compra.
function TablaComparativa({ sol, gestiona, abierta, busy, onResponder, onConvertir }) {
  const lineas = sol.lineas || []
  const provs = sol.proveedores || []

  // Precio por (proveedor, sku) y mínimo por sku para el resaltado.
  const precio = (p, sku) => {
    const l = (p.lineas || []).find((x) => x.sku === sku)
    return l ? Number(l.precioUnitario) : null
  }
  const minPorSku = useMemo(() => {
    const m = {}
    for (const l of lineas) {
      let min = null
      for (const p of provs) {
        const v = precio(p, l.sku)
        if (v != null && v > 0 && (min == null || v < min)) min = v
      }
      m[l.sku] = min
    }
    return m
  }, [sol])
  const respondidos = provs.filter((p) => p.respondida)
  const mejorTotal = respondidos.length ? Math.min(...respondidos.map((p) => Number(p.total) || Infinity)) : null

  if (!provs.length) {
    return <div className="text-[13px] text-slate-500 px-1 py-6 text-center">Esta solicitud aún no tiene proveedores. {gestiona ? 'Edítala para agregarlos.' : ''}</div>
  }

  return (
    <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
      <table className="w-full text-sm min-w-[640px]">
        <thead>
          <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
            <th className="py-2 px-3 font-medium">Producto</th>
            <th className="py-2 pr-3 font-medium text-center w-16">Cant.</th>
            {provs.map((p) => (
              <th key={p.proveedorId} className="py-2 px-3 font-medium text-right min-w-[120px]">
                <div className="truncate max-w-[160px] inline-block align-bottom">{p.proveedorNombre}</div>
                <div className="font-normal normal-case text-[10.5px] text-slate-400">{p.respondida ? 'respondió' : 'pendiente'}</div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {lineas.map((l) => (
            <tr key={l.sku} className="border-b border-slate-100 dark:border-slate-800/70">
              <td className="py-2 px-3">
                <div className="text-[13px] font-medium truncate max-w-[220px]">{l.nombre || l.sku}</div>
                <div className="text-[11px] text-slate-400 num">{l.sku}</div>
              </td>
              <td className="py-2 pr-3 text-center num text-[12.5px] text-slate-500">{fmtNum(l.cantidad)}</td>
              {provs.map((p) => {
                const v = precio(p, l.sku)
                const esMin = v != null && v > 0 && minPorSku[l.sku] != null && v === minPorSku[l.sku]
                return (
                  <td key={p.proveedorId} className="py-2 px-3 text-right num">
                    {v == null ? <span className="text-slate-300 dark:text-slate-600">—</span> : (
                      <span className={`private-mask inline-flex items-center gap-1 ${esMin ? 'text-emerald-600 dark:text-emerald-400 font-semibold' : 'text-slate-600 dark:text-slate-300'}`}>
                        {esMin ? <Icon.Star size={12} className="fill-current" /> : null}{fmtCurrency(v, 'VES')}
                      </span>
                    )}
                  </td>
                )
              })}
            </tr>
          ))}
          {/* Total por proveedor */}
          <tr className="bg-slate-50/60 dark:bg-slate-900/40">
            <td className="py-2.5 px-3 text-[12px] uppercase tracking-wide text-slate-400 font-medium" colSpan={2}>Total cotizado</td>
            {provs.map((p) => {
              const esMejor = p.respondida && mejorTotal != null && Number(p.total) === mejorTotal
              return (
                <td key={p.proveedorId} className="py-2.5 px-3 text-right num">
                  {p.respondida ? (
                    <span className={`private-mask font-semibold ${esMejor ? 'text-emerald-600 dark:text-emerald-400' : ''}`}>{fmtCurrency(p.total, 'VES')}</span>
                  ) : <span className="text-slate-300 dark:text-slate-600">—</span>}
                </td>
              )
            })}
          </tr>
          {/* Acciones por proveedor: convertir en orden */}
          {gestiona && abierta ? (
            <tr>
              <td className="py-2.5 px-3" colSpan={2}></td>
              {provs.map((p) => (
                <td key={p.proveedorId} className="py-2.5 px-3 text-right align-top">
                  <div className="flex flex-col items-end gap-1.5">
                    {onResponder ? (
                      <button onClick={() => onResponder(p)} className="text-[11.5px] text-elerp-600 hover:underline inline-flex items-center gap-1">
                        <Icon.Pencil size={12} /> {p.respondida ? 'Editar precios' : 'Cargar precios'}
                      </button>
                    ) : null}
                    {p.respondida ? (
                      <button onClick={() => onConvertir(p.proveedorId)} disabled={busy === 'conv:' + p.proveedorId}
                        className="inline-flex items-center gap-1 h-7 px-2.5 rounded-lg bg-elerp-600 text-white text-[11.5px] font-medium hover:bg-elerp-700 disabled:opacity-50">
                        {busy === 'conv:' + p.proveedorId ? <span className="spin inline-flex"><Icon.Refresh size={12} /></span> : <Icon.ArrowRight size={13} />} Convertir en orden
                      </button>
                    ) : null}
                  </div>
                </td>
              ))}
            </tr>
          ) : null}
        </tbody>
      </table>
    </div>
  )
}

// Modal para cargar/editar los precios que respondió un proveedor. Una fila por
// línea de la solicitud; el total se previsualiza en vivo (precio × cantidad).
function ModalRespuesta({ sol, proveedor, onClose, onSaved, toast }) {
  const lineas = sol.lineas || []
  const [precios, setPrecios] = useState(() => {
    const m = {}
    for (const l of lineas) {
      const r = (proveedor.lineas || []).find((x) => x.sku === l.sku)
      m[l.sku] = r ? String(r.precioUnitario) : ''
    }
    return m
  })
  const [busy, setBusy] = useState(false)

  const total = useMemo(() => lineas.reduce((acc, l) => {
    const v = Number(precios[l.sku])
    return acc + (v > 0 ? v * (Number(l.cantidad) || 0) : 0)
  }, 0), [precios, lineas])

  const algunPrecio = lineas.some((l) => Number(precios[l.sku]) > 0)

  const guardar = async () => {
    setBusy(true)
    try {
      const body = {
        proveedorId: proveedor.proveedorId,
        lineas: lineas
          .filter((l) => precios[l.sku] !== '' && Number(precios[l.sku]) >= 0)
          .map((l) => ({ sku: l.sku, precioUnitario: Number(precios[l.sku]) })),
      }
      await api.registrarRespuestaSolicitud(sol.id, body)
      toast({ title: 'Respuesta registrada', body: proveedor.proveedorNombre })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo registrar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Receipt size={18} />}
      title="Respuesta del proveedor" sub={proveedor.proveedorNombre}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} disabled={!algunPrecio} title={algunPrecio ? '' : 'Ingresa al menos un precio'} icon={<Icon.Check size={16} />}>Guardar respuesta</Button>
      </>}>
      <div className="space-y-3">
        <div className="text-[12px] text-slate-500">Ingresa el precio unitario que cotizó el proveedor para cada producto. Deja en blanco los que no coticó.</div>
        <div className="rounded-xl border border-slate-200 dark:border-slate-800 divide-y divide-slate-100 dark:divide-slate-800">
          {lineas.map((l) => {
            const v = Number(precios[l.sku])
            const sub = v > 0 ? v * (Number(l.cantidad) || 0) : 0
            return (
              <div key={l.sku} className="flex items-center gap-3 px-3 py-2">
                <div className="flex-1 min-w-0">
                  <div className="text-[13px] font-medium truncate">{l.nombre || l.sku}</div>
                  <div className="text-[11px] text-slate-400 num">{l.sku} · {fmtNum(l.cantidad)} u.</div>
                </div>
                <div className="w-32">
                  <input type="number" min="0" step="0.01" value={precios[l.sku]} onChange={(e) => setPrecios((c) => ({ ...c, [l.sku]: e.target.value }))}
                    placeholder="Precio unit." className="w-full h-8 text-right px-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num ring-focus private-mask" />
                </div>
                <div className="w-28 text-right num text-[12px] text-slate-500 private-mask">{sub > 0 ? fmtCurrency(sub, 'VES') : '—'}</div>
              </div>
            )
          })}
        </div>
        <div className="flex items-center justify-between rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3 py-2.5 text-[13px]">
          <span className="font-medium">Total cotizado</span>
          <span className="num font-semibold private-mask">{fmtCurrency(total, 'VES')}</span>
        </div>
      </div>
    </Modal>
  )
}
