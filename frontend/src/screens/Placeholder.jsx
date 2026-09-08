import { Icon } from '../components/Icon.jsx'
import { NAV, baseRoute, labelFor } from '../components/nav.js'

// Funciones planificadas por módulo (del SAD y del documento de flujos).
const PLANES = {
  compras: {
    icon: Icon.Cart,
    desc: 'Órdenes de compra y recepción de mercancía.',
    features: ['Órdenes de compra con aprobación por umbral', 'Comparador de cotizaciones', 'Escaneo de factura de proveedor con IA (humano confirma)', 'Recepción y actualización de inventario'],
  },
  contabilidad: {
    icon: Icon.Book,
    desc: 'Plan de cuentas, libro diario append-only y estados financieros.',
    features: ['Plan de cuentas jerárquico precargado por giro', 'Libro diario append-only (asientos, reversas — nunca edición)', 'Estados financieros en tiempo real', 'Cierre de períodos'],
  },
  tesoreria: {
    icon: Icon.Wallet,
    desc: 'Cobros, IGTF, CxC/CxP y conciliación.',
    features: ['Cuentas de cobro (Pago Móvil, Zelle, banco, punto de venta)', 'Cobro mixto multi-cuenta con IGTF 3%', 'CxC / CxP', 'Conciliación pago ↔ factura ↔ asiento'],
  },
  reportes: {
    icon: Icon.Chart,
    desc: 'Rentabilidad, KPIs y reportes fiscales exportables.',
    features: ['Rentabilidad y KPIs (réplica analítica)', 'Reportes fiscales (Art. 177, IGTF, libros)', 'Export a Excel/PDF reales'],
  },
  config: {
    icon: Icon.ModConfig,
    desc: 'Usuarios y roles, dispositivos fiscales e integraciones.',
    features: ['Usuarios y roles (matriz de permisos)', 'Dispositivos fiscales e integraciones', 'Seguridad de caja (PIN de supervisor)', 'Marketplace de módulos'],
  },
}

export function Placeholder({ route }) {
  const base = baseRoute(route)
  const plan = PLANES[base] || { icon: Icon.Layers, desc: 'Módulo en construcción.', features: [] }
  const IconC = plan.icon
  const roles = NAV.find((n) => n.id === base)?.roles || []

  return (
    <div className="p-4 md:p-6">
      <div className="max-w-2xl mx-auto">
        <div className="flex flex-col items-center text-center py-8">
          <div className="h-16 w-16 rounded-2xl bg-elerp-50 dark:bg-elerp-900/40 text-elerp-500 inline-flex items-center justify-center mb-4">
            <IconC size={30} />
          </div>
          <h2 className="text-xl font-semibold tracking-tight font-display">{labelFor(route)}</h2>
          <div className="mt-2 inline-flex items-center gap-1.5 text-[12px] font-medium text-amber-700 bg-amber-50 dark:bg-amber-900/30 dark:text-amber-300 px-2.5 py-1 rounded-full">
            <Icon.Clock size={13} /> Módulo en construcción
          </div>
          <p className="text-sm text-slate-500 mt-3 max-w-md">{plan.desc}</p>
        </div>

        {plan.features.length ? (
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-6">
            <div className="text-[13px] font-semibold text-slate-700 dark:text-slate-200 mb-3">Funciones planificadas</div>
            <ul className="space-y-2.5">
              {plan.features.map((f) => (
                <li key={f} className="flex items-start gap-2.5 text-[13.5px] text-slate-600 dark:text-slate-300">
                  <Icon.Check size={16} className="mt-0.5 shrink-0 text-teal-500" /> <span>{f}</span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>
    </div>
  )
}
