// Package store implementa la persistencia del BFF: operadores y sesiones, en memoria
// (dev/preview) o en Mongo (producción, base de datos aparte del core).
package store

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/mornix/elerp-platform/internal/operador"
)

// randID devuelve un identificador aleatorio de 256 bits en hex (para ids de sesión y
// de operador). Es opaco y no adivinable.
func randID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func ahoraRFC() string { return time.Now().UTC().Format(time.RFC3339) }

// --- Operadores en memoria -------------------------------------------------

// MemOperadores es el repositorio de operadores en memoria.
type MemOperadores struct {
	mu    sync.Mutex
	items map[string]operador.Operador // por ID
}

// NewMemOperadores crea un repositorio de operadores vacío.
func NewMemOperadores() *MemOperadores {
	return &MemOperadores{items: map[string]operador.Operador{}}
}

func (m *MemOperadores) ByEmail(email string) (operador.Operador, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range m.items {
		if o.Email == email {
			return o, true
		}
	}
	return operador.Operador{}, false
}

func (m *MemOperadores) ByID(id string) (operador.Operador, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.items[id]
	return o, ok
}

func (m *MemOperadores) Create(o operador.Operador) operador.Operador {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o.ID == "" {
		o.ID = randID()
	}
	o.Email = strings.ToLower(strings.TrimSpace(o.Email))
	if o.Creado == "" {
		o.Creado = ahoraRFC()
	}
	m.items[o.ID] = o
	return o
}

func (m *MemOperadores) Update(o operador.Operador) (operador.Operador, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[o.ID]; !ok {
		return operador.Operador{}, false
	}
	o.Email = strings.ToLower(strings.TrimSpace(o.Email))
	m.items[o.ID] = o
	return o, true
}

func (m *MemOperadores) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

// --- Sesiones en memoria ---------------------------------------------------

// MemSesiones es el store de sesiones en memoria (expira perezosamente en Get).
type MemSesiones struct {
	mu    sync.Mutex
	items map[string]operador.Session
}

// NewMemSesiones crea un store de sesiones vacío.
func NewMemSesiones() *MemSesiones {
	return &MemSesiones{items: map[string]operador.Session{}}
}

func (m *MemSesiones) Create(o operador.Operador, ttl time.Duration) operador.Session {
	now := time.Now().UTC()
	s := operador.Session{
		ID: randID(), OperadorID: o.ID, Email: o.Email, Nombre: o.Nombre,
		CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	m.mu.Lock()
	m.items[s.ID] = s
	m.mu.Unlock()
	return s
}

func (m *MemSesiones) Get(id string) (operador.Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.items[id]
	if !ok {
		return operador.Session{}, false
	}
	if s.Expired(time.Now().UTC()) {
		delete(m.items, id)
		return operador.Session{}, false
	}
	return s, true
}

func (m *MemSesiones) Delete(id string) {
	m.mu.Lock()
	delete(m.items, id)
	m.mu.Unlock()
}
