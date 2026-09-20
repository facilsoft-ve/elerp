import { describe, it, expect } from 'vitest'
import { COND_PAGO, RET_IVA_DEFAULT, perfilRetencionDe, proyectarRetenciones } from '../compras.js'

/* La proyección del frontend replica la del servidor: el usuario tiene que ver el
 * mismo neto a pagar mientras arma la orden que el que queda guardado. Si estos
 * números y los de compra_retenciones_test.go dejan de coincidir, uno de los dos
 * está mintiendo. Base de trabajo: subtotal 1.000, IVA 160, total 1.160. */

const ORDEN = { subtotal: 1000, iva: 160, total: 1160 }
const AGENTE = { agenteRetencionIVA: true, agenteRetencionISLR: true, retencionIVAPorcentaje: 75 }

/* Maestro de conceptos, como lo devuelve el API. El mismo código con DOS tarifas
 * según el sujeto es justo lo que el texto libre no podía expresar. */
const CONCEPTOS = [
  { codigo: 'honorarios', nombre: 'Honorarios profesionales', sujeto: 'natural_residente', porcentaje: 3, sustraendo: 0, activo: true },
  { codigo: 'honorarios', nombre: 'Honorarios profesionales', sujeto: 'juridica_domiciliada', porcentaje: 5, sustraendo: 0, activo: true },
  { codigo: 'consultoria', nombre: 'Consultoría', sujeto: 'juridica_domiciliada', porcentaje: 5, sustraendo: 0, baseMinima: 5000, activo: true },
]

describe('COND_PAGO', () => {
  it('arranca en Contado y ofrece los plazos usuales', () => {
    expect(COND_PAGO[0]).toBe('Contado')
    expect(COND_PAGO).toContain('30 días')
  })
})

describe('perfilRetencionDe', () => {
  it('un proveedor sin perfil no retiene nada', () => {
    expect(perfilRetencionDe(null)).toEqual({
      retieneIva: false, ivaPorcentaje: 0,
      retieneIslr: false, islrConceptoCodigo: '', islrSujeto: '',
    })
  })

  it('lee el perfil del proveedor tal como lo devuelve el API', () => {
    const p = perfilRetencionDe({
      retieneIva: true, retencionIvaPorcentaje: 100,
      retieneIslr: true, conceptoIslrCodigo: 'honorarios', sujetoIslr: 'natural_residente',
    })
    expect(p.ivaPorcentaje).toBe(100)
    expect(p.islrConceptoCodigo).toBe('honorarios')
    expect(p.islrSujeto).toBe('natural_residente')
  })
})

describe('proyectarRetenciones', () => {
  it('sin perfil, el neto a pagar es el total', () => {
    const r = proyectarRetenciones({ empresa: AGENTE, perfil: perfilRetencionDe(null), ...ORDEN })
    expect(r.ivaMonto).toBe(0)
    expect(r.islrMonto).toBe(0)
    expect(r.neto).toBe(1160)
  })

  it('retiene IVA con el porcentaje de la empresa cuando el proveedor no trae el suyo', () => {
    const r = proyectarRetenciones({ empresa: AGENTE, perfil: { retieneIva: true, ivaPorcentaje: 0 }, ...ORDEN })
    expect(r.ivaPorcentaje).toBe(75)
    expect(r.ivaMonto).toBe(120) // 160 × 75%
    expect(r.neto).toBe(1040)
  })

  it('el porcentaje del proveedor gana al de la empresa', () => {
    const r = proyectarRetenciones({ empresa: AGENTE, perfil: { retieneIva: true, ivaPorcentaje: 100 }, ...ORDEN })
    expect(r.ivaMonto).toBe(160)
  })

  it('cae al 75% de la providencia si nadie declara porcentaje', () => {
    const r = proyectarRetenciones({
      empresa: { agenteRetencionIVA: true }, perfil: { retieneIva: true, ivaPorcentaje: 0 }, ...ORDEN,
    })
    expect(r.ivaPorcentaje).toBe(RET_IVA_DEFAULT)
    expect(r.ivaMonto).toBe(120)
  })

  it('la tarifa de ISLR sale del maestro, no del perfil', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, conceptos: CONCEPTOS,
      perfil: { retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'natural_residente' },
      ...ORDEN,
    })
    expect(r.islrPorcentaje).toBe(3)
    expect(r.islrMonto).toBe(30) // 1.000 × 3%
    expect(r.neto).toBe(1130)
  })

  it('el MISMO concepto cobra distinto según el sujeto', () => {
    const base = { empresa: AGENTE, conceptos: CONCEPTOS, ...ORDEN }
    const natural = proyectarRetenciones({ ...base, perfil: { retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'natural_residente' } })
    const juridica = proyectarRetenciones({ ...base, perfil: { retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'juridica_domiciliada' } })
    expect(natural.islrMonto).toBe(30)  // 3%
    expect(juridica.islrMonto).toBe(50) // 5%
  })

  it('por debajo de la base mínima del concepto no se retiene', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, conceptos: CONCEPTOS,
      perfil: { retieneIslr: true, islrConceptoCodigo: 'consultoria', islrSujeto: 'juridica_domiciliada' },
      ...ORDEN, // base 1.000, mínimo 5.000
    })
    expect(r.islrMonto).toBe(0)
    expect(r.neto).toBe(1160)
  })

  it('un concepto que no está en el maestro no retiene: lo resuelve el servidor', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, conceptos: CONCEPTOS,
      perfil: { retieneIslr: true, islrConceptoCodigo: 'no_existe', islrSujeto: 'natural_residente' },
      ...ORDEN,
    })
    expect(r.islrMonto).toBe(0)
  })

  it('un sustraendo mayor que el cálculo no vuelve negativa la retención', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE,
      conceptos: [{ codigo: 'x', sujeto: 'juridica_domiciliada', porcentaje: 2, sustraendo: 500, activo: true }],
      perfil: { retieneIslr: true, islrConceptoCodigo: 'x', islrSujeto: 'juridica_domiciliada' },
      ...ORDEN,
    })
    expect(r.islrMonto).toBe(0)
    expect(r.neto).toBe(1160)
  })

  it('no retiene si la empresa no es agente de retención, aunque el proveedor lo tenga activo', () => {
    const r = proyectarRetenciones({
      empresa: { agenteRetencionIVA: false, agenteRetencionISLR: false }, conceptos: CONCEPTOS,
      perfil: { retieneIva: true, ivaPorcentaje: 75, retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'natural_residente' },
      ...ORDEN,
    })
    expect(r.ivaMonto).toBe(0)
    expect(r.islrMonto).toBe(0)
    expect(r.neto).toBe(1160)
  })

  it('combina ambas retenciones en el neto', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, conceptos: CONCEPTOS,
      perfil: { retieneIva: true, ivaPorcentaje: 100, retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'natural_residente' },
      ...ORDEN,
    })
    expect(r.ivaMonto).toBe(160)
    expect(r.islrMonto).toBe(30)
    expect(r.neto).toBe(970) // el mismo número que asserta el test de Go
  })

  it('una orden sin IVA (todo exento) no genera retención de IVA', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, perfil: { retieneIva: true, ivaPorcentaje: 75 },
      subtotal: 1000, iva: 0, total: 1000,
    })
    expect(r.ivaMonto).toBe(0)
    expect(r.neto).toBe(1000)
  })
})

/* EL CONCEPTO LO DECLARA LA LÍNEA (lo trae el producto).
 *
 * Misma tabla de casos que compra_islr_producto_test.go en el servidor. El
 * maestro de este fixture trae fletes SOLO para jurídica domiciliada, que es lo
 * que permite probar el concepto sin tarifa para el sujeto del proveedor. */
const CONCEPTOS_CON_FLETES = [
  ...CONCEPTOS,
  { codigo: 'fletes', nombre: 'Fletes y transporte', sujeto: 'juridica_domiciliada', porcentaje: 3, sustraendo: 0, activo: true },
]

const JURIDICA = { retieneIva: false, ivaPorcentaje: 0, retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'juridica_domiciliada' }

describe('proyectarRetenciones · el concepto sale de las líneas', () => {
  it('retiene por el concepto del PRODUCTO, no por el del perfil del proveedor', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE,
      // El perfil dice fletes (3 %), pero se compró una consultoría de honorarios.
      perfil: { ...JURIDICA, islrConceptoCodigo: 'fletes' },
      conceptos: CONCEPTOS_CON_FLETES,
      lineas: [{ conceptoIslr: 'honorarios', total: 1000 }],
      ...ORDEN,
    })
    expect(r.detalle).toHaveLength(1)
    expect(r.detalle[0].codigo).toBe('honorarios')
    expect(r.islrPorcentaje).toBe(5)
    expect(r.islrMonto).toBe(50)
  })

  it('la mercancía no entra en la base: solo el neto de las líneas con concepto', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, perfil: JURIDICA, conceptos: CONCEPTOS_CON_FLETES,
      lineas: [{ conceptoIslr: 'honorarios', total: 1000 }, { conceptoIslr: '', total: 1000 }],
      subtotal: 2000, iva: 320, total: 2320,
    })
    expect(r.detalle).toHaveLength(1)
    expect(r.detalle[0].base).toBe(1000)
    expect(r.islrMonto).toBe(50)
  })

  it('varios conceptos se desglosan y el porcentaje único deja de existir', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, perfil: JURIDICA, conceptos: CONCEPTOS_CON_FLETES,
      lineas: [{ conceptoIslr: 'honorarios', total: 1000 }, { conceptoIslr: 'fletes', total: 1000 }],
      subtotal: 2000, iva: 320, total: 2320,
    })
    // Ordenado por código, igual que en el servidor: fletes antes que honorarios.
    expect(r.detalle.map((d) => d.codigo)).toEqual(['fletes', 'honorarios'])
    expect(r.islrMonto).toBe(80) // 3% de 1.000 + 5% de 1.000
    expect(r.islrPorcentaje).toBe(0)
    expect(r.islrConcepto).toContain('·')
    expect(r.neto).toBe(2240)
  })

  it('un concepto sin tarifa para ese sujeto queda visible con monto 0', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE,
      perfil: { ...JURIDICA, islrSujeto: 'natural_residente' },
      conceptos: CONCEPTOS_CON_FLETES,
      lineas: [{ conceptoIslr: 'fletes', total: 1000 }],
      ...ORDEN,
    })
    expect(r.detalle).toHaveLength(1)
    expect(r.detalle[0].sinTarifa).toBe(true)
    expect(r.detalle[0].concepto).toBe('Fletes y transporte')
    expect(r.detalle[0].base).toBe(1000)
    expect(r.islrMonto).toBe(0)
  })

  it('sin concepto en ninguna línea manda el del proveedor sobre el neto', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, perfil: JURIDICA, conceptos: CONCEPTOS_CON_FLETES,
      lineas: [{ conceptoIslr: '', total: 1000 }],
      ...ORDEN,
    })
    expect(r.detalle).toHaveLength(1)
    expect(r.detalle[0].codigo).toBe('honorarios')
    expect(r.islrMonto).toBe(50)
  })

  it('la base mínima se evalúa POR CONCEPTO, no sobre el subtotal', () => {
    // Consultoría tiene mínimo 5.000: con 1.000 de base no retiene, aunque la
    // orden entera sume más.
    const r = proyectarRetenciones({
      empresa: AGENTE,
      perfil: { ...JURIDICA, islrConceptoCodigo: 'consultoria' },
      conceptos: CONCEPTOS_CON_FLETES,
      lineas: [{ conceptoIslr: 'consultoria', total: 1000 }, { conceptoIslr: 'honorarios', total: 6000 }],
      subtotal: 7000, iva: 1120, total: 8120,
    })
    const consultoria = r.detalle.find((d) => d.codigo === 'consultoria')
    expect(consultoria.monto).toBe(0)
    expect(r.islrMonto).toBe(300) // solo honorarios: 5% de 6.000
  })
})
