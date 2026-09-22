package mongo

import (
	"context"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	gomongo "go.mongodb.org/mongo-driver/mongo"
)

/* DEMO EFÍMERA: una copia por visitante, que se borra sola.
 *
 * EL PROBLEMA. La demo era un tenant compartido: quien entraba veía —y pisaba—
 * lo que había hecho el visitante anterior. Para una demostración comercial eso
 * es lo peor que puede pasar, porque el prospecto no distingue entre «así es el
 * producto» y «esto lo dejó otro»; y quien está presentando no puede tocar nada
 * sin arruinarle la sesión a alguien más.
 *
 * LA COPIA SE HACE EN LA CAPA DE PERSISTENCIA, no en el dominio, y esa es la
 * decisión que sostiene todo lo demás. Clonar tipo por tipo obliga a acordarse
 * de cada módulo nuevo —y el exportador de respaldos ya se quedó atrás: hoy no
 * incluye ni el salón ni los pedidos—. Copiar documentos crudos por colección
 * cubre lo que existe y lo que venga, sin que nadie tenga que acordarse.
 *
 * SE CONSERVAN LOS IDS y solo cambia `empresaid`. Como TODO repositorio filtra
 * por empresa (Mongo no tiene RLS: el aislamiento se hace en el adaptador), dos
 * tenants pueden tener un producto con el mismo id sin cruzarse, y las
 * referencias internas —sedeId, productoId, documentoId— siguen siendo válidas
 * sin remapear nada. Es el mismo criterio que ya usa RestaurarDatos.
 */

// prefijoDemo marca las empresas efímeras. Es lo que permite reconocerlas para
// borrarlas sin tocar jamás un tenant real.
const prefijoDemo = "empdemo_"

// EsDemoEfimera indica si un id de empresa es una copia de demostración.
func EsDemoEfimera(empresaID string) bool { return strings.HasPrefix(empresaID, prefijoDemo) }

// NuevoIDDemo genera el id de una empresa efímera.
func NuevoIDDemo() string { return newID(prefijoDemo) }

/* coleccionesSinEmpresa son las que NO llevan `empresaid` y por eso no se
 * clonan con el barrido genérico:
 *
 *   - usuarios, organizaciones, membresias: la identidad la crea quien abre la
 *     sesión, no la copia.
 *   - contadores: su clave es "empresa|sede|serie", así que se reescribe aparte.
 *   - sesiones, auditoria: no son datos de negocio del tenant.
 *   - seedmeta: es control del propio sembrado.
 */
var coleccionesSinEmpresa = map[string]bool{
	"usuarios": true, "organizaciones": true, "membresias": true,
	"contadores": true, "sesiones": true, "seedmeta": true, "credenciales": true,
}

// ClonarEmpresa copia TODOS los documentos de negocio de una empresa a otra.
// Devuelve cuántos documentos copió.
//
// Es una copia cruda: no re-corre lógica de negocio, así que los ledgers
// append-only se reponen tal cual con sus sellos intactos — que es exactamente
// lo que hace falta para que la demo se vea como un local que lleva meses
// trabajando y no como uno recién abierto.
func ClonarEmpresa(db *gomongo.Database, origenID, destinoID string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	nombres, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		log.Printf("demo: no se pudieron listar las colecciones: %v", err)
		return 0
	}
	total := 0
	for _, nombre := range nombres {
		if coleccionesSinEmpresa[nombre] {
			continue
		}
		total += clonarColeccion(ctx, db.Collection(nombre), origenID, destinoID)
	}
	total += clonarContadores(ctx, db.Collection("contadores"), origenID, destinoID)
	return total
}

// clonarColeccion copia los documentos de una colección cambiando `empresaid`.
func clonarColeccion(ctx context.Context, c *gomongo.Collection, origenID, destinoID string) int {
	cur, err := c.Find(ctx, bson.M{"empresaid": origenID})
	if err != nil {
		return 0
	}
	defer cur.Close(ctx)

	lote := make([]any, 0, 256)
	copiados := 0
	vaciar := func() {
		if len(lote) == 0 {
			return
		}
		if _, err := c.InsertMany(ctx, lote); err == nil {
			copiados += len(lote)
		}
		lote = lote[:0]
	}
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		// El _id se quita para que Mongo asigne uno nuevo: es la clave física de
		// la colección y repetirla haría chocar la inserción. El id LÓGICO del
		// documento sí se conserva — es lo que mantiene válidas las referencias.
		delete(doc, "_id")
		doc["empresaid"] = destinoID
		lote = append(lote, doc)
		if len(lote) >= 256 {
			vaciar()
		}
	}
	vaciar()
	return copiados
}

// clonarContadores reescribe el prefijo de la clave del numerador.
//
// Va aparte porque su clave es compuesta ("empresa|sede|serie") y no un campo
// `empresaid`. Sin esto, la demo copiada arrancaría su numeración en 1 y la
// primera factura chocaría con un folio que ya está en los documentos copiados.
func clonarContadores(ctx context.Context, c *gomongo.Collection, origenID, destinoID string) int {
	cur, err := c.Find(ctx, bson.M{"id": bson.M{"$regex": "^" + origenID + "\\|"}})
	if err != nil {
		return 0
	}
	defer cur.Close(ctx)
	n := 0
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		id, _ := doc["id"].(string)
		delete(doc, "_id")
		doc["id"] = destinoID + strings.TrimPrefix(id, origenID)
		if _, err := c.InsertOne(ctx, doc); err == nil {
			n++
		}
	}
	return n
}

// BorrarEmpresaDemo elimina una copia efímera y todo lo suyo.
//
// SOLO borra empresas con el prefijo de demo: un borrado sin ese candado es la
// clase de función que un día se llama con el id equivocado y se lleva un tenant
// real por delante.
func BorrarEmpresaDemo(db *gomongo.Database, empresaID string) int {
	if !EsDemoEfimera(empresaID) {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	nombres, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return 0
	}
	total := 0
	for _, nombre := range nombres {
		if nombre == "seedmeta" {
			continue
		}
		res, err := db.Collection(nombre).DeleteMany(ctx, bson.M{"empresaid": empresaID})
		if err == nil {
			total += int(res.DeletedCount)
		}
	}
	// Los contadores y la empresa misma, por sus claves propias.
	if res, err := db.Collection("contadores").DeleteMany(ctx,
		bson.M{"id": bson.M{"$regex": "^" + empresaID + "\\|"}}); err == nil {
		total += int(res.DeletedCount)
	}
	if res, err := db.Collection("empresas").DeleteMany(ctx, bson.M{"id": empresaID}); err == nil {
		total += int(res.DeletedCount)
	}
	if res, err := db.Collection("sedes").DeleteMany(ctx, bson.M{"empresaid": empresaID}); err == nil {
		total += int(res.DeletedCount)
	}
	if res, err := db.Collection("membresias").DeleteMany(ctx, bson.M{"empresaid": empresaID}); err == nil {
		total += int(res.DeletedCount)
	}
	return total
}

// BarrerDemosVencidas borra las copias cuya fecha de expiración ya pasó.
//
// Corre desde el lazo de fondo. Que la limpieza sea automática y no manual es
// parte del diseño: una demo que hay que recordar borrar termina siendo una base
// con doscientas copias abandonadas.
func BarrerDemosVencidas(db *gomongo.Database) int {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cur, err := db.Collection("empresas").Find(ctx, bson.M{
		"id":       bson.M{"$regex": "^" + prefijoDemo},
		"expirael": bson.M{"$lt": time.Now().UTC().Format(time.RFC3339)},
	})
	if err != nil {
		return 0
	}
	defer cur.Close(ctx)
	ids := []string{}
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err == nil {
			if id, ok := doc["id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	n := 0
	for _, id := range ids {
		if BorrarEmpresaDemo(db, id) > 0 {
			n++
		}
	}
	if n > 0 {
		log.Printf("demo: %d copia(s) vencida(s) eliminada(s)", n)
	}
	return n
}

/* ADAPTADOR del puerto application.CopiaDemo. */

// CopiaDemoRepo implementa la copia efímera sobre Mongo.
type CopiaDemoRepo struct{ db *gomongo.Database }

// NuevaCopiaDemo construye el adaptador.
func NuevaCopiaDemo(db *gomongo.Database) *CopiaDemoRepo { return &CopiaDemoRepo{db: db} }

func (r *CopiaDemoRepo) Clonar(origenID, destinoID string) int {
	return ClonarEmpresa(r.db, origenID, destinoID)
}
func (r *CopiaDemoRepo) Borrar(empresaID string) int { return BorrarEmpresaDemo(r.db, empresaID) }
func (r *CopiaDemoRepo) BarrerVencidas() int         { return BarrerDemosVencidas(r.db) }
func (r *CopiaDemoRepo) NuevoID() string             { return NuevoIDDemo() }
