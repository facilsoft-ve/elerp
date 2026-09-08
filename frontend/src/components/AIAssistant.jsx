import { useState, useEffect, useRef, useCallback } from 'react'
import { Icon } from './Icon.jsx'
import { Logo } from './Logo.jsx'
import { ModularCorner, HubDot } from './brand.jsx'
import { useData } from '../context/DataContext.jsx'
import { api } from '../lib/api.js'

// Asistente del ERP como ciudadano de primera clase (regla UX: la IA a un clic desde
// cualquier pantalla). Es el frente del módulo instalable "asistente-ia": si el módulo
// no está activo, no se monta nada.
//
// Responde en DOS capas (backend POST /api/ai/ask):
//   · MECÁNICA — 100% local, exacta, sobre los datos de la instancia y acotada al rol
//     (ventas, cartera, stock, por pagar, integridad) + ayudas del producto.
//   · IA — opt-in por empresa (Configuración › Asistente): solo las preguntas abiertas,
//     con un contexto acotado al rol y sin navegar internet.
// Cada respuesta trae su procedencia (tipo/fuente/modelo) y se muestra como badge, para
// que nunca se confunda un dato del ledger con una opinión del modelo.
const SUGERENCIAS = [
  { icon: Icon.Chart, text: '¿Cuánto vendí este mes?' },
  { icon: Icon.Wallet, text: '¿Quién me debe? Cartera vencida' },
  { icon: Icon.Boxes, text: '¿Qué productos tienen stock bajo?' },
  { icon: Icon.Receipt, text: '¿Qué es el IGTF?' },
  { icon: Icon.ArrowLeftRight, text: '¿Cómo transfiero stock entre sedes?' },
]

const ahora = () =>
  new Date().toLocaleTimeString('es-VE', { hour: '2-digit', minute: '2-digit' })

export function AIAssistant() {
  const { db } = useData()
  // Gating de cliente: el asistente es el módulo "asistente-ia". El servidor gatea
  // igual (/ai/* da 403 sin el módulo); esto evita mostrar el botón cuando no aplica.
  const instalado = Array.isArray(db?.MODULOS) && db.MODULOS.includes('asistente-ia')

  const [open, setOpen] = useState(false)
  const [messages, setMessages] = useState([])
  const [input, setInput] = useState('')
  const [pensando, setPensando] = useState(false)
  const scrollRef = useRef(null)
  const inputRef = useRef(null)

  const send = useCallback(async (text) => {
    const q = (typeof text === 'string' ? text : input).trim()
    if (!q || pensando) return
    setInput('')
    setPensando(true)
    setMessages((m) => [...m, { role: 'user', text: q, at: ahora() }])
    try {
      const r = await api.aiAsk(q)
      setMessages((m) => [...m, {
        role: 'assistant', text: r.respuesta, tipo: r.tipo, fuente: r.fuente, modelo: r.modelo, at: ahora(),
      }])
    } catch {
      setMessages((m) => [...m, {
        role: 'assistant', tipo: 'error', at: ahora(),
        text: 'No pude responder en este momento. Reintenta en un momento, o reformula tu pregunta como una consulta de datos.',
      }])
    } finally {
      setPensando(false)
    }
  }, [input, pensando])

  // Puente con el Dashboard: la tarjeta "Asistente" emite `huberp:ia` con la pregunta;
  // aquí se abre el panel y se envía. Se registra siempre que el módulo esté activo.
  useEffect(() => {
    if (!instalado) return
    const onAsk = (e) => {
      setOpen(true)
      const q = e.detail
      if (typeof q === 'string' && q.trim()) send(q)
    }
    window.addEventListener('huberp:ia', onAsk)
    return () => window.removeEventListener('huberp:ia', onAsk)
  }, [instalado, send])

  // Cerrar con Escape y enfocar el campo al abrir.
  useEffect(() => {
    if (!open) return
    const onKey = (e) => { if (e.key === 'Escape') setOpen(false) }
    window.addEventListener('keydown', onKey)
    const t = setTimeout(() => inputRef.current?.focus(), 120)
    return () => { window.removeEventListener('keydown', onKey); clearTimeout(t) }
  }, [open])

  // Autoscroll al último mensaje.
  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [messages, open, pensando])

  // Sin el módulo activo, el asistente no existe en la UI.
  if (!instalado) return null

  const vacio = messages.length === 0

  return (
    <>
      {/* ── BOTÓN FLOTANTE (FAB) ───────────────────────────────────────────── */}
      {!open && (
        <button
          onClick={() => setOpen(true)}
          aria-label="Abrir asistente de ElERP"
          title="Asistente ElERP"
          className="group fixed bottom-5 right-5 z-[120] h-14 w-14 rounded-full ring-focus
            inline-flex items-center justify-center text-white
            bg-gradient-to-br from-elerp-500 to-elerp-700
            shadow-pop ring-1 ring-white/10 hover:shadow-brand
            transition-transform duration-200 hover:scale-105 active:scale-95"
        >
          <span aria-hidden="true"
            className="pointer-events-none absolute inset-0 rounded-full bg-teal-500/25
              motion-safe:animate-ping opacity-70 group-hover:opacity-0 transition-opacity" />
          <span aria-hidden="true" className="pointer-events-none absolute inset-0 rounded-full ring-2 ring-teal-400/40" />
          <Icon.Sparkles size={22} className="relative" />
          <HubDot size={11} ring className="absolute -top-0.5 -right-0.5" />
        </button>
      )}

      {/* ── PANEL DEL CHAT ──────────────────────────────────────────────── */}
      {open && (
        <div className="fixed inset-0 z-[150] flex justify-end" role="dialog" aria-modal="true" aria-label="Asistente ElERP">
          <div className="absolute inset-0 bg-slate-900/30 backdrop-blur-sm backdrop-in" onClick={() => setOpen(false)} />

          <div className="relative flex flex-col max-w-full w-[420px] drawer-in
            bg-slate-50 dark:bg-slate-950 border-l border-slate-200 dark:border-slate-800 shadow-modal">

            {/* CABECERA de marca — hero navy con degradado + esquina modular. */}
            <div className="relative overflow-hidden shrink-0
              bg-gradient-to-br from-elerp-600 via-elerp-500 to-elerp-700 text-white
              px-5 pt-5 pb-4">
              <ModularCorner corner="tr" variant="full" tone="dark" className="opacity-90" />
              <div className="relative flex items-start gap-3">
                <span className="h-10 w-10 shrink-0 rounded-xl bg-white/95 shadow-brand inline-flex items-center justify-center">
                  <Logo size={22} />
                </span>
                <div className="flex-1 min-w-0 pt-0.5">
                  <div className="flex items-center gap-2">
                    <h2 className="font-display font-bold text-[16px] leading-tight tracking-tight">Asistente ElERP</h2>
                    <HubDot size={7} />
                  </div>
                  <p className="text-[12px] text-white/70 mt-0.5 leading-snug">Responde con tus datos · sin salir de tu empresa</p>
                </div>
                <button onClick={() => setOpen(false)} aria-label="Cerrar asistente"
                  className="h-8 w-8 shrink-0 inline-flex items-center justify-center rounded-lg
                    text-white/80 hover:text-white hover:bg-white/15 ring-focus transition-colors">
                  <Icon.X size={17} />
                </button>
              </div>
              <div className="relative mt-3 inline-flex items-center gap-1.5 rounded-full
                bg-white/12 border border-white/15 px-2.5 py-1 text-[11.5px] font-medium text-white/90">
                <span className="w-1.5 h-1.5 rounded-full bg-teal-300" />
                Cálculos exactos del sistema · IA opcional para preguntas abiertas
              </div>
            </div>

            {/* ÁREA DE MENSAJES */}
            <div ref={scrollRef} className="flex-1 overflow-auto px-4 py-5">
              {vacio ? (
                <div className="fadein flex flex-col items-center text-center px-2 pt-3">
                  <span className="h-16 w-16 rounded-2xl bg-gradient-to-br from-elerp-500 to-elerp-700
                    shadow-pop inline-flex items-center justify-center ring-1 ring-white/10">
                    <Logo size={34} mono />
                  </span>
                  <h3 className="font-display font-bold text-[18px] tracking-tight text-slate-900 dark:text-slate-50 mt-4">
                    Hola, ¿en qué te ayudo?
                  </h3>
                  <p className="text-[13px] text-slate-500 dark:text-slate-400 leading-relaxed mt-1.5 max-w-[19rem]">
                    Preguntame por tus ventas, cobranzas, stock o cumplimiento fiscal. Respondo con los datos de tu empresa.
                  </p>

                  <div className="w-full flex flex-col gap-2 mt-5">
                    {SUGERENCIAS.map(({ icon: Glyph, text }) => (
                      <button key={text} onClick={() => send(text)}
                        className="group flex items-center gap-2.5 w-full text-left rounded-xl px-3 py-2.5
                          bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800
                          hover:border-elerp-300 dark:hover:border-elerp-500/60 hover:shadow-brand
                          ring-focus transition-all">
                        <span className="h-7 w-7 shrink-0 rounded-lg inline-flex items-center justify-center
                          bg-elerp-50 text-elerp-500 dark:bg-elerp-900/50 dark:text-elerp-200
                          group-hover:bg-teal-50 group-hover:text-teal-600 dark:group-hover:bg-teal-500/15 dark:group-hover:text-teal-300 transition-colors">
                          <Glyph size={15} stroke={1.8} />
                        </span>
                        <span className="text-[13px] font-medium text-slate-700 dark:text-slate-200 flex-1 min-w-0">{text}</span>
                        <Icon.ArrowRight size={14} className="shrink-0 text-slate-300 dark:text-slate-600 group-hover:text-elerp-500 dark:group-hover:text-elerp-200 transition-colors" />
                      </button>
                    ))}
                  </div>
                </div>
              ) : (
                <div className="space-y-4">
                  {messages.map((m, i) => (
                    <Burbuja key={i} m={m} />
                  ))}
                  {pensando ? <Pensando /> : null}
                </div>
              )}
            </div>

            {/* INPUT on-brand */}
            <div className="shrink-0 border-t border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 px-4 py-3">
              <div className="flex items-end gap-2">
                <input ref={inputRef} value={input} onChange={(e) => setInput(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() } }}
                  placeholder="Escribe tu pregunta…"
                  aria-label="Mensaje para el asistente"
                  disabled={pensando}
                  className="flex-1 h-11 px-3.5 rounded-xl text-sm disabled:opacity-60
                    text-slate-900 dark:text-slate-100 placeholder:text-slate-400 dark:placeholder:text-slate-500
                    bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-700
                    focus:border-elerp-500 ring-focus transition-colors" />
                <button onClick={() => send()} disabled={!input.trim() || pensando} aria-label="Enviar mensaje"
                  className="h-11 w-11 shrink-0 rounded-xl inline-flex items-center justify-center text-white
                    bg-elerp-500 hover:bg-elerp-600 active:bg-elerp-700
                    disabled:bg-slate-200 disabled:text-slate-400 dark:disabled:bg-slate-800 dark:disabled:text-slate-600
                    ring-focus transition-colors">
                  <Icon.Send size={17} />
                </button>
              </div>
              <p className="text-[11px] text-slate-400 dark:text-slate-500 mt-2 text-center">
                Las cifras salen de tu ledger. Verifica siempre los datos fiscales antes de declarar.
              </p>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

// Badge de PROCEDENCIA: distingue a simple vista un dato del ledger (mecánica) de una
// respuesta de IA o una guía. Es el pacto de confianza del asistente.
function Procedencia({ m }) {
  if (m.tipo === 'ia') {
    return (
      <span className="inline-flex items-center gap-1 text-[10.5px] font-medium text-elerp-600 dark:text-elerp-300">
        <Icon.Sparkles size={11} /> IA{m.modelo ? ` · ${m.modelo.split('/').pop()}` : ''}
      </span>
    )
  }
  if (m.tipo === 'sin_respuesta') {
    return (
      <span className="inline-flex items-center gap-1 text-[10.5px] font-medium text-amber-600 dark:text-amber-400">
        <span className="w-1 h-1 rounded-full bg-amber-500" /> sugerencia
      </span>
    )
  }
  if (m.tipo === 'error') {
    return (
      <span className="inline-flex items-center gap-1 text-[10.5px] font-medium text-red-600 dark:text-red-400">
        <span className="w-1 h-1 rounded-full bg-red-500" /> sin conexión
      </span>
    )
  }
  if (m.tipo === 'mecanica') {
    return (
      <span className="inline-flex items-center gap-1 text-[10.5px] font-medium text-teal-600 dark:text-teal-300">
        <Icon.Boxes size={11} /> {m.fuente || 'tus datos'}
      </span>
    )
  }
  return null
}

/* Burbuja de mensaje on-brand. */
function Burbuja({ m }) {
  const user = m.role === 'user'
  return (
    <div className={`fadein flex gap-2.5 ${user ? 'justify-end' : ''}`}>
      {!user && (
        <span className="h-7 w-7 shrink-0 mt-0.5 rounded-lg inline-flex items-center justify-center
          bg-elerp-50 text-elerp-500 dark:bg-elerp-900/50 dark:text-elerp-200">
          <Icon.Sparkles size={14} />
        </span>
      )}
      <div className={`max-w-[82%] min-w-0 ${user ? 'items-end' : 'items-start'} flex flex-col gap-1`}>
        <div className={`text-[13px] leading-relaxed px-3.5 py-2.5 shadow-card whitespace-pre-wrap
          ${user
            ? 'bg-elerp-500 text-white rounded-2xl rounded-br-md'
            : 'bg-white dark:bg-slate-900 text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-800 rounded-2xl rounded-tl-md'}`}>
          {m.text}
        </div>
        <div className={`flex items-center gap-1.5 px-1 ${user ? 'flex-row-reverse' : ''}`}>
          {m.at ? <span className="text-[10.5px] text-slate-400 dark:text-slate-500 tabular-nums">{m.at}</span> : null}
          {!user ? <Procedencia m={m} /> : null}
        </div>
      </div>
    </div>
  )
}

/* Indicador de "pensando" mientras se consulta el backend. */
function Pensando() {
  return (
    <div className="fadein flex gap-2.5">
      <span className="h-7 w-7 shrink-0 mt-0.5 rounded-lg inline-flex items-center justify-center
        bg-elerp-50 text-elerp-500 dark:bg-elerp-900/50 dark:text-elerp-200">
        <Icon.Sparkles size={14} />
      </span>
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-2xl rounded-tl-md px-3.5 py-3 shadow-card">
        <span className="flex items-center gap-1">
          <span className="w-1.5 h-1.5 rounded-full bg-slate-400 dark:bg-slate-500 motion-safe:animate-bounce" style={{ animationDelay: '0ms' }} />
          <span className="w-1.5 h-1.5 rounded-full bg-slate-400 dark:bg-slate-500 motion-safe:animate-bounce" style={{ animationDelay: '150ms' }} />
          <span className="w-1.5 h-1.5 rounded-full bg-slate-400 dark:bg-slate-500 motion-safe:animate-bounce" style={{ animationDelay: '300ms' }} />
        </span>
      </div>
    </div>
  )
}
