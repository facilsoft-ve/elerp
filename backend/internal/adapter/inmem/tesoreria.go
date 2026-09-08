package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/tesoreria"
)

// CobroRepo es el ledger de cobros en memoria. SOLO-ANEXADO, como el del dominio.
type CobroRepo struct {
	mu    sync.Mutex
	items []tesoreria.Cobro
}

// NewCobroRepo construye el repositorio.
func NewCobroRepo() *CobroRepo { return &CobroRepo{} }

func (r *CobroRepo) Append(c tesoreria.Cobro) tesoreria.Cobro {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cob_")
	}
	r.items = append(r.items, c)
	return c
}

func (r *CobroRepo) List(empresaID string) []tesoreria.Cobro {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []tesoreria.Cobro{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CobroRepo) PorDocumento(empresaID, documentoID string) []tesoreria.Cobro {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []tesoreria.Cobro{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.DocumentoID == documentoID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CobroRepo) ByID(empresaID, id string) (tesoreria.Cobro, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.ID == id {
			return c, true
		}
	}
	return tesoreria.Cobro{}, false
}

// PagoProveedorRepo es el ledger de pagos a proveedor en memoria. SOLO-ANEXADO,
// como el del dominio: espejo de CobroRepo del lado del pasivo.
type PagoProveedorRepo struct {
	mu    sync.Mutex
	items []tesoreria.PagoProveedor
}

// NewPagoProveedorRepo construye el repositorio.
func NewPagoProveedorRepo() *PagoProveedorRepo { return &PagoProveedorRepo{} }

func (r *PagoProveedorRepo) Append(p tesoreria.PagoProveedor) tesoreria.PagoProveedor {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = nextID("pgp_")
	}
	r.items = append(r.items, p)
	return p
}

func (r *PagoProveedorRepo) List(empresaID string) []tesoreria.PagoProveedor {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []tesoreria.PagoProveedor{}
	for _, p := range r.items {
		if p.EmpresaID == empresaID {
			out = append(out, p)
		}
	}
	return out
}

func (r *PagoProveedorRepo) ByID(empresaID, id string) (tesoreria.PagoProveedor, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.ID == id {
			return p, true
		}
	}
	return tesoreria.PagoProveedor{}, false
}
