import { useState, useEffect } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Segmented, Empty, TableSkeleton } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { useUI } from '../context/UIContext.jsx'
import { useAuth } from '../context/AuthContext.jsx'

import { api } from '../lib/api.js'

/* VALORACIÓN DEL INVENTARIO.
 *
 * Responde dos preguntas que el Libro de Inventario no responde:
 *
 *  · DÓNDE está el valor — por almacén y ubicación. Es lo que se lleva al conteo.
 *  · SI LA CONTABILIDAD Y EL ANAQUEL DICEN LO MISMO. Esta es la importante: el
 *    balance cuadra igual esté el saldo en la cuenta que sea, así que un inventario
 *    contabilizado en la cuenta equivocada no lo delata ningún total.
 */
export function Valoracion() {
  const { ui } = useUI()
  const { activeSede, activeSedeId } = useAuth()
  const [ambito, setAmbito] = useState('empresa')
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)

  const cargar = () => {
    setData(null); setError(null)
    api.valoracion(ambito === 'sede' ? activeSedeId : '')
      .then(setData)
      .catch((e) => setError(e))
  }
  useEffect(cargar, [ambito, activeSedeId]) // eslint-disable-line react-hooks/exhaustive-deps

  if (error) {
    return <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar la valoración"
      body={String(error.message || error)} cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
  }
  if (data === null) {
    return <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4"><TableSkeleton rows={8} cols={5} /></div>
  }

  const descuadradas = (data.porCuenta || []).filter((c) => !c.cuadra)

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-3">
        <Segmented size="sm" value={ambito} onChange={setAmbito}
          options={[
            { value: 'empresa', label: 'Toda la empresa' },
            { value: 'sede', label: activeSede?.nombre || 'Esta sede' },
          ]} />
        <div className="ml-auto text-[12.5px] text-slate-500">
          Valor total: <span className="num font-medium text-slate-700 dark:text-slate-200 private-mask">{fmtCurrency(data.valorTotal, ui.ccy)}</span>
        </div>
      </div>

      {/* Lo primero que se lee: si el anaquel y la contabilidad cuentan lo mismo. */}
      {ambito === 'empresa' ? (
        <div className={`mb-3 rounded-xl px-3 py-2.5 border ${descuadradas.length
          ? 'bg-red-50 dark:bg-red-900/25 border-red-200 dark:border-red-900/40'
          : 'bg-emerald-50 dark:bg-emerald-900/25 border-emerald-200 dark:border-emerald-900/40'}`}>
          <div className="flex items-start gap-2.5">
            <Icon.CircleAlert size={15} className={`mt-0.5 shrink-0 ${descuadradas.length ? 'text-red-700 dark:text-red-400' : 'text-emerald-700 dark:text-emerald-400'}`} />
            <div className={`text-[12.5px] min-w-0 ${descuadradas.length ? 'text-red-900 dark:text-red-200' : 'text-emerald-900 dark:text-emerald-200'}`}>
              {descuadradas.length === 0 ? (
                <><strong>El inventario y la contabilidad dicen lo mismo.</strong> Cada cuenta de activo vale
                  exactamente la mercancía que la respalda.</>
              ) : (
                <>
                  <strong>{descuadradas.length} cuenta(s) no cuadran con el inventario.</strong> El balance
                  puede seguir cuadrando igual: lo que no coincide es <em>en qué cuenta</em> está el valor.
                  <div className="mt-1 space-y-0.5">
                    {descuadradas.map((c) => (
                      <div key={c.clave} className="flex items-baseline gap-2 flex-wrap text-[11.5px]">
                        <span className="num">{c.clave}</span>
                        <span>{c.nombre}</span>
                        <span>inventario {fmtCurrency(c.valor, ui.ccy)}</span>
                        <span>· contabilidad {fmtCurrency(c.contable, ui.ccy)}</span>
                        <span className="font-semibold">· diferencia {fmtCurrency(c.diferencia, ui.ccy)}</span>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>
          </div>
        </div>
      ) : (
        <div className="mb-3 text-[12px] text-slate-500">
          Filtrando por sede no se compara con la contabilidad: el saldo de una cuenta es de la empresa
          entera, y la diferencia sería el inventario de las demás sedes.
        </div>
      )}

      <div className="grid gap-3 md:grid-cols-2 mb-3">
        <Resumen titulo="Por almacén" filas={data.porAlmacen} ccy={ui.ccy} />
        <Resumen titulo="Por cuenta contable" filas={data.porCuenta} ccy={ui.ccy} conCuenta />
      </div>

      {data.filas.length === 0 ? (
        <Empty icon={<Icon.Boxes size={22} />} title="Sin existencias que valorar" body="No hay mercancía en este ámbito." />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-[11px] uppercase tracking-wide text-slate-400 bg-slate-50/60 dark:bg-slate-900/60 border-b border-slate-200 dark:border-slate-800">
                <tr>
                  <th className="py-2.5 pl-4 pr-3 text-left font-medium">Almacén</th>
                  <th className="py-2.5 pr-3 text-left font-medium">Ubicación</th>
                  <th className="py-2.5 pr-3 text-left font-medium">Producto</th>
                  <th className="py-2.5 pr-3 text-left font-medium">Cuenta</th>
                  <th className="py-2.5 pr-3 text-right font-medium">Cantidad</th>
                  <th className="py-2.5 pr-3 text-right font-medium">Costo</th>
                  <th className="py-2.5 pr-4 text-right font-medium">Valor</th>
                </tr>
              </thead>
              <tbody>
                {data.filas.map((f, i) => (
                  <tr key={f.almacenId + '/' + f.ubicacionId + '/' + f.sku + i} className="border-b border-slate-100 dark:border-slate-800/70">
                    <td className="py-2 pl-4 pr-3 text-[12.5px] text-slate-500">{f.almacenNombre}</td>
                    <td className="py-2 pr-3 num text-[12.5px]">{f.ubicacion}</td>
                    <td className="py-2 pr-3 text-[13px]">{f.nombre}<span className="text-slate-400 text-[11.5px]"> · {f.sku}</span></td>
                    <td className="py-2 pr-3 num text-[12px] text-slate-500">{f.cuenta}</td>
                    <td className="py-2 pr-3 text-right num">{fmtNum(f.cantidad)}</td>
                    <td className="py-2 pr-3 text-right num text-slate-500 private-mask">{fmtCurrency(f.costoPromedio, ui.ccy)}</td>
                    <td className="py-2 pr-4 text-right num font-medium private-mask">{fmtCurrency(f.valor, ui.ccy)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}

function Resumen({ titulo, filas, ccy, conCuenta }) {
  if (!filas || filas.length === 0) return null
  return (
    <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
      <div className="px-4 py-2 text-[11.5px] uppercase tracking-wide text-slate-400 bg-slate-50/60 dark:bg-slate-800/40">{titulo}</div>
      <table className="w-full text-[13px]">
        <tbody>
          {filas.map((f) => (
            <tr key={f.clave} className="border-t border-slate-100 dark:border-slate-800">
              <td className="px-4 py-2">
                {conCuenta ? <span className="num text-[12px] text-slate-500 mr-1.5">{f.clave}</span> : null}
                {f.nombre}
                <span className="text-slate-400 text-[11.5px]"> · {f.lineas} línea(s)</span>
              </td>
              <td className="px-4 py-2 text-right num font-medium private-mask">{fmtCurrency(f.valor, ccy)}</td>
              {conCuenta ? (
                <td className="px-4 py-2 text-right text-[11.5px]">
                  {f.contable || f.diferencia ? (
                    <span className={f.cuadra ? 'text-slate-400' : 'text-red-600 dark:text-red-400 font-medium'}>
                      {f.cuadra ? 'cuadra' : `dif. ${fmtCurrency(f.diferencia, ccy)}`}
                    </span>
                  ) : null}
                </td>
              ) : null}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
