import { useState, useEffect } from 'react'
import { Wordmark } from '../components/Logo.jsx'
import { RacimoModular } from '../components/brand.jsx'
import { Icon } from '../components/Icon.jsx'
import { BotonHubmy } from '../components/hubmy.jsx'
import { api } from '../lib/api.js'
import { useAuth } from '../context/AuthContext.jsx'


export function Login() {
  const { login, nativeLogin } = useAuth()
  /* A ElERP SE ENTRA CON HUBMY. Hubmy es el proveedor de identidad del producto
   * —ahí viven el 2FA, el SSO y la baja de un empleado—, y una segunda puerta
   * con contraseñas propias significa otro juego de credenciales que nadie rota
   * y que sobrevive al despido de quien la usaba.
   *
   * El formulario de email y contraseña sigue en el código pero solo aparece si
   * el SERVIDOR dice que esa puerta está abierta (AUTH_NATIVA, apagada por
   * defecto). Quien protege es el servidor: con el interruptor apagado la ruta
   * responde 404 aunque alguien arme la petición a mano. La pantalla solo
   * refleja la política, nunca la impone. */
  const [health, setHealth] = useState({ hubmy: false, devLogin: false, authNativa: false })
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    api.health()
      .then((h) => setHealth({ hubmy: !!h.hubmy, devLogin: !!h.devLogin, authNativa: !!h.authNativa }))
      .catch(() => {})
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
        {/* Racimo modular — el manual lo pide «a tamaño grande y CORTADO POR EL
            BORDE, no como confeti en las esquinas». Antes eran tres piezas
            chicas sueltas que caían justo encima del rótulo, del titular y del
            pie: se leían como manchas, no como textura de marca.
            Ahora son dos composiciones grandes, mordidas por el borde derecho
            —que es la franja vacía del panel— y lejos del texto, que va todo a
            la izquierda. */}
        <RacimoModular variante="grande" escala={2} style={{ right: -64, top: -28 }} />
        <RacimoModular variante="chico" escala={1.3} style={{ right: -26, bottom: 64 }} />

        <div className="relative flex items-center">
          <Wordmark size={30} mono />
        </div>

        <div className="relative">
          {/* Rótulo de la web: NO es la píldora de cualquier landing, es la BARRA
              sólida del wordmark —el bloque morado que corta la palabra— con el
              texto en mono. La barra es la misma figura del logo, a otra escala. */}
          <div className="flex items-center gap-2.5 mb-5">
            <span className="block rounded-sm" style={{ width: 26, height: 5, background: '#fff' }} />
            <span className="num text-[11.5px] font-semibold uppercase" style={{ letterSpacing: '.14em', color: '#D0BEF9' }}>
              ERP web para PyMEs venezolanas
            </span>
          </div>

          {/* El acento va en TINTE DE MARCA, no subrayado. El resaltador del sitio
              funciona sobre claro; acá, un blanco translúcido detrás de texto
              blanco se leía como un TACHADO sobre la frase. El tinte claro
              (elerp-300) da 5,96:1 sobre este fondo — de sobra para un titular. */}
          <h1 className="font-display font-bold text-[34px] leading-[1.2] max-w-[420px] m-0">
            Vende, factura y lleva tu contabilidad{' '}
            <span style={{ color: '#B295F4' }}>sin pelear con el sistema.</span>
          </h1>

          {/* En vez de tres promesas en viñetas, SE MUESTRA el producto: es la
              tarjeta del hero de la web, y dice más que cualquier lista. */}
          <div className="mt-8 rounded-2xl p-[18px] max-w-[360px]"
            style={{ background: 'rgba(255,255,255,.07)', border: '1px solid rgba(255,255,255,.12)', backdropFilter: 'blur(2px)' }}>
            <div className="flex items-center justify-between mb-3">
              <span className="font-display font-bold text-[14px]">Punto de Venta</span>
              <span className="text-[11px] font-semibold rounded-full px-2.5 py-[3px]"
                style={{ background: 'rgba(255,255,255,.16)', color: '#fff' }}>Caja abierta</span>
            </div>
            {[
              ['Harina de maíz 1 kg × 2', 'Bs 96,00'],
              ['Café molido 250 g', 'Bs 84,00'],
              ['Refresco 2 L', 'Bs 149,00'],
            ].map(([k, v]) => (
              <div key={k} className="flex items-center justify-between py-2 text-[13px]"
                style={{ borderTop: '1px solid rgba(255,255,255,.10)', color: '#D0BEF9' }}>
                <span>{k}</span><span className="num" style={{ color: '#fff' }}>{v}</span>
              </div>
            ))}
            <div className="flex items-center justify-between pt-2.5 mt-1 text-[14px] font-semibold"
              style={{ borderTop: '1px solid rgba(255,255,255,.22)' }}>
              <span>Total con IVA</span><span className="num">Bs 382,84</span>
            </div>
          </div>

          {/* Contraste medido sobre #2A2440: #D0BEF9 da 8,71:1. */}
          <div className="mt-5 text-[13px] max-w-[360px] leading-relaxed" style={{ color: '#D0BEF9' }}>
            Pensado para conectividad hostil: un corte de luz o de internet no detiene la caja.
          </div>
        </div>

        {/* Estaba en #8D5FF0 → 3,53:1, por debajo del 4,5:1 que exige AA para texto
            chico (regla del proyecto). #B295F4 da 5,96:1. */}
        <div className="relative text-[12.5px]" style={{ color: '#B295F4' }}>
          ERP web multi-empresa para Venezuela · por Mornix
        </div>
      </div>

      {/* Formulario (derecha) */}
      {/* El fondo es el del hero de la web: un halo de marca arriba a la derecha
          sobre blanco. Blanco plano al lado de un panel morado profundo se lee
          como un formulario suelto, no como la misma pieza. */}
      <div className="flex-1 flex items-center justify-center px-5 py-7 bg-hero-elerp">
        <div className="w-full max-w-[420px] fadein">
          <div className="lg:hidden mb-6 flex items-center">
            <Wordmark size={28} />
          </div>

          {/* Mismo rótulo de barra que el panel de marca, en su versión sobre
              claro: es la figura del logo, no un adorno distinto por pantalla. */}
          <div className="flex items-center gap-2.5 mb-3">
            <span className="block rounded-sm bg-elerp-500" style={{ width: 26, height: 5 }} />
            <span className="num text-[11px] font-semibold uppercase text-elerp-700 dark:text-elerp-300"
              style={{ letterSpacing: '.14em' }}>Acceso</span>
          </div>
          <div className="font-display font-bold text-[26px] text-slate-900 dark:text-slate-100 leading-tight">Hola de nuevo</div>
          <div className="text-slate-500 mt-1.5 text-[14.5px]">Entra para seguir facturando.</div>

          {/* Entrar con Hubmy: es LA puerta. Va solo, sin alternativas al lado,
              porque ofrecer dos caminos donde hay uno es lo que hace que la
              gente pruebe el equivocado primero. */}
          {health.hubmy ? <BotonHubmy onClick={login} className="mt-5" /> : null}

          {/* Sin Hubmy configurado la pantalla tiene que DECIRLO. Antes caía en
              el formulario de email y quien intentara entrar chocaría con
              «credenciales inválidas», que manda a buscar el problema en el
              lugar equivocado. */}
          {!health.hubmy && !health.authNativa ? (
            <div className="mt-5 rounded-lg px-3.5 py-3 flex gap-2.5 items-start text-[13px]"
              style={{ background: '#FBEDEB', border: '1px solid #ECC8C4', color: '#B3362C' }}>
              <Icon.CircleAlert size={16} className="shrink-0 mt-0.5" />
              <span>El acceso con Hubmy no está configurado en este servidor. Avisa a soporte: sin él no hay forma de entrar.</span>
            </div>
          ) : null}

          {error ? (
            <div className="mt-4 rounded-lg px-3 py-2.5 flex gap-2 items-start text-[13px]" style={{ background: '#FBEDEB', border: '1px solid #ECC8C4', color: '#B3362C' }}>
              <Icon.CircleAlert size={16} className="shrink-0 mt-0.5" />
              <span>{error}</span>
            </div>
          ) : null}

          {/* Email y contraseña: SOLO si el servidor declara esa puerta abierta
              (AUTH_NATIVA). Apagada por defecto — ver el comentario de arriba. */}
          {health.authNativa ? (
            <>
              {health.hubmy ? (
                <div className="my-4 flex items-center gap-3 text-[11px] uppercase tracking-wide text-slate-400">
                  <div className="h-px flex-1 bg-slate-200 dark:bg-slate-700" /> o con email <div className="h-px flex-1 bg-slate-200 dark:bg-slate-700" />
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
            </>
          ) : null}

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
