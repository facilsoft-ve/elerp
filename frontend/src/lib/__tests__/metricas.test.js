// Pruebas de la lógica PURA de las métricas del Inicio (src/lib/metricas.js).
//
// De acá salen todos los KPIs y los avisos de «Pendientes de hoy». Es el módulo donde
// apareció el aviso fantasma de «6 productos agotados» (contaba platos, que no se
// stockean), así que conviene fijar por escrito qué cuenta y qué no.
import { describe, it, expect } from 'vitest'
import {
  diaLocal, hoy, mesActual,
  facturasVigentes, reversas, sinSincronizar,
  serieVentas, metricas, alertas,
} from '../metricas.js'

// --- helpers ---------------------------------------------------------------

// Fecha LOCAL a las 21:00: en Venezuela (UTC−4) eso cae al día siguiente en UTC, que es
// justo el error que `diaLocal` existe para evitar.
const hoyALas = (h, m = 0) => {
  const d = new Date()
  d.setHours(h, m, 0, 0)
  return d.toISOString()
}
const hace = (dias) => {
  const d = new Date()
  d.setDate(d.getDate() - dias)
  d.setHours(12, 0, 0, 0)
  return d.toISOString()
}

const factura = (over = {}) => ({
  tipo: 'factura', fecha: hoyALas(12), total: 1000, iva: 160, igtf: 0,
  actor: 'usr_a', pagos: [], ...over,
})

describe('diaLocal', () => {
  it('agrupa por día LOCAL, no por UTC', () => {
    // Una venta de las 21:00 local no debe atribuirse al día siguiente.
    expect(diaLocal(hoyALas(21))).toBe(hoy())
    expect(diaLocal(hoyALas(0, 30))).toBe(hoy())
  })

  it('devuelve cadena vacía ante una fecha inválida, sin lanzar', () => {
    expect(diaLocal('no-es-fecha')).toBe('')
    expect(diaLocal('')).toBe('')
    expect(diaLocal(undefined)).toBe('')
  })

  it('mesActual es el prefijo AAAA-MM de hoy', () => {
    expect(mesActual()).toBe(hoy().slice(0, 7))
    expect(mesActual()).toMatch(/^\d{4}-\d{2}$/)
  })
})

describe('clasificación de documentos', () => {
  const docs = [
    factura(),
    factura({ anulado: true }),
    { tipo: 'anulacion', fecha: hoyALas(13), total: 1000 },
    { tipo: 'nota_credito', fecha: hoyALas(14), total: 300 },
    factura({ contingencia: true }),
  ]

  it('una factura ANULADA no cuenta como venta', () => {
    // `anulado` lo deriva el backend de que exista su reversa; contarla infla las ventas.
    expect(facturasVigentes(docs)).toHaveLength(2)
    expect(facturasVigentes(docs).every((d) => !d.anulado)).toBe(true)
  })

  it('las reversas son anulaciones y notas de crédito, no facturas', () => {
    expect(reversas(docs).map((d) => d.tipo).sort()).toEqual(['anulacion', 'nota_credito'])
  })

  it('las de contingencia se listan aparte (se emitieron sin conexión)', () => {
    expect(sinSincronizar(docs)).toHaveLength(1)
  })

  it('tolera null/undefined', () => {
    expect(facturasVigentes(null)).toEqual([])
    expect(reversas(undefined)).toEqual([])
    expect(sinSincronizar(null)).toEqual([])
  })
})

describe('serieVentas', () => {
  it('devuelve SIEMPRE la ventana completa, con ceros donde no hubo ventas', () => {
    // Si el eje temporal se saltara los días vacíos, el gráfico mentiría sobre la forma
    // de la venta (dos picos seguidos parecerían días consecutivos).
    const { puntos } = serieVentas([factura()], 14)
    expect(puntos).toHaveLength(14)
    expect(puntos.filter((p) => p.v === 0)).toHaveLength(13)
  })

  it('hayDatos distingue «sin ventas» de «ventas en cero»', () => {
    expect(serieVentas([], 7).hayDatos).toBe(false)
    expect(serieVentas([factura()], 7).hayDatos).toBe(true)
  })

  it('ignora las ventas fuera de la ventana', () => {
    const { puntos, hayDatos } = serieVentas([factura({ fecha: hace(60) })], 7)
    expect(hayDatos).toBe(false)
    expect(puntos.every((p) => p.v === 0)).toBe(true)
  })

  it('suma varias ventas del MISMO día en un solo punto', () => {
    const { puntos } = serieVentas([factura({ total: 100 }), factura({ total: 250 })], 7)
    expect(puntos[puntos.length - 1].v).toBe(350)
  })

  it('no cuenta las anuladas', () => {
    const { puntos, hayDatos } = serieVentas([factura({ total: 500, anulado: true })], 7)
    expect(hayDatos).toBe(false)
    expect(puntos[puntos.length - 1].v).toBe(0)
  })
})

describe('metricas: existencias', () => {
  const db = (existencias) => ({ DOCUMENTOS: [], EXISTENCIAS: existencias, TRANSFERENCIAS: [], PRODUCTOS: [] })

  it('agotado es cantidad <= 0; bajo mínimo incluye el umbral', () => {
    const mt = metricas(db([
      { sku: 'A', nombre: 'Agotado', cantidad: 0 },
      { sku: 'B', nombre: 'Negativo', cantidad: -2 },
      { sku: 'C', nombre: 'Justo en el umbral', cantidad: 5 },
      { sku: 'D', nombre: 'Sobrado', cantidad: 50 },
    ]), { umbralStockBajo: 5 })
    expect(mt.sinStock.map((e) => e.sku)).toEqual(['A', 'B'])
    // stockBajo es «<= umbral», así que arrastra también a los agotados.
    expect(mt.stockBajo.map((e) => e.sku)).toEqual(['A', 'B', 'C'])
  })

  it('cuenta lo que RECIBE: los no stockeables se excluyen antes, en el servidor', () => {
    // La proyección de existencias del backend ya excluye combos y PLATOS (un plato no
    // tiene stock propio: se prepara al momento). Si algún día volvieran a colarse, todo
    // el menú aparecería «agotado» — que es exactamente el bug que hubo.
    const mt = metricas(db([{ sku: 'INS-PASTA', nombre: 'Pasta', cantidad: 40 }]))
    expect(mt.sinStock).toHaveLength(0)
  })
})

describe('alertas', () => {
  const base = {
    pendientesSync: [], sinStock: [], stockBajo: [], transEnCurso: [], umbralStockBajo: 5,
  }

  it('el CAJERO no ve avisos de inventario (no es su trabajo reponer)', () => {
    const mt = { ...base, sinStock: [{ sku: 'A', nombre: 'Harina' }] }
    expect(alertas(mt, { rol: 'cajero' }).map((a) => a.id)).toEqual([])
    expect(alertas(mt, { rol: 'dueno' }).map((a) => a.id)).toEqual(['sinstock'])
  })

  it('la contingencia se avisa a CUALQUIER rol: es una obligación fiscal', () => {
    const mt = { ...base, pendientesSync: [{ id: 'd1' }] }
    for (const rol of ['dueno', 'cajero', 'vendedor', 'contadora', 'mesonero']) {
      expect(alertas(mt, { rol }).map((a) => a.id)).toContain('sync')
    }
  })

  it('«bajo mínimo» excluye los agotados: si no, el mismo producto sale en dos avisos', () => {
    const mt = {
      ...base,
      sinStock: [{ sku: 'A', nombre: 'Agotado' }],
      stockBajo: [{ sku: 'A', nombre: 'Agotado', cantidad: 0 }, { sku: 'C', nombre: 'Poco', cantidad: 3 }],
    }
    const av = alertas(mt, { rol: 'dueno' })
    expect(av.find((a) => a.id === 'stockbajo').titulo).toMatch(/^1 producto bajo mínimo$/)
  })

  it('nombra el detalle concreto: un pendiente sin sujeto no es accionable', () => {
    const mt = { ...base, sinStock: [{ sku: 'A', nombre: 'Harina' }, { sku: 'B', nombre: 'Arroz' }] }
    const av = alertas(mt, { rol: 'dueno' })[0]
    expect(av.detalle).toBe('Harina, Arroz')
    expect(av.titulo).toBe('2 productos agotados')
  })

  it('singular y plural bien conjugados', () => {
    const uno = alertas({ ...base, sinStock: [{ sku: 'A', nombre: 'Harina' }] }, { rol: 'dueno' })[0]
    expect(uno.titulo).toBe('1 producto agotado')
  })

  it('sin nada pendiente no inventa avisos', () => {
    expect(alertas(base, { rol: 'dueno' })).toEqual([])
  })

  it('cada aviso trae ruta para poder actuar', () => {
    const mt = { ...base, pendientesSync: [{ id: 'd1' }], sinStock: [{ sku: 'A', nombre: 'X' }], transEnCurso: [{ id: 't1' }] }
    for (const a of alertas(mt, { rol: 'dueno' })) {
      expect(a.ruta).toBeTruthy()
      expect(a.titulo).toBeTruthy()
    }
  })
})
