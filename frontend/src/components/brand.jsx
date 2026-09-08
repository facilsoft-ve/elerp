/* ============================================================================
 * GRÁFICOS COMPLEMENTARIOS ElERP
 *
 * Implementación literal de:
 *   · Documentos/"Branding y gráficos complementarios UI ElERP.pdf" §02, §04, §05
 *   · Documentos/"Manual de marca ElERP.pdf" §08 (gráficos modulares)
 *
 * Reglas de oro que estos componentes hacen cumplir:
 *   1. El azul estructura, el verde señala. Si una pantalla tiene más verde que
 *      azul, está mal balanceada.
 *   2. UN SOLO punto hub protagonista por vista.
 *   3. La esquina modular va en UNA sola esquina por componente, nunca en cuatro.
 *   4. Un solo círculo verde por composición modular.
 *   5. La textura modular no cubre más del 25% de una pieza.
 *   6. El degradado azul→verde solo existe en la barra de progreso.
 * ========================================================================== */
import { Icon } from './Icon.jsx'

/* ---------------------------------------------------------------------------
 * 1 · EL PUNTO HUB
 * El punto verde del logo es el gráfico complementario principal: marcador de
 * estado, viñeta de lista destacada o remate de título.
 * ------------------------------------------------------------------------- */

// `ring` añade el halo blanco que separa el punto de la superficie que lo aloja
// (así aparece sobre el cuadro de icono de módulo y sobre la loseta activa).
export const HubDot = ({ size = 8, ring = false, className = '', style }) => (
  <span
    className={`inline-block shrink-0 rounded-full bg-teal-500 ${className}`}
    style={{
      width: size,
      height: size,
      boxShadow: ring ? '0 0 0 2px #FFFFFF' : undefined,
      ...style,
    }}
  />
)

// Contraparte del punto hub para lo inactivo: cuadrito gris redondeado. Es el
// marcador de los ítems no activos del menú y de las listas de módulos.
// `tone="onDark"` para superficies azules de marca (el sidebar).
export const IdleMarker = ({ size = 8, tone = 'light', className = '' }) => (
  <span
    className={`inline-block shrink-0 rounded-[3px] ${tone === 'onDark' ? 'bg-white/30' : 'bg-slate-300 dark:bg-slate-600'} ${className}`}
    style={{ width: size, height: size }}
  />
)

// Remate de título: el punto cierra el titular, como en el logo. Usar UNA vez
// por vista — es el punto hub protagonista.
export const TitleHub = ({ children, as: Tag = 'h1', size = 'xl', className = '' }) => {
  const sizes = {
    lg: 'text-[19px]',   // H2 del manual: Poppins SemiBold 19
    xl: 'text-[22px]',
    '2xl': 'text-[28px]', // H1 del manual: Poppins Bold 28
  }
  return (
    <Tag className={`font-display font-bold tracking-tight text-elerp-500 dark:text-white inline-flex items-center gap-2 ${sizes[size] || sizes.xl} ${className}`}>
      <span>{children}</span>
      <HubDot size={size === '2xl' ? 9 : 7} className="mt-1.5" />
    </Tag>
  )
}

// Separador decorativo: barra azul + punto hub + barra gris.
// `tone="onDark"` para superficies azules de marca — sobre azul la barra navy
// sería invisible, así que pasa a blanco translúcido.
export const HubSeparator = ({ tone = 'light', className = '' }) => (
  <div className={`inline-flex items-center gap-1.5 ${className}`} aria-hidden="true">
    <span className={`h-1.5 w-7 rounded-full ${tone === 'onDark' ? 'bg-white/70' : 'bg-elerp-500'}`} />
    <HubDot size={7} />
    <span className={`h-1.5 w-4 rounded-full ${tone === 'onDark' ? 'bg-white/25' : 'bg-slate-200 dark:bg-slate-700'}`} />
  </div>
)

/* ---------------------------------------------------------------------------
 * 2 · LA ESQUINA MODULAR
 * Racimo de 2–4 cuadros redondeados en tintes de azul más un punto verde,
 * anclados a UNA esquina. Para heros, banners y cards destacadas.
 *   variant "full" → 4 módulos + hub grande (panel hero, sobre azul)
 *   variant "soft" → 2 módulos + hub pequeño (card clara, sobre claro)
 * Los módulos sangran fuera del viewBox: así quedan "cortados" por el borde,
 * como en el handoff.
 * ------------------------------------------------------------------------- */
/* RACIMO MODULAR — medidas tomadas del prototipo (login), pieza por pieza.
 * En el prototipo son divs absolutos dentro de un contenedor de 448×150:
 *
 *   Racimo grande (4 piezas)        Racimo chico (3 piezas)
 *   46×46 r12 blanco 12%  (0, 10)   38×38 r11 blanco 10%  (0, 0)
 *   64×64 r16 blanco  9%  (40, 44)  52×52 r14 blanco 13%  (22, 50)
 *   30×30 r9  blanco 16%  (96, 0)   punto verde 9         (1, 115)
 *   punto verde 16       (118, 56)
 *
 * El radio es ≈27% del lado (coherente con el isotipo) y las opacidades caen
 * entre 9% y 16%. Un solo punto verde por racimo: es el hub.
 *
 * Regla de aplicación (03 §1.4): SOLO como textura en superficies de marca —
 * login, cabeceras de Super Admin, portadas y paneles de bienvenida. Nunca
 * sobre áreas de trabajo con datos.
 */
const RACIMOS = {
  grande: {
    w: 142, h: 110,
    piezas: [
      { x: 0, y: 10, s: 46, r: 12, o: 0.12 },
      { x: 40, y: 44, s: 64, r: 16, o: 0.09 },
      { x: 96, y: 0, s: 30, r: 9, o: 0.16 },
    ],
    hub: { x: 118, y: 56, s: 16 },
  },
  chico: {
    w: 90, h: 124,
    piezas: [
      { x: 0, y: 0, s: 38, r: 11, o: 0.10 },
      { x: 22, y: 50, s: 52, r: 14, o: 0.13 },
    ],
    hub: { x: 1, y: 115, s: 9 },
  },
}

// Tintes sobre claro: los mismos pesos relativos, en azul de marca.
const TINTE_CLARO = ['#D4DCEE', '#E9EDF6', '#C7D2E9']

export const RacimoModular = ({ variante = 'grande', tone = 'onDark', escala = 1, className = '', style }) => {
  const r = RACIMOS[variante] || RACIMOS.grande
  return (
    <div className={`pointer-events-none select-none ${className}`} aria-hidden="true"
      style={{ position: 'absolute', width: r.w * escala, height: r.h * escala, ...style }}>
      {r.piezas.map((p, i) => (
        <span key={i} style={{
          position: 'absolute',
          left: p.x * escala, top: p.y * escala,
          width: p.s * escala, height: p.s * escala,
          borderRadius: p.r * escala,
          background: tone === 'onDark' ? `rgba(255,255,255,${p.o})` : TINTE_CLARO[i % TINTE_CLARO.length],
        }} />
      ))}
      <span style={{
        position: 'absolute',
        left: r.hub.x * escala, top: r.hub.y * escala,
        width: r.hub.s * escala, height: r.hub.s * escala,
        borderRadius: '50%', background: '#6A2CF0',
      }} />
    </div>
  )
}

const CORNER_ANCHOR = {
  tr: 'top-0 right-0',
  tl: 'top-0 left-0',
  br: 'bottom-0 right-0',
  bl: 'bottom-0 left-0',
}

export const ModularCorner = ({ corner = 'tr', variant = 'full', tone = 'dark', width, className = '' }) => {
  // Sobre azul: módulos blancos translúcidos (10–18%). Sobre claro: tintes de azul.
  const tints = tone === 'dark'
    ? ['rgba(255,255,255,.18)', 'rgba(255,255,255,.13)', 'rgba(255,255,255,.10)']
    : ['#D4DCEE', '#E9EDF6', '#E9EDF6']
  const hub = '#6A2CF0'

  // El racimo "full" está alineado a la derecha; el "soft", a la izquierda.
  const flipX = corner.endsWith('l') && variant === 'full'
  const flipY = corner.startsWith('b')
  const transform = `scale(${flipX ? -1 : 1}, ${flipY ? -1 : 1})`

  const w = width || (variant === 'full' ? 90 : 64)
  const h = variant === 'full' ? 56 : 26

  return (
    <svg
      className={`pointer-events-none absolute ${CORNER_ANCHOR[corner] || CORNER_ANCHOR.tr} ${className}`}
      width={w} height={(w / (variant === 'full' ? 90 : 64)) * h}
      viewBox={`0 0 ${variant === 'full' ? 90 : 64} ${h}`}
      style={{ transform }} aria-hidden="true"
    >
      {variant === 'full' ? (
        <>
          <rect x="16" y="-6" width="22" height="22" rx="7" fill={tints[2]} />
          <rect x="42" y="-6" width="22" height="22" rx="7" fill={tints[1]} />
          <rect x="68" y="-6" width="22" height="22" rx="7" fill={tints[0]} />
          <rect x="68" y="20" width="22" height="22" rx="7" fill={tints[1]} />
          <circle cx="52" cy="31" r="13" fill={hub} />
        </>
      ) : (
        <>
          <rect x="2" y="-4" width="18" height="18" rx="6" fill={tints[1]} />
          <rect x="26" y="-6" width="24" height="24" rx="7" fill={tints[0]} />
          <circle cx="58" cy="12" r="5" fill={hub} />
        </>
      )}
    </svg>
  )
}

/* ---------------------------------------------------------------------------
 * 3 · BANDA MODULAR (Manual §08)
 * Retícula de módulos redondeados para cabeceras y cierres de página. El
 * círculo verde aparece UNA sola vez por composición — es el hub.
 * Patrón determinista (sin aleatoriedad) para que no cambie entre renders.
 * ------------------------------------------------------------------------- */
export const ModularBand = ({ height = 44, tone = 'light', className = '' }) => {
  const tints = tone === 'dark'
    ? ['rgba(255,255,255,.20)', 'rgba(255,255,255,.13)', 'rgba(255,255,255,.08)']
    : ['#D4DCEE', '#E9EDF6', '#EDF0F7']
  // Retícula a escala 1:1 en píxeles: el SVG conserva su tamaño intrínseco y el
  // contenedor recorta el excedente. Así los módulos nunca se deforman ni se
  // escalan al ancho de la pantalla (el error de usar preserveAspectRatio slice).
  const step = 22, mod = 16
  const cols = 80
  const rows = Math.ceil(height / step) + 1
  const cells = []
  for (let c = 0; c < cols; c++) {
    for (let r = 0; r < rows; r++) {
      // Máscara regular pero variada; deja huecos para no pasar del 25% de tinta.
      if ((c * 3 + r * 7) % 5 >= 3) continue
      cells.push(
        <rect key={`${c}-${r}`} x={c * step} y={r * step - 4} width={mod} height={mod} rx="5"
          fill={tints[(c + r * 2) % 3]} />,
      )
    }
  }
  return (
    <div className={`overflow-hidden ${className}`} style={{ height }} aria-hidden="true">
      <svg width={cols * step} height={height} style={{ display: 'block' }}>
        {cells}
        {/* Un solo círculo verde: el hub. */}
        <circle cx={9 * step + mod / 2} cy={step + mod / 2 - 4} r={mod / 2} fill="#6A2CF0" />
      </svg>
    </div>
  )
}

/* ---------------------------------------------------------------------------
 * 4 · RACIMO MODULAR PARA ESTADOS VACÍOS
 * El handoff ilustra el estado vacío con el racimo modular (no con un icono).
 * ------------------------------------------------------------------------- */
export const ModularCluster = ({ width = 72, className = '' }) => (
  <svg className={className} width={width} height={(width / 72) * 32} viewBox="0 0 72 32" aria-hidden="true">
    <rect x="0" y="10" width="18" height="18" rx="6" className="fill-slate-200 dark:fill-slate-700" />
    <rect x="24" y="2" width="26" height="26" rx="8" className="fill-elerp-100 dark:fill-elerp-800" />
    <circle cx="60" cy="22" r="5" fill="#6A2CF0" />
  </svg>
)

/* ---------------------------------------------------------------------------
 * 5 · ICONOS DE MÓDULO (handoff §05)
 * Glifo de línea azul (trazo 1.8, retícula 24×24) dentro de un cuadro
 * redondeado azul suave. Todos comparten el mismo estilo — la identidad del
 * módulo está en el glifo, NUNCA en un color propio. El punto hub verde
 * aparece solo en el módulo activo o con notificaciones.
 * ------------------------------------------------------------------------- */
export const ModuleIcon = ({ glyph: Glyph, size = 40, hub = false, invert = false, className = '' }) => (
  <span
    className={`relative inline-flex items-center justify-center shrink-0 ${invert
      ? 'bg-elerp-500 text-white'
      : 'bg-elerp-50 text-elerp-500 dark:bg-elerp-900/50 dark:text-elerp-200'} ${className}`}
    style={{ width: size, height: size, borderRadius: Math.max(8, Math.round(size * 0.275)) }}
  >
    <Glyph size={Math.round(size * 0.6)} stroke={1.8} />
    {hub ? <HubDot size={Math.max(7, Math.round(size * 0.22))} ring className="absolute -top-1 -right-1" /> : null}
  </span>
)

// Familia completa de módulos: etiqueta oficial + glifo + ruta de la app.
// Etiqueta siempre en IBM Plex Sans (la pone <ModuleShortcut>).
export const MODULOS = [
  { id: 'facturacion', label: 'Facturación', glyph: Icon.ModFiscal },
  { id: 'pos', label: 'Punto de Venta', glyph: Icon.Cart },
  { id: 'ventas', label: 'Ventas', glyph: Icon.Users },
  { id: 'inventario', label: 'Inventario', glyph: Icon.ModInventario },
  { id: 'compras', label: 'Compras', glyph: Icon.Cart },
  { id: 'contabilidad', label: 'Contabilidad', glyph: Icon.ModContabilidad },
  { id: 'finanzas', label: 'Finanzas', glyph: Icon.ModTesoreria },
  { id: 'reportes', label: 'Reportes y BI', glyph: Icon.ModReportes },
  { id: 'config', label: 'Configuración', glyph: Icon.ModConfig },
]

export const moduloPorId = (id) => MODULOS.find((m) => m.id === id)

// Acceso directo a módulo: cuadro de icono + etiqueta debajo.
export const ModuleShortcut = ({ modulo, active = false, hub = false, onClick, size = 40 }) => (
  <button type="button" onClick={onClick} title={modulo.label}
    className="group flex flex-col items-center gap-2 w-[92px] ring-focus rounded-lg p-1">
    <ModuleIcon glyph={modulo.glyph} size={size} hub={hub || active}
      className="transition-transform group-hover:-translate-y-0.5" />
    <span className={`text-[12px] leading-tight text-center ${active
      ? 'text-elerp-500 dark:text-elerp-200 font-semibold'
      : 'text-slate-500 dark:text-slate-400'}`}>
      {modulo.label}
    </span>
  </button>
)

/* ---------------------------------------------------------------------------
 * 6 · LOSETAS TÁCTILES (handoff §05 — POS / alto tráfico)
 * Mínimo 120×120 px, glifo 40px, etiqueta 14px, separación 16px entre losetas
 * (la pone la retícula del contenedor). Toda la loseta es el área de toque.
 * Radio 16px. La loseta activa invierte a azul con glifo blanco y punto hub.
 * ------------------------------------------------------------------------- */
export const ModuleTile = ({ glyph: Glyph, label, state = 'idle', onClick, className = '', ...rest }) => {
  const states = {
    idle: 'bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 text-elerp-500 dark:text-slate-100 hover:border-elerp-300 hover:shadow-brand',
    active: 'bg-elerp-500 text-white border border-elerp-500 hover:bg-elerp-600',
    create: 'bg-teal-50 dark:bg-teal-500/15 text-teal-600 dark:text-teal-300 border border-transparent hover:bg-teal-100 dark:hover:bg-teal-500/25',
  }
  return (
    <button type="button" onClick={onClick} aria-pressed={state === 'active'}
      className={`relative rounded-tile min-w-[120px] min-h-[120px] p-4 flex flex-col items-center justify-center gap-3
        transition-all ring-focus ${states[state] || states.idle} ${className}`} {...rest}>
      <Glyph size={40} stroke={1.8} />
      <span className="font-display font-medium text-sm leading-none">{label}</span>
      {state === 'active' ? <HubDot size={12} className="absolute top-3 right-3" /> : null}
    </button>
  )
}

/* ---------------------------------------------------------------------------
 * 7 · BARRA DE PROGRESO (handoff §04)
 * Degradado azul → verde: "llegar al hub". Es el único degradado permitido.
 * ------------------------------------------------------------------------- */
export const Progress = ({ value = 0, showValue = true, height = 8, label, className = '' }) => {
  const pct = Math.max(0, Math.min(100, Math.round(value)))
  return (
    <div className={className}>
      {label ? <div className="text-[12px] text-slate-500 mb-1.5">{label}</div> : null}
      <div className="flex items-center gap-3">
        <div className="flex-1 rounded-full bg-slate-200 dark:bg-slate-700 overflow-hidden" style={{ height }}
          role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={label}>
          <div className="hb-progress-fill h-full rounded-full transition-[width] duration-500"
            style={{ width: `${pct}%` }} />
        </div>
        {showValue ? (
          <span className="font-display font-bold text-[13px] text-elerp-500 dark:text-elerp-200 tnum shrink-0">{pct}%</span>
        ) : null}
      </div>
    </div>
  )
}

/* ---------------------------------------------------------------------------
 * 8 · PANEL INFORMATIVO (handoff §03)
 * Fondo azul suave, SIN borde. Para ayudas, resúmenes y estados vacíos.
 * Nunca para contenido editable.
 * ------------------------------------------------------------------------- */
export const SoftPanel = ({ title, children, icon, corner = false, className = '' }) => (
  <div className={`relative overflow-hidden rounded-xl bg-elerp-50 dark:bg-elerp-900/40 p-[18px] ${className}`}>
    {corner ? <ModularCorner corner="br" variant="soft" tone="light" /> : null}
    <div className="relative">
      {title ? (
        <div className="flex items-center gap-2 mb-1.5">
          {icon ? <span className="text-elerp-500 dark:text-elerp-200 shrink-0">{icon}</span> : null}
          <div className="font-display font-semibold text-[14px] text-elerp-500 dark:text-elerp-100">{title}</div>
        </div>
      ) : null}
      <div className="text-[13px] text-slate-600 dark:text-slate-300 leading-relaxed">{children}</div>
    </div>
  </div>
)

/* ---------------------------------------------------------------------------
 * 9 · PANEL HERO / BANNER (handoff §02)
 * Superficie azul de marca con la esquina modular. Para cabeceras de módulo y
 * banners protagonistas. Una sola esquina, un solo hub.
 * ------------------------------------------------------------------------- */
export const HeroPanel = ({ children, corner = 'tr', className = '' }) => (
  <div className={`relative overflow-hidden rounded-xl bg-elerp-500 text-white p-6 ${className}`}>
    <ModularCorner corner={corner} variant="full" tone="dark" />
    <div className="relative">{children}</div>
  </div>
)
