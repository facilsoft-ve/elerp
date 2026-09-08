import { useEffect, useState } from 'react'
import { api, money } from '../lib/api.js'
import { Card, Button, Input, Field, Select, Badge, Modal, Empty, Spinner, useToast } from '../ui.jsx'

const INTERVALOS = [
  { id: 'mensual', label: 'Mensual' },
  { id: 'anual', label: 'Anual' },
  { id: 'unico', label: 'Pago único' },
]

export function Facturacion() {
  const toast = useToast()
  const [resumen, setResumen] = useState(null)
  const [planes, setPlanes] = useState(null)
  const [hubmy, setHubmy] = useState(false)
  const [editar, setEditar] = useState(null) // plan en edición o {} para nuevo

  const cargar = async () => {
    try {
      const [r, p] = await Promise.all([api.resumenFacturacion(), api.planes()])
      setResumen(r)
      setPlanes(p.planes || [])
      setHubmy(!!p.hubmyDisponible)
    } catch (e) { toast({ kind: 'err', title: 'No se pudo cargar', body: e.message }) }
  }
  useEffect(() => { cargar() }, [])

  if (!resumen || planes === null) return <div className="flex justify-center py-16 text-slate-500"><Spinner size={24} /></div>

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="font-display font-bold text-xl text-slate-100">Facturación</h1>
          <p className="text-[13px] text-slate-500">Planes comerciales y suscripciones de los clientes</p>
        </div>
        <Badge color={hubmy ? 'emerald' : 'slate'}>{hubmy ? 'Checkout Hubmy activo' : 'Cobro manual (Hubmy no configurado)'}</Badge>
      </div>

      {/* Métricas */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        <Metrica label="MRR estimado" valor={money(resumen.mrrCents, resumen.moneda || 'USD')} destacado />
        <Metrica label="Suscripciones activas" valor={resumen.activas} />
        <Metrica label="Total suscripciones" valor={resumen.suscripciones} />
        <Metrica label="Planes en catálogo" valor={planes.length} />
      </div>

      {(resumen.porEstado && Object.keys(resumen.porEstado).length > 0) ? (
        <div className="flex flex-wrap gap-2">
          {Object.entries(resumen.porEstado).map(([e, n]) => (
            <Badge key={e} color="slate">{e}: {n}</Badge>
          ))}
        </div>
      ) : null}

      {/* Catálogo de planes */}
      <Card className="overflow-hidden">
        <div className="flex items-center justify-between px-5 py-3.5 border-b border-slate-800">
          <h2 className="font-display font-bold text-[15px] text-slate-100">Catálogo de planes</h2>
          <Button size="sm" onClick={() => setEditar({})}>Nuevo plan</Button>
        </div>
        {planes.length === 0 ? (
          <Empty title="Sin planes" body="Creá el primer plan comercial para asignarlo a los clientes." cta={<Button size="sm" onClick={() => setEditar({})}>Nuevo plan</Button>} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11.5px] uppercase tracking-wide text-slate-500 border-b border-slate-800">
                  <th className="px-4 py-2.5 font-medium">Plan</th>
                  <th className="px-4 py-2.5 font-medium">Precio</th>
                  <th className="px-4 py-2.5 font-medium">Límites (fact/usr/suc)</th>
                  <th className="px-4 py-2.5 font-medium">Hubmy</th>
                  <th className="px-4 py-2.5"></th>
                </tr>
              </thead>
              <tbody>
                {planes.map((p) => (
                  <tr key={p.id} className={`border-b border-slate-800/70 last:border-0 ${p.activo ? '' : 'opacity-50'}`}>
                    <td className="px-4 py-3">
                      <div className="font-medium text-slate-100">{p.nombre}{!p.activo ? ' (archivado)' : ''}</div>
                      {p.descripcion ? <div className="text-[12px] text-slate-500">{p.descripcion}</div> : null}
                    </td>
                    <td className="px-4 py-3 text-slate-200 tabular-nums">{money(p.precioCents, p.moneda)}<span className="text-slate-500 text-[12px]">/{p.intervalo}</span></td>
                    <td className="px-4 py-3 text-slate-400 tabular-nums">{lim(p.limites)}</td>
                    <td className="px-4 py-3">{p.hubmyPackageId ? <Badge color="teal">{p.hubmyPackageId}</Badge> : <span className="text-slate-600">—</span>}</td>
                    <td className="px-4 py-3 text-right"><Button size="sm" variant="ghost" onClick={() => setEditar(p)}>Editar</Button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {editar !== null ? (
        <PlanModal plan={editar} onClose={() => setEditar(null)} onGuardado={() => { setEditar(null); cargar() }} />
      ) : null}
    </div>
  )
}

const lim = (l) => `${l?.facturasMes || '∞'} / ${l?.usuarios || '∞'} / ${l?.sucursales || '∞'}`.replace(/\b0\b/g, '∞')

function Metrica({ label, valor, destacado }) {
  return (
    <Card className={`p-4 ${destacado ? 'ring-1 ring-teal-500/30' : ''}`}>
      <div className="text-[11.5px] uppercase tracking-wide text-slate-500">{label}</div>
      <div className={`mt-1 font-display font-bold ${destacado ? 'text-teal-300 text-2xl' : 'text-slate-100 text-xl'} tabular-nums`}>{valor}</div>
    </Card>
  )
}

function PlanModal({ plan, onClose, onGuardado }) {
  const toast = useToast()
  const esNuevo = !plan.id
  const [f, setF] = useState({
    nombre: plan.nombre || '',
    descripcion: plan.descripcion || '',
    precio: plan.precioCents ? (plan.precioCents / 100).toString() : '',
    moneda: plan.moneda || 'USD',
    intervalo: plan.intervalo || 'mensual',
    facturasMes: plan.limites?.facturasMes || 0,
    usuarios: plan.limites?.usuarios || 0,
    sucursales: plan.limites?.sucursales || 0,
    hubmyPackageId: plan.hubmyPackageId || '',
  })
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))

  const guardar = async () => {
    if (!f.nombre.trim()) { toast({ kind: 'err', title: 'Falta el nombre' }); return }
    setBusy(true)
    const body = {
      nombre: f.nombre, descripcion: f.descripcion,
      precioCents: Math.round((Number(f.precio) || 0) * 100),
      moneda: f.moneda, intervalo: f.intervalo,
      limites: { facturasMes: Number(f.facturasMes) || 0, usuarios: Number(f.usuarios) || 0, sucursales: Number(f.sucursales) || 0 },
      hubmyPackageId: f.hubmyPackageId.trim(),
    }
    try {
      if (esNuevo) await api.crearPlan(body); else await api.editarPlan(plan.id, body)
      toast({ title: esNuevo ? 'Plan creado' : 'Plan actualizado' })
      onGuardado()
    } catch (e) { toast({ kind: 'err', title: 'No se pudo', body: e.message }) } finally { setBusy(false) }
  }

  const archivar = async () => {
    setBusy(true)
    try { await api.archivarPlan(plan.id); toast({ title: 'Plan archivado' }); onGuardado() }
    catch (e) { toast({ kind: 'err', title: 'No se pudo', body: e.message }) } finally { setBusy(false) }
  }

  return (
    <Modal open onClose={onClose} title={esNuevo ? 'Nuevo plan' : 'Editar plan'}
      footer={<>
        {!esNuevo && plan.activo ? <Button variant="danger" size="sm" onClick={archivar} loading={busy}>Archivar</Button> : null}
        <Button variant="ghost" size="sm" onClick={onClose}>Cancelar</Button>
        <Button size="sm" onClick={guardar} loading={busy}>Guardar</Button>
      </>}>
      <div className="space-y-3">
        <Field label="Nombre" required><Input value={f.nombre} onChange={(e) => set('nombre', e.target.value)} placeholder="Pro, Enterprise…" /></Field>
        <Field label="Descripción"><Input value={f.descripcion} onChange={(e) => set('descripcion', e.target.value)} /></Field>
        <div className="grid grid-cols-3 gap-2">
          <Field label="Precio"><Input type="number" min="0" step="0.01" value={f.precio} onChange={(e) => set('precio', e.target.value)} /></Field>
          <Field label="Moneda"><Select value={f.moneda} onChange={(e) => set('moneda', e.target.value)}><option>USD</option><option>VES</option></Select></Field>
          <Field label="Intervalo"><Select value={f.intervalo} onChange={(e) => set('intervalo', e.target.value)}>{INTERVALOS.map((i) => <option key={i.id} value={i.id}>{i.label}</option>)}</Select></Field>
        </div>
        <div className="grid grid-cols-3 gap-2">
          <Field label="Facturas/mes" hint="0 = ∞"><Input type="number" min="0" value={f.facturasMes} onChange={(e) => set('facturasMes', e.target.value)} /></Field>
          <Field label="Usuarios" hint="0 = ∞"><Input type="number" min="0" value={f.usuarios} onChange={(e) => set('usuarios', e.target.value)} /></Field>
          <Field label="Sucursales" hint="0 = ∞"><Input type="number" min="0" value={f.sucursales} onChange={(e) => set('sucursales', e.target.value)} /></Field>
        </div>
        <Field label="Package de Hubmy (opcional)" hint="para checkout con Stripe; vacío = cobro manual">
          <Input value={f.hubmyPackageId} onChange={(e) => set('hubmyPackageId', e.target.value)} placeholder="pkg_01H…" />
        </Field>
      </div>
    </Modal>
  )
}
