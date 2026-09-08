import { useState, useEffect, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Modal, Field, Input, useToast, Badge } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { api } from '../lib/api.js'

/* Ventas en espera (flujo 2.6).
 *
 * El caso real del mostrador: «voy al cajero automático y vuelvo». El carrito se
 * aparta con una nota para reconocerlo y la cola sigue avanzando.
 *
 * Dos decisiones que se ven en el comportamiento:
 *   · vive en el SERVIDOR, no en el navegador — sobrevive a que se recargue la
 *     página o se apague la tablet, y la puede retomar otro cajero de la sede,
 *     que es lo que declara el prototipo;
 *   · retomarla la CONSUME: dos cajeros no pueden cobrar el mismo carrito.
 */

// useEspera centraliza la lista para que el POS y el Modo caja compartan estado.
export function useEspera() {
  const [lista, setLista] = useState([])
  const [cargando, setCargando] = useState(false)

  const recargar = useCallback(async () => {
    setCargando(true)
    try {
      setLista(await api.ventasEnEspera() || [])
    } catch {
      // Sin conexión no se puede saber: se muestra vacío y el cajero sigue
      // cobrando, en vez de bloquear el mostrador por una lista secundaria.
      setLista([])
    } finally {
      setCargando(false)
    }
  }, [])

  useEffect(() => { recargar() }, [recargar])
  return { lista, cargando, recargar }
}

/* Modal para apartar el carrito, con la nota opcional del prototipo. */
export function DejarEnEsperaModal({ open, onClose, lineas, clienteId, onGuardada }) {
  const toast = useToast()
  const [nota, setNota] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => { if (open) setNota('') }, [open])

  const guardar = async () => {
    setBusy(true)
    try {
      await api.dejarEnEspera({
        nota: nota.trim(),
        clienteId: clienteId || '',
        lineas: lineas.map((l) => ({ sku: l.sku, cantidad: Number(l.cantidad), precioUnitario: Number(l.precioUnitario) })),
      })
      toast({ title: 'Venta en espera', body: nota.trim() || 'Puedes retomarla cuando el cliente vuelva.' })
      onGuardada()
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo apartar', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open={open} onClose={onClose} size="sm" icon={<Icon.Clock size={18} />}
      title="Dejar venta en espera" sub={`${fmtNum(lineas.length, 0)} ítem(s) · sigue atendiendo la cola`}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Clock size={16} />}>Dejar en espera</Button>
      </>}>
      <div className="space-y-3">
        <Field label="Nota" hint="opcional · para reconocerla">
          <Input value={nota} onChange={(e) => setNota(e.target.value)} autoFocus
            placeholder="Ej. Cliente va al cajero automático"
            onKeyDown={(e) => { if (e.key === 'Enter') guardar() }} />
        </Field>
        <div className="text-[12px] text-slate-500">
          El carrito queda guardado en el servidor, atado a esta sede. Cualquier cajero puede retomarlo;
          al retomarlo desaparece de la lista, para que nadie lo cobre dos veces.
        </div>
      </div>
    </Modal>
  )
}

/* Lista de ventas apartadas: retomar o descartar. */
export function EsperaModal({ open, onClose, lista, onRetomada, onCambio }) {
  const toast = useToast()
  const [busy, setBusy] = useState('')

  const retomar = async (v) => {
    setBusy(v.id)
    try {
      const venta = await api.retomarVenta(v.id)
      onRetomada(venta)
      onClose()
    } catch (e) {
      toast({ title: 'No se pudo retomar', body: e?.message || 'Error', kind: 'error' })
      onCambio()
    } finally { setBusy('') }
  }

  const descartar = async (v) => {
    setBusy(v.id)
    try {
      await api.descartarVenta(v.id)
      toast({ title: 'Venta descartada', body: v.nota || `${fmtNum(v.lineas?.length || 0, 0)} ítem(s)` })
      onCambio()
    } catch (e) {
      toast({ title: 'No se pudo descartar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusy('') }
  }

  return (
    <Modal open={open} onClose={onClose} size="sm" icon={<Icon.Clock size={18} />}
      title="Ventas en espera" sub="Cualquier cajero de esta sede puede retomarlas."
      footer={<Button variant="ghost" onClick={onClose}>Cerrar</Button>}>
      {lista.length === 0 ? (
        <div className="py-8 text-center text-[13px] text-slate-500">No hay ventas en espera.</div>
      ) : (
        <div className="space-y-2">
          {lista.map((v) => (
            <div key={v.id} className="rounded-xl border border-slate-200 dark:border-slate-700 p-3">
              <div className="flex items-start gap-3">
                <div className="flex-1 min-w-0">
                  <div className="text-[13.5px] font-semibold truncate">
                    {v.nota || `${fmtNum(v.lineas?.length || 0, 0)} ítem(s)`}
                  </div>
                  <div className="text-[11.5px] text-slate-500 mt-0.5">
                    {v.cajeroNombre ? `${v.cajeroNombre} · ` : ''}{fmtDate(v.creada)}
                  </div>
                  <div className="text-[11.5px] text-slate-400 truncate mt-0.5">
                    {(v.lineas || []).map((l) => `${fmtNum(l.cantidad, l.cantidad % 1 ? 2 : 0)}× ${l.nombre}`).join(' · ')}
                  </div>
                </div>
                <div className="text-right shrink-0">
                  <div className="num text-[14px] font-semibold">{fmtCurrency(v.total, 'VES')}</div>
                  <Badge size="sm" color="slate">sin IVA</Badge>
                </div>
              </div>
              <div className="flex items-center gap-2 mt-2.5">
                <Button size="sm" variant="dinero" loading={busy === v.id} onClick={() => retomar(v)}
                  icon={<Icon.Cart size={15} />}>Retomar y cobrar</Button>
                <Button size="sm" variant="ghost" onClick={() => descartar(v)} disabled={busy === v.id}>Descartar</Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </Modal>
  )
}
