import { useState, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, VistaDetalle, Empty, TableSkeleton } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { EmitirNotaCompraModal } from './NotasCompra.jsx'

// Emitir notas de crédito/débito de proveedor: misma matriz que su consulta.
const ROLES_NOTAS = ['dueno', 'desarrollador', 'contadora']

// Facturas de proveedor: vista de SOLO LECTURA de las facturas FISCALES de compra
// registradas (documento del proveedor, con su nº de control SENIAT). Es la cara
// fiscal de lo que el módulo de Compras registra al facturar una orden recibida:
// no se emiten aquí (append-only, una por orden) — esta pantalla las lista y las
// detalla. La fuente es GET /api/compras/facturas, ya cargada en db.FACTURAS_COMPRA.
//
// Acceso de consulta: Dueña / Desarrollador / Contadora (igual que los libros).
const ROLES_VER = ['dueno', 'desarrollador', 'contadora']

export function FacturasProveedor() {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const ccy = ui.ccy
  const puedeVer = ROLES_VER.includes(ui.rol)

  const facturas = db.FACTURAS_COMPRA || []
  const ordenes = db.ORDENES_COMPRA || []

  const [q, setQ] = useState('')
  const [detalle, setDetalle] = useState(null)

  const ordenDe = (id) => ordenes.find((o) => o.id === id) || null

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    const base = [...facturas]
    // Más recientes primero (por fecha del documento del proveedor).
    base.sort((a, b) => String(b.fecha || '').localeCompare(String(a.fecha || '')))
    if (!term) return base
    return base.filter((f) =>
      (f.numeroControl || '').toLowerCase().includes(term) ||
      (f.numeroFactura || '').toLowerCase().includes(term) ||
      (f.proveedorNombre || '').toLowerCase().includes(term) ||
      (f.proveedorRif || '').toLowerCase().includes(term))
  }, [facturas, q])

  const totalCredito = useMemo(() => facturas.reduce((a, f) => a + (Number(f.iva) || 0), 0), [facturas])

  if (!puedeVer) {
    return <Empty icon={<Icon.Lock size={22} />} title="Sin acceso a las facturas de proveedor"
      body="Las facturas de compra son de consulta de la Dueña, el Desarrollador y la Contadora." />
  }

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las facturas"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  // Detalle a pantalla completa (patrón maestro↔detalle del resto de Facturación).
  if (detalle) {
    return <DetalleFactura factura={detalle} orden={ordenDe(detalle.ordenCompraId)} ccy={ccy}
      puedeNotas={ROLES_NOTAS.includes(ui.rol)} onReload={reload} onVolver={() => setDetalle(null)} />
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Input className="w-72" icon={<Icon.Search size={15} />}
          placeholder="Buscar por nº de control, factura, proveedor o RIF…" value={q} onChange={(e) => setQ(e.target.value)} />
        <div className="ml-auto text-[12px] text-slate-500 hidden sm:block">
          IVA crédito fiscal: <span className="num font-medium text-slate-700 dark:text-slate-200 private-mask">{fmtCurrency(totalCredito, ccy, { max: 0 })}</span>
        </div>
      </div>

      <div className="mb-3 flex items-start gap-2 text-[11.5px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2">
        <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" />
        <span>Las facturas de compra se registran en <strong>Compras</strong> al facturar una orden recibida (nº de control del proveedor). Son append-only: aquí se consultan, no se editan. Alimentan el <strong>Libro de compras</strong> y el IVA crédito fiscal.</span>
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || db.FACTURAS_COMPRA === undefined ? (
          <div className="p-4"><TableSkeleton rows={6} cols={7} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.Receipt size={22} />}
            title={q ? 'Sin resultados' : 'Aún no hay facturas de proveedor'}
            body={q ? 'Prueba con otro término.'
              : 'Registra la factura del proveedor desde Compras, al facturar una orden de compra recibida.'} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Nº control</th>
                  <th className="py-2.5 px-3 font-medium">Nº factura</th>
                  <th className="py-2.5 px-3 font-medium">Proveedor</th>
                  <th className="py-2.5 px-3 font-medium">Fecha</th>
                  <th className="py-2.5 px-3 font-medium">Orden</th>
                  <th className="py-2.5 px-3 font-medium text-right">Base imp.</th>
                  <th className="py-2.5 px-3 font-medium text-right">IVA crédito</th>
                  <th className="py-2.5 px-3 font-medium text-right">Total</th>
                  <th className="py-2.5 px-3"></th>
                </tr>
              </thead>
              <tbody>
                {rows.map((f) => {
                  const o = ordenDe(f.ordenCompraId)
                  return (
                    <tr key={f.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover cursor-pointer" onClick={() => setDetalle(f)}>
                      <td className="py-2.5 px-3 num text-[12.5px] font-medium whitespace-nowrap">{f.numeroControl || '—'}</td>
                      <td className="py-2.5 px-3 num text-[12.5px] text-slate-500 whitespace-nowrap">{f.numeroFactura || '—'}</td>
                      <td className="py-2.5 px-3">
                        <div className="text-[13px] truncate max-w-[200px]">{f.proveedorNombre || '—'}</div>
                        {f.proveedorRif ? <div className="text-[11px] text-slate-400 num">{f.proveedorRif}</div> : null}
                      </td>
                      <td className="py-2.5 px-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(f.fecha)}</td>
                      <td className="py-2.5 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{o?.numeroCompleto || '—'}</td>
                      <td className="py-2.5 px-3 text-right num text-[12.5px] text-slate-500 private-mask">{fmtCurrency(f.baseImponible, ccy)}</td>
                      <td className="py-2.5 px-3 text-right num text-[12.5px] text-slate-500 private-mask">{fmtCurrency(f.iva, ccy)}</td>
                      <td className="py-2.5 px-3 text-right num text-[12.5px] font-medium private-mask">{fmtCurrency(f.total, ccy)}</td>
                      <td className="py-2.5 px-3 text-right"><Icon.ChevRight size={15} className="inline text-slate-300" /></td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} factura(s) de proveedor</div> : null}
    </div>
  )
}

function DetalleFactura({ factura, orden, ccy, puedeNotas, onReload, onVolver }) {
  const f = factura
  // Preferimos las líneas propias de la factura del proveedor (pueden diferir de la
  // orden); si es una factura antigua sin líneas, caemos a las de la orden.
  const lineas = (f.lineas && f.lineas.length) ? f.lineas : (orden?.lineas || [])
  const usaLineasFactura = !!(f.lineas && f.lineas.length)
  const hayDif = Math.abs(Number(f.diferenciaBase) || 0) > 0.005
  const [nota, setNota] = useState(null) // 'nota_credito' | 'nota_debito' | null
  return (
    <VistaDetalle onVolver={onVolver} icon={<Icon.Receipt size={18} />}
      titulo={`Factura ${f.numeroFactura || f.numeroControl || ''}`}
      sub={`Proveedor · ${fmtDate(f.fecha)}`}
      acciones={puedeNotas ? (
        <>
          <Button variant="secondary" size="sm" icon={<Icon.Receipt size={15} />} onClick={() => setNota('nota_credito')}>Nota de crédito</Button>
          <Button variant="secondary" size="sm" icon={<Icon.Receipt size={15} />} onClick={() => setNota('nota_debito')}>Nota de débito</Button>
        </>
      ) : null}>
      <div className="grid grid-cols-1 lg:grid-cols-[340px_1fr] gap-4 items-start">
        {/* Columna izquierda: proveedor, documento y orden */}
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3">
            <div className="flex items-center gap-2 flex-wrap">
              <Badge color="huberp">Factura de compra</Badge>
              <Badge color="emerald" dot>Registrada</Badge>
            </div>

            <div>
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Proveedor</div>
              <div className="text-[13.5px] font-medium">{f.proveedorNombre || '—'}</div>
              {f.proveedorRif ? <div className="text-[12px] text-slate-500 num">{f.proveedorRif}</div> : null}
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-0.5">Nº control</div>
                <div className="text-[13px] num font-medium">{f.numeroControl || '—'}</div>
              </div>
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-0.5">Nº factura</div>
                <div className="text-[13px] num font-medium">{f.numeroFactura || '—'}</div>
              </div>
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-0.5">Fecha documento</div>
                <div className="text-[13px] num">{fmtDate(f.fecha)}</div>
              </div>
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-0.5">Orden asociada</div>
                <div className="text-[13px] num">{orden?.numeroCompleto || '—'}</div>
              </div>
            </div>
          </div>

          <div className="flex items-start gap-2 text-[11.5px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2">
            <Icon.Lock size={13} className="mt-0.5 shrink-0" />
            <span>Documento append-only: no se edita. La base recibida ya se asentó al recibir la orden; la factura agrega el IVA crédito fiscal, la diferencia contra lo recibido (si la hay) y completa la deuda con el proveedor. Correcciones vía nota de crédito/débito.</span>
          </div>
        </div>

        {/* Columna derecha: líneas de la orden y totales de la factura */}
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
            <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">
              {usaLineasFactura ? 'Líneas de la factura' : 'Líneas'} {lineas.length ? `(${lineas.length})` : ''}
            </div>
            {lineas.length ? (
              <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
                {lineas.map((l, i) => {
                  const cant = usaLineasFactura ? (Number(l.cantidad) || 0) : (Number(l.cantidadRecibida) || Number(l.cantidad) || 0)
                  return (
                    <div key={i} className="flex items-center gap-3 px-3 py-2">
                      <div className="flex-1 min-w-0">
                        <div className="text-[13px] font-medium truncate">{l.nombre || l.sku}
                          {l.exento ? <Badge size="sm" color="slate" className="ml-1.5">exento</Badge> : null}</div>
                        <div className="text-[11px] text-slate-400 num">{fmtNum(cant)} × {fmtCurrency(l.costoUnitario, ccy)}</div>
                      </div>
                      <div className="num text-[13px] font-medium private-mask">{fmtCurrency(cant * (Number(l.costoUnitario) || 0), ccy)}</div>
                    </div>
                  )
                })}
              </div>
            ) : (
              <div className="text-[12.5px] text-slate-400">La orden de compra asociada no está disponible en esta vista; los totales fiscales se muestran a la derecha.</div>
            )}
          </div>

          {hayDif ? (
            <div className="rounded-lg border border-amber-300 dark:border-amber-700/60 bg-amber-50/60 dark:bg-amber-900/15 p-3 space-y-1 text-[12.5px] max-w-sm ml-auto">
              <div className="flex items-center justify-between"><span className="text-slate-500">Base recibida (OC)</span><span className="num private-mask">{fmtCurrency(f.baseRecibida, ccy)}</span></div>
              <div className="flex items-center justify-between"><span className="text-slate-500">Base facturada</span><span className="num private-mask">{fmtCurrency((Number(f.baseImponible) || 0) + (Number(f.baseExenta) || 0), ccy)}</span></div>
              <div className="flex items-center justify-between border-t border-amber-200 dark:border-amber-800/60 pt-1">
                <span className="text-amber-700 dark:text-amber-300 font-medium">Diferencia</span>
                <span className="num private-mask text-amber-700 dark:text-amber-300 font-semibold">{Number(f.diferenciaBase) > 0 ? '+' : ''}{fmtCurrency(f.diferenciaBase, ccy)}</span>
              </div>
              <div className="text-[11px] text-amber-700/80 dark:text-amber-300/80 leading-snug">Asentada como diferencia en compras (5202). El inventario conserva el costo recibido.</div>
            </div>
          ) : null}

          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-4 space-y-1.5 text-[13px] max-w-sm ml-auto">
            <TotRow label="Base imponible" value={f.baseImponible} ccy={ccy} />
            {f.baseExenta ? <TotRow label="Base exenta" value={f.baseExenta} ccy={ccy} /> : null}
            <TotRow label="IVA crédito fiscal" value={f.iva} ccy={ccy} />
            <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
              <span className="font-semibold">Total</span><span className="num font-semibold private-mask text-[16px]">{fmtCurrency(f.total, ccy)}</span>
            </div>
          </div>
        </div>
      </div>

      {nota ? (
        <EmitirNotaCompraModal tipo={nota} factura={f} ccy={ccy}
          onClose={() => setNota(null)} onSaved={onReload} />
      ) : null}
    </VistaDetalle>
  )
}

const TotRow = ({ label, value, ccy }) => (
  <div className="flex items-center justify-between"><span className="text-slate-500">{label}</span><span className="num private-mask">{fmtCurrency(value, ccy)}</span></div>
)
