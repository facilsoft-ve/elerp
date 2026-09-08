import { Icon } from '../components/Icon.jsx'
import { PageHeader } from '../components/primitives.jsx'
import { subLabelFor } from '../components/nav.js'
import { Documentos } from './Documentos.jsx'
import { Retenciones } from './Retenciones.jsx'
import { CierresZ } from './CierresZ.jsx'
import { LibrosFiscales } from './LibrosFiscales.jsx'
import { FacturasProveedor } from './FacturasProveedor.jsx'
import { NotasCompra } from './NotasCompra.jsx'
import { LibroInventario } from './LibroInventario.jsx'
import { PorPagar } from './Tesoreria.jsx'

/* Contenedor del módulo Facturación. El submenú (ver nav.js) está agrupado por
 * naturaleza del documento: Cliente (ventas) · Proveedores (compras) · Reportes
 * fiscales. Rutas "facturacion:<sub>". La navegación entre sub-vistas la maneja
 * el menú lateral; aquí solo se resuelve qué pantalla renderizar.
 *
 * Reutiliza pantallas existentes (un solo modelo, sin duplicar): Documentos
 * filtrado por tipo, Libros fiscales por libro, y Cuentas por pagar de Tesorería.
 * Lo que aún no está construido va como SubPlaceholder honesto. */

// Sub-vistas todavía no construidas (nav-only). Ya no queda ninguna en Facturación:
// notas de crédito/débito de proveedor y factura de proveedor están construidas.
const PLACEHOLDERS = {}

export function Facturacion({ route }) {
  const sub = (route || 'facturacion').split(':')[1] || 'factura'

  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader
        breadcrumb={['Facturación', subLabelFor(route) || 'Facturas']}
        title="Facturación"
        sub="Documentos de venta y compra, retenciones, cierres Z y libros fiscales con export SENIAT." />

      {/* Cliente */}
      {sub === 'factura' ? <Documentos tipo="factura" /> : null}
      {sub === 'nota-credito' ? <Documentos tipo="nota_credito" /> : null}
      {sub === 'nota-debito' ? <Documentos tipo="nota_debito" /> : null}

      {/* Proveedores */}
      {sub === 'factura-proveedor' ? <FacturasProveedor /> : null}
      {sub === 'nc-proveedor' ? <NotasCompra tipo="nota_credito" /> : null}
      {sub === 'nd-proveedor' ? <NotasCompra tipo="nota_debito" /> : null}
      {sub === 'cxp' ? <PorPagar /> : null}

      {/* Reportes fiscales */}
      {sub === 'libros-venta' ? <LibrosFiscales libroInicial="ventas" /> : null}
      {sub === 'libros-compra' ? <LibrosFiscales libroInicial="compras" /> : null}
      {sub === 'libro-inventario' ? <LibroInventario /> : null}
      {sub === 'retenciones' ? <Retenciones /> : null}
      {sub === 'cierres-z' ? <CierresZ /> : null}

      {/* Aún no construido (nav-only) */}
      {PLACEHOLDERS[sub] ? <SubPlaceholder label={subLabelFor(route)} {...PLACEHOLDERS[sub]} /> : null}
    </div>
  )
}

// Placeholder de sub-pestaña en construcción (mismo lenguaje visual que Placeholder).
export function SubPlaceholder({ label, icon, desc }) {
  const IconC = icon || Icon.Layers
  return (
    <div className="flex flex-col items-center text-center py-16">
      <div className="h-14 w-14 rounded-2xl bg-elerp-50 dark:bg-elerp-900/40 text-elerp-500 inline-flex items-center justify-center mb-4"><IconC size={26} /></div>
      <h3 className="text-lg font-semibold tracking-tight font-display">{label}</h3>
      <div className="mt-2 inline-flex items-center gap-1.5 text-[12px] font-medium text-amber-700 bg-amber-50 dark:bg-amber-900/30 dark:text-amber-300 px-2.5 py-1 rounded-full">
        <Icon.Clock size={13} /> Próximamente
      </div>
      {desc ? <p className="text-sm text-slate-500 mt-3 max-w-md">{desc}</p> : null}
    </div>
  )
}
