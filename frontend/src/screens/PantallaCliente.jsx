import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Logo } from '../components/Logo.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { montoEnMoneda, monedaLabel } from '../lib/precio.js'
import { IVA_TASA } from '../lib/fiscal.js'
import { resolverTema, estiloFondoGeneral } from '../lib/tema.js'
import { useData } from '../context/DataContext.jsx'

/* PANTALLA DEL CLIENTE — la segunda ventana del POS, orientada al cliente.
 *
 * Es la MISMA SPA en «modo display»: se abre desde el Modo caja con
 * window.open(...#pantalla-cliente) en la misma máquina y comparte sesión,
 * empresa y tasas (va dentro de los providers). No habla con el servidor para
 * esto: se sincroniza con la caja por BroadcastChannel (canal 'huberp-pos').
 *
 * Muestra, en grande y legible a distancia: el carrito en vivo, los totales en
 * Bs y su equivalente en las divisas activas, el vuelto durante el cobro, y una
 * pantalla de bienvenida cuando no hay venta. Alrededor, los banners
 * publicitarios que la empresa configure (BannerSuperior / BannerLateral).
 *
 * Es CONFIGURABLE por empresa (Ajustes › Pantalla del cliente) con tres modos:
 *  - solo_productos: sin publicidad; venta = carrito/totales, idle = bienvenida.
 *  - mixta: carrito + un panel de publicidad al lado en la venta, y el carrusel a
 *    pantalla grande en idle (lo de siempre). Sin slides, se comporta como solo.
 *  - publicidad: TODA la pantalla es el carrusel siempre (señalización), sin
 *    carrito. Sin slides, la bienvenida.
 * Cada slide del carrusel es de TEXTO (mensaje de marca) o de IMAGEN.
 */

// Canal de sincronización POS ↔ pantalla del cliente (misma máquina, sin servidor).
export const CANAL_POS = 'huberp-pos'

/* usePosCanal — abre el BroadcastChannel, entrega los mensajes a `onMensaje` y
 * devuelve un `publicar(msg)`. Cierra y limpia el canal al desmontar. El callback
 * se guarda en un ref para que el handler siempre vea el estado más reciente sin
 * reabrir el canal. */
export function usePosCanal(onMensaje) {
  const ref = useRef(null)
  const cbRef = useRef(onMensaje)
  cbRef.current = onMensaje
  useEffect(() => {
    if (typeof BroadcastChannel === 'undefined') return undefined
    const ch = new BroadcastChannel(CANAL_POS)
    ref.current = ch
    ch.onmessage = (e) => cbRef.current?.(e.data)
    return () => { ch.close(); ref.current = null }
  }, [])
  return useCallback((msg) => { ref.current?.postMessage(msg) }, [])
}

// Imagen servida por su ruta tal como se sirve el logo (relativa /api/archivos/…
// o data URI). Tolera undefined: no rompe si el banner aún no existe.
function Imagen({ src, className, alt = '' }) {
  const [err, setErr] = useState(false)
  useEffect(() => { setErr(false) }, [src])
  if (!src || err) return null
  return <img src={src} alt={alt} className={className} onError={() => setErr(true)} />
}

// Cabecera de marca del comercio: su logo (la versión que dicte la caja) si lo
// tiene, si no el isotipo ElERP. El acento (RIF) usa el color de marca.
function Marca({ empresa, logo, acento, size = 'lg' }) {
  const grande = size === 'lg'
  return (
    <div className="flex items-center gap-4">
      {logo ? (
        <Imagen src={logo} alt={empresa?.nombre || ''}
          className={`${grande ? 'h-20' : 'h-12'} w-auto object-contain drop-shadow`} />
      ) : (
        <div className="rounded-2xl bg-white/95 p-2.5 shadow"><Logo size={grande ? 56 : 36} /></div>
      )}
      <div className="min-w-0">
        <div className={`font-display font-bold leading-tight truncate ${grande ? 'text-4xl' : 'text-2xl'}`}>
          {empresa?.nombre || 'Bienvenido'}
        </div>
        {empresa?.rif ? <div className="num text-lg" style={{ color: acento }}>{empresa.rif}</div> : null}
      </div>
    </div>
  )
}

// Total en Bs (grande) con su equivalente en cada divisa activa (con tasa).
function TotalConDivisas({ totalBs, divisas, tasaDe, size = 'xl' }) {
  const enorme = size === 'xl'
  return (
    <div>
      <div className={`num font-extrabold leading-none ${enorme ? 'text-[clamp(2.5rem,7vw,5.5rem)]' : 'text-[clamp(1.75rem,5vw,3.25rem)]'}`}>
        {fmtCurrency(totalBs, 'VES')}
      </div>
      {divisas.length ? (
        <div className="mt-2.5 flex flex-wrap gap-x-6 gap-y-1">
          {divisas.map((c) => {
            const v = montoEnMoneda(totalBs, c, tasaDe)
            return (
              <span key={c} className="num text-teal-300/90 text-xl font-semibold">
                {v === null ? `${monedaLabel(c)} —` : fmtCurrency(v, c)}
              </span>
            )
          })}
        </div>
      ) : null}
    </div>
  )
}

export function PantallaCliente() {
  const { db, tasaDe } = useData()
  const empresa = db.EMPRESA || null

  // Estado espejado desde la caja.
  const [estado, setEstado] = useState({ carrito: [], totales: null, cliente: null })
  const [cobro, setCobro] = useState(null) // {recibido, recibidoMoneda, vuelto, total, falta, cobrado} | null
  const [venta, setVenta] = useState(null) // documento emitido (pantalla de gracias) | null
  // Tema que dicta la caja (ya resuelto: fondos por espacio, texto, énfasis y logo).
  // null = resolver desde db.EMPRESA. Ver el mensaje 'branding' del canal POS.
  const [branding, setBranding] = useState(null)

  const publicar = usePosCanal((m) => {
    if (!m || typeof m !== 'object') return
    switch (m.tipo) {
      case 'estado':
        // Un carrito en curso (o vaciado) reemplaza cualquier venta ya finalizada.
        setEstado({ carrito: m.carrito || [], totales: m.totales || null, cliente: m.cliente || null })
        setVenta(null)
        break
      case 'cobro':
        setCobro(m.activo ? m : null)
        break
      case 'emitida':
        setVenta(m.doc || null)
        setCobro(null)
        break
      case 'branding':
        // La caja emite el TEMA ya resuelto (empresa + override de la caja). Si un
        // display viejo/caja vieja no manda `tema`, se cae al fallback de db.EMPRESA.
        setBranding(m.tema ? { tema: m.tema } : null)
        break
      default:
        break
    }
  })

  // Tema resuelto: el que dicta la caja (branding) o, en su defecto, el de la
  // empresa (db.EMPRESA). Trae fondos por espacio, colores de texto/énfasis y el
  // logo de la versión elegida, todo listo para pintar.
  const tema = branding?.tema || resolverTema(empresa)
  const logo = tema.logo
  const acento = tema.enfasis
  const texto = tema.texto

  // Al abrir la pantalla (o al reconectar) se pide a la caja que reemita su
  // estado actual: así el display se pone al día aunque se abra a mitad de venta.
  useEffect(() => { publicar({ tipo: 'hello' }) }, [publicar])

  // Divisas activas con tasa (para los equivalentes del total). Cae a US$ si el
  // backend aún no expone db.TASAS, igual que el POS.
  const divisas = useMemo(() => {
    const a = db.TASAS?.activas
    const codigos = Array.isArray(a) && a.length
      ? a.map((x) => (x.codigo || '').toUpperCase()).filter((c) => c && c !== 'VES')
      : ['USD']
    return [...new Set(codigos)].filter((c) => tasaDe(c) > 0)
  }, [db.TASAS, tasaDe])

  // Modo de la pantalla del cliente (Ajustes). Vacío/desconocido = solo_productos.
  const modo = empresa?.pantallaClienteModo || 'solo_productos'
  const modoPublicidad = modo === 'publicidad'
  const modoMixta = modo === 'mixta'

  // Los slides del carrusel salen de DOS fuentes que se unifican en una sola lista
  // ordenada, ambas con la misma forma de slide (tipo/texto/subtexto/imagen):
  //  1. las PROMOCIONES del módulo (Ventas › Promociones) que estén activas y
  //     dentro de su vigencia, ordenadas por su `orden`; y
  //  2. los SLIDES MANUALES que la empresa carga en Ajustes (empresa.publicidad).
  // Las promociones van primero (la campaña vigente), luego los slides manuales.
  // Se filtran los vacíos y se tolera la forma antigua de publicidad ([]string).
  // Sin ninguno válido, el carrusel cae a la bienvenida.
  const promoSlides = useMemo(() => {
    // El carrusel de promociones pertenece al módulo Marketing: si no está activo, no
    // se muestran (el servidor ya vacía promocionesActivas; esto lo hace explícito y
    // evita el fallback a la biblioteca). Los slides manuales (branding, núcleo) siguen.
    if (!(db.MODULOS || []).includes('marketing')) return []
    // Fuente preferida: las promociones ya filtradas/ordenadas por el servidor
    // (bootstrap), disponibles bajo cualquier rol. Si no vinieran (backend viejo),
    // se cae a filtrar la biblioteca completa (db.PROMOCIONES) en el cliente.
    const hoy = new Date().toISOString().slice(0, 10)
    const activas = Array.isArray(db.PROMOCIONES_ACTIVAS) && db.PROMOCIONES_ACTIVAS.length
      ? db.PROMOCIONES_ACTIVAS
      : (Array.isArray(db.PROMOCIONES) ? db.PROMOCIONES : [])
          .filter((p) => p && p.activa
            && (!p.desde || hoy >= p.desde)
            && (!p.hasta || hoy <= p.hasta))
    return activas
      .slice()
      .sort((a, b) => (a.orden || 0) - (b.orden || 0) || String(a.nombre || '').localeCompare(String(b.nombre || '')))
      .map((p) => (p.tipo === 'imagen'
        ? { tipo: 'imagen', imagen: p.imagen }
        : { tipo: 'texto', texto: p.titulo, subtexto: p.subtexto }))
  }, [db.PROMOCIONES_ACTIVAS, db.PROMOCIONES, db.MODULOS])

  const slidesManuales = useMemo(() => {
    const lista = Array.isArray(empresa?.publicidad) ? empresa.publicidad : []
    return lista.map((s) => (typeof s === 'string' ? { tipo: 'imagen', imagen: s } : s))
  }, [empresa?.publicidad])

  const anuncios = useMemo(() => (
    [...promoSlides, ...slidesManuales].filter((s) => s && (
      (s.tipo === 'texto' && (s.texto || '').trim()) ||
      (s.tipo === 'imagen' && (s.imagen || '').trim())
    ))
  ), [promoSlides, slidesManuales])

  const carrito = estado.carrito || []
  const totales = estado.totales
  const hayCarrito = carrito.length > 0

  // Prioridad de vistas: venta emitida (gracias) → cobro → carrito → bienvenida.
  let vista = 'idle'
  if (venta) vista = 'gracias'
  else if (cobro) vista = 'cobro'
  else if (hayCarrito) vista = 'carrito'

  return (
    <div className="fixed inset-0 flex flex-col overflow-hidden"
      style={{ background: estiloFondoGeneral(tema), color: texto }}>
      {/* Banner superior (horizontal), si la empresa lo configuró. */}
      {empresa?.bannerSuperior ? (
        <div className="shrink-0 w-full bg-black/20">
          <Imagen src={empresa.bannerSuperior} alt="" className="w-full max-h-[16vh] object-cover" />
        </div>
      ) : null}

      <div className="flex-1 min-h-0 flex">
        {/* Contenido principal. El modo `publicidad` convierte TODA la pantalla en
            el carrusel siempre (señalización), ignorando la venta; si no hay slides,
            la bienvenida. Los demás modos siguen el flujo de la venta. */}
        <div className="flex-1 min-w-0 flex flex-col">
          {modoPublicidad ? (
            <VistaIdle empresa={empresa} logo={logo} acento={acento} anuncios={anuncios} />
          ) : vista === 'idle' ? (
            // En `mixta` el idle muestra el carrusel; en `solo_productos` la bienvenida.
            <VistaIdle empresa={empresa} logo={logo} acento={acento} anuncios={modoMixta ? anuncios : []} />
          ) : vista === 'gracias' ? (
            <VistaGracias empresa={empresa} acento={acento} venta={venta} divisas={divisas} tasaDe={tasaDe} />
          ) : (
            // El panel de publicidad al lado del carrito solo en modo `mixta`.
            <VistaVenta empresa={empresa} logo={logo} acento={acento} tema={tema} carrito={carrito} totales={totales} cliente={estado.cliente}
              cobro={vista === 'cobro' ? cobro : null} divisas={divisas} tasaDe={tasaDe}
              anuncios={modoMixta ? anuncios : []} />
          )}
        </div>

        {/* Banner lateral (vertical), si la empresa lo configuró. */}
        {empresa?.bannerLateral ? (
          <div className="hidden lg:block shrink-0 w-[22vw] max-w-[420px] bg-black/20">
            <Imagen src={empresa.bannerLateral} alt="" className="h-full w-full object-cover" />
          </div>
        ) : null}
      </div>
    </div>
  )
}

// Cada cuántos ms rota cada anuncio del carrusel de la pantalla del cliente.
const ROTACION_ANUNCIO_MS = 6000

// useReducedMotion sigue la preferencia del sistema «reducir movimiento» (WCAG
// 2.2). Cuando está activa, el carrusel cambia de anuncio sin animación de fundido.
function useReducedMotion() {
  const query = '(prefers-reduced-motion: reduce)'
  const [reduce, setReduce] = useState(() => typeof matchMedia !== 'undefined' && matchMedia(query).matches)
  useEffect(() => {
    if (typeof matchMedia === 'undefined') return undefined
    const mq = matchMedia(query)
    const on = () => setReduce(mq.matches)
    mq.addEventListener?.('change', on)
    return () => mq.removeEventListener?.('change', on)
  }, [])
  return reduce
}

// Estado idle (sin venta): con anuncios configurados, el carrusel rotativo; si no,
// la bienvenida de siempre. La decisión vive aquí para no romper el caso vacío.
function VistaIdle({ empresa, logo, acento, anuncios }) {
  if (!anuncios.length) return <VistaBienvenida empresa={empresa} logo={logo} acento={acento} />
  return <VistaAnuncios empresa={empresa} logo={logo} acento={acento} anuncios={anuncios} />
}

// Contenido de un slide del carrusel según su tipo. Imagen → la imagen a
// object-contain. Texto → un slide de marca (texto grande + subtexto) sobre un
// fondo navy→teal, legible a distancia, con equilibrio de líneas (text-balance) y
// tamaños con clamp que se adaptan al panel (variante `panel` de la venta mixta).
function SlideContenido({ slide, panel, acento = '#09B69B' }) {
  if (slide?.tipo === 'imagen') {
    return <Imagen src={slide.imagen} alt="" className="h-full w-full object-contain" />
  }
  return (
    <div className="h-full w-full flex flex-col items-center justify-center text-center gap-3 md:gap-5 px-6 md:px-10"
      style={{ background: `linear-gradient(150deg,#1D3477 0%,#173A78 45%,${acento} 135%)` }}>
      <div className={`font-display font-extrabold text-white text-balance leading-[1.05] ${panel ? 'text-[clamp(1.6rem,4.5vw,3rem)]' : 'text-[clamp(2.75rem,7.5vw,6.5rem)]'}`}>
        {slide.texto}
      </div>
      {slide.subtexto ? (
        <div className={`text-teal-100/95 font-semibold text-balance ${panel ? 'text-[clamp(0.95rem,2.2vw,1.4rem)]' : 'text-[clamp(1.4rem,3vw,2.5rem)]'}`}>
          {slide.subtexto}
        </div>
      ) : null}
    </div>
  )
}

// Carrusel de anuncios a pantalla grande: rota solo cada ROTACION_ANUNCIO_MS con
// un fundido suave (salvo «reducir movimiento», donde cambia sin animación). Con
// un solo anuncio, queda fijo. Cada slide es de texto o de imagen (SlideContenido).
// Sobre el slide, el nombre del comercio discreto para que se lea a distancia sin
// robarle protagonismo al anuncio. El interval se limpia al desmontar (al empezar
// una venta esta vista se desmonta) y al cambiar la lista de anuncios.
function VistaAnuncios({ empresa, logo, acento = '#09B69B', anuncios, variant = 'full' }) {
  const [i, setI] = useState(0)
  const reduce = useReducedMotion()
  const uno = anuncios.length <= 1
  const panel = variant === 'panel'

  // Si cambia la cantidad de anuncios, vuelve al primero (evita índice fuera de rango).
  useEffect(() => { setI(0) }, [anuncios.length])

  useEffect(() => {
    if (uno) return undefined
    const id = setInterval(() => setI((n) => (n + 1) % anuncios.length), ROTACION_ANUNCIO_MS)
    return () => clearInterval(id)
  }, [uno, anuncios.length])

  return (
    <div className={`relative overflow-hidden ${panel ? 'h-full rounded-3xl border border-white/10 bg-black/20' : 'flex-1 min-h-0'}`}>
      {anuncios.map((slide, k) => (
        <div key={k} aria-hidden={k === i ? undefined : true}
          className={`absolute inset-0 flex items-center justify-center ${reduce ? '' : 'transition-opacity duration-700 ease-in-out'} ${k === i ? 'opacity-100' : 'opacity-0'}`}>
          <SlideContenido slide={slide} panel={panel} acento={acento} />
        </div>
      ))}

      {/* Nombre del comercio, discreto, sobre el anuncio (solo a pantalla completa). */}
      {!panel && (empresa?.nombre || logo) ? (
        <div className="absolute left-0 bottom-0 p-5 md:p-6 pointer-events-none">
          <div className="inline-flex items-center gap-3 rounded-2xl bg-black/35 backdrop-blur px-4 py-2 shadow-lg">
            {logo ? <Imagen src={logo} alt="" className="h-8 w-auto object-contain" /> : null}
            {empresa?.nombre ? <span className="text-white/90 text-lg md:text-xl font-semibold">{empresa.nombre}</span> : null}
          </div>
        </div>
      ) : null}

      {/* Puntitos de posición cuando hay varios anuncios. */}
      {!uno ? (
        <div className="absolute bottom-5 md:bottom-6 left-1/2 -translate-x-1/2 flex items-center gap-2">
          {anuncios.map((_, k) => (
            <span key={k} className={`h-2 rounded-full transition-all ${k === i ? 'w-6' : 'w-2 bg-white/40'}`}
              style={k === i ? { backgroundColor: acento } : undefined} />
          ))}
        </div>
      ) : null}
    </div>
  )
}

// Estado idle: bienvenida con el logo / nombre del comercio.
function VistaBienvenida({ empresa, logo, acento = '#09B69B' }) {
  return (
    <div className="flex-1 flex flex-col items-center justify-center text-center px-10 gap-8">
      {logo ? (
        <Imagen src={logo} alt={empresa?.nombre || ''} className="max-h-[34vh] w-auto object-contain drop-shadow-lg" />
      ) : (
        <div className="rounded-3xl bg-white/95 p-8 shadow-xl"><Logo size={140} /></div>
      )}
      <div>
        <div className="font-display font-bold text-[clamp(2.5rem,6vw,5rem)] leading-tight">
          {empresa?.nombre || 'Bienvenido'}
        </div>
        <div className="mt-3 text-2xl font-medium" style={{ color: acento }}>Le damos la bienvenida</div>
      </div>
      <div className="text-white/50 text-lg">Su compra aparecerá aquí</div>
    </div>
  )
}

// Venta en curso: carrito + totales. Con `cobro` presente, el panel de totales
// se convierte en el panel de cobro (recibido + vuelto).
function VistaVenta({ empresa, logo, acento, tema = {}, carrito, totales, cliente, cobro, divisas, tasaDe, anuncios = [] }) {
  // Fondos POR ESPACIO: si el espacio tiene color propio, se pinta sólido; si no,
  // queda '' y el espacio conserva su superficie translúcida sobre el fondo general.
  const bgCabecera = tema.fondoCabecera ? { backgroundColor: tema.fondoCabecera } : undefined
  const bgProductos = tema.fondoProductos ? { backgroundColor: tema.fondoProductos } : undefined
  const bgPublicidad = tema.fondoPublicidad ? { backgroundColor: tema.fondoPublicidad } : undefined
  return (
    <div className="flex-1 min-h-0 flex flex-col p-6 gap-4">
      {/* Cabecera compacta con la marca */}
      <div className={`shrink-0 flex items-center justify-between gap-4 ${bgCabecera ? 'rounded-2xl px-4 py-3' : ''}`} style={bgCabecera}>
        <Marca empresa={empresa} logo={logo} acento={acento} size="sm" />
        {cliente?.nombre ? (
          <div className="text-right">
            <div className="text-white/50 text-sm uppercase tracking-wider">Cliente</div>
            <div className="text-white text-xl font-semibold truncate max-w-[40vw]">{cliente.nombre}</div>
          </div>
        ) : null}
      </div>

      {/* Fila media: área de publicidad (izquierda) + carrito (derecha). Los
          totales van abajo, a todo el ancho. La publicidad solo ocupa lugar si la
          empresa configuró anuncios (Ajustes › Pantalla del cliente). */}
      <div className="flex-1 min-h-0 flex gap-4">
        {anuncios.length ? (
          <div className="hidden lg:block w-[34%] max-w-[560px] shrink-0 min-h-0 rounded-3xl overflow-hidden" style={bgPublicidad}>
            <VistaAnuncios empresa={empresa} logo={logo} acento={acento} anuncios={anuncios} variant="panel" />
          </div>
        ) : null}
        {/* Carrito en vivo */}
        <div className="flex-1 min-w-0 min-h-0 rounded-3xl bg-white/[0.06] border border-white/10 overflow-hidden flex flex-col" style={bgProductos}>
        <div className="shrink-0 grid grid-cols-[1fr_auto_auto] gap-5 px-6 py-2.5 border-b border-white/10 text-teal-300/80 text-[13px] font-semibold uppercase tracking-wider">
          <span>Producto</span>
          <span className="text-right w-28">Cant. × Precio</span>
          <span className="text-right w-36">Importe</span>
        </div>
        <div className="flex-1 min-h-0 overflow-y-auto divide-y divide-white/5">
          {carrito.map((l, i) => {
            // Producto por peso: la cantidad es el peso en kg (3 decimales) y el
            // precio es por kg (/kg), para que el cliente lo entienda. Por unidad,
            // el formato de siempre. La caja envía tipoVenta/unidad en cada línea.
            const peso = l.tipoVenta === 'peso'
            return (
            <div key={l.sku || i} className="grid grid-cols-[1fr_auto_auto] gap-5 px-6 py-2.5 items-center">
              <span className="min-w-0">
                <span className="block text-lg font-semibold truncate">{l.nombre}</span>
                {l.exento ? <span className="text-amber-300/80 text-xs font-medium">Exento de IVA</span> : null}
              </span>
              <span className="text-right w-28 num text-white/70 text-[15px]">
                {peso
                  ? <>{fmtNum(l.cantidad, 3)} {l.unidad || 'kg'} × {fmtNum(l.precioUnitario, 2)}/{l.unidad || 'kg'}</>
                  : <>{fmtNum(l.cantidad, l.cantidad % 1 ? 2 : 0)} × {fmtNum(l.precioUnitario, 2)}</>}
              </span>
              <span className="text-right w-36 num text-white text-lg font-bold">
                {fmtCurrency((Number(l.cantidad) || 0) * (Number(l.precioUnitario) || 0), 'VES')}
              </span>
            </div>
            )
          })}
        </div>
        </div>
      </div>

      {/* Panel inferior: totales (o cobro, si está activo) */}
      {cobro ? (
        <PanelCobro cobro={cobro} divisas={divisas} tasaDe={tasaDe} acento={acento} bg={tema.fondoTotales} />
      ) : (
        <PanelTotales totales={totales} divisas={divisas} tasaDe={tasaDe} acento={acento} bg={tema.fondoTotales} />
      )}
    </div>
  )
}

// Panel de totales: subtotal, IVA y TOTAL grande en Bs + divisas.
function PanelTotales({ totales, divisas, tasaDe, acento = '#09B69B', bg = '' }) {
  if (!totales) return null
  const subtotal = totales.subtotal ?? ((totales.baseImponible || 0) + (totales.baseExenta || 0))
  return (
    <div className="shrink-0 rounded-3xl bg-black/25 border border-white/10 p-5" style={bg ? { backgroundColor: bg } : undefined}>
      <div className="flex flex-wrap items-end justify-between gap-6">
        <div className="space-y-1 text-white/70 text-lg">
          <div className="flex justify-between gap-10 min-w-[260px]">
            <span>Subtotal</span><span className="num">{fmtCurrency(subtotal, 'VES')}</span>
          </div>
          {totales.baseExenta > 0 ? (
            <div className="flex justify-between gap-10">
              <span>Exento de IVA</span><span className="num">{fmtCurrency(totales.baseExenta, 'VES')}</span>
            </div>
          ) : null}
          <div className="flex justify-between gap-10">
            <span>IVA {IVA_TASA * 100}%</span><span className="num">{fmtCurrency(totales.iva, 'VES')}</span>
          </div>
        </div>
        <div className="text-right">
          <div className="text-xl font-bold uppercase tracking-wider mb-1" style={{ color: acento }}>Total a pagar</div>
          <TotalConDivisas totalBs={totales.total} divisas={divisas} tasaDe={tasaDe} />
        </div>
      </div>
    </div>
  )
}

// Panel de cobro: total, lo recibido y el VUELTO (grande, en verde).
function PanelCobro({ cobro, divisas, tasaDe, acento = '#09B69B', bg = '' }) {
  const falta = Number(cobro.falta) || 0
  const pendiente = falta > 0.005
  const vuelto = cobro.vuelto && (Number(cobro.vuelto.monto) || 0) > 0 ? cobro.vuelto : null
  return (
    <div className="shrink-0 rounded-3xl bg-black/25 border border-white/10 p-7" style={bg ? { backgroundColor: bg } : undefined}>
      <div className="grid md:grid-cols-2 gap-6 items-center">
        {/* Total del documento + recibido */}
        <div className="space-y-3">
          <div>
            <div className="text-xl font-bold uppercase tracking-wider" style={{ color: acento }}>Total a pagar</div>
            <TotalConDivisas totalBs={cobro.total} divisas={divisas} tasaDe={tasaDe} size="lg" />
          </div>
          {Number(cobro.recibido) > 0 ? (
            <div className="text-white/70 text-xl">
              Recibido: <span className="num text-white font-semibold">
                {fmtCurrency(cobro.recibido, cobro.recibidoMoneda || 'VES')}
              </span>
            </div>
          ) : null}
          {/* IGTF del pago en divisas (3%): el cliente ve cuánto se le cobra. */}
          {Number(cobro.igtf) > 0.004 ? (
            <div className="inline-flex items-center gap-2 rounded-xl bg-amber-400/10 border border-amber-300/40 px-4 py-2 text-amber-300">
              <span className="text-base font-semibold uppercase tracking-wider">IGTF 3%</span>
              <span className="num text-xl font-bold">{fmtCurrency(cobro.igtf, 'VES')}</span>
            </div>
          ) : null}
        </div>

        {/* Vuelto o lo que falta */}
        <div className="md:text-right">
          {vuelto ? (
            <div className="rounded-2xl bg-emerald-500/15 border border-emerald-400/40 p-6">
              <div className="text-emerald-300 text-2xl font-bold uppercase tracking-wider mb-1 inline-flex items-center gap-2 md:justify-end">
                <Icon.Banknote size={26} /> Su vuelto
              </div>
              <div className="num font-extrabold text-emerald-300 leading-none text-[clamp(2.5rem,7vw,5rem)]">
                {fmtCurrency(vuelto.monto, vuelto.moneda)}
              </div>
              {/* Desglose del vuelto MIXTO: cada parte con su moneda y su medio. */}
              {Array.isArray(vuelto.partes) && vuelto.partes.length > 1 ? (
                <div className="mt-2 flex flex-wrap gap-2 md:justify-end">
                  {vuelto.partes.map((p, i) => (
                    <span key={i} className="num text-emerald-200/90 text-base md:text-lg font-semibold rounded-lg bg-emerald-500/10 border border-emerald-400/30 px-3 py-1">
                      {fmtCurrency(p.monto, p.moneda || 'VES')} · {p.metodo === 'pago_movil' ? 'pago móvil' : 'efectivo'}
                    </span>
                  ))}
                </div>
              ) : null}
            </div>
          ) : pendiente ? (
            <div className="rounded-2xl bg-amber-400/10 border border-amber-300/40 p-6">
              <div className="text-amber-300 text-2xl font-bold uppercase tracking-wider mb-1">Falta por pagar</div>
              <div className="num font-extrabold text-amber-300 leading-none text-[clamp(2.5rem,7vw,5rem)]">
                {fmtCurrency(falta, 'VES')}
              </div>
            </div>
          ) : (
            <div className="rounded-2xl bg-emerald-500/15 border border-emerald-400/40 p-6 inline-flex items-center gap-3">
              <Icon.CircleCheck size={40} className="text-emerald-300" />
              <div className="text-emerald-300 text-3xl font-bold">Pago completo</div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// Pantalla de gracias tras emitir la factura: total cobrado y, si hay, el vuelto.
function VistaGracias({ empresa, acento = '#09B69B', venta, divisas, tasaDe }) {
  const vuelto = Number(venta?.vuelto) || 0
  return (
    <div className="flex-1 flex flex-col items-center justify-center text-center px-10 gap-7">
      <div className="h-24 w-24 rounded-full bg-emerald-500/20 border border-emerald-400/40 inline-flex items-center justify-center">
        <Icon.CircleCheck size={56} className="text-emerald-300" />
      </div>
      <div>
        <div className="font-display font-bold text-[clamp(2.5rem,6vw,5rem)] leading-tight">¡Gracias por su compra!</div>
        {empresa?.nombre ? <div className="mt-2 text-2xl font-medium" style={{ color: acento }}>{empresa.nombre}</div> : null}
      </div>

      <div className="rounded-3xl bg-white/[0.06] border border-white/10 px-10 py-7">
        <div className="text-lg uppercase tracking-wider" style={{ color: acento }}>Total pagado</div>
        <div className="mt-1"><TotalConDivisas totalBs={venta?.total || 0} divisas={divisas} tasaDe={tasaDe} size="lg" /></div>
      </div>

      {vuelto > 0.004 ? (
        <div className="rounded-2xl bg-emerald-500/15 border border-emerald-400/40 px-8 py-5">
          <div className="text-emerald-300 text-xl font-bold uppercase tracking-wider">Su vuelto</div>
          <div className="num font-extrabold text-emerald-300 text-[clamp(2rem,5vw,3.5rem)] leading-none">
            {fmtCurrency(vuelto, venta?.vueltoMoneda || 'VES')}
          </div>
          {/* Desglose del vuelto MIXTO: cada parte con su moneda y su medio. */}
          {Array.isArray(venta?.vueltoPartes) && venta.vueltoPartes.length > 1 ? (
            <div className="mt-2 flex flex-wrap justify-center gap-2">
              {venta.vueltoPartes.map((p, i) => (
                <span key={i} className="num text-emerald-200/90 text-base md:text-lg font-semibold rounded-lg bg-emerald-500/10 border border-emerald-400/30 px-3 py-1">
                  {fmtCurrency(p.monto, p.moneda || 'VES')} · {p.metodo === 'pago_movil' ? 'pago móvil' : 'efectivo'}
                </span>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
