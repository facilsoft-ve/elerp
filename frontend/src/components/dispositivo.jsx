import { useState, useEffect, useMemo } from 'react'
import { Field, Select, Input } from './primitives.jsx'
import { api } from '../lib/api.js'

/* Selector de MARCA y MODELO contra el catálogo precargado del backend
 * (domain/dispositivo): impresoras fiscales homologadas, balanzas de mostrador
 * y comanderas térmicas del mercado venezolano.
 *
 * POR QUÉ: antes esto eran dos campos de texto libre. Quien configura una
 * impresora fiscal en una bodega no tiene por qué saber escribir "The Factory
 * HKA" igual que el vecino, ni saberse de memoria el protocolo de su balanza.
 * Con el catálogo, elegir el modelo PRECARGA la conexión y deja el maestro
 * normalizado (el backend vuelve a normalizar: la interfaz solo facilita).
 *
 * EL CATÁLOGO NO ES UNA REJA: siempre queda la salida "Otra marca…" para el
 * equipo que todavía no está en la lista. El mercado va por delante de la lista
 * y ningún negocio puede quedarse sin configurar su máquina esperándonos.
 */

// Valor centinela de la opción "otra/otro": no puede chocar con una marca real.
const OTRA = '__otra__'

/* El catálogo es dato de referencia, idéntico para todas las empresas y que no
 * cambia durante la sesión: se pide UNA vez por carga de la app y se comparte.
 * La promesa se cachea (no el resultado) para que dos formularios abiertos a la
 * vez no disparen dos peticiones. */
let catalogoPromesa = null
const cargarCatalogo = () => {
  if (!catalogoPromesa) {
    catalogoPromesa = api.catalogoDispositivos().catch((e) => {
      // Un fallo no se cachea: el siguiente intento vuelve a pedirlo. Si el
      // catálogo no llega, el formulario sigue funcionando con campos libres.
      catalogoPromesa = null
      throw e
    })
  }
  return catalogoPromesa
}

/* useCatalogoDispositivos devuelve los modelos de UN tipo
 * ('impresora_fiscal' | 'balanza' | 'comandera') más la fecha de revisión.
 * Nunca lanza: si el catálogo no carga, devuelve lista vacía y el formulario
 * cae con gracia a texto libre. */
export function useCatalogoDispositivos(tipo) {
  const [cat, setCat] = useState({ revisado: '', modelos: [] })

  useEffect(() => {
    let vivo = true
    cargarCatalogo()
      .then((r) => { if (vivo) setCat({ revisado: r?.revisado || '', modelos: r?.modelos || [] }) })
      .catch(() => { if (vivo) setCat({ revisado: '', modelos: [] }) })
    return () => { vivo = false }
  }, [])

  return useMemo(() => {
    const modelos = (cat.modelos || []).filter((m) => m.tipo === tipo)
    const marcas = []
    for (const m of modelos) if (!marcas.includes(m.marca)) marcas.push(m.marca)
    return { revisado: cat.revisado, modelos, marcas }
  }, [cat, tipo])
}

/* SelectorModelo pinta los dos campos (Marca, Modelo) y avisa con la ficha
 * completa del catálogo cuando se elige un modelo conocido, para que cada
 * pantalla decida qué precargar: la balanza toma protocolo y puerto; la
 * comandera, el ancho del papel y el modo de conexión.
 *
 * props:
 *   tipo      — 'impresora_fiscal' | 'balanza' | 'comandera'
 *   marca, modelo — valores actuales (el formulario sigue siendo el dueño)
 *   onChange({ marca, modelo }, fichaDelCatalogo|null)
 */
export function SelectorModelo({ tipo, marca, modelo, onChange, disabled = false }) {
  const { modelos, marcas } = useCatalogoDispositivos(tipo)

  // "Libre" es cuando lo guardado no está en el catálogo: puede ser un equipo
  // nuevo o una ficha vieja anterior al catálogo. En ambos casos se muestra tal
  // cual, en modo texto, sin perder lo que ya estaba escrito.
  const marcaCatalogada = marcas.some((m) => m.toLowerCase() === (marca || '').trim().toLowerCase())
  const [libre, setLibre] = useState(() => !!(marca || '').trim() && !marcaCatalogada)

  // Mientras el catálogo viaja, `marcas` está vacío y TODO parecería libre. Solo
  // se decide cuando ya hay catálogo; si no, un valor guardado saltaría a texto
  // libre por un instante y volvería, que se ve como un parpadeo.
  useEffect(() => {
    if (marcas.length === 0) return
    setLibre(!!(marca || '').trim() && !marcaCatalogada)
    // Solo al llegar el catálogo: después manda lo que la persona elija.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [marcas.length])

  const modelosDeMarca = modelos.filter((m) => m.marca.toLowerCase() === (marca || '').trim().toLowerCase())
  const fichaActual = modelosDeMarca.find((m) => m.modelo.toLowerCase() === (modelo || '').trim().toLowerCase()) || null

  const elegirMarca = (v) => {
    if (v === OTRA) { setLibre(true); onChange({ marca: '', modelo: '' }, null); return }
    // Cambiar de marca invalida el modelo: una HKA80 no es una marca Bematech.
    onChange({ marca: v, modelo: '' }, null)
  }

  const elegirModelo = (v) => {
    const ficha = modelosDeMarca.find((m) => m.modelo === v) || null
    onChange({ marca, modelo: v }, ficha)
  }

  // Sin catálogo (aún cargando o falló la petición): texto libre, como antes.
  if (marcas.length === 0 || libre) {
    return (
      <div className="space-y-2">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Marca" hint={libre && marcas.length > 0 ? 'fuera del catálogo' : 'opcional'}>
            <Input value={marca || ''} disabled={disabled} placeholder="Ej. The Factory HKA"
              onChange={(e) => onChange({ marca: e.target.value, modelo: modelo || '' }, null)} />
          </Field>
          <Field label="Modelo" hint="opcional">
            <Input value={modelo || ''} disabled={disabled} placeholder="Ej. HKA80"
              onChange={(e) => onChange({ marca: marca || '', modelo: e.target.value }, null)} />
          </Field>
        </div>
        {/* La vuelta al catálogo va FUERA de la etiqueta del campo: un botón
            dentro de un <label> es ruido para quien navega con lector. */}
        {marcas.length > 0 ? (
          <button type="button" disabled={disabled}
            className="text-[11.5px] text-slate-500 dark:text-slate-400 underline hover:no-underline"
            onClick={() => { setLibre(false); onChange({ marca: '', modelo: '' }, null) }}>
            Elegir del catálogo
          </button>
        ) : null}
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="grid grid-cols-2 gap-3">
        <Field label="Marca" hint="opcional">
          <Select value={marcaCatalogada ? marca : ''} disabled={disabled} onChange={(e) => elegirMarca(e.target.value)}>
            <option value="">Sin especificar</option>
            {marcas.map((m) => <option key={m} value={m}>{m}</option>)}
            <option value={OTRA}>Otra marca…</option>
          </Select>
        </Field>
        <Field label="Modelo" hint={marcaCatalogada ? 'opcional' : 'elige la marca primero'}>
          <Select value={fichaActual ? fichaActual.modelo : ''} disabled={disabled || !marcaCatalogada}
            onChange={(e) => elegirModelo(e.target.value)}>
            <option value="">Sin especificar</option>
            {modelosDeMarca.map((m) => <option key={m.modelo} value={m.modelo}>{m.modelo}</option>)}
          </Select>
        </Field>
      </div>
      {fichaActual?.nota || fichaActual?.tecnologia ? (
        <div className="text-[11.5px] text-slate-500 dark:text-slate-400">
          {fichaActual.tecnologia ? <span className="capitalize">{fichaActual.tecnologia === 'termica' ? 'Térmica' : 'Matricial'}</span> : null}
          {fichaActual.tecnologia && fichaActual.nota ? ' · ' : ''}
          {fichaActual.nota || ''}
        </div>
      ) : null}
    </div>
  )
}

/* NotaCatalogo dice de cuándo es la lista. Va al pie del formulario porque la
 * homologación del SENIAT es un trámite vivo: la lista AYUDA a elegir, no
 * certifica que un equipo esté homologado hoy. Decirlo es parte del trabajo. */
export function NotaCatalogo({ tipo }) {
  const { revisado, modelos } = useCatalogoDispositivos(tipo)
  if (!revisado || modelos.length === 0) return null
  const fiscal = tipo === 'impresora_fiscal'
  return (
    <div className="text-[11.5px] text-slate-400 dark:text-slate-500">
      {modelos.length} modelos precargados · lista revisada el {revisado}.
      {fiscal ? ' La homologación vigente la define el SENIAT; esta lista solo ayuda a elegir.' : ''}
      {' '}¿No está el tuyo? Elige <strong>«Otra marca…»</strong> y escríbelo.
    </div>
  )
}
