import { useState, useEffect, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Input, Select, Empty, TableSkeleton, useToast, Field, Modal } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { useUI } from '../context/UIContext.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { useData } from '../context/DataContext.jsx'
import { api } from '../lib/api.js'

/* CONTEO FÍSICO.
 *
 * Cuadrar un almacén producto por producto obliga a hacer una operación por
 * renglón, y en la práctica eso significa que no se cuadra: nadie hace ochenta
 * ajustes a mano. Aquí se teclea la hoja entera y se aplica de una vez.
 *
 * SE MIRA ANTES DE APLICAR, siempre. Un conteo mal tecleado es una merma masiva
 * que ya no se puede deshacer —el ledger es de solo anexado—, y el momento de
 * descubrir que alguien puso 5 donde había 500 es este, no el balance del mes.
 *
 * LO QUE NO SE TECLEA NO SE TOCA. Contar un pasillo un martes y otro el jueves es
 * lo normal; dejar en blanco una fila la excluye de la hoja en vez de ponerla en
 * cero, que convertiría cada conteo parcial en una merma del almacén entero.
 */
export function Conteo() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const { activeSede, activeSedeId } = useAuth()
  const toast = useToast()

  const almacenes = (db.ALMACENES || []).filter((a) => a.activo && a.sedeId === activeSede?.id)
  const [almacenSel, setAlmacenSel] = useState('')
  const [ubicaciones, setUbicaciones] = useState([])
  const [ubicacionSel, setUbicacionSel] = useState('')
  const [q, setQ] = useState('')
  const [contado, setContado] = useState({}) // sku → texto tecleado
  const [motivo, setMotivo] = useState('')
  const [previa, setPrevia] = useState(null)
  // PLANES: qué toca contar. El plan no cuenta por su cuenta —contar es ir al
  // estante— pero dice cuál toca y deja la hoja preparada.
  const [planes, setPlanes] = useState([])
  const [planSel, setPlanSel] = useState('')
  const [formPlan, setFormPlan] = useState(null)
  const [cargando, setCargando] = useState(false)
  const [existencias, setExistencias] = useState(null)

  useEffect(() => {
    if (almacenes.length && !almacenSel) setAlmacenSel(almacenes.find((a) => a.principal)?.id || almacenes[0].id)
  }, [almacenes.length]) // eslint-disable-line react-hooks/exhaustive-deps

  const cargarPlanes = async () => {
    try { setPlanes((await api.planesDeConteo())?.planes || []) } catch { setPlanes([]) }
  }
  useEffect(() => { cargarPlanes() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Tomar un plan NO cuenta nada: trae su hoja con lo que el sistema cree que hay,
  // ya acotada a su almacén y su casilla. Lo que falta es ir a mirar.
  const tomarPlan = async (id) => {
    setPlanSel(id)
    setPrevia(null)
    setContado({})
    if (!id) return
    try {
      const h = await api.hojaDeConteo(id)
      if (h.almacenId) setAlmacenSel(h.almacenId)
      setMotivo(h.nombre || '')
      toast({
        title: 'Hoja preparada',
        body: `${h.lineas.length} producto(s) que deberían estar ${h.ubicacion ? 'en ' + h.ubicacion : 'ahí'}. Ve a contarlos.`,
      })
    } catch (e) {
      toast({ title: 'No se pudo preparar la hoja', body: e?.message || 'Error', kind: 'error' })
    }
  }

  useEffect(() => {
    if (!almacenSel) { setUbicaciones([]); return }
    let vivo = true
    api.ubicaciones(almacenSel)
      .then((r) => { if (vivo) setUbicaciones((r?.ubicaciones || []).filter((u) => u.activa)) })
      .catch(() => { if (vivo) setUbicaciones([]) })
    return () => { vivo = false }
  }, [almacenSel])

  useEffect(() => {
    let vivo = true
    api.existencias(activeSedeId, almacenSel || undefined)
      .then((r) => { if (vivo) setExistencias(r || []) })
      .catch(() => { if (vivo) setExistencias([]) })
    return () => { vivo = false }
  }, [activeSedeId, almacenSel])

  const filas = useMemo(() => {
    const t = q.trim().toLowerCase()
    return (existencias || []).filter((e) => !t || e.sku.toLowerCase().includes(t) || (e.nombre || '').toLowerCase().includes(t))
  }, [existencias, q])

  // Solo las filas TECLEADAS entran en la hoja. Una casilla vacía no es un cero.
  const lineas = useMemo(() => Object.entries(contado)
    .filter(([, v]) => String(v).trim() !== '' && Number.isFinite(Number(v)))
    .map(([sku, v]) => ({ sku, ubicacionId: ubicacionSel, contado: Number(v) })), [contado, ubicacionSel])

  const guardarPlan = async () => {
    if (!formPlan.nombre.trim()) return toast({ title: 'Ponle un nombre al plan', kind: 'error' })
    try {
      await api.crearPlanDeConteo({
        nombre: formPlan.nombre.trim(),
        cadaDias: Number(formPlan.cadaDias) || 0,
        almacenId: formPlan.almacenId || '',
        ubicacionId: formPlan.ubicacionId || '',
        rubro: formPlan.rubro || '',
        activo: true,
      })
      toast({ title: 'Plan creado', body: `${formPlan.nombre.trim()}. Aparecerá arriba cuando toque.` })
      setFormPlan(null)
      cargarPlanes()
    } catch (e) {
      toast({ title: 'No se pudo crear', body: e?.message || 'Error', kind: 'error' })
    }
  }

  const previsualizar = async () => {
    if (lineas.length === 0) return toast({ title: 'No has contado nada todavía', kind: 'error' })
    setCargando(true)
    try {
      setPrevia(await api.previsualizarConteo({ almacenId: almacenSel, lineas }))
    } catch (e) {
      toast({ title: 'No se pudo calcular', body: e?.message || 'Error', kind: 'error' })
    } finally { setCargando(false) }
  }

  const aplicar = async () => {
    if (!motivo.trim()) return toast({ title: 'Ponle un nombre al conteo', body: 'Es lo que une todos los ajustes en el Kardex.', kind: 'error' })
    setCargando(true)
    try {
      // Con un plan elegido se aplica POR EL PLAN: es lo que sella la fecha del
      // último conteo. Aplicarlo suelto contaría igual pero dejaría el plan
      // marcado como pendiente para siempre.
      const r = planSel
        ? await api.aplicarConteoDePlan(planSel, { motivo: motivo.trim(), lineas })
        : await api.aplicarConteo({ almacenId: almacenSel, motivo: motivo.trim(), lineas })
      toast({
        title: 'Conteo aplicado',
        body: `${r.conAjuste} ajuste(s), ${r.sinCambio} sin cambio${r.conError ? `, ${r.conError} con error` : ''}.`,
        kind: r.conError ? 'error' : undefined,
      })
      setPrevia(r)
      setContado({})
      reload()
      cargarPlanes()
      api.existencias(activeSedeId, almacenSel || undefined).then(setExistencias).catch(() => {})
    } catch (e) {
      toast({ title: 'No se pudo aplicar', body: e?.message || 'Error', kind: 'error' })
    } finally { setCargando(false) }
  }

  const pad = 'py-2.5 pl-4'

  return (
    <div>
      <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-3xl mb-3">
        Teclea <strong>lo que hay de verdad</strong> en el anaquel, no la diferencia. Lo que dejes en
        blanco no se toca: contar un pasillo hoy y otro mañana es lo normal.
        <strong> Mira la vista previa antes de aplicar</strong> — los ajustes van al Kardex y no se pueden deshacer.
      </div>

      {/* Lo primero: qué toca contar. Sin esto, se cuenta el almacén que alguien
          recuerda — y lo que lleva meses sin revisarse es justo lo que nadie
          recuerda. */}
      {planes.some((p) => p.vencido) ? (
        <div className="mb-3 rounded-xl bg-amber-50 dark:bg-amber-900/25 border border-amber-200 dark:border-amber-900/40 px-3 py-2.5">
          <div className="flex items-start gap-2.5 text-[12.5px] text-amber-900 dark:text-amber-200">
            <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
            <div className="min-w-0">
              <strong>{planes.filter((p) => p.vencido).length} conteo(s) pendiente(s).</strong>
              <div className="mt-1 space-y-0.5">
                {planes.filter((p) => p.vencido).slice(0, 5).map((p) => (
                  <div key={p.id} className="flex items-baseline gap-2 flex-wrap">
                    <span className="text-[11.5px] font-medium">{p.nombre}</span>
                    <span className="text-[11.5px] opacity-90">
                      {p.diasDesde < 0 ? 'nunca contado' : `hace ${p.diasDesde} día(s)`}
                      {p.ubicacion ? ` · ${p.ubicacion}` : ''} · {p.productos} producto(s)
                    </span>
                    <button onClick={() => tomarPlan(p.id)} className="text-[11.5px] underline">preparar la hoja</button>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      ) : null}

      <div className="flex items-center gap-2 flex-wrap mb-3">
        {planes.length ? (
          <Select className="!w-60" value={planSel} onChange={(e) => tomarPlan(e.target.value)}
            title="Contar siguiendo un plan sella su fecha; sin plan, el conteo es suelto">
            <option value="">Conteo suelto (sin plan)</option>
            {planes.map((p) => (
              <option key={p.id} value={p.id}>{p.vencido ? '● ' : ''}{p.nombre}</option>
            ))}
          </Select>
        ) : null}
        <Select className="!w-56" value={almacenSel} onChange={(e) => { setAlmacenSel(e.target.value); setUbicacionSel(''); setPrevia(null) }}>
          {almacenes.map((a) => <option key={a.id} value={a.id}>{a.nombre}{a.principal ? ' ★' : ''}</option>)}
        </Select>
        {ubicaciones.length ? (
          <Select className="!w-56" value={ubicacionSel} onChange={(e) => { setUbicacionSel(e.target.value); setPrevia(null) }}
            title="Qué casilla estás contando">
            <option value="">Sin ubicar (el almacén, sin más detalle)</option>
            {ubicaciones.map((u) => <option key={u.id} value={u.id}>{u.codigo} · {u.nombre}</option>)}
          </Select>
        ) : null}
        <Input className="w-56" icon={<Icon.Search size={15} />} placeholder="Buscar producto…"
          value={q} onChange={(e) => setQ(e.target.value)} />
        <div className="ml-auto text-[12px] text-slate-500">
          {lineas.length} fila(s) contada(s)
        </div>
      </div>

      {existencias === null ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={6} cols={4} /></div>
      ) : filas.length === 0 ? (
        <Empty icon={<Icon.Boxes size={22} />} title="Sin productos que contar"
          body="Este almacén no tiene existencias, o el filtro no encuentra nada." />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <div className="overflow-x-auto max-h-[28rem]">
            <table className="w-full text-sm">
              <thead className="sticky top-0 bg-slate-50 dark:bg-slate-800/90 text-[11px] uppercase tracking-wide text-slate-400">
                <tr>
                  <th className={`${pad} pr-3 text-left font-medium`}>SKU</th>
                  <th className={`${pad} pr-3 text-left font-medium`}>Producto</th>
                  <th className={`${pad} pr-3 text-right font-medium`}>Según el sistema</th>
                  <th className={`${pad} pr-4 text-right font-medium`}>Contado</th>
                </tr>
              </thead>
              <tbody>
                {filas.map((e) => {
                  const v = contado[e.sku] ?? ''
                  const dif = String(v).trim() === '' ? null : Number(v) - (Number(e.cantidad) || 0)
                  return (
                    <tr key={e.sku} className="border-t border-slate-100 dark:border-slate-800/70">
                      <td className={`${pad} pr-3 num text-[12.5px] text-slate-500`}>{e.sku}</td>
                      <td className={`${pad} pr-3 text-[13px]`}>{e.nombre}</td>
                      <td className={`${pad} pr-3 text-right num text-slate-500`}>{fmtNum(e.cantidad)}</td>
                      <td className={`${pad} pr-4 text-right`}>
                        <input type="number" step="any" min="0" value={v}
                          onChange={(ev) => { setContado((c) => ({ ...c, [e.sku]: ev.target.value })); setPrevia(null) }}
                          className="num w-24 text-right rounded border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-2 py-1" />
                        {dif !== null && Math.abs(dif) > 0.0001 ? (
                          <div className={`text-[11px] ${dif < 0 ? 'text-amber-600 dark:text-amber-400' : 'text-emerald-600 dark:text-emerald-400'}`}>
                            {dif > 0 ? '+' : ''}{fmtNum(dif)}
                          </div>
                        ) : null}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <div className="mt-3 flex items-end gap-2 flex-wrap">
        <Field label="Nombre del conteo" hint="Une todos los ajustes en el Kardex.">
          <Input className="w-72" value={motivo} onChange={(e) => setMotivo(e.target.value)}
            placeholder="Conteo del pasillo A · marzo" />
        </Field>
        <Button variant="ghost" onClick={previsualizar} disabled={cargando || lineas.length === 0}>
          Ver qué pasaría
        </Button>
        <Button onClick={aplicar} disabled={cargando || lineas.length === 0 || !previa}>
          {cargando ? 'Aplicando…' : 'Aplicar conteo'}
        </Button>
        {!previa && lineas.length > 0 ? (
          <div className="text-[12px] text-slate-500 self-center">Mira primero qué pasaría.</div>
        ) : null}
      </div>

      {/* Los planes se administran aquí, donde se usan: mandarlos a Configuración
          los convertiría en algo que se define una vez y nadie vuelve a mirar. */}
      <div className="mt-6">
        <div className="flex items-center justify-between gap-3 mb-2">
          <div className="text-[12.5px] text-slate-500">
            Planes de conteo — <span className="text-slate-400">dicen qué toca y cada cuánto. Contar sigue siendo ir al estante.</span>
          </div>
          <Button size="sm" variant="ghost" icon={<Icon.Plus size={15} />}
            onClick={() => setFormPlan({ nombre: '', cadaDias: '30', almacenId: almacenSel, ubicacionId: '', rubro: '' })}>
            Nuevo plan
          </Button>
        </div>
        {planes.length === 0 ? (
          <div className="text-[12px] text-slate-400">
            Sin planes. Crea uno para que el sistema avise cuándo toca revisar cada almacén.
          </div>
        ) : (
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl overflow-hidden">
            <table className="w-full text-[13px]">
              <tbody>
                {planes.map((p) => (
                  <tr key={p.id} className={`border-b border-slate-100 dark:border-slate-800/70 ${p.activo ? '' : 'opacity-60'}`}>
                    <td className="px-4 py-2">
                      {p.vencido ? <span className="text-amber-600 dark:text-amber-400 mr-1.5">●</span> : null}
                      {p.nombre}
                      <span className="text-slate-400 text-[11.5px]">
                        {p.ubicacion ? ` · ${p.ubicacion}` : p.almacenNombre ? ` · ${p.almacenNombre}` : ''}
                        {p.rubro ? ` · ${p.rubro}` : ''}
                      </span>
                    </td>
                    <td className="px-4 py-2 text-[12px] text-slate-500">
                      {p.cadaDias > 0 ? `cada ${p.cadaDias} día(s)` : 'a demanda'}
                    </td>
                    <td className="px-4 py-2 text-[12px] text-slate-500">
                      {p.diasDesde < 0 ? 'nunca contado' : `contado hace ${p.diasDesde} día(s)`}
                    </td>
                    <td className="px-4 py-2 text-right num text-[12px] text-slate-400">{p.productos} prod.</td>
                    <td className="px-4 py-2 text-right">
                      <Button size="sm" variant="ghost" onClick={() => tomarPlan(p.id)}>Preparar hoja</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {formPlan ? (
        <Modal open onClose={() => setFormPlan(null)} title="Nuevo plan de conteo">
          <div className="space-y-3">
            <Field label="Nombre" hint="Lo que se lee en la lista: «Pasillo A, mensual».">
              <Input value={formPlan.nombre} autoFocus
                onChange={(e) => setFormPlan({ ...formPlan, nombre: e.target.value })} />
            </Field>
            <Field label="Cada cuántos días" hint="Vacío o 0 = a demanda: la hoja queda preparada pero el plan no vence nunca.">
              <Input type="number" min="0" value={formPlan.cadaDias}
                onChange={(e) => setFormPlan({ ...formPlan, cadaDias: e.target.value })} />
            </Field>
            <Field label="Almacén">
              <Select value={formPlan.almacenId} onChange={(e) => setFormPlan({ ...formPlan, almacenId: e.target.value, ubicacionId: '' })}>
                <option value="">El principal de la sede</option>
                {almacenes.map((a) => <option key={a.id} value={a.id}>{a.nombre}</option>)}
              </Select>
            </Field>
            {ubicaciones.length ? (
              <Field label="Ubicación" hint="Acotar a un pasillo es lo que permite contar por partes sin cerrar el almacén.">
                <Select value={formPlan.ubicacionId} onChange={(e) => setFormPlan({ ...formPlan, ubicacionId: e.target.value })}>
                  <option value="">Todo el almacén</option>
                  {ubicaciones.map((u) => <option key={u.id} value={u.id}>{u.codigo} · {u.nombre}</option>)}
                </Select>
              </Field>
            ) : null}
            <div className="flex justify-end gap-2 pt-1">
              <Button variant="ghost" onClick={() => setFormPlan(null)}>Cancelar</Button>
              <Button onClick={guardarPlan}>Crear plan</Button>
            </div>
          </div>
        </Modal>
      ) : null}

      {previa ? (
        <div className="mt-4 rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
          <div className="px-4 py-2.5 bg-slate-50 dark:bg-slate-800/60 text-[12.5px] flex flex-wrap gap-3">
            <span><strong>{previa.conAjuste}</strong> con diferencia</span>
            <span className="text-slate-500">{previa.sinCambio} sin cambio</span>
            {previa.conError ? <span className="text-red-600 dark:text-red-400">{previa.conError} con error</span> : null}
            <span className="ml-auto">
              Impacto en el valor del inventario:{' '}
              <strong className={previa.valorNeto < 0 ? 'text-amber-700 dark:text-amber-400' : ''}>
                {fmtCurrency(previa.valorNeto, ui.ccy)}
              </strong>
            </span>
          </div>
          <table className="w-full text-[13px]">
            <tbody>
              {previa.lineas.filter((l) => Math.abs(l.diferencia) > 0.0001 || l.error).map((l) => (
                <tr key={l.sku + l.ubicacionId} className="border-t border-slate-100 dark:border-slate-800">
                  <td className="px-4 py-2 num text-[12px] text-slate-500">{l.sku}</td>
                  <td className="px-4 py-2">{l.nombre}<span className="text-slate-400 text-[11.5px]"> · {l.ubicacion}</span></td>
                  <td className="px-4 py-2 text-right num text-slate-500">{fmtNum(l.sistema)} → {fmtNum(l.contado)}</td>
                  <td className={`px-4 py-2 text-right num font-medium ${l.diferencia < 0 ? 'text-amber-700 dark:text-amber-400' : 'text-emerald-700 dark:text-emerald-400'}`}>
                    {l.diferencia > 0 ? '+' : ''}{fmtNum(l.diferencia)}
                  </td>
                  <td className="px-4 py-2 text-[11.5px] text-red-600 dark:text-red-400">{l.error || (l.ajustado ? '✓ aplicado' : '')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  )
}
