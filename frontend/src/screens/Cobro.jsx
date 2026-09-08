import { useState, useMemo, useEffect, useRef } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Modal, Select, useToast } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { calcularTotales, totalPagosEnBs, excedenteBs, vueltoDefaultMoneda, IVA_TASA, IGTF_TASA } from '../lib/fiscal.js'
import { monedaLabel } from '../lib/precio.js'
import { useData, useTasa } from '../context/DataContext.jsx'
import { api } from '../lib/api.js'
import { fechaCortaVE } from '../components/tasa.jsx'
import { usePosCanal } from './PantallaCliente.jsx'
import { ImpresionFiscalModal, impresoraActivaDeSede } from './ImpresionFiscal.jsx'
import { ComprobanteModal, comprobanteDeFactura } from '../components/ComprobantePDF.jsx'

/* Cobro (modal) — la vista que manda el prototipo («Modal de cobro»).
 *
 * El flujo es: se elige CÓMO paga el cliente, y según el método aparece lo que
 * ese método necesita. Tres cosas que antes faltaban y son el corazón del
 * mostrador venezolano:
 *
 *   1. **Vuelto.** Se escribe cuánto entregó el cliente y el vuelto sale en vivo.
 *      El servidor lo recalcula y lo guarda en el documento: es dinero que sale
 *      de la gaveta, no un número de pantalla.
 *   2. **Multi-moneda.** Efectivo en dólares se captura en dólares y el IGTF del
 *      3% se avisa antes de cobrar, no después.
 *   3. **Cobro mixto.** Un poco por un lado y un poco por el otro, cada parte en
 *      su cuenta y en su moneda, con «cubierto / falta» en vivo.
 */

// Comportamiento de cada tipo de método: qué campos pide en el cobro. La lista
// visible ahora sale de la configuración (Ajustes → Métodos de pago); estos
// flags derivan del `tipo` de cada método configurado.
const FLAGS_TIPO = {
  efectivo_bs: { icon: 'Banknote', efectivo: true },
  efectivo_usd: { icon: 'Banknote', efectivo: true },
  tarjeta: { icon: 'Wallet', cuenta: true },
  pago_movil: { icon: 'Smartphone', cuenta: true, referencia: true },
  transferencia: { icon: 'Bank', cuenta: true, referencia: true },
  zelle: { icon: 'Globe', cuenta: true, referencia: true },
}

// Convierte un método configurado (contrato REST) en la tarjeta que usa el
// modal. `id` es el del método configurado (único); `tipo` es el enum que se le
// manda al servidor. `divisa` (causa IGTF) se deriva de la moneda.
function entradaDeMetodo(m) {
  const f = FLAGS_TIPO[m.tipo] || { icon: 'Wallet' }
  const moneda = (m.moneda || 'VES').toUpperCase()
  return {
    id: m.id, tipo: m.tipo, t: m.nombre, icon: f.icon || 'Wallet', moneda,
    efectivo: !!f.efectivo, cuenta: !!f.cuenta, referencia: !!f.referencia,
    // Cualquier moneda distinta de VES es divisa (causa IGTF), no solo el dólar.
    divisa: moneda !== 'VES', cuentaCobroId: m.cuentaCobroId || '',
  }
}

// Tarjeta de «Efectivo <divisa>» generada a partir de una divisa activa. El
// `tipo`/id son `efectivo_<codigo>` — para USD queda `efectivo_usd`, idéntico al
// método estándar de siempre (retrocompat). El servidor no valida el enum: lo que
// causa IGTF y fija la conversión es la `moneda`.
function efectivoDivisa(codigo) {
  const c = (codigo || '').toUpperCase()
  const id = `efectivo_${c.toLowerCase()}`
  return { id, tipo: id, t: `Efectivo ${monedaLabel(c)}`, icon: 'Banknote', moneda: c, efectivo: true, divisa: true }
}

// Fallback: los métodos estándar de siempre. Se usan solo si la empresa no tiene
// métodos configurados con enCaja && activo (así el POS nunca se queda sin cómo
// cobrar mientras el backend de configuración aún no responde). El efectivo en
// divisas ya NO se codifica aquí: se genera desde las divisas activas (abajo).
const METODOS_FALLBACK = [
  { id: 'efectivo_bs', tipo: 'efectivo_bs', t: 'Efectivo Bs', icon: 'Banknote', moneda: 'VES', efectivo: true },
  { id: 'tarjeta', tipo: 'tarjeta', t: 'Punto de venta', icon: 'Wallet', moneda: 'VES', cuenta: true },
  { id: 'pago_movil', tipo: 'pago_movil', t: 'Pago móvil', icon: 'Smartphone', moneda: 'VES', cuenta: true, referencia: true },
  { id: 'transferencia', tipo: 'transferencia', t: 'Transferencia', icon: 'Bank', moneda: 'VES', cuenta: true, referencia: true },
  { id: 'zelle', tipo: 'zelle', t: 'Zelle', icon: 'Globe', moneda: 'USD', cuenta: true, divisa: true, referencia: true },
]

// Bancos venezolanos con su código (el que exige el pago móvil). Lista de los
// principales; el valor guardado es el código de 4 dígitos.
const BANCOS_VE = [
  { codigo: '0102', nombre: 'Banco de Venezuela' },
  { codigo: '0104', nombre: 'Venezolano de Crédito' },
  { codigo: '0105', nombre: 'Mercantil' },
  { codigo: '0108', nombre: 'Provincial (BBVA)' },
  { codigo: '0114', nombre: 'Bancaribe' },
  { codigo: '0115', nombre: 'Exterior' },
  { codigo: '0116', nombre: 'BOD' },
  { codigo: '0128', nombre: 'Banco Caroní' },
  { codigo: '0134', nombre: 'Banesco' },
  { codigo: '0137', nombre: 'Sofitasa' },
  { codigo: '0138', nombre: 'Banco Plaza' },
  { codigo: '0151', nombre: 'BFC Banco Fondo Común' },
  { codigo: '0156', nombre: '100% Banco' },
  { codigo: '0163', nombre: 'Banco del Tesoro' },
  { codigo: '0166', nombre: 'Banco Agrícola de Venezuela' },
  { codigo: '0168', nombre: 'Bancrecer' },
  { codigo: '0169', nombre: 'Mi Banco' },
  { codigo: '0171', nombre: 'Banco Activo' },
  { codigo: '0172', nombre: 'Bancamiga' },
  { codigo: '0174', nombre: 'Banplus' },
  { codigo: '0175', nombre: 'Banco Bicentenario' },
  { codigo: '0191', nombre: 'BNC Banco Nacional de Crédito' },
]

// Modos especiales que no son «un método» sino formas de cobrar; van siempre,
// pase lo que pase con la configuración.
const METODOS_ESPECIALES = [
  { id: 'mixto', tipo: 'mixto', t: 'Mixto', icon: 'Shuffle', mixto: true },
  // Crédito: la factura se emite y el saldo queda por cobrar en Tesorería. Exige
  // cliente identificado — al consumidor final no se le fía.
  { id: 'credito', tipo: 'credito', t: 'A crédito', icon: 'Receipt', credito: true },
]

// Texto de ayuda de los métodos que no piden nada más que confirmar.
const AYUDA = {
  tarjeta: 'Pasa la tarjeta por tu punto de venta y confirma la operación. El pago queda asociado a la factura.',
  transferencia: 'El pago queda registrado con su referencia. La conciliación con el banco llega con Tesorería.',
  pago_movil: 'Anota la referencia que te dé el cliente. La verificación automática con el banco llega con Tesorería.',
  zelle: 'Cobro en divisas: causa IGTF del 3%. Anota la referencia del envío.',
}

// Plazos de crédito habituales entre comercios.
const PLAZOS = [15, 30, 45, 60]

// Redondeo a dos decimales para el preview del vuelto (el servidor recalcula).
const round2 = (v) => Math.round((Number(v) || 0) * 100) / 100

export function CobroModal({ open, onClose, lineas, clienteId, clienteNombre, contingencia, onEmitida, canal = 'caja', onCobrar, cuponCodigo = '' }) {
  const { db, tasaDe } = useData()
  const tasa = useTasa()
  const toast = useToast()
  const cuentas = db.CUENTAS_COBRO || []

  // Divisas activas de la empresa (multimoneda). Sin db.TASAS (backend viejo) se
  // cae a solo el dólar: así el POS de siempre no cambia.
  const activasDivisas = useMemo(() => {
    const a = db.TASAS?.activas
    const codigos = Array.isArray(a) && a.length
      ? a.map((x) => (x.codigo || '').toUpperCase()).filter((c) => c && c !== 'VES')
      : ['USD']
    return [...new Set(codigos)]
  }, [db.TASAS])

  // Métodos visibles en el POS: los configurados con enCaja && activo, ordenados
  // por `orden`. Si no hay ninguno (empresa sin configurar o backend en
  // construcción), se cae al set estándar. Además, se ofrece pagar en EFECTIVO en
  // cada divisa activa (generado dinámicamente). Los modos especiales van siempre.
  // El canal decide qué métodos se ofrecen: el mostrador (POS) usa los marcados
  // `enCaja`; el módulo Ventas (facturado de una cotización) usa los `enVentas`.
  const flagCanal = canal === 'ventas' ? 'enVentas' : 'enCaja'
  const METODOS = useMemo(() => {
    const conf = (db.METODOS_PAGO || [])
      .filter((m) => m[flagCanal] && m.activo)
      .sort((a, b) => (a.orden || 0) - (b.orden || 0))
      .map(entradaDeMetodo)
    const base = conf.length ? conf : METODOS_FALLBACK
    // Divisas que ya tienen un efectivo (configurado o del fallback): no se duplica.
    const efectivoCubierto = new Set(base.filter((m) => m.efectivo).map((m) => m.moneda))
    const extraEfectivo = activasDivisas.filter((c) => !efectivoCubierto.has(c)).map(efectivoDivisa)
    // Se insertan justo después del último efectivo para agrupar los efectivos
    // (Efectivo Bs, Efectivo US$, Efectivo €, …), como en el mostrador de hoy.
    const idx = base.reduce((last, m, i) => (m.efectivo ? i : last), -1)
    const merged = idx >= 0
      ? [...base.slice(0, idx + 1), ...extraEfectivo, ...base.slice(idx + 1)]
      : [...base, ...extraEfectivo]
    return [...merged, ...METODOS_ESPECIALES]
  }, [db.METODOS_PAGO, activasDivisas, flagCanal])
  // Métodos «reales» (sin mixto/crédito), para los selectores del cobro mixto.
  const metodosReales = METODOS.filter((m) => !m.mixto && !m.credito)
  const primerMetodoId = metodosReales[0]?.id || 'efectivo_bs'

  const [metodo, setMetodo] = useState('')
  const [recibido, setRecibido] = useState('')
  const [cuentaId, setCuentaId] = useState('')
  const [referencia, setReferencia] = useState('')
  const [mix, setMix] = useState([{ metodo: primerMetodoId, cuentaId: '', monto: '' }])
  // Crédito: abono inicial (puede ser cero) y plazo.
  const [plazo, setPlazo] = useState(30)
  // Vuelto MIXTO: el excedente puede repartirse en varias PARTES, cada una con su
  // moneda y su medio (efectivo / pago móvil) y, por parte, los datos de la
  // transferencia. Cada parte lleva su estado de ejecución del pago móvil
  // (pendiente → procesando → listo | error) que bloquea «Emitir» hasta «listo».
  const [vueltoPartes, setVueltoPartes] = useState([])
  const [busy, setBusy] = useState(false)
  const recibidoRef = useRef(null)

  // Al abrir se limpia todo: un cobro nunca hereda datos del anterior.
  useEffect(() => {
    if (!open) return
    setMetodo(''); setRecibido(''); setCuentaId(''); setReferencia('')
    setMix([{ metodo: primerMetodoId, cuentaId: '', monto: '' }]); setBusy(false)
    setVueltoPartes([])
  }, [open, primerMetodoId])

  const meta = METODOS.find((m) => m.id === metodo)

  // Pagos que se enviarán al servidor, según el método elegido.
  const pagos = useMemo(() => {
    if (!meta) return []
    if (meta.credito) {
      // El abono inicial es opcional: puede irse sin pagar nada.
      const abono = Number(recibido) || 0
      return abono > 0 ? [{ metodo: 'efectivo_bs', cuentaId, monto: abono, moneda: 'VES' }] : []
    }
    if (meta.mixto) {
      return mix
        .filter((l) => Number(l.monto) > 0)
        .map((l) => {
          const mm = METODOS.find((x) => x.id === l.metodo)
          // Al servidor se le manda el `tipo` (enum), no el id del método configurado.
          return { metodo: mm?.tipo || l.metodo, cuentaId: l.cuentaId, monto: Number(l.monto), moneda: mm?.moneda || 'VES' }
        })
    }
    const monto = Number(recibido) || 0
    if (monto <= 0) return []
    return [{ metodo: meta.tipo, cuentaId, monto, moneda: meta.moneda }]
  }, [meta, metodo, recibido, cuentaId, mix, METODOS])

  // Totales con las reglas del servidor (IVA solo sobre lo gravado, IGTF solo
  // sobre la porción de la factura pagada en divisas). Cada pago se convierte con
  // la tasa de SU divisa: por eso se pasa el resolutor `tasaDe`, no una tasa única.
  const totales = useMemo(() => calcularTotales(lineas, pagos, tasaDe), [lineas, pagos, tasaDe])
  const cobrado = totalPagosEnBs(pagos, tasaDe)
  const falta = totales.total - cobrado
  // ¿Aún falta por cubrir? (con la misma tolerancia que la validación). Gobierna
  // si se puede AÑADIR otro método en el cobro mixto.
  const faltaPorCobrar = falta > 0.005

  // Añadir un método al cobro mixto: solo tiene sentido si aún falta por cubrir.
  // La línea nueva se SUGIERE con el restante (falta en Bs), convertido a la
  // moneda del método por defecto; el cajero puede ajustarlo. Así el segundo pago
  // aparece ya con lo que queda, y no se ofrece añadir más cuando no falta nada.
  const agregarPago = () => {
    const m = METODOS.find((x) => x.id === primerMetodoId)
    const t = m?.divisa ? tasaDe(m.moneda) : 1
    const sugerido = faltaPorCobrar && t > 0 ? String(round2(falta / t)) : ''
    setMix((s) => [...s, { metodo: primerMetodoId, cuentaId: '', monto: sugerido }])
  }

  // Vuelto MIXTO: el excedente en Bs, la moneda por defecto (la del efectivo en
  // divisa entregado, si lo hubo) y las partes en que se reparte.
  const excedente = excedenteBs(pagos, totales.total, tasaDe)
  const hayVuelto = excedente > 0.004
  const vueltoMonedaDefault = vueltoDefaultMoneda(pagos)
  // Monedas ofrecibles para el vuelto: Bs siempre + las divisas activas CON tasa
  // (sin tasa el servidor lo rechaza; no se ofrece devolver algo inconvertible).
  const monedasVuelto = ['VES', ...activasDivisas.filter((c) => tasaDe(c) > 0)]

  // Partes del vuelto resueltas: cada una con su tasa y su equivalente en Bs. En el
  // caso SIMPLE (una sola parte) el monto es TODO el excedente en la moneda de la
  // parte (derivado, como siempre); al REPARTIR (2+ partes) cada monto lo teclea el
  // cajero y se exige que las partes cubran EXACTAMENTE el vuelto.
  const simpleVuelto = vueltoPartes.length <= 1
  const partesCalc = useMemo(() => vueltoPartes.map((p) => {
    const moneda = p.moneda || vueltoMonedaDefault
    const tasa = moneda === 'VES' ? 1 : tasaDe(moneda)
    const tasaOk = tasa > 0
    let montoBs, monto
    if (simpleVuelto) {
      montoBs = excedente
      monto = tasaOk ? round2(excedente / tasa) : null
    } else {
      monto = Number(p.monto) || 0
      montoBs = tasaOk ? round2(monto * tasa) : 0
    }
    return { ...p, moneda, tasa, tasaOk, monto, montoBs }
  }), [vueltoPartes, simpleVuelto, excedente, vueltoMonedaDefault, tasaDe])

  const cubiertoBs = round2(partesCalc.reduce((a, p) => a + (p.montoBs || 0), 0))
  const faltaVuelto = round2(excedente - cubiertoBs)
  // Tolerancia igual que el servidor: un céntimo de cada moneda (por su tasa).
  const tolVuelto = 0.01 + partesCalc.reduce((a, p) => a + 0.01 * (p.tasaOk ? p.tasa : 1), 0)
  const vueltoCuadra = !hayVuelto || Math.abs(faltaVuelto) <= tolVuelto
  const vueltoSinTasa = partesCalc.find((p) => !p.tasaOk)

  // Resumen {monto, moneda} + partes para las pistas «Vuelto: …» en vivo y para la
  // pantalla del cliente. Caso simple: la única parte; repartido: el total en Bs.
  const vuelto = !hayVuelto ? null
    : (simpleVuelto && partesCalc[0] && partesCalc[0].tasaOk
        ? { monto: partesCalc[0].monto, moneda: partesCalc[0].moneda }
        : { monto: excedente, moneda: 'VES' })
  const vueltoPartesResumen = partesCalc.map((p) => ({ monto: p.monto, moneda: p.moneda, metodo: p.metodo, montoBs: p.montoBs }))

  /* Espejo del cobro hacia la PANTALLA DEL CLIENTE (segunda ventana). Mientras el
   * modal está abierto se publica el total, lo recibido y el vuelto; al cerrarlo
   * se avisa que el cobro terminó (el display vuelve al carrito / bienvenida).
   * Si el display se abre a mitad del cobro, saluda y reemitimos. */
  const publicarPos = usePosCanal((m) => { if (m?.tipo === 'hello' && open) emitirCobro() })
  const emitirCobro = () => {
    if (!open) { publicarPos({ tipo: 'cobro', activo: false }); return }
    publicarPos({
      tipo: 'cobro', activo: true,
      total: totales.total,
      recibido: Number(recibido) || 0,
      recibidoMoneda: meta?.divisa ? meta.moneda : 'VES',
      // Vuelto con su desglose por parte (para el display, si es mixto).
      vuelto: vuelto ? { monto: vuelto.monto, moneda: vuelto.moneda, montoBs: excedente, partes: vueltoPartesResumen } : null,
      // IGTF del pago en divisas: el cliente ve cuánto se le cobra por IGTF.
      igtf: totales.igtf || 0,
      falta, cobrado,
    })
  }
  useEffect(() => { emitirCobro() },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [open, recibido, meta?.moneda, meta?.divisa, totales.total, totales.igtf, falta, cobrado, vuelto?.monto, vuelto?.moneda, cubiertoBs, vueltoPartes.length])

  // Cuánto hay que pedirle al cliente, en la moneda del método elegido.
  const tasaMetodo = meta?.divisa ? tasaDe(meta.moneda) : 1
  const aPedir = meta?.divisa && tasaMetodo > 0 ? totales.total / tasaMetodo : totales.total
  const monedaPedir = meta?.divisa ? meta.moneda : 'VES'

  // Sin tasa: cualquier pago en divisa cuya tasa no esté cargada bloquea el cobro.
  const divisaSinTasa = (() => {
    const cand = []
    if (meta?.divisa) cand.push(meta.moneda)
    if (meta?.mixto) mix.forEach((l) => { const mm = METODOS.find((x) => x.id === l.metodo); if (mm?.divisa) cand.push(mm.moneda) })
    return cand.find((m) => !(tasaDe(m) > 0)) || ''
  })()
  const sinTasa = !!divisaSinTasa

  const cuentasDe = (mid) => {
    const mm = METODOS.find((x) => x.id === mid)
    return cuentas.filter((c) => c.moneda === (mm?.moneda || 'VES'))
  }

  // Datos completos de una parte por pago móvil (para poder instruir el envío).
  const pmDatosCompletos = (p) => !!p.banco && !!String(p.cedula || '').trim() && !!String(p.telefono || '').trim()
  const partesPagoMovil = partesCalc.filter((p) => p.metodo === 'pago_movil')
  const faltanDatosPagoMovil = partesPagoMovil.some((p) => !pmDatosCompletos(p))
  // Cada parte por pago móvil (con datos) debe quedar en «listo» antes de emitir.
  const vueltoBloquea = partesPagoMovil.some((p) => pmDatosCompletos(p) && p.estado !== 'listo')

  // Vuelto por defecto: UNA sola parte que cubre TODO el excedente, con la moneda
  // derivada del pago (efectivo en divisa ⇒ esa divisa; si no, Bs). Se siembra al
  // aparecer el vuelto y se limpia cuando ya no hay excedente. No complica el caso
  // simple: una parte basta y su monto se deriva solo.
  useEffect(() => {
    setVueltoPartes((prev) => {
      if (!hayVuelto) return prev.length ? [] : prev
      if (prev.length) return prev
      return [{ moneda: vueltoMonedaDefault, metodo: 'efectivo', monto: '', banco: '', cedula: '', telefono: '', estado: 'pendiente' }]
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hayVuelto])

  // Bs de una parte del vuelto: su monto × la tasa de su moneda.
  const bsDeParte = (p) => (Number(p.monto) || 0) * (p.moneda === 'VES' ? 1 : (tasaDe(p.moneda) || 0))
  // Rebalanceo del vuelto: la ÚLTIMA parte que el cajero NO fijó a mano absorbe el
  // resto del excedente (en su moneda). Así, al teclear una parte o cambiar su
  // moneda, la otra se autocompleta con lo que falta para cuadrar; las partes
  // marcadas manuales (el cajero tecleó su monto) se respetan.
  const rebalancear = (parts) => {
    if (parts.length <= 1) return parts
    let idx = -1
    for (let k = parts.length - 1; k >= 0; k--) if (!parts[k].montoManual) { idx = k; break }
    if (idx < 0) return parts
    const otrosBs = parts.reduce((a, p, j) => (j === idx ? a : a + bsDeParte(p)), 0)
    const restBs = round2(excedente - otrosBs)
    const p = parts[idx]
    const t = p.moneda === 'VES' ? 1 : (tasaDe(p.moneda) || 0)
    const monto = t > 0 && restBs > 0 ? round2(restBs / t) : 0
    return parts.map((x, j) => (j === idx ? { ...x, monto: monto ? String(monto) : '' } : x))
  }

  // Editar datos de una parte (medio/banco/cédula/teléfono) reinicia su estado de
  // pago móvil: cualquier cambio obliga a volver a procesar el envío.
  const editarParte = (i, patch) =>
    setVueltoPartes((s) => s.map((x, j) => (j === i ? { ...x, ...patch, estado: 'pendiente' } : x)))
  // Teclear el monto de una parte la marca MANUAL (el cajero la fijó) y reparte el
  // resto en la parte automática.
  const cambiarMontoParte = (i, val) =>
    setVueltoPartes((s) => rebalancear(s.map((x, j) => (j === i ? { ...x, monto: val, montoManual: true, estado: 'pendiente' } : x))))
  // Elegir la moneda de una parte. En divisa (US$, €…) NO hay pago móvil (solo
  // existe en Bs): se fuerza efectivo. Si la parte no es manual, se le sugiere el
  // restante en la nueva moneda (lo pedido: al elegir Bs aparece cuánto falta).
  const seleccionarMoneda = (i, c) =>
    setVueltoPartes((s) => rebalancear(s.map((x, j) => (j === i
      ? { ...x, moneda: c, metodo: c !== 'VES' ? 'efectivo' : x.metodo, estado: 'pendiente' }
      : x))))
  const quitarParte = (i) => setVueltoPartes((s) => rebalancear(s.filter((_, j) => j !== i)))
  const agregarParte = () =>
    setVueltoPartes((s) => {
      let base = s
      if (s.length === 1) {
        // Al pasar a reparto, fija el monto de la primera parte en el vuelto
        // completo (su valor actual) y la marca MANUAL: el cajero la ajusta y la
        // segunda parte absorbe el resto automáticamente.
        const p = s[0]
        const moneda = p.moneda || vueltoMonedaDefault
        const t = moneda === 'VES' ? 1 : tasaDe(moneda)
        const monto = t > 0 ? round2(excedente / t) : 0
        base = [{ ...p, moneda, monto: monto ? String(monto) : '', montoManual: true }]
      }
      return rebalancear([...base, { moneda: 'VES', metodo: 'efectivo', monto: '', banco: '', cedula: '', telefono: '', estado: 'pendiente', montoManual: false }])
    })

  // PUNTO DE INTEGRACIÓN — envío del vuelto por pago móvil al cliente, POR PARTE.
  // Hoy es un STUB: simula la llamada (~1.2s) y resuelve «listo». Aquí se conectará
  // la API bancaria de pago móvil (débito de la cuenta del comercio a los datos
  // capturados: banco, cédula, teléfono). El error deja el botón «Reintentar».
  const procesarParte = async (i) => {
    setVueltoPartes((s) => s.map((x, j) => (j === i ? { ...x, estado: 'procesando' } : x)))
    try {
      // TODO(integración): reemplazar por la llamada real a la pasarela de pago móvil.
      await new Promise((resolve) => setTimeout(resolve, 1200))
      setVueltoPartes((s) => s.map((x, j) => (j === i ? { ...x, estado: 'listo' } : x)))
    } catch {
      setVueltoPartes((s) => s.map((x, j) => (j === i ? { ...x, estado: 'error' } : x)))
    }
  }

  // Validación: se dice qué falta, en vez de dejar emitir algo descuadrado.
  let error = ''
  if (!meta) error = 'Elige cómo te van a pagar.'
  else if (meta.credito && !clienteId) error = 'Una venta a crédito necesita un cliente identificado: elígelo arriba, en el punto de venta.'
  else if (meta.credito && (Number(recibido) || 0) > totales.total + 0.005) error = 'El abono no puede ser mayor que el total.'
  else if (meta.credito) error = ''
  else if (sinTasa) error = `No hay tasa de cambio cargada para ${monedaLabel(divisaSinTasa)}: no se puede cobrar en esa divisa.`
  else if (pagos.length === 0) error = 'Registra el monto que te están pagando.'
  else if (falta > 0.005) error = `Todavía faltan ${fmtCurrency(falta, 'VES')} para cubrir el total.`
  else if (meta.cuenta && !meta.mixto && !cuentaId) error = 'Elige en qué cuenta entra el pago.'
  else if (hayVuelto && vueltoSinTasa) error = `No hay tasa para devolver el vuelto en ${monedaLabel(vueltoSinTasa.moneda)}.`
  else if (hayVuelto && faltanDatosPagoMovil) error = 'Para el vuelto por pago móvil, completa banco, cédula y teléfono del cliente.'
  else if (hayVuelto && !vueltoCuadra) error = faltaVuelto > 0
    ? `Aún falta repartir ${fmtCurrency(faltaVuelto, 'VES')} del vuelto.`
    : `Te pasaste del vuelto por ${fmtCurrency(-faltaVuelto, 'VES')}.`
  else if (vueltoBloquea) error = 'Procesa el vuelto por pago móvil antes de emitir la factura.'

  const emitir = async () => {
    if (error) return
    setBusy(true)
    try {
      // Cobro que se envía al servidor. Es idéntico para el POS y para Ventas: los
      // mismos pagos + vuelto + crédito. Lo que cambia es la RUTA de emisión —el
      // POS emite un documento nuevo; Ventas factura una cotización existente— así
      // que quien recibe el cobro se inyecta con `onCobrar`; sin él, es el POS.
      const cobro = {
        clienteId: clienteId || '',
        lineas: lineas.map((l) => ({ sku: l.sku, cantidad: Number(l.cantidad), precioUnitario: Number(l.precioUnitario) })),
        pagos: pagos.map((p) => ({ metodo: p.metodo, cuentaId: p.cuentaId, monto: p.monto, moneda: p.moneda, referencia })),
        moneda: 'VES',
        contingencia,
        credito: !!meta?.credito,
        diasCredito: meta?.credito ? plazo : 0,
        // Cupón aplicado: el precio de las líneas ya viene descontado; el código se
        // envía sólo para que el servidor CONSUMA el uso del cupón (tope UsosMax).
        ...(cuponCodigo ? { cuponCodigo } : {}),
        // Vuelto declarado por la caja como PARTES (moneda + medio + monto en esa
        // moneda). Solo se manda cuando hay excedente; sin él el servidor no calcula
        // vuelto y el cobro se comporta igual que siempre. El servidor valida que la
        // suma en Bs cuadre con el excedente.
        ...(hayVuelto ? {
          vueltoPartes: partesCalc.map((p) => ({
            moneda: p.moneda, metodo: p.metodo, monto: p.monto,
            ...(p.metodo === 'pago_movil'
              ? { banco: p.banco, cedula: String(p.cedula || '').trim(), telefono: String(p.telefono || '').trim() }
              : {}),
          })),
        } : {}),
      }
      const doc = onCobrar ? await onCobrar(cobro) : await api.emitirDocumento(cobro)
      onEmitida(doc)
    } catch (e) {
      toast({ title: 'No se pudo emitir', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open={open} onClose={onClose} size="pos" icon={<Icon.Banknote size={18} />}
      title="Cobrar" sub={`${clienteNombre || 'Consumidor final'} · ${fmtNum(lineas.length, 0)} ítem(s)`}
      footer={
        <>
          <div className="flex-1 min-w-[170px] text-[13px] text-slate-500">
            <div className="flex justify-between"><span>Documento</span><span className="num">{fmtCurrency(totales.total - totales.igtf, 'VES')}</span></div>
            {totales.igtf > 0 ? (
              <div className="flex justify-between text-amber-700 dark:text-amber-400"><span>IGTF 3%</span><span className="num">{fmtCurrency(totales.igtf, 'VES')}</span></div>
            ) : null}
            <div className="flex justify-between text-[17px] font-semibold text-slate-900 dark:text-slate-100 mt-0.5">
              <span>A pagar</span><span className="num">{fmtCurrency(totales.total, 'VES')}</span>
            </div>
          </div>
          <Button variant="ghost" size="lg" onClick={onClose}>Cancelar</Button>
          <Button variant="dinero" size="lg" className="!h-[52px] !text-[15px] px-7" loading={busy} disabled={!!error}
            title={error || 'Emitir la factura legal'} onClick={emitir} icon={<Icon.Banknote size={19} />}>
            {meta?.credito ? 'Emitir a crédito' : 'Emitir factura'}
          </Button>
        </>
      }>
      {/* Layout de 2 columnas en pantallas grandes (tablet/escritorio del cajero):
          izquierda = cómo paga el cliente + los totales del documento (debajo de
          la selección del método); derecha = todo lo relativo al vuelto. Separar
          ambas columnas acorta el alto y evita el scroll dentro del modal. */}
      <div className="grid lg:grid-cols-[1.4fr_1fr] gap-4 lg:gap-5">
        {/* Columna izquierda — cómo paga el cliente + los totales del documento. */}
        <div className="space-y-4">
        {/* 1 · Método */}
        <div>
          <div className="text-[14px] font-semibold mb-2.5">¿Cómo te van a pagar?</div>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-2.5">
            {METODOS.map((m) => {
              const IconC = Icon[m.icon] || Icon.Wallet
              const activo = metodo === m.id
              return (
                <button key={m.id} onClick={() => { setMetodo(m.id); setRecibido(''); setCuentaId(m.cuentaCobroId || '') }}
                  className={`rounded-xl border p-3 min-h-[80px] flex flex-col items-center justify-center gap-2 transition-colors
                    ${activo ? 'border-elerp-500 bg-elerp-50 dark:bg-elerp-900/40 ring-1 ring-elerp-500' : 'border-slate-200 dark:border-slate-700 hover:border-elerp-300'}`}>
                  <IconC size={24} className={activo ? 'text-elerp-600 dark:text-elerp-300' : 'text-elerp-500/80'} />
                  <span className="text-[13px] font-semibold text-center leading-tight">{m.t}</span>
                </button>
              )
            })}
          </div>
        </div>

        {/* 2 · Lo que pide el método elegido */}
        {meta?.efectivo ? (
          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-3.5">
            <label className="block text-[13.5px] font-semibold mb-1.5">
              Monto recibido ({monedaLabel(meta.moneda)})
            </label>
            <input ref={recibidoRef} type="number" min="0" step="0.01" inputMode="decimal" autoFocus
              value={recibido} onChange={(e) => setRecibido(e.target.value)} placeholder="0,00"
              className="w-full h-14 px-3.5 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-[23px] num text-right ring-focus" />
            <div className="mt-2.5 flex items-center justify-between gap-3">
              <button type="button" onClick={() => setRecibido(String(Math.ceil(aPedir * 100) / 100))}
                className="h-9 px-3 rounded-lg border border-elerp-200 dark:border-elerp-700/60 text-[12.5px] font-semibold text-elerp-600 hover:bg-elerp-50 dark:hover:bg-elerp-900/30">
                Pagó justo ({fmtCurrency(aPedir, monedaPedir)})
              </button>
              {/* Vuelto en vivo: lo que hay que devolverle, o lo que falta. */}
              {Number(recibido) > 0 ? (
                vuelto ? (
                  <div className="text-[15px] font-semibold num text-emerald-600 dark:text-emerald-400">
                    Vuelto: {fmtCurrency(vuelto.monto, vuelto.moneda)}
                  </div>
                ) : falta > 0.005 ? (
                  <div className="text-[15px] font-semibold num text-amber-600 dark:text-amber-400">
                    Falta: {fmtCurrency(falta, 'VES')}
                  </div>
                ) : (
                  <div className="text-[13px] font-medium text-emerald-600 dark:text-emerald-400">Cobro exacto</div>
                )
              ) : null}
            </div>
            {meta.divisa && Number(recibido) > 0 && tasaMetodo > 0 ? (
              <div className="mt-2.5 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-700/60 px-3 py-2 text-[12.5px] text-amber-800 dark:text-amber-300">
                Con {fmtNum(Number(recibido), 2)} {monedaLabel(meta.moneda)} a {fmtNum(tasaMetodo, 2)} Bs entran {fmtCurrency(Number(recibido) * tasaMetodo, 'VES')}.
                El IGTF del 3% ({fmtCurrency(totales.igtf, 'VES')}) ya está sumado al total.
              </div>
            ) : null}
          </div>
        ) : null}

        {/* A crédito: abono opcional y plazo. El saldo va a Tesorería. */}
        {meta?.credito ? (
          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-3.5 space-y-3">
            <div className="text-[12.5px] text-slate-600 dark:text-slate-300">
              La factura se emite igual y el saldo queda <strong>por cobrar</strong> en Tesorería, con su fecha de
              vencimiento. Al consumidor final no se le fía: hace falta un cliente identificado.
            </div>
            <div className="grid grid-cols-2 gap-2.5">
              <div>
                <label className="block text-[12px] font-medium text-slate-500 mb-1">Abono de hoy (Bs)</label>
                <input type="number" min="0" step="0.01" value={recibido} placeholder="0,00"
                  onChange={(e) => setRecibido(e.target.value)}
                  className="w-full h-10 px-3 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num text-right ring-focus" />
                <div className="text-[11px] text-slate-400 mt-1">Opcional: puede quedar todo a crédito.</div>
              </div>
              <div>
                <label className="block text-[12px] font-medium text-slate-500 mb-1">Plazo</label>
                <Select value={plazo} onChange={(e) => setPlazo(Number(e.target.value))}>
                  {PLAZOS.map((d) => <option key={d} value={d}>{d} días</option>)}
                </Select>
                <div className="text-[11px] text-slate-400 mt-1 num">
                  Queda por cobrar: {fmtCurrency(Math.max(0, totales.total - (Number(recibido) || 0)), 'VES')}
                </div>
              </div>
            </div>
          </div>
        ) : null}

        {meta?.mixto ? (
          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-3.5">
            <div className="text-[12.5px] text-slate-500 mb-2.5">
              Reparte el cobro entre tus cuentas — el IGTF solo aplica sobre lo pagado en divisas.
            </div>
            {mix.map((l, i) => {
              const mm = METODOS.find((x) => x.id === l.metodo)
              return (
                <div key={i} className="grid grid-cols-[1.3fr_.9fr_auto] gap-2 items-end mb-2">
                  <div>
                    <label className="block text-[11px] text-slate-400 mb-1">Método</label>
                    <Select className="!h-9 text-[12.5px]" value={l.metodo}
                      onChange={(e) => setMix((s) => s.map((x, j) => (j === i ? { ...x, metodo: e.target.value, cuentaId: '' } : x)))}>
                      {metodosReales.map((m) => <option key={m.id} value={m.id}>{m.t}</option>)}
                    </Select>
                    {mm?.cuenta ? (
                      <Select className="!h-9 text-[12.5px] mt-1.5" value={l.cuentaId}
                        onChange={(e) => setMix((s) => s.map((x, j) => (j === i ? { ...x, cuentaId: e.target.value } : x)))}>
                        <option value="">{cuentasDe(l.metodo).length ? 'Elige cuenta…' : 'Sin cuentas en esa moneda'}</option>
                        {cuentasDe(l.metodo).map((c) => <option key={c.id} value={c.id}>{c.titular} · {c.datos}</option>)}
                      </Select>
                    ) : null}
                  </div>
                  <div>
                    <label className="block text-[11px] text-slate-400 mb-1">Monto ({monedaLabel(mm?.moneda || 'VES')})</label>
                    <input type="number" min="0" step="0.01" inputMode="decimal" value={l.monto} placeholder="0,00"
                      onChange={(e) => setMix((s) => s.map((x, j) => (j === i ? { ...x, monto: e.target.value } : x)))}
                      className="w-full h-9 px-2.5 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-[13px] num text-right ring-focus" />
                  </div>
                  {mix.length > 1 ? (
                    <button onClick={() => setMix((s) => s.filter((_, j) => j !== i))}
                      className="h-9 w-9 inline-flex items-center justify-center rounded-lg text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20">
                      <Icon.Trash size={15} />
                    </button>
                  ) : <span />}
                </div>
              )
            })}
            {/* Solo se ofrece añadir otro método si TODAVÍA falta por cubrir; al
                agregarlo, la línea nueva ya trae sugerido el restante. */}
            {faltaPorCobrar ? (
              <button onClick={agregarPago}
                className="mt-1 inline-flex items-center gap-1.5 rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-3 h-8 text-[12px] font-semibold text-elerp-600">
                <Icon.Plus size={13} /> Añadir método
              </button>
            ) : null}
            {totales.divisasEntregadasBs > 0 ? (
              <div className="mt-2.5 text-[11.5px] text-slate-400 text-right num">
                Equivale a {fmtCurrency(totales.divisasEntregadasBs, 'VES')} en divisas
              </div>
            ) : null}
            {cobrado > 0 ? (
              <div className="mt-2 text-right">
                <div className="text-[13px] font-medium">Cubierto: <span className="num">{fmtCurrency(cobrado, 'VES')}</span> de <span className="num">{fmtCurrency(totales.total, 'VES')}</span></div>
                {falta > 0.005 ? (
                  <div className="text-[13.5px] font-semibold num text-amber-600 dark:text-amber-400">Falta: {fmtCurrency(falta, 'VES')}</div>
                ) : vuelto ? (
                  <div className="text-[13.5px] font-semibold num text-emerald-600 dark:text-emerald-400">Vuelto: {fmtCurrency(vuelto.monto, vuelto.moneda)}</div>
                ) : (
                  <div className="text-[13px] font-medium text-emerald-600 dark:text-emerald-400">Cobro exacto</div>
                )}
              </div>
            ) : null}
          </div>
        ) : null}

        {/* Métodos con cuenta de cobro y referencia (pago móvil, transferencia, Zelle, punto de venta) */}
        {meta && !meta.efectivo && !meta.mixto ? (
          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-3.5 space-y-2.5">
            <div className="text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2 items-start">
              <Icon.CircleAlert size={15} className="mt-0.5 shrink-0 text-slate-400" />
              <span>{AYUDA[meta.tipo]}</span>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <div>
                <label className="block text-[12px] font-medium text-slate-500 mb-1">Monto ({monedaLabel(meta.moneda)})</label>
                <div className="flex gap-2">
                  <input type="number" min="0" step="0.01" inputMode="decimal" value={recibido} placeholder="0,00" autoFocus
                    onChange={(e) => setRecibido(e.target.value)}
                    className="flex-1 h-12 px-3.5 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-[16px] num text-right ring-focus" />
                  <button type="button" onClick={() => setRecibido(String(Math.ceil(aPedir * 100) / 100))}
                    className="h-12 px-4 rounded-xl border border-slate-300 dark:border-slate-600 text-[13px] font-semibold text-elerp-600 whitespace-nowrap">
                    Total
                  </button>
                </div>
              </div>
              <div>
                <label className="block text-[12px] font-medium text-slate-500 mb-1">Cuenta que recibe</label>
                <Select value={cuentaId} onChange={(e) => setCuentaId(e.target.value)}>
                  <option value="">{cuentasDe(metodo).length ? 'Elige cuenta…' : 'Sin cuentas en esa moneda'}</option>
                  {cuentasDe(metodo).map((c) => <option key={c.id} value={c.id}>{c.titular} · {c.datos}</option>)}
                </Select>
              </div>
              {meta.referencia ? (
                <div className="sm:col-span-2">
                  <label className="block text-[12px] font-medium text-slate-500 mb-1">Referencia <span className="text-slate-400 font-normal">· últimos dígitos</span></label>
                  <input value={referencia} onChange={(e) => setReferencia(e.target.value)} placeholder="123456"
                    className="w-full h-11 px-3.5 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-[15px] mono ring-focus" />
                </div>
              ) : null}
            </div>
          </div>
        ) : null}

        {/* El IGTF no lleva aviso aparte: aparece en el desglose de abajo, junto
            al IVA (redundante repetirlo en un banner). */}

        {/* Desglose del documento — total, subtotal e IGTF, debajo del método. */}
        <div className="rounded-xl border border-slate-200 dark:border-slate-700 p-3.5 text-[13px] space-y-1.5">
          <div className="flex justify-between text-slate-500"><span>Base imponible</span><span className="num">{fmtCurrency(totales.baseImponible, 'VES')}</span></div>
          {totales.baseExenta > 0 ? (
            <div className="flex justify-between text-slate-500"><span>Exento de IVA</span><span className="num">{fmtCurrency(totales.baseExenta, 'VES')}</span></div>
          ) : null}
          <div className="flex justify-between text-slate-500"><span>IVA {IVA_TASA * 100}%</span><span className="num">{fmtCurrency(totales.iva, 'VES')}</span></div>
          {totales.igtf > 0 ? (
            <div className="flex justify-between text-amber-700 dark:text-amber-400"><span>IGTF {IGTF_TASA * 100}%</span><span className="num">{fmtCurrency(totales.igtf, 'VES')}</span></div>
          ) : null}
          <div className="flex justify-between border-t border-slate-100 dark:border-slate-800 pt-1.5 text-[15px] font-semibold text-slate-900 dark:text-slate-100">
            <span>Total</span><span className="num">{fmtCurrency(totales.total, 'VES')}</span>
          </div>
          {tasa.hay ? (
            <div className="pt-1.5 border-t border-slate-100 dark:border-slate-800 flex justify-between text-[11.5px] text-slate-400">
              <span>Tasa aplicada</span>
              <span className="num">Bs {fmtNum(tasa.valor, 2)} / US$ · {tasa.fuenteLabel} · {fechaCortaVE(tasa.fechaValor, tasa.esDeHoy)}</span>
            </div>
          ) : null}
        </div>

        {error && metodo ? (
          <div className="flex items-start gap-1.5 text-[12.5px] text-amber-700 dark:text-amber-400">
            <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{error}</span>
          </div>
        ) : null}
        </div>{/* fin columna izquierda */}

        {/* Columna derecha — el vuelto para el cliente, repartible en varias partes.
            Aparece solo cuando el cliente pagó de más. Por defecto una sola parte
            que cubre todo el excedente; se puede repartir en varias monedas/medios
            (ej. US$20 en efectivo + el resto en Bs por pago móvil). Sin vuelto, un
            estado vacío. */}
        <div className="space-y-4">
        {hayVuelto ? (
          <div className="rounded-xl bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-700/60 p-3.5 space-y-3">
            {/* Cabecera: total del vuelto (en Bs) + «cubierto / falta» si hay reparto. */}
            <div className="flex items-center justify-between">
              <div className="text-[13px] font-semibold text-emerald-800 dark:text-emerald-300">Vuelto para el cliente</div>
              <div className="text-[17px] font-semibold num text-emerald-700 dark:text-emerald-300">
                {fmtCurrency(excedente, 'VES')}
              </div>
            </div>
            {!simpleVuelto ? (
              <div className={`text-[12px] num text-right ${vueltoCuadra ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-600 dark:text-amber-400'}`}>
                Cubierto {fmtCurrency(cubiertoBs, 'VES')} de {fmtCurrency(excedente, 'VES')}
                {faltaVuelto > tolVuelto ? ` · falta ${fmtCurrency(faltaVuelto, 'VES')}`
                  : faltaVuelto < -tolVuelto ? ` · sobran ${fmtCurrency(-faltaVuelto, 'VES')}`
                  : ' · cuadra'}
              </div>
            ) : null}

            {/* Partes del vuelto */}
            <div className="space-y-3">
              {partesCalc.map((p, i) => (
                <div key={i} className="rounded-lg border border-emerald-200/80 dark:border-emerald-700/50 bg-white/70 dark:bg-slate-900/40 p-2.5 space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-[11.5px] font-semibold text-emerald-800/80 dark:text-emerald-300/80">
                      {simpleVuelto ? 'Vuelto' : `Parte ${i + 1}`}
                    </span>
                    {!simpleVuelto ? (
                      <button type="button" onClick={() => quitarParte(i)}
                        className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20">
                        <Icon.Trash size={14} />
                      </button>
                    ) : null}
                  </div>

                  {/* Moneda: Bs siempre + divisas activas con tasa. */}
                  <div className="flex flex-wrap gap-1.5">
                    {monedasVuelto.map((c) => {
                      const activo = p.moneda === c
                      return (
                        <button key={c} type="button" onClick={() => seleccionarMoneda(i, c)}
                          className={`px-3 h-9 rounded-lg text-[12.5px] font-semibold border transition-colors
                            ${activo ? 'bg-white dark:bg-slate-800 border-emerald-400 text-emerald-700 dark:text-emerald-300 shadow-sm'
                              : 'bg-transparent border-emerald-200 dark:border-emerald-700/60 text-emerald-800/70 dark:text-emerald-300/70'}`}>
                          {monedaLabel(c)}
                        </button>
                      )
                    })}
                  </div>

                  {/* Monto: en el caso simple, derivado (todo el vuelto, solo lectura);
                      repartido, lo teclea el cajero en la moneda de la parte. */}
                  <div className="flex items-center gap-2">
                    <label className="text-[11.5px] text-emerald-800/70 dark:text-emerald-300/70 w-14 shrink-0">Monto</label>
                    {simpleVuelto ? (
                      <div className="flex-1 h-10 px-3 rounded-lg border border-emerald-200 dark:border-emerald-700/50 bg-emerald-50/60 dark:bg-slate-900/40 text-[14px] num text-right flex items-center justify-end text-emerald-800 dark:text-emerald-200">
                        {p.monto === null ? 'sin tasa' : `${fmtNum(p.monto, 2)} ${monedaLabel(p.moneda)}`}
                      </div>
                    ) : (
                      <input type="number" min="0" step="0.01" inputMode="decimal" value={vueltoPartes[i]?.monto ?? ''}
                        onChange={(e) => cambiarMontoParte(i, e.target.value)} placeholder="0,00"
                        className="flex-1 h-10 px-3 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-[14px] num text-right ring-focus" />
                    )}
                  </div>

                  {/* Medio: efectivo (de la gaveta) o pago móvil al cliente. En
                      divisa (US$, €…) solo efectivo — el pago móvil solo existe en Bs. */}
                  <div className="flex items-center gap-1.5 flex-wrap">
                    {(p.moneda === 'VES'
                      ? [{ v: 'efectivo', t: 'Efectivo', icon: 'Banknote' }, { v: 'pago_movil', t: 'Pago móvil', icon: 'Smartphone' }]
                      : [{ v: 'efectivo', t: 'Efectivo', icon: 'Banknote' }]
                    ).map((o) => {
                      const IconC = Icon[o.icon] || Icon.Wallet
                      const activo = p.metodo === o.v
                      return (
                        <button key={o.v} type="button" onClick={() => editarParte(i, { metodo: o.v })}
                          className={`inline-flex items-center gap-1.5 px-3 h-9 rounded-lg text-[12.5px] font-semibold border transition-colors
                            ${activo ? 'bg-white dark:bg-slate-800 border-emerald-400 text-emerald-700 dark:text-emerald-300 shadow-sm'
                              : 'bg-transparent border-emerald-200 dark:border-emerald-700/60 text-emerald-800/70 dark:text-emerald-300/70'}`}>
                          <IconC size={15} /> {o.t}
                        </button>
                      )
                    })}
                    {p.moneda !== 'VES' ? (
                      <span className="text-[11px] text-emerald-800/60 dark:text-emerald-300/60">En divisas, solo efectivo</span>
                    ) : null}
                  </div>

                  {/* Datos del pago móvil del cliente + paso de EJECUCIÓN (STUB), por
                      parte: bloquea «Emitir» hasta quedar en «listo». */}
                  {p.metodo === 'pago_movil' ? (
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                      <div className="sm:col-span-2">
                        <Select value={p.banco || ''} onChange={(e) => editarParte(i, { banco: e.target.value })}>
                          <option value="">Elige el banco…</option>
                          {BANCOS_VE.map((b) => <option key={b.codigo} value={b.codigo}>{b.codigo} · {b.nombre}</option>)}
                        </Select>
                      </div>
                      <input value={p.cedula || ''} onChange={(e) => editarParte(i, { cedula: e.target.value })} placeholder="V-12345678"
                        className={`h-10 px-3 rounded-lg border bg-white dark:bg-slate-900 text-[14px] mono ring-focus ${!String(p.cedula || '').trim() ? 'border-amber-400' : 'border-slate-300 dark:border-slate-700'}`} />
                      <input value={p.telefono || ''} onChange={(e) => editarParte(i, { telefono: e.target.value })} placeholder="0414-1234567" inputMode="tel"
                        className={`h-10 px-3 rounded-lg border bg-white dark:bg-slate-900 text-[14px] mono ring-focus ${!String(p.telefono || '').trim() ? 'border-amber-400' : 'border-slate-300 dark:border-slate-700'}`} />

                      {pmDatosCompletos(p) ? (
                        <div className="sm:col-span-2 rounded-lg border border-emerald-300/70 dark:border-emerald-700/60 bg-white/70 dark:bg-slate-900/50 p-2.5 flex items-center justify-between gap-3">
                          <div className="min-w-0 text-[11.5px] text-emerald-800/70 dark:text-emerald-300/70">
                            Envío de {fmtCurrency(p.montoBs, 'VES')} al pago móvil del cliente.
                          </div>
                          {p.estado === 'listo' ? (
                            <span className="inline-flex items-center gap-1.5 text-[12.5px] font-semibold text-emerald-700 dark:text-emerald-300 shrink-0">
                              <Icon.CircleCheck size={16} /> Enviado
                            </span>
                          ) : (
                            <Button variant="dinero" size="sm" loading={p.estado === 'procesando'}
                              onClick={() => procesarParte(i)} icon={p.estado === 'error' ? <Icon.Refresh size={14} /> : <Icon.Smartphone size={14} />}>
                              {p.estado === 'procesando' ? 'Procesando…' : p.estado === 'error' ? 'Reintentar' : 'Procesar'}
                            </Button>
                          )}
                        </div>
                      ) : null}
                      {p.estado === 'error' ? (
                        <div className="sm:col-span-2 flex items-start gap-1.5 text-[11.5px] text-red-600 dark:text-red-400">
                          <Icon.CircleAlert size={13} className="mt-0.5 shrink-0" /> No se pudo enviar. Toca «Reintentar».
                        </div>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>

            <button type="button" onClick={agregarParte}
              className="inline-flex items-center gap-1.5 rounded-lg border border-emerald-300 dark:border-emerald-600/60 bg-white/70 dark:bg-slate-900/40 px-3 h-9 text-[12.5px] font-semibold text-emerald-700 dark:text-emerald-300">
              <Icon.Plus size={14} /> Repartir en otra moneda / medio
            </button>

            <div className="text-[11px] text-emerald-800/60 dark:text-emerald-300/60">
              El vuelto por pago móvil deja registrada la instrucción de transferencia; la conexión real con el banco aún no está integrada.
            </div>
          </div>
        ) : (
          <div className="rounded-xl border border-dashed border-slate-200 dark:border-slate-700 p-6 text-center text-slate-400 flex flex-col items-center justify-center gap-2 min-h-[160px]">
            <Icon.Banknote size={26} className="opacity-40" />
            <div className="text-[13px] font-medium">Sin vuelto</div>
            <div className="text-[12px] max-w-[220px] leading-snug">Si el cliente paga de más, aquí eliges en qué moneda y cómo devolverle el excedente — puedes repartirlo en varias partes.</div>
          </div>
        )}
        </div>{/* fin columna derecha */}
      </div>{/* fin grid 2 columnas */}
    </Modal>
  )
}

/* Estado de éxito tras emitir, como el prototipo: número fiscal grande, total
 * cobrado, el VUELTO a entregar, acciones de entrega y «Nueva venta».
 *
 * Al montar (factura recién emitida) se dispara automáticamente la IMPRESIÓN
 * FISCAL: el documento se envía a la impresora fiscal activa de la sede. La
 * impresión es del lado del cajero y NO re-emite la factura (ya está emitida con
 * numeración atómica); reimprimir solo reenvía al hardware. Ver ImpresionFiscal. */
export function VentaEmitida({ doc, onNueva }) {
  const toast = useToast()
  const { db } = useData()
  // Impresora fiscal ACTIVA de la sede activa (destino de impresión). Si no hay,
  // el modal muestra «No hay impresora fiscal configurada».
  const dispositivo = impresoraActivaDeSede(db.DISPOSITIVOS, db.SEDE_ACTIVA?.id)
  // Se abre automáticamente al emitir; «Imprimir» lo reabre para reimprimir.
  const [imprimiendo, setImprimiendo] = useState(true)
  // Previsualización del comprobante como PDF (imprimir / correo / enlace). Es el
  // MISMO modal reutilizable que Ventas, Facturación y Compras.
  const [pdf, setPdf] = useState(false)

  const acciones = [
    { icon: <Icon.Printer size={15} />, t: 'Imprimir', on: () => setImprimiendo(true) },
    { icon: <Icon.Download size={15} />, t: 'PDF', on: () => setPdf(true) },
    { icon: <Icon.Message size={15} />, t: 'WhatsApp', on: () => toast({ title: 'WhatsApp', body: 'Requiere la integración de WhatsApp Business, todavía sin conectar.', kind: 'warn' }) },
    { icon: <Icon.Mail size={15} />, t: 'Correo', on: () => setPdf(true) },
  ]
  return (
    <div className="max-w-md mx-auto py-8 text-center">
      <div className="h-16 w-16 rounded-full bg-emerald-50 dark:bg-emerald-900/30 text-emerald-500 inline-flex items-center justify-center mb-3">
        <Icon.CircleCheck size={32} />
      </div>
      <h2 className="text-xl font-semibold tracking-tight font-display">¡Factura emitida!</h2>
      <div className="mt-3 text-[11.5px] font-semibold tracking-wider text-slate-400">NÚMERO FISCAL ASIGNADO</div>
      <div className="mono text-[26px] font-semibold tracking-wide">{doc.numeroCompleto}</div>
      <div className="text-[13px] text-slate-500 mt-1">
        Total cobrado: <strong className="num">{fmtCurrency(doc.total, 'VES')}</strong>
      </div>

      {doc.contingencia ? (
        <div className="mt-3 inline-flex items-center gap-1.5 rounded-full bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-700/60 px-3.5 py-1.5 text-[12.5px] font-semibold text-amber-800 dark:text-amber-300">
          <Icon.WifiOff size={13} /> Serie de contingencia · se sincroniza al reconectar
        </div>
      ) : null}

      {/* El vuelto: lo que el cajero tiene que devolver. Si es MIXTO (varias
          partes), se muestra el total en Bs y el desglose por parte. */}
      {doc.vuelto > 0.004 ? (
        <div className="mt-4 rounded-xl bg-emerald-50 dark:bg-emerald-900/25 border border-emerald-200 dark:border-emerald-700/50 px-4 py-3">
          <div className="text-[12px] text-emerald-800/80 dark:text-emerald-300/80">Entrega el vuelto</div>
          <div className="text-[22px] font-semibold num text-emerald-700 dark:text-emerald-300">
            {fmtCurrency(doc.vuelto, doc.vueltoMoneda || 'VES')}
          </div>
          {Array.isArray(doc.vueltoPartes) && doc.vueltoPartes.length > 1 ? (
            <div className="mt-1.5 flex flex-wrap justify-center gap-1.5">
              {doc.vueltoPartes.map((p, i) => (
                <span key={i} className="inline-flex items-center gap-1 rounded-full bg-white/70 dark:bg-slate-900/40 border border-emerald-200 dark:border-emerald-700/50 px-2.5 py-1 text-[11.5px] num text-emerald-800 dark:text-emerald-300">
                  {fmtCurrency(p.monto, p.moneda || 'VES')} · {p.metodo === 'pago_movil' ? 'pago móvil' : 'efectivo'}
                </span>
              ))}
            </div>
          ) : null}
          <div className="text-[11.5px] text-emerald-800/70 dark:text-emerald-300/70 num mt-1">
            Recibiste {fmtCurrency(doc.cobrado, 'VES')} · factura {fmtCurrency(doc.total, 'VES')}
          </div>
        </div>
      ) : null}

      <div className="mt-5 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 text-left space-y-1.5 text-[13px]">
        <div className="flex justify-between"><span className="text-slate-500">Base imponible</span><span className="num private-mask">{fmtCurrency(doc.baseImponible, 'VES')}</span></div>
        {doc.baseExenta > 0 ? <div className="flex justify-between"><span className="text-slate-500">Exento de IVA</span><span className="num private-mask">{fmtCurrency(doc.baseExenta, 'VES')}</span></div> : null}
        <div className="flex justify-between"><span className="text-slate-500">IVA</span><span className="num private-mask">{fmtCurrency(doc.iva, 'VES')}</span></div>
        {doc.igtf ? <div className="flex justify-between"><span className="text-slate-500">IGTF</span><span className="num private-mask">{fmtCurrency(doc.igtf, 'VES')}</span></div> : null}
        <div className="flex justify-between border-t border-slate-100 dark:border-slate-800 pt-1.5 font-semibold"><span>Total</span><span className="num private-mask">{fmtCurrency(doc.total, 'VES')}</span></div>
      </div>

      <div className="grid grid-cols-2 gap-2 mt-4">
        {acciones.map((a) => (
          <Button key={a.t} variant="secondary" icon={a.icon} onClick={a.on} className="justify-center">{a.t}</Button>
        ))}
      </div>
      <Button className="mt-3 w-full justify-center" size="lg" icon={<Icon.Plus size={17} />} onClick={onNueva}>Nueva venta</Button>

      {/* Impresión fiscal: se dispara al emitir y se puede reabrir con «Imprimir».
          Cerrar vuelve a esta pantalla (con el número y el vuelto a la vista) y
          «Nueva venta» deja la caja lista para el siguiente cobro. */}
      <ImpresionFiscalModal open={imprimiendo} doc={doc} dispositivo={dispositivo}
        onCerrar={() => setImprimiendo(false)} />

      {pdf ? (
        <ComprobanteModal open onClose={() => setPdf(false)} empresa={db.EMPRESA}
          data={comprobanteDeFactura(doc, db.EMPRESA, (db.CLIENTES || []).find((c) => c.id === doc.clienteId))} />
      ) : null}
    </div>
  )
}
