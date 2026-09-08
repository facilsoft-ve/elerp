import { useCallback, useEffect, useState } from 'react'
import { api } from '../lib/api.js'
import { Wordmark } from './Logo.jsx'

// Muro de aceptación de los documentos legales (Términos y Condiciones + Política de
// Privacidad). Bloquea el uso de la app hasta que el usuario acepte la versión VIGENTE.
// Cubre onboarding, usuarios invitados y re-aceptación cuando cambia la versión (el
// servidor calcula lo pendiente). La pantalla del cliente (ventana secundaria) no se
// gatea: la sesión ya aceptó en la ventana principal.
export function LegalGate({ children }) {
  const [estado, setEstado] = useState('cargando') // cargando | ok | pendiente
  const [pendientes, setPendientes] = useState([])

  const cargar = useCallback(async () => {
    try {
      const r = await api.legalEstado()
      const p = r.pendientes || []
      setPendientes(p)
      setEstado(p.length ? 'pendiente' : 'ok')
    } catch {
      // Fail-open: un fallo transitorio no debe dejar al usuario fuera de su app.
      setEstado('ok')
    }
  }, [])
  useEffect(() => { cargar() }, [cargar])

  if (estado === 'cargando') {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center gap-3 bg-slate-50 dark:bg-slate-950">
        <Wordmark size={40} />
        <div className="text-sm text-slate-500">Verificando términos…</div>
      </div>
    )
  }
  if (estado === 'pendiente') return <AceptacionLegal pendientes={pendientes} onListo={cargar} />
  return children
}

// renderMd es un convertidor de markdown MÍNIMO para los documentos legales (encabezados,
// negritas, enlaces, listas, citas, reglas). Escapa el HTML primero (el contenido viene de
// nuestro backend, pero igual se sanea). Suficiente para estos textos.
function renderMd(md) {
  const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const inline = (s) =>
    esc(s)
      .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer" class="text-elerp-600 dark:text-elerp-300 underline">$1</a>')
  const out = []
  let lista = null
  let cita = null
  const cerrarLista = () => { if (lista) { out.push(`<ul class="list-disc pl-5 space-y-1">${lista.join('')}</ul>`); lista = null } }
  const cerrarCita = () => { if (cita) { out.push(`<blockquote class="border-l-2 border-amber-400 pl-3 my-2 text-[12.5px] text-amber-800 dark:text-amber-200 bg-amber-50/60 dark:bg-amber-950/30 px-3 py-2 rounded-r space-y-1">${cita.join('')}</blockquote>`); cita = null } }
  const cerrar = () => { cerrarLista(); cerrarCita() }
  for (const raw of md.split('\n')) {
    const l = raw.trimEnd()
    if (/^>\s?/.test(l)) {
      cerrarLista()
      // Dentro de la cita, un encabezado (### x) se muestra en negrita, no con «###».
      const c = l.replace(/^>\s?/, '').replace(/^#{1,6}\s+(.*)$/, '**$1**')
      if (c.trim() !== '') (cita = cita || []).push(`<p class="my-0.5">${inline(c)}</p>`)
      continue
    }
    if (/^###\s+/.test(l)) { cerrar(); out.push(`<h3 class="font-display font-semibold text-[14.5px] mt-4 mb-1">${inline(l.replace(/^###\s+/, ''))}</h3>`) }
    else if (/^##\s+/.test(l)) { cerrar(); out.push(`<h2 class="font-display font-bold text-[16px] mt-5 mb-1.5">${inline(l.replace(/^##\s+/, ''))}</h2>`) }
    else if (/^#\s+/.test(l)) { cerrar(); out.push(`<h1 class="font-display font-bold text-[19px] mt-2 mb-2">${inline(l.replace(/^#\s+/, ''))}</h1>`) }
    else if (/^[-*]\s+/.test(l)) { cerrarCita(); (lista = lista || []).push(`<li>${inline(l.replace(/^[-*]\s+/, ''))}</li>`) }
    else if (/^---+$/.test(l)) { cerrar(); out.push('<hr class="my-3 border-slate-200 dark:border-slate-800" />') }
    else if (l.trim() === '') { cerrar() }
    else { cerrar(); out.push(`<p class="my-1.5 leading-relaxed">${inline(l)}</p>`) }
  }
  cerrar()
  return out.join('')
}

function AceptacionLegal({ pendientes, onListo }) {
  const [docs, setDocs] = useState(null)
  const [acepto, setAcepto] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    api.legalVigente().then((r) => setDocs(r.documentos || [])).catch(() => setDocs([]))
  }, [])

  const pendIds = new Set(pendientes.map((p) => p.documento))
  const mostrar = (docs || []).filter((d) => pendIds.has(d.documento))

  const aceptar = async () => {
    setBusy(true); setError('')
    try {
      for (const d of mostrar) await api.legalAceptar(d.documento)
      await onListo()
    } catch (e) {
      setError(e?.message || 'No se pudo registrar la aceptación. Reintenta.')
    } finally { setBusy(false) }
  }

  return (
    <div className="min-h-screen flex flex-col bg-slate-50 dark:bg-slate-950">
      {/* Cabecera de marca */}
      <div className="shrink-0 bg-gradient-to-br from-elerp-600 via-elerp-500 to-elerp-700 text-white px-5 py-4">
        <div className="max-w-3xl mx-auto flex items-center gap-3">
          <Wordmark size={30} mono />
          <div className="ml-auto text-[12.5px] text-white/80">Antes de continuar</div>
        </div>
      </div>

      <div className="flex-1 overflow-hidden">
        <div className="max-w-3xl mx-auto h-full flex flex-col px-4 py-5">
          <h1 className="font-display font-bold text-lg text-slate-900 dark:text-slate-50">Términos y privacidad</h1>
          <p className="text-[13px] text-slate-500 dark:text-slate-400 mt-1">
            Para usar ElERP necesitás leer y aceptar {mostrar.length > 1 ? 'los siguientes documentos' : 'el siguiente documento'}.
            Tu aceptación queda registrada de forma auditable (versión, fecha y hora).
          </p>

          {docs === null ? (
            <div className="flex-1 flex items-center justify-center text-slate-500">Cargando documentos…</div>
          ) : (
            <div className="flex-1 min-h-0 mt-4 space-y-4 overflow-auto pr-1">
              {mostrar.map((d) => (
                <div key={d.documento} className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-card">
                  <div className="px-4 py-2.5 border-b border-slate-100 dark:border-slate-800 flex items-center justify-between">
                    <span className="font-display font-semibold text-[14px] text-slate-800 dark:text-slate-100">{d.titulo}</span>
                    <span className="text-[11px] text-slate-400 font-mono">v{d.version}</span>
                  </div>
                  <div className="px-4 py-3 max-h-[42vh] overflow-auto text-[13px] text-slate-700 dark:text-slate-300 legal-doc"
                    dangerouslySetInnerHTML={{ __html: renderMd(d.contenido || '') }} />
                </div>
              ))}
            </div>
          )}

          {/* Aceptación */}
          <div className="shrink-0 mt-4 pt-4 border-t border-slate-200 dark:border-slate-800">
            <label className="flex items-start gap-2.5 cursor-pointer select-none">
              <input type="checkbox" checked={acepto} onChange={(e) => setAcepto(e.target.checked)}
                className="mt-0.5 h-4 w-4 accent-teal-500 ring-focus rounded" />
              <span className="text-[13px] text-slate-700 dark:text-slate-200">
                He leído y acepto {mostrar.map((d, i) => (
                  <span key={d.documento}>
                    {i > 0 ? ' y ' : ''}
                    <a href="/legal" target="_blank" rel="noreferrer" className="font-medium text-elerp-600 dark:text-elerp-300 underline">{d.titulo}</a>
                  </span>
                ))}.
              </span>
            </label>
            {error ? <div className="text-[12.5px] text-red-600 dark:text-red-400 mt-2">{error}</div> : null}
            <div className="flex justify-end mt-3">
              <button onClick={aceptar} disabled={!acepto || busy || mostrar.length === 0}
                className="h-10 px-5 rounded-xl text-sm font-medium text-white ring-focus transition-colors
                  bg-teal-500 hover:bg-teal-600 active:bg-teal-700
                  disabled:bg-slate-200 disabled:text-slate-400 dark:disabled:bg-slate-800 dark:disabled:text-slate-600">
                {busy ? 'Registrando…' : 'Aceptar y continuar'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
