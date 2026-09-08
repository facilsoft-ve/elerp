package inmem

import (
	"fmt"
	"math"
	"time"

	"github.com/mornix/elerp/internal/application"
	"github.com/mornix/elerp/internal/domain/cliente"

	"github.com/mornix/elerp/internal/domain/aplicacion"
	"github.com/mornix/elerp/internal/domain/caja"
	"github.com/mornix/elerp/internal/domain/compra"
	"github.com/mornix/elerp/internal/domain/cotizacion"
	"github.com/mornix/elerp/internal/domain/credencial"
	"github.com/mornix/elerp/internal/domain/cupon"
	"github.com/mornix/elerp/internal/domain/empresa"
	"github.com/mornix/elerp/internal/domain/fiscal"
	"github.com/mornix/elerp/internal/domain/inventario"
	"github.com/mornix/elerp/internal/domain/listaprecio"
	"github.com/mornix/elerp/internal/domain/mesa"
	"github.com/mornix/elerp/internal/domain/organizacion"
	"github.com/mornix/elerp/internal/domain/plantilla"
	"github.com/mornix/elerp/internal/domain/promocion"
	"github.com/mornix/elerp/internal/domain/proveedor"
	"github.com/mornix/elerp/internal/domain/sede"
	"github.com/mornix/elerp/internal/domain/tasa"
	"github.com/mornix/elerp/internal/domain/unidadmedida"
	"github.com/mornix/elerp/internal/domain/usuario"
)

// round2Demo redondea a 2 decimales para los datos sembrados (misma regla que
// round2 de la capa application, que no es exportada).
func round2Demo(v float64) float64 { return math.Round(v*100) / 100 }

// Store agrupa todos los repos en memoria (equivalente al window.DB del
// prototipo). Se siembra con una empresa demo para el modo DEV_LOGIN.
type Store struct {
	Productos        *ProductoRepo
	Movimientos      *MovimientoRepo
	Transferencias   *TransferenciaRepo
	Rubros           *RubroRepo
	Audit            *AuditRepo
	Organizaciones   *OrganizacionRepo
	Empresas         *EmpresaRepo
	Sedes            *SedeRepo
	Usuarios         *UsuarioRepo
	Membresias       *MembresiaRepo
	Credenciales     *CredencialRepo
	Clientes         *ClienteRepo
	Documentos       *DocumentoRepo
	Numerador        *NumeradorRepo
	CuentasCobro     *CuentaCobroRepo
	MetodosPago      *MetodoPagoRepo
	CierresZ         *CierreZRepo
	Cotizaciones     *CotizacionRepo
	Cajas            *CajaRepo
	Cajeros          *CajeroRepo
	SesionesCaja     *SesionCajaRepo
	Tasas            *TasaRepo
	VentasEnEspera   *VentaEnEsperaRepo
	Cobros           *CobroRepo
	PagosProveedor   *PagoProveedorRepo
	CuentasContables *CuentaContableRepo
	Asientos         *AsientoRepo
	Periodos         *PeriodoRepo
	Proveedores      *ProveedorRepo
	OrdenesCompra    *OrdenCompraRepo
	FacturasCompra   *FacturaCompraRepo
	Solicitudes      *SolicitudCompraRepo
	Retenciones      *RetencionRepo
	Dispositivos     *DispositivoFiscalRepo
	ListasPrecio     *ListaPrecioRepo
	Cupones          *CuponRepo
	Promociones      *PromocionRepo
	Unidades         *UnidadMedidaRepo
	Almacenes        *AlmacenRepo
	Modulos          *ModuloRepo
	DemoLeads        *DemoLeadRepo
	Legal            *LegalRepo
	Plantillas       *PlantillaRepo
	Mesas            *MesaRepo
	Planos           *PlanoRepo
	Impresoras       *ImpresoraRepo
	Cuentas          *CuentaRepo
}

// IDs fijos del seed demo (facilitan que el frontend seleccione contexto).
const (
	demoOrgID   = "org_demo"
	demoEmpID   = "emp_demo"
	demoSede1ID = "sede_demo_1"
	demoSede2ID = "sede_demo_2"
)

// New construye el Store en memoria ya sembrado con la empresa demo.
func New() *Store {
	s := &Store{
		Productos: NewProductoRepo(), Movimientos: NewMovimientoRepo(),
		Transferencias: NewTransferenciaRepo(), Rubros: NewRubroRepo(), Audit: NewAuditRepo(),
		Organizaciones: NewOrganizacionRepo(), Empresas: NewEmpresaRepo(), Sedes: NewSedeRepo(),
		Usuarios: NewUsuarioRepo(), Membresias: NewMembresiaRepo(), Credenciales: NewCredencialRepo(),
		Clientes: NewClienteRepo(), Documentos: NewDocumentoRepo(), Numerador: NewNumeradorRepo(),
		CuentasCobro: NewCuentaCobroRepo(), MetodosPago: NewMetodoPagoRepo(),
		CierresZ:     NewCierreZRepo(),
		Cotizaciones: NewCotizacionRepo(),
		Cajas:        NewCajaRepo(), Cajeros: NewCajeroRepo(), SesionesCaja: NewSesionCajaRepo(),
		Tasas: NewTasaRepo(), VentasEnEspera: NewVentaEnEsperaRepo(), Cobros: NewCobroRepo(),
		PagosProveedor:   NewPagoProveedorRepo(),
		CuentasContables: NewCuentaContableRepo(), Asientos: NewAsientoRepo(), Periodos: NewPeriodoRepo(),
		Proveedores: NewProveedorRepo(), OrdenesCompra: NewOrdenCompraRepo(),
		FacturasCompra: NewFacturaCompraRepo(), Solicitudes: NewSolicitudCompraRepo(),
		Retenciones:  NewRetencionRepo(),
		Dispositivos: NewDispositivoFiscalRepo(),
		ListasPrecio: NewListaPrecioRepo(),
		Cupones:      NewCuponRepo(),
		Promociones:  NewPromocionRepo(),
		Unidades:     NewUnidadMedidaRepo(),
		Almacenes:    NewAlmacenRepo(),
		Modulos:      NewModuloRepo(),
		DemoLeads:    NewDemoLeadRepo(),
		Legal:        NewLegalRepo(),
		Plantillas:   NewPlantillaRepo(),
		Mesas:        NewMesaRepo(),
		Planos:       NewPlanoRepo(),
		Impresoras:   NewImpresoraRepo(),
		Cuentas:      NewCuentaRepo(),
	}
	s.seedDemo()
	// Demos por RUBRO (restaurante, ferretería, farmacia): ver seed_nichos.go.
	s.seedNichos()
	return s
}

// hoyDemo y tasaDemo anclan los datos sembrados al día de hoy: así el gráfico
// del Inicio y los KPIs "de hoy"/"del mes" siempre tienen contenido, sin importar
// cuándo se levante la demo.
var hoyDemo = time.Now().UTC()

const tasaDemo = 745.63 // Bs/US$, la del prototipo

// publicidadDemo son anuncios de ejemplo para la pantalla del cliente del POS:
// una MEZCLA de slides de TEXTO (mensajes de marca) y uno de IMAGEN (data-URI SVG
// navy/teal, sin dependencia de red). Rotan solos en el carrusel para enseñar los
// dos tipos de slide y el modo `mixta` funcionando.
var publicidadDemo = []empresa.AnuncioSlide{
	{Tipo: empresa.SlideTexto, Texto: "2x1 en bebidas", Subtexto: "Solo esta semana — llévate dos y paga una"},
	{Tipo: empresa.SlideImagen, Imagen: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAxMjAwIDY3NSIgZm9udC1mYW1pbHk9IkludGVyLCBQb3BwaW5zLCBzYW5zLXNlcmlmIj4KPGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+CjxzdG9wIG9mZnNldD0iMCIgc3RvcC1jb2xvcj0iIzE1MkM2MSIvPjxzdG9wIG9mZnNldD0iMSIgc3RvcC1jb2xvcj0iIzA5MzQyQiIvPjwvbGluZWFyR3JhZGllbnQ+PC9kZWZzPgo8cmVjdCB3aWR0aD0iMTIwMCIgaGVpZ2h0PSI2NzUiIGZpbGw9InVybCgjZykiLz4KPGNpcmNsZSBjeD0iMTA0MCIgY3k9IjE1MCIgcj0iMjYwIiBmaWxsPSIjMDlCNjlCIiBvcGFjaXR5PSIwLjE2Ii8+CjxjaXJjbGUgY3g9IjEyMCIgY3k9IjYwMCIgcj0iMTgwIiBmaWxsPSIjMDlCNjlCIiBvcGFjaXR5PSIwLjEwIi8+CjxyZWN0IHg9IjkwIiB5PSIyNTAiIHdpZHRoPSI3MCIgaGVpZ2h0PSIxMCIgcng9IjUiIGZpbGw9IiMwOUI2OUIiLz4KPHRleHQgeD0iOTAiIHk9IjIxNSIgZmlsbD0iIzdGRTNENCIgZm9udC1zaXplPSIzNCIgZm9udC13ZWlnaHQ9IjYwMCIgbGV0dGVyLXNwYWNpbmc9IjMiPk5VRVZPIEVOIExBIENJTUE8L3RleHQ+Cjx0ZXh0IHg9Ijg4IiB5PSIzNjAiIGZpbGw9IiNGRkZGRkYiIGZvbnQtc2l6ZT0iMTE4IiBmb250LXdlaWdodD0iODAwIj5DYWZlIExhIENpbWE8L3RleHQ+Cjx0ZXh0IHg9IjkwIiB5PSI0MzAiIGZpbGw9IiNEN0UwRjIiIGZvbnQtc2l6ZT0iNDIiIGZvbnQtd2VpZ2h0PSI1MDAiPlRvc3RhZG8gYXJ0ZXNhbmFsLCByZWNpZW4gbGxlZ2FkbzwvdGV4dD4KPHJlY3QgeD0iOTAiIHk9IjQ4MCIgd2lkdGg9IjIxNCIgaGVpZ2h0PSI2NiIgcng9IjMzIiBmaWxsPSIjMDlCNjlCIi8+Cjx0ZXh0IHg9IjEyMCIgeT0iNTI0IiBmaWxsPSIjMDUyRTI3IiBmb250LXNpemU9IjM0IiBmb250LXdlaWdodD0iNzAwIj5Qcm9iYWxvPC90ZXh0Pgo8L3N2Zz4="},
	{Tipo: empresa.SlideTexto, Texto: "Delivery gratis", Subtexto: "En compras desde 20 US$"},
}

func (s *Store) seedDemo() {
	fecha := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)

	s.Organizaciones.Create(organizacion.Organizacion{ID: demoOrgID, Nombre: "Comercializadora La Cima", Estado: "activa", Plan: "base", Creada: fecha})
	s.Empresas.Create(empresa.Empresa{
		ID: demoEmpID, OrganizacionID: demoOrgID, Nombre: "Bodega La Cima, C.A.", RIF: "J-12345678-4",
		Giro: "bodega", Modalidad: empresa.ModalidadFormaLibre, Activa: true, OnboardingOK: true, Creada: fecha,
		// Contribuyente especial → agente de retención de IVA e ISLR: la demo muestra
		// la emisión de comprobantes de retención (número correlativo AAAAMM).
		AgenteRetencionIVA: true, AgenteRetencionISLR: true, RetencionIVAPorcentaje: 75,
		// Configuración de moneda (R10): la bodega piensa en bolívares —como
		// exige el SENIAT para sus libros— pero maneja algunos precios en
		// dólares, que es lo normal en el comercio venezolano.
		MonedaPrincipal: empresa.MonedaVES, FuenteTasa: empresa.FuenteTasaBCV, PreciosEnUsd: true,
		// Color de acento de marca (COMPAT del branding de dos niveles previo). El
		// modelo nuevo lo lleva en TemaPantalla.ColorEnfasis; se conserva por retrocompat.
		ColorMarca: "#09b69b",
		// Tema de la pantalla del cliente: se edita en Configuración › Marketing con
		// vista previa en vivo. La demo trae algo lindo —fondo navy, énfasis teal,
		// texto blanco, logo de color— para que la vista previa y la demo se vean bien.
		TemaPantalla: empresa.TemaPantallaCliente{
			Fondo:        "#152c61",
			ColorTexto:   "#ffffff",
			ColorEnfasis: "#09b69b",
			LogoVersion:  empresa.TemaLogoColor,
		},
		// Pantalla del cliente en modo MIXTA: carrito + publicidad al lado (y el
		// carrusel a pantalla grande sin venta), para enseñar la función completa.
		PantallaClienteModo: empresa.PantallaModoMixta,
		// Slides de ejemplo (mezcla de texto e imagen) para la pantalla del cliente.
		// Los de imagen son data-URIs SVG de marca que no dependen de la red: así la
		// demo enseña el carrusel funcionando sin cargar imágenes externas.
		Publicidad: publicidadDemo,
	})

	// Tasa inicial del modo demo. Se registra como SEMILLA, no como BCV: la
	// interfaz la rotula «Tasa de demostración» porque nadie la trajo del Banco
	// Central. En cuanto la sincronización corre, la sustituye la real (R9).
	s.Tasas.Append(tasa.Tasa{
		Valor: tasaDemo, Fuente: tasa.FuenteSemilla, FechaValor: hoyDemo.Format("2006-01-02"),
		ObtenidaEn: fecha, Actor: "seed", Detalle: "dato sembrado del modo demo",
		Estado: tasa.EstadoVigente,
	})
	s.Sedes.Create(sede.Sede{ID: demoSede1ID, EmpresaID: demoEmpID, Nombre: "Sede Principal", Direccion: "Av. Bolívar, Caracas", Activa: true})
	s.Sedes.Create(sede.Sede{ID: demoSede2ID, EmpresaID: demoEmpID, Nombre: "Sede Este", Direccion: "C.C. El Este, Caracas", Activa: true})

	// Usuario demo + membresía de Dueña (para "Entrar en modo demo").
	s.Usuarios.Create(usuario.Usuario{ID: application.DemoUserID, Nombre: application.DemoNombre, Email: application.DemoEmail})
	s.Membresias.Create(usuario.Membresia{
		UsuarioID: application.DemoUserID, Email: application.DemoEmail, Nombre: application.DemoNombre,
		EmpresaID: demoEmpID, Rol: usuario.RolDueno, Estado: usuario.EstadoActiva,
	})

	// Asistente IA instalado en la demo: enseña el asistente flotante respondiendo
	// MECÁNICAMENTE (local) sobre los datos sembrados. La capa IA queda apagada
	// (opt-in): en la demo no hay proxy de IA, así que solo corre la capa mecánica.
	s.Modulos.Upsert(aplicacion.Instalacion{
		EmpresaID: demoEmpID, ModuloID: aplicacion.ModAsistenteIA,
		Instalado: true, Activo: true, Actualizada: fecha,
	})

	// El módulo Restaurante NO va en la bodega: vive en la demo de restaurante
	// (emp_demo_rest), que sí tiene salón, mesas, platos con receta y comandas. Antes
	// se activaba acá como apaño, cuando no existía una demo del rubro — y dejaba a una
	// bodega con mapa de mesas, que no le corresponde.

	for _, n := range []string{"Víveres", "Bebidas", "Limpieza", "Charcutería", "Electrónica"} {
		s.Rubros.Create(inventario.Rubro{EmpresaID: demoEmpID, Nombre: n})
	}

	// Catálogo demo con existencias iniciales en la Sede Principal.
	type semilla struct {
		sku, nombre, rubro, unidad string
		precio, costo, cant        float64
		activo                     bool
		pres                       []inventario.Presentacion
		// moneda del PRECIO (R10). Vacío = la principal de la empresa (Bs).
		// El costo del ledger siempre va en bolívares: el Kardex es en Bs.
		moneda string
		// exento de IVA: la cesta básica venezolana lo está (harina, arroz).
		exento bool
		// código de barras propio (R11): lo que lee la pistola en el mostrador.
		codigo string
	}
	// El catálogo demo cubre a propósito toda la gama de casos que la interfaz
	// tiene que saber pintar:
	//   · las tres unidades base (unidad, kg, litro), incluida venta a granel;
	//   · con y sin presentaciones de venta (bulto, caja, six-pack);
	//   · los tres niveles del semáforo de stock: disponible, bajo y AGOTADO;
	//   · un producto inactivo (dado de baja pero con histórico);
	//   · nombres largos, para verificar truncados;
	//   · precios de 1 a 4 cifras, para verificar la alineación tabular.
	// costoPorSKU guarda el costo de cada producto para que los movimientos
	// sembrados (ventas, anulaciones) lleven su costo real: sin él el costo de
	// ventas del libro diario queda en cero y el margen sale 100%.
	costoPorSKU := map[string]float64{}
	catalogo := []semilla{
		// Los precios están en BOLÍVARES y son REALES DE MERCADO: cada uno es su
		// precio retail en US$ (harina 1,20 $, arroz 1,30 $, aceite 2,80 $…)
		// convertido a la tasa demo (× 745,63 Bs/US$) y redondeado a una cifra "de
		// bodega". Así el bolívar y su equivalente en dólares quedan creíbles en una
		// demostración: antes el seed traía cifras que a 745 Bs daban 6 céntimos de
		// dólar y no le decían nada a nadie.
		// Costo ≈ 70% del precio, que es el margen típico del abasto.

		// EXENTOS de IVA (cesta básica), con presentación y código de barras.
		{"HAR-001", "Harina de maíz 1kg", "Víveres", inventario.UnidadUnidad, 890.00, 625.00, 120, true,
			[]inventario.Presentacion{{ID: "pres_har24", Nombre: "Bulto x24", FactorConversion: 24, Precio: 19200.00, CodigoBarras: "7591234000241"}}, "", true, "7591234000301"},
		{"ARR-001", "Arroz blanco 1kg", "Víveres", inventario.UnidadUnidad, 970.00, 680.00, 85, true,
			[]inventario.Presentacion{{ID: "pres_arr25", Nombre: "Saco x25kg", FactorConversion: 25, Precio: 21800.00, CodigoBarras: "7591234000258"}}, "", true, "7591234000302"},
		// Gravados con IVA 16%.
		{"AZU-001", "Azúcar refinada 1kg", "Víveres", inventario.UnidadUnidad, 1040.00, 730.00, 90, true,
			[]inventario.Presentacion{{ID: "pres_azu12", Nombre: "Bulto x12", FactorConversion: 12, Precio: 11200.00, CodigoBarras: "7591234000122"}}, "", false, "7591234000303"},
		{"REF-2L", "Refresco 2L", "Bebidas", inventario.UnidadUnidad, 1490.00, 1045.00, 60, true,
			[]inventario.Presentacion{{ID: "pres_ref6", Nombre: "Six-pack", FactorConversion: 6, Precio: 8040.00, CodigoBarras: "7591234000061"}}, "", false, "7591234000304"},
		{"ACE-001", "Aceite de maíz 1L", "Víveres", inventario.UnidadLitro, 2090.00, 1465.00, 40, true, nil, "", false, "7591234000107"},
		{"PAS-001", "Pasta larga 500g", "Víveres", inventario.UnidadUnidad, 820.00, 575.00, 75, true, nil, "", false, "7591234000114"},
		{"JAB-001", "Jabón azul de panela", "Limpieza", inventario.UnidadUnidad, 595.00, 415.00, 200, true, nil, "", false, "7591234000121"},
		{"DET-001", "Detergente en polvo 1kg", "Limpieza", inventario.UnidadUnidad, 2240.00, 1570.00, 19, true, nil, "", false, "7591234000128"},
		{"AGU-15", "Agua mineral 1,5L", "Bebidas", inventario.UnidadUnidad, 595.00, 415.00, 120, true, nil, "", false, "7591234000135"},
		// Venta POR PESO (charcutería/granel): el precio es Bs/kg y la existencia va
		// en kg con decimales; se cobran fracciones leídas de la balanza (0,25 kg…).
		// Precios reales de mercado (× tasa demo): queso blanco ≈ 5,5 $/kg, jamón ≈
		// 9 $/kg, queso amarillo ≈ 7 $/kg, carne de res ≈ 6 $/kg, tomate ≈ 1,5 $/kg.
		{"QUE-KG", "Queso blanco duro (granel)", "Charcutería", inventario.UnidadKg, 4100.00, 2870.00, 22.5, true, nil, "", false, "7591234000142"},
		{"JAM-KG", "Jamón de pierna ahumado (granel)", "Charcutería", inventario.UnidadKg, 6710.00, 4700.00, 14.75, true, nil, "", false, "7591234000149"},
		{"QAM-KG", "Queso amarillo (granel)", "Charcutería", inventario.UnidadKg, 5220.00, 3655.00, 18.25, true, nil, "", false, "7591234000219"},
		{"CAR-KG", "Carne de res, muslo (granel)", "Charcutería", inventario.UnidadKg, 4470.00, 3130.00, 12.75, true, nil, "", false, "7591234000226"},
		{"TOM-KG", "Tomate perita (granel)", "Víveres", inventario.UnidadKg, 1120.00, 785.00, 30.5, true, nil, "", false, "7591234000233"},
		// Granel por volumen.
		{"DET-LT", "Detergente líquido (granel, rellenar)", "Limpieza", inventario.UnidadLitro, 1120.00, 785.00, 30, true, nil, "", false, "7591234000156"},
		// STOCK BAJO (≤ 5): dispara la alerta del Inicio y el semáforo ámbar.
		{"CAF-500", "Café molido premium 500g", "Víveres", inventario.UnidadUnidad, 3350.00, 2345.00, 4, true, nil, "", false, "7591234000163"},
		{"ATU-001", "Atún en aceite 170g", "Víveres", inventario.UnidadUnidad, 1190.00, 835.00, 2, true, nil, "", false, "7591234000170"},
		// AGOTADO: no se puede vender; semáforo rojo.
		{"LEC-001", "Leche en polvo 900g", "Víveres", inventario.UnidadUnidad, 4850.00, 3395.00, 0, true, nil, "", false, "7591234000177"},
		// Precio de 5 cifras, para verificar la alineación de cifras grandes.
		{"CAL-001", "Zapatos deportivos unisex", "Electrónica", inventario.UnidadUnidad, 22370.00, 15660.00, 12, true, nil, "", false, "7591234000184"},
		{"ELE-PB", "Power bank 10.000 mAh carga rápida", "Electrónica", inventario.UnidadUnidad, 11180.00, 7830.00, 8, true, nil, "", false, "7591234000191"},
		// Nombre deliberadamente largo: verifica truncados en tabla y tarjeta.
		{"PAN-001", "Pan campesino artesanal de masa madre horneado del día", "Víveres", inventario.UnidadUnidad, 1860.00, 1300.00, 18, true, nil, "", false, "7591234000198"},
		// INACTIVO: dado de baja, con histórico en el Kardex.
		{"GAL-OLD", "Galletas surtidas (descontinuado)", "Víveres", inventario.UnidadUnidad, 1120.00, 785.00, 6, false, nil, "", false, "7591234000205"},
		// PRECIO EN DÓLARES (R10): lo normal en electrodomésticos en Venezuela.
		// Su Bs se calcula con la tasa del día, nunca se guarda; el costo del
		// ledger sí está en Bs (≈ 110 US$ × 745,63), porque el Kardex se lleva en
		// bolívares.
		{"ELE-TV", "Televisor LED 32 pulgadas", "Electrónica", inventario.UnidadUnidad, 150.00, 82020.00, 5, true, nil, empresa.MonedaUSD, false, "7591234000212"},
	}
	// Productos que se venden POR PESO (balanza): precio Bs/kg, existencia en kg.
	// La forma de venta fija su unidad base en "kg" (misma regla que el servicio).
	pesoSKUs := map[string]bool{"QUE-KG": true, "JAM-KG": true, "QAM-KG": true, "CAR-KG": true, "TOM-KG": true}
	for _, c := range catalogo {
		costoPorSKU[c.sku] = c.costo
		pres := c.pres
		if pres == nil {
			pres = []inventario.Presentacion{}
		}
		tipoVenta, unidad := inventario.TipoVentaUnidad, c.unidad
		if pesoSKUs[c.sku] {
			tipoVenta, unidad = inventario.TipoVentaPeso, inventario.UnidadKg
		}
		p := s.Productos.Create(inventario.Producto{
			EmpresaID: demoEmpID, SKU: c.sku, Nombre: c.nombre, Rubro: c.rubro,
			UnidadBase: unidad, TipoVenta: tipoVenta, Precio: c.precio, Moneda: c.moneda, ExentoIVA: c.exento,
			CodigoBarras: c.codigo,
			Activo:       c.activo, Presentaciones: pres,
		})
		// Movimiento de entrada inicial (compra) al ledger. Los agotados no
		// reciben entrada: su existencia es 0 porque nunca entró stock.
		if c.cant <= 0 {
			continue
		}
		s.Movimientos.Append(inventario.Movimiento{
			EmpresaID: demoEmpID, SedeID: demoSede1ID, ProductoID: p.ID, SKU: p.SKU,
			Tipo: inventario.MovEntrada, Cantidad: c.cant, CostoUnitario: c.costo,
			Motivo: "inventario inicial", Actor: application.DemoUserID, Fecha: fecha,
		})
	}
	// Una venta (salida) para que el Kardex de la harina muestre movimiento.
	if p, ok := s.Productos.BySKU(demoEmpID, "HAR-001"); ok {
		s.Movimientos.Append(inventario.Movimiento{
			EmpresaID: demoEmpID, SedeID: demoSede1ID, ProductoID: p.ID, SKU: p.SKU,
			Tipo: inventario.MovSalida, Cantidad: -15, Motivo: "venta mostrador",
			Actor: application.DemoUserID, Fecha: time.Date(2026, 7, 3, 9, 30, 0, 0, time.UTC).Format(time.RFC3339Nano),
		})
	}

	// Stock también en la SEGUNDA SEDE, con cantidades distintas: es lo que hace
	// significativo el modal de disponibilidad por ubicación (un producto que
	// aquí está agotado y allá sí hay, y viceversa).
	for _, e := range []struct {
		sku  string
		cant float64
	}{
		{"HAR-001", 45}, {"AZU-001", 3}, {"REF-2L", 80},
		{"LEC-001", 22}, // agotado en la Principal, disponible en el Este
		{"CAF-500", 0},  // bajo en la Principal, agotado en el Este
		{"QUE-KG", 9}, {"ELE-PB", 2},
	} {
		if e.cant <= 0 {
			continue
		}
		if p, ok := s.Productos.BySKU(demoEmpID, e.sku); ok {
			s.Movimientos.Append(inventario.Movimiento{
				EmpresaID: demoEmpID, SedeID: demoSede2ID, ProductoID: p.ID, SKU: p.SKU,
				Tipo: inventario.MovEntrada, Cantidad: e.cant, CostoUnitario: p.Precio * 0.7,
				Motivo: "inventario inicial", Actor: application.DemoUserID, Fecha: fecha,
			})
		}
	}

	// Un ajuste auditado con motivo, para que el Kardex muestre los tres tipos
	// de movimiento (entrada, salida y ajuste).
	if p, ok := s.Productos.BySKU(demoEmpID, "JAB-001"); ok {
		s.Movimientos.Append(inventario.Movimiento{
			EmpresaID: demoEmpID, SedeID: demoSede1ID, ProductoID: p.ID, SKU: p.SKU,
			Tipo: inventario.MovAjuste, Cantidad: -4, Motivo: "merma por rotura en almacén",
			Actor: application.DemoUserID, Fecha: time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		})
	}

	// Producto COMBO: un paquete vendido como grupo de otros productos que YA
	// están en el catálogo (Harina + Café + Leche = "Combo Desayuno"). No se
	// stockea (no tiene existencia propia ni movimientos en el ledger) y su precio
	// es su propio precio de paquete, MENOR que la suma de sus componentes para que
	// haya "ahorro": 890 + 3350 + 4850 = 9090 → 8500 (ahorro de 590 Bs). Al
	// facturar no entra como línea: el frontend lo EXPLOTA en las líneas de sus
	// componentes (precio prorrateado e IVA por ítem).
	s.Productos.Create(inventario.Producto{
		EmpresaID: demoEmpID, SKU: "COMBO-DESAYUNO", Nombre: "Combo Desayuno",
		Rubro: "Víveres", UnidadBase: inventario.UnidadUnidad, TipoVenta: inventario.TipoVentaUnidad,
		Precio: 8500.00, Activo: true, EsCombo: true,
		Componentes: []inventario.ComboComponente{
			{SKU: "HAR-001", Cantidad: 1},
			{SKU: "CAF-500", Cantidad: 1},
			{SKU: "LEC-001", Cantidad: 1},
		},
		Presentaciones: []inventario.Presentacion{},
	})

	// Clientes que cubren los CUATRO tipos de documento (V, E, J, G) más el caso
	// sin teléfono y el de nombre largo.
	for _, c := range []cliente.Cliente{
		{Nombre: "Inversiones El Molino, C.A.", TipoDocumento: cliente.DocJ, Documento: "40123456-9", Telefono: "0212-5551234", NombreComercial: "El Molino", Contacto: "Ana Torres"},
		{Nombre: "María Alejandra Rodríguez Pérez", TipoDocumento: cliente.DocV, Documento: "12345678", Telefono: "0414-1234567", Email: "maria.rodriguez@correo.com"},
		{Nombre: "Giuseppe Antonio Barbieri", TipoDocumento: cliente.DocE, Documento: "84512399", Telefono: ""},
		{Nombre: "Alcaldía del Municipio Chacao", TipoDocumento: cliente.DocG, Documento: "20000123-2", Telefono: "0212-2088111"},
		// Ejemplo de cliente IMPORTADO desde otro CRM/ERP: conserva su procedencia
		// (Odoo + id externo) para una futura sincronización bidireccional.
		{Nombre: "Distribuidora de Alimentos y Bebidas del Centro Occidente, C.A.", TipoDocumento: cliente.DocJ, Documento: "31122334-7", Telefono: "0251-2334455", Origen: cliente.OrigenOdoo, SistemaExterno: "odoo", IdExterno: "res.partner:1042"},
		{Nombre: "Pedro Luis Salas", TipoDocumento: cliente.DocV, Documento: "9876543", Telefono: "0424-9998877"},
	} {
		c.EmpresaID = demoEmpID
		c.Activo = true
		if c.Origen == "" {
			c.Origen = cliente.OrigenManual
		}
		s.Clientes.Create(c)
	}

	// Cuentas de cobro de TODOS los tipos y ambas monedas: son los destinos que
	// ofrece el cobro mixto, y el IGTF depende de que haya cuentas en divisas.
	// cuentaPorTipo captura el id generado de cada cuenta para enlazar los métodos
	// de pago a su destino (los ids son dinámicos con prefijo cta_).
	cuentaPorTipo := map[string]string{}
	for _, cc := range []fiscal.CuentaCobro{
		{Tipo: "pago_movil", Moneda: "VES", Titular: "Bodega La Cima", Datos: "0102-04-12345678"},
		{Tipo: "banco", Moneda: "VES", Titular: "Bodega La Cima, C.A.", Datos: "Banesco 0134-0123-45-6789012345"},
		{Tipo: "punto_venta", Moneda: "VES", Titular: "Punto Sede Principal", Datos: "Terminal 8891-2210"},
		{Tipo: "efectivo", Moneda: "VES", Titular: "Caja chica Bs", Datos: "Efectivo en bolívares"},
		{Tipo: "zelle", Moneda: "USD", Titular: "La Cima LLC", Datos: "pagos@bodegalacima.com"},
		{Tipo: "efectivo", Moneda: "USD", Titular: "Caja chica US$", Datos: "Efectivo en divisas"},
	} {
		cc.EmpresaID = demoEmpID
		out := s.CuentasCobro.Create(cc)
		if _, visto := cuentaPorTipo[out.Tipo]; !visto {
			cuentaPorTipo[out.Tipo] = out.ID
		}
	}

	// Métodos de pago configurables (Orden 1..6): lo que la empresa ofrece en el
	// mostrador y/o en Ventas. Pago Móvil y Zelle enlazan a su cuenta de cobro.
	for _, mp := range []fiscal.MetodoPago{
		{Nombre: "Efectivo Bs", Tipo: fiscal.PagoEfectivoBs, Moneda: "VES", EnCaja: true, EnVentas: true, Orden: 1},
		{Nombre: "Efectivo USD", Tipo: fiscal.PagoEfectivoUSD, Moneda: "USD", EnCaja: true, EnVentas: true, Orden: 2},
		{Nombre: "Pago Móvil", Tipo: fiscal.PagoPagoMovil, Moneda: "VES", CuentaCobroID: cuentaPorTipo["pago_movil"], EnCaja: true, EnVentas: true, Orden: 3},
		{Nombre: "Zelle", Tipo: fiscal.PagoZelle, Moneda: "USD", CuentaCobroID: cuentaPorTipo["zelle"], EnCaja: false, EnVentas: true, Orden: 4},
		{Nombre: "Tarjeta", Tipo: fiscal.PagoTarjeta, Moneda: "VES", EnCaja: true, EnVentas: false, Orden: 5},
		{Nombre: "Transferencia", Tipo: fiscal.PagoTransfer, Moneda: "VES", EnCaja: false, EnVentas: true, Orden: 6},
	} {
		mp.EmpresaID = demoEmpID
		mp.Activo = true
		s.MetodosPago.Create(mp)
	}

	// Dispositivo fiscal demo: una impresora fiscal homologada en la sede 1. Su
	// conexión real la maneja el agente fiscal local; aquí solo queda registrada.
	s.Dispositivos.Create(fiscal.DispositivoFiscal{
		EmpresaID: demoEmpID, SedeID: demoSede1ID, Nombre: "Impresora fiscal caja 1",
		Tipo: fiscal.DispositivoImpresoraFiscal, Marca: "The Factory HKA", Modelo: "PP-9",
		Serie: "Z1B0000123", Activo: true, Creado: "2026-01-05T12:00:00Z",
	})
	// Balanza de charcutería: pesa el queso/jamón/carne de granel y el POS
	// multiplica los kg por el precio/kg. Su conexión (puerto serie + protocolo)
	// la maneja el agente fiscal local; aquí solo queda su ficha.
	s.Dispositivos.Create(fiscal.DispositivoFiscal{
		EmpresaID: demoEmpID, SedeID: demoSede1ID, Nombre: "Balanza charcutería",
		Tipo: fiscal.DispositivoBalanza, Marca: "Aclas", Modelo: "OS2X",
		Puerto: "/dev/ttyUSB0", Protocolo: "dialog06", Activo: true, Creado: "2026-01-05T12:00:00Z",
	})

	// Transferencias en CADA estado de la máquina, para poder ver la ficha de
	// avance en todas sus fases sin tener que crearlas a mano.
	linea := func(sku string, cant float64) []inventario.LineaTransferencia {
		p, ok := s.Productos.BySKU(demoEmpID, sku)
		if !ok {
			return nil
		}
		return []inventario.LineaTransferencia{{ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre, Cantidad: cant}}
	}
	for i, tr := range []struct {
		estado string
		sku    string
		cant   float64
		dia    int
	}{
		{inventario.TransfBorrador, "HAR-001", 10, 20},
		{inventario.TransfDespachada, "AZU-001", 6, 22},
		{inventario.TransfEnTransito, "REF-2L", 24, 24},
		{inventario.TransfRecibida, "JAB-001", 30, 26},
		{inventario.TransfCerrada, "QUE-KG", 5, 28},
	} {
		ls := linea(tr.sku, tr.cant)
		if ls == nil {
			continue
		}
		f := time.Date(2026, 7, tr.dia, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
		origen, destino := demoSede1ID, demoSede2ID
		if i%2 == 1 {
			// Alterna el sentido: también hay transferencias que llegan.
			origen, destino = demoSede2ID, demoSede1ID
		}
		s.Transferencias.Create(inventario.Transferencia{
			EmpresaID: demoEmpID, OrigenSedeID: origen, DestinoSedeID: destino,
			Estado: tr.estado, Lineas: ls, Creada: f, Actualizada: f,
		})
	}

	// --- Documentos fiscales ---
	// Se siembran CON sus movimientos de salida, para que el ledger de inventario
	// y los documentos cuenten la misma historia: un tester que abra el Kardex
	// después de ver una factura tiene que encontrar su salida.
	//
	// Cubren toda la gama: facturas normales repartidas en los últimos 30 días
	// (para que el gráfico del Inicio tenga forma real), una en CONTINGENCIA
	// (serie C, emitida sin conexión), una ANULADA con su documento de reversa, y
	// una NOTA DE CRÉDITO parcial.
	// La numeración va por el MISMO Numerador que usa la emisión viva, así el
	// contador queda ADELANTADO más allá del máximo sembrado y la primera factura
	// real no reinicia en 1 ni colisiona con los folios de la demo.
	// plazoDias > 0 emite la factura A CRÉDITO: el abono se cobra en el mostrador y
	// el resto queda por cobrar en Tesorería, con su fecha de vencimiento.
	emitir := func(diasAtras int, tipo, serie string, clienteNombre, clienteDoc string,
		sku string, cant float64, contingencia bool, enDivisas bool, ref, motivo string,
		plazoDias int, abonoBs float64) fiscal.Documento {
		prod, ok := s.Productos.BySKU(demoEmpID, sku)
		if !ok {
			return fiscal.Documento{}
		}
		num := s.Numerador.Siguiente(demoEmpID, demoSede1ID, serie)
		f := hoyDemo.AddDate(0, 0, -diasAtras)
		// El precio sale del catálogo: así los documentos sembrados y el catálogo
		// nunca cuentan precios distintos del mismo producto.
		precio := prod.Precio
		base := cant * precio
		// El IVA respeta la exención del producto (harina y arroz son exentos).
		gravada, exenta := base, 0.0
		if prod.ExentoIVA {
			gravada, exenta = 0, base
		}
		iva := gravada * fiscal.AlicuotaIVA
		total := base + iva
		igtf := 0.0
		pagos := []fiscal.Pago{{Metodo: fiscal.PagoEfectivoBs, Monto: total, Moneda: "VES"}}
		if plazoDias > 0 {
			// A crédito: solo entra el abono (puede ser cero).
			pagos = []fiscal.Pago{}
			if abonoBs > 0 {
				pagos = []fiscal.Pago{{Metodo: fiscal.PagoPagoMovil, Monto: abonoBs, Moneda: "VES"}}
			}
		} else if enDivisas {
			// La mitad en divisas: genera IGTF sobre esa porción, cobrado en Bs.
			mitad := total / 2
			igtf = mitad * fiscal.AlicuotaIGTF
			total += igtf
			pagos = []fiscal.Pago{
				{Metodo: fiscal.PagoEfectivoBs, Monto: mitad + igtf, Moneda: "VES"},
				{Metodo: fiscal.PagoEfectivoUSD, Monto: mitad / tasaDemo, Moneda: "USD", EnDivisa: true},
			}
		}
		doc := s.Documentos.Append(fiscal.Documento{
			EmpresaID: demoEmpID, SedeID: demoSede1ID, Tipo: tipo,
			Serie: serie, Numero: num, NumeroCompleto: fmt.Sprintf("%s-%08d", serie, num),
			NumeroControl: fmt.Sprintf("00-%08d", s.Numerador.Siguiente(demoEmpID, "", "CTRL")),
			Modalidad:     empresa.ModalidadFormaLibre, Contingencia: contingencia,
			ClienteNombre: clienteNombre, ClienteDocumento: clienteDoc,
			Lineas: []fiscal.Linea{{ProductoID: prod.ID, SKU: prod.SKU, Nombre: prod.Nombre,
				Cantidad: cant, PrecioUnitario: precio, Total: base, Exento: prod.ExentoIVA}},
			Subtotal: base, BaseImponible: gravada, BaseExenta: exenta,
			IVA: iva, IGTF: igtf, Total: total,
			// Contado: cobro exacto, sin vuelto. A crédito: solo el abono.
			Cobrado: map[bool]float64{true: abonoBs, false: total}[plazoDias > 0],
			Credito: plazoDias > 0,
			VenceEl: map[bool]string{true: f.AddDate(0, 0, plazoDias).Format("2006-01-02"), false: ""}[plazoDias > 0],
			Moneda:  "VES", TasaCambio: tasaDemo, TasaFuente: tasa.FuenteSemilla, Pagos: pagos,
			RefDocumentoID: ref, Motivo: motivo,
			CajaID: "caja_demo_1", CajaCodigo: "C-001", CajeroNombre: "Luis Marcano",
			Actor: application.DemoUserID, Fecha: f.Format(time.RFC3339Nano),
		})
		// Pata de inventario: la factura descuenta, la reversa repone.
		signo := -1.0
		if tipo != fiscal.TipoFactura {
			signo = 1.0
		}
		s.Movimientos.Append(inventario.Movimiento{
			EmpresaID: demoEmpID, SedeID: demoSede1ID, ProductoID: prod.ID, SKU: prod.SKU,
			Tipo:     map[bool]string{true: inventario.MovSalida, false: inventario.MovEntrada}[signo < 0],
			Cantidad: signo * cant, CostoUnitario: costoPorSKU[prod.SKU],
			Motivo:  "venta " + doc.NumeroCompleto,
			RefTipo: "documento", RefID: doc.ID,
			Actor: application.DemoUserID, Fecha: f.Format(time.RFC3339Nano),
		})
		return doc
	}

	// Facturas repartidas en el mes: distintos clientes, montos y monedas.
	emitir(27, fiscal.TipoFactura, "FL", "Consumidor final", "", "HAR-001", 6, false, false, "", "", 0, 0)
	emitir(24, fiscal.TipoFactura, "FL", "Inversiones El Molino, C.A.", "J-40123456-9", "AZU-001", 12, false, false, "", "", 0, 0)
	emitir(21, fiscal.TipoFactura, "FL", "María Alejandra Rodríguez Pérez", "V-12345678", "REF-2L", 4, false, true, "", "", 0, 0)
	emitir(17, fiscal.TipoFactura, "FL", "Consumidor final", "", "JAB-001", 10, false, false, "", "", 0, 0)
	emitir(14, fiscal.TipoFactura, "FL", "Alcaldía del Municipio Chacao", "G-20000123-2", "HAR-001", 24, false, false, "", "", 0, 0)
	emitir(10, fiscal.TipoFactura, "FL", "Giuseppe Antonio Barbieri", "E-84512399", "QUE-KG", 1.5, false, true, "", "", 0, 0)
	emitir(7, fiscal.TipoFactura, "FL", "Consumidor final", "", "REF-2L", 8, false, false, "", "", 0, 0)
	emitir(4, fiscal.TipoFactura, "FL", "Pedro Luis Salas", "V-9876543", "JAB-001", 5, false, false, "", "", 0, 0)
	emitir(2, fiscal.TipoFactura, "FL", "Consumidor final", "", "AZU-001", 3, false, false, "", "", 0, 0)

	// Emitida SIN CONEXIÓN: serie C reservada, pendiente de sincronizar.
	emitir(3, fiscal.TipoFactura, "C", "Consumidor final", "", "HAR-001", 2, true, false, "", "", 0, 0)

	// ANULADA: la factura se queda como está y su reversa la referencia. El
	// estado "anulado" lo DERIVA el backend de que exista esta reversa.
	anulable := emitir(9, fiscal.TipoFactura, "FL", "Distribuidora de Alimentos y Bebidas del Centro Occidente, C.A.", "J-31122334-7", "HAR-001", 20, false, false, "", "", 0, 0)
	if anulable.ID != "" {
		emitir(8, fiscal.TipoAnulacion, "FL", anulable.ClienteNombre, anulable.ClienteDocumento,
			"HAR-001", 20, false, false, anulable.ID, "cliente devolvió el pedido completo", 0, 0)
	}

	// NOTA DE CRÉDITO parcial sobre otra factura.
	parcial := emitir(6, fiscal.TipoFactura, "FL", "Inversiones El Molino, C.A.", "J-40123456-9", "REF-2L", 12, false, false, "", "", 0, 0)
	if parcial.ID != "" {
		emitir(5, fiscal.TipoNotaCredito, "FL", parcial.ClienteNombre, parcial.ClienteDocumento,
			"REF-2L", 3, false, false, parcial.ID, "3 unidades llegaron dañadas", 0, 0)
	}

	// --- Ventas A CRÉDITO y sus cobros (Tesorería) ---
	// La demo necesita cuentas por cobrar reales para que «Por cobrar vencido» y
	// «Efectivo y bancos» dejen de ser KPIs sin módulo detrás. Se siembran dos:
	// una VENCIDA (plazo de 15 días hace 40) con un abono parcial, y una vigente.
	// Vencida hace 25 días (plazo de 15, emitida hace 40) con un abono parcial: es
	// la que hace visible el KPI «Por cobrar vencido» en rojo.
	emitir(40, fiscal.TipoFactura, "FL", "Distribuidora de Alimentos y Bebidas del Centro Occidente, C.A.",
		"J-31122334-7", "ARR-001", 60, false, false, "", "", 15, 18000)
	// Vigente, a 30 días y sin abono todavía.
	vigenteCred := emitir(6, fiscal.TipoFactura, "FL", "Inversiones El Molino, C.A.",
		"J-40123456-9", "ACE-001", 24, false, false, "", "", 30, 0)

	// --- Retención de ISLR recibida (Fiscal ⇄ Tesorería ⇄ Contabilidad) ---
	// Un cliente agente de retención (Inversiones El Molino) retuvo ISLR al pagar
	// una venta a crédito: baja la CxC de esa factura y reconoce el activo
	// «Retenciones de ISLR a favor / anticipo» (1105). Es el caso que hace visible
	// en la pantalla de Retenciones un comprobante de ISLR además de los de IVA.
	// Su asiento lo deriva RecontabilizarPendientes al arrancar (idempotente).
	if vigenteCred.ID != "" && vigenteCred.BaseImponible > 0 {
		baseISLR := round2Demo(vigenteCred.BaseImponible)
		pctISLR := 2.0 // 2% por compra de bienes muebles (sin sustraendo)
		s.Retenciones.Append(fiscal.Retencion{
			EmpresaID: demoEmpID, Tipo: fiscal.RetencionRecibida, Impuesto: fiscal.ImpuestoISLR,
			DocumentoID: vigenteCred.ID, DocumentoNumero: vigenteCred.NumeroCompleto,
			NumeroComprobante: "20260800004321",
			Fecha:             hoyDemo.AddDate(0, 0, -3).Format("2006-01-02"),
			TerceroNombre:     vigenteCred.ClienteNombre, TerceroRIF: vigenteCred.ClienteDocumento,
			Base: baseISLR, Concepto: "Compra de bienes muebles", Sustraendo: 0,
			Porcentaje: pctISLR, MontoRetenido: round2Demo(baseISLR * pctISLR / 100),
			Actor: application.DemoUserID, Registrada: hoyDemo.AddDate(0, 0, -3).Format(time.RFC3339Nano),
		})
	}

	// --- Cotizaciones (venta forma libre) ---
	// Dos cotizaciones demo para el módulo Ventas: una en BORRADOR (recién armada,
	// aún negociable) y una CONFIRMADA (pedido/prefactura, lista para facturar).
	// Los precios salen del catálogo y el cliente es uno de los sembrados, para que
	// el flujo cotización → confirmar → facturar tenga con qué demostrarse.
	sembrarCot := func(estado string, clienteNombre, clienteDoc string, diasAtras int,
		validez, condicionesPago, terminos string, items []struct {
			sku    string
			cant   float64
			desc   string
			descto float64
		}) {
		lineas := []cotizacion.Linea{}
		var subtotal, baseImponible float64
		for _, it := range items {
			p, ok := s.Productos.BySKU(demoEmpID, it.sku)
			if !ok {
				continue
			}
			total := round2Demo(p.Precio * it.cant * (1 - it.descto/100))
			lineas = append(lineas, cotizacion.Linea{
				ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre, Descripcion: it.desc,
				Cantidad: it.cant, PrecioUnitario: p.Precio, Descuento: it.descto, Total: total, Exento: p.ExentoIVA,
			})
			subtotal += total
			if !p.ExentoIVA {
				baseImponible += total
			}
		}
		if len(lineas) == 0 {
			return
		}
		iva := round2Demo(round2Demo(baseImponible) * fiscal.AlicuotaIVA)
		subtotal = round2Demo(subtotal)
		nCot := s.Numerador.Siguiente(demoEmpID, demoSede1ID, "COT")
		f := hoyDemo.AddDate(0, 0, -diasAtras)
		s.Cotizaciones.Create(cotizacion.Cotizacion{
			EmpresaID: demoEmpID, SedeID: demoSede1ID,
			Numero: nCot, NumeroCompleto: fmt.Sprintf("COT-%06d", nCot), Estado: estado,
			ClienteNombre: clienteNombre, ClienteDocumento: clienteDoc,
			Lineas: lineas, Subtotal: subtotal, IVA: iva, IGTF: 0, Total: round2Demo(subtotal + iva),
			Moneda: empresa.MonedaVES, TasaCambio: tasaDemo, Validez: validez,
			CondicionesPago: condicionesPago, Terminos: terminos,
			Actor: application.DemoUserID, Fecha: f.Format(time.RFC3339Nano), Actualizada: f.Format(time.RFC3339Nano),
		})
	}
	sembrarCot(cotizacion.EstadoBorrador, "María Alejandra Rodríguez Pérez", "V-12345678", 2,
		"15 días", "Contado", "Precios sujetos a cambio según la tasa del BCV del día de la facturación.",
		[]struct {
			sku    string
			cant   float64
			desc   string
			descto float64
		}{
			{"REF-2L", 6, "Presentación retornable de 2 litros", 10},
			{"HAR-001", 12, "", 0},
			{"JAB-001", 4, "Combo de limpieza", 5},
		})
	sembrarCot(cotizacion.EstadoConfirmada, "Inversiones El Molino, C.A.", "J-40123456-9", 4,
		"30 días", "30 días", "",
		[]struct {
			sku    string
			cant   float64
			desc   string
			descto float64
		}{{"AZU-001", 24, "", 0}, {"ACE-001", 6, "", 0}})

	// --- Proveedores y órdenes de compra (Compras) ---
	// Proveedores demo para que el módulo Compras no arranque vacío: los tres
	// tipos de tercero (RIF jurídico) que surten a la bodega, uno de ellos ya
	// dado de baja (Activo=false) para poder ver la desactivación reversible.
	provsDemo := map[string]proveedor.Proveedor{}
	for _, pv := range []proveedor.Proveedor{
		{ID: "prov_demo_1", Nombre: "Distribuidora Polar, C.A.", Documento: "J-00012345-4", Email: "ventas@polar.test", Telefono: "0212-2020100", Direccion: "Zona Industrial, Caracas", Activo: true},
		{ID: "prov_demo_2", Nombre: "Alimentos Mary, C.A.", Documento: "J-30099887-8", Email: "pedidos@mary.test", Telefono: "0243-2334455", Direccion: "Maracay, Aragua", Activo: true},
		{ID: "prov_demo_3", Nombre: "Insumos del Centro (descontinuado)", Documento: "J-31200011-2", Email: "", Telefono: "0251-7778899", Direccion: "Barquisimeto, Lara", Activo: false},
	} {
		pv.EmpresaID = demoEmpID
		pv.Creado = fecha
		out := s.Proveedores.Create(pv)
		provsDemo[out.ID] = out
	}

	// Una orden de compra CONFIRMADA (lista para recibir) al proveedor principal,
	// con costos NETOS de dos productos del catálogo. Sus folios usan el mismo
	// Numerador (serie "OC"), así la primera orden viva no reinicia en 1.
	sembrarOC := func(estado, provID string, diasAtras int, condiciones string, recibido bool, items []struct {
		sku   string
		cant  float64
		costo float64
	}) {
		prov, ok := provsDemo[provID]
		if !ok {
			return
		}
		lineas := []compra.Linea{}
		var subtotal, baseImponible float64
		for _, it := range items {
			p, ok := s.Productos.BySKU(demoEmpID, it.sku)
			if !ok {
				continue
			}
			total := round2Demo(it.costo * it.cant)
			exento := p.ExentoIVA
			// Una orden recibida trae la mercancía completa (alimenta Cuentas por pagar).
			recibida := 0.0
			if recibido {
				recibida = it.cant
			}
			lineas = append(lineas, compra.Linea{
				ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre,
				Cantidad: it.cant, CostoUnitario: it.costo, CantidadRecibida: recibida,
				Total: total, Exento: exento,
			})
			subtotal += total
			if !exento {
				baseImponible += total
			}
		}
		if len(lineas) == 0 {
			return
		}
		iva := round2Demo(round2Demo(baseImponible) * fiscal.AlicuotaIVA)
		subtotal = round2Demo(subtotal)
		nOC := s.Numerador.Siguiente(demoEmpID, demoSede1ID, "OC")
		f := hoyDemo.AddDate(0, 0, -diasAtras).Format(time.RFC3339Nano)
		s.OrdenesCompra.Create(compra.OrdenCompra{
			EmpresaID: demoEmpID, SedeID: demoSede1ID,
			ProveedorID: prov.ID, ProveedorNombre: prov.Nombre,
			Numero: nOC, NumeroCompleto: fmt.Sprintf("OC-%06d", nOC), Serie: "OC", Estado: estado,
			Lineas: lineas, Subtotal: subtotal, IVA: iva, Total: round2Demo(subtotal + iva),
			Moneda: empresa.MonedaVES, TasaCambio: tasaDemo, TasaFuente: tasa.FuenteSemilla,
			CondicionesPago: condiciones,
			Actor:           application.DemoUserID, Creada: f, Actualizada: f,
		})
	}
	sembrarOC(compra.OCConfirmada, "prov_demo_1", 3, "30 días", false, []struct {
		sku   string
		cant  float64
		costo float64
	}{{"REF-2L", 48, 1045.00}, {"AGU-15", 60, 415.00}})
	// Orden RECIBIDA: la mercancía entró; genera Cuentas por pagar (Tesorería › CxP).
	sembrarOC(compra.OCRecibida, "prov_demo_2", 9, "contado", true, []struct {
		sku   string
		cant  float64
		costo float64
	}{{"HAR-001", 100, 610.00}, {"ACE-1L", 80, 1180.00}})

	// --- Solicitudes de presupuesto (RFQ) ---
	// El paso PREVIO a la orden de compra: se pide presupuesto a varios proveedores,
	// se comparan y se convierte a orden. La demo trae dos: una ENVIADA a dos
	// proveedores con ambas respuestas cargadas (para ver la tabla comparativa y el
	// «convertir en orden»), y una en BORRADOR (recién armada, aún editable).
	sembrarSolicitud := func(estado, notas string, diasAtras int,
		lineas []struct {
			sku  string
			cant float64
		},
		// respuestas: por proveedor, un mapa SKU→precio (vacío = pendiente).
		proveedores []struct {
			id        string
			respuesta map[string]float64
		}) {
		ls := []compra.LineaSolicitud{}
		cantPorSKU := map[string]float64{}
		for _, it := range lineas {
			p, ok := s.Productos.BySKU(demoEmpID, it.sku)
			if !ok {
				continue
			}
			ls = append(ls, compra.LineaSolicitud{ProductoID: p.ID, SKU: p.SKU, Nombre: p.Nombre, Cantidad: it.cant})
			cantPorSKU[p.SKU] = it.cant
		}
		if len(ls) == 0 {
			return
		}
		provs := []compra.ProveedorCotiza{}
		f := hoyDemo.AddDate(0, 0, -diasAtras).Format(time.RFC3339Nano)
		for _, pv := range proveedores {
			prov, ok := provsDemo[pv.id]
			if !ok {
				continue
			}
			pc := compra.ProveedorCotiza{
				ProveedorID: prov.ID, ProveedorNombre: prov.Nombre,
				Estado: compra.CotizaPendiente, Lineas: []compra.RespuestaLinea{},
			}
			if len(pv.respuesta) > 0 {
				var total float64
				for _, l := range ls {
					precio, tiene := pv.respuesta[l.SKU]
					if !tiene {
						continue
					}
					pc.Lineas = append(pc.Lineas, compra.RespuestaLinea{SKU: l.SKU, PrecioUnitario: precio})
					total += precio * cantPorSKU[l.SKU]
				}
				pc.Respondida = len(pc.Lineas) > 0
				if pc.Respondida {
					pc.Estado = compra.CotizaRespondida
					pc.Total = round2Demo(total)
					pc.RespondidaEn = f
				}
			}
			provs = append(provs, pc)
		}
		nSol := s.Numerador.Siguiente(demoEmpID, demoSede1ID, "SOL")
		s.Solicitudes.Create(compra.SolicitudCompra{
			EmpresaID: demoEmpID, SedeID: demoSede1ID,
			Numero: nSol, NumeroCompleto: fmt.Sprintf("SOL-%06d", nSol), Serie: "SOL",
			Estado: estado, Fecha: f, Notas: notas,
			Lineas: ls, Proveedores: provs,
			Actor: application.DemoUserID, Creada: f, Actualizada: f,
		})
	}
	// Enviada a DOS proveedores, ambos con respuesta: Polar más barato en refresco y
	// agua; Alimentos Mary más barato en pasta. Así la comparativa tiene un ganador
	// por línea distinto y el total decide.
	sembrarSolicitud(compra.SolRespondida, "Reposición mensual de bebidas y víveres. Comparar antes de emitir la orden.", 5,
		[]struct {
			sku  string
			cant float64
		}{{"REF-2L", 48}, {"AGU-15", 60}, {"PAS-001", 40}},
		[]struct {
			id        string
			respuesta map[string]float64
		}{
			{"prov_demo_1", map[string]float64{"REF-2L": 1045.00, "AGU-15": 415.00, "PAS-001": 605.00}},
			{"prov_demo_2", map[string]float64{"REF-2L": 1080.00, "AGU-15": 435.00, "PAS-001": 575.00}},
		})
	// Borrador: un solo proveedor, aún sin enviar ni responder.
	sembrarSolicitud(compra.SolBorrador, "Cotizar aceite para el próximo trimestre.", 1,
		[]struct {
			sku  string
			cant float64
		}{{"ACE-001", 24}},
		[]struct {
			id        string
			respuesta map[string]float64
		}{{"prov_demo_2", nil}})

	// --- Cajas y credenciales de puesto ---
	// Sin una caja abierta no se puede facturar (regla de EmitirFactura), así que
	// la demo necesita cajas habilitadas y cajeros con PIN. Los códigos y PINs
	// replican los del prototipo para que el recorrido sea el mismo.
	s.Cajas.Create(caja.Caja{ID: "caja_demo_1", EmpresaID: demoEmpID, SedeID: demoSede1ID, Nombre: "Caja 1", Codigo: "C-001", Estado: caja.EstadoHabilitada, Creada: fecha})
	s.Cajas.Create(caja.Caja{ID: "caja_demo_2", EmpresaID: demoEmpID, SedeID: demoSede1ID, Nombre: "Caja 2", Codigo: "C-002", Estado: caja.EstadoHabilitada, Creada: fecha})
	s.Cajas.Create(caja.Caja{ID: "caja_demo_3", EmpresaID: demoEmpID, SedeID: demoSede2ID, Nombre: "Caja Este", Codigo: "C-003", Estado: caja.EstadoHabilitada, Creada: fecha})

	// Pedro Salas es SUPERVISOR: es quien autoriza quitar una línea del carrito o
	// salir del modo caja cuando la empresa exige PIN de supervisor (flujo 2.4).
	// Un cajero raso no puede autorizarse a sí mismo.
	// TODOS los PIN de demostración son PinDemo ("1234"): un recorrido de demo no debe
	// frenarse porque alguien no recuerda cuál de tres PINs iba en qué puesto.
	for _, cj := range []struct {
		id, sede, codigo, nombre string
		supervisor               bool
	}{
		{"cjr_demo_1", demoSede1ID, "OP-001", "Luis Marcano", false},
		{"cjr_demo_2", demoSede2ID, "OP-002", "Ana Gómez", false},
		{"cjr_demo_3", demoSede1ID, "OP-003", "Pedro Salas", true},
	} {
		s.Cajeros.Create(caja.Cajero{
			ID: cj.id, EmpresaID: demoEmpID, SedeID: cj.sede, Codigo: cj.codigo,
			Nombre: cj.nombre, PinHash: hashDemo(), Supervisor: cj.supervisor, Activo: true,
		})
	}

	// --- Usuarios de la app por rol ---
	// El demo necesita más de una persona para que Configuración › Usuarios y
	// roles muestre algo real y para poder probar el Modo caja con un cajero de
	// verdad: el CAJERO va atado a UNA sede (SedeID), y el backend lo fija en cada
	// petición — no puede elegir sede ni ver otra.
	for _, u := range []struct{ id, nombre, email, rol, sede string }{
		{"usr_demo_vend", "José Rodríguez", "vendedor@lacima.test", usuario.RolVendedor, demoSede1ID},
		{"usr_demo_cajero", "Luis Marcano", "cajero@lacima.test", usuario.RolCajero, demoSede1ID},
		{"usr_demo_conta", "Lcda. Ana Torres", "contadora@lacima.test", usuario.RolContadora, ""},
	} {
		s.Usuarios.Create(usuario.Usuario{ID: u.id, Nombre: u.nombre, Email: u.email})
		s.Membresias.Create(usuario.Membresia{
			UsuarioID: u.id, Email: u.email, Nombre: u.nombre,
			EmpresaID: demoEmpID, Rol: u.rol, SedeID: u.sede, Estado: usuario.EstadoActiva,
		})
		// Con contraseña, para poder ENTRAR como ese rol y ver la app desde su lado
		// (la interfaz cambia por completo según el rol).
		s.Credenciales.Create(credencial.Credencial{
			Email: u.email, Hash: hashDemo(), UsuarioID: u.id, Nombre: u.nombre,
		})
	}
	// La dueña también entra con contraseña (además del botón de modo demo).
	s.Credenciales.Create(credencial.Credencial{
		Email: application.DemoEmail, Hash: hashDemo(),
		UsuarioID: application.DemoUserID, Nombre: application.DemoNombre,
	})

	// Listas de precio demo. Un maestro editable (no ledger): fija un precio
	// explícito por SKU que reemplaza al precio base del catálogo; los productos
	// no listados conservan su precio base. Dos tarifas de venta (mayorista con
	// descuento; una en dólares para el canal de electrónica) y una de compra.
	for _, l := range []listaprecio.ListaPrecio{
		{
			EmpresaID: demoEmpID, Nombre: "Mayorista", Tipo: listaprecio.TipoVenta, Activa: true, Moneda: empresa.MonedaVES,
			Items: []listaprecio.ItemLista{
				{SKU: "ARR-001", Precio: 870.00},  // ~10% menos que 970
				{SKU: "AZU-001", Precio: 935.00},  // ~10% menos que 1040
				{SKU: "REF-2L", Precio: 1340.00},  // ~10% menos que 1490
				{SKU: "ACE-001", Precio: 1880.00}, // ~10% menos que 2090
				{SKU: "PAS-001", Precio: 740.00},  // ~10% menos que 820
			},
		},
		{
			EmpresaID: demoEmpID, Nombre: "Distribuidor (US$)", Tipo: listaprecio.TipoVenta, Activa: true, Moneda: empresa.MonedaUSD,
			Items: []listaprecio.ItemLista{
				{SKU: "ELE-TV", Precio: 135.00}, // menos que los 150 US$ de lista
				{SKU: "ELE-PB", Precio: 13.50},  // ~10% menos que 15 US$
				{SKU: "CAL-001", Precio: 27.00}, // ~10% menos que 30 US$
			},
		},
		{
			EmpresaID: demoEmpID, Nombre: "Proveedor La Cima", Tipo: listaprecio.TipoCompra, Activa: true, Moneda: empresa.MonedaVES,
			Items: []listaprecio.ItemLista{
				{SKU: "ARR-001", Precio: 665.00},  // costo negociado (base costo 680)
				{SKU: "AZU-001", Precio: 715.00},  // (base costo 730)
				{SKU: "ACE-001", Precio: 1435.00}, // (base costo 1465)
			},
		},
	} {
		s.ListasPrecio.Create(l)
	}

	// Cupones de descuento demo. Un maestro editable (no ledger): un código con
	// descuento que baja el precioUnitario de las líneas al cobrar/cotizar. Tres
	// casos para ver los estados: uno PORCENTUAL vigente, uno de MONTO FIJO vigente
	// con monto mínimo, y uno VENCIDO (para ver el estado «vencido»). Las fechas se
	// anclan a hoy para que la vigencia sea real sin importar cuándo se levante.
	hoy := hoyDemo.Format("2006-01-02")
	for _, c := range []cupon.Cupon{
		{
			EmpresaID: demoEmpID, Codigo: "BIENVENIDO10", Descripcion: "10% de descuento de bienvenida",
			Tipo: cupon.TipoPorcentaje, Valor: 10, MontoMinimo: 0,
			Desde: hoyDemo.AddDate(0, -1, 0).Format("2006-01-02"), Hasta: hoyDemo.AddDate(0, 2, 0).Format("2006-01-02"),
			Activo: true, UsosMax: 0,
		},
		{
			EmpresaID: demoEmpID, Codigo: "TASA50", Descripcion: "Bs 50 de descuento en compras desde Bs 500",
			Tipo: cupon.TipoMonto, Valor: 50, MontoMinimo: 500,
			Desde: hoy, Hasta: hoyDemo.AddDate(0, 1, 0).Format("2006-01-02"),
			Activo: true, UsosMax: 100, UsosActuales: 12,
		},
		{
			EmpresaID: demoEmpID, Codigo: "VERANO24", Descripcion: "Promo de verano (vencida)",
			Tipo: cupon.TipoPorcentaje, Valor: 15, MontoMinimo: 0,
			Desde: hoyDemo.AddDate(0, -3, 0).Format("2006-01-02"), Hasta: hoyDemo.AddDate(0, 0, -30).Format("2006-01-02"),
			Activo: true, UsosMax: 0,
		},
	} {
		s.Cupones.Create(c)
	}

	// Promociones demo (biblioteca que alimenta el carrusel de la pantalla del
	// cliente). Maestro editable, no ledger. Tres casos para ver los estados y los
	// dos tipos: una de IMAGEN vigente, una de TEXTO vigente y una de texto VENCIDA
	// (para verla fuera del carrusel aunque siga activa). Las de imagen reusan el
	// data-URI SVG de marca de los slides, para no depender de la red en la demo.
	for _, p := range []promocion.Promocion{
		{
			EmpresaID: demoEmpID, Nombre: "Café La Cima (imagen)", Tipo: promocion.TipoImagen,
			Imagen: publicidadDemo[1].Imagen,
			Desde:  hoyDemo.AddDate(0, 0, -3).Format("2006-01-02"), Hasta: hoyDemo.AddDate(0, 1, 0).Format("2006-01-02"),
			Activa: true, Orden: 1,
		},
		{
			EmpresaID: demoEmpID, Nombre: "2x1 en bebidas", Tipo: promocion.TipoTexto,
			Titulo: "2x1 en bebidas", Subtexto: "Solo esta semana — llévate dos y paga una",
			Desde: hoy, Hasta: hoyDemo.AddDate(0, 0, 15).Format("2006-01-02"),
			Activa: true, Orden: 2,
		},
		{
			EmpresaID: demoEmpID, Nombre: "Promo de verano (vencida)", Tipo: promocion.TipoTexto,
			Titulo: "Verano La Cima", Subtexto: "Refréscate con 20% en toda la nevera",
			Desde: hoyDemo.AddDate(0, -3, 0).Format("2006-01-02"), Hasta: hoyDemo.AddDate(0, 0, -30).Format("2006-01-02"),
			Activa: true, Orden: 3,
		},
	} {
		s.Promociones.Create(p)
	}

	// Unidades de medida por defecto de la empresa demo. Maestro editable (no
	// ledger): el catálogo elige su UnidadBase de aquí. Se siembra el juego común
	// de Venezuela (conteo, peso, volumen, longitud) vía unidadmedida.PorDefecto,
	// una sola fuente compartida con el onboarding de empresas nuevas.
	for _, u := range unidadmedida.PorDefecto(demoEmpID) {
		u.Creada = fecha
		s.Unidades.Create(u)
	}

	// Formatos de documento por defecto de la empresa demo: un juego de presets con
	// los tamaños MÁS COMUNES en Venezuela — factura carta y media carta y ticket de
	// 80 mm; cotización carta; nota de entrega carta y media carta. El primero de
	// cada tipo queda predeterminado. Maestro editable (no ledger).
	presetsDemo := []struct {
		tipo, papel, nombre string
		predet              bool
	}{
		{plantilla.TipoFactura, plantilla.PapelCarta, "Factura · carta", true},
		{plantilla.TipoFactura, plantilla.PapelMediaCarta, "Factura · media carta", false},
		{plantilla.TipoFactura, plantilla.PapelTicket80, "Factura · ticket 80 mm", false},
		{plantilla.TipoCotizacion, plantilla.PapelCarta, "Cotización · carta", true},
		{plantilla.TipoNotaEntrega, plantilla.PapelCarta, "Nota de entrega · carta", true},
		{plantilla.TipoNotaEntrega, plantilla.PapelMediaCarta, "Nota de entrega · media carta", false},
	}
	for _, d := range presetsDemo {
		p := plantilla.PorDefecto(d.tipo, d.papel)
		p.Nombre = d.nombre
		p.EmpresaID = demoEmpID
		p.Activa = true
		p.Predeterminada = d.predet
		p.Creada = fecha
		p.Actualizada = fecha
		s.Plantillas.Create(p)
	}

	// (Sin mesas ni plano acá: el módulo Restaurante vive en su propia demo de rubro,
	// emp_demo_rest. Una bodega con mapa de mesas no tiene sentido.)

}

// Snapshot expone todos los registros sembrados (para que el adaptador Mongo
// siembre desde la misma fuente que in-memory).
type Snapshot struct {
	Orgs           []organizacion.Organizacion
	Empresas       []empresa.Empresa
	Sedes          []sede.Sede
	Usuarios       []usuario.Usuario
	Membresias     []usuario.Membresia
	Rubros         []inventario.Rubro
	Productos      []inventario.Producto
	Movimientos    []inventario.Movimiento
	Clientes       []cliente.Cliente
	CuentasCobro   []fiscal.CuentaCobro
	MetodosPago    []fiscal.MetodoPago
	Dispositivos   []fiscal.DispositivoFiscal
	Retenciones    []fiscal.Retencion
	Cotizaciones   []cotizacion.Cotizacion
	Cajas          []caja.Caja
	Cajeros        []caja.Cajero
	Documentos     []fiscal.Documento
	Transferencias []inventario.Transferencia
	Proveedores    []proveedor.Proveedor
	OrdenesCompra  []compra.OrdenCompra
	Solicitudes    []compra.SolicitudCompra
	ListasPrecio   []listaprecio.ListaPrecio
	Cupones        []cupon.Cupon
	Promociones    []promocion.Promocion
	Unidades       []unidadmedida.UnidadMedida
	Plantillas     []plantilla.Plantilla
	Mesas          []mesa.Mesa
	Tasas          []tasa.Tasa
	// Contadores: estado del numerador fiscal tras sembrar (clave
	// "empresaID|sedeID|serie" → último folio). El adaptador Mongo lo usa para
	// arrancar su colección de contadores adelantada, y no reiniciar en 1.
	Contadores map[string]int
}

// Snapshot vuelca el contenido actual del store.
func (s *Store) Snapshot() Snapshot {
	return Snapshot{
		Orgs:           s.Organizaciones.List(),
		Empresas:       s.Empresas.List(demoOrgID),
		Sedes:          s.Sedes.List(demoEmpID),
		Usuarios:       s.Usuarios.Todos(),
		Membresias:     s.Membresias.ByEmpresa(demoEmpID),
		Rubros:         s.Rubros.List(demoEmpID),
		Productos:      s.Productos.List(demoEmpID),
		Movimientos:    s.Movimientos.List(demoEmpID, inventario.FiltroMovimiento{}),
		Clientes:       s.Clientes.List(demoEmpID),
		CuentasCobro:   s.CuentasCobro.List(demoEmpID),
		MetodosPago:    s.MetodosPago.List(demoEmpID),
		Dispositivos:   s.Dispositivos.List(demoEmpID),
		Retenciones:    s.Retenciones.List(demoEmpID),
		Cotizaciones:   s.Cotizaciones.List(demoEmpID),
		Documentos:     s.Documentos.List(demoEmpID),
		Transferencias: s.Transferencias.List(demoEmpID),
		Proveedores:    s.Proveedores.List(demoEmpID),
		OrdenesCompra:  s.OrdenesCompra.List(demoEmpID),
		Solicitudes:    s.Solicitudes.List(demoEmpID),
		ListasPrecio:   s.ListasPrecio.List(demoEmpID),
		Cupones:        s.Cupones.List(demoEmpID),
		Promociones:    s.Promociones.List(demoEmpID),
		Unidades:       s.Unidades.List(demoEmpID),
		Plantillas:     s.Plantillas.List(demoEmpID),
		Mesas:          s.Mesas.List(demoEmpID, ""),
		Cajas:          s.Cajas.List(demoEmpID),
		Cajeros:        s.Cajeros.List(demoEmpID),
		// Ámbito de plataforma (empresaID vacío): la tasa oficial no es de un
		// tenant.
		Tasas:      s.Tasas.Historial("", 0),
		Contadores: s.Numerador.Estado(),
	}
}
