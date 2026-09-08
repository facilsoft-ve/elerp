import { useState, useMemo, useRef, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Input, Select, Toggle, Empty, useToast, Field, Modal } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, TIPOS_DOCUMENTO, validarRIF } from '../lib/format.js'
import { calcularTotales, IVA_TASA } from '../lib/fiscal.js'
import { useData, useTasa } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { useSesionCaja, AbrirCajaModal, SinCajaAbierta, BarraTurno } from './Caja.jsx'
import { RejillaProductos, DisponibilidadModal } from '../components/producto.jsx'
import { TasaModal, fechaCortaVE } from '../components/tasa.jsx'
import { precioEnBs, monedaDe, porCodigo } from '../lib/precio.js'
import { CobroModal, VentaEmitida } from './Cobro.jsx'
import { useEspera, DejarEnEsperaModal, EsperaModal } from './Espera.jsx'

export const puedeEmitir = (rol) => rol !== 'contadora'
export const puedeCrearCliente = (rol) => ['dueno', 'desarrollador', 'vendedor', 'cajero'].includes(rol)

/* Punto de venta.
 *
 * Izquierda: búsqueda (entrada de la pistola lectora) + catálogo visual +
 * carrito. Derecha: cliente, totales y un solo botón «Cobrar», que abre el
 * modal de cobro — ahí viven los métodos de pago, el cobro mixto y el vuelto.
 *
 * Optimizado para el teclado: agregar un ítem repetido se hace sin mouse. Los
 * montos definitivos los calcula el servidor; acá solo se previsualizan con las
 * mismas reglas.
 */
export function POS({ onModoCaja }) {
  const { db, reload, tasaDe } = useData()
  // La tasa la pone el servidor (R9): el POS la lee, nunca la teclea. `tasa` es
  // el dólar (para IGTF y cobros en US$); `tasaDe` resuelve cada divisa para
  // convertir precios de productos en cualquier moneda a Bs.
  const tasa = useTasa()
  const [verTasa, setVerTasa] = useState(false)
  const { ui } = useUI()
  const toast = useToast()
  const productos = db.PRODUCTOS || []

  const puede = puedeEmitir(ui.rol)

  // Turno de caja: sin él el backend rechaza la emisión, así que el POS ni se
  // renderiza (03 §4.4). La sesión se recarga tras abrir o cerrar.
  const { sesion, cargando: cargandoSesion, recargar: recargarSesion } = useSesionCaja()
  const [abrirCaja, setAbrirCaja] = useState(false)

  // Dos caminos al mismo producto (R12): el buscador —que es la entrada de la
  // pistola lectora de código de barras— y el catálogo visual con tarjetas.
  const [catalogo, setCatalogo] = useState(false)
  const [infoSku, setInfoSku] = useState('')

  // Moneda principal de la empresa: define en qué moneda se leen los precios
  // del catálogo que no declaran la suya (R10).
  const monedaEmpresa = db.EMPRESA?.monedaPrincipal || 'VES'

  const existencias = db.EXISTENCIAS || []
  const existenciaDe = (p) => {
    const e = existencias.find((x) => x.sku === p.sku)
    return e ? e.cantidad : 0
  }

  // Carrito: [{ sku, nombre, cantidad, precioUnitario (Bs), exento }]
  const [cart, setCart] = useState([])
  const [q, setQ] = useState('')
  const [sel, setSel] = useState(0)
  const searchRef = useRef(null)

  const [clienteId, setClienteId] = useState('') // '' = consumidor final
  const [nuevoCliente, setNuevoCliente] = useState(false)
  const [contingencia, setContingencia] = useState(false)

  const [cobrando, setCobrando] = useState(false)
  const [emitida, setEmitida] = useState(null) // documento emitido (estado de éxito)

  // Ventas en espera (2.6): el carrito apartado del mostrador.
  const espera = useEspera()
  const [dejando, setDejando] = useState(false)
  const [verEspera, setVerEspera] = useState(false)

  // Retomar una venta apartada carga su carrito tal como se armó.
  const retomar = (v) => {
    setCart((v.lineas || []).map((l) => ({
      sku: l.sku, nombre: l.nombre, cantidad: l.cantidad,
      precioUnitario: l.precioUnitario, exento: !!l.exento,
    })))
    if (v.clienteId) setClienteId(v.clienteId)
    espera.recargar()
    toast({ title: 'Venta retomada', body: v.nota || `${v.lineas?.length || 0} ítem(s)` })
  }

  const resultados = useMemo(() => {
    const term = q.trim().toLowerCase()
    if (!term) return productos.slice(0, 8)
    return productos
      .filter((p) => p.nombre.toLowerCase().includes(term)
        || (p.sku || '').toLowerCase().includes(term)
        || (p.codigoBarras || '').includes(term)
        || (p.presentaciones || []).some((pr) => (pr.codigoBarras || '').includes(term)))
      .slice(0, 8)
  }, [q, productos])

  /* Enter en el buscador: primero se prueba como CÓDIGO EXACTO (lo que manda la
   * pistola lectora, que termina con un Enter), y solo si no hay coincidencia se
   * toma el resultado resaltado de la lista. Así escanear nunca agrega el
   * producto equivocado por estar seleccionado otro. */
  const agregarDesdeBuscador = () => {
    const exacto = porCodigo(productos, q)
    if (exacto) { addProducto(exacto); return }
    addProducto(resultados[sel])
  }

  useEffect(() => { setSel(0) }, [q])

  const addProducto = (p) => {
    if (!p) return
    // El carrito trabaja en bolívares: un precio en US$ se convierte con la tasa
    // vigente, igual que hará el servidor al emitir.
    const bs = precioEnBs(p, monedaEmpresa, tasaDe)
    if (bs === null) {
      toast({
        title: 'Falta la tasa de cambio',
        body: `${p.nombre} tiene su precio en ${monedaDe(p, monedaEmpresa)} y no hay tasa cargada para convertirlo.`,
        kind: 'warn',
      })
      return
    }
    setCart((c) => {
      const i = c.findIndex((l) => l.sku === p.sku)
      if (i >= 0) return c.map((l, j) => (j === i ? { ...l, cantidad: l.cantidad + 1 } : l))
      return [...c, {
        sku: p.sku, nombre: p.nombre, cantidad: 1, precioUnitario: bs,
        // Se arrastra la exención para que el preview de IVA sea el del servidor.
        exento: !!p.exentoIva, monedaOriginal: monedaDe(p, monedaEmpresa),
      }]
    })
    setQ('')
    searchRef.current?.focus()
  }

  const setLinea = (sku, k, v) => setCart((c) => c.map((l) => (l.sku === sku ? { ...l, [k]: v } : l)))
  const rmLinea = (sku) => setCart((c) => c.filter((l) => l.sku !== sku))

  const onSearchKey = (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setSel((s) => Math.min(s + 1, resultados.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)) }
    else if (e.key === 'Enter') { e.preventDefault(); agregarDesdeBuscador() }
  }

  // Totales del documento (sin pagos todavía: el IGTF se calcula en el cobro).
  const totales = useMemo(() => calcularTotales(cart, [], tasa.valor), [cart, tasa.valor])

  const errCarrito = cart.length === 0 ? 'Agrega al menos un producto.' : ''
  const errLinea = cart.some((l) => !(Number(l.cantidad) > 0)) ? 'Todas las cantidades deben ser mayores a 0.' : ''

  const clienteNombre = (db.CLIENTES || []).find((c) => c.id === clienteId)?.nombre || 'Consumidor final'

  const nuevaVenta = () => {
    setEmitida(null)
    setCart([]); setQ(''); setClienteId('')
    setContingencia(false)
    searchRef.current?.focus()
  }

  if (emitida) return <VentaEmitida doc={emitida} onNueva={nuevaVenta} />

  // Sin turno abierto: en lugar del punto de venta va el estado que lo explica.
  if (!cargandoSesion && !sesion) {
    return (
      <>
        <SinCajaAbierta puedeAbrir={puede} onAbrir={() => setAbrirCaja(true)} />
        <AbrirCajaModal open={abrirCaja} onClose={() => setAbrirCaja(false)} onAbierta={recargarSesion} />
      </>
    )
  }

  return (
    <div className="space-y-3">
      <BarraTurno sesion={sesion} onCerrada={recargarSesion} onModoCaja={onModoCaja} />
      <DisponibilidadModal sku={infoSku} open={!!infoSku} onClose={() => setInfoSku('')} />
      <TasaModal open={verTasa} onClose={() => setVerTasa(false)} />
      <CobroModal open={cobrando} onClose={() => setCobrando(false)}
        lineas={cart} clienteId={clienteId} clienteNombre={clienteNombre} contingencia={contingencia}
        onEmitida={async (doc) => {
          setCobrando(false)
          setEmitida(doc)
          toast({ title: 'Factura emitida', body: `${doc.numeroCompleto} · ${fmtCurrency(doc.total, 'VES')}` })
          // Refresca existencias y documentos. Ya no desmonta la pantalla: el
          // DataContext distingue la primera carga de un refresco.
          await reload()
        }} />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {/* Izquierda: búsqueda + carrito */}
        <div className="lg:col-span-2 space-y-3">
          <div className="flex items-center gap-2">
            <div className="relative flex-1">
              <Input ref={searchRef} icon={<Icon.Search size={16} />} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onSearchKey}
                placeholder="Buscar o escanear código… (Enter para agregar)" autoFocus />
              {q ? (
                <div className="absolute z-20 mt-1 w-full bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-modal overflow-hidden">
                  {resultados.length === 0 ? (
                    <div className="px-4 py-6 text-center text-[13px] text-slate-500">Sin resultados para “{q}”.</div>
                  ) : resultados.map((p, i) => (
                    <button key={p.id} onClick={() => addProducto(p)} onMouseEnter={() => setSel(i)}
                      className={`w-full flex items-center gap-3 px-3 h-11 text-left ${i === sel ? 'bg-elerp-50 dark:bg-elerp-900/40' : ''}`}>
                      <Icon.Package size={16} className="text-slate-400" />
                      <div className="flex-1 min-w-0">
                        <div className="text-[13px] font-medium truncate">{p.nombre}</div>
                        <div className="text-[11px] text-slate-400 num">
                          {p.sku}
                          {p.codigoBarras ? ` · ${p.codigoBarras}` : ''}
                          {p.exentoIva ? ' · exento' : ''}
                        </div>
                      </div>
                      <div className="num text-[12.5px] font-medium">
                        {fmtCurrency(p.precio, monedaDe(p, monedaEmpresa))}
                      </div>
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
            <Button variant={catalogo ? 'primary' : 'secondary'} size="lg"
              icon={<Icon.Boxes size={17} />} onClick={() => setCatalogo((c) => !c)}
              title="Ver el catálogo con imágenes">Catálogo</Button>
            {espera.lista.length ? (
              <Button variant="secondary" size="lg" icon={<Icon.Clock size={17} />}
                onClick={() => setVerEspera(true)} title="Ventas apartadas de esta sede">
                En espera · {espera.lista.length}
              </Button>
            ) : null}
          </div>

          {catalogo ? (
            <RejillaProductos productos={productos} existenciaDe={existenciaDe}
              onAgregar={addProducto} onInfo={(p) => setInfoSku(p.sku)}
              ccy={ui.ccy} tasa={tasaDe} monedaEmpresa={monedaEmpresa} columnas={4} />
          ) : null}

          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
            {cart.length === 0 ? (
              <Empty icon={<Icon.Cart size={22} />} title="Carrito vacío"
                body="Busca un producto arriba y presiona Enter para agregarlo. Repetir el mismo ítem incrementa su cantidad." />
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                      <th className="py-2.5 px-3 font-medium">Producto</th>
                      <th className="py-2.5 pr-3 font-medium text-center w-28">Cantidad</th>
                      <th className="py-2.5 pr-3 font-medium text-right w-28">Precio</th>
                      <th className="py-2.5 pr-3 font-medium text-right w-28">Total</th>
                      <th className="py-2.5 pr-3 w-10"></th>
                    </tr>
                  </thead>
                  <tbody>
                    {cart.map((l) => (
                      <tr key={l.sku} className="border-b border-slate-100 dark:border-slate-800/70">
                        <td className="py-2 px-3">
                          <div className="font-medium text-[13px]">{l.nombre}</div>
                          <div className="text-[11px] text-slate-400 num">
                            {l.sku}
                            {l.exento ? <span className="ml-1.5 text-slate-500">exento de IVA</span> : null}
                            {l.monedaOriginal === 'USD' ? <span className="ml-1.5 text-slate-500">precio en US$</span> : null}
                          </div>
                        </td>
                        <td className="py-2 pr-3">
                          <div className="flex items-center justify-center gap-1">
                            <button onClick={() => setLinea(l.sku, 'cantidad', Math.max(1, l.cantidad - 1))} className="h-7 w-7 rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 inline-flex items-center justify-center"><Icon.ChevDown size={14} /></button>
                            <input type="number" min="0" step="1" value={l.cantidad} onChange={(e) => setLinea(l.sku, 'cantidad', Number(e.target.value))}
                              className="w-14 h-8 text-center rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num ring-focus" />
                            <button onClick={() => setLinea(l.sku, 'cantidad', l.cantidad + 1)} className="h-7 w-7 rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 inline-flex items-center justify-center"><Icon.ChevUp size={14} /></button>
                          </div>
                        </td>
                        <td className="py-2 pr-3 text-right">
                          <input type="number" min="0" step="0.01" value={l.precioUnitario} onChange={(e) => setLinea(l.sku, 'precioUnitario', Number(e.target.value))}
                            className="w-24 h-8 text-right px-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm num ring-focus private-mask" />
                        </td>
                        <td className="py-2 pr-3 text-right num font-medium private-mask">{fmtCurrency((Number(l.cantidad) || 0) * (Number(l.precioUnitario) || 0), 'VES')}</td>
                        <td className="py-2 pr-3 text-right">
                          <button onClick={() => rmLinea(l.sku)} className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800"><Icon.Trash size={15} /></button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
          {cart.length ? <div className="text-[11.5px] text-slate-400">{fmtNum(cart.length, 0)} línea(s) · {fmtNum(cart.reduce((a, l) => a + (Number(l.cantidad) || 0), 0), 0)} unidad(es)</div> : null}
        </div>

        {/* Derecha: cliente, totales y el botón de cobro */}
        <div className="space-y-3">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
            <div className="flex items-center justify-between mb-2">
              <div className="text-[12px] font-semibold text-slate-600 dark:text-slate-300 inline-flex items-center gap-1.5"><Icon.User size={14} /> Cliente</div>
              {puedeCrearCliente(ui.rol) ? <button onClick={() => setNuevoCliente(true)} className="text-[12px] font-medium text-elerp-600 hover:text-elerp-700 inline-flex items-center gap-1"><Icon.Plus size={13} /> Nuevo</button> : null}
            </div>
            <Select value={clienteId} onChange={(e) => setClienteId(e.target.value)}>
              <option value="">Consumidor final</option>
              {(db.CLIENTES || []).map((c) => (
                <option key={c.id} value={c.id}>{c.nombre}{c.documento ? ` · ${c.tipoDocumento}-${c.documento}` : ''}</option>
              ))}
            </Select>
          </div>

          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3">
            <div className="space-y-1.5 text-[13px]">
              <Row label="Base imponible" value={totales.baseImponible} />
              {totales.baseExenta > 0 ? <Row label="Exento de IVA" value={totales.baseExenta} /> : null}
              <Row label={`IVA ${IVA_TASA * 100}%`} value={totales.iva} />
              <div className="border-t border-slate-100 dark:border-slate-800 pt-2 mt-1 flex items-center justify-between">
                <span className="font-semibold">Total</span>
                <span className="num font-semibold text-[17px] private-mask">{fmtCurrency(totales.total, 'VES')}</span>
              </div>
              <div className="text-[11px] text-slate-400">El IGTF del 3% se calcula en el cobro, sobre lo que se pague en divisas.</div>
            </div>

            <Toggle checked={contingencia} onChange={setContingencia} label="Modo contingencia (sin conexión)" sub="Reserva serie de contingencia; no bloquea la venta." />

            {/* Rótulo de la tasa: nunca un campo editable (R9). */}
            <button type="button" onClick={() => setVerTasa(true)}
              className="w-full text-left rounded-lg border border-slate-200 dark:border-slate-700 px-3 py-2 hover:bg-slate-50 dark:hover:bg-slate-800/60 ring-focus">
              {tasa.hay ? (
                <>
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-[12px] text-slate-500">Tasa del día</span>
                    <span className="num text-[13px] font-medium">Bs {fmtNum(tasa.valor, 2)} / US$</span>
                  </div>
                  <div className="text-[11px] text-slate-400 mt-0.5">
                    {tasa.fuenteLabel} · {fechaCortaVE(tasa.fechaValor, tasa.esDeHoy)}
                  </div>
                </>
              ) : (
                <>
                  <div className="text-[12px] font-medium text-amber-700 dark:text-amber-400">Sin tasa de cambio</div>
                  <div className="text-[11px] text-slate-400 mt-0.5">Se puede cobrar en bolívares; no en divisas.</div>
                </>
              )}
            </button>

            {puede && cart.length ? (
              <Button className="w-full justify-center" size="sm" variant="secondary"
                icon={<Icon.Clock size={15} />} onClick={() => setDejando(true)}
                title="Apartar este carrito y seguir atendiendo">Dejar en espera</Button>
            ) : null}

            {puede ? (
              <Button className="w-full justify-center" size="lg" variant="dinero"
                disabled={!!errCarrito || !!errLinea} title={errCarrito || errLinea || 'Elegir método de pago y cobrar'}
                onClick={() => setCobrando(true)} icon={<Icon.Banknote size={17} />}>
                Cobrar {fmtCurrency(totales.total, 'VES')}
              </Button>
            ) : (
              <div className="text-center text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg py-2.5 px-3">Tu rol (contadora) es de solo lectura en el POS.</div>
            )}
            {errCarrito || errLinea ? (
              <div className="flex items-start gap-1.5 text-[11.5px] text-slate-500">
                <Icon.CircleAlert size={13} className="mt-0.5 shrink-0" /><span>{errCarrito || errLinea}</span>
              </div>
            ) : null}
          </div>
        </div>

        <DejarEnEsperaModal open={dejando} onClose={() => setDejando(false)} lineas={cart} clienteId={clienteId}
          onGuardada={() => { setCart([]); setClienteId(''); espera.recargar() }} />
        <EsperaModal open={verEspera} onClose={() => setVerEspera(false)} lista={espera.lista}
          onRetomada={retomar} onCambio={espera.recargar} />

        {nuevoCliente ? <NuevoClienteModal onClose={() => setNuevoCliente(false)} onSaved={async (c) => { await reload(); if (c?.id) setClienteId(c.id) }} toast={toast} /> : null}
      </div>
    </div>
  )
}

const Row = ({ label, value, muted }) => (
  <div className="flex items-center justify-between">
    <span className={muted ? 'text-slate-400' : 'text-slate-500'}>{label}</span>
    <span className={`num private-mask ${muted ? 'text-slate-400' : ''}`}>{fmtCurrency(value, 'VES')}</span>
  </div>
)

// Alta rápida de cliente desde el POS (reusa la misma validación que Clientes).
export function NuevoClienteModal({ onClose, onSaved, toast }) {
  const [f, setF] = useState({ nombre: '', tipoDocumento: 'V', documento: '', telefono: '', direccion: '', email: '' })
  const [touched, setTouched] = useState({})
  const [busy, setBusy] = useState(false)

  // Pasaporte (P) es texto libre; V/E/J/G validan como RIF/cédula.
  const rifCheck = f.tipoDocumento === 'P' ? { valid: true, msg: '' } : validarRIF(`${f.tipoDocumento}${f.documento}`)
  const emailOk = !f.email.trim() || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(f.email.trim())
  const errs = {
    nombre: !f.nombre.trim() ? 'Ingresa el nombre.' : '',
    documento: !f.documento.trim() ? 'Ingresa el documento.' : !rifCheck.valid ? rifCheck.msg : '',
    email: emailOk ? '' : 'El correo no tiene un formato válido.',
  }
  const valid = !errs.nombre && !errs.documento && !errs.email
  const set = (k) => (e) => setF((s) => ({ ...s, [k]: e.target.value }))

  const save = async () => {
    setTouched({ nombre: true, documento: true })
    if (!valid) return
    setBusy(true)
    try {
      const c = await api.crearCliente({ nombre: f.nombre.trim(), tipoDocumento: f.tipoDocumento, documento: f.documento.trim(), telefono: f.telefono.trim(), direccion: f.direccion.trim(), email: f.email.trim() })
      toast({ title: 'Cliente creado', body: f.nombre.trim() })
      await onSaved(c)
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo crear', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.User size={18} />} title="Nuevo cliente" sub="El documento es único por empresa."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={save} loading={busy} icon={<Icon.Check size={16} />}>Crear</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre / Razón social" required error={touched.nombre ? errs.nombre : ''}>
          <Input value={f.nombre} onChange={set('nombre')} onBlur={() => setTouched((t) => ({ ...t, nombre: true }))} invalid={touched.nombre && !!errs.nombre} autoFocus />
        </Field>
        <div className="grid grid-cols-[80px_1fr] gap-2">
          <Field label="Tipo">
            <Select value={f.tipoDocumento} onChange={set('tipoDocumento')}>
              {TIPOS_DOCUMENTO.map((t) => <option key={t} value={t}>{t}</option>)}
            </Select>
          </Field>
          <Field label="Documento" required error={touched.documento ? errs.documento : ''}>
            <Input value={f.documento} onChange={set('documento')} onBlur={() => setTouched((t) => ({ ...t, documento: true }))} invalid={touched.documento && !!errs.documento} placeholder="12345678" />
          </Field>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Teléfono" hint="opcional"><Input value={f.telefono} onChange={set('telefono')} placeholder="0412-…" /></Field>
          <Field label="Correo" hint="opcional" error={touched.email ? errs.email : ''}>
            <Input value={f.email} onChange={set('email')} onBlur={() => setTouched((t) => ({ ...t, email: true }))} invalid={touched.email && !!errs.email} type="email" placeholder="correo@dominio.com" />
          </Field>
        </div>
        <Field label="Dirección" hint="opcional"><Input value={f.direccion} onChange={set('direccion')} /></Field>
      </div>
    </Modal>
  )
}
