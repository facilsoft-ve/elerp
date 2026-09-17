import { useState, useRef, useEffect, useCallback } from 'react'
import { Icon } from './Icon.jsx'
import { Button } from './primitives.jsx'

/* SELECTOR DE UBICACIÓN DE LA SEDE, sobre un mapa.
 *
 * POR QUÉ SIN LIBRERÍA: este frontend depende de react y react-dom, nada más.
 * Meter Leaflet (~43 KB comprimido) por un selector que se usa una vez por sede
 * rompería esa disciplina, y el mapa que hace falta es un mapa de tiles con un
 * marcador: la matemática de Web Mercator son diez líneas y los tiles son
 * imágenes. Lo que sí evita una librería —gestos finos, capas, rotación— no
 * hace falta acá.
 *
 * DEGRADA CON GRACIA, que en Venezuela no es un detalle: si los tiles no cargan
 * (sin internet, servidor caído, red del local bloqueada), se ve la cuadrícula
 * vacía pero el marcador, el radio, las coordenadas y el botón «usar mi
 * ubicación» siguen funcionando. Poner la ubicación no puede depender de que
 * haya internet en ese momento.
 *
 * Los tiles vienen de OpenStreetMap. Es una pantalla de configuración —volumen
 * bajísimo, una vez por sede— que es justo el uso que su política admite.
 */

const TAM = 256 // lado del tile, en píxeles
const ZOOM_MIN = 3
const ZOOM_MAX = 18

/* --- Web Mercator ---------------------------------------------------------
 * Convierte entre grados y «píxeles del mundo» a un zoom dado. El mundo entero
 * mide 256 × 2^z píxeles; de ahí sale todo lo demás. */

const lonAX = (lon, z) => ((lon + 180) / 360) * TAM * 2 ** z

const latAY = (lat, z) => {
  const rad = (lat * Math.PI) / 180
  return ((1 - Math.log(Math.tan(rad) + 1 / Math.cos(rad)) / Math.PI) / 2) * TAM * 2 ** z
}

const xALon = (x, z) => (x / (TAM * 2 ** z)) * 360 - 180

const yALat = (y, z) => {
  const n = Math.PI - (2 * Math.PI * y) / (TAM * 2 ** z)
  return (180 / Math.PI) * Math.atan(Math.sinh(n))
}

// metrosPorPixel a una latitud y zoom dados. Se usa para dibujar el radio a
// escala: un círculo de 150 m tiene que MEDIR 150 m en el mapa, o no sirve para
// entender cuánto cubre la regla.
const metrosPorPixel = (lat, z) =>
  (156543.03392 * Math.cos((lat * Math.PI) / 180)) / 2 ** z

// Caracas, como centro por defecto cuando la sede aún no tiene ubicación.
const CENTRO_POR_DEFECTO = { lat: 10.4806, lon: -66.9036 }

export function MapaSede({ lat, lon, radioM = 150, onCambio, alto = 300, disabled = false }) {
  const hayPunto = Number.isFinite(lat) && Number.isFinite(lon) && !(lat === 0 && lon === 0)
  const centro = hayPunto ? { lat, lon } : CENTRO_POR_DEFECTO

  const [zoom, setZoom] = useState(hayPunto ? 16 : 12)
  // `vista` es el centro que se está mirando; puede diferir del punto elegido
  // mientras se arrastra el mapa.
  const [vista, setVista] = useState(centro)
  const [ancho, setAncho] = useState(480)
  const [arrastrando, setArrastrando] = useState(false)
  const cajaRef = useRef(null)
  const arrastre = useRef(null)
  const movio = useRef(false)

  // El ancho se mide del contenedor: el mapa ocupa lo que le den, y sin esto los
  // tiles se calcularían contra un ancho inventado y quedarían corridos.
  useEffect(() => {
    const medir = () => { if (cajaRef.current) setAncho(cajaRef.current.clientWidth) }
    medir()
    window.addEventListener('resize', medir)
    return () => window.removeEventListener('resize', medir)
  }, [])

  // Al recibir un punto nuevo desde afuera (p. ej. «usar mi ubicación»), la
  // vista lo sigue. Solo cuando cambia de verdad: seguirlo siempre impediría
  // arrastrar el mapa para mirar alrededor.
  useEffect(() => {
    if (hayPunto) setVista({ lat, lon })
  }, [lat, lon, hayPunto])

  // Píxeles del mundo del centro de la vista.
  const cx = lonAX(vista.lon, zoom)
  const cy = latAY(vista.lat, zoom)
  // Esquina superior izquierda del lienzo, en píxeles del mundo.
  const x0 = cx - ancho / 2
  const y0 = cy - alto / 2

  // Tiles visibles. Se calcula el rango y se pintan como <img>: el navegador se
  // encarga de la caché y de los que no lleguen.
  const tiles = []
  const maxTile = 2 ** zoom
  for (let tx = Math.floor(x0 / TAM); tx <= Math.floor((x0 + ancho) / TAM); tx++) {
    for (let ty = Math.floor(y0 / TAM); ty <= Math.floor((y0 + alto) / TAM); ty++) {
      if (ty < 0 || ty >= maxTile) continue // fuera de los polos: no existe tile
      // El eje X SÍ envuelve (el mundo es cilíndrico), así que se normaliza.
      const txn = ((tx % maxTile) + maxTile) % maxTile
      tiles.push({ tx, ty, txn, izq: tx * TAM - x0, arr: ty * TAM - y0 })
    }
  }

  const puntoEnPantalla = hayPunto
    ? { izq: lonAX(lon, zoom) - x0, arr: latAY(lat, zoom) - y0 }
    : null
  const radioPx = hayPunto ? radioM / metrosPorPixel(lat, zoom) : 0

  const alSoltar = useCallback((e) => {
    const d = arrastre.current
    arrastre.current = null
    setArrastrando(false)
    if (!d || !cajaRef.current) return
    // Un clic sin desplazamiento COLOCA el punto; con desplazamiento fue un
    // arrastre del mapa y no debe mover el marcador — si no, cada intento de
    // mirar alrededor reubicaría la sede.
    if (movio.current || disabled) return
    const r = cajaRef.current.getBoundingClientRect()
    const px = e.clientX - r.left
    const py = e.clientY - r.top
    onCambio?.({ lat: yALat(y0 + py, zoom), lon: xALon(x0 + px, zoom) })
  }, [x0, y0, zoom, onCambio, disabled])

  useEffect(() => {
    if (!arrastrando) return undefined
    const mover = (e) => {
      const d = arrastre.current
      if (!d) return
      const dx = e.clientX - d.x
      const dy = e.clientY - d.y
      if (Math.abs(dx) > 3 || Math.abs(dy) > 3) movio.current = true
      arrastre.current = { x: e.clientX, y: e.clientY }
      setVista((v) => ({
        lat: yALat(latAY(v.lat, zoom) - dy, zoom),
        lon: xALon(lonAX(v.lon, zoom) - dx, zoom),
      }))
    }
    window.addEventListener('pointermove', mover)
    window.addEventListener('pointerup', alSoltar)
    return () => {
      window.removeEventListener('pointermove', mover)
      window.removeEventListener('pointerup', alSoltar)
    }
  }, [arrastrando, zoom, alSoltar])

  const alPresionar = (e) => {
    arrastre.current = { x: e.clientX, y: e.clientY }
    movio.current = false
    setArrastrando(true)
  }

  const zoomA = (d) => setZoom((z) => Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, z + d)))

  return (
    <div>
      <div ref={cajaRef} onPointerDown={alPresionar}
        className="relative overflow-hidden rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-100 dark:bg-slate-800 select-none"
        style={{ height: alto, cursor: disabled ? 'default' : arrastrando ? 'grabbing' : 'crosshair' }}>
        {tiles.map((t) => (
          <img key={`${t.tx}-${t.ty}`} alt="" draggable={false} loading="lazy"
            src={`https://tile.openstreetmap.org/${zoom}/${t.txn}/${t.ty}.png`}
            style={{ position: 'absolute', left: t.izq, top: t.arr, width: TAM, height: TAM }}
            // Un tile que no llega se esconde: mejor el fondo liso que el icono
            // de imagen rota repetido por toda la pantalla.
            onError={(e) => { e.currentTarget.style.visibility = 'hidden' }} />
        ))}

        {/* El RADIO a escala: un círculo de 150 m mide 150 m. Es lo que deja ver
            de un vistazo cuánto territorio cubre la regla de presencia. */}
        {puntoEnPantalla && radioPx > 2 ? (
          <div className="pointer-events-none absolute rounded-full"
            style={{
              left: puntoEnPantalla.izq - radioPx, top: puntoEnPantalla.arr - radioPx,
              width: radioPx * 2, height: radioPx * 2,
              background: 'rgba(106,44,240,.14)', border: '2px solid rgba(106,44,240,.55)',
            }} />
        ) : null}

        {puntoEnPantalla ? (
          <div className="pointer-events-none absolute" style={{ left: puntoEnPantalla.izq - 9, top: puntoEnPantalla.arr - 9 }}>
            <span className="block h-[18px] w-[18px] rounded-full bg-elerp-500 border-[3px] border-white shadow-md" />
          </div>
        ) : null}

        <div className="absolute right-2 top-2 flex flex-col gap-1">
          {[['+', 1], ['−', -1]].map(([etiqueta, d]) => (
            <button key={etiqueta} type="button" aria-label={d > 0 ? 'Acercar' : 'Alejar'}
              onPointerDown={(e) => e.stopPropagation()} onClick={() => zoomA(d)}
              className="h-7 w-7 rounded-lg bg-white/95 dark:bg-slate-900/95 border border-slate-200 dark:border-slate-700 text-[15px] font-semibold shadow-sm">
              {etiqueta}
            </button>
          ))}
        </div>

        {/* Atribución: OpenStreetMap la exige para usar sus tiles. */}
        <div className="absolute bottom-0 right-0 text-[10px] px-1.5 py-0.5 bg-white/80 dark:bg-slate-900/80 text-slate-500">
          © OpenStreetMap
        </div>

        {!hayPunto ? (
          <div className="pointer-events-none absolute inset-0 flex items-end justify-center pb-6">
            <span className="text-[12px] px-3 py-1.5 rounded-full bg-slate-900/75 text-white">
              Toca el mapa para ubicar la sede
            </span>
          </div>
        ) : null}
      </div>
    </div>
  )
}

/* BotonMiUbicacion pide la posición al navegador. Existe porque es la forma
 * MÁS fiable de ubicar la sede: quien configura suele estar parado en el local.
 * Y funciona aunque los tiles no carguen. */
export function BotonMiUbicacion({ onUbicacion, disabled = false }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const pedir = () => {
    if (typeof navigator === 'undefined' || !navigator.geolocation) {
      setError('Este navegador no puede dar la ubicación.')
      return
    }
    setBusy(true); setError('')
    navigator.geolocation.getCurrentPosition(
      (p) => {
        setBusy(false)
        onUbicacion({ lat: p.coords.latitude, lon: p.coords.longitude, precisionM: p.coords.accuracy || 0 })
      },
      (e) => {
        setBusy(false)
        // Se distingue el permiso negado del fallo técnico: son dos problemas
        // distintos y la persona puede resolver el primero.
        setError(e?.code === 1
          ? 'Diste «no» al permiso de ubicación. Habilítalo en el navegador y vuelve a intentar.'
          : 'No se pudo obtener la ubicación. Puedes tocarla en el mapa.')
      },
      { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 },
    )
  }

  return (
    <div>
      <Button size="sm" variant="secondary" loading={busy} disabled={disabled}
        icon={<Icon.Globe size={15} />} onClick={pedir}>
        Usar mi ubicación actual
      </Button>
      {error ? <div className="mt-1.5 text-[12px] text-amber-700 dark:text-amber-400">{error}</div> : null}
    </div>
  )
}
