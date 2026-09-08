/* Tablero del módulo Restaurante: la PRIMERA pantalla al entrar.
 *
 * Responde de un vistazo las tres preguntas de quien administra un salón: cómo está el
 * salón ahora, qué está pasando en cocina, y cómo va el día. Y desde acá se salta a cada
 * sección, sin obligar a buscarla en el menú.
 *
 * Vive en su propio archivo (y no dentro de Restaurante.jsx, que ya pasa las 1.700
 * líneas) porque ese archivo es donde más se pisan los cambios concurrentes.
 *
 * El cálculo NO está acá: vive en lib/restaurante.js y tiene pruebas. Un KPI equivocado
 * se ve igual de convincente que el correcto, así que no puede quedar sin red. */
import { useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Card, Empty } from '../components/primitives.jsx'
import { useData } from '../context/DataContext.jsx'
import { fmtCurrency, fmtNum } from '../lib/format.js'
import { metricasRestaurante } from '../lib/restaurante.js'

/* Accesos del módulo. Los mismos destinos que el menú, pero puestos donde la mano ya
   está: el tablero es la pantalla de entrada. */
const ACCESOS = [
  { id: 'comandera', label: 'Comandera', sub: 'Tomar el pedido de una mesa', glyph: Icon.ClipboardList },
  { id: 'cocina', label: 'Cocina', sub: 'Comandas entrantes en vivo', glyph: Icon.Activity },
  { id: 'mesas', label: 'Mapa de mesas', sub: 'Diseñar el salón', glyph: Icon.Utensils },
  { id: 'mesoneros', label: 'Mesoneros y asignación', sub: 'Quién atiende cada mesa', glyph: Icon.Users },
  { id: 'platos', label: 'Platos y recetas', sub: 'Escandallo e insumos', glyph: Icon.Boxes },
  { id: 'impresora', label: 'Impresoras comanderas', sub: 'Por área: cocina, barra, postres', glyph: Icon.Printer },
]

export function RestauranteInicio({ irA }) {
  const { db } = useData()
  const m = useMemo(() => metricasRestaurante({
    mesas: db.MESAS || [],
    cuentas: db.CUENTAS_ABIERTAS || [],
    documentos: db.DOCUMENTOS || [],
    productos: db.PRODUCTOS || [],
  }), [db.MESAS, db.CUENTAS_ABIERTAS, db.DOCUMENTOS, db.PRODUCTOS])

  const sinMesas = m.mesasTotal === 0

  return (
    <div className="space-y-5">
      {sinMesas ? (
        <Empty title="El salón todavía no está armado"
          body="Empezá por el mapa de mesas: dibujá el salón sobre la grilla y ubicá las mesas. Después cargá los platos con su receta."
          cta={<Button onClick={() => irA('mesas')}>Diseñar el mapa de mesas</Button>} />
      ) : (
        <>
          {/* 1 · El salón ahora */}
          <section>
            <h2 className="font-display font-semibold text-[15px] mb-2.5">El salón ahora</h2>
            <div className="grid gap-3 grid-cols-2 lg:grid-cols-4">
              <Kpi label="Mesas libres" valor={`${m.libres} de ${m.mesasTotal}`}
                tono={m.libres === 0 ? 'alerta' : 'ok'}
                sub={m.libres === 0 ? 'Salón completo' : null} />
              <Kpi label="Ocupadas" valor={fmtNum(m.ocupadas, 0)}
                sub={m.comensales ? `${m.comensales} comensal(es) sentados` : null} />
              <Kpi label="Por cobrar" valor={fmtNum(m.porCobrar, 0)} tono={m.porCobrar > 0 ? 'aviso' : 'tinta'}
                sub={m.porCobrar > 0 ? 'Ya pidieron la cuenta' : null} />
              <Kpi label="Consumo en curso" valor={fmtCurrency(m.consumoEnCurso, 'VES')}
                sub="En mesas sin cobrar" />
            </div>
          </section>

          {/* 2 · Cocina */}
          <section>
            <h2 className="font-display font-semibold text-[15px] mb-2.5">Cocina</h2>
            <div className="grid gap-3 grid-cols-2 lg:grid-cols-3">
              <Kpi label="En preparación" valor={fmtNum(m.enCocina, 0)} sub="Renglones enviados y sin servir" />
              <Kpi label="Listos para llevar" valor={fmtNum(m.listosParaLlevar, 0)}
                tono={m.listosParaLlevar > 0 ? 'ok' : 'tinta'}
                sub={m.listosParaLlevar > 0 ? 'Esperando al mesonero' : null} />
              <Kpi label="Espera más larga" valor={m.enCocina ? `${m.esperaMaxima} min` : '—'}
                tono={m.esperaMaxima >= 20 ? 'alerta' : m.esperaMaxima >= 10 ? 'aviso' : 'ok'}
                sub={m.esperaMaxima >= 20 ? 'Hay una comanda demorada' : null} />
            </div>
          </section>

          {/* 3 · El día */}
          <section>
            <h2 className="font-display font-semibold text-[15px] mb-2.5">Hoy</h2>
            <div className="grid gap-3 grid-cols-2 lg:grid-cols-4">
              <Kpi label="Facturado hoy" valor={fmtCurrency(m.ventasHoy, 'VES')} />
              <Kpi label="Mesas cobradas" valor={fmtNum(m.mesasCobradasHoy, 0)} />
              <Kpi label="Ticket promedio" valor={m.mesasCobradasHoy ? fmtCurrency(m.ticketPromedio, 'VES') : '—'} />
              <Kpi label="Rotación" valor={m.mesasCobradasHoy ? `${m.rotacion.toFixed(1)}×` : '—'}
                sub="Veces que se usó cada mesa" />
            </div>
          </section>

          {/* 4 · Platos más vendidos */}
          <section>
            <h2 className="font-display font-semibold text-[15px] mb-2.5">Platos más vendidos hoy</h2>
            {m.platosTop.length === 0 ? (
              <Card className="!p-5 text-[13px] text-slate-500 dark:text-slate-400">
                Todavía no se facturó ningún plato hoy. En cuanto la caja cobre la primera mesa, acá aparece el ranking.
              </Card>
            ) : (
              <Card className="overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-[11.5px] uppercase tracking-wide text-slate-500 border-b border-slate-200 dark:border-slate-800">
                      <th className="px-4 py-2.5 font-medium">Plato</th>
                      <th className="px-4 py-2.5 font-medium text-right">Vendidos</th>
                      <th className="px-4 py-2.5 font-medium text-right">Facturado</th>
                    </tr>
                  </thead>
                  <tbody>
                    {m.platosTop.map((p) => (
                      <tr key={p.sku} className="border-b border-slate-100 dark:border-slate-800/70 last:border-0">
                        <td className="px-4 py-2.5">
                          <div className="font-medium text-[13px]">{p.nombre}</div>
                          <div className="text-[11.5px] text-slate-500 num">{p.sku}</div>
                        </td>
                        <td className="px-4 py-2.5 text-right tnum font-medium">{fmtNum(p.cantidad, 0)}</td>
                        <td className="px-4 py-2.5 text-right tnum">{fmtCurrency(p.total, 'VES')}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </Card>
            )}
          </section>
        </>
      )}

      {/* 5 · Accesos a las secciones del módulo */}
      <section>
        <h2 className="font-display font-semibold text-[15px] mb-2.5">Secciones</h2>
        <div className="grid gap-3 grid-cols-2 lg:grid-cols-3">
          {ACCESOS.map((a) => {
            const G = a.glyph
            return (
              <button key={a.id} onClick={() => irA(a.id)}
                className="text-left rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900
                  p-4 hover:border-elerp-300 hover:shadow-card ring-focus transition-all active:scale-[0.99]">
                <span className="h-9 w-9 rounded-icon bg-elerp-50 dark:bg-elerp-900/40 text-elerp-500 dark:text-elerp-200
                  inline-flex items-center justify-center mb-2.5"><G size={18} /></span>
                <div className="font-semibold text-[13.5px]">{a.label}</div>
                <div className="text-[12px] text-slate-500 dark:text-slate-400 mt-0.5">{a.sub}</div>
              </button>
            )
          })}
        </div>
      </section>
    </div>
  )
}

/* Kpi: una cifra con su rótulo. El tono codifica el estado en la FORMA además del
   número, para que lo que necesita atención se lea de un vistazo y no haya que
   comparar cifras. */
function Kpi({ label, valor, sub, tono = 'tinta' }) {
  const color = {
    tinta: 'text-slate-900 dark:text-slate-100',
    ok: 'text-teal-700 dark:text-teal-300',
    aviso: 'text-amber-700 dark:text-amber-300',
    alerta: 'text-[#B3362C] dark:text-red-400',
  }[tono]
  return (
    <Card className="!p-4">
      <div className="text-[12.5px] text-slate-500 dark:text-slate-400 truncate">{label}</div>
      <div className={`font-display font-bold text-[22px] tnum leading-tight mt-1.5 ${color}`}>{valor}</div>
      {sub ? <div className="text-[11.5px] text-slate-500 dark:text-slate-400 mt-0.5 truncate">{sub}</div> : null}
    </Card>
  )
}
