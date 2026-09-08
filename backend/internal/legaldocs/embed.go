// Package legaldocs incrusta los documentos legales que la app sirve y que el usuario
// acepta. El HASH que se registra en la aceptación se computa de ESTE contenido, de modo
// que la prueba fija exactamente lo aceptado. Los .md son copias sincronizadas de
// Documentos/legal-*.md (fuente humana); esta es la versión autoritativa que se sirve.
package legaldocs

import _ "embed"

//go:embed terminos.md
var Terminos string

//go:embed privacidad.md
var Privacidad string
