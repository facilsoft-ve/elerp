import { Field, Input, Select, Segmented, Toggle } from './primitives.jsx'
import { fmtCurrency } from '../lib/format.js'
import { ajusteVacio, calcularAjuste, cuerpoAjuste } from '../lib/nota.js'

// Se reexportan para que la pantalla importe todo el ajuste de un solo sitio.
export { ajusteVacio, calcularAjuste, cuerpoAjuste }

/* AJUSTE DE MONTO DE UNA NOTA (crédito o débito).
 *
 * Nota del usuario: «las notas de crédito y débito deben poder crearse
 * especificando un % sobre el monto total o el monto del producto seleccionado».
 *
 * POR QUÉ IMPORTA: un descuento o un recargo se PACTA en porcentaje —«10 % por
 * pronto pago», «5 % de mora»—, no en bolívares. Obligar a escribir el monto
 * hace que alguien saque la regla de tres en una calculadora y teclee el
 * resultado: ahí se cuela el céntimo que después no cuadra contra la factura, y
 * peor, nadie puede reconstruir después de qué porcentaje salió ese número.
 *
 * Quien calcula de verdad es el SERVIDOR: acá solo se previsualiza. La cuenta se
 * repite para que el número se vea antes de emitir un documento que ya no se
 * puede editar, pero el que queda asentado es el del backend.
 *
 * EL PORCENTAJE VA SOBRE LA BASE, no sobre el total con IVA. Da lo mismo en
 * plata (el IVA es proporcional: 10 % de la base baja el total un 10 %), pero
 * aplicarlo sobre el total y volver a sumar IVA encima descontaría dos veces el
 * impuesto.
 */

/* SelectorAjuste: sobre QUÉ se aplica (toda la factura o un producto) y CÓMO
 * (monto o porcentaje). */
export function SelectorAjuste({ doc, valor, onChange, etiqueta = 'Descuento', mostrarExento = true }) {
  const lineas = (doc?.lineas || []).filter((l) => l.sku)
  const porcentual = valor.modo === 'porcentaje'
  const set = (parche) => onChange({ ...valor, ...parche })

  return (
    <div className="space-y-3.5">
      <Field label="Se aplica sobre">
        <Segmented
          value={valor.alcance}
          onChange={(v) => set({ alcance: v, sku: v === 'producto' ? (valor.sku || lineas[0]?.sku || '') : '' })}
          options={[
            { value: 'total', label: 'Toda la factura' },
            // Sin renglones con SKU (una nota sobre un cargo suelto) no hay
            // producto que elegir: se deshabilita en vez de ofrecer una lista vacía.
            { value: 'producto', label: 'Un producto', disabled: lineas.length === 0 },
          ]} />
      </Field>

      {valor.alcance === 'producto' ? (
        <Field label="Producto">
          <Select value={valor.sku} onChange={(e) => set({ sku: e.target.value })}>
            <option value="">Elige el producto…</option>
            {lineas.map((l) => (
              <option key={l.sku} value={l.sku}>
                {l.nombre || l.sku} — {fmtCurrency(Math.abs(Number(l.total) || 0), 'VES')}{l.exento ? ' (exento)' : ''}
              </option>
            ))}
          </Select>
        </Field>
      ) : null}

      <Field label={`${etiqueta} expresado en`}>
        <Segmented value={valor.modo} onChange={(v) => set({ modo: v })}
          options={[{ value: 'monto', label: 'Monto (Bs)' }, { value: 'porcentaje', label: 'Porcentaje (%)' }]} />
      </Field>

      {porcentual ? (
        <Field label={`${etiqueta} (%)`} required
          hint={valor.alcance === 'producto' ? 'sobre el monto de ese producto' : 'sobre el monto de la factura'}>
          <Input type="number" min={0} max={100} step="any" className="num" value={valor.porcentaje}
            placeholder="0,00" onChange={(e) => set({ porcentaje: e.target.value })} />
        </Field>
      ) : (
        <Field label={`${etiqueta} (Bs)`} required>
          <Input type="number" min={0} step="any" className="num" value={valor.monto}
            placeholder="0,00" onChange={(e) => set({ monto: e.target.value })} />
        </Field>
      )}

      {/* La condición de IVA solo se pregunta cuando NO hay de dónde deducirla.
          Sobre un producto la hereda el renglón; en un porcentaje sobre toda la
          factura se reparte proporcional. Preguntarla ahí invitaría a
          contradecir la factura. */}
      {mostrarExento && valor.alcance === 'total' && !porcentual ? (
        <Toggle checked={valor.exento} onChange={(v) => set({ exento: v })}
          label="Exento de IVA" sub="Actívalo si el monto no causa IVA (p. ej. intereses de mora)." />
      ) : null}
    </div>
  )
}

/* ResumenAjuste muestra base, IVA y total antes de emitir. El documento es
 * inmutable: lo que se ve acá es la última oportunidad de detectar un error. */
export function ResumenAjuste({ calc, ccy, signo = '-', tono = 'text-amber-700 dark:text-amber-300', titulo = 'Total a acreditar' }) {
  return (
    <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 p-3 space-y-1.5 text-[13px]">
      <div className="flex items-center justify-between">
        <span className="text-slate-500">Base gravada</span>
        <span className="num private-mask">{fmtCurrency(calc.gravado, ccy)}</span>
      </div>
      {calc.exento > 0 ? (
        <div className="flex items-center justify-between">
          <span className="text-slate-500">Base exenta</span>
          <span className="num private-mask">{fmtCurrency(calc.exento, ccy)}</span>
        </div>
      ) : null}
      <div className="flex items-center justify-between">
        <span className="text-slate-500">IVA</span>
        <span className="num private-mask">{fmtCurrency(calc.iva, ccy)}</span>
      </div>
      <div className="border-t border-slate-200 dark:border-slate-700 pt-1.5 flex items-center justify-between">
        <span className="font-semibold">{titulo}</span>
        <span className={`num font-semibold private-mask ${tono}`}>{signo}{fmtCurrency(calc.total, ccy)}</span>
      </div>
    </div>
  )
}

