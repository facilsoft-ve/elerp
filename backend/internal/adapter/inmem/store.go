// Package inmem implementa los puertos del dominio en memoria. Sirve para
// desarrollo/seed y como fuente de siembra del adaptador Mongo. Es efímero.
package inmem

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/inventario"
)

var seq int64

func nextID(prefix string) string { return prefix + strconv.FormatInt(atomic.AddInt64(&seq, 1), 10) }

// --- Productos ---

type ProductoRepo struct {
	mu    sync.Mutex
	items []inventario.Producto
}

func NewProductoRepo() *ProductoRepo { return &ProductoRepo{} }

func (r *ProductoRepo) List(empresaID string) []inventario.Producto {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []inventario.Producto{}
	for _, p := range r.items {
		if p.EmpresaID == empresaID {
			out = append(out, p)
		}
	}
	return out
}

func (r *ProductoRepo) ByID(empresaID, id string) (inventario.Producto, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.ID == id {
			return p, true
		}
	}
	return inventario.Producto{}, false
}

func (r *ProductoRepo) BySKU(empresaID, sku string) (inventario.Producto, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.SKU == sku {
			return p, true
		}
	}
	return inventario.Producto{}, false
}

func (r *ProductoRepo) Create(p inventario.Producto) inventario.Producto {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = nextID("prod_")
	}
	r.items = append(r.items, p)
	return p
}

func (r *ProductoRepo) Update(p inventario.Producto) (inventario.Producto, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == p.EmpresaID && cur.ID == p.ID {
			r.items[i] = p
			return p, true
		}
	}
	return inventario.Producto{}, false
}

// --- Movimientos (ledger append-only) ---

type MovimientoRepo struct {
	mu    sync.Mutex
	items []inventario.Movimiento
}

func NewMovimientoRepo() *MovimientoRepo { return &MovimientoRepo{} }

func (r *MovimientoRepo) Append(m inventario.Movimiento) inventario.Movimiento {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ID == "" {
		m.ID = nextID("mov_")
	}
	r.items = append(r.items, m)
	return m
}

func (r *MovimientoRepo) List(empresaID string, f inventario.FiltroMovimiento) []inventario.Movimiento {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []inventario.Movimiento{}
	for _, m := range r.items {
		if m.EmpresaID != empresaID {
			continue
		}
		if f.SedeID != "" && m.SedeID != f.SedeID {
			continue
		}
		if f.AlmacenID != "" && m.AlmacenID != f.AlmacenID {
			continue
		}
		if f.ProductoID != "" && m.ProductoID != f.ProductoID {
			continue
		}
		if f.SKU != "" && m.SKU != f.SKU {
			continue
		}
		out = append(out, m)
	}
	return out
}

// --- Transferencias ---

type TransferenciaRepo struct {
	mu    sync.Mutex
	items []inventario.Transferencia
}

func NewTransferenciaRepo() *TransferenciaRepo { return &TransferenciaRepo{} }

func (r *TransferenciaRepo) List(empresaID string) []inventario.Transferencia {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []inventario.Transferencia{}
	for _, t := range r.items {
		if t.EmpresaID == empresaID {
			out = append(out, t)
		}
	}
	return out
}

func (r *TransferenciaRepo) ByID(empresaID, id string) (inventario.Transferencia, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.items {
		if t.EmpresaID == empresaID && t.ID == id {
			return t, true
		}
	}
	return inventario.Transferencia{}, false
}

func (r *TransferenciaRepo) Create(t inventario.Transferencia) inventario.Transferencia {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = nextID("trf_")
	}
	r.items = append(r.items, t)
	return t
}

func (r *TransferenciaRepo) Update(t inventario.Transferencia) (inventario.Transferencia, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == t.EmpresaID && cur.ID == t.ID {
			r.items[i] = t
			return t, true
		}
	}
	return inventario.Transferencia{}, false
}

// --- Rubros ---

type RubroRepo struct {
	mu    sync.Mutex
	items []inventario.Rubro
}

func NewRubroRepo() *RubroRepo { return &RubroRepo{} }

func (r *RubroRepo) List(empresaID string) []inventario.Rubro {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []inventario.Rubro{}
	for _, x := range r.items {
		if x.EmpresaID == empresaID {
			out = append(out, x)
		}
	}
	return out
}

func (r *RubroRepo) Create(x inventario.Rubro) inventario.Rubro {
	r.mu.Lock()
	defer r.mu.Unlock()
	if x.ID == "" {
		x.ID = nextID("rub_")
	}
	r.items = append(r.items, x)
	return x
}

// --- Auditoría (append-only) ---

type AuditRepo struct {
	mu    sync.Mutex
	items []auditoria.Evento
}

func NewAuditRepo() *AuditRepo { return &AuditRepo{} }

func (r *AuditRepo) Append(e auditoria.Evento) auditoria.Evento {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.ID == "" {
		e.ID = nextID("aud_")
	}
	r.items = append(r.items, e)
	return e
}

func (r *AuditRepo) List(empresaID string) []auditoria.Evento {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []auditoria.Evento{}
	for _, e := range r.items {
		if e.EmpresaID == empresaID {
			out = append(out, e)
		}
	}
	return out
}
