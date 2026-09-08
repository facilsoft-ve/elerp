// Pruebas de la resolución de tema de la pantalla del cliente (src/lib/tema.js).
import { describe, it, expect } from 'vitest'
import {
  ENFASIS_DEFAULT,
  TEXTO_DEFAULT,
  FONDO_DEFAULT,
  logoDeVersion,
  resolverTema,
  fondoEspacio,
  estiloFondoGeneral,
} from '../tema.js'

describe('logoDeVersion', () => {
  const empresa = {
    logo: 'color.svg',
    logoBlanco: 'blanco.svg',
    logoNegro: 'negro.svg',
    logoAlterno: 'alterno.svg',
  }
  it('color (o desconocido) → logo base', () => {
    expect(logoDeVersion(empresa, 'color')).toBe('color.svg')
    expect(logoDeVersion(empresa, 'zzz')).toBe('color.svg')
  })
  it('blanco → logoBlanco', () => {
    expect(logoDeVersion(empresa, 'blanco')).toBe('blanco.svg')
  })
  it('negro → logoNegro', () => {
    expect(logoDeVersion(empresa, 'negro')).toBe('negro.svg')
  })
  it('blanco degrada a logoAlterno y luego a logo', () => {
    expect(logoDeVersion({ logo: 'c.svg', logoAlterno: 'a.svg' }, 'blanco')).toBe('a.svg')
    expect(logoDeVersion({ logo: 'c.svg' }, 'blanco')).toBe('c.svg')
  })
  it('negro degrada a logo de color', () => {
    expect(logoDeVersion({ logo: 'c.svg' }, 'negro')).toBe('c.svg')
  })
  it('empresa nula → cadena vacía', () => {
    expect(logoDeVersion(null, 'color')).toBe('')
  })
})

describe('resolverTema — defaults de marca', () => {
  it('empresa nula → énfasis/texto/logoVersion por defecto', () => {
    const t = resolverTema(null)
    expect(t.enfasis).toBe(ENFASIS_DEFAULT)
    expect(t.texto).toBe(TEXTO_DEFAULT)
    expect(t.logoVersion).toBe('color')
    expect(t.fondo).toBe('')
    expect(t.logo).toBe('')
  })

  it('empresa sin temaPantalla → defaults, logo base', () => {
    const t = resolverTema({ logo: 'c.svg' })
    expect(t.enfasis).toBe(ENFASIS_DEFAULT)
    expect(t.logo).toBe('c.svg')
    expect(t.logoVersion).toBe('color')
  })
})

describe('resolverTema — tema de la empresa', () => {
  const empresa = {
    logo: 'color.svg',
    logoBlanco: 'blanco.svg',
    temaPantalla: {
      fondo: 'navy',
      fondoCabecera: 'head',
      colorEnfasis: '#123456',
      colorTexto: '#000000',
      logoVersion: 'blanco',
    },
  }

  it('usa colores y fondos del tema de la empresa', () => {
    const t = resolverTema(empresa)
    expect(t.fondo).toBe('navy')
    expect(t.fondoCabecera).toBe('head')
    expect(t.enfasis).toBe('#123456')
    expect(t.texto).toBe('#000000')
    expect(t.logoVersion).toBe('blanco')
    expect(t.logo).toBe('blanco.svg') // resuelto vía logoDeVersion
  })

  it('colorMarca (compat) alimenta el énfasis si no hay colorEnfasis nuevo', () => {
    const t = resolverTema({ colorMarca: '#abcdef', temaPantalla: {} })
    expect(t.enfasis).toBe('#abcdef')
  })
})

describe('resolverTema — override por caja', () => {
  const empresa = {
    logo: 'color.svg',
    logoBlanco: 'blanco.svg',
    temaPantalla: { fondo: 'navy', logoVersion: 'color' },
  }

  it('override.fondo pisa el fondo general', () => {
    const t = resolverTema(empresa, { fondo: 'rojo' })
    expect(t.fondo).toBe('rojo')
  })

  it('override.logoVersion pisa la versión y reresuelve el logo (contraste)', () => {
    const t = resolverTema(empresa, { logoVersion: 'blanco' })
    expect(t.logoVersion).toBe('blanco')
    expect(t.logo).toBe('blanco.svg')
  })

  it('override vacío → toma los valores del tema de la empresa', () => {
    const t = resolverTema(empresa, {})
    expect(t.fondo).toBe('navy')
    expect(t.logoVersion).toBe('color')
  })
})

describe('fondoEspacio', () => {
  it('color propio del espacio gana', () => {
    expect(fondoEspacio({ fondoCabecera: 'head', fondo: 'gen' }, 'fondoCabecera')).toBe('head')
  })
  it('sin color propio → hereda del fondo general', () => {
    expect(fondoEspacio({ fondoCabecera: '', fondo: 'gen' }, 'fondoCabecera')).toBe('gen')
  })
  it('ni propio ni general → cadena vacía (el llamador aplica su default)', () => {
    expect(fondoEspacio({}, 'fondoCabecera')).toBe('')
  })
  it('tema nulo → cadena vacía', () => {
    expect(fondoEspacio(null, 'fondoCabecera')).toBe('')
  })
})

describe('estiloFondoGeneral', () => {
  it('con fondo configurado → ese fondo', () => {
    expect(estiloFondoGeneral({ fondo: 'navy' })).toBe('navy')
  })
  it('sin fondo → degradado navy por defecto', () => {
    expect(estiloFondoGeneral({})).toBe(FONDO_DEFAULT)
    expect(estiloFondoGeneral(null)).toBe(FONDO_DEFAULT)
  })
})
