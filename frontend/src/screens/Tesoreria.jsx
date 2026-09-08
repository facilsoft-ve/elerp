import { useState, useEffect, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Select, Empty, PageHeader, useToast, TableSkeleton, Field, Input, Modal, Segmented, useConfirm } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useData, useTasa } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { useRecurso, EstadoRecurso } from '../lib/useRecurso.jsx'
import { METODOS_PAGO } from '../lib/fiscal.js'

/* Tesorería — el dinero, después de la factura.
 *
 * Tres cosas construidas y dos declaradas:
 *   · Cuentas por cobrar: las ventas a crédito con su saldo DERIVADO (total menos
 *     todo lo cobrado) y su antigüedad. Nada de un campo «saldo» que alguien
 *     tenga que acordarse de actualizar.
 *   · Efectivo y bancos: cuánto entró por cada cuenta y cuánto quedó en la
 *     gaveta, restando el vuelto entregado.
 *   · Reporte de IGTF: lo que hay que declarar junto al IVA, derivado de los
 *     documentos.
 *   · Cuentas por pagar: el espejo de por cobrar, el pasivo con proveedores
 *     DERIVADO de las órdenes de compra recibidas, con su saldo neto (adeudado
 *     menos pagado) y el registro de pagos —append-only, corregibles por reverso.
 *   · Enlaces de pago se declara pendiente: necesita las pasarelas.
 */
const TABS = [
  { id: 'cxc', label: 'Cuentas por cobrar', icon: <Icon.Receipt size={15} /> },
  { id: 'cxp', label: 'Cuentas por pagar', icon: <Icon.Cart size={15} /> },
  { id: 'saldos', label: 'Efectivo y bancos', icon: <Icon.Wallet size={15} /> },
  { id: 'igtf', label: 'IGTF', icon: <Icon.Shield size={15} /> },
  { id: 'links', label: 'Enlaces de pago', icon: <Icon.Globe size={15} /> },
]

// La Contadora tiene acceso total en su módulo; el Vendedor solo consulta.
const puedeCobrar = (rol) => ['dueno', 'desarrollador', 'contadora'].includes(rol)

export function Tesoreria({ route }) {
  const [tab, setTab] = useState((route || '').split(':')[1] || 'cxc')
  useEffect(() => { setTab((route || '').split(':')[1] || 'cxc') }, [route])
  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader
        breadcrumb={['Finanzas', TABS.find((t) => t.id === tab)?.label]}
        title="Finanzas"
        sub="Cuentas por cobrar y pagar, efectivo y bancos, IGTF y enlaces de pago: el dinero después de la factura."
        tabs={TABS} activeTab={tab} onTab={setTab} />
      {tab === 'cxc' ? <PorCobrar /> : null}
      {tab === 'cxp' ? <PorPagar /> : null}
      {tab === 'saldos' ? <Saldos /> : null}
      {tab === 'igtf' ? <ReporteIGTF /> : null}
      {tab === 'links' ? (
        <Empty icon={<Icon.Globe size={22} />} title="Enlaces de pago" framed
          body="Cobrar con un enlace y QR (Cashea, Biopago, WayuPay) necesita la integración con cada
            pasarela: credenciales, webhooks de confirmación y conciliación. Ninguna está conectada todavía." />
      ) : null}
    </div>
  )
}

/* --- Cuentas por cobrar --------------------------------------------------- */

function PorCobrar() {
  const { ui } = useUI()
  const toast = useToast()
  const [cobrando, setCobrando] = useState(null)
  const { data: res, loading, error, reload: cargar } = useRecurso(() => api.porCobrar(), [])

  if (loading || error) {
    return <EstadoRecurso loading={loading} error={error} onRetry={cargar} cols={5} rows={4}
      title="No se pudo cargar cuentas por cobrar" />
  }

  return (
    <div>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
        <Kpi label="Total por cobrar" valor={fmtCurrency(res.total, 'VES')} />
        <Kpi label="Vencido" valor={fmtCurrency(res.totalVencido, 'VES')} tono={res.totalVencido > 0 ? 'alerta' : ''} />
        <Kpi label="Cobrado este mes" valor={fmtCurrency(res.cobradoDelMes, 'VES')} tono="dinero" />
        <Kpi label="Clientes con saldo" valor={fmtNum(res.clientesConSaldo, 0)} />
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {res.cuentas.length === 0 ? (
          <Empty icon={<Icon.Receipt size={22} />} title="No hay nada por cobrar"
            body="Todas las facturas están cobradas. Una venta queda por cobrar cuando se emite a crédito desde el punto de venta." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Cliente</th>
                  <th className="py-2.5 pr-3 font-medium">Factura</th>
                  <th className="py-2.5 pr-3 font-medium">Vence</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Total</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Cobrado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Saldo</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {res.cuentas.map((c) => (
                  <tr key={c.documentoId} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2.5 px-3">
                      <div className="font-medium text-[13px]">{c.clienteNombre}</div>
                      <div className="text-[11px] text-slate-400 num">{c.clienteDocumento}</div>
                    </td>
                    <td className="py-2.5 pr-3 mono text-[12px] text-slate-500">{c.numeroCompleto}</td>
                    <td className="py-2.5 pr-3 text-[12.5px]">
                      <div className="num">{c.venceEl}</div>
                      {c.vencida ? (
                        <Badge size="sm" color="red" dot>vencida {fmtNum(c.diasVencido, 0)} día(s)</Badge>
                      ) : <span className="text-[11px] text-slate-400">al día</span>}
                    </td>
                    <td className="py-2.5 pr-3 text-right num private-mask">{fmtCurrency(c.total, 'VES')}</td>
                    <td className="py-2.5 pr-3 text-right num text-slate-500 private-mask">{fmtCurrency(c.cobrado, 'VES')}</td>
                    <td className={`py-2.5 pr-3 text-right num font-semibold private-mask ${c.vencida ? 'text-[#B3362C] dark:text-red-400' : ''}`}>
                      {fmtCurrency(c.saldo, 'VES')}
                    </td>
                    <td className="py-2.5 pr-3 text-right">
                      {puedeCobrar(ui.rol) ? (
                        <Button size="sm" variant="dinero" icon={<Icon.Banknote size={14} />} onClick={() => setCobrando(c)}>
                          Registrar cobro
                        </Button>
                      ) : (
                        <span className="text-[11.5px] text-slate-400">solo consulta</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="mt-3 text-[11.5px] text-slate-400">
        El saldo no es un campo: se calcula como total facturado menos todo lo cobrado. Corregir un cobro
        mal registrado se hace con su reverso, no borrándolo.
      </div>

      {cobrando ? (
        <RegistrarCobroModal cuenta={cobrando} onClose={() => setCobrando(null)}
          onHecho={() => { cargar(); toast({ title: 'Cobro registrado', body: cobrando.clienteNombre }) }} />
      ) : null}
    </div>
  )
}

function RegistrarCobroModal({ cuenta, onClose, onHecho }) {
  const { db } = useData()
  const tasa = useTasa()
  const [f, setF] = useState({ monto: '', moneda: 'VES', metodo: 'pago_movil', cuentaId: '', referencia: '' })
  const [busy, setBusy] = useState(false)
  const [touched, setTouched] = useState({})
  const [apiError, setApiError] = useState('')
  const cuentas = (db.CUENTAS_COBRO || []).filter((c) => c.moneda === f.moneda)

  // Cuánto habría que cobrar en la moneda elegida.
  const enMoneda = f.moneda === 'USD' && tasa.valor > 0 ? cuenta.saldo / tasa.valor : cuenta.saldo

  // Validación inline por campo. `moneda` valida la regla cruzada de la tasa:
  // sin tasa no se puede registrar un cobro en divisas.
  const errs = {
    monto: !(Number(f.monto) > 0) ? 'Escribe el monto cobrado (mayor que cero).' : '',
    moneda: f.moneda !== 'VES' && !tasa.hay ? 'No hay tasa cargada: no se puede cobrar en divisas.' : '',
  }
  const valid = !errs.monto && !errs.moneda
  const blur = (k) => () => setTouched((t) => ({ ...t, [k]: true }))

  const guardar = async () => {
    setTouched({ monto: true, moneda: true })
    if (!valid) return
    setBusy(true); setApiError('')
    try {
      await api.registrarCobro({
        documentoId: cuenta.documentoId, monto: Number(f.monto), moneda: f.moneda,
        metodo: f.metodo, cuentaId: f.cuentaId, referencia: f.referencia.trim(),
      })
      onHecho()
      onClose()
    } catch (e) {
      setApiError(e?.message || 'No se pudo registrar.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Banknote size={18} />}
      title="Registrar cobro" sub={`${cuenta.clienteNombre} · ${cuenta.numeroCompleto}`}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant="dinero" onClick={guardar} loading={busy} disabled={!valid}
          title={valid ? '' : (errs.monto || errs.moneda)} icon={<Icon.Check size={16} />}>Registrar</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 px-3.5 py-3 flex items-center justify-between">
          <span className="text-[12.5px] text-slate-500">Saldo pendiente</span>
          <span className="num text-[16px] font-semibold">{fmtCurrency(cuenta.saldo, 'VES')}</span>
        </div>
        <div className="grid grid-cols-[1fr_90px] gap-2 items-start">
          <Field label="Monto cobrado" required error={touched.monto ? errs.monto : ''}>
            <Input type="number" min="0" step="0.01" value={f.monto} autoFocus placeholder="0,00"
              invalid={touched.monto && !!errs.monto} onBlur={blur('monto')}
              onChange={(e) => setF((s) => ({ ...s, monto: e.target.value }))} />
          </Field>
          <Field label="Moneda">
            <Select value={f.moneda} onChange={(e) => { setF((s) => ({ ...s, moneda: e.target.value, cuentaId: '' })); setTouched((t) => ({ ...t, moneda: true })) }}>
              <option value="VES">Bs</option>
              <option value="USD">US$</option>
            </Select>
          </Field>
        </div>
        {touched.moneda && errs.moneda ? (
          <div className="flex items-start gap-1 text-[11.5px] text-red-600 dark:text-red-400">
            <Icon.CircleAlert size={13} className="mt-0.5 shrink-0" /><span>{errs.moneda}</span>
          </div>
        ) : null}
        <button type="button" onClick={() => setF((s) => ({ ...s, monto: String(Math.round(enMoneda * 100) / 100) }))}
          className="text-[11.5px] font-medium text-elerp-600 hover:text-elerp-700">
          Cobró todo el saldo ({fmtCurrency(enMoneda, f.moneda)})
        </button>
        <Field label="Método">
          <Select value={f.metodo} onChange={(e) => setF((s) => ({ ...s, metodo: e.target.value }))}>
            {METODOS_PAGO.map((m) => <option key={m.value} value={m.value}>{m.label}</option>)}
          </Select>
        </Field>
        <Field label="Cuenta que recibe" hint="vacío = efectivo en caja">
          <Select value={f.cuentaId} onChange={(e) => setF((s) => ({ ...s, cuentaId: e.target.value }))}>
            <option value="">Efectivo en caja</option>
            {cuentas.map((c) => <option key={c.id} value={c.id}>{c.titular} · {c.datos}</option>)}
          </Select>
        </Field>
        <Field label="Referencia" hint="opcional">
          <Input value={f.referencia} onChange={(e) => setF((s) => ({ ...s, referencia: e.target.value }))} className="mono" placeholder="123456" />
        </Field>
        {f.moneda === 'USD' && tasa.hay ? (
          <div className="text-[11.5px] text-slate-400 num">
            Se registra a Bs {fmtNum(tasa.valor, 2)} / US$ ({tasa.fuenteLabel}): la tasa del cobro queda guardada con él.
          </div>
        ) : null}
        {apiError ? (
          <div className="flex items-start gap-1.5 text-[12px] text-red-600 dark:text-red-400">
            <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{apiError}</span>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}

/* --- Cuentas por pagar ---------------------------------------------------- */

/* El espejo de Cuentas por cobrar, del lado del PASIVO: lo que le debemos a los
 * proveedores, DERIVADO de las órdenes de compra con mercancía recibida. El saldo
 * es una PROYECCIÓN del servidor —adeudado (recibido) menos lo pagado—, nunca un
 * campo. Los pagos son un histórico append-only: corregir uno mal registrado se
 * hace con su reverso, no borrándolo. */

// Set simple de métodos para el pago a proveedor cuando la empresa no tiene
// métodos de pago configurados en Configuración.
const METODOS_PAGO_PROVEEDOR = [
  { value: 'efectivo', label: 'Efectivo' },
  { value: 'transferencia', label: 'Transferencia' },
  { value: 'pago_movil', label: 'Pago Móvil' },
  { value: 'divisas', label: 'Divisas' },
]

export function PorPagar() {
  const { ui } = useUI()
  const { db } = useData()
  const toast = useToast()
  const confirm = useConfirm()
  const [res, setRes] = useState(null)
  const [pagos, setPagos] = useState(null)
  const [error, setError] = useState(false)
  const [vista, setVista] = useState('saldos')
  const [pagando, setPagando] = useState(null)
  const [reversando, setReversando] = useState('')

  const cargar = useCallback(() => {
    setError(false)
    api.porPagar()
      .then(setRes)
      .catch(() => { setRes({ proveedores: [], ordenes: [], totalPorPagar: 0, proveedoresConSaldo: 0, ordenesConSaldo: 0 }); setError(true) })
    api.pagosProveedor().then(setPagos).catch(() => setPagos([]))
  }, [])
  useEffect(() => { cargar() }, [cargar])

  const reversar = async (pago) => {
    const ok = await confirm({
      title: 'Reversar pago',
      body: `Se anexará un reverso del pago de ${fmtCurrency(pago.montoBs, ui.ccy)} a ${pago.proveedorNombre}. El pago original queda en el histórico; el saldo del proveedor vuelve a subir por ese monto.`,
      confirmLabel: 'Reversar', tone: 'danger',
    })
    if (!ok) return
    setReversando(pago.id)
    try {
      await api.reversarPagoProveedor(pago.id, 'Reverso desde Cuentas por pagar')
      toast({ title: 'Pago reversado', body: pago.proveedorNombre })
      cargar()
    } catch (e) {
      toast({ title: 'No se pudo reversar', body: e?.message || 'Error', kind: 'error' })
    } finally { setReversando('') }
  }

  if (res === null) return <div className="p-4"><TableSkeleton rows={4} cols={5} /></div>

  return (
    <div>
      <div className="grid grid-cols-2 lg:grid-cols-3 gap-3 mb-4">
        <Kpi label="Total por pagar" valor={fmtCurrency(res.totalPorPagar, ui.ccy)} tono="alerta" />
        <Kpi label="Proveedores con saldo" valor={fmtNum(res.proveedoresConSaldo, 0)} />
        <Kpi label="Órdenes con saldo" valor={fmtNum(res.ordenesConSaldo, 0)} />
      </div>

      <div className="flex items-center justify-between gap-3 mb-3">
        <Segmented value={vista} onChange={setVista} size="sm"
          options={[{ value: 'saldos', label: 'Saldos' }, { value: 'pagos', label: `Pagos${(pagos || []).length ? ' (' + pagos.length + ')' : ''}` }]} />
        {error ? (
          <span className="inline-flex items-center gap-2 text-[12px] text-red-600 dark:text-red-400">
            <Icon.CircleAlert size={13} /> No se pudieron cargar los saldos.
            <Button size="sm" variant="secondary" icon={<Icon.Refresh size={13} />} onClick={cargar}>Reintentar</Button>
          </span>
        ) : null}
      </div>

      {vista === 'saldos' ? (
        <SaldosPorPagar res={res} rol={ui.rol} ccy={ui.ccy} onPagar={setPagando} />
      ) : (
        <HistorialPagos pagos={pagos} ccy={ui.ccy} rol={ui.rol} onReversar={reversar} reversando={reversando} />
      )}

      <div className="mt-3 text-[11.5px] text-slate-400">
        El saldo no es un campo: se calcula como lo adeudado (mercancía recibida, al costo neto de inventario)
        menos todo lo pagado. Corregir un pago mal registrado se hace con su reverso, no borrándolo. El IVA
        crédito del proveedor va aparte.
      </div>

      {pagando ? (
        <RegistrarPagoProveedorModal proveedor={pagando} db={db} ccy={ui.ccy}
          onClose={() => setPagando(null)}
          onHecho={() => { cargar(); toast({ title: 'Pago registrado', body: pagando.proveedorNombre }) }} />
      ) : null}
    </div>
  )
}

function SaldosPorPagar({ res, rol, ccy, onPagar }) {
  return (
    <>
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {(res.proveedores || []).length === 0 ? (
          <Empty icon={<Icon.Cart size={22} />} title="No hay saldos por pagar"
            body="Se generan al recibir mercancía de una orden de compra: la recepción asienta la deuda con el proveedor." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Proveedor</th>
                  <th className="py-2.5 pr-3 font-medium">RIF</th>
                  <th className="py-2.5 pr-3 font-medium text-center">Órdenes</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Adeudado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Pagado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Saldo</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {res.proveedores.map((p) => (
                  <tr key={p.proveedorId} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
                    <td className="py-2.5 px-3 font-medium text-[13px]">{p.proveedorNombre}</td>
                    <td className="py-2.5 pr-3 mono text-[12px] text-slate-500">{p.rif || '—'}</td>
                    <td className="py-2.5 pr-3 text-center num text-[12.5px] text-slate-500">{fmtNum(p.ordenes, 0)}</td>
                    <td className="py-2.5 pr-3 text-right num text-slate-500 private-mask">{fmtCurrency(p.recibido, ccy)}</td>
                    <td className="py-2.5 pr-3 text-right num text-slate-500 private-mask">{fmtCurrency(p.pagado, ccy)}</td>
                    <td className={`py-2.5 pr-3 text-right num font-semibold private-mask ${p.saldo > 0 ? 'text-[#B3362C] dark:text-red-400' : 'text-slate-400'}`}>
                      {fmtCurrency(p.saldo, ccy)}
                    </td>
                    <td className="py-2.5 pr-3 text-right">
                      {p.saldo > 0 ? (
                        puedeCobrar(rol) ? (
                          <Button size="sm" variant="dinero" icon={<Icon.Banknote size={14} />} onClick={() => onPagar(p)}>
                            Registrar pago
                          </Button>
                        ) : (
                          <span className="text-[11.5px] text-slate-400">solo consulta</span>
                        )
                      ) : (
                        <Badge size="sm" color="emerald">al día</Badge>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {(res.ordenes || []).length > 0 ? (
        <div className="mt-4 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <div className="px-3.5 py-2.5 border-b border-slate-200 dark:border-slate-800 text-[12px] font-medium text-slate-500">
            Detalle por orden de compra
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Orden</th>
                  <th className="py-2.5 pr-3 font-medium">Proveedor</th>
                  <th className="py-2.5 pr-3 font-medium">Fecha</th>
                  <th className="py-2.5 pr-3 font-medium">Estado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Adeudado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Pagado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Saldo</th>
                </tr>
              </thead>
              <tbody>
                {res.ordenes.map((o) => (
                  <tr key={o.ordenId} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
                    <td className="py-2.5 px-3 mono text-[12px] text-slate-500">{o.numeroCompleto}</td>
                    <td className="py-2.5 pr-3 text-[13px]">{o.proveedorNombre}</td>
                    <td className="py-2.5 pr-3 text-[12.5px] text-slate-500 num">{fmtDate(o.fecha)}</td>
                    <td className="py-2.5 pr-3">
                      <Badge size="sm" color={o.estado === 'recibida' ? 'emerald' : 'amber'}>
                        {o.estado === 'recibida' ? 'recibida' : 'recibida parcial'}
                      </Badge>
                    </td>
                    <td className="py-2.5 pr-3 text-right num text-slate-500 private-mask">{fmtCurrency(o.recibido, ccy)}</td>
                    <td className="py-2.5 pr-3 text-right num text-slate-500 private-mask">{fmtCurrency(o.pagado, ccy)}</td>
                    <td className={`py-2.5 pr-3 text-right num font-medium private-mask ${o.saldo > 0 ? '' : 'text-slate-400'}`}>{fmtCurrency(o.saldo, ccy)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
    </>
  )
}

function HistorialPagos({ pagos, ccy, rol, onReversar, reversando }) {
  if (pagos === null) return <div className="p-4"><TableSkeleton rows={3} cols={5} /></div>
  return (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
      {pagos.length === 0 ? (
        <Empty icon={<Icon.Banknote size={22} />} title="Aún no se han registrado pagos"
          body="Cuando registres un pago a un proveedor desde la pestaña Saldos, aparecerá aquí con su método y referencia." />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                <th className="py-2.5 px-3 font-medium">Fecha</th>
                <th className="py-2.5 pr-3 font-medium">Proveedor</th>
                <th className="py-2.5 pr-3 font-medium">Método</th>
                <th className="py-2.5 pr-3 font-medium">Referencia</th>
                <th className="py-2.5 pr-3 font-medium text-right">Monto</th>
                <th className="py-2.5 pr-3 font-medium text-right">Acciones</th>
              </tr>
            </thead>
            <tbody>
              {pagos.map((p) => {
                const yaReversado = pagos.some((x) => x.reverso && x.refPagoId === p.id)
                return (
                  <tr key={p.id} className={`border-b border-slate-100 dark:border-slate-800/70 ${p.reverso ? 'bg-red-50/40 dark:bg-red-950/20' : ''}`}>
                    <td className="py-2.5 px-3 text-[12.5px] text-slate-500 num">{fmtDate(p.fecha)}</td>
                    <td className="py-2.5 pr-3 text-[13px]">
                      <div className="font-medium">{p.proveedorNombre}</div>
                      {p.reverso ? <Badge size="sm" color="red">reverso</Badge> : null}
                    </td>
                    <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">{metodoLabelProveedor(p.metodo)}</td>
                    <td className="py-2.5 pr-3 mono text-[12px] text-slate-500">{p.referencia || '—'}</td>
                    <td className={`py-2.5 pr-3 text-right num font-medium private-mask ${p.reverso ? 'text-[#B3362C] dark:text-red-400' : ''}`}>
                      {p.reverso ? '-' : ''}{fmtCurrency(Math.abs(p.montoBs), ccy)}
                    </td>
                    <td className="py-2.5 pr-3 text-right">
                      {p.reverso || yaReversado ? (
                        <span className="text-[11.5px] text-slate-400">{yaReversado ? 'reversado' : '—'}</span>
                      ) : puedeCobrar(rol) ? (
                        <Button size="sm" variant="destructive" icon={<Icon.Refresh size={13} />}
                          loading={reversando === p.id} onClick={() => onReversar(p)}>
                          Reversar
                        </Button>
                      ) : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

const metodoLabelProveedor = (v) =>
  METODOS_PAGO_PROVEEDOR.find((m) => m.value === v)?.label ||
  METODOS_PAGO.find((m) => m.value === v)?.label || v || '—'

function RegistrarPagoProveedorModal({ proveedor, db, ccy, onClose, onHecho }) {
  // Métodos configurados de la empresa (Configuración › Métodos de pago) si los
  // hay; si no, un set simple. El backend guarda `metodo` como texto libre.
  const metodos = (() => {
    const conf = (db.METODOS_PAGO || []).filter((m) => m.activo)
    if (conf.length) return conf.map((m) => ({ value: m.nombre, label: m.nombre }))
    return METODOS_PAGO_PROVEEDOR
  })()
  const [monto, setMonto] = useState(String(proveedor.saldo))
  const [metodo, setMetodo] = useState(metodos[0]?.value || 'efectivo')
  const [referencia, setReferencia] = useState('')
  const [busy, setBusy] = useState(false)
  const [tocado, setTocado] = useState(false)
  const [apiError, setApiError] = useState('')

  const excede = Number(monto) > proveedor.saldo + 0.001
  // Validación inline: monto obligatorio (>0) y sin exceder el saldo pendiente.
  const errMonto = !(Number(monto) > 0) ? 'Escribe el monto a pagar (mayor que cero).'
    : excede ? 'El pago no puede exceder el saldo pendiente.' : ''
  const valid = !errMonto

  const guardar = async () => {
    setTocado(true)
    if (!valid) return
    setBusy(true); setApiError('')
    try {
      await api.registrarPagoProveedor({
        proveedorId: proveedor.proveedorId, montoBs: Number(monto),
        metodo, referencia: referencia.trim(),
      })
      onHecho()
      onClose()
    } catch (e) {
      setApiError(e?.message || 'No se pudo registrar el pago.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Banknote size={18} />}
      title="Registrar pago" sub={proveedor.proveedorNombre}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button variant="dinero" onClick={guardar} loading={busy} disabled={!valid}
          title={valid ? '' : errMonto} icon={<Icon.Check size={16} />}>Registrar</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 px-3.5 py-3 flex items-center justify-between">
          <span className="text-[12.5px] text-slate-500">Saldo pendiente</span>
          <span className="num text-[16px] font-semibold">{fmtCurrency(proveedor.saldo, ccy)}</span>
        </div>
        <Field label="Monto a pagar" required hint={`máximo ${fmtCurrency(proveedor.saldo, ccy)}`}
          error={tocado ? errMonto : ''}>
          <Input type="number" min="0" step="0.01" value={monto} autoFocus placeholder="0,00"
            invalid={tocado && !!errMonto} onBlur={() => setTocado(true)}
            onChange={(e) => setMonto(e.target.value)} />
        </Field>
        <button type="button" onClick={() => setMonto(String(proveedor.saldo))}
          className="text-[11.5px] font-medium text-elerp-600 hover:text-elerp-700">
          Pagar todo el saldo ({fmtCurrency(proveedor.saldo, ccy)})
        </button>
        <Field label="Método">
          <Select value={metodo} onChange={(e) => setMetodo(e.target.value)}>
            {metodos.map((m) => <option key={m.value} value={m.value}>{m.label}</option>)}
          </Select>
        </Field>
        <Field label="Referencia" hint="opcional">
          <Input value={referencia} onChange={(e) => setReferencia(e.target.value)} className="mono" placeholder="123456" />
        </Field>
        {apiError ? (
          <div className="flex items-start gap-1.5 text-[12px] text-red-600 dark:text-red-400">
            <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{apiError}</span>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}

/* --- Efectivo y bancos ---------------------------------------------------- */

function Saldos() {
  const { data: res, loading, error, reload } = useRecurso(() => api.saldosTesoreria(), [])
  if (loading || error) {
    return <EstadoRecurso loading={loading} error={error} onRetry={reload} cols={3} rows={4}
      title="No se pudo cargar efectivo y bancos" />
  }

  return (
    <div>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
        <Kpi label="Efectivo en caja (Bs)" valor={fmtCurrency(res.efectivoBs, 'VES')} />
        <Kpi label="Efectivo en divisas" valor={fmtCurrency(res.efectivoUsd, 'USD')} />
        <Kpi label="Total cobrado" valor={fmtCurrency(res.totalBs, 'VES')} tono="dinero" />
        <Kpi label="Vuelto entregado" valor={fmtCurrency(res.vueltoEntregadoBs, 'VES')} />
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {(res.cuentas || []).length === 0 ? (
          <Empty icon={<Icon.Wallet size={22} />} title="Sin cuentas de cobro"
            body="Registra tus cuentas (banco, pago móvil, Zelle) para saber por dónde entra cada cobro." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Cuenta</th>
                  <th className="py-2.5 pr-3 font-medium">Tipo</th>
                  <th className="py-2.5 pr-3 font-medium text-center">Operaciones</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Entró</th>
                  <th className="py-2.5 pr-3 font-medium text-right">En bolívares</th>
                </tr>
              </thead>
              <tbody>
                {res.cuentas.map((c) => (
                  <tr key={c.cuentaId} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2.5 px-3">
                      <div className="font-medium text-[13px]">{c.titular}</div>
                      <div className="text-[11px] text-slate-400 num">{c.datos}</div>
                    </td>
                    <td className="py-2.5 pr-3"><Badge size="sm" color="slate">{c.tipo}</Badge></td>
                    <td className="py-2.5 pr-3 text-center num text-[12.5px] text-slate-500">{fmtNum(c.operaciones, 0)}</td>
                    <td className="py-2.5 pr-3 text-right num private-mask">{fmtCurrency(c.entradas, c.moneda)}</td>
                    <td className="py-2.5 pr-3 text-right num font-medium private-mask">{fmtCurrency(c.entradasBs, 'VES')}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
        <Icon.CircleAlert size={15} className="mt-0.5 shrink-0 text-slate-400" />
        <span>
          Esto es <strong>lo que entró</strong> por cada medio, no el saldo del banco: la conciliación
          bancaria (comparar contra el extracto) todavía no está construida. El efectivo ya tiene restado
          el vuelto entregado, para que cuadre con lo que hay en la gaveta.
        </span>
      </div>
    </div>
  )
}

/* --- Reporte de IGTF ------------------------------------------------------ */

function ReporteIGTF() {
  const { data, loading, error, reload } = useRecurso(() => api.reporteIGTF(), [])
  if (loading || error) {
    return <EstadoRecurso loading={loading} error={error} onRetry={reload} cols={4} rows={3}
      title="No se pudo cargar el reporte de IGTF" />
  }

  return (
    <div>
      <div className="grid grid-cols-2 lg:grid-cols-3 gap-3 mb-4">
        <Kpi label="IGTF a declarar" valor={fmtCurrency(data.total, 'VES')} tono="alerta" />
        <Kpi label="Operaciones en divisas" valor={fmtNum((data.operaciones || []).length, 0)} />
      </div>
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {(data.operaciones || []).length === 0 ? (
          <Empty icon={<Icon.Shield size={22} />} title="Sin operaciones con IGTF"
            body="El IGTF del 3% aparece cuando un cliente paga parte de la factura en divisas." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Factura</th>
                  <th className="py-2.5 pr-3 font-medium">Fecha</th>
                  <th className="py-2.5 pr-3 font-medium">Cliente</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Base en divisas</th>
                  <th className="py-2.5 pr-3 font-medium text-right">IGTF 3%</th>
                </tr>
              </thead>
              <tbody>
                {data.operaciones.map((o) => (
                  <tr key={o.documentoId} className={`border-b border-slate-100 dark:border-slate-800/70 ${o.anulada ? 'opacity-55' : ''}`}>
                    <td className="py-2.5 px-3 mono text-[12px]">
                      {o.numeroCompleto}
                      {o.anulada ? <Badge size="sm" color="red" className="ml-2">anulada</Badge> : null}
                    </td>
                    <td className="py-2.5 pr-3 text-[12.5px] text-slate-500 num">{fmtDate(o.fecha)}</td>
                    <td className="py-2.5 pr-3 text-[13px]">{o.clienteNombre}</td>
                    <td className="py-2.5 pr-3 text-right num text-slate-500">{fmtCurrency(o.baseDivisas, 'VES')}</td>
                    <td className="py-2.5 pr-3 text-right num font-medium">{fmtCurrency(o.igtf, 'VES')}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <div className="mt-3 text-[11.5px] text-slate-400">
        Se deriva de los documentos emitidos, así que no puede desincronizarse de lo facturado. Las anuladas
        se listan pero no suman al total a declarar.
      </div>
    </div>
  )
}

const Kpi = ({ label, valor, tono }) => (
  <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-3.5">
    <div className="text-[12px] font-medium text-slate-500">{label}</div>
    <div className={`font-display text-[20px] font-bold num mt-1 private-mask
      ${tono === 'alerta' ? 'text-[#B3362C] dark:text-red-400' : tono === 'dinero' ? 'text-teal-600 dark:text-teal-400' : ''}`}>
      {valor}
    </div>
  </div>
)
