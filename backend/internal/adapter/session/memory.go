// Package session implementa authn.Store en memoria (sesiones efímeras).
package session

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/mornix/elerp/internal/domain/authn"
)

type Memory struct {
	mu    sync.Mutex
	items map[string]authn.Session
}

// NewMemory crea un store de sesiones en memoria.
func NewMemory() *Memory { return &Memory{items: map[string]authn.Session{}} }

func newSessionID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *Memory) Create(p authn.Principal, hubmyToken string, ttl time.Duration) authn.Session {
	now := time.Now()
	sess := authn.Session{
		ID: newSessionID(), Principal: p, HubmyToken: hubmyToken,
		CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	m.mu.Lock()
	m.items[sess.ID] = sess
	m.mu.Unlock()
	return sess
}

func (m *Memory) Get(id string) (authn.Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.items[id]
	if !ok {
		return authn.Session{}, false
	}
	if sess.Expired(time.Now()) {
		delete(m.items, id)
		return authn.Session{}, false
	}
	return sess, true
}

func (m *Memory) Delete(id string) {
	m.mu.Lock()
	delete(m.items, id)
	m.mu.Unlock()
}

func (m *Memory) DeleteByUser(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.items {
		if s.Principal.UserID == userID {
			delete(m.items, id)
		}
	}
}

var _ authn.Store = (*Memory)(nil)
