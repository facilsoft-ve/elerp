/* GEOMETRÍA DEL PLANO DEL SALÓN.
 *
 * CADA CUADRO DEL PLANO ADMITE 4 PERSONAS. Una mesa de 2, 3 o 4 ocupa un
 * cuadro; para sentar a más hay que AMPLIARLA —arrastrando sus bordes en el
 * editor— y recién ahí se puede elegir un aforo mayor. Manda el TAMAÑO y el
 * aforo se acomoda: el plano es la realidad física del local, no al revés. Si el
 * aforo mandara sobre el tamaño, teclear «20» haría crecer la mesa sola y pisar
 * a las vecinas.
 *
 * Es ESPEJO de domain/mesa en el servidor (PersonasPorCelda, Dimension,
 * CapacidadMaxima, SeSolapaCon), igual que el dígito verificador del RIF: el
 * servidor vuelve a validar y rechaza los planos encimados, así que si las dos
 * reglas se separan, el editor deja dibujar algo que después no se puede
 * guardar — y el mensaje llega cuando ya se movieron veinte mesas.
 *
 * Vive en lib y no dentro de la pantalla porque es justo la clase de aritmética
 * que se rompe en silencio: una mesa mal medida se ve perfectamente normal.
 */

// PERSONAS_POR_CUADRO: cuánta gente cabe en un cuadro del plano.
export const PERSONAS_POR_CUADRO = 4

// MAX_CELDAS_MESA acota el lado de una mesa en cuadros (espejo de
// `maxCeldasMesa` en el servidor).
export const MAX_CELDAS_MESA = 6

/** dimensionDeMesa es el tamaño EFECTIVO en cuadros. El tamaño guardado manda;
 *  sin él (mesas anteriores al redimensionado) se deriva de la capacidad, así
 *  los planos ya dibujados no necesitan migración. */
export function dimensionDeMesa(m) {
  if (m?.anchoCeldas > 0 && m?.altoCeldas > 0) return [m.anchoCeldas, m.altoCeldas]
  const celdas = Math.max(1, Math.ceil((m?.capacidad || 0) / PERSONAS_POR_CUADRO))
  // Hasta tres cuadros se arma a lo LARGO, que es como se junta una mesa en un
  // salón real (tres mesas en fila, no un bloque).
  if (celdas <= 3) return [celdas, 1]
  // De ahí en adelante, el bloque más compacto que ALCANCE. Tiene que alcanzar
  // siempre: sugerir de menos haría que el alta fallara contra su propio valor
  // por defecto.
  let ancho = 1
  while (ancho * ancho < celdas) ancho++
  return [ancho, Math.ceil(celdas / ancho)]
}

/** aforoMaximo es cuánta gente admite un tamaño: 4 por cuadro. */
export const aforoMaximo = (ancho, alto) => ancho * alto * PERSONAS_POR_CUADRO

/** ocupaCelda indica si la mesa cubre (c, r) contando TODA su superficie, no
 *  solo su esquina. Es lo que impide soltar otra mesa sobre la mitad de un
 *  mesón, que a simple vista parece celda libre. */
export function ocupaCelda(m, c, r) {
  const [dc, df] = dimensionDeMesa(m)
  const mc = m.columna || 0
  const mf = m.fila || 0
  return c >= mc && c < mc + dc && r >= mf && r < mf + df
}

/** cubreCelda: ¿este rectángulo {columna,fila,ancho,alto} cubre (c, r)? */
export const cubreCelda = (x, c, r) =>
  c >= (x.columna || 0) && c < (x.columna || 0) + (x.ancho || 1) &&
  r >= (x.fila || 0) && r < (x.fila || 0) + (x.alto || 1)

/** dentroDelPlano acota un rectángulo a la grilla. */
const dentroDelPlano = (plano, c, r, dc, df) =>
  Number.isFinite(c) && Number.isFinite(r) && c >= 0 && r >= 0 && dc >= 1 && df >= 1 &&
  c + dc <= (plano?.columnas || 0) && r + df <= (plano?.filas || 0)

/** cabeEn responde si una mesa de dc×df entra con su esquina en (c, r): dentro
 *  del plano, sin pisar celdas bloqueadas, otras mesas ni un MOSTRADOR (la barra
 *  y la caja son muebles reales: ahí no entra una mesa). Se ignora a sí misma,
 *  porque mover una mesa un paso no puede chocar consigo misma.
 *
 *  Es la regla que el editor evalúa en CADA celda mientras se arrastra, para
 *  poder decir «acá no cabe» antes de soltar en vez de después de guardar. */
export function cabeEn({ mesas = [], plano }, id, c, r, dc, df) {
  if (!dentroDelPlano(plano, c, r, dc, df)) return false
  if (dc > MAX_CELDAS_MESA || df > MAX_CELDAS_MESA) return false
  const bloqueadas = plano?.bloqueadas || []
  const mostradores = plano?.mostradores || []
  for (let i = 0; i < dc; i++) {
    for (let j = 0; j < df; j++) {
      const cc = c + i
      const rr = r + j
      if (bloqueadas.some((b) => b.columna === cc && b.fila === rr)) return false
      if (mesas.some((m) => m.id !== id && ocupaCelda(m, cc, rr))) return false
      if (mostradores.some((x) => x.id !== id && cubreCelda(x, cc, rr))) return false
    }
  }
  return true
}

/** cabeMostrador: un mueble de servicio (barra, caja, barra de postres) entra
 *  donde no haya mesa, celda bloqueada ni otro mueble. No tiene el tope de 6
 *  cuadros de una mesa: una barra puede recorrer la pared entera. */
export function cabeMostrador({ mesas = [], plano }, id, c, r, dc, df) {
  if (!dentroDelPlano(plano, c, r, dc, df)) return false
  const bloqueadas = plano?.bloqueadas || []
  const mostradores = plano?.mostradores || []
  for (let i = 0; i < dc; i++) {
    for (let j = 0; j < df; j++) {
      const cc = c + i
      const rr = r + j
      if (bloqueadas.some((b) => b.columna === cc && b.fila === rr)) return false
      if (mesas.some((m) => ocupaCelda(m, cc, rr))) return false
      if (mostradores.some((x) => x.id !== id && cubreCelda(x, cc, rr))) return false
    }
  }
  return true
}

/** cabeArea: un área SÍ se superpone a mesas y muebles —para eso está, los
 *  contiene—, pero NO a otra área: dos zonas encimadas harían ambigua la zona de
 *  las mesas de esa franja. */
export function cabeArea({ plano }, id, c, r, dc, df) {
  if (!dentroDelPlano(plano, c, r, dc, df)) return false
  return !(plano?.areas || []).some((a) => a.id !== id && seSolapan(a, { columna: c, fila: r, ancho: dc, alto: df }))
}

const seSolapan = (a, b) =>
  (a.columna || 0) < (b.columna || 0) + (b.ancho || 1) &&
  (b.columna || 0) < (a.columna || 0) + (a.ancho || 1) &&
  (a.fila || 0) < (b.fila || 0) + (b.alto || 1) &&
  (b.fila || 0) < (a.fila || 0) + (a.alto || 1)

/** zonaDeCelda devuelve el nombre del área que contiene la celda. Es lo que
 *  deduce la zona de una mesa por DÓNDE ESTÁ, en vez de que alguien escriba
 *  «Terraza», «terraza» y «Terrraza» en tres fichas distintas. */
export function zonaDeCelda(areas, c, r) {
  const a = (areas || []).find((x) => cubreCelda(x, c, r))
  return a ? a.nombre : ''
}

/** rectDeArrastre normaliza el rectángulo entre dos celdas, venga el arrastre
 *  desde donde venga: hacia arriba y a la izquierda tiene que dibujar igual que
 *  hacia abajo y a la derecha. */
export function rectDeArrastre(c0, r0, c1, r1) {
  return {
    c: Math.min(c0, c1), r: Math.min(r0, r1),
    dc: Math.abs(c1 - c0) + 1, df: Math.abs(r1 - r0) + 1,
  }
}

/** siguienteNombreMesa propone el número siguiente. Las mesas de un salón se
 *  llaman por número, así que crear una no debería obligar a pensar el nombre:
 *  se propone el que sigue y se edita si hace falta. */
export function siguienteNombreMesa(mesas = []) {
  let max = 0
  for (const m of mesas) {
    const n = parseInt(String(m.nombre || '').trim(), 10)
    if (Number.isFinite(n) && n > max) max = n
  }
  return String(max + 1)
}

/** tamanoAlArrastrar traduce la celda bajo el cursor al tamaño que tendría la
 *  mesa, según qué tirador se agarró. La esquina se queda quieta y el lado sigue
 *  al cursor; el otro lado no se toca. */
export function tamanoAlArrastrar({ tipo, col, fila, dc, df }, c, r) {
  const acota = (v) => Math.max(1, Math.min(MAX_CELDAS_MESA, v))
  return [
    tipo === 'alto' ? dc : acota(c - col + 1),
    tipo === 'ancho' ? df : acota(r - fila + 1),
  ]
}

/** aforoAjustado recorta el aforo al tope del nuevo tamaño. Al achicar una mesa,
 *  dejar el aforo por encima haría que el servidor rechazara el guardado con un
 *  número que la pantalla mostraba como válido. */
export const aforoAjustado = (capacidad, ancho, alto) =>
  Math.min(Number(capacidad) || 0, aforoMaximo(ancho, alto))
