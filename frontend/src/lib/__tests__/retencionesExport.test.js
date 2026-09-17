import { describe, it, expect } from 'vitest'
import {
  filtrarRetenciones, txtRetencionesIVA, xmlRetencionesISLR,
  fechaSENIAT, periodoDe, escaparXML, nombreArchivoRetenciones,
  COLUMNAS_TXT_IVA,
} from '../retencionesExport.js'

/* Estos archivos se PRESENTAN ante el SENIAT: un campo mal armado no se descubre
 * revisando la pantalla, se descubre cuando el portal rechaza la declaración. Lo
 * que se prueba acá es justamente lo que no se ve a simple vista — los
 * identificadores que no se pueden tocar, el escape del XML y el filtro del
 * período. */

const empresa = { rif: 'J-40123456-9', razonSocial: 'Bodega La Cima, C.A.' }

const ivaAgosto = {
  id: 'r1', tipo: 'emitida', impuesto: 'iva',
  numeroComprobante: '00001234', fecha: '2026-08-14',
  terceroRif: 'J-12345678-9', terceroNombre: 'Distribuidora & Cía',
  documentoNumero: 'FAC-000045', base: 1600, porcentaje: 75, montoRetenido: 1200,
}
const islrAgosto = {
  id: 'r2', tipo: 'emitida', impuesto: 'islr',
  numeroComprobante: '00009999', fecha: '2026-08-20',
  terceroRif: 'V-12345678', terceroNombre: 'Luis <Pérez>',
  documentoNumero: 'FAC-000046', concepto: 'Honorarios profesionales',
  base: 10000, porcentaje: 3, sustraendo: 50, montoRetenido: 250,
}
const ivaSeptiembre = { ...ivaAgosto, id: 'r3', fecha: '2026-09-02' }

describe('filtrarRetenciones', () => {
  it('acota por período según la fecha del COMPROBANTE', () => {
    const todas = [ivaAgosto, islrAgosto, ivaSeptiembre]
    const agosto = filtrarRetenciones(todas, { anio: 2026, mes: 8 })
    expect(agosto.map((r) => r.id)).toEqual(['r1', 'r2'])
  })

  it('acota por impuesto, tratando el impuesto vacío como IVA', () => {
    // Un comprobante viejo puede no traer `impuesto`: antes solo había IVA, y
    // dejarlo fuera del archivo sería no declararlo.
    const viejo = { ...ivaAgosto, id: 'r4', impuesto: undefined }
    const iva = filtrarRetenciones([ivaAgosto, islrAgosto, viejo], { impuesto: 'iva' })
    expect(iva.map((r) => r.id)).toEqual(['r1', 'r4'])
  })

  it('acota por dirección cuando se pide', () => {
    const recibida = { ...ivaAgosto, id: 'r5', tipo: 'recibida' }
    const out = filtrarRetenciones([ivaAgosto, recibida], { tipo: 'emitida' })
    expect(out.map((r) => r.id)).toEqual(['r1'])
  })

  it('sin criterios devuelve todo, y tolera una lista vacía', () => {
    expect(filtrarRetenciones([ivaAgosto, islrAgosto])).toHaveLength(2)
    expect(filtrarRetenciones(null, { anio: 2026, mes: 8 })).toEqual([])
  })
})

describe('fechaSENIAT', () => {
  it('convierte a DD/MM/AAAA', () => {
    expect(fechaSENIAT('2026-08-14')).toBe('14/08/2026')
  })

  it('usa UTC: el primero del mes no puede correrse al mes anterior', () => {
    // Con hora local, una fecha a medianoche se va al día previo según la zona —
    // y una retención declarada en el mes equivocado es un problema real.
    expect(fechaSENIAT('2026-08-01T00:00:00Z')).toBe('01/08/2026')
  })

  it('no inventa una fecha cuando el dato no es una', () => {
    expect(fechaSENIAT('')).toBe('')
    expect(fechaSENIAT('sin fecha')).toBe('sin fecha')
  })
})

describe('txtRetencionesIVA', () => {
  it('escribe una línea por comprobante, sin encabezado', () => {
    const txt = txtRetencionesIVA([ivaAgosto], empresa, 2026, 8)
    const lineas = txt.trim().split('\r\n')
    expect(lineas).toHaveLength(1)
    // Sin fila de títulos: un archivo de presentación es solo datos.
    expect(txt).not.toContain('RIF del agente')
  })

  it('pone tantos campos como columnas documentadas', () => {
    const txt = txtRetencionesIVA([ivaAgosto], empresa, 2026, 8)
    expect(txt.trim().split('|')).toHaveLength(COLUMNAS_TXT_IVA.length)
  })

  it('NO toca los identificadores: el RIF conserva sus guiones y el correlativo sus ceros', () => {
    // Es el error que ya nos mordió en los exports: un RIF tomado por número se
    // vuelve una resta y «00001234» pierde los ceros de la izquierda.
    const campos = txtRetencionesIVA([ivaAgosto], empresa, 2026, 8).trim().split('|')
    // Se miran los CAMPOS, no subcadenas: «00001234» contiene «1234», así que
    // buscar la subcadena no distingue el correlativo bueno del mutilado.
    expect(campos[2]).toBe('J-12345678-9') // RIF con sus guiones
    expect(campos[3]).toBe('00001234')     // correlativo con sus ceros
  })

  it('escribe los montos con dos decimales', () => {
    const txt = txtRetencionesIVA([{ ...ivaAgosto, base: 1600, montoRetenido: 1200 }], empresa, 2026, 8)
    expect(txt).toContain('1600.00')
    expect(txt).toContain('1200.00')
  })

  it('lleva el RIF del agente y el período en cada línea', () => {
    const txt = txtRetencionesIVA([ivaAgosto, { ...ivaAgosto, id: 'x' }], empresa, 2026, 8)
    for (const l of txt.trim().split('\r\n')) {
      expect(l.startsWith('J-40123456-9|202608')).toBe(true)
    }
  })

  it('devuelve vacío sin comprobantes, para no ofrecer un archivo sin nada', () => {
    expect(txtRetencionesIVA([], empresa, 2026, 8)).toBe('')
  })

  it('termina en salto de línea: sin él varios lectores descartan el último renglón', () => {
    expect(txtRetencionesIVA([ivaAgosto], empresa, 2026, 8).endsWith('\r\n')).toBe(true)
  })
})

describe('xmlRetencionesISLR', () => {
  it('declara el agente con su RIF y su período', () => {
    const xml = xmlRetencionesISLR([islrAgosto], empresa, 2026, 8)
    expect(xml).toContain('<Rif>J-40123456-9</Rif>')
    expect(xml).toContain('<Periodo>202608</Periodo>')
  })

  it('escapa el contenido: un «&» o un «<» en un nombre rompen el archivo entero', () => {
    const xml = xmlRetencionesISLR([islrAgosto], { ...empresa, razonSocial: 'Pérez & Hijos' }, 2026, 8)
    expect(xml).toContain('Pérez &amp; Hijos')
    expect(xml).toContain('Luis &lt;Pérez&gt;')
    // Y ningún ampersand crudo quedó suelto.
    expect(xml).not.toMatch(/&(?!amp;|lt;|gt;|quot;|apos;)/)
  })

  it('incluye el SUSTRAENDO: sin él el monto retenido no se puede reconstruir', () => {
    // En ISLR el impuesto es base×% menos el sustraendo de la tabla; omitirlo
    // deja un monto que no cuadra con sus propios datos.
    const xml = xmlRetencionesISLR([islrAgosto], empresa, 2026, 8)
    expect(xml).toContain('<Sustraendo>50.00</Sustraendo>')
    expect(xml).toContain('<ImpuestoRetenido>250.00</ImpuestoRetenido>')
  })

  it('abre y cierra un <Retencion> por comprobante', () => {
    const xml = xmlRetencionesISLR([islrAgosto, { ...islrAgosto, id: 'z' }], empresa, 2026, 8)
    expect(xml.match(/<Retencion>/g)).toHaveLength(2)
    expect(xml.match(/<\/Retencion>/g)).toHaveLength(2)
  })

  it('sin comprobantes sigue siendo un XML bien formado', () => {
    const xml = xmlRetencionesISLR([], empresa, 2026, 8)
    expect(xml).toContain('<RelacionRetencionesISLR>')
    expect(xml).toContain('</RelacionRetencionesISLR>')
    expect(xml).not.toContain('<Retencion>')
  })
})

describe('escaparXML', () => {
  it('descarta los caracteres de control, que invalidan el XML entero', () => {
    expect(escaparXML('a\x00b\x1Fc')).toBe('abc')
  })
})

describe('periodoDe y nombreArchivoRetenciones', () => {
  it('arma el período AAAAMM con el mes en dos dígitos', () => {
    expect(periodoDe(2026, 8)).toBe('202608')
    expect(periodoDe(2026, 12)).toBe('202612')
  })

  it('nombra el archivo con RIF y período: se acumulan de varias empresas y meses', () => {
    expect(nombreArchivoRetenciones('iva', empresa, 2026, 8, 'txt'))
      .toBe('retenciones-iva-J401234569-202608.txt')
  })

  it('no deja un nombre roto cuando la empresa no tiene RIF', () => {
    expect(nombreArchivoRetenciones('islr', {}, 2026, 8, 'xml'))
      .toBe('retenciones-islr-sinrif-202608.xml')
  })
})
