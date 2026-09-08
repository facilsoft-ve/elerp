// Pruebas de la lógica PURA de qué se puede vender (src/lib/catalogo.js).
//
// Por qué importa tanto un filtro de una línea: si se invierte la condición, los INSUMOS
// vuelven al menú del mesonero y al POS. Fue el bug real que dio origen a este módulo (la
// comandera ofrecía «Pasta larga (spaghetti)» por kg), y nada avisaría si vuelve.
import { describe, it, expect } from 'vitest'
import { vendibles, esVendible } from '../catalogo.js'

const plato = { sku: 'PLA-BOLONESA', nombre: 'Spaghetti a la boloñesa', esPlato: true, precio: 32000 }
const bebida = { sku: 'BEB-AGUA', nombre: 'Agua mineral', precio: 1600 }
const insumo = { sku: 'INS-PASTA', nombre: 'Pasta larga', esInsumo: true, precio: 0 }
const inactivo = { sku: 'VIEJO-1', nombre: 'Descontinuado', activo: false, precio: 100 }
const combo = { sku: 'COMBO-1', nombre: 'Combo desayuno', esCombo: true, precio: 5000 }

describe('vendibles', () => {
  it('deja los platos, las bebidas y la reventa: es lo que vende un restaurante', () => {
    const out = vendibles([plato, bebida, combo])
    expect(out.map((p) => p.sku)).toEqual(['PLA-BOLONESA', 'BEB-AGUA', 'COMBO-1'])
  })

  it('quita los INSUMOS: la materia prima no se vende directamente', () => {
    const out = vendibles([plato, insumo, bebida])
    expect(out.map((p) => p.sku)).toEqual(['PLA-BOLONESA', 'BEB-AGUA'])
  })

  it('quita los inactivos por defecto', () => {
    expect(vendibles([bebida, inactivo]).map((p) => p.sku)).toEqual(['BEB-AGUA'])
  })

  it('con soloActivos:false conserva los inactivos, pero NUNCA los insumos', () => {
    // El insumo no es una cuestión de estado: no se vende ni estando activo.
    const out = vendibles([bebida, inactivo, insumo], { soloActivos: false })
    expect(out.map((p) => p.sku)).toEqual(['BEB-AGUA', 'VIEJO-1'])
  })

  it('un producto sin la bandera se considera vendible (retrocompatibilidad)', () => {
    // Los productos creados antes de que existiera `esInsumo` no traen el campo: leerlos
    // como insumos los sacaría del POS de golpe en todas las empresas.
    expect(vendibles([{ sku: 'ANTIGUO' }]).map((p) => p.sku)).toEqual(['ANTIGUO'])
  })

  it('tolera null, undefined y lista vacía sin explotar', () => {
    // Se llama con `db.PRODUCTOS` recién montado, que puede no haber cargado todavía.
    expect(vendibles(null)).toEqual([])
    expect(vendibles(undefined)).toEqual([])
    expect(vendibles([])).toEqual([])
  })

  it('no muta ni reordena la lista original', () => {
    const original = [plato, insumo, bebida]
    const copia = [...original]
    vendibles(original)
    expect(original).toEqual(copia)
  })
})

describe('esVendible', () => {
  it('acepta plato, bebida y combo', () => {
    expect(esVendible(plato)).toBe(true)
    expect(esVendible(bebida)).toBe(true)
    expect(esVendible(combo)).toBe(true)
  })

  it('rechaza insumo e inactivo', () => {
    expect(esVendible(insumo)).toBe(false)
    expect(esVendible(inactivo)).toBe(false)
  })

  it('rechaza null/undefined sin lanzar', () => {
    // Lo llama el POS al resolver un SKU escaneado que puede no existir.
    expect(esVendible(null)).toBe(false)
    expect(esVendible(undefined)).toBe(false)
  })

  it('es coherente con vendibles(): mismo criterio en los dos caminos', () => {
    const todos = [plato, bebida, insumo, inactivo, combo]
    expect(vendibles(todos).map((p) => p.sku)).toEqual(todos.filter(esVendible).map((p) => p.sku))
  })
})
