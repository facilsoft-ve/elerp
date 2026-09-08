package inmem

import (
	"sync"

	"github.com/mornix/elerp/internal/domain/cuenta"
)

// --- Cuentas de mesa (operativo, módulo Restaurante) ---

type CuentaRepo struct {
	mu    sync.Mutex
	items []cuenta.Cuenta
}

func NewCuentaRepo() *CuentaRepo { return &CuentaRepo{} }

func (r *CuentaRepo) Abiertas(empresaID, sedeID string) []cuenta.Cuenta {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []cuenta.Cuenta{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID && (sedeID == "" || c.SedeID == sedeID) && c.Estado == cuenta.EstadoAbierta {
			out = append(out, c)
		}
	}
	return out
}

func (r *CuentaRepo) ByID(empresaID, id string) (cuenta.Cuenta, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.ID == id {
			return c, true
		}
	}
	return cuenta.Cuenta{}, false
}

func (r *CuentaRepo) AbiertaDeMesa(empresaID, mesaID string) (cuenta.Cuenta, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.MesaID == mesaID && c.Estado == cuenta.EstadoAbierta {
			return c, true
		}
	}
	return cuenta.Cuenta{}, false
}

func (r *CuentaRepo) Create(c cuenta.Cuenta) cuenta.Cuenta {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cta_")
	}
	r.items = append(r.items, c)
	return c
}

func (r *CuentaRepo) Update(c cuenta.Cuenta) (cuenta.Cuenta, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.EmpresaID == c.EmpresaID && cur.ID == c.ID {
			r.items[i] = c
			return c, true
		}
	}
	return cuenta.Cuenta{}, false
}
