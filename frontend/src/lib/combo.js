// Explosión de productos COMBO (paquetes).
//
// Un combo es un producto que agrupa a otros: tiene un precio propio (el del
// paquete) y, al venderse, NO viaja como una línea de "combo" a la emisión —el
// motor fiscal la rechaza— sino que se EXPLOTA en una línea por cada componente.
//
// El precio del paquete se PRORRATEA entre los componentes en proporción a su
// precio de lista/base: cada componente conserva su condición de IVA (exento o
// gravado) y su precio unitario baja (o sube) por un factor uniforme
//   factor = precioCombo / Σ(cantidadComp × precioBaseComp)
// de modo que  Σ(cantidad × precioUnitario) ≈ precio del combo, con el IVA
// correcto POR componente. Así el motor de totales / IVA / IGTF (fiscal.js y el
// servidor) queda intacto: recibe líneas normales con su precioUnitario y su
// bandera `exento`, sin saber nunca que salieron de un combo.

// round2: redondeo a 2 decimales, misma regla que round2 del backend, para no
// arrastrar colas de flotante al prorratear los precios de línea.
const round2 = (v) => Math.round((Number(v) || 0) * 100) / 100

// resolverProducto normaliza `productosPorSku` a una función sku→producto.
// Admite una función resolutora, un Map, o un objeto plano indexado por SKU.
function resolverProducto(productosPorSku) {
  if (typeof productosPorSku === 'function') return productosPorSku
  if (productosPorSku instanceof Map) return (sku) => productosPorSku.get(sku)
  return (sku) => (productosPorSku ? productosPorSku[sku] : undefined)
}

/* explotarCombo(combo, productosPorSku, cantidad, precioBs)
 *
 * · combo           — el producto combo ({ esCombo:true, componentes:[{sku,cantidad}], precio })
 * · productosPorSku — resolutor de productos por SKU (función, Map u objeto)
 * · cantidad        — cuántos combos se venden (multiplica la cantidad de cada componente)
 * · precioBs        — función producto→precio unitario EN Bs (la misma que usan POS y
 *                     Ventas: precioBsDe / precioListaEnBs). Puede devolver null si
 *                     falta una tasa: entonces no se puede explotar.
 *
 * Devuelve una línea por componente:
 *   { sku, nombre, cantidad, precioUnitario, exento, monedaOriginal:'VES', combo, comboNombre }
 * o `null` si falta algún componente, falta una tasa, o Σ de componentes ≤ 0
 * (el llamador avisa con un toast y no agrega nada).
 *
 * NOTA: si algún componente es "por peso", acá se trata como unidad (su precio por
 * kg entra como precio unitario y su `cantidad` como número de kg del combo). Un
 * combo se vende por unidad, así que este caso es marginal; se documenta por
 * honestidad. */
export function explotarCombo(combo, productosPorSku, cantidad, precioBs) {
  if (!combo || !Array.isArray(combo.componentes) || combo.componentes.length === 0) return null
  const get = resolverProducto(productosPorSku)
  const cant = Number(cantidad) || 0
  if (cant <= 0) return null

  // Precio del paquete en Bs. Sin tasa (null) no se puede prorratear.
  const comboBs = precioBs(combo)
  if (comboBs === null || comboBs === undefined) return null

  // Resuelve cada componente y acumula la suma de sus precios base.
  const items = []
  let sumaBs = 0
  for (const c of combo.componentes || []) {
    const prod = get(c.sku)
    if (!prod) return null // componente ausente del catálogo
    const baseBs = precioBs(prod)
    if (baseBs === null || baseBs === undefined) return null // falta tasa de un componente
    const cc = Number(c.cantidad) || 0
    if (cc <= 0) continue
    sumaBs += cc * baseBs
    items.push({ prod, cc, baseBs })
  }
  if (!(sumaBs > 0) || items.length === 0) return null

  // Prorrateo: el precio del paquete se reparte proporcional al peso de cada
  // componente en la suma. factor < 1 cuando el combo trae descuento.
  const factor = comboBs / sumaBs

  return items.map(({ prod, cc, baseBs }) => ({
    sku: prod.sku,
    nombre: prod.nombre,
    cantidad: cc * cant,
    precioUnitario: round2(baseBs * factor),
    exento: !!prod.exentoIva,
    monedaOriginal: 'VES',
    combo: combo.sku,
    comboNombre: combo.nombre,
  }))
}
