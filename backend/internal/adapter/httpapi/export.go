package httpapi

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Export por-tenant (respaldo + derecho de salida). El CORE genera el archivo
// on-demand; el servicio de plataforma lo almacena/agenda/registra. El archivo
// es JSON → gzip → (si hay BACKUP_KEY) AES-256-GCM. Formato cifrado: nonce || ciphertext.

var errClaveBackup = errors.New("BACKUP_KEY inválida: se espera base64 de 32 bytes (AES-256)")

// cifrarGCM cifra con AES-256-GCM. La salida es nonce||ciphertext (el nonce va al
// frente para poder descifrar). keyB64 debe decodificar a exactamente 32 bytes.
func cifrarGCM(plain []byte, keyB64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != 32 {
		return nil, errClaveBackup
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// descifrarGCM revierte cifrarGCM (nonce||ciphertext). Se usa en pruebas y por el
// servicio de plataforma al restaurar.
func descifrarGCM(enc []byte, keyB64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != 32 {
		return nil, errClaveBackup
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(enc) < ns {
		return nil, errors.New("archivo cifrado demasiado corto")
	}
	return gcm.Open(nil, enc[:ns], enc[ns:], nil)
}

// POST /internal/tenants/:empresaId/export — volcado lógico del tenant, gzip y
// (si hay clave) cifrado, como descarga adjunta.
func (s *Server) handlePlatformExport(c *fiber.Ctx) error {
	empresaID := c.Params("empresaId")
	emp, ok := s.tenancy.Empresa(empresaID)
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tenant no encontrado"})
	}

	full := map[string]any{
		"meta": map[string]any{
			"empresaId":  empresaID,
			"generadoEl": time.Now().UTC().Format(time.RFC3339),
			"version":    1,
		},
		"empresa":  emp,
		"sedes":    s.tenancy.Sedes(empresaID),
		"miembros": s.tenancy.Miembros(empresaID),
		"datos":    s.svc.ExportarTenant(empresaID),
	}
	raw, err := json.Marshal(full)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "no se pudo serializar el export"})
	}

	// gzip
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(raw); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "no se pudo comprimir el export"})
	}
	if err := zw.Close(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "no se pudo comprimir el export"})
	}
	payload := gz.Bytes()

	// cifrado opcional (AES-256-GCM) si hay clave configurada
	cifrado := false
	if s.cfg.BackupKey != "" {
		enc, err := cifrarGCM(payload, s.cfg.BackupKey)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		payload = enc
		cifrado = true
	}

	fn := "huberp-export-" + empresaID + "-" + time.Now().UTC().Format("20060102-150405") + ".json.gz"
	if cifrado {
		fn += ".enc"
	}
	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", "attachment; filename=\""+fn+"\"")
	c.Set("X-Export-Encrypted", strconv.FormatBool(cifrado))
	return c.Send(payload)
}
