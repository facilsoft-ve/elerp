// Cliente HTTP del backend de ElERP.
// En dev, API_BASE queda vacío y el proxy de Vite reenvía /api y /auth al Go.
// En prod (front en Vercel), define VITE_API_BASE con la URL del backend.
const API_BASE = import.meta.env.VITE_API_BASE || ''

// Contexto activo (multi-tenant de 3 niveles: Organización → Empresa → Sede).
// El tenant es la Empresa; casi todo dato lleva también Sede. Se envían como
// headers X-Empresa-ID y X-Sede-ID en cada request de datos. Los setea
// AuthContext al elegir/cambiar de empresa o sede.
let activeEmpresaId = ''
let activeSedeId = ''
export function setActiveEmpresa(id) { activeEmpresaId = id || '' }
export function setActiveSede(id) { activeSedeId = id || '' }
// getActiveSede expone la sede activa actual (solo lectura) para resolver, del
// lado del cliente, qué formato de documento imprime esta sede.
export function getActiveSede() { return activeSedeId }

async function request(path, opts = {}) {
  // Con FormData NO se fija Content-Type: el navegador tiene que poner el suyo
  // con el boundary del multipart, o el backend no puede parsear el archivo.
  const esFormData = typeof FormData !== 'undefined' && opts.body instanceof FormData
  const headers = { ...(esFormData ? {} : { 'Content-Type': 'application/json' }), ...(opts.headers || {}) }
  if (activeEmpresaId) headers['X-Empresa-ID'] = activeEmpresaId
  if (activeSedeId) headers['X-Sede-ID'] = activeSedeId
  // `headers` va DESPUÉS de ...rest: si el llamador pasa `headers` en opts (p. ej.
  // subir imagen con `headers: {}` para no fijar Content-Type), ya está fusionado
  // arriba; volver a esparcir opts.headers acá borraría el X-Empresa-ID calculado.
  const { headers: _optsHeaders, ...rest } = opts
  const res = await fetch(API_BASE + path, {
    credentials: 'include',
    ...rest,
    headers,
  })
  if (res.status === 401) {
    const err = new Error('unauthorized')
    err.status = 401
    throw err
  }
  if (!res.ok) {
    let body
    try { body = await res.json() } catch { body = null }
    const err = new Error((body && body.error) || `HTTP ${res.status}`)
    err.status = res.status
    // `codigo` es el identificador estable del fallo cuando el servidor lo
    // manda. La pantalla necesita distinguir casos que comparten código HTTP
    // (p. ej. «sin ubicación» vs «fuera de la sede», ambos 403) y comparar los
    // textos —que son para leer y se pueden reescribir— sería frágil.
    if (body && body.codigo) err.codigo = body.codigo
    throw err
  }
  if (res.status === 204) return null
  return res.json()
}

export const api = {
  // URLs de arranque de login (navegación top-level, no fetch).
  loginURL: () => (API_BASE || '') + '/api/auth/login',
  devLoginURL: () => (API_BASE || '') + '/api/auth/dev-login',

  health: () => request('/api/health'),
  me: () => request('/api/me'),
  logout: () => request('/api/auth/logout', { method: 'POST' }),

  // Captación del prospecto antes de entrar a la demo (público, pre-login). El
  // lead se guarda de verdad en el backend; devuelve { ok, id }. No dispara
  // ningún correo (la notificación al equipo es un punto de integración aparte).

  // Auth alterno (email/contraseña) + aceptación de invitación por token.
  nativeLogin: (body) => request('/api/auth/native-login', { method: 'POST', body: JSON.stringify(body) }),
  invitePreview: (token) => request(`/api/auth/invite/${encodeURIComponent(token)}`),
  acceptInvite: (body) => request('/api/auth/accept-invite', { method: 'POST', body: JSON.stringify(body) }),

  // Onboarding de empresa: crear empresa → configurar (giro + modalidad) → 1a sede.
  createEmpresa: (body) => request('/api/empresas', { method: 'POST', body: JSON.stringify(body) }),
  onboardEmpresa: (id, body) => request(`/api/empresas/${encodeURIComponent(id)}/onboarding`, { method: 'POST', body: JSON.stringify(body) }),
  createSede: (id, body) => request(`/api/empresas/${encodeURIComponent(id)}/sedes`, { method: 'POST', body: JSON.stringify(body) }),

  // Configuración › Datos de empresa y Sedes (edición fuera del onboarding).
  actualizarEmpresa: (id, body) => request(`/api/empresas/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  crearSede: (empId, body) => request(`/api/empresas/${encodeURIComponent(empId)}/sedes`, { method: 'POST', body: JSON.stringify(body) }),
  actualizarSede: (empId, sedeId, body) => request(`/api/empresas/${encodeURIComponent(empId)}/sedes/${encodeURIComponent(sedeId)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  desactivarSede: (empId, sedeId) => request(`/api/empresas/${encodeURIComponent(empId)}/sedes/${encodeURIComponent(sedeId)}/desactivar`, { method: 'POST' }),
  reactivarSede: (empId, sedeId) => request(`/api/empresas/${encodeURIComponent(empId)}/sedes/${encodeURIComponent(sedeId)}/reactivar`, { method: 'POST' }),
  // Configuración › Impuestos y alícuotas. body = { alicuotaIVA, alicuotaIGTF } en fracciones (0.16 = 16%).
  actualizarImpuestos: (id, body) => request(`/api/empresas/${encodeURIComponent(id)}/impuestos`, { method: 'PATCH', body: JSON.stringify(body) }),
  // Control en tres vías (pedido · recepción · factura): política y tolerancia.
  actualizarControlCompras: (id, body) => request(`/api/empresas/${encodeURIComponent(id)}/control-compras`, { method: 'PATCH', body: JSON.stringify(body) }),

  // Datos de arranque de la empresa/sede activa.
  bootstrap: () => request('/api/bootstrap'),

  // ---- Inventario y Operaciones (primer vertical real) ----
  productos: () => request('/api/inventario/productos'),
  createProducto: (body) => request('/api/inventario/productos', { method: 'POST', body: JSON.stringify(body) }),
  // Carga masiva de productos. confirmar=false ⇒ vista previa (valida y clasifica
  // sin escribir); confirmar=true ⇒ aplica (UPSERT + existencia inicial).
  importarProductos: (body) => request('/api/inventario/productos/importar', { method: 'POST', body: JSON.stringify(body) }),
  createPresentacion: (id, body) => request(`/api/inventario/productos/${encodeURIComponent(id)}/presentaciones`, { method: 'POST', body: JSON.stringify(body) }),
  // existencias/kardex aceptan un almacén opcional: con él, la vista es de ESE
  // almacén; sin él, de la sede (suma de todos sus almacenes) como siempre.
  existencias: (sedeId, almacenId) => {
    const p = new URLSearchParams()
    if (sedeId) p.set('sede', sedeId)
    if (almacenId) p.set('almacen', almacenId)
    const qs = p.toString()
    return request('/api/inventario/existencias' + (qs ? '?' + qs : ''))
  },
  ajustarExistencia: (sku, body, almacenId) => {
    const qs = almacenId ? '?almacen=' + encodeURIComponent(almacenId) : ''
    return request(`/api/inventario/existencias/${encodeURIComponent(sku)}/ajustar` + qs, { method: 'POST', body: JSON.stringify(body) })
  },
  kardex: (sku, sedeId, almacenId) => {
    const p = new URLSearchParams()
    if (sedeId) p.set('sede', sedeId)
    if (almacenId) p.set('almacen', almacenId)
    const qs = p.toString()
    return request(`/api/inventario/kardex/${encodeURIComponent(sku)}` + (qs ? '?' + qs : ''))
  },
  // Registro de entradas y salidas: movimientos del ledger filtrables por sede,
  // almacén, sku, tipo y rango de fechas.
  movimientos: (f = {}) => {
    const p = new URLSearchParams()
    for (const k of ['sede', 'almacen', 'sku', 'tipo', 'desde', 'hasta']) if (f[k]) p.set(k, f[k])
    const qs = p.toString()
    return request('/api/inventario/movimientos' + (qs ? '?' + qs : ''))
  },
  transferencias: () => request('/api/inventario/transferencias'),
  createTransferencia: (body) => request('/api/inventario/transferencias', { method: 'POST', body: JSON.stringify(body) }),
  setTransferenciaEstado: (id, estado) => request(`/api/inventario/transferencias/${encodeURIComponent(id)}/estado`, { method: 'PATCH', body: JSON.stringify({ estado }) }),
  cancelarTransferencia: (id, body) => request(`/api/inventario/transferencias/${encodeURIComponent(id)}/cancelar`, { method: 'POST', body: JSON.stringify(body) }),

  // ---- Fiscal y Facturación (POS + documentos fiscales) ----
  // Documentos append-only: se emiten y se anulan (reversa), nunca se editan.
  documentos: () => request('/api/fiscal/documentos'),
  documento: (id) => request(`/api/fiscal/documentos/${encodeURIComponent(id)}`),
  emitirDocumento: (body) => request('/api/fiscal/documentos', { method: 'POST', body: JSON.stringify(body) }),
  anularDocumento: (id, motivo) => request(`/api/fiscal/documentos/${encodeURIComponent(id)}/anular`, { method: 'POST', body: JSON.stringify({ motivo }) }),
  // Nota de crédito PARCIAL: acredita solo las cantidades indicadas por línea.
  notaCredito: (id, body) => request(`/api/fiscal/documentos/${encodeURIComponent(id)}/nota-credito`, { method: 'POST', body: JSON.stringify(body) }),
  // Nota de crédito por DESCUENTO: baja el monto SIN devolver mercancía (por eso
  // no mueve inventario). body = { monto } o { porcentaje }, más `sku` para
  // acotarlo a un renglón, `exento` y `nota`.
  notaCreditoDescuento: (id, body) => request(`/api/fiscal/documentos/${encodeURIComponent(id)}/nota-credito/descuento`, { method: 'POST', body: JSON.stringify(body) }),
  // Nota de crédito por AJUSTE DE PRECIO: se facturó más caro de lo correcto y se
  // acredita la diferencia. body = { nota, ajustes: [{ sku, precioCorrecto }] }.
  notaCreditoAjustePrecio: (id, body) => request(`/api/fiscal/documentos/${encodeURIComponent(id)}/nota-credito/ajuste-precio`, { method: 'POST', body: JSON.stringify(body) }),
  // Catálogo de motivos de nota ({ credito, debito }). Del servidor y no escrito a
  // mano acá: el motivo decide si la nota mueve inventario, y una lista propia se
  // desincroniza del que valida.
  motivosNota: () => request('/api/fiscal/motivos-nota'),
  // Nota de débito: cargo adicional que AUMENTA el monto de una factura. Espejo
  // positivo de la nota de crédito. body = { concepto, monto | porcentaje, sku?,
  // exento?, motivoCodigo? }.
  emitirNotaDebito: (id, body) => request(`/api/fiscal/documentos/${encodeURIComponent(id)}/nota-debito`, { method: 'POST', body: JSON.stringify(body) }),

  // Cierres Z (reporte fiscal diario por sede). Append-only: se emiten y se
  // corrigen con un Z posterior, nunca se editan. La lista y el preview van por
  // la sede activa (headers X-Sede-ID). El preview calcula lo que se cerraría
  // AHORA sin persistir; emitir consolida y devuelve el Z (o 400 si no hay nada).
  cierresZ: () => request('/api/fiscal/cierres-z'),
  previewCierreZ: () => request('/api/fiscal/cierres-z/preview'),
  emitirCierreZ: () => request('/api/fiscal/cierres-z', { method: 'POST' }),

  // Retenciones de IVA (comprobantes). La lista consolida recibidas (sobre facturas
  // de venta, crédito a favor) y emitidas (sobre facturas de compra, si eres agente
  // de retención). Registrar es append-only: el backend rechaza con 400 duplicadas,
  // % inválido, sin IVA o documento anulado.
  retenciones: () => request('/api/fiscal/retenciones'),
  registrarRetencionRecibida: (docId, body) => request(`/api/fiscal/documentos/${encodeURIComponent(docId)}/retencion-recibida`, { method: 'POST', body: JSON.stringify(body) }),
  registrarRetencionEmitida: (facturaCompraId, body) => request(`/api/compras/facturas/${encodeURIComponent(facturaCompraId)}/retencion-emitida`, { method: 'POST', body: JSON.stringify(body) }),

  // Documentos RELACIONADOS de un documento fiscal: su origen (la factura que
  // corrige, si es una nota), lo que salió de él (notas y anulación) y sus
  // comprobantes de retención. Solo lectura de vínculos que ya existen en el dato
  // (`refDocumentoId`); permite recorrer la traza en los dos sentidos.
  documentosRelacionados: (docId) => request(`/api/fiscal/documentos/${encodeURIComponent(docId)}/relacionados`),

  // Libros fiscales (Libro de Ventas / Libro de Compras): reportes DERIVADOS del
  // ledger, de solo lectura, por contribuyente (empresa) y período mensual —
  // consolidan TODAS las sedes. anio/mes default al mes actual en el servidor.
  libroVentas: (anio, mes) => request(`/api/fiscal/libros/ventas?anio=${anio}&mes=${mes}`),
  libroCompras: (anio, mes) => request(`/api/fiscal/libros/compras?anio=${anio}&mes=${mes}`),
  // Libro de Inventario: existencia valorizada a la fecha (proyección del ledger),
  // consolidando TODAS las sedes. Solo lectura.
  libroInventario: () => request('/api/fiscal/libro-inventario'),

  // ---- Contabilidad ----
  // Todo es de lectura: los asientos se DERIVAN de las operaciones. La única
  // escritura es revertir, que anexa el asiento contrario.
  planDeCuentas: () => request('/api/contabilidad/plan'),
  crearCuenta: (body) => request('/api/contabilidad/plan', { method: 'POST', body: JSON.stringify(body) }),
  renombrarCuenta: (codigo, nombre) => request(`/api/contabilidad/plan/${encodeURIComponent(codigo)}`, { method: 'PATCH', body: JSON.stringify({ nombre }) }),
  activarCuenta: (codigo) => request(`/api/contabilidad/plan/${encodeURIComponent(codigo)}/activar`, { method: 'POST' }),
  desactivarCuenta: (codigo) => request(`/api/contabilidad/plan/${encodeURIComponent(codigo)}/desactivar`, { method: 'POST' }),
  // Registro de actividad (bitácora de auditoría del tenant). Filtros: rango de
  // fechas (desde/hasta), prefijo de acción y búsqueda libre (q).
  auditoria: ({ desde, hasta, accion, q, limite } = {}) => {
    const p = new URLSearchParams()
    if (desde) p.set('desde', desde)
    if (hasta) p.set('hasta', hasta)
    if (accion) p.set('accion', accion)
    if (q) p.set('q', q)
    if (limite) p.set('limite', limite)
    const qs = p.toString()
    return request('/api/auditoria' + (qs ? '?' + qs : ''))
  },
  libroDiario: () => request('/api/contabilidad/diario'),
  crearAsientoManual: (body) => request('/api/contabilidad/diario', { method: 'POST', body: JSON.stringify(body) }),
  // Sello de integridad: recalcula el hash-encadenado de asientos y documentos.
  verificarIntegridad: () => request('/api/contabilidad/integridad'),
  balanceContable: () => request('/api/contabilidad/balance'),
  // Estado de resultados (P&G). Con desde/hasta (YYYY-MM-DD) es del período; sin
  // ellos, acumulado desde el inicio.
  resultadosContables: (desde, hasta) => {
    const p = new URLSearchParams()
    if (desde) p.set('desde', desde)
    if (hasta) p.set('hasta', hasta)
    const qs = p.toString()
    return request('/api/contabilidad/resultados' + (qs ? '?' + qs : ''))
  },
  revertirAsiento: (id, motivo) => request(`/api/contabilidad/diario/${encodeURIComponent(id)}/revertir`, { method: 'POST', body: JSON.stringify({ motivo }) }),
  // Cierre de período (§7.3): definitivo, no reabrible.
  periodosCerrados: () => request('/api/contabilidad/periodos'),
  cerrarPeriodo: (body) => request('/api/contabilidad/periodos/cerrar', { method: 'POST', body: JSON.stringify(body) }),

  // ---- Tesorería ----
  // El saldo por cobrar es una PROYECCIÓN del servidor (total − cobros), no un
  // campo: la interfaz nunca lo recalcula a su manera.
  porCobrar: () => request('/api/tesoreria/por-cobrar'),
  porPagar: () => request('/api/tesoreria/por-pagar'),
  saldosTesoreria: () => request('/api/tesoreria/saldos'),
  reporteIGTF: () => request('/api/tesoreria/igtf'),
  cobros: () => request('/api/tesoreria/cobros'),
  registrarCobro: (body) => request('/api/tesoreria/cobros', { method: 'POST', body: JSON.stringify(body) }),
  reversarCobro: (id, motivo) => request(`/api/tesoreria/cobros/${encodeURIComponent(id)}/reversar`, { method: 'POST', body: JSON.stringify({ motivo }) }),
  // Pago a proveedores: histórico append-only (los reversos son registros con
  // reverso:true). El saldo por pagar es una PROYECCIÓN del servidor (recibido −
  // pagado); el backend rechaza con 400 si el pago excede el saldo pendiente.
  pagosProveedor: () => request('/api/tesoreria/pagos-proveedor'),
  registrarPagoProveedor: (body) => request('/api/tesoreria/pagos-proveedor', { method: 'POST', body: JSON.stringify(body) }),
  reversarPagoProveedor: (id, motivo) => request(`/api/tesoreria/pagos-proveedor/${encodeURIComponent(id)}/reversar`, { method: 'POST', body: JSON.stringify({ motivo }) }),

  // ---- Ventas en espera (2.6) ----
  // El carrito apartado vive en el SERVIDOR: sobrevive a recargar la página y lo
  // puede retomar otro cajero del turno siguiente, en la misma sede.
  ventasEnEspera: () => request('/api/pos/ventas-en-espera'),
  dejarEnEspera: (body) => request('/api/pos/ventas-en-espera', { method: 'POST', body: JSON.stringify(body) }),
  retomarVenta: (id) => request(`/api/pos/ventas-en-espera/${encodeURIComponent(id)}/retomar`, { method: 'POST' }),
  descartarVenta: (id) => request(`/api/pos/ventas-en-espera/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // ---- Tasa de cambio (R9) ----
  // La tasa NO se teclea: entra sola desde la fuente oficial. El frontend solo
  // la lee; escribirla es el último recurso (carga manual auditada).
  tasa: () => request('/api/tasa'),
  // Todas las divisas activas con su tasa y las activas configuradas
  // ({ tasas:[…], activas:[{codigo, fuente}] }). El dólar sigue en `tasa()`.
  tasas: () => request('/api/tasas'),
  historialTasa: (limite = 30) => request('/api/tasa/historial?limite=' + limite),
  cargarTasaManual: (valor, fechaValor) => request('/api/tasa/manual', { method: 'POST', body: JSON.stringify({ valor, fechaValor }) }),
  // Carga manual de la tasa de UNA divisa activa (default USD). body = { moneda?, valor, fechaValor? }.
  cargarTasaMoneda: (body) => request('/api/tasa/manual', { method: 'POST', body: JSON.stringify(body) }),
  sincronizarTasa: () => request('/api/tasa/sincronizar', { method: 'POST' }),
  aprobarTasaCuarentena: (id) => request(`/api/tasa/cuarentena/${encodeURIComponent(id)}/aprobar`, { method: 'POST' }),
  guardarConfigMoneda: (body) => request('/api/empresa/config/moneda', { method: 'PUT', body: JSON.stringify(body) }),

  // Editar producto (incluye el código de barras propio, R11) y escaneo.
  actualizarProducto: (sku, body) => request(`/api/inventario/productos/${encodeURIComponent(sku)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  buscarPorCodigo: (codigo) => request('/api/inventario/buscar?codigo=' + encodeURIComponent(codigo)),

  // ---- Configuración (empresa, usuarios, seguridad de caja) ----
  guardarConfigSeguridad: (requiereSupervisorPin) => request('/api/empresa/config/seguridad', { method: 'PUT', body: JSON.stringify({ requiereSupervisorPin }) }),
  usuarios: () => request('/api/usuarios'),
  guardarUsuario: (id, body) => request(`/api/usuarios/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  // Invitación de miembros. Crear devuelve { id, email, rol, token, enlace, correoEnviado }.
  // El correo automático aún no está conectado (correoEnviado:false): se comparte el `enlace`.
  invitarMiembro: (empresaId, body) => request(`/api/empresas/${encodeURIComponent(empresaId)}/invitaciones`, { method: 'POST', body: JSON.stringify(body) }),
  reenviarInvitacion: (empresaId, miembroId) => request(`/api/empresas/${encodeURIComponent(empresaId)}/invitaciones/${encodeURIComponent(miembroId)}/reenviar`, { method: 'POST' }),
  cancelarInvitacion: (empresaId, miembroId) => request(`/api/empresas/${encodeURIComponent(empresaId)}/invitaciones/${encodeURIComponent(miembroId)}/cancelar`, { method: 'POST' }),
  sesionesCaja: () => request('/api/cajas/sesiones'),
  // Autorización de supervisor para las acciones sensibles del modo caja.
  autorizarSupervisor: (accion, pin) => request('/api/cajas/autorizar', { method: 'POST', body: JSON.stringify({ accion, pin }) }),

  // ---- Cajas y turnos ----
  // Sin una caja abierta el backend rechaza la emisión: el POS consulta
  // miSesionCaja antes de renderizarse.
  cajas: (sedeId) => request('/api/cajas' + (sedeId ? '?sede=' + encodeURIComponent(sedeId) : '')),
  crearCaja: (body) => request('/api/cajas', { method: 'POST', body: JSON.stringify(body) }),
  actualizarCaja: (id, body) => request(`/api/cajas/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  estadoCaja: (id, estado) => request(`/api/cajas/${encodeURIComponent(id)}/estado`, { method: 'PATCH', body: JSON.stringify({ estado }) }),
  // La apertura acepta un fondo inicial (efectivo Bs de la gaveta) en el body.
  abrirCaja: (id, body) => request(`/api/cajas/${encodeURIComponent(id)}/abrir`, { method: 'POST', body: JSON.stringify(body) }),
  // Cerrar el turno con el ARQUEO. `opts` puede ser un booleano (forzado, uso
  // administrativo) o el conteo declarado { forzado?, efectivoContadoBs?,
  // contadoPorMetodo? }. El backend congela lo esperado y calcula la diferencia.
  cerrarCaja: (id, opts = false) => {
    const body = typeof opts === 'boolean' ? { forzado: opts } : (opts || {})
    return request(`/api/cajas/${encodeURIComponent(id)}/cerrar`, { method: 'POST', body: JSON.stringify(body) })
  },
  // Preview del arqueo del turno: lo esperado por método, derivado del servidor.
  arqueoSesion: (id) => request(`/api/cajas/sesiones/${encodeURIComponent(id)}/arqueo`),
  miSesionCaja: () => request('/api/cajas/mi-sesion'),
  cajeros: () => request('/api/cajeros'),
  crearCajero: (body) => request('/api/cajeros', { method: 'POST', body: JSON.stringify(body) }),
  actualizarCajero: (id, body) => request(`/api/cajeros/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),

  // Disponibilidad de un SKU en todas las sedes de la empresa (botón ⓘ del POS).
  disponibilidad: (sku) => request(`/api/inventario/productos/${encodeURIComponent(sku)}/disponibilidad`),

  // Imagen de producto: multipart, sin Content-Type manual (lo pone el browser
  // con su boundary).
  subirImagenProducto: (sku, file) => {
    const fd = new FormData()
    fd.append('archivo', file)
    return request(`/api/inventario/productos/${encodeURIComponent(sku)}/imagen`, {
      method: 'POST', body: fd, headers: {},
    })
  },
  quitarImagenProducto: (sku) => request(`/api/inventario/productos/${encodeURIComponent(sku)}/imagen`, { method: 'DELETE' }),

  // ---- Clientes ----
  // El maestro de clientes es EDITABLE (no es un ledger append-only). El modelo
  // guarda además la procedencia (origen/sistema externo/id externo) para poder
  // importar/sincronizar terceros desde Odoo u otro CRM sin perder trazabilidad.
  clientes: () => request('/api/crm/clientes'),
  crearCliente: (body) => request('/api/crm/clientes', { method: 'POST', body: JSON.stringify(body) }),
  actualizarCliente: (id, body) => request(`/api/crm/clientes/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),

  // ---- Ventas (forma libre): cotización → pedido/prefactura → factura ----
  // La cotización es una propuesta editable en borrador. Al confirmar se vuelve
  // pedido/prefactura (ya no se edita). Al facturar se emite el documento fiscal
  // (append-only) registrando el pago: la respuesta trae { cotizacion, documento }.
  cotizaciones: () => request('/api/ventas/cotizaciones'),
  cotizacion: (id) => request(`/api/ventas/cotizaciones/${encodeURIComponent(id)}`),
  crearCotizacion: (body) => request('/api/ventas/cotizaciones', { method: 'POST', body: JSON.stringify(body) }),
  actualizarCotizacion: (id, body) => request(`/api/ventas/cotizaciones/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  confirmarCotizacion: (id) => request(`/api/ventas/cotizaciones/${encodeURIComponent(id)}/confirmar`, { method: 'POST' }),
  facturarCotizacion: (id, body) => request(`/api/ventas/cotizaciones/${encodeURIComponent(id)}/facturar`, { method: 'POST', body: JSON.stringify(body) }),
  cancelarCotizacion: (id, motivo) => request(`/api/ventas/cotizaciones/${encodeURIComponent(id)}/cancelar`, { method: 'POST', body: JSON.stringify({ motivo }) }),

  // ---- Listas de precio (Ventas y Compras) ----
  // Maestro EDITABLE (no ledger): tarifas con un precio explícito por SKU que
  // reemplaza al precio base del catálogo. `tipo` filtra venta|compra. Fija/edita
  // los ítems con Create/Update; los documentos ya emitidos conservan su precio.
  listasPrecio: (tipo) => request('/api/listas-precio' + (tipo ? '?tipo=' + encodeURIComponent(tipo) : '')),
  crearListaPrecio: (body) => request('/api/listas-precio', { method: 'POST', body: JSON.stringify(body) }),
  actualizarListaPrecio: (id, body) => request(`/api/listas-precio/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),

  // ---- Cupones de descuento (Ventas) ----
  // Maestro EDITABLE (no ledger): un código con descuento (porcentual o de monto
  // fijo) que se aplica al cobrar/cotizar. `validarCupon` resuelve un código
  // contra un subtotal (Bs) y devuelve { descuentoBs, cupon } o un error claro;
  // quien cotiza/cobra baja el precioUnitario de las líneas con ese descuento
  // (el motor fiscal del servidor calcula el IVA sobre la base ya descontada).
  cupones: () => request('/api/ventas/cupones'),
  crearCupon: (body) => request('/api/ventas/cupones', { method: 'POST', body: JSON.stringify(body) }),
  actualizarCupon: (id, body) => request(`/api/ventas/cupones/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  validarCupon: (body) => request('/api/ventas/cupones/validar', { method: 'POST', body: JSON.stringify(body) }),

  // ---- Promociones (Ventas) ----
  // Maestro EDITABLE (no ledger): una biblioteca de anuncios (imagen o texto, con
  // vigencia) que alimentan el carrusel de la pantalla del cliente cuando están
  // activas y dentro de su vigencia. Se combinan con los slides manuales de Ajustes.
  promociones: () => request('/api/ventas/promociones'),
  crearPromocion: (body) => request('/api/ventas/promociones', { method: 'POST', body: JSON.stringify(body) }),
  actualizarPromocion: (id, body) => request(`/api/ventas/promociones/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),

  // Subidor GENÉRICO de imágenes al bucket del tenant (multipart, sin
  // Content-Type manual: lo pone el browser con su boundary). `carpeta` agrupa por
  // recurso (promociones|publicidad|empresa). Devuelve { url } con la ruta relativa
  // servida por /api/archivos/… (la misma que usan las imágenes de producto).
  subirArchivo: (file, carpeta) => {
    const fd = new FormData()
    fd.append('archivo', file)
    if (carpeta) fd.append('carpeta', carpeta)
    return request('/api/archivos/subir', { method: 'POST', body: fd, headers: {} })
  },

  // ---- Compras y Proveedores ----
  // Proveedores: CRUD con baja reversible (desactivar). Las órdenes de compra
  // son append-only en su máquina de estados; recibir mercancía anexa entradas
  // al ledger de inventario (mismo patrón que las ventas descuentan stock).
  proveedores: () => request('/api/compras/proveedores'),
  crearProveedor: (body) => request('/api/compras/proveedores', { method: 'POST', body: JSON.stringify(body) }),
  actualizarProveedor: (id, body) => request(`/api/compras/proveedores/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  desactivarProveedor: (id) => request(`/api/compras/proveedores/${encodeURIComponent(id)}/desactivar`, { method: 'POST' }),
  // Solicitudes de presupuesto (RFQ): el paso previo a la orden de compra. Se pide
  // cotización a varios proveedores, se cargan sus respuestas, se comparan y se
  // convierte la elegida en orden (que reusa el flujo de OC). Documento de gestión
  // editable en borrador; enviar la registra. Convertir devuelve { solicitud, orden }.
  solicitudesCompra: () => request('/api/compras/solicitudes'),
  solicitudCompra: (id) => request(`/api/compras/solicitudes/${encodeURIComponent(id)}`),
  crearSolicitudCompra: (body) => request('/api/compras/solicitudes', { method: 'POST', body: JSON.stringify(body) }),
  actualizarSolicitudCompra: (id, body) => request(`/api/compras/solicitudes/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  enviarSolicitudCompra: (id) => request(`/api/compras/solicitudes/${encodeURIComponent(id)}/enviar`, { method: 'POST' }),
  registrarRespuestaSolicitud: (id, body) => request(`/api/compras/solicitudes/${encodeURIComponent(id)}/respuesta`, { method: 'POST', body: JSON.stringify(body) }),
  cerrarSolicitudCompra: (id) => request(`/api/compras/solicitudes/${encodeURIComponent(id)}/cerrar`, { method: 'POST' }),
  convertirSolicitudEnOrden: (id, proveedorId) => request(`/api/compras/solicitudes/${encodeURIComponent(id)}/convertir`, { method: 'POST', body: JSON.stringify({ proveedorId }) }),

  ordenesCompra: () => request('/api/compras/ordenes'),
  ordenCompra: (id) => request(`/api/compras/ordenes/${encodeURIComponent(id)}`),
  crearOrdenCompra: (body) => request('/api/compras/ordenes', { method: 'POST', body: JSON.stringify(body) }),
  confirmarOrdenCompra: (id) => request(`/api/compras/ordenes/${encodeURIComponent(id)}/confirmar`, { method: 'POST' }),
  recibirOrdenCompra: (id, body) => request(`/api/compras/ordenes/${encodeURIComponent(id)}/recibir`, { method: 'POST', body: JSON.stringify(body) }),
  cancelarOrdenCompra: (id, motivo) => request(`/api/compras/ordenes/${encodeURIComponent(id)}/cancelar`, { method: 'POST', body: JSON.stringify({ motivo }) }),
  // Factura de compra del proveedor: al registrarla se reconoce el IVA crédito
  // (el pasivo ya venía de recibir la mercancía). Una OC no se puede facturar dos
  // veces (el backend rechaza con 400). La lista consolida las facturas de la empresa.
  registrarFacturaCompra: (ordenId, body) => request(`/api/compras/ordenes/${encodeURIComponent(ordenId)}/factura`, { method: 'POST', body: JSON.stringify(body) }),
  facturasCompra: () => request('/api/compras/facturas'),
  notasCompra: () => request('/api/compras/notas'),
  emitirNotaCreditoCompra: (id, body) => request(`/api/compras/facturas/${encodeURIComponent(id)}/nota-credito`, { method: 'POST', body: JSON.stringify(body) }),
  emitirNotaDebitoCompra: (id, body) => request(`/api/compras/facturas/${encodeURIComponent(id)}/nota-debito`, { method: 'POST', body: JSON.stringify(body) }),

  // ---- Tesorería (cuentas de cobro para el POS) ----
  cuentasCobro: () => request('/api/tesoreria/cuentas-cobro'),
  crearCuentaCobro: (body) => request('/api/tesoreria/cuentas-cobro', { method: 'POST', body: JSON.stringify(body) }),

  // ---- Configuración de Métodos de pago (POS + Ventas) ----
  // Los métodos vienen ordenados por `orden`. El POS usa los que tengan
  // enCaja && activo; el módulo de Ventas usa enVentas && activo.
  metodosPago: () => request('/api/config/metodos-pago'),
  crearMetodoPago: (body) => request('/api/config/metodos-pago', { method: 'POST', body: JSON.stringify(body) }),
  actualizarMetodoPago: (id, body) => request(`/api/config/metodos-pago/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  eliminarMetodoPago: (id) => request(`/api/config/metodos-pago/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // ---- Configuración › Dispositivos fiscales ----
  // Config CRUD por empresa. El registro/administración vive aquí; la conexión
  // real con la impresora la hace el agente fiscal local (binario aparte). Sin
  // borrado duro: se desactiva (Activo=false), reversible con el toggle.
  dispositivos: () => request('/api/config/dispositivos'),
  // Catálogo precargado de marcas/modelos del mercado venezolano (impresoras
  // fiscales, balanzas y comanderas). Dato de referencia, igual para todos los
  // tenants: se pide una vez por carga (ver components/dispositivo.jsx).
  catalogoDispositivos: () => request('/api/config/dispositivos/catalogo'),

  // ---- Configuración › Maestro de impuestos ----
  // Las alícuotas de IVA con su VIGENCIA. Las tasas viajan en fracción (0.16),
  // igual que se guardan: convertir a porcentaje es cosa de la pantalla, no del
  // transporte — si no, el mismo número significa dos cosas según por dónde entre.
  alicuotas: () => request('/api/config/alicuotas'),

  // ---- Maestro de conceptos ISLR ----
  // La tabla de conceptos retenibles con su tarifa y su sustraendo. `sugerencia`
  // resuelve cuánto retener EN EL SERVIDOR: la pantalla muestra, no calcula —
  // dos implementaciones de la misma fórmula terminan discrepando.
  conceptosISLR: () => request('/api/config/conceptos-islr'),
  // `fecha` es la del hecho (vacía = hoy): de ella sale el valor de la UT, así que
  // registrar en octubre una factura de agosto usa la UT de agosto.
  sugerenciaRetencionISLR: ({ codigo, sujeto, base, fecha = '' }) =>
    request(`/api/config/conceptos-islr/sugerencia?codigo=${encodeURIComponent(codigo)}&sujeto=${encodeURIComponent(sujeto)}&base=${encodeURIComponent(base)}&fecha=${encodeURIComponent(fecha)}`),
  guardarConceptoISLR: (body) => request('/api/config/conceptos-islr', { method: 'POST', body: JSON.stringify(body) }),
  // Acumulado del ejercicio por concepto: decide el TRAMO de la escala (Tarifa 2
  // de los no domiciliados). Sin él la pantalla proyectaría siempre el primero.
  // COSTOS EN DESTINO: fletes e impuestos que encarecen una compra ya recibida.
  // Solo anexado — se corrige aplicando otro en negativo.
  // TRAZABILIDAD POR LOTE. La existencia por lote es una proyección del ledger;
  // "por-vencer" alimenta el aviso de caducidad.
  lotesDeProducto: (sku, sedeId = '') =>
    request(`/api/inventario/productos/${encodeURIComponent(sku)}/lotes?sedeId=${encodeURIComponent(sedeId)}`),
  // El RASTRO de un lote: «¿a quién le vendí el lote X?». Sin sede por defecto —
  // en una alerta el lote no respeta los límites de una sucursal.
  lotesHistoricos: (sku, sedeId = '') =>
    request(`/api/inventario/productos/${encodeURIComponent(sku)}/lotes/historico?sedeId=${encodeURIComponent(sedeId)}`),
  rastroDeLote: (sku, lote, sedeId = '') =>
    request(`/api/inventario/productos/${encodeURIComponent(sku)}/lotes/${encodeURIComponent(lote)}/rastro?sedeId=${encodeURIComponent(sedeId)}`),
  lotesPorVencer: ({ dias = 30, sedeId = '' } = {}) =>
    request(`/api/inventario/lotes/por-vencer?dias=${dias}&sedeId=${encodeURIComponent(sedeId)}`),
  costosEnDestino: (ordenId) => request(`/api/compras/ordenes/${encodeURIComponent(ordenId)}/costos-destino`),
  aplicarCostoEnDestino: (ordenId, body) =>
    request(`/api/compras/ordenes/${encodeURIComponent(ordenId)}/costos-destino`, { method: 'POST', body: JSON.stringify(body) }),
  acumuladoISLR: ({ terceroId, fecha = '' }) =>
    request(`/api/config/conceptos-islr/acumulado?terceroId=${encodeURIComponent(terceroId)}&fecha=${encodeURIComponent(fecha)}`),
  // UNIDAD TRIBUTARIA: de ella salen los sustraendos y mínimos de ISLR en
  // bolívares. Solo anexado — una UT pasada no se corrige, se carga la siguiente.
  unidadesTributarias: () => request('/api/config/unidades-tributarias'),
  cargarUT: (body) => request('/api/config/unidades-tributarias', { method: 'POST', body: JSON.stringify(body) }),
  crearAlicuota: (body) => request('/api/config/alicuotas', { method: 'POST', body: JSON.stringify(body) }),
  actualizarAlicuota: (id, body) => request(`/api/config/alicuotas/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  crearDispositivo: (body) => request('/api/config/dispositivos', { method: 'POST', body: JSON.stringify(body) }),
  actualizarDispositivo: (id, body) => request(`/api/config/dispositivos/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  desactivarDispositivo: (id) => request(`/api/config/dispositivos/${encodeURIComponent(id)}/desactivar`, { method: 'POST' }),

  // ---- Configuración › Unidades de medida ----
  // Maestro EDITABLE por empresa (no ledger): símbolo + nombre + categoría
  // (conteo|peso|volumen|longitud). El catálogo de producto elige su `unidadBase`
  // (el símbolo) de aquí. Sin borrado duro: se desactiva (activa=false), reversible
  // con el toggle (actualizarUnidad { activa:true }). Las ACTIVAS viajan además en
  // el bootstrap (db.UNIDADES) para alimentar el select del editor de producto.
  unidades: () => request('/api/config/unidades'),
  crearUnidad: (body) => request('/api/config/unidades', { method: 'POST', body: JSON.stringify(body) }),
  actualizarUnidad: (id, body) => request(`/api/config/unidades/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  desactivarUnidad: (id) => request(`/api/config/unidades/${encodeURIComponent(id)}/desactivar`, { method: 'POST' }),

  // ---- Configuración › Formatos de documento ----
  // Maestro EDITABLE por empresa (no ledger): plantillas visuales con las que se
  // imprime cada tipo de documento (factura, cotización, …). Cada formato es un
  // lienzo de un tamaño de papel con bloques posicionados en mm; el editor del
  // front los arrastra. `asignarFormato` fija qué formato usa una sede para un
  // tipo (plantillaId vacío = cae al predeterminado del tipo).
  formatos: () => request('/api/config/formatos'),
  formatoCampos: () => request('/api/config/formatos/campos'),
  crearFormato: (body) => request('/api/config/formatos', { method: 'POST', body: JSON.stringify(body) }),
  actualizarFormato: (id, body) => request(`/api/config/formatos/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  eliminarFormato: (id) => request(`/api/config/formatos/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  formatoPredeterminado: (id) => request(`/api/config/formatos/${encodeURIComponent(id)}/predeterminada`, { method: 'POST' }),
  asignarFormato: (body) => request('/api/config/formatos/asignar', { method: 'POST', body: JSON.stringify(body) }),

  // ---- Módulo Restaurante: mesas del salón + impresora de comandas ----
  // ---- Pedidos y delivery ----
  // La bandeja: los tres orígenes (mostrador, tienda web, app) caen en la misma
  // lista y se atienden igual.
  pedidos: () => request('/api/pedidos/'),
  pedido: (id) => request(`/api/pedidos/${encodeURIComponent(id)}`),
  crearPedido: (body) => request('/api/pedidos/', { method: 'POST', body: JSON.stringify(body) }),
  // Las acciones del ciclo. Cada una valida la transición en el servidor: la
  // pantalla oculta lo que no aplica, pero quien decide es el backend.
  accionPedido: (id, accion, body) => request(`/api/pedidos/${encodeURIComponent(id)}/${accion}`,
    { method: 'POST', body: JSON.stringify(body || {}) }),
  canalesPedido: () => request('/api/pedidos/config/canales'),
  guardarCanalPedido: (body) => request('/api/pedidos/config/canales', { method: 'PUT', body: JSON.stringify(body) }),
  zonasPedido: () => request('/api/pedidos/config/zonas'),
  guardarZonaPedido: (body) => request('/api/pedidos/config/zonas', { method: 'PUT', body: JSON.stringify(body) }),
  repartidores: () => request('/api/pedidos/config/repartidores'),
  guardarRepartidor: (body) => request('/api/pedidos/config/repartidores', { method: 'PUT', body: JSON.stringify(body) }),

  mesas: () => request('/api/restaurante/mesas'),
  crearMesa: (body) => request('/api/restaurante/mesas', { method: 'POST', body: JSON.stringify(body) }),
  actualizarMesa: (id, body) => request(`/api/restaurante/mesas/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  eliminarMesa: (id) => request(`/api/restaurante/mesas/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  guardarMapaMesas: (posiciones) => request('/api/restaurante/mapa', { method: 'POST', body: JSON.stringify({ posiciones }) }),
  planoSalon: () => request('/api/restaurante/plano'),
  // Catálogo de tipos de mostrador (barra, caja, postres…) y tintes de área. Del
  // servidor y no escrito en la pantalla: es quien valida el tipo al guardar.
  tiposMostrador: () => request('/api/restaurante/tipos-mostrador'),
  guardarPlanoSalon: (body) => request('/api/restaurante/plano', { method: 'PUT', body: JSON.stringify(body) }),
  asignacionesMesas: () => request('/api/restaurante/asignaciones'),
  guardarAsignacionMesas: (body) => request('/api/restaurante/asignaciones', { method: 'PUT', body: JSON.stringify(body) }),
  configSalon: () => request('/api/restaurante/config'),
  guardarConfigSalon: (body) => request('/api/restaurante/config', { method: 'PUT', body: JSON.stringify(body) }),
  // ---- Restaurante › Turnos del salón ----
  // Credenciales de mesonero (MS-) y sus turnos. El PIN del mesonero y el del
  // supervisor viajan en el cuerpo y los valida el SERVIDOR: la pantalla nunca
  // decide si un PIN sirve. Un PIN equivocado vuelve como 403 (403 y no 401: la
  // sesión es válida, lo que falló es una autorización puntual).
  mesonerosSalon: () => request('/api/restaurante/salon/mesoneros'),
  crearMesoneroSalon: (body) => request('/api/restaurante/salon/mesoneros', { method: 'POST', body: JSON.stringify(body) }),
  actualizarMesoneroSalon: (id, body) => request(`/api/restaurante/salon/mesoneros/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  fijarPinMesonero: (id, body) => request(`/api/restaurante/salon/mesoneros/${encodeURIComponent(id)}/pin`, { method: 'POST', body: JSON.stringify(body) }),
  turnosSalon: () => request('/api/restaurante/salon/turnos'),
  historialTurnos: () => request('/api/restaurante/salon/turnos/historial'),
  iniciarTurno: (body) => request('/api/restaurante/salon/turnos', { method: 'POST', body: JSON.stringify(body) }),
  finalizarTurno: (id) => request(`/api/restaurante/salon/turnos/${encodeURIComponent(id)}/finalizar`, { method: 'POST' }),
  relevosTurno: (id) => request(`/api/restaurante/salon/turnos/${encodeURIComponent(id)}/relevos`),
  forzarCierreTurno: (id, body) => request(`/api/restaurante/salon/turnos/${encodeURIComponent(id)}/forzar-cierre`, { method: 'POST', body: JSON.stringify(body) }),
  // Horarios y tiempo extra. El horario AVISA o dispara el cierre suave según el
  // modo de la sede (`horarioModo` en la config del salón); el tiempo extra lo
  // aprueba un supervisor con su PIN y queda registrado.
  horariosSalon: () => request('/api/restaurante/salon/horarios'),
  guardarHorarioMesonero: (id, body) => request(`/api/restaurante/salon/mesoneros/${encodeURIComponent(id)}/horario`, { method: 'PUT', body: JSON.stringify(body) }),

  // ---- Presencia estricta (plataforma, la consume el salón) ----
  // Las coordenadas viven en la SEDE y los roles que exigen presencia en la
  // empresa: es capacidad de plataforma, no del módulo Restaurante (el cajero es
  // el otro candidato natural).
  fijarUbicacionSede: (empresaId, sedeId, body) =>
    request(`/api/empresas/${encodeURIComponent(empresaId)}/sedes/${encodeURIComponent(sedeId)}/ubicacion`,
      { method: 'PUT', body: JSON.stringify(body) }),
  fijarRolesPresencia: (body) => request('/api/empresa/config/presencia', { method: 'PUT', body: JSON.stringify(body) }),
  extenderTurno: (id, body) => request(`/api/restaurante/salon/turnos/${encodeURIComponent(id)}/extender`, { method: 'POST', body: JSON.stringify(body) }),

  impresorasComandas: () => request('/api/restaurante/impresoras'),
  guardarImpresora: (body) => request('/api/restaurante/impresoras', { method: 'PUT', body: JSON.stringify(body) }),
  eliminarImpresora: (id) => request(`/api/restaurante/impresoras/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  // Cuentas de mesa (comandera)
  cuentasAbiertas: () => request('/api/restaurante/cuentas'),
  prefacturarCuenta: (id, body) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}/prefacturar`, { method: 'POST', body: JSON.stringify(body) }),
  cancelarPrefactura: (id) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}/prefacturar/cancelar`, { method: 'POST' }),
  cuenta: (id) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}`),
  abrirCuenta: (body) => request('/api/restaurante/cuentas', { method: 'POST', body: JSON.stringify(body) }),
  agregarItemsCuenta: (id, items) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}/items`, { method: 'POST', body: JSON.stringify({ items }) }),
  enviarCocina: (id) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}/enviar`, { method: 'POST' }),
  cancelarItemCuenta: (id, itemId) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}/items/${encodeURIComponent(itemId)}`, { method: 'DELETE' }),
  marcarItemCuenta: (id, itemId, estado) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}/items/${encodeURIComponent(itemId)}/estado`, { method: 'POST', body: JSON.stringify({ estado }) }),
  cerrarCuenta: (id) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}/cerrar`, { method: 'POST' }),
  previewCobroCuenta: (id) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}/preview-cobro`),
  // Reservaciones del salón. `fecha` elige el día (vacío = hoy) y `q` busca por
  // nombre o cédula, que es como se verifica a quien llega a la puerta.
  reservas: ({ fecha = '', q = '', proximas = false } = {}) => {
    const p = new URLSearchParams()
    if (fecha) p.set('fecha', fecha)
    if (q) p.set('q', q)
    if (proximas) p.set('proximas', '1')
    const qs = p.toString()
    return request('/api/restaurante/reservas' + (qs ? `?${qs}` : ''))
  },
  mesasReservadas: () => request('/api/restaurante/reservas/mesas'),
  crearReserva: (body) => request('/api/restaurante/reservas', { method: 'POST', body: JSON.stringify(body) }),
  actualizarReserva: (id, body) => request(`/api/restaurante/reservas/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  estadoReserva: (id, estado) => request(`/api/restaurante/reservas/${encodeURIComponent(id)}/estado`, { method: 'POST', body: JSON.stringify({ estado }) }),
  sentarReserva: (id, mesaId = '') => request(`/api/restaurante/reservas/${encodeURIComponent(id)}/sentar`, { method: 'POST', body: JSON.stringify({ mesaId }) }),
  cobrarCuenta: (id, body) => request(`/api/restaurante/cuentas/${encodeURIComponent(id)}/cobrar`, { method: 'POST', body: JSON.stringify(body) }),

  // ---- Configuración › Almacenes (por sede) ----
  // Maestro EDITABLE por empresa: un almacén pertenece a una sede; una sede tiene
  // ≥1 almacén y exactamente uno principal (del que despacha el POS). Sin borrado
  // duro: se desactiva (activo=false). `sede` opcional filtra por sede.
  almacenes: (sedeId) => request(`/api/config/almacenes${sedeId ? `?sede=${encodeURIComponent(sedeId)}` : ''}`),
  crearAlmacen: (body) => request('/api/config/almacenes', { method: 'POST', body: JSON.stringify(body) }),
  // UBICACIONES dentro del almacén (pasillo, estante, muelle).
  ubicaciones: (almacenId) => request(`/api/config/almacenes/${encodeURIComponent(almacenId)}/ubicaciones`),
  crearUbicacion: (almacenId, body) =>
    request(`/api/config/almacenes/${encodeURIComponent(almacenId)}/ubicaciones`, { method: 'POST', body: JSON.stringify(body) }),
  actualizarUbicacion: (almacenId, ubicacionId, body) =>
    request(`/api/config/almacenes/${encodeURIComponent(almacenId)}/ubicaciones/${encodeURIComponent(ubicacionId)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  // Dónde está un producto dentro de la sede. La suma es la existencia de la sede.
  existenciaPorUbicacion: (sku, sedeId = '') =>
    request(`/api/inventario/productos/${encodeURIComponent(sku)}/ubicaciones?sedeId=${encodeURIComponent(sedeId)}`),
  actualizarAlmacen: (id, body) => request(`/api/config/almacenes/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  desactivarAlmacen: (id) => request(`/api/config/almacenes/${encodeURIComponent(id)}/desactivar`, { method: 'POST' }),

  // ---- Aplicaciones (módulos instalables/activables por empresa) ----
  aplicaciones: () => request('/api/config/aplicaciones'),
  instalarModulo: (id) => request(`/api/config/aplicaciones/${encodeURIComponent(id)}/instalar`, { method: 'POST' }),
  desinstalarModulo: (id) => request(`/api/config/aplicaciones/${encodeURIComponent(id)}/desinstalar`, { method: 'POST' }),
  activarModulo: (id) => request(`/api/config/aplicaciones/${encodeURIComponent(id)}/activar`, { method: 'POST' }),
  desactivarModulo: (id) => request(`/api/config/aplicaciones/${encodeURIComponent(id)}/desactivar`, { method: 'POST' }),

  // ---- Configuración › Series y numeración fiscal ----
  // El próximo folio SOLO se puede fijar hacia adelante (para continuar la
  // numeración de un sistema previo): jamás retrocede ni reusa folios. El backend
  // rechaza cualquier retroceso con un 400.
  // Correlativos: { tipos: [...], otras: [...] }. Cada tipo trae su prefijo, su
  // rango autorizado y el contador de cada sede.
  numeracion: () => request('/api/config/numeracion'),
  fijarNumeracion: (body) => request('/api/config/numeracion/fijar', { method: 'POST', body: JSON.stringify(body) }),
  // Configura prefijo y rango de un tipo. body = { tipo, prefijo, desde, hasta }.
  configurarSerie: (body) => request('/api/config/numeracion/serie', { method: 'POST', body: JSON.stringify(body) }),
  // Número de Control (rango autorizado por el SENIAT): prefijo + desde/hasta.
  numeroControl: () => request('/api/config/numero-control'),
  configurarNumeroControl: (body) => request('/api/config/numero-control', { method: 'POST', body: JSON.stringify(body) }),

  // ---- Reportes y BI ----
  // Agregaciones DERIVADAS del ledger (documentos, movimientos, órdenes, CxC), de
  // solo lectura. Consolidan todas las sedes de la empresa. Los reportes con rango
  // aceptan desde/hasta YYYY-MM-DD (default: mes en curso en el servidor).
  repPanel: () => request('/api/reportes/panel'),
  repVentas: (desde, hasta) => request(`/api/reportes/ventas?desde=${encodeURIComponent(desde || '')}&hasta=${encodeURIComponent(hasta || '')}`),
  repInventario: () => request('/api/reportes/inventario'),
  repCompras: (desde, hasta) => request(`/api/reportes/compras?desde=${encodeURIComponent(desde || '')}&hasta=${encodeURIComponent(hasta || '')}`),
  repCobranza: () => request('/api/reportes/cobranza'),

  // Documentos legales (Términos, Privacidad). `vigente` es público (contenido +
  // versión + hash). `estado` lista lo pendiente por el usuario; `aceptar` registra la
  // aceptación auditable (el servidor fija versión y hash).
  legalVigente: () => request('/api/legal/vigente'),
  legalEstado: () => request('/api/legal/estado'),
  legalAceptar: (documento) => request('/api/legal/aceptar', { method: 'POST', body: JSON.stringify({ documento }) }),

  // Asistente IA (módulo "asistente-ia"). /ai/ask responde en dos capas: primero
  // MECÁNICA (local, sobre los datos); si no aplica y la empresa habilitó la IA,
  // escala al proxy de IA de Hubmy. La configuración es el opt-in de la capa IA.
  aiAsk: (message) => request('/api/ai/ask', { method: 'POST', body: JSON.stringify({ message }) }),
  asistenteConfig: () => request('/api/ai/config'),
  configurarAsistente: (body) => request('/api/ai/config', { method: 'PATCH', body: JSON.stringify(body) }),
}
