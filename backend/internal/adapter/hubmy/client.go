// Package hubmy es el cliente HTTP del SDK de Hubmy (auth SSO + AI proxy).
package hubmy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Client habla con la API de Hubmy usando la API key de la app (backend-only).
type Client struct {
	base   string
	apiKey string
	http   *http.Client
}

// New construye el cliente. base p.ej. https://apidev.hubmy.app
func New(base, apiKey string) *Client {
	return &Client{base: base, apiKey: apiKey, http: &http.Client{Timeout: 12 * time.Second}}
}

// User es el snapshot de perfil que devuelve Hubmy en /validate.
type User struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
}

// SessionInfo es la sesión Hubmy asociada al token validado.
type SessionInfo struct {
	ID        string `json:"id"`
	AppID     string `json:"app_id"`
	ExpiresAt string `json:"expires_at"`
}

type validateResponse struct {
	Message string `json:"message"`
	Error   string `json:"error"`
	Data    struct {
		User    User        `json:"user"`
		Session SessionInfo `json:"session"`
	} `json:"data"`
}

// Validate confirma un JWT de sesión contra Hubmy y devuelve el perfil.
func (c *Client) Validate(ctx context.Context, token string) (User, SessionInfo, error) {
	endpoint := fmt.Sprintf("%s/v1/sdk/auth/validate?token=%s", c.base, url.QueryEscape(token))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return User{}, SessionInfo{}, fmt.Errorf("hubmy validate: %w", err)
	}
	defer resp.Body.Close()
	var out validateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return User{}, SessionInfo{}, fmt.Errorf("hubmy validate decode: %w", err)
	}
	if out.Error != "" {
		return User{}, SessionInfo{}, fmt.Errorf("hubmy: %s — %s", out.Error, out.Message)
	}
	return out.Data.User, out.Data.Session, nil
}

// AuthorizeURL construye la URL de /authorize a la que redirige el frontend.
func (c *Client) AuthorizeURL(appID, state string) string {
	return fmt.Sprintf("%s/v1/sdk/authorize?app_id=%s&state=%s", c.base, url.QueryEscape(appID), url.QueryEscape(state))
}

// Logout termina la sesión Hubmy asociada al token.
func (c *Client) Logout(ctx context.Context, token string) error {
	b, _ := json.Marshal(map[string]any{"token": token})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/sdk/auth/logout", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ChatMessage es un mensaje en el formato OpenAI-compatible del AI Proxy.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// AIChat llama al AI Proxy de Hubmy. model vacío => default del proxy.
func (c *Client) AIChat(ctx context.Context, model string, messages []ChatMessage) (string, error) {
	reqBody := map[string]any{"model": model, "messages": messages, "temperature": 0.4, "max_tokens": 1024}
	b, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/api/ai/chat", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("hubmy ai chat: %w", err)
	}
	defer resp.Body.Close()
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("hubmy ai chat decode: %w", err)
	}
	if out.Error.Message != "" {
		return "", fmt.Errorf("hubmy ai: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("hubmy ai: respuesta vacía")
	}
	return out.Choices[0].Message.Content, nil
}
