package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mornix/elerp/internal/domain/pedido"
)

func (st *Store) attachPedidos(db *gomongo.Database) {
	st.Pedidos = &PedidoRepo{coll[pedido.Pedido]{db.Collection("pedidos")}}
	st.CanalesPedido = &CanalPedidoRepo{coll[pedido.Canal]{db.Collection("canalespedido")}}
	st.ZonasPedido = &ZonaPedidoRepo{coll[pedido.Zona]{db.Collection("zonaspedido")}}
	st.Repartidores = &RepartidorRepo{coll[pedido.Repartidor]{db.Collection("repartidores")}}
}

// PedidoRepo persiste los pedidos con filtro empresaid obligatorio (Mongo no
// tiene RLS: el aislamiento se hace acá).
type PedidoRepo struct{ c coll[pedido.Pedido] }

func (r *PedidoRepo) Append(p pedido.Pedido) pedido.Pedido {
	if p.ID == "" {
		p.ID = newID("ped_")
	}
	r.c.insert(p)
	return p
}

func (r *PedidoRepo) Update(p pedido.Pedido) (pedido.Pedido, bool) {
	ctx, cancel := opctx()
	defer cancel()
	res, err := r.c.c.ReplaceOne(ctx, map[string]any{"empresaid": p.EmpresaID, "id": p.ID}, p,
		options.Replace().SetUpsert(false))
	if err != nil || res.MatchedCount == 0 {
		return pedido.Pedido{}, false
	}
	return p, true
}

func (r *PedidoRepo) ByID(empresaID, id string) (pedido.Pedido, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

func (r *PedidoRepo) ByReferencia(empresaID, canalID, referencia string) (pedido.Pedido, bool) {
	if referencia == "" {
		return pedido.Pedido{}, false
	}
	return r.c.one(map[string]any{"empresaid": empresaID, "canalid": canalID, "referenciaexterna": referencia})
}

func (r *PedidoRepo) ByTracking(empresaID, tracking string) (pedido.Pedido, bool) {
	if tracking == "" {
		return pedido.Pedido{}, false
	}
	return r.c.one(map[string]any{"empresaid": empresaID, "tracking": tracking})
}

// ByToken NO filtra por empresa: es la búsqueda de la página pública, donde el
// cliente solo trae el token. Por eso el token es aleatorio y no el tracking.
func (r *PedidoRepo) ByToken(token string) (pedido.Pedido, bool) {
	if token == "" {
		return pedido.Pedido{}, false
	}
	return r.c.one(map[string]any{"tokenpublico": token})
}

func (r *PedidoRepo) List(empresaID, sedeID string) []pedido.Pedido {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}

// CanalPedidoRepo, ZonaPedidoRepo y RepartidorRepo son maestros editables
// (Upsert), no ledgers.
type CanalPedidoRepo struct{ c coll[pedido.Canal] }

func (r *CanalPedidoRepo) List(empresaID string) []pedido.Canal {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *CanalPedidoRepo) ByID(empresaID, id string) (pedido.Canal, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}

// ByTokenHash NO filtra por empresa: quien llama es la tienda del cliente, que
// solo trae su token. El token ES la identidad, y de él sale la empresa.
func (r *CanalPedidoRepo) ByTokenHash(hash string) (pedido.Canal, bool) {
	if hash == "" {
		return pedido.Canal{}, false
	}
	return r.c.one(map[string]any{"tokenhash": hash})
}

func (r *CanalPedidoRepo) Upsert(c pedido.Canal) pedido.Canal {
	if c.ID == "" {
		c.ID = newID("can_")
	}
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx, map[string]any{"empresaid": c.EmpresaID, "id": c.ID}, c,
		options.Replace().SetUpsert(true))
	return c
}

type ZonaPedidoRepo struct{ c coll[pedido.Zona] }

func (r *ZonaPedidoRepo) List(empresaID, sedeID string) []pedido.Zona {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}
func (r *ZonaPedidoRepo) ByID(empresaID, id string) (pedido.Zona, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *ZonaPedidoRepo) Upsert(z pedido.Zona) pedido.Zona {
	if z.ID == "" {
		z.ID = newID("zon_")
	}
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx, map[string]any{"empresaid": z.EmpresaID, "id": z.ID}, z,
		options.Replace().SetUpsert(true))
	return z
}

type RepartidorRepo struct{ c coll[pedido.Repartidor] }

func (r *RepartidorRepo) List(empresaID, sedeID string) []pedido.Repartidor {
	f := map[string]any{"empresaid": empresaID}
	if sedeID != "" {
		f["sedeid"] = sedeID
	}
	return r.c.all(f)
}
func (r *RepartidorRepo) ByID(empresaID, id string) (pedido.Repartidor, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *RepartidorRepo) ByUsuario(empresaID, usuarioID string) (pedido.Repartidor, bool) {
	if usuarioID == "" {
		return pedido.Repartidor{}, false
	}
	return r.c.one(map[string]any{"empresaid": empresaID, "usuarioid": usuarioID})
}
func (r *RepartidorRepo) Upsert(x pedido.Repartidor) pedido.Repartidor {
	if x.ID == "" {
		x.ID = newID("rep_")
	}
	ctx, cancel := opctx()
	defer cancel()
	_, _ = r.c.c.ReplaceOne(ctx, map[string]any{"empresaid": x.EmpresaID, "id": x.ID}, x,
		options.Replace().SetUpsert(true))
	return x
}
