import { describe, it, expect } from 'vitest'
import {
  dimensionDeMesa, aforoMaximo, ocupaCelda, cabeEn, tamanoAlArrastrar, aforoAjustado,
  cabeArea, cabeMostrador, rectDeArrastre, zonaDeCelda, siguienteNombreMesa,
  MAX_CELDAS_MESA,
} from '../plano.js'

/* GEOMETRÍA DEL PLANO DEL SALÓN.
 *
 * Es espejo de domain/mesa en el servidor, que vuelve a validar y rechaza los
 * planos encimados. Si las dos reglas se separan, el editor deja dibujar algo
 * que después no se puede guardar — y el error llega cuando ya se movieron
 * veinte mesas. Por eso esto se prueba acá y allá. */

const plano = { filas: 5, columnas: 6, bloqueadas: [{ columna: 5, fila: 0 }] }
const meson = { id: 'm1', columna: 1, fila: 1, anchoCeldas: 2, altoCeldas: 2 }
const chica = { id: 'm2', columna: 4, fila: 4, anchoCeldas: 1, altoCeldas: 1 }
const salon = { mesas: [meson, chica], plano }

describe('dimensionDeMesa', () => {
  it('respeta el tamaño guardado', () => {
    expect(dimensionDeMesa({ anchoCeldas: 3, altoCeldas: 2, capacidad: 4 })).toEqual([3, 2])
  })

  // Sin tamaño guardado (mesas anteriores al redimensionado) se deriva de la
  // capacidad: así los planos ya dibujados no necesitan migración.
  it('sin tamaño guardado lo deriva de la capacidad', () => {
    expect(dimensionDeMesa({ capacidad: 4 })).toEqual([1, 1])
    expect(dimensionDeMesa({ capacidad: 5 })).toEqual([2, 1]) // 5 no caben en un cuadro
    expect(dimensionDeMesa({ capacidad: 12 })).toEqual([3, 1]) // hasta 3, a lo largo
  })

  it('el tamaño derivado SIEMPRE alcanza para la capacidad', () => {
    for (let cap = 0; cap <= 60; cap++) {
      const [a, b] = dimensionDeMesa({ capacidad: cap })
      expect(aforoMaximo(a, b)).toBeGreaterThanOrEqual(cap)
    }
  })

  it('nunca devuelve cero cuadros', () => {
    expect(dimensionDeMesa({})).toEqual([1, 1])
    expect(dimensionDeMesa({ capacidad: 0 })).toEqual([1, 1])
  })
})

describe('ocupaCelda', () => {
  // Mirar solo la esquina dejaría soltar una mesa sobre la mitad de un mesón,
  // que a simple vista parece celda libre.
  it('cubre toda la superficie, no solo la esquina', () => {
    expect(ocupaCelda(meson, 1, 1)).toBe(true)
    expect(ocupaCelda(meson, 2, 2)).toBe(true)
    expect(ocupaCelda(meson, 3, 1)).toBe(false)
    expect(ocupaCelda(meson, 0, 1)).toBe(false)
  })
})

describe('cabeEn', () => {
  it('acepta una superficie libre dentro del plano', () => {
    expect(cabeEn(salon, 'nueva', 3, 3, 1, 1)).toBe(true)
  })

  it('rechaza lo que se sale del plano', () => {
    expect(cabeEn(salon, 'nueva', 5, 0, 2, 1)).toBe(false)
    expect(cabeEn(salon, 'nueva', 0, 4, 1, 2)).toBe(false)
    expect(cabeEn(salon, 'nueva', -1, 0, 1, 1)).toBe(false)
  })

  it('rechaza pisar otra mesa, aunque sea por su interior', () => {
    expect(cabeEn(salon, 'nueva', 2, 2, 1, 1)).toBe(false)
    // Una mesa grande cuya ESQUINA cae en celda libre pero cuyo cuerpo pisa.
    expect(cabeEn(salon, 'nueva', 0, 0, 3, 3)).toBe(false)
  })

  it('rechaza celdas bloqueadas (paredes, cocina)', () => {
    expect(cabeEn(salon, 'nueva', 5, 0, 1, 1)).toBe(false)
  })

  // Mover una mesa un paso no puede chocar CONSIGO MISMA: sin esto, arrastrar
  // un mesón de 2×2 una celda sería siempre inválido.
  it('se ignora a sí misma al moverse', () => {
    expect(cabeEn(salon, 'm1', 2, 1, 2, 2)).toBe(true)
    expect(cabeEn(salon, 'm1', 1, 1, 3, 2)).toBe(true) // crecer sobre sí misma
  })

  it('rechaza tamaños imposibles', () => {
    expect(cabeEn(salon, 'nueva', 0, 0, 0, 1)).toBe(false)
    expect(cabeEn(salon, 'nueva', 0, 0, MAX_CELDAS_MESA + 1, 1)).toBe(false)
  })
})

describe('tamanoAlArrastrar', () => {
  const base = { col: 2, fila: 1, dc: 1, df: 1 }

  it('el tirador de ancho solo cambia el ancho', () => {
    expect(tamanoAlArrastrar({ ...base, tipo: 'ancho' }, 4, 9)).toEqual([3, 1])
  })

  it('el tirador de alto solo cambia el alto', () => {
    expect(tamanoAlArrastrar({ ...base, tipo: 'alto' }, 9, 3)).toEqual([1, 3])
  })

  it('el de la esquina cambia los dos', () => {
    expect(tamanoAlArrastrar({ ...base, tipo: 'ambos' }, 3, 2)).toEqual([2, 2])
  })

  // Arrastrar hacia atrás de la esquina no puede dar una mesa de cero cuadros.
  it('nunca baja de un cuadro ni pasa del máximo', () => {
    expect(tamanoAlArrastrar({ ...base, tipo: 'ambos' }, 0, 0)).toEqual([1, 1])
    expect(tamanoAlArrastrar({ ...base, tipo: 'ambos' }, 50, 50))
      .toEqual([MAX_CELDAS_MESA, MAX_CELDAS_MESA])
  })
})

describe('aforoAjustado', () => {
  // Al achicar, dejar el aforo por encima del tope haría que el servidor
  // rechazara el guardado con un número que la pantalla mostraba como válido.
  it('recorta el aforo al tope del nuevo tamaño', () => {
    expect(aforoAjustado(12, 1, 1)).toBe(4)
  })

  it('no toca un aforo que ya entra', () => {
    expect(aforoAjustado(6, 2, 1)).toBe(6)
  })
})


/* ÁREAS Y MOSTRADORES.
 *
 * La diferencia que manda: un MOSTRADOR ocupa piso (es un mueble real, ahí no
 * entra una mesa) y un ÁREA no (es una etiqueta de superficie, y las mesas viven
 * adentro). Confundirlas haría que dibujar la terraza expulsara sus mesas. */

const barra = { id: 'b1', nombre: 'Barra', tipo: 'barra', columna: 0, fila: 3, ancho: 3, alto: 1 }
const terraza = { id: 'a1', nombre: 'Terraza', columna: 0, fila: 0, ancho: 3, alto: 2 }
const conMuebles = {
  mesas: [meson, chica],
  plano: { ...plano, mostradores: [barra], areas: [terraza] },
}

describe('mostradores', () => {
  it('una mesa no puede ponerse encima de la barra', () => {
    expect(cabeEn(conMuebles, 'nueva', 0, 3, 1, 1)).toBe(false)
    expect(cabeEn(conMuebles, 'nueva', 3, 3, 1, 1)).toBe(true)
  })

  it('un mostrador no puede pisar una mesa ni otro mostrador', () => {
    expect(cabeMostrador(conMuebles, 'nuevo', 1, 1, 1, 1)).toBe(false) // sobre el mesón
    expect(cabeMostrador(conMuebles, 'nuevo', 2, 3, 1, 1)).toBe(false) // sobre la barra
    expect(cabeMostrador(conMuebles, 'b1', 0, 3, 4, 1)).toBe(true)     // alargarse a sí misma
  })

  // Una barra puede recorrer la pared entera: el tope de 6 cuadros es de las
  // mesas (por el aforo), no de los muebles.
  it('no tiene el tope de tamaño de una mesa', () => {
    const largo = { mesas: [], plano: { filas: 5, columnas: 10, bloqueadas: [], mostradores: [], areas: [] } }
    expect(cabeMostrador(largo, 'x', 0, 0, 10, 1)).toBe(true)
    expect(cabeEn(largo, 'x', 0, 0, 10, 1)).toBe(false)
  })
})

describe('áreas', () => {
  // Para esto están: contienen las mesas.
  it('se superpone a mesas y muebles sin problema', () => {
    // Un área sobre la barra y libre de otras áreas: para eso está, los contiene.
    expect(cabeArea(conMuebles, 'nueva', 0, 3, 3, 2)).toBe(true)
  })

  it('no se superpone a otra área: la zona quedaría ambigua', () => {
    expect(cabeArea(conMuebles, 'nueva', 2, 1, 2, 2)).toBe(false)
    expect(cabeArea(conMuebles, 'a1', 0, 0, 4, 3)).toBe(true) // agrandarse a sí misma
  })

  it('no se sale del plano', () => {
    expect(cabeArea(conMuebles, 'nueva', 5, 4, 2, 2)).toBe(false)
  })
})

describe('zonaDeCelda', () => {
  // La zona se DEDUCE de dónde está la mesa. Escrita a mano en cada ficha
  // terminaba con «Terraza», «terraza» y «Terrraza» conviviendo, y cualquier
  // agrupación por zona salía mal.
  it('devuelve el nombre del área que contiene la celda', () => {
    expect(zonaDeCelda([terraza], 1, 1)).toBe('Terraza')
    expect(zonaDeCelda([terraza], 1, 3)).toBe('')
    expect(zonaDeCelda([], 0, 0)).toBe('')
  })
})

describe('rectDeArrastre', () => {
  // Arrastrar hacia arriba y a la izquierda tiene que dibujar igual que hacia
  // abajo y a la derecha.
  it('normaliza el rectángulo venga de donde venga', () => {
    expect(rectDeArrastre(1, 1, 3, 2)).toEqual({ c: 1, r: 1, dc: 3, df: 2 })
    expect(rectDeArrastre(3, 2, 1, 1)).toEqual({ c: 1, r: 1, dc: 3, df: 2 })
  })

  it('un toque sin desplazamiento es un cuadro', () => {
    expect(rectDeArrastre(2, 2, 2, 2)).toEqual({ c: 2, r: 2, dc: 1, df: 1 })
  })
})

describe('siguienteNombreMesa', () => {
  // Las mesas de un salón se llaman por número: crear una no debería obligar a
  // pensar el nombre.
  it('propone el número que sigue', () => {
    expect(siguienteNombreMesa([{ nombre: '1' }, { nombre: '7' }, { nombre: '3' }])).toBe('8')
  })

  it('ignora los nombres que no son números', () => {
    expect(siguienteNombreMesa([{ nombre: 'Barra 1' }, { nombre: '2' }])).toBe('3')
  })

  it('empieza en 1 con el salón vacío', () => {
    expect(siguienteNombreMesa([])).toBe('1')
  })
})
