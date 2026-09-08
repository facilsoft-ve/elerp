package inmem

import (
	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/caja"
	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/cotizacion"
	"github.com/mornix/elerp/internal/domain/credencial"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/listaprecio"
	"github.com/mornix/elerp/internal/domain/organizacion"
	"github.com/mornix/elerp/internal/domain/promocion"
	"github.com/mornix/elerp/internal/domain/proveedor"
	"github.com/mornix/elerp/internal/domain/sede"
	"github.com/mornix/elerp/internal/domain/tasa"
	"github.com/mornix/elerp/internal/domain/tesoreria"
	"github.com/mornix/elerp/internal/domain/unidadmedida"
	"github.com/mornix/elerp/internal/domain/usuario"
	"github.com/mornix/elerp/internal/domain/venta"
)

// Aserciones de que cada repo in-memory implementa su puerto de dominio.
var (
	_ inventario.ProductoRepo      = (*ProductoRepo)(nil)
	_ inventario.MovimientoRepo    = (*MovimientoRepo)(nil)
	_ inventario.TransferenciaRepo = (*TransferenciaRepo)(nil)
	_ inventario.RubroRepo         = (*RubroRepo)(nil)
	_ auditoria.Repository         = (*AuditRepo)(nil)
	_ organizacion.Repository      = (*OrganizacionRepo)(nil)
	_ empresa.Repository           = (*EmpresaRepo)(nil)
	_ sede.Repository              = (*SedeRepo)(nil)
	_ usuario.UsuarioRepo          = (*UsuarioRepo)(nil)
	_ usuario.MembresiaRepo        = (*MembresiaRepo)(nil)
	_ credencial.Repository        = (*CredencialRepo)(nil)
	_ cliente.Repository           = (*ClienteRepo)(nil)
	_ listaprecio.Repository       = (*ListaPrecioRepo)(nil)
	_ fiscal.Repository            = (*DocumentoRepo)(nil)
	_ fiscal.Numerador             = (*NumeradorRepo)(nil)
	_ fiscal.CuentaCobroRepo       = (*CuentaCobroRepo)(nil)
	_ fiscal.MetodoPagoRepo        = (*MetodoPagoRepo)(nil)
	_ fiscal.CierreZRepo           = (*CierreZRepo)(nil)
	_ fiscal.RetencionRepo         = (*RetencionRepo)(nil)
	_ fiscal.DispositivoFiscalRepo = (*DispositivoFiscalRepo)(nil)
	_ cotizacion.Repository        = (*CotizacionRepo)(nil)
	_ caja.CajaRepo                = (*CajaRepo)(nil)
	_ caja.CajeroRepo              = (*CajeroRepo)(nil)
	_ caja.SesionRepo              = (*SesionCajaRepo)(nil)
	_ tasa.Repository              = (*TasaRepo)(nil)
	_ venta.Repository             = (*VentaEnEsperaRepo)(nil)
	_ tesoreria.Repository         = (*CobroRepo)(nil)
	_ tesoreria.PagoProveedorRepo  = (*PagoProveedorRepo)(nil)
	_ contabilidad.CuentaRepo      = (*CuentaContableRepo)(nil)
	_ contabilidad.AsientoRepo     = (*AsientoRepo)(nil)
	_ contabilidad.PeriodoRepo     = (*PeriodoRepo)(nil)
	_ proveedor.Repository         = (*ProveedorRepo)(nil)
	_ compra.Repository            = (*OrdenCompraRepo)(nil)
	_ compra.FacturaCompraRepo     = (*FacturaCompraRepo)(nil)
	_ compra.NotaCompraRepo        = (*NotaCompraRepo)(nil)
	_ compra.SolicitudCompraRepo   = (*SolicitudCompraRepo)(nil)
	_ promocion.Repository         = (*PromocionRepo)(nil)
	_ unidadmedida.Repository      = (*UnidadMedidaRepo)(nil)
)
