// Command server es el BFF de la consola de super usuario de ElERP (plataforma Mornix).
// Autentica operadores (contraseña + TOTP) con su propia base de datos y delega la
// gestión de clientes en el core ERP por /internal/*. Se separa a huberp-platform sin
// cambios: no importa ningún paquete del core.
package main

import (
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mornix/elerp-platform/internal/billing"
	"github.com/mornix/elerp-platform/internal/config"
	"github.com/mornix/elerp-platform/internal/core"
	"github.com/mornix/elerp-platform/internal/facturacion"
	"github.com/mornix/elerp-platform/internal/httpapi"
	"github.com/mornix/elerp-platform/internal/hubmy"
	"github.com/mornix/elerp-platform/internal/operador"
	"github.com/mornix/elerp-platform/internal/store"
)

func main() {
	cfg := config.Load()

	var ops operador.Repository
	var sess operador.SessionStore
	var planes facturacion.PlanRepo
	var subs facturacion.SuscripcionRepo
	if cfg.Persistent() {
		db, err := store.Conectar(cfg.MongoURI, cfg.MongoDB)
		if err != nil {
			log.Fatalf("mongo (consola): %v", err)
		}
		ops = store.NewMongoOperadores(db)
		sess = store.NewMongoSesiones(db)
		planes = store.NewMongoPlanes(db)
		subs = store.NewMongoSuscripciones(db)
		log.Printf("Consola: persistencia Mongo (db %s)", cfg.MongoDB)
	} else {
		ops = store.NewMemOperadores()
		sess = store.NewMemSesiones()
		planes = store.NewMemPlanes()
		subs = store.NewMemSuscripciones()
		log.Println("Consola: persistencia EN MEMORIA (sin PLATFORM_MONGO_URI)")
	}

	bootstrapOperador(cfg, ops)

	if cfg.PlatformAPIKey == "" {
		log.Println("⚠️  PLATFORM_API_KEY vacía: el core devolverá 503 en /internal/*. La consola no podrá operar hasta configurarla (compartida con el core).")
	}

	cli := core.New(cfg.CoreAPIBase, cfg.PlatformAPIKey)

	// Billing: cliente Hubmy opt-in (checkout de suscripciones). Sin HUBMY_API_KEY el
	// billing es 100% local (cobro manual).
	var hub *hubmy.Client
	if cfg.HubmyConfigured() {
		hub = hubmy.New(cfg.HubmyAPIBase, cfg.HubmyAPIKey)
		log.Println("Billing: checkout de Hubmy HABILITADO.")
	} else {
		log.Println("Billing: local (sin HUBMY_API_KEY; cobro manual).")
	}
	bill := billing.New(planes, subs, cli, hub)

	// Datos demo de billing (solo dev/preview): catálogo de planes + una suscripción de la
	// organización demo, para que la pantalla Facturación no salga vacía.
	if cfg.DevLogin {
		bootstrapBillingDemo(planes, subs)
	}

	app := httpapi.NewServer(cfg, ops, sess, cli, bill)

	log.Printf("Consola de plataforma escuchando en :%s (core: %s)", cfg.Port, cfg.CoreAPIBase)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

// bootstrapOperador crea el PRIMER operador si no existe, para poder entrar la primera
// vez. Dos fuentes: credenciales de bootstrap por entorno (producción) o el operador
// demo de desarrollo. En ambos casos la MFA queda por enrolar en el primer login.
func bootstrapOperador(cfg config.Config, ops operador.Repository) {
	crear := func(email, password, nombre, motivo string) {
		if email == "" || password == "" {
			return
		}
		if _, existe := ops.ByEmail(email); existe {
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("bootstrap operador: no se pudo hashear la contraseña: %v", err)
			return
		}
		ops.Create(operador.Operador{
			Email: email, Nombre: nombre, Hash: string(hash),
			MFAConfigurada: false, Creado: time.Now().UTC().Format(time.RFC3339),
		})
		log.Printf("Operador %s creado (%s). Debe enrolar su segundo factor (MFA) en el primer login.", email, motivo)
	}

	if cfg.HasBootstrap() {
		crear(cfg.BootstrapEmail, cfg.BootstrapPassword, cfg.BootstrapNombre, "bootstrap")
	}
	// Operador demo SOLO en desarrollo/preview (nunca en producción).
	if cfg.DevLogin && ops.Count() == 0 {
		crear("admin@mornix.tech", "mornix-demo-2026", "Operador Demo", "dev-login")
		log.Println("DEV_LOGIN: operador demo → admin@mornix.tech / mornix-demo-2026 (enrola MFA al entrar).")
	}
}

// bootstrapBillingDemo siembra un catálogo de planes y una suscripción de la organización
// demo (org_demo del core), para que Facturación muestre datos en la demo. Idempotente.
func bootstrapBillingDemo(planes facturacion.PlanRepo, subs facturacion.SuscripcionRepo) {
	if len(planes.List()) > 0 {
		return
	}
	lim := func(f, u, s int) facturacion.Limites { return facturacion.Limites{FacturasMes: f, Usuarios: u, Sucursales: s} }
	emprendedor := planes.Create(facturacion.Plan{Nombre: "Emprendedor", Descripcion: "Para empezar: una sede, lo esencial.", PrecioCents: 1500, Moneda: "USD", Intervalo: facturacion.IntervaloMensual, Limites: lim(200, 3, 1), Activo: true})
	pro := planes.Create(facturacion.Plan{Nombre: "Pro", Descripcion: "Multi-sede, contabilidad y tesorería completas.", PrecioCents: 3900, Moneda: "USD", Intervalo: facturacion.IntervaloMensual, Limites: lim(2000, 10, 5), Activo: true})
	planes.Create(facturacion.Plan{Nombre: "Empresa", Descripcion: "Sin límites, soporte prioritario.", PrecioCents: 9900, Moneda: "USD", Intervalo: facturacion.IntervaloMensual, Limites: lim(0, 0, 0), Activo: true})
	_ = emprendedor
	// La organización demo del core queda suscrita al plan Pro (activa).
	subs.Upsert(facturacion.Suscripcion{
		OrgID: "org_demo", PlanID: pro.ID, Estado: facturacion.EstadoActiva,
		Inicio: "2026-08-01T00:00:00Z", ProximoCobro: "2026-09-01T00:00:00Z",
	})
	log.Println("DEV_LOGIN: billing demo sembrado (3 planes + suscripción Pro de org_demo).")
}
