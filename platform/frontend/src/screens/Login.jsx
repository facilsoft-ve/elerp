import logoUrl from '../assets/elerp-logo.svg'
import { useState } from 'react'
import QRCode from 'qrcode'
import { api } from '../lib/api.js'
import { Button, Input, Field, Card, Spinner } from '../ui.jsx'

// Login de operadores Mornix en dos factores. Pasos:
//   password → (enrolar-mfa | codigo) → sesión.
export function Login({ onEntrar }) {
  const [paso, setPaso] = useState('password') // password | codigo | enrolar
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [codigo, setCodigo] = useState('')
  const [qr, setQr] = useState('')
  const [secreto, setSecreto] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const enrolar = async () => {
    const r = await api.mfaSetup({ email, password })
    setSecreto(r.secreto)
    try { setQr(await QRCode.toDataURL(r.otpauthUrl, { margin: 1, width: 220 })) } catch { setQr('') }
    setPaso('enrolar')
  }

  const submitPassword = async (e) => {
    e.preventDefault()
    setBusy(true); setError('')
    try {
      const r = await api.login({ email, password })
      if (r && r.paso === 'enrolar-mfa') { await enrolar() }
      else if (r && r.ok) { onEntrar({ email: r.operador.email, nombre: r.operador.nombre }) }
    } catch (err) {
      if (err.status === 401 && err.data?.paso === 'mfa') { setPaso('codigo') }
      else setError(err.message || 'No se pudo iniciar sesión')
    } finally { setBusy(false) }
  }

  const submitCodigo = async (e) => {
    e.preventDefault()
    setBusy(true); setError('')
    try {
      const r = await api.login({ email, password, codigo })
      if (r && r.ok) onEntrar({ email: r.operador.email, nombre: r.operador.nombre })
    } catch (err) {
      setError(err.data?.error || err.message || 'Código inválido')
    } finally { setBusy(false) }
  }

  const submitVerify = async (e) => {
    e.preventDefault()
    setBusy(true); setError('')
    try {
      const r = await api.mfaVerify({ email, password, codigo })
      if (r && r.ok) onEntrar({ email: r.operador.email, nombre: r.operador.nombre })
    } catch (err) {
      setError(err.data?.error || err.message || 'Código inválido')
    } finally { setBusy(false) }
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4 bg-gradient-to-br from-slate-950 via-elerp-950 to-slate-950">
      <Card className="w-full max-w-md p-7">
        <div className="flex flex-col items-center text-center mb-6">
          <img src={logoUrl} alt="elerp" className="mb-3" style={{ height: 26, width: 'auto', filter: 'brightness(0) invert(1)' }} />
          <h1 className="font-display font-bold text-lg text-slate-100">Consola de plataforma</h1>
          <p className="text-[12.5px] text-slate-500 mt-1">Acceso exclusivo del equipo Mornix · doble factor obligatorio</p>
        </div>

        {paso === 'password' ? (
          <form onSubmit={submitPassword} className="space-y-3.5">
            <Field label="Correo"><Input type="email" autoFocus autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="operador@mornix.tech" /></Field>
            <Field label="Contraseña"><Input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
            {error ? <div className="text-[12.5px] text-red-400">{error}</div> : null}
            <Button type="submit" loading={busy} disabled={!email || !password} className="w-full">Continuar</Button>
          </form>
        ) : null}

        {paso === 'codigo' ? (
          <form onSubmit={submitCodigo} className="space-y-3.5">
            <p className="text-[13px] text-slate-400">Ingresá el código de 6 dígitos de tu app de autenticación.</p>
            <Field label="Código">
              <Input inputMode="numeric" autoFocus maxLength={6} value={codigo}
                onChange={(e) => setCodigo(e.target.value.replace(/\D/g, ''))}
                className="text-center tracking-[0.4em] font-mono text-lg" placeholder="000000" />
            </Field>
            {error ? <div className="text-[12.5px] text-red-400">{error}</div> : null}
            <Button type="submit" loading={busy} disabled={codigo.length !== 6} className="w-full">Entrar</Button>
            <button type="button" onClick={() => { setPaso('password'); setCodigo(''); setError('') }} className="w-full text-[12px] text-slate-500 hover:text-slate-300">← Volver</button>
          </form>
        ) : null}

        {paso === 'enrolar' ? (
          <form onSubmit={submitVerify} className="space-y-3.5">
            <p className="text-[13px] text-slate-400">Configurá tu segundo factor: escaneá el código con Google Authenticator, Authy o 1Password.</p>
            <div className="flex justify-center">
              {qr ? <img src={qr} alt="Código QR para el autenticador" className="rounded-lg bg-white p-2" width={200} height={200} /> : <Spinner size={22} />}
            </div>
            <div className="text-center">
              <div className="text-[11px] text-slate-500 mb-1">o ingresá esta clave manualmente</div>
              <code className="text-[12px] text-teal-300 font-mono break-all">{secreto}</code>
            </div>
            <Field label="Código de verificación">
              <Input inputMode="numeric" autoFocus maxLength={6} value={codigo}
                onChange={(e) => setCodigo(e.target.value.replace(/\D/g, ''))}
                className="text-center tracking-[0.4em] font-mono text-lg" placeholder="000000" />
            </Field>
            {error ? <div className="text-[12.5px] text-red-400">{error}</div> : null}
            <Button type="submit" loading={busy} disabled={codigo.length !== 6} className="w-full">Activar y entrar</Button>
          </form>
        ) : null}
      </Card>
    </div>
  )
}
