import { useState } from 'react'
import { Wordmark } from '../components/Logo.jsx'
import { RacimoModular } from '../components/brand.jsx'
import { Icon } from '../components/Icon.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { validarRIF } from '../lib/format.js'

// El giro precarga catálogo y plan de cuentas del rubro (§2.3 del spec de flujos).
// `restaurante` opera hoy con el POS de retail; mesas, comandas y propinas quedan
// planificadas como módulo aparte (ver Documentos/backlog-ux.md, R1).
const GIROS = [
  { id: 'bodega', label: 'Bodega / Abasto', icon: Icon.Cart, sub: 'Retail de consumo masivo' },
  { id: 'restaurante', label: 'Restaurante', icon: Icon.Inbox, sub: 'Comida y bebida al consumidor' },
  { id: 'farmacia', label: 'Farmacia', icon: Icon.Receipt, sub: 'Medicamentos y cuidado' },
  { id: 'ferreteria', label: 'Ferretería', icon: Icon.Package, sub: 'Materiales y herramientas' },
  { id: 'servicios', label: 'Servicios', icon: Icon.Users, sub: 'Servicios profesionales' },
]

const MODALIDADES = [
  { id: 'forma_libre', label: 'Forma Libre', sub: 'Facturación digital autorizada' },
  { id: 'maquina_fiscal', label: 'Máquina Fiscal', sub: 'Impresora fiscal homologada' },
  { id: 'imprenta_digital', label: 'Imprenta Digital', sub: 'Documentos por imprenta digital' },
]

// Fuentes de la tasa, con las tres opciones exactas del prototipo. El rótulo de
// cada una dice qué implica: la tasa no se teclea salvo que se elija lo último.
const FUENTES_TASA = [
  { id: 'bcv', label: 'Tasa oficial BCV (automática)', sub: 'Se consulta todos los días; es la que exige el SENIAT para tus libros.' },
  { id: 'mercado', label: 'Tasa promedio del mercado', sub: 'Referencia comercial. No es la oficial y la interfaz lo declara.' },
  { id: 'manual', label: 'La escribo yo cada día', sub: 'Queda registrada a nombre de quien la carga.' },
]

const STEPS = ['Empresa', 'Giro', 'Monedas', 'Facturación', 'Sede']

export function Onboarding({ modo = 'inicial', onCerrar }) {
  const esAgregar = modo === 'agregar'
  const { user, onboard, logout } = useAuth()
  const [step, setStep] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(null)

  const [nombre, setNombre] = useState('')
  const [rif, setRif] = useState('')
  const [rifTouched, setRifTouched] = useState(false)
  const [giro, setGiro] = useState('')
  const [modalidad, setModalidad] = useState('')
  // Paso «Monedas y tu primer almacén» (R9 + R10). Por defecto: bolívar como
  // moneda principal y tasa oficial del BCV, que es lo que trae el prototipo.
  const [monedaPrincipal, setMonedaPrincipal] = useState('VES')
  const [preciosEnUsd, setPreciosEnUsd] = useState(true)
  const [fuenteTasa, setFuenteTasa] = useState('bcv')
  const [sedeNombre, setSedeNombre] = useState('Sede Principal')
  const [sedeDir, setSedeDir] = useState('')

  const rifCheck = validarRIF(rif)
  const field = 'w-full h-11 px-3 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm outline-none focus:ring-2 focus:ring-elerp-400'

  const canNext = () => {
    if (step === 0) return nombre.trim() && rifCheck.valid
    if (step === 1) return !!giro
    if (step === 2) return !!monedaPrincipal && !!fuenteTasa
    if (step === 3) return !!modalidad
    if (step === 4) return !!sedeNombre.trim()
    return false
  }

  const next = () => {
    if (step === 0) setRifTouched(true)
    if (!canNext()) return
    if (step < STEPS.length - 1) setStep((s) => s + 1)
    else finish()
  }

  const finish = async () => {
    setBusy(true)
    setError(null)
    try {
      await onboard({
        nombre: nombre.trim(),
        rif: rif.trim().toUpperCase(),
        giro,
        modalidadFacturacion: modalidad,
        monedaPrincipal,
        preciosEnUsd: monedaPrincipal === 'USD' ? true : preciosEnUsd,
        fuenteTasa,
        sede: { nombre: sedeNombre.trim(), direccion: sedeDir.trim() },
      })
      if (esAgregar) onCerrar?.() // cierra el asistente y vuelve a la app (ya en la nueva empresa)
    } catch (err) {
      setError(err?.message || 'No se pudo crear la empresa.')
      setBusy(false)
    }
  }

  return (
    <div className="relative overflow-hidden min-h-screen flex items-center justify-center bg-elerp-500 p-6">
      {/* Fondo de marca (superficie azul del asistente de alta): racimos
          modulares como en el login. Van en los márgenes, detrás de la tarjeta
          opaca, nunca sobre el formulario. */}
      <RacimoModular variante="grande" tone="onDark" style={{ left: 40, top: 64 }} />
      <RacimoModular variante="chico" tone="onDark" escala={0.9} style={{ right: 56, bottom: 52 }} />
      <div className="relative w-full max-w-lg bg-white dark:bg-slate-900 rounded-2xl shadow-modal p-8">
        <div className="flex items-start justify-between">
          <Wordmark size={30} className="mb-1" />
          {esAgregar ? (
            <button onClick={() => onCerrar?.()} title="Cancelar" className="h-8 w-8 -mr-1 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800">
              <Icon.X size={18} />
            </button>
          ) : null}
        </div>
        <p className="text-sm text-slate-600 dark:text-slate-300 mb-5">
          {esAgregar
            ? 'Añade otra empresa (otro RIF) a tu organización. Se creará y quedará activa al terminar.'
            : `Hola${user?.nombre ? `, ${user.nombre.split(' ')[0]}` : ''}. Configura tu primera empresa para empezar.`}
        </p>

        {/* Stepper */}
        <div className="flex items-center gap-2 mb-6">
          {STEPS.map((s, i) => (
            <div key={s} className="flex items-center gap-2 flex-1">
              <div className={`h-7 w-7 shrink-0 rounded-full inline-flex items-center justify-center text-[12px] font-semibold ${i < step ? 'bg-teal-500 text-white' : i === step ? 'bg-elerp-500 text-white' : 'bg-slate-100 dark:bg-slate-800 text-slate-400'}`}>
                {i < step ? <Icon.Check size={14} /> : i + 1}
              </div>
              <span className={`text-[11.5px] font-medium hidden sm:block ${i === step ? 'text-slate-900 dark:text-slate-100' : 'text-slate-400'}`}>{s}</span>
              {i < STEPS.length - 1 ? <div className={`flex-1 h-px ${i < step ? 'bg-teal-400' : 'bg-slate-200 dark:bg-slate-700'}`} /> : null}
            </div>
          ))}
        </div>

        {/* Paso 1 — datos de la empresa */}
        {step === 0 ? (
          <div className="space-y-3.5">
            <div>
              <label className="block text-[12px] font-medium text-slate-500 mb-1">Razón social</label>
              <input className={field} value={nombre} onChange={(e) => setNombre(e.target.value)}
                placeholder="Ej: Distribuidora Andina, C.A." autoFocus />
            </div>
            <div>
              <label className="block text-[12px] font-medium text-slate-500 mb-1">RIF</label>
              <input className={`${field} ${rifTouched && !rifCheck.valid ? 'border-red-400 focus:ring-red-300' : ''}`}
                value={rif} onChange={(e) => setRif(e.target.value)} onBlur={() => setRifTouched(true)}
                placeholder="J-12345678-9" />
              {rifTouched && !rifCheck.valid ? (
                <div className="mt-1 flex items-start gap-1 text-[11.5px] text-red-600 dark:text-red-400">
                  <Icon.CircleAlert size={13} className="mt-0.5 shrink-0" /><span>{rifCheck.msg}</span>
                </div>
              ) : rifCheck.valid ? (
                <div className="mt-1 text-[11.5px] text-teal-600 dark:text-teal-400">Tipo {rifCheck.tipo} válido.</div>
              ) : (
                <div className="mt-1 text-[11.5px] text-slate-400">Comienza con V, E, J o G.</div>
              )}
            </div>
          </div>
        ) : null}

        {/* Paso 2 — giro de negocio (precarga catálogo y plan de cuentas) */}
        {step === 1 ? (
          <div className="grid grid-cols-2 gap-3">
            {GIROS.map((g) => {
              const IconC = g.icon
              const active = giro === g.id
              return (
                <button key={g.id} onClick={() => setGiro(g.id)}
                  className={`text-left p-4 rounded-xl border transition-colors ${active ? 'border-elerp-500 bg-elerp-50 dark:bg-elerp-900/40' : 'border-slate-200 dark:border-slate-700 hover:border-elerp-300'}`}>
                  <IconC size={22} className={active ? 'text-elerp-600' : 'text-slate-400'} />
                  <div className="text-sm font-semibold mt-2">{g.label}</div>
                  <div className="text-[11.5px] text-slate-500">{g.sub}</div>
                </button>
              )
            })}
          </div>
        ) : null}

        {/* Paso 3 — monedas (R9 + R10). Tal como el prototipo: moneda principal
            en dos tarjetas, precios en dólares sí/no y fuente de la tasa. */}
        {step === 2 ? (
          <div className="space-y-5">
            <div className="text-[13px] text-slate-500">
              Toda cifra guardará la tasa de su día — así tus reportes fiscales siempre cuadran.
            </div>

            <div>
              <label className="block text-[13px] font-semibold mb-2">Moneda principal</label>
              <div className="grid grid-cols-2 gap-3">
                {[
                  { id: 'VES', t: 'Bolívares (Bs)', s: 'Facturas y libros en Bs — lo que exige el SENIAT' },
                  { id: 'USD', t: 'Dólares (US$)', s: 'Piensas en dólares; convertimos a Bs al facturar' },
                ].map((m) => {
                  const active = monedaPrincipal === m.id
                  return (
                    <button key={m.id} onClick={() => setMonedaPrincipal(m.id)}
                      className={`text-left p-3.5 rounded-xl border transition-colors ${active ? 'border-elerp-500 bg-elerp-50 dark:bg-elerp-900/40' : 'border-slate-200 dark:border-slate-700 hover:border-elerp-300'}`}>
                      <div className="text-sm font-semibold">{m.t}</div>
                      <div className="text-[12px] text-slate-500 mt-0.5">{m.s}</div>
                    </button>
                  )
                })}
              </div>
            </div>

            {/* Con moneda principal US$ la pregunta no aplica: ya son dólares. */}
            {monedaPrincipal === 'VES' ? (
              <div>
                <label className="block text-[13px] font-semibold mb-1">¿Manejas precios en dólares?</label>
                <div className="text-[12px] text-slate-500 mb-2">
                  Si dices que sí, cada producto podrá tener su precio en US$ y el sistema lo convierte a Bs con la tasa del día al facturar.
                </div>
                <div className="flex gap-2">
                  {[{ v: true, l: 'Sí' }, { v: false, l: 'No' }].map((o) => (
                    <button key={o.l} onClick={() => setPreciosEnUsd(o.v)}
                      className={`h-10 px-5 rounded-xl border text-[13.5px] font-semibold transition-colors ${preciosEnUsd === o.v ? 'border-elerp-500 bg-elerp-50 dark:bg-elerp-900/40 text-elerp-600 dark:text-elerp-200' : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300'}`}>
                      {o.l}
                    </button>
                  ))}
                </div>
              </div>
            ) : null}

            <div>
              <label className="block text-[13px] font-semibold mb-2">Fuente de la tasa</label>
              <div className="space-y-2">
                {FUENTES_TASA.map((f) => {
                  const active = fuenteTasa === f.id
                  return (
                    <button key={f.id} onClick={() => setFuenteTasa(f.id)}
                      className={`w-full text-left p-3 rounded-xl border flex items-start gap-3 transition-colors ${active ? 'border-elerp-500 bg-elerp-50 dark:bg-elerp-900/40' : 'border-slate-200 dark:border-slate-700 hover:border-elerp-300'}`}>
                      <span className={`h-4 w-4 mt-0.5 shrink-0 rounded-full border-2 ${active ? 'border-elerp-500 bg-elerp-500' : 'border-slate-300'}`} />
                      <span>
                        <span className="block text-[13.5px] font-semibold">{f.label}</span>
                        <span className="block text-[11.5px] text-slate-500">{f.sub}</span>
                      </span>
                    </button>
                  )
                })}
              </div>
            </div>
          </div>
        ) : null}

        {/* Paso 4 — modalidad de facturación */}
        {step === 3 ? (
          <div className="space-y-2.5">
            {MODALIDADES.map((m) => {
              const active = modalidad === m.id
              return (
                <button key={m.id} onClick={() => setModalidad(m.id)}
                  className={`w-full text-left p-3.5 rounded-xl border flex items-center gap-3 transition-colors ${active ? 'border-elerp-500 bg-elerp-50 dark:bg-elerp-900/40' : 'border-slate-200 dark:border-slate-700 hover:border-elerp-300'}`}>
                  <span className={`h-4 w-4 rounded-full border-2 ${active ? 'border-elerp-500 bg-elerp-500' : 'border-slate-300'}`} />
                  <div>
                    <div className="text-sm font-semibold">{m.label}</div>
                    <div className="text-[11.5px] text-slate-500">{m.sub}</div>
                  </div>
                </button>
              )
            })}
          </div>
        ) : null}

        {/* Paso 5 — primera sede */}
        {step === 4 ? (
          <div className="space-y-3.5">
            <div>
              <label className="block text-[12px] font-medium text-slate-500 mb-1">Nombre de la sede</label>
              <input className={field} value={sedeNombre} onChange={(e) => setSedeNombre(e.target.value)}
                placeholder="Ej: Sede Principal" autoFocus />
            </div>
            <div>
              <label className="block text-[12px] font-medium text-slate-500 mb-1">Dirección <span className="text-slate-400">(opcional)</span></label>
              <input className={field} value={sedeDir} onChange={(e) => setSedeDir(e.target.value)}
                placeholder="Av. Principal, Caracas" />
            </div>
          </div>
        ) : null}

        {error ? (
          <div className="mt-4 flex items-start gap-2 text-[12.5px] text-rose-600 dark:text-rose-400">
            <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" /> <span>{error}</span>
          </div>
        ) : null}

        <div className="mt-6 flex items-center justify-between gap-2">
          {step > 0 ? (
            <button onClick={() => setStep((s) => s - 1)} className="h-11 px-4 rounded-xl text-sm font-medium text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 inline-flex items-center gap-1.5">
              <Icon.ChevLeft size={16} /> Atrás
            </button>
          ) : <span />}
          <button onClick={next} disabled={busy || !canNext()}
            className="h-11 px-5 rounded-xl bg-elerp-500 hover:bg-elerp-600 disabled:opacity-50 text-white font-medium inline-flex items-center justify-center gap-2 transition-colors">
            {busy ? 'Creando…' : step < STEPS.length - 1 ? <>Continuar <Icon.ArrowRight size={17} /></> : <>Crear empresa <Icon.Check size={17} /></>}
          </button>
        </div>

        {esAgregar ? (
          <button onClick={() => onCerrar?.()} className="mt-4 w-full text-[12px] text-slate-400 hover:text-slate-600 dark:hover:text-slate-300">
            Cancelar
          </button>
        ) : (
          <button onClick={logout} className="mt-4 w-full text-[12px] text-slate-400 hover:text-slate-600 dark:hover:text-slate-300">
            Cerrar sesión
          </button>
        )}
      </div>
    </div>
  )
}
