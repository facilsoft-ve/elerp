package store

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp-platform/internal/facturacion"
)

// --- Planes en Mongo -------------------------------------------------------

// MongoPlanes persiste el catálogo de planes en la colección "planes".
type MongoPlanes struct{ c *mongo.Collection }

func NewMongoPlanes(db *mongo.Database) *MongoPlanes {
	return &MongoPlanes{c: db.Collection("planes")}
}

func (m *MongoPlanes) List() []facturacion.Plan {
	ctx, cancel := mctx()
	defer cancel()
	cur, err := m.c.Find(ctx, bson.M{})
	if err != nil {
		return []facturacion.Plan{}
	}
	var out []facturacion.Plan
	_ = cur.All(ctx, &out)
	if out == nil {
		out = []facturacion.Plan{}
	}
	return out
}

func (m *MongoPlanes) ByID(id string) (facturacion.Plan, bool) {
	ctx, cancel := mctx()
	defer cancel()
	var p facturacion.Plan
	err := m.c.FindOne(ctx, bson.M{"id": id}).Decode(&p)
	return p, err == nil
}

func (m *MongoPlanes) Create(p facturacion.Plan) facturacion.Plan {
	if p.ID == "" {
		p.ID = randID()
	}
	if p.Creado == "" {
		p.Creado = ahoraRFC()
	}
	ctx, cancel := mctx()
	defer cancel()
	_, _ = m.c.InsertOne(ctx, p)
	return p
}

func (m *MongoPlanes) Update(p facturacion.Plan) (facturacion.Plan, bool) {
	ctx, cancel := mctx()
	defer cancel()
	res, err := m.c.ReplaceOne(ctx, bson.M{"id": p.ID}, p)
	if err != nil || res.MatchedCount == 0 {
		return facturacion.Plan{}, false
	}
	return p, true
}

// --- Suscripciones en Mongo ------------------------------------------------

// MongoSuscripciones persiste una suscripción por organización (colección "suscripciones").
type MongoSuscripciones struct{ c *mongo.Collection }

func NewMongoSuscripciones(db *mongo.Database) *MongoSuscripciones {
	c := db.Collection("suscripciones")
	ctx, cancel := mctx()
	defer cancel()
	_, _ = c.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "orgid", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return &MongoSuscripciones{c: c}
}

func (m *MongoSuscripciones) List() []facturacion.Suscripcion {
	ctx, cancel := mctx()
	defer cancel()
	cur, err := m.c.Find(ctx, bson.M{})
	if err != nil {
		return []facturacion.Suscripcion{}
	}
	var out []facturacion.Suscripcion
	_ = cur.All(ctx, &out)
	if out == nil {
		out = []facturacion.Suscripcion{}
	}
	return out
}

func (m *MongoSuscripciones) ByOrg(orgID string) (facturacion.Suscripcion, bool) {
	ctx, cancel := mctx()
	defer cancel()
	var s facturacion.Suscripcion
	err := m.c.FindOne(ctx, bson.M{"orgid": orgID}).Decode(&s)
	return s, err == nil
}

func (m *MongoSuscripciones) Upsert(s facturacion.Suscripcion) facturacion.Suscripcion {
	if s.Actualizada == "" {
		s.Actualizada = ahoraRFC()
	}
	ctx, cancel := mctx()
	defer cancel()
	_, _ = m.c.ReplaceOne(ctx, bson.M{"orgid": s.OrgID}, s, options.Replace().SetUpsert(true))
	return s
}
