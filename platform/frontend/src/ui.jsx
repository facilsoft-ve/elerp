import { createContext, useContext, useState, useCallback } from 'react'

// Primitivas mínimas de la consola (mismo lenguaje visual que el ERP, sin arrastrar las
// 700 líneas del core). navy `huberp` estructura, `teal` señala acción/éxito/foco.

export function Button({ children, variant = 'primary', size = 'md', loading, icon, className = '', ...props }) {
  const sizes = { sm: 'h-8 px-3 text-[13px]', md: 'h-10 px-4 text-sm', lg: 'h-11 px-5 text-sm' }
  const variants = {
    primary: 'bg-teal-500 hover:bg-teal-600 active:bg-teal-700 text-white disabled:bg-slate-700 disabled:text-slate-400',
    navy: 'bg-elerp-500 hover:bg-elerp-600 text-white disabled:bg-slate-700 disabled:text-slate-400',
    ghost: 'bg-transparent hover:bg-slate-800 text-slate-200 border border-slate-700',
    danger: 'bg-red-600 hover:bg-red-700 text-white disabled:bg-slate-700',
  }
  return (
    <button {...props} disabled={props.disabled || loading}
      className={`inline-flex items-center justify-center gap-2 rounded-xl font-medium ring-focus transition-colors disabled:cursor-not-allowed ${sizes[size]} ${variants[variant]} ${className}`}>
      {loading ? <Spinner size={15} /> : icon}
      {children}
    </button>
  )
}

export function Input({ className = '', invalid, ...props }) {
  return (
    <input {...props}
      className={`w-full h-10 px-3 rounded-xl text-sm bg-slate-900 text-slate-100 placeholder:text-slate-500
        border ${invalid ? 'border-red-500' : 'border-slate-700 focus:border-teal-500'} ring-focus transition-colors ${className}`} />
  )
}

export function Select({ className = '', children, ...props }) {
  return (
    <select {...props}
      className={`w-full h-10 px-3 rounded-xl text-sm bg-slate-900 text-slate-100 border border-slate-700 focus:border-teal-500 ring-focus ${className}`}>
      {children}
    </select>
  )
}

export function Field({ label, hint, error, required, children }) {
  return (
    <label className="block">
      {label ? (
        <div className="flex items-baseline gap-1.5 mb-1">
          <span className="text-[12.5px] font-medium text-slate-300">{label}</span>
          {required ? <span className="text-teal-400">*</span> : null}
          {hint ? <span className="text-[11.5px] text-slate-500">· {hint}</span> : null}
        </div>
      ) : null}
      {children}
      {error ? <div className="text-[12px] text-red-400 mt-1">{error}</div> : null}
    </label>
  )
}

export function Toggle({ checked, onChange, label, sub }) {
  return (
    <button type="button" onClick={() => onChange(!checked)}
      className="flex items-start gap-3 w-full text-left ring-focus rounded-lg">
      <span className={`mt-0.5 shrink-0 w-10 h-6 rounded-full p-0.5 transition-colors ${checked ? 'bg-teal-500' : 'bg-slate-700'}`}>
        <span className={`block w-5 h-5 rounded-full bg-white transition-transform ${checked ? 'translate-x-4' : ''}`} />
      </span>
      <span className="min-w-0">
        <span className="block text-[13px] font-medium text-slate-200">{label}</span>
        {sub ? <span className="block text-[12px] text-slate-500">{sub}</span> : null}
      </span>
    </button>
  )
}

const BADGE = {
  teal: 'bg-teal-500/15 text-teal-300 border-teal-500/30',
  slate: 'bg-slate-700/40 text-slate-300 border-slate-600/50',
  amber: 'bg-amber-500/15 text-amber-300 border-amber-500/30',
  red: 'bg-red-500/15 text-red-300 border-red-500/40',
  emerald: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
}
export function Badge({ children, color = 'slate', className = '' }) {
  return <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11.5px] font-medium border ${BADGE[color] || BADGE.slate} ${className}`}>{children}</span>
}

export function Card({ children, className = '' }) {
  return <div className={`bg-slate-900 border border-slate-800 rounded-xl shadow-card ${className}`}>{children}</div>
}

export function Spinner({ size = 18 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" className="animate-spin" aria-hidden="true">
      <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="3" opacity="0.25" />
      <path d="M21 12a9 9 0 0 0-9-9" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  )
}

export function Empty({ title, body, cta }) {
  return (
    <div className="text-center py-14 px-6">
      <div className="text-[15px] font-semibold text-slate-200">{title}</div>
      {body ? <div className="text-[13px] text-slate-500 mt-1 max-w-md mx-auto">{body}</div> : null}
      {cta ? <div className="mt-4 flex justify-center">{cta}</div> : null}
    </div>
  )
}

// --- Modal ----------------------------------------------------------------
export function Modal({ open, onClose, title, children, footer, width = 'max-w-lg' }) {
  if (!open) return null
  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center p-4" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-slate-950/70 backdrop-blur-sm" onClick={onClose} />
      <div className={`relative w-full ${width} bg-slate-900 border border-slate-800 rounded-xl shadow-modal`}>
        <div className="flex items-center justify-between px-5 py-3.5 border-b border-slate-800">
          <h3 className="font-display font-bold text-[15px] text-slate-100">{title}</h3>
          <button onClick={onClose} aria-label="Cerrar" className="w-8 h-8 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 ring-focus">✕</button>
        </div>
        <div className="px-5 py-4">{children}</div>
        {footer ? <div className="px-5 py-3.5 border-t border-slate-800 flex justify-end gap-2">{footer}</div> : null}
      </div>
    </div>
  )
}

// --- Toasts ---------------------------------------------------------------
const ToastCtx = createContext(() => {})
export const useToast = () => useContext(ToastCtx)

export function ToastProvider({ children }) {
  const [items, setItems] = useState([])
  const push = useCallback((t) => {
    const id = Math.random().toString(36).slice(2)
    setItems((s) => [...s, { id, kind: 'ok', ...t }])
    setTimeout(() => setItems((s) => s.filter((x) => x.id !== id)), 4200)
  }, [])
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="fixed bottom-4 right-4 z-[200] flex flex-col gap-2 w-[340px] max-w-[92vw]">
        {items.map((t) => (
          <div key={t.id} className={`rounded-xl border px-3.5 py-3 shadow-pop text-sm ${t.kind === 'err' ? 'bg-red-950/80 border-red-800 text-red-100' : 'bg-slate-900 border-slate-700 text-slate-100'}`}>
            <div className="font-semibold">{t.title}</div>
            {t.body ? <div className="text-[12.5px] text-slate-400 mt-0.5">{t.body}</div> : null}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  )
}
