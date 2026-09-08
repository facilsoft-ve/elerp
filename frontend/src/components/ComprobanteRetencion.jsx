import { useRef } from 'react'
import { createPortal } from 'react-dom'
import { Icon } from './Icon.jsx'
import { Wordmark } from './Logo.jsx'
import { Button } from './primitives.jsx'
import { fmtCurrency, fmtDate } from '../lib/format.js'
import { imprimirNodo } from '../lib/imprimir.js'

/* Comprobante de retención IMPRIMIBLE (IVA o ISLR) — para el que la empresa EMITE
 * a su proveedor. Papel: fondo blanco, tinta oscura, sin variantes dark:. Reutiliza
 * el portal .comprobante-print-root (ver index.css) para "Descargar / Imprimir PDF"
 * vía window.print(). El número ya viene generado por el backend (AAAAMM+secuencia).
 */

const IMP = { iva: 'IVA', islr: 'ISLR' }

function Hoja({ r, empresa }) {
  const emp = empresa || {}
  const periodo = (r.fecha || '').slice(0, 7) // AAAA-MM
  const dato = (label, value) => (
    <div>
      <div style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: '.04em', color: '#8B94A3' }}>{label}</div>
      <div style={{ fontSize: 13, color: '#1F2430', fontWeight: 500 }}>{value || '—'}</div>
    </div>
  )
  return (
    <div style={{ color: '#1F2430', fontSize: 13 }}>
      {/* Cabecera */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16, borderBottom: '2px solid #6A2CF0', paddingBottom: 12 }}>
        <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
          <Wordmark size={20} />
          <div>
            <div style={{ fontWeight: 700, fontSize: 15 }}>{emp.nombre || 'Empresa'}</div>
            <div style={{ fontSize: 12, color: '#5C6470' }}>RIF: {emp.rif || '—'}</div>
          </div>
        </div>
        <div style={{ textAlign: 'right' }}>
          <div style={{ fontWeight: 700, fontSize: 14, color: '#6A2CF0' }}>COMPROBANTE DE RETENCIÓN</div>
          <div style={{ fontSize: 12, color: '#5C6470' }}>Retención de {IMP[r.impuesto] || r.impuesto?.toUpperCase()}</div>
          <div style={{ marginTop: 4, fontFamily: 'monospace', fontSize: 14, fontWeight: 700 }}>{r.numeroComprobante}</div>
        </div>
      </div>

      {/* Agente y sujeto retenido */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 14, marginTop: 14 }}>
        <div style={{ border: '1px solid #E8EAEF', borderRadius: 8, padding: 10 }}>
          <div style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: '.05em', color: '#8B94A3', marginBottom: 4 }}>Agente de retención</div>
          <div style={{ fontWeight: 600 }}>{emp.nombre || '—'}</div>
          <div style={{ color: '#5C6470' }}>RIF: {emp.rif || '—'}</div>
        </div>
        <div style={{ border: '1px solid #E8EAEF', borderRadius: 8, padding: 10 }}>
          <div style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: '.05em', color: '#8B94A3', marginBottom: 4 }}>Sujeto retenido</div>
          <div style={{ fontWeight: 600 }}>{r.terceroNombre || '—'}</div>
          <div style={{ color: '#5C6470' }}>RIF: {r.terceroRif || '—'}</div>
        </div>
      </div>

      {/* Datos del período y documento */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12, marginTop: 14 }}>
        {dato('Período fiscal', periodo)}
        {dato('Fecha del comprobante', fmtDate(r.fecha))}
        {dato('Factura afectada', r.documentoNumero)}
        {r.impuesto === 'islr' ? dato('Concepto', r.concepto) : null}
      </div>

      {/* Detalle del cálculo */}
      <table style={{ width: '100%', marginTop: 16, borderCollapse: 'collapse', fontSize: 13 }}>
        <thead>
          <tr style={{ background: '#F5F7FA', color: '#5C6470' }}>
            <th style={{ textAlign: 'left', padding: '8px 10px', fontWeight: 600 }}>Base imponible</th>
            <th style={{ textAlign: 'right', padding: '8px 10px', fontWeight: 600 }}>%</th>
            {r.impuesto === 'islr' ? <th style={{ textAlign: 'right', padding: '8px 10px', fontWeight: 600 }}>Sustraendo</th> : null}
            <th style={{ textAlign: 'right', padding: '8px 10px', fontWeight: 600 }}>Monto retenido</th>
          </tr>
        </thead>
        <tbody>
          <tr style={{ borderBottom: '1px solid #E8EAEF' }}>
            <td style={{ padding: '10px', fontFamily: 'monospace' }}>{fmtCurrency(r.base, 'VES')}</td>
            <td style={{ padding: '10px', textAlign: 'right' }}>{r.porcentaje}%</td>
            {r.impuesto === 'islr' ? <td style={{ padding: '10px', textAlign: 'right', fontFamily: 'monospace' }}>{fmtCurrency(r.sustraendo || 0, 'VES')}</td> : null}
            <td style={{ padding: '10px', textAlign: 'right', fontFamily: 'monospace', fontWeight: 700 }}>{fmtCurrency(r.montoRetenido, 'VES')}</td>
          </tr>
        </tbody>
      </table>

      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 10 }}>
        <div style={{ borderTop: '2px solid #1F2430', paddingTop: 8, minWidth: 220, display: 'flex', justifyContent: 'space-between' }}>
          <span style={{ fontWeight: 600 }}>Total retenido</span>
          <span style={{ fontFamily: 'monospace', fontWeight: 700, fontSize: 15 }}>{fmtCurrency(r.montoRetenido, 'VES')}</span>
        </div>
      </div>

      <div style={{ marginTop: 28, display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 28 }}>
        <div style={{ borderTop: '1px solid #8B94A3', paddingTop: 6, textAlign: 'center', fontSize: 11, color: '#5C6470' }}>Agente de retención</div>
        <div style={{ borderTop: '1px solid #8B94A3', paddingTop: 6, textAlign: 'center', fontSize: 11, color: '#5C6470' }}>Sujeto retenido</div>
      </div>

      <div style={{ marginTop: 14, fontSize: 10.5, color: '#8B94A3', textAlign: 'center' }}>
        Comprobante de retención emitido conforme a las providencias del SENIAT. El monto retenido se enterará al Fisco Nacional en el plazo legal.
      </div>
    </div>
  )
}

export function ComprobanteRetencionModal({ open, onClose, retencion, empresa }) {
  const hojaRef = useRef(null)
  if (!open || !retencion) return null
  const descargar = () => {
    // Ventana nueva autocontenida → "Guardar como PDF". Si el popup se bloquea,
    // respaldo con la impresión clásica (portal .comprobante-print-root).
    if (!imprimirNodo(hojaRef.current, 'Comprobante de retención ' + retencion.numeroComprobante)) window.print()
  }
  return createPortal(
    <>
      <div className="fixed inset-0 z-[150] flex items-stretch justify-center p-4 sm:p-6 cmp-screen-only">
        <div className="absolute inset-0 bg-slate-900/40 backdrop-blur-sm" onClick={onClose} />
        <div className="relative w-full max-w-2xl bg-slate-100 dark:bg-slate-950 rounded-xl shadow-modal border border-slate-200 dark:border-slate-800 flex flex-col max-h-[92vh]">
          <div className="flex items-center gap-3 px-5 py-3 border-b border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 rounded-t-xl">
            <span className="h-9 w-9 rounded-lg bg-elerp-50 text-elerp-600 dark:bg-elerp-900/40 dark:text-elerp-300 inline-flex items-center justify-center"><Icon.Receipt size={18} /></span>
            <div className="flex-1 min-w-0">
              <div className="text-[15px] font-semibold tracking-tight truncate">Comprobante de retención · {retencion.numeroComprobante}</div>
              <div className="text-[12px] text-slate-500">Vista previa · descarga como PDF con “Imprimir”</div>
            </div>
            <button onClick={onClose} className="h-8 w-8 inline-flex items-center justify-center rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-500"><Icon.X size={16} /></button>
          </div>
          <div className="flex-1 overflow-auto p-5">
            <div ref={hojaRef} className="mx-auto bg-white shadow-card rounded-lg" style={{ maxWidth: 640, padding: 28 }}>
              <Hoja r={retencion} empresa={empresa} />
            </div>
          </div>
          <div className="px-5 py-3 border-t border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 rounded-b-xl flex items-center gap-2 justify-end">
            <Button variant="ghost" onClick={onClose}>Cerrar</Button>
            <Button icon={<Icon.Download size={16} />} onClick={descargar}>Descargar / Imprimir PDF</Button>
          </div>
        </div>
      </div>
      <div className="comprobante-print-root">
        <div style={{ padding: 4 }}><Hoja r={retencion} empresa={empresa} /></div>
      </div>
    </>,
    document.body,
  )
}
