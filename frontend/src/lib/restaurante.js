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

/** totalDeCuenta suma TODO lo consumido en la mesa (renglones no cancelados),
 *  esté facturado o no. Es el consumo de la mesa, no lo que falta por cobrar. */
export function totalDeCuenta(c) {
  return suma((c?.items || []).filter((it) => it.estado !== 'cancelado'),
    (it) => (it.precioUnitario || 0) * (it.cantidad || 0))
}

/** pendienteDeCuenta es lo que TODAVÍA no entró en ninguna solicitud de facturación.
 *  Es lo que el tablero debe mostrar: una mesa donde uno ya pagó y se fue no le debe
 *  al local lo que ese comensal pagó, y mostrarlo hace que el mesonero cobre de más. */
export function pendienteDeCuenta(c) {
  return suma((c?.items || []).filter((it) => it.estado !== 'cancelado' && !it.prefacturaId),
    (it) => (it.precioUnitario || 0) * (it.cantidad || 0))
}

/** facturadoDeCuenta es lo que ya se pidió facturar (cobrado o esperando en la caja). */
export function facturadoDeCuenta(c) {
  return suma((c?.items || []).filter((it) => it.estado !== 'cancelado' && it.prefacturaId),
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

  // Estado del salón. «Por cobrar» = ya pidió factura de algo y sigue esperando a la
  // caja. Una mesa donde ya pagaron una parte y siguen comiendo cuenta como ocupada
  // mientras quede consumo sin pedir: lo que le falta al local es atenderla, no cobrarla.
  let ocupadas = 0, porCobrar = 0
  for (const m of activas) {
    const c = cuentaDe.get(m.id)
    if (!c) continue
    if ((c.prefacturas || []).length > 0 && pendienteDeCuenta(c) === 0) porCobrar++
    else ocupadas++
  }
  const libres = activas.length - ocupadas - porCobrar

  // Comensales sentados y consumo en curso (lo que hay en el salón sin cobrar).
  const abiertas = activas.map((m) => cuentaDe.get(m.id)).filter(Boolean)
  const comensales = suma(abiertas, (c) => c.comensales || 0)
  // Consumo EN CURSO = lo que todavía no se pidió facturar. Lo ya facturado no está
  // "en curso": o está en la caja o ya se cobró.
  const consumoEnCurso = suma(abiertas, pendienteDeCuenta)

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

/* solicitudesDeMesa son las SOLICITUDES DE FACTURACIÓN que esperan en la caja.
 *
 * Cuando el mesonero pide la cuenta (entera o de un segmento de la mesa), el servidor
 * crea una cotización CONFIRMADA rotulada con la mesa. Eso es lo único que el cajero
 * necesita ver: qué mesa pidió factura y por cuánto. No busca números de cotización.
 *
 * Se ordenan por mesa —el cajero piensa en mesas, no en documentos— y, dentro de la
 * misma mesa, por antigüedad: si una mesa pidió dos veces, primero la que lleva
 * esperando más rato.
 */
export function solicitudesDeMesa(cotizaciones = []) {
  const vivas = (cotizaciones || []).filter(
    (c) => c && c.estado === 'confirmada' && (c.cuentaMesaId || c.mesaNombre))
  return vivas
    .map((c) => ({
      id: c.id,
      numero: c.numeroCompleto || '',
      mesaNombre: c.mesaNombre || '',
      cuentaMesaId: c.cuentaMesaId || '',
      // La nota es lo que distingue una parte de otra ("Mesa 4 · parte 2 de 3").
      nota: c.notas || '',
      clienteId: c.clienteId || '',
      clienteNombre: c.clienteNombre || '',
      total: Number(c.total) || 0,
      creada: c.creada || '',
      lineas: (c.lineas || []).map((l) => ({
        sku: l.sku, nombre: l.nombre, cantidad: Number(l.cantidad) || 0,
        precioUnitario: Number(l.precioUnitario) || 0, exento: !!l.exento,
      })),
    }))
    .sort((a, b) => {
      const m = compararMesa(a.mesaNombre, b.mesaNombre)
      return m !== 0 ? m : String(a.creada).localeCompare(String(b.creada))
    })
}

/* compararMesa ordena "2" antes que "10" (numérico cuando ambos lo son) y, si no,
 * alfabéticamente: los nombres de mesa suelen ser números, pero admiten "Terraza 1". */
function compararMesa(a, b) {
  const na = Number(a), nb = Number(b)
  if (Number.isFinite(na) && Number.isFinite(nb)) return na - nb
  return String(a).localeCompare(String(b), 'es', { numeric: true })
}
