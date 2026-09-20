/* Piezas compartidas del módulo de Compras.
 *
 * Viven acá —y no dentro de una pantalla— porque las usan dos o más: la ficha del
 * proveedor pacta la condición de pago y el perfil de retenciones, y la orden de
 * compra los propone y proyecta. Tener una sola definición evita que la lista de
 * condiciones diga una cosa en un sitio y otra en el otro. */

// Condiciones de pago admitidas. El backend guarda el texto tal cual: es una
// etiqueta pactada, no una regla de vencimiento (Cuentas por pagar todavía no
// calcula fecha de vencimiento ni antigüedad de la deuda con proveedores).
export const COND_PAGO = ['Contado', '15 días', '30 días', '60 días']

/* --- Retenciones proyectadas -----------------------------------------------
 *
 * Réplica EN VIVO del cálculo del servidor, para que la orden muestre el neto a
 * pagar mientras se arma. El backend recalcula con las mismas reglas al guardar y
 * es la autoridad: si algo difiere, manda lo que devuelve el servidor.
 *
 * Dos condiciones para que se retenga: la EMPRESA debe ser agente de retención de
 * ese impuesto y el PROVEEDOR tenerlo activado. */

// Porcentaje de retención de IVA por defecto de la providencia, cuando ni el
// proveedor ni la empresa declaran el suyo.
export const RET_IVA_DEFAULT = 75

// perfilRetencionDe extrae del proveedor el perfil que la orden propone.
export function perfilRetencionDe(prov) {
  return {
    retieneIva: !!prov?.retieneIva,
    ivaPorcentaje: Number(prov?.retencionIvaPorcentaje) || 0,
    retieneIslr: !!prov?.retieneIslr,
    islrConceptoCodigo: prov?.conceptoIslrCodigo || '',
    islrSujeto: prov?.sujetoIslr || '',
  }
}

// basesIslrPorConcepto agrupa el neto de las líneas por concepto, devolviendo los
// códigos ORDENADOS y su base. Espejo de basesISLRPorConcepto en el servidor: el
// orden alfabético hace que el desglose salga igual en la pantalla y en la orden
// ya guardada. Las líneas sin concepto (mercancía) no entran en ninguna base.
function basesIslrPorConcepto(lineas) {
  const r2 = (v) => Math.round(v * 100) / 100
  const bases = new Map()
  for (const l of lineas || []) {
    const cod = String(l?.conceptoIslr || '').trim().toLowerCase()
    if (!cod) continue
    bases.set(cod, r2((bases.get(cod) || 0) + (Number(l.total) || 0)))
  }
  return [...bases.keys()].sort().map((codigo) => ({ codigo, base: bases.get(codigo) }))
}

// proyectarRetenciones calcula lo que se le retendrá al proveedor y el neto que
// recibirá. `empresa` aporta si es agente de retención y su % de IVA por defecto.
//
//   - IVA: se retiene sobre el IVA de la orden.
//   - ISLR: por CONCEPTO DEL PAGO. El concepto lo declara cada línea (lo trae el
//     producto) y se agrupa por concepto; si ninguna línea lo declara, se usa el
//     del perfil sobre el neto completo. La tarifa NO se teclea: sale del MAESTRO,
//     que también decide el sustraendo y la base mínima. `conceptos` son los del
//     maestro (hook useConceptosISLR); sin ellos la proyección de ISLR queda en
//     cero y la manda el servidor, que sí lo tiene.
//   - `acumulados` es cuántas UT se le llevan retenidas al proveedor en el
//     ejercicio, por código de concepto. Decide el TRAMO de la escala (Tarifa 2 de
//     los no domiciliados). Vacío ⇒ primer tramo, que es lo correcto mientras no
//     se haya elegido proveedor.
//   - `valorUt` es el valor de la Unidad Tributaria del día. El maestro guarda el
//     sustraendo y el mínimo en UT, no en bolívares. Sin UT, un concepto que la
//     necesita NO se calcula con cero —eso retendría de más en silencio—: se marca
//     con impedimento y se explica.
//
// `detalle` es el desglose por concepto y la fuente de verdad. Los escalares
// describen el caso de un solo concepto; con varios, `islrPorcentaje` queda en 0
// porque no existe una tarifa única que describa la mezcla.
export function proyectarRetenciones({ empresa, perfil, conceptos = [], valorUt = 0, acumulados = {}, lineas = [], subtotal, iva, total }) {
  const r2 = (v) => Math.round(v * 100) / 100
  const out = {
    ivaPorcentaje: 0, ivaMonto: 0,
    islrPorcentaje: 0, islrSustraendo: 0, islrMonto: 0, islrConcepto: '', detalle: [],
    neto: total,
  }

  if (empresa?.agenteRetencionIVA && perfil?.retieneIva && iva > 0.004) {
    out.ivaPorcentaje = perfil.ivaPorcentaje > 0
      ? perfil.ivaPorcentaje
      : (Number(empresa?.retencionIVAPorcentaje) || RET_IVA_DEFAULT)
    out.ivaMonto = r2(iva * out.ivaPorcentaje / 100)
  }
  if (!empresa?.agenteRetencionISLR || !perfil?.retieneIslr) {
    out.neto = total - out.ivaMonto
    return out
  }

  const activos = conceptos.filter((x) => x.activo !== false)
  let grupos = basesIslrPorConcepto(lineas)
  if (grupos.length === 0) {
    const cod = String(perfil.islrConceptoCodigo || '').trim().toLowerCase()
    if (!cod || !(subtotal > 0.004)) {
      out.neto = total - out.ivaMonto
      return out
    }
    grupos = [{ codigo: cod, base: subtotal }]
  }

  for (const g of grupos) {
    if (!(g.base > 0.004)) continue
    // Tramo de la escala: el de DesdeAcumuladoUT más alto que no supere al
    // acumulado del ejercicio CON este pago dentro. Espejo de fiscal.ConceptoPara.
    const tramos = activos.filter((x) => String(x.codigo).toLowerCase() === g.codigo && x.sujeto === perfil.islrSujeto)
    const pctBaseDe = (x) => (Number(x?.porcentajeBase) > 0 ? Number(x.porcentajeBase) : 100)
    const primero = tramos.slice().sort((a, b) => (a.desdeAcumuladoUt || 0) - (b.desdeAcumuladoUt || 0))[0]
    const gravablePrevio = g.base * pctBaseDe(primero) / 100
    const acumulado = (Number(acumulados[g.codigo]) || 0) + (valorUt > 0 ? gravablePrevio / valorUt : 0)
    const alcanzables = tramos.filter((x) => (Number(x.desdeAcumuladoUt) || 0) <= acumulado)
    // Sin tramo alcanzable —una escala mal cargada, sin piso en 0— se cae al más
    // bajo: retener por debajo es preferible a no retener y que nadie se entere.
    const c = (alcanzables.length ? alcanzables : tramos)
      .slice().sort((a, b) => (b.desdeAcumuladoUt || 0) - (a.desdeAcumuladoUt || 0))[alcanzables.length ? 0 : tramos.length - 1]
    if (!c) {
      // El maestro no tiene tarifa de ese concepto para este tipo de sujeto. Se
      // deja constancia con monto 0: callarlo haría que una tabla incompleta se
      // viera igual que «a este proveedor no se le retiene».
      const nombre = activos.find((x) => String(x.codigo).toLowerCase() === g.codigo)?.nombre || g.codigo
      out.detalle.push({ codigo: g.codigo, concepto: nombre, base: g.base, porcentaje: 0, sustraendo: 0, monto: 0, impedimento: 'sin_tarifa' })
      continue
    }
    const porcentaje = Number(c.porcentaje) || 0
    // No todo el pago forma la base en algunos conceptos (90 % en honorarios al
    // exterior). Aplicar la tarifa al total retendría de más.
    const gravable = g.base * pctBaseDe(c) / 100
    const sustraendoUt = Number(c.sustraendoUt) || 0
    const baseMinimaUt = Number(c.baseMinimaUt) || 0
    if ((sustraendoUt > 0 || baseMinimaUt > 0) && !(valorUt > 0)) {
      // El concepto está en unidades tributarias y no hay UT cargada. Calcular con
      // cero anularía el sustraendo y retendría DE MÁS, sin fallar nada.
      out.detalle.push({ codigo: c.codigo, concepto: c.nombre, base: r2(gravable), porcentaje, sustraendo: 0, monto: 0, impedimento: 'sin_ut' })
      continue
    }
    // Base mínima: por debajo de ella el concepto no retiene, y eso NO es lo mismo
    // que «no aplica». Misma regla que ConceptoISLR.Retener en el servidor: el
    // sustraendo sigue a la tarifa (UT × unidades × porcentaje).
    const sustraendoBs = sustraendoUt * valorUt * porcentaje / 100
    let monto = 0
    if (gravable >= baseMinimaUt * valorUt) {
      const bruto = gravable * porcentaje / 100 - sustraendoBs
      monto = bruto > 0 ? r2(bruto) : 0
    }
    // La base que se informa es la GRAVABLE: con el neto de las líneas, el monto
    // no se podría rehacer a mano.
    out.detalle.push({ codigo: c.codigo, concepto: c.nombre, base: r2(gravable), porcentaje, sustraendo: r2(sustraendoBs), monto })
    out.islrMonto = r2(out.islrMonto + monto)
  }

  if (out.detalle.length === 1) {
    const d = out.detalle[0]
    out.islrConcepto = d.concepto
    out.islrPorcentaje = d.porcentaje
    out.islrSustraendo = d.sustraendo
  } else if (out.detalle.length > 1) {
    out.islrConcepto = out.detalle.map((d) => d.concepto).join(' · ')
  }
  // Los tres importes ya están redondeados: restarlos no necesita otro redondeo.
  out.neto = total - out.ivaMonto - out.islrMonto
  return out
}
