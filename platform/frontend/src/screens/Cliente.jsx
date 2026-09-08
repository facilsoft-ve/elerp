import { useCallback, useEffect, useState } from 'react'
import { api, descargarExport, money, ESTADO_SUB } from '../lib/api.js'
import { Card, Button, Input, Field, Select, Badge, Toggle, Spinner, Empty, useToast } from '../ui.jsx'

function Seccion({ titulo, sub, children, aside }) {
  return (
    <Card className="p-5">
      <div className="flex items-start justify-between gap-4 mb-4">
        <div>
          <h2 className="font-display font-bold text-[15px] text-slate-100">{titulo}</h2>
          {sub ? <p className="text-[12.5px] text-slate-500 mt-0.5">{sub}</p> : null}
        </div>
        {aside}
      </div>
      {children}
    </Card>
  )
}

// Detalle de una CORPORACIÓN (organización) = el cliente que paga la licencia. Arriba, lo
// comercial (suscripción, plan, estado) a nivel corporación; abajo, sus EMPRESAS (tenants)
// con las operaciones por empresa (activar, sandbox, respaldo, bitácora).
export function Cliente({ orgId, volver }) {
  const toast = useToast()
  const [org, setOrg] = useState(null)
  const [uso, setUso] = useState(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const cargar = useCallback(async () => {
    try {
      const r = await api.tenants()
      const o = (r.tenants || []).find((x) => x.id === orgId)
      if (!o) { setError('No se encontró la corporación'); return }
      setOrg(o)
      try { setUso(await api.uso(orgId)) } catch { setUso(null) }
    } catch (e) { setError(e.message || 'No se pudo cargar la corporación') }
  }, [orgId])
  useEffect(() => { cargar() }, [cargar])

  if (error) return <Empty title="No se pudo cargar" body={error} cta={<Button onClick={volver}>Volver</Button>} />
  if (!org) return <div className="flex justify-center py-16 text-slate-500"><Spinner size={24} /></div>

  const suspendida = org.estado === 'suspendida'
  const empresas = org.empresas || []
  const usuarios = empresas.reduce((a, e) => a + (e.usuarios || 0), 0)

  const toggleEstadoOrg = async () => {
    setBusy(true)
    try {
      await api.fijarEstadoOrg(org.id, suspendida ? 'activa' : 'suspendida')
      toast({ title: suspendida ? 'Corporación reactivada' : 'Corporación suspendida' })
      await cargar()
    } catch (e) { toast({ kind: 'err', title: 'No se pudo', body: e.message }) } finally { setBusy(false) }
  }

  return (
    <div className="space-y-4">
      <button onClick={volver} className="text-[13px] text-slate-500 hover:text-slate-300">← Clientes</button>

      <div className="flex flex-wrap items-center gap-3">
        <h1 className="font-display font-bold text-xl text-slate-100">{org.nombre}</h1>
        <Badge color="teal">{org.plan || 'base'}</Badge>
        {suspendida ? <Badge color="red">Suspendida</Badge> : <Badge color="emerald">Activa</Badge>}
      </div>
      <p className="text-[13px] text-slate-500 -mt-2">Corporación (cliente / unidad de pago) · {empresas.length} empresa(s) · {usuarios} usuario(s)</p>

      {/* Comercial — a nivel CORPORACIÓN (lo que se paga). */}
      <SuscripcionCorp orgId={org.id} onCambio={cargar} />

      <div className="grid md:grid-cols-2 gap-4">
        <PlanLimites org={org} uso={uso} onGuardado={cargar} />
        <Seccion titulo="Estado y acceso" sub="Suspender corta el acceso de TODA la corporación (todas sus empresas).">
          <div className="flex items-center justify-between gap-3">
            <div className="text-[13px] text-slate-300">
              Corporación <span className="text-slate-500">— {suspendida ? 'sin acceso' : 'con acceso'}</span>
            </div>
            <Button size="sm" variant={suspendida ? 'primary' : 'danger'} loading={busy} onClick={toggleEstadoOrg}>
              {suspendida ? 'Reactivar' : 'Suspender'}
            </Button>
          </div>
        </Seccion>
      </div>

      {/* Empresas (tenants) de la corporación. */}
      <Seccion titulo={`Empresas de la corporación (${empresas.length})`} sub="Cada empresa es un tenant aislado. Operaciones por empresa: activar, QA/sandbox, respaldo y bitácora.">
        {empresas.length === 0 ? (
          <Empty title="Sin empresas" body="Esta corporación todavía no tiene empresas." />
        ) : (
          <div className="space-y-2.5">
            {empresas.map((emp) => <EmpresaCard key={emp.id} emp={emp} onCambio={cargar} />)}
          </div>
        )}
      </Seccion>
    </div>
  )
}

// EmpresaCard es una empresa (tenant) dentro de la corporación, expandible a sus
// operaciones propias.
function EmpresaCard({ emp, onCambio }) {
  const toast = useToast()
  const [abierta, setAbierta] = useState(false)
  const [busy, setBusy] = useState(false)

  const toggleActiva = async () => {
    setBusy(true)
    try {
      await api.fijarEmpresaActiva(emp.id, !emp.activa)
      toast({ title: emp.activa ? 'Empresa desactivada' : 'Empresa activada' })
      await onCambio()
    } catch (e) { toast({ kind: 'err', title: 'No se pudo', body: e.message }) } finally { setBusy(false) }
  }

  return (
    <div className="rounded-xl border border-slate-800 bg-slate-950/40">
      <button onClick={() => setAbierta((v) => !v)} className="w-full flex items-center gap-3 px-4 py-3 text-left ring-focus rounded-xl">
        <span className="text-slate-500">{abierta ? '▾' : '▸'}</span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="font-medium text-slate-100">{emp.nombre}</span>
            {emp.sandbox ? <Badge color="amber">Sandbox{emp.expiraEl ? ` · vence ${emp.expiraEl.slice(0, 10)}` : ''}</Badge> : null}
            {!emp.activa ? <Badge color="amber">Inactiva</Badge> : null}
          </div>
          <div className="text-[12px] text-slate-500 font-mono">{emp.rif}</div>
        </div>
        <div className="text-[12px] text-slate-500 tabular-nums hidden sm:block">{(emp.sedes || []).length} sede(s) · {emp.usuarios} usuario(s)</div>
      </button>

      {abierta ? (
        <div className="px-4 pb-4 pt-1 space-y-4 border-t border-slate-800/70">
          <div className="grid md:grid-cols-3 gap-4 pt-3">
            <div>
              <div className="text-[12.5px] font-medium text-slate-300 mb-1.5">Acceso</div>
              <Toggle checked={!!emp.activa} onChange={toggleActiva} label="Empresa activa" sub="Oculta esta empresa sin tocar el resto" />
            </div>
            <SandboxEmp emp={emp} onCambio={onCambio} />
            <RespaldoEmp emp={emp} />
          </div>
          <BitacoraEmp empresaId={emp.id} />
        </div>
      ) : null}
    </div>
  )
}

/* --- Comercial (corporación) ---------------------------------------------- */

function PlanLimites({ org, uso, onGuardado }) {
  const toast = useToast()
  const [plan, setPlan] = useState(org.plan || 'base')
  const [lim, setLim] = useState(() => ({
    facturasMes: org.limites?.facturasMes || 0,
    usuarios: org.limites?.usuarios || 0,
    sucursales: org.limites?.sucursales || 0,
  }))
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setLim((s) => ({ ...s, [k]: Math.max(0, Number(v) || 0) }))

  const guardar = async () => {
    setBusy(true)
    try {
      await api.fijarPlan(org.id, plan, { ...lim, modulos: org.limites?.modulos || [] })
      toast({ title: 'Plan actualizado' })
      await onGuardado()
    } catch (e) { toast({ kind: 'err', title: 'No se pudo', body: e.message }) } finally { setBusy(false) }
  }

  const cuota = (c) => c ? `${c.usado} / ${c.limite === 0 ? '∞' : c.limite}${c.excede ? ' ⚠' : ''}` : '—'

  return (
    <Seccion titulo="Plan y límites" sub="Aplican a toda la corporación. 0 = sin límite.">
      <div className="space-y-3">
        <Field label="Plan"><Input value={plan} onChange={(e) => setPlan(e.target.value)} placeholder="base | pro | enterprise" /></Field>
        <div className="grid grid-cols-3 gap-2">
          <Field label="Facturas/mes" hint={cuota(uso?.facturasMes)}><Input type="number" min="0" value={lim.facturasMes} onChange={(e) => set('facturasMes', e.target.value)} /></Field>
          <Field label="Usuarios" hint={cuota(uso?.usuarios)}><Input type="number" min="0" value={lim.usuarios} onChange={(e) => set('usuarios', e.target.value)} /></Field>
          <Field label="Sucursales" hint={cuota(uso?.sucursales)}><Input type="number" min="0" value={lim.sucursales} onChange={(e) => set('sucursales', e.target.value)} /></Field>
        </div>
        <div className="flex justify-end"><Button size="sm" loading={busy} onClick={guardar}>Guardar plan</Button></div>
      </div>
    </Seccion>
  )
}

function SuscripcionCorp({ orgId, onCambio }) {
  const toast = useToast()
  const [planes, setPlanes] = useState([])
  const [hubmy, setHubmy] = useState(false)
  const [sub, setSub] = useState(undefined)
  const [plan, setPlan] = useState(null)
  const [planSel, setPlanSel] = useState('')
  const [userId, setUserId] = useState('')
  const [busy, setBusy] = useState(false)

  const cargar = useCallback(async () => {
    try {
      const [p, s] = await Promise.all([api.planes(), api.suscripcion(orgId)])
      const activos = (p.planes || []).filter((x) => x.activo)
      setPlanes(activos); setHubmy(!!p.hubmyDisponible)
      setSub(s.suscripcion || null); setPlan(s.plan || null)
      setPlanSel(s.suscripcion?.planId || activos[0]?.id || '')
      setUserId(s.suscripcion?.hubmyUserId || '')
    } catch (e) { setSub(null); toast({ kind: 'err', title: 'No se pudo cargar la suscripción', body: e.message }) }
  }, [orgId])
  useEffect(() => { cargar() }, [cargar])

  const accion = (fn, ok) => async () => {
    setBusy(true)
    try { await fn(); toast({ title: ok }); await cargar(); onCambio && onCambio() }
    catch (e) { toast({ kind: 'err', title: 'No se pudo', body: e.message }) } finally { setBusy(false) }
  }
  const asignar = accion(() => api.asignarPlan(orgId, planSel, userId), 'Plan asignado')
  const pagar = accion(() => api.marcarPagada(orgId), 'Pago registrado')
  const sincronizar = accion(() => api.sincronizarSuscripcion(orgId), 'Suscripción sincronizada')
  const cancelar = accion(() => api.cancelarSuscripcion(orgId), 'Suscripción cancelada')

  const est = sub ? (ESTADO_SUB[sub.estado] || { label: sub.estado, color: 'slate' }) : null

  return (
    <Seccion titulo="Suscripción" sub="La CORPORACIÓN es la unidad de pago. Al asignar el plan se aplican sus límites a toda la corporación."
      aside={sub ? <Badge color={est.color}>{est.label}</Badge> : <Badge color="slate">Sin plan</Badge>}>
      {sub === undefined ? (
        <div className="flex justify-center py-6 text-slate-500"><Spinner size={20} /></div>
      ) : (
        <div className="grid md:grid-cols-2 gap-4">
          <div className="space-y-2 text-[13px]">
            {sub && plan ? (
              <>
                <div className="flex justify-between"><span className="text-slate-500">Plan</span><span className="text-slate-200">{plan.nombre} · {money(plan.precioCents, plan.moneda)}/{plan.intervalo}</span></div>
                {sub.inicio ? <div className="flex justify-between"><span className="text-slate-500">Desde</span><span className="text-slate-300 font-mono">{sub.inicio.slice(0, 10)}</span></div> : null}
                {sub.proximoCobro ? <div className="flex justify-between"><span className="text-slate-500">Próximo cobro</span><span className="text-slate-300 font-mono">{sub.proximoCobro.slice(0, 10)}</span></div> : null}
                {sub.checkoutUrl ? (
                  <a href={sub.checkoutUrl} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-teal-400 hover:text-teal-300 text-[12.5px]">Abrir checkout de pago ↗</a>
                ) : null}
                <div className="flex flex-wrap gap-2 pt-2">
                  {sub.estado !== 'activa' ? <Button size="sm" onClick={pagar} loading={busy}>Marcar pagada</Button> : null}
                  {sub.hubmySaleId ? <Button size="sm" variant="ghost" onClick={sincronizar} loading={busy}>Sincronizar</Button> : null}
                  {sub.estado !== 'cancelada' ? <Button size="sm" variant="danger" onClick={cancelar} loading={busy}>Cancelar</Button> : null}
                </div>
              </>
            ) : (
              <div className="text-slate-500">Esta corporación todavía no tiene un plan asignado.</div>
            )}
          </div>
          <div className="space-y-2.5 md:border-l md:border-slate-800 md:pl-4">
            {planes.length === 0 ? (
              <div className="text-[12.5px] text-slate-500">No hay planes en el catálogo. Creá uno en Facturación.</div>
            ) : (
              <>
                <Field label={sub ? 'Cambiar de plan' : 'Asignar plan'}>
                  <Select value={planSel} onChange={(e) => setPlanSel(e.target.value)}>
                    {planes.map((p) => <option key={p.id} value={p.id}>{p.nombre} — {money(p.precioCents, p.moneda)}/{p.intervalo}</option>)}
                  </Select>
                </Field>
                {hubmy ? (
                  <Field label="Usuario Hubmy que paga (opcional)" hint="id del dueño de la corporación; genera el checkout">
                    <Input value={userId} onChange={(e) => setUserId(e.target.value)} placeholder="usr_01H…" />
                  </Field>
                ) : null}
                <Button size="sm" onClick={asignar} loading={busy} disabled={!planSel}>{sub ? 'Cambiar plan' : 'Asignar plan'}</Button>
              </>
            )}
          </div>
        </div>
      )}
    </Seccion>
  )
}

/* --- Operaciones por EMPRESA ---------------------------------------------- */

function SandboxEmp({ emp, onCambio }) {
  const toast = useToast()
  const [ttl, setTtl] = useState(30)
  const [busy, setBusy] = useState(false)

  const crear = async () => {
    setBusy(true)
    try {
      const r = await api.crearSandbox(emp.id, Number(ttl) || 30)
      toast({ title: 'Sandbox creado', body: `Se clonaron los maestros (${Object.values(r.clon || {}).reduce((a, b) => a + b, 0)} registros).` })
      await onCambio()
    } catch (e) { toast({ kind: 'err', title: 'No se pudo crear', body: e.message }) } finally { setBusy(false) }
  }
  const archivar = async () => {
    setBusy(true)
    try { await api.eliminarSandbox(emp.id); toast({ title: 'Sandbox archivado' }); await onCambio() }
    catch (e) { toast({ kind: 'err', title: 'No se pudo archivar', body: e.message }) } finally { setBusy(false) }
  }

  return (
    <div>
      <div className="text-[12.5px] font-medium text-slate-300 mb-1.5">QA / Sandbox</div>
      {emp.sandbox ? (
        <Button size="sm" variant="danger" loading={busy} onClick={archivar}>Archivar sandbox</Button>
      ) : (
        <div className="flex items-end gap-2">
          <div className="w-20"><Field label="Días"><Input type="number" min="1" value={ttl} onChange={(e) => setTtl(e.target.value)} /></Field></div>
          <Button size="sm" loading={busy} onClick={crear}>Crear</Button>
        </div>
      )}
    </div>
  )
}

function RespaldoEmp({ emp }) {
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const bajar = async () => {
    setBusy(true)
    try {
      const r = await descargarExport(emp.id)
      toast({ title: 'Respaldo generado', body: `${r.nombre}${r.cifrado ? ' (cifrado)' : ''}` })
    } catch (e) { toast({ kind: 'err', title: 'No se pudo', body: e.message }) } finally { setBusy(false) }
  }
  return (
    <div>
      <div className="text-[12.5px] font-medium text-slate-300 mb-1.5">Respaldo / export</div>
      <Button size="sm" variant="navy" loading={busy} onClick={bajar}>Descargar</Button>
    </div>
  )
}

function BitacoraEmp({ empresaId }) {
  const [abierta, setAbierta] = useState(false)
  const [eventos, setEventos] = useState(null)
  const [desde, setDesde] = useState('')
  const [hasta, setHasta] = useState('')
  const [busy, setBusy] = useState(false)

  const cargar = useCallback(async () => {
    setBusy(true)
    try { const r = await api.auditoria(empresaId, { desde, hasta, limite: 200 }); setEventos(r.eventos || []) }
    catch { setEventos([]) } finally { setBusy(false) }
  }, [empresaId, desde, hasta])

  const abrir = () => { setAbierta((v) => !v); if (eventos === null) cargar() }

  return (
    <div className="border-t border-slate-800/70 pt-3">
      <button onClick={abrir} className="text-[12.5px] font-medium text-slate-300 hover:text-slate-100 ring-focus rounded">
        {abierta ? '▾' : '▸'} Bitácora de actividad
      </button>
      {abierta ? (
        <div className="mt-2">
          <div className="flex items-end gap-2 mb-2">
            <div className="w-36"><Field label="Desde"><Input type="date" value={desde} onChange={(e) => setDesde(e.target.value)} /></Field></div>
            <div className="w-36"><Field label="Hasta"><Input type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} /></Field></div>
            <Button size="sm" variant="ghost" loading={busy} onClick={cargar}>Filtrar</Button>
          </div>
          {eventos === null ? (
            <div className="flex justify-center py-6 text-slate-500"><Spinner size={18} /></div>
          ) : eventos.length === 0 ? (
            <Empty title="Sin eventos" body="No hay actividad en el rango." />
          ) : (
            <div className="overflow-x-auto max-h-80 overflow-y-auto">
              <table className="w-full text-[13px]">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wide text-slate-500 border-b border-slate-800 sticky top-0 bg-slate-900">
                    <th className="px-3 py-2 font-medium">Fecha</th>
                    <th className="px-3 py-2 font-medium">Actor</th>
                    <th className="px-3 py-2 font-medium">Acción</th>
                    <th className="px-3 py-2 font-medium">Detalle</th>
                  </tr>
                </thead>
                <tbody>
                  {eventos.map((e) => (
                    <tr key={e.id} className="border-b border-slate-800/60 last:border-0">
                      <td className="px-3 py-2 text-slate-400 font-mono whitespace-nowrap">{(e.fecha || '').replace('T', ' ').slice(0, 19)}</td>
                      <td className="px-3 py-2 text-slate-300">{e.actor}{e.rol ? <span className="text-slate-600"> · {e.rol}</span> : null}</td>
                      <td className="px-3 py-2"><Badge color="slate">{e.accion}</Badge></td>
                      <td className="px-3 py-2 text-slate-400">{e.detalle || e.entidad}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      ) : null}
    </div>
  )
}
