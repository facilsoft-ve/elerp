package application

import (
	"sort"

	"github.com/mornix/elerp/internal/domain/auditoria"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
)

// ClonarMaestros copia los MAESTROS de negocio de una empresa origen a la empresa
// destino (un sandbox recién creado por TenancyService.CrearSandboxBase), sin
// tocar ningún LEDGER (documentos, movimientos, asientos, cobros…): el sandbox
// nace con la configuración del cliente pero en blanco para operar.
//
// Referencias cruzadas: Producto.Rubro es un NOMBRE (no id) → no hay remapeo;
// Almacen.SedeID y DispositivoFiscal.SedeID sí son ids de sede → se remapean con
// `sedeMap`. RubrosAdmitidos del almacén se limpia (admite todos) en el sandbox.
// Devuelve el conteo de lo clonado por colección. Los módulos NO se clonan: el
// sandbox arranca con el núcleo y el tester activa lo que quiera probar.
func (s *Service) ClonarMaestros(origenID, destinoID string, sedeMap map[string]string) map[string]int {
	n := map[string]int{}
	clon := func(k string) { n[k]++ }

	for _, r := range s.Rubros(origenID) {
		r.ID, r.EmpresaID = "", destinoID
		s.rubros.Create(r)
		clon("rubros")
	}
	for _, p := range s.Productos(origenID) {
		p.ID, p.EmpresaID = "", destinoID // Rubro va por nombre: no requiere remapeo
		s.productos.Create(p)
		clon("productos")
	}
	for _, a := range s.Almacenes(origenID) {
		a.ID, a.EmpresaID = "", destinoID
		a.SedeID = sedeMap[a.SedeID]
		a.RubrosAdmitidos = nil // sandbox: sin restricción por rubro (evita remapear ids)
		s.almacenes.Create(a)
		clon("almacenes")
	}
	for _, c := range s.Clientes(origenID) {
		c.ID, c.EmpresaID = "", destinoID
		s.clientes.Create(c)
		clon("clientes")
	}
	for _, pr := range s.Proveedores(origenID) {
		pr.ID, pr.EmpresaID = "", destinoID
		s.provs.Create(pr)
		clon("proveedores")
	}
	for _, u := range s.Unidades(origenID) {
		u.ID, u.EmpresaID = "", destinoID
		s.unidades.Create(u)
		clon("unidades")
	}
	for _, c := range s.PlanDeCuentas(origenID) {
		c.ID, c.EmpresaID = "", destinoID // se referencia por Codigo (estable)
		s.cuentas.Create(c)
		clon("cuentas")
	}
	for _, m := range s.MetodosPago(origenID) {
		m.ID, m.EmpresaID = "", destinoID
		s.metodosPago.Create(m)
		clon("metodosPago")
	}
	for _, cc := range s.CuentasCobro(origenID) {
		cc.ID, cc.EmpresaID = "", destinoID
		s.cuentasCobro.Create(cc)
		clon("cuentasCobro")
	}
	for _, d := range s.DispositivosFiscales(origenID) {
		d.ID, d.EmpresaID = "", destinoID
		d.SedeID = sedeMap[d.SedeID] // "" → "" (aplica a toda la empresa)
		s.dispositivos.Create(d)
		clon("dispositivos")
	}
	for _, cu := range s.Cupones(origenID) {
		cu.ID, cu.EmpresaID = "", destinoID
		s.cupones.Create(cu)
		clon("cupones")
	}
	for _, pm := range s.Promociones(origenID) {
		pm.ID, pm.EmpresaID = "", destinoID
		s.promociones.Create(pm)
		clon("promociones")
	}
	return n
}

// ExportarTenant reúne un volcado LÓGICO de todos los datos de negocio de una
// empresa (para respaldo y derecho de salida). Solo lee por métodos públicos ya
// aislados por empresaID, así que nunca cruza tenants. El adaptador HTTP lo
// serializa, comprime y (si hay clave) cifra.
func (s *Service) ExportarTenant(empresaID string) map[string]any {
	return map[string]any{
		// Inventario y catálogo
		"productos":      s.Productos(empresaID),
		"movimientos":    s.Movimientos(empresaID, inventario.FiltroMovimiento{}, "", "", ""),
		"transferencias": s.Transferencias(empresaID),
		"almacenes":      s.Almacenes(empresaID),
		"rubros":         s.Rubros(empresaID),
		"unidades":       s.Unidades(empresaID),
		// Terceros
		"clientes":    s.Clientes(empresaID),
		"proveedores": s.Proveedores(empresaID),
		// Fiscal
		"documentos":           s.Documentos(empresaID),
		"retenciones":          s.Retenciones(empresaID),
		"dispositivosFiscales": s.DispositivosFiscales(empresaID),
		"cuentasCobro":         s.CuentasCobro(empresaID),
		"metodosPago":          s.MetodosPago(empresaID),
		"numeracion":           s.EstadoNumeracion(empresaID),
		"cupones":              s.Cupones(empresaID),
		"promociones":          s.Promociones(empresaID),
		// Contabilidad
		"planDeCuentas":    s.PlanDeCuentas(empresaID),
		"libroDiario":      s.LibroDiario(empresaID),
		"periodosCerrados": s.PeriodosCerrados(empresaID),
		// Tesorería
		"cobros":         s.Cobros(empresaID),
		"pagosProveedor": s.PagosProveedor(empresaID),
		// Compras y ventas
		"ordenesCompra":     s.OrdenesCompra(empresaID),
		"facturasCompra":    s.FacturasCompra(empresaID),
		"notasCompra":       s.NotasCompra(empresaID),
		"solicitudesCompra": s.SolicitudesCompra(empresaID),
		"cotizaciones":      s.Cotizaciones(empresaID),
		// Caja
		"cajeros":      s.ListarCajeros(empresaID),
		"sesionesCaja": s.SesionesDeCaja(empresaID),
		// Config / moneda / módulos / auditoría
		"monedasActivas": s.MonedasActivas(empresaID),
		"modulosActivos": s.ModulosActivos(empresaID),
		"auditoria":      s.Auditoria(empresaID),
	}
}

// FacturasDelMes cuenta las facturas emitidas por una empresa en un mes
// (`mes` = "YYYY-MM"; vacío ⇒ mes en curso UTC). Cuenta la emisión (consumo de
// numeración), incluidas las luego anuladas: el folio se gastó igual.
func (s *Service) FacturasDelMes(empresaID, mes string) int {
	if mes == "" {
		mes = ahora()[:7]
	}
	n := 0
	for _, d := range s.documentos.List(empresaID) {
		if d.Tipo == fiscal.TipoFactura && len(d.Fecha) >= 7 && d.Fecha[:7] == mes {
			n++
		}
	}
	return n
}

// CuotaUso describe el uso de un recurso frente a su tope de plan (0 = ilimitado).
type CuotaUso struct {
	Usado  int  `json:"usado"`
	Limite int  `json:"limite"`
	Excede bool `json:"excede"`
}

// Cuota arma el uso de un recurso; `excede` solo es true si hay tope (>0) superado.
func Cuota(usado, limite int) CuotaUso {
	return CuotaUso{Usado: usado, Limite: limite, Excede: limite > 0 && usado > limite}
}

// Métodos de negocio que consume la CONSOLA DE PLATAFORMA (super-admin / Mornix)
// vía la superficie interna /internal. Se agrupan aquí para no dispersar el
// contrato de plataforma por el resto de la capa de aplicación.

// AuditoriaEntre devuelve el registro de auditoría (log de cambios) de una
// empresa, filtrado por el rango [desde, hasta] y ordenado del más nuevo al más
// viejo, acotado a `limite` entradas (0 ⇒ sin tope).
//
// `desde`/`hasta` aceptan RFC3339 completo o fecha `YYYY-MM-DD`. Como `Fecha` es
// RFC3339 (lexicográficamente ordenable), el filtro es una comparación de string.
// Si `hasta` viene como fecha sin hora, se extiende al fin del día para que el
// rango sea inclusivo del día completo (si no, un evento de las 10:00 quedaría
// fuera de `hasta=YYYY-MM-DD`).
func (s *Service) AuditoriaEntre(empresaID, desde, hasta string, limite int) []auditoria.Evento {
	tope := hasta
	if len(tope) == 10 { // "YYYY-MM-DD" → fin del día
		tope += "T23:59:59Z"
	}
	eventos := s.audit.List(empresaID)
	out := make([]auditoria.Evento, 0, len(eventos))
	for _, e := range eventos {
		if desde != "" && e.Fecha < desde {
			continue
		}
		if tope != "" && e.Fecha > tope {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fecha > out[j].Fecha })
	if limite > 0 && len(out) > limite {
		out = out[:limite]
	}
	return out
}
