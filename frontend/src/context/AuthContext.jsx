import { createContext, useContext, useState, useEffect, useCallback, useMemo } from 'react'
import { api, setActiveEmpresa as setApiEmpresa, setActiveSede as setApiSede } from '../lib/api.js'

const AuthCtx = createContext(null)
export const useAuth = () => useContext(AuthCtx)

const LS_EMPRESA = 'huberp.activeEmpresa'
const LS_SEDE = 'huberp.activeSede'

// Multi-tenant de 3 niveles: Organización → Empresa (tenant, con RIF) → Sede.
// Aplana las organizaciones del usuario a una lista de empresas seleccionables,
// cada una con su organización, el rol del usuario y sus sedes.
function flattenEmpresas(orgs) {
  const out = []
  for (const org of orgs || []) {
    for (const emp of org.empresas || []) {
      out.push({ ...emp, orgId: org.id, orgName: org.nombre, sedes: emp.sedes || [] })
    }
  }
  return out
}

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const [orgs, setOrgs] = useState([])
  const [needsOnboarding, setNeedsOnboarding] = useState(false)
  // Alta de OTRA empresa (cliente ya activo): abre el asistente de onboarding.
  const [agregarEmpresa, setAgregarEmpresa] = useState(false)
  const [activeEmpresaId, setActiveEmpresaId] = useState(() => localStorage.getItem(LS_EMPRESA) || '')
  const [activeSedeId, setActiveSedeId] = useState(() => localStorage.getItem(LS_SEDE) || '')
  const [loading, setLoading] = useState(true)

  const empresas = useMemo(() => flattenEmpresas(orgs), [orgs])

  const applyMe = useCallback((me) => {
    setUser(me?.user || null)
    const list = me?.organizaciones || []
    setOrgs(list)
    setNeedsOnboarding(!!me?.needsOnboarding)
    return flattenEmpresas(list)
  }, [])

  const refresh = useCallback(async () => {
    try {
      const me = await api.me()
      const flat = applyMe(me)
      // Empresa activa: la guardada si sigue existiendo, si no la primera.
      setActiveEmpresaId((prev) => {
        const valid = flat.some((e) => e.id === prev)
        return valid ? prev : (flat[0]?.id || '')
      })
    } catch {
      setUser(null)
      setOrgs([])
      setNeedsOnboarding(false)
    } finally {
      setLoading(false)
    }
  }, [applyMe])

  useEffect(() => { refresh() }, [refresh])

  const activeEmpresa = useMemo(
    () => empresas.find((e) => e.id === activeEmpresaId) || null,
    [empresas, activeEmpresaId],
  )

  // Al cambiar de empresa, asegurar que la sede activa pertenezca a ella; si no,
  // usar la primera sede de la empresa.
  useEffect(() => {
    if (!activeEmpresa) return
    const sedes = activeEmpresa.sedes || []
    const valid = sedes.some((s) => s.id === activeSedeId)
    if (!valid) setActiveSedeId(sedes[0]?.id || '')
  }, [activeEmpresa, activeSedeId])

  // Mantener sincronizados los headers del cliente HTTP y el localStorage.
  useEffect(() => {
    setApiEmpresa(activeEmpresaId)
    if (activeEmpresaId) localStorage.setItem(LS_EMPRESA, activeEmpresaId)
    else localStorage.removeItem(LS_EMPRESA)
  }, [activeEmpresaId])

  useEffect(() => {
    setApiSede(activeSedeId)
    if (activeSedeId) localStorage.setItem(LS_SEDE, activeSedeId)
    else localStorage.removeItem(LS_SEDE)
  }, [activeSedeId])

  const activeSede = useMemo(
    () => (activeEmpresa?.sedes || []).find((s) => s.id === activeSedeId) || null,
    [activeEmpresa, activeSedeId],
  )

  // Organización activa (derivada de la empresa activa).
  const activeOrg = useMemo(
    () => orgs.find((o) => o.id === activeEmpresa?.orgId) || null,
    [orgs, activeEmpresa],
  )

  // Cambiar de empresa LIMPIA la sede en el mismo cambio: la sede activa pertenece a la
  // empresa anterior y una consulta con un X-Sede-ID de otra empresa devuelve 400 (rompía
  // la carga completa al cambiar de rubro en la demo). El efecto de más arriba elige
  // enseguida la primera sede de la empresa nueva; mientras tanto DataContext mantiene la
  // pantalla de carga.
  const setActiveEmpresa = useCallback((id) => {
    if (!id || id === activeEmpresaId) return
    setActiveSedeId('')
    setActiveEmpresaId(id)
  }, [activeEmpresaId])
  const setActiveSede = useCallback((id) => setActiveSedeId(id), [])

  // Cambiar de organización = activar su primera empresa.
  const setActiveOrg = useCallback((orgId) => {
    const org = orgs.find((o) => o.id === orgId)
    const first = org?.empresas?.[0]
    if (first) setActiveEmpresa(first.id)
  }, [orgs, setActiveEmpresa])

  // Onboarding: crea empresa → configura giro/modalidad → primera sede → refresca.
  const onboard = useCallback(async ({ nombre, rif, giro, modalidadFacturacion, monedaPrincipal, preciosEnUsd, fuenteTasa, sede }) => {
    const emp = await api.createEmpresa({ nombre, rif })
    const empId = emp?.id || emp?.empresa?.id
    if (empId) {
      // La configuración de moneda viaja con el onboarding: la empresa todavía no
      // es el tenant activo, así que no se puede usar la ruta con X-Empresa-ID.
      await api.onboardEmpresa(empId, { giro, modalidadFacturacion, monedaPrincipal, preciosEnUsd, fuenteTasa })
      if (sede?.nombre) await api.createSede(empId, { nombre: sede.nombre, direccion: sede.direccion || '' })
    }
    await refresh()
    if (empId) setActiveEmpresaId(empId)
    return emp
  }, [refresh])

  // Native login (email/password). La cookie de sesión queda seteada; refrescamos.
  const nativeLogin = useCallback(async (body) => {
    await api.nativeLogin(body)
    await refresh()
  }, [refresh])

  const login = useCallback(() => { window.location.href = api.loginURL() }, [])

  const logout = useCallback(async () => {
    try { await api.logout() } catch { /* noop */ }
    setUser(null)
    localStorage.removeItem(LS_EMPRESA)
    localStorage.removeItem(LS_SEDE)
    window.location.reload()
  }, [])

  return (
    <AuthCtx.Provider value={{
      user, loading, login, logout, refresh, nativeLogin,
      orgs, empresas, needsOnboarding, onboard,
      agregarEmpresa, setAgregarEmpresa,
      activeOrg, setActiveOrg,
      activeEmpresa, activeEmpresaId, setActiveEmpresa,
      activeSede, activeSedeId, setActiveSede,
      rol: activeEmpresa?.rol || user?.rol || 'vendedor',
    }}>
      {children}
    </AuthCtx.Provider>
  )
}
