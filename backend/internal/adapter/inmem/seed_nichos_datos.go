package inmem

import (
	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/mesa"
)

// especificacionesNicho son los DATOS de las demos por rubro. Están aparte del
// constructor (seed_nichos.go) porque son puro contenido: se ajustan sin tocar lógica.
//
// Criterio de los precios: en bolívares, con la tasa demo (~745 Bs/US$) para que las
// cifras se lean como reales. Las EXENCIONES de IVA no son decorativas: en Venezuela la
// harina de maíz, el arroz, el pan y los medicamentos están exentos, y facturarles IVA es
// un error fiscal, no un redondeo.
func especificacionesNicho() []especNicho {
	return []especNicho{restauranteDemo(), ferreteriaDemo(), farmaciaDemo()}
}

// --- Restaurante -----------------------------------------------------------

// El restaurante es el único con el módulo Restaurante activo: trae salón en grilla,
// mesas y platos con receta, así el prospecto ve comandera, cocina y escandallo con datos.
func restauranteDemo() especNicho {
	return especNicho{
		orgID: nichoRestOrgID, empID: nichoRestEmpID, sedeID: nichoRestSedeID,
		org:    "Grupo Gastronómico El Fogón",
		nombre: "Restaurante El Fogón, C.A.",
		rif:    "J-40218765-3",
		giro:   "restaurante", direccion: "Av. Francisco de Miranda, Caracas",
		colorMarca: "#B3362C",
		slug:       "elfogon", telefonoBanco: "0414-5567",
		cajero: "Yorman Piña", supervisor: "Rosa Delgado",
		vendedor: "Daniel Ochoa", contadora: "Lcda. Carmen Silva", mesonero: "Keiber Rojas",
		proveedores: []provNicho{
			{nombre: "Distribuidora de Alimentos Del Valle, C.A.", doc: "J-30871234-5", telefono: "0212-6651122"},
			{nombre: "Carnicería El Novillo, C.A.", doc: "J-31445566-0", telefono: "0212-7734455"},
			{nombre: "Frutas y Verduras La Cosecha", doc: "J-29887766-3", telefono: "0414-1239876"},
		},
		// Un mes de servicio: contado, una noche cobrada en divisas (IGTF), una
		// contingencia y dos cuentas corporativas a crédito.
		facturas: []emisionNicho{
			{diasAtras: 26, cliente: "Consumidor final", sku: "PLA-BOLONESA", cant: 3},
			{diasAtras: 23, cliente: "Luis Bermúdez", doc: "V-14875690", sku: "PLA-MILANESA", cant: 2},
			{diasAtras: 19, cliente: "Consumidor final", sku: "PLA-POLLO-ARROZ", cant: 4},
			{diasAtras: 16, cliente: "Consumidor final", sku: "PLA-ENSALADA", cant: 2, divisas: true},
			{diasAtras: 12, cliente: "Consumidor final", sku: "BEB-CERVEZA", cant: 12},
			{diasAtras: 9, cliente: "Consumidor final", sku: "PLA-PASTA-QUESO", cant: 3},
			{diasAtras: 6, cliente: "Consumidor final", sku: "POS-TORTA", cant: 5},
			{diasAtras: 3, cliente: "Consumidor final", sku: "PLA-BOLONESA", cant: 2, divisas: true},
			{diasAtras: 1, cliente: "Consumidor final", sku: "PLA-MILANESA", cant: 1},
			// Emitida SIN conexión (serie C reservada).
			{diasAtras: 4, serie: "C", cliente: "Consumidor final", sku: "BEB-REFRESCO", cant: 6, contingencia: true},
			// Almuerzos corporativos a crédito: una VENCIDA con abono parcial (hace que
			// «Por cobrar vencido» se vea en rojo) y una vigente sin abono.
			{diasAtras: 38, cliente: "Corporación Andina de Seguros, C.A.", doc: "J-30125678-4",
				sku: "PLA-POLLO-ARROZ", cant: 25, plazoDias: 15, abono: 200000},
			{diasAtras: 5, cliente: "Corporación Andina de Seguros, C.A.", doc: "J-30125678-4",
				sku: "PLA-MILANESA", cant: 18, plazoDias: 30},
		},
		rubros:  []string{"Insumos", "Cocina", "Bebidas", "Postres"},
		modulos: []string{aplicacion.ModRestaurante, aplicacion.ModAsistenteIA},

		// INSUMOS: lo que de verdad se stockea y se descuenta al vender un plato.
		// Van por peso/volumen porque una receta consume gramos, no unidades.
		productos: []prodNicho{
			{sku: "INS-PASTA", nombre: "Pasta larga (spaghetti)", rubro: "Insumos", costo: 3200, precio: 4800, stock: 40, porPeso: true, insumo: true},
			{sku: "INS-QUESO", nombre: "Queso parmesano", rubro: "Insumos", costo: 28000, precio: 42000, stock: 12, porPeso: true, insumo: true},
			{sku: "INS-CARNE", nombre: "Carne molida de res", rubro: "Insumos", costo: 21000, precio: 31000, stock: 25, porPeso: true, insumo: true},
			{sku: "INS-TOMATE", nombre: "Salsa de tomate natural", rubro: "Insumos", costo: 5400, precio: 8100, stock: 30, porPeso: true, insumo: true},
			{sku: "INS-POLLO", nombre: "Pechuga de pollo", rubro: "Insumos", costo: 14500, precio: 21000, stock: 30, porPeso: true, insumo: true},
			{sku: "INS-ARROZ", nombre: "Arroz blanco", rubro: "Insumos", costo: 2400, precio: 3600, stock: 50, porPeso: true, exento: true, insumo: true},
			{sku: "INS-PAPA", nombre: "Papa para freír", rubro: "Insumos", costo: 3900, precio: 5800, stock: 45, porPeso: true, insumo: true},
			{sku: "INS-LECHUGA", nombre: "Lechuga romana", rubro: "Insumos", costo: 4200, precio: 6300, stock: 10, porPeso: true, insumo: true},
			{sku: "INS-ACEITE", nombre: "Aceite de oliva", rubro: "Insumos", unidad: "litro", costo: 32000, precio: 46000, stock: 8, insumo: true},
			{sku: "INS-SAL", nombre: "Sal marina", rubro: "Insumos", costo: 900, precio: 1400, stock: 15, porPeso: true, insumo: true},
			// Bebidas y postres se venden tal cual (no llevan receta).
			{sku: "BEB-REFRESCO", nombre: "Refresco 355 ml", rubro: "Bebidas", unidad: "unidad", costo: 1100, precio: 2200, stock: 120},
			{sku: "BEB-AGUA", nombre: "Agua mineral 600 ml", rubro: "Bebidas", unidad: "unidad", costo: 700, precio: 1600, stock: 90},
			{sku: "BEB-CERVEZA", nombre: "Cerveza nacional 222 ml", rubro: "Bebidas", unidad: "unidad", costo: 1500, precio: 3200, stock: 150},
			{sku: "POS-TORTA", nombre: "Porción de torta de chocolate", rubro: "Postres", unidad: "unidad", costo: 3800, precio: 8500, stock: 18},
			// Agotado a propósito: enseña cómo se ve un insumo sin existencia.
			{sku: "INS-CAMARON", nombre: "Camarón pelado", rubro: "Insumos", costo: 62000, precio: 89000, stock: 0, porPeso: true, insumo: true},
		},

		// PLATOS (escandallo). Las cantidades están en la unidad del insumo: kg para los
		// que van por peso, litro para el aceite.
		platos: []platoNicho{
			{sku: "PLA-BOLONESA", nombre: "Spaghetti a la boloñesa", precio: 32000, receta: []inventario.ComboComponente{
				{SKU: "INS-PASTA", Cantidad: 0.140},
				{SKU: "INS-CARNE", Cantidad: 0.120},
				{SKU: "INS-TOMATE", Cantidad: 0.100},
				{SKU: "INS-QUESO", Cantidad: 0.020},
				{SKU: "INS-ACEITE", Cantidad: 0.010},
				{SKU: "INS-SAL", Cantidad: 0.004},
			}},
			{sku: "PLA-POLLO-ARROZ", nombre: "Pechuga a la plancha con arroz", precio: 29000, receta: []inventario.ComboComponente{
				{SKU: "INS-POLLO", Cantidad: 0.220},
				{SKU: "INS-ARROZ", Cantidad: 0.150},
				{SKU: "INS-ACEITE", Cantidad: 0.015},
				{SKU: "INS-SAL", Cantidad: 0.005},
			}},
			{sku: "PLA-MILANESA", nombre: "Milanesa de pollo con papas", precio: 34000, receta: []inventario.ComboComponente{
				{SKU: "INS-POLLO", Cantidad: 0.250},
				{SKU: "INS-PAPA", Cantidad: 0.200},
				{SKU: "INS-ACEITE", Cantidad: 0.030},
				{SKU: "INS-SAL", Cantidad: 0.005},
			}},
			{sku: "PLA-ENSALADA", nombre: "Ensalada César con pollo", precio: 26000, receta: []inventario.ComboComponente{
				{SKU: "INS-LECHUGA", Cantidad: 0.180},
				{SKU: "INS-POLLO", Cantidad: 0.130},
				{SKU: "INS-QUESO", Cantidad: 0.025},
				{SKU: "INS-ACEITE", Cantidad: 0.012},
			}},
			{sku: "PLA-PASTA-QUESO", nombre: "Pasta al burro con parmesano", precio: 24000, receta: []inventario.ComboComponente{
				{SKU: "INS-PASTA", Cantidad: 0.140},
				{SKU: "INS-QUESO", Cantidad: 0.040},
				{SKU: "INS-ACEITE", Cantidad: 0.015},
				{SKU: "INS-SAL", Cantidad: 0.004},
			}},
		},

		clientes: []cliNicho{
			{nombre: "Consumidor final", tipoDoc: "V", doc: "00000000", telefono: ""},
			{nombre: "Corporación Andina de Seguros, C.A.", tipoDoc: "J", doc: "301256784", telefono: "0212-7654321"},
			{nombre: "Luis Bermúdez", tipoDoc: "V", doc: "14875690", telefono: "0414-3216549"},
		},

		// Salón 8×6. Bloqueadas: la cocina (esquina superior derecha) y la barra
		// (columna izquierda abajo) — ahí no puede ir ninguna mesa.
		columnas: 8, filas: 6,
		bloqueadas: []mesa.Celda{
			{Columna: 7, Fila: 1}, {Columna: 8, Fila: 1},
			{Columna: 7, Fila: 2}, {Columna: 8, Fila: 2},
			{Columna: 1, Fila: 5}, {Columna: 1, Fila: 6},
		},
		mesas: []mesaNicho{
			{nombre: "1", zona: "Salón", forma: mesa.FormaCuadrada, capacidad: 4, columna: 2, fila: 1},
			{nombre: "2", zona: "Salón", forma: mesa.FormaCuadrada, capacidad: 4, columna: 4, fila: 1},
			{nombre: "3", zona: "Salón", forma: mesa.FormaRedonda, capacidad: 2, columna: 2, fila: 3},
			{nombre: "4", zona: "Salón", forma: mesa.FormaRedonda, capacidad: 2, columna: 4, fila: 3},
			{nombre: "5", zona: "Salón", forma: mesa.FormaRectangular, capacidad: 6, columna: 6, fila: 3},
			{nombre: "6", zona: "Terraza", forma: mesa.FormaCuadrada, capacidad: 4, columna: 3, fila: 5},
			{nombre: "7", zona: "Terraza", forma: mesa.FormaCuadrada, capacidad: 4, columna: 5, fila: 5},
			{nombre: "8", zona: "Terraza", forma: mesa.FormaRectangular, capacidad: 8, columna: 7, fila: 5},
		},
	}
}

// --- Ferretería ------------------------------------------------------------

// Catálogo amplio con rubros y unidades variadas (metro, kg, saco): la ferretería es el
// caso donde el prospecto quiere ver que el ERP no asume "todo se vende por unidad".
func ferreteriaDemo() especNicho {
	return especNicho{
		orgID: nichoFerrOrgID, empID: nichoFerrEmpID, sedeID: nichoFerrSedeID,
		org:    "Distribuidora Tornillo de Oro",
		nombre: "Ferretería Tornillo de Oro, C.A.",
		rif:    "J-31456982-7",
		giro:   "ferreteria", direccion: "Av. Intercomunal, Valencia",
		colorMarca: "#92600A",
		slug:       "tornillodeoro", telefonoBanco: "0241-8890",
		cajero: "Wilmer Castillo", supervisor: "Néstor Ramírez",
		vendedor: "Jhonny Peña", contadora: "Lcda. Yaneth Mora",
		proveedores: []provNicho{
			{nombre: "Importadora de Herramientas Andina, C.A.", doc: "J-30556677-8", telefono: "0241-8812233"},
			{nombre: "Cementos y Agregados del Centro, C.A.", doc: "J-29334455-1", telefono: "0241-8845566"},
			{nombre: "Electro Suministros Valencia, C.A.", doc: "J-31667788-2", telefono: "0241-8878899"},
		},
		facturas: []emisionNicho{
			{diasAtras: 28, cliente: "Consumidor final", sku: "HER-MAR-16", cant: 3},
			{diasAtras: 25, cliente: "Pedro Rangel (maestro de obra)", doc: "V-11298765", sku: "CON-CEM-42", cant: 20},
			{diasAtras: 21, cliente: "Consumidor final", sku: "ELE-CAB-12", cant: 50},
			{diasAtras: 18, cliente: "Inversiones Mardom, C.A.", doc: "J-31120987-5", sku: "HER-TAL-500", cant: 2, divisas: true},
			{diasAtras: 14, cliente: "Consumidor final", sku: "PLO-TUB-PVC", cant: 12},
			{diasAtras: 11, cliente: "Consumidor final", sku: "PIN-CAU-GAL", cant: 4},
			{diasAtras: 7, cliente: "Pedro Rangel (maestro de obra)", doc: "V-11298765", sku: "TOR-AUT-1", cant: 3.5},
			{diasAtras: 4, cliente: "Consumidor final", sku: "ELE-BRE-20", cant: 6},
			{diasAtras: 2, cliente: "Consumidor final", sku: "HER-CIN-5M", cant: 2},
			{diasAtras: 5, serie: "C", cliente: "Consumidor final", sku: "PLO-COD-PVC", cant: 20, contingencia: true},
			// Obra a crédito: vencida con abono, y una vigente a 30 días.
			{diasAtras: 42, cliente: "Constructora Los Andes, C.A.", doc: "J-29876453-1",
				sku: "CON-CAB-3/8", cant: 80, plazoDias: 15, abono: 500000},
			{diasAtras: 8, cliente: "Constructora Los Andes, C.A.", doc: "J-29876453-1",
				sku: "CON-CEM-42", cant: 60, plazoDias: 30},
		},
		rubros:  []string{"Herramientas", "Plomería", "Electricidad", "Construcción", "Pinturas", "Tornillería"},
		modulos: []string{aplicacion.ModAsistenteIA},
		productos: []prodNicho{
			{sku: "HER-TAL-500", nombre: "Taladro percutor 1/2\" 650 W", rubro: "Herramientas", unidad: "unidad", costo: 68000, precio: 112000, stock: 14},
			{sku: "HER-MAR-16", nombre: "Martillo uña 16 oz", rubro: "Herramientas", unidad: "unidad", costo: 9800, precio: 17500, stock: 40},
			{sku: "HER-LLA-JGO", nombre: "Juego de llaves mixtas 8 pzs", rubro: "Herramientas", unidad: "unidad", costo: 24000, precio: 41000, stock: 22},
			{sku: "HER-CIN-5M", nombre: "Cinta métrica 5 m", rubro: "Herramientas", unidad: "unidad", costo: 4200, precio: 8400, stock: 60},
			{sku: "PLO-TUB-PVC", nombre: "Tubo PVC 1/2\" (barra 6 m)", rubro: "Plomería", unidad: "unidad", costo: 7600, precio: 13200, stock: 85},
			{sku: "PLO-COD-PVC", nombre: "Codo PVC 1/2\" 90°", rubro: "Plomería", unidad: "unidad", costo: 620, precio: 1400, stock: 300},
			{sku: "PLO-LLA-PASO", nombre: "Llave de paso 1/2\" bronce", rubro: "Plomería", unidad: "unidad", costo: 8900, precio: 16000, stock: 35},
			{sku: "ELE-CAB-12", nombre: "Cable THW #12 (por metro)", rubro: "Electricidad", unidad: "metro", costo: 1150, precio: 2100, stock: 480},
			{sku: "ELE-BRE-20", nombre: "Breaker 20 A enchufable", rubro: "Electricidad", unidad: "unidad", costo: 6400, precio: 12500, stock: 48},
			{sku: "ELE-TOM-DOB", nombre: "Tomacorriente doble con placa", rubro: "Electricidad", unidad: "unidad", costo: 2700, precio: 5600, stock: 90},
			{sku: "CON-CEM-42", nombre: "Cemento gris saco 42,5 kg", rubro: "Construcción", unidad: "saco", costo: 34000, precio: 49000, stock: 120},
			{sku: "CON-ARE-M3", nombre: "Arena lavada (m³)", rubro: "Construcción", unidad: "m3", costo: 78000, precio: 115000, stock: 9},
			{sku: "CON-CAB-3/8", nombre: "Cabilla estriada 3/8\" (12 m)", rubro: "Construcción", unidad: "unidad", costo: 18500, precio: 28000, stock: 140},
			{sku: "PIN-CAU-GAL", nombre: "Pintura caucho blanca (galón)", rubro: "Pinturas", unidad: "galón", costo: 42000, precio: 68000, stock: 30},
			{sku: "PIN-BRO-4", nombre: "Brocha 4\"", rubro: "Pinturas", unidad: "unidad", costo: 3100, precio: 6200, stock: 55},
			{sku: "TOR-AUT-1", nombre: "Tornillo autorroscante 1\" (por kg)", rubro: "Tornillería", costo: 8200, precio: 14500, stock: 26, porPeso: true},
			{sku: "TOR-CLA-25", nombre: "Clavo 2 1/2\" (por kg)", rubro: "Tornillería", costo: 5600, precio: 9800, stock: 40, porPeso: true},
			// Agotado: el clásico "se acabó el silicón".
			{sku: "PIN-SIL-TUB", nombre: "Silicón transparente (tubo)", rubro: "Pinturas", unidad: "unidad", costo: 5200, precio: 9500, stock: 0},
		},
		clientes: []cliNicho{
			{nombre: "Consumidor final", tipoDoc: "V", doc: "00000000", telefono: ""},
			{nombre: "Constructora Los Andes, C.A.", tipoDoc: "J", doc: "298764531", telefono: "0241-8765432"},
			{nombre: "Inversiones Mardom, C.A.", tipoDoc: "J", doc: "311209875", telefono: "0241-5551234"},
			{nombre: "Pedro Rangel (maestro de obra)", tipoDoc: "V", doc: "11298765", telefono: "0424-1122334"},
		},
	}
}

// --- Farmacia --------------------------------------------------------------

// La farmacia es el caso de las EXENCIONES: los medicamentos no llevan IVA. El catálogo
// mezcla exentos (medicinas) con gravados (cuidado personal) para que la factura de la
// demo muestre las dos bases separadas.
func farmaciaDemo() especNicho {
	return especNicho{
		orgID: nichoFarmOrgID, empID: nichoFarmEmpID, sedeID: nichoFarmSedeID,
		org:    "Grupo Farmacéutico Santa Rosa",
		nombre: "Farmacia Santa Rosa, C.A.",
		rif:    "J-29873456-1",
		giro:   "farmacia", direccion: "Calle 72 con Av. 15, Maracaibo",
		colorMarca: "#166B41",
		slug:       "santarosa", telefonoBanco: "0261-7745",
		cajero: "Mariana Vílchez", supervisor: "Alberto Fuenmayor",
		vendedor: "Gabriel Urdaneta", contadora: "Lcda. Zaida Pirela",
		proveedores: []provNicho{
			{nombre: "Droguería Nacional, C.A.", doc: "J-30112233-4", telefono: "0261-7712345"},
			{nombre: "Laboratorios Vargas, C.A.", doc: "J-00034567-8", telefono: "0212-2029000"},
			{nombre: "Distribuidora de Cuidado Personal Zulia, C.A.", doc: "J-31556677-9", telefono: "0261-7756789"},
		},
		// Mezcla a propósito ventas de EXENTOS (medicinas) y GRAVADOS (cuidado
		// personal): así los libros fiscales muestran las dos bases separadas.
		facturas: []emisionNicho{
			{diasAtras: 27, cliente: "Consumidor final", sku: "MED-ACE-500", cant: 4},
			{diasAtras: 24, cliente: "Ana Rodríguez", doc: "V-17654321", sku: "CUI-SHA-400", cant: 2},
			{diasAtras: 20, cliente: "Consumidor final", sku: "MED-AMO-500", cant: 3},
			{diasAtras: 17, cliente: "Consumidor final", sku: "CUI-PRO-50", cant: 2, divisas: true},
			{diasAtras: 13, cliente: "Consumidor final", sku: "BEB-PAN-M", cant: 3},
			{diasAtras: 10, cliente: "Ana Rodríguez", doc: "V-17654321", sku: "VIT-CVI-1000", cant: 2},
			{diasAtras: 6, cliente: "Consumidor final", sku: "MED-OME-20", cant: 5},
			{diasAtras: 3, cliente: "Consumidor final", sku: "MAT-JER-5", cant: 20},
			{diasAtras: 1, cliente: "Consumidor final", sku: "CUI-CRE-DEN", cant: 6},
			{diasAtras: 4, serie: "C", cliente: "Consumidor final", sku: "MED-LOR-10", cant: 2, contingencia: true},
			// Convenios institucionales a crédito.
			{diasAtras: 40, cliente: "Clínica Materno Infantil, C.A.", doc: "J-30567891-2",
				sku: "MED-INS-FRA", cant: 8, plazoDias: 15, abono: 150000},
			{diasAtras: 7, cliente: "Seguros Altamira, C.A.", doc: "J-31234567-8",
				sku: "MAT-TEN-DIG", cant: 3, plazoDias: 30},
		},
		rubros:  []string{"Medicamentos", "Cuidado personal", "Bebé", "Vitaminas", "Material médico"},
		modulos: []string{aplicacion.ModAsistenteIA},
		productos: []prodNicho{
			// Exentos: medicamentos y material médico.
			{sku: "MED-ACE-500", nombre: "Acetaminofén 500 mg (caja x20)", rubro: "Medicamentos", unidad: "unidad", costo: 2800, precio: 5400, stock: 140, exento: true},
			{sku: "MED-IBU-400", nombre: "Ibuprofeno 400 mg (caja x20)", rubro: "Medicamentos", unidad: "unidad", costo: 3400, precio: 6500, stock: 110, exento: true},
			{sku: "MED-AMO-500", nombre: "Amoxicilina 500 mg (caja x12)", rubro: "Medicamentos", unidad: "unidad", costo: 7900, precio: 14200, stock: 65, exento: true},
			{sku: "MED-LOR-10", nombre: "Loratadina 10 mg (caja x10)", rubro: "Medicamentos", unidad: "unidad", costo: 2100, precio: 4300, stock: 95, exento: true},
			{sku: "MED-OME-20", nombre: "Omeprazol 20 mg (caja x14)", rubro: "Medicamentos", unidad: "unidad", costo: 4600, precio: 8700, stock: 78, exento: true},
			{sku: "MED-INS-FRA", nombre: "Insulina NPH (frasco 10 ml)", rubro: "Medicamentos", unidad: "unidad", costo: 46000, precio: 72000, stock: 18, exento: true},
			{sku: "MAT-JER-5", nombre: "Jeringa 5 ml estéril", rubro: "Material médico", unidad: "unidad", costo: 480, precio: 1100, stock: 400, exento: true},
			{sku: "MAT-GAS-EST", nombre: "Gasa estéril 10x10 (sobre)", rubro: "Material médico", unidad: "unidad", costo: 1200, precio: 2600, stock: 220, exento: true},
			{sku: "MAT-TEN-DIG", nombre: "Tensiómetro digital de brazo", rubro: "Material médico", unidad: "unidad", costo: 92000, precio: 148000, stock: 7, exento: true},
			// Gravados: cuidado personal, bebé y vitaminas de venta libre.
			{sku: "CUI-JAB-AVE", nombre: "Jabón de avena 110 g", rubro: "Cuidado personal", unidad: "unidad", costo: 1600, precio: 3400, stock: 130},
			{sku: "CUI-SHA-400", nombre: "Shampoo anticaspa 400 ml", rubro: "Cuidado personal", unidad: "unidad", costo: 8400, precio: 15500, stock: 60},
			{sku: "CUI-CRE-DEN", nombre: "Crema dental 90 g", rubro: "Cuidado personal", unidad: "unidad", costo: 2300, precio: 4800, stock: 150},
			{sku: "CUI-PRO-50", nombre: "Protector solar FPS 50 (120 ml)", rubro: "Cuidado personal", unidad: "unidad", costo: 21000, precio: 36000, stock: 25},
			{sku: "BEB-PAN-M", nombre: "Pañales talla M (paquete x40)", rubro: "Bebé", unidad: "unidad", costo: 26000, precio: 42000, stock: 44},
			{sku: "BEB-TOA-HUM", nombre: "Toallitas húmedas x80", rubro: "Bebé", unidad: "unidad", costo: 6200, precio: 11800, stock: 70},
			{sku: "VIT-CVI-1000", nombre: "Vitamina C 1000 mg (x30)", rubro: "Vitaminas", unidad: "unidad", costo: 7400, precio: 13900, stock: 85},
			{sku: "VIT-OME-3", nombre: "Omega 3 (x60 cápsulas)", rubro: "Vitaminas", unidad: "unidad", costo: 18500, precio: 31000, stock: 32},
			// Agotado: el que siempre falta.
			{sku: "MED-SAL-INH", nombre: "Salbutamol inhalador", rubro: "Medicamentos", unidad: "unidad", costo: 24000, precio: 39000, stock: 0, exento: true},
		},
		clientes: []cliNicho{
			{nombre: "Consumidor final", tipoDoc: "V", doc: "00000000", telefono: ""},
			{nombre: "Clínica Materno Infantil, C.A.", tipoDoc: "J", doc: "305678912", telefono: "0261-7778899"},
			{nombre: "Seguros Altamira, C.A.", tipoDoc: "J", doc: "312345678", telefono: "0261-4443322"},
			{nombre: "Ana Rodríguez", tipoDoc: "V", doc: "17654321", telefono: "0416-9988776"},
		},
	}
}
