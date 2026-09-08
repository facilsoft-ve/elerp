// imprimirNodo abre el contenido de un nodo del DOM en una VENTANA NUEVA y dispara
// la impresión (el navegador ofrece "Guardar como PDF"). Es más robusto que
// window.print() sobre la app: la ventana nueva contiene SOLO el comprobante, con
// sus estilos en línea, sin depender del CSS de impresión de la aplicación ni de
// que se oculte el #root. Devuelve false si el navegador bloqueó el popup (para
// que el llamador caiga a window.print() como respaldo).
export function imprimirNodo(el, titulo = 'Comprobante') {
  if (!el) return false
  const w = window.open('', '_blank', 'width=820,height=1040')
  if (!w) return false
  // Copiar los estilos de la app (Tailwind) para que el comprobante se vea idéntico
  // aunque use clases utilitarias y no solo estilos en línea. window.onload espera a
  // que esas hojas carguen antes de imprimir.
  const estilos = Array.from(document.querySelectorAll('link[rel="stylesheet"], style'))
    .map((n) => n.outerHTML)
    .join('')
  const doc =
    '<!doctype html><html><head><meta charset="utf-8"><title>' + titulo + '</title>' + estilos +
    '<style>*{box-sizing:border-box}' +
    'html,body{margin:0;background:#fff !important;color:#1F2430;font-family:Inter,system-ui,-apple-system,Segoe UI,Roboto,sans-serif;-webkit-print-color-adjust:exact;print-color-adjust:exact}' +
    '.sheet{max-width:660px;margin:0 auto;padding:28px;background:#fff}' +
    '@media print{@page{margin:14mm}}</style>' +
    '</head><body><div class="sheet">' + el.innerHTML + '</div>' +
    '<scr' + 'ipt>window.onload=function(){setTimeout(function(){window.print()},350)}</scr' + 'ipt>' +
    '</body></html>'
  w.document.write(doc)
  w.document.close()
  w.focus()
  return true
}
