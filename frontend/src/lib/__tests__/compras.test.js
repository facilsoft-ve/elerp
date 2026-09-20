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
  { codigo: 'honorarios', nombre: 'Honorarios profesionales', sujeto: 'natural_residente', porcentaje: 3, sustraendoUt: 0, activo: true },
  { codigo: 'honorarios', nombre: 'Honorarios profesionales', sujeto: 'juridica_domiciliada', porcentaje: 5, sustraendoUt: 0, activo: true },
  { codigo: 'consultoria', nombre: 'Consultoría', sujeto: 'juridica_domiciliada', porcentaje: 5, sustraendoUt: 0, baseMinimaUt: 5000, activo: true },
]

/* El sustraendo y el mínimo del maestro van en UNIDADES TRIBUTARIAS: los
 * bolívares salen de multiplicarlos por la UT del día. En las pruebas la UT vale
 * 1 para que los números se puedan seguir a mano. */
const UT = 1

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
      empresa: AGENTE, conceptos: CONCEPTOS, valorUt: UT,
      perfil: { retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'natural_residente' },
      ...ORDEN,
    })
    expect(r.islrPorcentaje).toBe(3)
    expect(r.islrMonto).toBe(30) // 1.000 × 3%
    expect(r.neto).toBe(1130)
  })

  it('el MISMO concepto cobra distinto según el sujeto', () => {
    const base = { empresa: AGENTE, conceptos: CONCEPTOS, valorUt: UT, ...ORDEN }
    const natural = proyectarRetenciones({ ...base, perfil: { retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'natural_residente' } })
    const juridica = proyectarRetenciones({ ...base, perfil: { retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'juridica_domiciliada' } })
    expect(natural.islrMonto).toBe(30)  // 3%
    expect(juridica.islrMonto).toBe(50) // 5%
  })

  it('por debajo de la base mínima del concepto no se retiene', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, conceptos: CONCEPTOS, valorUt: UT,
      perfil: { retieneIslr: true, islrConceptoCodigo: 'consultoria', islrSujeto: 'juridica_domiciliada' },
      ...ORDEN, // base 1.000, mínimo 5.000
    })
    expect(r.islrMonto).toBe(0)
    expect(r.neto).toBe(1160)
  })

  it('un concepto que no está en el maestro no retiene: lo resuelve el servidor', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, conceptos: CONCEPTOS, valorUt: UT,
      perfil: { retieneIslr: true, islrConceptoCodigo: 'no_existe', islrSujeto: 'natural_residente' },
      ...ORDEN,
    })
    expect(r.islrMonto).toBe(0)
  })

  it('un sustraendo mayor que el cálculo no vuelve negativa la retención', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE,
      conceptos: [{ codigo: 'x', sujeto: 'juridica_domiciliada', porcentaje: 2, sustraendoUt: 25000, activo: true }], valorUt: UT,
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
      empresa: AGENTE, conceptos: CONCEPTOS, valorUt: UT,
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
  { codigo: 'fletes', nombre: 'Fletes y transporte', sujeto: 'juridica_domiciliada', porcentaje: 3, sustraendoUt: 0, activo: true },
]

const JURIDICA = { retieneIva: false, ivaPorcentaje: 0, retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'juridica_domiciliada' }

describe('proyectarRetenciones · el concepto sale de las líneas', () => {
  it('retiene por el concepto del PRODUCTO, no por el del perfil del proveedor', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE,
      // El perfil dice fletes (3 %), pero se compró una consultoría de honorarios.
      perfil: { ...JURIDICA, islrConceptoCodigo: 'fletes' },
      conceptos: CONCEPTOS_CON_FLETES, valorUt: UT,
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
      empresa: AGENTE, perfil: JURIDICA, conceptos: CONCEPTOS_CON_FLETES, valorUt: UT,
      lineas: [{ conceptoIslr: 'honorarios', total: 1000 }, { conceptoIslr: '', total: 1000 }],
      subtotal: 2000, iva: 320, total: 2320,
    })
    expect(r.detalle).toHaveLength(1)
    expect(r.detalle[0].base).toBe(1000)
    expect(r.islrMonto).toBe(50)
  })

  it('varios conceptos se desglosan y el porcentaje único deja de existir', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, perfil: JURIDICA, conceptos: CONCEPTOS_CON_FLETES, valorUt: UT,
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
      conceptos: CONCEPTOS_CON_FLETES, valorUt: UT,
      lineas: [{ conceptoIslr: 'fletes', total: 1000 }],
      ...ORDEN,
    })
    expect(r.detalle).toHaveLength(1)
    expect(r.detalle[0].impedimento).toBe('sin_tarifa')
    expect(r.detalle[0].concepto).toBe('Fletes y transporte')
    expect(r.detalle[0].base).toBe(1000)
    expect(r.islrMonto).toBe(0)
  })

  it('sin concepto en ninguna línea manda el del proveedor sobre el neto', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, perfil: JURIDICA, conceptos: CONCEPTOS_CON_FLETES, valorUt: UT,
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
      conceptos: CONCEPTOS_CON_FLETES, valorUt: UT,
      lineas: [{ conceptoIslr: 'consultoria', total: 1000 }, { conceptoIslr: 'honorarios', total: 6000 }],
      subtotal: 7000, iva: 1120, total: 8120,
    })
    const consultoria = r.detalle.find((d) => d.codigo === 'consultoria')
    expect(consultoria.monto).toBe(0)
    expect(r.islrMonto).toBe(300) // solo honorarios: 5% de 6.000
  })
})

/* EL SUSTRAENDO VIVE EN UNIDADES TRIBUTARIAS.
 *
 * Misma tabla de casos que ut_test.go en el servidor. Si estos números y los de
 * allá dejan de coincidir, uno de los dos está mintiendo. */
describe('proyectarRetenciones · la unidad tributaria', () => {
  // Honorarios a persona natural con el sustraendo del reglamento (83,33 UT).
  const CON_SUSTRAENDO = [
    { codigo: 'honorarios', nombre: 'Honorarios profesionales', sujeto: 'natural_residente',
      porcentaje: 3, sustraendoUt: 83.33, baseMinimaUt: 83.33, activo: true },
  ]
  const NATURAL = { retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'natural_residente' }

  it('el sustraendo se convierte a bolívares con la UT del día', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, perfil: NATURAL, conceptos: CON_SUSTRAENDO, valorUt: 10, ...ORDEN,
    })
    // Mínimo 83,33 × 10 = 833,30 ≤ 1.000 ⇒ retiene.
    // Sustraendo 83,33 × 10 × 3 % = 25,00. Retención 30 − 25 = 5,00.
    expect(r.detalle[0].sustraendo).toBe(25)
    expect(r.islrMonto).toBe(5)
  })

  it('subir la UT sube el sustraendo sin tocar el maestro', () => {
    const con = (valorUt) => proyectarRetenciones({
      empresa: AGENTE, perfil: NATURAL, conceptos: CON_SUSTRAENDO, valorUt, ...ORDEN,
    }).detalle[0].sustraendo
    expect(con(10)).toBe(25)
    expect(con(20)).toBe(50)
  })

  it('sin UT cargada NO calcula con cero: lo dice', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, perfil: NATURAL, conceptos: CON_SUSTRAENDO, valorUt: 0, ...ORDEN,
    })
    // Con la UT en cero el sustraendo se anularía y retendría 30 —de más— sin
    // fallar nada. Ese es exactamente el error que hay que hacer visible.
    expect(r.detalle[0].impedimento).toBe('sin_ut')
    expect(r.islrMonto).toBe(0)
    expect(r.neto).toBe(1160)
  })

  it('una tarifa sin sustraendo ni mínimo no depende de la UT', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, valorUt: 0, conceptos: CONCEPTOS,
      perfil: { retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'juridica_domiciliada' },
      ...ORDEN,
    })
    expect(r.detalle[0].impedimento).toBeUndefined()
    expect(r.islrMonto).toBe(50)
  })
})

/* TARIFA 2: LA ESCALA DE LOS NO DOMICILIADOS.
 *
 * Misma tabla de casos que islr_tarifa2_test.go en el servidor. El tramo lo
 * decide el acumulado del ejercicio, no la factura de hoy, y la base es el 90 %
 * del pago. Con UT = 1 las UT acumuladas se leen como bolívares. */
describe('proyectarRetenciones · la escala por tramos', () => {
  const ESCALA = [
    { codigo: 'hon_ext', nombre: 'Honorarios del exterior', sujeto: 'juridica_no_domiciliada',
      porcentaje: 15, porcentajeBase: 90, desdeAcumuladoUt: 0, activo: true },
    { codigo: 'hon_ext', nombre: 'Honorarios del exterior', sujeto: 'juridica_no_domiciliada',
      porcentaje: 22, porcentajeBase: 90, desdeAcumuladoUt: 2000.01, activo: true },
    { codigo: 'hon_ext', nombre: 'Honorarios del exterior', sujeto: 'juridica_no_domiciliada',
      porcentaje: 34, porcentajeBase: 90, desdeAcumuladoUt: 3000.01, activo: true },
  ]
  const EXTERIOR = { retieneIslr: true, islrConceptoCodigo: 'hon_ext', islrSujeto: 'juridica_no_domiciliada' }
  const conAcumulado = (ut) => proyectarRetenciones({
    empresa: AGENTE, perfil: EXTERIOR, conceptos: ESCALA, valorUt: 1,
    acumulados: { hon_ext: ut },
    lineas: [{ conceptoIslr: 'hon_ext', total: 1000 }],
    ...ORDEN,
  })

  it('sin nada acumulado cae en el primer tramo', () => {
    const r = conAcumulado(0)
    expect(r.detalle[0].porcentaje).toBe(15)
    // La base es el 90 % del pago: 900, no 1.000. Sobre el total serían 150.
    expect(r.detalle[0].base).toBe(900)
    expect(r.islrMonto).toBe(135)
  })

  it('lo acumulado en el ejercicio empuja el tramo', () => {
    expect(conAcumulado(1500).detalle[0].porcentaje).toBe(22) // 1.500 + 900 = 2.400
    expect(conAcumulado(2500).detalle[0].porcentaje).toBe(34) // 2.500 + 900 = 3.400
  })

  it('el pago de hoy cuenta para decidir el tramo, no solo lo de antes', () => {
    // 1.200 acumuladas está en el primer tramo, pero con los 900 de este pago
    // el total del año llega a 2.100 y ya es el segundo.
    expect(conAcumulado(1200).detalle[0].porcentaje).toBe(22)
  })

  it('un concepto de un solo tramo ignora el acumulado', () => {
    const r = proyectarRetenciones({
      empresa: AGENTE, valorUt: 1, conceptos: CONCEPTOS, acumulados: { honorarios: 999999 },
      perfil: { retieneIslr: true, islrConceptoCodigo: 'honorarios', islrSujeto: 'juridica_domiciliada' },
      ...ORDEN,
    })
    expect(r.islrMonto).toBe(50)
  })
})
