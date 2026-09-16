import { useState, useMemo, useEffect, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, Segmented, VistaDetalle, Modal, Empty, useToast, Field, Toggle, TableSkeleton } from '../components/primitives.jsx'
import { TablaDatos } from '../components/TablaDatos.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { metodoLabel, IVA_TASA } from '../lib/fiscal.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { ComprobanteModal, comprobanteDeFactura } from '../components/ComprobantePDF.jsx'

// Anular (reversa total), nota de crédito (reversa parcial, resta) y nota de
// débito (cargo adicional, suma) son acciones sensibles: solo Dueña/Desarrollador.
const puedeAnular = (rol) => ['dueno', 'desarrollador'].includes(rol)

// Registrar la retención RECIBIDA es materia de cumplimiento fiscal, no del
// mostrador: mismo gate que ya aplica el servidor en /fiscal/documentos/:id/
// retencion-recibida (Dueña/Desarrollador/Contadora). La UI solo oculta; el
// backend es el que protege.
const puedeRetener = (rol) => ['dueno', 'desarrollador', 'contadora'].includes(rol)

// Porcentajes típicos de retención de IVA en Venezuela.
const PCT_RETENCION = [75, 100]
const IMPUESTO_LABEL = { iva: 'IVA', islr: 'ISLR' }
const hoyISO = () => new Date().toISOString().slice(0, 10)
const round2n = (v) => Math.round((Number(v) || 0) * 100) / 100

const TIPO_META = {
  factura: { label: 'Factura', color: 'huberp' },
  nota_credito: { label: 'Nota de crédito', color: 'amber' },
  nota_debito: { label: 'Nota de débito', color: 'sky' },
  anulacion: { label: 'Anulación', color: 'slate' },
}

// Origen de un documento (derivado, sin backend): si trae caja (código o id) salió
// del Punto de venta; si no, se emitió desde Ventas (forma libre / cotización
// facturada). Las notas de crédito y anulaciones heredan el mismo criterio por sus
// propios campos de caja. El origen y el tipo son dos ejes distintos, con paletas
// que no chocan: violet = POS, teal = Ventas (los tipos usan huberp/amber/slate).
const esPOS = (d) => !!((d.cajaCodigo && String(d.cajaCodigo).trim()) || (d.cajaId && String(d.cajaId).trim()))
const origenDe = (d) => (esPOS(d) ? 'pos' : 'ventas')

const ORIGEN_META = {
  pos: { label: 'Punto de venta', color: 'violet' },
  ventas: { label: 'Ventas', color: 'teal' },
}

// Segmentos-filtro por ORIGEN (estilo Odoo), con su copy de vacío por segmento.
const ORIGEN_SEG = [
  { value: 'todos', label: 'Todos', origen: null, vacio: 'Aún no hay documentos' },
  { value: 'pos', label: 'Punto de venta', origen: 'pos', vacio: 'No hay facturas de Punto de venta en este filtro' },
  { value: 'ventas', label: 'Ventas', origen: 'ventas', vacio: 'No hay facturas de Ventas en este filtro' },
]

// Documentos fiscales: tabla append-only. Los documentos NO se editan; una
// anulación crea un documento de reversa que referencia al original.
// Rótulo del origen de la tasa guardada en el documento.
const ETIQUETA_FUENTE = {
  bcv: 'BCV', respaldo: 'BCV (respaldo)', mercado: 'promedio del mercado',
  manual: 'tasa manual', semilla: 'dato de demostración',
}

// `tipo` (opcional) restringe la vista a un tipo de documento — 'factura' o
// 'nota_credito' — para las sub-vistas del menú de Facturación (Facturas / Notas
// de crédito). Sin `tipo`, muestra todos los documentos como antes.
export function Documentos({ tipo = null }) {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const documentos = db.DOCUMENTOS
  const clientes = db.CLIENTES || []

  const [q, setQ] = useState('')
  const [origen, setOrigen] = useState('todos')  // todos | pos | ventas
  const [filtro, setFiltro] = useState('todos')  // todos | emitidas | anuladas
  const [detalle, setDetalle] = useState(null)
  const [anular, setAnular] = useState(null)
  const [notaCredito, setNotaCredito] = useState(null)
  const [notaDebito, setNotaDebito] = useState(null)
  const [nuevaND, setNuevaND] = useState(false) // selector de factura para una ND desde la sección
  const [pdf, setPdf] = useState(null)  // documento a previsualizar como PDF

  // Facturas emitidas (no anuladas) sobre las que se puede emitir una nota de
  // débito — fuente del selector "Nueva nota de débito" de la sección.
  const facturasParaND = useMemo(
    () => (documentos || []).filter((d) => d.tipo === 'factura' && !d.anulado),
    [documentos],
  )

  const clienteName = (id, fallback) => clientes.find((c) => c.id === id)?.nombre || fallback || 'Consumidor final'

  // Contador por origen, calculado sobre TODOS los documentos (no sobre el filtro
  // activo): el chip muestra siempre cuántos vinieron de cada canal.
  // Base restringida por `tipo` (Facturas / Notas de crédito) si el contenedor lo pide.
  const baseDocs = useMemo(() => (documentos || []).filter((d) => !tipo || d.tipo === tipo), [documentos, tipo])

  const conteos = useMemo(() => {
    const c = { todos: 0, pos: 0, ventas: 0 }
    for (const d of baseDocs) {
      c.todos += 1
      c[origenDe(d)] += 1
    }
    return c
  }, [baseDocs])

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return baseDocs.filter((d) => {
      const nombre = (d.clienteNombre || '').toLowerCase()
      const okQ = !term || (d.numeroCompleto || '').toLowerCase().includes(term) || nombre.includes(term) || (d.clienteDocumento || '').toLowerCase().includes(term)
      const okF = filtro === 'todos' || (filtro === 'anuladas' ? d.anulado : !d.anulado)
      const okO = origen === 'todos' || origenDe(d) === origen
      return okQ && okF && okO
    })
  }, [baseDocs, q, filtro, origen])

  const segActivo = ORIGEN_SEG.find((s) => s.value === origen) || ORIGEN_SEG[0]
  const hayFiltro = !!q || filtro !== 'todos' || origen !== 'todos'

  const totalEmitido = baseDocs.filter((d) => (tipo ? d.tipo === tipo : d.tipo === 'factura') && !d.anulado).reduce((a, d) => a + (Number(d.total) || 0), 0)

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar los documentos"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  // Detalle a PANTALLA COMPLETA (estilo Odoo): reemplaza la lista y ocupa el ancho
  // principal, con «‹ Volver» para regresar. Las acciones (PDF, anular, nota de
  // crédito) van arriba; los modales se abren sobre esta vista o sobre la lista.
  if (detalle) {
    return (
      <div>
        <DetalleDocumento doc={detalle} clienteName={clienteName} onVolver={() => setDetalle(null)}
          onPDF={() => setPdf(detalle)} ccy={ui.ccy} rol={ui.rol}
          // Saltar a un documento relacionado: se busca en la lista ya cargada y
          // se reemplaza el detalle, para poder recorrer la traza sin volver atrás.
          onAbrirDoc={(id) => {
            const otro = (documentos || []).find((x) => x.id === id)
            if (otro) setDetalle(otro)
          }}
          onAnular={puedeAnular(ui.rol) ? (d) => { setDetalle(null); setAnular(d) } : null}
          onNotaCredito={puedeAnular(ui.rol) ? (d) => { setDetalle(null); setNotaCredito(d) } : null}
          onNotaDebito={puedeAnular(ui.rol) ? (d) => { setDetalle(null); setNotaDebito(d) } : null} />
        {anular ? <AnularModal doc={anular} onClose={() => setAnular(null)} onSaved={reload} toast={toast} /> : null}
        {notaCredito ? <NotaCreditoModal doc={notaCredito} onClose={() => setNotaCredito(null)} onSaved={reload} toast={toast} ccy={ui.ccy} /> : null}
        {notaDebito ? <NotaDebitoModal doc={notaDebito} onClose={() => setNotaDebito(null)} onSaved={reload} toast={toast} ccy={ui.ccy} /> : null}
        {pdf ? (
          <ComprobanteModal open onClose={() => setPdf(null)} empresa={db.EMPRESA}
            data={comprobanteDeFactura(pdf, db.EMPRESA, clientes.find((c) => c.id === pdf.clienteId))} />
        ) : null}
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        {/* Segmentos-filtro por ORIGEN (POS vs. Ventas), con contador por segmento. */}
        <div className="inline-flex items-center gap-1 p-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/70 overflow-x-auto">
          {ORIGEN_SEG.map((s) => {
            const activo = origen === s.value
            const n = conteos[s.value] || 0
            return (
              <button key={s.value} onClick={() => setOrigen(s.value)}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[12.5px] font-semibold whitespace-nowrap transition-colors ${activo ? 'bg-white dark:bg-slate-900 shadow-sm text-slate-900 dark:text-slate-100' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'}`}>
                {s.label}
                <span className={`num text-[10.5px] leading-none px-1.5 py-0.5 rounded-full ${activo ? 'bg-elerp-50 text-elerp-600 dark:bg-elerp-900/50 dark:text-elerp-200' : 'bg-slate-200 text-slate-500 dark:bg-slate-700 dark:text-slate-300'}`}>{fmtNum(n, 0)}</span>
              </button>
            )
          })}
        </div>
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por número, cliente o documento…" value={q} onChange={(e) => setQ(e.target.value)} />
        <Segmented size="sm" value={filtro} onChange={setFiltro}
          options={[{ value: 'todos', label: 'Todos' }, { value: 'emitidas', label: 'Emitidas' }, { value: 'anuladas', label: 'Anuladas' }]} />
        <div className="ml-auto flex items-center gap-3">
          <div className="text-[12px] text-slate-500 hidden sm:block">Facturado vigente: <span className="num font-medium text-slate-700 dark:text-slate-200 private-mask">{fmtCurrency(totalEmitido, ui.ccy, { max: 0 })}</span></div>
          {/* Alta de nota de débito desde su propia sección (además de hacerlo desde
              la factura): abre un selector de la factura de origen y luego el modal. */}
          {tipo === 'nota_debito' && puedeAnular(ui.rol) ? (
            <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setNuevaND(true)}>Nueva nota de débito</Button>
          ) : null}
        </div>
      </div>

      <TablaDatos
        rows={rows}
        loading={loading || documentos === undefined}
        rowKey={(d) => d.id}
        onRowClick={(d) => setDetalle(d)}
        rowClassName={(d) => (d.anulado ? 'opacity-60' : '')}
        csvName={`documentos${tipo ? '-' + tipo : ''}`}
        initialSort={{ key: 'fecha', dir: -1 }}
        footerLabel={(n) => `${fmtNum(n, 0)} documento(s)`}
        empty={<Empty icon={<Icon.Receipt size={22} />}
          title={q ? 'Sin resultados' : segActivo.vacio}
          body={q ? 'Prueba con otro término o filtro.'
            : hayFiltro ? 'Prueba con otro segmento o filtro.'
            : 'Emite tu primera factura desde el Punto de venta o desde Ventas.'} />}
        columns={[
          {
            key: 'numero', header: 'Número', sortable: true,
            sortValue: (d) => d.numeroCompleto || '', tdClassName: 'num text-[12.5px] font-medium',
            cell: (d) => d.numeroCompleto,
          },
          {
            key: 'tipo', header: 'Tipo', sortable: true,
            sortValue: (d) => (TIPO_META[d.tipo]?.label || d.tipo || ''),
            csv: (d) => (TIPO_META[d.tipo]?.label || d.tipo || ''),
            cell: (d) => { const tm = TIPO_META[d.tipo] || { label: d.tipo, color: 'slate' }; return <Badge size="sm" color={tm.color}>{tm.label}</Badge> },
          },
          {
            key: 'origen', header: 'Origen', sortable: true,
            sortValue: (d) => ORIGEN_META[origenDe(d)].label,
            csv: (d) => ORIGEN_META[origenDe(d)].label,
            cell: (d) => {
              const om = ORIGEN_META[origenDe(d)]
              const pos = origenDe(d) === 'pos'
              return (
                <>
                  <Badge size="sm" color={om.color}>{om.label}</Badge>
                  {pos ? (
                    <div className="text-[11px] text-slate-500 num mt-0.5 truncate max-w-[160px]">
                      {[d.cajaCodigo, d.cajeroNombre].filter(Boolean).join(' · ')}
                    </div>
                  ) : (
                    <div className="text-[11px] text-slate-500 mt-0.5">forma libre</div>
                  )}
                </>
              )
            },
          },
          {
            key: 'cliente', header: 'Cliente', sortable: true,
            sortValue: (d) => d.clienteNombre || 'Consumidor final',
            csv: (d) => `${d.clienteNombre || 'Consumidor final'}${d.clienteDocumento ? ' · ' + d.clienteDocumento : ''}`,
            cell: (d) => (
              <>
                <div className="text-[13px] truncate max-w-[180px]">{d.clienteNombre || 'Consumidor final'}</div>
                {d.clienteDocumento ? <div className="text-[11px] text-slate-500 num">{d.clienteDocumento}</div> : null}
              </>
            ),
          },
          {
            key: 'fecha', header: 'Fecha', sortable: true,
            sortValue: (d) => d.fecha || '', csv: (d) => fmtDate(d.fecha),
            tdClassName: 'whitespace-nowrap text-[12.5px] text-slate-500 num',
            cell: (d) => fmtDate(d.fecha),
          },
          {
            key: 'total', header: 'Total', align: 'right', sortable: true,
            sortValue: (d) => Number(d.total) || 0, csv: (d) => Number(d.total) || 0,
            tdClassName: 'num font-medium private-mask', cell: (d) => fmtCurrency(d.total, ui.ccy),
          },
          {
            key: 'estado', header: 'Estado', align: 'center',
            csv: (d) => (d.anulado ? 'Anulada' : d.tipo === 'anulacion' ? 'Reversa' : d.tipo === 'nota_credito' ? 'Crédito' : d.tipo === 'nota_debito' ? 'Débito' : 'Emitida'),
            cell: (d) => (
              d.anulado ? <Badge size="sm" color="red" dot>Anulada</Badge>
                : d.tipo === 'anulacion' ? <Badge size="sm" color="slate" dot>Reversa</Badge>
                : d.tipo === 'nota_credito' ? <Badge size="sm" color="amber" dot>Crédito</Badge>
                : d.tipo === 'nota_debito' ? <Badge size="sm" color="sky" dot>Débito</Badge>
                : <Badge size="sm" color="emerald" dot>Emitida</Badge>
            ),
          },
          {
            key: 'acciones', header: 'Acciones', align: 'right', stop: true, csv: false,
            cell: (d) => (
              puedeAnular(ui.rol) && d.tipo === 'factura' && !d.anulado ? (
                <div className="inline-flex items-center gap-1">
                  <button onClick={() => setNotaDebito(d)} title="Emitir nota de débito" aria-label={`Emitir nota de débito para ${d.numeroCompleto}`}
                    className="h-7 px-2 inline-flex items-center gap-1 rounded-lg text-[12px] text-sky-700 dark:text-sky-300 hover:bg-sky-50 dark:hover:bg-sky-900/20 ring-focus">
                    <Icon.Receipt size={14} /> Nota de débito
                  </button>
                  <button onClick={() => setNotaCredito(d)} title="Emitir nota de crédito" aria-label={`Emitir nota de crédito para ${d.numeroCompleto}`}
                    className="h-7 px-2 inline-flex items-center gap-1 rounded-lg text-[12px] text-[#B3362C] dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 ring-focus">
                    <Icon.Receipt size={14} /> Nota de crédito
                  </button>
                  <button onClick={() => setAnular(d)} title="Anular factura" aria-label={`Anular la factura ${d.numeroCompleto}`}
                    className="h-7 px-2 inline-flex items-center gap-1 rounded-lg text-[12px] text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 ring-focus">
                    <Icon.CircleX size={14} /> Anular
                  </button>
                </div>
              ) : <Icon.ChevRight size={15} className="inline text-slate-400" />
            ),
          },
        ]} />

      {anular ? <AnularModal doc={anular} onClose={() => setAnular(null)} onSaved={reload} toast={toast} /> : null}
      {notaCredito ? <NotaCreditoModal doc={notaCredito} onClose={() => setNotaCredito(null)} onSaved={reload} toast={toast} ccy={ui.ccy} /> : null}
      {notaDebito ? <NotaDebitoModal doc={notaDebito} onClose={() => setNotaDebito(null)} onSaved={reload} toast={toast} ccy={ui.ccy} /> : null}
      {nuevaND ? (
        <SelectorFacturaModal facturas={facturasParaND} clienteName={clienteName}
          onClose={() => setNuevaND(false)} onElegir={(f) => { setNuevaND(false); setNotaDebito(f) }} />
      ) : null}
      {pdf ? (
        <ComprobanteModal open onClose={() => setPdf(null)} empresa={db.EMPRESA}
          data={comprobanteDeFactura(pdf, db.EMPRESA, clientes.find((c) => c.id === pdf.clienteId))} />
      ) : null}
    </div>
  )
}

// Selector de la factura de origen para emitir una nota de débito desde la sección
// "Notas de débito" (además de hacerlo desde la propia factura). Lista las facturas
// emitidas no anuladas; al elegir una se abre el NotaDebitoModal sobre ella.
function SelectorFacturaModal({ facturas, clienteName, onClose, onElegir }) {
  const [q, setQ] = useState('')
  const term = q.trim().toLowerCase()
  const rows = useMemo(() => {
    if (!term) return facturas.slice(0, 30)
    return facturas.filter((d) =>
      (d.numeroCompleto || '').toLowerCase().includes(term)
      || (d.clienteNombre || '').toLowerCase().includes(term)
      || (d.clienteDocumento || '').toLowerCase().includes(term),
    ).slice(0, 30)
  }, [facturas, term])
  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Receipt size={18} />}
      title="Nueva nota de débito" sub="Elige la factura sobre la que se emite el cargo."
      footer={<Button variant="ghost" onClick={onClose}>Cancelar</Button>}>
      <div className="space-y-3">
        <Input icon={<Icon.Search size={15} />} placeholder="Buscar factura por número, cliente o documento…"
          value={q} onChange={(e) => setQ(e.target.value)} autoFocus />
        {rows.length === 0 ? (
          <div className="rounded-xl border border-dashed border-slate-300 dark:border-slate-700 px-4 py-6 text-center text-[13px] text-slate-500">
            {facturas.length === 0 ? 'No hay facturas emitidas sobre las que emitir una nota de débito.' : `Sin resultados para «${q}».`}
          </div>
        ) : (
          <div className="rounded-xl border border-slate-200 dark:border-slate-800 divide-y divide-slate-100 dark:divide-slate-800 max-h-[52vh] overflow-y-auto">
            {rows.map((d) => (
              <button key={d.id} onClick={() => onElegir(d)}
                className="w-full flex items-center gap-3 px-3.5 py-2.5 text-left hover:bg-slate-50 dark:hover:bg-slate-800/60 ring-focus">
                <div className="min-w-0 flex-1">
                  <div className="text-[13px] font-medium num truncate">{d.numeroCompleto}</div>
                  <div className="text-[12px] text-slate-500 truncate">
                    {clienteName(d.clienteId, d.clienteNombre)}{d.clienteDocumento ? ` · ${d.clienteDocumento}` : ''} · {fmtDate(d.fecha)}
                  </div>
                </div>
                <div className="num text-[13px] font-medium shrink-0 private-mask">{fmtCurrency(d.total, 'VES')}</div>
                <Icon.ChevRight size={15} className="text-slate-400 shrink-0" />
              </button>
            ))}
          </div>
        )}
      </div>
    </Modal>
  )
}

function DetalleDocumento({ doc, clienteName, onVolver, onPDF, onAnular, onNotaCredito, onNotaDebito, onAbrirDoc, ccy, rol }) {
  const toast = useToast()
  const tm = TIPO_META[doc.tipo] || { label: doc.tipo, color: 'slate' }
  // Traza del documento: de dónde viene y qué salió de él. Se pide al servidor
  // (no se deriva en la pantalla) porque incluye las retenciones, que no viven
  // en el `db` de documentos.
  const [rel, setRel] = useState(undefined)
  const [relErr, setRelErr] = useState(null)
  const [retener, setRetener] = useState(false)
  const cargarRel = useCallback(() => {
    setRel(undefined); setRelErr(null)
    api.documentosRelacionados(doc.id)
      .then(setRel)
      .catch((e) => { setRel(null); setRelErr(e) })
  }, [doc.id])
  useEffect(() => { cargarRel() }, [cargarRel])

  // Retención RECIBIDA: solo sobre una factura viva y A CRÉDITO — una venta de
  // contado ya se cobró completa en el mostrador, no deja saldo por cobrar que la
  // retención pueda bajar (el servidor lo rechaza con ErrRetencionSoloCredito).
  const retenible = doc.tipo === 'factura' && !doc.anulado && !!doc.credito
  const impuestosTomados = (rel?.retenciones || []).map((r) => r.impuesto || 'iva')
  const om = ORIGEN_META[origenDe(doc)]
  const pos = origenDe(doc) === 'pos'
  const accionable = doc.tipo === 'factura' && !doc.anulado
  const btnPDF = <Button variant="secondary" icon={<Icon.Download size={15} />} onClick={onPDF}>PDF</Button>
  // «Generar retención» desde la propia factura: la contadora pidió no tener que
  // salirse del documento para registrarla.
  const btnRetencion = retenible && puedeRetener(rol) ? (
    <Button variant="secondary" icon={<Icon.Shield size={16} />} onClick={() => setRetener(true)}>Retención</Button>
  ) : null
  const acciones = accionable && (onAnular || onNotaCredito || onNotaDebito) ? (
    <>
      {btnPDF}
      {btnRetencion}
      {onNotaDebito ? <Button variant="secondary" icon={<Icon.Receipt size={16} />} onClick={() => onNotaDebito(doc)}>Nota de débito</Button> : null}
      {onNotaCredito ? <Button variant="destructive" icon={<Icon.Receipt size={16} />} onClick={() => onNotaCredito(doc)}>Nota de crédito</Button> : null}
      {onAnular ? <Button variant="destructive" icon={<Icon.CircleX size={16} />} onClick={() => onAnular(doc)}>Anular</Button> : null}
    </>
  ) : <>{btnPDF}{btnRetencion}</>
  return (
    <VistaDetalle onVolver={onVolver} icon={<Icon.Receipt size={18} />}
      titulo={doc.numeroCompleto} sub={`${tm.label} · ${fmtDate(doc.fecha)}`} acciones={acciones}>
      <div className="grid grid-cols-1 lg:grid-cols-[340px_1fr] gap-4 items-start">
        {/* Columna izquierda: estado, origen, cliente y motivo */}
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3">
            <div className="flex items-center gap-2 flex-wrap">
              <Badge color={tm.color}>{tm.label}</Badge>
              <Badge color={om.color}>{om.label}</Badge>
              {doc.anulado ? <Badge color="red" dot>Anulada</Badge> : doc.tipo === 'factura' ? <Badge color="emerald" dot>Emitida</Badge> : doc.tipo === 'nota_credito' ? <Badge color="amber" dot>Crédito</Badge> : doc.tipo === 'nota_debito' ? <Badge color="sky" dot>Débito</Badge> : null}
              {doc.serie ? <span className="text-[11.5px] text-slate-400 num">Serie {doc.serie}</span> : null}
            </div>

            {/* Origen del documento: de dónde salió esta factura (dos ejes: tipo vs. canal). */}
            <div>
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Origen</div>
              {pos ? (
                <div className="flex items-start gap-2 text-[12.5px] bg-violet-50 dark:bg-violet-900/20 text-violet-800 dark:text-violet-200 rounded-lg px-3 py-2">
                  <Icon.Wallet size={15} className="mt-0.5 shrink-0" />
                  <span>
                    Emitida en el <strong>Punto de venta</strong>
                    {doc.cajaCodigo ? <> · caja <span className="num font-medium">{doc.cajaCodigo}</span></> : null}
                    {doc.cajeroNombre ? <> · cajero <span className="font-medium">{doc.cajeroNombre}</span></> : null}.
                  </span>
                </div>
              ) : (
                <div className="flex items-start gap-2 text-[12.5px] bg-teal-50 dark:bg-teal-500/10 text-teal-800 dark:text-teal-200 rounded-lg px-3 py-2">
                  <Icon.ClipboardList size={15} className="mt-0.5 shrink-0" />
                  <span>Emitida desde <strong>Ventas</strong> (forma libre / cotización facturada).</span>
                </div>
              )}
            </div>

            {(doc.tipo === 'anulacion' || doc.tipo === 'nota_credito' || doc.tipo === 'nota_debito') && doc.motivo ? (
              <div className="text-[12.5px] bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2">
                <span className="text-slate-500">{doc.tipo === 'nota_credito' ? 'Motivo de la nota de crédito: ' : doc.tipo === 'nota_debito' ? 'Concepto de la nota de débito: ' : 'Motivo de la reversa: '}</span><span className="font-medium">{doc.motivo}</span>
              </div>
            ) : null}

            <div>
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Cliente</div>
              <div className="text-[13.5px] font-medium">{doc.clienteNombre || 'Consumidor final'}</div>
              {doc.clienteDocumento ? <div className="text-[12px] text-slate-500 num">{doc.clienteDocumento}</div> : null}
            </div>
          </div>

          {(doc.pagos || []).length ? (
            <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Pagos</div>
              <div className="space-y-1.5">
                {(doc.pagos || []).map((p, i) => (
                  <div key={i} className="flex items-center justify-between text-[13px] px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700">
                    <span className="inline-flex items-center gap-2">{metodoLabel(p.metodo)}{p.enDivisa || p.moneda !== 'VES' ? <Badge size="sm" color="amber">divisa</Badge> : null}</span>
                    <span className="num font-medium private-mask">{fmtCurrency(p.monto, p.moneda || 'VES')}</span>
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          <Relacionados rel={rel} error={relErr} onReintentar={cargarRel} onAbrirDoc={onAbrirDoc} ccy={ccy} />

          <div className="flex items-start gap-2 text-[11.5px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2">
            <Icon.Lock size={13} className="mt-0.5 shrink-0" />
            <span>Los documentos fiscales no se editan. Para corregir, la anulación genera un documento de reversa que preserva la traza completa.</span>
          </div>
        </div>

        {/* Columna derecha: líneas y totales */}
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
            <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Líneas ({(doc.lineas || []).length})</div>
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
              {(doc.lineas || []).map((l, i) => (
                <div key={i} className="flex items-center gap-3 px-3 py-2">
                  <div className="flex-1 min-w-0">
                    <div className="text-[13px] font-medium truncate">{l.nombre || l.sku}</div>
                    <div className="text-[11px] text-slate-400 num">{fmtNum(l.cantidad)} × {fmtCurrency(l.precioUnitario, 'VES')}</div>
                  </div>
                  <div className="num text-[13px] font-medium private-mask">{fmtCurrency(l.total, 'VES')}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-4 space-y-1.5 text-[13px] max-w-sm ml-auto">
            <TotRow label="Subtotal" value={doc.subtotal} />
            <TotRow label="IVA" value={doc.iva} />
            {doc.igtf ? <TotRow label="IGTF" value={doc.igtf} /> : null}
            <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
              <span className="font-semibold">Total</span><span className="num font-semibold private-mask text-[16px]">{fmtCurrency(doc.total, 'VES')}</span>
            </div>
            {/* Tasa histórica del documento y de dónde salió: es lo que hace
                demostrable la conversión ante el SENIAT (Art. 177). */}
            {doc.tasaCambio ? (
              <div className="text-[11px] text-slate-400 num pt-0.5">
                Tasa: {fmtCurrency(doc.tasaCambio, 'VES')}/$
                {doc.tasaFuente ? ` · ${ETIQUETA_FUENTE[doc.tasaFuente] || doc.tasaFuente}` : ''}
              </div>
            ) : null}
          </div>
        </div>
      </div>

      {retener ? (
        <RetencionDesdeFacturaModal doc={doc} impuestosTomados={impuestosTomados} ccy={ccy}
          onClose={() => setRetener(false)}
          onSaved={() => { setRetener(false); cargarRel() }} toast={toast} />
      ) : null}
    </VistaDetalle>
  )
}

/* Relacionados — la traza del documento en los DOS sentidos: de dónde viene
 * (origen) y qué salió de él (notas, anulación) más sus retenciones.
 *
 * Los vínculos ya existían en el dato; lo que no había era manera de recorrerlos.
 * Antes, para saber si una factura tenía nota de crédito había que buscarla a
 * mano en la lista — y por eso nadie lo revisaba. */
function Relacionados({ rel, error, onReintentar, onAbrirDoc, ccy }) {
  const Caja = ({ children }) => (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
      <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Documentos relacionados</div>
      {children}
    </div>
  )
  if (error) {
    return (
      <Caja>
        <div className="text-[12.5px] text-slate-500">{String(error.message || error)}</div>
        <Button size="sm" variant="ghost" className="mt-2" icon={<Icon.Refresh size={14} />} onClick={onReintentar}>Reintentar</Button>
      </Caja>
    )
  }
  if (rel === undefined) return <Caja><TableSkeleton rows={2} cols={2} /></Caja>
  if (!rel) return null

  const derivados = rel.derivados || []
  const retenciones = rel.retenciones || []
  const vacio = !rel.origen && !derivados.length && !retenciones.length
  if (vacio) {
    return (
      <Caja>
        <div className="text-[12.5px] text-slate-400">
          Sin notas, anulaciones ni retenciones sobre este documento.
        </div>
      </Caja>
    )
  }

  const Fila = ({ v, etiqueta }) => {
    const meta = TIPO_META[v.tipo] || { label: v.tipo, color: 'slate' }
    return (
      <button type="button" onClick={() => onAbrirDoc?.(v.id)}
        className="w-full text-left px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 hover:border-elerp-400 hover:bg-slate-50 dark:hover:bg-slate-800/60 transition">
        <div className="flex items-center gap-1.5 flex-wrap">
          {etiqueta ? <span className="text-[10.5px] uppercase tracking-wide text-slate-400">{etiqueta}</span> : null}
          <Badge size="sm" color={meta.color}>{meta.label}</Badge>
          <span className="num text-[12.5px] font-medium">{v.numeroCompleto}</span>
          <span className="num text-[12px] text-slate-500 ml-auto private-mask">{fmtCurrency(v.total, ccy)}</span>
        </div>
        <div className="text-[11px] text-slate-400 mt-0.5">
          {fmtDate(v.fecha)}{v.motivo ? ` · ${v.motivo}` : ''}
        </div>
      </button>
    )
  }

  return (
    <Caja>
      <div className="space-y-1.5">
        {/* Sentido «hacia atrás»: desde una nota se vuelve a su factura. */}
        {rel.origen ? <Fila v={rel.origen} etiqueta="Origen" /> : null}
        {/* Sentido «hacia adelante»: lo que salió de este documento. */}
        {derivados.map((v) => <Fila key={v.id} v={v} />)}

        {/* Las retenciones van aparte: NO son documentos fiscales de venta (no
            llevan número de control ni entran al libro como una nota), y
            mezclarlas con las notas confundiría la lectura. */}
        {retenciones.length ? (
          <div className="pt-1.5">
            <div className="text-[10.5px] uppercase tracking-wide text-slate-400 mb-1">Retenciones</div>
            <div className="space-y-1.5">
              {retenciones.map((r) => (
                <div key={r.id} className="px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700">
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <Badge size="sm" color={r.impuesto === 'islr' ? 'teal' : 'sky'}>{IMPUESTO_LABEL[r.impuesto] || 'IVA'}</Badge>
                    <span className="num text-[12.5px] font-medium">{r.numeroComprobante}</span>
                    <span className="num text-[12px] text-violet-700 dark:text-violet-300 ml-auto private-mask">
                      {fmtCurrency(r.montoRetenido, ccy)}
                    </span>
                  </div>
                  <div className="text-[11px] text-slate-400 mt-0.5">
                    {fmtDate(r.fecha)} · {fmtNum(r.porcentaje, 2)}%{r.concepto ? ` · ${r.concepto}` : ''}
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : null}
      </div>
    </Caja>
  )
}

const TotRow = ({ label, value }) => (
  <div className="flex items-center justify-between"><span className="text-slate-500">{label}</span><span className="num private-mask">{fmtCurrency(value, 'VES')}</span></div>
)

// Confirmación de anulación: exige motivo (queda en la reversa y la auditoría).
function AnularModal({ doc, onClose, onSaved, toast }) {
  const [motivo, setMotivo] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)
  const err = !motivo.trim() ? 'El motivo es obligatorio (queda en el documento de reversa).' : ''

  const confirmar = async () => {
    setTouched(true)
    if (err) return
    setBusy(true)
    try {
      const rev = await api.anularDocumento(doc.id, motivo.trim())
      toast({ title: 'Factura anulada', body: `Se generó la reversa ${rev?.numeroCompleto || ''}.` })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo anular', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.CircleX size={18} />}
      title="Anular factura" sub={doc.numeroCompleto}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant="destructive" onClick={confirmar} loading={busy} icon={<Icon.CircleX size={16} />}>Anular factura</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="flex items-start gap-2 text-[12.5px] text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/20 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>Acción irreversible. La factura no se elimina: se crea un documento de reversa que la anula y revierte el inventario.</span>
        </div>
        <Field label="Motivo de la anulación" required error={touched ? err : ''}>
          <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!err} placeholder="Ej: Error en montos, devolución del cliente…" autoFocus />
        </Field>
      </div>
    </Modal>
  )
}

// Nota de crédito PARCIAL: acredita cantidades por línea (devolución/descuento).
// A diferencia de Anular (reversa total), aquí el cajero elige cuánto devolver de
// cada renglón. El backend impone el tope real (descontando notas previas); acá el
// máximo por línea es la cantidad facturada, con validación inmediata.
function NotaCreditoModal({ doc, onClose, onSaved, toast, ccy }) {
  const lineasOrig = doc.lineas || []
  const [cant, setCant] = useState(() => lineasOrig.map(() => ''))
  const [motivo, setMotivo] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  const parsed = lineasOrig.map((l, i) => {
    const n = Number(cant[i])
    return Number.isFinite(n) ? n : 0
  })
  const excede = lineasOrig.some((l, i) => parsed[i] > (Number(l.cantidad) || 0) + 1e-9)
  const negativo = parsed.some((n) => n < 0)
  const algo = parsed.some((n) => n > 0)

  // Total a acreditar EN VIVO, con la misma regla del backend: IVA solo sobre lo
  // gravado (líneas no exentas). El IGTF no se acredita.
  const { subtotal, iva, total } = useMemo(() => {
    let sub = 0, base = 0
    lineasOrig.forEach((l, i) => {
      const q = parsed[i]
      if (q <= 0) return
      const monto = (Number(l.precioUnitario) || 0) * q
      sub += monto
      if (!l.exento) base += monto
    })
    const ivaV = base * IVA_TASA
    return { subtotal: sub, iva: ivaV, total: sub + ivaV }
  }, [cant]) // eslint-disable-line react-hooks/exhaustive-deps

  const motivoErr = !motivo.trim() ? 'El motivo es obligatorio (queda en la nota de crédito).' : ''
  const puedeConfirmar = algo && !excede && !negativo && !motivoErr

  const confirmar = async () => {
    setTouched(true)
    if (!puedeConfirmar) return
    setBusy(true)
    try {
      const lineas = lineasOrig
        .map((l, i) => ({ sku: l.sku, cantidad: parsed[i] }))
        .filter((x) => x.cantidad > 0)
      const nc = await api.notaCredito(doc.id, { motivo: motivo.trim(), lineas })
      toast({ title: 'Nota de crédito emitida', body: `Se generó ${nc?.numeroCompleto || ''}.` })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo emitir la nota de crédito', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Receipt size={18} />}
      title="Nota de crédito" sub={doc.numeroCompleto}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant="destructive" onClick={confirmar} loading={busy} disabled={!puedeConfirmar} icon={<Icon.Receipt size={16} />}>Emitir nota de crédito</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="flex items-start gap-2 text-[12.5px] text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>Devolución/descuento parcial. Indica cuánto acreditar de cada renglón; la factura original no se modifica y se reingresa el inventario devuelto.</span>
        </div>

        <div>
          <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Líneas a acreditar</div>
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
            {lineasOrig.map((l, i) => {
              const max = Number(l.cantidad) || 0
              const invalid = touched && (parsed[i] > max + 1e-9 || parsed[i] < 0)
              return (
                <div key={i} className="flex items-center gap-3 px-3 py-2">
                  <div className="flex-1 min-w-0">
                    <div className="text-[13px] font-medium truncate">{l.nombre || l.sku}</div>
                    <div className="text-[11px] text-slate-400 num">Facturado {fmtNum(max)} × {fmtCurrency(l.precioUnitario, 'VES')}{l.exento ? ' · exento' : ''}</div>
                  </div>
                  <Input type="number" min={0} max={max} step="any" className="w-24 text-right num" invalid={invalid}
                    value={cant[i]} placeholder="0"
                    onChange={(e) => setCant((prev) => prev.map((v, j) => (j === i ? e.target.value : v)))}
                    onBlur={() => setTouched(true)} />
                </div>
              )
            })}
          </div>
          {touched && excede ? <div className="mt-1 text-[11.5px] text-red-600">No puedes acreditar más de lo facturado en alguna línea.</div> : null}
        </div>

        <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 p-3 space-y-1.5 text-[13px]">
          <div className="flex items-center justify-between"><span className="text-slate-500">Subtotal a acreditar</span><span className="num private-mask">{fmtCurrency(subtotal, ccy)}</span></div>
          <div className="flex items-center justify-between"><span className="text-slate-500">IVA</span><span className="num private-mask">{fmtCurrency(iva, ccy)}</span></div>
          <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
            <span className="font-semibold">Total a acreditar</span><span className="num font-semibold text-amber-700 dark:text-amber-300 private-mask">-{fmtCurrency(total, ccy)}</span>
          </div>
        </div>

        <Field label="Motivo de la nota de crédito" required error={touched ? motivoErr : ''}>
          <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!motivoErr} placeholder="Ej: 3 unidades llegaron dañadas, descuento acordado…" autoFocus />
        </Field>
      </div>
    </Modal>
  )
}

// Nota de débito: cargo adicional que AUMENTA el monto de una factura (cargos,
// intereses de mora, corrección de precio al alza). Espejo POSITIVO de la nota de
// crédito: en vez de acreditar líneas de la factura, carga un CONCEPTO nuevo con
// su monto y su IVA (calculado en vivo con la misma regla del backend). El IVA se
// omite si el cargo es exento (p. ej. intereses de mora). La factura original no
// se modifica; el backend emite un documento inmutable con serie propia (-ND).
function NotaDebitoModal({ doc, onClose, onSaved, toast, ccy }) {
  const [concepto, setConcepto] = useState('')
  const [monto, setMonto] = useState('')
  const [exento, setExento] = useState(false)
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  const base = Number(monto)
  const baseOk = Number.isFinite(base) && base > 0
  // Total a cargar EN VIVO, con la misma regla del backend: IVA solo si el cargo es
  // gravado; el IGTF no aplica (no es un cobro en divisas, es un ajuste de valor).
  const iva = useMemo(() => (baseOk && !exento ? base * IVA_TASA : 0), [monto, exento]) // eslint-disable-line react-hooks/exhaustive-deps
  const total = (baseOk ? base : 0) + iva

  const conceptoErr = !concepto.trim() ? 'El concepto es obligatorio (explica el cargo y queda en la nota).' : ''
  const montoErr = !baseOk ? 'Indica un monto mayor que cero.' : ''
  const puedeConfirmar = !conceptoErr && !montoErr

  const confirmar = async () => {
    setTouched(true)
    if (!puedeConfirmar) return
    setBusy(true)
    try {
      const nd = await api.emitirNotaDebito(doc.id, { concepto: concepto.trim(), monto: base, exento })
      toast({ title: 'Nota de débito emitida', body: `Se generó ${nd?.numeroCompleto || ''}.` })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo emitir la nota de débito', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Receipt size={18} />}
      title="Nota de débito" sub={doc.numeroCompleto}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={confirmar} loading={busy} disabled={!puedeConfirmar} icon={<Icon.Receipt size={16} />}>Emitir nota de débito</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="flex items-start gap-2 text-[12.5px] text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>Cargo adicional que <strong>aumenta</strong> el monto de la factura (cargos, intereses de mora, corrección de precio al alza). La factura original no se modifica; se emite un documento nuevo que la referencia.</span>
        </div>

        <Field label="Concepto del cargo" required error={touched ? conceptoErr : ''}>
          <Input value={concepto} onChange={(e) => setConcepto(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!conceptoErr} placeholder="Ej: Interés de mora por pago tardío, ajuste de precio…" autoFocus />
        </Field>

        <Field label="Monto base del cargo" required error={touched ? montoErr : ''}>
          <Input type="number" min={0} step="any" className="num" value={monto} placeholder="0,00"
            onChange={(e) => setMonto(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!montoErr} />
        </Field>

        <Toggle checked={exento} onChange={setExento} label="Cargo exento de IVA" sub="Actívalo si el cargo no causa IVA (p. ej. intereses de mora)." />

        <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 p-3 space-y-1.5 text-[13px]">
          <div className="flex items-center justify-between"><span className="text-slate-500">Base del cargo</span><span className="num private-mask">{fmtCurrency(baseOk ? base : 0, ccy)}</span></div>
          <div className="flex items-center justify-between"><span className="text-slate-500">IVA{exento ? ' (exento)' : ''}</span><span className="num private-mask">{fmtCurrency(iva, ccy)}</span></div>
          <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
            <span className="font-semibold">Total a cargar</span><span className="num font-semibold text-sky-700 dark:text-sky-300 private-mask">+{fmtCurrency(total, ccy)}</span>
          </div>
        </div>
      </div>
    </Modal>
  )
}

/* RetencionDesdeFacturaModal — registrar el comprobante de retención SIN salir
 * de la factura.
 *
 * Nota de la contadora: «al usuario no le gusta hacer las retenciones saliéndose
 * de la factura». Antes había que irse a Retenciones, buscar la factura en un
 * selector y recién ahí cargarla. Acá el documento ya está elegido: solo queda
 * el impuesto, el número del comprobante que entregó el cliente y el porcentaje.
 *
 * `impuestosTomados` son los impuestos que ESTA factura ya tiene retenidos. El
 * servidor rechaza el duplicado (único por dirección+impuesto+documento), así que
 * la pantalla lo dice ANTES en vez de dejar intentar y fallar.
 */
function RetencionDesdeFacturaModal({ doc, impuestosTomados = [], ccy, onClose, onSaved, toast }) {
  const libres = ['iva', 'islr'].filter((i) => !impuestosTomados.includes(i))
  const [impuesto, setImpuesto] = useState(libres[0] || 'iva')
  const [numeroComprobante, setNumeroComprobante] = useState('')
  const [fecha, setFecha] = useState(hoyISO())
  const [porcentaje, setPorcentaje] = useState('75')
  const [base, setBase] = useState('')
  const [concepto, setConcepto] = useState('')
  const [sustraendo, setSustraendo] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  const esISLR = impuesto === 'islr'
  const ivaDoc = Number(doc.iva) || 0
  const netoDoc = round2n((Number(doc.total) || 0) - ivaDoc - (Number(doc.igtf) || 0))

  // Al pasar a ISLR se sugiere la base = neto del documento, que es el punto de
  // partida habitual; sigue siendo editable porque el concepto puede acotarla.
  useEffect(() => { if (esISLR && !base) setBase(String(netoDoc)) }, [esISLR, base, netoDoc])

  const pct = Number(porcentaje)
  const pctValido = Number.isFinite(pct) && pct > 0 && pct <= 100
  const baseCalc = esISLR ? (Number(base) || 0) : ivaDoc
  const sust = esISLR ? (Number(sustraendo) || 0) : 0
  const montoRetenido = pctValido ? Math.max(0, round2n(baseCalc * (pct / 100) - sust)) : 0

  const errComp = !numeroComprobante.trim() ? 'El número de comprobante es obligatorio (lo emitió el cliente).' : ''
  const errFecha = !fecha ? 'La fecha es obligatoria.' : ''
  const errPct = !pctValido ? 'El porcentaje debe estar entre 0 y 100.' : ''
  const errBase = esISLR && !(baseCalc > 0) ? 'La base gravable debe ser mayor que cero.' : ''
  const errIVA = !esISLR && !(ivaDoc > 0) ? 'Esta factura no tiene IVA sobre el cual retener.' : ''
  const errConcepto = esISLR && !concepto.trim() ? 'Indica el concepto de la retención de ISLR.' : ''
  const errMonto = esISLR && pctValido && baseCalc > 0 && !(montoRetenido > 0)
    ? 'El monto a retener queda en cero (revisa el porcentaje y el sustraendo).' : ''
  const puede = !errComp && !errFecha && !errPct && !errBase && !errIVA && !errConcepto && !errMonto && !!libres.length

  const confirmar = async () => {
    setTouched(true)
    if (!puede) return
    setBusy(true)
    const body = { impuesto, numeroComprobante: numeroComprobante.trim(), fecha, porcentaje: pct }
    if (esISLR) { body.base = Number(base) || 0; body.concepto = concepto.trim(); body.sustraendo = Number(sustraendo) || 0 }
    try {
      const creada = await api.registrarRetencionRecibida(doc.id, body)
      toast({
        title: 'Retención registrada',
        body: `${IMPUESTO_LABEL[impuesto]} · Nº ${creada?.numeroComprobante || body.numeroComprobante} · ${fmtCurrency(montoRetenido, ccy)}.`,
      })
      await onSaved()
    } catch (e) {
      toast({ title: 'No se pudo registrar la retención', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Shield size={18} />}
      title="Registrar retención" sub={`Sobre ${doc.numeroCompleto} · ${doc.clienteNombre || 'cliente'}`}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant="secondary" onClick={confirmar} loading={busy} disabled={!libres.length} icon={<Icon.Shield size={16} />}>
          Registrar retención
        </Button>
      </>}>
      <div className="space-y-3.5">
        {!libres.length ? (
          <Empty framed icon={<Icon.CircleCheck size={20} />} title="Esta factura ya tiene sus retenciones"
            body="Hay un comprobante registrado de IVA y otro de ISLR. Solo se admite uno por impuesto y documento; para corregir, hay que revisarlo en Retenciones." />
        ) : (
          <>
            {impuestosTomados.length ? (
              <div className="flex items-start gap-2 text-[12.5px] text-amber-800 dark:text-amber-200 bg-amber-50 dark:bg-amber-900/20 rounded-lg px-3 py-2.5">
                <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
                <span>
                  Esta factura ya tiene retención de <strong>{impuestosTomados.map((i) => IMPUESTO_LABEL[i]).join(' y ')}</strong>.
                  Solo queda disponible {libres.map((i) => IMPUESTO_LABEL[i]).join(' y ')}.
                </span>
              </div>
            ) : null}

            <Field label="Impuesto">
              <Segmented value={impuesto} onChange={setImpuesto}
                options={libres.map((i) => ({ value: i, label: IMPUESTO_LABEL[i] }))} />
            </Field>

            <div className="grid grid-cols-2 gap-3">
              <Field label="Nº de comprobante" required error={touched ? errComp : ''} hint="el que emitió el cliente">
                <Input value={numeroComprobante} onChange={(e) => setNumeroComprobante(e.target.value)}
                  onBlur={() => setTouched(true)} invalid={touched && !!errComp}
                  placeholder="Ej: 20240800001234" className="num" autoFocus />
              </Field>
              <Field label="Fecha del comprobante" required error={touched ? errFecha : ''}>
                <Input type="date" value={fecha} onChange={(e) => setFecha(e.target.value)}
                  onBlur={() => setTouched(true)} invalid={touched && !!errFecha} />
              </Field>
            </div>

            {esISLR ? (
              <>
                <Field label="Concepto de la retención" required error={touched ? errConcepto : ''} hint="según la providencia">
                  <Input value={concepto} onChange={(e) => setConcepto(e.target.value)}
                    onBlur={() => setTouched(true)} invalid={touched && !!errConcepto}
                    placeholder="Ej: Honorarios profesionales, servicios, alquiler…" />
                </Field>
                <div className="grid grid-cols-2 gap-3">
                  <Field label="Base gravable" required error={touched ? errBase : ''} hint="sugerida: neto del documento">
                    <Input type="number" min={0} step="any" className="num" value={base}
                      onChange={(e) => setBase(e.target.value)} onBlur={() => setTouched(true)}
                      invalid={touched && !!errBase} placeholder="0,00" />
                  </Field>
                  <Field label="Sustraendo" hint="opcional">
                    <Input type="number" min={0} step="any" className="num" value={sustraendo}
                      onChange={(e) => setSustraendo(e.target.value)} placeholder="0,00" />
                  </Field>
                </div>
              </>
            ) : null}

            <Field label="Porcentaje retenido" required error={touched ? (errPct || errIVA) : ''}>
              {esISLR ? (
                <Input type="number" min={0} max={100} step="any" className="w-32 num" value={porcentaje}
                  onChange={(e) => setPorcentaje(e.target.value)} onBlur={() => setTouched(true)}
                  invalid={touched && !!errPct} placeholder="%" />
              ) : (
                <div className="flex items-center gap-2">
                  <Select value={PCT_RETENCION.includes(pct) ? String(pct) : 'otro'} className="w-40"
                    onChange={(e) => { const v = e.target.value; if (v !== 'otro') setPorcentaje(v) }}>
                    {PCT_RETENCION.map((p) => <option key={p} value={p}>{p}%</option>)}
                    <option value="otro">Otro…</option>
                  </Select>
                  {!PCT_RETENCION.includes(pct) ? (
                    <Input type="number" min={0} max={100} step="any" className="w-28 num" value={porcentaje}
                      onChange={(e) => setPorcentaje(e.target.value)} onBlur={() => setTouched(true)}
                      invalid={touched && !!errPct} placeholder="%" />
                  ) : null}
                </div>
              )}
            </Field>

            {/* Monto a retener EN VIVO: el servidor lo vuelve a derivar de la base
                y el porcentaje, así que esto es un espejo, nunca la fuente. */}
            <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 p-3 space-y-1.5 text-[13px]">
              <div className="flex items-center justify-between">
                <span className="text-slate-500">{esISLR ? 'Base gravable' : 'IVA de la factura'}</span>
                <span className="num private-mask">{fmtCurrency(baseCalc, ccy)}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-500">Porcentaje</span>
                <span className="num">{pctValido ? `${fmtNum(pct, 2)}%` : '—'}</span>
              </div>
              {esISLR && sust > 0 ? (
                <div className="flex items-center justify-between">
                  <span className="text-slate-500">Sustraendo</span>
                  <span className="num private-mask">−{fmtCurrency(sust, ccy)}</span>
                </div>
              ) : null}
              {touched && errMonto ? <div className="text-[11.5px] text-red-600">{errMonto}</div> : null}
              <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
                <span className="font-semibold">Monto a retener</span>
                <span className="num font-semibold private-mask text-violet-700 dark:text-violet-300">{fmtCurrency(montoRetenido, ccy)}</span>
              </div>
            </div>
          </>
        )}
      </div>
    </Modal>
  )
}
