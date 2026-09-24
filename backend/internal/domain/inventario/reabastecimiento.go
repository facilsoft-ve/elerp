package inventario

// REGLA DE REABASTECIMIENTO: cuándo hay que volver a comprar, y cuánto.
//
// POR QUÉ EXISTE. Sin esto, reponer depende de que alguien mire la pantalla de
// existencias a tiempo. Funciona con veinte productos y deja de funcionar con
// doscientos: lo que se agota es siempre lo que nadie estaba mirando.
//
// LO QUE NO HACE, Y ES DELIBERADO: no compra. Genera una SOLICITUD de compra —lo
// que en el flujo ya existente se le pide a los proveedores para que coticen—, y
// una persona decide. Una regla que emite órdenes de compra sola convierte un
// error de configuración en una deuda con un proveedor.
type ReglaReabastecimiento struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	SedeID    string `json:"sedeId" bson:"sedeid"`
	// AlmacenID acota la regla a un depósito. Vacío = toda la sede, que es lo
	// normal: se repone para la sede, no para un estante.
	AlmacenID  string `json:"almacenId,omitempty" bson:"almacenid,omitempty"`
	ProductoID string `json:"productoId" bson:"productoid"`
	SKU        string `json:"sku" bson:"sku"`
	// Minimo es el punto de pedido: cuando la cobertura baja hasta aquí, se repone.
	// Es el nivel con el que se aguanta lo que tarda el proveedor en traer más.
	Minimo float64 `json:"minimo" bson:"minimo"`
	// Maximo es hasta dónde se repone. Tiene que ser mayor que el mínimo: si fueran
	// iguales, cada venta dispararía un pedido del tamaño de esa venta.
	Maximo float64 `json:"maximo" bson:"maximo"`
	// Multiplo redondea la cantidad HACIA ARRIBA: si el proveedor solo vende cajas
	// de 12, pedir 7 es pedir 12. Cero o uno significa «sin restricción».
	Multiplo float64 `json:"multiplo,omitempty" bson:"multiplo,omitempty"`
	// ProveedorID es a quién se le suele comprar. Se usa para agrupar la solicitud
	// por proveedor; vacío significa que se decide al pedir presupuesto.
	ProveedorID string `json:"proveedorId,omitempty" bson:"proveedorid,omitempty"`
	Activa      bool   `json:"activa" bson:"activa"`
	Creada      string `json:"creada" bson:"creada"`
}

// CantidadAReponer calcula cuánto pedir dada la cobertura actual.
//
// COBERTURA, NO EXISTENCIA. Quien llama pasa lo disponible MÁS lo que ya viene en
// camino. Las dos partes importan y por motivos distintos:
//
//   - Si se mirara la existencia en vez del disponible, se pediría de menos cuando
//     hay mercancía apartada: está en el almacén pero ya tiene dueño.
//   - Si no se contara lo pedido y aún no recibido, la regla volvería a pedir lo
//     mismo cada vez que se ejecuta hasta que llegara el primer camión. Es el fallo
//     clásico de los reabastecimientos automáticos, y el que hace que se acaben
//     apagando.
//
// Devuelve 0 cuando no hay que pedir: por encima del mínimo, o con una regla mal
// puesta (máximo ≤ mínimo). Nunca devuelve negativo.
func (r ReglaReabastecimiento) CantidadAReponer(cobertura float64) float64 {
	if !r.Activa || r.Maximo <= r.Minimo {
		return 0
	}
	// El punto de pedido se cruza al TOCAR el mínimo, no al bajar de él: con mínimo
	// 10, tener exactamente 10 ya es el momento de reponer. Esperar a 9 significa
	// que el proveedor empieza a traer cuando ya se está por debajo del colchón.
	if cobertura > r.Minimo+0.0001 {
		return 0
	}
	falta := r.Maximo - cobertura
	if falta <= 0.0001 {
		return 0
	}
	if r.Multiplo > 0.0001 {
		// Hacia arriba: pedir menos que el múltiplo deja el pedido por debajo del
		// máximo y la regla volvería a dispararse en la siguiente revisión.
		veces := falta / r.Multiplo
		enteras := float64(int64(veces))
		if veces-enteras > 0.0001 {
			enteras++
		}
		falta = enteras * r.Multiplo
	}
	return falta
}

// ReglaReabastecimientoRepo persiste las reglas, aislado por empresaID. Editable,
// sin borrado duro: desactivar conserva el histórico de por qué se pedía.
type ReglaReabastecimientoRepo interface {
	List(empresaID string) []ReglaReabastecimiento
	ByID(empresaID, id string) (ReglaReabastecimiento, bool)
	Create(r ReglaReabastecimiento) ReglaReabastecimiento
	Update(r ReglaReabastecimiento) (ReglaReabastecimiento, bool)
}
