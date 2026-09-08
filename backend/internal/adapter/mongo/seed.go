package mongo

import (
	"log"
	"regexp"

	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	mesadom "github.com/mornix/elerp/internal/domain/mesa"
)

// versionSeedDemo se sube cuando los DATOS DE DEMOSTRACIÓN cambian de forma
// incompatible con lo ya sembrado —por ejemplo al pasar los precios del catálogo
// de «2,10 Bs» a los precios reales en bolívares—. Al arrancar con una versión
// vieja, los datos de negocio de la empresa demo se REGENERAN.
//
// Qué se regenera: catálogo, movimientos, documentos, transferencias, rubros,
// clientes y cuentas de cobro DE LA EMPRESA DEMO. Qué NO se toca nunca:
// organizaciones, empresas, sedes, usuarios y membresías (romperlas dejaría
// afuera a quien ya entró), ni ninguna otra empresa.
//
// Es la única parte del sistema que borra datos de negocio, y solo puede tocar el
// tenant de demostración. Si algún día hay clientes reales en esta instancia, la
// empresa demo sigue siendo un tenant aparte.
// v3: código de barras por producto y las dos ventas a crédito que hacen visibles
// las cuentas por cobrar.
// v4: se corrigieron cuatro códigos de barras duplicados del catálogo demo (dos
// productos con el mismo código hacen que un escaneo cobre el equivocado).
// v5: los movimientos sembrados llevan su costo (sin él el costo de ventas del
// libro diario quedaba en cero), y la limpieza incluye plan de cuentas y libro.
// v6: se asientan las entradas de inventario (sin ellas la cuenta 1201 salía
// negativa: las ventas la descargaban y nada la había cargado).
// v7: métodos de pago configurables por empresa (colección metodospago), que
// enlazan a las cuentas de cobro demo; se regeneran junto a estas.
// v8: cotizaciones de venta forma libre (colección cotizaciones) — dos casos
// demo (borrador y confirmada); se regeneran con el resto de datos de negocio.
// v9: la colección `contadores` se siembra ADELANTADA más allá de los folios de
// la demo (documentos FL/C y cotizaciones COT). Sin esto la primera emisión viva
// reiniciaba la correlativa en 1 y colisionaba con los números sembrados.
// v10: las cotizaciones demo llevan campos ampliados estilo Odoo (descripción y
// descuento por línea, condiciones de pago y términos); se regeneran.
// v11: módulo Compras — proveedores (colección proveedores) y órdenes de compra
// (colección ordenescompra) demo; se regeneran con el resto de datos de negocio.
// v13: la empresa demo trae anuncios de ejemplo (campo `publicidad`) para el
// carrusel de la pantalla del cliente. La empresa solo se siembra en base virgen
// (no se regenera con la limpieza), así que la bandera queda para bases nuevas.
// v14: la pantalla del cliente es configurable (`pantallaClienteModo`: mixta en la
// demo) y los slides de `publicidad` son de texto o imagen (`AnuncioSlide`, antes
// `[]string`). El decoder tolera la forma antigua; la empresa demo no se regenera,
// el equipo la re-setea aparte.
// v15: submódulo Listas de precio — tarifas por SKU (colección listasprecio) de
// venta y de compra demo; se regeneran con el resto de datos de negocio.
// v16: submódulo Solicitudes de presupuesto (RFQ) — solicitudes demo (colección
// solicitudescompra): una enviada a dos proveedores con respuestas para comparar
// y una en borrador; se regeneran con el resto de datos de negocio.
// v17: submódulo Cupones de descuento — cupones demo (colección cupones): uno
// porcentual vigente, uno de monto fijo con mínimo y uno vencido; se regeneran
// con el resto de datos de negocio.
// v18: módulo Promociones — promociones demo (colección promociones): una de
// imagen vigente, una de texto vigente y una vencida, que alimentan el carrusel
// de la pantalla del cliente; se regeneran con el resto de datos de negocio.
// v19: branding de dos niveles — la empresa demo trae `colorMarca` (teal de marca)
// como fuente del acento de la pantalla del cliente; la caja puede sobreescribir el
// contraste con `colorFondo`/`logoVersion`. La empresa demo NO se regenera con la
// limpieza (el equipo la re-setea aparte), así que la bandera queda para bases nuevas.
// v20: editor de tema de la pantalla del cliente — la empresa demo trae
// `temaPantalla` (fondo navy, énfasis teal, texto blanco, logo de color) que unifica
// la apariencia antes dispersa (empresa.colorMarca + caja.colorFondo/logoVersion). La
// empresa demo NO se regenera; la bandera queda para bases nuevas.
// v21: precios del catálogo demo SINCERADOS a valores reales de mercado — cada
// producto es su precio retail en US$ convertido a la tasa demo (× 745,63) y
// redondeado a cifra de bodega (harina 890 Bs ≈ 1,20 $, arroz 970 Bs ≈ 1,30 $…);
// costo ≈ 70% del precio. Se recalcularon en cascada las presentaciones (bulto/
// saco/six-pack con ~10% de descuento por volumen), las listas de precio
// (Mayorista, Distribuidor US$, Proveedor La Cima) y los costos de las órdenes/
// solicitudes de compra. Los productos SÍ se regeneran con limpiarDemo, así que la
// demo en vivo toma los precios nuevos.
//
// v22: venta POR PESO (balanza). Cinco productos de charcutería/granel se marcan
// con TipoVenta "peso" (QUE-KG, JAM-KG, QAM-KG, CAR-KG, TOM-KG) —precio Bs/kg,
// unidad base "kg", existencia en kg con decimales— y se añade un dispositivo
// tipo "balanza" (Aclas OS2X, puerto/protocolo) en la sede principal.
//
// v24: RIF/cédula SANEADOS al dígito verificador correcto (SENIAT módulo 11). Se
// corrigieron el RIF de la empresa demo (J-12345678-4), de los proveedores
// (J-00012345-4, J-30099887-8, J-31200011-2) y de los clientes / documentos
// fiscales de ejemplo (J-40123456-9, G-20000123-2, J-31122334-7), para que el
// backend valide el DV en el alta/edición de clientes, proveedores y empresa.
// v25: retenciones generalizadas a IVA e ISLR (modelo fiscal.Retencion). Se
// siembra un comprobante de retención de ISLR RECIBIDA (2%, compra de bienes)
// sobre la venta a crédito vigente, para poblar la pantalla de Retenciones con un
// caso de ISLR además del IVA; su asiento (Debe 1105 / Haber 1102) lo deriva el
// recontabilizado del arranque.
// v26: maestro de unidades de medida por empresa (colección unidades) — el juego
// común de Venezuela (conteo/peso/volumen/longitud) vía unidadmedida.PorDefecto,
// del que el catálogo elige su UnidadBase. Se regenera con el resto de datos de
// negocio de la demo.
const versionSeedDemo = 28

// Seed siembra la base con los datos demo (misma fuente que in-memory), de
// forma idempotente: si ya hay empresas, no hace nada.
func Seed(db *gomongo.Database) {
	st := New(db)
	semilla := inmem.New()
	snap := semilla.Snapshot()

	// Los guards cuentan SOLO el tenant demo, no la colección entera: si otra
	// empresa ya tiene productos, el conteo global no es cero y la demo se
	// quedaría sin sembrar (que es justo lo que pasó la primera vez).
	demoID := ""
	if len(snap.Empresas) > 0 {
		demoID = snap.Empresas[0].ID
	}

	// ¿Los datos de demostración quedaron viejos? Se limpian para que los vuelva
	// a sembrar la lógica de abajo, que ya es idempotente por colección.
	if demoID != "" && versionDelSeed(db) < versionSeedDemo {
		limpiarDemo(st, demoID)
		guardarVersionDelSeed(db)
	}

	// Siembra por colección, no todo-o-nada: una base ya sembrada antes de que
	// existieran las cajas sigue necesitando sus cajas y cajeros, o nadie puede
	// facturar (EmitirFactura exige turno abierto). Cada bloque se salta si su
	// colección ya tiene datos.
	if len(st.Cajas.List(demoID)) == 0 {
		for _, cj := range snap.Cajas {
			st.Cajas.c.insert(cj)
		}
		for _, cr := range snap.Cajeros {
			st.Cajeros.c.insert(cr)
		}
		log.Printf("Mongo: sembradas %d cajas y %d cajeros", len(snap.Cajas), len(snap.Cajeros))
	}

	// Documentos y transferencias: mismo criterio por colección, para que una
	// base sembrada antes de enriquecer la demo reciba los casos nuevos.
	if len(st.Documentos.List(demoID)) == 0 {
		for _, d := range snap.Documentos {
			st.Documentos.c.insert(d)
		}
		log.Printf("Mongo: sembrados %d documentos fiscales", len(snap.Documentos))
	}
	if len(st.Transferencias.List(demoID)) == 0 {
		for _, tr := range snap.Transferencias {
			st.Transferencias.c.insert(tr)
		}
	}

	// Identidad y estructura: solo en una base virgen. Recrearlas rompería las
	// membresías de usuarios reales que ya entraron.
	if st.Empresas.c.count() == 0 {
		for _, o := range snap.Orgs {
			st.Organizaciones.c.insert(o)
		}
		for _, e := range snap.Empresas {
			st.Empresas.c.insert(e)
		}
		for _, sd := range snap.Sedes {
			st.Sedes.c.insert(sd)
		}
		for _, u := range snap.Usuarios {
			st.Usuarios.c.insert(u)
		}
		for _, m := range snap.Membresias {
			st.Membresias.c.insert(m)
		}
	}

	// Usuarios de demostración que no existan todavía (vendedor, cajero,
	// contadora). Es ADITIVO: no toca a los usuarios ya presentes ni sus roles —
	// solo agrega los que faltan, para que Configuración › Usuarios y roles y el
	// Modo caja tengan con qué demostrarse.
	if demoID != "" {
		porEmail := map[string]bool{}
		for _, m := range st.Membresias.ByEmpresa(demoID) {
			porEmail[m.Email] = true
		}
		nuevos := 0
		for _, m := range snap.Membresias {
			if m.EmpresaID != demoID || porEmail[m.Email] {
				continue
			}
			for _, u := range snap.Usuarios {
				if u.Email == m.Email {
					if _, existe := st.Usuarios.ByEmail(u.Email); !existe {
						st.Usuarios.c.insert(u)
					}
				}
			}
			st.Membresias.c.insert(m)
			nuevos++
		}
		if nuevos > 0 {
			log.Printf("Mongo: agregados %d usuario(s) de demostración con su rol y sede", nuevos)
		}
	}

	// Datos de negocio de la demo, cada uno con su propio guard: así se pueden
	// vaciar las colecciones de emp_demo y recuperar el juego de pruebas
	// completo con un reinicio, sin tocar la identidad ni otras empresas.
	if len(st.Rubros.List(demoID)) == 0 {
		for _, r := range snap.Rubros {
			st.Rubros.c.insert(r)
		}
	}
	if len(st.Productos.List(demoID)) == 0 {
		for _, pr := range snap.Productos {
			st.Productos.c.insert(pr)
		}
		for _, mv := range snap.Movimientos {
			st.Movimientos.c.insert(mv)
		}
		log.Printf("Mongo: sembrados %d productos y %d movimientos", len(snap.Productos), len(snap.Movimientos))
	} else {
		// La base ya tiene catálogo, pero puede haberse sembrado antes de que la
		// demo cubriera un caso nuevo (p. ej. el producto con precio en US$ de
		// R10). Se agregan SOLO los SKU que falten, con su entrada inicial al
		// ledger: nada se sobrescribe, porque el ledger es de solo-anexado.
		existentes := map[string]bool{}
		for _, pr := range st.Productos.List(demoID) {
			existentes[pr.SKU] = true
		}
		nuevos := 0
		for _, pr := range snap.Productos {
			if existentes[pr.SKU] {
				continue
			}
			st.Productos.c.insert(pr)
			for _, mv := range snap.Movimientos {
				if mv.SKU == pr.SKU {
					st.Movimientos.c.insert(mv)
				}
			}
			nuevos++
		}
		if nuevos > 0 {
			log.Printf("Mongo: agregados %d producto(s) nuevo(s) de la demo", nuevos)
		}
	}
	if len(st.Clientes.List(demoID)) == 0 {
		for _, cl := range snap.Clientes {
			st.Clientes.c.insert(cl)
		}
	}
	if len(st.CuentasCobro.List(demoID)) == 0 {
		for _, cc := range snap.CuentasCobro {
			st.CuentasCobro.c.insert(cc)
		}
	}
	if len(st.MetodosPago.List(demoID)) == 0 {
		for _, mp := range snap.MetodosPago {
			st.MetodosPago.c.insert(mp)
		}
	}
	if len(st.Dispositivos.List(demoID)) == 0 {
		for _, dv := range snap.Dispositivos {
			st.Dispositivos.c.insert(dv)
		}
	}
	// Retenciones demo (IVA/ISLR): su asiento lo deriva RecontabilizarPendientes al
	// arrancar, idempotente por RefID.
	if len(st.Retenciones.List(demoID)) == 0 {
		for _, rt := range snap.Retenciones {
			st.Retenciones.c.insert(rt)
		}
		if len(snap.Retenciones) > 0 {
			log.Printf("Mongo: sembradas %d retenciones demo", len(snap.Retenciones))
		}
	}
	if len(st.Cotizaciones.List(demoID)) == 0 {
		for _, ct := range snap.Cotizaciones {
			st.Cotizaciones.c.insert(ct)
		}
	}
	// Compras: proveedores y órdenes de compra demo, con su propio guard por
	// colección igual que el resto de datos de negocio.
	if len(st.Proveedores.List(demoID)) == 0 {
		for _, pv := range snap.Proveedores {
			st.Proveedores.c.insert(pv)
		}
	}
	if len(st.OrdenesCompra.List(demoID)) == 0 {
		for _, oc := range snap.OrdenesCompra {
			st.OrdenesCompra.c.insert(oc)
		}
	}
	// Solicitudes de presupuesto (RFQ) demo, con su propio guard por colección.
	if len(st.Solicitudes.List(demoID)) == 0 {
		for _, sol := range snap.Solicitudes {
			st.Solicitudes.c.insert(sol)
		}
	}
	// Listas de precio demo (venta y compra), con su propio guard por colección.
	if len(st.ListasPrecio.List(demoID)) == 0 {
		for _, lp := range snap.ListasPrecio {
			st.ListasPrecio.c.insert(lp)
		}
	}
	// Cupones de descuento demo, con su propio guard por colección.
	if len(st.Cupones.List(demoID)) == 0 {
		for _, cp := range snap.Cupones {
			st.Cupones.c.insert(cp)
		}
	}
	// Promociones demo (carrusel de la pantalla del cliente), con su propio guard.
	if len(st.Promociones.List(demoID)) == 0 {
		for _, pr := range snap.Promociones {
			st.Promociones.c.insert(pr)
		}
	}
	// Unidades de medida demo (maestro del que el catálogo elige su UnidadBase),
	// con su propio guard por colección.
	if len(st.Unidades.List(demoID)) == 0 {
		for _, u := range snap.Unidades {
			st.Unidades.c.insert(u)
		}
	}
	// Formatos de documento demo (factura y cotización tamaño carta), con su propio
	// guard por colección.
	if len(st.Plantillas.List(demoID)) == 0 {
		for _, p := range snap.Plantillas {
			st.Plantillas.c.insert(p)
		}
	}
	// Mesas demo (módulo Restaurante) + plano del salón + activación del módulo, con
	// su propio guard.
	if demoID != "" && len(st.Mesas.List(demoID, "")) == 0 {
		var sedeDemo string
		for _, m := range snap.Mesas {
			st.Mesas.c.insert(m)
			sedeDemo = m.SedeID
		}
		if len(snap.Mesas) > 0 {
			st.Modulos.Upsert(aplicacion.Instalacion{
				EmpresaID: demoID, ModuloID: aplicacion.ModRestaurante,
				Instalado: true, Activo: true, Actualizada: snap.Mesas[0].Creada,
			})
			// Plano demo (grilla 8×6 con celdas bloqueadas), espejo del seed in-memory.
			st.Planos.Upsert(mesadom.Plano{
				EmpresaID: demoID, SedeID: sedeDemo, Filas: 6, Columnas: 8,
				Bloqueadas: []mesadom.Celda{{Columna: 4, Fila: 2}, {Columna: 5, Fila: 2}, {Columna: 6, Fila: 2}, {Columna: 3, Fila: 4}},
				Actualizada: snap.Mesas[0].Creada,
			})
			log.Printf("Mongo: sembradas %d mesas demo, plano y activado el módulo Restaurante", len(snap.Mesas))
		}
	}
	// Contadores de numeración (colección `contadores`). El Numerador vive en su
	// propia colección con documentos {id:"empresa|sede|serie", seq:n}, y arranca en
	// 0. Aquí se inicializa ADELANTADO con el estado del mismo Numerador in-memory
	// que ya numeró el snapshot: así la primera emisión viva sigue la correlativa en
	// vez de reiniciar en 1 y colisionar con los folios sembrados (FL/C y COT).
	if demoID != "" && len(snap.Contadores) > 0 && contadoresDemoVacio(st, demoID) {
		ctx, cancel := opctx()
		for key, seq := range snap.Contadores {
			_, _ = st.Numerador.c.InsertOne(ctx, map[string]any{"id": key, "seq": seq})
		}
		cancel()
		log.Printf("Mongo: inicializados %d contador(es) de numeración adelantados", len(snap.Contadores))
	}
	// Configuración de moneda (R10) de una empresa sembrada antes de que existiera.
	// Se rellena SOLO si está vacía: si alguien ya eligió su moneda principal, no
	// se le pisa. Sin esto, la demo en una base vieja no tendría con qué mostrar
	// el precio dual.
	for _, e := range snap.Empresas {
		actual, ok := st.Empresas.ByID(e.ID)
		if !ok || actual.MonedaPrincipal != "" {
			continue
		}
		actual.MonedaPrincipal = e.MonedaPrincipal
		actual.FuenteTasa = e.FuenteTasa
		actual.PreciosEnUsd = e.PreciosEnUsd
		st.Empresas.Update(actual)
		log.Printf("Mongo: configuración de moneda rellenada en %s (%s, fuente %s)", actual.ID, actual.MonedaPrincipal, actual.FuenteTasa)
	}

	// Tasa inicial (ámbito de plataforma). Solo si no hay ninguna: si la
	// sincronización ya trajo la del BCV, sembrar la de demostración por encima
	// sería degradar un dato real con uno inventado.
	if len(st.Tasas.Historial("", 1)) == 0 {
		for _, t := range snap.Tasas {
			st.Tasas.c.insert(t)
		}
		log.Printf("Mongo: sembrada la tasa inicial (%d registro(s))", len(snap.Tasas))
	}

	// Demos por RUBRO (restaurante, ferretería, farmacia). Va aparte y es ADITIVO por
	// empresa: los guards de arriba miran solo el tenant demo (emp_demo), así que en una
	// base ya sembrada nunca entrarían. No toca emp_demo ni exige subir versionSeedDemo
	// (que regeneraría la bodega demo).
	sembrarNichos(st, semilla)
}

// filtroContadoresDemo casa los contadores de numeración del tenant demo por el
// PREFIJO de su id ("empresaID|sedeID|serie"); esos documentos no llevan campo
// `empresaid`, así que se filtran por el id. QuoteMeta escapa el "|" (que en una
// expresión regular sería alternancia y casaría con todo).
func filtroContadoresDemo(demoID string) map[string]any {
	return map[string]any{"id": map[string]any{"$regex": "^" + regexp.QuoteMeta(demoID+"|")}}
}

// contadoresDemoVacio indica que todavía no hay contadores sembrados para el
// tenant demo; es el guard del bloque de siembra (idempotente por colección).
func contadoresDemoVacio(st *Store, demoID string) bool {
	ctx, cancel := opctx()
	defer cancel()
	n, err := st.Numerador.c.CountDocuments(ctx, filtroContadoresDemo(demoID))
	return err == nil && n == 0
}

// metaSeed es el registro de versión del seed de demostración.
type metaSeed struct {
	ID      string `bson:"id"`
	Version int    `bson:"version"`
}

// versionDelSeed lee la versión sembrada (0 si no hay registro).
func versionDelSeed(db *gomongo.Database) int {
	c := coll[metaSeed]{db.Collection("meta")}
	m, ok := c.one(map[string]any{"id": "seed_demo"})
	if !ok {
		return 0
	}
	return m.Version
}

func guardarVersionDelSeed(db *gomongo.Database) {
	c := coll[metaSeed]{db.Collection("meta")}
	c.replace("seed_demo", metaSeed{ID: "seed_demo", Version: versionSeedDemo})
}

// limpiarDemo borra los datos de negocio de la empresa DEMO para que se vuelvan a
// sembrar con la versión nueva. El filtro por `empresaid` es obligatorio en cada
// borrado: es lo que garantiza que no pueda tocar otro tenant.
func limpiarDemo(st *Store, demoID string) {
	if demoID == "" {
		return // sin tenant demo no hay nada que limpiar; jamás un borrado sin filtro
	}
	f := map[string]any{"empresaid": demoID}
	n := st.Productos.c.delMany(f) + st.Movimientos.c.delMany(f) + st.Documentos.c.delMany(f) +
		st.Transferencias.c.delMany(f) + st.Rubros.c.delMany(f) + st.Clientes.c.delMany(f) +
		st.CuentasCobro.c.delMany(f) + st.MetodosPago.c.delMany(f) + st.Dispositivos.c.delMany(f) + st.Cotizaciones.c.delMany(f) + st.Cajas.c.delMany(f) + st.Cajeros.c.delMany(f) +
		st.SesionesCaja.c.delMany(f) + st.CuentasContables.c.delMany(f) + st.Asientos.c.delMany(f) +
		st.Proveedores.c.delMany(f) + st.OrdenesCompra.c.delMany(f) + st.Solicitudes.c.delMany(f) + st.ListasPrecio.c.delMany(f) + st.Cupones.c.delMany(f) + st.Promociones.c.delMany(f) + st.Unidades.c.delMany(f) + st.Retenciones.c.delMany(f)
	// El numerador fiscal también se reinicia para que la siembra lo deje otra vez
	// adelantado sobre los folios nuevos. Va con la colección cruda porque el
	// numerador no usa el envoltorio genérico; y su filtro es por PREFIJO del id
	// ("empresa|sede|serie"), no por `empresaid` (esos documentos no lo llevan).
	ctx, cancel := opctx()
	defer cancel()
	if _, err := st.Numerador.c.DeleteMany(ctx, filtroContadoresDemo(demoID)); err != nil {
		log.Printf("Mongo: no se pudo reiniciar el numerador del demo: %v", err)
	}
	log.Printf("Mongo: datos de DEMOSTRACIÓN regenerados (versión %d) — %d registros reemplazados de %s",
		versionSeedDemo, n, demoID)
}
