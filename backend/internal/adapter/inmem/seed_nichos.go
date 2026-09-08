package inmem

import (
	"fmt"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/cliente"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/organizacion"
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

// especNicho es la receta completa de una empresa demo.
type especNicho struct {
	orgID, empID, sedeID string
	org, nombre, rif     string
	giro                 string
	direccion            string
	colorMarca           string
	rubros               []string
	productos            []prodNicho
	platos               []platoNicho
	clientes             []cliNicho
	modulos              []string
	// Salón (solo restaurante): grilla + mesas.
	filas, columnas int
	bloqueadas      []mesa.Celda
	mesas           []mesaNicho
}

type mesaNicho struct {
	nombre, zona, forma string
	capacidad           int
	columna, fila       int
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
		prod := s.Productos.Create(inventario.Producto{
			EmpresaID: e.empID, SKU: p.sku, Nombre: p.nombre, Rubro: p.rubro,
			UnidadBase: unidad, TipoVenta: tipoVenta, Precio: p.precio,
			ExentoIVA: p.exento, Activo: true,
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
}

// SnapshotNicho recoge todo lo sembrado para una empresa demo por rubro.
func (s *Store) SnapshotNicho(n NichoDemo) SnapshotEmpresa {
	org, _ := s.Organizaciones.ByID(n.OrgID)
	emp, _ := s.Empresas.ByID(n.EmpresaID)
	plano, tiene := s.Planos.Get(n.EmpresaID, n.SedeID)
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
	}
}
