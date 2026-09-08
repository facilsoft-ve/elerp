import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { vendibles } from '../lib/catalogo.js'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Card, Select, Segmented, Toggle, Empty, Input, Field, Modal, PageHeader, useToast, useConfirm, TableSkeleton } from '../components/primitives.jsx'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { api } from '../lib/api.js'
import { precioEnBs, monedaDe } from '../lib/precio.js'
import { fmtCurrency } from '../lib/format.js'
import { ComprobanteModal, comprobanteDeFactura } from '../components/ComprobantePDF.jsx'

/* Módulo Restaurante — Fase 0: el MAPA DE MESAS (editor de arrastrar y soltar) y
 * la CONFIGURACIÓN DE LA IMPRESORA de comandas.
 *
 * El mapa deja diseñar el salón: se agregan mesas y se ubican/redimensionan sobre
 * un plano; ese mismo plano alimentará el selector visual de mesas del POS y, en
 * fases siguientes, el estado en vivo (libre/ocupada/por cobrar) desde la cuenta.
 */

const PUEDE_EDITAR = ['dueno', 'desarrollador']
const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v))

// Plano de referencia (unidades lógicas). El lienzo se escala al ancho disponible.
const PLANO_W = 640
const PLANO_H = 460

const FORMAS = [
  { id: 'cuadrada', label: 'Cuadrada' },
  { id: 'redonda', label: 'Redonda' },
  { id: 'rectangular', label: 'Rectangular' },
]

// Colores por estado operativo (fase de comandas los pondrá en vivo).
const ESTADO_COLOR = {
  libre: { bg: '#F3EFFE', border: '#B295F4', text: '#4A1AB4', label: 'Libre' },
  ocupada: { bg: '#FDECEC', border: '#E7A6A0', text: '#B3362C', label: 'Ocupada' },
  por_cobrar: { bg: '#FEF3C7', border: '#E7C765', text: '#92600A', label: 'Por cobrar' },
  reservada: { bg: '#E8F5EE', border: '#8CCBA6', text: '#166B41', label: 'Reservada' },
}
const colorEstado = (e) => ESTADO_COLOR[e] || ESTADO_COLOR.libre

// `soloMesonero: true` marca las pestañas que también alcanza el mesonero. El resto son
// de administración o de cocina: mostrárselas al mesero no aporta y confunde (el sidebar
// ya se las oculta; acá se hace lo mismo con las pestañas del encabezado).
const TABS = [
  { id: 'comandera', label: 'Comandera', icon: <Icon.ClipboardList size={15} />, mesonero: true },
  { id: 'mesas', label: 'Mapa de mesas', icon: <Icon.Utensils size={15} /> },
  { id: 'mesoneros', label: 'Mesoneros y asignación', icon: <Icon.Users size={15} /> },
  { id: 'cocina', label: 'Cocina', icon: <Icon.Activity size={15} /> },
  { id: 'platos', label: 'Platos y recetas', icon: <Icon.Boxes size={15} /> },
  { id: 'impresora', label: 'Comanderas', icon: <Icon.Printer size={15} /> },
]

export function Restaurante({ route }) {
  const { ui } = useUI()
  const esMesonero = ui.rol === 'mesonero'
  const tabs = esMesonero ? TABS.filter((t) => t.mesonero) : TABS
  const inicial = esMesonero ? 'comandera' : 'mesas'
  const [tab, setTab] = useState((route || '').split(':')[1] || inicial)
  useEffect(() => {
    const pedido = (route || '').split(':')[1] || inicial
    // Un mesonero que llegue por URL a una pestaña que no le toca va a la Comandera.
    setTab(tabs.some((t) => t.id === pedido) ? pedido : inicial)
  }, [route, esMesonero])
  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader breadcrumb={['Restaurante', TABS.find((t) => t.id === tab)?.label]} title="Restaurante"
        sub="Diseña el mapa de mesas del salón y configura la impresora donde salen las comandas."
        tabs={tabs} activeTab={tab} onTab={setTab} />
      {tab === 'comandera' ? <Comandera /> : null}
      {tab === 'mesas' ? <MapaMesas /> : null}
      {tab === 'mesoneros' ? <MesonerosAsignacion /> : null}
      {tab === 'cocina' ? <Cocina /> : null}
      {tab === 'platos' ? <PlatosRecetas /> : null}
      {tab === 'impresora' ? <ImpresoraComandas /> : null}
    </div>
  )
}

// ======================= MAPA DE MESAS =======================
function MapaMesas() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const confirm = useConfirm()
  const puedeEditar = PUEDE_EDITAR.includes(ui.rol)

  const [mesas, setMesas] = useState(null)  // null = cargando
  const [plano, setPlano] = useState({ filas: 6, columnas: 8, bloqueadas: [] })
  const [err, setErr] = useState(null)
  const [sel, setSel] = useState(null)
  const [modo, setModo] = useState('mesas') // 'mesas' | 'bloquear'
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)
  const [nueva, setNueva] = useState(null)  // {columna,fila} de la celda a poblar

  const cargar = useCallback(async () => {
    setErr(null)
    try {
      const [ms, pl] = await Promise.all([api.mesas(), api.planoSalon()])
      setMesas(ms)
      setPlano({ filas: pl?.filas || 6, columnas: pl?.columnas || 8, bloqueadas: pl?.bloqueadas || [] })
    } catch (e) { setErr(e); setMesas([]) }
  }, [])
  useEffect(() => { cargar() }, [cargar])

  const wrapRef = useRef(null)
  const [ancho, setAncho] = useState(720)
  useEffect(() => {
    const medir = () => { if (wrapRef.current) setAncho(wrapRef.current.clientWidth - 24) }
    medir(); window.addEventListener('resize', medir)
    return () => window.removeEventListener('resize', medir)
  }, [mesas])
  const cel = Math.max(46, Math.min(96, Math.floor(ancho / (plano.columnas || 8)))) // lado de celda en px

  const mesaEn = (c, r) => (mesas || []).find((m) => (m.columna || 0) === c && (m.fila || 0) === r)
  const bloqueada = (c, r) => (plano.bloqueadas || []).some((b) => b.columna === c && b.fila === r)
  const celdaLibre = (c, r) => c >= 0 && c < plano.columnas && r >= 0 && r < plano.filas && !mesaEn(c, r) && !bloqueada(c, r)

  const setFilas = (n) => { const v = clamp(n, 1, 20); setPlano((p) => ({ ...p, filas: v, bloqueadas: (p.bloqueadas || []).filter((b) => b.fila < v) })); setDirty(true) }
  const setColumnas = (n) => { const v = clamp(n, 1, 20); setPlano((p) => ({ ...p, columnas: v, bloqueadas: (p.bloqueadas || []).filter((b) => b.columna < v) })); setDirty(true) }

  const toggleBloqueo = (c, r) => {
    if (mesaEn(c, r)) { toast({ title: 'Ahí hay una mesa', body: 'Mueve o elimina la mesa antes de bloquear la celda.', kind: 'warn' }); return }
    setPlano((p) => {
      const existe = (p.bloqueadas || []).some((b) => b.columna === c && b.fila === r)
      return { ...p, bloqueadas: existe ? p.bloqueadas.filter((b) => !(b.columna === c && b.fila === r)) : [...(p.bloqueadas || []), { columna: c, fila: r }] }
    })
    setDirty(true)
  }

  const clicCelda = (c, r) => {
    if (!puedeEditar) return
    if (modo === 'bloquear') { toggleBloqueo(c, r); return }
    if (mesaEn(c, r)) { setSel(mesaEn(c, r).id); return }
    if (bloqueada(c, r)) return
    setNueva({ columna: c, fila: r })
  }

  // Arrastre de una mesa a otra celda libre.
  const drag = useRef(null)
  const gridRef = useRef(null)
  const onMesaDown = (e, m) => {
    if (!puedeEditar || modo !== 'mesas') { setSel(m.id); return }
    e.preventDefault(); e.stopPropagation(); setSel(m.id)
    drag.current = { id: m.id }
    window.addEventListener('pointerup', onUp)
  }
  const onUp = (e) => {
    const d = drag.current; drag.current = null
    window.removeEventListener('pointerup', onUp)
    if (!d || !gridRef.current) return
    const rect = gridRef.current.getBoundingClientRect()
    const c = Math.floor((e.clientX - rect.left) / cel)
    const r = Math.floor((e.clientY - rect.top) / cel)
    if (!celdaLibre(c, r)) return
    setMesas((ms) => ms.map((m) => m.id === d.id ? { ...m, columna: c, fila: r } : m))
    setDirty(true)
  }

  const guardar = async () => {
    setBusy(true)
    try {
      const ms = (mesas || []).map((m) => ({ ...m, columna: clamp(m.columna || 0, 0, plano.columnas - 1), fila: clamp(m.fila || 0, 0, plano.filas - 1) }))
      await api.guardarPlanoSalon({ filas: plano.filas, columnas: plano.columnas, bloqueadas: plano.bloqueadas })
      await api.guardarMapaMesas(ms.map((m) => ({ id: m.id, columna: m.columna, fila: m.fila })))
      setMesas(ms); setDirty(false); reload()
      toast({ title: 'Plano guardado' })
    } catch (e) { toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy(false) }
  }
  const eliminar = async (m) => {
    if (!(await confirm({ title: '¿Eliminar esta mesa?', body: `La mesa «${m.nombre}» se quitará del salón.`, confirmLabel: 'Eliminar', tone: 'danger' }))) return
    try { await api.eliminarMesa(m.id); if (sel === m.id) setSel(null); await cargar(); reload(); toast({ title: 'Mesa eliminada' }) }
    catch (e) { toast({ title: 'No se pudo eliminar', body: e?.message || 'Error', kind: 'error' }) }
  }

  const seleccionada = (mesas || []).find((m) => m.id === sel) || null

  if (mesas === null) return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={5} cols={3} /></div>
  if (err) return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar el salón" body={String(err?.message || err)} cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />

  const stepper = (label, val, on) => (
    <div className="flex items-center gap-1">
      <span className="text-[11.5px] text-slate-500">{label}</span>
      <button className="w-7 h-7 rounded-md border border-slate-300 dark:border-slate-700 disabled:opacity-40" disabled={!puedeEditar} onClick={() => on(val - 1)}>−</button>
      <span className="w-6 text-center text-[13px] font-semibold tabular-nums">{val}</span>
      <button className="w-7 h-7 rounded-md border border-slate-300 dark:border-slate-700 disabled:opacity-40" disabled={!puedeEditar} onClick={() => on(val + 1)}>+</button>
    </div>
  )

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3 flex-wrap">
        <div className="flex items-center gap-4 flex-wrap">
          {stepper('Columnas', plano.columnas, setColumnas)}
          {stepper('Filas', plano.filas, setFilas)}
          {puedeEditar ? (
            <Segmented size="sm" value={modo} onChange={setModo}
              options={[{ value: 'mesas', label: 'Mesas' }, { value: 'bloquear', label: 'Bloquear' }]} />
          ) : null}
        </div>
        {puedeEditar ? (
          <Button size="sm" loading={busy} disabled={!dirty} icon={<Icon.Check size={15} />} onClick={guardar}>Guardar plano</Button>
        ) : null}
      </div>
      <div className="flex items-center gap-3 flex-wrap text-[11.5px] mb-2 text-slate-500">
        {Object.entries(ESTADO_COLOR).map(([k, c]) => (
          <span key={k} className="inline-flex items-center gap-1.5"><span className="w-3 h-3 rounded" style={{ background: c.bg, border: `1.5px solid ${c.border}` }} /> {c.label}</span>
        ))}
        <span className="inline-flex items-center gap-1.5"><span className="w-3 h-3 rounded" style={{ backgroundImage: 'repeating-linear-gradient(45deg,#e2e8f0,#e2e8f0 3px,#cbd5e1 3px,#cbd5e1 5px)' }} /> Bloqueado</span>
        {puedeEditar ? <span className="text-slate-400">· {modo === 'bloquear' ? 'Toca una celda para bloquearla/desbloquearla.' : 'Toca una celda vacía para agregar mesa; arrastra una mesa para moverla.'}</span> : null}
      </div>

      <div className="grid lg:grid-cols-[1fr,260px] gap-4">
        <div ref={wrapRef} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-3 overflow-auto">
          <div ref={gridRef} className="relative mx-auto select-none" style={{ width: cel * plano.columnas, display: 'grid', gridTemplateColumns: `repeat(${plano.columnas}, ${cel}px)`, gridTemplateRows: `repeat(${plano.filas}, ${cel}px)`, gap: 0 }}>
            {Array.from({ length: plano.filas }).flatMap((_, r) => Array.from({ length: plano.columnas }).map((_, c) => {
              const m = mesaEn(c, r); const blk = bloqueada(c, r)
              const col = m ? colorEstado(m.estado) : null
              const activo = m && sel === m.id
              return (
                <div key={c + '-' + r} onPointerDown={() => (m ? null : clicCelda(c, r))}
                  className="border border-slate-100 dark:border-slate-800 flex items-center justify-center"
                  style={{ background: blk ? 'repeating-linear-gradient(45deg,#e2e8f0,#e2e8f0 4px,#cbd5e1 4px,#cbd5e1 7px)' : 'transparent', cursor: puedeEditar && !m ? 'pointer' : 'default' }}>
                  {m ? (
                    <div onPointerDown={(e) => onMesaDown(e, m)} onClick={() => setSel(m.id)}
                      className="flex flex-col items-center justify-center"
                      style={{ width: cel - 8, height: cel - 8, background: col.bg, border: `2px solid ${activo ? '#6A2CF0' : col.border}`, borderRadius: m.forma === 'redonda' ? '50%' : 8, color: col.text, cursor: puedeEditar && modo === 'mesas' ? 'grab' : 'pointer', boxShadow: activo ? '0 0 0 3px rgba(106,44,240,.18)' : 'none' }}>
                      <span className="font-display font-bold leading-none" style={{ fontSize: Math.max(12, cel * 0.24) }}>{m.nombre}</span>
                      <span className="inline-flex items-center gap-0.5 opacity-80" style={{ fontSize: Math.max(9, cel * 0.16) }}><Icon.Users size={Math.max(9, cel * 0.16)} /> {m.capacidad || 0}</span>
                    </div>
                  ) : blk && puedeEditar ? (
                    <Icon.CircleX size={Math.max(12, cel * 0.28)} className="text-slate-400" />
                  ) : null}
                </div>
              )
            }))}
          </div>
        </div>

        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-3.5">
          {!seleccionada ? (
            <div className="text-[12.5px] text-slate-400">
              Ajusta <strong>columnas y filas</strong> para dar forma al salón, <strong>bloquea</strong> las celdas donde no van mesas (paredes, cocina) y coloca las mesas en la grilla.
              {puedeEditar ? ' Toca una mesa para editar su nombre, zona, capacidad y forma.' : ''}
            </div>
          ) : (
            <PanelMesa key={seleccionada.id} mesa={seleccionada} puedeEditar={puedeEditar}
              onGuardado={async () => { await cargar(); reload() }} onEliminar={() => eliminar(seleccionada)} toast={toast} />
          )}
        </div>
      </div>

      {nueva ? <NuevaMesaModal columna={nueva.columna} fila={nueva.fila} onClose={() => setNueva(null)}
        onCreada={async (m) => { setNueva(null); await cargar(); reload(); setSel(m.id) }} toast={toast} /> : null}
    </div>
  )
}

// Panel de edición de una mesa (datos; la posición/tamaño se editan arrastrando).
function PanelMesa({ mesa, puedeEditar, onGuardado, onEliminar, toast }) {
  const [f, setF] = useState({ nombre: mesa.nombre, zona: mesa.zona || '', capacidad: mesa.capacidad || 0, forma: mesa.forma || 'cuadrada' })
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))
  const guardar = async () => {
    setBusy(true)
    try {
      await api.actualizarMesa(mesa.id, { nombre: f.nombre, zona: f.zona, capacidad: Number(f.capacidad) || 0, forma: f.forma, columna: mesa.columna, fila: mesa.fila })
      await onGuardado(); toast({ title: 'Mesa actualizada', body: f.nombre })
    } catch (e) { toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy(false) }
  }
  return (
    <div className="space-y-3">
      <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Mesa seleccionada</div>
      <Field label="Nombre / número"><Input value={f.nombre} disabled={!puedeEditar} onChange={(e) => set('nombre', e.target.value)} /></Field>
      <Field label="Zona / salón"><Input value={f.zona} disabled={!puedeEditar} onChange={(e) => set('zona', e.target.value)} placeholder="Salón principal, Terraza…" /></Field>
      <div className="grid grid-cols-2 gap-2">
        <Field label="Capacidad"><Input type="number" min={0} value={f.capacidad} disabled={!puedeEditar} onChange={(e) => set('capacidad', e.target.value)} /></Field>
        <Field label="Forma">
          <Select value={f.forma} disabled={!puedeEditar} onChange={(e) => set('forma', e.target.value)}>
            {FORMAS.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}
          </Select>
        </Field>
      </div>
      {puedeEditar ? (
        <div className="flex items-center gap-2 pt-1">
          <Button size="sm" loading={busy} onClick={guardar}>Guardar cambios</Button>
          <Button size="sm" variant="ghost" icon={<Icon.Trash size={14} />} onClick={onEliminar}>Eliminar</Button>
        </div>
      ) : null}
    </div>
  )
}

function NuevaMesaModal({ onClose, onCreada, toast, columna = 0, fila = 0 }) {
  const [f, setF] = useState({ nombre: '', zona: '', capacidad: 4, forma: 'cuadrada' })
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))
  const crear = async () => {
    if (!f.nombre.trim()) { toast({ title: 'Ponle un nombre o número a la mesa', kind: 'warn' }); return }
    setBusy(true)
    try {
      const m = await api.crearMesa({ nombre: f.nombre.trim(), zona: f.zona.trim(), capacidad: Number(f.capacidad) || 0, forma: f.forma, columna, fila })
      toast({ title: 'Mesa agregada', body: m.nombre }); onCreada(m)
    } catch (e) { toast({ title: 'No se pudo agregar', body: e?.message || 'Error', kind: 'error' }); setBusy(false) }
  }
  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Utensils size={18} />} title="Agregar mesa"
      footer={<><Button variant="ghost" onClick={onClose}>Cancelar</Button><Button loading={busy} onClick={crear}>Agregar</Button></>}>
      <div className="space-y-3">
        <Field label="Nombre / número"><Input autoFocus value={f.nombre} onChange={(e) => set('nombre', e.target.value)} placeholder="1, Terraza 3, Barra 2…" /></Field>
        <Field label="Zona / salón"><Input value={f.zona} onChange={(e) => set('zona', e.target.value)} placeholder="Salón principal, Terraza…" /></Field>
        <div className="grid grid-cols-2 gap-2">
          <Field label="Capacidad"><Input type="number" min={0} value={f.capacidad} onChange={(e) => set('capacidad', e.target.value)} /></Field>
          <Field label="Forma">
            <Select value={f.forma} onChange={(e) => set('forma', e.target.value)}>
              {FORMAS.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}
            </Select>
          </Field>
        </div>
      </div>
    </Modal>
  )
}

// ======================= IMPRESORA DE COMANDAS =======================
/* --- Comanderas (puestos de impresión de comandas) --------------------- */

/* Un local tiene VARIAS: cocina, barra y postres son puestos de preparación distintos y
 * cada uno necesita su ticket con SUS renglones. Cada comandera declara de qué rubros
 * imprime, y una queda PREDETERMINADA para lo que no encaje en ninguno — así un producto
 * de un rubro nuevo no se pierde en el camino. */
function ImpresoraComandas() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const confirm = useConfirm()
  const puedeEditar = PUEDE_EDITAR.includes(ui.rol)

  const [lista, setLista] = useState(db.IMPRESORAS_COMANDAS || null)
  const [editando, setEditando] = useState(null)

  useEffect(() => {
    if (db.IMPRESORAS_COMANDAS) { setLista(db.IMPRESORAS_COMANDAS); return }
    api.impresorasComandas().then((r) => setLista(r.impresoras || [])).catch(() => setLista([]))
  }, [db.IMPRESORAS_COMANDAS])

  // Los rubros del catálogo son lo que se reparte entre comanderas.
  const rubros = ((db.RUBROS || []).map((r) => r.nombre)).filter(Boolean)

  const eliminar = async (imp) => {
    if (!(await confirm({
      title: `¿Eliminar la comandera «${imp.nombre}»?`,
      body: imp.predeterminada
        ? 'Es la predeterminada: si queda otra, hereda el papel de recibir lo que no encaje en ningún rubro.'
        : 'Sus rubros pasarán a imprimirse por la comandera predeterminada.',
      confirmLabel: 'Eliminar', tone: 'danger',
    }))) return
    try {
      await api.eliminarImpresora(imp.id)
      toast({ title: 'Comandera eliminada' })
      reload()
    } catch (e) { toast({ title: 'No se pudo eliminar', body: e?.message || 'Error', kind: 'error' }) }
  }

  if (lista === null) return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={3} cols={3} /></div>

  return (
    <div className="space-y-4 max-w-3xl">
      <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
        Cada <strong>comandera</strong> es un puesto donde salen las comandas: cocina, barra, postres. Declará de qué
        <strong> rubros</strong> imprime cada una y el pedido se reparte solo. La <strong>predeterminada</strong> recibe
        lo que no encaje en ningún rubro, para que nada se quede sin imprimir.
      </div>

      {lista.length === 0 ? (
        <Empty title="Todavía no hay comanderas"
          body="Sin comanderas la comanda igual se genera y se ve en pantalla. Agregá una por cada puesto de preparación."
          cta={puedeEditar ? <Button size="lg" icon={<Icon.Plus size={16} />} onClick={() => setEditando({})}>Agregar comandera</Button> : null} />
      ) : (
        <div className="space-y-2">
          {lista.map((imp) => (
            <div key={imp.id} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 flex items-start gap-3">
              <span className="h-10 w-10 rounded-icon inline-flex items-center justify-center shrink-0"
                style={{ background: 'var(--hb-azul-suave)', color: 'var(--hb-azul)' }}>
                <Icon.Printer size={18} />
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5 flex-wrap">
                  <span className="font-semibold text-[14px]">{imp.nombre}</span>
                  {imp.predeterminada ? <Badge size="sm" color="huberp">Predeterminada</Badge> : null}
                  {imp.activa ? <Badge size="sm" color="teal">Activa</Badge> : <Badge size="sm" color="amber">Apagada</Badge>}
                </div>
                <div className="text-[12.5px] text-slate-500 mt-0.5">
                  {imp.conexion === 'red' ? `Red · ${imp.host}:${imp.puerto}` : 'Local (agente del equipo)'} · {imp.anchoMm || 80} mm
                </div>
                <div className="text-[12.5px] text-slate-500 mt-1 flex flex-wrap items-center gap-1">
                  {(imp.rubros || []).length ? (
                    <>Imprime: {(imp.rubros || []).map((r) => <Badge key={r} size="sm" color="slate">{r}</Badge>)}</>
                  ) : (
                    <span className="text-slate-400">Sin rubros propios{imp.predeterminada ? ' (recibe lo no clasificado)' : ' — no recibiría nada'}</span>
                  )}
                </div>
              </div>
              {puedeEditar ? (
                <div className="flex gap-1 shrink-0">
                  <Button size="sm" variant="ghost" onClick={() => setEditando(imp)}>Editar</Button>
                  <Button size="sm" variant="ghost" onClick={() => eliminar(imp)}>Eliminar</Button>
                </div>
              ) : null}
            </div>
          ))}
          {puedeEditar ? (
            <Button variant="secondary" size="lg" icon={<Icon.Plus size={16} />} onClick={() => setEditando({})}>Agregar comandera</Button>
          ) : null}
        </div>
      )}

      {editando ? (
        <ComanderaModal imp={editando} rubros={rubros} hayOtras={lista.length > 0}
          onClose={() => setEditando(null)}
          onGuardado={() => { setEditando(null); reload() }} />
      ) : null}
    </div>
  )
}

function ComanderaModal({ imp, rubros, hayOtras, onClose, onGuardado }) {
  const toast = useToast()
  const [f, setF] = useState({
    id: imp.id || '', nombre: imp.nombre || '', conexion: imp.conexion || 'local',
    host: imp.host || '', puerto: imp.puerto || 9100, anchoMm: imp.anchoMm || 80,
    rubros: imp.rubros || [], predeterminada: !!imp.predeterminada || !hayOtras,
    activa: imp.activa !== undefined ? imp.activa : true,
  })
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))
  const esRed = f.conexion === 'red'

  const toggleRubro = (r) => setF((s) => ({
    ...s,
    rubros: s.rubros.some((x) => x.toLowerCase() === r.toLowerCase())
      ? s.rubros.filter((x) => x.toLowerCase() !== r.toLowerCase())
      : [...s.rubros, r],
  }))

  const guardar = async () => {
    setBusy(true)
    try {
      await api.guardarImpresora({
        id: f.id, nombre: f.nombre, conexion: f.conexion, host: f.host,
        puerto: Number(f.puerto) || 0, anchoMm: Number(f.anchoMm) || 80,
        rubros: f.rubros, predeterminada: !!f.predeterminada, activa: !!f.activa,
      })
      toast({ title: f.id ? 'Comandera actualizada' : 'Comandera agregada' })
      onGuardado()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} icon={<Icon.Printer size={18} />}
      title={f.id ? `Comandera «${imp.nombre}»` : 'Nueva comandera'}
      footer={<>
        <Button size="lg" variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button size="lg" loading={busy} disabled={!f.nombre.trim()} onClick={guardar}>Guardar</Button>
      </>}>
      <div className="space-y-4">
        <Field label="Nombre del puesto">
          <Input value={f.nombre} onChange={(e) => set('nombre', e.target.value)} placeholder="Cocina, Barra, Postres…" />
        </Field>

        <div>
          <div className="text-[13px] font-semibold mb-1.5 text-slate-700 dark:text-slate-300">Conexión</div>
          <Segmented value={f.conexion} onChange={(v) => set('conexion', v)}
            options={[{ value: 'local', label: 'Local (equipo)' }, { value: 'red', label: 'Red (IP)' }]} />
        </div>

        {esRed ? (
          <div className="grid grid-cols-[1fr,120px] gap-2">
            <Field label="IP o host"><Input value={f.host} onChange={(e) => set('host', e.target.value)} placeholder="192.168.1.50" /></Field>
            <Field label="Puerto"><Input type="number" value={f.puerto} onChange={(e) => set('puerto', e.target.value)} placeholder="9100" /></Field>
          </div>
        ) : (
          <div className="text-[12px] rounded-lg px-3 py-2" style={{ background: 'var(--hb-azul-suave)', color: 'var(--hb-azul)' }}>
            La comanda sale por la impresora predeterminada de ese equipo, a través del agente local (igual que la máquina fiscal).
          </div>
        )}

        <div>
          <div className="text-[13px] font-semibold mb-1.5 text-slate-700 dark:text-slate-300">Ancho del papel</div>
          <Segmented value={String(f.anchoMm)} onChange={(v) => set('anchoMm', Number(v))}
            options={[{ value: '80', label: '80 mm' }, { value: '58', label: '58 mm' }]} />
        </div>

        <Field label="¿Qué rubros imprime?" hint="Los productos de estos rubros salen por esta comandera.">
          {rubros.length === 0 ? (
            <div className="text-[12.5px] text-slate-400">El catálogo no tiene rubros todavía.</div>
          ) : (
            <div className="flex flex-wrap gap-1.5">
              {rubros.map((r) => {
                const on = f.rubros.some((x) => x.toLowerCase() === r.toLowerCase())
                return (
                  <button key={r} type="button" onClick={() => toggleRubro(r)}
                    className={`${T.chip} border ring-focus transition-colors ${
                      on ? 'bg-elerp-500 text-white border-elerp-500'
                         : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800'}`}>
                    {r}
                  </button>
                )
              })}
            </div>
          )}
        </Field>

        <Toggle checked={f.predeterminada} onChange={(v) => set('predeterminada', v)}
          label="Predeterminada"
          sub="Recibe los productos cuyo rubro no está en ninguna comandera. Debe haber exactamente una: sin ella, un rubro nuevo no se imprimiría en ninguna parte." />

        <Toggle checked={f.activa} onChange={(v) => set('activa', v)}
          label="Activa" sub="Apagada, sus comandas se ven en pantalla pero no se envían a imprimir." />
      </div>
    </Modal>
  )
}

export function Comandera() {
  const { db, reload, tasaDe } = useData()
  const { ui } = useUI()
  const { user } = useAuth()
  const toast = useToast()
  const confirm = useConfirm()
  const monedaEmpresa = db.EMPRESA?.monedaPrincipal || 'VES'
  const mesas = (db.MESAS || []).filter((m) => m.activa !== false)
  const cuentasAbiertas = db.CUENTAS_ABIERTAS || []
  const [cuenta, setCuenta] = useState(null)
  const [busy, setBusy] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const [comanda, setComanda] = useState(null)
  const [cobroOpen, setCobroOpen] = useState(false)
  const [divisionOpen, setDivisionOpen] = useState(false)
  const [factura, setFactura] = useState(null)
  // Cobrar es de la caja: el mesonero pide la cuenta y ahí termina su parte.
  const puedeCobrar = ['dueno', 'desarrollador', 'cajero', 'vendedor'].includes(ui.rol)
  const esMesonero = ui.rol === 'mesonero'

  const cuentaDeMesa = (mesaId) => cuentasAbiertas.find((c) => c.mesaId === mesaId)

  // --- Asignación de mesas ---
  // Una mesa es «de» un mesonero por id o por su zona. Sin nadie asignado, es de
  // cualquiera. Solo condiciona al mesonero: la caja y la dueña atienden todas.
  const asignaciones = db.ASIGNACIONES_MESAS || []
  const estricta = !!db.CONFIG_SALON?.asignacionEstricta
  const miUsuarioId = user?.userId || ''
  const cubreMesa = (a, m) =>
    (a.mesas || []).includes(m.id) ||
    (a.zonas || []).some((z) => (z || '').trim().toLowerCase() === (m.zona || '').trim().toLowerCase())
  const mesonerosDeMesa = (m) => asignaciones.filter((a) => cubreMesa(a, m))
  const esMiMesa = (m) => {
    const duenos = mesonerosDeMesa(m)
    if (duenos.length === 0) return true // de nadie en particular
    return duenos.some((a) => a.usuarioId === miUsuarioId)
  }

  const abrirMesa = async (m) => {
    // Si la mesa es de OTRO mesonero se avisa antes: el servidor lo permite (y lo deja
    // en la bitácora) salvo que la sede tenga la asignación estricta, donde lo rechaza.
    // El aviso es para que tomarla sea una decisión, no un descuido.
    if (esMesonero && !esMiMesa(m)) {
      const duenos = mesonerosDeMesa(m)
      if (duenos.length) {
        const ok = await confirm({
          title: `La mesa ${m.nombre} es de ${duenos.map((d) => d.nombre).join(', ')}`,
          body: estricta
            ? 'La sede tiene la asignación estricta: no vas a poder tomarla. Pedile a la administración que la reasigne.'
            : 'Podés tomarla igual —queda registrado quién la atendió— o dejársela a su mesonero.',
          confirmLabel: estricta ? 'Entendido' : 'Tomarla igual',
          tone: 'warn',
        })
        if (!ok || estricta) return
      }
    }
    setBusy(true)
    try {
      const ex = cuentaDeMesa(m.id)
      const c = ex ? await api.cuenta(ex.id) : await api.abrirCuenta({ mesaId: m.id, comensales: m.capacidad || 0 })
      setCuenta(c); if (!ex) reload()
    } catch (e) { toast({ title: 'No se pudo abrir la mesa', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy(false) }
  }
  const pedirCuenta = async (division) => {
    setBusy(true)
    try {
      const r = await api.prefacturarCuenta(cuenta.id, division)
      setCuenta(r.cuenta)
      setDivisionOpen(false)
      const n = (r.prefacturas || []).length
      toast({
        title: n > 1 ? `Cuenta dividida en ${n} prefacturas` : 'Cuenta pedida',
        body: n > 1
          ? 'La caja cobra cada parte por separado.'
          : `La caja la busca como «Mesa ${cuenta.mesaNombre}» en Ventas › Confirmadas.`,
      })
      reload()
    } catch (e) { toast({ title: 'No se pudo pedir la cuenta', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy(false) }
  }
  const volverAServicio = async () => {
    setBusy(true)
    try {
      setCuenta(await api.cancelarPrefactura(cuenta.id))
      toast({ title: 'Mesa de vuelta en servicio' })
      reload()
    } catch (e) { toast({ title: 'No se pudo anular la prefactura', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy(false) }
  }
  const agregarProducto = async (p) => {
    const bs = precioEnBs(p, monedaEmpresa, tasaDe)
    if (bs === null) { toast({ title: 'Falta la tasa', body: `${p.nombre} está en ${monedaDe(p, monedaEmpresa)} y no hay tasa cargada.`, kind: 'warn' }); return }
    try { setCuenta(await api.agregarItemsCuenta(cuenta.id, [{ sku: p.sku, nombre: p.nombre, cantidad: 1, precioUnitario: bs, exento: !!p.exentoIva, nota: '' }])) }
    catch (e) { toast({ title: 'No se pudo agregar', body: e?.message || 'Error', kind: 'error' }) }
  }
  const cancelarItem = async (it) => {
    try { setCuenta(await api.cancelarItemCuenta(cuenta.id, it.id)) }
    catch (e) { toast({ title: 'No se pudo quitar', body: e?.message || 'Error', kind: 'error' }) }
  }
  const enviar = async () => {
    setBusy(true)
    try { const r = await api.enviarCocina(cuenta.id); setCuenta(r.cuenta); setComanda(r.comanda); reload() }
    catch (e) { toast({ title: 'No se pudo enviar a cocina', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy(false) }
  }
  const cerrar = async () => {
    if (!(await confirm({ title: '¿Cerrar sin cobrar?', body: `Se cierra la cuenta de la mesa ${cuenta.mesaNombre} y queda libre SIN emitir factura (p. ej. una mesa que se anuló). Para facturar, usa «Cobrar y facturar».`, confirmLabel: 'Cerrar sin cobrar', tone: 'danger' }))) return
    try { await api.cerrarCuenta(cuenta.id); setCuenta(null); reload(); toast({ title: 'Mesa cerrada' }) }
    catch (e) { toast({ title: 'No se pudo cerrar', body: e?.message || 'Error', kind: 'error' }) }
  }

  // Modal de factura emitida (reusa el comprobante, que imprime con el formato).
  const facturaModal = factura ? (
    <ComprobanteModal open onClose={() => setFactura(null)} empresa={db.EMPRESA}
      data={comprobanteDeFactura(factura, db.EMPRESA, null)} />
  ) : null

  if (!mesas.length) {
    return <>{facturaModal}<Empty icon={<Icon.Utensils size={22} />} title="Aún no hay mesas" body="Crea el salón en «Mapa de mesas» para empezar a tomar pedidos." /></>
  }
  if (cuenta) {
    return (<>
      <CuentaDetalle cuenta={cuenta} busy={busy} onVolver={() => setCuenta(null)}
        onAgregar={() => setMenuOpen(true)} onCancelar={cancelarItem} onEnviar={enviar} onCerrar={cerrar}
        onCobrar={() => setCobroOpen(true)} onPedirCuenta={() => setDivisionOpen(true)}
        onVolverAServicio={volverAServicio} puedeCobrar={puedeCobrar} />
      {divisionOpen ? <DivisionModal cuenta={cuenta} busy={busy}
        onClose={() => setDivisionOpen(false)} onConfirmar={pedirCuenta} /> : null}
      {menuOpen ? <MenuProductos productos={vendibles(db.PRODUCTOS)} monedaEmpresa={monedaEmpresa}
        onAgregar={agregarProducto} onClose={() => setMenuOpen(false)} /> : null}
      {comanda ? <ComandaModal cuenta={cuenta} comanda={comanda} onClose={() => setComanda(null)} /> : null}
      {cobroOpen ? <CobroModal cuenta={cuenta} cuentasCobro={db.CUENTAS_COBRO || []} onClose={() => setCobroOpen(false)}
        onCobrado={(doc) => { setCobroOpen(false); setCuenta(null); setFactura(doc); reload() }} /> : null}
      {facturaModal}
    </>)
  }
  // Tablero de mesas
  const c = (e) => (ESTADO_COLOR[e] || ESTADO_COLOR.libre)
  return (
    <div>
      {facturaModal}
      <div className="text-[12.5px] text-slate-500 dark:text-slate-400 mb-3">Toca una mesa para abrir su cuenta y tomar el pedido. Cada envío a cocina genera una comanda.</div>
      <div className="grid gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
        {mesas.map((m) => {
          const cta = cuentaDeMesa(m.id)
          const est = cta ? 'ocupada' : (m.estado || 'libre')
          const col = c(est)
          // Las mesas de OTRO mesonero se atenúan: siguen siendo tocables (cubrir a un
          // compañero es normal) pero se ven distintas, así el mesero encuentra las
          // suyas de un vistazo en un salón lleno.
          const ajena = esMesonero && !esMiMesa(m)
          const duenos = ajena ? mesonerosDeMesa(m) : []
          return (
            <button key={m.id} disabled={busy} onClick={() => abrirMesa(m)}
              title={ajena ? `Asignada a ${duenos.map((d) => d.nombre).join(', ')}` : undefined}
              className={`rounded-xl ${T.tarjeta} text-left border-2 transition-shadow hover:shadow-card active:scale-[0.98] disabled:opacity-60 ${ajena ? 'opacity-60' : ''}`}
              style={{ background: col.bg, borderColor: col.border, color: col.text }}>
              <div className="flex items-center justify-between">
                <span className="font-display font-bold text-[22px] leading-none">{m.nombre}</span>
                <span className="inline-flex items-center gap-1 text-[13px] opacity-80"><Icon.Users size={15} /> {m.capacidad || 0}</span>
              </div>
              <div className="text-[12.5px] mt-1.5 opacity-80">{m.zona || '—'}</div>
              <div className="mt-2 text-[15px] font-bold">{cta ? fmtCurrency(totalCuenta(cta), 'VES') : col.label}</div>
            </button>
          )
        })}
      </div>
    </div>
  )
}

function CuentaDetalle({ cuenta, busy, onVolver, onAgregar, onCancelar, onEnviar, onCerrar, onCobrar, onPedirCuenta, onVolverAServicio, puedeCobrar }) {
  const items = cuenta.items || []
  const pendientes = items.filter((it) => it.estado === 'pendiente')
  // Cuenta ya pedida (prefacturada): no se agregan renglones y lo que queda es cobrar.
  const pedida = (cuenta.prefacturas || []).length > 0
  // Agrupar por ronda: 0 = por enviar, luego 1..n.
  const rondas = [...new Set(items.map((it) => it.ronda || 0))].sort((a, b) => a - b)
  return (
    <div className="grid lg:grid-cols-[1fr,320px] gap-4">
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card">
        <div className="flex items-center gap-3 px-4 py-3 border-b border-slate-200 dark:border-slate-800">
          <Button size="lg" variant="ghost" icon={<Icon.ChevLeft size={18} />} onClick={onVolver}>Mesas</Button>
          <div className="min-w-0">
            <div className="font-display font-bold text-[16px]">Mesa {cuenta.mesaNombre}</div>
            <div className="text-[11.5px] text-slate-500">{cuenta.mesoneroNombre || '—'} · {cuenta.comensales || 0} comensal(es)</div>
          </div>
          <div className="ml-auto text-right">
            <div className="text-[11px] text-slate-500">Total</div>
            <div className="font-display font-bold text-[18px]">{fmtCurrency(totalCuenta(cuenta), 'VES')}</div>
          </div>
        </div>
        <div className="p-3 space-y-3 max-h-[62vh] overflow-auto">
          {items.length === 0 ? (
            <Empty icon={<Icon.ClipboardList size={22} />} title="Cuenta vacía" body="Agrega productos para armar el pedido." framed={false} />
          ) : rondas.map((r) => (
            <div key={r}>
              <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">{r === 0 ? 'Por enviar' : `Ronda ${r}`}</div>
              <div className="space-y-1">
                {items.filter((it) => (it.ronda || 0) === r).map((it) => (
                  <div key={it.id} className="flex items-center gap-2 rounded-lg border border-slate-200 dark:border-slate-800 px-2.5 py-1.5">
                    <span className="font-mono text-[12px] text-slate-500 w-7 text-right">{it.cantidad}×</span>
                    <div className="min-w-0 flex-1">
                      <div className={`text-[13px] truncate ${it.estado === 'cancelado' ? 'line-through text-slate-400' : ''}`}>{it.nombre}</div>
                      {it.nota ? <div className="text-[11px] text-slate-400">{it.nota}</div> : null}
                    </div>
                    <span className={`text-[10.5px] px-2 py-0.5 rounded-full font-semibold ${ITEM_COLOR[it.estado] || ITEM_COLOR.pendiente}`}>{ITEM_LABEL[it.estado] || it.estado}</span>
                    <span className="text-[12.5px] font-medium tabular-nums w-20 text-right">{fmtCurrency((it.precioUnitario || 0) * (it.cantidad || 0), 'VES')}</span>
                    {it.estado !== 'cancelado' ? (
                      <button onClick={() => onCancelar(it)} title="Quitar" aria-label={`Quitar ${it.nombre}`}
                        className={`${T.icono} text-slate-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-950/40 shrink-0`}><Icon.Trash size={18} /></button>
                    ) : null}
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="space-y-2">
        {pedida ? (
          /* Cuenta ya pedida: el mesonero terminó su parte. Lo que queda es cobrar, y
             eso es de la caja. Se le muestra qué prefactura(s) generó para que sepa
             qué va a buscar el cajero, y una salida para volver a servicio. */
          <>
            <div className="rounded-xl border border-teal-200 dark:border-teal-900/60 bg-teal-50 dark:bg-teal-950/40 p-3">
              <div className="flex items-center gap-1.5 font-semibold text-[13px] text-teal-900 dark:text-teal-100">
                <Icon.CircleCheck size={15} /> Cuenta pedida
              </div>
              <div className="text-[12px] text-teal-800 dark:text-teal-200 mt-1 leading-relaxed">
                {(cuenta.prefacturas || []).length > 1
                  ? `Se generaron ${(cuenta.prefacturas || []).length} prefacturas (cuenta dividida). La caja las cobra por separado.`
                  : 'La prefactura está lista. La caja la cobra y emite la factura.'}
              </div>
            </div>
            <Button className="w-full" size="lg" variant="ghost" loading={busy} onClick={onVolverAServicio}>
              Volver a servicio (anular prefactura)
            </Button>
            <div className="text-[11px] text-slate-400 px-1 pt-1">
              Volvé a servicio si el cliente pide algo más: con la cuenta pedida no se pueden agregar renglones,
              porque la prefactura quedaría desactualizada.
            </div>
          </>
        ) : (
          <>
            <Button className="w-full" size="xl" icon={<Icon.Plus size={18} />} variant="secondary" onClick={onAgregar}>Agregar productos</Button>
            <Button className="w-full" size="xl" loading={busy} disabled={!pendientes.length} icon={<Icon.Send size={18} />} onClick={onEnviar}>
              Enviar a cocina{pendientes.length ? ` (${pendientes.length})` : ''}
            </Button>
            <Button className="w-full" size="xl" disabled={!items.length} icon={<Icon.Receipt size={18} />} onClick={onPedirCuenta}>
              Pedir la cuenta
            </Button>
            {puedeCobrar ? (
              <Button className="w-full" size="lg" variant="secondary" disabled={!items.length} icon={<Icon.Wallet size={18} />} onClick={onCobrar}>
                Cobrar acá (caja)
              </Button>
            ) : null}
            <Button className="w-full" size="lg" variant="ghost" onClick={onCerrar}>Cerrar sin cobrar</Button>
            <div className="text-[11px] text-slate-400 px-1 pt-1">
              «Pedir la cuenta» genera la prefactura rotulada con la mesa; la caja la cobra desde Ventas y emite la
              factura. Ahí se puede dividir el cobro entre comensales o partir la cuenta por productos.
            </div>
          </>
        )}
      </div>
    </div>
  )
}

function MenuProductos({ productos, monedaEmpresa, onAgregar, onClose }) {
  const [q, setQ] = useState('')
  const term = q.trim().toLowerCase()
  const lista = productos.filter((p) => p.activo !== false)
    .filter((p) => !term || (p.nombre || '').toLowerCase().includes(term) || (p.sku || '').toLowerCase().includes(term))
    .slice(0, 60)
  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Cart size={18} />} title="Agregar al pedido"
      sub="Toca un producto para sumarlo a la cuenta." footer={<Button onClick={onClose}>Listo</Button>}>
      <div className="space-y-2">
        <Input autoFocus icon={<Icon.Search size={15} />} value={q} onChange={(e) => setQ(e.target.value)} placeholder="Buscar producto…" />
        <div className="max-h-[52vh] overflow-auto -mx-1 px-1 grid grid-cols-2 lg:grid-cols-3 gap-2">
          {lista.map((p) => (
            <button key={p.sku} onClick={() => onAgregar(p)}
              className="text-left rounded-xl border border-slate-200 dark:border-slate-800 px-3.5 py-3 min-h-[4.5rem] hover:border-elerp-400 hover:bg-elerp-50/40 dark:hover:bg-elerp-900/20 active:scale-[0.98] transition-all">
              <div className="text-[15px] font-semibold leading-snug line-clamp-2">{p.nombre}</div>
              <div className="text-[13px] text-slate-500 mt-1">{fmtCurrency(p.precio, monedaDe(p, monedaEmpresa))}{p.exentoIva ? ' · exento' : ''}</div>
            </button>
          ))}
          {lista.length === 0 ? <div className="text-[12.5px] text-slate-400 col-span-2 py-4 text-center">Sin resultados.</div> : null}
        </div>
      </div>
    </Modal>
  )
}

/* La comanda sale REPARTIDA por comandera: los platos por cocina, las bebidas por la
 * barra, los postres por la suya. Se muestra un ticket por puesto —tal como se imprime—
 * para que el mesonero vea qué salió por dónde y note al instante si algo no fue. */
function ComandaModal({ cuenta, comanda, onClose }) {
  const tickets = comanda.tickets || []
  const apagadas = tickets.filter((t) => t.impresora?.id && !t.impresora?.activa).length
  const sinConfigurar = tickets.some((t) => !t.impresora?.id)
  return (
    <Modal open onClose={onClose} icon={<Icon.Printer size={18} />}
      title={`Comanda · ronda ${comanda.ronda}`}
      sub={sinConfigurar
        ? 'Sin comanderas configuradas: la comanda se muestra en pantalla.'
        : tickets.length > 1
          ? `Salió por ${tickets.length} comanderas.`
          : `Salió por «${tickets[0]?.impresora?.nombre || '—'}».`}
      footer={<Button size="lg" onClick={onClose}>Cerrar</Button>}>
      <div className="space-y-3">
        {tickets.map((t, idx) => (
          <div key={t.impresora?.id || idx}>
            {t.impresora?.nombre ? (
              <div className="flex items-center gap-1.5 mb-1.5 text-[12.5px] flex-wrap">
                <Icon.Printer size={14} className="text-slate-400" />
                <span className="font-semibold">{t.impresora.nombre}</span>
                {t.impresora.conexion === 'red'
                  ? <span className="text-slate-400">· {t.impresora.host}:{t.impresora.puerto}</span>
                  : <span className="text-slate-400">· equipo local</span>}
                {!t.impresora.activa ? <Badge size="sm" color="amber">apagada</Badge> : null}
              </div>
            ) : null}
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white text-slate-900 p-3 font-mono text-[13px]">
              <div className="text-center font-bold">COMANDA · Mesa {cuenta.mesaNombre}</div>
              <div className="text-center text-[11.5px] text-slate-500 mb-2">
                Ronda {comanda.ronda} · {cuenta.mesoneroNombre || ''}{t.impresora?.nombre ? ` · ${t.impresora.nombre}` : ''}
              </div>
              <div className="border-t border-dashed border-slate-300 pt-2 space-y-1">
                {(t.items || []).map((it) => (
                  <div key={it.id}>
                    <div className="flex justify-between"><span>{it.cantidad}× {it.nombre}</span></div>
                    {it.nota ? <div className="text-[11.5px] text-slate-500 pl-4">↳ {it.nota}</div> : null}
                  </div>
                ))}
              </div>
            </div>
          </div>
        ))}
      </div>
      {apagadas > 0 ? (
        <div className="text-[12px] text-amber-700 dark:text-amber-300 mt-2">
          {apagadas === 1 ? 'Una comandera está apagada' : `${apagadas} comanderas están apagadas`}: su ticket no se envió a imprimir.
          Se activan en «Comanderas».
        </div>
      ) : null}
    </Modal>
  )
}

// Cobro simple de la cuenta → factura fiscal (pago único en Bs). El cobro mixto/
// multimoneda y la propina se integran con el flujo del POS más adelante.
function CobroModal({ cuenta, cuentasCobro, onClose, onCobrado }) {
  const toast = useToast()
  const [prev, setPrev] = useState(null) // {subtotal, iva, total}
  const [metodo, setMetodo] = useState('efectivo_bs')
  const [cuentaCobroId, setCuentaCobroId] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.previewCobroCuenta(cuenta.id).then(setPrev).catch(() => setPrev({ subtotal: 0, iva: 0, total: totalCuenta(cuenta) }))
  }, [cuenta.id])

  const esPagoMovil = metodo === 'pago_movil'
  const cobrar = async () => {
    if (esPagoMovil && !cuentaCobroId) { toast({ title: 'Elige la cuenta de destino del pago móvil', kind: 'warn' }); return }
    setBusy(true)
    try {
      const r = await api.cobrarCuenta(cuenta.id, { metodo, cuentaCobroId: esPagoMovil ? cuentaCobroId : '', sinCaja: true })
      toast({ title: 'Factura emitida', body: r.documento?.numeroCompleto || '' })
      onCobrado(r.documento)
    } catch (e) { toast({ title: 'No se pudo cobrar', body: e?.message || 'Error', kind: 'error' }); setBusy(false) }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Receipt size={18} />} title={`Cobrar mesa ${cuenta.mesaNombre}`}
      sub="Pago único en bolívares. Se emite la factura fiscal y se libera la mesa."
      footer={<><Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button loading={busy} disabled={!prev} onClick={cobrar} icon={<Icon.Check size={15} />}>Cobrar {prev ? fmtCurrency(prev.total, 'VES') : ''}</Button></>}>
      <div className="space-y-3.5">
        <div className="rounded-lg border border-slate-200 dark:border-slate-800 p-3 text-[13px] space-y-1">
          <div className="flex justify-between text-slate-500"><span>Subtotal</span><span>{prev ? fmtCurrency(prev.subtotal, 'VES') : '—'}</span></div>
          <div className="flex justify-between text-slate-500"><span>IVA</span><span>{prev ? fmtCurrency(prev.iva, 'VES') : '—'}</span></div>
          <div className="flex justify-between font-semibold text-[15px] border-t border-slate-200 dark:border-slate-700 pt-1 mt-1"><span>Total</span><span>{prev ? fmtCurrency(prev.total, 'VES') : '—'}</span></div>
        </div>
        <div>
          <div className="text-[13px] font-semibold mb-1.5 text-slate-700 dark:text-slate-300">Método de pago</div>
          <Segmented value={metodo} onChange={setMetodo}
            options={[{ value: 'efectivo_bs', label: 'Efectivo Bs' }, { value: 'pago_movil', label: 'Pago móvil' }]} />
        </div>
        {esPagoMovil ? (
          <Field label="Cuenta de destino">
            <Select value={cuentaCobroId} onChange={(e) => setCuentaCobroId(e.target.value)}>
              <option value="">Elegir cuenta…</option>
              {cuentasCobro.map((cc) => <option key={cc.id} value={cc.id}>{cc.nombre || cc.banco || cc.id}</option>)}
            </Select>
          </Field>
        ) : null}
      </div>
    </Modal>
  )
}

// ======================= PLATOS Y RECETAS (escandallo) =======================
const PUEDE_EDITAR_PLATOS = ['dueno', 'desarrollador']

function PlatosRecetas() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const confirm = useConfirm()
  const puedeEditar = PUEDE_EDITAR_PLATOS.includes(ui.rol)
  const productos = db.PRODUCTOS || []
  const existencias = db.EXISTENCIAS || []
  const monedaEmpresa = db.EMPRESA?.monedaPrincipal || 'VES'
  const [form, setForm] = useState(null) // null | {} (nuevo) | plato (editar)

  const costoDe = (sku) => Number(existencias.find((x) => x.sku === sku)?.costoPromedio) || 0
  const platos = productos.filter((p) => p.esPlato)
  const insumosPosibles = productos.filter((p) => !p.esCombo && !p.esPlato && p.activo !== false)
  const costoReceta = (receta) => (receta || []).reduce((a, r) => a + costoDe(r.sku) * (Number(r.cantidad) || 0), 0)

  const eliminar = async (p) => {
    if (!(await confirm({ title: '¿Ya no es un plato?', body: `«${p.nombre}» dejará de ser un plato con receta (vuelve a ser un producto normal). No se borra.`, confirmLabel: 'Quitar receta', tone: 'danger' }))) return
    try { await api.actualizarProducto(p.sku, { esPlato: false, receta: [] }); reload(); toast({ title: 'Receta quitada' }) }
    catch (e) { toast({ title: 'No se pudo', body: e?.message || 'Error', kind: 'error' }) }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3 mb-4">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          Un <strong>plato</strong> es un producto compuesto: se vende a su precio de menú, pero al facturarlo el
          inventario descuenta sus <strong>insumos</strong> (la receta/escandallo) — g de pasta, queso, etc.
        </div>
        {puedeEditar ? <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setForm({})}>Nuevo plato</Button> : null}
      </div>

      {platos.length === 0 ? (
        <Empty icon={<Icon.Boxes size={22} />} title="Sin platos con receta"
          body="Crea tu primer plato y define sus insumos para que el inventario se descuente solo al vender."
          cta={puedeEditar ? <Button icon={<Icon.Plus size={16} />} onClick={() => setForm({})}>Crear plato</Button> : null} />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {platos.map((p) => {
            const costo = costoReceta(p.receta)
            const margen = (Number(p.precio) || 0) - costo
            return (
              <div key={p.sku} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-3.5 flex flex-col gap-2">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="font-display font-semibold text-[14px] truncate">{p.nombre}</div>
                    <div className="text-[11.5px] text-slate-500">{(p.receta || []).length} insumo(s)</div>
                  </div>
                  <Badge size="sm" color="huberp">Plato</Badge>
                </div>
                <div className="text-[12px] space-y-0.5">
                  {(p.receta || []).slice(0, 5).map((r) => {
                    const ins = productos.find((x) => x.sku === r.sku)
                    return <div key={r.sku} className="flex justify-between text-slate-500"><span className="truncate">{ins?.nombre || r.sku}</span><span className="tabular-nums">{r.cantidad} {ins?.unidadBase || ''}</span></div>
                  })}
                </div>
                <div className="mt-auto pt-2 border-t border-slate-100 dark:border-slate-800 text-[12px] space-y-0.5">
                  <div className="flex justify-between"><span className="text-slate-500">Precio</span><span className="font-medium">{fmtCurrency(p.precio, monedaDe(p, monedaEmpresa))}</span></div>
                  <div className="flex justify-between"><span className="text-slate-500">Costo receta</span><span>{fmtCurrency(costo, 'VES')}</span></div>
                  <div className="flex justify-between font-semibold"><span>Margen</span><span className={margen < 0 ? 'text-red-500' : 'text-emerald-600'}>{fmtCurrency(margen, 'VES')}</span></div>
                </div>
                {puedeEditar ? (
                  <div className="flex gap-1.5">
                    <Button size="sm" variant="secondary" icon={<Icon.Pencil size={14} />} onClick={() => setForm(p)}>Editar receta</Button>
                    <Button size="sm" variant="ghost" icon={<Icon.Trash size={14} />} onClick={() => eliminar(p)}>Quitar</Button>
                  </div>
                ) : null}
              </div>
            )
          })}
        </div>
      )}

      {form ? <PlatoModal plato={form.sku ? form : null} insumos={insumosPosibles} monedaEmpresa={monedaEmpresa}
        costoDe={costoDe} onClose={() => setForm(null)} onGuardado={() => { setForm(null); reload() }} toast={toast} /> : null}
    </div>
  )
}

function PlatoModal({ plato, insumos, monedaEmpresa, costoDe, onClose, onGuardado, toast }) {
  const editar = !!plato
  const [f, setF] = useState(() => ({
    sku: plato?.sku || '', nombre: plato?.nombre || '', precio: plato?.precio || 0,
    exentoIva: !!plato?.exentoIva, receta: (plato?.receta || []).map((r) => ({ ...r })),
  }))
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))
  const setRow = (i, k, v) => setF((s) => ({ ...s, receta: s.receta.map((r, j) => j === i ? { ...r, [k]: v } : r) }))
  const addRow = () => setF((s) => ({ ...s, receta: [...s.receta, { sku: '', cantidad: 1 }] }))
  const delRow = (i) => setF((s) => ({ ...s, receta: s.receta.filter((_, j) => j !== i) }))
  const costo = f.receta.reduce((a, r) => a + costoDe(r.sku) * (Number(r.cantidad) || 0), 0)

  const guardar = async () => {
    const receta = f.receta.filter((r) => r.sku && Number(r.cantidad) > 0).map((r) => ({ sku: r.sku, cantidad: Number(r.cantidad) }))
    if (!f.nombre.trim()) { toast({ title: 'Ponle nombre al plato', kind: 'warn' }); return }
    if (!receta.length) { toast({ title: 'Agrega al menos un insumo a la receta', kind: 'warn' }); return }
    setBusy(true)
    try {
      if (editar) {
        await api.actualizarProducto(f.sku, { nombre: f.nombre.trim(), precio: Number(f.precio) || 0, exentoIva: f.exentoIva, esPlato: true, receta })
      } else {
        const sku = (f.sku || ('PLATO-' + Date.now().toString(36).toUpperCase())).trim()
        await api.createProducto({ sku, nombre: f.nombre.trim(), precio: Number(f.precio) || 0, moneda: monedaEmpresa, exentoIva: f.exentoIva, esPlato: true, receta })
      }
      toast({ title: editar ? 'Plato actualizado' : 'Plato creado', body: f.nombre })
      onGuardado()
    } catch (e) { toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' }); setBusy(false) }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Utensils size={18} />} title={editar ? `Editar ${plato.nombre}` : 'Nuevo plato'}
      sub="Define el precio de menú y los insumos que consume cada plato."
      footer={<><Button variant="ghost" onClick={onClose}>Cancelar</Button><Button loading={busy} onClick={guardar}>{editar ? 'Guardar' : 'Crear plato'}</Button></>}>
      <div className="space-y-3.5">
        <div className="grid grid-cols-2 gap-2">
          <Field label="Nombre del plato"><Input autoFocus value={f.nombre} onChange={(e) => set('nombre', e.target.value)} placeholder="Pabellón criollo" /></Field>
          <Field label="Precio de menú (Bs)"><Input type="number" min={0} value={f.precio} onChange={(e) => set('precio', e.target.value)} /></Field>
        </div>
        <div>
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-[13px] font-semibold text-slate-700 dark:text-slate-300">Receta (insumos)</span>
            <Button size="sm" variant="ghost" icon={<Icon.Plus size={14} />} onClick={addRow}>Agregar insumo</Button>
          </div>
          <div className="space-y-1.5">
            {f.receta.length === 0 ? <div className="text-[12px] text-slate-400">Sin insumos aún. Agrega los materiales que consume el plato.</div> : null}
            {f.receta.map((r, i) => {
              const ins = insumos.find((x) => x.sku === r.sku)
              return (
                <div key={i} className="flex items-center gap-2">
                  <Select value={r.sku} onChange={(e) => setRow(i, 'sku', e.target.value)} className="flex-1">
                    <option value="">Elegir insumo…</option>
                    {insumos.map((p) => <option key={p.sku} value={p.sku}>{p.nombre}</option>)}
                  </Select>
                  <Input type="number" min={0} step="0.001" value={r.cantidad} onChange={(e) => setRow(i, 'cantidad', e.target.value)} className="w-24" />
                  <span className="text-[11.5px] text-slate-400 w-10">{ins?.unidadBase || ''}</span>
                  <button onClick={() => delRow(i)} className="p-1.5 rounded-md text-slate-400 hover:text-red-500"><Icon.Trash size={14} /></button>
                </div>
              )
            })}
          </div>
        </div>
        <div className="flex items-center justify-between rounded-lg bg-slate-50 dark:bg-slate-800/40 px-3 py-2 text-[13px]">
          <span className="text-slate-500">Costo de la receta (escandallo)</span>
          <span className="font-semibold">{fmtCurrency(costo, 'VES')}</span>
        </div>
        <Toggle checked={f.exentoIva} onChange={(v) => set('exentoIva', v)} label="Exento de IVA" sub="Marca solo si el plato no lleva IVA." />
      </div>
    </Modal>
  )
}

// ======================= COCINA (KDS) =======================
// Minutos transcurridos desde un instante RFC3339 (para «hace X min»).
function minutosDesde(iso) {
  if (!iso) return 0
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return 0
  return Math.max(0, Math.floor((Date.now() - t) / 60000))
}
// Color de urgencia de una comanda por su espera.
function urgencia(min) {
  if (min >= 20) return { bar: '#B3362C', txt: 'text-red-600' }
  if (min >= 10) return { bar: '#92600A', txt: 'text-amber-600' }
  return { bar: '#6A2CF0', txt: 'text-slate-500' }
}

function Cocina() {
  const toast = useToast()
  const [cuentas, setCuentas] = useState(null)
  const [err, setErr] = useState(null)
  const [tick, setTick] = useState(0) // fuerza recálculo de tiempos
  const [busy, setBusy] = useState('')

  const cargar = useCallback(async () => {
    try { setCuentas(await api.cuentasAbiertas()); setErr(null) }
    catch (e) { setErr(e) }
  }, [])
  useEffect(() => { cargar() }, [cargar])
  // En vivo: refresca datos cada 7 s y los relojes cada 30 s.
  useEffect(() => {
    const d = setInterval(cargar, 7000)
    const r = setInterval(() => setTick((t) => t + 1), 30000)
    return () => { clearInterval(d); clearInterval(r) }
  }, [cargar])

  // Arma las COMANDAS (mesa + ronda) con renglones aún no servidos.
  const comandas = []
  for (const c of (cuentas || [])) {
    const porRonda = {}
    for (const it of (c.items || [])) {
      if (it.estado !== 'en_cocina' && it.estado !== 'listo') continue
      const r = it.ronda || 0
      ;(porRonda[r] ||= []).push(it)
    }
    for (const [r, items] of Object.entries(porRonda)) {
      const enviadoEn = items.map((i) => i.enviadoEn).filter(Boolean).sort()[0]
      comandas.push({ cuentaId: c.id, mesa: c.mesaNombre, mesonero: c.mesoneroNombre, ronda: Number(r), enviadoEn, items })
    }
  }
  comandas.sort((a, b) => String(a.enviadoEn || '').localeCompare(String(b.enviadoEn || '')))

  const marcar = async (cuentaId, itemId, estado) => {
    setBusy(itemId + estado)
    try { await api.marcarItemCuenta(cuentaId, itemId, estado); await cargar() }
    catch (e) { toast({ title: 'No se pudo actualizar', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy('') }
  }
  const marcarComanda = async (com, estado) => {
    setBusy(com.cuentaId + com.ronda + estado)
    try {
      for (const it of com.items) {
        if (estado === 'listo' && it.estado !== 'en_cocina') continue
        await api.marcarItemCuenta(com.cuentaId, it.id, estado)
      }
      await cargar()
    } catch (e) { toast({ title: 'No se pudo actualizar', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy('') }
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
          Comandas entrantes en vivo. Marca cada plato <strong>Listo</strong> cuando salga de cocina; el mesonero lo ve al instante.
        </div>
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center gap-1.5 text-[11.5px] text-emerald-600"><span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" /> En vivo</span>
          <Button size="sm" variant="ghost" icon={<Icon.Refresh size={14} />} onClick={cargar}>Actualizar</Button>
        </div>
      </div>

      {cuentas === null ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={4} cols={3} /></div>
      ) : err ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar la cocina" body={String(err?.message || err)}
          cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : comandas.length === 0 ? (
        <Empty icon={<Icon.Activity size={22} />} title="Cocina al día" body="No hay comandas pendientes. Las nuevas aparecerán aquí en cuanto el mesonero las envíe." />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {comandas.map((com) => {
            const min = minutosDesde(com.enviadoEn)
            const u = urgencia(min)
            const todoListo = com.items.every((i) => i.estado === 'listo')
            return (
              <div key={com.cuentaId + '-' + com.ronda} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden flex flex-col">
                <div className="h-1.5" style={{ background: todoListo ? '#166B41' : u.bar }} />
                <div className="px-3 py-2.5 flex items-center justify-between border-b border-slate-100 dark:border-slate-800">
                  <div>
                    <div className="font-display font-bold text-[15px]">Mesa {com.mesa}</div>
                    <div className="text-[11px] text-slate-400">Ronda {com.ronda} · {com.mesonero || ''}</div>
                  </div>
                  <div className={`text-[12px] font-semibold ${u.txt}`}>{min === 0 ? 'ahora' : `${min} min`}</div>
                </div>
                <div className="p-2.5 space-y-1 flex-1">
                  {com.items.map((it) => {
                    const listo = it.estado === 'listo'
                    return (
                      <button key={it.id} disabled={listo || busy === it.id + 'listo'}
                        onClick={() => marcar(com.cuentaId, it.id, 'listo')}
                        className={`w-full flex items-center gap-2 rounded-lg px-2.5 py-2 text-left border transition-colors ${listo ? 'border-emerald-200 bg-emerald-50/60 dark:bg-emerald-900/20 dark:border-emerald-900' : 'border-slate-200 dark:border-slate-700 hover:border-elerp-400'}`}>
                        <span className="font-mono text-[13px] font-semibold w-7 text-right">{it.cantidad}×</span>
                        <div className="min-w-0 flex-1">
                          <div className={`text-[13.5px] font-medium ${listo ? 'text-emerald-700 dark:text-emerald-300' : ''}`}>{it.nombre}</div>
                          {it.nota ? <div className="text-[11px] text-slate-500">↳ {it.nota}</div> : null}
                        </div>
                        {listo ? <Icon.CircleCheck size={18} className="text-emerald-500 shrink-0" /> : <span className="text-[11px] text-slate-400 shrink-0">Listo →</span>}
                      </button>
                    )
                  })}
                </div>
                <div className="p-2.5 pt-0">
                  {todoListo ? (
                    <Button size="sm" variant="ghost" className="w-full" icon={<Icon.Check size={15} />}
                      loading={busy === com.cuentaId + com.ronda + 'servido'} onClick={() => marcarComanda(com, 'servido')}>Entregado a la mesa</Button>
                  ) : (
                    <Button size="sm" className="w-full" icon={<Icon.CircleCheck size={15} />}
                      loading={busy === com.cuentaId + com.ronda + 'listo'} onClick={() => marcarComanda(com, 'listo')}>Todo listo</Button>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

/* --- Mesoneros y asignación de mesas ------------------------------------- */

/* Organiza el turno: qué mesas o zonas atiende cada mesonero. Es una GUÍA, no un
 * candado — por defecto un mesonero puede tomar la mesa de otro, se le advierte y queda
 * en la bitácora. El interruptor «asignación estricta» la convierte en candado (lo
 * rechaza el servidor, no la interfaz).
 *
 * Se puede asignar desde los dos lados porque son dos vistas del MISMO vínculo: por
 * mesonero (qué atiende) y por mesa (quién la atiende). El dato se guarda una sola vez,
 * en la asignación del mesonero. */
function MesonerosAsignacion() {
  const { db, reload } = useData()
  const toast = useToast()
  const mesas = db.MESAS || []
  const [mesoneros, setMesoneros] = useState(null)
  const [asignaciones, setAsignaciones] = useState(db.ASIGNACIONES_MESAS || [])
  const [estricta, setEstricta] = useState(!!db.CONFIG_SALON?.asignacionEstricta)
  const [vista, setVista] = useState('mesonero') // mesonero | mesa
  const [guardando, setGuardando] = useState('')

  useEffect(() => {
    api.usuarios()
      .then((r) => setMesoneros((r.miembros || r.usuarios || r || []).filter((u) => u.rol === 'mesonero')))
      .catch(() => setMesoneros([]))
  }, [])

  const zonas = useMemo(() => {
    const set = new Map()
    for (const m of mesas) {
      const z = (m.zona || '').trim()
      if (z) set.set(z.toLowerCase(), z)
    }
    return [...set.values()].sort()
  }, [mesas])

  const asignacionDe = (usuarioId) =>
    asignaciones.find((a) => a.usuarioId === usuarioId) || { usuarioId, mesas: [], zonas: [] }

  // Una mesa está cubierta por id o por su zona (asignar la zona cubre las mesas que se
  // agreguen después, que es como se organiza un turno de verdad).
  const cubre = (a, m) =>
    (a.mesas || []).includes(m.id) ||
    (a.zonas || []).some((z) => z.trim().toLowerCase() === (m.zona || '').trim().toLowerCase())

  const guardar = async (usuarioId, nombre, cambios) => {
    const actual = asignacionDe(usuarioId)
    const body = {
      usuarioId, nombre,
      mesas: cambios.mesas ?? actual.mesas ?? [],
      zonas: cambios.zonas ?? actual.zonas ?? [],
    }
    setGuardando(usuarioId)
    try {
      const out = await api.guardarAsignacionMesas(body)
      setAsignaciones((prev) => {
        const resto = prev.filter((a) => a.usuarioId !== usuarioId)
        const vacia = (out.mesas || []).length === 0 && (out.zonas || []).length === 0
        return vacia ? resto : [...resto, out]
      })
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e.message, tone: 'error' })
    } finally {
      setGuardando('')
    }
  }

  const toggleZona = (u, zona) => {
    const a = asignacionDe(u.usuarioId || u.id)
    const tiene = (a.zonas || []).some((z) => z.trim().toLowerCase() === zona.trim().toLowerCase())
    const zonasNuevas = tiene
      ? (a.zonas || []).filter((z) => z.trim().toLowerCase() !== zona.trim().toLowerCase())
      : [...(a.zonas || []), zona]
    guardar(u.usuarioId || u.id, u.nombre, { zonas: zonasNuevas })
  }

  const toggleMesa = (u, mesaId) => {
    const uid = u.usuarioId || u.id
    const a = asignacionDe(uid)
    const tiene = (a.mesas || []).includes(mesaId)
    guardar(uid, u.nombre, {
      mesas: tiene ? (a.mesas || []).filter((x) => x !== mesaId) : [...(a.mesas || []), mesaId],
    })
  }

  const cambiarEstricta = async (v) => {
    setEstricta(v)
    try {
      await api.guardarConfigSalon({ asignacionEstricta: v })
      toast({ title: v ? 'Asignación estricta activada' : 'Asignación flexible' })
      reload()
    } catch (e) {
      setEstricta(!v)
      toast({ title: 'No se pudo guardar', body: e.message, tone: 'error' })
    }
  }

  if (mesoneros === null) return <div className="py-16 text-center text-slate-500">Cargando…</div>

  return (
    <div className="space-y-5">
      <Card className="!p-5">
        <Toggle checked={estricta} onChange={cambiarEstricta}
          label="Asignación estricta"
          sub="Apagada (recomendado): un mesonero puede tomar la mesa de otro; se le avisa y queda registrado en la bitácora. Encendida: el sistema lo rechaza y solo la administración puede reasignar." />
      </Card>

      {mesoneros.length === 0 ? (
        <Empty title="Todavía no hay mesoneros"
          body="Invitá a alguien con el rol Mesonero en Configuración › Usuarios y roles. Sin asignación, cualquier mesonero atiende cualquier mesa." />
      ) : (
        <>
          <div className="flex items-center gap-1.5">
            <span className="text-[13px] text-slate-500 mr-1">Asignar por:</span>
            {[{ id: 'mesonero', label: 'Mesonero' }, { id: 'mesa', label: 'Mesa' }].map((v) => (
              <button key={v.id} onClick={() => setVista(v.id)}
                className={`h-7 px-3 rounded-full text-[12.5px] font-medium border ring-focus transition-colors ${
                  vista === v.id
                    ? 'bg-elerp-500 text-white border-elerp-500'
                    : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800'}`}>
                {v.label}
              </button>
            ))}
          </div>

          {vista === 'mesonero' ? (
            <div className="grid gap-4 md:grid-cols-2">
              {mesoneros.map((u) => {
                const uid = u.usuarioId || u.id
                const a = asignacionDe(uid)
                const sinAsignar = (a.mesas || []).length === 0 && (a.zonas || []).length === 0
                return (
                  <Card key={uid} className="!p-5">
                    <div className="flex items-center justify-between gap-2 mb-3">
                      <div className="min-w-0">
                        <div className="font-semibold text-[14px] truncate">{u.nombre}</div>
                        <div className="text-[12px] text-slate-500 truncate">{u.email}</div>
                      </div>
                      {sinAsignar ? <Badge size="sm" color="slate">Atiende cualquier mesa</Badge> : null}
                    </div>
                    <div className="text-[11.5px] uppercase tracking-wide text-slate-500 mb-1.5">Zonas</div>
                    <div className="flex flex-wrap gap-1.5 mb-3">
                      {zonas.length === 0 ? <span className="text-[12.5px] text-slate-400">Las mesas no tienen zona</span> : null}
                      {zonas.map((z) => {
                        const on = (a.zonas || []).some((x) => x.trim().toLowerCase() === z.toLowerCase())
                        return (
                          <button key={z} disabled={guardando === uid} onClick={() => toggleZona(u, z)}
                            className={`h-7 px-3 rounded-full text-[12.5px] font-medium border ring-focus transition-colors disabled:opacity-50 ${
                              on ? 'bg-elerp-500 text-white border-elerp-500'
                                 : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800'}`}>
                            {z}
                          </button>
                        )
                      })}
                    </div>
                    <div className="text-[11.5px] uppercase tracking-wide text-slate-500 mb-1.5">Mesas sueltas</div>
                    <div className="flex flex-wrap gap-1.5">
                      {mesas.map((m) => {
                        const porZona = (a.zonas || []).some((x) => x.trim().toLowerCase() === (m.zona || '').trim().toLowerCase())
                        const on = (a.mesas || []).includes(m.id)
                        return (
                          <button key={m.id} disabled={guardando === uid || porZona}
                            title={porZona ? `Ya cubierta por la zona ${m.zona}` : m.zona || ''}
                            onClick={() => toggleMesa(u, m.id)}
                            className={`h-7 min-w-8 px-2 rounded-lg text-[12.5px] font-medium border ring-focus transition-colors disabled:opacity-40 ${
                              on || porZona ? 'bg-teal-500/15 text-teal-700 dark:text-teal-300 border-teal-500/40'
                                            : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800'}`}>
                            {m.nombre}
                          </button>
                        )
                      })}
                    </div>
                  </Card>
                )
              })}
            </div>
          ) : (
            <Card className="overflow-hidden">
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-[11.5px] uppercase tracking-wide text-slate-500 border-b border-slate-200 dark:border-slate-800">
                      <th className="px-4 py-2.5 font-medium">Mesa</th>
                      <th className="px-4 py-2.5 font-medium">Zona</th>
                      <th className="px-4 py-2.5 font-medium">La atiende</th>
                    </tr>
                  </thead>
                  <tbody>
                    {mesas.map((m) => (
                      <tr key={m.id} className="border-b border-slate-100 dark:border-slate-800/70">
                        <td className="px-4 py-2.5 font-medium">{m.nombre}</td>
                        <td className="px-4 py-2.5 text-slate-500">{m.zona || '—'}</td>
                        <td className="px-4 py-2.5">
                          <div className="flex flex-wrap gap-1.5">
                            {mesoneros.map((u) => {
                              const uid = u.usuarioId || u.id
                              const a = asignacionDe(uid)
                              const porZona = (a.zonas || []).some((x) => x.trim().toLowerCase() === (m.zona || '').trim().toLowerCase())
                              const on = cubre(a, m)
                              return (
                                <button key={uid} disabled={guardando === uid || porZona}
                                  title={porZona ? `Le corresponde por la zona ${m.zona}` : ''}
                                  onClick={() => toggleMesa(u, m.id)}
                                  className={`h-7 px-2.5 rounded-full text-[12px] font-medium border ring-focus transition-colors disabled:opacity-60 ${
                                    on ? 'bg-elerp-500 text-white border-elerp-500'
                                       : 'border-slate-200 dark:border-slate-700 text-slate-500 hover:bg-slate-50 dark:hover:bg-slate-800'}`}>
                                  {u.nombre.split(' ')[0]}
                                </button>
                              )
                            })}
                            {mesoneros.every((u) => !cubre(asignacionDe(u.usuarioId || u.id), m))
                              ? <span className="text-[12px] text-slate-400 self-center">cualquiera</span> : null}
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          )}
        </>
      )}
    </div>
  )
}

/* --- Pedir la cuenta: modo de división ---------------------------------- */

/* Dos formas, y la diferencia no es cosmética:
 *   · Juntos → UNA prefactura con los renglones enteros. Repartir entre comensales es un
 *     asunto del COBRO: la caja registra un pago por persona sobre la misma factura. Así
 *     la factura sale limpia ("3 × Spaghetti", no "1,5 ×") y el IGTF se calcula bien.
 *   · Por productos → una prefactura (y después una factura) POR PARTE. Es el caso del
 *     que necesita su propia factura con su RIF. */
function DivisionModal({ cuenta, busy, onClose, onConfirmar }) {
  const items = (cuenta.items || []).filter((it) => it.estado !== 'cancelado')
  const [modo, setModo] = useState('unica')
  const [comensales, setComensales] = useState(cuenta.comensales || 2)
  const [partes, setPartes] = useState(2)
  // itemID → nº de parte. Arranca todo en la parte 1: repartir es mover, no asignar
  // desde cero (y evita el error de dejar un renglón sin asignar).
  const [asignacion, setAsignacion] = useState(() => {
    const m = {}
    for (const it of items) m[it.id] = 1
    return m
  })

  const total = totalCuenta(cuenta)
  const nombresParte = (n) => `Parte ${n}`

  const totalDeParte = (n) => items
    .filter((it) => asignacion[it.id] === n)
    .reduce((a, it) => a + (it.cantidad || 0) * (it.precioUnitario || 0), 0)

  const vacias = Array.from({ length: partes }, (_, i) => i + 1).filter((n) => totalDeParte(n) <= 0)

  const confirmar = () => {
    if (modo === 'unica') {
      onConfirmar({ modo: 'unica', comensales: Number(comensales) || 1 })
      return
    }
    onConfirmar({
      modo: 'por_items',
      items: asignacion,
      nombres: Object.fromEntries(Array.from({ length: partes }, (_, i) => [String(i + 1), nombresParte(i + 1)])),
    })
  }

  return (
    <Modal open onClose={onClose} title={`Pedir la cuenta · Mesa ${cuenta.mesaNombre}`} width="max-w-2xl"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button size="lg" loading={busy} disabled={modo === 'por_items' && vacias.length > 0} onClick={confirmar}>
          {modo === 'unica' ? 'Generar prefactura' : `Generar ${partes} prefacturas`}
        </Button>
      </>}>
      <div className="space-y-4">
        <Segmented value={modo} onChange={setModo} options={[
          { value: 'unica', label: 'Pagan juntos' },
          { value: 'por_items', label: 'Dividir por productos' },
        ]} />

        {modo === 'unica' ? (
          <div className="space-y-3">
            <Field label="¿Entre cuántas personas reparten el pago?"
              hint="Es una referencia para la caja: se emite UNA factura y la caja registra un pago por persona. La factura no se parte.">
              <Input type="number" min="1" value={comensales}
                onChange={(e) => setComensales(e.target.value)} className="!w-28" />
            </Field>
            <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-3 text-[13px]">
              <div className="flex justify-between"><span className="text-slate-500">Consumo</span><span className="tnum font-medium">{fmtCurrency(total, 'VES')}</span></div>
              {Number(comensales) > 1 ? (
                <div className="flex justify-between mt-1">
                  <span className="text-slate-500">≈ por persona</span>
                  <span className="tnum font-medium">{fmtCurrency(total / (Number(comensales) || 1), 'VES')}</span>
                </div>
              ) : null}
              <div className="text-[11.5px] text-slate-400 mt-2">El total definitivo con IVA lo calcula la prefactura.</div>
            </div>
          </div>
        ) : (
          <div className="space-y-3">
            <Field label="¿En cuántas partes?" hint="Cada parte recibe su propia prefactura y, al cobrarla, su propia factura.">
              <Input type="number" min="2" max="10" value={partes}
                onChange={(e) => {
                  const n = Math.max(2, Math.min(10, Number(e.target.value) || 2))
                  setPartes(n)
                  // Los renglones que apuntaban a una parte que ya no existe vuelven a la 1.
                  setAsignacion((prev) => Object.fromEntries(
                    Object.entries(prev).map(([k, v]) => [k, v > n ? 1 : v])))
                }} className="!w-24" />
            </Field>
            <div className="border border-slate-200 dark:border-slate-800 rounded-xl divide-y divide-slate-100 dark:divide-slate-800">
              {items.map((it) => (
                <div key={it.id} className="flex items-center gap-3 px-3 py-2">
                  <div className="min-w-0 flex-1">
                    <div className="text-[13px] font-medium truncate">{it.cantidad}× {it.nombre}</div>
                    <div className="text-[11.5px] text-slate-500 tnum">{fmtCurrency((it.cantidad || 0) * (it.precioUnitario || 0), 'VES')}</div>
                  </div>
                  <div className="flex gap-1">
                    {Array.from({ length: partes }, (_, i) => i + 1).map((n) => (
                      <button key={n} onClick={() => setAsignacion((prev) => ({ ...prev, [it.id]: n }))}
                        aria-label={`Asignar ${it.nombre} a la parte ${n}`}
                        className={`${T.chip} border ring-focus transition-colors ${
                          asignacion[it.id] === n
                            ? 'bg-elerp-500 text-white border-elerp-500'
                            : 'border-slate-200 dark:border-slate-700 text-slate-500 hover:bg-slate-50 dark:hover:bg-slate-800'}`}>
                        {n}
                      </button>
                    ))}
                  </div>
                </div>
              ))}
            </div>
            <div className="grid grid-cols-2 gap-2 text-[13px]">
              {Array.from({ length: partes }, (_, i) => i + 1).map((n) => (
                <div key={n} className={`rounded-lg border p-2.5 flex justify-between ${
                  totalDeParte(n) <= 0
                    ? 'border-amber-300 bg-amber-50 dark:bg-amber-950/30 dark:border-amber-900'
                    : 'border-slate-200 dark:border-slate-700'}`}>
                  <span className="text-slate-500">{nombresParte(n)}</span>
                  <span className="tnum font-medium">{fmtCurrency(totalDeParte(n), 'VES')}</span>
                </div>
              ))}
            </div>
            {vacias.length > 0 ? (
              <div className="text-[12.5px] text-amber-700 dark:text-amber-300">
                {vacias.length === 1 ? `La ${nombresParte(vacias[0]).toLowerCase()} quedó vacía` : `Hay ${vacias.length} partes vacías`}:
                asigná al menos un renglón a cada una o reducí el número de partes.
              </div>
            ) : null}
          </div>
        )}
      </div>
    </Modal>
  )
}
