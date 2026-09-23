import { useState, useEffect, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Select, Empty, useToast, TableSkeleton, Field, Input, Modal, Segmented, useConfirm } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'

/* FABRICACIÓN — convertir insumos en producto terminado.
 *
 * La pantalla está armada alrededor de la pregunta que se hace quien produce, y
 * en su orden: ¿qué voy a fabricar, me alcanzan los insumos, cuánto me va a
 * costar? Por eso PLANEAR va antes de crear y muestra el costo y los faltantes
 * con la orden todavía sin existir: descubrir que falta un insumo cuando los
 * demás ya salieron del almacén es descubrirlo tarde.
 */

const ESTADOS = {
  borrador: { label: 'Planificada', color: 'slate' },
  en_proceso: { label: 'En proceso', color: 'amber' },
  terminada: { label: 'Terminada', color: 'green' },
  cancelada: { label: 'Cancelada', color: 'slate' },
}

export function Fabricacion() {
  const toast = useToast()
  const confirm = useConfirm()
  const { db, reload } = useData()
  const { ui } = useUI()
  /* Ver la producción le sirve a todo el mundo —quien cocina quiere saber qué hay
   * planificado— pero MOVER una orden toca el inventario, y eso el servidor solo
   * se lo permite a dueño y desarrollador. Se ocultan los botones que van a
   * fallar: mostrar un botón que siempre devuelve 403 es peor que no tenerlo.
   * La interfaz solo oculta; quien protege es el servidor. */
  const puedeMover = ['dueno', 'desarrollador'].includes(ui.rol)
  const [ordenes, setOrdenes] = useState(null)
  const [nueva, setNueva] = useState(false)
  const [abierta, setAbierta] = useState(null)
  const [filtro, setFiltro] = useState('activas')
  const [busy, setBusy] = useState('')

  const cargar = useCallback(() => {
    api.ordenesFabricacion()
      .then((r) => setOrdenes(r?.ordenes || []))
      .catch(() => setOrdenes([]))
  }, [])
  useEffect(() => { cargar() }, [cargar])

  const lista = useMemo(() => {
    const ls = ordenes || []
    if (filtro === 'activas') return ls.filter((o) => o.estado === 'borrador' || o.estado === 'en_proceso')
    if (filtro === 'terminadas') return ls.filter((o) => o.estado === 'terminada')
    return ls
  }, [ordenes, filtro])

  const actuar = async (o, fn, aviso) => {
    setBusy(o.id)
    try {
      await fn()
      toast({ title: aviso, body: o.numeroCompleto })
      cargar(); reload()
      setAbierta(null)
    } catch (e) {
      toast({ title: 'No se pudo', body: e?.message || 'Error', kind: 'error' })
    }
    setBusy('')
  }

  const cancelar = async (o) => {
    if (!(await confirm({
      title: `¿Cancelar la ${o.numeroCompleto}?`,
      body: o.estado === 'en_proceso'
        ? 'Los insumos que ya salieron vuelven al almacén. La orden no se borra: queda cancelada.'
        : 'La orden queda cancelada. Todavía no había tocado el inventario.',
      confirmLabel: 'Cancelar la orden', tone: 'danger',
    }))) return
    actuar(o, () => api.cancelarOrdenFabricacion(o.id, 'cancelada desde la pantalla'), 'Orden cancelada')
  }

  if (ordenes === null) {
    return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={5} cols={4} /></div>
  }

  /* Solo lo que SE GUARDA. Un producto «bajo pedido» se prepara al venderlo y no
   * se stockea: producirlo con una orden dejaría unidades en el ledger que
   * ninguna pantalla muestra. El servidor lo rechaza; acá ni se ofrece, y se dice
   * cómo cambiarlo en vez de dejar al usuario buscando. */
  const conReceta = (db.PRODUCTOS || []).filter((p) => (p.receta || []).length > 0)
  const fabricables = conReceta.filter((p) => p.modoFabricacion === 'para_stock')

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3 flex-wrap">
        <Segmented size="sm" value={filtro} onChange={setFiltro} options={[
          { value: 'activas', label: 'En curso' },
          { value: 'terminadas', label: 'Terminadas' },
          { value: 'todas', label: 'Todas' },
        ]} />
        {puedeMover ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setNueva(true)}
            disabled={!fabricables.length}>Nueva orden</Button>
        ) : null}
      </div>

      {!conReceta.length ? (
        <Empty icon={<Icon.Boxes size={22} />} title="Todavía no hay nada que fabricar"
          body="Una orden de fabricación parte de la RECETA de un producto: qué insumos consume una unidad. Define la receta de un producto en el catálogo y vuelve acá." />
      ) : !fabricables.length ? (
        <Empty icon={<Icon.Boxes size={22} />} title="Tus productos con receta se preparan al venderlos"
          body={`Hay ${conReceta.length} producto(s) con receta, pero todos están como «fabricar bajo pedido»: se preparan cuando se venden y no se guardan, así que no hay orden que hacer. Para producir antes y tener existencia —una bandeja de postres, un lote de pan—, cambia el producto a «fabricar para stock» en el catálogo.`} />
      ) : lista.length === 0 ? (
        <Empty icon={<Icon.Boxes size={22} />} title="Sin órdenes en esta vista"
          body="Crea una orden para producir: la pantalla te dice si alcanzan los insumos y cuánto va a costar antes de sacar nada del almacén."
          cta={puedeMover ? <Button icon={<Icon.Plus size={16} />} onClick={() => setNueva(true)}>Nueva orden</Button> : null} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Orden</th>
                  <th className="py-2.5 pr-3 font-medium">Producto</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Cantidad</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Costo unitario</th>
                  <th className="py-2.5 pr-3 font-medium">Estado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {lista.map((o) => {
                  const st = ESTADOS[o.estado] || { label: o.estado, color: 'slate' }
                  const cant = o.estado === 'terminada' ? o.cantidadProducida : o.cantidad
                  return (
                    <tr key={o.id} className="border-b border-slate-100 dark:border-slate-800/70">
                      <td className="py-2.5 px-3">
                        <button className="font-medium text-[13px] text-elerp-600 dark:text-teal-400"
                          onClick={() => setAbierta(o)}>{o.numeroCompleto}</button>
                      </td>
                      <td className="py-2.5 pr-3 text-[13px]">{o.nombre}<div className="text-[11px] text-slate-400 num">{o.sku}</div></td>
                      <td className="py-2.5 pr-3 text-right num">
                        {fmtNum(cant, 2)}
                        {/* LA MERMA A LA VISTA: de una masa para 20 salen 18, y ese
                            dato vale más que el número planificado. */}
                        {o.estado === 'terminada' && o.cantidad !== o.cantidadProducida ? (
                          <div className="text-[11px] text-amber-700 dark:text-amber-400">
                            de {fmtNum(o.cantidad, 2)} planificadas
                          </div>
                        ) : null}
                      </td>
                      <td className="py-2.5 pr-3 text-right num private-mask">
                        {o.costoUnitario > 0 ? fmtCurrency(o.costoUnitario, 'VES') : <span className="text-slate-300 dark:text-slate-600">—</span>}
                      </td>
                      <td className="py-2.5 pr-3">
                        <Badge size="sm" color={st.color}>{st.label}</Badge>
                        {/* Fuera de tolerancia no bloquea nada —la tanda ya salió—
                            pero queda señalada: el valor del control está en que
                            alguien mire las que se desviaron. */}
                        {o.fueraDeTolerancia ? (
                          <div className="mt-0.5"><Badge size="sm" color="amber">Rindió fuera de lo esperado</Badge></div>
                        ) : null}
                      </td>
                      <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                        {o.estado === 'borrador' && puedeMover ? (
                          <Button size="sm" variant="secondary" loading={busy === o.id}
                            onClick={() => actuar(o, () => api.iniciarOrdenFabricacion(o.id), 'Insumos consumidos')}>Arrancar</Button>
                        ) : null}
                        {o.estado === 'en_proceso' ? (
                          <Button size="sm" loading={busy === o.id} onClick={() => setAbierta(o)}>
                            {puedeMover ? 'Terminar' : 'Ver'}
                          </Button>
                        ) : null}
                        {puedeMover && (o.estado === 'borrador' || o.estado === 'en_proceso') ? (
                          <button className="ml-3 text-[12.5px] text-slate-400 hover:text-[#B3362C]"
                            onClick={() => cancelar(o)}>Cancelar</button>
                        ) : null}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {nueva ? <NuevaOrdenModal productos={fabricables} onClose={() => setNueva(false)}
        onCreada={() => { setNueva(false); cargar() }} toast={toast} /> : null}
      {abierta ? <FichaOrden orden={abierta} puedeMover={puedeMover} onClose={() => setAbierta(null)}
        onTerminar={(prod) => actuar(abierta, () => api.terminarOrdenFabricacion(abierta.id, prod), 'Producción ingresada al inventario')} /> : null}
    </div>
  )
}

/* NuevaOrdenModal: elegir qué y cuánto, VIENDO si alcanza y cuánto cuesta.
 *
 * El plan se pide al servidor mientras se escribe la cantidad. Es una consulta,
 * no una reserva: no toca nada. Pero es lo que convierte «crear una orden» en
 * una decisión informada en vez de un salto al vacío. */
function NuevaOrdenModal({ productos, onClose, onCreada, toast }) {
  const [sku, setSku] = useState(productos[0]?.sku || '')
  const [cantidad, setCantidad] = useState('1')
  const [lote, setLote] = useState('')
  const [vencimiento, setVencimiento] = useState('')
  const [peso, setPeso] = useState('')
  const [plan, setPlan] = useState(null)
  const [busy, setBusy] = useState(false)

  const prod = productos.find((p) => p.sku === sku)
  const cant = Number(cantidad) || 0

  useEffect(() => {
    if (!sku || cant <= 0) { setPlan(null); return }
    let vivo = true
    api.planearOrdenFabricacion({ sku, cantidad: cant })
      .then((r) => { if (vivo) setPlan(r) })
      .catch(() => { if (vivo) setPlan(null) })
    return () => { vivo = false }
  }, [sku, cant])

  const crear = async () => {
    setBusy(true)
    try {
      await api.crearOrdenFabricacion({
        sku, cantidad: cant, lote: lote.trim(), vencimiento,
        pesoUnitario: Number(peso) || 0,
      })
      toast({ title: 'Orden creada', body: `${cant} × ${prod?.nombre || sku}` })
      onCreada()
    } catch (e) {
      toast({ title: 'No se pudo crear', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Boxes size={18} />}
      title="Nueva orden de fabricación"
      sub="Planificar no toca el inventario: los insumos salen al arrancar."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={crear} loading={busy} disabled={!sku || cant <= 0} icon={<Icon.Check size={16} />}>Crear orden</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="grid sm:grid-cols-[1fr_140px] gap-3">
          <Field label="¿Qué se va a fabricar?" hint="solo los productos que tienen receta">
            <Select value={sku} onChange={(e) => setSku(e.target.value)}>
              {productos.map((p) => <option key={p.sku} value={p.sku}>{p.nombre}</option>)}
            </Select>
          </Field>
          <Field label="Cantidad" required>
            <Input type="number" inputMode="decimal" min="0" step="0.01" value={cantidad}
              onChange={(e) => setCantidad(e.target.value)} className="num" autoFocus />
          </Field>
        </div>

        {/* LO QUE VA A COSTAR Y SI ALCANZA, antes de existir la orden. */}
        {plan ? (
          <div className="rounded-xl border border-slate-200 dark:border-slate-700 overflow-hidden">
            <div className="px-3 py-2 text-[12px] bg-slate-50 dark:bg-slate-800/60 flex justify-between">
              <span className="text-slate-500">
                Va a consumir
                {/* De dónde salen las cantidades: sin esto, una receta «para 10»
                    con 80% de rendimiento muestra números que no cuadran con lo
                    que el usuario escribió, y parecen un error. */}
                {plan.factor && plan.factor !== cant ? (
                  <span className="text-slate-400"> · receta × {fmtNum(plan.factor, 2)}</span>
                ) : null}
              </span>
              <span className="num font-semibold">{fmtCurrency(plan.costoTotal, 'VES')}
                <span className="text-slate-400 font-normal"> · {fmtCurrency(plan.costoUnitario, 'VES')} c/u</span></span>
            </div>
            <div className="divide-y divide-slate-100 dark:divide-slate-800">
              {(plan.consumos || []).map((c, i) => {
                const hay = (plan.disponible || [])[i] ?? 0
                const falta = hay + 0.0001 < c.cantidad
                return (
                  <div key={c.sku} className="px-3 py-1.5 flex justify-between text-[13px]">
                    <span>{c.nombre}</span>
                    <span className={`num ${falta ? 'text-[#B3362C] dark:text-red-400 font-semibold' : 'text-slate-500'}`}>
                      {fmtNum(c.cantidad, 2)} <span className="text-slate-400">de {fmtNum(hay, 2)}</span>
                    </span>
                  </div>
                )
              })}
            </div>
            {!plan.alcanza ? (
              <div className="px-3 py-2 text-[12.5px]" style={{ background: '#FBEDEB', color: '#B3362C' }}>
                No alcanza: {(plan.faltantes || []).join(' · ')}. Puedes crear la orden igual, pero no va a poder arrancar.
              </div>
            ) : null}
          </div>
        ) : null}

        {/* Lote y peso: solo donde importan, para no pedir datos que nadie mira. */}
        {prod?.requiereLote ? (
          <div className="grid sm:grid-cols-2 gap-3">
            <Field label="Lote" required hint="lo fabricado hoy es un lote nuevo">
              <Input value={lote} onChange={(e) => setLote(e.target.value)} className="num" />
            </Field>
            <Field label="Vence el" hint="opcional">
              <Input type="date" value={vencimiento} onChange={(e) => setVencimiento(e.target.value)} />
            </Field>
          </div>
        ) : null}
        {prod?.tipoVenta === 'peso' ? (
          <Field label="Peso final por unidad (kg)"
            hint="una torta se fabrica «una torta» y se vende por kilo: sin el peso, lo fabricado y lo vendido no son la misma unidad">
            <Input type="number" inputMode="decimal" min="0" step="0.001" value={peso}
              onChange={(e) => setPeso(e.target.value)} className="num" />
          </Field>
        ) : null}
      </div>
    </Modal>
  )
}

/* FichaOrden: qué consumió y, si está en proceso, cuánto salió de verdad. */
function FichaOrden({ orden: o, puedeMover = true, onClose, onTerminar }) {
  const [producida, setProducida] = useState(String(o.cantidad))
  const st = ESTADOS[o.estado] || { label: o.estado, color: 'slate' }
  const enProceso = o.estado === 'en_proceso' && puedeMover
  const prod = Number(producida) || 0

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Boxes size={18} />}
      title={`${o.numeroCompleto} · ${o.nombre}`} sub={st.label}
      footer={enProceso ? <>
        <Button variant="ghost" onClick={onClose}>Cerrar</Button>
        <Button onClick={() => onTerminar(prod)} disabled={prod <= 0} icon={<Icon.Check size={16} />}>
          Terminar e ingresar al inventario
        </Button>
      </> : <Button variant="ghost" onClick={onClose}>Cerrar</Button>}>
      <div className="space-y-3.5 text-[13px]">
        {enProceso ? (
          <Field label="¿Cuántas salieron de verdad?"
            hint="de una masa para 20 panes salen 18: el costo se reparte entre los que salieron, y la diferencia queda registrada como merma">
            <Input type="number" inputMode="decimal" min="0" step="0.01" value={producida}
              onChange={(e) => setProducida(e.target.value)} className="num" autoFocus />
          </Field>
        ) : null}

        {(o.consumos || []).length ? (
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
            <div className="px-3 py-2 text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/60">
              Consumió — congelado al arrancar, porque la receta puede cambiar mañana
            </div>
            <div className="divide-y divide-slate-100 dark:divide-slate-800">
              {o.consumos.map((c) => (
                <div key={c.sku} className="px-3 py-1.5 flex justify-between">
                  <span>{c.nombre} <span className="text-slate-400 num">× {fmtNum(c.cantidad, 2)}</span></span>
                  <span className="num text-slate-500 private-mask">{fmtCurrency(c.cantidad * c.costoUnitario, 'VES')}</span>
                </div>
              ))}
              <div className="px-3 py-1.5 flex justify-between font-semibold">
                <span>Costo de los insumos</span>
                <span className="num private-mask">{fmtCurrency(o.costoTotal, 'VES')}</span>
              </div>
            </div>
          </div>
        ) : (
          <div className="text-[12.5px] text-slate-400">Todavía no ha consumido nada: los insumos salen al arrancar.</div>
        )}

        {o.estado === 'terminada' ? (
          <div className="rounded-lg px-3 py-2 text-[12.5px]" style={{ background: '#EAF5EF', color: '#166B41' }}>
            Ingresaron <strong>{fmtNum(o.cantidadProducida, 2)}</strong> al inventario
            a <strong>{fmtCurrency(o.costoUnitario, 'VES')}</strong> cada una.
            {o.cantidad !== o.cantidadProducida
              ? ` Merma: ${fmtNum(o.cantidad - o.cantidadProducida, 2)} — su costo lo absorbieron las que sí salieron.`
              : ''}
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
