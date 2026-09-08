// Package mongo implementa los puertos del dominio sobre MongoDB.
//
// Aislamiento de tenant (ADR-04 / §13.1): como Mongo no tiene Row-Level
// Security, TODA consulta de un agregado de negocio lleva un filtro obligatorio
// por `empresaid` a nivel de query (ver repos.go). Es la última línea de
// defensa además del filtrado en la capa de aplicación.
package mongo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Connect abre la conexión y hace ping.
func Connect(uri, dbName string) (*gomongo.Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := gomongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	return client.Database(dbName), nil
}

func opctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 8*time.Second)
}

func newID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// coll es un envoltorio genérico sobre una colección tipada por el campo lógico "id".
type coll[T any] struct{ c *gomongo.Collection }

func (m coll[T]) all(filter map[string]any) []T {
	ctx, cancel := opctx()
	defer cancel()
	cur, err := m.c.Find(ctx, filter, options.Find())
	out := []T{}
	if err != nil {
		return out
	}
	defer cur.Close(ctx)
	_ = cur.All(ctx, &out)
	if out == nil {
		out = []T{}
	}
	return out
}

func (m coll[T]) one(filter map[string]any) (T, bool) {
	ctx, cancel := opctx()
	defer cancel()
	var out T
	err := m.c.FindOne(ctx, filter).Decode(&out)
	if err != nil {
		var zero T
		return zero, false
	}
	return out, true
}

func (m coll[T]) insert(doc T) {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = m.c.InsertOne(ctx, doc)
}

func (m coll[T]) replace(id string, doc T) {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = m.c.ReplaceOne(ctx, map[string]any{"id": id}, doc, options.Replace().SetUpsert(true))
}

func (m coll[T]) count() int64 {
	ctx, cancel := opctx()
	defer cancel()
	n, err := m.c.CountDocuments(ctx, map[string]any{})
	if err != nil {
		return 0
	}
	return n
}

// delMany borra todos los documentos que casen con el filtro y devuelve cuántos.
// Solo lo usa la regeneración de datos de DEMOSTRACIÓN (ver seed.go): ninguna ruta
// de la aplicación borra registros de negocio.
func (m coll[T]) delMany(filter map[string]any) int64 {
	ctx, cancel := opctx()
	defer cancel()
	res, err := m.c.DeleteMany(ctx, filter)
	if err != nil {
		return 0
	}
	return res.DeletedCount
}

func (m coll[T]) del(filter map[string]any) bool {
	ctx, cancel := opctx()
	defer cancel()
	res, err := m.c.DeleteOne(ctx, filter)
	return err == nil && res.DeletedCount > 0
}
