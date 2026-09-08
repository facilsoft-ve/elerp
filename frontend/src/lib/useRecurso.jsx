import { useState, useEffect, useCallback } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Empty, Button, TableSkeleton } from '../components/primitives.jsx'

/* useRecurso — el ciclo de carga de un recurso remoto con TRES estados explícitos,
 * para no confundir un fallo de red con "no hay datos" (clave en el entorno de
 * conectividad hostil que asume el producto: un error debe verse como error y
 * poder reintentarse, no disfrazarse de vacío).
 *
 * Extraído del `useReporte` de screens/Reportes.jsx, que ya centralizaba este
 * patrón y funcionaba bien; aquí queda reutilizable para el resto de pantallas.
 *
 *   data === undefined → cargando · data === null → error · si no, datos.
 *
 * `fetcher` debe resolver a un valor NO nulo (usa `|| []` / un objeto por
 * defecto): un `null`/`undefined` resuelto se interpreta como error. Para cargar
 * varios recursos, combínalos en un solo fetcher (Promise.all → objeto).
 */
export function useRecurso(fetcher, deps = []) {
  const [data, setData] = useState(undefined)
  const [error, setError] = useState(null)
  const cargar = useCallback(() => {
    setError(null)
    setData(undefined)
    fetcher()
      .then((r) => { if (r == null) throw new Error('Respuesta vacía del servidor.'); setData(r) })
      .catch((e) => { setData(null); setError(e) })
  }, deps) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { cargar() }, [cargar])
  return { data, error, loading: data === undefined, reload: cargar }
}

/* EstadoRecurso — pinta el estado de carga (skeleton) y el de error (con botón
 * Reintentar) y delega los datos al render hijo. Reusa los primitivos existentes
 * (TableSkeleton / Empty / Button); no reinventa nada visual.
 *
 * `skeleton` permite pasar un esqueleto a medida; si no, cae a una tarjeta con
 * TableSkeleton (rows × cols). */
export function EstadoRecurso({
  loading, error, onRetry, cols = 4, rows = 6, skeleton,
  title = 'No se pudo cargar la información', children,
}) {
  if (loading) {
    return skeleton !== undefined ? skeleton : (
      <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card p-4">
        <TableSkeleton rows={rows} cols={cols} />
      </div>
    )
  }
  if (error) {
    return (
      <Empty icon={<Icon.CircleAlert size={22} />} title={title}
        body={String(error?.message || error)}
        cta={<Button onClick={onRetry} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
    )
  }
  return children
}
