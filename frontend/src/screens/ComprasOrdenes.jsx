import { useState, useMemo, useRef, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, VistaDetalle, Modal, Empty, TableSkeleton, useToast, Field, Segmented, Toggle } from '../components/primitives.jsx'
import { TablaDatos } from '../components/TablaDatos.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { calcularTotales, IVA_TASA } from '../lib/fiscal.js'
import { porCodigo } from '../lib/precio.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { ModalRecepcion } from './ComprasRecepcion.jsx'
import { ComprobanteModal, comprobanteDeCompra } from '../components/ComprobantePDF.jsx'

/* Órdenes de compra. Máquina de estados append-only:
 *   borrador → confirmada → recibida_parcial → recibida   (o cancelada)
 * Al recibir mercancía se anexan entradas al inventario; nada se edita en sitio. */
export const ESTADO_META = {
  borrador: { label: 'Borrador', color: 'slate' },
  confirmada: { label: 'Confirmada', color: 'blue' },
  recibida_parcial: { label: 'Recibida parcial', color: 'amber' },
  recibida: { label: 'Recibida', color: 'emerald' },
  cancelada: { label: 'Cancelada', color: 'rose' },
}

const FILTROS = [
  { value: 'todos', label: 'Todos los estados' },
  { value: 'borrador', label: 'Borradores' },
  { value: 'confirmada', label: 'Confirmadas' },
  { value: 'recibida_parcial', label: 'Recibidas parcial' },
  { value: 'recibida', label: 'Recibidas' },
  { value: 'cancelada', label: 'Canceladas' },
]

// Escritura sólo dueño/desarrollador; contadora lee.
export const puedeGestionar = (rol) => ['dueno', 'desarrollador'].includes(rol)
const ccyDe = (oc) => oc?.moneda || 'VES'

export function ComprasOrdenes() {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const ordenes = db.ORDENES_COMPRA
  const facturas = db.FACTURAS_COMPRA

  const [q, setQ] = useState('')
  const [filtro, setFiltro] = useState('todos')
  const [vista, setVista] = useState('ordenes')
  const [nueva, setNueva] = useState(false)
  const [detalle, setDetalle] = useState(null)
  const [recibir, setRecibir] = useState(null)
  const [cancelar, setCancelar] = useState(null)
  const [facturar, setFacturar] = useState(null)
  const [pdf, setPdf] = useState(null)  // orden a previsualizar como PDF
  const [busyId, setBusyId] = useState('')

  const gestiona = puedeGestionar(ui.rol)

  // Índice de facturas por orden: así sabemos qué OC ya está facturada (el backend
  // no lo repite en la orden) sin recalcular nada en cada fila.
  const facturaPorOrden = useMemo(() => {
    const m = new Map()
    for (const f of facturas || []) if (f?.ordenCompraId) m.set(f.ordenCompraId, f)
    return m
  }, [facturas])

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return (ordenes || []).filter((o) => {
      const okQ = !term
        || (o.numeroCompleto || '').toLowerCase().includes(term)
        || (o.proveedorNombre || '').toLowerCase().includes(term)
      const okF = filtro === 'todos' || o.estado === filtro
      return okQ && okF
    })
  }, [ordenes, q, filtro])

  const confirmar = async (oc) => {
    setBusyId(oc.id)
    try {
      await api.confirmarOrdenCompra(oc.id)
      toast({ title: 'Orden confirmada', body: `${oc.numeroCompleto} lista para recibir mercancía.` })
      await reload()
    } catch (e) {
      toast({ title: 'No se pudo confirmar', body: e?.message || 'Error', kind: 'error' })
    } finally {
      setBusyId('')
    }
  }

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las órdenes de compra"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  // Alta: formulario a pantalla completa (estilo Odoo) que reemplaza la lista.
  if (nueva) {
    return <FormOrdenCompra onVolver={() => setNueva(false)} onSaved={reload} toast={toast} />
  }

  // Detalle a PANTALLA COMPLETA: al abrir una orden, la vista de detalle reemplaza
  // la lista y ocupa el ancho principal, con «‹ Volver» para regresar.
  if (detalle) {
    return (
      <div>
        <DetalleOrden oc={detalle} gestiona={gestiona} sedes={db.SEDES || []} factura={facturaPorOrden.get(detalle.id)} onVolver={() => setDetalle(null)}
          onConfirmar={() => { setDetalle(null); confirmar(detalle) }}
          onRecibir={() => { setDetalle(null); setRecibir(detalle) }}
          onCancelar={() => { setDetalle(null); setCancelar(detalle) }}
          onFacturar={() => { setFacturar(detalle); setDetalle(null) }}
          onPDF={() => setPdf(detalle)} />
        {recibir ? <ModalRecepcion orden={recibir} onClose={() => setRecibir(null)} onSaved={reload} toast={toast} /> : null}
        {cancelar ? <CancelarModal oc={cancelar} onClose={() => setCancelar(null)} onSaved={reload} toast={toast} /> : null}
        {facturar ? <ModalFacturaCompra oc={facturar} onClose={() => setFacturar(null)} onSaved={reload} toast={toast} /> : null}
        {pdf ? (
          <ComprobanteModal open onClose={() => setPdf(null)} empresa={db.EMPRESA}
            data={comprobanteDeCompra(pdf, db.EMPRESA, (db.PROVEEDORES || []).find((p) => p.id === pdf.proveedorId), facturaPorOrden.get(pdf.id))} />
        ) : null}
      </div>
    )
  }

  if (vista === 'facturas') {
    return (
      <div>
        <div className="flex items-center gap-2 flex-wrap mb-3">
          <Segmented options={[{ value: 'ordenes', label: 'Órdenes' }, { value: 'facturas', label: 'Facturas' }]} value={vista} onChange={setVista} />
        </div>
        <FacturasCompra facturas={facturas} loading={loading || facturas === undefined} />
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Segmented options={[{ value: 'ordenes', label: 'Órdenes' }, { value: 'facturas', label: 'Facturas' }]} value={vista} onChange={setVista} />
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por número o proveedor…" value={q} onChange={(e) => setQ(e.target.value)} />
        <Select className="!w-52" value={filtro} onChange={(e) => setFiltro(e.target.value)}>
          {FILTROS.map((f) => <option key={f.value} value={f.value}>{f.label}</option>)}
        </Select>
        {gestiona ? <Button className="ml-auto" icon={<Icon.Plus size={16} />} onClick={() => setNueva(true)}>Nueva orden</Button> : null}
      </div>

      <TablaDatos
        rows={rows}
        loading={loading || ordenes === undefined}
        rowKey={(o) => o.id}
        onRowClick={(o) => setDetalle(o)}
        rowClassName={(o) => (o.estado === 'cancelada' ? 'opacity-60' : '')}
        csvName="ordenes-compra"
        initialSort={{ key: 'creada', dir: -1 }}
        footerLabel={(n) => `${fmtNum(n, 0)} orden(es)`}
        empty={<Empty icon={<Icon.ClipboardList size={22} />}
          title={q || filtro !== 'todos' ? 'Sin resultados' : 'Aún no hay órdenes de compra'}
          body={q || filtro !== 'todos' ? 'Prueba con otro término o filtro.' : 'Crea una orden de compra: la confirmas, y al recibir la mercancía se suma al inventario.'}
          cta={gestiona && !q && filtro === 'todos' ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNueva(true)}>Nueva orden</Button> : null} />}
        columns={[
          {
            key: 'numero', header: 'Número', sortable: true,
            sortValue: (o) => o.numeroCompleto || '', csv: (o) => o.numeroCompleto || '',
            tdClassName: 'num text-[12.5px] font-medium', cell: (o) => o.numeroCompleto || '—',
          },
          {
            key: 'proveedor', header: 'Proveedor', sortable: true,
            sortValue: (o) => o.proveedorNombre || '', csv: (o) => o.proveedorNombre || '',
            cell: (o) => <div className="text-[13px] truncate max-w-[220px]">{o.proveedorNombre || '—'}</div>,
          },
          {
            key: 'estado', header: 'Estado', align: 'center', sortable: true,
            sortValue: (o) => (ESTADO_META[o.estado]?.label || o.estado || ''),
            csv: (o) => (ESTADO_META[o.estado]?.label || o.estado || ''),
            cell: (o) => { const em = ESTADO_META[o.estado] || { label: o.estado, color: 'slate' }; return <Badge size="sm" color={em.color} dot>{em.label}</Badge> },
          },
          {
            key: 'iva', header: 'IVA', align: 'right', sortable: true,
            sortValue: (o) => Number(o.iva) || 0, csv: (o) => Number(o.iva) || 0,
            tdClassName: 'num text-slate-500 private-mask',
            cell: (o) => <span title="IVA estimado de la compra">{fmtCurrency(o.iva, ccyDe(o))}</span>,
          },
          {
            key: 'total', header: 'Total', align: 'right', sortable: true,
            sortValue: (o) => Number(o.total) || 0, csv: (o) => Number(o.total) || 0,
            tdClassName: 'num font-medium private-mask', cell: (o) => fmtCurrency(o.total, ccyDe(o)),
          },
          {
            key: 'creada', header: 'Creada', sortable: true,
            sortValue: (o) => o.creada || '', csv: (o) => fmtDate(o.creada),
            tdClassName: 'whitespace-nowrap text-[12.5px] text-slate-500 num', cell: (o) => fmtDate(o.creada),
          },
          {
            key: 'acciones', header: 'Acciones', align: 'right', stop: true, csv: false,
            tdClassName: 'whitespace-nowrap',
            cell: (o) => (
              <Acciones oc={o} gestiona={gestiona} busy={busyId === o.id} factura={facturaPorOrden.get(o.id)}
                onConfirmar={() => confirmar(o)} onRecibir={() => setRecibir(o)}
                onCancelar={() => setCancelar(o)} onFacturar={() => setFacturar(o)} onVer={() => setDetalle(o)} />
            ),
          },
        ]} />

      {recibir ? <ModalRecepcion orden={recibir} onClose={() => setRecibir(null)} onSaved={reload} toast={toast} /> : null}
      {cancelar ? <CancelarModal oc={cancelar} onClose={() => setCancelar(null)} onSaved={reload} toast={toast} /> : null}
      {facturar ? <ModalFacturaCompra oc={facturar} onClose={() => setFacturar(null)} onSaved={reload} toast={toast} /> : null}
      {pdf ? (
        <ComprobanteModal open onClose={() => setPdf(null)} empresa={db.EMPRESA}
          data={comprobanteDeCompra(pdf, db.EMPRESA, (db.PROVEEDORES || []).find((p) => p.id === pdf.proveedorId), facturaPorOrden.get(pdf.id))} />
      ) : null}
    </div>
  )
}

// Botonera por fila según estado y permiso. `factura` (si existe) marca la OC como
// ya facturada: se muestra el badge y se oculta la acción de registrar factura.
function Acciones({ oc, gestiona, busy, factura, onConfirmar, onRecibir, onCancelar, onFacturar, onVer }) {
  const btn = 'h-7 px-2 inline-flex items-center gap-1 rounded-lg text-[12px]'
  const facturarBtn = (
    <button onClick={onFacturar} title="Registrar factura del proveedor" className={`${btn} text-elerp-600 hover:bg-elerp-50 dark:hover:bg-elerp-900/30 font-semibold ring-focus`}>
      <Icon.Receipt size={14} /> Registrar factura
    </button>
  )
  // Ya facturada: badge (con el Nº de control) y, si aún queda por recibir, el botón de recibir.
  if (factura) {
    return (
      <div className="inline-flex items-center gap-1.5">
        <Badge size="sm" color="emerald" dot>Facturada · {factura.numeroControl || factura.numeroFactura || ''}</Badge>
        {gestiona && oc.estado === 'recibida_parcial'
          ? <button onClick={onRecibir} title="Recibir mercancía" aria-label={`Recibir mercancía de ${oc.numeroCompleto || 'la orden'}`} className={`${btn} text-teal-700 dark:text-teal-300 hover:bg-teal-50 dark:hover:bg-teal-900/20 ring-focus`}><Icon.Inbox size={14} /></button>
          : <button onClick={onVer} title="Ver" aria-label={`Ver la orden ${oc.numeroCompleto || ''}`} className={`${btn} text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus`}><Icon.Eye size={14} /></button>}
      </div>
    )
  }
  if (!gestiona || oc.estado === 'cancelada') {
    return <button onClick={onVer} title="Ver" className={`${btn} text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus`}><Icon.Eye size={14} /> Ver</button>
  }
  if (oc.estado === 'borrador') {
    return (
      <div className="inline-flex items-center gap-1">
        <button onClick={onConfirmar} disabled={busy} title="Confirmar orden" className={`${btn} text-elerp-600 hover:bg-elerp-50 dark:hover:bg-elerp-900/30 disabled:opacity-50 ring-focus`}>
          {busy ? <span className="spin inline-flex"><Icon.Refresh size={13} /></span> : <Icon.Check size={14} />} Confirmar
        </button>
        <button onClick={onCancelar} title="Cancelar" aria-label="Cancelar la orden" className={`${btn} text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-900/20 ring-focus`}><Icon.CircleX size={14} /></button>
      </div>
    )
  }
  // Recibida por completo (sin factura aún): solo queda registrar la factura.
  if (oc.estado === 'recibida') {
    return <div className="inline-flex items-center gap-1">{facturarBtn}</div>
  }
  // confirmada / recibida_parcial
  return (
    <div className="inline-flex items-center gap-1">
      <button onClick={onRecibir} title="Recibir mercancía" className={`${btn} text-teal-700 dark:text-teal-300 hover:bg-teal-50 dark:hover:bg-teal-900/20 font-semibold ring-focus`}><Icon.Inbox size={14} /> Recibir</button>
      {oc.estado === 'recibida_parcial' ? facturarBtn : null}
      <button onClick={onCancelar} title="Cancelar" aria-label="Cancelar la orden" className={`${btn} text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-900/20 ring-focus`}><Icon.CircleX size={14} /></button>
    </div>
  )
}

const COND_PAGO = ['Contado', '15 días', '30 días', '60 días']

/* Nueva orden de compra — formulario a PANTALLA COMPLETA (estilo Odoo). Es una
 * COMPRA: se captura el COSTO unitario, no el precio de venta. Los montos
 * definitivos los calcula el servidor; acá se previsualizan con las mismas reglas
 * (IVA sólo sobre la base imponible; las líneas exentas no lo causan). */
function FormOrdenCompra({ onVolver, onSaved, toast }) {
  const { db } = useData()
  // Los combos no se compran (empaquetado de venta; el backend rechaza el SKU combo
  // en una OC). Se excluyen del selector.
  const productos = (db.PRODUCTOS || []).filter((p) => !p.esCombo)
  const proveedores = (db.PROVEEDORES || []).filter((p) => p.activo)
  const sedes = db.SEDES || []

  const [proveedorId, setProveedorId] = useState('')
  const [sedeId, setSedeId] = useState(db.SEDE_ACTIVA?.id || sedes[0]?.id || '')
  const [condicionesPago, setCondicionesPago] = useState('Contado')
  const [notas, setNotas] = useState('')
  // Líneas: { sku, nombre, cantidad, costoUnitario, exento }
  const [lineas, setLineas] = useState([])
  const [q, setQ] = useState('')
  const [sel, setSel] = useState(0)
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)
  const searchRef = useRef(null)

  const resultados = useMemo(() => {
    const term = q.trim().toLowerCase()
    if (!term) return []
    return productos
      .filter((p) => p.nombre.toLowerCase().includes(term)
        || (p.sku || '').toLowerCase().includes(term)
        || (p.codigoBarras || '').includes(term))
      .slice(0, 8)
  }, [q, productos])

  useEffect(() => { setSel(0) }, [q])

  const addProducto = (p) => {
    if (!p) return
    setLineas((c) => {
      const i = c.findIndex((l) => l.sku === p.sku)
      if (i >= 0) return c.map((l, j) => (j === i ? { ...l, cantidad: l.cantidad + 1 } : l))
      const costo = Number(p.costo ?? p.costoPromedio ?? 0) || 0
      return [...c, { sku: p.sku, nombre: p.nombre, cantidad: 1, costoUnitario: costo, exento: !!p.exentoIva }]
    })
    setQ('')
    searchRef.current?.focus()
  }

  const onSearchKey = (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setSel((s) => Math.min(s + 1, resultados.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)) }
    else if (e.key === 'Enter') {
      e.preventDefault()
      const exacto = porCodigo(productos, q)
      addProducto(exacto || resultados[sel])
    }
  }

  const setLinea = (sku, k, v) => setLineas((c) => c.map((l) => (l.sku === sku ? { ...l, [k]: v } : l)))
  const rmLinea = (sku) => setLineas((c) => c.filter((l) => l.sku !== sku))
  const netoLinea = (l) => (Number(l.cantidad) || 0) * (Number(l.costoUnitario) || 0)
  // IVA estimado del renglón: alícuota vigente sobre el neto; 0 si es exento. La
  // OC no es un documento fiscal (la factura del proveedor se registra aparte);
  // el servidor fija el monto definitivo con las mismas reglas.
  const ivaLinea = (l) => (l.exento ? 0 : netoLinea(l) * IVA_TASA)

  const totales = useMemo(() => calcularTotales(
    lineas.map((l) => ({ cantidad: Number(l.cantidad) || 0, precioUnitario: Number(l.costoUnitario) || 0, exento: l.exento })),
    [], 0,
  ), [lineas])

  const errProveedor = !proveedorId ? 'Elige un proveedor.' : ''
  const errSede = !sedeId ? 'Elige la sede que recibe la mercancía.' : ''
  const errLineas = lineas.length === 0 ? 'Agrega al menos un producto.'
    : lineas.some((l) => !(Number(l.cantidad) > 0)) ? 'Todas las cantidades deben ser mayores a 0.'
    : lineas.some((l) => Number(l.costoUnitario) < 0) ? 'El costo unitario no puede ser negativo.'
    : ''
  const err = errProveedor || errSede || errLineas

  const guardar = async () => {
    setTouched(true)
    if (err) return
    setBusy(true)
    try {
      const oc = await api.crearOrdenCompra({
        proveedorId, sedeId, condicionesPago, notas: notas.trim() || undefined,
        lineas: lineas.map((l) => ({ sku: l.sku, cantidad: Number(l.cantidad), costoUnitario: Number(l.costoUnitario), exento: !!l.exento })),
      })
      toast({ title: 'Orden de compra creada', body: oc?.numeroCompleto ? `${oc.numeroCompleto} en borrador.` : 'Guardada en borrador.' })
      await onSaved()
      onVolver()
    } catch (e) {
      toast({ title: 'No se pudo crear', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <div>
      {/* Encabezado + acciones */}
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <button onClick={onVolver}
          className="h-9 px-2.5 inline-flex items-center gap-1.5 rounded-lg text-[13px] font-medium text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus">
          <Icon.ChevLeft size={16} /> Volver
        </button>
        <div className="flex-1 min-w-0">
          <h2 className="font-display text-[20px] font-bold tracking-tight truncate">Nueva orden de compra</h2>
          <div className="text-[12px] text-slate-500">Se guarda en borrador; al confirmarla queda lista para recibir.</div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" onClick={onVolver}>Cancelar</Button>
          <Button onClick={guardar} loading={busy} disabled={!!err} title={err || 'Guardar en borrador'} icon={<Icon.Check size={16} />}>Guardar orden</Button>
        </div>
      </div>

      {/* Cabecera */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 mb-4">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          <div className="lg:col-span-2">
            <Field label="Proveedor" required error={touched ? errProveedor : ''}>
              <Select value={proveedorId} onChange={(e) => setProveedorId(e.target.value)}>
                <option value="">Elige proveedor…</option>
                {proveedores.map((p) => <option key={p.id} value={p.id}>{p.nombre}{p.documento ? ` · ${p.documento}` : ''}</option>)}
              </Select>
            </Field>
            {proveedores.length === 0 ? (
              <div className="mt-1.5 text-[11.5px] text-amber-700 dark:text-amber-400">No hay proveedores activos. Crea uno en la pestaña Proveedores.</div>
            ) : null}
          </div>
          <Field label="Sede que recibe" required error={touched ? errSede : ''}>
            <Select value={sedeId} onChange={(e) => setSedeId(e.target.value)}>
              <option value="">Elige sede…</option>
              {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
            </Select>
          </Field>
          <Field label="Condiciones de pago">
            <Select value={condicionesPago} onChange={(e) => setCondicionesPago(e.target.value)}>
              {COND_PAGO.map((c) => <option key={c} value={c}>{c}</option>)}
            </Select>
          </Field>
        </div>
      </div>

      {/* Buscador + líneas */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 mb-4">
        <label className="block text-[12px] font-medium text-slate-500 mb-1">Agregar productos</label>
        <div className="relative mb-3">
          <Input ref={searchRef} icon={<Icon.Search size={15} />} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onSearchKey}
            placeholder="Buscar por nombre, SKU o código… (Enter para agregar)" />
          {q && resultados.length ? (
            <div className="absolute z-20 mt-1 w-full max-w-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-modal overflow-hidden">
              {resultados.map((p, i) => (
                <button key={p.id || p.sku} onClick={() => addProducto(p)} onMouseEnter={() => setSel(i)}
                  className={`w-full flex items-center gap-3 px-3 h-11 text-left ${i === sel ? 'bg-elerp-50 dark:bg-elerp-900/40' : ''}`}>
                  <Icon.Package size={16} className="text-slate-400" />
                  <div className="flex-1 min-w-0">
                    <div className="text-[13px] font-medium truncate">{p.nombre}</div>
                    <div className="text-[11px] text-slate-400 num">{p.sku}{p.exentoIva ? ' · exento' : ''}</div>
                  </div>
                </button>
              ))}
            </div>
          ) : q && !resultados.length ? (
            <div className="absolute z-20 mt-1 w-full max-w-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-modal px-4 py-3 text-center text-[13px] text-slate-500">Sin resultados para “{q}”.</div>
          ) : null}
        </div>

        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          {lineas.length === 0 ? (
            <div className="px-4 py-10 text-center text-[13px] text-slate-500">Sin líneas todavía. Busca un producto arriba y presiona Enter para agregarlo.</div>
          ) : (
            <table className="w-full text-sm min-w-[800px]">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2 px-3 font-medium">Producto</th>
                  <th className="py-2 pr-3 font-medium text-center w-20">Cantidad</th>
                  <th className="py-2 pr-3 font-medium text-right w-32">Costo unit.</th>
                  <th className="py-2 pr-3 font-medium text-center w-20">Exento</th>
                  <th className="py-2 pr-3 font-medium text-right w-32">Subtotal</th>
                  <th className="py-2 pr-3 font-medium text-right w-32">Impuestos (IVA)</th>
                  <th className="py-2 pr-3 w-9"></th>
                </tr>
              </thead>
              <tbody>
                {lineas.map((l) => (
                  <tr key={l.sku} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2 px-3 align-top">
                      <div className="font-medium text-[13px]">{l.nombre}</div>
                      <div className="text-[11px] text-slate-400 num">{l.sku}</div>
                    </td>
                    <td className="py-2 pr-3">
                      <input type="number" min="0" step="1" value={l.cantidad} onChange={(e) => setLinea(l.sku, 'cantidad', Number(e.target.value))}
                        className="w-16 h-8 text-center rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num ring-focus mx-auto block" />
                    </td>
                    <td className="py-2 pr-3 text-right">
                      <input type="number" min="0" step="0.01" value={l.costoUnitario} onChange={(e) => setLinea(l.sku, 'costoUnitario', Number(e.target.value))}
                        className="w-28 h-8 text-right px-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num ring-focus private-mask" />
                    </td>
                    <td className="py-2 pr-3 text-center">
                      <input type="checkbox" checked={!!l.exento} onChange={(e) => setLinea(l.sku, 'exento', e.target.checked)}
                        className="h-4 w-4 rounded border-slate-300 dark:border-slate-600 text-elerp-500 ring-focus" />
                    </td>
                    <td className="py-2 pr-3 text-right num font-medium private-mask">{fmtCurrency(netoLinea(l), 'VES')}</td>
                    <td className="py-2 pr-3 text-right num tabular-nums">
                      {l.exento
                        ? <span className="text-slate-400 text-[11px] uppercase tracking-wide">Exento</span>
                        : <span className="text-slate-500 private-mask">{fmtCurrency(ivaLinea(l), 'VES')}</span>}
                    </td>
                    <td className="py-2 pr-3 text-right align-top">
                      <button onClick={() => rmLinea(l.sku)} className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800"><Icon.Trash size={15} /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* Notas · Totales */}
      <div className="grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-4">
        <Field label="Notas" hint="opcional · referencia interna, condiciones acordadas…">
          <textarea value={notas} onChange={(e) => setNotas(e.target.value)} rows={3} placeholder="Ej: Entrega en depósito principal, factura a 30 días."
            className="w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm ring-focus resize-y" />
        </Field>
        <div>
          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-4 space-y-1.5 text-[13px] lg:sticky lg:top-4">
            <div className="flex justify-between text-slate-500"><span>Subtotal (neto)</span><span className="num private-mask">{fmtCurrency(totales.subtotal, 'VES')}</span></div>
            {totales.baseExenta > 0 ? <div className="flex justify-between text-slate-500"><span>Exento de IVA</span><span className="num private-mask">{fmtCurrency(totales.baseExenta, 'VES')}</span></div> : null}
            <div className="flex justify-between text-slate-500"><span>IVA {IVA_TASA * 100}% <span className="text-slate-400">(estimado)</span></span><span className="num private-mask">{fmtCurrency(totales.iva, 'VES')}</span></div>
            <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between font-semibold">
              <span>Total <span className="text-[11px] font-normal text-slate-400">con IVA</span></span><span className="num private-mask text-[16px]">{fmtCurrency(totales.total, 'VES')}</span>
            </div>
            <div className="pt-1 text-[11px] text-slate-400 leading-snug">IVA estimado de la compra. La factura fiscal del proveedor se registra aparte.</div>
          </div>
          {touched && err ? (
            <div className="mt-3 flex items-start gap-1.5 text-[12px] text-amber-700 dark:text-amber-400">
              <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{err}</span>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  )
}

// Detalle de una OC: líneas (con recibido), totales, proveedor, sede, estado y
// las acciones disponibles según el estado.
function DetalleOrden({ oc, gestiona, sedes, factura, onVolver, onConfirmar, onRecibir, onCancelar, onFacturar, onPDF }) {
  const em = ESTADO_META[oc.estado] || { label: oc.estado, color: 'slate' }
  const ccy = ccyDe(oc)
  const sedeNombre = sedes.find((s) => s.id === oc.sedeId)?.nombre || oc.sedeId || '—'
  const pendiente = ['confirmada', 'recibida_parcial'].includes(oc.estado)
  const recibida = ['recibida', 'recibida_parcial'].includes(oc.estado)

  const btnPDF = <Button variant="secondary" icon={<Icon.Download size={15} />} onClick={onPDF}>PDF</Button>
  const acciones = (() => {
    if (!gestiona) return btnPDF
    if (oc.estado === 'borrador') return (
      <>
        {btnPDF}
        <Button variant="destructive" onClick={onCancelar} icon={<Icon.CircleX size={16} />}>Cancelar</Button>
        <Button icon={<Icon.Check size={16} />} onClick={onConfirmar}>Confirmar orden</Button>
      </>
    )
    if (pendiente) return (
      <>
        {btnPDF}
        <Button variant="destructive" onClick={onCancelar} icon={<Icon.CircleX size={16} />}>Cancelar</Button>
        {recibida && !factura ? <Button variant="ghost" icon={<Icon.Receipt size={16} />} onClick={onFacturar}>Registrar factura</Button> : null}
        <Button variant="dinero" icon={<Icon.Inbox size={16} />} onClick={onRecibir}>Recibir mercancía</Button>
      </>
    )
    // recibida por completo sin factura → acción de facturar
    if (recibida && !factura) return (
      <>
        {btnPDF}
        <Button icon={<Icon.Receipt size={16} />} onClick={onFacturar}>Registrar factura</Button>
      </>
    )
    return btnPDF
  })()

  return (
    <VistaDetalle onVolver={onVolver} icon={<Icon.ClipboardList size={18} />}
      titulo={oc.numeroCompleto || 'Orden de compra'} sub={`${em.label} · ${fmtDate(oc.creada)}`} acciones={acciones}>
      <div className="grid grid-cols-1 lg:grid-cols-[340px_1fr] gap-4 items-start">
        {/* Columna izquierda: estado, proveedor, sede, factura y notas */}
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3">
            <div className="flex items-center gap-2 flex-wrap">
              <Badge color={em.color} dot>{em.label}</Badge>
              {factura ? <Badge color="emerald" dot>Facturada · {factura.numeroControl || factura.numeroFactura}</Badge> : null}
              {oc.condicionesPago ? <span className="text-[11.5px] text-slate-400">{oc.condicionesPago}</span> : null}
            </div>
            <div>
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Proveedor</div>
              <div className="text-[13.5px] font-medium">{oc.proveedorNombre || '—'}</div>
            </div>
            <div>
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Sede que recibe</div>
              <div className="text-[13.5px] font-medium inline-flex items-center gap-1.5"><Icon.Home size={14} className="text-slate-400" /> {sedeNombre}</div>
            </div>
          </div>

          {factura ? (
            <div className="rounded-xl border border-emerald-200 dark:border-emerald-900/50 bg-emerald-50 dark:bg-emerald-900/20 p-4 space-y-1.5 text-[12.5px]">
              <div className="flex items-center gap-1.5 font-semibold text-emerald-700 dark:text-emerald-300"><Icon.Receipt size={15} /> Factura del proveedor</div>
              <div className="flex items-center justify-between text-emerald-800/90 dark:text-emerald-200/90"><span>Nº de factura</span><span className="num">{factura.numeroFactura || '—'}</span></div>
              <div className="flex items-center justify-between text-emerald-800/90 dark:text-emerald-200/90"><span>Nº de control</span><span className="num">{factura.numeroControl || '—'}</span></div>
              <div className="flex items-center justify-between text-emerald-800/90 dark:text-emerald-200/90"><span>Fecha</span><span className="num">{fmtDate(factura.fecha)}</span></div>
              <div className="flex items-center justify-between text-emerald-800/90 dark:text-emerald-200/90"><span>IVA crédito</span><span className="num private-mask">{fmtCurrency(factura.iva, ccy)}</span></div>
            </div>
          ) : null}

          {oc.notas ? (
            <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Notas</div>
              <div className="text-[12.5px] text-slate-600 dark:text-slate-300 whitespace-pre-wrap">{oc.notas}</div>
            </div>
          ) : null}

          {oc.estado === 'recibida' ? (
            <div className="flex items-center gap-2 text-[12.5px] text-emerald-700 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-900/20 rounded-lg px-3 py-2">
              <Icon.CircleCheck size={15} /> Orden recibida por completo. La mercancía ya está en el inventario.
            </div>
          ) : oc.estado === 'cancelada' ? (
            <div className="flex items-center gap-2 text-[12.5px] text-rose-700 dark:text-rose-400 bg-rose-50 dark:bg-rose-900/20 rounded-lg px-3 py-2">
              <Icon.CircleAlert size={15} /> Orden cancelada.
            </div>
          ) : null}
        </div>

        {/* Columna derecha: líneas y totales */}
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
            <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Líneas ({(oc.lineas || []).length})</div>
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
              {(oc.lineas || []).map((l, i) => {
                const recibido = Number(l.cantidadRecibida) || 0
                const parcial = recibido > 0 && recibido < (Number(l.cantidad) || 0)
                // IVA estimado del renglón (alícuota vigente sobre el neto; 0 si exento).
                const ivaLn = l.exento ? 0 : (Number(l.total) || 0) * IVA_TASA
                return (
                  <div key={l.sku || i} className="flex items-center gap-3 px-3 py-2">
                    <div className="flex-1 min-w-0">
                      <div className="text-[13px] font-medium truncate">{l.nombre || l.sku}{l.exento ? <span className="ml-1.5 text-[11px] text-slate-400">exento</span> : null}</div>
                      <div className="text-[11px] text-slate-400 num">{fmtNum(l.cantidad)} × {fmtCurrency(l.costoUnitario, ccy)}
                        {recibido > 0 ? <span className={parcial ? 'text-amber-600 dark:text-amber-400 ml-1.5' : 'text-emerald-600 dark:text-emerald-400 ml-1.5'}> · recibido {fmtNum(recibido)}</span> : null}
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="num text-[13px] font-medium private-mask">{fmtCurrency(l.total, ccy)}</div>
                      <div className="num text-[11px] text-slate-400">{l.exento ? 'IVA exento' : <>IVA <span className="private-mask">{fmtCurrency(ivaLn, ccy)}</span></>}</div>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>

          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-4 space-y-1.5 text-[13px] max-w-sm ml-auto">
            <div className="flex items-center justify-between"><span className="text-slate-500">Subtotal (neto)</span><span className="num private-mask">{fmtCurrency(oc.subtotal, ccy)}</span></div>
            <div className="flex items-center justify-between"><span className="text-slate-500">IVA <span className="text-slate-400">(estimado)</span></span><span className="num private-mask">{fmtCurrency(oc.iva, ccy)}</span></div>
            <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
              <span className="font-semibold">Total <span className="text-[11px] font-normal text-slate-400">con IVA</span></span><span className="num font-semibold private-mask text-[16px]">{fmtCurrency(oc.total, ccy)}</span>
            </div>
            {oc.estado !== 'cancelada' && !factura ? <div className="text-[11px] text-slate-400 leading-snug pt-0.5">IVA estimado de la compra. La factura fiscal del proveedor se registra aparte.</div> : null}
          </div>
        </div>
      </div>
    </VistaDetalle>
  )
}

// Cancelar una orden de compra. Exige motivo (queda en la traza).
function CancelarModal({ oc, onClose, onSaved, toast }) {
  const [motivo, setMotivo] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)
  const err = !motivo.trim() ? 'El motivo es obligatorio.' : ''

  const confirmar = async () => {
    setTouched(true)
    if (err) return
    setBusy(true)
    try {
      await api.cancelarOrdenCompra(oc.id, motivo.trim())
      toast({ title: 'Orden cancelada', body: oc.numeroCompleto || '' })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo cancelar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.CircleX size={18} />}
      title="Cancelar orden de compra" sub={oc.numeroCompleto}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Volver</Button>
        <Button variant="destructive" onClick={confirmar} loading={busy} icon={<Icon.CircleX size={16} />}>Cancelar orden</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="flex items-start gap-2 text-[12.5px] text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/20 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>La orden no se elimina: queda cancelada, con su motivo, para la traza. Lo ya recibido permanece en el inventario.</span>
        </div>
        <Field label="Motivo de la cancelación" required error={touched ? err : ''}>
          <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!err} placeholder="Ej: El proveedor no tiene stock, precios vencidos…" autoFocus />
        </Field>
      </div>
    </Modal>
  )
}

// Fecha de hoy en formato YYYY-MM-DD, en hora local (no UTC), para el default del
// input date sin que se corra un día cerca de la medianoche.
const hoyISO = () => {
  const d = new Date()
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 10)
}

/* Registrar la factura de compra del proveedor sobre una OC ya recibida. La OC
 * recibida ya reconoció el pasivo; al registrar la factura se reconoce el IVA
 * crédito. Append-only: una OC no se puede facturar dos veces (el backend lo
 * rechaza con 400 y acá se muestra el mensaje). */
// recibidoDeOC construye, por SKU, lo efectivamente recibido de la orden
// (cantidad y costo con que entró al Kardex) para prefijar la factura y medir la
// diferencia. Solo líneas con recepción (>0) generan deuda que facturar.
function recibidoDeOC(oc) {
  const map = new Map()
  for (const l of oc?.lineas || []) {
    const cant = Number(l.cantidadRecibida) || 0
    if (cant <= 0) continue
    map.set(l.sku, { sku: l.sku, nombre: l.nombre || l.sku, cantidad: cant, costoUnitario: Number(l.costoUnitario) || 0, exento: !!l.exento })
  }
  return map
}

function ModalFacturaCompra({ oc, onClose, onSaved, toast }) {
  const ccy = ccyDe(oc)
  const { db } = useData()
  // Un combo no se factura como línea de compra (backend lo rechaza): fuera del selector.
  const productos = (db.PRODUCTOS || []).filter((p) => !p.esCombo)
  const [numeroFactura, setNumeroFactura] = useState('')
  const [numeroControl, setNumeroControl] = useState('')
  const [fecha, setFecha] = useState(hoyISO())
  // IVA del documento: vacío = usar el estimado (base gravada × alícuota); si el
  // proveedor facturó un IVA distinto (redondeo, alícuotas mixtas), se captura tal
  // cual y el servidor lo respeta en vez de recalcularlo.
  const [ivaStr, setIvaStr] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  // Referencia: lo recibido de la OC (base que ya asentó Inventario / CxP).
  const recibido = useMemo(() => recibidoDeOC(oc), [oc])
  const baseRecibida = useMemo(() => {
    let s = 0
    for (const r of recibido.values()) s += r.cantidad * r.costoUnitario
    return s
  }, [recibido])

  // Líneas EDITABLES de la factura del proveedor: prefill = lo recibido. Se pueden
  // ajustar cantidad/costo, marcar exento, quitar o agregar ítems (la OC no obliga).
  const [lineas, setLineas] = useState(() =>
    [...recibido.values()].map((r) => ({ sku: r.sku, nombre: r.nombre, cantidad: String(r.cantidad), costoUnitario: String(r.costoUnitario), exento: r.exento })))
  const [agregar, setAgregar] = useState('') // sku a agregar

  const setLinea = (i, patch) => setLineas((ls) => ls.map((l, j) => (j === i ? { ...l, ...patch } : l)))
  const quitar = (i) => setLineas((ls) => ls.filter((_, j) => j !== i))
  const agregarItem = () => {
    if (!agregar) return
    const p = productos.find((x) => x.sku === agregar)
    if (!p || lineas.some((l) => l.sku === p.sku)) { setAgregar(''); return }
    setLineas((ls) => [...ls, { sku: p.sku, nombre: p.nombre, cantidad: '1', costoUnitario: String(Number(p.costo) || 0), exento: !!p.exentoIva }])
    setAgregar('')
  }

  // Totales de la FACTURA (base gravada/exenta + IVA de vista previa) y diferencia
  // contra lo recibido. El servidor recalcula con la alícuota vigente y es la
  // autoridad; esto es una vista previa para que el usuario vea y confirme.
  const tot = useMemo(() => {
    let gravada = 0, exenta = 0
    for (const l of lineas) {
      const monto = (Number(l.cantidad) || 0) * (Number(l.costoUnitario) || 0)
      if (l.exento) exenta += monto; else gravada += monto
    }
    const iva = gravada * IVA_TASA
    const base = gravada + exenta
    return { gravada, exenta, iva, base, total: base + iva, diferencia: base - baseRecibida }
  }, [lineas, baseRecibida])

  // IVA efectivo: el capturado si el usuario lo ajustó, si no el estimado.
  const ivaCapturado = ivaStr.trim() !== ''
  const ivaEfectivo = ivaCapturado ? Math.max(0, Number(ivaStr) || 0) : tot.iva
  const totalEfectivo = tot.base + ivaEfectivo

  const disponibles = useMemo(
    () => productos.filter((p) => !lineas.some((l) => l.sku === p.sku)),
    [productos, lineas])

  const errFactura = !numeroFactura.trim() ? 'Ingresa el Nº de factura del proveedor.' : ''
  const errControl = !numeroControl.trim() ? 'Ingresa el Nº de control (SENIAT).' : ''
  const errFecha = !fecha ? 'Indica la fecha del documento.' : ''
  const lineasValidas = lineas.filter((l) => (Number(l.cantidad) || 0) > 0)
  const errLineas = lineasValidas.length === 0 ? 'La factura necesita al menos una línea con cantidad.' : ''
  const err = errFactura || errControl || errFecha || errLineas

  const registrar = async () => {
    setTouched(true)
    if (err) return
    setBusy(true)
    try {
      const f = await api.registrarFacturaCompra(oc.id, {
        numeroFactura: numeroFactura.trim(),
        numeroControl: numeroControl.trim(),
        fecha,
        lineas: lineasValidas.map((l) => ({
          sku: l.sku, cantidad: Number(l.cantidad) || 0,
          costoUnitario: Number(l.costoUnitario) || 0, exento: !!l.exento,
        })),
        // Sólo se envía el IVA si el usuario lo capturó; vacío ⇒ el servidor lo computa.
        ...(ivaCapturado ? { iva: ivaEfectivo } : {}),
      })
      const dif = Number(f?.diferenciaBase) || 0
      toast({
        title: 'Factura registrada',
        body: Math.abs(dif) > 0.005
          ? `Control ${f.numeroControl} · diferencia de ${fmtCurrency(dif, ccy)} reconocida.`
          : (f?.numeroControl ? `Control ${f.numeroControl} · IVA crédito reconocido.` : 'IVA crédito reconocido.'),
      })
      await onSaved()
      onClose()
    } catch (e) {
      // 400: duplicada / estado inválido / datos faltantes. Se muestra el mensaje
      // del backend y se deja el modal abierto para corregir.
      toast({ title: 'No se pudo registrar la factura', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  const hayDif = Math.abs(tot.diferencia) > 0.005

  return (
    <Modal open onClose={onClose} size="lg" icon={<Icon.Receipt size={18} />}
      title="Registrar factura del proveedor" sub={oc.numeroCompleto}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={registrar} loading={busy} disabled={touched && !!err} icon={<Icon.Check size={16} />}>Registrar factura</Button>
      </>}>
      <div className="space-y-4">
        <div className="flex items-start gap-2 text-[12px] text-elerp-700 dark:text-elerp-300 bg-elerp-50 dark:bg-elerp-900/20 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>Registra la factura <strong>tal como la emitió el proveedor</strong>: sus líneas y precios pueden diferir de la orden. La orden es prefill; cualquier diferencia contra lo recibido se muestra abajo y se asienta como <strong>diferencia en compras</strong> sin re-valuar el inventario.</span>
        </div>

        {/* Datos del documento */}
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <Field label="Nº de factura del proveedor" required error={touched ? errFactura : ''}>
            <Input value={numeroFactura} onChange={(e) => setNumeroFactura(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!errFactura} placeholder="Ej: 00012345" autoFocus />
          </Field>
          <Field label="Nº de control (SENIAT)" required error={touched ? errControl : ''}>
            <Input value={numeroControl} onChange={(e) => setNumeroControl(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!errControl} placeholder="Ej: 01-00012345" />
          </Field>
          <Field label="Fecha del documento" required error={touched ? errFecha : ''}>
            <Input type="date" value={fecha} max={hoyISO()} onChange={(e) => setFecha(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!errFecha} />
          </Field>
        </div>

        {/* Líneas editables con diferencia por renglón contra lo recibido */}
        <div>
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-[11px] uppercase tracking-wide text-slate-400">Líneas de la factura</span>
            {touched && errLineas ? <span className="text-[11px] text-rose-500">{errLineas}</span> : null}
          </div>
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 overflow-x-auto">
            <table className="w-full text-[12.5px]">
              <thead>
                <tr className="text-left text-[10.5px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-700 bg-slate-50/60 dark:bg-slate-900/40">
                  <th className="py-2 px-2.5 font-medium">Ítem</th>
                  <th className="py-2 px-2 font-medium text-right w-24">Cantidad</th>
                  <th className="py-2 px-2 font-medium text-right w-32">Costo unit.</th>
                  <th className="py-2 px-2 font-medium text-center w-16">Exento</th>
                  <th className="py-2 px-2 font-medium text-right w-28">Total</th>
                  <th className="py-2 px-2 font-medium text-right w-32">vs recibido</th>
                  <th className="py-2 px-1 w-8"></th>
                </tr>
              </thead>
              <tbody>
                {lineas.length === 0 ? (
                  <tr><td colSpan={7} className="py-4 px-2.5 text-center text-slate-400 text-[12px]">Sin líneas. Agrega un ítem del catálogo abajo.</td></tr>
                ) : lineas.map((l, i) => {
                  const ref = recibido.get(l.sku)
                  const total = (Number(l.cantidad) || 0) * (Number(l.costoUnitario) || 0)
                  const totRef = ref ? ref.cantidad * ref.costoUnitario : 0
                  const dif = total - totRef
                  return (
                    <tr key={l.sku} className="border-b border-slate-100 dark:border-slate-800/70">
                      <td className="py-1.5 px-2.5">
                        <div className="font-medium truncate max-w-[190px]">{l.nombre}</div>
                        <div className="text-[10.5px] text-slate-400 num">{l.sku}{ref ? '' : ' · fuera de la orden'}</div>
                      </td>
                      <td className="py-1.5 px-2"><Input className="text-right num h-8" type="number" min="0" step="any" value={l.cantidad} onChange={(e) => setLinea(i, { cantidad: e.target.value })} /></td>
                      <td className="py-1.5 px-2"><Input className="text-right num h-8" type="number" min="0" step="any" value={l.costoUnitario} onChange={(e) => setLinea(i, { costoUnitario: e.target.value })} /></td>
                      <td className="py-1.5 px-2 text-center"><input type="checkbox" checked={l.exento} onChange={(e) => setLinea(i, { exento: e.target.checked })} className="accent-elerp-600 w-4 h-4" /></td>
                      <td className="py-1.5 px-2 text-right num private-mask">{fmtCurrency(total, ccy)}</td>
                      <td className={`py-1.5 px-2 text-right num ${Math.abs(dif) > 0.005 ? 'text-amber-600 dark:text-amber-400 font-medium' : 'text-slate-300 dark:text-slate-600'}`}>
                        {!ref ? <span className="text-amber-600 dark:text-amber-400">nuevo</span> : Math.abs(dif) > 0.005 ? `${dif > 0 ? '+' : ''}${fmtCurrency(dif, ccy)}` : '—'}
                      </td>
                      <td className="py-1.5 px-1 text-center">
                        <button onClick={() => quitar(i)} title="Quitar línea" className="text-slate-300 hover:text-rose-500 ring-focus rounded p-0.5"><Icon.Trash size={14} /></button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          {disponibles.length ? (
            <div className="flex items-center gap-2 mt-2">
              <Select className="flex-1" value={agregar} onChange={(e) => setAgregar(e.target.value)}>
                <option value="">Agregar ítem del catálogo (no estaba en la orden)…</option>
                {disponibles.map((p) => <option key={p.sku} value={p.sku}>{p.sku} · {p.nombre}</option>)}
              </Select>
              <Button variant="secondary" size="sm" icon={<Icon.Plus size={15} />} onClick={agregarItem} disabled={!agregar}>Agregar</Button>
            </div>
          ) : null}
        </div>

        {/* Totales de la factura + diferencia contra lo recibido */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div className={`rounded-lg border p-3 space-y-1.5 text-[12.5px] ${hayDif ? 'border-amber-300 dark:border-amber-700/60 bg-amber-50/60 dark:bg-amber-900/15' : 'border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/60'}`}>
            <div className="flex items-center justify-between"><span className="text-slate-500">Base recibida (OC)</span><span className="num private-mask">{fmtCurrency(baseRecibida, ccy)}</span></div>
            <div className="flex items-center justify-between"><span className="text-slate-500">Base facturada</span><span className="num private-mask">{fmtCurrency(tot.base, ccy)}</span></div>
            <div className="flex items-center justify-between border-t border-slate-200 dark:border-slate-700 pt-1.5">
              <span className={hayDif ? 'text-amber-700 dark:text-amber-300 font-medium' : 'text-slate-500'}>Diferencia</span>
              <span className={`num private-mask ${hayDif ? 'text-amber-700 dark:text-amber-300 font-semibold' : ''}`}>{tot.diferencia > 0 ? '+' : ''}{fmtCurrency(tot.diferencia, ccy)}</span>
            </div>
            {hayDif ? <div className="text-[11px] text-amber-700/80 dark:text-amber-300/80 leading-snug">Se asienta como diferencia en compras (5202). El inventario conserva el costo recibido.</div> : null}
          </div>
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/60 p-3 space-y-1.5 text-[12.5px]">
            <div className="flex items-center justify-between"><span className="text-slate-500">Base gravada</span><span className="num private-mask">{fmtCurrency(tot.gravada, ccy)}</span></div>
            {tot.exenta ? <div className="flex items-center justify-between"><span className="text-slate-500">Base exenta</span><span className="num private-mask">{fmtCurrency(tot.exenta, ccy)}</span></div> : null}
            <div className="flex items-center justify-between gap-2">
              <span className="text-slate-500">IVA crédito {ivaCapturado ? '(del documento)' : `(${Math.round(IVA_TASA * 100)}% estimado)`}</span>
              <div className="flex items-center gap-1.5">
                <Input className="w-28 text-right num h-7 !text-[12.5px]" type="number" min="0" step="any"
                  value={ivaStr} placeholder={tot.iva.toFixed(2)}
                  onChange={(e) => setIvaStr(e.target.value)} title="IVA tal como lo facturó el proveedor (vacío = estimado)" />
                {ivaCapturado ? (
                  <button type="button" onClick={() => setIvaStr('')} title="Volver al IVA estimado"
                    className="text-slate-300 hover:text-slate-500 ring-focus rounded p-0.5"><Icon.X size={13} /></button>
                ) : null}
              </div>
            </div>
            <div className="flex items-center justify-between border-t border-slate-200 dark:border-slate-700 pt-1.5">
              <span className="font-semibold">Total factura</span><span className="num private-mask font-semibold text-[15px]">{fmtCurrency(totalEfectivo, ccy)}</span>
            </div>
          </div>
        </div>
      </div>
    </Modal>
  )
}

// Lista de facturas de compra registradas (solo lectura). Consolida las facturas
// de la empresa; el detalle vive en la OC de origen.
function FacturasCompra({ facturas, loading }) {
  return (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
      {loading ? (
        <div className="p-4"><TableSkeleton rows={6} cols={6} /></div>
      ) : (facturas || []).length === 0 ? (
        <Empty icon={<Icon.Receipt size={22} />} title="Aún no hay facturas de compra"
          body="Registra la factura del proveedor desde una orden de compra recibida: al hacerlo se reconoce el IVA crédito." />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm tbl-sticky">
            <thead>
              <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                <th className="py-2.5 pr-3 font-medium">Nº factura</th>
                <th className="py-2.5 pr-3 font-medium">Nº control</th>
                <th className="py-2.5 pr-3 font-medium">Proveedor</th>
                <th className="py-2.5 pr-3 font-medium">Fecha</th>
                <th className="py-2.5 pr-3 font-medium text-right">Base</th>
                <th className="py-2.5 pr-3 font-medium text-right">IVA</th>
                <th className="py-2.5 pr-3 font-medium text-right">Total</th>
              </tr>
            </thead>
            <tbody>
              {(facturas || []).map((f) => (
                <tr key={f.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover">
                  <td className="py-2.5 pr-3 num text-[12.5px] font-medium">{f.numeroFactura || '—'}</td>
                  <td className="py-2.5 pr-3 num text-[12.5px]">{f.numeroControl || '—'}</td>
                  <td className="py-2.5 pr-3"><div className="text-[13px] truncate max-w-[220px]">{f.proveedorNombre || '—'}</div>
                    {f.proveedorRif ? <div className="text-[11px] text-slate-500 num">{f.proveedorRif}</div> : null}</td>
                  <td className="py-2.5 pr-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(f.fecha)}</td>
                  <td className="py-2.5 pr-3 text-right num private-mask">{fmtCurrency(f.baseImponible, 'VES')}</td>
                  <td className="py-2.5 pr-3 text-right num private-mask">{fmtCurrency(f.iva, 'VES')}</td>
                  <td className="py-2.5 pr-3 text-right num font-medium private-mask">{fmtCurrency(f.total, 'VES')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
