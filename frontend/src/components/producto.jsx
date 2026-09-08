import { useState, useEffect } from 'react'
import { Icon } from './Icon.jsx'
import { Modal, Badge } from './primitives.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { api } from '../lib/api.js'
import { monedaDe, precioEnMoneda } from '../lib/precio.js'

/* Tarjeta de producto de la rejilla del punto de venta.
 *
 * Sigue al prototipo (03 §4.4/§4.8):
 *   · zona superior con la imagen; si no hay, un marcador EXPLÍCITO de «sin
 *     imagen» — nunca un hueco silencioso ni una foto ajena;
 *   · seis matices pastel derivados del código, estables por producto, para que
 *     el cajero reconozca la mercancía por color aunque no haya foto;
 *   · botón de información en la esquina → disponibilidad por sede;
 *   · nombre bien visible y CÓDIGO en pequeño, para confirmar que es el producto
 *     correcto antes de cobrarlo;
 *   · existencias con semáforo: rojo agotado, ámbar bajo, gris normal.
 */

// Los seis matices del prototipo. El índice sale de un hash estable del código,
// así el mismo producto siempre se ve del mismo color en todas las pantallas.
const MATICES = [
  { fondo: '#E9EDF6', tinta: '#1D3477' }, // azul
  { fondo: '#E4F3EC', tinta: '#1E7A4C' }, // verde
  { fondo: '#FBF1DC', tinta: '#92600A' }, // ámbar
  { fondo: '#FBEDEB', tinta: '#B3362C' }, // rojo
  { fondo: '#EDEAF6', tinta: '#4A3A8F' }, // violeta
  { fondo: '#E3F1F4', tinta: '#146B7A' }, // cian
]

export const maticeDe = (clave = '') => {
  let h = 0
  for (let i = 0; i < clave.length; i++) h = (h * 31 + clave.charCodeAt(i)) % 997
  return MATICES[h % MATICES.length]
}

const UMBRAL_BAJO = 5

// Nivel de existencias con su color semántico.
export function nivelStock(cant) {
  const n = Number(cant) || 0
  if (n <= 0) return { nivel: 'agotado', clase: 'text-[#B3362C] dark:text-red-400 font-semibold' }
  if (n <= UMBRAL_BAJO) return { nivel: 'bajo', clase: 'text-amber-700 dark:text-amber-400 font-semibold' }
  return { nivel: 'disponible', clase: 'text-slate-500' }
}

/* Zona de imagen. `producto.imagenUrl` vacío ⇒ marcador de «sin imagen». */
export function ImagenProducto({ producto, alto = 96, className = '' }) {
  const m = maticeDe(producto.sku || producto.nombre)
  // Si la URL existe pero el archivo no carga (se borró, cambió el bucket, o no
  // hay red), se cae al marcador de «sin imagen». Nunca se deja el icono de
  // imagen rota del navegador: en una caja eso parece una falla del sistema.
  const [falló, setFalló] = useState(false)
  if (producto.imagenUrl && !falló) {
    return (
      <div className={`w-full overflow-hidden ${className}`} style={{ height: alto, background: m.fondo }}>
        <img src={producto.imagenUrl} alt={producto.nombre} loading="lazy"
          onError={() => setFalló(true)} className="w-full h-full object-cover" />
      </div>
    )
  }
  return (
    <div className={`w-full flex flex-col items-center justify-center gap-1 ${className}`}
      style={{ height: alto, background: m.fondo, color: m.tinta }}>
      <span className="font-display font-bold leading-none" style={{ fontSize: Math.round(alto * 0.30) }}>
        {(producto.nombre || '?').trim().charAt(0).toUpperCase()}
      </span>
      <span className="text-[10px] font-medium opacity-70">Sin imagen</span>
    </div>
  )
}

/* Tarjeta táctil. `onInfo` abre la disponibilidad; sin él no se pinta el botón. */
export function ProductoTarjeta({ producto, existencia, onClick, onInfo, ccy = 'VES', tasa = 0, monedaEmpresa = 'VES' }) {
  const st = nivelStock(existencia)
  const agotado = st.nivel === 'agotado'
  // El precio puede estar guardado en US$ (R10): se muestra en la moneda de
  // presentación elegida, convirtiéndolo con la tasa vigente. Sin tasa, se
  // muestra en su moneda original en vez de un número convertido a la fuerza.
  const propia = monedaDe(producto, monedaEmpresa)
  const convertido = precioEnMoneda(producto, ccy, monedaEmpresa, tasa)
  const precio = convertido === null ? Number(producto.precio) || 0 : convertido
  const monedaMostrada = convertido === null ? propia : ccy

  return (
    <div className={`relative rounded-xl overflow-hidden bg-white dark:bg-slate-900 border
      border-slate-200 dark:border-slate-800 transition-all
      ${agotado ? 'opacity-70' : 'hover:border-elerp-300 hover:shadow-brand'}`}>
      <button type="button" onClick={onClick} disabled={agotado} title={agotado ? 'Sin existencias' : producto.nombre}
        className="w-full text-left disabled:cursor-not-allowed ring-focus">
        <ImagenProducto producto={producto} alto={96} />
        <div className="p-3">
          {/* Nombre bien visible; el código, pequeño, para confirmar el producto. */}
          <div className="text-[13.5px] font-semibold leading-tight line-clamp-2 min-h-[34px]">{producto.nombre}</div>
          <div className="mono text-[10.5px] text-slate-400 mt-0.5 truncate">{producto.sku}</div>
          <div className="flex items-end justify-between gap-2 mt-2">
            <span className="font-display font-bold text-[15px] tnum">{fmtCurrency(precio, monedaMostrada)}</span>
            <span className={`text-[11.5px] tnum ${st.clase}`}>
              {agotado ? 'Agotado' : `${fmtNum(existencia, 2)} disp.`}
            </span>
          </div>
        </div>
      </button>

      {onInfo ? (
        <button type="button" onClick={(e) => { e.stopPropagation(); onInfo(producto) }}
          title="Ver disponibilidad por sede" aria-label={`Disponibilidad de ${producto.nombre}`}
          className="absolute top-2 right-2 h-7 w-7 rounded-full bg-white/90 dark:bg-slate-900/90 backdrop-blur
            text-slate-500 hover:text-elerp-500 inline-flex items-center justify-center shadow-card ring-focus">
          <Icon.CircleAlert size={15} />
        </button>
      ) : null}
    </div>
  )
}

/* Modal de disponibilidad por ubicación (03 §4.7).
 * Responde «¿cuántas unidades hay y en qué tienda?» — la sede actual marcada, el
 * resto de la red, y el total al pie. El semáforo lo calcula el servidor. */
export function DisponibilidadModal({ sku, open, onClose }) {
  const [data, setData] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open || !sku) return
    setData(null); setError('')
    api.disponibilidad(sku)
      .then(setData)
      .catch((e) => setError(e?.message || 'No se pudo consultar la disponibilidad.'))
  }, [open, sku])

  const COLOR = {
    agotado: 'text-[#B3362C] dark:text-red-400',
    bajo: 'text-amber-700 dark:text-amber-400',
    disponible: 'text-[#1E7A4C] dark:text-teal-400',
  }
  const ETIQUETA = { agotado: 'Agotado', bajo: 'Quedan pocas', disponible: 'Disponible' }

  return (
    <Modal open={open} onClose={onClose} title="Disponibilidad por tienda"
      sub={data ? `${data.nombre} · ${data.sku}` : 'Consultando…'}>
      {error ? (
        <div className="flex items-start gap-2 text-[13px] text-[#B3362C] dark:text-red-400">
          <Icon.CircleAlert size={16} className="mt-0.5 shrink-0" /><span>{error}</span>
        </div>
      ) : !data ? (
        <div className="space-y-2">
          <div className="skeleton h-12 rounded-lg" /><div className="skeleton h-12 rounded-lg" />
        </div>
      ) : (
        <div>
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {data.sedes.map((s) => (
              <div key={s.sedeId} className="flex items-center gap-3 py-3">
                <Icon.Home size={16} className={s.estaTienda ? 'text-teal-500 shrink-0' : 'text-slate-300 shrink-0'} />
                <span className="flex-1 min-w-0">
                  <span className="block text-[13.5px] font-medium truncate">{s.sedeNombre}</span>
                  {s.estaTienda ? <span className="block text-[11px] text-teal-600 dark:text-teal-400">esta tienda</span> : null}
                </span>
                <span className="text-right shrink-0">
                  <span className={`block font-display font-bold text-[15px] tnum ${COLOR[s.nivel] || ''}`}>
                    {fmtNum(s.cantidad, 2)}
                  </span>
                  <span className="block text-[10.5px] text-slate-400">{ETIQUETA[s.nivel] || s.nivel}</span>
                </span>
              </div>
            ))}
          </div>
          <div className="flex items-center justify-between pt-3 mt-1 border-t border-slate-200 dark:border-slate-700">
            <span className="text-[13px] font-semibold">Total en la red</span>
            <span className="font-display font-bold text-[17px] tnum">{fmtNum(data.totalRed, 2)}</span>
          </div>
          <div className="text-[11.5px] text-slate-400 mt-3">
            Para traer mercancía de otra tienda, crea una transferencia entre sedes.
          </div>
        </div>
      )}
    </Modal>
  )
}

/* Rejilla + su cabecera. `modo` alterna entre la búsqueda por teclado (la que usa
 * la pistola lectora de código de barras) y el catálogo visual. */
export function RejillaProductos({ productos, existenciaDe, onAgregar, onInfo, ccy, tasa, monedaEmpresa = 'VES', columnas = 4 }) {
  if (!productos.length) {
    return (
      <div className="py-14 text-center">
        <Icon.Package size={26} className="mx-auto text-slate-300 mb-2" />
        <div className="text-[13.5px] font-medium">Sin resultados</div>
        <div className="text-[12.5px] text-slate-500 mt-0.5">Prueba con otro término, o revisa el catálogo completo.</div>
      </div>
    )
  }
  const cols = { 3: 'sm:grid-cols-2 lg:grid-cols-3', 4: 'sm:grid-cols-3 lg:grid-cols-4', 5: 'sm:grid-cols-3 lg:grid-cols-5' }
  return (
    <div className={`grid grid-cols-2 ${cols[columnas] || cols[4]} gap-3`}>
      {productos.map((p) => (
        <ProductoTarjeta key={p.id || p.sku} producto={p} existencia={existenciaDe(p)}
          onClick={() => onAgregar(p)} onInfo={onInfo} ccy={ccy} tasa={tasa} monedaEmpresa={monedaEmpresa} />
      ))}
    </div>
  )
}
