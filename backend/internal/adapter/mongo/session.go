package mongo

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/authn"
)

// SessionStore persiste sesiones en la colección "sessions".
type SessionStore struct{ c coll[authn.Session] }

// NewSessionStore construye el store de sesiones sobre Mongo.
func NewSessionStore(db *gomongo.Database) *SessionStore {
	return &SessionStore{coll[authn.Session]{db.Collection("sessions")}}
}

func sessionID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *SessionStore) Create(p authn.Principal, hubmyToken string, ttl time.Duration) authn.Session {
	now := time.Now()
	sess := authn.Session{ID: sessionID(), Principal: p, HubmyToken: hubmyToken, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	s.c.insert(sess)
	return sess
}

func (s *SessionStore) Get(id string) (authn.Session, bool) {
	sess, ok := s.c.one(map[string]any{"id": id})
	if !ok {
		return authn.Session{}, false
	}
	if sess.Expired(time.Now()) {
		s.Delete(id)
		return authn.Session{}, false
	}
	return sess, true
}

func (s *SessionStore) Delete(id string) { s.c.del(map[string]any{"id": id}) }

func (s *SessionStore) DeleteByUser(userID string) {
	ctx, cancel := opctx()
	defer cancel()
	_, _ = s.c.c.DeleteMany(ctx, map[string]any{"principal.userid": userID})
}

var _ authn.Store = (*SessionStore)(nil)
