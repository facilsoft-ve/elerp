import { useState, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Segmented, Toggle, Field, VistaDetalle, Empty, TableSkeleton, useToast } from '../components/primitives.jsx'
import { SubidorImagen } from '../components/SubidorImagen.jsx'
import { fmtNum, fmtDate } from '../lib/format.js'
import { api } from '../lib/api.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'

/* Promociones (submódulo de Ventas › Catálogo y precios). Una biblioteca de
 * anuncios (de IMAGEN o de TEXTO, con vigencia) que alimentan el espacio
 * promocional (carrusel) de la PANTALLA DEL CLIENTE del POS cuando están activas
 * y dentro de su vigencia.
 *
 * Es un maestro EDITABLE (no ledger append-only): se crean y editan. El carrusel
 * las combina con los slides manuales que la empresa carga en Ajustes › Pantalla
 * del cliente. Solo Dueña/Desarrollador editan; los demás roles ven en consulta.
 * El alta/edición va a PANTALLA COMPLETA (VistaDetalle). */

const puedeEditar = (rol) => ['dueno', 'desarrollador'].includes(rol)

const hoyISO = () => new Date().toISOString().slice(0, 10)

// estadoPromocion deriva el estado visual: inactiva, vencida, programada (aún no
// vigente) o vigente (activa y dentro del periodo — la que sale en el carrusel).
function estadoPromocion(p) {
  const hoy = hoyISO()
  if (!p.activa) return { label: 'Inactiva', color: 'slate' }
  if (p.hasta && hoy > p.hasta) return { label: 'Vencida', color: 'red' }
  if (p.desde && hoy < p.desde) return { label: 'Programada', color: 'amber' }
  return { label: 'Vigente', color: 'emerald' }
}

// Vigencia legible.
const vigenciaLabel = (p) => {
  if (!p.desde && !p.hasta) return 'Sin límite'
  if (p.desde && p.hasta) return `${fmtDate(p.desde)} → ${fmtDate(p.hasta)}`
  if (p.hasta) return `Hasta ${fmtDate(p.hasta)}`
  return `Desde ${fmtDate(p.desde)}`
}

export function Promociones() {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()

  const promociones = db.PROMOCIONES || []
  const [q, setQ] = useState('')
  const [editar, setEditar] = useState(null) // promoción en edición | 'nueva' | null

  const edita = puedeEditar(ui.rol)

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    const lista = promociones.slice().sort((a, b) => (a.orden || 0) - (b.orden || 0) || String(a.nombre || '').localeCompare(String(b.nombre || '')))
    return lista.filter((p) => !term
      || (p.nombre || '').toLowerCase().includes(term)
      || (p.titulo || '').toLowerCase().includes(term))
  }, [promociones, q])

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las promociones"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  if (editar) {
    return (
      <FormPromocion promocion={editar === 'nueva' ? null : editar}
        onVolver={() => setEditar(null)} onSaved={reload} toast={toast} />
    )
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por nombre o título…" value={q} onChange={(e) => setQ(e.target.value)} />
        {edita ? <Button className="ml-auto" icon={<Icon.Plus size={16} />} onClick={() => setEditar('nueva')}>Nueva promoción</Button> : null}
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || db.PROMOCIONES === undefined ? (
          <div className="p-4"><TableSkeleton rows={5} cols={edita ? 6 : 5} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.Sparkles size={22} />}
            title={q ? 'Sin resultados' : 'Aún no hay promociones'}
            body={q ? 'Prueba con otro término.' : 'Crea una promoción (de imagen o de texto) para que aparezca en el carrusel de la pantalla del cliente cuando esté vigente.'}
            cta={edita && !q ? <Button icon={<Icon.Plus size={16} />} onClick={() => setEditar('nueva')}>Nueva promoción</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 pr-3 font-medium">Promoción</th>
                  <th className="py-2.5 pr-3 font-medium">Tipo</th>
                  <th className="py-2.5 pr-3 font-medium">Vigencia</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Orden</th>
                  <th className="py-2.5 pr-3 font-medium">Estado</th>
                  {edita ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                </tr>
              </thead>
              <tbody>
                {rows.map((p) => {
                  const est = estadoPromocion(p)
                  const esImagen = p.tipo === 'imagen'
                  return (
                    <tr key={p.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover cursor-pointer" onClick={() => edita && setEditar(p)}>
                      <td className="py-2.5 pr-3">
                        <div className="flex items-center gap-2.5 min-w-0">
                          {esImagen && (p.imagen || '').trim() ? (
                            <img src={p.imagen} alt="" className="aspect-[16/9] w-16 rounded object-cover border border-slate-200 dark:border-slate-700 bg-white shrink-0"
                              onError={(e) => { e.currentTarget.style.visibility = 'hidden' }} />
                          ) : (
                            <span className="aspect-[16/9] w-16 rounded border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 inline-flex items-center justify-center text-slate-400 shrink-0">
                              <Icon.Tag size={15} />
                            </span>
                          )}
                          <div className="min-w-0">
                            <div className="font-medium text-[13px] truncate">{p.nombre}</div>
                            {p.titulo ? <div className="text-[11px] text-slate-400 truncate">{p.titulo}</div> : null}
                          </div>
                        </div>
                      </td>
                      <td className="py-2.5 pr-3"><Badge size="sm" color={esImagen ? 'teal' : 'slate'}>{esImagen ? 'Imagen' : 'Texto'}</Badge></td>
                      <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">{vigenciaLabel(p)}</td>
                      <td className="py-2.5 pr-3 text-right num text-[12.5px] text-slate-500">{fmtNum(p.orden || 0, 0)}</td>
                      <td className="py-2.5 pr-3"><Badge size="sm" color={est.color} dot={est.label === 'Vigente'}>{est.label}</Badge></td>
                      {edita ? (
                        <td className="py-2.5 pr-3 text-right" onClick={(e) => e.stopPropagation()}>
                          <Button variant="ghost" size="sm" icon={<Icon.Pencil size={14} />} onClick={() => setEditar(p)}>Editar</Button>
                        </td>
                      ) : null}
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} promoción(es) · las <strong>vigentes</strong> aparecen en el carrusel de la pantalla del cliente.</div> : null}
    </div>
  )
}

/* Alta/edición de una promoción, a pantalla completa. */
function FormPromocion({ promocion, onVolver, onSaved, toast }) {
  const editando = !!promocion
  const [nombre, setNombre] = useState(promocion?.nombre || '')
  const [tipo, setTipo] = useState(promocion?.tipo || 'imagen')
  const [titulo, setTitulo] = useState(promocion?.titulo || '')
  const [subtexto, setSubtexto] = useState(promocion?.subtexto || '')
  const [imagen, setImagen] = useState(promocion?.imagen || '')
  const [desde, setDesde] = useState(promocion?.desde || '')
  const [hasta, setHasta] = useState(promocion?.hasta || '')
  const [activa, setActiva] = useState(promocion ? promocion.activa !== false : true)
  const [orden, setOrden] = useState(promocion?.orden != null ? String(promocion.orden) : '')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  const esImagen = tipo === 'imagen'

  const errNombre = !nombre.trim() ? 'Ingresa un nombre para la promoción.' : ''
  const errContenido = esImagen
    ? (!imagen.trim() ? 'Sube o pega la imagen de la promoción.' : '')
    : (!titulo.trim() ? 'Escribe el título del mensaje.' : '')
  const errFechas = desde && hasta && hasta < desde ? 'La fecha «hasta» no puede ser anterior a «desde».' : ''
  const err = errNombre || errContenido || errFechas

  const guardar = async () => {
    setTouched(true)
    if (err) return
    setBusy(true)
    const body = {
      nombre: nombre.trim(),
      tipo,
      titulo: esImagen ? '' : titulo.trim(),
      subtexto: esImagen ? '' : subtexto.trim(),
      imagen: esImagen ? imagen.trim() : '',
      desde: desde || '',
      hasta: hasta || '',
      activa,
      orden: Number(orden) || 0,
    }
    try {
      if (editando) {
        await api.actualizarPromocion(promocion.id, body)
        toast({ title: 'Promoción actualizada', body: body.nombre })
      } else {
        await api.crearPromocion(body)
        toast({ title: 'Promoción creada', body: body.nombre })
      }
      await onSaved()
      onVolver()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  const acciones = (
    <>
      <Button variant="ghost" onClick={onVolver}>Cancelar</Button>
      <Button onClick={guardar} loading={busy} disabled={!!err} title={err || 'Guardar la promoción'} icon={<Icon.Check size={16} />}>
        {editando ? 'Guardar cambios' : 'Crear promoción'}
      </Button>
    </>
  )

  const dateInput = 'w-full h-9 px-3 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm ring-focus'

  return (
    <VistaDetalle onVolver={onVolver} icon={<Icon.Sparkles size={18} />}
      titulo={editando ? (promocion.nombre || 'Promoción') : 'Nueva promoción'}
      sub="Aparece en el carrusel de la pantalla del cliente mientras esté activa y dentro de su vigencia." acciones={acciones}>

      {/* Identidad y contenido */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 mb-4 space-y-4">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="md:col-span-2">
            <Field label="Nombre" required error={touched ? errNombre : ''} hint="uso interno; no se muestra al cliente">
              <Input value={nombre} onChange={(e) => setNombre(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!errNombre}
                placeholder="Ej: Café La Cima — septiembre" autoFocus />
            </Field>
          </div>
          <Field label="Tipo de slide">
            <Segmented value={tipo}
              options={[{ value: 'imagen', label: 'Imagen' }, { value: 'texto', label: 'Texto' }]}
              onChange={setTipo} />
          </Field>
        </div>

        {esImagen ? (
          <Field label="Imagen de la promoción" required error={touched ? errContenido : ''}>
            <SubidorImagen value={imagen} onChange={setImagen} carpeta="promociones" />
          </Field>
        ) : (
          <div className="grid grid-cols-1 gap-4">
            <Field label="Título" required error={touched ? errContenido : ''} hint="el texto grande del slide">
              <Input value={titulo} onChange={(e) => setTitulo(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!errContenido}
                placeholder="Ej: 2x1 en bebidas" />
            </Field>
            <Field label="Subtexto" hint="opcional">
              <Input value={subtexto} onChange={(e) => setSubtexto(e.target.value)}
                placeholder="Ej: Solo esta semana — llévate dos y paga una" />
            </Field>
          </div>
        )}
      </div>

      {/* Vigencia y visibilidad */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Vigente desde" hint="opcional">
            <input type="date" value={desde} onChange={(e) => setDesde(e.target.value)} className={dateInput} />
          </Field>
          <Field label="Vigente hasta" hint="opcional" error={touched ? errFechas : ''}>
            <input type="date" value={hasta} onChange={(e) => setHasta(e.target.value)}
              className={`${dateInput} ${touched && errFechas ? 'border-red-400 dark:border-red-500' : ''}`} />
          </Field>
          <Field label="Orden en el carrusel" hint="menor primero">
            <input type="number" min="0" step="1" value={orden} onChange={(e) => setOrden(e.target.value)}
              placeholder="0" className={`${dateInput} text-right num`} />
          </Field>
        </div>
        <div className="mt-4 pt-4 border-t border-slate-100 dark:border-slate-800">
          <Toggle checked={activa} onChange={setActiva}
            label={activa ? 'Activa' : 'Inactiva'}
            sub={activa ? 'Se muestra en el carrusel mientras esté dentro de su vigencia.' : 'Desactivada: se conserva, no se muestra.'} />
        </div>
      </div>

      {err && touched ? (
        <div className="mt-3 flex items-start gap-1.5 text-[12px] text-amber-700 dark:text-amber-400">
          <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{err}</span>
        </div>
      ) : null}
    </VistaDetalle>
  )
}
