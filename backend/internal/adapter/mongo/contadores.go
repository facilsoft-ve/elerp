package mongo

import (
	"log"

	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

/* INTEGRIDAD DEL CONTADOR DE FOLIOS.
 *
 * `contadores` es la colección que decide qué folio recibe el próximo
 * documento. Es, junto con los documentos, la pieza donde un error no se puede
 * corregir después: los documentos son append-only, así que un folio entregado
 * dos veces queda entregado dos veces.
 *
 * Nunca declaró un índice único sobre `id`, y una siembra que insertaba en vez
 * de hacer upsert llegó a dejar CINCO copias de la misma clave con secuencias
 * distintas (13 y 18 en la misma serie). Con copias divergentes, una lectura ve
 * un número y el incremento toca otro: exactamente la forma de repetir un folio.
 *
 * Esto corre al arrancar y hace las dos cosas en el orden que importa: primero
 * funde los duplicados quedándose con el MÁS ALTO —nunca se baja, porque ese
 * folio pudo haberse emitido—, y recién entonces crea el índice único que impide
 * que vuelva a pasar.
 */
func repararContadores(db *gomongo.Database) {
	c := db.Collection("contadores")
	ctx, cancel := opctx()
	defer cancel()

	// 1. Fundir duplicados. Se borra todo lo de la clave repetida y se reinserta
	//    una sola vez con el máximo: más simple y más seguro que elegir cuál de
	//    las copias conservar.
	cur, err := c.Aggregate(ctx, []map[string]any{
		{"$group": map[string]any{
			"_id": "$id",
			"n":   map[string]any{"$sum": 1},
			"max": map[string]any{"$max": "$seq"},
		}},
		{"$match": map[string]any{"n": map[string]any{"$gt": 1}}},
	})
	if err == nil {
		var grupos []struct {
			Clave string `bson:"_id"`
			Max   int    `bson:"max"`
		}
		if err := cur.All(ctx, &grupos); err == nil {
			for _, g := range grupos {
				if g.Clave == "" {
					continue
				}
				if _, err := c.DeleteMany(ctx, map[string]any{"id": g.Clave}); err != nil {
					continue
				}
				if _, err := c.InsertOne(ctx, map[string]any{"id": g.Clave, "seq": g.Max}); err != nil {
					continue
				}
				log.Printf("Mongo: el contador %q tenía copias divergentes; se conservó el folio más alto (%d)", g.Clave, g.Max)
			}
		}
		_ = cur.Close(ctx)
	}

	// 2. Índice único. A partir de acá una segunda copia de la misma clave es
	//    imposible: el upsert falla en vez de duplicar en silencio.
	if _, err := c.Indexes().CreateOne(ctx, gomongo.IndexModel{
		Keys:    map[string]any{"id": 1},
		Options: options.Index().SetUnique(true).SetName("contador_id_unico"),
	}); err != nil {
		log.Printf("Mongo: no se pudo crear el índice único de contadores: %v", err)
	}
}
