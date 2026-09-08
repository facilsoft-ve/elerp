// Pruebas de la lógica fiscal PURA del POS (src/lib/fiscal.js).
// Regla del proyecto: los importes de dinero se comparan con tolerancia
// (|a-b| < 0.005) porque son cálculos en punto flotante.
import { describe, it, expect } from 'vitest'
import {
  IVA_TASA,
  IGTF_TASA,
  calcularTotales,
  totalPagosEnBs,
  excedenteBs,
  vueltoDefaultMoneda,
  calcularVuelto,
} from '../fiscal.js'

// Aserción de dinero con tolerancia (evita ruido de coma flotante).
const money = (a, b) => expect(Math.abs(a - b) < 0.005).toBe(true)

describe('constantes fiscales', () => {
  it('IVA es 16% e IGTF es 3%', () => {
    expect(IVA_TASA).toBe(0.16)
    expect(IGTF_TASA).toBe(0.03)
  })
})

describe('calcularTotales — IVA sobre base gravada', () => {
  it('base 100 gravada → IVA 16, total 116 (sin pagos en divisa)', () => {
    const r = calcularTotales([{ cantidad: 1, precioUnitario: 100 }], [], 40)
    money(r.baseImponible, 100)
    money(r.baseExenta, 0)
    money(r.subtotal, 100)
    money(r.iva, 16)
    money(r.igtf, 0)
    money(r.total, 116)
  })

  it('cantidad × precio: 3 × 50 = 150 gravado → IVA 24, total 174', () => {
    const r = calcularTotales([{ cantidad: 3, precioUnitario: 50 }], [], 40)
    money(r.baseImponible, 150)
    money(r.iva, 24)
    money(r.total, 174)
  })

  it('línea exenta NO paga IVA; gravada sí (exento aparte)', () => {
    const r = calcularTotales(
      [
        { cantidad: 1, precioUnitario: 100 }, // gravado
        { cantidad: 1, precioUnitario: 50, exento: true }, // exento
      ],
      [],
      40,
    )
    money(r.baseImponible, 100)
    money(r.baseExenta, 50)
    money(r.subtotal, 150)
    money(r.iva, 16) // solo sobre los 100 gravados
    money(r.total, 166) // 150 + 16
  })

  it('todo exento → IVA 0', () => {
    const r = calcularTotales([{ cantidad: 2, precioUnitario: 25, exento: true }], [], 40)
    money(r.baseImponible, 0)
    money(r.baseExenta, 50)
    money(r.iva, 0)
    money(r.total, 50)
  })
})

describe('calcularTotales — IGTF 3% sobre la porción en divisas', () => {
  it('sin pagos en divisa → IGTF 0', () => {
    const r = calcularTotales([{ cantidad: 1, precioUnitario: 100 }], [{ monto: 116, moneda: 'VES' }], 40)
    money(r.igtf, 0)
    money(r.total, 116)
  })

  it('pago total en USD (tasa 40): $2.9 = 116 Bs → IGTF 3.48, total 119.48', () => {
    // docBase = 116; divisas entregadas = 2.9*40 = 116; IGTF = 116*0.03
    const r = calcularTotales(
      [{ cantidad: 1, precioUnitario: 100 }],
      [{ monto: 2.9, moneda: 'USD' }],
      40,
    )
    money(r.divisasEntregadasBs, 116)
    money(r.baseDivisasBs, 116)
    money(r.igtf, 3.48)
    money(r.total, 119.48)
  })

  it('pago parcial en divisa: $1 (40 Bs) grava solo esos 40 → IGTF 1.2', () => {
    const r = calcularTotales(
      [{ cantidad: 1, precioUnitario: 100 }],
      [
        { monto: 1, moneda: 'USD' }, // 40 Bs
        { monto: 76, moneda: 'VES' }, // resto en Bs
      ],
      40,
    )
    money(r.baseDivisasBs, 40)
    money(r.igtf, 1.2)
    money(r.total, 117.2)
  })

  it('IGTF se topa con (base+IVA): el vuelto de un billete grande no se grava', () => {
    // docBase = 116; entrega $5 = 200 Bs; base gravable por IGTF = min(200,116)=116
    const r = calcularTotales(
      [{ cantidad: 1, precioUnitario: 100 }],
      [{ monto: 5, moneda: 'USD' }],
      40,
    )
    money(r.divisasEntregadasBs, 200)
    money(r.baseDivisasBs, 116) // topado
    money(r.igtf, 3.48)
    money(r.total, 119.48)
  })

  it('resolutor multimoneda: cada divisa usa SU tasa (€ paga con tasa €)', () => {
    const tasaDe = (m) => ({ USD: 40, EUR: 45 }[m] || 0)
    const r = calcularTotales(
      [{ cantidad: 1, precioUnitario: 100 }],
      [{ monto: 1, moneda: 'EUR' }], // 45 Bs
      tasaDe,
    )
    money(r.baseDivisasBs, 45)
    money(r.igtf, 1.35) // 45 * 0.03
    money(r.total, 117.35)
  })
})

describe('calcularTotales — límites', () => {
  it('sin líneas ni pagos → todo en cero', () => {
    const r = calcularTotales([], [], 40)
    money(r.subtotal, 0)
    money(r.iva, 0)
    money(r.igtf, 0)
    money(r.total, 0)
  })

  it('argumentos nulos no rompen (defaults a listas vacías)', () => {
    const r = calcularTotales(null, null, 40)
    money(r.total, 0)
  })

  it('valores no numéricos se tratan como 0', () => {
    const r = calcularTotales([{ cantidad: 'x', precioUnitario: 'y' }], [], 40)
    money(r.total, 0)
  })
})

describe('totalPagosEnBs', () => {
  it('suma Bs directo y convierte divisa con la tasa (número)', () => {
    const r = totalPagosEnBs(
      [
        { monto: 100, moneda: 'VES' },
        { monto: 2, moneda: 'USD' }, // 80 Bs
      ],
      40,
    )
    money(r, 180)
  })

  it('resolutor multimoneda: USD y EUR con tasas distintas', () => {
    const tasaDe = (m) => ({ USD: 40, EUR: 45 }[m] || 0)
    const r = totalPagosEnBs(
      [
        { monto: 1, moneda: 'USD' }, // 40
        { monto: 1, moneda: 'EUR' }, // 45
      ],
      tasaDe,
    )
    money(r, 85)
  })

  it('solo VES → suma cruda', () => {
    money(totalPagosEnBs([{ monto: 50, moneda: 'VES' }, { monto: 66, moneda: 'VES' }], 40), 116)
  })

  it('lista vacía / nula → 0', () => {
    money(totalPagosEnBs([], 40), 0)
    money(totalPagosEnBs(null, 40), 0)
  })
})

describe('excedenteBs (vuelto en Bs)', () => {
  it('sobrepago → excedente positivo', () => {
    // paga 200 Bs, total 116 → 84
    money(excedenteBs([{ monto: 200, moneda: 'VES' }], 116, 40), 84)
  })

  it('pago justo → 0', () => {
    money(excedenteBs([{ monto: 116, moneda: 'VES' }], 116, 40), 0)
  })

  it('pago insuficiente → 0 (nunca negativo)', () => {
    money(excedenteBs([{ monto: 50, moneda: 'VES' }], 116, 40), 0)
  })

  it('excedente de un pago en divisa (convertido)', () => {
    // $5 = 200 Bs, total 116 → 84 Bs
    money(excedenteBs([{ monto: 5, moneda: 'USD' }], 116, 40), 84)
  })
})

describe('vueltoDefaultMoneda', () => {
  it('último efectivo en divisa define la moneda del vuelto', () => {
    expect(vueltoDefaultMoneda([{ metodo: 'efectivo_usd', moneda: 'USD', monto: 5 }])).toBe('USD')
  })

  it('gana el ÚLTIMO efectivo en divisa entregado', () => {
    expect(
      vueltoDefaultMoneda([
        { metodo: 'efectivo_usd', moneda: 'USD', monto: 5 },
        { metodo: 'efectivo_eur', moneda: 'EUR', monto: 2 },
      ]),
    ).toBe('EUR')
  })

  it('un pago NO efectivo en divisa (zelle) NO cuenta → VES', () => {
    expect(vueltoDefaultMoneda([{ metodo: 'zelle', moneda: 'USD', monto: 5 }])).toBe('VES')
  })

  it('efectivo en divisa con monto 0 no cuenta → VES', () => {
    expect(vueltoDefaultMoneda([{ metodo: 'efectivo_usd', moneda: 'USD', monto: 0 }])).toBe('VES')
  })

  it('sin efectivo en divisa → VES', () => {
    expect(vueltoDefaultMoneda([{ metodo: 'efectivo_bs', moneda: 'VES', monto: 100 }])).toBe('VES')
    expect(vueltoDefaultMoneda([])).toBe('VES')
    expect(vueltoDefaultMoneda(null)).toBe('VES')
  })
})

describe('calcularVuelto (derivación automática)', () => {
  it('sin excedente relevante (≤ 0.004) → null', () => {
    expect(calcularVuelto([{ monto: 116, moneda: 'VES' }], 116, 40)).toBe(null)
  })

  it('efectivo en USD: devuelve el vuelto en USD (billete recibido)', () => {
    // $5 = 200 Bs, total 116 → excedente 84 Bs → 84/40 = 2.1 USD
    const v = calcularVuelto([{ metodo: 'efectivo_usd', monto: 5, moneda: 'USD' }], 116, 40)
    expect(v.moneda).toBe('USD')
    money(v.monto, 2.1)
  })

  it('pago en Bs: vuelto en Bs', () => {
    const v = calcularVuelto([{ metodo: 'efectivo_bs', monto: 200, moneda: 'VES' }], 116, 40)
    expect(v.moneda).toBe('VES')
    money(v.monto, 84)
  })
})
