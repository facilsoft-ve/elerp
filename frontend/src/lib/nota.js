import { fmtCurrency } from './format.js'
import { IVA_TASA } from './fiscal.js'

/* ARITMÉTICA DEL AJUSTE DE UNA NOTA (crédito o débito).
 *
 * Nota del usuario: «las notas de crédito y débito deben poder crearse
 * especificando un % sobre el monto total o el monto del producto seleccionado».
 *
 * POR QUÉ IMPORTA: un descuento o un recargo se PACTA en porcentaje —«10 % por
 * pronto pago», «5 % de mora»—, no en bolívares. Obligar a escribir el monto
 * hace que alguien saque la regla de tres en una calculadora y teclee el
 * resultado: ahí se cuela el céntimo que después no cuadra contra la factura, y
 * peor, nadie puede reconstruir después de qué porcentaje salió ese número.
 *
 * Quien calcula de verdad es el SERVIDOR (`repartirBase` en el backend): esto es
 * la PREVISUALIZACIÓN. La cuenta se repite para que el número se vea antes de
 * emitir un documento que ya no se puede editar, pero el que queda asentado es
 * el del backend. Por eso vive acá y no en el componente: es lógica pura, y se
 * prueba.
 *
 * EL PORCENTAJE VA SOBRE LA BASE, no sobre el total con IVA. Da lo mismo en
 * plata (el IVA es proporcional: 10 % de la base baja el total un 10 %), pero
 * aplicarlo sobre el total y volver a sumar IVA encima descontaría dos veces el
 * impuesto.
 */

// Estado inicial del ajuste. Por defecto: monto fijo sobre toda la factura, que
// es lo que hacía antes: quien no necesita el porcentaje no ve nada nuevo.
export const ajusteVacio = () => ({
  alcance: 'total', sku: '', modo: 'monto', monto: '', porcentaje: '', exento: false,
})

// tasaIVADe usa la alícuota HISTÓRICA del documento (la vigente al facturar),
// con respaldo en la del sistema para facturas anteriores a la configuración.
// Previsualizar con la tasa de hoy mostraría un IVA que el backend no va a
// emitir.
const tasaIVADe = (doc) => (Number(doc?.alicuotaIVA) > 0 ? Number(doc.alicuotaIVA) : IVA_TASA)

/* calcularAjuste reparte el ajuste entre base gravada y exenta, igual que
 * `repartirBase` en el backend. Devuelve también `error` con el motivo por el
 * que todavía no se puede emitir, para validar mientras se escribe y no al
 * enviar. */
export function calcularAjuste(doc, aj, { tope = false } = {}) {
  const lineas = doc?.lineas || []
  const pct = Number(aj.porcentaje)
  const monto = Number(aj.monto)
  const porcentual = aj.modo === 'porcentaje'

  if (porcentual && !(Number.isFinite(pct) && pct > 0)) {
    return { gravado: 0, exento: 0, base: 0, iva: 0, total: 0, error: 'Indica un porcentaje mayor que cero.' }
  }
  if (porcentual && pct > 100) {
    return { gravado: 0, exento: 0, base: 0, iva: 0, total: 0, error: 'El porcentaje no puede pasar de 100 %.' }
  }
  if (!porcentual && !(Number.isFinite(monto) && monto > 0)) {
    return { gravado: 0, exento: 0, base: 0, iva: 0, total: 0, error: 'Indica un monto mayor que cero.' }
  }

  let gravado = 0
  let exento = 0
  let topeBase = 0

  if (aj.alcance === 'producto') {
    const l = lineas.find((x) => x.sku === aj.sku)
    if (!l) {
      return { gravado: 0, exento: 0, base: 0, iva: 0, total: 0, error: 'Elige el producto sobre el que se aplica.' }
    }
    topeBase = Math.abs(Number(l.total) || 0)
    const v = porcentual ? (topeBase * pct) / 100 : monto
    if (l.exento) exento = v
    else gravado = v
  } else if (porcentual) {
    // Reparto PROPORCIONAL entre gravado y exento: cargarlo todo a una sola base
    // cambiaría el IVA de la nota y descuadraría el libro.
    gravado = ((Number(doc?.baseImponible) || 0) * pct) / 100
    exento = ((Number(doc?.baseExenta) || 0) * pct) / 100
  } else if (aj.exento) {
    exento = monto
  } else {
    gravado = monto
  }

  const base = gravado + exento
  const iva = gravado * tasaIVADe(doc)
  // El tope solo aplica a la nota de crédito: no se puede acreditar más de lo
  // que se cobró. Un cargo posterior (mora, diferencial) sí puede superarlo.
  const error = tope && topeBase > 0 && base > topeBase + 0.005
    ? `No puedes pasar de ${fmtCurrency(topeBase, 'VES')} en ese producto.`
    : ''
  return { gravado, exento, base, iva, total: base + iva, error }
}

/* cuerpoAjuste arma el cuerpo para la API: manda monto O porcentaje, nunca los
 * dos (el servidor rechaza la ambigüedad en vez de elegir por su cuenta). */
export function cuerpoAjuste(aj) {
  const base = aj.alcance === 'producto' ? { sku: aj.sku } : {}
  return aj.modo === 'porcentaje'
    ? { ...base, porcentaje: Number(aj.porcentaje) }
    : { ...base, monto: Number(aj.monto), exento: aj.alcance === 'total' ? !!aj.exento : undefined }
}
