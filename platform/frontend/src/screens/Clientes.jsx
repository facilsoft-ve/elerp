import { useEffect, useMemo, useState } from 'react'
import { api } from '../lib/api.js'
import { Card, Input, Badge, Spinner, Empty, Button, Modal, Field, Select, useToast } from '../ui.jsx'

// Normaliza para búsqueda sin tildes ni mayúsculas.
const norm = (s) => (s || '').toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '')

// La UNIDAD DE CLIENTE es la CORPORACIÓN (organización): es quien paga la licencia. Las
// empresas son sus tenants hijos. Por eso la lista es de corporaciones, no de empresas.
export function Clientes({ abrir }) {
  const [orgs, setOrgs] = useState(null)
  const [error, setError] = useState('')
  const [q, setQ] = useState('')
  const [restaurar, setRestaurar] = useState(false)

  useEffect(() => {
    api.tenants()
      .then((r) => setOrgs(r.tenants || []))
      .catch((e) => setError(e.message || 'No se pudieron cargar los clientes'))
  }, [])

  const filas = useMemo(() => {
    const rows = (orgs || []).map((o) => {
      const emps = o.empresas || []
      return {
        o,
        empresas: emps.length,
        usuarios: emps.reduce((a, e) => a + (e.usuarios || 0), 0),
        sedes: emps.reduce((a, e) => a + (e.sedes || []).length, 0),
      }
    })
    if (!q.trim()) return rows
    const nq = norm(q)
    return rows.filter(({ o }) =>
      norm(o.nombre).includes(nq) ||
      (o.empresas || []).some((e) => norm(e.nombre).includes(nq) || norm(e.rif).includes(nq)))
  }, [orgs, q])

  if (error) return <Empty title="No se pudieron cargar los clientes" body={error} cta={<Button onClick={() => location.reload()}>Reintentar</Button>} />
  if (orgs === null) return <div className="flex justify-center py-16 text-slate-500"><Spinner size={24} /></div>

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-4">
        <div>
          <h1 className="font-display font-bold text-xl text-slate-100">Clientes</h1>
          <p className="text-[13px] text-slate-500">{filas.length} corporación(es) · la corporación es la unidad que paga la licencia</p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="ghost" onClick={() => setRestaurar(true)}>Restaurar respaldo</Button>
          <div className="w-64 max-w-[50vw]">
            <Input placeholder="Buscar corporación, empresa o RIF…" value={q} onChange={(e) => setQ(e.target.value)} />
          </div>
        </div>
      </div>

      {restaurar ? <RestaurarModal orgs={orgs} onClose={() => setRestaurar(false)} onHecho={() => { setRestaurar(false); location.reload() }} /> : null}

      <Card className="overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-[11.5px] uppercase tracking-wide text-slate-500 border-b border-slate-800">
                <th className="px-4 py-2.5 font-medium">Corporación (cliente)</th>
                <th className="px-4 py-2.5 font-medium">Plan</th>
                <th className="px-4 py-2.5 font-medium">Estado</th>
                <th className="px-4 py-2.5 font-medium text-right">Empresas</th>
                <th className="px-4 py-2.5 font-medium text-right">Sedes</th>
                <th className="px-4 py-2.5 font-medium text-right">Usuarios</th>
                <th className="px-4 py-2.5"></th>
              </tr>
            </thead>
            <tbody>
              {filas.map(({ o, empresas, usuarios, sedes }) => (
                <tr key={o.id} onClick={() => abrir(o.id)}
                  className="border-b border-slate-800/70 last:border-0 hover:bg-slate-800/50 cursor-pointer">
                  <td className="px-4 py-3">
                    <div className="font-medium text-slate-100">{o.nombre}</div>
                    <div className="text-[12px] text-slate-500 truncate max-w-[28rem]">
                      {(o.empresas || []).map((e) => e.nombre).join(' · ') || 'sin empresas'}
                    </div>
                  </td>
                  <td className="px-4 py-3"><Badge color="teal">{o.plan || 'base'}</Badge></td>
                  <td className="px-4 py-3">
                    {o.estado === 'suspendida' ? <Badge color="red">Suspendida</Badge> : <Badge color="emerald">Activa</Badge>}
                  </td>
                  <td className="px-4 py-3 text-right text-slate-300 tabular-nums">{empresas}</td>
                  <td className="px-4 py-3 text-right text-slate-300 tabular-nums">{sedes}</td>
                  <td className="px-4 py-3 text-right text-slate-300 tabular-nums">{usuarios}</td>
                  <td className="px-4 py-3 text-right text-slate-600">→</td>
                </tr>
              ))}
              {filas.length === 0 ? (
                <tr><td colSpan={7}><Empty title="Sin coincidencias" body="Ajustá la búsqueda." /></td></tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}

// RestaurarModal sube un archivo de respaldo y lo restaura como una EMPRESA NUEVA en la
// corporación destino (nunca sobrescribe un tenant vivo).
function RestaurarModal({ orgs, onClose, onHecho }) {
  const toast = useToast()
  const [orgId, setOrgId] = useState((orgs || [])[0]?.id || '')
  const [file, setFile] = useState(null)
  const [busy, setBusy] = useState(false)

  const restaurar = async () => {
    if (!orgId || !file) return
    setBusy(true)
    try {
      const r = await api.restaurar(orgId, file)
      const total = Object.values(r.restaurado || {}).reduce((a, b) => a + b, 0)
      toast({ title: 'Respaldo restaurado', body: `Empresa nueva creada (${total} registros importados).` })
      onHecho()
    } catch (e) { toast({ kind: 'err', title: 'No se pudo restaurar', body: e.message }) } finally { setBusy(false) }
  }

  return (
    <Modal open onClose={onClose} title="Restaurar respaldo"
      footer={<>
        <Button variant="ghost" size="sm" onClick={onClose}>Cancelar</Button>
        <Button size="sm" onClick={restaurar} loading={busy} disabled={!orgId || !file}>Restaurar</Button>
      </>}>
      <div className="space-y-3">
        <div className="rounded-lg bg-amber-500/10 border border-amber-500/30 px-3 py-2.5 text-[12.5px] text-amber-200">
          Crea una <strong>empresa nueva</strong> en la corporación elegida a partir del archivo de respaldo. No sobrescribe ningún tenant existente.
        </div>
        <Field label="Corporación destino">
          <Select value={orgId} onChange={(e) => setOrgId(e.target.value)}>
            {(orgs || []).map((o) => <option key={o.id} value={o.id}>{o.nombre}</option>)}
          </Select>
        </Field>
        <Field label="Archivo de respaldo" hint="el .json.gz (o .enc) descargado del cliente">
          <input type="file" onChange={(e) => setFile(e.target.files?.[0] || null)}
            className="block w-full text-[13px] text-slate-300 file:mr-3 file:py-1.5 file:px-3 file:rounded-lg file:border-0 file:bg-slate-700 file:text-slate-100 file:text-[13px]" />
        </Field>
      </div>
    </Modal>
  )
}
