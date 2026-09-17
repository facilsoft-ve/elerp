import { useState, useEffect, useMemo } from 'react'
import { Field, Select } from './primitives.jsx'
import { api } from '../lib/api.js'

/* Selector de ALÍCUOTA de un producto, contra el maestro de impuestos.
 *
 * POR QUÉ: hasta acá un producto solo podía ser «gravado» o «exento», y el IVA
 * era una constante del 16 %. En Venezuela conviven tres tasas — la general
 * (16 %), una reducida (8 %) para ciertos alimentos, y el recargo suntuario del
 * lujo, que NO reemplaza a la general sino que se le SUMA (16 % + 15 % = 31 %).
 * Un booleano no puede expresar eso.
 *
 * COMPATIBILIDAD, que es lo delicado: `producto.alicuotaCodigo` VACÍO es válido
 * y significa «como siempre» — exento si `exentoIva`, general si no. Los
 * catálogos ya cargados no se migran, así que este selector tiene que mostrar la
 * clasificación EFECTIVA aunque el código esté en blanco, o quien abra una ficha
 * vieja creería que no tiene impuesto configurado.
 */

/* El maestro es dato de configuración: igual para toda la sesión y para cada
 * ficha de producto que se abra. Se pide UNA vez y se comparte; se cachea la
 * promesa (no el resultado) para que dos fichas abiertas a la vez no disparen
 * dos peticiones. */
let maestroPromesa = null
const cargarMaestro = () => {
  if (!maestroPromesa) {
    maestroPromesa = api.alicuotas().catch((e) => {
      // Un fallo no se cachea: el siguiente intento vuelve a pedirlo.
      maestroPromesa = null
      throw e
    })
  }
  return maestroPromesa
}

// invalidarMaestro lo vuelve a pedir tras editarlo en Configuración. Sin esto,
// cambiar una tasa no se vería en las fichas hasta recargar la aplicación.
export function invalidarMaestro() { maestroPromesa = null }

/* useAlicuotas devuelve las alícuotas VIGENTES hoy (las históricas no se pueden
 * elegir: clasificar un producto con una tasa que ya no rige no significa nada).
 * Nunca lanza: si el maestro no carga, devuelve lista vacía y quien lo use cae a
 * su comportamiento anterior. */
export function useAlicuotas() {
  const [todas, setTodas] = useState([])

  useEffect(() => {
    let vivo = true
    cargarMaestro()
      .then((r) => { if (vivo) setTodas(r?.alicuotas || []) })
      .catch(() => { if (vivo) setTodas([]) })
    return () => { vivo = false }
  }, [])

  return useMemo(() => todas.filter((a) => a.vigente), [todas])
}

/* codigoEfectivo resuelve qué clasificación tiene REALMENTE un producto, con la
 * misma regla que el servidor (application.alicuotaDeProducto): su código si lo
 * tiene; si no, el booleano heredado. */
export function codigoEfectivo(producto) {
  const cod = (producto?.alicuotaCodigo || '').trim()
  if (cod) return cod
  return producto?.exentoIva ? 'exento' : 'general'
}

/* etiquetaAlicuota escribe una alícuota como se lee: "General (16%)" o
 * "Suntuaria · 16% + 15% adicional". El recargo se muestra SUMANDO y no como un
 * 31% de una pieza, porque así es como se declara y como hay que entenderlo. */
export function etiquetaAlicuota(a) {
  if (!a) return ''
  if (a.tipo === 'exento') return `${a.nombre} · sin IVA`
  const pct = (f) => `${Math.round((f || 0) * 10000) / 100}%`
  return a.adicional > 0
    ? `${a.nombre} · ${pct(a.porcentaje)} + ${pct(a.adicional)} adicional`
    : `${a.nombre} · ${pct(a.porcentaje)}`
}

/* SelectorAlicuota reemplaza al viejo interruptor «exento de IVA».
 *
 * props:
 *   valor    — el código actual (puede venir vacío en fichas viejas)
 *   producto — la ficha, para resolver la clasificación efectiva
 *   onChange(codigo)
 */
export function SelectorAlicuota({ valor, producto, onChange, disabled = false }) {
  const alicuotas = useAlicuotas()
  // Lo que hay que MOSTRAR seleccionado: el código elegido, o el efectivo si la
  // ficha es anterior al maestro.
  const actual = (valor || '').trim() || codigoEfectivo(producto)
  const ficha = alicuotas.find((a) => a.codigo === actual)

  // Sin maestro cargado no se puede ofrecer nada sensato: se avisa en vez de
  // mostrar un desplegable vacío que parecería un error de la ficha.
  if (alicuotas.length === 0) {
    return (
      <Field label="Alícuota de IVA" hint="cargando el maestro de impuestos…">
        <Select value="" disabled><option value="">—</option></Select>
      </Field>
    )
  }

  return (
    <div className="space-y-1.5">
      <Field label="Alícuota de IVA" required
        hint={!(valor || '').trim() ? 'heredada de la ficha' : ''}>
        <Select value={actual} disabled={disabled} onChange={(e) => onChange(e.target.value)}>
          {alicuotas.map((a) => (
            <option key={a.codigo} value={a.codigo}>{etiquetaAlicuota(a)}</option>
          ))}
        </Select>
      </Field>
      {ficha?.adicional > 0 ? (
        <div className="text-[11.5px] text-slate-500 dark:text-slate-400">
          El recargo suntuario se declara <strong>aparte</strong> de la general en el libro de ventas:
          la factura muestra las dos porciones, no un 31 % de una pieza.
        </div>
      ) : null}
    </div>
  )
}
