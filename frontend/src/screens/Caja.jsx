import { useState, useEffect, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Modal, Field, Input, Badge, useToast } from '../components/primitives.jsx'
import { api } from '../lib/api.js'
import { useAuth } from '../context/AuthContext.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { metodoLabel } from '../lib/fiscal.js'

/* Sesión de caja — el turno que habilita a facturar.
 *
 * Regla del diseño (03 §4.4/§4.5), replicada en el backend: sin una caja abierta
 * el punto de venta NO se renderiza; en su lugar va la vista de abajo. El
 * servidor rechaza igual la emisión, así que esto es comodidad, no seguridad.
 */

// useSesionCaja centraliza el estado del turno para que el POS y el Modo caja
// compartan una sola fuente de verdad.
export function useSesionCaja() {
  const { activeEmpresaId, activeSedeId } = useAuth()
  const [sesion, setSesion] = useState(null)
  const [cargando, setCargando] = useState(true)

  const recargar = useCallback(async () => {
    if (!activeEmpresaId || !activeSedeId) return
    setCargando(true)
    try {
      const r = await api.miSesionCaja()
      setSesion(r?.abierta ? r.sesion : null)
    } catch {
      // Sin conexión no se puede saber: se asume sin turno y el usuario lo abre.
      setSesion(null)
    } finally {
      setCargando(false)
    }
  }, [activeEmpresaId, activeSedeId])

  useEffect(() => { recargar() }, [recargar])
  return { sesion, cargando, recargar, setSesion }
}

/* Modal bloqueante de apertura (03 §4.3), en dos pasos dentro de un solo modal:
 *   1. elegir caja — las ocupadas por otro cajero salen deshabilitadas con su
 *      nombre; la propia se ofrece como «tu turno abierto · retomar»;
 *   2. código de cajero y PIN.
 * Tiene Cancelar y avisa explícitamente cuando no queda ninguna caja libre. */
export function AbrirCajaModal({ open, onClose, onAbierta }) {
  const toast = useToast()
  const { activeSedeId } = useAuth()
  const [cajas, setCajas] = useState([])
  const [cargando, setCargando] = useState(true)
  const [sel, setSel] = useState(null)
  const [codigo, setCodigo] = useState('')
  const [pin, setPin] = useState('')
  const [fondo, setFondo] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    setSel(null); setCodigo(''); setPin(''); setFondo(''); setError(''); setCargando(true)
    api.cajas(activeSedeId)
      .then((cs) => setCajas(cs || []))
      .catch((e) => setError(e?.message || 'No se pudieron cargar las cajas.'))
      .finally(() => setCargando(false))
  }, [open, activeSedeId])

  const disponibles = cajas.filter((c) => c.estado === 'habilitada' && (!c.ocupada || c.propia))
  const puedeAbrir = sel && codigo.trim() && pin.length === 4

  const abrir = async () => {
    if (!puedeAbrir) return
    setBusy(true); setError('')
    try {
      const fondoInicial = sel.propia ? 0 : Math.max(0, parseFloat((fondo || '').replace(',', '.')) || 0)
      const ses = await api.abrirCaja(sel.id, { codigoCajero: codigo.trim().toUpperCase(), pin, fondoInicial })
      toast({ title: sel.propia ? 'Turno retomado' : 'Caja abierta', body: `Caja ${ses.cajaCodigo} · ${ses.cajeroNombre}` })
      onAbierta?.(ses)
      onClose?.()
    } catch (e) {
      setError(e?.message || 'No se pudo abrir la caja.')
      setPin('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Abrir caja"
      sub="Identifícate para que cada cobro quede a tu nombre."
      footer={
        <div className="flex items-center justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button onClick={abrir} loading={busy} disabled={!puedeAbrir}
            icon={<Icon.Lock size={16} />}>
            {sel?.propia ? 'Retomar turno' : 'Abrir caja'}
          </Button>
        </div>
      }>
      <div className="space-y-4">
        <div>
          <div className="text-[12px] font-semibold text-slate-500 uppercase tracking-wide mb-2">1 · Elige la caja</div>
          {cargando ? (
            <div className="space-y-2">
              <div className="skeleton h-14 rounded-lg" /><div className="skeleton h-14 rounded-lg" />
            </div>
          ) : cajas.length === 0 ? (
            <div className="rounded-lg bg-amber-50 dark:bg-amber-900/25 text-amber-800 dark:text-amber-300 px-3 py-2.5 text-[13px]">
              Esta sede todavía no tiene cajas. Pídele a la Dueña o al administrador que cree una en Configuración.
            </div>
          ) : disponibles.length === 0 ? (
            <div className="rounded-lg bg-amber-50 dark:bg-amber-900/25 text-amber-800 dark:text-amber-300 px-3 py-2.5 text-[13px]">
              No queda ninguna caja libre en esta sede: todas están ocupadas por otro cajero o deshabilitadas.
            </div>
          ) : (
            <div className="space-y-2">
              {cajas.map((c) => {
                const bloqueada = c.estado !== 'habilitada' || (c.ocupada && !c.propia)
                const activa = sel?.id === c.id
                return (
                  <button key={c.id} disabled={bloqueada} onClick={() => setSel(c)}
                    className={`w-full text-left px-3 py-2.5 rounded-lg border flex items-center gap-3 transition-colors ring-focus
                      ${activa ? 'border-elerp-500 bg-elerp-50 dark:bg-elerp-900/40' : 'border-slate-200 dark:border-slate-700'}
                      ${bloqueada ? 'opacity-55 cursor-not-allowed' : 'hover:border-elerp-300'}`}>
                    <Icon.Wallet size={18} className={activa ? 'text-elerp-500' : 'text-slate-400'} />
                    <span className="flex-1 min-w-0">
                      <span className="block text-[13.5px] font-semibold truncate">{c.nombre}</span>
                      <span className="block text-[11.5px] text-slate-500 mono">{c.codigo}</span>
                    </span>
                    {c.estado !== 'habilitada'
                      ? <Badge size="sm" color="slate" dot>Deshabilitada</Badge>
                      : c.propia
                        ? <Badge size="sm" color="teal" dot>Tu turno · retomar</Badge>
                        : c.ocupada
                          ? <Badge size="sm" color="amber" dot>{c.ocupadaPor}</Badge>
                          : null}
                  </button>
                )
              })}
            </div>
          )}
        </div>

        <div>
          <div className="text-[12px] font-semibold text-slate-500 uppercase tracking-wide mb-2">2 · Tu credencial de cajero</div>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Código" required>
              <Input value={codigo} onChange={(e) => setCodigo(e.target.value)}
                placeholder="OP-001" className="mono" autoComplete="off" />
            </Field>
            <Field label="PIN" required hint="4 dígitos">
              <Input type="password" inputMode="numeric" maxLength={4} value={pin}
                onChange={(e) => setPin(e.target.value.replace(/\D/g, '').slice(0, 4))}
                placeholder="••••" className="mono" autoComplete="off"
                onKeyDown={(e) => { if (e.key === 'Enter') abrir() }} />
            </Field>
          </div>
          {/* Fondo inicial: el efectivo en Bs con el que abre la gaveta. Es la
              base para dar vuelto y el punto de partida del arqueo al cerrar. No
              se pide al RETOMAR un turno propio (ya abrió con su fondo). */}
          {sel && !sel.propia ? (
            <div className="mt-3">
              <Field label="Fondo inicial (Bs)" hint="opcional · efectivo con el que abres la gaveta">
                <Input inputMode="decimal" value={fondo}
                  onChange={(e) => setFondo(e.target.value.replace(/[^\d.,]/g, ''))}
                  placeholder="0,00" className="num" autoComplete="off"
                  onKeyDown={(e) => { if (e.key === 'Enter') abrir() }} />
              </Field>
            </div>
          ) : null}
          {error ? (
            <div className="mt-2.5 flex items-start gap-2 text-[12.5px] text-[#B3362C] dark:text-red-400">
              <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" /><span>{error}</span>
            </div>
          ) : null}
        </div>
      </div>
    </Modal>
  )
}

/* Vista de estado vacío que reemplaza al POS cuando no hay turno abierto.
 * Panel punteado, el porqué en lenguaje llano, y un solo botón. */
export function SinCajaAbierta({ onAbrir, puedeAbrir = true }) {
  return (
    <div className="rounded-xl border border-dashed border-slate-300 dark:border-slate-700 py-16 px-6 text-center">
      <div className="h-12 w-12 rounded-icon bg-elerp-50 dark:bg-elerp-900/40 text-elerp-500 dark:text-elerp-200 inline-flex items-center justify-center mb-4">
        <Icon.Wallet size={24} />
      </div>
      <div className="font-display font-semibold text-[17px] text-slate-900 dark:text-slate-100">No tienes una caja abierta</div>
      <p className="text-[13px] text-slate-500 mt-1.5 max-w-md mx-auto">
        Para facturar hace falta abrir una caja con tu código y PIN. Así cada cobro queda
        a nombre de quien lo hizo y el arqueo del día cuadra.
      </p>
      {puedeAbrir ? (
        <div className="mt-5">
          <Button size="lg" icon={<Icon.Lock size={17} />} onClick={onAbrir}>Abrir caja</Button>
        </div>
      ) : (
        <p className="text-[12.5px] text-slate-400 mt-4">Tu rol no puede abrir caja.</p>
      )}
    </div>
  )
}

/* Barra del turno activo: quién cobra, en qué caja, y cómo cerrar. Cerrar abre
 * el arqueo (cuadre de la gaveta) antes de terminar el turno. */
export function BarraTurno({ sesion, onCerrada, onModoCaja }) {
  const [arqueando, setArqueando] = useState(false)
  if (!sesion) return null

  return (
    <div className="flex items-center gap-3 rounded-lg bg-teal-50 dark:bg-teal-500/10 px-3.5 py-2">
      <Icon.Wallet size={16} className="text-teal-600 dark:text-teal-400 shrink-0" />
      <span className="text-[13px] min-w-0 flex-1">
        <span className="font-semibold">{sesion.cajeroNombre}</span>
        <span className="text-slate-500"> · caja </span>
        <span className="mono">{sesion.cajaCodigo}</span>
      </span>
      {onModoCaja ? (
        <Button size="sm" variant="secondary" icon={<Icon.Maximize size={15} />} onClick={onModoCaja}
          title="Abrir el puesto de cobro a pantalla completa">Modo caja</Button>
      ) : null}
      <Button size="sm" variant="ghost" onClick={() => setArqueando(true)}>Cerrar caja</Button>
      <ArqueoModal open={arqueando} sesion={sesion} onClose={() => setArqueando(false)} onCerrada={onCerrada} />
    </div>
  )
}

/* ARQUEO DE CAJA — el cuadre del turno al cerrar (03 §4.4).
 *
 * Muestra lo ESPERADO por método (derivado en el servidor plegando los
 * documentos del turno) y deja DECLARAR el conteo real (al menos el efectivo
 * Bs; el efectivo en divisas se cuenta aparte, no se mezclan monedas). La
 * DIFERENCIA se marca en vivo: verde si cuadra, ámbar sobrante, rojo faltante.
 * Al confirmar cierra el turno con el arqueo, que queda inmutable en la sesión.
 *
 * `avisoCarrito` (opcional) advierte que hay una venta sin cobrar que se
 * perderá al cerrar (lo usa el Modo caja).
 */
export function ArqueoModal({ open, sesion, onClose, onCerrada, avisoCarrito = false }) {
  const toast = useToast()
  const [arqueo, setArqueo] = useState(null)
  const [cargando, setCargando] = useState(true)
  const [error, setError] = useState('')
  const [conteo, setConteo] = useState({}) // clave `${metodo}|${moneda}` → texto
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open || !sesion?.id) return
    setArqueo(null); setError(''); setConteo({}); setCargando(true)
    api.arqueoSesion(sesion.id)
      .then((a) => {
        setArqueo(a)
        // El conteo arranca en el esperado (el cajero corrige lo que difiera):
        // así confirmar sin tocar nada declara «cuadra».
        const inicial = {}
        inicial['efectivo_bs|VES'] = String(a?.efectivoEsperadoBs ?? 0)
        for (const m of a?.metodos || []) {
          if (m.efectivo && m.enDivisa) inicial[`${m.metodo}|${m.moneda}`] = String(m.monto)
        }
        setConteo(inicial)
      })
      .catch((e) => setError(e?.message || 'No se pudo calcular el arqueo.'))
      .finally(() => setCargando(false))
  }, [open, sesion?.id])

  const num = (v) => Math.max(0, parseFloat(String(v ?? '').replace(',', '.')) || 0)
  const efectivoContado = num(conteo['efectivo_bs|VES'])
  const esperadoBs = arqueo?.efectivoEsperadoBs ?? 0
  const diferencia = useMemo(() => Math.round((efectivoContado - esperadoBs) * 100) / 100, [efectivoContado, esperadoBs])

  const cerrar = async () => {
    setBusy(true); setError('')
    try {
      const contadoPorMetodo = []
      for (const m of arqueo?.metodos || []) {
        if (!m.efectivo) continue
        const key = `${m.metodo}|${m.moneda}`
        contadoPorMetodo.push({ metodo: m.metodo, moneda: m.moneda, monto: num(conteo[key]) })
      }
      await api.cerrarCaja(sesion.cajaId, { efectivoContadoBs: efectivoContado, contadoPorMetodo })
      const dif = diferencia
      toast({
        title: 'Caja cerrada',
        body: dif === 0
          ? `${sesion.cajaCodigo} · la gaveta cuadra`
          : `${sesion.cajaCodigo} · ${dif > 0 ? 'sobrante' : 'faltante'} de ${fmtCurrency(Math.abs(dif), 'VES')}`,
      })
      onCerrada?.()
      onClose?.()
    } catch (e) {
      setError(e?.message || 'No se pudo cerrar la caja.')
    } finally {
      setBusy(false)
    }
  }

  // Tono de la diferencia: verde cuadra, ámbar sobrante, rojo faltante.
  const difTono = diferencia === 0
    ? { color: 'emerald', txt: 'text-emerald-700 dark:text-emerald-400', label: 'La gaveta cuadra' }
    : diferencia > 0
      ? { color: 'amber', txt: 'text-amber-700 dark:text-amber-400', label: `Sobrante de ${fmtCurrency(diferencia, 'VES')}` }
      : { color: 'red', txt: 'text-[#B3362C] dark:text-red-400', label: `Faltante de ${fmtCurrency(Math.abs(diferencia), 'VES')}` }

  const efectivosDivisa = (arqueo?.metodos || []).filter((m) => m.efectivo && m.enDivisa)
  const electronicos = (arqueo?.metodos || []).filter((m) => !m.efectivo)

  return (
    <Modal open={open} onClose={onClose} icon={<Icon.Wallet size={18} />}
      title="Arqueo de caja"
      sub="Cuadra la gaveta contra lo cobrado en el turno antes de cerrar."
      footer={
        <div className="flex items-center justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button onClick={cerrar} loading={busy} disabled={cargando || !!error}
            icon={<Icon.Lock size={16} />}>Cerrar caja</Button>
        </div>
      }>
      {cargando ? (
        <div className="space-y-2">
          <div className="skeleton h-16 rounded-lg" /><div className="skeleton h-24 rounded-lg" />
        </div>
      ) : error ? (
        <div className="rounded-lg bg-red-50 dark:bg-red-900/25 text-[#B3362C] dark:text-red-300 px-3 py-2.5 text-[13px]">{error}</div>
      ) : arqueo ? (
        <div className="space-y-4">
          {avisoCarrito ? (
            <div className="rounded-lg bg-amber-50 dark:bg-amber-900/25 text-amber-800 dark:text-amber-300 px-3 py-2.5 text-[12.5px]">
              Hay una venta sin cobrar en el carrito que se perderá al cerrar.
            </div>
          ) : null}

          {/* Efectivo Bs esperado en la gaveta: fondo + cobros − vuelto. */}
          <div className="rounded-xl border border-slate-200 dark:border-slate-800 p-3.5">
            <div className="text-[12px] font-semibold text-slate-500 uppercase tracking-wide mb-2">Efectivo en bolívares</div>
            <div className="space-y-1 text-[13px]">
              <FilaArqueo label="Fondo inicial" valor={arqueo.fondoInicial} />
              <FilaArqueo label="Cobros en efectivo Bs" valor={arqueo.cobrosEfectivoBs} />
              <FilaArqueo label="Vuelto entregado en efectivo Bs" valor={-arqueo.vueltoEfectivoBs} />
              <div className="flex items-center justify-between pt-1.5 border-t border-slate-100 dark:border-slate-800 font-semibold">
                <span>Efectivo Bs esperado</span>
                <span className="num">{fmtCurrency(esperadoBs, 'VES')}</span>
              </div>
            </div>
            <div className="mt-3">
              <Field label="Efectivo Bs contado en la gaveta" required>
                <Input inputMode="decimal" value={conteo['efectivo_bs|VES'] ?? ''}
                  onChange={(e) => setConteo((c) => ({ ...c, 'efectivo_bs|VES': e.target.value.replace(/[^\d.,]/g, '') }))}
                  className="num" autoFocus />
              </Field>
              <div className={`mt-2 flex items-center gap-2 text-[13px] font-semibold ${difTono.txt}`}>
                {diferencia === 0 ? <Icon.Check size={16} /> : <Icon.CircleAlert size={16} />}
                <span>{difTono.label}</span>
              </div>
            </div>
          </div>

          {/* Efectivo en divisas: se cuenta aparte del bolívar. */}
          {efectivosDivisa.length ? (
            <div className="rounded-xl border border-slate-200 dark:border-slate-800 p-3.5">
              <div className="text-[12px] font-semibold text-slate-500 uppercase tracking-wide mb-2">Efectivo en divisas</div>
              <div className="space-y-2.5">
                {efectivosDivisa.map((m) => {
                  const key = `${m.metodo}|${m.moneda}`
                  const cont = num(conteo[key])
                  const dif = Math.round((cont - m.monto) * 100) / 100
                  return (
                    <div key={key} className="grid grid-cols-[1fr_auto] items-end gap-3">
                      <div>
                        <div className="text-[13px] font-medium">{metodoLabel(m.metodo)}</div>
                        <div className="text-[11.5px] text-slate-500 num">
                          Esperado {fmtNum(m.monto, 2)} {m.moneda} · {fmtCurrency(m.equivalenteBs, 'VES')}
                        </div>
                      </div>
                      <div className="w-32">
                        <Input inputMode="decimal" value={conteo[key] ?? ''}
                          onChange={(e) => setConteo((c) => ({ ...c, [key]: e.target.value.replace(/[^\d.,]/g, '') }))}
                          className="num text-right" />
                        <div className={`text-[11px] text-right mt-0.5 ${dif === 0 ? 'text-slate-400' : dif > 0 ? 'text-amber-600 dark:text-amber-400' : 'text-[#B3362C] dark:text-red-400'}`}>
                          {dif === 0 ? 'cuadra' : `${dif > 0 ? '+' : '−'}${fmtNum(Math.abs(dif), 2)} ${m.moneda}`}
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          ) : null}

          {/* Medios electrónicos: solo lo esperado (se concilian contra el punto/banco). */}
          {electronicos.length ? (
            <div className="rounded-xl border border-slate-200 dark:border-slate-800 p-3.5">
              <div className="text-[12px] font-semibold text-slate-500 uppercase tracking-wide mb-2">Medios electrónicos (esperado)</div>
              <div className="space-y-1.5">
                {electronicos.map((m) => (
                  <div key={`${m.metodo}|${m.moneda}`} className="flex items-center justify-between text-[13px]">
                    <span className="text-slate-600 dark:text-slate-300">{metodoLabel(m.metodo)}</span>
                    <span className="num">{fmtCurrency(m.equivalenteBs, 'VES')}</span>
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          <div className="text-[11.5px] text-slate-400">
            {arqueo.documentos} documento(s) en este turno · total cobrado {fmtCurrency(arqueo.totalCobradoBs, 'VES')}.
            El arqueo queda registrado con el turno y no se puede editar después.
          </div>
        </div>
      ) : null}
    </Modal>
  )
}

const FilaArqueo = ({ label, valor }) => (
  <div className="flex items-center justify-between text-slate-500">
    <span>{label}</span><span className="num">{fmtCurrency(valor, 'VES')}</span>
  </div>
)
