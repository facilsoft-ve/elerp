import { useState, useMemo } from 'react'
import { Icon } from './Icon.jsx'
import { Segmented, Badge, Button, ConexionChip, useConfirm } from './primitives.jsx'
import { TasaChip } from './tasa.jsx'
import { useUI, useConexion } from '../context/UIContext.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { useData } from '../context/DataContext.jsx'
import { monedaLabel } from '../lib/precio.js'

export const ROLE_LABELS = {
  dueno: 'Dueña / Admin',
  vendedor: 'Vendedor',
  cajero: 'Cajero',
  contadora: 'Contadora',
  desarrollador: 'Desarrollador',
  super_admin: 'Super Admin',
}

// Ítem de un menú desplegable (organización / empresa / sede).
function SwitchItem({ active, onClick, children, sub }) {
  return (
    <button onClick={onClick}
      className={`w-full flex items-center gap-2 px-2.5 h-9 rounded-lg text-sm ${active ? 'bg-elerp-50 text-elerp-500 dark:bg-elerp-900/40 dark:text-elerp-200' : 'text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800'}`}>
      <span className="truncate flex-1 text-left">{children}{sub ? <span className="block text-[10.5px] text-slate-400">{sub}</span> : null}</span>
      {active ? <Icon.Check size={15} /> : null}
    </button>
  )
}

// Selector jerárquico Organización ▸ Empresa ▸ Sede (modelo multi-tenant de 3
// niveles de ElERP). El tenant es la Empresa; la Sede acota los datos operativos.
function TenantSwitcher() {
  const { orgs, activeOrg, setActiveOrg, activeEmpresa, setActiveEmpresa, activeSede, setActiveSede, setAgregarEmpresa } = useAuth()
  const { ui } = useUI()
  // 03 §3.1: «El chip de Organización solo aparece para Dueña y Desarrollador.»
  const verOrg = ['dueno', 'desarrollador'].includes(ui.rol)
  const [orgOpen, setOrgOpen] = useState(false)
  const [empOpen, setEmpOpen] = useState(false)
  const [sedeOpen, setSedeOpen] = useState(false)
  if (!activeEmpresa || !activeOrg) return null

  const empresas = activeOrg.empresas || []
  const sedes = activeEmpresa.sedes || []
  const trigger = 'h-9 pl-2.5 pr-2 inline-flex items-center gap-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-[13px] font-medium max-w-[150px] hover:bg-slate-50 dark:hover:bg-slate-800'
  const panel = 'absolute left-0 mt-2 w-60 z-50 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-xl shadow-modal p-1.5 max-h-[70vh] overflow-auto'
  const heading = 'px-2.5 pt-1 pb-1 text-[11px] font-semibold uppercase tracking-wide text-slate-400'
  // Rótulo de empresa de PRUEBA (QA). Ámbar, como el resto de estados de advertencia.
  const sandboxPill = 'ml-1 shrink-0 rounded px-1 py-0.5 text-[9px] font-bold uppercase tracking-wide bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300'

  return (
    <div className="flex items-center gap-1 min-w-0">
      {/* Organización — solo Dueña y Desarrollador */}
      <div className={`relative ${verOrg ? '' : 'hidden'}`}>
        <button onClick={() => setOrgOpen((o) => !o)} className={trigger}>
          <Icon.Layers size={15} className="text-elerp-500 shrink-0" />
          <span className="truncate">{activeOrg.nombre}</span>
          <Icon.ChevDown size={15} className="text-slate-400 shrink-0" />
        </button>
        {orgOpen ? (
          <>
            <div className="fixed inset-0 z-40" onClick={() => setOrgOpen(false)} />
            <div className={panel}>
              <div className={heading}>Organización</div>
              {orgs.map((o) => (
                <SwitchItem key={o.id} active={o.id === activeOrg.id}
                  onClick={() => { setActiveOrg(o.id); setOrgOpen(false) }}>{o.nombre}</SwitchItem>
              ))}
            </div>
          </>
        ) : null}
      </div>

      <Icon.ChevRight size={13} className={`text-slate-300 dark:text-slate-600 shrink-0 ${verOrg ? '' : 'hidden'}`} />

      {/* Empresa (tenant) */}
      <div className="relative">
        <button onClick={() => setEmpOpen((o) => !o)} className={trigger}>
          <Icon.Bank size={15} className="text-elerp-500 shrink-0" />
          <span className="truncate">{activeEmpresa.nombre}</span>
          {activeEmpresa.sandbox ? <span className={sandboxPill}>Sandbox</span> : null}
          <Icon.ChevDown size={15} className="text-slate-400 shrink-0" />
        </button>
        {empOpen ? (
          <>
            <div className="fixed inset-0 z-40" onClick={() => setEmpOpen(false)} />
            <div className={panel}>
              <div className={heading}>Empresa (RIF)</div>
              {empresas.map((e) => (
                <SwitchItem key={e.id} active={e.id === activeEmpresa.id} sub={e.rif}
                  onClick={() => { setActiveEmpresa(e.id); setEmpOpen(false) }}>
                  <span className="inline-flex items-center">{e.nombre}{e.sandbox ? <span className={sandboxPill}>Sandbox</span> : null}</span>
                </SwitchItem>
              ))}
              {verOrg ? (
                <button onClick={() => { setAgregarEmpresa(true); setEmpOpen(false) }}
                  className="w-full mt-1 px-2.5 py-2 rounded-lg flex items-center gap-2 text-[13px] font-medium text-elerp-600 dark:text-elerp-300 hover:bg-elerp-50 dark:hover:bg-elerp-900/30 border-t border-slate-100 dark:border-slate-800">
                  <Icon.Plus size={15} /> Nueva empresa
                </button>
              ) : null}
            </div>
          </>
        ) : null}
      </div>

      <Icon.ChevRight size={13} className="text-slate-300 dark:text-slate-600 shrink-0" />

      {/* Sede */}
      <div className="relative">
        <button onClick={() => setSedeOpen((o) => !o)} className={trigger} disabled={sedes.length === 0}>
          <Icon.Home size={15} className="text-teal-500 shrink-0" />
          <span className="truncate">{activeSede ? activeSede.nombre : 'Sin sede'}</span>
          <Icon.ChevDown size={15} className="text-slate-400 shrink-0" />
        </button>
        {sedeOpen ? (
          <>
            <div className="fixed inset-0 z-40" onClick={() => setSedeOpen(false)} />
            <div className={panel}>
              <div className={heading}>Sede</div>
              {sedes.map((s) => (
                <SwitchItem key={s.id} active={s.id === activeSede?.id}
                  onClick={() => { setActiveSede(s.id); setSedeOpen(false) }}>{s.nombre}</SwitchItem>
              ))}
            </div>
          </>
        ) : null}
      </div>
    </div>
  )
}

function IconBtn({ label, active, onClick, children }) {
  return (
    <button onClick={onClick} title={label} aria-label={label}
      className={`h-9 w-9 inline-flex items-center justify-center rounded-lg transition-colors
        ${active ? 'bg-elerp-50 text-elerp-600 dark:bg-elerp-900/40 dark:text-elerp-300' : 'text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800'}`}>
      {children}
    </button>
  )
}

// Selector "Ver como" — previsualiza la interfaz con los ojos de otro rol.
// Solo para Dueña y Desarrollador, y SOLO cambia lo que la UI muestra: el backend
// sigue autorizando por el rol real (la UI oculta, nunca protege). Existe porque
// la Regla UX E1 exige una experiencia por rol y hay que poder verificarla.
function RolePreviewMenu({ onDone }) {
  const { ui, setUi } = useUI()
  const { rol: rolReal } = useAuth()
  if (!['dueno', 'desarrollador'].includes(rolReal)) return null
  const previsualizando = ui.rol !== rolReal
  const opciones = ['dueno', 'vendedor', 'cajero', 'contadora']
  if (rolReal === 'desarrollador') opciones.unshift('desarrollador')

  return (
    <>
      <div className="h-px bg-slate-100 dark:bg-slate-800 my-1" />
      <div className="px-2.5 pt-1 pb-1.5 text-[11px] font-semibold uppercase tracking-wide text-slate-400">Ver la app como</div>
      {opciones.map((r) => (
        <SwitchItem key={r} active={ui.rol === r} sub={r === rolReal ? 'tu rol real' : undefined}
          onClick={() => { setUi((u) => ({ ...u, rol: r })); onDone?.() }}>
          {ROLE_LABELS[r] || r}
        </SwitchItem>
      ))}
      <div className="px-2.5 pt-1.5 pb-1 text-[11px] text-slate-400 leading-snug">
        {previsualizando
          ? 'Estás viendo la app como otro rol. Los permisos los sigue aplicando el servidor con tu rol real.'
          : 'Solo cambia lo que se muestra; el servidor sigue autorizando con tu rol real.'}
      </div>
    </>
  )
}

export function Topbar({ route, onOpenPalette, onNuevo, onOpenMenu }) {
  const { ui, setUi } = useUI()
  const { user, logout, activeEmpresa } = useAuth()
  const { db } = useData()
  const [menuOpen, setMenuOpen] = useState(false)
  const enLinea = useConexion()
  const confirm = useConfirm()

  // Opciones de moneda de presentación: Bs + una por divisa ACTIVA. Con solo el
  // dólar (o sin db.TASAS) se ve idéntico a siempre: Bs | US$.
  const monedaOpciones = useMemo(() => {
    const activas = db?.TASAS?.activas
    const codigos = Array.isArray(activas) && activas.length
      ? activas.map((a) => (a.codigo || '').toUpperCase()).filter(Boolean)
      : ['USD']
    const unicos = [...new Set(codigos.filter((c) => c && c !== 'VES'))]
    return [{ value: 'VES', label: 'Bs' }, ...unicos.map((c) => ({ value: c, label: monedaLabel(c) }))]
  }, [db?.TASAS])

  const cerrarSesion = async () => {
    setMenuOpen(false)
    if (await confirm({
      title: '¿Cerrar sesión?',
      body: 'Se cerrará tu sesión en este dispositivo y volverás a la pantalla de acceso. El trabajo guardado no se pierde.',
      confirmLabel: 'Cerrar sesión',
      tone: 'danger',
    })) logout()
  }

  return (
    <header className="sticky top-0 z-30 h-[58px] shrink-0 flex items-center gap-2 px-4 border-b border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900">
      {/* Hamburguesa — SOLO móvil/tablet (< md). Abre el sidebar como cajón
          deslizante: en pantallas chicas la barra lateral no existe, así que este
          es el único acceso a la navegación. En escritorio se oculta. */}
      {onOpenMenu ? (
        <button onClick={onOpenMenu} title="Abrir el menú" aria-label="Abrir el menú"
          className="md:hidden h-9 w-9 -ml-1 inline-flex items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus">
          <Icon.Menu size={20} />
        </button>
      ) : null}

      {/* El prototipo no repite el título del módulo acá: la barra arranca con el
          selector de ámbito, que es el contexto que condiciona todo lo demás. */}
      <div className="hidden md:block min-w-0"><TenantSwitcher /></div>

      <div className="ml-auto flex items-center gap-1.5 shrink-0">
        {/* Disparador de la paleta de comandos (Ctrl/⌘+K), SIEMPRE visible: en
            táctil no hay atajo de teclado, así que buscar/navegar necesita un botón
            real. Es la navegación primaria por tarea (Regla UX). */}
        {onOpenPalette ? (
          <IconBtn label="Buscar y navegar (Ctrl/⌘+K)" onClick={onOpenPalette}>
            <Icon.Search size={18} />
          </IconBtn>
        ) : null}

        {/* La búsqueda dejó de vivir en la barra: la navegación primaria es el
            sidebar acordeón + la paleta de comandos (Ctrl/⌘+K), sin campo fijo. */}
        {onNuevo ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={onNuevo} className="hidden sm:inline-flex">Nuevo</Button>
        ) : null}

        <Segmented size="sm" value={ui.ccy} onChange={(v) => setUi((u) => ({ ...u, ccy: v }))}
          options={monedaOpciones} />

        {/* Tasa Bs/US$ del día: la trae el servidor de la fuente oficial y el
            rótulo declara su origen y su fecha (R9). Al pulsarla se abre el
            detalle, con la carga manual como último recurso. */}
        <TasaChip className="hidden xl:block" />

        <ConexionChip estado={enLinea ? 'linea' : 'offline'} className="hidden xl:inline-flex shrink-0 whitespace-nowrap" />

        <IconBtn label="Modo privado" active={ui.private} onClick={() => setUi((u) => ({ ...u, private: !u.private }))}>
          {ui.private ? <Icon.EyeOff size={18} /> : <Icon.Eye size={18} />}
        </IconBtn>
        <IconBtn label="Tema" active={ui.dark} onClick={() => setUi((u) => ({ ...u, dark: !u.dark }))}>
          {ui.dark ? <Icon.Sun size={18} /> : <Icon.Moon size={18} />}
        </IconBtn>
        <IconBtn label="Notificaciones"><Icon.Bell size={18} /></IconBtn>


        <div className="relative ml-1">
          <button onClick={() => setMenuOpen((o) => !o)}
            className="h-8 w-8 rounded-full bg-elerp-500 text-white text-[12px] font-bold inline-flex items-center justify-center ring-focus">
            {(user?.nombre || '?').slice(0, 1).toUpperCase()}
          </button>
          {menuOpen ? (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setMenuOpen(false)} />
              <div className="absolute right-0 mt-2 w-56 z-50 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-xl shadow-modal p-1.5">
                <div className="px-2.5 py-2">
                  <div className="text-sm font-medium truncate">{user?.nombre || 'Usuario'}</div>
                  <div className="text-[12px] text-slate-500 truncate">{user?.email}</div>
                  {activeEmpresa ? (
                    <div className="mt-1.5 text-[11.5px] text-slate-400 truncate">
                      {activeEmpresa.orgName} · {ROLE_LABELS[ui.rol] || ui.rol}
                    </div>
                  ) : null}
                </div>
                <RolePreviewMenu onDone={() => setMenuOpen(false)} />
                <div className="h-px bg-slate-100 dark:bg-slate-800 my-1" />
                <button onClick={cerrarSesion}
                  className="w-full flex items-center gap-2 px-2.5 h-9 rounded-lg text-sm text-[#B3362C] hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20">
                  <Icon.Logout size={16} /> Cerrar sesión
                </button>
              </div>
            </>
          ) : null}
        </div>
      </div>
    </header>
  )
}
