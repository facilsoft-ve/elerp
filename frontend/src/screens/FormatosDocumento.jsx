import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Select, Segmented, Toggle, Empty, Input, Field, Modal, useToast, useConfirm, TableSkeleton } from '../components/primitives.jsx'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'

/* Configuración › Formatos de documento.
 *
 * Editor visual (arrastrar y soltar) de las plantillas con las que se imprime
 * cada tipo de documento: factura, cotización, nota de entrega, recibo,
 * presupuesto y orden de compra. Cada formato es un lienzo de un tamaño de papel
 * (carta, media carta, A4, tickets de 80/58 mm o a medida) sobre el que se
 * colocan bloques posicionados en milímetros. Se elige, por SEDE, cuál formato
 * usa cada tipo.
 *
 * La impresión ya usa estos formatos: al descargar/imprimir una factura o una
 * cotización, el documento sale EXACTAMENTE con el formato resuelto para la sede
 * (ver lib/plantillaImprimir). Los tipos aún sin conexión a impresión (nota de
 * entrega, recibo, presupuesto, orden) se diseñan aquí y caerán al motor cuando
 * su emisión lo consuma.
 */

// Roles que pueden modificar (crear/editar/borrar/asignar). El resto solo mira.
const PUEDE_EDITAR = ['dueno', 'desarrollador']

const TIPOS = [
  { id: 'factura', label: 'Factura' },
  { id: 'cotizacion', label: 'Cotización' },
  { id: 'nota_entrega', label: 'Nota de entrega' },
  { id: 'recibo', label: 'Recibo' },
  { id: 'presupuesto', label: 'Presupuesto' },
  { id: 'orden_compra', label: 'Orden de compra' },
]
const tipoLabel = (t) => TIPOS.find((x) => x.id === t)?.label || t

const PAPELES = [
  { id: 'carta', label: 'Carta · 216 × 279 mm', w: 215.9, h: 279.4 },
  { id: 'media_carta', label: 'Media carta · 216 × 140 mm', w: 215.9, h: 139.7 },
  { id: 'a4', label: 'A4 · 210 × 297 mm', w: 210, h: 297 },
  { id: 'ticket_80', label: 'Ticket · rollo 80 mm', w: 80, h: 200 },
  { id: 'ticket_58', label: 'Ticket · rollo 58 mm', w: 58, h: 200 },
  { id: 'custom', label: 'Personalizado', w: 0, h: 0 },
]
const papelLabel = (p) => PAPELES.find((x) => x.id === p)?.label || p

const TIPOS_BLOQUE = [
  { id: 'texto', label: 'Texto', icon: <Icon.Pencil size={14} /> },
  { id: 'campo', label: 'Campo', icon: <Icon.Tag size={14} /> },
  { id: 'tabla_items', label: 'Renglones', icon: <Icon.ClipboardList size={14} /> },
  { id: 'totales', label: 'Totales', icon: <Icon.Receipt size={14} /> },
  { id: 'separador', label: 'Separador', icon: <Icon.ArrowLeftRight size={14} /> },
  { id: 'logo', label: 'Logo', icon: <Icon.Sparkles size={14} /> },
  { id: 'imagen', label: 'Imagen', icon: <Icon.Image size={14} /> },
  { id: 'qr', label: 'QR', icon: <Icon.Command size={14} /> },
]

// Imagen embebida: formatos seguros (mapas de bits; nunca SVG) y peso máximo.
// Debe coincidir con la validación del backend (ImagenDataURISegura).
const IMAGEN_TIPOS_OK = ['image/png', 'image/jpeg', 'image/webp']
const IMAGEN_MAX_BYTES = 512 * 1024

// Valores de muestra para la previsualización de un bloque de campo, así el
// lienzo se ve como un documento real y no como una plantilla de marcadores.
const MUESTRA = {
  'emisor.nombre': 'Comercial La Económica, C.A.',
  'emisor.rif': 'J-40123456-7',
  'emisor.direccion': 'Av. Bolívar, Local 12, Caracas',
  'emisor.telefono': '0212-555-1234',
  'sede.nombre': 'Sede Principal',
  'cliente.nombre': 'Inversiones Andrade, C.A.',
  'cliente.rif': 'J-31987654-3',
  'cliente.direccion': 'C.C. El Este, Nivel PB, Caracas',
  'cliente.telefono': '0414-123-4567',
  'doc.tipo': 'FACTURA',
  'doc.numero': 'FL/C-000123',
  'doc.numeroControl': '00-0000456',
  'doc.fecha': '05/09/2026',
  'doc.vencimiento': '20/09/2026',
  'doc.moneda': 'Bs',
  'doc.tasa': 'Bs 132,50/US$',
  'doc.observaciones': 'Gracias por su compra. Pago a 15 días.',
  'doc.hash': '9f2a7c1e4b8d6035a1c9e2f7b04d8a3e5c6f19b2d7a0e4c8f3b1d6a9e2c5f7b0',
}

// Opciones (banderas) por tipo de bloque — espejan las constantes Op* del dominio.
const OPCIONES_TOTALES = [
  { key: 'mostrarTasa', label: 'Tasa de cambio (BCV)' },
  { key: 'mostrarDivisa', label: 'Total referencial en divisa (US$)' },
]
const OPCIONES_ITEMS = [
  { key: 'mostrarMoneda', label: 'Indicar la moneda en las columnas' },
  { key: 'marcarExento', label: 'Marcar productos exentos de IVA' },
]

const MM_POR_PT = 0.352777 // 1 punto = 1/72 pulgada
const ZOOM_MIN = 0.4 // 40 %
const ZOOM_MAX = 3   // 300 %
const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v))
const uid = () => 'b_' + Math.random().toString(36).slice(2, 9)

// Tamaño por defecto (en mm) de un bloque recién agregado, por tipo.
const NUEVO_BLOQUE = {
  texto: { w: 70, h: 8, texto: 'Texto', tamano: 10, alineacion: 'izquierda' },
  campo: { w: 70, h: 6, campo: 'doc.numero', tamano: 9, alineacion: 'izquierda' },
  tabla_items: { w: 180, h: 90 },
  totales: { w: 80, h: 32 },
  separador: { w: 180, h: 1 },
  logo: { w: 40, h: 18 },
  imagen: { w: 40, h: 25 },
  qr: { w: 22, h: 22 },
}

export function FormatosDocumento() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const puedeEditar = PUEDE_EDITAR.includes(ui.rol)

  const [rows, setRows] = useState(null) // null = cargando
  const [err, setErr] = useState(null)
  const [campos, setCampos] = useState([])
  const [editar, setEditar] = useState(null) // formato en edición | null
  const [nuevo, setNuevo] = useState(false)

  const cargar = useCallback(async () => {
    setErr(null)
    try { setRows(await api.formatos()) }
    catch (e) { setErr(e); setRows([]) }
  }, [])
  // Tras cualquier cambio se refresca la lista Y el bootstrap (db.FORMATOS), que es
  // de donde la impresión resuelve el formato de cada sede.
  const refrescar = useCallback(async () => { await cargar(); reload() }, [cargar, reload])
  useEffect(() => { cargar() }, [cargar])
  useEffect(() => { api.formatoCampos().then(setCampos).catch(() => setCampos([])) }, [])

  const sedes = (db.SEDES || []).filter((s) => s.activa !== false)

  if (editar) {
    return <EditorFormato formato={editar} campos={campos} puedeEditar={puedeEditar}
      empresa={db.EMPRESA || {}}
      onCerrar={() => setEditar(null)}
      onGuardado={(f) => { setEditar(null); refrescar() }} />
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3 mb-4">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          Diseña cómo se ven tus <strong>facturas, cotizaciones y demás documentos</strong>: arrastra
          los bloques, cambia el <strong>tamaño del papel</strong> y elige, por <strong>sede</strong>,
          cuál formato se usa. Los cambios rigen para lo que imprimas de aquí en adelante.
        </div>
        {puedeEditar ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setNuevo(true)}>Nuevo formato</Button>
        ) : null}
      </div>

      {rows === null ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
          <TableSkeleton rows={4} cols={4} />
        </div>
      ) : err ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar los formatos"
          body={String(err?.message || err)}
          cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : rows.length === 0 ? (
        <Empty icon={<Icon.FileText size={22} />} title="Sin formatos de documento"
          body="Crea el primer formato para diseñar cómo se imprimen tus facturas y cotizaciones."
          cta={puedeEditar ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNuevo(true)}>Crear formato</Button> : null} />
      ) : (
        <div className="space-y-5">
          <ListaFormatos rows={rows} puedeEditar={puedeEditar} onEditar={setEditar} onCambio={refrescar} toast={toast} />
          <AsignacionPorSede rows={rows} sedes={sedes} puedeEditar={puedeEditar} onCambio={refrescar} toast={toast} />
        </div>
      )}

      {nuevo ? (
        <NuevoFormatoModal onClose={() => setNuevo(false)}
          onCreado={(f) => { setNuevo(false); refrescar(); setEditar(f) }} toast={toast} />
      ) : null}
    </div>
  )
}

// ---- Lista de formatos, agrupada por tipo ----
function ListaFormatos({ rows, puedeEditar, onEditar, onCambio, toast }) {
  const confirm = useConfirm()
  const [busy, setBusy] = useState('')
  const porTipo = TIPOS.map((t) => ({ tipo: t, items: rows.filter((r) => r.tipo === t.id) })).filter((g) => g.items.length)

  const marcarDefault = async (f) => {
    setBusy(f.id)
    try { await api.formatoPredeterminado(f.id); await onCambio(); toast({ title: 'Formato predeterminado', body: f.nombre }) }
    catch (e) { toast({ title: 'No se pudo marcar', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy('') }
  }
  const duplicar = async (f) => {
    setBusy(f.id)
    try {
      await api.crearFormato({ nombre: f.nombre + ' (copia)', tipo: f.tipo, papel: f.papel, anchoMm: f.anchoMm, altoMm: f.altoMm, bloques: f.bloques })
      await onCambio(); toast({ title: 'Formato duplicado', body: f.nombre })
    } catch (e) { toast({ title: 'No se pudo duplicar', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy('') }
  }
  const eliminar = async (f) => {
    if (!(await confirm({ title: '¿Eliminar este formato?', body: `«${f.nombre}» se borrará. Las sedes que lo usaban pasarán a usar el predeterminado de ${tipoLabel(f.tipo)}.`, confirmLabel: 'Eliminar', tone: 'danger' }))) return
    setBusy(f.id)
    try { await api.eliminarFormato(f.id); await onCambio(); toast({ title: 'Formato eliminado', body: f.nombre }) }
    catch (e) { toast({ title: 'No se pudo eliminar', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy('') }
  }

  return (
    <div className="space-y-4">
      {porTipo.map(({ tipo, items }) => (
        <div key={tipo.id}>
          <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-2">{tipo.label}</div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {items.map((f) => (
              <div key={f.id} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-3.5 flex flex-col gap-2.5">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="font-display font-semibold text-[14px] text-slate-900 dark:text-slate-100 truncate">{f.nombre}</div>
                    <div className="text-[11.5px] text-slate-500 mt-0.5">{papelLabel(f.papel)}</div>
                  </div>
                  {f.predeterminada ? <Badge color="huberp" size="sm">Predeterminado</Badge> : null}
                </div>
                <MiniLienzo formato={f} />
                <div className="text-[11px] text-slate-400">
                  {(f.sedes || []).length ? `${f.sedes.length} sede(s) asignada(s)` : 'Sin sedes específicas'}
                </div>
                <div className="flex items-center gap-1.5 flex-wrap">
                  <Button size="sm" variant="secondary" icon={<Icon.Pencil size={14} />} onClick={() => onEditar(f)}>
                    {puedeEditar ? 'Editar' : 'Ver'}
                  </Button>
                  {puedeEditar ? (<>
                    {!f.predeterminada ? (
                      <Button size="sm" variant="ghost" loading={busy === f.id} icon={<Icon.Star size={14} />} onClick={() => marcarDefault(f)}>Predet.</Button>
                    ) : null}
                    <Button size="sm" variant="ghost" icon={<Icon.Copy size={14} />} onClick={() => duplicar(f)}>Duplicar</Button>
                    <Button size="sm" variant="ghost" icon={<Icon.Trash size={14} />} onClick={() => eliminar(f)}>Eliminar</Button>
                  </>) : null}
                </div>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

// Miniatura no interactiva de un formato (para la tarjeta de la lista).
function MiniLienzo({ formato }) {
  const anchoPx = 150
  const esc = anchoPx / (formato.anchoMm || 216)
  return (
    <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 overflow-hidden mx-auto"
      style={{ width: anchoPx, height: (formato.altoMm || 279) * esc, maxHeight: 150, position: 'relative' }}>
      {(formato.bloques || []).map((b) => (
        <div key={b.id} style={{ position: 'absolute', left: b.x * esc, top: b.y * esc, width: b.w * esc, height: Math.max(1, b.h * esc), background: b.tipo === 'separador' ? '#94a3b8' : 'rgba(106,44,240,0.12)', borderRadius: 1 }} />
      ))}
    </div>
  )
}

// ---- Asignación de formatos por sede ----
function AsignacionPorSede({ rows, sedes, puedeEditar, onCambio, toast }) {
  const [busy, setBusy] = useState('')
  const tiposConFormato = TIPOS.filter((t) => rows.some((r) => r.tipo === t.id))
  if (!sedes.length || !tiposConFormato.length) return null

  // Formato asignado a una sede para un tipo (el que la lista en `sedes`), o '' si
  // ninguno (cae al predeterminado).
  const asignado = (tipo, sedeId) => {
    const f = rows.find((r) => r.tipo === tipo && (r.sedes || []).includes(sedeId))
    return f ? f.id : ''
  }
  const cambiar = async (tipo, sedeId, plantillaId) => {
    const key = tipo + '|' + sedeId
    setBusy(key)
    try { await api.asignarFormato({ tipo, sedeId, plantillaId }); await onCambio() }
    catch (e) { toast({ title: 'No se pudo asignar', body: e?.message || 'Error', kind: 'error' }) }
    finally { setBusy('') }
  }
  const predetDe = (tipo) => rows.find((r) => r.tipo === tipo && r.predeterminada)

  return (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
      <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-800">
        <div className="font-display font-semibold text-[14px] text-slate-900 dark:text-slate-100">Qué formato usa cada sede</div>
        <div className="text-[12px] text-slate-500 mt-0.5">Cada sede puede imprimir con un formato distinto. «Predeterminado» usa el formato marcado por defecto de ese tipo.</div>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
              <th className="py-2.5 px-4 font-medium">Sede</th>
              {tiposConFormato.map((t) => <th key={t.id} className="py-2.5 px-3 font-medium">{t.label}</th>)}
            </tr>
          </thead>
          <tbody>
            {sedes.map((s) => (
              <tr key={s.id} className="border-b border-slate-100 dark:border-slate-800/60 last:border-0">
                <td className="py-2 px-4 font-medium text-slate-700 dark:text-slate-200 whitespace-nowrap">{s.nombre}</td>
                {tiposConFormato.map((t) => {
                  const opciones = rows.filter((r) => r.tipo === t.id)
                  const pd = predetDe(t.id)
                  return (
                    <td key={t.id} className="py-2 px-3">
                      <Select value={asignado(t.id, s.id)} disabled={!puedeEditar || busy === (t.id + '|' + s.id)}
                        onChange={(e) => cambiar(t.id, s.id, e.target.value)} className="min-w-[150px]">
                        <option value="">Predeterminado{pd ? ` · ${pd.nombre}` : ''}</option>
                        {opciones.map((o) => <option key={o.id} value={o.id}>{o.nombre}</option>)}
                      </Select>
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ---- Modal de creación ----
function NuevoFormatoModal({ onClose, onCreado, toast }) {
  const [nombre, setNombre] = useState('')
  const [tipo, setTipo] = useState('factura')
  const [papel, setPapel] = useState('carta')
  const [busy, setBusy] = useState(false)

  const crear = async () => {
    setBusy(true)
    try {
      const f = await api.crearFormato({ nombre: nombre.trim() || `Formato ${tipoLabel(tipo).toLowerCase()}`, tipo, papel })
      toast({ title: 'Formato creado', body: f.nombre })
      onCreado(f)
    } catch (e) { toast({ title: 'No se pudo crear', body: e?.message || 'Error', kind: 'error' }); setBusy(false) }
  }
  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.FileText size={18} />} title="Nuevo formato de documento"
      sub="Arranca con una disposición base que luego puedes arrastrar y ajustar."
      footer={<><Button variant="ghost" onClick={onClose}>Cancelar</Button><Button loading={busy} onClick={crear}>Crear y editar</Button></>}>
      <div className="space-y-3.5">
        <Field label="Nombre"><Input autoFocus value={nombre} onChange={(e) => setNombre(e.target.value)} placeholder="Factura carta" /></Field>
        <Field label="Tipo de documento">
          <Select value={tipo} onChange={(e) => setTipo(e.target.value)}>
            {TIPOS.map((t) => <option key={t.id} value={t.id}>{t.label}</option>)}
          </Select>
        </Field>
        <Field label="Tamaño de papel">
          <Select value={papel} onChange={(e) => setPapel(e.target.value)}>
            {PAPELES.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
          </Select>
        </Field>
      </div>
    </Modal>
  )
}

// ======================= EDITOR =======================
function EditorFormato({ formato, campos, puedeEditar, empresa, onCerrar, onGuardado }) {
  const toast = useToast()
  const [nombre, setNombre] = useState(formato.nombre)
  const [papel, setPapel] = useState(formato.papel)
  const [anchoMm, setAnchoMm] = useState(formato.anchoMm || 215.9)
  const [altoMm, setAltoMm] = useState(formato.altoMm || 279.4)
  const [bloques, setBloques] = useState(() => (formato.bloques || []).map((b) => ({ ...b })))
  const [sel, setSel] = useState(null)
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)
  const [zoom, setZoom] = useState(1)

  const preset = PAPELES.find((p) => p.id === papel)
  const esCustom = papel === 'custom'
  // Dimensiones efectivas: presets mandan; custom usa los inputs.
  const W = esCustom ? Number(anchoMm) || 100 : preset.w
  const H = esCustom ? Number(altoMm) || 150 : preset.h
  const esc = clamp(480 / W, 1.2, 5) * zoom // px por mm

  const marcar = (mut) => { setBloques(mut); setDirty(true) }
  const setPapelSel = (p) => {
    setPapel(p)
    const pr = PAPELES.find((x) => x.id === p)
    if (pr && pr.id !== 'custom') { setAnchoMm(pr.w); setAltoMm(pr.h) }
    setDirty(true)
  }
  const actualizarBloque = (id, patch) => marcar(bloques.map((b) => b.id === id ? { ...b, ...patch } : b))
  const agregar = (tipo) => {
    const def = NUEVO_BLOQUE[tipo] || { w: 40, h: 10 }
    const nb = { id: uid(), tipo, x: 12, y: 12, alineacion: 'izquierda', tamano: 9, ...def }
    marcar([...bloques, nb]); setSel(nb.id)
  }
  const borrarBloque = (id) => { marcar(bloques.filter((b) => b.id !== id)); if (sel === id) setSel(null) }

  const guardar = async () => {
    setBusy(true)
    try {
      const body = { nombre: nombre.trim() || formato.nombre, tipo: formato.tipo, papel, anchoMm: W, altoMm: H, bloques }
      const out = await api.actualizarFormato(formato.id, body)
      toast({ title: 'Formato guardado', body: out.nombre })
      setDirty(false); onGuardado(out)
    } catch (e) { toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' }); setBusy(false) }
  }

  const seleccionado = bloques.find((b) => b.id === sel) || null

  return (
    <div className="fixed inset-0 z-40 bg-slate-100 dark:bg-slate-950 flex flex-col">
      {/* Barra superior */}
      <div className="shrink-0 h-14 border-b border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 flex items-center gap-3 px-4">
        <Button variant="ghost" size="sm" icon={<Icon.ChevLeft size={16} />} onClick={() => {
          if (dirty && !confirm('Hay cambios sin guardar. ¿Salir de todos modos?')) return
          onCerrar()
        }}>Volver</Button>
        <div className="flex items-center gap-2 min-w-0">
          <Icon.FileText size={16} className="text-elerp-500 shrink-0" />
          {puedeEditar ? (
            <input value={nombre} onChange={(e) => { setNombre(e.target.value); setDirty(true) }}
              className="bg-transparent font-display font-semibold text-[15px] text-slate-900 dark:text-slate-100 outline-none border-b border-transparent focus:border-elerp-400 min-w-0 max-w-[240px]" />
          ) : <span className="font-display font-semibold text-[15px] text-slate-900 dark:text-slate-100 truncate">{nombre}</span>}
          <Badge color="slate" size="sm">{tipoLabel(formato.tipo)}</Badge>
        </div>
        <div className="ml-auto flex items-center gap-2">
          {/* Zoom del lienzo: alejar/acercar + barra deslizante. No cambia el
              formato, solo el aumento del área de edición. Clic en el % lo
              restablece a 100%. */}
          <div className="flex items-center gap-2 text-slate-500">
            <button className="p-1.5 rounded-md hover:bg-slate-100 dark:hover:bg-slate-800" title="Alejar" onClick={() => setZoom((z) => clamp(Math.round((z - 0.1) * 10) / 10, ZOOM_MIN, ZOOM_MAX))}><Icon.Minus size={16} /></button>
            <input type="range" min={ZOOM_MIN * 100} max={ZOOM_MAX * 100} step={10}
              value={Math.round(zoom * 100)} aria-label="Zoom del lienzo"
              onChange={(e) => setZoom(clamp(Number(e.target.value) / 100, ZOOM_MIN, ZOOM_MAX))}
              className="w-28 md:w-40 accent-elerp-500 cursor-pointer" />
            <button className="p-1.5 rounded-md hover:bg-slate-100 dark:hover:bg-slate-800" title="Acercar" onClick={() => setZoom((z) => clamp(Math.round((z + 0.1) * 10) / 10, ZOOM_MIN, ZOOM_MAX))}><Icon.Plus size={16} /></button>
            <button onClick={() => setZoom(1)} title="Restablecer a 100%"
              className="text-[11.5px] font-mono w-11 text-center rounded-md py-0.5 hover:bg-slate-100 dark:hover:bg-slate-800">{Math.round(zoom * 100)}%</button>
          </div>
          {puedeEditar ? <Button size="sm" loading={busy} disabled={!dirty} icon={<Icon.Check size={15} />} onClick={guardar}>Guardar</Button> : null}
        </div>
      </div>

      <div className="flex-1 min-h-0 flex">
        {/* Paleta de bloques */}
        {puedeEditar ? (
          <div className="shrink-0 w-40 border-r border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 p-3 overflow-y-auto">
            <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-2">Agregar bloque</div>
            <div className="space-y-1.5">
              {TIPOS_BLOQUE.map((t) => (
                <button key={t.id} onClick={() => agregar(t.id)}
                  className="w-full flex items-center gap-2 px-2.5 py-2 rounded-lg text-[12.5px] font-medium text-slate-600 dark:text-slate-300 border border-slate-200 dark:border-slate-700 hover:border-elerp-400 hover:text-elerp-600 dark:hover:text-elerp-300 transition-colors text-left">
                  {t.icon}{t.label}
                </button>
              ))}
            </div>
            <div className="text-[11px] text-slate-400 mt-3 leading-relaxed">Haz clic para agregarlo; luego arrástralo sobre el papel.</div>
          </div>
        ) : null}

        {/* Lienzo */}
        <div className="flex-1 min-w-0 overflow-auto p-6 flex justify-center items-start">
          <Lienzo W={W} H={H} esc={esc} bloques={bloques} sel={sel} setSel={setSel}
            campos={campos} empresa={empresa} puedeEditar={puedeEditar}
            onMover={(id, x, y) => actualizarBloque(id, { x, y })}
            onRedimensionar={(id, w, h) => actualizarBloque(id, { w, h })} tipoDoc={formato.tipo} />
        </div>

        {/* Panel de propiedades */}
        <div className="shrink-0 w-64 border-l border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 p-3.5 overflow-y-auto">
          <PanelPropiedades papel={papel} onPapel={setPapelSel} esCustom={esCustom}
            anchoMm={anchoMm} altoMm={altoMm} setAnchoMm={(v) => { setAnchoMm(v); setDirty(true) }} setAltoMm={(v) => { setAltoMm(v); setDirty(true) }}
            bloque={seleccionado} campos={campos} puedeEditar={puedeEditar}
            onBloque={(patch) => actualizarBloque(sel, patch)} onBorrar={() => borrarBloque(sel)} W={W} H={H} />
        </div>
      </div>
    </div>
  )
}

// El lienzo: el papel a escala con los bloques absolutos, arrastrables.
function Lienzo({ W, H, esc, bloques, sel, setSel, campos, empresa, puedeEditar, onMover, onRedimensionar, tipoDoc }) {
  const ref = useRef(null)
  const drag = useRef(null)

  const onDown = (e, b, modo) => {
    if (!puedeEditar) return
    e.preventDefault(); e.stopPropagation()
    setSel(b.id)
    drag.current = { id: b.id, modo, sx: e.clientX, sy: e.clientY, ox: b.x, oy: b.y, ow: b.w, oh: b.h }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }
  const onMove = (e) => {
    const d = drag.current; if (!d) return
    const dx = (e.clientX - d.sx) / esc, dy = (e.clientY - d.sy) / esc
    if (d.modo === 'move') {
      const b = bloques.find((x) => x.id === d.id)
      onMover(d.id, Math.round(clamp(d.ox + dx, 0, W - (b?.w || 0)) * 10) / 10, Math.round(clamp(d.oy + dy, 0, H - (b?.h || 0)) * 10) / 10)
    } else {
      onRedimensionar(d.id, Math.round(clamp(d.ow + dx, 4, W - d.ox) * 10) / 10, Math.round(clamp(d.oh + dy, 1, H - d.oy) * 10) / 10)
    }
  }
  const onUp = () => { drag.current = null; window.removeEventListener('pointermove', onMove); window.removeEventListener('pointerup', onUp) }

  return (
    <div ref={ref} onPointerDown={() => setSel(null)}
      className="relative bg-white shadow-lg select-none shrink-0"
      style={{ width: W * esc, height: H * esc, backgroundImage: 'linear-gradient(#f1f5f9 1px,transparent 1px),linear-gradient(90deg,#f1f5f9 1px,transparent 1px)', backgroundSize: `${10 * esc}px ${10 * esc}px` }}>
      {bloques.map((b) => (
        <BloqueVista key={b.id} b={b} esc={esc} activo={sel === b.id} campos={campos} empresa={empresa}
          puedeEditar={puedeEditar} onDown={onDown} tipoDoc={tipoDoc} />
      ))}
    </div>
  )
}

function BloqueVista({ b, esc, activo, campos, empresa, puedeEditar, onDown, tipoDoc }) {
  const fontPx = Math.max(6, (b.tamano || 9) * MM_POR_PT * esc)
  const style = {
    position: 'absolute', left: b.x * esc, top: b.y * esc, width: b.w * esc, height: Math.max(2, b.h * esc),
    fontSize: fontPx, fontWeight: b.negrita ? 700 : 400, textAlign: b.alineacion || 'left',
    color: b.color || '#141118', lineHeight: 1.15, overflow: 'hidden',
    outline: activo ? '2px solid #6A2CF0' : '1px dashed rgba(100,116,139,0.35)',
    cursor: puedeEditar ? 'move' : 'default', boxSizing: 'border-box', padding: '1px 2px',
  }
  return (
    <div style={style} onPointerDown={(e) => onDown(e, b, 'move')}>
      <ContenidoBloque b={b} esc={esc} campos={campos} empresa={empresa} tipoDoc={tipoDoc} />
      {activo && puedeEditar ? (
        <div onPointerDown={(e) => onDown(e, b, 'resize')}
          style={{ position: 'absolute', right: -5, bottom: -5, width: 10, height: 10, background: '#6A2CF0', borderRadius: 2, cursor: 'nwse-resize' }} />
      ) : null}
    </div>
  )
}

function ContenidoBloque({ b, esc, campos, empresa, tipoDoc }) {
  if (b.tipo === 'texto') return <span>{b.texto || 'Texto'}</span>
  if (b.tipo === 'campo') {
    const val = MUESTRA[b.campo] ?? (campos.find((c) => c.clave === b.campo)?.etiqueta || b.campo || 'Campo')
    // El tipo del documento se muestra según el formato en edición.
    const muestra = b.campo === 'doc.tipo' ? tipoEtiquetaMayus(tipoDoc) : val
    return <span>{muestra}</span>
  }
  if (b.tipo === 'separador') return <div style={{ borderTop: `${Math.max(1, b.h * esc)}px solid ${b.color || '#334155'}`, width: '100%' }} />
  if (b.tipo === 'logo') return (
    <div style={{ width: '100%', height: '100%', border: '1px dashed #94a3b8', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#64748b', fontSize: Math.max(7, 9 * MM_POR_PT * esc) }}>
      {empresa?.nombre ? 'LOGO' : 'LOGO'}
    </div>
  )
  if (b.tipo === 'qr') return (
    <div style={{ width: '100%', height: '100%', border: '1px solid #334155', display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gridTemplateRows: 'repeat(4,1fr)' }}>
      {Array.from({ length: 16 }).map((_, i) => <div key={i} style={{ background: (i * 7 + 3) % 3 === 0 ? '#141118' : 'transparent' }} />)}
    </div>
  )
  if (b.tipo === 'imagen') return (
    b.imagen
      ? <img src={b.imagen} alt="" style={{ width: '100%', height: '100%', objectFit: 'contain', display: 'block' }} />
      : <div style={{ width: '100%', height: '100%', border: '1px dashed #94a3b8', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#94a3b8', fontSize: Math.max(7, 8 * MM_POR_PT * esc), textAlign: 'center' }}>Imagen</div>
  )
  if (b.tipo === 'tabla_items') {
    const f = Math.max(5, 7 * MM_POR_PT * esc)
    const o = b.opciones || {}
    const enc = (t) => (o.mostrarMoneda ? `${t} (Bs)` : t)
    const cel = (conMoneda) => (o.mostrarMoneda ? '' : 'Bs ')
    // Muestra con un renglón EXENTO para ver el efecto de "marcar exentos".
    const filas = [
      { d: 'Harina de maíz 1kg', c: '2', p: '48,00', t: '96,00', ex: false },
      { d: 'Medicina genérica', c: '1', p: '120,00', t: '120,00', ex: true },
      { d: 'Café molido 250g', c: '1', p: '84,00', t: '84,00', ex: false },
    ]
    return (
      <>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: f }}>
          <thead><tr style={{ borderBottom: '1px solid #334155', textAlign: 'left' }}>
            <th style={{ padding: '1px 2px' }}>Descripción</th><th style={{ padding: '1px 2px', textAlign: 'right' }}>Cant</th>
            <th style={{ padding: '1px 2px', textAlign: 'right' }}>{enc('Precio')}</th><th style={{ padding: '1px 2px', textAlign: 'right' }}>{enc('Total')}</th>
          </tr></thead>
          <tbody>{filas.map((r, i) => (
            <tr key={i} style={{ borderBottom: '1px solid #e2e8f0' }}>
              <td style={{ padding: '1px 2px' }}>{r.d}{o.marcarExento && r.ex ? ' *' : ''}</td>
              <td style={{ padding: '1px 2px', textAlign: 'right' }}>{r.c}</td>
              <td style={{ padding: '1px 2px', textAlign: 'right' }}>{cel() + r.p}</td>
              <td style={{ padding: '1px 2px', textAlign: 'right' }}>{cel() + r.t}</td>
            </tr>))}</tbody>
        </table>
        {o.marcarExento ? <div style={{ fontSize: Math.max(5, 6 * MM_POR_PT * esc), color: '#5C6470', marginTop: 2 }}>* Exento de IVA</div> : null}
      </>
    )
  }
  if (b.tipo === 'totales') {
    const f = Math.max(5, 8 * MM_POR_PT * esc)
    const o = b.opciones || {}
    const fila = (k, v, bold) => (
      <div style={{ display: 'flex', justifyContent: 'space-between', fontWeight: bold ? 700 : 400, borderTop: bold ? '1px solid #334155' : 'none', paddingTop: bold ? 2 : 0 }}>
        <span>{k}</span><span>{v}</span>
      </div>
    )
    const info = (k, v) => (
      <div style={{ display: 'flex', justifyContent: 'space-between', color: '#5C6470', fontSize: Math.max(5, 6.5 * MM_POR_PT * esc), marginTop: 1 }}>
        <span>{k}</span><span>{v}</span>
      </div>
    )
    return (
      <div style={{ width: '100%', fontSize: f }}>
        {fila('Subtotal', 'Bs 180,00')}{fila('IVA 16%', 'Bs 28,80')}{fila('IGTF 3%', 'Bs 6,26')}{fila('Total', 'Bs 215,06', true)}
        {o.mostrarTasa ? info('Tasa BCV', 'Bs 132,50/US$') : null}
        {o.mostrarDivisa ? info('Total US$ (ref.)', 'US$ 1,62') : null}
      </div>
    )
  }
  return null
}

const tipoEtiquetaMayus = (t) => (tipoLabel(t) || '').toUpperCase()

// Panel de propiedades del papel + del bloque seleccionado.
function PanelPropiedades({ papel, onPapel, esCustom, anchoMm, altoMm, setAnchoMm, setAltoMm, bloque, campos, puedeEditar, onBloque, onBorrar, W, H }) {
  const [imgErr, setImgErr] = useState('')
  // Sube una imagen al bloque validando formato y peso EN EL CLIENTE (el backend
  // revalida). Se guarda como data URI dentro del formato.
  const subirImagen = (file) => {
    if (!file) return
    if (!IMAGEN_TIPOS_OK.includes(file.type)) { setImgErr('Formato no permitido. Usa PNG, JPEG o WebP.'); return }
    if (file.size > IMAGEN_MAX_BYTES) { setImgErr('La imagen supera el máximo de 512 KB.'); return }
    const r = new FileReader()
    r.onload = () => { setImgErr(''); onBloque({ imagen: String(r.result) }) }
    r.onerror = () => setImgErr('No se pudo leer la imagen.')
    r.readAsDataURL(file)
  }
  const numField = (label, val, on, extra = {}) => (
    <label className="block">
      <span className="text-[11px] text-slate-500">{label}</span>
      <input type="number" value={val ?? 0} disabled={!puedeEditar} onChange={(e) => on(Number(e.target.value))}
        className="w-full mt-0.5 px-2 py-1 text-[12.5px] rounded-md border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 disabled:opacity-60" {...extra} />
    </label>
  )
  const gruposCampo = useMemo(() => {
    const g = {}
    for (const c of campos) (g[c.grupo] ||= []).push(c)
    return g
  }, [campos])

  return (
    <div className="space-y-4">
      <div>
        <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-2">Papel</div>
        <Select value={papel} disabled={!puedeEditar} onChange={(e) => onPapel(e.target.value)}>
          {PAPELES.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
        </Select>
        {esCustom ? (
          <div className="grid grid-cols-2 gap-2 mt-2">
            {numField('Ancho (mm)', anchoMm, setAnchoMm, { min: 40, max: 600 })}
            {numField('Alto (mm)', altoMm, setAltoMm, { min: 40, max: 900 })}
          </div>
        ) : null}
      </div>

      <div className="border-t border-slate-200 dark:border-slate-800 pt-3">
        <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-2">Bloque seleccionado</div>
        {!bloque ? (
          <div className="text-[12px] text-slate-400">Selecciona un bloque en el lienzo para editar su contenido, posición y estilo.</div>
        ) : (
          <div className="space-y-2.5">
            <div className="text-[12px] font-medium text-slate-600 dark:text-slate-300">{TIPOS_BLOQUE.find((t) => t.id === bloque.tipo)?.label || bloque.tipo}</div>

            {bloque.tipo === 'texto' ? (
              <label className="block">
                <span className="text-[11px] text-slate-500">Contenido</span>
                <textarea value={bloque.texto || ''} disabled={!puedeEditar} onChange={(e) => onBloque({ texto: e.target.value })} rows={2}
                  className="w-full mt-0.5 px-2 py-1 text-[12.5px] rounded-md border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 disabled:opacity-60" />
              </label>
            ) : null}

            {bloque.tipo === 'campo' ? (
              <label className="block">
                <span className="text-[11px] text-slate-500">Dato a mostrar</span>
                <Select value={bloque.campo || ''} disabled={!puedeEditar} onChange={(e) => onBloque({ campo: e.target.value })} className="mt-0.5">
                  {Object.entries(gruposCampo).map(([g, items]) => (
                    <optgroup key={g} label={g}>{items.map((c) => <option key={c.clave} value={c.clave}>{c.etiqueta}</option>)}</optgroup>
                  ))}
                </Select>
              </label>
            ) : null}

            {(bloque.tipo === 'texto' || bloque.tipo === 'campo') ? (
              <>
                <div className="flex items-center gap-2">
                  {numField('Tamaño (pt)', bloque.tamano, (v) => onBloque({ tamano: v }), { min: 5, max: 48, step: 0.5 })}
                  <label className="block">
                    <span className="text-[11px] text-slate-500">Color</span>
                    <input type="color" value={bloque.color || '#141118'} disabled={!puedeEditar} onChange={(e) => onBloque({ color: e.target.value })}
                      className="w-full h-[30px] mt-0.5 rounded-md border border-slate-300 dark:border-slate-700 bg-white disabled:opacity-60" />
                  </label>
                </div>
                <div className="flex items-center justify-between">
                  <Segmented size="sm" value={bloque.alineacion || 'izquierda'} onChange={(v) => puedeEditar && onBloque({ alineacion: v })}
                    options={[{ value: 'izquierda', label: '≡' }, { value: 'centro', label: '≣' }, { value: 'derecha', label: '≡' }]} />
                  <label className="flex items-center gap-1.5 text-[12px] text-slate-600 dark:text-slate-300">
                    <input type="checkbox" checked={!!bloque.negrita} disabled={!puedeEditar} onChange={(e) => onBloque({ negrita: e.target.checked })} /> Negrita
                  </label>
                </div>
              </>
            ) : null}

            {bloque.tipo === 'imagen' ? (
              <div className="space-y-1.5">
                {bloque.imagen ? (
                  <img src={bloque.imagen} alt="" className="w-full max-h-24 object-contain rounded border border-slate-200 dark:border-slate-700 bg-white p-1" />
                ) : null}
                {puedeEditar ? (
                  <label className="block">
                    <span className="text-[11px] text-slate-500">Imagen · PNG, JPEG o WebP · máx. 512 KB</span>
                    <input type="file" accept="image/png,image/jpeg,image/webp"
                      onChange={(e) => subirImagen(e.target.files && e.target.files[0])}
                      className="mt-0.5 block w-full text-[11px] file:mr-2 file:py-1 file:px-2 file:rounded-md file:border-0 file:text-[11px] file:bg-elerp-50 file:text-elerp-600 dark:file:bg-elerp-900/40 dark:file:text-elerp-200 file:cursor-pointer" />
                  </label>
                ) : null}
                {imgErr ? <div className="text-[11.5px] text-red-500">{imgErr}</div> : null}
                {bloque.imagen && puedeEditar ? (
                  <button onClick={() => onBloque({ imagen: '' })} className="text-[11.5px] text-slate-500 hover:text-red-500">Quitar imagen</button>
                ) : null}
              </div>
            ) : null}

            {(bloque.tipo === 'totales' || bloque.tipo === 'tabla_items') ? (
              <div className="space-y-1.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 p-2">
                <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Datos a mostrar</div>
                {(bloque.tipo === 'totales' ? OPCIONES_TOTALES : OPCIONES_ITEMS).map((op) => (
                  <label key={op.key} className="flex items-start gap-1.5 text-[12px] text-slate-600 dark:text-slate-300">
                    <input type="checkbox" className="mt-0.5" checked={!!(bloque.opciones || {})[op.key]} disabled={!puedeEditar}
                      onChange={(e) => onBloque({ opciones: { ...(bloque.opciones || {}), [op.key]: e.target.checked } })} />
                    <span>{op.label}</span>
                  </label>
                ))}
                {bloque.tipo === 'totales' ? (
                  <div className="text-[10.5px] text-slate-400 leading-snug">El IGTF (3%) se muestra solo cuando el pago es en divisas.</div>
                ) : null}
              </div>
            ) : null}

            <div className="grid grid-cols-2 gap-2">
              {numField('X (mm)', bloque.x, (v) => onBloque({ x: clamp(v, 0, W) }), { min: 0, max: W, step: 0.5 })}
              {numField('Y (mm)', bloque.y, (v) => onBloque({ y: clamp(v, 0, H) }), { min: 0, max: H, step: 0.5 })}
              {numField('Ancho (mm)', bloque.w, (v) => onBloque({ w: clamp(v, 2, W) }), { min: 2, max: W, step: 0.5 })}
              {numField('Alto (mm)', bloque.h, (v) => onBloque({ h: clamp(v, 1, H) }), { min: 1, max: H, step: 0.5 })}
            </div>

            {puedeEditar ? (
              <Button variant="ghost" size="sm" icon={<Icon.Trash size={14} />} onClick={onBorrar} className="w-full">Quitar bloque</Button>
            ) : null}
          </div>
        )}
      </div>
    </div>
  )
}
