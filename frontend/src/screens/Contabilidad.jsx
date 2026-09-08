import { useState, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Empty, PageHeader, useToast, TableSkeleton, Modal, Field, Input, Select } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { useRecurso, EstadoRecurso } from '../lib/useRecurso.jsx'

/* Contabilidad — plan de cuentas, libro diario y estados financieros.
 *
 * Lo que hay que entender de esta pantalla: **no se captura nada acá**. Los
 * asientos se DERIVAN de las operaciones (una factura emitida genera el suyo, un
 * cobro el suyo), así que la contabilidad no se cuadra a fin de mes contra el
 * fiscal o el inventario: sale del mismo hecho. Y el libro es de solo-anexado —
 * un asiento se corrige con su contrario, nunca se edita.
 *
 * Por eso la única acción de la pantalla es «revertir», y el balance de
 * comprobación muestra si el libro cuadra: es la prueba, no un adorno.
 */
const TABS = [
  { id: 'plan', label: 'Plan de cuentas', icon: <Icon.Layers size={15} /> },
  { id: 'diario', label: 'Libro diario', icon: <Icon.Book size={15} /> },
  { id: 'estados', label: 'Estados financieros', icon: <Icon.Chart size={15} /> },
]

const TIPO_COLOR = {
  activo: 'blue', pasivo: 'amber', patrimonio: 'slate',
  ingreso: 'emerald', costo: 'red', gasto: 'red',
}

export function Contabilidad({ route }) {
  const [tab, setTab] = useState((route || '').split(':')[1] || 'diario')
  useEffect(() => { setTab((route || '').split(':')[1] || 'diario') }, [route])
  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader
        breadcrumb={['Contabilidad', TABS.find((t) => t.id === tab)?.label]}
        title="Contabilidad"
        sub="Plan de cuentas, libro diario derivado de las operaciones y estados financieros que cuadran por construcción."
        tabs={TABS} activeTab={tab} onTab={setTab} />
      {tab === 'diario' ? <LibroDiario /> : null}
      {tab === 'plan' ? <PlanDeCuentas /> : null}
      {tab === 'estados' ? <Estados /> : null}
    </div>
  )
}

/* --- Libro diario --------------------------------------------------------- */

function LibroDiario() {
  const { ui } = useUI()
  const toast = useToast()
  const [revirtiendo, setRevirtiendo] = useState(null)
  const [nuevoAbierto, setNuevoAbierto] = useState(false)
  const [integridad, setIntegridad] = useState(null) // null | 'cargando' | [SelloResultado]
  const { data: asientos, loading, error, reload: cargar } = useRecurso(() => api.libroDiario().then((r) => r || []), [])

  const verificarIntegridad = async () => {
    setIntegridad('cargando')
    try {
      const r = await api.verificarIntegridad()
      setIntegridad(r?.libros || [])
    } catch (e) {
      toast({ title: 'No se pudo verificar la integridad', body: e?.message || 'Error', kind: 'error' })
      setIntegridad(null)
    }
  }

  if (loading || error) {
    return <EstadoRecurso loading={loading} error={error} onRetry={cargar} cols={4} rows={5}
      title="No se pudo cargar el libro diario" />
  }

  // Los contrarios ya emitidos: su original no se puede revertir otra vez.
  const revertidos = new Set(asientos.filter((a) => a.contrario).map((a) => a.refAsientoId))
  const puedeAsentar = ui.rol === 'dueno' || ui.rol === 'contadora'

  return (
    <div>
      <div className="flex items-center justify-between gap-2 mb-3 flex-wrap">
        <Button size="sm" variant="secondary" icon={<Icon.Shield size={15} />}
          onClick={verificarIntegridad} loading={integridad === 'cargando'}>Verificar integridad</Button>
        {puedeAsentar ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setNuevoAbierto(true)}>Nuevo asiento</Button>
        ) : null}
      </div>
      {Array.isArray(integridad) ? <IntegridadBanner libros={integridad} /> : null}
      {asientos.length === 0 ? (
        <Empty icon={<Icon.Book size={22} />} title="El libro está vacío"
          body="Los asientos se derivan de las operaciones (factura, anulación, cobro). También puedes registrar un asiento manual (ajustes, provisiones, reclasificaciones) que tiene que cuadrar."
          cta={puedeAsentar ? <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setNuevoAbierto(true)}>Nuevo asiento</Button> : null} />
      ) : (
        <div className="space-y-3">
          {asientos.map((a) => (
            <div key={a.id} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
              <div className="flex items-start gap-3 px-4 py-3 border-b border-slate-100 dark:border-slate-800">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="mono text-[12.5px] font-semibold">{a.codigo}</span>
                    {a.refTipo === 'manual' ? <Badge size="sm" color="blue">manual</Badge> : null}
                    {a.contrario ? <Badge size="sm" color="amber">contrario</Badge> : null}
                    {revertidos.has(a.id) ? <Badge size="sm" color="slate">revertido</Badge> : null}
                  </div>
                  <div className="text-[13.5px] font-medium mt-0.5 truncate">{a.descripcion}</div>
                  <div className="text-[11.5px] text-slate-400 num">{fmtDate(a.fecha)}</div>
                </div>
                <div className="text-right shrink-0">
                  <div className="num text-[14px] font-semibold private-mask">{fmtCurrency(a.total, 'VES')}</div>
                  {!a.contrario && !revertidos.has(a.id) && ui.rol !== 'vendedor' ? (
                    <Button size="sm" variant="destructive" onClick={() => setRevirtiendo(a)}>Revertir</Button>
                  ) : null}
                </div>
              </div>
              <table className="w-full text-sm">
                <tbody>
                  {(a.lineas || []).map((l, i) => (
                    <tr key={i} className="border-b border-slate-50 dark:border-slate-800/50 last:border-0">
                      <td className="py-1.5 px-4 text-[12.5px]">
                        <span className="mono text-slate-400 mr-2">{l.codigo}</span>{l.nombre}
                      </td>
                      <td className="py-1.5 pr-3 text-right num text-[12.5px] w-32 private-mask">
                        {l.debe ? fmtCurrency(l.debe, 'VES') : ''}
                      </td>
                      <td className="py-1.5 pr-4 text-right num text-[12.5px] w-32 text-slate-500 private-mask">
                        {l.haber ? fmtCurrency(l.haber, 'VES') : ''}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
        </div>
      )}

      <div className="mt-3 text-[11.5px] text-slate-400">
        Cada asiento cuadra por construcción (debe = haber). Los derivados de operaciones no se editan; un asiento
        <b> manual</b> también es append-only y se corrige con su contrario. El original siempre queda intacto.
      </div>

      {revirtiendo ? (
        <RevertirModal asiento={revirtiendo} onClose={() => setRevirtiendo(null)}
          onHecho={() => { cargar(); toast({ title: 'Asiento contrario emitido', body: `${revirtiendo.codigo} quedó revertido; el original sigue en el libro.` }) }} />
      ) : null}

      {nuevoAbierto ? (
        <NuevoAsientoModal onClose={() => setNuevoAbierto(false)}
          onHecho={(a) => { cargar(); toast({ title: 'Asiento registrado', body: a?.codigo || '' }) }} />
      ) : null}
    </div>
  )
}

/* Veredicto del sello de integridad (hash-encadenado) de los libros append-only:
 * verde si la cadena verifica de punta a punta, rojo si algún registro fue alterado
 * o reordenado. Es la prueba ante el SENIAT de que el libro no se manipuló. */
function IntegridadBanner({ libros }) {
  const todoIntegro = libros.every((l) => l.integro)
  const nombre = { diario: 'Libro diario', documentos: 'Documentos fiscales' }
  return (
    <div className={`mb-3 rounded-xl border px-3.5 py-3 text-[12.5px] flex items-start gap-2.5 ${todoIntegro
      ? 'border-emerald-200 bg-emerald-50 dark:border-emerald-900/40 dark:bg-emerald-900/20'
      : 'border-red-200 bg-red-50 dark:border-red-900/40 dark:bg-red-900/20'}`}>
      <Icon.Shield size={16} className={`mt-0.5 shrink-0 ${todoIntegro ? 'text-emerald-600' : 'text-red-600'}`} />
      <div className="flex-1 min-w-0">
        <div className={`font-semibold ${todoIntegro ? 'text-emerald-700 dark:text-emerald-300' : 'text-red-700 dark:text-red-300'}`}>
          {todoIntegro ? '✓ Integridad verificada — los libros no fueron alterados' : '✗ Integridad comprometida'}
        </div>
        <div className="mt-1 space-y-0.5 text-slate-600 dark:text-slate-300">
          {libros.map((l) => (
            <div key={l.libro}>
              <b>{nombre[l.libro] || l.libro}:</b>{' '}
              {l.integro
                ? `${l.sellados} registro(s) sellado(s) verificado(s)${l.sinSellar ? ` · ${l.sinSellar} anterior(es) al sello` : ''}`
                : `cadena rota en ${l.rotoEn}`}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

/* Alta de un asiento MANUAL: fecha, descripción y N líneas (cuenta + debe/haber).
 * El botón «Registrar» solo se habilita si cuadra (debe = haber) y hay ≥2 líneas
 * con monto — la misma regla que el servidor hace cumplir. */
function NuevoAsientoModal({ onClose, onHecho }) {
  const hoy = new Date().toISOString().slice(0, 10)
  const [fecha, setFecha] = useState(hoy)
  const [descripcion, setDescripcion] = useState('')
  const [lineas, setLineas] = useState([{ codigo: '', debe: '', haber: '' }, { codigo: '', debe: '', haber: '' }])
  const [cuentas, setCuentas] = useState([])
  const [busy, setBusy] = useState(false)
  const [apiError, setApiError] = useState('')

  useEffect(() => {
    // Solo cuentas HOJA activas: en un padre (con subcuentas) no se asienta.
    api.planDeCuentas().then((p) => {
      const plan = p || []
      const padres = new Set(plan.map((c) => c.codigoPadre).filter(Boolean))
      setCuentas(plan.filter((c) => !c.desactivada && !padres.has(c.codigo)))
    }).catch(() => setCuentas([]))
  }, [])

  const setLinea = (i, campo, valor) => setLineas((ls) => ls.map((l, j) => (j === i ? { ...l, [campo]: valor } : l)))
  const agregar = () => setLineas((ls) => [...ls, { codigo: '', debe: '', haber: '' }])
  const quitar = (i) => setLineas((ls) => (ls.length <= 2 ? ls : ls.filter((_, j) => j !== i)))

  const num = (v) => { const n = parseFloat(v); return isNaN(n) ? 0 : n }
  const totalDebe = lineas.reduce((s, l) => s + num(l.debe), 0)
  const totalHaber = lineas.reduce((s, l) => s + num(l.haber), 0)
  const conMonto = lineas.filter((l) => l.codigo && (num(l.debe) > 0 || num(l.haber) > 0))
  const cuadra = Math.abs(totalDebe - totalHaber) < 0.005 && totalDebe > 0
  const valido = descripcion.trim() && cuadra && conMonto.length >= 2

  const registrar = async () => {
    if (!valido) return
    setBusy(true); setApiError('')
    try {
      const a = await api.crearAsientoManual({
        fecha,
        descripcion: descripcion.trim(),
        lineas: conMonto.map((l) => ({ codigo: l.codigo, debe: num(l.debe), haber: num(l.haber) })),
      })
      onHecho(a)
      onClose()
    } catch (e) {
      setApiError(e?.message || 'No se pudo registrar el asiento.')
      setBusy(false)
    }
  }

  const inputMonto = 'w-28 text-right num h-9 px-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900'

  return (
    <Modal open onClose={onClose} size="lg" icon={<Icon.Book size={18} />}
      title="Nuevo asiento manual" sub="Para ajustes, provisiones o reclasificaciones. Tiene que cuadrar."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={registrar} loading={busy} disabled={!valido} icon={<Icon.Check size={16} />}>Registrar</Button>
      </>}>
      <div className="space-y-3">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Fecha" required><Input type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} /></Field>
          <Field label="Descripción" required><Input value={descripcion} onChange={(e) => setDescripcion(e.target.value)} placeholder="Ajuste de..." /></Field>
        </div>
        <div className="border border-slate-200 dark:border-slate-800 rounded-lg overflow-hidden">
          <div className="flex items-center gap-2 px-3 py-1.5 text-[11px] uppercase tracking-wide text-slate-400 bg-slate-50/60 dark:bg-slate-900/60 border-b border-slate-200 dark:border-slate-800">
            <span className="flex-1">Cuenta</span><span className="w-28 text-right">Debe</span><span className="w-28 text-right">Haber</span><span className="w-6" />
          </div>
          {lineas.map((l, i) => (
            <div key={i} className="flex items-center gap-2 px-3 py-1.5 border-b border-slate-100 dark:border-slate-800/70 last:border-0">
              <div className="flex-1 min-w-0">
                <Select value={l.codigo} onChange={(e) => setLinea(i, 'codigo', e.target.value)}>
                  <option value="">Elegir cuenta…</option>
                  {cuentas.map((c) => <option key={c.codigo} value={c.codigo}>{c.codigo} · {c.nombre}</option>)}
                </Select>
              </div>
              <input className={inputMonto} inputMode="decimal" placeholder="0,00" value={l.debe}
                onChange={(e) => setLinea(i, 'debe', e.target.value)} disabled={num(l.haber) > 0} />
              <input className={inputMonto} inputMode="decimal" placeholder="0,00" value={l.haber}
                onChange={(e) => setLinea(i, 'haber', e.target.value)} disabled={num(l.debe) > 0} />
              <button onClick={() => quitar(i)} disabled={lineas.length <= 2}
                className="w-6 h-6 inline-flex items-center justify-center text-slate-400 hover:text-red-500 disabled:opacity-30" title="Quitar línea">
                <Icon.X size={15} />
              </button>
            </div>
          ))}
          <div className="flex items-center gap-2 px-3 py-2 bg-slate-50/60 dark:bg-slate-900/60">
            <button onClick={agregar} className="text-[12.5px] font-medium text-elerp-500 hover:underline inline-flex items-center gap-1">
              <Icon.Plus size={14} /> Agregar línea
            </button>
            <span className="flex-1" />
            <span className={`w-28 text-right num text-[13px] font-semibold ${cuadra ? '' : 'text-amber-600'}`}>{fmtCurrency(totalDebe, 'VES')}</span>
            <span className={`w-28 text-right num text-[13px] font-semibold ${cuadra ? '' : 'text-amber-600'}`}>{fmtCurrency(totalHaber, 'VES')}</span>
            <span className="w-6" />
          </div>
        </div>
        <div className={`text-[12px] ${cuadra ? 'text-emerald-600' : 'text-amber-600'}`}>
          {cuadra ? '✓ El asiento cuadra.' : `Diferencia: ${fmtCurrency(Math.abs(totalDebe - totalHaber), 'VES')} — el debe debe igualar al haber.`}
        </div>
        {apiError ? <div className="text-[12.5px] text-red-500">{apiError}</div> : null}
      </div>
    </Modal>
  )
}

function RevertirModal({ asiento, onClose, onHecho }) {
  const [motivo, setMotivo] = useState('')
  const [busy, setBusy] = useState(false)
  const [touched, setTouched] = useState(false)
  const [apiError, setApiError] = useState('')

  // Validación inline (touched + onBlur): el motivo es obligatorio porque queda
  // en el libro. `apiError` es distinto: un fallo del servidor al emitir.
  const errMotivo = !motivo.trim() ? 'Explica por qué se revierte: queda en el libro.' : ''
  const valid = !errMotivo

  const revertir = async () => {
    setTouched(true)
    if (!valid) return
    setBusy(true); setApiError('')
    try {
      await api.revertirAsiento(asiento.id, motivo.trim())
      onHecho()
      onClose()
    } catch (e) {
      setApiError(e?.message || 'No se pudo revertir.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Arrows size={18} />}
      title="Revertir asiento" sub={`${asiento.codigo} · ${asiento.descripcion}`}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant="destructive" onClick={revertir} loading={busy} disabled={!valid}
          title={valid ? '' : 'Escribe el motivo de la reversión'} icon={<Icon.Check size={16} />}>Emitir contrario</Button>
      </>}>
      <div className="space-y-3">
        <div className="text-[12.5px] text-slate-600 dark:text-slate-300">
          Se anexa un asiento con las mismas cuentas invertidas. El original <strong>no se toca</strong>: así el
          libro puede explicar cómo se llegó a cada saldo.
        </div>
        <Field label="Motivo" required error={touched ? errMotivo : ''}>
          <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} onBlur={() => setTouched(true)}
            invalid={touched && !!errMotivo} autoFocus placeholder="Ej. se asentó en la cuenta equivocada" />
        </Field>
        {apiError ? (
          <div className="flex items-start gap-1.5 text-[12px] text-red-600 dark:text-red-400">
            <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{apiError}</span>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}

/* --- Plan de cuentas ----------------------------------------------------- */

const TIPOS_CUENTA = ['activo', 'pasivo', 'patrimonio', 'ingreso', 'costo', 'gasto']

function PlanDeCuentas() {
  // El plan es el recurso que decide el estado (si falla, hay error+Reintentar);
  // el balance es complementario (solo alimenta la columna «Saldo»), así que se
  // tolera que falle sin tumbar la vista.
  const { data, loading, error, reload } = useRecurso(
    () => api.planDeCuentas().then(async (plan) => ({ plan: plan || [], balance: await api.balanceContable().catch(() => null) })), [])
  const { ui } = useUI()
  const toast = useToast()
  const puedeEditar = ['dueno', 'contadora', 'desarrollador'].includes(ui.rol)
  // form = null | { mode:'crear'|'editar', codigo, nombre, tipo }
  const [form, setForm] = useState(null)
  const [guardando, setGuardando] = useState(false)

  if (loading || error) {
    return <EstadoRecurso loading={loading} error={error} onRetry={reload} cols={5} rows={6}
      title="No se pudo cargar el plan de cuentas" />
  }
  const { plan, balance } = data
  const saldoDe = (codigo) => (balance?.cuentas || []).find((c) => c.codigo === codigo)?.saldo ?? 0

  // Jerarquía: agrupo por código de padre y recorro raíces→hojas para indentar.
  const porPadre = {}
  for (const c of plan) { const p = c.codigoPadre || ''; (porPadre[p] = porPadre[p] || []).push(c) }
  for (const k in porPadre) porPadre[k].sort((a, b) => a.codigo.localeCompare(b.codigo))
  const esPadre = (codigo) => !!porPadre[codigo]
  // Saldo con rollup: una cuenta padre agrega el saldo de sus subcuentas.
  const saldoRollup = (codigo) => saldoDe(codigo) + (porPadre[codigo] || []).reduce((s, ch) => s + saldoRollup(ch.codigo), 0)
  const aplanar = (padreCod, depth) => (porPadre[padreCod] || []).flatMap((c) => [{ c, depth }, ...aplanar(c.codigo, depth + 1)])
  const ordenadas = aplanar('', 0)

  const abrirCrear = () => setForm({ mode: 'crear', codigo: '', nombre: '', tipo: 'activo', codigoPadre: '' })
  const abrirEditar = (c) => setForm({ mode: 'editar', codigo: c.codigo, nombre: c.nombre, tipo: c.tipo })

  const guardar = async () => {
    setGuardando(true)
    try {
      if (form.mode === 'crear') {
        await api.crearCuenta({ codigo: form.codigo.trim(), nombre: form.nombre.trim(), tipo: form.tipo, codigoPadre: form.codigoPadre || '' })
        toast({ title: 'Cuenta creada', body: `${form.codigo.trim()} · ${form.nombre.trim()}` })
      } else {
        await api.renombrarCuenta(form.codigo, form.nombre.trim())
        toast({ title: 'Cuenta actualizada', body: form.nombre.trim() })
      }
      setForm(null)
      reload()
    } catch (e) {
      toast({ title: 'No se pudo guardar la cuenta', body: e?.message || 'Error', kind: 'error' })
    } finally {
      setGuardando(false)
    }
  }

  const alternarActiva = async (c) => {
    try {
      if (c.desactivada) { await api.activarCuenta(c.codigo); toast({ title: 'Cuenta reactivada', body: c.nombre }) }
      else { await api.desactivarCuenta(c.codigo); toast({ title: 'Cuenta desactivada', body: c.nombre }) }
      reload()
    } catch (e) {
      toast({ title: 'No se pudo cambiar el estado', body: e?.message || 'Error', kind: 'error' })
    }
  }

  return (
    <div>
      {puedeEditar ? (
        <div className="flex justify-end mb-3">
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={abrirCrear}>Nueva cuenta</Button>
        </div>
      ) : null}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                <th className="py-2.5 px-3 font-medium">Código</th>
                <th className="py-2.5 pr-3 font-medium">Cuenta</th>
                <th className="py-2.5 pr-3 font-medium">Tipo</th>
                <th className="py-2.5 pr-3 font-medium">Naturaleza</th>
                <th className="py-2.5 pr-3 font-medium text-right">Saldo</th>
                {puedeEditar ? <th className="py-2.5 px-3 font-medium text-right">Acciones</th> : null}
              </tr>
            </thead>
            <tbody>
              {ordenadas.map(({ c, depth }) => (
                <tr key={c.codigo} className={`border-b border-slate-100 dark:border-slate-800/70 ${c.desactivada ? 'opacity-50' : ''}`}>
                  <td className="py-2.5 px-3 mono text-[12.5px]" style={{ paddingLeft: 12 + depth * 18 }}>
                    {depth > 0 ? <span className="text-slate-300 dark:text-slate-600 mr-1">└</span> : null}{c.codigo}
                  </td>
                  <td className="py-2.5 pr-3 font-medium text-[13px]">
                    {c.nombre}
                    {esPadre(c.codigo) ? <Badge size="sm" color="blue" className="ml-2">grupo</Badge> : null}
                    {c.base ? <Badge size="sm" color="slate" className="ml-2">base</Badge> : null}
                    {c.desactivada ? <Badge size="sm" color="amber" className="ml-2">inactiva</Badge> : null}
                  </td>
                  <td className="py-2.5 pr-3"><Badge size="sm" color={TIPO_COLOR[c.tipo] || 'slate'}>{c.tipo}</Badge></td>
                  <td className="py-2.5 pr-3 text-[12px] text-slate-500">{c.deudora ? 'deudora' : 'acreedora'}</td>
                  <td className={`py-2.5 pr-3 text-right num private-mask ${esPadre(c.codigo) ? 'font-semibold' : ''}`}>{fmtCurrency(saldoRollup(c.codigo), 'VES')}</td>
                  {puedeEditar ? (
                    <td className="py-2.5 px-3">
                      <div className="flex items-center justify-end gap-1">
                        <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />} onClick={() => abrirEditar(c)} title="Renombrar">Renombrar</Button>
                        {c.base ? (
                          <span className="text-[11px] text-slate-400 px-1" title="Cuenta base del sistema: no se puede desactivar">del sistema</span>
                        ) : (
                          <Button size="sm" variant="ghost" onClick={() => alternarActiva(c)}>
                            {c.desactivada ? 'Activar' : 'Desactivar'}
                          </Button>
                        )}
                      </div>
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <div className="mt-3 text-[11.5px] text-slate-400">
        El plan base se siembra al entrar y se <b>amplía según el giro</b> de tu empresa (bodega, farmacia,
        ferretería, servicios…). Puedes agregar tus propias cuentas, renombrarlas y desactivar las que no uses;
        las cuentas <b>base del sistema</b> (las que usan los asientos automáticos) no se pueden desactivar, y
        ninguna cuenta se elimina —los asientos ya emitidos la referencian por código. Puedes crear
        <b> subcuentas</b> (grupo → detalle): solo las hojas reciben asientos y el grupo agrega sus saldos.
      </div>

      <Modal open={!!form} onClose={() => setForm(null)}
        title={form?.mode === 'crear' ? 'Nueva cuenta' : 'Renombrar cuenta'}
        sub={form?.mode === 'crear' ? 'Agrega una cuenta a tu plan.' : 'El código y el tipo no cambian; solo el nombre.'}
        footer={<>
          <Button variant="ghost" onClick={() => setForm(null)}>Cancelar</Button>
          <Button onClick={guardar} loading={guardando}
            disabled={!form?.nombre?.trim() || (form?.mode === 'crear' && !form?.codigo?.trim())}>
            {form?.mode === 'crear' ? 'Crear' : 'Guardar'}
          </Button>
        </>}>
        {form ? (
          <div className="space-y-3">
            <Field label="Código" required>
              <Input value={form.codigo} onChange={(e) => setForm({ ...form, codigo: e.target.value })}
                placeholder="6101" disabled={form.mode === 'editar'} className="mono" />
            </Field>
            <Field label="Nombre" required>
              <Input value={form.nombre} onChange={(e) => setForm({ ...form, nombre: e.target.value })}
                placeholder="Gastos de mercadeo" />
            </Field>
            {form.mode === 'crear' ? (
              <Field label="Cuenta padre (opcional)" hint="Conviértela en subcuenta; hereda el tipo del padre.">
                <Select value={form.codigoPadre} onChange={(e) => {
                  const padre = plan.find((x) => x.codigo === e.target.value)
                  setForm({ ...form, codigoPadre: e.target.value, tipo: padre ? padre.tipo : form.tipo })
                }}>
                  <option value="">— Cuenta de primer nivel —</option>
                  {plan.map((x) => <option key={x.codigo} value={x.codigo}>{x.codigo} · {x.nombre}</option>)}
                </Select>
              </Field>
            ) : null}
            <Field label="Tipo" required hint={form.codigoPadre ? 'Se hereda de la cuenta padre.' : undefined}>
              <Select value={form.tipo} onChange={(e) => setForm({ ...form, tipo: e.target.value })}
                disabled={form.mode === 'editar' || !!form.codigoPadre}>
                {TIPOS_CUENTA.map((t) => <option key={t} value={t}>{t}</option>)}
              </Select>
            </Field>
          </div>
        ) : null}
      </Modal>
    </div>
  )
}

/* --- Estados financieros ------------------------------------------------- */

function Estados() {
  // Período del Estado de Resultados (P&G). Vacío = acumulado desde el inicio. El
  // Balance de comprobación es siempre acumulado (balance de saldos).
  const [desde, setDesde] = useState('')
  const [hasta, setHasta] = useState('')
  // Balance y resultados son ambos esenciales para los estados financieros: si
  // cualquiera falla, se muestra error+Reintentar (antes: skeleton infinito).
  const { data, loading, error, reload } = useRecurso(
    () => Promise.all([api.balanceContable(), api.resultadosContables(desde, hasta)]).then(([balance, res]) => ({ balance, res })), [desde, hasta])
  if (loading || error) {
    return <EstadoRecurso loading={loading} error={error} onRetry={reload} cols={3} rows={5}
      title="No se pudieron cargar los estados financieros" />
  }
  const { balance, res } = data

  const filas = [
    { t: 'Ventas gravadas', v: res.ventas },
    { t: 'Ventas exentas', v: res.ventasExentas },
    { t: 'Ingreso total', v: res.ingresoTotal, fuerte: true },
    { t: 'Costo de ventas', v: -res.costoDeVentas, negativo: true },
    { t: 'Utilidad bruta', v: res.utilidadBruta, fuerte: true },
    { t: 'Gastos operativos', v: -res.gastos, negativo: true },
    { t: 'Utilidad neta', v: res.utilidadNeta, fuerte: true, dinero: true },
  ]

  return (
    <div className="space-y-4">
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
      {/* Estado de resultados */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
        <div className="flex items-center justify-between gap-2 mb-3 flex-wrap">
          <div className="text-[13px] font-semibold">Estado de resultados</div>
          <div className="flex items-center gap-1.5">
            <Input type="date" className="!h-8 !text-[12px]" value={desde} max={hasta || undefined} onChange={(e) => setDesde(e.target.value)} title="Desde" />
            <span className="text-slate-400 text-[12px]">a</span>
            <Input type="date" className="!h-8 !text-[12px]" value={hasta} min={desde || undefined} onChange={(e) => setHasta(e.target.value)} title="Hasta" />
            {desde || hasta ? (
              <button type="button" onClick={() => { setDesde(''); setHasta('') }} title="Todo (acumulado)"
                className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800"><Icon.X size={14} /></button>
            ) : null}
          </div>
        </div>
        <div className="text-[11px] text-slate-400 mb-2">{desde || hasta ? 'Período seleccionado' : 'Acumulado desde el inicio'}</div>
        <div className="space-y-1.5 text-[13px]">
          {filas.map((f) => (
            <div key={f.t} className={`flex items-center justify-between ${f.fuerte ? 'border-t border-slate-100 dark:border-slate-800 pt-1.5 font-semibold' : ''}`}>
              <span className={f.fuerte ? '' : 'text-slate-500'}>{f.t}</span>
              <span className={`num private-mask ${f.dinero ? 'text-teal-600 dark:text-teal-400' : f.negativo ? 'text-[#B3362C] dark:text-red-400' : ''}`}>
                {fmtCurrency(f.v, 'VES')}
              </span>
            </div>
          ))}
          <div className="flex items-center justify-between text-[11.5px] text-slate-400 pt-1">
            <span>Margen bruto</span><span className="num">{fmtNum(res.margenBruto, 1)}%</span>
          </div>
        </div>
        <div className="mt-3 text-[11px] text-slate-400">
          Sale del libro diario, que a su vez sale de las facturas y del costo promedio del inventario.
          Los gastos operativos aparecen cuando exista el módulo de Compras y gastos.
        </div>
      </div>

      {/* Balance de comprobación */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
        <div className="flex items-center justify-between mb-3">
          <div className="text-[13px] font-semibold">Balance de comprobación</div>
          {balance.cuadra
            ? <Badge size="sm" color="emerald" dot>cuadra</Badge>
            : <Badge size="sm" color="red" dot>descuadrado</Badge>}
        </div>
        <table className="w-full text-[12.5px]">
          <thead>
            <tr className="text-left text-[10.5px] uppercase tracking-wide text-slate-400">
              <th className="pb-1.5 font-medium">Cuenta</th>
              <th className="pb-1.5 font-medium text-right">Debe</th>
              <th className="pb-1.5 font-medium text-right">Haber</th>
            </tr>
          </thead>
          <tbody>
            {balance.cuentas.filter((c) => c.debe || c.haber).map((c) => (
              <tr key={c.codigo} className="border-t border-slate-50 dark:border-slate-800/60">
                <td className="py-1.5"><span className="mono text-slate-400 mr-1.5">{c.codigo}</span>{c.nombre}</td>
                <td className="py-1.5 text-right num private-mask">{c.debe ? fmtCurrency(c.debe, 'VES') : ''}</td>
                <td className="py-1.5 text-right num text-slate-500 private-mask">{c.haber ? fmtCurrency(c.haber, 'VES') : ''}</td>
              </tr>
            ))}
            <tr className="border-t border-slate-200 dark:border-slate-700 font-semibold">
              <td className="py-1.5">Totales · {fmtNum(balance.asientos, 0)} asiento(s)</td>
              <td className="py-1.5 text-right num private-mask">{fmtCurrency(balance.totalDebe, 'VES')}</td>
              <td className="py-1.5 text-right num private-mask">{fmtCurrency(balance.totalHaber, 'VES')}</td>
            </tr>
          </tbody>
        </table>
        <div className="mt-3 text-[11px] text-slate-400">
          Que los dos totales sean iguales es la prueba de que el libro está sano. Si algún día no cuadran,
          esta pantalla lo dice en lugar de esconderlo.
        </div>
      </div>
    </div>

    <Periodos />
    </div>
  )
}

/* --- Cierre de período contable (§7.3) -----------------------------------
 *
 * Cerrar un mes lo sella DEFINITIVAMENTE: no hay reapertura. A partir del cierre,
 * ningún asiento puede caer dentro de ese mes. Solo se cierra un mes ya terminado
 * (nunca el mes en curso), así la operación de hoy nunca se bloquea.
 */

const MESES = ['enero', 'febrero', 'marzo', 'abril', 'mayo', 'junio', 'julio', 'agosto', 'septiembre', 'octubre', 'noviembre', 'diciembre']

// mesAnterior devuelve el año/mes del mes calendario anterior al actual (1..12).
function mesAnterior() {
  const d = new Date()
  d.setDate(1)
  d.setMonth(d.getMonth() - 1)
  return { anio: d.getFullYear(), mes: d.getMonth() + 1 }
}

function Periodos() {
  const { ui } = useUI()
  const toast = useToast()
  const [cerrando, setCerrando] = useState(false)

  const puedeCerrar = ui.rol === 'dueno' || ui.rol === 'contadora'

  const { data: periodos, loading, error, reload: cargar } = useRecurso(() => api.periodosCerrados().then((r) => r || []), [])

  const orden = (periodos || []).slice().sort((a, b) => (b.anio - a.anio) || (b.mes - a.mes))

  return (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
      <div className="flex items-center justify-between mb-3">
        <div className="text-[13px] font-semibold">Períodos contables</div>
        {puedeCerrar ? (
          <Button size="sm" variant="destructive" icon={<Icon.Lock size={15} />} onClick={() => setCerrando(true)}>Cerrar mes</Button>
        ) : null}
      </div>

      {loading ? (
        <TableSkeleton rows={2} cols={3} />
      ) : error ? (
        <div className="flex items-center justify-between gap-3 rounded-lg border border-dashed border-slate-300 dark:border-slate-700 px-3.5 py-3">
          <span className="inline-flex items-center gap-1.5 text-[12.5px] text-red-600 dark:text-red-400">
            <Icon.CircleAlert size={14} /> No se pudieron cargar los períodos.
          </span>
          <Button size="sm" variant="secondary" icon={<Icon.Refresh size={14} />} onClick={cargar}>Reintentar</Button>
        </div>
      ) : orden.length === 0 ? (
        <div className="text-[12.5px] text-slate-500">
          Ningún mes cerrado todavía. Cerrar un mes lo sella: sus asientos quedan fijos y no admite ninguno nuevo.
        </div>
      ) : (
        <table className="w-full text-[12.5px]">
          <thead>
            <tr className="text-left text-[10.5px] uppercase tracking-wide text-slate-400">
              <th className="pb-1.5 font-medium">Mes cerrado</th>
              <th className="pb-1.5 font-medium">Cerrado por</th>
              <th className="pb-1.5 font-medium text-right">Cuándo</th>
            </tr>
          </thead>
          <tbody>
            {orden.map((p) => (
              <tr key={p.id} className="border-t border-slate-50 dark:border-slate-800/60">
                <td className="py-1.5">
                  <span className="font-medium capitalize">{MESES[p.mes - 1]} {p.anio}</span>
                  <Badge size="sm" color="slate" className="ml-2">cerrado</Badge>
                </td>
                <td className="py-1.5 text-slate-500">{p.cerradoPor}</td>
                <td className="py-1.5 text-right num text-slate-500">{fmtDate(p.cerradoEl)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div className="mt-3 text-[11px] text-slate-400">
        El cierre es <strong>definitivo</strong> (§7.3): un período cerrado no se reabre. Solo se puede cerrar un
        mes ya terminado, nunca el mes en curso, para no bloquear las operaciones del día.
      </div>

      {cerrando ? (
        <CerrarMesModal
          onClose={() => setCerrando(false)}
          onHecho={(p) => { cargar(); toast({ title: 'Período cerrado', body: `${MESES[p.mes - 1]} ${p.anio} quedó cerrado definitivamente.` }) }}
        />
      ) : null}
    </div>
  )
}

function CerrarMesModal({ onClose, onHecho }) {
  const def = mesAnterior()
  const [anio, setAnio] = useState(def.anio)
  const [mes, setMes] = useState(def.mes)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const anioActual = new Date().getFullYear()
  const anios = [anioActual, anioActual - 1, anioActual - 2, anioActual - 3]

  const cerrar = async () => {
    setBusy(true); setError('')
    try {
      const p = await api.cerrarPeriodo({ anio: Number(anio), mes: Number(mes) })
      onHecho(p)
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo cerrar el período.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Check size={18} />}
      title="Cerrar mes contable" sub="El cierre es definitivo — no se reabre (§7.3)"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant="destructive" onClick={cerrar} loading={busy} icon={<Icon.Lock size={16} />}>Cerrar definitivamente</Button>
      </>}>
      <div className="space-y-3">
        <div className="text-[12.5px] text-slate-600 dark:text-slate-300">
          Al cerrar, los asientos de ese mes quedan <strong>fijos</strong> y no se admite ninguno nuevo con esa
          fecha. Solo se puede cerrar un mes ya terminado y en orden ascendente.
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Mes">
            <Select value={mes} onChange={(e) => { setMes(e.target.value); setError('') }}>
              {MESES.map((m, i) => <option key={i} value={i + 1}>{m}</option>)}
            </Select>
          </Field>
          <Field label="Año">
            <Select value={anio} onChange={(e) => { setAnio(e.target.value); setError('') }}>
              {anios.map((a) => <option key={a} value={a}>{a}</option>)}
            </Select>
          </Field>
        </div>
        {error ? <div className="text-[12.5px] text-[#B3362C] dark:text-red-400">{error}</div> : null}
      </div>
    </Modal>
  )
}
