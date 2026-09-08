// Pruebas de la lógica PURA de precios / multimoneda (src/lib/precio.js).
import { describe, it, expect } from 'vitest'
import {
  resolveRate,
  monedaDe,
  monedaLabel,
  itemDeLista,
  precioListaEnBs,
  porCodigo,
} from '../precio.js'

const money = (a, b) => expect(Math.abs(a - b) < 0.005).toBe(true)

describe('resolveRate', () => {
  it('VES siempre vale 1 (aunque la tasa sea otra cosa)', () => {
    expect(resolveRate(40, 'VES')).toBe(1)
    expect(resolveRate((m) => 999, 'VES')).toBe(1)
  })

  it('tasa como NÚMERO → esa tasa para la divisa', () => {
    expect(resolveRate(40, 'USD')).toBe(40)
  })

  it('tasa como FUNCIÓN resolutora → tasa por moneda', () => {
    const tasaDe = (m) => ({ USD: 40, EUR: 45 }[m] || 0)
    expect(resolveRate(tasaDe, 'USD')).toBe(40)
    expect(resolveRate(tasaDe, 'EUR')).toBe(45)
    expect(resolveRate(tasaDe, 'COP')).toBe(0)
  })

  it('tasa inválida / ausente → 0 (nunca inventa un 1)', () => {
    expect(resolveRate(undefined, 'USD')).toBe(0)
    expect(resolveRate('abc', 'USD')).toBe(0)
  })
})

describe('monedaDe', () => {
  it('usa la moneda del producto (en mayúsculas)', () => {
    expect(monedaDe({ moneda: 'usd' }, 'VES')).toBe('USD')
  })
  it('sin moneda propia → la de la empresa', () => {
    expect(monedaDe({}, 'EUR')).toBe('EUR')
  })
  it('sin nada → VES', () => {
    expect(monedaDe(null)).toBe('VES')
    expect(monedaDe({})).toBe('VES')
  })
})

describe('monedaLabel', () => {
  it('rótulos conocidos', () => {
    expect(monedaLabel('VES')).toBe('Bs')
    expect(monedaLabel('USD')).toBe('US$')
    expect(monedaLabel('EUR')).toBe('€')
    expect(monedaLabel('COP')).toBe('COP')
    expect(monedaLabel('USDT')).toBe('USDT')
  })
  it('acepta minúsculas', () => {
    expect(monedaLabel('usd')).toBe('US$')
  })
  it('moneda desconocida → su código en mayúsculas', () => {
    expect(monedaLabel('gbp')).toBe('GBP')
  })
})

describe('itemDeLista', () => {
  const lista = { moneda: 'VES', items: [{ sku: 'ABC', precio: 8 }, { sku: 'XYZ', precio: 3 }] }
  it('encuentra el SKU (sin distinguir mayúsculas)', () => {
    expect(itemDeLista(lista, 'abc')).toEqual({ sku: 'ABC', precio: 8 })
  })
  it('SKU no listado → null', () => {
    expect(itemDeLista(lista, 'nope')).toBe(null)
  })
  it('lista nula o sin items → null', () => {
    expect(itemDeLista(null, 'abc')).toBe(null)
    expect(itemDeLista({}, 'abc')).toBe(null)
  })
})

describe('precioListaEnBs', () => {
  const prod = { sku: 'ABC', precio: 10, moneda: 'VES' }

  it('lista en VES con precio explícito → ese precio', () => {
    const lista = { moneda: 'VES', items: [{ sku: 'ABC', precio: 8 }] }
    money(precioListaEnBs(prod, lista, 'VES', 40), 8)
  })

  it('lista en divisa (USD) convierte a Bs con la tasa', () => {
    const lista = { moneda: 'USD', items: [{ sku: 'ABC', precio: 2 }] }
    money(precioListaEnBs(prod, lista, 'VES', 40), 80) // 2 * 40
  })

  it('lista en divisa SIN tasa → null (no se inventa un 1)', () => {
    const lista = { moneda: 'USD', items: [{ sku: 'ABC', precio: 2 }] }
    expect(precioListaEnBs(prod, lista, 'VES', 0)).toBe(null)
  })

  it('SKU no listado → cae al precio base del catálogo', () => {
    const lista = { moneda: 'USD', items: [{ sku: 'OTRO', precio: 99 }] }
    money(precioListaEnBs(prod, lista, 'VES', 40), 10) // precio base VES
  })

  it('sin lista → precio base', () => {
    money(precioListaEnBs(prod, null, 'VES', 40), 10)
  })

  it('producto en divisa sin lista → convierte base con la tasa', () => {
    const prodUsd = { sku: 'D', precio: 2, moneda: 'USD' }
    money(precioListaEnBs(prodUsd, null, 'VES', 40), 80)
  })

  it('producto en divisa sin lista y sin tasa → null', () => {
    const prodUsd = { sku: 'D', precio: 2, moneda: 'USD' }
    expect(precioListaEnBs(prodUsd, null, 'VES', 0)).toBe(null)
  })

  it('resolutor multimoneda: lista en EUR usa tasa EUR', () => {
    const tasaDe = (m) => ({ USD: 40, EUR: 45 }[m] || 0)
    const lista = { moneda: 'EUR', items: [{ sku: 'ABC', precio: 2 }] }
    money(precioListaEnBs(prod, lista, 'VES', tasaDe), 90) // 2 * 45
  })
})

describe('porCodigo', () => {
  const productos = [
    { sku: 'SKU-1', codigoBarras: '7591234567890', presentaciones: [{ codigoBarras: '111' }, { codigoBarras: '222' }] },
    { sku: 'SKU-2', codigoBarras: '7599999999999', presentaciones: [] },
  ]

  it('encuentra por código de barras exacto', () => {
    expect(porCodigo(productos, '7591234567890')).toBe(productos[0])
  })
  it('encuentra por SKU', () => {
    expect(porCodigo(productos, 'sku-2')).toBe(productos[1])
  })
  it('encuentra por código de barras de una presentación', () => {
    expect(porCodigo(productos, '222')).toBe(productos[0])
  })
  it('ignora mayúsculas y espacios de la pistola (trailing space/newline)', () => {
    expect(porCodigo(productos, '  SKU-1\n')).toBe(productos[0])
  })
  it('término vacío → null', () => {
    expect(porCodigo(productos, '   ')).toBe(null)
    expect(porCodigo(productos, '')).toBe(null)
  })
  it('sin coincidencia → null', () => {
    expect(porCodigo(productos, '000')).toBe(null)
  })
})
