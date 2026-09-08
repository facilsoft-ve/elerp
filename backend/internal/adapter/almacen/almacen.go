// Package almacen guarda los archivos que suben los usuarios — hoy imágenes de
// producto — con un BUCKET POR CLIENTE.
//
// «Bucket» aquí es un prefijo de ruta por tenant: cada empresa escribe y lee
// solo dentro de `<raíz>/<empresaID>/…`. La separación es física, no un filtro
// de consulta, así que un error de autorización en la aplicación no puede
// devolver el archivo de otra empresa: la ruta simplemente no existe en su
// bucket.
//
// La implementación es de disco local con una interfaz deliberadamente estrecha
// (Guardar / Borrar / URL). Migrar a S3-compatible es cambiar este adaptador sin
// tocar el dominio ni la aplicación — el SAD ya prevé object storage.
package almacen

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Errores del almacén.
var (
	ErrTipoNoPermitido = errors.New("solo se admiten imágenes JPG, PNG o WebP")
	ErrDemasiadoGrande = errors.New("la imagen supera el tamaño máximo")
	ErrTenantVacio     = errors.New("falta la empresa: no se puede resolver el bucket")
)

// TamMaximo es el límite por archivo. Una foto de producto tomada con teléfono
// entra de sobra; el límite existe para que un archivo enorme no llene el disco.
const TamMaximo = 4 << 20 // 4 MiB

// tiposPermitidos mapea el content-type declarado a su extensión. Se valida
// además el contenido real (magic bytes) antes de escribir.
var tiposPermitidos = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// Almacen escribe en disco bajo una raíz, un bucket por empresa.
type Almacen struct {
	raiz string
	// base es el prefijo público con el que se sirven los archivos, p. ej.
	// "/api/archivos". La URL guardada en el dominio es relativa a propósito:
	// así el mismo dato sirve en dev, en el dominio público y detrás de un
	// proxy, sin reescrituras.
	base string
}

// New crea el almacén y asegura la raíz.
func New(raiz, base string) (*Almacen, error) {
	if raiz == "" {
		raiz = "/data/archivos"
	}
	if base == "" {
		base = "/api/archivos"
	}
	if err := os.MkdirAll(raiz, 0o750); err != nil {
		return nil, fmt.Errorf("almacen: no se pudo crear la raíz %q: %w", raiz, err)
	}
	return &Almacen{raiz: raiz, base: strings.TrimRight(base, "/")}, nil
}

// Raiz expone la carpeta base (la usa el servidor para servir los archivos).
func (a *Almacen) Raiz() string { return a.raiz }

// bucket resuelve la carpeta de una empresa, rechazando cualquier intento de
// salirse de ella (path traversal) antes de tocar el disco.
func (a *Almacen) bucket(empresaID string) (string, error) {
	if empresaID == "" {
		return "", ErrTenantVacio
	}
	limpio := filepath.Base(filepath.Clean("/" + empresaID))
	if limpio == "." || limpio == "/" || limpio != empresaID {
		return "", fmt.Errorf("almacen: identificador de empresa inválido")
	}
	dir := filepath.Join(a.raiz, limpio)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	return dir, nil
}

// detectarTipo mira los primeros bytes reales del archivo. No se confía en el
// content-type que declara el cliente: es trivial de falsificar.
func detectarTipo(cab []byte) string {
	switch {
	case len(cab) >= 3 && cab[0] == 0xFF && cab[1] == 0xD8 && cab[2] == 0xFF:
		return "image/jpeg"
	case len(cab) >= 8 && string(cab[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(cab) >= 12 && string(cab[:4]) == "RIFF" && string(cab[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}

// Guardar escribe el archivo en el bucket de la empresa y devuelve la URL
// relativa con la que el frontend lo pide. `carpeta` agrupa por tipo de recurso
// (p. ej. "productos").
func (a *Almacen) Guardar(empresaID, carpeta string, r io.Reader) (string, error) {
	dir, err := a.bucket(empresaID)
	if err != nil {
		return "", err
	}
	carpeta = filepath.Base(filepath.Clean("/" + carpeta))
	destDir := filepath.Join(dir, carpeta)
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", err
	}

	// Se lee con un byte extra para distinguir "justo en el límite" de "se pasó".
	datos, err := io.ReadAll(io.LimitReader(r, TamMaximo+1))
	if err != nil {
		return "", err
	}
	if len(datos) > TamMaximo {
		return "", ErrDemasiadoGrande
	}
	tipo := detectarTipo(datos)
	ext, ok := tiposPermitidos[tipo]
	if !ok {
		return "", ErrTipoNoPermitido
	}

	nombre := aleatorio() + ext
	if err := os.WriteFile(filepath.Join(destDir, nombre), datos, 0o640); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%s/%s", a.base, empresaID, carpeta, nombre), nil
}

// Borrar elimina un archivo del bucket de la empresa. Ignora el archivo ausente:
// borrar dos veces no es un error para quien llama.
func (a *Almacen) Borrar(empresaID, url string) error {
	dir, err := a.bucket(empresaID)
	if err != nil {
		return err
	}
	prefijo := a.base + "/" + empresaID + "/"
	if !strings.HasPrefix(url, prefijo) {
		// La URL no pertenece a este bucket: no se toca nada.
		return nil
	}
	rel := filepath.Clean("/" + strings.TrimPrefix(url, prefijo))
	ruta := filepath.Join(dir, rel)
	// Cinturón y tirantes: la ruta final tiene que seguir dentro del bucket.
	if !strings.HasPrefix(ruta, dir+string(os.PathSeparator)) {
		return fmt.Errorf("almacen: ruta fuera del bucket")
	}
	if err := os.Remove(ruta); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func aleatorio() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// Sin aleatoriedad no se inventa un nombre predecible: mejor fallar arriba.
		return ""
	}
	return hex.EncodeToString(b)
}
