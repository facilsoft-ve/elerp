package application

import (
	"encoding/json"

	"github.com/mornix/elerp/internal/domain/almacen"
	"github.com/mornix/elerp/internal/domain/caja"
	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/contabilidad"
	"github.com/mornix/elerp/internal/domain/cotizacion"
	"github.com/mornix/elerp/internal/domain/cupon"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/promocion"
	"github.com/mornix/elerp/internal/domain/proveedor"
	"github.com/mornix/elerp/internal/domain/tesoreria"
	"github.com/mornix/elerp/internal/domain/unidadmedida"
)

// RestaurarDatos importa los DATOS de negocio de un respaldo (ExportarTenant) en una
// empresa recién creada, reescribiendo el EmpresaID de cada registro y CONSERVANDO el
// resto de sus IDs. Como los repos aíslan por (empresaID, id), conservar los IDs mantiene
// las referencias cruzadas (sedeID, productoID, documentoID…) sin remapeo y sin colisión
// con otros tenants. Es una inserción directa (no re-corre lógica de negocio): los ledgers
// append-only se reponen tal cual, preservando sus hashes/sellos. Devuelve el conteo por
// colección restaurada.
func (s *Service) RestaurarDatos(id string, datos map[string]json.RawMessage) map[string]int {
	n := map[string]int{}
	put := func(k string, c int) {
		if c > 0 {
			n[k] = c
		}
	}

	// Inventario y catálogo
	put("productos", restaurarLista(datos["productos"], func(x inventario.Producto) { x.EmpresaID = id; s.productos.Create(x) }))
	put("movimientos", restaurarLista(datos["movimientos"], func(x inventario.Movimiento) { x.EmpresaID = id; s.movimientos.Append(x) }))
	put("transferencias", restaurarLista(datos["transferencias"], func(x inventario.Transferencia) { x.EmpresaID = id; s.transferencias.Create(x) }))
	put("rubros", restaurarLista(datos["rubros"], func(x inventario.Rubro) { x.EmpresaID = id; s.rubros.Create(x) }))
	if s.almacenes != nil {
		put("almacenes", restaurarLista(datos["almacenes"], func(x almacen.Almacen) { x.EmpresaID = id; s.almacenes.Create(x) }))
	}
	if s.unidades != nil {
		put("unidades", restaurarLista(datos["unidades"], func(x unidadmedida.UnidadMedida) { x.EmpresaID = id; s.unidades.Create(x) }))
	}

	// Terceros
	put("clientes", restaurarLista(datos["clientes"], func(x cliente.Cliente) { x.EmpresaID = id; s.clientes.Create(x) }))
	put("proveedores", restaurarLista(datos["proveedores"], func(x proveedor.Proveedor) { x.EmpresaID = id; s.provs.Create(x) }))

	// Fiscal
	put("documentos", restaurarLista(datos["documentos"], func(x fiscal.Documento) { x.EmpresaID = id; s.documentos.Append(x) }))
	put("retenciones", restaurarLista(datos["retenciones"], func(x fiscal.Retencion) { x.EmpresaID = id; s.retenciones.Append(x) }))
	put("dispositivosFiscales", restaurarLista(datos["dispositivosFiscales"], func(x fiscal.DispositivoFiscal) { x.EmpresaID = id; s.dispositivos.Create(x) }))
	put("cuentasCobro", restaurarLista(datos["cuentasCobro"], func(x fiscal.CuentaCobro) { x.EmpresaID = id; s.cuentasCobro.Create(x) }))
	put("metodosPago", restaurarLista(datos["metodosPago"], func(x fiscal.MetodoPago) { x.EmpresaID = id; s.metodosPago.Create(x) }))
	if s.cupones != nil {
		put("cupones", restaurarLista(datos["cupones"], func(x cupon.Cupon) { x.EmpresaID = id; s.cupones.Create(x) }))
	}
	if s.promociones != nil {
		put("promociones", restaurarLista(datos["promociones"], func(x promocion.Promocion) { x.EmpresaID = id; s.promociones.Create(x) }))
	}

	// Contabilidad
	put("planDeCuentas", restaurarLista(datos["planDeCuentas"], func(x contabilidad.Cuenta) { x.EmpresaID = id; s.cuentas.Create(x) }))
	put("libroDiario", restaurarLista(datos["libroDiario"], func(x contabilidad.Asiento) { x.EmpresaID = id; s.asientos.Append(x) }))
	put("periodosCerrados", restaurarLista(datos["periodosCerrados"], func(x contabilidad.Periodo) { x.EmpresaID = id; s.periodos.Append(x) }))

	// Tesorería
	put("cobros", restaurarLista(datos["cobros"], func(x tesoreria.Cobro) { x.EmpresaID = id; s.cobros.Append(x) }))
	put("pagosProveedor", restaurarLista(datos["pagosProveedor"], func(x tesoreria.PagoProveedor) { x.EmpresaID = id; s.pagosProveedor.Append(x) }))

	// Compras y ventas
	put("ordenesCompra", restaurarLista(datos["ordenesCompra"], func(x compra.OrdenCompra) { x.EmpresaID = id; s.ordenesCompra.Create(x) }))
	put("facturasCompra", restaurarLista(datos["facturasCompra"], func(x compra.FacturaCompra) { x.EmpresaID = id; s.facturasCompra.Append(x) }))
	if s.notasCompra != nil {
		put("notasCompra", restaurarLista(datos["notasCompra"], func(x compra.NotaCompra) { x.EmpresaID = id; s.notasCompra.Append(x) }))
	}
	if s.solicitudes != nil {
		put("solicitudesCompra", restaurarLista(datos["solicitudesCompra"], func(x compra.SolicitudCompra) { x.EmpresaID = id; s.solicitudes.Create(x) }))
	}
	put("cotizaciones", restaurarLista(datos["cotizaciones"], func(x cotizacion.Cotizacion) { x.EmpresaID = id; s.cotizaciones.Create(x) }))

	// Caja
	put("cajeros", restaurarLista(datos["cajeros"], func(x caja.Cajero) { x.EmpresaID = id; s.cajeros.Create(x) }))
	put("sesionesCaja", restaurarLista(datos["sesionesCaja"], func(x caja.Sesion) { x.EmpresaID = id; s.sesiones.Create(x) }))

	return n
}

// restaurarLista deserializa una colección del respaldo e inserta cada registro con la
// función dada (que fija el EmpresaID nuevo). El tipo T se infiere de `insert`.
func restaurarLista[T any](raw json.RawMessage, insert func(T)) int {
	if len(raw) == 0 {
		return 0
	}
	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0
	}
	for _, it := range items {
		insert(it)
	}
	return len(items)
}
