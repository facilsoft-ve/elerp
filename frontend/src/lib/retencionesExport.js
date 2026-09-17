/* Exportación de los COMPROBANTES DE RETENCIÓN al formato del SENIAT.
 *
 * POR QUÉ ESTO Y NO XLSX: la contadora pidió las dos cosas, y no se contradicen
 * porque son cosas distintas. Los LIBROS fiscales (ventas y compras) se trabajan
 * en hoja de cálculo y van en XLSX — eso es lib/xlsx.js. Los COMPROBANTES DE
 * RETENCIÓN se PRESENTAN ante el SENIAT y van en su formato: TXT para las de
 * IVA, XML para las de ISLR.
 *
 *   «LAS RETENCIONES SÍ SE DEBEN EXPORTAR COMP TXT»                   (15:49)
 *   «RETENCIONES DE IVA SE EXPORTAN A TXT Y RETENCIONES DE ISLR
 *    EXPORTAN A XML»                                                  (15:58)
 *
 * ────────────────────────────────────────────────────────────────────────────
 * ADVERTENCIA, Y ES IMPORTANTE: EL LAYOUT EXACTO NO ESTÁ CONFIRMADO.
 *
 * El orden de columnas del TXT y el esquema del XML los define la providencia
 * vigente, y no la tenemos a mano. Lo que hay acá es una estructura RAZONABLE y
 * documentada —con los campos que el comprobante necesita— para que la contadora
 * la contraste contra el formato oficial y diga qué mover. NO está verificada
 * contra un archivo aceptado por el portal del SENIAT.
 *
 * Es el mismo criterio que se usó con el catálogo de impresoras fiscales: se
 * entrega algo utilizable y se dice con todas las letras qué falta confirmar, en
 * vez de presentar una suposición como si fuera la norma.
 *
 * Lo concreto que hay que confirmar está en PENDIENTE_CONFIRMAR, y la pantalla
 * lo muestra al lado del botón para que nadie lo presente creyendo que ya está.
 * ──────────────────────────────────────────────────────────────────────────── */

/* Lo que falta confirmar con la contadora, en orden de riesgo. Se exporta para
 * que la interfaz lo muestre y no quede solo en un comentario que nadie lee. */
export const PENDIENTE_CONFIRMAR = [
  'El orden exacto de las columnas del TXT de IVA y el separador (acá: «|»).',
  'El esquema del XML de ISLR: nombres de etiquetas y estructura.',
  'El formato de fecha (acá: DD/MM/AAAA) y el separador decimal (acá: punto).',
  'Si el TXT lleva fila de encabezado (acá NO, como un archivo de presentación).',
  'El código de tipo de documento y de operación que espera el portal.',
]

/* Orden de columnas del TXT de retenciones de IVA.
 *
 * Va EXPORTADO a propósito: el archivo se entrega sin fila de encabezado —como
 * un archivo de presentación— así que sin esta lista a la vista nadie puede
 * verificar qué es cada campo. La pantalla la muestra junto al botón.
 */
export const COLUMNAS_TXT_IVA = [
  'RIF del agente de retención',
  'Período (AAAAMM)',
  'RIF del sujeto retenido',
  'Nº de comprobante',
  'Fecha del documento',
  'Tipo de documento',
  'Nº del documento',
  'Base imponible',
  'Alícuota (%)',
  'IVA retenido',
]

// SEPARADOR del TXT. La barra vertical no aparece en una razón social ni en un
// RIF, así que no hay que escapar nada — a diferencia de la coma o el punto y
// coma, que sí aparecen en nombres de empresas.
const SEP = '|'

/* ---------------------------------------------------------------------------
 * Normalización de valores
 * ------------------------------------------------------------------------- */

/* texto deja el valor TAL CUAL vino, como cadena.
 *
 * Es deliberado y ya nos mordió antes en los exports: un RIF «J-12345678-9» que
 * se convierta a número se vuelve una resta, y un correlativo «00001234» pierde
 * los ceros de la izquierda. Los identificadores no se tocan nunca. */
const texto = (v) => (v == null ? '' : String(v)).trim()

/* monto con dos decimales y punto decimal. El separador es de lo que hay que
 * confirmar: si la providencia pide coma, se cambia acá y en un solo lugar. */
const monto = (v) => (Number(v) || 0).toFixed(2)

/* fecha ISO → DD/MM/AAAA en UTC. En UTC a propósito: con la hora local, una
 * fecha guardada como medianoche se corre al día anterior según la zona, y una
 * retención declarada en el mes equivocado es un problema de verdad. */
export function fechaSENIAT(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return String(iso)
  const dd = String(d.getUTCDate()).padStart(2, '0')
  const mm = String(d.getUTCMonth() + 1).padStart(2, '0')
  return `${dd}/${mm}/${d.getUTCFullYear()}`
}

/* periodoDe arma el AAAAMM del período. */
export const periodoDe = (anio, mes) => `${anio}${String(mes).padStart(2, '0')}`

/* escaparXML protege los cinco caracteres que rompen un XML, y descarta los de
 * control (que no son válidos en XML 1.0 y hacen que el documento entero se
 * rechace). Una razón social con «&» es más común de lo que parece. */
export const escaparXML = (v) => String(v == null ? '' : v)
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;').replace(/'/g, '&apos;')
  .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F]/g, '')

/* ---------------------------------------------------------------------------
 * Filtro por período e impuesto
 * ------------------------------------------------------------------------- */

/* filtrarRetenciones acota la lista al período y al impuesto.
 *
 * El período se compara por el prefijo AAAA-MM de la fecha del COMPROBANTE (no
 * la de registro): lo que se declara es cuándo se retuvo, no cuándo alguien lo
 * cargó en el sistema.
 *
 * `tipo` acota además la dirección (recibida/emitida) cuando se pasa. Se declara
 * lo EMITIDO —lo que uno le retuvo a terceros—, pero se deja el parámetro
 * abierto porque cuál de las dos va en cada archivo es justamente algo que la
 * contadora tiene que confirmar. */
export function filtrarRetenciones(retenciones, { anio, mes, impuesto, tipo } = {}) {
  const prefijo = anio && mes ? `${anio}-${String(mes).padStart(2, '0')}` : null
  return (retenciones || []).filter((r) => {
    if (impuesto && (r.impuesto === 'islr' ? 'islr' : 'iva') !== impuesto) return false
    if (tipo && r.tipo !== tipo) return false
    if (prefijo && String(r.fecha || '').slice(0, 7) !== prefijo) return false
    return true
  })
}

/* ---------------------------------------------------------------------------
 * TXT — retenciones de IVA
 * ------------------------------------------------------------------------- */

/* txtRetencionesIVA arma el archivo de retenciones de IVA.
 *
 * Una línea por comprobante, campos separados por «|», SIN fila de encabezado:
 * un archivo de presentación es solo datos, y una fila de títulos lo haría
 * rechazar. Por eso COLUMNAS_TXT_IVA se muestra en la pantalla.
 *
 * Termina con salto de línea: varios lectores descartan la última línea si no lo
 * tiene. */
export function txtRetencionesIVA(retenciones, empresa, anio, mes) {
  const rifAgente = texto(empresa?.rif)
  const periodo = periodoDe(anio, mes)

  const lineas = (retenciones || []).map((r) => [
    rifAgente,
    periodo,
    texto(r.terceroRif),
    texto(r.numeroComprobante),
    fechaSENIAT(r.fecha),
    // Tipo de documento: por ahora siempre factura. Cuál es el código que espera
    // el portal («01», «FAC»…) está en PENDIENTE_CONFIRMAR.
    '01',
    texto(r.documentoNumero),
    monto(r.base),
    monto(r.porcentaje),
    monto(r.montoRetenido),
  ].join(SEP))

  return lineas.length ? lineas.join('\r\n') + '\r\n' : ''
}

/* ---------------------------------------------------------------------------
 * XML — retenciones de ISLR
 * ------------------------------------------------------------------------- */

/* xmlRetencionesISLR arma el archivo de retenciones de ISLR.
 *
 * Estructura autodescriptiva: cada comprobante es un <Retencion> con sus campos
 * nombrados. Si la providencia pide otros nombres de etiqueta, se cambian acá —
 * el contenido es el mismo y está completo.
 *
 * Se incluye el SUSTRAENDO, que el TXT de IVA no lleva porque en IVA no existe:
 * en ISLR el impuesto es base×% MENOS el sustraendo de la tabla del reglamento,
 * y sin ese dato el monto retenido no se puede reconstruir ni auditar. */
export function xmlRetencionesISLR(retenciones, empresa, anio, mes) {
  const cuerpo = (retenciones || []).map((r) => [
    '    <Retencion>',
    `      <NumeroComprobante>${escaparXML(texto(r.numeroComprobante))}</NumeroComprobante>`,
    `      <FechaComprobante>${escaparXML(fechaSENIAT(r.fecha))}</FechaComprobante>`,
    `      <RifRetenido>${escaparXML(texto(r.terceroRif))}</RifRetenido>`,
    `      <NombreRetenido>${escaparXML(texto(r.terceroNombre))}</NombreRetenido>`,
    `      <Concepto>${escaparXML(texto(r.concepto))}</Concepto>`,
    `      <NumeroDocumento>${escaparXML(texto(r.documentoNumero))}</NumeroDocumento>`,
    `      <BaseGravable>${monto(r.base)}</BaseGravable>`,
    `      <Porcentaje>${monto(r.porcentaje)}</Porcentaje>`,
    `      <Sustraendo>${monto(r.sustraendo)}</Sustraendo>`,
    `      <ImpuestoRetenido>${monto(r.montoRetenido)}</ImpuestoRetenido>`,
    '    </Retencion>',
  ].join('\n')).join('\n')

  return [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<RelacionRetencionesISLR>',
    '  <Agente>',
    `    <Rif>${escaparXML(texto(empresa?.rif))}</Rif>`,
    `    <RazonSocial>${escaparXML(texto(empresa?.razonSocial || empresa?.nombre))}</RazonSocial>`,
    `    <Periodo>${periodoDe(anio, mes)}</Periodo>`,
    '  </Agente>',
    '  <Retenciones>',
    cuerpo,
    '  </Retenciones>',
    '</RelacionRetencionesISLR>',
    '',
  ].filter((l) => l !== '').join('\n')
}

/* ---------------------------------------------------------------------------
 * Descarga
 * ------------------------------------------------------------------------- */

/* descargarTexto dispara la descarga. Se separa del armado —igual que en
 * lib/xlsx.js— para que generar el archivo sea una función pura y se pueda
 * probar sin navegador.
 *
 * El BOM va deliberadamente FUERA: un archivo de presentación con BOM hace que
 * el primer campo llegue con basura adelante y el portal lo rechace. (En el XLSX
 * sí se usa, porque ahí lo lee Excel y no un validador.) */
export function descargarTexto(nombreArchivo, contenido, mime = 'text/plain;charset=utf-8') {
  const blob = new Blob([contenido], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = nombreArchivo
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

/* nombreArchivoRetenciones: RIF + período + impuesto. Lleva el RIF porque estos
 * archivos se acumulan en la carpeta de descargas mes a mes y de varias
 * empresas, y «retenciones.txt» tres veces no le sirve a nadie. */
export function nombreArchivoRetenciones(impuesto, empresa, anio, mes, extension) {
  const rif = texto(empresa?.rif).replace(/[^A-Za-z0-9]/g, '') || 'sinrif'
  return `retenciones-${impuesto}-${rif}-${periodoDe(anio, mes)}.${extension}`
}
