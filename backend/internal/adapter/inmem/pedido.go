package inmem

import (
	"fmt"
	"sync"

	"github.com/mornix/elerp/internal/domain/pedido"
)

// --- Pedidos de delivery ---

type PedidoRepo struct {
	mu    sync.RWMutex
	items []pedido.Pedido
	seq   int
}

func NewPedidoRepo() *PedidoRepo { return &PedidoRepo{} }

func (r *PedidoRepo) Append(p pedido.Pedido) pedido.Pedido {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		r.seq++
		p.ID = fmt.Sprintf("ped_%d", r.seq)
	}
	r.items = append(r.items, p)
	return p
}

func (r *PedidoRepo) Update(p pedido.Pedido) (pedido.Pedido, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cur := range r.items {
		if cur.ID == p.ID && cur.EmpresaID == p.EmpresaID {
			r.items[i] = p
			return p, true
		}
	}
	return pedido.Pedido{}, false
}

func (r *PedidoRepo) ByID(empresaID, id string) (pedido.Pedido, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.ID == id {
			return p, true
		}
	}
	return pedido.Pedido{}, false
}

func (r *PedidoRepo) ByReferencia(empresaID, canalID, referencia string) (pedido.Pedido, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if referencia == "" {
		return pedido.Pedido{}, false
	}
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.CanalID == canalID && p.ReferenciaExterna == referencia {
			return p, true
		}
	}
	return pedido.Pedido{}, false
}

func (r *PedidoRepo) ByTracking(empresaID, tracking string) (pedido.Pedido, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.items {
		if p.EmpresaID == empresaID && p.Tracking == tracking && tracking != "" {
			return p, true
		}
	}
	return pedido.Pedido{}, false
}

// ByToken NO filtra por empresa: es la búsqueda de la página pública de
// seguimiento, donde el cliente solo trae el token.
func (r *PedidoRepo) ByToken(token string) (pedido.Pedido, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.items {
		if p.TokenPublico == token && token != "" {
			return p, true
		}
	}
	return pedido.Pedido{}, false
}

func (r *PedidoRepo) List(empresaID, sedeID string) []pedido.Pedido {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []pedido.Pedido{}
	for _, p := range r.items {
		if p.EmpresaID == empresaID && (sedeID == "" || p.SedeID == sedeID) {
			out = append(out, p)
		}
	}
	return out
}

// --- Canales conectados ---

type CanalPedidoRepo struct {
	mu    sync.RWMutex
	items []pedido.Canal
	seq   int
}

func NewCanalPedidoRepo() *CanalPedidoRepo { return &CanalPedidoRepo{} }

func (r *CanalPedidoRepo) List(empresaID string) []pedido.Canal {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []pedido.Canal{}
	for _, c := range r.items {
		if c.EmpresaID == empresaID {
			out = append(out, c)
		}
	}
	return out
}

func (r *CanalPedidoRepo) ByID(empresaID, id string) (pedido.Canal, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.items {
		if c.EmpresaID == empresaID && c.ID == id {
			return c, true
		}
	}
	return pedido.Canal{}, false
}

func (r *CanalPedidoRepo) Upsert(c pedido.Canal) pedido.Canal {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		r.seq++
		c.ID = fmt.Sprintf("can_%d", r.seq)
	}
	for i, cur := range r.items {
		if cur.EmpresaID == c.EmpresaID && cur.ID == c.ID {
			r.items[i] = c
			return c
		}
	}
	r.items = append(r.items, c)
	return c
}

// --- Zonas de reparto ---

type ZonaPedidoRepo struct {
	mu    sync.RWMutex
	items []pedido.Zona
	seq   int
}

func NewZonaPedidoRepo() *ZonaPedidoRepo { return &ZonaPedidoRepo{} }

func (r *ZonaPedidoRepo) List(empresaID, sedeID string) []pedido.Zona {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []pedido.Zona{}
	for _, z := range r.items {
		if z.EmpresaID == empresaID && (sedeID == "" || z.SedeID == sedeID) {
			out = append(out, z)
		}
	}
	return out
}

func (r *ZonaPedidoRepo) ByID(empresaID, id string) (pedido.Zona, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, z := range r.items {
		if z.EmpresaID == empresaID && z.ID == id {
			return z, true
		}
	}
	return pedido.Zona{}, false
}

func (r *ZonaPedidoRepo) Upsert(z pedido.Zona) pedido.Zona {
	r.mu.Lock()
	defer r.mu.Unlock()
	if z.ID == "" {
		r.seq++
		z.ID = fmt.Sprintf("zon_%d", r.seq)
	}
	for i, cur := range r.items {
		if cur.EmpresaID == z.EmpresaID && cur.ID == z.ID {
			r.items[i] = z
			return z
		}
	}
	r.items = append(r.items, z)
	return z
}

// --- Repartidores ---

type RepartidorRepo struct {
	mu    sync.RWMutex
	items []pedido.Repartidor
	seq   int
}

func NewRepartidorRepo() *RepartidorRepo { return &RepartidorRepo{} }

func (r *RepartidorRepo) List(empresaID, sedeID string) []pedido.Repartidor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []pedido.Repartidor{}
	for _, x := range r.items {
		if x.EmpresaID == empresaID && (sedeID == "" || x.SedeID == sedeID) {
			out = append(out, x)
		}
	}
	return out
}

func (r *RepartidorRepo) ByID(empresaID, id string) (pedido.Repartidor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, x := range r.items {
		if x.EmpresaID == empresaID && x.ID == id {
			return x, true
		}
	}
	return pedido.Repartidor{}, false
}

func (r *RepartidorRepo) ByUsuario(empresaID, usuarioID string) (pedido.Repartidor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if usuarioID == "" {
		return pedido.Repartidor{}, false
	}
	for _, x := range r.items {
		if x.EmpresaID == empresaID && x.UsuarioID == usuarioID {
			return x, true
		}
	}
	return pedido.Repartidor{}, false
}

func (r *RepartidorRepo) Upsert(x pedido.Repartidor) pedido.Repartidor {
	r.mu.Lock()
	defer r.mu.Unlock()
	if x.ID == "" {
		r.seq++
		x.ID = fmt.Sprintf("rep_%d", r.seq)
	}
	for i, cur := range r.items {
		if cur.EmpresaID == x.EmpresaID && cur.ID == x.ID {
			r.items[i] = x
			return x
		}
	}
	r.items = append(r.items, x)
	return x
}
