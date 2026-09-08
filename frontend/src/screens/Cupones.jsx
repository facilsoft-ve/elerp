import { useState, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, Toggle, Field, VistaDetalle, Empty, TableSkeleton, useToast } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { api } from '../lib/api.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'

/* Cupones de descuento (submódulo de Ventas › Catálogo y precios). Un maestro
 * EDITABLE (no ledger append-only): un CÓDIGO con un descuento (porcentual o de
 * monto fijo) que se aplica al cotizar/cobrar.
 *
 * El descuento NO recalcula impuestos: al aplicarlo (ver Cotizaciones.jsx) se baja
 * el precioUnitario de las líneas y el motor fiscal del servidor calcula el IVA
 * sobre esa base ya descontada. Solo Dueña/Desarrollador editan; los demás roles
 * ven en consulta. El alta/edición va a PANTALLA COMPLETA (VistaDetalle). */

const puedeEditar = (rol) => ['dueno', 'desarrollador'].includes(rol)

// hoy en formato YYYY-MM-DD (comparación lexicográfica, igual que el backend).
const hoyISO = () => new Date().toISOString().slice(0, 10)

// estadoCupon deriva el estado visual de un cupón: inactivo, vencido, programado
// (aún no vigente), agotado (sin usos) o vigente.
function estadoCupon(c) {
  const hoy = hoyISO()
  if (!c.activo) return { label: 'Inactivo', color: 'slate' }
  if (c.hasta && hoy > c.hasta) return { label: 'Vencido', color: 'red' }
  if (c.desde && hoy < c.desde) return { label: 'Programado', color: 'amber' }
  if (c.usosMax > 0 && (c.usosActuales || 0) >= c.usosMax) return { label: 'Agotado', color: 'red' }
  return { label: 'Vigente', color: 'emerald' }
}

// Descuento legible: «10%» o el monto fijo en Bs.
const descuentoLabel = (c) =>
  c.tipo === 'porcentaje' ? `${fmtNum(c.valor, 2)}%` : fmtCurrency(c.valor, 'VES')

// Vigencia legible.
const vigenciaLabel = (c) => {
  if (!c.desde && !c.hasta) return 'Sin límite'
  if (c.desde && c.hasta) return `${fmtDate(c.desde)} → ${fmtDate(c.hasta)}`
  if (c.hasta) return `Hasta ${fmtDate(c.hasta)}`
  return `Desde ${fmtDate(c.desde)}`
}

export function Cupones() {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()

  const cupones = db.CUPONES || []
  const [q, setQ] = useState('')
  const [editar, setEditar] = useState(null) // cupón en edición | 'nuevo' | null

  const edita = puedeEditar(ui.rol)

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return cupones.filter((c) => !term
      || (c.codigo || '').toLowerCase().includes(term)
      || (c.descripcion || '').toLowerCase().includes(term))
  }, [cupones, q])

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar los cupones"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  if (editar) {
    return (
      <FormCupon cupon={editar === 'nuevo' ? null : editar}
        onVolver={() => setEditar(null)} onSaved={reload} toast={toast} />
    )
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por código o descripción…" value={q} onChange={(e) => setQ(e.target.value)} />
        {edita ? <Button className="ml-auto" icon={<Icon.Plus size={16} />} onClick={() => setEditar('nuevo')}>Nuevo cupón</Button> : null}
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || db.CUPONES === undefined ? (
          <div className="p-4"><TableSkeleton rows={5} cols={edita ? 6 : 5} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.Star size={22} />}
            title={q ? 'Sin resultados' : 'Aún no hay cupones'}
            body={q ? 'Prueba con otro término.' : 'Crea un código de descuento (porcentual o de monto fijo) para aplicarlo al cotizar o cobrar.'}
            cta={edita && !q ? <Button icon={<Icon.Plus size={16} />} onClick={() => setEditar('nuevo')}>Nuevo cupón</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 pr-3 font-medium">Código</th>
                  <th className="py-2.5 pr-3 font-medium">Descuento</th>
                  <th className="py-2.5 pr-3 font-medium">Vigencia</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Usos</th>
                  <th className="py-2.5 pr-3 font-medium">Estado</th>
                  {edita ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                </tr>
              </thead>
              <tbody>
                {rows.map((c) => {
                  const est = estadoCupon(c)
                  return (
                    <tr key={c.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover cursor-pointer" onClick={() => edita && setEditar(c)}>
                      <td className="py-2.5 pr-3">
                        <div className="font-medium text-[13px] num tracking-wide">{c.codigo}</div>
                        {c.descripcion ? <div className="text-[11px] text-slate-400">{c.descripcion}</div> : null}
                      </td>
                      <td className="py-2.5 pr-3">
                        <Badge size="sm" color={c.tipo === 'porcentaje' ? 'teal' : 'slate'}>{descuentoLabel(c)}</Badge>
                        {c.montoMinimo > 0 ? <div className="text-[11px] text-slate-400 mt-0.5">mín. {fmtCurrency(c.montoMinimo, 'VES')}</div> : null}
                      </td>
                      <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">{vigenciaLabel(c)}</td>
                      <td className="py-2.5 pr-3 text-right num text-[12.5px] text-slate-500">
                        {c.usosMax > 0 ? `${fmtNum(c.usosActuales || 0, 0)} / ${fmtNum(c.usosMax, 0)}` : fmtNum(c.usosActuales || 0, 0)}
                      </td>
                      <td className="py-2.5 pr-3"><Badge size="sm" color={est.color} dot={est.label === 'Vigente'}>{est.label}</Badge></td>
                      {edita ? (
                        <td className="py-2.5 pr-3 text-right" onClick={(e) => e.stopPropagation()}>
                          <Button variant="ghost" size="sm" icon={<Icon.Pencil size={14} />} onClick={() => setEditar(c)}>Editar</Button>
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
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} cupón(es)</div> : null}
    </div>
  )
}

/* Alta/edición de un cupón, a pantalla completa. */
function FormCupon({ cupon, onVolver, onSaved, toast }) {
  const editando = !!cupon
  const [codigo, setCodigo] = useState(cupon?.codigo || '')
  const [descripcion, setDescripcion] = useState(cupon?.descripcion || '')
  const [tipo, setTipo] = useState(cupon?.tipo || 'porcentaje')
  const [valor, setValor] = useState(cupon?.valor != null ? String(cupon.valor) : '')
  const [montoMinimo, setMontoMinimo] = useState(cupon?.montoMinimo ? String(cupon.montoMinimo) : '')
  const [desde, setDesde] = useState(cupon?.desde || '')
  const [hasta, setHasta] = useState(cupon?.hasta || '')
  const [activo, setActivo] = useState(cupon ? cupon.activo !== false : true)
  const [usosMax, setUsosMax] = useState(cupon?.usosMax ? String(cupon.usosMax) : '')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  const esPorc = tipo === 'porcentaje'
  const valorNum = Number(valor)

  const errCodigo = !codigo.trim() ? 'Ingresa un código para el cupón.' : ''
  const errValor = !(valorNum > 0) ? 'El valor debe ser mayor a 0.'
    : (esPorc && valorNum > 100) ? 'Un cupón porcentual no puede superar 100%.'
    : ''
  const errFechas = desde && hasta && hasta < desde ? 'La fecha «hasta» no puede ser anterior a «desde».' : ''
  const err = errCodigo || errValor || errFechas

  const guardar = async () => {
    setTouched(true)
    if (err) return
    setBusy(true)
    const body = {
      codigo: codigo.trim(),
      descripcion: descripcion.trim(),
      tipo,
      valor: Number(valor) || 0,
      montoMinimo: Number(montoMinimo) || 0,
      desde: desde || '',
      hasta: hasta || '',
      activo,
      usosMax: Number(usosMax) || 0,
    }
    try {
      if (editando) {
        await api.actualizarCupon(cupon.id, body)
        toast({ title: 'Cupón actualizado', body: body.codigo })
      } else {
        await api.crearCupon(body)
        toast({ title: 'Cupón creado', body: body.codigo })
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
      <Button onClick={guardar} loading={busy} disabled={!!err} title={err || 'Guardar el cupón'} icon={<Icon.Check size={16} />}>
        {editando ? 'Guardar cambios' : 'Crear cupón'}
      </Button>
    </>
  )

  const dateInput = 'w-full h-9 px-3 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm ring-focus'

  return (
    <VistaDetalle onVolver={onVolver} icon={<Icon.Star size={18} />}
      titulo={editando ? (cupon.codigo || 'Cupón') : 'Nuevo cupón'}
      sub="Un código de descuento que se aplica al cotizar o cobrar. El impuesto se recalcula sobre la base ya descontada." acciones={acciones}>

      {/* Identidad y descuento */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 mb-4">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Código" required error={touched ? errCodigo : ''} hint="se guarda en MAYÚSCULAS">
            <Input value={codigo} onChange={(e) => setCodigo(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!errCodigo}
              placeholder="Ej: BIENVENIDO10" className="num tracking-wide uppercase" autoFocus />
          </Field>
          <div className="md:col-span-2">
            <Field label="Descripción" hint="opcional">
              <Input value={descripcion} onChange={(e) => setDescripcion(e.target.value)} placeholder="Ej: 10% de descuento de bienvenida" />
            </Field>
          </div>
          <Field label="Tipo de descuento">
            <Select value={tipo} onChange={(e) => setTipo(e.target.value)}>
              <option value="porcentaje">Porcentaje (%)</option>
              <option value="monto">Monto fijo (Bs)</option>
            </Select>
          </Field>
          <Field label={esPorc ? 'Porcentaje' : 'Monto (Bs)'} required error={touched ? errValor : ''}
            hint={esPorc ? 'baja cada precio ese %' : 'se reparte entre las líneas'}>
            <input type="number" min="0" step="0.01" max={esPorc ? '100' : undefined} value={valor}
              onChange={(e) => setValor(e.target.value)} onBlur={() => setTouched(true)}
              placeholder={esPorc ? '10' : '50,00'}
              className={`${dateInput} text-right num ${touched && errValor ? 'border-red-400 dark:border-red-500' : ''}`} />
          </Field>
          <Field label="Monto mínimo de compra" hint="opcional · 0 = sin mínimo">
            <input type="number" min="0" step="0.01" value={montoMinimo} onChange={(e) => setMontoMinimo(e.target.value)}
              placeholder="0,00" className={`${dateInput} text-right num`} />
          </Field>
        </div>
      </div>

      {/* Vigencia y límites */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Vigente desde" hint="opcional">
            <input type="date" value={desde} onChange={(e) => setDesde(e.target.value)} className={dateInput} />
          </Field>
          <Field label="Vigente hasta" hint="opcional" error={touched ? errFechas : ''}>
            <input type="date" value={hasta} onChange={(e) => setHasta(e.target.value)}
              className={`${dateInput} ${touched && errFechas ? 'border-red-400 dark:border-red-500' : ''}`} />
          </Field>
          <Field label="Usos máximos" hint="opcional · 0 = ilimitado">
            <input type="number" min="0" step="1" value={usosMax} onChange={(e) => setUsosMax(e.target.value)}
              placeholder="0" className={`${dateInput} text-right num`} />
          </Field>
        </div>
        <div className="mt-4 pt-4 border-t border-slate-100 dark:border-slate-800">
          <Toggle checked={activo} onChange={setActivo}
            label={activo ? 'Activo' : 'Inactivo'}
            sub={activo ? 'Disponible para aplicar al cotizar/cobrar.' : 'Desactivado: se conserva, no se puede aplicar.'} />
        </div>
        {editando && cupon.usosMax > 0 ? (
          <div className="mt-3 text-[11.5px] text-slate-400">Usos registrados: {fmtNum(cupon.usosActuales || 0, 0)} de {fmtNum(cupon.usosMax, 0)}.</div>
        ) : null}
      </div>

      {err && touched ? (
        <div className="mt-3 flex items-start gap-1.5 text-[12px] text-amber-700 dark:text-amber-400">
          <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{err}</span>
        </div>
      ) : null}
    </VistaDetalle>
  )
}
