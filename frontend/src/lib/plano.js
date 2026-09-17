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

/** cabeEn responde si una mesa de dc×df entra con su esquina en (c, r): dentro
 *  del plano, sin pisar celdas bloqueadas ni otras mesas. Se ignora a sí misma,
 *  porque mover una mesa un paso no puede chocar consigo misma.
 *
 *  Es la regla que el editor evalúa en CADA celda mientras se arrastra, para
 *  poder decir «acá no cabe» antes de soltar en vez de después de guardar. */
export function cabeEn({ mesas = [], plano }, id, c, r, dc, df) {
  if (!Number.isFinite(c) || !Number.isFinite(r)) return false
  if (dc < 1 || df < 1 || dc > MAX_CELDAS_MESA || df > MAX_CELDAS_MESA) return false
  if (c < 0 || r < 0) return false
  if (c + dc > (plano?.columnas || 0) || r + df > (plano?.filas || 0)) return false
  const bloqueadas = plano?.bloqueadas || []
  for (let i = 0; i < dc; i++) {
    for (let j = 0; j < df; j++) {
      const cc = c + i
      const rr = r + j
      if (bloqueadas.some((b) => b.columna === cc && b.fila === rr)) return false
      if (mesas.some((m) => m.id !== id && ocupaCelda(m, cc, rr))) return false
    }
  }
  return true
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
