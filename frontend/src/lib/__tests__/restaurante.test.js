// Pruebas de las métricas del salón (src/lib/restaurante.js).
//
// Los KPIs de un tablero son justo donde un error pasa desapercibido: un número
// equivocado se ve igual de convincente que el correcto. Por eso el cálculo vive fuera de
// la pantalla y se prueba acá.
import { describe, it, expect } from 'vitest'
import { metricasRestaurante, totalDeCuenta, esperaMinutos } from '../restaurante.js'

const AHORA = new Date('2026-09-08T20:00:00Z').getTime()
const haceMin = (m) => new Date(AHORA - m * 60000).toISOString()

const mesa = (id, over = {}) => ({ id, nombre: id, activa: true, capacidad: 4, ...over })
const item = (over = {}) => ({ sku: 'PLA-1', nombre: 'Plato', cantidad: 1, precioUnitario: 10000, estado: 'pendiente', ...over })

// Factura de HOY en hora local (el módulo agrupa por día local, no UTC).
const hoyLocal = (h = 13) => { const d = new Date(); d.setHours(h, 0, 0, 0); return d.toISOString() }
const factura = (over = {}) => ({ tipo: 'factura', fecha: hoyLocal(), total: 50000, lineas: [], ...over })

describe('totalDeCuenta', () => {
  it('suma los renglones y NO cuenta los cancelados', () => {
    const c = { items: [item({ cantidad: 2 }), item({ estado: 'cancelado', cantidad: 5 })] }
    expect(totalDeCuenta(c)).toBe(20000)
  })
  it('tolera una cuenta vacía o ausente', () => {
    expect(totalDeCuenta({ items: [] })).toBe(0)
    expect(totalDeCuenta(null)).toBe(0)
  })
})

describe('esperaMinutos', () => {
  it('mide desde que el renglón se envió a cocina', () => {
    expect(esperaMinutos(item({ enviadoEn: haceMin(12) }), AHORA)).toBe(12)
  })
  it('un renglón sin enviar no tiene espera', () => {
    expect(esperaMinutos(item(), AHORA)).toBe(0)
    expect(esperaMinutos(item({ enviadoEn: 'no-es-fecha' }), AHORA)).toBe(0)
  })
  it('nunca devuelve negativo (un reloj adelantado no debe dar -5 min)', () => {
    expect(esperaMinutos(item({ enviadoEn: new Date(AHORA + 600000).toISOString() }), AHORA)).toBe(0)
  })
})

describe('metricasRestaurante · estado del salón', () => {
  it('clasifica libre / ocupada / por cobrar, y por cobrar es la que ya pidió la cuenta', () => {
    const mesas = [mesa('m1'), mesa('m2'), mesa('m3'), mesa('m4')]
    const cuentas = [
      { mesaId: 'm1', comensales: 2, items: [item()] },                          // ocupada
      { mesaId: 'm2', comensales: 4, items: [item()], prefacturas: ['cot_1'] },   // por cobrar
    ]
    const m = metricasRestaurante({ mesas, cuentas }, { ahora: AHORA })
    expect(m.mesasTotal).toBe(4)
    expect(m.ocupadas).toBe(1)
    expect(m.porCobrar).toBe(1)
    expect(m.libres).toBe(2)
    // Las tres categorías tienen que sumar el total: si no, el tablero miente.
    expect(m.libres + m.ocupadas + m.porCobrar).toBe(m.mesasTotal)
  })

  it('no cuenta las mesas INACTIVAS (una mesa guardada pero fuera de servicio)', () => {
    const m = metricasRestaurante({ mesas: [mesa('m1'), mesa('m2', { activa: false })] }, { ahora: AHORA })
    expect(m.mesasTotal).toBe(1)
    expect(m.libres).toBe(1)
  })

  it('suma comensales sentados y el consumo EN CURSO (lo que hay sin cobrar)', () => {
    const cuentas = [
      { mesaId: 'm1', comensales: 3, items: [item({ cantidad: 2 })] },
      { mesaId: 'm2', comensales: 2, items: [item({ cantidad: 1 }), item({ estado: 'cancelado', cantidad: 9 })] },
    ]
    const m = metricasRestaurante({ mesas: [mesa('m1'), mesa('m2')], cuentas }, { ahora: AHORA })
    expect(m.comensales).toBe(5)
    expect(m.consumoEnCurso).toBe(30000) // 2×10000 + 1×10000, sin el cancelado
  })

  it('ignora una cuenta cuya mesa no existe (dato huérfano)', () => {
    const m = metricasRestaurante({ mesas: [mesa('m1')], cuentas: [{ mesaId: 'fantasma', comensales: 9, items: [item()] }] }, { ahora: AHORA })
    expect(m.ocupadas).toBe(0)
    expect(m.comensales).toBe(0)
  })
})

describe('metricasRestaurante · cocina', () => {
  it('cuenta lo enviado y no servido, y la espera MÁS LARGA', () => {
    const cuentas = [{
      mesaId: 'm1', comensales: 2, items: [
        item({ estado: 'en_cocina', enviadoEn: haceMin(18) }),
        item({ estado: 'listo', enviadoEn: haceMin(4) }),
        item({ estado: 'servido', enviadoEn: haceMin(40) }),   // ya salió: no cuenta
        item({ estado: 'pendiente' }),                          // no se envió: no cuenta
      ],
    }]
    const m = metricasRestaurante({ mesas: [mesa('m1')], cuentas }, { ahora: AHORA })
    expect(m.enCocina).toBe(2)
    expect(m.listosParaLlevar).toBe(1)
    expect(m.esperaMaxima).toBe(18) // la más larga, no el promedio: es lo que urge
  })
})

describe('metricasRestaurante · lo vendido hoy', () => {
  it('ticket promedio y rotación de mesas', () => {
    const docs = [factura({ total: 30000 }), factura({ total: 50000 })]
    const m = metricasRestaurante({ mesas: [mesa('m1'), mesa('m2'), mesa('m3'), mesa('m4')], documentos: docs }, { ahora: AHORA })
    expect(m.ventasHoy).toBe(80000)
    expect(m.mesasCobradasHoy).toBe(2)
    expect(m.ticketPromedio).toBe(40000)
    expect(m.rotacion).toBe(0.5) // 2 cobradas / 4 mesas
  })

  it('una factura ANULADA no cuenta como venta', () => {
    const m = metricasRestaurante({ mesas: [mesa('m1')], documentos: [factura({ anulado: true })] }, { ahora: AHORA })
    expect(m.ventasHoy).toBe(0)
    expect(m.mesasCobradasHoy).toBe(0)
  })

  it('no divide por cero cuando todavía no hubo servicio', () => {
    const m = metricasRestaurante({ mesas: [] }, { ahora: AHORA })
    expect(m.ticketPromedio).toBe(0)
    expect(m.rotacion).toBe(0)
    expect(m.hayDatos).toBe(false)
  })

  it('platos más vendidos: solo PLATOS, ordenados y agrupados por SKU', () => {
    const productos = [
      { sku: 'PLA-A', esPlato: true }, { sku: 'PLA-B', esPlato: true },
      { sku: 'BEB-1' },  // bebida: no es plato
    ]
    const docs = [
      factura({ lineas: [{ sku: 'PLA-A', nombre: 'Pasta', cantidad: 2, total: 20000 }, { sku: 'BEB-1', nombre: 'Agua', cantidad: 9, total: 900 }] }),
      factura({ lineas: [{ sku: 'PLA-A', nombre: 'Pasta', cantidad: 1, total: 10000 }, { sku: 'PLA-B', nombre: 'Milanesa', cantidad: 4, total: 60000 }] }),
    ]
    const m = metricasRestaurante({ mesas: [mesa('m1')], documentos: docs, productos }, { ahora: AHORA })
    expect(m.platosTop.map((p) => p.sku)).toEqual(['PLA-B', 'PLA-A'])   // 4 > 3
    expect(m.platosTop[1].cantidad).toBe(3)                              // 2 + 1 agrupados
    expect(m.platosTop.some((p) => p.sku === 'BEB-1')).toBe(false)       // la bebida no
  })

  it('hayDatos distingue «sin servicio» de «servicio en cero»', () => {
    expect(metricasRestaurante({ mesas: [mesa('m1')] }, { ahora: AHORA }).hayDatos).toBe(false)
    const conMesa = metricasRestaurante({ mesas: [mesa('m1')], cuentas: [{ mesaId: 'm1', items: [] }] }, { ahora: AHORA })
    expect(conMesa.hayDatos).toBe(true)
  })

  it('tolera que no venga nada', () => {
    const m = metricasRestaurante(undefined, { ahora: AHORA })
    expect(m.mesasTotal).toBe(0)
    expect(m.platosTop).toEqual([])
  })
})
