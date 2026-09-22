// Command api arranca el backend de ElERP (Go + Fiber, arquitectura
// hexagonal). Cablea los adaptadores (in-memory o MongoDB, Hubmy, HTTP) con la
// capa de aplicación. El adaptador de persistencia se elige por configuración.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/mornix/elerp/internal/adapter/almacen"
	"github.com/mornix/elerp/internal/adapter/httpapi"
	"github.com/mornix/elerp/internal/adapter/hubmy"
	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/adapter/mongo"
	"github.com/mornix/elerp/internal/adapter/session"
	"github.com/mornix/elerp/internal/adapter/tasafuente"
	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/config"
	"github.com/mornix/elerp/internal/domain/authn"
)

func main() {
	cfg := config.Load()

	var svc *application.Service
	var tenancy *application.TenancyService
	var sessions authn.Store

	if cfg.Persistent() {
		db, err := mongo.Connect(cfg.MongoURI, cfg.MongoDB)
		if err != nil {
			log.Fatalf("MongoDB: %v", err)
		}
		mongo.Seed(db) // idempotente
		st := mongo.New(db)
		svc = application.New(st.Productos, st.Movimientos, st.Transferencias, st.Rubros, st.Audit, st.Documentos, st.Numerador, st.CuentasCobro, st.MetodosPago, st.Clientes, st.Cotizaciones, st.Cajas, st.Cajeros, st.SesionesCaja, st.Tasas, st.Empresas, st.VentasEnEspera, st.Cobros, st.CuentasContables, st.Asientos, st.Periodos, st.Proveedores, st.OrdenesCompra, st.CierresZ, st.FacturasCompra, st.PagosProveedor, st.Retenciones, st.Dispositivos)
		svc.ConListasPrecio(st.ListasPrecio)
		svc.ConCupones(st.Cupones)
		svc.ConPromociones(st.Promociones)
		svc.ConUnidades(st.Unidades)
		svc.ConPlantillas(st.Plantillas)
		svc.ConMesas(st.Mesas, st.Planos)
		svc.ConAsignacionMesas(st.Asignaciones, st.ConfigSalon)
		svc.ConImpresoras(st.Impresoras)
		svc.ConCuentas(st.Cuentas)
		svc.ConReservas(st.Reservas)
		svc.ConMesoneros(st.Mesoneros, st.Turnos)
		svc.ConAlicuotas(st.Alicuotas)
		svc.ConConceptosISLR(st.ConceptosISLR)
		svc.ConUnidadesTributarias(st.UnidadesTributarias)
		svc.ConCostosEnDestino(st.CostosEnDestino)
		svc.ConUbicaciones(st.Ubicaciones)
		svc.ConTiposOperacion(st.TiposOperacion)
		svc.ConApartados(st.Apartados)
		svc.ConHorarios(st.Horarios)
		svc.ConSedes(st.Sedes)
		svc.ConSeries(st.Series)
		svc.ConFacturacionDigital(st.ConfigDigital, st.EmisionesDigitales)
		svc.ConPedidos(st.Pedidos, st.CanalesPedido, st.ZonasPedido, st.Repartidores)
		svc.ConLiquidaciones(st.LiquidacionesPedido)
		svc.ConAlmacenes(st.Almacenes)
		svc.ConModulos(st.Modulos)
		svc.ConLegal(st.Legal)
		svc.ConNotasCompra(st.NotasCompra)
		svc.ConSolicitudesCompra(st.Solicitudes)
		// COPIA EFÍMERA DE LA DEMO: solo con persistencia. En memoria cada
		// arranque ya empieza limpio, así que la demo compartida es inofensiva.
		svc.ConCopiaDemo(mongo.NuevaCopiaDemo(db))
		tenancy = application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)
		sessions = mongo.NewSessionStore(db)
		log.Printf("Persistencia: MongoDB (%s)", cfg.MongoDB)
	} else {
		st := inmem.New()
		svc = application.New(st.Productos, st.Movimientos, st.Transferencias, st.Rubros, st.Audit, st.Documentos, st.Numerador, st.CuentasCobro, st.MetodosPago, st.Clientes, st.Cotizaciones, st.Cajas, st.Cajeros, st.SesionesCaja, st.Tasas, st.Empresas, st.VentasEnEspera, st.Cobros, st.CuentasContables, st.Asientos, st.Periodos, st.Proveedores, st.OrdenesCompra, st.CierresZ, st.FacturasCompra, st.PagosProveedor, st.Retenciones, st.Dispositivos)
		svc.ConListasPrecio(st.ListasPrecio)
		svc.ConCupones(st.Cupones)
		svc.ConPromociones(st.Promociones)
		svc.ConUnidades(st.Unidades)
		svc.ConPlantillas(st.Plantillas)
		svc.ConMesas(st.Mesas, st.Planos)
		svc.ConAsignacionMesas(st.Asignaciones, st.ConfigSalon)
		svc.ConImpresoras(st.Impresoras)
		svc.ConCuentas(st.Cuentas)
		svc.ConReservas(st.Reservas)
		svc.ConMesoneros(st.Mesoneros, st.Turnos)
		svc.ConAlicuotas(st.Alicuotas)
		svc.ConConceptosISLR(st.ConceptosISLR)
		svc.ConUnidadesTributarias(st.UnidadesTributarias)
		svc.ConCostosEnDestino(st.CostosEnDestino)
		svc.ConUbicaciones(st.Ubicaciones)
		svc.ConTiposOperacion(st.TiposOperacion)
		svc.ConApartados(st.Apartados)
		svc.ConHorarios(st.Horarios)
		svc.ConSedes(st.Sedes)
		svc.ConSeries(st.Series)
		svc.ConFacturacionDigital(st.ConfigDigital, st.EmisionesDigitales)
		svc.ConPedidos(st.Pedidos, st.CanalesPedido, st.ZonasPedido, st.Repartidores)
		svc.ConLiquidaciones(st.LiquidacionesPedido)
		svc.ConAlmacenes(st.Almacenes)
		svc.ConModulos(st.Modulos)
		svc.ConLegal(st.Legal)
		// Notas de crédito/débito de proveedor. El Store in-memory (seed.go) aún no
		// expone el repo; se construye aquí para no depender de ese cableado. Cuando
		// el seed lo agregue (st.NotasCompra), sustituir por ese campo para compartir
		// el mismo repo sembrado.
		svc.ConNotasCompra(inmem.NewNotaCompraRepo())
		svc.ConSolicitudesCompra(st.Solicitudes)
		tenancy = application.NewTenancy(st.Organizaciones, st.Empresas, st.Sedes, st.Usuarios, st.Membresias, st.Credenciales, st.Audit)
		sessions = session.NewMemory()
		log.Println("Persistencia: in-memory (efímera)")
	}

	// Recontabilizado: los documentos que existían antes de que hubiera libro
	// diario (los del seed y los de cualquier empresa que ya operaba) reciben su
	// asiento una sola vez. Es idempotente: si ya lo tienen, se salta.
	for _, emp := range tenancy.TodasLasEmpresas() {
		if n := svc.RecontabilizarPendientes(emp.ID, "sistema"); n > 0 {
			log.Printf("Contabilidad: %d documento(s) de %s recibieron su asiento", n, emp.ID)
		}
		// Almacenes: toda sede debe tener ≥1 almacén principal. Idempotente: crea el
		// "Almacén Principal" a las sedes que aún no tienen ninguno (seed y tenants
		// previos a esta función).
		for _, sd := range tenancy.Sedes(emp.ID) {
			if _, err := svc.AsegurarAlmacenPrincipal(emp.ID, sd.ID, "sistema", "sistema"); err != nil {
				log.Printf("Almacenes: no se pudo asegurar el principal de %s/%s: %v", emp.ID, sd.ID, err)
			}
		}
	}

	// Almacén de archivos: un bucket por cliente bajo ARCHIVOS_DIR.
	arch, err := almacen.New(os.Getenv("ARCHIVOS_DIR"), "/api/archivos")
	if err != nil {
		log.Fatalf("almacén de archivos: %v", err)
	}

	// Tasa de cambio (R9): fuentes externas en orden de preferencia y lazo
	// diario. Solo tráfico saliente; si nada responde, se opera con la última
	// tasa buena y la interfaz dice de cuándo es.
	if cfg.TasaAuto {
		svc.ConFuentesDeTasa(
			tasafuente.NewBCV(cfg.TasaURLBCV),
			tasafuente.NewRespaldo(cfg.TasaURLRespaldo),
		)
		go lazoDeTasa(svc)
	} else {
		log.Println("Tasa de cambio: obtención automática APAGADA (TASA_AUTO=false); solo carga manual.")
	}

	// Lazo de la IMPRENTA DIGITAL: envía lo encolado y busca los números de
	// control. Corre siempre que el módulo esté cableado; si ninguna empresa lo
	// tiene activo, cada vuelta no hace nada y no cuesta nada.
	go lazoFacturacionDigital(svc)
	// Limpieza de las copias de demostración vencidas. Que sea automática es
	// parte del diseño: una demo que hay que acordarse de borrar termina siendo
	// una base con doscientas copias abandonadas.
	go lazoDemoEfimera(svc)

	hb := hubmy.New(cfg.HubmyAPIBase, cfg.HubmyAPIKey)
	// Capa IA del asistente (opt-in por empresa): solo se cablea si Hubmy está
	// configurado. Sin proxy, el asistente responde solo con la capa mecánica (local).
	if cfg.HubmyConfigured() {
		svc.ConIA(hubmy.NewIAProxy(hb))
	}
	app := httpapi.NewServer(cfg, svc, tenancy, hb, sessions, arch)

	if !cfg.HubmyConfigured() {
		log.Println("⚠️  Hubmy sin configurar (HUBMY_APP_ID / HUBMY_API_KEY vacíos): login SSO e IA devolverán 503.")
	}
	log.Printf("ElERP API escuchando en :%s (callback: %s)", cfg.Port, cfg.CallbackURL())
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

/* lazoFacturacionDigital atiende el outbox de la imprenta digital.
 *
 * Despierta cada 20 segundos porque el número de control tarda entre uno y cinco
 * minutos y el cliente está mirando la página pública: revisar cada varios
 * minutos haría que la factura «tarde» mucho más de lo que realmente tarda.
 *
 * Procesa de a POCAS y de a UNA: la imprenta responde 429 cuando la petición
 * anterior de la misma cuenta sigue en curso, así que el paralelismo no acelera
 * — hace fallar envíos. Cada emisión lleva su propia espera creciente, de modo
 * que una que falla no bloquea a las demás.
 */
func lazoFacturacionDigital(svc *application.Service) {
	for {
		func() {
			// Contexto acotado por vuelta: una llamada colgada no puede dejar el
			// lazo detenido para siempre.
			ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancelar()
			defer func() {
				// Un pánico en la integración no puede tumbar el proceso que está
				// cobrando en las cajas.
				if r := recover(); r != nil {
					log.Printf("facturación digital: pánico en el lazo: %v", r)
				}
			}()
			svc.ProcesarEmisiones(ctx, 20)
		}()
		time.Sleep(20 * time.Second)
	}
}

// lazoDemoEfimera barre las copias de demostración vencidas. Despierta cada
// media hora: las copias viven tres horas, así que nada se acumula más de eso.
func lazoDemoEfimera(svc *application.Service) {
	for {
		time.Sleep(30 * time.Minute)
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("demo efímera: pánico en la limpieza: %v", r)
				}
			}()
			svc.BarrerDemosVencidas()
		}()
	}
}

// lazoDeTasa consulta la tasa del día. Despierta cada 30 minutos y el servicio
// decide si toca consultar: como máximo una o dos veces al día por instancia
// (condición 7 de R9), nunca una por tenant ni una por petición. Un fallo no
// interrumpe nada: la caja sigue operando con la última tasa conocida.
func lazoDeTasa(svc *application.Service) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		svc.SincronizarTasaDiaria(ctx)
		cancel()
		time.Sleep(30 * time.Minute)
	}
}
