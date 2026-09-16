/* Mesas que pidieron factura, dentro del Punto de Venta.
 *
 * El módulo Restaurante NO tiene su propia pantalla de cobro: cobrar es cobrar, y el
 * lugar donde se cobra es el mostrador. Cuando el mesonero pide la cuenta de una mesa
 * —entera o del segmento de un comensal— acá aparece esa solicitud; el cajero la elige
 * y sus renglones caen en el carrito del POS como cualquier venta, con la etiqueta de
 * la mesa encima. De ahí en adelante todo es el POS de siempre: cobro mixto,
 * multimoneda, IGTF, vuelto y la misma ruta fiscal.
 *
 * El cajero no decide qué llamar y qué no: solo ve lo que alguien pidió facturar.
 */
import { useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Modal, Empty, Badge } from '../components/primitives.jsx'
import { fmtCurrency } from '../lib/format.js'
import { solicitudesDeMesa } from '../lib/restaurante.js'
import { useData } from '../context/DataContext.jsx'

/** useSolicitudesMesa devuelve las solicitudes de facturación pendientes de cobro.
 *  Vacío cuando el módulo Restaurante no está activo: el POS queda idéntico al de
 *  un comercio normal, sin un botón que no significa nada. */
export function useSolicitudesMesa() {
  const { db } = useData()
  const activo = Array.isArray(db?.MODULOS) && db.MODULOS.includes('restaurante')
  return useMemo(
    () => (activo ? solicitudesDeMesa(db.COTIZACIONES) : []),
    [activo, db.COTIZACIONES],
  )
}

/** Botón que anuncia cuántas mesas están esperando en la caja. */
export function BotonMesasPorCobrar({ cantidad, onClick }) {
  if (!cantidad) return null
  return (
    <Button variant="secondary" size="lg" icon={<Icon.Utensils size={17} />} onClick={onClick}
      title="Mesas que pidieron factura">
      Mesas · {cantidad}
    </Button>
  )
}

/** Etiqueta del carrito cuando lo que se está cobrando es una mesa. */
export function EtiquetaMesa({ solicitud, onQuitar }) {
  if (!solicitud) return null
  return (
    <div className="flex items-center gap-2.5 px-3 py-2 border-b border-slate-200 dark:border-slate-800 bg-elerp-50/70 dark:bg-elerp-900/25">
      <Icon.Utensils size={15} className="text-elerp-600 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="text-[13px] font-semibold truncate">
          Mesa {solicitud.mesaNombre || '—'}
          {solicitud.clienteNombre ? <span className="font-normal text-slate-500"> · {solicitud.clienteNombre}</span> : null}
        </div>
        <div className="text-[11.5px] text-slate-500 truncate">{solicitud.nota || solicitud.numero}</div>
      </div>
      <button onClick={onQuitar} title="Soltar la mesa y volver a una venta normal"
        className="p-1.5 rounded-md text-slate-400 hover:text-red-500 ring-focus">
        <Icon.X size={15} />
      </button>
    </div>
  )
}

/** Lista para elegir qué mesa se cobra. */
export function MesasPorCobrarModal({ open, solicitudes, onElegir, onClose }) {
  return (
    <Modal open={open} onClose={onClose} size="md" icon={<Icon.Utensils size={18} />}
      title="Mesas que pidieron factura"
      sub="Elige una y sus productos pasan al carrito para cobrarlos acá mismo."
      footer={<Button variant="ghost" onClick={onClose}>Cerrar</Button>}>
      {solicitudes.length === 0 ? (
        <Empty icon={<Icon.Utensils size={22} />} title="Ninguna mesa pidió factura todavía"
          body="Cuando un mesonero pida la cuenta de una mesa —completa o de un comensal— aparece acá." />
      ) : (
        <div className="space-y-1.5">
          {solicitudes.map((s) => (
            <button key={s.id} onClick={() => onElegir(s)}
              className="w-full flex items-center gap-3 px-3 py-2.5 rounded-lg border border-slate-200 dark:border-slate-800 hover:border-elerp-400 hover:bg-elerp-50/60 dark:hover:bg-elerp-900/25 text-left ring-focus">
              <div className="w-11 h-11 rounded-lg bg-elerp-50 dark:bg-elerp-900/40 flex items-center justify-center shrink-0">
                <span className="font-display font-bold text-[15px] text-elerp-600">{s.mesaNombre || '—'}</span>
              </div>
              <div className="min-w-0 flex-1">
                <div className="text-[13.5px] font-medium truncate">{s.nota || `Mesa ${s.mesaNombre}`}</div>
                <div className="text-[11.5px] text-slate-500 truncate">
                  {s.lineas.length} renglón(es)
                  {s.clienteNombre ? ` · ${s.clienteNombre}` : ' · sin datos del cliente'}
                </div>
              </div>
              <div className="text-right shrink-0">
                <div className="num text-[14px] font-semibold">{fmtCurrency(s.total, 'VES')}</div>
                {s.clienteNombre ? null : <Badge size="sm" color="slate">Pide el RIF</Badge>}
              </div>
            </button>
          ))}
        </div>
      )}
    </Modal>
  )
}
