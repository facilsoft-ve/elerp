import { useState, useMemo } from 'react'
import { vendibles } from '../lib/catalogo.js'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Input, Select, Toggle, Field, VistaDetalle, Empty, TableSkeleton, useToast } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { monedaDe, monedaLabel, MONEDAS_META } from '../lib/precio.js'
import { api } from '../lib/api.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'

/* Listas de precio (tarifas). Un maestro EDITABLE (no ledger append-only): una
 * lista fija un precio explícito por SKU que reemplaza al precio base del
 * catálogo; los productos no listados conservan su precio base. La MISMA pantalla
 * sirve a Ventas (tipo="venta") y a Compras (tipo="compra"): solo cambia el tipo,
 * la copia y qué se compara como «precio base».
 *
 * El detalle/edición va a PANTALLA COMPLETA (VistaDetalle), como el resto del
 * módulo. Solo Dueña/Desarrollador editan; los demás roles ven en consulta. */

const MONEDAS = Object.keys(MONEDAS_META) // VES, USD, EUR, COP, USDT

// Copia por tipo de lista, para no duplicar la pantalla.
const COPY = {
  venta: {
    titulo: 'Listas de precio',
    sub: 'Tarifas de venta: un precio por producto que reemplaza al precio base del catálogo.',
    nueva: 'Nueva lista de venta',
    vacio: 'Aún no hay listas de precio de venta',
    vacioBody: 'Crea una tarifa (mayorista, por canal o volumen) con precios propios por producto. Los productos que no incluyas usan el precio base.',
    colLista: 'Precio de venta',
    icon: Icon.Tag,
  },
  compra: {
    titulo: 'Listas de precio de compras',
    sub: 'Tarifas de compra: costos negociados por producto que reemplazan al precio base del catálogo.',
    nueva: 'Nueva lista de compra',
    vacio: 'Aún no hay listas de precio de compras',
    vacioBody: 'Crea una tarifa negociada (por proveedor o volumen) con costos propios por producto. Los productos que no incluyas usan el precio base.',
    colLista: 'Costo de compra',
    icon: Icon.Tag,
  },
}

const puedeEditar = (rol) => ['dueno', 'desarrollador'].includes(rol)

export function ListasPrecio({ tipo = 'venta' }) {
  const { db, loading, error, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const copy = COPY[tipo] || COPY.venta

  const listas = useMemo(
    () => (db.LISTAS_PRECIO || []).filter((l) => l.tipo === tipo),
    [db.LISTAS_PRECIO, tipo],
  )

  const [q, setQ] = useState('')
  const [editar, setEditar] = useState(null) // lista en edición | 'nueva' | null

  const edita = puedeEditar(ui.rol)

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return listas.filter((l) => !term || (l.nombre || '').toLowerCase().includes(term))
  }, [listas, q])

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las listas de precio"
      body={String(error.message || error)} cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  // Alta / edición a PANTALLA COMPLETA (estilo del resto del módulo).
  if (editar) {
    return (
      <FormLista lista={editar === 'nueva' ? null : editar} tipo={tipo} copy={copy}
        onVolver={() => setEditar(null)} onSaved={reload} toast={toast} />
    )
  }

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Input className="w-64" icon={<Icon.Search size={15} />} placeholder="Buscar lista por nombre…" value={q} onChange={(e) => setQ(e.target.value)} />
        {edita ? <Button className="ml-auto" icon={<Icon.Plus size={16} />} onClick={() => setEditar('nueva')}>{copy.nueva}</Button> : null}
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {loading || db.LISTAS_PRECIO === undefined ? (
          <div className="p-4"><TableSkeleton rows={5} cols={edita ? 5 : 4} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<copy.icon size={22} />}
            title={q ? 'Sin resultados' : copy.vacio}
            body={q ? 'Prueba con otro término.' : copy.vacioBody}
            cta={edita && !q ? <Button icon={<Icon.Plus size={16} />} onClick={() => setEditar('nueva')}>{copy.nueva}</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 pr-3 font-medium">Lista</th>
                  <th className="py-2.5 pr-3 font-medium">Moneda</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Productos con precio</th>
                  <th className="py-2.5 pr-3 font-medium">Estado</th>
                  {edita ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                </tr>
              </thead>
              <tbody>
                {rows.map((l) => (
                  <tr key={l.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover cursor-pointer" onClick={() => setEditar(l)}>
                    <td className="py-2.5 pr-3 font-medium text-[13px]">{l.nombre}</td>
                    <td className="py-2.5 pr-3"><Badge size="sm" color="slate">{monedaLabel(l.moneda)}</Badge></td>
                    <td className="py-2.5 pr-3 text-right num text-[12.5px] text-slate-500">{fmtNum((l.items || []).length, 0)}</td>
                    <td className="py-2.5 pr-3">
                      {l.activa ? <Badge size="sm" color="emerald" dot>Activa</Badge> : <Badge size="sm" color="slate">Inactiva</Badge>}
                    </td>
                    {edita ? (
                      <td className="py-2.5 pr-3 text-right" onClick={(e) => e.stopPropagation()}>
                        <Button variant="ghost" size="sm" icon={<Icon.Pencil size={14} />} onClick={() => setEditar(l)}>Editar</Button>
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {rows.length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(rows.length, 0)} lista(s)</div> : null}
    </div>
  )
}

/* Formulario de alta/edición de una lista de precio, a pantalla completa.
 *
 * Muestra TODO el catálogo con su precio base y un campo editable «precio en la
 * lista» (en la moneda de la lista). Un campo vacío significa «usa el precio
 * base» (el SKU no entra en la lista); un valor —incluido 0— fija un precio
 * explícito. Guardar arma los ítems solo con los SKU que tengan precio. */
function FormLista({ lista, tipo, copy, onVolver, onSaved, toast }) {
  const { db } = useData()
  const editando = !!lista
  const monedaEmpresa = db.EMPRESA?.monedaPrincipal || 'VES'

  const productos = useMemo(() => {
    // Activos del catálogo, más cualquier SKU ya listado aunque se desactivara
    // (para no perder su precio al editar).
    // Una lista de precios es de VENTA: un insumo no tiene precio de venta.
    const activos = vendibles(db.PRODUCTOS)
    if (!lista) return activos
    const enLista = new Set((lista.items || []).map((it) => String(it.sku || '').toLowerCase()))
    const faltantes = (db.PRODUCTOS || []).filter((p) => p.activo === false && enLista.has(String(p.sku || '').toLowerCase()))
    return [...activos, ...faltantes]
  }, [db.PRODUCTOS, lista])

  const [nombre, setNombre] = useState(lista?.nombre || '')
  const [activa, setActiva] = useState(lista ? lista.activa !== false : true)
  const [moneda, setMoneda] = useState((lista?.moneda || monedaEmpresa || 'VES').toUpperCase())
  // Precios por SKU como STRING: '' = no incluido (usa precio base); cualquier
  // otro (incluido '0') = precio explícito.
  const [precios, setPrecios] = useState(() => {
    const m = {}
    for (const it of lista?.items || []) m[it.sku] = String(it.precio)
    return m
  })
  const [q, setQ] = useState('')
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)

  const setPrecio = (sku, v) => setPrecios((s) => ({ ...s, [sku]: v }))
  const conPrecio = (sku) => (precios[sku] ?? '') !== ''

  const listados = useMemo(() => Object.keys(precios).filter((sku) => (precios[sku] ?? '') !== '').length, [precios])

  const rows = useMemo(() => {
    const term = q.trim().toLowerCase()
    return productos.filter((p) => !term
      || (p.nombre || '').toLowerCase().includes(term)
      || (p.sku || '').toLowerCase().includes(term))
  }, [productos, q])

  const errNombre = !nombre.trim() ? 'Ingresa un nombre para la lista.' : ''
  const hayNegativo = Object.values(precios).some((v) => v !== '' && Number(v) < 0)
  const err = errNombre || (hayNegativo ? 'Los precios no pueden ser negativos.' : '')

  const guardar = async () => {
    setTouched(true)
    if (err) return
    setBusy(true)
    const items = productos
      .filter((p) => conPrecio(p.sku))
      .map((p) => ({ sku: p.sku, precio: Number(precios[p.sku]) || 0 }))
    const body = { nombre: nombre.trim(), tipo, activa, moneda, items }
    try {
      if (editando) {
        await api.actualizarListaPrecio(lista.id, body)
        toast({ title: 'Lista actualizada', body: nombre.trim() })
      } else {
        await api.crearListaPrecio(body)
        toast({ title: 'Lista creada', body: nombre.trim() })
      }
      await onSaved()
      onVolver()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  const acciones = (
    <>
      <Button variant="ghost" onClick={onVolver}>Cancelar</Button>
      <Button onClick={guardar} loading={busy} disabled={!!err} title={err || 'Guardar la lista'} icon={<Icon.Check size={16} />}>
        {editando ? 'Guardar cambios' : 'Crear lista'}
      </Button>
    </>
  )

  return (
    <VistaDetalle onVolver={onVolver} icon={<Icon.Tag size={18} />}
      titulo={editando ? (lista.nombre || 'Lista de precio') : copy.nueva}
      sub={copy.sub} acciones={acciones}>
      {/* Cabecera: nombre, moneda, estado */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 mb-4">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="md:col-span-2">
            <Field label="Nombre de la lista" required error={touched ? errNombre : ''}>
              <Input value={nombre} onChange={(e) => setNombre(e.target.value)} onBlur={() => setTouched(true)} invalid={touched && !!errNombre}
                placeholder={tipo === 'compra' ? 'Ej: Proveedor mayorista' : 'Ej: Mayorista, Distribuidor…'} autoFocus />
            </Field>
          </div>
          <Field label="Moneda" hint="de los precios de la lista">
            <Select value={moneda} onChange={(e) => setMoneda(e.target.value)}>
              {MONEDAS.map((m) => <option key={m} value={m}>{monedaLabel(m)} · {MONEDAS_META[m].nombre}</option>)}
            </Select>
          </Field>
        </div>
        <div className="mt-4 pt-4 border-t border-slate-100 dark:border-slate-800">
          <Toggle checked={activa} onChange={setActiva}
            label={activa ? 'Activa' : 'Inactiva'}
            sub={activa
              ? (tipo === 'venta' ? 'Disponible para elegir al cotizar/facturar.' : 'Disponible como tarifa de compra.')
              : 'Desactivada: se conserva, no se ofrece al operar.'} />
        </div>
      </div>

      {/* Tabla de productos: precio base vs precio en la lista */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
        <div className="flex items-center gap-2 flex-wrap mb-3">
          <label className="text-[12px] font-medium text-slate-500">Precios por producto</label>
          <span className="text-[11.5px] text-slate-400">· {fmtNum(listados, 0)} con precio propio · el resto usa el precio base</span>
          <Input className="w-60 ml-auto" icon={<Icon.Search size={15} />} placeholder="Buscar producto…" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>

        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          {rows.length === 0 ? (
            <div className="px-4 py-10 text-center text-[13px] text-slate-500">{q ? `Sin resultados para “${q}”.` : 'No hay productos en el catálogo.'}</div>
          ) : (
            <table className="w-full text-sm min-w-[640px]">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2 px-3 font-medium">Producto</th>
                  <th className="py-2 pr-3 font-medium text-right w-40">Precio base (catálogo)</th>
                  <th className="py-2 pr-3 font-medium text-right w-48">{copy.colLista} ({monedaLabel(moneda)})</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((p) => {
                  const activo = conPrecio(p.sku)
                  return (
                    <tr key={p.id} className="border-b border-slate-100 dark:border-slate-800/70">
                      <td className="py-2 px-3">
                        <div className="font-medium text-[13px] flex items-center gap-2">
                          {p.nombre}
                          {p.activo === false ? <Badge size="sm" color="slate">Inactivo</Badge> : null}
                          {activo ? <Badge size="sm" color="teal">en lista</Badge> : null}
                        </div>
                        <div className="text-[11px] text-slate-400 num">{p.sku}{p.exentoIva ? ' · exento' : ''}</div>
                      </td>
                      <td className="py-2 pr-3 text-right num text-[12.5px] text-slate-500 private-mask">
                        {fmtCurrency(p.precio, monedaDe(p, monedaEmpresa))}
                      </td>
                      <td className="py-2 pr-3 text-right">
                        <div className="inline-flex items-center gap-1.5 justify-end">
                          <input type="number" min="0" step="0.01" value={precios[p.sku] ?? ''}
                            onChange={(e) => setPrecio(p.sku, e.target.value)}
                            placeholder="base"
                            className={`w-28 h-8 text-right px-2 rounded-lg border text-sm num ring-focus private-mask bg-white dark:bg-slate-900 ${activo ? 'border-teal-300 dark:border-teal-700' : 'border-slate-200 dark:border-slate-700'}`} />
                          {activo ? (
                            <button type="button" onClick={() => setPrecio(p.sku, '')} title="Quitar de la lista (volver al precio base)"
                              className="h-7 w-7 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800"><Icon.X size={14} /></button>
                          ) : <span className="w-7" />}
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>
        <div className="mt-2 text-[11.5px] text-slate-400">Deja un precio en blanco para que ese producto use su precio base del catálogo.</div>
      </div>

      {err && touched ? (
        <div className="mt-3 flex items-start gap-1.5 text-[12px] text-amber-700 dark:text-amber-400">
          <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" /><span>{err}</span>
        </div>
      ) : null}
    </VistaDetalle>
  )
}
