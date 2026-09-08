/* Qué se puede VENDER del catálogo.
 *
 * Un INSUMO (materia prima) se compra, se stockea y se consume por la receta de un
 * plato, pero no se vende: un restaurante vende platos, bebidas y productos de reventa,
 * no el kilo de pasta cruda. Por eso queda fuera del POS, del modo caja, de la comandera,
 * de las cotizaciones y de las listas de precio — pero SÍ aparece en el catálogo, en
 * compras, en movimientos y en transferencias, que es donde se lo administra.
 *
 * Vive en un solo lugar a propósito: cada pantalla de venta que se agregue debe pasar por
 * acá, en vez de repetir el filtro y olvidarse en una. */

/** vendibles quita los insumos y, opcionalmente, los productos inactivos. */
export function vendibles(productos, { soloActivos = true } = {}) {
  return (productos || []).filter((p) => !p.esInsumo && (!soloActivos || p.activo !== false))
}

/** esVendible indica si un producto se puede ofrecer en una pantalla de venta. */
export function esVendible(p) {
  return !!p && !p.esInsumo && p.activo !== false
}
