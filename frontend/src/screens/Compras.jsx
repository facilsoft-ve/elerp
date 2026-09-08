import { PageHeader } from '../components/primitives.jsx'
import { subLabelFor } from '../components/nav.js'
import { ComprasOrdenes } from './ComprasOrdenes.jsx'
import { ComprasProveedores } from './ComprasProveedores.jsx'
import { ComprasSolicitudes } from './ComprasSolicitudes.jsx'
import { ListasPrecio } from './ListasPrecio.jsx'

/* Contenedor del módulo Compras. El submenú (ver nav.js) está agrupado:
 * Abastecimiento (el flujo de compra) · Maestros (datos base). Rutas
 * "compras:<sub>"; la navegación la maneja el menú lateral (patrón de Facturación).
 *
 * Todo el flujo de abastecimiento está construido: solicitudes de presupuesto
 * (RFQ) → órdenes de compra → recepción, más los maestros (proveedores, tarifas). */

export function Compras({ route, navigate }) {
  const sub = (route || 'compras').split(':')[1] || 'ordenes'

  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader
        breadcrumb={['Compras', subLabelFor(route) || 'Órdenes de compra']}
        title="Compras"
        sub="Presupuestos, órdenes de compra, proveedores y tarifas. La recepción de mercancía vive en Inventario." />

      {/* Abastecimiento */}
      {sub === 'solicitudes' ? <ComprasSolicitudes navigate={navigate} /> : null}
      {sub === 'ordenes' ? <ComprasOrdenes /> : null}

      {/* Maestros */}
      {sub === 'proveedores' ? <ComprasProveedores /> : null}
      {sub === 'lista-precio-compras' ? <ListasPrecio tipo="compra" /> : null}
    </div>
  )
}
