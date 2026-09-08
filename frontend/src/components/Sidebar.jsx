import { useState, useEffect, useRef } from 'react'
import { NAV, GRUPOS, baseRoute, subOf } from './nav.js'
import { Logo, Wordmark } from './Logo.jsx'
import { HubDot } from './brand.jsx'
import { Icon } from './Icon.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'

/* Menú lateral — superficie de MARCA (degradado navy, FONDO_MENU), la misma
 * familia que la card hero del inicio y la portada del manual de branding. El
 * logo va en su versión BLANCA (mono) para contrastar sobre el azul.
 *   · Nombre completo del módulo, agrupado por OPERACIÓN / FINANZAS / GESTIÓN.
 *   · Ítem activo: loseta blanca translúcida, glifo en cuadro BLANCO con el
 *     glifo navy, y el punto hub verde a la DERECHA del ítem.
 *   · Ítems inactivos: glifo de línea blanco suelto, sin cuadro.
 *   · Al pie: Configuración y el botón Colapsar.
 *   · Colapsado: solo el isotipo y los glifos (el isotipo nunca baja de 24px).
 *
 * En escritorio (md+) es la barra fija de siempre. En móvil/tablet se esconde y
 * el MISMO contenido se muestra como CAJÓN deslizante (overlay + backdrop) que
 * abre la hamburguesa del Topbar; `drawerOpen`/`onCloseDrawer` lo controlan desde
 * App.jsx. No se duplica el menú: el cuerpo se comparte entre ambas superficies.
 */
// Fondo del menú: superficie de MARCA (degradado navy), la misma familia que la
// card hero del inicio y la portada del manual de branding
// (lib/tema.js FONDO_DEFAULT). Es un fondo azul en AMBOS temas —el menú es una
// superficie de marca, no un área de trabajo—, así que el logo va en blanco y el
// contraste interno se resuelve sobre azul (blancos translúcidos + teal que señala).
const FONDO_MENU = 'linear-gradient(180deg,#5A21DB 0%,#3B168C 55%,#2A2440 100%)'

// Acentos de "resalte" para ítems destacados del menú (además del launcher teal del
// POS y del blanco del ítem activo). Un ítem con `resalte` se pinta SIEMPRE con su
// color, como acción especial. Hoy: Aplicaciones (violeta), anclada al pie.
// Tintes calibrados para leerse SOBRE el degradado navy.
const RESALTE = {
  violet: {
    wrap: 'bg-violet-500/20 hover:bg-violet-500/30 ring-1 ring-inset ring-violet-300/30',
    box: 'bg-violet-400 text-white shadow-sm',
    text: 'font-semibold text-violet-100',
    chev: 'text-violet-200',
  },
}

export function Sidebar({ route, setRoute, drawerOpen = false, onCloseDrawer }) {
  const { activeEmpresa, activeSede } = useAuth()
  const { db } = useData()
  const { ui, setUi } = useUI()
  // Módulos activos de la empresa (Aplicaciones): un sub con `modulo` solo se muestra
  // si su módulo está activo (p. ej. la sub "Marketing" de Configuración).
  const modulos = db?.MODULOS || []
  // Un sub se ve si su módulo está activo Y —si declara `roles`— el rol lo incluye. Lo
  // segundo existe para el MESONERO: alcanza el módulo Restaurante pero solo le
  // corresponde la Comandera; el mapa, la cocina, las recetas y la impresora son de
  // administración o de cocina.
  const subVisible = (s) => (!s.modulo || modulos.includes(s.modulo)) && (!s.roles || s.roles.includes(ui.rol))
  const base = baseRoute(route)
  const sub = subOf(route)
  const colapsadoDesktop = !!ui.menuColapsado
  // Un módulo del menú se muestra si el rol lo permite Y —si declara `modulo`— ese
  // módulo comercializable está activo en la empresa (p. ej. "Restaurante").
  const allowed = (n) => n.roles.includes(ui.rol) && (!n.modulo || modulos.includes(n.modulo))

  const toggle = () => setUi((u) => ({ ...u, menuColapsado: !u.menuColapsado }))

  // Acordeón de UNA SOLA categoría abierta: guardamos el id del módulo expandido
  // (no un Set). Abrir otro módulo recoge el anterior, para que el menú no se
  // acumule verticalmente. Auto-expandimos el módulo de la ruta activa al cambiar.
  const [expanded, setExpanded] = useState(base || null)
  useEffect(() => { if (base) setExpanded(base) }, [base])

  // --- Flyout del menú COLAPSADO (solo escritorio) ---
  // Con la barra retraída no se ven los submódulos: al pasar el mouse (o enfocar)
  // un ícono, se abre un panel flotante a la derecha con el nombre del módulo y
  // sus submódulos, para poder navegar sin expandir la barra. Un pequeño retardo
  // al salir permite cruzar el hueco ícono→panel sin que se cierre.
  const [fly, setFly] = useState(null) // { id, top, bottom, up }
  const flyTimer = useRef(null)
  const openFly = (n, el) => {
    if (flyTimer.current) { clearTimeout(flyTimer.current); flyTimer.current = null }
    const r = el.getBoundingClientRect()
    const vh = typeof window !== 'undefined' ? window.innerHeight : 800
    // Ítems de la mitad inferior: el panel se ancla por su base y crece hacia
    // arriba, para no salirse de la pantalla (Aplicaciones/Configuración al pie).
    setFly({ id: n.id, top: r.top, bottom: vh - r.bottom, up: r.top > vh / 2 })
  }
  const closeFlySoon = () => {
    if (flyTimer.current) clearTimeout(flyTimer.current)
    flyTimer.current = setTimeout(() => setFly(null), 130)
  }
  const keepFly = () => { if (flyTimer.current) { clearTimeout(flyTimer.current); flyTimer.current = null } }
  // Al expandir la barra, cualquier flyout abierto sobra.
  useEffect(() => { if (!colapsadoDesktop) setFly(null) }, [colapsadoDesktop])

  // --- Cajón móvil: bloqueo de scroll del fondo, foco inicial, Escape y trampa de
  //     foco básica. Solo mientras está abierto (aria: role dialog + aria-modal). ---
  const drawerRef = useRef(null)
  const closeBtnRef = useRef(null)

  useEffect(() => {
    if (!drawerOpen) return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = prev }
  }, [drawerOpen])

  useEffect(() => {
    if (!drawerOpen) return
    // Foco al botón de cerrar al abrir (arranque predecible para teclado/lector).
    closeBtnRef.current?.focus()
    const onKey = (e) => {
      if (e.key === 'Escape') { e.preventDefault(); onCloseDrawer?.(); return }
      if (e.key !== 'Tab') return
      const panel = drawerRef.current
      if (!panel) return
      const foco = panel.querySelectorAll(
        'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])',
      )
      if (!foco.length) return
      const first = foco[0]
      const last = foco[foco.length - 1]
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus() }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [drawerOpen, onCloseDrawer])

  // Clic en un módulo. `pos` es un lanzador y no se expande. Con subs: expande
  // (recogiendo cualquier otro) y navega al primer submódulo; si ya está activo y
  // abierto, lo colapsa (sin navegar). Sin subs: navega directo.
  // `onNav` (cajón móvil) cierra el cajón, pero SOLO en navegaciones «hoja»: al
  // expandir un módulo con submenú lo dejamos abierto para poder elegir el sub.
  const goModule = (n, colapsado, onNav) => {
    if (n.id === 'pos' || !(n.subs && n.subs.length)) { setRoute(n.id); onNav?.(); return }
    if (colapsado) { setRoute(`${n.id}:${n.subs[0].id}`); onNav?.(); return }
    if (expanded === n.id && base === n.id) { setExpanded(null); return }
    setExpanded(n.id)
    setRoute(`${n.id}:${n.subs[0].id}`)
  }

  const NavItem = ({ n, colapsado, onNav }) => {
    const active = base === n.id
    const Glyph = n.glyph
    // El Punto de Venta es un LANZADOR (abre el puesto de cobro a pantalla
    // completa): se resalta con la línea gráfica del dinero (teal) y un filete,
    // para que se lea como una acción especial y no como un módulo más.
    const esLauncher = !!n.launcher
    // Ítem con acento propio (p. ej. Aplicaciones): se pinta SIEMPRE con su color.
    const rs = !esLauncher && n.resalte ? RESALTE[n.resalte] : null
    const tieneSubs = !!(n.subs && n.subs.length) && n.id !== 'pos'
    const abierto = tieneSubs && expanded === n.id && !colapsado
    return (
      <div className={`mb-0.5 ${esLauncher ? 'mt-1' : ''}`}>
        <button onClick={() => goModule(n, colapsado, onNav)} title={esLauncher ? `${n.label} — abrir el puesto de cobro` : n.label}
          aria-expanded={tieneSubs ? abierto : undefined}
          onMouseEnter={colapsado ? (e) => openFly(n, e.currentTarget) : undefined}
          onMouseLeave={colapsado ? closeFlySoon : undefined}
          onFocus={colapsado ? (e) => openFly(n, e.currentTarget) : undefined}
          onBlur={colapsado ? closeFlySoon : undefined}
          className={`w-full flex items-center gap-3 rounded-lg transition-colors ring-focus
            ${colapsado ? 'justify-center px-0 py-2' : 'px-2.5 py-2'}
            ${esLauncher
              ? 'bg-teal-500/20 hover:bg-teal-500/30 ring-1 ring-inset ring-teal-300/30'
              : rs
                ? rs.wrap
                : active
                  ? 'bg-white/15 ring-1 ring-inset ring-white/15'
                  : 'hover:bg-white/10'}`}>
          <span className={`shrink-0 inline-flex items-center justify-center transition-colors
            ${esLauncher
              ? 'h-8 w-8 rounded-icon bg-teal-500 text-white shadow-sm'
              : rs
                ? `h-8 w-8 rounded-icon ${rs.box}`
                : active
                  ? 'h-8 w-8 rounded-icon bg-white text-elerp-600 shadow-sm'
                  : 'h-8 w-8 text-white/70'}`}>
            <Glyph size={18} stroke={1.8} />
          </span>
          {!colapsado ? (
            <>
              <span className={`flex-1 text-left text-[13.5px] truncate
                ${esLauncher
                  ? 'font-semibold text-teal-100'
                  : rs
                    ? rs.text
                    : active ? 'font-semibold text-white' : 'font-medium text-white/85'}`}>
                {n.label}
              </span>
              {esLauncher
                ? <Icon.ChevRight size={16} className="shrink-0 text-teal-200" />
                : rs
                  ? <Icon.ChevRight size={16} className={`shrink-0 ${rs.chev}`} />
                  : tieneSubs
                    ? <Icon.ChevDown size={15} className={`shrink-0 text-white/50 transition-transform ${abierto ? '' : '-rotate-90'}`} />
                    : active ? <HubDot size={7} className="shrink-0" /> : null}
            </>
          ) : null}
        </button>

        {abierto ? (
          <div className="mt-0.5 mb-1 ml-[26px] pl-3 border-l border-white/15 space-y-px">
            {n.subs.filter(subVisible).map((s, i, subsVis) => {
              const subActivo = active && (sub === s.id || (!sub && s.id === subsVis[0].id))
              // Encabezado de grupo del submenú (p. ej. Facturación: Cliente ·
              // Proveedores · Reportes fiscales). Se dibuja al cambiar de `grupo`.
              const grupoHeader = s.grupo && s.grupo !== (i > 0 ? subsVis[i - 1].grupo : null) ? (
                <div key={`g-${s.grupo}`} className={`px-2.5 pb-1 text-[10.5px] font-semibold uppercase tracking-wider text-white/40 ${i > 0 ? 'pt-2.5' : 'pt-1'}`}>
                  {s.grupo}
                </div>
              ) : null
              return (
                <div key={s.id}>
                {grupoHeader}
                <button onClick={() => { setRoute(`${n.id}:${s.id}`); onNav?.() }} title={s.label}
                  className={`w-full flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left text-[12.5px] transition-colors ring-focus
                    ${subActivo
                      ? 'bg-white/15 font-semibold text-white'
                      : 'text-white/65 hover:bg-white/10'}`}>
                  <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${subActivo ? 'bg-teal-500' : 'bg-transparent'}`} />
                  <span className="truncate">{s.label}</span>
                </button>
                </div>
              )
            })}
          </div>
        ) : null}
      </div>
    )
  }

  const visibles = NAV.filter(allowed)
  const pie = visibles.filter((n) => n.grupo === 'pie')

  // Cuerpo del menú (grupos + módulos). Compartido por la barra de escritorio y
  // el cajón móvil; `colapsado` solo aplica a escritorio, `onNav` solo al cajón.
  const NavScroll = ({ colapsado, onNav }) => (
    <nav className="flex-1 overflow-y-auto px-2.5 pb-2">
      {GRUPOS.map((g) => {
        const items = visibles.filter((n) => n.grupo === g)
        if (!items.length) return null
        return (
          <div key={g || 'top'} className="pt-1">
            {g && !colapsado ? (
              <div className="text-[10.5px] font-semibold uppercase tracking-[0.09em] text-white/45 px-2.5 pt-3 pb-1.5">{g}</div>
            ) : g ? <div className="h-px bg-white/10 my-2 mx-2" /> : null}
            {items.map((n) => <NavItem key={n.id} n={n} colapsado={colapsado} onNav={onNav} />)}
          </div>
        )
      })}
    </nav>
  )

  const empresaMeta = (
    <div className="px-2.5 pt-1.5 pb-0.5">
      <div className="text-[11px] font-semibold text-white/85 truncate">{activeEmpresa?.nombre || '—'}</div>
      <div className="text-[10.5px] text-white/50 truncate mono">
        {activeEmpresa?.rif || ''}{activeSede ? ' · ' + activeSede.nombre : ''}
      </div>
    </div>
  )

  return (
    <>
      {/* Barra fija de escritorio (md+). Idéntica a siempre. */}
      <aside style={{ background: FONDO_MENU }} className={`hidden md:flex shrink-0 flex-col border-r border-white/10
        text-white sticky top-0 h-screen transition-[width] duration-200
        ${colapsadoDesktop ? 'w-[68px]' : 'w-[248px]'}`}>

        {/* Marca + colapsar */}
        <div className={`flex items-center h-[58px] shrink-0 ${colapsadoDesktop ? 'justify-center px-0' : 'justify-between pl-4 pr-2.5'}`}>
          {colapsadoDesktop ? <Logo size={26} mono /> : <Wordmark size={26} mono />}
          {!colapsadoDesktop ? (
            <button onClick={toggle} title="Colapsar el menú"
              className="h-7 w-7 rounded-lg inline-flex items-center justify-center text-white/60 hover:bg-white/10 ring-focus">
              <Icon.ChevLeft size={16} />
            </button>
          ) : null}
        </div>

        <NavScroll colapsado={colapsadoDesktop} />

        <div className="border-t border-white/10 px-2.5 py-2">
          {pie.map((n) => <NavItem key={n.id} n={n} colapsado={colapsadoDesktop} />)}
          {colapsadoDesktop ? (
            <button onClick={toggle} title="Expandir el menú"
              className="w-full mt-0.5 py-2 rounded-lg inline-flex items-center justify-center text-white/60 hover:bg-white/10 ring-focus">
              <Icon.ChevRight size={16} />
            </button>
          ) : (
            <>
              <button onClick={toggle}
                className="w-full mt-0.5 px-2.5 py-2 rounded-lg flex items-center gap-3 text-[13px] font-medium text-white/60 hover:bg-white/10 ring-focus">
                <span className="h-8 w-8 inline-flex items-center justify-center shrink-0"><Icon.ChevLeft size={16} /></span>
                Colapsar
              </button>
              {empresaMeta}
            </>
          )}
        </div>
      </aside>

      {/* Flyout del menú colapsado: panel flotante con los submódulos del ícono
          bajo el cursor, para navegar sin expandir la barra. Solo escritorio. */}
      {colapsadoDesktop && fly ? (() => {
        const n = NAV.find((x) => x.id === fly.id)
        if (!n || !allowed(n)) return null
        const subs = (n.subs || []).filter(subVisible)
        const irAlModulo = () => { setRoute(subs.length ? `${n.id}:${subs[0].id}` : n.id); setFly(null) }
        return (
          <div
            style={{ background: FONDO_MENU, ...(fly.up ? { bottom: fly.bottom } : { top: fly.top }) }}
            onMouseEnter={keepFly} onMouseLeave={closeFlySoon}
            role="menu" aria-label={n.label}
            className="hidden md:flex flex-col fixed left-[64px] z-[130] w-[214px] max-h-[calc(100vh-16px)]
              overflow-y-auto rounded-xl shadow-modal ring-1 ring-white/15 p-1.5 text-white">
            <button onClick={irAlModulo}
              className="w-full text-left px-2.5 py-1.5 rounded-lg text-[12.5px] font-semibold text-white hover:bg-white/10 ring-focus">
              {n.label}
            </button>
            {subs.length ? (
              <>
                <div className="h-px bg-white/10 my-1 mx-1.5" />
                <div className="space-y-px">
                  {subs.map((s, i, arr) => {
                    const subActivo = base === n.id && (sub === s.id || (!sub && s.id === arr[0].id))
                    const grupoHeader = s.grupo && s.grupo !== (i > 0 ? arr[i - 1].grupo : null) ? (
                      <div key={`g-${s.grupo}`} className={`px-2.5 pb-1 text-[10px] font-semibold uppercase tracking-wider text-white/40 ${i > 0 ? 'pt-2' : 'pt-0.5'}`}>{s.grupo}</div>
                    ) : null
                    return (
                      <div key={s.id}>
                        {grupoHeader}
                        <button onClick={() => { setRoute(`${n.id}:${s.id}`); setFly(null) }} role="menuitem"
                          className={`w-full flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left text-[12.5px] transition-colors ring-focus
                            ${subActivo ? 'bg-white/15 font-semibold text-white' : 'text-white/70 hover:bg-white/10'}`}>
                          <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${subActivo ? 'bg-teal-400' : 'bg-white/25'}`} />
                          <span className="truncate">{s.label}</span>
                        </button>
                      </div>
                    )
                  })}
                </div>
              </>
            ) : null}
          </div>
        )
      })() : null}

      {/* Cajón de navegación móvil/tablet (< md). Overlay + backdrop; reutiliza el
          MISMO cuerpo del menú. Se cierra al navegar a una hoja, tocar el backdrop,
          pulsar Escape o el botón de cerrar. */}
      {drawerOpen ? (
        <div className="md:hidden fixed inset-0 z-[120]" role="dialog" aria-modal="true" aria-label="Menú de navegación">
          <div className="absolute inset-0 bg-slate-900/50 backdrop-blur-sm backdrop-in" onClick={onCloseDrawer} />
          <div ref={drawerRef} style={{ background: FONDO_MENU }}
            className="drawer-in-left absolute inset-y-0 left-0 w-[86%] max-w-[300px] flex flex-col
              text-white border-r border-white/10 shadow-modal">
            <div className="flex items-center justify-between h-[58px] shrink-0 pl-4 pr-2.5 border-b border-white/10">
              <Wordmark size={26} mono />
              <button ref={closeBtnRef} onClick={onCloseDrawer} title="Cerrar el menú" aria-label="Cerrar el menú"
                className="h-8 w-8 rounded-lg inline-flex items-center justify-center text-white/60 hover:bg-white/10 ring-focus">
                <Icon.X size={18} />
              </button>
            </div>

            <NavScroll colapsado={false} onNav={onCloseDrawer} />

            <div className="border-t border-white/10 px-2.5 py-2">
              {pie.map((n) => <NavItem key={n.id} n={n} colapsado={false} onNav={onCloseDrawer} />)}
              {empresaMeta}
            </div>
          </div>
        </div>
      ) : null}
    </>
  )
}
