package mongo

import (
	"log"

	"github.com/mornix/elerp/internal/adapter/inmem"
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
func sembrarNichos(st *Store, semilla *inmem.Store) {
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
		// Usuarios por rol: aditivo por id, para no pisar a nadie que ya exista.
		for _, u := range snap.Usuarios {
			if _, ya := st.Usuarios.ByID(u.ID); !ya {
				st.Usuarios.c.insert(u)
			}
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
			ctx, cancel := opctx()
			for clave, seq := range snap.Contadores {
				_, _ = st.Numerador.c.InsertOne(ctx, map[string]any{"id": clave, "seq": seq})
			}
			cancel()
			log.Printf("Mongo: %s → %d documentos fiscales y %d contador(es) de numeración",
				n.Giro, len(snap.Documentos), len(snap.Contadores))
		}
		// Cuentas de mesa abiertas (restaurante en servicio).
		if len(snap.CuentasMesa) > 0 && len(st.Cuentas.Abiertas(n.EmpresaID, n.SedeID)) == 0 {
			for _, c := range snap.CuentasMesa {
				st.Cuentas.c.insert(c)
			}
			log.Printf("Mongo: %s → %d cuentas de mesa abiertas", n.Giro, len(snap.CuentasMesa))
		}

		// Módulos instalados (el restaurante trae el suyo activo).
		for _, m := range snap.Modulos {
			if _, ya := st.Modulos.ByID(n.EmpresaID, m.ModuloID); !ya {
				st.Modulos.Upsert(m)
			}
		}

		// Salón: grilla + mesas (solo el restaurante).
		if snap.TienePlano {
			if _, ya := st.Planos.Get(n.EmpresaID, n.SedeID); !ya {
				st.Planos.Upsert(snap.Plano)
			}
		}
		if len(snap.Mesas) > 0 && len(st.Mesas.List(n.EmpresaID, "")) == 0 {
			for _, m := range snap.Mesas {
				st.Mesas.c.insert(m)
			}
			log.Printf("Mongo: %s → %d mesas y plano del salón", n.Giro, len(snap.Mesas))
		}
	}
}
