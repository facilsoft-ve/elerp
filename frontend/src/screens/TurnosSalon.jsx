import { useState, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Empty, Modal, Field, Input, Select, useToast, useConfirm } from '../components/primitives.jsx'
import { PedirPin } from '../components/pin.jsx'
import { useRecurso, EstadoRecurso } from '../lib/useRecurso.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { fmtNum } from '../lib/format.js'

/* TURNOS DEL SALÓN — la pantalla de entrada del mesonero y su supervisión.
 *
 * Reemplaza el correo y la contraseña por dos gestos: tocar tu foto y teclear un
 * PIN. La pantalla NO decide nada — quién puede trabajar lo resuelve el servidor
 * (application/mesonero.go): sin turno validado por un supervisor, el PIN no
 * abre nada. Acá solo se recogen los dígitos y se muestran los estados.
 *
 * Está pensada para una TABLET, de pie: tarjetas grandes, teclado numérico,
 * nada de formularios largos en el camino de entrar a trabajar. Lo de
 * administrar (crear credenciales, forzar cierres) queda debajo y solo lo ve
 * quien administra.
 */

const ADMIN = ['dueno', 'desarrollador']

export function TurnosSalon() {
  const { ui } = useUI()
  const esAdmin = ADMIN.includes(ui.rol)
  const { data, loading, error, reload } = useRecurso(() => api.mesonerosSalon(), [])
  const mesoneros = data?.mesoneros || []

  return (
    <EstadoRecurso loading={loading} error={error} onRetry={reload} rows={3} cols={3}
      title="No se pudieron cargar los mesoneros">
      <div className="space-y-6">
        <GrillaEntrada mesoneros={mesoneros} esAdmin={esAdmin} onCambio={reload} />
        {esAdmin ? <Credenciales mesoneros={mesoneros} onCambio={reload} /> : null}
        {esAdmin ? <Historial /> : null}
      </div>
    </EstadoRecurso>
  )
}

/* --- La grilla de entrada -------------------------------------------------- */

// estadoDe resume en una línea qué le pasa a cada quien. El orden importa: «sin
// PIN» manda sobre todo lo demás porque es lo único que hay que resolver antes.
function estadoDe(m) {
  if (!m.activo) return { chip: 'slate', txt: 'Inactivo', accion: null }
  if (!m.tienePin) return { chip: 'amber', txt: 'Sin PIN', accion: 'crear-pin' }
  if (m.turno?.estado === 'cerrando') return { chip: 'amber', txt: 'Cerrando turno', accion: null }
  if (m.turno?.estado === 'abierto') return { chip: 'teal', txt: 'En turno', accion: null }
  return { chip: 'slate', txt: 'Fuera de turno', accion: 'iniciar' }
}

// iniciales saca 1–2 letras del nombre para el círculo. Con foto real esto se
// reemplaza; mientras tanto, dos letras grandes se reconocen de lejos.
function iniciales(nombre) {
  const partes = String(nombre || '').trim().split(/\s+/).filter(Boolean)
  if (partes.length === 0) return '?'
  if (partes.length === 1) return partes[0].slice(0, 2).toUpperCase()
  return (partes[0][0] + partes[1][0]).toUpperCase()
}

function GrillaEntrada({ mesoneros, esAdmin, onCambio }) {
  const toast = useToast()
  // `flujo` es el paso del modal de entrada: null = nadie tocó nada.
  const [flujo, setFlujo] = useState(null)
  const activos = mesoneros.filter((m) => m.activo)

  const alTocar = (m) => {
    const e = estadoDe(m)
    if (e.accion === 'crear-pin') { setFlujo({ paso: 'pin-nuevo', m }); return }
    if (e.accion === 'iniciar') { setFlujo({ paso: 'pin-propio', m }); return }
    // Ya está en turno: desde acá solo se ve; terminarlo es del supervisor.
    toast({ title: `${m.nombre} ya está en turno`, body: 'El fin de turno lo ordena un supervisor.' })
  }

  if (activos.length === 0) {
    return (
      <Empty icon={<Icon.Users size={22} />} title="Todavía no hay mesoneros"
        body={esAdmin
          ? 'Da de alta a cada persona del salón acá abajo. Después ella misma elige su PIN en esta tablet, con un supervisor autorizando.'
          : 'Pídele a quien administra que dé de alta al personal del salón.'} />
    )
  }

  return (
    <div>
      <div className="flex items-baseline justify-between gap-3 mb-3">
        <div>
          <div className="text-[14px] font-semibold">¿Quién entra a trabajar?</div>
          <div className="text-[12.5px] text-slate-500">
            Toca tu nombre y teclea tu PIN. Un supervisor autoriza el inicio de cada turno.
          </div>
        </div>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
        {activos.map((m) => {
          const e = estadoDe(m)
          const enTurno = !!m.turno
          return (
            <button key={m.id} type="button" onClick={() => alTocar(m)}
              className={`text-left rounded-2xl border p-4 transition shadow-card bg-white dark:bg-slate-900
                ${enTurno ? 'border-teal-400/60' : 'border-slate-200 dark:border-slate-800'}
                hover:border-elerp-400 hover:shadow-md focus:outline-none focus:ring-2 focus:ring-elerp-400`}>
              <div className="flex items-center gap-3">
                <span className={`h-12 w-12 shrink-0 rounded-full inline-flex items-center justify-center text-[15px] font-bold
                  ${enTurno ? 'bg-teal-500 text-white' : 'bg-slate-100 dark:bg-slate-800 text-slate-500'}`}>
                  {iniciales(m.nombre)}
                </span>
                <div className="min-w-0">
                  <div className="font-semibold text-[14px] truncate">{m.nombre}</div>
                  <div className="text-[11.5px] text-slate-400 num">{m.codigo}</div>
                </div>
              </div>
              <div className="mt-3 flex items-center gap-1.5 flex-wrap">
                <Badge size="sm" color={e.chip} dot>{e.txt}</Badge>
                {m.mesasAbiertas > 0 ? (
                  <Badge size="sm" color="slate">{m.mesasAbiertas} mesa{m.mesasAbiertas === 1 ? '' : 's'}</Badge>
                ) : null}
              </div>
            </button>
          )
        })}
      </div>

      {esAdmin ? <TurnosVivos onCambio={onCambio} /> : null}

      {flujo ? (
        <ModalEntrada flujo={flujo} setFlujo={setFlujo}
          onListo={() => { setFlujo(null); onCambio() }} />
      ) : null}
    </div>
  )
}

/* ModalEntrada encadena los pasos del PIN. Son siempre DOS: el de la persona y
 * el del supervisor. Esa segunda mitad es lo que impide auto-asignarse un turno,
 * así que nunca se salta. */
function ModalEntrada({ flujo, setFlujo, onListo }) {
  const toast = useToast()
  const { m } = flujo

  const cerrar = () => setFlujo(null)

  const contenido = () => {
    switch (flujo.paso) {
      // Primera vez: la persona elige su propio PIN. Nadie más lo conoce — eso
      // importa porque el turno respalda su comisión.
      case 'pin-nuevo':
        return <PedirPin icono={<Icon.Key size={22} />} titulo={`Crea tu PIN, ${m.nombre.split(' ')[0]}`}
          sub="Cuatro dígitos que solo tú conoces. Con él entras a cada turno."
          onCancelar={cerrar}
          onEnviar={(pin) => { setFlujo({ paso: 'pin-nuevo-sup', m, pin }); return Promise.resolve() }} />

      case 'pin-nuevo-sup':
        return <PedirPin tono="amber" icono={<Icon.Shield size={22} />} titulo="Que un supervisor lo autorice"
          sub={`Pídele que ingrese su PIN para registrar el de ${m.nombre}.`}
          onCancelar={cerrar}
          onEnviar={async (pinSup) => {
            await api.fijarPinMesonero(m.id, { pin: flujo.pin, pinSupervisor: pinSup })
            toast({ title: 'PIN registrado', body: `${m.nombre} ya puede iniciar turno.` })
            onListo()
          }} />

      case 'pin-propio':
        return <PedirPin icono={<Icon.User size={22} />} titulo={`Hola, ${m.nombre.split(' ')[0]}`}
          sub="Teclea tu PIN para empezar tu turno."
          onCancelar={cerrar}
          onEnviar={(pin) => { setFlujo({ paso: 'pin-sup', m, pin }); return Promise.resolve() }} />

      case 'pin-sup':
        return <PedirPin tono="amber" icono={<Icon.Shield size={22} />} titulo="Falta la autorización"
          sub="Un supervisor tiene que validar el inicio del turno con su PIN."
          onCancelar={cerrar}
          onEnviar={async (pinSup) => {
            const t = await api.iniciarTurno({ mesoneroId: m.id, pin: flujo.pin, pinSupervisor: pinSup })
            toast({ title: `Turno abierto · ${m.nombre}`, body: `Validó ${t.validadoPor}.` })
            onListo()
          }} />

      default:
        return null
    }
  }

  return (
    <div className="fixed inset-0 z-[240] flex items-center justify-center p-5 bg-slate-900/55 backdrop-blur-sm"
      role="dialog" aria-modal="true">
      <div className="w-full max-w-[340px] rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-modal p-6 modal-in">
        {contenido()}
      </div>
    </div>
  )
}

/* --- Turnos en curso (supervisión) ---------------------------------------- */

function TurnosVivos({ onCambio }) {
  const toast = useToast()
  const confirm = useConfirm()
  const { data, loading, error, reload } = useRecurso(() => api.turnosSalon(), [])
  const [forzando, setForzando] = useState(null)
  const [busy, setBusy] = useState('')
  const turnos = data?.turnos || []

  const refrescar = () => { reload(); onCambio() }

  const finalizar = async (t) => {
    const ok = await confirm({
      title: `¿Terminar el turno de ${t.mesoneroNombre}?`,
      body: 'Si le quedan mesas abiertas no se cierra de golpe: deja de tomar mesas nuevas y sigue atendiendo las suyas. Al cerrar la última, su turno termina solo.',
      confirmLabel: 'Terminar turno',
    })
    if (!ok) return
    setBusy(t.id)
    try {
      const out = await api.finalizarTurno(t.id)
      toast({
        title: out.estado === 'cerrado' ? 'Turno cerrado' : 'Turno en cierre',
        body: out.estado === 'cerrado'
          ? `${t.mesoneroNombre} terminó.`
          : `${t.mesoneroNombre} ya no toma mesas nuevas; termina al cerrar las que tiene.`,
      })
      refrescar()
    } catch (e) {
      toast({ title: 'No se pudo', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusy('') }
  }

  if (loading || error) {
    return <div className="mt-5"><EstadoRecurso loading={loading} error={error} onRetry={reload} rows={2} cols={3}
      title="No se pudieron cargar los turnos en curso" /></div>
  }
  if (turnos.length === 0) return null

  return (
    <div className="mt-6">
      <div className="text-[14px] font-semibold mb-2">Turnos en curso</div>
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card divide-y divide-slate-100 dark:divide-slate-800">
        {turnos.map((t) => (
          <div key={t.id} className="p-3.5 flex items-center gap-3 flex-wrap">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5 flex-wrap">
                <span className="font-semibold text-[13.5px]">{t.mesoneroNombre}</span>
                <span className="text-[11.5px] text-slate-400 num">{t.mesoneroCodigo}</span>
                {t.estado === 'cerrando'
                  ? <Badge size="sm" color="amber" dot>Cerrando</Badge>
                  : <Badge size="sm" color="teal" dot>En turno</Badge>}
              </div>
              <div className="text-[12px] text-slate-500 mt-0.5">
                Desde {(t.apertura || '').slice(11, 16)} · validó {t.validadoPor}
              </div>
            </div>
            <div className="flex gap-1.5 shrink-0">
              <Button size="sm" variant="ghost" disabled={busy === t.id} onClick={() => finalizar(t)}>
                Terminar turno
              </Button>
              <Button size="sm" variant="ghost" disabled={busy === t.id} onClick={() => setForzando(t)}>
                Cerrar ya
              </Button>
            </div>
          </div>
        ))}
      </div>
      {forzando ? (
        <ModalForzarCierre turno={forzando} onCerrar={() => setForzando(null)}
          onListo={() => { setForzando(null); refrescar() }} />
      ) : null}
    </div>
  )
}

/* ModalForzarCierre resuelve el caso del mesonero que se fue con mesas encima:
 * alguien TIENE que heredarlas o esas cuentas quedan sin quién las cobre. El
 * servidor sugiere a quien menos carga tiene, pero decide el supervisor — él
 * sabe cosas que el sistema no (quién está por irse, quién conoce esa zona). */
function ModalForzarCierre({ turno, onCerrar, onListo }) {
  const toast = useToast()
  const { data, loading, error, reload } = useRecurso(() => api.relevosTurno(turno.id), [turno.id])
  const relevos = data?.relevos || []
  const [elegido, setElegido] = useState('')
  const [pidiendoPin, setPidiendoPin] = useState(false)

  // El sugerido viene marcado por el servidor y es la preselección.
  const seleccion = elegido || relevos.find((r) => r.sugerido)?.mesoneroId || ''

  if (pidiendoPin) {
    return (
      <div className="fixed inset-0 z-[240] flex items-center justify-center p-5 bg-slate-900/55 backdrop-blur-sm"
        role="dialog" aria-modal="true">
        <div className="w-full max-w-[340px] rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-modal p-6 modal-in">
          <PedirPin tono="amber" icono={<Icon.Shield size={22} />} titulo="Autoriza el cierre"
            sub={`Cerrar el turno de ${turno.mesoneroNombre} requiere el PIN de un supervisor.`}
            onCancelar={() => setPidiendoPin(false)}
            onEnviar={async (pinSup) => {
              await api.forzarCierreTurno(turno.id, { relevoMesoneroId: seleccion, pinSupervisor: pinSup })
              toast({ title: 'Turno cerrado', body: `${turno.mesoneroNombre} terminó; sus mesas quedaron asignadas.` })
              onListo()
            }} />
        </div>
      </div>
    )
  }

  return (
    <Modal open onClose={onCerrar} size="sm" icon={<Icon.ArrowLeftRight size={18} />}
      title={`Cerrar ya el turno de ${turno.mesoneroNombre}`}
      sub="Sus mesas abiertas pasan a otra persona"
      footer={<>
        <Button variant="ghost" onClick={onCerrar}>Cancelar</Button>
        <Button onClick={() => setPidiendoPin(true)} disabled={loading || (relevos.length > 0 && !seleccion)}>
          Continuar
        </Button>
      </>}>
      <EstadoRecurso loading={loading} error={error} onRetry={reload} rows={2} cols={2}
        title="No se pudieron cargar los relevos">
        {relevos.length === 0 ? (
          <div className="text-[13px] text-slate-600 dark:text-slate-300">
            No tiene mesas abiertas, así que el turno se cierra sin traspasar nada.
          </div>
        ) : (
          <div className="space-y-2">
            <div className="text-[12.5px] text-slate-500">
              ¿Quién atiende sus mesas? Arriba va quien menos carga tiene.
            </div>
            {relevos.map((r) => {
              const on = seleccion === r.mesoneroId
              return (
                <button key={r.mesoneroId} type="button" onClick={() => setElegido(r.mesoneroId)}
                  className={`w-full text-left rounded-xl border px-3.5 py-3 flex items-center gap-3 transition
                    ${on ? 'border-elerp-500 bg-elerp-50/60 dark:bg-elerp-900/20' : 'border-slate-200 dark:border-slate-700 hover:border-slate-300'}`}>
                  <span className={`h-4 w-4 rounded-full border-[1.5px] shrink-0 ${on ? 'bg-elerp-500 border-elerp-500' : 'border-slate-300'}`} />
                  <span className="min-w-0 flex-1">
                    <span className="font-semibold text-[13.5px] block truncate">{r.nombre}</span>
                    <span className="text-[11.5px] text-slate-400 num">{r.codigo}</span>
                  </span>
                  {r.sugerido ? <Badge size="sm" color="teal">Sugerido</Badge> : null}
                  <Badge size="sm" color="slate">{r.mesasAbiertas} mesa{r.mesasAbiertas === 1 ? '' : 's'}</Badge>
                </button>
              )
            })}
          </div>
        )}
      </EstadoRecurso>
    </Modal>
  )
}

/* --- Credenciales (administración) ---------------------------------------- */

function Credenciales({ mesoneros, onCambio }) {
  const toast = useToast()
  const confirm = useConfirm()
  const [nuevo, setNuevo] = useState(false)
  const [busy, setBusy] = useState('')

  const alternarActivo = async (m) => {
    if (m.activo && !(await confirm({
      title: `¿Desactivar a ${m.nombre}?`,
      body: 'No se borra: deja de aparecer en la grilla de entrada y no puede iniciar turno. Se puede reactivar cuando quieras.',
      confirmLabel: 'Desactivar', tone: 'danger',
    }))) return
    setBusy(m.id)
    try {
      await api.actualizarMesoneroSalon(m.id, { activo: !m.activo })
      onCambio()
    } catch (e) {
      toast({ title: 'No se pudo', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusy('') }
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3">
        <div>
          <div className="text-[14px] font-semibold">Credenciales del salón</div>
          <div className="text-[12.5px] text-slate-500">
            Da de alta a cada persona. El PIN <strong>lo elige ella</strong> en la tablet, con un supervisor
            autorizando — nunca sale nada a un correo ni a un teléfono personal.
          </div>
        </div>
        <Button size="sm" icon={<Icon.UserPlus size={15} />} onClick={() => setNuevo(true)}>Nuevo mesonero</Button>
      </div>

      {mesoneros.length === 0 ? null : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                <th className="py-2.5 px-3 font-medium">Código</th>
                <th className="py-2.5 pr-3 font-medium">Nombre</th>
                <th className="py-2.5 pr-3 font-medium">PIN</th>
                <th className="py-2.5 pr-3 font-medium">Estado</th>
                <th className="py-2.5 pr-3 font-medium text-right">Acciones</th>
              </tr>
            </thead>
            <tbody>
              {mesoneros.map((m) => (
                <tr key={m.id} className={`border-b border-slate-100 dark:border-slate-800/70 ${m.activo ? '' : 'opacity-60'}`}>
                  <td className="py-2.5 px-3 num text-[12.5px] font-medium">{m.codigo}</td>
                  <td className="py-2.5 pr-3 text-[13px]">{m.nombre}</td>
                  <td className="py-2.5 pr-3">
                    {m.tienePin
                      ? <Badge size="sm" color="teal">Registrado</Badge>
                      : <Badge size="sm" color="amber">Pendiente</Badge>}
                  </td>
                  <td className="py-2.5 pr-3">
                    <Badge size="sm" color={m.activo ? 'emerald' : 'slate'} dot>{m.activo ? 'Activo' : 'Inactivo'}</Badge>
                  </td>
                  <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                    <Button size="sm" variant="ghost" disabled={busy === m.id} onClick={() => alternarActivo(m)}>
                      {m.activo ? 'Desactivar' : 'Reactivar'}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {nuevo ? <ModalNuevoMesonero onCerrar={() => setNuevo(false)}
        onListo={() => { setNuevo(false); onCambio() }} /> : null}
    </div>
  )
}

function ModalNuevoMesonero({ onCerrar, onListo }) {
  const toast = useToast()
  const [f, setF] = useState({ nombre: '', usuarioId: '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [candidatos, setCandidatos] = useState([])

  // La credencial se ata a un USUARIO de la app: las mesas y las asignaciones del
  // salón ya se llevan por usuario, y partir esa identidad en dos dejaría mesas
  // sin dueño. Por eso se elige de la lista, no se teclea.
  useEffect(() => {
    api.usuarios()
      .then((r) => setCandidatos((r || []).filter((u) => u.rol === 'mesonero')))
      .catch(() => setCandidatos([]))
  }, [])

  // Al elegir el usuario se propone su nombre: teclearlo otra vez solo invita a
  // que la credencial y el usuario terminen llamándose distinto.
  const elegirUsuario = (id) => {
    const u = candidatos.find((x) => x.usuarioId === id)
    setF((s) => ({ usuarioId: id, nombre: s.nombre.trim() ? s.nombre : (u?.nombre || '') }))
    setError('')
  }

  const guardar = async () => {
    if (!f.nombre.trim() || !f.usuarioId) { setError('Hacen falta el nombre y el usuario.'); return }
    setBusy(true); setError('')
    try {
      const m = await api.crearMesoneroSalon({ nombre: f.nombre.trim(), usuarioId: f.usuarioId })
      toast({ title: `${m.codigo} · ${m.nombre}`, body: 'Ahora que elija su PIN en la tablet, con un supervisor.' })
      onListo()
    } catch (e) {
      setError(e?.message || 'No se pudo crear.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onCerrar} size="sm" icon={<Icon.UserPlus size={18} />}
      title="Nuevo mesonero" sub="El código lo asigna el servidor (MS-001, MS-002…)"
      footer={<>
        <Button variant="ghost" onClick={onCerrar}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Crear</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre" required>
          <Input value={f.nombre} placeholder="Ej. Luis Marcano" autoFocus
            onChange={(e) => { setF((s) => ({ ...s, nombre: e.target.value })); setError('') }} />
        </Field>
        <Field label="Usuario de la aplicación" required
          hint="las mesas y las asignaciones se llevan por usuario">
          <Select value={f.usuarioId} onChange={(e) => elegirUsuario(e.target.value)}>
            <option value="">Elige un usuario…</option>
            {candidatos.map((u) => <option key={u.usuarioId} value={u.usuarioId}>{u.nombre || u.email}</option>)}
          </Select>
        </Field>
        {candidatos.length === 0 ? (
          <div className="text-[12px] text-slate-500 rounded-lg bg-slate-50 dark:bg-slate-800/60 px-3 py-2.5">
            No hay usuarios con rol <strong>mesonero</strong>. Créalos primero en
            Configuración › Usuarios y roles, con su sede.
          </div>
        ) : null}
        {error ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
        <div className="text-[11.5px] text-slate-400">
          No se envía ningún enlace ni contraseña. La persona elige su PIN en esta pantalla,
          y un supervisor lo autoriza con el suyo.
        </div>
      </div>
    </Modal>
  )
}

/* --- Historial ------------------------------------------------------------- */

// El resumen se congela al cerrar: es la foto de esa jornada y no se recalcula.
// Es la base de «quién trabajó anoche» y de la comisión.
function Historial() {
  const { data, loading, error, reload } = useRecurso(() => api.historialTurnos(), [])
  const cerrados = (data?.turnos || []).filter((t) => t.estado === 'cerrado')

  if (loading || error) {
    return <EstadoRecurso loading={loading} error={error} onRetry={reload} rows={3} cols={5}
      title="No se pudo cargar el historial de turnos" />
  }
  if (cerrados.length === 0) return null

  return (
    <div>
      <div className="text-[14px] font-semibold mb-2">Jornadas cerradas</div>
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
              <th className="py-2.5 px-3 font-medium">Mesonero</th>
              <th className="py-2.5 pr-3 font-medium">Jornada</th>
              <th className="py-2.5 pr-3 font-medium text-right">Tiempo</th>
              <th className="py-2.5 pr-3 font-medium text-right">Mesas</th>
              <th className="py-2.5 pr-3 font-medium text-right">Personas</th>
              <th className="py-2.5 pr-3 font-medium text-right">Órdenes</th>
              <th className="py-2.5 pr-3 font-medium text-right">Facturado</th>
              <th className="py-2.5 pr-3 font-medium text-right">Ticket</th>
            </tr>
          </thead>
          <tbody>
            {cerrados.map((t) => {
              const r = t.resumen || {}
              const h = Math.floor((r.minutosTrabajados || 0) / 60)
              const min = (r.minutosTrabajados || 0) % 60
              return (
                <tr key={t.id} className="border-b border-slate-100 dark:border-slate-800/70">
                  <td className="py-2.5 px-3">
                    <span className="font-medium text-[13px]">{t.mesoneroNombre}</span>
                    <span className="text-[11px] text-slate-400 num ml-1.5">{t.mesoneroCodigo}</span>
                  </td>
                  <td className="py-2.5 pr-3 text-[12px] text-slate-500 num whitespace-nowrap">
                    {(t.apertura || '').slice(0, 10)} · {(t.apertura || '').slice(11, 16)}–{(t.cierre || '').slice(11, 16)}
                    {t.cerradoPor ? <span className="ml-1 text-amber-600 dark:text-amber-400">forzado</span> : null}
                  </td>
                  <td className="py-2.5 pr-3 text-right num text-[12.5px]">{h}h {min}m</td>
                  <td className="py-2.5 pr-3 text-right num text-[12.5px]">{fmtNum(r.mesas || 0, 0)}</td>
                  <td className="py-2.5 pr-3 text-right num text-[12.5px]">{fmtNum(r.personas || 0, 0)}</td>
                  <td className="py-2.5 pr-3 text-right num text-[12.5px]">{fmtNum(r.ordenes || 0, 0)}</td>
                  <td className="py-2.5 pr-3 text-right num text-[12.5px] font-medium">{fmtNum(r.totalFacturado || 0, 2)}</td>
                  <td className="py-2.5 pr-3 text-right num text-[12.5px] text-slate-500">{fmtNum(r.ticketPromedio || 0, 2)}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
