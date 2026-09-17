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
 *
 *  3. **El botón es CLARO también en modo oscuro.** La versión oscura teñía el
 *     logo entero de blanco (`brightness(0) invert(1)`), y con eso se perdía lo
 *     único que hace reconocible la marca: el degradado magenta→cian de «MY».
 *     Un botón de «entrar con X» existe para que se reconozca a X de un vistazo;
 *     si hay que elegir entre combinar con el tema y conservar la marca, gana la
 *     marca. Es lo mismo que hacen los botones de Google y Apple, que también se
 *     quedan claros sobre fondo oscuro.
 */
/* La ruta va con BASE_URL y no con «/hubmy-logo.png» a secas: la app se sirve
 * bajo /app, y en la raíz del dominio vive el SITIO público. Con la ruta
 * absoluta el navegador pedía elerp.tech/hubmy-logo.png, recibía el HTML de la
 * landing con un 200 —no un 404, que se habría notado antes—, no podía
 * decodificarlo como imagen y el botón se quedaba sin logo. */
const LOGO_HUBMY = `${import.meta.env.BASE_URL}hubmy-logo.png`

// La tipografía es la que especifica Hubmy para su botón.
const FUENTE_HUBMY = 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'

export function BotonHubmy({ onClick, className = '', etiqueta = 'Iniciar sesión con Hubmy' }) {
  const [sinLogo, setSinLogo] = useState(false)

  return (
    <button type="button" onClick={onClick} style={{ fontFamily: FUENTE_HUBMY }}
      className={`w-full inline-flex items-center justify-center gap-3 px-4 py-3 rounded-xl
        text-[15px] font-semibold transition-colors ring-focus shadow-sm
        bg-white text-[#141414] border border-[#e6e6e6] hover:bg-[#f7f7f7] active:bg-[#f0f0f0]
        ${className}`}>
      {!sinLogo ? (
        // El logo va a 22 px: el mismo peso visual que el texto que lo acompaña,
        // que es como lo muestra la referencia de marca.
        <img src={LOGO_HUBMY} alt="Hubmy" height="22" onError={() => setSinLogo(true)}
          className="h-[22px] w-auto block" />
      ) : null}
      {etiqueta}
    </button>
  )
}
