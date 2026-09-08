package inmem

import (
	"github.com/mornix/elerp/internal/domain/sello"
	"sync"

	"github.com/mornix/elerp/internal/domain/contabilidad"
)

// CuentaContableRepo es el plan de cuentas en memoria.
type CuentaContableRepo struct {
	mu    sync.Mutex
	items []contabilidad.Cuenta
}

// NewCuentaContableRepo construye el repositorio.
func NewCuentaContableRepo() *CuentaContableRepo { return &CuentaContableRepo{} }

func (r *CuentaContableRepo) List(empresaID string) []contabilidad.Cuenta {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []contabilidad.Cuenta{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CuentaContableRepo) ByCodigo(empresaID, codigo string) (contabilidad.Cuenta, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.Codigo == codigo {
			return c, true
		}
	}
	return contabilidad.Cuenta{}, false
}

func (r *CuentaContableRepo) Create(c contabilidad.Cuenta) contabilidad.Cuenta {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = nextID("cta_")
	}
	r.items = append(r.items, c)
	return c
}

func (r *CuentaContableRepo) Update(c contabilidad.Cuenta) (contabilidad.Cuenta, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.items {
		if x.ID == c.ID && x.EmpresaID == c.EmpresaID {
			r.items[i] = c
			return c, true
		}
	}
	return contabilidad.Cuenta{}, false
}

// AsientoRepo es el libro diario en memoria. SOLO-ANEXADO.
type AsientoRepo struct {
	mu    sync.Mutex
	items []contabilidad.Asiento
}

// NewAsientoRepo construye el repositorio.
func NewAsientoRepo() *AsientoRepo { return &AsientoRepo{} }

func (r *AsientoRepo) Append(a contabilidad.Asiento) contabilidad.Asiento {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.ID == "" {
		a.ID = nextID("as_")
	}
	prev := ""
	for i := len(r.items) - 1; i >= 0; i-- {
		if r.items[i].EmpresaID == a.EmpresaID {
			prev = r.items[i].Hash
			break
		}
	}
	a.PrevHash = prev
	a.Hash = sello.Encadenar(prev, a.Contenido())
	r.items = append(r.items, a)
	return a
}

func (r *AsientoRepo) List(empresaID string) []contabilidad.Asiento {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []contabilidad.Asiento{}
	for _, a := range r.items {
		if a.EmpresaID == empresaID {
			out = append(out, a)
		}
	}
	return out
}

func (r *AsientoRepo) ByID(empresaID, id string) (contabilidad.Asiento, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.items {
		if a.EmpresaID == empresaID && a.ID == id {
			return a, true
		}
	}
	return contabilidad.Asiento{}, false
}

func (r *AsientoRepo) PorRef(empresaID, refTipo, refID string) []contabilidad.Asiento {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []contabilidad.Asiento{}
	for _, a := range r.items {
		if a.EmpresaID == empresaID && a.RefTipo == refTipo && a.RefID == refID {
			out = append(out, a)
		}
	}
	return out
}

// SiguienteNumero da el correlativo del libro por empresa.
func (r *AsientoRepo) SiguienteNumero(empresaID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	max := 0
	for _, a := range r.items {
		if a.EmpresaID == empresaID && a.Numero > max {
			max = a.Numero
		}
	}
	return max + 1
}

// PeriodoRepo son los cierres de período en memoria. SOLO-ANEXADO.
type PeriodoRepo struct {
	mu    sync.Mutex
	items []contabilidad.Periodo
}

// NewPeriodoRepo construye el repositorio.
func NewPeriodoRepo() *PeriodoRepo { return &PeriodoRepo{} }

func (r *PeriodoRepo) List(empresaID string) []contabilidad.Periodo {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []contabilidad.Periodo{}
	for _, p := range r.items {
		if p.EmpresaID == empresaID {
			out = append(out, p)
		}
	}
	return out
}

func (r *PeriodoRepo) Append(p contabilidad.Periodo) contabilidad.Periodo {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = nextID("per_")
	}
	r.items = append(r.items, p)
	return p
}
