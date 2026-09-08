package inmem

import (
	"fmt"
	"strings"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/caja"
	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/cocina"
	"github.com/mornix/elerp/internal/domain/credencial"
	"github.com/mornix/elerp/internal/domain/cuenta"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/organizacion"
	"github.com/mornix/elerp/internal/domain/proveedor"
	"github.com/mornix/elerp/internal/domain/sede"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// DEMOS POR NICHO
//
// Además de la bodega demo (emp_demo, ver seedDemo), el modo demo trae una empresa por
// RUBRO para que el prospecto vea el ERP con datos parecidos a los de su negocio. El
// usuario demo es Dueña de todas, así que el SELECTOR DE EMPRESA que ya existe hace de
// selector de rubro: no hace falta un flujo nuevo, y de paso se demuestra el
// multi-empresa.
//
// Alcance deliberado: catálogo + existencias iniciales + clientes (+ mesas, plano y
// platos con receta en el restaurante). NO se siembran documentos fiscales: fabricar
// facturas, numeradores e IVA a mano en un ledger append-only es justo donde un seed
// puede dejar la contabilidad incoherente. El prospecto emite las suyas en la demo.

// IDs fijos de las demos por nicho (el frontend y los tests dependen de que no cambien).
const (
	nichoRestOrgID  = "org_demo_rest"
	nichoRestEmpID  = "emp_demo_rest"
	nichoRestSedeID = "sede_demo_rest"

	nichoFerrOrgID  = "org_demo_ferr"
	nichoFerrEmpID  = "emp_demo_ferr"
	nichoFerrSedeID = "sede_demo_ferr"

	nichoFarmOrgID  = "org_demo_farm"
	nichoFarmEmpID  = "emp_demo_farm"
	nichoFarmSedeID = "sede_demo_farm"
)

// prodNicho describe un producto sembrado. `stock` 0 deja el producto agotado a
// propósito (sin movimiento de entrada): la existencia es una proyección del ledger, no
// un contador, así que un agotado se representa por AUSENCIA de entradas.
type prodNicho struct {
	sku, nombre, rubro string
	unidad             string
	costo, precio      float64
	stock              float64
	exento             bool // exento de IVA (cesta básica, medicinas)
	porPeso            bool // se cobra por kg
	// insumo marca la MATERIA PRIMA: se stockea y se consume por receta, pero NO se
	// vende (queda fuera del POS y de la comandera) y no lleva precio de venta.
	insumo bool
}

// platoNicho es un producto COMPUESTO (escandallo): se vende como una línea a su precio
// de menú y descuenta sus INSUMOS del inventario. No se stockea.
type platoNicho struct {
	sku, nombre string
	precio      float64
	receta      []inventario.ComboComponente
}

type cliNicho struct {
	nombre, tipoDoc, doc, telefono string
}

type provNicho struct {
	nombre, doc, telefono string
}

// especNicho es la receta completa de una empresa demo.
type especNicho struct {
	orgID, empID, sedeID string
	org, nombre, rif     string
	giro                 string
	direccion            string
	colorMarca           string
	// slug identifica al tenant en los emails de sus usuarios demo
	// (vendedor@<slug>.test) y en los datos de sus cuentas de cobro.
	slug string
	// telefonoBanco es el número que decora las cuentas de cobro sembradas.
	telefonoBanco string
	// Nombres del personal demo. El cajero firma las facturas sembradas.
	cajero, supervisor, vendedor, contadora string
	// mesoneros son los del salón (dos en el restaurante, para que la asignación de
	// mesas se pueda ver funcionando); zonasMesoneros asigna una zona a cada uno, en
	// el mismo orden.
	mesoneros      []string
	zonasMesoneros []string
	// comanderas son los puestos de impresión del local (cocina, barra, postres) y de
	// qué rubros imprime cada uno.
	comanderas  []comanderaNicho
	proveedores []provNicho
	facturas    []emisionNicho
	rubros      []string
	productos   []prodNicho
	platos      []platoNicho
	clientes    []cliNicho
	modulos     []string
	// Salón (solo restaurante): grilla + mesas.
	filas, columnas int
	bloqueadas      []mesa.Celda
	mesas           []mesaNicho
}

// comanderaNicho es un puesto de impresión de comandas.
type comanderaNicho struct {
	nombre string
	rubros []string
	// predeterminada recibe lo que no encaja en ningún rubro configurado.
	predeterminada bool
	// red = impresora térmica con IP en la LAN; si no, local (por el agente).
	red    bool
	host   string
	puerto int
}

type mesaNicho struct {
	nombre, zona, forma string
	capacidad           int
	columna, fila       int
}

// mesoneroPrincipal es quien firma las cuentas y las facturas sembradas del salón.
func (e especNicho) mesoneroPrincipal() string {
	if len(e.mesoneros) > 0 {
		return e.mesoneros[0]
	}
	return ""
}

// costoDe devuelve el costo sembrado de un SKU. Se usa como costo del movimiento de
// salida: si no se pasa, el Kardex calcularía margen contra cero.
func (e especNicho) costoDe(sku string) float64 {
	for _, p := range e.productos {
		if p.sku == sku {
			return p.costo
		}
	}
	return 0
}

// seedNichos siembra las empresas demo de los rubros que no cubre la bodega.
func (s *Store) seedNichos() {
	for _, e := range especificacionesNicho() {
		s.seedEmpresaNicho(e)
	}
}

func (s *Store) seedEmpresaNicho(e especNicho) {
	fecha := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)

	s.Organizaciones.Create(organizacion.Organizacion{
		ID: e.orgID, Nombre: e.org, Estado: "activa", Plan: "base", Creada: fecha,
	})
	s.Empresas.Create(empresa.Empresa{
		ID: e.empID, OrganizacionID: e.orgID, Nombre: e.nombre, RIF: e.rif,
		Giro: e.giro, Modalidad: empresa.ModalidadFormaLibre,
		Activa: true, OnboardingOK: true, Creada: fecha,
		// Bolívares como moneda de los libros (lo que exige el SENIAT) con precios de
		// referencia en dólares, que es lo normal en el comercio venezolano.
		MonedaPrincipal: empresa.MonedaVES, FuenteTasa: empresa.FuenteTasaBCV, PreciosEnUsd: true,
		ColorMarca: e.colorMarca,
	})
	s.Sedes.Create(sede.Sede{
		ID: e.sedeID, EmpresaID: e.empID, Nombre: "Sede Principal",
		Direccion: e.direccion, Activa: true,
	})
	// La usuaria demo es Dueña también acá: por eso el selector de empresa del app
	// funciona como selector de rubro.
	s.Membresias.Create(usuario.Membresia{
		UsuarioID: application.DemoUserID, Email: application.DemoEmail, Nombre: application.DemoNombre,
		EmpresaID: e.empID, Rol: usuario.RolDueno, Estado: usuario.EstadoActiva,
	})
	for _, r := range e.rubros {
		s.Rubros.Create(inventario.Rubro{EmpresaID: e.empID, Nombre: r})
	}
	for _, m := range e.modulos {
		s.Modulos.Upsert(aplicacion.Instalacion{
			EmpresaID: e.empID, ModuloID: m, Instalado: true, Activo: true, Actualizada: fecha,
		})
	}

	// Catálogo + existencia inicial como movimiento de ENTRADA (nunca un contador).
	for _, p := range e.productos {
		unidad, tipoVenta := p.unidad, inventario.TipoVentaUnidad
		if p.porPeso {
			unidad, tipoVenta = "kg", inventario.TipoVentaPeso
		}
		// Un insumo no se vende: su precio de venta queda en cero (el costo vive en el
		// movimiento de entrada, que es de donde sale el escandallo).
		precio := p.precio
		if p.insumo {
			precio = 0
		}
		prod := s.Productos.Create(inventario.Producto{
			EmpresaID: e.empID, SKU: p.sku, Nombre: p.nombre, Rubro: p.rubro,
			UnidadBase: unidad, TipoVenta: tipoVenta, Precio: precio,
			ExentoIVA: p.exento, EsInsumo: p.insumo, Activo: true,
		})
		if p.stock <= 0 {
			continue
		}
		s.Movimientos.Append(inventario.Movimiento{
			EmpresaID: e.empID, SedeID: e.sedeID, ProductoID: prod.ID, SKU: prod.SKU,
			Tipo: inventario.MovEntrada, Cantidad: p.stock, CostoUnitario: p.costo,
			Motivo: "inventario inicial", Actor: application.DemoUserID, Fecha: fecha,
		})
	}

	// Platos: producto compuesto, se vende por unidad y NO se stockea (su existencia
	// sale de los insumos al facturar).
	for _, pl := range e.platos {
		s.Productos.Create(inventario.Producto{
			EmpresaID: e.empID, SKU: pl.sku, Nombre: pl.nombre, Rubro: "Cocina",
			UnidadBase: "unidad", TipoVenta: inventario.TipoVentaUnidad, Precio: pl.precio,
			EsPlato: true, Receta: pl.receta, Activo: true,
		})
	}

	for i, c := range e.clientes {
		s.Clientes.Create(cliente.Cliente{
			EmpresaID: e.empID, Nombre: c.nombre, TipoDocumento: c.tipoDoc,
			Documento: c.doc, Telefono: c.telefono, Activo: true,
			Notas: fmt.Sprintf("cliente de demostración %d", i+1),
		})
	}

	// Salón del restaurante: la grilla da la forma del local y las celdas bloqueadas
	// marcan dónde NO puede ir una mesa (cocina, barra, paredes).
	if len(e.mesas) > 0 {
		s.Planos.Upsert(mesa.Plano{
			EmpresaID: e.empID, SedeID: e.sedeID, Filas: e.filas, Columnas: e.columnas,
			Bloqueadas: e.bloqueadas, Actualizada: fecha,
		})
		for _, m := range e.mesas {
			s.Mesas.Create(mesa.Mesa{
				EmpresaID: e.empID, SedeID: e.sedeID, Nombre: m.nombre, Zona: m.zona,
				Capacidad: m.capacidad, Forma: m.forma, Columna: m.columna, Fila: m.fila,
				Estado: mesa.EstadoLibre, Activa: true, Creada: fecha,
			})
		}
	}

	// Datos de OPERACIÓN (caja, credenciales, cobros, facturación). Ver
	// seed_nichos_operacion.go.
	s.seedOperacionNicho(e)
}

// --- Exposición para el seed de Mongo --------------------------------------

// NichoDemo identifica una empresa demo por rubro y su estructura mínima.
type NichoDemo struct {
	OrgID, EmpresaID, SedeID, Giro string
}

// NichosDemo devuelve las empresas demo por rubro. NO incluye la bodega (emp_demo), que
// viaja por Snapshot().
func NichosDemo() []NichoDemo {
	out := make([]NichoDemo, 0, 3)
	for _, e := range especificacionesNicho() {
		out = append(out, NichoDemo{OrgID: e.orgID, EmpresaID: e.empID, SedeID: e.sedeID, Giro: e.giro})
	}
	return out
}

// SnapshotEmpresa son los datos sembrados de UNA empresa demo. Snapshot() está acotado a
// la bodega demo a propósito (es "el tenant demo"); esto es su equivalente por nicho, para
// que el seed de Mongo pueda plantar cada rubro por separado y de forma ADITIVA.
type SnapshotEmpresa struct {
	Org         organizacion.Organizacion
	Empresa     empresa.Empresa
	Sedes       []sede.Sede
	Membresias  []usuario.Membresia
	Rubros      []inventario.Rubro
	Productos   []inventario.Producto
	Movimientos []inventario.Movimiento
	Clientes    []cliente.Cliente
	Modulos     []aplicacion.Instalacion
	Mesas       []mesa.Mesa
	Plano       mesa.Plano
	TienePlano  bool

	// Operación (ver seed_nichos_operacion.go).
	Usuarios     []usuario.Usuario
	Credenciales []credencial.Credencial
	Cajas        []caja.Caja
	Cajeros      []caja.Cajero
	CuentasCobro []fiscal.CuentaCobro
	MetodosPago  []fiscal.MetodoPago
	Proveedores  []proveedor.Proveedor
	Documentos   []fiscal.Documento
	CuentasMesa  []cuenta.Cuenta
	Asignaciones []mesa.Asignacion
	Comanderas   []cocina.Impresora
	// Contadores es el estado del numerador fiscal de ESTA empresa tras sembrar
	// ("empresa|sede|serie" → último folio). Sin ellos, la primera factura real del
	// prospecto reiniciaría en 1 y colisionaría con un folio sembrado.
	Contadores map[string]int
}

// SnapshotNicho recoge todo lo sembrado para una empresa demo por rubro.
func (s *Store) SnapshotNicho(n NichoDemo) SnapshotEmpresa {
	org, _ := s.Organizaciones.ByID(n.OrgID)
	emp, _ := s.Empresas.ByID(n.EmpresaID)
	plano, tiene := s.Planos.Get(n.EmpresaID, n.SedeID)

	// Los usuarios y sus credenciales se derivan de las membresías de la empresa: el
	// repositorio de usuarios es global (no lleva empresaID).
	membresias := s.Membresias.ByEmpresa(n.EmpresaID)
	usuarios := make([]usuario.Usuario, 0, len(membresias))
	creds := make([]credencial.Credencial, 0, len(membresias))
	for _, m := range membresias {
		if u, ok := s.Usuarios.ByID(m.UsuarioID); ok {
			usuarios = append(usuarios, u)
		}
		if c, ok := s.Credenciales.ByEmail(m.Email); ok {
			creds = append(creds, c)
		}
	}

	// Contadores del numerador que pertenecen a esta empresa (clave con su prefijo).
	contadores := map[string]int{}
	for k, v := range s.Numerador.Estado() {
		if strings.HasPrefix(k, n.EmpresaID+"|") {
			contadores[k] = v
		}
	}

	return SnapshotEmpresa{
		Org: org, Empresa: emp,
		Sedes:       s.Sedes.List(n.EmpresaID),
		Membresias:  s.Membresias.ByEmpresa(n.EmpresaID),
		Rubros:      s.Rubros.List(n.EmpresaID),
		Productos:   s.Productos.List(n.EmpresaID),
		Movimientos: s.Movimientos.List(n.EmpresaID, inventario.FiltroMovimiento{}),
		Clientes:    s.Clientes.List(n.EmpresaID),
		Modulos:     s.Modulos.List(n.EmpresaID),
		Mesas:       s.Mesas.List(n.EmpresaID, ""),
		Plano:       plano, TienePlano: tiene,

		Usuarios:     usuarios,
		Credenciales: creds,
		Cajas:        s.Cajas.List(n.EmpresaID),
		Cajeros:      s.Cajeros.List(n.EmpresaID),
		CuentasCobro: s.CuentasCobro.List(n.EmpresaID),
		MetodosPago:  s.MetodosPago.List(n.EmpresaID),
		Proveedores:  s.Proveedores.List(n.EmpresaID),
		Documentos:   s.Documentos.List(n.EmpresaID),
		CuentasMesa:  s.Cuentas.Abiertas(n.EmpresaID, n.SedeID),
		Asignaciones: s.Asignaciones.List(n.EmpresaID, n.SedeID),
		Comanderas:   s.Impresoras.List(n.EmpresaID, n.SedeID),
		Contadores:   contadores,
	}
}
