// Package aplicacion es el marco de MÓDULOS de ElERP: funciones del ERP que se
// comercializan aparte y el cliente instala/activa/desactiva ("Aplicaciones").
//
// Dos piezas: un CATÁLOGO estático de módulos conocidos (definido en código,
// `Catalogo()`) y el ESTADO por empresa (`Instalacion`: si está instalado y activo).
// Los módulos `Core` vienen incluidos y siempre activos (no se desinstalan ni
// desactivan); los comercializables (`Core=false`) el cliente los instala y los
// puede prender/apagar. Un módulo `Proximamente` figura en el catálogo pero aún no
// se puede instalar.
//
// Como el maestro de unidades/almacenes, el estado es EDITABLE (Upsert), no un
// ledger; no hay borrado duro (desinstalar es Instalado=false).
package aplicacion

// Categorías de módulo (para agrupar en la vitrina de Aplicaciones).
const (
	CategoriaOperacion = "operacion"
	CategoriaComercial = "comercial"
	CategoriaFinanzas  = "finanzas"
	CategoriaGestion   = "gestion"
)

// IDs de módulo conocidos (los que el resto del código consulta para gating).
const (
	ModMarketing   = "marketing"
	ModAsistenteIA = "asistente-ia"
	ModRestaurante = "restaurante"
)

// Modulo es una entrada del catálogo estático (misma para todas las empresas).
type Modulo struct {
	ID           string `json:"id"`
	Nombre       string `json:"nombre"`
	Descripcion  string `json:"descripcion"`
	Detalle      string `json:"detalle"` // texto largo para "Más información"
	Categoria    string `json:"categoria"`
	Core         bool   `json:"core"`         // incluido: siempre activo, no se instala/desactiva
	Proximamente bool   `json:"proximamente"` // en el catálogo pero aún no instalable
	// Requiere lista los ids de módulos que deben estar ACTIVOS para instalar/activar
	// este módulo (dependencias). Vacío = sin dependencias.
	Requiere []string `json:"requiere,omitempty"`
	// AvisoDesinstalar es la advertencia que se muestra ANTES de desinstalar cuando
	// hay impacto (datos que se ocultan, funciones que se pierden). Vacío = sin aviso.
	AvisoDesinstalar string `json:"avisoDesinstalar,omitempty"`
}

// Instalacion es el estado de un módulo EN UNA empresa (solo para no-core).
type Instalacion struct {
	EmpresaID   string `json:"empresaId" bson:"empresaid"`
	ModuloID    string `json:"moduloId" bson:"moduloid"`
	Instalado   bool   `json:"instalado" bson:"instalado"`
	Activo      bool   `json:"activo" bson:"activo"`
	Actualizada string `json:"actualizada" bson:"actualizada"` // RFC3339
}

// Repository persiste el estado de módulos por empresa, aislado por empresaID.
type Repository interface {
	List(empresaID string) []Instalacion
	ByID(empresaID, moduloID string) (Instalacion, bool)
	// Upsert crea o reemplaza el estado (empresaID+moduloID es la clave lógica).
	Upsert(i Instalacion) Instalacion
}

// Catalogo devuelve el catálogo estático de módulos de ElERP. El orden es el de la
// vitrina. Los `Core` son el ERP incluido; `marketing` es el primer módulo
// comercializable real; el resto de comercializables van como `Proximamente`.
func Catalogo() []Modulo {
	return []Modulo{
		{ID: "inventario", Nombre: "Inventario y almacenes", Descripcion: "Catálogo, existencias por almacén, Kardex y transferencias.", Categoria: CategoriaOperacion, Core: true,
			Detalle: "Controla tus productos y su stock por sede y por almacén. Ledger append-only: existencias y costo promedio se derivan de los movimientos (nunca contadores editables), así el Kardex siempre cuadra. Incluye carga masiva de productos, transferencias entre almacenes, ajustes auditados y almacenes con capacidad y rubros admitidos."},
		{ID: "facturacion", Nombre: "Facturación fiscal", Descripcion: "Documentos fiscales, notas de crédito/débito, retenciones y cierres Z.", Categoria: CategoriaOperacion, Core: true,
			Detalle: "Emisión de facturas con RIF (dígito verificador SENIAT), notas de crédito y débito como reversas (append-only), retenciones de IVA/ISLR, libros de venta/compra y cierres Z. Numeración atómica por empresa+sede+serie."},
		{ID: "ventas", Nombre: "Ventas y clientes", Descripcion: "Cotizaciones, pedidos, clientes, cupones y listas de precio.", Categoria: CategoriaOperacion, Core: true,
			Detalle: "Venta forma libre: cotización → confirmación → factura, reusando el mismo motor fiscal del POS (cobro mixto multimoneda, IGTF y vuelto). Gestión de clientes, cupones de descuento y listas de precio."},
		{ID: "compras", Nombre: "Compras y proveedores", Descripcion: "Solicitudes, órdenes, recepción y facturas de proveedor.", Categoria: CategoriaOperacion, Core: true,
			Detalle: "Ciclo completo con proveedores: solicitud de presupuesto (RFQ) con comparativa, orden de compra, recepción (que mueve el inventario) y factura del proveedor flexible con captura del IVA real y diferencias a la cuenta de compras."},
		{ID: "contabilidad", Nombre: "Contabilidad", Descripcion: "Plan de cuentas, libro diario y estados financieros.", Categoria: CategoriaFinanzas, Core: true,
			Detalle: "Libro diario append-only con asientos DERIVADOS de las operaciones (venta, compra, cobro, pago, ajuste de inventario): nada se cuadra a mano. Plan de cuentas y estados financieros proyectados del diario."},
		{ID: "finanzas", Nombre: "Tesorería y finanzas", Descripcion: "Cuentas por cobrar y pagar, IGTF y enlaces de pago.", Categoria: CategoriaFinanzas, Core: true,
			Detalle: "Cuentas por cobrar (abonos de ventas a crédito) y por pagar (pagos a proveedor), reporte de IGTF, cuentas de cobro y métodos de pago configurables. Saldos derivados de los movimientos."},
		{ID: ModMarketing, Nombre: "Marketing y pantalla del cliente", Descripcion: "Publicidad en la pantalla auxiliar de la caja: biblioteca de promociones y carrusel del cliente.", Categoria: CategoriaComercial, Core: false,
			Detalle:          "Convierte la segunda pantalla de la caja en un canal de publicidad: una biblioteca de promociones (de imagen o de texto, con vigencia) que se muestran en un carrusel mientras se cobra. Al desactivar el módulo, la pantalla del cliente conserva su branding y el modo productos; solo se apaga la publicidad.",
			AvisoDesinstalar: "Se ocultará la sección Marketing de Configuración y el carrusel de publicidad de la pantalla del cliente. Tus promociones NO se borran: vuelven a estar disponibles al reactivar el módulo."},
		{ID: "cupones", Nombre: "Cupones de descuento", Descripcion: "Códigos de descuento aplicables en el punto de venta y en ventas.", Categoria: CategoriaComercial, Core: true,
			Detalle: "Códigos de descuento (monto o porcentaje) con vigencia y tope de usos, aplicables en el punto de venta y en cotizaciones. Ya incluido; se administra en Ventas › Cupones."},
		{ID: "listas-precio", Nombre: "Listas de precio", Descripcion: "Tarifas de venta y compra por lista.", Categoria: CategoriaComercial, Core: true,
			Detalle: "Define tarifas por lista para venta y compra. Ya incluido; se administra en Ventas › Listas de precio y en Compras."},
		{ID: "reportes-bi", Nombre: "Reportes y BI", Descripcion: "Paneles y análisis de ventas, inventario, compras y cobranza.", Categoria: CategoriaGestion, Core: true,
			Detalle: "Panel ejecutivo y reportes de ventas, inventario, compras y cobranza (con gráficos y antigüedad de saldos). Ya incluido; se abre desde el módulo Reportes y BI."},
		{ID: ModRestaurante, Nombre: "Restaurante", Descripcion: "Mapa de mesas, comandas a cocina y platos con receta (insumos).", Categoria: CategoriaOperacion, Core: false,
			Detalle:          "Convierte el punto de venta en el flujo de un restaurante: diseña el MAPA DE MESAS del salón (arrastrar y soltar), abre una CUENTA por mesa que el mesonero o la caja van armando, y cada pedido genera una COMANDA que se imprime en cocina. Los platos son productos COMPUESTOS (receta/escandallo): al vender un plato se descuentan sus insumos del inventario (gramos de pasta, queso, etc.). Al cerrar la mesa se emite la factura fiscal con el mismo motor de siempre (propina, IGTF en divisas, división de cuenta).",
			AvisoDesinstalar: "Se ocultarán el mapa de mesas, la comandera y la cocina. Tus mesas y recetas NO se borran: vuelven al reactivar el módulo."},
		{ID: ModAsistenteIA, Nombre: "Asistente IA", Descripcion: "Asistente conversacional flotante: responde sobre tus datos y guía el uso de ElERP.", Categoria: CategoriaGestion, Core: false,
			Detalle:          "Un asistente flotante en toda la app. Responde MECÁNICAMENTE (100% local, sin enviar datos a nadie) las consultas de tus datos —ventas del mes, cartera vencida, stock de un producto, cuentas por pagar, si el libro cuadra— y las dudas de uso del producto (¿qué es el IGTF?, ¿cómo transfiero stock?), acotado a tu rol. Opcionalmente puede habilitarse una capa de IA (opt-in, en Configuración › Asistente) para las preguntas abiertas: esa capa responde SOLO con el contexto de tu instancia y no navega internet.",
			AvisoDesinstalar: "Se ocultará el asistente flotante en toda la app. Tu configuración de IA se conserva y vuelve al reactivar el módulo."},
	}
}

// EnCatalogo busca un módulo del catálogo por id.
func EnCatalogo(id string) (Modulo, bool) {
	for _, m := range Catalogo() {
		if m.ID == id {
			return m, true
		}
	}
	return Modulo{}, false
}
