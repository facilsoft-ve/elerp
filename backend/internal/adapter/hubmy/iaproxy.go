package hubmy

import "context"

// IAProxyAdapter adapta el Client de Hubmy al puerto application.IAProxy: arma el par
// de mensajes [{system},{user}] y delega en AIChat (proxy de IA de Hubmy). Cumple el
// contrato de forma ESTRUCTURAL (mismo método), así el paquete hubmy no depende de la
// capa de aplicación. Se cablea en cmd/api con svc.ConIA(hubmy.NewIAProxy(hb)).
type IAProxyAdapter struct{ c *Client }

// NewIAProxy envuelve un Client como proxy de IA de la aplicación.
func NewIAProxy(c *Client) *IAProxyAdapter { return &IAProxyAdapter{c: c} }

// Chat manda un system prompt (que ya incluye el contexto acotado al rol) y el mensaje
// del usuario al modelo, y devuelve la respuesta en texto.
func (a *IAProxyAdapter) Chat(ctx context.Context, modelo, system, usuario string) (string, error) {
	return a.c.AIChat(ctx, modelo, []ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: usuario},
	})
}
