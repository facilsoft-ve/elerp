// Package application contiene los casos de uso de ElERP. Depende solo de los
// puertos del dominio; los adaptadores concretos se inyectan en cmd/api.
package application

import (
	"github.com/mornix/elerp/internal/domain/almacen"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/caja"
	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/cotizacion"
	"github.com/mornix/elerp/internal/domain/cupon"
	"github.com/mornix/elerp/internal/domain/demolead"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/cocina"
	"github.com/mornix/elerp/internal/domain/cuenta"
	"github.com/mornix/elerp/internal/domain/legal"
	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/listaprecio"
	"github.com/mornix/elerp/internal/domain/plantilla"
	"github.com/mornix/elerp/internal/domain/promocion"
	"github.com/mornix/elerp/internal/domain/proveedor"
	"github.com/mornix/elerp/internal/domain/tasa"
	"github.com/mornix/elerp/internal/domain/tesoreria"
	"github.com/mornix/elerp/internal/domain/unidadmedida"
	"github.com/mornix/elerp/internal/domain/venta"
)

// Service agrupa los casos de uso de datos de negocio (Inventario, Fiscal, CRM,
// Tesorería). Nuevos módulos añaden sus puertos aquí.
type Service struct {
	productos      inventario.ProductoRepo
	movimientos    inventario.MovimientoRepo
	transferencias inventario.TransferenciaRepo
	rubros         inventario.RubroRepo
	audit          auditoria.Repository

	documentos   fiscal.Repository
	numerador    fiscal.Numerador
	cuentasCobro fiscal.CuentaCobroRepo
	metodosPago  fiscal.MetodoPagoRepo
	// cierresZ son los cierres fiscales diarios por sede: append-only, derivados
	// del ledger de documentos (ver cierrez.go).
	cierresZ fiscal.CierreZRepo
	clientes cliente.Repository
	// cotizaciones: flujo de venta forma libre (cotización → confirmar → facturar).
	cotizaciones cotizacion.Repository

	// Cajas: cuarto nivel de la jerarquía. Sin sesión de caja abierta no se
	// puede emitir un documento fiscal (ver EmitirFactura).
	cajas    caja.CajaRepo
	cajeros  caja.CajeroRepo
	sesiones caja.SesionRepo

	// Tasa de cambio (R9): histórico de solo-anexado + fuentes externas. La
	// empresa se necesita aquí para saber qué fuente eligió y en qué moneda
	// piensa sus precios (R10).
	tasas    tasa.Repository
	empresas empresa.Repository
	// ventasEnEspera son los carritos apartados del mostrador (flujo 2.6).
	ventasEnEspera venta.Repository
	// cobros es el ledger de solo-anexado de Tesorería: el dinero que entra
	// después de la factura, en una venta a crédito.
	cobros tesoreria.Repository
	// pagosProveedor es el espejo de cobros del lado del pasivo: el dinero que sale
	// hacia un proveedor. También solo-anexado; el saldo de CxP se deriva de él.
	pagosProveedor tesoreria.PagoProveedorRepo
	// Contabilidad: plan de cuentas y libro diario. Los asientos son DERIVADOS de
	// las operaciones, así que viven en el mismo Service que las produce.
	cuentas  contabilidad.CuentaRepo
	asientos contabilidad.AsientoRepo
	// periodos son los cierres de mes contable. Un mes cerrado bloquea el asentado
	// de asientos con fecha dentro (o antes) de él (§7.3, no reabrible).
	periodos    contabilidad.PeriodoRepo
	proveedores []tasa.Proveedor
	sincro      *estadoSincro

	// Compras: maestro de proveedores y órdenes de compra. La recepción de una
	// orden alimenta el ledger de inventario y deriva el asiento de compra.
	provs         proveedor.Repository
	ordenesCompra compra.Repository
	// facturasCompra son las facturas fiscales de los proveedores (append-only):
	// reconocen el IVA crédito fiscal y completan la deuda de CxP (base + IVA).
	facturasCompra compra.FacturaCompraRepo
	// notasCompra son las notas de crédito/débito de PROVEEDOR (append-only): ajustan
	// una factura de compra (NC baja la deuda, ND la sube). Se cablea con
	// ConNotasCompra; ver notas_compra.go.
	notasCompra compra.NotaCompraRepo
	// retenciones son los comprobantes de retención (IVA e ISLR, append-only) en las
	// dos direcciones: recibidas (sobre ventas) y emitidas (sobre compras). Bajan CxC/CxP.
	retenciones fiscal.RetencionRepo
	// dispositivos son los dispositivos fiscales configurados por empresa (config
	// CRUD, soft-disable). La conexión real la hace el agente fiscal local.
	dispositivos fiscal.DispositivoFiscalRepo
	// demoLeads guarda las solicitudes de prueba de la demo (previas al login, sin
	// tenant). Se cablea con ConLeadsDemo; ver demo.go.
	demoLeads demolead.Repository
	// listasPrecio es el maestro de tarifas de precio (venta/compra). Editable, no
	// ledger. Se cablea con ConListasPrecio; ver listaprecio.go.
	listasPrecio listaprecio.Repository
	// cupones es el maestro de cupones de descuento. Editable, no ledger. Se cablea
	// con ConCupones; ver cupon.go.
	cupones cupon.Repository
	// promociones es la biblioteca de promociones (imagen o texto, con vigencia) que
	// alimenta el carrusel de la pantalla del cliente. Editable, no ledger. Se cablea
	// con ConPromociones; ver promocion.go.
	promociones promocion.Repository
	// solicitudes son las solicitudes de presupuesto (RFQ) de Compras: el paso
	// previo a la orden de compra (pedir cotización a varios proveedores, comparar
	// y convertir). Documento de gestión editable, no ledger. Se cablea con
	// ConSolicitudesCompra; ver solicitud_compra.go.
	solicitudes compra.SolicitudCompraRepo
	// unidades es el maestro de unidades de medida (símbolo + categoría) del que el
	// catálogo de producto elige su UnidadBase. Editable, no ledger. Se cablea con
	// ConUnidades; ver unidadmedida.go.
	unidades unidadmedida.Repository
	// almacenes es el maestro de almacenes (depósitos) por sede. Editable, no ledger.
	// Se cablea con ConAlmacenes; ver almacen.go.
	almacenes almacen.Repository
	// modulos es el estado de las Aplicaciones (módulos instalables) por empresa. Se
	// cablea con ConModulos; ver aplicacion.go.
	modulos aplicacion.Repository
	// ia es el proxy de inferencia de la capa IA del asistente (opt-in por empresa).
	// Nil ⇒ no hay IA configurada; el asistente responde solo con la capa mecánica.
	// Se cablea con ConIA; ver asistente.go.
	ia IAProxy
	// legales es la proyección de aceptación de documentos legales (Términos, Privacidad).
	// La fuente de verdad es la auditoría append-only. Se cablea con ConLegal; ver legal.go.
	legales legal.Repository
	// plantillas es el maestro de FORMATOS de documento (factura, cotización, …):
	// el lienzo editable que decide cómo se imprime cada tipo, con selección por
	// sede. Editable, no ledger. Se cablea con ConPlantillas; ver plantilla.go.
	plantillas plantilla.Repository
	// mesas es el maestro de MESAS del salón (módulo Restaurante), con su posición
	// en el mapa. Editable, no ledger. Se cablea con ConMesas; ver mesa.go.
	mesas mesa.Repository
	// planos es la configuración de la GRILLA del salón por sede (filas/columnas y
	// celdas bloqueadas). Se cablea con ConMesas (junto al repo de mesas); ver mesa.go.
	planos mesa.PlanoRepository
	// impresoras es la configuración de la impresora de comandas por sede (módulo
	// Restaurante). Se cablea con ConImpresoras; ver cocina.go.
	impresoras cocina.Repository
	// cuentasMesa son las cuentas de mesa (pedido abierto por mesa) del módulo
	// Restaurante. Operativo, no ledger. Se cablea con ConCuentas; ver cuenta.go.
	cuentasMesa cuenta.Repository
}

// New construye el Service con sus puertos (el orden debe coincidir con cmd/api).
func New(
	productos inventario.ProductoRepo,
	movimientos inventario.MovimientoRepo,
	transferencias inventario.TransferenciaRepo,
	rubros inventario.RubroRepo,
	audit auditoria.Repository,
	documentos fiscal.Repository,
	numerador fiscal.Numerador,
	cuentasCobro fiscal.CuentaCobroRepo,
	metodosPago fiscal.MetodoPagoRepo,
	clientes cliente.Repository,
	cotizaciones cotizacion.Repository,
	cajas caja.CajaRepo,
	cajeros caja.CajeroRepo,
	sesiones caja.SesionRepo,
	tasas tasa.Repository,
	empresas empresa.Repository,
	ventasEnEspera venta.Repository,
	cobros tesoreria.Repository,
	cuentas contabilidad.CuentaRepo,
	asientos contabilidad.AsientoRepo,
	periodos contabilidad.PeriodoRepo,
	provs proveedor.Repository,
	ordenesCompra compra.Repository,
	cierresZ fiscal.CierreZRepo,
	facturasCompra compra.FacturaCompraRepo,
	pagosProveedor tesoreria.PagoProveedorRepo,
	retenciones fiscal.RetencionRepo,
	dispositivos fiscal.DispositivoFiscalRepo,
) *Service {
	return &Service{
		tasas:          tasas,
		empresas:       empresas,
		ventasEnEspera: ventasEnEspera,
		cobros:         cobros,
		pagosProveedor: pagosProveedor,
		cuentas:        cuentas,
		asientos:       asientos,
		periodos:       periodos,
		provs:          provs,
		ordenesCompra:  ordenesCompra,
		facturasCompra: facturasCompra,
		retenciones:    retenciones,
		dispositivos:   dispositivos,
		cierresZ:       cierresZ,
		sincro:         &estadoSincro{},
		productos:      productos,
		movimientos:    movimientos,
		transferencias: transferencias,
		rubros:         rubros,
		audit:          audit,
		documentos:     documentos,
		numerador:      numerador,
		cuentasCobro:   cuentasCobro,
		metodosPago:    metodosPago,
		clientes:       clientes,
		cotizaciones:   cotizaciones,
		cajas:          cajas,
		cajeros:        cajeros,
		sesiones:       sesiones,
	}
}

// ConFuentesDeTasa cablea los proveedores externos de tasa, en orden de
// preferencia (primero el BCV, después el respaldo). Se configura aparte de New
// porque qué fuentes existen lo decide el entorno, no el dominio: sin fuentes,
// el servicio funciona igual y la tasa solo se puede cargar a mano.
func (s *Service) ConFuentesDeTasa(provs ...tasa.Proveedor) *Service {
	for _, p := range provs {
		if p != nil {
			s.proveedores = append(s.proveedores, p)
		}
	}
	return s
}

// Rubros devuelve las categorías de la empresa.
func (s *Service) Rubros(empresaID string) []inventario.Rubro {
	return s.rubros.List(empresaID)
}

// Auditoria devuelve el registro de auditoría de la empresa (solo lectura).
func (s *Service) Auditoria(empresaID string) []auditoria.Evento {
	return s.audit.List(empresaID)
}
