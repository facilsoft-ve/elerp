package fiscal

// MOTIVOS de las notas de crédito y débito.
//
// POR QUÉ UN CATÁLOGO Y NO TEXTO LIBRE (notas de la contadora, 15:18, 15:19 y
// 15:24): «se debe poder generar una nota de crédito por DESCUENTO», «por
// AJUSTE DE PRECIO INDIVIDUAL DE PRODUCTO», y «las notas de débito actualmente
// son por DIFERENCIAL CAMBIARIO o porque SE COBRÓ DE MENOS».
//
// Hasta acá el motivo era una cadena que alguien escribía, y la nota de crédito
// solo sabía hacer una cosa: devolver mercancía. Eso tiene dos consecuencias que
// no se ven hasta que duelen:
//
//   - No se puede declarar ni auditar por motivo: «descuento», «Descuento
//     comercial» y «dto.» son tres cosas distintas para una consulta.
//   - Y la grave: ACREDITAR UN DESCUENTO REINGRESABA MERCANCÍA AL INVENTARIO.
//     Nadie devolvió nada; solo se cobró de más. El stock quedaba inflado y el
//     costo de ventas mal, en silencio.
//
// De ahí que el motivo sea un CÓDIGO del catálogo y que de él dependa si la nota
// toca el inventario. Es la única pieza del documento que decide eso.

// Motivos de NOTA DE CRÉDITO.
const (
	// MotivoNCDevolucion: el cliente devolvió mercancía. Es el ÚNICO motivo que
	// reingresa stock, porque es el único en el que los bienes vuelven.
	MotivoNCDevolucion = "devolucion"
	// MotivoNCDescuento: se concede un descuento después de facturar (pronto
	// pago, acuerdo comercial). Baja el monto, NO devuelve mercancía.
	MotivoNCDescuento = "descuento"
	// MotivoNCAjustePrecio: se facturó a un precio MAYOR que el correcto. Se
	// acredita la diferencia por producto. Tampoco vuelve mercancía: lo que
	// estuvo mal fue el precio, no la cantidad.
	MotivoNCAjustePrecio = "ajuste_precio"
	// MotivoNCError: error de facturación distinto de los anteriores (datos del
	// receptor, renglón cargado de más). Existe para no empujar a la gente a
	// elegir un motivo falso cuando ninguno encaja.
	MotivoNCError = "error_facturacion"
)

// Motivos de NOTA DE DÉBITO. Los dos primeros son los que declaró la contadora
// como los usos reales de hoy.
const (
	// MotivoNDDiferencialCambiario: la factura se pactó en divisas y al cobrar la
	// tasa era otra; el cargo cubre la diferencia.
	MotivoNDDiferencialCambiario = "diferencial_cambiario"
	// MotivoNDCobroDeMenos: se facturó por debajo de lo que correspondía.
	MotivoNDCobroDeMenos = "cobro_de_menos"
	// MotivoNDInteresesMora: intereses por pago tardío. No causan IVA, así que
	// este cargo suele ir exento.
	MotivoNDInteresesMora = "intereses_mora"
	// MotivoNDGastos: gastos repercutidos (flete, manejo) posteriores a la
	// factura.
	MotivoNDGastos = "gastos"
)

// MotivoNota describe una entrada del catálogo, para que la interfaz no tenga
// que traer la lista escrita a mano y desincronizarse.
type MotivoNota struct {
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
	// Ayuda explica CUÁNDO se usa. Quien emite una nota no siempre sabe cuál le
	// toca, y elegir mal el motivo acá tiene consecuencias contables.
	Ayuda string `json:"ayuda"`
	// MueveInventario avisa en la interfaz lo que el servidor hace cumplir.
	MueveInventario bool `json:"mueveInventario"`
}

// MotivosNotaCredito es el catálogo de la NC.
func MotivosNotaCredito() []MotivoNota {
	return []MotivoNota{
		{Codigo: MotivoNCDevolucion, Nombre: "Devolución de mercancía",
			Ayuda: "El cliente devolvió los productos. Reingresan al inventario.", MueveInventario: true},
		{Codigo: MotivoNCDescuento, Nombre: "Descuento",
			Ayuda: "Se concede un descuento después de facturar. No devuelve mercancía."},
		{Codigo: MotivoNCAjustePrecio, Nombre: "Ajuste de precio",
			Ayuda: "Se facturó a un precio mayor que el correcto. Se acredita la diferencia."},
		{Codigo: MotivoNCError, Nombre: "Error de facturación",
			Ayuda: "Otro error del documento. No devuelve mercancía."},
	}
}

// MotivosNotaDebito es el catálogo de la ND.
func MotivosNotaDebito() []MotivoNota {
	return []MotivoNota{
		{Codigo: MotivoNDDiferencialCambiario, Nombre: "Diferencial cambiario",
			Ayuda: "La tasa al cobrar difiere de la de la factura; el cargo cubre la diferencia."},
		{Codigo: MotivoNDCobroDeMenos, Nombre: "Se cobró de menos",
			Ayuda: "La factura quedó por debajo de lo que correspondía."},
		{Codigo: MotivoNDInteresesMora, Nombre: "Intereses de mora",
			Ayuda: "Intereses por pago tardío. No causan IVA: suelen ir exentos."},
		{Codigo: MotivoNDGastos, Nombre: "Gastos repercutidos",
			Ayuda: "Flete, manejo u otros gastos posteriores a la factura."},
	}
}

// MotivoNCValido acota el motivo de una nota de crédito.
func MotivoNCValido(c string) bool { return buscarMotivo(MotivosNotaCredito(), c) }

// MotivoNDValido acota el motivo de una nota de débito.
func MotivoNDValido(c string) bool { return buscarMotivo(MotivosNotaDebito(), c) }

func buscarMotivo(cat []MotivoNota, codigo string) bool {
	for _, m := range cat {
		if m.Codigo == codigo {
			return true
		}
	}
	return false
}

// NCDevuelveMercancia decide si una nota de crédito con ese motivo reingresa
// stock. Es LA regla de este archivo: solo la devolución mueve inventario.
//
// El motivo vacío se trata como devolución por compatibilidad — las notas
// emitidas antes del catálogo eran todas devoluciones y su stock ya se reingresó;
// leerlas de otra forma cambiaría el histórico.
func NCDevuelveMercancia(motivoCodigo string) bool {
	return motivoCodigo == "" || motivoCodigo == MotivoNCDevolucion
}

// NombreMotivo devuelve la etiqueta legible de un código (vacío si no está en el
// catálogo), para que el documento guarde el texto que la gente lee.
func NombreMotivo(cat []MotivoNota, codigo string) string {
	for _, m := range cat {
		if m.Codigo == codigo {
			return m.Nombre
		}
	}
	return ""
}
