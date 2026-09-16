import { useState, useEffect, useRef } from 'react'
import { Icon } from './Icon.jsx'

/* Teclado numérico de PIN, reutilizable.
 *
 * POR QUÉ SEPARADO: el modo caja ya tiene uno (ModoCaja.PinSupervisorModal),
 * pero aquel está atado a `/api/cajas/autorizar` — pide el PIN y lo valida él
 * mismo. En los turnos del salón el PIN viaja DENTRO del cuerpo de la operación
 * (iniciar turno manda el del mesonero y el del supervisor a la vez), así que lo
 * que hace falta es un teclado que solo DEVUELVA el PIN y deje validar a quien
 * lo pidió. Este no llama a ninguna API a propósito.
 *
 * Táctil y físico a la vez: la tablet del salón puede no tener teclado, pero un
 * equipo de mostrador sí, y quien lo tiene lo usa.
 */

const LARGO = 4

export function TecladoPin({ valor, onCambio, onCompleto, disabled = false }) {
  // Actualización FUNCIONAL siempre: tecleando rápido, cerrar sobre `valor`
  // pierde dígitos.
  const tecla = (t) => {
    if (disabled) return
    onCambio(t === 'del' ? valor.slice(0, -1) : (valor.length >= LARGO ? valor : valor + t))
  }

  // Autoenvío al completar. Va en un efecto y no dentro de `tecla` para que se
  // dispare UNA vez, venga el dígito del táctil o del teclado físico.
  const enviado = useRef(false)
  useEffect(() => {
    if (valor.length < LARGO) { enviado.current = false; return }
    if (enviado.current || disabled) return
    enviado.current = true
    onCompleto?.(valor)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [valor, disabled])

  useEffect(() => {
    const onKey = (e) => {
      if (disabled) return
      if (/^[0-9]$/.test(e.key)) { e.preventDefault(); tecla(e.key) }
      else if (e.key === 'Backspace') { e.preventDefault(); tecla('del') }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [valor, disabled])

  return (
    <div>
      <div className="flex gap-2.5 justify-center" role="status"
        aria-label={`${valor.length} de ${LARGO} dígitos`}>
        {Array.from({ length: LARGO }, (_, i) => (
          <span key={i} className={`h-3.5 w-3.5 rounded-full border-[1.5px] ${i < valor.length
            ? 'bg-elerp-500 border-elerp-500' : 'border-slate-300 dark:border-slate-600'}`} />
        ))}
      </div>
      <div className="grid grid-cols-3 gap-2 mt-4">
        {['1', '2', '3', '4', '5', '6', '7', '8', '9', '', '0', 'del'].map((t, i) => (
          t === '' ? <span key={i} /> : (
            <button key={i} type="button" onClick={() => tecla(t)} disabled={disabled}
              aria-label={t === 'del' ? 'Borrar' : t}
              className="h-14 rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/60 text-[19px] font-semibold hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-50">
              {t === 'del' ? <Icon.ChevLeft size={19} className="mx-auto" /> : t}
            </button>
          )
        ))}
      </div>
    </div>
  )
}

/* PedirPin es el teclado dentro de un panel con título, error y cancelar. Lo usa
 * cada paso del turno (el PIN del mesonero, el del supervisor, el PIN nuevo).
 *
 * `onEnviar(pin)` puede ser asíncrono; si lanza, su mensaje se muestra y el PIN
 * se limpia para reintentar. */
export function PedirPin({ icono, titulo, sub, onEnviar, onCancelar, tono = 'elerp' }) {
  const [pin, setPin] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const enviar = async (valor) => {
    setBusy(true); setError('')
    try {
      await onEnviar(valor)
    } catch (e) {
      setError(e?.message || 'No se pudo continuar.')
      setPin('')
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => {
    const onKey = (e) => { if (e.key === 'Escape') { e.preventDefault(); onCancelar?.() } }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onCancelar])

  const color = tono === 'amber'
    ? 'bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400'
    : 'bg-elerp-50 dark:bg-elerp-900/30 text-elerp-700 dark:text-elerp-300'

  return (
    <div className="text-center">
      <div className={`h-12 w-12 rounded-full inline-flex items-center justify-center ${color}`}>
        {icono || <Icon.Lock size={22} />}
      </div>
      <div className="text-[16px] font-semibold mt-2.5">{titulo}</div>
      {sub ? <div className="text-[12.5px] text-slate-500 mt-1">{sub}</div> : null}
      <div className="mt-4">
        <TecladoPin valor={pin} onCambio={setPin} onCompleto={enviar} disabled={busy} />
      </div>
      {error ? <div className="mt-2.5 text-[12.5px] text-[#B3362C] dark:text-red-400">{error}</div> : null}
      {onCancelar ? (
        <button type="button" onClick={onCancelar}
          className="mt-3.5 text-[12.5px] font-semibold text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">
          Cancelar
        </button>
      ) : null}
    </div>
  )
}
