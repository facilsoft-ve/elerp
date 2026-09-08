import { useState, useEffect } from 'react'
import { Wordmark } from './components/Logo.jsx'
import { Sidebar } from './components/Sidebar.jsx'
import { Topbar } from './components/Topbar.jsx'
import { Icon } from './components/Icon.jsx'
import { CommandPalette } from './components/CommandPalette.jsx'
import { AIAssistant } from './components/AIAssistant.jsx'
import { LegalGate } from './components/LegalGate.jsx'
import { ToastProvider, ConfirmProvider } from './components/primitives.jsx'
import { NAV, baseRoute } from './components/nav.js'
import { Login } from './screens/Login.jsx'
import { AcceptInvite } from './screens/AcceptInvite.jsx'
import { Onboarding } from './screens/Onboarding.jsx'
import { Dashboard } from './screens/Dashboard.jsx'
import { Inventario } from './screens/Inventario.jsx'
import { Facturacion } from './screens/Facturacion.jsx'
import { Ventas } from './screens/Ventas.jsx'
import { Compras } from './screens/Compras.jsx'
import { SistemaDiseno } from './screens/SistemaDiseno.jsx'
import { Placeholder } from './screens/Placeholder.jsx'
import { ModoCaja } from './screens/ModoCaja.jsx'
import { PantallaCliente } from './screens/PantallaCliente.jsx'
import { Configuracion } from './screens/Configuracion.jsx'
import { Restaurante } from './screens/Restaurante.jsx'
import { Aplicaciones } from './screens/Aplicaciones.jsx'
import { Tesoreria } from './screens/Tesoreria.jsx'
import { Contabilidad } from './screens/Contabilidad.jsx'
import { Reportes } from './screens/Reportes.jsx'
import { AuthProvider, useAuth } from './context/AuthContext.jsx'
import { DataProvider, useData } from './context/DataContext.jsx'
import { UIProvider, useUI } from './context/UIContext.jsx'

function Splash({ text = 'Cargando…' }) {
  return (
    <div className="min-h-screen flex flex-col items-center justify-center gap-3 bg-slate-50 dark:bg-slate-950">
      <Wordmark size={40} />
      <div className="text-sm text-slate-500">{text}</div>
    </div>
  )
}

function ErrorScreen({ error, onRetry }) {
  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50 dark:bg-slate-950 p-6">
      <div className="text-center max-w-sm">
        <div className="text-lg font-semibold">No se pudieron cargar los datos</div>
        <div className="text-sm text-slate-500 mt-1">{String(error?.message || error)}</div>
        <button onClick={onRetry} className="mt-4 h-9 px-4 rounded-xl bg-elerp-500 text-white text-sm font-medium">Reintentar</button>
      </div>
    </div>
  )
}

// Franja de sesión de DEMOSTRACIÓN. Discreta y de marca (navy/teal): una línea
// fina bajo el topbar, presente en toda la app mientras la sesión sea demo. No
// interrumpe el trabajo (no es un modal), solo declara que es una sesión de
// prueba con datos de muestra. Contraste AA en claro y oscuro.
function DemoBanner() {
  return (
    <div className="shrink-0 flex items-center gap-2 px-4 py-1.5 text-[12.5px] border-b
      border-teal-200 dark:border-teal-900/60 bg-teal-50 dark:bg-teal-950/40 text-teal-900 dark:text-teal-100">
      <Icon.Sparkles size={14} className="shrink-0 text-teal-600 dark:text-teal-300" />
      <span className="min-w-0">
        <span className="font-semibold">Estás en una demostración</span>
        <span className="text-teal-700 dark:text-teal-300"> · sesión limitada con datos de muestra.</span>
      </span>
      <a href="https://mornix.tech" target="_blank" rel="noreferrer"
        className="ml-auto shrink-0 inline-flex items-center gap-1 font-semibold underline underline-offset-2 hover:text-teal-700 dark:hover:text-white ring-focus rounded">
        Solicitar acceso <Icon.ArrowRight size={13} />
      </a>
    </div>
  )
}

// Shell autenticado: sidebar + topbar + ruteo por estado (patrón del prototipo).
function Shell() {
  const { db, loading, refreshing, error, reload } = useData()
  // El rol EFECTIVO es el de la UI (permite «Ver la app como…» para revisar la
  // experiencia de cada rol). El servidor sigue autorizando con el rol real: esto
  // solo decide qué se muestra.
  const { ui } = useUI()
  const rol = ui.rol
  // El CAJERO no navega la aplicación: su pantalla es la caja (R2). Arranca en
  // Modo caja a pantalla completa y no ve Inicio ni el lanzador de módulos.
  const esCajero = rol === 'cajero'
  // El MESONERO arranca en la comandera del restaurante (su pantalla de tablet).
  const rutaInicial = esCajero ? 'pos' : (rol === 'mesonero' ? 'restaurante:comandera' : 'dashboard')
  const [route, setRoute] = useState(rutaInicial)
  const [paletteOpen, setPaletteOpen] = useState(false)
  // Cajón de navegación móvil: en < md el sidebar vive como overlay deslizante que
  // abre la hamburguesa del Topbar. El estado vive acá (el shell monta Sidebar y
  // Topbar) y se pasa a ambos.
  const [drawerOpen, setDrawerOpen] = useState(false)
  // Modo caja a pantalla completa: para el cajero es su estado natural; los
  // demás roles entran y salen desde el punto de venta.
  const [modoCaja, setModoCaja] = useState(esCajero)

  // Navegación única del shell: "pos" no es una vista, es un LANZADOR — abre el
  // Modo caja a pantalla completa. Cualquier otro id enruta normal. Todo el shell
  // (sidebar, paleta, dashboard) navega por acá para que el lanzador sea robusto.
  const navigate = (r) => {
    setDrawerOpen(false)
    if (baseRoute(r) === 'pos') { setModoCaja(true); return }
    setRoute(r)
  }

  // Cambiar de rol efectivo mueve el punto de partida: el cajero entra a su caja,
  // y quien deja de ser cajero vuelve a la aplicación normal.
  useEffect(() => {
    setModoCaja(esCajero)
    if (esCajero) setRoute('pos')
  }, [esCajero])

  // Guard de rol: si la ruta no está permitida para el rol activo, al inicio.
  useEffect(() => {
    const item = NAV.find((n) => n.id === baseRoute(route))
    if (item && !item.roles.includes(rol)) setRoute(rol === 'cajero' ? 'pos' : 'dashboard')
  }, [route, rol])

  // Paleta de comandos global (Ctrl/⌘+K).
  useEffect(() => {
    const onKey = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setPaletteOpen((o) => !o)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  if (loading) return <Splash text="Cargando datos de la empresa…" />
  if (error) return <ErrorScreen error={error} onRetry={reload} />

  // Pantalla completa: sin sidebar ni topbar. El cajero solo sale si la empresa
  // no exige PIN de supervisor, o si un supervisor lo autoriza (dentro de ModoCaja).
  if (modoCaja) {
    // Al salir del puesto de cobro se vuelve al shell (Inicio); el cajero, que no
    // navega la app, se mantiene en su lanzador de Punto de Venta.
    return <ModoCaja onSalir={() => { setModoCaja(false); setRoute(esCajero ? 'pos' : 'dashboard') }} />
  }

  const base = baseRoute(route)
  let screen
  if (route === 'dashboard') screen = <Dashboard setRoute={navigate} />
  else if (base === 'inventario') screen = <Inventario route={route} />
  else if (base === 'facturacion') screen = <Facturacion route={route} />
  else if (base === 'config') screen = <Configuracion route={route} />
  else if (base === 'finanzas') screen = <Tesoreria route={route} />
  else if (base === 'contabilidad') screen = <Contabilidad route={route} />
  else if (base === 'ventas') screen = <Ventas route={route} navigate={navigate} />
  else if (base === 'compras') screen = <Compras route={route} navigate={navigate} />
  else if (base === 'reportes') screen = <Reportes route={route} />
  else if (base === 'aplicaciones') screen = <Aplicaciones route={route} />
  else if (base === 'restaurante') screen = <Restaurante route={route} />
  else if (base === 'diseno') screen = <SistemaDiseno />
  else screen = <Placeholder route={route} />

  const nav = (r) => { navigate(r); setPaletteOpen(false) }

  return (
    <div className="min-h-screen flex bg-slate-50 dark:bg-slate-950 text-slate-900 dark:text-slate-100">
      <Sidebar route={route} setRoute={navigate} drawerOpen={drawerOpen} onCloseDrawer={() => setDrawerOpen(false)} />
      <div className="flex-1 min-w-0 flex flex-col">
        {/* "+ Nuevo" abre el buscador de comandos: es el único lugar que conoce
            todo lo que el rol puede crear, en vez de un menú duplicado. */}
        <Topbar route={route} onOpenMenu={() => setDrawerOpen(true)}
          onOpenPalette={() => setPaletteOpen(true)} onNuevo={() => setPaletteOpen(true)} />
        {db.DEMO ? <DemoBanner /> : null}
        {/* Refresco en curso: una línea, sin desmontar la pantalla (así ya no se
            borra la factura recién emitida). */}
        {refreshing ? <div className="h-0.5 bg-teal-400/70 animate-pulse" /> : null}
        <main className="flex-1 min-w-0"><div className="max-w-[1480px] mx-auto w-full">{screen}</div></main>
      </div>
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} setRoute={nav} />
      <AIAssistant />
    </div>
  )
}

// Modo display: la MISMA SPA abierta como «pantalla del cliente» del POS
// (window.open a …#pantalla-cliente). Se detecta temprano por un marcador en la
// URL (hash o ?display=cliente). Comparte origen/sesión con la caja.
function esModoDisplay() {
  if (typeof window === 'undefined') return false
  const hash = window.location.hash || ''
  const q = new URLSearchParams(window.location.search).get('display')
  return hash === '#pantalla-cliente' || q === 'cliente'
}

function Authed() {
  const { user, loading, needsOnboarding, empresas, activeEmpresa, rol, agregarEmpresa, setAgregarEmpresa } = useAuth()
  if (loading) return <Splash text="Verificando sesión…" />
  // Link de invitación (…/?invite=<token>): mostrar la aceptación si no hay sesión.
  const inviteToken = new URLSearchParams(window.location.search).get('invite')
  if (!user && inviteToken) return <AcceptInvite token={inviteToken} />
  if (!user) return <Login />
  // La pantalla del cliente se renderiza DENTRO de los providers (sesión +
  // empresa + tasas) pero sin el shell admin ni el POS.
  const display = esModoDisplay()

  let contenido
  if (needsOnboarding || empresas.length === 0 || !activeEmpresa) {
    // Primer login (sin empresas) o cuenta que aún debe completar onboarding.
    contenido = <Onboarding />
  } else if (agregarEmpresa) {
    // Alta de OTRA empresa a pedido del cliente (reusa el asistente, con cancelar).
    contenido = <Onboarding modo="agregar" onCerrar={() => setAgregarEmpresa(false)} />
  } else {
    contenido = (
      <DataProvider>
        <UIProvider rol={rol}>
          <ToastProvider>
            <ConfirmProvider>
              {display ? <PantallaCliente /> : <Shell />}
            </ConfirmProvider>
          </ToastProvider>
        </UIProvider>
      </DataProvider>
    )
  }

  // Muro de aceptación de Términos + Privacidad: bloquea la app hasta aceptar la versión
  // vigente. La pantalla del cliente (ventana secundaria) no se gatea (la sesión ya aceptó).
  if (display) return contenido
  return <LegalGate>{contenido}</LegalGate>
}

export default function App() {
  return (
    <AuthProvider>
      <Authed />
    </AuthProvider>
  )
}
