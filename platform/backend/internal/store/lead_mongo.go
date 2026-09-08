package store

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp-platform/internal/lead"
)

// MongoLeads persiste las solicitudes de demo en la colección "leads".
type MongoLeads struct{ c *mongo.Collection }

func NewMongoLeads(db *mongo.Database) *MongoLeads {
	return &MongoLeads{c: db.Collection("leads")}
}

// List devuelve las solicitudes más nuevas primero (bandeja de entrada).
func (m *MongoLeads) List() []lead.Lead {
	ctx, cancel := mctx()
	defer cancel()
	cur, err := m.c.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "creado", Value: -1}}))
	if err != nil {
		return []lead.Lead{}
	}
	var out []lead.Lead
	_ = cur.All(ctx, &out)
	if out == nil {
		out = []lead.Lead{}
	}
	return out
}

func (m *MongoLeads) ByID(id string) (lead.Lead, bool) {
	ctx, cancel := mctx()
	defer cancel()
	var l lead.Lead
	err := m.c.FindOne(ctx, bson.M{"id": id}).Decode(&l)
	return l, err == nil
}

func (m *MongoLeads) Create(l lead.Lead) lead.Lead {
	if l.ID == "" {
		l.ID = randID()
	}
	if l.Creado == "" {
		l.Creado = ahoraRFC()
	}
	l.Actualizado = l.Creado
	ctx, cancel := mctx()
	defer cancel()
	_, _ = m.c.InsertOne(ctx, l)
	return l
}

func (m *MongoLeads) Update(l lead.Lead) (lead.Lead, bool) {
	l.Actualizado = ahoraRFC()
	ctx, cancel := mctx()
	defer cancel()
	res, err := m.c.ReplaceOne(ctx, bson.M{"id": l.ID}, l)
	if err != nil || res.MatchedCount == 0 {
		return lead.Lead{}, false
	}
	return l, true
}
