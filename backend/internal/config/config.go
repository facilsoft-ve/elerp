// Package config carga la configuración del backend de ElERP desde variables
// de entorno, una sola vez al arranque (Load). Mismos patrones que Tesotrix.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config es la configuración inmutable del proceso.
type Config struct {
	Port        string
	AppBaseURL  string // URL pública de ESTE backend (para el callback de Hubmy)
	FrontendURL string // URL del frontend (redirección post-login + origen CORS)

	// Hubmy (SSO + IA). Vacío ⇒ login/IA devuelven 503.
	HubmyAPIBase          string
	HubmyAppID            string
	HubmyAPIKey           string // secreto — nunca se envía al frontend
	HubmyInviteTemplateID string

	// Sesión / cookie
	SessionTTL   time.Duration
	CookieName   string
	CookieSecure bool
	// CookieSameSite fija el atributo SameSite de la cookie de sesión de forma
	// explícita ("Lax" | "None" | "Strict"). Vacío ⇒ se deriva de CookieSecure
	// (Secure ⇒ None, por retrocompat con el despliegue cross-domain). En el
	// despliegue mismo-origen (nginx proxya /api y /auth) lo correcto es
	// Secure=true + SameSite=Lax: cookie solo por TLS y sin exponerla a envíos
	// cross-site que Lax ya no necesita.
	CookieSameSite string
	CookieDomain   string

	// Modo demo sin Hubmy (debe ser false en producción)
	DevLogin bool

	// PlatformAPIKey autentica (M2M) la superficie interna /internal que consume
	// la consola de plataforma (super-admin / Mornix). Vacío ⇒ /internal queda
	// CERRADO (503): seguro por defecto, no se expone sin una clave explícita.
	PlatformAPIKey string

	// BackupKey es la clave AES-256 (base64 de 32 bytes) para cifrar los exports
	// por-tenant. Vacía ⇒ el export sale comprimido pero SIN cifrar (útil en dev).
	BackupKey string

	// Persistencia. MongoURI vacío ⇒ adaptador in-memory (efímero).
	MongoURI string
	MongoDB  string

	// Tasa de cambio (R9). TasaAuto=false deja la obtención automática apagada
	// y la tasa solo se puede cargar a mano — útil en entornos sin salida a
	// internet, donde intentarlo solo llenaría el log de fallos.
	TasaAuto        bool
	TasaURLBCV      string // portada del BCV (fuente primaria, raspado)
	TasaURLRespaldo string // API JSON que republica la tasa oficial
	TasaURLMercado  string // API JSON del promedio de mercado (opcional)
}

// Load lee el entorno y aplica valores por defecto de desarrollo.
func Load() Config {
	ttlMin := envInt("SESSION_TTL_MIN", 1440)
	return Config{
		Port:        env("PORT", "8080"),
		AppBaseURL:  env("APP_BASE_URL", "http://localhost:8080"),
		FrontendURL: env("FRONTEND_URL", "http://localhost:5173"),

		HubmyAPIBase:          strings.TrimRight(env("HUBMY_API_BASE", "https://apidev.hubmy.app"), "/"),
		HubmyAppID:            env("HUBMY_APP_ID", ""),
		HubmyAPIKey:           env("HUBMY_API_KEY", ""),
		HubmyInviteTemplateID: env("HUBMY_INVITE_TEMPLATE_ID", ""),

		SessionTTL:     time.Duration(ttlMin) * time.Minute,
		CookieName:     env("SESSION_COOKIE", "elerp_session"),
		CookieSecure:   envBool("COOKIE_SECURE", false),
		CookieSameSite: env("COOKIE_SAMESITE", ""),
		CookieDomain:   env("COOKIE_DOMAIN", ""),

		// Seguro por defecto: un despliegue sin DEV_LOGIN explícito queda cerrado
		// (sin modo demo sin credenciales). La demo pública lo habilita a
		// propósito en su .env.
		DevLogin: envBool("DEV_LOGIN", false),

		PlatformAPIKey: env("PLATFORM_API_KEY", ""),
		BackupKey:      env("BACKUP_KEY", ""),

		MongoURI: env("MONGO_URI", ""),
		MongoDB:  env("MONGO_DB", "elerp"),

		TasaAuto:        envBool("TASA_AUTO", true),
		TasaURLBCV:      env("TASA_URL_BCV", "https://www.bcv.org.ve/"),
		TasaURLRespaldo: env("TASA_URL_RESPALDO", "https://pydolarve.org/api/v1/dollar?page=bcv&monitor=usd"),
		TasaURLMercado:  env("TASA_URL_MERCADO", ""),
	}
}

// Persistent indica si se debe usar MongoDB (true) o in-memory (false).
func (c Config) Persistent() bool { return c.MongoURI != "" }

// CallbackURL es la URL de callback OIDC que Hubmy debe tener registrada.
func (c Config) CallbackURL() string {
	return strings.TrimRight(c.AppBaseURL, "/") + "/auth/hubmy/callback"
}

// HubmyConfigured indica si hay credenciales de Hubmy.
func (c Config) HubmyConfigured() bool { return c.HubmyAppID != "" && c.HubmyAPIKey != "" }

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}
