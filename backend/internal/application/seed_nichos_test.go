package application_test

import (
	"strings"
	"testing"

	"github.com/mornix/elerp/internal/adapter/inmem"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// Demos por RUBRO: el modo demo trae una empresa por nicho para que el prospecto vea el
// ERP con datos de su negocio. Estos tests protegen la COHERENCIA de esos datos —un SKU
// mal escrito en una receta rompería el descuento de inventario sin dar error.

// nichosEsperados son los giros que deben existir además de la bodega.
var nichosEsperados = map[string]string{
	"emp_demo":      "bodega",
	"emp_demo_rest": "restaurante",
	"emp_demo_ferr": "ferreteria",
	"emp_demo_farm": "farmacia",
}

func TestDemosNicho_UnaEmpresaPorRubro(t *testing.T) {
	st := inmem.New()
	for id, giro := range nichosEsperados {
		e, ok := st.Empresas.ByID(id)
		if !ok {
			t.Errorf("falta la empresa demo %s (%s)", id, giro)
			continue
		}
		if e.Giro != giro {
			t.Errorf("%s: giro esperado %q, se obtuvo %q", id, giro, e.Giro)
		}
		if !e.Activa || !e.OnboardingOK {
			t.Errorf("%s: la empresa demo debe estar activa y con onboarding hecho", id)
		}
		if e.RIF == "" {
			t.Errorf("%s: la empresa demo necesita RIF para poder facturar", id)
		}
	}
}

// La usuaria demo debe ser Dueña de TODAS: el selector de empresa hace de selector de
// rubro, así que sin membresía el nicho es invisible.
func TestDemosNicho_UsuariaDemoEsDuenaDeTodas(t *testing.T) {
	st := inmem.New()
	porEmpresa := map[string]string{}
	for _, m := range st.Membresias.ByUsuario("usr_demo") {
		porEmpresa[m.EmpresaID] = m.Rol
	}
	for id := range nichosEsperados {
		rol, ok := porEmpresa[id]
		if !ok {
			t.Errorf("la usuaria demo no tiene membresía en %s: el rubro no aparecería en el selector", id)
			continue
		}
		if rol != usuario.RolDueno {
			t.Errorf("%s: la usuaria demo debe ser %q, es %q", id, usuario.RolDueno, rol)
		}
	}
}

// Cada SKU de cada receta tiene que existir como producto ACTIVO de la misma empresa, y
// no puede ser otro plato (no se permite anidar).
func TestDemosNicho_RecetasApuntanAInsumosReales(t *testing.T) {
	st := inmem.New()
	platos := 0
	for _, p := range st.Productos.List("emp_demo_rest") {
		if !p.EsPlato {
			continue
		}
		platos++
		if len(p.Receta) == 0 {
			t.Errorf("el plato %s no tiene receta: no descontaría nada del inventario", p.SKU)
		}
		for _, ins := range p.Receta {
			insumo, ok := st.Productos.BySKU("emp_demo_rest", ins.SKU)
			if !ok {
				t.Errorf("el plato %s usa el insumo %q, que NO existe en el catálogo", p.SKU, ins.SKU)
				continue
			}
			if !insumo.Activo {
				t.Errorf("el plato %s usa el insumo %q, que está inactivo", p.SKU, ins.SKU)
			}
			if insumo.EsPlato {
				t.Errorf("el plato %s usa %q, que es otro plato: no se permite anidar recetas", p.SKU, ins.SKU)
			}
			if ins.Cantidad <= 0 {
				t.Errorf("el plato %s usa %q con cantidad %v: debe ser mayor que cero", p.SKU, ins.SKU, ins.Cantidad)
			}
		}
	}
	if platos == 0 {
		t.Fatal("la demo de restaurante debe traer platos con receta")
	}
}

// Un plato NO se stockea: su existencia sale de los insumos al facturar. Si el seed le
// diera entrada de inventario, el Kardex quedaría contando algo que no existe.
func TestDemosNicho_LosPlatosNoTienenExistencia(t *testing.T) {
	st := inmem.New()
	for _, p := range st.Productos.List("emp_demo_rest") {
		if !p.EsPlato {
			continue
		}
		for _, m := range st.Movimientos.List("emp_demo_rest", inventario.FiltroMovimiento{}) {
			if m.SKU == p.SKU {
				t.Errorf("el plato %s tiene un movimiento de inventario (%s): los platos no se stockean", p.SKU, m.Tipo)
			}
		}
	}
}

// La existencia es una PROYECCIÓN del ledger: un producto con entrada sembrada tiene
// stock, y uno sin entrada queda agotado. Cada nicho debe traer ambos casos.
func TestDemosNicho_ExistenciasSalenDelLedger(t *testing.T) {
	st := inmem.New()
	for _, caso := range []struct{ emp, sede string }{
		{"emp_demo_rest", "sede_demo_rest"},
		{"emp_demo_ferr", "sede_demo_ferr"},
		{"emp_demo_farm", "sede_demo_farm"},
	} {
		conStock, agotados := 0, 0
		movPorSKU := map[string]float64{}
		for _, m := range st.Movimientos.List(caso.emp, inventario.FiltroMovimiento{}) {
			movPorSKU[m.SKU] += m.Cantidad
		}
		for _, p := range st.Productos.List(caso.emp) {
			if p.EsPlato {
				continue
			}
			if movPorSKU[p.SKU] > 0 {
				conStock++
			} else {
				agotados++
			}
		}
		if conStock < 5 {
			t.Errorf("%s: solo %d productos con existencia; la demo se vería vacía", caso.emp, conStock)
		}
		if agotados == 0 {
			t.Errorf("%s: no hay ningún producto agotado; la demo no mostraría ese estado", caso.emp)
		}
	}
}

// La farmacia es el caso de las EXENCIONES: los medicamentos no llevan IVA y el cuidado
// personal sí. Sin las dos bases, la factura de la demo no muestra la separación.
func TestDemosNicho_FarmaciaMezclaExentosYGravados(t *testing.T) {
	st := inmem.New()
	exentos, gravados := 0, 0
	for _, p := range st.Productos.List("emp_demo_farm") {
		if p.ExentoIVA {
			exentos++
		} else {
			gravados++
		}
	}
	if exentos == 0 || gravados == 0 {
		t.Fatalf("la farmacia demo debe mezclar exentos y gravados; hay %d exentos y %d gravados", exentos, gravados)
	}
}

// El salón del restaurante: la grilla existe, ninguna mesa cae en una celda bloqueada y
// ninguna se sale del plano ni comparte celda con otra.
func TestDemosNicho_SalonCoherente(t *testing.T) {
	st := inmem.New()
	plano, ok := st.Planos.Get("emp_demo_rest", "sede_demo_rest")
	if !ok {
		t.Fatal("la demo de restaurante debe traer el plano del salón")
	}
	bloqueada := map[[2]int]bool{}
	for _, c := range plano.Bloqueadas {
		bloqueada[[2]int{c.Columna, c.Fila}] = true
	}
	ocupada := map[[2]int]string{}
	mesas := st.Mesas.List("emp_demo_rest", "sede_demo_rest")
	if len(mesas) == 0 {
		t.Fatal("la demo de restaurante debe traer mesas")
	}
	for _, m := range mesas {
		celda := [2]int{m.Columna, m.Fila}
		if m.Columna < 1 || m.Columna > plano.Columnas || m.Fila < 1 || m.Fila > plano.Filas {
			t.Errorf("la mesa %s está fuera del plano (%d×%d): columna %d, fila %d",
				m.Nombre, plano.Columnas, plano.Filas, m.Columna, m.Fila)
		}
		if bloqueada[celda] {
			t.Errorf("la mesa %s está en una celda BLOQUEADA (%d,%d)", m.Nombre, m.Columna, m.Fila)
		}
		if otra, ya := ocupada[celda]; ya {
			t.Errorf("las mesas %s y %s comparten la celda (%d,%d)", otra, m.Nombre, m.Columna, m.Fila)
		}
		ocupada[celda] = m.Nombre
		if m.Capacidad <= 0 {
			t.Errorf("la mesa %s necesita capacidad de comensales", m.Nombre)
		}
	}
}

// Un plato no se stockea, así que NO puede aparecer en la proyección de existencias:
// si aparece, sale con cantidad 0 y todo el menú se reporta «agotado» (en el Inicio, en
// existencias y en los reportes).
func TestExistencias_ExcluyeCombosYPlatos(t *testing.T) {
	svc, _ := nuevoServicio(t)
	existencias := svc.Existencias("emp_demo_rest", "sede_demo_rest")
	if len(existencias) == 0 {
		t.Fatal("la demo de restaurante debe proyectar existencias de sus insumos")
	}
	for _, e := range existencias {
		if strings.HasPrefix(e.SKU, "PLA-") {
			t.Errorf("el plato %s aparece en existencias: los platos no se stockean", e.SKU)
		}
	}
	// Y los insumos sí están.
	var insumos int
	for _, e := range existencias {
		if strings.HasPrefix(e.SKU, "INS-") {
			insumos++
		}
	}
	if insumos == 0 {
		t.Error("los insumos del restaurante deben aparecer en existencias")
	}
}

// Cada rubro tiene que poder VENDER: caja habilitada y un cajero cuyo PIN sea el de
// demostración. Sin esto el prospecto abre el POS y no puede facturar nada.
func TestDemosNicho_CadaRubroPuedeAbrirCaja(t *testing.T) {
	for _, emp := range []string{"emp_demo_rest", "emp_demo_ferr", "emp_demo_farm"} {
		svc, st := nuevoServicio(t)
		cajas := st.Cajas.List(emp)
		if len(cajas) == 0 {
			t.Fatalf("%s: no tiene ninguna caja; no se puede facturar", emp)
		}
		if _, err := svc.AbrirCaja(emp, actorA, origenTst, cajas[0].ID, "OP-001", inmem.PinDemo); err != nil {
			t.Errorf("%s: el PIN de demostración debe abrir la caja: %v", emp, err)
		}
	}
}

// El numerador tiene que quedar ADELANTADO sobre el folio más alto sembrado. Si no, la
// primera factura real del prospecto reiniciaría en 1 y chocaría con un folio ya emitido
// — y los documentos son append-only: eso no se arregla después.
func TestDemosNicho_NumeradorAdelantado(t *testing.T) {
	_, st := nuevoServicio(t)
	for _, caso := range []struct{ emp, sede string }{
		{"emp_demo_rest", "sede_demo_rest"},
		{"emp_demo_ferr", "sede_demo_ferr"},
		{"emp_demo_farm", "sede_demo_farm"},
	} {
		maxPorSerie := map[string]int{}
		for _, d := range st.Documentos.List(caso.emp) {
			if d.Numero > maxPorSerie[d.Serie] {
				maxPorSerie[d.Serie] = d.Numero
			}
		}
		if len(maxPorSerie) == 0 {
			t.Errorf("%s: no hay documentos sembrados; el rubro se vería vacío", caso.emp)
			continue
		}
		for serie, ultimo := range maxPorSerie {
			actual := st.Numerador.Actual(caso.emp, caso.sede, serie)
			if actual < ultimo {
				t.Errorf("%s serie %s: el numerador está en %d pero ya hay un folio %d — la próxima factura colisionaría",
					caso.emp, serie, actual, ultimo)
			}
		}
	}
}

// La facturación sembrada debe cubrir la gama: contado, divisas con IGTF, contingencia,
// crédito (vencido y vigente) y una reversa. Es lo que hace que Tesorería, los libros
// fiscales y el gráfico del Inicio tengan algo que mostrar.
func TestDemosNicho_FacturacionCubreLaGama(t *testing.T) {
	_, st := nuevoServicio(t)
	for _, emp := range []string{"emp_demo_rest", "emp_demo_ferr", "emp_demo_farm"} {
		var conIGTF, contingencia, credito, reversas int
		for _, d := range st.Documentos.List(emp) {
			if d.IGTF > 0 {
				conIGTF++
			}
			if d.Contingencia {
				contingencia++
			}
			if d.Credito {
				credito++
			}
			if d.RefDocumentoID != "" {
				reversas++
			}
		}
		if conIGTF == 0 {
			t.Errorf("%s: falta una venta cobrada en divisas (IGTF)", emp)
		}
		if contingencia == 0 {
			t.Errorf("%s: falta una factura de contingencia", emp)
		}
		if credito < 2 {
			t.Errorf("%s: hacen falta 2 ventas a crédito (una vencida y una vigente), hay %d", emp, credito)
		}
		if reversas == 0 {
			t.Errorf("%s: falta una reversa (anulación o nota de crédito)", emp)
		}
	}
}

// El restaurante arranca EN SERVICIO: mesas ocupadas con comandas en cocina, para que la
// comandera y la pantalla de Cocina no se vean vacías.
func TestDemosNicho_RestauranteEnServicio(t *testing.T) {
	_, st := nuevoServicio(t)
	abiertas := st.Cuentas.Abiertas("emp_demo_rest", "sede_demo_rest")
	if len(abiertas) < 2 {
		t.Fatalf("el restaurante debe abrir con mesas ocupadas, hay %d cuentas", len(abiertas))
	}
	enCocina := 0
	for _, c := range abiertas {
		for _, it := range c.Items {
			if it.Estado == "en_cocina" || it.Estado == "listo" {
				if it.EnviadoEn == "" {
					t.Errorf("el renglón %s está en cocina sin hora de envío: la pantalla de Cocina no podría calcular la espera", it.SKU)
				}
				enCocina++
			}
		}
	}
	if enCocina == 0 {
		t.Error("debe haber al menos una comanda en cocina para que el KDS muestre algo")
	}
	// Las mesas con cuenta abierta tienen que reflejarlo en su estado.
	for _, c := range abiertas {
		m, ok := st.Mesas.ByID("emp_demo_rest", c.MesaID)
		if !ok {
			t.Errorf("la cuenta %s apunta a una mesa que no existe", c.ID)
			continue
		}
		if m.Estado == "libre" {
			t.Errorf("la mesa %s tiene cuenta abierta pero figura como libre", m.Nombre)
		}
	}
}

// El módulo Restaurante solo corresponde a la demo de restaurante. Una bodega, una
// ferretería o una farmacia con mapa de mesas no tiene sentido (y confunde al que prueba).
func TestDemosNicho_RestauranteSoloEnElRestaurante(t *testing.T) {
	_, st := nuevoServicio(t)
	for _, emp := range []string{"emp_demo", "emp_demo_ferr", "emp_demo_farm"} {
		for _, ins := range st.Modulos.List(emp) {
			if ins.ModuloID == "restaurante" && ins.Activo {
				t.Errorf("%s tiene el módulo Restaurante activo y no le corresponde", emp)
			}
		}
		if n := len(st.Mesas.List(emp, "")); n > 0 {
			t.Errorf("%s tiene %d mesas sembradas y no le corresponden", emp, n)
		}
	}
	// Y en el restaurante sí debe estar.
	activo := false
	for _, ins := range st.Modulos.List("emp_demo_rest") {
		if ins.ModuloID == "restaurante" && ins.Activo {
			activo = true
		}
	}
	if !activo {
		t.Error("la demo de restaurante debe tener el módulo Restaurante activo")
	}
}
