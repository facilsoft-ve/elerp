import { useState, useEffect, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Empty, PageHeader, useToast, TableSkeleton, useConfirm, Modal, Input } from '../components/primitives.jsx'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'

// Aplicaciones — vitrina de módulos: el ERP incluido (core) y las funciones
// comercializables que el cliente instala/activa/desactiva. Ver el marco en
// backend internal/domain/aplicacion + application/aplicacion.go.

const CATEGORIAS = {
  operacion: 'Operación',
  comercial: 'Comercial',
  finanzas: 'Finanzas',
  gestion: 'Gestión',
}

const PUEDE_EDITAR = ['dueno', 'desarrollador']

function estadoDe(m) {
  if (m.core) return { label: 'Incluido', color: 'huberp' }
  if (m.proximamente) return { label: 'Próximamente', color: 'slate' }
  if (m.instalado && m.activo) return { label: 'Activo', color: 'emerald' }
  if (m.instalado) return { label: 'Inactivo', color: 'amber' }
  return { label: 'No instalado', color: 'slate' }
}

export function Aplicaciones() {
  const { ui } = useUI()
  const { reload } = useData()
  const toast = useToast()
  const confirm = useConfirm()
  const puedeEditar = PUEDE_EDITAR.includes(ui.rol)

  const [rows, setRows] = useState(null)
  const [err, setErr] = useState(null)
  const [busyId, setBusyId] = useState('')
  const [q, setQ] = useState('')
  const [info, setInfo] = useState(null) // módulo mostrado en "Más información"

  const cargar = useCallback(async () => {
    setErr(null)
    try { setRows(await api.aplicaciones()) }
    catch (e) { setErr(e); setRows([]) }
  }, [])
  useEffect(() => { cargar() }, [cargar])

  // Tras un cambio, refrescar la vitrina Y el bootstrap (db.MODULOS alimenta el
  // ocultado del menú y el gating de la UI).
  const accion = async (fn, m, verbo) => {
    setBusyId(m.id)
    try {
      const res = await fn(m.id)
      if (Array.isArray(res)) setRows(res)
      await reload()
      toast({ title: `Módulo ${verbo}`, body: m.nombre })
    } catch (e) {
      toast({ title: 'No se pudo completar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  const desinstalar = async (m) => {
    if (!(await confirm({
      title: `¿Desinstalar ${m.nombre}?`,
      body: m.avisoDesinstalar || 'El módulo dejará de estar disponible y se ocultará su sección. Su configuración se conserva; puedes volver a instalarlo cuando quieras.',
      confirmLabel: 'Desinstalar', tone: 'danger',
    }))) return
    accion(api.desinstalarModulo, m, 'desinstalado')
  }

  const term = q.trim().toLowerCase()
  const lista = (rows || []).filter((m) => !term
    || (m.nombre || '').toLowerCase().includes(term)
    || (m.descripcion || '').toLowerCase().includes(term))
  const grupos = Object.keys(CATEGORIAS).filter((cat) => lista.some((m) => m.categoria === cat))

  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader title="Aplicaciones"
        sub="El ERP incluido y las funciones que puedes instalar, activar o desactivar según lo que tu negocio necesite." />

      {rows && rows.length ? (
        <div className="mb-4 max-w-md">
          <Input icon={<Icon.Search size={15} />} placeholder="Buscar aplicación…"
            value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
      ) : null}

      {rows === null ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={4} cols={3} /></div>
      ) : err ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las aplicaciones"
          body={String(err?.message || err)} cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : (
        <div className="space-y-6">
          {grupos.map((cat) => (
            <div key={cat}>
              <div className="text-[11px] uppercase tracking-wide text-slate-400 font-medium mb-2">{CATEGORIAS[cat]}</div>
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                {lista.filter((m) => m.categoria === cat).map((m) => {
                  const est = estadoDe(m)
                  const bloqueado = busyId === m.id || !puedeEditar
                  return (
                    <div key={m.id} className={`bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 flex flex-col ${m.activo || m.core ? '' : 'opacity-80'}`}>
                      <div className="flex items-start justify-between gap-2">
                        <div className="h-9 w-9 rounded-lg bg-elerp-50 dark:bg-elerp-900/30 text-elerp-600 dark:text-elerp-300 inline-flex items-center justify-center shrink-0">
                          <Icon.Package size={18} />
                        </div>
                        <Badge size="sm" color={est.color} dot>{est.label}</Badge>
                      </div>
                      <div className="mt-2.5 text-[14px] font-semibold text-slate-800 dark:text-slate-100">{m.nombre}</div>
                      <div className="mt-1 text-[12.5px] text-slate-500 dark:text-slate-400 leading-snug flex-1">{m.descripcion}</div>

                      <button type="button" onClick={() => setInfo(m)}
                        className="mt-2 self-start text-[12px] font-medium text-elerp-600 dark:text-elerp-300 hover:underline inline-flex items-center gap-1 ring-focus rounded">
                        Más información <Icon.ChevRight size={13} />
                      </button>

                      {puedeEditar ? (
                        <div className="mt-3 flex items-center gap-2 flex-wrap">
                          {m.core ? (
                            <span className="text-[11.5px] text-slate-400">Viene incluido con tu plan.</span>
                          ) : m.proximamente ? (
                            <span className="text-[11.5px] text-slate-400">Disponible pronto.</span>
                          ) : !m.instalado ? (
                            <Button size="sm" icon={<Icon.Download size={14} />} disabled={bloqueado} onClick={() => accion(api.instalarModulo, m, 'instalado')}>Instalar</Button>
                          ) : (
                            <>
                              {m.activo ? (
                                <Button size="sm" variant="secondary" icon={<Icon.EyeOff size={14} />} disabled={bloqueado} onClick={() => accion(api.desactivarModulo, m, 'desactivado')}>Desactivar</Button>
                              ) : (
                                <Button size="sm" icon={<Icon.Check size={14} />} disabled={bloqueado} onClick={() => accion(api.activarModulo, m, 'activado')}>Activar</Button>
                              )}
                              <Button size="sm" variant="ghost" icon={<Icon.Trash size={14} />} disabled={bloqueado} onClick={() => desinstalar(m)}>Desinstalar</Button>
                            </>
                          )}
                        </div>
                      ) : null}
                    </div>
                  )
                })}
              </div>
            </div>
          ))}
          {grupos.length === 0 ? (
            <Empty icon={<Icon.Search size={22} />} title="Sin resultados"
              body={`Ninguna aplicación coincide con «${q}».`} />
          ) : null}
          {!puedeEditar ? (
            <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
              <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
              <span>Solo la Dueña/Admin (o el Desarrollador) instala o activa módulos. Aquí los ves, pero no puedes cambiarlos.</span>
            </div>
          ) : null}
        </div>
      )}

      {info ? (
        <Modal open onClose={() => setInfo(null)} size="md" icon={<Icon.Package size={18} />}
          title={info.nombre} sub={CATEGORIAS[info.categoria] || ''}
          footer={<>
            <Button variant="ghost" onClick={() => setInfo(null)}>Cerrar</Button>
            {puedeEditar && !info.core && !info.proximamente ? (
              info.instalado
                ? (info.activo
                  ? <Button variant="secondary" icon={<Icon.EyeOff size={15} />} onClick={() => { accion(api.desactivarModulo, info, 'desactivado'); setInfo(null) }}>Desactivar</Button>
                  : <Button icon={<Icon.Check size={15} />} onClick={() => { accion(api.activarModulo, info, 'activado'); setInfo(null) }}>Activar</Button>)
                : <Button icon={<Icon.Download size={15} />} onClick={() => { accion(api.instalarModulo, info, 'instalado'); setInfo(null) }}>Instalar</Button>
            ) : null}
          </>}>
          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <Badge size="sm" color={estadoDe(info).color} dot>{estadoDe(info).label}</Badge>
              {info.core ? <span className="text-[11.5px] text-slate-400">Incluido · no se puede desvincular</span> : null}
            </div>
            <p className="text-[13px] text-slate-600 dark:text-slate-300 leading-relaxed">{info.detalle || info.descripcion}</p>
            {info.requiere && info.requiere.length ? (
              <div className="text-[12.5px] text-slate-500">
                <span className="font-medium text-slate-600 dark:text-slate-300">Requiere: </span>
                {info.requiere.map((id) => (rows || []).find((x) => x.id === id)?.nombre || id).join(', ')}
              </div>
            ) : null}
            {info.requeridoPor && info.requeridoPor.length ? (
              <div className="text-[12.5px] text-slate-500">
                <span className="font-medium text-slate-600 dark:text-slate-300">Lo requieren: </span>
                {info.requeridoPor.join(', ')}
              </div>
            ) : null}
            {info.avisoDesinstalar && info.instalado ? (
              <div className="flex items-start gap-2 text-[12px] text-amber-700 dark:text-amber-300 bg-amber-50/60 dark:bg-amber-900/15 border border-amber-200 dark:border-amber-800/50 rounded-lg px-3 py-2">
                <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" />
                <span><strong>Al desinstalar:</strong> {info.avisoDesinstalar}</span>
              </div>
            ) : null}
          </div>
        </Modal>
      ) : null}
    </div>
  )
}
