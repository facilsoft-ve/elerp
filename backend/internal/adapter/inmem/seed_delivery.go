package inmem

import (
	"fmt"
	"time"

	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/pedido"
)

/* CASO DE USO DEL MÓDULO DE PEDIDOS, sembrado para la demostración.
 *
 * Un módulo vacío no se puede mostrar: la bandeja sin pedidos enseña un estado
 * vacío, no cómo se trabaja. Así que la demo trae el ciclo ENTERO a la vista —
 * algo entrando, algo cocinándose, algo en la calle, algo entregado y algo que
 * salió mal— porque el valor del módulo se entiende viendo el movimiento, no la
 * pantalla en reposo.
 *
 * Los pedidos se anclan a la hora ACTUAL y no a una fecha fija: uno que entró
 * «hace tres minutos» con su reloj de aceptación corriendo se lee como un local
 * trabajando; los mismos datos con fecha de julio se leen como un archivo.
 *
 * Se siembra en las dos demos que reparten —la bodega y el restaurante— porque
 * son los dos casos que el módulo tiene que demostrar que sirve: uno SIN cocina,
 * donde el pedido se empaca y alguien lo marca listo, y otro CON cocina, donde la
 * comanda va al tablero. Es el mismo flujo, y verlo en los dos es la mitad del
 * argumento.
 */

// seedDelivery siembra canales, zonas, repartidores y un tablero de pedidos vivo.
func (s *Store) seedDelivery(empID, sedeID string, conCocina bool) {
	ahora := time.Now().UTC()
	fecha := ahora.Format(time.RFC3339Nano)

	// El módulo queda INSTALADO Y ACTIVO: si hubiera que activarlo a mano, la demo
	// arrancaría con el menú incompleto y la primera impresión sería un módulo que
	// no está.
	s.Modulos.Upsert(aplicacion.Instalacion{
		EmpresaID: empID, ModuloID: aplicacion.ModDelivery,
		Instalado: true, Activo: true, Actualizada: fecha,
	})

	/* CANALES: los tres casos reales conviviendo, que es justo lo que el módulo
	 * promete. La tienda propia confirma sola; las apps se revisan (una de ellas
	 * además reparte con su flota, así que el local no despacha). */
	tienda := s.CanalesPedido.Upsert(pedido.Canal{
		EmpresaID: empID, SedeID: sedeID, Nombre: "Tienda web", Origen: pedido.OrigenEcommerce,
		Activo: true, ConfirmacionAutomatica: true, Creado: fecha, Actualizado: fecha,
	})
	yummy := s.CanalesPedido.Upsert(pedido.Canal{
		EmpresaID: empID, SedeID: sedeID, Nombre: "Yummy", Origen: pedido.OrigenAppCommerce,
		Activo: true, MinutosAceptacion: 8, EnvioPropioDelCanal: true, Creado: fecha, Actualizado: fecha,
	})
	pedidosya := s.CanalesPedido.Upsert(pedido.Canal{
		EmpresaID: empID, SedeID: sedeID, Nombre: "PedidosYa", Origen: pedido.OrigenAppCommerce,
		Activo: true, MinutosAceptacion: 10, Creado: fecha, Actualizado: fecha,
	})

	// ZONAS: de la más cercana a la más lejana, con su costo y su promesa. La
	// cercana gana sobre la lejana que también alcanza.
	s.ZonasPedido.Upsert(pedido.Zona{
		EmpresaID: empID, SedeID: sedeID, Nombre: "Cercanías", RadioM: 2500,
		CostoEnvio: 30, MinutosPromesa: 30, Activa: true,
	})
	s.ZonasPedido.Upsert(pedido.Zona{
		EmpresaID: empID, SedeID: sedeID, Nombre: "Zona media", RadioM: 6000,
		CostoEnvio: 55, MinutosPromesa: 45, PedidoMinimo: 200, Activa: true,
	})
	s.ZonasPedido.Upsert(pedido.Zona{
		EmpresaID: empID, SedeID: sedeID, Nombre: "Zona extendida", RadioM: 12000,
		CostoEnvio: 90, MinutosPromesa: 70, PedidoMinimo: 400,
		ModoEnvio: pedido.EnvioApp, Activa: true,
	})

	// REPARTIDORES: dos disponibles y uno ocupado, para que asignar sea una
	// decisión y no un trámite de un solo botón.
	rafa := s.Repartidores.Upsert(pedido.Repartidor{
		EmpresaID: empID, SedeID: sedeID, Codigo: "RP-01", Nombre: "Rafael Mendoza",
		Telefono: "0414-5551122", Vehiculo: pedido.VehiculoMoto, Activo: true, Disponible: true, Creado: fecha,
	})
	s.Repartidores.Upsert(pedido.Repartidor{
		EmpresaID: empID, SedeID: sedeID, Codigo: "RP-02", Nombre: "Keiber Salazar",
		Telefono: "0424-5553344", Vehiculo: pedido.VehiculoMoto, Activo: true, Disponible: true, Creado: fecha,
	})
	s.Repartidores.Upsert(pedido.Repartidor{
		EmpresaID: empID, SedeID: sedeID, Codigo: "RP-03", Nombre: "Daniel Ochoa",
		Telefono: "0412-5559988", Vehiculo: pedido.VehiculoBicicleta, Activo: true, Disponible: false, Creado: fecha,
	})

	// Los productos salen del catálogo ya sembrado: un pedido demo con SKU
	// inventado no se podría confirmar ni facturar, y la demostración se cortaría
	// justo donde importa.
	prods := s.Productos.List(empID)
	if len(prods) == 0 {
		return
	}
	item := func(i int, cant float64) pedido.Item {
		p := prods[i%len(prods)]
		return pedido.Item{SKU: p.SKU, Nombre: p.Nombre, Cantidad: cant, PrecioUnitario: p.Precio}
	}
	total := func(items []pedido.Item) float64 {
		t := 0.0
		for _, x := range items {
			t += x.Cantidad * x.PrecioUnitario
		}
		return t
	}

	num := 100
	hace := func(min int) string { return ahora.Add(-time.Duration(min) * time.Minute).Format(time.RFC3339) }
	dentro := func(min int) string { return ahora.Add(time.Duration(min) * time.Minute).Format(time.RFC3339) }

	// crear arma un pedido ya posicionado en su estado, con la bitácora del
	// recorrido que lo llevó ahí. Se escribe la historia completa y no solo el
	// estado final porque la ficha muestra el historial, y un pedido «entregado»
	// sin recorrido se ve como un dato inventado — que es lo que sería.
	crear := func(p pedido.Pedido, pasos []struct {
		estado string
		hace   int
	}) {
		num++
		p.EmpresaID, p.SedeID, p.Numero = empID, sedeID, num
		p.TokenPublico = fmt.Sprintf("demo%02d%s", num, empID[len(empID)-4:])
		p.Creado = hace(pasos[0].hace)
		for _, s0 := range pasos {
			p.Bitacora = append(p.Bitacora, pedido.Evento{
				Cuando: hace(s0.hace), Estado: s0.estado, Actor: "demo", Origen: "seed",
			})
		}
		p.Estado = pasos[len(pasos)-1].estado
		s.Pedidos.Append(p)
	}
	type paso = struct {
		estado string
		hace   int
	}

	// 1. ENTRANDO: llegó por Yummy hace tres minutos y el reloj corre. Es lo
	//    primero que hay que ver al abrir la bandeja.
	it1 := []pedido.Item{item(0, 2), item(3, 1)}
	crear(pedido.Pedido{
		Origen: pedido.OrigenAppCommerce, CanalID: yummy.ID, CanalNombre: yummy.Nombre,
		ReferenciaExterna: "YMY-48213", ClienteNombre: "Mariana Figueroa",
		Destino: pedido.Destino{
			Direccion:  "Av. Rómulo Gallegos, Res. Los Samanes, apto 4-B",
			Referencia: "frente a la panadería", Telefono: "0414-7788990",
		},
		Items: it1, Total: total(it1), FormaPago: pedido.PagoEnCanal,
		VenceAceptacion: dentro(5),
	}, []paso{{pedido.EstadoNuevo, 3}})

	// 2. ENTRANDO por la tienda web, esperando revisión desde hace un rato: el
	//    contraste con el anterior muestra que la ventana es por canal.
	it2 := []pedido.Item{item(1, 1)}
	crear(pedido.Pedido{
		Origen: pedido.OrigenEcommerce, CanalID: pedidosya.ID, CanalNombre: pedidosya.Nombre,
		ReferenciaExterna: "PY-90771", ClienteNombre: "Jesús Villalobos",
		Destino: pedido.Destino{
			Direccion: "Calle Los Pinos, quinta Carolina", Referencia: "portón verde",
			Telefono: "0424-3312244",
		},
		Items: it2, Total: total(it2), FormaPago: pedido.PagoContraEntrega,
		VenceAceptacion: dentro(2),
	}, []paso{{pedido.EstadoNuevo, 8}})

	// 3. EN PREPARACIÓN: confirmado y trabajándose. En el restaurante su comanda
	//    está en cocina; en la bodega alguien lo está empacando. Mismo estado.
	it3 := []pedido.Item{item(2, 3), item(4, 1)}
	crear(pedido.Pedido{
		Origen: pedido.OrigenEcommerce, CanalID: tienda.ID, CanalNombre: tienda.Nombre,
		ReferenciaExterna: "WEB-1204", ClienteNombre: "Carmen Rodríguez",
		Destino: pedido.Destino{
			Direccion: "Av. Principal de Los Ruices, Edif. Tamanaco, piso 3",
			Telefono:  "0212-2345678", ZonaNombre: "Cercanías",
		},
		Items: it3, Total: total(it3), CostoEnvio: 30, FormaPago: pedido.PagoEnCanal,
		ModoEnvio: pedido.EnvioPropio, PromesaEntrega: dentro(18),
	}, []paso{{pedido.EstadoNuevo, 22}, {pedido.EstadoConfirmado, 21}, {pedido.EstadoEnPreparacion, 20}})

	// 4. LISTO esperando repartidor: con su número de envío ya emitido. Es donde
	//    se ve que el tracking nace al estar listo, no al confirmar.
	it4 := []pedido.Item{item(5, 2)}
	crear(pedido.Pedido{
		Origen: pedido.OrigenManual, ClienteNombre: "Pedro Salas",
		Destino: pedido.Destino{
			Direccion: "Calle El Bosque, casa 12", Referencia: "al lado del abasto",
			Telefono: "0416-8899001", ZonaNombre: "Zona media",
		},
		Items: it4, Total: total(it4), CostoEnvio: 55, FormaPago: pedido.PagoContraEntrega,
		ModoEnvio: pedido.EnvioPropio, Tracking: "DLV-2609-0041", PromesaEntrega: dentro(25),
	}, []paso{{pedido.EstadoNuevo, 30}, {pedido.EstadoConfirmado, 29}, {pedido.EstadoEnPreparacion, 28}, {pedido.EstadoListo, 6}})

	// 5. EN LA CALLE con repartidor propio y cobro contra entrega: el caso donde
	//    el repartidor vuelve con plata y liquida.
	it5 := []pedido.Item{item(0, 1), item(6, 2)}
	crear(pedido.Pedido{
		Origen: pedido.OrigenManual, ClienteNombre: "Familia Contreras",
		Destino: pedido.Destino{
			Direccion: "Urb. Santa Eduvigis, Res. Mirador, apto 9-A",
			Telefono:  "0414-1122334", ZonaNombre: "Cercanías",
		},
		Items: it5, Total: total(it5), CostoEnvio: 30, FormaPago: pedido.PagoContraEntrega,
		ModoEnvio: pedido.EnvioPropio, Tracking: "DLV-2609-0040",
		RepartidorID: rafa.ID, RepartidorNombre: rafa.Nombre, PromesaEntrega: dentro(8),
	}, []paso{{pedido.EstadoNuevo, 45}, {pedido.EstadoConfirmado, 44}, {pedido.EstadoEnPreparacion, 43},
		{pedido.EstadoListo, 20}, {pedido.EstadoAsignado, 18}, {pedido.EstadoEnRuta, 14}})

	// 6. DEMORADO en la calle: la promesa venció. Se ve en rojo, como una comanda
	//    demorada — sin un caso así, la señal no se muestra nunca.
	it6 := []pedido.Item{item(7, 1)}
	crear(pedido.Pedido{
		Origen: pedido.OrigenAppCommerce, CanalID: pedidosya.ID, CanalNombre: pedidosya.Nombre,
		ReferenciaExterna: "PY-90utf", ClienteNombre: "Andreína Pérez",
		Destino: pedido.Destino{
			Direccion: "Av. Sucre, Res. La Floresta, torre B", Telefono: "0426-5544332",
			ZonaNombre: "Zona extendida",
		},
		Items: it6, Total: total(it6), CostoEnvio: 90, FormaPago: pedido.PagoEnCanal,
		ModoEnvio: pedido.EnvioPropio, Tracking: "DLV-2609-0039",
		RepartidorID: rafa.ID, RepartidorNombre: rafa.Nombre,
		PromesaEntrega: hace(12), // vencida: aparece marcado
	}, []paso{{pedido.EstadoNuevo, 80}, {pedido.EstadoConfirmado, 79}, {pedido.EstadoEnPreparacion, 78},
		{pedido.EstadoListo, 55}, {pedido.EstadoAsignado, 53}, {pedido.EstadoEnRuta, 50}})

	// 7. RETIRADO POR LA APP: Yummy reparte con su flota, así que el local no
	//    despacha. Es el caso que explica por qué no se ofrece asignar repartidor.
	it7 := []pedido.Item{item(3, 2)}
	crear(pedido.Pedido{
		Origen: pedido.OrigenAppCommerce, CanalID: yummy.ID, CanalNombre: yummy.Nombre,
		ReferenciaExterna: "YMY-48190", ClienteNombre: "Luis Bermúdez",
		Destino: pedido.Destino{Direccion: "Av. Libertador, Edif. Caroní, piso 7", Telefono: "0414-3216549"},
		Items:   it7, Total: total(it7), FormaPago: pedido.PagoEnCanal,
		ModoEnvio: pedido.EnvioCanal, Tracking: "DLV-2609-0038", EnvioExternoID: "YMY-SHIP-7711",
	}, []paso{{pedido.EstadoNuevo, 60}, {pedido.EstadoConfirmado, 59}, {pedido.EstadoEnPreparacion, 58},
		{pedido.EstadoListo, 40}, {pedido.EstadoRetiradoCanal, 36}})

	// 8. ENTREGADO con prueba: el cierre normal, que es contra lo que se compara
	//    todo lo demás.
	it8 := []pedido.Item{item(2, 1), item(5, 1)}
	p8 := pedido.Pedido{
		Origen: pedido.OrigenEcommerce, CanalID: tienda.ID, CanalNombre: tienda.Nombre,
		ReferenciaExterna: "WEB-1198", ClienteNombre: "Rosa Angulo",
		Destino: pedido.Destino{Direccion: "Calle Bolívar, casa 45", Telefono: "0412-7778899", ZonaNombre: "Cercanías"},
		Items:   it8, Total: total(it8), CostoEnvio: 30, FormaPago: pedido.PagoEnCanal,
		ModoEnvio: pedido.EnvioPropio, Tracking: "DLV-2609-0035",
		RepartidorID: rafa.ID, RepartidorNombre: rafa.Nombre,
		PruebaEntrega: "recibió la señora Rosa", Cerrado: hace(95),
	}
	crear(p8, []paso{{pedido.EstadoNuevo, 140}, {pedido.EstadoConfirmado, 139}, {pedido.EstadoEnPreparacion, 138},
		{pedido.EstadoListo, 120}, {pedido.EstadoAsignado, 118}, {pedido.EstadoEnRuta, 115}, {pedido.EstadoEntregado, 95}})

	// 9. RECHAZADO con motivo: lo que pasa cuando no se puede, y por qué. Un
	//    módulo que solo muestra el camino feliz no enseña a operarlo.
	it9 := []pedido.Item{item(6, 1)}
	crear(pedido.Pedido{
		Origen: pedido.OrigenAppCommerce, CanalID: pedidosya.ID, CanalNombre: pedidosya.Nombre,
		ReferenciaExterna: "PY-90655", ClienteNombre: "Gustavo Rivas",
		Destino: pedido.Destino{Direccion: "Guarenas, sector La Vaquera", Telefono: "0416-1231234"},
		Items:   it9, Total: total(it9), FormaPago: pedido.PagoEnCanal,
		MotivoCierre: "la dirección está fuera de nuestras zonas de reparto", Cerrado: hace(150),
	}, []paso{{pedido.EstadoNuevo, 155}, {pedido.EstadoRechazado, 150}})

	// 10. ENTREGA FALLIDA: el cliente no estaba. Queda abierta, esperando decisión
	//     —reintentar o devolver—, que es precisamente el caso que hay que mostrar.
	it10 := []pedido.Item{item(4, 2)}
	crear(pedido.Pedido{
		Origen: pedido.OrigenManual, ClienteNombre: "Oswaldo Graterol",
		Destino: pedido.Destino{
			Direccion: "Av. Casanova, Edif. Las Delicias, apto 12-C", Telefono: "0424-9988776",
			ZonaNombre: "Cercanías",
		},
		Items: it10, Total: total(it10), CostoEnvio: 30, FormaPago: pedido.PagoContraEntrega,
		ModoEnvio: pedido.EnvioPropio, Tracking: "DLV-2609-0037",
		RepartidorID: rafa.ID, RepartidorNombre: rafa.Nombre,
	}, []paso{{pedido.EstadoNuevo, 190}, {pedido.EstadoConfirmado, 189}, {pedido.EstadoEnPreparacion, 188},
		{pedido.EstadoListo, 175}, {pedido.EstadoAsignado, 173}, {pedido.EstadoEnRuta, 170}, {pedido.EstadoEntregaFallida, 160}})

	// El contador arranca por encima de lo sembrado: el primer pedido real del
	// prospecto no puede repetir un número que ya está en la bandeja.
	s.Numerador.Fijar(empID, sedeID, "PED", num)
	s.Numerador.Fijar(empID, sedeID, "DLV", 41)
	_ = conCocina
}
