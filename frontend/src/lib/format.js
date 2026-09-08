// Helpers de formato (formato venezolano: miles con punto, decimales con coma)
// y validaciones de documentos fiscales (RIF/cédula V/E/J/G).

export const fmtCurrency = (n, ccy = 'VES', opts = {}) => {
  const max = opts.max ?? 2
  const val = Number(n) || 0
  const sign = val < 0 ? '-' : ''
  const abs = Math.abs(val)
  const parts = abs.toFixed(max).split('.')
  parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, '.')
  const numStr = parts[1] ? parts[0] + ',' + parts[1] : parts[0]
  const c = (ccy || 'VES').toUpperCase()
  // USD y EUR llevan su símbolo pegado al número; el resto (COP, USDT…) el
  // código con un espacio. VES sigue siendo «Bs 1.234,56».
  if (c === 'USD') return `${sign}$${numStr}`
  if (c === 'EUR') return `${sign}€${numStr}`
  const symbol = c === 'VES' ? 'Bs' : c
  return `${sign}${symbol} ${numStr}`
}

export const fmtCompact = (n, ccy = 'VES') => {
  const val = Number(n) || 0
  const abs = Math.abs(val)
  let s
  if (abs >= 1_000_000) s = (val / 1_000_000).toFixed(2).replace('.', ',') + 'M'
  else if (abs >= 1_000) s = (val / 1_000).toFixed(1).replace('.', ',') + 'K'
  else s = val.toFixed(2).replace('.', ',')
  const c = (ccy || 'VES').toUpperCase()
  const sym = c === 'USD' ? '$' : c === 'EUR' ? '€' : c === 'VES' ? 'Bs ' : c + ' '
  return sym + s
}

// Formato de número con separador de miles (sin símbolo de moneda).
export const fmtNum = (n, max = 2) => {
  const val = Number(n) || 0
  const sign = val < 0 ? '-' : ''
  const parts = Math.abs(val).toFixed(max).split('.')
  parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, '.')
  return sign + (parts[1] ? parts[0] + ',' + parts[1] : parts[0])
}

// Fecha: acepta "YYYY-MM-DD [HH:MM]" o ISO y la muestra dd-mm-yyyy.
export const fmtDate = (d) => {
  if (!d) return '—'
  if (typeof d !== 'string') return String(d)
  const iso = d.includes('T') ? d.split('T') : d.split(' ')
  const fecha = iso[0].split('-')
  if (fecha.length !== 3) return d
  const hora = iso[1] ? iso[1].slice(0, 5) : ''
  return fecha.reverse().join('-') + (hora ? ' · ' + hora : '')
}

// ---- Validación de RIF / documento (V/E/J/G) ----
// Tipos válidos: V (venezolano), E (extranjero), J (jurídico), G (gobierno).
export const TIPOS_DOCUMENTO = ['V', 'E', 'J', 'G', 'P']

// Dígito verificador del RIF — algoritmo del SENIAT (módulo 11). `letra` es el
// tipo (V/E/J/G/P/C) y `cuerpo` los 8 dígitos del cuerpo. Devuelve el dígito
// (0–9) o null si el cuerpo no son 8 dígitos. Pesos [letra, d1..d8].
const VALOR_LETRA_RIF = { V: 1, E: 2, J: 3, P: 4, G: 5, C: 6 }
const PESOS_RIF = [4, 3, 2, 7, 6, 5, 4, 3, 2]
export function digitoVerificadorRIF(letra, cuerpo) {
  const val = VALOR_LETRA_RIF[String(letra || '').toUpperCase()] || 0
  const c = String(cuerpo || '')
  if (!val || !/^\d{8}$/.test(c)) return null
  let suma = val * PESOS_RIF[0]
  for (let i = 0; i < 8; i++) suma += Number(c[i]) * PESOS_RIF[i + 1]
  let dv = 11 - (suma % 11)
  if (dv >= 10) dv = 0
  return dv
}

/* Valida un documento fiscal venezolano según su tipo (formato SENIAT). Acepta con
 * o sin guiones/espacios. Devuelve {valid, tipo, msg}. Reglas por prefijo:
 *   · V / E — cédula de persona natural (venezolano / extranjero): SOLO dígitos, la
 *     cédula tal cual (sin dígito verificador). Hoy llegan a 8 dígitos; se aceptan
 *     6–9 para cubrir cédulas viejas y el margen futuro.
 *   · J / G — RIF jurídico / gubernamental: 9 dígitos = 8 de cuerpo + 1 dígito
 *     verificador (p. ej. J-12345678-9).
 *   · P — pasaporte (extranjero sin cédula): alfanumérico, 4–20 caracteres.
 * NOTA: no se valida el dígito verificador del RIF (módulo 11). El modelo guarda
 * cédulas SIN verificador, y los RIF cargados/importados no siempre lo traen
 * consistente; validar el DV rechazaría datos válidos. Puede añadirse aparte. */
export function validarRIF(raw) {
  const v = String(raw || '').trim().toUpperCase()
  if (!v) return { valid: false, tipo: '', msg: 'Ingresa el RIF o documento.' }
  const tipo = v[0]
  if (!TIPOS_DOCUMENTO.includes(tipo)) {
    return { valid: false, tipo: '', msg: 'Debe empezar con V, E, J, G o P.' }
  }
  const resto = v.slice(1).replace(/[-\s]/g, '')
  if (tipo === 'P') {
    if (!/^[A-Z0-9]{4,20}$/.test(resto)) {
      return { valid: false, tipo, msg: 'El pasaporte debe tener entre 4 y 20 caracteres (letras o números).' }
    }
    return { valid: true, tipo, msg: '' }
  }
  if (tipo === 'J' || tipo === 'G') {
    if (!/^\d{9}$/.test(resto)) {
      return { valid: false, tipo, msg: 'El RIF debe tener 9 dígitos (8 + dígito verificador).' }
    }
    const dv = digitoVerificadorRIF(tipo, resto.slice(0, 8))
    if (String(dv) !== resto[8]) {
      return { valid: false, tipo, msg: `El dígito verificador del RIF no es correcto (debería terminar en ${dv}).` }
    }
    return { valid: true, tipo, msg: '' }
  }
  // V / E — cédula (sin dígito verificador).
  if (!/^\d{6,9}$/.test(resto)) {
    return { valid: false, tipo, msg: 'La cédula debe tener entre 6 y 9 dígitos.' }
  }
  return { valid: true, tipo, msg: '' }
}

/* Normaliza a la forma canónica según el tipo: J/G → X-12345678-9 (8 cuerpo + 1
 * verificador); V/E → X-<cédula> (sin verificador); P → P-<valor>. Si el documento
 * no valida el formato de su tipo, se devuelve tal cual (en mayúsculas, recortado). */
export function normalizarRIF(raw) {
  const v = String(raw || '').trim().toUpperCase()
  const tipo = v[0]
  const resto = v.slice(1).replace(/[-\s]/g, '')
  if (!TIPOS_DOCUMENTO.includes(tipo)) return v
  if ((tipo === 'J' || tipo === 'G') && /^\d{9}$/.test(resto)) {
    return `${tipo}-${resto.slice(0, -1)}-${resto.slice(-1)}`
  }
  if ((tipo === 'V' || tipo === 'E') && /^\d{6,9}$/.test(resto)) {
    return `${tipo}-${resto}`
  }
  if (tipo === 'P' && resto) return `P-${resto}`
  return v
}
