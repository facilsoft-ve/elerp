package application_test

import (
	"errors"
	"testing"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* REABASTECIMIENTO.
 *
 * La prueba que decide si esto sirve o se acaba apagando es
 * TestReabastecimiento_NoPideDosVecesLoMismo. Un reabastecimiento que no descuenta
 * lo ya pedido vuelve a sugerir lo mismo cada mañana hasta que llega el camión —y
 * para entonces nadie lo mira. */

// servicioConReabastecimiento cablea lo que la reposición necesita: almacenes,
// apartados (para saber lo comprometido) y las reglas.
func servicioConReabastecimiento(t *testing.T) *application.Service {
	t.Helper()
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes)
	svc.ConApartados(st.Apartados)
	svc.ConReglasReabastecimiento(st.ReglasReabastecimiento)
	return svc
}

// productoConStock crea un producto y le carga existencia.
func productoConStock(t *testing.T, svc *application.Service, sku string, cant float64) {
	t.Helper()
	if _, err := svc.CrearProducto(empDemo, actorA, origenTst, inventario.Producto{
		SKU: sku, Nombre: "Producto " + sku, Precio: 50,
	}); err != nil {
		t.Fatalf("crear producto %s: %v", sku, err)
	}
	if cant > 0 {
		if _, err := svc.Ajustar(empDemo, sede1, "", sku, "carga inicial", cant, actorA, origenTst); err != nil {
			t.Fatalf("cargar %s: %v", sku, err)
		}
	}
}

// reglaDe da de alta una regla de reposición.
func reglaDe(t *testing.T, svc *application.Service, sku string, min, max, multiplo float64) inventario.ReglaReabastecimiento {
	t.Helper()
	r, err := svc.CrearReglaReabastecimiento(empDemo, actorA, origenTst, inventario.ReglaReabastecimiento{
		SedeID: sede1, SKU: sku, Minimo: min, Maximo: max, Multiplo: multiplo,
	})
	if err != nil {
		t.Fatalf("crear regla de %s: %v", sku, err)
	}
	return r
}

// filaDe busca un SKU en la revisión.
func filaDe(rev application.RevisionReabastecimiento, sku string) (application.FilaReabastecimiento, bool) {
	for _, f := range rev.Filas {
		if f.SKU == sku {
			return f, true
		}
	}
	return application.FilaReabastecimiento{}, false
}

// TestReabastecimiento_PideHastaElMaximo: lo básico, y el redondeo al múltiplo.
func TestReabastecimiento_PideHastaElMaximo(t *testing.T) {
	svc := servicioConReabastecimiento(t)
	productoConStock(t, svc, "REA-1", 4)  // bajo mínimo
	productoConStock(t, svc, "REA-2", 50) // holgado
	productoConStock(t, svc, "REA-3", 5)  // bajo mínimo, con múltiplo
	reglaDe(t, svc, "REA-1", 10, 40, 0)
	reglaDe(t, svc, "REA-2", 10, 40, 0)
	reglaDe(t, svc, "REA-3", 10, 40, 12) // solo cajas de 12

	rev := svc.RevisarReabastecimiento(empDemo, sede1)
	if rev.Reglas != 3 {
		t.Errorf("hay 3 reglas: %d", rev.Reglas)
	}
	if _, hay := filaDe(rev, "REA-2"); hay {
		t.Error("REA-2 está por encima de su mínimo: no debía aparecer")
	}
	if rev.Cubierto != 1 {
		t.Errorf("uno de los tres estaba cubierto: %d", rev.Cubierto)
	}
	f1, _ := filaDe(rev, "REA-1")
	if !casi(f1.APedir, 36) {
		t.Errorf("de 4 a 40 son 36: %v", f1.APedir)
	}
	// Y el desglose explica la decisión, que es lo que hace que alguien la acepte.
	if !casi(f1.Existencia, 4) || !casi(f1.Cobertura, 4) || !casi(f1.Minimo, 10) {
		t.Errorf("la fila tiene que explicar por qué se pide: %+v", f1)
	}
	f3, _ := filaDe(rev, "REA-3")
	// De 5 a 40 son 35, pero solo se venden cajas de 12: 3 cajas = 36.
	if !casi(f3.APedir, 36) {
		t.Errorf("con múltiplo de 12, 35 se redondea HACIA ARRIBA a 36: %v", f3.APedir)
	}
}

// TestReabastecimiento_NoPideDosVecesLoMismo es LA prueba del módulo.
//
// Sin descontar lo que ya viene en camino, la regla vuelve a sugerir lo mismo en
// cada revisión hasta que llega el primer camión. Es el fallo que hace que estos
// sistemas se acaben apagando: sugieren comprar lo que ya está comprado.
func TestReabastecimiento_NoPideDosVecesLoMismo(t *testing.T) {
	svc := servicioConReabastecimiento(t)
	productoConStock(t, svc, "REA-4", 4)
	reglaDe(t, svc, "REA-4", 10, 40, 0)

	rev := svc.RevisarReabastecimiento(empDemo, sede1)
	f, hay := filaDe(rev, "REA-4")
	if !hay || !casi(f.APedir, 36) {
		t.Fatalf("de partida faltan 36: %+v", f)
	}

	// Se pide al proveedor: una orden CONFIRMADA es un compromiso real.
	oc, err := svc.CrearOrdenCompra(empDemo, sede1, actorA, origenTst, application.EntradaOC{
		ProveedorID: provDemo1, SedeID: sede1,
		Lineas: []application.LineaOCEntrada{{SKU: "REA-4", Cantidad: 36, CostoUnitario: 20}},
	})
	if err != nil {
		t.Fatalf("crear OC: %v", err)
	}
	if _, err := svc.ConfirmarOrdenCompra(empDemo, oc.ID, actorA, origenTst); err != nil {
		t.Fatalf("confirmar: %v", err)
	}

	// Ya no hay nada que pedir: lo que falta viene en camino.
	rev = svc.RevisarReabastecimiento(empDemo, sede1)
	if f, hay := filaDe(rev, "REA-4"); hay {
		t.Errorf("las 36 ya están pedidas: no se vuelven a pedir (%+v)", f)
	}

	// Y si llega solo la mitad, se vuelve a mirar con lo que sigue pendiente.
	if _, err := svc.RecibirOrdenCompra(empDemo, oc.ID, actorA, origenTst,
		[]application.LineaRecepcion{{SKU: "REA-4", Cantidad: 18}}); err != nil {
		t.Fatalf("recibir la mitad: %v", err)
	}
	rev = svc.RevisarReabastecimiento(empDemo, sede1)
	if f, hay := filaDe(rev, "REA-4"); hay {
		t.Errorf("con 22 en almacén y 18 en camino la cobertura es 40: nada que pedir (%+v)", f)
	}
}

// TestReabastecimiento_LoApartadoNoCuenta: la mercancía comprometida está en el
// almacén pero ya tiene dueño. Contarla como disponible haría reponer de menos, y
// el faltante aparecería justo cuando llega el cliente que la apartó.
func TestReabastecimiento_LoApartadoNoCuenta(t *testing.T) {
	svc := servicioConReabastecimiento(t)
	productoConStock(t, svc, "REA-5", 30)
	reglaDe(t, svc, "REA-5", 10, 40, 0)

	// Con 30 en el almacén y mínimo 10, no hay que pedir nada.
	if _, hay := filaDe(svc.RevisarReabastecimiento(empDemo, sede1), "REA-5"); hay {
		t.Fatal("con 30 y mínimo 10 no había que pedir nada")
	}

	// Se apartan 25 para un cliente: quedan 5 libres, por debajo del mínimo.
	if _, err := svc.CrearApartado(empDemo, actorA, origenTst, inventario.Apartado{
		SedeID: sede1, Motivo: "pedido grande",
		Lineas: []inventario.LineaApartado{{SKU: "REA-5", Cantidad: 25}},
	}); err != nil {
		t.Fatalf("apartar: %v", err)
	}
	f, hay := filaDe(svc.RevisarReabastecimiento(empDemo, sede1), "REA-5")
	if !hay {
		t.Fatal("con 25 apartadas quedan 5 libres: hay que reponer")
	}
	if !casi(f.Apartado, 25) || !casi(f.Disponible, 5) || !casi(f.APedir, 35) {
		t.Errorf("la fila tenía que explicar el apartado: %+v", f)
	}
}

// TestReabastecimiento_ElMaestroSeDefiende: cada guarda evita una regla que
// después haría algo absurdo.
func TestReabastecimiento_ElMaestroSeDefiende(t *testing.T) {
	svc := servicioConReabastecimiento(t)
	productoConStock(t, svc, "REA-6", 10)

	// Máximo igual al mínimo: cada venta dispararía un pedido del tamaño de la venta.
	if _, err := svc.CrearReglaReabastecimiento(empDemo, actorA, origenTst, inventario.ReglaReabastecimiento{
		SedeID: sede1, SKU: "REA-6", Minimo: 10, Maximo: 10,
	}); !errors.Is(err, application.ErrReglaNiveles) {
		t.Errorf("máximo = mínimo debía rechazarse: %v", err)
	}
	if _, err := svc.CrearReglaReabastecimiento(empDemo, actorA, origenTst, inventario.ReglaReabastecimiento{
		SedeID: sede1, SKU: "REA-6", Minimo: -1, Maximo: 10,
	}); !errors.Is(err, application.ErrReglaNegativa) {
		t.Errorf("un nivel negativo debía rechazarse: %v", err)
	}
	if _, err := svc.CrearReglaReabastecimiento(empDemo, actorA, origenTst, inventario.ReglaReabastecimiento{
		SedeID: sede1, SKU: "NO-EXISTE", Minimo: 1, Maximo: 10,
	}); !errors.Is(err, application.ErrReglaProductoNoExiste) {
		t.Errorf("un SKU inventado debía rechazarse: %v", err)
	}
	// Dos reglas del mismo producto y ámbito se pisarían, y cuál gana dependería del
	// orden de lectura — que en un mapa no existe.
	reglaDe(t, svc, "REA-6", 5, 20, 0)
	if _, err := svc.CrearReglaReabastecimiento(empDemo, actorA, origenTst, inventario.ReglaReabastecimiento{
		SedeID: sede1, SKU: "REA-6", Minimo: 8, Maximo: 30,
	}); !errors.Is(err, application.ErrReglaDuplicada) {
		t.Errorf("una segunda regla del mismo producto debía rechazarse: %v", err)
	}
}

// TestReabastecimiento_GeneraSolicitudesPorProveedor: la regla NO compra. Prepara
// solicitudes de presupuesto, una por proveedor — mezclarlos obligaría a quien la
// recibe a cotizar cosas que no vende.
func TestReabastecimiento_GeneraSolicitudesPorProveedor(t *testing.T) {
	svc := servicioConReabastecimiento(t)
	productoConStock(t, svc, "REA-7", 2)
	productoConStock(t, svc, "REA-8", 1)
	productoConStock(t, svc, "REA-9", 0)

	// Dos del mismo proveedor y uno sin proveedor asignado.
	r7 := reglaDe(t, svc, "REA-7", 10, 30, 0)
	r8 := reglaDe(t, svc, "REA-8", 10, 30, 0)
	reglaDe(t, svc, "REA-9", 10, 30, 0)
	for _, r := range []inventario.ReglaReabastecimiento{r7, r8} {
		if _, err := svc.ActualizarReglaReabastecimiento(empDemo, r.ID, actorA, origenTst,
			inventario.ReglaReabastecimiento{Minimo: 10, Maximo: 30, ProveedorID: provDemo1, Activa: true}); err != nil {
			t.Fatalf("asignar proveedor: %v", err)
		}
	}

	sols, err := svc.GenerarSolicitudesDeReabastecimiento(empDemo, sede1, actorA, origenTst)
	if err != nil {
		t.Fatalf("generar: %v", err)
	}
	if len(sols) != 2 {
		t.Fatalf("dos proveedores distintos (uno de ellos «sin asignar») son dos solicitudes: %d", len(sols))
	}
	// Ninguna se convirtió en orden: la regla prepara, una persona decide.
	for _, s := range sols {
		if s.OrdenGeneradaID != "" {
			t.Errorf("el reabastecimiento NO compra: %+v", s)
		}
		if len(s.Lineas) == 0 {
			t.Error("una solicitud sin líneas no pide nada")
		}
		// Y dice de dónde salió: una solicitud sin explicación obliga a reconstruir
		// por qué se pidió justo eso.
		if s.Notas == "" {
			t.Error("la solicitud tenía que decir que la generó el reabastecimiento")
		}
	}
	// Tras generarlas, la revisión avisa de que ya están pedidas.
	rev := svc.RevisarReabastecimiento(empDemo, sede1)
	for _, f := range rev.Filas {
		if !f.YaSolicitado {
			t.Errorf("%s ya está en una solicitud abierta: tenía que avisarse", f.SKU)
		}
	}
}

// TestReabastecimiento_SinNadaQuePedirNoGeneraNada: un documento vacío es ruido, y
// el que lo recibe deja de mirarlos.
func TestReabastecimiento_SinNadaQuePedirNoGeneraNada(t *testing.T) {
	svc := servicioConReabastecimiento(t)
	productoConStock(t, svc, "REA-10", 100)
	reglaDe(t, svc, "REA-10", 10, 40, 0)

	if _, err := svc.GenerarSolicitudesDeReabastecimiento(empDemo, sede1, actorA, origenTst); !errors.Is(err, application.ErrSinNadaQuePedir) {
		t.Errorf("con todo cubierto no hay solicitud que generar: %v", err)
	}
}

// TestReabastecimiento_SinCablearNoPasaNada: la regresión. Sin el maestro, todo se
// comporta como antes.
func TestReabastecimiento_SinCablearNoPasaNada(t *testing.T) {
	svc, st := nuevoServicio(t)
	svc.ConAlmacenes(st.Almacenes) // ...pero SIN ConReglasReabastecimiento

	if n := len(svc.ReglasReabastecimiento(empDemo, sede1)); n != 0 {
		t.Errorf("sin repositorio no hay reglas: %d", n)
	}
	rev := svc.RevisarReabastecimiento(empDemo, sede1)
	if len(rev.Filas) != 0 || rev.Reglas != 0 {
		t.Errorf("sin reglas no hay nada que reponer: %+v", rev)
	}
	if _, err := svc.GenerarSolicitudesDeReabastecimiento(empDemo, sede1, actorA, origenTst); err == nil {
		t.Error("sin reglas no se genera ninguna solicitud")
	}
}
