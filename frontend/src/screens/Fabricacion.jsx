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

// Etiqueta corta de cada destino para el listado. La larga y su ayuda vienen del
// servidor, que es quien valida el catálogo.
const DESTINO_CORTO = { perdida: 'pérdida', descarte: 'descarte', reproceso: 'para reprocesar' }

const ESTADOS = {
  borrador: { label: 'Planificada', color: 'slate' },
  en_proceso: { label: 'En proceso', color: 'amber' },
  terminada: { label: 'Terminada', color: 'green' },
  cancelada: { label: 'Cancelada', color: 'slate' },
}

export function Fabricacion({ route, navigate }) {
  const sub = (route || '').split(':')[1] || 'ordenes'
  return (
    <div>
      {sub === 'formulas' ? <Formulas />
        : sub === 'resumen' ? <Resumen />
          : <Ordenes irA={navigate} />}
    </div>
  )
}

function Ordenes({ irA = () => {} }) {
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
        /* Decir DÓNDE se hace, no solo que falta. Antes remitía al catálogo, que
           no edita fórmulas: quien activaba el módulo se quedaba mirando una
           pantalla vacía sin saber por dónde empezar. */
        <Empty icon={<Icon.Boxes size={22} />} title="Todavía no hay ninguna fórmula"
          body="Una orden parte de la FÓRMULA de un producto: qué insumos consume, para qué tanda está escrita y cuánto rinde. Créala en «Fórmulas» y vuelve acá."
          cta={puedeMover ? <Button icon={<Icon.Plus size={16} />} onClick={() => irA('fabricacion:formulas')}>Crear la primera fórmula</Button> : null} />
      ) : !fabricables.length ? (
        <Empty icon={<Icon.Boxes size={22} />} title="Tus productos con receta se preparan al venderlos"
          body={`Hay ${conReceta.length} producto(s) con fórmula, pero todos están como «fabricar bajo pedido»: se preparan cuando se venden y no se guardan, así que no hay orden que hacer. Para producir antes y tener existencia —una bandeja de postres, un lote de pan, una pieza armada—, cámbialo a «fabricar para stock» en Fórmulas.`}
          cta={puedeMover ? <Button variant="secondary" onClick={() => irA('fabricacion:formulas')}>Ir a Fórmulas</Button> : null} />
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
                        {/* El reparto, no solo el total: lo botado y lo recuperable
                            no son lo mismo y el listado es donde se comparan. */}
                        {(o.resultados || []).map((r, i) => (
                          <div key={i} className="text-[11px] text-slate-400">
                            {fmtNum(r.cantidad, 2)} · {DESTINO_CORTO[r.destino] || r.destino}
                          </div>
                        ))}
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
                            {puedeMover ? 'Reportar resultado' : 'Ver'}
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
        onReportar={(prod, resultados) => actuar(abierta,
          () => api.terminarOrdenFabricacion(abierta.id, prod, resultados), 'Resultado reportado')} /> : null}
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

  /* CREAR Y ARRANCAR EN UN PASO. Lo normal en un taller es planificar y empezar
   * de inmediato: separarlo en dos clics le hace teclear al operario una
   * transición que ya decidió. Queda el botón aparte para cuando de verdad se
   * planifica para después. */
  const crear = async (iniciar) => {
    setBusy(true)
    try {
      const r = await api.crearOrdenFabricacion({
        sku, cantidad: cant, lote: lote.trim(), vencimiento,
        pesoUnitario: Number(peso) || 0, iniciar,
      })
      if (r?.avisoInicio) {
        // La orden quedó creada; lo que falló fue arrancarla. Se dice cuál de las
        // dos cosas pasó, no un «no se pudo» que deja sin saber si existe o no.
        toast({ title: 'Orden creada, pero no arrancó', body: r.avisoInicio, kind: 'warn' })
      } else {
        toast({ title: iniciar ? 'Orden arrancada' : 'Orden creada', body: `${cant} × ${prod?.nombre || sku}` })
      }
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
        <Button variant="secondary" onClick={() => crear(false)} loading={busy} disabled={!sku || cant <= 0}>
          Solo planificar
        </Button>
        <Button onClick={() => crear(true)} loading={busy} disabled={!sku || cant <= 0 || !plan?.alcanza}
          title={plan && !plan.alcanza ? 'No alcanzan los insumos para arrancar' : 'Saca los insumos y empieza'}
          icon={<Icon.Check size={16} />}>Crear y arrancar</Button>
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

/* FichaOrden — y, cuando la tanda está en el mesón, EL REPORTE DEL RESULTADO.
 *
 * El resultado de fabricar no es un número: es un reparto. De una tanda de 15
 * pueden salir 10 buenos, 3 perdidos y 2 que sirven para reprocesar, y meterlos
 * todos en «salieron 10» borra la diferencia entre lo que se botó y lo que se
 * puede recuperar — que es justo la que dice si hay que cambiar algo del proceso.
 */
function FichaOrden({ orden: o, puedeMover = true, onClose, onReportar }) {
  const [producida, setProducida] = useState(String(o.cantidad))
  const [filas, setFilas] = useState([])
  const [destinos, setDestinos] = useState([])
  const st = ESTADOS[o.estado] || { label: o.estado, color: 'slate' }
  const enProceso = o.estado === 'en_proceso' && puedeMover
  const prod = Number(producida) || 0

  useEffect(() => {
    api.destinosFabricacion().then((r) => setDestinos(r?.destinos || [])).catch(() => setDestinos([]))
  }, [])

  const addFila = () => setFilas((s2) => [...s2, { cantidad: '', destino: 'perdida', motivo: '' }])
  const setFila = (i, k, v) => setFilas((s2) => s2.map((x, j) => (j === i ? { ...x, [k]: v } : x)))
  const delFila = (i) => setFilas((s2) => s2.filter((_, j) => j !== i))

  const noLogrado = filas.reduce((a, x) => a + (Number(x.cantidad) || 0), 0)
  // Falta explicar lo que no salió: si lo declarado no suma lo planificado, hay
  // unidades sin destino y nadie va a saber después qué pasó con ellas.
  const sinExplicar = Math.round((o.cantidad - prod - noLogrado) * 100) / 100
  const faltaMotivo = filas.some((x) => Number(x.cantidad) > 0 && !String(x.motivo || '').trim())

  const reportar = () => onReportar(prod, filas
    .filter((x) => Number(x.cantidad) > 0)
    .map((x) => ({ cantidad: Number(x.cantidad), destino: x.destino, motivo: x.motivo.trim() })))

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Boxes size={18} />}
      title={`${o.numeroCompleto} · ${o.nombre}`} sub={st.label}
      footer={enProceso ? <>
        <Button variant="ghost" onClick={onClose}>Cerrar</Button>
        <Button onClick={reportar} disabled={faltaMotivo} icon={<Icon.Check size={16} />}>
          Reportar resultado
        </Button>
      </> : <Button variant="ghost" onClick={onClose}>Cerrar</Button>}>
      <div className="space-y-3.5 text-[13px]">
        {enProceso ? (
          <>
            <Field label="¿Cuántas salieron BIEN?"
              hint="Lo que entra al inventario. Cero es una respuesta válida: la tanda se perdió entera.">
              <Input type="number" inputMode="decimal" min="0" step="0.01" value={producida}
                onChange={(e) => setProducida(e.target.value)} className="num" autoFocus />
            </Field>

            <div>
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-[13px] font-semibold text-slate-700 dark:text-slate-300">¿Y lo que no salió bien?</span>
                <Button size="sm" variant="ghost" icon={<Icon.Plus size={14} />} onClick={addFila}>Agregar</Button>
              </div>
              {filas.length === 0 ? (
                <div className="text-[12px] text-slate-400">
                  Nada más que declarar. Si parte de la tanda se perdió o sirve para reprocesar, agrégalo:
                  lo botado y lo recuperable no son lo mismo.
                </div>
              ) : null}
              <div className="space-y-1.5">
                {filas.map((x, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <Input type="number" min="0" step="0.01" value={x.cantidad} placeholder="0"
                      onChange={(e) => setFila(i, 'cantidad', e.target.value)} className="w-20 num" />
                    <Select value={x.destino} onChange={(e) => setFila(i, 'destino', e.target.value)} className="w-44">
                      {destinos.map((d) => <option key={d.codigo} value={d.codigo}>{d.nombre}</option>)}
                    </Select>
                    <Input value={x.motivo} placeholder="¿Qué pasó?"
                      onChange={(e) => setFila(i, 'motivo', e.target.value)} className="flex-1" />
                    <button onClick={() => delFila(i)} className="p-1.5 rounded-md text-slate-400 hover:text-red-500">
                      <Icon.Trash size={14} />
                    </button>
                  </div>
                ))}
              </div>
              {filas.length ? (
                <div className="mt-1.5 text-[11.5px] text-slate-400">
                  {destinos.find((d) => d.codigo === filas[filas.length - 1].destino)?.ayuda}
                </div>
              ) : null}
              {sinExplicar > 0.005 ? (
                <div className="mt-2 rounded-lg px-3 py-2 text-[12.5px]"
                  style={{ background: '#FFF7E8', border: '1px solid #EEDCB4', color: '#92600A' }}>
                  Quedan <strong>{fmtNum(sinExplicar, 2)}</strong> sin explicar de las {fmtNum(o.cantidad, 2)}
                  {' '}planificadas. Puedes reportarlo igual —a veces simplemente rinde menos— pero si se
                  perdieron o se pueden recuperar, decirlo ahora es la única oportunidad.
                </div>
              ) : null}
            </div>
          </>
        ) : null}

        {(o.consumos || []).length ? (
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
            <div className="px-3 py-2 text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/60">
              Consumió — congelado al arrancar, porque la fórmula puede cambiar mañana
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

        {/* EL REGISTRO DE LO QUE PASÓ, una vez reportado. */}
        {o.estado === 'terminada' ? (
          <>
            <div className="rounded-lg px-3 py-2 text-[12.5px]" style={{ background: '#EAF5EF', color: '#166B41' }}>
              Entraron <strong>{fmtNum(o.cantidadProducida, 2)}</strong> al inventario
              a <strong>{fmtCurrency(o.costoUnitario, 'VES')}</strong> cada una.
            </div>
            {(o.resultados || []).length ? (
              <div className="rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
                <div className="px-3 py-2 text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/60">Lo que no salió bien</div>
                <div className="divide-y divide-slate-100 dark:divide-slate-800">
                  {o.resultados.map((r, i) => (
                    <div key={i} className="px-3 py-1.5 flex justify-between gap-3">
                      <span>
                        <span className="num">{fmtNum(r.cantidad, 2)}</span>
                        {' · '}{destinos.find((d) => d.codigo === r.destino)?.nombre || r.destino}
                      </span>
                      <span className="text-slate-500 text-right">{r.motivo}</span>
                    </div>
                  ))}
                </div>
              </div>
            ) : null}
            {o.perdidaAnormal > 0 ? (
              /* La merma que se pasa de la tolerancia NO la cargan los buenos:
                 inflaría su costo y escondería el problema dentro del margen. */
              <div className="rounded-lg px-3 py-2 text-[12.5px]"
                style={{ background: '#FBEDEB', border: '1px solid #ECC8C4', color: '#B3362C' }}>
                <strong>{fmtCurrency(o.perdidaAnormal, 'VES')}</strong> fueron a pérdida del período: es la
                merma que se pasó de la tolerancia, y no la cargan las unidades buenas.
              </div>
            ) : null}
          </>
        ) : null}
      </div>
    </Modal>
  )
}

/* FÓRMULAS — qué consume un producto, para qué tanda y cuánto rinde.
 *
 * VIVE ACÁ Y NO SOLO EN RESTAURANTE, y esa era la falla: la receta únicamente se
 * podía definir dentro del módulo Restaurante, así que un taller que activaba
 * Fabricación no tenía dónde crear nada — abría la pantalla, la veía vacía y no
 * podía hacer una sola orden. El módulo prometía servir igual a una cocina que a
 * un taller y solo servía a la cocina.
 *
 * El vocabulario también cambia: acá no hay «platos» ni comanderas. Hay un
 * producto que se fabrica y los insumos que consume.
 */
function Formulas() {
  const toast = useToast()
  const confirm = useConfirm()
  const { db, reload } = useData()
  const { ui } = useUI()
  const puedeEditar = ['dueno', 'desarrollador'].includes(ui.rol)
  const [form, setForm] = useState(null)

  const productos = db.PRODUCTOS || []
  const conFormula = productos.filter((p) => (p.receta || []).length > 0)
  // Insumos posibles: lo que se stockea y no es el propio producto. Un preparado
  // que se fabrica para stock TAMBIÉN sirve —la salsa entra al sándwich— y eso
  // es lo que permite encadenar fórmulas.
  const insumos = productos.filter((p) => p.activo !== false && !p.esCombo && !p.esServicio
    && (!p.esPlato || p.modoFabricacion === 'para_stock'))

  const quitar = async (p) => {
    if (!(await confirm({
      title: `¿Quitar la fórmula de «${p.nombre}»?`,
      body: 'El producto queda en el catálogo sin fórmula: deja de poder fabricarse. Lo ya producido no se toca.',
      confirmLabel: 'Quitar fórmula', tone: 'danger',
    }))) return
    try {
      await api.actualizarProducto(p.sku, { esPlato: false, receta: [] })
      await reload()
      toast({ title: 'Fórmula quitada', body: p.nombre })
    } catch (e) {
      toast({ title: 'No se pudo', body: e?.message || 'Error', kind: 'error' })
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3 flex-wrap">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          Una fórmula dice <strong>qué consume</strong> el producto, <strong>para qué tanda</strong> está
          escrita y <strong>cuánto rinde</strong>. De ahí sale el costo real de lo que fabriques: no se
          teclea, se deriva de lo que salió del almacén.
        </div>
        {puedeEditar ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setForm({})}>Nueva fórmula</Button>
        ) : null}
      </div>

      {conFormula.length === 0 ? (
        <Empty icon={<Icon.Boxes size={22} />} title="Sin fórmulas todavía"
          body="Define qué insumos consume un producto y podrás producirlo con una orden: los insumos salen del almacén al arrancar y lo fabricado entra con su costo real."
          cta={puedeEditar ? <Button icon={<Icon.Plus size={16} />} onClick={() => setForm({})}>Crear la primera</Button> : null} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Producto</th>
                  <th className="py-2.5 pr-3 font-medium">Insumos</th>
                  <th className="py-2.5 pr-3 font-medium">Tanda / rinde</th>
                  <th className="py-2.5 pr-3 font-medium">Cuándo se produce</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {conFormula.map((p) => (
                  <tr key={p.sku} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2.5 px-3">
                      <div className="font-medium text-[13px]">{p.nombre}</div>
                      <div className="text-[11px] text-slate-400 num">{p.sku}</div>
                    </td>
                    <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">{(p.receta || []).length}</td>
                    <td className="py-2.5 pr-3 text-[12.5px] num text-slate-500">
                      {p.loteBase > 1 ? `para ${fmtNum(p.loteBase, 0)}` : 'por unidad'}
                      {p.rendimientoPct > 0 && p.rendimientoPct < 100 ? ` · rinde ${fmtNum(p.rendimientoPct, 0)}%` : ''}
                    </td>
                    <td className="py-2.5 pr-3">
                      {p.modoFabricacion === 'para_stock'
                        ? <Badge size="sm" color="teal">Se fabrica y se guarda</Badge>
                        : <Badge size="sm" color="slate">Se prepara al venderlo</Badge>}
                    </td>
                    <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                      {puedeEditar ? (
                        <>
                          <button className="text-[12.5px] text-elerp-600 dark:text-teal-400 font-medium"
                            onClick={() => setForm(p)}>Editar</button>
                          <button className="ml-3 text-[12.5px] text-slate-400 hover:text-[#B3362C]"
                            onClick={() => quitar(p)}>Quitar</button>
                        </>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {form ? <FormulaModal producto={form.sku ? form : null} insumos={insumos} toast={toast}
        onClose={() => setForm(null)} onGuardada={async () => { setForm(null); await reload() }} /> : null}
    </div>
  )
}

function FormulaModal({ producto, insumos, toast, onClose, onGuardada }) {
  const editar = !!producto
  const [f, setF] = useState(() => ({
    sku: producto?.sku || '', nombre: producto?.nombre || '', precio: producto?.precio || 0,
    receta: (producto?.receta || []).map((r) => ({ ...r })),
    /* El modo se lee tal cual viene: solo «para_stock» es para stock, y CUALQUIER
     * otra cosa —incluido el vacío de los registros viejos— es bajo pedido.
     * Caer a «para stock» por defecto era lo que convertía el producto al
     * guardar, sin que nadie lo hubiera pedido. */
    modoFabricacion: producto?.modoFabricacion === 'para_stock' ? 'para_stock' : 'bajo_pedido',
    loteBase: producto?.loteBase || '', rendimientoPct: producto?.rendimientoPct || '',
    toleranciaPct: producto?.toleranciaPct || '',
  }))
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))
  const setRow = (i, k, v) => setF((s) => ({ ...s, receta: s.receta.map((r, j) => (j === i ? { ...r, [k]: v } : r)) }))
  const addRow = () => setF((s) => ({ ...s, receta: [...s.receta, { sku: '', cantidad: 1 }] }))
  const delRow = (i) => setF((s) => ({ ...s, receta: s.receta.filter((_, j) => j !== i) }))

  const guardar = async () => {
    const receta = f.receta.filter((r) => r.sku && Number(r.cantidad) > 0)
      .map((r) => ({ sku: r.sku, cantidad: Number(r.cantidad), mermaPct: Number(r.mermaPct) || 0 }))
    if (!f.nombre.trim()) { toast({ title: 'Ponle nombre al producto', kind: 'warn' }); return }
    if (!receta.length) { toast({ title: 'Agrega al menos un insumo', kind: 'warn' }); return }
    const cuerpo = {
      nombre: f.nombre.trim(), precio: Number(f.precio) || 0,
      esPlato: true, receta, modoFabricacion: f.modoFabricacion,
      loteBase: Number(f.loteBase) || 0, rendimientoPct: Number(f.rendimientoPct) || 0,
      toleranciaPct: Number(f.toleranciaPct) || 0,
    }
    setBusy(true)
    try {
      if (editar) await api.actualizarProducto(f.sku, cuerpo)
      else await api.createProducto({ ...cuerpo, sku: (f.sku || `FAB-${Date.now().toString(36).toUpperCase()}`).trim() })
      toast({ title: editar ? 'Fórmula actualizada' : 'Fórmula creada', body: f.nombre })
      await onGuardada()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Boxes size={18} />}
      title={editar ? `Fórmula de ${producto.nombre}` : 'Nueva fórmula'}
      sub="De acá sale el costo real de lo que fabriques"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy}>{editar ? 'Guardar' : 'Crear'}</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="grid grid-cols-2 gap-2">
          <Field label="Producto que se fabrica" required>
            <Input value={f.nombre} autoFocus onChange={(e) => set('nombre', e.target.value)}
              placeholder="Pieza armada, bandeja de brownies…" />
          </Field>
          <Field label="Precio de venta" hint="opcional si no se vende directo">
            <Input type="number" min={0} value={f.precio} onChange={(e) => set('precio', e.target.value)} className="num" />
          </Field>
        </div>

        <Field label="¿Cuándo se produce?"
          hint={f.modoFabricacion === 'para_stock'
            ? 'Con una orden, antes de venderlo. Queda en existencia y al venderlo se descuenta él, no sus insumos.'
            : 'Al venderlo. No tiene existencia propia y descuenta sus insumos en ese momento — no se produce con órdenes.'}>
          <Select value={f.modoFabricacion} onChange={(e) => set('modoFabricacion', e.target.value)}>
            <option value="para_stock">Se fabrica y se guarda</option>
            <option value="bajo_pedido">Se prepara al venderlo</option>
          </Select>
        </Field>

        {f.modoFabricacion === 'para_stock' ? (
          <div className="grid grid-cols-3 gap-2">
            <Field label="La fórmula es para" hint="¿cuántas unidades? En blanco: una.">
              <Input type="number" min={0} step="0.01" placeholder="1" value={f.loteBase}
                onChange={(e) => set('loteBase', e.target.value)} className="num" />
            </Field>
            <Field label="Rendimiento %" hint="10 kg crudos dan 6,5 cocidos ⇒ 65%">
              <Input type="number" min={0} max={100} step="0.1" placeholder="100" value={f.rendimientoPct}
                onChange={(e) => set('rendimientoPct', e.target.value)} className="num" />
            </Field>
            <Field label="Tolerancia %" hint="en blanco no se controla">
              <Input type="number" min={0} step="0.1" placeholder="—" value={f.toleranciaPct}
                onChange={(e) => set('toleranciaPct', e.target.value)} className="num" />
            </Field>
          </div>
        ) : null}

        <div>
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-[13px] font-semibold text-slate-700 dark:text-slate-300">Insumos que consume</span>
            <Button size="sm" variant="ghost" icon={<Icon.Plus size={14} />} onClick={addRow}>Agregar</Button>
          </div>
          <div className="space-y-1.5">
            {f.receta.length === 0
              ? <div className="text-[12px] text-slate-400">Todavía ninguno. Agrega lo que consume una tanda.</div>
              : null}
            {f.receta.map((r, i) => {
              const ins = insumos.find((x) => x.sku === r.sku)
              return (
                <div key={i} className="flex items-center gap-2">
                  <Select value={r.sku} onChange={(e) => setRow(i, 'sku', e.target.value)} className="flex-1">
                    <option value="">Elegir insumo…</option>
                    {insumos.map((p) => <option key={p.sku} value={p.sku}>{p.nombre}</option>)}
                  </Select>
                  <Input type="number" min={0} step="0.001" value={r.cantidad}
                    onChange={(e) => setRow(i, 'cantidad', e.target.value)} className="w-24 num" />
                  <span className="text-[11.5px] text-slate-400 w-10">{ins?.unidadBase || ''}</span>
                  <Input type="number" min={0} max={99} step="0.1" placeholder="0" value={r.mermaPct ?? ''}
                    title="Merma al preparar este insumo (%)"
                    onChange={(e) => setRow(i, 'mermaPct', e.target.value)} className="w-16 num" />
                  <span className="text-[11.5px] text-slate-400">% merma</span>
                  <button onClick={() => delRow(i)} className="p-1.5 rounded-md text-slate-400 hover:text-red-500">
                    <Icon.Trash size={14} />
                  </button>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </Modal>
  )
}


/* RESUMEN DEL PERÍODO — lo que orden por orden no se ve.
 *
 * Una tanda que pierde tres unidades es mala suerte. Veinte tandas perdiendo
 * tres cada una es un problema del proceso, y esa diferencia solo aparece
 * sumando. Lo mismo con la pérdida anormal: orden por orden son cifras chicas;
 * junta es una línea del estado de resultados.
 */
function Resumen() {
  const [r, setR] = useState(null)
  const [desde, setDesde] = useState('')
  const [hasta, setHasta] = useState('')

  useEffect(() => {
    api.resumenFabricacion(desde, hasta).then(setR).catch(() => setR(null))
  }, [desde, hasta])

  if (!r) {
    return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={4} cols={4} /></div>
  }

  const rendimiento = r.planificado > 0 ? (r.producido / r.planificado) * 100 : 0

  return (
    <div className="space-y-4">
      <div className="flex items-end gap-3 flex-wrap">
        <Field label="Desde"><Input type="date" value={desde} onChange={(e) => setDesde(e.target.value)} /></Field>
        <Field label="Hasta"><Input type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} /></Field>
        <div className="text-[12px] text-slate-400 pb-2">
          {desde || hasta ? '' : 'Todo lo que hay. Acota el rango para comparar períodos.'}
        </div>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <Tarjeta label="Producido" valor={fmtNum(r.producido, 2)}
          sub={`de ${fmtNum(r.planificado, 2)} planificadas · ${fmtNum(rendimiento, 1)}% de rendimiento real`} />
        <Tarjeta label="Perdido" valor={fmtNum(r.perdido, 2)} tono={r.perdido > 0 ? 'malo' : ''}
          sub="no quedó nada" />
        <Tarjeta label="Descartado" valor={fmtNum(r.descartado, 2)} sub="existe, no se vende" />
        <Tarjeta label="Para reprocesar" valor={fmtNum(r.reprocesado, 2)} tono={r.reprocesado > 0 ? 'bueno' : ''}
          sub="vuelve a producción" />
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <Tarjeta label="Costo de insumos" valor={fmtCurrency(r.costoInsumos, 'VES')} />
        <Tarjeta label="Valor producido" valor={fmtCurrency(r.valorProducido, 'VES')} />
        {/* La pérdida anormal es el número que nadie mira hasta que duele. */}
        <Tarjeta label="Pérdida del período" valor={fmtCurrency(r.perdidaAnormal, 'VES')}
          tono={r.perdidaAnormal > 0 ? 'malo' : ''} sub="merma que se pasó de la tolerancia" />
        <Tarjeta label="Tandas a revisar" valor={fmtNum(r.fueraDeTolerancia, 0)}
          tono={r.fueraDeTolerancia > 0 ? 'malo' : ''} sub={`de ${fmtNum(r.terminadas, 0)} terminadas`} />
      </div>

      {/* El trabajo en proceso no entra en el rendimiento —todavía no falló ni
          salió bien— pero sus insumos YA salieron del almacén. Se dice aparte. */}
      {r.enCurso > 0 ? (
        <div className="text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 rounded-lg px-3 py-2">
          {fmtNum(r.enCurso, 0)} tanda(s) siguen en el taller: {fmtNum(r.enProceso, 2)} unidades planificadas
          con <span className="num private-mask">{fmtCurrency(r.costoEnProceso, 'VES')}</span> en insumos ya fuera del almacén.
          No entran en el rendimiento hasta que se reporte su resultado.
        </div>
      ) : null}

      {(r.motivos || []).length ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <div className="px-3.5 py-2.5 text-[12px] text-slate-500 border-b border-slate-100 dark:border-slate-800">
            Por qué no salió — ordenado por lo que más pesa, que es por donde hay que empezar a mirar
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {r.motivos.map((m, i) => (
              <div key={i} className="px-3.5 py-2 flex justify-between gap-3 text-[13px]">
                <span>{m.motivo} <span className="text-slate-400">· {DESTINO_CORTO[m.destino] || m.destino}</span></span>
                <span className="num text-slate-500 whitespace-nowrap">
                  {fmtNum(m.cantidad, 2)} <span className="text-slate-400">en {m.veces} tanda(s)</span>
                </span>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      {(r.porProducto || []).length ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Producto</th>
                  <th className="py-2.5 pr-3 font-medium text-center">Tandas</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Producido</th>
                  <th className="py-2.5 pr-3 font-medium text-right">No logrado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Rendimiento real</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Pérdida</th>
                </tr>
              </thead>
              <tbody>
                {r.porProducto.map((f) => (
                  <tr key={f.sku} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2.5 px-3">
                      <div className="font-medium text-[13px]">{f.nombre}</div>
                      <div className="text-[11px] text-slate-400 num">{f.sku}</div>
                    </td>
                    <td className="py-2.5 pr-3 text-center num text-slate-500">{f.ordenes}</td>
                    <td className="py-2.5 pr-3 text-right num">{fmtNum(f.producido, 2)}
                      <span className="text-slate-400"> / {fmtNum(f.planificado, 2)}</span></td>
                    <td className="py-2.5 pr-3 text-right num text-slate-500">{fmtNum(f.noLogrado, 2)}</td>
                    <td className={`py-2.5 pr-3 text-right num font-medium ${f.rendimientoPct < 90 ? 'text-amber-700 dark:text-amber-400' : ''}`}>
                      {fmtNum(f.rendimientoPct, 1)}%
                    </td>
                    <td className="py-2.5 pr-3 text-right num private-mask">
                      {f.perdidaAnormal > 0 ? fmtCurrency(f.perdidaAnormal, 'VES') : <span className="text-slate-300 dark:text-slate-600">—</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <Empty icon={<Icon.Boxes size={22} />} title="Sin producción en este período"
          body="Cuando haya órdenes terminadas, acá se ve cuánto salió, cuánto se perdió y por qué." />
      )}
    </div>
  )
}

function Tarjeta({ label, valor, sub, tono }) {
  const color = tono === 'malo' ? 'text-[#B3362C] dark:text-red-400'
    : tono === 'bueno' ? 'text-[#166B41] dark:text-emerald-400' : ''
  return (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-3.5">
      <div className="text-[11.5px] text-slate-500">{label}</div>
      <div className={`text-[19px] font-semibold num mt-0.5 private-mask ${color}`}>{valor}</div>
      {sub ? <div className="text-[11px] text-slate-400 mt-0.5">{sub}</div> : null}
    </div>
  )
}
