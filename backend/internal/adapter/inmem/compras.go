package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

// --- Proveedores (maestro) ---

// ProveedorRepo guarda los proveedores en memoria, aislados por empresaID.
type ProveedorRepo struct {
	mu    sync.Mutex
	items []proveedor.Proveedor
}

// NewProveedorRepo construye el repositorio.
func NewProveedorRepo() *ProveedorRepo { return &ProveedorRepo{} }

func (r *ProveedorRepo) List(empresaID string) []proveedor.Proveedor {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []proveedor.Proveedor{}
	for _, p := range r.items {
		if p.EmpresaID == empresaID {
			out = append(out, p)
		}
	}
	return out
}

func (r *ProveedorRepo) ByID(empresaID, id string) (proveedor.Proveedor, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.ID == id {
			return p, true
		}
	}
	return proveedor.Proveedor{}, false
}

func (r *ProveedorRepo) Create(p proveedor.Proveedor) proveedor.Proveedor {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = nextID("prov_")
	}
	r.items = append(r.items, p)
	return p
}

func (r *ProveedorRepo) Update(p proveedor.Proveedor) (proveedor.Proveedor, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == p.EmpresaID && cur.ID == p.ID {
			r.items[i] = p
			return p, true
		}
	}
	return proveedor.Proveedor{}, false
}

// --- Órdenes de compra (máquina de estados) ---

// OrdenCompraRepo guarda las órdenes de compra en memoria, aisladas por empresaID.
type OrdenCompraRepo struct {
	mu    sync.Mutex
	items []compra.OrdenCompra
}

// NewOrdenCompraRepo construye el repositorio.
func NewOrdenCompraRepo() *OrdenCompraRepo { return &OrdenCompraRepo{} }

func (r *OrdenCompraRepo) List(empresaID string) []compra.OrdenCompra {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []compra.OrdenCompra{}
	for _, o := range r.items {
		if o.EmpresaID == empresaID {
			out = append(out, o)
		}
	}
	return out
}

func (r *OrdenCompraRepo) ByID(empresaID, id string) (compra.OrdenCompra, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, o := range r.items {
		if o.EmpresaID == empresaID && o.ID == id {
			return o, true
		}
	}
	return compra.OrdenCompra{}, false
}

func (r *OrdenCompraRepo) Create(o compra.OrdenCompra) compra.OrdenCompra {
	r.mu.Lock()
	defer r.mu.Unlock()
	if o.ID == "" {
		o.ID = nextID("oc_")
	}
	r.items = append(r.items, o)
	return o
}

func (r *OrdenCompraRepo) Update(o compra.OrdenCompra) (compra.OrdenCompra, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == o.EmpresaID && cur.ID == o.ID {
			r.items[i] = o
			return o, true
		}
	}
	return compra.OrdenCompra{}, false
}

// --- Facturas de compra (append-only) ---

// FacturaCompraRepo guarda las facturas de compra en memoria, aisladas por
// empresaID. Append-only: no expone Update ni Delete.
type FacturaCompraRepo struct {
	mu    sync.Mutex
	items []compra.FacturaCompra
}

// NewFacturaCompraRepo construye el repositorio.
func NewFacturaCompraRepo() *FacturaCompraRepo { return &FacturaCompraRepo{} }

func (r *FacturaCompraRepo) List(empresaID string) []compra.FacturaCompra {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []compra.FacturaCompra{}
	for _, f := range r.items {
		if f.EmpresaID == empresaID {
			out = append(out, f)
		}
	}
	return out
}

func (r *FacturaCompraRepo) ByID(empresaID, id string) (compra.FacturaCompra, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, f := range r.items {
		if f.EmpresaID == empresaID && f.ID == id {
			return f, true
		}
	}
	return compra.FacturaCompra{}, false
}

func (r *FacturaCompraRepo) ByOrden(empresaID, ordenID string) (compra.FacturaCompra, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, f := range r.items {
		if f.EmpresaID == empresaID && f.OrdenCompraID == ordenID {
			return f, true
		}
	}
	return compra.FacturaCompra{}, false
}

func (r *FacturaCompraRepo) Append(f compra.FacturaCompra) compra.FacturaCompra {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f.ID == "" {
		f.ID = nextID("fc_")
	}
	r.items = append(r.items, f)
	return f
}

// --- Notas de crédito/débito de compra (append-only) ---

// NotaCompraRepo guarda las notas de crédito/débito de compra en memoria, aisladas
// por empresaID. Append-only: no expone Update ni Delete.
type NotaCompraRepo struct {
	mu    sync.Mutex
	items []compra.NotaCompra
}

// NewNotaCompraRepo construye el repositorio.
func NewNotaCompraRepo() *NotaCompraRepo { return &NotaCompraRepo{} }

func (r *NotaCompraRepo) List(empresaID string) []compra.NotaCompra {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []compra.NotaCompra{}
	for _, n := range r.items {
		if n.EmpresaID == empresaID {
			out = append(out, n)
		}
	}
	return out
}

func (r *NotaCompraRepo) ByID(empresaID, id string) (compra.NotaCompra, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range r.items {
		if n.EmpresaID == empresaID && n.ID == id {
			return n, true
		}
	}
	return compra.NotaCompra{}, false
}

func (r *NotaCompraRepo) ByFactura(empresaID, facturaID string) []compra.NotaCompra {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []compra.NotaCompra{}
	for _, n := range r.items {
		if n.EmpresaID == empresaID && n.FacturaCompraID == facturaID {
			out = append(out, n)
		}
	}
	return out
}

func (r *NotaCompraRepo) Append(n compra.NotaCompra) compra.NotaCompra {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n.ID == "" {
		n.ID = nextID("ncc_")
	}
	r.items = append(r.items, n)
	return n
}

// --- Solicitudes de presupuesto (RFQ, documento de gestión editable) ---

// SolicitudCompraRepo guarda las solicitudes de presupuesto en memoria, aisladas
// por empresaID.
type SolicitudCompraRepo struct {
	mu    sync.Mutex
	items []compra.SolicitudCompra
}

// NewSolicitudCompraRepo construye el repositorio.
func NewSolicitudCompraRepo() *SolicitudCompraRepo { return &SolicitudCompraRepo{} }

func (r *SolicitudCompraRepo) List(empresaID string) []compra.SolicitudCompra {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []compra.SolicitudCompra{}
	for _, s := range r.items {
		if s.EmpresaID == empresaID {
			out = append(out, s)
		}
	}
	return out
}

func (r *SolicitudCompraRepo) ByID(empresaID, id string) (compra.SolicitudCompra, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.items {
		if s.EmpresaID == empresaID && s.ID == id {
			return s, true
		}
	}
	return compra.SolicitudCompra{}, false
}

func (r *SolicitudCompraRepo) Create(s compra.SolicitudCompra) compra.SolicitudCompra {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		s.ID = nextID("sol_")
	}
	r.items = append(r.items, s)
	return s
}

func (r *SolicitudCompraRepo) Update(s compra.SolicitudCompra) (compra.SolicitudCompra, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == s.EmpresaID && cur.ID == s.ID {
			r.items[i] = s
			return s, true
		}
	}
	return compra.SolicitudCompra{}, false
}
