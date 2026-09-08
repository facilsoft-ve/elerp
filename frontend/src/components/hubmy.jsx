import { useState } from 'react'

/* Botón «Iniciar sesión con Hubmy».
 *
 * Respeta la línea gráfica que entrega Hubmy para su botón de identidad, no la de
 * ElERP: fondo blanco (o #1c1c1c en oscuro), borde #e2e2e2, radio de 8 px, peso
 * 600, el logo a 20 px de alto y la tipografía de sistema. Un botón de «entrar
 * con X» pertenece a la marca X — si lo pintáramos de azul ElERP dejaría de ser
 * reconocible, que es justo su función.
 *
 * Dos decisiones propias:
 *
 *  1. **No apunta a `hubmy.app/sdk/authorize` directo.** Llama a nuestro
 *     `/api/auth/login`, que es el que arma la URL de autorización con el
 *     `app_id` del entorno y un `state` aleatorio, y el que valida después el
 *     token en el callback. Con el enlace directo el `app_id` quedaría escrito en
 *     el bundle y perderíamos el `state` (la defensa contra CSRF del flujo OIDC).
 *
 *  2. **El logo se sirve desde nuestro propio origen.** `hubmy.app/logo-hubmy.png`
 *     no cargaba en el navegador, y además una imagen remota en la pantalla de
 *     login se rompe justo cuando no hay internet — que en Venezuela es la mitad
 *     del tiempo. El archivo local (`public/hubmy-logo.png`) es la marca «HUBMY»
 *     con fondo transparente, extraída de la referencia que entregó el cliente.
 *     Si Hubmy publica su asset oficial (idealmente un SVG), se reemplaza ese
 *     archivo y no hay que tocar nada más.
 *
 *     Aun así se conserva el respaldo: si la imagen no cargara, el botón se queda
 *     con su texto en vez de mostrar el icono roto.
 */
const LOGO_HUBMY = '/hubmy-logo.png'

// La tipografía es la que especifica Hubmy para su botón.
const FUENTE_HUBMY = 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'

export function BotonHubmy({ onClick, className = '', etiqueta = 'Iniciar sesión con Hubmy' }) {
  const [sinLogo, setSinLogo] = useState(false)

  return (
    <button type="button" onClick={onClick} style={{ fontFamily: FUENTE_HUBMY }}
      className={`w-full inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg
        text-[14.5px] font-semibold transition-colors ring-focus
        bg-white text-[#1c1c1c] border border-[#e2e2e2] hover:bg-[#f7f7f7] active:bg-[#f0f0f0]
        dark:bg-[#1c1c1c] dark:text-white dark:border-[#2a2a2a] dark:hover:bg-[#242424]
        ${className}`}>
      {!sinLogo ? (
        <img src={LOGO_HUBMY} alt="Hubmy" height="20" onError={() => setSinLogo(true)}
          className="h-5 w-auto block dark:[filter:brightness(0)_invert(1)]" />
      ) : null}
      {etiqueta}
    </button>
  )
}
