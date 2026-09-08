import { useState, useEffect, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, VistaDetalle, Modal, Empty, TableSkeleton, useToast } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { api } from '../lib/api.js'

// Solo Dueña/Desarrollador emiten un Cierre Z (cierre fiscal); el resto solo ve
// la lista. Es la misma frontera que anular/nota de crédito.
const puedeEmitir = (rol) => ['dueno', 'desarrollador'].includes(rol)

// Cierres Z: el reporte fiscal DIARIO por sede. Consolida los documentos emitidos
// desde el último Z; su numeración Z es secuencial por sede e INMUTABLE (se
// corrige con un Z posterior, nunca se edita). Todo se deriva del ledger de
// documentos: la interfaz nunca inventa un total.
export function CierresZ() {
  const { db } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const sede = db.SEDE_ACTIVA || null
  const sedeId = sede?.id || null
  const emite = puedeEmitir(ui.rol)

  const [lista, setLista] = useState(undefined) // undefined = cargando
  const [error, setError] = useState(null)
  const [preview, setPreview] = useState(null) // { totales, cantidadDocumentos } | null
  const [detalle, setDetalle] = useState(null)
  const [confirmar, setConfirmar] = useState(false)

  const cargar = useCallback(async () => {
    if (!sedeId) return
    setError(null)
    try {
      // El preview solo lo pide quien puede emitir (la ruta está restringida a
      // Dueña/Desarrollador); el resto solo consulta la lista.
      const [zs, prev] = await Promise.all([
        api.cierresZ(),
        emite ? api.previewCierreZ().catch(() => null) : Promise.resolve(null),
      ])
      // El más reciente arriba: el servidor los da por Nº ascendente, se invierte.
      setLista([...(zs || [])].reverse())
      setPreview(prev)
    } catch (e) {
      setLista([])
      setError(e)
    }
  }, [sedeId, emite])

  useEffect(() => { setLista(undefined); cargar() }, [cargar])

  const cantidadPendiente = preview?.cantidadDocumentos || 0

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar los cierres Z"
      body={String(error.message || error)} cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }

  // Detalle a PANTALLA COMPLETA: al abrir un cierre Z, la vista de detalle
  // reemplaza la lista y ocupa el ancho principal, con «‹ Volver» para regresar.
  if (detalle) {
    return (
      <div>
        <DetalleZ z={detalle} ccy={ui.ccy} onVolver={() => setDetalle(null)} />
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center gap-3 flex-wrap mb-3">
        <div className="text-[12.5px] text-slate-500">
          {sede ? <>Cierres de <span className="font-medium text-slate-700 dark:text-slate-200">{sede.nombre}</span></> : 'Selecciona una sede'}
        </div>
        {emite ? (
          <div className="ml-auto flex items-center gap-2">
            {cantidadPendiente > 0 ? (
              <span className="text-[12px] text-slate-500 hidden sm:inline">
                {fmtNum(cantidadPendiente, 0)} documento(s) por cerrar
              </span>
            ) : null}
            <Button icon={<Icon.ClipboardList size={15} />} disabled={cantidadPendiente === 0}
              onClick={() => setConfirmar(true)}
              title={cantidadPendiente === 0 ? 'No hay documentos nuevos por cerrar' : 'Emitir Cierre Z'}>
              Emitir Cierre Z
            </Button>
          </div>
        ) : null}
      </div>
      {emite && cantidadPendiente === 0 && (lista || []).length ? (
        <div className="mb-3 text-[11.5px] text-slate-400">No hay documentos nuevos por cerrar desde el último Z de esta sede.</div>
      ) : null}

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {lista === undefined ? (
          <div className="p-4"><TableSkeleton rows={5} cols={4} /></div>
        ) : lista.length === 0 ? (
          <Empty icon={<Icon.ClipboardList size={22} />}
            title="Aún no hay cierres Z en esta sede"
            body="El Z consolida las ventas del día. Emite el primero cuando haya documentos por cerrar." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm tbl-sticky">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 pr-3 font-medium">Nº Z</th>
                  <th className="py-2.5 pr-3 font-medium">Fecha</th>
                  <th className="py-2.5 pr-3 font-medium">Rango de folios</th>
                  <th className="py-2.5 pr-3 font-medium text-center">Docs</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Total neto</th>
                  <th className="py-2.5 pr-3 font-medium text-right"></th>
                </tr>
              </thead>
              <tbody>
                {lista.map((z) => {
                  const t = z.totales || {}
                  const docs = (t.cantidadFacturas || 0) + (t.cantidadNotas || 0) + (t.cantidadAnulaciones || 0)
                  return (
                    <tr key={z.id} className="border-b border-slate-100 dark:border-slate-800/70 row-hover cursor-pointer" onClick={() => setDetalle(z)}>
                      <td className="py-2.5 pr-3 num text-[12.5px] font-medium">{z.numeroCompleto}</td>
                      <td className="py-2.5 pr-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(z.fecha)}</td>
                      <td className="py-2.5 pr-3 num text-[12px] text-slate-500">
                        {z.docDesde ? <span className="truncate">{z.docDesde} → {z.docHasta}</span> : <span className="text-slate-400">—</span>}
                      </td>
                      <td className="py-2.5 pr-3 text-center num text-[12.5px] text-slate-500">{fmtNum(docs, 0)}</td>
                      <td className="py-2.5 pr-3 text-right num font-medium private-mask">{fmtCurrency(t.totalNeto, ui.ccy)}</td>
                      <td className="py-2.5 pr-3 text-right"><Icon.ChevRight size={15} className="inline text-slate-300" /></td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {(lista || []).length ? <div className="mt-2 text-[11.5px] text-slate-400">{fmtNum(lista.length, 0)} cierre(s) Z</div> : null}

      {confirmar ? (
        <ConfirmarZ preview={preview} sede={sede} ccy={ui.ccy} toast={toast}
          onClose={() => setConfirmar(false)}
          onEmitted={async () => { setConfirmar(false); await cargar() }} />
      ) : null}
    </div>
  )
}

// Desglose de un Z ya emitido a PANTALLA COMPLETA: todos los campos de TotalesZ y
// el rango cubierto. Reemplaza la lista; se vuelve con «‹ Volver».
function DetalleZ({ z, ccy, onVolver }) {
  const t = z.totales || {}
  return (
    <VistaDetalle onVolver={onVolver} icon={<Icon.ClipboardList size={18} />} maxWidth="max-w-4xl"
      titulo={z.numeroCompleto} sub={`Cierre Z · ${fmtDate(z.fecha)}`}>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
        {/* Columna izquierda: estado y rango de folios */}
        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3">
            <div className="flex items-center gap-2">
              <Badge color="huberp">Cierre Z</Badge>
              <Badge color="emerald" dot>Emitido</Badge>
            </div>
            <div className="grid grid-cols-2 gap-3 text-[12.5px]">
              <RangoBox label="Desde" folio={z.docDesde} fecha={z.desdeFecha} />
              <RangoBox label="Hasta" folio={z.docHasta} fecha={z.hastaFecha} />
            </div>
          </div>

          <div className="flex items-start gap-2 text-[11.5px] text-slate-500 bg-slate-50 dark:bg-slate-800/60 rounded-lg px-3 py-2">
            <Icon.Lock size={13} className="mt-0.5 shrink-0" />
            <span>El Cierre Z es inmutable: consolida el ledger de documentos y no se edita. Cualquier corrección va en un Z posterior.</span>
          </div>
        </div>

        {/* Columna derecha: totales del cierre */}
        <div>
          <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Totales del cierre</div>
          <ZTotales t={t} ccy={ccy} />
        </div>
      </div>
    </VistaDetalle>
  )
}

function RangoBox({ label, folio, fecha }) {
  return (
    <div className="rounded-lg border border-slate-200 dark:border-slate-700 px-3 py-2">
      <div className="text-[10.5px] uppercase tracking-wide text-slate-400">{label}</div>
      <div className="text-[12.5px] font-medium num truncate">{folio || '—'}</div>
      {fecha ? <div className="text-[11px] text-slate-400 num">{fmtDate(fecha)}</div> : null}
    </div>
  )
}

// Tabla de totales de un Z (o de un preview): comparten TotalesZ.
function ZTotales({ t, ccy }) {
  return (
    <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 p-3 space-y-1.5 text-[13px]">
      <Row label={`Ventas gravadas${t.cantidadFacturas ? ` (${fmtNum(t.cantidadFacturas, 0)} fact.)` : ''}`} value={t.ventasGravadas} ccy={ccy} />
      <Row label="Ventas exentas" value={t.ventasExentas} ccy={ccy} />
      <Row label="IVA débito" value={t.ivaDebito} ccy={ccy} />
      {t.igtf ? <Row label="IGTF" value={t.igtf} ccy={ccy} /> : null}
      <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5">
        <Row label="Total ventas" value={t.totalVentas} ccy={ccy} strong />
      </div>
      {t.notasCredito ? <Row label={`Notas de crédito${t.cantidadNotas ? ` (${fmtNum(t.cantidadNotas, 0)})` : ''}`} value={-t.notasCredito} ccy={ccy} neg /> : null}
      {t.anulaciones ? <Row label={`Anulaciones${t.cantidadAnulaciones ? ` (${fmtNum(t.cantidadAnulaciones, 0)})` : ''}`} value={-t.anulaciones} ccy={ccy} neg /> : null}
      <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
        <span className="font-semibold">Total neto</span>
        <span className="num font-semibold private-mask">{fmtCurrency(t.totalNeto, ccy)}</span>
      </div>
    </div>
  )
}

const Row = ({ label, value, ccy, strong, neg }) => (
  <div className="flex items-center justify-between">
    <span className={strong ? 'font-medium' : 'text-slate-500'}>{label}</span>
    <span className={`num private-mask ${neg ? 'text-amber-700 dark:text-amber-300' : strong ? 'font-medium' : ''}`}>{fmtCurrency(value, ccy)}</span>
  </div>
)

// Confirmación de emisión: muestra el PREVIEW (lo que se consolidaría ahora) y
// advierte que es irreversible antes de persistir.
function ConfirmarZ({ preview, sede, ccy, toast, onClose, onEmitted }) {
  const [busy, setBusy] = useState(false)
  const t = preview?.totales || {}
  const cantidad = preview?.cantidadDocumentos || 0

  const emitir = async () => {
    setBusy(true)
    try {
      const z = await api.emitirCierreZ()
      toast({ title: 'Cierre Z emitido', body: `Se generó ${z?.numeroCompleto || ''}.` })
      await onEmitted()
    } catch (e) {
      toast({ title: 'No se pudo emitir el Cierre Z', body: e?.message || 'Error', kind: 'error' })
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="md" icon={<Icon.ClipboardList size={18} />}
      title="Emitir Cierre Z" sub={sede?.nombre}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={emitir} loading={busy} disabled={cantidad === 0} icon={<Icon.ClipboardList size={16} />}>Emitir Cierre Z</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="flex items-start gap-2 text-[12.5px] text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/20 rounded-lg px-3 py-2.5">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>Acción irreversible. El Cierre Z consolida {fmtNum(cantidad, 0)} documento(s) emitidos desde el último Z y no se puede editar; para corregir se emite un Z posterior.</span>
        </div>
        <div>
          <div className="text-[11px] uppercase tracking-wide text-slate-400 mb-1.5">Se consolidará</div>
          <ZTotales t={t} ccy={ccy} />
        </div>
      </div>
    </Modal>
  )
}
