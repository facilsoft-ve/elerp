import { useState, useEffect, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, Modal, Empty, TableSkeleton, VistaDetalle, useToast, Field, Toggle } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'

/* Notas de crédito/débito de PROVEEDOR. Ajustan una FACTURA DE COMPRA ya registrada:
 *   · Nota de CRÉDITO (devolución/descuento): el proveedor te acredita → DISMINUYE lo
 *     que le debes. Se guarda con montos negativos.
 *   · Nota de DÉBITO (cargo adicional: flete, interés): el proveedor te carga más →
 *     AUMENTA lo que le debes. Montos positivos.
 * Append-only: aquí se listan y se detallan; se emiten desde una factura de compra.
 * El backend es la autoridad (calcula el IVA con la alícuota histórica de la factura,
 * cuadra el asiento y ajusta la CxP). La UI solo evita el error obvio y muestra el
 * mensaje del servidor.
 *
 * Consulta y emisión: Dueña / Desarrollador / Contadora (materia fiscal/contable). */

const ROLES = ['dueno', 'desarrollador', 'contadora']

const META = {
  nota_credito: {
    label: 'Nota de crédito de proveedor',
    corto: 'Nota de crédito',
    color: 'emerald',
    icon: Icon.Receipt,
    signo: 'Disminuye la deuda',
    desc: 'Devoluciones y descuentos que el proveedor te acredita sobre una compra: bajan lo que le debes.',
    vacio: 'Aún no hay notas de crédito de proveedor',
  },
  nota_debito: {
    label: 'Nota de débito de proveedor',
    corto: 'Nota de débito',
    color: 'amber',
    icon: Icon.Receipt,
    signo: 'Aumenta la deuda',
    desc: 'Cargos adicionales del proveedor sobre una compra (flete, intereses, corrección al alza): suben lo que le debes.',
    vacio: 'Aún no hay notas de débito de proveedor',
  },
}

const hoyISO = () => new Date().toISOString().slice(0, 10)

// Alícuota histórica de una factura de compra, derivada de sus propios montos (para
// el cálculo EN VIVO; el backend recalcula con la misma regla y es la autoridad).
function alicuotaDe(fc) {
  const base = Number(fc?.baseImponible) || 0
  const iva = Number(fc?.iva) || 0
  return base > 0 ? iva / base : 0.16
}

export function NotasCompra({ tipo = 'nota_credito' }) {
  const { ui } = useUI()
  const toast = useToast()
  const ccy = ui.ccy
  const meta = META[tipo] || META.nota_credito
  const puede = ROLES.includes(ui.rol)

  const [data, setData] = useState(undefined) // undefined = cargando; null = error
  const [error, setError] = useState(null)
  const [detalle, setDetalle] = useState(null)
  const [emitir, setEmitir] = useState(false)

  const cargar = useCallback(async () => {
    setError(null)
    setData(undefined)
    try {
      setData(await api.notasCompra())
    } catch (e) {
      setData(null)
      setError(e)
    }
  }, [])

  useEffect(() => { cargar() }, [cargar])

  const rows = useMemo(() => {
    const base = (data || []).filter((n) => n.tipo === tipo)
    base.sort((a, b) => String(b.registrada || '').localeCompare(String(a.registrada || '')))
    return base
  }, [data, tipo])

  if (!puede) {
    return <Empty icon={<Icon.Lock size={22} />} title="Sin acceso a las notas de proveedor"
      body="Las notas de crédito/débito de proveedor son de la Dueña, el Desarrollador y la Contadora." />
  }

  if (detalle) {
    return <DetalleNota nota={detalle} ccy={ccy} onVolver={() => setDetalle(null)} />
  }

  return (
    <div>
      <div className="mb-3 flex items-start gap-2 text-[12px] text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2.5">
        <Icon.CircleAlert size={15} className="mt-0.5 shrink-0 text-elerp-500" />
        <span>{meta.desc} Se emiten sobre una <strong>factura de compra</strong> registrada y son append-only: aquí se consultan, no se editan. El IVA usa la <strong>alícuota histórica</strong> de la factura y ajustan la <strong>cuenta por pagar</strong> del proveedor.</span>
      </div>

      <div className="flex items-center gap-2 flex-wrap mb-3">
        <div className="text-[12px] text-slate-500 hidden sm:block">{meta.label}</div>
        {puede ? (
          <div className="ml-auto">
            <Button variant="primary" icon={<Icon.Plus size={15} />} onClick={() => setEmitir(true)}>
              Emitir {meta.corto.toLowerCase()}
            </Button>
          </div>
        ) : null}
      </div>

      {error ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las notas"
          body={String(error.message || error)}
          cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          {data === undefined ? (
            <div className="p-4"><TableSkeleton rows={6} cols={7} /></div>
          ) : rows.length === 0 ? (
            <Empty framed={false} icon={<meta.icon size={22} />}
              title={meta.vacio}
              body="Emítela desde una factura de compra registrada con el botón de arriba." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm tbl-sticky">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                    <th className="py-2.5 px-3 font-medium">Nº nota</th>
                    <th className="py-2.5 px-3 font-medium">Nº control</th>
                    <th className="py-2.5 px-3 font-medium">Proveedor</th>
                    <th className="py-2.5 px-3 font-medium">Concepto</th>
                    <th className="py-2.5 px-3 font-medium">Fecha</th>
                    <th className="py-2.5 px-3 font-medium text-right">Base</th>
                    <th className="py-2.5 px-3 font-medium text-right">IVA</th>
                    <th className="py-2.5 px-3 font-medium text-right">Total</th>
                    <th className="py-2.5 px-3"></th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((n) => (
                    <tr key={n.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover cursor-pointer" onClick={() => setDetalle(n)}>
                      <td className="py-2.5 px-3 num text-[12.5px] font-medium whitespace-nowrap">{n.numeroCompleto}</td>
                      <td className="py-2.5 px-3 num text-[12.5px] text-slate-500 whitespace-nowrap">{n.numeroControl || '—'}</td>
                      <td className="py-2.5 px-3">
                        <div className="text-[13px] truncate max-w-[180px]">{n.proveedorNombre || '—'}</div>
                        {n.proveedorRif ? <div className="text-[11px] text-slate-400 num">{n.proveedorRif}</div> : null}
                      </td>
                      <td className="py-2.5 px-3"><div className="text-[12.5px] text-slate-500 truncate max-w-[220px]">{n.concepto || '—'}</div></td>
                      <td className="py-2.5 px-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(n.fecha)}</td>
                      <td className="py-2.5 px-3 text-right num text-[12.5px] text-slate-500 private-mask">{fmtCurrency(Math.abs(n.baseImponible) + Math.abs(n.baseExenta), ccy)}</td>
                      <td className="py-2.5 px-3 text-right num text-[12.5px] text-slate-500 private-mask">{fmtCurrency(Math.abs(n.iva), ccy)}</td>
                      <td className={`py-2.5 px-3 text-right num text-[12.5px] font-medium private-mask ${meta.color === 'emerald' ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-700 dark:text-amber-300'}`}>{fmtCurrency(Math.abs(n.total), ccy)}</td>
                      <td className="py-2.5 px-3 text-right"><Icon.ChevRight size={15} className="inline text-slate-300" /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} nota(s)</div> : null}

      {emitir ? (
        <EmitirNotaCompraModal tipo={tipo} onClose={() => setEmitir(false)} onSaved={cargar} ccy={ccy} />
      ) : null}
    </div>
  )
}

function DetalleNota({ nota: n, ccy, onVolver }) {
  const meta = META[n.tipo] || META.nota_credito
  return (
    <VistaDetalle onVolver={onVolver} icon={<meta.icon size={18} />}
      titulo={`${meta.corto} ${n.numeroCompleto}`}
      sub={`${n.proveedorNombre || 'Proveedor'} · ${fmtDate(n.fecha)}`}>
      <div className="grid grid-cols-1 lg:grid-cols-[340px_1fr] gap-4 items-start">
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3">
            <div className="flex items-center gap-2 flex-wrap">
              <Badge color={meta.color}>{meta.corto}</Badge>
              <Badge color="slate" dot>{meta.signo}</Badge>
            </div>
            <div>
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Proveedor</div>
              <div className="text-[13.5px] font-medium">{n.proveedorNombre || '—'}</div>
              {n.proveedorRif ? <div className="text-[12px] text-slate-500 num">{n.proveedorRif}</div> : null}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-0.5">Nº nota (interno)</div>
                <div className="text-[13px] num font-medium">{n.numeroCompleto}</div>
              </div>
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-0.5">Documento proveedor</div>
                <div className="text-[13px] num font-medium">{n.numeroDocumento || '—'}</div>
              </div>
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-0.5">Nº control</div>
                <div className="text-[13px] num font-medium">{n.numeroControl || '—'}</div>
              </div>
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-0.5">Fecha documento</div>
                <div className="text-[13px] num">{fmtDate(n.fecha)}</div>
              </div>
            </div>
          </div>
          <div className="flex items-start gap-2 text-[11.5px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2">
            <Icon.Lock size={13} className="mt-0.5 shrink-0" />
            <span>Documento append-only: no se edita. {n.tipo === 'nota_credito' ? 'Revierte parte de la compra (Debe CxP / Haber IVA crédito + Inventario).' : 'Carga un concepto adicional (Debe Inventario + IVA crédito / Haber CxP).'} Ajusta la cuenta por pagar del proveedor.</span>
          </div>
        </div>

        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
            <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Concepto</div>
            <div className="text-[13.5px]">{n.concepto || '—'}</div>
          </div>
          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-4 space-y-1.5 text-[13px] max-w-sm ml-auto">
            <TotRow label="Base imponible" value={Math.abs(n.baseImponible)} ccy={ccy} />
            {n.baseExenta ? <TotRow label="Base exenta" value={Math.abs(n.baseExenta)} ccy={ccy} /> : null}
            <TotRow label="IVA crédito fiscal" value={Math.abs(n.iva)} ccy={ccy} />
            <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
              <span className="font-semibold">Total ({meta.signo.toLowerCase()})</span>
              <span className={`num font-semibold private-mask text-[16px] ${meta.color === 'emerald' ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-700 dark:text-amber-300'}`}>{fmtCurrency(Math.abs(n.total), ccy)}</span>
            </div>
          </div>
        </div>
      </div>
    </VistaDetalle>
  )
}

const TotRow = ({ label, value, ccy }) => (
  <div className="flex items-center justify-between"><span className="text-slate-500">{label}</span><span className="num private-mask">{fmtCurrency(value, ccy)}</span></div>
)

/* Modal de emisión de NC/ND de proveedor. Reutilizable:
 *   - desde la pantalla de notas: sin `factura` → selector de factura de compra;
 *   - desde el detalle de una factura de proveedor: `factura` fija (sin selector).
 * Muestra el IVA y el total EN VIVO con la alícuota histórica de la factura. */
export function EmitirNotaCompraModal({ tipo = 'nota_credito', factura, onClose, onSaved, ccy }) {
  const toast = useToast()
  const meta = META[tipo] || META.nota_credito
  const esCredito = tipo === 'nota_credito'

  // Facturas candidatas: si viene una fija, esa; si no, se traen on-demand.
  const [opciones, setOpciones] = useState(factura ? [factura] : undefined)
  const [cargaErr, setCargaErr] = useState(null)
  const [facturaId, setFacturaId] = useState(factura?.id || '')

  const [concepto, setConcepto] = useState('')
  const [monto, setMonto] = useState('')
  const [exento, setExento] = useState(false)
  const [numeroDocumento, setNumeroDocumento] = useState('')
  const [numeroControl, setNumeroControl] = useState('')
  const [fecha, setFecha] = useState(hoyISO())
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (factura) return
    let vivo = true
    api.facturasCompra()
      .then((fc) => { if (vivo) setOpciones(fc || []) })
      .catch((e) => { if (vivo) { setOpciones([]); setCargaErr(e) } })
    return () => { vivo = false }
  }, [factura])

  const cargando = opciones === undefined
  const lista = opciones || []
  const seleccionada = lista.find((f) => f.id === facturaId) || null
  const alic = seleccionada ? alicuotaDe(seleccionada) : 0.16

  const base = Number(monto)
  const baseValida = Number.isFinite(base) && base > 0
  const iva = baseValida && !exento ? base * alic : 0
  const total = baseValida ? base + iva : 0

  const errFac = !facturaId ? 'Elige una factura de compra.' : ''
  const errConcepto = !concepto.trim() ? 'El concepto es obligatorio.' : ''
  const errMonto = !baseValida ? 'El monto base debe ser mayor que 0.' : ''
  const errDoc = !numeroDocumento.trim() ? 'El número del documento del proveedor es obligatorio.' : ''
  const errCtrl = !numeroControl.trim() ? 'El número de control es obligatorio.' : ''
  const errFecha = !fecha ? 'La fecha es obligatoria.' : ''
  const puedeConfirmar = !errFac && !errConcepto && !errMonto && !errDoc && !errCtrl && !errFecha

  const confirmar = async () => {
    setTouched(true)
    if (!puedeConfirmar) return
    setBusy(true)
    const body = {
      concepto: concepto.trim(), monto: base, exento,
      numeroDocumento: numeroDocumento.trim(), numeroControl: numeroControl.trim(), fecha,
    }
    try {
      if (esCredito) await api.emitirNotaCreditoCompra(facturaId, body)
      else await api.emitirNotaDebitoCompra(facturaId, body)
      toast({ title: `${meta.corto} emitida`, body: `${meta.signo} · ${fmtCurrency(total, ccy)}.` })
      await onSaved?.()
      onClose()
    } catch (e) {
      toast({ title: `No se pudo emitir la ${meta.corto.toLowerCase()}`, body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<meta.icon size={18} />}
      title={`Emitir ${meta.corto.toLowerCase()} de proveedor`}
      sub={esCredito ? 'Devolución o descuento: baja lo que le debes al proveedor' : 'Cargo adicional: sube lo que le debes al proveedor'}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant="primary" onClick={confirmar} loading={busy} disabled={cargando || !lista.length} icon={<meta.icon size={16} />}>
          Emitir {meta.corto.toLowerCase()}
        </Button>
      </>}>
      <div className="space-y-3.5">
        <div className="flex items-start gap-2 text-[12.5px] text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>{esCredito
            ? 'Registra la nota de crédito que emitió el proveedor. Baja la cuenta por pagar de esa compra. El IVA usa la alícuota histórica de la factura.'
            : 'Registra la nota de débito que emitió el proveedor. Sube la cuenta por pagar de esa compra. El IVA usa la alícuota histórica de la factura.'}</span>
        </div>

        {cargando ? (
          <div className="py-2"><TableSkeleton rows={2} cols={2} /></div>
        ) : !lista.length ? (
          <Empty framed icon={<Icon.Receipt size={20} />} title="No hay facturas de compra"
            body={cargaErr ? String(cargaErr.message || cargaErr) : 'Registra una factura de compra a un proveedor (en Compras) para poder emitir una nota.'} />
        ) : (
          <>
            {factura ? (
              <Field label="Factura de compra">
                <div className="text-[13px] num rounded-lg border border-slate-200 dark:border-slate-700 px-3 py-2 bg-slate-50 dark:bg-slate-800/60">
                  {etiquetaFactura(factura, ccy)}
                </div>
              </Field>
            ) : (
              <Field label="Factura de compra" required error={touched ? errFac : ''}>
                <Select value={facturaId} onChange={(e) => setFacturaId(e.target.value)} invalid={touched && !!errFac}>
                  <option value="">Selecciona una factura de compra…</option>
                  {lista.map((f) => (
                    <option key={f.id} value={f.id}>{etiquetaFactura(f, ccy)}</option>
                  ))}
                </Select>
              </Field>
            )}

            <Field label="Concepto" required error={touched ? errConcepto : ''}
              hint={esCredito ? 'p. ej. devolución de mercancía dañada' : 'p. ej. flete no incluido'}>
              <Input value={concepto} onChange={(e) => setConcepto(e.target.value)}
                onBlur={() => setTouched(true)} invalid={touched && !!errConcepto}
                placeholder={esCredito ? 'Motivo de la devolución o descuento' : 'Motivo del cargo adicional'} />
            </Field>

            <div className="grid grid-cols-2 gap-3">
              <Field label="Monto base (Bs)" required error={touched ? errMonto : ''}>
                <Input type="number" min={0} step="any" className="num" value={monto}
                  onChange={(e) => setMonto(e.target.value)} onBlur={() => setTouched(true)}
                  invalid={touched && !!errMonto} placeholder="0,00" />
              </Field>
              <Field label="Fecha del documento" required error={touched ? errFecha : ''}>
                <Input type="date" value={fecha} onChange={(e) => setFecha(e.target.value)}
                  onBlur={() => setTouched(true)} invalid={touched && !!errFecha} />
              </Field>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <Field label="Nº documento (proveedor)" required error={touched ? errDoc : ''}>
                <Input value={numeroDocumento} onChange={(e) => setNumeroDocumento(e.target.value)}
                  onBlur={() => setTouched(true)} invalid={touched && !!errDoc} className="num" placeholder="Ej: NC-000123" />
              </Field>
              <Field label="Nº de control" required error={touched ? errCtrl : ''}>
                <Input value={numeroControl} onChange={(e) => setNumeroControl(e.target.value)}
                  onBlur={() => setTouched(true)} invalid={touched && !!errCtrl} className="num" placeholder="Ej: 00-00012345" />
              </Field>
            </div>

            <div className="rounded-lg border border-slate-200 dark:border-slate-700 px-3 py-2">
              <Toggle checked={exento} onChange={setExento} label="Ajuste exento de IVA"
                sub="Marca si el concepto no causa IVA (p. ej. intereses de mora)." />
            </div>

            {/* Total EN VIVO con la alícuota histórica de la factura. */}
            <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 p-3 space-y-1.5 text-[13px]">
              <div className="flex items-center justify-between"><span className="text-slate-500">Base</span><span className="num private-mask">{fmtCurrency(baseValida ? base : 0, ccy)}</span></div>
              <div className="flex items-center justify-between"><span className="text-slate-500">IVA {exento ? '(exento)' : `(${fmtNum(alic * 100, 2)}%)`}</span><span className="num private-mask">{fmtCurrency(iva, ccy)}</span></div>
              <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
                <span className="font-semibold">Total · {meta.signo}</span>
                <span className={`num font-semibold private-mask ${esCredito ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-700 dark:text-amber-300'}`}>{fmtCurrency(total, ccy)}</span>
              </div>
            </div>
          </>
        )}
      </div>
    </Modal>
  )
}

function etiquetaFactura(f, ccy) {
  const numero = f.numeroFactura || f.numeroControl || f.id
  const prov = f.proveedorNombre || 's/proveedor'
  return `${numero} · ${prov} · total ${fmtCurrency(Number(f.total) || 0, ccy)}`
}
