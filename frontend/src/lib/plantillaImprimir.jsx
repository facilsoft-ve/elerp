// Render e impresión de un documento (factura, cotización, …) usando una PLANTILLA
// de formato (la que el usuario diseñó en Configuración › Formatos). Una sola
// fuente de verdad para la vista previa en pantalla y para la impresión: ambas
// arman el MISMO HTML de bloques posicionados en milímetros; la previsualización
// solo lo escala con transform.
//
// El modelo `data` es el comprobante común de ComprobantePDF (comprobanteDeFactura
// / comprobanteDeCotizacion): { titulo, numero, numeroControl, fecha, moneda,
// contraparte:{nombre,documento,direccion,telefono,email}, lineas[], totales{},
// tasaCambio, notas, venceEl }. `empresa` trae el emisor (razón social, RIF, …).

import { fmtCurrency, fmtNum, fmtDate } from './format.js'

const PX_POR_MM = 96 / 25.4 // 1 mm ≈ 3.7795 px CSS

// Mapa del `tipo` del comprobante (ComprobantePDF) al tipo de formato (dominio).
// Solo los tipos con editor de formato conectado a impresión se listan; el resto
// (nota de crédito, anulación, compra) cae al comprobante por defecto.
const TIPO_A_FORMATO = { factura: 'factura', cotizacion: 'cotizacion' }

// resolverFormato elige, del catálogo de formatos del tenant (db.FORMATOS), la
// plantilla que una sede debe usar para un tipo de comprobante: primero la que
// tenga la sede asignada; si ninguna, la predeterminada del tipo; si tampoco, la
// primera activa. Devuelve null si no hay formato usable (⇒ comprobante por
// defecto). Espeja la lógica de ResolverPlantilla del backend.
export function resolverFormato(formatos, tipoComprobante, sedeId) {
  const tipo = TIPO_A_FORMATO[tipoComprobante]
  if (!tipo || !Array.isArray(formatos)) return null
  const cands = formatos.filter((p) => p.tipo === tipo && p.activa !== false)
  return cands.find((p) => (p.sedes || []).includes(sedeId)) || cands.find((p) => p.predeterminada) || cands[0] || null
}

// esc escapa el texto dinámico para no romper el HTML (nombres con &, <, …).
function esc(s) {
  return String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]))
}

// simboloMoneda devuelve el símbolo corto de una moneda para rótulos ("Bs", "US$").
export function simboloMoneda(m) {
  const c = (m || 'VES').toUpperCase()
  if (c === 'VES' || c === 'BS') return 'Bs'
  if (c === 'USD') return 'US$'
  if (c === 'EUR') return '€'
  return c
}

// valorCampo resuelve el texto de un bloque `campo` a partir del documento real.
function valorCampo(clave, data, empresa) {
  const c = data.contraparte || {}
  const t = data.totales || {}
  switch (clave) {
    case 'emisor.nombre': return empresa?.razonSocial || empresa?.nombre || ''
    case 'emisor.rif': return empresa?.rif ? `RIF: ${empresa.rif}` : ''
    case 'emisor.direccion': return empresa?.direccion || ''
    case 'emisor.telefono': return empresa?.telefono || ''
    case 'sede.nombre': return data.sedeNombre || empresa?.sedeNombre || ''
    case 'cliente.nombre': return c.nombre || ''
    case 'cliente.rif': return c.documento || ''
    case 'cliente.direccion': return c.direccion || ''
    case 'cliente.telefono': return c.telefono || ''
    case 'doc.tipo': return data.titulo || ''
    case 'doc.numero': return data.numero || ''
    case 'doc.numeroControl': return data.numeroControl || ''
    case 'doc.fecha': return data.fecha ? fmtDate(data.fecha) : ''
    case 'doc.vencimiento': return data.venceEl ? fmtDate(data.venceEl) : ''
    case 'doc.moneda': return data.moneda || ''
    case 'doc.tasa': return data.tasaCambio ? `${fmtCurrency(data.tasaCambio, 'VES')}/US$` : ''
    case 'doc.observaciones': return data.notas || data.terminos || ''
    case 'doc.hash': return data.hash || ''
    default: return ''
  }
}

// Contenido interno (HTML) de un bloque, según su tipo. La geometría (posición y
// tamaño) la pone el envoltorio; aquí solo va el contenido.
function contenidoBloque(b, data, empresa) {
  const moneda = data.moneda || 'VES'
  if (b.tipo === 'texto') return esc(b.texto || '')
  if (b.tipo === 'campo') return esc(valorCampo(b.campo, data, empresa))
  if (b.tipo === 'separador') return `<div style="border-top:${Math.max(0.2, b.h)}mm solid ${b.color || '#334155'};width:100%"></div>`
  if (b.tipo === 'logo') {
    if (empresa?.logo) return `<img src="${esc(empresa.logo)}" style="max-width:100%;max-height:100%;object-fit:contain" alt="" />`
    const nom = empresa?.razonSocial || empresa?.nombre || ''
    return `<div style="width:100%;height:100%;border:0.3mm dashed #94a3b8;display:flex;align-items:center;justify-content:center;color:#64748b;font-size:8pt;text-align:center;padding:1mm">${esc(nom) || 'LOGO'}</div>`
  }
  if (b.tipo === 'imagen') {
    return b.imagen ? `<img src="${esc(b.imagen)}" style="width:100%;height:100%;object-fit:contain;display:block" alt="" />` : ''
  }
  if (b.tipo === 'qr') {
    return `<div style="width:100%;height:100%;border:0.3mm solid #cbd5e1;display:flex;align-items:center;justify-content:center;color:#94a3b8;font-size:6pt">QR</div>`
  }
  if (b.tipo === 'tabla_items') {
    const o = b.opciones || {}
    const sym = simboloMoneda(moneda)
    const hayExento = (data.lineas || []).some((l) => l.exento)
    const marca = !!o.marcarExento && hayExento
    // "Indicar la moneda": el símbolo va en el encabezado y la celda queda limpia.
    const cel = (v) => (o.mostrarMoneda ? fmtNum(v) : fmtCurrency(v, moneda))
    const enc = (t) => (o.mostrarMoneda ? `${t} (${sym})` : t)
    const filas = (data.lineas || []).map((l) => `
      <tr style="border-bottom:0.2mm solid #e2e8f0">
        <td style="padding:0.4mm 1mm">${esc(l.descripcion || l.sku || '')}${marca && l.exento ? ' *' : ''}</td>
        <td style="padding:0.4mm 1mm;text-align:right">${fmtNum(l.cantidad)}</td>
        <td style="padding:0.4mm 1mm;text-align:right">${cel(l.precioUnitario)}</td>
        <td style="padding:0.4mm 1mm;text-align:right">${cel(l.importe)}</td>
      </tr>`).join('')
    const nota = marca ? `<div style="font-size:7pt;color:#5C6470;margin-top:0.6mm">* Exento de IVA</div>` : ''
    return `<table style="width:100%;border-collapse:collapse;font-size:8pt">
      <thead><tr style="border-bottom:0.3mm solid #334155;text-align:left">
        <th style="padding:0.4mm 1mm">Descripción</th>
        <th style="padding:0.4mm 1mm;text-align:right">Cant</th>
        <th style="padding:0.4mm 1mm;text-align:right">${enc('Precio')}</th>
        <th style="padding:0.4mm 1mm;text-align:right">${enc('Total')}</th>
      </tr></thead><tbody>${filas}</tbody></table>${nota}`
  }
  if (b.tipo === 'totales') {
    const o = b.opciones || {}
    const t = data.totales || {}
    const fila = (k, v, bold) => `<div style="display:flex;justify-content:space-between;${bold ? 'font-weight:700;border-top:0.3mm solid #334155;padding-top:0.6mm;margin-top:0.6mm' : ''}"><span>${k}</span><span>${fmtCurrency(v, moneda)}</span></div>`
    // La alícuota puede venir como fracción (0.16) o como porcentaje (16); se
    // normaliza a porcentaje para el rótulo.
    const pct = t.alicuotaIVA ? (t.alicuotaIVA <= 1 ? t.alicuotaIVA * 100 : t.alicuotaIVA) : 0
    let rows = ''
    if (t.exento > 0) rows += fila('Exento', t.exento)
    rows += fila('Subtotal', t.subtotal || 0)
    if (t.iva > 0) rows += fila(`IVA${pct ? ' ' + fmtNum(pct, 0) + '%' : ''}`, t.iva)
    // IGTF: se muestra siempre que aplique (pago en divisas). No necesita bandera.
    if (t.igtf > 0) rows += fila('IGTF 3%', t.igtf)
    rows += fila('Total', t.total || 0, true)
    // Líneas informativas opcionales: tasa de cambio y total referencial en divisa.
    const tasa = data.tasaCambio || 0
    const linea = (k, v) => `<div style="display:flex;justify-content:space-between;font-size:7.5pt;color:#5C6470;margin-top:0.5mm"><span>${k}</span><span>${v}</span></div>`
    let extra = ''
    if (o.mostrarTasa && tasa > 0) extra += linea('Tasa BCV', `${fmtCurrency(tasa, 'VES')}/US$`)
    if (o.mostrarDivisa && tasa > 0) extra += linea('Total US$ (ref.)', fmtCurrency((t.total || 0) / tasa, 'USD'))
    return `<div style="width:100%;font-size:8.5pt">${rows}${extra}</div>`
  }
  return ''
}

// bloquesHTML arma el HTML de todos los bloques posicionados en mm sobre el papel.
function bloquesHTML(plantilla, data, empresa) {
  return (plantilla.bloques || []).map((b) => {
    const align = b.alineacion === 'centro' ? 'center' : b.alineacion === 'derecha' ? 'right' : 'left'
    const estilo = [
      'position:absolute',
      `left:${b.x}mm`, `top:${b.y}mm`, `width:${b.w}mm`, `height:${b.h}mm`,
      'overflow:hidden',
      `text-align:${align}`,
      b.tamano ? `font-size:${b.tamano}pt` : '',
      b.negrita ? 'font-weight:700' : '',
      `color:${b.color || '#141118'}`,
      'line-height:1.2',
    ].filter(Boolean).join(';')
    return `<div style="${estilo}">${contenidoBloque(b, data, empresa)}</div>`
  }).join('')
}

// imprimirPlantilla abre el documento en una ventana nueva sizeada EXACTAMENTE al
// papel de la plantilla (@page size en mm) y dispara la impresión. Devuelve false
// si el navegador bloqueó el popup (para caer al respaldo del llamador).
export function imprimirPlantilla(plantilla, data, empresa, titulo = 'Documento') {
  const W = plantilla.anchoMm || 215.9
  const H = plantilla.altoMm || 279.4
  const html = bloquesHTML(plantilla, data, empresa)
  const w = window.open('', '_blank', 'width=820,height=1040')
  if (!w) return false
  // Copiar las hojas de estilo de la app para heredar las fuentes (Inter/Poppins/
  // IBM Plex Mono). Los bloques usan estilos en línea, así que Tailwind no compite.
  const estilos = Array.from(document.querySelectorAll('link[rel="stylesheet"], style'))
    .map((n) => n.outerHTML).join('')
  w.document.write(
    '<!doctype html><html><head><meta charset="utf-8"><title>' + esc(titulo) + '</title>' + estilos +
    '<style>@page{size:' + W + 'mm ' + H + 'mm;margin:0}' +
    '*{box-sizing:border-box}html,body{margin:0;background:#fff!important;-webkit-print-color-adjust:exact;print-color-adjust:exact;' +
    'font-family:Inter,system-ui,-apple-system,Segoe UI,Roboto,sans-serif;color:#141118}' +
    '.pg{position:relative;width:' + W + 'mm;height:' + H + 'mm;overflow:hidden;background:#fff}</style>' +
    '</head><body><div class="pg">' + html + '</div>' +
    '<scr' + 'ipt>window.onload=function(){setTimeout(function(){window.print()},350)}</scr' + 'ipt>' +
    '</body></html>')
  w.document.close()
  w.focus()
  return true
}

// PreviewPlantilla muestra la plantilla en pantalla (WYSIWYG), escalando el mismo
// HTML de impresión a un ancho en píxeles. Así lo que se ve es lo que se imprime.
export function PreviewPlantilla({ plantilla, data, empresa, anchoPx = 620 }) {
  const W = plantilla.anchoMm || 215.9
  const H = plantilla.altoMm || 279.4
  const scale = anchoPx / (W * PX_POR_MM)
  const html = bloquesHTML(plantilla, data, empresa)
  return (
    <div style={{ width: anchoPx, height: H * PX_POR_MM * scale, overflow: 'hidden', margin: '0 auto', background: '#fff', boxShadow: '0 1px 8px rgba(0,0,0,.12)' }}>
      <div style={{ width: W + 'mm', height: H + 'mm', position: 'relative', background: '#fff', transformOrigin: 'top left', transform: `scale(${scale})`, fontFamily: 'Inter, system-ui, sans-serif', color: '#141118' }}
        dangerouslySetInnerHTML={{ __html: html }} />
    </div>
  )
}
