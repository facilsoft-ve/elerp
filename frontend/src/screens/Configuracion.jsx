import { useState, useEffect, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Select, Segmented, Toggle, Empty, PageHeader, useToast, TableSkeleton, Field, Input, Modal, useConfirm } from '../components/primitives.jsx'
import { SubidorImagen } from '../components/SubidorImagen.jsx'
import { MaquetaPantallaCliente } from '../components/MaquetaPantallaCliente.jsx'
import { resolverTema } from '../lib/tema.js'
import { fmtNum, fmtDate, fmtCurrency } from '../lib/format.js'
import { useData, useTasa } from '../context/DataContext.jsx'
import { useUI } from '../context/UIContext.jsx'
import { useAuth } from '../context/AuthContext.jsx'
import { api } from '../lib/api.js'
import { fechaCortaVE, explicarFallo } from '../components/tasa.jsx'
import { monedaLabel, monedaNombre, monedaSimbolo, permiteFuenteBcv } from '../lib/precio.js'
import { Promociones } from './Promociones.jsx'
import { FormatosDocumento } from './FormatosDocumento.jsx'

/* Configuración — las pestañas del prototipo, más la de Moneda que exige la
 * arquitectura (R9 + R10).
 *
 * Regla que se respeta en toda la pantalla: cada bloque muestra el ESTADO REAL
 * de lo que hay detrás. Donde todavía no hay módulo (dispositivos fiscales,
 * pasarelas de pago), se dice qué falta en vez de pintar un «Conectado» que
 * sería mentira — el prototipo los muestra activos porque es una maqueta.
 */
// Pestañas de Configuración, agrupadas por tema (el `grupo` alinea con el
// submenú del Sidebar en nav.js). El orden y los `label` COINCIDEN con nav.js
// para que el menú lateral y la miga de pan digan exactamente lo mismo. TABS
// alimenta la miga de pan del PageHeader y el ruteo por sub de esta pantalla.
const TABS = [
  { id: 'empresa', label: 'Datos de empresa', grupo: 'Empresa', icon: <Icon.Bank size={15} /> },
  { id: 'sedes', label: 'Sedes', grupo: 'Empresa', icon: <Icon.Home size={15} /> },
  { id: 'usuarios', label: 'Usuarios y roles', grupo: 'Empresa', icon: <Icon.Users size={15} /> },
  { id: 'unidades', label: 'Unidades de medida', grupo: 'Empresa', icon: <Icon.Boxes size={15} /> },
  { id: 'almacenes', label: 'Almacenes', grupo: 'Empresa', icon: <Icon.Package size={15} /> },
  { id: 'impuestos', label: 'Impuestos y alícuotas', grupo: 'Fiscal', icon: <Icon.Receipt size={15} /> },
  { id: 'numeracion', label: 'Series y numeración', grupo: 'Fiscal', icon: <Icon.ClipboardList size={15} /> },
  { id: 'formatos', label: 'Formatos de documento', grupo: 'Fiscal', icon: <Icon.FileText size={15} /> },
  { id: 'moneda', label: 'Moneda y tasa', grupo: 'Fiscal', icon: <Icon.Banknote size={15} /> },
  { id: 'dispositivos', label: 'Dispositivos fiscales', grupo: 'Fiscal', icon: <Icon.Printer size={15} /> },
  { id: 'cajas', label: 'Cajas y sesiones', grupo: 'Punto de venta', icon: <Icon.Wallet size={15} /> },
  { id: 'punto-venta', label: 'Punto de venta', grupo: 'Punto de venta', icon: <Icon.Cart size={15} /> },
  { id: 'metodos', label: 'Métodos de pago', grupo: 'Punto de venta', icon: <Icon.Wallet size={15} /> },
  { id: 'marketing', label: 'Marketing', grupo: 'Comercial', icon: <Icon.Sparkles size={15} /> },
  { id: 'asistente', label: 'Asistente IA', grupo: 'Plataforma', icon: <Icon.Sparkles size={15} /> },
  { id: 'integraciones', label: 'Integraciones', grupo: 'Plataforma', icon: <Icon.Plug size={15} /> },
]

const ROLES = [
  { id: 'dueno', label: 'Dueña / Admin' },
  { id: 'vendedor', label: 'Vendedor' },
  { id: 'cajero', label: 'Cajero' },
  { id: 'mesonero', label: 'Mesonero' },
  { id: 'contadora', label: 'Contadora' },
  { id: 'desarrollador', label: 'Desarrollador / IT' },
]
const rolLabel = (r) => ROLES.find((x) => x.id === r)?.label || r
// Roles atados a UNA sede: el servidor les fija la sede en cada petición.
const UNA_SOLA_SEDE = ['cajero', 'vendedor', 'mesonero']

export function Configuracion({ route }) {
  const { db, loading, error, reload } = useData()
  const [tab, setTab] = useState((route || '').split(':')[1] || 'empresa')
  useEffect(() => { setTab((route || '').split(':')[1] || 'empresa') }, [route])
  // Guard de carga/error del bootstrap del tenant: sin este, un fallo de red
  // dejaba los formularios EN BLANCO (indistinguibles de "sin configurar"). Si el
  // bootstrap no cargó la empresa, se explica el fallo y se ofrece reintentar en
  // vez de pintar campos vacíos.
  const bootstrapFallo = !!error && !db?.EMPRESA
  return (
    <div className="p-4 md:p-6 lg:px-8 lg:py-7">
      <PageHeader
        breadcrumb={['Configuración', TABS.find((t) => t.id === tab)?.label]}
        title="Configuración"
        sub="Usuarios y roles, cajas, moneda y tasa, métodos de pago, dispositivos fiscales e integraciones."
        tabs={TABS} activeTab={tab} onTab={setTab} />
      {loading && !db?.EMPRESA ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
          <TableSkeleton rows={6} cols={3} />
        </div>
      ) : bootstrapFallo ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar la configuración"
          body={String(error?.message || error)}
          cta={<Button onClick={reload} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : (<>
      {tab === 'empresa' ? <DatosEmpresa /> : null}
      {tab === 'sedes' ? <Sedes /> : null}
      {tab === 'usuarios' ? <Usuarios /> : null}
      {tab === 'unidades' ? <Unidades /> : null}
      {tab === 'almacenes' ? <Almacenes /> : null}
      {tab === 'impuestos' ? <Impuestos /> : null}
      {tab === 'numeracion' ? <Numeracion /> : null}
      {tab === 'formatos' ? <FormatosDocumento /> : null}
      {tab === 'moneda' ? <MonedaYTasa /> : null}
      {tab === 'dispositivos' ? <Dispositivos /> : null}
      {tab === 'cajas' ? <CajasYSesiones /> : null}
      {tab === 'punto-venta' ? <PuntoVenta /> : null}
      {tab === 'metodos' ? <MetodosPago /> : null}
      {tab === 'marketing' ? ((db.MODULOS || []).includes('marketing') ? <Marketing /> : (
        <Empty icon={<Icon.Package size={22} />} title="El módulo Marketing no está activo"
          body="Actívalo en la sección Aplicaciones para configurar la publicidad de la pantalla del cliente." />
      )) : null}
      {tab === 'asistente' ? ((db.MODULOS || []).includes('asistente-ia') ? <AsistenteIAConfig /> : (
        <Empty icon={<Icon.Package size={22} />} title="El módulo Asistente IA no está activo"
          body="Actívalo en la sección Aplicaciones para usar el asistente flotante y configurar la IA." />
      )) : null}
      {tab === 'integraciones' ? <Integraciones /> : null}
      </>)}
    </div>
  )
}

// Solo la Dueña/Admin y el Desarrollador editan la empresa y sus sedes. La
// interfaz oculta las acciones; el backend valida el rol en cada endpoint.
const PUEDE_ADMINISTRAR = ['dueno', 'desarrollador']

// normPublicidad normaliza la lista de slides: tolera la forma antigua ([]string
// de URLs), recorta y descarta los vacíos según el tipo. El backend la revalida.
function normPublicidad(lista) {
  return (Array.isArray(lista) ? lista : [])
    .map((s) => (typeof s === 'string' ? { tipo: 'imagen', imagen: s } : s))
    .map((s) => (s?.tipo === 'imagen'
      ? { tipo: 'imagen', imagen: (s.imagen || '').trim() }
      : { tipo: 'texto', texto: (s.texto || '').trim(), subtexto: (s.subtexto || '').trim() }))
    .filter((s) => (s.tipo === 'imagen' ? s.imagen : s.texto))
}

/* construirPayloadEmpresa arma el SET COMPLETO de campos que espera
 * `actualizarEmpresa` (endpoint full-replace: reemplaza toda la cabecera del
 * tenant). Parte de la empresa guardada (`emp`) y aplica solo los cambios de la
 * pantalla actual (`over`). Así cada pantalla (Datos de empresa / Marketing) edita
 * su parte sin pisar la del otro. */
function construirPayloadEmpresa(emp, over = {}) {
  const base = {
    razonSocial: emp.razonSocial || '',
    rif: emp.rif || '',
    nombre: emp.nombre || '',
    direccion: emp.direccion || '',
    telefono: emp.telefono || '',
    email: emp.email || '',
    logo: emp.logo || '',
    logoBlanco: emp.logoBlanco || '',
    logoNegro: emp.logoNegro || '',
    colorMarca: emp.colorMarca || '',
    logoAlterno: emp.logoAlterno || '',
    exigirCedulaCliente: !!emp.exigirCedulaCliente,
    bannerSuperior: emp.bannerSuperior || '',
    bannerLateral: emp.bannerLateral || '',
    pantallaClienteModo: emp.pantallaClienteModo || 'solo_productos',
    publicidad: normPublicidad(emp.publicidad),
    temaPantalla: emp.temaPantalla || {},
  }
  const out = { ...base, ...over }
  // Recorta strings de identidad de nivel superior si vinieron en `over`.
  return out
}

/* --- Datos de empresa ---------------------------------------------------- */

// BannerCampo: un uploader de imagen para la pantalla del cliente, con el mismo
// patrón que el Logo (URL + vista previa) más un botón para quitarla. `aspecto`
// controla la forma de la vista previa (horizontal el superior, vertical el
// lateral) para que se parezca a como se verá en la pantalla auxiliar.
function BannerCampo({ etiqueta, hint, placeholder, valor, puedeEditar, aspecto, onChange, onQuitar }) {
  const url = (valor || '').trim()
  return (
    <div className="space-y-2">
      <Field label={etiqueta} hint={hint}>
        <div className="flex items-center gap-2">
          <Input value={valor} placeholder={placeholder} disabled={!puedeEditar}
            onChange={(e) => onChange(e.target.value)} />
          {puedeEditar && url ? (
            <Button variant="ghost" size="sm" icon={<Icon.Trash size={15} />} onClick={onQuitar}>Quitar</Button>
          ) : null}
        </div>
      </Field>
      {url ? (
        <div className="flex items-center gap-2.5">
          <img src={url} alt={etiqueta} className={`${aspecto} rounded-lg object-cover border border-slate-200 dark:border-slate-700 bg-white`}
            onError={(e) => { e.currentTarget.style.display = 'none' }} />
          <span className="text-[11.5px] text-slate-400">Vista previa</span>
        </div>
      ) : null}
    </div>
  )
}

// ColorCampo: selector de color reutilizable (hex). Combina el <input type="color">
// nativo con un campo de texto para pegar/teclear el hex y un botón para volver al
// default. Vacío = usar el `defecto` (que hereda del fondo general o del color de
// marca). El backend valida/normaliza el hex. Lo usa el editor de tema.
const HEX_RE = /^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/
function ColorCampo({ label, hint, valor, puedeEditar, onChange, defecto = '#152c61', placeholder = '#RRGGBB' }) {
  const v = (valor || '').trim()
  const valido = !v || HEX_RE.test(v)
  // Para el <input type="color"> se necesita un #rrggbb; si el valor aún no es
  // válido, se muestra el `defecto` sin forzarlo sobre el texto del usuario.
  const swatch = valido && v ? (v.startsWith('#') ? v : `#${v}`) : defecto
  return (
    <Field label={label} hint={hint} error={valido ? '' : 'Usa un color hex, p. ej. #09B69B.'}>
      <div className="flex items-center gap-2.5">
        <input type="color" value={swatch} disabled={!puedeEditar}
          onChange={(e) => onChange(e.target.value)}
          className="h-9 w-12 shrink-0 rounded-lg border border-slate-200 dark:border-slate-700 bg-white p-0.5 cursor-pointer disabled:cursor-default"
          aria-label={`Elegir ${label}`} title={`Elegir ${label}`} />
        <Input value={valor || ''} placeholder={placeholder} disabled={!puedeEditar} invalid={!valido}
          onChange={(e) => onChange(e.target.value)} />
        {puedeEditar && v ? (
          <Button variant="ghost" size="sm" icon={<Icon.Trash size={15} />} onClick={() => onChange('')}>Default</Button>
        ) : null}
      </div>
    </Field>
  )
}

// Los tres modos de la pantalla del cliente del POS y su explicación. El valor
// coincide con empresa.pantallaClienteModo del backend.
const MODOS_PANTALLA = [
  { id: 'solo_productos', label: 'Solo productos y precios', desc: 'La pantalla muestra el carrito y los totales. Sin publicidad; en reposo, la bienvenida del comercio.' },
  { id: 'mixta', label: 'Mixta (productos + publicidad)', desc: 'El carrito con un panel de publicidad al lado durante la venta, y el carrusel a pantalla grande cuando no hay venta.' },
  { id: 'publicidad', label: 'Publicidad a pantalla completa', desc: 'Toda la pantalla es publicidad, siempre (cartelería). No muestra el carrito. Sin slides, la bienvenida.' },
]

// ModoPantallaCampo: selector del modo de la pantalla del cliente (tres tarjetas
// tipo radio con su explicación). El backend valida el modo; vacío = solo productos.
function ModoPantallaCampo({ valor, puedeEditar, onChange }) {
  const actual = MODOS_PANTALLA.some((m) => m.id === valor) ? valor : 'solo_productos'
  return (
    <div className="space-y-2">
      <div>
        <div className="text-[13px] font-medium text-slate-700 dark:text-slate-200">Modo de la pantalla</div>
        <div className="text-[12px] text-slate-500 dark:text-slate-400 mt-0.5">
          Qué ve el cliente en la segunda pantalla de la caja. Puedes cambiarlo cuando quieras.
        </div>
      </div>
      <div className="grid gap-2 sm:grid-cols-3">
        {MODOS_PANTALLA.map((m) => {
          const activo = actual === m.id
          return (
            <button key={m.id} type="button" disabled={!puedeEditar}
              onClick={() => onChange(m.id)}
              className={`text-left rounded-lg border p-2.5 transition-colors ${activo
                ? 'border-teal-500 bg-teal-50/70 dark:bg-teal-900/20 ring-1 ring-teal-500'
                : 'border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600'} ${puedeEditar ? '' : 'opacity-70 cursor-default'}`}>
              <div className="flex items-center gap-1.5">
                <span className={`h-3.5 w-3.5 rounded-full border-2 shrink-0 ${activo ? 'border-teal-500 bg-teal-500' : 'border-slate-300 dark:border-slate-600'}`} />
                <span className="text-[12.5px] font-semibold text-slate-700 dark:text-slate-200">{m.label}</span>
              </div>
              <div className="text-[11.5px] text-slate-500 dark:text-slate-400 mt-1 leading-snug">{m.desc}</div>
            </button>
          )
        })}
      </div>
    </div>
  )
}

// SlidesCampo: editor de los slides del carrusel de publicidad. Cada fila es un
// slide de TEXTO (mensaje de marca: texto + subtexto opcional) o de IMAGEN (URL o
// data URI, con vista previa). Se puede agregar, quitar y reordenar; el orden de la
// lista es el orden en que rotan. Estos slides se usan en los modos Mixta y
// Publicidad a pantalla completa.
function SlidesCampo({ valor, puedeEditar, onChange }) {
  const lista = Array.isArray(valor) ? valor : []
  const setEn = (i, patch) => onChange(lista.map((s, k) => (k === i ? { ...s, ...patch } : s)))
  const quitar = (i) => onChange(lista.filter((_, k) => k !== i))
  const mover = (i, d) => {
    const j = i + d
    if (j < 0 || j >= lista.length) return
    const copia = lista.slice()
    ;[copia[i], copia[j]] = [copia[j], copia[i]]
    onChange(copia)
  }
  const agregarTexto = () => onChange([...lista, { tipo: 'texto', texto: '', subtexto: '' }])
  const agregarImagen = () => onChange([...lista, { tipo: 'imagen', imagen: '' }])

  return (
    <div className="space-y-2.5">
      <div>
        <div className="text-[13px] font-medium text-slate-700 dark:text-slate-200">Slides de publicidad</div>
        <div className="text-[12px] text-slate-500 dark:text-slate-400 mt-0.5">
          Se muestran en la pantalla del cliente en los modos <strong>Mixta</strong> y <strong>Publicidad a pantalla completa</strong>,
          y <strong>rotan solos</strong> en el orden de la lista (con uno solo, queda fijo). Cada slide puede ser de
          <strong> texto</strong> (un mensaje de marca) o de <strong>imagen</strong> (horizontal, tipo 16:9, se ve mejor).
        </div>
      </div>
      {lista.length === 0 ? (
        <div className="text-[12px] text-slate-400 rounded-lg border border-dashed border-slate-300 dark:border-slate-700 px-3 py-3">
          Sin slides. En los modos Mixta y Publicidad se mostrará solo la bienvenida (logo y nombre del comercio).
        </div>
      ) : (
        <div className="space-y-2">
          {lista.map((slide, i) => {
            const esImagen = slide?.tipo === 'imagen'
            return (
              <div key={i} className="rounded-lg border border-slate-200 dark:border-slate-700 p-2.5 space-y-2">
                <div className="flex items-center gap-2">
                  <span className="text-[11px] font-semibold text-slate-400 w-5 shrink-0">{i + 1}</span>
                  <Segmented size="sm" value={esImagen ? 'imagen' : 'texto'}
                    options={[{ value: 'texto', label: 'Texto' }, { value: 'imagen', label: 'Imagen' }]}
                    onChange={(v) => puedeEditar && setEn(i, { tipo: v })} />
                  <div className="flex-1" />
                  {puedeEditar ? (
                    <div className="flex items-center gap-0.5 shrink-0">
                      <Button variant="ghost" size="sm" icon={<Icon.ArrowUp size={14} />} disabled={i === 0} onClick={() => mover(i, -1)} title="Subir" />
                      <Button variant="ghost" size="sm" icon={<Icon.ArrowDown size={14} />} disabled={i === lista.length - 1} onClick={() => mover(i, 1)} title="Bajar" />
                      <Button variant="ghost" size="sm" icon={<Icon.Trash size={15} />} onClick={() => quitar(i)}>Quitar</Button>
                    </div>
                  ) : null}
                </div>
                {esImagen ? (
                  <SubidorImagen value={slide?.imagen || ''} disabled={!puedeEditar}
                    onChange={(u) => setEn(i, { imagen: u })} carpeta="publicidad" />
                ) : (
                  <div className="space-y-2">
                    <Input value={slide?.texto || ''} placeholder="Ej. 2x1 en bebidas" disabled={!puedeEditar}
                      onChange={(e) => setEn(i, { texto: e.target.value })} />
                    <Input value={slide?.subtexto || ''} placeholder="Subtexto opcional — Ej. Solo esta semana" disabled={!puedeEditar}
                      onChange={(e) => setEn(i, { subtexto: e.target.value })} />
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
      {puedeEditar ? (
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" icon={<Icon.Plus size={15} />} onClick={agregarTexto}>Slide de texto</Button>
          <Button variant="ghost" size="sm" icon={<Icon.Plus size={15} />} onClick={agregarImagen}>Slide de imagen</Button>
        </div>
      ) : null}
    </div>
  )
}

function DatosEmpresa() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const { activeEmpresaId, refresh } = useAuth()
  const toast = useToast()
  const emp = db.EMPRESA || {}
  const puedeEditar = PUEDE_ADMINISTRAR.includes(ui.rol)

  // Datos de empresa edita SOLO la identidad legal + el logo base. El comportamiento
  // del POS (exigir cédula) vive en Configuración › Punto de venta; la APARIENCIA
  // (marca, tema, logos de contraste) y la pantalla del cliente (modo, banners, slides)
  // en Configuración › Marketing. Todos esos campos siguen viviendo en la empresa y se
  // conservan tal cual al guardar aquí (ver construirPayloadEmpresa, que parte de `emp`).
  const inicial = useCallback(() => ({
    razonSocial: emp.razonSocial || '',
    rif: emp.rif || '',
    nombre: emp.nombre || '',
    direccion: emp.direccion || '',
    telefono: emp.telefono || '',
    email: emp.email || '',
    logo: emp.logo || '',
  }), [emp.razonSocial, emp.rif, emp.nombre, emp.direccion, emp.telefono, emp.email, emp.logo])

  const [f, setF] = useState(inicial)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  // Al recargar el bootstrap, resembrar el formulario con lo que hay guardado.
  useEffect(() => { setF(inicial()) }, [inicial])

  const set = (campo, valor) => { setF((s) => ({ ...s, [campo]: valor })); setError('') }
  const emailOK = !f.email.trim() || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(f.email.trim())
  const cambio = JSON.stringify(f) !== JSON.stringify(inicial())

  const guardar = async () => {
    if (!f.rif.trim()) { setError('El RIF es obligatorio.'); return }
    if (!emailOK) { setError('El correo no tiene un formato válido.'); return }
    setBusy(true); setError('')
    try {
      // Full-replace: se envía el set completo (construirPayloadEmpresa preserva la
      // apariencia, la pantalla del cliente y el comportamiento del POS partiendo de
      // `emp`), sobreescribiendo solo la identidad.
      await api.actualizarEmpresa(activeEmpresaId, construirPayloadEmpresa(emp, {
        razonSocial: f.razonSocial.trim(),
        rif: f.rif.trim(),
        nombre: f.nombre.trim(),
        direccion: f.direccion.trim(),
        telefono: f.telefono.trim(),
        email: f.email.trim(),
        logo: f.logo.trim(),
      }))
      // Recargar bootstrap (db.EMPRESA) y el contexto de auth (/api/me) para que
      // la cabecera y el selector de empresa del sidebar reflejen el nombre nuevo.
      await Promise.all([reload(), refresh()])
      toast({ title: 'Datos de empresa guardados', body: 'La cabecera de tus facturas ya usa estos datos.' })
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
    } finally { setBusy(false) }
  }

  return (
    <div className="max-w-2xl">
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3.5">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
          Estos datos forman la <strong>cabecera legal de tus facturas</strong>. El RIF y la razón social los exige
          el SENIAT; el nombre comercial es el que ve tu cliente.
        </div>
        <Field label="Razón social" hint="nombre legal de la empresa">
          <Input value={f.razonSocial} placeholder="Ej. Distribuidora El Sol, C.A."
            disabled={!puedeEditar} onChange={(e) => set('razonSocial', e.target.value)} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="RIF" required error={error && !f.rif.trim() ? error : ''}>
            <Input value={f.rif} placeholder="J-12345678-9" invalid={!!error && !f.rif.trim()}
              disabled={!puedeEditar} onChange={(e) => set('rif', e.target.value)} />
          </Field>
          <Field label="Nombre comercial" hint="el que ve tu cliente">
            <Input value={f.nombre} placeholder="Ej. El Sol"
              disabled={!puedeEditar} onChange={(e) => set('nombre', e.target.value)} />
          </Field>
        </div>
        <Field label="Dirección fiscal">
          <Input value={f.direccion} placeholder="Av. Principal, Local 3, Caracas"
            disabled={!puedeEditar} onChange={(e) => set('direccion', e.target.value)} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Teléfono">
            <Input value={f.telefono} placeholder="0212-5551234"
              disabled={!puedeEditar} onChange={(e) => set('telefono', e.target.value)} />
          </Field>
          <Field label="Correo" error={error && !emailOK ? error : ''}>
            <Input type="email" value={f.email} placeholder="facturacion@empresa.com" invalid={!!error && !emailOK}
              disabled={!puedeEditar} onChange={(e) => set('email', e.target.value)} />
          </Field>
        </div>
        <Field label="Logo (URL)" hint="pega la URL de tu logo; la subida de archivo llega después">
          <Input value={f.logo} placeholder="https://…/logo.png"
            disabled={!puedeEditar} onChange={(e) => set('logo', e.target.value)} />
        </Field>
        {f.logo.trim() ? (
          <div className="flex items-center gap-2.5">
            <img src={f.logo.trim()} alt="Logo" className="h-11 w-11 rounded-lg object-contain border border-slate-200 dark:border-slate-700 bg-white"
              onError={(e) => { e.currentTarget.style.display = 'none' }} />
            <span className="text-[11.5px] text-slate-400">Vista previa</span>
          </div>
        ) : null}

        {/* La apariencia de la pantalla del cliente (color de marca, tema, logos de
            contraste, modo, banners y slides) se configura en Configuración › Marketing,
            y el comportamiento del POS (exigir cédula) en Configuración › Punto de venta.
            Todos esos campos siguen viviendo en la empresa, así que se conservan tal cual
            al guardar aquí (construirPayloadEmpresa parte de `emp`); esta pantalla queda
            solo con la identidad legal y el logo base. */}

        {error && f.rif.trim() && emailOK ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
        {puedeEditar ? (
          <div className="flex justify-end pt-1">
            <Button onClick={guardar} loading={busy} disabled={!cambio} icon={<Icon.Check size={16} />}>Guardar cambios</Button>
          </div>
        ) : null}
      </div>

      {!puedeEditar ? (
        <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>Solo la Dueña/Admin (o el Desarrollador) edita los datos de la empresa. Aquí los ves, pero no puedes cambiarlos.</span>
        </div>
      ) : null}
    </div>
  )
}

/* --- Punto de venta ------------------------------------------------------ */

// PuntoVenta: comportamiento del puesto de cobro (no identidad ni apariencia).
// Hoy aloja «Exigir cédula del cliente antes de cobrar» (antes mal ubicado en
// Datos de empresa). Se guarda en la empresa con `actualizarEmpresa`
// (exigirCedulaCliente); construirPayloadEmpresa parte de `emp`, así que preserva
// identidad, apariencia y pantalla del cliente. El PIN de supervisor vive en
// «Cajas y sesiones» y aquí solo se referencia (no se duplica).
function PuntoVenta() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const { activeEmpresaId } = useAuth()
  const toast = useToast()
  const emp = db.EMPRESA || {}
  const puedeEditar = PUEDE_ADMINISTRAR.includes(ui.rol)

  const inicial = useCallback(() => ({
    exigirCedulaCliente: !!emp.exigirCedulaCliente,
  }), [emp.exigirCedulaCliente])

  const [f, setF] = useState(inicial)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { setF(inicial()) }, [inicial])
  const set = (campo, valor) => { setF((s) => ({ ...s, [campo]: valor })); setError('') }
  const cambio = JSON.stringify(f) !== JSON.stringify(inicial())

  const guardar = async () => {
    setBusy(true); setError('')
    try {
      // Full-replace: construirPayloadEmpresa preserva identidad + apariencia +
      // pantalla del cliente partiendo de `emp`; aquí solo cambia el POS.
      await api.actualizarEmpresa(activeEmpresaId, construirPayloadEmpresa(emp, {
        exigirCedulaCliente: !!f.exigirCedulaCliente,
      }))
      await reload()
      toast({ title: 'Punto de venta guardado', body: 'El puesto de cobro ya usa esta configuración.' })
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
    } finally { setBusy(false) }
  }

  return (
    <div className="max-w-2xl">
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3.5">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
          Cómo se comporta el <strong>puesto de cobro</strong> al vender. Estos ajustes rigen para toda la empresa; los
          datos de cada caja y su turno se administran en <strong>Cajas y sesiones</strong>.
        </div>

        {/* Exigir identificación del cliente antes de cobrar (comportamiento del POS). */}
        <div className="rounded-lg border border-slate-200 dark:border-slate-800 px-3 py-2.5">
          <Toggle checked={!!f.exigirCedulaCliente} onChange={(v) => puedeEditar && set('exigirCedulaCliente', v)}
            disabled={!puedeEditar}
            label="Exigir cédula del cliente antes de cobrar"
            sub="Exige identificar al cliente (cédula/RIF) antes de cobrar en el punto de venta. Apagado, la venta es a Consumidor final por defecto." />
        </div>

        {/* Nota (no duplica): el PIN de supervisor se activa y asigna en Cajas y sesiones. */}
        <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3 py-2.5 text-[12px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>
            El <strong>PIN de supervisor</strong> (autorizar quitar líneas, vaciar el carrito o salir del modo caja) se
            activa y se asigna a los cajeros en <strong>Configuración › Cajas y sesiones</strong>.
          </span>
        </div>

        {error ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
        {puedeEditar ? (
          <div className="flex justify-end pt-1">
            <Button onClick={guardar} loading={busy} disabled={!cambio} icon={<Icon.Check size={16} />}>Guardar cambios</Button>
          </div>
        ) : null}
      </div>

      {!puedeEditar ? (
        <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>Solo la Dueña/Admin (o el Desarrollador) cambia estos ajustes. Aquí los ves, pero no puedes editarlos.</span>
        </div>
      ) : null}
    </div>
  )
}

/* --- Marketing ----------------------------------------------------------- */

// Versiones del logo que la pantalla del cliente puede mostrar (coincide con
// empresa.TemaPantalla.logoVersion del backend: color|blanco|negro).
const VERSIONES_LOGO = [
  { id: 'color', label: 'A color' },
  { id: 'blanco', label: 'Blanco' },
  { id: 'negro', label: 'Negro' },
]

// LogoCampo: uploader de una versión del logo con una etiqueta de contraste. `sobre`
// pinta la vista previa sobre un fondo claro u oscuro para juzgar el contraste real.
function LogoCampo({ label, hint, valor, puedeEditar, onChange, sobre = 'oscuro' }) {
  const fondo = sobre === 'oscuro' ? 'bg-slate-800' : 'bg-white'
  return (
    <div className="space-y-1.5">
      <div>
        <div className="text-[12.5px] font-medium text-slate-700 dark:text-slate-200">{label}</div>
        {hint ? <div className="text-[11.5px] text-slate-500 dark:text-slate-400">{hint}</div> : null}
      </div>
      <div className={`rounded-lg border border-slate-200 dark:border-slate-700 ${fondo} h-16 flex items-center justify-center px-3`}>
        {(valor || '').trim() ? (
          <img src={(valor || '').trim()} alt={label} className="max-h-12 w-auto object-contain"
            onError={(e) => { e.currentTarget.style.visibility = 'hidden' }} />
        ) : (
          <span className="text-[11px] text-slate-400">Sin logo</span>
        )}
      </div>
      <SubidorImagen value={valor || ''} disabled={!puedeEditar} onChange={onChange} carpeta="empresa" />
    </div>
  )
}

// PantallaClienteConfig: EDITOR DE TEMA de la segunda pantalla del POS. Unifica en
// un solo lugar toda la apariencia que antes estaba dispersa (color de marca en
// Datos de empresa; color de fondo / versión de logo por caja): fondos general y
// por espacio, colores de texto y de énfasis, las tres versiones de logo y cuál se
// muestra, más el modo, los banners y los slides. Tiene VISTA PREVIA EN VIVO. Se
// guarda en la empresa con `actualizarEmpresa` (full-replace: construirPayloadEmpresa
// preserva la identidad legal que se edita en Datos de empresa).
function PantallaClienteConfig() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const { activeEmpresaId } = useAuth()
  const toast = useToast()
  const emp = db.EMPRESA || {}
  const puedeEditar = PUEDE_ADMINISTRAR.includes(ui.rol)

  const inicial = useCallback(() => {
    const t = emp.temaPantalla || {}
    return {
      pantallaClienteModo: emp.pantallaClienteModo || 'solo_productos',
      bannerSuperior: emp.bannerSuperior || '',
      bannerLateral: emp.bannerLateral || '',
      // Slides del carrusel. Se tolera la forma antigua ([]string de URLs): cada
      // string se lee como un slide de imagen.
      publicidad: (Array.isArray(emp.publicidad) ? emp.publicidad : []).map((s) => (
        typeof s === 'string' ? { tipo: 'imagen', imagen: s } : { tipo: 'texto', texto: '', subtexto: '', imagen: '', ...s }
      )),
      // Logos por versión (el de color es el base de Datos de empresa; se puede
      // ajustar también aquí porque escribe el mismo campo).
      logo: emp.logo || '',
      logoBlanco: emp.logoBlanco || '',
      logoNegro: emp.logoNegro || '',
      // Tema: fondos, colores y versión de logo.
      fondo: t.fondo || '',
      fondoCabecera: t.fondoCabecera || '',
      fondoProductos: t.fondoProductos || '',
      fondoTotales: t.fondoTotales || '',
      fondoPublicidad: t.fondoPublicidad || '',
      colorTexto: t.colorTexto || '',
      // Énfasis: si aún no hay uno nuevo, se precarga con el color de marca previo.
      colorEnfasis: t.colorEnfasis || emp.colorMarca || '',
      logoVersion: t.logoVersion || 'color',
    }
  }, [emp.bannerSuperior, emp.bannerLateral, emp.pantallaClienteModo, emp.publicidad, emp.logo, emp.logoBlanco, emp.logoNegro, emp.colorMarca, emp.temaPantalla])

  const [f, setF] = useState(inicial)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { setF(inicial()) }, [inicial])
  const set = (campo, valor) => { setF((s) => ({ ...s, [campo]: valor })); setError('') }
  const cambio = JSON.stringify(f) !== JSON.stringify(inicial())

  // Empresa sintética para la vista previa: refleja lo que se está editando SIN
  // guardar. resolverTema tolera hex a medio teclear (CSS ignora lo inválido).
  const temaPreview = resolverTema({
    ...emp,
    logo: f.logo, logoBlanco: f.logoBlanco, logoNegro: f.logoNegro,
    temaPantalla: {
      fondo: f.fondo, fondoCabecera: f.fondoCabecera, fondoProductos: f.fondoProductos,
      fondoTotales: f.fondoTotales, fondoPublicidad: f.fondoPublicidad,
      colorTexto: f.colorTexto, colorEnfasis: f.colorEnfasis, logoVersion: f.logoVersion,
    },
  })

  const guardar = async () => {
    setBusy(true); setError('')
    try {
      // Full-replace: construirPayloadEmpresa preserva la identidad legal (editada en
      // Datos de empresa) y aquí sobreescribimos la apariencia + la pantalla del cliente.
      await api.actualizarEmpresa(activeEmpresaId, construirPayloadEmpresa(emp, {
        logo: (f.logo || '').trim(),
        logoBlanco: (f.logoBlanco || '').trim(),
        logoNegro: (f.logoNegro || '').trim(),
        pantallaClienteModo: f.pantallaClienteModo || 'solo_productos',
        bannerSuperior: (f.bannerSuperior || '').trim(),
        bannerLateral: (f.bannerLateral || '').trim(),
        publicidad: normPublicidad(f.publicidad),
        // El backend normaliza cada hex (inválido → '' = heredar/default) y acota la
        // versión de logo, así que se puede mandar tal cual.
        temaPantalla: {
          fondo: f.fondo || '', fondoCabecera: f.fondoCabecera || '',
          fondoProductos: f.fondoProductos || '', fondoTotales: f.fondoTotales || '',
          fondoPublicidad: f.fondoPublicidad || '', colorTexto: f.colorTexto || '',
          colorEnfasis: f.colorEnfasis || '', logoVersion: f.logoVersion || 'color',
        },
      }))
      await reload()
      toast({ title: 'Pantalla del cliente guardada', body: 'El punto de venta ya usa esta configuración.' })
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
    } finally { setBusy(false) }
  }

  return (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-4">
      <div>
        <div className="text-[13px] font-semibold text-slate-700 dark:text-slate-200">Pantalla del cliente</div>
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 mt-0.5">
          La <strong>segunda pantalla que ve el cliente en la caja</strong>: su apariencia (colores y logo), qué muestra y
          la publicidad de tu comercio. Los cambios se ven en la <strong>vista previa</strong> al instante.
        </div>
      </div>

      {/* Editor de tema + vista previa en vivo, lado a lado en pantallas anchas. */}
      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,420px)]">
        {/* Controles */}
        <div className="space-y-4 min-w-0">
          {/* Colores */}
          <div className="space-y-3">
            <div className="text-[12.5px] font-semibold text-slate-700 dark:text-slate-200">Colores</div>
            <ColorCampo label="Fondo general" hint="el fondo de toda la pantalla — vacío usa el degradado navy de marca"
              valor={f.fondo} puedeEditar={puedeEditar} onChange={(v) => set('fondo', v)} defecto="#152c61" />
            <div className="grid sm:grid-cols-2 gap-x-3 gap-y-2">
              <ColorCampo label="Fondo · cabecera" hint="vacío = hereda el general" valor={f.fondoCabecera}
                puedeEditar={puedeEditar} onChange={(v) => set('fondoCabecera', v)} defecto="#152c61" />
              <ColorCampo label="Fondo · productos" hint="vacío = hereda el general" valor={f.fondoProductos}
                puedeEditar={puedeEditar} onChange={(v) => set('fondoProductos', v)} defecto="#152c61" />
              <ColorCampo label="Fondo · totales" hint="vacío = hereda el general" valor={f.fondoTotales}
                puedeEditar={puedeEditar} onChange={(v) => set('fondoTotales', v)} defecto="#152c61" />
              <ColorCampo label="Fondo · publicidad" hint="vacío = hereda el general" valor={f.fondoPublicidad}
                puedeEditar={puedeEditar} onChange={(v) => set('fondoPublicidad', v)} defecto="#152c61" />
            </div>
            <div className="grid sm:grid-cols-2 gap-x-3 gap-y-2">
              <ColorCampo label="Texto" hint="texto principal — vacío = blanco" valor={f.colorTexto}
                puedeEditar={puedeEditar} onChange={(v) => set('colorTexto', v)} defecto="#ffffff" placeholder="#FFFFFF" />
              <ColorCampo label="Énfasis" hint="total, RIF y acentos — vacío = teal de marca" valor={f.colorEnfasis}
                puedeEditar={puedeEditar} onChange={(v) => set('colorEnfasis', v)} defecto="#09b69b" placeholder="#09B69B" />
            </div>
          </div>

          {/* Logos por versión */}
          <div className="space-y-3 pt-1 border-t border-slate-200 dark:border-slate-800">
            <div>
              <div className="text-[12.5px] font-semibold text-slate-700 dark:text-slate-200">Logo</div>
              <div className="text-[11.5px] text-slate-500 dark:text-slate-400 mt-0.5">
                Carga hasta tres versiones para asegurar el contraste según el fondo. El logo <strong>a color</strong> es el de tu
                identidad (también va en los comprobantes; puedes editarlo aquí o en Datos de empresa).
              </div>
            </div>
            <div className="grid sm:grid-cols-3 gap-3">
              <LogoCampo label="A color" hint="el de identidad" valor={f.logo} puedeEditar={puedeEditar}
                onChange={(u) => set('logo', u)} sobre="oscuro" />
              <LogoCampo label="Blanco" hint="para fondos oscuros" valor={f.logoBlanco} puedeEditar={puedeEditar}
                onChange={(u) => set('logoBlanco', u)} sobre="oscuro" />
              <LogoCampo label="Negro" hint="para fondos claros" valor={f.logoNegro} puedeEditar={puedeEditar}
                onChange={(u) => set('logoNegro', u)} sobre="claro" />
            </div>
            <Field label="Versión que se muestra" hint="qué logo aparece arriba a la izquierda de la pantalla del cliente">
              <Segmented value={f.logoVersion || 'color'} disabled={!puedeEditar}
                options={VERSIONES_LOGO.map((v) => ({ value: v.id, label: v.label }))}
                onChange={(v) => puedeEditar && set('logoVersion', v)} />
            </Field>
          </div>

          {/* Modo + banners + slides (ya vivían aquí) */}
          <div className="space-y-3.5 pt-1 border-t border-slate-200 dark:border-slate-800">
            <ModoPantallaCampo valor={f.pantallaClienteModo} puedeEditar={puedeEditar}
              onChange={(v) => set('pantallaClienteModo', v)} />
            <BannerCampo etiqueta="Banner superior" hint="horizontal, arriba de la pantalla del cliente"
              placeholder="https://…/banner-superior.png" valor={f.bannerSuperior} puedeEditar={puedeEditar}
              aspecto="aspect-[4/1]" onChange={(v) => set('bannerSuperior', v)} onQuitar={() => set('bannerSuperior', '')} />
            <BannerCampo etiqueta="Banner lateral" hint="vertical, al costado de la pantalla del cliente"
              placeholder="https://…/banner-lateral.png" valor={f.bannerLateral} puedeEditar={puedeEditar}
              aspecto="aspect-[3/4] w-28" onChange={(v) => set('bannerLateral', v)} onQuitar={() => set('bannerLateral', '')} />
            <SlidesCampo valor={f.publicidad || []} puedeEditar={puedeEditar}
              onChange={(lista) => set('publicidad', lista)} />
          </div>
        </div>

        {/* Vista previa en vivo (pegada arriba al hacer scroll en pantallas anchas). */}
        <div className="lg:sticky lg:top-4 self-start space-y-1.5">
          <div className="text-[11.5px] font-medium text-slate-500 dark:text-slate-400 uppercase tracking-wide">Vista previa</div>
          <MaquetaPantallaCliente tema={temaPreview} empresa={emp} />
          <div className="text-[11px] text-slate-400">
            Maqueta a escala. El logo que se muestra es la versión «{VERSIONES_LOGO.find((v) => v.id === (f.logoVersion || 'color'))?.label}».
          </div>
        </div>
      </div>

      {error ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
      {puedeEditar ? (
        <div className="flex justify-end pt-1">
          <Button onClick={guardar} loading={busy} disabled={!cambio} icon={<Icon.Check size={16} />}>Guardar cambios</Button>
        </div>
      ) : null}
    </div>
  )
}

// Marketing: unifica el espacio promocional en un solo lugar. Dos bloques —
// «Pantalla del cliente» (editor de tema + config de la segunda pantalla, guardada
// en la empresa) y «Promociones» (la biblioteca de anuncios del carrusel). Antes
// estaban repartidos entre Datos de empresa y Ventas › Promociones.
// Marketing (módulo): la pantalla del cliente (tema/branding + slides) y la
// biblioteca de Promociones. Toda esta pestaña se muestra SOLO si el módulo Marketing
// está activo — el gating vive a nivel de pestaña (submenú + Configuracion), no aquí.
function Marketing() {
  return (
    <div className="space-y-8">
      <section>
        <PantallaClienteConfig />
      </section>
      <section>
        <div className="mb-3">
          <div className="text-[13px] font-semibold text-slate-700 dark:text-slate-200">Promociones</div>
          <div className="text-[12.5px] text-slate-500 dark:text-slate-400 mt-0.5">
            Biblioteca de anuncios (de imagen o de texto, con vigencia) que alimentan el carrusel de la pantalla del
            cliente cuando están vigentes. Se combinan con los slides manuales de arriba.
          </div>
        </div>
        <Promociones />
      </section>
    </div>
  )
}

/* --- Sedes --------------------------------------------------------------- */

function Sedes() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const { activeEmpresaId, refresh } = useAuth()
  const toast = useToast()
  const confirm = useConfirm()
  const puedeEditar = PUEDE_ADMINISTRAR.includes(ui.rol)
  const [form, setForm] = useState(null)   // null | {} (nueva) | sede (edición)
  const [busyId, setBusyId] = useState('')

  const sedes = db.SEDES || []
  const activas = sedes.filter((s) => s.activa).length

  // Recargar bootstrap (db.SEDES) y auth (/api/me → selector de sede del header).
  const recargar = () => Promise.all([reload(), refresh()])

  const reactivar = async (s) => {
    setBusyId(s.id)
    try {
      await api.reactivarSede(activeEmpresaId, s.id)
      await recargar()
      toast({ title: 'Sede reactivada', body: s.nombre })
    } catch (e) {
      toast({ title: 'No se pudo reactivar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  const desactivar = async (s) => {
    const ok = await confirm({
      title: 'Desactivar sede',
      body: `«${s.nombre}» dejará de aparecer para abrir cajas y ventas. No se borra nada: su historial se conserva y puedes reactivarla cuando quieras.`,
      confirmLabel: 'Desactivar', tone: 'danger',
    })
    if (!ok) return
    setBusyId(s.id)
    try {
      await api.desactivarSede(activeEmpresaId, s.id)
      await recargar()
      toast({ title: 'Sede desactivada', body: `${s.nombre} — reversible desde aquí.` })
    } catch (e) {
      toast({ title: 'No se pudo desactivar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-xl">
          Cada sede es una tienda o local: allí viven sus existencias y sus cajas. Desactivar una sede es
          <strong> reversible</strong>; una empresa necesita al menos una activa.
        </div>
        {puedeEditar ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setForm({})}>Nueva sede</Button>
        ) : null}
      </div>

      {sedes.length === 0 ? (
        <Empty icon={<Icon.Home size={22} />} title="Sin sedes todavía"
          body="Crea la primera sede para poder abrir cajas y cargar existencias."
          cta={puedeEditar ? <Button icon={<Icon.Plus size={16} />} onClick={() => setForm({})}>Crear sede</Button> : null} />
      ) : (
        <div className="space-y-2.5">
          {sedes.map((s) => {
            const bloqueado = busyId === s.id
            const esUltimaActiva = s.activa && activas <= 1
            return (
              <div key={s.id}
                className={`bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 flex items-start gap-3 ${s.activa ? '' : 'opacity-60'}`}>
                <span className="text-slate-400 mt-0.5"><Icon.Home size={18} /></span>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-[13.5px] font-semibold truncate">{s.nombre}</span>
                    {s.activa ? <Badge size="sm" color="emerald" dot>Activa</Badge> : <Badge size="sm" color="slate">Inactiva</Badge>}
                  </div>
                  <div className="text-[12px] text-slate-500 mt-0.5">{s.direccion || <span className="text-slate-400">Sin dirección</span>}</div>
                </div>
                {puedeEditar ? (
                  <div className="flex items-center gap-1 shrink-0">
                    <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />} disabled={bloqueado} onClick={() => setForm(s)}>Editar</Button>
                    {s.activa ? (
                      <Button size="sm" variant="destructive" icon={<Icon.EyeOff size={14} />} disabled={bloqueado || esUltimaActiva}
                        title={esUltimaActiva ? 'Es la única sede activa: la empresa necesita al menos una.' : ''}
                        onClick={() => desactivar(s)}>Desactivar</Button>
                    ) : (
                      <Button size="sm" variant="ghost" icon={<Icon.Refresh size={14} />} loading={bloqueado} onClick={() => reactivar(s)}>Reactivar</Button>
                    )}
                  </div>
                ) : null}
              </div>
            )
          })}
        </div>
      )}

      {!puedeEditar ? (
        <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>Solo la Dueña/Admin (o el Desarrollador) administra las sedes. Aquí las ves, pero no puedes cambiarlas.</span>
        </div>
      ) : null}

      {form ? (
        <SedeForm sede={form.id ? form : null} empresaId={activeEmpresaId}
          onClose={() => setForm(null)}
          onSaved={(nombre, edicion) => { recargar(); toast({ title: edicion ? 'Sede actualizada' : 'Sede creada', body: nombre }) }} />
      ) : null}

    </div>
  )
}

function SedeForm({ sede, empresaId, onClose, onSaved }) {
  const edicion = !!sede
  const [f, setF] = useState({ nombre: sede?.nombre || '', direccion: sede?.direccion || '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const guardar = async () => {
    if (!f.nombre.trim()) { setError('El nombre es obligatorio.'); return }
    setBusy(true); setError('')
    const payload = { nombre: f.nombre.trim(), direccion: f.direccion.trim() }
    try {
      if (edicion) await api.actualizarSede(empresaId, sede.id, payload)
      else await api.crearSede(empresaId, payload)
      onSaved(payload.nombre, edicion)
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Home size={18} />}
      title={edicion ? 'Editar sede' : 'Nueva sede'} sub="Una tienda o local de la empresa"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre" required error={error && !f.nombre.trim() ? error : ''}>
          <Input value={f.nombre} placeholder="Ej. Sede Centro" invalid={!!error && !f.nombre.trim()}
            onChange={(e) => { setF((s) => ({ ...s, nombre: e.target.value })); setError('') }} />
        </Field>
        <Field label="Dirección" hint="opcional">
          <Input value={f.direccion} placeholder="Av. Principal, Local 3"
            onChange={(e) => setF((s) => ({ ...s, direccion: e.target.value }))} />
        </Field>
        {error && f.nombre.trim() ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
      </div>
    </Modal>
  )
}

/* --- Impuestos y alícuotas (compliance as configuration) ----------------- */

// Defaults del sistema (deben coincidir con fiscal.AlicuotaIVA / AlicuotaIGTF del
// backend). Se muestran cuando la empresa no ha configurado una tasa propia (0).
const IVA_SISTEMA = 0.16
const IGTF_SISTEMA = 0.03
// Conversión fracción↔porcentaje: se guarda 0.16, pero se edita 16.
const aPct = (frac) => Math.round((frac || 0) * 10000) / 100
const aFrac = (pct) => Math.round(Number(pct) * 100) / 10000

function Impuestos() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const { activeEmpresaId, refresh } = useAuth()
  const toast = useToast()
  const emp = db.EMPRESA || {}
  const puedeEditar = PUEDE_ADMINISTRAR.includes(ui.rol)

  // Si el campo viene 0 (o ausente), la empresa usa el valor del sistema: se
  // muestra ese default y se marca la nota.
  const ivaEsSistema = !(emp.alicuotaIVA > 0)
  const igtfEsSistema = !(emp.alicuotaIGTF > 0)
  const ivaGuardado = emp.alicuotaIVA > 0 ? emp.alicuotaIVA : IVA_SISTEMA
  const igtfGuardado = emp.alicuotaIGTF > 0 ? emp.alicuotaIGTF : IGTF_SISTEMA

  const inicial = useCallback(() => ({
    iva: String(aPct(ivaGuardado)),
    igtf: String(aPct(igtfGuardado)),
    agIVA: !!emp.agenteRetencionIVA,
    agISLR: !!emp.agenteRetencionISLR,
    retIVA: String(emp.retencionIVAPorcentaje > 0 ? emp.retencionIVAPorcentaje : 75),
  }), [ivaGuardado, igtfGuardado, emp.agenteRetencionIVA, emp.agenteRetencionISLR, emp.retencionIVAPorcentaje])

  const [f, setF] = useState(inicial)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { setF(inicial()) }, [inicial])

  const set = (campo, valor) => { setF((s) => ({ ...s, [campo]: valor })); setError('') }
  // Validación inline: número entre 0 y 100 (%).
  const rangoOK = (v) => v.trim() !== '' && !Number.isNaN(Number(v)) && Number(v) >= 0 && Number(v) <= 100
  const ivaOK = rangoOK(f.iva)
  const igtfOK = rangoOK(f.igtf)
  const cambio = JSON.stringify(f) !== JSON.stringify(inicial())

  const guardar = async () => {
    if (!ivaOK || !igtfOK) { setError('Las alícuotas van entre 0 y 100 %.'); return }
    setBusy(true); setError('')
    try {
      await api.actualizarImpuestos(activeEmpresaId, {
        alicuotaIVA: aFrac(f.iva),
        alicuotaIGTF: aFrac(f.igtf),
        agenteRetencionIVA: f.agIVA,
        agenteRetencionISLR: f.agISLR,
        retencionIVAPorcentaje: f.agIVA ? (Number(f.retIVA) || 75) : 0,
      })
      // Recargar bootstrap (db.EMPRESA) para que la nueva tasa rija en documentos
      // nuevos; refrescar auth por consistencia con las demás pantallas de empresa.
      await Promise.all([reload(), refresh()])
      toast({ title: 'Alícuotas actualizadas', body: 'Rigen para los documentos que emitas de ahora en adelante.' })
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
    } finally { setBusy(false) }
  }

  return (
    <div className="max-w-2xl">
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3.5">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
          Las alícuotas de <strong>IVA</strong> e <strong>IGTF</strong> son configurables por empresa: así una nueva
          providencia del SENIAT se aplica sin actualizar el sistema. Se guardan en fracción y se editan en porcentaje.
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Alícuota de IVA (%)" required
            hint={ivaEsSistema ? 'valor del sistema' : 'configurada por tu empresa'}
            error={error && !ivaOK ? error : ''}>
            <Input type="number" min="0" max="100" step="0.01" value={f.iva} invalid={!!error && !ivaOK}
              disabled={!puedeEditar} onChange={(e) => set('iva', e.target.value)} />
          </Field>
          <Field label="Alícuota de IGTF (%)" required
            hint={igtfEsSistema ? 'valor del sistema' : 'configurada por tu empresa'}
            error={error && !igtfOK ? error : ''}>
            <Input type="number" min="0" max="100" step="0.01" value={f.igtf} invalid={!!error && !igtfOK}
              disabled={!puedeEditar} onChange={(e) => set('igtf', e.target.value)} />
          </Field>
        </div>
        <div className="pt-3 border-t border-slate-100 dark:border-slate-800">
          <div className="text-[13px] font-semibold">Agente de retención</div>
          <div className="text-[12px] text-slate-500 dark:text-slate-400 mt-0.5 mb-2.5">
            Actívalo si tu empresa <strong>retiene</strong> impuestos a sus proveedores (contribuyente especial). Al
            registrar una retención <strong>emitida</strong>, el número de comprobante se genera automáticamente con el
            formato SENIAT (AAAAMM + secuencia).
          </div>
          <div className="space-y-2.5">
            <Toggle checked={f.agIVA} onChange={(v) => puedeEditar && set('agIVA', v)}
              label="Agente de retención de IVA" sub="Retienes IVA a tus proveedores sobre su factura de compra" />
            {f.agIVA ? (
              <div className="pl-12 max-w-[260px]">
                <Field label="% de retención de IVA por defecto" hint="75 % o 100 %; se puede ajustar por comprobante">
                  <Input type="number" min="0" max="100" step="1" value={f.retIVA}
                    disabled={!puedeEditar} onChange={(e) => set('retIVA', e.target.value)} />
                </Field>
              </div>
            ) : null}
            <Toggle checked={f.agISLR} onChange={(v) => puedeEditar && set('agISLR', v)}
              label="Agente de retención de ISLR" sub="Retienes ISLR (el % y el sustraendo dependen del concepto)" />
          </div>
        </div>
        <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3 py-2.5 text-[12px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>
            El cambio rige para los <strong>documentos nuevos</strong>. Los ya emitidos conservan la tasa con la que se
            calcularon: es un requisito del SENIAT (cada documento cuadra con su propia providencia), así que una nota
            de crédito o el reporte de IGTF de una factura vieja siguen usando la tasa de esa factura.
          </span>
        </div>
        {error && ivaOK && igtfOK ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
        {puedeEditar ? (
          <div className="flex justify-end pt-1">
            <Button onClick={guardar} loading={busy} disabled={!cambio || !ivaOK || !igtfOK} icon={<Icon.Check size={16} />}>Guardar cambios</Button>
          </div>
        ) : null}
      </div>

      {!puedeEditar ? (
        <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>Solo la Dueña/Admin (o el Desarrollador) cambia las alícuotas. Aquí las ves, pero no puedes editarlas.</span>
        </div>
      ) : null}
    </div>
  )
}

/* --- Asistente IA (opt-in de la capa de IA del módulo "asistente-ia") ----- */

// Modelos ofrecidos para la capa IA (IDs de OpenRouter, cobrados vía Hubmy). El
// default es barato y rápido; el resto son alternativas conocidas. La capa MECÁNICA
// (las cifras) no usa modelo: es local y no se configura.
const MODELOS_IA = [
  { id: 'google/gemini-2.5-flash-lite', label: 'Gemini 2.5 Flash Lite — económico (recomendado)' },
  { id: 'google/gemini-2.5-flash', label: 'Gemini 2.5 Flash — más capaz' },
  { id: 'openai/gpt-4o-mini', label: 'GPT-4o mini' },
  { id: 'anthropic/claude-3.5-haiku', label: 'Claude 3.5 Haiku' },
]

function AsistenteIAConfig() {
  const { ui } = useUI()
  const toast = useToast()
  const puedeEditar = PUEDE_ADMINISTRAR.includes(ui.rol)

  const [cfg, setCfg] = useState(null)
  const [f, setF] = useState({ habilitada: false, modelo: '' })
  const [cargando, setCargando] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const cargar = useCallback(async () => {
    setCargando(true)
    try {
      const c = await api.asistenteConfig()
      setCfg(c)
      setF({ habilitada: !!c.habilitada, modelo: c.modelo || c.modeloDefault || '' })
    } catch (e) {
      setError(e?.message || 'No se pudo cargar la configuración del asistente.')
    } finally { setCargando(false) }
  }, [])
  useEffect(() => { cargar() }, [cargar])

  const set = (campo, valor) => { setF((s) => ({ ...s, [campo]: valor })); setError('') }
  const inicial = cfg ? { habilitada: !!cfg.habilitada, modelo: cfg.modelo || cfg.modeloDefault || '' } : f
  const cambio = JSON.stringify(f) !== JSON.stringify(inicial)

  const guardar = async () => {
    setBusy(true); setError('')
    try {
      const c = await api.configurarAsistente({ habilitada: f.habilitada, modelo: f.modelo })
      setCfg(c)
      setF({ habilitada: !!c.habilitada, modelo: c.modelo || c.modeloDefault || '' })
      toast({ title: 'Asistente actualizado', body: f.habilitada ? 'La IA responderá las preguntas abiertas.' : 'La IA quedó desactivada; el asistente responde solo con tus datos.' })
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
    } finally { setBusy(false) }
  }

  if (cargando) {
    return <div className="max-w-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={4} cols={2} /></div>
  }

  const iaNoDisponible = cfg && cfg.iaDisponible === false

  return (
    <div className="max-w-2xl space-y-3">
      {/* Cómo responde el asistente: las dos capas, con la mecánica siempre local. */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3.5">
        <div className="flex items-start gap-2.5">
          <span className="h-8 w-8 shrink-0 rounded-lg inline-flex items-center justify-center bg-elerp-50 text-elerp-500 dark:bg-elerp-900/50 dark:text-elerp-200">
            <Icon.Sparkles size={16} />
          </span>
          <div className="min-w-0">
            <div className="text-[13.5px] font-semibold">Cómo responde el asistente</div>
            <div className="text-[12.5px] text-slate-500 dark:text-slate-400 mt-0.5 leading-relaxed">
              El asistente responde <strong>mecánicamente</strong> —tus ventas, cartera, stock, cuentas por pagar,
              integridad del libro— con cálculos exactos del sistema. Esa capa es <strong>100% local</strong>: ningún
              dato sale de tu empresa. La <strong>capa de IA</strong> (abajo) es opcional y solo entra en las preguntas
              abiertas que la capa mecánica no cubre.
            </div>
          </div>
        </div>
      </div>

      {/* Opt-in de la IA. */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3.5">
        <Toggle checked={f.habilitada} onChange={(v) => puedeEditar && set('habilitada', v)}
          label="Habilitar respuestas con IA" sub="Para las preguntas abiertas que los datos no responden solos" />

        {/* Nota de privacidad: qué viaja y qué no. */}
        <div className="rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-900/50 px-3 py-2.5 text-[12px] text-amber-800 dark:text-amber-200 flex gap-2.5 items-start">
          <Icon.CircleAlert size={15} className="mt-0.5 shrink-0" />
          <span>
            Al activarla, las <strong>preguntas abiertas</strong> envían un <strong>resumen de tus datos acotado a tu rol</strong>
            {' '}(no el detalle) al proveedor de IA (Hubmy) para redactar la respuesta. La IA no navega internet y solo ve
            ese contexto. Las <strong>cifras</strong> siguen saliendo del sistema, nunca de la IA. La capa mecánica no se ve
            afectada: sigue siendo local.
          </span>
        </div>

        {iaNoDisponible ? (
          <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3 py-2.5 text-[12px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
            <Icon.Plug size={15} className="mt-0.5 shrink-0 text-slate-400" />
            <span>Este servidor todavía no tiene el proveedor de IA configurado (clave de Hubmy). Puedes dejar el opt-in listo, pero las preguntas abiertas no tendrán respuesta de IA hasta que se configure.</span>
          </div>
        ) : null}

        {f.habilitada ? (
          <Field label="Modelo de IA" hint="se cobra al saldo de IA de Hubmy; el económico basta para el asistente">
            <Select value={f.modelo} disabled={!puedeEditar} onChange={(e) => set('modelo', e.target.value)}>
              {MODELOS_IA.map((m) => <option key={m.id} value={m.id}>{m.label}</option>)}
            </Select>
          </Field>
        ) : null}

        {error ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}

        {puedeEditar ? (
          <div className="flex justify-end pt-1">
            <Button onClick={guardar} loading={busy} disabled={!cambio} icon={<Icon.Check size={16} />}>Guardar cambios</Button>
          </div>
        ) : (
          <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
            <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
            <span>Solo la Dueña/Admin (o el Desarrollador) configura la IA del asistente. Aquí lo ves, pero no puedes editarlo.</span>
          </div>
        )}
      </div>
    </div>
  )
}

/* --- Series y numeración fiscal (forward-only) --------------------------- */

// Nombre legible de cada serie. Los códigos los emite el backend (serieDe +
// sufijos): FL/MF/ID según modalidad, sus notas -NC/-NA, y las series propias de
// cotización (COT) y compra (OC). Uno desconocido se muestra tal cual.
const SERIE_LABEL = {
  FL: 'Factura (forma libre)',
  MF: 'Factura (máquina fiscal)',
  ID: 'Factura (imprenta digital)',
  'FL-NC': 'Nota de crédito',
  'FL-NA': 'Anulación',
  'MF-NC': 'Nota de crédito',
  'MF-NA': 'Anulación',
  'ID-NC': 'Nota de crédito',
  'ID-NA': 'Anulación',
  COT: 'Cotización',
  OC: 'Orden de compra',
}
const serieLabel = (s) => SERIE_LABEL[s] || s
// Folio con el mismo formato que el documento (serie-00000042).
const pad8 = (n) => String(n || 0).padStart(8, '0')
const folioCompleto = (serie, n) => `${serie}-${pad8(n)}`

/* --- Número de Control (rango autorizado por el SENIAT) ------------------ */
function NumeroControlConfig({ puedeEditar, toast }) {
  const [v, setV] = useState(null)
  const [f, setF] = useState({ prefijo: '00', desde: '', hasta: '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const cargar = useCallback(() => {
    api.numeroControl().then((r) => {
      setV(r)
      setF({ prefijo: r.prefijo || '00', desde: r.desde ? String(r.desde) : '', hasta: r.hasta ? String(r.hasta) : '' })
    }).catch(() => setV(null))
  }, [])
  useEffect(() => { cargar() }, [cargar])

  const guardar = async () => {
    setBusy(true); setError('')
    try {
      const r = await api.configurarNumeroControl({ prefijo: f.prefijo.trim() || '00', desde: Number(f.desde) || 0, hasta: Number(f.hasta) || 0 })
      setV(r)
      toast({ title: 'Número de Control actualizado', body: `Prefijo ${r.prefijo} · próximo ${r.ejemplo}` })
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
    } finally { setBusy(false) }
  }

  return (
    <div className="mb-4 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
      <div className="flex items-baseline justify-between gap-2 flex-wrap">
        <div className="text-[13px] font-semibold">Número de Control (SENIAT)</div>
        {v ? <div className="text-[12px] text-slate-500 num">Próximo: <b>{v.ejemplo}</b>{v.restantes >= 0 ? ` · ${fmtNum(v.restantes, 0)} restantes del rango` : ''}</div> : null}
      </div>
      <div className="text-[12px] text-slate-500 dark:text-slate-400 mt-0.5 mb-2.5">
        Correlativo <strong>propio</strong>, distinto del número de factura, que el SENIAT autoriza por <strong>rango</strong>
        (prefijo + desde/hasta). Se imprime en cada comprobante de forma libre / imprenta digital. Solo avanza.
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-2.5 max-w-xl">
        <Field label="Prefijo (2 dígitos)">
          <Input value={f.prefijo} maxLength={2} disabled={!puedeEditar} className="num"
            onChange={(e) => { setF((s) => ({ ...s, prefijo: e.target.value.replace(/\D/g, '') })); setError('') }} placeholder="00" />
        </Field>
        <Field label="Desde" hint="inicio autorizado">
          <Input type="number" min="0" value={f.desde} disabled={!puedeEditar} className="num"
            onChange={(e) => { setF((s) => ({ ...s, desde: e.target.value })); setError('') }} placeholder="1" />
        </Field>
        <Field label="Hasta" hint="0 = sin tope">
          <Input type="number" min="0" value={f.hasta} disabled={!puedeEditar} className="num"
            onChange={(e) => { setF((s) => ({ ...s, hasta: e.target.value })); setError('') }} placeholder="500000" />
        </Field>
      </div>
      {error ? <div className="text-[12px] text-red-600 dark:text-red-400 mt-2">{error}</div> : null}
      {puedeEditar ? (
        <div className="flex justify-end mt-2.5">
          <Button size="sm" onClick={guardar} loading={busy} icon={<Icon.Check size={15} />}>Guardar rango</Button>
        </div>
      ) : null}
    </div>
  )
}

/* --- Series y numeración fiscal (forward-only) --------------------------- */

function Numeracion() {
  const { ui } = useUI()
  const toast = useToast()
  const puedeEditar = PUEDE_ADMINISTRAR.includes(ui.rol)
  const [rows, setRows] = useState(null) // null = cargando; [] = vacío
  const [error, setError] = useState('')
  const [fijando, setFijando] = useState(null) // fila a fijar (abre modal)

  const cargar = useCallback(() => {
    setError('')
    api.numeracion()
      .then((r) => setRows(r || []))
      .catch((e) => { setError(e?.message || 'No se pudo cargar la numeración.'); setRows([]) })
  }, [])
  useEffect(() => { cargar() }, [cargar])

  return (
    <div>
      <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl mb-3">
        Cada <strong>serie</strong> lleva su propio contador de folios por sede. Aquí ves el último folio entregado y el
        próximo que se asignará. El próximo folio solo se puede <strong>adelantar</strong>, para continuar la numeración
        de un sistema anterior sin repetir un número.
      </div>

      <NumeroControlConfig puedeEditar={puedeEditar} toast={toast} />

      {/* Advertencia de cumplimiento, siempre visible */}
      <div className="mb-3 rounded-xl bg-amber-50 dark:bg-amber-900/25 border border-amber-200 dark:border-amber-700/60 px-3.5 py-3 text-[12.5px] text-amber-800 dark:text-amber-300 flex gap-2.5 items-start">
        <Icon.Shield size={15} className="mt-0.5 shrink-0" />
        <span>
          Fijar la numeración es una acción de <strong>cumplimiento fiscal</strong>: solo avanza y es
          <strong> irreversible</strong>. Un folio ya usado no se puede reutilizar ni volver atrás, así que el próximo
          folio nunca puede ser menor que el actual. Úsalo solo para continuar desde un sistema previo.
        </span>
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {rows === null ? (
          <div className="p-4"><TableSkeleton rows={4} cols={4} /></div>
        ) : error ? (
          <div className="px-4 py-8 text-center">
            <div className="text-[13px] text-red-600 dark:text-red-400 mb-2">{error}</div>
            <Button size="sm" variant="ghost" icon={<Icon.Refresh size={14} />} onClick={cargar}>Reintentar</Button>
          </div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.ClipboardList size={22} />} title="Aún no se ha emitido ningún documento"
            body="Las series aparecen aquí al emitir la primera factura, cotización u orden de compra. Desde ese momento podrás fijar su próximo folio para continuar una numeración anterior." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Sede</th>
                  <th className="py-2.5 pr-3 font-medium">Serie</th>
                  <th className="py-2.5 pr-3 font-medium">Último folio</th>
                  <th className="py-2.5 pr-3 font-medium">Próximo folio</th>
                  {puedeEditar ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={`${r.sedeId}|${r.serie}`} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2.5 px-3 text-[13px]">{r.sedeNombre || r.sedeId}</td>
                    <td className="py-2.5 pr-3">
                      <div className="text-[13px] font-medium">{serieLabel(r.serie)}</div>
                      <div className="text-[11.5px] text-slate-400 mono">{r.serie}</div>
                    </td>
                    <td className="py-2.5 pr-3 mono text-[12.5px] text-slate-500 num">
                      {r.actual > 0 ? folioCompleto(r.serie, r.actual) : <span className="text-slate-400">Sin emitir</span>}
                    </td>
                    <td className="py-2.5 pr-3 mono text-[12.5px] font-semibold num">{folioCompleto(r.serie, r.proximo)}</td>
                    {puedeEditar ? (
                      <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                        <Button size="sm" variant="ghost" icon={<Icon.ArrowUp size={14} />} onClick={() => setFijando(r)}>Fijar próximo</Button>
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {!puedeEditar ? (
        <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>Solo la Dueña/Admin (o el Desarrollador) fija la numeración. Aquí ves el estado, pero no puedes cambiarlo.</span>
        </div>
      ) : null}

      {fijando ? (
        <FijarNumeracionModal fila={fijando} onClose={() => setFijando(null)}
          onSaved={(nuevas) => { setRows(nuevas); toast({ title: 'Numeración actualizada', body: `${serieLabel(fijando.serie)} · próximo ${folioCompleto(fijando.serie, fijando.proximo)}` }) }} />
      ) : null}
    </div>
  )
}

function FijarNumeracionModal({ fila, onClose, onSaved }) {
  const minimo = fila.proximo // no puede ser menor que el próximo actual
  const [valor, setValor] = useState(String(minimo))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const n = Number(valor)
  const entero = valor.trim() !== '' && Number.isInteger(n)
  const valido = entero && n >= minimo
  // Validación inline: entero y no menor que el próximo actual.
  const hint = !entero ? 'Escribe un número entero.' : (n < minimo ? `No puede ser menor que ${minimo} (el próximo actual).` : '')

  const confirmar = async () => {
    if (!valido) { setError(hint || 'Valor inválido.'); return }
    setBusy(true); setError('')
    try {
      // El backend confirma la regla forward-only y devuelve el estado actualizado.
      const estado = await api.fijarNumeracion({ sedeId: fila.sedeId, serie: fila.serie, proximo: n })
      onSaved(estado || [])
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo fijar la numeración.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.ArrowUp size={18} />}
      title="Fijar próximo folio" sub={`${serieLabel(fila.serie)} · ${fila.sedeNombre || fila.sedeId}`}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={confirmar} loading={busy} disabled={!valido} icon={<Icon.Check size={16} />}>Fijar folio</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="rounded-lg bg-amber-50 dark:bg-amber-900/25 border border-amber-200 dark:border-amber-700/60 px-3 py-2.5 text-[12px] text-amber-800 dark:text-amber-300">
          Esto <strong>solo avanza</strong> la numeración y es irreversible. El próximo documento de esta serie recibirá
          el folio que fijes aquí. Los folios que queden por el camino no se podrán usar.
        </div>
        <div className="grid grid-cols-2 gap-3 text-[12.5px]">
          <div>
            <div className="text-slate-400">Último emitido</div>
            <div className="mono num">{fila.actual > 0 ? folioCompleto(fila.serie, fila.actual) : 'Sin emitir'}</div>
          </div>
          <div>
            <div className="text-slate-400">Próximo actual</div>
            <div className="mono num">{folioCompleto(fila.serie, fila.proximo)}</div>
          </div>
        </div>
        <Field label="Próximo folio" required
          hint={`mínimo ${minimo}`}
          error={error || (valor.trim() !== '' && !valido ? hint : '')}>
          <Input type="number" min={minimo} step="1" value={valor} invalid={(!!error || (valor.trim() !== '' && !valido))}
            onChange={(e) => { setValor(e.target.value); setError('') }} />
        </Field>
        {valido && n > minimo ? (
          <div className="text-[12px] text-slate-500">
            El próximo documento se numerará <span className="mono font-semibold">{folioCompleto(fila.serie, n)}</span>.
          </div>
        ) : null}
      </div>
    </Modal>
  )
}

/* --- Usuarios y roles ---------------------------------------------------- */

// linkDeInvitacion arma el enlace de aceptación desde el origin actual del
// navegador (siempre correcto, sea dev o prod) usando el token que devuelve el
// backend. El backend también manda `enlace` absoluto (con FRONTEND_URL) como
// respaldo por si el token no viniera.
function linkDeInvitacion(res) {
  if (res?.token) return `${window.location.origin}/?invite=${encodeURIComponent(res.token)}`
  return res?.enlace || ''
}

function Usuarios() {
  const { db } = useData()
  const { ui } = useUI()
  const { activeEmpresaId } = useAuth()
  const toast = useToast()
  const confirm = useConfirm()
  const [rows, setRows] = useState(null)
  const [editando, setEditando] = useState(null)
  const [invitando, setInvitando] = useState(false)   // abre el formulario de invitar
  const [enlace, setEnlace] = useState(null)           // enlace recién generado a mostrar
  const [busyId, setBusyId] = useState('')
  const sedes = db.SEDES || []
  const puedeAdministrar = PUEDE_ADMINISTRAR.includes(ui.rol)

  const cargar = useCallback(() => {
    api.usuarios().then((r) => setRows(r || [])).catch(() => setRows([]))
  }, [])
  useEffect(() => { cargar() }, [cargar])

  const reenviar = async (u) => {
    setBusyId(u.id)
    try {
      const res = await api.reenviarInvitacion(activeEmpresaId, u.id)
      setEnlace(linkDeInvitacion(res))
      cargar()
    } catch (e) {
      toast({ title: 'No se pudo reenviar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  const cancelar = async (u) => {
    const ok = await confirm({
      title: 'Cancelar invitación',
      body: `Se anulará la invitación de ${u.email}. El enlace que hayas compartido dejará de funcionar.`,
      confirmLabel: 'Cancelar invitación', tone: 'danger',
    })
    if (!ok) return
    setBusyId(u.id)
    try {
      await api.cancelarInvitacion(activeEmpresaId, u.id)
      cargar()
      toast({ title: 'Invitación cancelada', body: u.email })
    } catch (e) {
      toast({ title: 'No se pudo cancelar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  return (
    <div>
      {puedeAdministrar ? (
        <div className="flex justify-end mb-3">
          <Button size="sm" icon={<Icon.UserPlus size={15} />} onClick={() => setInvitando(true)}>Invitar usuario</Button>
        </div>
      ) : null}

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {rows === null ? (
          <div className="p-4"><TableSkeleton rows={4} cols={4} /></div>
        ) : rows.length === 0 ? (
          <Empty icon={<Icon.Users size={22} />} title="Sin usuarios todavía"
            body="Cuando invites a alguien a la empresa aparecerá acá con su rol y su sede." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Usuario</th>
                  <th className="py-2.5 pr-3 font-medium">Rol</th>
                  <th className="py-2.5 pr-3 font-medium">Sede</th>
                  <th className="py-2.5 pr-3 font-medium">Estado</th>
                  <th className="py-2.5 pr-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((u) => {
                  const pendiente = u.estado !== 'active'
                  return (
                  <tr key={u.id} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2.5 px-3">
                      <div className="font-medium text-[13px]">{u.nombre || '—'}</div>
                      <div className="text-[11.5px] text-slate-400">{u.email}</div>
                    </td>
                    <td className="py-2.5 pr-3"><span className="text-[12.5px] font-semibold text-elerp-600 dark:text-elerp-300">{rolLabel(u.rol)}</span></td>
                    <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">
                      {u.sedeNombre}
                      {u.sedeObligatoria ? <span className="block text-[11px] text-slate-400">rol de una sola sede</span> : null}
                    </td>
                    <td className="py-2.5 pr-3">
                      {pendiente
                        ? <Badge size="sm" color="amber" dot>Invitación pendiente</Badge>
                        : <Badge size="sm" color="emerald" dot>Activo</Badge>}
                    </td>
                    <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                      {pendiente ? (
                        puedeAdministrar ? (
                          <div className="inline-flex gap-1">
                            <Button size="sm" variant="ghost" icon={<Icon.Link size={14} />}
                              loading={busyId === u.id} onClick={() => reenviar(u)}>Reenviar enlace</Button>
                            <Button size="sm" variant="ghost" icon={<Icon.Trash size={14} />}
                              disabled={busyId === u.id} onClick={() => cancelar(u)}>Cancelar</Button>
                          </div>
                        ) : <span className="text-[12px] text-slate-400">—</span>
                      ) : (
                        <Button size="sm" variant="ghost" icon={<Icon.Settings size={14} />} onClick={() => setEditando(u)}>Rol y sede</Button>
                      )}
                    </td>
                  </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
        <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
        <span>
          El <strong>cajero</strong> y el <strong>vendedor</strong> trabajan en una sola sede: el servidor les fija
          esa sede en cada petición, no pueden cambiarla ni ver otra. La <strong>contadora</strong> es de solo
          lectura fuera de Contabilidad y Tesorería. La interfaz solo oculta; lo que protege es el servidor.
        </span>
      </div>

      {invitando ? (
        <InvitarMiembro sedes={sedes} empresaId={activeEmpresaId} onClose={() => setInvitando(false)}
          onInvitado={(res, email) => {
            cargar()
            setInvitando(false)
            setEnlace(linkDeInvitacion(res))
            toast({ title: 'Invitación creada', body: email })
          }} />
      ) : null}

      {enlace ? <EnlaceInvitacion enlace={enlace} onClose={() => setEnlace(null)} /> : null}

      {editando ? (
        <EditarMiembro miembro={editando} sedes={sedes} onClose={() => setEditando(null)}
          onSaved={() => { cargar(); toast({ title: 'Usuario actualizado', body: `${editando.nombre || editando.email}` }) }} />
      ) : null}
    </div>
  )
}

// InvitarMiembro es el formulario de alta de un miembro nuevo: correo, nombre, rol
// y (si es cajero/vendedor) la sede obligatoria. Al crear, el componente padre
// muestra el enlace de aceptación para compartir.
function InvitarMiembro({ sedes, empresaId, onClose, onInvitado }) {
  const [email, setEmail] = useState('')
  const [nombre, setNombre] = useState('')
  const [rol, setRol] = useState('vendedor')
  const [sedeId, setSedeId] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const exigeSede = UNA_SOLA_SEDE.includes(rol)
  const emailOK = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())

  const invitar = async () => {
    if (!emailOK) { setError('Escribe un correo válido.'); return }
    if (exigeSede && !sedeId) { setError('Este rol trabaja en una sola sede: elige cuál.'); return }
    setBusy(true); setError('')
    try {
      const res = await api.invitarMiembro(empresaId, {
        email: email.trim(), nombre: nombre.trim(), rol, sedeId: exigeSede ? sedeId : '',
      })
      onInvitado(res, email.trim())
    } catch (e) {
      setError(e?.message || 'No se pudo crear la invitación.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.UserPlus size={18} />}
      title="Invitar usuario" sub="Se crea un acceso propio con su rol; al aceptar elige su contraseña"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={invitar} loading={busy} icon={<Icon.Send size={15} />}>Crear invitación</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Correo" required error={error && !emailOK ? error : undefined}
          hint="a este correo se asociará la cuenta">
          <Input type="email" autoComplete="off" placeholder="persona@empresa.com" invalid={!!email && !emailOK}
            value={email} onChange={(e) => { setEmail(e.target.value); setError('') }} />
        </Field>
        <Field label="Nombre" hint="opcional; la persona lo puede completar al aceptar">
          <Input type="text" placeholder="Nombre y apellido"
            value={nombre} onChange={(e) => setNombre(e.target.value)} />
        </Field>
        <Field label="Rol" required>
          <Select value={rol} onChange={(e) => { setRol(e.target.value); setError('') }}>
            {ROLES.map((r) => <option key={r.id} value={r.id}>{r.label}</option>)}
          </Select>
        </Field>
        {exigeSede ? (
          <Field label="Sede asignada" required error={error && exigeSede && !sedeId ? error : undefined}
            hint="el cajero/vendedor opera solo en esta sede">
            <Select value={sedeId} onChange={(e) => { setSedeId(e.target.value); setError('') }}>
              <option value="">Elige la sede…</option>
              {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
            </Select>
          </Field>
        ) : (
          <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 px-3 py-2.5 text-[12.5px] text-slate-500">
            Este rol ve todas las sedes de la empresa, así que no queda atado a ninguna.
          </div>
        )}
        {error && emailOK && !(exigeSede && !sedeId) ? (
          <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div>
        ) : null}
        <div className="rounded-lg bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-500/30 px-3 py-2.5 text-[12px] text-amber-800 dark:text-amber-300 flex gap-2 items-start">
          <Icon.Info size={14} className="mt-0.5 shrink-0" />
          <span>El correo de invitación automático todavía no está conectado. Al crear la invitación te mostramos el <strong>enlace</strong> para compartirlo tú mismo.</span>
        </div>
      </div>
    </Modal>
  )
}

// EnlaceInvitacion muestra el enlace de aceptación recién generado, listo para
// copiar y compartir (mientras el correo automático no esté cableado).
function EnlaceInvitacion({ enlace, onClose }) {
  const toast = useToast()
  const [copiado, setCopiado] = useState(false)
  const copiar = async () => {
    try {
      await navigator.clipboard.writeText(enlace)
      setCopiado(true)
      setTimeout(() => setCopiado(false), 2000)
    } catch {
      toast({ title: 'No se pudo copiar', body: 'Copia el enlace manualmente.', kind: 'error' })
    }
  }
  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Link size={18} />}
      title="Enlace de invitación" sub="Compártelo con la persona para que cree su acceso"
      footer={<Button onClick={onClose}>Listo</Button>}>
      <div className="space-y-3">
        <div className="flex gap-2">
          <Input readOnly value={enlace} onFocus={(e) => e.target.select()} className="font-mono text-[12px]" />
          <Button variant={copiado ? 'primary' : 'secondary'} icon={<Icon.Copy size={15} />} onClick={copiar}>
            {copiado ? 'Copiado' : 'Copiar'}
          </Button>
        </div>
        <div className="rounded-lg bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-500/30 px-3 py-2.5 text-[12px] text-amber-800 dark:text-amber-300 flex gap-2 items-start">
          <Icon.Info size={14} className="mt-0.5 shrink-0" />
          <span>El envío de correo automático aún no está conectado: comparte este enlace por tu cuenta. Al abrirlo, la persona elige su contraseña (o entra con Hubmy) y queda activa con el rol asignado.</span>
        </div>
      </div>
    </Modal>
  )
}

function EditarMiembro({ miembro, sedes, onClose, onSaved }) {
  const [rol, setRol] = useState(miembro.rol)
  const [sedeId, setSedeId] = useState(miembro.sedeId || '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const exigeSede = UNA_SOLA_SEDE.includes(rol)

  const guardar = async () => {
    if (exigeSede && !sedeId) {
      setError('Este rol trabaja en una sola sede: elige cuál.')
      return
    }
    setBusy(true); setError('')
    try {
      await api.guardarUsuario(miembro.id, { rol, sedeId: exigeSede ? sedeId : '' })
      onSaved()
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Users size={18} />}
      title={miembro.nombre || miembro.email} sub="Rol dentro de la empresa y sede asignada"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Rol" required>
          <Select value={rol} onChange={(e) => { setRol(e.target.value); setError('') }}>
            {ROLES.map((r) => <option key={r.id} value={r.id}>{r.label}</option>)}
          </Select>
        </Field>
        {exigeSede ? (
          <Field label="Sede asignada" required error={error}
            hint="el cajero abre caja y factura solo en esta sede">
            <Select value={sedeId} onChange={(e) => { setSedeId(e.target.value); setError('') }}>
              <option value="">Elige la sede…</option>
              {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
            </Select>
          </Field>
        ) : (
          <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 px-3 py-2.5 text-[12.5px] text-slate-500">
            Este rol ve todas las sedes de la empresa, así que no queda atado a ninguna.
          </div>
        )}
        {error && !exigeSede ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
      </div>
    </Modal>
  )
}

/* --- Cajas y sesiones ---------------------------------------------------- */

// Solo la Dueña/Admin y el Desarrollador dan de alta o configuran cajas y
// cajeros. El resto de roles (que llegan a la pestaña por el gate de lectura)
// las ve pero no las edita: la interfaz solo oculta, el backend protege.
const PUEDE_EDITAR_CAJAS = ['dueno', 'desarrollador']

function CajasYSesiones() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const [cajas, setCajas] = useState(null)
  const [cajeros, setCajeros] = useState(null)
  const [sesiones, setSesiones] = useState(null)
  const [pin, setPin] = useState(!!db.EMPRESA?.requiereSupervisorPin)
  const [busy, setBusy] = useState(false)
  const [formCaja, setFormCaja] = useState(null)     // null | {} (nueva) | caja (edición)
  const [formCajero, setFormCajero] = useState(null) // null | {} (nuevo) | cajero (edición)

  const puedeEditar = PUEDE_EDITAR_CAJAS.includes(ui.rol)
  const sedes = db.SEDES || []
  const dispositivos = db.DISPOSITIVOS || []
  const nombreSede = (id) => (id ? (sedes.find((s) => s.id === id)?.nombre || 'Sede desconocida') : 'Sin sede')
  const nombreDispositivo = (id) => (id ? (dispositivos.find((d) => d.id === id)?.nombre || 'Dispositivo no encontrado') : null)

  const cargar = useCallback(() => {
    api.cajas().then((r) => setCajas(r || [])).catch(() => setCajas([]))
    api.cajeros().then((r) => setCajeros(r || [])).catch(() => setCajeros([]))
    api.sesionesCaja().then((r) => setSesiones(r || [])).catch(() => setSesiones([]))
  }, [])

  useEffect(() => { cargar() }, [cargar])

  // Tras crear/editar, se refrescan las listas locales y el bootstrap (db.*).
  const trasGuardar = async () => { cargar(); await reload() }

  const togglePin = async (v) => {
    setPin(v); setBusy(true)
    try {
      await api.guardarConfigSeguridad(v)
      await reload()
      toast({
        title: v ? 'PIN de supervisor activado' : 'PIN de supervisor desactivado',
        body: v ? 'Quitar una línea, vaciar el carrito o salir del modo caja pedirá autorización.' : 'El cajero puede hacerlo sin autorización.',
      })
    } catch (e) {
      setPin(!v)
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusy(false) }
  }

  const supervisores = (cajeros || []).filter((c) => c.supervisor && c.activo)

  return (
    <div className="space-y-4">
      {/* Interruptor de seguridad de caja (flujo 2.4) */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
        <Toggle checked={pin} onChange={togglePin} disabled={busy || !puedeEditar}
          label="PIN de supervisor para quitar productos"
          sub="Pide autorización de un supervisor cuando el cajero quiera quitar una línea, vaciar el carrito o salir del modo caja. Desactivado por defecto." />
        {pin ? (
          supervisores.length === 0 ? (
            <div className="mt-3 rounded-lg bg-amber-50 dark:bg-amber-900/25 border border-amber-200 dark:border-amber-700/60 px-3 py-2.5 text-[12.5px] text-amber-800 dark:text-amber-300">
              Está activado pero <strong>no hay ningún cajero marcado como supervisor</strong>: nadie podría autorizar.
              Marca al menos uno para que la regla se pueda cumplir.
            </div>
          ) : (
            <div className="mt-3 text-[12px] text-slate-500">
              Autorizan: {supervisores.map((s) => `${s.nombre} (${s.codigo})`).join(' · ')}. Cada autorización queda en la auditoría con la acción concreta.
            </div>
          )
        ) : null}
      </div>

      {/* Cajas */}
      <div>
        <div className="flex items-center justify-between gap-2 mb-2">
          <div className="text-[12.5px] font-semibold text-slate-600 dark:text-slate-300">Cajas de la empresa</div>
          {puedeEditar && cajas && cajas.length > 0 ? (
            <Button size="sm" variant="ghost" icon={<Icon.Plus size={15} />}
              disabled={sedes.length === 0} onClick={() => setFormCaja({})}>Nueva caja</Button>
          ) : null}
        </div>
        {cajas === null ? (
          <div className="grid grid-cols-2 md:grid-cols-3 gap-2.5">{[0, 1, 2].map((i) => <div key={i} className="skeleton h-[68px] rounded-xl" />)}</div>
        ) : cajas.length === 0 ? (
          <Empty icon={<Icon.Wallet size={22} />} title="Sin cajas"
            body={sedes.length === 0
              ? 'Crea primero una sede: cada caja pertenece a un local.'
              : 'Crea una caja por puesto de cobro para que se puedan abrir turnos.'}
            cta={puedeEditar && sedes.length > 0 ? <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setFormCaja({})}>Nueva caja</Button> : null} />
        ) : (
          <div className="grid grid-cols-2 md:grid-cols-3 gap-2.5">
            {cajas.map((c) => (
              <div key={c.id} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-3">
                <div className="flex items-center justify-between gap-2">
                  <div className="text-[13.5px] font-semibold truncate">{c.nombre}</div>
                  {c.estado === 'habilitada' ? <Badge size="sm" color="emerald" dot>Habilitada</Badge> : <Badge size="sm" color="slate">Deshabilitada</Badge>}
                </div>
                <div className="text-[11.5px] text-slate-500 mt-1 mono">{c.codigo}</div>
                <div className="text-[11.5px] text-slate-500 mt-0.5">{nombreSede(c.sedeId)}</div>
                {c.dispositivoFiscalId ? (
                  <div className="text-[11.5px] text-slate-400 mt-0.5 flex items-center gap-1">
                    <Icon.Printer size={12} /> <span className="truncate">{nombreDispositivo(c.dispositivoFiscalId)}</span>
                  </div>
                ) : null}
                {c.ocupada ? <div className="text-[11.5px] text-teal-600 dark:text-teal-400 mt-1">Turno abierto · {c.ocupadaPor}</div> : null}
                {puedeEditar ? (
                  <div className="mt-2 pt-2 border-t border-slate-100 dark:border-slate-800/70">
                    <button className="text-[12px] text-elerp-600 dark:text-teal-400 hover:underline inline-flex items-center gap-1"
                      onClick={() => setFormCaja(c)}><Icon.Pencil size={13} /> Editar</button>
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Cajeros (credenciales de puesto) */}
      <div>
        <div className="flex items-center justify-between gap-2 mb-2">
          <div className="text-[12.5px] font-semibold text-slate-600 dark:text-slate-300">Cajeros (credenciales de puesto)</div>
          {puedeEditar && cajeros && cajeros.length > 0 ? (
            <Button size="sm" variant="ghost" icon={<Icon.Plus size={15} />} onClick={() => setFormCajero({})}>Nuevo cajero</Button>
          ) : null}
        </div>
        {cajeros === null ? (
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={3} cols={4} /></div>
        ) : cajeros.length === 0 ? (
          <Empty icon={<Icon.Users size={22} />} title="Sin cajeros"
            body="El cajero abre su turno con un código y un PIN, sin usar una sesión de la app. Da de alta al menos uno."
            cta={puedeEditar ? <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setFormCajero({})}>Nuevo cajero</Button> : null} />
        ) : (
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                    <th className="py-2.5 px-3 font-medium">Cajero</th>
                    <th className="py-2.5 pr-3 font-medium">Código</th>
                    <th className="py-2.5 pr-3 font-medium">Sede</th>
                    <th className="py-2.5 pr-3 font-medium">Rol</th>
                    <th className="py-2.5 pr-3 font-medium">Estado</th>
                    {puedeEditar ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                  </tr>
                </thead>
                <tbody>
                  {cajeros.map((c) => (
                    <tr key={c.id} className="border-b border-slate-100 dark:border-slate-800/70">
                      <td className="py-2.5 px-3 font-medium text-[13px]">{c.nombre}</td>
                      <td className="py-2.5 pr-3 mono text-[12.5px] text-slate-500">{c.codigo}</td>
                      <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">{c.sedeId ? nombreSede(c.sedeId) : 'Todas'}</td>
                      <td className="py-2.5 pr-3">{c.supervisor ? <Badge size="sm" color="huberp">Supervisor</Badge> : <span className="text-[12.5px] text-slate-500">Cajero</span>}</td>
                      <td className="py-2.5 pr-3">{c.activo ? <Badge size="sm" color="emerald" dot>Activo</Badge> : <Badge size="sm" color="slate">Inactivo</Badge>}</td>
                      {puedeEditar ? (
                        <td className="py-2.5 pr-3 text-right">
                          <button className="text-[12px] text-elerp-600 dark:text-teal-400 hover:underline inline-flex items-center gap-1"
                            onClick={() => setFormCajero(c)}><Icon.Pencil size={13} /> Editar</button>
                        </td>
                      ) : null}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>

      {formCaja ? (
        <CajaForm caja={formCaja.id ? formCaja : null} sedes={sedes} dispositivos={dispositivos}
          onClose={() => setFormCaja(null)}
          onSaved={async (nombre, edicion) => { await trasGuardar(); toast({ title: edicion ? 'Caja actualizada' : 'Caja creada', body: nombre }) }} />
      ) : null}
      {formCajero ? (
        <CajeroForm cajero={formCajero.id ? formCajero : null} sedes={sedes}
          onClose={() => setFormCajero(null)}
          onSaved={async (nombre, edicion) => { await trasGuardar(); toast({ title: edicion ? 'Cajero actualizado' : 'Cajero creado', body: nombre }) }} />
      ) : null}

      {/* Historial de sesiones (auditoría del arqueo): fondo, esperado, contado y
          diferencia de cada turno cerrado. El arqueo es inmutable una vez cerrado. */}
      <div>
        <div className="text-[12.5px] font-semibold text-slate-600 dark:text-slate-300 mb-2">Historial de sesiones (auditoría del arqueo)</div>
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          {sesiones === null ? (
            <div className="p-4"><TableSkeleton rows={3} cols={6} /></div>
          ) : sesiones.length === 0 ? (
            <div className="px-4 py-6 text-[13px] text-slate-500">Todavía no se ha abierto ningún turno.</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                    <th className="py-2.5 px-3 font-medium">Cajero</th>
                    <th className="py-2.5 pr-3 font-medium">Caja</th>
                    <th className="py-2.5 pr-3 font-medium">Apertura / Cierre</th>
                    <th className="py-2.5 pr-3 font-medium text-right">Fondo</th>
                    <th className="py-2.5 pr-3 font-medium text-right">Esperado</th>
                    <th className="py-2.5 pr-3 font-medium text-right">Contado</th>
                    <th className="py-2.5 pr-3 font-medium text-right">Diferencia</th>
                  </tr>
                </thead>
                <tbody>
                  {sesiones.map((s) => {
                    const a = s.arqueo
                    const dif = a?.declarado ? a.diferenciaBs : null
                    return (
                      <tr key={s.id} className="border-b border-slate-100 dark:border-slate-800/70">
                        <td className="py-2.5 px-3 font-medium text-[13px]">{s.cajeroNombre}</td>
                        <td className="py-2.5 pr-3 mono text-[12.5px] text-slate-500">{s.cajaCodigo}</td>
                        <td className="py-2.5 pr-3 text-[12.5px] text-slate-500 num">
                          <div>{fmtDate(s.apertura)}</div>
                          {s.cierre
                            ? <div className="text-slate-400">{fmtDate(s.cierre)}</div>
                            : <Badge size="sm" color="teal" dot>Abierto</Badge>}
                        </td>
                        <td className="py-2.5 pr-3 text-[12.5px] text-slate-500 num text-right">{fmtCurrency(s.fondoInicial || 0, 'VES')}</td>
                        <td className="py-2.5 pr-3 text-[12.5px] text-slate-500 num text-right">{a ? fmtCurrency(a.efectivoEsperadoBs, 'VES') : '—'}</td>
                        <td className="py-2.5 pr-3 text-[12.5px] text-slate-500 num text-right">{a?.declarado ? fmtCurrency(a.efectivoContadoBs, 'VES') : '—'}</td>
                        <td className="py-2.5 pr-3 text-[12.5px] num text-right">
                          {dif === null ? (
                            <span className="text-slate-400">—</span>
                          ) : dif === 0 ? (
                            <Badge size="sm" color="emerald" dot>Cuadra</Badge>
                          ) : (
                            <span className={dif > 0 ? 'text-amber-600 dark:text-amber-400' : 'text-[#B3362C] dark:text-red-400'}>
                              {dif > 0 ? '+' : '−'}{fmtCurrency(Math.abs(dif), 'VES')}
                            </span>
                          )}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

/* Alta y edición de una caja. Una caja nace deshabilitada; el interruptor
 * «Habilitada» la activa (o desactiva) sin borrarla. El dispositivo fiscal es
 * opcional y se ofrece de la misma sede o de los que aplican a toda la empresa. */
function CajaForm({ caja, sedes, dispositivos, onClose, onSaved }) {
  const edicion = !!caja
  const [f, setF] = useState(() => ({
    nombre: caja?.nombre || '',
    sedeId: caja?.sedeId || (sedes[0]?.id || ''),
    dispositivoFiscalId: caja?.dispositivoFiscalId || '',
    activa: edicion ? caja?.estado === 'habilitada' : false,
    // Override de branding por caja para la pantalla del cliente (contraste).
    colorFondo: caja?.colorFondo || '',
    logoVersion: caja?.logoVersion || '',
  }))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // Solo dispositivos activos de la sede de la caja o de toda la empresa. El
  // asignado actual se conserva aunque hoy no cumpla el filtro (no se pierde).
  const dispDisponibles = (dispositivos || []).filter(
    (d) => d.activo && d.tipo !== 'balanza' && (!d.sedeId || d.sedeId === f.sedeId))
  if (f.dispositivoFiscalId && !dispDisponibles.some((d) => d.id === f.dispositivoFiscalId)) {
    const actual = (dispositivos || []).find((d) => d.id === f.dispositivoFiscalId)
    if (actual) dispDisponibles.push(actual)
  }

  const guardar = async () => {
    if (!f.nombre.trim()) { setError('El nombre es obligatorio.'); return }
    if (!f.sedeId) { setError('Elige la sede de la caja.'); return }
    setBusy(true); setError('')
    try {
      if (edicion) {
        await api.actualizarCaja(caja.id, {
          nombre: f.nombre.trim(), sedeId: f.sedeId,
          dispositivoFiscalId: f.dispositivoFiscalId || '', activa: f.activa,
          colorFondo: (f.colorFondo || '').trim(), logoVersion: f.logoVersion || '',
        })
      } else {
        const creada = await api.crearCaja({
          nombre: f.nombre.trim(), sedeId: f.sedeId, dispositivoFiscalId: f.dispositivoFiscalId || '',
        })
        // La caja nace deshabilitada; si se pidió activa o hay override de branding,
        // se aplica con un PATCH tras crear (el alta no acepta esos campos).
        if (creada?.id && (f.activa || (f.colorFondo || '').trim() || f.logoVersion)) {
          await api.actualizarCaja(creada.id, {
            activa: f.activa, colorFondo: (f.colorFondo || '').trim(), logoVersion: f.logoVersion || '',
          })
        }
      }
      onSaved(f.nombre.trim(), edicion)
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Wallet size={18} />}
      title={edicion ? 'Editar caja' : 'Nueva caja'}
      sub={edicion ? caja.codigo : 'El código lo asigna el servidor (C-001, C-002…)'}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre" required error={error && !f.nombre.trim() ? error : ''}>
          <Input value={f.nombre} placeholder="Ej. Caja 1" invalid={!!error && !f.nombre.trim()}
            onChange={(e) => { setF((s) => ({ ...s, nombre: e.target.value })); setError('') }} />
        </Field>
        <Field label="Sede" required hint="el local donde está el puesto de cobro"
          error={error && !f.sedeId ? error : ''}>
          <Select value={f.sedeId} onChange={(e) => setF((s) => ({ ...s, sedeId: e.target.value, dispositivoFiscalId: '' }))}>
            <option value="" disabled>Elige una sede…</option>
            {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
          </Select>
        </Field>
        <Field label="Dispositivo fiscal" hint="opcional — la impresora fiscal de esta caja">
          <Select value={f.dispositivoFiscalId} onChange={(e) => setF((s) => ({ ...s, dispositivoFiscalId: e.target.value }))}>
            <option value="">Sin dispositivo</option>
            {dispDisponibles.map((d) => <option key={d.id} value={d.id}>{d.nombre}</option>)}
          </Select>
        </Field>
        <div className="rounded-lg border border-slate-200 dark:border-slate-800 px-3 py-2.5">
          <Toggle checked={f.activa} onChange={(v) => setF((s) => ({ ...s, activa: v }))}
            label="Habilitada" sub="Solo una caja habilitada admite abrir turno. Con un turno abierto no se puede deshabilitar." />
        </div>

        {/* Branding de la pantalla del cliente por caja (override de contraste).
            Vacío = usar el color y el logo de la empresa (Ajustes › Datos de empresa › Marca). */}
        <div className="pt-1 border-t border-slate-200 dark:border-slate-800 space-y-3">
          <div>
            <div className="text-[12.5px] font-semibold text-slate-700 dark:text-slate-200">Pantalla del cliente</div>
            <div className="text-[11.5px] text-slate-500 dark:text-slate-400 mt-0.5">
              Ajusta el contraste de esta caja. Si lo dejas vacío, usa el branding de la empresa.
            </div>
          </div>
          <Field label="Color de fondo" hint="hex — vacío usa el degradado navy por defecto">
            <div className="flex items-center gap-2.5">
              <input type="color" value={(f.colorFondo || '').trim() || '#152c61'}
                onChange={(e) => setF((s) => ({ ...s, colorFondo: e.target.value }))}
                className="h-9 w-12 shrink-0 rounded-lg border border-slate-200 dark:border-slate-700 bg-white p-0.5 cursor-pointer"
                aria-label="Elegir color de fondo" title="Elegir color de fondo" />
              <Input value={f.colorFondo || ''} placeholder="#152C61"
                onChange={(e) => setF((s) => ({ ...s, colorFondo: e.target.value }))} />
              {(f.colorFondo || '').trim() ? (
                <Button variant="ghost" size="sm" icon={<Icon.Trash size={15} />}
                  onClick={() => setF((s) => ({ ...s, colorFondo: '' }))}>Quitar</Button>
              ) : null}
            </div>
          </Field>
          <Field label="Versión del logo" hint="cuál mostrar en esta caja">
            <Select value={f.logoVersion || ''} onChange={(e) => setF((s) => ({ ...s, logoVersion: e.target.value }))}>
              <option value="">Principal (por defecto de la empresa)</option>
              <option value="principal">Logo principal</option>
              <option value="alterno">Logo alterno</option>
            </Select>
          </Field>
        </div>

        {error && f.nombre.trim() && f.sedeId ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
      </div>
    </Modal>
  )
}

/* Alta y edición de un cajero (credencial de puesto). El código no se cambia en
 * la edición: es lo que el cajero ya memorizó. El PIN es de 4 dígitos y, al
 * editar, se deja en blanco para no cambiarlo (reset opcional). */
function CajeroForm({ cajero, sedes, onClose, onSaved }) {
  const edicion = !!cajero
  const [f, setF] = useState(() => ({
    nombre: cajero?.nombre || '',
    codigo: cajero?.codigo || '',
    pin: '',
    supervisor: !!cajero?.supervisor,
    activo: edicion ? !!cajero?.activo : true,
    sedeId: cajero?.sedeId || '',
  }))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const pinValido = (p) => /^\d{4}$/.test(p)

  const guardar = async () => {
    if (!f.nombre.trim()) { setError('El nombre es obligatorio.'); return }
    if (!edicion && !pinValido(f.pin)) { setError('El PIN debe tener 4 dígitos.'); return }
    if (edicion && f.pin && !pinValido(f.pin)) { setError('El PIN debe tener 4 dígitos.'); return }
    setBusy(true); setError('')
    try {
      if (edicion) {
        const body = {
          nombre: f.nombre.trim(), supervisor: f.supervisor, activo: f.activo,
          sedeId: f.sedeId || '',
        }
        if (f.pin) body.pin = f.pin // reset opcional
        await api.actualizarCajero(cajero.id, body)
      } else {
        await api.crearCajero({
          nombre: f.nombre.trim(), codigo: f.codigo.trim(), pin: f.pin,
          supervisor: f.supervisor, sedeId: f.sedeId || '',
        })
      }
      onSaved(f.nombre.trim(), edicion)
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Users size={18} />}
      title={edicion ? 'Editar cajero' : 'Nuevo cajero'}
      sub={edicion ? `Código ${cajero.codigo}` : 'La credencial con la que se abre el turno en el mostrador'}
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre" required error={error && !f.nombre.trim() ? error : ''}>
          <Input value={f.nombre} placeholder="Ej. Luis Marcano" invalid={!!error && !f.nombre.trim()}
            onChange={(e) => { setF((s) => ({ ...s, nombre: e.target.value })); setError('') }} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Código" hint={edicion ? 'no se puede cambiar' : 'opcional — se genera solo'}>
            <Input value={f.codigo} disabled={edicion} placeholder="Ej. OP-004"
              onChange={(e) => setF((s) => ({ ...s, codigo: e.target.value }))} />
          </Field>
          <Field label={edicion ? 'Reiniciar PIN' : 'PIN'} required={!edicion}
            hint={edicion ? 'déjalo vacío para no cambiarlo' : '4 dígitos'}
            error={error && (error.includes('PIN')) ? error : ''}>
            <Input value={f.pin} inputMode="numeric" maxLength={4} type="password"
              placeholder={edicion ? '••••' : '4 dígitos'} invalid={!!error && error.includes('PIN')}
              onChange={(e) => { setF((s) => ({ ...s, pin: e.target.value.replace(/\D/g, '').slice(0, 4) })); setError('') }} />
          </Field>
        </div>
        <Field label="Sede" hint="opcional — vacío = puede abrir caja en cualquier sede">
          <Select value={f.sedeId} onChange={(e) => setF((s) => ({ ...s, sedeId: e.target.value }))}>
            <option value="">Cualquier sede</option>
            {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
          </Select>
        </Field>
        <div className="rounded-lg border border-slate-200 dark:border-slate-800 px-3 py-2.5 space-y-2.5">
          <Toggle checked={f.supervisor} onChange={(v) => setF((s) => ({ ...s, supervisor: v }))}
            label="Supervisor" sub="Puede autorizar quitar una línea, vaciar el carrito o salir del modo caja." />
          {edicion ? (
            <Toggle checked={f.activo} onChange={(v) => setF((s) => ({ ...s, activo: v }))}
              label="Activo" sub="Un cajero inactivo no puede abrir turno. No se borra: se puede reactivar." />
          ) : null}
        </div>
        {error && f.nombre.trim() && !error.includes('PIN') ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
      </div>
    </Modal>
  )
}

/* --- Moneda y tasa (R9 + R10) -------------------------------------------- */

// Divisas que el backend acepta (whitelist). El bolívar es la base y no entra
// en esta lista: es la moneda de las facturas y los libros.
const WHITELIST_DIVISAS = ['USD', 'EUR', 'COP', 'USDT']

// Fuentes de tasa válidas por divisa: BCV automática es SOLO para el dólar; las
// demás llevan tasa manual o de mercado.
const fuentesDe = (codigo) => (permiteFuenteBcv(codigo)
  ? [{ id: 'bcv', label: 'BCV (automática)' }, { id: 'mercado', label: 'Mercado' }, { id: 'manual', label: 'Manual' }]
  : [{ id: 'manual', label: 'Manual' }, { id: 'mercado', label: 'Mercado' }])

function MonedaYTasa() {
  const { db, reload, recargarTasa } = useData()
  const toast = useToast()
  const { ui } = useUI()
  const emp = db.EMPRESA || {}
  const admin = ['dueno', 'desarrollador'].includes(ui.rol)

  // Lista de divisas activas guardada en el backend (db.TASAS.activas). El dólar
  // siempre está presente: es la divisa base sobre la que se apoyan IGTF, cobros
  // en US$ y la conversión por defecto. Sin db.TASAS (backend viejo) se cae al
  // USD-único con la fuente de la empresa.
  const activasGuardadas = () => {
    const src = Array.isArray(db.TASAS?.activas) && db.TASAS.activas.length
      ? db.TASAS.activas.map((a) => ({ codigo: (a.codigo || '').toUpperCase(), fuente: a.fuente || 'manual' }))
      : [{ codigo: 'USD', fuente: emp.fuenteTasa || 'bcv' }]
    if (!src.some((a) => a.codigo === 'USD')) src.unshift({ codigo: 'USD', fuente: emp.fuenteTasa || 'bcv' })
    return src
  }

  const [activas, setActivas] = useState(activasGuardadas)
  const [monedaPrincipal, setMonedaPrincipal] = useState(emp.monedaPrincipal || 'VES')
  const [preciosEnUsd, setPreciosEnUsd] = useState(!!emp.preciosEnUsd)
  const [manuales, setManuales] = useState({}) // { codigo: valorEnBs (texto) }
  const [busy, setBusy] = useState(false)
  const [cargandoTasa, setCargandoTasa] = useState('')
  const [nuevaDivisa, setNuevaDivisa] = useState('')

  // Re-sincroniza el borrador cuando se cambia de empresa (no en cada recarga de
  // tasa, para no pisar ediciones sin guardar).
  useEffect(() => {
    setActivas(activasGuardadas())
    setMonedaPrincipal(emp.monedaPrincipal || 'VES')
    setPreciosEnUsd(!!emp.preciosEnUsd)
  }, [emp.id]) // eslint-disable-line react-hooks/exhaustive-deps

  // Tasa vigente de una divisa (para mostrar en su fila). USD cae a db.TASA.
  const tasaFila = (codigo) => {
    const f = (db.TASAS?.tasas || []).find((x) => (x.moneda || '').toUpperCase() === codigo)
    if (f && Number(f.valor) > 0) return f
    if (codigo === 'USD' && db.TASA?.hay) {
      return { valor: db.TASA.valor, fuenteLabel: db.TASA.fuenteLabel, fechaValor: db.TASA.fechaValor, esDeHoy: db.TASA.esDeHoy }
    }
    return null
  }

  const disponibles = WHITELIST_DIVISAS.filter((c) => !activas.some((a) => a.codigo === c))

  const setFuente = (codigo, fuente) => setActivas((l) => l.map((a) => (a.codigo === codigo ? { ...a, fuente } : a)))
  const quitarDivisa = (codigo) => { if (codigo !== 'USD') setActivas((l) => l.filter((a) => a.codigo !== codigo)) }
  const agregarDivisa = () => {
    const c = nuevaDivisa
    if (!c || activas.some((a) => a.codigo === c)) return
    // Fuente por defecto: BCV para el dólar (no debería pasar, ya está), manual
    // para el resto.
    setActivas((l) => [...l, { codigo: c, fuente: permiteFuenteBcv(c) ? 'bcv' : 'manual' }])
    setNuevaDivisa('')
  }

  const usdFuente = activas.find((a) => a.codigo === 'USD')?.fuente || 'bcv'

  const guardar = async () => {
    setBusy(true)
    try {
      await api.guardarConfigMoneda({
        monedaPrincipal,
        // Se conserva `fuenteTasa` (campo histórico del backend) = la fuente del
        // dólar, la divisa base.
        fuenteTasa: usdFuente,
        preciosEnUsd,
        monedasActivas: activas,
      })
      await reload()
      // El borrador queda sincronizado con lo recién guardado.
      setActivas(activas)
      toast({ title: 'Configuración guardada', body: 'Moneda principal y divisas activas actualizadas.' })
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusy(false) }
  }

  const cargarTasa = async (codigo) => {
    const valor = Number(manuales[codigo])
    if (!(valor > 0)) { toast({ title: 'Tasa inválida', body: `Escribe la tasa en Bs por 1 ${codigo}.`, kind: 'warn' }); return }
    setCargandoTasa(codigo)
    try {
      await api.cargarTasaMoneda({ moneda: codigo, valor })
      await recargarTasa()
      setManuales((m) => ({ ...m, [codigo]: '' }))
      toast({ title: 'Tasa cargada', body: `Bs ${fmtNum(valor, 2)} por 1 ${codigo} · queda registrada a tu nombre.` })
    } catch (e) {
      toast({ title: 'No se pudo cargar', body: e?.message || 'Error', kind: 'error' })
    } finally { setCargandoTasa('') }
  }

  // Comparación borrador ↔ guardado para habilitar «Guardar cambios».
  const guardadas = activasGuardadas()
  const mismasActivas = activas.length === guardadas.length
    && activas.every((a) => guardadas.some((g) => g.codigo === a.codigo && g.fuente === a.fuente))
  const cambio = monedaPrincipal !== (emp.monedaPrincipal || 'VES')
    || preciosEnUsd !== !!emp.preciosEnUsd
    || !mismasActivas

  return (
    <div className="space-y-4 max-w-2xl">
      {/* Moneda principal + precios en divisa */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-4">
        <div>
          <div className="text-[13px] font-semibold mb-2">Moneda principal</div>
          <div className="grid grid-cols-2 gap-3">
            {[
              { id: 'VES', t: 'Bolívares (Bs)', s: 'Facturas y libros en Bs — lo que exige el SENIAT' },
              { id: 'USD', t: 'Dólares (US$)', s: 'Piensas en dólares; se convierte a Bs al facturar' },
            ].map((m) => (
              <button key={m.id} onClick={() => setMonedaPrincipal(m.id)}
                className={`text-left p-3.5 rounded-xl border transition-colors ${monedaPrincipal === m.id ? 'border-elerp-500 bg-elerp-50 dark:bg-elerp-900/40' : 'border-slate-200 dark:border-slate-700 hover:border-elerp-300'}`}>
                <div className="text-sm font-semibold">{m.t}</div>
                <div className="text-[12px] text-slate-500 mt-0.5">{m.s}</div>
              </button>
            ))}
          </div>
        </div>

        {monedaPrincipal === 'VES' ? (
          <Toggle checked={preciosEnUsd} onChange={setPreciosEnUsd}
            label="Permitir precios en divisa"
            sub="Cada producto puede tener su precio en una divisa activa; se convierte a Bs con la tasa del día al facturar. El precio homólogo se calcula, nunca se guarda duplicado." />
        ) : null}

        <div className="flex justify-end">
          <Button onClick={guardar} loading={busy} disabled={!cambio} icon={<Icon.Check size={16} />}>Guardar cambios</Button>
        </div>
      </div>

      {/* Divisas activas */}
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3">
        <div>
          <div className="text-[13px] font-semibold">Divisas activas</div>
          <div className="text-[11.5px] text-slate-500 mt-0.5">
            La base es el bolívar. Elige en qué divisas puedes fijar precios y cobrar; cada una lleva su tasa (Bs por 1 unidad).
            <span className="font-medium text-slate-600 dark:text-slate-300"> La BCV automática es solo para el dólar</span> — las demás llevan tasa manual o de mercado.
          </div>
        </div>

        <div className="divide-y divide-slate-100 dark:divide-slate-800 border border-slate-200 dark:border-slate-800 rounded-xl overflow-hidden">
          {activas.map((a) => {
            const t = tasaFila(a.codigo)
            const esManual = a.fuente === 'manual' || a.fuente === 'mercado'
            return (
              <div key={a.codigo} className="p-3 space-y-2.5">
                <div className="flex items-center gap-3">
                  <div className="h-8 w-11 shrink-0 rounded-lg bg-slate-100 dark:bg-slate-800 inline-flex items-center justify-center text-[12px] font-bold">
                    {monedaSimbolo(a.codigo)}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="text-[13px] font-semibold flex items-center gap-2">
                      {monedaLabel(a.codigo)}
                      {a.codigo === 'USD' ? <Badge size="sm" color="slate">base</Badge> : null}
                    </div>
                    <div className="text-[11px] text-slate-500 truncate">{monedaNombre(a.codigo)}</div>
                  </div>
                  <Select className="!w-40" value={a.fuente} onChange={(e) => setFuente(a.codigo, e.target.value)}>
                    {fuentesDe(a.codigo).map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}
                  </Select>
                  {a.codigo !== 'USD' ? (
                    <Button size="sm" variant="ghost" icon={<Icon.Trash size={15} />} onClick={() => quitarDivisa(a.codigo)}
                      aria-label={`Quitar ${a.codigo}`}>Quitar</Button>
                  ) : <div className="w-[86px]" />}
                </div>

                <div className="flex items-center gap-3 pl-14">
                  <div className="text-[12px] text-slate-500 flex-1 min-w-0">
                    {t ? (
                      <span className="num">Bs {fmtNum(t.valor, 2)} <span className="text-slate-400">por 1 {a.codigo} · {t.fuenteLabel || a.fuente}{t.fechaValor ? ` · ${fechaCortaVE(t.fechaValor, t.esDeHoy)}` : ''}</span></span>
                    ) : (
                      <span className="text-amber-600 dark:text-amber-400">Sin tasa cargada</span>
                    )}
                  </div>
                  {esManual && admin ? (
                    <div className="flex items-end gap-1.5 shrink-0">
                      <Input className="!w-28" type="number" min="0" step="0.01" placeholder="Bs / unidad"
                        value={manuales[a.codigo] || ''} onChange={(e) => setManuales((m) => ({ ...m, [a.codigo]: e.target.value }))} />
                      <Button size="sm" variant="secondary" loading={cargandoTasa === a.codigo}
                        onClick={() => cargarTasa(a.codigo)} icon={<Icon.Check size={15} />}>Cargar</Button>
                    </div>
                  ) : a.codigo === 'USD' ? (
                    <span className="text-[11px] text-slate-400 shrink-0">Se administra desde el chip de la barra</span>
                  ) : null}
                </div>
              </div>
            )
          })}
        </div>

        {/* Agregar divisa */}
        {disponibles.length ? (
          <div className="flex items-end gap-2 pt-1">
            <div className="flex-1">
              <Field label="Agregar divisa">
                <Select value={nuevaDivisa} onChange={(e) => setNuevaDivisa(e.target.value)}>
                  <option value="">Elige una divisa…</option>
                  {disponibles.map((c) => <option key={c} value={c}>{monedaLabel(c)} — {monedaNombre(c)}</option>)}
                </Select>
              </Field>
            </div>
            <Button variant="secondary" icon={<Icon.Plus size={15} />} disabled={!nuevaDivisa} onClick={agregarDivisa}>Agregar</Button>
          </div>
        ) : (
          <div className="text-[11.5px] text-slate-400 pt-1">Ya están activas todas las divisas disponibles.</div>
        )}

        <div className="flex justify-end pt-1">
          <Button onClick={guardar} loading={busy} disabled={!cambio} icon={<Icon.Check size={16} />}>Guardar cambios</Button>
        </div>
      </div>
    </div>
  )
}

/* --- Métodos de pago ----------------------------------------------------- */

// Los 6 tipos del contrato. Cada tipo trae la moneda que le corresponde por
// defecto (se puede ajustar), y un icono para reconocerlo de un vistazo.
const TIPOS_METODO = [
  { id: 'efectivo_bs', label: 'Efectivo Bs', moneda: 'VES', icon: 'Banknote' },
  { id: 'efectivo_usd', label: 'Efectivo US$', moneda: 'USD', icon: 'Banknote' },
  { id: 'pago_movil', label: 'Pago móvil', moneda: 'VES', icon: 'Smartphone' },
  { id: 'zelle', label: 'Zelle', moneda: 'USD', icon: 'Globe' },
  { id: 'tarjeta', label: 'Punto de venta / Tarjeta', moneda: 'VES', icon: 'Wallet' },
  { id: 'transferencia', label: 'Transferencia', moneda: 'VES', icon: 'Bank' },
]
const tipoMeta = (t) => TIPOS_METODO.find((x) => x.id === t) || { label: t, icon: 'Wallet' }
// Solo la Dueña/Admin y el Desarrollador editan la configuración de cobro. La
// interfaz oculta las acciones; el backend es el que de verdad protege.
const PUEDE_EDITAR_METODOS = ['dueno', 'desarrollador']

function MetodosPago() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const confirm = useConfirm()
  const [busyId, setBusyId] = useState('')
  const [form, setForm] = useState(null) // null | {} (nuevo) | metodo (edición)
  const puedeEditar = PUEDE_EDITAR_METODOS.includes(ui.rol)

  const cuentas = db.CUENTAS_COBRO || []
  const nombreCuenta = (id) => {
    const c = cuentas.find((x) => x.id === id)
    return c ? `${c.titular}${c.datos ? ' · ' + c.datos : ''}` : null
  }
  // Vienen ordenados del servidor, pero reordenamos por si acaso.
  const rows = [...(db.METODOS_PAGO || [])].sort((a, b) => (a.orden || 0) - (b.orden || 0))

  const patch = async (m, campo, valor) => {
    setBusyId(m.id)
    try {
      await api.actualizarMetodoPago(m.id, { [campo]: valor })
      await reload()
    } catch (e) {
      toast({ title: 'No se pudo guardar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          <strong>En Caja</strong> = disponible en el Punto de Venta. <strong>En Ventas</strong> = disponible en el módulo de Ventas.
          Un método <strong>inactivo</strong> no aparece en ninguno de los dos.
        </div>
        {puedeEditar ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setForm({})}>Nuevo método</Button>
        ) : null}
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {rows.length === 0 ? (
          <Empty icon={<Icon.Wallet size={22} />} title="Sin métodos de pago configurados"
            body="Configura cómo te pueden pagar (efectivo, pago móvil, Zelle, tarjeta…). Mientras no haya ninguno, el Punto de Venta usa la lista estándar por defecto."
            cta={puedeEditar ? <Button icon={<Icon.Plus size={16} />} onClick={() => setForm({})}>Crear el primero</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Nombre</th>
                  <th className="py-2.5 pr-3 font-medium">Tipo</th>
                  <th className="py-2.5 pr-3 font-medium">Moneda</th>
                  <th className="py-2.5 pr-3 font-medium">Cuenta de cobro</th>
                  <th className="py-2.5 pr-3 font-medium text-center">En Caja</th>
                  <th className="py-2.5 pr-3 font-medium text-center">En Ventas</th>
                  <th className="py-2.5 pr-3 font-medium text-center">Activo</th>
                  {puedeEditar ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                </tr>
              </thead>
              <tbody>
                {rows.map((m) => {
                  const tm = tipoMeta(m.tipo)
                  const IconC = Icon[tm.icon] || Icon.Wallet
                  const bloqueado = busyId === m.id || !puedeEditar
                  return (
                    <tr key={m.id} className="border-b border-slate-100 dark:border-slate-800/70">
                      <td className="py-2.5 px-3 font-medium text-[13px]">{m.nombre}</td>
                      <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">
                        <span className="inline-flex items-center gap-1.5"><IconC size={14} className="text-slate-400" />{tm.label}</span>
                      </td>
                      <td className="py-2.5 pr-3"><Badge size="sm" color={m.moneda === 'USD' ? 'teal' : 'slate'}>{m.moneda === 'USD' ? 'US$' : 'Bs'}</Badge></td>
                      <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">
                        {nombreCuenta(m.cuentaCobroId) || <span className="text-slate-400">—</span>}
                      </td>
                      <td className="py-2.5 pr-3">
                        <div className="flex justify-center"><Toggle checked={!!m.enCaja} onChange={(v) => !bloqueado && patch(m, 'enCaja', v)} /></div>
                      </td>
                      <td className="py-2.5 pr-3">
                        <div className="flex justify-center"><Toggle checked={!!m.enVentas} onChange={(v) => !bloqueado && patch(m, 'enVentas', v)} /></div>
                      </td>
                      <td className="py-2.5 pr-3">
                        <div className="flex justify-center"><Toggle checked={!!m.activo} onChange={async (v) => {
                          if (bloqueado) return
                          // Desactivar es la acción destructiva (deja de cobrarse por ahí); pide confirmación.
                          // Reactivar no: no es destructivo.
                          if (!v && !(await confirm({
                            title: '¿Desactivar este método de pago?',
                            body: `«${m.nombre}» dejará de aparecer como opción de cobro en el Punto de Venta y en Ventas. Puedes reactivarlo cuando quieras.`,
                            confirmLabel: 'Desactivar', tone: 'danger',
                          }))) return
                          patch(m, 'activo', v)
                        }} /></div>
                      </td>
                      {puedeEditar ? (
                        <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                          <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />} onClick={() => setForm(m)}>Editar</Button>
                        </td>
                      ) : null}
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {!puedeEditar ? (
        <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>Solo la Dueña/Admin (o el Desarrollador) configura los métodos de pago. Aquí los ves, pero no puedes cambiarlos.</span>
        </div>
      ) : null}

      {form ? (
        <MetodoPagoForm metodo={form.id ? form : null} cuentas={cuentas}
          ordenSugerido={rows.length ? Math.max(...rows.map((r) => r.orden || 0)) + 1 : 0}
          onClose={() => setForm(null)}
          onSaved={(nombre) => { reload(); toast({ title: form.id ? 'Método actualizado' : 'Método creado', body: nombre }) }} />
      ) : null}
    </div>
  )
}

function MetodoPagoForm({ metodo, cuentas, ordenSugerido, onClose, onSaved }) {
  const edicion = !!metodo
  const [f, setF] = useState(() => ({
    nombre: metodo?.nombre || '',
    tipo: metodo?.tipo || 'efectivo_bs',
    moneda: metodo?.moneda || 'VES',
    cuentaCobroId: metodo?.cuentaCobroId || '',
    enCaja: metodo ? !!metodo.enCaja : true,
    enVentas: metodo ? !!metodo.enVentas : true,
    orden: metodo?.orden ?? ordenSugerido,
  }))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // Al elegir tipo, se sugiere la moneda que le corresponde (editable).
  const elegirTipo = (tipo) => {
    const tm = tipoMeta(tipo)
    setF((s) => ({ ...s, tipo, moneda: tm.moneda || s.moneda }))
  }

  const cuentasMoneda = cuentas.filter((c) => c.moneda === f.moneda)

  const guardar = async () => {
    if (!f.nombre.trim()) { setError('El nombre es obligatorio.'); return }
    setBusy(true); setError('')
    const payload = {
      nombre: f.nombre.trim(),
      tipo: f.tipo,
      moneda: f.moneda,
      cuentaCobroId: f.cuentaCobroId || '',
      enCaja: !!f.enCaja,
      enVentas: !!f.enVentas,
      orden: Number(f.orden) || 0,
    }
    try {
      if (edicion) await api.actualizarMetodoPago(metodo.id, payload)
      else await api.crearMetodoPago(payload)
      onSaved(payload.nombre)
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Wallet size={18} />}
      title={edicion ? 'Editar método de pago' : 'Nuevo método de pago'}
      sub="Cómo te pueden pagar en el Punto de Venta y en Ventas"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre" required error={error && !f.nombre.trim() ? error : ''}>
          <Input value={f.nombre} placeholder="Ej. Pago móvil Banesco"
            invalid={!!error && !f.nombre.trim()}
            onChange={(e) => { setF((s) => ({ ...s, nombre: e.target.value })); setError('') }} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Tipo" required>
            <Select value={f.tipo} onChange={(e) => elegirTipo(e.target.value)}>
              {TIPOS_METODO.map((t) => <option key={t.id} value={t.id}>{t.label}</option>)}
            </Select>
          </Field>
          <Field label="Moneda" required>
            <Select value={f.moneda} onChange={(e) => setF((s) => ({ ...s, moneda: e.target.value, cuentaCobroId: '' }))}>
              <option value="VES">Bolívares (Bs)</option>
              <option value="USD">Dólares (US$)</option>
            </Select>
          </Field>
        </div>
        <Field label="Cuenta de cobro" hint="opcional — dónde entra el dinero">
          <Select value={f.cuentaCobroId} onChange={(e) => setF((s) => ({ ...s, cuentaCobroId: e.target.value }))}>
            <option value="">{cuentasMoneda.length ? 'Sin cuenta asociada' : `Sin cuentas en ${f.moneda === 'USD' ? 'US$' : 'Bs'}`}</option>
            {cuentasMoneda.map((c) => <option key={c.id} value={c.id}>{c.titular}{c.datos ? ' · ' + c.datos : ''}</option>)}
          </Select>
        </Field>
        <Field label="Orden" hint="menor aparece primero">
          <Input type="number" min="0" step="1" value={f.orden}
            onChange={(e) => setF((s) => ({ ...s, orden: e.target.value }))} />
        </Field>
        <div className="rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 p-3.5 space-y-3">
          <Toggle checked={f.enCaja} onChange={(v) => setF((s) => ({ ...s, enCaja: v }))}
            label="Disponible en Punto de Venta" sub="Aparece como opción de cobro en el POS (Modo caja)." />
          <Toggle checked={f.enVentas} onChange={(v) => setF((s) => ({ ...s, enVentas: v }))}
            label="Disponible en Ventas" sub="Aparece como opción de pago en el módulo de Ventas." />
        </div>
        {error && f.nombre.trim() ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
      </div>
    </Modal>
  )
}

/* --- Unidades de medida -------------------------------------------------- */

// Categorías del maestro (deben coincidir con unidadmedida.Categoria* del
// backend: conteo | peso | volumen | longitud). `chip` es el color del Badge.
const CATEGORIAS_UNIDAD = [
  { id: 'conteo', label: 'Conteo', chip: 'slate' },
  { id: 'peso', label: 'Peso', chip: 'teal' },
  { id: 'volumen', label: 'Volumen', chip: 'sky' },
  { id: 'longitud', label: 'Longitud', chip: 'huberp' },
]
const catUnidadMeta = (c) => CATEGORIAS_UNIDAD.find((x) => x.id === c) || { label: c || '—', chip: 'slate' }
// Ver: quien entra a Ajustes (Dueña/Desarrollador/Contadora); editar: solo
// Dueña/Admin y Desarrollador — espeja el gate del backend. La interfaz oculta;
// el backend valida el rol en cada endpoint.
const PUEDE_EDITAR_UNIDADES = ['dueno', 'desarrollador']

function Unidades() {
  const { ui } = useUI()
  const { reload } = useData()
  const toast = useToast()
  const confirm = useConfirm()
  const puedeEditar = PUEDE_EDITAR_UNIDADES.includes(ui.rol)
  // El maestro COMPLETO (incluidas las inactivas) no está en el bootstrap
  // (db.UNIDADES trae solo las activas, para el select del producto): se pide
  // aparte a la API de configuración.
  const [rows, setRows] = useState(null) // null = cargando
  const [err, setErr] = useState(null)
  const [busyId, setBusyId] = useState('')
  const [form, setForm] = useState(null) // null | {} (nueva) | unidad (edición)

  const cargar = useCallback(async () => {
    setErr(null)
    try { setRows(await api.unidades()) }
    catch (e) { setErr(e); setRows([]) }
  }, [])
  useEffect(() => { cargar() }, [cargar])

  // Tras crear/editar/(des)activar se refresca la lista y el bootstrap (db.UNIDADES
  // alimenta el select de «Unidad base» del catálogo de producto).
  const refrescar = () => Promise.all([cargar(), reload()])

  const desactivar = async (u) => {
    if (!(await confirm({
      title: '¿Desactivar esta unidad?',
      body: `«${u.simbolo} — ${u.nombre}» dejará de ofrecerse al elegir la unidad base de un producto. No se borra: los productos que ya la usan la conservan y puedes reactivarla cuando quieras.`,
      confirmLabel: 'Desactivar', tone: 'danger',
    }))) return
    setBusyId(u.id)
    try {
      await api.desactivarUnidad(u.id)
      await refrescar()
      toast({ title: 'Unidad desactivada', body: `${u.simbolo} — ${u.nombre}` })
    } catch (e) {
      toast({ title: 'No se pudo desactivar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  const reactivar = async (u) => {
    setBusyId(u.id)
    try {
      await api.actualizarUnidad(u.id, { simbolo: u.simbolo, nombre: u.nombre, categoria: u.categoria, activa: true })
      await refrescar()
      toast({ title: 'Unidad reactivada', body: `${u.simbolo} — ${u.nombre}` })
    } catch (e) {
      toast({ title: 'No se pudo reactivar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  const lista = [...(rows || [])].sort((a, b) => (a.nombre || '').localeCompare(b.nombre || ''))

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          Cómo se cuentan y controlan tus productos: <strong>unidad</strong>, <strong>kg</strong>, <strong>litro</strong>, <strong>metro</strong>…
          Cada producto elige su <strong>unidad base</strong> de esta lista. Desactivar una unidad es
          <strong> reversible</strong>: los productos que ya la usan la conservan.
        </div>
        {puedeEditar ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setForm({})}>Nueva unidad</Button>
        ) : null}
      </div>

      {rows === null ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
          <TableSkeleton rows={5} cols={4} />
        </div>
      ) : err ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar las unidades"
          body={String(err?.message || err)}
          cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          {lista.length === 0 ? (
            <Empty icon={<Icon.Boxes size={22} />} title="Sin unidades de medida"
              body="Crea las unidades con las que vendes y controlas tus productos (unidad, kg, litro, metro…)."
              cta={puedeEditar ? <Button icon={<Icon.Plus size={16} />} onClick={() => setForm({})}>Crear la primera</Button> : null} />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                    <th className="py-2.5 px-3 font-medium">Símbolo</th>
                    <th className="py-2.5 pr-3 font-medium">Nombre</th>
                    <th className="py-2.5 pr-3 font-medium">Categoría</th>
                    <th className="py-2.5 pr-3 font-medium text-center">Estado</th>
                    {puedeEditar ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                  </tr>
                </thead>
                <tbody>
                  {lista.map((u) => {
                    const bloqueado = busyId === u.id || !puedeEditar
                    const meta = catUnidadMeta(u.categoria)
                    return (
                      <tr key={u.id} className={`border-b border-slate-100 dark:border-slate-800/70 ${u.activa ? '' : 'opacity-60'}`}>
                        <td className="py-2.5 px-3 font-medium text-[13px]"><span className="font-mono">{u.simbolo}</span></td>
                        <td className="py-2.5 pr-3 text-[12.5px]">{u.nombre}</td>
                        <td className="py-2.5 pr-3"><Badge size="sm" color={meta.chip}>{meta.label}</Badge></td>
                        <td className="py-2.5 pr-3 text-center">
                          <Badge size="sm" color={u.activa ? 'emerald' : 'slate'} dot>{u.activa ? 'Activa' : 'Inactiva'}</Badge>
                        </td>
                        {puedeEditar ? (
                          <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                            <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />} disabled={bloqueado} onClick={() => setForm(u)}>Editar</Button>
                            {u.activa ? (
                              <Button size="sm" variant="ghost" icon={<Icon.EyeOff size={14} />} disabled={bloqueado} onClick={() => desactivar(u)}>Desactivar</Button>
                            ) : (
                              <Button size="sm" variant="ghost" icon={<Icon.Refresh size={14} />} disabled={bloqueado} onClick={() => reactivar(u)}>Reactivar</Button>
                            )}
                          </td>
                        ) : null}
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {!puedeEditar ? (
        <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>Solo la Dueña/Admin (o el Desarrollador) administra las unidades de medida. Aquí las ves, pero no puedes cambiarlas.</span>
        </div>
      ) : null}

      {form ? (
        <UnidadForm unidad={form.id ? form : null}
          onClose={() => setForm(null)}
          onSaved={(nombre, edicion) => { refrescar(); toast({ title: edicion ? 'Unidad actualizada' : 'Unidad creada', body: nombre }) }} />
      ) : null}
    </div>
  )
}

function UnidadForm({ unidad, onClose, onSaved }) {
  const edicion = !!unidad
  const [f, setF] = useState(() => ({
    simbolo: unidad?.simbolo || '',
    nombre: unidad?.nombre || '',
    categoria: unidad?.categoria || 'conteo',
  }))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const guardar = async () => {
    if (!f.simbolo.trim()) { setError('El símbolo es obligatorio.'); return }
    if (!f.nombre.trim()) { setError('El nombre es obligatorio.'); return }
    setBusy(true); setError('')
    // Crear/editar deja la unidad ACTIVA (o conserva su estado al editar): la baja
    // va siempre por «Desactivar», nunca por este formulario.
    const payload = {
      simbolo: f.simbolo.trim(), nombre: f.nombre.trim(), categoria: f.categoria,
      activa: edicion ? (unidad.activa !== false) : true,
    }
    try {
      if (edicion) await api.actualizarUnidad(unidad.id, payload)
      else await api.crearUnidad(payload)
      onSaved(`${payload.simbolo} — ${payload.nombre}`, edicion)
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={<Icon.Boxes size={18} />}
      title={edicion ? 'Editar unidad de medida' : 'Nueva unidad de medida'}
      sub="La unidad base con la que se cuentan y controlan los productos"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Símbolo" required hint="p. ej. kg, L, m" error={error && !f.simbolo.trim() ? error : ''}>
            <Input value={f.simbolo} placeholder="kg" invalid={!!error && !f.simbolo.trim()} className="mono"
              onChange={(e) => { setF((s) => ({ ...s, simbolo: e.target.value })); setError('') }} />
          </Field>
          <Field label="Categoría" required>
            <Select value={f.categoria} onChange={(e) => setF((s) => ({ ...s, categoria: e.target.value }))}>
              {CATEGORIAS_UNIDAD.map((c) => <option key={c.id} value={c.id}>{c.label}</option>)}
            </Select>
          </Field>
        </div>
        <Field label="Nombre" required error={error && f.simbolo.trim() && !f.nombre.trim() ? error : ''}>
          <Input value={f.nombre} placeholder="Kilogramo" invalid={!!error && !!f.simbolo.trim() && !f.nombre.trim()}
            onChange={(e) => { setF((s) => ({ ...s, nombre: e.target.value })); setError('') }} />
        </Field>
        {error && f.simbolo.trim() && f.nombre.trim() ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
      </div>
    </Modal>
  )
}

/* --- Almacenes (depósitos por sede) ------------------------------------- */

const TIPOS_ALMACEN = [
  { id: 'principal', label: 'Principal', chip: 'huberp' },
  { id: 'general', label: 'General', chip: 'slate' },
  { id: 'transito', label: 'Tránsito', chip: 'sky' },
  { id: 'devoluciones', label: 'Devoluciones', chip: 'amber' },
  { id: 'materia_prima', label: 'Materia prima', chip: 'emerald' },
  { id: 'cuarentena', label: 'Cuarentena', chip: 'rose' },
  { id: 'refrigerado', label: 'Refrigerado', chip: 'blue' },
]
const tipoAlmacenMeta = (t) => TIPOS_ALMACEN.find((x) => x.id === t) || { label: t || 'General', chip: 'slate' }
const PUEDE_EDITAR_ALMACENES = ['dueno', 'desarrollador']

// BarraLlenado muestra qué tan lleno está un almacén respecto a su capacidad.
function BarraLlenado({ ocupado, capacidad, unidad }) {
  const pct = capacidad > 0 ? (Number(ocupado) || 0) / capacidad * 100 : 0
  const color = pct >= 100 ? 'bg-rose-500' : pct >= 80 ? 'bg-amber-500' : 'bg-emerald-500'
  return (
    <div className="min-w-[130px]">
      <div className="flex justify-between items-baseline text-[11px] text-slate-500 mb-0.5">
        <span className="num private-mask">{fmtNum(ocupado)} / {fmtNum(capacidad)} {unidad}</span>
        <span className={`num font-medium ${pct >= 100 ? 'text-rose-600 dark:text-rose-400' : ''}`}>{Math.round(pct)}%</span>
      </div>
      <div className="h-1.5 rounded-full bg-slate-200 dark:bg-slate-700 overflow-hidden">
        <div className={`h-full ${color} transition-[width]`} style={{ width: `${Math.min(100, pct)}%` }} />
      </div>
    </div>
  )
}

function Almacenes() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const confirm = useConfirm()
  const puedeEditar = PUEDE_EDITAR_ALMACENES.includes(ui.rol)
  const sedes = db.SEDES || []
  const sedeNombre = (id) => sedes.find((s) => s.id === id)?.nombre || '—'

  const [rows, setRows] = useState(null)
  const [err, setErr] = useState(null)
  const [busyId, setBusyId] = useState('')
  const [form, setForm] = useState(null)

  const cargar = useCallback(async () => {
    setErr(null)
    try { setRows(await api.almacenes()) }
    catch (e) { setErr(e); setRows([]) }
  }, [])
  useEffect(() => { cargar() }, [cargar])

  // Tras cualquier cambio se refresca la lista local Y el bootstrap (db.ALMACENES),
  // que es lo que consumen Existencias, Kardex y el modal de Transferencias. Sin
  // esto, un almacén recién creado no aparecía en esas pantallas hasta recargar.
  const refrescar = () => Promise.all([cargar(), reload()])

  const desactivar = async (a) => {
    if (!(await confirm({
      title: '¿Desactivar este almacén?',
      body: `«${a.nombre}» dejará de estar disponible para ubicar inventario. No se borra: los movimientos que ya lo referencian lo conservan y puedes reactivarlo cuando quieras.`,
      confirmLabel: 'Desactivar', tone: 'danger',
    }))) return
    setBusyId(a.id)
    try {
      await api.desactivarAlmacen(a.id)
      await refrescar()
      toast({ title: 'Almacén desactivado', body: a.nombre })
    } catch (e) {
      toast({ title: 'No se pudo desactivar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  const reactivar = async (a) => {
    setBusyId(a.id)
    try {
      await api.actualizarAlmacen(a.id, { nombre: a.nombre, tipo: a.tipo, principal: a.principal, activo: true })
      await refrescar()
      toast({ title: 'Almacén reactivado', body: a.nombre })
    } catch (e) {
      toast({ title: 'No se pudo reactivar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  const hacerPrincipal = async (a) => {
    setBusyId(a.id)
    try {
      await api.actualizarAlmacen(a.id, { nombre: a.nombre, tipo: a.tipo, principal: true })
      await refrescar()
      toast({ title: 'Almacén principal actualizado', body: `${a.nombre} · ${sedeNombre(a.sedeId)}` })
    } catch (e) {
      toast({ title: 'No se pudo cambiar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  // Orden: por sede (nombre) y dentro de cada sede el principal primero, luego nombre.
  const lista = [...(rows || [])].sort((a, b) => {
    const sa = sedeNombre(a.sedeId), sb = sedeNombre(b.sedeId)
    if (sa !== sb) return sa.localeCompare(sb)
    if (a.principal !== b.principal) return a.principal ? -1 : 1
    return (a.nombre || '').localeCompare(b.nombre || '')
  })

  // Alta/edición a PANTALLA COMPLETA (ocupa el área principal, no un modal).
  if (form) {
    return (
      <AlmacenForm almacen={form.id ? form : null} sedes={sedes} sedeNombre={sedeNombre}
        unidades={db.UNIDADES || []} rubros={db.RUBROS || []}
        onVolver={() => setForm(null)}
        onSaved={(nombre, edicion) => { refrescar(); setForm(null); toast({ title: edicion ? 'Almacén actualizado' : 'Almacén creado', body: nombre }) }} />
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          Los <strong>depósitos</strong> dentro de cada sede. Una sede puede tener varios almacenes y
          siempre tiene uno <strong>principal</strong> (del que despacha el punto de venta). Puedes fijarle
          <strong> capacidad</strong> y <strong>qué rubros admite</strong>. Desactivar es reversible.
        </div>
        {puedeEditar ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setForm({})} disabled={sedes.length === 0}>Nuevo almacén</Button>
        ) : null}
      </div>

      {rows === null ? (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
          <TableSkeleton rows={5} cols={5} />
        </div>
      ) : err ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudieron cargar los almacenes"
          body={String(err?.message || err)}
          cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          {lista.length === 0 ? (
            <Empty icon={<Icon.Package size={22} />} title="Sin almacenes"
              body="Crea el primer almacén de una sede. El primero de cada sede queda como principal."
              cta={puedeEditar && sedes.length ? <Button icon={<Icon.Plus size={16} />} onClick={() => setForm({})}>Crear el primero</Button> : null} />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                    <th className="py-2.5 px-3 font-medium">Almacén</th>
                    <th className="py-2.5 pr-3 font-medium">Sede</th>
                    <th className="py-2.5 pr-3 font-medium">Tipo</th>
                    <th className="py-2.5 pr-3 font-medium">Capacidad</th>
                    <th className="py-2.5 pr-3 font-medium text-center">Estado</th>
                    {puedeEditar ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                  </tr>
                </thead>
                <tbody>
                  {lista.map((a) => {
                    const bloqueado = busyId === a.id || !puedeEditar
                    const meta = tipoAlmacenMeta(a.tipo)
                    return (
                      <tr key={a.id} className={`border-b border-slate-100 dark:border-slate-800/70 ${a.activo ? '' : 'opacity-60'}`}>
                        <td className="py-2.5 px-3 font-medium text-[13px]">
                          {a.nombre}
                          {a.principal ? <Badge size="sm" color="huberp" className="ml-1.5">Principal</Badge> : null}
                        </td>
                        <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">{sedeNombre(a.sedeId)}</td>
                        <td className="py-2.5 pr-3"><Badge size="sm" color={meta.chip}>{meta.label}</Badge></td>
                        <td className="py-2.5 pr-3">
                          {a.capacidad > 0
                            ? <BarraLlenado ocupado={a.ocupado} capacidad={a.capacidad} unidad={a.capacidadUnidad} />
                            : <span className="text-[11.5px] text-slate-300 dark:text-slate-600">Sin límite</span>}
                        </td>
                        <td className="py-2.5 pr-3 text-center">
                          <Badge size="sm" color={a.activo ? 'emerald' : 'slate'} dot>{a.activo ? 'Activo' : 'Inactivo'}</Badge>
                        </td>
                        {puedeEditar ? (
                          <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                            {a.activo && !a.principal ? (
                              <Button size="sm" variant="ghost" icon={<Icon.Star size={14} />} disabled={bloqueado} onClick={() => hacerPrincipal(a)}>Hacer principal</Button>
                            ) : null}
                            <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />} disabled={bloqueado} onClick={() => setForm(a)}>Editar</Button>
                            {a.activo ? (
                              <Button size="sm" variant="ghost" icon={<Icon.EyeOff size={14} />} disabled={bloqueado || a.principal} title={a.principal ? 'Marca otro como principal primero' : ''} onClick={() => desactivar(a)}>Desactivar</Button>
                            ) : (
                              <Button size="sm" variant="ghost" icon={<Icon.Refresh size={14} />} disabled={bloqueado} onClick={() => reactivar(a)}>Reactivar</Button>
                            )}
                          </td>
                        ) : null}
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {puedeEditar && sedes.length === 0 ? (
        <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300">
          Primero crea al menos una sede (pestaña <strong>Sedes</strong>); los almacenes cuelgan de una sede.
        </div>
      ) : null}

    </div>
  )
}

function AlmacenForm({ almacen, sedes, sedeNombre, unidades, rubros, onVolver, onSaved }) {
  const edicion = !!almacen
  const [f, setF] = useState(() => ({
    sedeId: almacen?.sedeId || (sedes[0]?.id || ''),
    nombre: almacen?.nombre || '',
    tipo: almacen?.tipo || 'general',
    principal: !!almacen?.principal,
    capacidad: almacen?.capacidad ? String(almacen.capacidad) : '',
    capacidadUnidad: almacen?.capacidadUnidad || '',
    rubrosAdmitidos: Array.isArray(almacen?.rubrosAdmitidos) ? almacen.rubrosAdmitidos : [],
  }))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const cap = Number(f.capacidad) || 0
  const toggleRubro = (id) => setF((s) => ({
    ...s,
    rubrosAdmitidos: s.rubrosAdmitidos.includes(id)
      ? s.rubrosAdmitidos.filter((x) => x !== id)
      : [...s.rubrosAdmitidos, id],
  }))

  const guardar = async () => {
    if (!f.sedeId) { setError('Elige la sede.'); return }
    if (!f.nombre.trim()) { setError('El nombre es obligatorio.'); return }
    if (cap > 0 && !f.capacidadUnidad) { setError('Indica la unidad de la capacidad (p. ej. kg o L).'); return }
    setBusy(true); setError('')
    const payload = {
      sedeId: f.sedeId, nombre: f.nombre.trim(), tipo: f.tipo, principal: f.principal,
      capacidad: cap, capacidadUnidad: cap > 0 ? f.capacidadUnidad : '',
      rubrosAdmitidos: f.rubrosAdmitidos,
    }
    try {
      if (edicion) await api.actualizarAlmacen(almacen.id, { ...payload, activo: almacen.activo !== false })
      else await api.crearAlmacen(payload)
      onSaved(f.nombre.trim(), edicion)
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
      setBusy(false)
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-4">
        <div className="flex items-center gap-2 min-w-0">
          <button onClick={onVolver} title="Volver" className="h-8 w-8 inline-flex items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 ring-focus">
            <Icon.ChevLeft size={18} />
          </button>
          <div className="min-w-0">
            <div className="text-[15px] font-semibold text-slate-800 dark:text-slate-100">{edicion ? 'Editar almacén' : 'Nuevo almacén'}</div>
            <div className="text-[12px] text-slate-500 truncate">Un depósito dentro de una sede: capacidad y rubros admitidos.</div>
          </div>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button variant="ghost" onClick={onVolver}>Cancelar</Button>
          <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Guardar</Button>
        </div>
      </div>

      <div className="max-w-3xl space-y-5">
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3.5">
          <div className="text-[11px] uppercase tracking-wide text-slate-400 font-medium">Identidad</div>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <Field label="Sede" required hint={edicion ? 'no se cambia' : ''}>
              {edicion ? (
                <Input value={sedeNombre(f.sedeId)} disabled />
              ) : (
                <Select value={f.sedeId} onChange={(e) => setF((s) => ({ ...s, sedeId: e.target.value }))}>
                  {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
                </Select>
              )}
            </Field>
            <Field label="Nombre" required error={error && !f.nombre.trim() ? error : ''}>
              <Input value={f.nombre} placeholder="Depósito, Cava fría…" invalid={!!error && !f.nombre.trim()}
                onChange={(e) => { setF((s) => ({ ...s, nombre: e.target.value })); setError('') }} />
            </Field>
            <Field label="Tipo">
              <Select value={f.tipo} onChange={(e) => setF((s) => ({ ...s, tipo: e.target.value }))}>
                {TIPOS_ALMACEN.map((t) => <option key={t.id} value={t.id}>{t.label}</option>)}
              </Select>
            </Field>
          </div>
          <label className="flex items-center gap-2 text-[13px] text-slate-600 dark:text-slate-300 cursor-pointer">
            <input type="checkbox" checked={f.principal} onChange={(e) => setF((s) => ({ ...s, principal: e.target.checked }))} className="accent-elerp-600 w-4 h-4" />
            <span>Almacén <strong>principal</strong> de la sede (de él despacha el punto de venta). Marcarlo desmarca al anterior.</span>
          </label>
        </div>

        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3.5">
          <div className="text-[11px] uppercase tracking-wide text-slate-400 font-medium">Capacidad (opcional)</div>
          <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
            Límite físico del almacén. Se muestra como barra de llenado; no bloquea. La ocupación cuenta la
            existencia de los productos medidos en esa unidad (peso o volumen).
          </div>
          <div className="grid grid-cols-2 gap-3 max-w-sm">
            <Field label="Capacidad">
              <Input type="number" min="0" step="any" value={f.capacidad} placeholder="0 = sin límite"
                onChange={(e) => { setF((s) => ({ ...s, capacidad: e.target.value })); setError('') }} />
            </Field>
            <Field label="Unidad" required={cap > 0} error={error && cap > 0 && !f.capacidadUnidad ? error : ''}>
              <Select value={f.capacidadUnidad} onChange={(e) => setF((s) => ({ ...s, capacidadUnidad: e.target.value }))} disabled={cap <= 0}>
                <option value="">—</option>
                {unidades.map((u) => <option key={u.id || u.simbolo} value={u.simbolo}>{u.simbolo} · {u.nombre}</option>)}
              </Select>
            </Field>
          </div>
        </div>

        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 space-y-3">
          <div className="text-[11px] uppercase tracking-wide text-slate-400 font-medium">Rubros admitidos (opcional)</div>
          <div className="text-[12.5px] text-slate-500 dark:text-slate-400">
            Si eliges rubros, el almacén <strong>solo</strong> aceptará productos de esos rubros (al ajustar o
            transferir). Sin selección, admite todos.
          </div>
          {rubros.length === 0 ? (
            <div className="text-[12.5px] text-slate-400">No hay rubros definidos.</div>
          ) : (
            <div className="flex flex-wrap gap-2">
              {rubros.map((r) => {
                const on = f.rubrosAdmitidos.includes(r.id)
                return (
                  <button key={r.id} type="button" onClick={() => toggleRubro(r.id)}
                    className={`px-2.5 py-1 rounded-full text-[12.5px] border transition-colors ${on
                      ? 'bg-elerp-500 border-elerp-500 text-white'
                      : 'bg-white dark:bg-slate-900 border-slate-300 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:border-elerp-400'}`}>
                    {on ? '✓ ' : ''}{r.nombre}
                  </button>
                )
              })}
            </div>
          )}
          {f.rubrosAdmitidos.length ? (
            <div className="text-[11.5px] text-slate-400">Admite {f.rubrosAdmitidos.length} rubro(s). <button type="button" className="underline" onClick={() => setF((s) => ({ ...s, rubrosAdmitidos: [] }))}>Quitar todos (admitir cualquiera)</button></div>
          ) : null}
        </div>

        {error ? <div className="text-[12.5px] text-red-600 dark:text-red-400">{error}</div> : null}
      </div>
    </div>
  )
}

/* --- Dispositivos fiscales ---------------------------------------------- */

const TIPOS_DISPOSITIVO = [
  { id: 'impresora_fiscal', label: 'Impresora fiscal', chip: 'huberp' },
  { id: 'balanza', label: 'Balanza', chip: 'sky' },
  { id: 'otro', label: 'Otro dispositivo', chip: 'slate' },
]
const tipoDispMeta = (t) => TIPOS_DISPOSITIVO.find((x) => x.id === t) || { label: t || 'Otro dispositivo', chip: 'slate' }
// Ícono por tipo: la balanza se distingue de la impresora de un vistazo.
const iconoDisp = (t) => (t === 'balanza' ? <Icon.Activity size={15} className="text-slate-400 shrink-0" /> : <Icon.Printer size={15} className="text-slate-400 shrink-0" />)
// Protocolos de comunicación típicos de balanzas de mostrador en Venezuela. El
// valor guardado siempre queda seleccionable aunque no esté en la lista.
const PROTOCOLOS_BALANZA = [
  { id: 'continuo', label: 'Continuo (peso en vivo)' },
  { id: 'pedido', label: 'Bajo pedido (ENQ/poll)' },
  { id: 'toledo', label: 'Toledo' },
  { id: 'systel', label: 'Systel' },
  { id: 'kretz', label: 'Kretz' },
  { id: 'aclas', label: 'Aclas' },
]
// Solo la Dueña/Admin y el Desarrollador administran los dispositivos. La
// interfaz oculta las acciones; el backend es el que de verdad protege.
const PUEDE_EDITAR_DISPOSITIVOS = ['dueno', 'desarrollador']

function Dispositivos() {
  const { db, reload } = useData()
  const { ui } = useUI()
  const toast = useToast()
  const confirm = useConfirm()
  const [busyId, setBusyId] = useState('')
  const [form, setForm] = useState(null) // null | {} (nuevo) | dispositivo (edición)
  const puedeEditar = PUEDE_EDITAR_DISPOSITIVOS.includes(ui.rol)

  const sedes = db.SEDES || []
  const nombreSede = (id) => (id ? (sedes.find((s) => s.id === id)?.nombre || 'Sede desconocida') : null)
  const rows = [...(db.DISPOSITIVOS || [])].sort((a, b) => (a.nombre || '').localeCompare(b.nombre || ''))

  const desactivar = async (d) => {
    if (!(await confirm({
      title: '¿Desactivar este dispositivo?',
      body: `«${d.nombre}» quedará marcado como inactivo. No se borra: puedes reactivarlo cuando quieras.`,
      confirmLabel: 'Desactivar', tone: 'danger',
    }))) return
    setBusyId(d.id)
    try {
      await api.desactivarDispositivo(d.id)
      await reload()
      toast({ title: 'Dispositivo desactivado', body: d.nombre })
    } catch (e) {
      toast({ title: 'No se pudo desactivar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  const reactivar = async (d) => {
    setBusyId(d.id)
    try {
      await api.actualizarDispositivo(d.id, { activo: true })
      await reload()
      toast({ title: 'Dispositivo reactivado', body: d.nombre })
    } catch (e) {
      toast({ title: 'No se pudo reactivar', body: e?.message || 'Error', kind: 'error' })
    } finally { setBusyId('') }
  }

  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-3">
        <div className="text-[12.5px] text-slate-500 dark:text-slate-400 max-w-2xl">
          Registra y administra aquí tus impresoras fiscales, balanzas y otros dispositivos.
          La <strong>conexión real</strong> con el hardware la hace el <strong>agente fiscal local</strong> (un
          puente aparte, fuera de esta app); acá solo queda su ficha e identificación.
        </div>
        {puedeEditar ? (
          <Button size="sm" icon={<Icon.Plus size={15} />} onClick={() => setForm({})}>Nuevo dispositivo</Button>
        ) : null}
      </div>

      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
        {rows.length === 0 ? (
          <Empty icon={<Icon.Printer size={22} />} title="Sin dispositivos registrados"
            body="Registra tus impresoras fiscales (marca, modelo, serial y sede) y tus balanzas (puerto y protocolo) para tenerlas administradas en un solo lugar. La conexión con el hardware la hace el agente fiscal local."
            cta={puedeEditar ? <Button icon={<Icon.Plus size={16} />} onClick={() => setForm({})}>Registrar el primero</Button> : null} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-2.5 px-3 font-medium">Nombre</th>
                  <th className="py-2.5 pr-3 font-medium">Tipo</th>
                  <th className="py-2.5 pr-3 font-medium">Marca / Modelo</th>
                  <th className="py-2.5 pr-3 font-medium">Serie / Conexión</th>
                  <th className="py-2.5 pr-3 font-medium">Sede</th>
                  <th className="py-2.5 pr-3 font-medium text-center">Estado</th>
                  {puedeEditar ? <th className="py-2.5 pr-3 font-medium text-right">Acciones</th> : null}
                </tr>
              </thead>
              <tbody>
                {rows.map((d) => {
                  const bloqueado = busyId === d.id || !puedeEditar
                  const marcaModelo = [d.marca, d.modelo].filter(Boolean).join(' · ')
                  const meta = tipoDispMeta(d.tipo)
                  const esBalanza = d.tipo === 'balanza'
                  const conexion = [d.puerto, d.protocolo].filter(Boolean).join(' · ')
                  return (
                    <tr key={d.id} className={`border-b border-slate-100 dark:border-slate-800/70 ${d.activo ? '' : 'opacity-60'}`}>
                      <td className="py-2.5 px-3 font-medium text-[13px]">
                        <span className="inline-flex items-center gap-2">
                          {iconoDisp(d.tipo)}{d.nombre}
                        </span>
                      </td>
                      <td className="py-2.5 pr-3"><Badge size="sm" color={meta.chip}>{meta.label}</Badge></td>
                      <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">{marcaModelo || <span className="text-slate-400">—</span>}</td>
                      <td className="py-2.5 pr-3 text-[12.5px]">
                        {esBalanza
                          ? (conexion ? <span className="font-mono text-[12px]">{conexion}</span> : <span className="text-slate-400">—</span>)
                          : (d.serie ? <span className="font-mono text-[12px]">{d.serie}</span> : <span className="text-slate-400">—</span>)}
                      </td>
                      <td className="py-2.5 pr-3 text-[12.5px] text-slate-500">{nombreSede(d.sedeId) || <span className="text-slate-400">Toda la empresa</span>}</td>
                      <td className="py-2.5 pr-3 text-center">
                        <Badge size="sm" color={d.activo ? 'emerald' : 'slate'} dot>{d.activo ? 'Activo' : 'Inactivo'}</Badge>
                      </td>
                      {puedeEditar ? (
                        <td className="py-2.5 pr-3 text-right whitespace-nowrap">
                          <Button size="sm" variant="ghost" icon={<Icon.Pencil size={14} />} disabled={bloqueado} onClick={() => setForm(d)}>Editar</Button>
                          {d.activo ? (
                            <Button size="sm" variant="ghost" icon={<Icon.EyeOff size={14} />} disabled={bloqueado} onClick={() => desactivar(d)}>Desactivar</Button>
                          ) : (
                            <Button size="sm" variant="ghost" icon={<Icon.Refresh size={14} />} disabled={bloqueado} onClick={() => reactivar(d)}>Reactivar</Button>
                          )}
                        </td>
                      ) : null}
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {!puedeEditar ? (
        <div className="mt-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 px-3.5 py-3 text-[12.5px] text-slate-600 dark:text-slate-300 flex gap-2.5 items-start">
          <Icon.Shield size={15} className="mt-0.5 shrink-0 text-slate-400" />
          <span>Solo la Dueña/Admin (o el Desarrollador) administra los dispositivos fiscales. Aquí los ves, pero no puedes cambiarlos.</span>
        </div>
      ) : null}

      {form ? (
        <DispositivoForm dispositivo={form.id ? form : null} sedes={sedes}
          onClose={() => setForm(null)}
          onSaved={(nombre, edicion) => { reload(); toast({ title: edicion ? 'Dispositivo actualizado' : 'Dispositivo registrado', body: nombre }) }} />
      ) : null}
    </div>
  )
}

function DispositivoForm({ dispositivo, sedes, onClose, onSaved }) {
  const edicion = !!dispositivo
  const [f, setF] = useState(() => ({
    nombre: dispositivo?.nombre || '',
    tipo: dispositivo?.tipo || 'impresora_fiscal',
    marca: dispositivo?.marca || '',
    modelo: dispositivo?.modelo || '',
    serie: dispositivo?.serie || '',
    puerto: dispositivo?.puerto || '',
    protocolo: dispositivo?.protocolo || '',
    sedeId: dispositivo?.sedeId || '',
  }))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const esBalanza = f.tipo === 'balanza'
  // El protocolo guardado siempre queda seleccionable aunque no esté en la lista.
  const protocolos = PROTOCOLOS_BALANZA.some((p) => p.id === f.protocolo) || !f.protocolo
    ? PROTOCOLOS_BALANZA
    : [...PROTOCOLOS_BALANZA, { id: f.protocolo, label: f.protocolo }]

  const guardar = async () => {
    if (!f.nombre.trim()) { setError('El nombre es obligatorio.'); return }
    setBusy(true); setError('')
    const payload = {
      nombre: f.nombre.trim(),
      tipo: f.tipo,
      marca: f.marca.trim(),
      modelo: f.modelo.trim(),
      sedeId: f.sedeId || '',
      // La balanza se comunica por puerto+protocolo; la impresora por su serial.
      ...(esBalanza
        ? { puerto: f.puerto.trim(), protocolo: f.protocolo.trim() }
        : { serie: f.serie.trim() }),
    }
    try {
      if (edicion) await api.actualizarDispositivo(dispositivo.id, payload)
      else await api.crearDispositivo(payload)
      onSaved(payload.nombre, edicion)
      onClose()
    } catch (e) {
      setError(e?.message || 'No se pudo guardar.')
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} size="sm" icon={esBalanza ? <Icon.Activity size={18} /> : <Icon.Printer size={18} />}
      title={edicion ? 'Editar dispositivo' : 'Nuevo dispositivo'}
      sub="La conexión real la hace el agente fiscal local; aquí se registra su ficha"
      footer={<>
        <Button variant="ghost" onClick={onClose}>Cancelar</Button>
        <Button onClick={guardar} loading={busy} icon={<Icon.Check size={16} />}>Guardar</Button>
      </>}>
      <div className="space-y-3.5">
        <Field label="Nombre" required error={error && !f.nombre.trim() ? error : ''}>
          <Input value={f.nombre} placeholder="Ej. Impresora fiscal caja 1"
            invalid={!!error && !f.nombre.trim()}
            onChange={(e) => { setF((s) => ({ ...s, nombre: e.target.value })); setError('') }} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Tipo" required>
            <Select value={f.tipo} onChange={(e) => setF((s) => ({ ...s, tipo: e.target.value }))}>
              {TIPOS_DISPOSITIVO.map((t) => <option key={t.id} value={t.id}>{t.label}</option>)}
            </Select>
          </Field>
          <Field label="Sede" hint="opcional — vacío = toda la empresa">
            <Select value={f.sedeId} onChange={(e) => setF((s) => ({ ...s, sedeId: e.target.value }))}>
              <option value="">Toda la empresa</option>
              {sedes.map((s) => <option key={s.id} value={s.id}>{s.nombre}</option>)}
            </Select>
          </Field>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Marca" hint="opcional">
            <Input value={f.marca} placeholder="Ej. The Factory HKA"
              onChange={(e) => setF((s) => ({ ...s, marca: e.target.value }))} />
          </Field>
          <Field label="Modelo" hint="opcional">
            <Input value={f.modelo} placeholder="Ej. PP-9"
              onChange={(e) => setF((s) => ({ ...s, modelo: e.target.value }))} />
          </Field>
        </div>
        {esBalanza ? (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Puerto" hint="opcional — dónde la ve el equipo">
              <Input value={f.puerto} placeholder="Ej. COM3 · /dev/ttyUSB0"
                onChange={(e) => setF((s) => ({ ...s, puerto: e.target.value }))} className="mono" />
            </Field>
            <Field label="Protocolo" hint="opcional — cómo entrega el peso">
              <Select value={f.protocolo} onChange={(e) => setF((s) => ({ ...s, protocolo: e.target.value }))}>
                <option value="">Sin especificar</option>
                {protocolos.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
              </Select>
            </Field>
          </div>
        ) : (
          <Field label="Serie / serial fiscal" hint="opcional — debe ser único en la empresa">
            <Input value={f.serie} placeholder="Ej. Z1B0000123"
              onChange={(e) => setF((s) => ({ ...s, serie: e.target.value }))} />
          </Field>
        )}
        {error && f.nombre.trim() ? <div className="text-[12px] text-red-600 dark:text-red-400">{error}</div> : null}
      </div>
    </Modal>
  )
}

/* --- Integraciones ------------------------------------------------------- */

// Estados honestos de cada integración. «Conectado» solo se pinta cuando de
// verdad está funcionando (la app corre sobre Hubmy SSO); «Disponible» cuando la
// capacidad existe pero se administra fuera de esta app; «Próximamente» cuando
// todavía no está construida — nunca un «Conectado» que sería mentira.
const INTEG_COLOR = { conectado: 'emerald', disponible: 'sky', proximamente: 'slate', activa: 'emerald', parcial: 'amber', falla: 'amber' }
const INTEG_TXT = { conectado: 'Conectado', disponible: 'Disponible', proximamente: 'Próximamente', activa: 'Activa', parcial: 'Con reservas', falla: 'Revisar' }

function Integraciones() {
  const tasa = useTasa()
  const filas = [
    {
      icon: <Icon.Shield size={19} />,
      t: 'Hubmy — Autenticación (SSO) e IA',
      s: 'El inicio de sesión y el Asistente de IA funcionan sobre Hubmy.',
      estado: 'conectado',
      nota: 'La app corre sobre Hubmy SSO. El Asistente de IA usa el proxy de IA de Hubmy (se cobra contra el saldo Hubmy; requiere credenciales en el servidor).',
    },
    {
      icon: <Icon.Bank size={19} />,
      t: 'Tasa BCV automática',
      s: tasa.hay
        ? `Última: Bs ${fmtNum(tasa.valor, 2)} · ${tasa.fuenteLabel} · ${fechaCortaVE(tasa.fechaValor, tasa.esDeHoy)}`
        : 'Sin tasa cargada todavía.',
      estado: tasa.oficial ? 'activa' : tasa.hay ? 'parcial' : 'falla',
      nota: tasa.ultimoError ? explicarFallo(tasa.ultimoError) : 'Se consulta una vez al día, a partir de las 9 am.',
    },
    {
      icon: <Icon.Send size={19} />,
      t: 'Webhooks salientes (HMAC)',
      s: 'Notifica a sistemas externos cada hecho de negocio, firmado con HMAC-SHA256.',
      estado: 'disponible',
      nota: 'Se configuran desde el panel de Hubmy. Su administración dentro de la app llega próximamente.',
      accion: 'Configurar en Hubmy',
    },
    {
      icon: <Icon.Globe size={19} />,
      t: 'Hubmy Pages / Deploy',
      s: 'Publicar sitios y páginas de la empresa desde ElERP.',
      estado: 'proximamente',
      nota: 'Sin construir: se habilitará cuando el módulo de publicación esté disponible.',
      accion: 'Próximamente',
    },
    {
      icon: <Icon.Package size={19} />,
      t: 'Marketplace de módulos',
      s: 'Instalar módulos y extensiones adicionales al ERP.',
      estado: 'proximamente',
      nota: 'Sin construir: se activa cuando existan módulos instalables.',
      accion: 'Próximamente',
    },
  ]

  return (
    <div className="space-y-2.5">
      {filas.map((f) => (
        <div key={f.t} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4 flex items-start gap-3">
          <span className="text-slate-500 mt-0.5 shrink-0">{f.icon}</span>
          <div className="flex-1 min-w-0">
            <div className="text-[13.5px] font-semibold">{f.t}</div>
            <div className="text-[12px] text-slate-500">{f.s}</div>
            <div className="text-[11.5px] text-slate-400 mt-1">{f.nota}</div>
          </div>
          <div className="flex flex-col items-end gap-2 shrink-0">
            <Badge size="sm" color={INTEG_COLOR[f.estado]} dot>{INTEG_TXT[f.estado]}</Badge>
            {f.accion ? (
              <Button size="sm" variant="ghost" disabled title="Se administra fuera de esta app">{f.accion}</Button>
            ) : null}
          </div>
        </div>
      ))}
      <div className="text-[11.5px] text-slate-400 px-1">
        Solo se muestra «Conectado» cuando la integración está realmente en uso. Lo demás indica con honestidad qué falta.
      </div>
    </div>
  )
}
