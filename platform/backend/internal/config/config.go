// Package config carga la configuración del BFF de la consola de plataforma desde el
// entorno. El servicio es autocontenido (se separa a huberp-platform sin cambios): su
// único acoplamiento con el core es CoreAPIBase + PlatformAPIKey.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config es la configuración resuelta del BFF.
type Config struct {
	Port        string // puerto de escucha (PLATFORM_PORT, default 8090)
	CoreAPIBase string // URL del core ERP (CORE_API_BASE), p.ej. http://huberp-backend:8080
	// PlatformAPIKey es la clave M2M compartida con el core: el BFF la inyecta como
	// X-Platform-Key hacia /internal/*. NUNCA se expone al navegador.
	PlatformAPIKey string

	// Persistencia propia (base de datos aparte del core). Vacío ⇒ in-memory.
	MongoURI string
	MongoDB  string

	// Sesión del operador (cookie propia, distinta de la del ERP).
	CookieName     string
	CookieSecure   bool
	CookieSameSite string
	CookieDomain   string
	SessionTTL     time.Duration

	// Bootstrap del primer operador (para poder entrar la primera vez). Si el store no
	// tiene operadores y estos están seteados, se crea uno (MFA se enrola al entrar).
	BootstrapEmail    string
	BootstrapPassword string
	BootstrapNombre   string

	// WebDir sirve el SPA de la consola (estático). En dev se usa Vite y no hace falta.
	WebDir string
	// BasePath es el prefijo bajo el que se sirve el SPA (p. ej. "/consola" cuando la
	// consola vive como subruta del dominio principal detrás del proxy). Vacío = raíz.
	// Debe coincidir con el `base` del build de Vite. /papi NO lleva este prefijo.
	BasePath string

	// DevLogin siembra un operador demo (solo desarrollo/preview). NUNCA en producción.
	DevLogin bool
	// MFADeshabilitada omite el segundo factor (TOTP): el login emite sesión solo con
	// contraseña. TEMPORAL / INSEGURO — solo para entornos donde el reloj del servidor no
	// coincide con la hora real (TOTP no puede validar). Debe volver a false en producción.
	MFADeshabilitada bool

	// FrontendURL habilita CORS con credenciales para el dev server de Vite.
	FrontendURL string

	// Hubmy (billing opt-in). Con HubmyAPIKey se habilita el checkout de Hubmy para las
	// suscripciones; sin ella, el billing es 100% local (cobro manual).
	HubmyAPIBase string
	HubmyAPIKey  string
}

// Load construye la Config desde variables de entorno con defaults seguros.
func Load() Config {
	ttl := envInt("PLATFORM_SESSION_TTL_MIN", 720) // 12 h por defecto
	return Config{
		Port:              env("PLATFORM_PORT", "8090"),
		CoreAPIBase:       strings.TrimRight(env("CORE_API_BASE", "http://localhost:8080"), "/"),
		PlatformAPIKey:    env("PLATFORM_API_KEY", ""),
		MongoURI:          env("PLATFORM_MONGO_URI", ""),
		MongoDB:           env("PLATFORM_MONGO_DB", "elerp_platform"),
		CookieName:        env("PLATFORM_COOKIE", "elerp_platform_session"),
		CookieSecure:      envBool("PLATFORM_COOKIE_SECURE", false),
		CookieSameSite:    env("PLATFORM_COOKIE_SAMESITE", ""),
		CookieDomain:      env("PLATFORM_COOKIE_DOMAIN", ""),
		SessionTTL:        time.Duration(ttl) * time.Minute,
		BootstrapEmail:    strings.ToLower(strings.TrimSpace(env("PLATFORM_BOOTSTRAP_EMAIL", ""))),
		BootstrapPassword: env("PLATFORM_BOOTSTRAP_PASSWORD", ""),
		BootstrapNombre:   env("PLATFORM_BOOTSTRAP_NOMBRE", "Operador Mornix"),
		WebDir:            env("PLATFORM_WEB_DIR", "./web"),
		BasePath:          strings.TrimRight(env("PLATFORM_BASE_PATH", ""), "/"),
		DevLogin:          envBool("PLATFORM_DEV_LOGIN", false),
		MFADeshabilitada:  envBool("PLATFORM_MFA_DISABLED", false),
		FrontendURL:       env("PLATFORM_FRONTEND_URL", "http://localhost:5174"),
		HubmyAPIBase:      strings.TrimRight(env("HUBMY_API_BASE", "https://apidev.hubmy.app"), "/"),
		HubmyAPIKey:       env("HUBMY_API_KEY", ""),
	}
}

// HubmyConfigured indica si hay clave para el checkout de Hubmy (billing opt-in).
func (c Config) HubmyConfigured() bool { return c.HubmyAPIKey != "" }

// Persistent indica si hay Mongo configurado (si no, el store es in-memory).
func (c Config) Persistent() bool { return c.MongoURI != "" }

// HasBootstrap indica si hay credenciales de bootstrap del primer operador.
func (c Config) HasBootstrap() bool { return c.BootstrapEmail != "" && c.BootstrapPassword != "" }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		return v == "1" || strings.EqualFold(v, "true")
	}
	return def
}
