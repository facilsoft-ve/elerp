import { describe, it, expect } from 'vitest'
import { calcularAjuste, cuerpoAjuste, ajusteVacio } from '../nota.js'

/* PREVISUALIZACIÓN DEL % EN NOTAS DE CRÉDITO Y DÉBITO.
 *
 * Esta cuenta se ve en pantalla ANTES de emitir un documento que ya no se puede
 * editar: es la última oportunidad de detectar un error. Tiene que dar lo mismo
 * que el servidor (`repartirBase` en el backend); si se desvían, la pantalla
 * miente justo en el momento en que más se la mira. */

// Factura mixta: 200 gravados + 100 exentos, IVA histórico del 16 %.
const facturaMixta = {
  baseImponible: 200, baseExenta: 100, alicuotaIVA: 0.16,
  lineas: [
    { sku: 'A', nombre: 'Gravado', total: 200, precioUnitario: 200, cantidad: 1 },
    { sku: 'B', nombre: 'Exento', total: 100, precioUnitario: 100, cantidad: 1, exento: true },
  ],
}

const pct = (p, extra = {}) => ({ ...ajusteVacio(), modo: 'porcentaje', porcentaje: String(p), ...extra })

describe('calcularAjuste — porcentaje sobre toda la factura', () => {
  it('reparte en proporción entre base gravada y exenta', () => {
    const r = calcularAjuste(facturaMixta, pct(10))
    expect(r.gravado).toBeCloseTo(20, 6)
    expect(r.exento).toBeCloseTo(10, 6)
    expect(r.error).toBe('')
  })

  it('grava con IVA solo la parte gravada', () => {
    const r = calcularAjuste(facturaMixta, pct(10))
    expect(r.iva).toBeCloseTo(20 * 0.16, 6)
    expect(r.total).toBeCloseTo(30 + 3.2, 6)
  })

  // LA propiedad que justifica aplicar el % sobre la base y no sobre el total:
  // el resultado es el mismo % del total, sin descontar el IVA dos veces.
  it('equivale al mismo porcentaje del total con IVA', () => {
    const totalFactura = 200 + 100 + 200 * 0.16
    const r = calcularAjuste(facturaMixta, pct(10))
    expect(r.total).toBeCloseTo(totalFactura * 0.10, 6)
  })

  it('usa la alícuota histórica del documento, no la de hoy', () => {
    const vieja = { ...facturaMixta, alicuotaIVA: 0.12 }
    expect(calcularAjuste(vieja, pct(10)).iva).toBeCloseTo(20 * 0.12, 6)
  })
})

describe('calcularAjuste — porcentaje sobre un producto', () => {
  it('se acota al renglón elegido, no a la factura', () => {
    const r = calcularAjuste(facturaMixta, pct(50, { alcance: 'producto', sku: 'A' }))
    expect(r.gravado).toBeCloseTo(100, 6)
    expect(r.exento).toBe(0)
  })

  it('hereda la condición de IVA del renglón en vez de preguntarla', () => {
    const r = calcularAjuste(facturaMixta, pct(50, { alcance: 'producto', sku: 'B' }))
    expect(r.exento).toBeCloseTo(50, 6)
    expect(r.gravado).toBe(0)
    expect(r.iva).toBe(0)
  })

  it('pide elegir el producto antes de calcular nada', () => {
    const r = calcularAjuste(facturaMixta, pct(50, { alcance: 'producto', sku: '' }))
    expect(r.total).toBe(0)
    expect(r.error).toMatch(/producto/i)
  })
})

describe('calcularAjuste — validación', () => {
  it('rechaza porcentaje cero o mayor que 100', () => {
    expect(calcularAjuste(facturaMixta, pct(0)).error).toBeTruthy()
    expect(calcularAjuste(facturaMixta, pct(150)).error).toMatch(/100/)
  })

  it('rechaza un monto que no es un número positivo', () => {
    expect(calcularAjuste(facturaMixta, { ...ajusteVacio(), monto: '' }).error).toBeTruthy()
    expect(calcularAjuste(facturaMixta, { ...ajusteVacio(), monto: '-5' }).error).toBeTruthy()
  })

  // El tope es de la NOTA DE CRÉDITO: no se acredita más de lo que se cobró.
  it('con tope, no deja pasar de lo facturado en ese producto', () => {
    const aj = { ...ajusteVacio(), alcance: 'producto', sku: 'A', monto: '250' }
    expect(calcularAjuste(facturaMixta, aj, { tope: true }).error).toBeTruthy()
    // Justo en el tope sí pasa.
    expect(calcularAjuste(facturaMixta, { ...aj, monto: '200' }, { tope: true }).error).toBe('')
  })

  // La nota de DÉBITO no tiene tope: una mora sí puede superar lo que la originó.
  it('sin tope, un cargo puede superar el renglón', () => {
    const aj = { ...ajusteVacio(), alcance: 'producto', sku: 'A', monto: '250' }
    expect(calcularAjuste(facturaMixta, aj).error).toBe('')
  })
})

describe('cuerpoAjuste', () => {
  it('manda porcentaje o monto, nunca los dos', () => {
    const p = cuerpoAjuste(pct(10))
    expect(p.porcentaje).toBe(10)
    expect(p.monto).toBeUndefined()

    const m = cuerpoAjuste({ ...ajusteVacio(), monto: '25' })
    expect(m.monto).toBe(25)
    expect(m.porcentaje).toBeUndefined()
  })

  it('incluye el sku solo cuando el ajuste es sobre un producto', () => {
    expect(cuerpoAjuste(pct(10)).sku).toBeUndefined()
    expect(cuerpoAjuste(pct(10, { alcance: 'producto', sku: 'A' })).sku).toBe('A')
  })

  // La condición de IVA solo se declara donde no hay de dónde deducirla; sobre un
  // producto la manda el renglón, y enviarla desde acá podría contradecir la
  // factura.
  it('no declara exento cuando el ajuste va sobre un producto', () => {
    const b = cuerpoAjuste({ ...ajusteVacio(), alcance: 'producto', sku: 'A', monto: '10', exento: true })
    expect(b.exento).toBeUndefined()
  })
})
