/* Gráficos SVG puros, sin librería externa. Portado del prototipo Tesotrix. */

export const Sparkline = ({ data, width = 120, height = 30, color = '#6A2CF0', stroke = 1.5, fill = true }) => {
  if (!data || data.length === 0) return null
  const min = Math.min(...data), max = Math.max(...data)
  const range = max - min || 1
  const stepX = width / (data.length - 1)
  const points = data.map((v, i) => [i * stepX, height - ((v - min) / range) * (height - 2) - 1])
  const d = points.map((p, i) => (i === 0 ? 'M' : 'L') + p[0].toFixed(2) + ',' + p[1].toFixed(2)).join(' ')
  const area = d + ` L ${width},${height} L 0,${height} Z`
  return (
    <svg className="spark" width={width} height={height} viewBox={`0 0 ${width} ${height}`}>
      {fill ? <path d={area} fill={color} className="area" /> : null}
      <path d={d} fill="none" stroke={color} strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export const AreaChart = ({ data, series = [{ key: 'v', color: '#6A2CF0', label: '' }], xKey = 'd', width = 720, height = 240, yFmt = (v) => v, showLegend = true, showGrid = true, padTop = 16, padRight = 12, padBottom = 28, padLeft = 44 }) => {
  if (!data || data.length === 0) return null
  const w = width, h = height
  const innerW = w - padLeft - padRight
  const innerH = h - padTop - padBottom
  const ys = data.flatMap((d) => series.map((s) => d[s.key])).filter((v) => v != null)
  const min = Math.min(...ys), max = Math.max(...ys)
  const range = max - min || 1
  const pad = range * 0.1
  // El holgado inferior nunca cruza el cero cuando la serie es no negativa:
  // un eje con "-3" en una serie de dinero es un dato imposible en pantalla.
  const yMin = min >= 0 ? Math.max(0, min - pad) : min - pad
  const yMax = max + pad, yRange = yMax - yMin || 1
  const stepX = innerW / (data.length - 1 || 1)
  const xPx = (i) => padLeft + i * stepX
  const yPx = (v) => padTop + (1 - (v - yMin) / yRange) * innerH
  const ticks = 4
  const tickVals = Array.from({ length: ticks + 1 }, (_, i) => yMin + (i / ticks) * yRange)
  return (
    <div className="w-full">
      {showLegend && series.length > 1 ? (
        <div className="flex items-center gap-3 mb-2">
          {series.map((s) => (
            <div key={s.key} className="inline-flex items-center gap-1.5 text-[12px] text-slate-500">
              <span className="w-2.5 h-2.5 rounded-sm" style={{ background: s.color }} />{s.label || s.key}
            </div>
          ))}
        </div>
      ) : null}
      <svg viewBox={`0 0 ${w} ${h}`} className="w-full h-auto">
        <defs>
          {series.map((s, i) => (
            <linearGradient key={i} id={`grd-${i}-${s.color.replace('#', '')}`} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor={s.color} stopOpacity="0.22" />
              <stop offset="100%" stopColor={s.color} stopOpacity="0" />
            </linearGradient>
          ))}
        </defs>
        {showGrid ? tickVals.map((v, i) => (
          <g key={i}>
            <line x1={padLeft} x2={w - padRight} y1={yPx(v)} y2={yPx(v)} stroke="currentColor" strokeOpacity="0.07" />
            <text x={padLeft - 8} y={yPx(v) + 3} textAnchor="end" fontSize="10" fill="currentColor" opacity="0.55" className="num">{yFmt(v)}</text>
          </g>
        )) : null}
        {data.map((d, i) => {
          if (data.length > 14 && i % 2) return null
          return (
            <text key={i} x={xPx(i)} y={h - padBottom + 14} textAnchor="middle" fontSize="10" fill="currentColor" opacity="0.55" className="num">{d[xKey]}</text>
          )
        })}
        {series.map((s, si) => {
          const pts = data.map((d, i) => [xPx(i), yPx(d[s.key])])
          const line = pts.map((p, i) => (i === 0 ? 'M' : 'L') + p[0].toFixed(1) + ',' + p[1].toFixed(1)).join(' ')
          const area = line + ` L ${xPx(data.length - 1)} ${h - padBottom} L ${padLeft} ${h - padBottom} Z`
          return (
            <g key={si}>
              <path d={area} fill={`url(#grd-${si}-${s.color.replace('#', '')})`} />
              <path d={line} fill="none" stroke={s.color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
              {pts.map((p, i) => <circle key={i} cx={p[0]} cy={p[1]} r="2.4" fill={s.color} />)}
            </g>
          )
        })}
      </svg>
    </div>
  )
}

/* Gráfico de barras — el que usa el prototipo para "Ventas — últimos 30 días".
 * Sin eje Y ni retícula: la cifra exacta vive en el total del período y en el
 * tooltip de cada barra. Solo tres marcas en el eje X (inicio, medio, hoy).
 * Barras en azul de marca; el verde no se usa para series de datos. */
export const BarChart = ({ data, xKey = 'd', yKey = 'v', height = 240, color = '#6A2CF0', fmt = (v) => v }) => {
  if (!data || data.length === 0) return null
  const max = Math.max(...data.map((d) => Number(d[yKey]) || 0), 1)
  const n = data.length
  return (
    <div className="w-full">
      <div className="flex items-end gap-[3px]" style={{ height }}>
        {data.map((d, i) => {
          const v = Number(d[yKey]) || 0
          // Altura mínima visible de 2px para que un día con venta no parezca cero.
          const h = v > 0 ? Math.max(2, (v / max) * 100) : 0
          return (
            <div key={i} className="flex-1 min-w-0 h-full flex items-end" title={`${d[xKey]} · ${fmt(v)}`}>
              <div className="w-full rounded-t-[3px] transition-[height] duration-300"
                style={{ height: `${h}%`, background: color, minHeight: v > 0 ? 2 : 0 }} />
            </div>
          )
        })}
      </div>
      <div className="flex justify-between mt-2 text-[11px] text-slate-400">
        <span>{data[0]?.[xKey]}</span>
        <span>{data[Math.floor(n / 2)]?.[xKey]}</span>
        <span>hoy</span>
      </div>
    </div>
  )
}

export const DonutChart = ({ data, size = 180, thickness = 22, center }) => {
  const total = data.reduce((a, b) => a + b.value, 0) || 1
  const cx = size / 2, cy = size / 2
  const r = (size - thickness) / 2 - 2
  let acc = 0
  return (
    <div className="relative inline-block">
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        {data.map((d, i) => {
          const start = (acc / total) * Math.PI * 2 - Math.PI / 2
          acc += d.value
          const end = (acc / total) * Math.PI * 2 - Math.PI / 2
          const large = end - start > Math.PI ? 1 : 0
          const x1 = cx + r * Math.cos(start), y1 = cy + r * Math.sin(start)
          const x2 = cx + r * Math.cos(end), y2 = cy + r * Math.sin(end)
          return <path key={i} d={`M ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2}`} fill="none" stroke={d.color} strokeWidth={thickness} strokeLinecap="butt" />
        })}
      </svg>
      {center ? <div className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none">{center}</div> : null}
    </div>
  )
}
