import { useEffect, useState, useCallback } from 'react'
import { api } from './lib/api.js'
import { ToastProvider, Spinner, Button } from './ui.jsx'
import { Login } from './screens/Login.jsx'
import { Clientes } from './screens/Clientes.jsx'
import { Cliente } from './screens/Cliente.jsx'
import { Facturacion } from './screens/Facturacion.jsx'
import logoUrl from './assets/elerp-logo.svg'

// Marca de la consola: logotipo OFICIAL "elerp" (versión blanca, fondo oscuro) + etiqueta
// "Plataforma".
function Marca() {
  return (
    <div className="flex items-center gap-2.5">
      <img src={logoUrl} alt="elerp" style={{ height: 22, width: 'auto', display: 'block', filter: 'brightness(0) invert(1)' }} />
      <span className="text-[10.5px] text-elerp-300 tracking-wide uppercase border-l border-slate-700 pl-2.5">Plataforma</span>
    </div>
  )
}

function rutaActual() {
  const h = window.location.hash || ''
  const m = h.match(/^#\/org\/(.+)$/)
  if (m) return { name: 'org', orgId: decodeURIComponent(m[1]) }
  if (h === '#/facturacion') return { name: 'facturacion' }
  return { name: 'clientes' }
}

const NAV = [
  { hash: '', name: 'clientes', label: 'Clientes' },
  { hash: '/facturacion', name: 'facturacion', label: 'Facturación' },
]

function Shell({ sesion, onLogout }) {
  const [ruta, setRuta] = useState(rutaActual())
  useEffect(() => {
    const onHash = () => setRuta(rutaActual())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])
  const navegar = useCallback((hash) => { window.location.hash = hash }, [])

  return (
    <div className="min-h-screen flex flex-col">
      <header className="shrink-0 h-14 border-b border-slate-800 bg-slate-900/80 backdrop-blur flex items-center px-4 md:px-6 gap-4">
        <button onClick={() => navegar('')} className="ring-focus rounded-lg"><Marca /></button>
        <nav className="flex items-center gap-1 ml-2">
          {NAV.map((n) => {
            const activo = ruta.name === n.name || (n.name === 'clientes' && ruta.name === 'org')
            return (
              <button key={n.name} onClick={() => navegar(n.hash)}
                className={`h-8 px-3 rounded-lg text-[13px] font-medium ring-focus transition-colors ${activo ? 'bg-slate-800 text-slate-100' : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'}`}>
                {n.label}
              </button>
            )
          })}
        </nav>
        <div className="ml-auto flex items-center gap-3">
          <span className="text-[12.5px] text-slate-400 hidden sm:block">{sesion.email}</span>
          <Button variant="ghost" size="sm" onClick={onLogout}>Salir</Button>
        </div>
      </header>
      <main className="flex-1 min-w-0">
        <div className="max-w-[1200px] mx-auto w-full px-4 md:px-6 py-6">
          {ruta.name === 'org'
            ? <Cliente orgId={ruta.orgId} volver={() => navegar('')} />
            : ruta.name === 'facturacion'
              ? <Facturacion />
              : <Clientes abrir={(orgId) => navegar(`/org/${encodeURIComponent(orgId)}`)} />}
        </div>
      </main>
    </div>
  )
}

export function App() {
  const [sesion, setSesion] = useState(undefined) // undefined = cargando; null = sin sesión
  useEffect(() => {
    api.me().then(setSesion).catch(() => setSesion(null))
  }, [])

  const logout = useCallback(async () => {
    try { await api.logout() } catch { /* noop */ }
    setSesion(null)
    window.location.hash = ''
  }, [])

  let contenido
  if (sesion === undefined) {
    contenido = <div className="min-h-screen flex items-center justify-center text-slate-500"><Spinner size={24} /></div>
  } else if (!sesion) {
    contenido = <Login onEntrar={setSesion} />
  } else {
    contenido = <Shell sesion={sesion} onLogout={logout} />
  }
  return <ToastProvider>{contenido}</ToastProvider>
}
