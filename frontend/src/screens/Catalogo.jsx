import { useState, useMemo, useRef, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, VistaDetalle, Empty, TableSkeleton, useToast, Field, Toggle, Modal } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { useData, useTasa } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { PrecioDual, fechaCortaVE } from '../components/tasa.jsx'
import { ImagenProducto } from '../components/producto.jsx'
import { monedaDe, precioEnMoneda, precioEnBs } from '../lib/precio.js'

// round2: redondeo a 2 decimales (misma regla que el backend) para el prorrateo y
// los cálculos de precio/descuento/ahorro del editor de combos.
const round2 = (v) => Math.round((Number(v) || 0) * 100) / 100

// monedasDeProducto arma la lista de monedas en que se puede fijar un precio:
// VES + las divisas ACTIVAS. Se garantiza incluir la moneda principal y la de un
// producto ya guardado. Con solo el dólar (o sin db.TASAS) queda ['VES','USD'].
function monedasDeProducto(db, monedaEmpresa, permiteUSD, extra) {
  const activas = db?.TASAS?.activas
  const codigos = Array.isArray(activas)
    ? activas.map((a) => (a.codigo || '').toUpperCase()).filter(Boolean)
    : (permiteUSD ? ['USD'] : [])
  const set = new Set(['VES', ...codigos])
  if (monedaEmpresa) set.add(monedaEmpresa.toUpperCase())
  if (extra) set.add(extra.toUpperCase())
  // VES primero, el resto en orden de aparición.
  return ['VES', ...[...set].filter((c) => c !== 'VES')]
}

// UNIDADES_FALLBACK: opciones mínimas por si el maestro (db.UNIDADES, que viene del
// bootstrap con las unidades ACTIVAS) todavía está vacío. La fuente real es
// Configuración › Unidades de medida.
const UNIDADES_FALLBACK = [
  { simbolo: 'unidad', nombre: 'Unidad', categoria: 'conteo' },
  { simbolo: 'kg', nombre: 'Kilogramo', categoria: 'peso' },
  { simbolo: 'litro', nombre: 'Litro', categoria: 'volumen' },
]

// La unidad de medida es UN SOLO control (el maestro es la fuente): de la unidad
// elegida se DERIVA cómo se vende el producto — una unidad de categoría «peso»
// (kg, g) significa venta por peso (se pesa en balanza), el resto es por cantidad.
// Ya no hay un selector de «tipo de venta» aparte; eso era lo que enredaba.
const CAT_ORDEN = ['conteo', 'peso', 'volumen', 'longitud']
const CAT_LABEL = { conteo: 'Conteo', peso: 'Peso', volumen: 'Volumen', longitud: 'Longitud' }

// Unidades del maestro (Configuración › Unidades) agrupadas por categoría para un
// <select> con <optgroup>. Si el valor guardado no está en el catálogo (dato viejo),
// se devuelve como `legado` para incluirlo suelto y no perderlo. Value = símbolo.
function unidadesAgrupadas(unidades, valorActual) {
  const base = Array.isArray(unidades) && unidades.length ? unidades : UNIDADES_FALLBACK
  const grupos = CAT_ORDEN
    .map((cat) => ({
      cat, label: CAT_LABEL[cat],
      items: base.filter((u) => (u.categoria || 'conteo') === cat)
        .map((u) => ({ value: u.simbolo, label: `${u.simbolo} — ${u.nombre}` })),
    }))
    .filter((g) => g.items.length)
  const v = (valorActual || '').trim()
  const legado = v && !base.some((u) => u.simbolo === v) ? v : ''
  return { grupos, legado }
}

// Categoría (conteo|peso|volumen|longitud) de una unidad por su símbolo, según el
// maestro; '' si no se encuentra. `esUnidadDePeso` decide si el producto se pesa.
function categoriaUnidad(unidades, simbolo) {
  const base = Array.isArray(unidades) && unidades.length ? unidades : UNIDADES_FALLBACK
  const u = base.find((x) => x.simbolo === (simbolo || '').trim())
  return u ? (u.categoria || 'conteo') : ''
}
const esUnidadDePeso = (unidades, simbolo) => categoriaUnidad(unidades, simbolo) === 'peso'

// Select de unidad de medida, agrupado por categoría, alimentado del maestro.
function SelectUnidad({ unidades, value, onChange }) {
  const { grupos, legado } = unidadesAgrupadas(unidades, value)
  return (
    <Select value={value} onChange={onChange}>
      {legado ? <option value={legado}>{legado}</option> : null}
      {grupos.map((g) => (
        <optgroup key={g.cat} label={g.label}>
          {g.items.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </optgroup>
      ))}
    </Select>
  )
}

// Nota inline que explica la venta por peso (cuando la unidad elegida es de peso).
function NotaUnidadPeso({ simbolo }) {
  return (
    <div className="flex items-start gap-2 text-[12px] text-sky-800 dark:text-sky-300 bg-sky-50 dark:bg-sky-900/20 border border-sky-200 dark:border-sky-800/60 rounded-lg px-3 py-2">
      <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" />
      <span>Producto <strong>a granel</strong>: al vender se <strong>pesa en balanza</strong> y el precio es <strong>por {simbolo || 'kg'}</strong>.</span>
    </div>
  )
}
const puedeEditar = (rol) => ['dueno', 'desarrollador'].includes(rol)

export function Catalogo({ onKardex }) {
  const { db, loading, error, reload, tasaDe } = useData()
  const { ui } = useUI()
  // Configuración de moneda de la empresa (R10): en qué moneda se capturan los
  // precios y si se admite capturarlos en dólares.
  const monedaEmpresa = db.EMPRESA?.monedaPrincipal || 'VES'
  const permiteUSD = !!db.EMPRESA?.preciosEnUsd || monedaEmpresa === 'USD'
  // Monedas seleccionables para el precio del producto: VES + divisas activas.
  const monedasProducto = monedasDeProducto(db, monedaEmpresa, permiteUSD)
  const toast = useToast()
  const productos = db.PRODUCTOS
  const rubros = db.RUBROS || []

  const [q, setQ] = useState('')
  const [rubro, setRubro] = useState('')
  const [sort, setSort] = useState({ key: 'nombre', dir: 1 })
  const [nuevo, setNuevo] = useState(false)
  const [importar, setImportar] = useState(false)
  const [detalle, setDetalle] = useState(null) // producto seleccionado

  const editable = puedeEditar(ui.rol)

  const rows = useMemo(() => {
    let list = (productos || []).filter((p) => {
      const term = q.trim().toLowerCase()
      const okQ = !term || p.nombre.toLowerCase().includes(term) || (p.sku || '').toLowerCase().includes(term)
      const okR = !rubro || p.rubro === rubro
      return okQ && okR
    })
    const { key, dir } = sort
    list = [...list].sort((a, b) => {
      const av = a[key], bv = b[key]
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir
      return String(av ?? '').localeCompare(String(bv ?? '')) * dir
    })
    return list
  }, [productos, q, rubro, sort])

  const toggleSort = (key) => setSort((s) => (s.key === key ? { key, dir: -s.dir } : { key, dir: 1 }))
  const pad = 'py-2.5'

  const SortTh = ({ k, children, align = 'left' }) => (
    <th className={`${pad} pr-3 font-medium cursor-pointer select-none text-${align}`} onClick={() => toggleSort(k)}>
      <span className={`inline-flex items-center gap-1 ${align === 'right' ? 'justify-end' : ''}`}>
        {children}
        {sort.key === k ? (sort.dir === 1 ? <Icon.ChevUp size={12} /> : <Icon.ChevDown size={12} />) : null}
      </span>
    </th>
  )

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar el catálogo"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  // Alta de producto a PANTALLA COMPLETA (estilo Odoo): al abrir "Nuevo producto",
  // el formulario reemplaza la lista y ocupa el ancho principal; al Volver/Guardar/
  // Cancelar se regresa a la lista.
  if (nuevo) {
    return (
      <div>
        <NuevoProducto rubros={rubros} onClose={() => setNuevo(false)} onSaved={reload} toast={toast}
          monedaEmpresa={monedaEmpresa} monedas={monedasProducto} unidades={db.UNIDADES || []}
          productos={productos || []} tasaDe={tasaDe} />
      </div>
    )
  }

  // Detalle del PRODUCTO a PANTALLA COMPLETA (estilo Odoo): al abrir un producto,
  // la vista de detalle reemplaza la lista y ocupa el ancho principal.
  if (detalle) {
    return (
      <div>
        <DetalleProducto producto={detalle} editable={editable} onClose={() => setDetalle(null)} onSaved={reload}
          onKardex={onKardex} toast={toast} monedaEmpresa={monedaEmpresa} tasaDe={tasaDe} unidades={db.UNIDADES || []}
          productos={productos || []}
          monedas={monedasDeProducto(db, monedaEmpresa, permiteUSD, monedaDe(detalle, monedaEmpresa))} />
      </div>
    )
  }

  return (
    <div>
      {/* Barra de herramientas */}
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar por nombre o SKU…"
          value={q} onChange={(e) => setQ(e.target.value)} />
        <Select className="!w-44" value={rubro} onChange={(e) => setRubro(e.target.value)}>
          <option value="">Todos los rubros</option>
          {rubros.map((r) => <option key={r.id} value={r.id}>{r.nombre}</option>)}
        </Select>
        <div className="ml-auto flex items-center gap-2">
          {editable ? <Button variant="secondary" icon={<Icon.Upload size={15} />} onClick={() => setImportar(true)}>Carga masiva</Button> : null}
          {editable ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNuevo(true)}>Nuevo producto</Button> : null}
        </div>
      </div>

      {importar ? (
        <ModalCargaMasiva unidades={db.UNIDADES || []} onClose={() => setImportar(false)} onSaved={reload} toast={toast} />
      ) : null}

      {/* Tabla */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || productos === undefined ? (
          <div className="p-4"><TableSkeleton rows={7} cols={5} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.Package size={22} />}
            title={q || rubro ? 'Sin resultados' : 'Aún no hay productos'}
            body={q || rubro ? 'Prueba con otro término o filtro.' : 'Carga tu primer producto para empezar a controlar el inventario.'}
            cta={editable && !q && !rubro ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNuevo(true)}>Nuevo producto</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <SortTh k="sku">SKU</SortTh>
                  <SortTh k="nombre">Producto</SortTh>
                  <SortTh k="rubro">Rubro</SortTh>
                  <SortTh k="unidadBase">Unidad</SortTh>
                  <SortTh k="precio" align="right">Precio</SortTh>
                  <th className={`${pad} pr-3 font-medium text-center`}>Estado</th>
                  <th className={`${pad} pr-3 font-medium text-right`}>Acciones</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((p) => (
                  <tr key={p.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover cursor-pointer" onClick={() => setDetalle(p)}>
                    <td className={`${pad} pr-3 num text-[12.5px] text-slate-500`}>{p.sku}</td>
                    <td className={`${pad} pr-3`}>
                      <div className="font-medium text-[13px] flex items-center gap-1.5">
                        {p.nombre}
                        {p.esCombo ? <Badge size="sm" color="huberp">Combo</Badge> : null}
                        {p.tipoVenta === 'peso' ? <Badge size="sm" color="sky">Por peso</Badge> : null}
                      </div>
                      {p.esCombo
                        ? <div className="text-[11px] text-slate-400">paquete · {(p.componentes || []).length} componente(s)</div>
                        : (p.presentaciones?.length ? <div className="text-[11px] text-slate-400">{p.presentaciones.length} presentación(es)</div> : null)}
                    </td>
                    <td className={`${pad} pr-3`}><Badge size="sm" color="slate">{rubros.find((r) => r.id === p.rubro)?.nombre || p.rubro || '—'}</Badge></td>
                    <td className={`${pad} pr-3 text-[12.5px] text-slate-500`}>{p.unidadBase}</td>
                    <td className={`${pad} pr-3 text-right num private-mask`}>
                      <PrecioCelda producto={p} ccy={ui.ccy} monedaEmpresa={monedaEmpresa} tasaDe={tasaDe} />
                    </td>
                    <td className={`${pad} pr-3 text-center`}>
                      {p.activo !== false ? <Badge size="sm" color="emerald" dot>Activo</Badge> : <Badge size="sm" color="slate">Inactivo</Badge>}
                    </td>
                    <td className={`${pad} pr-3 text-right`} onClick={(e) => e.stopPropagation()}>
                      <button onClick={() => onKardex && onKardex(p.sku)} title="Ver Kardex"
                        className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800">
                        <Icon.History size={15} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} producto(s)</div> : null}
    </div>
  )
}

/* PrecioCelda muestra el precio en la moneda de presentación elegida y, cuando
 * el producto está capturado en la otra moneda, su valor original debajo. Así se
 * ve de un golpe qué precio se fijó y qué se está cobrando (R10). */
function PrecioCelda({ producto, ccy, monedaEmpresa, tasaDe }) {
  const propia = monedaDe(producto, monedaEmpresa)
  const convertido = precioEnMoneda(producto, ccy, monedaEmpresa, tasaDe)
  if (convertido === null) {
    // Sin tasa no se convierte: se muestra tal como está guardado.
    return (
      <>
        <div className="font-medium">{fmtCurrency(producto.precio, propia)}</div>
        <div className="text-[10.5px] text-amber-600 dark:text-amber-400 font-normal">sin tasa</div>
      </>
    )
  }
  return (
    <>
      <div className="font-medium">{fmtCurrency(convertido, ccy)}</div>
      {propia !== ccy ? (
        <div className="text-[10.5px] text-slate-400 font-normal">fijado en {fmtCurrency(producto.precio, propia)}</div>
      ) : null}
    </>
  )
}

/* EditorCombo — bloque reutilizable (alta y edición) para armar un COMBO/paquete.
 *
 * Gestiona: el selector/lista de componentes (excluye combos y el propio
 * producto), la SUMA de sus precios en Bs en vivo, el PRECIO del paquete (Bs), un
 * DESCUENTO % que fija precio = suma×(1−%/100), y el AHORRO resultante. El precio
 * se captura y guarda en Bs (moneda VES): así la suma, el descuento y el ahorro
 * son coherentes, y si se deja en 0 el servidor lo fija a la suma.
 *
 * Props:
 *  · productos, monedaEmpresa, tasaDe — para resolver y valorar los componentes
 *  · skuPropio                        — SKU a excluir de la búsqueda (edición)
 *  · componentes / onComponentes      — [{sku,cantidad}] controlado
 *  · precio / onPrecio                — precio del paquete en Bs (string)
 *  · autoInicial                      — true: el precio sigue a la suma hasta que se toque
 */
function EditorCombo({ productos, monedaEmpresa, tasaDe, skuPropio = '', componentes, onComponentes, precio, onPrecio, autoInicial = true }) {
  const [q, setQ] = useState('')
  const [descuento, setDescuento] = useState('')
  // precioAuto: mientras esté activo, el precio sigue a la suma de componentes.
  const [precioAuto, setPrecioAuto] = useState(autoInicial)

  // Precio unitario en Bs de un SKU del catálogo (base, con la tasa del día).
  const bsDe = (prod) => {
    const v = precioEnBs(prod, monedaEmpresa, tasaDe)
    return v === null ? null : v
  }

  const detalles = componentes.map((c) => {
    const prod = productos.find((p) => p.sku === c.sku)
    const base = prod ? bsDe(prod) : null
    const cc = Number(c.cantidad) || 0
    return { ...c, prod, base, subtotal: base === null ? null : round2(base * cc) }
  })
  const sumaBs = detalles.reduce((s, d) => s + (d.subtotal || 0), 0)
  const faltaTasa = detalles.some((d) => d.prod && d.base === null)

  // El precio sigue a la suma mientras no se haya tocado (alta) o el usuario deje
  // el auto activo. round2 para no arrastrar colas de flotante.
  useEffect(() => {
    if (precioAuto) onPrecio(sumaBs ? String(round2(sumaBs)) : '')
  }, [sumaBs, precioAuto]) // eslint-disable-line react-hooks/exhaustive-deps

  const precioNum = Number(precio) || 0
  const ahorro = round2(sumaBs - precioNum)
  const ahorroPct = sumaBs > 0 ? Math.round((ahorro / sumaBs) * 1000) / 10 : 0

  const candidatos = useMemo(() => {
    const term = q.trim().toLowerCase()
    const yaSku = new Set(componentes.map((c) => c.sku))
    return (productos || [])
      .filter((p) => !p.esCombo && p.sku !== skuPropio && !yaSku.has(p.sku) && p.activo !== false)
      .filter((p) => !term || p.nombre.toLowerCase().includes(term) || (p.sku || '').toLowerCase().includes(term))
      .slice(0, 6)
  }, [q, productos, componentes, skuPropio])

  const agregarComp = (p) => { onComponentes([...componentes, { sku: p.sku, cantidad: 1 }]); setQ('') }
  const setCant = (sku, v) => onComponentes(componentes.map((c) => (c.sku === sku ? { ...c, cantidad: v } : c)))
  const quitarComp = (sku) => onComponentes(componentes.filter((c) => c.sku !== sku))

  // Escribir el descuento fija el precio = suma×(1−%/100) y desactiva el auto.
  const setDesc = (v) => {
    setDescuento(v)
    setPrecioAuto(false)
    const d = Number(v)
    if (v !== '' && !isNaN(d)) onPrecio(String(round2(sumaBs * (1 - d / 100))))
  }
  // Editar el precio directo también es válido: desactiva el auto y limpia el %.
  const setPrecioDirecto = (v) => { setPrecioAuto(false); setDescuento(''); onPrecio(v) }

  return (
    <div className="space-y-3">
      {/* Selector de componentes */}
      <Field label="Componentes del combo" hint="productos que arma el paquete (no otros combos)">
        <Input value={q} onChange={(e) => setQ(e.target.value)} icon={<Icon.Search size={15} />}
          placeholder="Buscar producto por nombre o SKU…" />
        {q && candidatos.length ? (
          <div className="mt-1 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
            {candidatos.map((p) => (
              <button key={p.sku} type="button" onClick={() => agregarComp(p)}
                className="w-full flex items-center gap-2 px-3 h-10 text-left hover:bg-slate-50 dark:hover:bg-slate-800/60">
                <Icon.Package size={15} className="text-slate-400" />
                <span className="flex-1 min-w-0 text-[13px] truncate">{p.nombre}</span>
                <span className="num text-[11.5px] text-slate-400">{fmtCurrency(bsDe(p) ?? 0, 'VES')}</span>
              </button>
            ))}
          </div>
        ) : q && !candidatos.length ? (
          <div className="mt-1 text-[12px] text-slate-400 px-1">Sin productos para «{q}».</div>
        ) : null}
      </Field>

      {/* Lista editable de componentes */}
      {componentes.length === 0 ? (
        <div className="text-[12.5px] text-slate-400 rounded-lg border border-dashed border-slate-300 dark:border-slate-700 px-3 py-4 text-center">
          Aún no agregaste componentes. Busca productos arriba para armar el paquete.
        </div>
      ) : (
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
          {detalles.map((d) => (
            <div key={d.sku} className="flex items-center gap-3 px-3 py-2">
              <div className="flex-1 min-w-0">
                <div className="text-[13px] font-medium truncate">{d.prod?.nombre || d.sku}</div>
                <div className="text-[11px] text-slate-400 num">
                  {d.base === null ? 'sin tasa' : fmtCurrency(d.base, 'VES')} c/u
                </div>
              </div>
              <Input type="number" min="0" step="1" className="!w-20" value={d.cantidad}
                onChange={(e) => setCant(d.sku, e.target.value)} />
              <div className="num text-[12.5px] font-medium w-28 text-right">
                {d.subtotal === null ? '—' : fmtCurrency(d.subtotal, 'VES')}
              </div>
              <button type="button" onClick={() => quitarComp(d.sku)} title="Quitar"
                className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800">
                <Icon.X size={15} />
              </button>
            </div>
          ))}
          <div className="flex items-center justify-between px-3 py-2 bg-slate-50/60 dark:bg-slate-900/40">
            <span className="text-[12px] text-slate-500">Suma de componentes</span>
            <span className="num text-[13px] font-semibold">{fmtCurrency(sumaBs, 'VES')}</span>
          </div>
        </div>
      )}
      {faltaTasa ? (
        <div className="flex items-start gap-1.5 text-[12px] text-amber-600 dark:text-amber-400">
          <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" />
          <span>Algún componente está en divisa y falta su tasa: la suma y el prorrateo pueden quedar incompletos.</span>
        </div>
      ) : null}

      {/* Precio del paquete + descuento + ahorro */}
      <div className="grid grid-cols-2 gap-3">
        <Field label="Precio del combo (Bs)" hint="por defecto, la suma">
          <Input type="number" min="0" step="0.01" value={precio} onChange={(e) => setPrecioDirecto(e.target.value)} className="num" placeholder="0,00" />
        </Field>
        <Field label="Descuento %" hint="fija el precio sobre la suma">
          <Input type="number" min="0" max="100" step="0.1" value={descuento} onChange={(e) => setDesc(e.target.value)} className="num" placeholder="0" />
        </Field>
      </div>
      <div className="flex items-center justify-between rounded-lg bg-emerald-50 dark:bg-emerald-900/20 px-3 py-2">
        <span className="text-[12px] text-emerald-700 dark:text-emerald-300">Ahorro del cliente</span>
        <span className="num text-[13px] font-semibold text-emerald-700 dark:text-emerald-300">
          {fmtCurrency(ahorro, 'VES')}{sumaBs > 0 ? ` · ${fmtNum(ahorroPct, 1)}%` : ''}
        </span>
      </div>
    </div>
  )
}

// Alta de producto a PANTALLA COMPLETA. Validación inline, no solo al enviar.
function NuevoProducto({ rubros, onClose, onSaved, toast, monedaEmpresa = 'VES', monedas = ['VES'], unidades = [], productos = [], tasaDe }) {
  // El precio se captura en la moneda principal de la empresa (R10) y el
  // homólogo se calcula al lado: nunca se guardan los dos.
  const [f, setF] = useState({
    sku: '', nombre: '', rubro: rubros[0]?.id || '', unidadBase: 'unidad', precio: '',
    moneda: monedaEmpresa, codigoBarras: '', exentoIva: false,
    esCombo: false, componentes: [],
  })
  const esPeso = esUnidadDePeso(unidades, f.unidadBase)
  const esCombo = f.esCombo
  const [touched, setTouched] = useState({})
  const [busy, setBusy] = useState(false)

  const errs = {
    sku: !f.sku.trim() ? 'Ingresa un SKU.' : '',
    nombre: !f.nombre.trim() ? 'Ingresa el nombre.' : '',
    // Combo: el precio puede quedar en 0 (el servidor lo fija a la suma); en cambio
    // exige al menos un componente.
    precio: esCombo ? '' : (f.precio === '' || isNaN(Number(f.precio)) ? 'Precio inválido.' : Number(f.precio) < 0 ? 'No puede ser negativo.' : ''),
    componentes: esCombo && f.componentes.length === 0 ? 'Agrega al menos un componente.' : '',
  }
  const valid = !errs.sku && !errs.nombre && !errs.precio && !errs.componentes
  const set = (k) => (e) => setF((s) => ({ ...s, [k]: e.target.value }))
  const blur = (k) => () => setTouched((t) => ({ ...t, [k]: true }))

  const save = async () => {
    setTouched({ sku: true, nombre: true, precio: true, componentes: true })
    if (!valid) return
    setBusy(true)
    try {
      // Combo: se manda por unidad, con esCombo + componentes; el precio va en Bs
      // (VES) y, si es 0, el servidor lo fija a la suma de componentes.
      const payload = esCombo
        ? {
            sku: f.sku.trim(), nombre: f.nombre.trim(), rubro: f.rubro, unidadBase: 'unidad',
            precio: Number(f.precio) || 0, moneda: 'VES', codigoBarras: f.codigoBarras.trim(),
            exentoIva: f.exentoIva, tipoVenta: 'unidad', esCombo: true,
            componentes: f.componentes.map((c) => ({ sku: c.sku, cantidad: Number(c.cantidad) || 0 })),
          }
        : {
            sku: f.sku.trim(), nombre: f.nombre.trim(), rubro: f.rubro,
            unidadBase: f.unidadBase || 'unidad', precio: Number(f.precio), moneda: f.moneda,
            codigoBarras: f.codigoBarras.trim(), exentoIva: f.exentoIva, tipoVenta: esPeso ? 'peso' : 'unidad',
          }
      await api.createProducto(payload)
      toast({ title: 'Producto creado', body: `${f.nombre.trim()} se agregó al catálogo.` })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo crear', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  const acciones = (
    <>
      <Button variant="ghost" onClick={onClose}>Cancelar</Button>
      <Button onClick={save} loading={busy} icon={<Icon.Check size={16} />}>Crear</Button>
    </>
  )

  return (
    <VistaDetalle onVolver={onClose} icon={<Icon.Package size={18} />}
      titulo="Nuevo producto" sub="Datos esenciales; el resto es opcional." acciones={acciones} maxWidth="max-w-2xl">
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3.5">
        <Field label="SKU" required error={touched.sku ? errs.sku : ''}>
          <Input value={f.sku} onChange={set('sku')} onBlur={blur('sku')} invalid={touched.sku && !!errs.sku} placeholder="Ej: BEB-001" autoFocus />
        </Field>
        <Field label="Nombre" required error={touched.nombre ? errs.nombre : ''}>
          <Input value={f.nombre} onChange={set('nombre')} onBlur={blur('nombre')} invalid={touched.nombre && !!errs.nombre} placeholder="Ej: Refresco 2L" />
        </Field>
        {/* Combo/paquete: agrupa otros productos con un precio propio. Al venderse se
            explota en las líneas de sus componentes (precio prorrateado, IVA por
            ítem). Un combo es por unidad y no tiene existencia propia. */}
        <Toggle checked={f.esCombo} onChange={(v) => setF((s) => ({ ...s, esCombo: v, unidadBase: 'unidad' }))}
          label="Es un combo / paquete" sub="Agrupa varios productos con un precio propio; al venderse se descompone en sus componentes." />
        <div className={`grid ${esCombo ? 'grid-cols-1' : 'grid-cols-2'} gap-3`}>
          <Field label="Rubro">
            <Select value={f.rubro} onChange={set('rubro')}>
              {rubros.length === 0 ? <option value="">—</option> : null}
              {rubros.map((r) => <option key={r.id} value={r.id}>{r.nombre}</option>)}
            </Select>
          </Field>
          {/* Unidad de medida: UN solo control (del maestro). De la unidad se deriva
              si el producto se vende por peso (categoría peso). No aplica a un combo. */}
          {!esCombo ? (
            <Field label="Unidad de medida" hint="del maestro (Configuración › Unidades)">
              <SelectUnidad unidades={unidades} value={f.unidadBase} onChange={set('unidadBase')} />
            </Field>
          ) : null}
        </div>
        {!esCombo && esPeso ? <NotaUnidadPeso simbolo={f.unidadBase} /> : null}
        {esCombo ? (
          <>
            <EditorCombo productos={productos} monedaEmpresa={monedaEmpresa} tasaDe={tasaDe} skuPropio={f.sku.trim()}
              componentes={f.componentes} onComponentes={(cs) => { setF((s) => ({ ...s, componentes: cs })); setTouched((t) => ({ ...t, componentes: true })) }}
              precio={f.precio} onPrecio={(v) => setF((s) => ({ ...s, precio: v }))} autoInicial />
            {touched.componentes && errs.componentes ? (
              <div className="flex items-start gap-1.5 text-[12px] text-red-600 dark:text-red-400">
                <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{errs.componentes}</span>
              </div>
            ) : null}
          </>
        ) : (
          /* Patrón de moneda dual del prototipo: se escribe en una moneda y la
             otra se calcula sola con la tasa del día. Por peso, es el precio por kg. */
          <PrecioDual label={esPeso ? `Precio por ${f.unidadBase || 'kg'}` : 'Precio base por unidad'} valor={f.precio}
            onChange={(v) => { setF((s) => ({ ...s, precio: v })); setTouched((t) => ({ ...t, precio: true })) }}
            moneda={f.moneda} onMoneda={(m) => setF((s) => ({ ...s, moneda: m }))}
            monedas={monedas} error={touched.precio ? errs.precio : ''} />
        )}

        {/* Código de barras propio del producto (R11): es lo que la pistola
            lectora escribe en el buscador para que el ítem se agregue solo. */}
        <Field label="Código de barras" hint="opcional · el de la pistola lectora">
          <Input value={f.codigoBarras} onChange={set('codigoBarras')} placeholder="7591234000104" className="mono" />
        </Field>

        {/* Condición de IVA: en Venezuela buena parte de la cesta básica está
            exenta, y facturarle 16% es un error fiscal, no un redondeo. */}
        <Toggle checked={f.exentoIva} onChange={(v) => setF((s) => ({ ...s, exentoIva: v }))}
          label="Exento de IVA" sub="Harina de maíz, arroz y demás cesta básica. La factura separa base imponible de base exenta." />
      </div>
    </VistaDetalle>
  )
}

// Detalle del producto + gestión de presentaciones (Bulto x24, etc.).
function DetalleProducto({ producto, editable, onClose, onSaved, onKardex, toast, monedaEmpresa = 'VES', tasaDe, monedas = ['VES'], unidades = [], productos = [] }) {
  // Divisa de referencia para el equivalente del precio: la del producto si es
  // una divisa, o la primera divisa activa si el producto está en Bs.
  const propiaMon = monedaDe(producto, monedaEmpresa)
  const refDivisa = propiaMon !== 'VES' ? propiaMon : (monedas.find((c) => c !== 'VES') || 'USD')
  const tasa = useTasa(refDivisa)
  const esPeso = producto.tipoVenta === 'peso'
  const [adding, setAdding] = useState(false)
  const [f, setF] = useState({ nombre: '', factorConversion: '', precio: '', codigoBarras: '' })
  const [busy, setBusy] = useState(false)
  const [editando, setEditando] = useState(false)
  const [etiqueta, setEtiqueta] = useState(false)
  const [subiendo, setSubiendo] = useState(false)
  const archivoRef = useRef(null)

  // Imagen del producto (R4): se sube al bucket de la empresa desde acá mismo.
  const subirImagen = async (file) => {
    if (!file) return
    setSubiendo(true)
    try {
      await api.subirImagenProducto(producto.sku, file)
      toast({ title: 'Imagen actualizada', body: producto.nombre })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo subir', body: e?.message || 'Error', kind: 'error' })
    } finally {
      setSubiendo(false)
    }
  }

  const quitarImagen = async () => {
    setSubiendo(true)
    try {
      await api.quitarImagenProducto(producto.sku)
      toast({ title: 'Imagen quitada', body: producto.nombre })
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo quitar', body: e?.message || 'Error', kind: 'error' })
    } finally {
      setSubiendo(false)
    }
  }

  const validPres = f.nombre.trim() && Number(f.factorConversion) > 0 && f.precio !== '' && !isNaN(Number(f.precio))

  const addPres = async () => {
    if (!validPres) return
    setBusy(true)
    try {
      await api.createPresentacion(producto.id, {
        nombre: f.nombre.trim(), factorConversion: Number(f.factorConversion),
        precio: Number(f.precio), codigoBarras: f.codigoBarras.trim(),
      })
      toast({ title: 'Presentación agregada', body: `${f.nombre.trim()} para ${producto.nombre}.` })
      setF({ nombre: '', factorConversion: '', precio: '', codigoBarras: '' })
      setAdding(false)
      await onSaved()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo agregar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  const acciones = (
    <>
      {editable ? <Button variant="secondary" icon={<Icon.Settings size={15} />} onClick={() => setEditando(true)}>Editar</Button> : null}
      <Button variant="secondary" icon={<Icon.History size={15} />} onClick={() => { onClose(); onKardex && onKardex(producto.sku) }}>Ver Kardex</Button>
    </>
  )

  return (
    <VistaDetalle onVolver={onClose} icon={<Icon.Package size={18} />}
      titulo={producto.nombre} sub={`SKU ${producto.sku} · ${producto.unidadBase}`} acciones={acciones}>
      <div className="grid grid-cols-1 lg:grid-cols-[360px_1fr] gap-4 items-start">
        {/* Columna izquierda: imagen, código de barras, precio y estado */}
        <div className="space-y-4">
          {/* Imagen: se arrastra o se elige un archivo. Sin imagen se muestra el
              marcador explícito, nunca un hueco silencioso. */}
          <div
            onDragOver={(e) => { e.preventDefault() }}
            onDrop={(e) => { e.preventDefault(); if (editable) subirImagen(e.dataTransfer?.files?.[0]) }}
            className="rounded-xl border border-slate-200 dark:border-slate-700 overflow-hidden">
            <ImagenProducto producto={producto} alto={150} />
            {editable ? (
              <div className="flex items-center gap-2 p-2.5 border-t border-slate-200 dark:border-slate-800">
                <input ref={archivoRef} type="file" accept="image/png,image/jpeg,image/webp" className="hidden"
                  onChange={(e) => subirImagen(e.target.files?.[0])} />
                <Button size="sm" variant="secondary" loading={subiendo} icon={<Icon.Upload size={15} />}
                  onClick={() => archivoRef.current?.click()}>
                  {producto.imagenUrl ? 'Cambiar imagen' : 'Subir imagen'}
                </Button>
                {producto.imagenUrl ? (
                  <Button size="sm" variant="ghost" onClick={quitarImagen} loading={subiendo}>Quitar</Button>
                ) : null}
                <span className="text-[11px] text-slate-400 ml-auto">o arrastra el archivo · máx. 4 MiB</span>
              </div>
            ) : null}
          </div>

          {/* Código de barras propio (R11) + etiqueta */}
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-3 flex items-center gap-3">
            <Icon.Tag size={16} className="text-slate-400 shrink-0" />
            <div className="flex-1 min-w-0">
              <div className="text-[11px] uppercase tracking-wide text-slate-400">Código de barras</div>
              <div className="mono text-[13px] truncate">{producto.codigoBarras || 'sin código asignado'}</div>
            </div>
            <Button size="sm" variant="ghost" icon={<Icon.Printer size={15} />} onClick={() => setEtiqueta(true)}>Etiqueta</Button>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-3">
              <div className="text-[11px] uppercase tracking-wide text-slate-400">{esPeso ? 'Precio por kg' : 'Precio base'}</div>
              <div className="text-lg font-semibold num private-mask mt-0.5">
                {fmtCurrency(producto.precio, monedaDe(producto, monedaEmpresa))}{esPeso ? <span className="text-[11px] font-normal text-slate-400">/kg</span> : null}
              </div>
              {/* Homólogo calculado, nunca guardado: si cambia la tasa, cambia solo. */}
              <div className="text-[11px] text-slate-400 num mt-0.5">
                {(() => {
                  // El equivalente se muestra en Bs si el precio está en divisa, o
                  // en la divisa de referencia si está en Bs.
                  const otra = propiaMon !== 'VES' ? 'VES' : refDivisa
                  const eq = precioEnMoneda(producto, otra, monedaEmpresa, tasaDe)
                  if (eq === null) return 'Sin tasa para el equivalente'
                  return `≈ ${fmtCurrency(eq, otra)} a ${tasa.fuenteLabel} · ${fechaCortaVE(tasa.fechaValor, tasa.esDeHoy)}`
                })()}
              </div>
            </div>
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 p-3">
              <div className="text-[11px] uppercase tracking-wide text-slate-400">Estado e IVA</div>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {producto.activo !== false ? <Badge color="emerald" dot>Activo</Badge> : <Badge color="slate">Inactivo</Badge>}
                {producto.exentoIva ? <Badge color="slate">Exento de IVA</Badge> : <Badge color="huberp">IVA 16%</Badge>}
                <Badge color="sky">{esPeso ? 'Por peso (kg)' : 'Por unidad'}</Badge>
              </div>
            </div>
          </div>
        </div>

        {/* Columna derecha: presentaciones */}
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
          <div className="flex items-center justify-between mb-2">
            <div className="text-[13px] font-semibold">Presentaciones</div>
            {editable && !adding ? <Button size="sm" variant="ghost" icon={<Icon.Plus size={15} />} onClick={() => setAdding(true)}>Agregar</Button> : null}
          </div>
          <p className="text-[11.5px] text-slate-500 mb-2.5">Las presentaciones (ej. “Bulto x24”) descuentan del mismo stock base según su factor de conversión, sin duplicar el producto.</p>

          {(producto.presentaciones || []).length === 0 && !adding ? (
            <div className="text-[12.5px] text-slate-400 py-2">Sin presentaciones. Se vende por {producto.unidadBase}.</div>
          ) : (
            <div className="space-y-1.5">
              {(producto.presentaciones || []).map((pr) => (
                <div key={pr.id} className="flex items-center gap-3 p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/60">
                  <Icon.Boxes size={16} className="text-slate-400" />
                  <div className="flex-1 min-w-0">
                    <div className="text-[13px] font-medium truncate">{pr.nombre}</div>
                    <div className="text-[11px] text-slate-400 num">x{pr.factorConversion} · {pr.codigoBarras || 'sin código'}</div>
                  </div>
                  <div className="num text-[12.5px] font-medium private-mask">{fmtCurrency(pr.precio, 'VES')}</div>
                </div>
              ))}
            </div>
          )}

          {adding ? (
            <div className="mt-3 p-3 rounded-lg border border-slate-200 dark:border-slate-700 space-y-2.5">
              <Field label="Nombre" required><Input value={f.nombre} onChange={(e) => setF((s) => ({ ...s, nombre: e.target.value }))} placeholder="Ej: Bulto x24" autoFocus /></Field>
              <div className="grid grid-cols-2 gap-2.5">
                <Field label="Factor" required hint="unid. base"><Input type="number" min="1" step="1" value={f.factorConversion} onChange={(e) => setF((s) => ({ ...s, factorConversion: e.target.value }))} placeholder="24" /></Field>
                <Field label="Precio" required><Input type="number" min="0" step="0.01" value={f.precio} onChange={(e) => setF((s) => ({ ...s, precio: e.target.value }))} placeholder="0,00" /></Field>
              </div>
              <Field label="Código de barras" hint="opcional"><Input value={f.codigoBarras} onChange={(e) => setF((s) => ({ ...s, codigoBarras: e.target.value }))} placeholder="7591234567890" /></Field>
              <div className="flex items-center justify-end gap-2 pt-1">
                <Button size="sm" variant="ghost" onClick={() => setAdding(false)}>Cancelar</Button>
                <Button size="sm" onClick={addPres} loading={busy} disabled={!validPres} icon={<Icon.Check size={15} />}>Guardar</Button>
              </div>
            </div>
          ) : null}
        </div>
      </div>

      {editando ? (
        <EditarProducto producto={producto} monedaEmpresa={monedaEmpresa} monedas={monedas} unidades={unidades}
          productos={productos} tasaDe={tasaDe}
          onClose={() => setEditando(false)}
          onSaved={async () => { await onSaved(); onClose() }} toast={toast} />
      ) : null}
      {etiqueta ? <EtiquetaProducto producto={producto} monedaEmpresa={monedaEmpresa} onClose={() => setEtiqueta(false)} /> : null}
    </VistaDetalle>
  )
}

/* Editar producto: nombre, precio (con su moneda), condición de IVA y código de
 * barras. No toca existencias — eso solo se mueve por el ledger. */
function EditarProducto({ producto, monedaEmpresa, monedas = ['VES'], unidades = [], productos = [], tasaDe, onClose, onSaved, toast }) {
  const [f, setF] = useState({
    nombre: producto.nombre || '',
    precio: producto.precio ?? '',
    moneda: monedaDe(producto, monedaEmpresa),
    codigoBarras: producto.codigoBarras || '',
    exentoIva: !!producto.exentoIva,
    activo: producto.activo !== false,
    unidadBase: producto.unidadBase || 'unidad',
    esCombo: !!producto.esCombo,
    componentes: (producto.componentes || []).map((c) => ({ sku: c.sku, cantidad: Number(c.cantidad) || 0 })),
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const esPeso = esUnidadDePeso(unidades, f.unidadBase)
  const esCombo = f.esCombo

  const guardar = async () => {
    if (!f.nombre.trim()) { setError('El nombre no puede quedar vacío.'); return }
    if (esCombo && f.componentes.length === 0) { setError('Un combo necesita al menos un componente.'); return }
    if (!esCombo && !(Number(f.precio) > 0)) { setError('El precio debe ser mayor que 0.'); return }
    setBusy(true); setError('')
    try {
      // Combo: precio en Bs (VES); 0 deja que el servidor lo fije a la suma.
      const payload = esCombo
        ? {
            nombre: f.nombre.trim(), precio: Number(f.precio) || 0, moneda: 'VES',
            codigoBarras: f.codigoBarras.trim(), exentoIva: f.exentoIva, activo: f.activo,
            tipoVenta: 'unidad', unidadBase: 'unidad', esCombo: true,
            componentes: f.componentes.map((c) => ({ sku: c.sku, cantidad: Number(c.cantidad) || 0 })),
          }
        : {
            nombre: f.nombre.trim(), precio: Number(f.precio), moneda: f.moneda,
            codigoBarras: f.codigoBarras.trim(), exentoIva: f.exentoIva, activo: f.activo,
            tipoVenta: esPeso ? 'peso' : 'unidad', unidadBase: f.unidadBase || 'unidad',
          }
      await api.actualizarProducto(producto.sku, payload)
      toast({ title: 'Producto actualizado', body: f.nombre.trim() })
      await onSaved()
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Package size={18} />}
      title="Editar producto" sub={`SKU ${producto.sku} · no cambia existencias`}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre" required>
          <Input value={f.nombre} onChange={(e) => setF((s) => ({ ...s, nombre: e.target.value }))} autoFocus />
        </Field>
        {/* Combo/paquete: agrupa otros productos con un precio propio; al venderse se
            explota en sus componentes (precio prorrateado, IVA por ítem). */}
        <Toggle checked={f.esCombo} onChange={(v) => setF((s) => ({ ...s, esCombo: v, unidadBase: 'unidad' }))}
          label="Es un combo / paquete" sub="Agrupa varios productos con un precio propio; al venderse se descompone en sus componentes." />
        {!esCombo ? (
          <>
            {/* Unidad de medida: UN solo control (del maestro). De la unidad elegida se
                deriva si el producto se vende por peso (categoría peso → balanza). */}
            <Field label="Unidad de medida" hint="del maestro (Configuración › Unidades)">
              <SelectUnidad unidades={unidades} value={f.unidadBase} onChange={(e) => setF((s) => ({ ...s, unidadBase: e.target.value }))} />
            </Field>
            {esPeso ? <NotaUnidadPeso simbolo={f.unidadBase} /> : null}
          </>
        ) : null}
        {esCombo ? (
          <EditorCombo productos={productos} monedaEmpresa={monedaEmpresa} tasaDe={tasaDe} skuPropio={producto.sku}
            componentes={f.componentes} onComponentes={(cs) => setF((s) => ({ ...s, componentes: cs }))}
            precio={f.precio} onPrecio={(v) => setF((s) => ({ ...s, precio: v }))} autoInicial={false} />
        ) : (
          <PrecioDual label={esPeso ? `Precio por ${f.unidadBase || 'kg'}` : 'Precio base por unidad'} valor={f.precio}
            onChange={(v) => setF((s) => ({ ...s, precio: v }))}
            moneda={f.moneda} onMoneda={(m) => setF((s) => ({ ...s, moneda: m }))} monedas={monedas} />
        )}
        <Field label="Código de barras" hint="el de la pistola lectora">
          <Input value={f.codigoBarras} onChange={(e) => setF((s) => ({ ...s, codigoBarras: e.target.value }))}
            placeholder="7591234000104" className="mono" />
        </Field>
        <Toggle checked={f.exentoIva} onChange={(v) => setF((s) => ({ ...s, exentoIva: v }))}
          label="Exento de IVA" sub="Cambia la próxima factura: el IVA sale solo de la base imponible." />
        <Toggle checked={f.activo} onChange={(v) => setF((s) => ({ ...s, activo: v }))}
          label="Activo" sub="Un producto inactivo conserva su histórico pero no se puede vender." />
        {error ? (
          <div className="flex items-start gap-1.5 text-[12px] text-red-600 dark:text-red-400">
            <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{error}</span>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}

/* Etiqueta de estante/producto para imprimir: nombre, precio y el código en
 * monoespaciada grande.
 *
 * Deliberadamente NO dibuja las barras todavía: hacerlo bien exige las tablas de
 * codificación de Code 128 o EAN-13 tomadas de una librería verificada, y una
 * etiqueta que la pistola no pueda leer es peor que ninguna. El código impreso ya
 * sirve para teclearlo, y el gráfico llega con el módulo de impresión (que es
 * también quien habla con la impresora de etiquetas). */
function EtiquetaProducto({ producto, monedaEmpresa, onClose }) {
  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Printer size={18} />}
      title="Etiqueta del producto" sub="Para el estante o el empaque"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cerrar</Button>
        <Button variant="secondary" icon={<Icon.Printer size={15} />} onClick={() => window.print()}>Imprimir</Button>
      </>}>
      <div className="rounded-xl border-2 border-slate-900 dark:border-slate-100 p-4 text-center">
        <div className="text-[15px] font-semibold leading-tight">{producto.nombre}</div>
        <div className="text-[26px] font-bold num mt-1">
          {fmtCurrency(producto.precio, monedaDe(producto, monedaEmpresa))}
          {producto.tipoVenta === 'peso' ? <span className="text-[14px] font-normal text-slate-500"> /kg</span> : null}
        </div>
        <div className="mono text-[15px] tracking-[0.18em] mt-2">{producto.codigoBarras || producto.sku}</div>
        <div className="text-[10.5px] text-slate-500 mt-1">{producto.sku}{producto.exentoIva ? ' · exento de IVA' : ''}</div>
      </div>
      <div className="mt-3 text-[11.5px] text-slate-500">
        Falta el gráfico de barras: se dibuja con el módulo de impresión, que es el que habla con la
        impresora de etiquetas. El código de arriba ya se puede teclear o pegar en el buscador.
      </div>
    </Modal>
  )
}

/* --- Carga masiva de productos (CSV) --------------------------------------
 * Flujo en dos pasos sobre el MISMO endpoint: primero VALIDAR (confirmar=false,
 * vista previa que clasifica nuevo/actualiza/error sin escribir) y luego APLICAR
 * (confirmar=true, todo-o-nada). Es UPSERT: un SKU que ya existe se ACTUALIZA, y
 * eso exige una confirmación explícita del usuario. La unidad de cada fila debe
 * existir en el maestro (el backend rechaza símbolos desconocidos). La existencia
 * inicial de una fila nueva entra como ajuste auditado. */

// Parser CSV mínimo pero correcto: respeta comillas dobles, comillas escapadas
// ("") y CRLF/LF, y detecta el separador (coma o punto y coma, común en Excel en
// español) por la línea de encabezado.
function parseCSV(texto) {
  const limpio = texto.replace(/^﻿/, '')
  const primera = limpio.split(/\r?\n/)[0] || ''
  const sep = (primera.match(/;/g) || []).length > (primera.match(/,/g) || []).length ? ';' : ','
  const filas = []
  let campo = '', fila = [], enComillas = false
  const cerrarFila = () => { fila.push(campo); campo = ''; if (fila.some((x) => x.trim() !== '')) filas.push(fila); fila = [] }
  for (let i = 0; i < limpio.length; i++) {
    const c = limpio[i]
    if (enComillas) {
      if (c === '"') { if (limpio[i + 1] === '"') { campo += '"'; i++ } else enComillas = false }
      else campo += c
    } else if (c === '"') enComillas = true
    else if (c === sep) { fila.push(campo); campo = '' }
    else if (c === '\n') cerrarFila()
    else if (c === '\r') { if (limpio[i + 1] === '\n') i++; cerrarFila() }
    else campo += c
  }
  if (campo !== '' || fila.length) cerrarFila()
  return filas
}

const numeroCSV = (v) => Number(String(v || '0').trim().replace(/\./g, (m, o, s) => (s.includes(',') ? '' : m)).replace(',', '.')) || 0
const boolCSV = (v) => /^(1|si|sí|true|verdadero|x)$/i.test(String(v || '').trim())

// filasDeCSV mapea el CSV a las filas que espera el backend, tolerando el orden y
// mayúsculas del encabezado. Requiere al menos las columnas sku y nombre.
function filasDeCSV(texto) {
  const raw = parseCSV(texto)
  if (raw.length < 2) return { filas: [], error: 'El archivo no tiene filas de datos (sólo el encabezado o está vacío).' }
  const head = raw[0].map((h) => h.trim().toLowerCase().replace(/\s+/g, ''))
  const idx = (name) => head.indexOf(name)
  if (idx('sku') < 0 || idx('nombre') < 0) return { filas: [], error: 'El encabezado debe incluir al menos las columnas "sku" y "nombre".' }
  const get = (row, name) => { const j = idx(name); return j >= 0 ? String(row[j] ?? '').trim() : '' }
  const filas = raw.slice(1).map((row) => ({
    sku: get(row, 'sku'), nombre: get(row, 'nombre'), rubro: get(row, 'rubro'),
    unidad: get(row, 'unidad'), tipoVenta: get(row, 'tipoventa'),
    precio: numeroCSV(get(row, 'precio')), moneda: get(row, 'moneda'),
    exentoIva: boolCSV(get(row, 'exentoiva')), codigoBarras: get(row, 'codigobarras'),
    existenciaInicial: numeroCSV(get(row, 'existenciainicial')),
  }))
  return { filas, error: '' }
}

function descargarPlantillaProductos() {
  const headers = ['sku', 'nombre', 'rubro', 'unidad', 'tipoVenta', 'precio', 'moneda', 'exentoIva', 'codigoBarras', 'existenciaInicial']
  const ejemplos = [
    ['TOR-034', 'Tornillo 3/4" (100u)', 'Ferretería', 'caja', 'unidad', '5.50', 'VES', 'no', '', '30'],
    ['CEM-42', 'Cemento gris 42,5 kg', 'Construcción', 'kg', 'unidad', '120', 'VES', 'no', '7501234567890', '40'],
  ]
  const enc = (c) => (/[",;\n]/.test(c) ? '"' + c.replace(/"/g, '""') + '"' : c)
  const csv = [headers, ...ejemplos].map((r) => r.map(enc).join(',')).join('\r\n')
  const blob = new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url; a.download = 'plantilla-productos.csv'
  document.body.appendChild(a); a.click(); document.body.removeChild(a); URL.revokeObjectURL(url)
}

const ESTADO_IMPORT = {
  nuevo: { color: 'teal', label: 'Nuevo' },
  actualiza: { color: 'amber', label: 'Actualiza' },
  error: { color: 'red', label: 'Error' },
}

function ModalCargaMasiva({ unidades, onClose, onSaved, toast }) {
  const [filas, setFilas] = useState(null)
  const [nombreArch, setNombreArch] = useState('')
  const [parseErr, setParseErr] = useState('')
  const [preview, setPreview] = useState(null)
  const [confirmado, setConfirmado] = useState(false)
  const [busy, setBusy] = useState(false)
  const fileRef = useRef(null)

  const simbolos = (unidades || []).map((u) => u.simbolo).filter(Boolean)

  const onFile = async (e) => {
    const f = e.target.files?.[0]
    if (!f) return
    setNombreArch(f.name); setPreview(null); setConfirmado(false); setParseErr('')
    try {
      const { filas: fs, error } = filasDeCSV(await f.text())
      if (error) { setParseErr(error); setFilas(null); return }
      setFilas(fs)
    } catch {
      setParseErr('No se pudo leer el archivo.'); setFilas(null)
    }
  }

  const correr = async (confirmar) => {
    if (!filas?.length) return
    setBusy(true)
    try {
      const res = await api.importarProductos({ confirmar, filas })
      if (confirmar && res.aplicado) {
        toast({ title: 'Carga aplicada', body: `${res.nuevos} creado(s), ${res.actualizados} actualizado(s).` })
        await onSaved(); onClose(); return
      }
      setPreview(res); setConfirmado(false)
      if (confirmar && !res.aplicado) {
        toast({ title: 'No se aplicó la carga', body: `Hay ${res.errores} fila(s) con error. Corrige el archivo y vuelve a intentar.`, kind: 'error' })
      }
    } catch (err) {
      toast({ title: 'No se pudo procesar', body: err?.message || 'Error', kind: 'error' })
    }
    setBusy(false)
  }

  const hayErrores = preview && preview.errores > 0
  const faltaConfirmar = preview && preview.requiereConfirmacion && !confirmado
  const puedeAplicar = preview && !hayErrores

  return (
    <Modal open onClose={onClose} size="lg" icon={<Icon.Upload size={18} />}
      title="Carga masiva de productos" sub="Sube un CSV para crear o actualizar productos en lote"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        {!preview ? (
          <Button onClick={() => correr(false)} loading={busy} disabled={!filas?.length} icon={<Icon.Check size={16} />}>Validar archivo</Button>
        ) : (
          <Button variant="dinero" onClick={() => correr(true)} loading={busy} disabled={!puedeAplicar || faltaConfirmar}
            title={hayErrores ? 'Corrige las filas con error primero' : (faltaConfirmar ? 'Confirma la actualización de los productos existentes' : 'Aplicar la carga')}
            icon={<Icon.Check size={16} />}>Aplicar carga{preview ? ` (${preview.nuevos + preview.actualizados})` : ''}</Button>
        )}
      </>}>
      <div className="space-y-4">
        {/* Paso 1: plantilla + archivo */}
        <div className="flex items-start gap-2 text-[12.5px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>Es <strong>UPSERT</strong>: un SKU que ya existe se <strong>actualiza</strong> (te pediremos confirmarlo). La <strong>unidad</strong> debe existir en el maestro. La <strong>existencia inicial</strong> sólo se carga en productos nuevos, como ajuste auditado.</span>
        </div>

        <div className="flex items-center gap-2 flex-wrap">
          <Button variant="secondary" size="sm" icon={<Icon.Download size={15} />} onClick={descargarPlantillaProductos}>Descargar plantilla CSV</Button>
          <input ref={fileRef} type="file" accept=".csv,text/csv" className="hidden" onChange={onFile} />
          <Button variant="secondary" size="sm" icon={<Icon.Upload size={15} />} onClick={() => fileRef.current?.click()}>
            {nombreArch ? 'Cambiar archivo' : 'Seleccionar CSV'}
          </Button>
          {nombreArch ? <span className="text-[12px] text-slate-500 num truncate">{nombreArch}</span> : null}
        </div>

        {simbolos.length ? (
          <div className="text-[11.5px] text-slate-400">Unidades válidas del maestro: <span className="num text-slate-500">{simbolos.join(', ')}</span></div>
        ) : null}

        {parseErr ? (
          <div className="flex items-start gap-1.5 text-[12px] text-red-600 dark:text-red-400"><Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{parseErr}</span></div>
        ) : null}

        {filas && !preview ? (
          <div className="text-[12.5px] text-slate-600 dark:text-slate-300">{fmtNum(filas.length, 0)} fila(s) leídas. Pulsa <strong>Validar archivo</strong> para revisarlas antes de aplicar.</div>
        ) : null}

        {/* Paso 2: vista previa */}
        {preview ? (
          <div className="space-y-3">
            <div className="flex items-center gap-2 flex-wrap text-[12px]">
              <Badge color="teal" dot>{preview.nuevos} nuevo(s)</Badge>
              <Badge color="amber" dot>{preview.actualizados} a actualizar</Badge>
              {preview.errores > 0 ? <Badge color="red" dot>{preview.errores} con error</Badge> : <Badge color="emerald" dot>sin errores</Badge>}
            </div>

            <div className="rounded-lg border border-slate-200 dark:border-slate-700 overflow-x-auto max-h-72 overflow-y-auto">
              <table className="w-full text-[12.5px]">
                <thead className="sticky top-0">
                  <tr className="text-left text-[10.5px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/80">
                    <th className="py-2 px-2.5 font-medium w-12">#</th>
                    <th className="py-2 px-2 font-medium">SKU</th>
                    <th className="py-2 px-2 font-medium">Nombre</th>
                    <th className="py-2 px-2 font-medium w-24">Estado</th>
                    <th className="py-2 px-2 font-medium">Detalle</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.filas.map((f) => {
                    const est = ESTADO_IMPORT[f.estado] || ESTADO_IMPORT.error
                    return (
                      <tr key={f.indice} className="border-b border-slate-100 dark:border-slate-800/70">
                        <td className="py-1.5 px-2.5 num text-slate-400">{f.indice + 1}</td>
                        <td className="py-1.5 px-2 num">{f.sku || <span className="text-slate-300">—</span>}</td>
                        <td className="py-1.5 px-2 truncate max-w-[200px]">{f.nombre || <span className="text-slate-300">—</span>}</td>
                        <td className="py-1.5 px-2"><Badge size="sm" color={est.color}>{est.label}</Badge></td>
                        <td className={`py-1.5 px-2 text-[11.5px] ${f.estado === 'error' ? 'text-red-600 dark:text-red-400' : 'text-slate-400'}`}>{f.mensaje}</td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {hayErrores ? (
              <div className="flex items-start gap-1.5 text-[12px] text-red-600 dark:text-red-400"><Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>Hay filas con error. La carga es todo-o-nada: corrige el archivo y vuelve a subirlo (no se aplicará nada mientras haya errores).</span></div>
            ) : preview.requiereConfirmacion ? (
              <label className="flex items-start gap-2 text-[12.5px] text-slate-600 dark:text-slate-300 bg-amber-50/60 dark:bg-amber-900/15 border border-amber-200 dark:border-amber-800/60 rounded-lg px-3 py-2.5 cursor-pointer">
                <input type="checkbox" checked={confirmado} onChange={(e) => setConfirmado(e.target.checked)} className="accent-amber-600 w-4 h-4 mt-0.5" />
                <span>Revisé los <strong>{preview.actualizados}</strong> producto(s) existentes y confirmo <strong>actualizarlos</strong> con los datos del archivo.</span>
              </label>
            ) : (
              <div className="flex items-start gap-1.5 text-[12px] text-emerald-700 dark:text-emerald-400"><Icon.Check size={14} className="mt-0.5 shrink-0" /><span>Todo listo. Pulsa <strong>Aplicar carga</strong> para crear los productos.</span></div>
            )}
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
