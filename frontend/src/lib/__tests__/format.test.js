// Pruebas de formato es-VE y validación de RIF (src/lib/format.js).
import { describe, it, expect } from 'vitest'
import {
  fmtCurrency,
  fmtNum,
  fmtCompact,
  validarRIF,
  normalizarRIF,
  digitoVerificadorRIF,
  TIPOS_DOCUMENTO,
} from '../format.js'

describe('TIPOS_DOCUMENTO', () => {
  it('son V, E, J, G y P (pasaporte)', () => {
    expect(TIPOS_DOCUMENTO).toEqual(['V', 'E', 'J', 'G', 'P'])
  })
})

describe('fmtCurrency — miles con punto, decimales con coma', () => {
  it('VES: "Bs 1.234,56"', () => {
    expect(fmtCurrency(1234.56)).toBe('Bs 1.234,56')
  })
  it('USD lleva $ pegado al número', () => {
    expect(fmtCurrency(1234.5, 'USD')).toBe('$1.234,50')
  })
  it('EUR lleva € pegado al número', () => {
    expect(fmtCurrency(1234.5, 'EUR')).toBe('€1.234,50')
  })
  it('otras divisas: código + espacio', () => {
    expect(fmtCurrency(1000, 'COP')).toBe('COP 1.000,00')
    expect(fmtCurrency(1000, 'USDT')).toBe('USDT 1.000,00')
  })
  it('negativo lleva el signo delante del símbolo', () => {
    expect(fmtCurrency(-1234.56)).toBe('-Bs 1.234,56')
  })
  it('cero', () => {
    expect(fmtCurrency(0)).toBe('Bs 0,00')
  })
  it('opts.max controla los decimales (max 0 redondea sin coma)', () => {
    expect(fmtCurrency(1.5, 'VES', { max: 0 })).toBe('Bs 2')
  })
  it('valor no numérico → 0', () => {
    expect(fmtCurrency('x')).toBe('Bs 0,00')
  })
  it('ccy por defecto es VES', () => {
    expect(fmtCurrency(5)).toBe('Bs 5,00')
  })
})

describe('fmtNum — número sin símbolo', () => {
  it('separador de miles y 2 decimales por defecto', () => {
    expect(fmtNum(1234567.891)).toBe('1.234.567,89')
  })
  it('max 0 → sin decimales', () => {
    expect(fmtNum(1000, 0)).toBe('1.000')
  })
  it('negativo', () => {
    expect(fmtNum(-1234.5)).toBe('-1.234,50')
  })
  it('cero', () => {
    expect(fmtNum(0)).toBe('0,00')
  })
  it('valor no numérico → 0', () => {
    expect(fmtNum(null)).toBe('0,00')
  })
})

describe('fmtCompact', () => {
  it('millones → M', () => {
    expect(fmtCompact(2_500_000)).toBe('Bs 2,50M')
  })
  it('miles → K', () => {
    expect(fmtCompact(2_500, 'USD')).toBe('$2,5K')
  })
  it('menores a mil → valor con 2 decimales', () => {
    expect(fmtCompact(12.5, 'EUR')).toBe('€12,50')
  })
})

describe('validarRIF', () => {
  it('vacío → inválido con mensaje de ingreso', () => {
    const r = validarRIF('')
    expect(r.valid).toBe(false)
    expect(r.msg).toBe('Ingresa el RIF o documento.')
  })

  it('prefijo no permitido → inválido', () => {
    const r = validarRIF('X12345678')
    expect(r.valid).toBe(false)
    expect(r.msg).toBe('Debe empezar con V, E, J, G o P.')
  })

  // Cédula (V/E): 6–9 dígitos, SIN dígito verificador.
  it('V con 8 dígitos (cédula) → válido, tipo V', () => {
    const r = validarRIF('V12345678')
    expect(r.valid).toBe(true)
    expect(r.tipo).toBe('V')
  })

  it('V con 7 dígitos (cédula) → válido', () => {
    expect(validarRIF('V1234567').valid).toBe(true)
  })

  it('V con 6 dígitos (cédula vieja) → válido', () => {
    expect(validarRIF('V123456').valid).toBe(true)
  })

  it('V con 5 dígitos → inválido (mínimo 6)', () => {
    const r = validarRIF('V12345')
    expect(r.valid).toBe(false)
    expect(r.msg).toBe('La cédula debe tener entre 6 y 9 dígitos.')
  })

  it('V con 10 dígitos → inválido (máximo 9)', () => {
    expect(validarRIF('V1234567890').valid).toBe(false)
  })

  it('E es cédula de extranjero (8 dígitos) → válido', () => {
    expect(validarRIF('E84512399').valid).toBe(true)
  })

  it('acepta minúsculas (se normaliza a mayúsculas)', () => {
    const r = validarRIF('v12345678')
    expect(r.valid).toBe(true)
    expect(r.tipo).toBe('V')
  })

  // RIF (J/G): EXACTAMENTE 9 dígitos (8 cuerpo + 1 verificador) con DV válido.
  it('J con DV correcto y guiones → válido (ignora - y espacios)', () => {
    const r = validarRIF('J-12345678-4') // DV de 12345678 es 4
    expect(r.valid).toBe(true)
    expect(r.tipo).toBe('J')
  })

  it('G gubernamental con DV correcto → válido', () => {
    expect(validarRIF('G-20000123-2').valid).toBe(true) // DV de 20000123 es 2
  })

  it('J con 8 dígitos (falta el verificador) → inválido', () => {
    const r = validarRIF('J12345678')
    expect(r.valid).toBe(false)
    expect(r.msg).toBe('El RIF debe tener 9 dígitos (8 + dígito verificador).')
  })

  it('J con 10 dígitos → inválido', () => {
    expect(validarRIF('J1234567890').valid).toBe(false)
  })

  // Pasaporte (P): alfanumérico, 4–20 caracteres.
  it('P alfanumérico (4–20) → válido', () => {
    expect(validarRIF('PAB123456').valid).toBe(true)
    expect(validarRIF('P123456789').valid).toBe(true)
  })

  it('P demasiado corto (3) → inválido', () => {
    expect(validarRIF('PAB1').valid).toBe(false)
  })

  // Ahora SÍ valida el dígito verificador (módulo 11, SENIAT): un DV incorrecto
  // se rechaza con un mensaje que indica el dígito esperado.
  it('rechaza RIF con dígito verificador incorrecto', () => {
    const r = validarRIF('J-12345678-9') // el DV correcto es 4, no 9
    expect(r.valid).toBe(false)
    expect(r.msg).toContain('dígito verificador')
    expect(r.msg).toContain('4')
  })
})

describe('digitoVerificadorRIF (SENIAT módulo 11)', () => {
  it('calcula el DV de cuerpos conocidos', () => {
    expect(digitoVerificadorRIF('J', '12345678')).toBe(4)
    expect(digitoVerificadorRIF('G', '20000123')).toBe(2)
    expect(digitoVerificadorRIF('J', '00012345')).toBe(4)
    expect(digitoVerificadorRIF('J', '40123456')).toBe(9)
  })
  it('cuerpo que no son 8 dígitos → null', () => {
    expect(digitoVerificadorRIF('J', '1234567')).toBe(null)
    expect(digitoVerificadorRIF('J', 'abcdefgh')).toBe(null)
  })
})

describe('normalizarRIF', () => {
  it('canoniza a TIPO-CUERPO-DV', () => {
    expect(normalizarRIF('j123456789')).toBe('J-12345678-9')
  })
  it('respeta guiones/espacios de entrada', () => {
    expect(normalizarRIF('J-12345678-9')).toBe('J-12345678-9')
  })
  it('V/E se canoniza SIN verificador (X-<cédula>)', () => {
    expect(normalizarRIF('v12345678')).toBe('V-12345678')
    expect(normalizarRIF('E84512399')).toBe('E-84512399')
  })
  it('entrada inválida (muy corta) se devuelve tal cual en mayúsculas', () => {
    expect(normalizarRIF('v123')).toBe('V123')
  })
})
