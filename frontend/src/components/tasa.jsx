import { useState, useEffect } from 'react'
import { Icon } from './Icon.jsx'
import { Button, Modal, Field, Input, Empty, useToast, Badge } from './primitives.jsx'
import { fmtNum, fmtDate } from '../lib/format.js'
import { useTasa, useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { monedaLabel, monedaSimbolo } from '../lib/precio.js'
import { api } from '../lib/api.js'

/* Tasa de cambio en la interfaz (R9).
 *
 * Dos reglas que manda el cliente y que este archivo hace cumplir:
 *   1. La tasa NO se teclea en la operación. El campo manual del POS
 *      desapareció; queda una carga manual como último recurso, dentro de este
 *      modal, sola para Dueña/Desarrollador y auditada.
 *   2. La interfaz SIEMPRE declara el origen y la fecha de la cifra que está
 *      usando. «BCV» solo aparece cuando la trajo el BCV.
 */

/* explicarFallo traduce el error técnico de la consulta a algo que una dueña de
 * bodega pueda entender, sin esconder el detalle: el técnico queda en el title.
 * La interfaz tiene que poder decir POR QUÉ la tasa no es de hoy. */
export function explicarFallo(txt) {
  const t = String(txt || '')
  if (!t) return ''
  if (/certificate|x509|tls/i.test(t)) return 'El sitio del BCV no presenta bien su certificado y no se pudo verificar la conexión.'
  if (/timeout|deadline|EOF|refused|no such host/i.test(t)) return 'Ni el BCV ni la fuente de respaldo respondieron.'
  if (/reconocible|formato/i.test(t)) return 'La fuente respondió, pero su página cambió y no se encontró la cifra.'
  if (/cuarentena/i.test(t)) return 'Llegó una tasa con una variación anómala y quedó en espera de aprobación.'
  return 'La consulta a la fuente falló.'
}

// Roles que pueden intervenir la tasa (cargarla, forzar consulta, aprobar).
const puedeAdministrarTasa = (rol) => rol === 'dueno' || rol === 'desarrollador'

// fechaCortaVE muestra "hoy 31 jul" o "30 jul" a partir de YYYY-MM-DD.
const MESES = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic']
export function fechaCortaVE(iso, esDeHoy) {
  if (!iso || iso.length < 10) return ''
  const [a, m, d] = iso.slice(0, 10).split('-')
  const txt = `${Number(d)} ${MESES[Number(m) - 1] || m}`
  return esDeHoy ? `hoy ${txt}` : `${txt} de ${a}`
}

/* TasaChip es el bloque de la barra superior: valor, origen y fecha. Abre el
 * modal al pulsarlo. Sin tasa cargada muestra el aviso, no un cero. */
export function TasaChip({ className = '' }) {
  const { ui } = useUI()
  // La divisa que muestra el chip es la seleccionada en la barra; para VES cae
  // al dólar como referencia base (que es lo que el chip mostró siempre).
  const ccy = ui.ccy && ui.ccy !== 'VES' ? ui.ccy : 'USD'
  const t = useTasa(ccy)
  const [abierto, setAbierto] = useState(false)

  const desactualizada = t.hay && !t.esDeHoy
  return (
    <>
      <button onClick={() => setAbierto(true)} title="Tasa de cambio: origen, fecha e historial"
        className={`text-right leading-tight px-1.5 py-1 rounded-lg shrink-0 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus ${className}`}>
        {t.hay ? (
          <>
            <div className="text-[12px] font-semibold text-slate-700 dark:text-slate-200 tnum whitespace-nowrap">
              Bs {fmtNum(t.valor, 2)} / {monedaLabel(ccy)}
            </div>
            <div className={`text-[10.5px] whitespace-nowrap ${desactualizada ? 'text-amber-600 dark:text-amber-400' : 'text-slate-400'}`}>
              {t.fuenteLabel} · {fechaCortaVE(t.fechaValor, t.esDeHoy)}
              {t.enCuarentena ? ' · revisar' : ''}
            </div>
          </>
        ) : (
          <>
            <div className="text-[12px] font-semibold text-amber-600 dark:text-amber-400 whitespace-nowrap">Sin tasa</div>
            <div className="text-[10.5px] text-slate-400 whitespace-nowrap">Cárgala para cobrar en {monedaLabel(ccy)}</div>
          </>
        )}
      </button>
      <TasaModal open={abierto} onClose={() => setAbierto(false)} />
    </>
  )
}

/* TasaModal explica de dónde viene la tasa, deja forzar una consulta, resolver
 * una lectura en cuarentena y —como último recurso— cargarla a mano. */
export function TasaModal({ open, onClose }) {
  const t = useTasa()
  const { ui } = useUI()
  const toast = useToast()
  const admin = puedeAdministrarTasa(ui.rol)

  const [historial, setHistorial] = useState(null)
  const [busy, setBusy] = useState('')
  const [manual, setManual] = useState('')
  const [touched, setTouched] = useState(false)

  useEffect(() => {
    if (!open) return
    let vivo = true
    api.historialTasa(12).then((h) => { if (vivo) setHistorial(h || []) }).catch(() => { if (vivo) setHistorial([]) })
    return () => { vivo = false }
  }, [open, t.obtenidaEn])

  const errManual = !(Number(manual) > 0) ? 'Escribe la tasa en Bs por US$.' : ''

  const sincronizar = async () => {
    setBusy('sync')
    try {
      await api.sincronizarTasa()
      await t.recargar()
      toast({ title: 'Tasa consultada', body: 'Se pidió la tasa a la fuente oficial.' })
    } catch (e) {
      // 429 no es un error del usuario: es la condición de «una consulta al día».
      toast({ title: 'No se consultó', body: e?.message || 'Error', kind: e?.status === 429 ? 'warn' : 'error' })
    } finally { setBusy('') }
  }

  const cargar = async () => {
    setTouched(true)
    if (errManual) return
    setBusy('manual')
    try {
      await api.cargarTasaManual(Number(manual), '')
      await t.recargar()
      setManual('')
      toast({ title: 'Tasa cargada', body: `Bs ${fmtNum(Number(manual), 2)} / US$ · queda registrada a tu nombre.` })
    } catch (e) {
      toast({ title: 'No se pudo cargar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusy('') }
  }

  const aprobar = async () => {
    setBusy('aprobar')
    try {
      await api.aprobarTasaCuarentena(t.cuarentenaId)
      await t.recargar()
      toast({ title: 'Tasa aprobada', body: 'La lectura rechazada quedó vigente.' })
    } catch (e) {
      toast({ title: 'No se pudo aprobar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusy('') }
  }

  return (
    <Modal open={open} onClose={onClose} size="md" icon={<Icon.Banknote size={18} />}
      title="Tasa de cambio" sub="Entra sola desde la fuente oficial; cada documento guarda la tasa de su día."
      footer={<Button variant="ghost" onClick={onClose}>Cerrar</Button>}>
      <div className="space-y-4">
        {/* Estado actual */}
        {t.hay ? (
          <div className="rounded-xl border border-slate-200 dark:border-slate-700 p-4">
            <div className="flex items-end justify-between gap-3">
              <div>
                <div className="text-[22px] font-semibold tnum leading-none font-display">Bs {fmtNum(t.valor, 2)}</div>
                <div className="text-[12px] text-slate-500 mt-1">por 1 US$</div>
              </div>
              <div className="text-right">
                <Badge color={t.oficial ? 'emerald' : 'slate'}>{t.fuenteLabel}</Badge>
                <div className={`text-[11.5px] mt-1 ${t.esDeHoy ? 'text-slate-400' : 'text-amber-600 dark:text-amber-400'}`}>
                  {t.esDeHoy ? 'Tasa de hoy' : `Tasa del ${fechaCortaVE(t.fechaValor, false)}`}
                </div>
              </div>
            </div>
            <div className="mt-3 pt-3 border-t border-slate-100 dark:border-slate-800 text-[11.5px] text-slate-500 space-y-1">
              {t.detalle ? <div>Origen: {t.detalle}</div> : null}
              {t.obtenidaEn ? <div>Obtenida: {fmtDate(t.obtenidaEn)}</div> : null}
              {!t.esDeHoy ? (
                <div className="text-amber-700 dark:text-amber-400">
                  Se sigue operando con esta tasa porque la fuente no ha respondido hoy. La caja nunca se detiene por eso.
                </div>
              ) : null}
              {t.ultimoError ? (
                <div className="text-slate-400" title={t.ultimoError}>
                  {explicarFallo(t.ultimoError)} <span className="text-slate-300 dark:text-slate-600">(pasa el cursor para ver el detalle técnico)</span>
                </div>
              ) : null}
            </div>
          </div>
        ) : (
          <Empty icon={<Icon.CircleAlert size={22} />} title="Sin tasa cargada"
            body="Todavía no hay una tasa de cambio. Se puede facturar en bolívares, pero no cobrar en divisas ni ver precios en US$." />
        )}

        {/* Cuarentena: una lectura rechazada por variación anómala. */}
        {t.enCuarentena ? (
          <div className="rounded-xl border border-amber-300 dark:border-amber-700/60 bg-amber-50 dark:bg-amber-900/20 p-4">
            <div className="text-[13px] font-semibold text-amber-900 dark:text-amber-200 flex items-center gap-1.5">
              <Icon.CircleAlert size={15} /> Lectura sin aplicar: Bs {fmtNum(t.valorEnCuarentena, 2)}
            </div>
            <div className="text-[12px] text-amber-800 dark:text-amber-300/90 mt-1">
              {t.motivoCuarentena}. No se aplicó sola para que una fuente alterada no pueda mover todos tus precios.
              Si la devaluación fue real, apruébala.
            </div>
            {admin ? (
              <Button className="mt-3" size="sm" variant="secondary" loading={busy === 'aprobar'} onClick={aprobar}
                icon={<Icon.Check size={15} />}>Aprobar esta tasa</Button>
            ) : (
              <div className="mt-2 text-[11.5px] text-amber-800/80 dark:text-amber-300/70">Solo la Dueña o el Desarrollador pueden aprobarla.</div>
            )}
          </div>
        ) : null}

        {/* Acciones de administración */}
        {admin ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div className="rounded-xl border border-slate-200 dark:border-slate-700 p-3.5">
              <div className="text-[12px] font-semibold text-slate-600 dark:text-slate-300">Consultar ahora</div>
              <div className="text-[11.5px] text-slate-500 mt-1">
                Pide la tasa a la fuente oficial. Se consulta una vez al día para no abusar del sitio del BCV.
              </div>
              <Button className="mt-2.5" size="sm" variant="secondary" loading={busy === 'sync'} onClick={sincronizar}
                icon={<Icon.Refresh size={15} />}>Actualizar</Button>
            </div>
            <div className="rounded-xl border border-slate-200 dark:border-slate-700 p-3.5">
              <div className="text-[12px] font-semibold text-slate-600 dark:text-slate-300">Cargarla a mano</div>
              <div className="text-[11.5px] text-slate-500 mt-1">
                Último recurso si la fuente no responde. Queda registrada a tu nombre y se rotula «Tasa manual», nunca «BCV».
              </div>
              <div className="mt-2.5 flex items-end gap-2">
                <div className="flex-1">
                  <Field label="Bs por US$" error={touched ? errManual : ''}>
                    <Input type="number" min="0" step="0.01" value={manual} placeholder="0,00"
                      onChange={(e) => setManual(e.target.value)} invalid={touched && !!errManual} />
                  </Field>
                </div>
                <Button size="sm" loading={busy === 'manual'} onClick={cargar} icon={<Icon.Check size={15} />}>Cargar</Button>
              </div>
            </div>
          </div>
        ) : null}

        {/* Histórico */}
        <div>
          <div className="text-[12px] font-semibold text-slate-600 dark:text-slate-300 mb-1.5">Historial</div>
          {historial === null ? (
            <div className="h-20 rounded-xl bg-slate-100 dark:bg-slate-800/60 animate-pulse" />
          ) : historial.length === 0 ? (
            <div className="text-[12px] text-slate-400">Todavía no hay registros.</div>
          ) : (
            <div className="rounded-xl border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800 overflow-hidden">
              {historial.map((h) => (
                <div key={h.id} className="flex items-center gap-3 px-3 py-2">
                  <div className="num text-[13px] font-medium w-28 shrink-0">Bs {fmtNum(h.valor, 2)}</div>
                  <div className="flex-1 min-w-0 text-[11.5px] text-slate-500 truncate">
                    {h.fechaValor}
                    {h.detalle ? ` · ${h.detalle}` : ''}
                    {h.motivo ? ` · ${h.motivo}` : ''}
                  </div>
                  <Badge size="sm" color={h.estado === 'rechazada' ? 'amber' : h.fuente === 'bcv' || h.fuente === 'respaldo' ? 'emerald' : 'slate'}>
                    {h.estado === 'rechazada' ? 'rechazada' : h.fuente}
                  </Badge>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </Modal>
  )
}

/* PrecioDual es el patrón de moneda dual del Sistema de diseño del prototipo
 * (R10), ahora multimoneda: se escribe en la moneda elegida (Bs o cualquier
 * divisa activa) y el homólogo va al lado, de solo lectura y en gris.
 *
 * Bs es la base: si el precio se captura en una divisa, el homólogo es su valor
 * en Bs; si se captura en Bs, el homólogo es su valor en la divisa de referencia
 * (la primera divisa activa). El homólogo se CALCULA siempre: guardar los dos
 * precios los desincroniza en cuanto cambia la tasa.
 *
 * `monedas` es la lista de códigos seleccionables (p. ej. ['VES','USD','EUR']).
 * Se conserva `permiteUSD` como respaldo cuando no se pasa `monedas`.
 */
export function PrecioDual({ valor, onChange, moneda = 'VES', onMoneda, monedas, permiteUSD = false, label = 'Precio', error, autoFocus }) {
  const opciones = (monedas && monedas.length ? monedas : (permiteUSD ? ['VES', 'USD'] : ['VES']))
    .map((c) => (c || 'VES').toUpperCase())
  const m = (moneda || 'VES').toUpperCase()
  const esVES = m === 'VES'
  // Divisa de referencia para el homólogo cuando se captura en Bs.
  const refDivisa = esVES ? (opciones.find((c) => c !== 'VES') || 'USD') : m
  const t = useTasa(refDivisa)
  const n = Number(valor) || 0
  // Homólogo: en Bs si se captura en divisa; en la divisa de referencia si se
  // captura en Bs.
  const homologo = esVES ? t.aMoneda(n) : t.aBs(n)
  const homologoMoneda = esVES ? refDivisa : 'VES'

  return (
    <div>
      <div className="flex items-center justify-between mb-1.5">
        <label className="text-[13px] font-medium text-slate-700 dark:text-slate-200">{label}</label>
        {/* Selector de moneda del precio: Bs + divisas activas. */}
        {opciones.length > 1 && onMoneda ? (
          <div className="flex bg-slate-100 dark:bg-slate-800 rounded-lg p-0.5">
            {opciones.map((c) => (
              <button key={c} type="button" onClick={() => onMoneda(c)}
                className={`px-2.5 h-6 rounded-md text-[11.5px] font-medium ${m === c ? 'bg-white dark:bg-slate-700 shadow-sm text-slate-800 dark:text-slate-100' : 'text-slate-500'}`}>
                {monedaLabel(c)}
              </button>
            ))}
          </div>
        ) : null}
      </div>
      <div className="flex gap-2">
        <div className="relative flex-1">
          <span className="absolute left-3 top-1/2 -translate-y-1/2 text-[12.5px] font-semibold text-slate-400">
            {monedaSimbolo(m)}
          </span>
          <input type="number" min="0" step="0.01" value={valor} onChange={(e) => onChange(e.target.value)} autoFocus={autoFocus}
            placeholder="0,00"
            className={`w-full h-10 pl-9 pr-3 rounded-xl border bg-white dark:bg-slate-900 text-sm num text-right ring-focus ${error ? 'border-red-400' : 'border-slate-200 dark:border-slate-700'}`} />
        </div>
        {/* Homólogo: de solo lectura y en gris, como en el prototipo. */}
        <div className="relative flex-1">
          <span className="absolute left-3 top-1/2 -translate-y-1/2 text-[12.5px] font-semibold text-slate-400">
            {monedaSimbolo(homologoMoneda)}
          </span>
          <input readOnly tabIndex={-1}
            value={homologo === null ? 'sin tasa' : fmtNum(homologo, 2)}
            className="w-full h-10 pl-9 pr-3 rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/60 text-sm num text-right text-slate-500" />
        </div>
      </div>
      <div className="mt-1.5 text-[11.5px] text-slate-400 num">
        {t.hay
          ? `a Bs ${fmtNum(t.valor, 2)} / ${monedaLabel(refDivisa)} · ${t.fuenteLabel} · ${fechaCortaVE(t.fechaValor, t.esDeHoy)}`
          : `Sin tasa cargada para ${monedaLabel(refDivisa)}: el equivalente no se puede calcular.`}
      </div>
      {error ? <div className="mt-1 text-[11.5px] text-red-600 dark:text-red-400">{error}</div> : null}
    </div>
  )
}
