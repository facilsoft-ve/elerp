import { PageHeader } from '../components/primitives.jsx'
import { subLabelFor } from '../components/nav.js'
import { Cotizaciones } from './Cotizaciones.jsx'
import { Clientes } from './Clientes.jsx'
import { Catalogo } from './Catalogo.jsx'
import { ListasPrecio } from './ListasPrecio.jsx'
import { Cupones } from './Cupones.jsx'

/* Contenedor del módulo Ventas. El submenú (ver nav.js) está agrupado por
 * naturaleza: Catálogo y precios · Ventas. Rutas "ventas:<sub>"; la navegación
 * entre sub-vistas la maneja el menú lateral (mismo patrón que Facturación).
 *
 * Reutiliza pantallas existentes (un solo modelo, sin duplicar):
 *   · catalogo    → el catálogo REAL de Inventario (mismo componente).
 *   · cotizaciones→ la bandeja de Cotizaciones filtrada a las SIN confirmar.
 *   · pedidos     → la MISMA bandeja filtrada a las confirmadas/facturadas.
 *   · clientes    → la cartera de clientes.
 *
 * La biblioteca de promociones y la pantalla del cliente viven ahora en
 * Configuración › Marketing (un solo lugar para el espacio promocional). */

export function Ventas({ route, navigate }) {
  const sub = (route || 'ventas').split(':')[1] || 'catalogo'

  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader
        breadcrumb={['Ventas', subLabelFor(route) || 'Catálogo de producto']}
        title="Ventas"
        sub="Catálogo y precios, el ciclo cotización → pedido → factura y la cartera de clientes." />

      {/* Catálogo y precios */}
      {sub === 'catalogo' ? <Catalogo onKardex={(sku) => navigate?.('kardex:' + sku)} /> : null}
      {sub === 'lista-precio' ? <ListasPrecio tipo="venta" /> : null}
      {sub === 'cupones' ? <Cupones /> : null}

      {/* Ventas: una sola bandeja de cotizaciones, filtrada por etapa del ciclo. */}
      {sub === 'cotizaciones' ? <Cotizaciones navigate={navigate} etapaInicial="cotizacion" /> : null}
      {sub === 'pedidos' ? <Cotizaciones navigate={navigate} etapaInicial="pedido" /> : null}
      {sub === 'clientes' ? <Clientes /> : null}
    </div>
  )
}
