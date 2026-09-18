package inmem

import (
	"fmt"
	"sync"

	fd "github.com/mornix/elerp/internal/domain/facturaciondigital"
)

// --- Configuración de facturación digital (una por empresa) ---

type ConfigDigitalRepo struct {
	mu    sync.RWMutex
	items []fd.Config
}

func NewConfigDigitalRepo() *ConfigDigitalRepo { return &ConfigDigitalRepo{} }

func (r *ConfigDigitalRepo) Get(empresaID string) (fd.Config, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			return c, true
		}
	}
	return fd.Config{}, false
}

func (r *ConfigDigitalRepo) Upsert(c fd.Config) fd.Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == c.EmpresaID {
			r.items[i] = c
			return c
		}
	}
	r.items = append(r.items, c)
	return c
}

// --- Outbox de emisiones ---

type EmisionDigitalRepo struct {
	mu    sync.RWMutex
	items []fd.Emision
	seq   int
}

func NewEmisionDigitalRepo() *EmisionDigitalRepo { return &EmisionDigitalRepo{} }

func (r *EmisionDigitalRepo) Append(e fd.Emision) fd.Emision {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.ID == "" {
		r.seq++
		e.ID = fmt.Sprintf("emi_%d", r.seq)
	}
	r.items = append(r.items, e)
	return e
}

func (r *EmisionDigitalRepo) Update(e fd.Emision) (fd.Emision, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.ID == e.ID && cur.EmpresaID == e.EmpresaID {
			r.items[i] = e
			return e, true
		}
	}
	return fd.Emision{}, false
}

func (r *EmisionDigitalRepo) ByID(empresaID, id string) (fd.Emision, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.items {
		if e.EmpresaID == empresaID && e.ID == id {
			return e, true
		}
	}
	return fd.Emision{}, false
}

func (r *EmisionDigitalRepo) ByDocumento(empresaID, documentoID string) (fd.Emision, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.items {
		if e.EmpresaID == empresaID && e.DocumentoID == documentoID {
			return e, true
		}
	}
	return fd.Emision{}, false
}

// ByToken NO filtra por empresa a propósito: es la búsqueda de la página
// pública, donde el visitante solo trae el token y no sabe de qué empresa es.
// Por eso el token tiene que ser impredecible (ver el dominio).
func (r *EmisionDigitalRepo) ByToken(token string) (fd.Emision, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.items {
		if e.Token == token && token != "" {
			return e, true
		}
	}
	return fd.Emision{}, false
}

func (r *EmisionDigitalRepo) List(empresaID string) []fd.Emision {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []fd.Emision{}
	for _, e := range r.items {
		if e.EmpresaID == empresaID {
			out = append(out, e)
		}
	}
	return out
}

func (r *EmisionDigitalRepo) Pendientes(limite int) []fd.Emision {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []fd.Emision{}
	for _, e := range r.items {
		if !e.Terminada() && len(out) < limite {
			out = append(out, e)
		}
	}
	return out
}
