/* Generador mínimo de XLSX, sin dependencias.
 *
 * POR QUÉ EXISTE: la contadora pidió que los libros fiscales se exporten en
 * XLSX y no en TXT. Renombrar un CSV a .xlsx NO sirve — Excel lo abre con un
 * aviso de formato corrupto, o peor, lo interpreta con la configuración regional
 * del equipo y convierte «10-03» en una fecha. Un XLSX de verdad es un ZIP con
 * unos XML adentro, y eso se puede escribir a mano en menos de lo que pesa una
 * librería de hoja de cálculo (las habituales rondan el megabyte, y este
 * frontend solo depende de react y react-dom).
 *
 * Alcance deliberado: una hoja, celdas de texto o número, encabezado en negrita.
 * No hay fórmulas, ni formatos condicionales, ni varias hojas. Es lo que un
 * libro fiscal necesita: una tabla que la contadora pueda abrir y filtrar.
 */

// --- ZIP (método STORE, sin compresión) ------------------------------------
//
// Excel acepta entradas almacenadas sin comprimir, así que no hace falta
// implementar deflate. Un libro de un mes son decenas de KB: la diferencia de
// tamaño no justifica el código.

// crc32 con tabla precalculada. El ZIP exige el CRC de cada entrada; sin él
// Excel rechaza el archivo.
const TABLA_CRC = (() => {
  const t = new Uint32Array(256)
  for (let i = 0; i < 256; i++) {
    let c = i
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    t[i] = c >>> 0
  }
  return t
})()

function crc32(bytes) {
  let c = 0xffffffff
  for (let i = 0; i < bytes.length; i++) c = TABLA_CRC[(c ^ bytes[i]) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}

const utf8 = (s) => new TextEncoder().encode(s)

// Escritor de bytes que crece solo: más simple que calcular el tamaño final dos
// veces (una para medir y otra para escribir).
function buffer() {
  let bytes = []
  return {
    u8: (v) => bytes.push(v & 0xff),
    u16: (v) => bytes.push(v & 0xff, (v >>> 8) & 0xff),
    u32: (v) => bytes.push(v & 0xff, (v >>> 8) & 0xff, (v >>> 16) & 0xff, (v >>> 24) & 0xff),
    raw: (arr) => { for (let i = 0; i < arr.length; i++) bytes.push(arr[i]) },
    get len() { return bytes.length },
    done: () => new Uint8Array(bytes),
  }
}

/* zip arma un archivo ZIP a partir de {nombre: contenidoTexto}. Devuelve un
 * Uint8Array listo para envolver en Blob. */
export function zip(archivos) {
  const out = buffer()
  const centrales = []
  for (const [nombre, texto] of Object.entries(archivos)) {
    const nom = utf8(nombre)
    const datos = utf8(texto)
    const crc = crc32(datos)
    const offset = out.len

    // Cabecera local.
    out.u32(0x04034b50)
    out.u16(20)   // versión mínima
    out.u16(0x800) // bit 11: nombres en UTF-8
    out.u16(0)    // método 0 = almacenado
    out.u16(0); out.u16(0) // hora y fecha: irrelevantes para Excel
    out.u32(crc)
    out.u32(datos.length) // comprimido == sin comprimir
    out.u32(datos.length)
    out.u16(nom.length)
    out.u16(0)
    out.raw(nom)
    out.raw(datos)

    centrales.push({ nom, crc, size: datos.length, offset })
  }

  // Directorio central: es lo que el lector recorre para encontrar las entradas.
  const inicioCentral = out.len
  for (const e of centrales) {
    out.u32(0x02014b50)
    out.u16(20); out.u16(20)
    out.u16(0x800); out.u16(0)
    out.u16(0); out.u16(0)
    out.u32(e.crc)
    out.u32(e.size); out.u32(e.size)
    out.u16(e.nom.length)
    out.u16(0); out.u16(0); out.u16(0); out.u16(0)
    out.u32(0)
    out.u32(e.offset)
    out.raw(e.nom)
  }
  const tamCentral = out.len - inicioCentral

  // Fin del directorio central.
  out.u32(0x06054b50)
  out.u16(0); out.u16(0)
  out.u16(centrales.length); out.u16(centrales.length)
  out.u32(tamCentral); out.u32(inicioCentral)
  out.u16(0)
  return out.done()
}

// --- XLSX ------------------------------------------------------------------

// escaparXML protege los cinco caracteres que rompen un XML. La razón social de
// un proveedor trae ampersands más seguido de lo que uno cree.
const escaparXML = (v) => String(v == null ? '' : v)
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;').replace(/'/g, '&apos;')
  // Los caracteres de control no son válidos en XML 1.0 y Excel rechaza el
  // archivo entero si aparece uno.
  .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F]/g, '')

// columna convierte un índice 0-based en letra de columna (0→A, 26→AA).
export function columna(i) {
  let s = ''
  let n = i
  do {
    s = String.fromCharCode(65 + (n % 26)) + s
    n = Math.floor(n / 26) - 1
  } while (n >= 0)
  return s
}

// esNumero decide si una celda va como número (alineada a la derecha, sumable)
// o como texto. Un RIF como "J-12345678-9" NO es número, y un número guardado
// como texto no se puede sumar en la hoja: de esto depende que la contadora
// pueda trabajar el libro sin re-teclearlo.
const esNumero = (v) => typeof v === 'number' && Number.isFinite(v)

function celda(ref, valor, estilo) {
  const s = estilo ? ` s="${estilo}"` : ''
  if (esNumero(valor)) return `<c r="${ref}"${s}><v>${valor}</v></c>`
  const txt = escaparXML(valor)
  if (txt === '') return `<c r="${ref}"${s}/>`
  // t="inlineStr" evita la tabla de cadenas compartidas: más XML, mucho menos
  // código, y Excel lo lee igual.
  return `<c r="${ref}"${s} t="inlineStr"><is><t xml:space="preserve">${txt}</t></is></c>`
}

/* hojaXLSX arma el XLSX de UNA hoja.
 *
 *   nombre  — nombre de la pestaña
 *   filas   — array de arrays; la primera es el encabezado (sale en negrita)
 *
 * Devuelve un Uint8Array. */
export function hojaXLSX(nombre, filas) {
  const xmlFilas = filas.map((fila, i) => {
    const celdas = fila.map((v, j) => celda(`${columna(j)}${i + 1}`, v, i === 0 ? '1' : null)).join('')
    return `<row r="${i + 1}">${celdas}</row>`
  }).join('')

  // El nombre de pestaña de Excel no admite : \ / ? * [ ] y tope de 31 chars.
  const pestana = escaparXML(String(nombre || 'Hoja1').replace(/[:\\/?*[\]]/g, ' ').slice(0, 31))

  return zip({
    '[Content_Types].xml':
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
      `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
      `<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
      `<Default Extension="xml" ContentType="application/xml"/>` +
      `<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
      `<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
      `<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
      `</Types>`,
    '_rels/.rels':
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
      `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
      `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
      `</Relationships>`,
    'xl/workbook.xml':
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
      `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
      `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
      `<sheets><sheet name="${pestana}" sheetId="1" r:id="rId1"/></sheets></workbook>`,
    'xl/_rels/workbook.xml.rels':
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
      `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
      `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
      `<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
      `</Relationships>`,
    // Un solo estilo (el 1): negrita, para el encabezado.
    'xl/styles.xml':
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
      `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
      `<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font>` +
      `<font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
      `<fills count="1"><fill><patternFill patternType="none"/></fill></fills>` +
      `<borders count="1"><border/></borders>` +
      `<cellStyleXfs count="1"><xf/></cellStyleXfs>` +
      `<cellXfs count="2"><xf xfId="0"/><xf xfId="0" fontId="1" applyFont="1"/></cellXfs>` +
      `</styleSheet>`,
    'xl/worksheets/sheet1.xml':
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
      `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
      `<sheetData>${xmlFilas}</sheetData></worksheet>`,
  })
}

/* descargarXLSX dispara la descarga del libro en el navegador. Se separa de
 * hojaXLSX para que el armado sea una función pura y se pueda probar. */
export function descargarXLSX(nombreArchivo, nombreHoja, filas) {
  const bytes = hojaXLSX(nombreHoja, filas)
  const blob = new Blob([bytes], {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = nombreArchivo
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}
