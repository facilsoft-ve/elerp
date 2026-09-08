package store

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp-platform/internal/operador"
)

// Conectar abre la conexión a Mongo (base de datos propia de la consola) y verifica el
// ping. El core y la consola comparten instancia pero NO base de datos.
func Conectar(uri, db string) (*mongo.Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cli, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := cli.Ping(ctx, nil); err != nil {
		return nil, err
	}
	return cli.Database(db), nil
}

func mctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// --- Operadores en Mongo ---------------------------------------------------

// MongoOperadores persiste operadores en la colección "operadores".
type MongoOperadores struct{ c *mongo.Collection }

// NewMongoOperadores crea el repositorio y asegura el índice único por email.
func NewMongoOperadores(db *mongo.Database) *MongoOperadores {
	c := db.Collection("operadores")
	ctx, cancel := mctx()
	defer cancel()
	_, _ = c.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return &MongoOperadores{c: c}
}

func (m *MongoOperadores) ByEmail(email string) (operador.Operador, bool) {
	ctx, cancel := mctx()
	defer cancel()
	var o operador.Operador
	err := m.c.FindOne(ctx, bson.M{"email": strings.ToLower(strings.TrimSpace(email))}).Decode(&o)
	return o, err == nil
}

func (m *MongoOperadores) ByID(id string) (operador.Operador, bool) {
	ctx, cancel := mctx()
	defer cancel()
	var o operador.Operador
	err := m.c.FindOne(ctx, bson.M{"id": id}).Decode(&o)
	return o, err == nil
}

func (m *MongoOperadores) Create(o operador.Operador) operador.Operador {
	if o.ID == "" {
		o.ID = randID()
	}
	o.Email = strings.ToLower(strings.TrimSpace(o.Email))
	if o.Creado == "" {
		o.Creado = ahoraRFC()
	}
	ctx, cancel := mctx()
	defer cancel()
	_, _ = m.c.InsertOne(ctx, o)
	return o
}

func (m *MongoOperadores) Update(o operador.Operador) (operador.Operador, bool) {
	o.Email = strings.ToLower(strings.TrimSpace(o.Email))
	ctx, cancel := mctx()
	defer cancel()
	res, err := m.c.ReplaceOne(ctx, bson.M{"id": o.ID}, o)
	if err != nil || res.MatchedCount == 0 {
		return operador.Operador{}, false
	}
	return o, true
}

func (m *MongoOperadores) Count() int {
	ctx, cancel := mctx()
	defer cancel()
	n, err := m.c.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0
	}
	return int(n)
}

// --- Sesiones en Mongo -----------------------------------------------------

// MongoSesiones persiste sesiones en la colección "sessions".
type MongoSesiones struct{ c *mongo.Collection }

// NewMongoSesiones crea el store de sesiones.
func NewMongoSesiones(db *mongo.Database) *MongoSesiones {
	return &MongoSesiones{c: db.Collection("sessions")}
}

func (m *MongoSesiones) Create(o operador.Operador, ttl time.Duration) operador.Session {
	now := time.Now().UTC()
	s := operador.Session{
		ID: randID(), OperadorID: o.ID, Email: o.Email, Nombre: o.Nombre,
		CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	ctx, cancel := mctx()
	defer cancel()
	_, _ = m.c.InsertOne(ctx, s)
	return s
}

func (m *MongoSesiones) Get(id string) (operador.Session, bool) {
	ctx, cancel := mctx()
	defer cancel()
	var s operador.Session
	if err := m.c.FindOne(ctx, bson.M{"id": id}).Decode(&s); err != nil {
		return operador.Session{}, false
	}
	if s.Expired(time.Now().UTC()) {
		_, _ = m.c.DeleteOne(ctx, bson.M{"id": id})
		return operador.Session{}, false
	}
	return s, true
}

func (m *MongoSesiones) Delete(id string) {
	ctx, cancel := mctx()
	defer cancel()
	_, _ = m.c.DeleteOne(ctx, bson.M{"id": id})
}
