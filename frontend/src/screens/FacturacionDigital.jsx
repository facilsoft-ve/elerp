import { useState, useEffect, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Select, Toggle, Empty, useToast, TableSkeleton, Field, Input, Modal } from '../components/primitives.jsx'
import { api } from '../lib/api.js'
import { fechaCortaVE } from '../components/tasa.jsx'

/* FACTURACIÓN DIGITAL — conectar ElERP con una imprenta autorizada.
 *
 * Esta pantalla decide QUIÉN emite el documento fiscal, que es una decisión más
 * grande de lo que su tamaño sugiere: encendida, la factura la firma la imprenta
 * y la caja imprime un comprobante NO fiscal. Por eso está armada en el orden en
 * que la cosa se puede hacer, y no en el orden en que los campos caben:
 *
 *   1. ENTRAR. Ambiente y credenciales, y probar ANTES de guardar nada.
 *   2. ELEGIR LA SERIE, de la lista que devolvió la imprenta. Nunca se teclea un
 *      identificador: teclear un UUID es una forma de equivocarse en silencio.
 *   3. DECIR A DÓNDE VA LA FACTURA cuando el cliente no da correo — la imprenta
 *      exige destinatario en toda factura y el mostrador vende a consumidor
 *      final todo el día.
 *   4. ENCENDER, por canal, y decir qué imprime la caja.
 *
 * El botón de activar está DESHABILITADO hasta que los cuatro pasos estén, y
 * dice cuál falta. Dejar activar algo que va a rechazar todas las facturas es
 * peor que no dejar activarlo.
 */

const AMBIENTES = [
  { value: 'qa', label: 'Pruebas (QA)', nota: 'Los documentos NO son fiscales. Para ensayar.' },
  { value: 'produccion', label: 'Producción', nota: 'Los documentos SON fiscales. No se borran: solo se anulan.' },
]

const TICKETS = [
  { value: 'termica', label: 'Impresora térmica', nota: 'La de comandas. Imprime el comprobante con el QR.' },
  { value: 'fiscal_no_fiscal', label: 'La impresora fiscal, en modo no fiscal', nota: 'Usa el equipo que ya tienes, sin emitir un segundo documento fiscal.' },
  { value: 'ninguno', label: 'No imprimir nada', nota: 'El cliente recibe el enlace por correo o por WhatsApp.' },
]

const ESTADOS = {
  pendiente: { label: 'En cola', color: 'slate' },
  enviado: { label: 'Enviada, esperando control', color: 'amber' },
  fiscal: { label: 'Fiscal', color: 'green' },
  rechazado: { label: 'Rechazada', color: 'red' },
  anulado: { label: 'Anulada', color: 'slate' },
}

export function FacturacionDigital() {
  const toast = useToast()
  const [cfg, setCfg] = useState(null)
  const [tieneClave, setTieneClave] = useState(false)
  const [clave, setClave] = useState('')          // solo en memoria, nunca vuelve del servidor
  const [diag, setDiag] = useState(null)
  const [probando, setProbando] = useState(false)
  const [guardando, setGuardando] = useState(false)
  const [verEmisiones, setVerEmisiones] = useState(false)

  const cargar = useCallback(() => {
    api.configDigital()
      .then((r) => { setCfg(r?.config || {}); setTieneClave(!!r?.tieneClave) })
      .catch(() => { setCfg({}); setTieneClave(false) })
  }, [])
  useEffect(() => { cargar() }, [cargar])

  const set = (k, v) => setCfg((c) => ({ ...c, [k]: v }))

  /* LO QUE FALTA PARA PODER ENCENDER. Se calcula acá y se muestra donde el
   * usuario está mirando —junto al interruptor— y no como un error al guardar:
   * un error después de llenar todo es enterarse tarde. */
  const falta = useMemo(() => {
    if (!cfg) return []
    const f = []
    if (!(cfg.usuario || '').trim()) f.push('el usuario de la imprenta')
    if (!tieneClave && !clave.trim()) f.push('la contraseña')
    if (!(cfg.serieStrongId || '').trim()) f.push('elegir la serie')
    if (!(cfg.correoRespaldo || '').trim()) f.push('el correo de respaldo')
    return f
  }, [cfg, tieneClave, clave])
  const puedeActivar = falta.length === 0

  const probar = async () => {
    setProbando(true)
    try {
      const d = await api.probarDigital({
        ambiente: cfg.ambiente || 'qa', usuario: (cfg.usuario || '').trim(), password: clave,
      })
      setDiag(d)
      if (!d.ok) {
        toast({ title: 'No se pudo entrar', body: d.problema || d.error || 'Revisa usuario y contraseña.', kind: 'error' })
      } else if (!d.puedeEmitir) {
        toast({ title: 'Entró, pero la cuenta no puede emitir', body: d.problema || '', kind: 'error' })
      } else {
        // Una sola serie habilitada: se elige sola. Hacer elegir entre una opción
        // es hacer trabajo por nada.
        const unica = (d.configuradas || []).length === 1 ? d.configuradas[0] : null
        if (unica && !cfg.serieStrongId) {
          setCfg((c) => ({ ...c, serieStrongId: unica.strongId, serieNombre: unica.name }))
        }
        toast({ title: 'Conexión correcta', body: `${(d.configuradas || []).length} serie(s) habilitada(s) para emitir.` })
      }
    } catch (e) {
      toast({ title: 'No se pudo probar', body: e?.message || 'Error', kind: 'error' })
    }
    setProbando(false)
  }

  const guardar = async () => {
    setGuardando(true)
    try {
      const r = await api.guardarConfigDigital({
        activa: !!cfg.activa, porPOS: !!cfg.porPOS, porVentas: !!cfg.porVentas,
        ambiente: cfg.ambiente || 'qa', usuario: (cfg.usuario || '').trim(),
        // La contraseña solo viaja si se escribió una nueva: vacía = conserva la
        // guardada. El servidor nunca la devuelve, así que no hay cómo re-enviarla.
        password: clave,
        serieStrongId: cfg.serieStrongId || '', serieNombre: cfg.serieNombre || '',
        sucursalStrongId: cfg.sucursalStrongId || '', sucursalNombre: cfg.sucursalNombre || '',
        correoRespaldo: (cfg.correoRespaldo || '').trim(), ticketPOS: cfg.ticketPOS || 'termica',
      })
      setCfg(r?.config || cfg)
      setClave('')
      setTieneClave(true)
      toast({
        title: 'Configuración guardada',
        body: r?.lista ? 'La facturación digital está lista para emitir.' : 'Guardada. Todavía no emite: revisa lo que falta.',
      })
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
    }
    setGuardando(false)
  }

  if (!cfg) {
    return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={5} cols={3} /></div>
  }

  const ambiente = AMBIENTES.find((a) => a.value === (cfg.ambiente || 'qa'))
  const series = diag?.configuradas || []

  return (
    <div className="space-y-4 max-w-3xl">
      <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
        Con esto encendido, <strong>la factura fiscal la emite y la firma la imprenta digital</strong>, y
        el cliente la recibe por correo y por una página que abre escaneando el QR del ticket. La caja
        pasa a imprimir un <strong>comprobante no fiscal</strong>: nunca dos documentos fiscales por una
        misma venta.
      </div>

      {/* 1 · ENTRAR */}
      <Seccion n="1" titulo="Conectar con la imprenta"
        sub="Se prueba antes de guardar: así se sabe si las credenciales sirven sin dejar el módulo a medio configurar.">
        <div className="grid sm:grid-cols-2 gap-3">
          <Field label="Ambiente" hint={ambiente?.nota}>
            <Select value={cfg.ambiente || 'qa'} onChange={(e) => { set('ambiente', e.target.value); setDiag(null) }}>
              {AMBIENTES.map((a) => <option key={a.value} value={a.value}>{a.label}</option>)}
            </Select>
          </Field>
          <Field label="Usuario" required>
            <Input value={cfg.usuario || ''} placeholder="empresa@imprenta.global" autoComplete="off"
              onChange={(e) => { set('usuario', e.target.value); setDiag(null) }} />
          </Field>
        </div>
        <Field label="Contraseña" required={!tieneClave}
          hint={tieneClave ? 'Ya hay una guardada. Déjalo vacío para conservarla.' : 'Se guarda cifrada; no se puede volver a leer.'}>
          <Input type="password" value={clave} autoComplete="new-password"
            placeholder={tieneClave ? '••••••••  (sin cambios)' : ''}
            onChange={(e) => { setClave(e.target.value); setDiag(null) }} />
        </Field>
        <div className="flex items-center gap-2.5 flex-wrap">
          <Button size="sm" variant="secondary" onClick={probar} loading={probando}
            disabled={!(cfg.usuario || '').trim() || (!tieneClave && !clave.trim())}
            icon={<Icon.Plug size={15} />}>Probar conexión</Button>
          {diag ? (
            diag.ok && diag.puedeEmitir
              ? <Badge color="green" dot>Conectado y habilitado para emitir</Badge>
              : <Badge color="red" dot>{diag.ok ? 'Conectó, pero no puede emitir' : 'No conectó'}</Badge>
          ) : null}
        </div>
        {diag?.problema ? (
          <div className="rounded-lg px-3 py-2 text-[12.5px]"
            style={{ background: '#FBEDEB', border: '1px solid #ECC8C4', color: '#B3362C' }}>
            {diag.problema}
          </div>
        ) : null}
        {/* Los avisos de la imprenta se muestran TAL CUAL: son los que dicen, por
            ejemplo, que a una serie le falta la plantilla de correo — y ese dato
            no lo tenemos de ninguna otra parte. */}
        {(diag?.avisos || []).length ? (
          <div className="rounded-lg px-3 py-2 text-[12.5px]"
            style={{ background: '#FFF7E8', border: '1px solid #EEDCB4', color: '#92600A' }}>
            {diag.avisos.map((a, i) => <div key={i}>{a}</div>)}
          </div>
        ) : null}
      </Seccion>

      {/* 2 · LA SERIE */}
      <Seccion n="2" titulo="Serie de emisión"
        sub="Sale de la lista que devuelve la imprenta. No se teclea: un identificador mal copiado falla recién al emitir la primera factura.">
        {series.length === 0 ? (
          <div className="text-[12.5px] text-slate-500">
            {diag
              ? 'La imprenta no devolvió series habilitadas. Pídeselo a su soporte antes de seguir.'
              : 'Prueba la conexión para traer las series de tu cuenta.'}
            {cfg.serieNombre ? (
              <div className="mt-2 text-slate-700 dark:text-slate-300">
                Serie guardada: <strong>{cfg.serieNombre}</strong> <span className="text-slate-400">· {cfg.serieStrongId}</span>
              </div>
            ) : null}
          </div>
        ) : (
          <Field label="Serie" required>
            <Select value={cfg.serieStrongId || ''} onChange={(e) => {
              const s = series.find((x) => x.strongId === e.target.value)
              setCfg((c) => ({ ...c, serieStrongId: e.target.value, serieNombre: s?.name || '' }))
            }}>
              <option value="">Elige la serie…</option>
              {series.map((s) => <option key={s.strongId} value={s.strongId}>Serie {s.name}</option>)}
            </Select>
          </Field>
        )}

        {/* EN QUÉ NÚMERO VA LA IMPRENTA. Se muestra porque de ahí sale el punto de
            partida del correlativo: una cuenta que ya facturó desde otro sistema
            no arranca en 1, y verlo antes evita que la primera factura se rechace
            por número fuera de orden. */}
        {(diag?.contadores || []).length ? (
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
            <div className="px-3 py-2 text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/60">
              En qué número va la imprenta. ElERP toma esto como punto de partida al activar.
            </div>
            <div className="divide-y divide-slate-100 dark:divide-slate-800">
              {diag.contadores.map((c, i) => (
                <div key={i} className="px-3 py-1.5 flex justify-between text-[13px]">
                  <span className="text-slate-600 dark:text-slate-300">{c.documentType}</span>
                  <span className="num">{c.counter}</span>
                </div>
              ))}
            </div>
          </div>
        ) : null}

        {/* La SUCURSAL es opcional de verdad: se emitió contra una cuenta que no
            tiene ninguna. Solo se ofrece si la imprenta devolvió alguna. */}
        {(diag?.sucursales || []).length ? (
          <Field label="Sucursal" hint="opcional: solo si tu cuenta las usa">
            <Select value={cfg.sucursalStrongId || ''} onChange={(e) => {
              const s = (diag.sucursales || []).find((x) => x.strongId === e.target.value)
              setCfg((c) => ({ ...c, sucursalStrongId: e.target.value, sucursalNombre: s?.name || '' }))
            }}>
              <option value="">Sin sucursal</option>
              {diag.sucursales.map((s) => <option key={s.strongId} value={s.strongId}>{s.name}</option>)}
            </Select>
          </Field>
        ) : null}
      </Seccion>

      {/* 3 · EL CORREO */}
      <Seccion n="3" titulo="¿A dónde va la factura si el cliente no deja correo?"
        sub="La imprenta exige un destinatario en toda factura, y el mostrador vende a consumidor final todo el día. Sin esta dirección, esas ventas no se podrían emitir.">
        <Field label="Correo de respaldo" required
          hint="La copia cae aquí cuando el cliente no da el suyo. Suele ser el correo de administración.">
          <Input type="email" value={cfg.correoRespaldo || ''} placeholder="facturas@tuempresa.com"
            onChange={(e) => set('correoRespaldo', e.target.value)} />
        </Field>
      </Seccion>

      {/* LO QUE CAMBIA EN EL MOSTRADOR. Va antes del interruptor porque es la
          consecuencia que nadie espera: una bodega que hoy factura sin preguntar
          nada tiene que empezar a pedir la cédula, y enterarse vendiendo es
          enterarse con el cliente delante. */}
      <div className="rounded-xl p-4" style={{ background: '#FFF7E8', border: '1px solid #EEDCB4' }}>
        <div className="flex gap-2.5">
          <Icon.CircleAlert size={17} className="shrink-0 mt-0.5" style={{ color: '#92600A' }} />
          <div className="text-[12.5px]" style={{ color: '#92600A' }}>
            <strong>Con esto encendido hay que identificar al cliente en cada venta.</strong> La
            imprenta exige cédula o RIF <em>y</em> dirección en toda factura, también en la venta a
            consumidor final. ElERP lo pide <strong>antes</strong> de cobrar: una factura ya emitida
            sin esos datos no se podría fiscalizar, y para entonces el cliente ya se fue.
          </div>
        </div>
      </div>

      {/* 4 · ENCENDER */}
      <Seccion n="4" titulo="Encender"
        sub="Por canal: puedes empezar solo por el módulo de ventas y dejar el mostrador para después.">
        <Toggle checked={!!cfg.activa} disabled={!cfg.activa && !puedeActivar}
          onChange={(v) => set('activa', v)}
          label="Emitir con la imprenta digital"
          sub={cfg.activa
            ? 'Encendida: las facturas de los canales marcados salen por la imprenta.'
            : puedeActivar ? 'Todo listo para encenderla.' : `Falta ${falta.join(', ')}.`} />

        {cfg.activa ? (
          <>
            <div className="pl-1 space-y-2 border-l-2 border-slate-200 dark:border-slate-700 ml-1">
              <div className="pl-3">
                <Toggle checked={!!cfg.porPOS} onChange={(v) => set('porPOS', v)}
                  label="Punto de venta" sub="Las ventas del mostrador." />
              </div>
              <div className="pl-3">
                <Toggle checked={!!cfg.porVentas} onChange={(v) => set('porVentas', v)}
                  label="Módulo de ventas" sub="Las facturas que salen de una cotización." />
              </div>
            </div>
            {!cfg.porPOS && !cfg.porVentas ? (
              <div className="rounded-lg px-3 py-2 text-[12.5px]"
                style={{ background: '#FFF7E8', border: '1px solid #EEDCB4', color: '#92600A' }}>
                Está encendida pero sin ningún canal marcado: así no va a emitir nada.
              </div>
            ) : null}

            {cfg.porPOS ? (
              <Field label="¿Qué imprime la caja?"
                hint="Sea cual sea, es un comprobante NO fiscal: el documento fiscal lo emite la imprenta.">
                <Select value={cfg.ticketPOS || 'termica'} onChange={(e) => set('ticketPOS', e.target.value)}>
                  {TICKETS.map((t) => <option key={t.value} value={t.value}>{t.label}</option>)}
                </Select>
                <div className="mt-1 text-[12px] text-slate-500">
                  {TICKETS.find((t) => t.value === (cfg.ticketPOS || 'termica'))?.nota}
                </div>
              </Field>
            ) : null}

            {(cfg.ambiente || 'qa') === 'produccion' ? (
              <div className="rounded-lg px-3 py-2 text-[12.5px]"
                style={{ background: '#FBEDEB', border: '1px solid #ECC8C4', color: '#B3362C' }}>
                <strong>Ambiente de producción.</strong> Lo que se emita son documentos fiscales de
                verdad: no se borran, solo se anulan.
              </div>
            ) : null}
          </>
        ) : null}
      </Seccion>

      <div className="flex items-center justify-between gap-3 flex-wrap">
        <Button size="sm" variant="secondary" icon={<Icon.ClipboardList size={15} />}
          onClick={() => setVerEmisiones(true)}>Ver las facturas enviadas</Button>
        <Button onClick={guardar} loading={guardando} icon={<Icon.Check size={16} />}>Guardar</Button>
      </div>

      {verEmisiones ? <EmisionesModal onClose={() => setVerEmisiones(false)} /> : null}
    </div>
  )
}

function Seccion({ n, titulo, sub, children }) {
  return (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
      <div className="flex gap-3 mb-3">
        <span className="shrink-0 w-6 h-6 rounded-full bg-slate-100 dark:bg-slate-800 text-[12px] font-semibold flex items-center justify-center text-slate-500">{n}</span>
        <div>
          <div className="text-[14px] font-semibold">{titulo}</div>
          {sub ? <div className="text-[12.5px] text-slate-500 mt-0.5">{sub}</div> : null}
        </div>
      </div>
      <div className="space-y-3">{children}</div>
    </div>
  )
}

/* LA COLA. Es la pantalla de soporte: cuando un cliente reclama su factura, acá
 * se ve si salió, en qué quedó y por qué falló — con el mensaje literal de la
 * imprenta, que es lo que su soporte pide para diagnosticar. */
function EmisionesModal({ onClose }) {
  const [emisiones, setEmisiones] = useState(null)
  const [abierta, setAbierta] = useState(null)

  useEffect(() => {
    api.emisionesDigitales()
      .then((r) => setEmisiones(r?.emisiones || []))
      .catch(() => setEmisiones([]))
  }, [])

  return (
    <Modal open onClose={onClose} size="lg" icon={<Icon.ClipboardList size={18} />}
      title="Facturas enviadas a la imprenta"
      sub="El número de control llega unos minutos después de enviar: mientras tanto el documento no es fiscal todavía.">
      {emisiones === null ? <TableSkeleton rows={5} cols={4} /> : emisiones.length === 0 ? (
        <Empty icon={<Icon.FileText size={22} />} title="Todavía no se ha enviado ninguna"
          body="Acá van a aparecer las facturas a medida que se emitan, con su número de control y el enlace que ve el cliente." />
      ) : (
        <div className="overflow-x-auto -mx-1">
          <table className="w-full text-[13px]">
            <thead className="text-left text-slate-500 bg-slate-50 dark:bg-slate-800/60">
              <tr>
                <th className="px-3 py-2 font-medium">Documento</th>
                <th className="px-3 py-2 font-medium">Estado</th>
                <th className="px-3 py-2 font-medium">Nº de control</th>
                <th className="px-3 py-2 font-medium">Enviada</th>
                <th className="px-3 py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {[...emisiones].reverse().map((e) => {
                const st = ESTADOS[e.estado] || { label: e.estado, color: 'slate' }
                return (
                  <tr key={e.id} className="border-t border-slate-100 dark:border-slate-800">
                    <td className="px-3 py-2 font-medium">{e.tipo} {e.serie ? `${e.serie}-` : ''}{e.numero}</td>
                    <td className="px-3 py-2"><Badge color={st.color}>{st.label}</Badge></td>
                    <td className="px-3 py-2 num">{e.numeroControl || <span className="text-slate-400">—</span>}</td>
                    <td className="px-3 py-2 text-slate-500">{fechaCortaVE(e.creada)}</td>
                    <td className="px-3 py-2 text-right">
                      <button className="text-[12.5px] text-huberp-600 dark:text-teal-400 font-medium"
                        onClick={() => setAbierta(e)}>Detalle</button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      {abierta ? <DetalleEmision e={abierta} onClose={() => setAbierta(null)} /> : null}
    </Modal>
  )
}

function DetalleEmision({ e, onClose }) {
  const st = ESTADOS[e.estado] || { label: e.estado, color: 'slate' }
  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.FileText size={18} />}
      title={`${e.tipo} ${e.serie ? `${e.serie}-` : ''}${e.numero}`} sub={st.label}>
      <div className="space-y-3 text-[13px]">
        <Dato k="Documento en ElERP" v={e.documentoId} />
        <Dato k="Número de control" v={e.numeroControl || 'todavía no asignado'} />
        <Dato k="Código para compartir" v={e.codigoCorto} />
        <Dato k="Id en la imprenta" v={e.strongId} />
        {e.urlDocumento ? (
          <a className="block text-huberp-600 dark:text-teal-400 font-medium" href={e.urlDocumento}
            target="_blank" rel="noreferrer">Abrir la factura en la imprenta</a>
        ) : null}
        {e.token ? (
          <a className="block text-huberp-600 dark:text-teal-400 font-medium" href={`/f/${e.token}`}
            target="_blank" rel="noreferrer">Ver la página que abre el cliente</a>
        ) : null}

        {/* LA BITÁCORA, con el mensaje LITERAL de la imprenta. Traducirlo a algo
            más bonito sería quitarle a soporte lo único que puede reenviarles. */}
        {(e.intentos || []).length ? (
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
            <div className="px-3 py-2 text-[12px] text-slate-500 bg-slate-50 dark:bg-slate-800/60">Intentos de envío</div>
            <div className="divide-y divide-slate-100 dark:divide-slate-800">
              {e.intentos.map((it, i) => (
                <div key={i} className="px-3 py-2">
                  <div className="flex justify-between gap-2">
                    <span className="text-slate-500">{fechaCortaVE(it.cuando)}</span>
                    <span className={`num ${it.http >= 400 ? 'text-[#B3362C] dark:text-red-400 font-semibold' : 'text-slate-500'}`}>HTTP {it.http}</span>
                  </div>
                  {it.mensaje ? <div className="mt-0.5 text-[12.5px]">{it.mensaje}</div> : null}
                  {it.codigo ? <div className="text-[12px] text-slate-400">Código {it.codigo}</div> : null}
                </div>
              ))}
            </div>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}

const Dato = ({ k, v }) => (
  <div className="flex justify-between gap-3">
    <span className="text-slate-500">{k}</span>
    <span className="num text-right break-all">{v || <span className="text-slate-400">—</span>}</span>
  </div>
)
