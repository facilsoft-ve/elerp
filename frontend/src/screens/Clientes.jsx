import { useState, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, Toggle, Field, Modal, Empty, TableSkeleton, useToast } from '../components/primitives.jsx'
import { fmtNum, TIPOS_DOCUMENTO, validarRIF } from '../lib/format.js'
import { api } from '../lib/api.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { NuevoClienteModal, puedeCrearCliente } from './POS.jsx'

// Clientes (CRM). Documento único por empresa (V/E/J/G/P). El alta reusa el
// mismo modal del POS para mantener una sola validación; la edición usa el panel
// EditarClienteModal con la INFORMACIÓN AMPLIADA del cliente. El maestro es
// EDITABLE (no es un ledger append-only como los documentos fiscales).

// Etiqueta legible del origen/procedencia de un cliente (interop con otros CRM).
const ORIGEN_LABEL = { manual: 'Cargado en ElERP', odoo: 'Odoo', importado: 'Importado' }
const origenLabel = (o) => ORIGEN_LABEL[o] || (o ? o : 'Cargado en ElERP')
// Un cliente "importado" trae su procedencia de otro sistema (no es manual).
const esImportado = (c) => !!(c.origen && c.origen !== 'manual')

export function Clientes() {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const clientes = db.CLIENTES

  const [q, setQ] = useState('')
  const [nuevo, setNuevo] = useState(false)
  const [editar, setEditar] = useState(null) // cliente en edición | null

  // Mismos roles que crear: el maestro de clientes es editable (no ledger).
  const puedeEditar = puedeCrearCliente(ui.rol)

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return (clientes || []).filter((c) => {
      if (!term) return true
      return c.nombre.toLowerCase().includes(term) || (c.documento || '').toLowerCase().includes(term) || (c.telefono || '').toLowerCase().includes(term)
    })
  }, [clientes, q])

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar los clientes"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por nombre, documento o teléfono…" value={q} onChange={(e) => setQ(e.target.value)} />
        {puedeEditar ? <Button className="ml-auto" icon={<Icon.Plus size={16} />} onClick={() => setNuevo(true)}>Nuevo cliente</Button> : null}
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || clientes === undefined ? (
          <div className="p-4"><TableSkeleton rows={7} cols={puedeEditar ? 5 : 4} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.Users size={22} />}
            title={q ? 'Sin resultados' : 'Aún no hay clientes'}
            body={q ? 'Prueba con otro término.' : 'Carga tus clientes para facturar con sus datos fiscales.'}
            cta={puedeEditar && !q ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNuevo(true)}>Nuevo cliente</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 pr-3 font-medium">Cliente</th>
                  <th className="py-2.5 pr-3 font-medium">Documento</th>
                  <th className="py-2.5 pr-3 font-medium">Teléfono</th>
                  <th className="py-2.5 pr-3 font-medium">Dirección</th>
                  {puedeEditar ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                </tr>
              </thead>
              <tbody>
                {rows.map((c) => (
                  <tr key={c.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
                    <td className="py-2.5 pr-3 font-medium text-[13px]">
                      <span className="inline-flex items-center gap-2">
                        {c.nombre}
                        {c.activo === false ? <Badge size="sm" color="slate">Inactivo</Badge> : null}
                        {esImportado(c) ? <Badge size="sm" color="teal">{origenLabel(c.origen)}</Badge> : null}
                      </span>
                    </td>
                    <td className="py-2.5 pr-3"><Badge size="sm" color="slate">{c.tipoDocumento}</Badge> <span className="num text-[12.5px] text-slate-500">{c.documento}</span></td>
                    <td className="py-2.5 pr-3 text-[12.5px] text-slate-500 num">{c.telefono || '—'}</td>
                    <td className="py-2.5 pr-3 text-[12.5px] text-slate-500 truncate max-w-[240px]">{c.direccion || '—'}</td>
                    {puedeEditar ? (
                      <td className="py-2.5 pr-3 text-right">
                        <Button variant="ghost" size="sm" icon={<Icon.Pencil size={14} />} onClick={() => setEditar(c)}>Editar</Button>
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} cliente(s)</div> : null}

      {nuevo ? <NuevoClienteModal onClose={() => setNuevo(false)} onSaved={reload} toast={toast} /> : null}
      {editar ? <EditarClienteModal cliente={editar} onClose={() => setEditar(null)} onSaved={reload} toast={toast} /> : null}
    </div>
  )
}

// EditarClienteModal — panel de edición con la INFORMACIÓN AMPLIADA del cliente.
// Todos los datos fiscales/de contacto son editables; el bloque de
// interconexión/CRM (origen + sistema externo + id externo) es de solo lectura:
// preserva la procedencia para importar/sincronizar desde Odoo u otro CRM sin
// perder la trazabilidad. Guardar → PATCH → reload() del DataContext.
export function EditarClienteModal({ cliente, onClose, onSaved, toast }) {
  const importado = esImportado(cliente)
  const [f, setF] = useState({
    nombre: cliente.nombre || '',
    tipoDocumento: cliente.tipoDocumento || 'V',
    documento: cliente.documento || '',
    telefono: cliente.telefono || '',
    direccion: cliente.direccion || '',
    email: cliente.email || '',
    nombreComercial: cliente.nombreComercial || '',
    contacto: cliente.contacto || '',
    notas: cliente.notas || '',
    activo: cliente.activo !== false, // legado sin campo ⇒ activo
  })
  const [touched, setTouched] = useState({})
  const [busy, setBusy] = useState(false)

  // Pasaporte (P) es texto libre; V/E/J/G validan como RIF/cédula.
  const rifCheck = f.tipoDocumento === 'P' ? { valid: true, msg: '' } : validarRIF(`${f.tipoDocumento}${f.documento}`)
  const emailOk = !f.email.trim() || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(f.email.trim())
  const errs = {
    nombre: !f.nombre.trim() ? 'Ingresa el nombre.' : '',
    documento: !f.documento.trim() ? 'Ingresa el documento.' : !rifCheck.valid ? rifCheck.msg : '',
    email: emailOk ? '' : 'El correo no tiene un formato válido.',
  }
  const valid = !errs.nombre && !errs.documento && !errs.email
  const set = (k) => (e) => setF((s) => ({ ...s, [k]: e.target.value }))

  const save = async () => {
    setTouched({ nombre: true, documento: true, email: true })
    if (!valid) return
    setBusy(true)
    try {
      await api.actualizarCliente(cliente.id, {
        nombre: f.nombre.trim(), tipoDocumento: f.tipoDocumento, documento: f.documento.trim(),
        telefono: f.telefono.trim(), direccion: f.direccion.trim(), email: f.email.trim(),
        nombreComercial: f.nombreComercial.trim(), contacto: f.contacto.trim(), notas: f.notas.trim(),
        activo: f.activo,
      })
      toast({ title: 'Cliente actualizado', body: f.nombre.trim() })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="lg" icon={<Icon.User size={18} />} title="Editar cliente" sub="El documento es único por empresa."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={save} loading={busy} icon={<Icon.Check size={16} />}>Guardar cambios</Button>
      </>}>
      <div className="space-y-5">
        {/* Datos fiscales / identificación */}
        <div className="space-y-3.5">
          <Field label="Nombre / Razón social" required error={touched.nombre ? errs.nombre : ''}>
            <Input value={f.nombre} onChange={set('nombre')} onBlur={() => setTouched((t) => ({ ...t, nombre: true }))} invalid={touched.nombre && !!errs.nombre} autoFocus />
          </Field>
          <div className="grid grid-cols-[80px_1fr] gap-2">
            <Field label="Tipo">
              <Select value={f.tipoDocumento} onChange={set('tipoDocumento')}>
                {TIPOS_DOCUMENTO.map((t) => <option key={t} value={t}>{t}</option>)}
              </Select>
            </Field>
            <Field label="Documento" required error={touched.documento ? errs.documento : ''}>
              <Input value={f.documento} onChange={set('documento')} onBlur={() => setTouched((t) => ({ ...t, documento: true }))} invalid={touched.documento && !!errs.documento} placeholder="12345678" />
            </Field>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Teléfono" hint="opcional"><Input value={f.telefono} onChange={set('telefono')} placeholder="0412-…" /></Field>
            <Field label="Correo" hint="opcional" error={touched.email ? errs.email : ''}>
              <Input value={f.email} onChange={set('email')} onBlur={() => setTouched((t) => ({ ...t, email: true }))} invalid={touched.email && !!errs.email} type="email" placeholder="correo@dominio.com" />
            </Field>
          </div>
          <Field label="Dirección" hint="opcional"><Input value={f.direccion} onChange={set('direccion')} /></Field>
        </div>

        {/* Información ampliada (CRM) */}
        <div className="pt-4 border-t border-slate-100 dark:border-slate-800 space-y-3.5">
          <div className="text-[11px] uppercase tracking-wide text-slate-400 font-medium">Información ampliada</div>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Nombre comercial" hint="opcional"><Input value={f.nombreComercial} onChange={set('nombreComercial')} placeholder="Nombre de fantasía" /></Field>
            <Field label="Persona de contacto" hint="opcional"><Input value={f.contacto} onChange={set('contacto')} placeholder="Nombre y apellido" /></Field>
          </div>
          <Field label="Notas" hint="opcional">
            <textarea value={f.notas} onChange={set('notas')} rows={3}
              className="w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm text-slate-900 dark:text-slate-100 placeholder:text-slate-400 dark:placeholder:text-slate-500 focus:border-elerp-500 ring-focus resize-y"
              placeholder="Observaciones internas del cliente…" />
          </Field>
          <Toggle checked={f.activo} onChange={(v) => setF((s) => ({ ...s, activo: v }))}
            label={f.activo ? 'Activo' : 'Inactivo'}
            sub={f.activo ? 'El cliente está disponible para facturar.' : 'Desactivado: se conserva el histórico, no se borra.'} />
        </div>

        {/* Interconexión / CRM (procedencia, solo lectura) */}
        <div className="pt-4 border-t border-slate-100 dark:border-slate-800 space-y-3">
          <div className="flex items-center gap-2 text-[11px] uppercase tracking-wide text-slate-400 font-medium">
            <Icon.Link size={13} /> Interconexión / CRM
          </div>
          <div className="grid grid-cols-3 gap-3">
            <Field label="Origen">
              <div className="h-9 flex items-center"><Badge size="sm" color={importado ? 'teal' : 'slate'}>{origenLabel(cliente.origen)}</Badge></div>
            </Field>
            <Field label="Sistema externo">
              <Input value={cliente.sistemaExterno || '—'} readOnly disabled />
            </Field>
            <Field label="ID externo">
              <Input value={cliente.idExterno || '—'} readOnly disabled />
            </Field>
          </div>
          <p className="text-[11.5px] text-slate-400 leading-relaxed">
            La información de clientes puede provenir de o sincronizarse con otros sistemas (Odoo u otro CRM/ERP).
            Estos datos de procedencia se conservan para no perder la trazabilidad y no se editan aquí.
          </p>
        </div>
      </div>
    </Modal>
  )
}
