package fiscal

import "strings"

/* CORRELATIVOS CONFIGURABLES POR TIPO DE DOCUMENTO.
 *
 * Hasta acá el prefijo de cada serie vivía en el código («FL», «FL-NC»…) y el
 * contador solo se podía adelantar DESPUÉS de emitir el primer documento. Las
 * dos cosas fallan en el mismo momento: cuando una empresa migra de otro
 * sistema y necesita decir «mi factura arranca en 1500 con la serie que me
 * autorizaron», antes de vender.
 *
 * QUÉ ES CONFIGURABLE Y QUÉ NO. Es la distinción que ordena todo este archivo:
 *
 *   - Factura, nota de crédito, nota de débito y anulación: el prefijo y el
 *     rango los declara la empresa (o su imprenta autorizada).
 *   - Número de control: el formato lo fija el SENIAT (NN-NNNNNNNN); la empresa
 *     declara el prefijo de 2 dígitos y el rango autorizado.
 *   - Retenciones de IVA e ISLR: el formato es AAAAMM + 8 dígitos y REINICIA
 *     cada mes, por providencia. No hay prefijo que elegir ni rango que
 *     declarar: lo único configurable es el próximo secuencial del período en
 *     curso, para migrar a mitad de mes.
 *
 * Ofrecer un campo que la ley no deja mover sería peor que no ofrecerlo: quien
 * lo llene va a creer que sirve.
 */

// Tipos de documento con correlativo propio.
const (
	SerieFactura       = "factura"
	SerieNotaCredito   = "nota_credito"
	SerieNotaDebito    = "nota_debito"
	SerieAnulacion     = "anulacion"
	SerieRetencionIVA  = "retencion_iva"
	SerieRetencionISLR = "retencion_islr"
	SerieNumeroControl = "numero_control"
)

// SerieDocumento es la configuración del correlativo de un tipo de documento en
// una empresa. El CONTADOR no vive acá —lo lleva el Numerador, atómico por
// empresa+sede+serie—; esto declara con qué prefijo se numera y entre qué
// números autorizados.
type SerieDocumento struct {
	EmpresaID string `json:"empresaId" bson:"empresaid"`
	Tipo      string `json:"tipo" bson:"tipo"`
	// Prefijo es el código de la serie («FL», «A», «FACT-2026»). Vacío = se usa
	// el derivado de la modalidad, que es como funcionaba antes de esto.
	Prefijo string `json:"prefijo" bson:"prefijo"`
	// Desde y Hasta son el RANGO AUTORIZADO. Fijar Desde posiciona el contador en
	// Desde-1 (solo hacia adelante). Hasta en 0 = sin tope declarado.
	Desde       int    `json:"desde" bson:"desde"`
	Hasta       int    `json:"hasta" bson:"hasta"`
	Actualizada string `json:"actualizada" bson:"actualizada"`
}

// TipoSerie describe un tipo para la interfaz, que así no lleva la lista escrita
// a mano ni tiene que saber cuál campo es editable en cuál.
type TipoSerie struct {
	Tipo   string `json:"tipo"`
	Nombre string `json:"nombre"`
	Ayuda  string `json:"ayuda"`
	// PrefijoEditable distingue lo que declara la empresa de lo que fija la ley.
	PrefijoEditable bool `json:"prefijoEditable"`
	// ConRango: si admite [desde, hasta] autorizado.
	ConRango bool `json:"conRango"`
	// PorSede: el contador es por sede (los documentos de venta lo son; el número
	// de control y las retenciones son de la empresa).
	PorSede bool `json:"porSede"`
	// Mensual: el correlativo reinicia cada mes (retenciones, por providencia).
	Mensual bool `json:"mensual"`
}

// TiposSerie es el catálogo de correlativos configurables.
func TiposSerie() []TipoSerie {
	return []TipoSerie{
		{Tipo: SerieFactura, Nombre: "Factura", PrefijoEditable: true, ConRango: true, PorSede: true,
			Ayuda: "Correlativo de las facturas de venta. Su prefijo depende de la modalidad autorizada."},
		{Tipo: SerieNotaCredito, Nombre: "Nota de crédito", PrefijoEditable: true, ConRango: true, PorSede: true,
			Ayuda: "Devoluciones, descuentos y ajustes de precio posteriores a la factura."},
		{Tipo: SerieNotaDebito, Nombre: "Nota de débito", PrefijoEditable: true, ConRango: true, PorSede: true,
			Ayuda: "Cargos posteriores a la factura: diferencial cambiario, mora, gastos."},
		{Tipo: SerieAnulacion, Nombre: "Anulación", PrefijoEditable: true, ConRango: true, PorSede: true,
			Ayuda: "Documento de reversa que deja sin efecto una factura."},
		{Tipo: SerieNumeroControl, Nombre: "Número de control", PrefijoEditable: true, ConRango: true,
			Ayuda: "Correlativo propio del SENIAT (NN-NNNNNNNN), distinto del número de factura. En máquina fiscal lo asigna la impresora."},
		{Tipo: SerieRetencionIVA, Nombre: "Retención de IVA", Mensual: true,
			Ayuda: "Comprobantes emitidos como agente de retención. El formato (AAAAMM + 8 dígitos) y el reinicio mensual los fija el SENIAT."},
		{Tipo: SerieRetencionISLR, Nombre: "Retención de ISLR", Mensual: true,
			Ayuda: "Comprobantes de retención de ISLR. Mismo formato y reinicio mensual que los de IVA."},
	}
}

// TipoSerieDe busca un tipo en el catálogo.
func TipoSerieDe(tipo string) (TipoSerie, bool) {
	for _, t := range TiposSerie() {
		if t.Tipo == tipo {
			return t, true
		}
	}
	return TipoSerie{}, false
}

// SufijoSerie es lo que se le agrega al código de la modalidad para formar la
// serie de cada tipo. Es el comportamiento anterior a la configuración, y sigue
// siendo el valor por defecto: una empresa que no configure nada numera
// exactamente como hasta hoy.
func SufijoSerie(tipo string) string {
	switch tipo {
	case SerieNotaCredito:
		return "-NC"
	case SerieNotaDebito:
		return "-ND"
	case SerieAnulacion:
		return "-NA"
	default:
		return ""
	}
}

// CodigoModalidad es el prefijo histórico según la modalidad de facturación.
func CodigoModalidad(modalidad string) string {
	switch modalidad {
	case "maquina_fiscal":
		return "MF"
	case "imprenta_digital":
		return "ID"
	default:
		return "FL"
	}
}

// PrefijoValido acota el prefijo de una serie de venta: de 1 a 12 caracteres,
// letras, dígitos y guion. Se permite el guion porque las series autorizadas
// suelen traerlo («A-01»), y se prohíbe la barra vertical porque es el separador
// de la clave del contador — dejarla pasar partiría la clave y el correlativo
// terminaría en otra serie sin que nadie lo note.
func PrefijoValido(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" || len(p) > 12 {
		return false
	}
	for _, r := range p {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

// SerieRepository persiste la configuración de correlativos por empresa.
type SerieRepository interface {
	List(empresaID string) []SerieDocumento
	Get(empresaID, tipo string) (SerieDocumento, bool)
	Upsert(s SerieDocumento) SerieDocumento
}
