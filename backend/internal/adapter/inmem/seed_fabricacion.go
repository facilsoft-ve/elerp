package inmem

import (
	"fmt"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/fabricacion"
	"github.com/mornix/elerp/internal/domain/inventario"
)

/* DATOS DE DEMOSTRACIÓN DEL MÓDULO DE FABRICACIÓN.
 *
 * Lo que tiene que quedar claro al abrir la pantalla es que un mismo local
 * convive con los dos modos: la torta se prepara al pedirla y el brownie está
 * hecho en la vitrina. Por eso se agrega un producto NUEVO en vez de convertir
 * uno existente — así los dos casos se ven uno al lado del otro.
 *
 * Y las órdenes van en TRES estados, porque cada uno enseña una cosa distinta:
 * la planificada deja tocar «Arrancar», la que está en proceso deja «Terminar»
 * declarando cuánto salió de verdad, y la terminada muestra el costo derivado y
 * la merma.
 *
 * LO QUE NO SE PUEDE HACER es sembrar la orden sin sus movimientos. Una orden
 * terminada que no consumió ni produjo dejaría el Kardex diciendo una cosa y la
 * orden otra; y una entrada sin costo no asienta, que es exactamente el
 * descuadre que la pantalla de valoración destapó en este mismo seed.
 */
func (s *Store) seedFabricacion(empID, sedeID string) {
	// El módulo queda INSTALADO Y ACTIVO, igual que delivery: si hubiera que
	// activarlo a mano, las órdenes sembradas existirían y el menú no las
	// mostraría — la demo arrancaría enseñando un módulo que no está.
	s.Modulos.Upsert(aplicacion.Instalacion{
		EmpresaID: empID, ModuloID: aplicacion.ModFabricacion,
		Instalado: true, Activo: true,
		Actualizada: time.Now().UTC().Format(time.RFC3339),
	})

	// El brownie: se hornea por bandejas y se vende de la vitrina. Receta escrita
	// PARA LA BANDEJA (20 unidades), no por unidad — que es como se escribe una
	// receta de verdad y lo que el módulo vino a soportar.
	brownie := s.Productos.Create(inventario.Producto{
		EmpresaID: empID, SKU: "POS-BROWNIE", Nombre: "Brownie de chocolate",
		Rubro: "Postres", UnidadBase: inventario.UnidadUnidad, TipoVenta: inventario.TipoVentaUnidad,
		Precio: 5500, Activo: true, EsPlato: true,
		ModoFabricacion: inventario.FabricaParaStock,
		// Una bandeja rinde 20, se pierden un par al cortar los bordes, y una
		// desviación de más del 10% merece que alguien mire.
		LoteBase: 20, RendimientoPct: 90, ToleranciaPct: 10,
		Receta: []inventario.ComboComponente{
			// La harina se tamiza y algo se pierde: la merma es del insumo.
			{SKU: "INS-HARINA", Cantidad: 0.900, MermaPct: 5},
			{SKU: "INS-CHOCOLATE", Cantidad: 0.700},
			{SKU: "INS-AZUCAR", Cantidad: 0.800},
			{SKU: "INS-HUEVO", Cantidad: 8},
			{SKU: "INS-MANTEQUILLA", Cantidad: 0.500},
		},
	})

	hace := func(h int) string {
		return time.Now().Add(-time.Duration(h) * time.Hour).UTC().Format(time.RFC3339Nano)
	}
	num := 0
	// costoDe usa el promedio del insumo tal como lo ve la aplicación: sembrar un
	// costo distinto del que el Kardex proyecta es sembrar un descuadre.
	costoDe := func(sku string) (string, float64) {
		p, ok := s.Productos.BySKU(empID, sku)
		if !ok {
			return "", 0
		}
		_, avg := foldSemilla(s.Movimientos.List(empID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: p.ID}))
		return p.ID, avg
	}

	// consumosDe arma lo que la orden saca del almacén, con el mismo factor que
	// usaría la aplicación: tanda, rendimiento y merma por insumo.
	consumosDe := func(objetivo float64) ([]fabricacion.Consumo, float64) {
		factor := brownie.FactorDeFormula(objetivo)
		out := make([]fabricacion.Consumo, 0, len(brownie.Receta))
		total := 0.0
		for _, comp := range brownie.Receta {
			pid, avg := costoDe(comp.SKU)
			if pid == "" {
				continue
			}
			ins, _ := s.Productos.BySKU(empID, comp.SKU)
			c := fabricacion.Consumo{
				SKU: comp.SKU, ProductoID: pid, Nombre: ins.Nombre,
				Cantidad: r2Semilla(comp.CantidadBruta(factor)), CostoUnitario: r2Semilla(avg),
			}
			out = append(out, c)
			total = r2Semilla(total + c.Total())
		}
		return out, total
	}

	crear := func(estado string, objetivo, producida float64, horas int) fabricacion.Orden {
		num++
		o := fabricacion.Orden{
			EmpresaID: empID, SedeID: sedeID,
			Numero: num, NumeroCompleto: fmt.Sprintf("OF-%06d", num),
			ProductoID: brownie.ID, SKU: brownie.SKU, Nombre: brownie.Nombre,
			Cantidad: objetivo, Estado: estado, Creada: hace(horas),
		}
		o.Bitacora = append(o.Bitacora, fabricacion.Evento{
			Cuando: o.Creada, Estado: fabricacion.EstadoBorrador, Actor: application.DemoUserID,
		})
		if estado == fabricacion.EstadoBorrador {
			return s.OrdenesFabricacion.Append(o)
		}

		// ARRANCAR: los insumos salen del almacén de verdad.
		consumos, total := consumosDe(objetivo)
		o.Consumos, o.CostoTotal = consumos, total
		o.Iniciada = hace(horas - 1)
		o.Bitacora = append(o.Bitacora, fabricacion.Evento{
			Cuando: o.Iniciada, Estado: fabricacion.EstadoEnProceso, Actor: application.DemoUserID,
		})
		for _, c := range consumos {
			s.appendMov(inventario.Movimiento{
				EmpresaID: empID, SedeID: sedeID, ProductoID: c.ProductoID, SKU: c.SKU,
				Tipo: inventario.MovSalida, Cantidad: -c.Cantidad, CostoUnitario: c.CostoUnitario,
				Motivo:  "fabricación " + o.NumeroCompleto,
				RefTipo: application.RefFabricacion, RefID: o.ID,
				Actor: application.DemoUserID, Fecha: o.Iniciada,
			})
		}
		if estado == fabricacion.EstadoEnProceso {
			return s.OrdenesFabricacion.Append(o)
		}

		// TERMINAR: entra lo producido, con el costo de lo que consumió repartido
		// entre lo que REALMENTE salió.
		o.CantidadProducida = producida
		if producida > 0 {
			o.CostoUnitario = r2Semilla(o.CostoTotal / producida)
		}
		o.FueraDeTolerancia = !brownie.DesviacionAceptable(objetivo, producida)
		o.Terminada = hace(horas - 2)
		o.Bitacora = append(o.Bitacora, fabricacion.Evento{
			Cuando: o.Terminada, Estado: fabricacion.EstadoTerminada, Actor: application.DemoUserID,
		})
		s.appendMov(inventario.Movimiento{
			EmpresaID: empID, SedeID: sedeID, ProductoID: brownie.ID, SKU: brownie.SKU,
			Tipo: inventario.MovEntrada, Cantidad: producida, CostoUnitario: o.CostoUnitario,
			Motivo:  "fabricación " + o.NumeroCompleto,
			RefTipo: application.RefFabricacion, RefID: o.ID,
			Actor: application.DemoUserID, Fecha: o.Terminada,
		})
		return s.OrdenesFabricacion.Append(o)
	}

	// 1. Ayer: una bandeja que salió completa. Enseña el costo derivado.
	crear(fabricacion.EstadoTerminada, 20, 20, 30)
	// 2. Esta mañana: salieron 16 de 20. Es el caso que más enseña — el costo de
	//    los 20 se reparte entre 16, y la desviación (−20%) queda MARCADA porque
	//    supera la tolerancia del 10%.
	crear(fabricacion.EstadoTerminada, 20, 16, 8)
	// 3. En el horno ahora: deja tocar «Terminar» y declarar cuánto salió.
	crear(fabricacion.EstadoEnProceso, 20, 0, 2)
	// 4. Planificada para la tarde: deja tocar «Arrancar» y ver los insumos salir.
	crear(fabricacion.EstadoBorrador, 40, 0, 1)

	s.Numerador.Fijar(empID, sedeID, "OF", num)
}

// foldSemilla y r2Semilla replican la proyección y el redondeo de la aplicación.
// Se repiten acá porque el sembrado no puede importar la capa de aplicación sin
// ciclo, y usar otra aritmética sembraría un descuadre.
func foldSemilla(movs []inventario.Movimiento) (float64, float64) {
	cant, avg := 0.0, 0.0
	for _, m := range movs {
		if m.Cantidad > 0 {
			nuevo := cant + m.Cantidad
			if nuevo > 0 {
				avg = ((cant * avg) + (m.Cantidad * m.CostoUnitario)) / nuevo
			}
			cant = nuevo
			continue
		}
		cant += m.Cantidad
	}
	return cant, avg
}

func r2Semilla(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

/* EL RECETARIO DE LA FARMACIA.
 *
 * Fabricación fuera de una cocina, que es lo que este caso enseña: una farmacia
 * que además de revender prepara sus propias fórmulas. Cambia el vocabulario
 * —materia prima en vez de insumo, preparado en vez de plato— y no cambia nada
 * del mecanismo.
 *
 * Y es el caso donde LOTE Y VENCIMIENTO dejan de ser opcionales: un preparado
 * magistral sale rotulado con los dos por ley, y dura semanas, no años. Sin
 * vencimiento en el sistema, el que caducó sigue apareciendo como disponible.
 */
func (s *Store) seedRecetarioFarmacia(empID, sedeID string) {
	s.Modulos.Upsert(aplicacion.Instalacion{
		EmpresaID: empID, ModuloID: aplicacion.ModFabricacion,
		Instalado: true, Activo: true,
		Actualizada: time.Now().UTC().Format(time.RFC3339),
	})

	hace := func(h int) time.Time { return time.Now().Add(-time.Duration(h) * time.Hour).UTC() }
	num := 0

	// preparar ejecuta una orden completa contra el ledger, igual que lo haría la
	// aplicación: consume la materia prima al arrancar e ingresa el preparado al
	// terminar, con su lote y su vencimiento.
	preparar := func(sku, lote string, objetivo, producida float64, diasVida, horas int) {
		p, ok := s.Productos.BySKU(empID, sku)
		if !ok {
			return
		}
		num++
		inicio := hace(horas)
		fin := hace(horas - 1)
		o := fabricacion.Orden{
			EmpresaID: empID, SedeID: sedeID,
			Numero: num, NumeroCompleto: fmt.Sprintf("OF-%06d", num),
			ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre,
			Cantidad: objetivo, Estado: fabricacion.EstadoEnProceso,
			Lote: lote,
			// El vencimiento se cuenta desde que se PREPARÓ, no desde hoy: es la
			// fecha que va en el rótulo del frasco.
			Vencimiento: inicio.AddDate(0, 0, diasVida).Format("2006-01-02"),
			Creada:      inicio.Format(time.RFC3339Nano),
			Iniciada:    inicio.Format(time.RFC3339Nano),
		}
		o.Bitacora = append(o.Bitacora,
			fabricacion.Evento{Cuando: o.Creada, Estado: fabricacion.EstadoBorrador, Actor: application.DemoUserID},
			fabricacion.Evento{Cuando: o.Iniciada, Estado: fabricacion.EstadoEnProceso, Actor: application.DemoUserID},
		)

		factor := p.FactorDeFormula(objetivo)
		for _, comp := range p.Receta {
			ins, ok := s.Productos.BySKU(empID, comp.SKU)
			if !ok {
				continue
			}
			_, avg := foldSemilla(s.Movimientos.List(empID, inventario.FiltroMovimiento{SedeID: sedeID, ProductoID: ins.ID}))
			c := fabricacion.Consumo{
				SKU: ins.SKU, ProductoID: ins.ID, Nombre: ins.Nombre,
				Cantidad: r2Semilla(comp.CantidadBruta(factor)), CostoUnitario: r2Semilla(avg),
			}
			o.Consumos = append(o.Consumos, c)
			o.CostoTotal = r2Semilla(o.CostoTotal + c.Total())
			s.appendMov(inventario.Movimiento{
				EmpresaID: empID, SedeID: sedeID, ProductoID: c.ProductoID, SKU: c.SKU,
				Tipo: inventario.MovSalida, Cantidad: -c.Cantidad, CostoUnitario: c.CostoUnitario,
				Motivo: "preparación " + o.NumeroCompleto, RefTipo: application.RefFabricacion, RefID: o.ID,
				Actor: application.DemoUserID, Fecha: o.Iniciada,
			})
		}

		if producida <= 0 { // queda en el mesón, preparándose
			s.OrdenesFabricacion.Append(o)
			return
		}
		o.CantidadProducida = producida
		o.CostoUnitario = r2Semilla(o.CostoTotal / producida)
		o.FueraDeTolerancia = !p.DesviacionAceptable(objetivo, producida)
		o.Terminada = fin.Format(time.RFC3339Nano)
		o.Estado = fabricacion.EstadoTerminada
		o.Bitacora = append(o.Bitacora, fabricacion.Evento{
			Cuando: o.Terminada, Estado: fabricacion.EstadoTerminada, Actor: application.DemoUserID,
		})
		s.appendMov(inventario.Movimiento{
			EmpresaID: empID, SedeID: sedeID, ProductoID: p.ID, SKU: p.SKU,
			Tipo: inventario.MovEntrada, Cantidad: producida, CostoUnitario: o.CostoUnitario,
			Lote: o.Lote, Vencimiento: o.Vencimiento,
			Motivo: "preparación " + o.NumeroCompleto, RefTipo: application.RefFabricacion, RefID: o.ID,
			Actor: application.DemoUserID, Fecha: o.Terminada,
		})
		s.OrdenesFabricacion.Append(o)
	}

	// Dos tandas de crema de urea con lotes distintos: es lo que permite ver el
	// rastro por lote y que una vence antes que la otra.
	preparar("MAG-UREA-10", "U-2609A", 20, 19, 60, 240)
	preparar("MAG-UREA-10", "U-2609B", 20, 20, 60, 72)
	// La pasta de zinc rindió por debajo de la tolerancia: alguien tiene que mirar
	// por qué de 15 potes salieron 12.
	preparar("MAG-OXIDO-ZN", "Z-2609A", 15, 12, 90, 120)
	// Y una en el mesón ahora mismo, para poder terminarla desde la pantalla.
	preparar("MAG-UREA-10", "U-2609C", 20, 0, 60, 3)

	s.Numerador.Fijar(empID, sedeID, "OF", num)
}
