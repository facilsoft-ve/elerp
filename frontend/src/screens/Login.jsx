import { useState, useEffect } from 'react'
import { Wordmark } from '../components/Logo.jsx'
import { RacimoModular } from '../components/brand.jsx'
import { Icon } from '../components/Icon.jsx'
import { BotonHubmy } from '../components/hubmy.jsx'
import { api } from '../lib/api.js'
import { useAuth } from '../context/AuthContext.jsx'


export function Login() {
  const { login, nativeLogin } = useAuth()
  const [health, setHealth] = useState({ hubmy: false, devLogin: false })
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    api.health().then((h) => setHealth({ hubmy: !!h.hubmy, devLogin: !!h.devLogin })).catch(() => {})
  }, [])

  // La demo entra DIRECTO al dev-login: la captación del prospecto se hace por el
  // formulario de la landing (solicitud → correo + enlace temporal al aprobarla el
  // super_admin). Este botón es transitorio: el acceso a la demo pasará a ser por
  // solicitud aprobada, no un botón abierto.
  const irAlDemo = () => { window.location.href = api.devLoginURL() }

  async function submit(e) {
    e.preventDefault()
    if (!email.trim() || !password) return
    setBusy(true)
    setError(null)
    try {
      await nativeLogin({ email: email.trim(), password })
    } catch (err) {
      setError(err?.message || 'No se pudo iniciar sesión.')
      setBusy(false)
    }
  }

  const field = 'w-full px-3 py-2.5 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm text-slate-900 dark:text-slate-100 placeholder:text-slate-400 ring-focus focus:border-elerp-500'

  return (
    <div className="min-h-screen flex bg-white dark:bg-slate-950">
      {/* Panel de marca (izquierda) */}
      <div className="relative overflow-hidden hidden lg:flex w-[44%] max-w-[560px] shrink-0 flex-col justify-between p-14 text-white"
        style={{ background: 'var(--hb-azul-profundo)' }}>
        {/* Racimos modulares — textura de marca, tal como en el prototipo: dos
            composiciones en el tercio medio y una al pie. Nunca compiten en
            contraste con el contenido (opacidades del 9% al 16%). */}
        <RacimoModular variante="grande" style={{ left: 34, top: 168 }} />
        <RacimoModular variante="chico" style={{ right: 74, top: 150 }} />
        <RacimoModular variante="chico" escala={0.8} style={{ left: 286, bottom: 20 }} />

        <div className="relative flex items-center">
          <Wordmark size={30} mono />
        </div>
        <div className="relative">
          <div className="font-display font-bold text-[34px] leading-[1.2] max-w-[400px]">El ERP que habla venezolano.</div>
          <div className="mt-7 flex flex-col gap-4 text-[15px]" style={{ color: '#C7D4E8' }}>
            <div className="flex gap-3 items-start"><Icon.CircleCheck size={20} className="shrink-0 mt-0.5" /><span>Factura legal SENIAT en tres modalidades, sin ser contador</span></div>
            <div className="flex gap-3 items-start"><Icon.EyeOff size={20} className="shrink-0 mt-0.5" /><span>¿Se fue el internet? Sigues facturando en modo contingencia</span></div>
            <div className="flex gap-3 items-start"><Icon.Book size={20} className="shrink-0 mt-0.5" /><span>Tu contabilidad se lleva sola, asiento por asiento</span></div>
          </div>
        </div>
        <div className="relative text-[12.5px]" style={{ color: '#8FA5C6' }}>ERP web multi-empresa para Venezuela · por Mornix</div>
      </div>

      {/* Formulario (derecha) */}
      <div className="flex-1 flex items-center justify-center px-5 py-7">
        <div className="w-full max-w-[420px] fadein">
          <div className="lg:hidden mb-6 flex items-center">
            <Wordmark size={28} />
          </div>

          <div className="font-display font-bold text-[24px] text-slate-900 dark:text-slate-100">Hola de nuevo</div>
          <div className="text-slate-500 mt-1">Entra para seguir facturando.</div>

          {/* SSO de Hubmy: es el camino principal para un usuario real, así que va
              primero. Lleva la línea gráfica de Hubmy, no la de ElERP: un botón
              de «entrar con X» pertenece a la marca X. */}
          {health.hubmy ? <BotonHubmy onClick={login} className="mt-5" /> : null}

          {health.hubmy ? (
            <div className="my-4 flex items-center gap-3 text-[11px] uppercase tracking-wide text-slate-400">
              <div className="h-px flex-1 bg-slate-200 dark:bg-slate-700" /> o con email <div className="h-px flex-1 bg-slate-200 dark:bg-slate-700" />
            </div>
          ) : null}

          {error ? (
            <div className="mt-4 rounded-lg px-3 py-2.5 flex gap-2 items-start text-[13px]" style={{ background: '#FBEDEB', border: '1px solid #ECC8C4', color: '#B3362C' }}>
              <Icon.CircleAlert size={16} className="shrink-0 mt-0.5" />
              <span>{error}</span>
            </div>
          ) : null}

          <form onSubmit={submit} className="mt-5 space-y-3.5">
            <div>
              <label className="block text-[13px] font-semibold mb-1.5 text-slate-700 dark:text-slate-300">Correo electrónico</label>
              <input className={field} type="email" autoComplete="username" placeholder="tu@email.com"
                value={email} onChange={(e) => setEmail(e.target.value)} />
            </div>
            <div>
              <label className="block text-[13px] font-semibold mb-1.5 text-slate-700 dark:text-slate-300">Contraseña</label>
              <div className="relative">
                <input className={field + ' pr-11'} type={showPass ? 'text' : 'password'} autoComplete="current-password" placeholder="Contraseña"
                  value={password} onChange={(e) => setPassword(e.target.value)} />
                <button type="button" onClick={() => setShowPass((s) => !s)} title="Mostrar u ocultar contraseña"
                  className="absolute right-1.5 top-1/2 -translate-y-1/2 p-1.5 rounded-md text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800">
                  {showPass ? <Icon.EyeOff size={17} /> : <Icon.Eye size={17} />}
                </button>
              </div>
            </div>
            <button type="submit" disabled={busy || !email.trim() || !password}
              className="w-full rounded-lg bg-elerp-500 hover:bg-elerp-600 active:bg-elerp-700 text-white font-display font-medium py-3 text-[15px] inline-flex items-center justify-center gap-2 disabled:opacity-60 disabled:cursor-not-allowed transition-colors ring-focus">
              {busy ? <><span className="spin"><Icon.Refresh size={17} /></span> Entrando…</> : 'Entrar'}
            </button>
          </form>

          {health.devLogin ? (
            <button onClick={irAlDemo}
              className="mt-3 w-full rounded-lg border border-slate-300 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-[13.5px] font-semibold py-2.5 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors ring-focus">
              Entrar en modo demo
            </button>
          ) : null}

          <div className="mt-5 flex items-start gap-2 text-[12.5px] rounded-lg px-3 py-2.5" style={{ background: 'var(--hb-azul-suave)', color: 'var(--hb-azul)' }}>
            <Icon.Lock size={15} className="shrink-0 mt-0.5" />
            <span>{health.hubmy ? 'SSO seguro · 2FA/SSO gestionado por Hubmy.' : '2FA/SSO gestionado por Hubmy · acceso por invitación.'}</span>
          </div>
        </div>
      </div>
    </div>
  )
}
