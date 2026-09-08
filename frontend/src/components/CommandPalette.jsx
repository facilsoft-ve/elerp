import { useState, useEffect, useMemo, useRef } from 'react'
import { Icon } from './Icon.jsx'
import { NAV } from './nav.js'
import { useData } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'

// Paleta de comandos global (Ctrl/⌘+K) — NAVEGACIÓN PRIMARIA por tarea (regla UX
// vinculante). Cubre TODOS los módulos y submódulos (generados desde nav.js, sin
// lista paralela que se desincronice), las acciones rápidas más frecuentes y la
// búsqueda de productos. Todo filtrado por el rol activo, igual que el Sidebar.

// Normaliza para comparar sin acentos ni mayúsculas.
const norm = (s) => (s || '').toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g, '')

// ¿Es `t` subsecuencia de `hay`? (tolerante a huecos: "libven" → "libro de ventas")
const subseq = (t, hay) => {
  let i = 0
  for (let k = 0; k < hay.length && i < t.length; k++) if (hay[k] === t[i]) i++
  return i === t.length
}

// Puntúa una coincidencia contra etiqueta + contexto. -1 = no coincide. Más alto
// = mejor (prefijo de la etiqueta gana; luego substring; luego contexto; luego
// subsecuencia difusa). Multi-palabra: cada token debe aparecer en el conjunto.
const matchScore = (term, label, context) => {
  const t = norm(term).trim()
  if (!t) return 0
  const l = norm(label)
  const c = norm(context)
  const hay = l + ' ' + c
  const tokens = t.split(/\s+/)
  if (!tokens.every((tok) => hay.includes(tok))) {
    return subseq(t.replace(/\s+/g, ''), hay.replace(/\s+/g, '')) ? 10 : -1
  }
  if (l.startsWith(t)) return 100
  if (l.includes(t)) return 85
  if (tokens.every((tok) => l.includes(tok))) return 70
  if (c.includes(t)) return 60
  return 40
}

// Encabezado de sección del módulo → texto de contexto legible (no "pie" ni "").
const seccionModulo = (grupo) => (grupo && grupo !== 'pie' ? grupo : '')

export function CommandPalette({ open, onClose, setRoute }) {
  const { db } = useData()
  const { ui, setUi } = useUI()
  const [q, setQ] = useState('')
  const [sel, setSel] = useState(0)
  const inputRef = useRef(null)
  const listRef = useRef(null)

  useEffect(() => {
    if (open) { setQ(''); setSel(0); setTimeout(() => inputRef.current?.focus(), 30) }
  }, [open])

  const visible = (id) => NAV.find((n) => n.id === id)?.roles.includes(ui.rol)

  // NAVEGACIÓN: todos los módulos y submódulos visibles para el rol, derivados de
  // nav.js. El submódulo lleva su grupo como contexto ("Facturación › Cliente");
  // el módulo con subs también aparece como destino directo (abre su 1er submódulo).
  const navItems = useMemo(() => {
    const items = []
    for (const m of NAV) {
      if (!m.roles.includes(ui.rol)) continue
      const icon = m.glyph
      if (m.subs && m.subs.length) {
        items.push({ id: m.id, label: m.label, context: seccionModulo(m.grupo), icon, route: `${m.id}:${m.subs[0].id}` })
        for (const s of m.subs) {
          items.push({
            id: `${m.id}:${s.id}`,
            label: s.label,
            context: [m.label, s.grupo].filter(Boolean).join(' › '),
            icon,
            route: `${m.id}:${s.id}`,
          })
        }
      } else {
        items.push({ id: m.id, label: m.label, context: seccionModulo(m.grupo), icon, route: m.id })
      }
    }
    return items
  }, [ui.rol])

  // ACCIONES RÁPIDAS frecuentes. Solo las que tienen sentido sin depender de
  // estado profundo, y solo si el módulo destino es visible para el rol. No hay
  // deep-link a modales de alta: cada acción lleva a la pantalla correspondiente
  // (ver nota en el reporte). El tema claro/oscuro se conmuta en el sitio.
  const acciones = useMemo(() => {
    const list = []
    if (visible('dashboard')) list.push({ id: 'go-dash', label: 'Ir al Inicio', icon: Icon.Home, run: () => setRoute('dashboard') })
    if (visible('pos')) list.push({ id: 'go-pos', label: 'Nueva venta · Punto de Venta', icon: Icon.Cart, run: () => setRoute('pos') })
    if (visible('facturacion')) list.push({ id: 'new-factura', label: 'Nueva factura', icon: Icon.Receipt, run: () => setRoute('facturacion:factura') })
    if (visible('ventas')) list.push({ id: 'new-cotizacion', label: 'Nueva cotización', icon: Icon.ClipboardList, run: () => setRoute('ventas:cotizaciones') })
    if (visible('ventas')) list.push({ id: 'new-cliente', label: 'Nuevo cliente', icon: Icon.UserPlus, run: () => setRoute('ventas:clientes') })
    if (visible('inventario')) list.push({ id: 'new-producto', label: 'Nuevo producto', icon: Icon.Package, run: () => setRoute('inventario:catalogo') })
    list.push({
      id: 'toggle-tema',
      label: ui.dark ? 'Cambiar a tema claro' : 'Cambiar a tema oscuro',
      icon: ui.dark ? Icon.Sun : Icon.Moon,
      run: () => setUi((u) => ({ ...u, dark: !u.dark })),
      keepOpen: true,
    })
    return list
  }, [ui.rol, ui.dark, setRoute, setUi])

  // Secciones ordenadas y estables: Acciones · Ir a… · Productos. Cada ítem
  // guarda su sección para dibujar encabezados; la lista plana alimenta el teclado.
  const results = useMemo(() => {
    const term = q.trim()
    const out = []

    acciones
      .map((a) => ({ a, s: matchScore(term, a.label, 'acción') }))
      .filter((x) => x.s >= 0)
      .sort((x, y) => y.s - x.s)
      .forEach(({ a }) => out.push({
        section: 'Acciones', kind: 'acción', id: a.id, label: a.label, icon: a.icon,
        action: a.run, keepOpen: a.keepOpen,
      }))

    navItems
      .map((n) => ({ n, s: matchScore(term, n.label, n.context) }))
      .filter((x) => x.s >= 0)
      .sort((x, y) => y.s - x.s)
      .forEach(({ n }) => out.push({
        section: 'Ir a…', kind: 'ir a', id: n.id, label: n.label, sub: n.context,
        icon: n.icon, action: () => setRoute(n.route),
      }))

    if (term && (db.PRODUCTOS || []).length) {
      ;(db.PRODUCTOS || [])
        .filter((p) => matchScore(term, p.nombre, p.sku || '') >= 0)
        .slice(0, 6)
        .forEach((p) => out.push({
          section: 'Productos', kind: 'producto', id: p.id, label: p.nombre, sub: p.sku,
          icon: Icon.Package, action: () => setRoute('kardex:' + p.sku),
        }))
    }
    return out
  }, [q, acciones, navItems, db.PRODUCTOS, setRoute])

  useEffect(() => { setSel(0) }, [q])
  // Mantener el ítem seleccionado a la vista al navegar con el teclado.
  useEffect(() => {
    listRef.current?.querySelector(`[data-idx="${sel}"]`)?.scrollIntoView({ block: 'nearest' })
  }, [sel])

  if (!open) return null

  const go = (item) => { if (item) { item.action(); if (!item.keepOpen) onClose() } }

  const onKey = (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setSel((s) => Math.min(s + 1, results.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)) }
    else if (e.key === 'Enter') { e.preventDefault(); go(results[sel]) }
    else if (e.key === 'Escape') { e.preventDefault(); onClose() }
  }

  let lastSection = null

  return (
    <div className="fixed inset-0 z-[180] flex items-start justify-center pt-[12vh] px-4">
      <div className="absolute inset-0 bg-slate-900/40 backdrop-blur-sm backdrop-in" onClick={onClose} />
      <div className="relative w-full max-w-xl bg-white dark:bg-slate-900 rounded-2xl shadow-modal border border-slate-200 dark:border-slate-800 modal-in overflow-hidden"
        role="dialog" aria-modal="true" aria-label="Buscador de comandos">
        <div className="flex items-center gap-2.5 px-4 h-12 border-b border-slate-100 dark:border-slate-800">
          <Icon.Search size={18} className="text-slate-400" />
          <input ref={inputRef} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onKey}
            role="combobox" aria-expanded="true" aria-controls="cmdk-list" aria-autocomplete="list"
            aria-activedescendant={results[sel] ? `cmdk-opt-${sel}` : undefined}
            placeholder="Buscar módulos, acciones, productos…"
            className="flex-1 bg-transparent outline-none text-sm placeholder:text-slate-400" />
          <kbd>esc</kbd>
        </div>
        <div ref={listRef} id="cmdk-list" role="listbox" aria-label="Resultados" className="max-h-[52vh] overflow-auto py-2">
          {results.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm text-slate-500">Sin resultados para “{q}”.</div>
          ) : results.map((r, i) => {
            const header = r.section !== lastSection ? r.section : null
            lastSection = r.section
            // Defensa: un ítem sin glifo pinta uno genérico en vez de romper la
            // pantalla entera (la paleta es la navegación primaria).
            const IconC = r.icon || Icon.Package
            const activo = i === sel
            return (
              <div key={r.section + r.kind + r.id}>
                {header ? (
                  <div role="presentation" className="px-4 pt-2 pb-1 text-[10.5px] font-semibold uppercase tracking-wider text-slate-400">
                    {header}
                  </div>
                ) : null}
                <button id={`cmdk-opt-${i}`} data-idx={i} role="option" aria-selected={activo}
                  onClick={() => go(r)} onMouseEnter={() => setSel(i)}
                  className={`w-full flex items-center gap-3 px-4 h-11 text-left ${activo ? 'bg-elerp-50 dark:bg-elerp-900/40' : ''}`}>
                  <IconC size={17} className={activo ? 'text-elerp-600 dark:text-elerp-300' : 'text-slate-400'} />
                  <div className="flex-1 min-w-0">
                    <div className="text-[13.5px] font-medium truncate">{r.label}</div>
                    {r.sub ? <div className="text-[11.5px] text-slate-400 truncate">{r.sub}</div> : null}
                  </div>
                  <span className="text-[10.5px] uppercase tracking-wide text-slate-400 shrink-0">{r.kind}</span>
                </button>
              </div>
            )
          })}
        </div>
        <div className="px-4 h-9 flex items-center gap-3 text-[11px] text-slate-400 border-t border-slate-100 dark:border-slate-800">
          <span><kbd>↑</kbd><kbd>↓</kbd> navegar</span>
          <span><kbd>↵</kbd> abrir</span>
          <span className="ml-auto"><kbd>esc</kbd> cerrar</span>
        </div>
      </div>
    </div>
  )
}
