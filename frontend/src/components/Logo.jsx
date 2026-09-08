import logoUrl from '../assets/elerp-logo.svg'

// Marca ElERP — logotipo OFICIAL del manual (SVG vectorial "elerp", tinta negra +
// acento violeta #6A2CF0). En fondos oscuros se usa la versión BLANCA sólida (filtro
// brightness(0) invert(1), = versión "Blanco sólido" del manual).
//
// El SVG es el logotipo completo (ratio 3.43:1). El isotipo "el" (para íconos/espacios
// cuadrados) se obtiene recortando la parte izquierda del mismo archivo, así hay una sola
// fuente de verdad.

const filtroMono = 'brightness(0) invert(1)'

// Logo = ISOTIPO "el" (cuadrado). Recorta la parte izquierda del logotipo oficial.
export const Logo = ({ size = 28, mono = false }) => (
  <span style={{ display: 'inline-flex', alignItems: 'center', width: size, height: size, overflow: 'hidden', flex: 'none' }}>
    <img src={logoUrl} alt="elerp" aria-hidden="true"
      style={{ height: size, width: 'auto', maxWidth: 'none', display: 'block', filter: mono ? filtroMono : 'none' }} />
  </span>
)

// Wordmark = LOGOTIPO completo "elerp". `size` es la ALTURA; el ancho se ajusta al ratio.
export const Wordmark = ({ size = 28, className = '', mono = false }) => (
  <img src={logoUrl} alt="elerp"
    className={className}
    style={{ height: size, width: 'auto', display: 'block', filter: mono ? filtroMono : 'none' }} />
)
