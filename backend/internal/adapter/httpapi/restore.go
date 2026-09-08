package httpapi

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"

	"github.com/gofiber/fiber/v2"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/sede"
)

// handlePlatformRestore restaura un respaldo (el mismo formato del export) en una EMPRESA
// NUEVA dentro de la organización destino. Nunca sobrescribe un tenant vivo. El cuerpo es
// el archivo del respaldo (gzip, cifrado si hay BACKUP_KEY); `orgId` es la organización
// donde crear la empresa restaurada.
func (s *Server) handlePlatformRestore(c *fiber.Ctx) error {
	orgID := c.Query("orgId")
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "falta orgId"})
	}
	blob := c.Body()
	if len(blob) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cuerpo vacío"})
	}

	// Auto-detección: intentar descomprimir directo; si falla y hay clave, descifrar.
	raw, err := degzip(blob)
	if err != nil && s.cfg.BackupKey != "" {
		if dec, e2 := descifrarGCM(blob, s.cfg.BackupKey); e2 == nil {
			raw, err = degzip(dec)
		}
	}
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no se pudo leer el respaldo (¿cifrado sin BACKUP_KEY, o archivo inválido?)"})
	}

	var full struct {
		Empresa  empresa.Empresa            `json:"empresa"`
		Sedes    []sede.Sede                `json:"sedes"`
		Miembros []application.MiembroView  `json:"miembros"`
		Datos    map[string]json.RawMessage `json:"datos"`
	}
	if json.Unmarshal(raw, &full) != nil || full.Empresa.RIF == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "el respaldo no tiene el formato esperado"})
	}

	emp, err := s.tenancy.CrearEmpresaRestaurada(orgID, full.Empresa, full.Sedes, full.Miembros, platformActor(c), origen(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	counts := s.svc.RestaurarDatos(emp.ID, full.Datos)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"empresaId": emp.ID, "empresa": emp, "restaurado": counts})
}

// degzip descomprime un buffer gzip.
func degzip(b []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}
