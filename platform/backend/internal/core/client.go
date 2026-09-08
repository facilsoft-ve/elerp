// Package core es el cliente HTTP del BFF hacia la superficie interna del ERP
// (/internal/*). Inyecta la clave M2M (X-Platform-Key) —que nunca ve el navegador— y el
// actor (X-Platform-Actor = email del operador) para la auditoría del core. Relaya las
// respuestas de forma genérica: el BFF no re-modela los structs del core, solo autentica.
package core

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

// Client habla con el core ERP.
type Client struct {
	base string
	key  string
	http *http.Client
}

// New crea el cliente. base = CORE_API_BASE (sin barra final); key = PLATFORM_API_KEY.
func New(base, key string) *Client {
	return &Client{base: base, key: key, http: &http.Client{Timeout: 30 * time.Second}}
}

// Resp es la respuesta cruda del core que el BFF relaya al navegador.
type Resp struct {
	Status int
	Body   []byte
	Header http.Header
}

// Forward hace una petición al core /internal/* con la clave M2M y el actor. `path` es
// la ruta interna completa (p. ej. "/internal/orgs/abc/plan"). `ct` es el Content-Type
// del cuerpo (vacío si no hay cuerpo).
func (c *Client) Forward(ctx context.Context, method, path, actor string, body []byte, ct string) (*Resp, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Platform-Key", c.key)
	if actor != "" {
		req.Header.Set("X-Platform-Actor", actor)
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Resp{Status: resp.StatusCode, Body: b, Header: resp.Header}, nil
}
