// Cliente HTTP de la consola hacia su BFF (/papi/*). Sesión por cookie (credentials).
// El BFF es quien habla con el core /internal/* y guarda la clave M2M.

async function request(path, opts = {}) {
  const res = await fetch(path, {
    credentials: 'include',
    headers: opts.body ? { 'Content-Type': 'application/json' } : {},
    ...opts,
  })
  if (res.status === 204) return null
  const ct = res.headers.get('Content-Type') || ''
  const data = ct.includes('application/json') ? await res.json() : await res.text()
  if (!res.ok) {
    const err = new Error((data && data.error) || res.statusText || 'Error')
    err.status = res.status
    err.data = data
    throw err
  }
  return data
}

export const api = {
  // --- Autenticación ---
  me: () => request('/papi/auth/me'),
  login: (body) => request('/papi/auth/login', { method: 'POST', body: JSON.stringify(body) }),
  mfaSetup: (body) => request('/papi/auth/mfa/setup', { method: 'POST', body: JSON.stringify(body) }),
  mfaVerify: (body) => request('/papi/auth/mfa/verify', { method: 'POST', body: JSON.stringify(body) }),
  logout: () => request('/papi/auth/logout', { method: 'POST' }),

  // --- Clientes (tenants) ---
  tenants: () => request('/papi/tenants'),
  tenant: (empresaId) => request(`/papi/tenants/${encodeURIComponent(empresaId)}`),
  auditoria: (empresaId, { desde = '', hasta = '', limite = 200 } = {}) =>
    request(`/papi/tenants/${encodeURIComponent(empresaId)}/auditoria?desde=${encodeURIComponent(desde)}&hasta=${encodeURIComponent(hasta)}&limite=${limite}`),
  uso: (orgId) => request(`/papi/orgs/${encodeURIComponent(orgId)}/uso`),
  fijarEstadoOrg: (orgId, estado) => request(`/papi/orgs/${encodeURIComponent(orgId)}/estado`, { method: 'PATCH', body: JSON.stringify({ estado }) }),
  fijarPlan: (orgId, plan, limites) => request(`/papi/orgs/${encodeURIComponent(orgId)}/plan`, { method: 'PATCH', body: JSON.stringify({ plan, limites }) }),
  fijarEmpresaActiva: (empresaId, activa) => request(`/papi/empresas/${encodeURIComponent(empresaId)}/activa`, { method: 'PATCH', body: JSON.stringify({ activa }) }),
  crearSandbox: (empresaId, ttlDias) => request(`/papi/tenants/${encodeURIComponent(empresaId)}/sandbox`, { method: 'POST', body: JSON.stringify({ ttlDias }) }),
  eliminarSandbox: (empresaId) => request(`/papi/empresas/${encodeURIComponent(empresaId)}`, { method: 'DELETE' }),
  // Restaurar un respaldo (crea una empresa NUEVA en la org destino). Sube el binario.
  restaurar: async (orgId, file) => {
    const res = await fetch(`/papi/restore?orgId=${encodeURIComponent(orgId)}`, { method: 'POST', credentials: 'include', body: file })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) throw new Error(data.error || 'No se pudo restaurar')
    return data
  },

  // Export/backup: descarga binaria (octet-stream). Se maneja aparte para el blob.
  exportURL: (empresaId) => `/papi/tenants/${encodeURIComponent(empresaId)}/export`,

  // --- Solicitudes de demo (leads del formulario de la web) ---
  leads: (estado = '') => request(`/papi/leads${estado ? `?estado=${encodeURIComponent(estado)}` : ''}`),
  actualizarLead: (id, body) => request(`/papi/leads/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),

  // --- Facturación / suscripciones ---
  planes: () => request('/papi/planes'),
  crearPlan: (body) => request('/papi/planes', { method: 'POST', body: JSON.stringify(body) }),
  editarPlan: (id, body) => request(`/papi/planes/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) }),
  archivarPlan: (id) => request(`/papi/planes/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  resumenFacturacion: () => request('/papi/facturacion'),
  suscripcion: (orgId) => request(`/papi/orgs/${encodeURIComponent(orgId)}/suscripcion`),
  asignarPlan: (orgId, planId, hubmyUserId) => request(`/papi/orgs/${encodeURIComponent(orgId)}/suscripcion`, { method: 'POST', body: JSON.stringify({ planId, hubmyUserId }) }),
  marcarPagada: (orgId) => request(`/papi/orgs/${encodeURIComponent(orgId)}/suscripcion/marcar-pagada`, { method: 'POST' }),
  cancelarSuscripcion: (orgId) => request(`/papi/orgs/${encodeURIComponent(orgId)}/suscripcion/cancelar`, { method: 'POST' }),
  sincronizarSuscripcion: (orgId) => request(`/papi/orgs/${encodeURIComponent(orgId)}/suscripcion/sincronizar`, { method: 'POST' }),
}

// Formatea centavos + moneda (1099, "USD") → "$10,99" estilo VE.
export function money(cents, moneda = 'USD') {
  const v = (cents || 0) / 100
  const s = v.toLocaleString('es-VE', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  const simbolo = moneda === 'USD' ? '$' : moneda === 'VES' ? 'Bs ' : ''
  return simbolo + s + (simbolo ? '' : ' ' + moneda)
}

// Etiqueta legible de un estado de suscripción.
export const ESTADO_SUB = {
  activa: { label: 'Activa', color: 'emerald' },
  trial: { label: 'Prueba', color: 'teal' },
  pendiente_pago: { label: 'Pendiente de pago', color: 'amber' },
  vencida: { label: 'Vencida', color: 'red' },
  cancelada: { label: 'Cancelada', color: 'slate' },
}

// descargarExport hace el POST y fuerza la descarga del archivo (gzip[.enc]).
export async function descargarExport(empresaId) {
  const res = await fetch(api.exportURL(empresaId), { method: 'POST', credentials: 'include' })
  if (!res.ok) {
    const t = await res.text().catch(() => '')
    throw new Error(t || 'No se pudo generar el respaldo')
  }
  const blob = await res.blob()
  const cd = res.headers.get('Content-Disposition') || ''
  const m = cd.match(/filename="?([^"]+)"?/)
  const nombre = m ? m[1] : `huberp-export-${empresaId}.json.gz`
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = nombre
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
  return { nombre, cifrado: res.headers.get('X-Export-Encrypted') === 'true' }
}
