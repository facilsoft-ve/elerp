package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/proveedor"
)

func (st *Store) attachCompras(db *gomongo.Database) {
	st.Proveedores = &ProveedorRepo{coll[proveedor.Proveedor]{db.Collection("proveedores")}}
	st.OrdenesCompra = &OrdenCompraRepo{coll[compra.OrdenCompra]{db.Collection("ordenescompra")}}
	st.FacturasCompra = &FacturaCompraRepo{coll[compra.FacturaCompra]{db.Collection("facturascompra")}}
	st.NotasCompra = &NotaCompraRepo{coll[compra.NotaCompra]{db.Collection("notascompra")}}
	st.Solicitudes = &SolicitudCompraRepo{coll[compra.SolicitudCompra]{db.Collection("solicitudescompra")}}
}

// --- Proveedores (maestro) ---

// ProveedorRepo persiste proveedores con filtro empresaid obligatorio.
type ProveedorRepo struct{ c coll[proveedor.Proveedor] }

func (r *ProveedorRepo) List(empresaID string) []proveedor.Proveedor {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *ProveedorRepo) ByID(empresaID, id string) (proveedor.Proveedor, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *ProveedorRepo) Create(p proveedor.Proveedor) proveedor.Proveedor {
	if p.ID == "" {
		p.ID = newID("prov_")
	}
	r.c.insert(p)
	return p
}
func (r *ProveedorRepo) Update(p proveedor.Proveedor) (proveedor.Proveedor, bool) {
	if _, ok := r.ByID(p.EmpresaID, p.ID); !ok {
		return proveedor.Proveedor{}, false
	}
	r.c.replace(p.ID, p)
	return p, true
}

// --- Órdenes de compra (máquina de estados) ---

// OrdenCompraRepo persiste órdenes de compra con filtro empresaid obligatorio.
type OrdenCompraRepo struct{ c coll[compra.OrdenCompra] }

func (r *OrdenCompraRepo) List(empresaID string) []compra.OrdenCompra {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *OrdenCompraRepo) ByID(empresaID, id string) (compra.OrdenCompra, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *OrdenCompraRepo) Create(o compra.OrdenCompra) compra.OrdenCompra {
	if o.ID == "" {
		o.ID = newID("oc_")
	}
	r.c.insert(o)
	return o
}
func (r *OrdenCompraRepo) Update(o compra.OrdenCompra) (compra.OrdenCompra, bool) {
	if _, ok := r.ByID(o.EmpresaID, o.ID); !ok {
		return compra.OrdenCompra{}, false
	}
	r.c.replace(o.ID, o)
	return o, true
}

// --- Facturas de compra (append-only) ---

// FacturaCompraRepo persiste facturas de compra con filtro empresaid obligatorio.
// Append-only: no expone Update ni Delete.
type FacturaCompraRepo struct{ c coll[compra.FacturaCompra] }

func (r *FacturaCompraRepo) List(empresaID string) []compra.FacturaCompra {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *FacturaCompraRepo) ByID(empresaID, id string) (compra.FacturaCompra, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *FacturaCompraRepo) ByOrden(empresaID, ordenID string) (compra.FacturaCompra, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "ordencompraid": ordenID})
}
func (r *FacturaCompraRepo) Append(f compra.FacturaCompra) compra.FacturaCompra {
	if f.ID == "" {
		f.ID = newID("fc_")
	}
	r.c.insert(f)
	return f
}

// --- Notas de crédito/débito de compra (append-only) ---

// NotaCompraRepo persiste notas de crédito/débito de compra con filtro empresaid
// obligatorio. Append-only: no expone Update ni Delete.
type NotaCompraRepo struct{ c coll[compra.NotaCompra] }

func (r *NotaCompraRepo) List(empresaID string) []compra.NotaCompra {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *NotaCompraRepo) ByID(empresaID, id string) (compra.NotaCompra, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *NotaCompraRepo) ByFactura(empresaID, facturaID string) []compra.NotaCompra {
	return r.c.all(map[string]any{"empresaid": empresaID, "facturacompraid": facturaID})
}
func (r *NotaCompraRepo) Append(n compra.NotaCompra) compra.NotaCompra {
	if n.ID == "" {
		n.ID = newID("ncc_")
	}
	r.c.insert(n)
	return n
}

// --- Solicitudes de presupuesto (RFQ, documento de gestión editable) ---

// SolicitudCompraRepo persiste solicitudes de presupuesto con filtro empresaid
// obligatorio.
type SolicitudCompraRepo struct{ c coll[compra.SolicitudCompra] }

func (r *SolicitudCompraRepo) List(empresaID string) []compra.SolicitudCompra {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *SolicitudCompraRepo) ByID(empresaID, id string) (compra.SolicitudCompra, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *SolicitudCompraRepo) Create(s compra.SolicitudCompra) compra.SolicitudCompra {
	if s.ID == "" {
		s.ID = newID("sol_")
	}
	r.c.insert(s)
	return s
}
func (r *SolicitudCompraRepo) Update(s compra.SolicitudCompra) (compra.SolicitudCompra, bool) {
	if _, ok := r.ByID(s.EmpresaID, s.ID); !ok {
		return compra.SolicitudCompra{}, false
	}
	r.c.replace(s.ID, s)
	return s, true
}
