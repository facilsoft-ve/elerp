package httpapi

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"rsc.io/qr"
)

/* GENERACIÓN DEL QR.
 *
 * Nivel de corrección M (~15 %): el código va impreso en papel térmico, que se
 * borra con el calor y se arruga en un bolsillo. El nivel L ahorra módulos pero
 * un ticket maltratado deja de leerse, y entonces el cliente no llega a su
 * factura — que es justo lo único que el QR tiene que garantizar.
 */
func qrDe(texto string) ([]byte, error) {
	codigo, err := qr.Encode(texto, qr.M)
	if err != nil {
		return nil, err
	}
	// Se dibuja a mano en vez de usar el PNG de la librería para controlar dos
	// cosas que importan al imprimir: la escala (módulos de 8 px, legibles por
	// cualquier teléfono) y el MARGEN BLANCO. Sin margen, un lector no distingue
	// dónde empieza el código y muchos ni lo intentan.
	const escala, margen = 8, 4
	lado := codigo.Size
	px := (lado + margen*2) * escala
	img := image.NewGray(image.Rect(0, 0, px, px))
	for i := range img.Pix {
		img.Pix[i] = 0xFF
	}
	negro := color.Gray{Y: 0}
	for y := 0; y < lado; y++ {
		for x := 0; x < lado; x++ {
			if !codigo.Black(x, y) {
				continue
			}
			for dy := 0; dy < escala; dy++ {
				for dx := 0; dx < escala; dx++ {
					img.Set((x+margen)*escala+dx, (y+margen)*escala+dy, negro)
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
