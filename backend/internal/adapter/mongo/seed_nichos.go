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
