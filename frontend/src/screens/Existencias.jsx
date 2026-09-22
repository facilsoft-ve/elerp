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

/* APARTADOS ABIERTOS: mercancía comprometida que todavía no ha salido.
 *
 * Se muestra arriba, con lo pendiente de ubicar, porque las dos responden a la
 * misma pregunta —«¿qué hay aquí que no está donde parece?»— y las dos se olvidan
 * si no se ven: la existencia no las delata, porque las unidades están.
 */
function ApartadosAbiertos({ recarga, puedeEditar, toast, onCambio }) {
  const [rows, setRows] = useState([])
  const [ocupado, setOcupado] = useState('')
  useEffect(() => {
    let vivo = true
    api.apartados()
      .then((r) => { if (vivo) setRows((r?.apartados || []).filter((a) => a.estado === 'abierto')) })
      .catch(() => { if (vivo) setRows([]) })
    return () => { vivo = false }
  }, [recarga])
  if (rows.length === 0) return null

  const accion = async (a, cual) => {
    setOcupado(a.id)
    try {
      if (cual === 'despachar') await api.despacharApartado(a.id)
      else await api.liberarApartado(a.id)
      toast({
        title: cual === 'despachar' ? 'Apartado despachado' : 'Apartado liberado',
        body: cual === 'despachar'
          ? 'La mercancía salió del inventario.'
          : 'La mercancía vuelve a estar disponible. No se movió nada.',
      })
      onCambio()
    } catch (e) {
      toast({ title: 'No se pudo', body: e?.message || 'Error', kind: 'error' })
    } finally { setOcupado('') }
  }

  return (
    <div className="mb-3 rounded-xl bg-violet-50 dark:bg-violet-900/25 border border-violet-200 dark:border-violet-900/40 px-3 py-2.5">
      <div className="flex items-start gap-2.5">
        <Icon.Boxes size={15} className="mt-0.5 shrink-0 text-violet-700 dark:text-violet-400" />
        <div className="text-[12.5px] text-violet-900 dark:text-violet-200 min-w-0 flex-1">
          <strong>{rows.length} apartado(s) abierto(s)</strong> — mercancía comprometida que sigue en el almacén y no se puede vender.
          <div className="mt-1 space-y-1">
            {rows.slice(0, 6).map((a) => (
              <div key={a.id} className="flex items-baseline gap-2 flex-wrap">
                <span className="text-[11.5px] font-medium">{a.motivo || 'sin motivo'}</span>
                <span className="text-[11.5px] opacity-80">
                  {(a.lineas || []).map((l) => `${l.sku} ×${fmtNum(l.cantidad)}`).join(', ')}
                </span>
                {puedeEditar ? (
                  <>
                    <button disabled={ocupado === a.id} onClick={() => accion(a, 'despachar')}
                      className="text-[11.5px] underline disabled:opacity-50">despachar</button>
                    <button disabled={ocupado === a.id} onClick={() => accion(a, 'liberar')}
                      className="text-[11.5px] underline disabled:opacity-50">liberar</button>
                  </>
                ) : null}
              </div>
            ))}
            {rows.length > 6 ? <div className="text-[11.5px] opacity-80">…y {rows.length - 6} más.</div> : null}
          </div>
        </div>
      </div>
    </div>
  )
}

/* PENDIENTE DE UBICAR: el segundo paso de una recepción en dos pasos.
 *
 * Existe porque si no se viera, la mercancía se quedaría en el muelle sin que nadie
 * se entere: el total de existencia no lo delata —las unidades están ahí— y el
 * proceso en dos pasos se convertiría en un proceso en uno con la mercancía mal
 * colocada. No aparece si no hay dos pasos configurados.
 */
function PendienteDeUbicar({ recarga }) {
  const [rows, setRows] = useState([])
  useEffect(() => {
    let vivo = true
    api.pendienteDeUbicar()
      .then((r) => { if (vivo) setRows(r?.pendientes || []) })
      .catch(() => { if (vivo) setRows([]) })
    return () => { vivo = false }
  }, [recarga])
  if (rows.length === 0) return null

  const total = rows.reduce((a, r) => a + (Number(r.cantidad) || 0), 0)
  return (
    <div className="mb-3 rounded-xl bg-sky-50 dark:bg-sky-900/25 border border-sky-200 dark:border-sky-900/40 px-3 py-2.5">
      <div className="flex items-start gap-2.5">
        <Icon.Package size={15} className="mt-0.5 shrink-0 text-sky-700 dark:text-sky-400" />
        <div className="text-[12.5px] text-sky-900 dark:text-sky-200 min-w-0">
          <strong>{fmtNum(total)} unidad(es) esperando en {rows[0].codigo}</strong> — recibidas pero todavía sin ubicar.
          <div className="mt-1 space-y-0.5">
            {rows.slice(0, 6).map((r) => (
              <div key={r.sku} className="flex items-baseline gap-2 flex-wrap">
                <span className="num text-[11.5px]">{r.sku}</span>
                <span className="text-[11.5px]">{r.nombreProducto}</span>
                <span className="num text-[11.5px]">{fmtNum(r.cantidad)} u.</span>
              </div>
            ))}
            {rows.length > 6 ? <div className="text-[11.5px] opacity-80">…y {rows.length - 6} más.</div> : null}
          </div>
          <div className="mt-1.5 text-[11.5px] opacity-90">
            Busca el producto abajo, abre <strong>¿Dónde está?</strong> y usa <strong>mover</strong> para colocarlo.
          </div>
        </div>
      </div>
    </div>
  )
}

// MoverDesde traslada mercancía de una ubicación a otra del MISMO almacén.
//
// Vive pegado a la fila de la ubicación de origen y no en un modal aparte porque
// la pregunta que responde es «esto que está aquí, ¿a dónde lo llevo?»: el origen
// ya está elegido por el sitio desde el que se abre.
function MoverDesde({ sku, fila, onHecho, onCancelar, toast }) {
  const [destinos, setDestinos] = useState(null)
  const [destino, setDestino] = useState('')
  const [cantidad, setCantidad] = useState('')
  const [guardando, setGuardando] = useState(false)

  useEffect(() => {
    let vivo = true
    if (!fila.almacenId) { setDestinos([]); return }
    api.ubicaciones(fila.almacenId)
      .then((r) => { if (vivo) setDestinos((r?.ubicaciones || []).filter((u) => u.activa && u.id !== fila.ubicacionId)) })
      .catch(() => { if (vivo) setDestinos([]) })
    return () => { vivo = false }
  }, [fila.almacenId, fila.ubicacionId])

  const mover = async () => {
    const cant = Number(cantidad)
    if (!(cant > 0)) return toast({ title: 'Indica cuánto vas a mover', kind: 'error' })
    if (cant > fila.cantidad + 0.005) {
      return toast({ title: 'No hay tanto aquí', body: `En ${fila.codigo} solo hay ${fmtNum(fila.cantidad)}.`, kind: 'error' })
    }
    setGuardando(true)
    try {
      await api.trasladarUbicacion(sku, {
        almacenId: fila.almacenId, origen: fila.ubicacionId, destino, cantidad: cant,
      })
      toast({ title: 'Mercancía movida', body: `${fmtNum(cant)} desde ${fila.codigo}. La existencia no cambia: solo el sitio.` })
      onHecho()
    } catch (e) {
      toast({ title: 'No se pudo mover', body: e?.message || 'Error', kind: 'error' })
    } finally {
      setGuardando(false)
    }
  }

  // Sin almacén no hay ubicaciones a las que mover: es saldo histórico, anterior a
  // los almacenes, y se coloca dando entrada en el almacén que corresponda.
  if (!fila.almacenId) {
    return (
      <div className="pl-32 py-1 text-[12px] text-slate-500">
        Este saldo no está asignado a ningún almacén, así que no hay a dónde moverlo dentro de uno.
        <button onClick={onCancelar} className="ml-2 underline">cerrar</button>
      </div>
    )
  }

  return (
    <div className="pl-32 py-2 flex flex-wrap items-center gap-2 text-[12.5px]">
      <input type="number" min="0" step="any" value={cantidad} onChange={(e) => setCantidad(e.target.value)}
        placeholder={`máx ${fmtNum(fila.cantidad)}`} autoFocus
        className="num w-28 rounded border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-2 py-1" />
      <span className="text-slate-400">a</span>
      <select value={destino} onChange={(e) => setDestino(e.target.value)}
        className="rounded border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-2 py-1 max-w-[16rem]">
        <option value="">Sin ubicar (el almacén, sin más detalle)</option>
        {(destinos || []).map((u) => <option key={u.id} value={u.id}>{u.codigo} · {u.nombre}</option>)}
      </select>
      <button onClick={mover} disabled={guardando}
        className="rounded bg-slate-900 dark:bg-slate-100 text-white dark:text-slate-900 px-3 py-1 disabled:opacity-50">
        {guardando ? 'Moviendo…' : 'Mover'}
      </button>
      <button onClick={onCancelar} className="text-slate-500 underline">cancelar</button>
    </div>
  )
}

/* DÓNDE ESTÁ un producto dentro de la sede.
 *
 * La suma de las cantidades es la existencia de la sede, siempre — y se muestra,
 * porque es la comprobación que convierte esta pantalla en algo fiable: si el
 * desglose no cuadra con el total, la ubicación está mintiendo.
 */
function FilaUbicaciones({ sku, sedeId, total, puedeMover, toast, onMovido }) {
  const [rows, setRows] = useState(null)
  const [moviendo, setMoviendo] = useState('')
  const [refresco, setRefresco] = useState(0)
  useEffect(() => {
    let vivo = true
    api.existenciaPorUbicacion(sku, sedeId)
      .then((r) => { if (vivo) setRows(r?.ubicaciones || []) })
      .catch(() => { if (vivo) setRows([]) })
    return () => { vivo = false }
  }, [sku, sedeId, refresco])

  const suma = (rows || []).reduce((a, u) => a + (Number(u.cantidad) || 0), 0)
  const cuadra = Math.abs(suma - (Number(total) || 0)) < 0.005

  return (
    <tr className="bg-slate-50/70 dark:bg-slate-800/40">
      <td colSpan={7} className="px-4 py-3">
        {rows === null ? (
          <div className="text-[12px] text-slate-400">Cargando…</div>
        ) : (
          <div className="space-y-1">
            {rows.map((u) => {
              const clave = u.almacenId + '/' + u.ubicacionId
              return (
              <div key={clave}>
                <div className="flex items-baseline gap-2 text-[12.5px]">
                  <span className="num text-slate-500 w-32 shrink-0">{u.codigo}</span>
                  <span className="text-slate-600 dark:text-slate-300 flex-1 min-w-0 truncate">
                    {u.almacenNombre}{u.nombre && u.codigo !== u.nombre ? ` · ${u.nombre}` : ''}
                  </span>
                  <span className="num font-medium">{fmtNum(u.cantidad)}</span>
                  {puedeMover && u.cantidad > 0 ? (
                    <button onClick={() => setMoviendo(moviendo === clave ? '' : clave)}
                      className="text-[11.5px] text-slate-500 hover:text-slate-900 dark:hover:text-slate-100 underline shrink-0">
                      {moviendo === clave ? 'cerrar' : 'mover'}
                    </button>
                  ) : null}
                </div>
                {moviendo === clave ? (
                  <MoverDesde sku={sku} fila={u} toast={toast}
                    onCancelar={() => setMoviendo('')}
                    onHecho={() => { setMoviendo(''); setRefresco((n) => n + 1); onMovido?.() }} />
                ) : null}
              </div>
            )})}
            <div className={`flex items-baseline gap-2 text-[12px] pt-1 mt-1 border-t border-slate-200 dark:border-slate-700 ${cuadra ? 'text-slate-400' : 'text-red-600 dark:text-red-400 font-medium'}`}>
              <span className="flex-1">
                {cuadra
                  ? 'La suma por ubicación cuadra con la existencia de la sede.'
                  : `El desglose suma ${fmtNum(suma)} y arriba figura ${fmtNum(total)}. Recarga la pantalla; si sigue distinto, avisa al equipo.`}
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
  const [apartando, setApartando] = useState(null)
  // Qué SKU tiene desplegado su desglose por ubicación. Uno a la vez: la pregunta
  // «¿dónde está esto?» es de un producto concreto, no de la lista entera.
  const [dondeEsta, setDondeEsta] = useState('')
  // Cada traslado vacía un poco el muelle: el aviso de pendientes se recarga con
  // él, o seguiría anunciando mercancía que ya está colocada.
  const [pendRecarga, setPendRecarga] = useState(0)
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
      <PendienteDeUbicar recarga={pendRecarga} />
      <ApartadosAbiertos recarga={pendRecarga} puedeEditar={editable} toast={toast}
        onCambio={() => { setPendRecarga((n) => n + 1); recargar() }} />
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
                  // El stock bajo se mide sobre lo DISPONIBLE, no sobre la existencia:
                  // 20 unidades con 18 apartadas son 2 para quien viene a comprar.
                  const libre = e.disponible === undefined ? Number(e.cantidad) || 0 : Number(e.disponible)
                  const apartado = Number(e.apartado) || 0
                  const bajo = libre <= LOW_STOCK
                  return (
                    <tr key={e.productoId || e.sku} className={`border-b border-slate-100 dark:border-slate-800/70 row-hover ${bajo ? 'bg-amber-50/40 dark:bg-amber-900/10' : ''}`}>
                      <td className={`${pad} pr-3 num text-[12.5px] text-slate-500`}>{e.sku}</td>
                      <td className={`${pad} pr-3 font-medium text-[13px]`}>{e.nombre}</td>
                      <td className={`${pad} pr-3 text-right num font-medium ${bajo ? 'text-amber-700 dark:text-amber-400' : ''}`}>
                        {fmtNum(e.cantidad)} {bajo ? <Icon.CircleAlert size={13} className="inline ml-1 -mt-0.5" /> : null}
                        {apartado > 0 ? (
                          <div className={`text-[11px] font-normal ${libre < 0 ? 'text-red-600 dark:text-red-400' : 'text-slate-400'}`}>
                            {fmtNum(apartado)} apartada(s) · {fmtNum(libre)} libre(s)
                          </div>
                        ) : null}
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
                          <button onClick={() => setApartando(e)} title="Apartar para un cliente"
                            className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800">
                            <Icon.Boxes size={15} />
                          </button>
                        ) : null}
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
                  return [fila, <FilaUbicaciones key={e.sku + '-ubi'} sku={e.sku} sedeId={activeSedeId} total={e.cantidad} puedeMover={puedeAjustar(ui.rol)} toast={toast} onMovido={() => { setPendRecarga((n) => n + 1); recargar() }} />]
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} producto(s){soloBajo === 'bajo' ? ' con stock bajo' : ''}</div> : null}

      {apartando ? (
        <ApartarModal existencia={apartando} almacenId={almacenSel} toast={toast}
          onClose={() => setApartando(null)}
          onSaved={() => { setApartando(null); setPendRecarga((n) => n + 1); recargar() }} />
      ) : null}
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

/* APARTAR: comprometer mercancía para alguien.
 *
 * Pide un motivo obligatorio y no por burocracia: lo apartado deja de poder
 * venderse, así que quien se encuentre el bloqueo tiene que poder leer para quién
 * es y decidir si lo libera. Un apartado sin motivo es un bloqueo sin dueño.
 */
function ApartarModal({ existencia, almacenId, onClose, onSaved, toast }) {
  const [cantidad, setCantidad] = useState('')
  const [motivo, setMotivo] = useState('')
  const [guardando, setGuardando] = useState(false)
  const libre = existencia.disponible === undefined ? Number(existencia.cantidad) || 0 : Number(existencia.disponible)

  const guardar = async () => {
    const cant = Number(cantidad)
    if (!(cant > 0)) return toast({ title: 'Indica cuánto vas a apartar', kind: 'error' })
    if (!motivo.trim()) return toast({ title: 'Indica para quién', body: 'Quien se encuentre el bloqueo necesita saber de quién es.', kind: 'error' })
    setGuardando(true)
    try {
      await api.crearApartado({ motivo: motivo.trim(), almacenId: almacenId || '', lineas: [{ sku: existencia.sku, cantidad: cant }] })
      toast({ title: 'Mercancía apartada', body: `${fmtNum(cant)} de ${existencia.nombre}. Sigue en el almacén, pero ya no se puede vender.` })
      onSaved()
    } catch (e) {
      toast({ title: 'No se pudo apartar', body: e?.message || 'Error', kind: 'error' })
    } finally { setGuardando(false) }
  }

  return (
    <Modal open onClose={onClose} title={`Apartar · ${existencia.nombre}`}>
      <div className="space-y-3">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
          Hay <strong>{fmtNum(existencia.cantidad)}</strong> en la sede y <strong>{fmtNum(libre)}</strong> sin comprometer.
          Apartar no mueve la mercancía: sigue en el almacén, pero deja de poder venderse.
        </div>
        <Field label="Cantidad a apartar">
          <Input type="number" min="0" step="any" value={cantidad} autoFocus
            onChange={(e) => setCantidad(e.target.value)} placeholder={`máximo ${fmtNum(libre)}`} />
        </Field>
        <Field label="¿Para quién?" hint="Lo lee quien se encuentre el bloqueo y tenga que decidir si lo libera.">
          <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} placeholder="Pedido de…" />
        </Field>
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button onClick={guardar} disabled={guardando}>{guardando ? 'Apartando…' : 'Apartar'}</Button>
        </div>
      </div>
    </Modal>
  )
}
