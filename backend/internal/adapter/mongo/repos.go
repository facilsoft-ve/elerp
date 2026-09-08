package mongo

import (
	gomongo "go.mongodb.org/mongo-driver/mongo"

	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/legal"
)

// Store agrupa todos los repos Mongo. New lo construye desde una *Database.
type Store struct {
	Productos        *ProductoRepo
	Movimientos      *MovimientoRepo
	Transferencias   *TransferenciaRepo
	Rubros           *RubroRepo
	Audit            *AuditRepo
	Organizaciones   *OrganizacionRepo
	Empresas         *EmpresaRepo
	Sedes            *SedeRepo
	Usuarios         *UsuarioRepo
	Membresias       *MembresiaRepo
	Credenciales     *CredencialRepo
	Clientes         *ClienteRepo
	Documentos       *DocumentoRepo
	Numerador        *NumeradorRepo
	CuentasCobro     *CuentaCobroRepo
	MetodosPago      *MetodoPagoRepo
	Dispositivos     *DispositivoFiscalRepo
	CierresZ         *CierreZRepo
	Cotizaciones     *CotizacionRepo
	Cajas            *CajaRepo
	Cajeros          *CajeroRepo
	SesionesCaja     *SesionCajaRepo
	Tasas            *TasaRepo
	VentasEnEspera   *VentaEnEsperaRepo
	Cobros           *CobroRepo
	PagosProveedor   *PagoProveedorRepo
	CuentasContables *CuentaContableRepo
	Asientos         *AsientoRepo
	Periodos         *PeriodoRepo
	Proveedores      *ProveedorRepo
	OrdenesCompra    *OrdenCompraRepo
	FacturasCompra   *FacturaCompraRepo
	NotasCompra      *NotaCompraRepo
	Solicitudes      *SolicitudCompraRepo
	Retenciones      *RetencionRepo
	ListasPrecio     *ListaPrecioRepo
	Cupones          *CuponRepo
	Promociones      *PromocionRepo
	Unidades         *UnidadMedidaRepo
	Almacenes        *AlmacenRepo
	Modulos          *ModuloRepo
	Legal            *LegalRepo
	Plantillas       *PlantillaRepo
	Mesas            *MesaRepo
	Planos           *PlanoRepo
	Impresoras       *ImpresoraRepo
	Cuentas          *CuentaRepo
}

// New arma el Store cableando cada repo a su colección.
func New(db *gomongo.Database) *Store {
	st := &Store{
		Productos:      &ProductoRepo{coll[inventario.Producto]{db.Collection("productos")}},
		Movimientos:    &MovimientoRepo{coll[inventario.Movimiento]{db.Collection("movimientos")}},
		Transferencias: &TransferenciaRepo{coll[inventario.Transferencia]{db.Collection("transferencias")}},
		Rubros:         &RubroRepo{coll[inventario.Rubro]{db.Collection("rubros")}},
		Audit:          &AuditRepo{coll[auditoria.Evento]{db.Collection("auditoria")}},
		Legal:          &LegalRepo{coll[legal.Aceptacion]{db.Collection("legal_aceptaciones")}},
	}
	st.attachTenancy(db)      // definido en tenancy.go
	st.attachFiscal(db)       // definido en fiscal.go
	st.attachCotizacion(db)   // definido en cotizacion.go
	st.attachCaja(db)         // definido en caja.go
	st.attachTasa(db)         // definido en tasa.go
	st.attachVenta(db)        // definido en venta.go
	st.attachTesoreria(db)    // definido en tesoreria.go
	st.attachContabilidad(db) // definido en contabilidad.go
	st.attachCompras(db)      // definido en compras.go
	st.attachListasPrecio(db) // definido en listaprecio.go
	st.attachCupones(db)      // definido en cupon.go
	st.attachPromociones(db)  // definido en promocion.go
	st.attachUnidades(db)     // definido en unidadmedida.go
	st.attachAlmacenes(db)    // definido en almacen.go
	st.attachModulos(db)      // definido en aplicacion.go
	st.attachPlantillas(db)   // definido en plantilla.go
	st.attachMesas(db)        // definido en mesa.go
	st.attachPlanos(db)       // definido en mesa.go
	st.attachImpresoras(db)   // definido en cocina.go
	st.attachCuentas(db)      // definido en cuenta.go
	return st
}

// --- Productos ---

type ProductoRepo struct{ c coll[inventario.Producto] }

func (r *ProductoRepo) List(empresaID string) []inventario.Producto {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *ProductoRepo) ByID(empresaID, id string) (inventario.Producto, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *ProductoRepo) BySKU(empresaID, sku string) (inventario.Producto, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "sku": sku})
}
func (r *ProductoRepo) Create(p inventario.Producto) inventario.Producto {
	if p.ID == "" {
		p.ID = newID("prod_")
	}
	r.c.insert(p)
	return p
}
func (r *ProductoRepo) Update(p inventario.Producto) (inventario.Producto, bool) {
	if _, ok := r.ByID(p.EmpresaID, p.ID); !ok {
		return inventario.Producto{}, false
	}
	r.c.replace(p.ID, p)
	return p, true
}

// --- Movimientos (ledger solo-anexado) ---

type MovimientoRepo struct{ c coll[inventario.Movimiento] }

func (r *MovimientoRepo) Append(m inventario.Movimiento) inventario.Movimiento {
	if m.ID == "" {
		m.ID = newID("mov_")
	}
	r.c.insert(m)
	return m
}
func (r *MovimientoRepo) List(empresaID string, f inventario.FiltroMovimiento) []inventario.Movimiento {
	filter := map[string]any{"empresaid": empresaID}
	if f.SedeID != "" {
		filter["sedeid"] = f.SedeID
	}
	if f.AlmacenID != "" {
		filter["almacenid"] = f.AlmacenID
	}
	if f.ProductoID != "" {
		filter["productoid"] = f.ProductoID
	}
	if f.SKU != "" {
		filter["sku"] = f.SKU
	}
	return r.c.all(filter)
}

// --- Transferencias ---

type TransferenciaRepo struct {
	c coll[inventario.Transferencia]
}

func (r *TransferenciaRepo) List(empresaID string) []inventario.Transferencia {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *TransferenciaRepo) ByID(empresaID, id string) (inventario.Transferencia, bool) {
	return r.c.one(map[string]any{"empresaid": empresaID, "id": id})
}
func (r *TransferenciaRepo) Create(t inventario.Transferencia) inventario.Transferencia {
	if t.ID == "" {
		t.ID = newID("trf_")
	}
	r.c.insert(t)
	return t
}
func (r *TransferenciaRepo) Update(t inventario.Transferencia) (inventario.Transferencia, bool) {
	if _, ok := r.ByID(t.EmpresaID, t.ID); !ok {
		return inventario.Transferencia{}, false
	}
	r.c.replace(t.ID, t)
	return t, true
}

// --- Rubros ---

type RubroRepo struct{ c coll[inventario.Rubro] }

func (r *RubroRepo) List(empresaID string) []inventario.Rubro {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
func (r *RubroRepo) Create(x inventario.Rubro) inventario.Rubro {
	if x.ID == "" {
		x.ID = newID("rub_")
	}
	r.c.insert(x)
	return x
}

// --- Auditoría (append-only) ---

type AuditRepo struct{ c coll[auditoria.Evento] }

func (r *AuditRepo) Append(e auditoria.Evento) auditoria.Evento {
	if e.ID == "" {
		e.ID = newID("aud_")
	}
	r.c.insert(e)
	return e
}
func (r *AuditRepo) List(empresaID string) []auditoria.Evento {
	return r.c.all(map[string]any{"empresaid": empresaID})
}
