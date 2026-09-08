import { Icon } from './Icon.jsx'

/* Mapa de navegación — nombres y agrupación tal como los muestra el prototipo:
 * OPERACIÓN · FINANZAS · GESTIÓN, con el nombre completo de cada módulo
 * ("Inventario y Operaciones", no "Inventario"). `glyph` es el glifo oficial de
 * la familia de iconos de módulo.
 *
 * `roles` traduce la matriz de permisos de 02 §1. La UI solo oculta: el backend
 * valida por rol + empresa + sede en cada endpoint.
 */
const TODOS = ['dueno', 'desarrollador', 'vendedor', 'cajero', 'contadora']

/* `subs` son los submódulos desplegables del acordeón del menú lateral. La ruta
 * de un submódulo es "<moduloId>:<subId>"; cada `subId` COINCIDE con el id de
 * pestaña que ya usa el contenedor del módulo (así el sidebar y la sub-navegación
 * del contenido enrutan al mismo sitio). Módulos sin `subs` (Inicio, Punto de
 * Venta) no se expanden. */
export const NAV = [
  // Inicio NO es para el cajero: su pantalla es la caja (R2). Un lanzador de
  // módulos en el puesto de cobro solo estorba y ofrece lo que no puede abrir.
  { id: 'dashboard', label: 'Inicio', grupo: '', glyph: Icon.Home, roles: ['dueno', 'desarrollador', 'vendedor', 'contadora', 'mesonero'], ready: true },
  // Punto de Venta: LANZADOR del puesto de cobro (abre el Modo caja a pantalla
  // completa vía App.jsx). Va JUSTO DEBAJO de Inicio; el Sidebar lo resalta como
  // acción especial (no es un módulo administrativo, es la puerta al mostrador).
  { id: 'pos', label: 'Punto de Venta', grupo: '', glyph: Icon.Cart, roles: TODOS, ready: true, launcher: true },

  // Módulo Restaurante (comercializable): solo visible si está activo en la empresa
  // (`modulo` gatea el módulo del menú, igual que `modulo` gatea un sub).
  {
    id: 'restaurante', label: 'Restaurante', grupo: 'OPERACIÓN', glyph: Icon.Utensils, roles: [...TODOS, 'mesonero'], ready: true, modulo: 'restaurante',
    // El MESONERO alcanza el módulo pero solo la Comandera: el mapa, la cocina, las
    // recetas y la impresora son de administración o de cocina. Los subs sin `roles`
    // los ve cualquier rol que alcance el módulo.
    subs: [
      // El tablero es la PRIMERA pantalla del módulo para quien administra: responde
      // cómo está el salón, qué pasa en cocina y cómo va el día, y salta a cada sección.
      // El mesonero no lo tiene: él solo alcanza la Comandera.
      { id: 'inicio', label: 'Resumen del salón', grupo: 'Salón', roles: TODOS },
      { id: 'comandera', label: 'Comandera', grupo: 'Salón' },
      { id: 'mesas', label: 'Mapa de mesas', grupo: 'Salón', roles: TODOS },
      { id: 'mesoneros', label: 'Mesoneros y asignación', grupo: 'Salón', roles: TODOS },
      { id: 'cocina', label: 'Cocina', grupo: 'Cocina', roles: TODOS },
      { id: 'platos', label: 'Platos y recetas', grupo: 'Cocina', roles: TODOS },
      { id: 'impresora', label: 'Impresoras comanderas', grupo: 'Configuración', roles: TODOS },
    ],
  },

  {
    id: 'facturacion', label: 'Facturación', grupo: 'OPERACIÓN', glyph: Icon.ModFiscal, roles: TODOS, ready: true,
    // Submenú agrupado por naturaleza del documento: Cliente (ventas) ·
    // Proveedores (compras) · Reportes fiscales. `grupo` dibuja un encabezado en
    // el sidebar; el id de cada sub enruta en el contenedor Facturacion.
    subs: [
      { id: 'factura', label: 'Facturas', grupo: 'Cliente' },
      { id: 'nota-credito', label: 'Notas de crédito', grupo: 'Cliente' },
      { id: 'nota-debito', label: 'Notas de débito', grupo: 'Cliente' },
      { id: 'factura-proveedor', label: 'Facturas de proveedor', grupo: 'Proveedores' },
      { id: 'nc-proveedor', label: 'Notas de crédito (prov.)', grupo: 'Proveedores' },
      { id: 'nd-proveedor', label: 'Notas de débito (prov.)', grupo: 'Proveedores' },
      { id: 'cxp', label: 'Cuentas por pagar', grupo: 'Proveedores' },
      { id: 'libros-venta', label: 'Libro de ventas', grupo: 'Reportes fiscales' },
      { id: 'libros-compra', label: 'Libro de compras', grupo: 'Reportes fiscales' },
      { id: 'libro-inventario', label: 'Libro de inventario', grupo: 'Reportes fiscales' },
      { id: 'retenciones', label: 'Retenciones', grupo: 'Reportes fiscales' },
      { id: 'cierres-z', label: 'Cierres Z', grupo: 'Reportes fiscales' },
    ],
  },
  {
    id: 'ventas', label: 'Ventas', grupo: 'OPERACIÓN', glyph: Icon.Users, roles: ['dueno', 'desarrollador', 'vendedor', 'contadora'], ready: true,
    // Submenú agrupado: Catálogo y precios (un solo modelo con Inventario) ·
    // Ventas (el ciclo cotización → pedido → factura + cartera de clientes).
    subs: [
      { id: 'catalogo', label: 'Catálogo de producto', grupo: 'Catálogo y precios' },
      { id: 'lista-precio', label: 'Lista de precio', grupo: 'Catálogo y precios' },
      { id: 'cupones', label: 'Cupones', grupo: 'Catálogo y precios' },
      { id: 'cotizaciones', label: 'Cotizaciones', grupo: 'Ventas' },
      { id: 'pedidos', label: 'Pedido de ventas', grupo: 'Ventas' },
      { id: 'clientes', label: 'Clientes', grupo: 'Ventas' },
    ],
  },
  {
    id: 'inventario', label: 'Inventario', grupo: 'OPERACIÓN', glyph: Icon.ModInventario, roles: ['dueno', 'desarrollador', 'vendedor', 'contadora'], ready: true,
    subs: [
      { id: 'catalogo', label: 'Catálogo' },
      { id: 'existencias', label: 'Existencias' },
      { id: 'recepcion', label: 'Recepción' },
      { id: 'kardex', label: 'Kardex' },
      { id: 'movimientos', label: 'Movimientos' },
      { id: 'transferencias', label: 'Transferencias' },
    ],
  },
  {
    id: 'compras', label: 'Compras', grupo: 'OPERACIÓN', glyph: Icon.Cart, roles: ['dueno', 'desarrollador', 'contadora'], ready: true,
    // Submenú agrupado: Abastecimiento (el flujo de compra) · Maestros (datos base).
    subs: [
      { id: 'ordenes', label: 'Órdenes de compra', grupo: 'Abastecimiento' },
      { id: 'solicitudes', label: 'Solicitudes de presupuesto', grupo: 'Abastecimiento' },
      { id: 'proveedores', label: 'Proveedores', grupo: 'Maestros' },
      { id: 'lista-precio-compras', label: 'Lista de precio de compras', grupo: 'Maestros' },
    ],
  },

  {
    id: 'contabilidad', label: 'Contabilidad', grupo: 'FINANZAS', glyph: Icon.ModContabilidad, roles: ['dueno', 'desarrollador', 'contadora'], ready: true,
    subs: [
      { id: 'plan', label: 'Plan de cuentas' },
      { id: 'diario', label: 'Libro diario' },
      { id: 'estados', label: 'Estados financieros' },
    ],
  },
  {
    id: 'finanzas', label: 'Finanzas', grupo: 'FINANZAS', glyph: Icon.ModTesoreria, roles: ['dueno', 'desarrollador', 'contadora', 'vendedor'], ready: true,
    subs: [
      { id: 'cxc', label: 'Cuentas por cobrar' },
      { id: 'cxp', label: 'Cuentas por pagar' },
      { id: 'saldos', label: 'Efectivo y bancos' },
      { id: 'igtf', label: 'IGTF' },
      { id: 'links', label: 'Enlaces de pago' },
    ],
  },

  {
    id: 'reportes', label: 'Reportes y BI', grupo: 'GESTIÓN', glyph: Icon.ModReportes, roles: ['dueno', 'desarrollador', 'contadora'], ready: true,
    subs: [
      { id: 'panel', label: 'Panel ejecutivo' },
      { id: 'ventas', label: 'Ventas' },
      { id: 'inventario', label: 'Inventario' },
      { id: 'compras', label: 'Compras' },
      { id: 'cobranza', label: 'Cobranza' },
      { id: 'actividad', label: 'Registro de actividad' },
    ],
  },
  // Anclados al pie del menú. Aplicaciones (marketplace de módulos) va resaltada con
  // su color propio, como una acción especial; Configuración es un módulo normal
  // (expandible); "Sistema de diseño" es la referencia visual y queda también al pie.
  {
    id: 'aplicaciones', label: 'Aplicaciones', grupo: 'pie', glyph: Icon.Package,
    roles: ['dueno', 'desarrollador'], ready: true,
    // `resalte` pinta el ítem con un acento propio (ver Sidebar.NavItem): violeta,
    // distinto del teal del Punto de Venta y del navy del ítem activo.
    resalte: 'violet',
  },
  {
    id: 'config', label: 'Configuración', grupo: 'pie', glyph: Icon.ModConfig, roles: ['dueno', 'desarrollador'], ready: true,
    // Submenú agrupado por tema (el Sidebar dibuja un encabezado por `grupo`, igual
    // que en Facturación/Ventas/Compras): Empresa (identidad, sedes, gente) · Fiscal
    // (impuestos, series, moneda, dispositivos) · Punto de venta (cajas, comportamiento
    // del POS, métodos de cobro) · Comercial (marketing) · Plataforma (integraciones).
    // Los `label` COINCIDEN con los de las pestañas de Configuracion.jsx (TABS).
    subs: [
      { id: 'empresa', label: 'Datos de empresa', grupo: 'Empresa' },
      { id: 'sedes', label: 'Sedes', grupo: 'Empresa' },
      { id: 'usuarios', label: 'Usuarios y roles', grupo: 'Empresa' },
      { id: 'unidades', label: 'Unidades de medida', grupo: 'Empresa' },
      { id: 'almacenes', label: 'Almacenes', grupo: 'Empresa' },
      { id: 'impuestos', label: 'Impuestos y alícuotas', grupo: 'Fiscal' },
      { id: 'numeracion', label: 'Series y numeración', grupo: 'Fiscal' },
      { id: 'formatos', label: 'Formatos de documento', grupo: 'Fiscal' },
      { id: 'moneda', label: 'Moneda y tasa', grupo: 'Fiscal' },
      { id: 'dispositivos', label: 'Dispositivos fiscales', grupo: 'Fiscal' },
      { id: 'cajas', label: 'Cajas y sesiones', grupo: 'Punto de venta' },
      { id: 'punto-venta', label: 'Punto de venta', grupo: 'Punto de venta' },
      { id: 'metodos', label: 'Métodos de pago', grupo: 'Punto de venta' },
      // `modulo` gatea el sub por el módulo de Aplicaciones: si Marketing no está
      // activo, esta entrada NO aparece en el submenú (ver Sidebar).
      { id: 'marketing', label: 'Marketing', grupo: 'Comercial', modulo: 'marketing' },
      { id: 'asistente', label: 'Asistente IA', grupo: 'Plataforma', modulo: 'asistente-ia' },
      { id: 'integraciones', label: 'Integraciones', grupo: 'Plataforma' },
    ],
  },
  // Pantalla de referencia interna del sistema visual. Solo `desarrollador`
  // (modo dev): NO debe ser visible para dueños, cajeros ni el demo público.
  { id: 'diseno', label: 'Sistema de diseño', grupo: 'pie', glyph: Icon.Sparkles, roles: ['desarrollador'], ready: true },
]

// Orden de los grupos en el menú lateral.
export const GRUPOS = ['', 'OPERACIÓN', 'FINANZAS', 'GESTIÓN']

export const baseRoute = (route) => {
  const base = (route || '').split(':')[0]
  if (base === 'kardex') return 'inventario'
  return base
}

export const labelFor = (route) => {
  const item = NAV.find((n) => n.id === baseRoute(route))
  return item ? item.label : 'ElERP'
}

// subId de la ruta activa ("<mod>:<sub>" → "<sub>"). Para "kardex:<sku>" es "kardex".
export const subOf = (route) => {
  const [head, arg] = (route || '').split(':')
  if (head === 'kardex') return 'kardex'
  return arg || ''
}

// Etiqueta del submódulo activo (para el breadcrumb del contenido).
export const subLabelFor = (route) => {
  const item = NAV.find((n) => n.id === baseRoute(route))
  const sub = subOf(route)
  const found = (item?.subs || []).find((s) => s.id === sub)
  return found ? found.label : ''
}
