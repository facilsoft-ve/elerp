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

// proyectarRetenciones calcula lo que se le retendrá al proveedor y el neto que
// recibirá. `empresa` aporta si es agente de retención y su % de IVA por defecto.
//
//   - IVA: se retiene sobre el IVA de la orden.
//   - ISLR: sobre el NETO (subtotal sin impuesto, que es el ingreso del proveedor).
//     La tarifa NO se teclea: sale del MAESTRO de conceptos, que también decide el
//     sustraendo y la base mínima. `conceptos` son los del maestro (hook
//     useConceptosISLR); sin ellos la proyección de ISLR queda en cero y la manda
//     el servidor, que sí lo tiene.
export function proyectarRetenciones({ empresa, perfil, conceptos = [], subtotal, iva, total }) {
  const r2 = (v) => Math.round(v * 100) / 100
  const out = { ivaPorcentaje: 0, ivaMonto: 0, islrPorcentaje: 0, islrSustraendo: 0, islrMonto: 0, neto: total }

  if (empresa?.agenteRetencionIVA && perfil?.retieneIva && iva > 0.004) {
    out.ivaPorcentaje = perfil.ivaPorcentaje > 0
      ? perfil.ivaPorcentaje
      : (Number(empresa?.retencionIVAPorcentaje) || RET_IVA_DEFAULT)
    out.ivaMonto = r2(iva * out.ivaPorcentaje / 100)
  }
  if (empresa?.agenteRetencionISLR && perfil?.retieneIslr && subtotal > 0.004) {
    const c = conceptos.find((x) => x.activo !== false
      && x.codigo === perfil.islrConceptoCodigo && x.sujeto === perfil.islrSujeto)
    // Base mínima: por debajo de ella el concepto no retiene, y eso NO es lo mismo
    // que «no aplica». Misma regla que ConceptoISLR.Retener en el servidor.
    if (c && subtotal >= (Number(c.baseMinima) || 0)) {
      out.islrPorcentaje = Number(c.porcentaje) || 0
      out.islrSustraendo = r2(Number(c.sustraendo) || 0)
      const neto = r2(subtotal * out.islrPorcentaje / 100) - out.islrSustraendo
      if (neto > 0.004) out.islrMonto = r2(neto)
    }
  }
  // Los tres importes ya están redondeados: restarlos no necesita otro redondeo.
  out.neto = total - out.ivaMonto - out.islrMonto
  return out
}
