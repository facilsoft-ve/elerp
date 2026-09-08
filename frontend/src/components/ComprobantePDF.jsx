import { useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { imprimirNodo } from '../lib/imprimir.js'
import { imprimirPlantilla, PreviewPlantilla, resolverFormato } from '../lib/plantillaImprimir.jsx'
import { getActiveSede } from '../lib/api.js'
import { useData } from '../context/DataContext.jsx'
import { Icon } from './Icon.jsx'
import { Wordmark } from './Logo.jsx'
import { Button, Field, Input, useToast } from './primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'

/* Comprobante imprimible REUTILIZABLE — factura fiscal, cotización y compra.
 *
 * Un solo componente parametrizado por `tipo` que renderiza el documento como un
 * comprobante limpio para PAPEL (fondo blanco, tinta oscura): no usa variantes
 * `dark:` a propósito, así se ve como papel tanto en la vista previa (incluso en
 * modo oscuro) como al imprimir.
 *
 * Vías de salida:
 *   · Descargar / Imprimir PDF → REAL, vía window.print(). El comprobante se
 *     duplica en un portal fuera de #root (.comprobante-print-root); la hoja de
 *     impresión de index.css oculta la app y deja SOLO el comprobante, y el
 *     navegador ofrece "Guardar como PDF". Un render de PDF del lado del servidor
 *     (con sello/QR de verificación) sería el siguiente paso opcional.
 *   · Enviar por correo → punto de integración HONESTO: el formulario valida y
 *     prepara el mensaje, pero no simula un envío. El despacho real se conecta con
 *     la integración de correo (Hubmy email), aún sin cablear en este entorno.
 *   · Copiar enlace → el enlace público de verificación llega con el PORTAL DE
 *     VERIFICACIÓN (aún no existe): no se inventa una URL que no resuelve.
 */

// Rótulos por tipo de comprobante.
const TITULO_TIPO = {
  factura: 'Factura',
  nota_credito: 'Nota de crédito',
  anulacion: 'Anulación',
  cotizacion: 'Cotización',
  compra: 'Orden de compra',
}

// ---- Builders: normalizan cada origen a un modelo común de comprobante ----
// Modelo: { tipo, titulo, numero, fecha, moneda, estado?, contraparteLabel,
//   contraparte:{nombre,documento,direccion,telefono,email}, meta:[{label,value}],
//   lineas:[{descripcion,sku,cantidad,precioUnitario,descuento?,exento?,importe}],
//   totales:{subtotal,exento,iva,igtf,total}, tasaCambio?, notas?, terminos? }

export function comprobanteDeFactura(doc, empresa, cliente) {
  return {
    tipo: 'factura',
    titulo: TITULO_TIPO[doc.tipo] || 'Factura',
    numero: doc.numeroCompleto || '—',
    numeroControl: doc.numeroControl || '',
    fecha: doc.fecha,
    moneda: 'VES',
    contraparteLabel: 'Cliente',
    contraparte: {
      nombre: doc.clienteNombre || 'Consumidor final',
      documento: doc.clienteDocumento || cliente?.documento || '',
      direccion: cliente?.direccion || '',
      telefono: cliente?.telefono || '',
      email: cliente?.email || '',
    },
    meta: [
      doc.serie ? { label: 'Serie', value: doc.serie } : null,
      doc.motivo ? { label: 'Motivo', value: doc.motivo } : null,
    ].filter(Boolean),
    lineas: (doc.lineas || []).map((l) => ({
      descripcion: l.nombre || l.sku,
      sku: l.sku,
      cantidad: Number(l.cantidad) || 0,
      precioUnitario: Number(l.precioUnitario) || 0,
      exento: !!l.exento,
      importe: Number(l.total) || 0,
    })),
    totales: {
      subtotal: Number(doc.subtotal) || 0,
      baseImponible: Number(doc.baseImponible) || 0,
      exento: Number(doc.baseExenta) || 0,
      iva: Number(doc.iva) || 0,
      alicuotaIVA: Number(doc.alicuotaIVA) || 0,
      igtf: Number(doc.igtf) || 0,
      total: Number(doc.total) || 0,
    },
    credito: !!doc.credito,
    venceEl: doc.venceEl || '',
    tasaCambio: Number(doc.tasaCambio) || 0,
    anulado: !!doc.anulado,
    // Sello de integridad (SHA-256 encadenado) que asigna el motor al emitir; lo
    // consume el campo dinámico "doc.hash" del formato. Vacío en documentos
    // anteriores a la activación del sello.
    hash: doc.hash || '',
  }
}

export function comprobanteDeCotizacion(cot, empresa, cliente) {
  return {
    tipo: 'cotizacion',
    titulo: 'Cotización',
    numero: cot.numeroCompleto || '—',
    fecha: cot.fecha,
    moneda: cot.moneda || 'VES',
    contraparteLabel: 'Cliente',
    contraparte: {
      nombre: cot.clienteNombre || 'Consumidor final',
      documento: cot.clienteDocumento || cliente?.documento || '',
      direccion: cot.direccionEntrega || cliente?.direccion || '',
      telefono: cliente?.telefono || '',
      email: cliente?.email || '',
    },
    meta: [
      cot.validez ? { label: 'Válida hasta', value: fmtDate(cot.validez) } : null,
      cot.condicionesPago ? { label: 'Condiciones de pago', value: cot.condicionesPago } : null,
    ].filter(Boolean),
    lineas: (cot.lineas || []).map((l) => ({
      descripcion: l.descripcion || l.nombre || l.sku,
      sku: l.sku,
      cantidad: Number(l.cantidad) || 0,
      precioUnitario: Number(l.precioUnitario) || 0,
      descuento: Number(l.descuento) || 0,
      exento: !!l.exento,
      importe: Number(l.total) || 0,
    })),
    totales: {
      subtotal: Number(cot.subtotal) || 0,
      exento: Number(cot.baseExenta) || 0,
      iva: Number(cot.iva) || 0,
      igtf: Number(cot.igtf) || 0,
      total: Number(cot.total) || 0,
    },
    notas: cot.notas || '',
    terminos: cot.terminos || '',
  }
}

export function comprobanteDeCompra(oc, empresa, proveedor, factura) {
  return {
    tipo: 'compra',
    titulo: factura ? 'Factura de compra' : 'Orden de compra',
    numero: oc.numeroCompleto || '—',
    fecha: oc.creada,
    moneda: oc.moneda || 'VES',
    contraparteLabel: 'Proveedor',
    contraparte: {
      nombre: oc.proveedorNombre || proveedor?.nombre || '—',
      documento: proveedor?.documento || oc.proveedorRif || '',
      direccion: proveedor?.direccion || '',
      telefono: proveedor?.telefono || '',
      email: proveedor?.email || '',
    },
    meta: [
      oc.condicionesPago ? { label: 'Condiciones de pago', value: oc.condicionesPago } : null,
      factura?.numeroFactura ? { label: 'Nº factura proveedor', value: factura.numeroFactura } : null,
      factura?.numeroControl ? { label: 'Nº de control', value: factura.numeroControl } : null,
    ].filter(Boolean),
    lineas: (oc.lineas || []).map((l) => ({
      descripcion: l.nombre || l.sku,
      sku: l.sku,
      cantidad: Number(l.cantidad) || 0,
      precioUnitario: Number(l.costoUnitario) || 0,
      exento: !!l.exento,
      importe: Number(l.total) || 0,
    })),
    totales: {
      subtotal: Number(oc.subtotal) || 0,
      exento: 0,
      iva: Number(oc.iva) || 0,
      igtf: 0,
      total: Number(oc.total) || 0,
    },
    notas: oc.notas || '',
  }
}

/* El comprobante en sí — presentacional puro. Pensado para PAPEL: colores claros
 * fijos (nada de `dark:`), tipografías de marca. Se usa dos veces por el modal: en
 * la vista previa (pantalla) y en el portal de impresión. */
export function ComprobanteDoc({ data, empresa }) {
  const ccy = data.moneda || 'VES'
  const hayDesc = (data.lineas || []).some((l) => Number(l.descuento) > 0)
  const t = data.totales || {}
  // Equivalente en divisa: solo si el documento guarda una tasa histórica y está
  // en Bs. Nunca se calcula "al vuelo" con la tasa de hoy (Art. 177: se usa la del
  // hecho imponible; acá solo se muestra la que trae el documento).
  const equivUsd = ccy === 'VES' && data.tasaCambio > 0 ? t.total / data.tasaCambio : 0
  const razon = empresa?.razonSocial || empresa?.nombre || 'Tu empresa'

  return (
    <div className="cmp-doc" style={{ background: '#FFFFFF', color: '#1F2430' }}>
      {/* Encabezado: datos de la empresa + título del documento */}
      <div className="flex items-start justify-between gap-6 pb-4" style={{ borderBottom: '2px solid #6A2CF0' }}>
        <div className="flex items-start gap-3 min-w-0">
          {empresa?.logo ? (
            <img src={empresa.logo} alt="" style={{ height: 44, width: 44, objectFit: 'contain' }}
              onError={(e) => { e.currentTarget.style.display = 'none' }} />
          ) : (
            <div style={{ marginTop: 2 }}><Wordmark size={22} /></div>
          )}
          <div className="min-w-0">
            <div className="font-display font-bold leading-tight" style={{ fontSize: 16, color: '#6A2CF0' }}>{razon}</div>
            {empresa?.rif ? <div className="num" style={{ fontSize: 12, color: '#5C6470' }}>RIF: {empresa.rif}</div> : null}
            {empresa?.direccion ? <div style={{ fontSize: 11.5, color: '#5C6470', maxWidth: 320 }}>{empresa.direccion}</div> : null}
            <div style={{ fontSize: 11.5, color: '#5C6470' }}>
              {[empresa?.telefono, empresa?.email].filter(Boolean).join(' · ')}
            </div>
          </div>
        </div>
        <div className="text-right shrink-0">
          <div className="font-display font-bold" style={{ fontSize: 18, color: '#1F2430' }}>{data.titulo}</div>
          <div className="num" style={{ fontSize: 14, color: '#6A2CF0', fontWeight: 600 }}>{data.numero}</div>
          {data.numeroControl ? (
            <div className="num" style={{ fontSize: 12, color: '#1F2430', fontWeight: 600 }}>N° de Control: {data.numeroControl}</div>
          ) : null}
          <div style={{ fontSize: 11.5, color: '#5C6470' }}>Fecha: {fmtDate(data.fecha)}</div>
          {data.anulado ? (
            <div style={{ marginTop: 4, display: 'inline-block', fontSize: 11, fontWeight: 700, color: '#B3362C', border: '1px solid #B3362C', borderRadius: 6, padding: '1px 8px' }}>ANULADA</div>
          ) : null}
        </div>
      </div>

      {/* Contraparte + metadatos */}
      <div className="grid grid-cols-2 gap-6 py-4">
        <div>
          <div style={{ fontSize: 10.5, letterSpacing: '.04em', textTransform: 'uppercase', color: '#8B94A3', marginBottom: 3 }}>{data.contraparteLabel}</div>
          <div className="font-medium" style={{ fontSize: 13.5 }}>{data.contraparte.nombre}</div>
          {data.contraparte.documento ? <div className="num" style={{ fontSize: 12, color: '#5C6470' }}>{data.contraparte.documento}</div> : null}
          {data.contraparte.direccion ? <div style={{ fontSize: 11.5, color: '#5C6470' }}>{data.contraparte.direccion}</div> : null}
          {(data.contraparte.telefono || data.contraparte.email) ? (
            <div style={{ fontSize: 11.5, color: '#5C6470' }}>{[data.contraparte.telefono, data.contraparte.email].filter(Boolean).join(' · ')}</div>
          ) : null}
        </div>
        {(data.meta || []).length ? (
          <div className="text-right">
            {data.meta.map((m, i) => (
              <div key={i} style={{ fontSize: 12, color: '#5C6470' }}>
                <span style={{ color: '#8B94A3' }}>{m.label}: </span>
                <span className="num" style={{ color: '#1F2430' }}>{m.value}</span>
              </div>
            ))}
          </div>
        ) : null}
      </div>

      {/* Tabla de líneas */}
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
        <thead>
          <tr style={{ textAlign: 'left', color: '#5C6470', borderBottom: '1px solid #E8EAEF' }}>
            <th style={{ padding: '6px 8px', fontWeight: 600, textAlign: 'center', width: 54 }}>Cant.</th>
            <th style={{ padding: '6px 8px', fontWeight: 600 }}>Descripción</th>
            <th style={{ padding: '6px 8px', fontWeight: 600, textAlign: 'right', width: 100 }}>Precio</th>
            {hayDesc ? <th style={{ padding: '6px 8px', fontWeight: 600, textAlign: 'center', width: 60 }}>Desc.</th> : null}
            <th style={{ padding: '6px 8px', fontWeight: 600, textAlign: 'right', width: 110 }}>Importe</th>
          </tr>
        </thead>
        <tbody>
          {(data.lineas || []).map((l, i) => (
            <tr key={i} style={{ borderBottom: '1px solid #F0F2EF' }}>
              <td className="num" style={{ padding: '6px 8px', textAlign: 'center' }}>{fmtNum(l.cantidad)}</td>
              <td style={{ padding: '6px 8px' }}>
                <div style={{ fontWeight: 500 }}>{l.descripcion}</div>
                <div className="num" style={{ fontSize: 10.5, color: '#8B94A3' }}>{l.sku}{l.exento ? ' · exento de IVA' : ''}</div>
              </td>
              <td className="num" style={{ padding: '6px 8px', textAlign: 'right' }}>{fmtCurrency(l.precioUnitario, ccy)}</td>
              {hayDesc ? <td className="num" style={{ padding: '6px 8px', textAlign: 'center' }}>{Number(l.descuento) > 0 ? `${fmtNum(l.descuento, 0)}%` : '—'}</td> : null}
              <td className="num" style={{ padding: '6px 8px', textAlign: 'right', fontWeight: 500 }}>{fmtCurrency(l.importe, ccy)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* Totales */}
      <div className="flex justify-end pt-4">
        <div style={{ width: 260 }}>
          {(() => {
            const baseImp = t.baseImponible > 0 ? t.baseImponible : Math.max(0, t.subtotal - t.exento)
            const pct = t.alicuotaIVA > 0 ? Math.round(t.alicuotaIVA * 100) : (t.iva > 0 && baseImp > 0 ? Math.round((t.iva / baseImp) * 100) : 0)
            return (
              <>
                <TotLine label="Base imponible" value={baseImp} ccy={ccy} />
                {t.exento > 0 ? <TotLine label="Base exenta" value={t.exento} ccy={ccy} /> : null}
                <TotLine label={`IVA${pct ? ` (${pct}%)` : ''}`} value={t.iva} ccy={ccy} />
                {t.igtf > 0 ? <TotLine label="IGTF (3%)" value={t.igtf} ccy={ccy} /> : null}
              </>
            )
          })()}
          <div className="flex items-center justify-between" style={{ borderTop: '2px solid #6A2CF0', marginTop: 4, paddingTop: 6 }}>
            <span className="font-display font-bold" style={{ fontSize: 14 }}>Total</span>
            <span className="num font-bold" style={{ fontSize: 16, color: '#6A2CF0' }}>{fmtCurrency(t.total, ccy)}</span>
          </div>
          <div style={{ marginTop: 5, fontSize: 11, color: '#5C6470' }}>
            Condición: {data.credito ? `Crédito${data.venceEl ? ' · vence ' + fmtDate(data.venceEl) : ''}` : 'Contado'}
          </div>
          {equivUsd > 0 ? (
            <div className="flex items-center justify-between" style={{ marginTop: 3 }}>
              <span style={{ fontSize: 11, color: '#8B94A3' }}>Equivalente</span>
              <span className="num" style={{ fontSize: 12, color: '#5C6470' }}>≈ {fmtCurrency(equivUsd, 'USD')} · tasa {fmtCurrency(data.tasaCambio, 'VES')}/$</span>
            </div>
          ) : null}
        </div>
      </div>

      {/* Pie del formato: todo lo que va DEBAJO del total (términos y condiciones,
          notas y la línea de marca) se ancla al pie de la hoja (margin-top:auto,
          con .cmp-doc como columna flex de alto de página). */}
      <div style={{ marginTop: 'auto' }}>
      {/* Términos / notas */}
      {(data.terminos || data.notas) ? (
        <div style={{ marginTop: 18, borderTop: '1px solid #E8EAEF', paddingTop: 10 }}>
          {data.terminos ? (
            <div style={{ marginBottom: 6 }}>
              <div style={{ fontSize: 10.5, letterSpacing: '.04em', textTransform: 'uppercase', color: '#8B94A3', marginBottom: 2 }}>Términos y condiciones</div>
              <div style={{ fontSize: 11.5, color: '#5C6470', whiteSpace: 'pre-wrap' }}>{data.terminos}</div>
            </div>
          ) : null}
          {data.notas ? (
            <div>
              <div style={{ fontSize: 10.5, letterSpacing: '.04em', textTransform: 'uppercase', color: '#8B94A3', marginBottom: 2 }}>Notas</div>
              <div style={{ fontSize: 11.5, color: '#5C6470', whiteSpace: 'pre-wrap' }}>{data.notas}</div>
            </div>
          ) : null}
        </div>
      ) : null}

      {/* Línea de marca (discreta, para papel) */}
      <div className="flex items-center justify-between" style={{ marginTop: 22, borderTop: '1px solid #E8EAEF', paddingTop: 8 }}>
        <div className="flex items-center gap-1.5" style={{ fontSize: 10.5, color: '#8B94A3' }}>
          <span style={{ width: 6, height: 6, borderRadius: 999, background: '#6A2CF0', display: 'inline-block' }} />
          Generado con ElERP
        </div>
        <div style={{ fontSize: 10.5, color: '#8B94A3' }}>
          {data.tipo === 'cotizacion' ? 'Documento sin efecto fiscal' : data.tipo === 'compra' ? 'Documento interno de compra' : 'Documento fiscal'}
        </div>
      </div>
      </div>{/* fin del pie anclado */}
    </div>
  )
}

const TotLine = ({ label, value, ccy }) => (
  <div className="flex items-center justify-between" style={{ padding: '2px 0' }}>
    <span style={{ fontSize: 12, color: '#5C6470' }}>{label}</span>
    <span className="num" style={{ fontSize: 12.5 }}>{fmtCurrency(value, ccy)}</span>
  </div>
)

/* Modal de previsualización + acciones. Es un modal a medida (no el `Modal` de
 * primitives) porque necesita ser ancho como una hoja y contener el portal de
 * impresión. */
export function ComprobanteModal({ open, onClose, data, empresa, plantilla: plantillaProp }) {
  const toast = useToast()
  const { db } = useData()
  const [correoAbierto, setCorreoAbierto] = useState(false)
  const hojaRef = useRef(null)
  // Formato a usar: el que pase el llamador, o el resuelto para el tipo del
  // documento y la sede activa (db.FORMATOS viene en el bootstrap). Null ⇒ se
  // imprime el comprobante por defecto.
  const plantilla = plantillaProp || (data ? resolverFormato(db?.FORMATOS, data.tipo, getActiveSede()) : null)
  if (!open || !data) return null

  const imprimir = () => {
    // Con un FORMATO asignado (plantilla), el documento se imprime EXACTAMENTE
    // como se diseñó en Configuración › Formatos: ventana nueva sizeada al papel
    // (@page en mm) con los bloques en su posición. Sin formato, se usa el
    // comprobante por defecto. En ambos casos, si el navegador bloquea el popup,
    // se cae a window.print() (portal .comprobante-print-root).
    const titulo = `${data.titulo} ${data.numero}`
    const ok = plantilla ? imprimirPlantilla(plantilla, data, empresa, titulo) : imprimirNodo(hojaRef.current, titulo)
    if (!ok) window.print()
  }

  const copiarEnlace = () => {
    // Honestidad: el portal público de verificación aún no existe, así que NO hay
    // una URL que resuelva. No se inventa ni se copia una falsa.
    toast({
      title: 'Enlace de verificación no disponible',
      body: 'El enlace público para verificar el documento llega con el portal de verificación (aún no publicado).',
      kind: 'warn',
    })
  }

  return createPortal(
    <>
      {/* Vista previa en pantalla */}
      <div className="fixed inset-0 z-[150] flex items-stretch justify-center p-4 sm:p-6 cmp-screen-only">
        <div className="absolute inset-0 bg-slate-900/40 backdrop-blur-sm" onClick={onClose} />
        <div className="relative w-full max-w-3xl bg-slate-100 dark:bg-slate-950 rounded-xl shadow-modal border border-slate-200 dark:border-slate-800 flex flex-col max-h-[92vh]">
          {/* Barra superior */}
          <div className="flex items-center gap-3 px-5 py-3 border-b border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 rounded-t-xl">
            <span className="h-9 w-9 rounded-lg bg-elerp-50 text-elerp-600 dark:bg-elerp-900/40 dark:text-elerp-300 inline-flex items-center justify-center"><Icon.Receipt size={18} /></span>
            <div className="flex-1 min-w-0">
              <div className="text-[15px] font-semibold tracking-tight truncate">{data.titulo} · {data.numero}</div>
              <div className="text-[12px] text-slate-500">{plantilla ? `Formato: ${plantilla.nombre}` : 'Vista previa'} · descarga como PDF con “Imprimir”</div>
            </div>
            <button onClick={onClose} className="h-8 w-8 inline-flex items-center justify-center rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-500"><Icon.X size={16} /></button>
          </div>

          {/* Cuerpo desplazable con el comprobante como "hoja" */}
          <div className="flex-1 overflow-auto p-5">
            {plantilla ? (
              <PreviewPlantilla plantilla={plantilla} data={data} empresa={empresa} anchoPx={600} />
            ) : (
              <div ref={hojaRef} className="mx-auto bg-white shadow-card rounded-lg" style={{ maxWidth: 640, padding: 28 }}>
                <ComprobanteDoc data={data} empresa={empresa} />
              </div>
            )}

            {correoAbierto ? <CorreoForm data={data} onClose={() => setCorreoAbierto(false)} toast={toast} /> : null}
          </div>

          {/* Acciones */}
          <div className="px-5 py-3 border-t border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 rounded-b-xl flex items-center gap-2 flex-wrap justify-end">
            <Button variant="ghost" icon={<Icon.Link size={16} />} onClick={copiarEnlace}>Copiar enlace</Button>
            <Button variant="secondary" icon={<Icon.Mail size={16} />} onClick={() => setCorreoAbierto((v) => !v)}>Enviar por correo</Button>
            <Button icon={<Icon.Download size={16} />} onClick={imprimir}>Descargar / Imprimir PDF</Button>
          </div>
        </div>
      </div>

      {/* Copia para impresión: vive FUERA de #root; index.css la muestra solo al
          imprimir (oculta la app). No es visible en pantalla. */}
      <div className="comprobante-print-root">
        <div style={{ padding: 4 }}><ComprobanteDoc data={data} empresa={empresa} /></div>
      </div>
    </>,
    document.body,
  )
}

/* Formulario de envío por correo — punto de integración honesto. Valida y prepara
 * el mensaje; NO simula un envío. El despacho real se conecta con la integración
 * de correo (Hubmy email), aún sin cablear. */
function CorreoForm({ data, onClose, toast }) {
  const [para, setPara] = useState(data.contraparte?.email || '')
  const [asunto, setAsunto] = useState(`${data.titulo} ${data.numero}`)
  const [mensaje, setMensaje] = useState(`Adjunto el documento ${data.titulo.toLowerCase()} ${data.numero}. Cualquier consulta, quedamos a la orden.`)
  const [touched, setTouched] = useState(false)
  const emailOk = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(para.trim())
  const err = !para.trim() ? 'Indica el correo del destinatario.' : !emailOk ? 'El correo no tiene un formato válido.' : ''

  const preparar = () => {
    setTouched(true)
    if (err) return
    toast({
      title: 'Correo listo para enviar',
      body: `Destinatario ${para.trim()}. El despacho se conecta con la integración de correo (Hubmy), aún sin cablear en este entorno.`,
      kind: 'warn',
    })
  }

  return (
    <div className="mx-auto mt-4 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4" style={{ maxWidth: 640 }}>
      <div className="flex items-center gap-2 mb-3">
        <Icon.Mail size={16} className="text-elerp-600" />
        <div className="text-[14px] font-semibold flex-1">Enviar por correo</div>
        <button onClick={onClose} className="h-7 w-7 inline-flex items-center justify-center rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-500"><Icon.X size={15} /></button>
      </div>
      <div className="space-y-3">
        <Field label="Destinatario" required error={touched ? err : ''}>
          <Input type="email" value={para} onChange={(e) => setPara(e.target.value)} onBlur={() => setTouched(true)}
            invalid={touched && !!err} placeholder="correo@ejemplo.com" />
        </Field>
        <Field label="Asunto">
          <Input value={asunto} onChange={(e) => setAsunto(e.target.value)} />
        </Field>
        <Field label="Mensaje">
          <textarea value={mensaje} onChange={(e) => setMensaje(e.target.value)} rows={3}
            className="w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm ring-focus resize-y" />
        </Field>
        <div className="flex items-start gap-2 text-[12px] text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/20 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>El envío por correo se conecta con la integración de correo (Hubmy email). En este entorno aún no está cableado: el mensaje se prepara pero no se despacha.</span>
        </div>
        <div className="flex items-center justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>Cerrar</Button>
          <Button icon={<Icon.Send size={15} />} onClick={preparar}>Preparar envío</Button>
        </div>
      </div>
    </div>
  )
}
