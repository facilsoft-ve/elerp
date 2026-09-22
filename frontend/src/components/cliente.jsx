import { useState } from 'react'
import { Icon } from './Icon.jsx'
import { Button, Input, Select, Field, Modal } from './primitives.jsx'
import { TIPOS_DOCUMENTO, validarRIF } from '../lib/format.js'
import { api } from '../lib/api.js'

/* ALTA DE CLIENTE, compartida.
 *
 * Vivía dentro del punto de venta y la necesitan cuatro pantallas —incluido el
 * propio modal de cobro, que es donde más falta hace: con la facturación digital
 * encendida no se puede emitir sin cliente, y mandar al cajero a otra pantalla a
 * crearlo le costaba el cobro que ya tenía cargado.
 *
 * Importarla desde POS.jsx no era opción: el punto de venta importa el cobro, y
 * el cobro tendría que importar el punto de venta.
 */

export const puedeCrearCliente = (rol) => ['dueno', 'desarrollador', 'vendedor', 'cajero'].includes(rol)

export function NuevoClienteModal({ onClose, onSaved, toast }) {
  const [f, setF] = useState({ nombre: '', tipoDocumento: 'V', documento: '', telefono: '', direccion: '', email: '' })
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
    setTouched({ nombre: true, documento: true })
    if (!valid) return
    setBusy(true)
    try {
      const c = await api.crearCliente({ nombre: f.nombre.trim(), tipoDocumento: f.tipoDocumento, documento: f.documento.trim(), telefono: f.telefono.trim(), direccion: f.direccion.trim(), email: f.email.trim() })
      toast({ title: 'Cliente creado', body: f.nombre.trim() })
      await onSaved(c)
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo crear', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.User size={18} />} title="Nuevo cliente" sub="El documento es único por empresa."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={save} loading={busy} icon={<Icon.Check size={16} />}>Crear</Button>
      </>}>
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
    </Modal>
  )
}
