import { useState, useEffect, useMemo } from 'react'
import { Field, Select, Input } from './primitives.jsx'
import { api } from '../lib/api.js'
import { fmtNum } from '../lib/format.js'

/* Selector de CONCEPTO ISLR contra el maestro de la empresa.
 *
 * POR QUÉ: la tarifa de ISLR la fija el reglamento y depende del CONCEPTO y de a
 * QUIÉN se le retiene (honorarios: 3% a persona natural, 5% a jurídica). Antes
 * el concepto era texto libre y el porcentaje se tecleaba, así que quien
 * registraba la retención tenía que saberse la tabla de memoria — y un dedo de
 * más se convierte en un impuesto mal enterado al SENIAT. Con el maestro se
 * ELIGE y la tarifa viene con el concepto.
 *
 * EL CÁLCULO ES DEL SERVIDOR. Esta pantalla no multiplica nada: pide la
 * sugerencia y la muestra. Si la fórmula viviera también acá, tarde o temprano
 * una de las dos copias se quedaría vieja.
 *
 * NO ES UNA REJA: queda la salida «Otro concepto…» para lo que no esté en la
 * tabla. El reglamento cambia y nadie puede quedarse sin poder registrar una
 * retención esperando a que alguien actualice el maestro.
 */

// Centinela de la salida a texto libre: no puede chocar con un código real.
const OTRO = '__otro__'

export const SUJETOS = [
  { id: 'natural_residente', label: 'Persona natural residente' },
  { id: 'juridica_domiciliada', label: 'Persona jurídica domiciliada' },
]

/* El maestro es del tenant y no cambia mientras dura el modal: se pide UNA vez
 * por carga de la app. Se cachea la PROMESA (no el resultado) para que dos
 * pantallas abiertas a la vez no disparen dos peticiones. */
let maestroPromesa = null
const cargarMaestro = () => {
  if (!maestroPromesa) {
    maestroPromesa = api.conceptosISLR().catch((e) => {
      // Un fallo no se cachea: el siguiente intento vuelve a pedirlo. Sin
      // maestro, el formulario cae a texto libre y se sigue pudiendo trabajar.
      maestroPromesa = null
      throw e
    })
  }
  return maestroPromesa
}

/* useConceptosISLR devuelve los conceptos activos agrupados por código. Nunca
 * lanza: sin maestro devuelve lista vacía y quien lo use cae a texto libre. */
export function useConceptosISLR() {
  const [conceptos, setConceptos] = useState([])

  useEffect(() => {
    let vivo = true
    cargarMaestro()
      .then((r) => { if (vivo) setConceptos(r?.conceptos || []) })
      .catch(() => { if (vivo) setConceptos([]) })
    return () => { vivo = false }
  }, [])

  return useMemo(() => {
    const activos = conceptos.filter((c) => c.activo)
    // Un código aparece UNA vez en el desplegable aunque tenga dos tarifas (una
    // por sujeto): lo que se elige es el concepto, y el sujeto lo acota después.
    const porCodigo = []
    for (const c of activos) {
      if (!porCodigo.some((x) => x.codigo === c.codigo)) {
        porCodigo.push({ codigo: c.codigo, nombre: c.nombre })
      }
    }
    // Qué sujetos tiene cargados cada código: ofrecer uno sin tarifa solo lleva
    // a un «ese concepto no existe» después de elegirlo.
    const sujetosDe = (codigo) => activos.filter((c) => c.codigo === codigo).map((c) => c.sujeto)
    return { conceptos: activos, porCodigo, sujetosDe, hayMaestro: porCodigo.length > 0 }
  }, [conceptos])
}

/* SelectorConceptoISLR — concepto + sujeto, con la tarifa y el monto resueltos
 * por el servidor.
 *
 * props:
 *   base      — base gravable sobre la que se retiene (para la sugerencia)
 *   valor     — { codigo, sujeto, texto } del estado del formulario
 *   onChange(valor, sugerencia|null) — `sugerencia` trae porcentaje, sustraendo
 *               y monto calculados por el servidor; null en texto libre.
 */
export function SelectorConceptoISLR({ base = 0, valor, onChange, disabled = false, error = '' }) {
  const { porCodigo, sujetosDe, hayMaestro } = useConceptosISLR()
  const [sugerencia, setSugerencia] = useState(null)
  const [fallo, setFallo] = useState('')

  const codigo = valor?.codigo || ''
  const sujeto = valor?.sujeto || ''
  // Texto libre: cuando no hay maestro, o cuando se eligió «Otro concepto…».
  const libre = !hayMaestro || codigo === OTRO

  // La sugerencia se pide cuando están las tres cosas. Se vuelve a pedir si
  // cambia la BASE: el concepto puede tener un mínimo por debajo del cual no se
  // retiene, y eso depende del monto.
  useEffect(() => {
    if (libre || !codigo || !sujeto) { setSugerencia(null); setFallo(''); return undefined }
    let vivo = true
    api.sugerenciaRetencionISLR({ codigo, sujeto, base: base || 0 })
      .then((s) => { if (vivo) { setSugerencia(s); setFallo(''); onChange({ ...valor, codigo, sujeto }, s) } })
      .catch((e) => { if (vivo) { setSugerencia(null); setFallo(e?.message || 'No se pudo resolver la tarifa.') } })
    return () => { vivo = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [codigo, sujeto, base, libre])

  const elegirCodigo = (v) => {
    if (v === OTRO) { setSugerencia(null); onChange({ codigo: OTRO, sujeto: '', texto: '' }, null); return }
    // Cambiar de concepto invalida el sujeto si el nuevo no lo tiene cargado:
    // dejarlo puesto llevaría a un «ese concepto no existe» al guardar.
    const sujetos = sujetosDe(v)
    onChange({ codigo: v, sujeto: sujetos.includes(sujeto) ? sujeto : (sujetos[0] || ''), texto: '' }, null)
  }

  if (libre) {
    return (
      <div className="space-y-2">
        <Field label="Concepto de la retención" required error={error}
          hint={hayMaestro ? 'fuera del maestro · la tarifa se teclea' : 'según la providencia'}>
          <Input value={valor?.texto || ''} disabled={disabled}
            placeholder="Ej: Honorarios profesionales, servicios, alquiler…"
            onChange={(e) => onChange({ codigo: OTRO, sujeto: '', texto: e.target.value }, null)} />
        </Field>
        {hayMaestro ? (
          <button type="button" disabled={disabled}
            className="text-[11.5px] text-slate-500 dark:text-slate-400 underline hover:no-underline"
            onClick={() => onChange({ codigo: '', sujeto: '', texto: '' }, null)}>
            Elegir del maestro de conceptos
          </button>
        ) : null}
      </div>
    )
  }

  const sujetosDelCodigo = codigo ? sujetosDe(codigo) : []

  return (
    <div className="space-y-2">
      <div className="grid grid-cols-2 gap-3">
        <Field label="Concepto de la retención" required error={error}>
          <Select value={codigo} disabled={disabled} onChange={(e) => elegirCodigo(e.target.value)}>
            <option value="">Elige el concepto…</option>
            {porCodigo.map((c) => <option key={c.codigo} value={c.codigo}>{c.nombre}</option>)}
            <option value={OTRO}>Otro concepto…</option>
          </Select>
        </Field>
        <Field label="A quién se le retiene" required
          hint={codigo ? 'la tarifa depende de esto' : 'elige el concepto primero'}>
          <Select value={sujeto} disabled={disabled || !codigo}
            onChange={(e) => onChange({ ...valor, sujeto: e.target.value }, null)}>
            <option value="">Elige…</option>
            {SUJETOS.filter((x) => sujetosDelCodigo.includes(x.id))
              .map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}
          </Select>
        </Field>
      </div>

      {fallo ? <div className="text-[11.5px] text-[#B3362C] dark:text-red-400">{fallo}</div> : null}

      {/* La tarifa sale del maestro y se MUESTRA, no se teclea: es el punto de
          todo esto. Si el concepto tiene un mínimo y la base no llega, el
          servidor lo dice con `retiene: false` — que no es lo mismo que «no
          aplica» y por eso se explica en vez de mostrar un cero pelado. */}
      {sugerencia ? (
        <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 px-3 py-2 text-[12px] text-slate-600 dark:text-slate-300">
          Tarifa del maestro: <strong>{fmtNum(sugerencia.porcentaje, 2)}%</strong>
          {sugerencia.sustraendo > 0 ? <> · sustraendo {fmtNum(sugerencia.sustraendo, 2)}</> : null}
          {!sugerencia.retiene ? (
            <span className="block text-amber-700 dark:text-amber-400 mt-0.5">
              Con esta base no se retiene: no llega al mínimo del concepto.
            </span>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

// esTextoLibre indica si el valor del selector es un concepto fuera del maestro.
export const esTextoLibre = (valor) => !valor?.codigo || valor.codigo === OTRO

/* SelectorConceptoProducto — clasifica la FICHA DE UN PRODUCTO.
 *
 * Pide SOLO el concepto, sin sujeto: el producto declara QUÉ se paga (honorarios,
 * flete) y el proveedor declara A QUIÉN (persona natural o jurídica). De ese par
 * sale la tarifa al comprar. Preguntarle acá el sujeto sería pedirle a la ficha
 * algo que no puede saber — el mismo servicio se le compra a los dos.
 *
 * Vacío es el caso normal y por eso encabeza la lista: toda mercancía va sin
 * concepto. Solo los servicios sujetos a retención se clasifican.
 *
 * props:
 *   valor — el código actual ('' = no sujeto)
 *   onChange(codigo)
 */
export function SelectorConceptoProducto({ valor, onChange, disabled = false }) {
  const { porCodigo, hayMaestro } = useConceptosISLR()
  const codigo = (valor || '').trim()

  // Sin maestro no hay nada que ofrecer. Se oculta en vez de mostrar un
  // desplegable vacío, que parecería un error de la ficha: un catálogo de
  // mercancía no necesita esto para nada.
  if (!hayMaestro && !codigo) return null

  // Un código que ya no está en el maestro (se desactivó después de clasificar)
  // se sigue mostrando: si desapareciera del desplegable, la ficha se vería sin
  // concepto y el primer guardado lo borraría sin que nadie lo decidiera.
  const huerfano = codigo && !porCodigo.some((c) => c.codigo === codigo)

  return (
    <div className="space-y-1.5">
      <Field label="Concepto de ISLR"
        hint={codigo ? 'servicio sujeto a retención' : 'solo para servicios'}>
        <Select value={codigo} disabled={disabled} onChange={(e) => onChange(e.target.value)}>
          <option value="">No sujeto a retención (mercancía)</option>
          {porCodigo.map((c) => <option key={c.codigo} value={c.codigo}>{c.nombre}</option>)}
          {huerfano ? <option value={codigo}>{codigo} (fuera del maestro)</option> : null}
        </Select>
      </Field>
      {codigo ? (
        <div className="text-[11.5px] text-slate-500 dark:text-slate-400">
          Al comprarlo, la orden retiene por este concepto. La <strong>tarifa</strong> depende
          además de si el proveedor es persona natural o jurídica, que se declara en su ficha.
        </div>
      ) : null}
    </div>
  )
}
