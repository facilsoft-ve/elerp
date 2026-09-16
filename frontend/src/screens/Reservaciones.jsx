/* Reservaciones del salón.
 *
 * Dos usos distintos en una sola pantalla, porque son el mismo día de trabajo:
 *
 *  · TOMAR la reserva (por teléfono o en el mostrador): día, hora, cuántos son, a
 *    nombre de quién y, si hace falta, qué mesa o qué zona se le aparta.
 *  · RECIBIR en la puerta: alguien llega y dice un nombre. Se busca por nombre o por
 *    cédula —escrita como sea— y se le SIENTA, que abre su cuenta de mesa.
 *
 * Lo que hace que la reserva sirva es que la mesa esté libre a la hora. Por eso el
 * tablero de la comandera pinta como «reservada» la mesa apartada dentro de la ventana
 * (ver MesasReservadasAhora en el backend) y acá se avisa cuál es.
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Input, Select, Field, Badge, Empty, Modal, useToast, useConfirm, TableSkeleton } from '../components/primitives.jsx'
import { api } from '../lib/api.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'

const ESTADO = {
  pendiente: { label: 'Esperando', color: 'huberp' },
  sentada: { label: 'Sentada', color: 'teal' },
  no_llego: { label: 'No llegó', color: 'amber' },
  cancelada: { label: 'Cancelada', color: 'slate' },
}

/** hoyISO es la fecha de hoy en el huso del equipo (que es el del local). */
export function hoyISO(d = new Date()) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** fechaLarga escribe "jueves 16 de septiembre" para el encabezado del día. */
export function fechaLarga(iso) {
  const [a, m, d] = String(iso || '').split('-').map(Number)
  if (!a || !m || !d) return ''
  return new Date(a, m - 1, d).toLocaleDateString('es-VE', {
    weekday: 'long', day: 'numeric', month: 'long',
  })
}

export function Reservaciones() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const confirm = useConfirm()
  const mesas = useMemo(() => (db.MESAS || []).filter((m) => m.activa !== false), [db.MESAS])
  const zonas = useMemo(
    () => [...new Set(mesas.map((m) => (m.zona || '').trim()).filter(Boolean))].sort((a, b) => a.localeCompare(b, 'es')),
    [mesas],
  )

  const [fecha, setFecha] = useState(hoyISO())
  const [q, setQ] = useState('')
  const [lista, setLista] = useState(null) // null = cargando
  const [form, setForm] = useState(null)   // null | {} (nueva) | reserva (editar)
  const [sentando, setSentando] = useState(null)

  const cargar = useCallback(async () => {
    try { setLista(await api.reservas({ fecha, q })) }
    catch (e) { toast({ title: 'No se pudo cargar la agenda', body: e?.message || 'Error', kind: 'error' }); setLista([]) }
  }, [fecha, q, toast])

  useEffect(() => {
    // Se espera un momento entre teclas: la búsqueda de la puerta se escribe entera.
    const t = setTimeout(cargar, q ? 250 : 0)
    return () => clearTimeout(t)
  }, [cargar, q])

  const sentar = async (r, mesaId) => {
    try {
      await api.sentarReserva(r.id, mesaId || '')
      toast({ title: `${r.nombre} está en la mesa`, body: 'Se abrió su cuenta: ya puedes tomarle el pedido.' })
      setSentando(null)
      cargar(); reload()
    } catch (e) { toast({ title: 'No se pudo sentar', body: e?.message || 'Error', kind: 'error' }) }
  }

  const cambiarEstado = async (r, estado, texto) => {
    if (estado !== 'pendiente' && !(await confirm({
      title: texto, body: `La reserva de ${r.nombre} (${r.hora}) queda marcada así y su mesa se libera.`,
      confirmLabel: texto, tone: estado === 'cancelada' ? 'danger' : 'warn',
    }))) return
    try { await api.estadoReserva(r.id, estado); cargar() }
    catch (e) { toast({ title: 'No se pudo', body: e?.message || 'Error', kind: 'error' }) }
  }

  const esHoy = fecha === hoyISO()
  const pendientes = (lista || []).filter((r) => r.estado === 'pendiente')

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-2">
        <Field label="Día" className="!mb-0">
          <Input type="date" value={fecha} onChange={(e) => setFecha(e.target.value || hoyISO())} className="!w-44" />
        </Field>
        <div className="flex-1 min-w-[220px]">
          <Field label="Buscar en la puerta" hint="Por nombre o por cédula, escrita como sea." className="!mb-0">
            <Input icon={<Icon.Search size={16} />} value={q} onChange={(e) => setQ(e.target.value)}
              placeholder="Ana Pérez · V-12.345.678" />
          </Field>
        </div>
        <Button size="lg" icon={<Icon.Plus size={17} />} onClick={() => setForm({})}>Nueva reserva</Button>
      </div>

      <div className="flex items-baseline gap-2">
        <div className="font-display font-semibold text-[15px] capitalize">{fechaLarga(fecha)}</div>
        <div className="text-[12.5px] text-slate-500">
          {lista === null ? '—' : `${pendientes.length} esperando · ${lista.length} en la agenda`}
        </div>
      </div>

      {lista === null ? <TableSkeleton filas={4} /> : lista.length === 0 ? (
        <Empty icon={<Icon.Users size={22} />}
          title={q ? `Nadie con «${q}» en este día` : 'Sin reservas para este día'}
          body={q ? 'Revisa el nombre o pide la cédula: se busca por cualquiera de los dos.'
            : 'Toma la primera reserva y aparece acá, ordenada por hora.'}
          cta={q ? null : <Button icon={<Icon.Plus size={16} />} onClick={() => setForm({})}>Nueva reserva</Button>} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card divide-y divide-slate-100 dark:divide-slate-800">
          {lista.map((r) => {
            const est = ESTADO[r.estado] || ESTADO.pendiente
            return (
              <div key={r.id} className="flex flex-wrap items-center gap-3 px-3.5 py-3">
                <div className="w-14 shrink-0">
                  <div className="font-display font-bold text-[17px] tabular-nums leading-none">{r.hora}</div>
                  <div className="text-[11px] text-slate-400 mt-1">{r.personas} pers.</div>
                </div>
                <div className="min-w-0 flex-1">
                  <div className="font-medium text-[14px] truncate">{r.nombre}</div>
                  <div className="text-[11.5px] text-slate-500 truncate">
                    {r.documento || 'sin cédula'}
                    {r.telefono ? ` · ${r.telefono}` : ''}
                    {r.mesaNombre ? ` · mesa ${r.mesaNombre}` : r.zona ? ` · ${r.zona}` : ' · sin mesa asignada'}
                  </div>
                  {r.nota ? <div className="text-[11.5px] text-slate-400 truncate">{r.nota}</div> : null}
                </div>
                <Badge size="sm" color={est.color}>{est.label}</Badge>
                {r.estado === 'pendiente' ? (
                  <div className="flex items-center gap-1.5">
                    <Button size="sm" icon={<Icon.Check size={14} />}
                      onClick={() => (r.mesaId ? sentar(r, r.mesaId) : setSentando(r))}>Llegó</Button>
                    <Button size="sm" variant="ghost" onClick={() => setForm(r)} title="Editar la reserva">
                      <Icon.Pencil size={14} />
                    </Button>
                    {esHoy ? (
                      <Button size="sm" variant="ghost" onClick={() => cambiarEstado(r, 'no_llego', 'Marcar que no llegó')}
                        title="No llegó">
                        <Icon.Clock size={14} />
                      </Button>
                    ) : null}
                    <Button size="sm" variant="ghost" onClick={() => cambiarEstado(r, 'cancelada', 'Cancelar la reserva')}
                      title="Cancelar">
                      <Icon.Trash size={14} />
                    </Button>
                  </div>
                ) : r.estado === 'sentada' ? (
                  <div className="text-[11.5px] text-slate-400">en la mesa {r.mesaNombre}</div>
                ) : (
                  <Button size="sm" variant="ghost" onClick={() => cambiarEstado(r, 'pendiente', 'Devolver a la agenda')}>
                    Reactivar
                  </Button>
                )}
              </div>
            )
          })}
        </div>
      )}

      {form ? (
        <ReservaModal reserva={form.id ? form : null} mesas={mesas} zonas={zonas} fechaPorDefecto={fecha}
          onClose={() => setForm(null)}
          onGuardada={() => { setForm(null); cargar() }} toast={toast} rol={ui.rol} />
      ) : null}

      {sentando ? (
        <ElegirMesaModal reserva={sentando} mesas={mesas} onClose={() => setSentando(null)}
          onElegida={(mesaId) => sentar(sentando, mesaId)} />
      ) : null}
    </div>
  )
}

/* Tomar o editar una reserva. */
function ReservaModal({ reserva, mesas, zonas, fechaPorDefecto, onClose, onGuardada, toast }) {
  const editar = !!reserva
  const [f, setF] = useState(() => ({
    fecha: reserva?.fecha || fechaPorDefecto || hoyISO(),
    hora: reserva?.hora || '20:00',
    personas: reserva?.personas || 2,
    nombre: reserva?.nombre || '',
    documento: reserva?.documento || '',
    telefono: reserva?.telefono || '',
    mesaId: reserva?.mesaId || '',
    zona: reserva?.zona || '',
    nota: reserva?.nota || '',
  }))
  const [busy, setBusy] = useState(false)
  const set = (k, v) => setF((s) => ({ ...s, [k]: v }))

  // Mesas que caben: reservar una mesa de 2 para 6 personas se descubre con la gente
  // parada en la puerta, así que ni se ofrece.
  const caben = mesas.filter((m) => !m.capacidad || m.capacidad >= (Number(f.personas) || 1))

  const guardar = async () => {
    if (!f.nombre.trim()) { toast({ title: '¿A nombre de quién?', kind: 'warn' }); return }
    setBusy(true)
    const body = { ...f, personas: Number(f.personas) || 1 }
    try {
      if (editar) await api.actualizarReserva(reserva.id, body)
      else await api.crearReserva(body)
      toast({ title: editar ? 'Reserva actualizada' : 'Reserva tomada', body: `${f.nombre} · ${f.fecha} ${f.hora}` })
      onGuardada()
    } catch (e) { toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' }); setBusy(false) }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.Users size={18} />}
      title={editar ? `Reserva de ${reserva.nombre}` : 'Nueva reserva'}
      sub="Con el nombre y la cédula se verifica en la puerta a quien llega."
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button loading={busy} onClick={guardar}>{editar ? 'Guardar' : 'Tomar reserva'}</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="grid grid-cols-3 gap-2">
          <Field label="Día"><Input type="date" value={f.fecha} onChange={(e) => set('fecha', e.target.value)} /></Field>
          <Field label="Hora"><Input type="time" value={f.hora} onChange={(e) => set('hora', e.target.value)} /></Field>
          <Field label="Personas"><Input type="number" min={1} value={f.personas} onChange={(e) => set('personas', e.target.value)} /></Field>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <Field label="A nombre de"><Input autoFocus value={f.nombre} onChange={(e) => set('nombre', e.target.value)} placeholder="Ana Pérez" /></Field>
          <Field label="Cédula o RIF" hint="Es lo que se verifica al llegar.">
            <Input value={f.documento} onChange={(e) => set('documento', e.target.value)} placeholder="V-12345678" />
          </Field>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <Field label="Teléfono"><Input value={f.telefono} onChange={(e) => set('telefono', e.target.value)} placeholder="0414-1234567" /></Field>
          <Field label="Zona" hint="Si no eliges mesa, se aparta el área.">
            <Select value={f.zona} onChange={(e) => set('zona', e.target.value)}>
              <option value="">Cualquiera</option>
              {zonas.map((z) => <option key={z} value={z}>{z}</option>)}
            </Select>
          </Field>
        </div>
        <Field label="Mesa" hint="Opcional: apartar una mesa concreta. Solo se ofrecen las que caben.">
          <Select value={f.mesaId} onChange={(e) => set('mesaId', e.target.value)}>
            <option value="">Se decide al llegar</option>
            {caben.map((m) => (
              <option key={m.id} value={m.id}>Mesa {m.nombre}{m.zona ? ` · ${m.zona}` : ''} ({m.capacidad || '?'} pers.)</option>
            ))}
          </Select>
        </Field>
        <Field label="Nota" hint="Cumpleaños, silla para bebé, alergias…">
          <Input value={f.nota} onChange={(e) => set('nota', e.target.value)} />
        </Field>
      </div>
    </Modal>
  )
}

/* Elegir la mesa al recibir a una reserva que solo apartó zona. */
function ElegirMesaModal({ reserva, mesas, onClose, onElegida }) {
  const caben = mesas.filter((m) => !m.capacidad || m.capacidad >= (reserva.personas || 1))
  const deLaZona = reserva.zona
    ? caben.filter((m) => (m.zona || '').trim().toLowerCase() === reserva.zona.trim().toLowerCase())
    : caben
  const lista = deLaZona.length ? deLaZona : caben
  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Utensils size={18} />}
      title={`¿Dónde sentamos a ${reserva.nombre}?`}
      sub={`${reserva.personas} persona(s)${reserva.zona ? ` · reservó ${reserva.zona}` : ''}`}
      footer={<Button variant="ghost" onClick={onClose}>Cancelar</Button>}>
      {lista.length === 0 ? (
        <Empty icon={<Icon.Utensils size={20} />} title="Ninguna mesa tiene ese tamaño"
          body="Junta mesas o cambia la cantidad de personas en la reserva." framed={false} />
      ) : (
        <div className="grid grid-cols-3 gap-2">
          {lista.map((m) => (
            <button key={m.id} onClick={() => onElegida(m.id)}
              className="rounded-xl border border-slate-200 dark:border-slate-700 px-3 py-2.5 text-left hover:border-elerp-400 hover:bg-elerp-50/60 dark:hover:bg-elerp-900/25 ring-focus">
              <div className="font-display font-bold text-[16px]">{m.nombre}</div>
              <div className="text-[11.5px] text-slate-500">{m.zona || '—'} · {m.capacidad || '?'} pers.</div>
            </button>
          ))}
        </div>
      )}
    </Modal>
  )
}
