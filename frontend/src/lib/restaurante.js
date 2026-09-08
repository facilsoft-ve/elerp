/* Métricas del salón para el tablero de Restaurante.
 *
 * Todo se DERIVA de datos que ya trae el bootstrap: las mesas, las cuentas abiertas, los
 * documentos fiscales y el catálogo. Nada sintético: si un dato todavía no existe, la
 * vista muestra su estado vacío en vez de una cifra inventada (Regla A4 del manual UX).
 *
 * Vive en src/lib —y no dentro de la pantalla— para poder probarlo: los KPIs de un
 * tablero son justo el lugar donde un error pasa desapercibido, porque un número
 * equivocado se ve igual de convincente que el correcto. */

import { diaLocal, hoy, facturasVigentes } from './metricas.js'

const suma = (xs, f) => xs.reduce((a, x) => a + (Number(f(x)) || 0), 0)

/** totalDeCuenta suma los renglones NO cancelados de una cuenta de mesa. */
export function totalDeCuenta(c) {
  return suma((c?.items || []).filter((it) => it.estado !== 'cancelado'),
    (it) => (it.precioUnitario || 0) * (it.cantidad || 0))
}

/** esperaMinutos son los minutos desde que un renglón se envió a cocina. */
export function esperaMinutos(item, ahora = Date.now()) {
  if (!item?.enviadoEn) return 0
  const t = new Date(item.enviadoEn).getTime()
  if (Number.isNaN(t)) return 0
  return Math.max(0, Math.floor((ahora - t) / 60000))
}

/* metricasRestaurante devuelve el estado del salón AHORA y lo vendido HOY.
 *
 * · mesas       — maestro de mesas de la sede
 * · cuentas     — cuentas ABIERTAS (las que trae el bootstrap)
 * · documentos  — documentos fiscales de la empresa
 * · productos   — catálogo (para saber qué línea es un plato)
 */
export function metricasRestaurante({ mesas = [], cuentas = [], documentos = [], productos = [] } = {}, { ahora = Date.now() } = {}) {
  const activas = (mesas || []).filter((m) => m.activa !== false)
  const cuentaDe = new Map((cuentas || []).map((c) => [c.mesaId, c]))

  // Estado del salón. «Por cobrar» = ya pidió la cuenta (tiene prefactura).
  let ocupadas = 0, porCobrar = 0
  for (const m of activas) {
    const c = cuentaDe.get(m.id)
    if (!c) continue
    if ((c.prefacturas || []).length > 0) porCobrar++
    else ocupadas++
  }
  const libres = activas.length - ocupadas - porCobrar

  // Comensales sentados y consumo en curso (lo que hay en el salón sin cobrar).
  const abiertas = activas.map((m) => cuentaDe.get(m.id)).filter(Boolean)
  const comensales = suma(abiertas, (c) => c.comensales || 0)
  const consumoEnCurso = suma(abiertas, totalDeCuenta)

  // Cocina: renglones enviados y todavía no servidos, con la espera más larga.
  const enCocina = []
  for (const c of abiertas) {
    for (const it of c.items || []) {
      if (it.estado === 'en_cocina' || it.estado === 'listo') enCocina.push(it)
    }
  }
  const esperaMaxima = enCocina.reduce((max, it) => Math.max(max, esperaMinutos(it, ahora)), 0)
  const listosParaLlevar = enCocina.filter((it) => it.estado === 'listo').length

  // Facturado HOY (por día local: una venta de las 21:00 en Venezuela cae al día
  // siguiente en UTC y se atribuiría al día equivocado).
  const h = hoy()
  const delDia = facturasVigentes(documentos).filter((d) => diaLocal(d.fecha) === h)
  const ventasHoy = suma(delDia, (d) => d.total)
  const mesasCobradasHoy = delDia.length
  const ticketPromedio = mesasCobradasHoy ? ventasHoy / mesasCobradasHoy : 0
  // Rotación: cuántas veces se usó cada mesa (mesas cobradas / mesas del salón).
  const rotacion = activas.length ? mesasCobradasHoy / activas.length : 0

  // Platos más vendidos hoy (solo lo que el catálogo marca como plato).
  const esPlato = new Set((productos || []).filter((p) => p.esPlato).map((p) => p.sku))
  const porSKU = new Map()
  for (const d of delDia) {
    for (const l of d.lineas || []) {
      if (!esPlato.has(l.sku)) continue
      const prev = porSKU.get(l.sku) || { sku: l.sku, nombre: l.nombre, cantidad: 0, total: 0 }
      prev.cantidad += Number(l.cantidad) || 0
      prev.total += Number(l.total) || 0
      porSKU.set(l.sku, prev)
    }
  }
  const platosTop = [...porSKU.values()].sort((a, b) => b.cantidad - a.cantidad).slice(0, 5)

  return {
    mesasTotal: activas.length, libres, ocupadas, porCobrar,
    comensales, consumoEnCurso,
    enCocina: enCocina.length, listosParaLlevar, esperaMaxima,
    ventasHoy, mesasCobradasHoy, ticketPromedio, rotacion,
    platosTop,
    // hayDatos distingue «todavía no hubo servicio» de «un servicio en cero».
    hayDatos: mesasCobradasHoy > 0 || abiertas.length > 0,
  }
}
