import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { vendibles } from '../lib/catalogo.js'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Card, Select, Segmented, Toggle, Empty, Input, Field, Modal, PageHeader, useToast, useConfirm, TableSkeleton } from '../components/primitives.jsx'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { RestauranteInicio } from './RestauranteInicio.jsx'
import { Reservaciones } from './Reservaciones.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { api } from '../lib/api.js'
import {
  dimensionDeMesa, aforoMaximo, ocupaCelda, cabeEn, tamanoAlArrastrar, aforoAjustado,
  MAX_CELDAS_MESA,
} from '../lib/plano.js'
import { TurnosSalon } from './TurnosSalon.jsx'
import { SelectorModelo, NotaCatalogo } from '../components/dispositivo.jsx'
import { precioEnBs, monedaDe } from '../lib/precio.js'
import { EtiquetaAlicuota } from '../components/producto.jsx'
import { fmtCurrency } from '../lib/format.js'

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

/* ESCALA TÁCTIL de la comandera.
 *
 * Esta pantalla se opera de pie, con la tablet en una mano y a veces con el pulgar. Un
 * objetivo chico se falla, y un mesero que falla dos veces vuelve a la libreta de papel.
 * WCAG 2.2 pide 44 px como mínimo; acá se usa 48-56 px, que es el cómodo. Vive en una
 * constante para que se ajuste en un solo lugar y no se desincronice entre modales. */
const T = {
  // Botón de solo icono (quitar un renglón, cerrar).
  icono: 'h-11 w-11 inline-flex items-center justify-center rounded-xl',
  // Ficha/chip pulsable (asignar parte, elegir zona).
  chip: 'h-12 min-w-[3rem] px-3 text-[15px] font-semibold rounded-xl',
  // Tarjeta pulsable (mesa, producto del menú).
  tarjeta: 'min-h-[5.5rem] p-3.5',
}

// Total de una cuenta de mesa: suma de sus renglones NO cancelados. Se recuperó del
// historial (se había perdido en un refactor y dejaba la pantalla en blanco con
// «totalCuenta is not defined»: la interfaz entera del módulo caía por un helper).
// Estado de cada RENGLÓN de la cuenta (recuperados del historial: se perdieron en un
// refactor y dejaban la comandera en blanco al agregar un producto).
const ITEM_COLOR = {
  pendiente: 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300',
  en_cocina: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
  listo: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  servido: 'bg-elerp-100 text-elerp-700 dark:bg-elerp-900/40 dark:text-elerp-200',
  cancelado: 'bg-slate-100 text-slate-400 line-through dark:bg-slate-800',
}
const ITEM_LABEL = { pendiente: 'Por enviar', en_cocina: 'En cocina', listo: 'Listo', servido: 'Servido', cancelado: 'Anulado' }
const totalCuenta = (c) => (c?.items || []).reduce((a, it) => a + (it.estado === 'cancelado' ? 0 : (it.precioUnitario || 0) * (it.cantidad || 0)), 0)
// Lo que falta por facturar en la mesa. El tablero muestra ESTO y no el consumo: con
// facturación por partes, una mesa donde uno ya pagó seguiría enseñando su plato.
const pendienteCuenta = (c) => (c?.items || []).reduce(
  (a, it) => a + (it.estado === 'cancelado' || it.prefacturaId ? 0 : (it.precioUnitario || 0) * (it.cantidad || 0)), 0)

// `soloMesonero: true` marca las pestañas que también alcanza el mesonero. El resto son
// de administración o de cocina: mostrárselas al mesero no aporta y confunde (el sidebar
// ya se las oculta; acá se hace lo mismo con las pestañas del encabezado).
const TABS = [
  { id: 'inicio', label: 'Resumen del salón', icon: <Icon.Activity size={15} /> },
  { id: 'comandera', label: 'Comandera', icon: <Icon.ClipboardList size={15} />, mesonero: true },
  { id: 'reservas', label: 'Reservaciones', icon: <Icon.Users size={15} />, mesonero: true },
  { id: 'mesas', label: 'Mapa de mesas', icon: <Icon.Utensils size={15} /> },
  { id: 'turnos', label: 'Turnos del salón', icon: <Icon.Clock size={15} />, mesonero: true },
  { id: 'mesoneros', label: 'Mesoneros y asignación', icon: <Icon.Users size={15} /> },
  { id: 'cocina', label: 'Cocina', icon: <Icon.Activity size={15} /> },
  { id: 'platos', label: 'Platos y recetas', icon: <Icon.Boxes size={15} /> },
  { id: 'impresora', label: 'Impresoras comanderas', icon: <Icon.Printer size={15} /> },
]

// Un subtítulo POR PESTAÑA. Antes era uno fijo que hablaba del mapa y de la impresora:
// al mesonero —que solo alcanza la Comandera— le ofrecía dos cosas que no puede hacer, y
// en las otras cuatro pestañas describía algo distinto de lo que se estaba viendo.
const SUBTITULO = {
  inicio: 'Cómo está el salón ahora, qué pasa en cocina y cómo va el día. Desde acá saltas a cada sección.',
  comandera: 'Toma el pedido de cada mesa y envíalo a cocina. Cuando pidan la cuenta, prefactura y la caja cobra.',
  reservas: 'La agenda del salón: toma reservas y, cuando llegan, verifica por nombre o cédula y siéntalos (se abre su cuenta).',
  mesas: 'Diseña el salón sobre una grilla: ubica las mesas, bloquea los espacios donde no puede haber ninguna y fija cuántas personas caben.',
  turnos: 'Quién entra a trabajar: toca tu nombre y teclea tu PIN. Cada turno lo autoriza un supervisor, y al terminar queda registrado con lo que atendiste.',
  mesoneros: 'Asigna a cada mesonero las mesas o las zonas que atiende. Sin asignar, cualquiera atiende cualquier mesa.',
  cocina: 'Las comandas entrantes en vivo, con su nota y su tiempo de espera. Marca cada plato listo cuando salga.',
  platos: 'Los platos con su receta (escandallo): al venderlos se descuentan sus insumos del inventario.',
  impresora: 'Las comanderas por área —cocina, barra, postres— y qué rubro de productos sale por cada una.',
}

export function Restaurante({ route }) {
  const { ui } = useUI()
  const esMesonero = ui.rol === 'mesonero'
  const tabs = esMesonero ? TABS.filter((t) => t.mesonero) : TABS
  const inicial = esMesonero ? 'comandera' : 'inicio'
  const [tab, setTab] = useState((route || '').split(':')[1] || inicial)
  useEffect(() => {
    const pedido = (route || '').split(':')[1] || inicial
    // Un mesonero que llegue por URL a una pestaña que no le toca va a la Comandera.
    setTab(tabs.some((t) => t.id === pedido) ? pedido : inicial)
  }, [route, esMesonero])
  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader breadcrumb={['Restaurante', TABS.find((t) => t.id === tab)?.label]} title="Restaurante"
        sub={SUBTITULO[tab] || SUBTITULO.comandera}
        tabs={tabs} activeTab={tab} onTab={setTab} />
      {tab === 'inicio' ? <RestauranteInicio irA={setTab} /> : null}
      {tab === 'comandera' ? <Comandera /> : null}
      {tab === 'reservas' ? <Reservaciones /> : null}
      {tab === 'mesas' ? <MapaMesas /> : null}
      {tab === 'turnos' ? <TurnosSalon /> : null}
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

  // Una mesa de más capacidad ocupa más celdas. La regla es ESPEJO de
  // `mesa.Dimension` en el backend (mismo patrón que el dígito verificador del
  // RIF): el servidor la vuelve a aplicar y rechaza los planos encimados, así
  // que si las dos se separan, guardar falla.
  //
  // Se DERIVA de la capacidad y no se guarda: guardarla dejaría el tamaño viejo
  // al cambiar la capacidad, y el mapa mentiría.
  const dimension = (m) => dimensionDeMesa(m)
  // El ancho se recorta al borde de la grilla: una mesa larga en la última
  // columna se dibuja angosta en vez de desbordar el plano.
  const dimensionVisible = (m) => {
    const [dc, df] = dimension(m)
    return [
      Math.max(1, Math.min(dc, plano.columnas - (m.columna || 0))),
      Math.max(1, Math.min(df, plano.filas - (m.fila || 0))),
    ]
  }
  // Ocupa mira TODA la superficie, no solo la esquina: sin esto se podría soltar
  // una mesa sobre la mitad de un mesón, que a simple vista parece celda libre.
  const mesaEn = (c, r) => (mesas || []).find((m) => ocupaCelda(m, c, r))
  // La esquina (donde se dibuja la mesa) frente a una celda cubierta por ella.
  const anclaEn = (c, r) => (mesas || []).find((m) => (m.columna || 0) === c && (m.fila || 0) === r)
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

  /* ARRASTRE Y REDIMENSIONADO, en vivo.
   *
   * Antes el arrastre no mostraba nada hasta soltar: la mesa saltaba al final y
   * un destino inválido simplemente no hacía nada, sin decir por qué. Dibujar el
   * gesto MIENTRAS ocurre es lo que lo vuelve fluido — se ve a dónde va y si
   * cabe, antes de soltar.
   *
   * El estado vivo va en `gesto` (React) y el puntero en un ref: el ref evita
   * releer el estado en cada `pointermove`, y el estado es lo que pinta.
   *
   * `setPointerCapture` es lo que hace que el gesto NO se pierda al salir de la
   * mesa —que es justo lo que pasa al agrandarla— sin tener que escuchar en
   * window y limpiar a mano.
   */
  const gesto = useRef(null)
  const [vista, setVista] = useState(null) // {id, tipo, c, r, dc, df, valido}
  const gridRef = useRef(null)

  // celdaDe traduce un punto de la pantalla a celda de la grilla.
  const celdaDe = (e) => {
    const rect = gridRef.current.getBoundingClientRect()
    return {
      c: Math.floor((e.clientX - rect.left) / cel),
      r: Math.floor((e.clientY - rect.top) / cel),
    }
  }

  // cabe: ¿esta superficie está libre, ignorando la propia mesa? Es la misma
  // regla que el servidor aplica al guardar (rechaza planos encimados), así que
  // sin ella el gesto parecería funcionar y fallaría al guardar.
  const cabe = (id, c, r, dc, df) => cabeEn({ mesas: mesas || [], plano }, id, c, r, dc, df)

  const onMesaDown = (e, m, tipo = 'mover') => {
    if (!puedeEditar || modo !== 'mesas') { setSel(m.id); return }
    e.preventDefault(); e.stopPropagation()
    setSel(m.id)
    const [dc, df] = dimension(m)
    const origen = celdaDe(e)
    gesto.current = {
      id: m.id, tipo, dc, df,
      // Desfase entre la esquina de la mesa y la celda donde se agarró: sin esto
      // la mesa salta bajo el cursor al empezar a moverla, que es exactamente lo
      // que se sentía tosco.
      offC: origen.c - (m.columna || 0), offR: origen.r - (m.fila || 0),
      col: m.columna || 0, fil: m.fila || 0,
      movido: false,
    }
    try { e.currentTarget.setPointerCapture(e.pointerId) } catch { /* sin captura, el gesto igual funciona */ }
    const inicial = { id: m.id, tipo, c: m.columna || 0, r: m.fila || 0, dc, df, valido: true }
    gesto.current.v = inicial
    setVista(inicial)
  }

  const onMesaMove = (e) => {
    const g = gesto.current
    if (!g || !gridRef.current) return
    const { c, r } = celdaDe(e)
    let v
    if (g.tipo === 'mover') {
      const nc = c - g.offC, nr = r - g.offR
      v = { id: g.id, tipo: g.tipo, c: nc, r: nr, dc: g.dc, df: g.df }
    } else {
      // Redimensionar: la esquina se queda quieta y el lado sigue al cursor. El
      // mínimo es un cuadro —una mesa de cero no existe— y el máximo, el mismo
      // que acota el servidor.
      const [dc, df] = tamanoAlArrastrar({ tipo: g.tipo, col: g.col, fila: g.fil, dc: g.dc, df: g.df }, c, r)
      v = { id: g.id, tipo: g.tipo, c: g.col, r: g.fil, dc, df }
    }
    // Se repinta solo cuando cambia de celda: un setState por píxel haría
    // justo lo contrario de fluido.
    setVista((prev) => {
      if (prev && prev.c === v.c && prev.r === v.r && prev.dc === v.dc && prev.df === v.df) return prev
      g.movido = true
      const siguiente = { ...v, valido: cabe(g.id, v.c, v.r, v.dc, v.df) }
      g.v = siguiente
      return siguiente
    })
  }

  const onMesaUp = () => {
    const g = gesto.current
    gesto.current = null
    // Se lee del ref, no del estado: al soltar, el último `setVista` puede no
    // haber repintado todavía y se aplicaría la posición anterior.
    const v = g?.v
    setVista(null)
    if (!g || !v) return
    // Un toque sin desplazamiento es una selección, no un movimiento.
    if (!g.movido) return
    if (!v.valido) {
      toast({ title: 'Ahí no cabe', body: 'La mesa se sale del plano o pisa otra mesa.', kind: 'warn' })
      return
    }
    setMesas((ms) => ms.map((m) => {
      if (m.id !== g.id) return m
      // Al achicar, el aforo se recorta al nuevo tope: dejarlo por encima haría
      // que el servidor rechazara el guardado con un número que la pantalla
      // mostraba como válido.
      return {
        ...m, columna: v.c, fila: v.r, anchoCeldas: v.dc, altoCeldas: v.df,
        capacidad: aforoAjustado(m.capacidad, v.dc, v.df),
      }
    }))
    setDirty(true)
  }

  const guardar = async () => {
    setBusy(true)
    try {
      const ms = (mesas || []).map((m) => ({ ...m, columna: clamp(m.columna || 0, 0, plano.columnas - 1), fila: clamp(m.fila || 0, 0, plano.filas - 1) }))
      await api.guardarPlanoSalon({ filas: plano.filas, columnas: plano.columnas, bloqueadas: plano.bloqueadas })
      // El tamaño viaja junto con la posición: en el plano se redimensiona
      // arrastrando, así que mover y agrandar son el mismo gesto y tienen que
      // guardarse en la misma llamada o el plano queda a medias.
      await api.guardarMapaMesas(ms.map((m) => {
        const [dc, df] = dimensionDeMesa(m)
        return { id: m.id, columna: m.columna, fila: m.fila, anchoCeldas: dc, altoCeldas: df }
      }))
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
              const cubierta = mesaEn(c, r); const m = anclaEn(c, r); const blk = bloqueada(c, r)
              // Una celda TAPADA por una mesa grande (pero que no es su esquina)
              // no se dibuja: la mesa ya la abarca con su span.
              if (cubierta && !m) return null
              const [dc, df] = m ? dimensionVisible(m) : [1, 1]
              const col = m ? colorEstado(m.estado) : null
              const activo = m && sel === m.id
              // La mesa que se está arrastrando se atenúa: lo que se mira es la
              // vista previa, y dos copias opacas a la vez confunden.
              const arrastrandoEsta = !!(m && vista && vista.id === m.id && vista.tipo === 'mover')
              return (
                <div key={c + '-' + r} onPointerDown={() => (m ? null : clicCelda(c, r))}
                  className="border border-slate-100 dark:border-slate-800 flex items-center justify-center"
                  style={{
                    gridColumn: `${c + 1} / span ${dc}`, gridRow: `${r + 1} / span ${df}`,
                    background: blk ? 'repeating-linear-gradient(45deg,#e2e8f0,#e2e8f0 4px,#cbd5e1 4px,#cbd5e1 7px)' : 'transparent',
                    cursor: puedeEditar && !m ? 'pointer' : 'default',
                  }}>
                  {m ? (
                    <div onPointerDown={(e) => onMesaDown(e, m)} onPointerMove={onMesaMove}
                      onPointerUp={onMesaUp} onPointerCancel={onMesaUp} onClick={() => setSel(m.id)}
                      className="relative flex flex-col items-center justify-center touch-none"
                      style={{ width: cel * dc - 8, height: cel * df - 8, background: col.bg, border: `2px solid ${activo ? '#6A2CF0' : col.border}`, borderRadius: m.forma === 'redonda' ? '50%' : 8, color: col.text, cursor: puedeEditar && modo === 'mesas' ? (arrastrandoEsta ? 'grabbing' : 'grab') : 'pointer', boxShadow: activo ? '0 0 0 3px rgba(106,44,240,.18)' : 'none', opacity: arrastrandoEsta ? 0.4 : 1 }}>
                      <span className="font-display font-bold leading-none" style={{ fontSize: Math.max(12, cel * 0.24) }}>{m.nombre}</span>
                      <span className="inline-flex items-center gap-0.5 opacity-80" style={{ fontSize: Math.max(9, cel * 0.16) }}><Icon.Users size={Math.max(9, cel * 0.16)} /> {m.capacidad || 0}</span>

                      {/* TIRADORES DE TAMAÑO, solo en la mesa seleccionada: uno
                          por lado y uno en la esquina. Aparecen al seleccionar y
                          no siempre porque doce mesas con seis tiradores cada
                          una convierten el plano en un alfiletero. */}
                      {puedeEditar && modo === 'mesas' && activo ? TIRADORES.map((t) => (
                        <span key={t.tipo} title={t.titulo}
                          onPointerDown={(e) => onMesaDown(e, m, t.tipo)} onPointerMove={onMesaMove}
                          onPointerUp={onMesaUp} onPointerCancel={onMesaUp}
                          className="absolute bg-white dark:bg-slate-900 border-2 border-elerp-500 rounded-full touch-none"
                          style={{ width: 12, height: 12, cursor: t.cursor, ...t.pos }} />
                      )) : null}
                    </div>
                  ) : blk && puedeEditar ? (
                    <Icon.CircleX size={Math.max(12, cel * 0.28)} className="text-slate-400" />
                  ) : null}
                </div>
              )
            }))}

            {/* VISTA PREVIA del gesto: dónde va a quedar la mesa y si cabe. Es
                lo que vuelve el arrastre legible — antes no se veía nada hasta
                soltar, y un destino inválido no decía por qué no pasaba nada. */}
            {vista ? (
              <div className="pointer-events-none rounded-lg relative"
                style={{
                  gridColumn: `${clamp(vista.c, 0, plano.columnas - 1) + 1} / span ${Math.min(vista.dc, plano.columnas)}`,
                  gridRow: `${clamp(vista.r, 0, plano.filas - 1) + 1} / span ${Math.min(vista.df, plano.filas)}`,
                  border: `2px dashed ${vista.valido ? '#6A2CF0' : '#B3362C'}`,
                  background: vista.valido ? 'rgba(106,44,240,.10)' : 'rgba(179,54,44,.10)',
                  margin: 3,
                }}>
                <span className="absolute -mt-0.5 ml-1 num font-semibold"
                  style={{ fontSize: 11, color: vista.valido ? '#6A2CF0' : '#B3362C' }}>
                  {vista.dc}×{vista.df} · {aforoMaximo(vista.dc, vista.df)}p
                </span>
              </div>
            ) : null}
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

// Panel de edición de una mesa. La posición y el TAMAÑO se editan arrastrando
// la mesa y sus bordes en el plano; acá van los datos.
// La geometría (cuadros, aforo por cuadro, solapes) vive en lib/plano.js.

/* TIRADORES de redimensionado. Se dibujan sobre la mesa seleccionada: derecha
 * (ancho), abajo (alto) y esquina (los dos a la vez). Van con el cursor de
 * redimensionar del sistema, que es la única pista universal de «esto se
 * arrastra». */
const TIRADORES = [
  { tipo: 'ancho', titulo: 'Ensanchar', cursor: 'ew-resize', pos: { right: -7, top: '50%', marginTop: -6 } },
  { tipo: 'alto', titulo: 'Alargar', cursor: 'ns-resize', pos: { bottom: -7, left: '50%', marginLeft: -6 } },
  { tipo: 'ambos', titulo: 'Redimensionar', cursor: 'nwse-resize', pos: { right: -7, bottom: -7 } },
]

/* TamanoMesa son los controles de ampliación. Al agrandar sube el tope de aforo;
 * al achicar, el aforo se recorta solo al nuevo tope — así nunca queda un número
 * que el servidor va a rechazar al guardar. */
function TamanoMesa({ ancho, alto, disabled, onCambio }) {
  const paso = (dc, df) => {
    const a = clamp(ancho + dc, 1, MAX_CELDAS_MESA)
    const b = clamp(alto + df, 1, MAX_CELDAS_MESA)
    onCambio(a, b)
  }
  const btn = (etiqueta, dc, df, titulo) => (
    <button type="button" disabled={disabled} title={titulo} onClick={() => paso(dc, df)}
      className="h-7 w-7 rounded-lg border border-slate-200 dark:border-slate-700 text-[13px] font-semibold hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-40">
      {etiqueta}
    </button>
  )
  return (
    <div>
      <div className="text-[12px] font-medium text-slate-500 mb-1">Tamaño en cuadros</div>
      <div className="flex items-center gap-2 flex-wrap">
        <div className="flex items-center gap-1">
          {btn('−', -1, 0, 'Angostar')}
          <span className="num text-[13px] w-6 text-center">{ancho}</span>
          {btn('+', 1, 0, 'Ensanchar')}
        </div>
        <span className="text-slate-400 text-[12px]">×</span>
        <div className="flex items-center gap-1">
          {btn('−', 0, -1, 'Acortar')}
          <span className="num text-[13px] w-6 text-center">{alto}</span>
          {btn('+', 0, 1, 'Alargar')}
        </div>
        <span className="text-[11.5px] text-slate-400">
          admite hasta <strong>{aforoMaximo(ancho, alto)}</strong> personas
        </span>
      </div>
    </div>
  )
}

function PanelMesa({ mesa, puedeEditar, onGuardado, onEliminar, toast }) {
  const [dimA, dimB] = dimensionDeMesa(mesa)
  const [f, setF] = useState({
    nombre: mesa.nombre, zona: mesa.zona || '', capacidad: mesa.capacidad || 0,
    forma: mesa.forma || 'cuadrada', anchoCeldas: dimA, altoCeldas: dimB,
  })
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))
  const tope = aforoMaximo(f.anchoCeldas, f.altoCeldas)
  // Al achicar la mesa el aforo se recorta solo: dejarlo por encima del tope
  // haría que el servidor rechazara el guardado con un número que la pantalla
  // mostraba como válido.
  const cambiarTamano = (a, b) => setF((s) => ({
    ...s, anchoCeldas: a, altoCeldas: b,
    capacidad: aforoAjustado(s.capacidad, a, b),
  }))
  const guardar = async () => {
    setBusy(true)
    try {
      await api.actualizarMesa(mesa.id, { nombre: f.nombre, zona: f.zona, capacidad: Number(f.capacidad) || 0, forma: f.forma, columna: mesa.columna, fila: mesa.fila, anchoCeldas: f.anchoCeldas, altoCeldas: f.altoCeldas })
      await onGuardado(); toast({ title: 'Mesa actualizada', body: f.nombre })
    } catch (e) { toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy(false) }
  }
  return (
    <div className="space-y-3">
      <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Mesa seleccionada</div>
      <Field label="Nombre / número"><Input value={f.nombre} disabled={!puedeEditar} onChange={(e) => set('nombre', e.target.value)} /></Field>
      <Field label="Zona / salón"><Input value={f.zona} disabled={!puedeEditar} onChange={(e) => set('zona', e.target.value)} placeholder="Salón principal, Terraza…" /></Field>
      <TamanoMesa ancho={f.anchoCeldas} alto={f.altoCeldas} disabled={!puedeEditar} onCambio={cambiarTamano} />
      <div className="grid grid-cols-2 gap-2">
        <Field label="Capacidad" hint={`máx. ${tope}`}>
          <Input type="number" min={0} max={tope} value={f.capacidad} disabled={!puedeEditar}
            onChange={(e) => set('capacidad', Math.min(Number(e.target.value) || 0, tope))} />
        </Field>
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
  const [f, setF] = useState({ nombre: '', zona: '', capacidad: 4, forma: 'cuadrada', anchoCeldas: 1, altoCeldas: 1 })
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))
  const tope = aforoMaximo(f.anchoCeldas, f.altoCeldas)
  const cambiarTamano = (a, b) => setF((s) => ({
    ...s, anchoCeldas: a, altoCeldas: b,
    capacidad: aforoAjustado(s.capacidad, a, b),
  }))
  const crear = async () => {
    if (!f.nombre.trim()) { toast({ title: 'Ponle un nombre o número a la mesa', kind: 'warn' }); return }
    setBusy(true)
    try {
      const m = await api.crearMesa({ nombre: f.nombre.trim(), zona: f.zona.trim(), capacidad: Number(f.capacidad) || 0, forma: f.forma, columna, fila, anchoCeldas: f.anchoCeldas, altoCeldas: f.altoCeldas })
      toast({ title: 'Mesa agregada', body: m.nombre }); onCreada(m)
    } catch (e) { toast({ title: 'No se pudo agregar', body: e?.message || 'Error', kind: 'error' }); setBusy(false) }
  }
  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Utensils size={18} />} title="Agregar mesa"
      footer={<><Button variant="ghost" onClick={onClose}>Cancelar</Button><Button loading={busy} onClick={crear}>Agregar</Button></>}>
      <div className="space-y-3">
        <Field label="Nombre / número"><Input autoFocus value={f.nombre} onChange={(e) => set('nombre', e.target.value)} placeholder="1, Terraza 3, Barra 2…" /></Field>
        <Field label="Zona / salón"><Input value={f.zona} onChange={(e) => set('zona', e.target.value)} placeholder="Salón principal, Terraza…" /></Field>
        <TamanoMesa ancho={f.anchoCeldas} alto={f.altoCeldas} onCambio={cambiarTamano} />
        <div className="grid grid-cols-2 gap-2">
          <Field label="Capacidad" hint={`máx. ${tope}`}>
            <Input type="number" min={0} max={tope} value={f.capacidad}
              onChange={(e) => set('capacidad', Math.min(Number(e.target.value) || 0, tope))} />
          </Field>
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
        Cada <strong>comandera</strong> es un puesto donde salen las comandas: cocina, barra, postres. Declara de qué
        <strong> rubros</strong> imprime cada una y el pedido se reparte solo. La <strong>predeterminada</strong> recibe
        lo que no encaje en ningún rubro, para que nada se quede sin imprimir.
      </div>

      {lista.length === 0 ? (
        <Empty title="Todavía no hay comanderas"
          body="Sin comanderas la comanda igual se genera y se ve en pantalla. Agrega una por cada puesto de preparación."
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
                  {[imp.marca, imp.modelo].filter(Boolean).length
                    ? ` · ${[imp.marca, imp.modelo].filter(Boolean).join(' ')}`
                    : ''}
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
    marca: imp.marca || '', modelo: imp.modelo || '',
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
        id: f.id, nombre: f.nombre, marca: f.marca, modelo: f.modelo,
        conexion: f.conexion, host: f.host,
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

        {/* Qué EQUIPO es (distinto del puesto: el puesto es "Barra", el equipo es
            una "Epson TM-T20III"). Elegirlo del catálogo precarga el ancho del
            rollo y si se conecta por red o por el equipo — que es lo que a nadie
            en una cocina le consta de memoria. */}
        <SelectorModelo tipo="comandera" marca={f.marca} modelo={f.modelo}
          onChange={({ marca, modelo }, ficha) => setF((s) => ({
            ...s, marca, modelo,
            // Solo en alta: en una comandera ya configurada, cambiar el modelo no
            // puede reescribir un ancho o una conexión que ya funcionan.
            anchoMm: ficha && !s.id ? (ficha.anchoMm || s.anchoMm) : s.anchoMm,
            conexion: ficha && !s.id ? (ficha.conexion || s.conexion) : s.conexion,
          }))} />

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

        <NotaCatalogo tipo="comandera" />

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
  const [divisionOpen, setDivisionOpen] = useState(false)
  const esMesonero = ui.rol === 'mesonero'

  const cuentaDeMesa = (mesaId) => cuentasAbiertas.find((c) => c.mesaId === mesaId)

  // Mesas APARTADAS ahora por una reserva cercana. Es lo que hace que la reserva sirva:
  // que a las 8 la mesa esté libre. Se refresca solo cada pocos minutos —una reserva no
  // cambia de un segundo a otro— y si el módulo no responde, el tablero sigue igual.
  const [reservadas, setReservadas] = useState({})
  useEffect(() => {
    let vivo = true
    const traer = () => api.mesasReservadas()
      .then((r) => { if (vivo) setReservadas(r || {}) })
      .catch(() => {})
    traer()
    const t = setInterval(traer, 120000)
    return () => { vivo = false; clearInterval(t) }
  }, [])

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
            : 'Puedes tomarla igual —queda registrado quién la atendió— o dejársela a su mesonero.',
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
      toast({
        title: 'Factura pedida',
        body: `La caja la ve como «Mesa ${cuenta.mesaNombre}» en el Punto de Venta.`,
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

  if (!mesas.length) {
    return <Empty icon={<Icon.Utensils size={22} />} title="Aún no hay mesas" body="Crea el salón en «Mapa de mesas» para empezar a tomar pedidos." />
  }
  if (cuenta) {
    return (<>
      <CuentaDetalle cuenta={cuenta} busy={busy} onVolver={() => setCuenta(null)}
        onAgregar={() => setMenuOpen(true)} onCancelar={cancelarItem} onEnviar={enviar} onCerrar={cerrar}
        onPedirCuenta={() => setDivisionOpen(true)}
        onVolverAServicio={volverAServicio}
        puedeAnularEnviado={ui.rol !== 'mesonero'} />
      {divisionOpen ? <SolicitarFacturaModal cuenta={cuenta} clientes={db.CLIENTES || []} busy={busy}
        onClose={() => setDivisionOpen(false)} onConfirmar={pedirCuenta} /> : null}
      {menuOpen ? <MenuProductos productos={vendibles(db.PRODUCTOS)} monedaEmpresa={monedaEmpresa}
        onAgregar={agregarProducto} onClose={() => setMenuOpen(false)} /> : null}
      {comanda ? <ComandaModal cuenta={cuenta} comanda={comanda} onClose={() => setComanda(null)} /> : null}
    </>)
  }
  // Tablero de mesas
  const c = (e) => (ESTADO_COLOR[e] || ESTADO_COLOR.libre)
  return (
    <div>
      <div className="text-[12.5px] text-slate-500 dark:text-slate-400 mb-3">Toca una mesa para abrir su cuenta y tomar el pedido. Cada envío a cocina genera una comanda.</div>
      <div className="grid gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
        {mesas.map((m) => {
          const cta = cuentaDeMesa(m.id)
          const reservada = !cta ? reservadas[m.id] : null
          const est = cta ? 'ocupada' : reservada ? 'reservada' : (m.estado || 'libre')
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
              <div className="mt-2 text-[15px] font-bold">{cta ? fmtCurrency(pendienteCuenta(cta), 'VES') : col.label}</div>
              {reservada ? (
                <div className="text-[11.5px] opacity-80 truncate">{reservada.hora} · {reservada.nombre}</div>
              ) : null}
              {cta && pendienteCuenta(cta) !== totalCuenta(cta) ? (
                <div className="text-[11px] opacity-70">de {fmtCurrency(totalCuenta(cta), 'VES')} · parte ya facturada</div>
              ) : null}
            </button>
          )
        })}
      </div>
    </div>
  )
}

function CuentaDetalle({ cuenta, busy, onVolver, onAgregar, onCancelar, onEnviar, onCerrar, onPedirCuenta, onVolverAServicio, puedeAnularEnviado }) {
  const items = cuenta.items || []
  const pendientes = items.filter((it) => it.estado === 'pendiente')
  // Solicitudes de facturación ya emitidas y renglones que todavía nadie pidió. Una
  // mesa puede tener las dos cosas a la vez: uno pagó y se fue, el otro sigue comiendo.
  const solicitudes = (cuenta.prefacturas || []).length
  const sinPedir = items.filter((it) => it.estado !== 'cancelado' && !it.prefacturaId)
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
            <div className="text-[11px] text-slate-500">{solicitudes ? 'Falta por facturar' : 'Total'}</div>
            <div className="font-display font-bold text-[18px]">{fmtCurrency(pendienteCuenta(cuenta), 'VES')}</div>
            {solicitudes ? (
              <div className="text-[11px] text-slate-400">consumo {fmtCurrency(totalCuenta(cuenta), 'VES')}</div>
            ) : null}
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
                    {/* Ya entró en una solicitud: no se vuelve a pedir ni se toca. */}
                    {it.prefacturaId ? (
                      <span title="Ya está en una factura pedida"
                        className="text-[10.5px] px-2 py-0.5 rounded-full font-semibold bg-teal-50 text-teal-700 dark:bg-teal-950/50 dark:text-teal-300">Facturado</span>
                    ) : null}
                    <span className="text-[12.5px] font-medium tabular-nums w-20 text-right">{fmtCurrency((it.precioUnitario || 0) * (it.cantidad || 0), 'VES')}</span>
                    {/* Quitar: libre mientras el renglón NO fue a cocina. Ya enviado, el
                        mesonero no lo anula solo (cuesta comida) — lo autoriza la caja.
                        Al mesonero no se le muestra un botón que el servidor va a
                        rechazar: se le explica por qué no está. */}
                    {it.estado === 'cancelado' ? null
                      : it.estado === 'pendiente' || puedeAnularEnviado ? (
                        <button onClick={() => onCancelar(it)} title={it.estado === 'pendiente' ? 'Quitar' : 'Anular (ya está en cocina)'}
                          aria-label={`Quitar ${it.nombre}`}
                          className={`${T.icono} text-slate-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-950/40 shrink-0`}><Icon.Trash size={18} /></button>
                      ) : (
                        <span title="Ya está en cocina: pide a la caja que lo anule"
                          className={`${T.icono} text-slate-300 dark:text-slate-600 shrink-0 cursor-not-allowed`}>
                          <Icon.Lock size={16} />
                        </span>
                      )}
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="space-y-2">
        {solicitudes ? (
          /* Ya hay solicitudes en la caja. La mesa NO se bloquea: el que sigue comiendo
             puede pedir más y eso entra en la próxima solicitud. Se muestra qué está
             esperando la caja para que el mesonero sepa en qué va la mesa. */
          <div className="rounded-xl border border-teal-200 dark:border-teal-900/60 bg-teal-50 dark:bg-teal-950/40 p-3">
            <div className="flex items-center gap-1.5 font-semibold text-[13px] text-teal-900 dark:text-teal-100">
              <Icon.CircleCheck size={15} /> {solicitudes === 1 ? 'Factura pedida' : `${solicitudes} facturas pedidas`}
            </div>
            <div className="text-[12px] text-teal-800 dark:text-teal-200 mt-1 leading-relaxed">
              La caja la ve como «Mesa {cuenta.mesaNombre}» en el Punto de Venta y la cobra ahí.
              {sinPedir.length ? ` Quedan ${sinPedir.length} renglón(es) sin facturar.` : ' Al cobrarla, la mesa se libera sola.'}
            </div>
          </div>
        ) : null}

        <Button className="w-full" size="xl" icon={<Icon.Plus size={18} />} variant="secondary" onClick={onAgregar}>Agregar productos</Button>
        <Button className="w-full" size="xl" loading={busy} disabled={!pendientes.length} icon={<Icon.Send size={18} />} onClick={onEnviar}>
          Enviar a cocina{pendientes.length ? ` (${pendientes.length})` : ''}
        </Button>
        <Button className="w-full" size="xl" disabled={!sinPedir.length} icon={<Icon.Receipt size={18} />} onClick={onPedirCuenta}>
          {solicitudes ? 'Pedir factura de lo que falta' : 'Pedir la cuenta'}
        </Button>
        {solicitudes ? (
          <Button className="w-full" size="lg" variant="ghost" loading={busy} onClick={onVolverAServicio}>
            Anular lo pedido (volver a servicio)
          </Button>
        ) : null}
        <Button className="w-full" size="lg" variant="ghost" onClick={onCerrar}>Cerrar sin cobrar</Button>
        <div className="text-[11px] text-slate-400 px-1 pt-1">
          Puedes pedir la factura de toda la mesa o solo de unos renglones —cuando cada quien paga lo suyo— y
          repetir con el resto más tarde. El cobro es en el Punto de Venta; la mesa se cierra sola cuando ya no
          queda nada por cobrar.
        </div>
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
              <div className="text-[13px] text-slate-500 mt-1">{fmtCurrency(p.precio, monedaDe(p, monedaEmpresa))}<EtiquetaAlicuota producto={p} /></div>
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
  // Rubros que ya existen en el catálogo: una receta de postre se clasifica igual que
  // un postre de reventa, y así el ruteo por rubro sigue funcionando sin configurar nada.
  const rubros = [...new Set(productos.map((p) => (p.rubro || '').trim()).filter(Boolean))].sort((a, b) => a.localeCompare(b, 'es'))
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
                    <div className="text-[11.5px] text-slate-500">
                      {(p.receta || []).length} insumo(s)
                      {p.rubro ? ` · ${p.rubro}` : ''}
                    </div>
                    <div className="text-[11px] text-slate-400 truncate">
                      Sale por {(db.IMPRESORAS_COMANDAS || []).find((i) => i.id === p.comanderaId)?.nombre
                        || (p.rubro ? `la comandera de ${p.rubro}` : 'la comandera predeterminada')}
                    </div>
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
        rubros={rubros} comanderas={db.IMPRESORAS_COMANDAS || []}
        costoDe={costoDe} onClose={() => setForm(null)} onGuardado={() => { setForm(null); reload() }} toast={toast} /> : null}
    </div>
  )
}

function PlatoModal({ plato, insumos, monedaEmpresa, rubros = [], comanderas = [], costoDe, onClose, onGuardado, toast }) {
  const editar = !!plato
  const [f, setF] = useState(() => ({
    sku: plato?.sku || '', nombre: plato?.nombre || '', precio: plato?.precio || 0,
    exentoIva: !!plato?.exentoIva, receta: (plato?.receta || []).map((r) => ({ ...r })),
    rubro: plato?.rubro || '', comanderaId: plato?.comanderaId || '',
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
        await api.actualizarProducto(f.sku, {
          nombre: f.nombre.trim(), precio: Number(f.precio) || 0, exentoIva: f.exentoIva,
          esPlato: true, receta, rubro: f.rubro, comanderaId: f.comanderaId,
        })
      } else {
        const sku = (f.sku || ('PLATO-' + Date.now().toString(36).toUpperCase())).trim()
        await api.createProducto({
          sku, nombre: f.nombre.trim(), precio: Number(f.precio) || 0, moneda: monedaEmpresa,
          exentoIva: f.exentoIva, esPlato: true, receta, rubro: f.rubro, comanderaId: f.comanderaId,
        })
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
        <div className="grid grid-cols-2 gap-2">
          {/* Una receta no es solo comida de cocina: un postre, un jugo natural o un
              trago llevan receta igual. El rubro los clasifica y, si hace falta, la
              comandera manda el ticket al puesto que los prepara. */}
          <Field label="Rubro" hint="Clasifica el plato en la carta (Cocina, Postres, Bebidas…).">
            <Select value={f.rubro} onChange={(e) => set('rubro', e.target.value)}>
              <option value="">Sin rubro</option>
              {rubros.map((r) => <option key={r} value={r}>{r}</option>)}
            </Select>
          </Field>
          <Field label="¿Por qué comandera sale?"
            hint="Un postre puede ir a la barra de postres aunque comparta rubro con cocina.">
            <Select value={f.comanderaId} onChange={(e) => set('comanderaId', e.target.value)}>
              <option value="">Según su rubro</option>
              {comanderas.map((i) => <option key={i.id} value={i.id}>{i.nombre}</option>)}
            </Select>
          </Field>
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
          body="Invita a alguien con el rol Mesonero en Configuración › Usuarios y roles. Sin asignación, cualquier mesonero atiende cualquier mesa." />
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
/* Solicitar factura de una mesa.
 *
 * El mesonero elige QUÉ renglones entran en esta solicitud. Por defecto entran todos
 * los que nadie pidió todavía —"la cuenta, por favor" es el caso normal— pero si dos
 * comensales pagan por separado, marca los de uno, manda su solicitud, y después manda
 * la del otro. Cada solicitud es un documento propio y la caja las cobra por separado.
 *
 * Los datos del cliente son opcionales: si el mesonero los tomó en la mesa, la factura
 * sale a ese nombre sin que el cajero pregunte nada; si no, los pide la caja al cobrar.
 *
 * Repartir en partes IGUALES no parte el documento: es una referencia para que el
 * cajero cobre en varios pagos sobre una sola factura (y así el IVA y el IGTF salen
 * bien, y la factura no dice "1,5 × Spaghetti").
 */
function SolicitarFacturaModal({ cuenta, clientes = [], busy, onClose, onConfirmar }) {
  // Disponibles = vivos y sin pedir. Lo que ya se llevó otra solicitud no se ofrece.
  const items = (cuenta.items || []).filter((it) => it.estado !== 'cancelado' && !it.prefacturaId)
  const [sel, setSel] = useState(() => new Set(items.map((it) => it.id)))
  const [comensales, setComensales] = useState(1)
  const [clienteId, setClienteId] = useState('')

  const marcados = items.filter((it) => sel.has(it.id))
  const total = marcados.reduce((a, it) => a + (it.cantidad || 0) * (it.precioUnitario || 0), 0)
  const todo = marcados.length === items.length
  const alternar = (id) => setSel((prev) => {
    const n = new Set(prev)
    if (n.has(id)) n.delete(id); else n.add(id)
    return n
  })

  const confirmar = () => onConfirmar({
    modo: 'unica',
    comensales: Number(comensales) || 1,
    // Sin selección explícita el servidor toma todo lo que falte: se manda solo
    // cuando de verdad es un segmento, para que quede claro en la bitácora.
    seleccion: todo ? [] : marcados.map((it) => it.id),
    clienteId,
  })

  return (
    <Modal open onClose={onClose} title={`Pedir la cuenta · Mesa ${cuenta.mesaNombre}`} width="max-w-2xl"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button size="lg" loading={busy} disabled={marcados.length === 0} onClick={confirmar}>
          {todo ? 'Pedir la cuenta' : `Pedir por ${marcados.length} renglón(es)`}
        </Button>
      </>}>
      <div className="space-y-4">
        <div>
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-[13px] font-semibold text-slate-700 dark:text-slate-300">¿Qué se factura ahora?</span>
            <button onClick={() => setSel(todo ? new Set() : new Set(items.map((it) => it.id)))}
              className="text-[12px] text-elerp-600 hover:underline ring-focus rounded">
              {todo ? 'Desmarcar todo' : 'Marcar todo'}
            </button>
          </div>
          <div className="border border-slate-200 dark:border-slate-800 rounded-xl divide-y divide-slate-100 dark:divide-slate-800 max-h-64 overflow-auto">
            {items.map((it) => (
              <label key={it.id} className="flex items-center gap-3 px-3 py-2 cursor-pointer hover:bg-slate-50 dark:hover:bg-slate-800/50">
                <input type="checkbox" checked={sel.has(it.id)} onChange={() => alternar(it.id)}
                  className="w-4 h-4 accent-[#6A2CF0]" />
                <div className="min-w-0 flex-1">
                  <div className="text-[13px] font-medium truncate">{it.cantidad}× {it.nombre}</div>
                  {it.nota ? <div className="text-[11.5px] text-slate-400 truncate">{it.nota}</div> : null}
                </div>
                <span className="text-[12.5px] tabular-nums text-slate-500">{fmtCurrency((it.cantidad || 0) * (it.precioUnitario || 0), 'VES')}</span>
              </label>
            ))}
          </div>
          {!todo ? (
            <div className="text-[12px] text-slate-500 mt-1.5">
              Lo que dejes sin marcar se queda en la mesa y se puede pedir después, en otra factura.
            </div>
          ) : null}
        </div>

        <Field label="¿A nombre de quién?"
          hint="Opcional. Si no lo pones, el cajero pide el RIF al cobrar.">
          <Select value={clienteId} onChange={(e) => setClienteId(e.target.value)}>
            <option value="">Lo pide la caja (consumidor final)</option>
            {clientes.map((c) => <option key={c.id} value={c.id}>{c.nombre}{c.rif ? ` · ${c.rif}` : ''}</option>)}
          </Select>
        </Field>

        <Field label="¿Entre cuántas personas reparten el pago?"
          hint="Es una referencia para la caja: se emite UNA factura y se registra un pago por persona. La factura no se parte.">
          <Input type="number" min="1" value={comensales}
            onChange={(e) => setComensales(e.target.value)} className="!w-28" />
        </Field>

        <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-3 text-[13px]">
          <div className="flex justify-between"><span className="text-slate-500">Consumo de esta solicitud</span><span className="tnum font-medium">{fmtCurrency(total, 'VES')}</span></div>
          {Number(comensales) > 1 ? (
            <div className="flex justify-between mt-1">
              <span className="text-slate-500">≈ por persona</span>
              <span className="tnum font-medium">{fmtCurrency(total / (Number(comensales) || 1), 'VES')}</span>
            </div>
          ) : null}
          <div className="text-[11.5px] text-slate-400 mt-2">El total definitivo con IVA lo calcula la caja al cobrar.</div>
        </div>
      </div>
    </Modal>
  )
}
