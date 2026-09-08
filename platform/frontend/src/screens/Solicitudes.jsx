import { useEffect, useMemo, useState } from 'react'
import { api } from '../lib/api.js'
import { Card, Input, Badge, Spinner, Empty, Button, Modal, Field, Select, useToast } from '../ui.jsx'

// Bandeja de SOLICITUDES DE DEMO del formulario de elerp.tech. Es el embudo comercial de
// Mornix: prospectos, no tenants. Se guardan en la base de la consola (no en el core).

const norm = (s) => (s || '').toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '')

const ESTADO_LABEL = {
  nuevo: 'Nueva',
  contactado: 'Contactada',
  demo_creada: 'Demo creada',
  ganado: 'Ganado',
  descartado: 'Descartada',
}
const ESTADO_COLOR = {
  nuevo: 'teal',
  contactado: 'amber',
  demo_creada: 'slate',
  ganado: 'emerald',
  descartado: 'red',
}
const GIRO_LABEL = {
  bodega: 'Bodega / abasto',
  ferreteria: 'Ferretería',
  farmacia: 'Farmacia',
  restaurante: 'Restaurante',
  otro: 'Otro',
}

// Fecha corta en hora local; el backend guarda RFC3339 UTC.
function fecha(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d)) return iso
  return d.toLocaleDateString('es-VE', { day: '2-digit', month: 'short' }) + ' ' +
    d.toLocaleTimeString('es-VE', { hour: '2-digit', minute: '2-digit' })
}

export function Solicitudes() {
  const [leads, setLeads] = useState(null)
  const [estados, setEstados] = useState([])
  const [error, setError] = useState('')
  const [q, setQ] = useState('')
  const [filtro, setFiltro] = useState('')
  const [abierta, setAbierta] = useState(null)

  const [demosPorGiro, setDemosPorGiro] = useState({})

  const cargar = () => {
    api.leads()
      .then((r) => { setLeads(r.leads || []); setEstados(r.estados || []) })
      .catch((e) => setError(e.message || 'No se pudieron cargar las solicitudes'))
  }
  useEffect(cargar, [])

  // Empresa demo de cada rubro: es la PLANTILLA que se clona para el prospecto. Se
  // resuelve por `giro`, no por IDs fijos, así el día que cambien los datos sembrados
  // esto sigue funcionando.
  useEffect(() => {
    api.tenants()
      .then((r) => {
        const porGiro = {}
        for (const org of r.tenants || []) {
          for (const emp of org.empresas || []) {
            if (emp.giro && !emp.sandbox && !porGiro[emp.giro]) porGiro[emp.giro] = emp
          }
        }
        setDemosPorGiro(porGiro)
      })
      .catch(() => setDemosPorGiro({}))
  }, [])

  const filas = useMemo(() => {
    let rows = leads || []
    if (filtro) rows = rows.filter((l) => l.estado === filtro)
    if (q.trim()) {
      const nq = norm(q)
      rows = rows.filter((l) => norm(l.nombre).includes(nq) || norm(l.empresa).includes(nq) ||
        norm(l.email).includes(nq) || norm(l.telefono).includes(nq))
    }
    return rows
  }, [leads, filtro, q])

  const nuevas = (leads || []).filter((l) => l.estado === 'nuevo').length

  if (error) return <Empty title="No se pudieron cargar las solicitudes" body={error} cta={<Button onClick={cargar}>Reintentar</Button>} />
  if (leads === null) return <div className="flex justify-center py-16 text-slate-500"><Spinner size={24} /></div>

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-4 flex-wrap">
        <div>
          <h1 className="font-display font-bold text-xl text-slate-100">Solicitudes de demo</h1>
          <p className="text-[13px] text-slate-500">
            {leads.length} en total{nuevas > 0 ? <> · <span className="text-teal-300">{nuevas} sin atender</span></> : null} · llegan del formulario de elerp.tech
          </p>
        </div>
        <div className="w-72 max-w-[60vw]">
          <Input placeholder="Buscar nombre, empresa, email…" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
      </div>

      <div className="flex items-center gap-1.5 mb-4 flex-wrap">
        <Chip activo={filtro === ''} onClick={() => setFiltro('')}>Todas</Chip>
        {estados.map((e) => (
          <Chip key={e} activo={filtro === e} onClick={() => setFiltro(e)}>{ESTADO_LABEL[e] || e}</Chip>
        ))}
      </div>

      {filas.length === 0 ? (
        <Empty
          title={leads.length === 0 ? 'Todavía no llegó ninguna solicitud' : 'Ninguna solicitud coincide'}
          body={leads.length === 0
            ? 'Cuando alguien complete el formulario de elerp.tech, aparece acá.'
            : 'Probá con otro texto o quitá el filtro de estado.'}
        />
      ) : (
        <Card className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11.5px] uppercase tracking-wide text-slate-500 border-b border-slate-800">
                  <th className="px-4 py-2.5 font-medium">Recibida</th>
                  <th className="px-4 py-2.5 font-medium">Prospecto</th>
                  <th className="px-4 py-2.5 font-medium">Contacto</th>
                  <th className="px-4 py-2.5 font-medium">Rubro</th>
                  <th className="px-4 py-2.5 font-medium">Estado</th>
                  <th className="px-4 py-2.5"></th>
                </tr>
              </thead>
              <tbody>
                {filas.map((l) => (
                  <tr key={l.id} className="border-b border-slate-800/60 last:border-0 hover:bg-slate-800/30">
                    <td className="px-4 py-2.5 text-slate-400 whitespace-nowrap tabular-nums">{fecha(l.creado)}</td>
                    <td className="px-4 py-2.5">
                      <div className="text-slate-100 font-medium">{l.empresa || l.nombre}</div>
                      {l.empresa ? <div className="text-[12px] text-slate-500">{l.nombre}</div> : null}
                    </td>
                    <td className="px-4 py-2.5">
                      <a className="text-teal-300 hover:underline" href={`mailto:${l.email}`}>{l.email}</a>
                      {l.telefono ? <div className="text-[12px] text-slate-500 tabular-nums">{l.telefono}</div> : null}
                    </td>
                    <td className="px-4 py-2.5 text-slate-400">{GIRO_LABEL[l.giro] || '—'}</td>
                    <td className="px-4 py-2.5">
                      <Badge color={ESTADO_COLOR[l.estado]}>{ESTADO_LABEL[l.estado] || l.estado}</Badge>
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <Button size="sm" variant="ghost" onClick={() => setAbierta(l)}>Ver</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      {abierta ? (
        <DetalleSolicitud
          lead={abierta}
          estados={estados}
          plantilla={demosPorGiro[abierta.giro]}
          onClose={() => setAbierta(null)}
          onGuardado={(l) => {
            setLeads((prev) => prev.map((x) => (x.id === l.id ? l : x)))
            setAbierta(null)
          }}
        />
      ) : null}
    </div>
  )
}

function Chip({ activo, onClick, children }) {
  return (
    <button onClick={onClick}
      className={`h-7 px-3 rounded-full text-[12.5px] font-medium border ring-focus transition-colors ${
        activo ? 'bg-slate-800 text-slate-100 border-slate-600' : 'text-slate-400 border-slate-800 hover:text-slate-200 hover:bg-slate-800/50'}`}>
      {children}
    </button>
  )
}

function DetalleSolicitud({ lead, estados, plantilla, onClose, onGuardado }) {
  const toast = useToast()
  const [estado, setEstado] = useState(lead.estado)
  const [notas, setNotas] = useState(lead.notas || '')
  const [empresaDemoId, setEmpresaDemoId] = useState(lead.empresaDemoId || '')
  const [guardando, setGuardando] = useState(false)
  const [creando, setCreando] = useState(false)

  // Abre una demo para ESTE prospecto: clona la empresa demo de su rubro como SANDBOX
  // (maestros y configuración copiados, ledgers vacíos, con fecha de caducidad). Así el
  // prospecto trabaja sobre datos de su rubro sin ensuciar la demo pública ni ver lo que
  // hizo otro.
  const crearDemo = async () => {
    if (!plantilla) return
    setCreando(true)
    try {
      const r = await api.crearSandbox(plantilla.id, 14)
      const nueva = r?.sandbox?.id || ''
      const actualizado = await api.actualizarLead(lead.id, {
        estado: 'demo_creada',
        empresaDemoId: nueva,
        notas: notas,
      })
      toast(`Demo creada: ${r?.sandbox?.nombre || nueva}`)
      onGuardado(actualizado)
    } catch (e) {
      toast(e.message || 'No se pudo crear la demo', 'error')
    } finally {
      setCreando(false)
    }
  }

  const guardar = async () => {
    setGuardando(true)
    try {
      const actualizado = await api.actualizarLead(lead.id, { estado, notas, empresaDemoId })
      toast('Solicitud actualizada')
      onGuardado(actualizado)
    } catch (e) {
      toast(e.message || 'No se pudo guardar', 'error')
    } finally {
      setGuardando(false)
    }
  }

  return (
    <Modal open onClose={onClose} title={lead.empresa || lead.nombre} width="max-w-2xl"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cerrar</Button>
        {plantilla && !empresaDemoId ? (
          <Button variant="navy" onClick={crearDemo} loading={creando}>
            Crear demo de {GIRO_LABEL[lead.giro] || 'su rubro'}
          </Button>
        ) : null}
        <Button onClick={guardar} loading={guardando}>Guardar</Button>
      </>}>
      <div className="space-y-4">
        <div className="grid sm:grid-cols-2 gap-x-6 gap-y-3 text-sm">
          <Dato label="Nombre" valor={lead.nombre} />
          <Dato label="Empresa" valor={lead.empresa} />
          <Dato label="Email" valor={<a className="text-teal-300 hover:underline" href={`mailto:${lead.email}`}>{lead.email}</a>} />
          <Dato label="Teléfono" valor={lead.telefono} />
          <Dato label="Rubro" valor={GIRO_LABEL[lead.giro]} />
          <Dato label="Recibida" valor={fecha(lead.creado)} />
        </div>

        {lead.mensaje ? (
          <div>
            <div className="text-[11.5px] uppercase tracking-wide text-slate-500 mb-1">Qué le gustaría ver</div>
            <p className="text-sm text-slate-300 whitespace-pre-wrap bg-slate-800/40 border border-slate-800 rounded-lg p-3">{lead.mensaje}</p>
          </div>
        ) : null}

        <div className="grid sm:grid-cols-2 gap-4">
          <Field label="Estado">
            <Select value={estado} onChange={(e) => setEstado(e.target.value)}>
              {estados.map((e) => <option key={e} value={e}>{ESTADO_LABEL[e] || e}</option>)}
            </Select>
          </Field>
          <Field label="Empresa de demo"
            hint={empresaDemoId
              ? 'Sandbox abierto para este prospecto'
              : plantilla
                ? `Se clonará «${plantilla.nombre}» como sandbox de 14 días`
                : 'Sin rubro elegido (o sin demo de ese rubro): cargá el ID a mano'}>
            <Input value={empresaDemoId} onChange={(e) => setEmpresaDemoId(e.target.value)} placeholder="emp_…" />
          </Field>
        </div>

        <Field label="Notas del equipo">
          <textarea value={notas} onChange={(e) => setNotas(e.target.value)} rows={4}
            placeholder="Qué se conversó, cuándo volver a llamar…"
            className="w-full bg-slate-800/60 text-slate-100 placeholder:text-slate-500 rounded-xl px-3 py-2 text-sm
              border border-slate-700 focus:border-teal-500 ring-focus transition-colors resize-y" />
        </Field>

        {/* Trazabilidad de dónde vino: útil para detectar abuso del formulario público. */}
        <p className="text-[11.5px] text-slate-600">
          Origen: {lead.origen || '—'}{lead.ip ? ` · IP ${lead.ip}` : ''}
        </p>
      </div>
    </Modal>
  )
}

function Dato({ label, valor }) {
  return (
    <div>
      <div className="text-[11.5px] uppercase tracking-wide text-slate-500">{label}</div>
      <div className="text-slate-200">{valor || '—'}</div>
    </div>
  )
}
