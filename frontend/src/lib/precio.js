// Precio de producto y su equivalente en otras monedas (R10, multimoneda).
//
// Regla de diseño: el precio se GUARDA una sola vez, en la moneda en la que la
// empresa lo piensa (`producto.moneda`, o la principal de la empresa si el
// producto no la declara). El equivalente en cualquier otra moneda se CALCULA
// con la tasa del día — nunca se guarda, porque dos precios guardados se
// desincronizan en cuanto la tasa cambia.
//
// La base contable SIEMPRE es el bolívar (VES): todo importe se guarda y se
// factura en Bs. Las divisas (USD, EUR, …) son solo capa de presentación y de
// captura; para pasar de una divisa a Bs se usa su tasa (Bs por 1 unidad).
//
// Todas estas funciones devuelven `null` cuando hace falta una tasa y no hay
// ninguna. Ninguna inventa un 1: un importe convertido con una tasa falsa es
// peor que un espacio en blanco que dice qué falta.

// Metadatos de presentación por moneda. `label` es el rótulo del toggle; `simbolo`
// el prefijo que se antepone al número. VES es la base. La whitelist del backend
// es USD, EUR, COP, USDT.
export const MONEDAS_META = {
  VES: { label: 'Bs', simbolo: 'Bs', nombre: 'Bolívares' },
  USD: { label: 'US$', simbolo: '$', nombre: 'Dólares' },
  EUR: { label: '€', simbolo: '€', nombre: 'Euros' },
  COP: { label: 'COP', simbolo: 'COP', nombre: 'Pesos colombianos' },
  USDT: { label: 'USDT', simbolo: 'USDT', nombre: 'Tether' },
}
export const monedaLabel = (c) => MONEDAS_META[(c || '').toUpperCase()]?.label || (c || '').toUpperCase()
export const monedaSimbolo = (c) => MONEDAS_META[(c || '').toUpperCase()]?.simbolo || (c || '').toUpperCase()
export const monedaNombre = (c) => MONEDAS_META[(c || '').toUpperCase()]?.nombre || (c || '').toUpperCase()

// Solo el dólar puede alimentarse de la fuente oficial (BCV). Las demás divisas
// llevan tasa manual o de mercado, nunca «bcv».
export const permiteFuenteBcv = (c) => (c || '').toUpperCase() === 'USD'

// resolveRate obtiene la tasa (Bs por 1 unidad) de una moneda a partir del
// argumento `tasa`, que admite dos formas por retrocompatibilidad:
//   · una FUNCIÓN resolutora  tasaDe(moneda) => Bs/unidad   (multimoneda)
//   · un NÚMERO               la tasa de la única divisa extranjera (USD legacy)
// VES es siempre 1. Así, los llamadores que aún pasan `tasa.valor` (USD) siguen
// funcionando idénticos, y los que pasan el resolutor obtienen cada divisa.
export function resolveRate(tasa, moneda = 'USD') {
  const m = (moneda || 'USD').toUpperCase()
  if (m === 'VES') return 1
  if (typeof tasa === 'function') return Number(tasa(m)) || 0
  return Number(tasa) || 0
}

// monedaDe resuelve en qué moneda está expresado el precio de un producto.
export const monedaDe = (producto, monedaEmpresa = 'VES') =>
  (producto?.moneda || monedaEmpresa || 'VES').toUpperCase()

// itemDeLista busca el precio explícito de un SKU dentro de una lista de precio.
// Devuelve el ítem { sku, precio } o null si la lista no menciona ese producto
// (en cuyo caso rige el precio base del catálogo).
export function itemDeLista(lista, sku) {
  if (!lista || !Array.isArray(lista.items)) return null
  const s = String(sku || '').toLowerCase()
  return lista.items.find((it) => String(it.sku || '').toLowerCase() === s) || null
}

// precioListaEnBs devuelve el precio unitario de un producto en bolívares para
// una lista de precio dada. Si la lista trae un precio explícito para el SKU, lo
// usa (convertido desde la moneda de la lista); si no —o si no hay lista— cae al
// precio base del catálogo. Devuelve null cuando hace falta una tasa y no hay
// ninguna, igual que precioEnBs: no se inventa un 1.
export function precioListaEnBs(producto, lista, monedaEmpresa = 'VES', tasa = 0) {
  const it = itemDeLista(lista, producto?.sku)
  if (it) {
    const m = (lista?.moneda || monedaEmpresa || 'VES').toUpperCase()
    const valor = Number(it.precio) || 0
    if (m === 'VES') return valor
    const t = resolveRate(tasa, m)
    return t > 0 ? valor * t : null
  }
  return precioEnBs(producto, monedaEmpresa, tasa)
}

// precioEnBs devuelve el precio del producto en bolívares, que es la moneda en
// la que se emite la factura legal. Convierte desde la moneda propia del
// producto usando su tasa (no la del dólar): un producto en EUR usa la tasa EUR.
export function precioEnBs(producto, monedaEmpresa = 'VES', tasa = 0) {
  const valor = Number(producto?.precio) || 0
  const m = monedaDe(producto, monedaEmpresa)
  if (m === 'VES') return valor
  const t = resolveRate(tasa, m)
  return t > 0 ? valor * t : null
}

// montoEnMoneda pasa un importe en Bs a la moneda de presentación pedida. Bs es
// el pivote: para USD divide por la tasa USD; para EUR por la tasa EUR.
export function montoEnMoneda(bs, ccy = 'VES', tasa = 0) {
  const n = Number(bs) || 0
  const m = (ccy || 'VES').toUpperCase()
  if (m === 'VES') return n
  const t = resolveRate(tasa, m)
  return t > 0 ? n / t : null
}

// precioEnUsd devuelve el precio del producto en dólares (retrocompat).
export function precioEnUsd(producto, monedaEmpresa = 'VES', tasa = 0) {
  const bs = precioEnBs(producto, monedaEmpresa, tasa)
  if (bs === null) return null
  return montoEnMoneda(bs, 'USD', tasa)
}

// precioEnMoneda devuelve el precio expresado en la moneda de presentación que
// el usuario eligió en la barra superior (Bs, US$, €, …). Pasa primero a Bs
// (con la tasa de la moneda del producto) y de Bs a la moneda de presentación
// (con la tasa de esa moneda). Con un resolutor `tasa` esto funciona aunque la
// moneda del producto y la de presentación sean dos divisas distintas.
export function precioEnMoneda(producto, ccy = 'VES', monedaEmpresa = 'VES', tasa = 0) {
  const bs = precioEnBs(producto, monedaEmpresa, tasa)
  if (bs === null) return null
  return montoEnMoneda(bs, ccy, tasa)
}

// convertir pasa un importe entre dos monedas cualesquiera pivotando por Bs.
export function convertir(monto, desde, hacia, tasa = 0) {
  const n = Number(monto) || 0
  const d = (desde || 'VES').toUpperCase()
  const h = (hacia || 'VES').toUpperCase()
  if (d === h) return n
  // A Bs primero.
  let bs
  if (d === 'VES') bs = n
  else { const td = resolveRate(tasa, d); if (td <= 0) return null; bs = n * td }
  return montoEnMoneda(bs, h, tasa)
}

/* porCodigo resuelve un término tecleado o ESCANEADO a un producto del catálogo,
 * buscando en el código de barras propio, en el de cada presentación y en el SKU
 * (R11). Es lo que hace que la pistola lectora sirva: escribe el código en el
 * buscador y el ítem se agrega solo.
 *
 * Compara sin distinguir mayúsculas y sin espacios, porque algunas pistolas
 * agregan un espacio o un salto de línea al final.
 */
export function porCodigo(productos, termino) {
  const t = String(termino || '').trim().toLowerCase()
  if (!t) return null
  for (const p of productos || []) {
    if ((p.codigoBarras || '').toLowerCase() === t) return p
    if ((p.sku || '').toLowerCase() === t) return p
    for (const pr of p.presentaciones || []) {
      if ((pr.codigoBarras || '').toLowerCase() === t) return p
    }
  }
  return null
}
