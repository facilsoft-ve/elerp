package httpapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/domain/usuario"
)

// carpetasSubida son las carpetas (agrupación por tipo de recurso dentro del
// bucket del tenant) admitidas por el subidor genérico de imágenes. Acotar el
// conjunto mantiene el bucket ordenado y evita que el cliente escriba en rutas
// arbitrarias (además el almacén ya sanea el nombre).
var carpetasSubida = map[string]bool{
	"promociones": true, // imágenes del módulo de Promociones
	"publicidad":  true, // slides manuales de la pantalla del cliente
	"empresa":     true, // logo / banners del comercio
}

// registerArchivos monta el subidor GENÉRICO de imágenes al bucket del tenant.
// Devuelve la URL relativa servida por /api/archivos/:empresa/… (la misma que
// usan las imágenes de producto). Lo usan el editor de promociones y el editor de
// slides de la pantalla del cliente. Mutar la configuración visual de la empresa
// es de Dueña/Desarrollador.
func (s *Server) registerArchivos(r fiber.Router) {
	edit := s.requireRoles(usuario.RolDueno, usuario.RolDesarrollador)
	r.Post("/archivos/subir", edit, s.handleSubirArchivo)
}

// handleSubirArchivo recibe un multipart con el campo "archivo" y una "carpeta"
// (whitelist), lo guarda en el bucket de la empresa y devuelve { url }. La
// validación real de tipo/tamaño la hace el almacén (magic bytes + límite).
func (s *Server) handleSubirArchivo(c *fiber.Ctx) error {
	if s.archivos == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "el almacenamiento de archivos no está configurado"})
	}
	fh, err := c.FormFile("archivo")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "falta el archivo"})
	}
	carpeta := c.FormValue("carpeta")
	if !carpetasSubida[carpeta] {
		carpeta = "publicidad" // destino seguro por defecto
	}
	f, err := fh.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no se pudo leer el archivo"})
	}
	defer f.Close()

	url, err := s.archivos.Guardar(empresaIDOf(c), carpeta, f)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"url": url})
}
