import { describe, it, expect } from 'vitest'
import { accionesDe, agruparBandeja, estaDemorado, minutosRestantes, esFinal, etiquetaOrigen } from '../pedidos.js'

/* LO QUE SE PUEDE HACER CON UN PEDIDO.
 *
 * Esta tabla es el espejo de las transiciones del servidor. Si las dos se
 * separan, la pantalla ofrece botones que fallan — y lo hace en hora pico, que
 * es cuando el módulo se usa. */

const ids = (p) => accionesDe(p).map((a) => a.id)

describe('acciones por estado', () => {
  it('un pedido por revisar se confirma o se rechaza, nada más', () => {
    expect(ids({ estado: 'nuevo' })).toEqual(['confirmar', 'rechazar'])
  })

  it('uno en preparación se puede marcar listo o cancelar', () => {
    expect(ids({ estado: 'en_preparacion' })).toContain('listo')
    expect(ids({ estado: 'en_preparacion' })).toContain('cancelar')
    // Todavía no salió: no puede entregarse.
    expect(ids({ estado: 'en_preparacion' })).not.toContain('entregado')
  })

  it('uno en ruta se entrega o falla, y ya no se cancela', () => {
    const a = ids({ estado: 'en_ruta' })
    expect(a).toContain('entregado')
    expect(a).toContain('fallida')
    expect(a).not.toContain('cancelar')
  })

  it('un pedido cerrado no ofrece nada', () => {
    for (const e of ['entregado', 'rechazado', 'cancelado', 'devuelto']) {
      expect(accionesDe({ estado: e })).toHaveLength(0)
    }
  })

  // Con envío del canal el local NO despacha: ofrecer asignar repartidor sería
  // ofrecer algo que no existe.
  it('con envío del canal no se asigna repartidor', () => {
    expect(ids({ estado: 'listo', modoEnvio: 'canal' })).not.toContain('asignar')
    expect(ids({ estado: 'listo', modoEnvio: 'propio' })).toContain('asignar')
  })

  // Rechazar y cancelar piden motivo: un pedido que se cae sin explicación no
  // deja aprender nada ni responderle al cliente.
  it('rechazar y cancelar exigen motivo', () => {
    const rechazar = accionesDe({ estado: 'nuevo' }).find((a) => a.id === 'rechazar')
    expect(rechazar.pideMotivo).toBe(true)
    const cancelar = accionesDe({ estado: 'listo', modoEnvio: 'propio' }).find((a) => a.id === 'cancelar')
    expect(cancelar.pideMotivo).toBe(true)
  })
})

describe('agruparBandeja', () => {
  // Revisar y atender son dos trabajos distintos: mezclarlos hace que lo urgente
  // se pierda entre lo que simplemente se está cocinando.
  it('separa lo que hay que revisar de lo que está en curso', () => {
    const g = agruparBandeja([
      { estado: 'nuevo' }, { estado: 'nuevo' },
      { estado: 'en_preparacion' }, { estado: 'en_ruta' },
      { estado: 'entregado' }, { estado: 'cancelado' },
    ])
    expect(g.nuevos).toHaveLength(2)
    expect(g.activos).toHaveLength(2)
    expect(g.cerrados).toHaveLength(2)
  })

  it('cuenta los demorados', () => {
    const ayer = new Date(Date.now() - 3600e3).toISOString()
    const g = agruparBandeja([{ estado: 'en_ruta', promesaEntrega: ayer }, { estado: 'en_ruta' }])
    expect(g.demorados).toBe(1)
  })

  // Un pedido ya entregado no puede estar «demorado»: la promesa se cumplió o no,
  // pero ya no hay nada que apurar.
  it('un pedido cerrado nunca cuenta como demorado', () => {
    const ayer = new Date(Date.now() - 3600e3).toISOString()
    expect(estaDemorado({ estado: 'entregado', promesaEntrega: ayer })).toBe(false)
  })
})

describe('ventana de aceptación', () => {
  it('sin ventana no dibuja reloj', () => {
    expect(minutosRestantes('')).toBe(null)
    expect(minutosRestantes(undefined)).toBe(null)
  })

  it('vencida se queda en cero, no en negativo', () => {
    expect(minutosRestantes(new Date(Date.now() - 600e3).toISOString())).toBe(0)
  })

  it('cuenta los minutos que faltan', () => {
    const m = minutosRestantes(new Date(Date.now() + 5 * 60e3).toISOString())
    expect(m).toBeGreaterThanOrEqual(4)
    expect(m).toBeLessThanOrEqual(5)
  })
})

describe('origen', () => {
  it('nombra los tres orígenes en castellano', () => {
    expect(etiquetaOrigen('manual')).toBe('Mostrador')
    expect(etiquetaOrigen('ecommerce')).toBe('Tienda web')
    expect(etiquetaOrigen('app_commerce')).toBe('App de pedidos')
  })
})

describe('esFinal', () => {
  it('reconoce los cuatro cierres', () => {
    expect(['entregado', 'rechazado', 'cancelado', 'devuelto'].every(esFinal)).toBe(true)
    expect(esFinal('en_ruta')).toBe(false)
  })
})
