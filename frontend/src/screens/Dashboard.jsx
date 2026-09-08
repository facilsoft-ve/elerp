import { useMemo, useState, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Card, Badge, Button, Empty, InfoTip, Segmented } from '../components/primitives.jsx'
import { ModuleIcon, ModularCorner } from '../components/brand.jsx'
import { BarChart } from '../components/charts.jsx'
import { NAV } from '../components/nav.js'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { metricas, alertas, serieVentas, hoy, diaLocal } from '../lib/metricas.js'
import { useData, useTasa } from '../context/DataContext.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'
import { monedaLabel } from '../lib/precio.js'

/* INICIO — panel de trabajo (Regla UX A4: tareas, KPIs del rol, accesos y
 * alertas; nunca un catálogo estático de módulos).
 *
 * La composición sigue al prototipo, que es el árbitro visual:
 *   1. Saludo en panel azul suave, con la fecha y las acciones del rol.
 *   2. Lanzador con la familia completa de iconos de módulo.
 *   3. Cuatro KPIs con su tooltip «¿qué es esto?».
 *   4. Ventas de los últimos 30 días en barras + Pendientes de hoy.
 *   5. Tarjeta del Asistente con preguntas sugeridas.
 *
 * Las cifras se derivan del ledger real (lib/metricas.js). Los KPIs cuyo módulo
 * todavía no existe muestran «sin datos aún» y nombran lo que falta: nunca un
 * número inventado.
 */

const saludo = () => {
  const h = new Date().getHours()
  if (h < 12) return 'Buenos días'
  if (h < 19) return 'Buenas tardes'
  return 'Buenas noches'
}

const DIAS = ['domingo', 'lunes', 'martes', 'miércoles', 'jueves', 'viernes', 'sábado']
const MESES = ['enero', 'febrero', 'marzo', 'abril', 'mayo', 'junio', 'julio', 'agosto', 'septiembre', 'octubre', 'noviembre', 'diciembre']
const fechaLarga = () => {
  const d = new Date()
  return `${DIAS[d.getDay()]} ${d.getDate()} de ${MESES[d.getMonth()]}`
}

// Acciones del panel de saludo por rol. El verde es exclusivo de las acciones
// que mueven dinero — de ahí que "Vender ahora" y "Registrar cobro" lo lleven y
// "Registrar compra" no.
const ACCIONES = {
  dueno: [
    { label: 'Vender ahora', icon: Icon.Activity, ruta: 'pos', variant: 'dinero' },
    { label: 'Registrar compra', icon: Icon.Cart, ruta: 'compras', variant: 'secondary' },
    { label: 'Registrar cobro', icon: Icon.Banknote, ruta: 'finanzas', variant: 'secondary' },
  ],
  vendedor: [
    { label: 'Vender ahora', icon: Icon.Activity, ruta: 'pos', variant: 'dinero' },
    { label: 'Nuevo cliente', icon: Icon.Users, ruta: 'ventas', variant: 'secondary' },
  ],
  cajero: [
    { label: 'Ir a la caja', icon: Icon.Cart, ruta: 'pos', variant: 'dinero' },
    { label: 'Mis documentos', icon: Icon.Receipt, ruta: 'facturacion:factura', variant: 'secondary' },
  ],
  contadora: [
    { label: 'Ver documentos', icon: Icon.Receipt, ruta: 'facturacion:factura', variant: 'secondary' },
    { label: 'Libros fiscales', icon: Icon.Book, ruta: 'facturacion:libros-venta', variant: 'secondary' },
  ],
}

// KPI: cifra real, o el motivo por el que todavía no hay cifra.
/* useTesoreriaKpis trae los totales de Tesorería para el Inicio. Si la consulta
 * falla, los KPIs quedan en cero declarando que no se pudo consultar, en vez de
 * inventar un número. */
function useTesoreriaKpis() {
  // Depende del TENANT activo: con la lista de dependencias vacía, al cambiar de
  // empresa el Inicio seguía mostrando las cifras de Tesorería de la empresa anterior
  // (el Dashboard no se desmonta al cambiar). Parecía una fuga entre tenants y era
  // caché del cliente: los datos se piden con los headers X-Empresa-ID/X-Sede-ID.
  const { activeEmpresaId, activeSedeId } = useAuth()
  const [estado, setEstado] = useState({ cargado: false, porCobrar: null, saldos: null })
  useEffect(() => {
    let vivo = true
    // Limpiar antes de consultar: mientras carga es mejor «sin datos» que el número
    // de otra empresa.
    setEstado({ cargado: false, porCobrar: null, saldos: null })
    Promise.all([api.porCobrar().catch(() => null), api.saldosTesoreria().catch(() => null)])
      .then(([porCobrar, saldos]) => { if (vivo) setEstado({ cargado: true, porCobrar, saldos }) })
    return () => { vivo = false }
  }, [activeEmpresaId, activeSedeId])
  return estado
}

function Kpi({ label, ayuda, value, sub, tono = 'tinta', progreso, faltante }) {
  const color = tono === 'alerta' ? 'text-[#B3362C] dark:text-red-400' : 'text-slate-900 dark:text-slate-100'
  return (
    <Card className="!p-5">
      <div className="flex items-center gap-1.5 text-[13px] text-slate-500 dark:text-slate-400">
        <span className="truncate">{label}</span>
        {ayuda ? <InfoTip>{ayuda}</InfoTip> : null}
      </div>
      {faltante ? (
        <>
          <div className="font-display font-bold text-[20px] text-slate-300 dark:text-slate-600 mt-2.5">Sin datos aún</div>
          <div className="text-[12px] text-slate-400 mt-1">{faltante}</div>
        </>
      ) : (
        <>
          <div className={`font-display font-bold text-[27px] tnum leading-tight mt-2 private-mask truncate ${color}`}>{value}</div>
          {sub ? <div className="text-[12.5px] text-slate-500 dark:text-slate-400 mt-1 truncate private-mask">{sub}</div> : null}
          {progreso != null ? (
            <div className="mt-3 h-1.5 rounded-full bg-slate-200 dark:bg-slate-700 overflow-hidden">
              <div className="h-full rounded-full bg-elerp-500" style={{ width: `${Math.min(100, progreso)}%` }} />
            </div>
          ) : null}
        </>
      )}
    </Card>
  )
}

const PREGUNTAS = ['¿Cuánto IVA debo pagar este mes?', '¿Qué producto me deja más margen?', '¿Quién me debe más?']

// Glifo por tipo de pendiente (lib/metricas.js devuelve una clave, no JSX).
const ICONO_AVISO = {
  doc: Icon.Receipt,
  caja: Icon.ModInventario,
  camion: Icon.Truck,
  reloj: Icon.Clock,
  banco: Icon.Bank,
}

export function Dashboard({ setRoute }) {
  const { db } = useData()
  const tasa = useTasa()
  // Tesorería alimenta dos KPIs. Se pide aparte del bootstrap porque son
  // proyecciones que pueden tardar más y no deben retrasar el Inicio.
  const tes = useTesoreriaKpis()
  const { user, activeEmpresa, activeSede } = useAuth()
  const { ui, setUi } = useUI()

  const rol = ui.rol
  const propio = rol === 'vendedor' || rol === 'cajero'
  const mt = useMemo(() => metricas(db, { actor: propio ? user?.userId : null }), [db, propio, user])
  const avisos = useMemo(() => alertas(mt, { rol }), [mt, rol])
  const serie = useMemo(() => serieVentas(db.DOCUMENTOS, 30), [db.DOCUMENTOS])
  const totalPeriodo = useMemo(() => serie.puntos.reduce((a, p) => a + p.v, 0), [serie])

  const misDocumentos = useMemo(() => {
    const h = hoy()
    return (db.DOCUMENTOS || [])
      .filter((d) => d.tipo === 'factura' && d.actor === user?.userId && diaLocal(d.fecha) === h)
      .sort((a, b) => String(b.fecha).localeCompare(String(a.fecha)))
      .slice(0, 8)
  }, [db.DOCUMENTOS, user])

  const acciones = ACCIONES[rol] || ACCIONES.dueno
  const enBs = ui.ccy === 'VES'
  // Tasa de la divisa de presentación elegida (VES=identidad). Los importes se
  // guardan en Bs; se convierten a la divisa activa para mostrarlos.
  const tMostrar = useTasa(ui.ccy)
  // Presentación en la moneda activa; los importes se guardan en Bs. Sin tasa
  // cargada no se puede expresar en divisa: se dice, en vez de convertir con un 1.
  const enMoneda = (bs) => {
    if (enBs) return fmtCurrency(bs, 'VES', { max: 2 })
    const v = tMostrar.aMoneda(bs)
    return v === null ? 'sin tasa' : fmtCurrency(v, ui.ccy, { max: 2 })
  }
  // Opciones del toggle de moneda: Bs + divisas activas (igual que la barra).
  const monedaOpciones = useMemo(() => {
    const activas = db?.TASAS?.activas
    const codigos = Array.isArray(activas) && activas.length
      ? activas.map((a) => (a.codigo || '').toUpperCase()).filter(Boolean)
      : ['USD']
    const unicos = [...new Set(codigos.filter((c) => c && c !== 'VES'))]
    return [{ value: 'VES', label: 'Bs' }, ...unicos.map((c) => ({ value: c, label: monedaLabel(c) }))]
  }, [db?.TASAS])

  // Módulos que este rol alcanza, en el orden del menú.
  // El lanzador cruza ROL y MÓDULO ACTIVO, igual que el Sidebar: si el módulo no está
  // instalado en la empresa (p. ej. Restaurante en una farmacia) su tarjeta no va —
  // ofrecerla llevaría a una pantalla que el guard de ruta rebota al inicio.
  const modulosActivos = db?.MODULOS || []
  const modulos = NAV.filter((n) => n.roles.includes(rol) && n.id !== 'dashboard' && n.id !== 'diseno'
    && (!n.modulo || modulosActivos.includes(n.modulo)))

  const esCajero = rol === 'cajero'
  const esContadora = rol === 'contadora'

  return (
    <div className="p-4 md:p-6 space-y-5">
      {/* 1 · Saludo */}
      <div className="relative overflow-hidden rounded-xl bg-gradient-to-br from-[#1D3477] to-[#152C61] px-6 py-5">
        {/* Panel de bienvenida: superficie de marca navy (como el login), donde el
            racimo modular blanco translúcido resalta. Un solo hub verde, anclado
            arriba a la derecha, lejos del saludo y las acciones. */}
        <ModularCorner corner="tr" variant="full" tone="dark" width={150} />
        <div className="relative">
        <h1 className="font-display font-bold text-[26px] leading-tight text-white">
          {saludo()}{user?.nombre ? `, ${user.nombre.split(' ')[0]}` : ''}
        </h1>
        <p className="text-[13.5px] text-white/70 mt-1">
          {activeEmpresa?.nombre}{activeSede ? ` · ${activeSede.nombre}` : ''} · {fechaLarga()}
        </p>
        <div className="flex flex-wrap gap-2.5 mt-4">
          {acciones.map((a) => {
            const IconoA = a.icon
            return (
              <Button key={a.label} variant={a.variant} icon={<IconoA size={16} />}
                onClick={() => setRoute(a.ruta)}>{a.label}</Button>
            )
          })}
        </div>
        </div>
      </div>

      {/* 2 · Lanzador de módulos */}
      <div className="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-6 xl:grid-cols-9 gap-3">
        {modulos.map((m) => (
          <button key={m.id} onClick={() => setRoute(m.id)} title={m.label}
            className="rounded-xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800
              px-2 py-4 flex flex-col items-center gap-2.5 text-center transition-all ring-focus
              hover:border-elerp-300 hover:shadow-brand">
            <ModuleIcon glyph={m.glyph} size={44} />
            <span className="text-[12px] font-medium leading-tight text-slate-700 dark:text-slate-300">{m.label}</span>
            {!m.ready ? <span className="text-[9.5px] uppercase tracking-wide text-slate-400 font-semibold">pronto</span> : null}
          </button>
        ))}
      </div>

      {/* 3 · KPIs */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        {esContadora ? (
          <>
            <Kpi label="Ventas del mes" value={enMoneda(mt.ventasMes)} sub={`${mt.facturasMes} factura(s) vigentes`}
              ayuda="Suma de las facturas emitidas este mes que no tienen reversa." />
            <Kpi label="IVA débito fiscal del mes" value={enMoneda(mt.ivaMes)} sub="Alícuota general 16%"
              ayuda="El IVA que cobraste en tus ventas del mes. Es lo que debes declarar, menos el crédito fiscal de tus compras." />
            <Kpi label="IGTF del mes" value={enMoneda(mt.igtfMes)} sub="3% sobre cobros en divisas"
              ayuda="Impuesto a las Grandes Transacciones Financieras: 3% sobre la parte que te pagaron en divisas, cobrado en bolívares." />
            <Kpi label="Reversas del mes" value={fmtNum(mt.reversasMes, 0)} sub="Anulaciones y notas de crédito"
              ayuda="Una factura nunca se edita ni se borra: se corrige con un documento de reversa que la referencia." />
          </>
        ) : (
          <>
            <Kpi label={propio ? 'Mis ventas de hoy' : 'Ventas de hoy'}
              value={enMoneda(propio ? mt.ventasMias : mt.ventasHoy)}
              sub={enBs
                ? (tasa.hay
                    ? `≈ ${fmtCurrency(tasa.aUsd(propio ? mt.ventasMias : mt.ventasHoy), 'USD')} a la tasa de ${tasa.fuenteLabel}`
                    : `${propio ? mt.facturasMias : mt.facturasHoy} factura(s) hoy · sin tasa para el equivalente en US$`)
                : `${propio ? mt.facturasMias : mt.facturasHoy} factura(s) hoy`}
              ayuda="Facturas emitidas hoy que no tienen reversa. Se agrupan por día local, no UTC." />
            {/* Ya tienen módulo detrás (Tesorería): salen del servidor, no de un
                cálculo de la interfaz. */}
            <Kpi label="Por cobrar vencido" tono={tes.porCobrar?.totalVencido > 0 ? 'alerta' : 'tinta'}
              value={tes.cargado ? enMoneda(tes.porCobrar?.totalVencido || 0) : '…'}
              sub={tes.cargado
                ? `${enMoneda(tes.porCobrar?.total || 0)} por cobrar en total`
                : 'consultando Tesorería…'}
              ayuda="Lo que te deben tus clientes y ya pasó su fecha de pago acordada. Se deriva de las facturas a crédito menos todo lo cobrado." />
            <Kpi label="Efectivo y bancos"
              value={tes.cargado ? enMoneda(tes.saldos?.totalBs || 0) : '…'}
              sub={tes.cargado
                ? `${fmtCurrency(tes.saldos?.efectivoBs || 0, 'VES')} en caja · ya sin el vuelto entregado`
                : 'consultando Tesorería…'}
              ayuda="Lo que entró por cada medio de cobro. No es el saldo del banco: la conciliación bancaria todavía no existe." />
            <Kpi label="Facturas del mes" value={fmtNum(mt.facturasMes, 0)}
              sub={`${mt.reversasMes} reversa(s) en el período`}
              ayuda="Documentos fiscales emitidos este mes. El límite por plan lo definirá el panel de la organización." />
          </>
        )}
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-3 gap-4">
        {/* 4a · Panel principal */}
        {esCajero ? (
          <Card className="xl:col-span-2">
            <div className="font-display font-semibold text-[16px]">Mis documentos de hoy</div>
            <div className="text-[12.5px] text-slate-500 mb-3">Las facturas que emitiste en tu turno.</div>
            {misDocumentos.length ? (
              <div className="divide-y divide-slate-100 dark:divide-slate-800">
                {misDocumentos.map((d) => (
                  <button key={d.id} onClick={() => setRoute('facturacion:factura')}
                    className="w-full flex items-center gap-3 py-2.5 px-2 text-left row-hover rounded-lg ring-focus">
                    <span className="mono text-[12.5px] text-slate-500 shrink-0">{d.numeroCompleto}</span>
                    <span className="flex-1 text-[13px] font-medium truncate">{d.clienteNombre || 'Consumidor final'}</span>
                    {d.contingencia ? <Badge size="sm" color="amber" dot>Contingencia</Badge> : null}
                    <span className="num text-[13px] font-semibold shrink-0 private-mask">{fmtCurrency(d.total, 'VES')}</span>
                  </button>
                ))}
              </div>
            ) : (
              <Empty framed={false} icon={<Icon.Receipt size={22} />}
                title="Todavía no has cobrado nada hoy"
                body="Abre la caja y emite tu primera factura del turno."
                cta={<Button variant="dinero" icon={<Icon.Cart size={16} />} onClick={() => setRoute('pos')}>Ir a la caja</Button>} />
            )}
          </Card>
        ) : (
          <Card className="xl:col-span-2">
            <div className="flex items-start justify-between gap-3 mb-4">
              <div>
                <div className="font-display font-semibold text-[16px]">Ventas — últimos 30 días</div>
                <div className="text-[12.5px] text-slate-500">
                  Total del período: <span className="font-semibold text-slate-700 dark:text-slate-200 tnum">{enMoneda(totalPeriodo)}</span>
                </div>
              </div>
              <Segmented size="sm" value={ui.ccy} onChange={(v) => setUi((u) => ({ ...u, ccy: v }))}
                options={monedaOpciones} />
            </div>
            {serie.hayDatos ? (
              <BarChart data={serie.puntos} height={230} fmt={(v) => enMoneda(v)} />
            ) : (
              <Empty framed={false} icon={<Icon.Chart size={22} />}
                title="Todavía no hay ventas registradas"
                body="En cuanto emitas la primera factura, aquí verás la evolución diaria real de tus ventas."
                cta={<Button icon={<Icon.Plus size={16} />} onClick={() => setRoute('pos')}>Emitir la primera factura</Button>} />
            )}
          </Card>
        )}

        {/* 4b · Pendientes de hoy */}
        <Card>
          <div className="flex items-center justify-between mb-3">
            <div className="font-display font-semibold text-[16px]">Pendientes de hoy</div>
            {avisos.length ? (
              <span className="h-6 min-w-6 px-1.5 rounded-full bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 text-[12px] font-bold inline-flex items-center justify-center tnum">
                {avisos.length}
              </span>
            ) : null}
          </div>
          {avisos.length === 0 ? (
            <div className="py-10 text-center">
              <Icon.CircleCheck size={26} className="mx-auto text-teal-500 mb-2" />
              <div className="text-[13.5px] font-medium">Todo al día</div>
              <div className="text-[12.5px] text-slate-500 mt-0.5">Sin pendientes para tu rol.</div>
            </div>
          ) : (
            <div className="divide-y divide-slate-100 dark:divide-slate-800 -mx-1">
              {avisos.map((a) => {
                const tono = a.nivel === 'error'
                  ? 'bg-red-50 text-[#B3362C] dark:bg-red-900/25 dark:text-red-300'
                  : a.nivel === 'warn'
                    ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/25 dark:text-amber-300'
                    : 'bg-elerp-50 text-elerp-500 dark:bg-elerp-900/40 dark:text-elerp-200'
                const IconoA = ICONO_AVISO[a.icono] || Icon.CircleAlert
                return (
                  <button key={a.id} onClick={() => setRoute(a.ruta)}
                    className="w-full flex items-center gap-3 py-3 px-1 text-left row-hover rounded-lg ring-focus">
                    <span className={`h-9 w-9 rounded-icon inline-flex items-center justify-center shrink-0 ${tono}`}>
                      <IconoA size={17} />
                    </span>
                    <span className="flex-1 min-w-0">
                      <span className="block text-[13px] font-semibold text-slate-800 dark:text-slate-100 truncate">{a.titulo}</span>
                      <span className="block text-[12px] text-slate-500 truncate">{a.detalle}</span>
                    </span>
                    <Icon.ChevRight size={15} className="text-slate-300 shrink-0" />
                  </button>
                )
              })}
            </div>
          )}
        </Card>
      </div>

      {/* 5 · Asistente */}
      <Card className="xl:max-w-md relative overflow-hidden">
        {/* Tarjeta de bienvenida a la IA: superficie de marca. Esquina modular
            suave (un solo hub verde) arriba a la derecha, sin tocar las
            preguntas sugeridas. */}
        <ModularCorner corner="tr" variant="soft" tone="light" />
        <div className="flex items-center gap-2.5 mb-2">
          <span className="h-9 w-9 rounded-icon bg-elerp-500 text-white inline-flex items-center justify-center shrink-0">
            <Icon.Sparkles size={18} />
          </span>
          <div className="font-display font-semibold text-[16px]">Asistente ElERP</div>
        </div>
        <div className="text-[12.5px] text-slate-500 mb-2.5">Pregúntale a tus números en lenguaje llano:</div>
        <div className="space-y-2">
          {PREGUNTAS.map((q) => (
            <button key={q} onClick={() => window.dispatchEvent(new CustomEvent('huberp:ia', { detail: q }))}
              className="w-full flex items-center gap-2 px-3 py-2.5 rounded-lg border border-slate-200 dark:border-slate-700
                text-left text-[13px] font-medium text-elerp-500 dark:text-slate-200 hover:bg-elerp-50 dark:hover:bg-slate-800 ring-focus">
              <Icon.Sparkles size={14} className="shrink-0 text-slate-400" /> {q}
            </button>
          ))}
        </div>
        <div className="text-[11.5px] text-slate-400 mt-3">Respondo solo con los datos que tu usuario puede ver.</div>
      </Card>
    </div>
  )
}
