import { useState, useMemo, useRef, useEffect, Fragment } from 'react'
import { vendibles } from '../lib/catalogo.js'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, VistaDetalle, Modal, Empty, useToast, Field } from '../components/primitives.jsx'
import { TablaDatos } from '../components/TablaDatos.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { calcularTotales, IVA_TASA } from '../lib/fiscal.js'
import { useData, useTasa } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { precioListaEnBs, monedaDe, porCodigo } from '../lib/precio.js'
import { explotarCombo } from '../lib/combo.js'
import { NuevoClienteModal, puedeCrearCliente } from './POS.jsx'
import { CobroModal } from './Cobro.jsx'
import { ComprobanteModal, comprobanteDeCotizacion } from '../components/ComprobantePDF.jsx'

/* Ventas (forma libre). El ciclo es de tres etapas, y el vocabulario lo respeta:
 *
 *   Cotización  → propuesta editable en borrador, sin efecto fiscal.
 *   Pedido      → la cotización confirmada (prefactura): ya no se edita, pero
 *                 todavía no es una factura.
 *   Factura     → documento fiscal inmutable, emitido al registrar el pago.
 *
 * Como en el POS, los montos definitivos los calcula el servidor; acá se
 * previsualizan con las mismas reglas (IVA solo sobre la base imponible).
 */
const ESTADO_META = {
  borrador: { label: 'Borrador', color: 'slate' },
  confirmada: { label: 'Pedido', color: 'teal' },
  facturada: { label: 'Facturada', color: 'emerald' },
  cancelada: { label: 'Cancelada', color: 'red' },
}

/* Segmentos-filtro (estilo Odoo): una sola bandeja, filtrada por la etapa del
 * ciclo. `estado` es null en "Todas". El vacío de cada segmento tiene su copy. */
const SEGMENTOS = [
  { value: 'todos', label: 'Todas', estado: null, vacio: 'Aún no hay cotizaciones' },
  { value: 'borrador', label: 'Cotizaciones', estado: 'borrador', vacio: 'No hay cotizaciones en borrador' },
  { value: 'confirmada', label: 'Confirmadas', estado: 'confirmada', vacio: 'No hay pedidos confirmados' },
  { value: 'facturada', label: 'Facturadas', estado: 'facturada', vacio: 'Aún no hay ventas facturadas' },
  { value: 'cancelada', label: 'Anuladas', estado: 'cancelada', vacio: 'No hay cotizaciones anuladas' },
]

// Las tres etapas visibles del ciclo, en orden. La anulación es transversal.
const ETAPAS = [
  { key: 'borrador', label: 'Cotización' },
  { key: 'confirmada', label: 'Confirmada' },
  { key: 'facturada', label: 'Facturada' },
]
const ETAPA_IDX = { borrador: 0, confirmada: 1, facturada: 2 }

const puedeGestionar = (rol) => ['dueno', 'desarrollador', 'vendedor'].includes(rol)
const ccyDe = (cot) => cot?.moneda || 'VES'
// Redondeo a 2 decimales (misma regla que round2 del backend) para no arrastrar
// colas de flotante al bajar precios de línea por un cupón.
const round2Cot = (v) => Math.round((Number(v) || 0) * 100) / 100

/* Barra de estado por fila (statusbar tipo Odoo): mini-stepper compacto
 * «Cotización › Confirmada › Facturada». La etapa actual va resaltada en teal;
 * las ya pasadas, rellenas en muted; las futuras, vacías. Una cotización anulada
 * no tiene stepper: lleva un badge distintivo «Anulada» (rose). */
function EtapaBar({ estado }) {
  if (estado === 'cancelada') return <Badge size="sm" color="rose" dot>Anulada</Badge>
  const idx = ETAPA_IDX[estado] ?? 0
  const paso = 'px-1.5 py-0.5 rounded text-[10.5px] leading-none whitespace-nowrap'
  const cls = (i) => i < idx
    ? `${paso} font-medium bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400`
    : i === idx
      ? `${paso} font-semibold bg-teal-50 text-teal-700 ring-1 ring-teal-200 dark:bg-teal-500/15 dark:text-teal-300 dark:ring-teal-500/30`
      : `${paso} font-medium text-slate-300 dark:text-slate-600 border border-dashed border-slate-200 dark:border-slate-700`
  return (
    <div className="inline-flex items-center gap-1">
      {ETAPAS.map((e, i) => (
        <Fragment key={e.key}>
          {i > 0 ? <Icon.ChevRight size={11} className="text-slate-300 dark:text-slate-600 shrink-0" /> : null}
          <span className={cls(i)}>{e.label}</span>
        </Fragment>
      ))}
    </div>
  )
}

// Segmento inicial según la etapa con que se entra a la bandeja desde el menú:
//   · 'cotizacion' (Ventas › Cotizaciones) → las SIN confirmar (borrador).
//   · 'pedido'     (Ventas › Pedido de ventas) → las confirmadas (pedido/prefactura).
// Sin etapa (uso directo) cae en "Todas". Es solo el segmento de aterrizaje: el
// usuario puede cambiar de segmento libremente.
const filtroDeEtapa = (etapa) => (etapa === 'cotizacion' ? 'borrador' : etapa === 'pedido' ? 'confirmada' : 'todos')

export function Cotizaciones({ navigate, etapaInicial }) {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const cotizaciones = db.COTIZACIONES

  // Índice de facturas por id, para resolver el número de la factura emitida a
  // partir de `cotizacion.documentoId` (cuando el estado es "facturada").
  const docsById = useMemo(() => {
    const m = {}
    for (const d of db.DOCUMENTOS || []) m[d.id] = d
    return m
  }, [db.DOCUMENTOS])

  const [q, setQ] = useState('')
  const [filtro, setFiltro] = useState(() => filtroDeEtapa(etapaInicial))
  const [nueva, setNueva] = useState(false)
  const [editar, setEditar] = useState(null)     // cotización borrador a editar
  const [detalle, setDetalle] = useState(null)
  const [facturar, setFacturar] = useState(null)
  const [cancelar, setCancelar] = useState(null)
  const [pdf, setPdf] = useState(null)            // cotización a previsualizar como PDF
  const [busyId, setBusyId] = useState('')        // id en proceso (confirmar)

  const gestiona = puedeGestionar(ui.rol)

  // Contador por segmento, calculado sobre TODAS las cotizaciones (no sobre el
  // filtro activo): así el chip muestra siempre cuántas hay en cada etapa.
  const conteos = useMemo(() => {
    const c = { todos: 0, borrador: 0, confirmada: 0, facturada: 0, cancelada: 0 }
    for (const x of cotizaciones || []) {
      c.todos += 1
      if (x.estado in c) c[x.estado] += 1
    }
    return c
  }, [cotizaciones])

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return (cotizaciones || []).filter((c) => {
      const okQ = !term
        || (c.numeroCompleto || '').toLowerCase().includes(term)
        || (c.clienteNombre || '').toLowerCase().includes(term)
        || (c.clienteDocumento || '').toLowerCase().includes(term)
      const okF = filtro === 'todos' || c.estado === filtro
      return okQ && okF
    })
  }, [cotizaciones, q, filtro])

  const segActivo = SEGMENTOS.find((s) => s.value === filtro) || SEGMENTOS[0]

  const confirmar = async (cot) => {
    setBusyId(cot.id)
    try {
      await api.confirmarCotizacion(cot.id)
      toast({ title: 'Cotización confirmada', body: `${cot.numeroCompleto} pasó a pedido/prefactura.` })
      await reload()
    } catch (e) {
      toast({ title: 'No se pudo confirmar', body: e?.message || 'Error', kind: 'error' })
    } finally {
      setBusyId('')
    }
  }

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las cotizaciones"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  // Alta / edición: formulario a pantalla completa que reemplaza la lista
  // (estilo Odoo), no un drawer. Se vuelve a la lista con "← Volver".
  if (nueva || editar) {
    return (
      <FormCotizacion cotizacion={editar || null}
        onVolver={() => { setNueva(false); setEditar(null) }}
        onSaved={reload} toast={toast} />
    )
  }

  // Detalle a PANTALLA COMPLETA (estilo Odoo): al abrir una cotización, la vista
  // de detalle reemplaza la lista y ocupa el ancho principal (no un drawer).
  if (detalle) {
    return (
      <div>
        <DetalleCotizacion cot={detalle} gestiona={gestiona} onVolver={() => setDetalle(null)}
          onConfirmar={() => { setDetalle(null); confirmar(detalle) }}
          onFacturar={() => { setFacturar(detalle); setDetalle(null) }}
          onEditar={() => { setEditar(detalle); setDetalle(null) }}
          onCancelar={() => { setCancelar(detalle); setDetalle(null) }}
          onPDF={() => setPdf(detalle)} />
        {facturar ? <FacturarModal cot={facturar} onClose={() => setFacturar(null)} onSaved={reload} toast={toast} /> : null}
        {cancelar ? <CancelarModal cot={cancelar} onClose={() => setCancelar(null)} onSaved={reload} toast={toast} /> : null}
        {pdf ? (
          <ComprobanteModal open onClose={() => setPdf(null)} empresa={db.EMPRESA}
            data={comprobanteDeCotizacion(pdf, db.EMPRESA, (db.CLIENTES || []).find((c) => c.id === pdf.clienteId))} />
        ) : null}
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        {/* Segmentos-filtro por etapa del ciclo, con contador por segmento. */}
        <div className="inline-flex items-center gap-1 p-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/70 overflow-x-auto">
          {SEGMENTOS.map((s) => {
            const activo = filtro === s.value
            const n = conteos[s.value] || 0
            return (
              <button key={s.value} onClick={() => setFiltro(s.value)}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[12.5px] font-semibold whitespace-nowrap transition-colors ${activo ? 'bg-white dark:bg-slate-900 shadow-sm text-slate-900 dark:text-slate-100' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'}`}>
                {s.label}
                <span className={`num text-[10.5px] leading-none px-1.5 py-0.5 rounded-full ${activo ? 'bg-elerp-50 text-elerp-600 dark:bg-elerp-900/50 dark:text-elerp-200' : 'bg-slate-200 text-slate-500 dark:bg-slate-700 dark:text-slate-300'}`}>{fmtNum(n, 0)}</span>
              </button>
            )
          })}
        </div>
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por número, cliente o documento…" value={q} onChange={(e) => setQ(e.target.value)} />
        {gestiona ? <Button className="ml-auto" icon={<Icon.Plus size={16} />} onClick={() => setNueva(true)}>Nueva cotización</Button> : null}
      </div>

      <TablaDatos
        rows={rows}
        loading={loading || cotizaciones === undefined}
        rowKey={(c) => c.id}
        onRowClick={(c) => setDetalle(c)}
        rowClassName={(c) => (c.estado === 'cancelada' ? 'opacity-60' : '')}
        csvName="cotizaciones"
        initialSort={{ key: 'fecha', dir: -1 }}
        footerLabel={(n) => `${fmtNum(n, 0)} cotización(es)`}
        empty={<Empty icon={<Icon.ClipboardList size={22} />}
          title={q ? 'Sin resultados' : segActivo.vacio}
          body={q ? 'Prueba con otro término o segmento.'
            : filtro === 'todos' ? 'Crea una cotización: se la propones al cliente, la confirmas como pedido y la facturas al cobrar.'
            : 'No hay ventas en esta etapa por ahora.'}
          cta={gestiona && !q && filtro === 'todos' ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNueva(true)}>Nueva cotización</Button> : null} />}
        columns={[
          {
            key: 'numero', header: 'Número', sortable: true,
            sortValue: (c) => c.numeroCompleto || '', csv: (c) => c.numeroCompleto || '',
            tdClassName: 'num text-[12.5px] font-medium',
            cell: (c) => {
              const factura = c.estado === 'facturada' && c.documentoId ? docsById[c.documentoId] : null
              const numFactura = factura?.numeroCompleto || c.numeroFactura || c.documentoNumero || ''
              return (
                <>
                  {c.numeroCompleto || '—'}
                  {c.estado === 'facturada' && numFactura ? (
                    navigate ? (
                      <button onClick={(e) => { e.stopPropagation(); navigate('facturacion:factura') }}
                        title="Ver la factura en Facturación → Documentos" aria-label={`Ver la factura ${numFactura} en Facturación`}
                        className="mt-1 inline-flex items-center gap-1 rounded-full bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300 px-2 py-0.5 text-[10.5px] font-semibold hover:bg-emerald-100 dark:hover:bg-emerald-900/50 ring-focus">
                        <Icon.Receipt size={11} /> {numFactura}
                      </button>
                    ) : (
                      <span title="Factura emitida"
                        className="mt-1 inline-flex items-center gap-1 rounded-full bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300 px-2 py-0.5 text-[10.5px] font-semibold">
                        <Icon.Receipt size={11} /> {numFactura}
                      </span>
                    )
                  ) : null}
                </>
              )
            },
          },
          {
            key: 'cliente', header: 'Cliente', sortable: true,
            sortValue: (c) => c.clienteNombre || 'Consumidor final',
            csv: (c) => `${c.clienteNombre || 'Consumidor final'}${c.clienteDocumento ? ' · ' + c.clienteDocumento : ''}`,
            cell: (c) => (
              <>
                <div className="text-[13px] truncate max-w-[200px]">{c.clienteNombre || 'Consumidor final'}</div>
                {c.clienteDocumento ? <div className="text-[11px] text-slate-500 num">{c.clienteDocumento}</div> : null}
              </>
            ),
          },
          {
            key: 'etapa', header: 'Etapa',
            csv: (c) => (ESTADO_META[c.estado]?.label || c.estado || ''),
            cell: (c) => <EtapaBar estado={c.estado} />,
          },
          {
            key: 'total', header: 'Total', align: 'right', sortable: true,
            sortValue: (c) => Number(c.total) || 0, csv: (c) => Number(c.total) || 0,
            tdClassName: 'num font-medium private-mask', cell: (c) => fmtCurrency(c.total, ccyDe(c)),
          },
          {
            key: 'fecha', header: 'Fecha', sortable: true,
            sortValue: (c) => c.fecha || '', csv: (c) => fmtDate(c.fecha),
            tdClassName: 'whitespace-nowrap text-[12.5px] text-slate-500 num',
            cell: (c) => fmtDate(c.fecha),
          },
          {
            key: 'acciones', header: 'Acciones', align: 'right', stop: true, csv: false,
            tdClassName: 'whitespace-nowrap',
            cell: (c) => (
              <div className="inline-flex items-center gap-1">
                <button onClick={() => setPdf(c)} title="Previsualizar / PDF" aria-label={`Previsualizar o descargar el PDF de ${c.numeroCompleto || 'la cotización'}`}
                  className="h-7 px-2 inline-flex items-center gap-1 rounded-lg text-[12px] text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus"><Icon.Download size={14} /> PDF</button>
                <Acciones cot={c} gestiona={gestiona} busy={busyId === c.id}
                  onEditar={() => setEditar(c)} onConfirmar={() => confirmar(c)}
                  onFacturar={() => setFacturar(c)} onCancelar={() => setCancelar(c)}
                  onVer={() => setDetalle(c)} />
              </div>
            ),
          },
        ]} />

      {facturar ? <FacturarModal cot={facturar} onClose={() => setFacturar(null)} onSaved={reload} toast={toast} /> : null}
      {cancelar ? <CancelarModal cot={cancelar} onClose={() => setCancelar(null)} onSaved={reload} toast={toast} /> : null}
      {pdf ? (
        <ComprobanteModal open onClose={() => setPdf(null)} empresa={db.EMPRESA}
          data={comprobanteDeCotizacion(pdf, db.EMPRESA, (db.CLIENTES || []).find((c) => c.id === pdf.clienteId))} />
      ) : null}
    </div>
  )
}

// Botonera por fila, según el estado y el permiso del rol.
function Acciones({ cot, gestiona, busy, onEditar, onConfirmar, onFacturar, onCancelar, onVer }) {
  const btn = 'h-7 px-2 inline-flex items-center gap-1 rounded-lg text-[12px]'
  if (cot.estado === 'borrador' && gestiona) {
    return (
      <div className="inline-flex items-center gap-1">
        <button onClick={onEditar} title="Editar" className={`${btn} text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus`}><Icon.Pencil size={14} /> Editar</button>
        <button onClick={onConfirmar} disabled={busy} title="Confirmar como pedido/prefactura" className={`${btn} text-elerp-600 hover:bg-elerp-50 dark:hover:bg-elerp-900/30 disabled:opacity-50 ring-focus`}>
          {busy ? <span className="spin inline-flex"><Icon.Refresh size={13} /></span> : <Icon.Check size={14} />} Confirmar
        </button>
        <button onClick={onCancelar} title="Cancelar" aria-label="Cancelar la cotización" className={`${btn} text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 ring-focus`}><Icon.CircleX size={14} /></button>
      </div>
    )
  }
  if (cot.estado === 'confirmada' && gestiona) {
    return (
      <div className="inline-flex items-center gap-1">
        <button onClick={onFacturar} title="Registrar pago y facturar" className={`${btn} text-teal-700 dark:text-teal-300 hover:bg-teal-50 dark:hover:bg-teal-900/20 font-semibold ring-focus`}><Icon.Banknote size={14} /> Facturar</button>
        <button onClick={onCancelar} title="Cancelar" aria-label="Cancelar el pedido" className={`${btn} text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 ring-focus`}><Icon.CircleX size={14} /></button>
      </div>
    )
  }
  // facturada / cancelada (o rol de solo lectura): solo Ver.
  return <button onClick={onVer} title="Ver" className={`${btn} text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus`}><Icon.Eye size={14} /> Ver</button>
}

const COND_PAGO = ['Contado', '15 días', '30 días']

/* Alta / edición de cotización — formulario a PANTALLA COMPLETA (estilo Odoo):
 * reemplaza la lista y aprovecha todo el ancho, con "← Volver" para regresar.
 * Solo el borrador se edita; una cotización confirmada o facturada nunca llega
 * acá. El neto por línea aplica el descuento: cant × precio × (1 − desc/100). */
function FormCotizacion({ cotizacion, onVolver, onSaved, toast }) {
  const { db, reload, tasaDe } = useData()
  const { ui } = useUI()
  const tasa = useTasa()
  const editando = !!cotizacion
  const productos = vendibles(db.PRODUCTOS)
  const clientes = db.CLIENTES || []
  const monedaEmpresa = db.EMPRESA?.monedaPrincipal || 'VES'

  // Almacenes/sedes de despacho: las ACTIVAS de la empresa, más la ya guardada en
  // la cotización aunque se haya desactivado (para no perderla al editar).
  const sedes = db.SEDES || []
  const sedesDespacho = useMemo(() => {
    const activas = sedes.filter((s) => s.activa)
    if (cotizacion?.sedeDespacho && !activas.some((s) => s.id === cotizacion.sedeDespacho)) {
      const guardada = sedes.find((s) => s.id === cotizacion.sedeDespacho)
      if (guardada) return [guardada, ...activas]
    }
    return activas
  }, [sedes, cotizacion])

  const [clienteId, setClienteId] = useState(cotizacion?.clienteId || '')
  const [clienteQ, setClienteQ] = useState('')
  const [nuevoCliente, setNuevoCliente] = useState(false)
  const [fecha, setFecha] = useState(cotizacion?.fecha ? String(cotizacion.fecha).slice(0, 10) : new Date().toISOString().slice(0, 10))
  const [validez, setValidez] = useState(cotizacion?.validez ? String(cotizacion.validez).slice(0, 10) : '')
  const [condicionesPago, setCondicionesPago] = useState(cotizacion?.condicionesPago || 'Contado')
  const [terminos, setTerminos] = useState(cotizacion?.terminos || '')
  const [notas, setNotas] = useState(cotizacion?.notas || '')
  // Lista de precio: 'base' (precio del catálogo) o el id de una lista de venta
  // activa. Al elegir una lista, las líneas usan el precio de la lista para los
  // SKU que estén en ella; los demás, su precio base (ver aplicarLista).
  const listasVenta = useMemo(
    () => (db.LISTAS_PRECIO || []).filter((l) => l.tipo === 'venta' && l.activa),
    [db.LISTAS_PRECIO],
  )
  const [listaPrecio, setListaPrecio] = useState(() => {
    const guardada = cotizacion?.listaPrecio || 'base'
    // Si la lista guardada ya no existe/está inactiva, cae a base (retrocompat).
    if (guardada !== 'base' && !(db.LISTAS_PRECIO || []).some((l) => l.id === guardada && l.tipo === 'venta')) return 'base'
    return guardada
  })
  const listaSel = listasVenta.find((l) => l.id === listaPrecio) || null
  // Precio en Bs de un producto bajo la lista elegida (o precio base si 'base' o
  // si el SKU no está en la lista). null cuando falta la tasa para convertir.
  const precioBsDe = (p) => precioListaEnBs(p, listaSel, monedaEmpresa, tasaDe)
  // Dirección de entrega: por defecto la del cliente (editable); ver el efecto
  // de autocompletado más abajo.
  const [direccionEntrega, setDireccionEntrega] = useState(cotizacion?.direccionEntrega || '')
  // Almacén / sede de despacho: de qué depósito sale la mercancía. Determina de
  // qué sede se descuenta el inventario al facturar (backend: SedeDespacho).
  const [sedeDespacho, setSedeDespacho] = useState(
    cotizacion?.sedeDespacho || db.SEDE_ACTIVA?.id || sedesDespacho[0]?.id || '',
  )
  // Líneas: { sku, nombre, descripcion, cantidad, precioUnitario (Bs), descuento %, exento }
  const [lineas, setLineas] = useState(() => (cotizacion?.lineas || []).map((l) => ({
    sku: l.sku, nombre: l.nombre, descripcion: l.descripcion || '',
    cantidad: Number(l.cantidad) || 1, precioUnitario: Number(l.precioUnitario) || 0,
    descuento: Number(l.descuento) || 0, exento: !!l.exento,
  })))
  const [q, setQ] = useState('')
  const [sel, setSel] = useState(0)
  const [busy, setBusy] = useState(false)
  const searchRef = useRef(null)

  // Cupón de descuento. Al aplicarlo se baja el precioUnitario de las líneas (así
  // el IVA lo recalcula el servidor sobre la base ya descontada, sin tocar el
  // motor de totales). `cupon` guarda el código, el descuento en Bs y los precios
  // PREVIOS por SKU, para poder quitarlo y restaurar. El descuento ya queda
  // «horneado» en los precioUnitario que se guardan/facturan.
  const [cuponCodigo, setCuponCodigo] = useState('')
  const [cupon, setCupon] = useState(null)
  const [cuponBusy, setCuponBusy] = useState(false)
  const [cuponErr, setCuponErr] = useState('')

  const clienteSel = clientes.find((c) => c.id === clienteId) || null

  // Autocompleta la dirección de entrega con la del cliente al seleccionarlo, sin
  // pisar lo que el usuario ya haya escrito (o la dirección guardada al editar).
  useEffect(() => {
    if (clienteSel?.direccion && !direccionEntrega.trim()) setDireccionEntrega(clienteSel.direccion)
  }, [clienteId]) // eslint-disable-line react-hooks/exhaustive-deps

  const clientesFiltrados = useMemo(() => {
    const term = clienteQ.trim().toLowerCase()
    if (!term) return []
    return clientes
      .filter((c) => c.nombre.toLowerCase().includes(term) || (c.documento || '').toLowerCase().includes(term))
      .slice(0, 6)
  }, [clienteQ, clientes])

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
    // COMBO: no entra como línea de combo (el servidor la rechaza); se EXPLOTA en
    // una línea por componente con el precio del paquete prorrateado (IVA por
    // ítem). Las líneas explotadas entran como líneas normales (con su descuento).
    if (p.esCombo) {
      const explotadas = explotarCombo(p, (sku) => productos.find((x) => x.sku === sku), 1, precioBsDe)
      if (!explotadas) {
        toast({ title: 'No se pudo agregar el combo', body: `Falta la tasa o algún componente de ${p.nombre}.`, kind: 'warn' })
        return
      }
      setLineas((c) => {
        let next = c
        for (const nl of explotadas) {
          const i = next.findIndex((l) => l.sku === nl.sku)
          if (i >= 0) next = next.map((l, j) => (j === i ? { ...l, cantidad: l.cantidad + nl.cantidad } : l))
          else next = [...next, { sku: nl.sku, nombre: nl.nombre, descripcion: nl.nombre, cantidad: nl.cantidad, precioUnitario: nl.precioUnitario, descuento: 0, exento: nl.exento }]
        }
        return next
      })
      setQ('')
      searchRef.current?.focus()
      return
    }
    // El precio toma la lista seleccionada (o el precio base si es 'base' o si el
    // SKU no está en la lista). La lista puede estar en otra moneda (p. ej. US$):
    // precioListaEnBs la convierte a Bs con la tasa del día.
    const bs = precioBsDe(p)
    if (bs === null) {
      const m = listaSel ? listaSel.moneda : monedaDe(p, monedaEmpresa)
      toast({ title: 'Falta la tasa de cambio', body: `${p.nombre} se cotiza en ${m} y no hay tasa cargada para convertirlo a Bs.`, kind: 'warn' })
      return
    }
    setLineas((c) => {
      const i = c.findIndex((l) => l.sku === p.sku)
      if (i >= 0) return c.map((l, j) => (j === i ? { ...l, cantidad: l.cantidad + 1 } : l))
      return [...c, { sku: p.sku, nombre: p.nombre, descripcion: p.nombre, cantidad: 1, precioUnitario: bs, descuento: 0, exento: !!p.exentoIva }]
    })
    setQ('')
    searchRef.current?.focus()
  }

  // Cambiar de lista de precio: reaplica el precio de la nueva lista a TODAS las
  // líneas ya cargadas (los SKU no listados vuelven a su precio base). Si falta la
  // tasa para convertir un precio, esa línea conserva el que tenía.
  const cambiarLista = (id) => {
    setListaPrecio(id)
    // Cambiar de lista reescribe los precioUnitario: un cupón aplicado quedaría
    // sobre precios que ya no existen. Se quita (el usuario lo reaplica si quiere).
    if (cupon) { setCupon(null); setCuponErr('') }
    const nueva = listasVenta.find((l) => l.id === id) || null
    setLineas((c) => c.map((l) => {
      const p = productos.find((pr) => pr.sku === l.sku)
      if (!p) return l
      const bs = precioListaEnBs(p, nueva, monedaEmpresa, tasaDe)
      return bs === null ? l : { ...l, precioUnitario: bs }
    }))
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

  // Neto por línea con el descuento aplicado (lo que factura y lo que suma a totales).
  const netoLinea = (l) => (Number(l.cantidad) || 0) * (Number(l.precioUnitario) || 0) * (1 - (Number(l.descuento) || 0) / 100)
  // IVA por línea: tasa × neto para las gravadas; null (se muestra «Exento») para
  // las exentas. Misma regla que el backend (IVA solo sobre la base imponible).
  const ivaLinea = (l) => (l.exento ? null : IVA_TASA * netoLinea(l))

  // Totales con las mismas reglas del servidor: el descuento entra bajando el
  // precio unitario neto; el IVA sale solo de la base imponible (no exenta).
  const totales = useMemo(() => calcularTotales(
    lineas.map((l) => ({
      cantidad: Number(l.cantidad) || 0,
      precioUnitario: (Number(l.precioUnitario) || 0) * (1 - (Number(l.descuento) || 0) / 100),
      exento: l.exento,
    })), [], tasa.valor,
  ), [lineas, tasa.valor])

  // Aplicar un cupón: valida el código contra el subtotal (servidor, fuente de
  // verdad) y baja cada precioUnitario por un factor uniforme. Para un cupón
  // porcentual el factor es (1 − %); para uno de monto fijo, (1 − descuento/subtotal),
  // que equivale a repartir el monto proporcional al importe de cada línea. Solo se
  // puede aplicar uno a la vez: para cambiarlo, primero se quita.
  const aplicarCupon = async () => {
    const cod = cuponCodigo.trim()
    if (!cod || cupon) return
    if (lineas.length === 0) { setCuponErr('Agrega productos antes de aplicar un cupón.'); return }
    setCuponBusy(true); setCuponErr('')
    const subtotal = round2Cot(lineas.reduce((s, l) => s + (Number(l.cantidad) || 0) * (Number(l.precioUnitario) || 0), 0))
    try {
      const res = await api.validarCupon({ codigo: cod, subtotal })
      const desc = Math.min(Number(res?.descuentoBs) || 0, subtotal)
      const factor = subtotal > 0 ? 1 - desc / subtotal : 1
      const previos = {}
      for (const l of lineas) previos[l.sku] = l.precioUnitario
      setLineas((c) => c.map((l) => ({ ...l, precioUnitario: round2Cot((Number(l.precioUnitario) || 0) * factor) })))
      setCupon({ codigo: res?.cupon?.codigo || cod.toUpperCase(), tipo: res?.cupon?.tipo, descuentoBs: round2Cot(desc), previos })
      setCuponCodigo('')
    } catch (e) {
      setCuponErr(e?.message || 'Cupón inválido')
    }
    setCuponBusy(false)
  }

  // Quitar el cupón: restaura los precioUnitario previos de las líneas que sigan
  // presentes (las agregadas después conservan su precio).
  const quitarCupon = () => {
    if (!cupon) return
    setLineas((c) => c.map((l) => (cupon.previos[l.sku] != null ? { ...l, precioUnitario: cupon.previos[l.sku] } : l)))
    setCupon(null)
    setCuponErr('')
  }

  const errLineas = lineas.length === 0 ? 'Agrega al menos un producto.'
    : lineas.some((l) => !(Number(l.cantidad) > 0)) ? 'Todas las cantidades deben ser mayores a 0.'
    : lineas.some((l) => Number(l.descuento) < 0 || Number(l.descuento) > 100) ? 'El descuento debe estar entre 0 y 100.'
    : ''

  const buildBody = () => ({
    clienteId: clienteId || '',
    condicionesPago: condicionesPago || '',
    terminos: terminos.trim() || undefined,
    validez: validez || undefined,
    notas: notas.trim() || undefined,
    listaPrecio: listaPrecio || '',
    direccionEntrega: direccionEntrega.trim() || undefined,
    sedeDespacho: sedeDespacho || '',
    // El cupón (si se aplicó) baja el precio de las líneas y se guarda en la
    // cotización para consumir su uso al facturarla (tope UsosMax).
    cuponCodigo: cupon?.codigo || undefined,
    lineas: lineas.map((l) => ({
      sku: l.sku,
      cantidad: Number(l.cantidad),
      precioUnitario: Number(l.precioUnitario),
      descripcion: (l.descripcion || '').trim() || undefined,
      descuento: Number(l.descuento) || 0,
    })),
  })

  const guardar = async () => {
    if (errLineas) return
    setBusy(true)
    try {
      if (editando) {
        await api.actualizarCotizacion(cotizacion.id, buildBody())
        toast({ title: 'Cotización actualizada', body: cotizacion.numeroCompleto || '' })
      } else {
        const c = await api.crearCotizacion(buildBody())
        toast({ title: 'Cotización creada', body: c?.numeroCompleto ? `${c.numeroCompleto} en borrador.` : 'Guardada en borrador.' })
      }
      await onSaved()
      onVolver()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  // Confirmar (solo borrador existente): guarda los cambios y pasa a pedido.
  const confirmar = async () => {
    if (errLineas || !editando) return
    setBusy(true)
    try {
      await api.actualizarCotizacion(cotizacion.id, buildBody())
      await api.confirmarCotizacion(cotizacion.id)
      toast({ title: 'Cotización confirmada', body: `${cotizacion.numeroCompleto || ''} pasó a pedido/prefactura.` })
      await onSaved()
      onVolver()
    } catch (e) {
      toast({ title: 'No se pudo confirmar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  const dateInput = 'w-full h-9 px-3 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm ring-focus'

  return (
    <div>
      {/* Encabezado + acciones */}
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <button onClick={onVolver}
          className="h-9 px-2.5 inline-flex items-center gap-1.5 rounded-lg text-[13px] font-medium text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus">
          <Icon.ChevLeft size={16} /> Volver
        </button>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="font-display text-[20px] font-bold tracking-tight truncate">
              {editando ? (cotizacion.numeroCompleto || 'Cotización') : 'Nueva cotización'}
            </h2>
            {editando ? <Badge size="sm" color="slate" dot>Borrador</Badge> : null}
          </div>
          <div className="text-[12px] text-slate-500">
            {editando ? 'Borrador · en edición' : 'Propuesta al cliente; no tiene efecto fiscal hasta facturarse.'}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" onClick={onVolver}>Cancelar</Button>
          {editando ? (
            <Button variant="secondary" onClick={confirmar} loading={busy} disabled={!!errLineas}
              title={errLineas || 'Guardar y confirmar como pedido'} icon={<Icon.Check size={16} />}>Confirmar</Button>
          ) : null}
          <Button onClick={guardar} loading={busy} disabled={!!errLineas}
            title={errLineas || 'Guardar en borrador'} icon={<Icon.Check size={16} />}>
            {editando ? 'Guardar cambios' : 'Guardar cotización'}
          </Button>
        </div>
      </div>

      {/* Cabecera en columnas */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 mb-4">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          <div className="lg:col-span-2">
            <div className="flex items-center justify-between mb-1">
              <label className="text-[12px] font-medium text-slate-500">Cliente</label>
              {puedeCrearCliente(ui.rol) ? (
                <button onClick={() => setNuevoCliente(true)} className="text-[12px] font-medium text-elerp-600 hover:text-elerp-700 inline-flex items-center gap-1"><Icon.Plus size={13} /> Alta rápida</button>
              ) : null}
            </div>
            {clienteSel ? (
              <div className="flex items-center gap-2 h-9 px-3 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900">
                <Icon.Users size={15} className="text-slate-400 shrink-0" />
                <span className="flex-1 truncate text-[13px]">{clienteSel.nombre}{clienteSel.documento ? ` · ${clienteSel.tipoDocumento}-${clienteSel.documento}` : ''}</span>
                <button onClick={() => { setClienteId(''); setClienteQ('') }} title="Quitar (Consumidor final)"
                  className="h-6 w-6 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800"><Icon.X size={14} /></button>
              </div>
            ) : (
              <div className="relative">
                <Input icon={<Icon.Search size={15} />} value={clienteQ} onChange={(e) => setClienteQ(e.target.value)}
                  placeholder="Buscar cliente… (vacío = Consumidor final)" />
                {clienteQ && clientesFiltrados.length ? (
                  <div className="absolute z-20 mt-1 w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-modal overflow-hidden">
                    {clientesFiltrados.map((c) => (
                      <button key={c.id} onClick={() => { setClienteId(c.id); setClienteQ('') }}
                        className="w-full text-left px-3 py-2 hover:bg-elerp-50 dark:hover:bg-elerp-900/40">
                        <div className="text-[13px] font-medium truncate">{c.nombre}</div>
                        {c.documento ? <div className="text-[11px] text-slate-400 num">{c.tipoDocumento}-{c.documento}</div> : null}
                      </button>
                    ))}
                  </div>
                ) : clienteQ ? (
                  <div className="absolute z-20 mt-1 w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-modal px-4 py-3 text-center text-[12.5px] text-slate-500">
                    Sin resultados. Usa “Alta rápida” para crearlo.
                  </div>
                ) : null}
              </div>
            )}
          </div>

          <Field label="Fecha">
            <input type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} className={dateInput} />
          </Field>
          <Field label="Válida hasta" hint="opcional">
            <input type="date" value={validez} onChange={(e) => setValidez(e.target.value)} className={dateInput} />
          </Field>
          <div className="lg:col-span-2">
            <Field label="Condiciones de pago">
              <Select value={condicionesPago} onChange={(e) => setCondicionesPago(e.target.value)}>
                {COND_PAGO.map((c) => <option key={c} value={c}>{c}</option>)}
              </Select>
            </Field>
          </div>

          <div className="lg:col-span-2">
            <Field label="Lista de precio" hint={listasVenta.length ? 'aplica precios propios por producto' : 'precio base del catálogo'}>
              <Select value={listaPrecio} onChange={(e) => cambiarLista(e.target.value)}>
                <option value="base">Precio base</option>
                {listasVenta.map((l) => (
                  <option key={l.id} value={l.id}>{l.nombre} ({l.moneda})</option>
                ))}
              </Select>
            </Field>
          </div>
          <div className="lg:col-span-2">
            <Field label="Almacén / sede de despacho" hint="de aquí sale la mercancía al facturar">
              {sedesDespacho.length ? (
                <Select value={sedeDespacho} onChange={(e) => setSedeDespacho(e.target.value)}>
                  {sedesDespacho.map((s) => <option key={s.id} value={s.id}>{s.nombre}{s.activa ? '' : ' (inactiva)'}</option>)}
                </Select>
              ) : (
                <div className="h-9 px-3 inline-flex items-center rounded-lg border border-slate-200 dark:border-slate-700 text-[12.5px] text-slate-400">Sin sedes disponibles</div>
              )}
            </Field>
          </div>
          <div className="lg:col-span-4">
            <Field label="Dirección de entrega" hint="opcional · por defecto la del cliente">
              <input type="text" value={direccionEntrega} onChange={(e) => setDireccionEntrega(e.target.value)}
                placeholder="Dirección de despacho de la mercancía…" className={dateInput} />
            </Field>
          </div>
        </div>
      </div>

      {/* Buscador de productos + tabla de líneas */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 mb-4">
        <label className="block text-[12px] font-medium text-slate-500 mb-1">Agregar productos</label>
        <div className="relative mb-3">
          <Input ref={searchRef} icon={<Icon.Search size={15} />} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onSearchKey}
            placeholder="Buscar por nombre, SKU o código… (Enter para agregar)" />
          {q && resultados.length ? (
            <div className="absolute z-20 mt-1 w-full max-w-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-modal overflow-hidden">
              {resultados.map((p, i) => (
                <button key={p.id} onClick={() => addProducto(p)} onMouseEnter={() => setSel(i)}
                  className={`w-full flex items-center gap-3 px-3 h-11 text-left ${i === sel ? 'bg-elerp-50 dark:bg-elerp-900/40' : ''}`}>
                  <Icon.Package size={16} className="text-slate-400" />
                  <div className="flex-1 min-w-0">
                    <div className="text-[13px] font-medium truncate flex items-center gap-1.5">
                      <span className="truncate">{p.nombre}</span>
                      {p.esCombo ? <Badge size="sm" color="huberp">Combo</Badge> : null}
                    </div>
                    <div className="text-[11px] text-slate-400 num">{p.sku}{p.esCombo ? ` · ${(p.componentes || []).length} ítems` : (p.exentoIva ? ' · exento' : '')}</div>
                  </div>
                  <div className="num text-[12.5px] font-medium">{fmtCurrency(p.precio, monedaDe(p, monedaEmpresa))}</div>
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
            <table className="w-full text-sm min-w-[820px]">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2 px-3 font-medium">Producto</th>
                  <th className="py-2 pr-3 font-medium">Descripción</th>
                  <th className="py-2 pr-3 font-medium text-center w-20">Cantidad</th>
                  <th className="py-2 pr-3 font-medium text-right w-28">Precio unit.</th>
                  <th className="py-2 pr-3 font-medium text-center w-20">Desc. %</th>
                  <th className="py-2 pr-3 font-medium text-right w-28">Impuesto</th>
                  <th className="py-2 pr-3 font-medium text-right w-32">Subtotal</th>
                  <th className="py-2 pr-3 w-9"></th>
                </tr>
              </thead>
              <tbody>
                {lineas.map((l) => (
                  <tr key={l.sku} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2 px-3 align-top">
                      <div className="font-medium text-[13px]">{l.nombre}</div>
                      <div className="text-[11px] text-slate-400 num">{l.sku}{l.exento ? <span className="ml-1.5 text-slate-500">exento de IVA</span> : null}</div>
                    </td>
                    <td className="py-2 pr-3">
                      <input type="text" value={l.descripcion} onChange={(e) => setLinea(l.sku, 'descripcion', e.target.value)}
                        placeholder="Descripción…"
                        className="w-full min-w-[140px] h-8 px-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-[12.5px] ring-focus" />
                    </td>
                    <td className="py-2 pr-3">
                      <input type="number" min="0" step="1" value={l.cantidad} onChange={(e) => setLinea(l.sku, 'cantidad', Number(e.target.value))}
                        className="w-16 h-8 text-center rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num ring-focus mx-auto block" />
                    </td>
                    <td className="py-2 pr-3 text-right">
                      <input type="number" min="0" step="0.01" value={l.precioUnitario} onChange={(e) => setLinea(l.sku, 'precioUnitario', Number(e.target.value))}
                        className="w-24 h-8 text-right px-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num ring-focus private-mask" />
                    </td>
                    <td className="py-2 pr-3">
                      <input type="number" min="0" max="100" step="1" value={l.descuento} onChange={(e) => setLinea(l.sku, 'descuento', Number(e.target.value))}
                        className="w-16 h-8 text-center rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num ring-focus mx-auto block" />
                    </td>
                    <td className="py-2 pr-3 text-right num text-[12.5px]">
                      {l.exento
                        ? <span className="text-slate-400">Exento</span>
                        : <span className="private-mask">{fmtCurrency(ivaLinea(l), 'VES')}</span>}
                    </td>
                    <td className="py-2 pr-3 text-right num font-medium private-mask">{fmtCurrency(netoLinea(l), 'VES')}</td>
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

      {/* Términos y notas · Totales */}
      <div className="grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-4">
        <div className="space-y-4">
          <Field label="Términos y condiciones" hint="opcional">
            <textarea value={terminos} onChange={(e) => setTerminos(e.target.value)} rows={3} placeholder="Ej: Validez de la oferta, garantía, tiempos de entrega…"
              className="w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm ring-focus resize-y" />
          </Field>
          <Field label="Notas" hint="opcional · referencias internas o del cliente">
            <textarea value={notas} onChange={(e) => setNotas(e.target.value)} rows={3} placeholder="Ej: Precios sujetos a la tasa del día de la factura."
              className="w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm ring-focus resize-y" />
          </Field>
        </div>

        <div className="lg:sticky lg:top-4 space-y-3">
          {/* Cupón de descuento: baja los precios de línea (el IVA lo recalcula el
              servidor sobre la base ya descontada). */}
          <div className="rounded-xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 p-3">
            <label className="block text-[12px] font-medium text-slate-500 mb-1.5 flex items-center gap-1.5">
              <Icon.Star size={13} className="text-teal-500" /> Cupón de descuento
            </label>
            {cupon ? (
              <div className="flex items-center gap-2 rounded-lg bg-teal-50 dark:bg-teal-500/10 border border-teal-200 dark:border-teal-800 px-3 py-2">
                <div className="flex-1 min-w-0">
                  <div className="text-[12.5px] font-medium num tracking-wide text-teal-800 dark:text-teal-300">{cupon.codigo}</div>
                  <div className="text-[11px] text-teal-700/80 dark:text-teal-400/80 private-mask">−{fmtCurrency(cupon.descuentoBs, 'VES')} aplicado a las líneas</div>
                </div>
                <button type="button" onClick={quitarCupon} title="Quitar el cupón"
                  className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-teal-700 dark:text-teal-300 hover:bg-teal-100 dark:hover:bg-teal-500/20"><Icon.X size={14} /></button>
              </div>
            ) : (
              <>
                <div className="flex items-center gap-2">
                  <Input className="flex-1 num tracking-wide uppercase" placeholder="Código" value={cuponCodigo}
                    onChange={(e) => { setCuponCodigo(e.target.value); setCuponErr('') }}
                    onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); aplicarCupon() } }} />
                  <Button variant="secondary" size="sm" onClick={aplicarCupon} loading={cuponBusy} disabled={!cuponCodigo.trim() || lineas.length === 0}>Aplicar</Button>
                </div>
                {cuponErr ? <div className="mt-1.5 text-[11.5px] text-red-600 dark:text-red-400 flex items-start gap-1"><Icon.CircleAlert size={13} className="mt-0.5 shrink-0" /><span>{cuponErr}</span></div> : null}
              </>
            )}
          </div>

          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-4 space-y-1.5 text-[13px]">
            <div className="flex justify-between text-slate-500"><span>Subtotal</span><span className="num private-mask">{fmtCurrency(totales.subtotal, 'VES')}</span></div>
            {totales.baseExenta > 0 ? <div className="flex justify-between text-slate-500"><span>Exento de IVA</span><span className="num private-mask">{fmtCurrency(totales.baseExenta, 'VES')}</span></div> : null}
            <div className="flex justify-between text-slate-500"><span>IVA {IVA_TASA * 100}%</span><span className="num private-mask">{fmtCurrency(totales.iva, 'VES')}</span></div>
            <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between font-semibold">
              <span>Total</span><span className="num private-mask text-[16px]">{fmtCurrency(totales.total, 'VES')}</span>
            </div>
            <div className="text-[11px] text-slate-400 pt-0.5">El IGTF (3%) no aplica a la cotización: se calcula al facturar, sobre lo que se pague en divisas.</div>
          </div>
          {errLineas ? (
            <div className="mt-3 flex items-start gap-1.5 text-[12px] text-amber-700 dark:text-amber-400">
              <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{errLineas}</span>
            </div>
          ) : null}
        </div>
      </div>

      {nuevoCliente ? <NuevoClienteModal onClose={() => setNuevoCliente(false)} onSaved={async (c) => { await reload(); if (c?.id) setClienteId(c.id) }} toast={toast} /> : null}
    </div>
  )
}

// Detalle de una cotización a PANTALLA COMPLETA (líneas, totales, cliente, estado
// y factura asociada). Reemplaza la lista; se vuelve con «‹ Volver».
function DetalleCotizacion({ cot, gestiona, onVolver, onConfirmar, onFacturar, onEditar, onCancelar, onPDF }) {
  const { db } = useData()
  const em = ESTADO_META[cot.estado] || { label: cot.estado, color: 'slate' }
  const ccy = ccyDe(cot)
  const sedeDespachoNombre = cot.sedeDespacho ? (db.SEDES || []).find((s) => s.id === cot.sedeDespacho)?.nombre || cot.sedeDespacho : ''

  const btnPDF = <Button variant="secondary" icon={<Icon.Download size={15} />} onClick={onPDF}>PDF</Button>
  const acciones = (() => {
    if (!gestiona) return btnPDF
    if (cot.estado === 'borrador') return (
      <>
        {btnPDF}
        <Button variant="secondary" icon={<Icon.Pencil size={15} />} onClick={onEditar}>Editar</Button>
        <Button icon={<Icon.Check size={16} />} onClick={onConfirmar}>Confirmar pedido</Button>
      </>
    )
    if (cot.estado === 'confirmada') return (
      <>
        {btnPDF}
        <Button variant="destructive" onClick={onCancelar}>Cancelar cotización</Button>
        <Button variant="dinero" icon={<Icon.Banknote size={16} />} onClick={onFacturar}>Facturar</Button>
      </>
    )
    return btnPDF
  })()

  return (
    <VistaDetalle onVolver={onVolver} icon={<Icon.ClipboardList size={18} />}
      titulo={cot.numeroCompleto || 'Cotización'} sub={`${em.label} · ${fmtDate(cot.fecha)}`} acciones={acciones}>
      <div className="grid grid-cols-1 lg:grid-cols-[340px_1fr] gap-4 items-start">
        {/* Columna izquierda: estado, cliente y despacho */}
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3">
            <div className="flex items-center gap-2 flex-wrap">
              <Badge color={em.color} dot>{em.label}</Badge>
              {cot.validez ? <span className="text-[11.5px] text-slate-400">Válida hasta {fmtDate(cot.validez)}</span> : null}
            </div>
            {/* Etapa del ciclo, en lenguaje claro */}
            <div className="text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2">
              {cot.estado === 'borrador' ? 'Cotización: propuesta editable. Al confirmarla se vuelve pedido/prefactura.'
                : cot.estado === 'confirmada' ? 'Pedido / prefactura: confirmada y en firme. Al facturarla se emite la factura fiscal.'
                : cot.estado === 'facturada' ? 'Facturada: se emitió la factura fiscal. La factura es inmutable.'
                : 'Cancelada: no genera efecto fiscal.'}
            </div>
            <div>
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Cliente</div>
              <div className="text-[13.5px] font-medium">{cot.clienteNombre || 'Consumidor final'}</div>
              {cot.clienteDocumento ? <div className="text-[12px] text-slate-500 num">{cot.clienteDocumento}</div> : null}
            </div>
            {sedeDespachoNombre ? (
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Despacho</div>
                <div className="text-[13px] font-medium inline-flex items-center gap-1.5"><Icon.Package size={14} className="text-slate-400" />{sedeDespachoNombre}</div>
              </div>
            ) : null}
            {cot.direccionEntrega ? (
              <div>
                <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Entrega</div>
                <div className="text-[12.5px] text-slate-600 dark:text-slate-300">{cot.direccionEntrega}</div>
              </div>
            ) : null}
          </div>

          {cot.notas ? (
            <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
              <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1">Notas</div>
              <div className="text-[12.5px] text-slate-600 dark:text-slate-300 whitespace-pre-wrap">{cot.notas}</div>
            </div>
          ) : null}

          {cot.documentoId ? (
            <div className="flex items-start gap-2 text-[12.5px] text-emerald-800 dark:text-emerald-300 bg-emerald-50 dark:bg-emerald-900/20 rounded-lg px-3 py-2.5">
              <Icon.Receipt size={15} className="mt-0.5 shrink-0" />
              <span>Factura emitida{cot.numeroFactura ? <>: <strong className="num">{cot.numeroFactura}</strong></> : cot.documentoNumero ? <>: <strong className="num">{cot.documentoNumero}</strong></> : ''}. El documento fiscal es inmutable; para corregir se anula con una reversa desde Facturación.</span>
            </div>
          ) : null}
        </div>

        {/* Columna derecha: líneas y totales */}
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
            <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Líneas ({(cot.lineas || []).length})</div>
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
              {(cot.lineas || []).map((l, i) => (
                <div key={i} className="flex items-center gap-3 px-3 py-2">
                  <div className="flex-1 min-w-0">
                    <div className="text-[13px] font-medium truncate">{l.nombre || l.sku}{l.exento ? <span className="ml-1.5 text-[11px] text-slate-400">exento</span> : null}</div>
                    <div className="text-[11px] text-slate-400 num">{fmtNum(l.cantidad)} × {fmtCurrency(l.precioUnitario, ccy)}</div>
                  </div>
                  <div className="num text-[13px] font-medium private-mask">{fmtCurrency(l.total, ccy)}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-4 space-y-1.5 text-[13px] max-w-sm ml-auto">
            <TotRow label="Subtotal" value={cot.subtotal} ccy={ccy} />
            <TotRow label="IVA" value={cot.iva} ccy={ccy} />
            {cot.igtf ? <TotRow label="IGTF" value={cot.igtf} ccy={ccy} /> : null}
            <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
              <span className="font-semibold">Total</span><span className="num font-semibold private-mask text-[16px]">{fmtCurrency(cot.total, ccy)}</span>
            </div>
          </div>
        </div>
      </div>
    </VistaDetalle>
  )
}

const TotRow = ({ label, value, ccy = 'VES' }) => (
  <div className="flex items-center justify-between"><span className="text-slate-500">{label}</span><span className="num private-mask">{fmtCurrency(value, ccy)}</span></div>
)

/* Facturar (registro de pago) — convierte el pedido/prefactura en factura fiscal.
 *
 * Reutiliza EL MISMO modal de cobro del POS (CobroModal): cobro mixto,
 * multimoneda, IGTF y vuelto flexible (moneda/medio, incluido el pago móvil). El
 * cobro se procesa por la MISMA ruta del servidor que el mostrador —EmitirFactura,
 * vía `facturarCotizacion`—: la conversión por divisa, el IGTF y el vuelto los
 * calcula el servidor, no esta pantalla. `canal="ventas"` selecciona los métodos
 * habilitados en Ventas; `onCobrar` cambia la ruta de emisión (facturar la
 * cotización en vez de emitir un documento nuevo) sin tocar el uso del POS. */
function FacturarModal({ cot, onClose, onSaved, toast }) {
  const [emitida, setEmitida] = useState(null) // documento fiscal tras el éxito

  // Líneas para el cobro: el precio NETO en Bs (con el descuento de línea ya
  // aplicado), exactamente como las arma el servidor al facturar. Así el preview
  // de totales de CobroModal coincide con la factura que se emitirá.
  const lineasCobro = useMemo(() => (cot.lineas || []).map((l) => ({
    sku: l.sku,
    cantidad: Number(l.cantidad) || 0,
    precioUnitario: Math.round((Number(l.precioUnitario) || 0) * (1 - (Number(l.descuento) || 0) / 100) * 100) / 100,
    exento: !!l.exento,
  })), [cot])

  const cerrarExito = async () => { await onSaved(); onClose() }

  // Estado de éxito: número de la factura emitida (documento fiscal).
  if (emitida) {
    return (
      <Modal open onClose={cerrarExito} size="sm" icon={<Icon.CircleCheck size={18} />}
        title="Factura emitida" sub={cot.numeroCompleto}
        footer={<Button onClick={cerrarExito} icon={<Icon.Check size={16} />}>Listo</Button>}>
        <div className="py-4 text-center">
          <div className="h-14 w-14 rounded-full bg-emerald-50 dark:bg-emerald-900/30 text-emerald-500 inline-flex items-center justify-center mb-3"><Icon.Receipt size={28} /></div>
          <div className="text-[11.5px] font-semibold tracking-wider text-slate-400">NÚMERO FISCAL ASIGNADO</div>
          <div className="mono text-[24px] font-semibold tracking-wide">{emitida.numeroCompleto || '—'}</div>
          <div className="text-[13px] text-slate-500 mt-1">Total: <strong className="num">{fmtCurrency(emitida.total ?? cot.total, 'VES')}</strong></div>
          {emitida.vuelto > 0.004 ? (
            <div className="mt-3 rounded-xl bg-emerald-50 dark:bg-emerald-900/25 border border-emerald-200 dark:border-emerald-700/50 px-4 py-2.5">
              <div className="text-[12px] text-emerald-800/80 dark:text-emerald-300/80">Entrega el vuelto</div>
              <div className="text-[19px] font-semibold num text-emerald-700 dark:text-emerald-300">{fmtCurrency(emitida.vuelto, emitida.vueltoMoneda || 'VES')}</div>
            </div>
          ) : null}
          <div className="mt-4 flex items-start gap-2 text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2.5 text-left">
            <Icon.Lock size={14} className="mt-0.5 shrink-0" />
            <span>La factura es un documento fiscal inmutable. Para corregir se anula con una reversa desde Facturación → Documentos.</span>
          </div>
        </div>
      </Modal>
    )
  }

  return (
    <CobroModal open onClose={onClose} canal="ventas"
      lineas={lineasCobro} clienteId={cot.clienteId} clienteNombre={cot.clienteNombre} contingencia={false}
      onCobrar={async (cobro) => {
        const res = await api.facturarCotizacion(cot.id, cobro)
        return res?.documento || res
      }}
      onEmitida={(doc) => {
        setEmitida(doc || null)
        toast({ title: 'Factura emitida', body: doc?.numeroCompleto || cot.numeroCompleto })
      }} />
  )
}

// Cancelar una cotización o pedido. Exige motivo (queda en la traza).
function CancelarModal({ cot, onClose, onSaved, toast }) {
  const [motivo, setMotivo] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)
  const err = !motivo.trim() ? 'El motivo es obligatorio.' : ''

  const confirmar = async () => {
    setTouched(true)
    if (err) return
    setBusy(true)
    try {
      await api.cancelarCotizacion(cot.id, motivo.trim())
      toast({ title: 'Cotización cancelada', body: cot.numeroCompleto || '' })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo cancelar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.CircleX size={18} />}
      title="Cancelar cotización" sub={cot.numeroCompleto}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Volver</Button>
        <Button variant="destructive" onClick={confirmar} loading={busy} icon={<Icon.CircleX size={16} />}>Cancelar cotización</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="flex items-start gap-2 text-[12.5px] text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/20 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>Una cotización cancelada no genera factura. No se elimina: queda con su estado para la traza.</span>
        </div>
        <Field label="Motivo de la cancelación" required error={touched ? err : ''}>
          <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!err} placeholder="Ej: El cliente desistió, precios vencidos…" autoFocus />
        </Field>
      </div>
    </Modal>
  )
}
