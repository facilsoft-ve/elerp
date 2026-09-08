import { estiloFondoGeneral } from '../lib/tema.js'

/* MaquetaPantallaCliente — una MAQUETA A ESCALA de la segunda pantalla del POS
 * para la vista previa en vivo del editor de tema (Configuración › Marketing).
 *
 * No es la pantalla real (esa es `fixed inset-0` y depende del canal POS): es una
 * réplica ligera y dedicada que refleja en tiempo real el tema elegido —fondo
 * general y por espacio (cabecera / productos / totales / publicidad), color de
 * texto y de énfasis, y el logo de la versión seleccionada—. Así el usuario ve el
 * efecto de cada color/logo mientras lo cambia, sin abrir la caja.
 *
 * Props: `tema` (YA resuelto por lib/tema.resolverTema) y `empresa` (para el
 * nombre/RIF de la cabecera). */
export function MaquetaPantallaCliente({ tema, empresa }) {
  const t = tema || {}
  // Fondo por espacio: color propio si se configuró; si no, superficie translúcida
  // sobre el fondo general (igual que la pantalla real).
  const bgCabecera = t.fondoCabecera ? { backgroundColor: t.fondoCabecera } : undefined
  const bgProductos = t.fondoProductos ? { backgroundColor: t.fondoProductos } : undefined
  const bgTotales = t.fondoTotales ? { backgroundColor: t.fondoTotales } : undefined
  const bgPublicidad = t.fondoPublicidad ? { backgroundColor: t.fondoPublicidad } : undefined
  const enfasis = t.enfasis || '#6A2CF0'

  const lineas = [
    { n: 'Harina de maíz', c: '2 × 45,00', imp: 'Bs 90,00' },
    { n: 'Café La Cima 250g', c: '1 × 120,00', imp: 'Bs 120,00' },
    { n: 'Leche completa 1L', c: '3 × 38,50', imp: 'Bs 115,50' },
  ]

  return (
    <div className="w-full rounded-xl overflow-hidden border border-slate-200 dark:border-slate-700 shadow-sm select-none">
      <div className="aspect-[16/10] w-full flex flex-col p-3 gap-2 text-[9px] leading-tight"
        style={{ background: estiloFondoGeneral(t), color: t.texto || '#ffffff' }}>
        {/* Cabecera: logo + nombre + RIF */}
        <div className={`shrink-0 flex items-center gap-2 ${bgCabecera ? 'rounded-lg px-2 py-1.5' : ''}`} style={bgCabecera}>
          {t.logo ? (
            <img src={t.logo} alt="" className="h-6 w-auto object-contain"
              onError={(e) => { e.currentTarget.style.display = 'none' }} />
          ) : (
            <div className="h-6 w-6 rounded bg-white/90 shrink-0" />
          )}
          <div className="min-w-0">
            <div className="font-bold text-[12px] truncate" style={{ fontFamily: 'Poppins, Inter, sans-serif' }}>
              {empresa?.nombre || 'Tu comercio'}
            </div>
            <div className="text-[9px] font-semibold num" style={{ color: enfasis }}>{empresa?.rif || 'J-12345678-9'}</div>
          </div>
        </div>

        {/* Fila media: publicidad (izq) + carrito (der) */}
        <div className="flex-1 min-h-0 flex gap-2">
          <div className="w-[34%] rounded-lg border border-white/10 flex items-center justify-center text-center px-2"
            style={bgPublicidad || { backgroundColor: 'rgba(0,0,0,0.2)' }}>
            <div>
              <div className="font-extrabold text-[13px]" style={{ fontFamily: 'Poppins, Inter, sans-serif' }}>2x1 en bebidas</div>
              <div className="text-[8px] mt-0.5" style={{ color: enfasis }}>Solo esta semana</div>
            </div>
          </div>
          <div className="flex-1 rounded-lg border border-white/10 overflow-hidden flex flex-col"
            style={bgProductos || { backgroundColor: 'rgba(255,255,255,0.06)' }}>
            <div className="shrink-0 flex justify-between px-2 py-1 border-b border-white/10 uppercase tracking-wide font-semibold text-[7px]"
              style={{ color: enfasis }}>
              <span>Producto</span><span>Importe</span>
            </div>
            <div className="flex-1 divide-y divide-white/5">
              {lineas.map((l, i) => (
                <div key={i} className="flex items-center justify-between px-2 py-1">
                  <span className="min-w-0 truncate font-semibold">{l.n}</span>
                  <span className="num opacity-90 pl-2 shrink-0">{l.imp}</span>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Totales */}
        <div className="shrink-0 rounded-lg border border-white/10 px-2.5 py-1.5 flex items-end justify-between"
          style={bgTotales || { backgroundColor: 'rgba(0,0,0,0.25)' }}>
          <div className="space-y-0.5 opacity-80">
            <div className="flex justify-between gap-6"><span>Subtotal</span><span className="num">Bs 325,50</span></div>
            <div className="flex justify-between gap-6"><span>IVA 16%</span><span className="num">Bs 52,08</span></div>
          </div>
          <div className="text-right">
            <div className="uppercase tracking-wider font-bold text-[7px]" style={{ color: enfasis }}>Total a pagar</div>
            <div className="num font-extrabold text-[16px] leading-none">Bs 377,58</div>
          </div>
        </div>
      </div>
    </div>
  )
}
