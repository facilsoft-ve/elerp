import { useState, useEffect, useCallback, useMemo } from 'react'
import { Icon } from '../components/Icon.jsx'
import { Button, Badge, Segmented, Select, Empty, TableSkeleton } from '../components/primitives.jsx'
import { fmtCurrency, fmtNum, fmtDate } from '../lib/format.js'
import { useUI } from '../context/UIContext.jsx'
import { useData } from '../context/DataContext.jsx'
import { api } from '../lib/api.js'
import { descargarXLSX } from '../lib/xlsx.js'

// Libros fiscales (Libro de Ventas / Libro de Compras): reportes DERIVADOS del
// ledger, de SOLO LECTURA. Por contribuyente (empresa/RIF, consolidando TODAS
// las sedes) y por período mensual. La interfaz nunca inventa un total: pinta lo
// que el servidor plegó del ledger de documentos y de las órdenes de compra.
//
// Solo entran los roles con acceso de consulta fiscal: la Contadora DEBE poder
// leerlos (acceso del regulador, Providencia 000121). El backend es la autoridad
// real; aquí solo evitamos una llamada que devolvería 403.
const ROLES_LIBROS = ['dueno', 'desarrollador', 'contadora']

const MESES = [
  'Enero', 'Febrero', 'Marzo', 'Abril', 'Mayo', 'Junio',
  'Julio', 'Agosto', 'Septiembre', 'Octubre', 'Noviembre', 'Diciembre',
]

// Etiqueta legible del tipo de documento del Libro de Ventas.
const TIPO_LABEL = { factura: 'Factura', nota_credito: 'Nota de crédito', anulacion: 'Anulación' }
const esNegativo = (tipo) => tipo === 'nota_credito' || tipo === 'anulacion'

export function LibrosFiscales({ libroInicial = 'ventas' }) {
  const { ui } = useUI()
  const { db } = useData()
  const ccy = ui.ccy
  const puedeVer = ROLES_LIBROS.includes(ui.rol)

  const hoy = new Date()
  const [libro, setLibro] = useState(libroInicial) // 'ventas' | 'compras'
  const [anio, setAnio] = useState(hoy.getUTCFullYear())
  const [mes, setMes] = useState(hoy.getUTCMonth() + 1) // 1..12
  const [data, setData] = useState(undefined) // undefined = cargando
  const [error, setError] = useState(null)

  const cargar = useCallback(async () => {
    if (!puedeVer) return
    setError(null)
    setData(undefined)
    try {
      const res = libro === 'ventas' ? await api.libroVentas(anio, mes) : await api.libroCompras(anio, mes)
      setData(res)
    } catch (e) {
      setData(null)
      setError(e)
    }
  }, [libro, anio, mes, puedeVer])

  useEffect(() => { cargar() }, [cargar])

  // Años disponibles en el selector: del actual hacia atrás unos cuantos.
  const anios = useMemo(() => {
    const y = hoy.getUTCFullYear()
    return [y, y - 1, y - 2, y - 3, y - 4]
  }, [hoy])

  if (!puedeVer) {
    return <Empty icon={<Icon.Lock size={22} />} title="Sin acceso a los libros fiscales"
      body="Los libros de ventas y compras son de consulta de la Dueña, el Desarrollador y la Contadora." />
  }

  const filas = data?.filas || []
  const totales = data?.totales || {}

  // La contadora trabaja los libros en hoja de cálculo: se exporta XLSX y no
  // TXT ni CSV. Un CSV renombrado a .xlsx Excel lo rechaza, y el CSV crudo se
  // reinterpreta con la configuración regional del equipo (un «10-03» se vuelve
  // fecha). Ver lib/xlsx.js.
  const exportar = () => exportarLibroXLSX(libro, anio, mes, filas, totales, TIPO_LABEL, db.EMPRESA)

  return (
    <div>
      {/* Controles: tipo de libro + período + exportar */}
      <div className="flex items-center gap-3 flex-wrap mb-3">
        <Segmented
          options={[{ value: 'ventas', label: 'Ventas' }, { value: 'compras', label: 'Compras' }]}
          value={libro} onChange={setLibro} />
        <div className="flex items-center gap-2">
          <Select value={mes} onChange={(e) => setMes(Number(e.target.value))} className="w-36">
            {MESES.map((m, i) => <option key={i} value={i + 1}>{m}</option>)}
          </Select>
          <Select value={anio} onChange={(e) => setAnio(Number(e.target.value))} className="w-24">
            {anios.map((y) => <option key={y} value={y}>{y}</option>)}
          </Select>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Button variant="secondary" icon={<Icon.Download size={15} />} disabled={!filas.length} onClick={exportar}
            title={filas.length ? 'Exportar el libro a Excel (XLSX)' : 'No hay filas que exportar'}>
            Exportar XLSX
          </Button>
        </div>
      </div>

      <div className="mb-2 text-[12.5px] text-slate-500">
        {libro === 'ventas' ? 'Libro de Ventas' : 'Libro de Compras'} ·
        <span className="font-medium text-slate-700 dark:text-slate-200"> {MESES[mes - 1]} {anio}</span> ·
        <span className="text-slate-400"> todas las sedes del contribuyente</span>
      </div>

      {/* Nota del Libro de Compras: base best-effort en órdenes recibidas. */}
      {libro === 'compras' ? (
        <div className="mb-3 flex items-start gap-2 text-[11.5px] text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/20 rounded-lg px-3 py-2">
          <Icon.CircleAlert size={14} className="mt-0.5 shrink-0" />
          <span>Se declaran las <strong>facturas fiscales</strong> registradas del proveedor (con su nº de control y la retención que se le hizo). Las órdenes recibidas sin factura no son compras fiscales y no entran.</span>
        </div>
      ) : null}

      {error ? (
        <Empty icon={<Icon.CircleAlert size={22} />} title="No se pudo cargar el libro"
          body={String(error.message || error)}
          cta={<Button onClick={cargar} icon={<Icon.Refresh size={15} />}>Reintentar</Button>} />
      ) : (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-card overflow-hidden">
          {data === undefined ? (
            <div className="p-4"><TableSkeleton rows={6} cols={libro === 'ventas' ? 17 : 13} /></div>
          ) : filas.length === 0 ? (
            <Empty framed={false} icon={<Icon.Book size={22} />}
              title="Sin documentos en este período"
              body={`No hay ${libro === 'ventas' ? 'documentos fiscales' : 'órdenes de compra recibidas'} en ${MESES[mes - 1]} ${anio}.`} />
          ) : libro === 'ventas' ? (
            <TablaVentas filas={filas} totales={totales} ccy={ccy} />
          ) : (
            <TablaCompras filas={filas} totales={totales} ccy={ccy} />
          )}
        </div>
      )}

      {filas.length ? (
        <div className="mt-2 text-[11.5px] text-slate-400">
          {fmtNum(filas.length, 0)} documento(s) · El XLSX trae el libro completo con una columna por alícuota (general, reducida y recargo suntuario), el nº de control y el IVA retenido. Los comprobantes de retención se exportan aparte, en el formato oficial del SENIAT.
        </div>
      ) : null}
    </div>
  )
}

/* Alic pinta el par base/IVA de UNA alícuota. Un cero se muestra como «—» y no
 * como «0,00»: en un libro con tres columnas de alícuota, la mayoría de las
 * filas usa una sola, y los ceros escritos hacen ilegible lo que sí tiene valor. */
const Alic = ({ base, iva, ccy, neg }) => {
  const cls = `py-2 px-3 text-right num text-[12.5px] ${neg ? 'text-amber-700 dark:text-amber-300' : 'text-slate-500'}`
  const vacio = !base && !iva
  return (
    <>
      <td className={cls}>{vacio ? <span className="text-slate-300 dark:text-slate-600">—</span> : fmtCurrency(base, ccy)}</td>
      <td className={cls}>{vacio ? <span className="text-slate-300 dark:text-slate-600">—</span> : fmtCurrency(iva, ccy)}</td>
    </>
  )
}

// --- Libro de Ventas ---
function TablaVentas({ filas, totales, ccy }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm tbl-sticky">
        <thead>
          <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
            <th className="py-2.5 px-3 font-medium">Fecha</th>
            <th className="py-2.5 px-3 font-medium">Tipo</th>
            <th className="py-2.5 px-3 font-medium">Documento</th>
            <th className="py-2.5 px-3 font-medium">Nº control</th>
            <th className="py-2.5 px-3 font-medium">RIF / Cédula</th>
            <th className="py-2.5 px-3 font-medium">Cliente</th>
            <th className="py-2.5 px-3 font-medium text-right">Base exenta</th>
            {/* Una columna por alícuota: el SENIAT no admite sumarlas. El
                recargo suntuario va aparte aunque comparta base con la general. */}
            <th className="py-2.5 px-3 font-medium text-right">Base 16%</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA 16%</th>
            <th className="py-2.5 px-3 font-medium text-right">Base 8%</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA 8%</th>
            <th className="py-2.5 px-3 font-medium text-right">Base adic.</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA adic.</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA débito</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA retenido</th>
            <th className="py-2.5 px-3 font-medium text-right">IGTF</th>
            <th className="py-2.5 px-3 font-medium text-right">Total</th>
          </tr>
        </thead>
        <tbody>
          {filas.map((f, i) => {
            const neg = esNegativo(f.tipo)
            return (
              <tr key={i} className={`border-b border-slate-100 dark:border-slate-800/70 ${neg ? 'bg-amber-50/40 dark:bg-amber-900/10' : ''}`}>
                <td className="py-2 px-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(f.fecha)}</td>
                <td className="py-2 px-3 text-[12.5px]">
                  {neg ? <Badge color="amber" size="sm">{TIPO_LABEL[f.tipo] || f.tipo}</Badge>
                    : <span className="text-slate-500">{TIPO_LABEL[f.tipo] || f.tipo}</span>}
                </td>
                <td className="py-2 px-3 num text-[12.5px] font-medium whitespace-nowrap">{f.numeroCompleto}</td>
                <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.numeroControl || '—'}</td>
                <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.clienteDocumento || '—'}</td>
                <td className="py-2 px-3 text-[12.5px] text-slate-600 dark:text-slate-300 max-w-[180px] truncate">{f.clienteNombre || '—'}</td>
                <td className={`py-2 px-3 text-right num text-[12.5px] ${neg ? 'text-amber-700 dark:text-amber-300' : 'text-slate-500'}`}>{fmtCurrency(f.baseExenta, ccy)}</td>
                <Alic neg={neg} base={f.baseGeneral} iva={f.ivaGeneral} ccy={ccy} />
                <Alic neg={neg} base={f.baseReducida} iva={f.ivaReducida} ccy={ccy} />
                <Alic neg={neg} base={f.baseAdicional} iva={f.ivaAdicional} ccy={ccy} />
                <td className={`py-2 px-3 text-right num text-[12.5px] font-medium ${neg ? 'text-amber-700 dark:text-amber-300' : ''}`}>{fmtCurrency(f.ivaDebito, ccy)}</td>
                <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{f.ivaRetenido ? fmtCurrency(f.ivaRetenido, ccy) : '—'}</td>
                <td className={`py-2 px-3 text-right num text-[12.5px] ${neg ? 'text-amber-700 dark:text-amber-300' : 'text-slate-500'}`}>{fmtCurrency(f.igtf, ccy)}</td>
                <td className={`py-2 px-3 text-right num text-[12.5px] font-medium ${neg ? 'text-amber-700 dark:text-amber-300' : ''}`}>{fmtCurrency(f.total, ccy)}</td>
              </tr>
            )
          })}
        </tbody>
        <tfoot>
          <tr className="sticky bottom-0 bg-slate-100 dark:bg-slate-800 border-t-2 border-slate-300 dark:border-slate-700 font-semibold text-[12.5px]">
            <td className="py-2.5 px-3" colSpan={6}>Totales del período</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseExenta, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseGeneral, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaGeneral, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseReducida, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaReducida, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseAdicional, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaAdicional, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaDebito, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaRetenido, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.igtf, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.total, ccy)}</td>
          </tr>
        </tfoot>
      </table>
    </div>
  )
}

// --- Libro de Compras ---
function TablaCompras({ filas, totales, ccy }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm tbl-sticky">
        <thead>
          <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-200 dark:border-slate-800 bg-slate-50/60 dark:bg-slate-900/60">
            <th className="py-2.5 px-3 font-medium">Fecha</th>
            <th className="py-2.5 px-3 font-medium">RIF proveedor</th>
            <th className="py-2.5 px-3 font-medium">Proveedor</th>
            {/* Es la FACTURA del proveedor, no la orden de compra: la columna
                estaba mal rotulada y además leía un campo que no existe. */}
            <th className="py-2.5 px-3 font-medium">Nº factura</th>
            <th className="py-2.5 px-3 font-medium">Nº control</th>
            <th className="py-2.5 px-3 font-medium text-right">Base exenta</th>
            <th className="py-2.5 px-3 font-medium text-right">Base 16%</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA 16%</th>
            <th className="py-2.5 px-3 font-medium text-right">Base 8%</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA 8%</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA crédito</th>
            <th className="py-2.5 px-3 font-medium text-right">IVA retenido</th>
            <th className="py-2.5 px-3 font-medium text-right">Total</th>
          </tr>
        </thead>
        <tbody>
          {filas.map((f, i) => (
            <tr key={i} className="border-b border-slate-100 dark:border-slate-800/70">
              <td className="py-2 px-3 whitespace-nowrap text-[12.5px] text-slate-500 num">{fmtDate(f.fecha)}</td>
              <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.proveedorRif || '—'}</td>
              <td className="py-2 px-3 text-[12.5px] text-slate-600 dark:text-slate-300 max-w-[200px] truncate">{f.proveedorNombre || '—'}</td>
              <td className="py-2 px-3 num text-[12.5px] font-medium whitespace-nowrap">{f.numeroFactura || '—'}</td>
              <td className="py-2 px-3 num text-[12px] text-slate-500 whitespace-nowrap">{f.numeroControl || '—'}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{fmtCurrency(f.baseExenta, ccy)}</td>
              <Alic base={f.baseGeneral} iva={f.ivaGeneral} ccy={ccy} />
              <Alic base={f.baseReducida} iva={f.ivaReducida} ccy={ccy} />
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium">{fmtCurrency(f.ivaCreditoFiscal, ccy)}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] text-slate-500">{f.ivaRetenido ? fmtCurrency(f.ivaRetenido, ccy) : '—'}</td>
              <td className="py-2 px-3 text-right num text-[12.5px] font-medium">{fmtCurrency(f.total, ccy)}</td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          <tr className="sticky bottom-0 bg-slate-100 dark:bg-slate-800 border-t-2 border-slate-300 dark:border-slate-700 font-semibold text-[12.5px]">
            <td className="py-2.5 px-3" colSpan={5}>Totales del período</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseExenta, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseGeneral, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaGeneral, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.baseReducida, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaReducida, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaCreditoFiscal, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.ivaRetenido, ccy)}</td>
            <td className="py-2.5 px-3 text-right num">{fmtCurrency(totales.total, ccy)}</td>
          </tr>
        </tfoot>
      </table>
    </div>
  )
}

/* Exportación del libro a XLSX.
 *
 * La contadora pidió hoja de cálculo y no TXT. Se arma con lib/xlsx.js (ZIP +
 * XML a mano, sin dependencias): un CSV renombrado a .xlsx no lo abre Excel, y
 * el CSV crudo se reinterpreta con la configuración regional del equipo.
 *
 * Los montos van como NÚMERO, no como texto formateado: el libro se abre para
 * sumarlo y filtrarlo. Los identificadores (RIF, nº de control, nº de factura)
 * van como TEXTO — un RIF «J-12345678-9» que Excel tome por número se convierte
 * en una resta, y un nº de control con ceros a la izquierda los pierde.
 */

// fechaLibro: ISO → DD/MM/AAAA en UTC (evita corrimientos por zona horaria).
function fechaLibro(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d)) return String(iso)
  const dd = String(d.getUTCDate()).padStart(2, '0')
  const mm = String(d.getUTCMonth() + 1).padStart(2, '0')
  return `${dd}/${mm}/${d.getUTCFullYear()}`
}

// n deja el valor como número puro para la celda (null/undefined → 0).
const n = (v) => Number(v) || 0
// pct: fracción (0.16) → 16, para que la columna se lea como porcentaje.
const pct = (v) => Math.round((Number(v) || 0) * 10000) / 100

function exportarLibroXLSX(libro, anio, mes, filas, totales, tipoLabel, empresa) {
  const per = `${anio}-${String(mes).padStart(2, '0')}`
  const rif = empresa?.rif || ''
  const razon = empresa?.razonSocial || empresa?.nombre || ''
  const titulo = libro === 'ventas' ? 'LIBRO DE VENTAS' : 'LIBRO DE COMPRAS'

  // Cabecera de identificación del contribuyente: el libro impreso la lleva y
  // sin ella la hoja no se puede presentar tal cual.
  const cuerpo = [
    [titulo],
    ['RIF', rif, 'Razón social', razon, 'Período', `${String(mes).padStart(2, '0')}/${anio}`],
    [],
  ]

  let encabezado, mapa, totalRow
  if (libro === 'ventas') {
    encabezado = ['Fecha', 'Tipo', 'Nº documento', 'Nº control', 'RIF / Cédula', 'Razón social',
      'Base exenta', 'Base 16%', 'Alíc. 16%', 'IVA 16%', 'Base 8%', 'Alíc. 8%', 'IVA 8%',
      'Base adicional', 'Alíc. adicional', 'IVA adicional', 'IVA débito', 'IVA retenido', 'IGTF', 'Total']
    mapa = (f) => [
      fechaLibro(f.fecha), tipoLabel[f.tipo] || f.tipo, f.numeroCompleto || '', f.numeroControl || '',
      f.clienteDocumento || '', f.clienteNombre || '',
      n(f.baseExenta),
      n(f.baseGeneral), pct(f.alicuotaGeneral), n(f.ivaGeneral),
      n(f.baseReducida), pct(f.alicuotaReducida), n(f.ivaReducida),
      n(f.baseAdicional), pct(f.alicuotaAdicional), n(f.ivaAdicional),
      n(f.ivaDebito), n(f.ivaRetenido), n(f.igtf), n(f.total),
    ]
    totalRow = ['TOTALES', '', '', '', '', '',
      n(totales.baseExenta),
      n(totales.baseGeneral), '', n(totales.ivaGeneral),
      n(totales.baseReducida), '', n(totales.ivaReducida),
      n(totales.baseAdicional), '', n(totales.ivaAdicional),
      n(totales.ivaDebito), n(totales.ivaRetenido), n(totales.igtf), n(totales.total)]
  } else {
    encabezado = ['Fecha', 'RIF proveedor', 'Razón social', 'Nº factura', 'Nº control',
      'Base exenta', 'Base 16%', 'Alíc. 16%', 'IVA 16%', 'Base 8%', 'Alíc. 8%', 'IVA 8%',
      'IVA crédito fiscal', 'IVA retenido', 'Total']
    mapa = (f) => [
      fechaLibro(f.fecha), f.proveedorRif || '', f.proveedorNombre || '',
      f.numeroFactura || '', f.numeroControl || '',
      n(f.baseExenta),
      n(f.baseGeneral), pct(f.alicuotaGeneral), n(f.ivaGeneral),
      n(f.baseReducida), pct(f.alicuotaReducida), n(f.ivaReducida),
      n(f.ivaCreditoFiscal), n(f.ivaRetenido), n(f.total),
    ]
    totalRow = ['TOTALES', '', '', '', '',
      n(totales.baseExenta),
      n(totales.baseGeneral), '', n(totales.ivaGeneral),
      n(totales.baseReducida), '', n(totales.ivaReducida),
      n(totales.ivaCreditoFiscal), n(totales.ivaRetenido), n(totales.total)]
  }

  // El encabezado de columnas va en su propia fila para que salga en negrita:
  // hojaXLSX destaca la PRIMERA fila, así que el título del libro va arriba y
  // las columnas se repiten como fila normal. Se marca visualmente con el
  // contenido, no con estilo.
  const filasXLSX = [encabezado, ...filas.map(mapa), totalRow]
  const hoja = [...cuerpo, ...filasXLSX]

  descargarXLSX(`libro-${libro}-${per}.xlsx`, libro === 'ventas' ? 'Ventas' : 'Compras', hoja)
}
