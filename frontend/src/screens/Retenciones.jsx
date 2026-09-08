import { useState, useEffect, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, Segmented, Modal, Empty, TableSkeleton, useToast, Field } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { ComprobanteRetencionModal } from '../components/ComprobanteRetencion.jsx'

// Retenciones de IVA (comprobantes). Dos direcciones:
//   · Recibida — un cliente AGENTE DE RETENCIÓN te retiene un % (75/100) del IVA de
//     TU factura de venta y te entrega el comprobante. Es un crédito a tu favor
//     (baja la CxC de esa factura).
//   · Emitida — si ERES agente de retención, tú retienes el IVA a tu proveedor sobre
//     su factura de compra y le emites el comprobante (baja la CxP).
// El backend es la autoridad: la lista y el alta son append-only; registrar rechaza
// con 400 (duplicada, % inválido, sin IVA, documento anulado). La UI solo evita el
// error obvio y muestra el mensaje del servidor.

// El % típico de retención de IVA en Venezuela es 75% o 100%.
const PORCENTAJES = [75, 100]

// Segmentos por dirección de la retención, con su copy de vacío por segmento.
const SEGMENTOS = [
  { value: 'todas', label: 'Todas', tipo: null, vacio: 'Aún no hay retenciones registradas' },
  { value: 'recibida', label: 'Recibidas', tipo: 'recibida', vacio: 'No hay retenciones recibidas' },
  { value: 'emitida', label: 'Emitidas', tipo: 'emitida', vacio: 'No hay retenciones emitidas' },
]

const TIPO_META = {
  recibida: { label: 'Recibida', color: 'violet' },
  emitida: { label: 'Emitida', color: 'amber' },
}

// Impuesto de la retención: IVA (base = IVA del documento) o ISLR (base gravable
// entrada, con concepto y sustraendo opcional). Documento sin impuesto ⇒ IVA.
const IMPUESTO_META = {
  iva: { label: 'IVA', color: 'sky' },
  islr: { label: 'ISLR', color: 'teal' },
}
const impuestoDe = (r) => (r.impuesto === 'islr' ? 'islr' : 'iva')
const round2 = (v) => Math.round((Number(v) || 0) * 100) / 100
// Neto/base del documento (subtotal), tolerante a distintas formas del objeto.
const netoDe = (o) => Number(o?.subtotal) || Number(o?.base) || Number(o?.neto)
  || ((Number(o?.total) || 0) - (Number(o?.iva) || 0))

// Escritura: la retención recibida es materia de cumplimiento fiscal, así que la
// pueden registrar Dueña/Desarrollador/Contadora. La emitida vive en Compras y la
// registran Dueña/Desarrollador (el backend impone el gate real; aquí solo evitamos
// mostrar acciones que devolverían 403).
const puedeRecibida = (rol) => ['dueno', 'desarrollador', 'contadora'].includes(rol)
const puedeEmitida = (rol) => ['dueno', 'desarrollador'].includes(rol)

const hoyISO = () => new Date().toISOString().slice(0, 10)

export function Retenciones() {
  const { db } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const ccy = ui.ccy

  const [seg, setSeg] = useState('todas')
  const [data, setData] = useState(undefined) // undefined = cargando; null = error
  const [error, setError] = useState(null)
  const [alta, setAlta] = useState(null) // 'recibida' | 'emitida' | null
  const [imprimir, setImprimir] = useState(null) // retención emitida a imprimir

  const cargar = useCallback(async () => {
    setError(null)
    setData(undefined)
    try {
      setData(await api.retenciones())
    } catch (e) {
      setData(null)
      setError(e)
    }
  }, [])

  useEffect(() => { cargar() }, [cargar])

  const lista = data || []

  // Contadores por dirección sobre TODA la lista (no sobre el filtro activo).
  const conteos = useMemo(() => {
    const c = { todas: lista.length, recibida: 0, emitida: 0 }
    for (const r of lista) c[r.tipo] = (c[r.tipo] || 0) + 1
    return c
  }, [lista])

  const segActivo = SEGMENTOS.find((s) => s.value === seg) || SEGMENTOS[0]
  const rows = segActivo.tipo ? lista.filter((r) => r.tipo === segActivo.tipo) : lista

  const puedeAltaRecibida = puedeRecibida(ui.rol)
  const puedeAltaEmitida = puedeEmitida(ui.rol)

  return (
    <div>
      {/* Nota conceptual: recibida vs. emitida. */}
      <div className="mb-3 flex items-start gap-2 text-[12px] text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2.5">
        <Icon.Shield size={15} className="mt-0.5 shrink-0 text-elerp-500" />
        <span>
          <strong>Recibida:</strong> un cliente agente de retención te retiene un % del IVA de tu factura de venta y te da el comprobante (crédito a tu favor).{' '}
          <strong>Emitida:</strong> si eres agente de retención, tú retienes el IVA a tu proveedor sobre su factura de compra y le emites el comprobante. El % típico es 75% o 100%.
        </span>
      </div>

      <div className="flex items-center gap-2 flex-wrap mb-3">
        {/* Segmentos por dirección, con contador por segmento. */}
        <div className="inline-flex items-center gap-1 p-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/70 overflow-x-auto">
          {SEGMENTOS.map((s) => {
            const activo = seg === s.value
            const n = conteos[s.value] || 0
            return (
              <button key={s.value} onClick={() => setSeg(s.value)}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[12.5px] font-semibold whitespace-nowrap transition-colors ${activo ? 'bg-white dark:bg-slate-900 shadow-sm text-slate-900 dark:text-slate-100' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'}`}>
                {s.label}
                <span className={`num text-[10.5px] leading-none px-1.5 py-0.5 rounded-full ${activo ? 'bg-elerp-50 text-elerp-600 dark:bg-elerp-900/50 dark:text-elerp-200' : 'bg-slate-200 text-slate-500 dark:bg-slate-700 dark:text-slate-300'}`}>{fmtNum(n, 0)}</span>
              </button>
            )
          })}
        </div>
        {(puedeAltaRecibida || puedeAltaEmitida) ? (
          <div className="ml-auto flex items-center gap-2">
            {puedeAltaRecibida ? (
              <Button variant="secondary" icon={<Icon.Plus size={15} />} onClick={() => setAlta('recibida')}>Registrar retención recibida</Button>
            ) : null}
            {puedeAltaEmitida ? (
              <Button variant="primary" icon={<Icon.Plus size={15} />} onClick={() => setAlta('emitida')}>Registrar retención emitida</Button>
            ) : null}
          </div>
        ) : null}
      </div>

      {error ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las retenciones"
          body={String(error.message || error)}
          cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          {data === undefined ? (
            <div className="p-4"><TableSkeleton rows={7} cols={7} /></div>
          ) : rows.length === 0 ? (
            <Empty framed={false} icon={<Icon.Shield size={22} />}
              title={segActivo.vacio}
              body={seg === 'emitida'
                ? 'Se registran desde una factura de compra cuando actúas como agente de retención.'
                : seg === 'recibida'
                ? 'Se registran cuando un cliente agente de retención te entrega su comprobante.'
                : 'Registra la primera con los botones de arriba.'} />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm tbl-sticky">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                    <th className="py-2.5 px-3 font-medium">Fecha</th>
                    <th className="py-2.5 px-3 font-medium">Tipo</th>
                    <th className="py-2.5 px-3 font-medium">Impuesto</th>
                    <th className="py-2.5 px-3 font-medium">Nº comprobante</th>
                    <th className="py-2.5 px-3 font-medium">Documento</th>
                    <th className="py-2.5 px-3 font-medium">Tercero</th>
                    <th className="py-2.5 px-3 font-medium text-right">Base</th>
                    <th className="py-2.5 px-3 font-medium text-center">%</th>
                    <th className="py-2.5 px-3 font-medium text-right">Monto retenido</th>
                    <th className="py-2.5 px-3 font-medium text-right">Comprobante</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => {
                    const tm = TIPO_META[r.tipo] || { label: r.tipo, color: 'slate' }
                    const im = IMPUESTO_META[impuestoDe(r)]
                    return (
                      <tr key={r.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
                        <td className="py-2.5 px-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(r.fecha)}</td>
                        <td className="py-2.5 px-3"><Badge size="sm" color={tm.color}>{tm.label}</Badge></td>
                        <td className="py-2.5 px-3">
                          <Badge size="sm" color={im.color}>{im.label}</Badge>
                          {impuestoDe(r) === 'islr' && r.concepto ? <div className="text-[11px] text-slate-400 truncate max-w-[160px]">{r.concepto}</div> : null}
                        </td>
                        <td className="py-2.5 px-3 num text-[12.5px] font-medium whitespace-nowrap">{r.numeroComprobante}</td>
                        <td className="py-2.5 px-3 num text-[12.5px] whitespace-nowrap">{r.documentoNumero}</td>
                        <td className="py-2.5 px-3">
                          <div className="text-[13px] truncate max-w-[200px]">{r.terceroNombre || '—'}</div>
                          {r.terceroRif ? <div className="text-[11px] text-slate-400 num">{r.terceroRif}</div> : null}
                        </td>
                        <td className="py-2.5 px-3 text-right num text-[12.5px] text-slate-500 private-mask">{fmtCurrency(r.base, ccy)}</td>
                        <td className="py-2.5 px-3 text-center num text-[12px] text-slate-400">{fmtNum(r.porcentaje, 0)}%</td>
                        <td className="py-2.5 px-3 text-right num text-[12.5px] font-medium private-mask">{fmtCurrency(r.montoRetenido, ccy)}</td>
                        <td className="py-2.5 px-3 text-right">
                          {r.tipo === 'emitida' ? (
                            <Button size="sm" variant="ghost" icon={<Icon.Receipt size={14} />} onClick={() => setImprimir(r)}>Imprimir</Button>
                          ) : <span className="text-[11px] text-slate-300 dark:text-slate-600">—</span>}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} comprobante(s)</div> : null}

      {alta === 'recibida' ? (
        <AltaRetencionModal
          modo="recibida"
          documentos={documentosRetenibles(db.DOCUMENTOS)}
          onClose={() => setAlta(null)}
          onSaved={cargar}
          toast={toast}
          ccy={ccy} />
      ) : null}
      {alta === 'emitida' ? (
        <AltaRetencionModal
          modo="emitida"
          onClose={() => setAlta(null)}
          onSaved={cargar}
          toast={toast}
          ccy={ccy} />
      ) : null}
      <ComprobanteRetencionModal open={!!imprimir} retencion={imprimir} empresa={db.EMPRESA} onClose={() => setImprimir(null)} />
    </div>
  )
}

// Facturas de venta candidatas a retención recibida: tipo factura, no anuladas y
// A CRÉDITO. Sólo una venta a crédito tiene un cliente identificado (agente de
// retención) que retiene al pagar; una venta de contado a consumidor final no
// genera comprobante de retención. El filtro por impuesto (IVA exige IVA>0; ISLR
// admite cualquiera) lo aplica el modal, y el backend valida de nuevo.
function documentosRetenibles(documentos) {
  return (documentos || []).filter((d) => d.tipo === 'factura' && !d.anulado && d.credito)
}

// Modal de alta compartido por ambas direcciones (recibida/emitida) y ambos
// impuestos (IVA/ISLR). IVA: la base es el IVA del documento y el monto = IVA×%.
// ISLR: la base la teclea el usuario (se sugiere el neto del documento), con
// concepto y sustraendo, y el monto = max(0, base×% − sustraendo). La factura de
// compra se pide on-demand al abrir (no vive en el `db` para todos los roles).
function AltaRetencionModal({ modo, documentos, onClose, onSaved, toast, ccy }) {
  const esRecibida = modo === 'recibida'

  const [opciones, setOpciones] = useState(esRecibida ? (documentos || []) : undefined)
  const [cargaErr, setCargaErr] = useState(null)

  const [impuesto, setImpuesto] = useState('iva')
  const [docId, setDocId] = useState('')
  const [numeroComprobante, setNumeroComprobante] = useState('')
  const [fecha, setFecha] = useState(hoyISO())
  const [porcentaje, setPorcentaje] = useState('75')
  const [base, setBase] = useState('')
  const [concepto, setConcepto] = useState('')
  const [sustraendo, setSustraendo] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  const esISLR = impuesto === 'islr'

  useEffect(() => {
    if (esRecibida) return
    let vivo = true
    api.facturasCompra()
      .then((fc) => { if (vivo) setOpciones(fc || []) })
      .catch((e) => { if (vivo) { setOpciones([]); setCargaErr(e) } })
    return () => { vivo = false }
  }, [esRecibida])

  const cargando = opciones === undefined
  const todas = opciones || []
  // Candidatas por impuesto: IVA exige IVA>0; ISLR admite cualquier factura.
  const lista = useMemo(
    () => (impuesto === 'iva' ? todas.filter((o) => (Number(o.iva) || 0) > 0) : todas),
    [todas, impuesto],
  )

  const seleccionado = lista.find((o) => idDe(o) === docId) || null
  const ivaDoc = seleccionado ? (Number(seleccionado.iva) || 0) : 0
  const netoDoc = seleccionado ? netoDe(seleccionado) : 0

  // Al elegir documento (o cambiar a ISLR) se sugiere la base = neto del documento.
  useEffect(() => {
    if (esISLR && seleccionado) setBase(String(round2(netoDoc)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [docId, impuesto])
  // Si al cambiar de impuesto el documento deja de ser candidato, se limpia.
  useEffect(() => {
    if (docId && !lista.some((o) => idDe(o) === docId)) setDocId('')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [impuesto])

  const pct = Number(porcentaje)
  const pctValido = Number.isFinite(pct) && pct > 0 && pct <= 100
  const baseCalc = esISLR ? (Number(base) || 0) : ivaDoc
  const sust = esISLR ? (Number(sustraendo) || 0) : 0
  const montoRetenido = pctValido ? Math.max(0, round2(baseCalc * (pct / 100) - sust)) : 0

  const errDoc = !docId ? `Elige una factura de ${esRecibida ? 'venta' : 'compra'}.` : ''
  // En la EMITIDA el número lo genera el sistema (correlativo AAAAMM); solo la
  // RECIBIDA exige el número (el que emitió el cliente agente de retención).
  const errComp = esRecibida && !numeroComprobante.trim() ? 'El número de comprobante es obligatorio.' : ''
  const errFecha = !fecha ? 'La fecha es obligatoria.' : ''
  const errPct = !pctValido ? 'El porcentaje debe estar entre 0 y 100.' : ''
  const errBase = esISLR && !(baseCalc > 0) ? 'La base gravable debe ser mayor que cero.' : ''
  const errConcepto = esISLR && !concepto.trim() ? 'Indica el concepto de la retención de ISLR.' : ''
  const errMonto = esISLR && pctValido && baseCalc > 0 && !(montoRetenido > 0)
    ? 'El monto a retener queda en cero (revisa el porcentaje y el sustraendo).' : ''
  const puedeConfirmar = !errDoc && !errComp && !errFecha && !errPct && !errBase && !errConcepto && !errMonto

  const confirmar = async () => {
    setTouched(true)
    if (!puedeConfirmar) return
    setBusy(true)
    const body = { impuesto, numeroComprobante: numeroComprobante.trim(), fecha, porcentaje: pct }
    if (esISLR) { body.base = Number(base) || 0; body.concepto = concepto.trim(); body.sustraendo = Number(sustraendo) || 0 }
    try {
      const creada = esRecibida ? await api.registrarRetencionRecibida(docId, body) : await api.registrarRetencionEmitida(docId, body)
      const num = creada?.numeroComprobante || body.numeroComprobante
      toast({
        title: esRecibida ? 'Retención registrada' : 'Comprobante de retención emitido',
        body: `${IMPUESTO_META[impuesto].label} · Nº ${num} · ${fmtCurrency(montoRetenido, ccy)}.`,
      })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo registrar la retención', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  const titulo = esRecibida ? 'Registrar retención recibida' : 'Registrar retención emitida'
  const subt = esRecibida ? 'Sobre una factura de venta (crédito a tu favor)' : 'Sobre una factura de compra (retienes a tu proveedor)'

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Shield size={18} />}
      title={titulo} sub={subt}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant={esRecibida ? 'secondary' : 'primary'} onClick={confirmar} loading={busy} disabled={cargando || !lista.length} icon={<Icon.Shield size={16} />}>
          Registrar retención
        </Button>
      </>}>
      <div className="space-y-3.5">
        {/* Impuesto: IVA (base = IVA del documento) o ISLR (base gravable + concepto). */}
        <Field label="Impuesto">
          <Segmented value={impuesto} onChange={setImpuesto}
            options={[{ value: 'iva', label: 'IVA' }, { value: 'islr', label: 'ISLR' }]} />
        </Field>

        <div className="flex items-start gap-2 text-[12.5px] text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>{esISLR
            ? (esRecibida
              ? 'Comprobante de ISLR que te entregó el cliente: baja tu cuenta por cobrar por lo retenido (anticipo de ISLR a tu favor). Una por factura e impuesto.'
              : 'Comprobante de ISLR que le emites al proveedor: baja tu cuenta por pagar por lo retenido (ISLR por enterar al SENIAT). Una por factura e impuesto.')
            : (esRecibida
              ? 'Comprobante de IVA que te entregó el cliente: baja tu cuenta por cobrar por el IVA retenido. Una por factura e impuesto.'
              : 'Comprobante de IVA que le emites al proveedor: baja tu cuenta por pagar por el IVA retenido. Una por factura e impuesto.')}</span>
        </div>

        {cargando ? (
          <div className="py-2"><TableSkeleton rows={2} cols={2} /></div>
        ) : !lista.length ? (
          <Empty framed icon={<Icon.Receipt size={20} />}
            title={esRecibida ? 'No hay facturas de venta disponibles' : 'No hay facturas de compra'}
            body={cargaErr ? String(cargaErr.message || cargaErr)
              : impuesto === 'iva'
                ? (esRecibida ? 'Emite una factura de venta gravada (con IVA) para una retención de IVA.' : 'Registra una factura de compra con IVA para retener IVA.')
                : (esRecibida ? 'Emite una factura de venta para registrar una retención de ISLR.' : 'Registra una factura de compra para retener ISLR.')} />
        ) : (
          <>
            <Field label={esRecibida ? 'Factura de venta' : 'Factura de compra'} required error={touched ? errDoc : ''}>
              <Select value={docId} onChange={(e) => setDocId(e.target.value)} invalid={touched && !!errDoc}>
                <option value="">Selecciona una factura…</option>
                {lista.map((o) => (
                  <option key={idDe(o)} value={idDe(o)}>{etiquetaDoc(o, esRecibida, ccy)}</option>
                ))}
              </Select>
            </Field>

            <div className="grid grid-cols-2 gap-3">
              {esRecibida ? (
                <Field label="Nº de comprobante" required error={touched ? errComp : ''}>
                  <Input value={numeroComprobante} onChange={(e) => setNumeroComprobante(e.target.value)}
                    onBlur={() => setTouched(true)} invalid={touched && !!errComp}
                    placeholder="Ej: 20240800001234" className="num" />
                </Field>
              ) : (
                <Field label="Nº de comprobante" hint="lo genera el sistema al emitir">
                  <div className="h-10 flex items-center px-3 rounded-lg border border-dashed border-slate-300 dark:border-slate-700 text-[12.5px] text-slate-400 num">
                    AAAAMM········ (automático)
                  </div>
                </Field>
              )}
              <Field label="Fecha del comprobante" required error={touched ? errFecha : ''}>
                <Input type="date" value={fecha} onChange={(e) => setFecha(e.target.value)}
                  onBlur={() => setTouched(true)} invalid={touched && !!errFecha} />
              </Field>
            </div>

            {/* ISLR: concepto, base gravable (sugerida del neto) y sustraendo. */}
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
                      onChange={(e) => setBase(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!errBase} placeholder="0,00" />
                  </Field>
                  <Field label="Sustraendo" hint="opcional">
                    <Input type="number" min={0} step="any" className="num" value={sustraendo}
                      onChange={(e) => setSustraendo(e.target.value)} placeholder="0,00" />
                  </Field>
                </div>
              </>
            ) : null}

            <Field label="Porcentaje retenido" required error={touched ? errPct : ''}>
              {esISLR ? (
                <Input type="number" min={0} max={100} step="any" className="w-32 num"
                  value={porcentaje} onChange={(e) => setPorcentaje(e.target.value)}
                  onBlur={() => setTouched(true)} invalid={touched && !!errPct} placeholder="%" />
              ) : (
                <div className="flex items-center gap-2">
                  <Select value={PORCENTAJES.includes(pct) ? String(pct) : 'otro'} className="w-40"
                    onChange={(e) => { const v = e.target.value; if (v !== 'otro') setPorcentaje(v) }}>
                    {PORCENTAJES.map((p) => <option key={p} value={p}>{p}%</option>)}
                    <option value="otro">Otro…</option>
                  </Select>
                  {!PORCENTAJES.includes(pct) ? (
                    <Input type="number" min={0} max={100} step="any" className="w-28 num"
                      value={porcentaje} onChange={(e) => setPorcentaje(e.target.value)}
                      onBlur={() => setTouched(true)} invalid={touched && !!errPct} placeholder="%" />
                  ) : null}
                </div>
              )}
            </Field>

            {/* Monto a retener EN VIVO. */}
            <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 p-3 space-y-1.5 text-[13px]">
              <div className="flex items-center justify-between"><span className="text-slate-500">{esISLR ? 'Base gravable' : 'IVA del documento'}</span><span className="num private-mask">{fmtCurrency(baseCalc, ccy)}</span></div>
              <div className="flex items-center justify-between"><span className="text-slate-500">Porcentaje</span><span className="num">{pctValido ? `${fmtNum(pct, 2)}%` : '—'}</span></div>
              {esISLR && sust > 0 ? <div className="flex items-center justify-between"><span className="text-slate-500">Sustraendo</span><span className="num private-mask">−{fmtCurrency(sust, ccy)}</span></div> : null}
              {touched && errMonto ? <div className="text-[11.5px] text-red-600">{errMonto}</div> : null}
              <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
                <span className="font-semibold">Monto a retener</span>
                <span className={`num font-semibold private-mask ${esRecibida ? 'text-violet-700 dark:text-violet-300' : 'text-amber-700 dark:text-amber-300'}`}>{fmtCurrency(montoRetenido, ccy)}</span>
              </div>
            </div>
          </>
        )}
      </div>
    </Modal>
  )
}

const idDe = (o) => o.id

// Etiqueta del documento en el selector: número + tercero + neto (base).
function etiquetaDoc(o, esRecibida, ccy) {
  const numero = esRecibida ? o.numeroCompleto : (o.numeroFactura || o.numeroControl || o.id)
  const tercero = esRecibida ? (o.clienteNombre || 'Consumidor final') : (o.proveedorNombre || 's/proveedor')
  return `${numero} · ${tercero} · ${fmtCurrency(netoDe(o), ccy)}`
}
