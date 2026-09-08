/* Tema de la PANTALLA DEL CLIENTE — resolución unificada de la apariencia.
 *
 * La apariencia de la segunda pantalla del POS vive en la empresa
 * (`empresa.temaPantalla`): fondos por espacio, colores de texto/énfasis y qué
 * versión del logo se muestra. Antes estaba dispersa (empresa.colorMarca +
 * caja.colorFondo/logoVersion); esto la unifica.
 *
 * `resolverTema` combina el tema de la empresa con un OVERRIDE por caja (fondo
 * general + versión de logo para contraste) y devuelve valores YA resueltos, con
 * fallback a los defaults de marca. Lo usan tanto la pantalla real
 * (PantallaCliente), la caja (ModoCaja, que lo emite por el canal 'branding') y
 * el editor con vista previa (Configuración › Marketing). */

// Acento por defecto (teal de marca) y fondo navy por defecto.
export const ENFASIS_DEFAULT = '#6A2CF0'
export const TEXTO_DEFAULT = '#ffffff'
export const FONDO_DEFAULT = 'linear-gradient(160deg,#5A21DB 0%,#3B168C 55%,#2A2440 100%)'

// logoDeVersion devuelve la URL del logo de la versión pedida (color|blanco|negro),
// con degradación segura: blanco cae al alterno (compat) y luego al de color; negro
// cae al de color. Nunca rompe si la empresa no cargó esa versión.
export function logoDeVersion(empresa, version) {
  if (!empresa) return ''
  switch (version) {
    case 'blanco': return empresa.logoBlanco || empresa.logoAlterno || empresa.logo || ''
    case 'negro': return empresa.logoNegro || empresa.logo || ''
    default: return empresa.logo || ''
  }
}

/* resolverTema — devuelve la apariencia efectiva de la pantalla del cliente.
 *
 * `empresa` es db.EMPRESA (con `temaPantalla`, `logo`, `logoBlanco`, `logoNegro`).
 * `override` es el ajuste por caja: { fondo, logoVersion } (ambos opcionales), que
 * pisan el fondo general y la versión de logo para contraste. Todos los campos del
 * resultado están listos para pintar; los fondos por espacio quedan '' cuando
 * heredan (usa `fondoEspacio` para resolver la herencia en el render). */
export function resolverTema(empresa, override = {}) {
  const t = (empresa && empresa.temaPantalla) || {}
  const logoVersion = override.logoVersion || t.logoVersion || 'color'
  return {
    fondo: override.fondo || t.fondo || '',
    fondoCabecera: t.fondoCabecera || '',
    fondoProductos: t.fondoProductos || '',
    fondoTotales: t.fondoTotales || '',
    fondoPublicidad: t.fondoPublicidad || '',
    // ColorMarca es el acento previo (compat): si no hay énfasis nuevo, se respeta.
    enfasis: t.colorEnfasis || (empresa && empresa.colorMarca) || ENFASIS_DEFAULT,
    texto: t.colorTexto || TEXTO_DEFAULT,
    logoVersion,
    logo: logoDeVersion(empresa, logoVersion),
  }
}

// fondoEspacio resuelve el fondo de un ESPACIO concreto del tema resuelto: si el
// espacio tiene color propio lo usa; si no, hereda del fondo general; si ese
// también está vacío, devuelve '' (el llamador aplica su default translúcido /
// degradado navy). `key` es 'fondoCabecera'|'fondoProductos'|'fondoTotales'|
// 'fondoPublicidad'.
export function fondoEspacio(tema, key) {
  if (!tema) return ''
  return tema[key] || tema.fondo || ''
}

// estiloFondoGeneral devuelve el `style.background` del contenedor raíz: el fondo
// general si se configuró, o el degradado navy por defecto.
export function estiloFondoGeneral(tema) {
  return (tema && tema.fondo) ? tema.fondo : FONDO_DEFAULT
}
