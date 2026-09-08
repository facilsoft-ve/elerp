// Package plantilla modela los FORMATOS de documento que la empresa imprime:
// factura, cotización, nota de entrega, recibo, presupuesto y orden de compra.
//
// Un formato es una PLANTILLA visual: un lienzo de un tamaño de papel dado
// (carta, media carta, A4, tickets de 80/58 mm o uno a medida) sobre el que se
// colocan BLOQUES posicionados en milímetros (texto fijo, campos dinámicos del
// documento, la tabla de renglones, los totales, el logo, un separador o un QR).
// El editor del front arrastra y suelta esos bloques; aquí solo se guardan sus
// coordenadas y estilo.
//
// Es CONFIGURACIÓN EDITABLE (como el maestro de unidades o el de cupones), no un
// ledger append-only: una plantilla se crea, edita y borra. Lo que un documento
// YA emitido imprimió no depende de la plantilla viva —el documento fiscal es
// inmutable por su cuenta—; cambiar un formato solo afecta a lo que se imprima
// de aquí en adelante.
//
// Selección por sede: cada sede puede usar un formato distinto para un mismo
// tipo de documento. La asignación vive en el campo `Sedes` de la plantilla (las
// sedes que la usan para SU tipo); una plantilla marcada `Predeterminada` es la
// que se usa cuando una sede no tiene asignación explícita para ese tipo.
package plantilla

import "strings"

// Tipos de documento que un formato puede representar.
const (
	TipoFactura     = "factura"
	TipoCotizacion  = "cotizacion"
	TipoNotaEntrega = "nota_entrega"
	TipoRecibo      = "recibo"
	TipoPresupuesto = "presupuesto"
	TipoOrden       = "orden_compra"
)

// TiposValidos es el orden canónico de los tipos, para poblar el select del front.
var TiposValidos = []string{
	TipoFactura, TipoCotizacion, TipoNotaEntrega, TipoRecibo, TipoPresupuesto, TipoOrden,
}

// TipoValido indica si el tipo de documento es uno de los admitidos.
func TipoValido(t string) bool {
	for _, v := range TiposValidos {
		if v == t {
			return true
		}
	}
	return false
}

// NormalizarTipo deja el tipo en minúsculas y sin espacios.
func NormalizarTipo(t string) string { return strings.ToLower(strings.TrimSpace(t)) }

// Tamaños de papel preestablecidos. `PapelPersonalizado` usa AnchoMM/AltoMM del
// propio formato; el resto deriva sus dimensiones de PapelDimensiones.
const (
	PapelCarta         = "carta"       // Letter US: 215.9 × 279.4 mm
	PapelMediaCarta    = "media_carta" // Half Letter: 215.9 × 139.7 mm
	PapelA4            = "a4"          // A4: 210 × 297 mm
	PapelTicket80      = "ticket_80"   // rollo POS 80 mm
	PapelTicket58      = "ticket_58"   // rollo POS 58 mm
	PapelPersonalizado = "custom"
)

// PapelDimensiones devuelve el ancho y alto en milímetros de un tamaño
// preestablecido. Para los tickets de rollo el alto es NOMINAL (el papel es
// continuo): se usa un largo cómodo para editar y previsualizar. Devuelve
// ok=false para el papel a medida (custom), cuyas dimensiones las trae el formato.
func PapelDimensiones(papel string) (ancho, alto float64, ok bool) {
	switch papel {
	case PapelCarta:
		return 215.9, 279.4, true
	case PapelMediaCarta:
		return 215.9, 139.7, true
	case PapelA4:
		return 210, 297, true
	case PapelTicket80:
		return 80, 200, true
	case PapelTicket58:
		return 58, 200, true
	}
	return 0, 0, false
}

// PapelValido indica si el tamaño de papel es uno de los admitidos (incluido
// custom).
func PapelValido(p string) bool {
	if p == PapelPersonalizado {
		return true
	}
	_, _, ok := PapelDimensiones(p)
	return ok
}

// Tipos de bloque del lienzo.
const (
	BloqueTexto      = "texto"       // texto fijo escrito por el usuario
	BloqueCampo      = "campo"       // campo dinámico del documento (ver CamposDisponibles)
	BloqueTablaItems = "tabla_items" // la tabla de renglones del documento
	BloqueTotales    = "totales"     // el bloque de subtotal/IVA/total
	BloqueSeparador  = "separador"   // una línea horizontal
	BloqueLogo       = "logo"        // el logo de la empresa
	BloqueQR         = "qr"          // QR de verificación del documento
	BloqueImagen     = "imagen"      // una imagen subida por el usuario (sello, banner, firma)
)

// Límite y formatos SEGUROS de una imagen embebida en un bloque. La imagen se
// guarda como data URI dentro del formato; se aceptan solo mapas de bits comunes
// (PNG/JPEG/WebP) y NUNCA SVG (evita XSS/scripts embebidos). El límite acota el
// peso del documento y del bootstrap.
const (
	// MaxImagenBase64 es el largo máximo del data URI (≈ 512 KB de imagen binaria).
	MaxImagenBase64 = 700 * 1024
)

// prefijosImagenSeguros son los encabezados de data URI aceptados (mapas de bits).
var prefijosImagenSeguros = []string{
	"data:image/png;base64,",
	"data:image/jpeg;base64,",
	"data:image/jpg;base64,",
	"data:image/webp;base64,",
}

// ImagenDataURISegura indica si el data URI es una imagen de un formato seguro
// (PNG/JPEG/WebP) y no excede el peso máximo. Una cadena vacía es válida (bloque
// de imagen aún sin subir).
func ImagenDataURISegura(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > MaxImagenBase64 {
		return false
	}
	for _, p := range prefijosImagenSeguros {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// Alineaciones de texto de un bloque.
const (
	AlinIzquierda = "izquierda"
	AlinCentro    = "centro"
	AlinDerecha   = "derecha"
)

// Bloque es un elemento posicionado sobre el lienzo, en milímetros desde la
// esquina superior izquierda del papel. El front lo arrastra, redimensiona y
// estiliza; el backend solo lo guarda y revalida sus límites.
type Bloque struct {
	ID   string  `json:"id" bson:"id"`
	Tipo string  `json:"tipo" bson:"tipo"`
	X    float64 `json:"x" bson:"x"`
	Y    float64 `json:"y" bson:"y"`
	W    float64 `json:"w" bson:"w"`
	H    float64 `json:"h" bson:"h"`
	// Texto es el contenido del bloque `texto` (o la etiqueta de un `campo`).
	Texto string `json:"texto,omitempty" bson:"texto,omitempty"`
	// Campo es el identificador del dato dinámico para el bloque `campo`
	// (p. ej. "emisor.nombre", "doc.numero", "cliente.rif"). Ver CamposDisponibles.
	Campo string `json:"campo,omitempty" bson:"campo,omitempty"`
	// Imagen es el data URI (PNG/JPEG/WebP) del bloque `imagen`: un sello, banner o
	// firma que sube el usuario. Se valida formato y peso (ver ImagenDataURISegura).
	Imagen string `json:"imagen,omitempty" bson:"imagen,omitempty"`
	// Estilo del texto.
	Alineacion string  `json:"alineacion,omitempty" bson:"alineacion,omitempty"`
	Tamano     float64 `json:"tamano,omitempty" bson:"tamano,omitempty"` // en puntos
	Negrita    bool    `json:"negrita,omitempty" bson:"negrita,omitempty"`
	Color      string  `json:"color,omitempty" bson:"color,omitempty"`
	// Opciones son ajustes específicos del tipo de bloque (banderas on/off) que el
	// render respeta al imprimir. Ver las constantes Op*. Un mapa (en vez de campos
	// fijos) mantiene el bloque genérico: cada tipo usa solo las que le aplican.
	Opciones map[string]bool `json:"opciones,omitempty" bson:"opciones,omitempty"`
}

// Opciones de bloque (banderas). El render las respeta al imprimir; el editor las
// ofrece como interruptores según el tipo de bloque.
const (
	// --- Bloque de totales ---
	// OpTotalesTasa muestra la tasa de cambio del documento ("Tasa BCV: Bs …/US$").
	OpTotalesTasa = "mostrarTasa"
	// OpTotalesDivisa muestra el total REFERENCIAL en divisa ("Total US$ (ref.): …"),
	// convertido con la tasa del documento.
	OpTotalesDivisa = "mostrarDivisa"
	// Nota: el IGTF (pago en divisas) se muestra SIEMPRE que el documento traiga un
	// monto de IGTF > 0; no necesita bandera.

	// --- Bloque de tabla de renglones ---
	// OpItemsMoneda indica la moneda en el encabezado de las columnas de importe
	// ("Precio (Bs)", "Total (Bs)") y deja las celdas como número limpio.
	OpItemsMoneda = "mostrarMoneda"
	// OpItemsExento marca con asterisco los renglones exentos de IVA y agrega una
	// nota al pie ("* Exento de IVA"), solo si hay al menos uno.
	OpItemsExento = "marcarExento"
)

// TipoBloqueValido indica si el tipo de bloque es uno de los admitidos.
func TipoBloqueValido(t string) bool {
	switch t {
	case BloqueTexto, BloqueCampo, BloqueTablaItems, BloqueTotales, BloqueSeparador, BloqueLogo, BloqueQR, BloqueImagen:
		return true
	}
	return false
}

// Plantilla es un formato de documento de la empresa.
type Plantilla struct {
	ID        string `json:"id" bson:"id"`
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Nombre    string `json:"nombre" bson:"nombre"`
	Tipo      string `json:"tipo" bson:"tipo"`   // TipoFactura, TipoCotizacion, …
	Papel     string `json:"papel" bson:"papel"` // PapelCarta, PapelTicket80, …
	// AnchoMM/AltoMM son las dimensiones efectivas del lienzo. Para un papel
	// preestablecido las fija el dominio (PapelDimensiones); para `custom` las
	// define el usuario.
	AnchoMM float64  `json:"anchoMm" bson:"anchomm"`
	AltoMM  float64  `json:"altoMm" bson:"altomm"`
	Bloques []Bloque `json:"bloques" bson:"bloques"`
	// Sedes son las sedes que usan ESTE formato para su tipo. Una sede aparece en
	// a lo sumo una plantilla por tipo (la asignación la mueve de una a otra).
	Sedes []string `json:"sedes" bson:"sedes"`
	// Predeterminada marca el formato usado para su tipo cuando una sede no tiene
	// asignación explícita. A lo sumo una plantilla por tipo lo es.
	Predeterminada bool `json:"predeterminada" bson:"predeterminada"`
	// Activa: una plantilla desactivada no se ofrece para asignar ni imprimir.
	Activa      bool   `json:"activa" bson:"activa"`
	Creada      string `json:"creada" bson:"creada"`
	Actualizada string `json:"actualizada" bson:"actualizada"`
}

// Repository persiste plantillas de documento, aislado por empresaID. Editable
// (no ledger): expone Update y Delete a propósito.
type Repository interface {
	List(empresaID string) []Plantilla
	ByID(empresaID, id string) (Plantilla, bool)
	Create(p Plantilla) Plantilla
	Update(p Plantilla) (Plantilla, bool)
	Delete(empresaID, id string) bool
}

// TipoEtiqueta devuelve el nombre legible de un tipo de documento (para títulos
// y encabezados por defecto).
func TipoEtiqueta(tipo string) string {
	switch tipo {
	case TipoFactura:
		return "FACTURA"
	case TipoCotizacion:
		return "COTIZACIÓN"
	case TipoNotaEntrega:
		return "NOTA DE ENTREGA"
	case TipoRecibo:
		return "RECIBO"
	case TipoPresupuesto:
		return "PRESUPUESTO"
	case TipoOrden:
		return "ORDEN DE COMPRA"
	}
	return strings.ToUpper(tipo)
}

// esNotaEntrega indica si el tipo es una nota de entrega/despacho: su pie lleva
// líneas de RECEPCIÓN (recibido por / firma), no el bloque de totales.
func esNotaEntrega(tipo string) bool { return tipo == TipoNotaEntrega }

// PorDefecto arma un formato con una disposición sensata para el TIPO de documento
// y el TAMAÑO de papel dados — el punto de partida que el editor de arrastrar-y-
// soltar deja retocar. Distingue dos familias de papel:
//
//   - Página completa (carta, media carta, A4, o a medida ≥ 120 mm de ancho):
//     encabezado a dos columnas (logo + emisor a la izquierda; título, número y
//     fecha a la derecha), datos del cliente, la tabla de renglones que llena el
//     centro, y un pie. En factura/cotización/presupuesto el pie lleva el bloque
//     de totales y las observaciones; en nota de entrega, líneas de recepción.
//   - Ticket de rollo (ticket 80/58 mm, o a medida angosto): todo apilado y
//     centrado, con fuentes chicas, tal como sale de una impresora térmica.
//
// Son los formatos MÁS COMUNES en Venezuela: forma libre tamaño carta y media
// carta (Providencia SNAT/2011/00071), y el rollo de 80/58 mm de las cajas.
// Coordenadas en milímetros. ID/EmpresaID/fechas los fija el caso de uso.
func PorDefecto(tipo, papel string) Plantilla {
	W, H, ok := PapelDimensiones(papel)
	if !ok { // custom u desconocido: se asume carta
		W, H = 215.9, 279.4
	}
	titulo := TipoEtiqueta(tipo)
	b := func(bl Bloque) Bloque {
		if bl.Alineacion == "" {
			bl.Alineacion = AlinIzquierda
		}
		if bl.Tamano == 0 {
			bl.Tamano = 9
		}
		return bl
	}
	campo := func(id, clave string, x, y, w, h, pt float64, alin string, neg bool) Bloque {
		return b(Bloque{ID: id, Tipo: BloqueCampo, Campo: clave, X: x, Y: y, W: w, H: h, Tamano: pt, Alineacion: alin, Negrita: neg})
	}
	texto := func(id, t string, x, y, w, h, pt float64, alin string, neg bool) Bloque {
		return b(Bloque{ID: id, Tipo: BloqueTexto, Texto: t, X: x, Y: y, W: w, H: h, Tamano: pt, Alineacion: alin, Negrita: neg})
	}

	var bloques []Bloque
	if W >= 120 {
		// ---- Página completa ----
		m := 12.0
		if W < 160 {
			m = 10
		}
		cw := W - 2*m
		derX := W - m - 74 // columna derecha (título/número/fecha)
		bloques = []Bloque{
			{ID: "logo", Tipo: BloqueLogo, X: m, Y: m, W: 34, H: 16},
			campo("emisor_nombre", "emisor.nombre", m+38, m, derX-(m+38)-2, 6, 12, AlinIzquierda, true),
			campo("emisor_rif", "emisor.rif", m+38, m+6.5, derX-(m+38)-2, 5, 9, AlinIzquierda, false),
			campo("emisor_dir", "emisor.direccion", m+38, m+11.5, derX-(m+38)-2, 8, 8, AlinIzquierda, false),
			texto("titulo", titulo, derX, m, 74, 8, 16, AlinDerecha, true),
			texto("lbl_numero", "N°:", derX, m+10, 26, 5, 9, AlinDerecha, true),
			campo("numero", "doc.numero", derX+28, m+10, 46, 5, 9, AlinDerecha, false),
			texto("lbl_fecha", "Fecha:", derX, m+15.5, 26, 5, 9, AlinDerecha, true),
			campo("fecha", "doc.fecha", derX+28, m+15.5, 46, 5, 9, AlinDerecha, false),
			{ID: "sep1", Tipo: BloqueSeparador, X: m, Y: m + 24, W: cw, H: 0.4},
			texto("lbl_cliente", "Cliente", m, m+27, 40, 5, 9, AlinIzquierda, true),
			campo("cliente_nombre", "cliente.nombre", m, m+32, cw*0.62, 6, 10, AlinIzquierda, false),
			campo("cliente_rif", "cliente.rif", m, m+38, cw*0.62, 5, 9, AlinIzquierda, false),
			campo("cliente_dir", "cliente.direccion", m, m+43, cw*0.62, 6, 8, AlinIzquierda, false),
		}
		itemsY := m + 51
		footerH := 32.0
		if esNotaEntrega(tipo) {
			footerH = 24
		}
		footerY := H - m - footerH
		itemsH := footerY - itemsY - 3
		if itemsH < 18 {
			itemsH = 18
		}
		bloques = append(bloques, Bloque{ID: "items", Tipo: BloqueTablaItems, X: m, Y: itemsY, W: cw, H: itemsH, Opciones: map[string]bool{OpItemsMoneda: true, OpItemsExento: true}})
		if esNotaEntrega(tipo) {
			bloques = append(bloques,
				texto("recibido", "Recibido por: ______________________________", m, footerY+8, cw*0.6, 6, 9, AlinIzquierda, false),
				texto("ci_fecha", "C.I.: ________________   Fecha: ____________", m, footerY+16, cw*0.6, 6, 9, AlinIzquierda, false),
				texto("firma", "Firma y sello", W-m-56, footerY+16, 56, 6, 8, AlinCentro, false),
			)
		} else {
			bloques = append(bloques,
				Bloque{ID: "totales", Tipo: BloqueTotales, X: W - m - 80, Y: footerY, W: 80, H: footerH, Opciones: map[string]bool{OpTotalesTasa: true, OpTotalesDivisa: true}},
				texto("lbl_obs", "Observaciones", m, footerY, cw-90, 5, 8, AlinIzquierda, true),
				campo("obs", "doc.observaciones", m, footerY+5, cw-90, footerH-5, 8, AlinIzquierda, false),
			)
		}
	} else {
		// ---- Ticket de rollo (angosto) ----
		m := 4.0
		cw := W - 2*m
		bloques = []Bloque{
			campo("emisor_nombre", "emisor.nombre", m, 4, cw, 6, 10, AlinCentro, true),
			campo("emisor_rif", "emisor.rif", m, 10, cw, 4, 8, AlinCentro, false),
			campo("emisor_dir", "emisor.direccion", m, 14, cw, 8, 7, AlinCentro, false),
			texto("titulo", titulo, m, 24, cw, 6, 12, AlinCentro, true),
			campo("numero", "doc.numero", m, 31, cw, 4, 9, AlinCentro, false),
			campo("fecha", "doc.fecha", m, 35, cw, 4, 8, AlinCentro, false),
			{ID: "sep1", Tipo: BloqueSeparador, X: m, Y: 41, W: cw, H: 0.4},
			campo("cliente_nombre", "cliente.nombre", m, 44, cw, 5, 8, AlinIzquierda, false),
			campo("cliente_rif", "cliente.rif", m, 49, cw, 4, 8, AlinIzquierda, false),
			{ID: "items", Tipo: BloqueTablaItems, X: m, Y: 55, W: cw, H: 100, Opciones: map[string]bool{OpItemsMoneda: true, OpItemsExento: true}},
		}
		if esNotaEntrega(tipo) {
			bloques = append(bloques,
				texto("recibido", "Recibido por: ____________________", m, 160, cw, 6, 8, AlinIzquierda, false),
				texto("firma", "Firma", m, 172, cw, 6, 8, AlinCentro, false),
			)
		} else {
			bloques = append(bloques,
				Bloque{ID: "totales", Tipo: BloqueTotales, X: m, Y: 158, W: cw, H: 22, Opciones: map[string]bool{OpTotalesTasa: true, OpTotalesDivisa: true}},
				texto("gracias", "¡Gracias por su compra!", m, 184, cw, 6, 8, AlinCentro, false),
			)
		}
	}

	return Plantilla{
		Nombre:  "Formato " + strings.ToLower(TipoEtiqueta(tipo)),
		Tipo:    tipo,
		Papel:   papel,
		AnchoMM: W,
		AltoMM:  H,
		Bloques: bloques,
		Sedes:   []string{},
	}
}

// CampoDisponible describe un campo dinámico que un bloque `campo` puede mostrar.
// Alimenta el panel del editor; la RESOLUCIÓN del valor real al imprimir es del
// front (o del render del comprobante), no del dominio.
type CampoDisponible struct {
	Clave   string `json:"clave"`   // "emisor.nombre"
	Etiqueta string `json:"etiqueta"` // "Nombre del emisor"
	Grupo   string `json:"grupo"`   // "Emisor" | "Cliente" | "Documento"
}

// CamposDisponibles es el catálogo de campos dinámicos que el editor ofrece.
// Es dato de referencia estable (no depende del tenant): lo consume el front
// para poblar el selector de un bloque de campo.
func CamposDisponibles() []CampoDisponible {
	return []CampoDisponible{
		{"emisor.nombre", "Razón social", "Emisor"},
		{"emisor.rif", "RIF", "Emisor"},
		{"emisor.direccion", "Dirección", "Emisor"},
		{"emisor.telefono", "Teléfono", "Emisor"},
		{"sede.nombre", "Sede", "Emisor"},
		{"cliente.nombre", "Nombre del cliente", "Cliente"},
		{"cliente.rif", "RIF / C.I. del cliente", "Cliente"},
		{"cliente.direccion", "Dirección del cliente", "Cliente"},
		{"cliente.telefono", "Teléfono del cliente", "Cliente"},
		{"doc.tipo", "Tipo de documento", "Documento"},
		{"doc.numero", "Número", "Documento"},
		{"doc.numeroControl", "Número de control", "Documento"},
		{"doc.fecha", "Fecha de emisión", "Documento"},
		{"doc.vencimiento", "Vencimiento", "Documento"},
		{"doc.moneda", "Moneda", "Documento"},
		{"doc.tasa", "Tasa de cambio", "Documento"},
		{"doc.observaciones", "Observaciones", "Documento"},
		// Sello de integridad (SHA-256 encadenado) que el motor calcula al emitir:
		// la "huella" verificable del documento. No es una firma electrónica
		// certificada (PKI/SUSCERTE), sino el sello de inmutabilidad append-only.
		{"doc.hash", "Sello de integridad (hash)", "Documento"},
	}
}
