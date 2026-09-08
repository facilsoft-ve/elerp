/* Métricas del workspace de Inicio.
 *
 * REGLA DE ORO: toda métrica se DERIVA de datos reales — el ledger de documentos
 * fiscales, las existencias y las transferencias. Nada sintético. El Manual UX
 * (fundamento de la Regla A4, citando a Few y Tufte) exige maximizar la relación
 * tinta-datos y prohíbe la decoración que no informa: si un dato todavía no
 * existe, la vista muestra un estado vacío, nunca una serie inventada.
 *
 * FECHAS: `Documento.fecha` es UTC (RFC3339) por §10 de los principios
 * (trazabilidad en UTC). Aquí se agrupa por día LOCAL a propósito: una venta de
 * las 21:00 en Venezuela (UTC−4) cae al día siguiente en UTC y se atribuiría al
 * día equivocado en "ventas de hoy".
 */

const clave = (d) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

export const diaLocal = (v) => {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '' : clave(d)
}
export const hoy = () => clave(new Date())
export const mesActual = () => hoy().slice(0, 7)

const suma = (arr, f) => arr.reduce((a, x) => a + (Number(f(x)) || 0), 0)

// Facturas que cuentan como venta: tipo factura y sin reversa. `anulado` lo
// deriva el backend (existe un documento de anulación que la referencia).
export const facturasVigentes = (docs) => (docs || []).filter((d) => d.tipo === 'factura' && !d.anulado)
export const reversas = (docs) => (docs || []).filter((d) => d.tipo === 'nota_credito' || d.tipo === 'anulacion')
export const sinSincronizar = (docs) => (docs || []).filter((d) => d.contingencia)

/* Serie de ventas por día para el gráfico. Devuelve SIEMPRE los últimos `dias`
 * días (con ceros donde no hubo ventas) para que el eje temporal no mienta, y
 * `hayDatos` para que la vista decida entre gráfico y estado vacío. */
export function serieVentas(docs, dias = 14) {
  const fs = facturasVigentes(docs)
  const base = new Date()
  const puntos = []
  const idx = new Map()
  for (let i = dias - 1; i >= 0; i--) {
    const d = new Date(base.getFullYear(), base.getMonth(), base.getDate() - i)
    idx.set(clave(d), puntos.length)
    puntos.push({ d: `${d.getDate()}/${d.getMonth() + 1}`, v: 0 })
  }
  let hayDatos = false
  for (const doc of fs) {
    const k = idx.get(diaLocal(doc.fecha))
    if (k !== undefined) {
      puntos[k].v += Number(doc.total) || 0
      hayDatos = true
    }
  }
  return { puntos, hayDatos }
}

/* Métricas agregadas. `actor` (= user.userId, que es lo que el backend guarda en
 * Documento.actor) acota a "lo mío" para vendedor y cajero. */
export function metricas(db, { actor, umbralStockBajo = 5 } = {}) {
  const docs = db.DOCUMENTOS || []
  const existencias = db.EXISTENCIAS || []
  const productos = db.PRODUCTOS || []
  const transferencias = db.TRANSFERENCIAS || []

  const fs = facturasVigentes(docs)
  const h = hoy()
  const m = mesActual()

  const deHoy = fs.filter((d) => diaLocal(d.fecha) === h)
  const delMes = fs.filter((d) => diaLocal(d.fecha).startsWith(m))
  const mias = actor ? fs.filter((d) => d.actor === actor) : fs
  const miasHoy = mias.filter((d) => diaLocal(d.fecha) === h)

  const pagosHoy = deHoy.flatMap((d) => d.pagos || [])
  const pagosMiosHoy = miasHoy.flatMap((d) => d.pagos || [])

  const ventasHoy = suma(deHoy, (d) => d.total)
  const ventasMias = suma(miasHoy, (d) => d.total)

  const stockBajo = existencias.filter((e) => (Number(e.cantidad) || 0) <= umbralStockBajo)
  const sinStock = existencias.filter((e) => (Number(e.cantidad) || 0) <= 0)
  const transEnCurso = transferencias.filter((t) => t.estado !== 'cerrada' && t.estado !== 'recibida')
  const pendientesSync = sinSincronizar(docs)

  return {
    // Ventas
    ventasHoy,
    ventasMes: suma(delMes, (d) => d.total),
    ventasMias,
    facturasHoy: deHoy.length,
    facturasMias: miasHoy.length,
    facturasMes: delMes.length,
    ticketHoy: deHoy.length ? ventasHoy / deHoy.length : 0,
    ticketMio: miasHoy.length ? ventasMias / miasHoy.length : 0,

    // Cobros (para la caja)
    efectivoBsHoy: suma(pagosHoy.filter((p) => p.metodo === 'efectivo_bs'), (p) => p.monto),
    efectivoBsMio: suma(pagosMiosHoy.filter((p) => p.metodo === 'efectivo_bs'), (p) => p.monto),
    divisasHoy: suma(pagosHoy.filter((p) => p.enDivisa), (p) => p.monto),

    // Fiscal (para la contadora)
    ivaMes: suma(delMes, (d) => d.iva),
    igtfMes: suma(delMes, (d) => d.igtf),
    reversasMes: reversas(docs).filter((d) => diaLocal(d.fecha).startsWith(m)).length,

    // Inventario
    valorInventario: suma(existencias, (e) => e.valor),
    productosActivos: productos.filter((p) => p.activo !== false).length,
    productosTotal: productos.length,
    stockBajo,
    sinStock,
    transEnCurso,
    transferenciasTotal: transferencias.length,
    pendientesSync,
    umbralStockBajo,
  }
}

/* Alertas accionables. El spec de flujos §2.7 define que cada notificación lleva
 * una acción directa; hasta que exista `GET /notificaciones` en el backend, estas
 * se derivan del estado real de los datos.
 * `nivel`: error | warn | info — el rojo se reserva a lo que impide operar. */
export function alertas(mt, { rol }) {
  const out = []
  const puedeInventario = ['dueno', 'desarrollador', 'vendedor', 'contadora'].includes(rol)
  // El prototipo nombra el detalle concreto (los productos, el cliente, el
  // monto): un pendiente sin sujeto no es accionable.
  const nombres = (lista) => lista.slice(0, 3).map((e) => e.nombre || e.sku).filter(Boolean).join(', ')

  if (mt.pendientesSync.length) {
    out.push({
      id: 'sync', nivel: 'warn', icono: 'doc',
      titulo: `${mt.pendientesSync.length} documento${mt.pendientesSync.length === 1 ? '' : 's'} en contingencia`,
      detalle: 'Emitidos sin conexión con serie reservada · verifica que estén reportados',
      accion: 'Ver documentos', ruta: 'facturacion:factura',
    })
  }
  if (puedeInventario && mt.sinStock.length) {
    out.push({
      id: 'sinstock', nivel: 'error', icono: 'caja',
      titulo: `${mt.sinStock.length} producto${mt.sinStock.length === 1 ? '' : 's'} agotado${mt.sinStock.length === 1 ? '' : 's'}`,
      detalle: nombres(mt.sinStock) || 'No se pueden vender hasta reponer',
      accion: 'Reponer', ruta: 'inventario:existencias',
    })
  }
  const bajos = mt.stockBajo.filter((e) => (Number(e.cantidad) || 0) > 0)
  if (puedeInventario && bajos.length) {
    out.push({
      id: 'stockbajo', nivel: 'warn', icono: 'caja',
      titulo: `${bajos.length} producto${bajos.length === 1 ? '' : 's'} bajo mínimo`,
      detalle: nombres(bajos) || `Quedan ${mt.umbralStockBajo} unidades o menos`,
      accion: 'Reponer', ruta: 'inventario:existencias',
    })
  }
  if (puedeInventario && mt.transEnCurso.length) {
    out.push({
      id: 'transf', nivel: 'info', icono: 'camion',
      titulo: `${mt.transEnCurso.length} transferencia${mt.transEnCurso.length === 1 ? '' : 's'} en curso`,
      detalle: 'Stock en tránsito entre sedes, pendiente de recepción',
      accion: 'Ver transferencias', ruta: 'inventario:transferencias',
    })
  }
  return out
}
