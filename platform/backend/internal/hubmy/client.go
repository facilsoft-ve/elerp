// Package hubmy es el cliente OPT-IN de billing de Hubmy (packages → checkout de Stripe).
// Solo se usa si la consola tiene HUBMY_API_KEY configurada; si no, el billing es local
// (cobro manual). Docs: https://apidev.hubmy.app · GET/POST /v1/api/{packages,sales}.
package hubmy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Client habla con la API de Hubmy con la app API key (Bearer).
type Client struct {
	base string
	key  string
	http *http.Client
}

// New crea el cliente. key vacío ⇒ no configurado (Configurado() == false).
func New(base, key string) *Client {
	return &Client{base: base, key: key, http: &http.Client{Timeout: 20 * time.Second}}
}

// Configurado indica si hay API key (si no, el billing con Hubmy está deshabilitado).
func (c *Client) Configurado() bool { return c != nil && c.key != "" }

// Venta es una venta/checkout de Hubmy.
type Venta struct {
	ID          string `json:"id"`
	Status      string `json:"status"` // pending | paid | refunded | disputed
	CheckoutURL string `json:"checkout_url"`
}

// Paquete es un package de Hubmy (aplanado package+version).
type Paquete struct {
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	Estado      string `json:"estado"`
	PrecioCents int    `json:"precioCents"`
	Moneda      string `json:"moneda"`
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

func (c *Client) do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hubmy: %w", err)
	}
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("hubmy: respuesta ilegible")
	}
	if env.Error != "" {
		return nil, errors.New("hubmy: " + env.Error)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("hubmy: HTTP %d", resp.StatusCode)
	}
	return env.Data, nil
}

// CrearVenta genera un checkout para un usuario Hubmy contra un package.
func (c *Client) CrearVenta(ctx context.Context, packageID, userID string) (Venta, error) {
	data, err := c.do(ctx, http.MethodPost, "/v1/api/sales", map[string]string{"package_id": packageID, "user_id": userID})
	if err != nil {
		return Venta{}, err
	}
	var v Venta
	if err := json.Unmarshal(data, &v); err != nil {
		return Venta{}, fmt.Errorf("hubmy: venta ilegible")
	}
	return v, nil
}

// Venta consulta el estado de una venta (para sincronizar el pago).
func (c *Client) Venta(ctx context.Context, saleID string) (Venta, error) {
	data, err := c.do(ctx, http.MethodGet, "/v1/api/sales/"+saleID, nil)
	if err != nil {
		return Venta{}, err
	}
	var v Venta
	if err := json.Unmarshal(data, &v); err != nil {
		return Venta{}, fmt.Errorf("hubmy: venta ilegible")
	}
	return v, nil
}

// ListarPaquetes devuelve los packages de la app (aplana package+version).
func (c *Client) ListarPaquetes(ctx context.Context) ([]Paquete, error) {
	data, err := c.do(ctx, http.MethodGet, "/v1/api/packages", nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Items []struct {
			Package struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"package"`
			Version struct {
				Name       string `json:"name"`
				PriceCents int    `json:"price_cents"`
				Currency   string `json:"currency"`
			} `json:"version"`
		} `json:"items"`
	}
	// La lista puede venir como {data:{items:[]}} o {items:[]}; se intenta con data primero.
	if json.Unmarshal(data, &raw) != nil || len(raw.Items) == 0 {
		_ = json.Unmarshal(data, &raw)
	}
	out := make([]Paquete, 0, len(raw.Items))
	for _, it := range raw.Items {
		out = append(out, Paquete{ID: it.Package.ID, Estado: it.Package.Status, Nombre: it.Version.Name, PrecioCents: it.Version.PriceCents, Moneda: it.Version.Currency})
	}
	return out, nil
}
