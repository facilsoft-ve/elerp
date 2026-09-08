/* Primitivas de UI portadas del prototipo Tesotrix, re-tematizadas a ElERP. */
import React, { useState, useEffect, useRef, useId, createContext, useContext, useCallback } from 'react'
import { Icon } from './Icon.jsx'
import { HubDot, ModularCorner } from './brand.jsx'
import { fmtCurrency } from '../lib/format.js'

/* Botones — referencia: pantalla /design-system del prototipo.
 * «Una sola acción primaria por pantalla. El acento cálido es exclusivo de las
 * acciones que mueven dinero. Todo botón tiene hover, foco visible, cargando y
 * deshabilitado con su porqué.»
 *
 * Variantes tal como las muestra el prototipo:
 *   primary     → azul navy.  Guardar, Crear, Emitir. La acción de la vista.
 *   dinero      → verde.      EXCLUSIVO: Cobrar, Registrar pago, Liquidar,
 *                             Vender ahora. (alias: `cta`, `teal`)
 *   secondary   → blanco con borde gris. Exportar, acciones alternas.
 *   ghost       → gris sin fondo. Cancelar, cerrar.
 *   destructive → rojo #B3362C. Anular, revertir, eliminar.
 *   [disabled]  → gris claro (atributo, no variante).
 *
 * Ojo: el verde NO es el color de «crear algo nuevo» — eso es azul. El PDF de
 * branding lo llama «CTA / crear», pero el prototipo manda y ahí el verde es
 * dinero (ver Documentos/sistema-visual.md). */
export const Button = ({ variant = 'primary', size = 'md', icon, iconRight, children, className = '', loading, ...rest }) => {
  const sizes = {
    sm: 'h-9 px-3.5 text-[13px]',
    md: 'h-10 px-4 text-sm',   // 40px
    lg: 'h-11 px-5 text-sm',   // 44px — mínimo táctil en POS y Modo caja
    xl: 'h-14 px-6 text-[15px]', // 56px — objetivo CÓMODO con tablet en mano (comandera)
  }
  const variants = {
    primary: 'bg-elerp-500 hover:bg-elerp-600 active:bg-elerp-700 text-white',
    dinero: 'bg-teal-500 hover:bg-teal-600 active:bg-teal-700 text-white',
    cta: 'bg-teal-500 hover:bg-teal-600 active:bg-teal-700 text-white',
    teal: 'bg-teal-500 hover:bg-teal-600 active:bg-teal-700 text-white',
    secondary: 'bg-white dark:bg-slate-900 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-800 dark:text-slate-100 border border-slate-300 dark:border-slate-700',
    tertiary: 'bg-transparent hover:bg-elerp-50 dark:hover:bg-elerp-900/40 text-elerp-500 dark:text-elerp-200',
    ghost: 'bg-transparent hover:bg-slate-100 dark:hover:bg-slate-800/70 text-slate-500 dark:text-slate-300',
    destructive: 'bg-[#B3362C] hover:bg-[#9A2E25] text-white',
    subtle: 'bg-slate-100 dark:bg-slate-800/60 hover:bg-slate-200 dark:hover:bg-slate-800 text-slate-800 dark:text-slate-100',
  }
  const disabled = loading || rest.disabled
  return (
    <button
      className={`inline-flex items-center justify-center gap-1.5 rounded-lg font-display font-medium transition-colors ring-focus
        disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-400 disabled:border-transparent
        dark:disabled:bg-slate-800 dark:disabled:text-slate-500
        ${sizes[size]} ${variants[variant]} ${className}`}
      disabled={disabled} {...rest}>
      {loading ? <span className="spin shrink-0"><Icon.Refresh size={15} /></span> : icon ? <span className="shrink-0">{icon}</span> : null}
      {children ? <span>{children}</span> : null}
      {iconRight ? <span className="shrink-0">{iconRight}</span> : null}
    </button>
  )
}

/* Card — borde gris, SIN sombra en reposo (handoff §03).
 * `selected` marca la selección con el filo verde de 4px en el borde izquierdo
 * y fondo blanco, tal como especifica el handoff. */
export const Card = ({ className = '', children, padding = true, selected = false }) => (
  <div className={`bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl
    ${selected ? 'border-l-4 border-l-teal-500' : ''} ${padding ? 'p-[22px]' : ''} ${className}`}>{children}</div>
)

// Título de sección de tarjeta (17px/700) + ayuda opcional (12.5-13.5px muted).
export const CardTitle = ({ children, help, className = '' }) => (
  <div className={className}>
    <div className="text-[17px] font-bold tracking-tight text-slate-900 dark:text-slate-100">{children}</div>
    {help ? <div className="text-[13px] text-slate-500 mt-1">{help}</div> : null}
  </div>
)

export const Badge = ({ color = 'slate', children, dot = false, className = '', size = 'md' }) => {
  const palette = {
    slate: 'bg-slate-100 text-slate-700 dark:bg-slate-800/70 dark:text-slate-300',
    emerald: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    rose: 'bg-rose-50 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300',
    amber: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    sky: 'bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
    blue: 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    violet: 'bg-violet-50 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300',
    red: 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300',
    huberp: 'bg-elerp-50 text-elerp-500 dark:bg-elerp-900/40 dark:text-elerp-200',
    teal: 'bg-teal-50 text-teal-700 dark:bg-teal-500/15 dark:text-teal-300',
  }
  const dots = {
    slate: 'bg-slate-400', emerald: 'bg-emerald-500', rose: 'bg-rose-500', amber: 'bg-amber-500',
    sky: 'bg-sky-500', blue: 'bg-blue-500', violet: 'bg-violet-500', red: 'bg-red-500', huberp: 'bg-elerp-500', teal: 'bg-teal-500',
  }
  const sz = size === 'sm' ? 'text-[11px] px-2.5 py-0.5' : 'text-[11.5px] px-2.5 py-1'
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full font-semibold ${sz} ${palette[color] || palette.slate} ${className}`}>
      {dot ? <span className={`w-1.5 h-1.5 rounded-full ${dots[color] || dots.slate}`} /> : null}
      {children}
    </span>
  )
}

/* Badges de estado fiscal — los cinco del prototipo (/design-system).
 * «El color nunca es el único portador del significado: cada estado lleva su
 * texto.» Siempre con punto. Cualquier estado de la app se mapea a uno de estos
 * en lugar de inventar un color nuevo. */
export const ESTADOS = {
  emitida: { label: 'Emitida', color: 'emerald' },
  contingencia: { label: 'Contingencia', color: 'amber' },
  anulada: { label: 'Anulada', color: 'red' },
  borrador: { label: 'Borrador', color: 'slate' },
  sincronizando: { label: 'Sincronizando', color: 'huberp' },
}

export const StatusBadge = ({ estado = 'borrador', children, size = 'md', className = '' }) => {
  const e = ESTADOS[estado] || ESTADOS.borrador
  return <Badge color={e.color} dot size={size} className={className}>{children || e.label}</Badge>
}

/* Tooltip «¿qué es esto?» — el prototipo lo pone junto a cada KPI y a cada
 * término fiscal. «Todo término fiscal inevitable lleva el suyo, con ejemplo.»
 * Se abre con hover y con foco de teclado (no solo con mouse). */
export const InfoTip = ({ children, label = '¿Qué es esto?' }) => (
  <span className="relative inline-flex group align-middle">
    <button type="button" aria-label={label}
      className="text-slate-300 hover:text-slate-500 dark:text-slate-600 dark:hover:text-slate-400 ring-focus rounded-full">
      <Icon.CircleAlert size={13} />
    </button>
    <span role="tooltip"
      className="pointer-events-none absolute left-1/2 -translate-x-1/2 bottom-full mb-2 w-56 z-40
        opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity
        rounded-lg bg-slate-900 text-white text-[12px] leading-snug px-2.5 py-2 shadow-modal">
      {children}
    </span>
  </span>
)

/* Chip de estado de conexión — offline-first es visible, no implícito.
 * En línea (verde) · Sin conexión — contingencia (ámbar) · Sincronizando (azul). */
export const ConexionChip = ({ estado = 'linea', pendientes = 0, className = '' }) => {
  const mapa = {
    linea: { label: 'En línea', color: 'emerald', icon: <Icon.Globe size={13} /> },
    offline: { label: 'Sin conexión — contingencia', color: 'amber', icon: <Icon.EyeOff size={13} /> },
    sync: { label: `Sincronizando ${pendientes || ''} documento${pendientes === 1 ? '' : 's'}…`, color: 'huberp', icon: <span className="spin inline-flex"><Icon.Refresh size={13} /></span> },
  }
  const e = mapa[estado] || mapa.linea
  return <Badge color={e.color} size="sm" className={className}>{e.icon}{e.label}</Badge>
}

export const Money = ({ value, ccy = 'VES', signed = false, className = '' }) => {
  const sign = signed ? (value > 0 ? '+' : value < 0 ? '−' : '') : ''
  const display = fmtCurrency(Math.abs(value), ccy)
  const color = signed ? (value > 0 ? 'text-emerald-600 dark:text-emerald-400' : value < 0 ? 'text-slate-900 dark:text-slate-100' : 'text-slate-500') : ''
  return <span className={`private-mask num font-medium ${color} ${className}`}>{sign}{display}</span>
}

/* Card de métrica — handoff §03: borde gris, sin sombra en reposo.
 * La cifra destacada va en Poppins Bold (así la muestra el handoff: "$84,200");
 * las cifras de tabla siguen en IBM Plex Mono tabular.
 * `accent` = filo verde de 4px (marca la métrica seleccionada/protagonista).
 * `hub`    = punto hub como marcador de estado. Opcional y excepcional: solo un
 *            punto hub protagonista por vista. */
export const Stat = ({ label, value, sub, delta, deltaPositive, icon, accent, hub }) => (
  <Card className={`relative overflow-hidden !p-5 ${accent ? 'border-l-4 border-l-teal-500' : ''}`}>
    <div className="flex items-center justify-between gap-2 min-w-0">
      <div className="text-[11px] text-slate-500 dark:text-slate-400 font-medium tracking-wide uppercase inline-flex items-center gap-1.5 truncate min-w-0">
        {icon ? <span className="shrink-0">{icon}</span> : null}
        <span className="truncate">{label}</span>
      </div>
      {hub ? <HubDot size={8} className="shrink-0" /> : null}
    </div>
    <div className="font-display font-bold text-[26px] tracking-tight tnum private-mask leading-tight mt-2.5 truncate">{value}</div>
    <div className="flex items-center gap-2 mt-1.5 min-w-0">
      {delta !== undefined ? (
        <span className={`shrink-0 inline-flex items-center gap-0.5 text-[12px] font-medium tnum ${deltaPositive ? 'text-teal-600 dark:text-teal-400' : 'text-red-600 dark:text-red-400'}`}>
          {deltaPositive ? <Icon.ArrowUp size={12} /> : <Icon.ArrowDown size={12} />}{delta}
        </span>
      ) : null}
      {sub ? <div className="text-[12px] text-slate-500 dark:text-slate-400 private-mask truncate">{sub}</div> : null}
    </div>
  </Card>
)

export const Toggle = ({ checked, onChange, label, sub }) => (
  <label className="flex items-center gap-3 cursor-pointer select-none">
    <button type="button" role="switch" aria-checked={checked} onClick={() => onChange(!checked)}
      className={`relative h-5 w-9 rounded-full transition-colors ${checked ? 'bg-elerp-500' : 'bg-slate-200 dark:bg-slate-700'} ring-focus`}>
      <span className={`absolute top-0.5 left-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform ${checked ? 'translate-x-4' : ''}`} />
    </button>
    {label ? <span className="text-sm">{label}{sub ? <span className="block text-[12px] text-slate-500">{sub}</span> : null}</span> : null}
  </label>
)

export const Input = React.forwardRef(({ className = '', icon, invalid, ...rest }, ref) => (
  <div className={`relative ${className}`}>
    {icon ? <span className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-400">{icon}</span> : null}
    <input ref={ref} {...rest}
      className={`w-full h-9 ${icon ? 'pl-8' : 'pl-3'} pr-3 rounded-lg border ${invalid ? 'border-red-400 focus:ring-red-300' : 'border-slate-300 dark:border-slate-700'} bg-white dark:bg-slate-900 text-sm text-slate-900 dark:text-slate-100 placeholder:text-slate-400 dark:placeholder:text-slate-500 focus:border-elerp-500 ring-focus`} />
  </div>
))

export const Select = React.forwardRef(({ className = '', children, ...rest }, ref) => (
  <select ref={ref} {...rest}
    className={`w-full h-9 pl-3 pr-8 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm text-slate-900 dark:text-slate-100 focus:border-elerp-500 ring-focus ${className}`}>
    {children}
  </select>
))

/* Tabs — handoff §04: la pestaña activa va en azul semibold con subrayado
 * VERDE (el verde señala el estado activo). Las inactivas en gris. */
export const Tabs = ({ tabs, active, onChange, right }) => (
  <div className="flex items-center justify-between border-b border-slate-200 dark:border-slate-800">
    <div className="flex gap-1 overflow-x-auto">
      {tabs.map((t) => (
        <button key={t.id} onClick={() => onChange(t.id)}
          className={`relative h-10 px-3 text-sm transition-colors whitespace-nowrap ring-focus ${active === t.id ? 'font-semibold text-elerp-500 dark:text-white' : 'font-medium text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'}`}>
          <span className="inline-flex items-center gap-2">{t.icon}{t.label}{t.count != null ? <Badge size="sm" color="slate">{t.count}</Badge> : null}</span>
          {active === t.id ? <span className="absolute left-2 right-2 -bottom-px h-[2.5px] bg-teal-500 rounded-full" /> : null}
        </button>
      ))}
    </div>
    {right ? <div className="pb-1">{right}</div> : null}
  </div>
)

/* Breadcrumb de módulo › submódulo. Ubica al usuario dentro del módulo y da
 * contexto a la sub-navegación (las pestañas) que va justo debajo. El último
 * tramo es el submódulo activo (resaltado); los anteriores, muted. */
export const Breadcrumb = ({ items = [], className = '' }) => (
  <nav className={`flex items-center gap-1.5 text-[12px] ${className}`} aria-label="Ruta">
    {items.filter(Boolean).map((it, i, arr) => (
      <span key={i} className="inline-flex items-center gap-1.5 min-w-0">
        {i > 0 ? <Icon.ChevRight size={13} className="text-slate-300 dark:text-slate-600 shrink-0" /> : null}
        <span className={`truncate ${i === arr.length - 1 ? 'font-semibold text-slate-700 dark:text-slate-200' : 'text-slate-400'}`}>{it}</span>
      </span>
    ))}
  </nav>
)

/* Encabezado de módulo unificado — misma cadencia en TODAS las pantallas.
 * La navegación entre submódulos vive en el menú lateral (acordeón), así que el
 * contenedor NO repite pestañas: solo el título del módulo con el submódulo
 * activo como un breadcrumb PEQUEÑO al lado (más chico que el título), el
 * subtítulo opcional y las acciones a la derecha. `tabs/activeTab/onTab` se
 * aceptan por compatibilidad pero ya no se pintan. */
export const PageHeader = ({ breadcrumb, title, sub, actions }) => {
  const items = (breadcrumb || []).filter(Boolean)
  const subActual = items.length > 1 ? items[items.length - 1] : null
  return (
    <div className="relative mb-5">
      {/* Racimo modular de marca en la cabecera (Manual §08: retícula modular para
          cabeceras). Sutil, esquina superior derecha; nunca sobre datos. */}
      {!actions ? <ModularCorner corner="tr" variant="full" tone="light" className="opacity-90 hidden sm:block dark:opacity-45" /> : null}
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div className="min-w-0">
          <div className="flex items-baseline gap-2 flex-wrap">
            <h1 className="font-display text-[23px] leading-tight font-bold tracking-tight text-slate-900 dark:text-slate-50">{title}</h1>
            {subActual ? (
              <span className="inline-flex items-center gap-1 text-[14px] font-medium text-slate-400 dark:text-slate-500 min-w-0">
                <Icon.ChevRight size={14} className="text-slate-300 dark:text-slate-600 shrink-0" />
                <span className="truncate">{subActual}</span>
              </span>
            ) : null}
          </div>
          {sub ? <p className="text-[13.5px] text-slate-500 dark:text-slate-400 mt-1 max-w-2xl">{sub}</p> : null}
        </div>
        {actions ? <div className="flex items-center gap-2 shrink-0">{actions}</div> : null}
      </div>
    </div>
  )
}

export const Segmented = ({ options, value, onChange, size = 'md' }) => (
  <div className={`inline-flex items-center p-0.5 rounded-lg bg-slate-200 dark:bg-slate-800/70 ${size === 'sm' ? 'text-[12.5px]' : 'text-[13px]'}`}>
    {options.map((o) => (
      <button key={o.value} onClick={() => onChange(o.value)}
        className={`px-2.5 py-1 rounded-md font-semibold transition-colors ${value === o.value ? 'bg-white dark:bg-slate-900 shadow-sm text-slate-900 dark:text-slate-100' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'}`}>
        {o.label}
      </button>
    ))}
  </div>
)

export const HealthDot = ({ status }) => {
  const map = { ok: 'bg-emerald-500', warn: 'bg-amber-500', err: 'bg-red-500' }
  return <span className={`inline-block w-2 h-2 rounded-full ${map[status] || 'bg-slate-400'}`} />
}

/* Estado vacío — tal como lo muestra el prototipo: icono en cuadro azul suave,
 * título, una línea de explicación y la acción que lo resuelve (azul, no verde).
 * «Toda lista define su vacío con una acción que lo resuelve, nunca una tabla en
 * blanco» (03 §Convenciones).
 *
 * El racimo modular NO va aquí: el sistema gráfico es «solo textura en
 * superficies de marca; nunca sobre áreas de trabajo con datos» (03 §1.4). */
export const Empty = ({ icon, title, body, cta, framed = true }) => (
  <div className={`flex flex-col items-center justify-center py-14 px-6 text-center ${framed ? 'rounded-xl border border-dashed border-slate-300 dark:border-slate-700' : ''}`}>
    <div className="h-11 w-11 rounded-icon bg-elerp-50 text-elerp-500 dark:bg-elerp-900/40 dark:text-elerp-200 inline-flex items-center justify-center mb-3">
      {icon || <Icon.Package size={22} />}
    </div>
    <div className="font-display font-semibold text-[16px] text-slate-900 dark:text-slate-100">{title}</div>
    {body ? <div className="text-[13px] text-slate-500 mt-1 max-w-sm">{body}</div> : null}
    {cta ? <div className="mt-4">{cta}</div> : null}
  </div>
)

// Skeleton para estados de carga (regla UX: diseñar el estado "cargando").
export const Skeleton = ({ className = '' }) => <div className={`skeleton ${className}`} />

export const TableSkeleton = ({ rows = 6, cols = 4 }) => (
  <div className="space-y-2 p-1">
    {Array.from({ length: rows }).map((_, r) => (
      <div key={r} className="flex gap-3">
        {Array.from({ length: cols }).map((_, c) => (
          <Skeleton key={c} className={`h-8 ${c === 0 ? 'w-1/3' : 'flex-1'}`} />
        ))}
      </div>
    ))}
  </div>
)

/* ---- Toasts ---- */
const ToastCtx = createContext(null)
export const useToast = () => useContext(ToastCtx) || (() => {})
export const ToastProvider = ({ children }) => {
  const [list, setList] = useState([])
  const push = useCallback((t) => {
    const id = Math.random().toString(36).slice(2)
    setList((l) => [...l, { id, ...t }])
    setTimeout(() => setList((l) => l.filter((x) => x.id !== id)), t.duration || 4200)
  }, [])
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="fixed top-4 right-4 z-[200] flex flex-col gap-2 w-80">
        {list.map((t) => (
          <div key={t.id} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-xl shadow-modal p-3 flex items-start gap-3 fadein">
            <span className={`mt-0.5 ${t.kind === 'error' ? 'text-red-500' : t.kind === 'warn' ? 'text-amber-500' : 'text-emerald-500'}`}>
              {t.kind === 'error' ? <Icon.CircleX /> : t.kind === 'warn' ? <Icon.CircleAlert /> : <Icon.CircleCheck />}
            </span>
            <div className="flex-1 min-w-0">
              <div className="text-sm font-medium">{t.title}</div>
              {t.body ? <div className="text-[12.5px] text-slate-500 mt-0.5">{t.body}</div> : null}
            </div>
            <button onClick={() => setList((l) => l.filter((x) => x.id !== t.id))} className="text-slate-400 hover:text-slate-600"><Icon.X size={14} /></button>
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  )
}

/* ---- Accesibilidad compartida de diálogos (Modal y Drawer) ----
 * Un diálogo accesible no solo escucha Escape: anuncia su rol (role="dialog" +
 * aria-modal), atrapa el foco (Tab cicla dentro, nunca se escapa al fondo) y
 * DEVUELVE el foco al elemento que lo abrió al cerrarse. Sigue el mismo patrón de
 * trampa de foco del Sidebar (cajón móvil). Las animaciones ya respetan
 * `prefers-reduced-motion` desde index.css, así que aquí no hay que repetirlo.
 *
 * Detalles que evitan regresiones:
 *  · No dependemos de `onClose` en las deps del efecto (suele ser una flecha en
 *    línea que cambia cada render); lo leemos por ref, así el efecto corre solo al
 *    abrir/cerrar y la restauración de foco no se dispara a media interacción.
 *  · El foco inicial respeta el `autoFocus` nativo de React (que ya corrió en el
 *    commit): solo movemos el foco si NO quedó dentro del panel. */
const FOCUSABLE_SEL = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
function useDialogA11y(open, onClose, panelRef) {
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose
  useEffect(() => {
    if (!open) return
    const opener = document.activeElement
    const panel = panelRef.current
    // Foco inicial: si React no dejó ya el foco dentro (autoFocus), lo llevamos al
    // primer elemento enfocable, o al propio panel como último recurso.
    if (panel && !panel.contains(document.activeElement)) {
      const foco = panel.querySelectorAll(FOCUSABLE_SEL)
      ;(foco[0] || panel).focus?.()
    }
    const onKey = (e) => {
      if (e.key === 'Escape') { e.preventDefault(); onCloseRef.current?.(); return }
      if (e.key !== 'Tab' || !panel) return
      const foco = panel.querySelectorAll(FOCUSABLE_SEL)
      if (!foco.length) { e.preventDefault(); return }
      const first = foco[0]
      const last = foco[foco.length - 1]
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus() }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      // Devuelve el foco al elemento que abrió el diálogo (si sigue en el DOM).
      if (opener && typeof opener.focus === 'function' && document.contains(opener)) opener.focus()
    }
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps
}

/* ---- Modal ---- */
export const Modal = ({ open, onClose, title, sub, children, footer, size = 'md', icon }) => {
  const panelRef = useRef(null)
  const titleId = useId()
  useDialogA11y(open, onClose, panelRef)
  if (!open) return null
  const sizes = { sm: 'max-w-md', md: 'max-w-2xl', lg: 'max-w-3xl', xl: 'max-w-5xl', pos: 'max-w-6xl' }
  return (
    <div className="fixed inset-0 z-[150] flex items-center justify-center p-6">
      <div className="absolute inset-0 bg-slate-900/30 backdrop-blur-sm backdrop-in" onClick={onClose} />
      <div ref={panelRef} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}
        className={`relative w-full ${sizes[size]} bg-white dark:bg-slate-900 rounded-xl shadow-modal border border-slate-200 dark:border-slate-800 modal-in max-h-[88vh] flex flex-col`}>
        <div className="flex items-start gap-3 px-6 pt-5 pb-4 border-b border-slate-100 dark:border-slate-800">
          {icon ? <span className="mt-0.5 h-9 w-9 rounded-lg bg-elerp-50 text-elerp-600 dark:bg-elerp-900/40 dark:text-elerp-300 inline-flex items-center justify-center">{icon}</span> : null}
          <div className="flex-1 min-w-0">
            <div id={titleId} className="text-[15px] font-semibold tracking-tight">{title}</div>
            {sub ? <div className="text-[12.5px] text-slate-500 mt-0.5">{sub}</div> : null}
          </div>
          <button onClick={onClose} aria-label="Cerrar" className="h-8 w-8 inline-flex items-center justify-center rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-500 ring-focus"><Icon.X size={16} /></button>
        </div>
        <div className="flex-1 overflow-auto px-6 py-5">{children}</div>
        {footer ? <div className="px-6 py-3 border-t border-slate-100 dark:border-slate-800 flex items-center justify-end gap-2 bg-slate-50/60 dark:bg-slate-900/60 rounded-b-xl">{footer}</div> : null}
      </div>
    </div>
  )
}

/* ---- Confirmación (diálogo compartido) ----
 * Mecanismo reutilizable sobre el Modal: cualquier pantalla obtiene con el hook
 * `useConfirm()` una función `confirm(opts) → Promise<boolean>`. Las acciones
 * destructivas piden confirmación con UN mismo diálogo, sin repetir markup.
 *   opts = { title, body, confirmLabel='Confirmar', cancelLabel='Cancelar',
 *            tone='danger'|'default', icon }
 * El botón de confirmar usa `variant='destructive'` cuando tone==='danger' (o
 * 'primary' si default); el de cancelar —que SOLO cierra, no ejecuta— queda
 * neutro (`ghost`), nunca rojo. Resuelve true al confirmar, false al cancelar
 * o cerrar (Esc / clic fuera / la X). */
const ConfirmCtx = createContext(null)
export const useConfirm = () => useContext(ConfirmCtx) || (async () => false)
export const ConfirmProvider = ({ children }) => {
  const [state, setState] = useState(null) // { opts, resolve } | null
  const confirm = useCallback((opts = {}) => new Promise((resolve) => {
    setState({ opts, resolve })
  }), [])
  const close = (result) => {
    if (state) state.resolve(result)
    setState(null)
  }
  const opts = state?.opts || {}
  const tone = opts.tone || 'danger'
  return (
    <ConfirmCtx.Provider value={confirm}>
      {children}
      <Modal open={!!state} onClose={() => close(false)} size="sm"
        title={opts.title || '¿Confirmar acción?'}
        icon={opts.icon || (tone === 'danger' ? <Icon.CircleAlert size={18} /> : undefined)}
        footer={<>
          <Button variant="ghost" onClick={() => close(false)}>{opts.cancelLabel || 'Cancelar'}</Button>
          <Button variant={tone === 'danger' ? 'destructive' : 'primary'} onClick={() => close(true)} autoFocus>{opts.confirmLabel || 'Confirmar'}</Button>
        </>}>
        {opts.body ? <div className="text-sm text-slate-600 dark:text-slate-300 leading-relaxed">{opts.body}</div> : null}
      </Modal>
    </ConfirmCtx.Provider>
  )
}

/* ---- Drawer ---- */
export const Drawer = ({ open, onClose, title, sub, children, width = 480, footer, icon }) => {
  const panelRef = useRef(null)
  const titleId = useId()
  useDialogA11y(open, onClose, panelRef)
  if (!open) return null
  return (
    <div className="fixed inset-0 z-[150] flex justify-end">
      <div className="absolute inset-0 bg-slate-900/30 backdrop-blur-sm backdrop-in" onClick={onClose} />
      <div ref={panelRef} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}
        className="relative bg-white dark:bg-slate-900 border-l border-slate-200 dark:border-slate-800 shadow-modal drawer-in flex flex-col max-w-full" style={{ width }}>
        <div className="flex items-start gap-3 px-5 pt-5 pb-4 border-b border-slate-100 dark:border-slate-800">
          {icon ? <span className="mt-0.5 h-9 w-9 rounded-lg bg-slate-100 dark:bg-slate-800 inline-flex items-center justify-center">{icon}</span> : null}
          <div className="flex-1 min-w-0">
            <div id={titleId} className="text-[15px] font-semibold tracking-tight">{title}</div>
            {sub ? <div className="text-[12.5px] text-slate-500 mt-0.5">{sub}</div> : null}
          </div>
          <button onClick={onClose} aria-label="Cerrar" className="h-8 w-8 inline-flex items-center justify-center rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-500 ring-focus"><Icon.X size={16} /></button>
        </div>
        <div className="flex-1 overflow-auto px-5 py-5">{children}</div>
        {footer ? <div className="px-5 py-3 border-t border-slate-100 dark:border-slate-800 flex items-center justify-end gap-2">{footer}</div> : null}
      </div>
    </div>
  )
}

/* ---- Vista de detalle a pantalla completa ----
 * Reemplaza al Drawer para las VISUALIZACIONES de detalle: en vez de superponer
 * un panel angosto a la lista, el detalle ocupa el ancho principal de la pantalla
 * (patrón maestro↔detalle estilo Odoo). Mismo encabezado que el alta a pantalla
 * completa de Cotizaciones: botón «‹ Volver» a la izquierda, ícono + título + sub,
 * y las acciones a la derecha. El cuerpo va en un contenedor cómodo (por defecto
 * max-w-5xl), nunca en una columna tipo drawer. Línea de marca, dark-mode y
 * responsive. Se usa igual en todas las pantallas para que se vean idénticas. */
export const VistaDetalle = ({ onVolver, icon, titulo, sub, acciones, children, maxWidth = 'max-w-5xl' }) => (
  <div>
    <div className="flex flex-wrap items-center gap-3 mb-4 pb-3 border-b border-slate-200 dark:border-slate-800">
      <button onClick={onVolver}
        className="h-9 px-2.5 inline-flex items-center gap-1.5 rounded-lg text-[13px] font-medium text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus shrink-0">
        <Icon.ChevLeft size={16} /> Volver
      </button>
      {icon ? (
        <span className="h-9 w-9 rounded-lg bg-elerp-50 text-elerp-600 dark:bg-elerp-900/40 dark:text-elerp-300 inline-flex items-center justify-center shrink-0">{icon}</span>
      ) : null}
      <div className="flex-1 min-w-0">
        <h2 className="font-display text-[20px] leading-tight font-bold tracking-tight truncate">{titulo}</h2>
        {sub ? <div className="text-[12px] text-slate-500 truncate">{sub}</div> : null}
      </div>
      {acciones ? <div className="flex items-center gap-2 flex-wrap shrink-0">{acciones}</div> : null}
    </div>
    <div className={maxWidth}>{children}</div>
  </div>
)

// Etiqueta de campo de formulario reutilizable.
export const Field = ({ label, hint, error, children, required }) => (
  <div>
    <label className="block text-[12px] font-medium text-slate-500 mb-1">
      {label} {required ? <span className="text-red-400">*</span> : null}
      {hint ? <span className="text-slate-400 font-normal"> · {hint}</span> : null}
    </label>
    {children}
    {error ? <div className="mt-1 flex items-start gap-1 text-[11.5px] text-red-600 dark:text-red-400"><Icon.CircleAlert size={13} className="mt-0.5 shrink-0" /><span>{error}</span></div> : null}
  </div>
)
