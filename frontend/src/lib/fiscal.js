// Metadatos y cálculo fiscal para el POS y los documentos.
// OJO: el backend calcula los montos de forma autoritativa; esto es solo para el
// preview en vivo del POS. Debe reflejar exactamente las reglas del backend
// (ver internal/application/fiscal.go: EmitirFactura).

import { resolveRate } from './precio.js'

export const IVA_TASA = 0.16 // IVA 16% sobre la base imponible
export const IGTF_TASA = 0.03 // IGTF 3% sobre la porción de la factura pagada en divisas

// Métodos de pago del POS. `moneda` es la moneda por defecto; `cuenta` indica si
// el método normalmente requiere una cuenta de cobro (efectivo no).
export const METODOS_PAGO = [
  { value: 'efectivo_bs', label: 'Efectivo (Bs)', moneda: 'VES', cuenta: false, icon: 'Banknote' },
  { value: 'efectivo_usd', label: 'Efectivo ($)', moneda: 'USD', cuenta: false, icon: 'Banknote' },
  { value: 'pago_movil', label: 'Pago Móvil', moneda: 'VES', cuenta: true, icon: 'Wallet' },
  { value: 'zelle', label: 'Zelle', moneda: 'USD', cuenta: true, icon: 'Globe' },
  { value: 'tarjeta', label: 'Tarjeta / Punto de venta', moneda: 'VES', cuenta: true, icon: 'Wallet' },
  { value: 'transferencia', label: 'Transferencia', moneda: 'VES', cuenta: true, icon: 'Bank' },
]

export const metodoLabel = (v) => METODOS_PAGO.find((m) => m.value === v)?.label || v

// Tipos de cuenta de cobro (Tesorería).
export const TIPOS_CUENTA = [
  { value: 'pago_movil', label: 'Pago Móvil' },
  { value: 'zelle', label: 'Zelle' },
  { value: 'banco', label: 'Cuenta bancaria' },
  { value: 'punto_venta', label: 'Punto de venta' },
]

/* Preview de totales, con las mismas reglas que el servidor.
 *
 * `lineas`: [{cantidad, precioUnitario, exento}] con los precios ya en Bs.
 * `pagos`:  [{metodo, monto, moneda}] en la moneda de cada pago.
 * `tasa`:   la tasa de cambio, en dos formas (retrocompat):
 *            · una FUNCIÓN resolutora  tasaDe(moneda) => Bs/unidad   (multimoneda)
 *            · un NÚMERO               la tasa del dólar (USD legacy)
 *           El servidor la fija; el POS no la teclea (R9). Cada pago se convierte
 *           con la tasa de SU divisa: un pago en € usa la tasa del €, no la del $.
 *
 * Dos reglas copiadas del backend que no se pueden desviar:
 *   · el IVA sale SOLO de la base imponible — la cesta básica está exenta;
 *   · el IGTF grava la porción de LA FACTURA pagada en divisas, topada con
 *     (base + IVA): el vuelto de un billete grande no es un pago en divisas.
 */
export function calcularTotales(lineas, pagos, tasa) {
  let baseImponible = 0
  let baseExenta = 0
  for (const l of lineas || []) {
    const monto = (Number(l.cantidad) || 0) * (Number(l.precioUnitario) || 0)
    if (l.exento) baseExenta += monto
    else baseImponible += monto
  }
  const subtotal = baseImponible + baseExenta
  const iva = baseImponible * IVA_TASA
  const docBase = subtotal + iva

  // Cada divisa se convierte con SU propia tasa (Bs/unidad), no con una única.
  const divisasEntregadasBs = (pagos || [])
    .filter((p) => p.moneda && p.moneda !== 'VES')
    .reduce((a, p) => a + (Number(p.monto) || 0) * resolveRate(tasa, p.moneda), 0)
  const baseDivisasBs = Math.min(divisasEntregadasBs, docBase)
  const igtf = baseDivisasBs * IGTF_TASA
  const total = docBase + igtf
  return { subtotal, baseImponible, baseExenta, iva, igtf, total, baseDivisasBs, divisasEntregadasBs }
}

// Total cobrado convertido a Bs (para comparar contra el total del documento).
// Cada pago se convierte con la tasa de su moneda (resolutor o número USD).
export function totalPagosEnBs(pagos, tasa) {
  return (pagos || []).reduce((a, p) => {
    const monto = Number(p.monto) || 0
    return a + (p.moneda && p.moneda !== 'VES' ? monto * resolveRate(tasa, p.moneda) : monto)
  }, 0)
}

/* Vuelto: el excedente que hay que devolverle al cliente, con la misma regla del
 * servidor — el excedente de un pago en efectivo en divisas se devuelve en esa
 * divisa (cualquiera, no solo US$), porque es el billete que el cliente puso.
 * Devuelve {monto, moneda} o null si no hay vuelto.
 *
 * El POS ahora deja ELEGIR la moneda del vuelto (ver excedenteBs +
 * vueltoDefaultMoneda); esta función conserva la derivación automática para
 * quien no declara nada, idéntica al servidor.
 */
export function calcularVuelto(pagos, total, tasa) {
  const cobrado = totalPagosEnBs(pagos, tasa)
  const excedente = cobrado - total
  if (excedente <= 0.004) return null
  const moneda = vueltoDefaultMoneda(pagos)
  if (moneda !== 'VES') {
    const t = resolveRate(tasa, moneda)
    if (t > 0) return { monto: excedente / t, moneda }
  }
  return { monto: excedente, moneda: 'VES' }
}

// Excedente (en Bs) pagado de más sobre el total del documento. 0 si no sobra.
export function excedenteBs(pagos, total, tasa) {
  return Math.max(0, totalPagosEnBs(pagos, tasa) - total)
}

/* Moneda por defecto del vuelto: la del ÚLTIMO efectivo entregado en divisa (así
 * la caja devuelve billetes de la misma moneda que recibió); si no hubo efectivo
 * en divisa, bolívares. Coincide con el default del servidor y con lo de hoy
 * (efectivo en US$ ⇒ vuelto en US$). */
export function vueltoDefaultMoneda(pagos) {
  let moneda = 'VES'
  for (const p of pagos || []) {
    const esEfectivoDivisa = String(p.metodo || '').startsWith('efectivo_') && p.moneda && p.moneda !== 'VES' && (Number(p.monto) || 0) > 0
    if (esEfectivoDivisa) moneda = p.moneda
  }
  return moneda
}
