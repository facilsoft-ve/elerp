import { useState, useEffect, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, Field, Modal, Empty, useToast, TableSkeleton, Toggle, Segmented } from '../components/primitives.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { fmtCurrency } from '../lib/format.js'
import {
  ESTADOS, etiquetaEstado, colorEstado, accionesDe, etiquetaOrigen,
  agruparBandeja, minutosRestantes, estaDemorado,
} from '../lib/pedidos.js'

/* PEDIDOS PARA LLEVAR.
 *
 * LA BANDEJA ES UNA PANTALLA DE TRABAJO, no un listado. Se parece al tablero de
 * comandas a propósito: quien atiende pedidos hace lo mismo todo el día —mirar
 * qué entró, decidir, empujar al siguiente paso— y para eso necesita ver el
 * estado de todo de un vistazo y actuar sin abrir nada.
 *
 * Los TRES ORÍGENES caen en la misma lista. Un pedido del mostrador, uno de la
 * tienda web y uno de una app se atienden igual; lo único que cambia es de dónde
 * vino, y eso se marca en la ficha. Separarlos en tres pantallas obligaría a
 * mirar tres sitios en hora pico, que es cuando menos se puede.
 */
export function Pedidos({ route }) {
  const sub = (route || '').split(':')[1] || 'bandeja'
  return (
    <div>
      {sub === 'bandeja' ? <Bandeja /> : null}
      {sub === 'despacho' ? <Despacho /> : null}
      {sub === 'canales' ? <Canales /> : null}
      {sub === 'zonas' ? <Zonas /> : null}
      {sub === 'repartidores' ? <Repartidores /> : null}
    </div>
  )
}

/* --- Bandeja -------------------------------------------------------------- */

function Bandeja() {
  const toast = useToast()
  const { ui } = useUI()
  const [pedidos, setPedidos] = useState(null)
  const [error, setError] = useState('')
  const [abierto, setAbierto] = useState(null)   // pedido en ficha
  const [nuevo, setNuevo] = useState(false)
  const [filtro, setFiltro] = useState('activos')

  const cargar = useCallback(() => {
    api.pedidos()
      .then((r) => { setPedidos(r?.pedidos || []); setError('') })
      .catch((e) => { setError(e?.message || 'No se pudieron cargar los pedidos.'); setPedidos([]) })
  }, [])
  useEffect(() => { cargar() }, [cargar])

  /* REFRESCO AUTOMÁTICO. Los pedidos entran solos por API y la ventana de
   * aceptación corre en segundo plano: una bandeja que solo se actualiza al
   * recargar haría que un pedido venza mientras alguien lo mira. Cada 20
   * segundos alcanza y no pesa. */
  useEffect(() => {
    const t = setInterval(cargar, 20000)
    return () => clearInterval(t)
  }, [cargar])

  const grupos = useMemo(() => agruparBandeja(pedidos || []), [pedidos])
  const lista = filtro === 'activos' ? grupos.activos : filtro === 'nuevos' ? grupos.nuevos : grupos.cerrados

  const actuar = async (p, accion, body) => {
    try {
      await api.accionPedido(p.id, accion, body)
      cargar()
      setAbierto(null)
      toast({ title: 'Pedido actualizado', body: `#${p.numero}` })
    } catch (e) {
      toast({ title: 'No se pudo', body: e?.message || 'Error', kind: 'error' })
    }
  }

  if (pedidos === null) {
    return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={5} cols={4} /></div>
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3 flex-wrap">
        <div className="flex items-center gap-3 flex-wrap">
          <Segmented value={filtro} onChange={setFiltro} options={[
            { value: 'activos', label: `En curso (${grupos.activos.length})` },
            { value: 'nuevos', label: `Por revisar (${grupos.nuevos.length})` },
            { value: 'cerrados', label: 'Historial' },
          ]} />
          {grupos.demorados > 0 ? (
            <span className="inline-flex items-center gap-1.5 text-[12.5px] text-[#B3362C] dark:text-red-400 font-semibold">
              <Icon.CircleAlert size={15} /> {grupos.demorados} demorado{grupos.demorados > 1 ? 's' : ''}
            </span>
          ) : null}
        </div>
        <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setNuevo(true)}>Nuevo pedido</Button>
      </div>

      {error ? (
        <div className="mb-3 rounded-lg px-3 py-2.5 text-[13px]" style={{ background: '#FBEDEB', border: '1px solid #ECC8C4', color: '#B3362C' }}>{error}</div>
      ) : null}

      {lista.length === 0 ? (
        <Empty icon={<Icon.Truck size={22} />}
          title={filtro === 'nuevos' ? 'Nada por revisar' : filtro === 'activos' ? 'Ningún pedido en curso' : 'Sin pedidos cerrados'}
          body={filtro === 'cerrados'
            ? 'Acá aparecen los pedidos entregados, rechazados y cancelados.'
            : 'Los pedidos de tu tienda web y de las apps conectadas entran acá solos. También puedes cargar uno del mostrador.'}
          cta={filtro !== 'cerrados' ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNuevo(true)}>Nuevo pedido</Button> : null} />
      ) : (
        <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
          {lista.map((p) => (
            <TarjetaPedido key={p.id} p={p} ccy={ui.ccy} onAbrir={() => setAbierto(p)} onAccion={(a, b) => actuar(p, a, b)} />
          ))}
        </div>
      )}

      {abierto ? <FichaPedido p={abierto} ccy={ui.ccy} onClose={() => setAbierto(null)} onAccion={(a, b) => actuar(abierto, a, b)} /> : null}
      {nuevo ? <NuevoPedidoModal onClose={() => setNuevo(false)} onCreado={() => { setNuevo(false); cargar() }} toast={toast} /> : null}
    </div>
  )
}

/* TarjetaPedido: lo que hay que saber sin abrir nada — de dónde vino, a nombre
 * de quién, cuánto lleva esperando y qué sigue. */
function TarjetaPedido({ p, ccy, onAbrir, onAccion }) {
  const col = colorEstado(p.estado)
  const demorado = estaDemorado(p)
  const restan = minutosRestantes(p.venceAceptacion)
  const acciones = accionesDe(p)
  return (
    <div className={`bg-white dark:bg-slate-900 border rounded-xl shadow-card p-3.5 ${demorado
      ? 'border-[#B3362C]/60 dark:border-red-500/50' : 'border-slate-200 dark:border-slate-800'}`}>
      <div className="flex items-start justify-between gap-2">
        <button type="button" onClick={onAbrir} className="text-left min-w-0 flex-1">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-display font-bold text-[15px]">#{p.numero}</span>
            <span className="text-[11.5px] px-1.5 py-0.5 rounded font-semibold"
              style={{ background: col.bg, color: col.text }}>{etiquetaEstado(p.estado)}</span>
            {demorado ? <Badge size="sm" color="red" dot>Demorado</Badge> : null}
          </div>
          {/* DE DÓNDE VINO, con nombre propio: «Yummy», no «app». Cuando el
              courier pregunta por su número, es el de ellos. */}
          <div className="text-[12px] text-slate-500 mt-1 truncate">
            {p.canalNombre || etiquetaOrigen(p.origen)}
            {p.referenciaExterna ? <span className="mono"> · {p.referenciaExterna}</span> : null}
          </div>
          <div className="text-[13px] font-medium mt-1.5 truncate">{p.clienteNombre || 'Consumidor final'}</div>
          <div className="text-[12px] text-slate-500 truncate">{p.destino?.direccion}</div>
        </button>
        <div className="text-right shrink-0">
          <div className="num font-semibold text-[14px] private-mask">{fmtCurrency(p.total, ccy)}</div>
          {restan !== null ? (
            <div className={`text-[11.5px] num mt-1 ${restan <= 2 ? 'text-[#B3362C] font-semibold' : 'text-slate-500'}`}>
              {restan > 0 ? `${restan} min para aceptar` : 'ventana vencida'}
            </div>
          ) : null}
        </div>
      </div>

      {acciones.length > 0 ? (
        <div className="flex items-center gap-1.5 mt-3 flex-wrap">
          {acciones.map((a) => (
            <Button key={a.id} size="sm" variant={a.tono === 'peligro' ? 'destructive' : a.tono === 'suave' ? 'ghost' : 'primary'}
              onClick={() => (a.pideMotivo || a.pideDato ? onAbrir() : onAccion(a.id))}>
              {a.label}
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

/* FichaPedido: el detalle y la bitácora. La bitácora está acá y no escondida
 * porque es lo que responde «¿qué pasó con este pedido?», que es la pregunta
 * que llega por teléfono. */
function FichaPedido({ p, ccy, onClose, onAccion }) {
  const [motivo, setMotivo] = useState('')
  const [prueba, setPrueba] = useState('')
  const [repartidorId, setRepartidorId] = useState('')
  const [repartidores, setRepartidores] = useState([])
  const acciones = accionesDe(p)

  useEffect(() => {
    if (!acciones.some((a) => a.id === 'asignar')) return
    api.repartidores().then((r) => setRepartidores((r?.repartidores || []).filter((x) => x.activo))).catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [p.id])

  const ejecutar = (a) => {
    if (a.id === 'asignar') return onAccion('asignar', { repartidorId })
    if (a.id === 'entregado') return onAccion('entregado', { prueba })
    if (a.pideMotivo) return onAccion(a.id, { motivo })
    return onAccion(a.id)
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Truck size={18} />}
      title={`Pedido #${p.numero}`} sub={`${p.canalNombre || etiquetaOrigen(p.origen)}${p.referenciaExterna ? ` · ${p.referenciaExterna}` : ''}`}>
      <div className="space-y-3.5">
        <div className="grid grid-cols-2 gap-3 text-[12.5px]">
          <div>
            <div className="text-slate-400 text-[11px] uppercase tracking-wide">Cliente</div>
            <div className="font-medium">{p.clienteNombre || 'Consumidor final'}</div>
            {p.destino?.telefono ? <div className="num text-slate-500">{p.destino.telefono}</div> : null}
          </div>
          <div>
            <div className="text-slate-400 text-[11px] uppercase tracking-wide">Entrega</div>
            <div>{p.destino?.direccion}</div>
            {p.destino?.referencia ? <div className="text-slate-500">{p.destino.referencia}</div> : null}
            {p.destino?.zonaNombre ? <div className="text-slate-500">Zona: {p.destino.zonaNombre}</div> : null}
          </div>
        </div>

        <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
          {(p.items || []).map((it, i) => (
            <div key={i} className="flex items-center justify-between gap-3 px-3 py-2 text-[13px]">
              <span className="truncate">{it.cantidad} × {it.nombre || it.sku}{it.nota ? <span className="text-slate-400"> · {it.nota}</span> : null}</span>
              <span className="num private-mask">{fmtCurrency((it.precioUnitario || 0) * (it.cantidad || 0), ccy)}</span>
            </div>
          ))}
          <div className="flex items-center justify-between px-3 py-2 text-[13px] font-semibold">
            <span>Total{p.costoEnvio > 0 ? ' (con envío)' : ''}</span>
            <span className="num private-mask">{fmtCurrency((p.total || 0) + (p.costoEnvio || 0), ccy)}</span>
          </div>
        </div>

        {p.tracking ? (
          <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 px-3 py-2.5 text-[12.5px]">
            Nº de envío <span className="mono font-semibold">{p.tracking}</span>
            {p.repartidorNombre ? <> · lo lleva <strong>{p.repartidorNombre}</strong></> : null}
          </div>
        ) : null}

        {/* Bitácora: qué pasó y cuándo. Es lo que responde el reclamo. */}
        <div>
          <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Historial</div>
          <ul className="text-[12.5px] text-slate-500 space-y-1">
            {(p.bitacora || []).map((e, i) => (
              <li key={i} className="flex gap-2">
                <span className="num shrink-0">{(e.cuando || '').slice(11, 16)}</span>
                <span>{etiquetaEstado(e.estado)}{e.motivo ? ` — ${e.motivo}` : ''}</span>
              </li>
            ))}
          </ul>
        </div>

        {acciones.some((a) => a.pideMotivo) ? (
          <Field label="Motivo" hint="queda en el historial y se le informa al canal">
            <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} placeholder="Ej: fuera de zona, sin stock…" />
          </Field>
        ) : null}
        {acciones.some((a) => a.id === 'asignar') ? (
          <Field label="Repartidor">
            <Select value={repartidorId} onChange={(e) => setRepartidorId(e.target.value)}>
              <option value="">Elige un repartidor…</option>
              {repartidores.map((r) => <option key={r.id} value={r.id}>{r.nombre}{r.disponible ? '' : ' (ocupado)'}</option>)}
            </Select>
          </Field>
        ) : null}
        {acciones.some((a) => a.id === 'entregado') ? (
          <Field label="Prueba de entrega" hint="quién recibió, o una nota">
            <Input value={prueba} onChange={(e) => setPrueba(e.target.value)} placeholder="Ej: recibió la señora de la casa" />
          </Field>
        ) : null}

        <div className="flex items-center gap-2 flex-wrap pt-1">
          {acciones.map((a) => (
            <Button key={a.id} size="sm" variant={a.tono === 'peligro' ? 'destructive' : a.tono === 'suave' ? 'ghost' : 'primary'}
              disabled={(a.pideMotivo && !motivo.trim()) || (a.id === 'asignar' && !repartidorId)}
              onClick={() => ejecutar(a)}>
              {a.label}
            </Button>
          ))}
        </div>
      </div>
    </Modal>
  )
}

/* --- Nuevo pedido del mostrador ------------------------------------------ */

/* El pedido de mostrador nace CONFIRMADO: quien lo arma ya lo revisó al armarlo.
 * Por eso este formulario pide lo mínimo para poder despachar —a quién, a dónde,
 * qué— y no repite la revisión que la persona acaba de hacer con el cliente al
 * teléfono. */
function NuevoPedidoModal({ onClose, onCreado, toast }) {
  const [f, setF] = useState({ clienteNombre: '', telefono: '', direccion: '', referencia: '', formaPago: 'contra_entrega' })
  const [items, setItems] = useState([])
  const [busca, setBusca] = useState('')
  const [catalogo, setCatalogo] = useState([])
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))

  useEffect(() => { api.productos().then((r) => setCatalogo(r?.productos || r || [])).catch(() => {}) }, [])

  const resultados = useMemo(() => {
    const q = busca.trim().toLowerCase()
    if (!q) return []
    return (catalogo || []).filter((p) => (p.nombre || '').toLowerCase().includes(q) || (p.sku || '').toLowerCase().includes(q)).slice(0, 6)
  }, [busca, catalogo])

  const agregar = (p) => {
    setItems((xs) => {
      const i = xs.findIndex((x) => x.sku === p.sku)
      if (i >= 0) return xs.map((x, j) => (j === i ? { ...x, cantidad: x.cantidad + 1 } : x))
      return [...xs, { sku: p.sku, nombre: p.nombre, cantidad: 1, precioUnitario: p.precio || 0 }]
    })
    setBusca('')
  }

  const total = items.reduce((a, x) => a + x.cantidad * x.precioUnitario, 0)
  const puede = f.direccion.trim() !== '' && items.length > 0

  const crear = async () => {
    setBusy(true)
    try {
      await api.crearPedido({
        origen: 'manual', clienteNombre: f.clienteNombre, formaPago: f.formaPago, total,
        destino: { direccion: f.direccion, referencia: f.referencia, telefono: f.telefono },
        items,
      })
      toast({ title: 'Pedido creado' })
      onCreado()
    } catch (e) {
      toast({ title: 'No se pudo crear', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Plus size={18} />} title="Nuevo pedido" sub="Del mostrador o por teléfono"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={crear} loading={busy} disabled={!puede}>Crear pedido</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Cliente" hint="opcional"><Input value={f.clienteNombre} onChange={(e) => set('clienteNombre', e.target.value)} placeholder="Nombre de quien pide" /></Field>
          <Field label="Teléfono"><Input value={f.telefono} onChange={(e) => set('telefono', e.target.value)} placeholder="0414…" /></Field>
        </div>
        <Field label="Dirección de entrega" required>
          <Input value={f.direccion} onChange={(e) => set('direccion', e.target.value)} placeholder="Av. Principal, casa 4" />
        </Field>
        {/* La referencia no es un adorno: media Venezuela dicta su dirección por
            referencia y no por número de casa. */}
        <Field label="Punto de referencia" hint="al lado de…, frente a…">
          <Input value={f.referencia} onChange={(e) => set('referencia', e.target.value)} placeholder="Al lado de la panadería" />
        </Field>

        <Field label="Productos" required>
          <Input value={busca} onChange={(e) => setBusca(e.target.value)} placeholder="Busca por nombre o código…" />
        </Field>
        {resultados.length > 0 ? (
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
            {resultados.map((p) => (
              <button key={p.sku} type="button" onClick={() => agregar(p)}
                className="w-full text-left px-3 py-2 hover:bg-slate-50 dark:hover:bg-slate-800 flex items-center justify-between gap-3">
                <span className="text-[13px] truncate">{p.nombre}</span>
                <span className="num text-[12.5px] text-slate-500">{fmtCurrency(p.precio, 'VES')}</span>
              </button>
            ))}
          </div>
        ) : null}

        {items.length > 0 ? (
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 divide-y divide-slate-100 dark:divide-slate-800">
            {items.map((x) => (
              <div key={x.sku} className="flex items-center gap-3 px-3 py-2">
                <span className="flex-1 text-[13px] truncate">{x.nombre}</span>
                <Input type="number" min={1} step="1" className="w-16 text-center num" value={x.cantidad}
                  onChange={(e) => setItems((xs) => xs.map((y) => (y.sku === x.sku ? { ...y, cantidad: Number(e.target.value) || 1 } : y)))} />
                <span className="num text-[12.5px] w-24 text-right private-mask">{fmtCurrency(x.cantidad * x.precioUnitario, 'VES')}</span>
                <button type="button" onClick={() => setItems((xs) => xs.filter((y) => y.sku !== x.sku))}
                  className="text-slate-400 hover:text-slate-600"><Icon.Trash size={15} /></button>
              </div>
            ))}
            <div className="flex items-center justify-between px-3 py-2 font-semibold text-[13.5px]">
              <span>Total</span><span className="num private-mask">{fmtCurrency(total, 'VES')}</span>
            </div>
          </div>
        ) : null}

        <Field label="Cómo paga">
          <Select value={f.formaPago} onChange={(e) => set('formaPago', e.target.value)}>
            <option value="contra_entrega">Contra entrega (el repartidor cobra)</option>
            <option value="en_canal">Ya pagado</option>
          </Select>
        </Field>
      </div>
    </Modal>
  )
}

/* --- Despacho ------------------------------------------------------------- */

/* El tablero de despacho es la misma información que la bandeja, ordenada por lo
 * que despacho necesita: qué está listo esperando salir, qué ya tiene repartidor
 * y qué anda en la calle. Tres columnas porque son tres decisiones distintas. */
function Despacho() {
  const { ui } = useUI()
  const toast = useToast()
  const [pedidos, setPedidos] = useState(null)
  const cargar = useCallback(() => {
    api.pedidos().then((r) => setPedidos(r?.pedidos || [])).catch(() => setPedidos([]))
  }, [])
  useEffect(() => { cargar() }, [cargar])
  useEffect(() => { const t = setInterval(cargar, 20000); return () => clearInterval(t) }, [cargar])

  const cols = useMemo(() => ({
    listo: (pedidos || []).filter((p) => p.estado === 'listo'),
    asignado: (pedidos || []).filter((p) => p.estado === 'asignado'),
    ruta: (pedidos || []).filter((p) => p.estado === 'en_ruta' || p.estado === 'retirado_canal'),
  }), [pedidos])

  const actuar = async (p, accion, body) => {
    try { await api.accionPedido(p.id, accion, body); cargar() }
    catch (e) { toast({ title: 'No se pudo', body: e?.message || 'Error', kind: 'error' }) }
  }

  if (pedidos === null) return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={4} cols={3} /></div>

  const columna = (titulo, lista, vacio) => (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-3">
      <div className="text-[12px] font-semibold mb-2 flex items-center justify-between">
        <span>{titulo}</span><span className="num text-slate-400">{lista.length}</span>
      </div>
      {lista.length === 0 ? <div className="text-[12.5px] text-slate-400 py-6 text-center">{vacio}</div> : (
        <div className="space-y-2">
          {lista.map((p) => (
            <div key={p.id} className="rounded-lg border border-slate-200 dark:border-slate-700 p-2.5">
              <div className="flex items-center justify-between gap-2">
                <span className="font-semibold text-[13.5px]">#{p.numero}</span>
                <span className="num text-[12px] private-mask">{fmtCurrency(p.total, ui.ccy)}</span>
              </div>
              <div className="text-[12px] text-slate-500 truncate">{p.destino?.direccion}</div>
              {p.tracking ? <div className="mono text-[11px] text-slate-400">{p.tracking}</div> : null}
              {p.repartidorNombre ? <div className="text-[12px] mt-0.5">{p.repartidorNombre}</div> : null}
              <div className="flex gap-1.5 mt-2">
                {accionesDe(p).filter((a) => !a.pideMotivo && !a.pideDato).map((a) => (
                  <Button key={a.id} size="sm" variant="ghost" onClick={() => actuar(p, a.id)}>{a.label}</Button>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )

  return (
    <div>
      <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl mb-3">
        Lo que está esperando salir, lo que ya tiene repartidor y lo que anda en la calle. Se actualiza solo.
      </div>
      <div className="grid gap-3 md:grid-cols-3">
        {columna('Listos para despachar', cols.listo, 'Nada esperando salir.')}
        {columna('Asignados', cols.asignado, 'Sin pedidos asignados.')}
        {columna('En la calle', cols.ruta, 'Ningún pedido en ruta.')}
      </div>
    </div>
  )
}

/* --- Canales conectados --------------------------------------------------- */

/* DOS DIRECCIONES DISTINTAS, y es lo que decide la forma del formulario:
 *
 *   - TIENDA WEB (entrante): ellos nos llaman. Les damos una URL y un token para
 *     pegar en su tienda. Nosotros no sabemos nada de su sistema.
 *   - APP DE PEDIDOS (saliente): nos adaptamos a cada una, y cómo se conecta
 *     depende del acuerdo comercial con esa app. Por eso acá se declara el canal
 *     —para que sus pedidos entren identificados y con su propia regla— y las
 *     credenciales concretas se agregan cuando se sepa cómo conecta cada una.
 *
 * Mientras tanto el canal ya sirve: da nombre propio al origen, ventana de
 * aceptación, confirmación automática y la regla de quién reparte.
 */
function Canales() {
  const toast = useToast()
  const [canales, setCanales] = useState(null)
  const [edit, setEdit] = useState(null)

  const cargar = useCallback(() => {
    api.canalesPedido().then((r) => setCanales(r?.canales || [])).catch(() => setCanales([]))
  }, [])
  useEffect(() => { cargar() }, [cargar])

  if (canales === null) return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={3} cols={3} /></div>

  return (
    <div>
      <div className="flex items-start justify-between gap-3 mb-3 flex-wrap">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          Por dónde entran tus pedidos. Puedes tener <strong>varios a la vez</strong> —tu tienda web y las apps donde
          publicas— y cada uno con su propia regla: cuánto tiempo hay para aceptar, si se confirma solo y quién reparte.
        </div>
        <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setEdit({ origen: 'ecommerce', activo: true })}>Conectar canal</Button>
      </div>

      {canales.length === 0 ? (
        <Empty icon={<Icon.Globe size={22} />} title="Ningún canal conectado"
          body="Conecta tu tienda web para que sus pedidos entren solos, o declara las apps donde publicas tus productos."
          cta={<Button icon={<Icon.Plus size={16} />} onClick={() => setEdit({ origen: 'ecommerce', activo: true })}>Conectar canal</Button>} />
      ) : (
        <div className="space-y-2.5">
          {canales.map((c) => (
            <div key={c.id} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 flex items-start gap-3">
              <span className="text-slate-400 mt-0.5">{c.origen === 'ecommerce' ? <Icon.Globe size={18} /> : <Icon.Truck size={18} />}</span>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="text-[13.5px] font-semibold">{c.nombre}</span>
                  {c.activo ? <Badge size="sm" color="emerald" dot>Activo</Badge> : <Badge size="sm" color="slate">Inactivo</Badge>}
                  {c.envioPropioDelCanal ? <Badge size="sm" color="violet">Reparte la app</Badge> : null}
                </div>
                <div className="text-[12px] text-slate-500 mt-1">
                  {etiquetaOrigen(c.origen)}
                  {c.confirmacionAutomatica ? ' · confirma solo' : ' · requiere revisión'}
                  {c.minutosAceptacion > 0 ? ` · ${c.minutosAceptacion} min para aceptar` : ''}
                </div>
              </div>
              <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />} onClick={() => setEdit(c)}>Editar</Button>
            </div>
          ))}
        </div>
      )}

      {edit ? <CanalModal canal={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); cargar() }} toast={toast} /> : null}
    </div>
  )
}

function CanalModal({ canal, onClose, onSaved, toast }) {
  const [f, setF] = useState({
    id: canal.id || '', nombre: canal.nombre || '', origen: canal.origen || 'ecommerce',
    activo: canal.activo !== false, confirmacionAutomatica: !!canal.confirmacionAutomatica,
    minutosAceptacion: canal.minutosAceptacion || 0, envioPropioDelCanal: !!canal.envioPropioDelCanal,
  })
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))
  const esApp = f.origen === 'app_commerce'

  const guardar = async () => {
    setBusy(true)
    try {
      await api.guardarCanalPedido({ ...f, minutosAceptacion: Number(f.minutosAceptacion) || 0 })
      toast({ title: 'Canal guardado', body: f.nombre })
      onSaved()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  // La URL que la tienda web tiene que llamar. Se muestra armada con el origen
  // real del navegador: escribirla a mano es de donde salen la mitad de los
  // errores de integración.
  const urlEntrante = `${window.location.origin}/api/pedidos/`

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Globe size={18} />}
      title={canal.id ? 'Editar canal' : 'Conectar canal'} sub="Por dónde entran los pedidos"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} disabled={!f.nombre.trim()}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre del canal" required hint="como lo conoce tu equipo">
          <Input value={f.nombre} onChange={(e) => set('nombre', e.target.value)} placeholder="Yummy, PedidosYa, Mi tienda…" autoFocus />
        </Field>
        <Field label="Tipo">
          <Select value={f.origen} onChange={(e) => set('origen', e.target.value)}>
            <option value="ecommerce">Tienda web propia — nos manda los pedidos</option>
            <option value="app_commerce">App de pedidos — publicamos ahí nuestros productos</option>
          </Select>
        </Field>

        {/* ENTRANTE: la tienda nos llama. Se le entrega la dirección. */}
        {!esApp ? (
          <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 px-3 py-2.5 text-[12.5px] text-slate-600 dark:text-slate-300">
            Tu tienda web publica los pedidos llamando a esta dirección:
            <div className="mono text-[11.5px] mt-1 break-all">{urlEntrante}</div>
            <div className="mt-1.5 text-slate-500">Al guardar te damos la clave de acceso para que la pegues en tu tienda.</div>
          </div>
        ) : (
          /* SALIENTE: nosotros nos adaptamos a la app. Cómo conecta cada una
             depende del acuerdo comercial, así que todavía no hay credenciales
             que pedir — y pedir campos que no sabemos usar sería peor. */
          <div className="rounded-lg px-3 py-2.5 text-[12.5px]"
            style={{ background: '#FDF6E7', border: '1px solid #F0DFB8', color: '#92600A' }}>
            Cada app de pedidos se conecta a su manera, según el acuerdo que tengas con ella. Declara el canal ahora para
            que sus pedidos entren identificados con su nombre y su propia regla; la conexión técnica se configura aquí
            cuando sepamos cómo publica los pedidos esa app.
          </div>
        )}

        <Toggle checked={f.activo} onChange={(v) => set('activo', v)} label="Canal activo"
          sub="Apagado, deja de aceptar pedidos de este origen." />
        {/* Confirmar reserva inventario y factura: el default seguro es que
            alguien mire. Por eso esto se enciende a propósito y por canal. */}
        <Toggle checked={f.confirmacionAutomatica} onChange={(v) => set('confirmacionAutomatica', v)}
          label="Confirmar automáticamente"
          sub="Los pedidos que cumplen stock, zona y pago pasan solos; solo se detienen los que fallan algo." />
        <Toggle checked={f.envioPropioDelCanal} onChange={(v) => set('envioPropioDelCanal', v)}
          label="La app reparte con su propia flota"
          sub="Tu local prepara y entrega al repartidor que la app manda. No se asigna repartidor propio." />
        <Field label="Tiempo para aceptar" hint="minutos; 0 = sin límite">
          <Input type="number" min={0} step="1" className="w-28 num" value={f.minutosAceptacion}
            onChange={(e) => set('minutosAceptacion', e.target.value)} />
        </Field>
      </div>
    </Modal>
  )
}

/* --- Zonas de reparto ----------------------------------------------------- */

/* La zona decide tres cosas al confirmar: si se puede llegar, cuánto cuesta el
 * envío y en cuánto se promete. Sin zonas el módulo funciona igual —no bloquea—
 * porque un local que todavía no las dibujó tiene que poder despachar. */
function Zonas() {
  const toast = useToast()
  const [zonas, setZonas] = useState(null)
  const [edit, setEdit] = useState(null)
  const { ui } = useUI()

  const cargar = useCallback(() => {
    api.zonasPedido().then((r) => setZonas(r?.zonas || [])).catch(() => setZonas([]))
  }, [])
  useEffect(() => { cargar() }, [cargar])

  if (zonas === null) return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={3} cols={4} /></div>

  return (
    <div>
      <div className="flex items-start justify-between gap-3 mb-3 flex-wrap">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          Hasta dónde llegas, cuánto cobras por llevar y en cuánto prometes. Se miden por distancia desde la sede, así
          que la más cercana gana sobre una lejana que también alcance.
        </div>
        <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setEdit({ activa: true })}>Nueva zona</Button>
      </div>

      {zonas.length === 0 ? (
        <Empty icon={<Icon.Globe size={22} />} title="Sin zonas de reparto"
          body="Mientras no definas zonas, los pedidos se aceptan sin validar la distancia y sin costo de envío automático."
          cta={<Button icon={<Icon.Plus size={16} />} onClick={() => setEdit({ activa: true })}>Crear zona</Button>} />
      ) : (
        <div className="space-y-2.5">
          {zonas.map((z) => (
            <div key={z.id} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 flex items-center gap-3">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-[13.5px] font-semibold">{z.nombre}</span>
                  {z.activa ? null : <Badge size="sm" color="slate">Inactiva</Badge>}
                </div>
                <div className="text-[12px] text-slate-500 mt-1 num">
                  hasta {z.radioM} m · envío {fmtCurrency(z.costoEnvio, ui.ccy)}
                  {z.minutosPromesa > 0 ? ` · ${z.minutosPromesa} min` : ''}
                  {z.pedidoMinimo > 0 ? ` · mínimo ${fmtCurrency(z.pedidoMinimo, ui.ccy)}` : ''}
                </div>
              </div>
              <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />} onClick={() => setEdit(z)}>Editar</Button>
            </div>
          ))}
        </div>
      )}

      {edit ? <ZonaModal zona={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); cargar() }} toast={toast} /> : null}
    </div>
  )
}

function ZonaModal({ zona, onClose, onSaved, toast }) {
  const [f, setF] = useState({
    id: zona.id || '', nombre: zona.nombre || '', radioM: zona.radioM || 3000,
    costoEnvio: zona.costoEnvio || 0, minutosPromesa: zona.minutosPromesa || 45,
    pedidoMinimo: zona.pedidoMinimo || 0, activa: zona.activa !== false,
    modoEnvio: zona.modoEnvio || '',
  })
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))

  const guardar = async () => {
    setBusy(true)
    try {
      await api.guardarZonaPedido({
        ...f, radioM: Number(f.radioM) || 0, costoEnvio: Number(f.costoEnvio) || 0,
        minutosPromesa: Number(f.minutosPromesa) || 0, pedidoMinimo: Number(f.pedidoMinimo) || 0,
      })
      toast({ title: 'Zona guardada', body: f.nombre })
      onSaved()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Globe size={18} />}
      title={zona.id ? 'Editar zona' : 'Nueva zona de reparto'} sub="Hasta dónde llegas y en qué condiciones"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} disabled={!f.nombre.trim()}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre" required><Input value={f.nombre} onChange={(e) => set('nombre', e.target.value)} placeholder="Cercanías, Los Ruices…" autoFocus /></Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Alcance" hint="metros desde la sede">
            <Input type="number" min={100} step="100" className="num" value={f.radioM} onChange={(e) => set('radioM', e.target.value)} />
          </Field>
          <Field label="Costo del envío">
            <Input type="number" min={0} step="0.01" className="num" value={f.costoEnvio} onChange={(e) => set('costoEnvio', e.target.value)} />
          </Field>
          <Field label="Promesa" hint="minutos">
            <Input type="number" min={0} step="5" className="num" value={f.minutosPromesa} onChange={(e) => set('minutosPromesa', e.target.value)} />
          </Field>
          <Field label="Pedido mínimo" hint="0 = sin mínimo">
            <Input type="number" min={0} step="0.01" className="num" value={f.pedidoMinimo} onChange={(e) => set('pedidoMinimo', e.target.value)} />
          </Field>
        </div>
        <Field label="Quién cubre esta zona" hint="por defecto, tu flota">
          <Select value={f.modoEnvio} onChange={(e) => set('modoEnvio', e.target.value)}>
            <option value="">Flota propia</option>
            <option value="app">App de envío contratada</option>
          </Select>
        </Field>
        <Toggle checked={f.activa} onChange={(v) => set('activa', v)} label="Zona activa"
          sub="Apagada, las direcciones de esta zona quedan fuera de cobertura." />
      </div>
    </Modal>
  )
}

/* --- Repartidores --------------------------------------------------------- */

function Repartidores() {
  const toast = useToast()
  const [lista, setLista] = useState(null)
  const [edit, setEdit] = useState(null)

  const cargar = useCallback(() => {
    api.repartidores().then((r) => setLista(r?.repartidores || [])).catch(() => setLista([]))
  }, [])
  useEffect(() => { cargar() }, [cargar])

  if (lista === null) return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={3} cols={3} /></div>

  return (
    <div>
      <div className="flex items-start justify-between gap-3 mb-3 flex-wrap">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          Tu flota propia. Despacho les asigna pedidos y marca quién está disponible.
        </div>
        <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setEdit({ activo: true, disponible: true })}>Nuevo repartidor</Button>
      </div>

      {lista.length === 0 ? (
        <Empty icon={<Icon.Users size={22} />} title="Sin repartidores"
          body="Si repartes con flota propia, agrégalos acá para poder asignarles pedidos desde despacho."
          cta={<Button icon={<Icon.Plus size={16} />} onClick={() => setEdit({ activo: true, disponible: true })}>Agregar repartidor</Button>} />
      ) : (
        <div className="space-y-2.5">
          {lista.map((r) => (
            <div key={r.id} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 flex items-center gap-3">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="text-[13.5px] font-semibold">{r.nombre}</span>
                  {r.disponible ? <Badge size="sm" color="emerald" dot>Disponible</Badge> : <Badge size="sm" color="slate">No disponible</Badge>}
                </div>
                <div className="text-[12px] text-slate-500 mt-1">
                  {r.codigo ? <span className="mono">{r.codigo} · </span> : null}{r.vehiculo || 'sin vehículo declarado'}
                  {r.telefono ? <span className="num"> · {r.telefono}</span> : null}
                </div>
              </div>
              <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />} onClick={() => setEdit(r)}>Editar</Button>
            </div>
          ))}
        </div>
      )}

      {edit ? <RepartidorModal r={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); cargar() }} toast={toast} /> : null}
    </div>
  )
}

function RepartidorModal({ r, onClose, onSaved, toast }) {
  const [f, setF] = useState({
    id: r.id || '', nombre: r.nombre || '', codigo: r.codigo || '', telefono: r.telefono || '',
    vehiculo: r.vehiculo || 'moto', activo: r.activo !== false, disponible: r.disponible !== false,
  })
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))

  const guardar = async () => {
    setBusy(true)
    try {
      await api.guardarRepartidor(f)
      toast({ title: 'Repartidor guardado', body: f.nombre })
      onSaved()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Users size={18} />}
      title={r.id ? 'Editar repartidor' : 'Nuevo repartidor'}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} disabled={!f.nombre.trim()}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre" required><Input value={f.nombre} onChange={(e) => set('nombre', e.target.value)} autoFocus /></Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Código" hint="opcional"><Input value={f.codigo} onChange={(e) => set('codigo', e.target.value)} placeholder="RP-01" /></Field>
          <Field label="Teléfono"><Input value={f.telefono} onChange={(e) => set('telefono', e.target.value)} placeholder="0414…" /></Field>
        </div>
        <Field label="Vehículo">
          <Select value={f.vehiculo} onChange={(e) => set('vehiculo', e.target.value)}>
            <option value="moto">Moto</option>
            <option value="bicicleta">Bicicleta</option>
            <option value="carro">Carro</option>
            <option value="a_pie">A pie</option>
          </Select>
        </Field>
        <Toggle checked={f.activo} onChange={(v) => set('activo', v)} label="Activo" sub="Inactivo, no aparece para asignar." />
        <Toggle checked={f.disponible} onChange={(v) => set('disponible', v)} label="Disponible ahora"
          sub="Despacho lo ve primero al asignar el próximo pedido." />
      </div>
    </Modal>
  )
}
