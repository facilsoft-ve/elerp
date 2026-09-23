package mongo

import (
	"log"
	"strings"

	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/adapter/inmem"
	mesadom "github.com/mornix/elerp/internal/domain/mesa"
)

// sembrarNichos planta las empresas demo por RUBRO (restaurante, ferretería, farmacia).
//
// Por qué va aparte del cuerpo de Seed: los guards de allí se preguntan por el tenant
// demo (`emp_demo`) y el bloque de identidad exige una base virgen
// (`st.Empresas.c.count() == 0`). En el servidor la base YA está sembrada, así que por
// esa vía los rubros nuevos nunca aparecerían. Acá el criterio es por EMPRESA: cada
// nicho se siembra solo si falta, sin tocar `emp_demo` y sin subir versionSeedDemo (que
// regeneraría la bodega demo, algo que no corresponde hacer de rebote).
//
// Es idempotente: correrlo dos veces no duplica nada.
func sembrarNichos(st *Store, semilla *inmem.Store, refrescar bool) {
	// Credenciales de DEMOSTRACIÓN (contraseña única) de todos los tenants demo,
	// incluida la bodega: el bloque de identidad del Seed principal solo corre en una
	// base virgen, así que en el servidor no llegarían nunca. Aditivo por email.
	for _, c := range semilla.Credenciales.Todas() {
		if _, ya := st.Credenciales.ByEmail(c.Email); !ya {
			st.Credenciales.Create(c)
		}
	}

	for _, n := range inmem.NichosDemo() {
		snap := semilla.SnapshotNicho(n)
		if snap.Empresa.ID == "" {
			log.Printf("Mongo: el nicho %s no está en la semilla; se omite", n.EmpresaID)
			continue
		}

		/* NO SE BORRAN LOS ASIENTOS DEL NICHO AL REGENERAR, y se intentó.
		 *
		 * Parecía correcto —el libro es una proyección, que se rehaga— y el
		 * resultado fue peor: el demo del restaurante tiene un PERÍODO CERRADO hasta
		 * agosto, y su inventario inicial está fechado dentro de él. Borrados los
		 * asientos, el backfill no puede recrearlos: el período los rechaza, uno por
		 * uno, y el tenant queda sin libro en vez de con un libro viejo.
		 *
		 * La lección es del diseño, no del seed: reconstruir un libro append-only
		 * solo es seguro si nada bloquea la reconstrucción. Con períodos cerrados de
		 * por medio, borrar es un camino de ida.
		 *
		 * Queda pendiente de verdad: la valoración del restaurante no cuadra contra
		 * su cuenta 1201 (ver docs). La causa es el cruce entre el período cerrado y
		 * datos sembrados, y se resuelve en Contabilidad, no acá. */

		// Identidad y estructura: organización, empresa, sedes y membresías.
		if _, existe := st.Empresas.ByID(n.EmpresaID); !existe {
			if _, hayOrg := st.Organizaciones.ByID(n.OrgID); !hayOrg {
				st.Organizaciones.c.insert(snap.Org)
			}
			st.Empresas.c.insert(snap.Empresa)
			for _, sd := range snap.Sedes {
				st.Sedes.c.insert(sd)
			}
			// Sin membresía el rubro no aparece en el selector de empresa del app.
			for _, m := range snap.Membresias {
				st.Membresias.c.insert(m)
			}
			log.Printf("Mongo: sembrada la demo de %s (%s)", n.Giro, snap.Empresa.Nombre)
		}

		// Catálogo + existencias. La existencia es una proyección del ledger, así que
		// productos y movimientos van juntos: sembrar uno sin el otro dejaría el Kardex
		// descuadrado.
		if len(st.Productos.List(n.EmpresaID)) == 0 {
			for _, r := range snap.Rubros {
				st.Rubros.c.insert(r)
			}
			for _, pr := range snap.Productos {
				st.Productos.c.insert(pr)
			}
			for _, mv := range snap.Movimientos {
				st.Movimientos.c.insert(mv)
			}
			log.Printf("Mongo: %s → %d productos y %d movimientos", n.Giro, len(snap.Productos), len(snap.Movimientos))
		}

		// Puesta al día del CATÁLOGO demo (solo tenants demo).
		//
		// El bloque de arriba solo siembra si el catálogo está vacío, así que una
		// demo ya plantada nunca vería un producto nuevo ni una corrección. Y las
		// demos son material de venta: si enseñan algo equivocado —los postres
		// cargados como reventa, cuando la receta de postre es justo lo que hay que
		// mostrar— el prospecto se lleva esa idea.
		//
		// Es aditivo y acotado: inserta lo que falta por SKU y pone al día SOLO la
		// definición de plato (receta, rubro y comandera) de los que ya están. No
		// toca precios, existencias ni documentos, ni ninguna empresa que no sea demo.
		actualizarCatalogoDemo(st, n.EmpresaID, snap)

		if len(st.Clientes.List(n.EmpresaID)) == 0 {
			for _, cl := range snap.Clientes {
				st.Clientes.c.insert(cl)
			}
		}

		// --- Operación: caja, personal, cobros, proveedores y facturación ---
		// Sin caja habilitada no se puede facturar, y sin documentos el rubro se ve
		// vacío en Facturación, Ventas, Tesorería y Contabilidad.
		if len(st.Cajas.List(n.EmpresaID)) == 0 {
			for _, cj := range snap.Cajas {
				st.Cajas.c.insert(cj)
			}
			for _, cr := range snap.Cajeros {
				st.Cajeros.c.insert(cr)
			}
			log.Printf("Mongo: %s → %d cajas y %d cajeros (PIN de demostración)", n.Giro, len(snap.Cajas), len(snap.Cajeros))
		}
		// Usuarios por rol y SUS MEMBRESÍAS: aditivo, para no pisar a nadie que ya exista.
		//
		// Las membresías van acá y no en el bloque de identidad de arriba por una razón
		// concreta: ese bloque solo corre cuando la empresa NO existe, así que en una base
		// ya sembrada (el servidor) los usuarios por rol que se agregaron después quedaban
		// con credencial pero SIN empresa — podían autenticarse y la app los mandaba al
		// asistente de «configura tu primera empresa», que es lo último que debe ver un
		// empleado. Pasó de verdad con el mesonero.
		for _, u := range snap.Usuarios {
			if _, ya := st.Usuarios.ByID(u.ID); !ya {
				st.Usuarios.c.insert(u)
			}
		}
		yaMiembro := map[string]bool{}
		for _, m := range st.Membresias.ByEmpresa(n.EmpresaID) {
			yaMiembro[m.UsuarioID] = true
		}
		faltantes := 0
		for _, m := range snap.Membresias {
			if m.UsuarioID == "" || yaMiembro[m.UsuarioID] {
				continue
			}
			st.Membresias.c.insert(m)
			yaMiembro[m.UsuarioID] = true
			faltantes++
		}
		if faltantes > 0 {
			log.Printf("Mongo: %s → %d membresía(s) de usuarios por rol", n.Giro, faltantes)
		}
		if len(st.CuentasCobro.List(n.EmpresaID)) == 0 {
			for _, cc := range snap.CuentasCobro {
				st.CuentasCobro.c.insert(cc)
			}
			for _, mp := range snap.MetodosPago {
				st.MetodosPago.c.insert(mp)
			}
		}
		if len(st.Proveedores.List(n.EmpresaID)) == 0 {
			for _, pr := range snap.Proveedores {
				st.Proveedores.c.insert(pr)
			}
		}
		if len(st.Documentos.List(n.EmpresaID)) == 0 {
			for _, d := range snap.Documentos {
				st.Documentos.c.insert(d)
			}
			// Los contadores del numerador van CON los documentos: si se sembraran
			// folios sin adelantar el contador, la primera factura real del prospecto
			// reiniciaría en 1 y colisionaría con un folio ya emitido (y los
			// documentos son append-only: no hay forma de arreglarlo después).
			// SOLO los contadores de ESTA empresa. El snapshot trae los de todas las
			// demo (comparten el Numerador in-memory), y sembrarlos completos en
			// cada nicho insertaba una copia del contador ajeno por arranque:
			// llegaron a convivir cinco copias de la misma clave con folios
			// distintos, que es la forma exacta de entregar un folio repetido.
			sembrarContadores(st, n.EmpresaID, snap.Contadores)
			log.Printf("Mongo: %s → %d documentos fiscales y %d contador(es) de numeración",
				n.Giro, len(snap.Documentos), len(snap.Contadores))
		}
		// Cuentas de mesa abiertas (restaurante en servicio).
		//
		// Se REMAPEA el id de la mesa por su NOMBRE. Los ids de la semilla los genera un
		// contador en memoria que cambia en cada arranque, mientras las mesas de Mongo
		// conservan los suyos del primer sembrado: insertar la cuenta con el id de la
		// semilla la deja HUÉRFANA —apuntando a una mesa que no existe— y la mesa nunca
		// se ve ocupada. Pasó de verdad: cuentas en mesa_196 con mesas en mesa_206.
		// El nombre ("1", "4", "T2") sí es estable, así que es la clave correcta.
		if len(snap.CuentasMesa) > 0 && len(st.Cuentas.Abiertas(n.EmpresaID, n.SedeID)) == 0 {
			porNombre := map[string]mesadom.Mesa{}
			for _, m := range st.Mesas.List(n.EmpresaID, n.SedeID) {
				porNombre[m.Nombre] = m
			}
			insertadas := 0
			for _, c := range snap.CuentasMesa {
				real, ok := porNombre[c.MesaNombre]
				if !ok {
					log.Printf("Mongo: %s → se omite la cuenta de la mesa %q (no existe en esta base)", n.Giro, c.MesaNombre)
					continue
				}
				c.MesaID = real.ID
				st.Cuentas.c.insert(c)
				insertadas++
				// Y la mesa queda OCUPADA: el tablero y el mapa leen su estado, y una
				// mesa "libre" con una cuenta abierta es una contradicción visible.
				if real.Estado != mesadom.EstadoOcupada {
					real.Estado = mesadom.EstadoOcupada
					st.Mesas.Update(real)
				}
			}
			log.Printf("Mongo: %s → %d cuentas de mesa abiertas", n.Giro, insertadas)
		}

		// Módulos instalados (el restaurante trae el suyo activo).
		for _, m := range snap.Modulos {
			if _, ya := st.Modulos.ByID(n.EmpresaID, m.ModuloID); !ya {
				st.Modulos.Upsert(m)
			}
		}

		// Asignación de mesas por mesonero (solo el restaurante): aditiva por usuario.
		if len(snap.Asignaciones) > 0 {
			yaAsignado := map[string]bool{}
			for _, a := range st.Asignaciones.List(n.EmpresaID, n.SedeID) {
				yaAsignado[a.UsuarioID] = true
			}
			nuevas := 0
			for _, a := range snap.Asignaciones {
				if yaAsignado[a.UsuarioID] {
					continue
				}
				st.Asignaciones.Upsert(a)
				nuevas++
			}
			if nuevas > 0 {
				log.Printf("Mongo: %s → %d asignación(es) de mesas a mesoneros", n.Giro, nuevas)
			}
		}

		// Credenciales de turno del salón (MS-): aditivas por código. Sin ellas la
		// pantalla de Turnos arranca vacía y no se entiende para qué sirve.
		for _, ms := range snap.Mesoneros {
			if _, ya := st.Mesoneros.ByCodigo(n.EmpresaID, ms.Codigo); !ya {
				st.Mesoneros.Create(ms)
			}
		}

		// Horario semanal de cada mesonero: aditivo por mesonero. Sin él el turno
		// no tiene hora de salida y el cierre automático no se puede ver.
		for _, h := range snap.Horarios {
			if ms, ok := st.Mesoneros.ByCodigo(n.EmpresaID, codigoDeHorario(snap, h.MesoneroID)); ok {
				h.MesoneroID = ms.ID // el id de la semilla cambia en cada arranque
				if _, ya := st.Horarios.ByMesonero(n.EmpresaID, ms.ID); !ya {
					st.Horarios.Upsert(h)
				}
			}
		}

		// Horario de ATENCIÓN del salón: es contra lo que se avisa una reserva
		// fuera de hora. Solo si no estaba configurado.
		if snap.TieneConfig {
			if cur, ok := st.ConfigSalon.Get(n.EmpresaID, n.SedeID); !ok || cur.HoraApertura == "" {
				st.ConfigSalon.Upsert(snap.ConfigSalon)
			}
		}

		// Comanderas (puestos de impresión de comandas): aditivas por nombre.
		if len(snap.Comanderas) > 0 {
			yaComandera := map[string]bool{}
			for _, imp := range st.Impresoras.List(n.EmpresaID, n.SedeID) {
				yaComandera[strings.ToLower(imp.Nombre)] = true
			}
			nuevas := 0
			for _, imp := range snap.Comanderas {
				if yaComandera[strings.ToLower(imp.Nombre)] {
					continue
				}
				st.Impresoras.Create(imp)
				nuevas++
			}
			if nuevas > 0 {
				log.Printf("Mongo: %s → %d comandera(s)", n.Giro, nuevas)
			}
		}

		// PEDIDOS: canales, zonas, repartidores y el tablero vivo. Sin esto la
		// bandeja arranca vacía en una base con Mongo, y el módulo se ve como una
		// pantalla sin usar en vez de un local trabajando.
		// Se REEMPLAZAN al regenerar la demo, como el plano: si solo se sembraran
		// cuando la lista está vacía, una base ya sembrada nunca vería el tablero
		// nuevo y la demostración quedaría con los pedidos de la versión anterior
		// —que es justo lo que pasó al cambiar qué productos se piden—.
		if len(snap.Pedidos) > 0 && (refrescar || len(st.Pedidos.List(n.EmpresaID, "")) == 0) {
			if refrescar {
				f := map[string]any{"empresaid": n.EmpresaID}
				st.Pedidos.c.delMany(f)
				st.CanalesPedido.c.delMany(f)
				st.ZonasPedido.c.delMany(f)
				st.Repartidores.c.delMany(f)
			}
			for _, c := range snap.CanalesPedido {
				st.CanalesPedido.Upsert(c)
			}
			for _, z := range snap.ZonasPedido {
				st.ZonasPedido.Upsert(z)
			}
			for _, r := range snap.Repartidores {
				st.Repartidores.Upsert(r)
			}
			for _, p := range snap.Pedidos {
				st.Pedidos.Append(p)
			}
			log.Printf("Mongo: %s → %d pedidos de delivery", n.Giro, len(snap.Pedidos))
		}

		/* ÓRDENES DE FABRICACIÓN. Mismo criterio que los pedidos: se reemplazan al
		 * regenerar, o una base ya sembrada nunca vería el módulo con datos.
		 *
		 * Los MOVIMIENTOS que estas órdenes generaron ya viajan con el inventario
		 * del nicho: sembrar las órdenes sin ellos dejaría el Kardex diciendo una
		 * cosa y la orden otra. */
		if len(snap.OrdenesFabricacion) > 0 && (refrescar || len(st.OrdenesFabricacion.List(n.EmpresaID, "")) == 0) {
			/* LAS ÓRDENES Y SUS MOVIMIENTOS VIAJAN JUNTOS, y esto no es una comodidad.
			 *
			 * El top-up de arriba copia los movimientos de los productos NUEVOS. Las
			 * salidas que una orden hace sobre insumos que YA existían quedan fuera de
			 * ese criterio, y entonces la orden dice que consumió y el Kardex dice que
			 * no: el inventario aparece con producto terminado que salió de la nada.
			 * Pasó exactamente así al sembrar la primera orden. */
			f := map[string]any{"empresaid": n.EmpresaID}
			if refrescar {
				st.OrdenesFabricacion.c.delMany(f)
				st.Movimientos.c.delMany(map[string]any{"empresaid": n.EmpresaID, "reftipo": "fabricacion"})
			}
			for _, o := range snap.OrdenesFabricacion {
				st.OrdenesFabricacion.Append(o)
			}
			movs := 0
			for _, mv := range snap.Movimientos {
				if mv.RefTipo != "fabricacion" {
					continue
				}
				if _, ya := st.Movimientos.c.one(map[string]any{"empresaid": n.EmpresaID, "id": mv.ID}); ya {
					continue
				}
				st.Movimientos.c.insert(mv)
				movs++
			}
			log.Printf("Mongo: %s → %d orden(es) de fabricación y %d movimiento(s)",
				n.Giro, len(snap.OrdenesFabricacion), movs)
		}

		// Salón: grilla + mesas (solo el restaurante).
		if snap.TienePlano {
			// El plano es CONFIGURACIÓN, no ledger: cuando la demo se regenera se
			// reemplaza, igual que el resto de los datos de demostración. Sin esto
			// una base ya sembrada nunca vería las zonas ni los mostradores nuevos,
			// que fue justo lo que pasó al agregarlos.
			if _, ya := st.Planos.Get(n.EmpresaID, n.SedeID); !ya || refrescar {
				st.Planos.Upsert(snap.Plano)
			}
		}
		/* MESAS del salón. Se repone lo que FALTE, no todo o nada.
		 *
		 * Antes solo se sembraba con el salón vacío, y eso dejaba sin arreglo un
		 * salón a medias — que es exactamente como quedó cuando el `replace` de
		 * Mongo, que filtraba solo por id y no por tenant, se llevó mesas del demo
		 * base hacia copias de demostración. Reponer por nombre es idempotente y
		 * devuelve el salón completo sin tocar lo que alguien haya movido. */
		if len(snap.Mesas) > 0 {
			hay := map[string]bool{}
			for _, m := range st.Mesas.List(n.EmpresaID, "") {
				hay[m.Nombre] = true
			}
			repuestas := 0
			for _, m := range snap.Mesas {
				if hay[m.Nombre] {
					continue
				}
				st.Mesas.c.insert(m)
				repuestas++
			}
			if repuestas > 0 {
				log.Printf("Mongo: %s → %d mesa(s) repuesta(s) en el salón", n.Giro, repuestas)
			}
		}
	}
}

// actualizarCatalogoDemo mantiene al día el catálogo de una empresa DEMO ya sembrada:
// agrega los productos (y su carga inicial) que falten por SKU y pone al día la
// definición de plato de los que ya existen. Idempotente y sin efectos fuera de la
// empresa demo que recibe.
func actualizarCatalogoDemo(st *Store, empresaID string, snap inmem.SnapshotEmpresa) {
	existentes := st.Productos.List(empresaID)
	if len(existentes) == 0 {
		return // recién sembrada por el bloque de arriba: ya está al día
	}
	porSKU := make(map[string]int, len(existentes))
	for i, p := range existentes {
		porSKU[p.SKU] = i
	}

	// La comandera del snapshot trae el id GENERADO EN MEMORIA, que no es el de esta
	// base (mismo problema que las cuentas de mesa: los ids de la semilla cambian en
	// cada arranque). Se traduce por NOMBRE, que sí es estable; si el puesto no existe
	// acá, se deja sin fijar y el producto se rutea por su rubro.
	nombreSemilla := make(map[string]string, len(snap.Comanderas)) // idSemilla → nombre
	for _, c := range snap.Comanderas {
		nombreSemilla[c.ID] = c.Nombre
	}
	idReal := map[string]string{} // nombre → id en esta base
	for _, c := range snap.Comanderas {
		for _, m := range st.Impresoras.List(empresaID, c.SedeID) {
			idReal[m.Nombre] = m.ID
		}
	}
	comanderaLocal := func(idDeLaSemilla string) string {
		if idDeLaSemilla == "" {
			return ""
		}
		return idReal[nombreSemilla[idDeLaSemilla]]
	}

	// 1) Productos que faltan: van con sus movimientos de carga inicial, porque la
	// existencia es una PROYECCIÓN del ledger — sembrar el producto sin su movimiento
	// deja un insumo en cero y la receta que lo usa no se puede preparar.
	nuevos := 0
	for _, pr := range snap.Productos {
		if _, ya := porSKU[pr.SKU]; ya {
			continue
		}
		pr.ComanderaID = comanderaLocal(pr.ComanderaID)
		st.Productos.c.insert(pr)
		for _, mv := range snap.Movimientos {
			if mv.SKU == pr.SKU {
				st.Movimientos.c.insert(mv)
			}
		}
		nuevos++
	}

	// 2) Definición de plato de los que ya existían. Solo estos tres campos: cambiar
	// precios o nombres de una demo en uso sería pisarle datos al que la está viendo.
	ajustados := 0
	for _, pr := range snap.Productos {
		i, ya := porSKU[pr.SKU]
		if !ya {
			continue
		}
		actual := existentes[i]
		comandera := comanderaLocal(pr.ComanderaID)
		if actual.EsPlato == pr.EsPlato && actual.ComanderaID == comandera &&
			len(actual.Receta) == len(pr.Receta) && actual.Rubro == pr.Rubro {
			continue
		}
		actual.EsPlato = pr.EsPlato
		actual.Receta = pr.Receta
		actual.Rubro = pr.Rubro
		actual.ComanderaID = comandera
		st.Productos.Update(actual)
		ajustados++
	}
	if nuevos > 0 || ajustados > 0 {
		log.Printf("Mongo: %s → catálogo demo al día (%d producto(s) nuevo(s), %d ajustado(s))",
			empresaID, nuevos, ajustados)
	}
}

// codigoDeHorario traduce el id de mesonero de la SEMILLA al código estable
// (MS-001). Los ids de la semilla los genera un contador en memoria que cambia
// en cada arranque, mientras los de Mongo conservan el suyo del primer sembrado:
// insertar el horario con el id de la semilla lo dejaría huérfano. Es el mismo
// problema —y la misma solución— que con las cuentas de mesa.
func codigoDeHorario(snap inmem.SnapshotEmpresa, mesoneroIDSemilla string) string {
	for _, m := range snap.Mesoneros {
		if m.ID == mesoneroIDSemilla {
			return m.Codigo
		}
	}
	return ""
}

/* CONTADORES DE NUMERACIÓN: sembrar sin duplicar.
 *
 * La colección `contadores` es la que decide qué folio recibe el próximo
 * documento. Dos copias de la misma clave con valores distintos significan que
 * una lectura puede ver 13 mientras la otra va en 18 — y ahí se entrega un folio
 * ya usado, sobre documentos que son append-only y no se pueden corregir
 * después.
 *
 * Por eso se siembra con `$max` y upsert: la clave se crea si falta y su
 * secuencia SOLO AVANZA. Repetir la siembra deja de tener consecuencias.
 */
func sembrarContadores(st *Store, empresaID string, contadores map[string]int) {
	ctx, cancel := opctx()
	defer cancel()
	prefijo := empresaID + "|"
	n := 0
	for clave, seq := range contadores {
		if !strings.HasPrefix(clave, prefijo) {
			continue // contador de otra empresa demo: no es asunto de este nicho
		}
		_, err := st.Numerador.c.UpdateOne(ctx,
			map[string]any{"id": clave},
			map[string]any{"$max": map[string]any{"seq": seq}, "$setOnInsert": map[string]any{"id": clave}},
			options.Update().SetUpsert(true))
		if err == nil {
			n++
		}
	}
	if n > 0 {
		log.Printf("Mongo: %s → %d contador(es) de numeración", empresaID, n)
	}
}
