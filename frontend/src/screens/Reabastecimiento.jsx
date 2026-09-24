import { useState, useEffect, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Input, Select, Empty, TableSkeleton, useToast, Modal, Field, Badge } from '../components/primitives.jsx'
import { fmtNum } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { api } from '../lib/api.js'

/* REABASTECIMIENTO.
 *
 * Reponer a ojo funciona con veinte productos y deja de funcionar con doscientos:
 * lo que se agota es siempre lo que nadie estaba mirando.
 *
 * LA REGLA NO COMPRA. Prepara una solicitud de presupuesto y una persona decide.
 * Por eso la pantalla enseña PRIMERO lo que se pediría y por qué —lo que hay, lo
 * comprometido y lo que viene en camino— y solo después ofrece generarla. Una
 * sugerencia de compra que aparece ya convertida en documento no se lee: se firma.
 */
export function Reabastecimiento() {
  const { db } = useData()
  const toast = useToast()
  const [reglas, setReglas] = useState(null)
  const [rev, setRev] = useState(null)
  const [form, setForm] = useState(null)
  const [busy, setBusy] = useState(false)

  const productos = (db.PRODUCTOS || []).filter((p) => !p.esCombo && !p.esPlato)
  const proveedores = db.PROVEEDORES || []

  const cargar = async () => {
    try {
      const [r, v] = await Promise.all([api.reglasReabastecimiento(), api.revisarReabastecimiento()])
      setReglas(r?.reglas || [])
      setRev(v)
    } catch {
      setReglas([])
      setRev(null)
    }
  }
  useEffect(() => { cargar() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const nombreProv = useMemo(() => Object.fromEntries(proveedores.map((p) => [p.id, p.nombre])), [proveedores])

  const guardar = async () => {
    const min = Number(form.minimo), max = Number(form.maximo)
    if (!form.sku) return toast({ title: 'Elige el producto', kind: 'error' })
    if (!(max > min)) return toast({ title: 'El máximo tiene que superar al mínimo', body: 'Si fueran iguales, cada venta dispararía un pedido.', kind: 'error' })
    setBusy(true)
    try {
      const body = {
        sku: form.sku, minimo: min, maximo: max,
        multiplo: Number(form.multiplo) || 0, proveedorId: form.proveedorId || '', activa: true,
      }
      if (form.id) await api.actualizarReglaReabastecimiento(form.id, body)
      else await api.crearReglaReabastecimiento(body)
      toast({ title: form.id ? 'Regla actualizada' : 'Regla creada', body: `${form.sku}: repone de ${min} hasta ${max}.` })
      setForm(null)
      cargar()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusy(false) }
  }

  const generar = async () => {
    setBusy(true)
    try {
      const r = await api.generarReabastecimiento()
      const n = (r?.solicitudes || []).length
      toast({
        title: `${n} solicitud(es) de compra creada(s)`,
        body: 'Están en Compras › Solicitudes, listas para pedir presupuesto. No se compró nada todavía.',
      })
      cargar()
    } catch (e) {
      toast({ title: 'No se generó nada', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusy(false) }
  }

  const bajoMinimos = rev?.filas || []

  return (
    <div>
      <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-3xl mb-3">
        Define <strong>hasta cuánto</strong> repones y <strong>a partir de qué nivel</strong>. Cuando lo
        disponible baja hasta el mínimo, el producto aparece abajo con lo que habría que pedir.
        <strong> Generar no compra</strong>: prepara solicitudes de presupuesto para que alguien decida.
      </div>

      {/* Lo primero que se lee: qué está bajo mínimos y por qué. */}
      {rev === null ? null : bajoMinimos.length === 0 ? (
        <div className="mb-3 rounded-xl bg-emerald-50 dark:bg-emerald-900/25 border border-emerald-200 dark:border-emerald-900/40 px-3 py-2.5">
          <div className="flex items-start gap-2.5 text-[12.5px] text-emerald-900 dark:text-emerald-200">
            <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
            <div>
              <strong>Nada bajo mínimos.</strong> {rev.reglas === 0
                ? 'Todavía no hay reglas definidas: crea la primera abajo.'
                : `Las ${rev.reglas} regla(s) están cubiertas, contando lo que ya viene en camino.`}
            </div>
          </div>
        </div>
      ) : (
        <div className="mb-3 rounded-xl border border-amber-200 dark:border-amber-900/40 overflow-hidden">
          <div className="px-3 py-2.5 bg-amber-50 dark:bg-amber-900/25 text-[12.5px] text-amber-900 dark:text-amber-200 flex items-center gap-3 flex-wrap">
            <Icon.CircleAlert size={15} className="shrink-0" />
            <strong>{bajoMinimos.length} producto(s) bajo mínimos.</strong>
            <span className="opacity-90">Se descuenta lo apartado y lo que ya está pedido al proveedor.</span>
            <Button size="sm" className="ml-auto" onClick={generar} disabled={busy}>
              {busy ? 'Generando…' : 'Generar solicitudes de compra'}
            </Button>
          </div>
          <div className="overflow-x-auto bg-white dark:bg-slate-900">
            <table className="w-full text-[13px]">
              <thead className="text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800">
                <tr>
                  <th className="py-2 pl-4 pr-3 text-left font-medium">Producto</th>
                  <th className="py-2 pr-3 text-right font-medium">En almacén</th>
                  <th className="py-2 pr-3 text-right font-medium">Apartado</th>
                  <th className="py-2 pr-3 text-right font-medium">En camino</th>
                  <th className="py-2 pr-3 text-right font-medium">Cobertura</th>
                  <th className="py-2 pr-3 text-right font-medium">Mín / Máx</th>
                  <th className="py-2 pr-3 text-right font-medium">A pedir</th>
                  <th className="py-2 pr-4 text-left font-medium">Proveedor</th>
                </tr>
              </thead>
              <tbody>
                {bajoMinimos.map((f) => (
                  <tr key={f.sku} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2 pl-4 pr-3">
                      {f.nombre}<span className="text-slate-400 text-[11.5px]"> · {f.sku}</span>
                      {f.yaSolicitado ? (
                        <Badge size="sm" color="slate" className="ml-1.5">ya solicitado</Badge>
                      ) : null}
                    </td>
                    <td className="py-2 pr-3 text-right num text-slate-500">{fmtNum(f.existencia)}</td>
                    <td className="py-2 pr-3 text-right num text-slate-500">{f.apartado ? fmtNum(f.apartado) : '—'}</td>
                    <td className="py-2 pr-3 text-right num text-slate-500">{f.enCamino ? fmtNum(f.enCamino) : '—'}</td>
                    <td className="py-2 pr-3 text-right num font-medium">{fmtNum(f.cobertura)}</td>
                    <td className="py-2 pr-3 text-right num text-slate-500">{fmtNum(f.minimo)} / {fmtNum(f.maximo)}</td>
                    <td className="py-2 pr-3 text-right num font-semibold text-amber-700 dark:text-amber-400">{fmtNum(f.aPedir)}</td>
                    <td className="py-2 pr-4 text-[12.5px] text-slate-500">{f.proveedor || <span className="text-slate-400">sin asignar</span>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <div className="flex items-center justify-between gap-3 mb-3">
        <div className="text-[12.5px] text-slate-500">Reglas definidas</div>
        <Button size="sm" icon={<Icon.Plus size={15} />}
          onClick={() => setForm({ sku: '', minimo: '', maximo: '', multiplo: '', proveedorId: '' })}>
          Nueva regla
        </Button>
      </div>

      {reglas === null ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={4} cols={5} /></div>
      ) : reglas.length === 0 ? (
        <Empty icon={<Icon.Boxes size={22} />} title="Sin reglas de reposición"
          body="Define el mínimo y el máximo de los productos que no pueden faltar. El resto sigue reponiéndose a mano." />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <table className="w-full text-[13px]">
            <thead className="bg-slate-50 dark:bg-slate-800/60 text-[11.5px] uppercase tracking-wide text-slate-500">
              <tr>
                <th className="text-left px-4 py-2">Producto</th>
                <th className="text-right px-4 py-2">Mínimo</th>
                <th className="text-right px-4 py-2">Máximo</th>
                <th className="text-right px-4 py-2">Múltiplo</th>
                <th className="text-left px-4 py-2">Proveedor</th>
                <th className="px-4 py-2" />
              </tr>
            </thead>
            <tbody>
              {reglas.map((r) => (
                <tr key={r.id} className={`border-t border-slate-100 dark:border-slate-800 ${r.activa ? '' : 'opacity-60'}`}>
                  <td className="px-4 py-2 num text-[12.5px]">{r.sku}</td>
                  <td className="px-4 py-2 text-right num">{fmtNum(r.minimo)}</td>
                  <td className="px-4 py-2 text-right num">{fmtNum(r.maximo)}</td>
                  <td className="px-4 py-2 text-right num text-slate-500">{r.multiplo ? fmtNum(r.multiplo) : '—'}</td>
                  <td className="px-4 py-2 text-[12.5px] text-slate-500">{nombreProv[r.proveedorId] || <span className="text-slate-400">sin asignar</span>}</td>
                  <td className="px-4 py-2 text-right">
                    <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />}
                      onClick={() => setForm({ ...r, minimo: String(r.minimo), maximo: String(r.maximo), multiplo: String(r.multiplo || '') })}>
                      Editar
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {form ? (
        <Modal open onClose={() => setForm(null)} title={form.id ? `Regla de ${form.sku}` : 'Nueva regla de reposición'}>
          <div className="space-y-3">
            {form.id ? null : (
              <Field label="Producto">
                <Select value={form.sku} onChange={(e) => setForm({ ...form, sku: e.target.value })}>
                  <option value="">Elige uno…</option>
                  {productos.map((p) => <option key={p.sku} value={p.sku}>{p.sku} · {p.nombre}</option>)}
                </Select>
              </Field>
            )}
            <div className="grid grid-cols-2 gap-3">
              <Field label="Mínimo" hint="Cuando baja hasta aquí, se repone.">
                <Input type="number" min="0" step="any" value={form.minimo}
                  onChange={(e) => setForm({ ...form, minimo: e.target.value })} />
              </Field>
              <Field label="Máximo" hint="Hasta dónde se repone.">
                <Input type="number" min="0" step="any" value={form.maximo}
                  onChange={(e) => setForm({ ...form, maximo: e.target.value })} />
              </Field>
            </div>
            <Field label="Múltiplo de compra" hint="Si el proveedor solo vende cajas de 12, pon 12: pedir 7 será pedir 12. Vacío = sin restricción.">
              <Input type="number" min="0" step="any" value={form.multiplo}
                onChange={(e) => setForm({ ...form, multiplo: e.target.value })} />
            </Field>
            <Field label="Proveedor habitual" hint="Agrupa la solicitud. Sin asignar va en una solicitud aparte, para decidir a quién pedirle.">
              <Select value={form.proveedorId || ''} onChange={(e) => setForm({ ...form, proveedorId: e.target.value })}>
                <option value="">Sin asignar</option>
                {proveedores.map((p) => <option key={p.id} value={p.id}>{p.nombre}</option>)}
              </Select>
            </Field>
            {form.id ? (
              <label className="flex items-center gap-2 text-[13px]">
                <input type="checkbox" checked={form.activa !== false}
                  onChange={(e) => setForm({ ...form, activa: e.target.checked })} />
                Activa
              </label>
            ) : null}
            <div className="flex justify-end gap-2 pt-1">
              <Button variant="ghost" onClick={() => setForm(null)}>Cancelar</Button>
              <Button onClick={guardar} disabled={busy}>{busy ? 'Guardando…' : 'Guardar'}</Button>
            </div>
          </div>
        </Modal>
      ) : null}
    </div>
  )
}
