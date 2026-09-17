import { describe, it, expect } from 'vitest'
import { hojaXLSX, zip, columna } from '../xlsx.js'

/* El XLSX se escribe a mano (ZIP + XML) para no arrastrar una librería de un
 * megabyte. El riesgo de hacerlo a mano es que Excel rechace el archivo y nadie
 * se entere hasta que la contadora intente abrirlo, así que las piezas que Excel
 * valida —firmas del ZIP, CRC, XML bien formado— se prueban acá. */

// texto lee un tramo del Uint8Array como ASCII.
const texto = (bytes, desde, largo) =>
  String.fromCharCode(...bytes.slice(desde, desde + largo))

describe('zip', () => {
  it('escribe la firma de cabecera local y la de fin de directorio', () => {
    const b = zip({ 'a.txt': 'hola' })
    // "PK\x03\x04" al inicio: sin esto ningún lector reconoce el archivo.
    expect(texto(b, 0, 4)).toBe('PK')
    // "PK\x05\x06" al final (sin comentario, son los últimos 22 bytes).
    expect(texto(b, b.length - 22, 4)).toBe('PK')
  })

  it('guarda el contenido literal de cada entrada', () => {
    const b = zip({ 'a.txt': 'hola mundo' })
    expect(String.fromCharCode(...b)).toContain('hola mundo')
  })

  it('declara tantas entradas como archivos', () => {
    const b = zip({ 'a.xml': '<a/>', 'b/c.xml': '<b/>', 'd.rels': '<d/>' })
    // El contador de entradas del EOCD vive en los bytes 10-11 desde su inicio.
    const eocd = b.length - 22
    expect(b[eocd + 10] | (b[eocd + 11] << 8)).toBe(3)
  })
})

describe('columna', () => {
  it('numera como Excel, incluido el salto a dos letras', () => {
    expect(columna(0)).toBe('A')
    expect(columna(25)).toBe('Z')
    // El salto Z→AA es donde fallan casi todas las implementaciones ingenuas.
    expect(columna(26)).toBe('AA')
    expect(columna(27)).toBe('AB')
    expect(columna(51)).toBe('AZ')
    expect(columna(52)).toBe('BA')
  })
})

describe('hojaXLSX', () => {
  // Se decodifica como UTF-8 (no byte a byte): las tildes de una razón social
  // ocupan dos bytes y leerlas como ASCII las partiría, haciendo fallar la
  // prueba por culpa del lector y no del archivo.
  const cadena = (filas, nombre = 'Hoja') =>
    new TextDecoder('utf-8').decode(hojaXLSX(nombre, filas))

  it('incluye las partes que Excel exige', () => {
    const s = cadena([['A'], ['B']])
    for (const parte of [
      '[Content_Types].xml', '_rels/.rels', 'xl/workbook.xml',
      'xl/_rels/workbook.xml.rels', 'xl/worksheets/sheet1.xml', 'xl/styles.xml',
    ]) {
      expect(s).toContain(parte)
    }
  })

  it('escribe los números como número y el texto como cadena', () => {
    const s = cadena([['Base'], [1234.56], ['J-12345678-9']])
    // Un número va en <v> y es sumable en la hoja.
    expect(s).toContain('<v>1234.56</v>')
    // Un RIF NO puede irse como número: Excel lo convertiría en una resta.
    expect(s).toContain('J-12345678-9')
    expect(s).toContain('t="inlineStr"')
  })

  it('escapa los caracteres que romperían el XML', () => {
    const s = cadena([['Proveedor'], ['Pérez & Cía. <S.A.>']])
    expect(s).toContain('Pérez &amp; Cía. &lt;S.A.&gt;')
    // Y el crudo no queda suelto, que es lo que corrompería el archivo.
    expect(s).not.toContain('& Cía. <S.A.>')
  })

  it('pone el encabezado en negrita y el cuerpo sin estilo', () => {
    const s = cadena([['Fecha', 'Total'], ['01/09/2026', 100]])
    expect(s).toContain('<c r="A1" s="1"')  // fila 1 = encabezado
    expect(s).toContain('<c r="A2"')        // fila 2 = cuerpo, sin s="1"
    expect(s).not.toContain('<c r="A2" s="1"')
  })

  it('sanea el nombre de la pestaña', () => {
    // Excel no admite : \ / ? * [ ] ni más de 31 caracteres en una pestaña; con
    // uno de esos, el archivo no abre.
    const s = cadena([['x']], 'Libro/Ventas:2026*[borrador]')
    expect(s).toContain('name="Libro Ventas 2026  borrador "')
  })

  it('no deja caracteres de control, que invalidan el XML entero', () => {
    // El literal lleva un BEL de verdad (invisible en el editor): es el caso
    // real, un caracter de control que se cuela en una razon social pegada
    // desde otro sistema y que invalidaria el XML entero.
    const s = cadena([['a'], ['concampana']])
    expect(s).toContain('concampana')
  })

  it('numera las filas desde 1', () => {
    const s = cadena([['a'], ['b'], ['c']])
    expect(s).toContain('<row r="1">')
    expect(s).toContain('<row r="3">')
  })
})
