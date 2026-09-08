import { useState } from 'react'
import { Icon } from '../components/Icon.jsx'
import {
  Button, Card, CardTitle, Badge, StatusBadge, ESTADOS, Stat, Tabs, Segmented,
  Input, Select, Field, Toggle, Empty, Skeleton,
} from '../components/primitives.jsx'
import {
  HubDot, IdleMarker, TitleHub, HubSeparator, ModularCorner, ModularBand,
  ModularCluster, ModuleIcon, ModuleShortcut, ModuleTile, MODULOS, Progress,
  SoftPanel, HeroPanel,
} from '../components/brand.jsx'

/* Pantalla de referencia del sistema de diseño.
 * Es el inventario vivo de la marca dentro del producto: cada bloque cita la
 * regla del handoff que implementa, para que cualquier pantalla nueva se pueda
 * verificar contra esto sin volver a abrir los PDF. */

// Ficha de token de color con su valor y su variable CSS.
const Swatch = ({ name, hex, cssVar, dark = false }) => (
  <div className="min-w-0">
    <div className="h-14 rounded-lg border border-slate-200 dark:border-slate-700" style={{ background: hex }} />
    <div className="mt-1.5 text-[12px] font-semibold text-slate-900 dark:text-slate-100 truncate">{name}</div>
    <div className="text-[11px] text-slate-500 mono">{hex}</div>
    <div className="text-[10.5px] text-slate-400 mono truncate">{cssVar}</div>
  </div>
)

const Section = ({ n, title, rule, children }) => (
  <section className="space-y-3">
    <div>
      <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-teal-600 dark:text-teal-400">{n}</div>
      <h2 className="font-display font-bold text-[19px] text-elerp-500 dark:text-white mt-0.5">{title}</h2>
      {rule ? <p className="text-[13px] text-slate-500 mt-1 max-w-2xl">{rule}</p> : null}
    </div>
    {children}
  </section>
)

export function SistemaDiseno() {
  const [tab, setTab] = useState('fundamentos')
  const [tile, setTile] = useState('vender')
  const [modActivo, setModActivo] = useState('facturacion')
  const [toggle, setToggle] = useState(true)

  const TABS = [
    { id: 'fundamentos', label: 'Fundamentos', icon: <Icon.Layers size={15} /> },
    { id: 'graficos', label: 'Gráficos complementarios', icon: <Icon.Sparkles size={15} /> },
    { id: 'componentes', label: 'Componentes', icon: <Icon.ClipboardList size={15} /> },
    { id: 'modulos', label: 'Iconos de módulo', icon: <Icon.Boxes size={15} /> },
  ]

  return (
    <div className="p-4 md:p-6 space-y-6">
      {/* Panel hero con la esquina modular: la propia página demuestra la regla. */}
      <HeroPanel corner="tr">
        <div className="max-w-lg">
          <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-white/60">Referencia interna</div>
          <div className="font-display font-bold text-[28px] leading-tight mt-1">Sistema de diseño</div>
          <p className="text-[13.5px] text-white/75 mt-2">
            Los tokens, gráficos y componentes de ElERP tal como los define el handoff
            de branding. El azul estructura, el verde señala.
          </p>
          <div className="mt-4"><HubSeparator tone="onDark" /></div>
        </div>
      </HeroPanel>

      <Tabs tabs={TABS} active={tab} onChange={setTab} />

      {/* ---------------------------------------------------------------- */}
      {tab === 'fundamentos' ? (
        <div className="space-y-6">
          <Section n="01 · Fundamentos" title="Tokens de color"
            rule="Definidos una sola vez como custom properties en src/index.css y reflejados en la escala de Tailwind. Nunca uses valores sueltos en el código.">
            <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-5 gap-4">
              <Swatch name="Azul ElERP" hex="#1D3477" cssVar="--hb-azul" />
              <Swatch name="Verde ElERP" hex="#09B69B" cssVar="--hb-verde" />
              <Swatch name="Carbón" hex="#1F2430" cssVar="--hb-carbon" />
              <Swatch name="Gris medio" hex="#5C6470" cssVar="--hb-gris" />
              <Swatch name="Borde" hex="#E8EAEF" cssVar="--hb-borde" />
              <Swatch name="Fondo app" hex="#FAFBFC" cssVar="--hb-fondo" />
              <Swatch name="Azul suave" hex="#E9EDF6" cssVar="--hb-azul-suave" />
              <Swatch name="Azul suave 2" hex="#D4DCEE" cssVar="--hb-azul-suave-2" />
              <Swatch name="Verde suave" hex="#E2F7F3" cssVar="--hb-verde-suave" />
              <Swatch name="Azul hover" hex="#16295F" cssVar="--hb-azul-hover" />
            </div>
            <SoftPanel title="Regla de oro del color" icon={<Icon.CircleAlert size={16} />}>
              El verde aparece <strong>solo</strong> donde hay acción o estado positivo: botón de
              crear, elemento activo del menú, indicadores de éxito y el anillo de foco.
              Si una pantalla tiene más verde que azul, está mal balanceada. El rojo
              existe únicamente para errores.
            </SoftPanel>
          </Section>

          <Section n="01 · Fundamentos" title="Tipografía"
            rule="Poppins solo para títulos de pantalla, cifras destacadas y el nombre de la marca. Todo lo demás — tablas, formularios, menús — en IBM Plex Sans. Tamaño mínimo de UI: 13px. Las cifras de tabla, RIF, SKU y hashes van en IBM Plex Mono tabular.">
            <Card>
              <div className="space-y-3">
                <div className="font-display font-bold text-[28px] text-elerp-500 dark:text-white leading-tight">Título H1 — Poppins Bold 28</div>
                <div className="font-display font-semibold text-[19px] text-elerp-500 dark:text-slate-100">Subtítulo H2 — Poppins SemiBold 19</div>
                <div className="font-display font-semibold text-[13px] uppercase tracking-[0.14em] text-slate-500">Etiqueta — Poppins SemiBold 13</div>
                <div className="text-sm text-slate-600 dark:text-slate-300 max-w-[65ch] leading-relaxed">
                  Cuerpo de texto — IBM Plex Sans Regular 14/24. Longitud de línea recomendada
                  entre 60 y 75 caracteres para una lectura cómoda en pantalla.
                </div>
                <div className="flex flex-wrap items-baseline gap-6 pt-1">
                  <div>
                    <div className="text-[11px] uppercase tracking-wide text-slate-500 mb-1">Cifra destacada · Poppins</div>
                    <div className="font-display font-bold text-[26px] tnum text-slate-900 dark:text-slate-100">$84.200,00</div>
                  </div>
                  <div>
                    <div className="text-[11px] uppercase tracking-wide text-slate-500 mb-1">Cifra de tabla · Plex Mono</div>
                    <div className="num text-[18px] text-slate-900 dark:text-slate-100">1.284.905,37</div>
                  </div>
                  <div>
                    <div className="text-[11px] uppercase tracking-wide text-slate-500 mb-1">Identificador · Plex Mono</div>
                    <div className="mono text-[18px] text-slate-900 dark:text-slate-100">J-40123456-7</div>
                  </div>
                </div>
              </div>
            </Card>
          </Section>

          <Section n="01 · Fundamentos" title="Forma"
            rule="Radio 8px en controles, 12px en cards, 11px en el cuadro de icono de módulo y 16px en losetas táctiles. Sombra tintada de azul; las cards de métrica van sin sombra en reposo. No mezclar otros radios ni sombras duras.">
            <div className="flex flex-wrap gap-4">
              {[
                { r: 'rounded-lg', l: '8px · controles', v: '--hb-radio' },
                { r: 'rounded-icon', l: '11px · icono módulo', v: '--hb-radio-icono' },
                { r: 'rounded-xl', l: '12px · cards', v: '--hb-radio-card' },
                { r: 'rounded-tile', l: '16px · losetas', v: '--hb-radio-loseta' },
              ].map((x) => (
                <div key={x.l} className="text-center">
                  <div className={`h-16 w-16 bg-elerp-50 dark:bg-elerp-900/50 border border-elerp-100 dark:border-elerp-800 ${x.r}`} />
                  <div className="text-[11.5px] font-medium mt-1.5">{x.l}</div>
                  <div className="text-[10.5px] text-slate-400 mono">{x.v}</div>
                </div>
              ))}
              <div className="text-center">
                <div className="h-16 w-16 bg-white dark:bg-slate-900 rounded-xl shadow-brand border border-slate-200 dark:border-slate-700" />
                <div className="text-[11.5px] font-medium mt-1.5">Sombra de marca</div>
                <div className="text-[10.5px] text-slate-400 mono">--hb-sombra</div>
              </div>
            </div>
          </Section>
        </div>
      ) : null}

      {/* ---------------------------------------------------------------- */}
      {tab === 'graficos' ? (
        <div className="space-y-6">
          <Section n="02 · Gráficos complementarios" title="El punto hub"
            rule="El punto verde del logo es el gráfico complementario principal: marcador de estado, viñeta de lista destacada o remate de título. Regla: un solo punto hub protagonista por vista.">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <Card>
                <div className="text-[12px] uppercase tracking-wide text-slate-500 mb-3">Marcador de estado</div>
                <div className="space-y-2">
                  <div className="flex items-center gap-2.5 text-[13.5px]"><HubDot /> Módulo activo</div>
                  <div className="flex items-center gap-2.5 text-[13.5px] text-slate-400"><IdleMarker /> Módulo inactivo</div>
                </div>
              </Card>
              <Card>
                <div className="text-[12px] uppercase tracking-wide text-slate-500 mb-3">Remate de título</div>
                <TitleHub as="div" size="lg">Inventario</TitleHub>
                <div className="text-[12.5px] text-slate-500 mt-1">El punto cierra el titular, como en el logo.</div>
              </Card>
              <Card>
                <div className="text-[12px] uppercase tracking-wide text-slate-500 mb-3">Separador decorativo</div>
                <div className="py-3"><HubSeparator /></div>
                <div className="text-[12.5px] text-slate-500">Barra azul + punto + barra gris.</div>
              </Card>
            </div>
          </Section>

          <Section n="02 · Gráficos complementarios" title="La esquina modular"
            rule="Racimo de 2–4 cuadros redondeados en tintes de azul más un punto verde, anclados a una esquina. Nunca en las cuatro a la vez — una sola por componente.">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <HeroPanel corner="tr">
                  <div className="font-display font-bold text-[17px]">Panel hero / banner</div>
                  <div className="text-[13px] text-white/70 mt-0.5">Esquina modular superior derecha</div>
                </HeroPanel>
                <div className="text-[12px] text-slate-500 mt-2">Sobre azul: módulos blancos translúcidos (10–18%) + hub verde sólido.</div>
              </div>
              <div>
                <Card className="relative overflow-hidden min-h-[118px]">
                  <ModularCorner corner="bl" variant="soft" tone="light" />
                  <div className="relative">
                    <div className="font-display font-bold text-[17px] text-elerp-500 dark:text-white">Card clara</div>
                    <div className="text-[13px] text-slate-500 mt-0.5">Esquina modular inferior izquierda</div>
                  </div>
                </Card>
                <div className="text-[12px] text-slate-500 mt-2">Sobre claro: tintes de azul (#E9EDF6, #D4DCEE) + hub verde.</div>
              </div>
            </div>
          </Section>

          <Section n="Manual §08" title="Banda modular"
            rule="Retícula de módulos redondeados para cabeceras y cierres de página. El círculo verde aparece una sola vez por composición — es el hub. No cubrir más del 25% de una pieza con la textura.">
            <Card padding={false} className="overflow-hidden">
              <ModularBand height={64} />
            </Card>
          </Section>

          <Section n="04 · Estados" title="Progreso y estado vacío"
            rule="La barra de progreso termina en verde: «llegar al hub». Es el único degradado permitido en la interfaz. Los estados vacíos usan el racimo modular como ilustración, mensaje corto y CTA verde.">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Card className="space-y-4">
                <Progress value={64} label="Sincronización de documentos" />
                <Progress value={28} label="Carga de catálogo" />
                <Progress value={100} label="Cierre Z" />
              </Card>
              <Card padding={false}>
                <Empty
                  title="Aún no hay módulos configurados"
                  body="Agrega tu primer módulo para empezar a construir tu sistema."
                  cta={<Button variant="cta" icon={<Icon.Plus size={16} />}>Agregar módulo</Button>}
                />
              </Card>
            </div>
            <div className="flex items-center gap-6 pt-1">
              <div className="text-[12px] text-slate-500">Racimo modular como ilustración:</div>
              <ModularCluster width={72} />
            </div>
          </Section>
        </div>
      ) : null}

      {/* ---------------------------------------------------------------- */}
      {tab === 'componentes' ? (
        <div className="space-y-6">
          <Section n="03 · Componentes" title="Botones"
            rule="Radio 8px, Poppins Medium 14px, altura 40px (44px mínimo táctil en móvil). Una sola acción primaria por vista. Nunca dos primarios juntos.">
            <Card>
              <div className="flex flex-wrap items-center gap-3">
                <Button variant="primary">Guardar cambios</Button>
                <Button variant="cta" icon={<Icon.Plus size={16} />}>Nuevo módulo</Button>
                <Button variant="secondary">Secundario</Button>
                <Button variant="tertiary">Terciario / enlace</Button>
                <Button disabled>Deshabilitado</Button>
              </div>
              <div className="grid grid-cols-2 md:grid-cols-5 gap-x-4 gap-y-2 mt-5 text-[12px] text-slate-500">
                <div><span className="font-semibold text-elerp-500 dark:text-elerp-200">Primario</span> — azul. Acción principal de la vista.</div>
                <div><span className="font-semibold text-teal-600 dark:text-teal-400">CTA / crear</span> — verde. Crear algo nuevo, confirmar éxito.</div>
                <div><span className="font-semibold text-elerp-500 dark:text-elerp-200">Secundario</span> — contorno azul.</div>
                <div><span className="font-semibold text-teal-600 dark:text-teal-400">Terciario</span> — texto verde, sin fondo.</div>
                <div><span className="font-semibold text-slate-400">Deshabilitado</span> — gris claro.</div>
              </div>
              <div className="mt-5 pt-4 border-t border-slate-100 dark:border-slate-800 text-[12px] text-slate-500">
                <strong className="text-slate-700 dark:text-slate-300">Estados:</strong> hover = oscurecer 8% (azul → #16295F, verde → #079C85) ·
                focus = anillo 2px #09B69B con offset 2px · active = oscurecer 14%.
              </div>
              <div className="mt-4 flex flex-wrap items-center gap-3">
                <Button size="sm">Pequeño 36px</Button>
                <Button size="md">Oficial 40px</Button>
                <Button size="lg">Táctil 44px</Button>
                <Button loading>Cargando</Button>
                <Button variant="destructive" icon={<Icon.Trash size={15} />}>Anular</Button>
              </div>
            </Card>
          </Section>

          <Section n="03 · Componentes" title="Cards y paneles"
            rule="Card de métrica: borde gris, sin sombra en reposo. Card seleccionada: filo verde de 4px en el borde izquierdo. Panel suave: fondo azul suave sin borde, nunca para contenido editable.">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <Stat label="Ventas del mes" value="$84.200" delta="12%" deltaPositive sub="vs. junio" />
              <Card selected>
                <CardTitle>Módulo seleccionado</CardTitle>
                <div className="text-[13px] text-slate-500 mt-1.5">La selección se marca con el filo verde de 4px en el borde izquierdo y fondo blanco.</div>
              </Card>
              <SoftPanel title="Panel informativo">
                Fondo azul suave para ayudas, resúmenes y estados vacíos. Nunca para contenido editable.
              </SoftPanel>
            </div>
          </Section>

          <Section n="04 · Componentes" title="Badges de estado, tabs y controles"
            rule="Los cuatro estados canónicos, siempre con punto: el punto hub unifica badges y navegación. Cualquier estado nuevo se mapea a uno de estos cuatro en lugar de inventar un color.">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Card>
                <div className="text-[12px] uppercase tracking-wide text-slate-500 mb-3">Badges de estado</div>
                <div className="flex flex-wrap gap-2">
                  {Object.keys(ESTADOS).map((k) => <StatusBadge key={k} estado={k} />)}
                </div>
                <div className="text-[12px] uppercase tracking-wide text-slate-500 mt-5 mb-2.5">Otros badges de dominio</div>
                <div className="flex flex-wrap gap-2">
                  <Badge color="huberp">Factura</Badge>
                  <Badge color="teal" dot>USD</Badge>
                  <Badge color="amber" dot>Contingencia</Badge>
                  <Badge color="slate">Consumidor final</Badge>
                </div>
              </Card>
              <Card className="space-y-5">
                <div>
                  <div className="text-[12px] uppercase tracking-wide text-slate-500 mb-2">Segmentado</div>
                  <Segmented value="VES" onChange={() => {}} options={[{ value: 'VES', label: 'Bs' }, { value: 'USD', label: '$' }]} />
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <Field label="Producto" required>
                    <Input placeholder="Buscar por nombre o SKU" icon={<Icon.Search size={15} />} />
                  </Field>
                  <Field label="Sede" hint="acota los datos">
                    <Select><option>Sede Principal</option><option>Sede Este</option></Select>
                  </Field>
                </div>
                <Field label="Cantidad" error="Debe ser mayor a 0.">
                  <Input invalid defaultValue="0" />
                </Field>
                <Toggle checked={toggle} onChange={setToggle} label="Modo contingencia" sub="Numeración pre-asignada offline" />
                <div>
                  <div className="text-[12px] uppercase tracking-wide text-slate-500 mb-2">Skeleton de carga</div>
                  <div className="space-y-2"><Skeleton className="h-8 w-2/3" /><Skeleton className="h-8 w-full" /></div>
                </div>
              </Card>
            </div>
          </Section>
        </div>
      ) : null}

      {/* ---------------------------------------------------------------- */}
      {tab === 'modulos' ? (
        <div className="space-y-6">
          <Section n="05 · Iconos de módulo" title="Familia completa"
            rule="Glifo de línea azul (trazo 1.8, retícula 24×24) dentro de un cuadro redondeado azul suave. Todos comparten el mismo estilo — la identidad del módulo está en el glifo, nunca en un color propio. El punto hub verde aparece solo en el módulo activo o con notificaciones. Etiqueta debajo, siempre en IBM Plex Sans.">
            <Card>
              <div className="flex flex-wrap gap-1 justify-center md:justify-start">
                {MODULOS.map((m) => (
                  <ModuleShortcut key={m.id} modulo={m} active={modActivo === m.id}
                    onClick={() => setModActivo(m.id)} />
                ))}
              </div>
              <div className="text-[12px] text-slate-500 mt-3">
                Haz clic para mover el punto hub — solo el módulo activo lo lleva.
              </div>
            </Card>
            <div className="flex flex-wrap items-end gap-6">
              {[28, 40, 56, 72].map((s) => (
                <div key={s} className="text-center">
                  <ModuleIcon glyph={Icon.ModFiscal} size={s} />
                  <div className="text-[11px] text-slate-500 mt-1.5 mono">{s}px</div>
                </div>
              ))}
              <div className="text-center">
                <ModuleIcon glyph={Icon.ModFiscal} size={40} hub />
                <div className="text-[11px] text-slate-500 mt-1.5">con hub</div>
              </div>
              <div className="text-center">
                <ModuleIcon glyph={Icon.ModFiscal} size={40} invert />
                <div className="text-[11px] text-slate-500 mt-1.5">invertido</div>
              </div>
            </div>
          </Section>

          <Section n="05 · Losetas táctiles" title="POS y alto tráfico"
            rule="Para cajas, kioscos y pantallas táctiles el icono crece a loseta: mínimo 120×120 px, glifo de 40px, etiqueta de 14px y separación de 16px entre losetas. Toda la loseta es el área de toque. La loseta activa invierte a azul con glifo blanco y punto hub. Radio 16px.">
            <Card>
              <div className="flex flex-wrap gap-4">
                <ModuleTile glyph={Icon.Cart} label="Vender" state={tile === 'vender' ? 'active' : 'idle'} onClick={() => setTile('vender')} />
                <ModuleTile glyph={Icon.ModInventario} label="Inventario" state={tile === 'inventario' ? 'active' : 'idle'} onClick={() => setTile('inventario')} />
                <ModuleTile glyph={Icon.ModTesoreria} label="Cobrar" state={tile === 'cobrar' ? 'active' : 'idle'} onClick={() => setTile('cobrar')} />
                <ModuleTile glyph={Icon.Plus} label="Nuevo" state="create" />
              </div>
            </Card>
          </Section>

          <Section n="06 · Resumen" title="Checklist para desarrolladores" rule="">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Card>
                <div className="font-display font-semibold text-[14px] text-teal-600 dark:text-teal-400 mb-2.5">Sí · Do</div>
                <ul className="space-y-2 text-[13px] text-slate-600 dark:text-slate-300">
                  {[
                    'Usar los tokens, nunca colores sueltos.',
                    'Un punto hub protagonista por vista.',
                    'Esquina modular en una sola esquina.',
                    'Verde solo para acción, éxito y estado activo.',
                    'Radio 8px en controles, 12px en cards.',
                    'Isotipo en el sidebar y favicon; logo lite en login y pantallas de marca.',
                    'Iconos de módulo: glifo azul, trazo 1.8, fondo #E9EDF6, radio 11px.',
                  ].map((t) => (
                    <li key={t} className="flex gap-2"><Icon.Check size={15} className="mt-0.5 shrink-0 text-teal-500" /><span>{t}</span></li>
                  ))}
                </ul>
              </Card>
              <Card>
                <div className="font-display font-semibold text-[14px] text-red-600 dark:text-red-400 mb-2.5">No · Don't</div>
                <ul className="space-y-2 text-[13px] text-slate-600 dark:text-slate-300">
                  {[
                    'No usar verde como color de fondo grande.',
                    'No poner dos botones primarios juntos.',
                    'No usar el logotipo completo dentro de la UI (solo isotipo o lite).',
                    'No mezclar otros radios ni sombras duras.',
                    'No usar degradados salvo la barra de progreso.',
                    'No introducir colores fuera de la paleta (rojo solo para errores).',
                  ].map((t) => (
                    <li key={t} className="flex gap-2"><Icon.X size={15} className="mt-0.5 shrink-0 text-red-500" /><span>{t}</span></li>
                  ))}
                </ul>
              </Card>
            </div>
          </Section>
        </div>
      ) : null}
    </div>
  )
}
