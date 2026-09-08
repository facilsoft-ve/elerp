import { createContext, useContext, useState, useEffect, useCallback, useRef } from 'react'
import { api, setActiveEmpresa as setApiEmpresa, setActiveSede as setApiSede } from '../lib/api.js'
import { useAuth } from './AuthContext.jsx'

const DataCtx = createContext(null)
export const useData = () => useContext(DataCtx)

// Carga los datos de la empresa/sede activa y los expone con un shape tipo `db`
// (espejo del objeto `db` del backend/inmem). Recarga al cambiar de empresa o
// sede. El Kardex NO se precarga: se pide on-demand por SKU (api.kardex).
export function DataProvider({ children }) {
  const { activeEmpresaId, activeSedeId, activeSede } = useAuth()
  const [db, setDb] = useState({})
  // `loading` es SOLO la primera carga (la que justifica una pantalla de espera).
  // Un refresco posterior no vuelve a ponerlo en true: si lo hiciera, el Shell
  // desmontaría la pantalla activa y se perdería lo que el usuario tiene en
  // pantalla — es exactamente lo que hacía desaparecer la factura recién emitida
  // en el punto de venta (parpadeo y carrito vacío, sin mostrar el resultado).
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const cargadoAlguna = useRef(false)
  // Empresa a la que pertenece el `db` que está en memoria. Cambiar de empresa NO es un
  // refresco: TODO el db pasa a ser de otro tenant (catálogo, documentos, módulos
  // activos…), así que mostrarlo mientras llega el nuevo es mostrar datos de otra
  // empresa. En ese caso se vacía y se bloquea con la pantalla de carga.
  const empresaDelDb = useRef(null)
  const [error, setError] = useState(null)

  const load = useCallback(async () => {
    if (!activeEmpresaId) { setLoading(false); return }
    const cambioDeEmpresa = empresaDelDb.current !== null && empresaDelDb.current !== activeEmpresaId
    if (cambioDeEmpresa) {
      setDb({})
      setRefreshing(false)
      setLoading(true)
      // Se vuelve a tratar como primera carga: la de esta empresa.
      cargadoAlguna.current = false
      empresaDelDb.current = null
    }
    // Esperar a que AuthContext auto-seleccione la sede: sin X-Sede-ID la
    // consulta de existencias devuelve 400 y rompería toda la carga (carrera
    // entre el montaje de DataProvider y el efecto que fija la sede).
    if (!activeSedeId) { setLoading(true); return }
    if (cargadoAlguna.current) setRefreshing(true)
    else setLoading(true)
    setError(null)
    // Garantizar los headers X-Empresa-ID / X-Sede-ID antes de las llamadas
    // (evita carreras con los efectos de sincronización de AuthContext).
    setApiEmpresa(activeEmpresaId)
    setApiSede(activeSedeId)
    try {
      const [boot, productos, existencias, transferencias, almacenes, clientes, cuentasCobro, documentos, metodosPago, cotizaciones, proveedores, ordenesCompra, facturasCompra, tasas, dispositivos, listasPrecio, solicitudesCompra, cupones, promociones] = await Promise.all([
        api.bootstrap(),
        api.productos(),
        api.existencias(activeSedeId),
        api.transferencias(),
        // Almacenes de TODAS las sedes de la empresa (para el filtro por almacén de
        // Inventario y el selector de transferencias entre sedes). Cada pantalla
        // filtra por su sede. Gate admin/contadora en el backend: para vendedor/cajero
        // devuelve 403 → [] y las pantallas caen al total por sede.
        api.almacenes().catch(() => []),
        // .catch: un rol acotado (p. ej. Mesonero) no tiene acceso a clientes/
        // tesorería/documentos; degradan a vacío en vez de tumbar el bootstrap.
        api.clientes().catch(() => []),
        api.cuentasCobro().catch(() => []),
        api.documentos().catch(() => []),
        // Métodos de pago configurados. Puede venir vacío (backend en construcción
        // o empresa sin configurar): el POS cae a su lista por defecto en ese caso.
        api.metodosPago().catch(() => []),
        // Ventas (forma libre): cotizaciones/pedidos/facturas. El backend puede
        // estar en construcción, así que se tolera que falle sin romper la carga.
        api.cotizaciones().catch(() => []),
        // Compras: proveedores y órdenes de compra. Se toleran fallos para no
        // romper la carga si el vertical aún no está disponible en el backend.
        api.proveedores().catch(() => []),
        api.ordenesCompra().catch(() => []),
        // Facturas de compra registradas. Gate Dueña/Desarrollador/Contadora en el
        // backend: para vendedor/cajero devuelve vacío y se tolera sin romper.
        api.facturasCompra().catch(() => []),
        // Tasas de TODAS las divisas activas (multimoneda). Puede no cargar
        // (backend viejo o sin configurar): se tolera y se cae al modo USD-único.
        api.tasas().catch(() => null),
        // Dispositivos fiscales configurados. Gate Dueña/Desarrollador/Contadora en
        // el backend: para vendedor/cajero devuelve vacío y se tolera sin romper.
        api.dispositivos().catch(() => []),
        // Listas de precio (venta y compra) de la empresa. Vertical nuevo: se
        // tolera que falle sin romper la carga (backend en construcción o rol sin
        // acceso). El selector de la cotización y las pantallas de gestión leen esto.
        api.listasPrecio().catch(() => []),
        // Solicitudes de presupuesto (RFQ) de Compras. Vertical nuevo: se tolera que
        // falle sin romper la carga (backend en construcción o rol sin acceso).
        api.solicitudesCompra().catch(() => []),
        // Cupones de descuento (Ventas). Vertical nuevo: se tolera que falle sin
        // romper la carga (backend en construcción o rol sin acceso). La pantalla de
        // gestión y el campo «Cupón» de la cotización leen esto.
        api.cupones().catch(() => []),
        // Promociones (Ventas). Vertical nuevo: se tolera que falle sin romper la
        // carga (backend en construcción o rol sin acceso). La pantalla de gestión
        // y el carrusel de la pantalla del cliente leen esto.
        api.promociones().catch(() => []),
      ])
      setDb({
        EMPRESA: boot.empresa || null,
        // Sesión de DEMOSTRACIÓN (DEV_LOGIN): el chrome muestra la franja de
        // «sesión limitada» mientras esto sea true.
        DEMO: !!boot.demo,
        SEDES: boot.sedes || [],
        SEDE_ACTIVA: activeSede || (boot.sedes || []).find((s) => s.id === activeSedeId) || null,
        ROLES: boot.roles || {},
        RUBROS: boot.rubros || [],
        PRODUCTOS: productos || [],
        EXISTENCIAS: existencias || [],
        TRANSFERENCIAS: transferencias || [],
        // Almacenes de la sede activa (Fase 3: filtro por almacén y transferencias).
        ALMACENES: almacenes || [],
        CLIENTES: clientes || [],
        CUENTAS_COBRO: cuentasCobro || [],
        DOCUMENTOS: documentos || [],
        METODOS_PAGO: metodosPago || [],
        DISPOSITIVOS: dispositivos || [],
        // Unidades de medida ACTIVAS del maestro (Configuración › Unidades), tal
        // como las trae el bootstrap. Alimentan el select de «Unidad base» del
        // editor de producto sin una llamada extra. La pantalla de gestión pide el
        // maestro completo (incluidas las inactivas) aparte con api.unidades().
        UNIDADES: boot.unidades || [],
        // Módulos ACTIVOS de la empresa (Aplicaciones): core siempre + comercializables
        // instalados/activos. La UI oculta lo que no esté en esta lista.
        MODULOS: boot.modulos || [],
        // Formatos de documento (Configuración › Formatos). Todos los del tenant,
        // para resolver del lado del cliente qué plantilla imprime cada tipo según
        // la sede activa (ver lib/plantillaImprimir › resolverFormato).
        FORMATOS: boot.formatos || [],
        // Módulo Restaurante: mesas del salón de la sede activa e impresora de
        // comandas. Vacío/null cuando el módulo no está activo.
        MESAS: boot.mesas || [],
        PLANO_SALON: boot.planoSalon || null,
        // Comanderas de la sede (cocina, barra, postres) — varias por sede.
        IMPRESORAS_COMANDAS: boot.impresorasComandas || [],
        // Asignación de mesas por mesonero y config del módulo: la comandera destaca las
        // mesas propias y avisa antes de tomar una ajena.
        ASIGNACIONES_MESAS: boot.asignacionesMesas || [],
        CONFIG_SALON: boot.configSalon || null,
        CUENTAS_ABIERTAS: boot.cuentasAbiertas || [],
        COTIZACIONES: cotizaciones || [],
        LISTAS_PRECIO: listasPrecio || [],
        CUPONES: cupones || [],
        // PROMOCIONES: la biblioteca completa (para la pantalla de gestión); viene
        // del endpoint gestionado (dueño/desarrollador/vendedor). PROMOCIONES_ACTIVAS:
        // solo las vigentes, del bootstrap, disponibles para cualquier rol — lo que
        // el carrusel de la pantalla del cliente consume aunque lo opere un cajero.
        PROMOCIONES: promociones || [],
        PROMOCIONES_ACTIVAS: boot.promocionesActivas || [],
        PROVEEDORES: proveedores || [],
        ORDENES_COMPRA: ordenesCompra || [],
        FACTURAS_COMPRA: facturasCompra || [],
        SOLICITUDES_COMPRA: solicitudesCompra || [],
        // Tasa de cambio vigente, tal como la resolvió el servidor: valor,
        // origen y fecha. Puede venir `hay: false` (ninguna tasa cargada) y en
        // ese caso la interfaz muestra el estado vacío, nunca un número.
        TASA: boot.tasa || { hay: false },
        // Multimoneda: { tasas:[{moneda, valor, fuente, fuenteLabel, esDeHoy…}],
        // activas:[{codigo, fuente}] }. `null` cuando no cargó: la app cae al
        // comportamiento USD-único de siempre (retrocompat).
        TASAS: tasas || null,
      })
      cargadoAlguna.current = true
      // Queda registrado a qué empresa pertenece el db en memoria.
      empresaDelDb.current = activeEmpresaId
    } catch (e) {
      // Un refresco que falla no borra los datos que ya están en pantalla: se
      // sigue operando con lo último bueno, igual que con la tasa.
      if (!cargadoAlguna.current) setError(e)
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [activeEmpresaId, activeSedeId, activeSede])

  useEffect(() => { load() }, [load])

  // recargarTasa refresca solo las tasas, sin volver a traer todo el tenant: lo
  // usan el modal de la tasa, Configuración y el POS tras una carga manual o una
  // sincronización. Refresca el dólar (db.TASA) y todas las divisas (db.TASAS).
  const recargarTasa = useCallback(async () => {
    try {
      const [t, ts] = await Promise.all([api.tasa(), api.tasas().catch(() => null)])
      setDb((d) => ({ ...d, TASA: t || { hay: false }, TASAS: ts ?? d.TASAS ?? null }))
      return t
    } catch {
      return null
    }
  }, [])

  // tasaDe resuelve la tasa (Bs por 1 unidad) de cualquier moneda. VES ⇒ 1;
  // USD y las demás ⇒ su `valor` en db.TASAS, con el dólar cayendo a db.TASA
  // como respaldo (retrocompat con el bootstrap). 0 cuando no hay tasa.
  const tasaDe = useCallback((moneda) => {
    const m = (moneda || 'VES').toUpperCase()
    if (m === 'VES') return 1
    const fila = (db.TASAS?.tasas || []).find((x) => (x.moneda || '').toUpperCase() === m)
    if (fila && Number(fila.valor) > 0) return Number(fila.valor)
    if (m === 'USD' && db.TASA?.hay && Number(db.TASA.valor) > 0) return Number(db.TASA.valor)
    return 0
  }, [db.TASAS, db.TASA])

  return (
    <DataCtx.Provider value={{ db, loading, refreshing, error, reload: load, recargarTasa, tasaDe }}>
      {children}
    </DataCtx.Provider>
  )
}

/* useTasa expone la tasa vigente de una moneda y sus helpers de conversión
 * (R9 + R10, multimoneda).
 *
 * `useTasa()` sin argumento sigue devolviendo el DÓLAR, con el objeto rico de
 * db.TASA (fuente, cuarentena, obtenidaEn…) — idéntico a lo de siempre, para no
 * tocar el POS, el cobro ni el resto de pantallas que ya lo usaban.
 * `useTasa('EUR')` (u otra divisa) devuelve la tasa de esa divisa desde
 * db.TASAS; `useTasa('VES')` devuelve la identidad (1).
 *
 * Regla: si no hay tasa, `hay` es false y NO se convierte nada — la interfaz
 * dice qué falta en lugar de mostrar un importe calculado con un 1 inventado.
 */
export function useTasa(moneda = 'USD') {
  const { db, recargarTasa } = useData()
  const m = (moneda || 'USD').toUpperCase()

  if (m === 'VES') {
    return {
      hay: true, valor: 1, moneda: 'VES', esDeHoy: true, oficial: true, fuenteLabel: 'Bs',
      aBs: (x) => Number(x) || 0, aUsd: (x) => Number(x) || 0, aMoneda: (x) => Number(x) || 0,
      recargar: recargarTasa,
    }
  }

  // El dólar conserva el objeto rico del bootstrap; las demás salen de db.TASAS.
  const base = m === 'USD'
    ? (db.TASA || { hay: false })
    : (() => {
        const fila = (db.TASAS?.tasas || []).find((x) => (x.moneda || '').toUpperCase() === m)
        return fila ? { ...fila, hay: Number(fila.valor) > 0 } : { hay: false, moneda: m }
      })()

  const valor = base.hay ? Number(base.valor) || 0 : 0
  return {
    ...base,
    moneda: m,
    valor,
    // aBs / aMoneda (y el alias histórico aUsd) convierten solo si hay tasa.
    aBs: (x) => (valor > 0 ? (Number(x) || 0) * valor : null),
    aUsd: (bs) => (valor > 0 ? (Number(bs) || 0) / valor : null),
    aMoneda: (bs) => (valor > 0 ? (Number(bs) || 0) / valor : null),
    recargar: recargarTasa,
  }
}

// useTasaDe expone el resolutor de tasas (Bs por 1 unidad de cualquier moneda),
// pensado para pasárselo a los helpers de lib/precio.js (que aceptan tanto un
// número —el dólar, retrocompat— como este resolutor).
export function useTasaDe() {
  return useData().tasaDe
}
