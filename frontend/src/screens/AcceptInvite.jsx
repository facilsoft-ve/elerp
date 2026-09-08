import { useState, useEffect } from 'react'
import { Wordmark } from '../components/Logo.jsx'
import { Icon } from '../components/Icon.jsx'
import { api } from '../lib/api.js'
import { useAuth } from '../context/AuthContext.jsx'
import { ROLE_LABELS } from '../components/Topbar.jsx'

// Abre el link de invitación (…/?invite=<token>): muestra a qué empresa/organización
// te invitaron y deja aceptar con Hubmy o creando nombre + contraseña.
export function AcceptInvite({ token }) {
  const { login, refresh } = useAuth()
  const [preview, setPreview] = useState(null)
  const [loading, setLoading] = useState(true)
  const [invalid, setInvalid] = useState(false)
  const [nombre, setNombre] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    api.invitePreview(token)
      .then((p) => setPreview(p))
      .catch(() => setInvalid(true))
      .finally(() => setLoading(false))
  }, [token])

  async function accept(e) {
    e.preventDefault()
    if (!password || password.length < 8) { setError('La contraseña debe tener al menos 8 caracteres.'); return }
    setBusy(true)
    setError(null)
    try {
      await api.acceptInvite({ token, nombre: nombre.trim(), password })
      window.history.replaceState({}, '', window.location.pathname)
      await refresh()
    } catch (err) {
      setError(err?.message || 'No se pudo crear la cuenta.')
      setBusy(false)
    }
  }

  const field = 'w-full h-11 px-3 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm outline-none focus:ring-2 focus:ring-elerp-400'

  return (
    <div className="min-h-screen flex items-center justify-center bg-elerp-500 p-6">
      <div className="w-full max-w-sm bg-white dark:bg-slate-900 rounded-2xl shadow-modal p-8">
        <Wordmark size={32} className="mb-6" />

        {loading ? (
          <div className="text-sm text-slate-500">Cargando invitación…</div>
        ) : invalid ? (
          <div>
            <div className="flex items-start gap-2 text-[13px] text-rose-600 dark:text-rose-400 mb-4">
              <Icon.CircleX size={16} className="mt-0.5 shrink-0" />
              <span>Esta invitación no es válida o ya fue utilizada.</span>
            </div>
            <a href={window.location.pathname} className="text-[13px] text-elerp-600 font-medium">Ir al inicio de sesión</a>
          </div>
        ) : (
          <>
            <p className="text-sm text-slate-600 dark:text-slate-300 mb-1">
              Te sumaron a <b>{preview.empresaNombre || preview.orgNombre}</b> como <b>{ROLE_LABELS[preview.rol] || preview.rol}</b>.
            </p>
            <p className="text-[12px] text-slate-500 mb-5">Invitación para {preview.email}</p>

            {preview.hubmy ? (
              <>
                <button onClick={login}
                  className="w-full h-11 rounded-xl bg-elerp-500 hover:bg-elerp-600 text-white font-medium inline-flex items-center justify-center gap-2 transition-colors">
                  <Icon.Shield size={18} /> Entrar con Hubmy
                </button>
                <div className="my-4 flex items-center gap-3 text-[11px] uppercase tracking-wide text-slate-400">
                  <div className="h-px flex-1 bg-slate-200 dark:bg-slate-700" /> o crea una contraseña <div className="h-px flex-1 bg-slate-200 dark:bg-slate-700" />
                </div>
              </>
            ) : null}

            <form onSubmit={accept} className="space-y-2.5">
              <input className={field} type="text" autoComplete="name" placeholder="Tu nombre"
                value={nombre} onChange={(e) => setNombre(e.target.value)} />
              <input className={field} type="password" autoComplete="new-password" placeholder="Contraseña (mín. 8 caracteres)"
                value={password} onChange={(e) => setPassword(e.target.value)} />
              {error ? (
                <div className="flex items-start gap-2 text-[12.5px] text-rose-600 dark:text-rose-400">
                  <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" /> <span>{error}</span>
                </div>
              ) : null}
              <button type="submit" disabled={busy || password.length < 8}
                className="w-full h-11 rounded-xl bg-slate-900 dark:bg-slate-100 dark:text-slate-900 text-white font-medium inline-flex items-center justify-center gap-2 disabled:opacity-60 transition-colors">
                {busy ? 'Creando cuenta…' : <>Crear cuenta y entrar <Icon.ArrowRight size={17} /></>}
              </button>
            </form>
          </>
        )}
      </div>
    </div>
  )
}
