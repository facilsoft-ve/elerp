import { createContext, useContext, useState, useEffect, useRef } from 'react'
import { useData } from './DataContext.jsx'

const UICtx = createContext(null)
export const useUI = () => useContext(UICtx)

/* Estado de UI global:
 *   ccy            moneda de presentación (los importes se guardan en Bs)
 *   dark, private  tema y modo privado
 *   rol            rol efectivo para lo que la UI muestra
 *   menuColapsado  menú lateral contraído
 *   zurdo          Modo caja: invierte productos ↔ carrito
 *
 * La tasa Bs/US$ NO vive aquí: la trae el backend desde la fuente oficial y se
 * lee con `useTasa()` (R9). Nunca hubo ni habrá un valor por defecto en el
 * frontend — una cifra inventada en la interfaz es una cifra que alguien va a
 * usar para cobrar.
 *
 * `zurdo`, `dark` y `menuColapsado` se guardan en el dispositivo: la caja es un
 * puesto físico compartido entre turnos, la preferencia es del puesto.
 */
const PERSISTENTES = ['dark', 'zurdo', 'menuColapsado']
const LLAVE = 'huberp.ui'

const leerGuardado = () => {
  try {
    const raw = localStorage.getItem(LLAVE)
    if (!raw) return {}
    const o = JSON.parse(raw)
    return Object.fromEntries(PERSISTENTES.filter((k) => k in o).map((k) => [k, o[k]]))
  } catch { return {} }
}

export function UIProvider({ rol = 'vendedor', children }) {
  const data = useData()
  const [ui, setUi] = useState(() => ({
    ccy: 'VES', dark: false, private: false, rol,
    menuColapsado: false, zurdo: false,
    ...leerGuardado(),
  }))

  // El rol efectivo sigue al rol real cuando cambia la empresa activa.
  useEffect(() => {
    setUi((u) => (u.rol === rol ? u : { ...u, rol }))
  }, [rol])

  // El MESONERO arranca con el menú COLAPSADO. Solo alcanza la Comandera, y en una
  // tablet en vertical la barra se comía cerca de un tercio del ancho útil para mostrar
  // un único destino. Es solo el valor POR DEFECTO: `menuColapsado` se guarda por
  // dispositivo, así que si él lo despliega su preferencia manda.
  //
  // Va en un efecto y no en el estado inicial porque el rol real llega DESPUÉS del
  // primer render (el mismo motivo por el que el aterrizaje del mesonero es un efecto).
  const ajustadoParaRol = useRef(false)
  useEffect(() => {
    if (ajustadoParaRol.current || rol !== 'mesonero') return
    ajustadoParaRol.current = true
    if (leerGuardado().menuColapsado === undefined) {
      setUi((u) => ({ ...u, menuColapsado: true }))
    }
  }, [rol])

  // La moneda de presentación (`ccy`) puede ser VES o cualquier divisa ACTIVA.
  // Si la que estaba elegida deja de estar activa (se quitó en Configuración),
  // se cae a VES. Solo actúa cuando db.TASAS realmente cargó: si no cargó, no se
  // fuerza nada (retrocompat con el modo USD-único).
  const activas = data?.db?.TASAS?.activas
  useEffect(() => {
    if (!Array.isArray(activas)) return
    const validas = ['VES', ...activas.map((a) => (a.codigo || '').toUpperCase())]
    setUi((u) => (validas.includes(u.ccy) ? u : { ...u, ccy: 'VES' }))
  }, [activas])

  useEffect(() => {
    document.documentElement.classList.toggle('dark', ui.dark)
    document.body.classList.toggle('private', ui.private)
  }, [ui.dark, ui.private])

  useEffect(() => {
    try {
      localStorage.setItem(LLAVE, JSON.stringify(Object.fromEntries(PERSISTENTES.map((k) => [k, ui[k]]))))
    } catch { /* almacenamiento no disponible: la preferencia dura la sesión */ }
  }, [ui.dark, ui.zurdo, ui.menuColapsado])

  return <UICtx.Provider value={{ ui, setUi }}>{children}</UICtx.Provider>
}

/* Estado de conexión real del navegador. Sostiene el chip del topbar y la regla
 * de contingencia: sin conexión la operación sigue, con serie reservada. */
export function useConexion() {
  const [enLinea, setEnLinea] = useState(() => (typeof navigator === 'undefined' ? true : navigator.onLine))
  useEffect(() => {
    const on = () => setEnLinea(true)
    const off = () => setEnLinea(false)
    window.addEventListener('online', on)
    window.addEventListener('offline', off)
    return () => { window.removeEventListener('online', on); window.removeEventListener('offline', off) }
  }, [])
  return enLinea
}
