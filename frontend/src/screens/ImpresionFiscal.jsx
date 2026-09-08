import { useState, useEffect, useRef, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Modal } from '../components/primitives.jsx'
import { fmtCurrency } from '../lib/format.js'

/* ENTORNO DE IMPRESORA FISCAL (POS).
 *
 * En el punto de venta la factura NO es un PDF: al emitir, el documento fiscal se
 * ENVÍA A LA IMPRESORA FISCAL (el dispositivo configurado de la sede). La conexión
 * real con el hardware la hace el AGENTE FISCAL LOCAL (SAD §9.3): un binario
 * aparte, canal saliente, que aún NO está conectado. Todo lo de este archivo es el
 * entorno alrededor de ese agente —envío, estados, errores y reintento— con el
 * agente como un PUNTO DE INTEGRACIÓN (stub) claramente marcado.
 *
 * Regla dura de integridad: la factura ya está emitida con numeración atómica
 * (append-only). Reimprimir / reintentar la impresión NO re-emite ni duplica el
 * `Documento`: solo REENVÍA los datos ya emitidos al hardware. La factura es
 * válida aunque la impresión falle, y la impresión es reintentable.
 */

// Selecciona la impresora fiscal ACTIVA destino de la sede. Se prefiere una
// impresora atada a la sede activa; si no hay, se cae a una registrada para
// "toda la empresa" (sedeId vacío). Solo se consideran dispositivos activos y de
// tipo impresora fiscal (o sin tipo, por retrocompatibilidad).
export function impresoraActivaDeSede(dispositivos, sedeId) {
  const activas = (dispositivos || []).filter(
    (d) => d && d.activo && (d.tipo === 'impresora_fiscal' || !d.tipo),
  )
  return (
    activas.find((d) => d.sedeId && d.sedeId === sedeId)
    || activas.find((d) => !d.sedeId)
    || null
  )
}

// Códigos de error que el AGENTE FISCAL LOCAL reportará (contrato previsto), ya
// mapeados a un estado de UI en español con su ícono y mensaje. Cuando el agente
// real responda `sin_papel` / `sin_conexion` / etc., el modal ya sabe pintarlos.
export const ERRORES_IMPRESORA = {
  sin_papel: {
    icon: 'CircleAlert',
    tono: 'amber',
    titulo: 'Falta papel en la impresora',
    detalle: 'La impresora fiscal se quedó sin papel. Coloca un rollo nuevo y reintenta la impresión. La factura ya quedó emitida; solo falta imprimirla.',
  },
  sin_conexion: {
    icon: 'WifiOff',
    tono: 'amber',
    titulo: 'Sin conexión con la impresora fiscal',
    detalle: 'El agente fiscal local no responde. Verifica que la impresora esté encendida y que el agente esté en ejecución en este equipo, y reintenta.',
  },
  error: {
    icon: 'CircleX',
    tono: 'red',
    titulo: 'No se pudo imprimir la factura',
    detalle: 'La impresora fiscal reportó un error al procesar el documento. Reintenta la impresión; si persiste, revisa el dispositivo.',
  },
}

const TONO_CLASES = {
  amber: 'bg-amber-50 dark:bg-amber-900/25 text-amber-600 dark:text-amber-400',
  red: 'bg-red-50 dark:bg-red-900/25 text-[#B3362C] dark:text-red-400',
}

// Opciones del simulador de demo (ver nota abajo). El valor coincide con el
// código que emitiría el agente real; 'ok' = impresión exitosa.
const SIMULACIONES = [
  { v: 'ok', t: 'Impresión OK' },
  { v: 'sin_papel', t: 'Sin papel' },
  { v: 'sin_conexion', t: 'Sin conexión' },
]

/* PUNTO DE INTEGRACIÓN — AGENTE FISCAL LOCAL (SAD §9.3).
 * TODO(agente fiscal local): reemplazar este STUB por la llamada real al agente
 * local (canal saliente: el navegador NO habla con la impresora; le pide al
 * binario local que imprima, p. ej. `POST http://127.0.0.1:<puerto>/imprimir`
 * firmado). El agente devuelve éxito o un código de error de hardware.
 *
 * Contrato de error previsto: en caso de fallo se lanza un Error con `.codigo`
 * en { 'sin_papel', 'sin_conexion', 'error' } — ya mapeados en ERRORES_IMPRESORA.
 *
 * Hoy simula el canal (~1.2s). `simular` existe SOLO para la demo: fuerza el
 * resultado para que el dueño pueda ver los estados de error sin hardware. En
 * producción el resultado lo determina el agente, no este parámetro.
 *
 * IMPORTANTE: esto NO emite ni re-emite la factura. El `doc` ya viene emitido
 * (numeración fiscal atómica, append-only). Aquí solo se reenvían sus datos al
 * hardware; llamar de nuevo (reimprimir/reintentar) nunca duplica el documento. */
export async function enviarAImpresoraFiscal(doc, dispositivo, simular = 'ok') {
  await new Promise((resolve) => setTimeout(resolve, 1200))
  if (simular && simular !== 'ok') {
    const err = new Error(`Impresora fiscal: ${simular}`)
    err.codigo = ERRORES_IMPRESORA[simular] ? simular : 'error'
    throw err
  }
  // El agente real devolvería aquí el nº de reporte / estado del documento fiscal.
  return { ok: true, dispositivoId: dispositivo?.id || '', numeroCompleto: doc?.numeroCompleto || '' }
}

/* Modal del flujo de impresión fiscal. Se abre tras una emisión exitosa (el
 * `Documento` ya existe) y también al pulsar «Imprimir» / «Reimprimir». Maneja:
 *   enviando → impresa → (reimprimir) · error(sin papel / sin conexión / genérico) → reintentar
 *   no_config → cuando la sede no tiene impresora fiscal activa (continuar sin imprimir).
 *
 * `canal` distingue dónde se usa el modal:
 *   · 'ventas' (por defecto) — dentro de la pantalla `VentaEmitida` (facturación
 *     forma libre): al cerrar se vuelve a esa pantalla de éxito con sus entregas.
 *   · 'caja' — en el POS (ModoCaja) el modal es la pantalla TERMINAL del cobro: no
 *     hay pantalla de éxito detrás; «Cerrar» equivale a «Nueva venta» y `onCerrar`
 *     deja la caja lista para el próximo cliente (carrito vacío).
 *
 * En ambos canales, cuando el documento dejó VUELTO se muestra bien visible el
 * monto a entregar al cliente (el cajero lo necesita a la mano en el mostrador).
 *
 * Es del lado del cajero: NO publica nada por BroadcastChannel (la pantalla del
 * cliente ya mostró su «gracias» al emitir). No interfiere con esos eventos. */
export function ImpresionFiscalModal({ open, doc, dispositivo, onCerrar, canal = 'ventas' }) {
  const esCaja = canal === 'caja'
  // fase: 'enviando' | 'impresa' | 'error' | 'no_config'
  const [fase, setFase] = useState('enviando')
  const [codigo, setCodigo] = useState('')
  // Simulación de demo: qué resultado forzar en el PRÓXIMO envío manual.
  const [simular, setSimular] = useState('ok')
  const iniciado = useRef(false)

  // Envía (o reenvía) el documento YA emitido a la impresora. NO re-emite: la
  // factura existe desde la emisión; esto solo reenvía sus datos al hardware.
  const enviar = useCallback(async (sim) => {
    setFase('enviando'); setCodigo('')
    try {
      await enviarAImpresoraFiscal(doc, dispositivo, sim)
      setFase('impresa')
    } catch (e) {
      setCodigo(e?.codigo || 'error')
      setFase('error')
    }
  }, [doc, dispositivo])

  // Al abrir: si no hay impresora activa de la sede, estado «no configurada»; si
  // la hay, se dispara UN envío automático (siempre intenta imprimir de verdad,
  // sim='ok'). El simulador solo afecta reimpresiones/reintentos manuales.
  useEffect(() => {
    if (!open) { iniciado.current = false; return }
    if (!dispositivo) { setFase('no_config'); return }
    if (iniciado.current) return
    iniciado.current = true
    setSimular('ok')
    enviar('ok')
  }, [open, dispositivo, enviar])

  if (!open) return null

  const destino = dispositivo
    ? [dispositivo.nombre, dispositivo.serie ? `serie ${dispositivo.serie}` : '']
        .filter(Boolean).join(' · ')
    : ''
  const err = fase === 'error' ? (ERRORES_IMPRESORA[codigo] || ERRORES_IMPRESORA.error) : null
  const ErrIcon = err ? (Icon[err.icon] || Icon.CircleX) : null

  // Botones del pie según la fase.
  const footer = (() => {
    if (fase === 'no_config') {
      return <Button variant="secondary" size="lg" onClick={onCerrar} icon={<Icon.ArrowRight size={16} />}>Continuar sin imprimir</Button>
    }
    if (fase === 'enviando') {
      return <Button variant="ghost" size="lg" disabled>Enviando…</Button>
    }
    if (fase === 'impresa') {
      return (
        <>
          <Button variant="secondary" size="lg" onClick={() => enviar(simular)} icon={<Icon.Printer size={16} />}>Reimprimir</Button>
          {esCaja ? (
            <Button variant="primary" size="lg" onClick={onCerrar} icon={<Icon.Plus size={16} />}>Nueva venta</Button>
          ) : (
            <Button variant="primary" size="lg" onClick={onCerrar} icon={<Icon.Check size={16} />}>Cerrar</Button>
          )}
        </>
      )
    }
    // error
    return (
      <>
        <Button variant="ghost" size="lg" onClick={onCerrar}>Continuar sin imprimir</Button>
        <Button variant="primary" size="lg" onClick={() => enviar(simular)} icon={<Icon.Refresh size={16} />}>Reintentar imprimir factura</Button>
      </>
    )
  })()

  return (
    <Modal open={open} onClose={onCerrar} size="sm" icon={<Icon.Printer size={18} />}
      title="Impresión fiscal" sub={destino || 'Dispositivo fiscal de la sede'} footer={footer}>
      <div className="py-2 text-center">

        {/* ENVIANDO */}
        {fase === 'enviando' ? (
          <>
            <div className="h-16 w-16 rounded-full bg-elerp-50 dark:bg-elerp-900/40 text-elerp-500 inline-flex items-center justify-center mb-3">
              <span className="spin inline-flex"><Icon.Refresh size={30} /></span>
            </div>
            <div className="text-[17px] font-semibold font-display">Enviando a la impresora fiscal…</div>
            <div className="text-[13px] text-slate-500 mt-1.5">
              {destino ? <>Se está imprimiendo en <strong>{destino}</strong>.</> : 'Enviando el documento fiscal.'}
            </div>
          </>
        ) : null}

        {/* IMPRESA (éxito) */}
        {fase === 'impresa' ? (
          <>
            <div className="h-16 w-16 rounded-full bg-emerald-50 dark:bg-emerald-900/30 text-emerald-500 inline-flex items-center justify-center mb-3">
              <Icon.CircleCheck size={32} />
            </div>
            <div className="text-[19px] font-semibold font-display">Factura impresa</div>
            {doc?.numeroCompleto ? (
              <div className="mono text-[15px] font-semibold tracking-wide mt-1">{doc.numeroCompleto}</div>
            ) : null}
            {destino ? <div className="text-[12.5px] text-slate-500 mt-1">Impresa en {destino}.</div> : null}
          </>
        ) : null}

        {/* ERROR (sin papel / sin conexión / genérico) */}
        {fase === 'error' && err ? (
          <>
            <div className={`h-16 w-16 rounded-full inline-flex items-center justify-center mb-3 ${TONO_CLASES[err.tono] || TONO_CLASES.red}`}>
              <ErrIcon size={30} />
            </div>
            <div className="text-[17px] font-semibold font-display">{err.titulo}</div>
            <div className="text-[13px] text-slate-500 mt-1.5 max-w-sm mx-auto">{err.detalle}</div>
          </>
        ) : null}

        {/* NO CONFIGURADA */}
        {fase === 'no_config' ? (
          <>
            <div className="h-16 w-16 rounded-full bg-amber-50 dark:bg-amber-900/25 text-amber-600 dark:text-amber-400 inline-flex items-center justify-center mb-3">
              <Icon.Printer size={30} />
            </div>
            <div className="text-[17px] font-semibold font-display">No hay impresora fiscal configurada</div>
            <div className="text-[13px] text-slate-500 mt-1.5 max-w-sm mx-auto">
              Esta sede no tiene una impresora fiscal activa. Configúrala en
              {' '}<strong>Configuración › Dispositivos fiscales</strong> para imprimir automáticamente al cobrar.
            </div>
          </>
        ) : null}

        {/* VUELTO A ENTREGAR — el cajero lo necesita a la mano. Se muestra en todas
            las fases (enviando, impresa, error, no_config): la impresión no cambia
            que hay dinero que devolver de la gaveta. Solo aparece con excedente. */}
        {doc?.vuelto > 0.004 ? (
          <div className="mt-4 rounded-xl bg-emerald-50 dark:bg-emerald-900/25 border border-emerald-200 dark:border-emerald-700/50 px-4 py-3">
            <div className="text-[12px] font-medium text-emerald-800/80 dark:text-emerald-300/80">Entrega el vuelto</div>
            <div className="text-[24px] font-semibold num text-emerald-700 dark:text-emerald-300 leading-tight">
              {fmtCurrency(doc.vuelto, doc.vueltoMoneda || 'VES')}
            </div>
            {doc.cobrado ? (
              <div className="text-[11.5px] text-emerald-800/70 dark:text-emerald-300/70 num mt-0.5">
                Recibiste {fmtCurrency(doc.cobrado, 'VES')} · factura {fmtCurrency(doc.total, 'VES')}
              </div>
            ) : null}
          </div>
        ) : null}

        {/* La factura vale aunque la impresión falle: nota tranquilizadora en los
            estados que no son el de éxito. */}
        {fase !== 'impresa' && fase !== 'enviando' ? (
          <div className="mt-4 rounded-lg bg-emerald-50/70 dark:bg-emerald-900/15 border border-emerald-200 dark:border-emerald-800/60 px-3 py-2 text-[12px] text-emerald-800 dark:text-emerald-300 inline-flex items-start gap-2 text-left">
            <Icon.CircleCheck size={15} className="mt-0.5 shrink-0" />
            <span>La factura <strong>{doc?.numeroCompleto || ''}</strong> ya está emitida y es válida. Reintentar solo la reenvía a la impresora; no se vuelve a emitir ni se duplica.</span>
          </div>
        ) : null}

        {/* SIMULADOR DE DEMO — solo para que el dueño VEA los estados de error sin
            hardware. Rotulado como simulación; nada de esto va a producción: el
            resultado real lo decide el agente fiscal local. */}
        {fase !== 'no_config' ? (
          <div className="mt-4 pt-3 border-t border-dashed border-slate-200 dark:border-slate-700 flex items-center justify-center gap-2 flex-wrap">
            <span className="text-[11px] font-semibold uppercase tracking-wide text-slate-400">Simular (demo)</span>
            <div className="inline-flex items-center p-0.5 rounded-lg bg-slate-200 dark:bg-slate-800/70">
              {SIMULACIONES.map((o) => (
                <button key={o.v} type="button" onClick={() => setSimular(o.v)}
                  className={`px-2.5 py-1 rounded-md text-[12px] font-semibold transition-colors ${
                    simular === o.v ? 'bg-white dark:bg-slate-900 shadow-sm text-slate-900 dark:text-slate-100' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'}`}>
                  {o.t}
                </button>
              ))}
            </div>
            <span className="w-full text-[11px] text-slate-400 text-center">Elige un resultado y toca «{fase === 'error' ? 'Reintentar' : 'Reimprimir'}» para verlo.</span>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
