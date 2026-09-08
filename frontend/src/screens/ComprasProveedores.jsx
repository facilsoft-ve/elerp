import { useState, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Modal, Empty, TableSkeleton, useToast, Field, useConfirm } from '../components/primitives.jsx'
import { fmtNum } from '../lib/format.js'
import { validarRIF } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'

// Escritura sólo dueño/desarrollador; contadora lee (consulta/regulador).
const puedeGestionar = (rol) => ['dueno', 'desarrollador'].includes(rol)

/* Proveedores (Compras) — CRUD espejo de Clientes: lista + buscar + alta/edición
 * + baja reversible (Activo). Los inactivos se muestran atenuados con su badge. */
export function ComprasProveedores() {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const confirm = useConfirm()
  const proveedores = db.PROVEEDORES

  const [q, setQ] = useState('')
  const [editar, setEditar] = useState(null)   // proveedor a editar
  const [nuevo, setNuevo] = useState(false)
  const [busyId, setBusyId] = useState('')

  const gestiona = puedeGestionar(ui.rol)

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return (proveedores || []).filter((p) => {
      if (!term) return true
      return p.nombre.toLowerCase().includes(term)
        || (p.documento || '').toLowerCase().includes(term)
        || (p.email || '').toLowerCase().includes(term)
        || (p.telefono || '').toLowerCase().includes(term)
    })
  }, [proveedores, q])

  const toggleActivo = async (p) => {
    // Desactivar es destructivo (deja de estar disponible); reactivar no lo es.
    if (p.activo && !(await confirm({
      title: '¿Desactivar proveedor?',
      body: `«${p.nombre}» ya no aparecerá para nuevas órdenes de compra. Su historial se conserva y puedes reactivarlo cuando quieras.`,
      confirmLabel: 'Desactivar', tone: 'danger',
    }))) return
    setBusyId(p.id)
    try {
      if (p.activo) {
        await api.desactivarProveedor(p.id)
        toast({ title: 'Proveedor desactivado', body: `${p.nombre} ya no aparecerá para nuevas órdenes.` })
      } else {
        await api.actualizarProveedor(p.id, { activo: true })
        toast({ title: 'Proveedor reactivado', body: p.nombre })
      }
      await reload()
    } catch (e) {
      toast({ title: 'No se pudo actualizar', body: e?.message || 'Error', kind: 'error' })
    } finally {
      setBusyId('')
    }
  }

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar los proveedores"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por nombre, RIF, correo o teléfono…" value={q} onChange={(e) => setQ(e.target.value)} />
        {gestiona ? <Button className="ml-auto" icon={<Icon.Plus size={16} />} onClick={() => setNuevo(true)}>Nuevo proveedor</Button> : null}
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || proveedores === undefined ? (
          <div className="p-4"><TableSkeleton rows={7} cols={5} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.Truck size={22} />}
            title={q ? 'Sin resultados' : 'Aún no hay proveedores'}
            body={q ? 'Prueba con otro término.' : 'Registra tus proveedores para emitir órdenes de compra y recibir mercancía.'}
            cta={gestiona && !q ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNuevo(true)}>Nuevo proveedor</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 pr-3 font-medium">Proveedor</th>
                  <th className="py-2.5 pr-3 font-medium">RIF / Documento</th>
                  <th className="py-2.5 pr-3 font-medium">Contacto</th>
                  <th className="py-2.5 pr-3 font-medium text-center">Estado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((p) => (
                  <tr key={p.id} className={`border-b border-slate-100 dark:border-slate-800/70 row-hover ${p.activo ? '' : 'opacity-60'}`}>
                    <td className="py-2.5 pr-3">
                      <div className="font-medium text-[13px] truncate max-w-[220px]">{p.nombre}</div>
                      {p.direccion ? <div className="text-[11px] text-slate-400 truncate max-w-[220px]">{p.direccion}</div> : null}
                    </td>
                    <td className="py-2.5 pr-3"><span className="num text-[12.5px] text-slate-500">{p.documento || '—'}</span></td>
                    <td className="py-2.5 pr-3">
                      <div className="text-[12.5px] text-slate-500 truncate max-w-[200px]">{p.email || '—'}</div>
                      {p.telefono ? <div className="text-[11px] text-slate-400 num">{p.telefono}</div> : null}
                    </td>
                    <td className="py-2.5 pr-3 text-center">
                      {p.activo ? <Badge size="sm" color="emerald" dot>Activo</Badge> : <Badge size="sm" color="slate" dot>Inactivo</Badge>}
                    </td>
                    <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                      {gestiona ? (
                        <div className="inline-flex items-center gap-1">
                          <button onClick={() => setEditar(p)} title="Editar"
                            className="h-7 px-2 inline-flex items-center gap-1 rounded-lg text-[12px] text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800"><Icon.Pencil size={14} /> Editar</button>
                          <button onClick={() => toggleActivo(p)} disabled={busyId === p.id} title={p.activo ? 'Desactivar' : 'Reactivar'}
                            className={`h-7 px-2 inline-flex items-center gap-1 rounded-lg text-[12px] disabled:opacity-50 ${p.activo ? 'text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-900/20' : 'text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-900/20'}`}>
                            {busyId === p.id ? <span className="spin inline-flex"><Icon.Refresh size={13} /></span> : p.activo ? <Icon.EyeOff size={14} /> : <Icon.Check size={14} />}
                            {p.activo ? 'Desactivar' : 'Reactivar'}
                          </button>
                        </div>
                      ) : <span className="text-[11.5px] text-slate-400">Sólo lectura</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} proveedor(es)</div> : null}

      {(nuevo || editar) ? (
        <ProveedorModal proveedor={editar || null} onClose={() => { setNuevo(false); setEditar(null) }} onSaved={reload} toast={toast} />
      ) : null}
    </div>
  )
}

/* Alta / edición de proveedor. Documento = RIF (V/E/J/G); nombre y documento son
 * obligatorios y se validan inline. El resto es opcional. */
function ProveedorModal({ proveedor, onClose, onSaved, toast }) {
  const editando = !!proveedor
  const [f, setF] = useState({
    nombre: proveedor?.nombre || '',
    documento: proveedor?.documento || '',
    email: proveedor?.email || '',
    telefono: proveedor?.telefono || '',
    direccion: proveedor?.direccion || '',
  })
  const [touched, setTouched] = useState({})
  const [busy, setBusy] = useState(false)

  const rif = validarRIF(f.documento)
  const emailOk = !f.email.trim() || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(f.email.trim())
  const errs = {
    nombre: !f.nombre.trim() ? 'Ingresa el nombre o razón social.' : '',
    documento: !f.documento.trim() ? 'Ingresa el RIF o documento.' : !rif.valid ? rif.msg : '',
    email: emailOk ? '' : 'Correo con formato inválido.',
  }
  const valid = !errs.nombre && !errs.documento && !errs.email
  const set = (k) => (e) => setF((s) => ({ ...s, [k]: e.target.value }))
  const blur = (k) => () => setTouched((t) => ({ ...t, [k]: true }))

  const save = async () => {
    setTouched({ nombre: true, documento: true, email: true })
    if (!valid) return
    setBusy(true)
    const body = {
      nombre: f.nombre.trim(), documento: f.documento.trim(),
      email: f.email.trim(), telefono: f.telefono.trim(), direccion: f.direccion.trim(),
    }
    try {
      if (editando) {
        await api.actualizarProveedor(proveedor.id, body)
        toast({ title: 'Proveedor actualizado', body: body.nombre })
      } else {
        await api.crearProveedor(body)
        toast({ title: 'Proveedor creado', body: body.nombre })
      }
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Truck size={18} />}
      title={editando ? 'Editar proveedor' : 'Nuevo proveedor'} sub="El RIF/documento identifica al proveedor."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={save} loading={busy} icon={<Icon.Check size={16} />}>{editando ? 'Guardar cambios' : 'Crear'}</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre / Razón social" required error={touched.nombre ? errs.nombre : ''}>
          <Input value={f.nombre} onChange={set('nombre')} onBlur={blur('nombre')} invalid={touched.nombre && !!errs.nombre} autoFocus />
        </Field>
        <Field label="RIF / Documento" required error={touched.documento ? errs.documento : ''}>
          <Input value={f.documento} onChange={set('documento')} onBlur={blur('documento')} invalid={touched.documento && !!errs.documento} placeholder="J-12345678-9" />
        </Field>
        <Field label="Correo" hint="opcional" error={touched.email ? errs.email : ''}>
          <Input type="email" value={f.email} onChange={set('email')} onBlur={blur('email')} invalid={touched.email && !!errs.email} placeholder="compras@proveedor.com" />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Teléfono" hint="opcional"><Input value={f.telefono} onChange={set('telefono')} placeholder="0212-…" /></Field>
          <Field label="Dirección" hint="opcional"><Input value={f.direccion} onChange={set('direccion')} /></Field>
        </div>
      </div>
    </Modal>
  )
}
