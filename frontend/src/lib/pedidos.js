/* LÓGICA DE LA BANDEJA DE PEDIDOS.
 *
 * Vive en lib y no dentro de la pantalla porque es donde se decide QUÉ PUEDE
 * HACERSE con un pedido, y eso tiene que coincidir con lo que el servidor
 * permite. La pantalla solo oculta; quien decide es el backend — pero ofrecer un
 * botón que va a fallar es peor que no ofrecerlo, sobre todo en hora pico.
 */

export const ESTADOS = {
  nuevo: 'Por revisar',
  confirmado: 'Confirmado',
  en_preparacion: 'En preparación',
  listo: 'Listo para despachar',
  asignado: 'Asignado',
  retirado_canal: 'Retirado por la app',
  en_ruta: 'En camino',
  entregado: 'Entregado',
  rechazado: 'Rechazado',
  cancelado: 'Cancelado',
  entrega_fallida: 'Entrega fallida',
  devuelto: 'Devuelto al local',
}

export const etiquetaEstado = (e) => ESTADOS[e] || e

const COLORES = {
  nuevo: { bg: '#EDF2F9', text: '#1D3477' },
  confirmado: { bg: '#EDF2F9', text: '#1D3477' },
  en_preparacion: { bg: '#FDF6E7', text: '#92600A' },
  listo: { bg: '#EAF5EF', text: '#166B41' },
  asignado: { bg: '#EAF5EF', text: '#166B41' },
  retirado_canal: { bg: '#EAF5EF', text: '#166B41' },
  en_ruta: { bg: '#EAF5EF', text: '#166B41' },
  entregado: { bg: '#F0F2EF', text: '#5C6470' },
  rechazado: { bg: '#FBEDEB', text: '#B3362C' },
  cancelado: { bg: '#FBEDEB', text: '#B3362C' },
  entrega_fallida: { bg: '#FBEDEB', text: '#B3362C' },
  devuelto: { bg: '#F0F2EF', text: '#5C6470' },
}

export const colorEstado = (e) => COLORES[e] || COLORES.nuevo

export function etiquetaOrigen(o) {
  if (o === 'manual') return 'Mostrador'
  if (o === 'ecommerce') return 'Tienda web'
  if (o === 'app_commerce') return 'App de pedidos'
  return 'Delivery'
}

/** FINALES: los estados donde el pedido dejó de moverse. */
const FINALES = ['entregado', 'rechazado', 'cancelado', 'devuelto']
export const esFinal = (estado) => FINALES.includes(estado)

/** accionesDe devuelve qué se puede hacer AHORA con un pedido. Es el espejo de
 *  la tabla de transiciones del servidor: si las dos se separan, la pantalla
 *  ofrece botones que fallan.
 *
 *  `pideMotivo` marca las que no se ejecutan de un toque: un pedido que se cae
 *  sin explicación no deja aprender nada ni responderle al cliente. */
export function accionesDe(p) {
  const e = p?.estado
  const propio = p?.modoEnvio !== 'canal'
  switch (e) {
    case 'nuevo':
      return [
        { id: 'confirmar', label: 'Confirmar' },
        { id: 'rechazar', label: 'Rechazar', tono: 'peligro', pideMotivo: true },
      ]
    case 'confirmado':
    case 'en_preparacion':
      return [
        { id: 'listo', label: 'Listo para despachar' },
        { id: 'cancelar', label: 'Cancelar', tono: 'suave', pideMotivo: true },
      ]
    case 'listo':
      // Con envío del canal no se asigna repartidor: el courier de la app lo
      // retira, y ofrecer asignar sería ofrecer algo que no existe.
      return propio
        ? [
          { id: 'asignar', label: 'Asignar repartidor', pideDato: true },
          { id: 'en-ruta', label: 'Salió a ruta', tono: 'suave' },
          { id: 'cancelar', label: 'Cancelar', tono: 'suave', pideMotivo: true },
        ]
        : [{ id: 'cancelar', label: 'Cancelar', tono: 'suave', pideMotivo: true }]
    case 'asignado':
      return [
        { id: 'en-ruta', label: 'Salió a ruta' },
        { id: 'cancelar', label: 'Cancelar', tono: 'suave', pideMotivo: true },
      ]
    case 'en_ruta':
    case 'retirado_canal':
      return [
        { id: 'entregado', label: 'Entregado', pideDato: true },
        { id: 'fallida', label: 'No se pudo entregar', tono: 'peligro', pideMotivo: true },
      ]
    case 'entrega_fallida':
      return [
        { id: 'en-ruta', label: 'Reintentar entrega' },
        { id: 'entregado', label: 'Entregado', pideDato: true },
      ]
    default:
      return []
  }
}

/** esperaConfirmacionDeListo: cocina terminó y falta que alguien confirme que el
 *  pedido está armado y se puede retirar.
 *
 *  PRODUCCIÓN LISTA NO ES PEDIDO LISTO, y confundirlas manda al repartidor a
 *  buscar algo que todavía está en la cocina. El plato puede estar hecho y el
 *  pedido sin empacar, sin la bebida, o incompleto. */
export const esperaConfirmacionDeListo = (p) =>
  p?.estado === 'en_preparacion' && !!p?.produccionLista

/** pagoDelPedido resume si hay que cobrar en la puerta. Es lo PRIMERO que el
 *  repartidor necesita saber: cobrar lo ya pagado es el error que más caro
 *  sale, y no cobrar lo que había que cobrar lo paga él. */
export function pagoDelPedido(p) {
  if (p?.formaPago === 'en_canal') {
    return { cobra: false, etiqueta: 'Ya pagado', detalle: 'El cliente pagó al hacer el pedido. No cobres nada.' }
  }
  return { cobra: true, etiqueta: 'Cobrar al entregar', detalle: 'Cobra el total en la puerta.' }
}

/** minutosRestantes de la ventana de aceptación. null cuando no hay ventana: un
 *  reloj que no existe no debe dibujarse como si corriera. */
export function minutosRestantes(vence) {
  if (!vence) return null
  const ms = new Date(vence).getTime() - Date.now()
  if (Number.isNaN(ms)) return null
  return Math.max(0, Math.round(ms / 60000))
}

/** estaDemorado: venció la promesa de entrega sin haber entregado. Se marca en
 *  rojo igual que una comanda demorada en cocina — es la misma urgencia. */
export function estaDemorado(p) {
  if (!p?.promesaEntrega || esFinal(p.estado)) return false
  return new Date(p.promesaEntrega).getTime() < Date.now()
}

/** agruparBandeja separa lo que hay que revisar de lo que ya está en curso.
 *
 *  Son dos trabajos distintos: revisar es decidir si se acepta, y atender es
 *  empujar lo aceptado. Mezclarlos hace que lo urgente se pierda entre lo que
 *  simplemente está cocinándose. */
export function agruparBandeja(pedidos) {
  const nuevos = []
  const activos = []
  const cerrados = []
  let demorados = 0
  for (const p of pedidos || []) {
    if (esFinal(p.estado)) { cerrados.push(p); continue }
    if (estaDemorado(p)) demorados++
    if (p.estado === 'nuevo') nuevos.push(p)
    else activos.push(p)
  }
  return { nuevos, activos, cerrados, demorados }
}
